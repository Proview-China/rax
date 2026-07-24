package knowledge

import (
	"fmt"
	"slices"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

const watchPageTTL = 5 * time.Minute

func sealKnowledgeChange(tenant string, sequence uint64, kind contract.ChangeKind, subject, previous, authority, policy contract.Ref, scope, sensitivity, license string, occurredAt time.Time) (contract.ChangeEventV1, error) {
	return contract.SealChangeEventV1(contract.ChangeEventV1{
		Ref: contract.Ref{ID: fmt.Sprintf("knowledge/change/%d", sequence), Revision: 1}, Owner: contract.OwnerKnowledge,
		TenantID: tenant, Sequence: sequence, Kind: kind, SubjectRef: subject, PreviousRef: previous,
		AuthorityRef: authority, PolicyRef: policy, Scope: scope, Sensitivity: sensitivity, License: license, OccurredAt: occurredAt,
	})
}

// WatchChanges returns a consistent owner-local page of metadata-only changes.
// Its cursor is bound to the exact authority, policy, and Knowledge View.
func (s *Store) WatchChanges(access Access, request contract.WatchRequestV1) (contract.ChangePageV1, error) {
	if err := access.Validate(); err != nil {
		return contract.ChangePageV1{}, err
	}
	now := s.clock.Now().UTC()
	if request.ViewRef.Validate() != nil || request.Limit < 1 || request.Limit > 256 || !request.ExpiresAt.After(now) {
		return contract.ChangePageV1{}, contract.ErrInvalidArgument
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, err := s.tenantLocked(access.TenantID, false)
	if err != nil {
		return contract.ChangePageV1{}, err
	}
	view, ok := viewByRef(t, request.ViewRef)
	if !ok || !accessAllows(access, view.AuthorityRef, view.PolicyRef) || !view.ExpiresAt.After(now) {
		return contract.ChangePageV1{}, contract.ErrNotCurrent
	}
	snapshot, ok := snapshotByRef(t, view.SnapshotRef)
	if !ok || snapshot.State != SnapshotPublished {
		return contract.ChangePageV1{}, contract.ErrNotCurrent
	}
	after, err := knowledgeWatchStart(request.Cursor, access, view.Ref, t.changeSequence, now)
	if err != nil {
		return contract.ChangePageV1{}, err
	}
	events := make([]contract.ChangeEventV1, 0, request.Limit)
	scanned := after
	for _, event := range t.changes {
		if event.Sequence <= after || event.Sequence > t.changeSequence {
			continue
		}
		scanned = event.Sequence
		if !contract.SameRef(event.AuthorityRef, access.AuthorityRef) || !contract.SameRef(event.PolicyRef, access.PolicyRef) || !slices.Contains(view.Scopes, event.Scope) || !slices.Contains(view.AllowedLicenses, event.License) || !sensitivityAtMost(event.Sensitivity, view.SensitivityMax) {
			continue
		}
		events = append(events, event)
		if len(events) == request.Limit {
			break
		}
	}
	if len(events) < request.Limit {
		scanned = t.changeSequence
	}
	expires := minimumWatchTime(request.ExpiresAt.UTC(), view.ExpiresAt.UTC(), now.Add(watchPageTTL))
	cursor, err := contract.SealWatchCursorV1(contract.WatchCursorV1{
		Ref: contract.Ref{ID: "knowledge/watch/" + access.TenantID, Revision: scanned + 1}, Owner: contract.OwnerKnowledge,
		TenantID: access.TenantID, AuthorityRef: access.AuthorityRef, PolicyRef: access.PolicyRef, ViewRef: view.Ref,
		Sequence: scanned, CreatedAt: now, ExpiresAt: expires,
	})
	if err != nil {
		return contract.ChangePageV1{}, err
	}
	boundary := makeKnowledgeChangeBoundary(access.TenantID, t.changeSequence)
	return contract.SealChangePageV1(contract.ChangePageV1{
		Ref: contract.Ref{ID: fmt.Sprintf("knowledge/watch-page/%d/%d", after, t.changeSequence), Revision: 1}, Owner: contract.OwnerKnowledge,
		TenantID: access.TenantID, ViewRef: view.Ref, BoundaryRef: boundary,
		Events: contract.CloneChangeEvents(events), NextCursor: cursor, CreatedAt: now, ExpiresAt: expires,
	})
}

func knowledgeWatchStart(cursor *contract.WatchCursorV1, access Access, view contract.Ref, boundary uint64, now time.Time) (uint64, error) {
	if cursor == nil {
		return 0, nil
	}
	if !cursor.ExpiresAt.After(now) {
		return 0, contract.ErrNotCurrent
	}
	if err := cursor.Validate(now); err != nil {
		return 0, err
	}
	if cursor.Owner != contract.OwnerKnowledge || cursor.TenantID != access.TenantID || !contract.SameRef(cursor.AuthorityRef, access.AuthorityRef) || !contract.SameRef(cursor.PolicyRef, access.PolicyRef) || !contract.SameRef(cursor.ViewRef, view) {
		return 0, contract.ErrScopeDenied
	}
	if cursor.Sequence > boundary {
		return 0, contract.ErrNotCurrent
	}
	return cursor.Sequence, nil
}

func makeKnowledgeChangeBoundary(tenant string, sequence uint64) contract.Ref {
	body := struct {
		Domain   string `json:"domain"`
		TenantID string `json:"tenant_id"`
		Sequence uint64 `json:"sequence"`
	}{Domain: "praxis.knowledge/change-boundary/v1", TenantID: tenant, Sequence: sequence}
	digest, _ := contract.Digest(body)
	return contract.Ref{ID: "knowledge/change-boundary/" + tenant, Revision: sequence + 1, Digest: digest}
}

func minimumWatchTime(values ...time.Time) time.Time {
	minimum := values[0]
	for _, value := range values[1:] {
		if value.Before(minimum) {
			minimum = value
		}
	}
	return minimum
}

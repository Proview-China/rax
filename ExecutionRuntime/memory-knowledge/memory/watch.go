package memory

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

const watchPageTTL = 5 * time.Minute

// WatchChanges returns metadata-only changes visible through one exact Memory
// View. It never reads content and therefore cannot turn a change event into a
// Context fact or a disclosure of record bytes.
func (s *Store) WatchChanges(access Access, request contract.WatchRequestV1) (contract.ChangePageV1, error) {
	if err := access.validate(); err != nil {
		return contract.ChangePageV1{}, err
	}
	now := s.clock.Now().UTC()
	if request.ViewRef.Validate() != nil || request.Limit < 1 || request.Limit > 256 || !request.ExpiresAt.After(now) {
		return contract.ChangePageV1{}, contract.ErrInvalidArgument
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	t := s.tenants[access.TenantID]
	if t == nil {
		return contract.ChangePageV1{}, contract.ErrNotFound
	}
	view, ok := currentViewByRef(t, request.ViewRef)
	if !ok || view.PrincipalID != access.IdentityID || view.AuthorityEpoch != access.AuthorityEpoch || !contract.SameRef(view.AuthorityRef, access.AuthorityRef) || !contract.SameRef(view.PolicyRef, access.PolicyRef) || !view.ExpiresAt.After(now) {
		return contract.ChangePageV1{}, contract.ErrNotCurrent
	}
	after, err := watchStart(request.Cursor, contract.OwnerMemory, access.TenantID, access.AuthorityRef, access.PolicyRef, view.Ref, t.watermark, now)
	if err != nil {
		return contract.ChangePageV1{}, err
	}
	records := make([]Record, 0)
	for _, history := range t.records {
		records = append(records, history...)
	}
	slices.SortFunc(records, func(a, b Record) int {
		if a.Watermark < b.Watermark {
			return -1
		}
		if a.Watermark > b.Watermark {
			return 1
		}
		if compared := strings.Compare(a.Ref.ID, b.Ref.ID); compared != 0 {
			return compared
		}
		if a.Ref.Revision < b.Ref.Revision {
			return -1
		}
		if a.Ref.Revision > b.Ref.Revision {
			return 1
		}
		return strings.Compare(a.Ref.Digest, b.Ref.Digest)
	})
	events := make([]contract.ChangeEventV1, 0, request.Limit)
	scanned := after
	for _, record := range records {
		if record.Watermark <= after || record.Watermark > t.watermark {
			continue
		}
		scanned = record.Watermark
		if record.IdentityID != access.IdentityID || record.AuthorityEpoch != access.AuthorityEpoch || !contract.SameRef(record.AuthorityRef, access.AuthorityRef) || !contract.SameRef(record.PolicyRef, access.PolicyRef) || !contains(view.Scopes, record.Scope) || !sensitivityAllowed(view.SensitivityMax, record.Sensitivity) {
			continue
		}
		event, sealErr := contract.SealChangeEventV1(contract.ChangeEventV1{
			Ref: contract.Ref{ID: fmt.Sprintf("memory/change/%d", record.Watermark), Revision: 1}, Owner: contract.OwnerMemory,
			TenantID: access.TenantID, Sequence: record.Watermark, Kind: memoryChangeKind(record), SubjectRef: record.Ref,
			PreviousRef: record.Corrects, AuthorityRef: record.AuthorityRef, PolicyRef: record.PolicyRef,
			Scope: record.Scope, Sensitivity: record.Sensitivity, OccurredAt: record.CreatedAt,
		})
		if sealErr != nil {
			return contract.ChangePageV1{}, sealErr
		}
		events = append(events, event)
		if len(events) == request.Limit {
			break
		}
	}
	if len(events) < request.Limit {
		scanned = t.watermark
	}
	expires := minimumTime(request.ExpiresAt.UTC(), view.ExpiresAt.UTC(), now.Add(watchPageTTL))
	cursor, err := contract.SealWatchCursorV1(contract.WatchCursorV1{
		Ref: contract.Ref{ID: "memory/watch/" + access.TenantID, Revision: scanned + 1}, Owner: contract.OwnerMemory,
		TenantID: access.TenantID, AuthorityRef: access.AuthorityRef, PolicyRef: access.PolicyRef, ViewRef: view.Ref,
		Sequence: scanned, CreatedAt: now, ExpiresAt: expires,
	})
	if err != nil {
		return contract.ChangePageV1{}, err
	}
	return contract.SealChangePageV1(contract.ChangePageV1{
		Ref: contract.Ref{ID: fmt.Sprintf("memory/watch-page/%d/%d", after, t.watermark), Revision: 1}, Owner: contract.OwnerMemory,
		TenantID: access.TenantID, ViewRef: view.Ref, BoundaryRef: makeWatermark(access.TenantID, t.watermark).Ref,
		Events: events, NextCursor: cursor, CreatedAt: now, ExpiresAt: expires,
	})
}

func watchStart(cursor *contract.WatchCursorV1, owner contract.OwnerDomain, tenant string, authority, policy, view contract.Ref, boundary uint64, now time.Time) (uint64, error) {
	if cursor == nil {
		return 0, nil
	}
	if !cursor.ExpiresAt.After(now) {
		return 0, contract.ErrNotCurrent
	}
	if err := cursor.Validate(now); err != nil {
		return 0, err
	}
	if cursor.Owner != owner || cursor.TenantID != tenant || !contract.SameRef(cursor.AuthorityRef, authority) || !contract.SameRef(cursor.PolicyRef, policy) || !contract.SameRef(cursor.ViewRef, view) {
		return 0, contract.ErrScopeDenied
	}
	if cursor.Sequence > boundary {
		return 0, contract.ErrNotCurrent
	}
	return cursor.Sequence, nil
}

func memoryChangeKind(record Record) contract.ChangeKind {
	switch CandidateKind(record.Kind) {
	case CandidateCorrection, CandidateTombstone:
		return contract.ChangeRecordCorrected
	case CandidatePin:
		return contract.ChangeRecordPinned
	case CandidateArchive:
		return contract.ChangeRecordArchived
	case CandidateForget:
		return contract.ChangeRecordForgotten
	case CandidateMerge:
		return contract.ChangeRecordMerged
	default:
		return contract.ChangeRecordCommitted
	}
}

func minimumTime(values ...time.Time) time.Time {
	minimum := values[0]
	for _, value := range values[1:] {
		if value.Before(minimum) {
			minimum = value
		}
	}
	return minimum
}

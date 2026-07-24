package memory

import (
	"slices"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

type PurgeRequest struct {
	ID                     string        `json:"id"`
	TargetRef              contract.Ref  `json:"target_ref"`
	ScopeRef               contract.Ref  `json:"scope_ref"`
	OperationRef           contract.Ref  `json:"operation_ref"`
	RequestedByRef         contract.Ref  `json:"requested_by_ref"`
	RetentionDecisionRef   contract.Ref  `json:"retention_decision_ref"`
	LegalHoldInspectionRef contract.Ref  `json:"legal_hold_inspection_ref"`
	NotBefore              time.Time     `json:"not_before"`
	TTL                    time.Duration `json:"ttl"`
}

// PreparePurge creates metadata-only Owner intent. It never deletes content;
// execution remains a governed external effect and is explicitly unsupported.
func (s *Store) PreparePurge(access Access, request PurgeRequest) (contract.PurgeIntentV1, error) {
	if err := access.validate(); err != nil {
		return contract.PurgeIntentV1{}, err
	}
	if strings.TrimSpace(request.ID) == "" || request.TargetRef.Validate() != nil || request.ScopeRef.Validate() != nil || request.OperationRef.Validate() != nil || request.RequestedByRef.Validate() != nil || request.RetentionDecisionRef.Validate() != nil || request.LegalHoldInspectionRef.Validate() != nil || request.TTL <= 0 {
		return contract.PurgeIntentV1{}, contract.ErrInvalidArgument
	}
	now := s.clock.Now().UTC()
	notBefore := request.NotBefore.UTC()
	if request.NotBefore.IsZero() {
		notBefore = now
	}
	if notBefore.Before(now) {
		return contract.PurgeIntentV1{}, contract.ErrNotCurrent
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.tenants[access.TenantID]
	if t == nil {
		return contract.PurgeIntentV1{}, contract.ErrNotFound
	}
	if existing, ok := t.purgeIntents[request.ID]; ok {
		if contract.SameRef(existing.TargetRef, request.TargetRef) && contract.SameRef(existing.OperationRef, request.OperationRef) {
			return clonePurgeIntent(existing), nil
		}
		return contract.PurgeIntentV1{}, contract.ErrEvidenceConflict
	}
	history := t.records[request.TargetRef.ID]
	if len(history) == 0 {
		return contract.PurgeIntentV1{}, contract.ErrNotFound
	}
	current := history[len(history)-1]
	if !contract.SameRef(current.Ref, request.TargetRef) || current.Status != RecordTombstoned {
		return contract.PurgeIntentV1{}, contract.ErrNotCurrent
	}
	if current.IdentityID != access.IdentityID || current.AuthorityEpoch != access.AuthorityEpoch || !contract.SameRef(current.AuthorityRef, access.AuthorityRef) || !contract.SameRef(current.PolicyRef, access.PolicyRef) {
		return contract.PurgeIntentV1{}, contract.ErrScopeDenied
	}
	contentsByID := make(map[string]contract.ContentRef)
	for _, record := range history {
		if record.LegalHoldRef != (contract.Ref{}) {
			return contract.PurgeIntentV1{}, contract.ErrScopeDenied
		}
		if record.ContentRef != nil {
			contentsByID[record.ContentRef.ID] = *record.ContentRef
		}
	}
	contents := make([]contract.ContentRef, 0, len(contentsByID))
	for _, content := range contentsByID {
		contents = append(contents, content)
	}
	projections := make([]contract.Ref, 0)
	for _, versions := range t.projections {
		if len(versions) > 0 && versions[len(versions)-1].RecordRef.ID == request.TargetRef.ID {
			projections = append(projections, versions[len(versions)-1].Ref)
		}
	}
	intent, err := contract.SealPurgeIntentV1(contract.PurgeIntentV1{
		Ref: contract.Ref{ID: request.ID, Revision: 1}, Owner: contract.OwnerMemory, TenantID: access.TenantID, TargetKind: "record",
		TargetRef: request.TargetRef, TombstoneRef: request.TargetRef, ContentRefs: contents, ProjectionRefs: projections,
		AuthorityRef: access.AuthorityRef, PolicyRef: access.PolicyRef, ScopeRef: request.ScopeRef, OperationRef: request.OperationRef,
		RequestedByRef: request.RequestedByRef, RetentionDecisionRef: request.RetentionDecisionRef, LegalHoldInspectionRef: request.LegalHoldInspectionRef,
		NotBefore: notBefore, CreatedAt: now, ExpiresAt: now.Add(request.TTL),
	})
	if err != nil {
		return contract.PurgeIntentV1{}, err
	}
	t.purgeIntents[request.ID] = clonePurgeIntent(intent)
	return clonePurgeIntent(intent), nil
}

func (s *Store) InspectPurge(access Access, exact contract.Ref) (contract.PurgeIntentV1, error) {
	if err := access.validate(); err != nil {
		return contract.PurgeIntentV1{}, err
	}
	if exact.Validate() != nil {
		return contract.PurgeIntentV1{}, contract.ErrInvalidArgument
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	t := s.tenants[access.TenantID]
	if t == nil {
		return contract.PurgeIntentV1{}, contract.ErrNotFound
	}
	intent, ok := t.purgeIntents[exact.ID]
	if !ok {
		return contract.PurgeIntentV1{}, contract.ErrNotFound
	}
	if !contract.SameRef(intent.Ref, exact) {
		return contract.PurgeIntentV1{}, contract.ErrEvidenceConflict
	}
	if !contract.SameRef(intent.AuthorityRef, access.AuthorityRef) || !contract.SameRef(intent.PolicyRef, access.PolicyRef) {
		return contract.PurgeIntentV1{}, contract.ErrScopeDenied
	}
	if err := intent.Validate(s.clock.Now()); err != nil {
		return contract.PurgeIntentV1{}, err
	}
	return clonePurgeIntent(intent), nil
}

func clonePurgeIntent(in contract.PurgeIntentV1) contract.PurgeIntentV1 {
	in.ContentRefs = slices.Clone(in.ContentRefs)
	in.ProjectionRefs = slices.Clone(in.ProjectionRefs)
	return in
}

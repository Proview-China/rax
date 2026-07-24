package knowledge

import (
	"slices"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

type PurgeRequest struct {
	ID                     string        `json:"id"`
	TargetKind             string        `json:"target_kind"`
	TargetRef              contract.Ref  `json:"target_ref"`
	ScopeRef               contract.Ref  `json:"scope_ref"`
	OperationRef           contract.Ref  `json:"operation_ref"`
	RequestedByRef         contract.Ref  `json:"requested_by_ref"`
	RetentionDecisionRef   contract.Ref  `json:"retention_decision_ref"`
	LegalHoldInspectionRef contract.Ref  `json:"legal_hold_inspection_ref"`
	NotBefore              time.Time     `json:"not_before"`
	TTL                    time.Duration `json:"ttl"`
}

// PreparePurge freezes the exact derived Knowledge material a governed purge
// would affect. AssetRef bytes remain owned by Asset Manager and are never
// listed as Knowledge-owned content.
func (s *Store) PreparePurge(access Access, request PurgeRequest) (contract.PurgeIntentV1, error) {
	if err := access.Validate(); err != nil {
		return contract.PurgeIntentV1{}, err
	}
	if strings.TrimSpace(request.ID) == "" || (request.TargetKind != "source" && request.TargetKind != "record") || request.TargetRef.Validate() != nil || request.ScopeRef.Validate() != nil || request.OperationRef.Validate() != nil || request.RequestedByRef.Validate() != nil || request.RetentionDecisionRef.Validate() != nil || request.LegalHoldInspectionRef.Validate() != nil || request.TTL <= 0 {
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
	t, err := s.tenantLocked(access.TenantID, false)
	if err != nil {
		return contract.PurgeIntentV1{}, err
	}
	if existing, ok := t.purgeIntents[request.ID]; ok {
		if contract.SameRef(existing.TargetRef, request.TargetRef) && contract.SameRef(existing.OperationRef, request.OperationRef) {
			return clonePurgeIntent(existing), nil
		}
		return contract.PurgeIntentV1{}, contract.ErrEvidenceConflict
	}
	tomb, ok := t.tombstones[request.TargetKind+"/"+request.TargetRef.ID]
	if !ok || !contract.SameRef(tomb.TargetRef, request.TargetRef) {
		return contract.PurgeIntentV1{}, contract.ErrNotCurrent
	}
	contentsByID := make(map[string]contract.ContentRef)
	recordIDs := make(map[string]struct{})
	var authority, policy contract.Ref
	switch request.TargetKind {
	case "source":
		history := t.sources[request.TargetRef.ID]
		if len(history) == 0 || history[len(history)-1].State != SourceWithdrawn || history[len(history)-1].Ref.Revision != request.TargetRef.Revision+1 {
			return contract.PurgeIntentV1{}, contract.ErrNotCurrent
		}
		source, found := sourceByRef(t, request.TargetRef)
		if !found || !accessAllows(access, source.AuthorityRef, source.PolicyRef) {
			return contract.PurgeIntentV1{}, contract.ErrScopeDenied
		}
		authority, policy = source.AuthorityRef, source.PolicyRef
		for id, records := range t.records {
			for _, record := range records {
				if hasRefID(record.SourceRefs, request.TargetRef.ID) {
					recordIDs[id] = struct{}{}
					contentsByID[record.ContentRef.ID] = record.ContentRef
				}
			}
		}
	case "record":
		history := t.records[request.TargetRef.ID]
		if len(history) == 0 || history[len(history)-1].Status != RecordWithdrawn || history[len(history)-1].Ref.Revision != request.TargetRef.Revision+1 {
			return contract.PurgeIntentV1{}, contract.ErrNotCurrent
		}
		record, found := recordByRef(t, request.TargetRef)
		if !found || !accessAllows(access, record.AuthorityRef, record.PolicyRef) {
			return contract.PurgeIntentV1{}, contract.ErrScopeDenied
		}
		authority, policy = record.AuthorityRef, record.PolicyRef
		recordIDs[request.TargetRef.ID] = struct{}{}
		for _, item := range history {
			contentsByID[item.ContentRef.ID] = item.ContentRef
		}
	}
	contents := make([]contract.ContentRef, 0, len(contentsByID))
	for _, content := range contentsByID {
		contents = append(contents, content)
	}
	projections := make([]contract.Ref, 0)
	for _, history := range t.projections {
		if len(history) == 0 {
			continue
		}
		current := history[len(history)-1]
		for _, recordRef := range current.RecordRefs {
			if _, affected := recordIDs[recordRef.ID]; affected {
				projections = append(projections, current.Ref)
				break
			}
		}
	}
	intent, err := contract.SealPurgeIntentV1(contract.PurgeIntentV1{
		Ref: contract.Ref{ID: request.ID, Revision: 1}, Owner: contract.OwnerKnowledge, TenantID: access.TenantID, TargetKind: request.TargetKind,
		TargetRef: request.TargetRef, TombstoneRef: tomb.Ref, ContentRefs: contents, ProjectionRefs: projections,
		AuthorityRef: authority, PolicyRef: policy, ScopeRef: request.ScopeRef, OperationRef: request.OperationRef,
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
	if err := access.Validate(); err != nil {
		return contract.PurgeIntentV1{}, err
	}
	if exact.Validate() != nil {
		return contract.PurgeIntentV1{}, contract.ErrInvalidArgument
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, err := s.tenantLocked(access.TenantID, false)
	if err != nil {
		return contract.PurgeIntentV1{}, err
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

func hasRefID(refs []contract.Ref, id string) bool {
	for _, ref := range refs {
		if ref.ID == id {
			return true
		}
	}
	return false
}

func clonePurgeIntent(in contract.PurgeIntentV1) contract.PurgeIntentV1 {
	in.ContentRefs = slices.Clone(in.ContentRefs)
	in.ProjectionRefs = slices.Clone(in.ProjectionRefs)
	return in
}

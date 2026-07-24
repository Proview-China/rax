package memory

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

type historyDerivationCandidateKeyV1 struct {
	tenantID, scopeDigest, candidateID string
}

type historyDerivationRequestKeyV1 struct {
	tenantID, scopeDigest, idempotencyKey string
}

func (b *Backend) CreateHistoryDerivationCandidateFactV1(_ context.Context, fact contract.HistoryDerivationCandidateFactV1) (contract.HistoryDerivationCandidateFactV1, bool, error) {
	if err := fact.Validate(); err != nil {
		return contract.HistoryDerivationCandidateFactV1{}, false, err
	}
	key := historyDerivationCandidateKeyV1{fact.Scope.TenantID, fact.Scope.ExecutionScopeDigest, fact.CandidateID}
	requestKey := historyDerivationRequestKeyV1{key.tenantID, key.scopeDigest, fact.IdempotencyKey}
	b.mu.Lock()
	defer b.mu.Unlock()
	if existingKey, ok := b.historyDerivationByRequestV1[requestKey]; ok && existingKey != key {
		return contract.HistoryDerivationCandidateFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "idempotency_key", "request already created another History Derivation Candidate")
	}
	if existing, ok := b.historyDerivationCandidatesV1[key]; ok {
		if existing.Ref().Exact().Equal(fact.Ref().Exact()) && existing.IdempotencyKey == fact.IdempotencyKey && existing.RequestDigest == fact.RequestDigest {
			return existing.Clone(), true, nil
		}
		return contract.HistoryDerivationCandidateFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "candidate_id", "create-once History Derivation Candidate changed content")
	}
	b.historyDerivationCandidatesV1[key] = fact.Clone()
	b.historyDerivationByRequestV1[requestKey] = key
	return fact.Clone(), false, nil
}

func (b *Backend) InspectHistoryDerivationCandidateV1(_ context.Context, request ports.InspectHistoryDerivationCandidateRequestV1) (contract.HistoryDerivationCandidateFactV1, error) {
	if err := request.Ref.Validate(); err != nil {
		return contract.HistoryDerivationCandidateFactV1{}, err
	}
	ref := request.Ref.Exact()
	key := historyDerivationCandidateKeyV1{ref.TenantID, ref.ScopeDigest, ref.ID}
	b.mu.RLock()
	defer b.mu.RUnlock()
	fact, ok := b.historyDerivationCandidatesV1[key]
	if !ok {
		return contract.HistoryDerivationCandidateFactV1{}, contract.NewError(contract.ErrNotFound, "history_derivation_ref", "History Derivation Candidate not found")
	}
	if !fact.Ref().Exact().Equal(ref) {
		return contract.HistoryDerivationCandidateFactV1{}, contract.NewError(contract.ErrRevisionConflict, "history_derivation_ref", "exact History Derivation Candidate ref mismatch")
	}
	return fact.Clone(), nil
}

func (b *Backend) InspectHistoryDerivationCandidateByIDV1(_ context.Context, request ports.InspectHistoryDerivationCandidateByIDRequestV1) (contract.HistoryDerivationCandidateFactV1, error) {
	if err := validateHistoryDerivationByIDRequestV1(request); err != nil {
		return contract.HistoryDerivationCandidateFactV1{}, err
	}
	key := historyDerivationCandidateKeyV1{request.TenantID, request.ScopeDigest, request.CandidateID}
	b.mu.RLock()
	defer b.mu.RUnlock()
	fact, ok := b.historyDerivationCandidatesV1[key]
	if !ok {
		return contract.HistoryDerivationCandidateFactV1{}, contract.NewError(contract.ErrNotFound, "history_derivation_id", "History Derivation Candidate not found")
	}
	if fact.Owner != request.Owner {
		return contract.HistoryDerivationCandidateFactV1{}, contract.NewError(contract.ErrRevisionConflict, "owner_binding", "History Derivation Candidate Owner mismatch")
	}
	return fact.Clone(), nil
}

func validateHistoryDerivationByIDRequestV1(request ports.InspectHistoryDerivationCandidateByIDRequestV1) error {
	for field, value := range map[string]string{"tenant_id": request.TenantID, "scope_digest": request.ScopeDigest, "candidate_id": request.CandidateID} {
		if err := contract.ValidateToken(field, value); err != nil {
			return err
		}
	}
	if err := request.Owner.Validate(); err != nil {
		return err
	}
	if request.Owner.ComponentID != contract.ContinuityComponentID || request.Owner.Capability != contract.HistoryDerivationCapabilityV1 || request.Owner.FactKind != "history_derivation_candidate_fact_v1" {
		return contract.NewError(contract.ErrInvalidArgument, "owner_binding", "wrong Continuity History Derivation owner")
	}
	return nil
}

var _ ports.HistoryDerivationCandidateReaderV1 = (*Backend)(nil)

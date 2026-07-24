package memory

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

type contentDeltaKeyV1 struct {
	tenantID    string
	scopeDigest string
	deltaID     string
}

type contentDeltaRequestKeyV1 struct {
	tenantID       string
	scopeDigest    string
	idempotencyKey string
}

func (b *Backend) CreateContentDeltaFactV1(_ context.Context, fact contract.ContentDeltaFactV1) (contract.ContentDeltaFactV1, bool, error) {
	if err := fact.Validate(); err != nil {
		return contract.ContentDeltaFactV1{}, false, err
	}
	key := contentDeltaKeyV1{tenantID: fact.Scope.TenantID, scopeDigest: fact.Scope.ExecutionScopeDigest, deltaID: fact.DeltaID}
	requestKey := contentDeltaRequestKeyV1{tenantID: key.tenantID, scopeDigest: key.scopeDigest, idempotencyKey: fact.IdempotencyKey}
	b.mu.Lock()
	defer b.mu.Unlock()
	if existingKey, ok := b.contentDeltaByRequestV1[requestKey]; ok && existingKey != key {
		return contract.ContentDeltaFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "idempotency_key", "request already created another Content Delta")
	}
	if existing, ok := b.contentDeltasV1[key]; ok {
		if existing.Ref().Exact().Equal(fact.Ref().Exact()) && existing.IdempotencyKey == fact.IdempotencyKey && existing.RequestDigest == fact.RequestDigest {
			return existing.Clone(), true, nil
		}
		return contract.ContentDeltaFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "delta_id", "create-once Content Delta changed content")
	}
	b.contentDeltasV1[key] = fact.Clone()
	b.contentDeltaByRequestV1[requestKey] = key
	return fact.Clone(), false, nil
}

func (b *Backend) InspectContentDeltaV1(_ context.Context, request ports.InspectContentDeltaRequestV1) (contract.ContentDeltaFactV1, error) {
	if err := request.Ref.Validate(); err != nil {
		return contract.ContentDeltaFactV1{}, err
	}
	ref := request.Ref.Exact()
	key := contentDeltaKeyV1{tenantID: ref.TenantID, scopeDigest: ref.ScopeDigest, deltaID: ref.ID}
	b.mu.RLock()
	defer b.mu.RUnlock()
	fact, ok := b.contentDeltasV1[key]
	if !ok {
		return contract.ContentDeltaFactV1{}, contract.NewError(contract.ErrNotFound, "content_delta_ref", "Content Delta not found")
	}
	if !fact.Ref().Exact().Equal(ref) {
		return contract.ContentDeltaFactV1{}, contract.NewError(contract.ErrRevisionConflict, "content_delta_ref", "exact Content Delta ref mismatch")
	}
	return fact.Clone(), nil
}

func (b *Backend) InspectContentDeltaByIDV1(_ context.Context, request ports.InspectContentDeltaByIDRequestV1) (contract.ContentDeltaFactV1, error) {
	if err := validateContentDeltaByIDRequestV1(request); err != nil {
		return contract.ContentDeltaFactV1{}, err
	}
	key := contentDeltaKeyV1{tenantID: request.TenantID, scopeDigest: request.ScopeDigest, deltaID: request.DeltaID}
	b.mu.RLock()
	defer b.mu.RUnlock()
	fact, ok := b.contentDeltasV1[key]
	if !ok {
		return contract.ContentDeltaFactV1{}, contract.NewError(contract.ErrNotFound, "content_delta_id", "Content Delta not found")
	}
	if fact.Owner != request.Owner {
		return contract.ContentDeltaFactV1{}, contract.NewError(contract.ErrRevisionConflict, "owner_binding", "Content Delta Owner mismatch")
	}
	return fact.Clone(), nil
}

func validateContentDeltaByIDRequestV1(request ports.InspectContentDeltaByIDRequestV1) error {
	for field, value := range map[string]string{"tenant_id": request.TenantID, "scope_digest": request.ScopeDigest, "delta_id": request.DeltaID} {
		if err := contract.ValidateToken(field, value); err != nil {
			return err
		}
	}
	if err := request.Owner.Validate(); err != nil {
		return err
	}
	if request.Owner.ComponentID != contract.ContinuityComponentID || request.Owner.Capability != contract.ContentDeltaCapabilityV1 || request.Owner.FactKind != "content_delta_fact_v1" {
		return contract.NewError(contract.ErrInvalidArgument, "owner_binding", "wrong Continuity Content Delta owner")
	}
	return nil
}

var _ ports.ContentDeltaReaderV1 = (*Backend)(nil)

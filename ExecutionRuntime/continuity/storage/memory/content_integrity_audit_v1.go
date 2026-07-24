package memory

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

type contentIntegrityAuditKeyV1 struct {
	tenantID    string
	scopeDigest string
	auditID     string
}

type contentIntegrityAuditRequestKeyV1 struct {
	tenantID       string
	scopeDigest    string
	idempotencyKey string
}

func (b *Backend) CreateContentIntegrityAuditFactV1(_ context.Context, fact contract.ContentIntegrityAuditFactV1) (contract.ContentIntegrityAuditFactV1, bool, error) {
	if err := fact.Validate(); err != nil {
		return contract.ContentIntegrityAuditFactV1{}, false, err
	}
	key := contentIntegrityAuditKeyV1{tenantID: fact.Scope.TenantID, scopeDigest: fact.Scope.ExecutionScopeDigest, auditID: fact.AuditID}
	requestKey := contentIntegrityAuditRequestKeyV1{tenantID: key.tenantID, scopeDigest: key.scopeDigest, idempotencyKey: fact.IdempotencyKey}
	b.mu.Lock()
	defer b.mu.Unlock()
	if existingKey, ok := b.contentIntegrityAuditByRequestV1[requestKey]; ok && existingKey != key {
		return contract.ContentIntegrityAuditFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "idempotency_key", "request already created another Content Integrity Audit")
	}
	if existing, ok := b.contentIntegrityAuditsV1[key]; ok {
		if existing.Ref().Exact().Equal(fact.Ref().Exact()) && existing.IdempotencyKey == fact.IdempotencyKey && existing.RequestDigest == fact.RequestDigest {
			return existing.Clone(), true, nil
		}
		return contract.ContentIntegrityAuditFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "audit_id", "create-once Content Integrity Audit changed content")
	}
	b.contentIntegrityAuditsV1[key] = fact.Clone()
	b.contentIntegrityAuditByRequestV1[requestKey] = key
	return fact.Clone(), false, nil
}

func (b *Backend) InspectContentIntegrityAuditV1(_ context.Context, request ports.InspectContentIntegrityAuditRequestV1) (contract.ContentIntegrityAuditFactV1, error) {
	if err := request.Ref.Validate(); err != nil {
		return contract.ContentIntegrityAuditFactV1{}, err
	}
	ref := request.Ref.Exact()
	key := contentIntegrityAuditKeyV1{tenantID: ref.TenantID, scopeDigest: ref.ScopeDigest, auditID: ref.ID}
	b.mu.RLock()
	defer b.mu.RUnlock()
	fact, ok := b.contentIntegrityAuditsV1[key]
	if !ok {
		return contract.ContentIntegrityAuditFactV1{}, contract.NewError(contract.ErrNotFound, "content_integrity_audit_ref", "Content Integrity Audit not found")
	}
	if !fact.Ref().Exact().Equal(ref) {
		return contract.ContentIntegrityAuditFactV1{}, contract.NewError(contract.ErrRevisionConflict, "content_integrity_audit_ref", "exact Content Integrity Audit ref mismatch")
	}
	return fact.Clone(), nil
}

func (b *Backend) InspectContentIntegrityAuditByIDV1(_ context.Context, request ports.InspectContentIntegrityAuditByIDRequestV1) (contract.ContentIntegrityAuditFactV1, error) {
	if err := validateContentIntegrityAuditByIDRequestV1(request); err != nil {
		return contract.ContentIntegrityAuditFactV1{}, err
	}
	key := contentIntegrityAuditKeyV1{tenantID: request.TenantID, scopeDigest: request.ScopeDigest, auditID: request.AuditID}
	b.mu.RLock()
	defer b.mu.RUnlock()
	fact, ok := b.contentIntegrityAuditsV1[key]
	if !ok {
		return contract.ContentIntegrityAuditFactV1{}, contract.NewError(contract.ErrNotFound, "content_integrity_audit_id", "Content Integrity Audit not found")
	}
	if fact.Owner != request.Owner {
		return contract.ContentIntegrityAuditFactV1{}, contract.NewError(contract.ErrRevisionConflict, "owner_binding", "Content Integrity Audit Owner mismatch")
	}
	return fact.Clone(), nil
}

func validateContentIntegrityAuditByIDRequestV1(request ports.InspectContentIntegrityAuditByIDRequestV1) error {
	for field, value := range map[string]string{"tenant_id": request.TenantID, "scope_digest": request.ScopeDigest, "audit_id": request.AuditID} {
		if err := contract.ValidateToken(field, value); err != nil {
			return err
		}
	}
	if err := request.Owner.Validate(); err != nil {
		return err
	}
	if request.Owner.ComponentID != contract.ContinuityComponentID || request.Owner.Capability != contract.ContentIntegrityAuditCapabilityV1 || request.Owner.FactKind != "content_integrity_audit_fact_v1" {
		return contract.NewError(contract.ErrInvalidArgument, "owner_binding", "wrong Continuity Content Integrity Audit owner")
	}
	return nil
}

var _ ports.ContentIntegrityAuditReaderV1 = (*Backend)(nil)

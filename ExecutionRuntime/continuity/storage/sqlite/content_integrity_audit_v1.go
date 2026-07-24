package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

func (s *Store) CreateContentIntegrityAuditFactV1(ctx context.Context, fact contract.ContentIntegrityAuditFactV1) (contract.ContentIntegrityAuditFactV1, bool, error) {
	if err := fact.Validate(); err != nil {
		return contract.ContentIntegrityAuditFactV1{}, false, err
	}
	body, _, err := encode(fact)
	if err != nil {
		return contract.ContentIntegrityAuditFactV1{}, false, err
	}
	tenant, scope, id := fact.Scope.TenantID, fact.Scope.ExecutionScopeDigest, fact.AuditID
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contract.ContentIntegrityAuditFactV1{}, false, unavailable("begin Content Integrity Audit create", err)
	}
	defer tx.Rollback()
	var boundID string
	err = tx.QueryRowContext(ctx, "SELECT audit_id FROM content_integrity_audit_facts WHERE tenant_id=? AND scope_digest=? AND idempotency_key=?", tenant, scope, fact.IdempotencyKey).Scan(&boundID)
	if err == nil && boundID != id {
		return contract.ContentIntegrityAuditFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "idempotency_key", "request already created another Content Integrity Audit")
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return contract.ContentIntegrityAuditFactV1{}, false, unavailable("inspect Content Integrity Audit idempotency", err)
	}
	existing, found, err := inspectContentIntegrityAuditByIDTxV1(ctx, tx, tenant, scope, id)
	if err != nil {
		return contract.ContentIntegrityAuditFactV1{}, false, err
	}
	if found {
		if existing.Ref().Exact().Equal(fact.Ref().Exact()) && existing.IdempotencyKey == fact.IdempotencyKey && existing.RequestDigest == fact.RequestDigest {
			return existing.Clone(), true, tx.Commit()
		}
		return contract.ContentIntegrityAuditFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "audit_id", "create-once Content Integrity Audit changed content")
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO content_integrity_audit_facts(
		tenant_id,scope_digest,audit_id,idempotency_key,request_digest,ref_digest,body)
		VALUES(?,?,?,?,?,?,?)`, tenant, scope, id, fact.IdempotencyKey, fact.RequestDigest, fact.Digest, body); err != nil {
		return contract.ContentIntegrityAuditFactV1{}, false, unavailable("insert Content Integrity Audit", err)
	}
	if err = tx.Commit(); err != nil {
		return contract.ContentIntegrityAuditFactV1{}, false, unavailable("commit Content Integrity Audit", err)
	}
	return fact.Clone(), false, nil
}

func (s *Store) InspectContentIntegrityAuditV1(ctx context.Context, request ports.InspectContentIntegrityAuditRequestV1) (contract.ContentIntegrityAuditFactV1, error) {
	if err := s.validateCall(ctx); err != nil {
		return contract.ContentIntegrityAuditFactV1{}, err
	}
	if err := request.Ref.Validate(); err != nil {
		return contract.ContentIntegrityAuditFactV1{}, err
	}
	ref := request.Ref.Exact()
	fact, err := s.inspectContentIntegrityAuditByIDV1(ctx, ref.TenantID, ref.ScopeDigest, ref.ID)
	if err != nil {
		return contract.ContentIntegrityAuditFactV1{}, err
	}
	if !fact.Ref().Exact().Equal(ref) {
		return contract.ContentIntegrityAuditFactV1{}, contract.NewError(contract.ErrRevisionConflict, "content_integrity_audit_ref", "exact Content Integrity Audit ref mismatch")
	}
	return fact.Clone(), nil
}

func (s *Store) InspectContentIntegrityAuditByIDV1(ctx context.Context, request ports.InspectContentIntegrityAuditByIDRequestV1) (contract.ContentIntegrityAuditFactV1, error) {
	if err := s.validateCall(ctx); err != nil {
		return contract.ContentIntegrityAuditFactV1{}, err
	}
	if err := validateContentIntegrityAuditByIDRequestV1(request); err != nil {
		return contract.ContentIntegrityAuditFactV1{}, err
	}
	fact, err := s.inspectContentIntegrityAuditByIDV1(ctx, request.TenantID, request.ScopeDigest, request.AuditID)
	if err != nil {
		return contract.ContentIntegrityAuditFactV1{}, err
	}
	if fact.Owner != request.Owner {
		return contract.ContentIntegrityAuditFactV1{}, contract.NewError(contract.ErrRevisionConflict, "owner_binding", "Content Integrity Audit Owner mismatch")
	}
	return fact.Clone(), nil
}

func (s *Store) inspectContentIntegrityAuditByIDV1(ctx context.Context, tenant, scope, id string) (contract.ContentIntegrityAuditFactV1, error) {
	var body []byte
	if err := s.db.QueryRowContext(ctx, "SELECT body FROM content_integrity_audit_facts WHERE tenant_id=? AND scope_digest=? AND audit_id=?", tenant, scope, id).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.ContentIntegrityAuditFactV1{}, notFound("content_integrity_audit_id", "Content Integrity Audit not found")
		}
		return contract.ContentIntegrityAuditFactV1{}, unavailable("inspect Content Integrity Audit", err)
	}
	return decodeContentIntegrityAuditV1(body)
}

func inspectContentIntegrityAuditByIDTxV1(ctx context.Context, tx *sql.Tx, tenant, scope, id string) (contract.ContentIntegrityAuditFactV1, bool, error) {
	var body []byte
	err := tx.QueryRowContext(ctx, "SELECT body FROM content_integrity_audit_facts WHERE tenant_id=? AND scope_digest=? AND audit_id=?", tenant, scope, id).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.ContentIntegrityAuditFactV1{}, false, nil
	}
	if err != nil {
		return contract.ContentIntegrityAuditFactV1{}, false, unavailable("inspect Content Integrity Audit", err)
	}
	fact, err := decodeContentIntegrityAuditV1(body)
	return fact, true, err
}

func decodeContentIntegrityAuditV1(body []byte) (contract.ContentIntegrityAuditFactV1, error) {
	var fact contract.ContentIntegrityAuditFactV1
	if err := decode(body, &fact); err != nil {
		return contract.ContentIntegrityAuditFactV1{}, err
	}
	if err := fact.Validate(); err != nil {
		return contract.ContentIntegrityAuditFactV1{}, contract.NewError(contract.ErrContentDigestMismatch, "content_integrity_audit", "stored Content Integrity Audit failed validation")
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

var _ ports.ContentIntegrityAuditReaderV1 = (*Store)(nil)

package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

func (s *Store) CreateContentDeltaFactV1(ctx context.Context, fact contract.ContentDeltaFactV1) (contract.ContentDeltaFactV1, bool, error) {
	if err := fact.Validate(); err != nil {
		return contract.ContentDeltaFactV1{}, false, err
	}
	body, _, err := encode(fact)
	if err != nil {
		return contract.ContentDeltaFactV1{}, false, err
	}
	tenant, scope, id := fact.Scope.TenantID, fact.Scope.ExecutionScopeDigest, fact.DeltaID
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contract.ContentDeltaFactV1{}, false, unavailable("begin Content Delta create", err)
	}
	defer tx.Rollback()
	var boundID string
	err = tx.QueryRowContext(ctx, "SELECT delta_id FROM content_delta_facts WHERE tenant_id=? AND scope_digest=? AND idempotency_key=?", tenant, scope, fact.IdempotencyKey).Scan(&boundID)
	if err == nil && boundID != id {
		return contract.ContentDeltaFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "idempotency_key", "request already created another Content Delta")
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return contract.ContentDeltaFactV1{}, false, unavailable("inspect Content Delta idempotency", err)
	}
	existing, found, err := inspectContentDeltaByIDTxV1(ctx, tx, tenant, scope, id)
	if err != nil {
		return contract.ContentDeltaFactV1{}, false, err
	}
	if found {
		if existing.Ref().Exact().Equal(fact.Ref().Exact()) && existing.IdempotencyKey == fact.IdempotencyKey && existing.RequestDigest == fact.RequestDigest {
			return existing.Clone(), true, tx.Commit()
		}
		return contract.ContentDeltaFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "delta_id", "create-once Content Delta changed content")
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO content_delta_facts(
		tenant_id,scope_digest,delta_id,idempotency_key,request_digest,base_object_id,target_object_id,ref_digest,body)
		VALUES(?,?,?,?,?,?,?,?,?)`, tenant, scope, id, fact.IdempotencyKey, fact.RequestDigest, fact.Base.ObjectID, fact.Target.ObjectID, fact.Digest, body); err != nil {
		return contract.ContentDeltaFactV1{}, false, unavailable("insert Content Delta", err)
	}
	if err = tx.Commit(); err != nil {
		return contract.ContentDeltaFactV1{}, false, unavailable("commit Content Delta", err)
	}
	return fact.Clone(), false, nil
}

func (s *Store) InspectContentDeltaV1(ctx context.Context, request ports.InspectContentDeltaRequestV1) (contract.ContentDeltaFactV1, error) {
	if err := s.validateCall(ctx); err != nil {
		return contract.ContentDeltaFactV1{}, err
	}
	if err := request.Ref.Validate(); err != nil {
		return contract.ContentDeltaFactV1{}, err
	}
	ref := request.Ref.Exact()
	fact, err := s.inspectContentDeltaByIDV1(ctx, ref.TenantID, ref.ScopeDigest, ref.ID)
	if err != nil {
		return contract.ContentDeltaFactV1{}, err
	}
	if !fact.Ref().Exact().Equal(ref) {
		return contract.ContentDeltaFactV1{}, contract.NewError(contract.ErrRevisionConflict, "content_delta_ref", "exact Content Delta ref mismatch")
	}
	return fact.Clone(), nil
}

func (s *Store) InspectContentDeltaByIDV1(ctx context.Context, request ports.InspectContentDeltaByIDRequestV1) (contract.ContentDeltaFactV1, error) {
	if err := s.validateCall(ctx); err != nil {
		return contract.ContentDeltaFactV1{}, err
	}
	if err := validateContentDeltaByIDRequestV1(request); err != nil {
		return contract.ContentDeltaFactV1{}, err
	}
	fact, err := s.inspectContentDeltaByIDV1(ctx, request.TenantID, request.ScopeDigest, request.DeltaID)
	if err != nil {
		return contract.ContentDeltaFactV1{}, err
	}
	if fact.Owner != request.Owner {
		return contract.ContentDeltaFactV1{}, contract.NewError(contract.ErrRevisionConflict, "owner_binding", "Content Delta Owner mismatch")
	}
	return fact.Clone(), nil
}

func (s *Store) inspectContentDeltaByIDV1(ctx context.Context, tenant, scope, id string) (contract.ContentDeltaFactV1, error) {
	var body []byte
	if err := s.db.QueryRowContext(ctx, "SELECT body FROM content_delta_facts WHERE tenant_id=? AND scope_digest=? AND delta_id=?", tenant, scope, id).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.ContentDeltaFactV1{}, notFound("content_delta_id", "Content Delta not found")
		}
		return contract.ContentDeltaFactV1{}, unavailable("inspect Content Delta", err)
	}
	return decodeContentDeltaV1(body)
}

func inspectContentDeltaByIDTxV1(ctx context.Context, tx *sql.Tx, tenant, scope, id string) (contract.ContentDeltaFactV1, bool, error) {
	var body []byte
	err := tx.QueryRowContext(ctx, "SELECT body FROM content_delta_facts WHERE tenant_id=? AND scope_digest=? AND delta_id=?", tenant, scope, id).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.ContentDeltaFactV1{}, false, nil
	}
	if err != nil {
		return contract.ContentDeltaFactV1{}, false, unavailable("inspect Content Delta", err)
	}
	fact, err := decodeContentDeltaV1(body)
	return fact, true, err
}

func decodeContentDeltaV1(body []byte) (contract.ContentDeltaFactV1, error) {
	var fact contract.ContentDeltaFactV1
	if err := decode(body, &fact); err != nil {
		return contract.ContentDeltaFactV1{}, err
	}
	if err := fact.Validate(); err != nil {
		return contract.ContentDeltaFactV1{}, contract.NewError(contract.ErrContentDigestMismatch, "content_delta", "stored Content Delta failed validation")
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

var _ ports.ContentDeltaReaderV1 = (*Store)(nil)

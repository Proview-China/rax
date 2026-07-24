package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

func (s *Store) CreateRewindPlanFactV2(ctx context.Context, plan contract.RewindPlanFactV2) (contract.RewindPlanFactV2, bool, error) {
	if err := plan.Validate(); err != nil {
		return contract.RewindPlanFactV2{}, false, err
	}
	if plan.Revision != 1 || plan.State != contract.RewindPlanDraftV2 || plan.UpdatedUnixNano != plan.CreatedUnixNano {
		return contract.RewindPlanFactV2{}, false, contract.NewError(contract.ErrInvalidArgument, "rewind_plan_create", "revision 1 draft is required")
	}
	if err := plan.ValidateCurrent(s.clock()); err != nil {
		return contract.RewindPlanFactV2{}, false, err
	}
	body, _, err := encode(plan)
	if err != nil {
		return contract.RewindPlanFactV2{}, false, err
	}
	tenant, scope, id := plan.Scope.TenantID, plan.Scope.ExecutionScopeDigest, plan.PlanID
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contract.RewindPlanFactV2{}, false, unavailable("begin Rewind Plan create", err)
	}
	defer tx.Rollback()
	var bound string
	err = tx.QueryRowContext(ctx, "SELECT plan_id FROM rewind_plan_idempotency WHERE tenant_id=? AND scope_digest=? AND idempotency_key=?", tenant, scope, plan.IdempotencyKey).Scan(&bound)
	if err == nil && bound != id {
		return contract.RewindPlanFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "idempotency_key", "request already created another Rewind Plan")
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return contract.RewindPlanFactV2{}, false, unavailable("inspect Rewind Plan idempotency", err)
	}
	_, found, err := inspectCurrentRewindPlanTx(ctx, tx, tenant, scope, id)
	if err != nil {
		return contract.RewindPlanFactV2{}, false, err
	}
	if found {
		first, err := inspectRewindPlanRevisionTx(ctx, tx, tenant, scope, id, 1)
		if err == nil && first.Ref().Exact().Equal(plan.Ref().Exact()) {
			return first, true, tx.Commit()
		}
		return contract.RewindPlanFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "rewind_plan_id", "create-once Rewind Plan identity changed")
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO rewind_plan_history(tenant_id,scope_digest,plan_id,revision,ref_digest,body) VALUES(?,?,?,?,?,?)", tenant, scope, id, 1, plan.Ref().Exact().Digest, body); err != nil {
		return contract.RewindPlanFactV2{}, false, unavailable("insert Rewind Plan history", err)
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO rewind_plan_current(tenant_id,scope_digest,plan_id,revision) VALUES(?,?,?,1)", tenant, scope, id); err != nil {
		return contract.RewindPlanFactV2{}, false, unavailable("insert Rewind Plan current", err)
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO rewind_plan_idempotency(tenant_id,scope_digest,idempotency_key,plan_id) VALUES(?,?,?,?)", tenant, scope, plan.IdempotencyKey, id); err != nil {
		return contract.RewindPlanFactV2{}, false, unavailable("insert Rewind Plan idempotency", err)
	}
	if err = tx.Commit(); err != nil {
		return contract.RewindPlanFactV2{}, false, unavailable("commit Rewind Plan create", err)
	}
	return plan.Clone(), false, nil
}

func (s *Store) CompareAndSwapRewindPlanFactV2(ctx context.Context, expected contract.RewindPlanRefV2, next contract.RewindPlanFactV2) (contract.RewindPlanFactV2, bool, error) {
	if err := expected.Validate(); err != nil {
		return contract.RewindPlanFactV2{}, false, err
	}
	if err := next.Validate(); err != nil {
		return contract.RewindPlanFactV2{}, false, err
	}
	ref := expected.Exact()
	if ref.TenantID != next.Scope.TenantID || ref.ScopeDigest != next.Scope.ExecutionScopeDigest || ref.ID != next.PlanID {
		return contract.RewindPlanFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "rewind_plan_key", "tenant, scope, or Plan ID changed")
	}
	body, _, err := encode(next)
	if err != nil {
		return contract.RewindPlanFactV2{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contract.RewindPlanFactV2{}, false, unavailable("begin Rewind Plan CAS", err)
	}
	defer tx.Rollback()
	current, found, err := inspectCurrentRewindPlanTx(ctx, tx, ref.TenantID, ref.ScopeDigest, ref.ID)
	if err != nil {
		return contract.RewindPlanFactV2{}, false, err
	}
	if !found {
		return contract.RewindPlanFactV2{}, false, notFound("rewind_plan_key", "Rewind Plan not found")
	}
	if current.Revision == ref.Revision+1 && current.Ref().Exact().Equal(next.Ref().Exact()) {
		return current, true, tx.Commit()
	}
	if !current.Ref().Exact().Equal(ref) {
		return contract.RewindPlanFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "rewind_plan_ref", "CAS expected ref is not current")
	}
	if next.Revision != current.Revision+1 || !contract.SameRewindPlanStableIdentityV2(current, next) {
		return contract.RewindPlanFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "rewind_plan_identity", "CAS changed immutable identity or skipped a revision")
	}
	now := s.clock()
	if next.UpdatedUnixNano < current.UpdatedUnixNano || next.UpdatedUnixNano > now.UnixNano() {
		return contract.RewindPlanFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "updated_unix_nano", "Rewind Plan update time is invalid")
	}
	if err := contract.AdvanceRewindPlanStateV2(current, next.State, now); err != nil {
		return contract.RewindPlanFactV2{}, false, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO rewind_plan_history(tenant_id,scope_digest,plan_id,revision,ref_digest,body) VALUES(?,?,?,?,?,?)", ref.TenantID, ref.ScopeDigest, ref.ID, next.Revision, next.Ref().Exact().Digest, body); err != nil {
		return contract.RewindPlanFactV2{}, false, unavailable("insert Rewind Plan revision", err)
	}
	result, err := tx.ExecContext(ctx, "UPDATE rewind_plan_current SET revision=? WHERE tenant_id=? AND scope_digest=? AND plan_id=? AND revision=?", next.Revision, ref.TenantID, ref.ScopeDigest, ref.ID, ref.Revision)
	if err != nil {
		return contract.RewindPlanFactV2{}, false, unavailable("update Rewind Plan current", err)
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return contract.RewindPlanFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "rewind_plan_ref", "Rewind Plan current CAS lost")
	}
	if err = tx.Commit(); err != nil {
		return contract.RewindPlanFactV2{}, false, unavailable("commit Rewind Plan CAS", err)
	}
	return next.Clone(), false, nil
}

func (s *Store) InspectRewindPlanV2(ctx context.Context, request ports.InspectRewindPlanRequestV2) (contract.RewindPlanFactV2, error) {
	if err := s.validateCall(ctx); err != nil {
		return contract.RewindPlanFactV2{}, err
	}
	if err := request.Ref.Validate(); err != nil {
		return contract.RewindPlanFactV2{}, err
	}
	ref := request.Ref.Exact()
	var body []byte
	if err := s.db.QueryRowContext(ctx, "SELECT body FROM rewind_plan_history WHERE tenant_id=? AND scope_digest=? AND plan_id=? AND revision=?", ref.TenantID, ref.ScopeDigest, ref.ID, ref.Revision).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.RewindPlanFactV2{}, notFound("rewind_plan_revision", "Rewind Plan revision not found")
		}
		return contract.RewindPlanFactV2{}, unavailable("inspect Rewind Plan", err)
	}
	plan, err := decodeRewindPlan(body)
	if err != nil {
		return contract.RewindPlanFactV2{}, err
	}
	if !plan.Ref().Exact().Equal(ref) {
		return contract.RewindPlanFactV2{}, contract.NewError(contract.ErrRevisionConflict, "rewind_plan_ref", "exact Rewind Plan ref or Owner mismatch")
	}
	return plan, nil
}

func (s *Store) InspectCurrentRewindPlanV2(ctx context.Context, request ports.InspectCurrentRewindPlanRequestV2) (contract.RewindPlanFactV2, error) {
	if err := s.validateCall(ctx); err != nil {
		return contract.RewindPlanFactV2{}, err
	}
	if err := request.Validate(); err != nil {
		return contract.RewindPlanFactV2{}, err
	}
	var body []byte
	err := s.db.QueryRowContext(ctx, `SELECT h.body FROM rewind_plan_current c
		JOIN rewind_plan_history h ON h.tenant_id=c.tenant_id AND h.scope_digest=c.scope_digest AND h.plan_id=c.plan_id AND h.revision=c.revision
		WHERE c.tenant_id=? AND c.scope_digest=? AND c.plan_id=?`, request.TenantID, request.ScopeDigest, request.PlanID).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.RewindPlanFactV2{}, notFound("rewind_plan_key", "Rewind Plan not found")
	}
	if err != nil {
		return contract.RewindPlanFactV2{}, unavailable("inspect current Rewind Plan", err)
	}
	plan, err := decodeRewindPlan(body)
	if err != nil {
		return contract.RewindPlanFactV2{}, err
	}
	if plan.Owner != request.Owner {
		return contract.RewindPlanFactV2{}, contract.NewError(contract.ErrRevisionConflict, "owner_binding", "current Rewind Plan Owner mismatch")
	}
	return plan, nil
}

func inspectCurrentRewindPlanTx(ctx context.Context, tx *sql.Tx, tenant, scope, id string) (contract.RewindPlanFactV2, bool, error) {
	var body []byte
	err := tx.QueryRowContext(ctx, `SELECT h.body FROM rewind_plan_current c
		JOIN rewind_plan_history h ON h.tenant_id=c.tenant_id AND h.scope_digest=c.scope_digest AND h.plan_id=c.plan_id AND h.revision=c.revision
		WHERE c.tenant_id=? AND c.scope_digest=? AND c.plan_id=?`, tenant, scope, id).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.RewindPlanFactV2{}, false, nil
	}
	if err != nil {
		return contract.RewindPlanFactV2{}, false, unavailable("inspect current Rewind Plan transaction", err)
	}
	plan, err := decodeRewindPlan(body)
	return plan, err == nil, err
}

func inspectRewindPlanRevisionTx(ctx context.Context, tx *sql.Tx, tenant, scope, id string, revision uint64) (contract.RewindPlanFactV2, error) {
	var body []byte
	if err := tx.QueryRowContext(ctx, "SELECT body FROM rewind_plan_history WHERE tenant_id=? AND scope_digest=? AND plan_id=? AND revision=?", tenant, scope, id, revision).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.RewindPlanFactV2{}, notFound("rewind_plan_revision", "Rewind Plan revision not found")
		}
		return contract.RewindPlanFactV2{}, unavailable("inspect Rewind Plan revision", err)
	}
	return decodeRewindPlan(body)
}

func decodeRewindPlan(body []byte) (contract.RewindPlanFactV2, error) {
	var plan contract.RewindPlanFactV2
	if err := decode(body, &plan); err != nil {
		return contract.RewindPlanFactV2{}, err
	}
	if err := plan.Validate(); err != nil {
		return contract.RewindPlanFactV2{}, contract.NewError(contract.ErrContentDigestMismatch, "rewind_plan", "stored Rewind Plan failed validation")
	}
	return plan.Clone(), nil
}

var _ ports.RewindPlanRepositoryV2 = (*Store)(nil)

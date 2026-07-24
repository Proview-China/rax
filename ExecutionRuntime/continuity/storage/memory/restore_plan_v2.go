package memory

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

func restorePlanKeyFromFactV2(plan contract.RestorePlanFactV2) checkpointObjectKeyV2 {
	return checkpointObjectKeyV2{tenantID: plan.Scope.TenantID, scopeDigest: plan.Scope.ExecutionScopeDigest, id: plan.PlanID}
}

func (b *Backend) CreateRestorePlanFactV2(_ context.Context, plan contract.RestorePlanFactV2) (contract.RestorePlanFactV2, bool, error) {
	if err := plan.Validate(); err != nil {
		return contract.RestorePlanFactV2{}, false, err
	}
	if plan.Revision != 1 || plan.State != contract.RestorePlanDraftV2 || plan.UpdatedUnixNano != plan.CreatedUnixNano {
		return contract.RestorePlanFactV2{}, false, contract.NewError(contract.ErrInvalidArgument, "restore_plan_create", "revision 1 draft is required")
	}
	if err := plan.ValidateCurrent(b.clock()); err != nil {
		return contract.RestorePlanFactV2{}, false, err
	}
	key := restorePlanKeyFromFactV2(plan)
	requestKey := checkpointRequestKeyV2{tenantID: key.tenantID, scopeDigest: key.scopeDigest, idempotencyKey: plan.IdempotencyKey}
	b.mu.Lock()
	defer b.mu.Unlock()
	if existingKey, ok := b.restorePlanByRequestV2[requestKey]; ok && existingKey != key {
		return contract.RestorePlanFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "idempotency_key", "request already created another Restore Plan in this tenant scope")
	}
	if history := b.restorePlansV2[key]; history != nil {
		if first, ok := history[1]; ok && first.Ref().Exact().Equal(plan.Ref().Exact()) {
			return first.Clone(), true, nil
		}
		return contract.RestorePlanFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "restore_plan_id", "create-once Restore Plan identity changed")
	}
	b.restorePlansV2[key] = map[uint64]contract.RestorePlanFactV2{1: plan.Clone()}
	b.restorePlanCurrentV2[key] = 1
	b.restorePlanByRequestV2[requestKey] = key
	return plan.Clone(), false, nil
}

func (b *Backend) CompareAndSwapRestorePlanFactV2(_ context.Context, expected contract.RestorePlanRefV2, next contract.RestorePlanFactV2) (contract.RestorePlanFactV2, bool, error) {
	if err := expected.Validate(); err != nil {
		return contract.RestorePlanFactV2{}, false, err
	}
	if err := next.Validate(); err != nil {
		return contract.RestorePlanFactV2{}, false, err
	}
	key := objectKeyFromExactRefV2(expected.Exact())
	if key != restorePlanKeyFromFactV2(next) {
		return contract.RestorePlanFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "restore_plan_key", "tenant, scope, or Plan ID changed")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	history := b.restorePlansV2[key]
	if history == nil {
		return contract.RestorePlanFactV2{}, false, contract.NewError(contract.ErrNotFound, "restore_plan_key", "Restore Plan not found")
	}
	current := history[b.restorePlanCurrentV2[key]]
	if current.Revision == expected.Exact().Revision+1 && current.Ref().Exact().Equal(next.Ref().Exact()) {
		return current.Clone(), true, nil
	}
	if !current.Ref().Exact().Equal(expected.Exact()) {
		return contract.RestorePlanFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "restore_plan_ref", "CAS expected ref is not current")
	}
	if next.Revision != current.Revision+1 || !contract.SameRestorePlanStableIdentityV2(current, next) {
		return contract.RestorePlanFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "restore_plan_identity", "CAS changed immutable identity or skipped a revision")
	}
	now := b.clock()
	if next.UpdatedUnixNano < current.UpdatedUnixNano || next.UpdatedUnixNano > now.UnixNano() {
		return contract.RestorePlanFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "updated_unix_nano", "Restore Plan update time is invalid")
	}
	if err := contract.AdvanceRestorePlanStateV2(current, next.State, now); err != nil {
		return contract.RestorePlanFactV2{}, false, err
	}
	if _, exists := history[next.Revision]; exists {
		return contract.RestorePlanFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "restore_plan_revision", "history revision already exists")
	}
	history[next.Revision] = next.Clone()
	b.restorePlanCurrentV2[key] = next.Revision
	return next.Clone(), false, nil
}

func (b *Backend) InspectRestorePlanV2(_ context.Context, request ports.InspectRestorePlanRequestV2) (contract.RestorePlanFactV2, error) {
	if err := request.Ref.Validate(); err != nil {
		return contract.RestorePlanFactV2{}, err
	}
	ref := request.Ref.Exact()
	key := objectKeyFromExactRefV2(ref)
	b.mu.RLock()
	defer b.mu.RUnlock()
	history := b.restorePlansV2[key]
	if history == nil {
		return contract.RestorePlanFactV2{}, contract.NewError(contract.ErrNotFound, "restore_plan_key", "Restore Plan not found")
	}
	plan, ok := history[ref.Revision]
	if !ok {
		return contract.RestorePlanFactV2{}, contract.NewError(contract.ErrNotFound, "restore_plan_revision", "Restore Plan revision not found")
	}
	if !plan.Ref().Exact().Equal(ref) {
		return contract.RestorePlanFactV2{}, contract.NewError(contract.ErrRevisionConflict, "restore_plan_ref", "exact Restore Plan ref or Owner mismatch")
	}
	return plan.Clone(), nil
}

func (b *Backend) InspectCurrentRestorePlanV2(_ context.Context, request ports.InspectCurrentRestorePlanRequestV2) (contract.RestorePlanFactV2, error) {
	if err := request.Validate(); err != nil {
		return contract.RestorePlanFactV2{}, err
	}
	key := checkpointObjectKeyV2{tenantID: request.TenantID, scopeDigest: request.ScopeDigest, id: request.PlanID}
	b.mu.RLock()
	defer b.mu.RUnlock()
	history := b.restorePlansV2[key]
	if history == nil {
		return contract.RestorePlanFactV2{}, contract.NewError(contract.ErrNotFound, "restore_plan_key", "Restore Plan not found")
	}
	plan := history[b.restorePlanCurrentV2[key]]
	if plan.Owner != request.Owner {
		return contract.RestorePlanFactV2{}, contract.NewError(contract.ErrRevisionConflict, "owner_binding", "current Restore Plan Owner mismatch")
	}
	return plan.Clone(), nil
}

var _ ports.RestorePlanRepositoryV2 = (*Backend)(nil)

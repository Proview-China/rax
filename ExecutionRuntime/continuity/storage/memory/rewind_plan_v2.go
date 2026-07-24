package memory

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

func rewindPlanKeyFromFactV2(plan contract.RewindPlanFactV2) checkpointObjectKeyV2 {
	return checkpointObjectKeyV2{tenantID: plan.Scope.TenantID, scopeDigest: plan.Scope.ExecutionScopeDigest, id: plan.PlanID}
}

func (b *Backend) CreateRewindPlanFactV2(_ context.Context, plan contract.RewindPlanFactV2) (contract.RewindPlanFactV2, bool, error) {
	if err := plan.Validate(); err != nil {
		return contract.RewindPlanFactV2{}, false, err
	}
	if plan.Revision != 1 || plan.State != contract.RewindPlanDraftV2 || plan.UpdatedUnixNano != plan.CreatedUnixNano {
		return contract.RewindPlanFactV2{}, false, contract.NewError(contract.ErrInvalidArgument, "rewind_plan_create", "revision 1 draft is required")
	}
	if err := plan.ValidateCurrent(b.clock()); err != nil {
		return contract.RewindPlanFactV2{}, false, err
	}
	key := rewindPlanKeyFromFactV2(plan)
	requestKey := checkpointRequestKeyV2{tenantID: key.tenantID, scopeDigest: key.scopeDigest, idempotencyKey: plan.IdempotencyKey}
	b.mu.Lock()
	defer b.mu.Unlock()
	if existingKey, ok := b.rewindPlanByRequestV2[requestKey]; ok && existingKey != key {
		return contract.RewindPlanFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "idempotency_key", "request already created another Rewind Plan in this tenant scope")
	}
	if history := b.rewindPlansV2[key]; history != nil {
		if first, ok := history[1]; ok && first.Ref().Exact().Equal(plan.Ref().Exact()) {
			return first.Clone(), true, nil
		}
		return contract.RewindPlanFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "rewind_plan_id", "create-once Rewind Plan identity changed")
	}
	b.rewindPlansV2[key] = map[uint64]contract.RewindPlanFactV2{1: plan.Clone()}
	b.rewindPlanCurrentV2[key] = 1
	b.rewindPlanByRequestV2[requestKey] = key
	return plan.Clone(), false, nil
}

func (b *Backend) CompareAndSwapRewindPlanFactV2(_ context.Context, expected contract.RewindPlanRefV2, next contract.RewindPlanFactV2) (contract.RewindPlanFactV2, bool, error) {
	if err := expected.Validate(); err != nil {
		return contract.RewindPlanFactV2{}, false, err
	}
	if err := next.Validate(); err != nil {
		return contract.RewindPlanFactV2{}, false, err
	}
	key := objectKeyFromExactRefV2(expected.Exact())
	if key != rewindPlanKeyFromFactV2(next) {
		return contract.RewindPlanFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "rewind_plan_key", "tenant, scope, or Plan ID changed")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	history := b.rewindPlansV2[key]
	if history == nil {
		return contract.RewindPlanFactV2{}, false, contract.NewError(contract.ErrNotFound, "rewind_plan_key", "Rewind Plan not found")
	}
	current := history[b.rewindPlanCurrentV2[key]]
	if current.Revision == expected.Exact().Revision+1 && current.Ref().Exact().Equal(next.Ref().Exact()) {
		return current.Clone(), true, nil
	}
	if !current.Ref().Exact().Equal(expected.Exact()) {
		return contract.RewindPlanFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "rewind_plan_ref", "CAS expected ref is not current")
	}
	if next.Revision != current.Revision+1 || !contract.SameRewindPlanStableIdentityV2(current, next) {
		return contract.RewindPlanFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "rewind_plan_identity", "CAS changed immutable identity or skipped a revision")
	}
	now := b.clock()
	if next.UpdatedUnixNano < current.UpdatedUnixNano || next.UpdatedUnixNano > now.UnixNano() {
		return contract.RewindPlanFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "updated_unix_nano", "Rewind Plan update time is invalid")
	}
	if err := contract.AdvanceRewindPlanStateV2(current, next.State, now); err != nil {
		return contract.RewindPlanFactV2{}, false, err
	}
	if _, exists := history[next.Revision]; exists {
		return contract.RewindPlanFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "rewind_plan_revision", "history revision already exists")
	}
	history[next.Revision] = next.Clone()
	b.rewindPlanCurrentV2[key] = next.Revision
	return next.Clone(), false, nil
}

func (b *Backend) InspectRewindPlanV2(_ context.Context, request ports.InspectRewindPlanRequestV2) (contract.RewindPlanFactV2, error) {
	if err := request.Ref.Validate(); err != nil {
		return contract.RewindPlanFactV2{}, err
	}
	ref := request.Ref.Exact()
	key := objectKeyFromExactRefV2(ref)
	b.mu.RLock()
	defer b.mu.RUnlock()
	history := b.rewindPlansV2[key]
	if history == nil {
		return contract.RewindPlanFactV2{}, contract.NewError(contract.ErrNotFound, "rewind_plan_key", "Rewind Plan not found")
	}
	plan, ok := history[ref.Revision]
	if !ok {
		return contract.RewindPlanFactV2{}, contract.NewError(contract.ErrNotFound, "rewind_plan_revision", "Rewind Plan revision not found")
	}
	if !plan.Ref().Exact().Equal(ref) {
		return contract.RewindPlanFactV2{}, contract.NewError(contract.ErrRevisionConflict, "rewind_plan_ref", "exact Rewind Plan ref or Owner mismatch")
	}
	return plan.Clone(), nil
}

func (b *Backend) InspectCurrentRewindPlanV2(_ context.Context, request ports.InspectCurrentRewindPlanRequestV2) (contract.RewindPlanFactV2, error) {
	if err := request.Validate(); err != nil {
		return contract.RewindPlanFactV2{}, err
	}
	key := checkpointObjectKeyV2{tenantID: request.TenantID, scopeDigest: request.ScopeDigest, id: request.PlanID}
	b.mu.RLock()
	defer b.mu.RUnlock()
	history := b.rewindPlansV2[key]
	if history == nil {
		return contract.RewindPlanFactV2{}, contract.NewError(contract.ErrNotFound, "rewind_plan_key", "Rewind Plan not found")
	}
	plan := history[b.rewindPlanCurrentV2[key]]
	if plan.Owner != request.Owner {
		return contract.RewindPlanFactV2{}, contract.NewError(contract.ErrRevisionConflict, "owner_binding", "current Rewind Plan Owner mismatch")
	}
	return plan.Clone(), nil
}

var _ ports.RewindPlanRepositoryV2 = (*Backend)(nil)

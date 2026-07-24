package domain

import (
	"context"
	"errors"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

type RewindPlanControllerV2 struct {
	repository ports.RewindPlanRepositoryV2
	clock      Clock
}

func NewRewindPlanControllerV2(repository ports.RewindPlanRepositoryV2, clock Clock) (*RewindPlanControllerV2, error) {
	if nilInterfaceV2(repository) || nilInterfaceV2(clock) {
		return nil, contract.NewError(contract.ErrInvalidArgument, "rewind_plan_controller", "repository and clock are required")
	}
	return &RewindPlanControllerV2{repository: repository, clock: clock}, nil
}

func (c *RewindPlanControllerV2) CreateRewindPlanV2(ctx context.Context, request ports.CreateRewindPlanRequestV2) (contract.RewindPlanFactV2, bool, error) {
	if ctx == nil || !request.ExpectAbsent {
		return contract.RewindPlanFactV2{}, false, contract.NewError(contract.ErrInvalidArgument, "rewind_plan_create", "context and expectAbsent=true are required")
	}
	plan := request.Candidate.Clone()
	if err := plan.Validate(); err != nil {
		return contract.RewindPlanFactV2{}, false, err
	}
	if plan.Revision != 1 || plan.State != contract.RewindPlanDraftV2 || plan.UpdatedUnixNano != plan.CreatedUnixNano {
		return contract.RewindPlanFactV2{}, false, contract.NewError(contract.ErrInvalidArgument, "rewind_plan_create", "revision 1 draft with equal creation/update time is required")
	}
	if err := plan.ValidateCurrent(c.clock.Now()); err != nil {
		return contract.RewindPlanFactV2{}, false, err
	}
	created, replay, err := c.repository.CreateRewindPlanFactV2(ctx, plan)
	if err == nil {
		return created.Clone(), replay, nil
	}
	inspected, inspectErr := c.repository.InspectRewindPlanV2(context.WithoutCancel(ctx), ports.InspectRewindPlanRequestV2{Ref: plan.Ref()})
	if inspectErr != nil {
		return contract.RewindPlanFactV2{}, false, errors.Join(err, inspectErr)
	}
	if !inspected.Ref().Exact().Equal(plan.Ref().Exact()) {
		return contract.RewindPlanFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "rewind_plan_create", "durable create differs from the original exact plan")
	}
	return inspected.Clone(), true, nil
}

func (c *RewindPlanControllerV2) InspectRewindPlanV2(ctx context.Context, request ports.InspectRewindPlanRequestV2) (contract.RewindPlanFactV2, error) {
	if ctx == nil {
		return contract.RewindPlanFactV2{}, contract.NewError(contract.ErrInvalidArgument, "context", "context is required")
	}
	if err := request.Ref.Validate(); err != nil {
		return contract.RewindPlanFactV2{}, err
	}
	plan, err := c.repository.InspectRewindPlanV2(ctx, request)
	if err != nil {
		return contract.RewindPlanFactV2{}, err
	}
	if err := plan.Validate(); err != nil || !plan.Ref().Exact().Equal(request.Ref.Exact()) {
		return contract.RewindPlanFactV2{}, contract.NewError(contract.ErrRevisionConflict, "rewind_plan_ref", "repository returned a non-exact Rewind Plan")
	}
	return plan.Clone(), nil
}

func (c *RewindPlanControllerV2) InspectCurrentRewindPlanV2(ctx context.Context, request ports.InspectCurrentRewindPlanRequestV2) (contract.RewindPlanFactV2, error) {
	if ctx == nil {
		return contract.RewindPlanFactV2{}, contract.NewError(contract.ErrInvalidArgument, "context", "context is required")
	}
	if err := request.Validate(); err != nil {
		return contract.RewindPlanFactV2{}, err
	}
	current, err := c.repository.InspectCurrentRewindPlanV2(ctx, request)
	if err != nil {
		return contract.RewindPlanFactV2{}, err
	}
	if err := current.ValidateCurrent(c.clock.Now()); err != nil {
		return contract.RewindPlanFactV2{}, err
	}
	if current.Owner != request.Owner || current.Scope.TenantID != request.TenantID || current.Scope.ExecutionScopeDigest != request.ScopeDigest || current.PlanID != request.PlanID {
		return contract.RewindPlanFactV2{}, contract.NewError(contract.ErrRevisionConflict, "rewind_plan_current", "repository returned another current Rewind Plan")
	}
	return current.Clone(), nil
}

func (c *RewindPlanControllerV2) CompareAndSwapRewindPlanV2(ctx context.Context, request ports.CompareAndSwapRewindPlanRequestV2) (contract.RewindPlanFactV2, bool, error) {
	if ctx == nil {
		return contract.RewindPlanFactV2{}, false, contract.NewError(contract.ErrInvalidArgument, "context", "context is required")
	}
	if err := request.Expected.Validate(); err != nil {
		return contract.RewindPlanFactV2{}, false, err
	}
	current, err := c.InspectRewindPlanV2(ctx, ports.InspectRewindPlanRequestV2{Ref: request.Expected})
	if err != nil {
		return contract.RewindPlanFactV2{}, false, err
	}
	now := c.clock.Now()
	if err := contract.AdvanceRewindPlanStateV2(current, request.NextState, now); err != nil {
		return contract.RewindPlanFactV2{}, false, err
	}
	next := current.Clone()
	next.Revision++
	next.State = request.NextState
	next.UpdatedUnixNano = now.UnixNano()
	next.Digest = ""
	digest, err := next.CanonicalDigest()
	if err != nil {
		return contract.RewindPlanFactV2{}, false, err
	}
	next.Digest = digest
	if err := next.Validate(); err != nil {
		return contract.RewindPlanFactV2{}, false, err
	}
	updated, replay, err := c.repository.CompareAndSwapRewindPlanFactV2(ctx, request.Expected, next)
	if err == nil {
		return updated.Clone(), replay, nil
	}
	inspected, inspectErr := c.repository.InspectRewindPlanV2(context.WithoutCancel(ctx), ports.InspectRewindPlanRequestV2{Ref: next.Ref()})
	if inspectErr != nil {
		return contract.RewindPlanFactV2{}, false, errors.Join(err, inspectErr)
	}
	if !inspected.Ref().Exact().Equal(next.Ref().Exact()) {
		return contract.RewindPlanFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "rewind_plan_cas", "durable CAS differs from the original exact transition")
	}
	return inspected.Clone(), true, nil
}

var _ ports.RewindPlanGovernancePortV2 = (*RewindPlanControllerV2)(nil)

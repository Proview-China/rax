package domain

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

type RestorePlanControllerV2 struct {
	repository ports.RestorePlanRepositoryV2
	clock      Clock
}

func NewRestorePlanControllerV2(repository ports.RestorePlanRepositoryV2, clock Clock) (*RestorePlanControllerV2, error) {
	if nilInterfaceV2(repository) || nilInterfaceV2(clock) {
		return nil, contract.NewError(contract.ErrInvalidArgument, "restore_plan_controller", "repository and clock are required")
	}
	return &RestorePlanControllerV2{repository: repository, clock: clock}, nil
}

func (c *RestorePlanControllerV2) CreateRestorePlanV2(ctx context.Context, request ports.CreateRestorePlanRequestV2) (contract.RestorePlanFactV2, bool, error) {
	if !request.ExpectAbsent {
		return contract.RestorePlanFactV2{}, false, contract.NewError(contract.ErrInvalidArgument, "expect_absent", "create-once requires expectAbsent=true")
	}
	plan := request.Candidate.Clone()
	if err := plan.Validate(); err != nil {
		return contract.RestorePlanFactV2{}, false, err
	}
	if plan.Revision != 1 || plan.State != contract.RestorePlanDraftV2 || plan.UpdatedUnixNano != plan.CreatedUnixNano {
		return contract.RestorePlanFactV2{}, false, contract.NewError(contract.ErrInvalidArgument, "restore_plan_create", "revision 1 draft with equal creation/update time is required")
	}
	if err := plan.ValidateCurrent(c.clock.Now()); err != nil {
		return contract.RestorePlanFactV2{}, false, err
	}
	return c.repository.CreateRestorePlanFactV2(ctx, plan)
}

func (c *RestorePlanControllerV2) InspectRestorePlanV2(ctx context.Context, request ports.InspectRestorePlanRequestV2) (contract.RestorePlanFactV2, error) {
	if err := request.Ref.Validate(); err != nil {
		return contract.RestorePlanFactV2{}, err
	}
	return c.repository.InspectRestorePlanV2(ctx, request)
}

func (c *RestorePlanControllerV2) InspectCurrentRestorePlanV2(ctx context.Context, request ports.InspectCurrentRestorePlanRequestV2) (contract.RestorePlanFactV2, error) {
	if err := request.Validate(); err != nil {
		return contract.RestorePlanFactV2{}, err
	}
	current, err := c.repository.InspectCurrentRestorePlanV2(ctx, request)
	if err != nil {
		return contract.RestorePlanFactV2{}, err
	}
	if err := current.ValidateCurrent(c.clock.Now()); err != nil {
		return contract.RestorePlanFactV2{}, err
	}
	return current, nil
}

func (c *RestorePlanControllerV2) CompareAndSwapRestorePlanV2(ctx context.Context, request ports.CompareAndSwapRestorePlanRequestV2) (contract.RestorePlanFactV2, bool, error) {
	if err := request.Expected.Validate(); err != nil {
		return contract.RestorePlanFactV2{}, false, err
	}
	current, err := c.repository.InspectRestorePlanV2(ctx, ports.InspectRestorePlanRequestV2{Ref: request.Expected})
	if err != nil {
		return contract.RestorePlanFactV2{}, false, err
	}
	now := c.clock.Now()
	if err := contract.AdvanceRestorePlanStateV2(current, request.NextState, now); err != nil {
		return contract.RestorePlanFactV2{}, false, err
	}
	next := current.Clone()
	next.Revision++
	next.State = request.NextState
	next.UpdatedUnixNano = now.UnixNano()
	next.Digest = ""
	digest, err := next.CanonicalDigest()
	if err != nil {
		return contract.RestorePlanFactV2{}, false, err
	}
	next.Digest = digest
	if err := next.Validate(); err != nil {
		return contract.RestorePlanFactV2{}, false, err
	}
	return c.repository.CompareAndSwapRestorePlanFactV2(ctx, request.Expected, next)
}

var _ ports.RestorePlanGovernancePortV2 = (*RestorePlanControllerV2)(nil)

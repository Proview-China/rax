package application

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type NextModelTurnEligibilityConfigV1 struct {
	Continuation applicationports.TurnContinuationCurrentReaderV1
	Clock        func() time.Time
}

// NextModelTurnEligibilityV1 is the read-only Application half-slice between
// a completed ActiveContext CAS and a future Harness V2 dispatch adapter. Its
// single Continuation read is an advisory snapshot only: dispatch must fresh-read
// Harness and Runtime currentness again. It writes no facts and never calls a
// Model or Provider.
type NextModelTurnEligibilityV1 struct {
	config NextModelTurnEligibilityConfigV1
}

func NewNextModelTurnEligibilityV1(config NextModelTurnEligibilityConfigV1) (*NextModelTurnEligibilityV1, error) {
	if nilInterfaceV1(config.Continuation) || config.Clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "next Model Turn eligibility requires continuation current and clock readers")
	}
	return &NextModelTurnEligibilityV1{config: config}, nil
}

func (i *NextModelTurnEligibilityV1) InspectNextModelTurnEligibilityV1(
	ctx context.Context,
	request contract.NextModelTurnEligibilityRequestV1,
) (contract.NextModelTurnEligibilityProjectionV1, error) {
	if i == nil || nilInterfaceV1(ctx) {
		return contract.NextModelTurnEligibilityProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "next Model Turn eligibility inspector or context is nil")
	}
	if err := ctx.Err(); err != nil {
		return contract.NextModelTurnEligibilityProjectionV1{}, err
	}
	now, err := i.nextClockV1(time.Time{})
	if err != nil {
		return contract.NextModelTurnEligibilityProjectionV1{}, err
	}
	if err = request.ValidateCurrent(now); err != nil {
		return contract.NextModelTurnEligibilityProjectionV1{}, err
	}

	inspect, err := contract.SealTurnContinuationInspectRequestV1(contract.TurnContinuationInspectRequestV1{
		AttemptRef: request.ContinuationAttempt,
	})
	if err != nil {
		return contract.NextModelTurnEligibilityProjectionV1{}, err
	}
	current, err := i.config.Continuation.InspectTurnContinuationV1(ctx, inspect)
	if err != nil {
		return contract.NextModelTurnEligibilityProjectionV1{}, err
	}
	now, err = i.nextClockV1(now)
	if err != nil {
		return contract.NextModelTurnEligibilityProjectionV1{}, err
	}
	if err = validateNextModelTurnContinuationV1(current, request, now); err != nil {
		return contract.NextModelTurnEligibilityProjectionV1{}, err
	}
	if err = ctx.Err(); err != nil {
		return contract.NextModelTurnEligibilityProjectionV1{}, err
	}
	now, err = i.nextClockV1(now)
	if err != nil {
		return contract.NextModelTurnEligibilityProjectionV1{}, err
	}
	if err = request.ValidateCurrent(now); err != nil {
		return contract.NextModelTurnEligibilityProjectionV1{}, err
	}
	if err = validateNextModelTurnContinuationV1(current, request, now); err != nil {
		return contract.NextModelTurnEligibilityProjectionV1{}, err
	}
	return contract.SealNextModelTurnEligibilityProjectionV1(request, current.ExpiresUnixNano, now)
}

func (i *NextModelTurnEligibilityV1) nextClockV1(previous time.Time) (time.Time, error) {
	if i == nil || i.config.Clock == nil {
		return time.Time{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "next Model Turn eligibility clock is unavailable")
	}
	now := i.config.Clock()
	if now.IsZero() || now.UnixNano() <= 0 || !previous.IsZero() && now.Before(previous) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "next Model Turn eligibility clock regressed")
	}
	return now, nil
}

func validateNextModelTurnContinuationV1(
	current contract.TurnContinuationCurrentV1,
	request contract.NextModelTurnEligibilityRequestV1,
	now time.Time,
) error {
	if err := current.ModelTurnAllowedV1(now); err != nil {
		return err
	}
	if current.Start.AttemptRefV1() != request.ContinuationAttempt ||
		current.Digest != request.ContinuationCurrentDigest ||
		current.ActiveContext != request.ActiveContext ||
		current.Start.ExecutionScopeDigest != request.ContinuationAttempt.ExecutionScopeDigest ||
		current.Start.RunID != request.Run.RunID ||
		current.Start.Source.Session != request.Session ||
		current.Start.TargetTurn != request.TargetTurn {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "next Model Turn continuation current differs from the exact request")
	}
	return nil
}

var _ applicationports.NextModelTurnEligibilityPortV1 = (*NextModelTurnEligibilityV1)(nil)

package application

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// ContextTurnRefreshCoordinatorPortV1 is the narrow Application-local seam
// needed by TurnContinuationCoordinatorV1. The concrete Context coordinator
// still owns Prepare/Apply/Inspect recovery and all Context facts.
type ContextTurnRefreshCoordinatorPortV1 interface {
	CoordinateContextTurnRefreshV1(context.Context, ContextTurnRefreshCoordinationRequestV1) (contract.ContextTurnRefreshResultV1, error)
}

type TurnContinuationCoordinatorConfigV1 struct {
	Continuation applicationports.TurnContinuationPortV1
	Context      ContextTurnRefreshCoordinatorPortV1
	Clock        func() time.Time
}

type TurnContinuationCoordinatorV1 struct {
	config TurnContinuationCoordinatorConfigV1
	gates  singleCallCoordinatorGateV2
}

// TurnContinuationCoordinationRequestV1 binds the already-sealed Harness
// continuation Start to the Context coordination request that must publish the
// exact ExpectedContextRefreshAttempt. It contains no Model, Tool or Provider
// execution authority.
type TurnContinuationCoordinationRequestV1 struct {
	Start          contract.TurnContinuationStartRequestV1
	ContextRefresh ContextTurnRefreshCoordinationRequestV1
}

func NewTurnContinuationCoordinatorV1(config TurnContinuationCoordinatorConfigV1) (*TurnContinuationCoordinatorV1, error) {
	if nilInterfaceV1(config.Continuation) || nilInterfaceV1(config.Context) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "turn continuation coordinator requires Harness continuation and Context refresh")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	return &TurnContinuationCoordinatorV1{
		config: config,
		gates:  singleCallCoordinatorGateV2{entries: make(map[string]*singleCallCoordinatorGateEntryV2)},
	}, nil
}

// CoordinateTurnContinuationV1 performs only the cross-owner ordering:
// Harness Begin(pending) -> Context refresh -> Harness Commit(CAS) -> gate the
// next Model turn. Unknown Harness mutation outcomes are recovered exclusively
// by Inspect of the original Attempt; no mutation is redispatched here.
func (c *TurnContinuationCoordinatorV1) CoordinateTurnContinuationV1(ctx context.Context, request TurnContinuationCoordinationRequestV1) (contract.TurnContinuationCurrentV1, error) {
	if c == nil || ctx == nil {
		return contract.TurnContinuationCurrentV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "turn continuation coordinator is nil")
	}
	start := request.Start
	release := c.gates.acquire(start.AttemptID + "\x00" + string(start.ExecutionScopeDigest))
	defer release()

	now := c.config.Clock()
	if err := validateTurnContinuationCoordinationRequestV1(request, now); err != nil {
		return contract.TurnContinuationCurrentV1{}, err
	}

	current, err := c.config.Continuation.BeginTurnContinuationV1(ctx, start)
	if err != nil {
		if !core.HasCategory(err, core.ErrorIndeterminate) {
			return contract.TurnContinuationCurrentV1{}, err
		}
		current, err = c.inspectOriginalTurnContinuationV1(context.WithoutCancel(ctx), start)
		if err != nil {
			return contract.TurnContinuationCurrentV1{}, err
		}
	}
	now, err = c.validateContinuationCurrentV1(current, start, now)
	if err != nil {
		return contract.TurnContinuationCurrentV1{}, err
	}
	if current.State == contract.TurnContinuationContextCurrentV1 {
		if err := current.ModelTurnAllowedV1(now); err != nil {
			return contract.TurnContinuationCurrentV1{}, err
		}
		return current.Clone(), nil
	}
	if current.State != contract.TurnContinuationPendingV1 {
		return contract.TurnContinuationCurrentV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidState, "TurnContinuation Begin did not publish continuation_pending")
	}

	refresh, err := c.config.Context.CoordinateContextTurnRefreshV1(ctx, request.ContextRefresh)
	if err != nil {
		return contract.TurnContinuationCurrentV1{}, err
	}
	now = c.config.Clock()
	if now.IsZero() || now.UnixNano() < current.CheckedUnixNano {
		return contract.TurnContinuationCurrentV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "turn continuation clock regressed after Context refresh")
	}
	if refresh.Validate() != nil || refresh.State != contract.ContextTurnRefreshAppliedStateV1 || refresh.AttemptRef != start.ExpectedContextRefreshAttempt {
		return contract.TurnContinuationCurrentV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Context refresh did not return the exact expected Attempt")
	}

	deadline := start.RequestedNotAfterUnixNano
	if current.ExpiresUnixNano < deadline {
		deadline = current.ExpiresUnixNano
	}
	if refresh.ExpiresUnixNano < deadline {
		deadline = refresh.ExpiresUnixNano
	}
	commit, err := contract.SealTurnContinuationCommitRequestV1(contract.TurnContinuationCommitRequestV1{
		Pending:                   current,
		AppliedContextRefresh:     refresh,
		RequestedNotAfterUnixNano: deadline,
	}, now)
	if err != nil {
		return contract.TurnContinuationCurrentV1{}, err
	}
	committed, commitErr := c.config.Continuation.CommitTurnContinuationV1(ctx, commit)
	if commitErr != nil {
		if !core.HasCategory(commitErr, core.ErrorIndeterminate) {
			return contract.TurnContinuationCurrentV1{}, commitErr
		}
		committed, err = c.inspectOriginalTurnContinuationV1(context.WithoutCancel(ctx), start)
		if err != nil {
			return contract.TurnContinuationCurrentV1{}, err
		}
	}
	now, err = c.validateContinuationCurrentV1(committed, start, now)
	if err != nil {
		return contract.TurnContinuationCurrentV1{}, err
	}
	if committed.State != contract.TurnContinuationContextCurrentV1 || committed.AppliedContextRefresh == nil || committed.AppliedContextRefresh.Digest != refresh.Digest || committed.CommitRequestDigest != commit.Digest {
		return contract.TurnContinuationCurrentV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Harness Commit did not publish the exact Context successor")
	}
	if err := committed.ModelTurnAllowedV1(now); err != nil {
		return contract.TurnContinuationCurrentV1{}, err
	}
	return committed.Clone(), nil
}

func validateTurnContinuationCoordinationRequestV1(request TurnContinuationCoordinationRequestV1, now time.Time) error {
	start := request.Start
	refresh := request.ContextRefresh
	if err := start.ValidateCurrent(now); err != nil {
		return err
	}
	// V1 can prove the exact refresh payload before Begin only for Tool-only
	// requests. Memory/Knowledge projections are read and sealed inside the
	// Context coordinator, so accepting them here would let a final replay return
	// before proving that this request is the one bound by ExpectedAttempt.
	if refresh.Memory != nil || refresh.Knowledge != nil {
		return core.NewError(core.ErrorCapabilityUnavailable, core.ReasonUnknownCapability, "turn continuation V1 supports only a pre-sealable Tool-only Context refresh")
	}
	if refresh.ID != start.AttemptID ||
		refresh.ExecutionScopeDigest != start.ExecutionScopeDigest ||
		refresh.RunID != start.RunID ||
		refresh.SourceSession != start.Source.Session ||
		refresh.SessionApplicability != start.Source.SessionApplicability ||
		refresh.SourceTurn != start.Source.Turn ||
		refresh.TurnApplicability != start.Source.TurnApplicability ||
		refresh.SourceTurn.Ordinal == ^uint32(0) ||
		refresh.SourceTurn.Ordinal+1 != start.TargetTurn ||
		refresh.RequestedNotAfterNano <= 0 ||
		refresh.RequestedNotAfterNano > start.RequestedNotAfterUnixNano ||
		len(refresh.OpaqueContextRequest) == 0 {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "turn continuation and Context refresh coordinates differ")
	}
	if start.ExpectedContextRefreshAttempt.ID != refresh.ID || start.ExpectedContextRefreshAttempt.Kind != contract.ContextTurnRefreshApplicationAttemptKindV1 || start.ExpectedContextRefreshAttempt.Revision != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "turn continuation expected another Context refresh Attempt")
	}
	prepare, err := contract.SealContextTurnRefreshPrepareRequestV1(contract.ContextTurnRefreshPrepareRequestV1{
		ID: refresh.ID, ExecutionScopeDigest: refresh.ExecutionScopeDigest, RunID: refresh.RunID,
		SourceSession: refresh.SourceSession, SessionApplicability: refresh.SessionApplicability,
		SourceTurn: refresh.SourceTurn, TurnApplicability: refresh.TurnApplicability,
		ExpectedTargetTurn: refresh.SourceTurn.Ordinal + 1, OpaqueContextRequest: refresh.OpaqueContextRequest,
		RequestedNotAfterNano: refresh.RequestedNotAfterNano,
	})
	if err != nil {
		return err
	}
	attempt, err := prepare.AttemptRefV1()
	if err != nil || attempt != start.ExpectedContextRefreshAttempt {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "turn continuation Context payload differs from the expected Attempt")
	}
	return nil
}

func (c *TurnContinuationCoordinatorV1) inspectOriginalTurnContinuationV1(ctx context.Context, start contract.TurnContinuationStartRequestV1) (contract.TurnContinuationCurrentV1, error) {
	inspect, err := contract.SealTurnContinuationInspectRequestV1(contract.TurnContinuationInspectRequestV1{AttemptRef: start.AttemptRefV1()})
	if err != nil {
		return contract.TurnContinuationCurrentV1{}, err
	}
	return c.config.Continuation.InspectTurnContinuationV1(ctx, inspect)
}

func (c *TurnContinuationCoordinatorV1) validateContinuationCurrentV1(current contract.TurnContinuationCurrentV1, start contract.TurnContinuationStartRequestV1, floor time.Time) (time.Time, error) {
	now := c.config.Clock()
	if now.IsZero() || now.Before(floor) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "turn continuation clock regressed")
	}
	if current.ValidateCurrent(now) != nil || current.Start.Digest != start.Digest || current.Start.AttemptRefV1() != start.AttemptRefV1() {
		return time.Time{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Harness returned another or invalid TurnContinuation current")
	}
	return now, nil
}

package application

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type SingleCallToolActionCoordinatorConfigV1 struct {
	Facts       applicationports.SingleCallToolActionCoordinationFactPortV1
	Tool        applicationports.SingleCallToolActionPortV1
	Inputs      applicationports.SingleCallToolActionInputCurrentReaderV1
	Settlements applicationports.SingleCallOperationSettlementCurrentReaderV1
	Clock       func() time.Time
}

type SingleCallToolActionCoordinatorV1 struct {
	config SingleCallToolActionCoordinatorConfigV1
	gates  singleCallCoordinatorGateV1
}

func NewSingleCallToolActionCoordinatorV1(config SingleCallToolActionCoordinatorConfigV1) (*SingleCallToolActionCoordinatorV1, error) {
	if config.Facts == nil || config.Tool == nil || config.Inputs == nil || config.Settlements == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "single-call coordinator requires Fact, Tool, input-current and settlement-current ports")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	return &SingleCallToolActionCoordinatorV1{config: config, gates: singleCallCoordinatorGateV1{entries: make(map[string]*singleCallCoordinatorGateEntryV1)}}, nil
}

// CoordinateSingleCallToolActionV1 closes only G6A. It has no Context refresh,
// Harness continuation, capability activation, Turn advancement or Provider
// Boundary proof dependency.
func (c *SingleCallToolActionCoordinatorV1) CoordinateSingleCallToolActionV1(ctx context.Context, request contract.SingleCallToolActionRequestV1) (contract.SingleCallToolActionResultV1, error) {
	now, err := c.nowAfter(time.Time{})
	if err != nil {
		return contract.SingleCallToolActionResultV1{}, err
	}
	if err := request.ValidateCurrent(now); err != nil {
		return contract.SingleCallToolActionResultV1{}, err
	}
	release := c.gates.acquire(request.ID + "\x00" + string(request.Digest) + "\x00" + string(request.ExecutionScopeDigest))
	defer release()

	_, now, err = c.inspectInputs(ctx, request, now)
	if err != nil {
		return contract.SingleCallToolActionResultV1{}, err
	}

	fact, err := contract.NewSingleCallToolActionCoordinationFactV1(request, now)
	if err != nil {
		return contract.SingleCallToolActionResultV1{}, err
	}
	current, createErr := c.config.Facts.CreateSingleCallToolActionCoordinationV1(ctx, fact)
	if createErr != nil {
		current, err = c.inspectExactCoordination(context.WithoutCancel(ctx), request)
		if err != nil {
			return contract.SingleCallToolActionResultV1{}, createErr
		}
	}
	if err := validateSingleCallCoordinationForRequestV1(current, request); err != nil {
		return contract.SingleCallToolActionResultV1{}, err
	}

	if current.State == contract.SingleCallToolActionPreparedV1 {
		now, err = c.nowAfter(now)
		if err != nil {
			return contract.SingleCallToolActionResultV1{}, err
		}
		if err = request.ValidateCurrent(now); err != nil {
			return contract.SingleCallToolActionResultV1{}, err
		}
		current, err = c.advanceCoordination(ctx, current, contract.SingleCallToolActionDispatchIntentV1, nil, now)
		if err != nil {
			return contract.SingleCallToolActionResultV1{}, err
		}
	}

	inspectKey := applicationports.InspectSingleCallToolActionRequestV1{RequestID: request.ID, RequestDigest: request.Digest, ScopeDigest: request.ExecutionScopeDigest}
	if err := inspectKey.Validate(); err != nil {
		return contract.SingleCallToolActionResultV1{}, err
	}
	switch current.State {
	case contract.SingleCallToolActionWaitingInspectV1, contract.SingleCallToolActionCompletedV1:
		return c.inspectOnly(ctx, request, current, inspectKey, now)
	case contract.SingleCallToolActionDispatchIntentV1:
		// A generic Tool NotFound is not a proof that dispatch never started. The
		// only start authority is a successful, observable CAS which consumes
		// dispatch_intent into waiting_inspect before the Tool call.
		_, now, err = c.inspectInputs(ctx, request, now)
		if err != nil {
			return contract.SingleCallToolActionResultV1{}, err
		}
		current, claimed, claimErr := c.claimStartOrInspect(ctx, current, now)
		if claimErr != nil {
			return contract.SingleCallToolActionResultV1{}, claimErr
		}
		if !claimed {
			return c.inspectOnly(ctx, request, current, inspectKey, now)
		}
		now, err = c.nowAfter(now)
		if err != nil {
			return contract.SingleCallToolActionResultV1{}, err
		}
		if err = request.ValidateCurrent(now); err != nil {
			return contract.SingleCallToolActionResultV1{}, err
		}
		result, executeErr := c.config.Tool.ExecuteSingleCallToolActionV1(ctx, request)
		afterExecute, clockErr := c.nowAfter(now)
		if clockErr != nil {
			return contract.SingleCallToolActionResultV1{}, clockErr
		}
		if executeErr == nil {
			return c.finalize(ctx, request, current, result, afterExecute)
		}
		recovered, recoveredErr := c.config.Tool.InspectSingleCallToolActionV1(context.WithoutCancel(ctx), inspectKey)
		afterInspect, inspectClockErr := c.nowAfter(afterExecute)
		if inspectClockErr != nil {
			return contract.SingleCallToolActionResultV1{}, inspectClockErr
		}
		if recoveredErr == nil {
			return c.finalize(context.WithoutCancel(ctx), request, current, recovered, afterInspect)
		}
		if !core.HasCategory(recoveredErr, core.ErrorNotFound) {
			return contract.SingleCallToolActionResultV1{}, recoveredErr
		}
		return contract.SingleCallToolActionResultV1{}, executeErr
	default:
		return contract.SingleCallToolActionResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "single-call coordination state cannot invoke Tool")
	}
}

func (c *SingleCallToolActionCoordinatorV1) inspectInputs(ctx context.Context, request contract.SingleCallToolActionRequestV1, previous time.Time) (contract.SingleCallToolActionInputCurrentProjectionV1, time.Time, error) {
	projection, err := c.config.Inputs.InspectSingleCallToolActionInputCurrentV1(ctx, request)
	if err != nil {
		return contract.SingleCallToolActionInputCurrentProjectionV1{}, time.Time{}, err
	}
	// First reject a Reader which claims it checked after the time supplied to
	// this boundary. Then take a fresh clock sample to catch TTL crossing while
	// the Reader was running.
	if err := projection.ValidateFor(request, previous); err != nil {
		return contract.SingleCallToolActionInputCurrentProjectionV1{}, time.Time{}, err
	}
	after, err := c.nowAfter(previous)
	if err != nil {
		return contract.SingleCallToolActionInputCurrentProjectionV1{}, time.Time{}, err
	}
	if err := projection.ValidateFor(request, after); err != nil {
		return contract.SingleCallToolActionInputCurrentProjectionV1{}, time.Time{}, err
	}
	return projection, after, nil
}

func (c *SingleCallToolActionCoordinatorV1) inspectOnly(ctx context.Context, request contract.SingleCallToolActionRequestV1, current contract.SingleCallToolActionCoordinationFactV1, key applicationports.InspectSingleCallToolActionRequestV1, previous time.Time) (contract.SingleCallToolActionResultV1, error) {
	before, err := c.nowAfter(previous)
	if err != nil {
		return contract.SingleCallToolActionResultV1{}, err
	}
	if err := request.ValidateCurrent(before); err != nil {
		return contract.SingleCallToolActionResultV1{}, err
	}
	result, inspectErr := c.config.Tool.InspectSingleCallToolActionV1(ctx, key)
	after, clockErr := c.nowAfter(before)
	if clockErr != nil {
		return contract.SingleCallToolActionResultV1{}, clockErr
	}
	if inspectErr != nil {
		return contract.SingleCallToolActionResultV1{}, inspectErr
	}
	return c.finalize(ctx, request, current, result, after)
}

// claimStartOrInspect consumes the only start authority. Only a successful CAS
// response grants this caller the right to invoke the Tool port. Lost replies,
// conflicts and recovered waiting_inspect facts are permanently inspect-only.
func (c *SingleCallToolActionCoordinatorV1) claimStartOrInspect(ctx context.Context, current contract.SingleCallToolActionCoordinationFactV1, now time.Time) (contract.SingleCallToolActionCoordinationFactV1, bool, error) {
	if current.State != contract.SingleCallToolActionDispatchIntentV1 {
		return current, false, nil
	}
	claimID, err := newSingleCallStartClaimIDV1()
	if err != nil {
		return contract.SingleCallToolActionCoordinationFactV1{}, false, err
	}
	next, err := contract.ClaimSingleCallToolActionStartV1(current, claimID, now)
	if err != nil {
		return contract.SingleCallToolActionCoordinationFactV1{}, false, err
	}
	stored, casErr := c.config.Facts.CompareAndSwapSingleCallToolActionCoordinationV1(ctx, applicationports.SingleCallToolActionCoordinationCASRequestV1{Scope: current.Request.ExecutionScope, ID: current.ID, ExpectedRevision: current.Revision, Next: next})
	if casErr == nil {
		if err := validateSingleCallCoordinationForRequestV1(stored, current.Request); err != nil {
			return contract.SingleCallToolActionCoordinationFactV1{}, false, err
		}
		if stored.State != contract.SingleCallToolActionWaitingInspectV1 || stored.StartClaimID != claimID || stored.Digest != next.Digest {
			return contract.SingleCallToolActionCoordinationFactV1{}, false, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "single-call start claim returned another successor")
		}
		return stored, true, nil
	}
	inspected, inspectErr := c.config.Facts.InspectSingleCallToolActionCoordinationV1(context.WithoutCancel(ctx), current.Request.ExecutionScope, current.ID)
	if inspectErr != nil {
		return contract.SingleCallToolActionCoordinationFactV1{}, false, casErr
	}
	if err := validateSingleCallCoordinationForRequestV1(inspected, current.Request); err != nil {
		return contract.SingleCallToolActionCoordinationFactV1{}, false, err
	}
	if inspected.State == contract.SingleCallToolActionWaitingInspectV1 || inspected.State == contract.SingleCallToolActionCompletedV1 {
		return inspected, false, nil
	}
	return contract.SingleCallToolActionCoordinationFactV1{}, false, casErr
}

func (c *SingleCallToolActionCoordinatorV1) inspectExactCoordination(ctx context.Context, request contract.SingleCallToolActionRequestV1) (contract.SingleCallToolActionCoordinationFactV1, error) {
	fact, err := c.config.Facts.InspectSingleCallToolActionCoordinationV1(ctx, request.ExecutionScope, request.ID)
	if err != nil {
		return contract.SingleCallToolActionCoordinationFactV1{}, err
	}
	if err := validateSingleCallCoordinationForRequestV1(fact, request); err != nil {
		return contract.SingleCallToolActionCoordinationFactV1{}, err
	}
	return fact, nil
}

func (c *SingleCallToolActionCoordinatorV1) advanceCoordination(ctx context.Context, current contract.SingleCallToolActionCoordinationFactV1, state contract.SingleCallToolActionCoordinationStateV1, result *contract.SingleCallToolActionResultRefV1, now time.Time) (contract.SingleCallToolActionCoordinationFactV1, error) {
	next, err := contract.NextSingleCallToolActionCoordinationFactV1(current, state, result, now)
	if err != nil {
		return contract.SingleCallToolActionCoordinationFactV1{}, err
	}
	stored, casErr := c.config.Facts.CompareAndSwapSingleCallToolActionCoordinationV1(ctx, applicationports.SingleCallToolActionCoordinationCASRequestV1{Scope: current.Request.ExecutionScope, ID: current.ID, ExpectedRevision: current.Revision, Next: next})
	if casErr == nil {
		if err := stored.Validate(); err != nil {
			return contract.SingleCallToolActionCoordinationFactV1{}, err
		}
		if stored.Digest != next.Digest {
			return contract.SingleCallToolActionCoordinationFactV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "single-call CAS returned a different successor")
		}
		return stored, nil
	}
	inspected, inspectErr := c.config.Facts.InspectSingleCallToolActionCoordinationV1(context.WithoutCancel(ctx), current.Request.ExecutionScope, current.ID)
	if inspectErr != nil {
		return contract.SingleCallToolActionCoordinationFactV1{}, casErr
	}
	if err := validateSingleCallCoordinationForRequestV1(inspected, current.Request); err != nil {
		return contract.SingleCallToolActionCoordinationFactV1{}, err
	}
	if inspected.State == state {
		if state != contract.SingleCallToolActionCompletedV1 || result != nil && inspected.Result != nil && contract.SameSingleCallToolActionResultRefV1(*result, *inspected.Result) {
			return inspected, nil
		}
	}
	if state == contract.SingleCallToolActionDispatchIntentV1 && (inspected.State == contract.SingleCallToolActionWaitingInspectV1 || inspected.State == contract.SingleCallToolActionCompletedV1) {
		return inspected, nil
	}
	if state == contract.SingleCallToolActionWaitingInspectV1 && inspected.State == contract.SingleCallToolActionCompletedV1 {
		return inspected, nil
	}
	return contract.SingleCallToolActionCoordinationFactV1{}, casErr
}

func (c *SingleCallToolActionCoordinatorV1) finalize(ctx context.Context, request contract.SingleCallToolActionRequestV1, currentFact contract.SingleCallToolActionCoordinationFactV1, result contract.SingleCallToolActionResultV1, previous time.Time) (contract.SingleCallToolActionResultV1, error) {
	now, err := c.nowAfter(previous)
	if err != nil {
		return c.failFinalize(ctx, currentFact, err)
	}
	if err := result.ValidateCurrentFor(request, now); err != nil {
		return c.failFinalize(ctx, currentFact, err)
	}
	_, now, err = c.inspectInputs(ctx, request, now)
	if err != nil {
		return c.failFinalize(ctx, currentFact, err)
	}
	operation := result.Inspection.DomainResult.Operation
	current, err := c.config.Settlements.InspectCurrentOperationSettlementV4(ctx, runtimeports.InspectCurrentOperationSettlementRequestV4{Operation: operation, EffectID: result.Inspection.Settlement.EffectID})
	if err != nil {
		return c.failFinalize(ctx, currentFact, err)
	}
	now, err = c.nowAfter(now)
	if err != nil {
		return c.failFinalize(ctx, currentFact, err)
	}
	if err := request.ValidateCurrent(now); err != nil {
		return c.failFinalize(ctx, currentFact, err)
	}
	if err := result.ValidateCurrentFor(request, now); err != nil {
		return c.failFinalize(ctx, currentFact, err)
	}
	if current.CheckedUnixNano > now.UnixNano() {
		return c.failFinalize(ctx, currentFact, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "single-call settlement inspection is future-dated"))
	}
	if err := current.Validate(now); err != nil {
		return c.failFinalize(ctx, currentFact, err)
	}
	if current.Digest != result.Inspection.Digest || !runtimeports.SameOperationSettlementRefV4(current.Settlement, result.ToolResult.Settlement) || !runtimeports.SameOperationSettlementEvidenceAssociationRefV4(current.Association, result.Association) {
		return c.failFinalize(ctx, currentFact, core.NewError(core.ErrorConflict, core.ReasonEffectSettlementMissing, "single-call current V4 inspection drifted"))
	}
	association, err := c.config.Settlements.InspectOperationSettlementEvidenceAssociationV4(ctx, operation, current.Association)
	if err != nil {
		return c.failFinalize(ctx, currentFact, err)
	}
	now, err = c.nowAfter(now)
	if err != nil {
		return c.failFinalize(ctx, currentFact, err)
	}
	if err := request.ValidateCurrent(now); err != nil {
		return c.failFinalize(ctx, currentFact, err)
	}
	if err := result.ValidateCurrentFor(request, now); err != nil {
		return c.failFinalize(ctx, currentFact, err)
	}
	if current.CheckedUnixNano > now.UnixNano() {
		return c.failFinalize(ctx, currentFact, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "single-call settlement inspection is future-dated"))
	}
	if err := current.Validate(now); err != nil {
		return c.failFinalize(ctx, currentFact, err)
	}
	if err := association.Validate(); err != nil || !runtimeports.SameOperationSettlementEvidenceAssociationRefV4(association.RefV4(), current.Association) || !runtimeports.SameOperationSettlementRefV4(association.Settlement, current.Settlement) {
		return c.failFinalize(ctx, currentFact, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "single-call public V4 Association is not exact"))
	}

	ref := result.RefV1()
	if currentFact.State == contract.SingleCallToolActionCompletedV1 {
		if currentFact.Result == nil || !contract.SameSingleCallToolActionResultRefV1(*currentFact.Result, ref) {
			return contract.SingleCallToolActionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "single-call coordination already completed with another Result")
		}
		return c.validateReturn(request, result, current, now)
	}
	completed, err := c.advanceCoordination(ctx, currentFact, contract.SingleCallToolActionCompletedV1, &ref, now)
	if err != nil {
		return contract.SingleCallToolActionResultV1{}, err
	}
	if completed.Result == nil || !contract.SameSingleCallToolActionResultRefV1(*completed.Result, ref) {
		return contract.SingleCallToolActionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "single-call completion recovered another Result")
	}
	return c.validateReturn(request, result, current, now)
}

func (c *SingleCallToolActionCoordinatorV1) failFinalize(_ context.Context, current contract.SingleCallToolActionCoordinationFactV1, cause error) (contract.SingleCallToolActionResultV1, error) {
	if current.State == contract.SingleCallToolActionWaitingInspectV1 || current.State == contract.SingleCallToolActionCompletedV1 {
		return contract.SingleCallToolActionResultV1{}, cause
	}
	return contract.SingleCallToolActionResultV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonInvalidState, "single-call finalize was reached without a persisted waiting_inspect start claim")
}

func (c *SingleCallToolActionCoordinatorV1) validateReturn(request contract.SingleCallToolActionRequestV1, result contract.SingleCallToolActionResultV1, current runtimeports.OperationInspectionSettlementRefV4, previous time.Time) (contract.SingleCallToolActionResultV1, error) {
	now, err := c.nowAfter(previous)
	if err != nil {
		return contract.SingleCallToolActionResultV1{}, err
	}
	if err := request.ValidateCurrent(now); err != nil {
		return contract.SingleCallToolActionResultV1{}, err
	}
	if err := result.ValidateCurrentFor(request, now); err != nil {
		return contract.SingleCallToolActionResultV1{}, err
	}
	if current.CheckedUnixNano > now.UnixNano() {
		return contract.SingleCallToolActionResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "single-call settlement inspection is future-dated")
	}
	if err := current.Validate(now); err != nil {
		return contract.SingleCallToolActionResultV1{}, err
	}
	return result, nil
}

func (c *SingleCallToolActionCoordinatorV1) nowAfter(previous time.Time) (time.Time, error) {
	if c == nil || c.config.Clock == nil {
		return time.Time{}, core.NewError(core.ErrorUnavailable, core.ReasonClockRegression, "single-call coordinator clock is unavailable")
	}
	now := c.config.Clock()
	if now.IsZero() || !previous.IsZero() && now.Before(previous) {
		return time.Time{}, core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "single-call coordinator clock is indeterminate or regressed")
	}
	return now, nil
}

func validateSingleCallCoordinationForRequestV1(fact contract.SingleCallToolActionCoordinationFactV1, request contract.SingleCallToolActionRequestV1) error {
	if err := fact.Validate(); err != nil {
		return err
	}
	if fact.ID != request.ID || fact.Request.Digest != request.Digest || !runtimeports.SameExecutionScopeV2(fact.Request.ExecutionScope, request.ExecutionScope) {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "single-call coordination belongs to another canonical request")
	}
	return nil
}

func newSingleCallStartClaimIDV1() (string, error) {
	var nonce [16]byte
	if _, err := cryptorand.Read(nonce[:]); err != nil {
		return "", core.NewError(core.ErrorUnavailable, core.ReasonOwnerConflict, "single-call start claim entropy is unavailable")
	}
	return "start-claim/" + hex.EncodeToString(nonce[:]), nil
}

type singleCallCoordinatorGateEntryV1 struct {
	mu   sync.Mutex
	refs int
}

type singleCallCoordinatorGateV1 struct {
	mu      sync.Mutex
	entries map[string]*singleCallCoordinatorGateEntryV1
}

func (g *singleCallCoordinatorGateV1) acquire(key string) func() {
	g.mu.Lock()
	if g.entries == nil {
		g.entries = make(map[string]*singleCallCoordinatorGateEntryV1)
	}
	entry := g.entries[key]
	if entry == nil {
		entry = &singleCallCoordinatorGateEntryV1{}
		g.entries[key] = entry
	}
	entry.refs++
	g.mu.Unlock()
	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		g.mu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(g.entries, key)
		}
		g.mu.Unlock()
	}
}

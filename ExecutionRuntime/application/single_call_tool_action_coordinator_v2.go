package application

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type SingleCallToolActionCoordinatorConfigV2 struct {
	Facts       applicationports.SingleCallToolActionCoordinationFactPortV2
	Tool        applicationports.SingleCallToolActionPortV2
	Inputs      applicationports.SingleCallToolActionInputCurrentReaderV2
	Settlements applicationports.SingleCallOperationSettlementCurrentReaderV2
	Clock       func() time.Time
}

type SingleCallToolActionCoordinatorV2 struct {
	config SingleCallToolActionCoordinatorConfigV2
	gates  singleCallCoordinatorGateV2
}

func isNilSingleCallPortV2(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	}
	return false
}

func NewSingleCallToolActionCoordinatorV2(config SingleCallToolActionCoordinatorConfigV2) (*SingleCallToolActionCoordinatorV2, error) {
	if isNilSingleCallPortV2(config.Facts) || isNilSingleCallPortV2(config.Tool) || isNilSingleCallPortV2(config.Inputs) || isNilSingleCallPortV2(config.Settlements) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "single-call V2 coordinator requires Fact, Tool, input-current and settlement-current ports")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	return &SingleCallToolActionCoordinatorV2{config: config, gates: singleCallCoordinatorGateV2{entries: make(map[string]*singleCallCoordinatorGateEntryV2)}}, nil
}

// CoordinateSingleCallToolActionV2 closes only the Application owner-local P2
// coordination slice. It does not implement Tool binding P4, a system fixture,
// Context refresh, continuation, Turn advancement, capability enablement or a
// production composition root.
func (c *SingleCallToolActionCoordinatorV2) CoordinateSingleCallToolActionV2(ctx context.Context, request contract.SingleCallToolActionRequestV2) (contract.SingleCallToolActionResultV2, error) {
	if c == nil {
		return contract.SingleCallToolActionResultV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "single-call V2 coordinator is nil")
	}
	if isNilSingleCallPortV2(ctx) {
		return contract.SingleCallToolActionResultV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call V2 coordinator context is nil")
	}
	now, err := c.nowAfterV2(time.Time{})
	if err != nil {
		return contract.SingleCallToolActionResultV2{}, err
	}
	if err = request.ValidateCurrent(now); err != nil {
		return contract.SingleCallToolActionResultV2{}, err
	}
	release := c.gates.acquire(request.ID + "\x00" + string(request.Digest) + "\x00" + string(request.Action.ExecutionScopeDigest))
	defer release()

	_, now, err = c.inspectInputsV2(ctx, request, now)
	if err != nil {
		return contract.SingleCallToolActionResultV2{}, err
	}
	initial, err := contract.NewSingleCallToolActionCoordinationFactV2(request)
	if err != nil {
		return contract.SingleCallToolActionResultV2{}, err
	}
	current, createErr := c.config.Facts.CreateSingleCallToolActionCoordinationV2(ctx, initial)
	if createErr != nil {
		current, err = c.inspectExactCoordinationV2(context.WithoutCancel(ctx), request)
		if err != nil {
			return contract.SingleCallToolActionResultV2{}, createErr
		}
	}
	if err = validateSingleCallCoordinationForRequestV2(current, request); err != nil {
		return contract.SingleCallToolActionResultV2{}, err
	}

	dispatchOwnedThisCall := false
	if current.State == contract.SingleCallToolActionPreparedV2 {
		now, err = c.nowAfterV2(now)
		if err != nil {
			return contract.SingleCallToolActionResultV2{}, err
		}
		if err = request.ValidateCurrent(now); err != nil {
			return contract.SingleCallToolActionResultV2{}, err
		}
		current, dispatchOwnedThisCall, err = c.advanceCoordinationV2(ctx, current, contract.SingleCallToolActionDispatchIntentV2, nil, now)
		if err != nil {
			return contract.SingleCallToolActionResultV2{}, err
		}
	}
	inspectKey, err := contract.SealSingleCallToolActionInspectKeyV2(request)
	if err != nil {
		return contract.SingleCallToolActionResultV2{}, err
	}
	switch current.State {
	case contract.SingleCallToolActionWaitingInspectV2, contract.SingleCallToolActionCompletedV2:
		return c.inspectOnlyV2(ctx, request, current, inspectKey, now)
	case contract.SingleCallToolActionDispatchIntentV2:
		if !dispatchOwnedThisCall {
			return c.inspectOnlyV2(ctx, request, current, inspectKey, now)
		}
		_, now, err = c.inspectInputsV2(ctx, request, now)
		if err != nil {
			return contract.SingleCallToolActionResultV2{}, err
		}
		current, claimed, claimErr := c.claimStartOrInspectV2(ctx, current, now)
		if claimErr != nil {
			return contract.SingleCallToolActionResultV2{}, claimErr
		}
		if !claimed {
			return c.inspectOnlyV2(ctx, request, current, inspectKey, now)
		}
		now, err = c.nowAfterV2(now)
		if err != nil {
			return contract.SingleCallToolActionResultV2{}, err
		}
		if err = request.ValidateCurrent(now); err != nil {
			return contract.SingleCallToolActionResultV2{}, err
		}
		result, executeErr := c.config.Tool.ExecuteSingleCallToolActionV2(ctx, request)
		afterExecute, clockErr := c.nowAfterV2(now)
		if clockErr != nil {
			return contract.SingleCallToolActionResultV2{}, clockErr
		}
		if executeErr == nil {
			return c.finalizeV2(ctx, request, current, result, afterExecute)
		}
		recovered, inspectErr := c.config.Tool.InspectSingleCallToolActionV2(context.WithoutCancel(ctx), inspectKey)
		afterInspect, clockErr := c.nowAfterV2(afterExecute)
		if clockErr != nil {
			return contract.SingleCallToolActionResultV2{}, clockErr
		}
		if inspectErr == nil {
			return c.finalizeV2(context.WithoutCancel(ctx), request, current, recovered, afterInspect)
		}
		if core.HasCategory(inspectErr, core.ErrorNotFound) {
			return contract.SingleCallToolActionResultV2{}, executeErr
		}
		return contract.SingleCallToolActionResultV2{}, inspectErr
	default:
		return contract.SingleCallToolActionResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "single-call V2 coordination state cannot invoke Tool")
	}
}

func (c *SingleCallToolActionCoordinatorV2) inspectInputsV2(ctx context.Context, request contract.SingleCallToolActionRequestV2, previous time.Time) (contract.SingleCallToolActionInputCurrentProjectionV2, time.Time, error) {
	projection, err := c.config.Inputs.InspectSingleCallToolActionInputCurrentV2(ctx, request)
	if err != nil {
		return contract.SingleCallToolActionInputCurrentProjectionV2{}, time.Time{}, err
	}
	if err = projection.ValidateFor(request, previous); err != nil {
		return contract.SingleCallToolActionInputCurrentProjectionV2{}, time.Time{}, err
	}
	after, err := c.nowAfterV2(previous)
	if err != nil {
		return contract.SingleCallToolActionInputCurrentProjectionV2{}, time.Time{}, err
	}
	if err = projection.ValidateFor(request, after); err != nil {
		return contract.SingleCallToolActionInputCurrentProjectionV2{}, time.Time{}, err
	}
	return projection, after, nil
}

func (c *SingleCallToolActionCoordinatorV2) inspectOnlyV2(ctx context.Context, request contract.SingleCallToolActionRequestV2, current contract.SingleCallToolActionCoordinationFactV2, key contract.SingleCallToolActionInspectKeyV2, previous time.Time) (contract.SingleCallToolActionResultV2, error) {
	before, err := c.nowAfterV2(previous)
	if err != nil {
		return contract.SingleCallToolActionResultV2{}, err
	}
	if err = request.ValidateCurrent(before); err != nil {
		return contract.SingleCallToolActionResultV2{}, err
	}
	result, err := c.config.Tool.InspectSingleCallToolActionV2(ctx, key)
	after, clockErr := c.nowAfterV2(before)
	if clockErr != nil {
		return contract.SingleCallToolActionResultV2{}, clockErr
	}
	if err != nil {
		return contract.SingleCallToolActionResultV2{}, err
	}
	return c.finalizeV2(ctx, request, current, result, after)
}

func (c *SingleCallToolActionCoordinatorV2) claimStartOrInspectV2(ctx context.Context, current contract.SingleCallToolActionCoordinationFactV2, now time.Time) (contract.SingleCallToolActionCoordinationFactV2, bool, error) {
	if current.State != contract.SingleCallToolActionDispatchIntentV2 {
		return current, false, nil
	}
	claimID, err := contract.DeriveSingleCallToolActionStartClaimIDV2(current.Request)
	if err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, false, err
	}
	next, err := contract.ClaimSingleCallToolActionStartV2(current, claimID, now)
	if err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, false, err
	}
	cas, err := applicationports.SealSingleCallToolActionCoordinationCASRequestV2(applicationports.SingleCallToolActionCoordinationCASRequestV2{Scope: current.Request.Action.ExecutionScope, ID: current.ID, ExpectedRevision: current.Revision, ExpectedDigest: current.Digest, Next: next})
	if err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, false, err
	}
	stored, casErr := c.config.Facts.CompareAndSwapSingleCallToolActionCoordinationV2(ctx, cas)
	if casErr == nil {
		if err = validateSingleCallCoordinationForRequestV2(stored, current.Request); err != nil {
			return contract.SingleCallToolActionCoordinationFactV2{}, false, err
		}
		if stored.State != contract.SingleCallToolActionWaitingInspectV2 || stored.StartClaimID != claimID || stored.Digest != next.Digest {
			return contract.SingleCallToolActionCoordinationFactV2{}, false, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "single-call V2 start claim returned another successor")
		}
		return stored, true, nil
	}
	inspected, inspectErr := c.config.Facts.InspectSingleCallToolActionCoordinationV2(context.WithoutCancel(ctx), current.Request.Action.ExecutionScope, current.ID)
	if inspectErr != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, false, casErr
	}
	if err = validateSingleCallCoordinationForRequestV2(inspected, current.Request); err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, false, err
	}
	if inspected.State == contract.SingleCallToolActionWaitingInspectV2 || inspected.State == contract.SingleCallToolActionCompletedV2 {
		return inspected, false, nil
	}
	return contract.SingleCallToolActionCoordinationFactV2{}, false, casErr
}

func (c *SingleCallToolActionCoordinatorV2) advanceCoordinationV2(ctx context.Context, current contract.SingleCallToolActionCoordinationFactV2, state contract.SingleCallToolActionCoordinationStateV2, result *contract.SingleCallToolActionResultRefV2, now time.Time) (contract.SingleCallToolActionCoordinationFactV2, bool, error) {
	next, err := contract.NextSingleCallToolActionCoordinationFactV2(current, state, result, now)
	if err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, false, err
	}
	cas, err := applicationports.SealSingleCallToolActionCoordinationCASRequestV2(applicationports.SingleCallToolActionCoordinationCASRequestV2{Scope: current.Request.Action.ExecutionScope, ID: current.ID, ExpectedRevision: current.Revision, ExpectedDigest: current.Digest, Next: next})
	if err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, false, err
	}
	stored, casErr := c.config.Facts.CompareAndSwapSingleCallToolActionCoordinationV2(ctx, cas)
	if casErr == nil {
		if err = validateSingleCallCoordinationForRequestV2(stored, current.Request); err != nil {
			return contract.SingleCallToolActionCoordinationFactV2{}, false, err
		}
		if stored.Digest != next.Digest {
			return contract.SingleCallToolActionCoordinationFactV2{}, false, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "single-call V2 CAS returned another successor")
		}
		return stored, true, nil
	}
	inspected, inspectErr := c.config.Facts.InspectSingleCallToolActionCoordinationV2(context.WithoutCancel(ctx), current.Request.Action.ExecutionScope, current.ID)
	if inspectErr != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, false, casErr
	}
	if err = validateSingleCallCoordinationForRequestV2(inspected, current.Request); err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, false, err
	}
	if inspected.State == state && inspected.Digest == next.Digest {
		return inspected, false, nil
	}
	if state == contract.SingleCallToolActionDispatchIntentV2 && (inspected.State == contract.SingleCallToolActionWaitingInspectV2 || inspected.State == contract.SingleCallToolActionCompletedV2) {
		return inspected, false, nil
	}
	return contract.SingleCallToolActionCoordinationFactV2{}, false, casErr
}

func (c *SingleCallToolActionCoordinatorV2) finalizeV2(ctx context.Context, request contract.SingleCallToolActionRequestV2, currentFact contract.SingleCallToolActionCoordinationFactV2, result contract.SingleCallToolActionResultV2, previous time.Time) (contract.SingleCallToolActionResultV2, error) {
	now, err := c.nowAfterV2(previous)
	if err != nil {
		return contract.SingleCallToolActionResultV2{}, err
	}
	if err = result.ValidateCurrentFor(request, now); err != nil {
		return contract.SingleCallToolActionResultV2{}, err
	}
	_, now, err = c.inspectInputsV2(ctx, request, now)
	if err != nil {
		return contract.SingleCallToolActionResultV2{}, err
	}
	operation := result.Coordinate.Inspection.DomainResult.Operation
	current, err := c.config.Settlements.InspectCurrentOperationSettlementV4(ctx, runtimeports.InspectCurrentOperationSettlementRequestV4{Operation: operation, EffectID: result.Coordinate.Inspection.Settlement.EffectID})
	if err != nil {
		return contract.SingleCallToolActionResultV2{}, err
	}
	now, err = c.nowAfterV2(now)
	if err != nil {
		return contract.SingleCallToolActionResultV2{}, err
	}
	if err = result.ValidateCurrentFor(request, now); err != nil {
		return contract.SingleCallToolActionResultV2{}, err
	}
	if current.CheckedUnixNano > now.UnixNano() {
		return contract.SingleCallToolActionResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "single-call V2 settlement inspection is future-dated")
	}
	if err = current.Validate(now); err != nil {
		return contract.SingleCallToolActionResultV2{}, err
	}
	if current.Digest != result.Coordinate.Inspection.Digest || !runtimeports.SameOperationSettlementEvidenceAssociationRefV4(current.Association, result.Coordinate.Association) {
		return contract.SingleCallToolActionResultV2{}, core.NewError(core.ErrorConflict, core.ReasonEffectSettlementMissing, "single-call V2 current settlement drifted")
	}
	association, err := c.config.Settlements.InspectOperationSettlementEvidenceAssociationV4(ctx, operation, current.Association)
	if err != nil {
		return contract.SingleCallToolActionResultV2{}, err
	}
	now, err = c.nowAfterV2(now)
	if err != nil {
		return contract.SingleCallToolActionResultV2{}, err
	}
	if err = result.ValidateCurrentFor(request, now); err != nil {
		return contract.SingleCallToolActionResultV2{}, err
	}
	if err = association.Validate(); err != nil || !runtimeports.SameOperationSettlementEvidenceAssociationRefV4(association.RefV4(), current.Association) || !runtimeports.SameOperationSettlementRefV4(association.Settlement, current.Settlement) {
		return contract.SingleCallToolActionResultV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "single-call V2 public association is not exact")
	}
	ref := result.RefV2()
	if err = ref.Validate(); err != nil {
		return contract.SingleCallToolActionResultV2{}, err
	}
	// Owner calls above may overlap another coordinator that advances the same
	// request from dispatch_intent to waiting_inspect or completed.  The Fact
	// passed into finalize is therefore only historical evidence; re-read the
	// exact coordination current before constructing the terminal successor.
	currentFact, err = c.inspectExactCoordinationV2(context.WithoutCancel(ctx), request)
	if err != nil {
		return contract.SingleCallToolActionResultV2{}, err
	}
	now, err = c.nowAfterV2(now)
	if err != nil {
		return contract.SingleCallToolActionResultV2{}, err
	}
	if err = result.ValidateCurrentFor(request, now); err != nil {
		return contract.SingleCallToolActionResultV2{}, err
	}
	if currentFact.State == contract.SingleCallToolActionCompletedV2 {
		if currentFact.Result == nil || *currentFact.Result != ref {
			return contract.SingleCallToolActionResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "single-call V2 already completed with another result")
		}
		return result, nil
	}
	completed, err := c.completeCoordinationV2(ctx, currentFact, result, now)
	if err != nil {
		return contract.SingleCallToolActionResultV2{}, err
	}
	if completed.Result == nil || *completed.Result != ref {
		return contract.SingleCallToolActionResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "single-call V2 completion recovered another result")
	}
	return result, nil
}

func (c *SingleCallToolActionCoordinatorV2) completeCoordinationV2(ctx context.Context, current contract.SingleCallToolActionCoordinationFactV2, result contract.SingleCallToolActionResultV2, now time.Time) (contract.SingleCallToolActionCoordinationFactV2, error) {
	next, err := contract.CompleteSingleCallToolActionCoordinationFactV2(current, result, now)
	if err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, err
	}
	cas, err := applicationports.SealSingleCallToolActionCoordinationCASRequestV2(applicationports.SingleCallToolActionCoordinationCASRequestV2{Scope: current.Request.Action.ExecutionScope, ID: current.ID, ExpectedRevision: current.Revision, ExpectedDigest: current.Digest, Next: next})
	if err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, err
	}
	stored, casErr := c.config.Facts.CompareAndSwapSingleCallToolActionCoordinationV2(ctx, cas)
	if casErr == nil {
		if err = validateSingleCallCoordinationForRequestV2(stored, current.Request); err != nil {
			return contract.SingleCallToolActionCoordinationFactV2{}, err
		}
		if stored.Digest != next.Digest || stored.Result == nil || next.Result == nil || *stored.Result != *next.Result {
			return contract.SingleCallToolActionCoordinationFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "single-call V2 completion CAS returned another successor")
		}
		return stored, nil
	}
	inspected, inspectErr := c.config.Facts.InspectSingleCallToolActionCoordinationV2(context.WithoutCancel(ctx), current.Request.Action.ExecutionScope, current.ID)
	if inspectErr != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, casErr
	}
	if err = validateSingleCallCoordinationForRequestV2(inspected, current.Request); err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, err
	}
	if inspected.State == contract.SingleCallToolActionCompletedV2 && inspected.Digest == next.Digest && inspected.Result != nil && next.Result != nil && *inspected.Result == *next.Result {
		return inspected, nil
	}
	return contract.SingleCallToolActionCoordinationFactV2{}, casErr
}

func (c *SingleCallToolActionCoordinatorV2) inspectExactCoordinationV2(ctx context.Context, request contract.SingleCallToolActionRequestV2) (contract.SingleCallToolActionCoordinationFactV2, error) {
	fact, err := c.config.Facts.InspectSingleCallToolActionCoordinationV2(ctx, request.Action.ExecutionScope, request.ID)
	if err != nil {
		return contract.SingleCallToolActionCoordinationFactV2{}, err
	}
	return fact, validateSingleCallCoordinationForRequestV2(fact, request)
}

func validateSingleCallCoordinationForRequestV2(fact contract.SingleCallToolActionCoordinationFactV2, request contract.SingleCallToolActionRequestV2) error {
	if err := fact.Validate(); err != nil {
		return err
	}
	if fact.ID != request.ID || fact.Request.Digest != request.Digest || !runtimeports.SameExecutionScopeV2(fact.Request.Action.ExecutionScope, request.Action.ExecutionScope) {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "single-call V2 coordination belongs to another request")
	}
	return nil
}

func (c *SingleCallToolActionCoordinatorV2) nowAfterV2(previous time.Time) (time.Time, error) {
	if c == nil || c.config.Clock == nil {
		return time.Time{}, core.NewError(core.ErrorUnavailable, core.ReasonClockRegression, "single-call V2 coordinator clock is unavailable")
	}
	now := c.config.Clock()
	if now.IsZero() || !previous.IsZero() && now.Before(previous) {
		return time.Time{}, core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "single-call V2 coordinator clock regressed")
	}
	return now, nil
}

type singleCallCoordinatorGateEntryV2 struct {
	mu   sync.Mutex
	refs int
}
type singleCallCoordinatorGateV2 struct {
	mu      sync.Mutex
	entries map[string]*singleCallCoordinatorGateEntryV2
}

func (g *singleCallCoordinatorGateV2) acquire(key string) func() {
	g.mu.Lock()
	e := g.entries[key]
	if e == nil {
		e = &singleCallCoordinatorGateEntryV2{}
		g.entries[key] = e
	}
	e.refs++
	g.mu.Unlock()
	e.mu.Lock()
	return func() {
		e.mu.Unlock()
		g.mu.Lock()
		e.refs--
		if e.refs == 0 {
			delete(g.entries, key)
		}
		g.mu.Unlock()
	}
}

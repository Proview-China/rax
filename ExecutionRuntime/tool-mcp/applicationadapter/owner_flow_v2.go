package applicationadapter

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolaction "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/action"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type ToolOwnerSingleCallExecutionV2 struct {
	Request applicationcontract.SingleCallToolActionRequestV2 `json:"request"`
	Binding SingleCallToolActionBindingCurrentProjectionV2    `json:"binding"`
}

func (v ToolOwnerSingleCallExecutionV2) Validate() error {
	if err := v.Request.Validate(); err != nil {
		return err
	}
	if err := v.Binding.Validate(); err != nil {
		return err
	}
	resolve := SingleCallToolActionBindingResolveRequestV2{ApplicationRequest: v.Request, SourceSubject: v.Request.Action.PendingSubject, RequestedExpiresUnixNano: v.Binding.RequestedExpiresUnixNano}
	_, issuance, err := sealBindingIssuanceV2(resolve)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(v.Binding.IssuanceSubject, issuance) || v.Binding.CandidateClosure.ApplicationInput.RequestID != v.Request.ID || v.Binding.CandidateClosure.ApplicationInput.RequestDigest != v.Request.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Tool Owner V2 binding differs from immutable request")
	}
	candidate := v.Binding.CandidateClosure.Candidate
	if candidate.ObjectRef() != v.Binding.CandidateRef || candidate.PendingAction.ID != v.Request.Action.PendingSubject.PendingActionRef || candidate.PendingAction.RequestDigest != v.Request.Action.PendingSubject.PendingActionDigest || candidate.TenantID != v.Request.Action.ExecutionScope.Identity.TenantID || candidate.RunID != string(v.Request.Action.PendingSubject.Run.RunID) || candidate.SessionID != v.Request.Action.PendingSubject.SessionID || candidate.TurnID != strconv.FormatUint(uint64(v.Request.Action.PendingSubject.Turn), 10) || candidate.OperationScopeDigest != v.Request.Action.ExecutionScopeDigest || candidate.EffectKind != runtimeports.OperationScopeEvidenceActionEffectKindV3 {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Tool Owner V2 execution differs from BindingV2/CandidateV3")
	}
	return nil
}

func (v ToolOwnerSingleCallExecutionV2) ValidateCurrent(now time.Time) error {
	if err := v.Validate(); err != nil {
		return err
	}
	if err := v.Request.ValidateCurrent(now); err != nil {
		return err
	}
	resolve := SingleCallToolActionBindingResolveRequestV2{ApplicationRequest: v.Request, SourceSubject: v.Request.Action.PendingSubject, RequestedExpiresUnixNano: v.Binding.RequestedExpiresUnixNano}
	if err := v.Binding.ValidateAgainst(resolve, now); err != nil {
		return err
	}
	return nil
}

// ToolOwnerSingleCallExecutionPortV2 is an owner-local completion seam. It is
// not a Provider port and receives no Evidence Consumption supplied by a caller.
type ToolOwnerSingleCallExecutionPortV2 interface {
	ExecuteBoundSingleCallToolActionV2(context.Context, ToolOwnerSingleCallExecutionV2) (toolcontract.ToolResultV2, error)
	InspectBoundSingleCallToolActionV2(context.Context, ToolOwnerSingleCallExecutionV2) (toolcontract.ToolResultV2, error)
}

// ToolOwnerSingleCallStartOrInspectPortV2 is the only execution seam accepted
// by durable composition. StartOrInspect must be create-once for the exact
// ExecutionAttemptID derived by the Tool Owner marker.
type ToolOwnerSingleCallStartOrInspectPortV2 interface {
	StartOrInspectBoundSingleCallToolActionV2(context.Context, ToolOwnerSingleCallStartOrInspectInputV2) (toolcontract.ToolResultV2, error)
	InspectBoundSingleCallToolActionV2(context.Context, ToolOwnerSingleCallStartOrInspectInputV2) (toolcontract.ToolResultV2, error)
}

type ToolOwnerSingleCallStartOrInspectInputV2 struct {
	Execution          ToolOwnerSingleCallExecutionV2 `json:"execution"`
	ExecutionAttemptID string                         `json:"execution_attempt_id"`
}

func (v ToolOwnerSingleCallStartOrInspectInputV2) Validate() error {
	if err := v.Execution.Validate(); err != nil {
		return err
	}
	if err := toolcontract.ValidateStableID(v.ExecutionAttemptID); err != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool Owner downstream execution attempt ID is invalid")
	}
	return nil
}

// ToolOwnerSettledResultReaderV2 is the Tool Owner's exact read after its
// DomainResult -> Runtime Settlement -> ApplySettlement chain has committed.
// It exposes no create/apply/Provider capability.
type ToolOwnerSettledResultReaderV2 interface {
	InspectSettledResultForApplyV2(string, toolcontract.ObjectRef) (toolcontract.ToolResultV2, error)
}

type ToolOwnerSingleCallFlowV2 interface {
	StartOrInspectToolOwnerSingleCallV2(context.Context, ToolOwnerSingleCallExecutionV2) (toolcontract.ToolResultV2, error)
	InspectToolOwnerSingleCallV2(context.Context, ToolOwnerSingleCallExecutionV2) (toolcontract.ToolResultV2, error)
	InspectToolOwnerSingleCallByKeyV2(context.Context, applicationcontract.SingleCallToolActionInspectKeyV2) (ToolOwnerSingleCallExecutionV2, toolcontract.ToolResultV2, error)
}

// ToolOwnerSingleCallFlowImplV2 provides only create-once/start-or-inspect
// coordination over an injected Tool Owner execution seam. It does not call a
// Provider or create Evidence, DomainResult, Settlement or a production root.
type ToolOwnerSingleCallFlowImplV2 struct {
	execution       ToolOwnerSingleCallExecutionPortV2
	settled         ToolOwnerSettledResultReaderV2
	claims          ToolOwnerSingleCallClaimStoreV2
	states          ToolOwnerSingleCallExecutionStateStoreV2
	entryLeases     ToolOwnerSingleCallEntryLeaseStoreV2
	facts           toolaction.FactStoreV2
	start           ToolOwnerSingleCallStartOrInspectPortV2
	durable         bool
	clock           ClockV1
	clockMu         sync.Mutex
	lastNow         time.Time
	recoveryTimeout time.Duration
	entryLeaseTTL   time.Duration
	incarnationID   string
	entryMu         sync.Mutex
	entryGate       map[string]*toolOwnerExecutionEntryGateV2
}

const (
	defaultToolOwnerRecoveryTimeoutV2 = 5 * time.Second
	defaultToolOwnerEntryLeaseTTLV2   = 2 * time.Second
	initialToolOwnerEntryPollDelayV2  = 25 * time.Millisecond
	maxToolOwnerEntryPollDelayV2      = 100 * time.Millisecond
)

var (
	toolOwnerFlowIncarnationCounterV2 atomic.Uint64
	toolOwnerFlowProcessNonceV2       = strconv.FormatInt(time.Now().UnixNano(), 10)
)

type toolOwnerExecutionEntryGateV2 struct {
	token chan struct{}
	refs  int
}

// NewDurableToolOwnerSingleCallFlowV2 installs the frozen restart-safe
// Tool-owned composition. It does not create a production root and does not
// make CoordinationStoreV1 durable.
func NewDurableToolOwnerSingleCallFlowV2(execution ToolOwnerSingleCallStartOrInspectPortV2, settled ToolOwnerSettledResultReaderV2, claims ToolOwnerSingleCallClaimStoreV2, states ToolOwnerSingleCallExecutionStateStoreV2, facts toolaction.FactStoreV2, clock ClockV1) (*ToolOwnerSingleCallFlowImplV2, error) {
	return NewDurableToolOwnerSingleCallFlowWithRecoveryTimeoutV2(execution, settled, claims, states, facts, clock, defaultToolOwnerRecoveryTimeoutV2)
}

// NewDurableToolOwnerSingleCallFlowWithRecoveryTimeoutV2 keeps recovery
// bounded after the caller transport context is cancelled. External Inspect is
// additionally capped by the exact execution/input expiry at each call site.
func NewDurableToolOwnerSingleCallFlowWithRecoveryTimeoutV2(execution ToolOwnerSingleCallStartOrInspectPortV2, settled ToolOwnerSettledResultReaderV2, claims ToolOwnerSingleCallClaimStoreV2, states ToolOwnerSingleCallExecutionStateStoreV2, facts toolaction.FactStoreV2, clock ClockV1, recoveryTimeout time.Duration) (*ToolOwnerSingleCallFlowImplV2, error) {
	incarnationID, err := newToolOwnerFlowIncarnationIDV2()
	if err != nil {
		return nil, err
	}
	entryLeaseTTL := defaultToolOwnerEntryLeaseTTLV2
	if recoveryTimeout > 0 && recoveryTimeout < entryLeaseTTL {
		entryLeaseTTL = recoveryTimeout
	}
	return NewDurableToolOwnerSingleCallFlowWithEntryLeaseV2(execution, settled, claims, states, facts, clock, recoveryTimeout, entryLeaseTTL, incarnationID)
}

// NewDurableToolOwnerSingleCallFlowWithEntryLeaseV2 makes the owner
// incarnation and short entry-lease TTL explicit for composition and
// deterministic crash/takeover tests.
func NewDurableToolOwnerSingleCallFlowWithEntryLeaseV2(execution ToolOwnerSingleCallStartOrInspectPortV2, settled ToolOwnerSettledResultReaderV2, claims ToolOwnerSingleCallClaimStoreV2, states ToolOwnerSingleCallExecutionStateStoreV2, facts toolaction.FactStoreV2, clock ClockV1, recoveryTimeout, entryLeaseTTL time.Duration, incarnationID string) (*ToolOwnerSingleCallFlowImplV2, error) {
	if isNilFlowDependencyV1(execution) || isNilFlowDependencyV1(settled) || isNilFlowDependencyV1(claims) || isNilFlowDependencyV1(states) || isNilFlowDependencyV1(facts) || isNilFlowDependencyV1(clock) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "durable Tool Owner V2 flow dependencies are incomplete")
	}
	leases, ok := states.(ToolOwnerSingleCallEntryLeaseStoreV2)
	if !ok || isNilFlowDependencyV1(leases) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "durable Tool Owner V2 entry lease store is required")
	}
	if recoveryTimeout <= 0 || entryLeaseTTL <= 0 || entryLeaseTTL > recoveryTimeout || toolcontract.ValidateStableID(incarnationID) != nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "durable Tool Owner V2 recovery and entry lease options are invalid")
	}
	return &ToolOwnerSingleCallFlowImplV2{start: execution, settled: settled, claims: claims, states: states, entryLeases: leases, facts: facts, durable: true, clock: clock, recoveryTimeout: recoveryTimeout, entryLeaseTTL: entryLeaseTTL, incarnationID: incarnationID, entryGate: make(map[string]*toolOwnerExecutionEntryGateV2)}, nil
}

func newToolOwnerFlowIncarnationIDV2() (string, error) {
	return toolcontract.StableID("tool-owner-flow-incarnation-v2", toolOwnerFlowProcessNonceV2, strconv.FormatUint(toolOwnerFlowIncarnationCounterV2.Add(1), 10))
}

// NewToolOwnerSingleCallFlowV2 is retained for isolated fixtures. Production
// composition must inject a durable claim store through
// NewToolOwnerSingleCallFlowWithClaimStoreV2.
func NewToolOwnerSingleCallFlowV2(execution ToolOwnerSingleCallExecutionPortV2, clock ClockV1) (*ToolOwnerSingleCallFlowImplV2, error) {
	if isNilFlowDependencyV1(execution) || isNilFlowDependencyV1(clock) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Tool Owner V2 flow dependencies are incomplete")
	}
	settled, _ := execution.(ToolOwnerSettledResultReaderV2)
	return &ToolOwnerSingleCallFlowImplV2{execution: execution, settled: settled, claims: NewInMemoryToolOwnerSingleCallClaimStoreV2(), clock: clock}, nil
}

func NewToolOwnerSingleCallFlowWithClaimStoreV2(execution ToolOwnerSingleCallExecutionPortV2, claims ToolOwnerSingleCallClaimStoreV2, clock ClockV1) (*ToolOwnerSingleCallFlowImplV2, error) {
	settled, _ := execution.(ToolOwnerSettledResultReaderV2)
	if isNilFlowDependencyV1(settled) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Tool Owner V2 exact settled-result reader is required with a restart-safe claim store")
	}
	return NewToolOwnerSingleCallFlowWithStoresV2(execution, settled, claims, clock)
}

// NewToolOwnerSingleCallFlowWithStoresV2 is the contract-correct constructor
// for a restart-safe composition. The settled reader must be backed by the
// Tool Owner that atomically linked ApplySettlement and ToolResult.
func NewToolOwnerSingleCallFlowWithStoresV2(execution ToolOwnerSingleCallExecutionPortV2, settled ToolOwnerSettledResultReaderV2, claims ToolOwnerSingleCallClaimStoreV2, clock ClockV1) (*ToolOwnerSingleCallFlowImplV2, error) {
	if isNilFlowDependencyV1(execution) || isNilFlowDependencyV1(settled) || isNilFlowDependencyV1(claims) || isNilFlowDependencyV1(clock) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Tool Owner V2 flow dependencies are incomplete")
	}
	return &ToolOwnerSingleCallFlowImplV2{execution: execution, settled: settled, claims: claims, clock: clock}, nil
}

func (f *ToolOwnerSingleCallFlowImplV2) StartOrInspectToolOwnerSingleCallV2(ctx context.Context, input ToolOwnerSingleCallExecutionV2) (toolcontract.ToolResultV2, error) {
	if f == nil || isNilFlowDependencyV1(ctx) {
		return toolcontract.ToolResultV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool Owner V2 flow input is invalid")
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if f.durable {
		return f.startOrInspectDurableV2(ctx, input)
	}
	now, err := f.nowAfterV2(time.Time{})
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if err := input.ValidateCurrent(now); err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	claim, err := newToolOwnerSingleCallClaimV2(input, now.UnixNano())
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	record := ToolOwnerSingleCallClaimRecordV2{Claim: claim, Input: input}
	winner, created, createErr := f.claims.CreateToolOwnerSingleCallClaimV2(ctx, record)
	if createErr != nil {
		key, keyErr := applicationcontract.SealSingleCallToolActionInspectKeyV2(input.Request)
		if keyErr != nil {
			return toolcontract.ToolResultV2{}, keyErr
		}
		recoveryCtx, cancel := boundedOwnerLocalRecoveryContextV2(ctx, defaultToolOwnerRecoveryTimeoutV2)
		defer cancel()
		winner, err = f.claims.InspectToolOwnerSingleCallClaimV2(recoveryCtx, key)
		if err != nil {
			return toolcontract.ToolResultV2{}, err
		}
		created = false
	}
	same, compareErr := sameToolOwnerSingleCallClaimPayloadV2(winner, record)
	if compareErr != nil {
		return toolcontract.ToolResultV2{}, compareErr
	}
	if !same {
		return toolcontract.ToolResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Tool Owner V2 claim recovery found different content")
	}
	if !created {
		return f.inspectClaimedToolOwnerSingleCallV2(ctx, winner.Input)
	}

	result, executeErr := f.execution.ExecuteBoundSingleCallToolActionV2(ctx, input)
	if executeErr != nil {
		inspectNow, clockErr := f.nowAfterV2(now)
		if clockErr != nil {
			return toolcontract.ToolResultV2{}, clockErr
		}
		if currentErr := input.ValidateCurrent(inspectNow); currentErr != nil {
			return toolcontract.ToolResultV2{}, currentErr
		}
		recoveryCtx, cancel := f.boundedRecoveryContextV2(ctx, inspectNow, inputExpiryUnixNanoV2(input))
		defer cancel()
		result, executeErr = f.execution.InspectBoundSingleCallToolActionV2(recoveryCtx, input)
	}
	if executeErr == nil {
		executeErr = f.validatePersistedToolOwnerResultV2(input, result)
	}
	return cloneToolResultV2(result), executeErr
}

func (f *ToolOwnerSingleCallFlowImplV2) InspectToolOwnerSingleCallV2(ctx context.Context, input ToolOwnerSingleCallExecutionV2) (toolcontract.ToolResultV2, error) {
	if f == nil || isNilFlowDependencyV1(ctx) {
		return toolcontract.ToolResultV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool Owner V2 inspect input is invalid")
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if f.durable {
		return f.inspectDurableV2(ctx, input)
	}
	if _, err := f.nowAfterV2(time.Time{}); err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if err := input.Validate(); err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	key, err := applicationcontract.SealSingleCallToolActionInspectKeyV2(input.Request)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	record, err := f.claims.InspectToolOwnerSingleCallClaimV2(ctx, key)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if record.Claim.BindingRef != input.Binding.Ref || record.Input.Request.Digest != input.Request.Digest {
		return toolcontract.ToolResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Tool Owner V2 Inspect input differs from persisted claim")
	}
	return f.inspectClaimedToolOwnerSingleCallV2(ctx, record.Input)
}

func (f *ToolOwnerSingleCallFlowImplV2) InspectToolOwnerSingleCallByKeyV2(ctx context.Context, key applicationcontract.SingleCallToolActionInspectKeyV2) (ToolOwnerSingleCallExecutionV2, toolcontract.ToolResultV2, error) {
	if f == nil || isNilFlowDependencyV1(ctx) {
		return ToolOwnerSingleCallExecutionV2{}, toolcontract.ToolResultV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool Owner V2 keyed Inspect input is invalid")
	}
	if _, err := f.nowAfterV2(time.Time{}); err != nil {
		return ToolOwnerSingleCallExecutionV2{}, toolcontract.ToolResultV2{}, err
	}
	record, err := f.claims.InspectToolOwnerSingleCallClaimV2(ctx, key)
	if err != nil {
		return ToolOwnerSingleCallExecutionV2{}, toolcontract.ToolResultV2{}, err
	}
	if f.durable {
		result, inspectErr := f.inspectDurableClaimedV2(ctx, record)
		return record.Input, result, inspectErr
	}
	result, err := f.inspectClaimedToolOwnerSingleCallV2(ctx, record.Input)
	return record.Input, result, err
}

func (f *ToolOwnerSingleCallFlowImplV2) startOrInspectDurableV2(ctx context.Context, input ToolOwnerSingleCallExecutionV2) (toolcontract.ToolResultV2, error) {
	now, err := f.nowAfterV2(time.Time{})
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if err = input.ValidateCurrent(now); err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	claim, err := newToolOwnerSingleCallClaimV2(input, now.UnixNano())
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	requested := ToolOwnerSingleCallClaimRecordV2{Claim: claim, Input: input}
	winner, _, createErr := f.claims.CreateToolOwnerSingleCallClaimV2(ctx, requested)
	key, keyErr := applicationcontract.SealSingleCallToolActionInspectKeyV2(input.Request)
	if keyErr != nil {
		return toolcontract.ToolResultV2{}, keyErr
	}
	if createErr != nil {
		recoveryCtx, cancel := f.boundedRecoveryContextV2(ctx, now, inputExpiryUnixNanoV2(input))
		defer cancel()
		winner, err = f.claims.InspectToolOwnerSingleCallClaimV2(recoveryCtx, key)
		if err != nil {
			return toolcontract.ToolResultV2{}, createErr
		}
	}
	same, err := sameToolOwnerSingleCallClaimPayloadV2(winner, requested)
	if err != nil || !same {
		if err != nil {
			return toolcontract.ToolResultV2{}, err
		}
		return toolcontract.ToolResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "durable Tool Owner claim winner binds different input")
	}
	startState, err := NewToolOwnerSingleCallExecutionStartV2(winner, now.UnixNano())
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	state, _, stateErr := f.states.CreateExecutionStartV2(ctx, startState)
	if stateErr != nil {
		recoveryCtx, cancel := f.boundedRecoveryContextV2(ctx, now, inputExpiryUnixNanoV2(winner.Input))
		defer cancel()
		state, err = f.states.InspectExecutionStateV2(recoveryCtx, key)
		if err != nil {
			return toolcontract.ToolResultV2{}, stateErr
		}
	}
	if !sameExecutionIdentityV2(state, startState) {
		return toolcontract.ToolResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "durable Tool Owner execution marker binds different input")
	}
	return f.coordinateDurableEntryV2(ctx, winner.Input, state, true)
}

func (f *ToolOwnerSingleCallFlowImplV2) startDurableLeaseHolderV2(ctx context.Context, input ToolOwnerSingleCallExecutionV2, state ToolOwnerSingleCallExecutionStateV2, lease ToolOwnerSingleCallEntryLeaseV2) (toolcontract.ToolResultV2, error) {
	downstream := ToolOwnerSingleCallStartOrInspectInputV2{Execution: input, ExecutionAttemptID: state.ExecutionAttemptID}
	if err := downstream.Validate(); err != nil {
		return toolcontract.ToolResultV2{}, f.handoffDurableEntryLeaseV2(ctx, lease, ToolOwnerEntryStartOrInspectV2, err)
	}
	state, entryNow, err := f.validateDurableActualEntryV2(ctx, input, state, lease, ToolOwnerEntryStartOrInspectV2)
	if err != nil {
		return toolcontract.ToolResultV2{}, f.handoffDurableEntryLeaseV2(ctx, lease, ToolOwnerEntryStartOrInspectV2, err)
	}
	result, startErr := f.start.StartOrInspectBoundSingleCallToolActionV2(ctx, downstream)
	if startErr == nil {
		result, err = f.validateAndMarkDurableSettledV2(ctx, input, state, result)
		if err != nil {
			markedNow, clockErr := f.nowAfterV2(entryNow)
			if clockErr != nil {
				return toolcontract.ToolResultV2{}, f.handoffDurableEntryLeaseV2(ctx, lease, ToolOwnerEntryInspectV2, clockErr)
			}
			current, markErr := f.advanceDurableInspectOnlyAfterEntryV2(ctx, input, state, err, ToolOwnerEntryOutcomeUnknownV2, markedNow)
			if markErr != nil {
				return toolcontract.ToolResultV2{}, f.handoffDurableEntryLeaseV2(ctx, lease, ToolOwnerEntryInspectV2, markErr)
			}
			if current.State == ToolOwnerExecutionSettledV2 {
				return f.readDurableSettledV2(ctx, input, current)
			}
			return toolcontract.ToolResultV2{}, f.handoffDurableEntryLeaseV2(ctx, lease, ToolOwnerEntryInspectV2, err)
		}
		return result, nil
	}
	// A returned external entry error cannot prove that the actual point was
	// not crossed. Persist inspect_only before any recovery attempt so every
	// successor is restricted to the original exact Attempt Inspect.
	inspectNow, clockErr := f.nowAfterV2(entryNow)
	if clockErr != nil {
		return toolcontract.ToolResultV2{}, f.handoffDurableEntryLeaseV2(ctx, lease, ToolOwnerEntryInspectV2, clockErr)
	}
	if err = input.ValidateCurrent(inspectNow); err != nil {
		return toolcontract.ToolResultV2{}, f.handoffDurableEntryLeaseV2(ctx, lease, ToolOwnerEntryInspectV2, err)
	}
	if err = lease.ValidateCurrent(inspectNow); err != nil {
		return toolcontract.ToolResultV2{}, f.handoffDurableEntryLeaseV2(ctx, lease, ToolOwnerEntryInspectV2, err)
	}
	state, err = f.advanceDurableInspectOnlyAfterEntryV2(ctx, input, state, startErr, ToolOwnerEntryOutcomeUnknownV2, inspectNow)
	if err != nil {
		return toolcontract.ToolResultV2{}, f.handoffDurableEntryLeaseV2(ctx, lease, ToolOwnerEntryInspectV2, err)
	}
	if state.State == ToolOwnerExecutionSettledV2 {
		return f.readDurableSettledV2(ctx, input, state)
	}
	recoveryCtx, cancel := f.boundedRecoveryContextV2(ctx, inspectNow, minUnixNanoV2(inputExpiryUnixNanoV2(input), state.ExpiresUnixNano, lease.ExpiresUnixNano))
	defer cancel()
	inspected, inspectErr := f.start.InspectBoundSingleCallToolActionV2(recoveryCtx, downstream)
	if inspectErr == nil {
		result, err = f.validateAndMarkDurableSettledV2(recoveryCtx, input, state, inspected)
		if err != nil {
			return toolcontract.ToolResultV2{}, f.handoffDurableEntryLeaseV2(ctx, lease, ToolOwnerEntryInspectV2, err)
		}
		return result, nil
	}
	return toolcontract.ToolResultV2{}, f.handoffDurableEntryLeaseV2(ctx, lease, ToolOwnerEntryInspectV2, inspectErr)
}

func (f *ToolOwnerSingleCallFlowImplV2) advanceDurableInspectOnlyAfterEntryV2(ctx context.Context, input ToolOwnerSingleCallExecutionV2, state ToolOwnerSingleCallExecutionStateV2, entryErr error, class ToolOwnerSingleCallUnknownClassV2, marked time.Time) (ToolOwnerSingleCallExecutionStateV2, error) {
	if state.State == ToolOwnerExecutionInspectOnlyV2 || state.State == ToolOwnerExecutionSettledV2 {
		return state, nil
	}
	if state.State != ToolOwnerExecutionStartCommittedV2 || entryErr == nil ||
		(class != ToolOwnerEntryOutcomeUnknownV2 && class != ToolOwnerInspectionIndeterminateV2) || marked.IsZero() {
		return ToolOwnerSingleCallExecutionStateV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidTransition, "Tool Owner post-entry inspect-only transition is invalid")
	}
	unknown := ToolOwnerSingleCallExecutionUnknownV2{
		Class:          class,
		ErrorDigest:    durableFlowErrorDigestV2(entryErr, nil),
		MarkedUnixNano: marked.UnixNano(),
	}
	recoveryCtx, cancel := boundedOwnerLocalRecoveryContextV2(ctx, f.recoveryTimeout)
	defer cancel()
	next, err := f.states.AdvanceExecutionInspectOnlyV2(recoveryCtx, state.RefV2(), unknown)
	if err == nil {
		return next, nil
	}
	current, inspectErr := f.states.InspectExecutionStateV2(recoveryCtx, mustToolOwnerInspectKeyV2(input))
	if inspectErr != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, err
	}
	if !sameExecutionIdentityV2(current, state) {
		return ToolOwnerSingleCallExecutionStateV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Tool Owner post-entry marker changed identity")
	}
	if current.State == ToolOwnerExecutionInspectOnlyV2 || current.State == ToolOwnerExecutionSettledV2 {
		return current, nil
	}
	return ToolOwnerSingleCallExecutionStateV2{}, err
}

func (f *ToolOwnerSingleCallFlowImplV2) handoffDurableEntryLeaseV2(ctx context.Context, lease ToolOwnerSingleCallEntryLeaseV2, nextPhase ToolOwnerSingleCallEntryLeasePhaseV2, cause error) error {
	if cause == nil {
		cause = core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Tool Owner entry holder exited without a result")
	}
	if f == nil || isNilFlowDependencyV1(f.entryLeases) || lease.Validate() != nil || !isActiveEntryLeasePhaseV2(nextPhase) {
		return cause
	}
	handoffUnixNano := lease.AcquiredUnixNano
	now, clockErr := f.nowAfterV2(time.Unix(0, lease.AcquiredUnixNano))
	if clockErr == nil && now.UnixNano() >= lease.ExpiresUnixNano {
		// Natural takeover is already eligible in Owner time.
		return cause
	}
	if clockErr == nil {
		handoffUnixNano = now.UnixNano()
	}
	// A clock failure after an external boundary must not strand the active
	// lease. The acquisition time was already owner-validated and is a safe,
	// non-extending timestamp for the owner-local release transition.
	recoveryCtx, cancel := boundedOwnerLocalRecoveryContextV2(ctx, f.recoveryTimeout)
	defer cancel()
	handoff, advanceErr := f.entryLeases.AdvanceExecutionEntryLeaseHandoffV2(recoveryCtx, lease, nextPhase, handoffUnixNano)
	if advanceErr == nil && isEntryLeaseHandoffPhaseV2(handoff.Phase) && activeEntryLeasePhaseV2(handoff.Phase) == nextPhase {
		return cause
	}
	current, inspectErr := f.entryLeases.InspectExecutionEntryLeaseV2(recoveryCtx, lease.ExecutionAttemptID)
	if inspectErr == nil && sameEntryLeaseIdentityV2(current, ToolOwnerSingleCallEntryLeaseAcquireV2{
		RequestKey: lease.RequestKey, RequestDigest: lease.RequestDigest, ExecutionInputDigest: lease.ExecutionInputDigest,
		ExecutionAttemptID: lease.ExecutionAttemptID, HolderIncarnationID: lease.HolderIncarnationID, Phase: nextPhase,
		AcquiredUnixNano: handoffUnixNano, ExpiresUnixNano: lease.ExpiresUnixNano,
	}) && current.RefV2() != lease.RefV2() {
		// Either the exact handoff or its single CAS successor is durable.
		return cause
	}
	if advanceErr != nil {
		return advanceErr
	}
	if inspectErr != nil {
		return inspectErr
	}
	return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Tool Owner entry lease handoff did not become durable")
}

func (f *ToolOwnerSingleCallFlowImplV2) coordinateDurableEntryV2(ctx context.Context, input ToolOwnerSingleCallExecutionV2, initial ToolOwnerSingleCallExecutionStateV2, allowStart bool) (toolcontract.ToolResultV2, error) {
	release, err := f.acquireExecutionEntryGateV2(ctx, initial.ExecutionAttemptID)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	defer release()
	key := mustToolOwnerInspectKeyV2(input)
	for {
		if err := ctx.Err(); err != nil {
			return toolcontract.ToolResultV2{}, err
		}
		state, err := f.states.InspectExecutionStateV2(ctx, key)
		if err != nil {
			return toolcontract.ToolResultV2{}, err
		}
		if !sameExecutionIdentityV2(state, initial) {
			return toolcontract.ToolResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "durable Tool Owner execution marker changed before entry")
		}
		if state.State == ToolOwnerExecutionSettledV2 {
			return f.readDurableSettledV2(ctx, input, state)
		}
		phase := ToolOwnerEntryInspectV2
		if allowStart && state.State == ToolOwnerExecutionStartCommittedV2 {
			phase = ToolOwnerEntryStartOrInspectV2
		} else if state.State != ToolOwnerExecutionStartCommittedV2 && state.State != ToolOwnerExecutionInspectOnlyV2 {
			return toolcontract.ToolResultV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "durable Tool Owner execution marker state is unsupported")
		}
		// A durable handoff is the stronger recovery authority. In particular,
		// handoff_inspect must be consumed before a caller that is otherwise
		// allowed to StartOrInspect may re-enter the same exact Attempt.
		currentLease, leaseErr := f.entryLeases.InspectExecutionEntryLeaseV2(ctx, state.ExecutionAttemptID)
		switch {
		case leaseErr == nil && isEntryLeaseHandoffPhaseV2(currentLease.Phase):
			handoffPhase := activeEntryLeasePhaseV2(currentLease.Phase)
			if handoffPhase == ToolOwnerEntryInspectV2 {
				phase = ToolOwnerEntryInspectV2
			} else if handoffPhase != ToolOwnerEntryStartOrInspectV2 ||
				!allowStart || state.State != ToolOwnerExecutionStartCommittedV2 {
				return toolcontract.ToolResultV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "durable Tool Owner handoff does not authorize this caller entry phase")
			}
		case leaseErr == nil:
		case core.HasCategory(leaseErr, core.ErrorNotFound):
		default:
			return toolcontract.ToolResultV2{}, leaseErr
		}
		now, err := f.nowAfterV2(time.Unix(0, state.UpdatedUnixNano))
		if err != nil {
			return toolcontract.ToolResultV2{}, err
		}
		if err = input.ValidateCurrent(now); err != nil {
			return toolcontract.ToolResultV2{}, err
		}
		expires := minUnixNanoV2(inputExpiryUnixNanoV2(input), state.ExpiresUnixNano, now.Add(f.entryLeaseTTL).UnixNano())
		if expires <= now.UnixNano() {
			return toolcontract.ToolResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Tool Owner entry lease has no current lifetime")
		}
		request := ToolOwnerSingleCallEntryLeaseAcquireV2{
			RequestKey: key, RequestDigest: state.RequestDigest, ExecutionInputDigest: state.ExecutionInputDigest,
			ExecutionAttemptID: state.ExecutionAttemptID, HolderIncarnationID: f.incarnationID, Phase: phase,
			AcquiredUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
		}
		lease, acquired, err := f.entryLeases.TryAcquireExecutionEntryLeaseV2(ctx, request)
		if err != nil {
			if core.HasCategory(err, core.ErrorIndeterminate) {
				recoveryCtx, cancel := f.boundedRecoveryContextV2(ctx, now, expires)
				inspected, inspectErr := f.entryLeases.InspectExecutionEntryLeaseV2(recoveryCtx, state.ExecutionAttemptID)
				cancel()
				if inspectErr != nil {
					return toolcontract.ToolResultV2{}, err
				}
				if isEntryLeaseHandoffPhaseV2(inspected.Phase) {
					if activeEntryLeasePhaseV2(inspected.Phase) != phase {
						return toolcontract.ToolResultV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "durable Tool Owner recovered handoff forbids the requested entry phase")
					}
					// Handoff is not a held lease. Re-enter the create-once
					// acquisition CAS immediately; waiting for Owner time
					// would deadlock a fixed clock.
					continue
				}
				lease = inspected
				acquired = inspected.HolderIncarnationID == f.incarnationID && inspected.Phase == phase && inspected.AcquiredUnixNano == now.UnixNano()
			} else {
				return toolcontract.ToolResultV2{}, err
			}
		}
		if !sameEntryLeaseIdentityV2(lease, request) {
			return toolcontract.ToolResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "durable Tool Owner entry lease winner binds different execution")
		}
		if !acquired {
			result, retry, waitErr := f.waitForDurableEntryWinnerV2(ctx, input, initial, lease)
			if waitErr != nil {
				return toolcontract.ToolResultV2{}, waitErr
			}
			if retry {
				continue
			}
			return result, nil
		}
		fresh, err := f.states.InspectExecutionStateV2(ctx, key)
		if err != nil {
			return toolcontract.ToolResultV2{}, f.handoffDurableEntryLeaseV2(ctx, lease, phase, err)
		}
		if !sameExecutionIdentityV2(fresh, initial) {
			driftErr := core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "durable Tool Owner execution marker drifted after entry lease acquisition")
			return toolcontract.ToolResultV2{}, f.handoffDurableEntryLeaseV2(ctx, lease, phase, driftErr)
		}
		if fresh.State == ToolOwnerExecutionSettledV2 {
			return f.readDurableSettledV2(ctx, input, fresh)
		}
		if phase == ToolOwnerEntryStartOrInspectV2 && fresh.State == ToolOwnerExecutionStartCommittedV2 {
			return f.startDurableLeaseHolderV2(ctx, input, fresh, lease)
		}
		if phase == ToolOwnerEntryInspectV2 && (fresh.State == ToolOwnerExecutionStartCommittedV2 || fresh.State == ToolOwnerExecutionInspectOnlyV2) {
			return f.inspectAndSettleDurableLeaseHolderV2(ctx, input, fresh, lease)
		}
		stateErr := core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Tool Owner execution state changed after entry lease acquisition")
		return toolcontract.ToolResultV2{}, f.handoffDurableEntryLeaseV2(ctx, lease, phase, stateErr)
	}
}

func (f *ToolOwnerSingleCallFlowImplV2) waitForDurableEntryWinnerV2(ctx context.Context, input ToolOwnerSingleCallExecutionV2, initial ToolOwnerSingleCallExecutionStateV2, lease ToolOwnerSingleCallEntryLeaseV2) (toolcontract.ToolResultV2, bool, error) {
	now, err := f.nowAfterV2(time.Unix(0, initial.UpdatedUnixNano))
	if err != nil {
		return toolcontract.ToolResultV2{}, false, err
	}
	waitCtx, cancel := f.boundedEntryWaitContextV2(ctx, now, minUnixNanoV2(inputExpiryUnixNanoV2(input), initial.ExpiresUnixNano))
	defer cancel()
	pollDelay := initialToolOwnerEntryPollDelayV2
	timer := time.NewTimer(pollDelay)
	defer timer.Stop()
	key := mustToolOwnerInspectKeyV2(input)
	pollCount := 0
	for {
		select {
		case <-waitCtx.Done():
			return toolcontract.ToolResultV2{}, false, waitCtx.Err()
		case <-timer.C:
		}
		state, inspectErr := f.states.InspectExecutionStateV2(waitCtx, key)
		if inspectErr != nil {
			return toolcontract.ToolResultV2{}, false, inspectErr
		}
		if !sameExecutionIdentityV2(state, initial) {
			return toolcontract.ToolResultV2{}, false, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Tool Owner execution marker drifted while waiting for entry lease")
		}
		if state.State == ToolOwnerExecutionSettledV2 {
			result, readErr := f.readDurableSettledV2(waitCtx, input, state)
			return result, false, readErr
		}
		// A current lease cannot legally gain a successor before its expiry.
		// Re-read the durable head periodically to catch store corruption or a
		// successor published by an owner whose clock has advanced further,
		// without doubling every waiter's SQLite read load on each poll.
		pollCount++
		if pollCount == 1 || pollCount%10 == 0 {
			currentLease, leaseErr := f.entryLeases.InspectExecutionEntryLeaseV2(waitCtx, initial.ExecutionAttemptID)
			if leaseErr != nil {
				return toolcontract.ToolResultV2{}, false, leaseErr
			}
			if currentLease.RefV2() != lease.RefV2() {
				return toolcontract.ToolResultV2{}, true, nil
			}
		}
		currentNow, clockErr := f.nowAfterV2(now)
		if clockErr != nil {
			return toolcontract.ToolResultV2{}, false, clockErr
		}
		now = currentNow
		if now.UnixNano() >= lease.ExpiresUnixNano {
			return toolcontract.ToolResultV2{}, true, nil
		}
		if pollDelay < maxToolOwnerEntryPollDelayV2 {
			pollDelay *= 2
			if pollDelay > maxToolOwnerEntryPollDelayV2 {
				pollDelay = maxToolOwnerEntryPollDelayV2
			}
		}
		timer.Reset(pollDelay)
	}
}

func (f *ToolOwnerSingleCallFlowImplV2) acquireExecutionEntryGateV2(ctx context.Context, attemptID string) (func(), error) {
	if ctx == nil || ctx.Err() != nil {
		if ctx != nil {
			return nil, ctx.Err()
		}
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool Owner entry gate context is invalid")
	}
	f.entryMu.Lock()
	if f.entryGate == nil {
		f.entryGate = make(map[string]*toolOwnerExecutionEntryGateV2)
	}
	gate := f.entryGate[attemptID]
	if gate == nil {
		gate = &toolOwnerExecutionEntryGateV2{token: make(chan struct{}, 1)}
		gate.token <- struct{}{}
		f.entryGate[attemptID] = gate
	}
	gate.refs++
	f.entryMu.Unlock()

	select {
	case <-ctx.Done():
		f.entryMu.Lock()
		gate.refs--
		if gate.refs == 0 {
			delete(f.entryGate, attemptID)
		}
		f.entryMu.Unlock()
		return nil, ctx.Err()
	case <-gate.token:
	}
	return func() {
		gate.token <- struct{}{}
		f.entryMu.Lock()
		gate.refs--
		if gate.refs == 0 {
			delete(f.entryGate, attemptID)
		}
		f.entryMu.Unlock()
	}, nil
}

func (f *ToolOwnerSingleCallFlowImplV2) inspectDurableV2(ctx context.Context, input ToolOwnerSingleCallExecutionV2) (toolcontract.ToolResultV2, error) {
	if err := input.Validate(); err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	key, err := applicationcontract.SealSingleCallToolActionInspectKeyV2(input.Request)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	record, err := f.claims.InspectToolOwnerSingleCallClaimV2(ctx, key)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if record.Claim.BindingRef != input.Binding.Ref || record.Input.Request.Digest != input.Request.Digest {
		return toolcontract.ToolResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "durable Tool Owner Inspect input differs from claim")
	}
	return f.inspectDurableClaimedV2(ctx, record)
}

func (f *ToolOwnerSingleCallFlowImplV2) inspectDurableClaimedV2(ctx context.Context, record ToolOwnerSingleCallClaimRecordV2) (toolcontract.ToolResultV2, error) {
	state, err := f.states.InspectExecutionStateV2(ctx, mustToolOwnerInspectKeyV2(record.Input))
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if state.State == ToolOwnerExecutionSettledV2 {
		return f.readDurableSettledV2(ctx, record.Input, state)
	}
	return f.coordinateDurableEntryV2(ctx, record.Input, state, false)
}

func (f *ToolOwnerSingleCallFlowImplV2) inspectAndSettleDurableLeaseHolderV2(ctx context.Context, input ToolOwnerSingleCallExecutionV2, state ToolOwnerSingleCallExecutionStateV2, lease ToolOwnerSingleCallEntryLeaseV2) (toolcontract.ToolResultV2, error) {
	downstream := ToolOwnerSingleCallStartOrInspectInputV2{Execution: input, ExecutionAttemptID: state.ExecutionAttemptID}
	if err := downstream.Validate(); err != nil {
		return toolcontract.ToolResultV2{}, f.handoffDurableEntryLeaseV2(ctx, lease, ToolOwnerEntryInspectV2, err)
	}
	state, now, err := f.validateDurableActualEntryV2(ctx, input, state, lease, ToolOwnerEntryInspectV2)
	if err != nil {
		return toolcontract.ToolResultV2{}, f.handoffDurableEntryLeaseV2(ctx, lease, ToolOwnerEntryInspectV2, err)
	}
	recoveryCtx, cancel := f.boundedRecoveryContextV2(ctx, now, minUnixNanoV2(inputExpiryUnixNanoV2(input), state.ExpiresUnixNano, lease.ExpiresUnixNano))
	defer cancel()
	result, err := f.start.InspectBoundSingleCallToolActionV2(recoveryCtx, downstream)
	if err != nil {
		nextPhase := ToolOwnerEntryInspectV2
		if state.State == ToolOwnerExecutionStartCommittedV2 {
			if core.HasCategory(err, core.ErrorNotFound) {
				// Frozen narrow exception: the marker is still start_committed
				// and exact Inspect proved this same create-once Attempt absent.
				// Preserve the exact Attempt coordinate and permit only its
				// durable handoff_start_or_inspect recovery.
				nextPhase = ToolOwnerEntryStartOrInspectV2
				return toolcontract.ToolResultV2{}, f.handoffDurableEntryLeaseV2(ctx, lease, nextPhase, err)
			}
			marked, clockErr := f.nowAfterV2(now)
			if clockErr != nil {
				return toolcontract.ToolResultV2{}, f.handoffDurableEntryLeaseV2(ctx, lease, ToolOwnerEntryInspectV2, clockErr)
			}
			current, markErr := f.advanceDurableInspectOnlyAfterEntryV2(ctx, input, state, err, ToolOwnerInspectionIndeterminateV2, marked)
			if markErr != nil {
				return toolcontract.ToolResultV2{}, f.handoffDurableEntryLeaseV2(ctx, lease, ToolOwnerEntryInspectV2, markErr)
			}
			if current.State == ToolOwnerExecutionSettledV2 {
				return f.readDurableSettledV2(ctx, input, current)
			}
		}
		return toolcontract.ToolResultV2{}, f.handoffDurableEntryLeaseV2(ctx, lease, nextPhase, err)
	}
	result, err = f.validateAndMarkDurableSettledV2(recoveryCtx, input, state, result)
	if err != nil {
		return toolcontract.ToolResultV2{}, f.handoffDurableEntryLeaseV2(ctx, lease, ToolOwnerEntryInspectV2, err)
	}
	return result, nil
}

// validateDurableActualEntryV2 is the final owner-local gate immediately
// before the external create-once StartOrInspect or exact Inspect seam. The
// acquisition timestamp is only a lower clock bound; it is never reused as
// proof that the input, lease, or execution current remains live.
func (f *ToolOwnerSingleCallFlowImplV2) validateDurableActualEntryV2(ctx context.Context, input ToolOwnerSingleCallExecutionV2, expected ToolOwnerSingleCallExecutionStateV2, lease ToolOwnerSingleCallEntryLeaseV2, phase ToolOwnerSingleCallEntryLeasePhaseV2) (ToolOwnerSingleCallExecutionStateV2, time.Time, error) {
	if err := ctx.Err(); err != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, time.Time{}, err
	}
	if !isActiveEntryLeasePhaseV2(phase) || lease.Phase != phase {
		return ToolOwnerSingleCallExecutionStateV2{}, time.Time{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "Tool Owner actual entry lease phase drifted")
	}
	current, err := f.states.InspectExecutionStateV2(ctx, mustToolOwnerInspectKeyV2(input))
	if err != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, time.Time{}, err
	}
	if err = ctx.Err(); err != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, time.Time{}, err
	}
	freshNow, err := f.nowAfterV2(time.Unix(0, lease.AcquiredUnixNano))
	if err != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, time.Time{}, err
	}
	if err = ctx.Err(); err != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, time.Time{}, err
	}
	if err = input.ValidateCurrent(freshNow); err != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, time.Time{}, err
	}
	if err = lease.ValidateCurrent(freshNow); err != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, time.Time{}, err
	}
	if err = current.Validate(); err != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, time.Time{}, err
	}
	if current.RefV2() != expected.RefV2() || !sameExecutionIdentityV2(current, expected) {
		return ToolOwnerSingleCallExecutionStateV2{}, time.Time{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Tool Owner execution current drifted at actual entry")
	}
	if freshNow.UnixNano() < current.UpdatedUnixNano {
		return ToolOwnerSingleCallExecutionStateV2{}, time.Time{}, core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "Tool Owner execution current clock regressed at actual entry")
	}
	if freshNow.UnixNano() >= current.ExpiresUnixNano {
		return ToolOwnerSingleCallExecutionStateV2{}, time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Tool Owner execution current expired at actual entry")
	}
	switch phase {
	case ToolOwnerEntryStartOrInspectV2:
		if current.State != ToolOwnerExecutionStartCommittedV2 {
			return ToolOwnerSingleCallExecutionStateV2{}, time.Time{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "Tool Owner StartOrInspect actual entry requires start_committed current")
		}
	case ToolOwnerEntryInspectV2:
		if current.State != ToolOwnerExecutionStartCommittedV2 && current.State != ToolOwnerExecutionInspectOnlyV2 {
			return ToolOwnerSingleCallExecutionStateV2{}, time.Time{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "Tool Owner Inspect actual entry requires recoverable current")
		}
	}
	return current, freshNow, nil
}

func (f *ToolOwnerSingleCallFlowImplV2) validateAndMarkDurableSettledV2(ctx context.Context, input ToolOwnerSingleCallExecutionV2, state ToolOwnerSingleCallExecutionStateV2, result toolcontract.ToolResultV2) (toolcontract.ToolResultV2, error) {
	if err := f.validateDurableResultV2(ctx, input, result); err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	now, err := f.nowAfterV2(time.Unix(0, state.UpdatedUnixNano))
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	resultRef := objectRefResultV1(result)
	_, err = f.states.AdvanceExecutionSettledV2(ctx, state.RefV2(), resultRef, now.UnixNano())
	if err != nil {
		key := mustToolOwnerInspectKeyV2(input)
		recoveryCtx, cancel := f.boundedRecoveryContextV2(ctx, now, minUnixNanoV2(inputExpiryUnixNanoV2(input), state.ExpiresUnixNano))
		defer cancel()
		current, inspectErr := f.states.InspectExecutionStateV2(recoveryCtx, key)
		if inspectErr == nil && current.State == ToolOwnerExecutionSettledV2 && current.Result != nil && *current.Result == resultRef {
			return cloneToolResultV2(result), nil
		}
		return toolcontract.ToolResultV2{}, err
	}
	return cloneToolResultV2(result), nil
}

func (f *ToolOwnerSingleCallFlowImplV2) validateDurableResultV2(ctx context.Context, input ToolOwnerSingleCallExecutionV2, result toolcontract.ToolResultV2) error {
	if err := validateToolOwnerResultV2(input, result); err != nil {
		return err
	}
	actionID := input.Binding.CandidateClosure.Candidate.ID
	exact, err := f.facts.InspectResultExactV2(ctx, actionID, objectRefResultV1(result))
	if err != nil {
		return err
	}
	settled, err := f.facts.InspectSettledResultForApplyV2(ctx, actionID, result.Apply)
	if err != nil {
		return err
	}
	legacy, err := f.settled.InspectSettledResultForApplyV2(actionID, result.Apply)
	if err != nil {
		return err
	}
	for _, persisted := range []toolcontract.ToolResultV2{exact, settled, legacy} {
		if err = validateToolOwnerResultV2(input, persisted); err != nil {
			return err
		}
		if !reflect.DeepEqual(persisted, result) {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "durable Tool Owner exact result readers returned different closure")
		}
	}
	return nil
}

func (f *ToolOwnerSingleCallFlowImplV2) readDurableSettledV2(ctx context.Context, input ToolOwnerSingleCallExecutionV2, state ToolOwnerSingleCallExecutionStateV2) (toolcontract.ToolResultV2, error) {
	if state.State != ToolOwnerExecutionSettledV2 || state.Result == nil {
		return toolcontract.ToolResultV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "durable Tool Owner marker is not settled")
	}
	result, err := f.facts.InspectResultExactV2(ctx, input.Binding.CandidateClosure.Candidate.ID, *state.Result)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if err = f.validateDurableResultV2(ctx, input, result); err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	return result, nil
}

func mustToolOwnerInspectKeyV2(input ToolOwnerSingleCallExecutionV2) applicationcontract.SingleCallToolActionInspectKeyV2 {
	key, _ := applicationcontract.SealSingleCallToolActionInspectKeyV2(input.Request)
	return key
}

func durableFlowErrorDigestV2(startErr, inspectErr error) core.Digest {
	startMessage, inspectMessage := "<nil>", "<nil>"
	if startErr != nil {
		startMessage = startErr.Error()
	}
	if inspectErr != nil {
		inspectMessage = inspectErr.Error()
	}
	return core.DigestBytes([]byte("start=" + startMessage + "\ninspect=" + inspectMessage))
}

func (f *ToolOwnerSingleCallFlowImplV2) inspectClaimedToolOwnerSingleCallV2(ctx context.Context, input ToolOwnerSingleCallExecutionV2) (toolcontract.ToolResultV2, error) {
	now, err := f.nowAfterV2(time.Time{})
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if err = input.ValidateCurrent(now); err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	recoveryCtx, cancel := f.boundedRecoveryContextV2(ctx, now, inputExpiryUnixNanoV2(input))
	defer cancel()
	result, err := f.execution.InspectBoundSingleCallToolActionV2(recoveryCtx, input)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if err = f.validatePersistedToolOwnerResultV2(input, result); err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	return cloneToolResultV2(result), nil
}

func toolOwnerFlowKeyV2(input ToolOwnerSingleCallExecutionV2) string {
	return input.Request.ID + "\x00" + string(input.Request.Digest) + "\x00" + input.Binding.Ref.ID + "\x00" + string(input.Binding.Ref.Digest)
}

func validateToolOwnerResultV2(input ToolOwnerSingleCallExecutionV2, result toolcontract.ToolResultV2) error {
	if err := result.Validate(); err != nil {
		return err
	}
	candidate := input.Binding.CandidateClosure.Candidate
	domain := result.Inspection.DomainResult
	domainObject := toolcontract.ObjectRef{ID: domain.ID, Revision: domain.Revision, Digest: domain.Digest}
	owner := candidate.ExpectedOwner
	if result.Action != candidate.ObjectRef() || result.DomainResult != domainObject || !runtimeports.SameExecutionScopeV2(domain.Operation.ExecutionScope, input.Request.Action.ExecutionScope) || domain.Operation.ExecutionScopeDigest != candidate.OperationScopeDigest || domain.TenantID != candidate.TenantID || domain.Owner.ComponentID != owner.ComponentID || domain.Owner.ManifestDigest != owner.ManifestDigest || domain.Owner.Capability != runtimeports.CapabilityNameV2(candidate.EffectKind) || result.Inspection.Owner != owner || result.Schema != domain.Schema || result.PayloadDigest != domain.PayloadDigest || result.PayloadRevision != domain.PayloadRevision {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Tool Owner V2 result differs from CandidateV3 or Runtime settlement")
	}
	expectedApplyID, err := toolcontract.StableID("tool-apply-v2", candidate.ID, domain.ID, string(result.Inspection.Digest))
	if err != nil || result.Apply.ID != expectedApplyID {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Tool Owner V2 ApplySettlement does not close the exact DomainResult and settlement")
	}
	expectedResultID, err := toolcontract.StableID("tool-result-v2", candidate.ID, domain.ID, result.Apply.ID, string(result.Apply.Digest))
	if err != nil || result.ID != expectedResultID {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Tool Owner V2 ToolResult stable identity drifted")
	}
	return nil
}

func (f *ToolOwnerSingleCallFlowImplV2) validatePersistedToolOwnerResultV2(input ToolOwnerSingleCallExecutionV2, result toolcontract.ToolResultV2) error {
	if err := validateToolOwnerResultV2(input, result); err != nil {
		return err
	}
	if isNilFlowDependencyV1(f.settled) {
		// The source-compatible constructor is fixture-only. Production/restart
		// safe composition uses NewToolOwnerSingleCallFlowWithStoresV2.
		return nil
	}
	persisted, err := f.settled.InspectSettledResultForApplyV2(input.Binding.CandidateClosure.Candidate.ID, result.Apply)
	if err != nil {
		return err
	}
	if err = validateToolOwnerResultV2(input, persisted); err != nil {
		return err
	}
	left, leftErr := core.CanonicalJSONDigest("praxis.tool-mcp.single-call-owner-result", "2.0.0", "ToolResultV2", result)
	right, rightErr := core.CanonicalJSONDigest("praxis.tool-mcp.single-call-owner-result", "2.0.0", "ToolResultV2", persisted)
	if leftErr != nil || rightErr != nil || left != right {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Tool Owner V2 settled-result reader returned another closure")
	}
	return nil
}

func cloneToolResultV2(value toolcontract.ToolResultV2) toolcontract.ToolResultV2 {
	// JSON cloning preserves the exact canonical shape while breaking nested
	// SandboxLease and slice aliases. ToolResult contains no opaque handles.
	payload, marshalErr := json.Marshal(value)
	if marshalErr != nil {
		return toolcontract.ToolResultV2{}
	}
	var out toolcontract.ToolResultV2
	if json.Unmarshal(payload, &out) != nil {
		return toolcontract.ToolResultV2{}
	}
	return out
}

func (f *ToolOwnerSingleCallFlowImplV2) nowAfterV2(previous time.Time) (time.Time, error) {
	if f == nil || isNilFlowDependencyV1(f.clock) {
		return time.Time{}, core.NewError(core.ErrorUnavailable, core.ReasonClockRegression, "Tool Owner V2 clock is unavailable")
	}
	f.clockMu.Lock()
	defer f.clockMu.Unlock()
	now := f.clock.Now()
	floor := previous
	if floor.Before(f.lastNow) {
		floor = f.lastNow
	}
	if now.IsZero() || now.Before(floor) {
		return time.Time{}, core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "Tool Owner V2 clock regressed")
	}
	f.lastNow = now
	return now, nil
}

func (f *ToolOwnerSingleCallFlowImplV2) boundedRecoveryContextV2(ctx context.Context, now time.Time, expiresUnixNano int64) (context.Context, context.CancelFunc) {
	if ctx.Err() == nil {
		// A live transport deadline is the caller's authority. Detaching it here
		// would let exact recovery outlive cancellation. Logical Owner expiry is
		// still re-read and enforced before every external entry.
		return ctx, func() {}
	}
	timeout := f.recoveryTimeout
	if timeout <= 0 {
		timeout = defaultToolOwnerRecoveryTimeoutV2
	}
	if expiresUnixNano > 0 {
		remaining := time.Duration(expiresUnixNano - now.UnixNano())
		if remaining <= 0 {
			remaining = time.Nanosecond
		}
		if remaining < timeout {
			timeout = remaining
		}
	}
	return context.WithTimeout(context.WithoutCancel(ctx), timeout)
}

// boundedEntryWaitContextV2 preserves a live caller's context while the loop
// enforces the exact Tool and lease lifetime through the injected owner clock.
// Converting that logical TTL to a wall-clock timeout would make a slow race
// build expire even though no owner current had expired. Only callers whose
// transport context is already cancelled enter the shorter, strictly bounded
// WithoutCancel recovery window.
func (f *ToolOwnerSingleCallFlowImplV2) boundedEntryWaitContextV2(ctx context.Context, now time.Time, expiresUnixNano int64) (context.Context, context.CancelFunc) {
	remaining := time.Duration(expiresUnixNano - now.UnixNano())
	if remaining <= 0 {
		remaining = time.Nanosecond
	}
	if ctx.Err() == nil {
		return ctx, func() {}
	}
	timeout := f.recoveryTimeout
	if timeout <= 0 {
		timeout = defaultToolOwnerRecoveryTimeoutV2
	}
	if remaining < timeout {
		timeout = remaining
	}
	return context.WithTimeout(context.WithoutCancel(ctx), timeout)
}

func boundedOwnerLocalRecoveryContextV2(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = defaultToolOwnerRecoveryTimeoutV2
	}
	return context.WithTimeout(context.WithoutCancel(ctx), timeout)
}

func inputExpiryUnixNanoV2(input ToolOwnerSingleCallExecutionV2) int64 {
	return minUnixNanoV2(input.Request.ExpiresUnixNano, input.Binding.ExpiresUnixNano)
}

func minUnixNanoV2(values ...int64) int64 {
	var minimum int64
	for _, value := range values {
		if value > 0 && (minimum == 0 || value < minimum) {
			minimum = value
		}
	}
	return minimum
}

var _ ToolOwnerSingleCallFlowV2 = (*ToolOwnerSingleCallFlowImplV2)(nil)

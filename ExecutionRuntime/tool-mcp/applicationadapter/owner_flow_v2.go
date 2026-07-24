package applicationadapter

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"sync"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
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
	execution ToolOwnerSingleCallExecutionPortV2
	settled   ToolOwnerSettledResultReaderV2
	claims    ToolOwnerSingleCallClaimStoreV2
	clock     ClockV1
	clockMu   sync.Mutex
	lastNow   time.Time
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
		winner, err = f.claims.InspectToolOwnerSingleCallClaimV2(context.WithoutCancel(ctx), key)
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
		result, executeErr = f.execution.InspectBoundSingleCallToolActionV2(context.WithoutCancel(ctx), input)
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
	result, err := f.inspectClaimedToolOwnerSingleCallV2(ctx, record.Input)
	return record.Input, result, err
}

func (f *ToolOwnerSingleCallFlowImplV2) inspectClaimedToolOwnerSingleCallV2(ctx context.Context, input ToolOwnerSingleCallExecutionV2) (toolcontract.ToolResultV2, error) {
	result, err := f.execution.InspectBoundSingleCallToolActionV2(ctx, input)
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

var _ ToolOwnerSingleCallFlowV2 = (*ToolOwnerSingleCallFlowImplV2)(nil)

package applicationadapter

import (
	"context"
	"reflect"
	"sync"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/action"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type ClockV1 interface{ Now() time.Time }

type SingleCallToolExactBindingsV1 struct {
	ActionID        string                               `json:"action_id"`
	Candidate       toolcontract.ActionCandidateV2       `json:"candidate"`
	PendingAction   toolcontract.PendingActionExactRefV2 `json:"pending_action"`
	Capability      toolcontract.ObjectRef               `json:"capability"`
	Tool            toolcontract.ObjectRef               `json:"tool"`
	SourceCandidate toolcontract.ObjectRef               `json:"source_candidate"`
	Provider        runtimeports.ProviderBindingRefV2    `json:"provider"`
}

func (b SingleCallToolExactBindingsV1) ValidateFor(request applicationcontract.SingleCallToolActionRequestV1) error {
	if toolcontract.ValidateStableID(b.ActionID) != nil || b.Candidate.Validate() != nil || b.PendingAction.Validate() != nil || b.Capability.Validate() != nil || b.Tool.Validate() != nil || b.SourceCandidate.Validate() != nil || b.Provider.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool exact bindings are incomplete")
	}
	if b.Candidate.ID != b.ActionID || b.Candidate.TenantID != request.ExecutionScope.Identity.TenantID || b.Candidate.RunID != string(request.Run.RunID) || b.Candidate.SessionID != request.Session.ID || b.Candidate.TurnID != request.Turn.ID || b.Candidate.PendingAction != b.PendingAction || b.Candidate.Capability != b.Capability || b.Candidate.Tool != b.Tool || b.Candidate.SourceCandidate != b.SourceCandidate || b.Candidate.InputSchema != request.PendingAction.PayloadSchema || b.Candidate.Payload.Schema != request.PendingAction.PayloadSchema || b.Candidate.Payload.ContentDigest != request.PendingAction.PayloadDigest || b.Candidate.OperationScopeDigest != request.ExecutionScopeDigest || b.Candidate.EffectKind != "praxis.tool/execute" || b.PendingAction.ID != request.PendingAction.ActionRef || b.PendingAction.RequestDigest != request.PendingAction.RequestDigest || b.Capability.ID != string(request.PendingAction.Capability) || b.SourceCandidate.ID != request.PendingAction.SourceCandidateID || b.SourceCandidate.Revision != request.PendingAction.SourceCandidateRevision || b.SourceCandidate.Digest != request.PendingAction.SourceCandidateDigest || !reflect.DeepEqual(b.Provider, request.Assembly.ToolProvider) || b.Candidate.ExpectedOwner.ComponentID != b.Provider.ComponentID || b.Candidate.ExpectedOwner.ManifestDigest != b.Provider.ManifestDigest {
		return core.NewError(core.ErrorConflict, core.ReasonComponentMismatch, "Tool exact bindings drifted from Application coordinates")
	}
	return nil
}

type SingleCallToolExactBindingReaderV1 interface {
	InspectSingleCallToolExactBindingsV1(context.Context, applicationcontract.SingleCallToolActionRequestV1) (SingleCallToolExactBindingsV1, error)
}

type ToolOwnerSingleCallExecutionV1 struct {
	Watermark              toolcontract.ToolProviderBoundarySourceRefV1 `json:"watermark"`
	ActionCoordinateDigest core.Digest                                  `json:"action_coordinate_digest"`
	RequestID              string                                       `json:"request_id"`
	RequestRevision        core.Revision                                `json:"request_revision"`
	RequestDigest          core.Digest                                  `json:"request_digest"`
	ScopeDigest            core.Digest                                  `json:"scope_digest"`
}

func (r ToolOwnerSingleCallExecutionV1) Validate() error {
	if r.Watermark.Validate() != nil || r.ActionCoordinateDigest.Validate() != nil || r.RequestID == "" || r.RequestRevision == 0 || r.RequestDigest.Validate() != nil || r.ScopeDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool single-call execution coordinate is incomplete")
	}
	return nil
}

type ToolOwnerSingleCallFlowV1 interface {
	StartOrInspectToolOwnerSingleCallV1(context.Context, ToolOwnerSingleCallExecutionV1) (toolcontract.ToolResultV2, error)
}

type ApplicationResultStoreV1 interface {
	CreateSingleCallApplicationResultV1(context.Context, applicationcontract.SingleCallToolActionRequestV1, applicationcontract.SingleCallToolActionResultV1) (applicationcontract.SingleCallToolActionResultV1, error)
	InspectSingleCallApplicationResultV1(context.Context, applicationports.InspectSingleCallToolActionRequestV1) (applicationcontract.SingleCallToolActionResultV1, error)
}

type SingleCallToolActionAdapterV1 struct {
	model        modelinvoker.ToolCallCandidateObservationProjectionReaderV1
	bindings     SingleCallToolExactBindingReaderV1
	coordination *action.CoordinationStoreV1
	flow         ToolOwnerSingleCallFlowV1
	settlement   applicationports.SingleCallOperationSettlementCurrentReaderV1
	results      ApplicationResultStoreV1
	clock        ClockV1
	gates        keyedGateV1
}

func NewSingleCallToolActionAdapterV1(model modelinvoker.ToolCallCandidateObservationProjectionReaderV1, bindings SingleCallToolExactBindingReaderV1, coordination *action.CoordinationStoreV1, flow ToolOwnerSingleCallFlowV1, settlement applicationports.SingleCallOperationSettlementCurrentReaderV1, results ApplicationResultStoreV1, clock ClockV1) *SingleCallToolActionAdapterV1 {
	return &SingleCallToolActionAdapterV1{model: model, bindings: bindings, coordination: coordination, flow: flow, settlement: settlement, results: results, clock: clock, gates: keyedGateV1{entries: make(map[string]*keyedGateEntryV1)}}
}

func (a *SingleCallToolActionAdapterV1) ExecuteSingleCallToolActionV1(ctx context.Context, request applicationcontract.SingleCallToolActionRequestV1) (applicationcontract.SingleCallToolActionResultV1, error) {
	now, err := a.nowAfter(time.Time{})
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, err
	}
	if err = request.ValidateCurrent(now); err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, err
	}
	projection, err := a.inspectModelExact(ctx, request)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, err
	}
	if a.bindings == nil || a.coordination == nil || a.flow == nil || a.settlement == nil || a.results == nil {
		return applicationcontract.SingleCallToolActionResultV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Tool application adapter dependencies are unavailable")
	}
	bindings, err := a.bindings.InspectSingleCallToolExactBindingsV1(ctx, request)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, err
	}
	if err = bindings.ValidateFor(request); err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, err
	}
	command, err := canonicalCommandV1(request, projection, bindings)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, err
	}
	commandDigest, err := command.DigestV1()
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, err
	}
	now, err = a.nowAfter(now)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, err
	}
	if err = request.ValidateCurrent(now); err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, err
	}
	watermark, err := a.coordination.StartOrInspectV1(ctx, command, now, time.Unix(0, minimumUnixNano(request.ExpiresUnixNano, request.Session.ExpiresUnixNano, request.ParentFrame.ExpiresUnixNano)))
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, err
	}
	gateKey := request.ID + "\x00" + string(request.Digest) + "\x00" + string(request.ExecutionScopeDigest) + "\x00" + string(commandDigest)
	release := a.gates.acquire(gateKey)
	defer release()
	key := applicationports.InspectSingleCallToolActionRequestV1{RequestID: request.ID, RequestDigest: request.Digest, ScopeDigest: request.ExecutionScopeDigest}
	existing, inspectErr := a.results.InspectSingleCallApplicationResultV1(ctx, key)
	if inspectErr == nil {
		now, err = a.nowAfter(now)
		if err != nil {
			return applicationcontract.SingleCallToolActionResultV1{}, err
		}
		return existing, existing.ValidateCurrentFor(request, now)
	}
	if !core.HasCategory(inspectErr, core.ErrorNotFound) {
		return applicationcontract.SingleCallToolActionResultV1{}, inspectErr
	}
	now, err = a.nowAfter(now)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, err
	}
	if err = request.ValidateCurrent(now); err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, err
	}
	actionDigest, err := applicationcontract.SingleCallActionCoordinateDigestV1(request)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, err
	}
	flowRequest := ToolOwnerSingleCallExecutionV1{Watermark: toolcontract.ToolProviderBoundarySourceRefV1{WatermarkID: watermark.ID, WatermarkRevision: watermark.Revision, WatermarkDigest: watermark.Digest}, ActionCoordinateDigest: actionDigest, RequestID: request.ID, RequestRevision: request.Revision, RequestDigest: request.Digest, ScopeDigest: request.ExecutionScopeDigest}
	if err = flowRequest.Validate(); err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, err
	}
	toolResult, err := a.flow.StartOrInspectToolOwnerSingleCallV1(ctx, flowRequest)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, err
	}
	result, closedAt, err := a.closeApplicationResult(ctx, request, toolResult, now)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, err
	}
	stored, err := a.results.CreateSingleCallApplicationResultV1(ctx, request, result)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, err
	}
	returnNow, err := a.nowAfter(closedAt)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, err
	}
	return stored, stored.ValidateCurrentFor(request, returnNow)
}

func (a *SingleCallToolActionAdapterV1) InspectSingleCallToolActionV1(ctx context.Context, key applicationports.InspectSingleCallToolActionRequestV1) (applicationcontract.SingleCallToolActionResultV1, error) {
	if err := key.Validate(); err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, err
	}
	if a == nil || a.results == nil {
		return applicationcontract.SingleCallToolActionResultV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Tool Application result reader is unavailable")
	}
	return a.results.InspectSingleCallApplicationResultV1(ctx, key)
}

func (a *SingleCallToolActionAdapterV1) inspectModelExact(ctx context.Context, request applicationcontract.SingleCallToolActionRequestV1) (modelinvoker.ToolCallCandidateObservationProjectionV1, error) {
	if a == nil || a.model == nil {
		return modelinvoker.ToolCallCandidateObservationProjectionV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "Model exact Projection reader is unavailable")
	}
	coordinate := request.Observation
	ref := modelinvoker.ToolCallCandidateObservationRefV1{ID: coordinate.ProjectionID, Revision: coordinate.ProjectionRevision, Digest: coordinate.ProjectionDigest, InvocationID: coordinate.InvocationID, InvocationDigest: coordinate.InvocationDigest, ObservationDigest: coordinate.ObservationDigest, Source: modelinvoker.ToolCallCandidateObservationSourceCoordinateV1{SourceSequence: coordinate.SourceSequence, ResponseID: coordinate.SourceResponseID}}
	if coordinate.ProjectionContractVersion != modelinvoker.ToolCallCandidateObservationProjectionContractVersionV1 {
		return modelinvoker.ToolCallCandidateObservationProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonComponentMismatch, "Model Projection contract version drifted")
	}
	projection, err := a.model.InspectExactProjectionV1(ctx, ref)
	if err != nil {
		return modelinvoker.ToolCallCandidateObservationProjectionV1{}, err
	}
	if err = projection.Validate(); err != nil {
		return modelinvoker.ToolCallCandidateObservationProjectionV1{}, err
	}
	if !reflect.DeepEqual(projection.Ref, ref) || projection.Observation.Digest != coordinate.ObservationDigest || len(projection.Observation.Calls) != 1 || coordinate.CallCount != 1 {
		return modelinvoker.ToolCallCandidateObservationProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Model exact Projection or N=1 cardinality drifted")
	}
	return projection, nil
}

func (a *SingleCallToolActionAdapterV1) closeApplicationResult(ctx context.Context, request applicationcontract.SingleCallToolActionRequestV1, toolResult toolcontract.ToolResultV2, previous time.Time) (applicationcontract.SingleCallToolActionResultV1, time.Time, error) {
	if err := toolResult.Validate(); err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, time.Time{}, err
	}
	beforeSettlement, err := a.nowAfter(previous)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, time.Time{}, err
	}
	if err = request.ValidateCurrent(beforeSettlement); err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, time.Time{}, err
	}
	inspection, err := a.settlement.InspectCurrentOperationSettlementV4(ctx, runtimeports.InspectCurrentOperationSettlementRequestV4{Operation: toolResult.Inspection.DomainResult.Operation, EffectID: toolResult.Inspection.Settlement.EffectID})
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, time.Time{}, err
	}
	afterSettlement, err := a.nowAfter(beforeSettlement)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, time.Time{}, err
	}
	if err = request.ValidateCurrent(afterSettlement); err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, time.Time{}, err
	}
	if err = inspection.Validate(afterSettlement); err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, time.Time{}, err
	}
	if !reflect.DeepEqual(inspection, toolResult.Inspection) {
		return applicationcontract.SingleCallToolActionResultV1{}, time.Time{}, core.NewError(core.ErrorConflict, core.ReasonEffectSettlementMissing, "Runtime current V4 Inspection drifted from settled ToolResult")
	}
	association, err := a.settlement.InspectOperationSettlementEvidenceAssociationV4(ctx, inspection.DomainResult.Operation, inspection.Association)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, time.Time{}, err
	}
	afterAssociation, err := a.nowAfter(afterSettlement)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, time.Time{}, err
	}
	if err = request.ValidateCurrent(afterAssociation); err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, time.Time{}, err
	}
	if err = inspection.Validate(afterAssociation); err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, time.Time{}, err
	}
	if err = association.Validate(); err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, time.Time{}, err
	}
	if !runtimeports.SameOperationSettlementEvidenceAssociationRefV4(association.RefV4(), inspection.Association) {
		return applicationcontract.SingleCallToolActionResultV1{}, time.Time{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "public Association Inspect drifted")
	}
	actionDigest, err := applicationcontract.SingleCallActionCoordinateDigestV1(request)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, time.Time{}, err
	}
	expires := minimumUnixNano(request.ExpiresUnixNano, inspection.ExpiresUnixNano)
	coordinate := applicationcontract.SingleCallToolResultCoordinateV1{ID: toolResult.ID, Revision: toolResult.Revision, Digest: toolResult.Digest, ActionCoordinateDigest: actionDigest, ApplySettlementID: toolResult.Apply.ID, ApplySettlementRevision: toolResult.Apply.Revision, ApplySettlementDigest: toolResult.Apply.Digest, Settlement: inspection.Settlement, ResultSchema: toolResult.Schema, PayloadDigest: toolResult.PayloadDigest, FinalizedUnixNano: toolResult.FinalizedUnixNano, ExpiresUnixNano: expires}
	result, err := applicationcontract.SealSingleCallToolActionResultV1(applicationcontract.SingleCallToolActionResultV1{ToolResult: coordinate, Inspection: inspection, Association: inspection.Association, AssociationCheckedUnixNano: afterAssociation.UnixNano(), ExpiresUnixNano: expires}, request, afterAssociation)
	return result, afterAssociation, err
}

func (a *SingleCallToolActionAdapterV1) nowAfter(previous time.Time) (time.Time, error) {
	if a == nil || a.clock == nil {
		return time.Time{}, core.NewError(core.ErrorUnavailable, core.ReasonClockRegression, "Tool Application adapter clock is unavailable")
	}
	now := a.clock.Now()
	if now.IsZero() || !previous.IsZero() && now.Before(previous) {
		return time.Time{}, core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "Tool Application adapter clock is indeterminate or regressed")
	}
	return now, nil
}

func canonicalCommandV1(request applicationcontract.SingleCallToolActionRequestV1, projection modelinvoker.ToolCallCandidateObservationProjectionV1, bindings SingleCallToolExactBindingsV1) (toolcontract.SingleCallCanonicalCommandV1, error) {
	call := projection.Observation.Calls[0]
	actionDigest, err := applicationcontract.SingleCallActionCoordinateDigestV1(request)
	if err != nil {
		return toolcontract.SingleCallCanonicalCommandV1{}, err
	}
	command := toolcontract.SingleCallCanonicalCommandV1{TenantID: request.ExecutionScope.Identity.TenantID, ApplicationRequestID: request.ID, ApplicationRequestRevision: request.Revision, ApplicationRequestDigest: request.Digest, ActionCoordinateDigest: actionDigest, OperationScopeDigest: request.ExecutionScopeDigest, ModelProjection: projection.Ref, ObservationDigest: projection.Observation.Digest, CallID: call.CallID, CallName: call.Name, CanonicalArgumentsDigest: core.DigestBytes(call.CanonicalArguments), PendingAction: bindings.PendingAction, RunID: string(request.Run.RunID), SessionID: request.Session.ID, TurnID: request.Turn.ID, ActionID: bindings.ActionID, ActionCandidate: objectRefCandidateV1(bindings.Candidate), Capability: bindings.Capability, Tool: bindings.Tool, InputSchema: request.PendingAction.PayloadSchema, SourceCandidate: bindings.SourceCandidate, PayloadDigest: request.PendingAction.PayloadDigest, Provider: bindings.Provider, EffectKind: "praxis.tool/execute", PolicyProfile: "praxis.tool/single-call-action-v1"}
	if err := command.Validate(); err != nil {
		return toolcontract.SingleCallCanonicalCommandV1{}, err
	}
	return command, nil
}
func minimumUnixNano(values ...int64) int64 {
	minimum := int64(0)
	for _, value := range values {
		if value > 0 && (minimum == 0 || value < minimum) {
			minimum = value
		}
	}
	return minimum
}

var _ applicationports.SingleCallToolActionPortV1 = (*SingleCallToolActionAdapterV1)(nil)

type keyedGateEntryV1 struct {
	mu   sync.Mutex
	refs int
}

type keyedGateV1 struct {
	mu      sync.Mutex
	entries map[string]*keyedGateEntryV1
}

func (g *keyedGateV1) acquire(key string) func() {
	g.mu.Lock()
	if g.entries == nil {
		g.entries = make(map[string]*keyedGateEntryV1)
	}
	entry := g.entries[key]
	if entry == nil {
		entry = &keyedGateEntryV1{}
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

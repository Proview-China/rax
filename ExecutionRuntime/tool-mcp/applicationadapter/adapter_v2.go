package applicationadapter

import (
	"context"
	"reflect"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type SingleCallToolActionAdapterV2 struct {
	bindings   SingleCallToolActionBindingCurrentReaderV2
	flow       ToolOwnerSingleCallFlowV2
	settlement applicationports.SingleCallOperationSettlementCurrentReaderV2
	results    ApplicationResultStoreV2
	clock      ClockV1
}

func NewSingleCallToolActionAdapterV2(bindings SingleCallToolActionBindingCurrentReaderV2, flow ToolOwnerSingleCallFlowV2, settlement applicationports.SingleCallOperationSettlementCurrentReaderV2, results ApplicationResultStoreV2, clock ClockV1) (*SingleCallToolActionAdapterV2, error) {
	for _, dependency := range []any{bindings, flow, settlement, results, clock} {
		if isNilFlowDependencyV1(dependency) {
			return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Tool Application V2 adapter dependencies are incomplete")
		}
	}
	return &SingleCallToolActionAdapterV2{bindings: bindings, flow: flow, settlement: settlement, results: results, clock: clock}, nil
}

func (a *SingleCallToolActionAdapterV2) ExecuteSingleCallToolActionV2(ctx context.Context, request applicationcontract.SingleCallToolActionRequestV2) (applicationcontract.SingleCallToolActionResultV2, error) {
	if a == nil || isNilFlowDependencyV1(ctx) {
		return applicationcontract.SingleCallToolActionResultV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool Application V2 adapter input is invalid")
	}
	now, err := a.nowAfterV2(time.Time{})
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	if err := request.ValidateCurrent(now); err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	key, err := applicationcontract.SealSingleCallToolActionInspectKeyV2(request)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	if existing, inspectErr := a.results.InspectSingleCallApplicationResultRecordV2(ctx, key); inspectErr == nil {
		if err = existing.ValidateForKey(key); err != nil {
			return applicationcontract.SingleCallToolActionResultV2{}, err
		}
		return existing.Result, existing.Result.ValidateCurrentFor(request, now)
	} else if !core.HasCategory(inspectErr, core.ErrorNotFound) {
		return applicationcontract.SingleCallToolActionResultV2{}, inspectErr
	}
	resolve := SingleCallToolActionBindingResolveRequestV2{ApplicationRequest: request, SourceSubject: request.Action.PendingSubject, RequestedExpiresUnixNano: request.ExpiresUnixNano}
	binding, err := a.bindings.ResolveSingleCallToolActionBindingCurrentV2(ctx, resolve)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	now, err = a.nowAfterV2(now)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	input := ToolOwnerSingleCallExecutionV2{Request: request, Binding: binding}
	if err = input.ValidateCurrent(now); err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	toolResult, err := a.flow.StartOrInspectToolOwnerSingleCallV2(ctx, input)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	result, err := a.closeApplicationResultV2(ctx, request, binding, toolResult, now)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	stored, createErr := a.results.CreateSingleCallApplicationResultV2(ctx, request, result)
	if createErr != nil {
		recovered, inspectErr := a.results.InspectSingleCallApplicationResultRecordV2(context.WithoutCancel(ctx), key)
		if inspectErr != nil {
			return applicationcontract.SingleCallToolActionResultV2{}, inspectErr
		}
		if err = recovered.ValidateForKey(key); err != nil {
			return applicationcontract.SingleCallToolActionResultV2{}, err
		}
		stored = recovered.Result
	}
	now, err = a.nowAfterV2(time.Unix(0, result.Coordinate.AssociationCheckedUnixNano))
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	if err = stored.ValidateCurrentFor(request, now); err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	if stored.Digest != result.Digest {
		return applicationcontract.SingleCallToolActionResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Tool Application V2 result recovery found different content")
	}
	return stored, nil
}

func (a *SingleCallToolActionAdapterV2) InspectSingleCallToolActionV2(ctx context.Context, key applicationcontract.SingleCallToolActionInspectKeyV2) (applicationcontract.SingleCallToolActionResultV2, error) {
	if a == nil || isNilFlowDependencyV1(ctx) {
		return applicationcontract.SingleCallToolActionResultV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool Application V2 inspect input is invalid")
	}
	if err := key.Validate(); err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	record, err := a.results.InspectSingleCallApplicationResultRecordV2(ctx, key)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	if err = record.ValidateForKey(key); err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	return record.Result, nil
}

func (a *SingleCallToolActionAdapterV2) closeApplicationResultV2(ctx context.Context, request applicationcontract.SingleCallToolActionRequestV2, binding SingleCallToolActionBindingCurrentProjectionV2, toolResult toolcontract.ToolResultV2, previous time.Time) (applicationcontract.SingleCallToolActionResultV2, error) {
	if err := validateToolOwnerResultV2(ToolOwnerSingleCallExecutionV2{Request: request, Binding: binding}, toolResult); err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	now, err := a.nowAfterV2(previous)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	inspection, err := a.settlement.InspectCurrentOperationSettlementV4(ctx, runtimeports.InspectCurrentOperationSettlementRequestV4{Operation: toolResult.Inspection.DomainResult.Operation, EffectID: toolResult.Inspection.Settlement.EffectID})
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	now, err = a.nowAfterV2(now)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	if err = inspection.Validate(now); err != nil || !reflect.DeepEqual(inspection, toolResult.Inspection) {
		return applicationcontract.SingleCallToolActionResultV2{}, core.NewError(core.ErrorConflict, core.ReasonEffectSettlementMissing, "Tool Application V2 current settlement drifted")
	}
	association, err := a.settlement.InspectOperationSettlementEvidenceAssociationV4(ctx, inspection.DomainResult.Operation, inspection.Association)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	now, err = a.nowAfterV2(now)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	if err = request.ValidateCurrent(now); err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	if err = association.Validate(); err != nil || !runtimeports.SameOperationSettlementEvidenceAssociationRefV4(association.RefV4(), inspection.Association) || !runtimeports.SameOperationSettlementRefV4(association.Settlement, inspection.Settlement) {
		return applicationcontract.SingleCallToolActionResultV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Tool Application V2 Association drifted")
	}
	candidate := binding.CandidateClosure.Candidate
	ownerResult := applicationcontract.SingleCallToolOwnerResultRefCoordinateV2{OwnerContractVersion: applicationcontract.SingleCallToolOwnerResultContractVersionV2, ID: toolResult.ID, Revision: toolResult.Revision, Digest: toolResult.Digest, ActionID: request.Action.PendingSubject.PendingActionRef, ActionRevision: candidate.PendingAction.Revision, ActionDigest: request.Action.Digest, ApplyID: toolResult.Apply.ID, ApplyRevision: toolResult.Apply.Revision, ApplyDigest: toolResult.Apply.Digest, Inspection: inspection, Schema: toolResult.Schema, PayloadDigest: toolResult.PayloadDigest, PayloadRevision: toolResult.PayloadRevision, FinalizedUnixNano: toolResult.FinalizedUnixNano}
	expires := request.ExpiresUnixNano
	if inspection.ExpiresUnixNano < expires {
		expires = inspection.ExpiresUnixNano
	}
	coordinate, err := applicationcontract.SealSingleCallToolActionResultCoordinateV2(applicationcontract.SingleCallToolActionResultCoordinateV2{ToolResult: ownerResult, Inspection: inspection, Association: association.RefV4(), AssociationCheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires}, request, now)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	return applicationcontract.SealSingleCallToolActionResultV2(applicationcontract.SingleCallToolActionResultV2{Coordinate: coordinate}, request, now)
}

func (a *SingleCallToolActionAdapterV2) nowAfterV2(previous time.Time) (time.Time, error) {
	if a == nil || isNilFlowDependencyV1(a.clock) {
		return time.Time{}, core.NewError(core.ErrorUnavailable, core.ReasonClockRegression, "Tool Application V2 clock is unavailable")
	}
	now := a.clock.Now()
	if now.IsZero() || !previous.IsZero() && now.Before(previous) {
		return time.Time{}, core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "Tool Application V2 clock is indeterminate or regressed")
	}
	return now, nil
}

var _ applicationports.SingleCallToolActionPortV2 = (*SingleCallToolActionAdapterV2)(nil)

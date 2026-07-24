package runtimeadapter

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/applicationadapter"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type MCPFormalProviderObservationReaderV1 interface {
	InspectOperationProviderReceiptObservationV1(context.Context, runtimeports.ExecutionDelegationRefV2, string) (runtimeports.ProviderAttemptObservationRefV2, error)
}

// MCPProviderObservationReaderV1 independently joins Runtime's formal
// Observation with Tool-owned immutable command and receipt facts. It never
// calls a Provider and does not create DomainResult or Settlement facts.
type MCPProviderObservationReaderV1 struct {
	commands     toolcontract.MCPExecutionCommandAttemptReaderV1
	receipts     toolcontract.MCPProtocolReceiptIDReaderV1
	observations MCPFormalProviderObservationReaderV1
}

func NewMCPProviderObservationReaderV1(commands toolcontract.MCPExecutionCommandAttemptReaderV1, receipts toolcontract.MCPProtocolReceiptIDReaderV1, observations MCPFormalProviderObservationReaderV1) (*MCPProviderObservationReaderV1, error) {
	if nilLikeMCPProviderObservationDependencyV1(commands) || nilLikeMCPProviderObservationDependencyV1(receipts) || nilLikeMCPProviderObservationDependencyV1(observations) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Provider Observation reader dependencies are incomplete")
	}
	return &MCPProviderObservationReaderV1{commands: commands, receipts: receipts, observations: observations}, nil
}

func (r *MCPProviderObservationReaderV1) InspectSingleCallToolProviderObservationV1(ctx context.Context, attempt runtimeports.OperationDispatchAttemptRefV3) (applicationadapter.SingleCallToolProviderInspectionV1, error) {
	if err := mcpProviderObservationContextV1(ctx); err != nil {
		return applicationadapter.SingleCallToolProviderInspectionV1{}, err
	}
	if r == nil || nilLikeMCPProviderObservationDependencyV1(r.commands) || nilLikeMCPProviderObservationDependencyV1(r.receipts) || nilLikeMCPProviderObservationDependencyV1(r.observations) {
		return applicationadapter.SingleCallToolProviderInspectionV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Provider Observation reader is unavailable")
	}
	if attempt.Validate() != nil || attempt.Delegation == nil {
		return applicationadapter.SingleCallToolProviderInspectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Provider Observation Attempt is incomplete")
	}
	command, err := r.commands.InspectMCPExecutionCommandByAttemptV1(ctx, attempt)
	if err != nil {
		return applicationadapter.SingleCallToolProviderInspectionV1{}, err
	}
	if command.Validate() != nil || command.Attempt != attempt || command.Prepared.AttemptID != attempt.AttemptID || command.Prepared.DeclaredDelegation.ID != attempt.Delegation.ID {
		return applicationadapter.SingleCallToolProviderInspectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP command belongs to another governed Attempt")
	}
	observation, err := r.observations.InspectOperationProviderReceiptObservationV1(ctx, *attempt.Delegation, command.Prepared.ID)
	if err != nil {
		return applicationadapter.SingleCallToolProviderInspectionV1{}, err
	}
	if observation.Validate() != nil || observation.Delegation != *attempt.Delegation || observation.PreparedAttemptID != command.Prepared.ID || observation.State != runtimeports.ProviderAttemptObservedV2 {
		return applicationadapter.SingleCallToolProviderInspectionV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Runtime MCP Provider Observation exact coordinates drifted")
	}
	receipt, err := r.receipts.InspectMCPProtocolReceiptByIDV1(ctx, observation.ProviderOperationRef)
	if err != nil {
		return applicationadapter.SingleCallToolProviderInspectionV1{}, err
	}
	if receipt.Validate() != nil || receipt.Command != command.Ref || receipt.Ref.ID != observation.ProviderOperationRef || receipt.ResponseDigest != observation.PayloadDigest || observation.PayloadRevision != 1 || receipt.ObservedUnixNano != observation.ObservedUnixNano {
		return applicationadapter.SingleCallToolProviderInspectionV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP Protocol Receipt differs from the formal Runtime Observation")
	}
	payload, err := mcpProtocolReceiptOpaquePayloadV1(receipt)
	if err != nil {
		return applicationadapter.SingleCallToolProviderInspectionV1{}, err
	}
	outcome, disposition := toolcontract.ToolOutcomeSucceededV2, toolcontract.ToolDispositionConfirmedAppliedV2
	if receipt.ToolError {
		outcome = toolcontract.ToolOutcomeFailedV2
	}
	return applicationadapter.SingleCallToolProviderInspectionV1{
		Operation: command.Operation, Attempt: command.Attempt, Prepared: command.Prepared,
		Observation: observation, Schema: payload.Schema, PayloadDigest: observation.PayloadDigest,
		PayloadRevision: observation.PayloadRevision, Residuals: []toolcontract.Residual{},
		Outcome: outcome, Disposition: disposition, ObservedUnixNano: observation.ObservedUnixNano,
	}, nil
}

func mcpProviderObservationContextV1(ctx context.Context) error {
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Provider Observation context is nil")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func nilLikeMCPProviderObservationDependencyV1(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

var _ applicationadapter.SingleCallToolProviderObservationReaderV1 = (*MCPProviderObservationReaderV1)(nil)

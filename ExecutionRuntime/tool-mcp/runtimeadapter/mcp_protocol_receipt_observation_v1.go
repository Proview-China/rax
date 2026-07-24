package runtimeadapter

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

const MCPProtocolReceiptRuntimeKindV1 = runtimeports.NamespacedNameV2("praxis.mcp/protocol-receipt")

var mcpProtocolReceiptSchemaDocumentV1 = []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","title":"MCP tools/call protocol receipt","type":"object"}`)

// MCPProtocolReceiptObservationReaderV1 exposes an exact historical MCP
// receipt through Runtime's neutral Provider receipt reader. It grants no
// execution authority and does not create Evidence or DomainResult facts.
type MCPProtocolReceiptObservationReaderV1 struct {
	receipts toolcontract.MCPProtocolReceiptExactReaderV1
	commands toolcontract.MCPExecutionCommandExactReaderV1
	owner    runtimeports.EffectOwnerRefV2
	clock    func() time.Time
}

func NewMCPProtocolReceiptObservationReaderV1(
	receipts toolcontract.MCPProtocolReceiptExactReaderV1,
	commands toolcontract.MCPExecutionCommandExactReaderV1,
	owner runtimeports.EffectOwnerRefV2,
	clock func() time.Time,
) (*MCPProtocolReceiptObservationReaderV1, error) {
	if nilLikeMCPReceiptDependencyV1(receipts) || nilLikeMCPReceiptDependencyV1(commands) || owner.Role != runtimeports.OwnerSettlement || runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(owner.ComponentID)) != nil || owner.ManifestDigest.Validate() != nil || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Protocol Receipt observation reader dependencies are incomplete")
	}
	return &MCPProtocolReceiptObservationReaderV1{receipts: receipts, commands: commands, owner: owner, clock: clock}, nil
}

func (r *MCPProtocolReceiptObservationReaderV1) InspectOperationProviderReceiptV1(ctx context.Context, exact runtimeports.OperationProviderReceiptRefV1) (runtimeports.OperationProviderReceiptProjectionV1, error) {
	if err := mcpReceiptContextErrorV1(ctx); err != nil {
		return runtimeports.OperationProviderReceiptProjectionV1{}, err
	}
	if r == nil || nilLikeMCPReceiptDependencyV1(r.receipts) || nilLikeMCPReceiptDependencyV1(r.commands) || r.clock == nil {
		return runtimeports.OperationProviderReceiptProjectionV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Protocol Receipt observation reader is unavailable")
	}
	if exact.Validate() != nil || exact.Owner != r.owner || exact.Kind != MCPProtocolReceiptRuntimeKindV1 {
		return runtimeports.OperationProviderReceiptProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Runtime receipt Ref does not select this MCP receipt Owner and kind")
	}
	toolRef := toolcontract.MCPProtocolReceiptRefV1{ID: exact.ID, Revision: exact.Revision, Digest: exact.Digest}
	receipt, err := r.receipts.InspectMCPProtocolReceiptV1(ctx, toolRef)
	if err != nil {
		return runtimeports.OperationProviderReceiptProjectionV1{}, err
	}
	if receipt.Validate() != nil || receipt.Ref != toolRef {
		return runtimeports.OperationProviderReceiptProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP Protocol Receipt exact content drifted")
	}
	command, err := r.commands.InspectMCPExecutionCommandV1(ctx, receipt.Command)
	if err != nil {
		return runtimeports.OperationProviderReceiptProjectionV1{}, err
	}
	if command.Validate() != nil || command.Ref != receipt.Command || command.Owner != r.owner || command.Prepared.AttemptID != command.Attempt.AttemptID {
		return runtimeports.OperationProviderReceiptProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Protocol Receipt command causal chain drifted")
	}
	now := r.clock()
	if now.IsZero() || now.UnixNano() < receipt.ObservedUnixNano {
		return runtimeports.OperationProviderReceiptProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Protocol Receipt inspection clock regressed")
	}
	payload, err := mcpProtocolReceiptOpaquePayloadV1(receipt)
	if err != nil {
		return runtimeports.OperationProviderReceiptProjectionV1{}, err
	}
	return runtimeports.SealOperationProviderReceiptProjectionV1(runtimeports.OperationProviderReceiptProjectionV1{
		Ref: exact, Operation: command.Operation, OperationDigest: command.OperationDigest,
		Prepared: command.Prepared, Attempt: command.Attempt, Provider: command.Provider,
		ProviderOperationRef: receipt.Ref.ID, Payload: payload, PayloadRevision: 1,
		ObservedUnixNano: receipt.ObservedUnixNano, CheckedUnixNano: now.UnixNano(),
	})
}

func MCPProtocolReceiptRuntimeRefV1(owner runtimeports.EffectOwnerRefV2, ref toolcontract.MCPProtocolReceiptRefV1) (runtimeports.OperationProviderReceiptRefV1, error) {
	result := runtimeports.OperationProviderReceiptRefV1{Owner: owner, Kind: MCPProtocolReceiptRuntimeKindV1, ID: ref.ID, Revision: ref.Revision, Digest: ref.Digest}
	return result, result.Validate()
}

func mcpProtocolReceiptOpaquePayloadV1(receipt toolcontract.MCPProtocolReceiptV1) (runtimeports.OpaquePayloadV2, error) {
	policyDigest, err := core.CanonicalJSONDigest("praxis.tool-mcp.mcp-protocol-receipt", toolcontract.MCPProtocolReceiptContractVersionV1, "MCPProtocolReceiptOpaqueLimitPolicyV1", struct {
		MaxBytes uint64 `json:"max_bytes"`
		RefOnly  bool   `json:"ref_only"`
	}{toolcontract.MaxMCPProtocolReceiptBytesV1, true})
	if err != nil {
		return runtimeports.OpaquePayloadV2{}, err
	}
	payload := runtimeports.OpaquePayloadV2{
		Schema:        runtimeports.SchemaRefV2{Namespace: "praxis.tool-mcp", Name: "mcp-protocol-receipt", Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes(mcpProtocolReceiptSchemaDocumentV1)},
		ContentDigest: receipt.ResponseDigest, Length: uint64(len(receipt.CanonicalResponse)), Ref: "mcp-receipt://" + receipt.Ref.ID,
		LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "praxis.tool-mcp/mcp-protocol-receipt-v1", Digest: policyDigest},
	}
	return payload, payload.Validate()
}

func mcpReceiptContextErrorV1(ctx context.Context) error {
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Protocol Receipt inspection context is nil")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func nilLikeMCPReceiptDependencyV1(value any) bool {
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

var _ runtimeports.OperationProviderReceiptReaderV1 = (*MCPProtocolReceiptObservationReaderV1)(nil)

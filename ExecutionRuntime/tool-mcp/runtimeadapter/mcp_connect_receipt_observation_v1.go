package runtimeadapter

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	toolmcp "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
)

const MCPConnectProtocolReceiptRuntimeKindV1 = runtimeports.NamespacedNameV2("praxis.mcp/connect-protocol-receipt")

var mcpConnectReceiptSchemaDocumentV1 = []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","title":"MCP Connect initialize receipt","type":"object"}`)

type MCPConnectPhysicalEntryReaderV1 interface {
	InspectMCPConnectPhysicalV1(context.Context, core.Digest) (toolmcp.MCPConnectPhysicalEntryV1, error)
}

// MCPConnectReceiptObservationReaderV1 maps one Tool-owned immutable Connect
// receipt to Runtime's neutral historical receipt projection. It creates no
// Evidence, Provider Observation, Connection Fact, DomainResult or Settlement.
type MCPConnectReceiptObservationReaderV1 struct {
	receipts toolcontract.MCPConnectProtocolReceiptExactReaderV1
	entries  MCPConnectPhysicalEntryReaderV1
	owner    runtimeports.EffectOwnerRefV2
	clock    func() time.Time
}

func NewMCPConnectReceiptObservationReaderV1(receipts toolcontract.MCPConnectProtocolReceiptExactReaderV1, entries MCPConnectPhysicalEntryReaderV1, owner runtimeports.EffectOwnerRefV2, clock func() time.Time) (*MCPConnectReceiptObservationReaderV1, error) {
	if nilLikeMCPConnectReceiptDependencyV1(receipts) || nilLikeMCPConnectReceiptDependencyV1(entries) || owner.Role != runtimeports.OwnerSettlement || runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(owner.ComponentID)) != nil || owner.ManifestDigest.Validate() != nil || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Connect Receipt observation reader dependencies are incomplete")
	}
	return &MCPConnectReceiptObservationReaderV1{receipts: receipts, entries: entries, owner: owner, clock: clock}, nil
}

func (r *MCPConnectReceiptObservationReaderV1) InspectOperationProviderReceiptV1(ctx context.Context, exact runtimeports.OperationProviderReceiptRefV1) (runtimeports.OperationProviderReceiptProjectionV1, error) {
	if err := mcpConnectReceiptContextV1(ctx); err != nil {
		return runtimeports.OperationProviderReceiptProjectionV1{}, err
	}
	if r == nil || nilLikeMCPConnectReceiptDependencyV1(r.receipts) || nilLikeMCPConnectReceiptDependencyV1(r.entries) || r.clock == nil {
		return runtimeports.OperationProviderReceiptProjectionV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Connect Receipt observation reader is unavailable")
	}
	if exact.Validate() != nil || exact.Owner != r.owner || exact.Kind != MCPConnectProtocolReceiptRuntimeKindV1 {
		return runtimeports.OperationProviderReceiptProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Runtime receipt Ref does not select this MCP Connect Owner and kind")
	}
	toolRef := toolcontract.MCPConnectProtocolReceiptRefV1{ID: exact.ID, Revision: exact.Revision, Digest: exact.Digest}
	receipt, err := r.receipts.InspectMCPConnectProtocolReceiptV1(ctx, toolRef)
	if err != nil {
		return runtimeports.OperationProviderReceiptProjectionV1{}, err
	}
	entry, err := r.entries.InspectMCPConnectPhysicalV1(ctx, receipt.StableKeyDigest)
	if err != nil {
		return runtimeports.OperationProviderReceiptProjectionV1{}, err
	}
	if err = validateMCPConnectReceiptEntryV1(receipt, entry, r.owner); err != nil {
		return runtimeports.OperationProviderReceiptProjectionV1{}, err
	}
	now := r.clock()
	if now.IsZero() || now.UnixNano() < receipt.ObservedUnixNano {
		return runtimeports.OperationProviderReceiptProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Connect Receipt inspection clock regressed")
	}
	payload, err := mcpConnectReceiptOpaquePayloadV1(receipt)
	if err != nil {
		return runtimeports.OperationProviderReceiptProjectionV1{}, err
	}
	authorization := entry.Authorization
	return runtimeports.SealOperationProviderReceiptProjectionV1(runtimeports.OperationProviderReceiptProjectionV1{
		Ref: exact, Operation: authorization.Operation, OperationDigest: authorization.OperationDigest,
		Prepared: authorization.Prepared, Attempt: authorization.Attempt, Provider: authorization.Provider,
		ProviderOperationRef: receipt.Ref.ID, Payload: payload, PayloadRevision: 1,
		ObservedUnixNano: receipt.ObservedUnixNano, CheckedUnixNano: now.UnixNano(),
	})
}

func MCPConnectProtocolReceiptRuntimeRefV1(owner runtimeports.EffectOwnerRefV2, ref toolcontract.MCPConnectProtocolReceiptRefV1) (runtimeports.OperationProviderReceiptRefV1, error) {
	result := runtimeports.OperationProviderReceiptRefV1{Owner: owner, Kind: MCPConnectProtocolReceiptRuntimeKindV1, ID: ref.ID, Revision: ref.Revision, Digest: ref.Digest}
	return result, result.Validate()
}

func validateMCPConnectReceiptEntryV1(receipt toolcontract.MCPConnectProtocolReceiptV1, entry toolmcp.MCPConnectPhysicalEntryV1, owner runtimeports.EffectOwnerRefV2) error {
	if receipt.Validate() != nil || entry.State != toolmcp.MCPConnectPhysicalObservedV1 || entry.ProtocolReceipt == nil || entry.ProtocolReceipt.Ref != receipt.Ref || entry.StableKeyDigest != receipt.StableKeyDigest || entry.AdmissionReceipt != receipt.AdmissionReceipt || entry.Intent != receipt.Intent || entry.TransportConfig != receipt.TransportConfig || entry.Authorization.Validate() != nil || entry.Authorization.Digest != entry.AuthorizationDigest || entry.Authorization.StableKeyDigest != receipt.StableKeyDigest || entry.Authorization.DomainCommand.Owner != owner {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP Connect Receipt physical causal chain drifted")
	}
	return nil
}

func mcpConnectReceiptOpaquePayloadV1(receipt toolcontract.MCPConnectProtocolReceiptV1) (runtimeports.OpaquePayloadV2, error) {
	policyDigest, err := core.CanonicalJSONDigest("praxis.tool-mcp.mcp-connect-protocol-receipt", toolcontract.MCPConnectProtocolReceiptContractVersionV1, "MCPConnectProtocolReceiptOpaqueLimitPolicyV1", struct {
		MaxBytes uint64 `json:"max_bytes"`
		RefOnly  bool   `json:"ref_only"`
	}{toolcontract.MaxMCPConnectInitializeReceiptBytesV1, true})
	if err != nil {
		return runtimeports.OpaquePayloadV2{}, err
	}
	payload := runtimeports.OpaquePayloadV2{
		Schema:        runtimeports.SchemaRefV2{Namespace: "praxis.tool-mcp", Name: "mcp-connect-protocol-receipt", Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes(mcpConnectReceiptSchemaDocumentV1)},
		ContentDigest: receipt.ResponseDigest, Length: uint64(len(receipt.InitializeResponse)), Ref: "mcp-connect-receipt://" + receipt.Ref.ID,
		LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "praxis.tool-mcp/mcp-connect-protocol-receipt-v1", Digest: policyDigest},
	}
	return payload, payload.Validate()
}

func mcpConnectReceiptContextV1(ctx context.Context) error {
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Connect Receipt inspection context is nil")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func nilLikeMCPConnectReceiptDependencyV1(value any) bool {
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

var _ runtimeports.OperationProviderReceiptReaderV1 = (*MCPConnectReceiptObservationReaderV1)(nil)

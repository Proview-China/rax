package runtimeadapter

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	toolmcp "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
)

const MCPDiscoveryPageProtocolReceiptRuntimeKindV1 = runtimeports.NamespacedNameV2("praxis.mcp/discovery-page-protocol-receipt")

type MCPDiscoveryPagePhysicalReaderV1 interface {
	InspectMCPDiscoveryPagePhysicalV1(context.Context, core.Digest) (toolmcp.MCPDiscoveryPagePhysicalEntryV1, error)
	InspectMCPDiscoveryPageObservationV1(context.Context, toolcontract.ObjectRef) (toolmcp.MCPDiscoveryPageObservationV1, error)
}

type MCPDiscoveryPageReceiptObservationReaderV1 struct {
	receipts toolcontract.MCPDiscoveryPageProtocolReceiptExactReaderV1
	physical MCPDiscoveryPagePhysicalReaderV1
	owner    runtimeports.EffectOwnerRefV2
	clock    func() time.Time
}

func NewMCPDiscoveryPageReceiptObservationReaderV1(receipts toolcontract.MCPDiscoveryPageProtocolReceiptExactReaderV1, physical MCPDiscoveryPagePhysicalReaderV1, owner runtimeports.EffectOwnerRefV2, clock func() time.Time) (*MCPDiscoveryPageReceiptObservationReaderV1, error) {
	if nilLikeMCPConnectReceiptDependencyV1(receipts) || nilLikeMCPConnectReceiptDependencyV1(physical) || owner.Role != runtimeports.OwnerSettlement || runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(owner.ComponentID)) != nil || owner.ManifestDigest.Validate() != nil || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Discovery Page Receipt reader dependencies are incomplete")
	}
	return &MCPDiscoveryPageReceiptObservationReaderV1{receipts: receipts, physical: physical, owner: owner, clock: clock}, nil
}

func MCPDiscoveryPageProtocolReceiptRuntimeRefV1(owner runtimeports.EffectOwnerRefV2, ref toolcontract.ObjectRef) (runtimeports.OperationProviderReceiptRefV1, error) {
	result := runtimeports.OperationProviderReceiptRefV1{Owner: owner, Kind: MCPDiscoveryPageProtocolReceiptRuntimeKindV1, ID: ref.ID, Revision: ref.Revision, Digest: ref.Digest}
	return result, result.Validate()
}

func (r *MCPDiscoveryPageReceiptObservationReaderV1) InspectOperationProviderReceiptV1(ctx context.Context, exact runtimeports.OperationProviderReceiptRefV1) (runtimeports.OperationProviderReceiptProjectionV1, error) {
	if ctx == nil {
		return runtimeports.OperationProviderReceiptProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Discovery Page Receipt context is nil")
	}
	if err := ctx.Err(); err != nil {
		return runtimeports.OperationProviderReceiptProjectionV1{}, err
	}
	if r == nil || nilLikeMCPConnectReceiptDependencyV1(r.receipts) || nilLikeMCPConnectReceiptDependencyV1(r.physical) || r.clock == nil {
		return runtimeports.OperationProviderReceiptProjectionV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Discovery Page Receipt reader is unavailable")
	}
	if exact.Validate() != nil || exact.Owner != r.owner || exact.Kind != MCPDiscoveryPageProtocolReceiptRuntimeKindV1 {
		return runtimeports.OperationProviderReceiptProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Runtime Receipt Ref selects another MCP Discovery owner or kind")
	}
	toolRef := toolcontract.ObjectRef{ID: exact.ID, Revision: exact.Revision, Digest: exact.Digest}
	receipt, err := r.receipts.InspectMCPDiscoveryPageProtocolReceiptV1(ctx, toolRef)
	if err != nil {
		return runtimeports.OperationProviderReceiptProjectionV1{}, err
	}
	entry, err := r.physical.InspectMCPDiscoveryPagePhysicalV1(ctx, receipt.StableKeyDigest)
	if err != nil {
		return runtimeports.OperationProviderReceiptProjectionV1{}, err
	}
	observation, err := r.physical.InspectMCPDiscoveryPageObservationV1(ctx, toolRef)
	if err != nil {
		return runtimeports.OperationProviderReceiptProjectionV1{}, err
	}
	if entry.State != toolmcp.MCPDiscoveryPagePhysicalObservedV1 || entry.ProtocolReceipt == nil || entry.ProtocolReceipt.Ref != receipt.Ref || entry.Observation == nil || entry.Observation.Digest != receipt.ResponsePageDigest || entry.Authorization.DomainCommand.Owner != r.owner || observation.Digest != receipt.ResponsePageDigest {
		return runtimeports.OperationProviderReceiptProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP Discovery Page Receipt physical closure drifted")
	}
	now := r.clock()
	if now.IsZero() || now.UnixNano() < receipt.ObservedUnixNano {
		return runtimeports.OperationProviderReceiptProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Discovery Page Receipt clock regressed")
	}
	data, err := json.Marshal(observation)
	if err != nil {
		return runtimeports.OperationProviderReceiptProjectionV1{}, err
	}
	policy, err := core.CanonicalJSONDigest("praxis.tool-mcp.mcp-discovery-page-receipt", toolcontract.MCPDiscoveryPageReceiptContractVersionV1, "MCPDiscoveryPageReceiptOpaqueLimitPolicyV1", struct {
		MaxBytes uint64 `json:"max_bytes"`
		RefOnly  bool   `json:"ref_only"`
	}{uint64(len(data)), true})
	if err != nil {
		return runtimeports.OperationProviderReceiptProjectionV1{}, err
	}
	payload := runtimeports.OpaquePayloadV2{Schema: runtimeports.SchemaRefV2{Namespace: "praxis.tool-mcp", Name: "mcp-discovery-page-receipt", Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes([]byte(`{"type":"object","title":"MCP Discovery Page Receipt"}`))}, ContentDigest: receipt.ResponsePageDigest, Length: uint64(len(data)), Ref: "mcp-discovery-page-receipt://" + receipt.Ref.ID, LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "praxis.tool-mcp/mcp-discovery-page-receipt-v1", Digest: policy}}
	if err = payload.Validate(); err != nil {
		return runtimeports.OperationProviderReceiptProjectionV1{}, err
	}
	a := entry.Authorization
	return runtimeports.SealOperationProviderReceiptProjectionV1(runtimeports.OperationProviderReceiptProjectionV1{Ref: exact, Operation: a.Operation, OperationDigest: a.OperationDigest, Prepared: a.Prepared, Attempt: a.Attempt, Provider: a.Provider, ProviderOperationRef: receipt.Ref.ID, Payload: payload, PayloadRevision: 1, ObservedUnixNano: receipt.ObservedUnixNano, CheckedUnixNano: now.UnixNano()})
}

var _ runtimeports.OperationProviderReceiptReaderV1 = (*MCPDiscoveryPageReceiptObservationReaderV1)(nil)

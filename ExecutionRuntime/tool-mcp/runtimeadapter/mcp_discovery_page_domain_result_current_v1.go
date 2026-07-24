package runtimeadapter

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type MCPDiscoveryPageDomainResultCurrentAdapterV1 struct {
	source toolcontract.MCPDiscoveryPageDomainResultExactReaderV1
	owner  runtimeports.ProviderBindingRefV2
	clock  func() time.Time
	ttl    time.Duration
}

func NewMCPDiscoveryPageDomainResultCurrentAdapterV1(source toolcontract.MCPDiscoveryPageDomainResultExactReaderV1, owner runtimeports.ProviderBindingRefV2, clock func() time.Time, ttl time.Duration) (*MCPDiscoveryPageDomainResultCurrentAdapterV1, error) {
	if nilLikeMCPConnectReceiptDependencyV1(source) || owner.Validate() != nil || owner.Capability != runtimeports.CapabilityNameV2(runtimeports.OperationScopeEvidenceMCPDiscoveryPageEffectKindV1) || clock == nil || ttl <= 0 || ttl > toolcontract.MaxMCPDiscoveryPageDomainResultCurrentTTLV1 {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Discovery Page DomainResult adapter dependencies are incomplete")
	}
	return &MCPDiscoveryPageDomainResultCurrentAdapterV1{source: source, owner: owner, clock: clock, ttl: ttl}, nil
}

func (a *MCPDiscoveryPageDomainResultCurrentAdapterV1) InspectOperationSettlementDomainResultCurrentV4(ctx context.Context, effectKind runtimeports.EffectKindV2, exact runtimeports.OperationSettlementDomainResultFactRefV4) (runtimeports.OperationSettlementDomainResultCurrentV4, error) {
	if ctx == nil {
		return runtimeports.OperationSettlementDomainResultCurrentV4{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Discovery Page DomainResult context is nil")
	}
	if a == nil || nilLikeMCPConnectReceiptDependencyV1(a.source) || a.clock == nil {
		return runtimeports.OperationSettlementDomainResultCurrentV4{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Discovery Page DomainResult adapter is unavailable")
	}
	if effectKind != runtimeports.OperationScopeEvidenceMCPDiscoveryPageEffectKindV1 || exact.Validate() != nil {
		return runtimeports.OperationSettlementDomainResultCurrentV4{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Discovery Page Runtime DomainResult ref is invalid")
	}
	now := a.clock()
	if now.IsZero() {
		return runtimeports.OperationSettlementDomainResultCurrentV4{}, core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "MCP Discovery Page DomainResult current time is missing")
	}
	ref := toolcontract.ObjectRef{ID: exact.ID, Revision: exact.Revision, Digest: exact.Digest}
	fact, err := a.source.InspectMCPDiscoveryPageDomainResultV1(ctx, ref)
	if err != nil {
		return runtimeports.OperationSettlementDomainResultCurrentV4{}, err
	}
	mapped := MCPDiscoveryPageDomainResultRuntimeRefV1(a.owner, fact)
	if !runtimeports.SameOperationSettlementDomainResultFactRefV4(mapped, exact) {
		return runtimeports.OperationSettlementDomainResultCurrentV4{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "Runtime DomainResult ref cannot be losslessly mapped to MCP Discovery Page fact")
	}
	current, err := a.source.InspectCurrentMCPDiscoveryPageDomainResultV1(ctx, ref, a.ttl)
	if err != nil {
		return runtimeports.OperationSettlementDomainResultCurrentV4{}, err
	}
	if current.Fact != ref || current.Command != fact.Command || current.ProtocolReceipt != fact.ProtocolReceipt || current.Observation != fact.Observation || current.Owner != fact.Owner {
		return runtimeports.OperationSettlementDomainResultCurrentV4{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "MCP Discovery Page DomainResult current projection drifted")
	}
	return runtimeports.SealOperationSettlementDomainResultCurrentV4(runtimeports.OperationSettlementDomainResultCurrentV4{EffectKind: effectKind, Fact: exact, CheckedUnixNano: current.CheckedUnixNano, ExpiresUnixNano: current.ExpiresUnixNano}, now)
}

func MCPDiscoveryPageDomainResultRuntimeRefV1(owner runtimeports.ProviderBindingRefV2, fact toolcontract.MCPDiscoveryPageDomainResultFactV1) runtimeports.OperationSettlementDomainResultFactRefV4 {
	return runtimeports.OperationSettlementDomainResultFactRefV4{Owner: owner, Kind: toolcontract.MCPDiscoveryPageDomainResultRuntimeKindV1, ID: fact.ID, Revision: fact.Revision, Digest: fact.Digest, TenantID: fact.TenantID, EffectID: fact.Attempt.EffectID, EffectRevision: fact.Attempt.IntentRevision, Operation: fact.Operation, OperationDigest: fact.Attempt.OperationDigest, Attempt: fact.Attempt, Schema: fact.Schema, PayloadDigest: fact.PayloadDigest, PayloadRevision: fact.PayloadRevision, AuthoritativeTime: fact.CreatedUnixNano}
}

var _ runtimeports.OperationSettlementDomainResultCurrentReaderV4 = (*MCPDiscoveryPageDomainResultCurrentAdapterV1)(nil)

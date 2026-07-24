package runtimeadapter

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type MCPConnectDomainResultCurrentSourceV1 interface {
	InspectMCPConnectDomainResultV1(context.Context, toolcontract.ObjectRef) (toolcontract.MCPConnectDomainResultFactV1, error)
	InspectCurrentMCPConnectDomainResultV1(context.Context, toolcontract.ObjectRef, time.Duration) (toolcontract.MCPConnectDomainResultCurrentProjectionV1, error)
}

type MCPConnectDomainResultCurrentAdapterV1 struct {
	source MCPConnectDomainResultCurrentSourceV1
	owner  runtimeports.ProviderBindingRefV2
	clock  func() time.Time
	ttl    time.Duration
}

func NewMCPConnectDomainResultCurrentAdapterV1(source MCPConnectDomainResultCurrentSourceV1, owner runtimeports.ProviderBindingRefV2, clock func() time.Time, ttl time.Duration) (*MCPConnectDomainResultCurrentAdapterV1, error) {
	if nilLikeMCPConnectReceiptDependencyV1(source) || owner.Validate() != nil || owner.Capability != runtimeports.CapabilityNameV2(toolcontract.MCPConnectEffectKindV1) || clock == nil || ttl <= 0 || ttl > toolcontract.MaxMCPConnectDomainResultCurrentTTLV1 {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Connect DomainResult current adapter dependencies are incomplete")
	}
	return &MCPConnectDomainResultCurrentAdapterV1{source: source, owner: owner, clock: clock, ttl: ttl}, nil
}

func (a *MCPConnectDomainResultCurrentAdapterV1) InspectOperationSettlementDomainResultCurrentV4(ctx context.Context, effectKind runtimeports.EffectKindV2, exact runtimeports.OperationSettlementDomainResultFactRefV4) (runtimeports.OperationSettlementDomainResultCurrentV4, error) {
	if ctx == nil {
		return runtimeports.OperationSettlementDomainResultCurrentV4{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Connect DomainResult current context is nil")
	}
	if a == nil || nilLikeMCPConnectReceiptDependencyV1(a.source) || a.clock == nil {
		return runtimeports.OperationSettlementDomainResultCurrentV4{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Connect DomainResult current adapter is unavailable")
	}
	if effectKind != toolcontract.MCPConnectEffectKindV1 || exact.Validate() != nil {
		return runtimeports.OperationSettlementDomainResultCurrentV4{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Connect DomainResult Runtime exact ref is invalid")
	}
	now := a.clock()
	if now.IsZero() {
		return runtimeports.OperationSettlementDomainResultCurrentV4{}, core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "MCP Connect DomainResult current time is missing")
	}
	toolRef := toolcontract.ObjectRef{ID: exact.ID, Revision: exact.Revision, Digest: exact.Digest}
	fact, err := a.source.InspectMCPConnectDomainResultV1(ctx, toolRef)
	if err != nil {
		return runtimeports.OperationSettlementDomainResultCurrentV4{}, err
	}
	mapped := MCPConnectDomainResultRuntimeRefV1(a.owner, fact)
	if !runtimeports.SameOperationSettlementDomainResultFactRefV4(mapped, exact) {
		return runtimeports.OperationSettlementDomainResultCurrentV4{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "Runtime DomainResult ref cannot be losslessly mapped to MCP Connect fact")
	}
	current, err := a.source.InspectCurrentMCPConnectDomainResultV1(ctx, toolRef, a.ttl)
	if err != nil {
		return runtimeports.OperationSettlementDomainResultCurrentV4{}, err
	}
	if current.Fact != toolRef || current.Connection != fact.Connection || current.Observation != fact.Observation || current.PrepareConsumption != fact.PrepareConsumption || current.ExecuteConsumption != fact.ExecuteConsumption || current.Owner != fact.Owner {
		return runtimeports.OperationSettlementDomainResultCurrentV4{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "MCP Connect DomainResult current projection drifted")
	}
	return runtimeports.SealOperationSettlementDomainResultCurrentV4(runtimeports.OperationSettlementDomainResultCurrentV4{EffectKind: effectKind, Fact: exact, CheckedUnixNano: current.CheckedUnixNano, ExpiresUnixNano: current.ExpiresUnixNano}, now)
}

func MCPConnectDomainResultRuntimeRefV1(owner runtimeports.ProviderBindingRefV2, fact toolcontract.MCPConnectDomainResultFactV1) runtimeports.OperationSettlementDomainResultFactRefV4 {
	return runtimeports.OperationSettlementDomainResultFactRefV4{Owner: owner, Kind: toolcontract.MCPConnectDomainResultRuntimeKindV1, ID: fact.ID, Revision: fact.Revision, Digest: fact.Digest, TenantID: fact.TenantID, EffectID: fact.Attempt.EffectID, EffectRevision: fact.Attempt.IntentRevision, Operation: fact.Operation, OperationDigest: fact.Attempt.OperationDigest, Attempt: fact.Attempt, Schema: fact.Schema, PayloadDigest: fact.PayloadDigest, PayloadRevision: fact.PayloadRevision, AuthoritativeTime: fact.CreatedUnixNano}
}

var _ runtimeports.OperationSettlementDomainResultCurrentReaderV4 = (*MCPConnectDomainResultCurrentAdapterV1)(nil)

package mcp

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type MCPDiscoveryPageApplyStoreV1 struct {
	mu          sync.RWMutex
	values      map[string]toolcontract.MCPDiscoveryPageApplySettlementFactV1
	byCommand   map[string]string
	domains     toolcontract.MCPDiscoveryPageDomainResultExactReaderV1
	receipts    toolcontract.MCPDiscoveryPageProtocolReceiptExactReaderV1
	settlements MCPConnectSettlementInspectionReaderV1
	provider    runtimeports.ProviderBindingRefV2
	clock       func() time.Time
}

func NewMCPDiscoveryPageApplyStoreV1(domains toolcontract.MCPDiscoveryPageDomainResultExactReaderV1, receipts toolcontract.MCPDiscoveryPageProtocolReceiptExactReaderV1, settlements MCPConnectSettlementInspectionReaderV1, provider runtimeports.ProviderBindingRefV2, clock func() time.Time) (*MCPDiscoveryPageApplyStoreV1, error) {
	if nilLikeOfficialSDKConnectV1(domains) || nilLikeOfficialSDKConnectV1(receipts) || nilLikeOfficialSDKConnectV1(settlements) || provider.Validate() != nil || provider.Capability != runtimeports.CapabilityNameV2(runtimeports.OperationScopeEvidenceMCPDiscoveryPageEffectKindV1) || clock == nil {
		return nil, invalid("MCP Discovery Page Apply Store dependencies are incomplete")
	}
	return &MCPDiscoveryPageApplyStoreV1{values: map[string]toolcontract.MCPDiscoveryPageApplySettlementFactV1{}, byCommand: map[string]string{}, domains: domains, receipts: receipts, settlements: settlements, provider: provider, clock: clock}, nil
}

func (s *MCPDiscoveryPageApplyStoreV1) ApplyMCPDiscoveryPageSettlementV1(ctx context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPDiscoveryPageApplySettlementFactV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPDiscoveryPageApplySettlementFactV1{}, err
	}
	if s == nil || exact.Validate() != nil {
		return toolcontract.MCPDiscoveryPageApplySettlementFactV1{}, invalid("MCP Discovery Page Apply request is invalid")
	}
	previous, err := s.freshDiscoveryApplyV1(time.Time{})
	if err != nil {
		return toolcontract.MCPDiscoveryPageApplySettlementFactV1{}, err
	}
	c1, err := s.inspectDiscoveryApplyClosureV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPDiscoveryPageApplySettlementFactV1{}, err
	}
	after, err := s.freshDiscoveryApplyV1(previous)
	if err != nil {
		return toolcontract.MCPDiscoveryPageApplySettlementFactV1{}, err
	}
	if err = c1.validate(after, s.provider); err != nil {
		return toolcontract.MCPDiscoveryPageApplySettlementFactV1{}, err
	}
	c2, err := s.inspectDiscoveryApplyClosureV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPDiscoveryPageApplySettlementFactV1{}, err
	}
	actual, err := s.freshDiscoveryApplyV1(after)
	if err != nil {
		return toolcontract.MCPDiscoveryPageApplySettlementFactV1{}, err
	}
	if !reflect.DeepEqual(c1, c2) {
		return toolcontract.MCPDiscoveryPageApplySettlementFactV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery Page Apply closure drifted")
	}
	if err = c2.validate(actual, s.provider); err != nil {
		return toolcontract.MCPDiscoveryPageApplySettlementFactV1{}, err
	}
	fact, err := toolcontract.SealMCPDiscoveryPageApplySettlementFactV1(toolcontract.MCPDiscoveryPageApplySettlementFactV1{Command: c2.domain.Command, DomainResult: c2.domain.ObjectRef(), Inspection: c2.inspection, Owner: c2.domain.Owner, AppliedUnixNano: actual.UnixNano()})
	if err != nil {
		return fact, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return fact, err
	}
	if id, ok := s.byCommand[fact.Command.ID]; ok {
		winner := s.values[id]
		if !reflect.DeepEqual(winner, fact) {
			return fact, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "MCP Discovery Page already applied another settlement")
		}
		return winner, nil
	}
	s.values[fact.Ref.ID] = fact
	s.byCommand[fact.Command.ID] = fact.Ref.ID
	return fact, nil
}

func (s *MCPDiscoveryPageApplyStoreV1) InspectMCPDiscoveryPageApplySettlementV1(ctx context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPDiscoveryPageApplySettlementFactV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPDiscoveryPageApplySettlementFactV1{}, err
	}
	s.mu.RLock()
	fact, ok := s.values[exact.ID]
	s.mu.RUnlock()
	if !ok {
		return fact, core.NewError(core.ErrorNotFound, core.ReasonEffectSettlementMissing, "MCP Discovery Page ApplySettlement not found")
	}
	if fact.Ref != exact {
		return fact, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery Page ApplySettlement Ref drifted")
	}
	return fact, nil
}

func (s *MCPDiscoveryPageApplyStoreV1) InspectCurrentMCPDiscoveryPageAppliedV1(ctx context.Context, command toolcontract.ObjectRef, ttl time.Duration) (toolcontract.MCPDiscoveryPageAppliedCurrentProjectionV1, error) {
	if ttl <= 0 || ttl > toolcontract.MaxMCPDiscoveryPageAppliedCurrentTTLV1 {
		return toolcontract.MCPDiscoveryPageAppliedCurrentProjectionV1{}, invalid("MCP Discovery Page applied TTL is invalid")
	}
	s.mu.RLock()
	id, ok := s.byCommand[command.ID]
	apply := s.values[id]
	s.mu.RUnlock()
	if !ok || apply.Command != command {
		return toolcontract.MCPDiscoveryPageAppliedCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonEffectSettlementMissing, "MCP Discovery Page is not applied")
	}
	now := s.clock()
	if now.IsZero() || now.UnixNano() < apply.AppliedUnixNano {
		return toolcontract.MCPDiscoveryPageAppliedCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Discovery Page applied clock regressed")
	}
	domain, err := s.domains.InspectCurrentMCPDiscoveryPageDomainResultV1(ctx, apply.DomainResult, ttl)
	if err != nil {
		return toolcontract.MCPDiscoveryPageAppliedCurrentProjectionV1{}, err
	}
	receipt, err := s.receipts.InspectMCPDiscoveryPageProtocolReceiptV1(ctx, domain.ProtocolReceipt)
	if err != nil {
		return toolcontract.MCPDiscoveryPageAppliedCurrentProjectionV1{}, err
	}
	inspection, err := s.settlements.InspectCurrentOperationSettlementV4(ctx, runtimeports.InspectCurrentOperationSettlementRequestV4{Operation: apply.Inspection.DomainResult.Operation, EffectID: apply.Inspection.Settlement.EffectID})
	if err != nil {
		return toolcontract.MCPDiscoveryPageAppliedCurrentProjectionV1{}, err
	}
	if inspection.Digest != apply.Inspection.Digest || domain.Fact != apply.DomainResult {
		return toolcontract.MCPDiscoveryPageAppliedCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "MCP Discovery Page applied settlement drifted")
	}
	expires := now.Add(ttl).UnixNano()
	for _, bound := range []int64{domain.ExpiresUnixNano, inspection.ExpiresUnixNano} {
		if bound < expires {
			expires = bound
		}
	}
	return toolcontract.SealMCPDiscoveryPageAppliedCurrentProjectionV1(toolcontract.MCPDiscoveryPageAppliedCurrentProjectionV1{Command: command, ProtocolReceipt: receipt.Ref, DomainResult: apply.DomainResult, ApplySettlement: apply.Ref, Namespace: receipt.Namespace, PageOrdinal: receipt.PageOrdinal, NextCursor: receipt.NextCursor, Owner: apply.Owner, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires}, now)
}

type mcpDiscoveryApplyClosureV1 struct {
	domain     toolcontract.MCPDiscoveryPageDomainResultFactV1
	current    toolcontract.MCPDiscoveryPageDomainResultCurrentProjectionV1
	inspection runtimeports.OperationInspectionSettlementRefV4
	bundle     runtimeports.OperationSettlementCommitBundleV4
}

func (s *MCPDiscoveryPageApplyStoreV1) inspectDiscoveryApplyClosureV1(ctx context.Context, exact toolcontract.ObjectRef) (mcpDiscoveryApplyClosureV1, error) {
	var c mcpDiscoveryApplyClosureV1
	var err error
	c.domain, err = s.domains.InspectMCPDiscoveryPageDomainResultV1(ctx, exact)
	if err != nil {
		return c, err
	}
	c.current, err = s.domains.InspectCurrentMCPDiscoveryPageDomainResultV1(ctx, exact, 5*time.Second)
	if err != nil {
		return c, err
	}
	c.inspection, err = s.settlements.InspectCurrentOperationSettlementV4(ctx, runtimeports.InspectCurrentOperationSettlementRequestV4{Operation: c.domain.Operation, EffectID: c.domain.Attempt.EffectID})
	if err != nil {
		return c, err
	}
	c.bundle, err = s.settlements.InspectOperationSettlementClosureV4(ctx, runtimeports.InspectOperationSettlementRequestV4{Operation: c.domain.Operation, SettlementID: c.inspection.Settlement.ID})
	return c, err
}
func (c mcpDiscoveryApplyClosureV1) validate(now time.Time, provider runtimeports.ProviderBindingRefV2) error {
	if c.domain.Validate() != nil || c.current.Validate(now) != nil || c.inspection.Validate(now) != nil || c.bundle.Validate() != nil || c.current.Fact != c.domain.ObjectRef() {
		return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "MCP Discovery Page Apply closure is invalid")
	}
	expected := mcpDiscoveryPageRuntimeDomainResultRefV1(provider, c.domain)
	if !runtimeports.SameOperationSettlementDomainResultFactRefV4(expected, c.inspection.DomainResult) || !runtimeports.SameOperationSettlementDomainResultFactRefV4(expected, c.bundle.Settlement.Submission.DomainResult) || !runtimeports.SameOperationSettlementRefV4(c.inspection.Settlement, c.bundle.Settlement.RefV4()) || c.inspection.Owner != c.domain.Owner {
		return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "Runtime Settlement V4 does not close MCP Discovery Page DomainResult")
	}
	if c.bundle.Association.Prepare.Consumption != c.domain.PrepareConsumption || c.bundle.Association.Execute.Consumption != c.domain.ExecuteConsumption || c.bundle.Association.Prepare.EnforcementPhase != c.domain.PrepareEnforcement || c.bundle.Association.Execute.EnforcementPhase != c.domain.ExecuteEnforcement {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Runtime Settlement Evidence does not close MCP Discovery Page")
	}
	return nil
}
func mcpDiscoveryPageRuntimeDomainResultRefV1(owner runtimeports.ProviderBindingRefV2, f toolcontract.MCPDiscoveryPageDomainResultFactV1) runtimeports.OperationSettlementDomainResultFactRefV4 {
	return runtimeports.OperationSettlementDomainResultFactRefV4{Owner: owner, Kind: toolcontract.MCPDiscoveryPageDomainResultRuntimeKindV1, ID: f.ID, Revision: f.Revision, Digest: f.Digest, TenantID: f.TenantID, EffectID: f.Attempt.EffectID, EffectRevision: f.Attempt.IntentRevision, Operation: f.Operation, OperationDigest: f.Attempt.OperationDigest, Attempt: f.Attempt, Schema: f.Schema, PayloadDigest: f.PayloadDigest, PayloadRevision: f.PayloadRevision, AuthoritativeTime: f.CreatedUnixNano}
}
func (s *MCPDiscoveryPageApplyStoreV1) freshDiscoveryApplyV1(previous time.Time) (time.Time, error) {
	now := s.clock()
	if now.IsZero() || !previous.IsZero() && now.Before(previous) {
		return time.Time{}, core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "MCP Discovery Page Apply clock regressed")
	}
	return now, nil
}

var _ toolcontract.MCPDiscoveryPageApplyExactReaderV1 = (*MCPDiscoveryPageApplyStoreV1)(nil)

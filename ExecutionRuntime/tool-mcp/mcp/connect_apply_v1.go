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

type MCPConnectSettlementInspectionReaderV1 interface {
	InspectCurrentOperationSettlementV4(context.Context, runtimeports.InspectCurrentOperationSettlementRequestV4) (runtimeports.OperationInspectionSettlementRefV4, error)
	InspectOperationSettlementClosureV4(context.Context, runtimeports.InspectOperationSettlementRequestV4) (runtimeports.OperationSettlementCommitBundleV4, error)
}

type MCPConnectApplyStoreV1 struct {
	mu           sync.RWMutex
	values       map[string]toolcontract.MCPConnectApplySettlementFactV1
	byConnection map[string]string
	domains      toolcontract.MCPConnectDomainResultExactReaderV1
	connections  toolcontract.MCPConnectionFactCurrentReaderV2
	settlements  MCPConnectSettlementInspectionReaderV1
	clock        func() time.Time
}

func NewMCPConnectApplyStoreV1(domains toolcontract.MCPConnectDomainResultExactReaderV1, connections toolcontract.MCPConnectionFactCurrentReaderV2, settlements MCPConnectSettlementInspectionReaderV1, clock func() time.Time) (*MCPConnectApplyStoreV1, error) {
	if nilLikeOfficialSDKConnectV1(domains) || nilLikeOfficialSDKConnectV1(connections) || nilLikeOfficialSDKConnectV1(settlements) || clock == nil {
		return nil, invalid("MCP Connect Apply Store V1 dependencies are incomplete")
	}
	return &MCPConnectApplyStoreV1{values: make(map[string]toolcontract.MCPConnectApplySettlementFactV1), byConnection: make(map[string]string), domains: domains, connections: connections, settlements: settlements, clock: clock}, nil
}

func (s *MCPConnectApplyStoreV1) ApplyMCPConnectSettlementV1(ctx context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPConnectApplySettlementFactV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPConnectApplySettlementFactV1{}, err
	}
	if s == nil || nilLikeOfficialSDKConnectV1(s.domains) || nilLikeOfficialSDKConnectV1(s.connections) || nilLikeOfficialSDKConnectV1(s.settlements) || s.clock == nil || exact.Validate() != nil {
		return toolcontract.MCPConnectApplySettlementFactV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Connect Apply Store V1 is unavailable")
	}
	previous, err := s.freshApplyV1(time.Time{})
	if err != nil {
		return toolcontract.MCPConnectApplySettlementFactV1{}, err
	}
	closure, err := s.inspectApplyClosureV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPConnectApplySettlementFactV1{}, err
	}
	afterS1, err := s.freshApplyV1(previous)
	if err != nil {
		return toolcontract.MCPConnectApplySettlementFactV1{}, err
	}
	if err = closure.validateV1(afterS1); err != nil {
		return toolcontract.MCPConnectApplySettlementFactV1{}, err
	}
	closure2, err := s.inspectApplyClosureV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPConnectApplySettlementFactV1{}, err
	}
	actual, err := s.freshApplyV1(afterS1)
	if err != nil {
		return toolcontract.MCPConnectApplySettlementFactV1{}, err
	}
	if !reflect.DeepEqual(closure, closure2) {
		return toolcontract.MCPConnectApplySettlementFactV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Connect settlement closure drifted between S1 and S2")
	}
	if err = closure2.validateV1(actual); err != nil {
		return toolcontract.MCPConnectApplySettlementFactV1{}, err
	}
	fact, err := toolcontract.SealMCPConnectApplySettlementFactV1(toolcontract.MCPConnectApplySettlementFactV1{Connection: closure2.domain.Connection, DomainResult: closure2.domain.ObjectRef(), Inspection: closure2.inspection, Owner: closure2.domain.Owner, AppliedUnixNano: actual.UnixNano()})
	if err != nil {
		return toolcontract.MCPConnectApplySettlementFactV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return toolcontract.MCPConnectApplySettlementFactV1{}, err
	}
	if id, ok := s.byConnection[fact.Connection.ID]; ok {
		winner := s.values[id]
		if !reflect.DeepEqual(winner, fact) {
			return toolcontract.MCPConnectApplySettlementFactV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "MCP Connection already applied another settlement")
		}
		return winner, nil
	}
	if winner, ok := s.values[fact.Ref.ID]; ok {
		if !reflect.DeepEqual(winner, fact) {
			return toolcontract.MCPConnectApplySettlementFactV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "MCP Connect ApplySettlement ID binds another fact")
		}
		return winner, nil
	}
	s.values[fact.Ref.ID] = fact
	s.byConnection[fact.Connection.ID] = fact.Ref.ID
	return fact, nil
}

func (s *MCPConnectApplyStoreV1) InspectMCPConnectApplySettlementV1(ctx context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPConnectApplySettlementFactV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPConnectApplySettlementFactV1{}, err
	}
	if s == nil || exact.Validate() != nil {
		return toolcontract.MCPConnectApplySettlementFactV1{}, invalid("MCP Connect ApplySettlement exact Inspect is invalid")
	}
	s.mu.RLock()
	fact, ok := s.values[exact.ID]
	s.mu.RUnlock()
	if !ok {
		return toolcontract.MCPConnectApplySettlementFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonEffectSettlementMissing, "MCP Connect ApplySettlement not found")
	}
	if fact.Ref != exact {
		return toolcontract.MCPConnectApplySettlementFactV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Connect ApplySettlement exact Ref drifted")
	}
	return fact, nil
}

func (s *MCPConnectApplyStoreV1) InspectCurrentMCPConnectionAvailabilityV1(ctx context.Context, connection toolcontract.MCPConnectionFactRefV2, ttl time.Duration) (toolcontract.MCPConnectionAvailabilityCurrentProjectionV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPConnectionAvailabilityCurrentProjectionV1{}, err
	}
	if s == nil || connection.Validate() != nil || ttl <= 0 || ttl > toolcontract.MaxMCPConnectionAvailabilityTTLV1 || s.clock == nil {
		return toolcontract.MCPConnectionAvailabilityCurrentProjectionV1{}, invalid("MCP Connection availability request is invalid")
	}
	s.mu.RLock()
	id, ok := s.byConnection[connection.ID]
	apply := s.values[id]
	s.mu.RUnlock()
	if !ok {
		return toolcontract.MCPConnectionAvailabilityCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonEffectSettlementMissing, "MCP Connection has no applied settlement")
	}
	if apply.Connection != connection {
		return toolcontract.MCPConnectionAvailabilityCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Connection availability exact Ref drifted")
	}
	now := s.clock()
	if now.IsZero() || now.UnixNano() < apply.AppliedUnixNano {
		return toolcontract.MCPConnectionAvailabilityCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Connection availability clock regressed")
	}
	current, err := s.connections.InspectCurrentMCPConnectionFactV2(ctx, connection)
	if err != nil {
		return toolcontract.MCPConnectionAvailabilityCurrentProjectionV1{}, err
	}
	inspection, err := s.settlements.InspectCurrentOperationSettlementV4(ctx, runtimeports.InspectCurrentOperationSettlementRequestV4{Operation: apply.Inspection.DomainResult.Operation, EffectID: apply.Inspection.Settlement.EffectID})
	if err != nil {
		return toolcontract.MCPConnectionAvailabilityCurrentProjectionV1{}, err
	}
	if inspection.Digest != apply.Inspection.Digest || inspection.DomainResult.ID != apply.DomainResult.ID || inspection.DomainResult.Revision != apply.DomainResult.Revision || inspection.DomainResult.Digest != apply.DomainResult.Digest || current.Ref != connection {
		return toolcontract.MCPConnectionAvailabilityCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "MCP Connection availability settlement drifted")
	}
	expires := now.Add(ttl).UnixNano()
	for _, bound := range []int64{current.ExpiresUnixNano, inspection.ExpiresUnixNano} {
		if bound < expires {
			expires = bound
		}
	}
	return toolcontract.SealMCPConnectionAvailabilityCurrentProjectionV1(toolcontract.MCPConnectionAvailabilityCurrentProjectionV1{Connection: connection, ApplySettlement: apply.Ref, DomainResult: apply.DomainResult, Owner: apply.Owner, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires}, now)
}

type mcpConnectApplyClosureV1 struct {
	domain     toolcontract.MCPConnectDomainResultFactV1
	domainNow  toolcontract.MCPConnectDomainResultCurrentProjectionV1
	connection toolcontract.MCPConnectionFactV2
	inspection runtimeports.OperationInspectionSettlementRefV4
	bundle     runtimeports.OperationSettlementCommitBundleV4
}

func (s *MCPConnectApplyStoreV1) inspectApplyClosureV1(ctx context.Context, exact toolcontract.ObjectRef) (mcpConnectApplyClosureV1, error) {
	var out mcpConnectApplyClosureV1
	var err error
	out.domain, err = s.domains.InspectMCPConnectDomainResultV1(ctx, exact)
	if err != nil {
		return out, err
	}
	out.domainNow, err = s.domains.InspectCurrentMCPConnectDomainResultV1(ctx, exact, 5*time.Second)
	if err != nil {
		return out, err
	}
	out.connection, err = s.connections.InspectCurrentMCPConnectionFactV2(ctx, out.domain.Connection)
	if err != nil {
		return out, err
	}
	out.inspection, err = s.settlements.InspectCurrentOperationSettlementV4(ctx, runtimeports.InspectCurrentOperationSettlementRequestV4{Operation: out.domain.Operation, EffectID: out.domain.Attempt.EffectID})
	if err != nil {
		return out, err
	}
	out.bundle, err = s.settlements.InspectOperationSettlementClosureV4(ctx, runtimeports.InspectOperationSettlementRequestV4{Operation: out.domain.Operation, SettlementID: out.inspection.Settlement.ID})
	return out, err
}

func (c mcpConnectApplyClosureV1) validateV1(now time.Time) error {
	if c.domain.Validate() != nil || c.domainNow.Validate(now) != nil || c.connection.Validate() != nil || c.inspection.Validate(now) != nil || c.bundle.Validate() != nil || c.domainNow.Fact != c.domain.ObjectRef() || c.domainNow.Connection != c.domain.Connection || c.connection.Ref != c.domain.Connection {
		return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "MCP Connect Apply closure is invalid")
	}
	expected := mcpConnectRuntimeDomainResultRefV1(c.connection.Provider, c.domain)
	if !runtimeports.SameOperationSettlementDomainResultFactRefV4(expected, c.inspection.DomainResult) || !runtimeports.SameOperationSettlementDomainResultFactRefV4(expected, c.bundle.Settlement.Submission.DomainResult) || !runtimeports.SameOperationSettlementRefV4(c.inspection.Settlement, c.bundle.Settlement.RefV4()) || !runtimeports.SameOperationSettlementEvidenceAssociationRefV4(c.inspection.Association, c.bundle.Association.RefV4()) || !runtimeports.SameOperationSettlementTerminalGuardRefV4(c.inspection.Guard, c.bundle.Guard.RefV4()) || !runtimeports.SameOperationSettlementTerminalProjectionRefV4(c.inspection.Projection, c.bundle.Projection.RefV4()) || c.inspection.Owner != c.domain.Owner || c.bundle.Settlement.Submission.Owner != c.domain.Owner {
		return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "Runtime V4 settlement does not close the exact MCP Connect DomainResult")
	}
	if c.bundle.Association.Prepare.Consumption != c.domain.PrepareConsumption || c.bundle.Association.Execute.Consumption != c.domain.ExecuteConsumption || c.bundle.Association.Prepare.EnforcementPhase != c.domain.PrepareEnforcement || c.bundle.Association.Execute.EnforcementPhase != c.domain.ExecuteEnforcement || c.bundle.Association.Prepare.Attempt != c.domain.Attempt || c.bundle.Association.Execute.Attempt != c.domain.Attempt {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Runtime V4 settlement Evidence does not close MCP Connect prepare and execute")
	}
	return nil
}

func mcpConnectRuntimeDomainResultRefV1(owner runtimeports.ProviderBindingRefV2, fact toolcontract.MCPConnectDomainResultFactV1) runtimeports.OperationSettlementDomainResultFactRefV4 {
	return runtimeports.OperationSettlementDomainResultFactRefV4{Owner: owner, Kind: toolcontract.MCPConnectDomainResultRuntimeKindV1, ID: fact.ID, Revision: fact.Revision, Digest: fact.Digest, TenantID: fact.TenantID, EffectID: fact.Attempt.EffectID, EffectRevision: fact.Attempt.IntentRevision, Operation: fact.Operation, OperationDigest: fact.Attempt.OperationDigest, Attempt: fact.Attempt, Schema: fact.Schema, PayloadDigest: fact.PayloadDigest, PayloadRevision: fact.PayloadRevision, AuthoritativeTime: fact.CreatedUnixNano}
}

func (s *MCPConnectApplyStoreV1) freshApplyV1(previous time.Time) (time.Time, error) {
	now := s.clock()
	if now.IsZero() || !previous.IsZero() && now.Before(previous) {
		return time.Time{}, core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "MCP Connect Apply clock regressed")
	}
	return now, nil
}

var _ toolcontract.MCPConnectApplySettlementExactReaderV1 = (*MCPConnectApplyStoreV1)(nil)
var _ toolcontract.MCPConnectionAvailabilityCurrentReaderV1 = (*MCPConnectApplyStoreV1)(nil)

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

type CreateMCPDiscoveryPageDomainResultRequestV1 struct {
	Command            toolcontract.ObjectRef                              `json:"command"`
	ExecuteConsumption runtimeports.OperationScopeEvidenceConsumptionRefV3 `json:"execute_consumption"`
}

type MCPDiscoveryPageDomainResultStoreV1 struct {
	mu           sync.RWMutex
	values       map[string]toolcontract.MCPDiscoveryPageDomainResultFactV1
	commands     toolcontract.MCPDiscoveryPageCommandExactReaderV1
	connections  toolcontract.MCPConnectionFactCurrentReaderV2
	physical     *InMemoryMCPDiscoveryPagePhysicalRepositoryV1
	observations MCPConnectFormalProviderObservationReaderV1
	evidence     runtimeports.OperationScopeEvidenceConsumptionClosureReaderV1
	records      MCPConnectEvidenceRecordReaderV1
	clock        func() time.Time
}

func NewMCPDiscoveryPageDomainResultStoreV1(commands toolcontract.MCPDiscoveryPageCommandExactReaderV1, connections toolcontract.MCPConnectionFactCurrentReaderV2, physical *InMemoryMCPDiscoveryPagePhysicalRepositoryV1, observations MCPConnectFormalProviderObservationReaderV1, evidence runtimeports.OperationScopeEvidenceConsumptionClosureReaderV1, records MCPConnectEvidenceRecordReaderV1, clock func() time.Time) (*MCPDiscoveryPageDomainResultStoreV1, error) {
	if nilLikeOfficialSDKConnectV1(commands) || nilLikeOfficialSDKConnectV1(connections) || physical == nil || nilLikeOfficialSDKConnectV1(observations) || nilLikeOfficialSDKConnectV1(evidence) || nilLikeOfficialSDKConnectV1(records) || clock == nil {
		return nil, invalid("MCP Discovery Page DomainResult Store dependencies are incomplete")
	}
	return &MCPDiscoveryPageDomainResultStoreV1{values: make(map[string]toolcontract.MCPDiscoveryPageDomainResultFactV1), commands: commands, connections: connections, physical: physical, observations: observations, evidence: evidence, records: records, clock: clock}, nil
}

func (s *MCPDiscoveryPageDomainResultStoreV1) CreateMCPDiscoveryPageDomainResultV1(ctx context.Context, request CreateMCPDiscoveryPageDomainResultRequestV1) (toolcontract.MCPDiscoveryPageDomainResultFactV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPDiscoveryPageDomainResultFactV1{}, err
	}
	if s == nil || request.Command.Validate() != nil || request.ExecuteConsumption.Validate() != nil {
		return toolcontract.MCPDiscoveryPageDomainResultFactV1{}, invalid("MCP Discovery Page DomainResult request is invalid")
	}
	previous, err := s.freshDiscoveryDomainV1(time.Time{})
	if err != nil {
		return toolcontract.MCPDiscoveryPageDomainResultFactV1{}, err
	}
	c1, err := s.inspectDiscoveryDomainClosureV1(ctx, request)
	if err != nil {
		return toolcontract.MCPDiscoveryPageDomainResultFactV1{}, err
	}
	afterS1, err := s.freshDiscoveryDomainV1(previous)
	if err != nil || c1.validate(afterS1) != nil {
		if err != nil {
			return toolcontract.MCPDiscoveryPageDomainResultFactV1{}, err
		}
		return toolcontract.MCPDiscoveryPageDomainResultFactV1{}, c1.validate(afterS1)
	}
	c2, err := s.inspectDiscoveryDomainClosureV1(ctx, request)
	if err != nil {
		return toolcontract.MCPDiscoveryPageDomainResultFactV1{}, err
	}
	actual, err := s.freshDiscoveryDomainV1(afterS1)
	if err != nil {
		return toolcontract.MCPDiscoveryPageDomainResultFactV1{}, err
	}
	if !reflect.DeepEqual(c1, c2) {
		return toolcontract.MCPDiscoveryPageDomainResultFactV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery Page DomainResult closure drifted between S1 and S2")
	}
	if err = c2.validate(actual); err != nil {
		return toolcontract.MCPDiscoveryPageDomainResultFactV1{}, err
	}
	schema := runtimeports.SchemaRefV2{Namespace: "praxis.tool-mcp", Name: "mcp-discovery-page-result", Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes([]byte(`{"type":"object","title":"MCP Discovery Page Result"}`))}
	fact, err := toolcontract.SealMCPDiscoveryPageDomainResultFactV1(toolcontract.MCPDiscoveryPageDomainResultFactV1{TenantID: c2.authorization.Operation.ExecutionScope.Identity.TenantID, Operation: c2.authorization.Operation, OperationScopeDigest: c2.authorization.OperationScopeDigest, Connection: c2.connection.Ref, Command: c2.command.Ref, ProtocolReceipt: c2.receipt.Ref, Namespace: c2.command.Namespace, CursorDigest: c2.command.CursorDigest, PageOrdinal: c2.command.PageOrdinal, PreparedAttempt: c2.authorization.Prepared, Attempt: c2.authorization.Attempt, Observation: c2.observation, PrepareEnforcement: c2.prepareHandoff.Phase, ExecuteEnforcement: c2.executeHandoff.Phase, PrepareConsumption: c2.prepareConsumption.RefV3(), ExecuteConsumption: c2.executeConsumption.RefV3(), Schema: schema, PayloadDigest: c2.receipt.ResponsePageDigest, PayloadRevision: 1, Owner: c2.command.Owner, Outcome: toolcontract.ToolOutcomeSucceededV2, Disposition: toolcontract.ToolDispositionConfirmedAppliedV2, CreatedUnixNano: actual.UnixNano()})
	if err != nil {
		return toolcontract.MCPDiscoveryPageDomainResultFactV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return toolcontract.MCPDiscoveryPageDomainResultFactV1{}, err
	}
	if winner, ok := s.values[fact.ID]; ok {
		if !reflect.DeepEqual(winner, fact) {
			return toolcontract.MCPDiscoveryPageDomainResultFactV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "MCP Discovery Page DomainResult ID binds another fact")
		}
		return winner, nil
	}
	s.values[fact.ID] = fact
	return fact, nil
}

func (s *MCPDiscoveryPageDomainResultStoreV1) InspectMCPDiscoveryPageDomainResultV1(ctx context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPDiscoveryPageDomainResultFactV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPDiscoveryPageDomainResultFactV1{}, err
	}
	s.mu.RLock()
	fact, ok := s.values[exact.ID]
	s.mu.RUnlock()
	if !ok {
		return fact, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Discovery Page DomainResult not found")
	}
	if fact.ObjectRef() != exact {
		return fact, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery Page DomainResult exact Ref drifted")
	}
	return fact, nil
}

func (s *MCPDiscoveryPageDomainResultStoreV1) InspectCurrentMCPDiscoveryPageDomainResultV1(ctx context.Context, exact toolcontract.ObjectRef, ttl time.Duration) (toolcontract.MCPDiscoveryPageDomainResultCurrentProjectionV1, error) {
	fact, err := s.InspectMCPDiscoveryPageDomainResultV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPDiscoveryPageDomainResultCurrentProjectionV1{}, err
	}
	if ttl <= 0 || ttl > toolcontract.MaxMCPDiscoveryPageDomainResultCurrentTTLV1 {
		return toolcontract.MCPDiscoveryPageDomainResultCurrentProjectionV1{}, invalid("MCP Discovery Page DomainResult current TTL is invalid")
	}
	now := s.clock()
	if now.IsZero() || now.UnixNano() < fact.CreatedUnixNano {
		return toolcontract.MCPDiscoveryPageDomainResultCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Discovery Page DomainResult clock regressed")
	}
	closure, err := s.inspectDiscoveryDomainClosureV1(ctx, CreateMCPDiscoveryPageDomainResultRequestV1{Command: fact.Command, ExecuteConsumption: fact.ExecuteConsumption})
	if err != nil || closure.validate(now) != nil {
		if err != nil {
			return toolcontract.MCPDiscoveryPageDomainResultCurrentProjectionV1{}, err
		}
		return toolcontract.MCPDiscoveryPageDomainResultCurrentProjectionV1{}, closure.validate(now)
	}
	expires := now.Add(ttl).UnixNano()
	for _, bound := range []int64{closure.connection.ExpiresUnixNano, closure.prepareQualification.ExpiresUnixNano, closure.executeQualification.ExpiresUnixNano, closure.prepareHandoff.NotAfterUnixNano, closure.executeHandoff.NotAfterUnixNano} {
		if bound < expires {
			expires = bound
		}
	}
	return toolcontract.SealMCPDiscoveryPageDomainResultCurrentProjectionV1(toolcontract.MCPDiscoveryPageDomainResultCurrentProjectionV1{Fact: exact, Command: fact.Command, ProtocolReceipt: fact.ProtocolReceipt, Observation: fact.Observation, PrepareEnforcement: fact.PrepareEnforcement, ExecuteEnforcement: fact.ExecuteEnforcement, PrepareConsumption: fact.PrepareConsumption, ExecuteConsumption: fact.ExecuteConsumption, Owner: fact.Owner, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires}, now)
}

type mcpDiscoveryPageDomainClosureV1 struct {
	command              toolcontract.MCPDiscoveryPageCommandV1
	connection           toolcontract.MCPConnectionFactV2
	entry                MCPDiscoveryPagePhysicalEntryV1
	receipt              toolcontract.MCPDiscoveryPageProtocolReceiptV1
	authorization        runtimeports.ControlledMCPDiscoveryPagePhysicalAuthorizationV1
	observation          runtimeports.ProviderAttemptObservationRefV2
	prepareConsumption   runtimeports.OperationScopeEvidenceConsumptionFactV3
	prepareQualification runtimeports.OperationScopeEvidenceQualificationFactV3
	prepareHandoff       runtimeports.OperationScopeEvidenceProviderHandoffFactV3
	prepareRecord        runtimeports.OperationScopeEvidenceRecordV3
	executeConsumption   runtimeports.OperationScopeEvidenceConsumptionFactV3
	executeQualification runtimeports.OperationScopeEvidenceQualificationFactV3
	executeHandoff       runtimeports.OperationScopeEvidenceProviderHandoffFactV3
	executeRecord        runtimeports.OperationScopeEvidenceRecordV3
}

func (s *MCPDiscoveryPageDomainResultStoreV1) inspectDiscoveryDomainClosureV1(ctx context.Context, request CreateMCPDiscoveryPageDomainResultRequestV1) (mcpDiscoveryPageDomainClosureV1, error) {
	var c mcpDiscoveryPageDomainClosureV1
	var err error
	c.command, err = s.commands.InspectMCPDiscoveryPageCommandV1(ctx, request.Command)
	if err != nil {
		return c, err
	}
	c.connection, err = s.connections.InspectCurrentMCPConnectionFactV2(ctx, c.command.Connection)
	if err != nil {
		return c, err
	}
	c.entry, err = s.physical.InspectMCPDiscoveryPagePhysicalByCommandV1(ctx, c.command.Ref)
	if err != nil {
		return c, err
	}
	if c.entry.ProtocolReceipt == nil {
		return c, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectUnknownOutcome, "MCP Discovery Page has no settled Receipt")
	}
	c.receipt, err = s.physical.InspectMCPDiscoveryPageProtocolReceiptV1(ctx, c.entry.ProtocolReceipt.Ref)
	if err != nil {
		return c, err
	}
	c.authorization = c.entry.Authorization
	if c.authorization.Attempt.Delegation == nil {
		return c, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery Page Attempt has no Delegation")
	}
	c.observation, err = s.observations.InspectOperationProviderReceiptObservationV1(ctx, *c.authorization.Attempt.Delegation, c.authorization.Prepared.ID)
	if err != nil {
		return c, err
	}
	c.prepareConsumption, c.prepareQualification, c.prepareHandoff, err = s.evidence.InspectOperationScopeEvidenceConsumptionClosureV1(ctx, c.authorization.PrepareConsumption)
	if err != nil {
		return c, err
	}
	c.prepareRecord, err = s.records.InspectOperationScopeEvidenceRecordV3(ctx, c.prepareConsumption.Record)
	if err != nil {
		return c, err
	}
	c.executeConsumption, c.executeQualification, c.executeHandoff, err = s.evidence.InspectOperationScopeEvidenceConsumptionClosureV1(ctx, request.ExecuteConsumption)
	if err != nil {
		return c, err
	}
	c.executeRecord, err = s.records.InspectOperationScopeEvidenceRecordV3(ctx, c.executeConsumption.Record)
	return c, err
}

func (c mcpDiscoveryPageDomainClosureV1) validate(now time.Time) error {
	if c.command.Validate() != nil || c.connection.Validate() != nil || c.entry.State != MCPDiscoveryPagePhysicalObservedV1 || c.receipt.Validate() != nil || c.authorization.Validate() != nil || c.observation.Validate() != nil {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP Discovery Page DomainResult closure is invalid")
	}
	if c.entry.Command != c.command.Ref || c.receipt.Command != c.command.Ref || c.authorization.DomainCommand != c.command.RuntimeDomainCommandRefV1() || c.authorization.ConnectionAvailability != c.command.Availability || c.authorization.Namespace != c.command.Namespace || c.authorization.CursorDigest != c.command.CursorDigest || c.authorization.PageOrdinal != c.command.PageOrdinal || c.observation.ProviderOperationRef != c.receipt.Ref.ID || c.observation.PayloadDigest != c.receipt.ResponsePageDigest || c.observation.PreparedAttemptID != c.authorization.Prepared.ID {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP Discovery Page Receipt or Observation drifted")
	}
	if now.IsZero() || !now.Before(time.Unix(0, c.connection.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "MCP Discovery Page DomainResult closure expired")
	}
	if err := validateMCPDiscoveryPageEvidenceV1(c.prepareConsumption, c.prepareQualification, c.prepareHandoff, c.prepareRecord, runtimeports.OperationDispatchEnforcementPrepareV4, c.authorization, nil, now); err != nil {
		return err
	}
	return validateMCPDiscoveryPageEvidenceV1(c.executeConsumption, c.executeQualification, c.executeHandoff, c.executeRecord, runtimeports.OperationDispatchEnforcementExecuteV4, c.authorization, &c.observation, now)
}

func validateMCPDiscoveryPageEvidenceV1(consumption runtimeports.OperationScopeEvidenceConsumptionFactV3, qualification runtimeports.OperationScopeEvidenceQualificationFactV3, handoff runtimeports.OperationScopeEvidenceProviderHandoffFactV3, record runtimeports.OperationScopeEvidenceRecordV3, phase runtimeports.OperationDispatchEnforcementPhaseV4, authorization runtimeports.ControlledMCPDiscoveryPagePhysicalAuthorizationV1, observation *runtimeports.ProviderAttemptObservationRefV2, now time.Time) error {
	if consumption.Validate() != nil || qualification.Validate() != nil || handoff.Validate() != nil || record.Validate() != nil || consumption.Qualification != handoff.Qualification || qualification.ID != consumption.Qualification.ID || qualification.Revision != consumption.Qualification.Revision+1 || qualification.State != runtimeports.OperationScopeEvidenceConsumedCurrentV3 || qualification.Consumption == nil || *qualification.Consumption != consumption.RefV3() || consumption.Handoff != handoff.RefV3() || consumption.Record != record.Ref || handoff.Phase.Phase != phase || handoff.Phase.AttemptID != authorization.Attempt.AttemptID || qualification.Scope.EffectKind != runtimeports.OperationScopeEvidenceMCPDiscoveryPageEffectKindV1 || !now.Before(time.Unix(0, qualification.ExpiresUnixNano)) || !now.Before(time.Unix(0, handoff.NotAfterUnixNano)) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP Discovery Page Evidence closure drifted")
	}
	if phase == runtimeports.OperationDispatchEnforcementPrepareV4 {
		if consumption.RefV3() != authorization.PrepareConsumption || observation != nil {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP Discovery prepare Evidence drifted")
		}
		return nil
	}
	if handoff.RefV3() != authorization.ExecuteHandoff || observation == nil || record.Candidate.EventID != observation.ProviderOperationRef || record.Candidate.CorrelationID != observation.PreparedAttemptID || record.Candidate.Payload.ContentDigest != observation.PayloadDigest || record.Candidate.Payload.Revision != observation.PayloadRevision || record.Candidate.ObservedUnixNano != observation.ObservedUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP Discovery execute Evidence lacks the exact Provider Observation")
	}
	return nil
}

func (s *MCPDiscoveryPageDomainResultStoreV1) freshDiscoveryDomainV1(previous time.Time) (time.Time, error) {
	now := s.clock()
	if now.IsZero() || !previous.IsZero() && now.Before(previous) {
		return time.Time{}, core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "MCP Discovery Page DomainResult clock regressed")
	}
	return now, nil
}

var _ toolcontract.MCPDiscoveryPageDomainResultExactReaderV1 = (*MCPDiscoveryPageDomainResultStoreV1)(nil)

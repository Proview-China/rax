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

type MCPConnectFormalProviderObservationReaderV1 interface {
	InspectOperationProviderReceiptObservationV1(context.Context, runtimeports.ExecutionDelegationRefV2, string) (runtimeports.ProviderAttemptObservationRefV2, error)
}

type MCPConnectEvidenceRecordReaderV1 interface {
	InspectOperationScopeEvidenceRecordV3(context.Context, runtimeports.OperationScopeEvidenceRecordRefV3) (runtimeports.OperationScopeEvidenceRecordV3, error)
}

type CreateMCPConnectDomainResultRequestV1 struct {
	Connection         toolcontract.MCPConnectionFactRefV2                 `json:"connection"`
	ExecuteConsumption runtimeports.OperationScopeEvidenceConsumptionRefV3 `json:"execute_consumption"`
}

type MCPConnectDomainResultStoreV1 struct {
	mu           sync.RWMutex
	values       map[string]toolcontract.MCPConnectDomainResultFactV1
	connections  toolcontract.MCPConnectionFactCurrentReaderV2
	intents      MCPConnectIntentReaderV1
	receipts     toolcontract.MCPConnectProtocolReceiptIDReaderV1
	physical     *InMemoryMCPConnectPhysicalRepositoryV1
	observations MCPConnectFormalProviderObservationReaderV1
	evidence     runtimeports.OperationScopeEvidenceConsumptionClosureReaderV1
	records      MCPConnectEvidenceRecordReaderV1
	clock        func() time.Time
}

func NewMCPConnectDomainResultStoreV1(connections toolcontract.MCPConnectionFactCurrentReaderV2, intents MCPConnectIntentReaderV1, receipts toolcontract.MCPConnectProtocolReceiptIDReaderV1, physical *InMemoryMCPConnectPhysicalRepositoryV1, observations MCPConnectFormalProviderObservationReaderV1, evidence runtimeports.OperationScopeEvidenceConsumptionClosureReaderV1, records MCPConnectEvidenceRecordReaderV1, clock func() time.Time) (*MCPConnectDomainResultStoreV1, error) {
	if nilLikeOfficialSDKConnectV1(connections) || nilLikeOfficialSDKConnectV1(intents) || nilLikeOfficialSDKConnectV1(receipts) || physical == nil || nilLikeOfficialSDKConnectV1(observations) || nilLikeOfficialSDKConnectV1(evidence) || nilLikeOfficialSDKConnectV1(records) || clock == nil {
		return nil, invalid("MCP Connect DomainResult Store V1 dependencies are incomplete")
	}
	return &MCPConnectDomainResultStoreV1{values: make(map[string]toolcontract.MCPConnectDomainResultFactV1), connections: connections, intents: intents, receipts: receipts, physical: physical, observations: observations, evidence: evidence, records: records, clock: clock}, nil
}

func (s *MCPConnectDomainResultStoreV1) CreateMCPConnectDomainResultV1(ctx context.Context, request CreateMCPConnectDomainResultRequestV1) (toolcontract.MCPConnectDomainResultFactV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPConnectDomainResultFactV1{}, err
	}
	if s == nil || nilLikeOfficialSDKConnectV1(s.connections) || nilLikeOfficialSDKConnectV1(s.intents) || nilLikeOfficialSDKConnectV1(s.receipts) || s.physical == nil || nilLikeOfficialSDKConnectV1(s.observations) || nilLikeOfficialSDKConnectV1(s.evidence) || nilLikeOfficialSDKConnectV1(s.records) || s.clock == nil {
		return toolcontract.MCPConnectDomainResultFactV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Connect DomainResult Store V1 is unavailable")
	}
	if request.Connection.Validate() != nil || request.ExecuteConsumption.Validate() != nil {
		return toolcontract.MCPConnectDomainResultFactV1{}, invalid("MCP Connect DomainResult create request is invalid")
	}
	previous, err := s.freshV1(time.Time{})
	if err != nil {
		return toolcontract.MCPConnectDomainResultFactV1{}, err
	}
	closure, err := s.inspectClosureV1(ctx, request)
	if err != nil {
		return toolcontract.MCPConnectDomainResultFactV1{}, err
	}
	afterS1, err := s.freshV1(previous)
	if err != nil {
		return toolcontract.MCPConnectDomainResultFactV1{}, err
	}
	if err = closure.validateV1(afterS1); err != nil {
		return toolcontract.MCPConnectDomainResultFactV1{}, err
	}
	closure2, err := s.inspectClosureV1(ctx, request)
	if err != nil {
		return toolcontract.MCPConnectDomainResultFactV1{}, err
	}
	actual, err := s.freshV1(afterS1)
	if err != nil {
		return toolcontract.MCPConnectDomainResultFactV1{}, err
	}
	if !reflect.DeepEqual(closure, closure2) {
		return toolcontract.MCPConnectDomainResultFactV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Connect DomainResult closure drifted between S1 and S2")
	}
	if err = closure2.validateV1(actual); err != nil {
		return toolcontract.MCPConnectDomainResultFactV1{}, err
	}
	schema := runtimeports.SchemaRefV2{Namespace: "praxis.tool-mcp", Name: "mcp-connection-fact", Version: "2.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes([]byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","title":"MCP Connection Fact V2","type":"object"}`))}
	fact, err := toolcontract.SealMCPConnectDomainResultFactV1(toolcontract.MCPConnectDomainResultFactV1{
		TenantID: core.TenantID(closure2.connection.Coordinate.TenantID), Operation: closure2.authorization.Operation, OperationScopeDigest: closure2.authorization.OperationScopeDigest,
		Connection: closure2.connection.Ref, Intent: closure2.intent.Ref, ProtocolReceipt: closure2.receipt.Ref,
		PreparedAttempt: closure2.authorization.Prepared, Attempt: closure2.authorization.Attempt, Observation: closure2.observation,
		PrepareEnforcement: closure2.prepareHandoff.Phase, ExecuteEnforcement: closure2.executeHandoff.Phase,
		PrepareConsumption: closure2.prepareConsumption.RefV3(), ExecuteConsumption: closure2.executeConsumption.RefV3(),
		Schema: schema, PayloadDigest: closure2.connection.Ref.Digest, PayloadRevision: 1,
		Owner: closure2.intent.Owner, Outcome: toolcontract.ToolOutcomeSucceededV2, Disposition: toolcontract.ToolDispositionConfirmedAppliedV2,
		CreatedUnixNano: actual.UnixNano(),
	})
	if err != nil {
		return toolcontract.MCPConnectDomainResultFactV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return toolcontract.MCPConnectDomainResultFactV1{}, err
	}
	if winner, ok := s.values[fact.ID]; ok {
		if !reflect.DeepEqual(winner, fact) {
			return toolcontract.MCPConnectDomainResultFactV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "MCP Connect DomainResult ID binds another fact")
		}
		return winner, nil
	}
	s.values[fact.ID] = fact
	return fact, nil
}

func (s *MCPConnectDomainResultStoreV1) InspectMCPConnectDomainResultV1(ctx context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPConnectDomainResultFactV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPConnectDomainResultFactV1{}, err
	}
	if s == nil || exact.Validate() != nil {
		return toolcontract.MCPConnectDomainResultFactV1{}, invalid("MCP Connect DomainResult exact Inspect is invalid")
	}
	s.mu.RLock()
	fact, ok := s.values[exact.ID]
	s.mu.RUnlock()
	if !ok {
		return toolcontract.MCPConnectDomainResultFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Connect DomainResult not found")
	}
	if fact.ObjectRef() != exact {
		return toolcontract.MCPConnectDomainResultFactV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Connect DomainResult exact Ref drifted")
	}
	return fact, nil
}

func (s *MCPConnectDomainResultStoreV1) InspectCurrentMCPConnectDomainResultV1(ctx context.Context, exact toolcontract.ObjectRef, ttl time.Duration) (toolcontract.MCPConnectDomainResultCurrentProjectionV1, error) {
	fact, err := s.InspectMCPConnectDomainResultV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPConnectDomainResultCurrentProjectionV1{}, err
	}
	if ttl <= 0 || ttl > toolcontract.MaxMCPConnectDomainResultCurrentTTLV1 || s.clock == nil {
		return toolcontract.MCPConnectDomainResultCurrentProjectionV1{}, invalid("MCP Connect DomainResult current lease request is invalid")
	}
	now := s.clock()
	if now.IsZero() || now.UnixNano() < fact.CreatedUnixNano {
		return toolcontract.MCPConnectDomainResultCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Connect DomainResult current clock regressed")
	}
	connection, err := s.connections.InspectCurrentMCPConnectionFactV2(ctx, fact.Connection)
	if err != nil {
		return toolcontract.MCPConnectDomainResultCurrentProjectionV1{}, err
	}
	expires := now.Add(ttl).UnixNano()
	if connection.ExpiresUnixNano < expires {
		expires = connection.ExpiresUnixNano
	}
	return toolcontract.SealMCPConnectDomainResultCurrentProjectionV1(toolcontract.MCPConnectDomainResultCurrentProjectionV1{Fact: exact, Connection: fact.Connection, Observation: fact.Observation, PrepareEnforcement: fact.PrepareEnforcement, ExecuteEnforcement: fact.ExecuteEnforcement, PrepareConsumption: fact.PrepareConsumption, ExecuteConsumption: fact.ExecuteConsumption, Owner: fact.Owner, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires}, now)
}

func (s *MCPConnectDomainResultStoreV1) freshV1(previous time.Time) (time.Time, error) {
	now := s.clock()
	if now.IsZero() || !previous.IsZero() && now.Before(previous) {
		return time.Time{}, core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "MCP Connect DomainResult clock regressed")
	}
	return now, nil
}

type mcpConnectDomainClosureV1 struct {
	connection           toolcontract.MCPConnectionFactV2
	receipt              toolcontract.MCPConnectProtocolReceiptV1
	entry                MCPConnectPhysicalEntryV1
	authorization        runtimeports.ControlledMCPConnectPhysicalAuthorizationV1
	intent               toolcontract.MCPConnectIntentV1
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

func (s *MCPConnectDomainResultStoreV1) inspectClosureV1(ctx context.Context, request CreateMCPConnectDomainResultRequestV1) (mcpConnectDomainClosureV1, error) {
	var out mcpConnectDomainClosureV1
	var err error
	out.connection, err = s.connections.InspectCurrentMCPConnectionFactV2(ctx, request.Connection)
	if err != nil {
		return out, err
	}
	out.receipt, err = s.receipts.InspectMCPConnectProtocolReceiptByIDV1(ctx, out.connection.ProtocolReceipt.ID)
	if err != nil {
		return out, err
	}
	out.entry, err = s.physical.InspectMCPConnectPhysicalV1(ctx, out.receipt.StableKeyDigest)
	if err != nil {
		return out, err
	}
	out.authorization = out.entry.Authorization
	out.intent, err = s.intents.InspectMCPConnectIntentV1(ctx, out.connection.Intent)
	if err != nil {
		return out, err
	}
	currentIntent, err := s.intents.InspectCurrentMCPConnectIntentV1(ctx, out.intent.Ref.ID)
	if err != nil {
		return out, err
	}
	if !reflect.DeepEqual(out.intent, currentIntent) {
		return out, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Connect Intent is no longer current during DomainResult construction")
	}
	if out.authorization.Attempt.Delegation == nil {
		return out, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Connect Attempt has no current Delegation")
	}
	out.observation, err = s.observations.InspectOperationProviderReceiptObservationV1(ctx, *out.authorization.Attempt.Delegation, out.authorization.Prepared.ID)
	if err != nil {
		return out, err
	}
	out.prepareConsumption, out.prepareQualification, out.prepareHandoff, err = s.evidence.InspectOperationScopeEvidenceConsumptionClosureV1(ctx, out.authorization.PrepareConsumption)
	if err != nil {
		return out, err
	}
	out.prepareRecord, err = s.records.InspectOperationScopeEvidenceRecordV3(ctx, out.prepareConsumption.Record)
	if err != nil {
		return out, err
	}
	out.executeConsumption, out.executeQualification, out.executeHandoff, err = s.evidence.InspectOperationScopeEvidenceConsumptionClosureV1(ctx, request.ExecuteConsumption)
	if err != nil {
		return out, err
	}
	out.executeRecord, err = s.records.InspectOperationScopeEvidenceRecordV3(ctx, out.executeConsumption.Record)
	return out, err
}

func (c mcpConnectDomainClosureV1) validateV1(now time.Time) error {
	if c.connection.Validate() != nil || c.receipt.Validate() != nil || c.entry.State != MCPConnectPhysicalObservedV1 || c.entry.ProtocolReceipt == nil || c.entry.ProtocolReceipt.Ref != c.receipt.Ref || c.authorization.Validate() != nil || c.intent.Validate() != nil || c.observation.Validate() != nil || c.prepareConsumption.Validate() != nil || c.prepareQualification.Validate() != nil || c.prepareHandoff.Validate() != nil || c.prepareRecord.Validate() != nil || c.executeConsumption.Validate() != nil || c.executeQualification.Validate() != nil || c.executeHandoff.Validate() != nil || c.executeRecord.Validate() != nil {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP Connect DomainResult closure is invalid")
	}
	if now.IsZero() || now.UnixNano() < c.connection.CreatedUnixNano || !now.Before(time.Unix(0, c.connection.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "MCP Connect DomainResult closure is not current")
	}
	if c.connection.Intent != c.intent.Ref || c.connection.ProtocolReceipt != c.receipt.Ref || c.entry.Intent != c.intent.Ref || c.authorization.DomainCommand != c.intent.RuntimeDomainCommandRefV1() || c.authorization.Attempt != c.intent.Attempt || c.authorization.Prepared.AttemptID != c.intent.Attempt.AttemptID || c.observation.Delegation != *c.authorization.Attempt.Delegation || c.observation.PreparedAttemptID != c.authorization.Prepared.ID || c.observation.ProviderOperationRef != c.receipt.Ref.ID || c.observation.PayloadDigest != c.receipt.ResponseDigest || c.observation.PayloadRevision != 1 || c.observation.ObservedUnixNano != c.receipt.ObservedUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP Connect DomainResult receipt or Observation drifted")
	}
	if err := validateMCPConnectEvidenceClosureV1(c.prepareConsumption, c.prepareQualification, c.prepareHandoff, c.prepareRecord, runtimeports.OperationDispatchEnforcementPrepareV4, c.authorization, nil, now); err != nil {
		return err
	}
	return validateMCPConnectEvidenceClosureV1(c.executeConsumption, c.executeQualification, c.executeHandoff, c.executeRecord, runtimeports.OperationDispatchEnforcementExecuteV4, c.authorization, &c.observation, now)
}

func validateMCPConnectEvidenceClosureV1(consumption runtimeports.OperationScopeEvidenceConsumptionFactV3, qualification runtimeports.OperationScopeEvidenceQualificationFactV3, handoff runtimeports.OperationScopeEvidenceProviderHandoffFactV3, record runtimeports.OperationScopeEvidenceRecordV3, phase runtimeports.OperationDispatchEnforcementPhaseV4, authorization runtimeports.ControlledMCPConnectPhysicalAuthorizationV1, observation *runtimeports.ProviderAttemptObservationRefV2, now time.Time) error {
	if consumption.Qualification != handoff.Qualification || qualification.ID != consumption.Qualification.ID || qualification.Revision != consumption.Qualification.Revision+1 || qualification.State != runtimeports.OperationScopeEvidenceConsumedCurrentV3 || qualification.Consumption == nil || *qualification.Consumption != consumption.RefV3() || consumption.Handoff != handoff.RefV3() || consumption.Record != record.Ref || consumption.CandidateDigest != record.CandidateDigest || handoff.Phase.Phase != phase || handoff.Phase.AttemptID != authorization.Attempt.AttemptID || handoff.Phase.OperationDigest != authorization.OperationDigest || handoff.Phase.EffectID != authorization.EffectID || qualification.Scope.AttemptID != authorization.Attempt.AttemptID || qualification.Scope.OperationDigest != authorization.OperationDigest || qualification.Scope.EffectID != authorization.EffectID {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP Connect Evidence closure binds another Attempt or phase")
	}
	if now.IsZero() || !now.Before(time.Unix(0, qualification.ExpiresUnixNano)) || !now.Before(time.Unix(0, handoff.NotAfterUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "MCP Connect Evidence closure is no longer current")
	}
	if phase == runtimeports.OperationDispatchEnforcementPrepareV4 {
		if consumption.RefV3() != authorization.PrepareConsumption || handoff.RefV3() == authorization.ExecuteHandoff {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP Connect prepare Evidence drifted or was reused")
		}
		return nil
	}
	if handoff.RefV3() != authorization.ExecuteHandoff || consumption.RefV3() == authorization.PrepareConsumption || observation == nil || record.Candidate.EventID != observation.ProviderOperationRef || record.Candidate.CorrelationID != observation.PreparedAttemptID || record.Candidate.Payload.ContentDigest != observation.PayloadDigest || record.Candidate.Payload.Revision != observation.PayloadRevision || record.Candidate.ObservedUnixNano != observation.ObservedUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP Connect execute Evidence does not come from the formal Observation")
	}
	return nil
}

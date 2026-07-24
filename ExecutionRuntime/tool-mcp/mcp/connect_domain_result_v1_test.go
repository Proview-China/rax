package mcp

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

func TestMCPConnectDomainResultStoreV1CreatesAuthoritativeFact(t *testing.T) {
	f := newMCPConnectDomainResultFixtureV1(t)
	fact, err := f.store.CreateMCPConnectDomainResultV1(context.Background(), f.request)
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.store.CreateMCPConnectDomainResultV1(context.Background(), f.request)
	if err != nil {
		t.Fatal(err)
	}
	if fact.ObjectRef() != again.ObjectRef() || fact.Connection != f.connection.Ref || fact.Observation != f.observation || fact.PrepareConsumption != f.prepare.consumption.RefV3() || fact.ExecuteConsumption != f.execute.consumption.RefV3() {
		t.Fatalf("MCP Connect DomainResult lost exact closure: %#v", fact)
	}
	current, err := f.store.InspectCurrentMCPConnectDomainResultV1(context.Background(), fact.ObjectRef(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if current.Fact != fact.ObjectRef() || current.Connection != fact.Connection || current.Observation != fact.Observation {
		t.Fatalf("MCP Connect DomainResult current projection drifted: %#v", current)
	}
}

func TestMCPConnectDomainResultStoreV1ConcurrentSingleWinner(t *testing.T) {
	f := newMCPConnectDomainResultFixtureV1(t)
	const workers = 64
	refs := make(chan toolcontract.ObjectRef, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fact, err := f.store.CreateMCPConnectDomainResultV1(context.Background(), f.request)
			refs <- fact.ObjectRef()
			errs <- err
		}()
	}
	wg.Wait()
	close(refs)
	close(errs)
	var winner toolcontract.ObjectRef
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for ref := range refs {
		if winner.ID == "" {
			winner = ref
		} else if ref != winner {
			t.Fatalf("concurrent DomainResult winners drifted: %#v != %#v", ref, winner)
		}
	}
}

func TestMCPConnectDomainResultStoreV1FailsClosed(t *testing.T) {
	t.Run("nil_context", func(t *testing.T) {
		f := newMCPConnectDomainResultFixtureV1(t)
		if _, err := f.store.CreateMCPConnectDomainResultV1(nil, f.request); !core.HasCategory(err, core.ErrorInvalidArgument) {
			t.Fatalf("nil context error=%v", err)
		}
	})

	t.Run("execute_observation_drift", func(t *testing.T) {
		f := newMCPConnectDomainResultFixtureV1(t)
		drifted := f.execute.record
		drifted.Candidate.EventID = "another-provider-receipt"
		var err error
		drifted, err = runtimeports.SealOperationScopeEvidenceRecordV3(drifted)
		if err != nil {
			t.Fatal(err)
		}
		f.evidence.records[f.execute.record.Ref] = drifted
		if _, err = f.store.CreateMCPConnectDomainResultV1(context.Background(), f.request); !core.HasReason(err, core.ReasonEvidenceConflict) {
			t.Fatalf("execute Observation drift error=%v", err)
		}
	})

	t.Run("prepare_execute_reuse", func(t *testing.T) {
		f := newMCPConnectDomainResultFixtureV1(t)
		f.request.ExecuteConsumption = f.prepare.consumption.RefV3()
		if _, err := f.store.CreateMCPConnectDomainResultV1(context.Background(), f.request); !core.HasReason(err, core.ReasonEvidenceConflict) {
			t.Fatalf("prepare/execute Evidence reuse error=%v", err)
		}
	})

	t.Run("expired_evidence", func(t *testing.T) {
		f := newMCPConnectDomainResultFixtureV1(t)
		f.now = f.now.Add(20 * time.Second)
		if _, err := f.store.CreateMCPConnectDomainResultV1(context.Background(), f.request); !core.HasReason(err, core.ReasonBindingExpired) && !core.HasReason(err, core.ReasonEvidenceUnavailable) {
			t.Fatalf("expired Evidence error=%v", err)
		}
	})

	t.Run("clock_regression", func(t *testing.T) {
		f := newMCPConnectDomainResultFixtureV1(t)
		calls := 0
		f.store.clock = func() time.Time {
			calls++
			if calls == 1 {
				return f.now
			}
			return f.now.Add(-time.Second)
		}
		if _, err := f.store.CreateMCPConnectDomainResultV1(context.Background(), f.request); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("clock regression error=%v", err)
		}
	})
}

type mcpConnectDomainResultFixtureV1 struct {
	now         time.Time
	store       *MCPConnectDomainResultStoreV1
	request     CreateMCPConnectDomainResultRequestV1
	connection  toolcontract.MCPConnectionFactV2
	connections *InMemoryMCPConnectionFactRepositoryV2
	observation runtimeports.ProviderAttemptObservationRefV2
	prepare     mcpConnectEvidenceTestFixtureV1
	execute     mcpConnectEvidenceTestFixtureV1
	evidence    *mcpConnectDomainEvidenceReaderV1
}

func newMCPConnectDomainResultFixtureV1(t *testing.T) *mcpConnectDomainResultFixtureV1 {
	t.Helper()
	base := newMCPConnectExecutorFixtureV1(t, toolcontract.MCPTransportStreamableHTTPV1, "http://127.0.0.1:8123/mcp")
	now := base.now
	intent := base.mustIntent(t)
	preparePhase := base.authorization.ExecuteEnforcement
	preparePhase.Phase = runtimeports.OperationDispatchEnforcementPrepareV4
	preparePhase.ReceiptDigest = base.authorization.ExecuteEnforcement.PrepareReceiptDigest
	preparePhase.JournalRevision = 1
	preparePhase.PrepareReceiptDigest = ""
	preparePhase.PreparedAttemptDigest = ""
	prepare := newMCPConnectEvidenceTestFixtureV1(t, now, intent, base.authorization.Attempt, preparePhase, nil, "prepare")
	executeAdmission := newMCPConnectEvidenceTestFixtureV1(t, now, intent, base.authorization.Attempt, base.authorization.ExecuteEnforcement, nil, "execute")
	base.authorization.PrepareConsumption = prepare.consumption.RefV3()
	base.authorization.ExecuteHandoff = executeAdmission.handoff.RefV3()
	var err error
	base.authorization, err = runtimeports.SealControlledMCPConnectPhysicalAuthorizationV1(base.authorization)
	if err != nil {
		t.Fatal(err)
	}
	executor := base.executor(t, &fakeMCPConnectDriverV1{session: newFakeConnectSessionV1()}, base.clock)
	if _, err = executor.ConnectControlledMCPV1(context.Background(), base.authorization); err != nil {
		t.Fatal(err)
	}
	entry, err := executor.InspectMCPConnectPhysicalV1(context.Background(), base.authorization.StableKeyDigest)
	if err != nil || entry.ProtocolReceipt == nil {
		t.Fatalf("MCP Connect receipt missing: %#v err=%v", entry, err)
	}
	facts, err := NewInMemoryMCPConnectionFactRepositoryV2(base.clock)
	if err != nil {
		t.Fatal(err)
	}
	inspector, err := NewMCPConnectReceiptInspectorV2(base.entries, base.intents, base.configs, base.servers, facts, base.clock)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := inspector.InspectAndCreateMCPConnectionFactV2(context.Background(), InspectMCPConnectReceiptRequestV2{Receipt: entry.ProtocolReceipt.Ref, RequestedExpiresUnixNano: now.Add(7 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	observation := runtimeports.ProviderAttemptObservationRefV2{
		Delegation: *base.authorization.Attempt.Delegation, PreparedAttemptID: base.authorization.Prepared.ID,
		ProviderOperationRef: entry.ProtocolReceipt.Ref.ID, Revision: 1, State: runtimeports.ProviderAttemptObservedV2,
		Digest: testDigestV1("mcp-connect-formal-observation"), PayloadDigest: entry.ProtocolReceipt.ResponseDigest, PayloadRevision: 1,
		SourceRegistrationID: "mcp-connect-formal-source", SourceEpoch: 1, SourceSequence: 1,
		Evidence:         runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: testDigestV1("mcp-connect-formal-ledger"), Sequence: 1, RecordDigest: testDigestV1("mcp-connect-formal-record")},
		ObservedUnixNano: entry.ProtocolReceipt.ObservedUnixNano,
	}
	if err = observation.Validate(); err != nil {
		t.Fatal(err)
	}
	execute := newMCPConnectEvidenceTestFixtureV1(t, now, intent, base.authorization.Attempt, base.authorization.ExecuteEnforcement, &observation, "execute")
	if execute.handoff.RefV3() != base.authorization.ExecuteHandoff {
		t.Fatal("execute Evidence reconstruction changed the authorization handoff")
	}
	evidence := &mcpConnectDomainEvidenceReaderV1{
		closures: map[string]mcpConnectEvidenceTestFixtureV1{prepare.consumption.ID: prepare, execute.consumption.ID: execute},
		records:  map[runtimeports.OperationScopeEvidenceRecordRefV3]runtimeports.OperationScopeEvidenceRecordV3{prepare.record.Ref: prepare.record, execute.record.Ref: execute.record},
	}
	store, err := NewMCPConnectDomainResultStoreV1(facts, base.intents, base.entries, base.entries, mcpConnectObservationReaderV1{observation}, evidence, evidence, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	fixture := &mcpConnectDomainResultFixtureV1{now: now, store: store, request: CreateMCPConnectDomainResultRequestV1{Connection: connection.Ref, ExecuteConsumption: execute.consumption.RefV3()}, connection: connection, connections: facts, observation: observation, prepare: prepare, execute: execute, evidence: evidence}
	store.clock = func() time.Time { return fixture.now }
	return fixture
}

type mcpConnectEvidenceTestFixtureV1 struct {
	consumption   runtimeports.OperationScopeEvidenceConsumptionFactV3
	qualification runtimeports.OperationScopeEvidenceQualificationFactV3
	handoff       runtimeports.OperationScopeEvidenceProviderHandoffFactV3
	record        runtimeports.OperationScopeEvidenceRecordV3
}

func newMCPConnectEvidenceTestFixtureV1(t *testing.T, now time.Time, intent toolcontract.MCPConnectIntentV1, attempt runtimeports.OperationDispatchAttemptRefV3, phase runtimeports.OperationDispatchEnforcementPhaseRefV4, observation *runtimeports.ProviderAttemptObservationRefV2, label string) mcpConnectEvidenceTestFixtureV1 {
	t.Helper()
	applicability := make([]runtimeports.OperationScopeEvidenceApplicabilityV3, 0, 5)
	for _, dimension := range []runtimeports.OperationScopeEvidenceApplicabilityDimensionV3{runtimeports.OperationScopeEvidenceRunV3, runtimeports.OperationScopeEvidenceSessionV3, runtimeports.OperationScopeEvidenceTurnV3, runtimeports.OperationScopeEvidenceActionV3, runtimeports.OperationScopeEvidenceContextV3} {
		value := runtimeports.OperationScopeEvidenceApplicabilityV3{Dimension: dimension, Mode: runtimeports.OperationScopeEvidenceForbiddenV3}
		for _, route := range runtimeports.OperationScopeEvidenceMCPConnectRoutesV1() {
			if route.Dimension == dimension {
				value.Mode = runtimeports.OperationScopeEvidenceRequiredV3
				value.Fact = &runtimeports.OperationScopeEvidenceApplicabilityFactRefV3{Kind: route.Kind, ID: "mcp-connect-" + string(dimension) + "-domain", Revision: 1, Digest: testDigestV1("mcp-connect-" + string(dimension) + "-domain")}
			}
		}
		applicability = append(applicability, value)
	}
	applicability = runtimeports.NormalizeOperationScopeEvidenceApplicabilityV3(applicability)
	policy := runtimeports.OperationScopeEvidenceApplicabilityPolicyRefV3{ID: "mcp-connect-applicability-" + label, Revision: 1, Digest: testDigestV1("mcp-connect-applicability-" + label), ExpiresUnixNano: now.Add(8 * time.Second).UnixNano()}
	scope := runtimeports.OperationScopeEvidenceScopeV3{
		LedgerScope: runtimeports.OperationScopeEvidenceLedgerScopeV3{TenantID: intent.Operation.ExecutionScope.Identity.TenantID, OperationDigest: intent.OperationDigest, ChainID: "mcp-connect-domain-chain-" + label},
		Operation:   intent.Operation, OperationDigest: intent.OperationDigest, EffectID: intent.EffectID, EffectRevision: intent.EffectRevision,
		EffectDigest: intent.IntentDigest, EffectKind: toolcontract.MCPConnectEffectKindV1, AttemptID: attempt.AttemptID, Phase: phase.Phase,
		ApplicabilityPolicy: policy, Applicability: applicability,
		Generation: runtimeports.GenerationBindingAssociationRefV1{ID: "mcp-connect-domain-generation", Revision: 1, Digest: testDigestV1("mcp-connect-domain-generation")},
	}
	runtimeCurrent, err := runtimeports.SealOperationScopeEvidenceRuntimeCurrentProjectionV3(runtimeports.OperationScopeEvidenceRuntimeCurrentProjectionV3{
		Scope: scope, PermitID: phase.PermitID, PermitFactRevision: phase.PermitFactRevision, PermitDigest: phase.PermitDigest,
		AdmissionDigest: phase.AdmissionDigest, Authorization: phase.ReviewAuthorization, Phase: phase,
		CheckedUnixNano: now.Add(-2 * time.Second).UnixNano(), ExpiresUnixNano: now.Add(6 * time.Second).UnixNano(),
	}, now.Add(-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	schema := runtimeports.SchemaRefV2{Namespace: "praxis.mcp", Name: "connect-" + label, Version: "1.0.0", MediaType: "application/json", ContentDigest: testDigestV1("mcp-connect-domain-schema-" + label)}
	registration := runtimeports.OperationScopeEvidenceFactRefV3{ID: "mcp-connect-domain-registration-" + label, Revision: 1, Digest: testDigestV1("mcp-connect-domain-registration-" + label), ExpiresUnixNano: now.Add(8 * time.Second).UnixNano()}
	source := runtimeports.OperationScopeEvidenceSourceKeyV3{RegistrationID: registration.ID, SourceEpoch: 1, SourceSequence: 1}
	issued, err := runtimeports.SealOperationScopeEvidenceQualificationFactV3(runtimeports.OperationScopeEvidenceQualificationFactV3{
		ID: "mcp-connect-domain-qualification-" + label, Revision: 1, State: runtimeports.OperationScopeEvidenceIssuedV3, Scope: scope, Runtime: runtimeCurrent,
		EvidencePolicy: runtimeports.OperationScopeEvidencePolicyRefV3{ID: "mcp-connect-domain-policy-" + label, Revision: 1, Digest: testDigestV1("mcp-connect-domain-policy-" + label), ExpiresUnixNano: now.Add(7 * time.Second).UnixNano()},
		Reservation:    runtimeports.OperationScopeEvidenceSourceReservationV3{Registration: registration, Source: source, EventID: "mcp-connect-domain-event-" + label, Schema: schema},
		RequestedTTL:   6 * time.Second, CreatedUnixNano: now.Add(-time.Second).UnixNano(), UpdatedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(6 * time.Second).UnixNano(), IngestNotAfterUnixNano: now.Add(7 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := runtimeports.SealOperationScopeEvidenceProviderHandoffFactV3(runtimeports.OperationScopeEvidenceProviderHandoffFactV3{ID: "mcp-connect-domain-handoff-" + label, Revision: 1, Qualification: issued.RefV3(), Phase: phase, CheckedUnixNano: now.Add(-time.Second).UnixNano(), NotAfterUnixNano: now.Add(5 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	eventID := "mcp-connect-domain-provider-" + label
	correlationID := "mcp-connect-domain-correlation-" + label
	payloadDigest := testDigestV1("mcp-connect-domain-payload-" + label)
	observed := now.UnixNano()
	if observation != nil {
		eventID, correlationID, payloadDigest, observed = observation.ProviderOperationRef, observation.PreparedAttemptID, observation.PayloadDigest, observation.ObservedUnixNano
	}
	candidate := runtimeports.OperationScopeEvidenceCandidateV3{ContractVersion: runtimeports.OperationScopeEvidenceContractVersionV3, Qualification: issued.RefV3(), Source: source, EventID: eventID, TrustClass: runtimeports.EvidenceTrustObservation, Payload: runtimeports.EvidencePayloadRefV2{Schema: schema, ContentDigest: payloadDigest, Revision: 1, Length: 1, Ref: "evidence://mcp-connect/" + label}, Causation: []runtimeports.EvidenceCausationRefV2{}, CorrelationID: correlationID, ObservedUnixNano: observed}
	ledgerDigest, err := scope.LedgerScope.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	record, err := runtimeports.SealOperationScopeEvidenceRecordV3(runtimeports.OperationScopeEvidenceRecordV3{Ref: runtimeports.OperationScopeEvidenceRecordRefV3{LedgerScopeDigest: ledgerDigest, Sequence: 1}, Candidate: candidate, PreviousRecordDigest: runtimeports.EvidenceGenesisDigestV2, IngestedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	consumption, err := runtimeports.SealOperationScopeEvidenceConsumptionFactV3(runtimeports.OperationScopeEvidenceConsumptionFactV3{ID: "mcp-connect-domain-consumption-" + label, Revision: 1, Qualification: issued.RefV3(), Handoff: handoff.RefV3(), CandidateDigest: record.CandidateDigest, Record: record.Ref, CreatedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	final := issued
	final.Revision = 2
	final.State = runtimeports.OperationScopeEvidenceConsumedCurrentV3
	ref := consumption.RefV3()
	final.Consumption = &ref
	final.UpdatedUnixNano = now.UnixNano()
	final.Digest = ""
	final, err = runtimeports.SealOperationScopeEvidenceQualificationFactV3(final)
	if err != nil {
		t.Fatal(err)
	}
	return mcpConnectEvidenceTestFixtureV1{consumption: consumption, qualification: final, handoff: handoff, record: record}
}

type mcpConnectObservationReaderV1 struct {
	observation runtimeports.ProviderAttemptObservationRefV2
}

func (r mcpConnectObservationReaderV1) InspectOperationProviderReceiptObservationV1(_ context.Context, delegation runtimeports.ExecutionDelegationRefV2, preparedID string) (runtimeports.ProviderAttemptObservationRefV2, error) {
	if delegation != r.observation.Delegation || preparedID != r.observation.PreparedAttemptID {
		return runtimeports.ProviderAttemptObservationRefV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Connect formal Observation not found")
	}
	return r.observation, nil
}

type mcpConnectDomainEvidenceReaderV1 struct {
	closures map[string]mcpConnectEvidenceTestFixtureV1
	records  map[runtimeports.OperationScopeEvidenceRecordRefV3]runtimeports.OperationScopeEvidenceRecordV3
}

func (r *mcpConnectDomainEvidenceReaderV1) InspectOperationScopeEvidenceConsumptionClosureV1(_ context.Context, exact runtimeports.OperationScopeEvidenceConsumptionRefV3) (runtimeports.OperationScopeEvidenceConsumptionFactV3, runtimeports.OperationScopeEvidenceQualificationFactV3, runtimeports.OperationScopeEvidenceProviderHandoffFactV3, error) {
	value, ok := r.closures[exact.ID]
	if !ok || value.consumption.RefV3() != exact {
		return runtimeports.OperationScopeEvidenceConsumptionFactV3{}, runtimeports.OperationScopeEvidenceQualificationFactV3{}, runtimeports.OperationScopeEvidenceProviderHandoffFactV3{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Connect Evidence closure not found")
	}
	return value.consumption, value.qualification, value.handoff, nil
}

func (r *mcpConnectDomainEvidenceReaderV1) InspectOperationScopeEvidenceRecordV3(_ context.Context, exact runtimeports.OperationScopeEvidenceRecordRefV3) (runtimeports.OperationScopeEvidenceRecordV3, error) {
	value, ok := r.records[exact]
	if !ok {
		return runtimeports.OperationScopeEvidenceRecordV3{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Connect Evidence record not found")
	}
	return value, nil
}

var _ runtimeports.OperationScopeEvidenceConsumptionClosureReaderV1 = (*mcpConnectDomainEvidenceReaderV1)(nil)

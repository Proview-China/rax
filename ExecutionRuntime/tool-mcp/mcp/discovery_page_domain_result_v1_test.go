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

func TestMCPDiscoveryPageDomainResultStoreV1AuthoritativeFact(t *testing.T) {
	f := newMCPDiscoveryPageDomainFixtureV1(t)
	fact, err := f.store.CreateMCPDiscoveryPageDomainResultV1(context.Background(), f.request)
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.store.CreateMCPDiscoveryPageDomainResultV1(context.Background(), f.request)
	if err != nil || again.ObjectRef() != fact.ObjectRef() {
		t.Fatalf("idempotent create=%#v err=%v", again, err)
	}
	if fact.Command != f.request.Command || fact.ProtocolReceipt != f.receipt.Ref || fact.Observation != f.observation || fact.PrepareConsumption != f.prepare.consumption.RefV3() || fact.ExecuteConsumption != f.execute.consumption.RefV3() {
		t.Fatalf("DomainResult lost exact closure: %#v", fact)
	}
	current, err := f.store.InspectCurrentMCPDiscoveryPageDomainResultV1(context.Background(), fact.ObjectRef(), 3*time.Second)
	if err != nil || current.Fact != fact.ObjectRef() || current.ProtocolReceipt != fact.ProtocolReceipt {
		t.Fatalf("current=%#v err=%v", current, err)
	}
}

func TestMCPDiscoveryPageDomainResultStoreV1ConcurrentAndFailsClosed(t *testing.T) {
	t.Run("64_single_winner", func(t *testing.T) {
		f := newMCPDiscoveryPageDomainFixtureV1(t)
		const workers = 64
		refs := make(chan toolcontract.ObjectRef, workers)
		errs := make(chan error, workers)
		var group sync.WaitGroup
		for range workers {
			group.Add(1)
			go func() {
				defer group.Done()
				fact, err := f.store.CreateMCPDiscoveryPageDomainResultV1(context.Background(), f.request)
				refs <- fact.ObjectRef()
				errs <- err
			}()
		}
		group.Wait()
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
				t.Fatal("multiple DomainResult winners")
			}
		}
	})
	t.Run("observation_drift", func(t *testing.T) {
		f := newMCPDiscoveryPageDomainFixtureV1(t)
		changed := f.execute.record
		changed.Candidate.EventID = "other-receipt"
		changed, _ = runtimeports.SealOperationScopeEvidenceRecordV3(changed)
		f.evidence.records[f.execute.record.Ref] = changed
		if _, err := f.store.CreateMCPDiscoveryPageDomainResultV1(context.Background(), f.request); !core.HasReason(err, core.ReasonEvidenceConflict) {
			t.Fatalf("drift error=%v", err)
		}
	})
	t.Run("prepare_execute_reuse", func(t *testing.T) {
		f := newMCPDiscoveryPageDomainFixtureV1(t)
		f.request.ExecuteConsumption = f.prepare.consumption.RefV3()
		if _, err := f.store.CreateMCPDiscoveryPageDomainResultV1(context.Background(), f.request); !core.HasReason(err, core.ReasonEvidenceConflict) {
			t.Fatalf("reuse error=%v", err)
		}
	})
	t.Run("expired", func(t *testing.T) {
		f := newMCPDiscoveryPageDomainFixtureV1(t)
		f.now = f.now.Add(20 * time.Second)
		if _, err := f.store.CreateMCPDiscoveryPageDomainResultV1(context.Background(), f.request); err == nil {
			t.Fatal("expired closure accepted")
		}
	})
	t.Run("nil_context", func(t *testing.T) {
		f := newMCPDiscoveryPageDomainFixtureV1(t)
		if _, err := f.store.CreateMCPDiscoveryPageDomainResultV1(nil, f.request); !core.HasCategory(err, core.ErrorInvalidArgument) {
			t.Fatalf("nil context=%v", err)
		}
	})
}

type mcpDiscoveryPageDomainFixtureV1 struct {
	now              time.Time
	executorFixture  discoveryPageExecutorFixtureV1
	store            *MCPDiscoveryPageDomainResultStoreV1
	request          CreateMCPDiscoveryPageDomainResultRequestV1
	receipt          toolcontract.MCPDiscoveryPageProtocolReceiptV1
	observation      runtimeports.ProviderAttemptObservationRefV2
	prepare, execute mcpConnectEvidenceTestFixtureV1
	evidence         *mcpConnectDomainEvidenceReaderV1
}

func newMCPDiscoveryPageDomainFixtureV1(t *testing.T) *mcpDiscoveryPageDomainFixtureV1 {
	t.Helper()
	base := newDiscoveryPageExecutorFixtureV1(t, "domain")
	now := base.now
	commandRef := toolcontract.ObjectRef{ID: base.authorization.DomainCommand.ID, Revision: base.authorization.DomainCommand.Revision, Digest: base.authorization.DomainCommand.Digest}
	command, err := base.executor.commands.InspectMCPDiscoveryPageCommandV1(context.Background(), commandRef)
	if err != nil {
		t.Fatal(err)
	}
	preparePhase := base.authorization.ExecuteEnforcement
	preparePhase.Phase = runtimeports.OperationDispatchEnforcementPrepareV4
	preparePhase.ReceiptDigest = base.authorization.ExecuteEnforcement.PrepareReceiptDigest
	preparePhase.JournalRevision = 1
	preparePhase.PrepareReceiptDigest = ""
	preparePhase.PreparedAttemptDigest = ""
	prepare := newMCPDiscoveryPageEvidenceFixtureV1(t, now, command, preparePhase, nil, "prepare")
	executeAdmission := newMCPDiscoveryPageEvidenceFixtureV1(t, now, command, base.authorization.ExecuteEnforcement, nil, "execute")
	base.authorization.PrepareConsumption, base.authorization.ExecuteHandoff = prepare.consumption.RefV3(), executeAdmission.handoff.RefV3()
	base.authorization, err = runtimeports.SealControlledMCPDiscoveryPagePhysicalAuthorizationV1(base.authorization)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = base.executor.DiscoverControlledMCPPageV1(context.Background(), base.authorization); err != nil {
		t.Fatal(err)
	}
	entry, err := base.entries.InspectMCPDiscoveryPagePhysicalV1(context.Background(), base.authorization.StableKeyDigest)
	if err != nil || entry.ProtocolReceipt == nil {
		t.Fatalf("receipt missing: %v", err)
	}
	receipt := *entry.ProtocolReceipt
	observation := runtimeports.ProviderAttemptObservationRefV2{Delegation: *base.authorization.Attempt.Delegation, PreparedAttemptID: base.authorization.Prepared.ID, ProviderOperationRef: receipt.Ref.ID, Revision: 1, State: runtimeports.ProviderAttemptObservedV2, Digest: testDigestV1("discovery-page-formal-observation"), PayloadDigest: receipt.ResponsePageDigest, PayloadRevision: 1, SourceRegistrationID: "discovery-page-source", SourceEpoch: 1, SourceSequence: 1, Evidence: runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: testDigestV1("discovery-page-ledger"), Sequence: 1, RecordDigest: testDigestV1("discovery-page-record")}, ObservedUnixNano: receipt.ObservedUnixNano}
	if err = observation.Validate(); err != nil {
		t.Fatal(err)
	}
	execute := newMCPDiscoveryPageEvidenceFixtureV1(t, now, command, base.authorization.ExecuteEnforcement, &observation, "execute")
	if execute.handoff.RefV3() != base.authorization.ExecuteHandoff {
		t.Fatal("execute handoff changed")
	}
	evidence := &mcpConnectDomainEvidenceReaderV1{closures: map[string]mcpConnectEvidenceTestFixtureV1{prepare.consumption.ID: prepare, execute.consumption.ID: execute}, records: map[runtimeports.OperationScopeEvidenceRecordRefV3]runtimeports.OperationScopeEvidenceRecordV3{prepare.record.Ref: prepare.record, execute.record.Ref: execute.record}}
	store, err := NewMCPDiscoveryPageDomainResultStoreV1(base.executor.commands, base.source, base.entries, mcpConnectObservationReaderV1{observation}, evidence, evidence, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	f := &mcpDiscoveryPageDomainFixtureV1{now: now, executorFixture: base, store: store, request: CreateMCPDiscoveryPageDomainResultRequestV1{Command: command.Ref, ExecuteConsumption: execute.consumption.RefV3()}, receipt: receipt, observation: observation, prepare: prepare, execute: execute, evidence: evidence}
	store.clock = func() time.Time { return f.now }
	return f
}

func newMCPDiscoveryPageEvidenceFixtureV1(t *testing.T, now time.Time, command toolcontract.MCPDiscoveryPageCommandV1, phase runtimeports.OperationDispatchEnforcementPhaseRefV4, observation *runtimeports.ProviderAttemptObservationRefV2, label string) mcpConnectEvidenceTestFixtureV1 {
	t.Helper()
	applicability := make([]runtimeports.OperationScopeEvidenceApplicabilityV3, 0, 5)
	for _, dimension := range []runtimeports.OperationScopeEvidenceApplicabilityDimensionV3{runtimeports.OperationScopeEvidenceRunV3, runtimeports.OperationScopeEvidenceSessionV3, runtimeports.OperationScopeEvidenceTurnV3, runtimeports.OperationScopeEvidenceActionV3, runtimeports.OperationScopeEvidenceContextV3} {
		value := runtimeports.OperationScopeEvidenceApplicabilityV3{Dimension: dimension, Mode: runtimeports.OperationScopeEvidenceForbiddenV3}
		for _, route := range runtimeports.OperationScopeEvidenceMCPDiscoveryPageRoutesV1() {
			if route.Dimension == dimension {
				value.Mode = runtimeports.OperationScopeEvidenceRequiredV3
				value.Fact = &runtimeports.OperationScopeEvidenceApplicabilityFactRefV3{Kind: route.Kind, ID: "discovery-page-" + string(dimension) + "-domain", Revision: 1, Digest: testDigestV1("discovery-page-" + string(dimension) + "-domain")}
			}
		}
		applicability = append(applicability, value)
	}
	applicability = runtimeports.NormalizeOperationScopeEvidenceApplicabilityV3(applicability)
	policy := runtimeports.OperationScopeEvidenceApplicabilityPolicyRefV3{ID: "discovery-page-applicability-" + label, Revision: 1, Digest: testDigestV1("discovery-page-applicability-" + label), ExpiresUnixNano: now.Add(8 * time.Second).UnixNano()}
	scope := runtimeports.OperationScopeEvidenceScopeV3{LedgerScope: runtimeports.OperationScopeEvidenceLedgerScopeV3{TenantID: command.Operation.ExecutionScope.Identity.TenantID, OperationDigest: command.OperationDigest, ChainID: "discovery-page-chain-" + label}, Operation: command.Operation, OperationDigest: command.OperationDigest, EffectID: command.EffectID, EffectRevision: command.EffectRevision, EffectDigest: command.IntentDigest, EffectKind: runtimeports.OperationScopeEvidenceMCPDiscoveryPageEffectKindV1, AttemptID: command.Attempt.AttemptID, Phase: phase.Phase, ApplicabilityPolicy: policy, Applicability: applicability, Generation: runtimeports.GenerationBindingAssociationRefV1{ID: "discovery-page-generation", Revision: 1, Digest: testDigestV1("discovery-page-generation")}}
	runtimeCurrent, err := runtimeports.SealOperationScopeEvidenceRuntimeCurrentProjectionV3(runtimeports.OperationScopeEvidenceRuntimeCurrentProjectionV3{Scope: scope, PermitID: phase.PermitID, PermitFactRevision: phase.PermitFactRevision, PermitDigest: phase.PermitDigest, AdmissionDigest: phase.AdmissionDigest, Authorization: phase.ReviewAuthorization, Phase: phase, CheckedUnixNano: now.Add(-2 * time.Second).UnixNano(), ExpiresUnixNano: now.Add(6 * time.Second).UnixNano()}, now.Add(-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	schema := runtimeports.SchemaRefV2{Namespace: "praxis.mcp", Name: "discovery-page-" + label, Version: "1.0.0", MediaType: "application/json", ContentDigest: testDigestV1("discovery-page-schema-" + label)}
	registration := runtimeports.OperationScopeEvidenceFactRefV3{ID: "discovery-page-registration-" + label, Revision: 1, Digest: testDigestV1("discovery-page-registration-" + label), ExpiresUnixNano: now.Add(8 * time.Second).UnixNano()}
	source := runtimeports.OperationScopeEvidenceSourceKeyV3{RegistrationID: registration.ID, SourceEpoch: 1, SourceSequence: 1}
	issued, err := runtimeports.SealOperationScopeEvidenceQualificationFactV3(runtimeports.OperationScopeEvidenceQualificationFactV3{ID: "discovery-page-qualification-" + label, Revision: 1, State: runtimeports.OperationScopeEvidenceIssuedV3, Scope: scope, Runtime: runtimeCurrent, EvidencePolicy: runtimeports.OperationScopeEvidencePolicyRefV3{ID: "discovery-page-policy-" + label, Revision: 1, Digest: testDigestV1("discovery-page-policy-" + label), ExpiresUnixNano: now.Add(7 * time.Second).UnixNano()}, Reservation: runtimeports.OperationScopeEvidenceSourceReservationV3{Registration: registration, Source: source, EventID: "discovery-page-event-" + label, Schema: schema}, RequestedTTL: 6 * time.Second, CreatedUnixNano: now.Add(-time.Second).UnixNano(), UpdatedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(6 * time.Second).UnixNano(), IngestNotAfterUnixNano: now.Add(7 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := runtimeports.SealOperationScopeEvidenceProviderHandoffFactV3(runtimeports.OperationScopeEvidenceProviderHandoffFactV3{ID: "discovery-page-handoff-" + label, Revision: 1, Qualification: issued.RefV3(), Phase: phase, CheckedUnixNano: now.Add(-time.Second).UnixNano(), NotAfterUnixNano: now.Add(5 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	event, correlation, payload, observed := "discovery-page-provider-"+label, "discovery-page-correlation-"+label, testDigestV1("discovery-page-payload-"+label), now.UnixNano()
	if observation != nil {
		event, correlation, payload, observed = observation.ProviderOperationRef, observation.PreparedAttemptID, observation.PayloadDigest, observation.ObservedUnixNano
	}
	candidate := runtimeports.OperationScopeEvidenceCandidateV3{ContractVersion: runtimeports.OperationScopeEvidenceContractVersionV3, Qualification: issued.RefV3(), Source: source, EventID: event, TrustClass: runtimeports.EvidenceTrustObservation, Payload: runtimeports.EvidencePayloadRefV2{Schema: schema, ContentDigest: payload, Revision: 1, Length: 1, Ref: "evidence://discovery-page/" + label}, Causation: []runtimeports.EvidenceCausationRefV2{}, CorrelationID: correlation, ObservedUnixNano: observed}
	ledger, _ := scope.LedgerScope.DigestV3()
	record, err := runtimeports.SealOperationScopeEvidenceRecordV3(runtimeports.OperationScopeEvidenceRecordV3{Ref: runtimeports.OperationScopeEvidenceRecordRefV3{LedgerScopeDigest: ledger, Sequence: 1}, Candidate: candidate, PreviousRecordDigest: runtimeports.EvidenceGenesisDigestV2, IngestedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	consumption, err := runtimeports.SealOperationScopeEvidenceConsumptionFactV3(runtimeports.OperationScopeEvidenceConsumptionFactV3{ID: "discovery-page-consumption-" + label, Revision: 1, Qualification: issued.RefV3(), Handoff: handoff.RefV3(), CandidateDigest: record.CandidateDigest, Record: record.Ref, CreatedUnixNano: now.UnixNano()})
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

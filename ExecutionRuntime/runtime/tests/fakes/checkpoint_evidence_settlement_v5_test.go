package fakes_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestCheckpointEvidenceSettlementV5PublicConformance(t *testing.T) {
	fixture := newCheckpointSettlementFixtureV5(t, "conformance")
	evidence := fakes.NewCheckpointEvidenceStoreV1(func() time.Time { return fixture.now })
	evidenceGateway := newCheckpointEvidenceGatewayV1(t, fixture, evidence)
	fixture.gateway.Evidence = evidenceGateway
	report, err := conformance.RunCheckpointEvidenceSettlementConformanceV5(context.Background(), evidenceGateway, fixture.gateway, conformance.CheckpointEvidenceSettlementFixtureV5{
		Qualification: fixture.qualification,
		Handoff: ports.CreateCheckpointPhaseProviderHandoffRequestV1{
			ID: "checkpoint-handoff-conformance", Attempt: fixture.submission.DispatchAttempt, Phase: ports.CheckpointPhasePrepareV2, ScopeDigest: checkpointEvidenceScopeDigestV1(t, fixture.qualification.Scope),
		},
		Consumption: ports.ConsumeCheckpointPhaseEvidenceRequestV1{ID: "checkpoint-consumption-conformance", Record: fixture.record.Ref, Source: fixture.qualification.Scope.Source},
		Settlement:  fixture.submission,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.IssueDoesNotSettle || !report.QualificationOwnerExact || !report.QualificationTTLBounded || !report.ConsumedCurrentExact || !report.HistoricalClosureExact || !report.CurrentClosureExact || !report.AssociationClosureExact || report.ProviderCalls != 0 || report.ProductionClaimEligible {
		t.Fatalf("unexpected checkpoint Evidence/Settlement conformance report: %#v", report)
	}
}

func TestCheckpointEvidenceV1IssueDoesNotAdvanceCursorAndObservationCannotSettleV5(t *testing.T) {
	fixture := newCheckpointSettlementFixtureV5(t, "evidence")
	store := fakes.NewCheckpointEvidenceStoreV1(func() time.Time { return fixture.now })
	evidenceGateway := newCheckpointEvidenceGatewayV1(t, fixture, store)
	fixture.gateway.Evidence = evidenceGateway
	request := fixture.qualification
	qualification, err := evidenceGateway.IssueCheckpointPhaseQualificationV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.CursorV1(); got != 0 {
		t.Fatalf("qualification Issue advanced Evidence cursor: %d", got)
	}
	historicalQualification, err := store.InspectCheckpointPhaseQualificationHistoricalV1(context.Background(), qualification)
	if err != nil || historicalQualification.Validate() != nil {
		t.Fatalf("historical Qualification must validate at its frozen Created time: fact=%+v err=%v", historicalQualification, err)
	}
	invalidCreated := historicalQualification
	invalidCreated.CreatedUnixNano = invalidCreated.Ref.ExpiresUnixNano
	if err := invalidCreated.Validate(); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("Qualification created at its expiry must fail closed: %v", err)
	}
	handoff, err := store.CreateCheckpointPhaseProviderHandoffV1(context.Background(), ports.CreateCheckpointPhaseProviderHandoffRequestV1{
		ID: "checkpoint-handoff-evidence", Qualification: qualification, Attempt: fixture.submission.DispatchAttempt, Phase: ports.CheckpointPhasePrepareV2, ScopeDigest: qualification.ScopeDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	consume := ports.ConsumeCheckpointPhaseEvidenceRequestV1{ID: "checkpoint-consumption-evidence", Qualification: qualification, Handoff: handoff, Record: fixture.record.Ref, Source: fixture.qualification.Scope.Source}
	observation, err := store.ConsumeCheckpointPhaseEvidenceObservationV1(context.Background(), consume)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.CursorV1(); got != 1 {
		t.Fatalf("Evidence consume must advance exactly once: %d", got)
	}
	changed := fixture.submission
	changed.Evidence = observation
	changed.Handoff = handoff
	if _, err := fixture.gateway.SettleCheckpointPhaseV5(context.Background(), changed); err == nil {
		t.Fatal("consumed_observation upgraded into Runtime Settlement V5")
	}
	if fixture.effect.effect.effect.store.CheckpointSettlementV5CommitCount() != 0 {
		t.Fatal("rejected observation published a V5 terminal object")
	}
}

func TestCheckpointEvidenceV1ScopeBindsExactLeaseAndFence(t *testing.T) {
	fixture := newCheckpointSettlementFixtureV5(t, "lease-fence")
	scope := fixture.qualification.Scope

	leaseDrift := scope
	leaseDrift.SandboxLease.Epoch++
	if err := leaseDrift.Validate(); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("Sandbox Lease drift must be rejected: %v", err)
	}

	fenceDrift := scope
	fenceDrift.FenceEpoch++
	if err := fenceDrift.Validate(); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("Fence epoch drift must be rejected: %v", err)
	}

	missingLease := scope
	missingLease.Operation.ExecutionScope.SandboxLease = nil
	missingLease.Operation.ExecutionScopeDigest, _ = ports.ExecutionScopeDigestV2(missingLease.Operation.ExecutionScope)
	missingLease.OperationDigest, _ = missingLease.Operation.DigestV3()
	if err := missingLease.Validate(); err == nil {
		t.Fatal("checkpoint Evidence without the operation Sandbox Lease must be rejected")
	}
}

func TestCheckpointEvidenceV1QualificationBindsOwnerCoordinatesAndDerivedTTL(t *testing.T) {
	fixture := newCheckpointSettlementFixtureV5(t, "qualification-governance")
	store := fakes.NewCheckpointEvidenceStoreV1(func() time.Time { return fixture.now })
	gateway := newCheckpointEvidenceGatewayV1(t, fixture, store)
	request := fixture.qualification
	request.ExpiresUnixNano = fixture.now.Add(time.Minute).UnixNano()
	if _, err := gateway.IssueCheckpointPhaseQualificationV1(context.Background(), request); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("caller-forged long qualification TTL must conflict: %v", err)
	}
	if store.QualificationCountV1() != 0 {
		t.Fatal("forged qualification TTL reached the Evidence Fact Owner")
	}

	request.ExpiresUnixNano = 0
	store.LoseNextReplyV1()
	ref, err := gateway.IssueCheckpointPhaseQualificationV1(context.Background(), request)
	if err != nil {
		t.Fatalf("lost qualification reply must recover by exact Inspect: %v", err)
	}
	if ref.Barrier != request.Barrier || ref.EffectCut != request.EffectCut || ref.Reservation != request.Reservation || ref.ExpiresUnixNano > fixture.now.Add(30*time.Second).UnixNano() {
		t.Fatalf("qualification ref did not freeze exact governance coordinates and TTL: %+v", ref)
	}
	fact, err := gateway.InspectCheckpointPhaseQualificationHistoricalV1(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*ports.CheckpointRestoreEvidenceQualificationFactV1){
		func(value *ports.CheckpointRestoreEvidenceQualificationFactV1) { value.Request.Barrier.ID += "-drift" },
		func(value *ports.CheckpointRestoreEvidenceQualificationFactV1) {
			value.Request.EffectCut.ID += "-drift"
		},
		func(value *ports.CheckpointRestoreEvidenceQualificationFactV1) {
			value.Request.Reservation.ID += "-drift"
		},
		func(value *ports.CheckpointRestoreEvidenceQualificationFactV1) {
			value.Request.Scope.PermitDigest = digestCheckpointV2("qualification-permit-drift")
		},
		func(value *ports.CheckpointRestoreEvidenceQualificationFactV1) {
			value.Request.Scope.EvidencePolicy.ID += "-drift"
		},
		func(value *ports.CheckpointRestoreEvidenceQualificationFactV1) {
			value.Request.Scope.Source.SourceSequence++
		},
		func(value *ports.CheckpointRestoreEvidenceQualificationFactV1) {
			value.Request.Scope.PayloadDigest = digestCheckpointV2("qualification-payload-drift")
		},
	} {
		drifted := fact
		mutate(&drifted)
		if err := drifted.Validate(); err == nil {
			t.Fatal("same qualification ref accepted substituted governance coordinates")
		}
	}
}

func TestCheckpointEvidenceV1HandoffAndConsumptionLostRepliesRecoverExactHistory(t *testing.T) {
	fixture := newCheckpointSettlementFixtureV5(t, "handoff-consume-lost")
	store := fakes.NewCheckpointEvidenceStoreV1(func() time.Time { return fixture.now })
	gateway := newCheckpointEvidenceGatewayV1(t, fixture, store)
	qualification, err := gateway.IssueCheckpointPhaseQualificationV1(context.Background(), fixture.qualification)
	if err != nil {
		t.Fatal(err)
	}
	scopeDigest, _ := fixture.qualification.Scope.DigestV1()
	handoffRequest := ports.CreateCheckpointPhaseProviderHandoffRequestV1{ID: "checkpoint-handoff-lost", Qualification: qualification, Attempt: fixture.qualification.Scope.DispatchAttempt, Phase: fixture.qualification.Phase, ScopeDigest: scopeDigest}
	store.LoseNextReplyV1()
	handoff, err := gateway.CreateCheckpointPhaseProviderHandoffV1(context.Background(), handoffRequest)
	if err != nil {
		t.Fatalf("lost handoff reply did not recover exact immutable history: %v", err)
	}
	if historical, err := gateway.InspectCheckpointPhaseProviderHandoffHistoricalV1(context.Background(), handoff); err != nil || historical != handoff {
		t.Fatalf("handoff historical Inspect is not exact: %+v err=%v", historical, err)
	}
	handoffDigestDrift := handoff
	handoffDigestDrift.Attempt.AttemptID += "-drift"
	if err := handoffDigestDrift.Validate(); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("handoff self digest did not reject changed Attempt: %v", err)
	}
	consumeRequest := ports.ConsumeCheckpointPhaseEvidenceRequestV1{ID: "checkpoint-consumption-lost", Qualification: qualification, Handoff: handoff, Record: fixture.record.Ref, Source: fixture.qualification.Scope.Source}
	store.LoseNextReplyV1()
	consumption, err := gateway.ConsumeCheckpointPhaseEvidenceCurrentV1(context.Background(), consumeRequest)
	if err != nil {
		t.Fatalf("lost consumption reply did not recover exact immutable history: %v", err)
	}
	if historical, err := gateway.InspectCheckpointPhaseEvidenceConsumptionHistoricalV1(context.Background(), consumption); err != nil || historical != consumption {
		t.Fatalf("consumption historical Inspect is not exact: %+v err=%v", historical, err)
	}
	consumptionDigestDrift := consumption
	consumptionDigestDrift.Source.SourceSequence++
	if err := consumptionDigestDrift.Validate(); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("consumption self digest did not reject changed source sequence: %v", err)
	}
	if _, err := gateway.InspectCheckpointPhaseEvidenceConsumptionCurrentV1(context.Background(), consumption); err != nil {
		t.Fatalf("consumption current Inspect did not revalidate qualification and handoff: %v", err)
	}
	drifted := handoffRequest
	drifted.Attempt.AttemptID += "-drift"
	if _, err := gateway.CreateCheckpointPhaseProviderHandoffV1(context.Background(), drifted); err == nil {
		t.Fatal("same handoff ID with changed immutable Attempt was accepted")
	}
}

func TestCheckpointEvidenceV1FreshHandoffIDCannotSpliceQualifiedAttempt(t *testing.T) {
	fixture := newCheckpointSettlementFixtureV5(t, "fresh-handoff-splice")
	store := fakes.NewCheckpointEvidenceStoreV1(func() time.Time { return fixture.now })
	gateway := newCheckpointEvidenceGatewayV1(t, fixture, store)
	qualification, err := gateway.IssueCheckpointPhaseQualificationV1(context.Background(), fixture.qualification)
	if err != nil {
		t.Fatal(err)
	}
	scopeDigest, _ := fixture.qualification.Scope.DigestV1()
	base := ports.CreateCheckpointPhaseProviderHandoffRequestV1{Qualification: qualification, Attempt: fixture.qualification.Scope.DispatchAttempt, Phase: fixture.qualification.Phase, ScopeDigest: scopeDigest}
	mutations := map[string]func(*ports.OperationDispatchAttemptRefV3){
		"attempt-id": func(value *ports.OperationDispatchAttemptRefV3) { value.AttemptID += "-spliced" },
		"permit-revision": func(value *ports.OperationDispatchAttemptRefV3) {
			value.PermitRevision++
			value.PermitDigest = digestCheckpointV2("spliced-permit-revision")
		},
		"intent-digest": func(value *ports.OperationDispatchAttemptRefV3) {
			value.IntentRevision++
			value.IntentDigest = digestCheckpointV2("spliced-intent")
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			request := base
			request.ID = "checkpoint-handoff-fresh-splice-" + name
			mutate(&request.Attempt)
			if err := request.Validate(); err != nil {
				t.Fatalf("splice fixture must remain structurally valid: %v", err)
			}
			if _, err := gateway.CreateCheckpointPhaseProviderHandoffV1(context.Background(), request); !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("fresh handoff ID accepted another valid Operation Attempt: %v", err)
			}
			ref := ports.CheckpointRestoreEvidenceProviderHandoffRefV1{ID: request.ID, Revision: 1, Qualification: request.Qualification, Attempt: request.Attempt, Phase: request.Phase, ScopeDigest: request.ScopeDigest}
			ref.Digest, _ = ref.DigestV1()
			if _, err := store.InspectCheckpointPhaseProviderHandoffHistoricalV1(context.Background(), ref); !core.HasCategory(err, core.ErrorNotFound) {
				t.Fatalf("fresh handoff splice wrote an Owner Fact: %v", err)
			}
		})
	}
}

func TestCheckpointEvidenceV1FreshConsumptionIDCannotSpliceLedgerRecord(t *testing.T) {
	fixture := newCheckpointSettlementFixtureV5(t, "fresh-record-splice")
	store := fakes.NewCheckpointEvidenceStoreV1(func() time.Time { return fixture.now })
	gateway := newCheckpointEvidenceGatewayV1(t, fixture, store)
	qualification, err := gateway.IssueCheckpointPhaseQualificationV1(context.Background(), fixture.qualification)
	if err != nil {
		t.Fatal(err)
	}
	scopeDigest, _ := fixture.qualification.Scope.DigestV1()
	handoff, err := gateway.CreateCheckpointPhaseProviderHandoffV1(context.Background(), ports.CreateCheckpointPhaseProviderHandoffRequestV1{ID: "checkpoint-handoff-fresh-record-splice", Qualification: qualification, Attempt: fixture.qualification.Scope.DispatchAttempt, Phase: fixture.qualification.Phase, ScopeDigest: scopeDigest})
	if err != nil {
		t.Fatal(err)
	}
	splicedCandidate := fixture.record.Candidate
	splicedCandidate.Payload.ContentDigest = digestCheckpointV2("fresh-record-spliced-payload")
	splicedCandidate.Payload.Ref = "memory://checkpoint-evidence/fresh-record-spliced-payload"
	splicedRecord, err := control.NewEvidenceLedgerRecordV2(splicedCandidate, 1, ports.EvidenceGenesisDigestV2, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	gateway.Records = checkpointEvidenceRecordReaderV1{record: splicedRecord}
	request := ports.ConsumeCheckpointPhaseEvidenceRequestV1{ID: "checkpoint-consumption-fresh-record-splice", Qualification: qualification, Handoff: handoff, Record: splicedRecord.Ref, Source: fixture.qualification.Scope.Source}
	if _, err := gateway.ConsumeCheckpointPhaseEvidenceCurrentV1(context.Background(), request); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("fresh consumption ID upgraded a different valid ledger record: %v", err)
	}
	if got := store.CursorV1(); got != 0 {
		t.Fatalf("rejected ledger record splice advanced the Evidence cursor: %d", got)
	}
}

func TestCheckpointEvidenceV1RecordReaderFailuresWriteNothing(t *testing.T) {
	for name, replace := range map[string]func(*kernel.CheckpointRestoreEvidenceGatewayV1){
		"nil": func(gateway *kernel.CheckpointRestoreEvidenceGatewayV1) {
			gateway.Records = nil
		},
		"typed-nil": func(gateway *kernel.CheckpointRestoreEvidenceGatewayV1) {
			var reader *checkpointUnavailableEvidenceRecordReaderV1
			gateway.Records = reader
		},
		"unavailable": func(gateway *kernel.CheckpointRestoreEvidenceGatewayV1) {
			gateway.Records = &checkpointUnavailableEvidenceRecordReaderV1{}
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newCheckpointSettlementFixtureV5(t, "record-reader-"+name)
			store := fakes.NewCheckpointEvidenceStoreV1(func() time.Time { return fixture.now })
			gateway := newCheckpointEvidenceGatewayV1(t, fixture, store)
			qualification, handoff := createCheckpointEvidenceQualificationAndHandoffV1(t, &gateway, fixture, "record-reader-"+name)
			replace(&gateway)
			request := ports.ConsumeCheckpointPhaseEvidenceRequestV1{ID: "checkpoint-consumption-record-reader-" + name, Qualification: qualification, Handoff: handoff, Record: fixture.record.Ref, Source: fixture.qualification.Scope.Source}
			if _, err := gateway.ConsumeCheckpointPhaseEvidenceCurrentV1(context.Background(), request); err == nil || (!core.HasCategory(err, core.ErrorUnavailable) && !core.HasReason(err, core.ReasonComponentMissing)) {
				t.Fatalf("missing/unavailable Evidence record Reader did not fail closed: %v", err)
			}
			if got := store.CursorV1(); got != 0 {
				t.Fatalf("record Reader failure advanced Evidence cursor: %d", got)
			}
		})
	}
}

func TestCheckpointEvidenceV1RecordMappingDriftBetweenReadsWritesNothing(t *testing.T) {
	fixture := newCheckpointSettlementFixtureV5(t, "record-s1-s2-drift")
	store := fakes.NewCheckpointEvidenceStoreV1(func() time.Time { return fixture.now })
	gateway := newCheckpointEvidenceGatewayV1(t, fixture, store)
	qualification, handoff := createCheckpointEvidenceQualificationAndHandoffV1(t, &gateway, fixture, "record-s1-s2-drift")
	driftedCandidate := fixture.record.Candidate
	driftedCandidate.Payload.ContentDigest = digestCheckpointV2("record-s1-s2-drifted-payload")
	driftedCandidate.Payload.Ref = "memory://checkpoint-evidence/record-s1-s2-drifted-payload"
	driftedRecord, err := control.NewEvidenceLedgerRecordV2(driftedCandidate, 1, ports.EvidenceGenesisDigestV2, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	gateway.Records = &checkpointDriftingEvidenceRecordReaderV1{first: fixture.record, second: driftedRecord}
	request := ports.ConsumeCheckpointPhaseEvidenceRequestV1{ID: "checkpoint-consumption-record-s1-s2-drift", Qualification: qualification, Handoff: handoff, Record: fixture.record.Ref, Source: fixture.qualification.Scope.Source}
	if _, err := gateway.ConsumeCheckpointPhaseEvidenceCurrentV1(context.Background(), request); err == nil {
		t.Fatal("Evidence record/source mapping drift between S1/S2 reached the Fact Owner")
	}
	if got := store.CursorV1(); got != 0 {
		t.Fatalf("S1/S2 record drift advanced Evidence cursor: %d", got)
	}
}

func TestCheckpointEvidenceV1FreshConsumptionRecordDriftMatrixWritesNothing(t *testing.T) {
	mutations := map[string]func(*ports.EvidenceEventCandidateV2){
		"source-epoch":    func(candidate *ports.EvidenceEventCandidateV2) { candidate.SourceEpoch++ },
		"source-sequence": func(candidate *ports.EvidenceEventCandidateV2) { candidate.SourceSequence++ },
		"schema": func(candidate *ports.EvidenceEventCandidateV2) {
			candidate.Payload.Schema.Name = "checkpoint-evidence-other"
		},
		"payload-digest": func(candidate *ports.EvidenceEventCandidateV2) {
			candidate.Payload.ContentDigest = digestCheckpointV2("record-drift-payload")
		},
		"payload-revision": func(candidate *ports.EvidenceEventCandidateV2) { candidate.Payload.Revision++ },
		"payload-length":   func(candidate *ports.EvidenceEventCandidateV2) { candidate.Payload.Length++ },
		"execution-scope": func(candidate *ports.EvidenceEventCandidateV2) {
			candidate.ExecutionScope.Instance.ID += "-other"
			candidate.LedgerScope.InstanceID = candidate.ExecutionScope.Instance.ID
		},
		"authority": func(candidate *ports.EvidenceEventCandidateV2) {
			candidate.Authority.Revision++
			candidate.Authority.Digest = digestCheckpointV2("record-drift-authority")
		},
		"ledger-scope": func(candidate *ports.EvidenceEventCandidateV2) {
			candidate.LedgerScope.Partition = ports.EvidencePartitionTenant
			candidate.LedgerScope.IdentityID = ""
			candidate.LedgerScope.LineageID = ""
			candidate.LedgerScope.InstanceID = ""
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			fixture := newCheckpointSettlementFixtureV5(t, "record-drift-"+name)
			store := fakes.NewCheckpointEvidenceStoreV1(func() time.Time { return fixture.now })
			gateway := newCheckpointEvidenceGatewayV1(t, fixture, store)
			qualification, handoff := createCheckpointEvidenceQualificationAndHandoffV1(t, &gateway, fixture, "record-drift-"+name)
			candidate := fixture.record.Candidate
			mutate(&candidate)
			candidate.Payload.Ref = "memory://checkpoint-evidence/record-drift-" + name
			record, err := control.NewEvidenceLedgerRecordV2(candidate, 1, ports.EvidenceGenesisDigestV2, fixture.now)
			if err != nil {
				t.Fatalf("drift fixture must remain structurally valid: %v", err)
			}
			gateway.Records = checkpointEvidenceRecordReaderV1{record: record}
			request := ports.ConsumeCheckpointPhaseEvidenceRequestV1{ID: "checkpoint-consumption-record-drift-" + name, Qualification: qualification, Handoff: handoff, Record: record.Ref, Source: fixture.qualification.Scope.Source}
			if _, err := gateway.ConsumeCheckpointPhaseEvidenceCurrentV1(context.Background(), request); err == nil {
				t.Fatal("fresh consumption ID upgraded a structurally valid but unqualified Evidence record")
			}
			if got := store.CursorV1(); got != 0 {
				t.Fatalf("record drift advanced Evidence cursor: %d", got)
			}
		})
	}

	t.Run("ledger-sequence-or-predecessor", func(t *testing.T) {
		fixture := newCheckpointSettlementFixtureV5(t, "record-drift-ledger-sequence")
		store := fakes.NewCheckpointEvidenceStoreV1(func() time.Time { return fixture.now })
		gateway := newCheckpointEvidenceGatewayV1(t, fixture, store)
		qualification, handoff := createCheckpointEvidenceQualificationAndHandoffV1(t, &gateway, fixture, "record-drift-ledger-sequence")
		record, err := control.NewEvidenceLedgerRecordV2(fixture.record.Candidate, 2, digestCheckpointV2("missing-ledger-predecessor"), fixture.now)
		if err != nil {
			t.Fatal(err)
		}
		gateway.Records = checkpointEvidenceRecordReaderV1{record: record}
		request := ports.ConsumeCheckpointPhaseEvidenceRequestV1{ID: "checkpoint-consumption-record-drift-ledger-sequence", Qualification: qualification, Handoff: handoff, Record: record.Ref, Source: fixture.qualification.Scope.Source}
		if _, err := gateway.ConsumeCheckpointPhaseEvidenceCurrentV1(context.Background(), request); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("unresolved ledger predecessor must fail closed: %v", err)
		}
		if got := store.CursorV1(); got != 0 {
			t.Fatalf("unresolved ledger predecessor advanced Evidence cursor: %d", got)
		}
	})
}

func TestCheckpointEvidenceV1PermitPolicySourceAndPayloadDriftWriteNothing(t *testing.T) {
	for _, name := range []string{"permit", "policy", "source", "payload"} {
		t.Run(name, func(t *testing.T) {
			fixture := newCheckpointSettlementFixtureV5(t, "owner-drift-"+name)
			store := fakes.NewCheckpointEvidenceStoreV1(func() time.Time { return fixture.now })
			gateway := newCheckpointEvidenceGatewayV1(t, fixture, store)
			other := newCheckpointSettlementFixtureV5(t, "owner-drift-other-"+name)
			otherStore := fakes.NewCheckpointEvidenceStoreV1(func() time.Time { return other.now })
			otherGateway := newCheckpointEvidenceGatewayV1(t, other, otherStore)
			switch name {
			case "permit":
				gateway.Execution = otherGateway.Execution
			case "policy":
				gateway.Policies = otherGateway.Policies
			case "source":
				gateway.Sources = otherGateway.Sources
			case "payload":
				request := fixture.qualification
				request.Scope.PayloadRevision++
				if _, err := gateway.IssueCheckpointPhaseQualificationV1(context.Background(), request); err == nil {
					t.Fatal("payload revision drift was accepted")
				}
				if store.QualificationCountV1() != 0 {
					t.Fatal("payload revision drift reached Evidence Fact Owner")
				}
				return
			}
			if _, err := gateway.IssueCheckpointPhaseQualificationV1(context.Background(), fixture.qualification); err == nil {
				t.Fatalf("%s Owner current drift was accepted", name)
			}
			if store.QualificationCountV1() != 0 {
				t.Fatalf("%s Owner current drift reached Evidence Fact Owner", name)
			}
		})
	}
}

func TestCheckpointEvidenceV1QualificationCurrentDriftWritesNothing(t *testing.T) {
	fixture := newCheckpointSettlementFixtureV5(t, "qualification-drift")
	base, err := ports.SealCheckpointEvidenceAttemptCurrentProjectionV1(ports.CheckpointEvidenceAttemptCurrentProjectionV1{Attempt: fixture.qualification.Attempt, Barrier: fixture.qualification.Barrier, EffectCut: fixture.qualification.EffectCut, Current: true, CheckedUnixNano: fixture.now.UnixNano(), ExpiresUnixNano: fixture.qualification.Barrier.ExpiresUnixNano}, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		mutate func(*kernel.CheckpointRestoreEvidenceGatewayV1)
	}{
		{name: "attempt", mutate: func(gateway *kernel.CheckpointRestoreEvidenceGatewayV1) {
			second := base
			second.Attempt.Digest = digestCheckpointV2("qualification-attempt-drift")
			second.ProjectionDigest = ""
			second, _ = ports.SealCheckpointEvidenceAttemptCurrentProjectionV1(second, fixture.now)
			gateway.Checkpoints = &checkpointEvidenceAttemptReaderV1{projection: base, second: &second}
		}},
		{name: "inputs", mutate: func(gateway *kernel.CheckpointRestoreEvidenceGatewayV1) {
			second := fixture.domain.inputs
			second.Authority.Digest = digestCheckpointV2("qualification-authority-drift")
			second.ProjectionDigest = ""
			second, _ = ports.SealCheckpointAttemptInputsCurrentProjectionV2(second, fixture.now)
			fixture.domain.inputCalls.Store(0)
			fixture.domain.secondInputs = &second
		}},
		{name: "reservation", mutate: func(gateway *kernel.CheckpointRestoreEvidenceGatewayV1) {
			second := fixture.domain.reservation
			second.OwnerBinding.ArtifactDigest = digestCheckpointV2("qualification-reservation-drift")
			second.ProjectionDigest = ""
			second.ProjectionDigest, _ = second.DigestV2()
			fixture.domain.reservationCalls.Store(0)
			fixture.domain.secondReservation = &second
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			local := newCheckpointSettlementFixtureV5(t, "qualification-drift-"+testCase.name)
			store := fakes.NewCheckpointEvidenceStoreV1(func() time.Time { return local.now })
			gateway := newCheckpointEvidenceGatewayV1(t, local, store)
			fixture = local
			base, _ = ports.SealCheckpointEvidenceAttemptCurrentProjectionV1(ports.CheckpointEvidenceAttemptCurrentProjectionV1{Attempt: local.qualification.Attempt, Barrier: local.qualification.Barrier, EffectCut: local.qualification.EffectCut, Current: true, CheckedUnixNano: local.now.UnixNano(), ExpiresUnixNano: local.qualification.Barrier.ExpiresUnixNano}, local.now)
			testCase.mutate(&gateway)
			if _, err := gateway.IssueCheckpointPhaseQualificationV1(context.Background(), local.qualification); err == nil {
				t.Fatalf("%s current drift was accepted", testCase.name)
			}
			if store.QualificationCountV1() != 0 {
				t.Fatalf("%s current drift reached Evidence Fact Owner", testCase.name)
			}
		})
	}
}

func TestCheckpointSettlementV5AtomicStagesLostReplyAndExactClosure(t *testing.T) {
	for stage := 1; stage <= 6; stage++ {
		fixture := newCheckpointSettlementFixtureV5(t, "stage-"+string(rune('0'+stage)))
		fixture.effect.effect.effect.store.FailNextCheckpointSettlementV5CommitAfterStage(stage)
		if _, err := fixture.gateway.SettleCheckpointPhaseV5(context.Background(), fixture.submission); !core.HasCategory(err, core.ErrorIndeterminate) {
			t.Fatalf("stage %d should remain indeterminate after zero publish: %v", stage, err)
		}
		if _, err := fixture.gateway.InspectCheckpointPhaseSettlementHistoricalV5(context.Background(), ports.InspectOperationCheckpointRestoreSettlementRequestV5{Operation: fixture.submission.Operation, SettlementID: fixture.submission.ID}); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("stage %d published a partial V5 closure: %v", stage, err)
		}
		if _, err := fixture.gateway.InspectCheckpointPhaseSettlementCurrentV5(context.Background(), ports.InspectCurrentOperationCheckpointRestoreSettlementRequestV5{Operation: fixture.submission.Operation, EffectID: fixture.submission.EffectID}); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("stage %d published a terminal Effect/current guard sidecar: %v", stage, err)
		}
		if _, err := fixture.gateway.SettleCheckpointPhaseV5(context.Background(), fixture.submission); err != nil {
			t.Fatalf("stage %d did not recover with the same canonical submission: %v", stage, err)
		}
	}

	fixture := newCheckpointSettlementFixtureV5(t, "lost-reply")
	fixture.effect.effect.effect.store.LoseNextCheckpointSettlementV5Reply()
	ref, err := fixture.gateway.SettleCheckpointPhaseV5(context.Background(), fixture.submission)
	if err != nil {
		t.Fatal(err)
	}
	if fixture.effect.effect.effect.store.CheckpointSettlementV5CommitCount() != 1 {
		t.Fatal("lost reply retried the V5 commit")
	}
	historical, err := fixture.gateway.InspectCheckpointPhaseSettlementHistoricalV5(context.Background(), ports.InspectOperationCheckpointRestoreSettlementRequestV5{Operation: fixture.submission.Operation, SettlementID: ref.ID})
	if err != nil || historical.Settlement != ref {
		t.Fatalf("historical V5 closure is not exact: ref=%#v err=%v", historical.Settlement, err)
	}
	if historical.EffectTerminal.Ref.Settlement != ref || historical.EffectTerminal.Ref.PreviousRevision != fixture.submission.ExpectedEffectRevision || historical.EffectTerminal.State != "settled" {
		t.Fatalf("terminal Effect was not atomically published with the four-object closure: %#v", historical.EffectTerminal)
	}
	current, err := fixture.gateway.InspectCheckpointPhaseSettlementCurrentV5(context.Background(), ports.InspectCurrentOperationCheckpointRestoreSettlementRequestV5{Operation: fixture.submission.Operation, EffectID: fixture.submission.EffectID})
	if err != nil || current.Bundle.Settlement != ref {
		t.Fatalf("current V5 closure is not exact: %#v err=%v", current, err)
	}
	if _, err := fixture.gateway.InspectCheckpointPhaseSettlementAssociationV5(context.Background(), fixture.submission.Operation, historical.Association.Ref); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.gateway.InspectCheckpointPhaseTerminalGuardV5(context.Background(), fixture.submission.Operation, historical.Guard.Ref); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.gateway.InspectCheckpointPhaseTerminalProjectionV5(context.Background(), fixture.submission.Operation, historical.Projection.Ref); err != nil {
		t.Fatal(err)
	}
}

func TestCheckpointSettlementV5SharedGuardWithV4AndConcurrent64(t *testing.T) {
	legacy := newLegacyOperationSettlementFixtureV3(t, "checkpoint-v3-first")
	if _, err := legacy.gateway.SettleOperationEffectV3(context.Background(), legacy.effect.intent, legacy.submission); err != nil {
		t.Fatal(err)
	}
	template := newCheckpointSettlementFixtureV5(t, "v3-template")
	v3Submission := retargetCheckpointSettlementSubmissionV5(t, template.submission, legacy.effect.intent, legacy.submission.Attempt, "v3-first")
	v3Domain := checkpointDomainProjectionV2(t, v3Submission.DomainResult, legacy.effect.now)
	v3Gateway := template.gateway
	v3Gateway.Facts = legacy.effect.store
	v3Gateway.DomainResults = &checkpointDomainReaderV2{projection: v3Domain, inputs: template.domain.inputs, reservation: template.domain.reservation, participant: template.domain.participant}
	v3Gateway.Clock = func() time.Time { return legacy.effect.now }
	if _, err := v3Gateway.SettleCheckpointPhaseV5(context.Background(), v3Submission); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("V3-first did not block V5: %v", err)
	}

	fixture := newCheckpointSettlementFixtureV5(t, "v5-first")
	if _, err := fixture.gateway.SettleCheckpointPhaseV5(context.Background(), fixture.submission); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.effect.gateway.SettleOperationV4(context.Background(), fixture.effect.submission); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("V5-first did not block V4: %v", err)
	}
	currentAfterV5, err := fixture.effect.effect.effect.store.InspectOperationEffectV3(context.Background(), fixture.submission.Operation, fixture.submission.EffectID)
	if err != nil {
		t.Fatal(err)
	}
	nextAfterV5 := currentAfterV5
	nextAfterV5.State = control.OperationEffectSettledV3
	nextAfterV5.Revision++
	nextAfterV5.Settlement = legacySettlementFactForV4Guard(t, fixture.effect)
	if _, err := fixture.effect.effect.effect.store.CompareAndSwapOperationEffectV3(context.Background(), fixture.submission.Operation, control.OperationEffectCASRequestV3{ExpectedRevision: currentAfterV5.Revision, Next: nextAfterV5}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("V5-first did not block V3: %v", err)
	}

	v4First := newCheckpointSettlementFixtureV5(t, "v4-first")
	if _, err := v4First.effect.gateway.SettleOperationV4(context.Background(), v4First.effect.submission); err != nil {
		t.Fatal(err)
	}
	if _, err := v4First.gateway.SettleCheckpointPhaseV5(context.Background(), v4First.submission); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("V4-first did not block V5: %v", err)
	}

	race := newCheckpointSettlementFixtureV5(t, "race")
	const workers = 64
	var success atomic.Int64
	var conflicts atomic.Int64
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func(v5 bool) {
			defer wait.Done()
			var err error
			if v5 {
				_, err = race.gateway.SettleCheckpointPhaseV5(context.Background(), race.submission)
			} else {
				_, err = race.effect.gateway.SettleOperationV4(context.Background(), race.effect.submission)
			}
			switch {
			case err == nil:
				success.Add(1)
			case core.HasCategory(err, core.ErrorConflict):
				conflicts.Add(1)
			default:
				t.Errorf("unexpected V4/V5 race error: %v", err)
			}
		}(index%2 == 0)
	}
	wait.Wait()
	if success.Load() == 0 || success.Load()+conflicts.Load() != workers {
		t.Fatalf("V4/V5 shared guard race did not converge: success=%d conflict=%d", success.Load(), conflicts.Load())
	}
}

func TestCheckpointSettlementV5TerminalGuardPartitionsByTenantAndNotOperationDigest(t *testing.T) {
	left := newCheckpointSettlementFixtureV5(t, "guard-left")
	if _, err := left.gateway.SettleCheckpointPhaseV5(context.Background(), left.submission); err != nil {
		t.Fatal(err)
	}

	current, err := left.effect.effect.effect.store.InspectOperationEffectV3(context.Background(), left.submission.Operation, left.submission.EffectID)
	if err != nil {
		t.Fatal(err)
	}
	forgedIntent := current.Intent
	forgedIntent.Operation.ActivationAttemptID = "checkpoint-another-activation-attempt"
	attempt := left.submission.DispatchAttempt
	attempt.OperationDigest, _ = forgedIntent.Operation.DigestV3()
	forgedSubmission := retargetCheckpointSettlementSubmissionV5(t, left.submission, forgedIntent, attempt, "guard-same-tenant")
	if forgedSubmission.OperationDigest == left.submission.OperationDigest {
		t.Fatal("same-tenant guard fixture did not change Operation digest")
	}
	forgedBundle, err := control.BuildOperationCheckpointRestoreSettlementBundleV5(forgedSubmission)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := left.effect.effect.effect.store.CommitCheckpointPhaseSettlementV5(context.Background(), forgedBundle); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same tenant/effect with another Operation digest bypassed shared guard: %v", err)
	}

	crossTenantEnforcement := newOperationEnforcementFixtureForScopeV4(t, "checkpoint-guard-cross-tenant", "", "tenant-checkpoint-v5-other", "praxis.sandbox/allocate")
	crossTenantOperation := newOperationSettlementFixtureFromEnforcementV4(t, crossTenantEnforcement, "checkpoint-guard-cross-tenant")
	crossTenant := newCheckpointSettlementFixtureFromOperationV5(t, crossTenantOperation, "guard-cross-tenant")
	if crossTenant.submission.EffectID != left.submission.EffectID || crossTenant.submission.CheckpointAttempt.TenantID == left.submission.CheckpointAttempt.TenantID {
		t.Fatalf("cross-tenant guard fixture is not exact: left=%s/%s right=%s/%s", left.submission.EffectID, left.submission.CheckpointAttempt.TenantID, crossTenant.submission.EffectID, crossTenant.submission.CheckpointAttempt.TenantID)
	}
	crossTenantCurrent, err := crossTenantOperation.effect.effect.store.InspectOperationEffectV3(context.Background(), crossTenant.submission.Operation, crossTenant.submission.EffectID)
	if err != nil {
		t.Fatal(err)
	}
	if err := left.effect.effect.effect.store.InstallOperationEffectFactForTestV3(crossTenantCurrent); err != nil {
		t.Fatal(err)
	}
	crossTenant.gateway.Facts = left.effect.effect.effect.store
	if _, err := crossTenant.gateway.SettleCheckpointPhaseV5(context.Background(), crossTenant.submission); err != nil {
		t.Fatalf("cross-tenant equal Effect ID was incorrectly locked: %v", err)
	}
	if got := left.effect.effect.effect.store.CheckpointSettlementV5CommitCount(); got != 2 {
		t.Fatalf("cross-tenant terminal effects did not commit independently: %d", got)
	}
}

func TestCheckpointSettlementV5FreshCurrentAndClockRollbackFailBeforeCommit(t *testing.T) {
	drift := newCheckpointSettlementFixtureV5(t, "domain-drift")
	drifted := drift.domain.projection
	drifted.Ref.Digest = digestCheckpointV2("checkpoint-domain-drifted")
	drifted.ProjectionDigest = ""
	copyDrifted := drifted
	drifted.ProjectionDigest, _ = core.CanonicalJSONDigest("praxis.runtime.checkpoint-governance", ports.CheckpointGovernanceContractVersionV2, "CheckpointParticipantDomainResultCurrentProjectionV2", copyDrifted)
	drift.domain.second = &drifted
	if _, err := drift.gateway.SettleCheckpointPhaseV5(context.Background(), drift.submission); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("second DomainResult current read drift did not fail closed: %v", err)
	}
	if drift.effect.effect.effect.store.CheckpointSettlementV5CommitCount() != 0 {
		t.Fatal("DomainResult drift published V5 terminal objects")
	}

	rollback := newCheckpointSettlementFixtureV5(t, "clock-rollback")
	var calls atomic.Int64
	rollback.gateway.Clock = func() time.Time {
		if calls.Add(1) == 1 {
			return rollback.now
		}
		return rollback.now.Add(-time.Nanosecond)
	}
	if _, err := rollback.gateway.SettleCheckpointPhaseV5(context.Background(), rollback.submission); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock rollback did not fail closed: %v", err)
	}
	if rollback.effect.effect.effect.store.CheckpointSettlementV5CommitCount() != 0 {
		t.Fatal("clock rollback published V5 terminal objects")
	}
}

func TestCheckpointSettlementV5RejectsCallerFabricatedQualificationOwnerCoordinates(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*ports.CheckpointRestoreEvidenceQualificationFactV1)
	}{
		{name: "barrier", mutate: func(f *ports.CheckpointRestoreEvidenceQualificationFactV1) { f.Request.Barrier.ID += "-forged" }},
		{name: "effect-cut", mutate: func(f *ports.CheckpointRestoreEvidenceQualificationFactV1) { f.Request.EffectCut.ID += "-forged" }},
		{name: "reservation", mutate: func(f *ports.CheckpointRestoreEvidenceQualificationFactV1) { f.Request.Reservation.ID += "-forged" }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newCheckpointSettlementFixtureV5(t, "qualification-owner-"+testCase.name)
			fixture.gateway.Evidence = checkpointEvidenceFactDriftReaderV1{CheckpointRestoreEvidenceGovernancePortV1: fixture.evidencePort, mutate: testCase.mutate}
			if _, err := fixture.gateway.SettleCheckpointPhaseV5(context.Background(), fixture.submission); err == nil {
				t.Fatalf("caller-fabricated Qualification %s coordinate did not fail closed: %v", testCase.name, err)
			}
			if fixture.effect.effect.effect.store.CheckpointSettlementV5CommitCount() != 0 {
				t.Fatal("caller-fabricated Qualification published terminal Effect or settlement sidecars")
			}
		})
	}
}

func TestCheckpointSettlementV5CurrentOwnerDriftBetweenS1S2PublishesNothing(t *testing.T) {
	cases := []struct {
		name  string
		drift func(*checkpointSettlementFixtureV5)
	}{
		{name: "attempt-inputs", drift: func(f *checkpointSettlementFixtureV5) {
			value := f.domain.inputs
			value.Authority.Digest = digestCheckpointV2("v5-authority-current-drift")
			value.ProjectionDigest = ""
			sealed, err := ports.SealCheckpointAttemptInputsCurrentProjectionV2(value, f.now)
			if err != nil {
				t.Fatal(err)
			}
			f.domain.secondInputs = &sealed
		}},
		{name: "reservation", drift: func(f *checkpointSettlementFixtureV5) {
			value := f.domain.reservation
			value.OwnerBinding.ArtifactDigest = digestCheckpointV2("v5-reservation-owner-drift")
			value.ProjectionDigest = ""
			value.ProjectionDigest, _ = value.DigestV2()
			if err := value.Validate(f.now); err != nil {
				t.Fatal(err)
			}
			f.domain.secondReservation = &value
		}},
		{name: "participant", drift: func(f *checkpointSettlementFixtureV5) {
			value := f.domain.participant
			value.Ref.Digest = digestCheckpointV2("v5-participant-drift")
			value.ProjectionDigest = ""
			copyValue := value
			value.ProjectionDigest, _ = core.CanonicalJSONDigest("praxis.runtime.checkpoint-governance", ports.CheckpointGovernanceContractVersionV2, "CheckpointParticipantPhaseCurrentProjectionV2", copyValue)
			if err := value.Validate(f.now); err != nil {
				t.Fatal(err)
			}
			f.domain.secondParticipant = &value
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newCheckpointSettlementFixtureV5(t, "current-drift-"+testCase.name)
			testCase.drift(&fixture)
			if _, err := fixture.gateway.SettleCheckpointPhaseV5(context.Background(), fixture.submission); !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("%s current Owner drift did not fail closed: %v", testCase.name, err)
			}
			if fixture.effect.effect.effect.store.CheckpointSettlementV5CommitCount() != 0 {
				t.Fatalf("%s current Owner drift published a terminal closure", testCase.name)
			}
		})
	}
}

func TestCheckpointSettlementV5TypedNilDependenciesFailBeforeFactOwner(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*checkpointSettlementFixtureV5)
	}{
		{name: "inputs", mutate: func(f *checkpointSettlementFixtureV5) {
			var reader *checkpointDomainReaderV2
			f.gateway.Inputs = reader
		}},
		{name: "evidence", mutate: func(f *checkpointSettlementFixtureV5) {
			var reader *kernel.CheckpointRestoreEvidenceGatewayV1
			f.gateway.Evidence = reader
		}},
		{name: "fact-owner", mutate: func(f *checkpointSettlementFixtureV5) {
			var owner *fakes.OperationEffectStoreV3
			f.gateway.Facts = owner
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newCheckpointSettlementFixtureV5(t, "typed-nil-"+testCase.name)
			testCase.mutate(&fixture)
			if _, err := fixture.gateway.SettleCheckpointPhaseV5(context.Background(), fixture.submission); !core.HasReason(err, core.ReasonComponentMissing) {
				t.Fatalf("typed-nil %s dependency did not fail closed: %v", testCase.name, err)
			}
			if fixture.effect.effect.effect.store.CheckpointSettlementV5CommitCount() != 0 {
				t.Fatalf("typed-nil %s dependency reached Fact Owner", testCase.name)
			}
		})
	}
}

type checkpointEvidenceFactDriftReaderV1 struct {
	ports.CheckpointRestoreEvidenceGovernancePortV1
	mutate func(*ports.CheckpointRestoreEvidenceQualificationFactV1)
}

func (r checkpointEvidenceFactDriftReaderV1) InspectCheckpointPhaseQualificationHistoricalV1(ctx context.Context, ref ports.CheckpointRestoreEvidenceQualificationRefV1) (ports.CheckpointRestoreEvidenceQualificationFactV1, error) {
	fact, err := r.CheckpointRestoreEvidenceGovernancePortV1.InspectCheckpointPhaseQualificationHistoricalV1(ctx, ref)
	if err == nil && r.mutate != nil {
		r.mutate(&fact)
	}
	return fact, err
}

type checkpointSettlementFixtureV5 struct {
	now           time.Time
	effect        *operationSettlementFixtureV4
	domain        *checkpointDomainReaderV2
	gateway       kernel.OperationCheckpointRestoreSettlementGatewayV5
	submission    ports.OperationCheckpointRestoreSettlementSubmissionV5
	qualification ports.IssueCheckpointPhaseQualificationRequestV1
	evidence      *fakes.CheckpointEvidenceStoreV1
	evidencePort  ports.CheckpointRestoreEvidenceGovernancePortV1
	record        ports.EvidenceLedgerRecordV2
}

func newCheckpointSettlementFixtureV5(t *testing.T, suffix string) checkpointSettlementFixtureV5 {
	t.Helper()
	effect := newOperationSettlementFixtureV4(t, "checkpoint-v5-"+suffix)
	return newCheckpointSettlementFixtureFromOperationV5(t, effect, suffix)
}

func newCheckpointSettlementFixtureFromOperationV5(t *testing.T, effect *operationSettlementFixtureV4, suffix string) checkpointSettlementFixtureV5 {
	t.Helper()
	operation := effect.submission.Operation
	operationDigest := effect.submission.OperationDigest
	attempt := effect.submission.DomainResult.Attempt
	var enforcement ports.OperationDispatchEnforcementPhaseRefV4
	for _, binding := range effect.submission.Evidence {
		if binding.Phase == ports.OperationDispatchEnforcementExecuteV4 {
			enforcement = binding.EnforcementPhase
		}
	}
	checkpointAttempt := ports.CheckpointAttemptRefV2{ID: "checkpoint-attempt-v5-" + suffix, Revision: 1, TenantID: operation.ExecutionScope.Identity.TenantID, Digest: digestCheckpointV2("checkpoint-attempt-v5-" + suffix)}
	owner := checkpointSettlementOwnerBindingV5(t, effect.effect.effect.intent)
	participant := ports.CheckpointParticipantRefV2{ID: "checkpoint-participant-v5-" + suffix, Owner: owner, Digest: digestCheckpointV2("checkpoint-participant-v5-" + suffix)}
	phaseFact := ports.CheckpointParticipantPhaseRefV2{ID: "checkpoint-phase-v5-" + suffix, Revision: 1, Phase: ports.CheckpointPhasePrepareV2, State: ports.CheckpointParticipantPreparedV2, Digest: digestCheckpointV2("checkpoint-phase-v5-" + suffix)}
	domainResult := ports.CheckpointParticipantDomainResultRefV2{ID: "checkpoint-domain-result-v5-" + suffix, Revision: 1, Kind: "praxis.sandbox/checkpoint-domain-result", Attempt: checkpointAttempt, Participant: participant, Phase: ports.CheckpointPhasePrepareV2, Operation: operation, OperationDigest: operationDigest, Digest: digestCheckpointV2("checkpoint-domain-result-v5-" + suffix)}
	reservation := ports.CheckpointParticipantPhaseReservationRefV2{ID: "checkpoint-reservation-v5-" + suffix, Revision: 1, Digest: digestCheckpointV2("checkpoint-reservation-v5-" + suffix), ExpiresUnixNano: effect.now.Add(time.Minute).UnixNano()}
	barrier := ports.CheckpointBarrierLeaseRefV2{ID: "checkpoint-barrier-v5-" + suffix, Revision: 1, TenantID: checkpointAttempt.TenantID, AttemptID: checkpointAttempt.ID, Digest: digestCheckpointV2("checkpoint-barrier-v5-" + suffix), ExpiresUnixNano: effect.now.Add(time.Minute).UnixNano()}
	cut := ports.EffectCutRefV2{ID: "checkpoint-cut-v5-" + suffix, Revision: 1, Attempt: checkpointAttempt, RootDigest: digestCheckpointV2("checkpoint-cut-root-v5-" + suffix), Watermark: 1, Count: 0, Digest: digestCheckpointV2("checkpoint-cut-v5-" + suffix)}
	evidenceScope := checkpointEvidenceScopeV1(t, effect, attempt, suffix)
	record := checkpointEvidenceLedgerRecordV1(t, evidenceScope, owner, effect.now, suffix)
	qualificationRequest := ports.IssueCheckpointPhaseQualificationRequestV1{ID: "checkpoint-qualification-v5-" + suffix, Attempt: checkpointAttempt, Barrier: barrier, EffectCut: cut, Reservation: reservation, Phase: ports.CheckpointPhasePrepareV2, Scope: evidenceScope}
	scopeDigest, _ := evidenceScope.DigestV1()
	evidenceStore := fakes.NewCheckpointEvidenceStoreV1(func() time.Time { return effect.now })
	submission := ports.OperationCheckpointRestoreSettlementSubmissionV5{ID: "checkpoint-settlement-v5-" + suffix, Operation: operation, OperationDigest: operationDigest, EffectID: effect.submission.EffectID, ExpectedEffectRevision: effect.submission.ExpectedEffectRevision, CheckpointAttempt: checkpointAttempt, Phase: ports.CheckpointPhasePrepareV2, ParticipantFact: phaseFact, Reservation: reservation, DomainResult: domainResult, DispatchAttempt: attempt, Enforcement: enforcement, Owner: owner, SettledUnixNano: effect.now.UnixNano()}
	projection := ports.CheckpointParticipantDomainResultCurrentProjectionV2{Ref: domainResult, Current: true, CheckedUnixNano: effect.now.UnixNano(), ExpiresUnixNano: effect.now.Add(time.Minute).UnixNano()}
	copyProjection := projection
	copyProjection.ProjectionDigest = ""
	projection.ProjectionDigest, _ = core.CanonicalJSONDigest("praxis.runtime.checkpoint-governance", ports.CheckpointGovernanceContractVersionV2, "CheckpointParticipantDomainResultCurrentProjectionV2", copyProjection)
	inputs := checkpointSettlementAttemptInputsProjectionV5(t, submission, evidenceScope, effect.now)
	reservationProjection := checkpointSettlementReservationProjectionV5(t, submission, evidenceScope, effect.now, cut, barrier, participant, owner)
	participantProjection := checkpointSettlementParticipantProjectionV5(t, submission, effect.now)
	domain := &checkpointDomainReaderV2{projection: projection, inputs: inputs, reservation: reservationProjection, participant: participantProjection}
	partial := checkpointSettlementFixtureV5{now: effect.now, effect: effect, domain: domain, submission: submission, qualification: qualificationRequest, evidence: evidenceStore, record: record}
	evidenceGateway := newCheckpointEvidenceGatewayV1(t, partial, evidenceStore)
	qualification, err := evidenceGateway.IssueCheckpointPhaseQualificationV1(context.Background(), qualificationRequest)
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := evidenceGateway.CreateCheckpointPhaseProviderHandoffV1(context.Background(), ports.CreateCheckpointPhaseProviderHandoffRequestV1{ID: "checkpoint-handoff-v5-" + suffix, Qualification: qualification, Attempt: attempt, Phase: ports.CheckpointPhasePrepareV2, ScopeDigest: scopeDigest})
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := evidenceGateway.ConsumeCheckpointPhaseEvidenceCurrentV1(context.Background(), ports.ConsumeCheckpointPhaseEvidenceRequestV1{ID: "checkpoint-consumption-v5-" + suffix, Qualification: qualification, Handoff: handoff, Record: record.Ref, Source: evidenceScope.Source})
	if err != nil {
		t.Fatal(err)
	}
	submission.Evidence = evidence
	submission.Handoff = handoff
	if err := submission.Validate(); err != nil {
		t.Fatal(err)
	}
	gateway := kernel.OperationCheckpointRestoreSettlementGatewayV5{Facts: effect.effect.effect.store, Inputs: domain, Reservations: domain, Participants: domain, DomainResults: domain, Evidence: evidenceGateway, Enforcement: effect.effect.enforcement, Clock: func() time.Time { return effect.now }}
	domain.inputCalls.Store(0)
	domain.reservationCalls.Store(0)
	domain.participantCalls.Store(0)
	domain.calls.Store(0)
	return checkpointSettlementFixtureV5{now: effect.now, effect: effect, domain: domain, gateway: gateway, submission: submission, qualification: qualificationRequest, evidence: evidenceStore, evidencePort: evidenceGateway, record: record}
}

type checkpointEvidenceAttemptReaderV1 struct {
	projection ports.CheckpointEvidenceAttemptCurrentProjectionV1
	second     *ports.CheckpointEvidenceAttemptCurrentProjectionV1
	calls      atomic.Int64
}

func (r *checkpointEvidenceAttemptReaderV1) InspectCheckpointEvidenceAttemptCurrentV1(_ context.Context, attempt ports.CheckpointAttemptRefV2, barrier ports.CheckpointBarrierLeaseRefV2, cut ports.EffectCutRefV2) (ports.CheckpointEvidenceAttemptCurrentProjectionV1, error) {
	projection := r.projection
	if r.calls.Add(1) > 1 && r.second != nil {
		projection = *r.second
	}
	if projection.Attempt != attempt || projection.Barrier != barrier || projection.EffectCut != cut {
		return ports.CheckpointEvidenceAttemptCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "fixture checkpoint Evidence current coordinates drifted")
	}
	return projection, nil
}

func newCheckpointEvidenceGatewayV1(t *testing.T, fixture checkpointSettlementFixtureV5, store *fakes.CheckpointEvidenceStoreV1) kernel.CheckpointRestoreEvidenceGatewayV1 {
	t.Helper()
	projection, err := ports.SealCheckpointEvidenceAttemptCurrentProjectionV1(ports.CheckpointEvidenceAttemptCurrentProjectionV1{Attempt: fixture.qualification.Attempt, Barrier: fixture.qualification.Barrier, EffectCut: fixture.qualification.EffectCut, Current: true, CheckedUnixNano: fixture.now.UnixNano(), ExpiresUnixNano: fixture.qualification.Barrier.ExpiresUnixNano}, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	policy := checkpointEvidencePolicyFactV1(t, fixture.qualification.Scope, fixture.now)
	source, err := ports.SealCheckpointEvidenceSourceCurrentProjectionV1(ports.CheckpointEvidenceSourceCurrentProjectionV1{Source: fixture.qualification.Scope.Source, Policy: fixture.qualification.Scope.EvidencePolicy, Schema: fixture.qualification.Scope.PayloadSchema, Current: true, CheckedUnixNano: fixture.now.UnixNano(), ExpiresUnixNano: fixture.now.Add(20 * time.Second).UnixNano()}, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	scope := fixture.qualification.Scope
	execution, err := ports.SealCheckpointEvidenceExecutionCurrentProjectionV1(ports.CheckpointEvidenceExecutionCurrentProjectionV1{Operation: scope.Operation, OperationDigest: scope.OperationDigest, EffectID: scope.EffectID, EffectRevision: scope.EffectRevision, IntentRevision: scope.DispatchAttempt.IntentRevision, IntentDigest: scope.IntentDigest, DispatchAttempt: scope.DispatchAttempt, PermitID: scope.PermitID, PermitFactRevision: scope.PermitFactRevision, PermitDigest: scope.PermitDigest, AuthorizedAdmissionDigest: scope.AuthorizedAdmissionDigest, Authorization: scope.Authorization, PrepareEnforcement: scope.PrepareEnforcement, ExecuteEnforcement: scope.ExecuteEnforcement, SandboxAttempt: scope.SandboxAttempt, SandboxProjectionDigest: scope.SandboxProjectionDigest, SandboxLease: scope.SandboxLease, FenceEpoch: scope.FenceEpoch, PayloadSchema: scope.PayloadSchema, PayloadDigest: scope.PayloadDigest, PayloadRevision: scope.PayloadRevision, Current: true, CheckedUnixNano: fixture.now.UnixNano(), ExpiresUnixNano: fixture.now.Add(8 * time.Second).UnixNano()}, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	return kernel.CheckpointRestoreEvidenceGatewayV1{Facts: store, Checkpoints: &checkpointEvidenceAttemptReaderV1{projection: projection}, Inputs: fixture.domain, Reservations: fixture.domain, Execution: checkpointEvidenceExecutionReaderV1{projection: execution}, Policies: checkpointEvidencePolicyReaderV1{fact: policy}, Sources: checkpointEvidenceSourceReaderV1{projection: source}, Records: checkpointEvidenceRecordReaderV1{record: fixture.record}, Clock: func() time.Time { return fixture.now }}
}

type checkpointEvidenceRecordReaderV1 struct {
	record ports.EvidenceLedgerRecordV2
}

func (r checkpointEvidenceRecordReaderV1) InspectRecord(_ context.Context, ref ports.EvidenceRecordRefV2) (ports.EvidenceLedgerRecordV2, error) {
	if r.record.Ref != ref {
		return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "checkpoint fixture Evidence record not found")
	}
	return r.record, nil
}

func (r checkpointEvidenceRecordReaderV1) InspectBySource(_ context.Context, source ports.EvidenceSourceKeyV2) (ports.EvidenceLedgerRecordV2, error) {
	candidate := r.record.Candidate
	if candidate.RegistrationID != source.RegistrationID || candidate.SourceEpoch != source.SourceEpoch || candidate.SourceSequence != source.SourceSequence {
		return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "checkpoint fixture Evidence source record not found")
	}
	return r.record, nil
}

type checkpointUnavailableEvidenceRecordReaderV1 struct{}

func (*checkpointUnavailableEvidenceRecordReaderV1) InspectRecord(context.Context, ports.EvidenceRecordRefV2) (ports.EvidenceLedgerRecordV2, error) {
	return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "checkpoint fixture Evidence record Reader unavailable")
}

func (*checkpointUnavailableEvidenceRecordReaderV1) InspectBySource(context.Context, ports.EvidenceSourceKeyV2) (ports.EvidenceLedgerRecordV2, error) {
	return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "checkpoint fixture Evidence source Reader unavailable")
}

type checkpointDriftingEvidenceRecordReaderV1 struct {
	first         ports.EvidenceLedgerRecordV2
	second        ports.EvidenceLedgerRecordV2
	bySourceCalls atomic.Int64
}

func (r *checkpointDriftingEvidenceRecordReaderV1) InspectRecord(_ context.Context, ref ports.EvidenceRecordRefV2) (ports.EvidenceLedgerRecordV2, error) {
	if r.first.Ref != ref {
		return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "checkpoint fixture Evidence record not found")
	}
	return r.first, nil
}

func (r *checkpointDriftingEvidenceRecordReaderV1) InspectBySource(_ context.Context, source ports.EvidenceSourceKeyV2) (ports.EvidenceLedgerRecordV2, error) {
	record := r.first
	if r.bySourceCalls.Add(1) > 1 {
		record = r.second
	}
	candidate := record.Candidate
	if candidate.RegistrationID != source.RegistrationID || candidate.SourceEpoch != source.SourceEpoch || candidate.SourceSequence != source.SourceSequence {
		return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "checkpoint fixture Evidence source record not found")
	}
	return record, nil
}

func createCheckpointEvidenceQualificationAndHandoffV1(t *testing.T, gateway *kernel.CheckpointRestoreEvidenceGatewayV1, fixture checkpointSettlementFixtureV5, suffix string) (ports.CheckpointRestoreEvidenceQualificationRefV1, ports.CheckpointRestoreEvidenceProviderHandoffRefV1) {
	t.Helper()
	qualification, err := gateway.IssueCheckpointPhaseQualificationV1(context.Background(), fixture.qualification)
	if err != nil {
		t.Fatal(err)
	}
	scopeDigest, err := fixture.qualification.Scope.DigestV1()
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := gateway.CreateCheckpointPhaseProviderHandoffV1(context.Background(), ports.CreateCheckpointPhaseProviderHandoffRequestV1{ID: "checkpoint-handoff-" + suffix, Qualification: qualification, Attempt: fixture.qualification.Scope.DispatchAttempt, Phase: fixture.qualification.Phase, ScopeDigest: scopeDigest})
	if err != nil {
		t.Fatal(err)
	}
	return qualification, handoff
}

type checkpointEvidenceExecutionReaderV1 struct {
	projection ports.CheckpointEvidenceExecutionCurrentProjectionV1
}

func (r checkpointEvidenceExecutionReaderV1) InspectCheckpointEvidenceExecutionCurrentV1(context.Context, ports.OperationSubjectV3, core.EffectIntentID, ports.OperationDispatchAttemptRefV3) (ports.CheckpointEvidenceExecutionCurrentProjectionV1, error) {
	return r.projection, nil
}

type checkpointEvidencePolicyReaderV1 struct {
	fact ports.OperationScopeEvidencePolicyFactV3
}

func (r checkpointEvidencePolicyReaderV1) InspectCurrentControlledOperationEvidencePolicyV2(_ context.Context, _ ports.OperationScopeEvidencePolicyRefV3) (ports.OperationScopeEvidencePolicyFactV3, error) {
	return r.fact, nil
}

func (r checkpointEvidencePolicyReaderV1) InspectCurrentControlledOperationApplicabilityPolicyV2(context.Context, ports.OperationScopeEvidenceApplicabilityPolicyRefV3) (ports.OperationScopeEvidenceApplicabilityPolicyFactV3, error) {
	return ports.OperationScopeEvidenceApplicabilityPolicyFactV3{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "checkpoint fixture has no applicability policy")
}

type checkpointEvidenceSourceReaderV1 struct {
	projection ports.CheckpointEvidenceSourceCurrentProjectionV1
}

func (r checkpointEvidenceSourceReaderV1) InspectCheckpointEvidenceSourceCurrentV1(context.Context, ports.EvidenceSourceKeyV2) (ports.CheckpointEvidenceSourceCurrentProjectionV1, error) {
	return r.projection, nil
}

func checkpointEvidenceScopeV1(t *testing.T, fixture *operationSettlementFixtureV4, attempt ports.OperationDispatchAttemptRefV3, suffix string) ports.CheckpointRestoreEvidenceScopeV1 {
	t.Helper()
	var prepare, execute ports.OperationDispatchEnforcementPhaseRefV4
	for _, binding := range fixture.submission.Evidence {
		if binding.Phase == ports.OperationDispatchEnforcementPrepareV4 {
			prepare = binding.EnforcementPhase
		} else if binding.Phase == ports.OperationDispatchEnforcementExecuteV4 {
			execute = binding.EnforcementPhase
		}
	}
	intent := fixture.effect.effect.intent
	intentDigest, err := intent.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	operationDigest, err := intent.Operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	lease := *intent.Operation.ExecutionScope.SandboxLease
	current, err := fixture.effect.dispatch.gateway.InspectOperationDispatchRecordV4(context.Background(), ports.InspectOperationDispatchRecordRequestV4{Operation: intent.Operation, EffectID: intent.ID, PermitID: fixture.effect.dispatch.issue.PermitID})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := ports.SealOperationScopeEvidencePolicyFactV3(ports.OperationScopeEvidencePolicyFactV3{ID: "checkpoint-evidence-policy-" + suffix, Revision: 1, State: ports.OperationScopeEvidencePolicyActiveV3, OperationKind: intent.Operation.Kind, EffectKind: intent.Kind, AllowedPhases: []ports.OperationDispatchEnforcementPhaseV4{ports.OperationDispatchEnforcementPrepareV4, ports.OperationDispatchEnforcementExecuteV4}, ExpectedSchema: intent.Payload.Schema, MaximumPayloadBytes: ports.MaxOperationScopeEvidencePayloadBytesV3, MaximumQualificationTTL: 30 * time.Second, MaximumIngestGrace: 0, ExpiresUnixNano: fixture.now.Add(20 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	scope := ports.CheckpointRestoreEvidenceScopeV1{Operation: intent.Operation, OperationDigest: operationDigest, EffectID: intent.ID, EffectRevision: fixture.submission.ExpectedEffectRevision, EffectKind: intent.Kind, IntentDigest: intentDigest, Admission: fixture.effect.dispatch.issue.Admission, Authorization: fixture.effect.dispatch.authorization.RefV4(), DispatchAttempt: attempt, PermitID: current.Permit.LegacyPermit.ID, PermitFactRevision: current.Revision, PermitDigest: current.PermitDigest, AuthorizedAdmissionDigest: current.Permit.Admission.Digest, PrepareEnforcement: prepare, ExecuteEnforcement: execute, Generation: ports.GenerationArtifactRefV1{ID: "checkpoint-generation-" + suffix, Revision: 1, Digest: digestCheckpointV2("checkpoint-generation-" + suffix), InputDigest: digestCheckpointV2("checkpoint-input-" + suffix), ManifestDigest: digestCheckpointV2("checkpoint-manifest-" + suffix), GraphDigest: digestCheckpointV2("checkpoint-graph-" + suffix), CatalogDigest: digestCheckpointV2("checkpoint-catalog-" + suffix)}, Assembly: fixture.effect.sandbox.value.Generation, SandboxAttempt: execute.SandboxAttempt, SandboxProjectionDigest: fixture.effect.sandbox.value.ProjectionDigest, SandboxLease: lease, FenceEpoch: lease.Epoch, Authority: intent.Authority, EvidencePolicy: policy.RefV3(), PayloadSchema: intent.Payload.Schema, PayloadDigest: intent.Payload.ContentDigest, PayloadRevision: intent.PayloadRevision, PayloadLength: intent.Payload.Length, Source: ports.EvidenceSourceKeyV2{RegistrationID: "checkpoint-source-" + suffix, SourceEpoch: 1, SourceSequence: 1}}
	if err := scope.Validate(); err != nil {
		t.Fatal(err)
	}
	return scope
}

func checkpointEvidencePolicyFactV1(t *testing.T, scope ports.CheckpointRestoreEvidenceScopeV1, now time.Time) ports.OperationScopeEvidencePolicyFactV3 {
	t.Helper()
	fact, err := ports.SealOperationScopeEvidencePolicyFactV3(ports.OperationScopeEvidencePolicyFactV3{ID: scope.EvidencePolicy.ID, Revision: scope.EvidencePolicy.Revision, State: ports.OperationScopeEvidencePolicyActiveV3, OperationKind: scope.Operation.Kind, EffectKind: scope.EffectKind, AllowedPhases: []ports.OperationDispatchEnforcementPhaseV4{ports.OperationDispatchEnforcementPrepareV4, ports.OperationDispatchEnforcementExecuteV4}, ExpectedSchema: scope.PayloadSchema, MaximumPayloadBytes: ports.MaxOperationScopeEvidencePayloadBytesV3, MaximumQualificationTTL: 30 * time.Second, MaximumIngestGrace: 0, ExpiresUnixNano: scope.EvidencePolicy.ExpiresUnixNano})
	if err != nil || fact.RefV3() != scope.EvidencePolicy {
		t.Fatalf("checkpoint Evidence policy fixture drifted: fact=%+v err=%v", fact, err)
	}
	return fact
}

func checkpointEvidenceScopeDigestV1(t *testing.T, scope ports.CheckpointRestoreEvidenceScopeV1) core.Digest {
	t.Helper()
	digest, err := scope.DigestV1()
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func checkpointSettlementOwnerBindingV5(t *testing.T, intent ports.OperationEffectIntentV3) ports.ProviderBindingRefV2 {
	t.Helper()
	for _, owner := range intent.Owners {
		if owner.Role == ports.OwnerSettlement {
			return ports.ProviderBindingRefV2{BindingSetID: "binding-set-checkpoint-v5", BindingSetRevision: 1, ComponentID: owner.ComponentID, ManifestDigest: owner.ManifestDigest, ArtifactDigest: digestCheckpointV2("checkpoint-owner-artifact"), Capability: "praxis.runtime/checkpoint-settlement"}
		}
	}
	t.Fatal("checkpoint settlement Owner missing")
	return ports.ProviderBindingRefV2{}
}

func checkpointEvidenceRecordRefV1(suffix string) ports.EvidenceRecordRefV2 {
	return ports.EvidenceRecordRefV2{LedgerScopeDigest: digestCheckpointV2("checkpoint-ledger-" + suffix), Sequence: 1, RecordDigest: digestCheckpointV2("checkpoint-record-" + suffix)}
}

func checkpointEvidenceLedgerRecordV1(t *testing.T, scope ports.CheckpointRestoreEvidenceScopeV1, owner ports.ProviderBindingRefV2, now time.Time, suffix string) ports.EvidenceLedgerRecordV2 {
	t.Helper()
	candidate := ports.EvidenceEventCandidateV2{
		ContractVersion:           ports.EvidenceContractVersionV2,
		LedgerScope:               ports.EvidenceLedgerScopeV2{Partition: ports.EvidencePartitionInstance, TenantID: scope.Operation.ExecutionScope.Identity.TenantID, IdentityID: scope.Operation.ExecutionScope.Identity.ID, LineageID: scope.Operation.ExecutionScope.Lineage.ID, InstanceID: scope.Operation.ExecutionScope.Instance.ID},
		EventID:                   "checkpoint-evidence-event-" + suffix,
		RegistrationID:            scope.Source.RegistrationID,
		RegistrationRevision:      1,
		SourceConfigurationDigest: digestCheckpointV2("checkpoint-evidence-source-configuration-" + suffix),
		SourcePolicy:              ports.EvidenceSourcePolicyBindingRefV2{Ref: "checkpoint-evidence-source-policy", Digest: digestCheckpointV2("checkpoint-evidence-source-policy"), Revision: 1},
		SourceID:                  "praxis.runtime/checkpoint-evidence-source",
		SourceEpoch:               scope.Source.SourceEpoch,
		SourceSequence:            scope.Source.SourceSequence,
		TrustClass:                ports.EvidenceTrustObservation,
		EventKind:                 "praxis.runtime/checkpoint-evidence",
		CustomClass:               "praxis.runtime/checkpoint-observation",
		ExecutionScope:            scope.Operation.ExecutionScope,
		Payload:                   ports.EvidencePayloadRefV2{Schema: scope.PayloadSchema, ContentDigest: scope.PayloadDigest, Revision: scope.PayloadRevision, Length: scope.PayloadLength, Ref: "memory://checkpoint-evidence/" + suffix},
		Causation:                 []ports.EvidenceCausationRefV2{},
		CorrelationID:             "checkpoint-evidence-correlation-" + suffix,
		Producer:                  ports.EvidenceProducerBindingRefV2(owner),
		Authority:                 scope.Authority,
		ObservedUnixNano:          now.UnixNano(),
	}
	record, err := control.NewEvidenceLedgerRecordV2(candidate, 1, ports.EvidenceGenesisDigestV2, now)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func retargetCheckpointSettlementSubmissionV5(t *testing.T, base ports.OperationCheckpointRestoreSettlementSubmissionV5, intent ports.OperationEffectIntentV3, attempt ports.OperationDispatchAttemptRefV3, suffix string) ports.OperationCheckpointRestoreSettlementSubmissionV5 {
	t.Helper()
	operationDigest, err := intent.Operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	checkpointAttempt := base.CheckpointAttempt
	checkpointAttempt.ID = "checkpoint-attempt-retarget-" + suffix
	checkpointAttempt.TenantID = intent.Operation.ExecutionScope.Identity.TenantID
	checkpointAttempt.Digest = digestCheckpointV2("checkpoint-attempt-retarget-" + suffix)
	value := base
	value.ID = "checkpoint-settlement-retarget-" + suffix
	value.Operation = intent.Operation
	value.OperationDigest = operationDigest
	value.EffectID = intent.ID
	value.ExpectedEffectRevision = 1
	value.CheckpointAttempt = checkpointAttempt
	value.DispatchAttempt = attempt
	value.Owner = checkpointSettlementOwnerBindingV5(t, intent)
	value.DomainResult.ID = "checkpoint-domain-retarget-" + suffix
	value.DomainResult.Attempt = checkpointAttempt
	value.DomainResult.Operation = intent.Operation
	value.DomainResult.OperationDigest = operationDigest
	value.DomainResult.Participant.Owner = value.Owner
	value.DomainResult.Digest = digestCheckpointV2("checkpoint-domain-retarget-" + suffix)
	value.Evidence.Qualification.Attempt = checkpointAttempt
	value.Evidence.Qualification.Barrier.TenantID = checkpointAttempt.TenantID
	value.Evidence.Qualification.Barrier.AttemptID = checkpointAttempt.ID
	value.Evidence.Qualification.Barrier.ID = "checkpoint-barrier-retarget-" + suffix
	value.Evidence.Qualification.Barrier.Digest = digestCheckpointV2("checkpoint-barrier-retarget-" + suffix)
	value.Evidence.Qualification.EffectCut.Attempt = checkpointAttempt
	value.Evidence.Qualification.EffectCut.ID = "checkpoint-cut-retarget-" + suffix
	value.Evidence.Qualification.EffectCut.Digest = digestCheckpointV2("checkpoint-cut-retarget-" + suffix)
	value.Evidence.Qualification.Reservation = value.Reservation
	value.Evidence.Qualification.Digest, _ = value.Evidence.Qualification.DigestV1()
	value.Evidence.Handoff.Qualification = value.Evidence.Qualification
	value.Evidence.Handoff.Attempt = attempt
	value.Evidence.Handoff.Digest, _ = value.Evidence.Handoff.DigestV1()
	value.Evidence.Attempt = checkpointAttempt
	value.Evidence.Digest, _ = value.Evidence.DigestV1()
	value.Handoff = value.Evidence.Handoff
	value.Enforcement.OperationDigest = operationDigest
	value.Enforcement.EffectID = intent.ID
	value.Enforcement.PermitID = attempt.PermitID
	value.Enforcement.AttemptID = attempt.AttemptID
	value.Enforcement.SandboxAttempt.ID = attempt.AttemptID
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	return value
}

func checkpointDomainProjectionV2(t *testing.T, ref ports.CheckpointParticipantDomainResultRefV2, now time.Time) ports.CheckpointParticipantDomainResultCurrentProjectionV2 {
	t.Helper()
	projection := ports.CheckpointParticipantDomainResultCurrentProjectionV2{Ref: ref, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	copyProjection := projection
	projection.ProjectionDigest, _ = core.CanonicalJSONDigest("praxis.runtime.checkpoint-governance", ports.CheckpointGovernanceContractVersionV2, "CheckpointParticipantDomainResultCurrentProjectionV2", copyProjection)
	if err := projection.Validate(now); err != nil {
		t.Fatal(err)
	}
	return projection
}

type checkpointDomainReaderV2 struct {
	projection        ports.CheckpointParticipantDomainResultCurrentProjectionV2
	inputs            ports.CheckpointAttemptInputsCurrentProjectionV2
	reservation       ports.CheckpointParticipantPhaseReservationCurrentProjectionV2
	participant       ports.CheckpointParticipantPhaseCurrentProjectionV2
	second            *ports.CheckpointParticipantDomainResultCurrentProjectionV2
	secondInputs      *ports.CheckpointAttemptInputsCurrentProjectionV2
	secondReservation *ports.CheckpointParticipantPhaseReservationCurrentProjectionV2
	secondParticipant *ports.CheckpointParticipantPhaseCurrentProjectionV2
	calls             atomic.Int64
	inputCalls        atomic.Int64
	reservationCalls  atomic.Int64
	participantCalls  atomic.Int64
}

func (r *checkpointDomainReaderV2) InspectCheckpointAttemptInputsCurrentV2(context.Context, ports.CheckpointAttemptRefV2) (ports.CheckpointAttemptInputsCurrentProjectionV2, error) {
	if r.inputCalls.Add(1) > 1 && r.secondInputs != nil {
		return *r.secondInputs, nil
	}
	return r.inputs, nil
}

func (r *checkpointDomainReaderV2) InspectCheckpointParticipantPhaseReservationCurrentV2(context.Context, ports.CheckpointParticipantPhaseReservationRefV2, ports.CheckpointParticipantPhaseV2) (ports.CheckpointParticipantPhaseReservationCurrentProjectionV2, error) {
	if r.reservationCalls.Add(1) > 1 && r.secondReservation != nil {
		return *r.secondReservation, nil
	}
	return r.reservation, nil
}

func (r *checkpointDomainReaderV2) InspectCheckpointParticipantPhaseCurrentV2(context.Context, ports.CheckpointParticipantPhaseRefV2) (ports.CheckpointParticipantPhaseCurrentProjectionV2, error) {
	if r.participantCalls.Add(1) > 1 && r.secondParticipant != nil {
		return *r.secondParticipant, nil
	}
	return r.participant, nil
}

func (r *checkpointDomainReaderV2) ReserveCheckpointPhaseV2(context.Context, ports.ReserveCheckpointParticipantPhaseRequestV2) (ports.CheckpointParticipantPhaseReservationRefV2, error) {
	return ports.CheckpointParticipantPhaseReservationRefV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownGovernanceCategory, "fixture does not reserve checkpoint phases")
}
func (r *checkpointDomainReaderV2) InspectCheckpointPhaseReservationHistoricalV2(context.Context, ports.CheckpointParticipantPhaseReservationRefV2) (ports.CheckpointParticipantPhaseReservationFactV2, error) {
	return ports.CheckpointParticipantPhaseReservationFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownGovernanceCategory, "fixture does not own checkpoint reservations")
}
func (r *checkpointDomainReaderV2) InspectCheckpointPhaseV2(context.Context, ports.CheckpointParticipantPhaseRefV2) (ports.CheckpointParticipantPhaseFactV2, error) {
	return ports.CheckpointParticipantPhaseFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownGovernanceCategory, "fixture does not own checkpoint phases")
}
func (r *checkpointDomainReaderV2) ReadCheckpointDomainResultCurrentV2(context.Context, ports.CheckpointParticipantDomainResultRefV2) (ports.CheckpointParticipantDomainResultCurrentProjectionV2, error) {
	if r.calls.Add(1) > 1 && r.second != nil {
		return *r.second, nil
	}
	return r.projection, nil
}
func (r *checkpointDomainReaderV2) ApplyCheckpointPhaseSettlementV2(context.Context, ports.ApplyCheckpointPhaseSettlementRequestV2) (ports.CheckpointParticipantPhaseFactV2, error) {
	return ports.CheckpointParticipantPhaseFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownGovernanceCategory, "Runtime gateway never applies participant settlement")
}

var _ ports.CheckpointRestoreParticipantGovernancePortV2 = (*checkpointDomainReaderV2)(nil)
var _ ports.CheckpointAttemptInputsCurrentReaderV2 = (*checkpointDomainReaderV2)(nil)
var _ ports.CheckpointParticipantPhaseReservationCurrentReaderV2 = (*checkpointDomainReaderV2)(nil)
var _ ports.CheckpointParticipantPhaseCurrentReaderV2 = (*checkpointDomainReaderV2)(nil)

func checkpointSettlementAttemptInputsProjectionV5(t *testing.T, submission ports.OperationCheckpointRestoreSettlementSubmissionV5, scope ports.CheckpointRestoreEvidenceScopeV1, now time.Time) ports.CheckpointAttemptInputsCurrentProjectionV2 {
	t.Helper()
	current := func(kind, id string) ports.CheckpointCurrentInputRefV2 {
		return checkpointCurrentInputRefV2(kind, id, now)
	}
	workflow := ports.CheckpointWorkflowRefV2{ID: "workflow-" + submission.CheckpointAttempt.ID, Revision: 1, Digest: digestCheckpointV2("workflow-" + submission.CheckpointAttempt.ID), NotAfter: now.Add(time.Minute).UnixNano()}
	binding := ports.RunBindingSetRefV2{ID: "binding-set-" + submission.CheckpointAttempt.ID, Revision: 1, Digest: digestCheckpointV2("binding-set-" + submission.CheckpointAttempt.ID), SemanticDigest: digestCheckpointV2("binding-semantic-" + submission.CheckpointAttempt.ID)}
	certification := ports.CheckpointParticipantSetCertificationRefV2{ID: "participant-cert-" + submission.CheckpointAttempt.ID, Revision: 1, Digest: digestCheckpointV2("participant-cert-" + submission.CheckpointAttempt.ID)}
	projection := ports.CheckpointAttemptInputsCurrentProjectionV2{AttemptID: submission.CheckpointAttempt.ID, TenantID: submission.CheckpointAttempt.TenantID, Run: current("praxis.runtime/run-current", "run-"+submission.CheckpointAttempt.ID), RunID: "run-" + core.AgentRunID(submission.CheckpointAttempt.ID), RunStableIdentityDigest: digestCheckpointV2("run-stable-" + submission.CheckpointAttempt.ID), Generation: current("praxis.runtime/generation-current", scope.Generation.ID), GenerationArtifact: scope.Generation, GenerationBinding: scope.Assembly, Binding: current("praxis.runtime/binding-current", binding.ID), BindingSet: binding, ParticipantCertification: current("praxis.runtime/participant-certification-current", certification.ID), ParticipantSetCertification: certification, WorkflowCurrent: current("praxis.runtime/workflow-current", workflow.ID), Workflow: workflow, Authority: current("praxis.runtime/authority-current", scope.Authority.Ref), AuthorityRef: scope.Authority, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	sealed, err := ports.SealCheckpointAttemptInputsCurrentProjectionV2(projection, now)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func checkpointSettlementReservationProjectionV5(t *testing.T, submission ports.OperationCheckpointRestoreSettlementSubmissionV5, scope ports.CheckpointRestoreEvidenceScopeV1, now time.Time, cut ports.EffectCutRefV2, barrier ports.CheckpointBarrierLeaseRefV2, participant ports.CheckpointParticipantRefV2, owner ports.ProviderBindingRefV2) ports.CheckpointParticipantPhaseReservationCurrentProjectionV2 {
	t.Helper()
	domain := ports.CheckpointParticipantDomainReservationRefV2{ID: "domain-reservation-" + submission.ID, Revision: 1, Digest: digestCheckpointV2("domain-reservation-" + submission.ID)}
	projection := ports.CheckpointParticipantPhaseReservationCurrentProjectionV2{ContractVersion: ports.CheckpointParticipantReservationContractVersionV2, Ref: submission.Reservation, Participant: participant, OwnerBinding: owner, Phase: submission.Phase, Attempt: submission.CheckpointAttempt, Barrier: barrier, EffectCut: cut, Operation: submission.Operation, OperationDigest: submission.OperationDigest, EffectID: submission.EffectID, EffectKind: scope.EffectKind, IntentDigest: scope.IntentDigest, Domain: domain, Generation: scope.Assembly, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	digest, err := projection.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	projection.ProjectionDigest = digest
	if err := projection.Validate(now); err != nil {
		t.Fatal(err)
	}
	return projection
}

func checkpointSettlementParticipantProjectionV5(t *testing.T, submission ports.OperationCheckpointRestoreSettlementSubmissionV5, now time.Time) ports.CheckpointParticipantPhaseCurrentProjectionV2 {
	t.Helper()
	projection := ports.CheckpointParticipantPhaseCurrentProjectionV2{Ref: submission.ParticipantFact, Reservation: submission.Reservation, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	copy := projection
	copy.ProjectionDigest = ""
	projection.ProjectionDigest, _ = core.CanonicalJSONDigest("praxis.runtime.checkpoint-governance", ports.CheckpointGovernanceContractVersionV2, "CheckpointParticipantPhaseCurrentProjectionV2", copy)
	if err := projection.Validate(now); err != nil {
		t.Fatal(err)
	}
	return projection
}

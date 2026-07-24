package runtimeadapter

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
)

type checkpointSettlementReaderStubV2 struct {
	value       runtimeports.OperationCheckpointRestoreSettlementInspectionV5
	driftSecond bool
	calls       atomic.Uint64
}

func (r *checkpointSettlementReaderStubV2) InspectCheckpointPhaseSettlementCurrentV5(context.Context, runtimeports.InspectCurrentOperationCheckpointRestoreSettlementRequestV5) (runtimeports.OperationCheckpointRestoreSettlementInspectionV5, error) {
	value := r.value
	if r.driftSecond && r.calls.Add(1) > 1 {
		value.CheckedUnixNano++
	}
	return value, nil
}

func TestCheckpointSettlementCurrentAdapterMapsExactRuntimeV5ClosureV2(t *testing.T) {
	fixture := checkpointDispatchReaderFixtureV1(t, "settlement-current", nil)
	domain := checkpointDomainResultFixtureV2(t, fixture.reservation)
	results := &checkpointResultStoreStubV2{value: domain}
	reader, settlement := checkpointRuntimeSettlementFixtureV2(t, domain, fixture.runtimeReservation)
	adapter, err := NewCheckpointSettlementCurrentAdapterV2(fixture.store, results, &checkpointRuntimeReservationReaderV1{value: fixture.runtimeReservation}, reader, func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	projection, err := adapter.InspectCheckpointPhaseSettlementCurrentV2(context.Background(), domain.ExactRef(), settlement)
	if err != nil {
		t.Fatal(err)
	}
	if projection.DomainResultRef != domain.ExactRef() || !contract.SameRef(projection.RuntimeSettlementRef, settlement) || projection.ValidateCurrent(testkit.FixedNow) != nil {
		t.Fatalf("checkpoint Runtime V5 projection drifted: %+v", projection)
	}
}

func TestCheckpointSettlementCurrentAdapterRejectsS1S2RuntimeDriftV2(t *testing.T) {
	fixture := checkpointDispatchReaderFixtureV1(t, "settlement-drift", nil)
	domain := checkpointDomainResultFixtureV2(t, fixture.reservation)
	results := &checkpointResultStoreStubV2{value: domain}
	reader, settlement := checkpointRuntimeSettlementFixtureV2(t, domain, fixture.runtimeReservation)
	reader.driftSecond = true
	adapter, err := NewCheckpointSettlementCurrentAdapterV2(fixture.store, results, &checkpointRuntimeReservationReaderV1{value: fixture.runtimeReservation}, reader, func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.InspectCheckpointPhaseSettlementCurrentV2(context.Background(), domain.ExactRef(), settlement); err == nil {
		t.Fatal("Runtime V5 Settlement S1/S2 drift was accepted")
	}
}

func checkpointRuntimeSettlementFixtureV2(t *testing.T, domain contract.CheckpointPhaseDomainResultV2, reservation runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2) (*checkpointSettlementReaderStubV2, contract.Ref) {
	t.Helper()
	now := testkit.FixedNow
	expires := domain.Meta.ExpiresUnixNano
	digest := func(value string) runtimecore.Digest { return runtimecore.DigestBytes([]byte(value)) }
	operationDigest, err := reservation.Operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	dispatch := runtimeports.OperationDispatchAttemptRefV3{OperationDigest: operationDigest, EffectID: reservation.EffectID, IntentRevision: 1, IntentDigest: reservation.IntentDigest, PermitID: "checkpoint-v5-permit", PermitRevision: 1, PermitDigest: digest("checkpoint-v5-permit"), AttemptID: "checkpoint-v5-dispatch"}
	review := runtimeports.OperationReviewAuthorizationRefV4{ID: "checkpoint-v5-review", Revision: 1, Digest: digest("checkpoint-v5-review")}
	sandboxAttempt := runtimeports.OperationDispatchSandboxFactRefV4{ID: dispatch.AttemptID, Revision: 1, Digest: digest("checkpoint-v5-sandbox-attempt"), ExpiresUnixNano: expires}
	enforcement := runtimeports.OperationDispatchEnforcementPhaseRefV4{OperationDigest: operationDigest, EffectID: reservation.EffectID, PermitID: dispatch.PermitID, PermitFactRevision: 1, PermitDigest: dispatch.PermitDigest, AdmissionDigest: digest("checkpoint-v5-admission"), ReviewAuthorization: review, AttemptID: dispatch.AttemptID, SandboxAttempt: sandboxAttempt, Phase: runtimeports.OperationDispatchEnforcementExecuteV4, ReceiptDigest: digest("checkpoint-v5-execute"), JournalRevision: 2, ValidatedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: expires, PrepareReceiptDigest: digest("checkpoint-v5-prepare"), PreparedAttemptDigest: digest("checkpoint-v5-prepared-attempt")}
	if err := dispatch.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := enforcement.Validate(); err != nil {
		t.Fatal(err)
	}
	phaseCurrent := runtimeports.CheckpointParticipantPhaseRefV2{ID: domain.Meta.ID + checkpointRuntimePhaseResultIDSuffixV2, Revision: runtimecore.Revision(domain.Meta.Revision), Phase: reservation.Phase, State: runtimeports.CheckpointParticipantPreparedV2, Digest: digest("checkpoint-v5-phase")}
	if err := phaseCurrent.Validate(); err != nil {
		t.Fatal(err)
	}
	scopeDigest := digest("checkpoint-v5-evidence-scope")
	qualification := runtimeports.CheckpointRestoreEvidenceQualificationRefV1{ID: "checkpoint-v5-qualification", Revision: 1, Attempt: reservation.Attempt, Barrier: reservation.Barrier, EffectCut: reservation.EffectCut, Reservation: reservation.Ref, Phase: reservation.Phase, ScopeDigest: scopeDigest, ExpiresUnixNano: expires}
	qualification.Digest, err = qualification.DigestV1()
	if err != nil {
		t.Fatal(err)
	}
	handoff := runtimeports.CheckpointRestoreEvidenceProviderHandoffRefV1{ID: "checkpoint-v5-handoff", Revision: 1, Qualification: qualification, Attempt: dispatch, Phase: reservation.Phase, ScopeDigest: scopeDigest}
	handoff.Digest, err = handoff.DigestV1()
	if err != nil {
		t.Fatal(err)
	}
	source := runtimeports.EvidenceSourceKeyV2{RegistrationID: "checkpoint-v5-source", SourceEpoch: 1, SourceSequence: 1}
	record := runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: digest("checkpoint-v5-ledger"), Sequence: 1, RecordDigest: digest("checkpoint-v5-record")}
	consumption := runtimeports.CheckpointRestoreEvidenceConsumptionRefV1{ID: "checkpoint-v5-consumption", Revision: 1, Qualification: qualification, Handoff: handoff, Record: record, Attempt: reservation.Attempt, Phase: reservation.Phase, State: runtimeports.CheckpointEvidenceConsumedCurrentV1, ScopeDigest: scopeDigest, Source: source}
	consumption.Digest, err = consumption.DigestV1()
	if err != nil {
		t.Fatal(err)
	}
	mappedDomain := checkpointRuntimeDomainResultRefV2(domain, reservation)
	submission := runtimeports.OperationCheckpointRestoreSettlementSubmissionV5{ID: "checkpoint-v5-settlement", Operation: reservation.Operation, OperationDigest: operationDigest, EffectID: reservation.EffectID, ExpectedEffectRevision: 1, CheckpointAttempt: reservation.Attempt, Phase: reservation.Phase, ParticipantFact: phaseCurrent, Reservation: reservation.Ref, DomainResult: mappedDomain, Evidence: consumption, Handoff: handoff, DispatchAttempt: dispatch, Enforcement: enforcement, Owner: reservation.OwnerBinding, SettledUnixNano: now.UnixNano()}
	if err := submission.Validate(); err != nil {
		t.Fatal(err)
	}
	bundle := checkpointSettlementBundleV2(t, submission)
	inspection := runtimeports.OperationCheckpointRestoreSettlementInspectionV5{Bundle: bundle, Current: true, CheckedUnixNano: now.UnixNano()}
	if err := inspection.Validate(); err != nil {
		t.Fatal(err)
	}
	settlement := contract.Ref{ID: bundle.Settlement.ID, Revision: uint64(bundle.Settlement.Revision), Digest: trimRuntimeDigestV1(bundle.Settlement.Digest)}
	return &checkpointSettlementReaderStubV2{value: inspection}, settlement
}

func checkpointSettlementBundleV2(t *testing.T, submission runtimeports.OperationCheckpointRestoreSettlementSubmissionV5) runtimeports.OperationCheckpointRestoreSettlementCommitBundleV5 {
	t.Helper()
	domain := "praxis.runtime.operation-settlement-checkpoint-restore"
	version := runtimeports.OperationCheckpointRestoreSettlementContractVersionV5
	settlementDigest, err := runtimecore.CanonicalJSONDigest(domain, version, "OperationCheckpointRestoreSettlementV5", submission)
	if err != nil {
		t.Fatal(err)
	}
	settlement := runtimeports.OperationCheckpointRestoreSettlementRefV5{ID: submission.ID, Revision: 1, TenantID: submission.Operation.ExecutionScope.Identity.TenantID, EffectID: submission.EffectID, Attempt: submission.CheckpointAttempt, Phase: submission.Phase, OperationDigest: submission.OperationDigest, Digest: settlementDigest}
	associationRef := runtimeports.OperationCheckpointRestoreSettlementAssociationRefV5{ID: submission.ID + "-association", Revision: 1, Settlement: settlement}
	associationRef.Digest, _ = runtimecore.CanonicalJSONDigest(domain, version, "OperationCheckpointRestoreSettlementAssociationRefV5", associationRef)
	guardRef := runtimeports.OperationCheckpointRestoreTerminalGuardRefV5{TenantID: settlement.TenantID, EffectID: settlement.EffectID, Revision: 1, Settlement: settlement}
	guardRef.Digest, _ = runtimecore.CanonicalJSONDigest(domain, version, "OperationCheckpointRestoreTerminalGuardRefV5", guardRef)
	projectionRef := runtimeports.OperationCheckpointRestoreTerminalProjectionRefV5{ID: submission.ID + "-projection", Revision: 1, Settlement: settlement}
	projectionRef.Digest, _ = runtimecore.CanonicalJSONDigest(domain, version, "OperationCheckpointRestoreTerminalProjectionRefV5", projectionRef)
	effectRef := runtimeports.OperationCheckpointRestoreEffectTerminalRefV5{TenantID: settlement.TenantID, EffectID: settlement.EffectID, PreviousRevision: submission.ExpectedEffectRevision, Revision: submission.ExpectedEffectRevision + 1, OperationDigest: submission.OperationDigest, Settlement: settlement}
	effectRef.Digest, _ = runtimecore.CanonicalJSONDigest(domain, version, "OperationCheckpointRestoreEffectTerminalRefV5", effectRef)
	bundle := runtimeports.OperationCheckpointRestoreSettlementCommitBundleV5{Submission: submission, Settlement: settlement, Association: runtimeports.OperationCheckpointRestoreSettlementAssociationV5{Ref: associationRef, SubmissionDigest: settlementDigest}, Guard: runtimeports.OperationCheckpointRestoreTerminalGuardV5{Ref: guardRef, OperationDigest: submission.OperationDigest}, Projection: runtimeports.OperationCheckpointRestoreTerminalProjectionV5{Ref: projectionRef, Association: associationRef, Guard: guardRef, DomainResult: submission.DomainResult}, EffectTerminal: runtimeports.OperationCheckpointRestoreEffectTerminalV5{Ref: effectRef, State: "settled", PublishedUnixNano: submission.SettledUnixNano}}
	if err := bundle.Validate(); err != nil {
		t.Fatal(err)
	}
	return bundle
}

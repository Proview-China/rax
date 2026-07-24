package sandbox_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/runtimeadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/storage/sqlite"
)

func TestCheckpointRuntimeV5CurrentToSandboxApplySettlementCASBlackBoxV2(t *testing.T) {
	ctx := context.Background()
	now := testkit.FixedNow
	clock := func() time.Time { return now }
	store, err := sqlite.OpenWithClock(ctx, filepath.Join(t.TempDir(), "sandbox.db"), clock)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	participant := testkit.CheckpointParticipant("runtime-v5-apply")
	if err := store.CreateCheckpointParticipant(ctx, participant); err != nil {
		t.Fatal(err)
	}
	reservation := testkit.CheckpointReservation(contract.CheckpointPhasePrepare, "runtime-v5-apply", participant, nil)
	controller, err := kernel.NewCheckpointController(store, clock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.ReservePhase(ctx, &reservation); err != nil {
		t.Fatal(err)
	}
	phaseProjection := checkpointResultProjectionV2(t, reservation, contract.CheckpointPhasePrepared, now)
	localSettlementReader := &checkpointSettlementCurrentReaderV2{}
	recorder, err := kernel.NewCheckpointPhaseResultOwnerV2(store, store, &checkpointResultCurrentReaderV2{value: phaseProjection}, localSettlementReader, clock, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	domain, err := recorder.RecordCheckpointPhaseDomainResultV2(ctx, &contract.RecordCheckpointPhaseDomainResultRequestV2{ReservationRef: reservation.Meta.Ref(), ExpectedProjectionDigest: phaseProjection.ProjectionDigest, RequestedNotAfter: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}

	runtimeReservation := checkpointRuntimeReservationBlackBoxV2(t, reservation)
	runtimeSettlement, settlementRef := checkpointRuntimeSettlementBlackBoxV2(t, domain, runtimeReservation)
	settlementCurrent, err := runtimeadapter.NewCheckpointSettlementCurrentAdapterV2(store, store, &checkpointRuntimeReservationReaderBlackBoxV2{value: runtimeReservation}, runtimeSettlement, clock)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := kernel.NewCheckpointPhaseResultOwnerV2(store, store, &checkpointResultCurrentReaderV2{value: phaseProjection}, settlementCurrent, clock, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	apply := &contract.CheckpointPhaseApplySettlementV2{DomainResultRef: domain.ExactRef(), RuntimeSettlementRef: settlementRef}
	fact, err := owner.ApplyCheckpointPhaseSettlementV2(ctx, apply)
	if err != nil {
		t.Fatal(err)
	}
	if fact.DomainResultRef.ID != domain.Meta.ID || !contract.SameRef(fact.RuntimeSettlementRef, settlementRef) || fact.State != contract.CheckpointPhasePrepared {
		t.Fatalf("Runtime V5 current did not close Sandbox ApplySettlement: %+v", fact)
	}
	current, err := store.InspectCheckpointParticipantCurrent(ctx, participant.Meta.ID)
	if err != nil || current.Closure == nil || !contract.SameCheckpointPhaseClosure(*current.Closure, fact.ClosureRef()) {
		t.Fatalf("Sandbox Participant CAS did not publish the exact closure: current=%+v err=%v", current, err)
	}
	replay, err := owner.ApplyCheckpointPhaseSettlementV2(ctx, apply)
	if err != nil || !contract.SameRef(replay.Meta.Ref(), fact.Meta.Ref()) {
		t.Fatalf("lost ApplySettlement reply did not recover the exact fact: replay=%+v err=%v", replay, err)
	}
}

type checkpointRuntimeReservationReaderBlackBoxV2 struct {
	value runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2
}

func (r *checkpointRuntimeReservationReaderBlackBoxV2) InspectCheckpointParticipantPhaseReservationCurrentV2(_ context.Context, expected runtimeports.CheckpointParticipantPhaseReservationRefV2, phase runtimeports.CheckpointParticipantPhaseV2) (runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2, error) {
	if r.value.Ref != expected || r.value.Phase != phase {
		return runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2{}, sandboxports.ErrNotFound
	}
	return r.value, nil
}

type checkpointRuntimeSettlementReaderBlackBoxV2 struct {
	value runtimeports.OperationCheckpointRestoreSettlementInspectionV5
}

func (r *checkpointRuntimeSettlementReaderBlackBoxV2) InspectCheckpointPhaseSettlementCurrentV5(context.Context, runtimeports.InspectCurrentOperationCheckpointRestoreSettlementRequestV5) (runtimeports.OperationCheckpointRestoreSettlementInspectionV5, error) {
	return r.value, nil
}

func checkpointRuntimeReservationBlackBoxV2(t *testing.T, local contract.CheckpointPhaseReservation) runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2 {
	t.Helper()
	lease := runtimecore.SandboxLeaseRef{ID: runtimecore.SandboxLeaseID(local.Runtime.LeaseID), Epoch: runtimecore.Epoch(local.Runtime.LeaseEpoch)}
	scope := runtimecore.ExecutionScope{
		Identity:       runtimecore.AgentIdentityRef{TenantID: runtimecore.TenantID(local.TenantID), ID: "checkpoint-identity", Epoch: 1},
		Lineage:        runtimecore.LineageRef{ID: "checkpoint-lineage", PlanDigest: runtimecore.DigestBytes([]byte("checkpoint-lineage"))},
		Instance:       runtimecore.InstanceRef{ID: runtimecore.AgentInstanceID(local.Runtime.InstanceID), Epoch: runtimecore.Epoch(local.Runtime.InstanceEpoch)},
		SandboxLease:   &lease,
		AuthorityEpoch: runtimecore.Epoch(local.Runtime.FenceEpoch),
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	operation := runtimeports.OperationSubjectV3{Kind: runtimeports.OperationScopeKindV3("praxis.checkpoint/participant"), ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, CustomOperationID: local.OperationID, SubjectRevision: 1, CurrentProjectionRef: "checkpoint-current-operation", CurrentProjectionRevision: 1, CurrentProjectionDigest: runtimecore.DigestBytes([]byte("checkpoint-current-operation"))}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	owner := runtimeports.ProviderBindingRefV2{BindingSetID: "checkpoint-binding-runtime-v5-apply", BindingSetRevision: 1, ComponentID: "praxis/sandbox", ManifestDigest: runtimecore.DigestBytes([]byte("checkpoint-manifest-runtime-v5-apply")), ArtifactDigest: runtimecore.DigestBytes([]byte("checkpoint-artifact-runtime-v5-apply")), Capability: "praxis.sandbox/checkpoint"}
	attempt := runtimeports.CheckpointAttemptRefV2{TenantID: runtimecore.TenantID(local.TenantID), ID: local.Base.CheckpointAttempt.ID, Revision: runtimecore.Revision(local.Base.CheckpointAttempt.Revision), Digest: runtimeDigestBlackBoxV2(local.Base.CheckpointAttempt.Digest)}
	expires := local.Meta.ExpiresUnixNano
	value := runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2{
		ContractVersion: runtimeports.CheckpointParticipantReservationContractVersionV2,
		Ref:             runtimeports.CheckpointParticipantPhaseReservationRefV2{ID: local.Meta.ID, Revision: runtimecore.Revision(local.Meta.Revision), Digest: runtimeDigestBlackBoxV2(local.Meta.Digest), ExpiresUnixNano: expires},
		Participant:     runtimeports.CheckpointParticipantRefV2{ID: local.ParticipantRef.ID, Owner: owner, Digest: runtimeDigestBlackBoxV2(local.ParticipantRef.Digest)},
		OwnerBinding:    owner,
		Phase:           runtimeports.CheckpointPhasePrepareV2,
		Attempt:         attempt,
		Barrier:         runtimeports.CheckpointBarrierLeaseRefV2{TenantID: attempt.TenantID, ID: local.Base.Barrier.ID, AttemptID: attempt.ID, Revision: runtimecore.Revision(local.Base.Barrier.Revision), Digest: runtimeDigestBlackBoxV2(local.Base.Barrier.Digest), ExpiresUnixNano: expires},
		EffectCut:       runtimeports.EffectCutRefV2{ID: local.Base.EffectCut.ID, Revision: runtimecore.Revision(local.Base.EffectCut.Revision), Attempt: attempt, RootDigest: runtimecore.DigestBytes([]byte("checkpoint-cut-root")), Watermark: 1, Digest: runtimeDigestBlackBoxV2(local.Base.EffectCut.Digest)},
		Operation:       operation, OperationDigest: operationDigest,
		EffectID: runtimecore.EffectIntentID(local.EffectID), EffectKind: "praxis.sandbox/checkpoint", IntentDigest: runtimecore.DigestBytes([]byte("checkpoint-intent-runtime-v5-apply")),
		Domain:          runtimeports.CheckpointParticipantDomainReservationRefV2{ID: "checkpoint-domain-" + local.Meta.ID, Revision: 1, Digest: runtimecore.DigestBytes([]byte("checkpoint-domain-" + local.Meta.ID))},
		Generation:      runtimeports.GenerationBindingAssociationRefV1{ID: local.Base.Generation.ID, Revision: runtimecore.Revision(local.Base.Generation.Revision), Digest: runtimeDigestBlackBoxV2(local.Base.Generation.Digest)},
		CheckedUnixNano: testkit.FixedNow.UnixNano(), ExpiresUnixNano: expires,
	}
	value.ProjectionDigest, err = value.DigestV2()
	if err != nil || value.Validate(testkit.FixedNow) != nil {
		t.Fatalf("build Runtime checkpoint Reservation: %v", err)
	}
	return value
}

func checkpointRuntimeSettlementBlackBoxV2(t *testing.T, domain contract.CheckpointPhaseDomainResultV2, reservation runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2) (*checkpointRuntimeSettlementReaderBlackBoxV2, contract.Ref) {
	t.Helper()
	now := testkit.FixedNow
	expires := domain.Meta.ExpiresUnixNano
	digest := func(value string) runtimecore.Digest { return runtimecore.DigestBytes([]byte(value)) }
	dispatch := runtimeports.OperationDispatchAttemptRefV3{OperationDigest: reservation.OperationDigest, EffectID: reservation.EffectID, IntentRevision: 1, IntentDigest: reservation.IntentDigest, PermitID: "checkpoint-v5-permit", PermitRevision: 1, PermitDigest: digest("checkpoint-v5-permit"), AttemptID: "checkpoint-v5-dispatch"}
	review := runtimeports.OperationReviewAuthorizationRefV4{ID: "checkpoint-v5-review", Revision: 1, Digest: digest("checkpoint-v5-review")}
	enforcement := runtimeports.OperationDispatchEnforcementPhaseRefV4{OperationDigest: reservation.OperationDigest, EffectID: reservation.EffectID, PermitID: dispatch.PermitID, PermitFactRevision: 1, PermitDigest: dispatch.PermitDigest, AdmissionDigest: digest("checkpoint-v5-admission"), ReviewAuthorization: review, AttemptID: dispatch.AttemptID, SandboxAttempt: runtimeports.OperationDispatchSandboxFactRefV4{ID: dispatch.AttemptID, Revision: 1, Digest: digest("checkpoint-v5-sandbox-attempt"), ExpiresUnixNano: expires}, Phase: runtimeports.OperationDispatchEnforcementExecuteV4, ReceiptDigest: digest("checkpoint-v5-execute"), JournalRevision: 2, ValidatedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: expires, PrepareReceiptDigest: digest("checkpoint-v5-prepare"), PreparedAttemptDigest: digest("checkpoint-v5-prepared-attempt")}
	if err := dispatch.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := enforcement.Validate(); err != nil {
		t.Fatal(err)
	}
	phaseCurrent := runtimeports.CheckpointParticipantPhaseRefV2{ID: domain.Meta.ID + "-owner-phase-result", Revision: runtimecore.Revision(domain.Meta.Revision), Phase: reservation.Phase, State: runtimeports.CheckpointParticipantPreparedV2, Digest: digest("checkpoint-v5-phase")}
	scopeDigest := digest("checkpoint-v5-evidence-scope")
	qualification := runtimeports.CheckpointRestoreEvidenceQualificationRefV1{ID: "checkpoint-v5-qualification", Revision: 1, Attempt: reservation.Attempt, Barrier: reservation.Barrier, EffectCut: reservation.EffectCut, Reservation: reservation.Ref, Phase: reservation.Phase, ScopeDigest: scopeDigest, ExpiresUnixNano: expires}
	qualification.Digest, _ = qualification.DigestV1()
	handoff := runtimeports.CheckpointRestoreEvidenceProviderHandoffRefV1{ID: "checkpoint-v5-handoff", Revision: 1, Qualification: qualification, Attempt: dispatch, Phase: reservation.Phase, ScopeDigest: scopeDigest}
	handoff.Digest, _ = handoff.DigestV1()
	source := runtimeports.EvidenceSourceKeyV2{RegistrationID: "checkpoint-v5-source", SourceEpoch: 1, SourceSequence: 1}
	consumption := runtimeports.CheckpointRestoreEvidenceConsumptionRefV1{ID: "checkpoint-v5-consumption", Revision: 1, Qualification: qualification, Handoff: handoff, Record: runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: digest("checkpoint-v5-ledger"), Sequence: 1, RecordDigest: digest("checkpoint-v5-record")}, Attempt: reservation.Attempt, Phase: reservation.Phase, State: runtimeports.CheckpointEvidenceConsumedCurrentV1, ScopeDigest: scopeDigest, Source: source}
	consumption.Digest, _ = consumption.DigestV1()
	mappedDomain := runtimeports.CheckpointParticipantDomainResultRefV2{ID: domain.Meta.ID, Revision: runtimecore.Revision(domain.Meta.Revision), Kind: "praxis.sandbox/checkpoint-phase-domain-result", Attempt: reservation.Attempt, Participant: reservation.Participant, Phase: reservation.Phase, Operation: reservation.Operation, OperationDigest: reservation.OperationDigest, Digest: runtimeDigestBlackBoxV2(domain.Meta.Digest)}
	submission := runtimeports.OperationCheckpointRestoreSettlementSubmissionV5{ID: "checkpoint-v5-settlement", Operation: reservation.Operation, OperationDigest: reservation.OperationDigest, EffectID: reservation.EffectID, ExpectedEffectRevision: 1, CheckpointAttempt: reservation.Attempt, Phase: reservation.Phase, ParticipantFact: phaseCurrent, Reservation: reservation.Ref, DomainResult: mappedDomain, Evidence: consumption, Handoff: handoff, DispatchAttempt: dispatch, Enforcement: enforcement, Owner: reservation.OwnerBinding, SettledUnixNano: now.UnixNano()}
	if err := submission.Validate(); err != nil {
		t.Fatal(err)
	}
	bundle := checkpointRuntimeSettlementBundleBlackBoxV2(t, submission)
	inspection := runtimeports.OperationCheckpointRestoreSettlementInspectionV5{Bundle: bundle, Current: true, CheckedUnixNano: now.UnixNano()}
	if err := inspection.Validate(); err != nil {
		t.Fatal(err)
	}
	ref := contract.Ref{ID: bundle.Settlement.ID, Revision: uint64(bundle.Settlement.Revision), Digest: trimRuntimeDigestBlackBoxV2(bundle.Settlement.Digest)}
	return &checkpointRuntimeSettlementReaderBlackBoxV2{value: inspection}, ref
}

func checkpointRuntimeSettlementBundleBlackBoxV2(t *testing.T, submission runtimeports.OperationCheckpointRestoreSettlementSubmissionV5) runtimeports.OperationCheckpointRestoreSettlementCommitBundleV5 {
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

func runtimeDigestBlackBoxV2(value string) runtimecore.Digest {
	if len(value) >= len("sha256:") && value[:len("sha256:")] == "sha256:" {
		return runtimecore.Digest(value)
	}
	return runtimecore.Digest("sha256:" + value)
}

func trimRuntimeDigestBlackBoxV2(value runtimecore.Digest) string {
	text := string(value)
	if len(text) > len("sha256:") && text[:len("sha256:")] == "sha256:" {
		return text[len("sha256:"):]
	}
	return text
}

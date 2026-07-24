package applicationadapter

import (
	"testing"
	"time"

	appcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/dataplaneadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
)

func TestCheckpointParticipantCommitFromPlansCarriesBothPhaseEvidenceV2(t *testing.T) {
	work, candidate, aggregate, _ := governedCheckpointFixtureV1(t, "phase-lifecycle")
	prepareEvidence := governedCheckpointExternalRefV1(candidate.Participant.Owner, work.Attempt.TenantID, work.Gate.ScopeDigest, work.Gate.RunID, aggregate.Prepare.Evidence.ID, "checkpoint_prepare_evidence_v1")
	prepareEvidence.Revision = aggregate.Prepare.Evidence.Revision
	prepareEvidence.Digest = aggregate.Prepare.Evidence.Digest
	commitEvidence := governedCheckpointExternalRefV1(candidate.Participant.Owner, work.Attempt.TenantID, work.Gate.ScopeDigest, work.Gate.RunID, aggregate.Terminal.Evidence.ID, "checkpoint_commit_evidence_v1")
	commitEvidence.Revision = aggregate.Terminal.Evidence.Revision
	commitEvidence.Digest = aggregate.Terminal.Evidence.Digest

	got, err := checkpointParticipantCommitFromPlansV2(aggregate.Prepare, *aggregate.Terminal, candidate, []appcontract.CheckpointExternalExactRefV1{prepareEvidence, commitEvidence})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Evidence) != 2 || got.Evidence[0] != prepareEvidence || got.Evidence[1] != commitEvidence || got.ValidateForAttemptV1(work.Participant, work.Attempt) != nil {
		t.Fatalf("checkpoint phase evidence was not preserved: %+v", got)
	}
}

func TestCheckpointEvidenceExternalMappingRejectsDigestDriftV2(t *testing.T) {
	work, candidate, aggregate, _ := governedCheckpointFixtureV1(t, "phase-evidence-map")
	external := governedCheckpointExternalRefV1(candidate.Participant.Owner, work.Attempt.TenantID, work.Gate.ScopeDigest, work.Gate.RunID, aggregate.Prepare.Evidence.ID, "checkpoint_prepare_evidence_v1")
	external.Revision = aggregate.Prepare.Evidence.Revision
	external.Digest = aggregate.Prepare.Evidence.Digest
	if !checkpointEvidenceExternalMatchesV2(external, aggregate.Prepare.Evidence) {
		t.Fatal("exact checkpoint Evidence mapping was rejected")
	}
	external.Digest = checkpointDigestV1("drift")
	if checkpointEvidenceExternalMatchesV2(external, aggregate.Prepare.Evidence) {
		t.Fatal("checkpoint Evidence digest drift was accepted")
	}
}

func TestCheckpointPhaseLifecycleRejectsIncompleteProductionCompositionV2(t *testing.T) {
	if _, err := NewGovernedCheckpointParticipantPhaseLifecycleV2(GovernedCheckpointParticipantPhaseLifecycleConfigV2{}); err == nil {
		t.Fatal("checkpoint phase lifecycle accepted an incomplete production composition")
	}
}

func TestCheckpointPhaseExecutionPlanSealBindsTTLAndRejectsTamperV2(t *testing.T) {
	plan := checkpointPhaseExecutionPlanFixtureV2(t, "sealed-plan")
	sealed, err := SealCheckpointPhaseExecutionPlanV2(plan, testkit.FixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if sealed.ExpiresUnixNano != plan.Provider.NotAfter.UnixNano() || sealed.ValidateCurrent(testkit.FixedNow) != nil {
		t.Fatalf("checkpoint phase plan TTL was not sealed to shortest current input: %+v", sealed)
	}
	tampered := sealed
	tampered.SettlementID += "-tampered"
	if tampered.ValidateCurrent(testkit.FixedNow) == nil {
		t.Fatal("checkpoint phase plan digest tamper was accepted")
	}
	if sealed.ValidateCurrent(time.Unix(0, sealed.ExpiresUnixNano)) == nil {
		t.Fatal("checkpoint phase plan accepted now == expires")
	}
}

func checkpointPhaseExecutionPlanFixtureV2(t *testing.T, suffix string) CheckpointPhaseExecutionPlanV2 {
	t.Helper()
	participant := testkit.CheckpointParticipant(suffix)
	reservation := testkit.CheckpointReservation(contract.CheckpointPhasePrepare, suffix, participant, nil)
	expires := testkit.FixedNow.Add(20 * time.Minute)
	lease := runtimecore.SandboxLeaseRef{ID: runtimecore.SandboxLeaseID(reservation.Runtime.LeaseID), Epoch: runtimecore.Epoch(reservation.Runtime.LeaseEpoch)}
	scope := runtimecore.ExecutionScope{Identity: runtimecore.AgentIdentityRef{TenantID: runtimecore.TenantID(participant.TenantID), ID: "checkpoint-plan-identity", Epoch: 1}, Lineage: runtimecore.LineageRef{ID: "checkpoint-plan-lineage", PlanDigest: checkpointDigestV1("checkpoint-plan-lineage")}, Instance: runtimecore.InstanceRef{ID: runtimecore.AgentInstanceID(reservation.Runtime.InstanceID), Epoch: runtimecore.Epoch(reservation.Runtime.InstanceEpoch)}, SandboxLease: &lease, AuthorityEpoch: runtimecore.Epoch(reservation.Runtime.FenceEpoch)}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	operation := runtimeports.OperationSubjectV3{Kind: "praxis.checkpoint/participant", ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, CustomOperationID: reservation.OperationID, SubjectRevision: 1, CurrentProjectionRef: "checkpoint-plan-current", CurrentProjectionRevision: 1, CurrentProjectionDigest: checkpointDigestV1("checkpoint-plan-current")}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	owner := runtimeports.ProviderBindingRefV2{BindingSetID: "checkpoint-plan-binding", BindingSetRevision: 1, ComponentID: "praxis/sandbox", ManifestDigest: checkpointDigestV1("checkpoint-plan-manifest"), ArtifactDigest: checkpointDigestV1("checkpoint-plan-artifact"), Capability: "praxis.sandbox/checkpoint"}
	runtimeParticipant := runtimeports.CheckpointParticipantRefV2{ID: participant.Meta.ID, Owner: owner, Digest: checkpointRuntimeDigestV2(participant.Meta.Digest)}
	attempt := runtimeports.CheckpointAttemptRefV2{TenantID: scope.Identity.TenantID, ID: participant.CheckpointAttemptRef.ID, Revision: runtimecore.Revision(participant.CheckpointAttemptRef.Revision), Digest: checkpointRuntimeDigestV2(participant.CheckpointAttemptRef.Digest)}
	barrier := runtimeports.CheckpointBarrierLeaseRefV2{TenantID: attempt.TenantID, ID: reservation.Base.Barrier.ID, AttemptID: attempt.ID, Revision: runtimecore.Revision(reservation.Base.Barrier.Revision), Digest: checkpointRuntimeDigestV2(reservation.Base.Barrier.Digest), ExpiresUnixNano: expires.Add(time.Minute).UnixNano()}
	cut := runtimeports.EffectCutRefV2{ID: reservation.Base.EffectCut.ID, Revision: runtimecore.Revision(reservation.Base.EffectCut.Revision), Attempt: attempt, RootDigest: checkpointDigestV1("checkpoint-plan-cut-root"), Watermark: 1, Digest: checkpointRuntimeDigestV2(reservation.Base.EffectCut.Digest)}
	run := runtimecore.AgentRunID("checkpoint-plan-run")
	gate := governedCheckpointExternalRefV1(owner, attempt.TenantID, scopeDigest, run, "checkpoint-plan-gate", "checkpoint_gate_v1")
	snapshot := governedCheckpointExternalRefV1(owner, attempt.TenantID, scopeDigest, run, "checkpoint-plan-snapshot", "checkpoint_snapshot_v1")
	work := appcontract.CheckpointParticipantWorkRequestV1{Attempt: attempt, Barrier: barrier, EffectCut: cut, Participant: runtimeParticipant, Gate: gate, Snapshot: snapshot, NotAfter: expires.Add(10 * time.Minute).UnixNano()}
	reservationRef := runtimeports.CheckpointParticipantPhaseReservationRefV2{ID: reservation.Meta.ID, Revision: runtimecore.Revision(reservation.Meta.Revision), Digest: checkpointRuntimeDigestV2(reservation.Meta.Digest), ExpiresUnixNano: reservation.Meta.ExpiresUnixNano}
	effectID := runtimecore.EffectIntentID(reservation.EffectID)
	intentDigest := checkpointDigestV1("checkpoint-plan-intent")
	permitDigest := checkpointDigestV1("checkpoint-plan-permit")
	admissionDigest := checkpointDigestV1("checkpoint-plan-admission")
	review := runtimeports.OperationReviewAuthorizationRefV4{ID: "checkpoint-plan-review", Revision: 1, Digest: checkpointDigestV1("checkpoint-plan-review")}
	prepare := runtimeports.EnforceCurrentCheckpointRestoreDispatchRequestV1{Operation: operation, EffectID: effectID, PermitID: "checkpoint-plan-permit", ExpectedPermitFactRevision: 1, PermitDigest: permitDigest, AdmissionDigest: admissionDigest, ReviewAuthorization: review, AttemptID: reservation.ExpectedRuntimeAttemptRef.ID, Phase: runtimeports.OperationDispatchEnforcementPrepareV4, Reservation: reservationRef, SandboxProjectionDigest: checkpointDigestV1("checkpoint-plan-sandbox-current"), Verifier: owner}
	execute := prepare
	execute.Phase = runtimeports.OperationDispatchEnforcementExecuteV4
	dispatch := runtimeports.OperationDispatchAttemptRefV3{OperationDigest: operationDigest, EffectID: effectID, IntentRevision: 1, IntentDigest: intentDigest, PermitID: prepare.PermitID, PermitRevision: 1, PermitDigest: permitDigest, AttemptID: prepare.AttemptID}
	source := runtimeports.EvidenceSourceKeyV2{RegistrationID: "checkpoint-plan-source", SourceEpoch: 1, SourceSequence: 1}
	provider := CheckpointProviderPlanV1{Prepare: prepare, Execute: execute, CheckpointAttempt: attempt, Barrier: barrier, EffectCut: cut, Reservation: reservationRef, Phase: runtimeports.CheckpointPhasePrepareV2, DeclaredDelegation: runtimeports.ExecutionDelegationRefV2{ID: "checkpoint-plan-delegation", Revision: 1, Digest: checkpointDigestV1("checkpoint-plan-delegation")}, PrepareRequestID: "checkpoint-plan-prepare", ExecuteRequestID: "checkpoint-plan-execute", PayloadSchema: "praxis.sandbox/checkpoint/v1", PayloadRevision: 1, Payload: dataplaneadapter.ProviderPayloadV1{ProviderKind: "host_workspace"}, NotAfter: expires, QualificationID: "checkpoint-plan-qualification", HandoffID: "checkpoint-plan-handoff", ConsumptionID: "checkpoint-plan-consumption", EvidenceScope: runtimeports.CheckpointRestoreEvidenceScopeV1{Operation: operation, EffectID: effectID, DispatchAttempt: dispatch, Source: source}, EvidenceEvent: runtimeports.EvidenceEventCandidateV2{ContractVersion: runtimeports.EvidenceContractVersionV2}}
	evidence := governedCheckpointExternalRefV1(owner, attempt.TenantID, scopeDigest, run, "checkpoint-plan-consumption", "checkpoint_evidence_v1")
	storageNamespace := contract.SnapshotArtifactExactRefV2{TypeURL: "praxis.sandbox/checkpoint-store-namespace/v1", Version: 1, ID: "checkpoint-store", Revision: 1, DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: "praxis.sandbox/checkpoint-store-namespace/body/v1", Digest: testkit.Ref("checkpoint-store").Digest, ExpiresUnixNano: expires.Add(time.Minute).UnixNano()}
	capture := &CheckpointSnapshotCapturePlanV2{DataDomain: contract.WorkspaceSnapshotDataDomain, SchemaRef: testkit.Ref("checkpoint-snapshot-schema"), RetentionPolicyRef: testkit.Ref("checkpoint-retention"), EncryptionPolicyRef: testkit.Ref("checkpoint-encryption-policy"), ResidencyPolicyRef: testkit.Ref("checkpoint-residency-policy"), StorageNamespaceExactRef: storageNamespace, EncryptionFactRef: testkit.Ref("checkpoint-encryption-fact"), ResidencyFactRef: testkit.Ref("checkpoint-residency-fact"), WorkspaceStableID: "checkpoint-plan-workspace", CoveragePolicyRef: testkit.Ref("checkpoint-coverage-policy"), Included: []string{"workspace/content", "workspace/metadata"}, DeclaredExcluded: []string{"device_state", "network_session", "process_state", "secret_material"}, ResidualRefs: []contract.Ref{}, RequestedNotAfter: expires.Add(time.Minute).UnixNano()}
	return CheckpointPhaseExecutionPlanV2{Work: work, ParticipantBootstrap: participant, Reservation: reservation, Provider: provider, SettlementID: "checkpoint-plan-settlement", EvidenceExternal: evidence, SnapshotCapture: capture}
}

package applicationadapter

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	appcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestGovernedCheckpointParticipantOrdersPrepareCurrentCommitCurrent(t *testing.T) {
	work, candidate, closure, clock := governedCheckpointFixtureV1(t, "ordered")
	trace := []string{}
	current := &governedCheckpointCurrentStubV1{candidate: candidate, trace: &trace}
	lifecycle := &governedCheckpointLifecycleStubV1{closure: closure, candidate: candidate, trace: &trace}
	adapter, err := NewGovernedCheckpointParticipantApplicationAdapterV1(GovernedCheckpointParticipantApplicationAdapterConfigV1{ParticipantID: work.Participant.ID, Current: current, Lifecycle: lifecycle, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	got, err := adapter.CompleteCheckpointParticipantV1(context.Background(), work)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"prepare", "current", "commit", "current"}
	if len(trace) != len(want) {
		t.Fatalf("unexpected checkpoint order: %v", trace)
	}
	for index := range want {
		if trace[index] != want[index] {
			t.Fatalf("unexpected checkpoint order: %v", trace)
		}
	}
	if got.RuntimeClosure.Prepare != closure.Prepare || lifecycle.prepareCalls != 1 || lifecycle.commitCalls != 1 {
		t.Fatalf("checkpoint adapter did not preserve one exact prepare: %+v", got.RuntimeClosure)
	}
}

func TestGovernedCheckpointParticipantLostRepliesOnlyInspectOriginalIdentity(t *testing.T) {
	work, candidate, closure, clock := governedCheckpointFixtureV1(t, "lost")
	trace := []string{}
	lifecycle := &governedCheckpointLifecycleStubV1{closure: closure, candidate: candidate, trace: &trace, losePrepareReply: true, loseCommitReply: true}
	adapter, err := NewGovernedCheckpointParticipantApplicationAdapterV1(GovernedCheckpointParticipantApplicationAdapterConfigV1{ParticipantID: work.Participant.ID, Current: &governedCheckpointCurrentStubV1{candidate: candidate, trace: &trace}, Lifecycle: lifecycle, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.CompleteCheckpointParticipantV1(context.Background(), work); err != nil {
		t.Fatal(err)
	}
	if lifecycle.prepareCalls != 1 || lifecycle.commitCalls != 1 || lifecycle.inspectPrepareCalls != 1 || lifecycle.inspectCommitCalls != 1 {
		t.Fatalf("lost reply retried a phase instead of Inspect: %+v", lifecycle)
	}
}

func TestGovernedCheckpointParticipantRejectsPreparedClosureDriftBeforeOwnerRead(t *testing.T) {
	work, candidate, closure, clock := governedCheckpointFixtureV1(t, "drift")
	drift := closure.Prepare
	drift.DomainResult.Participant.ID = "other-participant"
	trace := []string{}
	lifecycle := &governedCheckpointLifecycleStubV1{closure: closure, candidate: candidate, trace: &trace, preparedOverride: &drift}
	adapter, err := NewGovernedCheckpointParticipantApplicationAdapterV1(GovernedCheckpointParticipantApplicationAdapterConfigV1{ParticipantID: work.Participant.ID, Current: &governedCheckpointCurrentStubV1{candidate: candidate, trace: &trace}, Lifecycle: lifecycle, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.CompleteCheckpointParticipantV1(context.Background(), work); err == nil {
		t.Fatal("cross-participant prepared closure was accepted")
	}
	if len(trace) != 1 || trace[0] != "prepare" {
		t.Fatalf("Owner current was read after invalid prepare: %v", trace)
	}
}

func TestGovernedCheckpointParticipantRejectsTypedNilLifecycle(t *testing.T) {
	work, candidate, _, clock := governedCheckpointFixtureV1(t, "typed-nil")
	var lifecycle *governedCheckpointLifecycleStubV1
	if _, err := NewGovernedCheckpointParticipantApplicationAdapterV1(GovernedCheckpointParticipantApplicationAdapterConfigV1{ParticipantID: work.Participant.ID, Current: &governedCheckpointCurrentStubV1{candidate: candidate, trace: &[]string{}}, Lifecycle: lifecycle, Clock: clock}); err == nil {
		t.Fatal("typed-nil checkpoint lifecycle was accepted")
	}
}

type governedCheckpointCurrentStubV1 struct {
	candidate appcontract.CheckpointParticipantOwnerCandidateV1
	trace     *[]string
}

func (s *governedCheckpointCurrentStubV1) InspectCheckpointParticipantOwnerCurrentV1(context.Context, appcontract.CheckpointParticipantWorkRequestV1) (appcontract.CheckpointParticipantOwnerCandidateV1, error) {
	*s.trace = append(*s.trace, "current")
	return s.candidate, nil
}

type governedCheckpointLifecycleStubV1 struct {
	mu                  sync.Mutex
	closure             runtimeports.CheckpointParticipantClosureRefV2
	candidate           appcontract.CheckpointParticipantOwnerCandidateV1
	trace               *[]string
	preparedOverride    *runtimeports.CheckpointParticipantPhaseClosureRefV2
	losePrepareReply    bool
	loseCommitReply     bool
	prepareCalls        int
	commitCalls         int
	inspectPrepareCalls int
	inspectCommitCalls  int
}

func (s *governedCheckpointLifecycleStubV1) PrepareCheckpointParticipantPhaseV1(context.Context, appcontract.CheckpointParticipantWorkRequestV1) (runtimeports.CheckpointParticipantPhaseClosureRefV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	*s.trace = append(*s.trace, "prepare")
	s.prepareCalls++
	if s.preparedOverride != nil {
		return *s.preparedOverride, nil
	}
	if s.losePrepareReply {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "injected prepare reply loss")
	}
	return s.closure.Prepare, nil
}

func (s *governedCheckpointLifecycleStubV1) InspectCheckpointParticipantPrepareV1(context.Context, runtimeports.CheckpointAttemptRefV2, runtimeports.CheckpointParticipantRefV2) (runtimeports.CheckpointParticipantPhaseClosureRefV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inspectPrepareCalls++
	return s.closure.Prepare, nil
}

func (s *governedCheckpointLifecycleStubV1) CommitCheckpointParticipantPhaseV1(_ context.Context, _ appcontract.CheckpointParticipantWorkRequestV1, prepared runtimeports.CheckpointParticipantPhaseClosureRefV2, candidate appcontract.CheckpointParticipantOwnerCandidateV1) (appcontract.CheckpointParticipantCommitV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	*s.trace = append(*s.trace, "commit")
	s.commitCalls++
	if prepared != s.closure.Prepare || candidate.ProjectionDigest != s.candidate.ProjectionDigest {
		return appcontract.CheckpointParticipantCommitV1{}, errors.New("commit inputs drifted")
	}
	if s.loseCommitReply {
		return appcontract.CheckpointParticipantCommitV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonCheckpointInconsistent, "injected commit reply loss")
	}
	return governedCheckpointCommitV1(s.closure, s.candidate), nil
}

func (s *governedCheckpointLifecycleStubV1) InspectCheckpointParticipantPhaseV1(context.Context, runtimeports.CheckpointAttemptRefV2, runtimeports.CheckpointParticipantRefV2) (appcontract.CheckpointParticipantCommitV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inspectCommitCalls++
	return governedCheckpointCommitV1(s.closure, s.candidate), nil
}

func governedCheckpointCommitV1(closure runtimeports.CheckpointParticipantClosureRefV2, candidate appcontract.CheckpointParticipantOwnerCandidateV1) appcontract.CheckpointParticipantCommitV1 {
	evidence := candidate.Snapshot
	evidence.ID, evidence.Digest, evidence.FactKind = "evidence-"+candidate.Participant.ID, checkpointDigestV1("evidence-"+candidate.Participant.ID), "evidence_record_v3"
	return appcontract.CheckpointParticipantCommitV1{RuntimeClosure: closure, ParticipantFact: candidate.ParticipantFact, Snapshot: candidate.Snapshot, Coverage: candidate.Coverage, Evidence: []appcontract.CheckpointExternalExactRefV1{evidence}, Residuals: []appcontract.CheckpointExternalExactRefV1{}}
}

func governedCheckpointFixtureV1(t *testing.T, suffix string) (appcontract.CheckpointParticipantWorkRequestV1, appcontract.CheckpointParticipantOwnerCandidateV1, runtimeports.CheckpointParticipantClosureRefV2, func() time.Time) {
	t.Helper()
	now := time.Unix(1_900_300_000, 0).UTC()
	clock := func() time.Time { return now }
	lease := core.SandboxLeaseRef{ID: core.SandboxLeaseID("lease-" + suffix), Epoch: 1}
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: core.TenantID("tenant-" + suffix), ID: core.AgentIdentityID("identity-" + suffix), Epoch: 1}, Lineage: core.LineageRef{ID: core.InstanceLineageID("lineage-" + suffix), PlanDigest: checkpointDigestV1("plan-" + suffix)}, Instance: core.InstanceRef{ID: core.AgentInstanceID("instance-" + suffix), Epoch: 1}, SandboxLease: &lease, AuthorityEpoch: 1}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	owner := runtimeports.ProviderBindingRefV2{BindingSetID: "binding-" + suffix, BindingSetRevision: 1, ComponentID: "praxis/sandbox", ManifestDigest: checkpointDigestV1("manifest-" + suffix), ArtifactDigest: checkpointDigestV1("artifact-" + suffix), Capability: "praxis.sandbox/checkpoint-participant-v1"}
	participant := runtimeports.CheckpointParticipantRefV2{ID: "participant-" + suffix, Owner: owner, Digest: checkpointDigestV1("participant-" + suffix)}
	attempt := runtimeports.CheckpointAttemptRefV2{TenantID: scope.Identity.TenantID, ID: "checkpoint-attempt-" + suffix, Revision: 1, Digest: checkpointDigestV1("checkpoint-attempt-" + suffix)}
	barrier := runtimeports.CheckpointBarrierLeaseRefV2{TenantID: attempt.TenantID, ID: "barrier-" + suffix, AttemptID: attempt.ID, Revision: 1, Digest: checkpointDigestV1("barrier-" + suffix), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	cut := runtimeports.EffectCutRefV2{ID: "cut-" + suffix, Revision: 1, Attempt: attempt, RootDigest: checkpointDigestV1("cut-root-" + suffix), Watermark: 1, Digest: checkpointDigestV1("cut-" + suffix)}
	gate := governedCheckpointExternalRefV1(owner, attempt.TenantID, scopeDigest, core.AgentRunID("run-"+suffix), "gate-"+suffix, "gate_fact_v1")
	snapshot := governedCheckpointExternalRefV1(owner, attempt.TenantID, scopeDigest, core.AgentRunID("run-"+suffix), "snapshot-gate-"+suffix, "snapshot_gate_fact_v1")
	work := appcontract.CheckpointParticipantWorkRequestV1{Attempt: attempt, Barrier: barrier, EffectCut: cut, Participant: participant, Gate: gate, Snapshot: snapshot, NotAfter: now.Add(time.Minute).UnixNano()}
	fact := governedCheckpointExternalRefV1(owner, attempt.TenantID, scopeDigest, gate.RunID, "participant-fact-"+suffix, "sandbox_checkpoint_participant_fact_v2")
	artifact := governedCheckpointExternalRefV1(owner, attempt.TenantID, scopeDigest, gate.RunID, "snapshot-artifact-"+suffix, "snapshot_artifact_fact_v2")
	coverage := governedCheckpointExternalRefV1(owner, attempt.TenantID, scopeDigest, gate.RunID, "snapshot-coverage-"+suffix, "snapshot_coverage_fact_v2")
	candidate, err := appcontract.SealCheckpointParticipantOwnerCandidateV1(appcontract.CheckpointParticipantOwnerCandidateV1{Participant: participant, ParticipantFact: fact, Snapshot: artifact, Coverage: coverage, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: work.NotAfter}, work, now)
	if err != nil {
		t.Fatal(err)
	}
	prepare := governedCheckpointPhaseClosureV1(t, scope, attempt, barrier, cut, participant, runtimeports.CheckpointPhasePrepareV2, runtimeports.CheckpointParticipantPreparedV2, suffix, now)
	commit := governedCheckpointPhaseClosureV1(t, scope, attempt, barrier, cut, participant, runtimeports.CheckpointPhaseCommitV2, runtimeports.CheckpointParticipantCommittedV2, suffix, now)
	closure := runtimeports.CheckpointParticipantClosureRefV2{ID: "closure-" + suffix, Participant: participant, Prepare: prepare, Terminal: &commit}
	closure.Digest, err = closure.DigestV2()
	if err != nil || closure.Validate() != nil {
		t.Fatalf("build closure: %v", err)
	}
	return work, candidate, closure, clock
}

func governedCheckpointPhaseClosureV1(t *testing.T, scope core.ExecutionScope, attempt runtimeports.CheckpointAttemptRefV2, barrier runtimeports.CheckpointBarrierLeaseRefV2, cut runtimeports.EffectCutRefV2, participant runtimeports.CheckpointParticipantRefV2, phase runtimeports.CheckpointParticipantPhaseV2, state runtimeports.CheckpointParticipantPhaseStateV2, suffix string, now time.Time) runtimeports.CheckpointParticipantPhaseClosureRefV2 {
	t.Helper()
	phaseSuffix := string(phase) + "-" + suffix
	scopeDigest, _ := runtimeports.ExecutionScopeDigestV2(scope)
	operation := runtimeports.OperationSubjectV3{Kind: "praxis.checkpoint/participant", ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, CustomOperationID: "operation-" + phaseSuffix, SubjectRevision: 1, CurrentProjectionRef: "current-" + phaseSuffix, CurrentProjectionDigest: checkpointDigestV1("current-" + phaseSuffix), CurrentProjectionRevision: 1}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	reservation := runtimeports.CheckpointParticipantPhaseReservationRefV2{ID: "reservation-" + phaseSuffix, Revision: 1, Digest: checkpointDigestV1("reservation-" + phaseSuffix), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	phaseFact := runtimeports.CheckpointParticipantPhaseRefV2{ID: "phase-" + phaseSuffix, Revision: 1, Phase: phase, State: state, Digest: checkpointDigestV1("phase-" + phaseSuffix)}
	domain := runtimeports.CheckpointParticipantDomainResultRefV2{ID: "domain-" + phaseSuffix, Revision: 1, Kind: "praxis.checkpoint/domain-result", Attempt: attempt, Participant: participant, Phase: phase, Operation: operation, OperationDigest: operationDigest, Digest: checkpointDigestV1("domain-" + phaseSuffix)}
	effectID := core.EffectIntentID("effect-" + phaseSuffix)
	dispatch := runtimeports.OperationDispatchAttemptRefV3{OperationDigest: operationDigest, EffectID: effectID, IntentRevision: 1, IntentDigest: checkpointDigestV1("intent-" + phaseSuffix), PermitID: "permit-" + phaseSuffix, PermitRevision: 1, PermitDigest: checkpointDigestV1("permit-" + phaseSuffix), AttemptID: "dispatch-" + phaseSuffix}
	evidenceScope := checkpointDigestV1("evidence-scope-" + phaseSuffix)
	qualification := runtimeports.CheckpointRestoreEvidenceQualificationRefV1{ID: "qualification-" + phaseSuffix, Revision: 1, Attempt: attempt, Barrier: barrier, EffectCut: cut, Reservation: reservation, Phase: phase, ScopeDigest: evidenceScope, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	qualification.Digest, _ = qualification.DigestV1()
	handoff := runtimeports.CheckpointRestoreEvidenceProviderHandoffRefV1{ID: "handoff-" + phaseSuffix, Revision: 1, Qualification: qualification, Attempt: dispatch, Phase: phase, ScopeDigest: evidenceScope}
	handoff.Digest, _ = handoff.DigestV1()
	source := runtimeports.EvidenceSourceKeyV2{RegistrationID: "source-" + phaseSuffix, SourceEpoch: 1, SourceSequence: 1}
	evidence := runtimeports.CheckpointRestoreEvidenceConsumptionRefV1{ID: "consumption-" + phaseSuffix, Revision: 1, Qualification: qualification, Handoff: handoff, Record: runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: evidenceScope, Sequence: 1, RecordDigest: checkpointDigestV1("record-" + phaseSuffix)}, Attempt: attempt, Phase: phase, State: runtimeports.CheckpointEvidenceConsumedCurrentV1, ScopeDigest: evidenceScope, Source: source}
	evidence.Digest, _ = evidence.DigestV1()
	settlement := runtimeports.OperationCheckpointRestoreSettlementRefV5{ID: "settlement-" + phaseSuffix, Revision: 1, TenantID: attempt.TenantID, EffectID: effectID, Attempt: attempt, Phase: phase, OperationDigest: operationDigest, Digest: checkpointDigestV1("settlement-" + phaseSuffix)}
	apply := runtimeports.CheckpointParticipantApplySettlementRefV2{ID: "apply-" + phaseSuffix, Revision: 1, Participant: participant, Phase: phase, SettlementID: settlement.ID, Digest: checkpointDigestV1("apply-" + phaseSuffix)}
	result := runtimeports.CheckpointParticipantPhaseClosureRefV2{ID: "phase-closure-" + phaseSuffix, Phase: phase, Reservation: reservation, PhaseFact: phaseFact, DomainResult: domain, Evidence: evidence, Settlement: settlement, ApplySettlement: apply}
	result.Digest, err = result.DigestV2()
	if err != nil || result.Validate() != nil {
		t.Fatalf("build phase closure: %v", err)
	}
	return result
}

func governedCheckpointExternalRefV1(owner runtimeports.ProviderBindingRefV2, tenant core.TenantID, scope core.Digest, run core.AgentRunID, id, kind string) appcontract.CheckpointExternalExactRefV1 {
	return appcontract.CheckpointExternalExactRefV1{ContractVersion: "1.0.0", ExactSchemaRef: "praxis.sandbox/" + kind + "/v1", FactKind: kind, Schema: runtimeports.SchemaRefV2{Namespace: "praxis.sandbox", Name: kind, Version: "1.0.0", MediaType: "application/json", ContentDigest: checkpointDigestV1("schema-" + kind)}, Owner: owner, TenantID: tenant, ScopeDigest: scope, RunID: run, ID: id, Revision: 1, Digest: checkpointDigestV1(id)}
}

func checkpointDigestV1(value string) core.Digest { return core.DigestBytes([]byte(value)) }

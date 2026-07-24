package fakes

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// BuildCommittedCheckpointParticipantClosureV2 creates a structurally complete
// reference-test closure. It is a fake-only fixture: it does not execute a
// Provider and must never be used as production Evidence or Settlement.
func BuildCommittedCheckpointParticipantClosureV2(scope core.ExecutionScope, runID core.AgentRunID, attempt ports.CheckpointAttemptRefV2, barrier ports.CheckpointBarrierLeaseRefV2, cut ports.EffectCutRefV2, participant ports.CheckpointParticipantRefV2, suffix string, now time.Time) (ports.CheckpointParticipantClosureRefV2, ports.CheckpointParticipantBranchGuardRefV2, error) {
	prepare, err := buildCheckpointParticipantPhaseFixtureV2(scope, runID, attempt, barrier, cut, participant, ports.CheckpointPhasePrepareV2, ports.CheckpointParticipantPreparedV2, suffix, now)
	if err != nil {
		return ports.CheckpointParticipantClosureRefV2{}, ports.CheckpointParticipantBranchGuardRefV2{}, err
	}
	commit, err := buildCheckpointParticipantPhaseFixtureV2(scope, runID, attempt, barrier, cut, participant, ports.CheckpointPhaseCommitV2, ports.CheckpointParticipantCommittedV2, suffix, now)
	if err != nil {
		return ports.CheckpointParticipantClosureRefV2{}, ports.CheckpointParticipantBranchGuardRefV2{}, err
	}
	closure := ports.CheckpointParticipantClosureRefV2{ID: "checkpoint-closure-" + suffix, Participant: participant, Prepare: prepare, Terminal: &commit}
	closure.Digest, err = closure.DigestV2()
	if err != nil {
		return ports.CheckpointParticipantClosureRefV2{}, ports.CheckpointParticipantBranchGuardRefV2{}, err
	}
	if err = closure.Validate(); err != nil {
		return ports.CheckpointParticipantClosureRefV2{}, ports.CheckpointParticipantBranchGuardRefV2{}, err
	}
	guard := ports.CheckpointParticipantBranchGuardRefV2{TenantID: attempt.TenantID, AttemptID: attempt.ID, ParticipantID: participant.ID, SelectedPhase: ports.CheckpointPhaseCommitV2, Revision: 1, Digest: checkpointFixtureDigestV2("branch-" + suffix)}
	return closure, guard, guard.Validate()
}

func buildCheckpointParticipantPhaseFixtureV2(scope core.ExecutionScope, runID core.AgentRunID, attempt ports.CheckpointAttemptRefV2, barrier ports.CheckpointBarrierLeaseRefV2, cut ports.EffectCutRefV2, participant ports.CheckpointParticipantRefV2, phase ports.CheckpointParticipantPhaseV2, state ports.CheckpointParticipantPhaseStateV2, suffix string, now time.Time) (ports.CheckpointParticipantPhaseClosureRefV2, error) {
	phaseSuffix := string(phase) + "-" + suffix
	scopeDigest, err := ports.ExecutionScopeDigestV2(scope)
	if err != nil {
		return ports.CheckpointParticipantPhaseClosureRefV2{}, err
	}
	operation := ports.OperationSubjectV3{Kind: ports.OperationScopeKindV3("praxis.checkpoint/participant"), ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, RunID: runID, CustomOperationID: "checkpoint-operation-" + phaseSuffix, SubjectRevision: 1, CurrentProjectionRef: "checkpoint-current-" + phaseSuffix, CurrentProjectionDigest: checkpointFixtureDigestV2("current-" + phaseSuffix), CurrentProjectionRevision: 1}
	// Custom operations use CustomOperationID and no RunID.
	operation.RunID = ""
	operationDigest, err := operation.DigestV3()
	if err != nil {
		return ports.CheckpointParticipantPhaseClosureRefV2{}, err
	}
	reservation := ports.CheckpointParticipantPhaseReservationRefV2{ID: "checkpoint-reservation-" + phaseSuffix, Revision: 1, Digest: checkpointFixtureDigestV2("reservation-" + phaseSuffix), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	phaseFact := ports.CheckpointParticipantPhaseRefV2{ID: "checkpoint-phase-" + phaseSuffix, Revision: 1, Phase: phase, State: state, Digest: checkpointFixtureDigestV2("phase-" + phaseSuffix)}
	domain := ports.CheckpointParticipantDomainResultRefV2{ID: "checkpoint-domain-" + phaseSuffix, Revision: 1, Kind: ports.NamespacedNameV2("praxis.checkpoint/domain-result"), Attempt: attempt, Participant: participant, Phase: phase, Operation: operation, OperationDigest: operationDigest, Digest: checkpointFixtureDigestV2("domain-" + phaseSuffix)}
	effectID := core.EffectIntentID("checkpoint-effect-" + phaseSuffix)
	dispatch := ports.OperationDispatchAttemptRefV3{OperationDigest: operationDigest, EffectID: effectID, IntentRevision: 1, IntentDigest: checkpointFixtureDigestV2("intent-" + phaseSuffix), PermitID: "checkpoint-permit-" + phaseSuffix, PermitRevision: 1, PermitDigest: checkpointFixtureDigestV2("permit-" + phaseSuffix), AttemptID: "checkpoint-dispatch-" + phaseSuffix}
	evidenceScopeDigest := checkpointFixtureDigestV2("evidence-scope-" + phaseSuffix)
	qualification := ports.CheckpointRestoreEvidenceQualificationRefV1{ID: "checkpoint-qualification-" + phaseSuffix, Revision: 1, Attempt: attempt, Barrier: barrier, EffectCut: cut, Reservation: reservation, Phase: phase, ScopeDigest: evidenceScopeDigest, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	qualification.Digest, err = qualification.DigestV1()
	if err != nil {
		return ports.CheckpointParticipantPhaseClosureRefV2{}, err
	}
	handoff := ports.CheckpointRestoreEvidenceProviderHandoffRefV1{ID: "checkpoint-handoff-" + phaseSuffix, Revision: 1, Qualification: qualification, Attempt: dispatch, Phase: phase, ScopeDigest: evidenceScopeDigest}
	handoff.Digest, err = handoff.DigestV1()
	if err != nil {
		return ports.CheckpointParticipantPhaseClosureRefV2{}, err
	}
	source := ports.EvidenceSourceKeyV2{RegistrationID: "checkpoint-source-" + phaseSuffix, SourceEpoch: 1, SourceSequence: 1}
	evidence := ports.CheckpointRestoreEvidenceConsumptionRefV1{ID: "checkpoint-consumption-" + phaseSuffix, Revision: 1, Qualification: qualification, Handoff: handoff, Record: ports.EvidenceRecordRefV2{LedgerScopeDigest: evidenceScopeDigest, Sequence: 1, RecordDigest: checkpointFixtureDigestV2("record-" + phaseSuffix)}, Attempt: attempt, Phase: phase, State: ports.CheckpointEvidenceConsumedCurrentV1, ScopeDigest: evidenceScopeDigest, Source: source}
	evidence.Digest, err = evidence.DigestV1()
	if err != nil {
		return ports.CheckpointParticipantPhaseClosureRefV2{}, err
	}
	settlement := ports.OperationCheckpointRestoreSettlementRefV5{ID: "checkpoint-settlement-" + phaseSuffix, Revision: 1, TenantID: attempt.TenantID, EffectID: effectID, Attempt: attempt, Phase: phase, OperationDigest: operationDigest, Digest: checkpointFixtureDigestV2("settlement-" + phaseSuffix)}
	apply := ports.CheckpointParticipantApplySettlementRefV2{ID: "checkpoint-apply-" + phaseSuffix, Revision: 1, Participant: participant, Phase: phase, SettlementID: settlement.ID, Digest: checkpointFixtureDigestV2("apply-" + phaseSuffix)}
	closure := ports.CheckpointParticipantPhaseClosureRefV2{ID: "checkpoint-phase-closure-" + phaseSuffix, Phase: phase, Reservation: reservation, PhaseFact: phaseFact, DomainResult: domain, Evidence: evidence, Settlement: settlement, ApplySettlement: apply}
	closure.Digest, err = closure.DigestV2()
	if err != nil {
		return ports.CheckpointParticipantPhaseClosureRefV2{}, err
	}
	return closure, closure.Validate()
}

func checkpointFixtureDigestV2(value string) core.Digest { return core.DigestBytes([]byte(value)) }

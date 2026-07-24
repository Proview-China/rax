package application

import (
	"context"
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationfakes "github.com/Proview-China/rax/ExecutionRuntime/application/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestRestoreProductionCompositionV1ExecutesOrderedVerticalRoute(t *testing.T) {
	execution := newRestoreExecutionFixtureV1(t)
	stageRequest := prepareRestoreStageRequestForCompositionV1(t, execution)
	authorization := newRestoreStageAuthorizationFixtureForRequestV1(t, execution.now, stageRequest)

	participant := &restoreStageParticipantFakeV1{now: execution.now}
	enforcement := &restoreStageEnforcementFakeV1{now: execution.now}
	governance := &restoreStageGovernanceFakeV1{now: execution.now, participant: participant, enforcement: enforcement}
	participant.governance = governance
	evidenceRecord := runtimeports.EvidenceRecordRefV2{
		LedgerScopeDigest: core.DigestBytes([]byte("restore-production-composition-ledger")),
		Sequence:          1,
		RecordDigest:      core.DigestBytes([]byte("restore-production-composition-evidence")),
	}
	evidence := &restoreStageEvidenceRuntimeFakeV1{record: runtimeports.EvidenceLedgerRecordV2{Ref: evidenceRecord}}
	stageResults := applicationfakes.NewRestoreStageActionResultStoreV1()
	settlements := &restoreStageSettlementFakeV1{}

	composition, err := NewRestoreProductionCompositionV1(RestoreProductionConfigV1{
		ExecutionIntents: execution.results, StageResults: stageResults, ExecutionResults: execution.results,
		Restore: execution.restore, Materialization: execution.materialization,
		AuthorizationInputs: authorization.inputs, Admission: authorization.admission,
		Reviews: authorization.reviews, Dispatch: authorization.dispatch,
		Participant: participant, Enforcement: enforcement, StageGovernance: governance,
		Evidence: evidence, StageSettlements: settlements,
		Context: execution.context, Activation: execution.activation,
		Clock: func() time.Time { return execution.now },
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := composition.Restore.ExecuteRestoreV1(context.Background(), execution.request)
	if err != nil {
		t.Fatal(err)
	}
	if err := result.ValidateFor(execution.request, execution.now); err != nil {
		t.Fatal(err)
	}
	if authorization.admission.admitCalls != 1 || authorization.reviews.createCalls != 1 || authorization.dispatch.issueCalls != 1 || authorization.dispatch.beginCalls != 1 {
		t.Fatalf("governance sequence drifted: admission=%d review=%d issue=%d begin=%d", authorization.admission.admitCalls, authorization.reviews.createCalls, authorization.dispatch.issueCalls, authorization.dispatch.beginCalls)
	}
	if participant.prepareCalls != 1 || enforcement.prepareCalls != 1 || enforcement.executeCalls != 1 || participant.executeCalls != 1 || evidence.publishCalls != 1 || settlements.calls != 1 || participant.applyCalls != 1 || execution.activation.calls != 1 {
		t.Fatalf("vertical route call counts drifted: participant=(%d,%d,%d) enforcement=(%d,%d) evidence=%d settlement=%d activation=%d", participant.prepareCalls, participant.executeCalls, participant.applyCalls, enforcement.prepareCalls, enforcement.executeCalls, evidence.publishCalls, settlements.calls, execution.activation.calls)
	}

	replayed, err := composition.Restore.ExecuteRestoreV1(context.Background(), execution.request)
	if err != nil || replayed.Digest != result.Digest || participant.executeCalls != 1 || execution.activation.calls != 1 {
		t.Fatalf("production composition replay repeated an Effect: replay=%+v err=%v execute=%d activation=%d", replayed, err, participant.executeCalls, execution.activation.calls)
	}
}

func TestRestoreProductionCompositionV1RejectsTypedNilOwnerPort(t *testing.T) {
	execution := newRestoreExecutionFixtureV1(t)
	stageRequest := prepareRestoreStageRequestForCompositionV1(t, execution)
	authorization := newRestoreStageAuthorizationFixtureForRequestV1(t, execution.now, stageRequest)
	var participant *restoreStageParticipantFakeV1
	_, err := NewRestoreProductionCompositionV1(RestoreProductionConfigV1{
		ExecutionIntents: execution.results, StageResults: applicationfakes.NewRestoreStageActionResultStoreV1(), ExecutionResults: execution.results,
		Restore: execution.restore, Materialization: execution.materialization,
		AuthorizationInputs: authorization.inputs, Admission: authorization.admission,
		Reviews: authorization.reviews, Dispatch: authorization.dispatch,
		Participant: participant, Enforcement: &restoreStageEnforcementFakeV1{now: execution.now},
		StageGovernance: &restoreStageGovernanceFakeV1{now: execution.now},
		Evidence:        &restoreStageEvidenceRuntimeFakeV1{}, StageSettlements: &restoreStageSettlementFakeV1{},
		Context: execution.context, Activation: execution.activation, Clock: func() time.Time { return execution.now },
	})
	if err == nil {
		t.Fatal("typed-nil Restore Participant was accepted by production composition")
	}
}

func TestRestoreProductionCompositionV1RecoversLostRepliesWithoutRepeatingEffect(t *testing.T) {
	execution := newRestoreExecutionFixtureV1(t)
	stageRequest := prepareRestoreStageRequestForCompositionV1(t, execution)
	authorization := newRestoreStageAuthorizationFixtureForRequestV1(t, execution.now, stageRequest)
	authorization.admission.loseReply = true
	authorization.reviews.loseReply = true
	authorization.dispatch.loseIssueReply = true
	authorization.dispatch.loseBeginReply = true

	participant := &restoreStageParticipantFakeV1{now: execution.now}
	enforcement := &restoreStageEnforcementFakeV1{now: execution.now, losePrepareReply: true, loseExecuteReply: true}
	governance := &restoreStageGovernanceFakeV1{now: execution.now, participant: participant, enforcement: enforcement}
	participant.governance = governance
	evidenceRef := runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: core.DigestBytes([]byte("restore-production-lost-ledger")), Sequence: 1, RecordDigest: core.DigestBytes([]byte("restore-production-lost-evidence"))}
	evidence := &restoreStageEvidenceRuntimeFakeV1{record: runtimeports.EvidenceLedgerRecordV2{Ref: evidenceRef}, loseReply: true}
	stageResults := applicationfakes.NewRestoreStageActionResultStoreV1()
	stageResults.LoseNextReplyForTestV1()
	execution.results.LoseNextReplyForTestV1()
	execution.activation.loseNext = true
	settlements := &restoreStageSettlementFakeV1{loseReply: true}
	composition, err := NewRestoreProductionCompositionV1(RestoreProductionConfigV1{
		ExecutionIntents: execution.results, StageResults: stageResults, ExecutionResults: execution.results,
		Restore: execution.restore, Materialization: execution.materialization,
		AuthorizationInputs: authorization.inputs, Admission: authorization.admission,
		Reviews: authorization.reviews, Dispatch: authorization.dispatch,
		Participant: participant, Enforcement: enforcement, StageGovernance: governance,
		Evidence: evidence, StageSettlements: settlements, Context: execution.context,
		Activation: execution.activation, Clock: func() time.Time { return execution.now },
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := composition.Restore.ExecuteRestoreV1(context.Background(), execution.request)
	if err != nil || result.ValidateFor(execution.request, execution.now) != nil {
		t.Fatalf("lost-reply route failed: result=%+v err=%v", result, err)
	}
	if participant.executeCalls != 1 || enforcement.prepareCalls != 1 || enforcement.executeCalls != 1 || evidence.publishCalls != 1 || evidence.inspectCalls != 1 || settlements.calls != 1 || execution.activation.calls != 1 || execution.activation.inspectCalls != 1 {
		t.Fatalf("lost-reply route repeated or skipped work: execute=%d evidence=(%d,%d) settlement=%d activation=(%d,%d)", participant.executeCalls, evidence.publishCalls, evidence.inspectCalls, settlements.calls, execution.activation.calls, execution.activation.inspectCalls)
	}
	replayed, err := composition.Restore.ExecuteRestoreV1(context.Background(), execution.request)
	if err != nil || replayed.Digest != result.Digest || participant.executeCalls != 1 || settlements.calls != 1 || execution.activation.calls != 1 {
		t.Fatalf("lost-reply replay repeated Effect: replay=%+v err=%v execute=%d settlement=%d activation=%d", replayed, err, participant.executeCalls, settlements.calls, execution.activation.calls)
	}
}

func prepareRestoreStageRequestForCompositionV1(t *testing.T, fixture *restoreExecutionFixtureV1) applicationcontract.RestoreStageActionRequestV1 {
	t.Helper()
	attempt, err := fixture.restore.CreateRestoreAttemptV2(context.Background(), runtimeports.CreateRestoreAttemptRequestV2{AttemptID: fixture.request.RestoreAttemptID, IdempotencyKey: fixture.request.IdempotencyKey, RestorePlan: fixture.request.RestorePlan, RequestedNotAfter: fixture.request.NotAfterUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := fixture.restore.IssueRestoreEligibilityV2(context.Background(), runtimeports.IssueRestoreEligibilityRequestV2{EligibilityID: fixture.request.RestoreEligibilityID, Attempt: attempt.Ref, RequestedTTL: fixture.request.EligibilityTTL})
	if err != nil {
		t.Fatal(err)
	}
	materialization, err := fixture.materialization.InspectRestoreMaterializationCurrentV1(context.Background(), runtimeports.InspectRestoreMaterializationCurrentRequestV1{Attempt: bundle.Attempt.Ref, Eligibility: bundle.Eligibility.Ref})
	if err != nil {
		t.Fatal(err)
	}
	request, err := applicationcontract.SealRestoreStageActionRequestV1(applicationcontract.RestoreStageActionRequestV1{
		ID: fixture.request.StageActionID, IdempotencyKey: fixture.request.StageIdempotencyKey,
		Attempt: bundle.Attempt.Ref, Eligibility: bundle.Eligibility.Ref, Materialization: materialization,
		NotAfterUnixNano: minimumRestoreExecutionTimeV1(fixture.request.NotAfterUnixNano, materialization.ExpiresUnixNano, bundle.Eligibility.Ref.ExpiresUnixNano),
	})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

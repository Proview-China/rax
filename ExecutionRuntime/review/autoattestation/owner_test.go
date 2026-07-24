package autoattestation_test

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/autoattestation"
	"github.com/Proview-China/rax/ExecutionRuntime/review/caseengine"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/storetestkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type fullStoreV1 interface {
	reviewport.StoreV1
	reviewport.AutoReviewerStoreV1
	reviewport.TraceEventStoreV2
	AdvanceCaseForTestV1(context.Context, reviewport.ExpectedFactV1, contract.ReviewCaseV1) (contract.ReviewCaseV1, error)
}

type fixtureV1 struct {
	ctx         context.Context
	base        time.Time
	clock       *testkit.ManualClock
	store       fullStoreV1
	caseFact    contract.ReviewCaseV1
	round       contract.ReviewRoundV1
	assignment  contract.ReviewerAssignmentV1
	rubric      contract.RubricDefinitionV1
	attempt     contract.AutoReviewerAttemptV1
	observation contract.AutoReviewerInvocationObservationV1
	result      contract.ReviewerInvocationResultFactV1
	apply       contract.DomainApplySettlementFactV1
	command     autoattestation.RecordCommandV1
}

func newFixtureV1(t testing.TB, suffix string) fixtureV1 {
	return newFixtureWithResolutionV1(t, suffix, contract.ResolutionAcceptV1)
}

func newFixtureWithResolutionV1(t testing.TB, suffix string, resolution contract.ResolutionV1) fixtureV1 {
	t.Helper()
	ctx := context.Background()
	base := time.Unix(1_905_000_000, 0)
	clock := testkit.NewClock(base)
	store := storetestkit.NewMemoryStoreV1(clock.Now)
	rubric := testkit.Rubric(base, "tenant-a")
	if _, err := store.PublishRubricV1(ctx, reviewport.PublishRubricMutationV1{Next: rubric}); err != nil {
		t.Fatal(err)
	}
	target := testkit.Target(base)
	engine, err := caseengine.New(store, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	caseID := "case-auto-attestation-" + suffix
	request := testkit.Request(base, target, caseID)
	clock.Advance(time.Second)
	caseFact, err := engine.CreateCaseV1(ctx, caseengine.CreateCaseCommandV1{CaseID: caseID, Request: &request, Target: target, ExpiresUnixNano: request.ExpiresUnixNano, Trace: testkit.TraceForTarget(clock.Now(), caseID, target, contract.TraceRequestedV1, 1, request.ID)})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []contract.CaseStateV1{contract.CaseAdmittedV1, contract.CaseRoutedV1} {
		clock.Advance(time.Second)
		caseFact, err = engine.TransitionWithTraceV2(ctx, caseengine.TransitionWithTraceCommandV2{TransitionCommandV1: caseengine.TransitionCommandV1{TenantID: caseFact.TenantID, CaseID: caseFact.ID, Expected: reviewport.ExpectedV1(caseFact.Revision, caseFact.Digest), Next: state}, Trace: testkit.TransitionTrace(clock.Now(), caseFact, state)})
		if err != nil {
			t.Fatal(err)
		}
	}
	clock.Advance(time.Second)
	round := testkit.Round(clock.Now(), caseFact, contract.RouteAutoV1)
	round.ID, round.AssignmentID, round.RubricDigest = "round-aa-"+suffix, "assignment-aa-"+suffix, rubric.Digest
	rubricRef := rubric.ExactRef()
	round.Rubric = &rubricRef
	round.Digest = ""
	round, err = contract.SealReviewRoundV1(round)
	if err != nil {
		t.Fatal(err)
	}
	assignment := testkit.Assignment(clock.Now(), caseFact, round, contract.RouteAutoV1)
	assignment.ID, assignment.RoundID, assignment.RoundDigest = round.AssignmentID, round.ID, round.Digest
	assignment.Digest = ""
	assignment, err = contract.SealReviewerAssignmentV1(assignment)
	if err != nil {
		t.Fatal(err)
	}
	caseFact, _, assignment, err = engine.StartRoundV1(ctx, reviewport.StartRoundMutationV1{Expected: reviewport.ExpectedV1(caseFact.Revision, caseFact.Digest), Round: round, Assignment: assignment, Trace: testkit.Trace(clock.Now(), caseFact, contract.TraceAssignedV1, 2, round.ID, assignment.ID)})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	caseFact, assignment, err = engine.ClaimAssignmentV1(ctx, reviewport.ClaimAssignmentMutationV1{TenantID: caseFact.TenantID, ExpectedCase: reviewport.ExpectedV1(caseFact.Revision, caseFact.Digest), ExpectedAssignment: reviewport.ExpectedV1(assignment.Revision, assignment.Digest), CaseID: caseFact.ID, AssignmentID: assignment.ID, LeaseHolder: "worker-aa-" + suffix, LeaseExpiresUnixNano: base.Add(15 * time.Minute).UnixNano(), UpdatedUnixNano: clock.Now().UnixNano(), Traces: []contract.TraceFactV1{testkit.StartedTrace(clock.Now(), caseFact, assignment.ID)}})
	if err != nil {
		t.Fatal(err)
	}

	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(target.Scope)
	if err != nil {
		t.Fatal(err)
	}
	operation := runtimeports.OperationSubjectV3{Kind: runtimeports.OperationScopeRunV3, ExecutionScope: target.Scope, ExecutionScopeDigest: scopeDigest, RunID: target.RunID, SubjectRevision: 1, CurrentProjectionRef: "aa-operation-current-" + suffix, CurrentProjectionDigest: testkit.Digest("aa-operation-current-" + suffix), CurrentProjectionRevision: 1}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	provider := runtimeports.ProviderBindingRefV2{BindingSetID: "aa-binding", BindingSetRevision: 1, ComponentID: "praxis.model/reviewer", ManifestDigest: testkit.Digest("aa-model-manifest"), ArtifactDigest: testkit.Digest("aa-model-artifact"), Capability: "praxis.model/review"}
	prepared, err := contract.SealAutoReviewerAttemptV1(contract.AutoReviewerAttemptV1{FactIdentityV1: contract.FactIdentityV1{TenantID: caseFact.TenantID, ID: "aa-attempt-" + suffix, Revision: 1, CreatedUnixNano: base.Add(6 * time.Second).UnixNano(), UpdatedUnixNano: base.Add(6 * time.Second).UnixNano()}, IdempotencyKey: "aa-attempt-idem-" + suffix, Case: resourceRefV1(caseFact.FactIdentityV1), Round: resourceRefV1(round.FactIdentityV1), Assignment: resourceRefV1(assignment.FactIdentityV1), Target: resourceRefV1(target.FactIdentityV1), Rubric: rubric.ExactRef(), ContextFrameDigest: round.ContextFrameDigest, ReviewerID: assignment.ReviewerID, ReviewerAuthority: assignment.ReviewerAuthority, ReviewerBinding: assignment.ReviewerBinding, RouteID: "praxis.model/review-route", Operation: operation, OperationDigest: operationDigest, InvocationEffect: runtimeports.ReviewInvocationEffectRefV2{EffectID: core.EffectIntentID("aa-effect-" + suffix), EffectRevision: 1, EffectKind: "praxis.review/auto-reviewer-invoke", PayloadDigest: testkit.Digest("aa-payload-" + suffix), Provider: provider}, ResultSchema: testkit.Schema("aa-result"), RoundOrdinal: 1, MaxCostMicros: 1_000, State: contract.AutoReviewerAttemptPreparedV1, ExpiresUnixNano: base.Add(10 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err = store.BeginAutoReviewerAttemptV1(ctx, reviewport.BeginAutoReviewerAttemptMutationV1{Attempt: prepared})
	if err != nil {
		t.Fatal(err)
	}
	prepared, invocationRef := persistInvocationStartClaimV1(t, store, ctx, prepared, base.Add(7*time.Second))

	evidence := []runtimeports.ReviewEvidenceRefV2{
		{Ref: "evidence://authority-" + suffix, Classification: "review.authority/current", Digest: testkit.Digest("aa-authority-" + suffix)},
		{Ref: "evidence://scope-" + suffix, Classification: "review.scope/current", Digest: testkit.Digest("aa-scope-" + suffix)},
	}
	var conditions []runtimeports.ReviewConditionV2
	if resolution == contract.ResolutionConditionalV1 {
		conditions = []runtimeports.ReviewConditionV2{{ID: "review.auto/followup", Revision: 1, Schema: testkit.Schema("condition"), ConstraintDigest: testkit.Digest("condition-constraint-" + suffix), SatisfactionOwner: assignment.ReviewerBinding, ScopeDigest: target.ActionScopeDigest, Authority: target.ActorAuthority, ExpiresUnixNano: base.Add(7 * time.Minute).UnixNano()}}
	}
	output, err := contract.SealAutoReviewerStructuredOutputV1(contract.AutoReviewerStructuredOutputV1{Resolution: resolution, ReasonCodes: []string{"review.auto/checked"}, Findings: []contract.AutoFindingDraftV1{{Category: "review.correctness", Priority: "high", Anchor: "result/root", Claim: "the result satisfies the exact rubric", Impact: "the result may be accepted", Evidence: evidence}}, Evidence: evidence, Conditions: conditions})
	if err != nil {
		t.Fatal(err)
	}
	delegation := runtimeports.ExecutionDelegationRefV2{ID: "aa-delegation-" + suffix, Revision: 1, Digest: testkit.Digest("aa-delegation-" + suffix)}
	runtimeAttempt := runtimeports.OperationDispatchAttemptRefV3{OperationDigest: prepared.OperationDigest, EffectID: prepared.InvocationEffect.EffectID, IntentRevision: prepared.InvocationEffect.EffectRevision, IntentDigest: testkit.Digest("aa-intent-" + suffix), PermitID: "aa-permit-" + suffix, PermitRevision: 1, PermitDigest: testkit.Digest("aa-permit-" + suffix), AttemptID: "aa-runtime-attempt-" + suffix, Delegation: &delegation}
	observedAt := base.Add(8 * time.Second)
	observation, err := contract.SealAutoReviewerInvocationObservationV1(contract.AutoReviewerInvocationObservationV1{FactIdentityV1: contract.FactIdentityV1{TenantID: prepared.TenantID, ID: "aa-observation-" + suffix, Revision: 1, CreatedUnixNano: observedAt.UnixNano(), UpdatedUnixNano: observedAt.UnixNano()}, AttemptID: prepared.ID, AttemptRevision: invocationRef.Revision, AttemptDigest: invocationRef.Digest, OperationDigest: prepared.OperationDigest, RuntimeAttempt: runtimeAttempt, ProviderObservation: runtimeports.ProviderAttemptObservationRefV2{Delegation: delegation, PreparedAttemptID: "aa-prepared-" + suffix, ProviderOperationRef: "aa-provider-op-" + suffix, Revision: 1, State: runtimeports.ProviderAttemptObservedV2, Digest: testkit.Digest("aa-provider-observation-" + suffix), PayloadDigest: testkit.Digest("aa-provider-payload-" + suffix), PayloadRevision: 1, SourceRegistrationID: "aa-provider-source-" + suffix, SourceEpoch: 1, SourceSequence: 1, Evidence: runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: testkit.Digest("aa-ledger-" + suffix), Sequence: 1, RecordDigest: testkit.Digest("aa-record-" + suffix)}, ObservedUnixNano: observedAt.UnixNano()}, Output: output, ResultSchema: prepared.ResultSchema, Tokens: 100, CostMicros: 50, ObservedUnixNano: observedAt.UnixNano(), ExpiresUnixNano: base.Add(8 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	result, err := contract.SealReviewerInvocationResultFactV1(contract.ReviewerInvocationResultFactV1{FactIdentityV1: contract.FactIdentityV1{TenantID: prepared.TenantID, ID: "aa-result-" + suffix, Revision: 1, CreatedUnixNano: observedAt.UnixNano(), UpdatedUnixNano: observedAt.UnixNano()}, CaseID: prepared.Case.ID, CaseRevision: prepared.Case.Revision, RoundID: prepared.Round.ID, RoundRevision: prepared.Round.Revision, RoundDigest: prepared.Round.Digest, AssignmentID: prepared.Assignment.ID, AssignmentRevision: prepared.Assignment.Revision, AssignmentDigest: prepared.Assignment.Digest, TargetID: prepared.Target.ID, TargetRevision: prepared.Target.Revision, TargetDigest: prepared.Target.Digest, AttemptID: runtimeAttempt.AttemptID, ResultSchema: prepared.ResultSchema, ResultDigest: output.Digest, ObservationRefs: []string{observation.ID}})
	if err != nil {
		t.Fatal(err)
	}
	observed := prepared
	observed.Revision++
	observed.UpdatedUnixNano = observedAt.UnixNano()
	observed.State = contract.AutoReviewerAttemptObservedV1
	observationRef, resultRef := observation.Ref(), result.ExactRef()
	observed.InvocationAttempt, observed.Observation, observed.DomainResult = &invocationRef, &observationRef, &resultRef
	observed.Digest = ""
	observed, err = contract.SealAutoReviewerAttemptV1(observed)
	if err != nil {
		t.Fatal(err)
	}
	observed, result, err = store.RecordAutoReviewerObservationV1(ctx, reviewport.RecordAutoReviewerObservationMutationV1{Expected: prepared.ExactRef(), Next: observed, Observation: observation, DomainResult: result})
	if err != nil {
		t.Fatal(err)
	}

	apply, err := contract.SealDomainApplySettlementFactV1(contract.DomainApplySettlementFactV1{FactIdentityV1: contract.FactIdentityV1{TenantID: result.TenantID, ID: "aa-apply-" + suffix, Revision: 1, CreatedUnixNano: base.Add(9 * time.Second).UnixNano(), UpdatedUnixNano: base.Add(9 * time.Second).UnixNano()}, DomainResultID: result.ID, DomainResultDigest: result.Digest, RuntimeSettlementID: "aa-runtime-settlement-" + suffix, RuntimeSettlementRevision: 1, RuntimeSettlementDigest: testkit.Digest("aa-runtime-settlement-" + suffix), RuntimeContractVersion: runtimeports.OperationSettlementContractVersionV4, RuntimeInspectionDigest: testkit.Digest("aa-runtime-inspection-" + suffix), State: contract.DomainApplyAppliedV1})
	if err != nil {
		t.Fatal(err)
	}
	apply, err = store.CreateApplySettlementV1(ctx, apply)
	if err != nil {
		t.Fatal(err)
	}
	findingIDs, err := autoattestation.DeterministicFindingIDsV1(observed.TenantID, observed, observation)
	if err != nil {
		t.Fatal(err)
	}
	attestationID := "aa-attestation-" + suffix
	refs := []string{observed.ID, observation.ID, result.ID, apply.ID, rubric.ID, attestationID}
	refs = append(refs, findingIDs...)
	command := autoattestation.RecordCommandV1{TenantID: observed.TenantID, Attempt: observed.ExactRef(), ApplySettlement: apply.Ref(), AttestationID: attestationID, IdempotencyKey: "aa-attestation-idem-" + suffix, Trace: autoAttestedTraceV2(t, base.Add(10*time.Second), caseFact, 3, attestationID, refs)}
	return fixtureV1{ctx: ctx, base: base, clock: clock, store: store, caseFact: caseFact, round: round, assignment: assignment, rubric: rubric, attempt: observed, observation: observation, result: result, apply: apply, command: command}
}

func resourceRefV1(value contract.FactIdentityV1) contract.ExactResourceRefV1 {
	return contract.ExactResourceRefV1{ID: value.ID, Revision: value.Revision, Digest: value.Digest}
}

func persistInvocationStartClaimV1(t testing.TB, store reviewport.AutoReviewerStoreV1, ctx context.Context, prepared contract.AutoReviewerAttemptV1, updated time.Time) (contract.AutoReviewerAttemptV1, contract.ExactResourceRefV1) {
	t.Helper()
	origin := prepared.ExactRef()
	waiting := prepared
	waiting.Revision++
	waiting.UpdatedUnixNano = updated.UnixNano()
	waiting.State = contract.AutoReviewerAttemptWaitingInspectV1
	waiting.InvocationAttempt = &origin
	waiting.Digest = ""
	waiting, err := contract.SealAutoReviewerAttemptV1(waiting)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := store.MarkAutoReviewerWaitingInspectV1(ctx, reviewport.MarkAutoReviewerWaitingInspectMutationV1{Expected: origin, Next: waiting})
	if err != nil || !receipt.Applied {
		t.Fatalf("persistent Auto Reviewer start claim failed: receipt=%+v err=%v", receipt, err)
	}
	return receipt.Attempt, origin
}

func advanceToSecondRoundV1(t testing.TB, first fixtureV1, suffix string) fixtureV1 {
	t.Helper()
	step := first.base.Add(20 * time.Second)
	clock := first.clock
	clock.Set(step)
	engine, err := caseengine.New(first.store, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	current := first.caseFact
	waitingRevision := testkit.CaseSuccessor(clock.Now(), current, contract.CaseWaitingRevisionV1)
	current, err = first.store.AdvanceCaseForTestV1(first.ctx, reviewport.ExpectedV1(current.Revision, current.Digest), waitingRevision)
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	current, err = engine.TransitionWithTraceV2(first.ctx, caseengine.TransitionWithTraceCommandV2{TransitionCommandV1: caseengine.TransitionCommandV1{TenantID: current.TenantID, CaseID: current.ID, Expected: reviewport.ExpectedV1(current.Revision, current.Digest), Next: contract.CaseRoutedV1}, Trace: testkit.TransitionTrace(clock.Now(), current, contract.CaseRoutedV1)})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	round := testkit.Round(clock.Now(), current, contract.RouteAutoV1)
	round.ID, round.AssignmentID, round.RubricDigest, round.Digest = "round-aa-"+suffix, "assignment-aa-"+suffix, first.rubric.Digest, ""
	rubricRef := first.rubric.ExactRef()
	round.Rubric = &rubricRef
	round, err = contract.SealReviewRoundV1(round)
	if err != nil {
		t.Fatal(err)
	}
	assignment := testkit.Assignment(clock.Now(), current, round, contract.RouteAutoV1)
	assignment.ID, assignment.RoundID, assignment.RoundDigest, assignment.Digest = round.AssignmentID, round.ID, round.Digest, ""
	assignment, err = contract.SealReviewerAssignmentV1(assignment)
	if err != nil {
		t.Fatal(err)
	}
	current, _, assignment, err = engine.StartRoundV1(first.ctx, reviewport.StartRoundMutationV1{Expected: reviewport.ExpectedV1(current.Revision, current.Digest), Round: round, Assignment: assignment, Trace: testkit.Trace(clock.Now(), current, contract.TraceAssignedV1, 3, round.ID, assignment.ID)})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	current, assignment, err = engine.ClaimAssignmentV1(first.ctx, reviewport.ClaimAssignmentMutationV1{TenantID: current.TenantID, ExpectedCase: reviewport.ExpectedV1(current.Revision, current.Digest), ExpectedAssignment: reviewport.ExpectedV1(assignment.Revision, assignment.Digest), CaseID: current.ID, AssignmentID: assignment.ID, LeaseHolder: "worker-aa-" + suffix, LeaseExpiresUnixNano: first.base.Add(15 * time.Minute).UnixNano(), UpdatedUnixNano: clock.Now().UnixNano(), Traces: []contract.TraceFactV1{testkit.StartedTrace(clock.Now(), current, assignment.ID)}})
	if err != nil {
		t.Fatal(err)
	}

	prepared := first.attempt
	prepared.FactIdentityV1.ID, prepared.FactIdentityV1.Revision = "aa-attempt-"+suffix, 1
	prepared.FactIdentityV1.CreatedUnixNano, prepared.FactIdentityV1.UpdatedUnixNano = clock.Now().Add(time.Second).UnixNano(), clock.Now().Add(time.Second).UnixNano()
	prepared.IdempotencyKey = "aa-attempt-idem-" + suffix
	prepared.Case, prepared.Round, prepared.Assignment = resourceRefV1(current.FactIdentityV1), resourceRefV1(round.FactIdentityV1), resourceRefV1(assignment.FactIdentityV1)
	prepared.InvocationEffect.EffectID = core.EffectIntentID("aa-effect-" + suffix)
	prepared.RoundOrdinal, prepared.State = 2, contract.AutoReviewerAttemptPreparedV1
	prepared.InvocationAttempt, prepared.Observation, prepared.DomainResult = nil, nil, nil
	prepared.Digest = ""
	prepared, err = contract.SealAutoReviewerAttemptV1(prepared)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err = first.store.BeginAutoReviewerAttemptV1(first.ctx, reviewport.BeginAutoReviewerAttemptMutationV1{Attempt: prepared})
	if err != nil {
		t.Fatal(err)
	}
	prepared, preparedRef := persistInvocationStartClaimV1(t, first.store, first.ctx, prepared, time.Unix(0, prepared.UpdatedUnixNano).Add(time.Second))

	observedAt := clock.Now().Add(3 * time.Second)
	delegation := runtimeports.ExecutionDelegationRefV2{ID: "aa-delegation-" + suffix, Revision: 1, Digest: testkit.Digest("aa-delegation-" + suffix)}
	runtimeAttempt := runtimeports.OperationDispatchAttemptRefV3{OperationDigest: prepared.OperationDigest, EffectID: prepared.InvocationEffect.EffectID, IntentRevision: prepared.InvocationEffect.EffectRevision, IntentDigest: testkit.Digest("aa-intent-" + suffix), PermitID: "aa-permit-" + suffix, PermitRevision: 1, PermitDigest: testkit.Digest("aa-permit-" + suffix), AttemptID: "aa-runtime-attempt-" + suffix, Delegation: &delegation}
	observation, err := contract.SealAutoReviewerInvocationObservationV1(contract.AutoReviewerInvocationObservationV1{FactIdentityV1: contract.FactIdentityV1{TenantID: prepared.TenantID, ID: "aa-observation-" + suffix, Revision: 1, CreatedUnixNano: observedAt.UnixNano(), UpdatedUnixNano: observedAt.UnixNano()}, AttemptID: prepared.ID, AttemptRevision: preparedRef.Revision, AttemptDigest: preparedRef.Digest, OperationDigest: prepared.OperationDigest, RuntimeAttempt: runtimeAttempt, ProviderObservation: runtimeports.ProviderAttemptObservationRefV2{Delegation: delegation, PreparedAttemptID: "aa-prepared-" + suffix, ProviderOperationRef: "aa-provider-op-" + suffix, Revision: 1, State: runtimeports.ProviderAttemptObservedV2, Digest: testkit.Digest("aa-provider-observation-" + suffix), PayloadDigest: testkit.Digest("aa-provider-payload-" + suffix), PayloadRevision: 1, SourceRegistrationID: "aa-provider-source-" + suffix, SourceEpoch: 1, SourceSequence: 1, Evidence: runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: testkit.Digest("aa-ledger-" + suffix), Sequence: 1, RecordDigest: testkit.Digest("aa-record-" + suffix)}, ObservedUnixNano: observedAt.UnixNano()}, Output: first.observation.Output, ResultSchema: prepared.ResultSchema, Tokens: 100, CostMicros: 50, ObservedUnixNano: observedAt.UnixNano(), ExpiresUnixNano: first.base.Add(8 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	result, err := contract.SealReviewerInvocationResultFactV1(contract.ReviewerInvocationResultFactV1{FactIdentityV1: contract.FactIdentityV1{TenantID: prepared.TenantID, ID: "aa-result-" + suffix, Revision: 1, CreatedUnixNano: observedAt.UnixNano(), UpdatedUnixNano: observedAt.UnixNano()}, CaseID: prepared.Case.ID, CaseRevision: prepared.Case.Revision, RoundID: prepared.Round.ID, RoundRevision: prepared.Round.Revision, RoundDigest: prepared.Round.Digest, AssignmentID: prepared.Assignment.ID, AssignmentRevision: prepared.Assignment.Revision, AssignmentDigest: prepared.Assignment.Digest, TargetID: prepared.Target.ID, TargetRevision: prepared.Target.Revision, TargetDigest: prepared.Target.Digest, AttemptID: runtimeAttempt.AttemptID, ResultSchema: prepared.ResultSchema, ResultDigest: observation.Output.Digest, ObservationRefs: []string{observation.ID}})
	if err != nil {
		t.Fatal(err)
	}
	observed := prepared
	observed.Revision++
	observed.UpdatedUnixNano = observedAt.UnixNano()
	observed.State = contract.AutoReviewerAttemptObservedV1
	observationRef, resultRef := observation.Ref(), result.ExactRef()
	observed.InvocationAttempt, observed.Observation, observed.DomainResult = &preparedRef, &observationRef, &resultRef
	observed.Digest = ""
	observed, err = contract.SealAutoReviewerAttemptV1(observed)
	if err != nil {
		t.Fatal(err)
	}
	observed, result, err = first.store.RecordAutoReviewerObservationV1(first.ctx, reviewport.RecordAutoReviewerObservationMutationV1{Expected: prepared.ExactRef(), Next: observed, Observation: observation, DomainResult: result})
	if err != nil {
		t.Fatal(err)
	}
	apply, err := contract.SealDomainApplySettlementFactV1(contract.DomainApplySettlementFactV1{FactIdentityV1: contract.FactIdentityV1{TenantID: result.TenantID, ID: "aa-apply-" + suffix, Revision: 1, CreatedUnixNano: observedAt.Add(time.Second).UnixNano(), UpdatedUnixNano: observedAt.Add(time.Second).UnixNano()}, DomainResultID: result.ID, DomainResultDigest: result.Digest, RuntimeSettlementID: "aa-runtime-settlement-" + suffix, RuntimeSettlementRevision: 1, RuntimeSettlementDigest: testkit.Digest("aa-runtime-settlement-" + suffix), RuntimeContractVersion: runtimeports.OperationSettlementContractVersionV4, RuntimeInspectionDigest: testkit.Digest("aa-runtime-inspection-" + suffix), State: contract.DomainApplyAppliedV1})
	if err != nil {
		t.Fatal(err)
	}
	apply, err = first.store.CreateApplySettlementV1(first.ctx, apply)
	if err != nil {
		t.Fatal(err)
	}
	findingIDs, err := autoattestation.DeterministicFindingIDsV1(observed.TenantID, observed, observation)
	if err != nil {
		t.Fatal(err)
	}
	recordAt := observedAt.Add(2 * time.Second)
	attestationID := "aa-attestation-" + suffix
	refs := []string{observed.ID, observation.ID, result.ID, apply.ID, first.rubric.ID, attestationID}
	refs = append(refs, findingIDs...)
	command := autoattestation.RecordCommandV1{TenantID: observed.TenantID, Attempt: observed.ExactRef(), ApplySettlement: apply.Ref(), AttestationID: attestationID, IdempotencyKey: "aa-attestation-idem-" + suffix, Trace: autoAttestedTraceV2(t, recordAt, current, 4, attestationID, refs)}
	return fixtureV1{ctx: first.ctx, base: first.base, clock: clock, store: first.store, caseFact: current, round: round, assignment: assignment, rubric: first.rubric, attempt: observed, observation: observation, result: result, apply: apply, command: command}
}

func autoAttestedTraceV2(t testing.TB, at time.Time, c contract.ReviewCaseV1, sequence uint64, attestationID string, refs []string) contract.TraceFactV1 {
	t.Helper()
	refs = append([]string(nil), refs...)
	sort.Strings(refs)
	value, err := contract.SealTraceFactV1(contract.TraceFactV1{FactIdentityV1: contract.FactIdentityV1{TenantID: c.TenantID, ID: fmt.Sprintf("trace-auto-attested-%d", sequence), Revision: 1, CreatedUnixNano: at.UnixNano(), UpdatedUnixNano: at.UnixNano()}, CaseID: c.ID, CaseRevision: c.Revision, TargetID: c.TargetID, TargetRevision: c.TargetRevision, TargetDigest: c.TargetDigest, Event: contract.TraceAttestedV1, SourceID: "review.auto/attestation", SourceEpoch: 1, SourceSequence: sequence, CausationID: attestationID, CorrelationID: c.ID, FactRefs: refs})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestAutoAttestationOwnerAppliedResultFindingAndAttestationV1(t *testing.T) {
	fixture := newFixtureV1(t, "happy")
	now := fixture.base.Add(10 * time.Second)
	owner, err := autoattestation.NewV1(fixture.store, fixture.store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	result, err := owner.RecordV1(fixture.ctx, fixture.command)
	if err != nil {
		t.Fatal(err)
	}
	if result.Case.State != contract.CaseAttestedV1 || result.Attestation.Resolution != fixture.observation.Output.Resolution || result.Attestation.ReviewerResultDigest != fixture.observation.Output.Digest || len(result.Findings) != 1 || result.Findings[0].Claim != fixture.observation.Output.Findings[0].Claim {
		t.Fatalf("applied output was not preserved exactly: %+v", result)
	}
	for _, finding := range result.Findings {
		eventID := "trace-" + finding.ID
		events, inspectErr := fixture.store.ListTracePageV2(fixture.ctx, reviewport.ListTracePageRequestV2{TenantID: finding.TenantID, CaseID: finding.CaseID, Limit: reviewport.MaxTracePageV2})
		if inspectErr != nil {
			t.Fatal(inspectErr)
		}
		count := 0
		for _, event := range events.Events {
			if event.ID == eventID {
				count++
				if event.Event != contract.TraceFindingV1 || event.CausationID != fixture.observation.ID || len(event.FactRefs) != 3 {
					t.Fatalf("auto FindingObserved event drifted: %+v", event)
				}
			}
		}
		if count != 1 {
			t.Fatalf("auto FindingObserved count=%d, want 1", count)
		}
	}
	if replay, err := owner.RecordV1(fixture.ctx, fixture.command); err != nil || replay.Attestation.Digest != result.Attestation.Digest {
		t.Fatalf("canonical replay failed: %+v %v", replay, err)
	}
}

func TestConditionV2AutoAttestationPreservesExactSet(t *testing.T) {
	fixture := newFixtureWithResolutionV1(t, "condition-v2", contract.ResolutionConditionalV1)
	now := fixture.base.Add(10 * time.Second)
	owner, err := autoattestation.NewV1(fixture.store, fixture.store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	result, err := owner.RecordV1(fixture.ctx, fixture.command)
	if err != nil {
		t.Fatal(err)
	}
	if result.Attestation.Resolution != contract.ResolutionConditionalV1 || len(result.Attestation.Conditions) != 1 || result.Attestation.Conditions[0] != fixture.observation.Output.Conditions[0] || result.Attestation.ConditionsDigest != fixture.observation.Output.ConditionsDigest || result.Attestation.ExpiresUnixNano > fixture.observation.Output.Conditions[0].ExpiresUnixNano {
		t.Fatalf("Auto output condition set was not preserved exactly: %+v", result.Attestation)
	}
	mutated := fixture.observation.Output.Clone()
	mutated.Conditions[0].ConstraintDigest = testkit.Digest("condition-drift")
	if mutated.Conditions[0] == result.Attestation.Conditions[0] {
		t.Fatal("Attestation retained a mutable Auto output condition alias")
	}
}

type faultStoreV1 struct {
	reviewport.StoreV1
	reviewport.AutoReviewerStoreV1
	trace             reviewport.TraceEventStoreV2
	loseFinding       atomic.Bool
	loseAttestation   atomic.Bool
	blockFinding      atomic.Bool
	blockAttestation  atomic.Bool
	driftCurrent      atomic.Bool
	driftOriginal     atomic.Bool
	driftObservation  atomic.Bool
	driftResult       atomic.Bool
	driftApply        atomic.Bool
	findingCalls      atomic.Int32
	recordCalls       atomic.Int32
	mutateTermination func(*reviewport.AutoReviewTerminationCurrentProjectionV1)
}

func (s *faultStoreV1) InspectFindingExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (contract.FindingV1, error) {
	if s.blockFinding.Load() {
		<-ctx.Done()
		return contract.FindingV1{}, ctx.Err()
	}
	return s.StoreV1.InspectFindingExactV1(ctx, tenant, ref)
}

func (s *faultStoreV1) InspectAttestationExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (contract.AttestationV1, error) {
	if s.blockAttestation.Load() {
		<-ctx.Done()
		return contract.AttestationV1{}, ctx.Err()
	}
	return s.StoreV1.InspectAttestationExactV1(ctx, tenant, ref)
}

func (s *faultStoreV1) CreateFindingWithTraceV2(ctx context.Context, value reviewport.CreateFindingWithTraceMutationV2) (contract.FindingV1, error) {
	s.findingCalls.Add(1)
	created, err := s.trace.CreateFindingWithTraceV2(ctx, value)
	if err == nil && s.loseFinding.CompareAndSwap(true, false) {
		return contract.FindingV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost Finding reply")
	}
	return created, err
}
func (s *faultStoreV1) ListTracePageV2(ctx context.Context, request reviewport.ListTracePageRequestV2) (reviewport.ListTracePageResultV2, error) {
	return s.trace.ListTracePageV2(ctx, request)
}
func (s *faultStoreV1) RecordAttestationV1(ctx context.Context, value reviewport.RecordAttestationMutationV1) (contract.ReviewCaseV1, contract.AttestationV1, error) {
	s.recordCalls.Add(1)
	c, a, err := s.StoreV1.RecordAttestationV1(ctx, value)
	if err == nil && s.loseAttestation.CompareAndSwap(true, false) {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost Attestation reply")
	}
	return c, a, err
}
func (s *faultStoreV1) InspectAutoReviewTerminationCurrentV1(ctx context.Context, request reviewport.AutoReviewTerminationCurrentRequestV1) (reviewport.AutoReviewTerminationCurrentProjectionV1, error) {
	value, err := s.AutoReviewerStoreV1.InspectAutoReviewTerminationCurrentV1(ctx, request)
	if err != nil || s.mutateTermination == nil {
		return value, err
	}
	s.mutateTermination(&value)
	if value.ClosureDigest == "" && value.ProjectionDigest == "" {
		return reviewport.SealAutoReviewTerminationCurrentProjectionV1(value)
	}
	return value, nil
}
func (s *faultStoreV1) InspectAutoReviewerAttemptCurrentV1(ctx context.Context, tenant core.TenantID, id string) (contract.AutoReviewerAttemptV1, error) {
	value, err := s.AutoReviewerStoreV1.InspectAutoReviewerAttemptCurrentV1(ctx, tenant, id)
	if err == nil && s.driftCurrent.Load() {
		value.Digest = testkit.Digest("drift")
	}
	return value, err
}
func (s *faultStoreV1) InspectAutoReviewerAttemptExactV1(ctx context.Context, tenant core.TenantID, ref contract.ExactResourceRefV1) (contract.AutoReviewerAttemptV1, error) {
	value, err := s.AutoReviewerStoreV1.InspectAutoReviewerAttemptExactV1(ctx, tenant, ref)
	if err == nil && s.driftOriginal.Load() && value.State == contract.AutoReviewerAttemptPreparedV1 {
		value.ReviewerID = "drifted-reviewer"
	}
	return value, err
}
func (s *faultStoreV1) InspectAutoReviewerObservationExactV1(ctx context.Context, tenant core.TenantID, ref contract.AutoReviewerInvocationObservationRefV1) (contract.AutoReviewerInvocationObservationV1, error) {
	value, err := s.AutoReviewerStoreV1.InspectAutoReviewerObservationExactV1(ctx, tenant, ref)
	if err == nil && s.driftObservation.Load() {
		value.RuntimeAttempt.EffectID = "drifted-effect"
	}
	return value, err
}
func (s *faultStoreV1) InspectDomainResultExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (contract.ReviewerInvocationResultFactV1, error) {
	value, err := s.StoreV1.InspectDomainResultExactV1(ctx, tenant, ref)
	if err == nil && s.driftResult.Load() {
		value.AssignmentID = "drifted-assignment"
	}
	return value, err
}
func (s *faultStoreV1) InspectApplySettlementExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (contract.DomainApplySettlementFactV1, error) {
	value, err := s.StoreV1.InspectApplySettlementExactV1(ctx, tenant, ref)
	if err == nil && s.driftApply.Load() {
		value.State = contract.DomainApplyNotAppliedV1
	}
	return value, err
}

func TestAutoAttestationOwnerLostReplyIsExactInspectOnlyV1(t *testing.T) {
	fixture := newFixtureV1(t, "lost")
	faults := &faultStoreV1{StoreV1: fixture.store, AutoReviewerStoreV1: fixture.store, trace: fixture.store}
	faults.loseFinding.Store(true)
	faults.loseAttestation.Store(true)
	owner, err := autoattestation.NewV1(faults, faults, func() time.Time { return fixture.base.Add(10 * time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	result, err := owner.RecordV1(fixture.ctx, fixture.command)
	if err != nil {
		t.Fatal(err)
	}
	if result.Case.State != contract.CaseAttestedV1 || result.Attestation.ID != fixture.command.AttestationID {
		t.Fatalf("lost reply did not recover exact facts: %+v", result)
	}
	if faults.findingCalls.Load() != 1 || faults.recordCalls.Load() != 1 {
		t.Fatalf("lost replies retried a mutation: finding=%d record=%d", faults.findingCalls.Load(), faults.recordCalls.Load())
	}
	for _, finding := range result.Findings {
		events, inspectErr := fixture.store.ListTracePageV2(context.Background(), reviewport.ListTracePageRequestV2{TenantID: finding.TenantID, CaseID: finding.CaseID, Limit: reviewport.MaxTracePageV2})
		if inspectErr != nil {
			t.Fatal(inspectErr)
		}
		count := 0
		for _, event := range events.Events {
			if event.ID == "trace-"+finding.ID {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("lost reply recovered Finding without exactly one FindingObserved event: %d", count)
		}
	}
}

func TestAutoAttestationOwnerLostReplyRecoveryIsBoundedWhenExactInspectNeverReturnsV1(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*faultStoreV1)
	}{
		{"finding", func(s *faultStoreV1) { s.loseFinding.Store(true); s.blockFinding.Store(true) }},
		{"attestation", func(s *faultStoreV1) { s.loseAttestation.Store(true); s.blockAttestation.Store(true) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newFixtureV1(t, "bounded-"+tc.name)
			faults := &faultStoreV1{StoreV1: fixture.store, AutoReviewerStoreV1: fixture.store, trace: fixture.store}
			tc.setup(faults)
			owner, err := autoattestation.NewV1(faults, faults, func() time.Time { return fixture.base.Add(10 * time.Second) })
			if err != nil {
				t.Fatal(err)
			}
			autoattestation.SetRecoveryTimeoutForTestV1(owner, 10*time.Millisecond)
			started := time.Now()
			_, err = owner.RecordV1(fixture.ctx, fixture.command)
			if !core.HasCategory(err, core.ErrorIndeterminate) {
				t.Fatalf("lost reply did not preserve original Unknown: %v", err)
			}
			if elapsed := time.Since(started); elapsed >= 500*time.Millisecond {
				t.Fatalf("blocking exact Inspect exceeded bounded recovery: %v", elapsed)
			}
			if faults.findingCalls.Load() != 1 || faults.recordCalls.Load() > 1 {
				t.Fatalf("recovery repeated a mutation: finding=%d record=%d", faults.findingCalls.Load(), faults.recordCalls.Load())
			}
		})
	}
}

func TestAutoAttestationOwnerCurrentDriftTTLAndClockRollbackFailClosedV1(t *testing.T) {
	t.Run("current_drift", func(t *testing.T) {
		fixture := newFixtureV1(t, "drift")
		faults := &faultStoreV1{StoreV1: fixture.store, AutoReviewerStoreV1: fixture.store, trace: fixture.store}
		faults.driftCurrent.Store(true)
		owner, _ := autoattestation.NewV1(faults, faults, func() time.Time { return fixture.base.Add(10 * time.Second) })
		if _, err := owner.RecordV1(fixture.ctx, fixture.command); err == nil {
			t.Fatal("current drift created an Attestation")
		}
		if _, err := fixture.store.InspectAttestationV1(fixture.ctx, fixture.command.TenantID, fixture.command.AttestationID); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("drift leaked Attestation: %v", err)
		}
	})
	t.Run("ttl", func(t *testing.T) {
		fixture := newFixtureV1(t, "ttl")
		owner, _ := autoattestation.NewV1(fixture.store, fixture.store, func() time.Time { return fixture.base.Add(20 * time.Minute) })
		if _, err := owner.RecordV1(fixture.ctx, fixture.command); err == nil {
			t.Fatal("expired current cut created an Attestation")
		}
	})
	t.Run("rollback", func(t *testing.T) {
		fixture := newFixtureV1(t, "rollback")
		var calls atomic.Int32
		clock := func() time.Time {
			switch calls.Add(1) {
			case 1:
				return fixture.base.Add(12 * time.Second)
			default:
				return fixture.base.Add(11 * time.Second)
			}
		}
		owner, _ := autoattestation.NewV1(fixture.store, fixture.store, clock)
		if _, err := owner.RecordV1(fixture.ctx, fixture.command); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("rollback was not rejected: %v", err)
		}
	})
}

func TestAutoAttestationOwnerExactProvenanceDriftMatrixV1(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*faultStoreV1)
	}{
		{name: "original_invocation", mutate: func(value *faultStoreV1) { value.driftOriginal.Store(true) }},
		{name: "observation_runtime_attempt", mutate: func(value *faultStoreV1) { value.driftObservation.Store(true) }},
		{name: "domain_result_assignment", mutate: func(value *faultStoreV1) { value.driftResult.Store(true) }},
		{name: "apply_not_applied", mutate: func(value *faultStoreV1) { value.driftApply.Store(true) }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFixtureV1(t, "matrix-"+test.name)
			faults := &faultStoreV1{StoreV1: fixture.store, AutoReviewerStoreV1: fixture.store, trace: fixture.store}
			test.mutate(faults)
			owner, _ := autoattestation.NewV1(faults, faults, func() time.Time { return fixture.base.Add(10 * time.Second) })
			if _, err := owner.RecordV1(fixture.ctx, fixture.command); err == nil {
				t.Fatal("drifted provenance created an Attestation")
			}
			if _, err := fixture.store.InspectAttestationV1(fixture.ctx, fixture.command.TenantID, fixture.command.AttestationID); !core.HasCategory(err, core.ErrorNotFound) {
				t.Fatalf("drift leaked Attestation: %v", err)
			}
			findingIDs, err := autoattestation.DeterministicFindingIDsV1(fixture.attempt.TenantID, fixture.attempt, fixture.observation)
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range findingIDs {
				if _, err := fixture.store.InspectFindingV1(fixture.ctx, fixture.command.TenantID, id); !core.HasCategory(err, core.ErrorNotFound) {
					t.Fatalf("drift leaked Finding %s: %v", id, err)
				}
			}
		})
	}
}

func TestAutoAttestationOwnerTerminationS1ThresholdDriftAndTTLV1(t *testing.T) {
	t.Run("threshold_forces_human", func(t *testing.T) {
		first := newFixtureV1(t, "termination-1")
		fixture := advanceToSecondRoundV1(t, first, "termination-2")
		recordAt := time.Unix(0, fixture.command.Trace.CreatedUnixNano)
		owner, _ := autoattestation.NewV1(fixture.store, fixture.store, func() time.Time { return recordAt })
		result, err := owner.RecordV1(fixture.ctx, fixture.command)
		if err != nil {
			t.Fatal(err)
		}
		if result.Case.State != contract.CaseWaitingHumanV1 || result.Attestation.Resolution != contract.ResolutionEscalateHumanV1 || len(result.Attestation.ReasonCodes) != 1 || result.Attestation.ReasonCodes[0] != contract.AutoReviewTerminationCeilingReasonV1 {
			t.Fatalf("termination threshold did not force the exact human escalation: %+v", result)
		}
		if len(result.Attestation.Evidence) != len(fixture.observation.Output.Evidence) || result.Attestation.Evidence[0] != fixture.observation.Output.Evidence[0] {
			t.Fatal("termination escalation discarded the machine Observation evidence")
		}
	})
	t.Run("bad_closure_fails_closed", func(t *testing.T) {
		fixture := newFixtureV1(t, "closure-drift")
		faults := &faultStoreV1{StoreV1: fixture.store, AutoReviewerStoreV1: fixture.store, trace: fixture.store}
		faults.mutateTermination = func(value *reviewport.AutoReviewTerminationCurrentProjectionV1) {
			value.ClosureDigest = testkit.Digest("bad-closure")
		}
		owner, _ := autoattestation.NewV1(faults, faults, func() time.Time { return fixture.base.Add(10 * time.Second) })
		if _, err := owner.RecordV1(fixture.ctx, fixture.command); err == nil {
			t.Fatal("bad termination closure created an Attestation")
		}
	})
	t.Run("actual_point_ttl_crossing", func(t *testing.T) {
		fixture := newFixtureV1(t, "termination-ttl")
		faults := &faultStoreV1{StoreV1: fixture.store, AutoReviewerStoreV1: fixture.store, trace: fixture.store}
		faults.mutateTermination = func(value *reviewport.AutoReviewTerminationCurrentProjectionV1) {
			value.ExpiresUnixNano = fixture.base.Add(10*time.Second + 500*time.Millisecond).UnixNano()
			value.ClosureDigest, value.ProjectionDigest = "", ""
		}
		var calls atomic.Int32
		owner, _ := autoattestation.NewV1(faults, faults, func() time.Time {
			if calls.Add(1) < 4 {
				return fixture.base.Add(10 * time.Second)
			}
			return fixture.base.Add(11 * time.Second)
		})
		if _, err := owner.RecordV1(fixture.ctx, fixture.command); err == nil {
			t.Fatal("termination TTL crossing created an Attestation")
		}
		if faults.recordCalls.Load() != 0 {
			t.Fatal("termination TTL crossing reached the Attestation CAS")
		}
	})
}

func TestAutoAttestationOwnerConcurrentCanonicalRecordV1(t *testing.T) {
	fixture := newFixtureV1(t, "race")
	owner, _ := autoattestation.NewV1(fixture.store, fixture.store, func() time.Time { return fixture.base.Add(10 * time.Second) })
	const workers = 64
	results := make(chan autoattestation.RecordResultV1, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for index := 0; index < workers; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, err := owner.RecordV1(fixture.ctx, fixture.command)
			results <- value
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent canonical record failed: %v", err)
		}
	}
	var digest core.Digest
	for value := range results {
		if digest == "" {
			digest = value.Attestation.Digest
		}
		if value.Attestation.Digest != digest {
			t.Fatal("concurrent callers observed different Attestations")
		}
	}
}

func TestAutoAttestationOwnerRejectsTypedNilDependenciesV1(t *testing.T) {
	type nilStoreV1 struct {
		fullStoreV1
	}
	var typed *nilStoreV1
	if _, err := autoattestation.NewV1(typed, &faultStoreV1{}, time.Now); err == nil {
		t.Fatal("typed-nil StoreV1 was accepted")
	}
	if _, err := autoattestation.NewV1(&faultStoreV1{}, typed, time.Now); err == nil {
		t.Fatal("typed-nil AutoReviewerStoreV1 was accepted")
	}
	if _, err := autoattestation.NewV1(&faultStoreV1{}, &faultStoreV1{}, nil); err == nil {
		t.Fatal("nil clock was accepted")
	}
}

func BenchmarkAutoAttestationDeterministicFindingIDsV1(b *testing.B) {
	fixture := newFixtureV1(b, "bench")
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := autoattestation.DeterministicFindingIDsV1(fixture.attempt.TenantID, fixture.attempt, fixture.observation); err != nil {
			b.Fatal(fmt.Errorf("derive IDs: %w", err))
		}
	}
}

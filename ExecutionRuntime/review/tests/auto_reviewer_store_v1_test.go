package review_test

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/caseengine"
	"github.com/Proview-China/rax/ExecutionRuntime/review/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/storetestkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	reviewsqlite "github.com/Proview-China/rax/ExecutionRuntime/review/storage/sqlite"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type autoReviewerTestStoreV1 interface {
	reviewport.StoreV1
	reviewport.AutoReviewerStoreV1
}

type autoReviewerFixtureV1 struct {
	ctx        context.Context
	now        time.Time
	store      autoReviewerTestStoreV1
	target     contract.TargetSnapshotV1
	caseValue  contract.ReviewCaseV1
	round      contract.ReviewRoundV1
	assignment contract.ReviewerAssignmentV1
	rubric     contract.RubricDefinitionV1
	attempt    contract.AutoReviewerAttemptV1
}

func newAutoReviewerFixtureV1(t testing.TB, store autoReviewerTestStoreV1, now time.Time, suffix string) autoReviewerFixtureV1 {
	t.Helper()
	ctx := context.Background()
	clock := testkit.NewClock(now)
	rubric := testkit.Rubric(now, "tenant-a")
	if _, err := store.PublishRubricV1(ctx, reviewport.PublishRubricMutationV1{Next: rubric}); err != nil {
		t.Fatal(err)
	}
	target := testkit.Target(now)
	engine, err := caseengine.New(store, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	caseID := "case-auto-" + suffix
	request := testkit.Request(now, target, caseID)
	clock.Advance(time.Second)
	caseValue, err := engine.CreateCaseV1(ctx, caseengine.CreateCaseCommandV1{CaseID: caseID, Request: &request, Target: target, ExpiresUnixNano: request.ExpiresUnixNano, Trace: testkit.TraceForTarget(clock.Now(), caseID, target, contract.TraceRequestedV1, 1, request.ID)})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []contract.CaseStateV1{contract.CaseAdmittedV1, contract.CaseRoutedV1} {
		clock.Advance(time.Second)
		caseValue, err = engine.TransitionWithTraceV2(ctx, caseengine.TransitionWithTraceCommandV2{TransitionCommandV1: caseengine.TransitionCommandV1{TenantID: caseValue.TenantID, CaseID: caseValue.ID, Expected: reviewport.ExpectedV1(caseValue.Revision, caseValue.Digest), Next: state}, Trace: testkit.TransitionTrace(clock.Now(), caseValue, state)})
		if err != nil {
			t.Fatal(err)
		}
	}
	clock.Advance(time.Second)
	round := testkit.Round(clock.Now(), caseValue, contract.RouteAutoV1)
	round.ID = "round-auto-" + suffix
	round.AssignmentID = "assignment-auto-" + suffix
	round.RubricDigest = rubric.Digest
	rubricRef := rubric.ExactRef()
	round.Rubric = &rubricRef
	round.Digest = ""
	round, err = contract.SealReviewRoundV1(round)
	if err != nil {
		t.Fatal(err)
	}
	assignment := testkit.Assignment(clock.Now(), caseValue, round, contract.RouteAutoV1)
	assignment.ID = round.AssignmentID
	assignment.RoundID = round.ID
	assignment.RoundDigest = round.Digest
	assignment.Digest = ""
	assignment, err = contract.SealReviewerAssignmentV1(assignment)
	if err != nil {
		t.Fatal(err)
	}
	caseValue, _, assignment, err = engine.StartRoundV1(ctx, reviewport.StartRoundMutationV1{Expected: reviewport.ExpectedV1(caseValue.Revision, caseValue.Digest), Round: round, Assignment: assignment, Trace: testkit.Trace(clock.Now(), caseValue, contract.TraceAssignedV1, 2, round.ID, assignment.ID)})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	caseValue, assignment, err = engine.ClaimAssignmentV1(ctx, reviewport.ClaimAssignmentMutationV1{TenantID: caseValue.TenantID, ExpectedCase: reviewport.ExpectedV1(caseValue.Revision, caseValue.Digest), ExpectedAssignment: reviewport.ExpectedV1(assignment.Revision, assignment.Digest), CaseID: caseValue.ID, AssignmentID: assignment.ID, LeaseHolder: "auto-worker-" + suffix, LeaseExpiresUnixNano: now.Add(15 * time.Minute).UnixNano(), UpdatedUnixNano: clock.Now().UnixNano(), Traces: []contract.TraceFactV1{testkit.StartedTrace(clock.Now(), caseValue, assignment.ID)}})
	if err != nil {
		t.Fatal(err)
	}
	currentNow := clock.Now()
	provider := runtimeports.ProviderBindingRefV2{BindingSetID: "binding-auto", BindingSetRevision: 1, ComponentID: "praxis.model/reviewer", ManifestDigest: testkit.Digest("model-manifest"), ArtifactDigest: testkit.Digest("model-artifact"), Capability: "praxis.model/review"}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(target.Scope)
	if err != nil {
		t.Fatal(err)
	}
	operation := runtimeports.OperationSubjectV3{Kind: runtimeports.OperationScopeRunV3, ExecutionScope: target.Scope, ExecutionScopeDigest: scopeDigest, RunID: target.RunID, SubjectRevision: 1, CurrentProjectionRef: "auto-operation-current-" + suffix, CurrentProjectionDigest: testkit.Digest("auto-operation-current-" + suffix), CurrentProjectionRevision: 1}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := contract.SealAutoReviewerAttemptV1(contract.AutoReviewerAttemptV1{
		FactIdentityV1: contract.FactIdentityV1{TenantID: caseValue.TenantID, ID: "auto-attempt-" + suffix, Revision: 1, CreatedUnixNano: currentNow.Add(time.Second).UnixNano(), UpdatedUnixNano: currentNow.Add(time.Second).UnixNano()},
		IdempotencyKey: "auto-idem-" + suffix, Case: exactResourceV1(caseValue.FactIdentityV1), Round: exactResourceV1(round.FactIdentityV1), Assignment: exactResourceV1(assignment.FactIdentityV1), Target: exactResourceV1(target.FactIdentityV1), Rubric: rubric.ExactRef(), ContextFrameDigest: round.ContextFrameDigest,
		ReviewerID: assignment.ReviewerID, ReviewerAuthority: assignment.ReviewerAuthority, ReviewerBinding: assignment.ReviewerBinding, RouteID: "praxis.model/review-route", Operation: operation, OperationDigest: operationDigest,
		InvocationEffect: runtimeports.ReviewInvocationEffectRefV2{EffectID: core.EffectIntentID("auto-effect-" + suffix), EffectRevision: 1, EffectKind: "praxis.review/auto-reviewer-invoke", PayloadDigest: testkit.Digest("auto-payload-" + suffix), Provider: provider},
		ResultSchema:     testkit.Schema("auto-reviewer-result"), RoundOrdinal: 1, MaxCostMicros: 1_000,
		State: contract.AutoReviewerAttemptPreparedV1, ExpiresUnixNano: currentNow.Add(10 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return autoReviewerFixtureV1{ctx: ctx, now: currentNow, store: store, target: target, caseValue: caseValue, round: round, assignment: assignment, rubric: rubric, attempt: attempt}
}

func exactResourceV1(value contract.FactIdentityV1) contract.ExactResourceRefV1 {
	return contract.ExactResourceRefV1{ID: value.ID, Revision: value.Revision, Digest: value.Digest}
}

func autoReviewerOutputV1(t testing.TB) contract.AutoReviewerStructuredOutputV1 {
	t.Helper()
	value, err := contract.SealAutoReviewerStructuredOutputV1(contract.AutoReviewerStructuredOutputV1{Resolution: contract.ResolutionAcceptV1, ReasonCodes: []string{"review.auto/checked"}, Evidence: []runtimeports.ReviewEvidenceRefV2{{Ref: "evidence://authority", Classification: "review.authority/current", Digest: testkit.Digest("authority-current")}, {Ref: "evidence://scope", Classification: "review.scope/current", Digest: testkit.Digest("scope-current")}}})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func autoReviewerObservedMutationV1(t testing.TB, fixture autoReviewerFixtureV1, current contract.AutoReviewerAttemptV1, suffix string) reviewport.RecordAutoReviewerObservationMutationV1 {
	t.Helper()
	invocationAttempt := current.ExactRef()
	if current.InvocationAttempt != nil {
		invocationAttempt = *current.InvocationAttempt
	}
	delegation := runtimeports.ExecutionDelegationRefV2{ID: "delegation-" + suffix, Revision: 1, Digest: testkit.Digest("delegation-" + suffix)}
	runtimeAttempt := runtimeports.OperationDispatchAttemptRefV3{OperationDigest: current.OperationDigest, EffectID: current.InvocationEffect.EffectID, IntentRevision: current.InvocationEffect.EffectRevision, IntentDigest: testkit.Digest("intent-" + suffix), PermitID: "permit-" + suffix, PermitRevision: 1, PermitDigest: testkit.Digest("permit-" + suffix), AttemptID: "runtime-attempt-" + suffix, Delegation: &delegation}
	providerObservation := runtimeports.ProviderAttemptObservationRefV2{Delegation: delegation, PreparedAttemptID: "prepared-" + suffix, ProviderOperationRef: "provider-operation-" + suffix, Revision: 1, State: runtimeports.ProviderAttemptObservedV2, Digest: testkit.Digest("provider-observation-" + suffix), PayloadDigest: testkit.Digest("provider-payload-" + suffix), PayloadRevision: 1, SourceRegistrationID: "provider-source-" + suffix, SourceEpoch: 1, SourceSequence: 1, Evidence: runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: testkit.Digest("provider-ledger-" + suffix), Sequence: 1, RecordDigest: testkit.Digest("provider-record-" + suffix)}, ObservedUnixNano: fixture.now.Add(3 * time.Second).UnixNano()}
	output := autoReviewerOutputV1(t)
	observation, err := contract.SealAutoReviewerInvocationObservationV1(contract.AutoReviewerInvocationObservationV1{FactIdentityV1: contract.FactIdentityV1{TenantID: current.TenantID, ID: "auto-observation-" + suffix, Revision: 1, CreatedUnixNano: fixture.now.Add(3 * time.Second).UnixNano(), UpdatedUnixNano: fixture.now.Add(3 * time.Second).UnixNano()}, AttemptID: current.ID, AttemptRevision: invocationAttempt.Revision, AttemptDigest: invocationAttempt.Digest, OperationDigest: current.OperationDigest, RuntimeAttempt: runtimeAttempt, ProviderObservation: providerObservation, Output: output, ResultSchema: current.ResultSchema, Tokens: 100, CostMicros: 50, ObservedUnixNano: fixture.now.Add(3 * time.Second).UnixNano(), ExpiresUnixNano: fixture.now.Add(8 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	result, err := contract.SealReviewerInvocationResultFactV1(contract.ReviewerInvocationResultFactV1{FactIdentityV1: contract.FactIdentityV1{TenantID: current.TenantID, ID: "auto-result-" + suffix, Revision: 1, CreatedUnixNano: fixture.now.Add(3 * time.Second).UnixNano(), UpdatedUnixNano: fixture.now.Add(3 * time.Second).UnixNano()}, CaseID: current.Case.ID, CaseRevision: current.Case.Revision, RoundID: current.Round.ID, RoundRevision: current.Round.Revision, RoundDigest: current.Round.Digest, AssignmentID: current.Assignment.ID, AssignmentRevision: current.Assignment.Revision, AssignmentDigest: current.Assignment.Digest, TargetID: current.Target.ID, TargetRevision: current.Target.Revision, TargetDigest: current.Target.Digest, AttemptID: runtimeAttempt.AttemptID, ResultSchema: current.ResultSchema, ResultDigest: output.Digest, ObservationRefs: []string{observation.ID}})
	if err != nil {
		t.Fatal(err)
	}
	next := current
	next.Revision++
	next.UpdatedUnixNano = fixture.now.Add(4 * time.Second).UnixNano()
	next.State = contract.AutoReviewerAttemptObservedV1
	next.InvocationAttempt = refPtrResourceV1(invocationAttempt)
	observationRef, resultRef := observation.Ref(), result.ExactRef()
	next.Observation, next.DomainResult = &observationRef, &resultRef
	next.Digest = ""
	next, err = contract.SealAutoReviewerAttemptV1(next)
	if err != nil {
		t.Fatal(err)
	}
	return reviewport.RecordAutoReviewerObservationMutationV1{Expected: current.ExactRef(), Next: next, Observation: observation, DomainResult: result}
}

func autoReviewerStartClaimMutationV1(t testing.TB, current contract.AutoReviewerAttemptV1, updated time.Time) reviewport.MarkAutoReviewerWaitingInspectMutationV1 {
	t.Helper()
	next := current
	next.Revision++
	next.UpdatedUnixNano = updated.UnixNano()
	next.State = contract.AutoReviewerAttemptWaitingInspectV1
	origin := current.ExactRef()
	next.InvocationAttempt = &origin
	next.Digest = ""
	next, err := contract.SealAutoReviewerAttemptV1(next)
	if err != nil {
		t.Fatal(err)
	}
	return reviewport.MarkAutoReviewerWaitingInspectMutationV1{Expected: current.ExactRef(), Next: next}
}

func applyAutoReviewerStartClaimV1(t testing.TB, store reviewport.AutoReviewerStoreV1, fixture autoReviewerFixtureV1, current contract.AutoReviewerAttemptV1) contract.AutoReviewerAttemptV1 {
	t.Helper()
	mutation := autoReviewerStartClaimMutationV1(t, current, fixture.now.Add(2*time.Second))
	receipt, err := store.MarkAutoReviewerWaitingInspectV1(fixture.ctx, mutation)
	if err != nil || !receipt.Applied {
		t.Fatalf("persistent Auto Reviewer start claim failed: receipt=%+v err=%v", receipt, err)
	}
	return receipt.Attempt
}

func autoReviewerObservedMutationWithOutputV1(t testing.TB, fixture autoReviewerFixtureV1, current contract.AutoReviewerAttemptV1, suffix string, output contract.AutoReviewerStructuredOutputV1) reviewport.RecordAutoReviewerObservationMutationV1 {
	t.Helper()
	mutation := autoReviewerObservedMutationV1(t, fixture, current, suffix)
	mutation.Observation.Output = output
	mutation.Observation.Digest = ""
	var err error
	mutation.Observation, err = contract.SealAutoReviewerInvocationObservationV1(mutation.Observation)
	if err != nil {
		t.Fatal(err)
	}
	mutation.DomainResult.ResultDigest = output.Digest
	mutation.DomainResult.Digest = ""
	mutation.DomainResult, err = contract.SealReviewerInvocationResultFactV1(mutation.DomainResult)
	if err != nil {
		t.Fatal(err)
	}
	observationRef, resultRef := mutation.Observation.Ref(), mutation.DomainResult.ExactRef()
	mutation.Next.Observation, mutation.Next.DomainResult = &observationRef, &resultRef
	mutation.Next.Digest = ""
	mutation.Next, err = contract.SealAutoReviewerAttemptV1(mutation.Next)
	if err != nil {
		t.Fatal(err)
	}
	return mutation
}

func repeatedRejectOutputV1(t testing.TB) contract.AutoReviewerStructuredOutputV1 {
	t.Helper()
	evidence := []runtimeports.ReviewEvidenceRefV2{{Ref: "evidence://authority", Classification: "review.authority/current", Digest: testkit.Digest("authority-current")}, {Ref: "evidence://scope", Classification: "review.scope/current", Digest: testkit.Digest("scope-current")}}
	value, err := contract.SealAutoReviewerStructuredOutputV1(contract.AutoReviewerStructuredOutputV1{Resolution: contract.ResolutionRejectV1, ReasonCodes: []string{"review.auto/rejected"}, Findings: []contract.AutoFindingDraftV1{{Category: "safety", Priority: "p1", Anchor: "operation", Claim: "unsafe operation remains", Impact: "external effect", Evidence: evidence}}, Evidence: evidence})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestAutoReviewerStoreMemorySQLiteRestartAndLostReplyV1(t *testing.T) {
	now := time.Unix(1_902_000_000, 0)
	t.Run("memory", func(t *testing.T) {
		store := storetestkit.NewMemoryStoreV1(func() time.Time { return now.Add(time.Hour) })
		fixture := newAutoReviewerFixtureV1(t, store, now, "memory")
		created, err := store.BeginAutoReviewerAttemptV1(fixture.ctx, reviewport.BeginAutoReviewerAttemptMutationV1{Attempt: fixture.attempt})
		if err != nil {
			t.Fatal(err)
		}
		waiting := applyAutoReviewerStartClaimV1(t, store, fixture, created)
		mutation := autoReviewerObservedMutationV1(t, fixture, waiting, "memory")
		observed, result, err := store.RecordAutoReviewerObservationV1(fixture.ctx, mutation)
		if err != nil {
			t.Fatal(err)
		}
		if replayed, replayResult, err := store.RecordAutoReviewerObservationV1(fixture.ctx, mutation); err != nil || replayed.Digest != observed.Digest || replayResult.Digest != result.Digest {
			t.Fatalf("canonical lost-reply replay failed: %+v %+v %v", replayed, replayResult, err)
		}
		if historical, err := store.InspectAutoReviewerAttemptExactV1(fixture.ctx, created.TenantID, created.ExactRef()); err != nil || historical.State != contract.AutoReviewerAttemptPreparedV1 {
			t.Fatalf("historical exact Attempt was lost: %+v %v", historical, err)
		}
		copyObservation, err := store.InspectAutoReviewerObservationExactV1(fixture.ctx, observed.TenantID, mutation.Observation.Ref())
		if err != nil {
			t.Fatal(err)
		}
		copyObservation.Output.ReasonCodes[0] = "mutated"
		again, _ := store.InspectAutoReviewerObservationExactV1(fixture.ctx, observed.TenantID, mutation.Observation.Ref())
		if again.Output.ReasonCodes[0] == "mutated" {
			t.Fatal("Observation exact Inspect leaked mutable alias")
		}
		checked := fixture.now.Add(5 * time.Second)
		request := reviewport.AutoReviewTerminationCurrentRequestV1{TenantID: fixture.target.TenantID, Target: exactResourceV1(fixture.target.FactIdentityV1), Case: exactResourceV1(fixture.caseValue.FactIdentityV1), Rubric: fixture.rubric.ExactRef(), ExpectedRound: exactResourceV1(fixture.round.FactIdentityV1), CheckedUnixNano: checked.UnixNano()}
		projection, err := store.InspectAutoReviewTerminationCurrentV1(fixture.ctx, request)
		if err != nil || projection.RoundCount != 1 || projection.HighestRoundOrdinal != 1 || projection.RepeatedFindingCount != 0 || projection.RepeatedRejectionCount != 0 {
			t.Fatalf("termination current cut drifted: projection=%+v err=%v", projection, err)
		}
		if err := projection.ValidateCurrent(request, checked); err != nil {
			t.Fatal(err)
		}
		if err := projection.ValidateCurrent(request, checked.Add(-time.Nanosecond)); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("termination current accepted clock rollback: %v", err)
		}
		drift := request
		drift.Case.Digest = testkit.Digest("another-case-revision")
		if _, err := store.InspectAutoReviewTerminationCurrentV1(fixture.ctx, drift); !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("termination current accepted exact Case drift: %v", err)
		}
	})
	t.Run("sqlite_restart", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "auto-reviewer.sqlite")
		store, err := reviewsqlite.Open(context.Background(), reviewsqlite.Config{Path: path, Clock: func() time.Time { return now.Add(time.Hour) }})
		if err != nil {
			t.Fatal(err)
		}
		fixture := newAutoReviewerFixtureV1(t, store, now, "sqlite")
		created, err := store.BeginAutoReviewerAttemptV1(fixture.ctx, reviewport.BeginAutoReviewerAttemptMutationV1{Attempt: fixture.attempt})
		if err != nil {
			t.Fatal(err)
		}
		waiting := applyAutoReviewerStartClaimV1(t, store, fixture, created)
		mutation := autoReviewerObservedMutationV1(t, fixture, waiting, "sqlite")
		observed, _, err := store.RecordAutoReviewerObservationV1(fixture.ctx, mutation)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		restarted, err := reviewsqlite.Open(context.Background(), reviewsqlite.Config{Path: path, Clock: func() time.Time { return now.Add(time.Minute) }})
		if err != nil {
			t.Fatal(err)
		}
		defer restarted.Close()
		if err := restarted.IntegrityCheckV1(context.Background()); err != nil {
			t.Fatal(err)
		}
		if current, err := restarted.InspectAutoReviewerAttemptCurrentV1(context.Background(), observed.TenantID, observed.ID); err != nil || current.Digest != observed.Digest {
			t.Fatalf("Auto Reviewer Attempt did not survive restart: %+v %v", current, err)
		}
	})
}

func TestAutoReviewerUnknownOutcomeIsInspectOnlyAndAtomicV1(t *testing.T) {
	now := time.Unix(1_902_000_100, 0)
	store := storetestkit.NewMemoryStoreV1(func() time.Time { return now.Add(time.Hour) })
	fixture := newAutoReviewerFixtureV1(t, store, now, "unknown")
	created, err := store.BeginAutoReviewerAttemptV1(fixture.ctx, reviewport.BeginAutoReviewerAttemptMutationV1{Attempt: fixture.attempt})
	if err != nil {
		t.Fatal(err)
	}
	waiting := created
	waiting.Revision++
	waiting.UpdatedUnixNano = time.Unix(0, created.UpdatedUnixNano).Add(time.Second).UnixNano()
	waiting.State = contract.AutoReviewerAttemptWaitingInspectV1
	waiting.InvocationAttempt = refPtrResourceV1(created.ExactRef())
	waiting.Digest = ""
	waiting, err = contract.SealAutoReviewerAttemptV1(waiting)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := store.MarkAutoReviewerWaitingInspectV1(fixture.ctx, reviewport.MarkAutoReviewerWaitingInspectMutationV1{Expected: created.ExactRef(), Next: waiting})
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Applied || receipt.Attempt.Digest != waiting.Digest {
		t.Fatalf("waiting start claim did not return the unique applied receipt: %+v", receipt)
	}
	waiting = receipt.Attempt
	replayed, replayErr := store.MarkAutoReviewerWaitingInspectV1(fixture.ctx, reviewport.MarkAutoReviewerWaitingInspectMutationV1{Expected: created.ExactRef(), Next: waiting})
	if replayErr != nil || replayed.Applied || replayed.Attempt.ExactRef() != waiting.ExactRef() {
		t.Fatalf("canonical start-claim replay did not return applied=false: %+v err=%v", replayed, replayErr)
	}
	changed := fixture.attempt
	changed.ID = "blind-redispatch"
	changed.IdempotencyKey = fixture.attempt.IdempotencyKey
	changed.Digest = ""
	changed, _ = contract.SealAutoReviewerAttemptV1(changed)
	if _, err := store.BeginAutoReviewerAttemptV1(fixture.ctx, reviewport.BeginAutoReviewerAttemptMutationV1{Attempt: changed}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("waiting_inspect allowed a new dispatch identity: %v", err)
	}
	mutation := autoReviewerObservedMutationV1(t, fixture, waiting, "unknown")
	bad := mutation
	bad.DomainResult.ResultDigest = testkit.Digest("drift")
	bad.DomainResult.Digest = ""
	bad.DomainResult, _ = contract.SealReviewerInvocationResultFactV1(bad.DomainResult)
	bad.Next.DomainResult = refPtrResourceV1(bad.DomainResult.ExactRef())
	bad.Next.Digest = ""
	bad.Next, _ = contract.SealAutoReviewerAttemptV1(bad.Next)
	if _, _, err := store.RecordAutoReviewerObservationV1(fixture.ctx, bad); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("drifted DomainResult did not fail closed: %v", err)
	}
	if _, err := store.InspectAutoReviewerObservationExactV1(fixture.ctx, fixture.attempt.TenantID, bad.Observation.Ref()); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("failed staged mutation leaked Observation: %v", err)
	}
	if _, err := store.InspectDomainResultExactV1(fixture.ctx, fixture.attempt.TenantID, reviewport.ExactV1(bad.DomainResult.ID, bad.DomainResult.Revision, bad.DomainResult.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("failed staged mutation leaked DomainResult: %v", err)
	}
	if _, _, err := store.RecordAutoReviewerObservationV1(fixture.ctx, mutation); err != nil {
		t.Fatal(err)
	}
}

func TestAutoReviewTerminationCurrentCountsTargetFamilyHistoryV1(t *testing.T) {
	base := time.Unix(1_902_000_150, 0)
	store := storetestkit.NewMemoryStoreV1(func() time.Time { return base.Add(time.Hour) })
	fixture := newAutoReviewerFixtureV1(t, store, base, "loop-1")
	first, err := store.BeginAutoReviewerAttemptV1(fixture.ctx, reviewport.BeginAutoReviewerAttemptMutationV1{Attempt: fixture.attempt})
	if err != nil {
		t.Fatal(err)
	}
	first = applyAutoReviewerStartClaimV1(t, store, fixture, first)
	output := repeatedRejectOutputV1(t)
	firstMutation := autoReviewerObservedMutationWithOutputV1(t, fixture, first, "loop-1", output)
	if _, _, err := store.RecordAutoReviewerObservationV1(fixture.ctx, firstMutation); err != nil {
		t.Fatal(err)
	}

	step := fixture.now.Add(10 * time.Second)
	engine, err := caseengine.New(store, func() time.Time { current := step; step = step.Add(time.Second); return current })
	if err != nil {
		t.Fatal(err)
	}
	currentCase := fixture.caseValue
	waitingRevision := testkit.CaseSuccessor(step, currentCase, contract.CaseWaitingRevisionV1)
	currentCase, err = store.AdvanceCaseForTestV1(fixture.ctx, reviewport.ExpectedV1(currentCase.Revision, currentCase.Digest), waitingRevision)
	if err != nil {
		t.Fatal(err)
	}
	step = step.Add(time.Second)
	currentCase, err = engine.TransitionWithTraceV2(fixture.ctx, caseengine.TransitionWithTraceCommandV2{TransitionCommandV1: caseengine.TransitionCommandV1{TenantID: currentCase.TenantID, CaseID: currentCase.ID, Expected: reviewport.ExpectedV1(currentCase.Revision, currentCase.Digest), Next: contract.CaseRoutedV1}, Trace: testkit.TransitionTrace(step, currentCase, contract.CaseRoutedV1)})
	if err != nil {
		t.Fatal(err)
	}
	round2 := testkit.Round(step, currentCase, contract.RouteAutoV1)
	round2.ID, round2.AssignmentID, round2.RubricDigest, round2.Digest = "round-auto-loop-2", "assignment-auto-loop-2", fixture.rubric.Digest, ""
	round2, err = contract.SealReviewRoundV1(round2)
	if err != nil {
		t.Fatal(err)
	}
	assignment2 := testkit.Assignment(step, currentCase, round2, contract.RouteAutoV1)
	assignment2.ID, assignment2.RoundID, assignment2.RoundDigest, assignment2.Digest = round2.AssignmentID, round2.ID, round2.Digest, ""
	assignment2, err = contract.SealReviewerAssignmentV1(assignment2)
	if err != nil {
		t.Fatal(err)
	}
	currentCase, _, assignment2, err = engine.StartRoundV1(fixture.ctx, reviewport.StartRoundMutationV1{Expected: reviewport.ExpectedV1(currentCase.Revision, currentCase.Digest), Round: round2, Assignment: assignment2, Trace: testkit.Trace(step, currentCase, contract.TraceAssignedV1, 3, round2.ID, assignment2.ID)})
	if err != nil {
		t.Fatal(err)
	}
	currentCase, assignment2, err = engine.ClaimAssignmentV1(fixture.ctx, reviewport.ClaimAssignmentMutationV1{TenantID: currentCase.TenantID, ExpectedCase: reviewport.ExpectedV1(currentCase.Revision, currentCase.Digest), ExpectedAssignment: reviewport.ExpectedV1(assignment2.Revision, assignment2.Digest), CaseID: currentCase.ID, AssignmentID: assignment2.ID, LeaseHolder: "auto-worker-loop-2", LeaseExpiresUnixNano: base.Add(15 * time.Minute).UnixNano(), UpdatedUnixNano: step.Add(time.Second).UnixNano(), Traces: []contract.TraceFactV1{testkit.StartedTrace(step.Add(time.Second), currentCase, assignment2.ID)}})
	if err != nil {
		t.Fatal(err)
	}
	second := fixture.attempt
	second.ID, second.IdempotencyKey = "auto-attempt-loop-2", "auto-idem-loop-2"
	second.Case, second.Round, second.Assignment = exactResourceV1(currentCase.FactIdentityV1), exactResourceV1(round2.FactIdentityV1), exactResourceV1(assignment2.FactIdentityV1)
	second.InvocationEffect.EffectID = "auto-effect-loop-2"
	second.RoundOrdinal = 2
	second.CreatedUnixNano, second.UpdatedUnixNano = step.Add(2*time.Second).UnixNano(), step.Add(2*time.Second).UnixNano()
	second.Digest = ""
	second, err = contract.SealAutoReviewerAttemptV1(second)
	if err != nil {
		t.Fatal(err)
	}
	second, err = store.BeginAutoReviewerAttemptV1(fixture.ctx, reviewport.BeginAutoReviewerAttemptMutationV1{Attempt: second})
	if err != nil {
		t.Fatal(err)
	}
	fixture2 := fixture
	fixture2.now, fixture2.caseValue, fixture2.round, fixture2.assignment = step.Add(2*time.Second), currentCase, round2, assignment2
	second = applyAutoReviewerStartClaimV1(t, store, fixture2, second)
	secondMutation := autoReviewerObservedMutationWithOutputV1(t, fixture2, second, "loop-2", output)
	if _, _, err := store.RecordAutoReviewerObservationV1(fixture.ctx, secondMutation); err != nil {
		t.Fatal(err)
	}
	checked := fixture2.now.Add(5 * time.Second)
	request := reviewport.AutoReviewTerminationCurrentRequestV1{TenantID: fixture.target.TenantID, Target: exactResourceV1(fixture.target.FactIdentityV1), Case: exactResourceV1(currentCase.FactIdentityV1), Rubric: fixture.rubric.ExactRef(), ExpectedRound: exactResourceV1(round2.FactIdentityV1), CheckedUnixNano: checked.UnixNano()}
	projection, err := store.InspectAutoReviewTerminationCurrentV1(fixture.ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if projection.RoundCount != 2 || projection.HighestRoundOrdinal != 2 || projection.RepeatedFindingCount != 2 || projection.RepeatedRejectionCount != 2 {
		t.Fatalf("target-family termination history drifted: %+v", projection)
	}
}

func refPtrResourceV1(value contract.ExactResourceRefV1) *contract.ExactResourceRefV1 { return &value }

func TestAutoReviewerConcurrentCreateAndFinalizeHaveOneCanonicalWinnerV1(t *testing.T) {
	now := time.Unix(1_902_000_200, 0)
	store := storetestkit.NewMemoryStoreV1(func() time.Time { return now.Add(time.Hour) })
	fixture := newAutoReviewerFixtureV1(t, store, now, "race")
	var createSuccess atomic.Int32
	var wg sync.WaitGroup
	for index := 0; index < 64; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := store.BeginAutoReviewerAttemptV1(fixture.ctx, reviewport.BeginAutoReviewerAttemptMutationV1{Attempt: fixture.attempt}); err == nil {
				createSuccess.Add(1)
			}
		}()
	}
	wg.Wait()
	if createSuccess.Load() != 64 {
		t.Fatalf("same-canonical create was not idempotent: successes=%d", createSuccess.Load())
	}
	created, _ := store.InspectAutoReviewerAttemptCurrentV1(fixture.ctx, fixture.attempt.TenantID, fixture.attempt.ID)
	created = applyAutoReviewerStartClaimV1(t, store, fixture, created)
	mutations := make([]reviewport.RecordAutoReviewerObservationMutationV1, 64)
	for index := range mutations {
		mutations[index] = autoReviewerObservedMutationV1(t, fixture, created, fmt.Sprintf("race-%02d", index))
	}
	var finalizeSuccess atomic.Int32
	for index := range mutations {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			if _, _, err := store.RecordAutoReviewerObservationV1(fixture.ctx, mutations[index]); err == nil {
				finalizeSuccess.Add(1)
			} else if !core.HasCategory(err, core.ErrorConflict) {
				t.Errorf("unexpected finalize error: %v", err)
			}
		}(index)
	}
	wg.Wait()
	if finalizeSuccess.Load() != 1 {
		t.Fatalf("concurrent distinct Observations produced %d winners", finalizeSuccess.Load())
	}
	current, err := store.InspectAutoReviewerAttemptCurrentV1(fixture.ctx, created.TenantID, created.ID)
	if err != nil || current.State != contract.AutoReviewerAttemptObservedV1 {
		t.Fatalf("final current Attempt is invalid: %+v %v", current, err)
	}
}

func TestAutoReviewerStoreConformanceMemoryAndSQLiteV1(t *testing.T) {
	now := time.Unix(1_902_000_300, 0)
	t.Run("memory", func(t *testing.T) {
		store := storetestkit.NewMemoryStoreV1(func() time.Time { return now.Add(time.Hour) })
		fixture := newAutoReviewerFixtureV1(t, store, now, "conformance-memory")
		direct := autoReviewerObservedMutationV1(t, fixture, fixture.attempt, "conformance-memory-direct")
		claim := autoReviewerStartClaimMutationV1(t, fixture.attempt, fixture.now.Add(2*time.Second))
		observe := autoReviewerObservedMutationV1(t, fixture, claim.Next, "conformance-memory")
		if err := conformance.CheckAutoReviewerStoreV1(fixture.ctx, store, conformance.AutoReviewerStoreFixtureV1{Begin: reviewport.BeginAutoReviewerAttemptMutationV1{Attempt: fixture.attempt}, DirectObserve: direct, Claim: claim, Observe: observe}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("sqlite", func(t *testing.T) {
		store, err := reviewsqlite.Open(context.Background(), reviewsqlite.Config{Path: filepath.Join(t.TempDir(), "auto-conformance.sqlite"), Clock: func() time.Time { return now.Add(time.Hour) }})
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		fixture := newAutoReviewerFixtureV1(t, store, now, "conformance-sqlite")
		direct := autoReviewerObservedMutationV1(t, fixture, fixture.attempt, "conformance-sqlite-direct")
		claim := autoReviewerStartClaimMutationV1(t, fixture.attempt, fixture.now.Add(2*time.Second))
		observe := autoReviewerObservedMutationV1(t, fixture, claim.Next, "conformance-sqlite")
		if err := conformance.CheckAutoReviewerStoreV1(fixture.ctx, store, conformance.AutoReviewerStoreFixtureV1{Begin: reviewport.BeginAutoReviewerAttemptMutationV1{Attempt: fixture.attempt}, DirectObserve: direct, Claim: claim, Observe: observe}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestAutoReviewerCrossFactRubricBudgetAndCurrentDriftFailClosedV1(t *testing.T) {
	now := time.Unix(1_902_000_400, 0)
	t.Run("begin_cross_fact", func(t *testing.T) {
		for name, mutate := range map[string]func(*contract.AutoReviewerAttemptV1){
			"case":  func(value *contract.AutoReviewerAttemptV1) { value.Case.Digest = testkit.Digest("wrong-case") },
			"round": func(value *contract.AutoReviewerAttemptV1) { value.Round.Digest = testkit.Digest("wrong-round") },
			"assignment": func(value *contract.AutoReviewerAttemptV1) {
				value.Assignment.Digest = testkit.Digest("wrong-assignment")
			},
			"target":   func(value *contract.AutoReviewerAttemptV1) { value.Target.Digest = testkit.Digest("wrong-target") },
			"rubric":   func(value *contract.AutoReviewerAttemptV1) { value.Rubric.Digest = testkit.Digest("wrong-rubric") },
			"reviewer": func(value *contract.AutoReviewerAttemptV1) { value.ReviewerID = "another-reviewer" },
			"binding": func(value *contract.AutoReviewerAttemptV1) {
				value.ReviewerBinding.ArtifactDigest = testkit.Digest("wrong-binding")
			},
		} {
			t.Run(name, func(t *testing.T) {
				store := storetestkit.NewMemoryStoreV1(func() time.Time { return now.Add(time.Hour) })
				fixture := newAutoReviewerFixtureV1(t, store, now, "drift-"+name)
				value := fixture.attempt
				mutate(&value)
				value.Digest = ""
				value, err := contract.SealAutoReviewerAttemptV1(value)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := store.BeginAutoReviewerAttemptV1(fixture.ctx, reviewport.BeginAutoReviewerAttemptMutationV1{Attempt: value}); err == nil {
					t.Fatal("cross-fact Attempt was admitted")
				}
				if _, err := store.InspectAutoReviewerAttemptCurrentV1(fixture.ctx, value.TenantID, value.ID); !core.HasCategory(err, core.ErrorNotFound) {
					t.Fatalf("failed Begin leaked Attempt: %v", err)
				}
			})
		}
	})
	t.Run("budgets", func(t *testing.T) {
		for name, mutate := range map[string]func(*reviewport.RecordAutoReviewerObservationMutationV1){
			"tokens": func(value *reviewport.RecordAutoReviewerObservationMutationV1) {
				value.Observation.Tokens = 64_001
			},
			"cost": func(value *reviewport.RecordAutoReviewerObservationMutationV1) {
				value.Observation.CostMicros = value.Next.MaxCostMicros + 1
			},
		} {
			t.Run(name, func(t *testing.T) {
				store := storetestkit.NewMemoryStoreV1(func() time.Time { return now.Add(time.Hour) })
				fixture := newAutoReviewerFixtureV1(t, store, now, "budget-"+name)
				created, _ := store.BeginAutoReviewerAttemptV1(fixture.ctx, reviewport.BeginAutoReviewerAttemptMutationV1{Attempt: fixture.attempt})
				waiting := applyAutoReviewerStartClaimV1(t, store, fixture, created)
				mutation := autoReviewerObservedMutationV1(t, fixture, waiting, "budget-"+name)
				mutate(&mutation)
				mutation.Observation.Digest = ""
				var err error
				mutation.Observation, err = contract.SealAutoReviewerInvocationObservationV1(mutation.Observation)
				if err != nil {
					t.Fatal(err)
				}
				observationRef := mutation.Observation.Ref()
				mutation.Next.Observation = &observationRef
				mutation.Next.Digest = ""
				mutation.Next, err = contract.SealAutoReviewerAttemptV1(mutation.Next)
				if err != nil {
					t.Fatal(err)
				}
				if _, _, err := store.RecordAutoReviewerObservationV1(fixture.ctx, mutation); !core.HasCategory(err, core.ErrorPreconditionFailed) {
					t.Fatalf("budget breach was not rejected: %v", err)
				}
				if _, err := store.InspectAutoReviewerObservationExactV1(fixture.ctx, created.TenantID, mutation.Observation.Ref()); !core.HasCategory(err, core.ErrorNotFound) {
					t.Fatalf("budget breach leaked Observation: %v", err)
				}
			})
		}
	})
}

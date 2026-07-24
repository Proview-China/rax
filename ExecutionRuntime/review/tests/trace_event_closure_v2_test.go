package review_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/caseengine"
	"github.com/Proview-China/rax/ExecutionRuntime/review/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/storetestkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/service"
	"github.com/Proview-China/rax/ExecutionRuntime/review/verdictowner"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func traceFindingV2(t *testing.T, f *flow, id string, sequence uint64) (contract.FindingV1, contract.TraceFactV1) {
	t.Helper()
	evidence := []runtimeports.ReviewEvidenceRefV2{testkit.Evidence("finding-" + id)}
	finding, err := contract.SealFindingV1(contract.FindingV1{
		FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: f.caseValue.TenantID, ID: id, Revision: 1, CreatedUnixNano: f.clock.Now().UnixNano(), UpdatedUnixNano: f.clock.Now().UnixNano()},
		CaseID:         f.caseValue.ID, CaseRevision: f.caseValue.Revision, RoundID: f.round.ID, RoundRevision: f.round.Revision, RoundDigest: f.round.Digest,
		TargetID: f.caseValue.TargetID, TargetRevision: f.caseValue.TargetRevision, TargetDigest: f.caseValue.TargetDigest,
		Category: "review.test/correctness", Priority: "high", Anchor: "candidate.go:1", Claim: "candidate violates invariant", Impact: "execution remains blocked",
		Evidence: evidence, Status: contract.FindingOpenV1, ExpiresUnixNano: f.clock.Now().Add(5 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	trace := testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceFindingV1, sequence, finding.ID)
	return finding, trace
}

func newWaitingReviewerFlowV2(t *testing.T) *flow {
	t.Helper()
	clock := testkit.NewClock(time.Unix(1_950_000_000, 0))
	ctx := context.Background()
	store := storetestkit.NewMemoryStoreV1(clock.Now)
	engine, err := caseengine.New(store, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	target := testkit.Target(clock.Now())
	testkit.PublishRubric(ctx, store, clock.Now(), target.TenantID)
	request := testkit.Request(clock.Now(), target, "case-trace-v2")
	clock.Advance(time.Second)
	c, err := engine.CreateCaseV1(ctx, caseengine.CreateCaseCommandV1{CaseID: "case-trace-v2", Request: &request, Target: target, ExpiresUnixNano: request.ExpiresUnixNano, Trace: testkit.TraceForTarget(clock.Now(), "case-trace-v2", target, contract.TraceRequestedV1, 1, request.ID)})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []contract.CaseStateV1{contract.CaseAdmittedV1, contract.CaseRoutedV1} {
		clock.Advance(time.Second)
		c, err = engine.TransitionWithTraceV2(ctx, caseengine.TransitionWithTraceCommandV2{TransitionCommandV1: caseengine.TransitionCommandV1{TenantID: c.TenantID, CaseID: c.ID, Expected: reviewport.ExpectedV1(c.Revision, c.Digest), Next: state}, Trace: testkit.TransitionTrace(clock.Now(), c, state)})
		if err != nil {
			t.Fatal(err)
		}
	}
	clock.Advance(time.Second)
	round := testkit.Round(clock.Now(), c, contract.RouteHumanV1)
	assignment := testkit.Assignment(clock.Now(), c, round, contract.RouteHumanV1)
	c, round, assignment, err = engine.StartRoundV1(ctx, reviewport.StartRoundMutationV1{Expected: reviewport.ExpectedV1(c.Revision, c.Digest), Round: round, Assignment: assignment, Trace: testkit.Trace(clock.Now(), c, contract.TraceAssignedV1, 2, round.ID, assignment.ID)})
	if err != nil {
		t.Fatal(err)
	}
	return &flow{ctx: ctx, clock: clock, store: store, engine: engine, target: target, caseValue: c, round: round, assignment: assignment, sequence: 3}
}

func TestTraceEventV2ClaimStartedIsAtomic(t *testing.T) {
	f := newWaitingReviewerFlowV2(t)
	f.clock.Advance(time.Second)
	successor := f.caseValue
	successor.Revision++
	started := testkit.Trace(f.clock.Now(), successor, contract.TraceStartedV1, 3, f.assignment.ID)
	c, assignment, err := f.engine.ClaimAssignmentV1(f.ctx, reviewport.ClaimAssignmentMutationV1{
		TenantID: f.caseValue.TenantID, CaseID: f.caseValue.ID, AssignmentID: f.assignment.ID,
		ExpectedCase: reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), ExpectedAssignment: reviewport.ExpectedV1(f.assignment.Revision, f.assignment.Digest),
		LeaseHolder: "reviewer-a", LeaseExpiresUnixNano: f.clock.Now().Add(10 * time.Minute).UnixNano(), UpdatedUnixNano: f.clock.Now().UnixNano(), Traces: []contract.TraceFactV1{started},
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.State != contract.CaseReviewingV1 || assignment.State != contract.AssignmentClaimedV1 {
		t.Fatalf("claim closure drifted: case=%s assignment=%s", c.State, assignment.State)
	}
	if got, err := f.store.InspectTraceExactV1(f.ctx, started.TenantID, reviewport.ExactV1(started.ID, started.Revision, started.Digest)); err != nil || got.Event != contract.TraceStartedV1 {
		t.Fatalf("started trace missing: %+v err=%v", got, err)
	}
}

func TestTraceEventV2ProductionClaimWithoutStartedFailsClosed(t *testing.T) {
	f := newWaitingReviewerFlowV2(t)
	owner, err := service.New(f.store, f.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Second)
	mutation := reviewport.ClaimAssignmentMutationV1{
		TenantID: f.caseValue.TenantID, CaseID: f.caseValue.ID, AssignmentID: f.assignment.ID,
		ExpectedCase: reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), ExpectedAssignment: reviewport.ExpectedV1(f.assignment.Revision, f.assignment.Digest),
		LeaseHolder: "reviewer-a", LeaseExpiresUnixNano: f.clock.Now().Add(10 * time.Minute).UnixNano(), UpdatedUnixNano: f.clock.Now().UnixNano(),
	}
	if _, _, engineErr := f.engine.ClaimAssignmentV1(f.ctx, mutation); !core.HasReason(engineErr, core.ReasonInvalidState) {
		t.Fatalf("eventless direct Engine Claim did not fail closed: %v", engineErr)
	}
	_, _, err = owner.ClaimV1(f.ctx, mutation)
	if !core.HasReason(err, core.ReasonInvalidState) {
		t.Fatalf("eventless production Claim did not fail closed: %v", err)
	}
	caseCurrent, caseErr := f.store.InspectCaseV1(f.ctx, f.caseValue.TenantID, f.caseValue.ID)
	assignmentCurrent, assignmentErr := f.store.InspectAssignmentV1(f.ctx, f.assignment.TenantID, f.assignment.ID)
	if caseErr != nil || assignmentErr != nil || caseCurrent.Digest != f.caseValue.Digest || assignmentCurrent.Digest != f.assignment.Digest {
		t.Fatalf("eventless production Claim changed facts: case=%+v assignment=%+v errors=%v/%v", caseCurrent, assignmentCurrent, caseErr, assignmentErr)
	}
}

func TestTraceEventV2ReviewDocumentEventVocabulary(t *testing.T) {
	now := time.Unix(1_956_000_000, 0)
	target := testkit.Target(now)
	caseFact := contract.ReviewCaseV1{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: "case-event-vocabulary", Revision: 1}, TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest}
	for sequence, event := range []contract.TraceEventV1{
		contract.TraceRequestedV1,
		contract.TraceAssignedV1,
		contract.TraceStartedV1,
		contract.TraceFindingV1,
		contract.TraceAttestedV1,
		contract.TraceVerdictV1,
		contract.TraceExpiredV1,
		contract.TraceEscalatedV1,
		contract.TraceSupersededV1,
		contract.TraceResolvedV1,
	} {
		if got := testkit.Trace(now, caseFact, event, uint64(sequence+1), "fact-a"); got.Event != event || got.Validate() != nil {
			t.Fatalf("Review.md event %q is not a sealed Trace: %+v", event, got)
		}
	}
}

func TestTraceEventV2ClaimConflictLeavesCaseAndAssignmentUnchanged(t *testing.T) {
	f := newWaitingReviewerFlowV2(t)
	f.clock.Advance(time.Second)
	successor := f.caseValue
	successor.Revision++
	started := testkit.Trace(f.clock.Now(), successor, contract.TraceStartedV1, 101, f.assignment.ID)
	occupied := testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceRoutedV1, 101, "occupied")
	occupied.ID = "trace-claim-occupied"
	occupied.Digest = ""
	occupied, _ = contract.SealTraceFactV1(occupied)
	if _, err := f.store.InjectTraceForTestV1(f.ctx, occupied); err != nil {
		t.Fatal(err)
	}
	_, _, err := f.engine.ClaimAssignmentV1(f.ctx, reviewport.ClaimAssignmentMutationV1{TenantID: f.caseValue.TenantID, CaseID: f.caseValue.ID, AssignmentID: f.assignment.ID, ExpectedCase: reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), ExpectedAssignment: reviewport.ExpectedV1(f.assignment.Revision, f.assignment.Digest), LeaseHolder: "reviewer-a", LeaseExpiresUnixNano: f.clock.Now().Add(10 * time.Minute).UnixNano(), UpdatedUnixNano: f.clock.Now().UnixNano(), Traces: []contract.TraceFactV1{started}})
	if !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("claim Trace conflict must fail closed, got %v", err)
	}
	caseCurrent, err := f.store.InspectCaseV1(f.ctx, f.caseValue.TenantID, f.caseValue.ID)
	if err != nil || caseCurrent.Digest != f.caseValue.Digest {
		t.Fatalf("failed claim changed Case: %+v err=%v", caseCurrent, err)
	}
	assignmentCurrent, err := f.store.InspectAssignmentV1(f.ctx, f.assignment.TenantID, f.assignment.ID)
	if err != nil || assignmentCurrent.Digest != f.assignment.Digest {
		t.Fatalf("failed claim changed Assignment: %+v err=%v", assignmentCurrent, err)
	}
	if _, err := f.store.InspectTraceExactV1(f.ctx, started.TenantID, reviewport.ExactV1(started.ID, started.Revision, started.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("failed claim leaked Started Trace: %v", err)
	}
}

func TestTraceEventV2FindingStagedConflictLeaksNothing(t *testing.T) {
	f := newReviewingFlow(t, contract.RouteHumanV1)
	finding, trace := traceFindingV2(t, f, "finding-staged-conflict", 71)
	occupied := testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceStartedV1, 71, f.assignment.ID)
	occupied.ID = "trace-occupied-source-sequence"
	occupied.Digest = ""
	occupied, _ = contract.SealTraceFactV1(occupied)
	if _, err := f.store.InjectTraceForTestV1(f.ctx, occupied); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.CreateFindingWithTraceV2(f.ctx, reviewport.CreateFindingWithTraceMutationV2{Finding: finding, Trace: trace}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("source conflict must fail closed, got %v", err)
	}
	if _, err := f.store.InspectFindingExactV1(f.ctx, finding.TenantID, reviewport.ExactV1(finding.ID, finding.Revision, finding.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("failed compound mutation leaked Finding: %v", err)
	}
	if _, err := f.store.InspectTraceExactV1(f.ctx, trace.TenantID, reviewport.ExactV1(trace.ID, trace.Revision, trace.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("failed compound mutation leaked Trace: %v", err)
	}
}

func TestTraceEventStoreV2ConformanceMemory(t *testing.T) {
	f := newReviewingFlow(t, contract.RouteHumanV1)
	finding, trace := traceFindingV2(t, f, "finding-conformance-v2", 70)
	if err := conformance.CheckTraceEventStoreV2(f.ctx, f.store, conformance.TraceEventStoreFixtureV2{Mutation: reviewport.CreateFindingWithTraceMutationV2{Finding: finding, Trace: trace}}); err != nil {
		t.Fatal(err)
	}
}

type loseFindingReplyStoreV2 struct {
	*memory.Store
	calls atomic.Int32
}

type loseClaimReplyStoreV2 struct {
	*memory.Store
	cancel context.CancelFunc
	calls  atomic.Int32
}

func (s *loseClaimReplyStoreV2) ClaimAssignmentV1(ctx context.Context, mutation reviewport.ClaimAssignmentMutationV1) (contract.ReviewCaseV1, contract.ReviewerAssignmentV1, error) {
	s.calls.Add(1)
	if _, _, err := s.Store.ClaimAssignmentV1(ctx, mutation); err != nil {
		return contract.ReviewCaseV1{}, contract.ReviewerAssignmentV1{}, err
	}
	s.cancel()
	return contract.ReviewCaseV1{}, contract.ReviewerAssignmentV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "committed claim response was lost")
}

type loseDecideReplyStoreV2 struct {
	*memory.Store
	cancel context.CancelFunc
	calls  atomic.Int32
}

type loseAttestationReplyStoreV2 struct {
	*memory.Store
	cancel context.CancelFunc
	calls  atomic.Int32
}

func (s *loseAttestationReplyStoreV2) RecordAttestationV1(ctx context.Context, mutation reviewport.RecordAttestationMutationV1) (contract.ReviewCaseV1, contract.AttestationV1, error) {
	s.calls.Add(1)
	if _, _, err := s.Store.RecordAttestationV1(ctx, mutation); err != nil {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, err
	}
	s.cancel()
	return contract.ReviewCaseV1{}, contract.AttestationV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEffectUnknownOutcome, "committed attestation response was lost")
}

func (s *loseDecideReplyStoreV2) DecideV1(ctx context.Context, mutation reviewport.DecideMutationV1) (contract.ReviewCaseV1, contract.VerdictV1, error) {
	s.calls.Add(1)
	if _, _, err := s.Store.DecideV1(ctx, mutation); err != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, err
	}
	s.cancel()
	return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEffectUnknownOutcome, "committed decision response was lost")
}

func (s *loseFindingReplyStoreV2) CreateFindingWithTraceV2(ctx context.Context, mutation reviewport.CreateFindingWithTraceMutationV2) (contract.FindingV1, error) {
	s.calls.Add(1)
	_, err := s.Store.CreateFindingWithTraceV2(ctx, mutation)
	if err != nil {
		return contract.FindingV1{}, err
	}
	return contract.FindingV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEffectUnknownOutcome, "committed response was lost")
}

func TestTraceEventV2FindingLostReplyRecoversByExactInspectOnly(t *testing.T) {
	f := newReviewingFlow(t, contract.RouteHumanV1)
	wrapper := &loseFindingReplyStoreV2{Store: f.store}
	owner, err := service.New(wrapper, f.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	finding, trace := traceFindingV2(t, f, "finding-lost-reply", 72)
	got, err := owner.CreateFindingWithTraceV2(f.ctx, reviewport.CreateFindingWithTraceMutationV2{Finding: finding, Trace: trace})
	if err != nil || got.Digest != finding.Digest {
		t.Fatalf("exact recovery failed: %+v err=%v", got, err)
	}
	if wrapper.calls.Load() != 1 {
		t.Fatalf("unknown outcome retried mutation %d times", wrapper.calls.Load())
	}
}

func TestTraceEventV2ClaimLostReplyUsesDetachedExactInspectOnly(t *testing.T) {
	f := newWaitingReviewerFlowV2(t)
	ctx, cancel := context.WithCancel(context.Background())
	wrapper := &loseClaimReplyStoreV2{Store: f.store, cancel: cancel}
	owner, err := service.New(wrapper, f.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Second)
	successor := f.caseValue
	successor.Revision++
	started := testkit.Trace(f.clock.Now(), successor, contract.TraceStartedV1, 74, f.assignment.ID)
	c, assignment, err := owner.ClaimV1(ctx, reviewport.ClaimAssignmentMutationV1{TenantID: f.caseValue.TenantID, CaseID: f.caseValue.ID, AssignmentID: f.assignment.ID, ExpectedCase: reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), ExpectedAssignment: reviewport.ExpectedV1(f.assignment.Revision, f.assignment.Digest), LeaseHolder: "reviewer-a", LeaseExpiresUnixNano: f.clock.Now().Add(10 * time.Minute).UnixNano(), UpdatedUnixNano: f.clock.Now().UnixNano(), Traces: []contract.TraceFactV1{started}})
	if err != nil || c.State != contract.CaseReviewingV1 || assignment.State != contract.AssignmentClaimedV1 {
		t.Fatalf("detached claim recovery failed: case=%+v assignment=%+v err=%v", c, assignment, err)
	}
	if ctx.Err() != context.Canceled || wrapper.calls.Load() != 1 {
		t.Fatalf("claim recovery replayed mutation or reused cancelled context: ctx=%v calls=%d", ctx.Err(), wrapper.calls.Load())
	}
	if _, err := f.store.InspectTraceExactV1(context.Background(), started.TenantID, reviewport.ExactV1(started.ID, started.Revision, started.Digest)); err != nil {
		t.Fatalf("claim recovery missed exact Started event: %v", err)
	}
}

func TestTraceEventV2EscalationLostReplyRecoversBothEventsExactly(t *testing.T) {
	f := newReviewingFlow(t, contract.RouteHumanV1)
	ctx, cancel := context.WithCancel(context.Background())
	wrapper := &loseAttestationReplyStoreV2{Store: f.store, cancel: cancel}
	engine, err := caseengine.New(wrapper, f.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Second)
	attestation := testkit.HumanAttestation(f.clock.Now(), f.caseValue, f.round, f.assignment, contract.ResolutionEscalateHumanV1, "idem-escalate-lost-reply")
	primary := testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceAttestedV1, 75, attestation.ID)
	successor := f.caseValue
	successor.Revision++
	escalated := testkit.Trace(f.clock.Now(), successor, contract.TraceEscalatedV1, 76, attestation.ID)
	c, got, err := engine.RecordAttestationWithTraceV2(ctx, reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), attestation, primary, []contract.TraceFactV1{escalated})
	if err != nil || c.State != contract.CaseWaitingHumanV1 || got.Digest != attestation.Digest {
		t.Fatalf("detached escalation recovery failed: case=%+v attestation=%+v err=%v", c, got, err)
	}
	if ctx.Err() != context.Canceled || wrapper.calls.Load() != 1 {
		t.Fatalf("escalation recovery replayed mutation or reused cancelled context: ctx=%v calls=%d", ctx.Err(), wrapper.calls.Load())
	}
	for _, event := range []contract.TraceFactV1{primary, escalated} {
		if _, err := f.store.InspectTraceExactV1(context.Background(), event.TenantID, reviewport.ExactV1(event.ID, event.Revision, event.Digest)); err != nil {
			t.Fatalf("escalation recovery missed exact %s event: %v", event.Event, err)
		}
	}
}

func TestTraceEventV2ConcurrentCanonicalFindingPublishesOnce(t *testing.T) {
	f := newReviewingFlow(t, contract.RouteHumanV1)
	finding, trace := traceFindingV2(t, f, "finding-concurrent", 73)
	mutation := reviewport.CreateFindingWithTraceMutationV2{Finding: finding, Trace: trace}
	const workers = 64
	errCh := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := f.store.CreateFindingWithTraceV2(f.ctx, mutation)
			if err == nil && got.Digest != finding.Digest {
				err = core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "canonical replay returned a different Finding")
			}
			errCh <- err
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	events, err := f.store.ListTraceV1(f.ctx, f.caseValue.TenantID, f.caseValue.ID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range events {
		if event.ID == trace.ID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("canonical event count=%d, want 1", count)
	}
}

func TestTraceEventV2PageExactCursorAndDeepClone(t *testing.T) {
	f := newReviewingFlow(t, contract.RouteHumanV1)
	for i, id := range []string{"finding-page-a", "finding-page-b", "finding-page-c"} {
		finding, trace := traceFindingV2(t, f, id, uint64(80+i))
		if _, err := f.store.CreateFindingWithTraceV2(f.ctx, reviewport.CreateFindingWithTraceMutationV2{Finding: finding, Trace: trace}); err != nil {
			t.Fatal(err)
		}
	}
	first, err := f.store.ListTracePageV2(f.ctx, reviewport.ListTracePageRequestV2{TenantID: f.caseValue.TenantID, CaseID: f.caseValue.ID, Limit: 2})
	if err != nil || len(first.Events) != 2 || first.Next == nil {
		t.Fatalf("first page: %+v err=%v", first, err)
	}
	all, err := f.store.ListTracePageV2(f.ctx, reviewport.ListTracePageRequestV2{TenantID: f.caseValue.TenantID, CaseID: f.caseValue.ID, Limit: reviewport.MaxTracePageV2})
	if err != nil || len(all.Events) < 3 {
		t.Fatalf("all events: %+v err=%v", all, err)
	}
	original := all.Events[len(all.Events)-1]
	if len(original.FactRefs) == 0 {
		t.Fatal("FindingObserved event lost its exact Finding ref")
	}
	all.Events[len(all.Events)-1].FactRefs[0] = "mutated"
	stored, err := f.store.InspectTraceExactV1(f.ctx, original.TenantID, reviewport.ExactV1(original.ID, original.Revision, original.Digest))
	if err != nil || stored.FactRefs[0] == "mutated" {
		t.Fatalf("reader exposed mutable alias: %+v err=%v", stored, err)
	}
	second, err := f.store.ListTracePageV2(f.ctx, reviewport.ListTracePageRequestV2{TenantID: f.caseValue.TenantID, CaseID: f.caseValue.ID, After: first.Next, Limit: 2})
	if err != nil || len(second.Events) == 0 {
		t.Fatalf("second page: %+v err=%v", second, err)
	}
	drift := *first.Next
	drift.Trace.Digest = testkit.Digest("cursor-drift")
	if _, err := f.store.ListTracePageV2(f.ctx, reviewport.ListTracePageRequestV2{TenantID: f.caseValue.TenantID, CaseID: f.caseValue.ID, After: &drift, Limit: 2}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("drifted exact cursor must fail closed, got %v", err)
	}
}

func TestTraceEventV2EscalatedAndResolvedCloseWithDomainMutations(t *testing.T) {
	f := newReviewingFlow(t, contract.RouteHumanV1)
	f.clock.Advance(time.Second)
	attestation := testkit.HumanAttestation(f.clock.Now(), f.caseValue, f.round, f.assignment, contract.ResolutionEscalateHumanV1, "idem-escalate-v2")
	successor := f.caseValue
	successor.Revision++
	escalated := testkit.Trace(f.clock.Now(), successor, contract.TraceEscalatedV1, 91, attestation.ID)
	waiting, _, err := f.engine.RecordAttestationWithTraceV2(f.ctx, reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), attestation, testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceAttestedV1, 90, attestation.ID), []contract.TraceFactV1{escalated})
	if err != nil || waiting.State != contract.CaseWaitingHumanV1 {
		t.Fatalf("escalation closure: case=%+v err=%v", waiting, err)
	}
	if _, err := f.store.InspectTraceExactV1(f.ctx, escalated.TenantID, reviewport.ExactV1(escalated.ID, escalated.Revision, escalated.Digest)); err != nil {
		t.Fatalf("Escalated event missing: %v", err)
	}

	accepted := newReviewingFlow(t, contract.RouteHumanV1)
	accepted.clock.Advance(time.Second)
	att := testkit.HumanAttestation(accepted.clock.Now(), accepted.caseValue, accepted.round, accepted.assignment, contract.ResolutionAcceptV1, "idem-resolve-v2")
	c, _, err := accepted.engine.RecordAttestationV1(accepted.ctx, reviewport.ExpectedV1(accepted.caseValue.Revision, accepted.caseValue.Digest), att, testkit.Trace(accepted.clock.Now(), accepted.caseValue, contract.TraceAttestedV1, 92, att.ID))
	if err != nil {
		t.Fatal(err)
	}
	accepted.clock.Advance(time.Second)
	owner := newVerdictOwner(t, accepted, nil)
	resolvedCase := c
	resolvedCase.Revision++
	resolved := testkit.Trace(accepted.clock.Now(), resolvedCase, contract.TraceResolvedV1, 94, "verdict-trace-v2")
	caseFact, verdict, err := owner.DecideV1(accepted.ctx, verdictowner.DecideCommandV1{TenantID: c.TenantID, CaseID: c.ID, Expected: reviewport.ExpectedV1(c.Revision, c.Digest), AttestationID: att.ID, VerdictID: "verdict-trace-v2", Trace: testkit.Trace(accepted.clock.Now(), c, contract.TraceVerdictV1, 93, "verdict-trace-v2"), AdditionalTraces: []contract.TraceFactV1{resolved}})
	if err != nil || caseFact.State != contract.CaseResolvedV1 || verdict.ID != "verdict-trace-v2" {
		t.Fatalf("resolution closure: case=%+v verdict=%+v err=%v", caseFact, verdict, err)
	}
	if _, err := accepted.store.InspectTraceExactV1(accepted.ctx, resolved.TenantID, reviewport.ExactV1(resolved.ID, resolved.Revision, resolved.Digest)); err != nil {
		t.Fatalf("Resolved event missing: %v", err)
	}
}

func TestTraceEventV2EscalationWithoutEscalatedFailsClosed(t *testing.T) {
	f := newReviewingFlow(t, contract.RouteHumanV1)
	f.clock.Advance(time.Second)
	attestation := testkit.HumanAttestation(f.clock.Now(), f.caseValue, f.round, f.assignment, contract.ResolutionEscalateHumanV1, "idem-escalate-empty")
	primary := testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceAttestedV1, 95, attestation.ID)
	if _, _, err := f.engine.RecordAttestationV1(f.ctx, reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), attestation, primary); !core.HasReason(err, core.ReasonInvalidState) {
		t.Fatalf("eventless escalation did not fail closed: %v", err)
	}
	if _, err := f.store.InspectAttestationExactV1(f.ctx, attestation.TenantID, reviewport.ExactV1(attestation.ID, attestation.Revision, attestation.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("eventless escalation leaked Attestation: %v", err)
	}
	if _, err := f.store.InspectTraceExactV1(f.ctx, primary.TenantID, reviewport.ExactV1(primary.ID, primary.Revision, primary.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("eventless escalation leaked primary Trace: %v", err)
	}
}

func TestTraceEventV2VerdictOwnerDerivesExactlyOneResolved(t *testing.T) {
	f := newReviewingFlow(t, contract.RouteHumanV1)
	f.clock.Advance(time.Second)
	attestation := testkit.HumanAttestation(f.clock.Now(), f.caseValue, f.round, f.assignment, contract.ResolutionAcceptV1, "idem-resolved-derived")
	c, _, err := f.engine.RecordAttestationV1(f.ctx, reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), attestation, testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceAttestedV1, 96, attestation.ID))
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Second)
	owner := newVerdictOwner(t, f, nil)
	command := verdictowner.DecideCommandV1{TenantID: c.TenantID, CaseID: c.ID, Expected: reviewport.ExpectedV1(c.Revision, c.Digest), AttestationID: attestation.ID, VerdictID: "verdict-resolved-derived", Trace: testkit.Trace(f.clock.Now(), c, contract.TraceVerdictV1, 97, "verdict-resolved-derived")}
	caseFact, verdict, err := owner.DecideV1(f.ctx, command)
	if err != nil || caseFact.State != contract.CaseResolvedV1 {
		t.Fatalf("derived resolution failed: case=%+v verdict=%+v err=%v", caseFact, verdict, err)
	}
	events, err := f.store.ListTracePageV2(f.ctx, reviewport.ListTracePageRequestV2{TenantID: c.TenantID, CaseID: c.ID, Limit: reviewport.MaxTracePageV2})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range events.Events {
		if event.Event == contract.TraceResolvedV1 && event.CausationID == verdict.ID {
			count++
			if event.CaseRevision != caseFact.Revision || !containsStringTestV2(event.FactRefs, verdict.ID) {
				t.Fatalf("derived Resolved event drifted: %+v", event)
			}
		}
	}
	if count != 1 {
		t.Fatalf("derived Resolved count=%d, want 1", count)
	}
	if replayCase, replayVerdict, replayErr := owner.DecideV1(f.ctx, command); replayErr != nil || replayCase.Digest != caseFact.Digest || replayVerdict.Digest != verdict.Digest {
		t.Fatalf("derived Resolved replay was not exact: case=%+v verdict=%+v err=%v", replayCase, replayVerdict, replayErr)
	}
}

func containsStringTestV2(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func TestTraceEventV2EscalatedConflictLeavesAttestationAndPrimaryTraceAbsent(t *testing.T) {
	f := newReviewingFlow(t, contract.RouteHumanV1)
	f.clock.Advance(time.Second)
	attestation := testkit.HumanAttestation(f.clock.Now(), f.caseValue, f.round, f.assignment, contract.ResolutionEscalateHumanV1, "idem-escalate-conflict")
	primary := testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceAttestedV1, 102, attestation.ID)
	successor := f.caseValue
	successor.Revision++
	escalated := testkit.Trace(f.clock.Now(), successor, contract.TraceEscalatedV1, 103, attestation.ID)
	occupied := testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceRoutedV1, 103, "occupied")
	occupied.ID = "trace-escalation-occupied"
	occupied.Digest = ""
	occupied, _ = contract.SealTraceFactV1(occupied)
	if _, err := f.store.InjectTraceForTestV1(f.ctx, occupied); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.engine.RecordAttestationWithTraceV2(f.ctx, reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), attestation, primary, []contract.TraceFactV1{escalated}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("escalation Trace conflict must fail closed, got %v", err)
	}
	if _, err := f.store.InspectAttestationExactV1(f.ctx, attestation.TenantID, reviewport.ExactV1(attestation.ID, attestation.Revision, attestation.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("failed escalation leaked Attestation: %v", err)
	}
	for _, event := range []contract.TraceFactV1{primary, escalated} {
		if _, err := f.store.InspectTraceExactV1(f.ctx, event.TenantID, reviewport.ExactV1(event.ID, event.Revision, event.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("failed escalation leaked %s: %v", event.Event, err)
		}
	}
}

func TestTraceEventV2ResolvedConflictLeavesVerdictAndPrimaryTraceAbsent(t *testing.T) {
	f := newReviewingFlow(t, contract.RouteHumanV1)
	f.clock.Advance(time.Second)
	attestation := testkit.HumanAttestation(f.clock.Now(), f.caseValue, f.round, f.assignment, contract.ResolutionAcceptV1, "idem-resolved-conflict")
	c, _, err := f.engine.RecordAttestationV1(f.ctx, reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), attestation, testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceAttestedV1, 104, attestation.ID))
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Second)
	primary := testkit.Trace(f.clock.Now(), c, contract.TraceVerdictV1, 105, "verdict-resolved-conflict")
	successor := c
	successor.Revision++
	resolved := testkit.Trace(f.clock.Now(), successor, contract.TraceResolvedV1, 106, "verdict-resolved-conflict")
	occupied := testkit.Trace(f.clock.Now(), c, contract.TraceRoutedV1, 106, "occupied")
	occupied.ID = "trace-resolution-occupied"
	occupied.Digest = ""
	occupied, _ = contract.SealTraceFactV1(occupied)
	if _, err := f.store.InjectTraceForTestV1(f.ctx, occupied); err != nil {
		t.Fatal(err)
	}
	owner := newVerdictOwner(t, f, nil)
	if _, _, err := owner.DecideV1(f.ctx, verdictowner.DecideCommandV1{TenantID: c.TenantID, CaseID: c.ID, Expected: reviewport.ExpectedV1(c.Revision, c.Digest), AttestationID: attestation.ID, VerdictID: "verdict-resolved-conflict", Trace: primary, AdditionalTraces: []contract.TraceFactV1{resolved}}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("resolved Trace conflict must fail closed, got %v", err)
	}
	if _, err := f.store.InspectVerdictV1(f.ctx, c.TenantID, "verdict-resolved-conflict"); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("failed resolution leaked Verdict: %v", err)
	}
	for _, event := range []contract.TraceFactV1{primary, resolved} {
		if _, err := f.store.InspectTraceExactV1(f.ctx, event.TenantID, reviewport.ExactV1(event.ID, event.Revision, event.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("failed resolution leaked %s: %v", event.Event, err)
		}
	}
}

func TestTraceEventV2DecisionLostReplyRecoversVerdictAndResolvedExactly(t *testing.T) {
	f := newReviewingFlow(t, contract.RouteHumanV1)
	f.clock.Advance(time.Second)
	attestation := testkit.HumanAttestation(f.clock.Now(), f.caseValue, f.round, f.assignment, contract.ResolutionAcceptV1, "idem-resolved-lost-reply")
	c, _, err := f.engine.RecordAttestationV1(f.ctx, reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), attestation, testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceAttestedV1, 107, attestation.ID))
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	wrapper := &loseDecideReplyStoreV2{Store: f.store, cancel: cancel}
	source, err := memory.NewDecisionCurrentSourceV1(wrapper, &testkit.ExternalCurrentReader{}, f.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := verdictowner.New(wrapper, source, f.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	primary := testkit.Trace(f.clock.Now(), c, contract.TraceVerdictV1, 108, "verdict-resolved-lost-reply")
	successor := c
	successor.Revision++
	resolved := testkit.Trace(f.clock.Now(), successor, contract.TraceResolvedV1, 109, "verdict-resolved-lost-reply")
	caseFact, verdict, err := owner.DecideV1(ctx, verdictowner.DecideCommandV1{TenantID: c.TenantID, CaseID: c.ID, Expected: reviewport.ExpectedV1(c.Revision, c.Digest), AttestationID: attestation.ID, VerdictID: "verdict-resolved-lost-reply", Trace: primary, AdditionalTraces: []contract.TraceFactV1{resolved}})
	if err != nil || caseFact.State != contract.CaseResolvedV1 || verdict.ID != "verdict-resolved-lost-reply" {
		t.Fatalf("detached decision recovery failed: case=%+v verdict=%+v err=%v", caseFact, verdict, err)
	}
	if ctx.Err() != context.Canceled || wrapper.calls.Load() != 1 {
		t.Fatalf("decision recovery replayed mutation or reused cancelled context: ctx=%v calls=%d", ctx.Err(), wrapper.calls.Load())
	}
	for _, event := range []contract.TraceFactV1{primary, resolved} {
		if _, err := f.store.InspectTraceExactV1(context.Background(), event.TenantID, reviewport.ExactV1(event.ID, event.Revision, event.Digest)); err != nil {
			t.Fatalf("decision recovery missed exact %s event: %v", event.Event, err)
		}
	}
}

package application_test

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	application "github.com/Proview-China/rax/ExecutionRuntime/application"
	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/application/fakes"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type reviewWaitingInputFakeV1 struct {
	mu     sync.Mutex
	values []contract.ReviewWaitingInputCurrentProjectionV1
	calls  int
	err    error
}

func (f *reviewWaitingInputFakeV1) InspectReviewWaitingInputCurrentV1(_ context.Context, subject contract.ReviewWaitingInputSubjectV1) (contract.ReviewWaitingInputCurrentProjectionV1, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return contract.ReviewWaitingInputCurrentProjectionV1{}, f.err
	}
	if len(f.values) == 0 {
		return contract.ReviewWaitingInputCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "input absent")
	}
	index := f.calls
	if index >= len(f.values) {
		index = len(f.values) - 1
	}
	f.calls++
	value := f.values[index]
	if value.Subject != subject {
		return contract.ReviewWaitingInputCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "input subject drift")
	}
	return value, nil
}

type reviewWaitingReviewFakeV1 struct {
	mu                       sync.Mutex
	current                  *contract.ReviewWaitingCurrentProjectionV1
	startCalls, inspectCalls int
	startErr, inspectErr     error
	cancelOnStart            func()
}

func (f *reviewWaitingReviewFakeV1) StartOrInspectReviewV1(ctx context.Context, request contract.ReviewWaitingRequestV1) (contract.ReviewWaitingCurrentProjectionV1, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return contract.ReviewWaitingCurrentProjectionV1{}, err
	}
	f.startCalls++
	if f.cancelOnStart != nil {
		f.cancelOnStart()
	}
	if f.startErr != nil {
		return contract.ReviewWaitingCurrentProjectionV1{}, f.startErr
	}
	if f.current == nil {
		return contract.ReviewWaitingCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Review Case absent")
	}
	value := f.current.Clone()
	if err := value.ValidateFor(request, time.Unix(0, value.CheckedUnixNano)); err != nil {
		return contract.ReviewWaitingCurrentProjectionV1{}, err
	}
	return value, nil
}
func (f *reviewWaitingReviewFakeV1) InspectReviewV1(ctx context.Context, request contract.ReviewWaitingInspectRequestV1) (contract.ReviewWaitingCurrentProjectionV1, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return contract.ReviewWaitingCurrentProjectionV1{}, err
	}
	f.inspectCalls++
	if f.inspectErr != nil {
		return contract.ReviewWaitingCurrentProjectionV1{}, f.inspectErr
	}
	if f.current == nil {
		return contract.ReviewWaitingCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Review Case absent")
	}
	value := f.current.Clone()
	if value.RequestID != request.Request.ID || value.RequestDigest != request.Request.Digest || value.Case.Target != request.Target {
		return contract.ReviewWaitingCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review Inspect drift")
	}
	return value, nil
}
func (f *reviewWaitingReviewFakeV1) set(value contract.ReviewWaitingCurrentProjectionV1) {
	f.mu.Lock()
	copy := value.Clone()
	f.current = &copy
	f.mu.Unlock()
}
func (f *reviewWaitingReviewFakeV1) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.startCalls, f.inspectCalls
}

type reviewWaitingIdempotentClaimPortV1 struct {
	applicationports.ReviewWaitingCoordinationFactPortV1
}

func (p reviewWaitingIdempotentClaimPortV1) CompareAndSwapReviewWaitingCoordinationV1(ctx context.Context, request applicationports.ReviewWaitingCoordinationCASRequestV1) (applicationports.ReviewWaitingCoordinationCASReceiptV1, error) {
	receipt, err := p.ReviewWaitingCoordinationFactPortV1.CompareAndSwapReviewWaitingCoordinationV1(ctx, request)
	if err == nil && request.Next.State == contract.ReviewInspectStateV1 {
		receipt.Applied = false
	}
	return receipt, err
}

func TestReviewWaitingContractNeutralDeliveryMinTTLAndHardNegativesV1(t *testing.T) {
	fixture := newReviewWaitingFixtureV1(t)
	if fixture.request.Delivery != contract.ReviewWaitingInlineV1 {
		t.Fatal("delivery drift")
	}
	detached := fixture.request
	detached.Delivery = contract.ReviewWaitingDetachedV1
	detached, err := contract.SealReviewWaitingRequestV1(detached)
	if err != nil || detached.ID == fixture.request.ID {
		t.Fatalf("Delivery not independently sealed: %v", err)
	}
	coordination, err := contract.NewReviewWaitingCoordinationFactV1(fixture.request, fixture.now)
	if err != nil || coordination.State != contract.ReviewWaitingStateV1 {
		t.Fatalf("write-ahead Fact failed: %+v %v", coordination, err)
	}
	claimed, err := contract.ClaimReviewWaitingStartV1(coordination, "review-start-claim/test", fixture.now.Add(time.Second))
	if err != nil || claimed.State != contract.ReviewInspectStateV1 {
		t.Fatal(err)
	}
	receipt, err := contract.SealReviewPhaseReceiptV1(contract.ReviewPhaseReceiptV1{Coordination: claimed.RefV1()}, fixture.request, fixture.allow, fixture.input, fixture.now.Add(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	wantExpiry := fixture.allow.ExpiresUnixNano
	if fixture.input.ExpiresUnixNano < wantExpiry {
		wantExpiry = fixture.input.ExpiresUnixNano
	}
	if receipt.ExpiresUnixNano != wantExpiry {
		t.Fatalf("receipt min TTL=%d want=%d", receipt.ExpiresUnixNano, wantExpiry)
	}
	t.Run("receipt-only-clock-rollback", func(t *testing.T) {
		rollback := time.Unix(0, receipt.CheckedUnixNano).Add(-time.Nanosecond)
		if err := receipt.ValidateCurrentFor(fixture.request, rollback); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("Receipt clock rollback was not classified as clock regression: %v", err)
		}
	})
	outcome := contract.ReviewWaitingOutcomeV1{Coordination: claimed.RefV1(), Review: fixture.allow.Clone(), Receipt: &receipt}
	if err := outcome.ValidateFor(fixture.request, fixture.now.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	t.Run("receipt-verdict-value-drift", func(t *testing.T) {
		drifted := outcome.Clone()
		verdict := *drifted.Receipt.Verdict
		verdict.ID += "-drifted"
		verdict.Digest = core.DigestBytes([]byte("receipt-verdict-value-drift"))
		drifted.Receipt.Verdict = &verdict
		drifted.Receipt.Digest, err = drifted.Receipt.DigestV1()
		if err != nil {
			t.Fatal(err)
		}
		if err := drifted.Receipt.ValidateCurrentFor(fixture.request, fixture.now.Add(5*time.Second)); err != nil {
			t.Fatalf("counterexample Receipt must be independently valid: %v", err)
		}
		if err := drifted.ValidateFor(fixture.request, fixture.now.Add(5*time.Second)); !core.HasReason(err, core.ReasonReviewVerdictStale) {
			t.Fatalf("Receipt/current Verdict drift accepted: %v", err)
		}
	})
	t.Run("receipt-verdict-nil-drift", func(t *testing.T) {
		current := fixture.allow.Clone()
		current.Decision = contract.ReviewPhaseDenyV1
		current, err = contract.SealReviewWaitingCurrentProjectionV1(current)
		if err != nil {
			t.Fatal(err)
		}
		deniedReceipt, err := contract.SealReviewPhaseReceiptV1(contract.ReviewPhaseReceiptV1{Coordination: claimed.RefV1()}, fixture.request, current, fixture.input, fixture.now.Add(5*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		deniedReceipt.Verdict = nil
		deniedReceipt.Digest, err = deniedReceipt.DigestV1()
		if err != nil {
			t.Fatal(err)
		}
		if err := deniedReceipt.ValidateCurrentFor(fixture.request, fixture.now.Add(5*time.Second)); err != nil {
			t.Fatalf("counterexample Receipt must be independently valid: %v", err)
		}
		drifted := contract.ReviewWaitingOutcomeV1{Coordination: claimed.RefV1(), Review: current, Receipt: &deniedReceipt}
		if err := drifted.ValidateFor(fixture.request, fixture.now.Add(5*time.Second)); !core.HasReason(err, core.ReasonReviewVerdictStale) {
			t.Fatalf("Receipt/current Verdict nil drift accepted: %v", err)
		}
	})
	bad := fixture.allow.Clone()
	bad.Case.Target.Revision++
	bad, _ = contract.SealReviewWaitingCurrentProjectionV1(bad)
	if bad.ValidateFor(fixture.request, fixture.now.Add(5*time.Second)) == nil {
		t.Fatal("cross Target current accepted")
	}
	bad = fixture.allow.Clone()
	bad.Verdict.CaseRevision = bad.Case.Revision
	bad, _ = contract.SealReviewWaitingCurrentProjectionV1(bad)
	if bad.Validate() == nil {
		t.Fatal("non-atomic Case/Verdict revision accepted")
	}
	bad = fixture.allow.Clone()
	bad.Decision = contract.ReviewPhaseDeferV1
	bad, _ = contract.SealReviewWaitingCurrentProjectionV1(bad)
	if bad.Validate() == nil {
		t.Fatal("defer with Verdict accepted")
	}
}

func TestReviewWaitingCoordinatorWriteAheadResumeAndHardStopV1(t *testing.T) {
	fixture := newReviewWaitingFixtureV1(t)
	clockNow := fixture.now.Add(5 * time.Second)
	store := fakes.NewReviewWaitingStoreV1(func() time.Time { return clockNow })
	review := &reviewWaitingReviewFakeV1{}
	review.set(fixture.pending)
	coordinator := newReviewWaitingCoordinatorV1(t, store, &reviewWaitingInputFakeV1{values: []contract.ReviewWaitingInputCurrentProjectionV1{fixture.input}}, review, func() time.Time { return clockNow }, "claim/main")
	outcome, err := coordinator.CoordinateReviewWaitingV1(context.Background(), fixture.request)
	if err != nil || outcome.Review.Decision != contract.ReviewPhaseDeferV1 || outcome.Receipt != nil {
		t.Fatalf("initial wait failed: %+v %v", outcome, err)
	}
	start, _ := review.counts()
	if start != 1 {
		t.Fatalf("Start calls=%d", start)
	}
	current, err := store.InspectCurrentReviewWaitingCoordinationV1(context.Background(), fixture.request.ExecutionScope, fixture.request.ID)
	if err != nil || current.State != contract.ReviewInspectStateV1 || current.Case == nil {
		t.Fatalf("waiting state not durable: %+v %v", current, err)
	}
	review.set(fixture.allow)
	outcome, err = coordinator.CoordinateReviewWaitingV1(context.Background(), fixture.request)
	if err != nil || outcome.Receipt == nil || outcome.Receipt.Decision != contract.ReviewPhaseAllowV1 {
		t.Fatalf("resume completion failed: %+v %v", outcome, err)
	}
	start, _ = review.counts()
	if start != 1 {
		t.Fatalf("resume restarted Review: %d", start)
	}
	current, _ = store.InspectCurrentReviewWaitingCoordinationV1(context.Background(), fixture.request.ExecutionScope, fixture.request.ID)
	if current.State != contract.ReviewCompletedStateV1 {
		t.Fatalf("completion not durable: %+v", current)
	}
	if _, err := coordinator.CoordinateReviewWaitingV1(context.Background(), fixture.request); err != nil {
		t.Fatalf("completed recovery failed: %v", err)
	}
	start, _ = review.counts()
	if start != 1 {
		t.Fatalf("completed recovery restarted Review: %d", start)
	}
}

func TestReviewWaitingCoordinatorLostRepliesAreInspectOnlyV1(t *testing.T) {
	fixture := newReviewWaitingFixtureV1(t)
	clockNow := fixture.now.Add(5 * time.Second)
	store := fakes.NewReviewWaitingStoreV1(func() time.Time { return clockNow })
	store.LoseNextCreateReplyV1()
	store.LoseNextCASReplyV1()
	review := &reviewWaitingReviewFakeV1{}
	review.set(fixture.pending)
	coordinator := newReviewWaitingCoordinatorV1(t, store, &reviewWaitingInputFakeV1{values: []contract.ReviewWaitingInputCurrentProjectionV1{fixture.input}}, review, func() time.Time { return clockNow }, "claim/lost")
	if _, err := coordinator.CoordinateReviewWaitingV1(context.Background(), fixture.request); err != nil {
		t.Fatalf("lost mutation recovery failed: %v", err)
	}
	start, _ := review.counts()
	if start != 0 {
		t.Fatalf("lost claim reply allowed Start: %d", start)
	}

	fixture2 := newReviewWaitingFixtureV1(t)
	fixture2.request.Phase.ID = "phase-lost-start"
	fixture2.request, _ = contract.SealReviewWaitingRequestV1(fixture2.request)
	fixture2 = resealReviewWaitingFixtureV1(t, fixture2)
	store2 := fakes.NewReviewWaitingStoreV1(func() time.Time { return clockNow })
	ctx, cancel := context.WithCancel(context.Background())
	review2 := &reviewWaitingReviewFakeV1{
		startErr:      core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost Review reply"),
		cancelOnStart: cancel,
	}
	review2.set(fixture2.pending)
	coordinator2 := newReviewWaitingCoordinatorV1(t, store2, &reviewWaitingInputFakeV1{values: []contract.ReviewWaitingInputCurrentProjectionV1{fixture2.input}}, review2, func() time.Time { return clockNow }, "claim/start-lost")
	if _, err := coordinator2.CoordinateReviewWaitingV1(ctx, fixture2.request); err != nil {
		t.Fatalf("Review lost reply exact Inspect failed: %v", err)
	}
	start, _ = review2.counts()
	if start != 1 {
		t.Fatalf("lost Start was replayed: %d", start)
	}
}

func TestReviewWaitingCoordinatorAuthoritativeNotFoundBeforeBoundaryOnlyV1(t *testing.T) {
	fixture := newReviewWaitingFixtureV1(t)
	clockNow := fixture.now.Add(5 * time.Second)
	store := fakes.NewReviewWaitingStoreV1(func() time.Time { return clockNow })
	store.FailNextCreateBeforeCommitV1(core.ErrorIndeterminate)
	review := &reviewWaitingReviewFakeV1{}
	review.set(fixture.pending)
	coordinator := newReviewWaitingCoordinatorV1(t, store, &reviewWaitingInputFakeV1{values: []contract.ReviewWaitingInputCurrentProjectionV1{fixture.input}}, review, func() time.Time { return clockNow }, "claim/create-retry")
	if _, err := coordinator.CoordinateReviewWaitingV1(context.Background(), fixture.request); err != nil {
		t.Fatalf("authoritative pre-boundary NotFound did not permit same-canonical create retry: %v", err)
	}
	start, _ := review.counts()
	if start != 1 {
		t.Fatalf("pre-boundary recovery Start calls=%d", start)
	}

	fixture2 := newReviewWaitingFixtureV1(t)
	fixture2.request.Phase.ID = "phase-post-boundary-not-found"
	fixture2.request, _ = contract.SealReviewWaitingRequestV1(fixture2.request)
	fixture2 = resealReviewWaitingFixtureV1(t, fixture2)
	store2 := fakes.NewReviewWaitingStoreV1(func() time.Time { return clockNow })
	review2 := &reviewWaitingReviewFakeV1{startErr: core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "unknown Review start")}
	coordinator2 := newReviewWaitingCoordinatorV1(t, store2, &reviewWaitingInputFakeV1{values: []contract.ReviewWaitingInputCurrentProjectionV1{fixture2.input}}, review2, func() time.Time { return clockNow }, "claim/post-boundary")
	if _, err := coordinator2.CoordinateReviewWaitingV1(context.Background(), fixture2.request); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("post-boundary NotFound error degraded: %v", err)
	}
	if _, err := coordinator2.CoordinateReviewWaitingV1(context.Background(), fixture2.request); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("post-boundary recovery did not remain Inspect-only: %v", err)
	}
	start, _ = review2.counts()
	if start != 1 {
		t.Fatalf("post-boundary NotFound replayed Start: %d", start)
	}
}

type reviewWaitingReviewDriftV1 struct {
	mu     sync.Mutex
	values []contract.ReviewWaitingCurrentProjectionV1
	calls  int
	starts int
}

func (f *reviewWaitingReviewDriftV1) StartOrInspectReviewV1(context.Context, contract.ReviewWaitingRequestV1) (contract.ReviewWaitingCurrentProjectionV1, error) {
	f.mu.Lock()
	f.starts++
	value := f.values[0].Clone()
	f.mu.Unlock()
	return value, nil
}
func (f *reviewWaitingReviewDriftV1) InspectReviewV1(context.Context, contract.ReviewWaitingInspectRequestV1) (contract.ReviewWaitingCurrentProjectionV1, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	index := f.calls + 1
	if index >= len(f.values) {
		index = len(f.values) - 1
	}
	f.calls++
	return f.values[index].Clone(), nil
}

func TestReviewWaitingCoordinatorReviewS1S2AndTTLCrossingClosedV1(t *testing.T) {
	fixture := newReviewWaitingFixtureV1(t)
	clockNow := fixture.now.Add(5 * time.Second)
	store := fakes.NewReviewWaitingStoreV1(func() time.Time { return clockNow })
	review := &reviewWaitingReviewDriftV1{values: []contract.ReviewWaitingCurrentProjectionV1{fixture.pending, fixture.allow}}
	coordinator := newReviewWaitingCoordinatorV1(t, store, &reviewWaitingInputFakeV1{values: []contract.ReviewWaitingInputCurrentProjectionV1{fixture.input}}, review, func() time.Time { return clockNow }, "claim/review-drift")
	if _, err := coordinator.CoordinateReviewWaitingV1(context.Background(), fixture.request); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("Review S1/S2 drift accepted: %v", err)
	}
	current, err := store.InspectCurrentReviewWaitingCoordinationV1(context.Background(), fixture.request.ExecutionScope, fixture.request.ID)
	if err != nil || current.State != contract.ReviewInspectStateV1 || current.Receipt != nil {
		t.Fatalf("Review drift wrote terminal receipt: %+v %v", current, err)
	}

	fixture2 := newReviewWaitingFixtureV1(t)
	fixture2.request.Phase.ID = "phase-ttl-cross"
	fixture2.request, _ = contract.SealReviewWaitingRequestV1(fixture2.request)
	fixture2 = resealReviewWaitingFixtureV1(t, fixture2)
	expiring := fixture2.pending
	expiring.ExpiresUnixNano = fixture2.now.Add(6 * time.Second).UnixNano()
	expiring, _ = contract.SealReviewWaitingCurrentProjectionV1(expiring)
	store2 := fakes.NewReviewWaitingStoreV1(func() time.Time { return fixture2.now.Add(7 * time.Second) })
	review2 := &reviewWaitingReviewFakeV1{}
	review2.set(expiring)
	coordinator2 := newReviewWaitingCoordinatorV1(t, store2, &reviewWaitingInputFakeV1{values: []contract.ReviewWaitingInputCurrentProjectionV1{fixture2.input}}, review2, func() time.Time { return fixture2.now.Add(7 * time.Second) }, "claim/ttl-cross")
	if _, err := coordinator2.CoordinateReviewWaitingV1(context.Background(), fixture2.request); !core.HasReason(err, core.ReasonReviewVerdictStale) {
		t.Fatalf("Review TTL crossing accepted: %v", err)
	}
	current, err = store2.InspectCurrentReviewWaitingCoordinationV1(context.Background(), fixture2.request.ExecutionScope, fixture2.request.ID)
	if err != nil || current.State != contract.ReviewInspectStateV1 || current.Receipt != nil {
		t.Fatalf("TTL crossing wrote terminal receipt: %+v %v", current, err)
	}
}

func TestReviewWaitingCoordinatorIdempotentNonOwnerCannotStartV1(t *testing.T) {
	fixture := newReviewWaitingFixtureV1(t)
	clockNow := fixture.now.Add(5 * time.Second)
	base := fakes.NewReviewWaitingStoreV1(func() time.Time { return clockNow })
	review := &reviewWaitingReviewFakeV1{}
	review.set(fixture.pending)
	port := reviewWaitingIdempotentClaimPortV1{ReviewWaitingCoordinationFactPortV1: base}
	coordinator := newReviewWaitingCoordinatorV1(t, port, &reviewWaitingInputFakeV1{values: []contract.ReviewWaitingInputCurrentProjectionV1{fixture.input}}, review, func() time.Time { return clockNow }, "claim/idempotent")
	if _, err := coordinator.CoordinateReviewWaitingV1(context.Background(), fixture.request); err != nil {
		t.Fatal(err)
	}
	start, _ := review.counts()
	if start != 0 {
		t.Fatalf("Applied=false caller started Review: %d", start)
	}
}

func TestReviewWaitingCoordinatorTargetDriftSupersedesWithoutReviewMutationV1(t *testing.T) {
	fixture := newReviewWaitingFixtureV1(t)
	clockNow := fixture.now.Add(5 * time.Second)
	drift := fixture.input
	drift.Target.Revision++
	drift.Target.Digest = reviewWaitingDigestV1(t, "target-drift")
	drift, _ = contract.SealReviewWaitingInputCurrentProjectionV1(drift)
	store := fakes.NewReviewWaitingStoreV1(func() time.Time { return clockNow })
	review := &reviewWaitingReviewFakeV1{}
	review.set(fixture.pending)
	coordinator := newReviewWaitingCoordinatorV1(t, store, &reviewWaitingInputFakeV1{values: []contract.ReviewWaitingInputCurrentProjectionV1{fixture.input, drift}}, review, func() time.Time { return clockNow }, "claim/drift")
	if _, err := coordinator.CoordinateReviewWaitingV1(context.Background(), fixture.request); !core.HasReason(err, core.ReasonReviewCandidateConflict) {
		t.Fatalf("Target drift not closed: %v", err)
	}
	current, err := store.InspectCurrentReviewWaitingCoordinationV1(context.Background(), fixture.request.ExecutionScope, fixture.request.ID)
	if err != nil || current.State != contract.ReviewSupersededStateV1 {
		t.Fatalf("Target drift not persisted superseded: %+v %v", current, err)
	}
	start, _ := review.counts()
	if start != 0 {
		t.Fatalf("Target drift called Review: %d", start)
	}
}

func TestReviewWaitingCoordinator64IndependentCoordinatorsOneStartV1(t *testing.T) {
	fixture := newReviewWaitingFixtureV1(t)
	clockNow := fixture.now.Add(5 * time.Second)
	store := fakes.NewReviewWaitingStoreV1(func() time.Time { return clockNow })
	review := &reviewWaitingReviewFakeV1{}
	review.set(fixture.pending)
	const workers = 64
	var claims atomic.Uint64
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			coordinator, err := application.NewReviewWaitingCoordinatorV1(application.ReviewWaitingCoordinatorConfigV1{Facts: store, Inputs: &reviewWaitingInputFakeV1{values: []contract.ReviewWaitingInputCurrentProjectionV1{fixture.input}}, Review: review, Clock: func() time.Time { return clockNow }, ClaimID: func() (string, error) {
				return "claim/concurrent/" + time.Unix(0, int64(claims.Add(1))).Format("150405.000000000"), nil
			}})
			if err == nil {
				_, err = coordinator.CoordinateReviewWaitingV1(context.Background(), fixture.request)
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent coordinator failed: %v", err)
		}
	}
	start, _ := review.counts()
	if start != 1 {
		t.Fatalf("logical Review Start calls=%d want=1", start)
	}
}

func TestReviewWaitingCoordinatorClockRollbackAndTypedNilV1(t *testing.T) {
	fixture := newReviewWaitingFixtureV1(t)
	store := fakes.NewReviewWaitingStoreV1(func() time.Time { return fixture.now.Add(5 * time.Second) })
	review := &reviewWaitingReviewFakeV1{}
	review.set(fixture.pending)
	times := []time.Time{fixture.now.Add(5 * time.Second), fixture.now.Add(6 * time.Second), fixture.now.Add(4 * time.Second), fixture.now.Add(7 * time.Second), fixture.now.Add(8 * time.Second)}
	clock := func() time.Time {
		value := times[0]
		if len(times) > 1 {
			times = times[1:]
		}
		return value
	}
	coordinator := newReviewWaitingCoordinatorV1(t, store, &reviewWaitingInputFakeV1{values: []contract.ReviewWaitingInputCurrentProjectionV1{fixture.input}}, review, clock, "claim/rollback")
	if _, err := coordinator.CoordinateReviewWaitingV1(context.Background(), fixture.request); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock rollback accepted: %v", err)
	}
	start, _ := review.counts()
	if start != 0 {
		t.Fatalf("rollback crossed Review boundary: %d", start)
	}
	var typedNil *fakes.ReviewWaitingStoreV1
	if _, err := application.NewReviewWaitingCoordinatorV1(application.ReviewWaitingCoordinatorConfigV1{Facts: typedNil, Inputs: &reviewWaitingInputFakeV1{}, Review: review}); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed nil accepted: %v", err)
	}
}

type reviewWaitingFixtureV1 struct {
	now            time.Time
	request        contract.ReviewWaitingRequestV1
	input          contract.ReviewWaitingInputCurrentProjectionV1
	pending, allow contract.ReviewWaitingCurrentProjectionV1
}

func newReviewWaitingFixtureV1(t *testing.T) reviewWaitingFixtureV1 {
	t.Helper()
	now := time.Unix(2_900_000_000, 0)
	tenant := core.TenantID("tenant-review-wait")
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: tenant, ID: "agent", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage", PlanDigest: reviewWaitingDigestV1(t, "plan")}, Instance: core.InstanceRef{ID: "instance", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "sandbox", Epoch: 1}, AuthorityEpoch: 1}
	scopeDigest, _ := runtimeports.ExecutionScopeDigestV2(scope)
	target := contract.ReviewWaitingTargetCoordinateV1{TenantID: tenant, ID: "target-a", Revision: 1, Digest: reviewWaitingDigestV1(t, "target"), RunID: "run-a", CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(50 * time.Minute).UnixNano()}
	request, err := contract.SealReviewWaitingRequestV1(contract.ReviewWaitingRequestV1{Delivery: contract.ReviewWaitingInlineV1, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, Phase: contract.ReviewPhasePointCoordinateV1{Kind: contract.ReviewPhaseActionV1, ID: "phase-a", Revision: 1, Digest: reviewWaitingDigestV1(t, "phase"), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}, Target: target, ReviewRequest: contract.ReviewRequestCoordinateV1{TenantID: tenant, ID: "review-request-a", Revision: 1, Digest: reviewWaitingDigestV1(t, "review-request"), CaseID: "case-a"}, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(40 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	input, err := contract.SealReviewWaitingInputCurrentProjectionV1(contract.ReviewWaitingInputCurrentProjectionV1{Subject: request.InputSubjectV1(), Phase: request.Phase, Target: target, ExecutionScopeDigest: scopeDigest, CheckedUnixNano: now.Add(time.Second).UnixNano(), ExpiresUnixNano: now.Add(30 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	deferProjection, err := contract.SealReviewWaitingCurrentProjectionV1(contract.ReviewWaitingCurrentProjectionV1{RequestID: request.ReviewRequest.ID, RequestDigest: request.ReviewRequest.Digest, Case: contract.ReviewWaitingCaseCoordinateV1{TenantID: tenant, ID: "case-a", Revision: 1, Digest: reviewWaitingDigestV1(t, "case-1"), Target: target, ExpiresUnixNano: now.Add(25 * time.Minute).UnixNano()}, Decision: contract.ReviewPhaseDeferV1, Current: true, CheckedUnixNano: now.Add(2 * time.Second).UnixNano(), ExpiresUnixNano: now.Add(20 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	verdict := contract.ReviewWaitingVerdictCoordinateV1{TenantID: tenant, ID: "verdict-a", Revision: 1, Digest: reviewWaitingDigestV1(t, "verdict"), CaseID: "case-a", CaseRevision: 1, CaseDigest: deferProjection.Case.Digest, Target: target, ExpiresUnixNano: now.Add(18 * time.Minute).UnixNano()}
	allow, err := contract.SealReviewWaitingCurrentProjectionV1(contract.ReviewWaitingCurrentProjectionV1{RequestID: request.ReviewRequest.ID, RequestDigest: request.ReviewRequest.Digest, Case: contract.ReviewWaitingCaseCoordinateV1{TenantID: tenant, ID: "case-a", Revision: 2, Digest: reviewWaitingDigestV1(t, "case-2"), Target: target, ExpiresUnixNano: now.Add(18 * time.Minute).UnixNano()}, Verdict: &verdict, Decision: contract.ReviewPhaseAllowV1, Current: true, CheckedUnixNano: now.Add(3 * time.Second).UnixNano(), ExpiresUnixNano: now.Add(15 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return reviewWaitingFixtureV1{now: now, request: request, input: input, pending: deferProjection, allow: allow}
}

func resealReviewWaitingFixtureV1(t *testing.T, fixture reviewWaitingFixtureV1) reviewWaitingFixtureV1 {
	t.Helper()
	fixture.input.Subject = fixture.request.InputSubjectV1()
	fixture.input.Phase = fixture.request.Phase
	fixture.input.Target = fixture.request.Target
	fixture.input, _ = contract.SealReviewWaitingInputCurrentProjectionV1(fixture.input)
	for _, p := range []*contract.ReviewWaitingCurrentProjectionV1{&fixture.pending, &fixture.allow} {
		p.RequestID = fixture.request.ReviewRequest.ID
		p.RequestDigest = fixture.request.ReviewRequest.Digest
		p.Case.Target = fixture.request.Target
		if p.Verdict != nil {
			p.Verdict.Target = fixture.request.Target
		}
		*p, _ = contract.SealReviewWaitingCurrentProjectionV1(*p)
	}
	return fixture
}

func newReviewWaitingCoordinatorV1(t *testing.T, facts applicationports.ReviewWaitingCoordinationFactPortV1, inputs applicationports.ReviewWaitingInputCurrentReaderV1, review applicationports.ReviewStartOrInspectPortV1, clock func() time.Time, claim string) *application.ReviewWaitingCoordinatorV1 {
	t.Helper()
	c, err := application.NewReviewWaitingCoordinatorV1(application.ReviewWaitingCoordinatorConfigV1{Facts: facts, Inputs: inputs, Review: review, Clock: clock, ClaimID: func() (string, error) { return claim, nil }})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func reviewWaitingDigestV1(t *testing.T, value string) core.Digest {
	t.Helper()
	d, err := core.CanonicalJSONDigest("application-review-waiting-test", "v1", "value", value)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

var _ = reflect.DeepEqual

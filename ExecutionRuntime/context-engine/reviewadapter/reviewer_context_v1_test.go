package reviewadapter

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/reviewcontextstore"
	reviewcontract "github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type repositoryFaultV1 struct {
	reviewcontextstore.RepositoryV1
	mu                 sync.Mutex
	commitCalls        int
	historicalCalls    int
	commitBeforeErr    error
	commitAfterErr     error
	cancelAfterCommit  context.CancelFunc
	historicalOverride *reviewcontract.ReviewerContextEnvelopeV1
}

func (r *repositoryFaultV1) CommitV1(ctx context.Context, request reviewport.ReviewerContextPublishRequestV1) (reviewport.ReviewerContextPublishReceiptV1, error) {
	r.mu.Lock()
	r.commitCalls++
	before, after, cancel := r.commitBeforeErr, r.commitAfterErr, r.cancelAfterCommit
	r.mu.Unlock()
	if before != nil {
		return reviewport.ReviewerContextPublishReceiptV1{}, before
	}
	receipt, err := r.RepositoryV1.CommitV1(ctx, request)
	if err != nil {
		return receipt, err
	}
	if cancel != nil {
		cancel()
	}
	if after != nil {
		return reviewport.ReviewerContextPublishReceiptV1{}, after
	}
	return receipt, nil
}

func (r *repositoryFaultV1) InspectHistoricalV1(ctx context.Context, exact reviewcontract.ReviewerContextEnvelopeRefV1) (reviewcontract.ReviewerContextEnvelopeV1, error) {
	r.mu.Lock()
	r.historicalCalls++
	override := r.historicalOverride
	r.mu.Unlock()
	if override != nil {
		return override.Clone(), nil
	}
	return r.RepositoryV1.InspectHistoricalV1(ctx, exact)
}

func (r *repositoryFaultV1) countsV1() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.commitCalls, r.historicalCalls
}

type readErrorRepositoryV1 struct {
	reviewcontextstore.RepositoryV1
	err error
}

func (r readErrorRepositoryV1) ResolveV1(context.Context, reviewcontract.ReviewerContextSubjectV1) (reviewcontract.ReviewerContextEnvelopeRefV1, error) {
	return reviewcontract.ReviewerContextEnvelopeRefV1{}, r.err
}

func adapterFixtureV1(t testing.TB, suffix string, clock ClockV1) (*ReviewerContextAdapterV1, *reviewcontextstore.MemoryV1, reviewcontract.ReviewerContextEnvelopeV1) {
	t.Helper()
	now := time.Unix(1_931_000_000, 0)
	value, err := testfixture.ReviewerContextEnvelopeV1(testfixture.ReviewerContextSubjectV1(suffix), 1, now, now.Add(time.Hour), suffix)
	if err != nil {
		t.Fatal(err)
	}
	store := reviewcontextstore.NewMemoryV1()
	if clock == nil {
		clock = func() time.Time { return now.Add(time.Minute) }
	}
	adapter, err := NewReviewerContextAdapterV1(store, clock)
	if err != nil {
		t.Fatal(err)
	}
	return adapter, store, value
}

func TestReviewerContextAdapterCurrentUsesFreshClockWithoutResealingV1(t *testing.T) {
	now := time.Unix(1_931_000_000, 0)
	clock := testfixture.NewSequenceClockV1(now.Add(time.Second), now.Add(2*time.Second), now.Add(3*time.Second), now.Add(4*time.Second))
	adapter, _, value := adapterFixtureV1(t, "fresh", clock.Now)
	if _, err := adapter.PublishReviewerContextV1(context.Background(), reviewport.ReviewerContextPublishRequestV1{Value: value}); err != nil {
		t.Fatal(err)
	}
	ref, err := adapter.ResolveCurrentReviewerContextV1(context.Background(), reviewport.ReviewerContextCurrentResolveRequestV1{Subject: value.Subject})
	if err != nil || ref != value.Ref {
		t.Fatalf("resolve current: %#v %v", ref, err)
	}
	inspected, err := adapter.InspectCurrentReviewerContextV1(context.Background(), value.Subject, value.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if inspected.CheckedUnixNano != value.CheckedUnixNano || inspected.ExpiresUnixNano != value.ExpiresUnixNano || inspected.ProjectionDigest != value.ProjectionDigest {
		t.Fatal("current inspect re-sealed immutable Checked/Expires/Digest")
	}
	inspected.Materials[0].Content = "alias"
	again, err := adapter.InspectHistoricalReviewerContextV1(context.Background(), value.Ref)
	if err != nil || again.Materials[0].Content == "alias" {
		t.Fatal("adapter leaked a mutable alias")
	}
}

func TestReviewerContextAdapterTTLAndRollbackFailClosedV1(t *testing.T) {
	now := time.Unix(1_931_000_000, 0)
	for _, tc := range []struct {
		name   string
		clocks []time.Time
		reason core.ReasonCode
	}{
		{name: "ttl-crossing", clocks: []time.Time{now.Add(59 * time.Minute), now.Add(time.Hour)}, reason: core.ReasonReviewVerdictStale},
		{name: "rollback", clocks: []time.Time{now.Add(time.Minute), now.Add(30 * time.Second)}, reason: core.ReasonClockRegression},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clock := testfixture.NewSequenceClockV1(tc.clocks...)
			adapter, store, value := adapterFixtureV1(t, tc.name, clock.Now)
			if _, err := store.CommitV1(context.Background(), reviewport.ReviewerContextPublishRequestV1{Value: value}); err != nil {
				t.Fatal(err)
			}
			if got, err := adapter.InspectCurrentReviewerContextV1(context.Background(), value.Subject, value.Ref); !core.HasReason(err, tc.reason) || got.Ref.ID != "" {
				t.Fatalf("current actual-point failure did not return zero: %#v %v", got, err)
			}
		})
	}
}

func TestReviewerContextAdapterLostPublishReplyIsExactInspectOnlyV1(t *testing.T) {
	now := time.Unix(1_931_000_000, 0)
	value, err := testfixture.ReviewerContextEnvelopeV1(testfixture.ReviewerContextSubjectV1("lost"), 1, now, now.Add(time.Hour), "lost")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	fault := &repositoryFaultV1{
		RepositoryV1:      reviewcontextstore.NewMemoryV1(),
		commitAfterErr:    context.Canceled,
		cancelAfterCommit: cancel,
	}
	adapter, err := NewReviewerContextAdapterV1(fault, func() time.Time { return now.Add(time.Minute) })
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := adapter.PublishReviewerContextV1(ctx, reviewport.ReviewerContextPublishRequestV1{Value: value})
	if err != nil || receipt.Ref != value.Ref || !receipt.Created {
		t.Fatalf("lost reply exact recovery: %#v %v", receipt, err)
	}
	commits, inspections := fault.countsV1()
	if commits != 1 || inspections != 1 {
		t.Fatalf("lost reply retried mutation or skipped exact inspect: commit=%d inspect=%d", commits, inspections)
	}
}

func TestReviewerContextAdapterUncommittedUnknownDoesNotRepublishV1(t *testing.T) {
	now := time.Unix(1_931_000_000, 0)
	value, err := testfixture.ReviewerContextEnvelopeV1(testfixture.ReviewerContextSubjectV1("unknown"), 1, now, now.Add(time.Hour), "unknown")
	if err != nil {
		t.Fatal(err)
	}
	fault := &repositoryFaultV1{RepositoryV1: reviewcontextstore.NewMemoryV1(), commitBeforeErr: errors.New("opaque transport failure")}
	adapter, err := NewReviewerContextAdapterV1(fault, func() time.Time { return now.Add(time.Minute) })
	if err != nil {
		t.Fatal(err)
	}
	if receipt, err := adapter.PublishReviewerContextV1(context.Background(), reviewport.ReviewerContextPublishRequestV1{Value: value}); !core.HasCategory(err, core.ErrorIndeterminate) || receipt.Ref.ID != "" {
		t.Fatalf("uncommitted unknown was not fail closed: %#v %v", receipt, err)
	}
	commits, inspections := fault.countsV1()
	if commits != 1 || inspections != 1 {
		t.Fatalf("unknown outcome retried mutation: commit=%d inspect=%d", commits, inspections)
	}
}

func TestReviewerContextAdapterLostReplyCanonicalDriftConflictsV1(t *testing.T) {
	now := time.Unix(1_931_000_000, 0)
	value, err := testfixture.ReviewerContextEnvelopeV1(testfixture.ReviewerContextSubjectV1("drift"), 1, now, now.Add(time.Hour), "drift")
	if err != nil {
		t.Fatal(err)
	}
	drift := value.Clone()
	drift.Materials[0].Content = "different"
	fault := &repositoryFaultV1{
		RepositoryV1:       reviewcontextstore.NewMemoryV1(),
		commitBeforeErr:    core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost"),
		historicalOverride: &drift,
	}
	adapter, err := NewReviewerContextAdapterV1(fault, func() time.Time { return now.Add(time.Minute) })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.PublishReviewerContextV1(context.Background(), reviewport.ReviewerContextPublishRequestV1{Value: value}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("lost reply canonical drift was accepted: %v", err)
	}
}

func TestReviewerContextAdapterClosedErrorsAndTypedNilV1(t *testing.T) {
	var nilStore *reviewcontextstore.MemoryV1
	if _, err := NewReviewerContextAdapterV1(nilStore, time.Now); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("typed-nil repository accepted: %v", err)
	}
	if _, err := NewReviewerContextAdapterV1(reviewcontextstore.NewMemoryV1(), nil); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil clock accepted: %v", err)
	}
	now := time.Unix(1_931_000_000, 0)
	value, err := testfixture.ReviewerContextEnvelopeV1(testfixture.ReviewerContextSubjectV1("errors"), 1, now, now.Add(time.Hour), "errors")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		err      error
		category core.ErrorCategory
	}{
		{name: "opaque", err: errors.New("opaque"), category: core.ErrorIndeterminate},
		{name: "unavailable", err: core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "down"), category: core.ErrorUnavailable},
		{name: "not-found", err: core.NewError(core.ErrorNotFound, core.ReasonEvidenceSourceMissing, "missing"), category: core.ErrorNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter, newErr := NewReviewerContextAdapterV1(readErrorRepositoryV1{RepositoryV1: reviewcontextstore.NewMemoryV1(), err: tc.err}, func() time.Time { return now.Add(time.Minute) })
			if newErr != nil {
				t.Fatal(newErr)
			}
			if _, gotErr := adapter.ResolveCurrentReviewerContextV1(context.Background(), reviewport.ReviewerContextCurrentResolveRequestV1{Subject: value.Subject}); !core.HasCategory(gotErr, tc.category) {
				t.Fatalf("closed error category drift: %v", gotErr)
			}
		})
	}
}

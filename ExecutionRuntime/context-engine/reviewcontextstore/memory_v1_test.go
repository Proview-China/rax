package reviewcontextstore

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	reviewcontract "github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func repositoryFixtureV1(t testing.TB, suffix string) (*MemoryV1, reviewcontract.ReviewerContextEnvelopeV1, reviewcontract.ReviewerContextEnvelopeV1) {
	t.Helper()
	now := time.Unix(1_930_000_000, 0)
	subject := testfixture.ReviewerContextSubjectV1(suffix)
	first, err := testfixture.ReviewerContextEnvelopeV1(subject, 1, now, now.Add(time.Hour), "first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := testfixture.ReviewerContextEnvelopeV1(subject, 2, now, now.Add(time.Hour), "second")
	if err != nil {
		t.Fatal(err)
	}
	return NewMemoryV1(), first, second
}

func TestReviewerContextRepositoryCreateCASHistoryAndDeepCloneV1(t *testing.T) {
	store, first, second := repositoryFixtureV1(t, "repository")
	ctx := context.Background()
	receipt, err := store.CommitV1(ctx, reviewport.ReviewerContextPublishRequestV1{Value: first})
	if err != nil || !receipt.Created || receipt.Ref != first.Ref {
		t.Fatalf("initial commit: %#v %v", receipt, err)
	}
	replay, err := store.CommitV1(ctx, reviewport.ReviewerContextPublishRequestV1{Value: first})
	if err != nil || replay.Created {
		t.Fatalf("exact replay was not read-only idempotent: %#v %v", replay, err)
	}
	if _, err = store.CommitV1(ctx, reviewport.ReviewerContextPublishRequestV1{Previous: &first.Ref, Value: second}); err != nil {
		t.Fatal(err)
	}
	current, err := store.ResolveV1(ctx, first.Subject)
	if err != nil || current != second.Ref {
		t.Fatalf("current ref: %#v %v", current, err)
	}
	historical, err := store.InspectHistoricalV1(ctx, first.Ref)
	if err != nil || historical.Ref != first.Ref {
		t.Fatalf("historical exact inspect: %#v %v", historical.Ref, err)
	}
	historical.Materials[0].Content = "mutated"
	again, err := store.InspectHistoricalV1(ctx, first.Ref)
	if err != nil || again.Materials[0].Content == "mutated" {
		t.Fatal("historical inspect leaked a mutable alias")
	}
	currentValue, err := store.InspectCurrentV1(ctx, second.Subject, second.Ref)
	if err != nil {
		t.Fatal(err)
	}
	currentValue.AllowedReadCapabilities[0] = "mutated"
	again, err = store.InspectCurrentV1(ctx, second.Subject, second.Ref)
	if err != nil || again.AllowedReadCapabilities[0] == "mutated" {
		t.Fatal("current inspect leaked a mutable alias")
	}
	if _, err = store.InspectCurrentV1(ctx, first.Subject, first.Ref); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("historical ref passed current index: %v", err)
	}
}

func TestReviewerContextRepositoryConflictsNeverOverwriteHistoryV1(t *testing.T) {
	store, first, second := repositoryFixtureV1(t, "conflict")
	ctx := context.Background()
	if _, err := store.CommitV1(ctx, reviewport.ReviewerContextPublishRequestV1{Value: first}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitV1(ctx, reviewport.ReviewerContextPublishRequestV1{Previous: &first.Ref, Value: second}); err != nil {
		t.Fatal(err)
	}
	drift, err := testfixture.ReviewerContextEnvelopeV1(first.Subject, 2, time.Unix(1_930_000_000, 0), time.Unix(1_930_000_000, 0).Add(time.Hour), "different")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CommitV1(ctx, reviewport.ReviewerContextPublishRequestV1{Previous: &first.Ref, Value: drift}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same revision payload drift was accepted: %v", err)
	}
	old, err := store.InspectHistoricalV1(ctx, second.Ref)
	if err != nil || old.Ref != second.Ref {
		t.Fatal("same-revision conflict overwrote old exact history")
	}
	badPrevious := first.Ref
	badPrevious.Digest = core.DigestBytes([]byte("wrong"))
	third, buildErr := testfixture.ReviewerContextEnvelopeV1(first.Subject, 3, time.Unix(1_930_000_000, 0), time.Unix(1_930_000_000, 0).Add(time.Hour), "third")
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	if _, err = store.CommitV1(ctx, reviewport.ReviewerContextPublishRequestV1{Previous: &badPrevious, Value: third}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("drifted predecessor was accepted: %v", err)
	}
	if ref, err := store.ResolveV1(ctx, first.Subject); err != nil || ref != second.Ref {
		t.Fatal("failed CAS changed current state")
	}
}

func TestReviewerContextRepositoryInvalidStageLeaksNothingV1(t *testing.T) {
	store, first, _ := repositoryFixtureV1(t, "stage")
	invalid := first.Clone()
	invalid.Materials[0].Content = "digest drift"
	if _, err := store.CommitV1(context.Background(), reviewport.ReviewerContextPublishRequestV1{Value: invalid}); err == nil {
		t.Fatal("invalid staged value was accepted")
	}
	if len(store.history) != 0 || len(store.highest) != 0 || len(store.current) != 0 {
		t.Fatal("failed staged commit leaked history, highest, or current")
	}
	if _, err := store.ResolveV1(context.Background(), first.Subject); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("failed stage became visible: %v", err)
	}
}

func TestReviewerContextRepositoryHistoricalInspectDoesNotBorrowCurrentV1(t *testing.T) {
	store, first, second := repositoryFixtureV1(t, "history")
	ctx := context.Background()
	if _, err := store.CommitV1(ctx, reviewport.ReviewerContextPublishRequestV1{Value: first}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitV1(ctx, reviewport.ReviewerContextPublishRequestV1{Previous: &first.Ref, Value: second}); err != nil {
		t.Fatal(err)
	}
	identity := identityKeyV1{tenant: second.Ref.TenantID, id: second.Ref.ID}
	store.mu.Lock()
	store.current[identity] = first.Ref // corrupt current only
	store.mu.Unlock()
	if _, err := store.InspectHistoricalV1(ctx, second.Ref); err != nil {
		t.Fatalf("historical exact inspect borrowed bad current index: %v", err)
	}
	if _, err := store.ResolveV1(ctx, second.Subject); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("bad current closure was not detected: %v", err)
	}
}

func TestReviewerContextRepositoryConcurrentCreateAndCASV1(t *testing.T) {
	store, first, second := repositoryFixtureV1(t, "concurrent")
	ctx := context.Background()
	var created atomic.Int64
	var failures atomic.Int64
	run := func(request reviewport.ReviewerContextPublishRequestV1) {
		var wg sync.WaitGroup
		for range 64 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				receipt, err := store.CommitV1(ctx, request)
				if err != nil {
					failures.Add(1)
					return
				}
				if receipt.Created {
					created.Add(1)
				}
			}()
		}
		wg.Wait()
	}
	run(reviewport.ReviewerContextPublishRequestV1{Value: first})
	if created.Load() != 1 || failures.Load() != 0 {
		t.Fatalf("concurrent create: created=%d failures=%d", created.Load(), failures.Load())
	}
	created.Store(0)
	run(reviewport.ReviewerContextPublishRequestV1{Previous: &first.Ref, Value: second})
	if created.Load() != 1 || failures.Load() != 0 {
		t.Fatalf("concurrent CAS: created=%d failures=%d", created.Load(), failures.Load())
	}
	if ref, err := store.ResolveV1(ctx, second.Subject); err != nil || ref != second.Ref {
		t.Fatalf("concurrent current closure: %#v %v", ref, err)
	}
}

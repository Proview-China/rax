package blackbox_test

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/reviewadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/reviewcontextstore"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
)

func TestReviewerContextPublicPortsConcurrentCanonicalPublishV1(t *testing.T) {
	now := time.Unix(1_932_000_000, 0)
	value, err := testfixture.ReviewerContextEnvelopeV1(testfixture.ReviewerContextSubjectV1("blackbox"), 1, now, now.Add(time.Hour), "blackbox")
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := reviewadapter.NewReviewerContextAdapterV1(reviewcontextstore.NewMemoryV1(), func() time.Time { return now.Add(time.Minute) })
	if err != nil {
		t.Fatal(err)
	}
	var created atomic.Int64
	var failed atomic.Int64
	var wg sync.WaitGroup
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			receipt, publishErr := adapter.PublishReviewerContextV1(context.Background(), reviewport.ReviewerContextPublishRequestV1{Value: value})
			if publishErr != nil {
				failed.Add(1)
				return
			}
			if receipt.Created {
				created.Add(1)
			}
		}()
	}
	wg.Wait()
	if failed.Load() != 0 || created.Load() != 1 {
		t.Fatalf("public concurrent create-once: created=%d failed=%d", created.Load(), failed.Load())
	}
	ref, err := adapter.ResolveCurrentReviewerContextV1(context.Background(), reviewport.ReviewerContextCurrentResolveRequestV1{Subject: value.Subject})
	if err != nil || ref != value.Ref {
		t.Fatalf("public current resolve: %#v %v", ref, err)
	}
}

func TestDurableReviewerContextPublicRestartRecoveryV1(t *testing.T) {
	now := time.Unix(1_932_000_000, 0)
	path := filepath.Join(t.TempDir(), "reviewer-context.db")
	value, err := testfixture.ReviewerContextEnvelopeV1(testfixture.ReviewerContextSubjectV1("durable-blackbox"), 1, now, now.Add(time.Hour), "durable-blackbox")
	if err != nil {
		t.Fatal(err)
	}
	first, err := reviewadapter.OpenDurableReviewerContextAdapterV1(context.Background(), reviewcontextstore.SQLiteConfigV1{Path: path}, func() time.Time { return now.Add(time.Minute) })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.PublishReviewerContextV1(context.Background(), reviewport.ReviewerContextPublishRequestV1{Value: value}); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := reviewadapter.OpenDurableReviewerContextAdapterV1(context.Background(), reviewcontextstore.SQLiteConfigV1{Path: path}, func() time.Time { return now.Add(2 * time.Minute) })
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	if ref, err := reopened.ResolveCurrentReviewerContextV1(context.Background(), reviewport.ReviewerContextCurrentResolveRequestV1{Subject: value.Subject}); err != nil || ref != value.Ref {
		t.Fatalf("durable public restart recovery: %#v %v", ref, err)
	}
}

func TestReviewerContextStableSubjectIdentityAndIndependentSubjectsV1(t *testing.T) {
	now := time.Unix(1_932_000_000, 0)
	store := reviewcontextstore.NewMemoryV1()
	adapter, err := reviewadapter.NewReviewerContextAdapterV1(store, func() time.Time { return now.Add(time.Minute) })
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"subject-a", "subject-b"} {
		value, buildErr := testfixture.ReviewerContextEnvelopeV1(testfixture.ReviewerContextSubjectV1(suffix), 1, now, now.Add(time.Hour), suffix)
		if buildErr != nil {
			t.Fatal(buildErr)
		}
		if _, publishErr := adapter.PublishReviewerContextV1(context.Background(), reviewport.ReviewerContextPublishRequestV1{Value: value}); publishErr != nil {
			t.Fatal(publishErr)
		}
		resolved, resolveErr := adapter.ResolveCurrentReviewerContextV1(context.Background(), reviewport.ReviewerContextCurrentResolveRequestV1{Subject: value.Subject})
		if resolveErr != nil || resolved.ID != value.Ref.ID {
			t.Fatalf("stable subject identity: %#v %v", resolved, resolveErr)
		}
	}
}

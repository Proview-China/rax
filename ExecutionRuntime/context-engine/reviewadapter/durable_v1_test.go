package reviewadapter

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/reviewcontextstore"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestDurableReviewerContextAdapterRestartAndImmutableCurrentV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reviewer-context.db")
	now := time.Unix(1_934_000_000, 0)
	value, err := testfixture.ReviewerContextEnvelopeV1(testfixture.ReviewerContextSubjectV1("durable-adapter"), 1, now, now.Add(time.Hour), "durable")
	if err != nil {
		t.Fatal(err)
	}
	first, err := OpenDurableReviewerContextAdapterV1(context.Background(), reviewcontextstore.SQLiteConfigV1{Path: path}, func() time.Time { return now.Add(time.Minute) })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.PublishReviewerContextV1(context.Background(), reviewport.ReviewerContextPublishRequestV1{Value: value}); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenDurableReviewerContextAdapterV1(context.Background(), reviewcontextstore.SQLiteConfigV1{Path: path}, func() time.Time { return now.Add(2 * time.Minute) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if err := reopened.IntegrityCheckV1(context.Background()); err != nil {
		t.Fatal(err)
	}
	ref, err := reopened.ResolveCurrentReviewerContextV1(context.Background(), reviewport.ReviewerContextCurrentResolveRequestV1{Subject: value.Subject})
	if err != nil || ref != value.Ref {
		t.Fatalf("durable current resolve after restart: %#v %v", ref, err)
	}
	inspected, err := reopened.InspectCurrentReviewerContextV1(context.Background(), value.Subject, value.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if inspected.CheckedUnixNano != value.CheckedUnixNano || inspected.ExpiresUnixNano != value.ExpiresUnixNano || inspected.ProjectionDigest != value.ProjectionDigest {
		t.Fatal("durable adapter re-sealed immutable current projection")
	}
}

func TestDurableReviewerContextAdapterConstructorFailClosedV1(t *testing.T) {
	if adapter, err := OpenDurableReviewerContextAdapterV1(context.Background(), reviewcontextstore.SQLiteConfigV1{Path: filepath.Join(t.TempDir(), "reviewer-context.db")}, nil); !core.HasCategory(err, core.ErrorInvalidArgument) || adapter != nil {
		t.Fatalf("durable adapter accepted nil clock: %#v %v", adapter, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if adapter, err := OpenDurableReviewerContextAdapterV1(ctx, reviewcontextstore.SQLiteConfigV1{Path: filepath.Join(t.TempDir(), "reviewer-context.db")}, time.Now); !core.HasCategory(err, core.ErrorIndeterminate) || adapter != nil {
		t.Fatalf("durable adapter accepted canceled open: %#v %v", adapter, err)
	}
}

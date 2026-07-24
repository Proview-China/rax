package conformance_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/reviewadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/reviewcontextconformance"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/reviewcontextstore"
)

func TestReviewerContextOwnerConformanceV1(t *testing.T) {
	reviewcontextconformance.RunV1(t, func(t *testing.T) reviewcontextconformance.FixtureV1 {
		now := time.Unix(1_932_000_000, 0)
		subject := testfixture.ReviewerContextSubjectV1("conformance")
		initial, err := testfixture.ReviewerContextEnvelopeV1(subject, 1, now, now.Add(time.Hour), "initial")
		if err != nil {
			t.Fatal(err)
		}
		next, err := testfixture.ReviewerContextEnvelopeV1(subject, 2, now, now.Add(time.Hour), "next")
		if err != nil {
			t.Fatal(err)
		}
		adapter, err := reviewadapter.NewReviewerContextAdapterV1(reviewcontextstore.NewMemoryV1(), func() time.Time { return now.Add(time.Minute) })
		if err != nil {
			t.Fatal(err)
		}
		return reviewcontextconformance.FixtureV1{Publisher: adapter, Reader: adapter, Initial: initial, Next: next}
	})
}

func TestDurableReviewerContextOwnerConformanceV1(t *testing.T) {
	reviewcontextconformance.RunV1(t, func(t *testing.T) reviewcontextconformance.FixtureV1 {
		now := time.Unix(1_932_000_000, 0)
		subject := testfixture.ReviewerContextSubjectV1("durable-conformance")
		initial, err := testfixture.ReviewerContextEnvelopeV1(subject, 1, now, now.Add(time.Hour), "initial")
		if err != nil {
			t.Fatal(err)
		}
		next, err := testfixture.ReviewerContextEnvelopeV1(subject, 2, now, now.Add(time.Hour), "next")
		if err != nil {
			t.Fatal(err)
		}
		adapter, err := reviewadapter.OpenDurableReviewerContextAdapterV1(context.Background(), reviewcontextstore.SQLiteConfigV1{Path: filepath.Join(t.TempDir(), "reviewer-context.db")}, func() time.Time { return now.Add(time.Minute) })
		if err != nil {
			t.Fatal(err)
		}
		return reviewcontextconformance.FixtureV1{Publisher: adapter, Reader: adapter, Initial: initial, Next: next, Close: func() { _ = adapter.Close() }}
	})
}

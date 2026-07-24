// Package reviewcontextconformance provides a reusable owner conformance suite
// for Review's public Reviewer Context publisher/current-reader ports.
package reviewcontextconformance

import (
	"context"
	"testing"

	reviewcontract "github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type FixtureV1 struct {
	Publisher reviewport.ReviewerContextPublisherV1
	Reader    reviewport.ReviewerContextCurrentReaderV1
	Initial   reviewcontract.ReviewerContextEnvelopeV1
	Next      reviewcontract.ReviewerContextEnvelopeV1
	Close     func()
}

type FactoryV1 func(*testing.T) FixtureV1

func RunV1(t *testing.T, factory FactoryV1) {
	t.Helper()
	fixture := factory(t)
	if fixture.Close != nil {
		defer fixture.Close()
	}
	if fixture.Publisher == nil || fixture.Reader == nil {
		t.Fatal("Reviewer Context conformance fixture has nil public ports")
	}
	ctx := context.Background()
	receipt, err := fixture.Publisher.PublishReviewerContextV1(ctx, reviewport.ReviewerContextPublishRequestV1{Value: fixture.Initial})
	if err != nil || !receipt.Created || receipt.Ref != fixture.Initial.Ref {
		t.Fatalf("create-once publish: %#v %v", receipt, err)
	}
	resolved, err := fixture.Reader.ResolveCurrentReviewerContextV1(ctx, reviewport.ReviewerContextCurrentResolveRequestV1{Subject: fixture.Initial.Subject})
	if err != nil || resolved != fixture.Initial.Ref {
		t.Fatalf("subject exact resolve: %#v %v", resolved, err)
	}
	first, err := fixture.Reader.InspectCurrentReviewerContextV1(ctx, fixture.Initial.Subject, fixture.Initial.Ref)
	if err != nil || first.Ref != fixture.Initial.Ref {
		t.Fatalf("current exact inspect: %#v %v", first.Ref, err)
	}
	first.Materials[0].Content = "alias"
	again, err := fixture.Reader.InspectCurrentReviewerContextV1(ctx, fixture.Initial.Subject, fixture.Initial.Ref)
	if err != nil || again.Materials[0].Content == "alias" {
		t.Fatal("public current reader leaked mutable state")
	}
	receipt, err = fixture.Publisher.PublishReviewerContextV1(ctx, reviewport.ReviewerContextPublishRequestV1{Previous: &fixture.Initial.Ref, Value: fixture.Next})
	if err != nil || !receipt.Created || receipt.Ref != fixture.Next.Ref {
		t.Fatalf("full-ref CAS publish: %#v %v", receipt, err)
	}
	if _, err = fixture.Reader.InspectCurrentReviewerContextV1(ctx, fixture.Initial.Subject, fixture.Initial.Ref); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("old revision remained current: %v", err)
	}
	historical, err := fixture.Reader.InspectHistoricalReviewerContextV1(ctx, fixture.Initial.Ref)
	if err != nil || historical.Ref != fixture.Initial.Ref {
		t.Fatalf("append-only historical inspect: %#v %v", historical.Ref, err)
	}
	resolved, err = fixture.Reader.ResolveCurrentReviewerContextV1(ctx, reviewport.ReviewerContextCurrentResolveRequestV1{Subject: fixture.Next.Subject})
	if err != nil || resolved != fixture.Next.Ref {
		t.Fatalf("post-CAS current resolve: %#v %v", resolved, err)
	}
}

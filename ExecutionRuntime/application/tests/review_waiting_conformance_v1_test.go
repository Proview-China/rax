package application_test

import (
	"context"
	"testing"
	"time"

	applicationconformance "github.com/Proview-China/rax/ExecutionRuntime/application/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/application/fakes"
)

func TestReviewWaitingCoordinationConformanceV1(t *testing.T) {
	fixture := newReviewWaitingFixtureV1(t)
	initial, err := contract.NewReviewWaitingCoordinationFactV1(fixture.request, fixture.now.Add(5))
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := contract.ClaimReviewWaitingStartV1(initial, "review-start-claim/conformance", fixture.now.Add(6))
	if err != nil {
		t.Fatal(err)
	}
	store := fakes.NewReviewWaitingStoreV1(func() time.Time { return fixture.now.Add(10) })
	report, err := applicationconformance.RunReviewWaitingCoordinationV1(context.Background(), store, applicationconformance.ReviewWaitingCoordinationFixtureV1{Initial: initial, Claimed: claimed})
	if err != nil {
		t.Fatal(err)
	}
	if !report.CreateOnce || !report.FullRefCAS || !report.HistoricalExact || !report.IdempotentNonOwner || report.ProductionEligible {
		t.Fatalf("conformance report drifted: %+v", report)
	}
}

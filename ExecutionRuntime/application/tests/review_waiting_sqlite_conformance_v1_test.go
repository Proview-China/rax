package application_test

import (
	"context"
	"testing"
	"time"

	applicationconformance "github.com/Proview-China/rax/ExecutionRuntime/application/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationsqlite "github.com/Proview-China/rax/ExecutionRuntime/application/storage/sqlite"
)

func TestReviewWaitingSQLiteConformanceV1(t *testing.T) {
	fixture := newReviewWaitingFixtureV1(t)
	initial, err := contract.NewReviewWaitingCoordinationFactV1(fixture.request, fixture.now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := contract.ClaimReviewWaitingStartV1(initial, "review-start-claim/sqlite-conformance", fixture.now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	store, err := applicationsqlite.OpenV1(context.Background(), applicationsqlite.ConfigV1{Path: t.TempDir() + "/application.db", Clock: func() time.Time { return fixture.now.Add(5 * time.Second) }})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	report, err := applicationconformance.RunReviewWaitingCoordinationV1(context.Background(), store, applicationconformance.ReviewWaitingCoordinationFixtureV1{Initial: initial, Claimed: claimed})
	if err != nil {
		t.Fatal(err)
	}
	if !report.CreateOnce || !report.FullRefCAS || !report.HistoricalExact || !report.IdempotentNonOwner || report.ProductionEligible {
		t.Fatalf("SQLite conformance drifted: %+v", report)
	}
}

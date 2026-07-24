package bridgecontract_test

import (
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/bridgecontract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestCompletionReviewGateV2UsesApplicationObjectsAndExactMinimumTTL(t *testing.T) {
	fixture := testkit.CompletionReviewGateV2(t, applicationcontract.ReviewPhaseRunV1, "contract-min")
	now := fixture.Now.Add(5 * time.Second)
	result, err := bridgecontract.NewCompletionReviewGateResultV2(fixture.Request, fixture.Input, fixture.Outcome, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExpiresUnixNano != fixture.Outcome.Review.ExpiresUnixNano || result.Outcome.Receipt == nil || result.Outcome.Receipt.Digest != fixture.Outcome.Receipt.Digest {
		t.Fatalf("Application objects or min TTL changed: %#v", result)
	}
	tampered := result.Clone()
	tampered.Input.Target.Digest = core.DigestBytes([]byte("tampered-target"))
	if err := tampered.ValidateCurrentFor(fixture.Request, now); err == nil {
		t.Fatal("tampered Application current projection was accepted")
	}
}

func TestCompletionReviewGateV2RejectsActionPhaseWithoutResealingApplication(t *testing.T) {
	fixture := testkit.CompletionReviewGateV2(t, applicationcontract.ReviewPhaseSubagentV1, "contract-phase")
	waitingDigest := fixture.Request.Waiting.Digest
	fixture.Request.Waiting.Phase.Kind = applicationcontract.ReviewPhaseActionV1
	if err := fixture.Request.ValidateCurrent(fixture.Now.Add(5 * time.Second)); err == nil {
		t.Fatal("action.review entered completion Gate")
	}
	if fixture.Request.Waiting.Digest != waitingDigest {
		t.Fatal("bridge validation resealed the Application Request")
	}
}

func TestCompletionReviewGateV2PreservesClockRegressionReason(t *testing.T) {
	fixture := testkit.CompletionReviewGateV2(t, applicationcontract.ReviewPhaseRunV1, "contract-rollback")
	checked := fixture.Now.Add(5 * time.Second)
	result, err := bridgecontract.NewCompletionReviewGateResultV2(fixture.Request, fixture.Input, fixture.Outcome, checked)
	if err != nil {
		t.Fatal(err)
	}
	rollback := checked.Add(-time.Nanosecond)
	if err := result.ValidateCurrentFor(fixture.Request, rollback); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("Result rollback was not preserved: %v", err)
	}
}

func TestCompletionReviewGateV2EachTTLSourceExpiresAtItsActualPoint(t *testing.T) {
	fixture := testkit.CompletionReviewGateV2(t, applicationcontract.ReviewPhaseRunV1, "contract-ttl-sources")
	checks := []struct {
		name    string
		expires int64
		check   func(time.Time) error
	}{
		{"request", fixture.Request.Waiting.ExpiresUnixNano, func(now time.Time) error { return fixture.Request.ValidateCurrent(now) }},
		{"input", fixture.Input.ExpiresUnixNano, func(now time.Time) error { return fixture.Input.ValidateFor(fixture.Request.Waiting, now) }},
		{"review", fixture.Outcome.Review.ExpiresUnixNano, func(now time.Time) error { return fixture.Outcome.Review.ValidateFor(fixture.Request.Waiting, now) }},
		{"receipt", fixture.Outcome.Receipt.ExpiresUnixNano, func(now time.Time) error {
			return fixture.Outcome.Receipt.ValidateCurrentFor(fixture.Request.Waiting, now)
		}},
	}
	for _, testCase := range checks {
		t.Run(testCase.name, func(t *testing.T) {
			if err := testCase.check(time.Unix(0, testCase.expires)); !core.HasReason(err, core.ReasonReviewVerdictStale) {
				t.Fatalf("%s TTL actual point was accepted: %v", testCase.name, err)
			}
		})
	}
}

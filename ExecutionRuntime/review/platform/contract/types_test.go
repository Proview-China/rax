package platformcontract_test

import (
	"testing"
	"time"

	reviewcontract "github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	platformcontract "github.com/Proview-China/rax/ExecutionRuntime/review/platform/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestDeliveryIntentAndInboundBindingRemainExactCandidatesV1(t *testing.T) {
	now := time.Unix(1_900_150_000, 0)
	caseRef := reviewcontract.ExactResourceRefV1{ID: "case-a", Revision: 2, Digest: testkit.Digest("case-a")}
	targetRef := reviewcontract.ExactResourceRefV1{ID: "target-a", Revision: 7, Digest: testkit.Digest("target-a")}
	envelope, err := platformcontract.SealHumanReviewEnvelopeV1(platformcontract.HumanReviewEnvelopeV1{TenantID: "tenant-a", EnvelopeID: "envelope-a", Case: caseRef, Target: targetRef, Title: "Human review required", Summary: "Review the exact candidate and evidence", DeepLink: "https://review.example.invalid/case-a", AllowedResolutions: []reviewcontract.ResolutionV1{reviewcontract.ResolutionAcceptV1, reviewcontract.ResolutionRejectV1}, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := platformcontract.BindingFromEnvelopeV1(envelope)
	if err != nil || binding.TenantID != envelope.TenantID || binding.Case != caseRef || binding.Target != targetRef || binding.EnvelopeDigest != envelope.Digest {
		t.Fatalf("envelope binding drifted: %+v %v", binding, err)
	}
	intent, err := platformcontract.SealDeliveryIntentV1(platformcontract.DeliveryIntentV1{TenantID: envelope.TenantID, ID: "delivery-a", Revision: 1, Platform: platformcontract.SlackV1, DestinationRef: "slack://channel-a", Envelope: envelope, IdempotencyKey: "delivery-case-a", CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Minute).UnixNano()})
	if err != nil || intent.Digest == "" {
		t.Fatalf("delivery intent did not seal: %+v %v", intent, err)
	}
	drift := binding
	drift.Target.Digest = ""
	if err := drift.Validate(); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("drifted inbound binding was accepted: %v", err)
	}
}

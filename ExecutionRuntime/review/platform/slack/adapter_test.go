package slack

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	platformcontract "github.com/Proview-China/rax/ExecutionRuntime/review/platform/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func testBindingV1() platformcontract.EnvelopeBindingV1 {
	return platformcontract.EnvelopeBindingV1{TenantID: "tenant-a", EnvelopeID: "envelope-a", EnvelopeDigest: testkit.Digest("envelope-a"), Revision: 1, Case: contract.ExactResourceRefV1{ID: "case-a", Revision: 1, Digest: testkit.Digest("case-a")}, Target: contract.ExactResourceRefV1{ID: "target-a", Revision: 1, Digest: testkit.Digest("target-a")}}
}

func sign(secret []byte, timestamp string, raw []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(append([]byte("v0:"+timestamp+":"), raw...))
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

func TestSlackObservationSignatureReplayAndDecisionV1(t *testing.T) {
	now := time.Unix(1_900_200_000, 0)
	timestamp := "1900200000"
	secret := []byte("slack-signing-secret-32-bytes-long!!")
	raw := []byte(`{"type":"block_actions","user":{"id":"U123"},"team":{"id":"T123"},"actions":[{"action_id":"praxis_review_accept"}]}`)
	headers := HeadersV1{Signature: sign(secret, timestamp, raw), RequestTimestamp: timestamp, ContentType: "application/json"}
	value, err := ParseObservationV1(secret, "tenant-a", testBindingV1(), headers, raw, now)
	if err != nil {
		t.Fatal(err)
	}
	if value.ProposedResolution != contract.ResolutionAcceptV1 || value.ActionID != "praxis_review_accept" {
		t.Fatalf("unexpected observation: %+v", value)
	}
	if err := value.ObservationBaseV1.Validate(now); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseObservationV1(secret, "tenant-a", testBindingV1(), headers, raw, now.Add(MaxSkewV1+time.Second)); !core.HasCategory(err, core.ErrorUnauthenticated) {
		t.Fatalf("old Slack request must fail: %v", err)
	}
	headers.Signature = headers.Signature[:len(headers.Signature)-1] + "0"
	if _, err := ParseObservationV1(secret, "tenant-a", testBindingV1(), headers, raw, now); !core.HasCategory(err, core.ErrorUnauthenticated) {
		t.Fatalf("tampered Slack signature must fail: %v", err)
	}
}

func TestSlackFreeTextCannotBecomeResolutionV1(t *testing.T) {
	now := time.Unix(1_900_200_001, 0)
	timestamp := "1900200001"
	secret := []byte("slack-signing-secret-32-bytes-long!!")
	raw := []byte(`{"type":"block_actions","user":{"id":"U123"},"team":{"id":"T123"},"actions":[{"action_id":"comment","value":"approved"}]}`)
	headers := HeadersV1{Signature: sign(secret, timestamp, raw), RequestTimestamp: timestamp, ContentType: "application/json"}
	if _, err := ParseObservationV1(secret, "tenant-a", testBindingV1(), headers, raw, now); !core.HasReason(err, core.ReasonUnknownGovernanceCategory) {
		t.Fatalf("free text approval was accepted: %v", err)
	}
}

func TestSlackObservationRejectsUnboundEnvelopeV1(t *testing.T) {
	now := time.Unix(1_900_200_002, 0)
	timestamp := "1900200002"
	secret := []byte("slack-signing-secret-32-bytes-long!!")
	raw := []byte(`{"type":"block_actions","user":{"id":"U123"},"team":{"id":"T123"},"actions":[{"action_id":"praxis_review_accept"}]}`)
	headers := HeadersV1{Signature: sign(secret, timestamp, raw), RequestTimestamp: timestamp, ContentType: "application/json"}
	drift := testBindingV1()
	drift.Case.Digest = ""
	if _, err := ParseObservationV1(secret, "tenant-a", drift, headers, raw, now); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("unbound Slack envelope was accepted: %v", err)
	}
	drift = testBindingV1()
	drift.TenantID = "tenant-b"
	if _, err := ParseObservationV1(secret, "tenant-a", drift, headers, raw, now); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("cross-tenant Slack envelope was accepted: %v", err)
	}
}

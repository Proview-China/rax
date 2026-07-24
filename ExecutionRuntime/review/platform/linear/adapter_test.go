package linear

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

func linearSign(secret, raw []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(raw)
	return hex.EncodeToString(mac.Sum(nil))
}
func TestLinearObservationAndTimestampBindingV1(t *testing.T) {
	now := time.UnixMilli(1_900_300_000_000)
	secret := []byte("linear-signing-secret-32-bytes-long!")
	raw := []byte(`{"action":"create","actor":{"id":"actor-a"},"data":{"id":"comment-a","body":"approved"},"organizationId":"org-a","webhookTimestamp":1900300000000}`)
	headers := HeadersV1{Signature: linearSign(secret, raw), Delivery: "delivery-a", Event: "Comment", Timestamp: "1900300000000"}
	value, err := ParseObservationV1(secret, "tenant-a", testBindingV1(), headers, raw, now)
	if err != nil {
		t.Fatal(err)
	}
	if value.SourceEventID != "linear:delivery-a" || value.ActorHandle != "actor-a" {
		t.Fatalf("unexpected Linear observation: %+v", value)
	}
	headers.Timestamp = "1900300000001"
	if _, err := ParseObservationV1(secret, "tenant-a", testBindingV1(), headers, raw, now); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("Linear timestamp drift must fail: %v", err)
	}
}

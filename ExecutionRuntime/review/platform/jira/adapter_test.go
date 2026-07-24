package jira

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

func jiraSign(secret, raw []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(raw)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
func TestJiraObservationIdentifierAndSignatureV1(t *testing.T) {
	now := time.UnixMilli(1_900_400_000_000)
	secret := []byte("jira-signing-secret-32-bytes-long!!")
	raw := []byte(`{"timestamp":1900400000000,"webhookEvent":"comment_created","user":{"accountId":"actor-a"},"issue":{"id":"10001","key":"PRX-1"},"comment":{"id":"20001","body":"approved"}}`)
	headers := HeadersV1{Signature: jiraSign(secret, raw), WebhookIdentifier: "delivery-a", Flow: "Primary"}
	value, err := ParseObservationV1(secret, "tenant-a", testBindingV1(), headers, raw, now)
	if err != nil {
		t.Fatal(err)
	}
	if value.SourceEventID != "jira:delivery-a" || value.IssueKey != "PRX-1" {
		t.Fatalf("unexpected Jira observation: %+v", value)
	}
	headers.WebhookIdentifier = ""
	if _, err := ParseObservationV1(secret, "tenant-a", testBindingV1(), headers, raw, now); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("missing Jira id must fail: %v", err)
	}
}

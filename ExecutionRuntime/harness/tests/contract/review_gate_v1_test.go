package contract_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestReviewGateReceiptV1IsObservationOnlyAndDigestProtected(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	receipt, err := contract.SealReviewGateReceiptV1(contract.ReviewGateReceiptV1{RunID: "run-review-gate", SessionID: "session-review-gate", SessionRevision: 2, SessionDigest: testkit.Digest("session-review-gate"), Turn: 1, ActionRef: "action-review-gate", ActionRequestDigest: testkit.Digest("action-review-gate"), Target: runtimeports.OperationReviewTargetRefV4{Ref: "target-review-gate", Revision: 1, Digest: testkit.Digest("target-review-gate")}, Authorization: &runtimeports.OperationReviewAuthorizationRefV5{ID: "authorization-review-gate", Revision: 1, Digest: testkit.Digest("authorization-review-gate")}, Basis: runtimeports.OperationReviewBasisAcceptedQuorumV5, ReviewProjectionDigest: testkit.Digest("projection-review-gate"), Decision: contract.ReviewGateAllowV1, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"verdict_authority", "authorization_fact", "dispatch", "commit"} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("receipt leaked an owner capability field %q: %s", forbidden, payload)
		}
	}
	tampered := receipt
	tampered.Decision = contract.ReviewGateDenyV1
	tampered.ErrorCategory = core.ErrorForbidden
	tampered.Reason = core.ReasonReviewVerdictStale
	if err := tampered.Validate(); !core.HasReason(err, core.ReasonInvalidDigest) {
		t.Fatalf("tampered receipt was not rejected by digest: %v", err)
	}
}

func TestReviewGateResultV1StrictDecodeRejectsDuplicateNestedKey(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	receipt, err := contract.SealReviewGateReceiptV1(contract.ReviewGateReceiptV1{RunID: "run-review-gate", SessionID: "session-review-gate", SessionRevision: 2, SessionDigest: testkit.Digest("session-review-gate"), Turn: 1, ActionRef: "action-review-gate", ActionRequestDigest: testkit.Digest("action-review-gate"), Target: runtimeports.OperationReviewTargetRefV4{Ref: "target-review-gate", Revision: 1, Digest: testkit.Digest("target-review-gate")}, Authorization: &runtimeports.OperationReviewAuthorizationRefV5{ID: "authorization-review-gate", Revision: 1, Digest: testkit.Digest("authorization-review-gate")}, Basis: runtimeports.OperationReviewBasisAcceptedQuorumV5, ReviewProjectionDigest: testkit.Digest("projection-review-gate"), Decision: contract.ReviewGateAllowV1, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	result := contract.ReviewGateResultV1{ContractVersion: contract.ReviewGateContractVersionV1, Decision: contract.ReviewGateAllowV1, Receipt: receipt}
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := strings.Replace(string(payload), `"target":{"ref":`, `"target":{"ref":"duplicate","ref":`, 1)
	if _, err := contract.DecodeReviewGateResultV1([]byte(duplicate)); !core.HasReason(err, core.ReasonDuplicateCanonicalKey) {
		t.Fatalf("duplicate nested key was not rejected: %v", err)
	}
}

func TestReviewGateReceiptV1NilAuthorizationCannotAllowDenyOrDefer(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	base := contract.ReviewGateReceiptV1{RunID: "run-review-gate", SessionID: "session-review-gate", SessionRevision: 2, SessionDigest: testkit.Digest("session-review-gate"), Turn: 1, ActionRef: "action-review-gate", ActionRequestDigest: testkit.Digest("action-review-gate"), Target: runtimeports.OperationReviewTargetRefV4{Ref: "target-review-gate", Revision: 1, Digest: testkit.Digest("target-review-gate")}, Basis: runtimeports.OperationReviewBasisAcceptedQuorumV5, ReviewProjectionDigest: testkit.Digest("projection-review-gate"), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Second).UnixNano()}
	for _, decision := range []contract.ReviewGatePhaseDecisionV1{contract.ReviewGateAllowV1, contract.ReviewGateDenyV1, contract.ReviewGateDeferV1} {
		candidate := base
		candidate.Decision = decision
		if decision != contract.ReviewGateAllowV1 {
			candidate.ErrorCategory = core.ErrorPreconditionFailed
			candidate.Reason = core.ReasonReviewVerdictStale
		}
		if _, err := contract.SealReviewGateReceiptV1(candidate); err == nil {
			t.Fatalf("nil Authorization sealed %q", decision)
		}
	}
}

package kernel

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
)

func TestSettlementIsRefOnlyAndDigestBound(t *testing.T) {
	result := contract.DomainResultFact{ContractVersion: contract.Version, ID: "result-1", Revision: 1, AttemptID: "attempt-1", IntentDigest: testkit.D("intent"), ResultDigest: testkit.D("result-payload"), State: contract.DomainResultSucceeded, CreatedUnixNano: testkit.Now}
	digest, _ := contract.DigestJSON(result)
	op := contract.OperationSettlementRef{OperationID: "operation-1", OperationRevision: 2, SettlementDigest: testkit.D("runtime-settlement"), DomainResultDigest: digest}
	fact, err := ApplySettlement("domain-settlement-1", result, op, testkit.Now+1)
	if err != nil || fact.OperationRef != op {
		t.Fatalf("fact=%#v err=%v", fact, err)
	}
	op.DomainResultDigest = testkit.D("other-result")
	if _, err := ApplySettlement("domain-settlement-2", result, op, testkit.Now+1); err == nil {
		t.Fatal("unbound runtime settlement was applied")
	}
}

func TestUnknownOutcomeOnlyInspectsOriginalAttempt(t *testing.T) {
	result := contract.DomainResultFact{ContractVersion: contract.Version, ID: "result-unknown", Revision: 1, AttemptID: "attempt-original", IntentDigest: testkit.D("intent"), ResultDigest: testkit.D("unknown"), State: contract.DomainResultUnknown, CreatedUnixNano: testkit.Now}
	action, err := RecoveryAction(result)
	if err != nil || action != "inspect_original_attempt" {
		t.Fatalf("action=%q err=%v", action, err)
	}
}

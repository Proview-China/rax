package domain_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
)

func TestApplySettlementOnlyValidatesOpaqueBinding(t *testing.T) {
	for _, outcome := range []contract.Outcome{contract.OutcomeSucceeded, contract.OutcomeFailed, contract.OutcomeUnknown} {
		factRef := *testkit.FactRef("domain-result-" + string(outcome))
		fact := contract.DomainResultFact{
			FactRef: factRef, OperationID: "operation-1", OperationDigest: "operation-digest-1",
			EffectKind: "continuity/remote-content-put", Outcome: outcome,
			ObservationEvidenceRefs: []string{"evidence-1"},
		}
		settlement := contract.OperationSettlementRef{
			SettlementID: "settlement-1", SettlementDigest: "settlement-digest-1",
			OperationID: fact.OperationID, OperationDigest: fact.OperationDigest,
			DomainResultFactID: factRef.ID, DomainResultDigest: factRef.Digest,
		}
		applied, err := domain.ApplySettlement(fact, settlement)
		if err != nil || applied.DomainResultFactRef.ID != factRef.ID || applied.RuntimeSettlement != settlement {
			t.Fatalf("outcome %s changed opaque binding behavior: %#v err=%v", outcome, applied, err)
		}
	}
}

func TestApplySettlementRejectsIdentityOrDigestMismatch(t *testing.T) {
	factRef := *testkit.FactRef("domain-result-1")
	fact := contract.DomainResultFact{
		FactRef: factRef, OperationID: "operation-1", OperationDigest: "operation-digest-1",
		EffectKind: "continuity/remote-content-put", Outcome: contract.OutcomeUnknown,
		ObservationEvidenceRefs: []string{"evidence-1"},
	}
	base := contract.OperationSettlementRef{
		SettlementID: "settlement-1", SettlementDigest: "settlement-digest-1",
		OperationID: fact.OperationID, OperationDigest: fact.OperationDigest,
		DomainResultFactID: factRef.ID, DomainResultDigest: factRef.Digest,
	}
	tests := map[string]func(*contract.OperationSettlementRef){
		"operation identity": func(ref *contract.OperationSettlementRef) { ref.OperationID = "operation-2" },
		"operation digest":   func(ref *contract.OperationSettlementRef) { ref.OperationDigest = "changed" },
		"fact identity":      func(ref *contract.OperationSettlementRef) { ref.DomainResultFactID = "fact-2" },
		"fact digest":        func(ref *contract.OperationSettlementRef) { ref.DomainResultDigest = "changed" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			ref := base
			mutate(&ref)
			if _, err := domain.ApplySettlement(fact, ref); !contract.HasCode(err, contract.ErrRevisionConflict) {
				t.Fatalf("mismatch should fail, got %v", err)
			}
		})
	}
}

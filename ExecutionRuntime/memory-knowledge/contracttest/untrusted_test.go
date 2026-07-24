package contracttest

import (
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func TestDomainResultCanonicalDigestCoversEveryAuthoritativeField(t *testing.T) {
	t.Parallel()
	result := validDomainResult(t, contract.OwnerMemory)
	tamperCases := map[string]func(*contract.DomainResultFact){
		"ref-digest": func(v *contract.DomainResultFact) { v.Ref.Digest = "sha256:wrong" },
		"attempt":    func(v *contract.DomainResultFact) { v.AttemptID = "other-attempt" },
		"operation":  func(v *contract.DomainResultFact) { v.OperationRef.Digest = "sha256:other-operation" },
		"subject":    func(v *contract.DomainResultFact) { v.SubjectRef.Revision++ },
		"cas":        func(v *contract.DomainResultFact) { v.CASAfter++ },
		"inspection": func(v *contract.DomainResultFact) { v.InspectionRef.ID = "other-inspection" },
		"evidence": func(v *contract.DomainResultFact) {
			v.EvidenceRefs = append(v.EvidenceRefs, trustedRef("other-evidence"))
		},
		"coverage":       func(v *contract.DomainResultFact) { v.Coverage.Available++ },
		"cleanup":        func(v *contract.DomainResultFact) { v.CleanupState = "other-cleanup" },
		"residuals":      func(v *contract.DomainResultFact) { v.Residuals = append(v.Residuals, "new-residual") },
		"state":          func(v *contract.DomainResultFact) { v.State = contract.DomainResultReconciliation },
		"transaction-at": func(v *contract.DomainResultFact) { v.CreatedAt = v.CreatedAt.Add(time.Nanosecond) },
	}
	for name, tamper := range tamperCases {
		t.Run(name, func(t *testing.T) {
			changed := result
			changed.EvidenceRefs = append([]contract.Ref(nil), result.EvidenceRefs...)
			changed.Residuals = append([]string(nil), result.Residuals...)
			tamper(&changed)
			if err := changed.Validate(); !errors.Is(err, contract.ErrEvidenceConflict) {
				t.Fatalf("tamper was not rejected as evidence conflict: %v", err)
			}
		})
	}
}

func TestUntrustedContractInputsReturnErrorsWithoutPanic(t *testing.T) {
	t.Parallel()
	result := validDomainResult(t, contract.OwnerKnowledge)
	invalidResult := result
	invalidResult.Ref.Digest = "sha256:tampered"
	inputs := map[string]func() error{
		"zero-result":      func() error { return (contract.DomainResultFact{}).Validate() },
		"tampered-result":  invalidResult.Validate,
		"zero-association": func() error { return (contract.DomainResultAssociation{}).Verify(result) },
		"wrong-association": func() error {
			return (contract.DomainResultAssociation{DomainResultRef: trustedRef("other-result")}).Verify(result)
		},
		"zero-settlement":  func() error { return (contract.RuntimeSettlementRef{}).Validate() },
		"zero-application": func() error { return (contract.SettlementApplication{}).Validate() },
		"malformed-json": func() error {
			var dst contract.DomainResultAssociation
			return contract.StrictDecode([]byte(`{"domain_result_ref":`), &dst)
		},
		"unsupported-digest": func() error {
			_, err := contract.Digest(make(chan int))
			return err
		},
	}
	for name, call := range inputs {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("untrusted input panicked: %v", recovered)
				}
			}()
			if err := call(); err == nil {
				t.Fatal("untrusted input was accepted")
			}
		})
	}
}

func TestDomainResultAssociationCannotCrossOwnerWithSameIdentity(t *testing.T) {
	t.Parallel()
	memoryResult := validDomainResult(t, contract.OwnerMemory)
	knowledgeResult := validDomainResult(t, contract.OwnerKnowledge)
	if memoryResult.Ref.ID != knowledgeResult.Ref.ID || memoryResult.Ref.Revision != knowledgeResult.Ref.Revision {
		t.Fatal("fixture must exercise the same result identity")
	}
	if memoryResult.Ref.Digest == knowledgeResult.Ref.Digest {
		t.Fatal("owner was not covered by the canonical domain result digest")
	}
	association := contract.DomainResultAssociation{DomainResultRef: memoryResult.Ref}
	if err := association.Verify(knowledgeResult); !errors.Is(err, contract.ErrSettlementMismatch) {
		t.Fatalf("memory association crossed into knowledge owner: %v", err)
	}
}

func validDomainResult(t *testing.T, owner contract.OwnerDomain) contract.DomainResultFact {
	t.Helper()
	ref := trustedRef("fact")
	result, err := contract.NewDomainResultFact(
		owner, "result", "attempt", ref, ref, ref, 0, 1,
		[]contract.Ref{trustedRef("evidence")},
		contract.Coverage{Status: contract.CoverageComplete, Expected: 1, Available: 1},
		"complete", []string{"retained-residual"},
		time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func trustedRef(id string) contract.Ref {
	return contract.Ref{ID: id, Revision: 1, Digest: "sha256:" + id}
}

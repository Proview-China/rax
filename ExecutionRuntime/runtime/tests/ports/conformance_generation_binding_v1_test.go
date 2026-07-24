package ports_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestGenerationBindingAssociationConformanceV1IsContractOnly(t *testing.T) {
	t.Parallel()
	now := time.Unix(82_000, 0)
	candidate := generationBindingCandidatePortV1(t, now)
	fact, err := ports.SealGenerationBindingAssociationFactV1(ports.GenerationBindingAssociationFactV1{
		ID: candidate.AssociationID, Revision: 1, State: ports.GenerationBindingAssociationActiveV1,
		Candidate: candidate, CandidateDigest: candidate.Digest,
		CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := conformance.CheckGenerationBindingAssociationV1(context.Background(), conformance.GenerationBindingAssociationCaseV1{Gateway: staticGenerationBindingGovernanceV1{fact: fact}, Candidate: candidate})
	if err != nil {
		t.Fatal(err)
	}
	if !report.RuntimeFactOwnerObserved || !report.CurrentInspectObserved || report.CandidateIsBindingFact || report.ProductionClaimEligible {
		t.Fatalf("contract testkit claimed Binding or production eligibility: %+v", report)
	}
}

type staticGenerationBindingGovernanceV1 struct {
	fact ports.GenerationBindingAssociationFactV1
}

func (s staticGenerationBindingGovernanceV1) AssociateGenerationBindingV1(context.Context, ports.GenerationBindingAssociationCandidateV1) (ports.GenerationBindingAssociationFactV1, error) {
	return s.fact, nil
}

func (s staticGenerationBindingGovernanceV1) InspectCurrentGenerationBindingAssociationV1(context.Context, string) (ports.GenerationBindingAssociationFactV1, error) {
	return s.fact, nil
}

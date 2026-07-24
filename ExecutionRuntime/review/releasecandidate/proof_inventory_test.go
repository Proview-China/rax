package releasecandidate_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/releasecandidate"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestProductionProofInventoryKeepsAllElevenProofsMissingV1(t *testing.T) {
	candidate := build(t)
	assessments := candidate.Readiness.ProofAssessments
	if len(assessments) != 11 || len(candidate.Readiness.MissingProductionProofs) != 11 {
		t.Fatalf("Review proof inventory is incomplete: assessments=%d missing=%d", len(assessments), len(candidate.Readiness.MissingProductionProofs))
	}
	for index, assessment := range assessments {
		if assessment.Requirement != candidate.Readiness.RequiredProductionProofs[index] || assessment.Requirement != candidate.Readiness.MissingProductionProofs[index] || assessment.ProductionSatisfied || assessment.Blocker == "" {
			t.Fatalf("Review proof %d crossed its boundary: %+v", index, assessment)
		}
	}
	if !assessments[0].OwnerLocalImplemented || !assessments[1].OwnerLocalImplemented || !assessments[6].OwnerLocalImplemented || !assessments[8].OwnerLocalImplemented {
		t.Fatal("Review owner-local implementation inventory drifted")
	}
}

func TestProductionProofInventoryIsDeepCopiedAndValidatedV1(t *testing.T) {
	first := releasecandidate.CurrentProofAssessmentsV1()
	second := releasecandidate.CurrentProofAssessmentsV1()
	first[0].Blocker = releasecandidate.ProofBlockerCompositionRootMissingV1
	if second[0].Blocker == first[0].Blocker {
		t.Fatal("Review proof inventory leaked a mutable alias")
	}

	candidate := build(t)
	candidate.Readiness.ProofAssessments[0].ProductionSatisfied = true
	if err := candidate.ValidateCurrentV1(testNow.Add(time.Minute)); !core.HasReason(err, core.ReasonBindingNotCertified) {
		t.Fatalf("self-signed Review production proof was accepted: %v", err)
	}
	candidate = build(t)
	candidate.Readiness.ProofAssessments = candidate.Readiness.ProofAssessments[:10]
	if err := candidate.ValidateCurrentV1(testNow.Add(time.Minute)); !core.HasReason(err, core.ReasonBindingNotCertified) {
		t.Fatalf("incomplete Review proof inventory was accepted: %v", err)
	}
}

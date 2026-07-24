package releasecandidate_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/releasecandidate"
)

func TestProductionProofInventoryV1IsExactAndCannotSelfPromote(t *testing.T) {
	candidate := build(t)
	assessments := candidate.Readiness.ProofAssessments
	if len(assessments) != 11 || len(candidate.Readiness.RequiredProductionProofs) != 11 || len(candidate.Readiness.MissingProductionProofs) != 11 {
		t.Fatalf("proof inventory length drifted: assessments=%d required=%d missing=%d", len(assessments), len(candidate.Readiness.RequiredProductionProofs), len(candidate.Readiness.MissingProductionProofs))
	}
	for index, assessment := range assessments {
		if assessment.Requirement != candidate.Readiness.RequiredProductionProofs[index] || assessment.ProductionSatisfied || assessment.Blocker == "" {
			t.Fatalf("proof assessment %d drifted: %+v", index, assessment)
		}
		wantLocal := index < 6
		if assessment.OwnerLocalImplemented != wantLocal {
			t.Fatalf("proof assessment %d owner-local=%v want=%v", index, assessment.OwnerLocalImplemented, wantLocal)
		}
	}

	candidate.Readiness.ProofAssessments[0].ProductionSatisfied = true
	if err := candidate.ValidateCurrentV1(testNow); err == nil {
		t.Fatal("owner-local proof self-promoted to production")
	}
}

func TestCurrentProofAssessmentsV1ReturnsDeepCopy(t *testing.T) {
	first := releasecandidate.CurrentProofAssessmentsV1()
	first[0].Blocker = releasecandidate.BlockerDeploymentRootMissingV1
	second := releasecandidate.CurrentProofAssessmentsV1()
	if second[0].Blocker != releasecandidate.BlockerOwnerCurrentCertificationMissingV1 {
		t.Fatal("proof inventory accessor aliases package state")
	}
}

package conformance

import (
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type ReviewAdapterCaseV2 struct {
	Candidate   ports.ReviewCandidateV2              `json:"candidate"`
	Attestation ports.ReviewAttestationObservationV2 `json:"attestation"`
}

type ReviewAdapterReportV2 struct {
	AttestationValid        bool `json:"attestation_valid"`
	CertificationCandidate  bool `json:"certification_candidate"`
	BindingEligible         bool `json:"binding_eligible"`
	ProductionClaimEligible bool `json:"production_claim_eligible"`
	DispatchEligible        bool `json:"dispatch_eligible"`
	DomainCommitEligible    bool `json:"domain_commit_eligible"`
}

// CheckReviewAdapterV2 validates only a custom reviewer's opaque
// attestation envelope. Registration and a syntactically valid observation do
// not certify, bind, authorize dispatch or commit a domain result.
func CheckReviewAdapterV2(testCase ReviewAdapterCaseV2) (ReviewAdapterReportV2, error) {
	if err := testCase.Candidate.Validate(); err != nil {
		return ReviewAdapterReportV2{}, err
	}
	if err := testCase.Attestation.Validate(); err != nil {
		return ReviewAdapterReportV2{}, err
	}
	digest, err := testCase.Candidate.DigestV2()
	if err != nil {
		return ReviewAdapterReportV2{}, err
	}
	if testCase.Attestation.CaseID != testCase.Candidate.ID || testCase.Attestation.CandidateDigest != digest || testCase.Attestation.ReviewerBinding != testCase.Candidate.ReviewerBinding || testCase.Attestation.ReviewerAuthority != testCase.Candidate.ReviewerAuthority {
		return ReviewAdapterReportV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewCandidateConflict, "custom reviewer attestation drifted from governed candidate")
	}
	return ReviewAdapterReportV2{AttestationValid: true, CertificationCandidate: true}, nil
}

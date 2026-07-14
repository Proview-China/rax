package conformance

import (
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"time"
)

type EvidenceSourceAdapterCaseV2 struct {
	Source    ports.EvidenceSourceRegistrationFactV2 `json:"source"`
	Candidate ports.EvidenceEventCandidateV2         `json:"candidate"`
	Now       time.Time                              `json:"now"`
}
type EvidenceSourceAdapterReportV2 struct {
	EnvelopeValid             bool `json:"envelope_valid"`
	CertificationCandidate    bool `json:"certification_candidate"`
	BindingEligible           bool `json:"binding_eligible"`
	TrustGranted              bool `json:"trust_granted"`
	ClaimEligible             bool `json:"claim_eligible"`
	AuthoritativeFactEligible bool `json:"authoritative_fact_eligible"`
	AppendEligible            bool `json:"append_eligible"`
	DomainCommitEligible      bool `json:"domain_commit_eligible"`
}

// CheckEvidenceSourceAdapterV2 proves only that a custom source can form a
// structurally conforming envelope. Independent Binding, Source Policy,
// Authority and CurrentScope facts are intentionally absent, so no trust or
// append right can be self-granted.
func CheckEvidenceSourceAdapterV2(testCase EvidenceSourceAdapterCaseV2) (EvidenceSourceAdapterReportV2, error) {
	if err := control.ValidateEvidenceAppendV2(testCase.Source, ports.EvidenceAppendRequestV2{Candidate: testCase.Candidate, ExpectedSourceRevision: testCase.Source.Revision}, testCase.Now); err != nil {
		return EvidenceSourceAdapterReportV2{}, err
	}
	if testCase.Candidate.Producer != testCase.Source.Producer || testCase.Candidate.Authority != testCase.Source.Authority {
		return EvidenceSourceAdapterReportV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceSourceStale, "custom source envelope drifted")
	}
	return EvidenceSourceAdapterReportV2{EnvelopeValid: true, CertificationCandidate: true}, nil
}

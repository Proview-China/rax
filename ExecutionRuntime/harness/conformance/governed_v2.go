// Package conformance provides backend-neutral Harness contract checks. A
// passing report is a certification candidate only; it never grants Binding,
// production durability, dispatch, or completion authority.
package conformance

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type GovernedFactCaseV2 struct {
	Sessions   harnessports.SessionFactPortV2
	Candidates harnessports.CandidateFactPortV2
	Session    contract.GovernedSessionV2
	Candidate  contract.ModelTurnCandidateV2
}

type GovernedFactReportV2 struct {
	SessionCreateIdempotent   bool `json:"session_create_idempotent"`
	CandidateCreateIdempotent bool `json:"candidate_create_idempotent"`
	SessionCASLinearized      bool `json:"session_cas_linearized"`
	ExactInspectVerified      bool `json:"exact_inspect_verified"`
	CertificationCandidate    bool `json:"certification_candidate"`
	ProductionClaimEligible   bool `json:"production_claim_eligible"`
	DispatchEligible          bool `json:"dispatch_eligible"`
	CompletionEligible        bool `json:"completion_eligible"`
}

func CheckGovernedFactsV2(ctx context.Context, testCase GovernedFactCaseV2) (GovernedFactReportV2, error) {
	if testCase.Sessions == nil || testCase.Candidates == nil {
		return GovernedFactReportV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "session and candidate fact ports are required")
	}
	if err := testCase.Session.Validate(); err != nil {
		return GovernedFactReportV2{}, err
	}
	if testCase.Session.Phase != contract.SessionCreatingV2 || testCase.Session.Revision != 1 {
		return GovernedFactReportV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "conformance session must be a fresh creating fact")
	}
	if err := testCase.Candidate.Validate(timeZeroV2()); err != nil {
		return GovernedFactReportV2{}, err
	}

	createdSession, err := testCase.Sessions.CreateSessionV2(ctx, testCase.Session)
	if err != nil {
		return GovernedFactReportV2{}, err
	}
	replayedSession, err := testCase.Sessions.CreateSessionV2(ctx, testCase.Session)
	if err != nil || digestSessionV2(createdSession) != digestSessionV2(replayedSession) {
		return GovernedFactReportV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "session create is not content-idempotent")
	}
	createdCandidate, err := testCase.Candidates.CreateCandidateV2(ctx, testCase.Candidate)
	if err != nil {
		return GovernedFactReportV2{}, err
	}
	replayedCandidate, err := testCase.Candidates.CreateCandidateV2(ctx, testCase.Candidate)
	if err != nil {
		return GovernedFactReportV2{}, err
	}
	createdDigest, err := createdCandidate.DigestV2()
	if err != nil {
		return GovernedFactReportV2{}, err
	}
	replayedDigest, err := replayedCandidate.DigestV2()
	if err != nil || createdDigest != replayedDigest {
		return GovernedFactReportV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "candidate create is not content-idempotent")
	}
	ref, err := testCase.Candidate.RefV2()
	if err != nil {
		return GovernedFactReportV2{}, err
	}
	next := testCase.Session
	next.Revision++
	next.Phase = contract.SessionWaitingModelDispatchV2
	next.Turn = 1
	next.Candidate = &ref
	next.UpdatedUnixNano++
	transitioned, err := testCase.Sessions.CompareAndSwapSessionV2(ctx, harnessports.SessionCASRequestV2{ExpectedRevision: testCase.Session.Revision, Next: next})
	if err != nil {
		return GovernedFactReportV2{}, err
	}
	if _, err := testCase.Sessions.CompareAndSwapSessionV2(ctx, harnessports.SessionCASRequestV2{ExpectedRevision: testCase.Session.Revision, Next: next}); !core.HasCategory(err, core.ErrorConflict) {
		return GovernedFactReportV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "session CAS did not reject a consumed revision")
	}
	inspectedSession, err := testCase.Sessions.InspectSessionV2(ctx, testCase.Session.Run, testCase.Session.ID)
	if err != nil || digestSessionV2(inspectedSession) != digestSessionV2(transitioned) {
		return GovernedFactReportV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "session exact inspect differs from committed CAS")
	}
	inspectedCandidate, err := testCase.Candidates.InspectCandidateV2(ctx, testCase.Candidate.Run, testCase.Candidate.ID)
	if err != nil {
		return GovernedFactReportV2{}, err
	}
	inspectedDigest, err := inspectedCandidate.DigestV2()
	if err != nil || inspectedDigest != createdDigest {
		return GovernedFactReportV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "candidate exact inspect differs from create")
	}
	return GovernedFactReportV2{SessionCreateIdempotent: true, CandidateCreateIdempotent: true, SessionCASLinearized: true, ExactInspectVerified: true, CertificationCandidate: true, ProductionClaimEligible: false, DispatchEligible: false, CompletionEligible: false}, nil
}

func digestSessionV2(value contract.GovernedSessionV2) core.Digest {
	digest, _ := core.CanonicalJSONDigest("praxis.harness.governed", contract.GovernedContractVersionV2, "GovernedSessionV2", value)
	return digest
}

func timeZeroV2() (zero time.Time) { return zero }

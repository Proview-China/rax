package contract

import (
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type CaseStateV1 string

const (
	CaseRequestedV1       CaseStateV1 = "requested"
	CaseAdmittedV1        CaseStateV1 = "admitted"
	CaseRoutedV1          CaseStateV1 = "routed"
	CaseWaitingReviewerV1 CaseStateV1 = "waiting_reviewer"
	CaseReviewingV1       CaseStateV1 = "reviewing"
	CaseAttestedV1        CaseStateV1 = "attested"
	CaseDecidingV1        CaseStateV1 = "deciding"
	CaseWaitingRevisionV1 CaseStateV1 = "waiting_revision"
	CaseWaitingHumanV1    CaseStateV1 = "waiting_human"
	CaseWaitingEvidenceV1 CaseStateV1 = "waiting_evidence"
	CaseResolvedV1        CaseStateV1 = "resolved"
	CaseExpiredV1         CaseStateV1 = "expired"
	CaseRevokedV1         CaseStateV1 = "revoked"
	CaseSupersededV1      CaseStateV1 = "superseded"
	CaseCancelledV1       CaseStateV1 = "cancelled"
	CaseIndeterminateV1   CaseStateV1 = "indeterminate"
)

type ReviewCaseV1 struct {
	FactIdentityV1
	TargetID       string        `json:"target_id"`
	TargetRevision core.Revision `json:"target_revision"`
	TargetDigest   core.Digest   `json:"target_digest"`
	// Rubric is optional only when decoding historical V1 facts created before
	// exact Rubric binding was introduced. Every new Round write requires it.
	Rubric             *ExactResourceRefV1 `json:"rubric,omitempty"`
	State              CaseStateV1         `json:"state"`
	CurrentRoundID     string              `json:"current_round_id,omitempty"`
	CurrentAssignment  string              `json:"current_assignment_id,omitempty"`
	VerdictID          string              `json:"verdict_id,omitempty"`
	VerdictRevision    core.Revision       `json:"verdict_revision,omitempty"`
	VerdictDigest      core.Digest         `json:"verdict_digest,omitempty"`
	ExpiresUnixNano    int64               `json:"expires_unix_nano"`
	InvalidationReason core.ReasonCode     `json:"invalidation_reason,omitempty"`
}

func (c ReviewCaseV1) validateShape() error {
	if err := c.FactIdentityV1.ValidateShape(); err != nil {
		return err
	}
	if invalidID(c.TargetID) || c.TargetRevision == 0 || c.TargetDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewCandidateConflict, "review case target is incomplete")
	}
	if c.Rubric != nil {
		if err := c.Rubric.Validate(); err != nil {
			return err
		}
	}
	if err := ValidateCaseStateV1(c.State); err != nil {
		return err
	}
	if err := ValidateExpires(c.CreatedUnixNano, c.ExpiresUnixNano); err != nil {
		return err
	}
	if c.State == CaseResolvedV1 {
		if invalidID(c.VerdictID) || c.VerdictRevision == 0 || c.VerdictDigest.Validate() != nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictMissing, "resolved review case requires exact verdict")
		}
	} else if c.VerdictID != "" || c.VerdictRevision != 0 || c.VerdictDigest != "" {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "non-resolved review case cannot claim a verdict")
	}
	if terminalCaseState(c.State) && c.State != CaseResolvedV1 && c.InvalidationReason == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "invalidated review case requires a reason")
	}
	return nil
}

func (c ReviewCaseV1) digestValue() ReviewCaseV1 { c.Digest = ""; return c }

func SealReviewCaseV1(c ReviewCaseV1) (ReviewCaseV1, error) {
	c.ContractVersion = ContractVersionV1
	c.Digest = ""
	if err := c.validateShape(); err != nil {
		return ReviewCaseV1{}, err
	}
	digest, err := seal("ReviewCaseV1", c.digestValue())
	if err != nil {
		return ReviewCaseV1{}, err
	}
	c.Digest = digest
	return c, c.Validate()
}

func (c ReviewCaseV1) Validate() error {
	if err := c.validateShape(); err != nil {
		return err
	}
	return validateSealed("ReviewCaseV1", c.digestValue(), c.Digest)
}

func ValidateCaseStateV1(s CaseStateV1) error {
	switch s {
	case CaseRequestedV1, CaseAdmittedV1, CaseRoutedV1, CaseWaitingReviewerV1, CaseReviewingV1, CaseAttestedV1, CaseDecidingV1, CaseWaitingRevisionV1, CaseWaitingHumanV1, CaseWaitingEvidenceV1, CaseResolvedV1, CaseExpiredV1, CaseRevokedV1, CaseSupersededV1, CaseCancelledV1, CaseIndeterminateV1:
		return nil
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "unknown review case state")
	}
}

func CanTransitionCaseV1(from, to CaseStateV1) bool {
	if from == to || terminalCaseState(from) {
		return false
	}
	if to == CaseExpiredV1 || to == CaseRevokedV1 || to == CaseSupersededV1 || to == CaseCancelledV1 || to == CaseIndeterminateV1 {
		return true
	}
	allowed := map[CaseStateV1]map[CaseStateV1]bool{
		CaseRequestedV1:       {CaseAdmittedV1: true},
		CaseAdmittedV1:        {CaseRoutedV1: true},
		CaseRoutedV1:          {CaseWaitingReviewerV1: true, CaseReviewingV1: true},
		CaseWaitingReviewerV1: {CaseReviewingV1: true},
		CaseReviewingV1:       {CaseAttestedV1: true, CaseWaitingRevisionV1: true, CaseWaitingHumanV1: true, CaseWaitingEvidenceV1: true},
		CaseAttestedV1:        {CaseDecidingV1: true, CaseWaitingRevisionV1: true, CaseWaitingHumanV1: true, CaseWaitingEvidenceV1: true},
		CaseDecidingV1:        {CaseResolvedV1: true, CaseWaitingRevisionV1: true, CaseWaitingHumanV1: true, CaseWaitingEvidenceV1: true},
		CaseWaitingRevisionV1: {CaseRoutedV1: true},
		CaseWaitingHumanV1:    {CaseRoutedV1: true},
		CaseWaitingEvidenceV1: {CaseDecidingV1: true, CaseRoutedV1: true},
		CaseIndeterminateV1:   {CaseAdmittedV1: true, CaseRoutedV1: true, CaseWaitingReviewerV1: true, CaseReviewingV1: true, CaseAttestedV1: true, CaseDecidingV1: true},
	}
	return allowed[from][to]
}

func terminalCaseState(s CaseStateV1) bool {
	return s == CaseResolvedV1 || s == CaseExpiredV1 || s == CaseRevokedV1 || s == CaseSupersededV1 || s == CaseCancelledV1
}

type RouteV1 string

const (
	RouteHumanV1 RouteV1 = "human"
	RouteAutoV1  RouteV1 = "auto"
)

type RoundStateV1 string

const (
	RoundPreparedV1       RoundStateV1 = "prepared"
	RoundAdmittedV1       RoundStateV1 = "admitted"
	RoundDeliveredV1      RoundStateV1 = "delivered"
	RoundObservedV1       RoundStateV1 = "observed"
	RoundInspectedV1      RoundStateV1 = "inspected"
	RoundAttestedV1       RoundStateV1 = "attested"
	RoundTerminatedV1     RoundStateV1 = "terminated"
	RoundUnknownOutcomeV1 RoundStateV1 = "unknown_outcome"
)

type ReviewRoundV1 struct {
	FactIdentityV1
	CaseID             string        `json:"case_id"`
	CaseRevision       core.Revision `json:"case_revision"`
	TargetID           string        `json:"target_id"`
	TargetRevision     core.Revision `json:"target_revision"`
	TargetDigest       core.Digest   `json:"target_digest"`
	Route              RouteV1       `json:"route"`
	State              RoundStateV1  `json:"state"`
	AssignmentID       string        `json:"assignment_id"`
	ContextFrameDigest core.Digest   `json:"context_frame_digest"`
	// Rubric is optional only for historical decoding. New Round publication
	// requires the full exact ref and RubricDigest must equal Rubric.Digest.
	Rubric            *ExactResourceRefV1 `json:"rubric,omitempty"`
	RubricDigest      core.Digest         `json:"rubric_digest"`
	TerminationReason string              `json:"termination_reason,omitempty"`
	ExpiresUnixNano   int64               `json:"expires_unix_nano"`
}

func (r ReviewRoundV1) digestValue() ReviewRoundV1 { r.Digest = ""; return r }
func (r ReviewRoundV1) validateShape() error {
	if err := r.FactIdentityV1.ValidateShape(); err != nil {
		return err
	}
	if invalidID(r.CaseID) || r.CaseRevision == 0 || invalidID(r.TargetID) || r.TargetRevision == 0 || invalidID(r.AssignmentID) || r.TargetDigest.Validate() != nil || r.ContextFrameDigest.Validate() != nil || r.RubricDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review round is incomplete")
	}
	if r.Rubric != nil {
		if err := r.Rubric.Validate(); err != nil {
			return err
		}
		if r.Rubric.Digest != r.RubricDigest {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "review round Rubric exact ref and digest drifted")
		}
	}
	if r.Route != RouteHumanV1 && r.Route != RouteAutoV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "review round route is unsupported")
	}
	switch r.State {
	case RoundPreparedV1, RoundAdmittedV1, RoundDeliveredV1, RoundObservedV1, RoundInspectedV1, RoundAttestedV1, RoundTerminatedV1, RoundUnknownOutcomeV1:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "review round state is unsupported")
	}
	if r.State == RoundTerminatedV1 && blank(r.TerminationReason) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "terminated review round requires a reason")
	}
	return ValidateExpires(r.CreatedUnixNano, r.ExpiresUnixNano)
}
func SealReviewRoundV1(r ReviewRoundV1) (ReviewRoundV1, error) {
	r.ContractVersion = ContractVersionV1
	r.Digest = ""
	if err := r.validateShape(); err != nil {
		return ReviewRoundV1{}, err
	}
	d, err := seal("ReviewRoundV1", r.digestValue())
	if err != nil {
		return ReviewRoundV1{}, err
	}
	r.Digest = d
	return r, r.Validate()
}
func (r ReviewRoundV1) Validate() error {
	if err := r.validateShape(); err != nil {
		return err
	}
	return validateSealed("ReviewRoundV1", r.digestValue(), r.Digest)
}

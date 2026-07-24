package contract

import (
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const CommittedPendingActionApplicabilityBindingContractVersionV1 = "praxis.harness.committed-pending-action-applicability-binding/v1"

// CommittedPendingActionApplicabilityBindingV1 is immutable, request-scoped
// adapter configuration. It is not a Fact, authority, Evidence, Permit, or an
// Application DTO.
type CommittedPendingActionApplicabilityBindingV1 struct {
	ContractVersion           string                                                 `json:"contract_version"`
	Subject                   CommittedPendingActionSubjectV1                        `json:"subject"`
	ExpectedSessionCoordinate CommittedPendingActionSessionApplicabilityCoordinateV1 `json:"expected_session_coordinate"`
	ExpectedTurnCoordinate    CommittedPendingActionTurnApplicabilityCoordinateV1    `json:"expected_turn_coordinate"`
	Digest                    core.Digest                                            `json:"digest"`
}

func (b CommittedPendingActionApplicabilityBindingV1) Clone() CommittedPendingActionApplicabilityBindingV1 {
	clone := b
	clone.Subject = b.Subject.Clone()
	return clone
}

func (b CommittedPendingActionApplicabilityBindingV1) Validate() error {
	if b.ContractVersion != CommittedPendingActionApplicabilityBindingContractVersionV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "committed PendingAction applicability Binding version is invalid")
	}
	if err := b.Subject.Validate(); err != nil {
		return err
	}
	if err := b.ExpectedSessionCoordinate.Validate(); err != nil {
		return err
	}
	if err := b.ExpectedTurnCoordinate.Validate(); err != nil {
		return err
	}
	if b.ExpectedSessionCoordinate.Revision != b.Subject.ExpectedSessionRevision || b.ExpectedTurnCoordinate.Revision != b.Subject.ExpectedSessionRevision {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "committed PendingAction applicability Binding revision drifted")
	}
	digest, err := b.DigestV1()
	if err != nil || digest != b.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "committed PendingAction applicability Binding digest drifted")
	}
	return nil
}

func (b CommittedPendingActionApplicabilityBindingV1) DigestV1() (core.Digest, error) {
	clone := b.Clone()
	clone.Digest = ""
	return core.CanonicalJSONDigest(
		"praxis.harness.committed-pending-action-applicability-binding",
		CommittedPendingActionApplicabilityBindingContractVersionV1,
		"CommittedPendingActionApplicabilityBindingV1",
		clone,
	)
}

func SealCommittedPendingActionApplicabilityBindingV1(
	request InspectCommittedPendingActionCurrentRequestV1,
	session CommittedPendingActionSessionApplicabilityCoordinateV1,
	turn CommittedPendingActionTurnApplicabilityCoordinateV1,
) (CommittedPendingActionApplicabilityBindingV1, error) {
	binding := CommittedPendingActionApplicabilityBindingV1{
		ContractVersion:           CommittedPendingActionApplicabilityBindingContractVersionV1,
		Subject:                   request.SubjectV1(),
		ExpectedSessionCoordinate: session,
		ExpectedTurnCoordinate:    turn,
	}
	var err error
	binding.Digest, err = binding.DigestV1()
	if err != nil {
		return CommittedPendingActionApplicabilityBindingV1{}, err
	}
	return binding, binding.Validate()
}

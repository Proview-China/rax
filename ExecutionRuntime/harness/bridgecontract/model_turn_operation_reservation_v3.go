package bridgecontract

import (
	"strings"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	harnesscontract "github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const ModelTurnOperationReservationContractV3 = "praxis.harness.model-turn-operation-reservation/v3"

// ModelTurnOperationReservationFactV3 is the Harness-owned create-once index
// from an Application attempt to the exact Session/Candidate subject it won.
// The Session CAS remains the linearization point for subject uniqueness.
type ModelTurnOperationReservationFactV3 struct {
	ContractVersion string                                              `json:"contract_version"`
	Scope           core.ExecutionScope                                 `json:"scope"`
	StepKind        runtimeports.NamespacedNameV2                       `json:"step_kind"`
	Run             harnesscontract.RunRef                              `json:"run"`
	SessionID       string                                              `json:"session_id"`
	SessionRevision core.Revision                                       `json:"session_revision"`
	Candidate       harnesscontract.CandidateRefV2                      `json:"candidate"`
	Application     applicationcontract.GovernedOperationAttemptRefV3   `json:"application_attempt"`
	Reservation     applicationcontract.OperationDomainReservationRefV3 `json:"reservation"`
}

func (f ModelTurnOperationReservationFactV3) Validate() error {
	if f.ContractVersion != ModelTurnOperationReservationContractV3 || strings.TrimSpace(f.SessionID) == "" || len(f.SessionID) > harnesscontract.MaxReferenceBytes || f.SessionRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "model-turn reservation identity is incomplete")
	}
	if err := f.Scope.Validate(); err != nil {
		return err
	}
	if runtimeports.ValidateNamespacedNameV2(f.StepKind) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidNamespace, "model-turn reservation StepKind must be namespaced")
	}
	if err := f.Run.Validate(); err != nil {
		return err
	}
	if !runtimeports.SameExecutionScopeV2(f.Scope, f.Run.Scope) {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "model-turn reservation Run belongs to another scope")
	}
	if err := f.Candidate.Validate(); err != nil {
		return err
	}
	if err := f.Application.Validate(); err != nil {
		return err
	}
	if err := f.Reservation.Validate(); err != nil {
		return err
	}
	if f.Application.State != applicationcontract.OperationIntentRecordedV3 || f.Application.Revision != 1 || f.Application.DomainReservation != nil || f.Application.ID != f.Reservation.AttemptID || f.Application.Digest != f.Reservation.AttemptDigest || f.StepKind != f.Application.StepKind || f.StepKind != f.Reservation.StepKind || f.Application.DomainAdapter != f.Reservation.DomainAdapter || f.SessionID != f.Reservation.SessionRef || f.Candidate.Digest != f.Reservation.CandidateDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "model-turn reservation changed its exact Application, Session or Candidate")
	}
	return nil
}

func (f ModelTurnOperationReservationFactV3) DigestV3() (core.Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.harness.model-turn-operation-reservation", ModelTurnOperationReservationContractV3, "ModelTurnOperationReservationFactV3", f)
}

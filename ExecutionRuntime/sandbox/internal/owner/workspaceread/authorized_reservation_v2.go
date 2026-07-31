package workspaceread

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

const authorizedReservationContractVersionV2 = "praxis.sandbox/workspace-read-authorized-reservation/v2"

// AuthorizedReservationV2 is an internal Sandbox-owner capability envelope.
// Its fields are deliberately private: public consumers may inspect the V2
// historical binding, but cannot submit owner facts to the SQLite writer.
type AuthorizedReservationV2 struct {
	reservation   contract.WorkspaceReadReservationV1
	attempt       contract.WorkspaceReadAttemptV1
	binding       sandboxports.WorkspaceReadAdmissionAttemptBindingV1
	authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3
	checkedNano   int64
	seal          runtimecore.Digest
}

// NewAuthorizedReservationV2 is the owner-private issuance seam used by the
// Sandbox kernel only after it has completed its S1 current closure. It does
// not replace or weaken the kernel's current-reader checks.
func NewAuthorizedReservationV2(
	reservation contract.WorkspaceReadReservationV1,
	attempt contract.WorkspaceReadAttemptV1,
	binding sandboxports.WorkspaceReadAdmissionAttemptBindingV1,
	authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3,
	checked time.Time,
) (AuthorizedReservationV2, error) {
	if checked.IsZero() {
		return AuthorizedReservationV2{}, errors.New("workspace read owner capability clock is required")
	}
	if err := reservation.ValidateCurrent(checked); err != nil {
		return AuthorizedReservationV2{}, err
	}
	if err := attempt.ValidateCurrent(checked); err != nil {
		return AuthorizedReservationV2{}, err
	}
	if err := binding.Validate(); err != nil {
		return AuthorizedReservationV2{}, err
	}
	if err := authorization.ValidateCurrent(checked); err != nil {
		return AuthorizedReservationV2{}, err
	}
	reservationClone, err := cloneJSON(reservation)
	if err != nil {
		return AuthorizedReservationV2{}, err
	}
	attemptClone, err := cloneJSON(attempt)
	if err != nil {
		return AuthorizedReservationV2{}, err
	}
	bindingClone, err := cloneJSON(binding)
	if err != nil {
		return AuthorizedReservationV2{}, err
	}
	authorizationClone, err := cloneJSON(authorization)
	if err != nil {
		return AuthorizedReservationV2{}, err
	}
	value := AuthorizedReservationV2{
		reservation:   reservationClone,
		attempt:       attemptClone,
		binding:       bindingClone,
		authorization: authorizationClone,
		checkedNano:   checked.UnixNano(),
	}
	seal, err := value.digest()
	if err != nil {
		return AuthorizedReservationV2{}, err
	}
	value.seal = seal
	return value, nil
}

// Open validates the nominal capability and returns detached copies to the
// Sandbox-owned store. It is not a public execution authorization.
func (value AuthorizedReservationV2) Open(
	now time.Time,
) (
	contract.WorkspaceReadReservationV1,
	contract.WorkspaceReadAttemptV1,
	sandboxports.WorkspaceReadAdmissionAttemptBindingV1,
	runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3,
	error,
) {
	if now.IsZero() || value.checkedNano <= 0 || now.UnixNano() < value.checkedNano {
		return contract.WorkspaceReadReservationV1{}, contract.WorkspaceReadAttemptV1{}, sandboxports.WorkspaceReadAdmissionAttemptBindingV1{}, runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3{}, errors.New("workspace read owner capability clock regressed")
	}
	expected, err := value.digest()
	if err != nil || expected != value.seal {
		return contract.WorkspaceReadReservationV1{}, contract.WorkspaceReadAttemptV1{}, sandboxports.WorkspaceReadAdmissionAttemptBindingV1{}, runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3{}, errors.New("workspace read owner capability is invalid")
	}
	if err = value.reservation.ValidateCurrent(now); err != nil {
		return contract.WorkspaceReadReservationV1{}, contract.WorkspaceReadAttemptV1{}, sandboxports.WorkspaceReadAdmissionAttemptBindingV1{}, runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3{}, err
	}
	if err = value.attempt.ValidateCurrent(now); err != nil {
		return contract.WorkspaceReadReservationV1{}, contract.WorkspaceReadAttemptV1{}, sandboxports.WorkspaceReadAdmissionAttemptBindingV1{}, runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3{}, err
	}
	if err = value.binding.Validate(); err != nil {
		return contract.WorkspaceReadReservationV1{}, contract.WorkspaceReadAttemptV1{}, sandboxports.WorkspaceReadAdmissionAttemptBindingV1{}, runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3{}, err
	}
	if err = value.authorization.ValidateCurrent(now); err != nil {
		return contract.WorkspaceReadReservationV1{}, contract.WorkspaceReadAttemptV1{}, sandboxports.WorkspaceReadAdmissionAttemptBindingV1{}, runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3{}, err
	}
	reservation, err := cloneJSON(value.reservation)
	if err != nil {
		return contract.WorkspaceReadReservationV1{}, contract.WorkspaceReadAttemptV1{}, sandboxports.WorkspaceReadAdmissionAttemptBindingV1{}, runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3{}, err
	}
	attempt, err := cloneJSON(value.attempt)
	if err != nil {
		return contract.WorkspaceReadReservationV1{}, contract.WorkspaceReadAttemptV1{}, sandboxports.WorkspaceReadAdmissionAttemptBindingV1{}, runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3{}, err
	}
	binding, err := cloneJSON(value.binding)
	if err != nil {
		return contract.WorkspaceReadReservationV1{}, contract.WorkspaceReadAttemptV1{}, sandboxports.WorkspaceReadAdmissionAttemptBindingV1{}, runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3{}, err
	}
	authorization, err := cloneJSON(value.authorization)
	if err != nil {
		return contract.WorkspaceReadReservationV1{}, contract.WorkspaceReadAttemptV1{}, sandboxports.WorkspaceReadAdmissionAttemptBindingV1{}, runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3{}, err
	}
	return reservation, attempt, binding, authorization, nil
}

func (value AuthorizedReservationV2) digest() (runtimecore.Digest, error) {
	return runtimecore.CanonicalJSONDigest(
		"praxis.sandbox.workspace-read-owner",
		authorizedReservationContractVersionV2,
		"AuthorizedReservationV2",
		struct {
			Reservation   contract.WorkspaceReadReservationV1                              `json:"reservation"`
			Attempt       contract.WorkspaceReadAttemptV1                                  `json:"attempt"`
			Binding       sandboxports.WorkspaceReadAdmissionAttemptBindingV1              `json:"binding"`
			Authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3 `json:"authorization"`
			CheckedNano   int64                                                            `json:"checked_unix_nano"`
		}{
			Reservation: value.reservation, Attempt: value.attempt, Binding: value.binding,
			Authorization: value.authorization, CheckedNano: value.checkedNano,
		},
	)
}

func cloneJSON[T any](value T) (T, error) {
	var clone T
	body, err := json.Marshal(value)
	if err != nil {
		return clone, fmt.Errorf("%w: workspace read owner capability clone marshal failed: %v", sandboxports.ErrConflict, err)
	}
	if err = json.Unmarshal(body, &clone); err != nil {
		return clone, fmt.Errorf("%w: workspace read owner capability clone unmarshal failed: %v", sandboxports.ErrConflict, err)
	}
	return clone, nil
}

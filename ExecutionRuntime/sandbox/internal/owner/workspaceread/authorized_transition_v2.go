package workspaceread

import (
	"errors"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

const authorizedTransitionContractVersionV2 = "praxis.sandbox/workspace-read-authorized-transition/v2"

type authorizedTransitionKindV2 string

const (
	authorizedTransitionObservedV2 authorizedTransitionKindV2 = "observed"
	authorizedTransitionUnknownV2  authorizedTransitionKindV2 = "indeterminate"
	authorizedTransitionFailedV2   authorizedTransitionKindV2 = "failed"
)

// AuthorizedTransitionV2 is a Sandbox-owner capability. Its private body can
// only be issued by this internal package after the kernel has retained the
// original Runtime authorization through the physical boundary. Public V1
// Store methods never receive this capability and therefore cannot advance a
// Started attempt.
type AuthorizedTransitionV2 struct {
	attempt       contract.WorkspaceReadAttemptRefV1
	authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3
	kind          authorizedTransitionKindV2
	observation   *contract.WorkspaceReadObservationV1
	unknown       string
	failure       string
	checkedNano   int64
	seal          runtimecore.Digest
}

// AuthorizedExecutionV2 is issued once, while Runtime authorization and the
// original Sandbox Attempt are still current, before the physical actual
// point. Later transition sealing never refreshes that authorization.
type AuthorizedExecutionV2 struct {
	attempt       contract.WorkspaceReadAttemptRefV1
	authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3
	checkedNano   int64
	seal          runtimecore.Digest
}

func NewAuthorizedExecutionV2(
	attempt contract.WorkspaceReadAttemptRefV1,
	authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3,
	checked time.Time,
) (AuthorizedExecutionV2, error) {
	if checked.IsZero() || attempt.Validate() != nil || authorization.ValidateCurrent(checked) != nil {
		return AuthorizedExecutionV2{}, errors.New("workspace read authorized execution closure is invalid")
	}
	clone, err := cloneJSON(authorization)
	if err != nil {
		return AuthorizedExecutionV2{}, err
	}
	value := AuthorizedExecutionV2{attempt: attempt, authorization: clone, checkedNano: checked.UnixNano()}
	seal, err := value.digest()
	if err != nil {
		return AuthorizedExecutionV2{}, err
	}
	value.seal = seal
	return value, nil
}

func (value AuthorizedExecutionV2) Observed(
	observation contract.WorkspaceReadObservationV1,
	checked time.Time,
) (AuthorizedTransitionV2, error) {
	if err := value.validate(); err != nil {
		return AuthorizedTransitionV2{}, err
	}
	transition := AuthorizedTransitionV2{
		attempt: value.attempt, authorization: value.authorization,
		kind: authorizedTransitionObservedV2, observation: &observation,
		checkedNano: checked.UnixNano(),
	}
	return sealAuthorizedTransitionV2(transition, checked, value.checkedNano)
}

func (value AuthorizedExecutionV2) Unknown(
	unknown string,
	checked time.Time,
) (AuthorizedTransitionV2, error) {
	if err := value.validate(); err != nil {
		return AuthorizedTransitionV2{}, err
	}
	transition := AuthorizedTransitionV2{
		attempt: value.attempt, authorization: value.authorization,
		kind: authorizedTransitionUnknownV2, unknown: unknown,
		checkedNano: checked.UnixNano(),
	}
	return sealAuthorizedTransitionV2(transition, checked, value.checkedNano)
}

func (value AuthorizedExecutionV2) Failed(
	failure string,
	checked time.Time,
) (AuthorizedTransitionV2, error) {
	if err := value.validate(); err != nil {
		return AuthorizedTransitionV2{}, err
	}
	transition := AuthorizedTransitionV2{
		attempt: value.attempt, authorization: value.authorization,
		kind: authorizedTransitionFailedV2, failure: failure,
		checkedNano: checked.UnixNano(),
	}
	return sealAuthorizedTransitionV2(transition, checked, value.checkedNano)
}

func sealAuthorizedTransitionV2(value AuthorizedTransitionV2, checked time.Time, issuedNano int64) (AuthorizedTransitionV2, error) {
	if checked.IsZero() || value.checkedNano < issuedNano || value.attempt.Validate() != nil || value.authorization.Validate() != nil {
		return AuthorizedTransitionV2{}, errors.New("workspace read authorized transition closure is invalid")
	}
	switch value.kind {
	case authorizedTransitionObservedV2:
		if value.observation == nil || value.observation.ValidateCurrent(checked) != nil || value.unknown != "" || value.failure != "" {
			return AuthorizedTransitionV2{}, errors.New("workspace read observed transition lacks a current provider observation")
		}
		clone, err := cloneJSON(*value.observation)
		if err != nil {
			return AuthorizedTransitionV2{}, err
		}
		value.observation = &clone
	case authorizedTransitionUnknownV2:
		if value.observation != nil || !contract.ValidDigest(value.unknown) || value.failure != "" {
			return AuthorizedTransitionV2{}, errors.New("workspace read unknown transition is invalid")
		}
	case authorizedTransitionFailedV2:
		if value.observation != nil || value.unknown != "" || !contract.ValidDigest(value.failure) {
			return AuthorizedTransitionV2{}, errors.New("workspace read failed transition is invalid")
		}
	default:
		return AuthorizedTransitionV2{}, errors.New("workspace read authorized transition kind is invalid")
	}
	authorization, err := cloneJSON(value.authorization)
	if err != nil {
		return AuthorizedTransitionV2{}, err
	}
	value.authorization = authorization
	seal, err := value.digest()
	if err != nil {
		return AuthorizedTransitionV2{}, err
	}
	value.seal = seal
	return value, nil
}

// Open validates the nominal capability at the Store mutation clock and
// returns detached values. The capability itself is not a Runtime permit and
// cannot refresh any expiry.
func (value AuthorizedTransitionV2) Open(
	now time.Time,
) (
	contract.WorkspaceReadAttemptRefV1,
	runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3,
	*contract.WorkspaceReadObservationV1,
	string,
	string,
	error,
) {
	if now.IsZero() || value.checkedNano <= 0 || now.UnixNano() < value.checkedNano {
		return contract.WorkspaceReadAttemptRefV1{}, runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3{}, nil, "", "", errors.New("workspace read authorized transition clock regressed")
	}
	expected, err := value.digest()
	if err != nil || expected != value.seal || value.attempt.Validate() != nil || value.authorization.Validate() != nil {
		return contract.WorkspaceReadAttemptRefV1{}, runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3{}, nil, "", "", errors.New("workspace read authorized transition is invalid")
	}
	authorization, err := cloneJSON(value.authorization)
	if err != nil {
		return contract.WorkspaceReadAttemptRefV1{}, runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3{}, nil, "", "", err
	}
	var observation *contract.WorkspaceReadObservationV1
	if value.observation != nil {
		if value.observation.ValidateCurrent(now) != nil {
			return contract.WorkspaceReadAttemptRefV1{}, runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3{}, nil, "", "", errors.New("workspace read authorized provider observation expired")
		}
		clone, cloneErr := cloneJSON(*value.observation)
		if cloneErr != nil {
			return contract.WorkspaceReadAttemptRefV1{}, runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3{}, nil, "", "", cloneErr
		}
		observation = &clone
	}
	return value.attempt, authorization, observation, value.unknown, value.failure, nil
}

func (value AuthorizedExecutionV2) validate() error {
	if value.checkedNano <= 0 || value.attempt.Validate() != nil || value.authorization.Validate() != nil {
		return errors.New("workspace read authorized execution is invalid")
	}
	expected, err := value.digest()
	if err != nil || expected != value.seal {
		return errors.New("workspace read authorized execution seal drifted")
	}
	return nil
}

func (value AuthorizedExecutionV2) digest() (runtimecore.Digest, error) {
	return runtimecore.CanonicalJSONDigest(
		"praxis.sandbox.workspace-read-owner",
		authorizedTransitionContractVersionV2,
		"AuthorizedExecutionV2",
		struct {
			Attempt       contract.WorkspaceReadAttemptRefV1                               `json:"attempt"`
			Authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3 `json:"authorization"`
			CheckedNano   int64                                                            `json:"checked_unix_nano"`
		}{value.attempt, value.authorization, value.checkedNano},
	)
}

func (value AuthorizedTransitionV2) digest() (runtimecore.Digest, error) {
	return runtimecore.CanonicalJSONDigest(
		"praxis.sandbox.workspace-read-owner",
		authorizedTransitionContractVersionV2,
		"AuthorizedTransitionV2",
		struct {
			Attempt       contract.WorkspaceReadAttemptRefV1                               `json:"attempt"`
			Authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3 `json:"authorization"`
			Kind          authorizedTransitionKindV2                                       `json:"kind"`
			Observation   *contract.WorkspaceReadObservationV1                             `json:"observation,omitempty"`
			Unknown       string                                                           `json:"unknown,omitempty"`
			Failure       string                                                           `json:"failure,omitempty"`
			CheckedNano   int64                                                            `json:"checked_unix_nano"`
		}{
			Attempt: value.attempt, Authorization: value.authorization, Kind: value.kind,
			Observation: value.observation, Unknown: value.unknown, Failure: value.failure,
			CheckedNano: value.checkedNano,
		},
	)
}

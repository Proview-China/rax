package ports

import (
	"context"
	"errors"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

var ErrWorkspaceReadUnknown = errors.New("workspace read outcome is indeterminate; inspect the original attempt")

const WorkspaceReadAdmissionAttemptBindingContractVersionV1 = "praxis.sandbox/workspace-read-admission-attempt-binding/v1"

// WorkspaceReadAdmissionAttemptBindingV1 is an immutable Sandbox-owned
// historical handoff. StableKeyDigest and AuthorizationDigest are sealed
// evidence only; the sole lookup coordinate is the complete exact Runtime
// AdmissionReceipt.
type WorkspaceReadAdmissionAttemptBindingV1 struct {
	ContractVersion     string                                                        `json:"contract_version"`
	AdmissionReceipt    runtimeports.ControlledOperationProviderAdmissionReceiptRefV2 `json:"admission_receipt"`
	Attempt             contract.WorkspaceReadAttemptRefV1                            `json:"attempt"`
	Command             contract.Ref                                                  `json:"command"`
	AuthorizationDigest runtimecore.Digest                                            `json:"authorization_digest"`
	StableKeyDigest     runtimecore.Digest                                            `json:"stable_key_digest"`
	Association         runtimeports.PreparedDomainCommandAssociationRefV1            `json:"association"`
	DomainCommand       runtimeports.OperationDomainCommandRefV1                      `json:"domain_command"`
	CreatedUnixNano     int64                                                         `json:"created_unix_nano"`
	ExpiresUnixNano     int64                                                         `json:"expires_unix_nano"`
	Digest              string                                                        `json:"digest"`
}

func (b WorkspaceReadAdmissionAttemptBindingV1) Validate() error {
	if b.ContractVersion != WorkspaceReadAdmissionAttemptBindingContractVersionV1 ||
		b.AdmissionReceipt.Validate() != nil ||
		!b.AdmissionReceipt.Admitted ||
		b.AdmissionReceipt.NoEffect ||
		b.Attempt.Validate() != nil ||
		b.Command.ValidateShape("workspace read command") != nil ||
		b.AuthorizationDigest.Validate() != nil ||
		b.StableKeyDigest.Validate() != nil ||
		b.AdmissionReceipt.StableKeyDigest != b.StableKeyDigest ||
		b.Association.Validate() != nil ||
		b.DomainCommand.Validate() != nil ||
		b.CreatedUnixNano <= 0 ||
		b.ExpiresUnixNano <= b.CreatedUnixNano ||
		!contract.ValidDigest(b.Digest) {
		return errors.New("workspace read admission-to-attempt binding is incomplete")
	}
	copy := b
	copy.Digest = ""
	digest, err := contract.Digest("workspace-read-admission-attempt-binding", copy)
	if err != nil || digest != b.Digest {
		return errors.New("workspace read admission-to-attempt binding digest drifted")
	}
	return nil
}

func SealWorkspaceReadAdmissionAttemptBindingV1(b WorkspaceReadAdmissionAttemptBindingV1) (WorkspaceReadAdmissionAttemptBindingV1, error) {
	b.ContractVersion = WorkspaceReadAdmissionAttemptBindingContractVersionV1
	b.Digest = ""
	digest, err := contract.Digest("workspace-read-admission-attempt-binding", b)
	if err != nil {
		return WorkspaceReadAdmissionAttemptBindingV1{}, err
	}
	b.Digest = digest
	return b, b.Validate()
}

type WorkspaceReadCommandCurrentReaderV1 interface {
	InspectWorkspaceReadCommandCurrentV1(context.Context, contract.Ref) (contract.WorkspaceReadCommandV1, error)
}

// WorkspaceReadPublishedCommandCurrentReaderV2 is the only current Command
// qualification accepted by the physical executor. Implementations must join
// the immutable Command, V2 Publication, authoritative OwnerCurrent pointer,
// and fresh upstream projections. A raw V1 row never satisfies this port.
type WorkspaceReadPublishedCommandCurrentReaderV2 interface {
	InspectWorkspaceReadPublishedCommandCurrentV2(context.Context, contract.Ref) (contract.WorkspaceReadCommandV1, error)
}

// WorkspaceReadCommandExactReaderV1 reads one immutable Sandbox-owned Command
// by its complete exact Ref. It is a historical reader: implementations must
// validate the stored shape and exact coordinate, but must not require the
// Command to remain current or renew any execution lifetime.
type WorkspaceReadCommandExactReaderV1 interface {
	InspectWorkspaceReadCommandExactV1(context.Context, contract.Ref) (contract.WorkspaceReadCommandV1, error)
}

type WorkspaceReadAdmissionAttemptReaderV1 interface {
	InspectWorkspaceReadAttemptForAdmissionV1(context.Context, runtimeports.ControlledOperationProviderAdmissionReceiptRefV2) (WorkspaceReadAdmissionAttemptBindingV1, error)
}

type WorkspaceReadAttemptCurrentReaderV1 interface {
	InspectWorkspaceReadAttemptCurrentV1(context.Context, contract.WorkspaceReadAttemptRefV1) (contract.WorkspaceReadAttemptV1, error)
}

type WorkspaceReadReservationExactReaderV1 interface {
	InspectWorkspaceReadReservationExactV1(context.Context, contract.Ref) (contract.WorkspaceReadReservationV1, error)
}

type WorkspaceReadOwnerStoreV1 interface {
	WorkspaceReadAdmissionAttemptReaderV1
	ReserveWorkspaceReadV1(context.Context, contract.WorkspaceReadReservationV1, contract.WorkspaceReadAttemptV1, WorkspaceReadAdmissionAttemptBindingV1) (contract.WorkspaceReadExecutionProjectionV1, bool, error)
	CompleteWorkspaceReadV1(context.Context, contract.Ref, contract.WorkspaceReadObservationV1) (contract.WorkspaceReadExecutionProjectionV1, error)
	MarkWorkspaceReadUnknownV1(context.Context, contract.Ref, string) (contract.WorkspaceReadExecutionProjectionV1, error)
	FailWorkspaceReadV1(context.Context, contract.Ref, string) (contract.WorkspaceReadExecutionProjectionV1, error)
	RecoverStartedWorkspaceReadAfterRestartV1(context.Context, contract.WorkspaceReadAttemptRefV1) (contract.WorkspaceReadExecutionProjectionV1, error)
	InspectBoundedWorkspaceReadV1(context.Context, contract.WorkspaceReadAttemptRefV1) (contract.WorkspaceReadExecutionProjectionV1, error)
}

type WorkspaceReadExecutionPortV1 interface {
	runtimeports.ControlledOperationPhysicalExecutionPortV3
	WorkspaceReadAdmissionAttemptReaderV1
	InspectBoundedWorkspaceReadV1(context.Context, contract.WorkspaceReadAttemptRefV1) (contract.WorkspaceReadExecutionProjectionV1, error)
}

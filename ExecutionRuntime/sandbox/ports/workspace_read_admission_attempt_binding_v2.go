package ports

import (
	"context"
	"errors"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

const (
	WorkspaceReadAdmissionAttemptBindingContractVersionV2 = "praxis.sandbox/workspace-read-admission-attempt-binding/v2"
	WorkspaceReadAdmissionAttemptBindingTypeURLV2         = "type.googleapis.com/praxis.sandbox.WorkspaceReadAdmissionAttemptBindingV2"
)

// WorkspaceReadAdmissionAttemptBindingV2 is an immutable Sandbox-owned
// historical proof from one exact Runtime dispatch Attempt to the Sandbox
// Admission and original workspace-read Attempt. It grants no current or
// execution authority.
type WorkspaceReadAdmissionAttemptBindingV2 struct {
	ContractVersion      string                                                        `json:"contract_version"`
	TypeURL              string                                                        `json:"type_url"`
	RuntimeAttempt       runtimeports.OperationDispatchAttemptRefV3                    `json:"runtime_attempt"`
	AdmissionBinding     WorkspaceReadAdmissionAttemptBindingV1                        `json:"admission_binding"`
	AuthorizationDigest  runtimecore.Digest                                            `json:"authorization_digest"`
	Association          runtimeports.PreparedDomainCommandAssociationRefV1            `json:"association"`
	DomainCommand        runtimeports.OperationDomainCommandRefV1                      `json:"domain_command"`
	WorkspaceReadCommand contract.WorkspaceReadCommandV1                               `json:"workspace_read_command"`
	AdmissionReceipt     runtimeports.ControlledOperationProviderAdmissionReceiptRefV2 `json:"admission_receipt"`
	WorkspaceReadAttempt contract.WorkspaceReadAttemptRefV1                            `json:"workspace_read_attempt"`
	Digest               string                                                        `json:"digest"`
}

func (b WorkspaceReadAdmissionAttemptBindingV2) Validate() error {
	if b.ContractVersion != WorkspaceReadAdmissionAttemptBindingContractVersionV2 ||
		b.TypeURL != WorkspaceReadAdmissionAttemptBindingTypeURLV2 ||
		b.RuntimeAttempt.Validate() != nil ||
		b.AdmissionBinding.Validate() != nil ||
		b.AuthorizationDigest.Validate() != nil ||
		b.Association.Validate() != nil ||
		b.DomainCommand.Validate() != nil ||
		b.WorkspaceReadCommand.ValidateShape() != nil ||
		b.AdmissionReceipt.Validate() != nil ||
		!b.AdmissionReceipt.Admitted ||
		b.AdmissionReceipt.NoEffect ||
		b.WorkspaceReadAttempt.Validate() != nil ||
		!contract.ValidDigest(b.Digest) {
		return errors.New("workspace read Runtime-attempt admission binding is incomplete")
	}
	if b.AuthorizationDigest != b.AdmissionBinding.AuthorizationDigest ||
		b.Association != b.AdmissionBinding.Association ||
		b.DomainCommand != b.AdmissionBinding.DomainCommand ||
		b.WorkspaceReadCommand.Meta.Ref() != b.AdmissionBinding.Command ||
		b.AdmissionReceipt != b.AdmissionBinding.AdmissionReceipt ||
		b.WorkspaceReadAttempt != b.AdmissionBinding.Attempt {
		return errors.New("workspace read Runtime-attempt admission binding axes drifted")
	}
	command := b.WorkspaceReadCommand
	if command.OperationDigest != string(b.RuntimeAttempt.OperationDigest) ||
		command.EffectID != string(b.RuntimeAttempt.EffectID) ||
		command.IntentRevision != uint64(b.RuntimeAttempt.IntentRevision) ||
		command.IntentDigest != string(b.RuntimeAttempt.IntentDigest) ||
		command.AttemptID != b.RuntimeAttempt.AttemptID {
		return errors.New("workspace read Command differs from the exact Runtime attempt")
	}
	dispatchDigest, err := runtimecore.CanonicalJSONDigest(
		"praxis.sandbox.workspace-read",
		"1.0.0",
		"OperationDispatchAttemptRefV3",
		b.RuntimeAttempt,
	)
	if err != nil || command.DispatchDigest != string(dispatchDigest) {
		return errors.New("workspace read Command does not seal the exact Runtime permit and delegation")
	}
	copy := cloneWorkspaceReadAdmissionAttemptBindingV2(b)
	copy.Digest = ""
	digest, err := contract.Digest("workspace-read-admission-attempt-binding-v2", copy)
	if err != nil || digest != b.Digest {
		return errors.New("workspace read Runtime-attempt admission binding digest drifted")
	}
	return nil
}

// SealWorkspaceReadAdmissionAttemptBindingV2 is a pure canonical helper. It
// grants no authority, currentness, execution eligibility, or owner-write
// capability; only the Sandbox internal owner transaction may persist it.
func SealWorkspaceReadAdmissionAttemptBindingV2(b WorkspaceReadAdmissionAttemptBindingV2) (WorkspaceReadAdmissionAttemptBindingV2, error) {
	b = cloneWorkspaceReadAdmissionAttemptBindingV2(b)
	b.ContractVersion = WorkspaceReadAdmissionAttemptBindingContractVersionV2
	b.TypeURL = WorkspaceReadAdmissionAttemptBindingTypeURLV2
	b.Digest = ""
	digest, err := contract.Digest("workspace-read-admission-attempt-binding-v2", b)
	if err != nil {
		return WorkspaceReadAdmissionAttemptBindingV2{}, err
	}
	b.Digest = digest
	return b, b.Validate()
}

func WorkspaceReadRuntimeAttemptDigestV2(attempt runtimeports.OperationDispatchAttemptRefV3) (runtimecore.Digest, error) {
	if err := attempt.Validate(); err != nil {
		return "", err
	}
	return runtimecore.CanonicalJSONDigest(
		"praxis.sandbox.workspace-read",
		"2.0.0",
		"OperationDispatchAttemptRefV3",
		attempt,
	)
}

func cloneWorkspaceReadAdmissionAttemptBindingV2(b WorkspaceReadAdmissionAttemptBindingV2) WorkspaceReadAdmissionAttemptBindingV2 {
	if b.RuntimeAttempt.Delegation != nil {
		delegation := *b.RuntimeAttempt.Delegation
		b.RuntimeAttempt.Delegation = &delegation
	}
	if b.WorkspaceReadCommand.ExpectedFileRef != nil {
		expected := *b.WorkspaceReadCommand.ExpectedFileRef
		b.WorkspaceReadCommand.ExpectedFileRef = &expected
	}
	return b
}

type WorkspaceReadRuntimeAttemptAdmissionReaderV2 interface {
	InspectWorkspaceReadAdmissionForRuntimeAttemptV2(context.Context, runtimeports.OperationDispatchAttemptRefV3) (WorkspaceReadAdmissionAttemptBindingV2, error)
}

type WorkspaceReadExecutionPortV3 interface {
	WorkspaceReadExecutionPortV2
	WorkspaceReadRuntimeAttemptAdmissionReaderV2
}

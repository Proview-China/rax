package contract

import (
	"context"
	"reflect"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	WorkspaceReadRuntimeAdmissionContractVersionV1 = "praxis.tool-mcp/workspace-read-runtime-admission-inspection/v1"
	WorkspaceReadRuntimeAdmissionMaximumTTLV1      = 30 * time.Second
)

// WorkspaceReadRuntimeAdmissionInspectionV1 is a Tool-owned read-only
// relationship projection over Runtime public exact refs. It does not copy or
// create Runtime facts and grants no authority to execute.
type WorkspaceReadRuntimeAdmissionInspectionV1 struct {
	ContractVersion string                                                        `json:"contract_version"`
	Attempt         runtimeports.OperationDispatchAttemptRefV3                    `json:"attempt"`
	Admission       runtimeports.ControlledOperationProviderAdmissionReceiptRefV2 `json:"admission"`
	CheckedUnixNano int64                                                         `json:"checked_unix_nano"`
	ExpiresUnixNano int64                                                         `json:"expires_unix_nano"`
	Digest          runtimecore.Digest                                            `json:"digest"`
}

func SealWorkspaceReadRuntimeAdmissionInspectionV1(
	inspection WorkspaceReadRuntimeAdmissionInspectionV1,
) (WorkspaceReadRuntimeAdmissionInspectionV1, error) {
	inspection.ContractVersion = WorkspaceReadRuntimeAdmissionContractVersionV1
	inspection.Digest = ""
	digest, err := inspection.computeDigestV1()
	if err != nil {
		return WorkspaceReadRuntimeAdmissionInspectionV1{}, err
	}
	inspection.Digest = digest
	if err = inspection.ValidateCurrent(inspection.Attempt, time.Unix(0, inspection.CheckedUnixNano)); err != nil {
		return WorkspaceReadRuntimeAdmissionInspectionV1{}, err
	}
	return inspection, nil
}

func (inspection WorkspaceReadRuntimeAdmissionInspectionV1) ValidateCurrent(
	exact runtimeports.OperationDispatchAttemptRefV3,
	now time.Time,
) error {
	if inspection.ContractVersion != WorkspaceReadRuntimeAdmissionContractVersionV1 ||
		inspection.Attempt.Validate() != nil ||
		inspection.Admission.Validate() != nil ||
		!inspection.Admission.Admitted ||
		inspection.Admission.NoEffect ||
		inspection.CheckedUnixNano <= 0 ||
		inspection.ExpiresUnixNano <= inspection.CheckedUnixNano ||
		inspection.ExpiresUnixNano-inspection.CheckedUnixNano > WorkspaceReadRuntimeAdmissionMaximumTTLV1.Nanoseconds() ||
		inspection.Digest.Validate() != nil {
		return runtimecore.NewError(runtimecore.ErrorInvalidArgument, runtimecore.ReasonInvalidReference, "workspace.read Runtime Admission inspection is incomplete")
	}
	if !reflect.DeepEqual(inspection.Attempt, exact) {
		return runtimecore.NewError(runtimecore.ErrorConflict, runtimecore.ReasonBindingDrift, "workspace.read Runtime Admission inspection changed the exact Attempt")
	}
	digest, err := inspection.computeDigestV1()
	if err != nil || digest != inspection.Digest {
		return runtimecore.NewError(runtimecore.ErrorConflict, runtimecore.ReasonInvalidDigest, "workspace.read Runtime Admission inspection digest drifted")
	}
	if now.IsZero() || now.UnixNano() < inspection.CheckedUnixNano {
		return runtimecore.NewError(runtimecore.ErrorPreconditionFailed, runtimecore.ReasonClockRegression, "workspace.read Runtime Admission inspection clock regressed")
	}
	if !now.Before(time.Unix(0, inspection.ExpiresUnixNano)) {
		return runtimecore.NewError(runtimecore.ErrorPreconditionFailed, runtimecore.ReasonBindingExpired, "workspace.read Runtime Admission inspection expired")
	}
	return nil
}

func (inspection WorkspaceReadRuntimeAdmissionInspectionV1) computeDigestV1() (runtimecore.Digest, error) {
	inspection.Digest = ""
	return runtimecore.CanonicalJSONDigest(
		"praxis.tool-mcp.workspace-read-runtime-admission",
		WorkspaceReadRuntimeAdmissionContractVersionV1,
		"WorkspaceReadRuntimeAdmissionInspectionV1",
		inspection,
	)
}

type WorkspaceReadRuntimeAdmissionCurrentReaderV1 interface {
	InspectWorkspaceReadAdmissionForRuntimeAttemptV1(
		context.Context,
		runtimeports.OperationDispatchAttemptRefV3,
	) (WorkspaceReadRuntimeAdmissionInspectionV1, error)
}

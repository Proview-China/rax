package ports

import (
	"context"
	"errors"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

var ErrWorkspaceReadUnknown = errors.New("workspace read outcome is indeterminate; inspect the original attempt")

type WorkspaceReadCommandCurrentReaderV1 interface {
	InspectWorkspaceReadCommandCurrentV1(context.Context, contract.Ref) (contract.WorkspaceReadCommandV1, error)
}
type WorkspaceReadOwnerStoreV1 interface {
	WorkspaceReadCommandCurrentReaderV1
	CreateWorkspaceReadCommandV1(context.Context, contract.WorkspaceReadCommandV1) (contract.WorkspaceReadCommandV1, error)
	ReserveWorkspaceReadV1(context.Context, contract.WorkspaceReadReservationV1, contract.WorkspaceReadAttemptV1) (contract.WorkspaceReadExecutionProjectionV1, bool, error)
	CompleteWorkspaceReadV1(context.Context, contract.Ref, contract.WorkspaceReadObservationV1) (contract.WorkspaceReadExecutionProjectionV1, error)
	MarkWorkspaceReadUnknownV1(context.Context, contract.Ref, string) (contract.WorkspaceReadExecutionProjectionV1, error)
	FailWorkspaceReadV1(context.Context, contract.Ref, string) (contract.WorkspaceReadExecutionProjectionV1, error)
	RecoverStartedWorkspaceReadAfterRestartV1(context.Context, contract.WorkspaceReadAttemptRefV1) (contract.WorkspaceReadExecutionProjectionV1, error)
	InspectBoundedWorkspaceReadV1(context.Context, contract.WorkspaceReadAttemptRefV1) (contract.WorkspaceReadExecutionProjectionV1, error)
}

type WorkspaceReadExecutionPortV1 interface {
	runtimeports.ControlledOperationPhysicalExecutionPortV3
	InspectBoundedWorkspaceReadV1(context.Context, contract.WorkspaceReadAttemptRefV1) (contract.WorkspaceReadExecutionProjectionV1, error)
}

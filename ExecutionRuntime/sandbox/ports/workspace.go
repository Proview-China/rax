package ports

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

// CaptureWorkspaceChangeSetRequest is an Owner-local, coordinate-only request.
// Filesystem roots are injected through the Driver's trusted binding config and
// never travel in this DTO.
type CaptureWorkspaceChangeSetRequest struct {
	ChangeSetID       string
	View              contract.WorkspaceView
	RequestedNotAfter time.Time
}

// WorkspaceChangeSetCapturePortV1 captures a real Base/Overlay difference as a
// staged ChangeSet. It has no commit, settlement, or Provider authority.
type WorkspaceChangeSetCapturePortV1 interface {
	CaptureWorkspaceChangeSetV1(context.Context, CaptureWorkspaceChangeSetRequest) (contract.WorkspaceChangeSet, error)
}

// WorkspaceCurrentReaderV1 is the exact-current Owner reader used before a
// governed workspace commit reaches the Provider actual point.
type WorkspaceCurrentReaderV1 interface {
	InspectWorkspaceViewCurrentV1(context.Context, contract.Ref) (contract.WorkspaceView, error)
	InspectWorkspaceChangeSetCurrentV1(context.Context, contract.Ref) (contract.WorkspaceChangeSet, error)
}

// WorkspaceOwnerStoreV1 is an internal Sandbox Owner persistence seam. It is
// not exposed by SDK/API and grants no Runtime or Provider authority.
type WorkspaceOwnerStoreV1 interface {
	WorkspaceCurrentReaderV1
	CreateWorkspaceViewV1(context.Context, contract.WorkspaceView) (contract.WorkspaceView, error)
	CreateWorkspaceChangeSetV1(context.Context, contract.WorkspaceChangeSet) (contract.WorkspaceChangeSet, error)
	InspectWorkspaceViewHistoryV1(context.Context, contract.Ref) (contract.WorkspaceView, error)
	InspectWorkspaceChangeSetHistoryV1(context.Context, contract.Ref) (contract.WorkspaceChangeSet, error)
	// InspectWorkspaceChangeSetOwnerCurrentByIDV1 is an Owner-internal recovery
	// seam. It must not be exposed as a caller-selected "current" public Port.
	InspectWorkspaceChangeSetOwnerCurrentByIDV1(context.Context, string) (contract.WorkspaceChangeSet, error)
	CompareAndSwapWorkspaceChangeSetV1(context.Context, contract.Ref, contract.WorkspaceChangeSet) error
}

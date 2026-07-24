package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

type WorkspaceRewindCompositionRepositoryV1 interface {
	CreateWorkspaceRewindCompositionV1(context.Context, contract.WorkspaceRewindCompositionFactV1) (contract.WorkspaceRewindCompositionFactV1, error)
	InspectWorkspaceRewindCompositionV1(context.Context, contract.Ref) (contract.WorkspaceRewindCompositionFactV1, error)
	InspectWorkspaceRewindCompositionByRequestV1(context.Context, string, string, string) (contract.WorkspaceRewindCompositionFactV1, error)
}

type WorkspaceRewindCompositionResultV1 struct {
	Composition contract.WorkspaceRewindCompositionFactV1 `json:"composition"`
	ChangeSet   contract.WorkspaceChangeSet               `json:"change_set"`
}

// WorkspaceRewindCompositionPortV1 is Owner-local and side-effect free with
// respect to the filesystem. It may create only a staged ChangeSet and an
// immutable composition fact. The existing governed workspace-commit flow is
// the sole execution route.
type WorkspaceRewindCompositionPortV1 interface {
	ComposeWorkspaceRewindV1(context.Context, contract.ComposeWorkspaceRewindRequestV1) (WorkspaceRewindCompositionResultV1, error)
	InspectWorkspaceRewindV1(context.Context, contract.ComposeWorkspaceRewindRequestV1) (WorkspaceRewindCompositionResultV1, error)
}

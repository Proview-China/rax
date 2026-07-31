package workspacereadcommandrepo

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

var ownerWriteSealV1 = &struct{}{}

// WriteCapabilityV1 makes the raw command writer reachable only from packages
// below ExecutionRuntime/tool-mcp. Cross-owner packages cannot import this Go
// internal package and therefore cannot construct or name the capability.
type WriteCapabilityV1 struct {
	seal *struct{}
}

func NewWriteCapabilityV1() WriteCapabilityV1 {
	return WriteCapabilityV1{seal: ownerWriteSealV1}
}

func (c WriteCapabilityV1) Validate() error {
	if c.seal != ownerWriteSealV1 {
		return core.NewError(core.ErrorForbidden, core.ReasonOwnerConflict, "workspace.read execution command owner write capability is invalid")
	}
	return nil
}

// RepositoryV1 is the Tool-owner-private mutation seam. Public packages expose
// only exact/reverse/current readers.
type RepositoryV1 interface {
	CreateWorkspaceReadExecutionCommandOwnedV1(
		context.Context,
		WriteCapabilityV1,
		toolcontract.WorkspaceReadExecutionCommandV1,
	) (toolcontract.WorkspaceReadExecutionCommandV1, bool, error)
	toolcontract.WorkspaceReadExecutionCommandExactReaderV1
	toolcontract.WorkspaceReadExecutionCommandAttemptReaderV1
}

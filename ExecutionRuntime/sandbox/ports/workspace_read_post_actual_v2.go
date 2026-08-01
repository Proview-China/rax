package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

// WorkspaceReadTerminalExactReaderV2 is the public historical read surface.
// It validates an exact fact and does not require Qualification currentness.
type WorkspaceReadTerminalExactReaderV2 interface {
	InspectWorkspaceReadTerminalExactV2(
		context.Context,
		contract.WorkspaceReadTerminalRefV2,
	) (contract.WorkspaceReadTerminalFactV2, error)
}

type WorkspaceReadTerminalOriginReaderV2 interface {
	InspectWorkspaceReadTerminalByOriginV2(
		context.Context,
		contract.WorkspaceReadAttemptRefV1,
	) (contract.WorkspaceReadTerminalFactV2, error)
}

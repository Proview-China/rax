package ports

import (
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type WorkspaceReadExecutionCommandReadersV1 interface {
	toolcontract.WorkspaceReadExecutionCommandExactReaderV1
	toolcontract.WorkspaceReadExecutionCommandAttemptReaderV1
	toolcontract.WorkspaceReadExecutionCommandCurrentReaderV1
}

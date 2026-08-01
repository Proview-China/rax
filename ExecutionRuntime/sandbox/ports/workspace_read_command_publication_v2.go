package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

type WorkspaceReadSourceCurrentReaderV2 interface {
	InspectWorkspaceReadSourceCurrentV2(
		context.Context,
		contract.WorkspaceReadSourceCommandRefV2,
	) (contract.WorkspaceReadSourceCurrentProjectionV2, error)
}

type EnsureWorkspaceReadCommandRequestV2 struct {
	SourceCommand contract.WorkspaceReadSourceCommandRefV2 `json:"source_command"`
}

func (r EnsureWorkspaceReadCommandRequestV2) Validate() error {
	return r.SourceCommand.Validate()
}

type WorkspaceReadCommandPublicationExactReaderV2 interface {
	InspectWorkspaceReadCommandPublicationExactV2(
		context.Context,
		contract.Ref,
	) (contract.WorkspaceReadCommandPublicationV2, error)
}

type WorkspaceReadCommandOwnerCurrentReaderV2 interface {
	InspectWorkspaceReadCommandOwnerCurrentExactV2(
		context.Context,
		contract.Ref,
	) (contract.WorkspaceReadCommandOwnerCurrentV2, error)
	InspectWorkspaceReadCommandOwnerCurrentByCommandV2(
		context.Context,
		contract.Ref,
	) (contract.WorkspaceReadCommandOwnerCurrentV2, error)
}

type WorkspaceReadCommandEnsurePortV2 interface {
	EnsureWorkspaceReadCommandV2(
		context.Context,
		EnsureWorkspaceReadCommandRequestV2,
	) (contract.WorkspaceReadCommandOwnerCurrentV2, error)
}

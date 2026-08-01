package sqlite

import (
	"context"
	"errors"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	ownerworkspaceread "github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/owner/workspaceread"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

// TransitionWorkspaceReadAuthorizedV2 is the sole durable terminal writer for
// a governed workspace.read attempt. The nominal request is Sandbox-internal;
// the public V1 transition methods remain compatibility surfaces but always
// fail closed.
func (s *Store) TransitionWorkspaceReadAuthorizedV2(
	ctx context.Context,
	request ownerworkspaceread.AuthorizedTransitionV2,
) (contract.WorkspaceReadExecutionProjectionV1, error) {
	if ctx == nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, errors.New("workspace read authorized transition context is required")
	}
	if err := ctx.Err(); err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, err
	}
	if s == nil || s.db == nil || s.clock == nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, errors.New("workspace read authorized transition store is unavailable")
	}
	now := s.clock()
	attempt, authorization, observation, unknown, failure, err := request.Open(now)
	if err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, ports.ErrConflict
	}
	return s.finishWorkspaceReadAuthorizedV2(ctx, attempt.OwnerRef(), observation, unknown, failure, authorization)
}

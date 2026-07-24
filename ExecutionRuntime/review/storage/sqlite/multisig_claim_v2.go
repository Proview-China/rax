package sqlite

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
)

func (s *Store) ClaimHumanAssignmentV2(ctx context.Context, m reviewport.ClaimHumanAssignmentMutationV2) (out reviewport.ClaimHumanAssignmentResultV2, err error) {
	err = s.mutate(ctx, m.NextAssignment.TenantID, func(state *memory.Store) error {
		out, err = state.ClaimHumanAssignmentV2(ctx, m)
		return err
	})
	return
}

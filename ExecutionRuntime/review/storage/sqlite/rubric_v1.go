package sqlite

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

var _ reviewport.RubricStoreV1 = (*Store)(nil)

func (s *Store) PublishRubricV1(ctx context.Context, m reviewport.PublishRubricMutationV1) (out contract.RubricDefinitionV1, err error) {
	err = s.mutate(ctx, m.Next.TenantID, func(state *memory.Store) error {
		out, err = state.PublishRubricV1(ctx, m)
		return err
	})
	return
}

func (s *Store) RevokeRubricV1(ctx context.Context, m reviewport.RevokeRubricMutationV1) (out contract.RubricDefinitionV1, err error) {
	err = s.mutate(ctx, m.Next.TenantID, func(state *memory.Store) error {
		out, err = state.RevokeRubricV1(ctx, m)
		return err
	})
	return
}

func (s *Store) InspectRubricExactV1(ctx context.Context, tenant core.TenantID, ref contract.ExactResourceRefV1) (out contract.RubricDefinitionV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error {
		out, err = state.InspectRubricExactV1(ctx, tenant, ref)
		return err
	})
	return
}

func (s *Store) InspectRubricCurrentV1(ctx context.Context, tenant core.TenantID, ref contract.ExactResourceRefV1, now time.Time) (out contract.RubricDefinitionV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error {
		out, err = state.InspectRubricCurrentV1(ctx, tenant, ref, now)
		return err
	})
	return
}

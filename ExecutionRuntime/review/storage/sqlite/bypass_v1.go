package sqlite

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
)

var _ reviewport.BypassStoreV1 = (*Store)(nil)

func (s *Store) CreateBypassDecisionV1(ctx context.Context, m reviewport.CreateBypassDecisionMutationV1) (out contract.BypassDecisionV1, err error) {
	err = s.mutate(ctx, m.Decision.TenantID, func(state *memory.Store) error {
		out, err = state.CreateBypassDecisionV1(ctx, m)
		return err
	})
	return
}

func (s *Store) InspectBypassDecisionExactV1(ctx context.Context, ref contract.BypassDecisionExactRefV1) (out contract.BypassDecisionV1, err error) {
	err = s.read(ctx, ref.TenantID, func(state *memory.Store) error {
		out, err = state.InspectBypassDecisionExactV1(ctx, ref)
		return err
	})
	return
}

func (s *Store) InspectCurrentBypassDecisionByCaseV1(ctx context.Context, ref contract.BypassCaseExactRefV1) (out contract.BypassDecisionV1, err error) {
	err = s.read(ctx, ref.TenantID, func(state *memory.Store) error {
		out, err = state.InspectCurrentBypassDecisionByCaseV1(ctx, ref)
		return err
	})
	return
}

func (s *Store) CompareAndSwapBypassDecisionV1(ctx context.Context, m reviewport.BypassDecisionCASMutationV1) (out contract.BypassDecisionV1, err error) {
	err = s.mutate(ctx, m.Next.TenantID, func(state *memory.Store) error {
		out, err = state.CompareAndSwapBypassDecisionV1(ctx, m)
		return err
	})
	return
}

package sqlite

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

var _ reviewport.AutoReviewerStoreV1 = (*Store)(nil)

func (s *Store) BeginAutoReviewerAttemptV1(ctx context.Context, mutation reviewport.BeginAutoReviewerAttemptMutationV1) (out contract.AutoReviewerAttemptV1, err error) {
	err = s.mutate(ctx, mutation.Attempt.TenantID, func(state *memory.Store) error {
		out, err = state.BeginAutoReviewerAttemptV1(ctx, mutation)
		return err
	})
	return
}

func (s *Store) MarkAutoReviewerWaitingInspectV1(ctx context.Context, mutation reviewport.MarkAutoReviewerWaitingInspectMutationV1) (out reviewport.AutoReviewerInvocationStartClaimReceiptV1, err error) {
	err = s.mutate(ctx, mutation.Next.TenantID, func(state *memory.Store) error {
		out, err = state.MarkAutoReviewerWaitingInspectV1(ctx, mutation)
		return err
	})
	return
}

func (s *Store) RecordAutoReviewerObservationV1(ctx context.Context, mutation reviewport.RecordAutoReviewerObservationMutationV1) (attempt contract.AutoReviewerAttemptV1, result contract.ReviewerInvocationResultFactV1, err error) {
	err = s.mutate(ctx, mutation.Next.TenantID, func(state *memory.Store) error {
		attempt, result, err = state.RecordAutoReviewerObservationV1(ctx, mutation)
		return err
	})
	return
}

func (s *Store) TerminateAutoReviewerAttemptV1(ctx context.Context, mutation reviewport.TerminateAutoReviewerAttemptMutationV1) (out contract.AutoReviewerAttemptV1, err error) {
	err = s.mutate(ctx, mutation.Next.TenantID, func(state *memory.Store) error {
		out, err = state.TerminateAutoReviewerAttemptV1(ctx, mutation)
		return err
	})
	return
}

func (s *Store) InspectAutoReviewerAttemptExactV1(ctx context.Context, tenant core.TenantID, ref contract.ExactResourceRefV1) (out contract.AutoReviewerAttemptV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error {
		out, err = state.InspectAutoReviewerAttemptExactV1(ctx, tenant, ref)
		return err
	})
	return
}

func (s *Store) InspectAutoReviewerAttemptCurrentV1(ctx context.Context, tenant core.TenantID, id string) (out contract.AutoReviewerAttemptV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error {
		out, err = state.InspectAutoReviewerAttemptCurrentV1(ctx, tenant, id)
		return err
	})
	return
}

func (s *Store) InspectAutoReviewerAttemptByIdempotencyV1(ctx context.Context, tenant core.TenantID, idempotency string) (out contract.AutoReviewerAttemptV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error {
		out, err = state.InspectAutoReviewerAttemptByIdempotencyV1(ctx, tenant, idempotency)
		return err
	})
	return
}

func (s *Store) InspectAutoReviewerObservationExactV1(ctx context.Context, tenant core.TenantID, ref contract.AutoReviewerInvocationObservationRefV1) (out contract.AutoReviewerInvocationObservationV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error {
		out, err = state.InspectAutoReviewerObservationExactV1(ctx, tenant, ref)
		return err
	})
	return
}

func (s *Store) InspectAutoReviewTerminationCurrentV1(ctx context.Context, request reviewport.AutoReviewTerminationCurrentRequestV1) (out reviewport.AutoReviewTerminationCurrentProjectionV1, err error) {
	err = s.read(ctx, request.TenantID, func(state *memory.Store) error {
		out, err = state.InspectAutoReviewTerminationCurrentV1(ctx, request)
		return err
	})
	return
}

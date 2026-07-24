package service

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func (s *Service) PublishRubricV1(ctx context.Context, mutation reviewport.PublishRubricMutationV1) (contract.RubricDefinitionV1, error) {
	baseline := s.clock()
	if baseline.IsZero() || baseline.UnixNano() < mutation.Next.UpdatedUnixNano {
		return contract.RubricDefinitionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "rubric publish clock is unavailable or predates the definition")
	}
	if err := mutation.Next.ValidateCurrent(mutation.Next.ExactRef(), baseline); err != nil {
		return contract.RubricDefinitionV1{}, err
	}
	if mutation.Expected != nil {
		if _, err := s.store.InspectRubricCurrentV1(ctx, mutation.Next.TenantID, *mutation.Expected, baseline); err != nil {
			return contract.RubricDefinitionV1{}, err
		}
	}
	fresh := s.clock()
	if fresh.IsZero() || fresh.Before(baseline) || fresh.UnixNano() < mutation.Next.UpdatedUnixNano {
		return contract.RubricDefinitionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "rubric publish clock regressed")
	}
	if err := mutation.Next.ValidateCurrent(mutation.Next.ExactRef(), fresh); err != nil {
		return contract.RubricDefinitionV1{}, err
	}
	published, err := s.store.PublishRubricV1(ctx, mutation)
	if err == nil || (!core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorIndeterminate)) {
		return published, err
	}
	// Unknown mutation outcome never replays the mutation. Exact immutable
	// history is inspected with a detached context.
	recoveryCtx, cancel, ok := s.boundedRecoveryContextV1(ctx, fresh, mutation.Next.ExpiresUnixNano)
	if !ok {
		return contract.RubricDefinitionV1{}, err
	}
	defer cancel()
	inspected, inspectErr := s.store.InspectRubricExactV1(recoveryCtx, mutation.Next.TenantID, mutation.Next.ExactRef())
	if inspectErr != nil || !reflect.DeepEqual(inspected, mutation.Next) || !s.recoveryStillCurrentV1(fresh, mutation.Next.ExpiresUnixNano) {
		return contract.RubricDefinitionV1{}, err
	}
	return inspected, nil
}

func (s *Service) RevokeRubricV1(ctx context.Context, mutation reviewport.RevokeRubricMutationV1) (contract.RubricDefinitionV1, error) {
	baseline := s.clock()
	if baseline.IsZero() || baseline.UnixNano() < mutation.Next.UpdatedUnixNano {
		return contract.RubricDefinitionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "rubric revoke clock is unavailable or predates the terminal revision")
	}
	if _, err := s.store.InspectRubricCurrentV1(ctx, mutation.Next.TenantID, mutation.Expected, baseline); err != nil {
		return contract.RubricDefinitionV1{}, err
	}
	fresh := s.clock()
	if fresh.IsZero() || fresh.Before(baseline) || fresh.UnixNano() < mutation.Next.UpdatedUnixNano {
		return contract.RubricDefinitionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "rubric revoke clock regressed")
	}
	revoked, err := s.store.RevokeRubricV1(ctx, mutation)
	if err == nil || (!core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorIndeterminate)) {
		return revoked, err
	}
	recoveryCtx, cancel, ok := s.boundedRecoveryContextV1(ctx, fresh, mutation.Next.ExpiresUnixNano)
	if !ok {
		return contract.RubricDefinitionV1{}, err
	}
	defer cancel()
	inspected, inspectErr := s.store.InspectRubricExactV1(recoveryCtx, mutation.Next.TenantID, mutation.Next.ExactRef())
	if inspectErr != nil || !reflect.DeepEqual(inspected, mutation.Next) || !s.recoveryStillCurrentV1(fresh, mutation.Next.ExpiresUnixNano) {
		return contract.RubricDefinitionV1{}, err
	}
	return inspected, nil
}

func (s *Service) InspectRubricExactV1(ctx context.Context, tenant core.TenantID, ref contract.ExactResourceRefV1) (contract.RubricDefinitionV1, error) {
	return s.store.InspectRubricExactV1(ctx, tenant, ref)
}

func (s *Service) InspectRubricCurrentV1(ctx context.Context, tenant core.TenantID, ref contract.ExactResourceRefV1) (contract.RubricDefinitionV1, error) {
	baseline := s.clock()
	if baseline.IsZero() {
		return contract.RubricDefinitionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "rubric current clock is unavailable")
	}
	first, err := s.store.InspectRubricCurrentV1(ctx, tenant, ref, baseline)
	if err != nil {
		return contract.RubricDefinitionV1{}, err
	}
	fresh := s.clock()
	if fresh.IsZero() || fresh.Before(baseline) {
		return contract.RubricDefinitionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "rubric current clock regressed")
	}
	second, err := s.store.InspectRubricCurrentV1(ctx, tenant, ref, fresh)
	if err != nil {
		return contract.RubricDefinitionV1{}, err
	}
	if first.Digest != second.Digest {
		return contract.RubricDefinitionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "rubric current ref drifted between reads")
	}
	return second, nil
}

package runtimeadapter

import (
	"context"
	"errors"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// GovernedRelayV2 is the host-side translation boundary between Application
// orchestration and a data-plane provider. It owns no Effect, Permit,
// Enforcement, Observation, Settlement or Outcome facts.
type GovernedRelayV2 struct {
	Provider runtimeports.GovernedExecutionProviderV2
	Clock    func() time.Time
}

var _ runtimeports.GovernedExecutionPortV2 = (*GovernedRelayV2)(nil)

func NewGovernedRelayV2(provider runtimeports.GovernedExecutionProviderV2, clock func() time.Time) (*GovernedRelayV2, error) {
	if provider == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "governed data-plane provider is required")
	}
	if clock == nil {
		clock = time.Now
	}
	return &GovernedRelayV2{Provider: provider, Clock: clock}, nil
}

func (r *GovernedRelayV2) RelayPrepare(ctx context.Context, request runtimeports.PrepareGovernedExecutionRequestV2) (runtimeports.ProviderPreparationAttestationV2, error) {
	if err := request.Validate(); err != nil {
		return runtimeports.ProviderPreparationAttestationV2{}, err
	}
	prepared, err := r.Provider.Prepare(ctx, request)
	if err != nil {
		if !unknownProviderReplyV2(err) {
			return runtimeports.ProviderPreparationAttestationV2{}, err
		}
		prepared, err = r.Provider.InspectPrepared(context.WithoutCancel(ctx), runtimeports.InspectPreparedProviderRequestV2{DeclaredDelegation: request.Delegation, PreparedAttemptID: mustPreparedAttemptIDV2(request), PermitID: request.Permit.ID, AttemptID: request.Permit.AttemptID})
		if err != nil {
			return runtimeports.ProviderPreparationAttestationV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "provider Prepare reply is unknown and local Inspect cannot prove it")
		}
	}
	if err := prepared.ValidateAgainstPrepare(request, r.Clock()); err != nil {
		return runtimeports.ProviderPreparationAttestationV2{}, err
	}
	return prepared, nil
}

func (r *GovernedRelayV2) RelayInspectPrepared(ctx context.Context, request runtimeports.InspectPreparedProviderRequestV2) (runtimeports.ProviderPreparationAttestationV2, error) {
	if err := request.Validate(); err != nil {
		return runtimeports.ProviderPreparationAttestationV2{}, err
	}
	prepared, err := r.Provider.InspectPrepared(ctx, request)
	if err != nil {
		return runtimeports.ProviderPreparationAttestationV2{}, err
	}
	if err := prepared.Validate(); err != nil {
		return runtimeports.ProviderPreparationAttestationV2{}, err
	}
	if prepared.Delegation != request.DeclaredDelegation || prepared.Prepared.ID != request.PreparedAttemptID || prepared.Prepared.PermitID != request.PermitID || prepared.Prepared.AttemptID != request.AttemptID {
		return runtimeports.ProviderPreparationAttestationV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "provider prepared Inspect returned another attempt")
	}
	return prepared, nil
}

func (r *GovernedRelayV2) RelayExecutePrepared(ctx context.Context, request runtimeports.ExecutePreparedRequestV2) (runtimeports.ProviderAttemptObservationV2, error) {
	if err := request.Validate(); err != nil {
		return runtimeports.ProviderAttemptObservationV2{}, err
	}
	observation, err := r.Provider.ExecutePrepared(ctx, request)
	if err != nil {
		if !unknownProviderReplyV2(err) {
			return runtimeports.ProviderAttemptObservationV2{}, err
		}
		observation, err = r.Provider.InspectLocalAttempt(context.WithoutCancel(ctx), runtimeports.InspectLocalProviderAttemptRequestV2{Delegation: request.Delegation, Prepared: request.Prepared})
		if err != nil {
			return runtimeports.ProviderAttemptObservationV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "provider Execute reply is unknown and local Inspect cannot prove it")
		}
	}
	if err := observation.ValidateAgainstPrepared(request.Prepared); err != nil {
		return runtimeports.ProviderAttemptObservationV2{}, err
	}
	if observation.Delegation != request.Delegation {
		return runtimeports.ProviderAttemptObservationV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "provider Execute returned another delegation")
	}
	return observation, nil
}

func (r *GovernedRelayV2) RelayInspectLocalAttempt(ctx context.Context, request runtimeports.InspectLocalProviderAttemptRequestV2) (runtimeports.ProviderAttemptObservationV2, error) {
	if err := request.Validate(); err != nil {
		return runtimeports.ProviderAttemptObservationV2{}, err
	}
	observation, err := r.Provider.InspectLocalAttempt(ctx, request)
	if err != nil {
		return runtimeports.ProviderAttemptObservationV2{}, err
	}
	if err := observation.ValidateAgainstPrepared(request.Prepared); err != nil {
		return runtimeports.ProviderAttemptObservationV2{}, err
	}
	if observation.Delegation != request.Delegation {
		return runtimeports.ProviderAttemptObservationV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "provider local Inspect returned another delegation")
	}
	return observation, nil
}

func mustPreparedAttemptIDV2(request runtimeports.PrepareGovernedExecutionRequestV2) string {
	id, _ := runtimeports.DerivePreparedProviderAttemptIDV2(request.Delegation.ID, request.Permit.ID, request.Permit.AttemptID)
	return id
}

func unknownProviderReplyV2(err error) bool {
	return core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

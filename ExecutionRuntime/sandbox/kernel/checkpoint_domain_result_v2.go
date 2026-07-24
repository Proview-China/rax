package kernel

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type CheckpointPhaseResultOwnerV2 struct {
	phases      ports.CheckpointPhaseStore
	results     ports.CheckpointPhaseResultStoreV2
	current     ports.CheckpointPhaseResultCurrentReaderV2
	settlements ports.CheckpointPhaseSettlementCurrentReaderV2
	clock       func() time.Time
	maxTTL      time.Duration
}

func NewCheckpointPhaseResultOwnerV2(phases ports.CheckpointPhaseStore, results ports.CheckpointPhaseResultStoreV2, current ports.CheckpointPhaseResultCurrentReaderV2, settlements ports.CheckpointPhaseSettlementCurrentReaderV2, clock func() time.Time, maxTTL time.Duration) (*CheckpointPhaseResultOwnerV2, error) {
	if nilInterface(phases) || nilInterface(results) || nilInterface(current) || nilInterface(settlements) || clock == nil || maxTTL <= 0 {
		return nil, errors.New("checkpoint phase result Owner dependencies are required")
	}
	return &CheckpointPhaseResultOwnerV2{phases: phases, results: results, current: current, settlements: settlements, clock: clock, maxTTL: maxTTL}, nil
}

func (o *CheckpointPhaseResultOwnerV2) RecordCheckpointPhaseDomainResultV2(ctx context.Context, input *contract.RecordCheckpointPhaseDomainResultRequestV2) (contract.CheckpointPhaseDomainResultV2, error) {
	if input == nil {
		return contract.CheckpointPhaseDomainResultV2{}, errors.New("checkpoint DomainResult record request is required")
	}
	request := *input
	now := o.clock()
	if err := request.ValidateCurrent(now); err != nil {
		return contract.CheckpointPhaseDomainResultV2{}, err
	}
	reservation, err := o.phases.InspectCheckpointPhaseReservation(ctx, request.ReservationRef)
	if err != nil {
		return contract.CheckpointPhaseDomainResultV2{}, err
	}
	if err := reservation.ValidateCurrent(now); err != nil {
		return contract.CheckpointPhaseDomainResultV2{}, err
	}
	if existing, err := o.results.InspectCheckpointPhaseDomainResultByReservationV2(ctx, reservation.Meta.Ref()); err == nil {
		return existing, existing.ValidateShape()
	} else if !errors.Is(err, ports.ErrNotFound) {
		return contract.CheckpointPhaseDomainResultV2{}, err
	}
	s1, err := o.current.InspectCheckpointPhaseResultCurrentV2(ctx, reservation.Meta.Ref())
	if err != nil {
		return contract.CheckpointPhaseDomainResultV2{}, err
	}
	if err := validateCheckpointPhaseResultProjectionV2(s1, reservation, request, now); err != nil {
		return contract.CheckpointPhaseDomainResultV2{}, err
	}
	fresh := o.clock()
	s2, err := o.current.InspectCheckpointPhaseResultCurrentV2(ctx, reservation.Meta.Ref())
	if err != nil {
		return contract.CheckpointPhaseDomainResultV2{}, err
	}
	if err := validateCheckpointPhaseResultProjectionV2(s2, reservation, request, fresh); err != nil || s1.ProjectionDigest != s2.ProjectionDigest {
		if err != nil {
			return contract.CheckpointPhaseDomainResultV2{}, err
		}
		return contract.CheckpointPhaseDomainResultV2{}, fmt.Errorf("%w: checkpoint phase result current changed between S1 and S2", ports.ErrConflict)
	}
	expires := min(reservation.Meta.ExpiresUnixNano, s2.ExpiresUnixNano, request.RequestedNotAfter, fresh.Add(o.maxTTL).UnixNano())
	result, err := contract.SealCheckpointPhaseDomainResultV2(contract.CheckpointPhaseDomainResultV2{
		Meta:           contract.Meta{ContractVersion: contract.ContractFamily, ID: reservation.Meta.ID + "-domain-result", Revision: 1, CreatedUnixNano: fresh.UnixNano(), UpdatedUnixNano: fresh.UnixNano(), ExpiresUnixNano: expires},
		ReservationRef: reservation.Meta.Ref(), TenantID: reservation.TenantID, ParticipantRef: reservation.ParticipantRef,
		CheckpointAttemptRef: reservation.Base.CheckpointAttempt, Phase: reservation.Phase, PreviousPresence: reservation.PreviousPresence,
		PreviousPhase: reservation.PreviousPhase, OperationID: reservation.OperationID, EffectID: reservation.EffectID,
		AttemptID: reservation.AttemptID, State: s2.State, ProviderAttemptRef: s2.ProviderAttemptRef,
		ProviderObservation: s2.ProviderObservation, ProviderReceipt: s2.ProviderReceipt, EvidenceConsumption: s2.EvidenceConsumption,
	})
	if err != nil {
		return contract.CheckpointPhaseDomainResultV2{}, err
	}
	created, createErr := o.results.CreateCheckpointPhaseDomainResultV2(ctx, result)
	if createErr == nil && created {
		return result, nil
	}
	recovered, inspectErr := o.results.InspectCheckpointPhaseDomainResultByReservationV2(context.WithoutCancel(ctx), reservation.Meta.Ref())
	if inspectErr == nil && recovered.ExactRef() == result.ExactRef() {
		return recovered, nil
	}
	if createErr != nil {
		return contract.CheckpointPhaseDomainResultV2{}, createErr
	}
	if inspectErr != nil {
		return contract.CheckpointPhaseDomainResultV2{}, inspectErr
	}
	return contract.CheckpointPhaseDomainResultV2{}, fmt.Errorf("%w: checkpoint DomainResult create-once winner differs", ports.ErrConflict)
}

func (o *CheckpointPhaseResultOwnerV2) InspectCheckpointPhaseDomainResultV2(ctx context.Context, input *contract.SnapshotArtifactExactRefV2) (contract.CheckpointPhaseDomainResultV2, error) {
	if input == nil || input.ValidateShape("checkpoint phase DomainResult") != nil || input.TypeURL != contract.CheckpointPhaseDomainResultTypeURLV2 {
		return contract.CheckpointPhaseDomainResultV2{}, errors.New("checkpoint DomainResult Inspect ref is invalid")
	}
	result, err := o.results.InspectCheckpointPhaseDomainResultV2(ctx, *input)
	if err != nil {
		return contract.CheckpointPhaseDomainResultV2{}, err
	}
	return result, result.ValidateShape()
}

func (o *CheckpointPhaseResultOwnerV2) ApplyCheckpointPhaseSettlementV2(ctx context.Context, input *contract.CheckpointPhaseApplySettlementV2) (contract.CheckpointPhaseFact, error) {
	if input == nil || input.ValidateShape() != nil {
		return contract.CheckpointPhaseFact{}, errors.New("checkpoint ApplySettlement request is invalid")
	}
	request := *input
	now := o.clock()
	domain, err := o.results.InspectCheckpointPhaseDomainResultV2(ctx, request.DomainResultRef)
	if err != nil {
		return contract.CheckpointPhaseFact{}, err
	}
	if err := domain.ValidateCurrent(now); err != nil {
		return contract.CheckpointPhaseFact{}, err
	}
	reservation, err := o.phases.InspectCheckpointPhaseReservation(ctx, domain.ReservationRef)
	if err != nil || reservation.ValidateCurrent(now) != nil {
		return contract.CheckpointPhaseFact{}, fmt.Errorf("%w: checkpoint ApplySettlement reservation is unavailable", ports.ErrStale)
	}
	if existing, err := o.phases.InspectCheckpointPhaseFactByReservation(ctx, reservation.Meta.Ref()); err == nil {
		return existing, existing.ValidateShape()
	} else if !errors.Is(err, ports.ErrNotFound) {
		return contract.CheckpointPhaseFact{}, err
	}
	s1, err := o.settlements.InspectCheckpointPhaseSettlementCurrentV2(ctx, domain.ExactRef(), request.RuntimeSettlementRef)
	if err != nil {
		return contract.CheckpointPhaseFact{}, err
	}
	if err := validateCheckpointSettlementProjectionV2(s1, request, now); err != nil {
		return contract.CheckpointPhaseFact{}, err
	}
	fresh := o.clock()
	s2, err := o.settlements.InspectCheckpointPhaseSettlementCurrentV2(ctx, domain.ExactRef(), request.RuntimeSettlementRef)
	if err != nil {
		return contract.CheckpointPhaseFact{}, err
	}
	if err := validateCheckpointSettlementProjectionV2(s2, request, fresh); err != nil || s1.ProjectionDigest != s2.ProjectionDigest {
		if err != nil {
			return contract.CheckpointPhaseFact{}, err
		}
		return contract.CheckpointPhaseFact{}, fmt.Errorf("%w: Runtime checkpoint Settlement current changed between S1 and S2", ports.ErrConflict)
	}
	participant, err := o.phases.InspectCheckpointParticipantCurrent(ctx, reservation.ParticipantRef.ID)
	if err != nil || participant.ActiveReservation.Ref == nil || !contract.SameRef(*participant.ActiveReservation.Ref, reservation.Meta.Ref()) || participant.Meta.Revision != reservation.ExpectedParticipantRevision+1 {
		if recovered, inspectErr := o.phases.InspectCheckpointPhaseFactByReservation(context.WithoutCancel(ctx), reservation.Meta.Ref()); inspectErr == nil {
			return recovered, recovered.ValidateShape()
		}
		return contract.CheckpointPhaseFact{}, fmt.Errorf("%w: checkpoint Participant current no longer owns the reservation", ports.ErrConflict)
	}
	expires := min(domain.Meta.ExpiresUnixNano, reservation.Meta.ExpiresUnixNano, participant.Meta.ExpiresUnixNano, s2.ExpiresUnixNano)
	applyDigest, err := contract.Digest("praxis.sandbox/checkpoint-phase-apply-settlement/ref/v2", struct {
		Domain     contract.SnapshotArtifactExactRefV2
		Settlement contract.Ref
	}{domain.ExactRef(), request.RuntimeSettlementRef})
	if err != nil {
		return contract.CheckpointPhaseFact{}, err
	}
	applyRef := contract.Ref{ID: request.RuntimeSettlementRef.ID + "-apply", Revision: 1, Digest: applyDigest}
	fact, err := contract.SealCheckpointPhaseFact(contract.CheckpointPhaseFact{
		Meta:           contract.Meta{ContractVersion: contract.ContractFamily, ID: reservation.Meta.ID + "-phase-fact", Revision: 1, CreatedUnixNano: fresh.UnixNano(), UpdatedUnixNano: fresh.UnixNano(), ExpiresUnixNano: expires},
		ReservationRef: reservation.Meta.Ref(), TenantID: reservation.TenantID, ParticipantRef: participant.Meta.Ref(), CheckpointAttemptRef: reservation.Base.CheckpointAttempt,
		Phase: reservation.Phase, PreviousPresence: reservation.PreviousPresence, PreviousPhase: reservation.PreviousPhase,
		OperationID: reservation.OperationID, EffectID: reservation.EffectID, AttemptID: reservation.AttemptID, State: domain.State,
		EvidenceRefs: []contract.Ref{domain.EvidenceConsumption}, DomainResultRef: contract.Ref{ID: domain.Meta.ID, Revision: domain.Meta.Revision, Digest: domain.Meta.Digest}, RuntimeSettlementRef: request.RuntimeSettlementRef, ApplySettlementRef: applyRef,
	})
	if err != nil {
		return contract.CheckpointPhaseFact{}, err
	}
	next, err := cloneCheckpointValue(participant)
	if err != nil {
		return contract.CheckpointPhaseFact{}, err
	}
	next.Meta.Revision++
	next.Meta.UpdatedUnixNano = fresh.UnixNano()
	next.Meta.ExpiresUnixNano = expires
	next.State = fact.ParticipantState()
	closure := fact.ClosureRef()
	next.Closure = &closure
	next, err = contract.SealCheckpointParticipantFact(next)
	if err != nil {
		return contract.CheckpointPhaseFact{}, err
	}
	created, createErr := o.results.CommitCheckpointPhaseApplySettlementV2(ctx, participant.Meta.Ref(), fact, next)
	if createErr == nil && created {
		return fact, nil
	}
	recovered, inspectErr := o.phases.InspectCheckpointPhaseFactByReservation(context.WithoutCancel(ctx), reservation.Meta.Ref())
	if inspectErr == nil && contract.SameRef(recovered.Meta.Ref(), fact.Meta.Ref()) {
		return recovered, nil
	}
	if createErr != nil {
		return contract.CheckpointPhaseFact{}, createErr
	}
	if inspectErr != nil {
		return contract.CheckpointPhaseFact{}, inspectErr
	}
	return contract.CheckpointPhaseFact{}, fmt.Errorf("%w: checkpoint ApplySettlement winner differs", ports.ErrConflict)
}

func validateCheckpointPhaseResultProjectionV2(projection contract.CheckpointPhaseResultCurrentProjectionV2, reservation contract.CheckpointPhaseReservation, request contract.RecordCheckpointPhaseDomainResultRequestV2, now time.Time) error {
	if err := projection.ValidateCurrent(now); err != nil {
		return err
	}
	if !contract.SameRef(projection.ReservationRef, reservation.Meta.Ref()) || projection.ProjectionDigest != request.ExpectedProjectionDigest || projection.ExpiresUnixNano > request.RequestedNotAfter || projection.State.ValidateFor(reservation.Phase) != nil {
		return fmt.Errorf("%w: checkpoint phase result current crosses reservation", ports.ErrConflict)
	}
	return nil
}

func validateCheckpointSettlementProjectionV2(projection contract.CheckpointPhaseSettlementCurrentProjectionV2, request contract.CheckpointPhaseApplySettlementV2, now time.Time) error {
	if err := projection.ValidateCurrent(now); err != nil {
		return err
	}
	if projection.DomainResultRef != request.DomainResultRef || !contract.SameRef(projection.RuntimeSettlementRef, request.RuntimeSettlementRef) {
		return fmt.Errorf("%w: Runtime checkpoint Settlement current crosses DomainResult", ports.ErrConflict)
	}
	return nil
}

var _ ports.CheckpointPhaseResultOwnerPortV2 = (*CheckpointPhaseResultOwnerV2)(nil)

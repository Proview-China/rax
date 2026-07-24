package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

// CheckpointController deliberately exposes no RecordPhaseFact or reconciliation
// write method. Until the cross-owner ApplySettlement chain exists, only an
// already-applied, append-only Owner Fact may unlock a successor reservation.
type CheckpointController struct {
	store ports.CheckpointPhaseStore
	now   func() time.Time
}

func NewCheckpointController(store ports.CheckpointPhaseStore, now func() time.Time) (*CheckpointController, error) {
	if nilInterface(store) || now == nil {
		return nil, errors.New("checkpoint phase store and clock are required")
	}
	return &CheckpointController{store: store, now: now}, nil
}

func (c *CheckpointController) ReservePhase(ctx context.Context, input *contract.CheckpointPhaseReservation) (contract.CheckpointPhaseReservation, error) {
	if input == nil {
		return contract.CheckpointPhaseReservation{}, errors.New("checkpoint reservation is required")
	}
	reservation, err := cloneCheckpointValue(*input)
	if err != nil {
		return contract.CheckpointPhaseReservation{}, err
	}
	now := c.now()
	if err := reservation.ValidateCurrent(now); err != nil {
		return contract.CheckpointPhaseReservation{}, fmt.Errorf("validate checkpoint reservation: %w", err)
	}
	if existing, inspectErr := c.store.InspectCheckpointPhaseReservation(ctx, reservation.Meta.Ref()); inspectErr == nil {
		current, currentErr := c.store.InspectCheckpointParticipantCurrent(ctx, reservation.ParticipantRef.ID)
		if currentErr == nil && current.ActiveReservation.Ref != nil && contract.SameRef(*current.ActiveReservation.Ref, reservation.Meta.Ref()) {
			return existing, nil
		}
		return contract.CheckpointPhaseReservation{}, fmt.Errorf("%w: checkpoint reservation exists but participant owner current drifted", ports.ErrConflict)
	} else if !errors.Is(inspectErr, ports.ErrNotFound) {
		return contract.CheckpointPhaseReservation{}, inspectErr
	}
	participant, err := c.store.InspectCheckpointParticipantCurrent(ctx, reservation.ParticipantRef.ID)
	if err != nil {
		return contract.CheckpointPhaseReservation{}, err
	}
	if err := participant.ValidateCurrent(now); err != nil {
		return contract.CheckpointPhaseReservation{}, fmt.Errorf("validate checkpoint participant: %w", err)
	}
	if !contract.SameRef(participant.Meta.Ref(), reservation.ParticipantRef) ||
		participant.Meta.Revision != reservation.ExpectedParticipantRevision ||
		participant.TenantID != reservation.TenantID ||
		!contract.SameRef(participant.CheckpointAttemptRef, reservation.Base.CheckpointAttempt) {
		return contract.CheckpointPhaseReservation{}, fmt.Errorf("%w: checkpoint participant current drifted", ports.ErrConflict)
	}
	if reservation.Meta.ExpiresUnixNano > participant.Meta.ExpiresUnixNano {
		return contract.CheckpointPhaseReservation{}, fmt.Errorf("%w: checkpoint reservation extends participant TTL", ports.ErrStale)
	}
	if reservation.PreviousPresence == contract.CheckpointPresent {
		if participant.Closure == nil || !contract.SameCheckpointPhaseClosure(*participant.Closure, *reservation.PreviousPhase) || participant.State != contract.CheckpointParticipantPrepared {
			return contract.CheckpointPhaseReservation{}, fmt.Errorf("%w: participant Owner current does not expose the exact prepared closure", ports.ErrConflict)
		}
		previous, inspectErr := c.store.InspectCheckpointPhaseFact(ctx, reservation.PreviousPhase.Ref)
		if inspectErr != nil {
			return contract.CheckpointPhaseReservation{}, inspectErr
		}
		if err := previous.ValidateCurrent(now); err != nil {
			return contract.CheckpointPhaseReservation{}, fmt.Errorf("validate previous checkpoint phase: %w", err)
		}
		if !contract.SameCheckpointPhaseClosure(previous.ClosureRef(), *reservation.PreviousPhase) ||
			previous.Phase != contract.CheckpointPhasePrepare || previous.State != contract.CheckpointPhasePrepared ||
			previous.TenantID != reservation.TenantID || previous.ParticipantRef.ID != reservation.ParticipantRef.ID ||
			!contract.SameRef(previous.CheckpointAttemptRef, reservation.Base.CheckpointAttempt) {
			return contract.CheckpointPhaseReservation{}, fmt.Errorf("%w: checkpoint successor does not bind exact Owner-applied prepared closure", ErrInvalidTransition)
		}
	}
	nextParticipant, err := reservedCheckpointParticipant(participant, reservation, now)
	if err != nil {
		return contract.CheckpointPhaseReservation{}, err
	}
	created, err := c.store.ReserveCheckpointPhase(ctx, participant.Meta.Ref(), reservation, nextParticipant)
	if err != nil {
		if recovered, inspectErr := c.store.InspectCheckpointPhaseReservation(ctx, reservation.Meta.Ref()); inspectErr == nil {
			current, currentErr := c.store.InspectCheckpointParticipantCurrent(ctx, participant.Meta.ID)
			if currentErr == nil && contract.SameRef(recovered.Meta.Ref(), reservation.Meta.Ref()) && contract.SameRef(current.Meta.Ref(), nextParticipant.Meta.Ref()) {
				return recovered, nil
			}
		}
		return contract.CheckpointPhaseReservation{}, err
	}
	if !created {
		existing, inspectErr := c.store.InspectCheckpointPhaseReservation(ctx, reservation.Meta.Ref())
		if inspectErr != nil {
			return contract.CheckpointPhaseReservation{}, inspectErr
		}
		if !contract.SameRef(existing.Meta.Ref(), reservation.Meta.Ref()) {
			return contract.CheckpointPhaseReservation{}, fmt.Errorf("%w: checkpoint reservation replay changed content", ports.ErrConflict)
		}
		return existing, nil
	}
	return reservation, nil
}

func reservedCheckpointParticipant(current contract.CheckpointParticipantFact, reservation contract.CheckpointPhaseReservation, now time.Time) (contract.CheckpointParticipantFact, error) {
	next, err := cloneCheckpointValue(current)
	if err != nil {
		return contract.CheckpointParticipantFact{}, err
	}
	next.Meta.Revision++
	next.Meta.UpdatedUnixNano = now.UnixNano()
	next.Meta.ExpiresUnixNano = min(next.Meta.ExpiresUnixNano, reservation.Meta.ExpiresUnixNano)
	next.ActivePhase = reservation.Phase
	ref := reservation.Meta.Ref()
	next.ActiveReservation = contract.CheckpointOptionalRef{Presence: contract.CheckpointPresent, Ref: &ref}
	if reservation.Phase == contract.CheckpointPhasePrepare {
		next.Closure = nil
	}
	return contract.SealCheckpointParticipantFact(next)
}

func (c *CheckpointController) InspectParticipantState(ctx context.Context, participantID string) (contract.CheckpointParticipantState, error) {
	participant, err := c.store.InspectCheckpointParticipantCurrent(ctx, participantID)
	if err != nil {
		return "", err
	}
	if err := participant.ValidateShape(); err != nil {
		return "", err
	}
	return participant.State, nil
}

type CheckpointCurrentReader struct {
	store  ports.CheckpointPhaseStore
	source ports.CheckpointCurrentSource
	now    func() time.Time
}

func NewCheckpointCurrentReader(store ports.CheckpointPhaseStore, source ports.CheckpointCurrentSource, now func() time.Time) (*CheckpointCurrentReader, error) {
	if nilInterface(store) || nilInterface(source) || now == nil {
		return nil, errors.New("checkpoint phase store, current source, and clock are required")
	}
	return &CheckpointCurrentReader{store: store, source: source, now: now}, nil
}

func (r *CheckpointCurrentReader) ReadCheckpointParticipantCurrent(ctx context.Context, input *contract.CheckpointCurrentReadRequest) (contract.CheckpointParticipantCurrentProjection, error) {
	if input == nil {
		return contract.CheckpointParticipantCurrentProjection{}, errors.New("checkpoint current request is required")
	}
	request, err := cloneCheckpointValue(*input)
	if err != nil {
		return contract.CheckpointParticipantCurrentProjection{}, err
	}
	if err := request.ValidateShape(); err != nil {
		return contract.CheckpointParticipantCurrentProjection{}, fmt.Errorf("validate checkpoint current request: %w", err)
	}
	now := r.now()
	reservation, err := r.store.InspectCheckpointPhaseReservation(ctx, request.ExpectedReservationRef)
	if err != nil {
		return contract.CheckpointParticipantCurrentProjection{}, err
	}
	if err := reservation.ValidateCurrent(now); err != nil {
		return contract.CheckpointParticipantCurrentProjection{}, fmt.Errorf("validate current checkpoint reservation: %w", err)
	}
	participant, err := r.store.InspectCheckpointParticipantCurrent(ctx, request.ParticipantRef.ID)
	if err != nil {
		return contract.CheckpointParticipantCurrentProjection{}, err
	}
	if err := participant.ValidateCurrent(now); err != nil {
		return contract.CheckpointParticipantCurrentProjection{}, err
	}
	if !contract.SameRef(participant.Meta.Ref(), request.ParticipantRef) {
		return contract.CheckpointParticipantCurrentProjection{}, fmt.Errorf("%w: checkpoint participant is no longer current", ports.ErrStale)
	}
	if !sameCheckpointRequestReservation(request, reservation, participant) {
		return contract.CheckpointParticipantCurrentProjection{}, fmt.Errorf("%w: checkpoint current request binds another reservation or participant revision", ports.ErrStale)
	}
	if err := matchCheckpointPreviousPhase(reservation.PreviousPresence, reservation.PreviousPhase, request.PreviousPresence, request.ExpectedPreviousPhase); err != nil {
		return contract.CheckpointParticipantCurrentProjection{}, err
	}
	expires := min(reservation.Meta.ExpiresUnixNano, participant.Meta.ExpiresUnixNano)
	if reservation.PreviousPresence == contract.CheckpointPresent {
		previous, inspectErr := r.store.InspectCheckpointPhaseFact(ctx, reservation.PreviousPhase.Ref)
		if inspectErr != nil {
			return contract.CheckpointParticipantCurrentProjection{}, inspectErr
		}
		if err := previous.ValidateCurrent(now); err != nil {
			return contract.CheckpointParticipantCurrentProjection{}, err
		}
		if !contract.SameCheckpointPhaseClosure(previous.ClosureRef(), *reservation.PreviousPhase) || previous.State != contract.CheckpointPhasePrepared {
			return contract.CheckpointParticipantCurrentProjection{}, fmt.Errorf("%w: previous checkpoint phase is not exact prepared closure", ports.ErrStale)
		}
		expires = min(expires, previous.Meta.ExpiresUnixNano)
	}
	base := checkpointBaseRefMap(reservation)
	current := make([]contract.CheckpointCurrentCoordinate, 0, len(request.ExpectedCurrentRefs))
	absent := make([]contract.CheckpointCurrentKind, 0, len(request.ExpectedCurrentRefs))
	query := contract.CheckpointCurrentQuery{
		TenantID:                  reservation.TenantID,
		ParticipantID:             reservation.ParticipantRef.ID,
		CheckpointAttemptRef:      reservation.Base.CheckpointAttempt,
		Phase:                     reservation.Phase,
		OperationID:               reservation.OperationID,
		EffectID:                  reservation.EffectID,
		AttemptID:                 reservation.AttemptID,
		ExpectedRuntimeAttemptRef: reservation.ExpectedRuntimeAttemptRef,
	}
	for _, expected := range request.ExpectedCurrentRefs {
		query.Kind = expected.Kind
		coordinate, inspectErr := r.source.InspectCheckpointCurrent(ctx, query)
		if expected.Presence == contract.CheckpointAbsent {
			if inspectErr == nil {
				return contract.CheckpointParticipantCurrentProjection{}, fmt.Errorf("%w: checkpoint gate %s must be absent at %s", ports.ErrConflict, expected.Kind, request.Stage)
			}
			if !errors.Is(inspectErr, ports.ErrNotFound) {
				return contract.CheckpointParticipantCurrentProjection{}, inspectErr
			}
			absent = append(absent, expected.Kind)
			continue
		}
		if inspectErr != nil {
			return contract.CheckpointParticipantCurrentProjection{}, inspectErr
		}
		if err := coordinate.ValidateCurrent(now); err != nil {
			return contract.CheckpointParticipantCurrentProjection{}, fmt.Errorf("validate checkpoint current %s: %w", expected.Kind, err)
		}
		if expected.Ref == nil || !contract.SameRef(coordinate.Meta.Ref(), *expected.Ref) {
			return contract.CheckpointParticipantCurrentProjection{}, fmt.Errorf("%w: checkpoint current %s exact ref drifted", ports.ErrStale, expected.Kind)
		}
		if baseRef, bound := base[expected.Kind]; bound && !sameOptionalCheckpointRef(baseRef, contract.CheckpointOptionalRef{Presence: expected.Presence, Ref: expected.Ref}) {
			return contract.CheckpointParticipantCurrentProjection{}, fmt.Errorf("%w: checkpoint base current ref drifted for %s", ports.ErrStale, expected.Kind)
		}
		if !sameCheckpointCoordinate(coordinate, reservation) {
			return contract.CheckpointParticipantCurrentProjection{}, fmt.Errorf("%w: checkpoint current %s binds another phase/effect/runtime coordinate", ports.ErrStale, expected.Kind)
		}
		expires = min(expires, coordinate.Meta.ExpiresUnixNano)
		current = append(current, coordinate)
	}
	slices.SortFunc(current, func(a, b contract.CheckpointCurrentCoordinate) int { return cmpCheckpointKind(a.Kind, b.Kind) })
	slices.Sort(absent)
	projection, err := contract.SealCheckpointParticipantCurrentProjection(contract.CheckpointParticipantCurrentProjection{
		TenantID:                  reservation.TenantID,
		ReservationRef:            reservation.Meta.Ref(),
		ParticipantRef:            participant.Meta.Ref(),
		CheckpointAttemptRef:      reservation.Base.CheckpointAttempt,
		Phase:                     reservation.Phase,
		Stage:                     request.Stage,
		PreviousPresence:          reservation.PreviousPresence,
		PreviousPhase:             reservation.PreviousPhase,
		OperationID:               reservation.OperationID,
		EffectID:                  reservation.EffectID,
		AttemptID:                 reservation.AttemptID,
		ExpectedRuntimeAttemptRef: reservation.ExpectedRuntimeAttemptRef,
		Runtime:                   reservation.Runtime,
		ChangeSet:                 reservation.ChangeSet,
		Watermarks:                reservation.Watermarks,
		Current:                   current,
		Absent:                    absent,
		ProjectionRevision:        participant.Meta.Revision,
		OwnerComputedCurrent:      true,
		CheckedUnixNano:           now.UnixNano(),
		ExpiresUnixNano:           expires,
	})
	if err != nil {
		return contract.CheckpointParticipantCurrentProjection{}, err
	}
	if err := projection.ValidateCurrent(now); err != nil {
		return contract.CheckpointParticipantCurrentProjection{}, err
	}
	return projection, nil
}

func sameCheckpointRequestReservation(request contract.CheckpointCurrentReadRequest, reservation contract.CheckpointPhaseReservation, participant contract.CheckpointParticipantFact) bool {
	return contract.SameRef(reservation.Meta.Ref(), request.ExpectedReservationRef) &&
		reservation.TenantID == request.TenantID && reservation.ParticipantRef.ID == request.ParticipantRef.ID &&
		participant.ActiveReservation.Ref != nil && contract.SameRef(*participant.ActiveReservation.Ref, reservation.Meta.Ref()) &&
		contract.SameRef(reservation.Base.CheckpointAttempt, request.CheckpointAttemptRef) && reservation.Phase == request.Phase &&
		reservation.OperationID == request.OperationID && reservation.EffectID == request.EffectID && reservation.AttemptID == request.AttemptID &&
		contract.SameRef(reservation.ExpectedRuntimeAttemptRef, request.ExpectedRuntimeAttempt) && reservation.Runtime == request.Runtime &&
		sameOptionalCheckpointRef(reservation.ChangeSet, request.ChangeSet) && slices.Equal(reservation.Watermarks, request.Watermarks)
}

func matchCheckpointPreviousPhase(actualPresence contract.CheckpointPresence, actual *contract.CheckpointPhaseClosureRef, expectedPresence contract.CheckpointPresence, expected *contract.CheckpointPhaseClosureRef) error {
	if actualPresence != expectedPresence || (actual == nil) != (expected == nil) {
		return fmt.Errorf("%w: checkpoint previous phase presence drifted", ports.ErrStale)
	}
	if actual != nil && !contract.SameCheckpointPhaseClosure(*actual, *expected) {
		return fmt.Errorf("%w: checkpoint previous phase closure drifted", ports.ErrStale)
	}
	return nil
}

func checkpointBaseRefMap(reservation contract.CheckpointPhaseReservation) map[contract.CheckpointCurrentKind]contract.CheckpointOptionalRef {
	present := func(ref contract.Ref) contract.CheckpointOptionalRef {
		return contract.CheckpointOptionalRef{Presence: contract.CheckpointPresent, Ref: &ref}
	}
	return map[contract.CheckpointCurrentKind]contract.CheckpointOptionalRef{
		contract.CheckpointCurrentCheckpointAttempt: present(reservation.Base.CheckpointAttempt),
		contract.CheckpointCurrentBarrier:           present(reservation.Base.Barrier),
		contract.CheckpointCurrentEffectCut:         present(reservation.Base.EffectCut),
		contract.CheckpointCurrentRuntimeLease:      present(reservation.Base.RuntimeLeaseBinding),
		contract.CheckpointCurrentRequirement:       present(reservation.Base.Requirement),
		contract.CheckpointCurrentPolicy:            present(reservation.Base.Policy),
		contract.CheckpointCurrentWorkspace:         present(reservation.Base.Workspace),
		contract.CheckpointCurrentChangeSet:         reservation.ChangeSet,
		contract.CheckpointCurrentPlacement:         present(reservation.Base.Placement),
		contract.CheckpointCurrentBackend:           present(reservation.Base.Backend),
		contract.CheckpointCurrentSlot:              present(reservation.Base.Slot),
		contract.CheckpointCurrentGeneration:        present(reservation.Base.Generation),
		contract.CheckpointCurrentAttempt:           present(reservation.ExpectedRuntimeAttemptRef),
	}
}

func sameCheckpointCoordinate(coordinate contract.CheckpointCurrentCoordinate, reservation contract.CheckpointPhaseReservation) bool {
	return coordinate.TenantID == reservation.TenantID && coordinate.ParticipantID == reservation.ParticipantRef.ID &&
		contract.SameRef(coordinate.CheckpointAttemptRef, reservation.Base.CheckpointAttempt) && coordinate.Phase == reservation.Phase &&
		coordinate.OperationID == reservation.OperationID && coordinate.EffectID == reservation.EffectID && coordinate.AttemptID == reservation.AttemptID &&
		contract.SameRef(coordinate.ExpectedRuntimeAttemptRef, reservation.ExpectedRuntimeAttemptRef) &&
		coordinate.Runtime == reservation.Runtime && sameOptionalCheckpointRef(coordinate.ChangeSet, reservation.ChangeSet) &&
		slices.Equal(coordinate.Watermarks, reservation.Watermarks)
}

func sameOptionalCheckpointRef(a, b contract.CheckpointOptionalRef) bool {
	if a.Presence != b.Presence || (a.Ref == nil) != (b.Ref == nil) {
		return false
	}
	return a.Ref == nil || contract.SameRef(*a.Ref, *b.Ref)
}

func cmpCheckpointKind(a, b contract.CheckpointCurrentKind) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func cloneCheckpointValue[T any](value T) (T, error) {
	var zero T
	payload, err := json.Marshal(value)
	if err != nil {
		return zero, err
	}
	var result T
	if err := json.Unmarshal(payload, &result); err != nil {
		return zero, err
	}
	return result, nil
}

func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

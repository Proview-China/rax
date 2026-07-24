package kernel

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

var ErrInvalidTransition = errors.New("sandbox invalid transition")

type Controller struct {
	store ports.FactStore
	now   func() time.Time
}

func NewController(store ports.FactStore, now func() time.Time) (*Controller, error) {
	if store == nil || now == nil {
		return nil, errors.New("fact store and clock are required")
	}
	return &Controller{store: store, now: now}, nil
}

func (c *Controller) Reserve(ctx context.Context, reservation contract.DomainReservation) error {
	now := c.now()
	if err := reservation.ValidateCurrent(now); err != nil {
		return fmt.Errorf("validate reservation: %w", err)
	}
	projection, err := c.store.GetProjection(ctx, reservation.Lease.LeaseID)
	if err != nil {
		return err
	}
	if err := projection.ValidateCurrent(now); err != nil {
		return fmt.Errorf("validate current projection: %w", err)
	}
	if projection.Meta.Revision != reservation.ExpectedProjectionRevision {
		return fmt.Errorf("%w: expected projection revision %d, got %d", ports.ErrStale, reservation.ExpectedProjectionRevision, projection.Meta.Revision)
	}
	if !contract.SameRuntimeLeaseBinding(projection.Lease, reservation.Lease) {
		return fmt.Errorf("%w: runtime lease binding changed", ports.ErrStale)
	}
	if projection.LastDomainResultRef.ID != "" {
		lastResult, err := c.store.GetDomainResult(ctx, projection.LastDomainResultRef.ID)
		if err != nil {
			return err
		}
		if !contract.SameRef(lastResult.Meta.Ref(), projection.LastDomainResultRef) {
			return fmt.Errorf("%w: projection domain result ref is not exact", ErrInvalidTransition)
		}
		if lastResult.Disposition == contract.DispositionUnknown {
			return fmt.Errorf("%w: indeterminate attempt may only be inspected", ErrInvalidTransition)
		}
	}
	return c.store.CreateReservation(ctx, reservation)
}

func (c *Controller) RecordObservation(ctx context.Context, observation contract.Observation) (bool, error) {
	if err := observation.ValidateCurrent(c.now()); err != nil {
		return false, fmt.Errorf("validate observation: %w", err)
	}
	reservation, err := c.store.GetReservation(ctx, observation.ReservationRef.ID)
	if err != nil {
		return false, err
	}
	if err := matchReservationObservation(reservation, observation); err != nil {
		return false, err
	}
	return c.store.AppendObservation(ctx, reservation.Meta.ID, observation)
}

func (c *Controller) RecordInspection(ctx context.Context, inspection contract.InspectionFact) error {
	if err := inspection.ValidateCurrent(c.now()); err != nil {
		return fmt.Errorf("validate inspection: %w", err)
	}
	reservation, err := c.store.GetReservation(ctx, inspection.ReservationRef.ID)
	if err != nil {
		return err
	}
	observation, err := c.store.GetObservation(ctx, inspection.ObservationRef.ID)
	if err != nil {
		return err
	}
	if !contract.SameRef(reservation.Meta.Ref(), inspection.ReservationRef) ||
		!contract.SameRef(observation.Meta.Ref(), inspection.ObservationRef) ||
		reservation.OperationID != inspection.OperationID || reservation.AttemptID != inspection.AttemptID ||
		observation.OperationID != inspection.OperationID || observation.AttemptID != inspection.AttemptID {
		return fmt.Errorf("%w: inspection does not exactly bind reservation and observation", ErrInvalidTransition)
	}
	return c.store.CreateInspection(ctx, inspection)
}

func (c *Controller) CommitDomainResult(ctx context.Context, result contract.SandboxDomainResultFact) error {
	if err := result.ValidateCurrent(c.now()); err != nil {
		return fmt.Errorf("validate domain result: %w", err)
	}
	reservation, err := c.store.GetReservation(ctx, result.ReservationRef.ID)
	if err != nil {
		return err
	}
	inspection, err := c.store.GetInspection(ctx, result.InspectionRef.ID)
	if err != nil {
		return err
	}
	if !contract.SameRef(reservation.Meta.Ref(), result.ReservationRef) ||
		!contract.SameRef(inspection.Meta.Ref(), result.InspectionRef) ||
		reservation.OperationID != result.OperationID || reservation.AttemptID != result.AttemptID ||
		inspection.OperationID != result.OperationID || inspection.AttemptID != result.AttemptID ||
		reservation.Kind != result.Kind || inspection.Disposition != result.Disposition ||
		!contract.SameRuntimeLeaseBinding(reservation.Lease, result.Lease) {
		return fmt.Errorf("%w: domain result does not exactly bind reservation, inspection, and runtime lease", ErrInvalidTransition)
	}
	if result.Kind == contract.EffectCleanup && !reflect.DeepEqual(inspection.Cleanup, result.Payload.Cleanup) {
		return fmt.Errorf("%w: cleanup domain result differs from inspected cleanup", ErrInvalidTransition)
	}
	return c.store.CreateDomainResult(ctx, result)
}

// ApplySettlement consumes only a Runtime Operation Settlement reference. It
// never creates or mutates Runtime state and never treats the DomainResultFact
// itself as a Runtime settlement.
func (c *Controller) ApplySettlement(ctx context.Context, resultID string, settlement contract.RuntimeOperationSettlementRef) (contract.EnvironmentProjection, error) {
	if err := settlement.ValidateShape(); err != nil {
		return contract.EnvironmentProjection{}, fmt.Errorf("validate runtime settlement ref: %w", err)
	}
	result, err := c.store.GetDomainResult(ctx, resultID)
	if err != nil {
		return contract.EnvironmentProjection{}, err
	}
	reservation, err := c.store.GetReservation(ctx, result.ReservationRef.ID)
	if err != nil {
		return contract.EnvironmentProjection{}, err
	}
	if !contract.SameRef(result.Meta.Ref(), settlement.DomainResultRef) ||
		result.OperationID != settlement.OperationID || result.AttemptID != settlement.AttemptID {
		return contract.EnvironmentProjection{}, fmt.Errorf("%w: runtime settlement does not exactly reference domain result", ErrInvalidTransition)
	}
	if appliedResultRef, err := c.store.GetSettlementBinding(ctx, settlement.OpaqueRef); err == nil {
		if !contract.SameRef(appliedResultRef, result.Meta.Ref()) {
			return contract.EnvironmentProjection{}, fmt.Errorf("%w: opaque runtime settlement ref is already bound", ports.ErrConflict)
		}
		return c.currentProjection(ctx, reservation.Lease.LeaseID)
	} else if !errors.Is(err, ports.ErrNotFound) {
		return contract.EnvironmentProjection{}, err
	}
	projection, err := c.store.GetProjection(ctx, reservation.Lease.LeaseID)
	if err != nil {
		return contract.EnvironmentProjection{}, err
	}
	if err := projection.ValidateShape(); err != nil {
		return contract.EnvironmentProjection{}, fmt.Errorf("validate historical projection shape: %w", err)
	}
	if contract.SameRef(projection.LastDomainResultRef, result.Meta.Ref()) && contract.SameRef(projection.LastSettlementRef, settlement.OpaqueRef) {
		return projection, nil
	}
	if projection.Meta.Revision != reservation.ExpectedProjectionRevision {
		return contract.EnvironmentProjection{}, fmt.Errorf("%w: expected projection revision %d, got %d", ports.ErrStale, reservation.ExpectedProjectionRevision, projection.Meta.Revision)
	}
	if !contract.SameRuntimeLeaseBinding(projection.Lease, reservation.Lease) {
		return contract.EnvironmentProjection{}, fmt.Errorf("%w: runtime lease binding changed", ports.ErrStale)
	}
	next, err := applyResult(projection, result)
	if err != nil {
		return contract.EnvironmentProjection{}, err
	}
	next.LastDomainResultRef = result.Meta.Ref()
	next.LastSettlementRef = settlement.OpaqueRef
	payload := struct {
		LeaseID             string
		PreviousRevision    uint64
		ResultRef           contract.Ref
		OpaqueSettlementRef contract.Ref
		State               contract.EnvironmentProjection
	}{
		LeaseID:             projection.Lease.LeaseID,
		PreviousRevision:    projection.Meta.Revision,
		ResultRef:           result.Meta.Ref(),
		OpaqueSettlementRef: settlement.OpaqueRef,
		State:               next,
	}
	nextMeta, err := contract.NextMeta(projection.Meta, c.now(), "environment-projection", payload)
	if err != nil {
		return contract.EnvironmentProjection{}, err
	}
	next.Meta = nextMeta
	if err := next.ValidateShape(); err != nil {
		return contract.EnvironmentProjection{}, fmt.Errorf("validate next projection: %w", err)
	}
	if err := c.store.CompareAndSwapProjection(ctx, projection.Meta.Revision, next); err != nil {
		// A successful CAS can lose its reply, and an exact concurrent replay can
		// observe the winner only after its own CAS reports stale. The durable
		// opaque-ref binding, not the transport error, decides idempotence.
		if appliedResultRef, lookupErr := c.store.GetSettlementBinding(ctx, settlement.OpaqueRef); lookupErr == nil {
			if !contract.SameRef(appliedResultRef, result.Meta.Ref()) {
				return contract.EnvironmentProjection{}, fmt.Errorf("%w: opaque runtime settlement ref is already bound", ports.ErrConflict)
			}
			return c.currentProjection(ctx, reservation.Lease.LeaseID)
		}
		return contract.EnvironmentProjection{}, err
	}
	return next, nil
}

func (c *Controller) currentProjection(ctx context.Context, leaseID string) (contract.EnvironmentProjection, error) {
	projection, err := c.store.GetProjection(ctx, leaseID)
	if err != nil {
		return contract.EnvironmentProjection{}, err
	}
	if err := projection.ValidateShape(); err != nil {
		return contract.EnvironmentProjection{}, fmt.Errorf("validate historical projection shape: %w", err)
	}
	return projection, nil
}

func matchReservationObservation(reservation contract.DomainReservation, observation contract.Observation) error {
	if !contract.SameRef(reservation.Meta.Ref(), observation.ReservationRef) ||
		reservation.OperationID != observation.OperationID || reservation.AttemptID != observation.AttemptID {
		return fmt.Errorf("%w: observation does not exactly bind reservation", ErrInvalidTransition)
	}
	return nil
}

func applyResult(projection contract.EnvironmentProjection, result contract.SandboxDomainResultFact) (contract.EnvironmentProjection, error) {
	next := projection
	if result.Disposition != contract.DispositionConfirmedApplied {
		return next, nil
	}
	switch result.Kind {
	case contract.EffectAllocate:
		if projection.Allocated || projection.Fenced || projection.Released {
			return next, fmt.Errorf("%w: allocation requires a fresh projection", ErrInvalidTransition)
		}
		next.Allocated = true
	case contract.EffectActivate:
		if !projection.Allocated || projection.Activated || projection.EnvironmentClosed || projection.Fenced {
			return next, fmt.Errorf("%w: activation requires allocated, inactive, openable environment", ErrInvalidTransition)
		}
		next.Activated = true
	case contract.EffectOpen:
		if !projection.Activated || projection.Open || projection.EnvironmentClosed || projection.Fenced {
			return next, fmt.Errorf("%w: open requires active, unfenced, non-closed environment", ErrInvalidTransition)
		}
		next.Open = true
		next.ExecutionQuiesced = false
	case contract.EffectCancel:
		if !projection.Open || projection.EnvironmentClosed {
			return next, fmt.Errorf("%w: cancel requires an open environment", ErrInvalidTransition)
		}
		next.ExecutionQuiesced = true
	case contract.EffectClose:
		if !projection.ExecutionQuiesced || projection.EnvironmentClosed {
			return next, fmt.Errorf("%w: close requires separately proven execution quiescence", ErrInvalidTransition)
		}
		next.EnvironmentClosed = true
		next.Open = false
	case contract.EffectFence:
		if projection.Fenced {
			return next, fmt.Errorf("%w: environment is already fenced", ErrInvalidTransition)
		}
		next.Fenced = true
	case contract.EffectRelease:
		if !projection.ExecutionQuiesced || !projection.EnvironmentClosed || !projection.Fenced || projection.Released {
			return next, fmt.Errorf("%w: release requires quiesced, closed, fenced, unreleased environment", ErrInvalidTransition)
		}
		next.Released = true
	case contract.EffectCleanup:
		if !projection.Released || result.Payload.Cleanup == nil {
			return next, fmt.Errorf("%w: cleanup requires released environment and seven-dimensional report", ErrInvalidTransition)
		}
		next.Cleanup = *result.Payload.Cleanup
	case contract.EffectInspect:
		if result.Payload.ExecutionQuiesced {
			if !projection.Open || projection.EnvironmentClosed {
				return next, fmt.Errorf("%w: quiescence inspection requires open, non-closed environment", ErrInvalidTransition)
			}
			next.ExecutionQuiesced = true
		}
	case contract.EffectWorkspaceCommit:
		// Workspace settlement does not change environment lifecycle fields.
	default:
		return next, fmt.Errorf("%w: unsupported effect kind %q", ErrInvalidTransition, result.Kind)
	}
	return next, nil
}

package api

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"
)

type ServiceV1 struct {
	store   OperationStoreV1
	handler GovernedHandlerV1
	clock   func() time.Time
}

func NewServiceV1(store OperationStoreV1, handler GovernedHandlerV1, clock func() time.Time) (*ServiceV1, error) {
	if nilLike(store) || nilLike(handler) || clock == nil {
		return nil, errors.New("sandbox API service requires non-nil store, governed handler, and clock")
	}
	return &ServiceV1{store: store, handler: handler, clock: clock}, nil
}

// Submit reserves one create-once operation. A repeated tenant/idempotency key
// returns the same exact fact only when the sealed request is identical.
func (s *ServiceV1) Submit(ctx context.Context, request OperationRequestV1) (OperationFactV1, error) {
	now := s.clock()
	if err := request.ValidateCurrent(now); err != nil {
		return OperationFactV1{}, fmt.Errorf("%w: %v", ErrStale, err)
	}
	fact, err := SealOperationFactV1(OperationFactV1{
		ID:              request.RequestID,
		Revision:        1,
		Request:         request,
		State:           OperationQueuedV1,
		CreatedUnixNano: now.UnixNano(),
		UpdatedUnixNano: now.UnixNano(),
		ExpiresUnixNano: request.RequestedNotAfterUnixNano,
	})
	if err != nil {
		return OperationFactV1{}, err
	}
	stored, created, err := s.store.CreateOnce(ctx, request.TenantID, request.IdempotencyKey, fact)
	if err != nil {
		return OperationFactV1{}, err
	}
	if !created && stored.Request.Digest != request.Digest {
		return OperationFactV1{}, fmt.Errorf("%w: idempotency key already binds another request", ErrConflict)
	}
	return cloneFact(stored)
}

func (s *ServiceV1) Inspect(ctx context.Context, id string) (OperationFactV1, error) {
	fact, err := s.store.InspectCurrent(ctx, id)
	if err != nil {
		return OperationFactV1{}, err
	}
	return cloneFact(fact)
}

func (s *ServiceV1) InspectByIdempotency(ctx context.Context, tenantID, key string) (OperationFactV1, error) {
	fact, err := s.store.InspectByIdempotency(ctx, tenantID, key)
	if err != nil {
		return OperationFactV1{}, err
	}
	return cloneFact(fact)
}

// Execute wins queued->running by exact CAS before calling the governed handler.
// Any lost/unknown handler reply is reconciled by Inspect of the original attempt;
// Execute is never retried.
func (s *ServiceV1) Execute(ctx context.Context, id string) (OperationFactV1, error) {
	current, err := s.store.InspectCurrent(ctx, id)
	if err != nil {
		return OperationFactV1{}, err
	}
	if current.State != OperationQueuedV1 {
		return OperationFactV1{}, fmt.Errorf("%w: operation is %s", ErrConflict, current.State)
	}
	now := s.clock()
	if err := current.Request.ValidateCurrent(now); err != nil {
		return OperationFactV1{}, fmt.Errorf("%w: %v", ErrStale, err)
	}
	running, err := s.transition(current, now, OperationRunningV1, current.CancellationRequested, nil, nil)
	if err != nil {
		return OperationFactV1{}, err
	}
	running, err = s.store.CompareAndSwap(ctx, current.Ref(), running)
	if err != nil {
		return OperationFactV1{}, err
	}

	outcome, executeErr := s.handler.Execute(ctx, cloneRequest(running.Request))
	if executeErr != nil {
		outcome, err = s.handler.Reconcile(ctx, cloneRequest(running.Request))
		if err != nil {
			outcome = indeterminate("governed_handler_unknown", "governed handler outcome remains unknown after Inspect")
		}
	}
	if err := outcome.Validate(); err != nil {
		outcome = indeterminate("invalid_governed_outcome", err.Error())
	}
	return s.finish(ctx, running.ID, outcome)
}

// Reconcile only Inspects a running original attempt. It never calls Execute.
func (s *ServiceV1) Reconcile(ctx context.Context, id string) (OperationFactV1, error) {
	current, err := s.store.InspectCurrent(ctx, id)
	if err != nil {
		return OperationFactV1{}, err
	}
	if current.State != OperationRunningV1 {
		return OperationFactV1{}, fmt.Errorf("%w: only a running operation can be reconciled", ErrConflict)
	}
	outcome, err := s.handler.Reconcile(ctx, cloneRequest(current.Request))
	if err != nil {
		outcome = indeterminate("governed_handler_unknown", "governed handler outcome remains unknown after Inspect")
	}
	if err := outcome.Validate(); err != nil {
		outcome = indeterminate("invalid_governed_outcome", err.Error())
	}
	return s.finish(ctx, id, outcome)
}

// Cancel only prevents a queued API operation from starting. For a running
// operation it records intent; it neither claims nor performs Sandbox cancel.
func (s *ServiceV1) Cancel(ctx context.Context, id string) (OperationFactV1, error) {
	for {
		current, err := s.store.InspectCurrent(ctx, id)
		if err != nil {
			return OperationFactV1{}, err
		}
		if current.State.Terminal() || current.CancellationRequested {
			return cloneFact(current)
		}
		now := s.clock()
		if current.ExpiresUnixNano <= now.UnixNano() {
			return OperationFactV1{}, fmt.Errorf("%w: operation expired", ErrStale)
		}
		state := current.State
		var closed *ClosedErrorV1
		if state == OperationQueuedV1 {
			state = OperationCancelledV1
			closed = &ClosedErrorV1{Category: "cancelled", Reason: "cancelled_before_execution", Message: "operation was cancelled before governed execution"}
		}
		next, err := s.transition(current, now, state, true, nil, closed)
		if err != nil {
			return OperationFactV1{}, err
		}
		updated, err := s.store.CompareAndSwap(ctx, current.Ref(), next)
		if errors.Is(err, ErrConflict) {
			continue
		}
		if err != nil {
			return OperationFactV1{}, err
		}
		return cloneFact(updated)
	}
}

func (s *ServiceV1) Watch(ctx context.Context, after uint64, limit int) ([]OperationFactV1, uint64, error) {
	items, cursor, err := s.store.ListAfter(ctx, after, limit)
	if err != nil {
		return nil, 0, err
	}
	cloned := make([]OperationFactV1, len(items))
	for index := range items {
		cloned[index], err = cloneFact(items[index])
		if err != nil {
			return nil, 0, err
		}
	}
	return cloned, cursor, nil
}

func (s *ServiceV1) finish(ctx context.Context, id string, outcome HandlerOutcomeV1) (OperationFactV1, error) {
	for {
		current, err := s.store.InspectCurrent(ctx, id)
		if err != nil {
			return OperationFactV1{}, err
		}
		if current.State.Terminal() {
			return cloneFact(current)
		}
		if current.State != OperationRunningV1 {
			return OperationFactV1{}, fmt.Errorf("%w: operation left running state", ErrConflict)
		}
		next, err := s.transition(current, s.clock(), outcome.State, current.CancellationRequested, outcome.Result, outcome.Error)
		if err != nil {
			return OperationFactV1{}, err
		}
		updated, err := s.store.CompareAndSwap(ctx, current.Ref(), next)
		if errors.Is(err, ErrConflict) {
			continue
		}
		if err != nil {
			return OperationFactV1{}, err
		}
		return cloneFact(updated)
	}
}

func (s *ServiceV1) transition(current OperationFactV1, now time.Time, state OperationStateV1, cancellationRequested bool, result *ResultV1, closed *ClosedErrorV1) (OperationFactV1, error) {
	next := current
	next.Revision++
	next.State = state
	next.CancellationRequested = cancellationRequested
	next.Result = cloneResult(result)
	next.Error = cloneError(closed)
	next.UpdatedUnixNano = now.UnixNano()
	return SealOperationFactV1(next)
}

func indeterminate(reason, message string) HandlerOutcomeV1 {
	return HandlerOutcomeV1{State: OperationIndeterminateV1, Error: &ClosedErrorV1{Category: "unknown_outcome", Reason: reason, Message: message}}
}

func cloneFact(value OperationFactV1) (OperationFactV1, error) {
	request := cloneRequest(value.Request)
	value.Request = request
	value.Result = cloneResult(value.Result)
	value.Error = cloneError(value.Error)
	return value, value.ValidateShape()
}

func cloneRequest(value OperationRequestV1) OperationRequestV1 {
	value.Payload = append([]byte(nil), value.Payload...)
	return value
}

func cloneResult(value *ResultV1) *ResultV1 {
	if value == nil {
		return nil
	}
	copy := *value
	copy.Payload = append([]byte(nil), value.Payload...)
	return &copy
}

func cloneError(value *ClosedErrorV1) *ClosedErrorV1 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func nilLike(value any) bool {
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

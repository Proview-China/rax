package api

import (
	"context"
	"errors"
)

var (
	ErrNotFound = errors.New("sandbox API operation not found")
	ErrConflict = errors.New("sandbox API operation conflict")
	ErrStale    = errors.New("sandbox API operation is stale")
)

// OperationRefV1 is the exact CAS coordinate for an API operation fact.
type OperationRefV1 struct {
	ID       string `json:"id"`
	Revision uint64 `json:"revision"`
	Digest   string `json:"digest"`
}

func (r OperationRefV1) Validate() error {
	if r.ID == "" || r.Revision == 0 || r.Digest == "" {
		return errors.New("sandbox API operation ref is incomplete")
	}
	return nil
}

func (f OperationFactV1) Ref() OperationRefV1 {
	return OperationRefV1{ID: f.ID, Revision: f.Revision, Digest: f.Digest}
}

// OperationStoreV1 owns append-only operation history and one monotonic current
// pointer. CreateOnce and CompareAndSwap must be atomic. Implementations must
// deep-copy mutable payloads on both read and write.
type OperationStoreV1 interface {
	CreateOnce(context.Context, string, string, OperationFactV1) (OperationFactV1, bool, error)
	InspectCurrent(context.Context, string) (OperationFactV1, error)
	InspectByIdempotency(context.Context, string, string) (OperationFactV1, error)
	CompareAndSwap(context.Context, OperationRefV1, OperationFactV1) (OperationFactV1, error)
	ListAfter(context.Context, uint64, int) ([]OperationFactV1, uint64, error)
}

// HandlerOutcomeV1 is the governed Application result. Only confirmed success,
// confirmed failure, or final indeterminate are accepted. It is never a raw
// Provider observation.
type HandlerOutcomeV1 struct {
	State  OperationStateV1
	Result *ResultV1
	Error  *ClosedErrorV1
}

func (o HandlerOutcomeV1) Validate() error {
	switch o.State {
	case OperationSucceededV1:
		if o.Result == nil || o.Result.Validate() != nil || o.Error != nil {
			return errors.New("successful sandbox API handler outcome is invalid")
		}
	case OperationFailedV1, OperationIndeterminateV1:
		if o.Result != nil || o.Error == nil || o.Error.Validate() != nil {
			return errors.New("failed sandbox API handler outcome is invalid")
		}
	default:
		return errors.New("sandbox API handler outcome is not terminal")
	}
	return nil
}

// GovernedHandlerV1 is an Application/Runtime-governed execution boundary.
// Execute may be called at most once after the API operation wins queued->running.
// Reconcile must only Inspect the original attempt and must never replay Execute.
type GovernedHandlerV1 interface {
	Execute(context.Context, OperationRequestV1) (HandlerOutcomeV1, error)
	Reconcile(context.Context, OperationRequestV1) (HandlerOutcomeV1, error)
}

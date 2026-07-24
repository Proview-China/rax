package applicationadapter

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// ApplicationResultStoreV2 is the Tool-owned create-once result boundary used
// by the Application V2 adapter. It has no Provider, Evidence or Settlement
// write capability.
type ApplicationResultStoreV2 interface {
	CreateSingleCallApplicationResultV2(context.Context, applicationcontract.SingleCallToolActionRequestV2, applicationcontract.SingleCallToolActionResultV2) (applicationcontract.SingleCallToolActionResultV2, error)
	InspectSingleCallApplicationResultRecordV2(context.Context, applicationcontract.SingleCallToolActionInspectKeyV2) (ApplicationResultRecordV2, error)
}

// ApplicationResultRecordV2 retains the immutable request needed to validate
// an Inspect result without guessing a request body from its digest-only key.
// The record is Tool-owned reference-store data, not a new Application fact.
type ApplicationResultRecordV2 struct {
	Request applicationcontract.SingleCallToolActionRequestV2 `json:"request"`
	Result  applicationcontract.SingleCallToolActionResultV2  `json:"result"`
}

func (r ApplicationResultRecordV2) ValidateForKey(key applicationcontract.SingleCallToolActionInspectKeyV2) error {
	if err := key.Validate(); err != nil {
		return err
	}
	if err := r.Request.Validate(); err != nil {
		return err
	}
	expected, err := applicationcontract.SealSingleCallToolActionInspectKeyV2(r.Request)
	if err != nil {
		return err
	}
	if expected != key {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Application V2 result record belongs to another request")
	}
	checked := time.Unix(0, r.Result.Coordinate.AssociationCheckedUnixNano)
	if err = r.Result.ValidateCurrentFor(r.Request, checked); err != nil {
		return err
	}
	return nil
}

// InMemoryApplicationResultStoreV2 is a reference/test store, not a production
// backend, durability claim, composition root or SLA.
type InMemoryApplicationResultStoreV2 struct {
	mu     sync.RWMutex
	values map[string]ApplicationResultRecordV2
}

func NewInMemoryApplicationResultStoreV2() *InMemoryApplicationResultStoreV2 {
	return &InMemoryApplicationResultStoreV2{values: make(map[string]ApplicationResultRecordV2)}
}

func (s *InMemoryApplicationResultStoreV2) CreateSingleCallApplicationResultV2(ctx context.Context, request applicationcontract.SingleCallToolActionRequestV2, result applicationcontract.SingleCallToolActionResultV2) (applicationcontract.SingleCallToolActionResultV2, error) {
	if s == nil || isNilFlowDependencyV1(ctx) || request.Validate() != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Application V2 result create input is invalid")
	}
	if err := ctx.Err(); err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	now := time.Unix(0, result.Coordinate.AssociationCheckedUnixNano)
	if err := result.ValidateCurrentFor(request, now); err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	key, err := applicationcontract.SealSingleCallToolActionInspectKeyV2(request)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	value, err := cloneApplicationResultRecordV2(ApplicationResultRecordV2{Request: request, Result: result})
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	mapKey := applicationResultKeyV2(key)
	if existing, ok := s.values[mapKey]; ok {
		if existing.Request.Digest != value.Request.Digest || existing.Result.Digest != value.Result.Digest {
			return applicationcontract.SingleCallToolActionResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Application V2 result key binds different content")
		}
		clone, cloneErr := cloneApplicationResultRecordV2(existing)
		return clone.Result, cloneErr
	}
	s.values[mapKey] = value
	clone, cloneErr := cloneApplicationResultRecordV2(value)
	return clone.Result, cloneErr
}

func (s *InMemoryApplicationResultStoreV2) InspectSingleCallApplicationResultRecordV2(ctx context.Context, key applicationcontract.SingleCallToolActionInspectKeyV2) (ApplicationResultRecordV2, error) {
	if s == nil || isNilFlowDependencyV1(ctx) {
		return ApplicationResultRecordV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Application V2 result store is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return ApplicationResultRecordV2{}, err
	}
	if err := key.Validate(); err != nil {
		return ApplicationResultRecordV2{}, err
	}
	s.mu.RLock()
	value, ok := s.values[applicationResultKeyV2(key)]
	s.mu.RUnlock()
	if !ok {
		return ApplicationResultRecordV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Application V2 result not found")
	}
	return cloneApplicationResultRecordV2(value)
}

func applicationResultKeyV2(key applicationcontract.SingleCallToolActionInspectKeyV2) string {
	return key.RequestID + "\x00" + string(key.RequestDigest) + "\x00" + string(key.ActionCoordinateDigest) + "\x00" + string(key.ScopeDigest)
}

func cloneApplicationResultV2(value applicationcontract.SingleCallToolActionResultV2) (applicationcontract.SingleCallToolActionResultV2, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	var clone applicationcontract.SingleCallToolActionResultV2
	if err = json.Unmarshal(payload, &clone); err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	return clone, nil
}

func cloneApplicationResultRecordV2(value ApplicationResultRecordV2) (ApplicationResultRecordV2, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return ApplicationResultRecordV2{}, err
	}
	var clone ApplicationResultRecordV2
	if err = json.Unmarshal(payload, &clone); err != nil {
		return ApplicationResultRecordV2{}, err
	}
	return clone, nil
}

var _ ApplicationResultStoreV2 = (*InMemoryApplicationResultStoreV2)(nil)

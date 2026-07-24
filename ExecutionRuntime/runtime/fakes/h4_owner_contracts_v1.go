package fakes

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// H4OwnerCurrentStoreV1 is a thread-safe reference-test store for the neutral
// H4 current-reader contracts. It is not durable and makes no production claim.
type H4OwnerCurrentStoreV1 struct {
	mu           sync.RWMutex
	availability map[string]ports.AgentExecutionAvailabilityProjectionV1
	handles      map[string]ports.ResourceHandleCurrentV1
	sets         map[string]ports.ResourceBindingSetV1
}

func NewH4OwnerCurrentStoreV1() *H4OwnerCurrentStoreV1 {
	return &H4OwnerCurrentStoreV1{
		availability: make(map[string]ports.AgentExecutionAvailabilityProjectionV1),
		handles:      make(map[string]ports.ResourceHandleCurrentV1),
		sets:         make(map[string]ports.ResourceBindingSetV1),
	}
}

func (s *H4OwnerCurrentStoreV1) EnsureAgentExecutionAvailabilityV1(ctx context.Context, value ports.AgentExecutionAvailabilityProjectionV1) (ports.AgentExecutionAvailabilityProjectionV1, error) {
	if err := h4ContextV1(ctx); err != nil {
		return ports.AgentExecutionAvailabilityProjectionV1{}, err
	}
	if s == nil {
		return ports.AgentExecutionAvailabilityProjectionV1{}, unavailableH4V1("Agent execution availability store")
	}
	if err := value.Validate(); err != nil {
		return ports.AgentExecutionAvailabilityProjectionV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.availability[value.Ref.ID]
	if exists {
		if current.Ref == value.Ref {
			return current, nil
		}
		if err := ports.ValidateAgentExecutionAvailabilityTransitionV1(current, value); err != nil {
			return ports.AgentExecutionAvailabilityProjectionV1{}, err
		}
	}
	s.availability[value.Ref.ID] = value
	return value, nil
}

func (s *H4OwnerCurrentStoreV1) InspectAgentExecutionAvailabilityCurrentV1(ctx context.Context, exact ports.AgentExecutionAvailabilityRefV1) (ports.AgentExecutionAvailabilityProjectionV1, error) {
	if err := h4ContextV1(ctx); err != nil {
		return ports.AgentExecutionAvailabilityProjectionV1{}, err
	}
	if s == nil || exact.Validate() != nil {
		return ports.AgentExecutionAvailabilityProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent execution availability exact lookup is invalid")
	}
	s.mu.RLock()
	value, exists := s.availability[exact.ID]
	s.mu.RUnlock()
	if !exists {
		return ports.AgentExecutionAvailabilityProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Agent execution availability is absent")
	}
	if value.Ref != exact {
		return ports.AgentExecutionAvailabilityProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent execution availability exact Ref is not current")
	}
	return value, nil
}

func (s *H4OwnerCurrentStoreV1) EnsureResourceHandleCurrentV1(ctx context.Context, value ports.ResourceHandleCurrentV1) (ports.ResourceHandleCurrentV1, error) {
	if err := h4ContextV1(ctx); err != nil {
		return ports.ResourceHandleCurrentV1{}, err
	}
	if s == nil {
		return ports.ResourceHandleCurrentV1{}, unavailableH4V1("Resource current store")
	}
	if err := value.Validate(); err != nil {
		return ports.ResourceHandleCurrentV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := value.Ref.Owner.Domain + "/" + string(value.Ref.Owner.ID) + "/" + value.Ref.ID
	if current, exists := s.handles[key]; exists {
		if current.Ref != value.Ref {
			return ports.ResourceHandleCurrentV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Resource handle current already exists")
		}
		return current, nil
	}
	s.handles[key] = value
	return value, nil
}

func (s *H4OwnerCurrentStoreV1) InspectResourceHandleCurrentV1(ctx context.Context, exact ports.ResourceHandleRefV1) (ports.ResourceHandleCurrentV1, error) {
	if err := h4ContextV1(ctx); err != nil {
		return ports.ResourceHandleCurrentV1{}, err
	}
	if s == nil || exact.Validate() != nil {
		return ports.ResourceHandleCurrentV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Resource handle current exact lookup is invalid")
	}
	key := exact.Owner.Domain + "/" + string(exact.Owner.ID) + "/" + exact.ID
	s.mu.RLock()
	value, exists := s.handles[key]
	s.mu.RUnlock()
	if !exists {
		return ports.ResourceHandleCurrentV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Resource handle current is absent")
	}
	if value.Ref != exact {
		return ports.ResourceHandleCurrentV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Resource handle current exact Ref drifted")
	}
	return value, nil
}

func (s *H4OwnerCurrentStoreV1) EnsureResourceBindingSetCurrentV1(ctx context.Context, value ports.ResourceBindingSetV1) (ports.ResourceBindingSetV1, error) {
	if err := h4ContextV1(ctx); err != nil {
		return ports.ResourceBindingSetV1{}, err
	}
	if s == nil {
		return ports.ResourceBindingSetV1{}, unavailableH4V1("Resource BindingSet current store")
	}
	if err := value.Validate(); err != nil {
		return ports.ResourceBindingSetV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, exists := s.sets[value.Ref.ID]; exists {
		if current.Ref != value.Ref {
			return ports.ResourceBindingSetV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Resource BindingSet current already exists")
		}
		return cloneResourceBindingSetV1(current), nil
	}
	s.sets[value.Ref.ID] = cloneResourceBindingSetV1(value)
	return cloneResourceBindingSetV1(value), nil
}

func (s *H4OwnerCurrentStoreV1) InspectResourceBindingSetCurrentV1(ctx context.Context, exact ports.ResourceBindingSetRefV1) (ports.ResourceBindingSetV1, error) {
	if err := h4ContextV1(ctx); err != nil {
		return ports.ResourceBindingSetV1{}, err
	}
	if s == nil || exact.Validate() != nil {
		return ports.ResourceBindingSetV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Resource BindingSet exact lookup is invalid")
	}
	s.mu.RLock()
	value, exists := s.sets[exact.ID]
	s.mu.RUnlock()
	if !exists {
		return ports.ResourceBindingSetV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Resource BindingSet current is absent")
	}
	if value.Ref != exact {
		return ports.ResourceBindingSetV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Resource BindingSet exact Ref drifted")
	}
	return cloneResourceBindingSetV1(value), nil
}

// BindingAdmissionStoreV1 is a reference-only create-once admission fake. It
// linearizes one result per AttemptID without constructing real Binding facts.
type BindingAdmissionStoreV1 struct {
	mu        sync.RWMutex
	clock     func() time.Time
	results   map[string]ports.BindingAdmissionResultV1
	commits   uint64
	loseReply bool
}

func NewBindingAdmissionStoreV1(clock func() time.Time) *BindingAdmissionStoreV1 {
	return &BindingAdmissionStoreV1{clock: clock, results: make(map[string]ports.BindingAdmissionResultV1)}
}

func (s *BindingAdmissionStoreV1) InjectLostStartReplyV1() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.loseReply = true
	s.mu.Unlock()
}

func (s *BindingAdmissionStoreV1) CommitCountV1() uint64 {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.commits
}

func (s *BindingAdmissionStoreV1) StartOrInspectBindingAdmissionV1(ctx context.Context, request ports.BindingAdmissionRequestV1) (ports.BindingAdmissionResultV1, error) {
	if err := h4ContextV1(ctx); err != nil {
		return ports.BindingAdmissionResultV1{}, err
	}
	if s == nil || s.clock == nil {
		return ports.BindingAdmissionResultV1{}, unavailableH4V1("Binding admission store")
	}
	now := s.clock()
	if err := request.ValidateCurrent(now); err != nil {
		return ports.BindingAdmissionResultV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.results[request.AttemptID]; ok {
		if existing.RequestDigest != request.RequestDigest {
			return ports.BindingAdmissionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Binding admission Attempt already has different content")
		}
		return cloneBindingAdmissionResultV1(existing), nil
	}
	result, err := buildReferenceBindingAdmissionResultV1(request, now)
	if err != nil {
		return ports.BindingAdmissionResultV1{}, err
	}
	s.results[request.AttemptID] = cloneBindingAdmissionResultV1(result)
	s.commits++
	if s.loseReply {
		s.loseReply = false
		return ports.BindingAdmissionResultV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "injected lost Binding admission reply")
	}
	return cloneBindingAdmissionResultV1(result), nil
}

func (s *BindingAdmissionStoreV1) InspectBindingAdmissionV1(ctx context.Context, request ports.BindingAdmissionInspectRequestV1) (ports.BindingAdmissionResultV1, error) {
	if err := h4ContextV1(ctx); err != nil {
		return ports.BindingAdmissionResultV1{}, err
	}
	if s == nil || request.Validate() != nil {
		return ports.BindingAdmissionResultV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Binding admission inspect request is invalid")
	}
	s.mu.RLock()
	result, exists := s.results[request.AttemptID]
	s.mu.RUnlock()
	if !exists {
		return ports.BindingAdmissionResultV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Binding admission result is absent")
	}
	if result.RequestDigest != request.RequestDigest {
		return ports.BindingAdmissionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Binding admission inspect digest drifted")
	}
	return cloneBindingAdmissionResultV1(result), nil
}

func buildReferenceBindingAdmissionResultV1(request ports.BindingAdmissionRequestV1, now time.Time) (ports.BindingAdmissionResultV1, error) {
	bindings := make([]ports.BindingAdmissionBindingRefV1, 0, len(request.Releases))
	for index, release := range request.Releases {
		id := fmt.Sprintf("binding-%03d-%s", index, string(request.RequestDigest)[7:31])
		digest, err := core.CanonicalJSONDigest("praxis.runtime.binding-admission-fake", ports.BindingAdmissionContractVersionV1, "ReferenceBindingV1", struct {
			AttemptID     string              `json:"attempt_id"`
			RequestDigest core.Digest         `json:"request_digest"`
			ComponentID   ports.ComponentIDV2 `json:"component_id"`
		}{request.AttemptID, request.RequestDigest, release.ComponentID})
		if err != nil {
			return ports.BindingAdmissionResultV1{}, err
		}
		bindings = append(bindings, ports.BindingAdmissionBindingRefV1{ComponentID: release.ComponentID, ID: id, Revision: 1, Digest: digest, ExpiresUnixNano: request.RequestedNotAfterUnixNano})
	}
	setDigest, err := core.CanonicalJSONDigest("praxis.runtime.binding-admission-fake", ports.BindingAdmissionContractVersionV1, "ReferenceBindingSetV1", struct {
		ID            string                               `json:"id"`
		RequestDigest core.Digest                          `json:"request_digest"`
		Bindings      []ports.BindingAdmissionBindingRefV1 `json:"bindings"`
	}{request.ExpectedBindingSetID, request.RequestDigest, bindings})
	if err != nil {
		return ports.BindingAdmissionResultV1{}, err
	}
	return ports.SealBindingAdmissionResultV1(ports.BindingAdmissionResultV1{
		AttemptID: request.AttemptID, RequestDigest: request.RequestDigest,
		BindingSet: ports.BindingAdmissionBindingSetRefV1{ID: request.ExpectedBindingSetID, Revision: 1, Digest: setDigest, ExpiresUnixNano: request.RequestedNotAfterUnixNano},
		Bindings:   bindings, ResourceBindingSet: request.ResourceBindingSet,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: request.RequestedNotAfterUnixNano,
	})
}

func cloneBindingAdmissionResultV1(value ports.BindingAdmissionResultV1) ports.BindingAdmissionResultV1 {
	value.Bindings = append([]ports.BindingAdmissionBindingRefV1{}, value.Bindings...)
	return value
}

func cloneResourceBindingSetV1(value ports.ResourceBindingSetV1) ports.ResourceBindingSetV1 {
	value.Bindings = append([]ports.ResourceBindingV1{}, value.Bindings...)
	return value
}

func h4ContextV1(ctx context.Context) error {
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "H4 reference store context is nil")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func unavailableH4V1(subject string) error {
	return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, subject+" is unavailable")
}

var _ ports.AgentExecutionAvailabilityCurrentReaderV1 = (*H4OwnerCurrentStoreV1)(nil)
var _ ports.ResourceCurrentReaderV1 = (*H4OwnerCurrentStoreV1)(nil)
var _ ports.ResourceOwnerRepositoryV1 = (*H4OwnerCurrentStoreV1)(nil)
var _ ports.BindingAdmissionGovernancePortV1 = (*BindingAdmissionStoreV1)(nil)

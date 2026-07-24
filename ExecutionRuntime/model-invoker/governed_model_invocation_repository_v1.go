package modelinvoker

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type GovernedModelInvocationMutationV1 struct {
	Fact    GovernedModelInvocationFactV1 `json:"fact"`
	Applied bool                          `json:"applied"`
}

type GovernedModelInvocationCASV1 struct {
	Expected GovernedModelInvocationRefV1  `json:"expected"`
	Next     GovernedModelInvocationFactV1 `json:"next"`
}

func (r GovernedModelInvocationCASV1) Validate() error {
	if err := r.Expected.Validate(); err != nil {
		return governedErrorV1(GovernedModelInvocationErrorInvalid, "cas", "expected exact Ref is invalid", err)
	}
	if err := r.Next.Validate(); err != nil {
		return governedErrorV1(GovernedModelInvocationErrorInvalid, "cas", "next Fact is invalid", err)
	}
	next := r.Next.RefV1()
	if r.Expected.ID != next.ID || r.Expected.PreparedRef != next.PreparedRef || r.Expected.AttemptRequestDigest != next.AttemptRequestDigest || r.Expected.RouteCallDigest != next.RouteCallDigest || r.Expected.DispatchSequence != next.DispatchSequence || r.Expected.ProviderAttemptOrdinal != next.ProviderAttemptOrdinal || next.Revision != r.Expected.Revision+1 {
		return governedErrorV1(GovernedModelInvocationErrorConflict, "cas", "CAS expected and next coordinates are not adjacent exact revisions", nil)
	}
	return nil
}

// GovernedModelInvocationRepositoryV1 owns append-only history plus one
// highest-revision current index. Applied is true only for the caller that
// linearized a mutation; idempotent replays never regain provider call rights.
type GovernedModelInvocationRepositoryV1 interface {
	CreateGovernedModelInvocationV1(context.Context, GovernedModelInvocationFactV1) (GovernedModelInvocationMutationV1, error)
	CompareAndSwapGovernedModelInvocationV1(context.Context, GovernedModelInvocationCASV1) (GovernedModelInvocationMutationV1, error)
	InspectExactGovernedModelInvocationV1(context.Context, GovernedModelInvocationRefV1) (GovernedModelInvocationFactV1, error)
	InspectCurrentGovernedModelInvocationV1(context.Context, string) (GovernedModelInvocationFactV1, error)
}

type storedGovernedModelInvocationV1 struct {
	ref  GovernedModelInvocationRefV1
	wire json.RawMessage
}

// governedModelInvocationAttemptKeyV1 is the stable logical provider-attempt
// coordinate. Route/request changes under the same Prepared invocation,
// dispatch sequence and provider ordinal are conflicts, not a second provider
// opportunity with a different derived invocation ID.
type governedModelInvocationAttemptKeyV1 struct {
	PreparedRef          PreparedModelInvocationRefV1
	DispatchSequence     uint64
	ProviderAttemptOrder uint32
}

// InMemoryGovernedModelInvocationStoreV1 is a thread-safe reference/test
// implementation. It is not a production backend, root, retention service or
// SLA claim.
type InMemoryGovernedModelInvocationStoreV1 struct {
	mu      sync.RWMutex
	history map[string]map[core.Revision]storedGovernedModelInvocationV1
	current map[string]GovernedModelInvocationRefV1
	attempt map[governedModelInvocationAttemptKeyV1]string
}

func NewInMemoryGovernedModelInvocationStoreV1() *InMemoryGovernedModelInvocationStoreV1 {
	return &InMemoryGovernedModelInvocationStoreV1{
		history: make(map[string]map[core.Revision]storedGovernedModelInvocationV1),
		current: make(map[string]GovernedModelInvocationRefV1),
		attempt: make(map[governedModelInvocationAttemptKeyV1]string),
	}
}

func (s *InMemoryGovernedModelInvocationStoreV1) CreateGovernedModelInvocationV1(ctx context.Context, fact GovernedModelInvocationFactV1) (GovernedModelInvocationMutationV1, error) {
	if err := governedRepositoryContextV1(ctx, "create"); err != nil {
		return GovernedModelInvocationMutationV1{}, err
	}
	if s == nil {
		return GovernedModelInvocationMutationV1{}, governedErrorV1(GovernedModelInvocationErrorUnavailable, "create", "repository is unavailable", nil)
	}
	if err := fact.Validate(); err != nil || fact.Revision != 1 || fact.State != GovernedModelInvocationPreparedV1 {
		return GovernedModelInvocationMutationV1{}, governedErrorV1(GovernedModelInvocationErrorInvalid, "create", "create requires sealed prepared revision one", err)
	}
	wire, err := encodeGovernedModelInvocationFactV1(fact)
	if err != nil {
		return GovernedModelInvocationMutationV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := governedRepositoryContextV1(ctx, "create"); err != nil {
		return GovernedModelInvocationMutationV1{}, err
	}
	s.initLockedV1()
	attemptKey := governedModelInvocationAttemptKeyV1{
		PreparedRef:          fact.PreparedRef,
		DispatchSequence:     fact.DispatchSequence,
		ProviderAttemptOrder: fact.ProviderAttemptOrdinal,
	}
	existingAttemptID, attemptExists := s.attempt[attemptKey]
	if attemptExists && existingAttemptID != fact.ID {
		return GovernedModelInvocationMutationV1{}, governedErrorV1(GovernedModelInvocationErrorConflict, "create", "logical provider attempt contains different canonical content", nil)
	}
	if current, exists := s.current[fact.ID]; exists {
		if !attemptExists || existingAttemptID != fact.ID {
			return GovernedModelInvocationMutationV1{}, governedErrorV1(GovernedModelInvocationErrorConflict, "create", "invocation current index lost its logical attempt guard", nil)
		}
		stored, ok := s.history[fact.ID][current.Revision]
		if !ok {
			return GovernedModelInvocationMutationV1{}, governedErrorV1(GovernedModelInvocationErrorConflict, "create", "current index has no history", nil)
		}
		first, ok := s.history[fact.ID][1]
		if !ok || first.ref != fact.RefV1() || !bytes.Equal(first.wire, wire) {
			return GovernedModelInvocationMutationV1{}, governedErrorV1(GovernedModelInvocationErrorConflict, "create", "invocation ID contains different canonical content", nil)
		}
		currentFact, err := decodeGovernedStoredV1(stored, current)
		return GovernedModelInvocationMutationV1{Fact: currentFact, Applied: false}, err
	}
	s.history[fact.ID] = map[core.Revision]storedGovernedModelInvocationV1{1: {ref: fact.RefV1(), wire: append(json.RawMessage(nil), wire...)}}
	s.current[fact.ID] = fact.RefV1()
	s.attempt[attemptKey] = fact.ID
	return GovernedModelInvocationMutationV1{Fact: fact.CloneV1(), Applied: true}, nil
}

func (s *InMemoryGovernedModelInvocationStoreV1) CompareAndSwapGovernedModelInvocationV1(ctx context.Context, request GovernedModelInvocationCASV1) (GovernedModelInvocationMutationV1, error) {
	if err := governedRepositoryContextV1(ctx, "cas"); err != nil {
		return GovernedModelInvocationMutationV1{}, err
	}
	if s == nil {
		return GovernedModelInvocationMutationV1{}, governedErrorV1(GovernedModelInvocationErrorUnavailable, "cas", "repository is unavailable", nil)
	}
	if err := request.Validate(); err != nil {
		return GovernedModelInvocationMutationV1{}, err
	}
	wire, err := encodeGovernedModelInvocationFactV1(request.Next)
	if err != nil {
		return GovernedModelInvocationMutationV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := governedRepositoryContextV1(ctx, "cas"); err != nil {
		return GovernedModelInvocationMutationV1{}, err
	}
	s.initLockedV1()
	currentRef, ok := s.current[request.Next.ID]
	if !ok {
		return GovernedModelInvocationMutationV1{}, governedErrorV1(GovernedModelInvocationErrorNotFound, "cas", "invocation is absent", nil)
	}
	currentStored, ok := s.history[request.Next.ID][currentRef.Revision]
	if !ok {
		return GovernedModelInvocationMutationV1{}, governedErrorV1(GovernedModelInvocationErrorConflict, "cas", "current index has no history", nil)
	}
	current, err := decodeGovernedStoredV1(currentStored, currentRef)
	if err != nil {
		return GovernedModelInvocationMutationV1{}, err
	}
	attemptKey := governedModelInvocationAttemptKeyV1{PreparedRef: current.PreparedRef, DispatchSequence: current.DispatchSequence, ProviderAttemptOrder: current.ProviderAttemptOrdinal}
	if guardedID, exists := s.attempt[attemptKey]; !exists || guardedID != current.ID {
		return GovernedModelInvocationMutationV1{}, governedErrorV1(GovernedModelInvocationErrorConflict, "cas", "invocation current index lost its logical attempt guard", nil)
	}
	if currentRef == request.Next.RefV1() {
		if !bytes.Equal(currentStored.wire, wire) {
			return GovernedModelInvocationMutationV1{}, governedErrorV1(GovernedModelInvocationErrorConflict, "cas", "same Ref contains different canonical content", nil)
		}
		return GovernedModelInvocationMutationV1{Fact: current, Applied: false}, nil
	}
	if currentRef != request.Expected {
		return GovernedModelInvocationMutationV1{}, governedErrorV1(GovernedModelInvocationErrorConflict, "cas", "current exact Ref changed", nil)
	}
	if err := ValidateGovernedModelInvocationTransitionV1(current, request.Next); err != nil {
		return GovernedModelInvocationMutationV1{}, err
	}
	if _, exists := s.history[request.Next.ID][request.Next.Revision]; exists {
		return GovernedModelInvocationMutationV1{}, governedErrorV1(GovernedModelInvocationErrorConflict, "cas", "history revision already exists", nil)
	}
	s.history[request.Next.ID][request.Next.Revision] = storedGovernedModelInvocationV1{ref: request.Next.RefV1(), wire: append(json.RawMessage(nil), wire...)}
	s.current[request.Next.ID] = request.Next.RefV1()
	return GovernedModelInvocationMutationV1{Fact: request.Next.CloneV1(), Applied: true}, nil
}

func (s *InMemoryGovernedModelInvocationStoreV1) InspectExactGovernedModelInvocationV1(ctx context.Context, ref GovernedModelInvocationRefV1) (GovernedModelInvocationFactV1, error) {
	if err := governedRepositoryContextV1(ctx, "inspect_exact"); err != nil {
		return GovernedModelInvocationFactV1{}, err
	}
	if s == nil {
		return GovernedModelInvocationFactV1{}, governedErrorV1(GovernedModelInvocationErrorUnavailable, "inspect_exact", "repository is unavailable", nil)
	}
	if err := ref.Validate(); err != nil {
		return GovernedModelInvocationFactV1{}, governedErrorV1(GovernedModelInvocationErrorInvalid, "inspect_exact", "full exact Ref is invalid", err)
	}
	s.mu.RLock()
	history := s.history[ref.ID]
	stored, ok := history[ref.Revision]
	if ok {
		stored.wire = append(json.RawMessage(nil), stored.wire...)
	}
	s.mu.RUnlock()
	if err := governedRepositoryContextV1(ctx, "inspect_exact"); err != nil {
		return GovernedModelInvocationFactV1{}, err
	}
	if !ok {
		return GovernedModelInvocationFactV1{}, governedErrorV1(GovernedModelInvocationErrorNotFound, "inspect_exact", "exact invocation history is absent", nil)
	}
	if stored.ref != ref {
		return GovernedModelInvocationFactV1{}, governedErrorV1(GovernedModelInvocationErrorConflict, "inspect_exact", "stored exact Ref drifted", nil)
	}
	return decodeGovernedStoredV1(stored, ref)
}

func (s *InMemoryGovernedModelInvocationStoreV1) InspectCurrentGovernedModelInvocationV1(ctx context.Context, id string) (GovernedModelInvocationFactV1, error) {
	if err := governedRepositoryContextV1(ctx, "inspect_current"); err != nil {
		return GovernedModelInvocationFactV1{}, err
	}
	if s == nil {
		return GovernedModelInvocationFactV1{}, governedErrorV1(GovernedModelInvocationErrorUnavailable, "inspect_current", "repository is unavailable", nil)
	}
	if blankGovernedV1(id) {
		return GovernedModelInvocationFactV1{}, governedErrorV1(GovernedModelInvocationErrorInvalid, "inspect_current", "invocation ID is required", nil)
	}
	s.mu.RLock()
	ref, ok := s.current[id]
	stored := s.history[id][ref.Revision]
	attemptKey := governedModelInvocationAttemptKeyV1{PreparedRef: ref.PreparedRef, DispatchSequence: ref.DispatchSequence, ProviderAttemptOrder: ref.ProviderAttemptOrdinal}
	guardedID, guardOK := s.attempt[attemptKey]
	if ok {
		stored.wire = append(json.RawMessage(nil), stored.wire...)
	}
	s.mu.RUnlock()
	if err := governedRepositoryContextV1(ctx, "inspect_current"); err != nil {
		return GovernedModelInvocationFactV1{}, err
	}
	if !ok {
		return GovernedModelInvocationFactV1{}, governedErrorV1(GovernedModelInvocationErrorNotFound, "inspect_current", "invocation current index is absent", nil)
	}
	if !guardOK || guardedID != id {
		return GovernedModelInvocationFactV1{}, governedErrorV1(GovernedModelInvocationErrorConflict, "inspect_current", "invocation current index lost its logical attempt guard", nil)
	}
	return decodeGovernedStoredV1(stored, ref)
}

func (s *InMemoryGovernedModelInvocationStoreV1) initLockedV1() {
	if s.history == nil {
		s.history = make(map[string]map[core.Revision]storedGovernedModelInvocationV1)
	}
	if s.current == nil {
		s.current = make(map[string]GovernedModelInvocationRefV1)
	}
	if s.attempt == nil {
		s.attempt = make(map[governedModelInvocationAttemptKeyV1]string)
	}
}

func encodeGovernedModelInvocationFactV1(fact GovernedModelInvocationFactV1) (json.RawMessage, error) {
	if err := fact.Validate(); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(fact)
	if err != nil {
		return nil, governedErrorV1(GovernedModelInvocationErrorInvalid, "encode", "Fact is not JSON serializable", err)
	}
	var exact GovernedModelInvocationFactV1
	if err := core.DecodeStrictJSON(payload, &exact); err != nil {
		return nil, err
	}
	return payload, nil
}

func decodeGovernedStoredV1(stored storedGovernedModelInvocationV1, expected GovernedModelInvocationRefV1) (GovernedModelInvocationFactV1, error) {
	var fact GovernedModelInvocationFactV1
	if err := core.DecodeStrictJSON(stored.wire, &fact); err != nil {
		return GovernedModelInvocationFactV1{}, governedErrorV1(GovernedModelInvocationErrorConflict, "decode", "stored Fact failed strict decoding", err)
	}
	if err := fact.Validate(); err != nil || fact.RefV1() != expected || stored.ref != expected {
		return GovernedModelInvocationFactV1{}, governedErrorV1(GovernedModelInvocationErrorConflict, "decode", "stored Fact failed exact revalidation", err)
	}
	return fact.CloneV1(), nil
}

func governedRepositoryContextV1(ctx context.Context, operation string) error {
	if ctx == nil {
		return governedErrorV1(GovernedModelInvocationErrorInvalid, operation, "context is nil", nil)
	}
	if err := ctx.Err(); err != nil {
		return governedErrorV1(GovernedModelInvocationErrorIndeterminate, operation, "context ended before linearization", err)
	}
	return nil
}

var _ GovernedModelInvocationRepositoryV1 = (*InMemoryGovernedModelInvocationStoreV1)(nil)

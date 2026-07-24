package modelinvokeradapter

import (
	"context"
	"sync"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	preparedModelInvocationAckRepositoryCanonicalDomainV1  = "praxis.harness.prepared-model-invocation-ack-repository"
	preparedModelInvocationAckRepositoryCanonicalVersionV1 = "v1"
)

// PreparedModelInvocationAckRepositoryV1 is the Harness-owned create-once
// repository used by the concrete Model CommitGate. The internal lookup is
// intentionally absent from Model's public Gate method set.
type PreparedModelInvocationAckRepositoryV1 interface {
	EnsureAck(context.Context, modelinvoker.PreparedModelInvocationCommitAckV1) (modelinvoker.PreparedModelInvocationCommitAckV1, error)
	InspectExactAck(context.Context, modelinvoker.PreparedModelInvocationCommitAckRefV1) (modelinvoker.PreparedModelInvocationCommitAckV1, error)
	inspectByPreparedCurrent(context.Context, modelinvoker.PreparedModelInvocationRefV1, modelinvoker.PreparedModelInvocationCurrentRefV1) (modelinvoker.PreparedModelInvocationCommitAckV1, error)
}

// InMemoryPreparedModelInvocationAckRepositoryV1 is one linearization domain
// for the ACK ID, Prepared+Current recovery key, and Prepared epoch indexes.
// It is a production-capable in-process repository, not a persistence claim.
type InMemoryPreparedModelInvocationAckRepositoryV1 struct {
	mu                sync.RWMutex
	byAckID           map[string]modelinvoker.PreparedModelInvocationCommitAckV1
	byPreparedCurrent map[core.Digest]string
	byPreparedRef     map[core.Digest]core.Digest
}

type preparedModelInvocationAckPreparedCurrentCanonicalV1 struct {
	Prepared modelinvoker.PreparedModelInvocationRefV1        `json:"prepared"`
	Current  modelinvoker.PreparedModelInvocationCurrentRefV1 `json:"current"`
}

func NewInMemoryPreparedModelInvocationAckRepositoryV1() *InMemoryPreparedModelInvocationAckRepositoryV1 {
	return &InMemoryPreparedModelInvocationAckRepositoryV1{
		byAckID:           make(map[string]modelinvoker.PreparedModelInvocationCommitAckV1),
		byPreparedCurrent: make(map[core.Digest]string),
		byPreparedRef:     make(map[core.Digest]core.Digest),
	}
}

func (r *InMemoryPreparedModelInvocationAckRepositoryV1) EnsureAck(ctx context.Context, ack modelinvoker.PreparedModelInvocationCommitAckV1) (modelinvoker.PreparedModelInvocationCommitAckV1, error) {
	if r == nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, preparedAckRepositoryUnavailableV1("Model ACK Repository is unavailable")
	}
	if err := preparedAckRepositoryContextErrorV1(ctx); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	if err := ack.Validate(); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	stableKey, preparedKey, err := preparedAckRepositoryKeysV1(ack.PreparedRef, ack.CurrentRef)
	if err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	ack = ack.Clone()

	r.mu.Lock()
	if err := preparedAckRepositoryContextErrorV1(ctx); err != nil {
		r.mu.Unlock()
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}

	if stored, exists := r.byAckID[ack.ID]; exists {
		if stored != ack || r.byPreparedCurrent[stableKey] != ack.ID || r.byPreparedRef[preparedKey] != stableKey {
			r.mu.Unlock()
			return modelinvoker.PreparedModelInvocationCommitAckV1{}, preparedAckRepositoryConflictV1("Model ACK ID already binds different canonical content")
		}
		r.mu.Unlock()
		if err := preparedAckRepositoryContextErrorV1(ctx); err != nil {
			return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
		}
		return stored.Clone(), nil
	}
	if _, exists := r.byPreparedCurrent[stableKey]; exists {
		r.mu.Unlock()
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, preparedAckRepositoryConflictV1("Prepared+Current recovery key already binds another Model ACK")
	}
	if existingKey, exists := r.byPreparedRef[preparedKey]; exists && existingKey != stableKey {
		r.mu.Unlock()
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, preparedAckRepositoryConflictV1("Prepared epoch already binds another Current/ACK")
	}
	if err := preparedAckRepositoryContextErrorV1(ctx); err != nil {
		r.mu.Unlock()
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	r.byAckID[ack.ID] = ack
	r.byPreparedCurrent[stableKey] = ack.ID
	r.byPreparedRef[preparedKey] = stableKey
	r.mu.Unlock()

	if err := preparedAckRepositoryContextErrorV1(ctx); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	return ack.Clone(), nil
}

func (r *InMemoryPreparedModelInvocationAckRepositoryV1) InspectExactAck(ctx context.Context, ref modelinvoker.PreparedModelInvocationCommitAckRefV1) (modelinvoker.PreparedModelInvocationCommitAckV1, error) {
	if r == nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, preparedAckRepositoryUnavailableV1("Model ACK Repository is unavailable")
	}
	if err := preparedAckRepositoryContextErrorV1(ctx); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}

	r.mu.RLock()
	if err := preparedAckRepositoryContextErrorV1(ctx); err != nil {
		r.mu.RUnlock()
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	stored, exists := r.byAckID[ref.ID]
	r.mu.RUnlock()
	if !exists {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, preparedAckRepositoryAbsentV1("exact Model ACK is absent")
	}
	if stored.Ref() != ref {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, preparedAckRepositoryConflictV1("Model ACK ID exists with another exact Ref")
	}
	if err := preparedAckRepositoryContextErrorV1(ctx); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	return stored.Clone(), nil
}

func (r *InMemoryPreparedModelInvocationAckRepositoryV1) inspectByPreparedCurrent(ctx context.Context, prepared modelinvoker.PreparedModelInvocationRefV1, current modelinvoker.PreparedModelInvocationCurrentRefV1) (modelinvoker.PreparedModelInvocationCommitAckV1, error) {
	if r == nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, preparedAckRepositoryUnavailableV1("Model ACK Repository is unavailable")
	}
	if err := preparedAckRepositoryContextErrorV1(ctx); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	stableKey, preparedKey, err := preparedAckRepositoryKeysV1(prepared, current)
	if err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}

	r.mu.RLock()
	if err := preparedAckRepositoryContextErrorV1(ctx); err != nil {
		r.mu.RUnlock()
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	preparedWinner, preparedExists := r.byPreparedRef[preparedKey]
	ackID, stableExists := r.byPreparedCurrent[stableKey]
	stored, ackExists := r.byAckID[ackID]
	r.mu.RUnlock()
	if preparedExists && preparedWinner != stableKey {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, preparedAckRepositoryConflictV1("Prepared epoch already binds another Current/ACK")
	}
	if !preparedExists && !stableExists && !ackExists {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, preparedAckRepositoryAbsentV1("Prepared+Current Model ACK is authoritatively absent")
	}
	if !preparedExists || !stableExists || !ackExists || stored.ID != ackID || stored.PreparedRef != prepared || stored.CurrentRef != current {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, preparedAckRepositoryConflictV1("Model ACK Repository indexes drifted")
	}
	if err := preparedAckRepositoryContextErrorV1(ctx); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	return stored.Clone(), nil
}

func preparedAckRepositoryKeysV1(prepared modelinvoker.PreparedModelInvocationRefV1, current modelinvoker.PreparedModelInvocationCurrentRefV1) (core.Digest, core.Digest, error) {
	if err := prepared.Validate(); err != nil {
		return "", "", err
	}
	if err := current.Validate(); err != nil {
		return "", "", err
	}
	if current.Prepared != prepared {
		return "", "", preparedAckRepositoryConflictV1("Prepared and Current lineage drifted")
	}
	stableKey, err := core.CanonicalJSONDigest(
		preparedModelInvocationAckRepositoryCanonicalDomainV1,
		preparedModelInvocationAckRepositoryCanonicalVersionV1,
		"PreparedModelInvocationAckPreparedCurrentKeyV1",
		preparedModelInvocationAckPreparedCurrentCanonicalV1{Prepared: prepared, Current: current},
	)
	if err != nil {
		return "", "", err
	}
	preparedKey, err := core.CanonicalJSONDigest(
		preparedModelInvocationAckRepositoryCanonicalDomainV1,
		preparedModelInvocationAckRepositoryCanonicalVersionV1,
		"PreparedModelInvocationAckPreparedKeyV1",
		prepared,
	)
	if err != nil {
		return "", "", err
	}
	return stableKey, preparedKey, nil
}

func preparedAckRepositoryContextErrorV1(ctx context.Context) error {
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Model ACK Repository context is required")
	}
	if ctx.Err() != nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonInvalidState, "Model ACK Repository context is canceled")
	}
	return nil
}

func preparedAckRepositoryUnavailableV1(message string) error {
	return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, message)
}

func preparedAckRepositoryAbsentV1(message string) error {
	return core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, message)
}

func preparedAckRepositoryConflictV1(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, message)
}

var _ PreparedModelInvocationAckRepositoryV1 = (*InMemoryPreparedModelInvocationAckRepositoryV1)(nil)

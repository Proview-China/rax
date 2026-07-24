// Package outcomestore provides a process-local reference implementation for
// immutable Context Outcome/Evaluation/Feedback facts. It is not a production
// State Plane backend, persistence root or SLA.
package outcomestore

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
)

type Memory struct {
	mu          sync.RWMutex
	outcomes    map[string]contract.ContextOutcomeFactV1
	evaluations map[string]contract.ContextEvaluationFactV1
	feedback    map[string]contract.ContextFeedbackCandidateFactV1
}

func NewMemory() *Memory {
	return &Memory{outcomes: make(map[string]contract.ContextOutcomeFactV1), evaluations: make(map[string]contract.ContextEvaluationFactV1), feedback: make(map[string]contract.ContextFeedbackCandidateFactV1)}
}

var _ contextports.ContextOutcomeFactStoreV1 = (*Memory)(nil)

func (m *Memory) PutContextOutcomeV1(ctx context.Context, value contract.ContextOutcomeFactV1) (contract.FactRef, error) {
	return putImmutable(ctx, &m.mu, m.outcomes, value.ID, value, value.Validate, value.DigestValue)
}

func (m *Memory) InspectContextOutcomeV1(ctx context.Context, ref contract.FactRef) (contract.ContextOutcomeFactV1, error) {
	return inspectImmutable(ctx, &m.mu, m.outcomes, ref, func(v contract.ContextOutcomeFactV1) (contract.Digest, error) { return v.DigestValue() })
}

func (m *Memory) PutContextEvaluationV1(ctx context.Context, value contract.ContextEvaluationFactV1) (contract.FactRef, error) {
	return putImmutable(ctx, &m.mu, m.evaluations, value.ID, value, value.Validate, value.DigestValue)
}

func (m *Memory) InspectContextEvaluationV1(ctx context.Context, ref contract.FactRef) (contract.ContextEvaluationFactV1, error) {
	return inspectImmutable(ctx, &m.mu, m.evaluations, ref, func(v contract.ContextEvaluationFactV1) (contract.Digest, error) { return v.DigestValue() })
}

func (m *Memory) PutContextFeedbackCandidateV1(ctx context.Context, value contract.ContextFeedbackCandidateFactV1) (contract.FactRef, error) {
	return putImmutable(ctx, &m.mu, m.feedback, value.ID, value, value.Validate, value.DigestValue)
}

func (m *Memory) InspectContextFeedbackCandidateV1(ctx context.Context, ref contract.FactRef) (contract.ContextFeedbackCandidateFactV1, error) {
	return inspectImmutable(ctx, &m.mu, m.feedback, ref, func(v contract.ContextFeedbackCandidateFactV1) (contract.Digest, error) { return v.DigestValue() })
}

func putImmutable[T any](ctx context.Context, mu *sync.RWMutex, values map[string]T, id string, value T, validate func() error, digest func() (contract.Digest, error)) (contract.FactRef, error) {
	if err := ctx.Err(); err != nil {
		return contract.FactRef{}, err
	}
	if err := validate(); err != nil {
		return contract.FactRef{}, err
	}
	d, err := digest()
	if err != nil {
		return contract.FactRef{}, err
	}
	copy, err := clone(value)
	if err != nil {
		return contract.FactRef{}, err
	}
	ref := contract.FactRef{ID: id, Revision: 1, Digest: d}
	mu.Lock()
	defer mu.Unlock()
	if prior, ok := values[id]; ok {
		priorDigest, digestErr := contract.DigestJSON(prior)
		if digestErr != nil || priorDigest != d {
			return contract.FactRef{}, fmt.Errorf("%w: immutable fact identity collision", contract.ErrConflict)
		}
		return ref, nil
	}
	values[id] = copy
	return ref, nil
}

func inspectImmutable[T any](ctx context.Context, mu *sync.RWMutex, values map[string]T, ref contract.FactRef, digest func(T) (contract.Digest, error)) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if ref.Validate() != nil {
		return zero, fmt.Errorf("%w: immutable fact ref", contract.ErrInvalid)
	}
	mu.RLock()
	value, ok := values[ref.ID]
	mu.RUnlock()
	if !ok {
		return zero, fmt.Errorf("%w: immutable fact", contract.ErrNotFound)
	}
	d, err := digest(value)
	if err != nil {
		return zero, err
	}
	if ref.Revision != 1 || ref.Digest != d {
		return zero, fmt.Errorf("%w: exact immutable fact", contract.ErrConflict)
	}
	return clone(value)
}

func clone[T any](value T) (T, error) {
	var result T
	payload, err := json.Marshal(value)
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		return result, err
	}
	return result, nil
}

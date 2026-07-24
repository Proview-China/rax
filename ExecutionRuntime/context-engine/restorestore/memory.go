package restorestore

import (
	"context"
	"fmt"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
)

type scopedFactKeyV1 struct {
	Scope contract.Digest
	Ref   contract.FactRef
}

type Memory struct {
	mu sync.Mutex

	facts       map[contract.FactRef]contract.RestoreContextMaterializationFactV1
	current     map[contract.RestoreContextTargetBindingV1]contract.FactRef
	generations map[scopedFactKeyV1]contract.RestoredContextGenerationV1
	frames      map[scopedFactKeyV1]contract.RestoredContextFrameV1
	loseNext    bool
	failNext    bool
}

func NewMemory() *Memory {
	return &Memory{facts: map[contract.FactRef]contract.RestoreContextMaterializationFactV1{}, current: map[contract.RestoreContextTargetBindingV1]contract.FactRef{}, generations: map[scopedFactKeyV1]contract.RestoredContextGenerationV1{}, frames: map[scopedFactKeyV1]contract.RestoredContextFrameV1{}}
}

func (m *Memory) LoseNextReplyV1() { m.mu.Lock(); m.loseNext = true; m.mu.Unlock() }
func (m *Memory) FailNextWriteV1() { m.mu.Lock(); m.failNext = true; m.mu.Unlock() }

func (m *Memory) CommitRestoreContextMaterializationV1(ctx context.Context, request contract.RestoreContextMaterializationRequestV1, frames []contract.RestoredContextFrameV1, generation contract.RestoredContextGenerationV1, fact contract.RestoreContextMaterializationFactV1) (contract.RestoreContextMaterializationFactV1, error) {
	if err := contextCheckV1(ctx); err != nil {
		return contract.RestoreContextMaterializationFactV1{}, err
	}
	requestDigest, err := request.DigestValue()
	if err != nil || generation.Validate() != nil || fact.Validate() != nil || fact.RequestDigest != requestDigest || fact.Target != request.Target || fact.TargetGeneration != generation.Ref() || len(frames) != len(fact.TargetFrames) {
		return contract.RestoreContextMaterializationFactV1{}, fmt.Errorf("%w: Restore Context atomic commit", contract.ErrInvalid)
	}
	for index, frame := range frames {
		if frame.Validate() != nil || frame.Target != request.Target || frame.Ref() != fact.TargetFrames[index] {
			return contract.RestoreContextMaterializationFactV1{}, fmt.Errorf("%w: Restore Context Frame commit closure", contract.ErrConflict)
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if ref, ok := m.current[request.Target]; ok {
		existing := m.facts[ref]
		if existing.Ref() == fact.Ref() && existing.RequestDigest == fact.RequestDigest && existing.CurrentDigest == fact.CurrentDigest {
			return cloneFactV1(existing), nil
		}
		return contract.RestoreContextMaterializationFactV1{}, fmt.Errorf("%w: Restore Context target current already changed", contract.ErrConflict)
	}
	if existing, ok := m.facts[fact.Ref()]; ok {
		if existing.RequestDigest == fact.RequestDigest && existing.CurrentDigest == fact.CurrentDigest {
			return cloneFactV1(existing), nil
		}
		return contract.RestoreContextMaterializationFactV1{}, fmt.Errorf("%w: Restore Context materialization ID drift", contract.ErrConflict)
	}
	if m.failNext {
		m.failNext = false
		return contract.RestoreContextMaterializationFactV1{}, fmt.Errorf("%w: injected Restore Context write failure", contract.ErrUnavailable)
	}
	for _, frame := range frames {
		m.frames[scopedFactKeyV1{Scope: request.Target.ScopeDigest, Ref: frame.Ref()}] = cloneFrameV1(frame)
	}
	m.generations[scopedFactKeyV1{Scope: request.Target.ScopeDigest, Ref: generation.Ref()}] = cloneGenerationV1(generation)
	m.facts[fact.Ref()] = cloneFactV1(fact)
	m.current[request.Target] = fact.Ref()
	if m.loseNext {
		m.loseNext = false
		return contract.RestoreContextMaterializationFactV1{}, fmt.Errorf("%w: injected Restore Context reply loss", contract.ErrUnavailable)
	}
	return cloneFactV1(fact), nil
}

func (m *Memory) InspectRestoreContextMaterializationV1(ctx context.Context, ref contract.FactRef) (contract.RestoreContextMaterializationFactV1, error) {
	if err := contextCheckV1(ctx); err != nil {
		return contract.RestoreContextMaterializationFactV1{}, err
	}
	if ref.Validate() != nil {
		return contract.RestoreContextMaterializationFactV1{}, fmt.Errorf("%w: Restore Context exact ref", contract.ErrInvalid)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.facts[ref]
	if !ok {
		return contract.RestoreContextMaterializationFactV1{}, fmt.Errorf("%w: Restore Context materialization", contract.ErrNotFound)
	}
	return cloneFactV1(value), nil
}

func (m *Memory) InspectRestoreContextMaterializationByTargetV1(ctx context.Context, target contract.RestoreContextTargetBindingV1) (contract.RestoreContextMaterializationFactV1, error) {
	if err := contextCheckV1(ctx); err != nil {
		return contract.RestoreContextMaterializationFactV1{}, err
	}
	if target.Validate() != nil {
		return contract.RestoreContextMaterializationFactV1{}, fmt.Errorf("%w: Restore Context target", contract.ErrInvalid)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	ref, ok := m.current[target]
	if !ok {
		return contract.RestoreContextMaterializationFactV1{}, fmt.Errorf("%w: Restore Context target current", contract.ErrNotFound)
	}
	value, ok := m.facts[ref]
	if !ok || value.Target != target {
		return contract.RestoreContextMaterializationFactV1{}, fmt.Errorf("%w: Restore Context current index drift", contract.ErrConflict)
	}
	return cloneFactV1(value), nil
}

func (m *Memory) InspectRestoredContextGenerationV1(ctx context.Context, ref contract.FactRef, scope contract.Digest) (contract.RestoredContextGenerationV1, error) {
	if err := contextCheckV1(ctx); err != nil {
		return contract.RestoredContextGenerationV1{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.generations[scopedFactKeyV1{Scope: scope, Ref: ref}]
	if !ok {
		return contract.RestoredContextGenerationV1{}, fmt.Errorf("%w: restored Context Generation", contract.ErrNotFound)
	}
	return cloneGenerationV1(value), nil
}

func (m *Memory) InspectRestoredContextFrameV1(ctx context.Context, ref contract.FactRef, scope contract.Digest) (contract.RestoredContextFrameV1, error) {
	if err := contextCheckV1(ctx); err != nil {
		return contract.RestoredContextFrameV1{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.frames[scopedFactKeyV1{Scope: scope, Ref: ref}]
	if !ok {
		return contract.RestoredContextFrameV1{}, fmt.Errorf("%w: restored Context Frame", contract.ErrNotFound)
	}
	return cloneFrameV1(value), nil
}

func cloneFactV1(value contract.RestoreContextMaterializationFactV1) contract.RestoreContextMaterializationFactV1 {
	value.TargetFrames = append([]contract.FactRef{}, value.TargetFrames...)
	value.Requirements.Proofs = append([]contract.FactRef{}, value.Requirements.Proofs...)
	value.Requirements.Residuals = append([]contract.FactRef{}, value.Requirements.Residuals...)
	return value
}

func cloneFrameV1(value contract.RestoredContextFrameV1) contract.RestoredContextFrameV1 {
	if value.SemiStable != nil {
		copy := *value.SemiStable
		value.SemiStable = &copy
	}
	return value
}

func cloneGenerationV1(value contract.RestoredContextGenerationV1) contract.RestoredContextGenerationV1 {
	value.Frames = append([]contract.FactRef{}, value.Frames...)
	return value
}

func contextCheckV1(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("%w: context", contract.ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

var _ contextports.RestoreContextMaterializationStoreV1 = (*Memory)(nil)

// Package promptstore provides a process-local PromptAsset and pre-release
// lifecycle reference store. It is not a production backend or publication root.
package promptstore

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
)

type Memory struct {
	mu        sync.RWMutex
	assets    map[string]contract.PromptAssetV1
	lifecycle map[string]contract.ContextPromptLifecycleFactV1
	heads     map[contract.PromptAssetRefV1]contract.FactRef
}

func NewMemory() *Memory {
	return &Memory{
		assets:    make(map[string]contract.PromptAssetV1),
		lifecycle: make(map[string]contract.ContextPromptLifecycleFactV1),
		heads:     make(map[contract.PromptAssetRefV1]contract.FactRef),
	}
}

var _ contextports.ContextPromptLifecycleStoreV1 = (*Memory)(nil)

func (m *Memory) PutPromptAssetV1(ctx context.Context, asset contract.PromptAssetV1) (contract.PromptAssetRefV1, error) {
	if err := promptContextErrV1(ctx); err != nil {
		return contract.PromptAssetRefV1{}, err
	}
	digest, err := asset.DigestValue()
	if err != nil {
		return contract.PromptAssetRefV1{}, err
	}
	copy, err := clonePromptV1(asset)
	if err != nil {
		return contract.PromptAssetRefV1{}, err
	}
	ref := contract.PromptAssetRefV1{ID: asset.ID, Revision: asset.Revision, Digest: digest}
	m.mu.Lock()
	defer m.mu.Unlock()
	if prior, ok := m.assets[asset.ID]; ok {
		priorDigest, _ := prior.DigestValue()
		if prior.Revision != asset.Revision || priorDigest != digest {
			return contract.PromptAssetRefV1{}, fmt.Errorf("%w: immutable prompt asset identity", contract.ErrConflict)
		}
		return ref, nil
	}
	m.assets[asset.ID] = copy
	return ref, nil
}

func (m *Memory) InspectPromptAssetV1(ctx context.Context, ref contract.PromptAssetRefV1) (contract.PromptAssetV1, error) {
	if err := promptContextErrV1(ctx); err != nil {
		return contract.PromptAssetV1{}, err
	}
	if ref.Validate() != nil {
		return contract.PromptAssetV1{}, fmt.Errorf("%w: prompt asset ref", contract.ErrInvalid)
	}
	m.mu.RLock()
	asset, ok := m.assets[ref.ID]
	m.mu.RUnlock()
	if !ok {
		return contract.PromptAssetV1{}, fmt.Errorf("%w: prompt asset", contract.ErrNotFound)
	}
	digest, err := asset.DigestValue()
	if err != nil {
		return contract.PromptAssetV1{}, err
	}
	if asset.Revision != ref.Revision || digest != ref.Digest {
		return contract.PromptAssetV1{}, fmt.Errorf("%w: exact prompt asset", contract.ErrConflict)
	}
	return clonePromptV1(asset)
}

func (m *Memory) CreatePromptDraftV1(ctx context.Context, fact contract.ContextPromptLifecycleFactV1) (contract.FactRef, error) {
	if err := promptContextErrV1(ctx); err != nil {
		return contract.FactRef{}, err
	}
	if fact.Validate() != nil || fact.State != contract.ContextPromptDraftV1 {
		return contract.FactRef{}, fmt.Errorf("%w: prompt draft", contract.ErrInvalid)
	}
	digest, _ := fact.DigestValue()
	ref := contract.FactRef{ID: fact.ID, Revision: fact.Revision, Digest: digest}
	copy, err := clonePromptV1(fact)
	if err != nil {
		return contract.FactRef{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	asset, ok := m.assets[fact.PromptAssetRef.ID]
	if !ok {
		return contract.FactRef{}, fmt.Errorf("%w: prompt asset", contract.ErrNotFound)
	}
	assetDigest, _ := asset.DigestValue()
	if asset.Revision != fact.PromptAssetRef.Revision || assetDigest != fact.PromptAssetRef.Digest {
		return contract.FactRef{}, fmt.Errorf("%w: exact draft prompt asset", contract.ErrConflict)
	}
	if _, exists := m.heads[fact.PromptAssetRef]; exists {
		return contract.FactRef{}, fmt.Errorf("%w: prompt lifecycle already exists", contract.ErrConflict)
	}
	if prior, exists := m.lifecycle[fact.ID]; exists {
		priorDigest, _ := prior.DigestValue()
		if priorDigest != digest {
			return contract.FactRef{}, fmt.Errorf("%w: prompt lifecycle identity", contract.ErrConflict)
		}
	}
	m.lifecycle[fact.ID] = copy
	m.heads[fact.PromptAssetRef] = ref
	return ref, nil
}

func (m *Memory) CompareAndSwapPromptLifecycleV1(ctx context.Context, expected contract.FactRef, next contract.ContextPromptLifecycleFactV1) (contract.FactRef, error) {
	if err := promptContextErrV1(ctx); err != nil {
		return contract.FactRef{}, err
	}
	if expected.Validate() != nil || next.Validate() != nil || next.PreviousLifecycleRef == nil || *next.PreviousLifecycleRef != expected {
		return contract.FactRef{}, fmt.Errorf("%w: prompt lifecycle CAS request", contract.ErrInvalid)
	}
	digest, _ := next.DigestValue()
	ref := contract.FactRef{ID: next.ID, Revision: next.Revision, Digest: digest}
	copy, err := clonePromptV1(next)
	if err != nil {
		return contract.FactRef{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.heads[next.PromptAssetRef]
	if !ok {
		return contract.FactRef{}, fmt.Errorf("%w: prompt lifecycle head", contract.ErrNotFound)
	}
	if current != expected {
		return contract.FactRef{}, fmt.Errorf("%w: prompt lifecycle head CAS", contract.ErrConflict)
	}
	prior, ok := m.lifecycle[expected.ID]
	if !ok {
		return contract.FactRef{}, fmt.Errorf("%w: prior prompt lifecycle", contract.ErrNotFound)
	}
	priorDigest, _ := prior.DigestValue()
	if prior.Revision != expected.Revision || priorDigest != expected.Digest || prior.PromptAssetRef != next.PromptAssetRef || !allowedPromptLifecycleTransitionV1(prior.State, next.State) {
		return contract.FactRef{}, fmt.Errorf("%w: prompt lifecycle transition", contract.ErrConflict)
	}
	if _, exists := m.lifecycle[next.ID]; exists {
		return contract.FactRef{}, fmt.Errorf("%w: prompt lifecycle successor identity", contract.ErrConflict)
	}
	m.lifecycle[next.ID] = copy
	m.heads[next.PromptAssetRef] = ref
	return ref, nil
}

func (m *Memory) InspectPromptLifecycleV1(ctx context.Context, ref contract.FactRef) (contract.ContextPromptLifecycleFactV1, error) {
	if err := promptContextErrV1(ctx); err != nil {
		return contract.ContextPromptLifecycleFactV1{}, err
	}
	if ref.Validate() != nil {
		return contract.ContextPromptLifecycleFactV1{}, fmt.Errorf("%w: prompt lifecycle ref", contract.ErrInvalid)
	}
	m.mu.RLock()
	fact, ok := m.lifecycle[ref.ID]
	m.mu.RUnlock()
	if !ok {
		return contract.ContextPromptLifecycleFactV1{}, fmt.Errorf("%w: prompt lifecycle fact", contract.ErrNotFound)
	}
	digest, _ := fact.DigestValue()
	if fact.Revision != ref.Revision || digest != ref.Digest {
		return contract.ContextPromptLifecycleFactV1{}, fmt.Errorf("%w: exact prompt lifecycle fact", contract.ErrConflict)
	}
	return clonePromptV1(fact)
}

func (m *Memory) InspectPromptLifecycleHeadV1(ctx context.Context, assetRef contract.PromptAssetRefV1) (contract.ContextPromptLifecycleHeadV1, error) {
	if err := promptContextErrV1(ctx); err != nil {
		return contract.ContextPromptLifecycleHeadV1{}, err
	}
	if assetRef.Validate() != nil {
		return contract.ContextPromptLifecycleHeadV1{}, fmt.Errorf("%w: prompt lifecycle head request", contract.ErrInvalid)
	}
	m.mu.RLock()
	lifecycleRef, ok := m.heads[assetRef]
	m.mu.RUnlock()
	if !ok {
		return contract.ContextPromptLifecycleHeadV1{}, fmt.Errorf("%w: prompt lifecycle head", contract.ErrNotFound)
	}
	return contract.ContextPromptLifecycleHeadV1{PromptAssetRef: assetRef, LifecycleRef: lifecycleRef}, nil
}

func allowedPromptLifecycleTransitionV1(from, to contract.ContextPromptLifecycleStateV1) bool {
	if to == contract.ContextPromptRejectedV1 {
		return from == contract.ContextPromptDraftV1 || from == contract.ContextPromptValidatedV1 || from == contract.ContextPromptEvaluatedV1 || from == contract.ContextPromptReviewPendingV1
	}
	return from == contract.ContextPromptDraftV1 && to == contract.ContextPromptValidatedV1 || from == contract.ContextPromptValidatedV1 && to == contract.ContextPromptEvaluatedV1 || from == contract.ContextPromptEvaluatedV1 && to == contract.ContextPromptReviewPendingV1
}

func promptContextErrV1(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("%w: nil context", contract.ErrInvalid)
	}
	return ctx.Err()
}

func clonePromptV1[T any](value T) (T, error) {
	var out T
	payload, err := json.Marshal(value)
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(payload, &out)
	return out, err
}

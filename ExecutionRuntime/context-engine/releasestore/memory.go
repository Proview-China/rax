// Package releasestore provides a process-local pre-release lifecycle store.
// It has no production Recipe current binding and cannot publish, rollback or revoke.
package releasestore

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
	recipes   map[string]contract.ContextRecipe
	lifecycle map[string]contract.ContextRecipeLifecycleFactV1
	heads     map[contract.FactRef]contract.FactRef
}

func NewMemory() *Memory {
	return &Memory{recipes: make(map[string]contract.ContextRecipe), lifecycle: make(map[string]contract.ContextRecipeLifecycleFactV1), heads: make(map[contract.FactRef]contract.FactRef)}
}

var _ contextports.ContextRecipeLifecycleStoreV1 = (*Memory)(nil)

func (m *Memory) PutRecipeV1(ctx context.Context, recipe contract.ContextRecipe) (contract.FactRef, error) {
	if err := ctx.Err(); err != nil {
		return contract.FactRef{}, err
	}
	digest, err := recipe.DigestValue()
	if err != nil {
		return contract.FactRef{}, err
	}
	copy, err := clone(recipe)
	if err != nil {
		return contract.FactRef{}, err
	}
	ref := contract.FactRef{ID: recipe.ID, Revision: recipe.Revision, Digest: digest}
	m.mu.Lock()
	defer m.mu.Unlock()
	if prior, ok := m.recipes[recipe.ID]; ok {
		priorDigest, _ := prior.DigestValue()
		if prior.Revision != recipe.Revision || priorDigest != digest {
			return contract.FactRef{}, fmt.Errorf("%w: immutable recipe identity", contract.ErrConflict)
		}
		return ref, nil
	}
	m.recipes[recipe.ID] = copy
	return ref, nil
}

func (m *Memory) InspectRecipeV1(ctx context.Context, ref contract.FactRef) (contract.ContextRecipe, error) {
	if err := ctx.Err(); err != nil {
		return contract.ContextRecipe{}, err
	}
	if ref.Validate() != nil {
		return contract.ContextRecipe{}, fmt.Errorf("%w: recipe ref", contract.ErrInvalid)
	}
	m.mu.RLock()
	recipe, ok := m.recipes[ref.ID]
	m.mu.RUnlock()
	if !ok {
		return contract.ContextRecipe{}, fmt.Errorf("%w: recipe", contract.ErrNotFound)
	}
	digest, err := recipe.DigestValue()
	if err != nil {
		return contract.ContextRecipe{}, err
	}
	if recipe.Revision != ref.Revision || digest != ref.Digest {
		return contract.ContextRecipe{}, fmt.Errorf("%w: exact recipe", contract.ErrConflict)
	}
	return clone(recipe)
}

func (m *Memory) CreateRecipeDraftV1(ctx context.Context, fact contract.ContextRecipeLifecycleFactV1) (contract.FactRef, error) {
	if err := ctx.Err(); err != nil {
		return contract.FactRef{}, err
	}
	if fact.Validate() != nil || fact.State != contract.ContextRecipeDraftV1 {
		return contract.FactRef{}, fmt.Errorf("%w: recipe draft", contract.ErrInvalid)
	}
	digest, _ := fact.DigestValue()
	ref := contract.FactRef{ID: fact.ID, Revision: fact.Revision, Digest: digest}
	copy, err := clone(fact)
	if err != nil {
		return contract.FactRef{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if recipe, ok := m.recipes[fact.RecipeRef.ID]; !ok {
		return contract.FactRef{}, fmt.Errorf("%w: recipe", contract.ErrNotFound)
	} else if recipeDigest, _ := recipe.DigestValue(); recipe.Revision != fact.RecipeRef.Revision || recipeDigest != fact.RecipeRef.Digest {
		return contract.FactRef{}, fmt.Errorf("%w: exact draft recipe", contract.ErrConflict)
	}
	if _, exists := m.heads[fact.RecipeRef]; exists {
		return contract.FactRef{}, fmt.Errorf("%w: recipe lifecycle already exists", contract.ErrConflict)
	}
	if prior, exists := m.lifecycle[fact.ID]; exists {
		priorDigest, _ := prior.DigestValue()
		if priorDigest != digest {
			return contract.FactRef{}, fmt.Errorf("%w: lifecycle identity", contract.ErrConflict)
		}
	}
	m.lifecycle[fact.ID] = copy
	m.heads[fact.RecipeRef] = ref
	return ref, nil
}

func (m *Memory) CompareAndSwapRecipeLifecycleV1(ctx context.Context, expected contract.FactRef, next contract.ContextRecipeLifecycleFactV1) (contract.FactRef, error) {
	if err := ctx.Err(); err != nil {
		return contract.FactRef{}, err
	}
	if expected.Validate() != nil || next.Validate() != nil || next.PreviousLifecycleRef == nil || *next.PreviousLifecycleRef != expected {
		return contract.FactRef{}, fmt.Errorf("%w: lifecycle CAS request", contract.ErrInvalid)
	}
	digest, _ := next.DigestValue()
	ref := contract.FactRef{ID: next.ID, Revision: 1, Digest: digest}
	copy, err := clone(next)
	if err != nil {
		return contract.FactRef{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.heads[next.RecipeRef]
	if !ok {
		return contract.FactRef{}, fmt.Errorf("%w: lifecycle head", contract.ErrNotFound)
	}
	if current != expected {
		return contract.FactRef{}, fmt.Errorf("%w: lifecycle head CAS", contract.ErrConflict)
	}
	prior, ok := m.lifecycle[expected.ID]
	if !ok {
		return contract.FactRef{}, fmt.Errorf("%w: prior lifecycle", contract.ErrNotFound)
	}
	priorDigest, _ := prior.DigestValue()
	if prior.Revision != expected.Revision || priorDigest != expected.Digest || prior.RecipeRef != next.RecipeRef || !allowedLifecycleTransition(prior.State, next.State) {
		return contract.FactRef{}, fmt.Errorf("%w: lifecycle transition", contract.ErrConflict)
	}
	if _, exists := m.lifecycle[next.ID]; exists {
		return contract.FactRef{}, fmt.Errorf("%w: lifecycle successor identity", contract.ErrConflict)
	}
	m.lifecycle[next.ID] = copy
	m.heads[next.RecipeRef] = ref
	return ref, nil
}

func (m *Memory) InspectRecipeLifecycleV1(ctx context.Context, ref contract.FactRef) (contract.ContextRecipeLifecycleFactV1, error) {
	if err := ctx.Err(); err != nil {
		return contract.ContextRecipeLifecycleFactV1{}, err
	}
	if ref.Validate() != nil {
		return contract.ContextRecipeLifecycleFactV1{}, fmt.Errorf("%w: lifecycle ref", contract.ErrInvalid)
	}
	m.mu.RLock()
	fact, ok := m.lifecycle[ref.ID]
	m.mu.RUnlock()
	if !ok {
		return contract.ContextRecipeLifecycleFactV1{}, fmt.Errorf("%w: lifecycle fact", contract.ErrNotFound)
	}
	digest, _ := fact.DigestValue()
	if fact.Revision != ref.Revision || digest != ref.Digest {
		return contract.ContextRecipeLifecycleFactV1{}, fmt.Errorf("%w: exact lifecycle fact", contract.ErrConflict)
	}
	return clone(fact)
}

func (m *Memory) InspectRecipeLifecycleHeadV1(ctx context.Context, recipeRef contract.FactRef) (contract.ContextRecipeLifecycleHeadV1, error) {
	if err := ctx.Err(); err != nil {
		return contract.ContextRecipeLifecycleHeadV1{}, err
	}
	if recipeRef.Validate() != nil {
		return contract.ContextRecipeLifecycleHeadV1{}, fmt.Errorf("%w: recipe lifecycle head request", contract.ErrInvalid)
	}
	m.mu.RLock()
	lifecycleRef, ok := m.heads[recipeRef]
	m.mu.RUnlock()
	if !ok {
		return contract.ContextRecipeLifecycleHeadV1{}, fmt.Errorf("%w: recipe lifecycle head", contract.ErrNotFound)
	}
	return contract.ContextRecipeLifecycleHeadV1{RecipeRef: recipeRef, LifecycleRef: lifecycleRef}, nil
}

func allowedLifecycleTransition(from, to contract.ContextRecipeLifecycleStateV1) bool {
	if to == contract.ContextRecipeRejectedV1 {
		return from == contract.ContextRecipeDraftV1 || from == contract.ContextRecipeValidatedV1 || from == contract.ContextRecipeEvaluatedV1 || from == contract.ContextRecipeReviewPendingV1
	}
	return from == contract.ContextRecipeDraftV1 && to == contract.ContextRecipeValidatedV1 || from == contract.ContextRecipeValidatedV1 && to == contract.ContextRecipeEvaluatedV1 || from == contract.ContextRecipeEvaluatedV1 && to == contract.ContextRecipeReviewPendingV1
}

func clone[T any](value T) (T, error) {
	var out T
	payload, err := json.Marshal(value)
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(payload, &out)
	return out, err
}

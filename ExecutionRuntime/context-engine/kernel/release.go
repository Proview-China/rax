package kernel

import (
	"context"
	"fmt"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
)

type ContextRecipeLifecycleServiceV1 struct {
	store contextports.ContextRecipeLifecycleStoreV1
}

func NewContextRecipeLifecycleServiceV1(store contextports.ContextRecipeLifecycleStoreV1) (*ContextRecipeLifecycleServiceV1, error) {
	if store == nil {
		return nil, fmt.Errorf("%w: recipe lifecycle store", contract.ErrInvalid)
	}
	return &ContextRecipeLifecycleServiceV1{store: store}, nil
}

func (s *ContextRecipeLifecycleServiceV1) CreateDraft(ctx context.Context, recipe contract.ContextRecipe, draft contract.ContextRecipeLifecycleFactV1) (contract.FactRef, error) {
	if err := ctx.Err(); err != nil {
		return contract.FactRef{}, err
	}
	if err := draft.Validate(); err != nil {
		return contract.FactRef{}, err
	}
	recipeDigest, err := recipe.DigestValue()
	if err != nil {
		return contract.FactRef{}, err
	}
	expectedRecipeRef := contract.FactRef{ID: recipe.ID, Revision: recipe.Revision, Digest: recipeDigest}
	if draft.RecipeRef != expectedRecipeRef {
		return contract.FactRef{}, fmt.Errorf("%w: draft recipe binding", contract.ErrConflict)
	}
	recipeRef, err := s.store.PutRecipeV1(ctx, recipe)
	if err != nil {
		return contract.FactRef{}, err
	}
	if draft.RecipeRef != recipeRef {
		return contract.FactRef{}, fmt.Errorf("%w: draft recipe binding", contract.ErrConflict)
	}
	return s.store.CreateRecipeDraftV1(ctx, draft)
}

func (s *ContextRecipeLifecycleServiceV1) Advance(ctx context.Context, expected contract.FactRef, next contract.ContextRecipeLifecycleFactV1) (contract.FactRef, error) {
	if err := ctx.Err(); err != nil {
		return contract.FactRef{}, err
	}
	if _, err := s.store.InspectRecipeLifecycleV1(ctx, expected); err != nil {
		return contract.FactRef{}, err
	}
	return s.store.CompareAndSwapRecipeLifecycleV1(ctx, expected, next)
}

func (s *ContextRecipeLifecycleServiceV1) Inspect(ctx context.Context, ref contract.FactRef) (contract.ContextRecipeLifecycleFactV1, error) {
	return s.store.InspectRecipeLifecycleV1(ctx, ref)
}

func (s *ContextRecipeLifecycleServiceV1) InspectHead(ctx context.Context, recipeRef contract.FactRef) (contract.ContextRecipeLifecycleHeadV1, error) {
	return s.store.InspectRecipeLifecycleHeadV1(ctx, recipeRef)
}

func (s *ContextRecipeLifecycleServiceV1) ProductionAction(context.Context, contract.ContextRecipeProductionActionV1) error {
	return fmt.Errorf("%w: CTX-D07 production recipe action is not available", contract.ErrUnsupported)
}

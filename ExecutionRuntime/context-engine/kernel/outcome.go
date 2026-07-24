package kernel

import (
	"context"
	"fmt"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
)

type ContextOutcomeCoordinatorV1 struct {
	store contextports.ContextOutcomeFactStoreV1
}

func NewContextOutcomeCoordinatorV1(store contextports.ContextOutcomeFactStoreV1) (*ContextOutcomeCoordinatorV1, error) {
	if store == nil {
		return nil, fmt.Errorf("%w: outcome store", contract.ErrInvalid)
	}
	return &ContextOutcomeCoordinatorV1{store: store}, nil
}

func (c *ContextOutcomeCoordinatorV1) RecordOutcome(ctx context.Context, outcome contract.ContextOutcomeFactV1) (contract.FactRef, error) {
	if err := ctx.Err(); err != nil {
		return contract.FactRef{}, err
	}
	return c.store.PutContextOutcomeV1(ctx, outcome)
}

func (c *ContextOutcomeCoordinatorV1) RecordEvaluation(ctx context.Context, evaluation contract.ContextEvaluationFactV1) (contract.FactRef, error) {
	if err := ctx.Err(); err != nil {
		return contract.FactRef{}, err
	}
	if err := evaluation.Validate(); err != nil {
		return contract.FactRef{}, err
	}
	for _, ref := range evaluation.OutcomeRefs {
		outcome, err := c.store.InspectContextOutcomeV1(ctx, ref)
		if err != nil {
			return contract.FactRef{}, err
		}
		if outcome.EvaluationPolicyRef != evaluation.PolicyRef || (outcome.RecipeRef != evaluation.BaselineRecipeRef && outcome.RecipeRef != evaluation.CandidateRecipeRef) || evaluation.CreatedUnixNano < outcome.CreatedUnixNano || evaluation.ExpiresUnixNano > outcome.ExpiresUnixNano {
			return contract.FactRef{}, fmt.Errorf("%w: evaluation outcome exact binding", contract.ErrConflict)
		}
	}
	return c.store.PutContextEvaluationV1(ctx, evaluation)
}

func (c *ContextOutcomeCoordinatorV1) RecordFeedbackCandidate(ctx context.Context, feedback contract.ContextFeedbackCandidateFactV1) (contract.FactRef, error) {
	if err := ctx.Err(); err != nil {
		return contract.FactRef{}, err
	}
	if err := feedback.Validate(); err != nil {
		return contract.FactRef{}, err
	}
	evaluation, err := c.store.InspectContextEvaluationV1(ctx, feedback.EvaluationRef)
	if err != nil {
		return contract.FactRef{}, err
	}
	if feedback.BaseRecipeRef != evaluation.BaselineRecipeRef || feedback.RiskScorePPM != evaluation.RiskScorePPM || !sameFactRefSliceV1(feedback.OutcomeRefs, evaluation.OutcomeRefs) || feedback.CreatedUnixNano < evaluation.CreatedUnixNano || feedback.ExpiresUnixNano > evaluation.ExpiresUnixNano {
		return contract.FactRef{}, fmt.Errorf("%w: feedback evaluation exact binding", contract.ErrConflict)
	}
	return c.store.PutContextFeedbackCandidateV1(ctx, feedback)
}

func (c *ContextOutcomeCoordinatorV1) InspectOutcome(ctx context.Context, ref contract.FactRef) (contract.ContextOutcomeFactV1, error) {
	return c.store.InspectContextOutcomeV1(ctx, ref)
}

func (c *ContextOutcomeCoordinatorV1) InspectEvaluation(ctx context.Context, ref contract.FactRef) (contract.ContextEvaluationFactV1, error) {
	return c.store.InspectContextEvaluationV1(ctx, ref)
}

func (c *ContextOutcomeCoordinatorV1) InspectFeedbackCandidate(ctx context.Context, ref contract.FactRef) (contract.ContextFeedbackCandidateFactV1, error) {
	return c.store.InspectContextFeedbackCandidateV1(ctx, ref)
}

func sameFactRefSliceV1(left, right []contract.FactRef) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

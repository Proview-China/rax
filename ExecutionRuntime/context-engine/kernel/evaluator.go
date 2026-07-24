package kernel

import (
	"context"
	"fmt"
	"sort"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

func PrepareContextEvaluationInputV1(
	ctx context.Context,
	evaluationID string,
	evaluatorRef contract.ContextEvaluatorRefV1,
	outcomes []contract.ContextOutcomeFactV1,
	baselineRecipeRef contract.FactRef,
	candidateRecipeRef contract.FactRef,
	policyRef contract.FactRef,
	checkedUnixNano int64,
	notAfterUnixNano int64,
) (contract.ContextEvaluationInputV1, error) {
	if err := evaluatorKernelContextErrV1(ctx); err != nil {
		return contract.ContextEvaluationInputV1{}, err
	}
	if evaluatorRef.Validate() != nil || baselineRecipeRef.Validate() != nil || candidateRecipeRef.Validate() != nil || baselineRecipeRef == candidateRecipeRef || policyRef.Validate() != nil || checkedUnixNano <= 0 || notAfterUnixNano <= checkedUnixNano {
		return contract.ContextEvaluationInputV1{}, fmt.Errorf("%w: evaluation preparation", contract.ErrInvalid)
	}
	refs, expires, err := exactEvaluationOutcomesV1(ctx, outcomes, baselineRecipeRef, candidateRecipeRef, policyRef, checkedUnixNano, notAfterUnixNano)
	if err != nil {
		return contract.ContextEvaluationInputV1{}, err
	}
	return contract.SealContextEvaluationInputV1(contract.ContextEvaluationInputV1{
		EvaluationID: evaluationID, EvaluatorRef: evaluatorRef, OutcomeRefs: refs,
		BaselineRecipeRef: baselineRecipeRef, CandidateRecipeRef: candidateRecipeRef,
		PolicyRef: policyRef, CheckedUnixNano: checkedUnixNano, ExpiresUnixNano: expires,
	})
}

func AdmitContextEvaluationObservationV1(ctx context.Context, outcomes []contract.ContextOutcomeFactV1, input contract.ContextEvaluationInputV1, observation contract.ContextEvaluationObservationV1) (contract.ContextEvaluationFactV1, contract.FactRef, error) {
	if err := evaluatorKernelContextErrV1(ctx); err != nil {
		return contract.ContextEvaluationFactV1{}, contract.FactRef{}, err
	}
	if err := input.Validate(); err != nil {
		return contract.ContextEvaluationFactV1{}, contract.FactRef{}, err
	}
	if err := observation.Validate(); err != nil {
		return contract.ContextEvaluationFactV1{}, contract.FactRef{}, err
	}
	fresh, err := PrepareContextEvaluationInputV1(ctx, input.EvaluationID, input.EvaluatorRef, outcomes, input.BaselineRecipeRef, input.CandidateRecipeRef, input.PolicyRef, input.CheckedUnixNano, input.ExpiresUnixNano)
	if err != nil {
		return contract.ContextEvaluationFactV1{}, contract.FactRef{}, err
	}
	if fresh.InputDigest != input.InputDigest || observation.EvaluatorRef != input.EvaluatorRef || observation.InputDigest != input.InputDigest || observation.PolicyRef != input.PolicyRef || !sameFactRefSliceV1(observation.OutcomeRefs, input.OutcomeRefs) {
		return contract.ContextEvaluationFactV1{}, contract.FactRef{}, fmt.Errorf("%w: evaluator observation exact binding", contract.ErrConflict)
	}
	expires := input.ExpiresUnixNano
	if observation.ExpiresUnixNano < expires {
		expires = observation.ExpiresUnixNano
	}
	if observation.ObservedUnixNano < input.CheckedUnixNano || observation.ObservedUnixNano >= expires {
		return contract.ContextEvaluationFactV1{}, contract.FactRef{}, fmt.Errorf("%w: evaluator observation currentness", contract.ErrExpired)
	}
	evaluation := contract.ContextEvaluationFactV1{
		ContractVersion: contract.Version, ID: input.EvaluationID, Revision: 1,
		OutcomeRefs:       append([]contract.FactRef(nil), input.OutcomeRefs...),
		BaselineRecipeRef: input.BaselineRecipeRef, CandidateRecipeRef: input.CandidateRecipeRef,
		PolicyRef: input.PolicyRef, QualityScorePPM: observation.QualityScorePPM,
		EconomicScorePPM: observation.EconomicScorePPM, RiskScorePPM: observation.RiskScorePPM,
		Disposition: observation.Disposition, Evidence: append([]contract.EvidenceRef(nil), observation.Evidence...),
		CreatedUnixNano: observation.ObservedUnixNano, ExpiresUnixNano: expires,
	}
	digest, err := evaluation.DigestValue()
	if err != nil {
		return contract.ContextEvaluationFactV1{}, contract.FactRef{}, err
	}
	return evaluation, contract.FactRef{ID: evaluation.ID, Revision: evaluation.Revision, Digest: digest}, nil
}

func BuildContextFeedbackCandidateV1(
	ctx context.Context,
	feedbackCandidateID string,
	outcomes []contract.ContextOutcomeFactV1,
	evaluation contract.ContextEvaluationFactV1,
	changeDigest contract.Digest,
	evidence []contract.EvidenceRef,
	createdUnixNano int64,
	notAfterUnixNano int64,
) (contract.ContextFeedbackCandidateFactV1, contract.FactRef, error) {
	if err := evaluatorKernelContextErrV1(ctx); err != nil {
		return contract.ContextFeedbackCandidateFactV1{}, contract.FactRef{}, err
	}
	if err := evaluation.Validate(); err != nil {
		return contract.ContextFeedbackCandidateFactV1{}, contract.FactRef{}, err
	}
	if changeDigest.Validate() != nil || createdUnixNano < evaluation.CreatedUnixNano || notAfterUnixNano <= createdUnixNano {
		return contract.ContextFeedbackCandidateFactV1{}, contract.FactRef{}, fmt.Errorf("%w: feedback candidate request", contract.ErrInvalid)
	}
	refs, expires, err := exactEvaluationOutcomesV1(ctx, outcomes, evaluation.BaselineRecipeRef, evaluation.CandidateRecipeRef, evaluation.PolicyRef, createdUnixNano, notAfterUnixNano)
	if err != nil {
		return contract.ContextFeedbackCandidateFactV1{}, contract.FactRef{}, err
	}
	if !sameFactRefSliceV1(refs, evaluation.OutcomeRefs) {
		return contract.ContextFeedbackCandidateFactV1{}, contract.FactRef{}, fmt.Errorf("%w: feedback outcome exact binding", contract.ErrConflict)
	}
	if evaluation.ExpiresUnixNano < expires {
		expires = evaluation.ExpiresUnixNano
	}
	if createdUnixNano >= expires {
		return contract.ContextFeedbackCandidateFactV1{}, contract.FactRef{}, fmt.Errorf("%w: feedback candidate currentness", contract.ErrExpired)
	}
	evidenceCopy := append([]contract.EvidenceRef(nil), evidence...)
	sort.Slice(evidenceCopy, func(i, j int) bool {
		if evidenceCopy[i].ID != evidenceCopy[j].ID {
			return evidenceCopy[i].ID < evidenceCopy[j].ID
		}
		return evidenceCopy[i].Digest < evidenceCopy[j].Digest
	})
	feedback := contract.ContextFeedbackCandidateFactV1{
		ContractVersion: contract.Version, ID: feedbackCandidateID, Revision: 1,
		BaseRecipeRef: evaluation.BaselineRecipeRef, OutcomeRefs: append([]contract.FactRef(nil), evaluation.OutcomeRefs...),
		EvaluationRef: contract.FactRef{ID: evaluation.ID, Revision: evaluation.Revision},
		ChangeDigest:  changeDigest, RiskScorePPM: evaluation.RiskScorePPM, Evidence: evidenceCopy,
		State: contract.ContextFeedbackEvaluatedV1, CreatedUnixNano: createdUnixNano, ExpiresUnixNano: expires,
	}
	evaluationDigest, err := evaluation.DigestValue()
	if err != nil {
		return contract.ContextFeedbackCandidateFactV1{}, contract.FactRef{}, err
	}
	feedback.EvaluationRef.Digest = evaluationDigest
	feedbackDigest, err := feedback.DigestValue()
	if err != nil {
		return contract.ContextFeedbackCandidateFactV1{}, contract.FactRef{}, err
	}
	return feedback, contract.FactRef{ID: feedback.ID, Revision: feedback.Revision, Digest: feedbackDigest}, nil
}

func exactEvaluationOutcomesV1(ctx context.Context, outcomes []contract.ContextOutcomeFactV1, baselineRecipeRef, candidateRecipeRef, policyRef contract.FactRef, checkedUnixNano, notAfterUnixNano int64) ([]contract.FactRef, int64, error) {
	if len(outcomes) == 0 || len(outcomes) > contract.MaxEngineeringOutcomesV1 {
		return nil, 0, fmt.Errorf("%w: evaluation outcome count", contract.ErrLimitExceeded)
	}
	refs := make([]contract.FactRef, 0, len(outcomes))
	identities := make(map[string]contract.FactRef, len(outcomes))
	expires := notAfterUnixNano
	seenBaseline := false
	seenCandidate := false
	for _, outcome := range outcomes {
		if err := evaluatorKernelContextErrV1(ctx); err != nil {
			return nil, 0, err
		}
		digest, err := outcome.DigestValue()
		if err != nil {
			return nil, 0, err
		}
		ref := contract.FactRef{ID: outcome.ID, Revision: outcome.Revision, Digest: digest}
		if prior, exists := identities[ref.ID]; exists {
			if prior != ref {
				return nil, 0, fmt.Errorf("%w: same-id outcome drift", contract.ErrConflict)
			}
			return nil, 0, fmt.Errorf("%w: duplicate outcome", contract.ErrConflict)
		}
		identities[ref.ID] = ref
		if outcome.EvaluationPolicyRef != policyRef || (outcome.RecipeRef != baselineRecipeRef && outcome.RecipeRef != candidateRecipeRef) {
			return nil, 0, fmt.Errorf("%w: outcome evaluation closure", contract.ErrConflict)
		}
		if checkedUnixNano < outcome.CreatedUnixNano || checkedUnixNano >= outcome.ExpiresUnixNano {
			return nil, 0, fmt.Errorf("%w: outcome evaluation currentness", contract.ErrExpired)
		}
		if outcome.RecipeRef == baselineRecipeRef {
			seenBaseline = true
		}
		if outcome.RecipeRef == candidateRecipeRef {
			seenCandidate = true
		}
		if outcome.ExpiresUnixNano < expires {
			expires = outcome.ExpiresUnixNano
		}
		refs = append(refs, ref)
	}
	if !seenBaseline || !seenCandidate {
		return nil, 0, fmt.Errorf("%w: two-sided evaluation outcomes required", contract.ErrConflict)
	}
	if checkedUnixNano >= expires {
		return nil, 0, fmt.Errorf("%w: evaluation lifetime", contract.ErrExpired)
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].ID != refs[j].ID {
			return refs[i].ID < refs[j].ID
		}
		if refs[i].Revision != refs[j].Revision {
			return refs[i].Revision < refs[j].Revision
		}
		return refs[i].Digest < refs[j].Digest
	})
	return refs, expires, nil
}

func evaluatorKernelContextErrV1(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("%w: nil context", contract.ErrInvalid)
	}
	return ctx.Err()
}

package kernel

import (
	"context"
	"fmt"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

func CompareContextRecipesV1(ctx context.Context, base, candidate contract.ContextRecipe, checkedUnixNano, expiresUnixNano int64) (contract.ContextRecipeComparisonV1, error) {
	if ctx == nil {
		return contract.ContextRecipeComparisonV1{}, fmt.Errorf("%w: nil context", contract.ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return contract.ContextRecipeComparisonV1{}, err
	}
	baseDigest, err := base.DigestValue()
	if err != nil {
		return contract.ContextRecipeComparisonV1{}, err
	}
	candidateDigest, err := candidate.DigestValue()
	if err != nil {
		return contract.ContextRecipeComparisonV1{}, err
	}
	if checkedUnixNano <= 0 || expiresUnixNano <= checkedUnixNano {
		return contract.ContextRecipeComparisonV1{}, fmt.Errorf("%w: recipe comparison lifetime", contract.ErrInvalid)
	}
	if checkedUnixNano < base.CreatedUnixNano || checkedUnixNano < candidate.CreatedUnixNano || checkedUnixNano >= base.ExpiresUnixNano || checkedUnixNano >= candidate.ExpiresUnixNano {
		return contract.ContextRecipeComparisonV1{}, fmt.Errorf("%w: recipe comparison checked time", contract.ErrExpired)
	}
	if expiresUnixNano > base.ExpiresUnixNano || expiresUnixNano > candidate.ExpiresUnixNano {
		return contract.ContextRecipeComparisonV1{}, fmt.Errorf("%w: recipe comparison exceeds recipe lifetime", contract.ErrExpired)
	}
	changes := make([]contract.ContextRecipeChangeV1, 0, 16)
	appendModified := func(path string, before, after any, kind contract.ContextRecipeChangeKindV1) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		beforeDigest, err := contract.DigestJSON(before)
		if err != nil {
			return err
		}
		afterDigest, err := contract.DigestJSON(after)
		if err != nil {
			return err
		}
		if beforeDigest != afterDigest {
			changes = append(changes, contract.ContextRecipeChangeV1{FieldPath: path, Kind: kind, BeforeDigest: &beforeDigest, AfterDigest: &afterDigest})
		}
		return nil
	}
	for _, field := range []struct {
		path          string
		before, after any
	}{
		{"recipe_id", base.ID, candidate.ID}, {"semantic_version", base.SemanticVersion, candidate.SemanticVersion},
		{"revision", base.Revision, candidate.Revision}, {"owner", base.Owner, candidate.Owner},
		{"budget", base.Budget, candidate.Budget}, {"render_version", base.RenderVersion, candidate.RenderVersion},
		{"created_unix_nano", base.CreatedUnixNano, candidate.CreatedUnixNano}, {"expires_unix_nano", base.ExpiresUnixNano, candidate.ExpiresUnixNano},
	} {
		if err := appendModified(field.path, field.before, field.after, contract.ContextRecipeChangeModifiedV1); err != nil {
			return contract.ContextRecipeComparisonV1{}, err
		}
	}
	baseKinds := make([]contract.FragmentKind, len(base.Rules))
	candidateKinds := make([]contract.FragmentKind, len(candidate.Rules))
	baseRules := make(map[contract.FragmentKind]contract.FragmentRule, len(base.Rules))
	candidateRules := make(map[contract.FragmentKind]contract.FragmentRule, len(candidate.Rules))
	for index, rule := range base.Rules {
		baseKinds[index], baseRules[rule.Kind] = rule.Kind, rule
	}
	for index, rule := range candidate.Rules {
		candidateKinds[index], candidateRules[rule.Kind] = rule.Kind, rule
	}
	if err := appendModified("rules_order", baseKinds, candidateKinds, contract.ContextRecipeChangeReorderedV1); err != nil {
		return contract.ContextRecipeComparisonV1{}, err
	}
	for kind, rule := range baseRules {
		candidateRule, ok := candidateRules[kind]
		path := "rules/" + string(kind)
		if !ok {
			digest, digestErr := contract.DigestJSON(rule)
			if digestErr != nil {
				return contract.ContextRecipeComparisonV1{}, digestErr
			}
			changes = append(changes, contract.ContextRecipeChangeV1{FieldPath: path, Kind: contract.ContextRecipeChangeRemovedV1, BeforeDigest: &digest})
			continue
		}
		if err := appendModified(path, rule, candidateRule, contract.ContextRecipeChangeModifiedV1); err != nil {
			return contract.ContextRecipeComparisonV1{}, err
		}
	}
	for kind, rule := range candidateRules {
		if _, ok := baseRules[kind]; ok {
			continue
		}
		digest, digestErr := contract.DigestJSON(rule)
		if digestErr != nil {
			return contract.ContextRecipeComparisonV1{}, digestErr
		}
		changes = append(changes, contract.ContextRecipeChangeV1{FieldPath: "rules/" + string(kind), Kind: contract.ContextRecipeChangeAddedV1, AfterDigest: &digest})
	}
	changes = contract.CanonicalRecipeChangesV1(changes)
	result := contract.ContextRecipeComparisonV1{
		ContractVersion: contract.Version,
		BaseRecipeRef:   contract.FactRef{ID: base.ID, Revision: base.Revision, Digest: baseDigest},
		CandidateRef:    contract.FactRef{ID: candidate.ID, Revision: candidate.Revision, Digest: candidateDigest},
		Changes:         changes,
		CheckedUnixNano: checkedUnixNano,
		ExpiresUnixNano: expiresUnixNano,
	}
	result.ComparisonDigest, err = contract.DigestJSON(result)
	if err != nil {
		return contract.ContextRecipeComparisonV1{}, err
	}
	if err := ctx.Err(); err != nil {
		return contract.ContextRecipeComparisonV1{}, err
	}
	return result, result.Validate()
}

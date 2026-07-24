package contract

import (
	"fmt"
	"sort"
)

type FrameRegion string

const (
	RegionStablePrefix FrameRegion = "stable_prefix"
	RegionSemiStable   FrameRegion = "semi_stable"
	RegionDynamicTail  FrameRegion = "dynamic_tail"
)

type DegradationPolicy string

const (
	DegradeReject  DegradationPolicy = "reject"
	DegradeExclude DegradationPolicy = "exclude"
)

type FragmentRule struct {
	Kind        FragmentKind      `json:"kind"`
	Region      FrameRegion       `json:"region"`
	Required    bool              `json:"required"`
	MaxTokens   uint64            `json:"max_tokens"`
	Degradation DegradationPolicy `json:"degradation"`
}

func (r FragmentRule) Validate() error {
	if !validFragmentKind(r.Kind) || !validRegion(r.Region) || r.MaxTokens == 0 {
		return fmt.Errorf("%w: fragment rule", ErrInvalid)
	}
	if r.Required && r.Degradation != DegradeReject || !r.Required && r.Degradation != DegradeExclude && r.Degradation != DegradeReject {
		return fmt.Errorf("%w: fragment degradation", ErrInvalid)
	}
	return nil
}

type BudgetPolicy struct {
	TotalTokens     uint64 `json:"total_tokens"`
	StablePrefixMax uint64 `json:"stable_prefix_max"`
	SemiStableMax   uint64 `json:"semi_stable_max"`
	DynamicTailMax  uint64 `json:"dynamic_tail_max"`
}

func (p BudgetPolicy) Validate() error {
	if p.TotalTokens == 0 || p.StablePrefixMax == 0 || p.SemiStableMax == 0 || p.DynamicTailMax == 0 || p.StablePrefixMax+p.SemiStableMax+p.DynamicTailMax < p.TotalTokens {
		return fmt.Errorf("%w: budget policy", ErrInvalid)
	}
	return nil
}

type ContextRecipe struct {
	ContractVersion string         `json:"contract_version"`
	ID              string         `json:"recipe_id"`
	SemanticVersion string         `json:"semantic_version"`
	Revision        uint64         `json:"revision"`
	Owner           OwnerRef       `json:"owner"`
	Rules           []FragmentRule `json:"rules"`
	Budget          BudgetPolicy   `json:"budget"`
	RenderVersion   string         `json:"render_version"`
	CreatedUnixNano int64          `json:"created_unix_nano"`
	ExpiresUnixNano int64          `json:"expires_unix_nano"`
}

func (r ContextRecipe) Validate() error {
	if ValidateContract(r.ContractVersion) != nil || validateID(r.ID) != nil || validateID(r.SemanticVersion) != nil || r.Revision == 0 || r.Owner.Validate() != nil || r.Budget.Validate() != nil || validateID(r.RenderVersion) != nil || validateTimes(r.CreatedUnixNano, r.ExpiresUnixNano) != nil {
		return fmt.Errorf("%w: recipe", ErrInvalid)
	}
	if len(r.Rules) == 0 || len(r.Rules) > 64 {
		return fmt.Errorf("%w: recipe rules", ErrInvalid)
	}
	seen := make(map[FragmentKind]struct{}, len(r.Rules))
	for _, rule := range r.Rules {
		if err := rule.Validate(); err != nil {
			return err
		}
		if _, exists := seen[rule.Kind]; exists {
			return fmt.Errorf("%w: duplicate recipe kind", ErrConflict)
		}
		seen[rule.Kind] = struct{}{}
	}
	return nil
}

func (r ContextRecipe) DigestValue() (Digest, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	return DigestJSON(r)
}

func (r ContextRecipe) Rule(kind FragmentKind) (FragmentRule, bool) {
	for _, rule := range r.Rules {
		if rule.Kind == kind {
			return rule, true
		}
	}
	return FragmentRule{}, false
}

type AdmissionDisposition string

const (
	AdmissionAdmitted AdmissionDisposition = "admitted"
	AdmissionExcluded AdmissionDisposition = "excluded"
	AdmissionRejected AdmissionDisposition = "rejected"
	AdmissionResidual AdmissionDisposition = "residual"
)

type AdmissionDecision struct {
	CandidateRef FactRef              `json:"candidate_ref"`
	Disposition  AdmissionDisposition `json:"disposition"`
	Reason       string               `json:"reason"`
	Region       FrameRegion          `json:"region,omitempty"`
	Tokens       uint64               `json:"tokens"`
}

func (d AdmissionDecision) Validate() error {
	if d.CandidateRef.Validate() != nil || validateID(d.Reason) != nil {
		return fmt.Errorf("%w: admission decision", ErrInvalid)
	}
	switch d.Disposition {
	case AdmissionAdmitted:
		if !validRegion(d.Region) || d.Tokens == 0 {
			return fmt.Errorf("%w: admitted decision", ErrInvalid)
		}
	case AdmissionExcluded, AdmissionRejected, AdmissionResidual:
		if d.Region != "" {
			return fmt.Errorf("%w: non-admitted region", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: admission disposition", ErrInvalid)
	}
	return nil
}

func StableSortCandidates(candidates []ContextCandidate, recipe ContextRecipe) []ContextCandidate {
	return stableSortCandidatesObservedV1(candidates, recipe, nil)
}

func stableSortCandidatesObservedV1(candidates []ContextCandidate, recipe ContextRecipe, comparison func()) []ContextCandidate {
	order := make(map[FragmentKind]int, len(recipe.Rules))
	for index, rule := range recipe.Rules {
		order[rule.Kind] = index
	}
	result := append([]ContextCandidate(nil), candidates...)
	sort.SliceStable(result, func(i, j int) bool {
		if comparison != nil {
			comparison()
		}
		li, lok := order[result[i].Kind]
		rj, rok := order[result[j].Kind]
		if lok != rok {
			return lok
		}
		if li != rj {
			return li < rj
		}
		if result[i].SourceRef != result[j].SourceRef {
			return result[i].SourceRef < result[j].SourceRef
		}
		return result[i].ID < result[j].ID
	})
	return result
}

func validRegion(v FrameRegion) bool {
	return v == RegionStablePrefix || v == RegionSemiStable || v == RegionDynamicTail
}

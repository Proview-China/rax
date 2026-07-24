package contract

import (
	"fmt"
	"sort"
)

type ContextRecipeChangeKindV1 string

const (
	ContextRecipeChangeAddedV1     ContextRecipeChangeKindV1 = "added"
	ContextRecipeChangeRemovedV1   ContextRecipeChangeKindV1 = "removed"
	ContextRecipeChangeModifiedV1  ContextRecipeChangeKindV1 = "modified"
	ContextRecipeChangeReorderedV1 ContextRecipeChangeKindV1 = "reordered"
)

type ContextRecipeChangeV1 struct {
	FieldPath    string                    `json:"field_path"`
	Kind         ContextRecipeChangeKindV1 `json:"kind"`
	BeforeDigest *Digest                   `json:"before_digest,omitempty"`
	AfterDigest  *Digest                   `json:"after_digest,omitempty"`
}

func (c ContextRecipeChangeV1) Validate() error {
	if validateID(c.FieldPath) != nil {
		return fmt.Errorf("%w: recipe comparison field path", ErrInvalid)
	}
	if c.BeforeDigest != nil && c.BeforeDigest.Validate() != nil || c.AfterDigest != nil && c.AfterDigest.Validate() != nil {
		return fmt.Errorf("%w: recipe comparison field digest", ErrInvalid)
	}
	switch c.Kind {
	case ContextRecipeChangeAddedV1:
		if c.BeforeDigest != nil || c.AfterDigest == nil {
			return fmt.Errorf("%w: added recipe change", ErrConflict)
		}
	case ContextRecipeChangeRemovedV1:
		if c.BeforeDigest == nil || c.AfterDigest != nil {
			return fmt.Errorf("%w: removed recipe change", ErrConflict)
		}
	case ContextRecipeChangeModifiedV1, ContextRecipeChangeReorderedV1:
		if c.BeforeDigest == nil || c.AfterDigest == nil || *c.BeforeDigest == *c.AfterDigest {
			return fmt.Errorf("%w: changed recipe field", ErrConflict)
		}
	default:
		return fmt.Errorf("%w: recipe change kind", ErrInvalid)
	}
	return nil
}

type ContextRecipeComparisonV1 struct {
	ContractVersion  string                  `json:"contract_version"`
	BaseRecipeRef    FactRef                 `json:"base_recipe_ref"`
	CandidateRef     FactRef                 `json:"candidate_recipe_ref"`
	Changes          []ContextRecipeChangeV1 `json:"changes"`
	CheckedUnixNano  int64                   `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                   `json:"expires_unix_nano"`
	ComparisonDigest Digest                  `json:"comparison_digest"`
}

func (c ContextRecipeComparisonV1) Validate() error {
	if ValidateContract(c.ContractVersion) != nil {
		return fmt.Errorf("%w: recipe comparison contract", ErrInvalid)
	}
	if c.BaseRecipeRef.Validate() != nil || c.CandidateRef.Validate() != nil {
		return fmt.Errorf("%w: recipe comparison refs", ErrInvalid)
	}
	if c.Changes == nil {
		return fmt.Errorf("%w: recipe comparison changes", ErrInvalid)
	}
	if validateTimes(c.CheckedUnixNano, c.ExpiresUnixNano) != nil {
		return fmt.Errorf("%w: recipe comparison lifetime", ErrInvalid)
	}
	if c.ComparisonDigest.Validate() != nil {
		return fmt.Errorf("%w: recipe comparison digest", ErrInvalid)
	}
	for index := range c.Changes {
		if err := c.Changes[index].Validate(); err != nil {
			return err
		}
		if index > 0 && c.Changes[index-1].FieldPath >= c.Changes[index].FieldPath {
			return fmt.Errorf("%w: non-canonical recipe changes", ErrConflict)
		}
	}
	if c.BaseRecipeRef == c.CandidateRef && len(c.Changes) != 0 {
		return fmt.Errorf("%w: identical recipe refs changed", ErrConflict)
	}
	return nil
}

func (c ContextRecipeComparisonV1) DigestValue() (Digest, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	copy := c
	copy.ComparisonDigest = ""
	return DigestJSON(copy)
}

func CanonicalRecipeChangesV1(values []ContextRecipeChangeV1) []ContextRecipeChangeV1 {
	result := make([]ContextRecipeChangeV1, len(values))
	copy(result, values)
	sort.Slice(result, func(i, j int) bool {
		if result[i].FieldPath != result[j].FieldPath {
			return result[i].FieldPath < result[j].FieldPath
		}
		return result[i].Kind < result[j].Kind
	})
	return result
}

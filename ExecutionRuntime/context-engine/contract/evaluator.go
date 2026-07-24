package contract

import (
	"fmt"
	"sort"
)

const MaxEngineeringOutcomesV1 = 64

// ContextEvaluatorRefV1 is a nominal reference to an evaluator implementation
// and its frozen policy-compatible configuration. It is not a FactRef.
type ContextEvaluatorRefV1 struct {
	ID       string `json:"id"`
	Revision uint64 `json:"revision"`
	Digest   Digest `json:"digest"`
}

func (r ContextEvaluatorRefV1) Validate() error {
	if validateID(r.ID) != nil || r.Revision == 0 || r.Digest.Validate() != nil {
		return fmt.Errorf("%w: context evaluator reference", ErrInvalid)
	}
	return nil
}

type ContextEvaluationInputV1 struct {
	ContractVersion    string                `json:"contract_version"`
	EvaluationID       string                `json:"evaluation_id"`
	EvaluatorRef       ContextEvaluatorRefV1 `json:"evaluator_ref"`
	OutcomeRefs        []FactRef             `json:"outcome_refs"`
	BaselineRecipeRef  FactRef               `json:"baseline_recipe_ref"`
	CandidateRecipeRef FactRef               `json:"candidate_recipe_ref"`
	PolicyRef          FactRef               `json:"policy_ref"`
	CheckedUnixNano    int64                 `json:"checked_unix_nano"`
	ExpiresUnixNano    int64                 `json:"expires_unix_nano"`
	InputDigest        Digest                `json:"input_digest"`
}

func (v ContextEvaluationInputV1) digestValue() (Digest, error) {
	copy := v
	copy.InputDigest = ""
	return DigestJSON(copy)
}

func (v ContextEvaluationInputV1) Validate() error {
	if ValidateContract(v.ContractVersion) != nil || validateID(v.EvaluationID) != nil || v.EvaluatorRef.Validate() != nil || v.BaselineRecipeRef.Validate() != nil || v.CandidateRecipeRef.Validate() != nil || v.PolicyRef.Validate() != nil || v.BaselineRecipeRef == v.CandidateRecipeRef || validateTimes(v.CheckedUnixNano, v.ExpiresUnixNano) != nil || v.InputDigest.Validate() != nil {
		return fmt.Errorf("%w: context evaluation input", ErrInvalid)
	}
	if len(v.OutcomeRefs) == 0 || len(v.OutcomeRefs) > MaxEngineeringOutcomesV1 || !canonicalFactRefsV1(v.OutcomeRefs) {
		return fmt.Errorf("%w: context evaluation input outcomes", ErrConflict)
	}
	want, err := v.digestValue()
	if err != nil || want != v.InputDigest {
		return fmt.Errorf("%w: context evaluation input digest", ErrConflict)
	}
	return nil
}

func SealContextEvaluationInputV1(v ContextEvaluationInputV1) (ContextEvaluationInputV1, error) {
	v.ContractVersion = Version
	v.OutcomeRefs = canonicalEvaluatorFactRefsV1(v.OutcomeRefs)
	v.InputDigest = ""
	digest, err := v.digestValue()
	if err != nil {
		return ContextEvaluationInputV1{}, err
	}
	v.InputDigest = digest
	return v, v.Validate()
}

type ContextEvaluationObservationV1 struct {
	ContractVersion   string                         `json:"contract_version"`
	EvaluatorRef      ContextEvaluatorRefV1          `json:"evaluator_ref"`
	InputDigest       Digest                         `json:"input_digest"`
	OutcomeRefs       []FactRef                      `json:"outcome_refs"`
	PolicyRef         FactRef                        `json:"policy_ref"`
	QualityScorePPM   uint32                         `json:"quality_score_ppm"`
	EconomicScorePPM  uint32                         `json:"economic_score_ppm"`
	RiskScorePPM      uint32                         `json:"risk_score_ppm"`
	Disposition       ContextEvaluationDispositionV1 `json:"disposition"`
	Evidence          []EvidenceRef                  `json:"evidence"`
	ObservedUnixNano  int64                          `json:"observed_unix_nano"`
	ExpiresUnixNano   int64                          `json:"expires_unix_nano"`
	ObservationDigest Digest                         `json:"observation_digest"`
}

func (v ContextEvaluationObservationV1) digestValue() (Digest, error) {
	copy := v
	copy.ObservationDigest = ""
	return DigestJSON(copy)
}

func (v ContextEvaluationObservationV1) Validate() error {
	if ValidateContract(v.ContractVersion) != nil || v.EvaluatorRef.Validate() != nil || v.InputDigest.Validate() != nil || v.PolicyRef.Validate() != nil || v.QualityScorePPM > ScorePPMMaxV1 || v.EconomicScorePPM > ScorePPMMaxV1 || v.RiskScorePPM > ScorePPMMaxV1 || validateTimes(v.ObservedUnixNano, v.ExpiresUnixNano) != nil || v.ObservationDigest.Validate() != nil {
		return fmt.Errorf("%w: context evaluation observation", ErrInvalid)
	}
	if v.Disposition != ContextEvaluationBetterV1 && v.Disposition != ContextEvaluationWorseV1 && v.Disposition != ContextEvaluationInconclusiveV1 {
		return fmt.Errorf("%w: context evaluation observation disposition", ErrInvalid)
	}
	if len(v.OutcomeRefs) == 0 || len(v.OutcomeRefs) > MaxEngineeringOutcomesV1 || !canonicalFactRefsV1(v.OutcomeRefs) || len(v.Evidence) == 0 || len(v.Evidence) > MaxOutcomeRefsV1 || !canonicalEvidenceRefsV1(v.Evidence) {
		return fmt.Errorf("%w: context evaluation observation references", ErrConflict)
	}
	want, err := v.digestValue()
	if err != nil || want != v.ObservationDigest {
		return fmt.Errorf("%w: context evaluation observation digest", ErrConflict)
	}
	return nil
}

func SealContextEvaluationObservationV1(v ContextEvaluationObservationV1) (ContextEvaluationObservationV1, error) {
	v.ContractVersion = Version
	v.OutcomeRefs = canonicalEvaluatorFactRefsV1(v.OutcomeRefs)
	v.Evidence = canonicalEvaluatorEvidenceRefsV1(v.Evidence)
	v.ObservationDigest = ""
	digest, err := v.digestValue()
	if err != nil {
		return ContextEvaluationObservationV1{}, err
	}
	v.ObservationDigest = digest
	return v, v.Validate()
}

func canonicalEvaluatorFactRefsV1(refs []FactRef) []FactRef {
	result := append([]FactRef(nil), refs...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].ID != result[j].ID {
			return result[i].ID < result[j].ID
		}
		if result[i].Revision != result[j].Revision {
			return result[i].Revision < result[j].Revision
		}
		return result[i].Digest < result[j].Digest
	})
	return result
}

func canonicalEvaluatorEvidenceRefsV1(refs []EvidenceRef) []EvidenceRef {
	result := append([]EvidenceRef(nil), refs...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].ID != result[j].ID {
			return result[i].ID < result[j].ID
		}
		return result[i].Digest < result[j].Digest
	})
	return result
}

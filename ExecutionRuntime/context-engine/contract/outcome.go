package contract

import (
	"fmt"
	"strings"
)

const MaxOutcomeRefsV1 = 512
const ScorePPMMaxV1 = 1_000_000

type ContextOutcomeMetricsV1 struct {
	InputTokens               uint64 `json:"input_tokens"`
	OutputTokens              uint64 `json:"output_tokens"`
	CacheEligiblePrefixTokens uint64 `json:"cache_eligible_prefix_tokens"`
	CacheReadTokens           uint64 `json:"cache_read_tokens"`
	CacheWriteTokens          uint64 `json:"cache_write_tokens"`
	DynamicTokens             uint64 `json:"dynamic_tokens"`
	RetryCount                uint32 `json:"retry_count"`
	LatencyNanos              uint64 `json:"latency_nanos"`
	CostMicros                uint64 `json:"cost_micros"`
	CompactionLossTokens      uint64 `json:"compaction_loss_tokens"`
}

func (m ContextOutcomeMetricsV1) Validate() error {
	if m.InputTokens == 0 || m.LatencyNanos == 0 || m.CacheEligiblePrefixTokens > m.InputTokens || m.DynamicTokens > m.InputTokens || m.CacheReadTokens > m.CacheEligiblePrefixTokens || m.CacheWriteTokens > m.CacheEligiblePrefixTokens || m.CompactionLossTokens > m.InputTokens {
		return fmt.Errorf("%w: context outcome metrics", ErrInvalid)
	}
	return nil
}

type ContextOutcomeFactV1 struct {
	ContractVersion             string                  `json:"contract_version"`
	ID                          string                  `json:"outcome_id"`
	Revision                    uint64                  `json:"revision"`
	Execution                   ExecutionBinding        `json:"execution"`
	FrameRef                    FactRef                 `json:"frame_ref"`
	ManifestRef                 FactRef                 `json:"manifest_ref"`
	RecipeRef                   FactRef                 `json:"recipe_ref"`
	GenerationRef               FactRef                 `json:"generation_ref"`
	ModelAttemptObservationRef  FactRef                 `json:"model_attempt_observation_ref"`
	ModelResponseObservationRef FactRef                 `json:"model_response_observation_ref"`
	ModelSettlementRef          *FactRef                `json:"model_settlement_ref,omitempty"`
	ActualInjectionManifestRef  *FactRef                `json:"actual_injection_manifest_ref,omitempty"`
	ToolActionRefs              []FactRef               `json:"tool_action_refs"`
	UserCorrectionEvidence      []EvidenceRef           `json:"user_correction_evidence"`
	TaskEvidenceRefs            []FactRef               `json:"task_evidence_refs"`
	Metrics                     ContextOutcomeMetricsV1 `json:"metrics"`
	EvaluationPolicyRef         FactRef                 `json:"evaluation_policy_ref"`
	CreatedUnixNano             int64                   `json:"created_unix_nano"`
	ExpiresUnixNano             int64                   `json:"expires_unix_nano"`
}

func (f ContextOutcomeFactV1) Validate() error {
	if ValidateContract(f.ContractVersion) != nil || validateID(f.ID) != nil || f.Revision != 1 || f.Execution.Validate() != nil || f.FrameRef.Validate() != nil || f.ManifestRef.Validate() != nil || f.RecipeRef.Validate() != nil || f.GenerationRef.Validate() != nil || f.ModelAttemptObservationRef.Validate() != nil || f.ModelResponseObservationRef.Validate() != nil || f.Metrics.Validate() != nil || f.EvaluationPolicyRef.Validate() != nil || validateTimes(f.CreatedUnixNano, f.ExpiresUnixNano) != nil {
		return fmt.Errorf("%w: context outcome fact", ErrInvalid)
	}
	for _, ref := range []*FactRef{f.ModelSettlementRef, f.ActualInjectionManifestRef} {
		if ref != nil && ref.Validate() != nil {
			return fmt.Errorf("%w: context outcome optional ref", ErrInvalid)
		}
	}
	if f.ToolActionRefs == nil || f.TaskEvidenceRefs == nil || f.UserCorrectionEvidence == nil || len(f.ToolActionRefs) > MaxOutcomeRefsV1 || len(f.TaskEvidenceRefs) > MaxOutcomeRefsV1 || len(f.UserCorrectionEvidence) > MaxOutcomeRefsV1 || !canonicalFactRefsV1(f.ToolActionRefs) || !canonicalFactRefsV1(f.TaskEvidenceRefs) || !canonicalEvidenceRefsV1(f.UserCorrectionEvidence) {
		return fmt.Errorf("%w: context outcome reference set", ErrConflict)
	}
	return nil
}

func (f ContextOutcomeFactV1) DigestValue() (Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	return DigestJSON(f)
}

type ContextEvaluationDispositionV1 string

const (
	ContextEvaluationBetterV1       ContextEvaluationDispositionV1 = "better"
	ContextEvaluationWorseV1        ContextEvaluationDispositionV1 = "worse"
	ContextEvaluationInconclusiveV1 ContextEvaluationDispositionV1 = "inconclusive"
)

type ContextEvaluationFactV1 struct {
	ContractVersion    string                         `json:"contract_version"`
	ID                 string                         `json:"evaluation_id"`
	Revision           uint64                         `json:"revision"`
	OutcomeRefs        []FactRef                      `json:"outcome_refs"`
	BaselineRecipeRef  FactRef                        `json:"baseline_recipe_ref"`
	CandidateRecipeRef FactRef                        `json:"candidate_recipe_ref"`
	PolicyRef          FactRef                        `json:"policy_ref"`
	QualityScorePPM    uint32                         `json:"quality_score_ppm"`
	EconomicScorePPM   uint32                         `json:"economic_score_ppm"`
	RiskScorePPM       uint32                         `json:"risk_score_ppm"`
	Disposition        ContextEvaluationDispositionV1 `json:"disposition"`
	Evidence           []EvidenceRef                  `json:"evidence"`
	CreatedUnixNano    int64                          `json:"created_unix_nano"`
	ExpiresUnixNano    int64                          `json:"expires_unix_nano"`
}

func (f ContextEvaluationFactV1) Validate() error {
	if ValidateContract(f.ContractVersion) != nil || validateID(f.ID) != nil || f.Revision != 1 || f.BaselineRecipeRef.Validate() != nil || f.CandidateRecipeRef.Validate() != nil || f.PolicyRef.Validate() != nil || f.QualityScorePPM > ScorePPMMaxV1 || f.EconomicScorePPM > ScorePPMMaxV1 || f.RiskScorePPM > ScorePPMMaxV1 || validateTimes(f.CreatedUnixNano, f.ExpiresUnixNano) != nil {
		return fmt.Errorf("%w: context evaluation fact", ErrInvalid)
	}
	if f.Disposition != ContextEvaluationBetterV1 && f.Disposition != ContextEvaluationWorseV1 && f.Disposition != ContextEvaluationInconclusiveV1 {
		return fmt.Errorf("%w: context evaluation disposition", ErrInvalid)
	}
	if len(f.OutcomeRefs) == 0 || len(f.OutcomeRefs) > MaxOutcomeRefsV1 || !canonicalFactRefsV1(f.OutcomeRefs) || len(f.Evidence) == 0 || len(f.Evidence) > MaxOutcomeRefsV1 || !canonicalEvidenceRefsV1(f.Evidence) {
		return fmt.Errorf("%w: context evaluation evidence", ErrConflict)
	}
	return nil
}

func (f ContextEvaluationFactV1) DigestValue() (Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	return DigestJSON(f)
}

type ContextFeedbackStateV1 string

const (
	ContextFeedbackCandidateV1     ContextFeedbackStateV1 = "candidate"
	ContextFeedbackEvaluatedV1     ContextFeedbackStateV1 = "evaluated"
	ContextFeedbackDeclinedV1      ContextFeedbackStateV1 = "declined"
	ContextFeedbackReviewPendingV1 ContextFeedbackStateV1 = "review_pending"
)

type ContextFeedbackCandidateFactV1 struct {
	ContractVersion string                 `json:"contract_version"`
	ID              string                 `json:"feedback_candidate_id"`
	Revision        uint64                 `json:"revision"`
	BaseRecipeRef   FactRef                `json:"base_recipe_ref"`
	OutcomeRefs     []FactRef              `json:"outcome_refs"`
	EvaluationRef   FactRef                `json:"evaluation_ref"`
	ChangeDigest    Digest                 `json:"change_digest"`
	RiskScorePPM    uint32                 `json:"risk_score_ppm"`
	Evidence        []EvidenceRef          `json:"evidence"`
	State           ContextFeedbackStateV1 `json:"state"`
	CreatedUnixNano int64                  `json:"created_unix_nano"`
	ExpiresUnixNano int64                  `json:"expires_unix_nano"`
}

func (f ContextFeedbackCandidateFactV1) Validate() error {
	if ValidateContract(f.ContractVersion) != nil || validateID(f.ID) != nil || f.Revision != 1 || f.BaseRecipeRef.Validate() != nil || f.EvaluationRef.Validate() != nil || f.ChangeDigest.Validate() != nil || f.RiskScorePPM > ScorePPMMaxV1 || validateTimes(f.CreatedUnixNano, f.ExpiresUnixNano) != nil {
		return fmt.Errorf("%w: context feedback candidate", ErrInvalid)
	}
	if f.State != ContextFeedbackCandidateV1 && f.State != ContextFeedbackEvaluatedV1 && f.State != ContextFeedbackDeclinedV1 && f.State != ContextFeedbackReviewPendingV1 {
		return fmt.Errorf("%w: context feedback state", ErrInvalid)
	}
	if len(f.OutcomeRefs) == 0 || len(f.OutcomeRefs) > MaxOutcomeRefsV1 || !canonicalFactRefsV1(f.OutcomeRefs) || len(f.Evidence) == 0 || len(f.Evidence) > MaxOutcomeRefsV1 || !canonicalEvidenceRefsV1(f.Evidence) {
		return fmt.Errorf("%w: context feedback references", ErrConflict)
	}
	return nil
}

func (f ContextFeedbackCandidateFactV1) DigestValue() (Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	return DigestJSON(f)
}

func canonicalEvidenceRefsV1(refs []EvidenceRef) bool {
	previous := ""
	for index, ref := range refs {
		if ref.Validate() != nil {
			return false
		}
		key := ref.ID + "\x00" + string(ref.Digest)
		if index > 0 && strings.Compare(previous, key) >= 0 {
			return false
		}
		previous = key
	}
	return true
}

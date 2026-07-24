package contract

import "fmt"

type FragmentKind string

const (
	FragmentInstruction        FragmentKind = "instruction"
	FragmentConversation       FragmentKind = "conversation"
	FragmentArtifactInline     FragmentKind = "artifact_inline"
	FragmentArtifactReference  FragmentKind = "artifact_reference"
	FragmentToolDeclaration    FragmentKind = "tool_declaration"
	FragmentToolCall           FragmentKind = "tool_call"
	FragmentToolResult         FragmentKind = "tool_result"
	FragmentStateObservation   FragmentKind = "state_observation"
	FragmentMemoryRecall       FragmentKind = "memory_recall"
	FragmentKnowledgeReference FragmentKind = "knowledge_reference"
	FragmentPolicySnapshot     FragmentKind = "policy_snapshot"
	FragmentCompactionSummary  FragmentKind = "compaction_summary"
)

type TrustClass string

const (
	TrustAuthoritativeInstruction TrustClass = "authoritative_instruction"
	TrustUserInput                TrustClass = "user_input"
	TrustRestrictedMaterial       TrustClass = "restricted_material"
	TrustObservation              TrustClass = "observation"
)

type Sensitivity string

const (
	SensitivityPublic       Sensitivity = "public"
	SensitivityInternal     Sensitivity = "internal"
	SensitivityConfidential Sensitivity = "confidential"
	SensitivityRestricted   Sensitivity = "restricted"
)

type MaterializationMode string

const (
	MaterializationInline    MaterializationMode = "inline"
	MaterializationReference MaterializationMode = "reference"
)

type ContextCandidate struct {
	ContractVersion string              `json:"contract_version"`
	ID              string              `json:"candidate_id"`
	Revision        uint64              `json:"revision"`
	Kind            FragmentKind        `json:"kind"`
	Owner           OwnerRef            `json:"owner"`
	Execution       ExecutionBinding    `json:"execution"`
	SourceRef       string              `json:"source_ref"`
	SourceRevision  uint64              `json:"source_revision"`
	Content         ContentRef          `json:"content"`
	Trust           TrustClass          `json:"trust"`
	Sensitivity     Sensitivity         `json:"sensitivity"`
	Mode            MaterializationMode `json:"materialization_mode"`
	Required        bool                `json:"required"`
	TokenEstimate   uint64              `json:"token_estimate"`
	EstimatorDigest Digest              `json:"estimator_digest"`
	CacheStability  uint8               `json:"cache_stability"`
	Evidence        EvidenceRef         `json:"evidence"`
	IdempotencyKey  string              `json:"idempotency_key"`
	CreatedUnixNano int64               `json:"created_unix_nano"`
	ExpiresUnixNano int64               `json:"expires_unix_nano"`
}

func (c ContextCandidate) Validate() error {
	if ValidateContract(c.ContractVersion) != nil || validateID(c.ID) != nil || c.Revision != 1 || !validFragmentKind(c.Kind) {
		return fmt.Errorf("%w: candidate identity", ErrInvalid)
	}
	if c.Owner.Validate() != nil || c.Execution.Validate() != nil || validateID(c.SourceRef) != nil || c.SourceRevision == 0 || c.Content.Validate() != nil {
		return fmt.Errorf("%w: candidate source", ErrInvalid)
	}
	if !validTrust(c.Trust) || !validSensitivity(c.Sensitivity) || (c.Mode != MaterializationInline && c.Mode != MaterializationReference) {
		return fmt.Errorf("%w: candidate policy", ErrInvalid)
	}
	if c.TokenEstimate == 0 || c.EstimatorDigest.Validate() != nil || c.CacheStability > 100 || c.Evidence.Validate() != nil || validateID(c.IdempotencyKey) != nil || validateTimes(c.CreatedUnixNano, c.ExpiresUnixNano) != nil {
		return fmt.Errorf("%w: candidate evidence or lifetime", ErrInvalid)
	}
	return nil
}

func (c ContextCandidate) DigestValue() (Digest, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	return DigestJSON(c)
}

func validFragmentKind(v FragmentKind) bool {
	switch v {
	case FragmentInstruction, FragmentConversation, FragmentArtifactInline, FragmentArtifactReference, FragmentToolDeclaration, FragmentToolCall, FragmentToolResult, FragmentStateObservation, FragmentMemoryRecall, FragmentKnowledgeReference, FragmentPolicySnapshot, FragmentCompactionSummary:
		return true
	default:
		return false
	}
}

func validTrust(v TrustClass) bool {
	return v == TrustAuthoritativeInstruction || v == TrustUserInput || v == TrustRestrictedMaterial || v == TrustObservation
}

func validSensitivity(v Sensitivity) bool {
	return v == SensitivityPublic || v == SensitivityInternal || v == SensitivityConfidential || v == SensitivityRestricted
}

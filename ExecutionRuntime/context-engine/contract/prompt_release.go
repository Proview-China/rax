package contract

import "fmt"

type ContextPromptLifecycleStateV1 string

const (
	ContextPromptDraftV1         ContextPromptLifecycleStateV1 = "draft"
	ContextPromptValidatedV1     ContextPromptLifecycleStateV1 = "validated"
	ContextPromptEvaluatedV1     ContextPromptLifecycleStateV1 = "evaluated"
	ContextPromptReviewPendingV1 ContextPromptLifecycleStateV1 = "review_pending"
	ContextPromptRejectedV1      ContextPromptLifecycleStateV1 = "rejected"
)

type ContextPromptLifecycleFactV1 struct {
	ContractVersion      string                        `json:"contract_version"`
	ID                   string                        `json:"lifecycle_id"`
	Revision             uint64                        `json:"revision"`
	PromptAssetRef       PromptAssetRefV1              `json:"prompt_asset_ref"`
	PreviousLifecycleRef *FactRef                      `json:"previous_lifecycle_ref,omitempty"`
	State                ContextPromptLifecycleStateV1 `json:"state"`
	ValidationReportRef  *FactRef                      `json:"validation_report_ref,omitempty"`
	EvaluationRef        *FactRef                      `json:"evaluation_ref,omitempty"`
	FeedbackCandidateRef *FactRef                      `json:"feedback_candidate_ref,omitempty"`
	ReviewCaseRef        *FactRef                      `json:"review_case_ref,omitempty"`
	Evidence             []EvidenceRef                 `json:"evidence"`
	CreatedUnixNano      int64                         `json:"created_unix_nano"`
	ExpiresUnixNano      int64                         `json:"expires_unix_nano"`
}

func (f ContextPromptLifecycleFactV1) Validate() error {
	if ValidateContract(f.ContractVersion) != nil || validateID(f.ID) != nil || f.Revision != 1 || f.PromptAssetRef.Validate() != nil || validateTimes(f.CreatedUnixNano, f.ExpiresUnixNano) != nil || f.Evidence == nil || len(f.Evidence) > MaxOutcomeRefsV1 || !canonicalEvidenceRefsV1(f.Evidence) {
		return fmt.Errorf("%w: prompt lifecycle fact", ErrInvalid)
	}
	for _, ref := range []*FactRef{f.PreviousLifecycleRef, f.ValidationReportRef, f.EvaluationRef, f.FeedbackCandidateRef, f.ReviewCaseRef} {
		if ref != nil && ref.Validate() != nil {
			return fmt.Errorf("%w: prompt lifecycle reference", ErrInvalid)
		}
	}
	switch f.State {
	case ContextPromptDraftV1:
		if f.PreviousLifecycleRef != nil || f.ValidationReportRef != nil || f.EvaluationRef != nil || f.FeedbackCandidateRef != nil || f.ReviewCaseRef != nil {
			return fmt.Errorf("%w: prompt draft lifecycle presence", ErrConflict)
		}
	case ContextPromptValidatedV1:
		if f.PreviousLifecycleRef == nil || f.ValidationReportRef == nil || f.EvaluationRef != nil || f.FeedbackCandidateRef != nil || f.ReviewCaseRef != nil {
			return fmt.Errorf("%w: prompt validated lifecycle presence", ErrConflict)
		}
	case ContextPromptEvaluatedV1:
		if f.PreviousLifecycleRef == nil || f.ValidationReportRef == nil || f.EvaluationRef == nil || f.ReviewCaseRef != nil {
			return fmt.Errorf("%w: prompt evaluated lifecycle presence", ErrConflict)
		}
	case ContextPromptReviewPendingV1:
		if f.PreviousLifecycleRef == nil || f.ValidationReportRef == nil || f.EvaluationRef == nil || f.ReviewCaseRef == nil {
			return fmt.Errorf("%w: prompt review-pending lifecycle presence", ErrConflict)
		}
	case ContextPromptRejectedV1:
		if f.PreviousLifecycleRef == nil || len(f.Evidence) == 0 {
			return fmt.Errorf("%w: prompt rejected lifecycle presence", ErrConflict)
		}
	default:
		return fmt.Errorf("%w: prompt lifecycle state", ErrInvalid)
	}
	return nil
}

func (f ContextPromptLifecycleFactV1) DigestValue() (Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	return DigestJSON(f)
}

type ContextPromptLifecycleHeadV1 struct {
	PromptAssetRef PromptAssetRefV1 `json:"prompt_asset_ref"`
	LifecycleRef   FactRef          `json:"lifecycle_ref"`
}

func (h ContextPromptLifecycleHeadV1) Validate() error {
	if h.PromptAssetRef.Validate() != nil || h.LifecycleRef.Validate() != nil {
		return fmt.Errorf("%w: prompt lifecycle head", ErrInvalid)
	}
	return nil
}

type ContextPromptProductionActionV1 string

const (
	ContextPromptPublishV1  ContextPromptProductionActionV1 = "publish"
	ContextPromptRollbackV1 ContextPromptProductionActionV1 = "rollback"
	ContextPromptRevokeV1   ContextPromptProductionActionV1 = "revoke"
)

func (a ContextPromptProductionActionV1) Validate() error {
	if a != ContextPromptPublishV1 && a != ContextPromptRollbackV1 && a != ContextPromptRevokeV1 {
		return fmt.Errorf("%w: prompt production action", ErrInvalid)
	}
	return nil
}

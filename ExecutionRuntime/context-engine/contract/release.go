package contract

import "fmt"

type ContextRecipeLifecycleStateV1 string

const (
	ContextRecipeDraftV1         ContextRecipeLifecycleStateV1 = "draft"
	ContextRecipeValidatedV1     ContextRecipeLifecycleStateV1 = "validated"
	ContextRecipeEvaluatedV1     ContextRecipeLifecycleStateV1 = "evaluated"
	ContextRecipeReviewPendingV1 ContextRecipeLifecycleStateV1 = "review_pending"
	ContextRecipeRejectedV1      ContextRecipeLifecycleStateV1 = "rejected"
)

type ContextRecipeLifecycleFactV1 struct {
	ContractVersion      string                        `json:"contract_version"`
	ID                   string                        `json:"lifecycle_id"`
	Revision             uint64                        `json:"revision"`
	RecipeRef            FactRef                       `json:"recipe_ref"`
	PreviousLifecycleRef *FactRef                      `json:"previous_lifecycle_ref,omitempty"`
	State                ContextRecipeLifecycleStateV1 `json:"state"`
	ValidationReportRef  *FactRef                      `json:"validation_report_ref,omitempty"`
	EvaluationRef        *FactRef                      `json:"evaluation_ref,omitempty"`
	FeedbackCandidateRef *FactRef                      `json:"feedback_candidate_ref,omitempty"`
	ReviewCaseRef        *FactRef                      `json:"review_case_ref,omitempty"`
	Evidence             []EvidenceRef                 `json:"evidence"`
	CreatedUnixNano      int64                         `json:"created_unix_nano"`
	ExpiresUnixNano      int64                         `json:"expires_unix_nano"`
}

func (f ContextRecipeLifecycleFactV1) Validate() error {
	if ValidateContract(f.ContractVersion) != nil || validateID(f.ID) != nil || f.Revision != 1 || f.RecipeRef.Validate() != nil || validateTimes(f.CreatedUnixNano, f.ExpiresUnixNano) != nil || f.Evidence == nil || len(f.Evidence) > MaxOutcomeRefsV1 || !canonicalEvidenceRefsV1(f.Evidence) {
		return fmt.Errorf("%w: recipe lifecycle fact", ErrInvalid)
	}
	for _, ref := range []*FactRef{f.PreviousLifecycleRef, f.ValidationReportRef, f.EvaluationRef, f.FeedbackCandidateRef, f.ReviewCaseRef} {
		if ref != nil && ref.Validate() != nil {
			return fmt.Errorf("%w: recipe lifecycle reference", ErrInvalid)
		}
	}
	switch f.State {
	case ContextRecipeDraftV1:
		if f.PreviousLifecycleRef != nil || f.ValidationReportRef != nil || f.EvaluationRef != nil || f.FeedbackCandidateRef != nil || f.ReviewCaseRef != nil {
			return fmt.Errorf("%w: draft lifecycle presence", ErrConflict)
		}
	case ContextRecipeValidatedV1:
		if f.PreviousLifecycleRef == nil || f.ValidationReportRef == nil || f.EvaluationRef != nil || f.FeedbackCandidateRef != nil || f.ReviewCaseRef != nil {
			return fmt.Errorf("%w: validated lifecycle presence", ErrConflict)
		}
	case ContextRecipeEvaluatedV1:
		if f.PreviousLifecycleRef == nil || f.ValidationReportRef == nil || f.EvaluationRef == nil || f.ReviewCaseRef != nil {
			return fmt.Errorf("%w: evaluated lifecycle presence", ErrConflict)
		}
	case ContextRecipeReviewPendingV1:
		if f.PreviousLifecycleRef == nil || f.ValidationReportRef == nil || f.EvaluationRef == nil || f.ReviewCaseRef == nil {
			return fmt.Errorf("%w: review-pending lifecycle presence", ErrConflict)
		}
	case ContextRecipeRejectedV1:
		if f.PreviousLifecycleRef == nil || len(f.Evidence) == 0 {
			return fmt.Errorf("%w: rejected lifecycle presence", ErrConflict)
		}
	default:
		return fmt.Errorf("%w: recipe lifecycle state", ErrInvalid)
	}
	return nil
}

func (f ContextRecipeLifecycleFactV1) DigestValue() (Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	return DigestJSON(f)
}

type ContextRecipeLifecycleHeadV1 struct {
	RecipeRef    FactRef `json:"recipe_ref"`
	LifecycleRef FactRef `json:"lifecycle_ref"`
}

func (h ContextRecipeLifecycleHeadV1) Validate() error {
	if h.RecipeRef.Validate() != nil || h.LifecycleRef.Validate() != nil {
		return fmt.Errorf("%w: recipe lifecycle head", ErrInvalid)
	}
	return nil
}

type ContextRecipeProductionActionV1 string

const (
	ContextRecipePublishV1  ContextRecipeProductionActionV1 = "publish"
	ContextRecipeRollbackV1 ContextRecipeProductionActionV1 = "rollback"
	ContextRecipeRevokeV1   ContextRecipeProductionActionV1 = "revoke"
)

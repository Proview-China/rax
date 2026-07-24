package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type ReviewerContextCurrentResolveRequestV1 struct {
	Subject contract.ReviewerContextSubjectV1 `json:"subject"`
}

func (r ReviewerContextCurrentResolveRequestV1) Validate() error { return r.Subject.Validate() }

type ReviewerContextPublishRequestV1 struct {
	Previous *contract.ReviewerContextEnvelopeRefV1 `json:"previous,omitempty"`
	Value    contract.ReviewerContextEnvelopeV1     `json:"value"`
}

func (r ReviewerContextPublishRequestV1) Validate() error {
	if err := r.Value.Validate(); err != nil {
		return err
	}
	if r.Previous == nil {
		if r.Value.Ref.Revision != 1 {
			return reviewerContextRevisionConflictV1("initial Reviewer Context revision must be one")
		}
		return nil
	}
	if err := r.Previous.Validate(); err != nil {
		return err
	}
	if r.Previous.TenantID != r.Value.Ref.TenantID || r.Previous.ID != r.Value.Ref.ID || r.Value.Ref.Revision != r.Previous.Revision+1 {
		return reviewerContextRevisionConflictV1("Reviewer Context full-ref CAS revision drifted")
	}
	return nil
}

type ReviewerContextPublishReceiptV1 struct {
	Ref     contract.ReviewerContextEnvelopeRefV1 `json:"ref"`
	Created bool                                  `json:"created"`
}

// ReviewerContextCurrentReaderV1 is implemented by the Context Owner adapter.
// Resolve starts a new S1 and never claims recovery of an unknown result;
// current Inspect atomically verifies the subject index equals the expected
// full ref. Historical Inspect depends only on the full ref. Successes are
// immutable deep clones; closed errors are InvalidArgument, NotFound,
// Conflict, PreconditionFailed, Indeterminate and Unavailable.
type ReviewerContextCurrentReaderV1 interface {
	ResolveCurrentReviewerContextV1(context.Context, ReviewerContextCurrentResolveRequestV1) (contract.ReviewerContextEnvelopeRefV1, error)
	InspectCurrentReviewerContextV1(context.Context, contract.ReviewerContextSubjectV1, contract.ReviewerContextEnvelopeRefV1) (contract.ReviewerContextEnvelopeV1, error)
	InspectHistoricalReviewerContextV1(context.Context, contract.ReviewerContextEnvelopeRefV1) (contract.ReviewerContextEnvelopeV1, error)
}

// ReviewerContextPublisherV1 belongs to the Context Owner. Publication and
// current full-ref CAS are one transaction. A lost publish reply is recovered
// only by exact historical Inspect; changed canonical content is Conflict.
type ReviewerContextPublisherV1 interface {
	PublishReviewerContextV1(context.Context, ReviewerContextPublishRequestV1) (ReviewerContextPublishReceiptV1, error)
}

func reviewerContextRevisionConflictV1(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, message)
}

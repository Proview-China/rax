package control

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ReviewEvidenceApplicabilityFactPortV1 is the Runtime Evidence Owner's
// atomic storage seam. It is intentionally narrower than the public Reader:
// Review consumers cannot publish facts or obtain this interface.
type ReviewEvidenceApplicabilityFactPortV1 interface {
	InspectReviewEvidenceApplicabilityCurrentFactV1(context.Context, core.Digest) (ports.ReviewEvidenceApplicabilityCurrentSnapshotV1, error)
	InspectReviewEvidenceApplicabilityProjectionFactV1(context.Context, ports.ReviewEvidenceApplicabilityRefV1) (ports.ReviewEvidenceApplicabilityProjectionV1, error)
	PublishReviewEvidenceApplicabilityFactV1(context.Context, ports.PublishReviewEvidenceApplicabilityRequestV1, ports.ReviewEvidenceApplicabilityPublishReceiptV1) (ports.ReviewEvidenceApplicabilityPublishReceiptV1, error)
	InspectReviewEvidenceApplicabilityPublishFactV1(context.Context, string) (ports.ReviewEvidenceApplicabilityPublishReceiptV1, error)
}

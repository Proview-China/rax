package contract

import (
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// DeliveryV1 describes how a Review waits. It does not select or authorize a
// reviewer and therefore remains independent from RouteV1.
type DeliveryV1 string

const (
	DeliveryInlineV1   DeliveryV1 = "inline"
	DeliveryDetachedV1 DeliveryV1 = "detached"
)

// ExactResourceRefV1 is a Review-owned nominal reference for immutable Review
// inputs such as rubrics and result bundles. It grants no read or write power.
type ExactResourceRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r ExactResourceRefV1) Validate() error {
	if invalidID(r.ID) || r.Revision == 0 || r.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review exact resource ref is incomplete")
	}
	return nil
}

// ReviewRequestV1 is a sealed, create-once admission candidate. Admission
// creates the Target and Case; this object is not a Verdict, authority grant,
// operation permit or evidence fact.
type ReviewRequestV1 struct {
	FactIdentityV1
	IdempotencyKey string             `json:"idempotency_key"`
	CaseID         string             `json:"case_id"`
	TargetID       string             `json:"target_id"`
	TargetRevision core.Revision      `json:"target_revision"`
	TargetDigest   core.Digest        `json:"target_digest"`
	Delivery       DeliveryV1         `json:"delivery"`
	Profile        ProfileV1          `json:"profile"`
	Rubric         ExactResourceRefV1 `json:"rubric"`
	// ResultBundle is the legacy V1 Request-to-Bundle association. V2 reverses
	// this edge: ReviewResultBundleV2 binds the already-sealed Request exactly,
	// avoiding an impossible Request-digest <-> Bundle-digest cycle.
	ResultBundle             *ExactResourceRefV1                `json:"result_bundle,omitempty"`
	RequesterID              string                             `json:"requester_id"`
	RequesterAuthority       runtimeports.AuthorityBindingRefV2 `json:"requester_authority"`
	AttachmentEvidence       []runtimeports.ReviewEvidenceRefV2 `json:"attachment_evidence"`
	AttachmentEvidenceDigest core.Digest                        `json:"attachment_evidence_digest"`
	RequestedVerdictTTL      int64                              `json:"requested_verdict_ttl_nanos"`
	BudgetDigest             core.Digest                        `json:"budget_digest"`
	ExpiresUnixNano          int64                              `json:"expires_unix_nano"`
}

func (r ReviewRequestV1) digestValue() ReviewRequestV1 { r.Digest = ""; return r }

func (r ReviewRequestV1) validateShape() error {
	if err := r.FactIdentityV1.ValidateShape(); err != nil {
		return err
	}
	if r.Revision != 1 || invalidID(r.IdempotencyKey) || invalidID(r.CaseID) || invalidID(r.TargetID) || r.TargetRevision == 0 || r.TargetDigest.Validate() != nil || invalidID(r.RequesterID) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review request identity is incomplete")
	}
	if r.Delivery != DeliveryInlineV1 && r.Delivery != DeliveryDetachedV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "review request delivery is unsupported")
	}
	if err := ValidateProfileV1(r.Profile); err != nil {
		return err
	}
	if err := r.Rubric.Validate(); err != nil {
		return err
	}
	if r.ResultBundle != nil {
		if err := r.ResultBundle.Validate(); err != nil {
			return err
		}
	}
	if err := r.RequesterAuthority.Validate(); err != nil {
		return err
	}
	if len(r.AttachmentEvidence) > MaxListItemsV1 || !sort.SliceIsSorted(r.AttachmentEvidence, func(i, j int) bool { return r.AttachmentEvidence[i].Ref < r.AttachmentEvidence[j].Ref }) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "review request evidence must be bounded and sorted")
	}
	for _, evidence := range r.AttachmentEvidence {
		if err := evidence.Validate(); err != nil {
			return err
		}
	}
	digest, err := ComputeReviewEvidenceDigestV1(r.AttachmentEvidence)
	if err != nil {
		return err
	}
	if digest != r.AttachmentEvidenceDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "review request evidence digest drifted")
	}
	if r.RequestedVerdictTTL <= 0 || r.RequestedVerdictTTL > int64(30*24*time.Hour) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "review request verdict TTL is outside its bound")
	}
	if err := r.BudgetDigest.Validate(); err != nil {
		return err
	}
	return ValidateExpires(r.CreatedUnixNano, r.ExpiresUnixNano)
}

func SealReviewRequestV1(r ReviewRequestV1) (ReviewRequestV1, error) {
	r.ContractVersion = ContractVersionV1
	r.Digest = ""
	sort.Slice(r.AttachmentEvidence, func(i, j int) bool { return r.AttachmentEvidence[i].Ref < r.AttachmentEvidence[j].Ref })
	if err := r.validateShape(); err != nil {
		return ReviewRequestV1{}, err
	}
	digest, err := seal("ReviewRequestV1", r.digestValue())
	if err != nil {
		return ReviewRequestV1{}, err
	}
	r.Digest = digest
	return r, r.Validate()
}

func (r ReviewRequestV1) Validate() error {
	if err := r.validateShape(); err != nil {
		return err
	}
	return validateSealed("ReviewRequestV1", r.digestValue(), r.Digest)
}

func (r ReviewRequestV1) ValidateTarget(target TargetSnapshotV1, now time.Time) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if err := target.Validate(); err != nil {
		return err
	}
	if r.TenantID != target.TenantID || r.TargetID != target.ID || r.TargetRevision != target.Revision || r.TargetDigest != target.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "review request target exact identity drifted")
	}
	if err := ValidateNow(now, r.CreatedUnixNano, r.ExpiresUnixNano); err != nil {
		return err
	}
	if now.UnixNano() >= target.ExpiresUnixNano || r.ExpiresUnixNano > target.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "review request exceeds the target currentness window")
	}
	return nil
}

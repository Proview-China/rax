package contract

import (
	"sort"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// EvidenceAttachmentV1 is a Review-owned immutable association between an
// exact Case/Target and Evidence Owner references. It is not an Evidence fact,
// does not grant authority, and cannot by itself satisfy a Verdict condition.
type EvidenceAttachmentV1 struct {
	FactIdentityV1
	IdempotencyKey   string                             `json:"idempotency_key"`
	Case             ExactResourceRefV1                 `json:"case"`
	Target           ExactResourceRefV1                 `json:"target"`
	SubmitterID      string                             `json:"submitter_id"`
	Evidence         []runtimeports.ReviewEvidenceRefV2 `json:"evidence"`
	EvidenceDigest   core.Digest                        `json:"evidence_digest"`
	ObservedUnixNano int64                              `json:"observed_unix_nano"`
	ExpiresUnixNano  int64                              `json:"expires_unix_nano"`
}

type evidenceAttachmentDigestItemV1 struct {
	Ref            string                        `json:"ref"`
	Classification runtimeports.NamespacedNameV2 `json:"classification"`
	Digest         core.Digest                   `json:"digest"`
}

func evidenceAttachmentSortKeyV1(value runtimeports.ReviewEvidenceRefV2) string {
	return value.Ref
}

func ComputeEvidenceAttachmentSetDigestV1(values []runtimeports.ReviewEvidenceRefV2) (core.Digest, error) {
	if len(values) == 0 || len(values) > MaxListItemsV1 {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "review Evidence Attachment refs are empty or exceed their bound")
	}
	items := make([]evidenceAttachmentDigestItemV1, len(values))
	previous := ""
	for i, value := range values {
		if err := value.Validate(); err != nil {
			return "", err
		}
		key := evidenceAttachmentSortKeyV1(value)
		if i > 0 && key <= previous {
			return "", core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "review Evidence Attachment refs must be unique and canonically sorted")
		}
		previous = key
		items[i] = evidenceAttachmentDigestItemV1{Ref: value.Ref, Classification: value.Classification, Digest: value.Digest}
	}
	return seal("EvidenceAttachmentSetV1", items)
}

func (v EvidenceAttachmentV1) digestValue() EvidenceAttachmentV1 {
	v.Digest = ""
	return v
}

func (v EvidenceAttachmentV1) validateShape() error {
	if err := v.FactIdentityV1.ValidateShape(); err != nil {
		return err
	}
	if v.Revision != 1 || invalidID(v.IdempotencyKey) || invalidID(v.Case.ID) || v.Case.Revision == 0 || v.Case.Digest.Validate() != nil || invalidID(v.Target.ID) || v.Target.Revision == 0 || v.Target.Digest.Validate() != nil || invalidID(v.SubmitterID) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review Evidence Attachment identity or exact refs are incomplete")
	}
	if v.CreatedUnixNano != v.ObservedUnixNano || v.UpdatedUnixNano != v.ObservedUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "review Evidence Attachment timestamps drifted")
	}
	if err := ValidateExpires(v.ObservedUnixNano, v.ExpiresUnixNano); err != nil {
		return err
	}
	digest, err := ComputeEvidenceAttachmentSetDigestV1(v.Evidence)
	if err != nil {
		return err
	}
	if digest != v.EvidenceDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "review Evidence Attachment set digest drifted")
	}
	return nil
}

func SealEvidenceAttachmentV1(v EvidenceAttachmentV1) (EvidenceAttachmentV1, error) {
	v.ContractVersion = ContractVersionV1
	v.Digest = ""
	v.Evidence = append([]runtimeports.ReviewEvidenceRefV2(nil), v.Evidence...)
	sort.Slice(v.Evidence, func(i, j int) bool {
		return evidenceAttachmentSortKeyV1(v.Evidence[i]) < evidenceAttachmentSortKeyV1(v.Evidence[j])
	})
	digest, err := ComputeEvidenceAttachmentSetDigestV1(v.Evidence)
	if err != nil {
		return EvidenceAttachmentV1{}, err
	}
	v.EvidenceDigest = digest
	if err := v.validateShape(); err != nil {
		return EvidenceAttachmentV1{}, err
	}
	v.Digest, err = seal("EvidenceAttachmentV1", v.digestValue())
	if err != nil {
		return EvidenceAttachmentV1{}, err
	}
	return v, v.Validate()
}

func (v EvidenceAttachmentV1) Validate() error {
	if err := v.validateShape(); err != nil {
		return err
	}
	return validateSealed("EvidenceAttachmentV1", v.digestValue(), v.Digest)
}

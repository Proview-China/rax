package contract

import (
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	ReviewerContextEnvelopeContractV1 = "praxis.review/reviewer-context-envelope-v1"
	reviewerContextCanonicalDomainV1  = "praxis.review.reviewer-context-envelope"
	MaxReviewerContextMaterialBytesV1 = 1 << 20
)

type ReviewerContextMaterialKindV1 string

const (
	ReviewerContextOriginalIntentV1      ReviewerContextMaterialKindV1 = "original_intent"
	ReviewerContextRequirementV1         ReviewerContextMaterialKindV1 = "requirement"
	ReviewerContextAcceptanceCriterionV1 ReviewerContextMaterialKindV1 = "acceptance_criterion"
	ReviewerContextStableRuleV1          ReviewerContextMaterialKindV1 = "stable_rule"
	ReviewerContextConfirmedDecisionV1   ReviewerContextMaterialKindV1 = "confirmed_decision"
	ReviewerContextCandidateV1           ReviewerContextMaterialKindV1 = "candidate"
	ReviewerContextArtifactV1            ReviewerContextMaterialKindV1 = "artifact"
	ReviewerContextDiffV1                ReviewerContextMaterialKindV1 = "diff"
	ReviewerContextEvidenceV1            ReviewerContextMaterialKindV1 = "evidence"
	ReviewerContextKnownRiskV1           ReviewerContextMaterialKindV1 = "known_risk"
	ReviewerContextLimitationV1          ReviewerContextMaterialKindV1 = "limitation"
	ReviewerContextDeltaV1               ReviewerContextMaterialKindV1 = "context_delta"
)

type ReviewerContextMaterialTrustV1 string

const (
	ReviewerContextInstructionV1 ReviewerContextMaterialTrustV1 = "instruction"
	ReviewerContextObservationV1 ReviewerContextMaterialTrustV1 = "observation"
)

type ReviewerContextSourceRefV1 struct {
	Owner           runtimeports.NamespacedNameV2 `json:"owner"`
	ID              string                        `json:"id"`
	Revision        core.Revision                 `json:"revision"`
	Digest          core.Digest                   `json:"digest"`
	ExpiresUnixNano int64                         `json:"expires_unix_nano"`
}

func (r ReviewerContextSourceRefV1) Validate() error {
	if err := runtimeports.ValidateNamespacedNameV2(r.Owner); err != nil {
		return err
	}
	if invalidID(r.ID) || r.Revision == 0 || r.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Reviewer Context source exact ref is incomplete")
	}
	return r.Digest.Validate()
}

type ReviewerContextMaterialV1 struct {
	Kind          ReviewerContextMaterialKindV1  `json:"kind"`
	Source        ReviewerContextSourceRefV1     `json:"source"`
	MediaType     string                         `json:"media_type"`
	Content       string                         `json:"content"`
	ContentDigest core.Digest                    `json:"content_digest"`
	Trust         ReviewerContextMaterialTrustV1 `json:"trust"`
}

func (m ReviewerContextMaterialV1) Validate() error {
	if err := m.Source.Validate(); err != nil {
		return err
	}
	if !validReviewerContextMaterialKindV1(m.Kind) || !utf8.ValidString(m.Content) || len(m.Content) == 0 || len(m.Content) > MaxReviewerContextMaterialBytesV1 || strings.TrimSpace(m.MediaType) == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "Reviewer Context material is invalid or unbounded")
	}
	wantTrust := ReviewerContextObservationV1
	switch m.Kind {
	case ReviewerContextOriginalIntentV1, ReviewerContextRequirementV1, ReviewerContextAcceptanceCriterionV1, ReviewerContextStableRuleV1, ReviewerContextConfirmedDecisionV1:
		wantTrust = ReviewerContextInstructionV1
	}
	if m.Trust != wantTrust {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceTrustInvalid, "Reviewer Context material trust class drifted")
	}
	if m.ContentDigest != core.DigestBytes([]byte(m.Content)) {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Reviewer Context material content digest drifted")
	}
	return nil
}

func validReviewerContextMaterialKindV1(kind ReviewerContextMaterialKindV1) bool {
	switch kind {
	case ReviewerContextOriginalIntentV1, ReviewerContextRequirementV1, ReviewerContextAcceptanceCriterionV1, ReviewerContextStableRuleV1, ReviewerContextConfirmedDecisionV1, ReviewerContextCandidateV1, ReviewerContextArtifactV1, ReviewerContextDiffV1, ReviewerContextEvidenceV1, ReviewerContextKnownRiskV1, ReviewerContextLimitationV1, ReviewerContextDeltaV1:
		return true
	default:
		return false
	}
}

type ReviewerContextSubjectV1 struct {
	TenantID           core.TenantID            `json:"tenant_id"`
	Case               ExactResourceRefV1       `json:"case"`
	Round              ExactResourceRefV1       `json:"round"`
	Assignment         ExactResourceRefV1       `json:"assignment"`
	Target             ExactResourceRefV1       `json:"target"`
	Rubric             ExactResourceRefV1       `json:"rubric"`
	ContextFrameDigest core.Digest              `json:"context_frame_digest"`
	OutputSchema       runtimeports.SchemaRefV2 `json:"output_schema"`
}

func (s ReviewerContextSubjectV1) Validate() error {
	if strings.TrimSpace(string(s.TenantID)) == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Reviewer Context subject tenant is required")
	}
	for _, ref := range []ExactResourceRefV1{s.Case, s.Round, s.Assignment, s.Target, s.Rubric} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if err := s.ContextFrameDigest.Validate(); err != nil {
		return err
	}
	return s.OutputSchema.Validate()
}

type ReviewerContextEnvelopeStateV1 string

const (
	ReviewerContextEnvelopeActiveV1     ReviewerContextEnvelopeStateV1 = "active"
	ReviewerContextEnvelopeRevokedV1    ReviewerContextEnvelopeStateV1 = "revoked"
	ReviewerContextEnvelopeSupersededV1 ReviewerContextEnvelopeStateV1 = "superseded"
)

type ReviewerContextEnvelopeRefV1 struct {
	TenantID core.TenantID `json:"tenant_id"`
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r ReviewerContextEnvelopeRefV1) Validate() error {
	if strings.TrimSpace(string(r.TenantID)) == "" || invalidID(r.ID) || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Reviewer Context envelope exact ref is incomplete")
	}
	return r.Digest.Validate()
}

type ReviewerContextEnvelopeV1 struct {
	ContractVersion         string                         `json:"contract_version"`
	Ref                     ReviewerContextEnvelopeRefV1   `json:"ref"`
	Subject                 ReviewerContextSubjectV1       `json:"subject"`
	Materials               []ReviewerContextMaterialV1    `json:"materials"`
	AllowedReadCapabilities []string                       `json:"allowed_read_capabilities"`
	ReadOnly                bool                           `json:"read_only"`
	WorkIdentityRemoved     bool                           `json:"work_identity_removed"`
	State                   ReviewerContextEnvelopeStateV1 `json:"state"`
	Current                 bool                           `json:"current"`
	CheckedUnixNano         int64                          `json:"checked_unix_nano"`
	ExpiresUnixNano         int64                          `json:"expires_unix_nano"`
	ProjectionDigest        core.Digest                    `json:"projection_digest"`
}

func (v ReviewerContextEnvelopeV1) Clone() ReviewerContextEnvelopeV1 {
	v.Materials = append([]ReviewerContextMaterialV1(nil), v.Materials...)
	v.AllowedReadCapabilities = append([]string(nil), v.AllowedReadCapabilities...)
	return v
}

func (v ReviewerContextEnvelopeV1) digestValue() ReviewerContextEnvelopeV1 {
	v = v.Clone()
	v.Ref.Digest = ""
	v.ProjectionDigest = ""
	return v
}

func (v ReviewerContextEnvelopeV1) Validate() error {
	if v.ContractVersion != ReviewerContextEnvelopeContractV1 || v.Ref.Validate() != nil || v.Subject.Validate() != nil || v.Ref.TenantID != v.Subject.TenantID || !v.ReadOnly || !v.WorkIdentityRemoved || v.CheckedUnixNano <= 0 || v.CheckedUnixNano >= v.ExpiresUnixNano || v.Ref.Digest != v.ProjectionDigest {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Reviewer Context envelope is incomplete")
	}
	if (v.State == ReviewerContextEnvelopeActiveV1) != v.Current {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "Reviewer Context state/current truth table drifted")
	}
	if v.State != ReviewerContextEnvelopeActiveV1 && v.State != ReviewerContextEnvelopeRevokedV1 && v.State != ReviewerContextEnvelopeSupersededV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "Reviewer Context state is unsupported")
	}
	if len(v.Materials) == 0 || len(v.Materials) > MaxListItemsV1 || !sort.SliceIsSorted(v.Materials, func(i, j int) bool {
		return reviewerContextMaterialKeyV1(v.Materials[i]) < reviewerContextMaterialKeyV1(v.Materials[j])
	}) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "Reviewer Context materials must be bounded and sorted")
	}
	counts := make(map[ReviewerContextMaterialKindV1]int)
	minimum := v.ExpiresUnixNano
	for i, material := range v.Materials {
		if err := material.Validate(); err != nil {
			return err
		}
		if i > 0 && reviewerContextMaterialKeyV1(v.Materials[i-1]) == reviewerContextMaterialKeyV1(material) {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "Reviewer Context material exact source is duplicated")
		}
		counts[material.Kind]++
		if material.Source.ExpiresUnixNano < minimum {
			minimum = material.Source.ExpiresUnixNano
		}
	}
	for _, required := range []ReviewerContextMaterialKindV1{ReviewerContextOriginalIntentV1, ReviewerContextRequirementV1, ReviewerContextAcceptanceCriterionV1, ReviewerContextStableRuleV1, ReviewerContextCandidateV1, ReviewerContextEvidenceV1, ReviewerContextKnownRiskV1} {
		if counts[required] == 0 {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "Reviewer Context lacks a required material class")
		}
	}
	if counts[ReviewerContextOriginalIntentV1] != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "Reviewer Context requires exactly one original intent")
	}
	if v.ExpiresUnixNano != minimum {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Reviewer Context expiry is not the exact source minimum")
	}
	if len(v.AllowedReadCapabilities) == 0 || len(v.AllowedReadCapabilities) > MaxListItemsV1 || !sort.StringsAreSorted(v.AllowedReadCapabilities) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "Reviewer Context read capabilities must be bounded and sorted")
	}
	for i, capability := range v.AllowedReadCapabilities {
		lower := strings.ToLower(strings.TrimSpace(capability))
		if lower == "" || strings.Contains(lower, "write") || strings.Contains(lower, "dispatch") || strings.Contains(lower, "commit") || strings.Contains(lower, "spawn") || strings.Contains(lower, "execute") || (i > 0 && v.AllowedReadCapabilities[i-1] == capability) {
			return core.NewError(core.ErrorForbidden, core.ReasonUnknownCapability, "Reviewer Context contains a forbidden or duplicate capability")
		}
	}
	wantID, err := DeriveReviewerContextEnvelopeIDV1(v.Subject)
	if err != nil || v.Ref.ID != wantID {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "Reviewer Context stable envelope ID drifted")
	}
	digest, err := v.DigestV1()
	if err != nil || digest != v.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Reviewer Context envelope digest drifted")
	}
	return nil
}

func (v ReviewerContextEnvelopeV1) ValidateCurrent(expected ReviewerContextEnvelopeRefV1, subject ReviewerContextSubjectV1, now time.Time) error {
	if err := v.Validate(); err != nil {
		return err
	}
	if expected != v.Ref || subject != v.Subject {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Reviewer Context exact ref or subject drifted")
	}
	if now.IsZero() || now.UnixNano() < v.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Reviewer Context current clock regressed")
	}
	if !v.Current || v.State != ReviewerContextEnvelopeActiveV1 || now.UnixNano() >= v.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Reviewer Context envelope is inactive or expired")
	}
	return nil
}

func (v ReviewerContextEnvelopeV1) DigestV1() (core.Digest, error) {
	return core.CanonicalJSONDigest(reviewerContextCanonicalDomainV1, ReviewerContextEnvelopeContractV1, "ReviewerContextEnvelopeV1", v.digestValue())
}

func SealReviewerContextEnvelopeV1(v ReviewerContextEnvelopeV1) (ReviewerContextEnvelopeV1, error) {
	v = v.Clone()
	v.ContractVersion = ReviewerContextEnvelopeContractV1
	sort.Slice(v.Materials, func(i, j int) bool {
		return reviewerContextMaterialKeyV1(v.Materials[i]) < reviewerContextMaterialKeyV1(v.Materials[j])
	})
	sort.Strings(v.AllowedReadCapabilities)
	id, err := DeriveReviewerContextEnvelopeIDV1(v.Subject)
	if err != nil {
		return ReviewerContextEnvelopeV1{}, err
	}
	if v.Ref.ID == "" {
		v.Ref.ID = id
	}
	if v.Ref.TenantID == "" {
		v.Ref.TenantID = v.Subject.TenantID
	}
	v.Ref.Digest, v.ProjectionDigest = "", ""
	digest, err := v.DigestV1()
	if err != nil {
		return ReviewerContextEnvelopeV1{}, err
	}
	v.Ref.Digest, v.ProjectionDigest = digest, digest
	return v, v.Validate()
}

func DeriveReviewerContextEnvelopeIDV1(subject ReviewerContextSubjectV1) (string, error) {
	if err := subject.Validate(); err != nil {
		return "", err
	}
	digest, err := core.CanonicalJSONDigest(reviewerContextCanonicalDomainV1, ReviewerContextEnvelopeContractV1, "ReviewerContextEnvelopeIdentityV1", subject)
	if err != nil {
		return "", err
	}
	return "reviewer-context-" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func reviewerContextMaterialKeyV1(material ReviewerContextMaterialV1) string {
	return string(material.Kind) + "\x00" + string(material.Source.Owner) + "\x00" + material.Source.ID + "\x00" + string(material.Source.Digest)
}

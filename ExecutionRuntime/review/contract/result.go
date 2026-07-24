package contract

import (
	"sort"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type ResultClaimV1 struct {
	ID        string                             `json:"id"`
	Statement string                             `json:"statement"`
	Artifact  ExactResourceRefV1                 `json:"artifact"`
	Anchor    string                             `json:"anchor"`
	Evidence  []runtimeports.ReviewEvidenceRefV2 `json:"evidence"`
}

func (c ResultClaimV1) validateShape() error {
	if invalidID(c.ID) || invalidText(c.Statement) || invalidText(c.Anchor) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "result claim is incomplete")
	}
	if err := c.Artifact.Validate(); err != nil {
		return err
	}
	if len(c.Evidence) == 0 || len(c.Evidence) > MaxListItemsV1 || !sort.SliceIsSorted(c.Evidence, func(i, j int) bool { return c.Evidence[i].Ref < c.Evidence[j].Ref }) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "result claim evidence must be non-empty, sorted and bounded")
	}
	for i, evidence := range c.Evidence {
		if err := evidence.Validate(); err != nil {
			return err
		}
		if i > 0 && c.Evidence[i-1].Ref == evidence.Ref {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "result claim evidence is duplicated")
		}
	}
	return nil
}

type ReviewResultBundleV1 struct {
	FactIdentityV1
	OriginalTaskDigest    core.Digest          `json:"original_task_digest"`
	AcceptanceDigest      core.Digest          `json:"acceptance_digest"`
	Artifacts             []ExactResourceRefV1 `json:"artifacts"`
	Claims                []ResultClaimV1      `json:"claims"`
	EnvironmentDigest     core.Digest          `json:"environment_digest"`
	ValidationScopeDigest core.Digest          `json:"validation_scope_digest"`
	Limitations           []string             `json:"limitations"`
	Uncovered             []string             `json:"uncovered"`
	EvidenceSetDigest     core.Digest          `json:"evidence_set_digest"`
	ExpiresUnixNano       int64                `json:"expires_unix_nano"`
}

func (v ReviewResultBundleV1) digestValue() ReviewResultBundleV1 { v.Digest = ""; return v }
func (v ReviewResultBundleV1) validateShape() error {
	if err := v.FactIdentityV1.ValidateShape(); err != nil {
		return err
	}
	if v.Revision != 1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "result bundle must be create-once")
	}
	for _, digest := range []core.Digest{v.OriginalTaskDigest, v.AcceptanceDigest, v.EnvironmentDigest, v.ValidationScopeDigest, v.EvidenceSetDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if len(v.Artifacts) == 0 || len(v.Artifacts) > MaxListItemsV1 || !sort.SliceIsSorted(v.Artifacts, func(i, j int) bool { return v.Artifacts[i].ID < v.Artifacts[j].ID }) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "result bundle artifacts must be non-empty, sorted and bounded")
	}
	artifactByID := make(map[string]ExactResourceRefV1, len(v.Artifacts))
	for _, artifact := range v.Artifacts {
		if err := artifact.Validate(); err != nil {
			return err
		}
		if _, ok := artifactByID[artifact.ID]; ok {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "result bundle artifact is duplicated")
		}
		artifactByID[artifact.ID] = artifact
	}
	if len(v.Claims) == 0 || len(v.Claims) > MaxListItemsV1 || !sort.SliceIsSorted(v.Claims, func(i, j int) bool { return v.Claims[i].ID < v.Claims[j].ID }) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "result bundle claims must be non-empty, sorted and bounded")
	}
	evidenceByRef := map[string]runtimeports.ReviewEvidenceRefV2{}
	for i, claim := range v.Claims {
		if err := claim.validateShape(); err != nil {
			return err
		}
		if i > 0 && v.Claims[i-1].ID == claim.ID {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "result bundle claim is duplicated")
		}
		artifact, ok := artifactByID[claim.Artifact.ID]
		if !ok || artifact != claim.Artifact {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "result claim artifact exact ref drifted")
		}
		for _, evidence := range claim.Evidence {
			if old, ok := evidenceByRef[evidence.Ref]; ok && old != evidence {
				return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "result bundle evidence ref changed content")
			}
			evidenceByRef[evidence.Ref] = evidence
		}
	}
	evidence := make([]runtimeports.ReviewEvidenceRefV2, 0, len(evidenceByRef))
	for _, value := range evidenceByRef {
		evidence = append(evidence, value)
	}
	digest, err := ComputeReviewEvidenceDigestV1(evidence)
	if err != nil {
		return err
	}
	if digest != v.EvidenceSetDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "result bundle evidence set digest drifted")
	}
	for _, values := range [][]string{v.Limitations, v.Uncovered} {
		if len(values) > MaxListItemsV1 || !sort.StringsAreSorted(values) {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "result bundle limitation lists must be sorted and bounded")
		}
		for i, value := range values {
			if invalidText(value) || (i > 0 && values[i-1] == value) {
				return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "result bundle limitation item is invalid or duplicated")
			}
		}
	}
	return ValidateExpires(v.CreatedUnixNano, v.ExpiresUnixNano)
}
func SealReviewResultBundleV1(v ReviewResultBundleV1) (ReviewResultBundleV1, error) {
	if v.Limitations == nil {
		v.Limitations = []string{}
	}
	if v.Uncovered == nil {
		v.Uncovered = []string{}
	}
	v.ContractVersion = ContractVersionV1
	v.Digest = ""
	sort.Slice(v.Artifacts, func(i, j int) bool { return v.Artifacts[i].ID < v.Artifacts[j].ID })
	sort.Slice(v.Claims, func(i, j int) bool { return v.Claims[i].ID < v.Claims[j].ID })
	for i := range v.Claims {
		sort.Slice(v.Claims[i].Evidence, func(a, b int) bool { return v.Claims[i].Evidence[a].Ref < v.Claims[i].Evidence[b].Ref })
	}
	sort.Strings(v.Limitations)
	sort.Strings(v.Uncovered)
	if err := v.validateShape(); err != nil {
		return ReviewResultBundleV1{}, err
	}
	digest, err := seal("ReviewResultBundleV1", v.digestValue())
	if err != nil {
		return ReviewResultBundleV1{}, err
	}
	v.Digest = digest
	return v, v.Validate()
}
func (v ReviewResultBundleV1) Validate() error {
	if err := v.validateShape(); err != nil {
		return err
	}
	return validateSealed("ReviewResultBundleV1", v.digestValue(), v.Digest)
}

type BehaviorFeedbackCandidateV1 struct {
	FactIdentityV1
	Case            ExactResourceRefV1                       `json:"case"`
	Target          ExactResourceRefV1                       `json:"target"`
	Verdict         ExactResourceRefV1                       `json:"verdict"`
	Findings        []ExactResourceRefV1                     `json:"findings"`
	Policy          runtimeports.ReviewPolicyBindingRefV2    `json:"policy"`
	ReviewerID      string                                   `json:"reviewer_id"`
	ReviewerBinding runtimeports.ReviewComponentBindingRefV2 `json:"reviewer_binding"`
	BehaviorClass   string                                   `json:"behavior_class"`
	SignalDigest    core.Digest                              `json:"signal_digest"`
	Evidence        []runtimeports.ReviewEvidenceRefV2       `json:"evidence"`
	EvidenceDigest  core.Digest                              `json:"evidence_digest"`
	ExpiresUnixNano int64                                    `json:"expires_unix_nano"`
}

func (v BehaviorFeedbackCandidateV1) digestValue() BehaviorFeedbackCandidateV1 {
	v.Digest = ""
	return v
}
func (v BehaviorFeedbackCandidateV1) validateShape() error {
	if err := v.FactIdentityV1.ValidateShape(); err != nil {
		return err
	}
	if v.Revision != 1 || invalidID(v.ReviewerID) || invalidText(v.BehaviorClass) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "behavior feedback candidate is incomplete")
	}
	for _, ref := range []ExactResourceRefV1{v.Case, v.Target, v.Verdict} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if len(v.Findings) > MaxListItemsV1 || !sort.SliceIsSorted(v.Findings, func(i, j int) bool { return v.Findings[i].ID < v.Findings[j].ID }) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "behavior feedback findings must be sorted and bounded")
	}
	for i, ref := range v.Findings {
		if err := ref.Validate(); err != nil {
			return err
		}
		if i > 0 && v.Findings[i-1].ID == ref.ID {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "behavior feedback finding is duplicated")
		}
	}
	if err := v.Policy.Validate(); err != nil {
		return err
	}
	if err := v.ReviewerBinding.Validate(); err != nil {
		return err
	}
	if err := v.SignalDigest.Validate(); err != nil {
		return err
	}
	if len(v.Evidence) == 0 || len(v.Evidence) > MaxListItemsV1 || !sort.SliceIsSorted(v.Evidence, func(i, j int) bool { return v.Evidence[i].Ref < v.Evidence[j].Ref }) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "behavior feedback evidence is invalid")
	}
	digest, err := ComputeReviewEvidenceDigestV1(v.Evidence)
	if err != nil {
		return err
	}
	if digest != v.EvidenceDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "behavior feedback evidence digest drifted")
	}
	return ValidateExpires(v.CreatedUnixNano, v.ExpiresUnixNano)
}
func SealBehaviorFeedbackCandidateV1(v BehaviorFeedbackCandidateV1) (BehaviorFeedbackCandidateV1, error) {
	v.ContractVersion = ContractVersionV1
	v.Digest = ""
	sort.Slice(v.Findings, func(i, j int) bool { return v.Findings[i].ID < v.Findings[j].ID })
	sort.Slice(v.Evidence, func(i, j int) bool { return v.Evidence[i].Ref < v.Evidence[j].Ref })
	if err := v.validateShape(); err != nil {
		return BehaviorFeedbackCandidateV1{}, err
	}
	digest, err := seal("BehaviorFeedbackCandidateV1", v.digestValue())
	if err != nil {
		return BehaviorFeedbackCandidateV1{}, err
	}
	v.Digest = digest
	return v, v.Validate()
}
func (v BehaviorFeedbackCandidateV1) Validate() error {
	if err := v.validateShape(); err != nil {
		return err
	}
	return validateSealed("BehaviorFeedbackCandidateV1", v.digestValue(), v.Digest)
}

package contract

import (
	"bytes"
	"reflect"
	"sort"
	"strconv"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type ReviewResultArtifactBindingV2 struct {
	Source  runtimeports.ReviewArtifactExactSourceRefV2 `json:"source"`
	Anchors []runtimeports.ReviewArtifactLocatorV2      `json:"anchors"`
}

func (v ReviewResultArtifactBindingV2) Clone() ReviewResultArtifactBindingV2 {
	v.Anchors = cloneResultLocatorsV2(v.Anchors)
	return v
}

func (v ReviewResultArtifactBindingV2) Validate() error {
	if err := v.Source.Validate(); err != nil {
		return err
	}
	if len(v.Anchors) == 0 || len(v.Anchors) > MaxListItemsV1 || !sort.SliceIsSorted(v.Anchors, func(i, j int) bool {
		return resultLocatorKeyV2(v.Anchors[i]) < resultLocatorKeyV2(v.Anchors[j])
	}) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "result artifact anchors must be non-empty, bounded and sorted")
	}
	for i, anchor := range v.Anchors {
		if err := anchor.Validate(); err != nil {
			return err
		}
		if i > 0 && resultLocatorKeyV2(v.Anchors[i-1]) == resultLocatorKeyV2(anchor) {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "result artifact anchor is duplicated")
		}
	}
	return nil
}

type ReviewResultClaimV2 struct {
	ID        string                                      `json:"id"`
	Statement string                                      `json:"statement"`
	Artifact  runtimeports.ReviewArtifactExactSourceRefV2 `json:"artifact"`
	Anchor    runtimeports.ReviewArtifactLocatorV2        `json:"anchor"`
	Evidence  []runtimeports.ReviewEvidenceRefV2          `json:"evidence"`
}

func (v ReviewResultClaimV2) Clone() ReviewResultClaimV2 {
	v.Anchor.Payload.Inline = bytes.Clone(v.Anchor.Payload.Inline)
	v.Evidence = append([]runtimeports.ReviewEvidenceRefV2(nil), v.Evidence...)
	return v
}

func (v ReviewResultClaimV2) Validate() error {
	if invalidID(v.ID) || invalidText(v.Statement) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "result claim is incomplete")
	}
	if err := v.Artifact.Validate(); err != nil {
		return err
	}
	if err := v.Anchor.Validate(); err != nil {
		return err
	}
	if len(v.Evidence) == 0 || len(v.Evidence) > MaxListItemsV1 || !sort.SliceIsSorted(v.Evidence, func(i, j int) bool {
		return resultEvidenceKeyV2(v.Evidence[i]) < resultEvidenceKeyV2(v.Evidence[j])
	}) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "result claim evidence must be non-empty, bounded and sorted")
	}
	for i, evidence := range v.Evidence {
		if err := evidence.Validate(); err != nil {
			return err
		}
		if i > 0 && resultEvidenceKeyV2(v.Evidence[i-1]) == resultEvidenceKeyV2(evidence) {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "result claim evidence is duplicated")
		}
	}
	return nil
}

type ReviewResultBundleV2 struct {
	FactIdentityV1
	Request                ExactResourceRefV1                           `json:"request"`
	Target                 ExactResourceRefV1                           `json:"target"`
	OriginalIntent         ReviewerContextSourceRefV1                   `json:"original_intent"`
	AcceptanceCriteria     []ReviewerContextSourceRefV1                 `json:"acceptance_criteria"`
	Artifacts              []ReviewResultArtifactBindingV2              `json:"artifacts"`
	Claims                 []ReviewResultClaimV2                        `json:"claims"`
	Environment            runtimeports.ReviewEnvironmentExactRefV2     `json:"environment"`
	ReviewerContext        ReviewerContextEnvelopeRefV1                 `json:"reviewer_context"`
	ReviewerContextSources []ReviewerContextSourceRefV1                 `json:"reviewer_context_sources"`
	ValidationScope        runtimeports.ReviewValidationScopeExactRefV2 `json:"validation_scope"`
	Limitations            []string                                     `json:"limitations"`
	Uncovered              []string                                     `json:"uncovered"`
	EvidenceSetDigest      core.Digest                                  `json:"evidence_set_digest"`
	ExpiresUnixNano        int64                                        `json:"expires_unix_nano"`
}

func (v ReviewResultBundleV2) Clone() ReviewResultBundleV2 {
	v.AcceptanceCriteria = append([]ReviewerContextSourceRefV1(nil), v.AcceptanceCriteria...)
	v.Artifacts = append([]ReviewResultArtifactBindingV2(nil), v.Artifacts...)
	for i := range v.Artifacts {
		v.Artifacts[i] = v.Artifacts[i].Clone()
	}
	v.Claims = append([]ReviewResultClaimV2(nil), v.Claims...)
	for i := range v.Claims {
		v.Claims[i] = v.Claims[i].Clone()
	}
	v.ReviewerContextSources = append([]ReviewerContextSourceRefV1(nil), v.ReviewerContextSources...)
	v.Limitations = append([]string(nil), v.Limitations...)
	v.Uncovered = append([]string(nil), v.Uncovered...)
	return v
}

func (v ReviewResultBundleV2) digestValue() ReviewResultBundleV2 {
	v = v.Clone()
	v.Digest = ""
	return v
}

func (v ReviewResultBundleV2) Validate() error {
	if err := v.FactIdentityV1.ValidateShape(); err != nil {
		return err
	}
	if v.Revision != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "result bundle V2 is create-once")
	}
	for _, ref := range []ExactResourceRefV1{v.Request, v.Target} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if err := v.OriginalIntent.Validate(); err != nil {
		return err
	}
	if err := validateContextSourceListV2(v.AcceptanceCriteria, false); err != nil {
		return err
	}
	if len(v.Artifacts) == 0 || len(v.Artifacts) > MaxListItemsV1 || !sort.SliceIsSorted(v.Artifacts, func(i, j int) bool {
		return resultArtifactKeyV2(v.Artifacts[i].Source) < resultArtifactKeyV2(v.Artifacts[j].Source)
	}) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "result bundle V2 artifacts must be non-empty, bounded and sorted")
	}
	artifacts := make(map[string]ReviewResultArtifactBindingV2, len(v.Artifacts))
	reachableAnchors := make(map[string]bool)
	for i, artifact := range v.Artifacts {
		if err := artifact.Validate(); err != nil {
			return err
		}
		key := resultArtifactKeyV2(artifact.Source)
		if i > 0 && resultArtifactKeyV2(v.Artifacts[i-1].Source) == key {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "result bundle V2 artifact is duplicated")
		}
		artifacts[key] = artifact
		for _, anchor := range artifact.Anchors {
			reachableAnchors[key+"\x00"+resultLocatorKeyV2(anchor)] = false
		}
	}
	if len(v.Claims) == 0 || len(v.Claims) > MaxListItemsV1 || !sort.SliceIsSorted(v.Claims, func(i, j int) bool { return v.Claims[i].ID < v.Claims[j].ID }) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "result bundle V2 claims must be non-empty, bounded and sorted")
	}
	evidenceByKey := map[string]runtimeports.ReviewEvidenceRefV2{}
	for i, claim := range v.Claims {
		if err := claim.Validate(); err != nil {
			return err
		}
		if i > 0 && v.Claims[i-1].ID == claim.ID {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "result bundle V2 claim is duplicated")
		}
		artifactKey := resultArtifactKeyV2(claim.Artifact)
		artifact, ok := artifacts[artifactKey]
		if !ok || artifact.Source != claim.Artifact {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "result claim artifact exact ref drifted")
		}
		anchorKey := artifactKey + "\x00" + resultLocatorKeyV2(claim.Anchor)
		found := false
		for _, anchor := range artifact.Anchors {
			if reflect.DeepEqual(anchor, claim.Anchor) {
				found = true
				break
			}
		}
		if !found {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "result claim anchor is not declared by its artifact")
		}
		reachableAnchors[anchorKey] = true
		for _, evidence := range claim.Evidence {
			key := resultEvidenceKeyV2(evidence)
			if previous, ok := evidenceByKey[key]; ok && previous != evidence {
				return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "result evidence exact ref drifted")
			}
			evidenceByKey[key] = evidence
		}
	}
	for _, reached := range reachableAnchors {
		if !reached {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "result artifact contains an unreachable anchor")
		}
	}
	if err := v.Environment.Validate(); err != nil {
		return err
	}
	if err := v.ReviewerContext.Validate(); err != nil {
		return err
	}
	if err := validateContextSourceListV2(v.ReviewerContextSources, false); err != nil {
		return err
	}
	if !containsContextSourceV2(v.ReviewerContextSources, v.OriginalIntent) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "original intent is absent from Reviewer Context sources")
	}
	for _, criterion := range v.AcceptanceCriteria {
		if !containsContextSourceV2(v.ReviewerContextSources, criterion) {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "acceptance criterion is absent from Reviewer Context sources")
		}
	}
	if err := v.ValidationScope.Validate(); err != nil {
		return err
	}
	for _, values := range [][]string{v.Limitations, v.Uncovered} {
		if len(values) > MaxListItemsV1 || !sort.StringsAreSorted(values) {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "result disclosure lists must be bounded and sorted")
		}
		for i, value := range values {
			if invalidText(value) || (i > 0 && values[i-1] == value) {
				return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "result disclosure item is invalid or duplicated")
			}
		}
	}
	evidence := make([]runtimeports.ReviewEvidenceRefV2, 0, len(evidenceByKey))
	for _, value := range evidenceByKey {
		evidence = append(evidence, value)
	}
	digest, err := ComputeReviewEvidenceDigestV1(evidence)
	if err != nil {
		return err
	}
	if digest != v.EvidenceSetDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "result bundle V2 evidence set digest drifted")
	}
	if err := ValidateExpires(v.CreatedUnixNano, v.ExpiresUnixNano); err != nil {
		return err
	}
	if err := v.Digest.Validate(); err != nil {
		return err
	}
	expected, err := core.CanonicalJSONDigest("praxis.review.result-bundle/body/v2", "praxis.review/result-bundle-v2", "ReviewResultBundleV2", v.digestValue())
	if err != nil {
		return err
	}
	if expected != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "result bundle V2 digest does not bind exact content")
	}
	return nil
}

func SealReviewResultBundleV2(v ReviewResultBundleV2) (ReviewResultBundleV2, error) {
	v.ContractVersion = ContractVersionV1
	v.Digest = ""
	v = v.Clone()
	sort.Slice(v.AcceptanceCriteria, func(i, j int) bool {
		return contextSourceKeyV2(v.AcceptanceCriteria[i]) < contextSourceKeyV2(v.AcceptanceCriteria[j])
	})
	sort.Slice(v.Artifacts, func(i, j int) bool {
		return resultArtifactKeyV2(v.Artifacts[i].Source) < resultArtifactKeyV2(v.Artifacts[j].Source)
	})
	for i := range v.Artifacts {
		sort.Slice(v.Artifacts[i].Anchors, func(a, b int) bool {
			return resultLocatorKeyV2(v.Artifacts[i].Anchors[a]) < resultLocatorKeyV2(v.Artifacts[i].Anchors[b])
		})
	}
	sort.Slice(v.Claims, func(i, j int) bool { return v.Claims[i].ID < v.Claims[j].ID })
	for i := range v.Claims {
		sort.Slice(v.Claims[i].Evidence, func(a, b int) bool {
			return resultEvidenceKeyV2(v.Claims[i].Evidence[a]) < resultEvidenceKeyV2(v.Claims[i].Evidence[b])
		})
	}
	sort.Slice(v.ReviewerContextSources, func(i, j int) bool {
		return contextSourceKeyV2(v.ReviewerContextSources[i]) < contextSourceKeyV2(v.ReviewerContextSources[j])
	})
	sort.Strings(v.Limitations)
	sort.Strings(v.Uncovered)
	digest, err := core.CanonicalJSONDigest("praxis.review.result-bundle/body/v2", "praxis.review/result-bundle-v2", "ReviewResultBundleV2", v.digestValue())
	if err != nil {
		return ReviewResultBundleV2{}, err
	}
	v.Digest = digest
	return v, v.Validate()
}

func resultArtifactKeyV2(v runtimeports.ReviewArtifactExactSourceRefV2) string {
	return string(v.Kind) + "\x00" + string(v.Owner.Binding.ComponentID) + "\x00" + v.ID + "\x00" + strconv.FormatUint(uint64(v.Revision), 10) + "\x00" + string(v.Digest)
}
func resultLocatorKeyV2(v runtimeports.ReviewArtifactLocatorV2) string {
	return string(v.Kind) + "\x00" + v.Schema.Key() + "\x00" + string(v.LocatorDigest)
}
func resultEvidenceKeyV2(v runtimeports.ReviewEvidenceRefV2) string {
	return v.Ref + "\x00" + string(v.Classification) + "\x00" + string(v.Digest)
}
func contextSourceKeyV2(v ReviewerContextSourceRefV1) string {
	return string(v.Owner) + "\x00" + v.ID + "\x00" + strconv.FormatUint(uint64(v.Revision), 10) + "\x00" + string(v.Digest)
}
func cloneResultLocatorsV2(values []runtimeports.ReviewArtifactLocatorV2) []runtimeports.ReviewArtifactLocatorV2 {
	out := append([]runtimeports.ReviewArtifactLocatorV2(nil), values...)
	for i := range out {
		out[i].Payload.Inline = bytes.Clone(out[i].Payload.Inline)
	}
	return out
}
func validateContextSourceListV2(values []ReviewerContextSourceRefV1, allowEmpty bool) error {
	if (!allowEmpty && len(values) == 0) || len(values) > MaxListItemsV1 || !sort.SliceIsSorted(values, func(i, j int) bool { return contextSourceKeyV2(values[i]) < contextSourceKeyV2(values[j]) }) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "Reviewer Context source list is empty, unbounded or unsorted")
	}
	for i, value := range values {
		if err := value.Validate(); err != nil {
			return err
		}
		if i > 0 && contextSourceKeyV2(values[i-1]) == contextSourceKeyV2(value) {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "Reviewer Context source is duplicated")
		}
	}
	return nil
}
func containsContextSourceV2(values []ReviewerContextSourceRefV1, expected ReviewerContextSourceRefV1) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

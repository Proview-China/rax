package contract

import (
	"fmt"
	"mime"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
)

const (
	PromptUpstreamProvenanceContractVersionV1 = "praxis.context.prompt-upstream-provenance/v1"
	MaxPromptUpstreamArtifactsV1              = 16
	MaxPromptUpstreamRangesV1                 = 64
	MaxPromptTransformStepsV1                 = 64
	MaxPromptGeneratedContentV1               = 64
	MaxPromptUpstreamInputBytesV1             = uint64(32 * 1024 * 1024)
)

var promptCommitPatternV1 = regexp.MustCompile(`^[0-9a-f]{40}$`)

type PromptUpstreamSourceClassV1 string

const (
	PromptSourceOfficialCodingAgentV1   PromptUpstreamSourceClassV1 = "official_coding_agent_prompt"
	PromptSourceOfficialSDKPresetV1     PromptUpstreamSourceClassV1 = "official_agent_sdk_preset_reference"
	PromptSourceOfficialModelTemplateV1 PromptUpstreamSourceClassV1 = "official_model_chat_template"
	PromptSourceOfficialPolicyPrefixV1  PromptUpstreamSourceClassV1 = "official_policy_prefix"
	PromptSourceComparativeOpenSourceV1 PromptUpstreamSourceClassV1 = "comparative_open_source"
)

func (v PromptUpstreamSourceClassV1) Validate() error {
	switch v {
	case PromptSourceOfficialCodingAgentV1, PromptSourceOfficialSDKPresetV1, PromptSourceOfficialModelTemplateV1, PromptSourceOfficialPolicyPrefixV1, PromptSourceComparativeOpenSourceV1:
		return nil
	default:
		return fmt.Errorf("%w: prompt upstream source class", ErrInvalid)
	}
}

type PromptUpstreamRangeV1 struct {
	Start  uint64 `json:"start"`
	End    uint64 `json:"end"`
	Digest Digest `json:"digest"`
}

func (v PromptUpstreamRangeV1) Validate(byteLength uint64) error {
	if v.Start >= v.End || v.End > byteLength || v.Digest.Validate() != nil {
		return fmt.Errorf("%w: prompt upstream range", ErrInvalid)
	}
	return nil
}

type PromptUpstreamArtifactV1 struct {
	ID              string                  `json:"id"`
	Repository      string                  `json:"repository"`
	Commit          string                  `json:"commit"`
	Path            string                  `json:"path"`
	MediaType       string                  `json:"media_type"`
	ByteLength      uint64                  `json:"byte_length"`
	ContentDigest   Digest                  `json:"content_digest"`
	ExtractedRanges []PromptUpstreamRangeV1 `json:"extracted_ranges"`
}

func (v PromptUpstreamArtifactV1) Validate() error {
	if validateID(v.ID) != nil || validatePromptRepositoryV1(v.Repository) != nil || !promptCommitPatternV1.MatchString(v.Commit) || validatePromptPathV1(v.Path) != nil || validatePromptMediaTypeV1(v.MediaType) != nil || v.ByteLength == 0 || v.ContentDigest.Validate() != nil || len(v.ExtractedRanges) == 0 || len(v.ExtractedRanges) > MaxPromptUpstreamRangesV1 {
		return fmt.Errorf("%w: prompt upstream artifact", ErrInvalid)
	}
	previousEnd := uint64(0)
	for index, current := range v.ExtractedRanges {
		if err := current.Validate(v.ByteLength); err != nil {
			return err
		}
		if index > 0 && current.Start < previousEnd {
			return fmt.Errorf("%w: prompt upstream ranges overlap or are not canonical", ErrConflict)
		}
		previousEnd = current.End
	}
	return nil
}

type PromptUpstreamLicenseV1 struct {
	SPDXID         string        `json:"spdx_id"`
	Repository     string        `json:"repository"`
	Commit         string        `json:"commit"`
	Path           string        `json:"path"`
	ByteLength     uint64        `json:"byte_length"`
	ContentDigest  Digest        `json:"content_digest"`
	ReviewEvidence []EvidenceRef `json:"review_evidence"`
}

func (v PromptUpstreamLicenseV1) Validate() error {
	if validateID(v.SPDXID) != nil || validatePromptRepositoryV1(v.Repository) != nil || !promptCommitPatternV1.MatchString(v.Commit) || validatePromptPathV1(v.Path) != nil || v.ByteLength == 0 || v.ContentDigest.Validate() != nil || len(v.ReviewEvidence) == 0 || len(v.ReviewEvidence) > MaxOutcomeRefsV1 || !canonicalEvidenceRefsV1(v.ReviewEvidence) {
		return fmt.Errorf("%w: prompt upstream license", ErrInvalid)
	}
	return nil
}

type PromptTransformKindV1 string

const (
	PromptTransformExtractV1      PromptTransformKindV1 = "extract"
	PromptTransformRemoveClaimsV1 PromptTransformKindV1 = "remove_host_claims"
	PromptTransformParameterizeV1 PromptTransformKindV1 = "parameterize"
	PromptTransformSplitClosureV1 PromptTransformKindV1 = "split_closure"
	PromptTransformCanonicalizeV1 PromptTransformKindV1 = "canonicalize"
)

func (v PromptTransformKindV1) Validate() error {
	switch v {
	case PromptTransformExtractV1, PromptTransformRemoveClaimsV1, PromptTransformParameterizeV1, PromptTransformSplitClosureV1, PromptTransformCanonicalizeV1:
		return nil
	default:
		return fmt.Errorf("%w: prompt transform kind", ErrInvalid)
	}
}

type PromptTransformStepV1 struct {
	ID           string                `json:"id"`
	Revision     uint64                `json:"revision"`
	Kind         PromptTransformKindV1 `json:"kind"`
	InputDigest  Digest                `json:"input_digest"`
	RulesDigest  Digest                `json:"rules_digest"`
	ToolDigest   Digest                `json:"tool_digest"`
	OutputDigest Digest                `json:"output_digest"`
}

func (v PromptTransformStepV1) Validate() error {
	if validateID(v.ID) != nil || v.Revision == 0 || v.Kind.Validate() != nil || v.InputDigest.Validate() != nil || v.RulesDigest.Validate() != nil || v.ToolDigest.Validate() != nil || v.OutputDigest.Validate() != nil {
		return fmt.Errorf("%w: prompt transform step", ErrInvalid)
	}
	return nil
}

type PromptClosureManifestV1 struct {
	Stable           []ContentRef `json:"stable"`
	SemiStable       []ContentRef `json:"semi_stable"`
	DynamicTemplate  []ContentRef `json:"dynamic_template"`
	StableDigest     Digest       `json:"stable_digest"`
	SemiStableDigest Digest       `json:"semi_stable_digest"`
	DynamicDigest    Digest       `json:"dynamic_digest"`
	ClosureDigest    Digest       `json:"closure_digest"`
}

func (v PromptClosureManifestV1) Validate(generated []ContentRef) error {
	if v.StableDigest.Validate() != nil || v.SemiStableDigest.Validate() != nil || v.DynamicDigest.Validate() != nil || v.ClosureDigest.Validate() != nil || !canonicalContentRefsV1(v.Stable) || !canonicalContentRefsV1(v.SemiStable) || !canonicalContentRefsV1(v.DynamicTemplate) {
		return fmt.Errorf("%w: prompt closure manifest", ErrInvalid)
	}
	stableDigest, _ := digestPromptClosureRegionV1("stable", v.Stable)
	semiDigest, _ := digestPromptClosureRegionV1("semi_stable", v.SemiStable)
	dynamicDigest, _ := digestPromptClosureRegionV1("dynamic_template", v.DynamicTemplate)
	if v.StableDigest != stableDigest || v.SemiStableDigest != semiDigest || v.DynamicDigest != dynamicDigest {
		return fmt.Errorf("%w: prompt closure region digest", ErrConflict)
	}
	union := make([]ContentRef, 0, len(v.Stable)+len(v.SemiStable)+len(v.DynamicTemplate))
	union = append(union, v.Stable...)
	union = append(union, v.SemiStable...)
	union = append(union, v.DynamicTemplate...)
	sortContentRefsV1(union)
	if !canonicalContentRefsV1(union) || !equalContentRefsV1(union, generated) {
		return fmt.Errorf("%w: prompt closure exact set", ErrConflict)
	}
	closureDigest, _ := digestPromptClosureV1(v.StableDigest, v.SemiStableDigest, v.DynamicDigest)
	if v.ClosureDigest != closureDigest {
		return fmt.Errorf("%w: prompt closure digest", ErrConflict)
	}
	return nil
}

type PromptUpstreamProvenanceV1 struct {
	ContractVersion    string                      `json:"contract_version"`
	ID                 string                      `json:"id"`
	Revision           uint64                      `json:"revision"`
	SourceClass        PromptUpstreamSourceClassV1 `json:"source_class"`
	SourceProduct      string                      `json:"source_product"`
	Artifacts          []PromptUpstreamArtifactV1  `json:"artifacts"`
	License            PromptUpstreamLicenseV1     `json:"license"`
	SourceSetDigest    Digest                      `json:"source_set_digest"`
	TransformChain     []PromptTransformStepV1     `json:"transform_chain"`
	GeneratedContent   []ContentRef                `json:"generated_content"`
	GeneratedSetDigest Digest                      `json:"generated_set_digest"`
	Closure            PromptClosureManifestV1     `json:"closure"`
	Evidence           []EvidenceRef               `json:"evidence"`
	CreatedUnixNano    int64                       `json:"created_unix_nano"`
	ExpiresUnixNano    int64                       `json:"expires_unix_nano"`
	ProvenanceDigest   Digest                      `json:"provenance_digest"`
}

type PromptUpstreamProvenanceRefV1 struct {
	ID       string `json:"id"`
	Revision uint64 `json:"revision"`
	Digest   Digest `json:"digest"`
}

func (v PromptUpstreamProvenanceRefV1) Validate() error {
	if validateID(v.ID) != nil || v.Revision == 0 || v.Digest.Validate() != nil {
		return fmt.Errorf("%w: prompt upstream provenance reference", ErrInvalid)
	}
	return nil
}

func (v PromptUpstreamProvenanceRefV1) FactRefV1() FactRef {
	return FactRef{ID: v.ID, Revision: v.Revision, Digest: v.Digest}
}

func (v PromptUpstreamProvenanceV1) Validate() error {
	if v.ContractVersion != PromptUpstreamProvenanceContractVersionV1 || validateID(v.ID) != nil || v.Revision == 0 || v.SourceClass.Validate() != nil || validateID(v.SourceProduct) != nil || len(v.Artifacts) == 0 || len(v.Artifacts) > MaxPromptUpstreamArtifactsV1 || v.License.Validate() != nil || v.SourceSetDigest.Validate() != nil || len(v.TransformChain) == 0 || len(v.TransformChain) > MaxPromptTransformStepsV1 || len(v.GeneratedContent) > MaxPromptGeneratedContentV1 || v.GeneratedSetDigest.Validate() != nil || len(v.Evidence) == 0 || len(v.Evidence) > MaxOutcomeRefsV1 || !canonicalEvidenceRefsV1(v.Evidence) || validateTimes(v.CreatedUnixNano, v.ExpiresUnixNano) != nil || v.ProvenanceDigest.Validate() != nil {
		return fmt.Errorf("%w: prompt upstream provenance", ErrInvalid)
	}
	rangeCount := 0
	previousArtifactID := ""
	for index, artifact := range v.Artifacts {
		if err := artifact.Validate(); err != nil {
			return err
		}
		if index > 0 && previousArtifactID >= artifact.ID {
			return fmt.Errorf("%w: prompt upstream artifacts not canonical", ErrConflict)
		}
		previousArtifactID = artifact.ID
		rangeCount += len(artifact.ExtractedRanges)
	}
	if rangeCount > MaxPromptUpstreamRangesV1 {
		return fmt.Errorf("%w: prompt upstream range limit", ErrLimitExceeded)
	}
	if !canonicalContentRefsV1(v.GeneratedContent) {
		return fmt.Errorf("%w: prompt generated content not canonical", ErrConflict)
	}
	if v.SourceClass != PromptSourceOfficialSDKPresetV1 && len(v.GeneratedContent) == 0 {
		return fmt.Errorf("%w: prompt source requires generated content", ErrInvalid)
	}
	sourceDigest, _ := digestPromptSourceSetV1(v.Artifacts)
	generatedDigest, _ := digestPromptGeneratedSetV1(v.GeneratedContent)
	if v.SourceSetDigest != sourceDigest || v.GeneratedSetDigest != generatedDigest {
		return fmt.Errorf("%w: prompt provenance set digest", ErrConflict)
	}
	previous := v.SourceSetDigest
	seenSteps := make(map[string]struct{}, len(v.TransformChain))
	for _, step := range v.TransformChain {
		if err := step.Validate(); err != nil {
			return err
		}
		key := fmt.Sprintf("%s/%d", step.ID, step.Revision)
		if _, exists := seenSteps[key]; exists {
			return fmt.Errorf("%w: duplicate prompt transform step", ErrConflict)
		}
		seenSteps[key] = struct{}{}
		if step.InputDigest != previous {
			return fmt.Errorf("%w: prompt transform chain", ErrConflict)
		}
		previous = step.OutputDigest
	}
	if previous != v.GeneratedSetDigest {
		return fmt.Errorf("%w: prompt transform terminal digest", ErrConflict)
	}
	if err := v.Closure.Validate(v.GeneratedContent); err != nil {
		return err
	}
	want, _ := v.digestValue()
	if want != v.ProvenanceDigest {
		return fmt.Errorf("%w: prompt provenance digest", ErrConflict)
	}
	return nil
}

func (v PromptUpstreamProvenanceV1) digestValue() (Digest, error) {
	copy := v
	copy.ProvenanceDigest = ""
	return DigestJSON(copy)
}

func (v PromptUpstreamProvenanceV1) RefV1() (PromptUpstreamProvenanceRefV1, error) {
	if err := v.Validate(); err != nil {
		return PromptUpstreamProvenanceRefV1{}, err
	}
	return PromptUpstreamProvenanceRefV1{ID: v.ID, Revision: v.Revision, Digest: v.ProvenanceDigest}, nil
}

func SealPromptUpstreamProvenanceV1(v PromptUpstreamProvenanceV1) (PromptUpstreamProvenanceV1, error) {
	v.ContractVersion = PromptUpstreamProvenanceContractVersionV1
	v.Artifacts = clonePromptUpstreamArtifactsV1(v.Artifacts)
	sort.Slice(v.Artifacts, func(i, j int) bool { return v.Artifacts[i].ID < v.Artifacts[j].ID })
	for index := range v.Artifacts {
		sort.Slice(v.Artifacts[index].ExtractedRanges, func(i, j int) bool {
			left, right := v.Artifacts[index].ExtractedRanges[i], v.Artifacts[index].ExtractedRanges[j]
			if left.Start != right.Start {
				return left.Start < right.Start
			}
			return left.End < right.End
		})
	}
	v.License.ReviewEvidence = canonicalPromptEvidenceRefsV1(v.License.ReviewEvidence)
	v.GeneratedContent = append([]ContentRef(nil), v.GeneratedContent...)
	sortContentRefsV1(v.GeneratedContent)
	v.Closure.Stable = append([]ContentRef(nil), v.Closure.Stable...)
	v.Closure.SemiStable = append([]ContentRef(nil), v.Closure.SemiStable...)
	v.Closure.DynamicTemplate = append([]ContentRef(nil), v.Closure.DynamicTemplate...)
	sortContentRefsV1(v.Closure.Stable)
	sortContentRefsV1(v.Closure.SemiStable)
	sortContentRefsV1(v.Closure.DynamicTemplate)
	v.Evidence = canonicalPromptEvidenceRefsV1(v.Evidence)
	v.SourceSetDigest, _ = digestPromptSourceSetV1(v.Artifacts)
	v.GeneratedSetDigest, _ = digestPromptGeneratedSetV1(v.GeneratedContent)
	v.Closure.StableDigest, _ = digestPromptClosureRegionV1("stable", v.Closure.Stable)
	v.Closure.SemiStableDigest, _ = digestPromptClosureRegionV1("semi_stable", v.Closure.SemiStable)
	v.Closure.DynamicDigest, _ = digestPromptClosureRegionV1("dynamic_template", v.Closure.DynamicTemplate)
	v.Closure.ClosureDigest, _ = digestPromptClosureV1(v.Closure.StableDigest, v.Closure.SemiStableDigest, v.Closure.DynamicDigest)
	if len(v.TransformChain) > 0 {
		v.TransformChain = append([]PromptTransformStepV1(nil), v.TransformChain...)
		v.TransformChain[0].InputDigest = v.SourceSetDigest
		v.TransformChain[len(v.TransformChain)-1].OutputDigest = v.GeneratedSetDigest
		for index := 1; index < len(v.TransformChain); index++ {
			v.TransformChain[index].InputDigest = v.TransformChain[index-1].OutputDigest
		}
	}
	v.ProvenanceDigest = ""
	v.ProvenanceDigest, _ = v.digestValue()
	return v, v.Validate()
}

func digestPromptSourceSetV1(v []PromptUpstreamArtifactV1) (Digest, error) {
	return DigestJSON(struct {
		Domain    string                     `json:"domain"`
		Version   string                     `json:"version"`
		Artifacts []PromptUpstreamArtifactV1 `json:"artifacts"`
	}{Domain: "praxis.context.prompt-upstream-source-set", Version: "v1", Artifacts: v})
}

func digestPromptGeneratedSetV1(v []ContentRef) (Digest, error) {
	return DigestJSON(struct {
		Domain  string       `json:"domain"`
		Version string       `json:"version"`
		Refs    []ContentRef `json:"refs"`
	}{Domain: "praxis.context.prompt-generated-set", Version: "v1", Refs: v})
}

func digestPromptClosureRegionV1(region string, v []ContentRef) (Digest, error) {
	return DigestJSON(struct {
		Domain  string       `json:"domain"`
		Version string       `json:"version"`
		Region  string       `json:"region"`
		Refs    []ContentRef `json:"refs"`
	}{Domain: "praxis.context.prompt-closure-region", Version: "v1", Region: region, Refs: v})
}

func digestPromptClosureV1(stable, semi, dynamic Digest) (Digest, error) {
	return DigestJSON(struct {
		Domain  string `json:"domain"`
		Version string `json:"version"`
		Stable  Digest `json:"stable"`
		Semi    Digest `json:"semi_stable"`
		Dynamic Digest `json:"dynamic"`
	}{Domain: "praxis.context.prompt-closure", Version: "v1", Stable: stable, Semi: semi, Dynamic: dynamic})
}

func canonicalContentRefsV1(v []ContentRef) bool {
	previous := ""
	previousRef := ""
	for index, current := range v {
		if current.Validate() != nil {
			return false
		}
		key := fmt.Sprintf("%s\x00%020d\x00%s", current.Ref, current.Length, current.Digest)
		if index > 0 && (previous >= key || previousRef == current.Ref) {
			return false
		}
		previous = key
		previousRef = current.Ref
	}
	return true
}

func sortContentRefsV1(v []ContentRef) {
	sort.Slice(v, func(i, j int) bool {
		if v[i].Ref != v[j].Ref {
			return v[i].Ref < v[j].Ref
		}
		if v[i].Length != v[j].Length {
			return v[i].Length < v[j].Length
		}
		return v[i].Digest < v[j].Digest
	})
}

func equalContentRefsV1(left, right []ContentRef) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func clonePromptUpstreamArtifactsV1(v []PromptUpstreamArtifactV1) []PromptUpstreamArtifactV1 {
	result := append([]PromptUpstreamArtifactV1(nil), v...)
	for index := range result {
		result[index].ExtractedRanges = append([]PromptUpstreamRangeV1(nil), result[index].ExtractedRanges...)
	}
	return result
}

func validatePromptRepositoryV1(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%w: prompt upstream repository", ErrInvalid)
	}
	return nil
}

func validatePromptPathV1(value string) error {
	if value == "" || strings.HasPrefix(value, "/") || path.Clean(value) != value || value == "." || strings.HasPrefix(value, "../") {
		return fmt.Errorf("%w: prompt upstream path", ErrInvalid)
	}
	return nil
}

func validatePromptMediaTypeV1(value string) error {
	if strings.TrimSpace(value) != value || value == "" {
		return fmt.Errorf("%w: prompt upstream media type", ErrInvalid)
	}
	if _, _, err := mime.ParseMediaType(value); err != nil {
		return fmt.Errorf("%w: prompt upstream media type", ErrInvalid)
	}
	return nil
}

type PromptUpstreamArtifactBytesV1 struct {
	ArtifactID string `json:"artifact_id"`
	Bytes      []byte `json:"bytes"`
}

type PromptGeneratedContentBytesV1 struct {
	Ref   ContentRef `json:"ref"`
	Bytes []byte     `json:"bytes"`
}

type VerifyPromptUpstreamProvenanceRequestV1 struct {
	Provenance      PromptUpstreamProvenanceV1      `json:"provenance"`
	ArtifactBytes   []PromptUpstreamArtifactBytesV1 `json:"artifact_bytes"`
	LicenseBytes    []byte                          `json:"license_bytes"`
	GeneratedBytes  []PromptGeneratedContentBytesV1 `json:"generated_bytes"`
	CheckedUnixNano int64                           `json:"checked_unix_nano"`
	MaxInputBytes   uint64                          `json:"max_input_bytes"`
	RequestDigest   Digest                          `json:"request_digest"`
}

func (v VerifyPromptUpstreamProvenanceRequestV1) ValidateShapeV1() error {
	if v.Provenance.Validate() != nil || v.CheckedUnixNano <= 0 || v.MaxInputBytes == 0 || v.MaxInputBytes > MaxPromptUpstreamInputBytesV1 || v.RequestDigest.Validate() != nil || len(v.ArtifactBytes) != len(v.Provenance.Artifacts) || len(v.LicenseBytes) == 0 || len(v.GeneratedBytes) != len(v.Provenance.GeneratedContent) {
		return fmt.Errorf("%w: verify prompt provenance request", ErrInvalid)
	}
	previous := ""
	for index, item := range v.ArtifactBytes {
		if validateID(item.ArtifactID) != nil {
			return fmt.Errorf("%w: prompt artifact bytes", ErrInvalid)
		}
		if index > 0 && previous >= item.ArtifactID {
			return fmt.Errorf("%w: prompt artifact bytes not canonical", ErrConflict)
		}
		previous = item.ArtifactID
	}
	previous = ""
	for index, item := range v.GeneratedBytes {
		if item.Ref.Validate() != nil {
			return fmt.Errorf("%w: prompt generated bytes", ErrInvalid)
		}
		key := fmt.Sprintf("%s\x00%020d\x00%s", item.Ref.Ref, item.Ref.Length, item.Ref.Digest)
		if index > 0 && previous >= key {
			return fmt.Errorf("%w: prompt generated bytes not canonical", ErrConflict)
		}
		previous = key
	}
	return nil
}

type PromptUpstreamVerificationReportV1 struct {
	ProvenanceRef       PromptUpstreamProvenanceRefV1 `json:"provenance_ref"`
	SourceSetDigest     Digest                        `json:"source_set_digest"`
	GeneratedSetDigest  Digest                        `json:"generated_set_digest"`
	ClosureDigest       Digest                        `json:"closure_digest"`
	VerifiedArtifactIDs []string                      `json:"verified_artifact_ids"`
	VerifiedContentRefs []ContentRef                  `json:"verified_content_refs"`
	CheckedUnixNano     int64                         `json:"checked_unix_nano"`
	ExpiresUnixNano     int64                         `json:"expires_unix_nano"`
	ReportDigest        Digest                        `json:"report_digest"`
}

func (v PromptUpstreamVerificationReportV1) Validate() error {
	if v.ProvenanceRef.Validate() != nil || v.SourceSetDigest.Validate() != nil || v.GeneratedSetDigest.Validate() != nil || v.ClosureDigest.Validate() != nil || validateTimes(v.CheckedUnixNano, v.ExpiresUnixNano) != nil || v.ReportDigest.Validate() != nil || !canonicalPromptArtifactIDsV1(v.VerifiedArtifactIDs) || !canonicalContentRefsV1(v.VerifiedContentRefs) {
		return fmt.Errorf("%w: prompt provenance verification report", ErrInvalid)
	}
	want, _ := v.digestValue()
	if want != v.ReportDigest {
		return fmt.Errorf("%w: prompt verification report digest", ErrConflict)
	}
	return nil
}

func (v PromptUpstreamVerificationReportV1) digestValue() (Digest, error) {
	copy := v
	copy.ReportDigest = ""
	return DigestJSON(copy)
}

func SealPromptUpstreamVerificationReportV1(v PromptUpstreamVerificationReportV1) (PromptUpstreamVerificationReportV1, error) {
	v.VerifiedArtifactIDs = append([]string(nil), v.VerifiedArtifactIDs...)
	sort.Strings(v.VerifiedArtifactIDs)
	v.VerifiedContentRefs = append([]ContentRef(nil), v.VerifiedContentRefs...)
	sortContentRefsV1(v.VerifiedContentRefs)
	v.ReportDigest = ""
	v.ReportDigest, _ = v.digestValue()
	return v, v.Validate()
}

func canonicalPromptArtifactIDsV1(v []string) bool {
	if len(v) == 0 || len(v) > MaxPromptUpstreamArtifactsV1 {
		return false
	}
	previous := ""
	for index, current := range v {
		if validateID(current) != nil || index > 0 && previous >= current {
			return false
		}
		previous = current
	}
	return true
}

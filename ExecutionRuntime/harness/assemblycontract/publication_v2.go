package assemblycontract

import (
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const PublicationContractVersionV2 = "praxis.harness.assembly.publication/v2"

// AssemblyPublicationRefV2 identifies one create-once publication. Revision is
// permanently one; current movement is represented by AssemblyPublicationCurrentV2.
type AssemblyPublicationRefV2 struct {
	PublicationID string        `json:"publication_id"`
	Revision      core.Revision `json:"revision"`
	Digest        core.Digest   `json:"digest"`
}

type AssemblyPublicationArtifactRefsV2 struct {
	Generation ObjectRefV1 `json:"generation"`
	Manifest   ObjectRefV1 `json:"manifest"`
	Graph      ObjectRefV1 `json:"graph"`
	Handoff    ObjectRefV1 `json:"handoff"`
}

// AssemblyPublicationV2 is the immutable historical commit marker. The four
// objects are readable only after this marker and the scope current are made
// visible by one store commit barrier.
type AssemblyPublicationV2 struct {
	ContractVersion string                            `json:"contract_version"`
	PublicationID   string                            `json:"publication_id"`
	Revision        core.Revision                     `json:"revision"`
	ScopeRef        string                            `json:"scope_ref"`
	InputDigest     core.Digest                       `json:"input_digest"`
	Artifacts       AssemblyPublicationArtifactRefsV2 `json:"artifacts"`
	ContentDigest   core.Digest                       `json:"content_digest"`
	Digest          core.Digest                       `json:"digest"`
}

type AssemblyPublicationBundleV2 struct {
	Publication AssemblyPublicationV2  `json:"publication"`
	Generation  AssemblyGenerationV1   `json:"generation"`
	Manifest    AssemblyManifestV1     `json:"manifest"`
	Graph       CompiledHarnessGraphV1 `json:"graph"`
	Handoff     AssemblyHandoffV1      `json:"handoff"`
}

// AssemblyPublicationCurrentV2 is the sole externally visible publication
// barrier for one Assembly scope.
type AssemblyPublicationCurrentV2 struct {
	ContractVersion string                            `json:"contract_version"`
	ScopeRef        string                            `json:"scope_ref"`
	Revision        core.Revision                     `json:"revision"`
	Publication     AssemblyPublicationRefV2          `json:"publication"`
	InputDigest     core.Digest                       `json:"input_digest"`
	Artifacts       AssemblyPublicationArtifactRefsV2 `json:"artifacts"`
	CommitAttemptID string                            `json:"commit_attempt_id"`
	CheckedUnixNano int64                             `json:"checked_unix_nano"`
	ExpiresUnixNano int64                             `json:"expires_unix_nano"`
	Digest          core.Digest                       `json:"digest"`
}

type AssemblyPublicationCurrentExpectationV2 struct {
	Exists   bool          `json:"exists"`
	Revision core.Revision `json:"revision,omitempty"`
	Digest   core.Digest   `json:"digest,omitempty"`
}

type CompileAndPublishAssemblyRequestV2 struct {
	ContractVersion          string                                  `json:"contract_version"`
	AttemptID                string                                  `json:"attempt_id"`
	Input                    AssemblyInputV1                         `json:"input"`
	ExpectedCurrent          AssemblyPublicationCurrentExpectationV2 `json:"expected_current"`
	RequestedExpiresUnixNano int64                                   `json:"requested_expires_unix_nano"`
}

type CompileAndPublishAssemblyResultV2 struct {
	Publication        AssemblyPublicationV2        `json:"publication"`
	Current            AssemblyPublicationCurrentV2 `json:"current"`
	RecoveredByInspect bool                         `json:"recovered_by_inspect"`
}

func (value AssemblyPublicationRefV2) Validate() error {
	if err := validateID(value.PublicationID); err != nil {
		return err
	}
	if value.Revision != 1 || value.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "publication ref requires its create-once revision and digest")
	}
	return nil
}

func (value CompileAndPublishAssemblyRequestV2) Validate() error {
	if value.ContractVersion != PublicationContractVersionV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Assembly publication request contract is unsupported")
	}
	if err := validateID(value.AttemptID); err != nil {
		return err
	}
	if err := value.Input.Validate(); err != nil {
		return err
	}
	if err := value.ExpectedCurrent.Validate(); err != nil {
		return err
	}
	if value.RequestedExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonBindingExpired, "Assembly publication requested expiry is required")
	}
	return nil
}

func DeriveAssemblyPublicationIDV2(inputDigest core.Digest, generationID string) (string, error) {
	if err := inputDigest.Validate(); err != nil {
		return "", err
	}
	if err := validateID(generationID); err != nil {
		return "", err
	}
	digest, err := core.CanonicalJSONDigest("praxis.harness.assembly", PublicationContractVersionV2, "AssemblyPublicationIDV2", struct {
		InputDigest  core.Digest `json:"input_digest"`
		GenerationID string      `json:"generation_id"`
	}{inputDigest, generationID})
	if err != nil {
		return "", err
	}
	return "assembly-publication-" + strings.TrimPrefix(string(digest), "sha256:")[:32], nil
}

func NewAssemblyPublicationBundleV2(scopeRef string, result CompileResultV1) (AssemblyPublicationBundleV2, error) {
	if err := validateID(scopeRef); err != nil {
		return AssemblyPublicationBundleV2{}, err
	}
	if result.Generation == nil || result.Manifest == nil || result.Graph == nil || result.Handoff == nil {
		return AssemblyPublicationBundleV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReadyEvidenceIncomplete, "successful Assembly publication requires Generation, Manifest, Graph and Handoff")
	}
	generation, manifest, graph, handoff := *result.Generation, *result.Manifest, *result.Graph, *result.Handoff
	if err := validatePublicationArtifactsV2(generation, manifest, graph, handoff); err != nil {
		return AssemblyPublicationBundleV2{}, err
	}
	publicationID, err := DeriveAssemblyPublicationIDV2(generation.InputDigest, generation.GenerationID)
	if err != nil {
		return AssemblyPublicationBundleV2{}, err
	}
	refs := AssemblyPublicationArtifactRefsV2{
		Generation: ObjectRefV1{ID: generation.GenerationID, Revision: generation.Revision, Digest: generation.Digest},
		Manifest:   ObjectRefV1{ID: publicationID + "/manifest", Revision: 1, Digest: manifest.Digest},
		Graph:      ObjectRefV1{ID: publicationID + "/graph", Revision: 1, Digest: graph.Digest},
		Handoff:    ObjectRefV1{ID: publicationID + "/handoff", Revision: 1, Digest: handoff.Digest},
	}
	contentDigest, err := publicationContentDigestV2(generation.InputDigest, refs)
	if err != nil {
		return AssemblyPublicationBundleV2{}, err
	}
	publication := AssemblyPublicationV2{ContractVersion: PublicationContractVersionV2, PublicationID: publicationID, Revision: 1, ScopeRef: scopeRef, InputDigest: generation.InputDigest, Artifacts: refs, ContentDigest: contentDigest}
	publication.Digest, err = AssemblyPublicationDigestV2(publication)
	if err != nil {
		return AssemblyPublicationBundleV2{}, err
	}
	bundle := AssemblyPublicationBundleV2{Publication: publication, Generation: generation, Manifest: manifest, Graph: graph, Handoff: handoff}
	if err := bundle.Validate(); err != nil {
		return AssemblyPublicationBundleV2{}, err
	}
	return bundle, nil
}

func NewAssemblyPublicationCurrentV2(bundle AssemblyPublicationBundleV2, attemptID string, revision core.Revision, checked time.Time, expiresUnixNano int64) (AssemblyPublicationCurrentV2, error) {
	if err := bundle.Validate(); err != nil {
		return AssemblyPublicationCurrentV2{}, err
	}
	if err := validateID(attemptID); err != nil {
		return AssemblyPublicationCurrentV2{}, err
	}
	if revision == 0 || checked.IsZero() || expiresUnixNano <= checked.UnixNano() {
		return AssemblyPublicationCurrentV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonBindingExpired, "publication current revision and a live current window are required")
	}
	value := AssemblyPublicationCurrentV2{
		ContractVersion: PublicationContractVersionV2, ScopeRef: bundle.Publication.ScopeRef, Revision: revision,
		Publication: AssemblyPublicationRefV2{PublicationID: bundle.Publication.PublicationID, Revision: bundle.Publication.Revision, Digest: bundle.Publication.Digest},
		InputDigest: bundle.Publication.InputDigest, Artifacts: bundle.Publication.Artifacts, CommitAttemptID: attemptID,
		CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: expiresUnixNano,
	}
	var err error
	value.Digest, err = AssemblyPublicationCurrentDigestV2(value)
	if err != nil {
		return AssemblyPublicationCurrentV2{}, err
	}
	if err := value.ValidateAt(checked); err != nil {
		return AssemblyPublicationCurrentV2{}, err
	}
	return value, nil
}

func AssemblyPublicationDigestV2(value AssemblyPublicationV2) (core.Digest, error) {
	value.Digest = ""
	return core.CanonicalJSONDigest("praxis.harness.assembly", PublicationContractVersionV2, "AssemblyPublicationV2", value)
}

func AssemblyPublicationCurrentDigestV2(value AssemblyPublicationCurrentV2) (core.Digest, error) {
	value.Digest = ""
	return core.CanonicalJSONDigest("praxis.harness.assembly", PublicationContractVersionV2, "AssemblyPublicationCurrentV2", value)
}

func (value AssemblyPublicationBundleV2) Validate() error {
	if err := value.Publication.Validate(); err != nil {
		return err
	}
	if err := validatePublicationArtifactsV2(value.Generation, value.Manifest, value.Graph, value.Handoff); err != nil {
		return err
	}
	refs := value.Publication.Artifacts
	if refs.Generation != (ObjectRefV1{ID: value.Generation.GenerationID, Revision: value.Generation.Revision, Digest: value.Generation.Digest}) ||
		refs.Manifest != (ObjectRefV1{ID: value.Publication.PublicationID + "/manifest", Revision: 1, Digest: value.Manifest.Digest}) ||
		refs.Graph != (ObjectRefV1{ID: value.Publication.PublicationID + "/graph", Revision: 1, Digest: value.Graph.Digest}) ||
		refs.Handoff != (ObjectRefV1{ID: value.Publication.PublicationID + "/handoff", Revision: 1, Digest: value.Handoff.Digest}) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "publication artifact refs drifted from the immutable objects")
	}
	return nil
}

func (value AssemblyPublicationV2) Validate() error {
	if value.ContractVersion != PublicationContractVersionV2 || value.Revision != 1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "publication contract and create-once revision are required")
	}
	if err := validateID(value.PublicationID); err != nil {
		return err
	}
	if err := validateID(value.ScopeRef); err != nil {
		return err
	}
	if err := value.InputDigest.Validate(); err != nil {
		return err
	}
	for _, ref := range []ObjectRefV1{value.Artifacts.Generation, value.Artifacts.Manifest, value.Artifacts.Graph, value.Artifacts.Handoff} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if value.Artifacts.Manifest.ID != value.PublicationID+"/manifest" || value.Artifacts.Manifest.Revision != 1 || value.Artifacts.Graph.ID != value.PublicationID+"/graph" || value.Artifacts.Graph.Revision != 1 || value.Artifacts.Handoff.ID != value.PublicationID+"/handoff" || value.Artifacts.Handoff.Revision != 1 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "publication artifact identities drifted")
	}
	expectedID, err := DeriveAssemblyPublicationIDV2(value.InputDigest, value.Artifacts.Generation.ID)
	if err != nil || expectedID != value.PublicationID {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "publication ID is not derived from InputDigest and GenerationID")
	}
	contentDigest, err := publicationContentDigestV2(value.InputDigest, value.Artifacts)
	if err != nil || contentDigest != value.ContentDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "publication content digest drifted")
	}
	digest, err := AssemblyPublicationDigestV2(value)
	if err != nil || digest != value.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "publication digest drifted")
	}
	return nil
}

func (value AssemblyPublicationCurrentV2) ValidateAt(now time.Time) error {
	if value.ContractVersion != PublicationContractVersionV2 || value.Revision == 0 || now.IsZero() {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "publication current contract, revision and observation time are required")
	}
	if err := validateID(value.ScopeRef); err != nil {
		return err
	}
	if err := validateID(value.CommitAttemptID); err != nil {
		return err
	}
	if value.Publication.Validate() != nil || value.InputDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "publication current requires an exact publication and input")
	}
	for _, ref := range []ObjectRefV1{value.Artifacts.Generation, value.Artifacts.Manifest, value.Artifacts.Graph, value.Artifacts.Handoff} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	expectedID, err := DeriveAssemblyPublicationIDV2(value.InputDigest, value.Artifacts.Generation.ID)
	if err != nil || expectedID != value.Publication.PublicationID || value.Artifacts.Manifest.ID != expectedID+"/manifest" || value.Artifacts.Graph.ID != expectedID+"/graph" || value.Artifacts.Handoff.ID != expectedID+"/handoff" {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "publication current artifact identities drifted")
	}
	if value.CheckedUnixNano <= 0 || now.UnixNano() < value.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "publication current observation clock regressed")
	}
	if value.ExpiresUnixNano <= now.UnixNano() {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "publication current expired")
	}
	digest, err := AssemblyPublicationCurrentDigestV2(value)
	if err != nil || digest != value.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "publication current digest drifted")
	}
	return nil
}

func (value AssemblyPublicationCurrentExpectationV2) Validate() error {
	if !value.Exists {
		if value.Revision != 0 || value.Digest != "" {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "absent publication current expectation must not carry revision or digest")
		}
		return nil
	}
	if value.Revision == 0 || value.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "existing publication current expectation requires revision and digest")
	}
	return nil
}

func publicationContentDigestV2(inputDigest core.Digest, refs AssemblyPublicationArtifactRefsV2) (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.harness.assembly", PublicationContractVersionV2, "AssemblyPublicationContentV2", struct {
		InputDigest core.Digest                       `json:"input_digest"`
		Artifacts   AssemblyPublicationArtifactRefsV2 `json:"artifacts"`
	}{inputDigest, refs})
}

func validatePublicationArtifactsV2(generation AssemblyGenerationV1, manifest AssemblyManifestV1, graph CompiledHarnessGraphV1, handoff AssemblyHandoffV1) error {
	if generation.ContractVersion != ContractVersionV1 || generation.CompilerVersion != CompilerVersionV1 || generation.Revision == 0 || generation.State != AssemblyStateSealedV1 || generation.CreatedUnixNano <= 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "publication requires one sealed Assembly Generation")
	}
	generationDigest, err := GenerationDigestV1(generation)
	if err != nil || generationDigest != generation.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Assembly Generation digest drifted")
	}
	manifestDigest, err := ManifestDigestV1(manifest)
	if err != nil || manifestDigest != manifest.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Assembly Manifest digest drifted")
	}
	graphDigest, err := GraphDigestV1(graph)
	if err != nil || graphDigest != graph.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Compiled Harness Graph digest drifted")
	}
	if err := handoff.Validate(); err != nil {
		return err
	}
	if generation.InputDigest != manifest.InputDigest || generation.InputDigest != graph.InputDigest || generation.ManifestDigest != manifest.Digest || generation.GraphDigest != graph.Digest || handoff.GenerationRef != (ObjectRefV1{ID: generation.GenerationID, Revision: generation.Revision, Digest: generation.Digest}) || handoff.ManifestDigest != manifest.Digest || handoff.GraphDigest != graph.Digest || handoff.CatalogDigest != manifest.CatalogDigest || handoff.CatalogDigest != graph.CatalogDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Assembly publication artifact chain drifted")
	}
	return nil
}

package ports

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	ModelPreDispatchAssemblyCurrentContractVersionV1 = "1.0.0"
	modelPreDispatchAssemblyCurrentCanonicalDomainV1 = "praxis.runtime.model-predispatch-assembly-current/v1"
	registrySnapshotRefCanonicalDomainV1             = "praxis.runtime.registry-snapshot-ref/v1"
)

// RegistrySnapshotRefV1 is a Runtime-neutral coordinate issued by the
// Registry authority named by Owner. Runtime owns this Go nominal, not the
// Registry fact, repository or current pointer.
type RegistrySnapshotRefV1 struct {
	Owner           core.OwnerRef `json:"owner"`
	ContractVersion string        `json:"contract_version"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
}

func (r RegistrySnapshotRefV1) Validate() error {
	if err := r.Owner.Validate(); err != nil {
		return err
	}
	version, err := core.ParseSemanticVersion(r.ContractVersion)
	if err != nil || version.String() != r.ContractVersion {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidSemanticVersion, "Registry snapshot contract version must be canonical SemVer")
	}
	if validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Registry snapshot identity and revision are required")
	}
	return r.Digest.Validate()
}

// CanonicalIdentityDigestV1 is a carrier digest over the complete exact Ref.
// It never replaces the Registry-owned fact Digest stored in the Ref.
func (r RegistrySnapshotRefV1) CanonicalIdentityDigestV1() (core.Digest, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest(registrySnapshotRefCanonicalDomainV1, ModelPreDispatchAssemblyCurrentContractVersionV1, "RegistrySnapshotRefV1", r)
}

// RegistrySnapshotExactReaderV1 is implemented by the Registry authority. It
// performs an exact historical read and verifies that the same Ref is still
// current before returning a value clone. It exposes no publish or CAS method.
type RegistrySnapshotExactReaderV1 interface {
	InspectExactRegistrySnapshotV1(context.Context, RegistrySnapshotRefV1) (RegistrySnapshotRefV1, error)
}

type ModelPreDispatchAssemblyExactRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r ModelPreDispatchAssemblyExactRefV1) Validate() error {
	if validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Model pre-dispatch Assembly exact ref is incomplete")
	}
	return r.Digest.Validate()
}

func (r ModelPreDispatchAssemblyExactRefV1) CanonicalIdentityDigestV1() (core.Digest, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest(modelPreDispatchAssemblyCurrentCanonicalDomainV1, ModelPreDispatchAssemblyCurrentContractVersionV1, "ModelPreDispatchAssemblyExactRefV1", r)
}

type ModelPreDispatchAssemblyBindingSetRefV1 struct {
	ID                string        `json:"id"`
	Revision          core.Revision `json:"revision"`
	Digest            core.Digest   `json:"digest"`
	SemanticDigest    core.Digest   `json:"semantic_digest"`
	CurrentnessDigest core.Digest   `json:"currentness_digest"`
	ProjectionDigest  core.Digest   `json:"projection_digest"`
	ExpiresUnixNano   int64         `json:"expires_unix_nano"`
}

func (r ModelPreDispatchAssemblyBindingSetRefV1) Validate() error {
	if validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 || r.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Model pre-dispatch BindingSet ref is incomplete")
	}
	for _, digest := range []core.Digest{r.Digest, r.SemanticDigest, r.CurrentnessDigest, r.ProjectionDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (r ModelPreDispatchAssemblyBindingSetRefV1) CanonicalIdentityDigestV1() (core.Digest, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest(modelPreDispatchAssemblyCurrentCanonicalDomainV1, ModelPreDispatchAssemblyCurrentContractVersionV1, "ModelPreDispatchAssemblyBindingSetRefV1", r)
}

type ModelPreDispatchAssemblyCurrentRefV1 struct {
	ContractVersion   string                                  `json:"contract_version"`
	ID                string                                  `json:"id"`
	Revision          core.Revision                           `json:"revision"`
	Digest            core.Digest                             `json:"digest"`
	Generation        GenerationArtifactRefV1                 `json:"generation"`
	Handoff           ModelPreDispatchAssemblyExactRefV1      `json:"handoff"`
	BindingSet        ModelPreDispatchAssemblyBindingSetRefV1 `json:"binding_set"`
	Manifest          ModelPreDispatchAssemblyExactRefV1      `json:"manifest"`
	Conformance       ModelPreDispatchAssemblyExactRefV1      `json:"conformance"`
	WatermarkDigest   core.Digest                             `json:"watermark_digest"`
	ToolSurface       ModelPreDispatchAssemblyExactRefV1      `json:"tool_surface"`
	ProfileDigest     core.Digest                             `json:"profile_digest"`
	RegistrySnapshot  RegistrySnapshotRefV1                   `json:"registry_snapshot"`
	SemanticDigest    core.Digest                             `json:"semantic_digest"`
	CurrentnessDigest core.Digest                             `json:"currentness_digest"`
	CheckedUnixNano   int64                                   `json:"checked_unix_nano"`
	ExpiresUnixNano   int64                                   `json:"expires_unix_nano"`
	ProjectionDigest  core.Digest                             `json:"projection_digest"`
}

func (r ModelPreDispatchAssemblyCurrentRefV1) Validate() error {
	if err := validateModelPreDispatchAssemblyCurrentRefShapeV1(r); err != nil {
		return err
	}
	expectedID, err := DeriveModelPreDispatchAssemblyCurrentIDV1(projectionFromModelPreDispatchAssemblyRefV1(r))
	if err != nil || expectedID != r.ID {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Model pre-dispatch Assembly current ID drifted")
	}
	projection := projectionFromModelPreDispatchAssemblyRefV1(r)
	watermark, err := DigestModelPreDispatchAssemblyWatermarkV1(projection)
	if err != nil || watermark != r.WatermarkDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Model pre-dispatch Assembly watermark drifted")
	}
	digest, err := DigestModelPreDispatchAssemblyCurrentProjectionV1(projection)
	if err != nil || digest != r.Digest || digest != r.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Model pre-dispatch Assembly current Ref digest drifted")
	}
	return nil
}

func (r ModelPreDispatchAssemblyCurrentRefV1) DigestV1() (core.Digest, error) {
	return DigestModelPreDispatchAssemblyCurrentProjectionV1(projectionFromModelPreDispatchAssemblyRefV1(r))
}

type ModelPreDispatchAssemblyCurrentProjectionV1 struct {
	ContractVersion   string                                  `json:"contract_version"`
	Ref               ModelPreDispatchAssemblyCurrentRefV1    `json:"ref"`
	Generation        GenerationArtifactRefV1                 `json:"generation"`
	Handoff           ModelPreDispatchAssemblyExactRefV1      `json:"handoff"`
	BindingSet        ModelPreDispatchAssemblyBindingSetRefV1 `json:"binding_set"`
	Manifest          ModelPreDispatchAssemblyExactRefV1      `json:"manifest"`
	Conformance       ModelPreDispatchAssemblyExactRefV1      `json:"conformance"`
	ToolSurface       ModelPreDispatchAssemblyExactRefV1      `json:"tool_surface"`
	ProfileDigest     core.Digest                             `json:"profile_digest"`
	RegistrySnapshot  RegistrySnapshotRefV1                   `json:"registry_snapshot"`
	SemanticDigest    core.Digest                             `json:"semantic_digest"`
	CurrentnessDigest core.Digest                             `json:"currentness_digest"`
	CheckedUnixNano   int64                                   `json:"checked_unix_nano"`
	ExpiresUnixNano   int64                                   `json:"expires_unix_nano"`
	ProjectionDigest  core.Digest                             `json:"projection_digest"`
}

func (p ModelPreDispatchAssemblyCurrentProjectionV1) Validate() error {
	if err := validateModelPreDispatchAssemblyProjectionInputsV1(p); err != nil {
		return err
	}
	if err := p.Ref.Validate(); err != nil {
		return err
	}
	if p.Ref.ContractVersion != p.ContractVersion || p.Ref.Generation != p.Generation || p.Ref.Handoff != p.Handoff || p.Ref.BindingSet != p.BindingSet || p.Ref.Manifest != p.Manifest || p.Ref.Conformance != p.Conformance || p.Ref.ToolSurface != p.ToolSurface || p.Ref.ProfileDigest != p.ProfileDigest || p.Ref.RegistrySnapshot != p.RegistrySnapshot || p.Ref.SemanticDigest != p.SemanticDigest || p.Ref.CurrentnessDigest != p.CurrentnessDigest || p.Ref.CheckedUnixNano != p.CheckedUnixNano || p.Ref.ExpiresUnixNano != p.ExpiresUnixNano || p.Ref.ProjectionDigest != p.ProjectionDigest || p.Ref.Digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Model pre-dispatch Assembly Ref and projection drifted")
	}
	watermark, err := DigestModelPreDispatchAssemblyWatermarkV1(p)
	if err != nil || watermark != p.Ref.WatermarkDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Model pre-dispatch Assembly projection watermark drifted")
	}
	digest, err := DigestModelPreDispatchAssemblyCurrentProjectionV1(p)
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Model pre-dispatch Assembly projection digest drifted")
	}
	return nil
}

func (p ModelPreDispatchAssemblyCurrentProjectionV1) ValidateCurrent(expected ModelPreDispatchAssemblyCurrentRefV1, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if err := expected.Validate(); err != nil {
		return err
	}
	if p.Ref != expected {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Model pre-dispatch Assembly current Ref drifted")
	}
	if now.IsZero() {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Model pre-dispatch Assembly current validation requires time")
	}
	if now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Model pre-dispatch Assembly current clock regressed")
	}
	if !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Model pre-dispatch Assembly current projection expired")
	}
	return nil
}

func (p ModelPreDispatchAssemblyCurrentProjectionV1) DigestV1() (core.Digest, error) {
	return DigestModelPreDispatchAssemblyCurrentProjectionV1(p)
}

func SealModelPreDispatchAssemblyCurrentProjectionV1(p ModelPreDispatchAssemblyCurrentProjectionV1) (ModelPreDispatchAssemblyCurrentProjectionV1, error) {
	if p.ContractVersion != "" && p.ContractVersion != ModelPreDispatchAssemblyCurrentContractVersionV1 {
		return ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Model pre-dispatch Assembly contract version is invalid")
	}
	p.ContractVersion = ModelPreDispatchAssemblyCurrentContractVersionV1
	if err := validateModelPreDispatchAssemblyProjectionInputsV1(p); err != nil {
		return ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	if p.Ref.ContractVersion != "" && p.Ref.ContractVersion != p.ContractVersion {
		return ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Model pre-dispatch Assembly supplied a wrong Ref contract version")
	}
	p.Ref.ContractVersion = p.ContractVersion
	if err := setExactModelPreDispatchAssemblyRefFieldsV1(&p); err != nil {
		return ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	expectedID, err := DeriveModelPreDispatchAssemblyCurrentIDV1(p)
	if err != nil {
		return ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	if p.Ref.ID != "" && p.Ref.ID != expectedID {
		return ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Model pre-dispatch Assembly supplied a wrong current ID")
	}
	p.Ref.ID = expectedID
	if p.Ref.Revision == 0 {
		return ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Model pre-dispatch Assembly current revision is required")
	}
	expectedWatermark, err := DigestModelPreDispatchAssemblyWatermarkV1(p)
	if err != nil {
		return ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	if p.Ref.WatermarkDigest != "" && p.Ref.WatermarkDigest != expectedWatermark {
		return ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Model pre-dispatch Assembly supplied a wrong watermark")
	}
	p.Ref.WatermarkDigest = expectedWatermark
	providedRefDigest := p.Ref.Digest
	providedRefProjectionDigest := p.Ref.ProjectionDigest
	providedProjectionDigest := p.ProjectionDigest
	p.Ref.Digest = ""
	p.Ref.ProjectionDigest = ""
	p.ProjectionDigest = ""
	digest, err := DigestModelPreDispatchAssemblyCurrentProjectionV1(p)
	if err != nil {
		return ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	for _, provided := range []core.Digest{providedRefDigest, providedRefProjectionDigest, providedProjectionDigest} {
		if provided != "" && provided != digest {
			return ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Model pre-dispatch Assembly supplied a wrong nonzero projection digest")
		}
	}
	p.Ref.Digest = digest
	p.Ref.ProjectionDigest = digest
	p.ProjectionDigest = digest
	return p, p.Validate()
}

func DeriveModelPreDispatchAssemblyCurrentIDV1(p ModelPreDispatchAssemblyCurrentProjectionV1) (string, error) {
	if p.ContractVersion != ModelPreDispatchAssemblyCurrentContractVersionV1 || p.Generation.Validate() != nil || p.Handoff.Validate() != nil || p.BindingSet.Validate() != nil || p.Manifest.Validate() != nil || p.Conformance.Validate() != nil || p.ToolSurface.Validate() != nil || p.ProfileDigest.Validate() != nil || p.RegistrySnapshot.Validate() != nil {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Model pre-dispatch Assembly stable identity inputs are incomplete")
	}
	digest, err := core.CanonicalJSONDigest(modelPreDispatchAssemblyCurrentCanonicalDomainV1, ModelPreDispatchAssemblyCurrentContractVersionV1, "ModelPreDispatchAssemblyCurrentIdentityV1", struct {
		ContractVersion         string        `json:"contract_version"`
		GenerationID            string        `json:"generation_id"`
		HandoffID               string        `json:"handoff_id"`
		BindingSetID            string        `json:"binding_set_id"`
		ManifestID              string        `json:"manifest_id"`
		ConformanceID           string        `json:"conformance_id"`
		ToolSurfaceID           string        `json:"tool_surface_id"`
		ProfileDigest           core.Digest   `json:"profile_digest"`
		RegistryOwner           core.OwnerRef `json:"registry_owner"`
		RegistryContractVersion string        `json:"registry_contract_version"`
		RegistryID              string        `json:"registry_id"`
	}{p.ContractVersion, p.Generation.ID, p.Handoff.ID, p.BindingSet.ID, p.Manifest.ID, p.Conformance.ID, p.ToolSurface.ID, p.ProfileDigest, p.RegistrySnapshot.Owner, p.RegistrySnapshot.ContractVersion, p.RegistrySnapshot.ID})
	if err != nil {
		return "", err
	}
	return "model-predispatch-assembly-current-" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func DigestModelPreDispatchAssemblyWatermarkV1(p ModelPreDispatchAssemblyCurrentProjectionV1) (core.Digest, error) {
	if err := validateModelPreDispatchAssemblyProjectionInputsV1(p); err != nil {
		return "", err
	}
	p.Ref.WatermarkDigest = ""
	p.Ref.Digest = ""
	p.Ref.ProjectionDigest = ""
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest(modelPreDispatchAssemblyCurrentCanonicalDomainV1, ModelPreDispatchAssemblyCurrentContractVersionV1, "ModelPreDispatchAssemblyWatermarkV1", p)
}

func DigestModelPreDispatchAssemblyCurrentProjectionV1(p ModelPreDispatchAssemblyCurrentProjectionV1) (core.Digest, error) {
	if err := validateModelPreDispatchAssemblyProjectionInputsV1(p); err != nil {
		return "", err
	}
	p.Ref.Digest = ""
	p.Ref.ProjectionDigest = ""
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest(modelPreDispatchAssemblyCurrentCanonicalDomainV1, ModelPreDispatchAssemblyCurrentContractVersionV1, "ModelPreDispatchAssemblyCurrentProjectionV1", p)
}

type ModelPreDispatchAssemblyCurrentReaderV1 interface {
	InspectCurrentModelPreDispatchAssemblyV1(context.Context, ModelPreDispatchAssemblyCurrentRefV1) (ModelPreDispatchAssemblyCurrentProjectionV1, error)
}

func validateModelPreDispatchAssemblyProjectionInputsV1(p ModelPreDispatchAssemblyCurrentProjectionV1) error {
	if p.ContractVersion != ModelPreDispatchAssemblyCurrentContractVersionV1 || p.Generation.Validate() != nil || p.Handoff.Validate() != nil || p.BindingSet.Validate() != nil || p.Manifest.Validate() != nil || p.Conformance.Validate() != nil || p.ToolSurface.Validate() != nil || p.ProfileDigest.Validate() != nil || p.RegistrySnapshot.Validate() != nil || p.SemanticDigest.Validate() != nil || p.CurrentnessDigest.Validate() != nil || p.CheckedUnixNano <= 0 || p.CheckedUnixNano >= p.ExpiresUnixNano || p.ExpiresUnixNano > p.BindingSet.ExpiresUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Model pre-dispatch Assembly projection inputs are incomplete")
	}
	return nil
}

func validateModelPreDispatchAssemblyCurrentRefShapeV1(r ModelPreDispatchAssemblyCurrentRefV1) error {
	if r.ContractVersion != ModelPreDispatchAssemblyCurrentContractVersionV1 || validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 || r.Generation.Validate() != nil || r.Handoff.Validate() != nil || r.BindingSet.Validate() != nil || r.Manifest.Validate() != nil || r.Conformance.Validate() != nil || r.ToolSurface.Validate() != nil || r.ProfileDigest.Validate() != nil || r.RegistrySnapshot.Validate() != nil || r.SemanticDigest.Validate() != nil || r.CurrentnessDigest.Validate() != nil || r.WatermarkDigest.Validate() != nil || r.Digest.Validate() != nil || r.ProjectionDigest.Validate() != nil || r.CheckedUnixNano <= 0 || r.CheckedUnixNano >= r.ExpiresUnixNano || r.ExpiresUnixNano > r.BindingSet.ExpiresUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Model pre-dispatch Assembly current Ref is incomplete")
	}
	return nil
}

func setExactModelPreDispatchAssemblyRefFieldsV1(p *ModelPreDispatchAssemblyCurrentProjectionV1) error {
	checks := []struct {
		provided bool
		exact    bool
		name     string
	}{
		{p.Ref.Generation != (GenerationArtifactRefV1{}), p.Ref.Generation == p.Generation, "Generation"},
		{p.Ref.Handoff != (ModelPreDispatchAssemblyExactRefV1{}), p.Ref.Handoff == p.Handoff, "Handoff"},
		{p.Ref.BindingSet != (ModelPreDispatchAssemblyBindingSetRefV1{}), p.Ref.BindingSet == p.BindingSet, "BindingSet"},
		{p.Ref.Manifest != (ModelPreDispatchAssemblyExactRefV1{}), p.Ref.Manifest == p.Manifest, "Manifest"},
		{p.Ref.Conformance != (ModelPreDispatchAssemblyExactRefV1{}), p.Ref.Conformance == p.Conformance, "Conformance"},
		{p.Ref.ToolSurface != (ModelPreDispatchAssemblyExactRefV1{}), p.Ref.ToolSurface == p.ToolSurface, "ToolSurface"},
		{p.Ref.ProfileDigest != "", p.Ref.ProfileDigest == p.ProfileDigest, "ProfileDigest"},
		{p.Ref.RegistrySnapshot != (RegistrySnapshotRefV1{}), p.Ref.RegistrySnapshot == p.RegistrySnapshot, "RegistrySnapshot"},
		{p.Ref.SemanticDigest != "", p.Ref.SemanticDigest == p.SemanticDigest, "SemanticDigest"},
		{p.Ref.CurrentnessDigest != "", p.Ref.CurrentnessDigest == p.CurrentnessDigest, "CurrentnessDigest"},
		{p.Ref.CheckedUnixNano != 0, p.Ref.CheckedUnixNano == p.CheckedUnixNano, "CheckedUnixNano"},
		{p.Ref.ExpiresUnixNano != 0, p.Ref.ExpiresUnixNano == p.ExpiresUnixNano, "ExpiresUnixNano"},
	}
	for _, check := range checks {
		if check.provided && !check.exact {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Model pre-dispatch Assembly supplied a wrong nonzero "+check.name)
		}
	}
	p.Ref.Generation = p.Generation
	p.Ref.Handoff = p.Handoff
	p.Ref.BindingSet = p.BindingSet
	p.Ref.Manifest = p.Manifest
	p.Ref.Conformance = p.Conformance
	p.Ref.ToolSurface = p.ToolSurface
	p.Ref.ProfileDigest = p.ProfileDigest
	p.Ref.RegistrySnapshot = p.RegistrySnapshot
	p.Ref.SemanticDigest = p.SemanticDigest
	p.Ref.CurrentnessDigest = p.CurrentnessDigest
	p.Ref.CheckedUnixNano = p.CheckedUnixNano
	p.Ref.ExpiresUnixNano = p.ExpiresUnixNano
	return nil
}

func projectionFromModelPreDispatchAssemblyRefV1(r ModelPreDispatchAssemblyCurrentRefV1) ModelPreDispatchAssemblyCurrentProjectionV1 {
	return ModelPreDispatchAssemblyCurrentProjectionV1{
		ContractVersion:   r.ContractVersion,
		Ref:               r,
		Generation:        r.Generation,
		Handoff:           r.Handoff,
		BindingSet:        r.BindingSet,
		Manifest:          r.Manifest,
		Conformance:       r.Conformance,
		ToolSurface:       r.ToolSurface,
		ProfileDigest:     r.ProfileDigest,
		RegistrySnapshot:  r.RegistrySnapshot,
		SemanticDigest:    r.SemanticDigest,
		CurrentnessDigest: r.CurrentnessDigest,
		CheckedUnixNano:   r.CheckedUnixNano,
		ExpiresUnixNano:   r.ExpiresUnixNano,
		ProjectionDigest:  r.ProjectionDigest,
	}
}

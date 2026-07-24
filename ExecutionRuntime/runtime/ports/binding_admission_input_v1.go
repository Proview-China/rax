package ports

import (
	"context"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type BindingAdmissionPlanCurrentV1 struct {
	ContractVersion  string            `json:"contract_version"`
	Ref              OwnerCurrentRefV1 `json:"ref"`
	Plan             BindingPlanV2     `json:"plan"`
	CheckedUnixNano  int64             `json:"checked_unix_nano"`
	ExpiresUnixNano  int64             `json:"expires_unix_nano"`
	ProjectionDigest core.Digest       `json:"projection_digest"`
}

func (p BindingAdmissionPlanCurrentV1) DigestV1() (core.Digest, error) {
	c := p
	c.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.binding-admission-input", BindingAdmissionContractVersionV1, "BindingAdmissionPlanCurrentV1", c)
}

func SealBindingAdmissionPlanCurrentV1(p BindingAdmissionPlanCurrentV1) (BindingAdmissionPlanCurrentV1, error) {
	p.ContractVersion = BindingAdmissionContractVersionV1
	provided := p.ProjectionDigest
	p.ProjectionDigest = ""
	d, err := p.DigestV1()
	if err != nil {
		return BindingAdmissionPlanCurrentV1{}, err
	}
	if provided != "" && provided != d {
		return BindingAdmissionPlanCurrentV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Binding admission Plan current supplied a wrong digest")
	}
	p.ProjectionDigest = d
	return p, p.Validate()
}

func (p BindingAdmissionPlanCurrentV1) Validate() error {
	if p.ContractVersion != BindingAdmissionContractVersionV1 || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.Ref.ExpiresUnixNano != p.ExpiresUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Binding admission Plan current is incomplete")
	}
	if err := p.Ref.Validate(); err != nil {
		return err
	}
	if err := p.Plan.Validate(); err != nil {
		return err
	}
	d, err := p.DigestV1()
	if err != nil || d != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Binding admission Plan current digest drifted")
	}
	return nil
}

type BindingAdmissionAssemblyCurrentV1 struct {
	ContractVersion  string                `json:"contract_version"`
	Ref              OwnerCurrentRefV1     `json:"ref"`
	Manifests        []ComponentManifestV2 `json:"manifests"`
	CheckedUnixNano  int64                 `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                 `json:"expires_unix_nano"`
	ProjectionDigest core.Digest           `json:"projection_digest"`
}

func (p BindingAdmissionAssemblyCurrentV1) canonicalV1() BindingAdmissionAssemblyCurrentV1 {
	c := p
	c.Manifests = append([]ComponentManifestV2{}, p.Manifests...)
	sort.Slice(c.Manifests, func(i, j int) bool { return c.Manifests[i].ComponentID < c.Manifests[j].ComponentID })
	return c
}

func (p BindingAdmissionAssemblyCurrentV1) DigestV1() (core.Digest, error) {
	c := p.canonicalV1()
	c.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.binding-admission-input", BindingAdmissionContractVersionV1, "BindingAdmissionAssemblyCurrentV1", c)
}

func SealBindingAdmissionAssemblyCurrentV1(p BindingAdmissionAssemblyCurrentV1) (BindingAdmissionAssemblyCurrentV1, error) {
	p.ContractVersion = BindingAdmissionContractVersionV1
	p = p.canonicalV1()
	provided := p.ProjectionDigest
	p.ProjectionDigest = ""
	d, err := p.DigestV1()
	if err != nil {
		return BindingAdmissionAssemblyCurrentV1{}, err
	}
	if provided != "" && provided != d {
		return BindingAdmissionAssemblyCurrentV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Binding admission Assembly current supplied a wrong digest")
	}
	p.ProjectionDigest = d
	return p, p.Validate()
}

func (p BindingAdmissionAssemblyCurrentV1) Validate() error {
	if p.ContractVersion != BindingAdmissionContractVersionV1 || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.Ref.ExpiresUnixNano != p.ExpiresUnixNano || len(p.Manifests) == 0 || len(p.Manifests) > MaxBindingAdmissionReleasesV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Binding admission Assembly current is incomplete")
	}
	if err := p.Ref.Validate(); err != nil {
		return err
	}
	for i, manifest := range p.Manifests {
		if err := manifest.Validate(); err != nil {
			return err
		}
		if i > 0 && p.Manifests[i-1].ComponentID >= manifest.ComponentID {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "Binding admission Assembly manifests must be canonical")
		}
	}
	d, err := p.DigestV1()
	if err != nil || d != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Binding admission Assembly current digest drifted")
	}
	return nil
}

type BindingAdmissionCatalogCurrentV1 struct {
	ContractVersion  string              `json:"contract_version"`
	Ref              OwnerCurrentRefV1   `json:"ref"`
	Catalog          GovernanceCatalogV2 `json:"catalog"`
	CheckedUnixNano  int64               `json:"checked_unix_nano"`
	ExpiresUnixNano  int64               `json:"expires_unix_nano"`
	ProjectionDigest core.Digest         `json:"projection_digest"`
}

func (p BindingAdmissionCatalogCurrentV1) DigestV1() (core.Digest, error) {
	c := p
	c.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.binding-admission-input", BindingAdmissionContractVersionV1, "BindingAdmissionCatalogCurrentV1", c)
}

func SealBindingAdmissionCatalogCurrentV1(p BindingAdmissionCatalogCurrentV1) (BindingAdmissionCatalogCurrentV1, error) {
	p.ContractVersion = BindingAdmissionContractVersionV1
	provided := p.ProjectionDigest
	p.ProjectionDigest = ""
	d, err := p.DigestV1()
	if err != nil {
		return BindingAdmissionCatalogCurrentV1{}, err
	}
	if provided != "" && provided != d {
		return BindingAdmissionCatalogCurrentV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Binding admission Catalog current supplied a wrong digest")
	}
	p.ProjectionDigest = d
	return p, p.Validate()
}

func (p BindingAdmissionCatalogCurrentV1) Validate() error {
	if p.ContractVersion != BindingAdmissionContractVersionV1 || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.Ref.ExpiresUnixNano != p.ExpiresUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Binding admission Catalog current is incomplete")
	}
	if err := p.Ref.Validate(); err != nil {
		return err
	}
	if err := p.Catalog.Validate(); err != nil {
		return err
	}
	d, err := p.DigestV1()
	if err != nil || d != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Binding admission Catalog current digest drifted")
	}
	return nil
}

type BindingAdmissionReleaseCurrentV1 struct {
	ContractVersion           string                       `json:"contract_version"`
	Expected                  PreBindingComponentReleaseV1 `json:"expected"`
	ManifestDigest            core.Digest                  `json:"manifest_digest"`
	Grants                    []CapabilityGrantV2          `json:"grants"`
	ConformanceEvidenceDigest core.Digest                  `json:"conformance_evidence_digest"`
	CheckedUnixNano           int64                        `json:"checked_unix_nano"`
	ExpiresUnixNano           int64                        `json:"expires_unix_nano"`
	ProjectionDigest          core.Digest                  `json:"projection_digest"`
}

func (p BindingAdmissionReleaseCurrentV1) canonicalV1() BindingAdmissionReleaseCurrentV1 {
	c := p
	c.Grants = append([]CapabilityGrantV2{}, p.Grants...)
	sort.Slice(c.Grants, func(i, j int) bool { return c.Grants[i].Capability < c.Grants[j].Capability })
	return c
}

func (p BindingAdmissionReleaseCurrentV1) DigestV1() (core.Digest, error) {
	c := p.canonicalV1()
	c.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.binding-admission-input", BindingAdmissionContractVersionV1, "BindingAdmissionReleaseCurrentV1", c)
}

func SealBindingAdmissionReleaseCurrentV1(p BindingAdmissionReleaseCurrentV1) (BindingAdmissionReleaseCurrentV1, error) {
	p.ContractVersion = BindingAdmissionContractVersionV1
	p = p.canonicalV1()
	provided := p.ProjectionDigest
	p.ProjectionDigest = ""
	d, err := p.DigestV1()
	if err != nil {
		return BindingAdmissionReleaseCurrentV1{}, err
	}
	if provided != "" && provided != d {
		return BindingAdmissionReleaseCurrentV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Binding admission Release current supplied a wrong digest")
	}
	p.ProjectionDigest = d
	return p, p.Validate()
}

func (p BindingAdmissionReleaseCurrentV1) Validate() error {
	if p.ContractVersion != BindingAdmissionContractVersionV1 || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || len(p.Grants) == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Binding admission Release current is incomplete")
	}
	if err := p.Expected.Validate(); err != nil {
		return err
	}
	if err := p.ManifestDigest.Validate(); err != nil {
		return err
	}
	if err := p.ConformanceEvidenceDigest.Validate(); err != nil {
		return err
	}
	if err := ValidateCapabilityGrantStructureV2(p.Grants); err != nil {
		return err
	}
	minimum := p.Expected.Release.ExpiresUnixNano
	for _, expiry := range []int64{p.Expected.Certification.ExpiresUnixNano, p.Expected.DeploymentReadiness.ExpiresUnixNano} {
		if expiry < minimum {
			minimum = expiry
		}
	}
	for _, grant := range p.Grants {
		if grant.ExpiresUnixNano < minimum {
			minimum = grant.ExpiresUnixNano
		}
	}
	if p.ExpiresUnixNano > minimum {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Binding admission Release current exceeds an owner or grant TTL")
	}
	d, err := p.DigestV1()
	if err != nil || d != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Binding admission Release current digest drifted")
	}
	return nil
}

func ValidateBindingAdmissionProjectionCurrentV1(checked, expires int64, now time.Time) error {
	if now.IsZero() || now.UnixNano() < checked {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Binding admission input current clock regressed")
	}
	if !now.Before(time.Unix(0, expires)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Binding admission input current expired")
	}
	return nil
}

// BindingAdmissionInputCurrentReaderV1 is nominal by position. Implementations
// aggregate owner reads but do not become another semantic fact owner.
type BindingAdmissionInputCurrentReaderV1 interface {
	InspectBindingAdmissionDefinitionCurrentV1(context.Context, OwnerCurrentRefV1) (OwnerCurrentRefV1, error)
	InspectBindingAdmissionPlanCurrentV1(context.Context, OwnerCurrentRefV1) (BindingAdmissionPlanCurrentV1, error)
	InspectBindingAdmissionAssemblyCurrentV1(context.Context, OwnerCurrentRefV1) (BindingAdmissionAssemblyCurrentV1, error)
	InspectBindingAdmissionCatalogCurrentV1(context.Context, OwnerCurrentRefV1) (BindingAdmissionCatalogCurrentV1, error)
	InspectBindingAdmissionResolutionCurrentV1(context.Context, OwnerCurrentRefV1) (OwnerCurrentRefV1, error)
	InspectBindingAdmissionReleaseCurrentV1(context.Context, PreBindingComponentReleaseV1) (BindingAdmissionReleaseCurrentV1, error)
	InspectBindingAdmissionResourceBindingSetCurrentV1(context.Context, ResourceBindingSetRefV1) (ResourceBindingSetV1, error)
	InspectBindingAdmissionAuthorityCurrentV1(context.Context, OwnerCurrentRefV1) (OwnerCurrentRefV1, error)
	InspectBindingAdmissionPolicyCurrentV1(context.Context, OwnerCurrentRefV1) (OwnerCurrentRefV1, error)
}

package contract

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const ToolSurfaceManifestCurrentContractVersionV1 = "praxis.tool-mcp.surface-manifest-current/v1"

const toolSurfaceManifestCurrentCanonicalDomainV1 = "praxis.tool-mcp.surface-manifest-current"

type ToolSurfaceManifestCurrentRefV1 struct {
	ContractVersion string        `json:"contract_version"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
}

func (r ToolSurfaceManifestCurrentRefV1) Validate() error {
	if r.ContractVersion != ToolSurfaceManifestCurrentContractVersionV1 || ValidateStableID(r.ID) != nil || r.Revision == 0 {
		return invalid("Tool Surface Manifest current Ref is invalid")
	}
	return r.Digest.Validate()
}

type ToolSurfaceManifestCurrentEnsureRequestV1 struct {
	ContractVersion string                          `json:"contract_version"`
	Manifest        ToolSurfaceManifest             `json:"manifest"`
	ExpectedCurrent ToolSurfaceManifestCurrentRefV1 `json:"expected_current"`
}

func (r ToolSurfaceManifestCurrentEnsureRequestV1) Validate() error {
	if r.ContractVersion != ToolSurfaceManifestCurrentContractVersionV1 {
		return invalid("Tool Surface Manifest current Ensure contract version is invalid")
	}
	if err := r.Manifest.Validate(); err != nil {
		return err
	}
	if r.Manifest.Revision == 1 {
		if r.ExpectedCurrent != (ToolSurfaceManifestCurrentRefV1{}) {
			return conflict("initial Tool Surface Manifest current requires an empty ExpectedCurrent")
		}
		return nil
	}
	if err := r.ExpectedCurrent.Validate(); err != nil {
		return err
	}
	if r.ExpectedCurrent.ID != r.Manifest.ID || r.Manifest.Revision != r.ExpectedCurrent.Revision+1 {
		return conflict("Tool Surface Manifest successor does not extend ExpectedCurrent exactly")
	}
	return nil
}

type ToolSurfaceManifestCurrentProjectionV1 struct {
	ContractVersion  string                          `json:"contract_version"`
	Ref              ToolSurfaceManifestCurrentRefV1 `json:"ref"`
	Manifest         ToolSurfaceManifest             `json:"manifest"`
	Owner            core.OwnerRef                   `json:"owner"`
	CheckedUnixNano  int64                           `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                           `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                     `json:"projection_digest"`
}

func (p ToolSurfaceManifestCurrentProjectionV1) Validate() error {
	if p.ContractVersion != ToolSurfaceManifestCurrentContractVersionV1 {
		return invalid("Tool Surface Manifest current Projection contract version is invalid")
	}
	if err := p.Ref.Validate(); err != nil {
		return err
	}
	if err := p.Manifest.Validate(); err != nil {
		return err
	}
	if err := p.Owner.Validate(); err != nil {
		return err
	}
	if p.Ref.ContractVersion != p.ContractVersion || p.Ref.ID != p.Manifest.ID || p.Ref.Revision != p.Manifest.Revision || p.Ref.Digest != p.Manifest.Digest || p.Owner != p.Manifest.Owner || p.ExpiresUnixNano != p.Manifest.ExpiresUnixNano {
		return conflict("Tool Surface Manifest current duplicate fields drifted")
	}
	if p.CheckedUnixNano <= 0 || p.CheckedUnixNano < p.Manifest.CreatedUnixNano || p.CheckedUnixNano >= p.ExpiresUnixNano {
		return invalid("Tool Surface Manifest current time bounds are invalid")
	}
	if err := p.ProjectionDigest.Validate(); err != nil {
		return err
	}
	digest, err := digestToolSurfaceManifestCurrentProjectionV1(p)
	if err != nil || digest != p.ProjectionDigest {
		return conflict("Tool Surface Manifest current Projection digest drifted")
	}
	return nil
}

func (p ToolSurfaceManifestCurrentProjectionV1) ValidateCurrent(expected ToolSurfaceManifestCurrentRefV1, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if err := expected.Validate(); err != nil {
		return err
	}
	if p.Ref != expected {
		return conflict("Tool Surface Manifest current Ref drifted")
	}
	if now.IsZero() {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Surface Manifest current validation requires fresh time")
	}
	if now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Surface Manifest current clock regressed")
	}
	if !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Tool Surface Manifest current expired")
	}
	return nil
}

func SealToolSurfaceManifestCurrentV1(p ToolSurfaceManifestCurrentProjectionV1) (ToolSurfaceManifestCurrentProjectionV1, error) {
	p = cloneToolSurfaceManifestCurrentProjectionV1(p)
	if p.ContractVersion != "" && p.ContractVersion != ToolSurfaceManifestCurrentContractVersionV1 {
		return ToolSurfaceManifestCurrentProjectionV1{}, invalid("Tool Surface Manifest current Projection contract version drifted")
	}
	p.ContractVersion = ToolSurfaceManifestCurrentContractVersionV1
	if err := p.Manifest.Validate(); err != nil {
		return ToolSurfaceManifestCurrentProjectionV1{}, err
	}
	expectedRef := ToolSurfaceManifestCurrentRefV1{
		ContractVersion: ToolSurfaceManifestCurrentContractVersionV1,
		ID:              p.Manifest.ID, Revision: p.Manifest.Revision, Digest: p.Manifest.Digest,
	}
	if p.Ref != (ToolSurfaceManifestCurrentRefV1{}) && p.Ref != expectedRef {
		return ToolSurfaceManifestCurrentProjectionV1{}, conflict("supplied Tool Surface Manifest current Ref drifted")
	}
	p.Ref = expectedRef
	if p.Owner != (core.OwnerRef{}) && p.Owner != p.Manifest.Owner {
		return ToolSurfaceManifestCurrentProjectionV1{}, conflict("supplied Tool Surface Manifest current Owner drifted")
	}
	p.Owner = p.Manifest.Owner
	if p.ExpiresUnixNano != 0 && p.ExpiresUnixNano != p.Manifest.ExpiresUnixNano {
		return ToolSurfaceManifestCurrentProjectionV1{}, conflict("supplied Tool Surface Manifest current expiry drifted")
	}
	p.ExpiresUnixNano = p.Manifest.ExpiresUnixNano
	provided := p.ProjectionDigest
	p.ProjectionDigest = ""
	digest, err := digestToolSurfaceManifestCurrentProjectionV1(p)
	if err != nil {
		return ToolSurfaceManifestCurrentProjectionV1{}, err
	}
	if provided != "" && provided != digest {
		return ToolSurfaceManifestCurrentProjectionV1{}, conflict("supplied Tool Surface Manifest current Projection digest drifted")
	}
	p.ProjectionDigest = digest
	return p, p.Validate()
}

type ToolSurfaceManifestCurrentReaderV1 interface {
	InspectExactToolSurfaceManifestCurrentV1(context.Context, ToolSurfaceManifestCurrentRefV1) (ToolSurfaceManifestCurrentProjectionV1, error)
}

type ToolSurfaceManifestCurrentRepositoryV1 interface {
	ToolSurfaceManifestCurrentReaderV1
	EnsureExactToolSurfaceManifestCurrentV1(context.Context, ToolSurfaceManifestCurrentEnsureRequestV1) (ToolSurfaceManifestCurrentProjectionV1, error)
}

func digestToolSurfaceManifestCurrentProjectionV1(p ToolSurfaceManifestCurrentProjectionV1) (core.Digest, error) {
	p = cloneToolSurfaceManifestCurrentProjectionV1(p)
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest(toolSurfaceManifestCurrentCanonicalDomainV1, ToolSurfaceManifestCurrentContractVersionV1, "ToolSurfaceManifestCurrentProjectionV1", p)
}

func cloneToolSurfaceManifestV1(manifest ToolSurfaceManifest) ToolSurfaceManifest {
	manifest.Entries = append([]ToolSurfaceEntry(nil), manifest.Entries...)
	for index := range manifest.Entries {
		manifest.Entries[index].EffectKinds = append([]runtimeports.NamespacedNameV2(nil), manifest.Entries[index].EffectKinds...)
	}
	manifest.Residuals = append([]Residual(nil), manifest.Residuals...)
	return manifest
}

func cloneToolSurfaceManifestCurrentProjectionV1(p ToolSurfaceManifestCurrentProjectionV1) ToolSurfaceManifestCurrentProjectionV1 {
	p.Manifest = cloneToolSurfaceManifestV1(p.Manifest)
	return p
}

package contract

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const ModelToolInjectionPairCurrentContractVersionV2 = "praxis.tool-mcp.model-tool-injection-pair-current/v2"

const modelToolInjectionPairCurrentCanonicalDomainV2 = "praxis.tool-mcp.model-tool-injection-pair-current"

// ModelToolInjectionPairCurrentProjectionV2 is the Tool-owned, Model-neutral
// proof that one exact injection material and one exact current Surface close
// both the stored CompiledModelTools bytes and the actual Model request tools.
// StoredCompiledToolsDigest and ActualCompiledToolsDigest deliberately remain
// separate coordinates even though a valid projection requires equality.
type ModelToolInjectionPairCurrentProjectionV2 struct {
	ContractVersion string `json:"contract_version"`

	MaterialSource ModelToolInjectionLineageSourceRefV1 `json:"material_source"`
	SurfaceSource  ModelToolInjectionLineageSourceRefV1 `json:"surface_source"`

	ExpectedInjectionDigest   core.Digest `json:"expected_injection_digest"`
	StoredCompiledToolsDigest core.Digest `json:"stored_compiled_tools_digest"`
	ActualCompiledToolsDigest core.Digest `json:"actual_compiled_tools_digest"`
	RequestToolsDigest        core.Digest `json:"request_tools_digest"`

	CheckedUnixNano  int64       `json:"checked_unix_nano"`
	ExpiresUnixNano  int64       `json:"expires_unix_nano"`
	ProjectionDigest core.Digest `json:"projection_digest"`
}

func (p ModelToolInjectionPairCurrentProjectionV2) Validate() error {
	if p.ContractVersion != ModelToolInjectionPairCurrentContractVersionV2 {
		return invalid("Model Tool Injection pair current contract version is invalid")
	}
	if p.MaterialSource.Validate() != nil || p.SurfaceSource.Validate() != nil {
		return invalid("Model Tool Injection pair current exact source is invalid")
	}
	if p.MaterialSource.Kind != ModelToolInjectionMaterialSourceKindV1 ||
		p.SurfaceSource.Kind != ToolSurfaceManifestCurrentSourceKindV1 {
		return conflict("Model Tool Injection pair current source role drifted")
	}
	if p.MaterialSource.Owner != p.SurfaceSource.Owner ||
		p.MaterialSource == p.SurfaceSource {
		return conflict("Model Tool Injection pair current source Owner or role collapsed")
	}
	for _, digest := range []core.Digest{
		p.ExpectedInjectionDigest,
		p.StoredCompiledToolsDigest,
		p.ActualCompiledToolsDigest,
		p.RequestToolsDigest,
		p.ProjectionDigest,
	} {
		if digest.Validate() != nil {
			return invalid("Model Tool Injection pair current digest is invalid")
		}
	}
	if p.StoredCompiledToolsDigest != p.ActualCompiledToolsDigest {
		return conflict("stored and actual Compiled Model Tools digests differ")
	}
	if err := p.validateDigestRolesV2(); err != nil {
		return err
	}
	if p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano {
		return invalid("Model Tool Injection pair current window is invalid")
	}
	expected, err := modelToolInjectionPairCurrentDigestV2(p)
	if err != nil || expected != p.ProjectionDigest {
		return conflict("Model Tool Injection pair current Projection digest drifted")
	}
	return nil
}

func (p ModelToolInjectionPairCurrentProjectionV2) validateDigestRolesV2() error {
	// Stored and actual Compiled Tools are two observations of one semantic
	// role and were checked equal by Validate. Every other digest is an exact
	// coordinate in a different canonical domain and must not substitute for
	// another role, even when an attacker recomputes ProjectionDigest.
	roles := []struct {
		name   string
		digest core.Digest
	}{
		{name: "Material source", digest: p.MaterialSource.Digest},
		{name: "Surface source", digest: p.SurfaceSource.Digest},
		{name: "expected injection", digest: p.ExpectedInjectionDigest},
		{name: "Compiled Tools", digest: p.StoredCompiledToolsDigest},
		{name: "request Tools", digest: p.RequestToolsDigest},
		{name: "pair Projection", digest: p.ProjectionDigest},
	}
	for left := range roles {
		for right := left + 1; right < len(roles); right++ {
			if roles[left].digest == roles[right].digest {
				return conflict("Model Tool Injection pair current digest roles collapsed: " +
					roles[left].name + " and " + roles[right].name)
			}
		}
	}
	return nil
}

func (p ModelToolInjectionPairCurrentProjectionV2) ValidateCurrent(
	material ModelToolInjectionLineageSourceRefV1,
	surface ModelToolInjectionLineageSourceRefV1,
	requestToolsDigest core.Digest,
	now time.Time,
) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if material.Validate() != nil || surface.Validate() != nil ||
		requestToolsDigest.Validate() != nil ||
		p.MaterialSource != material ||
		p.SurfaceSource != surface ||
		p.RequestToolsDigest != requestToolsDigest {
		return conflict("Model Tool Injection pair current exact binding drifted")
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Model Tool Injection pair current clock regressed")
	}
	if !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Model Tool Injection pair current expired")
	}
	return nil
}

func SealModelToolInjectionPairCurrentV2(
	p ModelToolInjectionPairCurrentProjectionV2,
) (ModelToolInjectionPairCurrentProjectionV2, error) {
	if p.ContractVersion != "" &&
		p.ContractVersion != ModelToolInjectionPairCurrentContractVersionV2 {
		return ModelToolInjectionPairCurrentProjectionV2{}, invalid("Model Tool Injection pair current contract version drifted")
	}
	p.ContractVersion = ModelToolInjectionPairCurrentContractVersionV2
	provided := p.ProjectionDigest
	p.ProjectionDigest = ""
	digest, err := modelToolInjectionPairCurrentDigestV2(p)
	if err != nil {
		return ModelToolInjectionPairCurrentProjectionV2{}, err
	}
	if provided != "" && provided != digest {
		return ModelToolInjectionPairCurrentProjectionV2{}, conflict("supplied Model Tool Injection pair current Projection digest drifted")
	}
	p.ProjectionDigest = digest
	if err := p.Validate(); err != nil {
		return ModelToolInjectionPairCurrentProjectionV2{}, err
	}
	return p, nil
}

func modelToolInjectionPairCurrentDigestV2(
	p ModelToolInjectionPairCurrentProjectionV2,
) (core.Digest, error) {
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest(
		modelToolInjectionPairCurrentCanonicalDomainV2,
		ModelToolInjectionPairCurrentContractVersionV2,
		"ModelToolInjectionPairCurrentProjectionV2",
		p,
	)
}

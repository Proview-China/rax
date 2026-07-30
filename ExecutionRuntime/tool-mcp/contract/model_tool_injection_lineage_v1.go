package contract

import (
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	ModelToolInjectionMaterialSourceKindV1 runtimeports.NamespacedNameV2 = "praxis.tool/model-tool-injection-material"
	ToolSurfaceManifestCurrentSourceKindV1 runtimeports.NamespacedNameV2 = "praxis.tool/surface-manifest-current"
)

// ModelToolInjectionLineageSourceRefV1 is a Tool-owned exact-source
// coordinate. Digest always identifies the referenced owner fact. In
// particular, ExpectedInjectionDigest must never be substituted for Digest.
type ModelToolInjectionLineageSourceRefV1 struct {
	Owner    core.OwnerRef                 `json:"owner"`
	Kind     runtimeports.NamespacedNameV2 `json:"kind"`
	ID       string                        `json:"id"`
	Revision core.Revision                 `json:"revision"`
	Digest   core.Digest                   `json:"digest"`
}

func (r ModelToolInjectionLineageSourceRefV1) Validate() error {
	if err := r.Owner.Validate(); err != nil {
		return err
	}
	if r.Kind != ModelToolInjectionMaterialSourceKindV1 &&
		r.Kind != ToolSurfaceManifestCurrentSourceKindV1 {
		return invalid("Model Tool Injection lineage source Kind is invalid")
	}
	if strings.TrimSpace(r.ID) == "" || r.Revision == 0 {
		return invalid("Model Tool Injection lineage source identity is invalid")
	}
	return r.Digest.Validate()
}

func ModelToolInjectionMaterialSourceRefV1(
	owner core.OwnerRef,
	ref ModelToolInjectionMaterialRefV1,
) (ModelToolInjectionLineageSourceRefV1, error) {
	source := ModelToolInjectionLineageSourceRefV1{
		Owner: owner, Kind: ModelToolInjectionMaterialSourceKindV1,
		ID: ref.ID, Revision: ref.Revision, Digest: ref.Digest,
	}
	return source, source.Validate()
}

func ToolSurfaceManifestSourceRefV1(
	projection ToolSurfaceManifestCurrentProjectionV1,
) (ModelToolInjectionLineageSourceRefV1, error) {
	source := ModelToolInjectionLineageSourceRefV1{
		Owner: projection.Owner, Kind: ToolSurfaceManifestCurrentSourceKindV1,
		ID: projection.Ref.ID, Revision: projection.Ref.Revision, Digest: projection.Ref.Digest,
	}
	return source, source.Validate()
}

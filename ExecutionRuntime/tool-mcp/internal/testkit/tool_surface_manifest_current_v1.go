package testkit

import (
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type ManualClock struct {
	mu  sync.Mutex
	now time.Time
}

func NewManualClock(now time.Time) *ManualClock { return &ManualClock{now: now} }

func (c *ManualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *ManualClock) Set(now time.Time) {
	c.mu.Lock()
	c.now = now
	c.mu.Unlock()
}

type SequenceClock struct {
	mu     sync.Mutex
	values []time.Time
	index  int
}

func NewSequenceClock(values ...time.Time) *SequenceClock {
	return &SequenceClock{values: append([]time.Time(nil), values...)}
}

func (c *SequenceClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.values) == 0 {
		return time.Time{}
	}
	index := c.index
	if index >= len(c.values) {
		index = len(c.values) - 1
	} else {
		c.index++
	}
	return c.values[index]
}

func ToolSurfaceManifestV1(revision core.Revision) contract.ToolSurfaceManifest {
	capability, tool := Capability(), Tool()
	manifest, err := contract.SealSurface(contract.ToolSurfaceManifest{
		ID:                     "surface_example",
		Revision:               revision,
		Owner:                  Owner(),
		ResolvedPlanDigest:     Digest("surface-plan"),
		ProfileDigest:          Digest("surface-profile"),
		CapabilityGrantDigest:  Digest("surface-grant"),
		RegistrySnapshotDigest: Digest("surface-registry"),
		Entries: []contract.ToolSurfaceEntry{{
			Capability: contract.ObjectRef{ID: string(capability.ID), Revision: capability.Revision, Digest: capability.Digest},
			Tool:       contract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest},
			ModelName:  "tool.example", InputSchema: tool.InputSchema, DescriptionDigest: Digest("surface-description"),
			Visibility: contract.SurfaceVisible, Allowed: true, Admission: contract.AdmissionRequired,
			MechanismDigest: Digest("surface-mechanism"), EffectKinds: []runtimeports.NamespacedNameV2{"praxis.tool/execute"},
		}},
		Dialect:         "model/default",
		Residuals:       []contract.Residual{{Class: runtimeports.ResidualInspectable, Code: "praxis.tool/test-residual", Inspectable: true, Detail: "fixture"}},
		CreatedUnixNano: FixedTime.Add(-time.Minute).UnixNano(),
		ExpiresUnixNano: FixedTime.Add(time.Hour).UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	return manifest
}

func ToolSurfaceManifestCurrentRequestV1(revision core.Revision) contract.ToolSurfaceManifestCurrentEnsureRequestV1 {
	return contract.ToolSurfaceManifestCurrentEnsureRequestV1{
		ContractVersion: contract.ToolSurfaceManifestCurrentContractVersionV1,
		Manifest:        ToolSurfaceManifestV1(revision),
	}
}

func ToolSurfaceManifestSuccessorRequestV1(current contract.ToolSurfaceManifestCurrentProjectionV1) contract.ToolSurfaceManifestCurrentEnsureRequestV1 {
	manifest := CloneToolSurfaceManifestV1(current.Manifest)
	manifest.Revision = current.Ref.Revision + 1
	manifest.ProfileDigest = Digest("surface-profile-successor")
	manifest.Digest = ""
	sealed, err := contract.SealSurface(manifest)
	if err != nil {
		panic(err)
	}
	return contract.ToolSurfaceManifestCurrentEnsureRequestV1{
		ContractVersion: contract.ToolSurfaceManifestCurrentContractVersionV1,
		Manifest:        sealed,
		ExpectedCurrent: current.Ref,
	}
}

func ToolSurfaceManifestCurrentProjectionV1(revision core.Revision) contract.ToolSurfaceManifestCurrentProjectionV1 {
	manifest := ToolSurfaceManifestV1(revision)
	projection, err := contract.SealToolSurfaceManifestCurrentV1(contract.ToolSurfaceManifestCurrentProjectionV1{
		ContractVersion: contract.ToolSurfaceManifestCurrentContractVersionV1,
		Ref: contract.ToolSurfaceManifestCurrentRefV1{
			ContractVersion: contract.ToolSurfaceManifestCurrentContractVersionV1,
			ID:              manifest.ID, Revision: manifest.Revision, Digest: manifest.Digest,
		},
		Manifest: manifest, Owner: manifest.Owner,
		CheckedUnixNano: FixedTime.UnixNano(), ExpiresUnixNano: manifest.ExpiresUnixNano,
	})
	if err != nil {
		panic(err)
	}
	return projection
}

func CloneToolSurfaceManifestV1(manifest contract.ToolSurfaceManifest) contract.ToolSurfaceManifest {
	manifest.Entries = append([]contract.ToolSurfaceEntry(nil), manifest.Entries...)
	for i := range manifest.Entries {
		manifest.Entries[i].EffectKinds = append([]runtimeports.NamespacedNameV2(nil), manifest.Entries[i].EffectKinds...)
	}
	manifest.Residuals = append([]contract.Residual(nil), manifest.Residuals...)
	return manifest
}

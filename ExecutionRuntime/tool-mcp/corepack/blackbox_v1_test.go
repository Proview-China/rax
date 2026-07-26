package corepack_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/corepack"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
)

func TestPublicCorePackCatalogRegistrySurfaceFlow(t *testing.T) {
	now := time.Unix(1000, 0)
	owner := core.OwnerRef{Domain: "praxis.tool-mcp", ID: "core-pack"}
	catalog, err := corepack.BuildCatalogV1(corepack.CatalogConfigV1{
		Owner: owner, ArtifactDigest: core.DigestBytes([]byte("artifact")),
		SignatureDigest:  core.DigestBytes([]byte("signature")),
		ProvenanceDigest: core.DigestBytes([]byte("provenance")), CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	target := registry.New()
	if _, err := corepack.RegisterV1(target, catalog, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	surface, err := corepack.BuildSurfaceV1(catalog, corepack.SurfaceConfigV1{
		ID: "public-core-surface-v1", Owner: owner,
		ResolvedPlanDigest: core.DigestBytes([]byte("plan")), ProfileDigest: core.DigestBytes([]byte("profile")),
		CapabilityGrantDigest: core.DigestBytes([]byte("grant")), RegistrySnapshotDigest: core.DigestBytes([]byte("registry")),
		CreatedAt: now.Add(2 * time.Second), ExpiresAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(surface.Entries) != 5 {
		t.Fatalf("got %d surface entries", len(surface.Entries))
	}
	for _, entry := range surface.Entries {
		if entry.Allowed && entry.Tool.ID == "" {
			t.Fatal("allowed entry lacks exact Tool")
		}
	}
}

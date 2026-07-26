package corepack

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

func previewAssemblyFixtureV1(t *testing.T) (*CorePackAssemblyKitV1, CorePackAssemblyRequestV1) {
	t.Helper()
	now := time.Unix(1_000, 0).UTC()
	target := registry.New()
	client, err := sdk.NewV1(target, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	kit, err := NewCorePackAssemblyKitV1(target, client, nil, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	owner := core.OwnerRef{Domain: "praxis.tool-mcp", ID: "core-pack"}
	request := CorePackAssemblyRequestV1{
		ContractVersion: CorePackAssemblyContractVersionV1,
		Mode:            CorePackAssemblyPreviewV1,
		Catalog: CatalogConfigV1{
			Owner: owner, ArtifactDigest: core.DigestBytes([]byte("artifact")),
			SignatureDigest: core.DigestBytes([]byte("signature")), ProvenanceDigest: core.DigestBytes([]byte("provenance")), CreatedAt: now,
		},
		Surface: SurfaceConfigV1{
			ID: "core-tool-surface-v1", Owner: owner,
			ResolvedPlanDigest: core.DigestBytes([]byte("plan")), ProfileDigest: core.DigestBytes([]byte("profile")),
			CapabilityGrantDigest: core.DigestBytes([]byte("grant")), RegistrySnapshotDigest: core.DigestBytes([]byte("placeholder")),
			CreatedAt: now, ExpiresAt: now.Add(time.Hour),
		},
		RequestedExpiresUnixNano: now.Add(time.Hour).UnixNano(),
	}
	return kit, request
}

func TestCorePackAssemblyPreviewV1(t *testing.T) {
	kit, request := previewAssemblyFixtureV1(t)
	factory, err := NewCorePackAssemblyFactoryV1(kit)
	if err != nil {
		t.Fatal(err)
	}
	first, err := factory.BuildV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := kit.AssembleV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest != second.Digest || first.RegistrySnapshot != second.RegistrySnapshot {
		t.Fatalf("preview is not deterministic: %s != %s", first.Digest, second.Digest)
	}
	if !first.ReferenceOnly || first.Admitted || first.Executable || first.PackageRecord.State != registry.StateSubmitted || first.PackageAssembly != nil {
		t.Fatalf("preview authority drifted: %+v", first)
	}
	if first.Surface.RegistrySnapshotDigest != first.RegistrySnapshot.Digest || len(first.Surface.Entries) != 5 {
		t.Fatal("preview Surface is not bound to the exact Registry Snapshot")
	}
	clone := first.Clone()
	clone.Catalog.Materials[0].InputSchema[0] = '!'
	clone.Catalog.Capabilities[0].EffectKinds[0] = "changed/capability"
	clone.Catalog.Tools[0].EffectKinds[0] = "changed/tool"
	clone.Surface.Entries[0].EffectKinds[0] = "changed/effect"
	if first.Catalog.Materials[0].InputSchema[0] == '!' || first.Catalog.Capabilities[0].EffectKinds[0] == "changed/capability" || first.Catalog.Tools[0].EffectKinds[0] == "changed/tool" || first.Surface.Entries[0].EffectKinds[0] == "changed/effect" {
		t.Fatal("result clone aliases mutable slices")
	}
}

func TestCorePackAssemblyPreviewConcurrentV1(t *testing.T) {
	kit, request := previewAssemblyFixtureV1(t)
	const workers = 64
	results := make(chan CorePackAssemblyResultV1, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := kit.AssembleV1(context.Background(), request)
			results <- result
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var digest core.Digest
	for result := range results {
		if digest == "" {
			digest = result.Digest
		}
		if result.Digest != digest {
			t.Fatal("concurrent preview digest drifted")
		}
	}
}

func TestCorePackAssemblyRejectsInvalidDependenciesCancelAndTTLV1(t *testing.T) {
	var target *registry.Registry
	if _, err := NewCorePackAssemblyKitV1(target, nil, nil, nil); err == nil {
		t.Fatal("typed-nil dependencies were accepted")
	}
	kit, request := previewAssemblyFixtureV1(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := kit.AssembleV1(ctx, request); err != context.Canceled {
		t.Fatalf("cancel=%v", err)
	}
	request.RequestedExpiresUnixNano = request.Surface.CreatedAt.UnixNano()
	if _, err := kit.AssembleV1(context.Background(), request); err == nil {
		t.Fatal("expired request was accepted")
	}
}

func TestCorePackAssemblyAdmittedFailsClosedWithoutVerificationV1(t *testing.T) {
	kit, request := previewAssemblyFixtureV1(t)
	request.Mode = CorePackAssemblyAdmittedV1
	if _, err := kit.AssembleV1(context.Background(), request); err == nil {
		t.Fatal("admitted mode bypassed verification")
	}
}

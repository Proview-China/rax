package api

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/corepack"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

func previewFixtureV1(t *testing.T) (*CorePackPreviewV1, CorePackPreviewConfigV1) {
	t.Helper()
	now := time.Unix(10_000, 0).UTC()
	store := registry.New()
	client, err := sdk.NewV1(store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	kit, err := corepack.NewCorePackAssemblyKitV1(store, client, nil, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	factory, err := corepack.NewCorePackAssemblyFactoryV1(kit)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := NewCorePackPreviewV1(factory, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	config := CorePackPreviewConfigV1{ContractVersion: CorePackPreviewContractVersionV1, Owner: core.OwnerRef{Domain: "praxis.tool-mcp", ID: "core-pack-preview"}, ArtifactDigest: core.DigestBytes([]byte("artifact")), SignatureDigest: core.DigestBytes([]byte("signature")), ProvenanceDigest: core.DigestBytes([]byte("provenance")), SurfaceID: "core-pack-preview-surface-v1", ResolvedPlanDigest: core.DigestBytes([]byte("plan")), ProfileDigest: core.DigestBytes([]byte("profile")), CapabilityGrantDigest: core.DigestBytes([]byte("grant")), CreatedUnixNano: now.UnixNano(), SurfaceExpiresUnixNano: now.Add(time.Hour).UnixNano(), RequestedExpiresUnixNano: now.Add(2 * time.Hour).UnixNano()}
	return preview, config
}

func TestCorePackPreviewV1DeterministicAndExact(t *testing.T) {
	preview, config := previewFixtureV1(t)
	first, err := preview.PreviewV1(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	second, err := preview.PreviewV1(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest != second.Digest || first.AssemblyDigest != second.AssemblyDigest || len(first.Declarations) != 5 {
		t.Fatalf("non-deterministic preview: %+v", first)
	}
	want := []string{contract.CoreToolProcessExecV1, contract.CoreToolWorkspaceInspectV1, contract.CoreToolWorkspacePatchV1, contract.CoreToolWorkspaceReadV1, contract.CoreToolWorkspaceSearchV1}
	for i := range want {
		if first.Declarations[i].ModelName != want[i] || first.Declarations[i].Order != uint32(i) {
			t.Fatalf("declaration[%d]=%+v", i, first.Declarations[i])
		}
	}
	clone := first.Clone()
	clone.Declarations[0].EffectKinds[0] = "forged/effect"
	if first.Declarations[0].EffectKinds[0] == "forged/effect" {
		t.Fatal("clone aliases effects")
	}
	clone.Digest = ""
	digest, err := contract.Seal("praxis.tool-mcp.core-pack-preview", CorePackPreviewContractVersionV1, "CorePackPreviewResultV1", clone)
	if err != nil {
		t.Fatal(err)
	}
	clone.Digest = digest
	if clone.Validate() == nil {
		t.Fatal("forged declaration was accepted after re-seal")
	}
}

func TestCorePackPreviewV1StrictJSONCurrentnessAndCancellation(t *testing.T) {
	preview, config := previewFixtureV1(t)
	payload, _ := json.Marshal(config)
	if _, err := DecodeCorePackPreviewConfigV1(payload); err != nil {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{[]byte(`{}`), append(payload, []byte(` {}`)...), []byte(strings.Replace(string(payload), `"surface_id":`, `"unknown":1,"surface_id":`, 1)), []byte(strings.Replace(string(payload), `"surface_id":`, `"surface_id":"duplicate","surface_id":`, 1)), make([]byte, core.MaxCanonicalDocumentBytes+1)} {
		if _, err := DecodeCorePackPreviewConfigV1(raw); err == nil {
			t.Fatalf("invalid JSON accepted: %d bytes", len(raw))
		}
	}
	expired := config
	expired.SurfaceExpiresUnixNano = config.CreatedUnixNano
	if _, err := preview.PreviewV1(context.Background(), expired); err == nil {
		t.Fatal("expired config accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := preview.PreviewV1(ctx, config); err != context.Canceled {
		t.Fatalf("cancel=%v", err)
	}
	var nilFactory *corepack.CorePackAssemblyFactoryV1
	if _, err := NewCorePackPreviewV1(nilFactory, time.Now); err == nil {
		t.Fatal("typed-nil factory accepted")
	}
}

func TestCorePackPreviewV1Concurrent(t *testing.T) {
	preview, config := previewFixtureV1(t)
	const workers = 64
	var wg sync.WaitGroup
	results := make(chan CorePackPreviewResultV1, workers)
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := preview.PreviewV1(context.Background(), config)
			results <- r
			errs <- e
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
			t.Fatal("concurrent digest drift")
		}
	}
}

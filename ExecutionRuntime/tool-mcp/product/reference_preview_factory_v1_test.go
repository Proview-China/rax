package product_test

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/api"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/product"
)

func productConfigV1(now time.Time) api.CorePackPreviewConfigV1 {
	return api.CorePackPreviewConfigV1{ContractVersion: api.CorePackPreviewContractVersionV1, Owner: core.OwnerRef{Domain: "praxis.tool-mcp", ID: "reference-preview-product"}, ArtifactDigest: core.DigestBytes([]byte("artifact")), SignatureDigest: core.DigestBytes([]byte("signature")), ProvenanceDigest: core.DigestBytes([]byte("provenance")), SurfaceID: "reference-preview-product-surface-v1", ResolvedPlanDigest: core.DigestBytes([]byte("plan")), ProfileDigest: core.DigestBytes([]byte("profile")), CapabilityGrantDigest: core.DigestBytes([]byte("grant")), CreatedUnixNano: now.UnixNano(), SurfaceExpiresUnixNano: now.Add(time.Hour).UnixNano(), RequestedExpiresUnixNano: now.Add(2 * time.Hour).UnixNano()}
}

func TestReferencePreviewFactoryV1BuildsUsableAPIAndCLI(t *testing.T) {
	now := time.Unix(30_000, 0).UTC()
	factory, err := product.NewReferencePreviewFactoryV1(func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := factory.BuildV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	config := productConfigV1(now)
	typed, err := bundle.Preview.PreviewV1(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(config)
	var output bytes.Buffer
	if err = bundle.CLI.RunV1(context.Background(), []string{"tool", "core-pack", "preview", "--config-json", string(payload)}, &output); err != nil {
		t.Fatal(err)
	}
	var projected struct {
		Digest       core.Digest       `json:"digest"`
		Declarations []json.RawMessage `json:"declarations"`
		Executable   bool              `json:"executable"`
	}
	if err = json.Unmarshal(output.Bytes(), &projected); err != nil {
		t.Fatal(err)
	}
	if projected.Digest != typed.Digest || len(projected.Declarations) != 5 || projected.Executable {
		t.Fatalf("typed/CLI product drift: %+v", projected)
	}
	for _, command := range []string{"admit", "enable", "execute", "publish"} {
		output.Reset()
		if err = bundle.CLI.RunV1(context.Background(), []string{"tool", "core-pack", command}, &output); err == nil || output.Len() != 0 {
			t.Fatalf("dangerous command %s was exposed", command)
		}
	}
}

func TestReferencePreviewFactoryV1FailuresAndIsolation(t *testing.T) {
	if _, err := product.NewReferencePreviewFactoryV1(nil); err == nil {
		t.Fatal("nil clock accepted")
	}
	now := time.Unix(31_000, 0).UTC()
	factory, _ := product.NewReferencePreviewFactoryV1(func() time.Time { return now })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := factory.BuildV1(ctx); err != context.Canceled {
		t.Fatalf("cancel=%v", err)
	}
	first, err := factory.BuildV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := factory.BuildV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	config := productConfigV1(now)
	a, err := first.Preview.PreviewV1(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	b, err := second.Preview.PreviewV1(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	if a.Digest != b.Digest {
		t.Fatal("fresh product roots are not deterministic")
	}
}

func TestReferencePreviewFactoryV1ConcurrentFreshProducts(t *testing.T) {
	now := time.Unix(32_000, 0).UTC()
	factory, _ := product.NewReferencePreviewFactoryV1(func() time.Time { return now })
	config := productConfigV1(now)
	const workers = 64
	var wg sync.WaitGroup
	digests := make(chan core.Digest, workers)
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bundle, e := factory.BuildV1(context.Background())
			if e != nil {
				errs <- e
				return
			}
			result, e := bundle.Preview.PreviewV1(context.Background(), config)
			errs <- e
			digests <- result.Digest
		}()
	}
	wg.Wait()
	close(errs)
	close(digests)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var want core.Digest
	for d := range digests {
		if want == "" {
			want = d
		}
		if d != want {
			t.Fatal("concurrent product digest drift")
		}
	}
}

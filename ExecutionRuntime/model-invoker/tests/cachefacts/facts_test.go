package cachefacts_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/cachefacts"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

func TestGeneratedProviderCacheFactsMatchCatalogAndCheckedInAsset(t *testing.T) {
	routeCatalog, err := catalog.NewDefault(time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	matrix, err := cachefacts.Build(routeCatalog)
	if err != nil {
		t.Fatal(err)
	}
	if len(matrix.Rows) != 39 {
		t.Fatalf("cache fact rows = %d, want 39", len(matrix.Rows))
	}
	generated, err := matrix.CSV()
	if err != nil {
		t.Fatal(err)
	}
	checkedIn, err := os.ReadFile(filepath.Join(repositoryRoot(t), ".properties.rax", "design", "model-invoker", "provider-cache-facts-v1candidate.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(generated, checkedIn) {
		t.Fatal("Provider cache fact asset drifted; run cmd/cachefactsgen")
	}
}

func TestOnlyXAIResponsesExposesStrictCacheKeyTransport(t *testing.T) {
	routeCatalog, err := catalog.NewDefault(time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	matrix, err := cachefacts.Build(routeCatalog)
	if err != nil {
		t.Fatal(err)
	}
	strict := 0
	for _, row := range matrix.Rows {
		if row.RequestControl != cachefacts.RequestControlStrictProviderOption {
			continue
		}
		strict++
		if row.AdapterID != "xai" || row.Protocol != upstream.ProtocolResponses || row.KeyOwnership != cachefacts.KeyOwnershipCallerProviderNamespace {
			t.Fatalf("unexpected strict cache transport row: %#v", row)
		}
	}
	if strict != 1 {
		t.Fatalf("strict cache transports = %d, want 1", strict)
	}
}

func TestCacheFactsNeverClaimPolicyOwnership(t *testing.T) {
	routeCatalog, err := catalog.NewDefault(time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	matrix, err := cachefacts.Build(routeCatalog)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range matrix.Rows {
		if row.TTLControl != "not_exposed" || row.StateRelation != "separate_binding_scoped_continuation" {
			t.Fatalf("cache policy leaked into transport facts for %q", row.RouteID)
		}
		if _, err := os.Stat(filepath.Join(moduleRoot(t), filepath.FromSlash(row.TransportCodePath))); err != nil {
			t.Errorf("transport code path for %q: %v", row.RouteID, err)
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	return filepath.Join(repositoryRoot(t), "ExecutionRuntime", "model-invoker")
}

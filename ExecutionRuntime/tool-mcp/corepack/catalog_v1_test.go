package corepack

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
)

func testCatalogV1(t *testing.T) CatalogV1 {
	t.Helper()
	catalog, err := BuildCatalogV1(CatalogConfigV1{
		Owner:            core.OwnerRef{Domain: "praxis.tool-mcp", ID: "core-pack"},
		ArtifactDigest:   core.DigestBytes([]byte("artifact")),
		SignatureDigest:  core.DigestBytes([]byte("signature")),
		ProvenanceDigest: core.DigestBytes([]byte("provenance")),
		CreatedAt:        time.Unix(100, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestCatalogV1ExactStableIdentities(t *testing.T) {
	catalog := testCatalogV1(t)
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	want := []string{"process.exec", "workspace.inspect", "workspace.patch", "workspace.read", "workspace.search"}
	for i, name := range want {
		if catalog.Definitions[i].ModelName != name || catalog.Tools[i].Mechanism != contract.MechanismLocal {
			t.Fatalf("definition %d drifted: %#v", i, catalog.Definitions[i])
		}
	}
	again := testCatalogV1(t)
	if catalog.Package.Digest != again.Package.Digest {
		t.Fatal("same catalog input is not deterministic")
	}
	if len(catalog.Package.Descriptors) != 5 {
		t.Fatal("package does not bind five tools")
	}
}

func TestCatalogV1PublishesStrictTypedOutputSchemas(t *testing.T) {
	catalog := testCatalogV1(t)
	wantFields := map[string][]string{
		"process.exec":      {"attempt_ref", "stdout_artifact_ref", "stderr_artifact_ref", "checked_unix_nano", "allOf"},
		"workspace.inspect": {"object", "entries", "range_valid", "checked_unix_nano", "maxItems"},
		"workspace.patch":   {"change_set_ref", "base_workspace", "result_workspace", "files", "minItems"},
		"workspace.read":    {"file", "content", "artifact_ref", "checked_unix_nano", "oneOf"},
		"workspace.search":  {"matches", "artifact_ref", "preview", "checked_unix_nano", "oneOf"},
	}
	for _, definition := range catalog.Definitions {
		if !json.Valid(definition.OutputSchema) {
			t.Fatalf("%s output schema is not valid JSON", definition.ModelName)
		}
		text := string(definition.OutputSchema)
		if !strings.Contains(text, `"additionalProperties":false`) {
			t.Fatalf("%s output schema is not closed", definition.ModelName)
		}
		for _, field := range wantFields[definition.ModelName] {
			if !strings.Contains(text, `"`+field+`"`) {
				t.Fatalf("%s output schema omits %s", definition.ModelName, field)
			}
		}
	}
}

func TestRegisterV1IsIdempotentAndDoesNotAdmitPackage(t *testing.T) {
	catalog := testCatalogV1(t)
	target := registry.New()
	now := time.Unix(101, 0)
	first, err := RegisterV1(target, catalog, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RegisterV1(target, catalog, now)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first.State != registry.StateSubmitted {
		t.Fatalf("package verification was bypassed or registration drifted: %#v %#v", first, second)
	}
	for _, tool := range catalog.Tools {
		_, record, ok := target.ResolveTool(string(tool.ID))
		if !ok || record.State != registry.StateActive {
			t.Fatalf("tool %s is not active", tool.ID)
		}
	}
}

func TestRegisterV1ConcurrentSameCatalog(t *testing.T) {
	catalog := testCatalogV1(t)
	target := registry.New()
	now := time.Unix(101, 0)
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := RegisterV1(target, catalog, now)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestBuildSurfaceV1IsCanonicalAndMCPFree(t *testing.T) {
	catalog := testCatalogV1(t)
	now := time.Unix(200, 0)
	surface, err := BuildSurfaceV1(catalog, SurfaceConfigV1{
		ID: "core-tool-surface-v1", Owner: core.OwnerRef{Domain: "praxis.tool-mcp", ID: "core-pack"},
		ResolvedPlanDigest: core.DigestBytes([]byte("plan")), ProfileDigest: core.DigestBytes([]byte("profile")),
		CapabilityGrantDigest: core.DigestBytes([]byte("grant")), RegistrySnapshotDigest: core.DigestBytes([]byte("registry")),
		CreatedAt: now, ExpiresAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := surface.Validate(); err != nil {
		t.Fatal(err)
	}
	want := []string{"process.exec", "workspace.inspect", "workspace.patch", "workspace.read", "workspace.search"}
	for i, entry := range surface.Entries {
		if entry.ModelName != want[i] || entry.Order != uint32(i) {
			t.Fatalf("surface order drifted: %#v", surface.Entries)
		}
	}
}

func TestCatalogCloneDoesNotAliasSchemaBytes(t *testing.T) {
	catalog := testCatalogV1(t)
	clone := catalog.Clone()
	clone.Materials[0].InputSchema[0] = '!'
	if catalog.Materials[0].InputSchema[0] == '!' {
		t.Fatal("catalog clone aliases schema bytes")
	}
}

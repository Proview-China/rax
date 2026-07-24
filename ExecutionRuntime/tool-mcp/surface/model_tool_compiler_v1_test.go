package surface_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/surface"
)

func TestToolDefinitionMaterialRepositoryV1CreateInspectAndConcurrentWinner(t *testing.T) {
	repo := surface.NewInMemoryToolDefinitionMaterialRepositoryV1()
	material := modelToolMaterialV1(t, "weather", "Look up weather")
	const workers = 64
	var failures atomic.Int32
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			winner, err := repo.EnsureExactToolDefinitionMaterialV1(context.Background(), material)
			if err != nil || winner.Ref != material.Ref {
				failures.Add(1)
			}
		}()
	}
	wg.Wait()
	if failures.Load() != 0 {
		t.Fatalf("concurrent Ensure failures = %d", failures.Load())
	}
	winner, err := repo.InspectExactToolDefinitionMaterialV1(context.Background(), material.Ref)
	if err != nil {
		t.Fatal(err)
	}
	winner.InputSchema[0] = '['
	again, err := repo.InspectExactToolDefinitionMaterialV1(context.Background(), material.Ref)
	if err != nil || again.InputSchema[0] != '{' {
		t.Fatalf("Repository returned aliased content: %v", err)
	}
	if _, err := repo.InspectExactToolDefinitionMaterialV1(context.Background(), driftMaterialRefV1(t, material.Ref)); err == nil {
		t.Fatal("same source with drifting exact Ref was accepted")
	}
}

func TestCompileModelToolsV1UsesPublicNeutralToolAndStableOrder(t *testing.T) {
	repo := surface.NewInMemoryToolDefinitionMaterialRepositoryV1()
	alpha := modelToolMaterialV1(t, "alpha", "Alpha tool")
	zeta := modelToolMaterialV1(t, "zeta", "Zeta tool")
	for _, value := range []toolcontract.ToolDefinitionMaterialV1{zeta, alpha} {
		if _, err := repo.EnsureExactToolDefinitionMaterialV1(context.Background(), value); err != nil {
			t.Fatal(err)
		}
	}
	current := modelToolSurfaceCurrentV1(t, []toolcontract.ToolDefinitionMaterialV1{zeta, alpha})
	clock := func() time.Time { return testkit.FixedTime.Add(time.Second) }
	compiled, err := surface.CompileModelToolsV1(context.Background(), current, repo, clock)
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Tools) != 2 || compiled.Tools[0].Name != "alpha" || compiled.Tools[1].Name != "zeta" || compiled.Tools[0].Strict == nil || !*compiled.Tools[0].Strict || compiled.Digest.Validate() != nil {
		t.Fatalf("neutral Tool compilation drifted: %#v", compiled)
	}
	request := modelinvoker.Request{
		Provider: "conformance", Model: "neutral", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "use a tool")},
		Tools: compiled.Tools, ToolChoice: modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceAuto},
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("compiled Tools are not accepted by Model Invoker public Request: %v", err)
	}
	compiled.Tools[0].Parameters[0] = '['
	again, err := repo.InspectExactToolDefinitionMaterialV1(context.Background(), alpha.Ref)
	if err != nil || again.InputSchema[0] != '{' {
		t.Fatalf("compiled output aliased Material Repository: %v", err)
	}
}

func TestCompileModelToolsV1FailsClosed(t *testing.T) {
	repo := surface.NewInMemoryToolDefinitionMaterialRepositoryV1()
	material := modelToolMaterialV1(t, "weather", "Look up weather")
	if _, err := repo.EnsureExactToolDefinitionMaterialV1(context.Background(), material); err != nil {
		t.Fatal(err)
	}
	valid := modelToolSurfaceCurrentV1(t, []toolcontract.ToolDefinitionMaterialV1{material})
	validClock := func() time.Time { return testkit.FixedTime.Add(time.Second) }
	var typedNil *surface.InMemoryToolDefinitionMaterialRepositoryV1
	cases := []struct {
		name    string
		ctx     context.Context
		current toolcontract.ToolSurfaceManifestCurrentProjectionV1
		reader  toolcontract.ToolDefinitionMaterialReaderV1
		clock   func() time.Time
	}{
		{name: "nil context", current: valid, reader: repo, clock: validClock},
		{name: "typed nil reader", ctx: context.Background(), current: valid, reader: typedNil, clock: validClock},
		{name: "nil clock", ctx: context.Background(), current: valid, reader: repo},
		{name: "expired surface", ctx: context.Background(), current: valid, reader: repo, clock: func() time.Time { return time.Unix(0, valid.ExpiresUnixNano) }},
		{name: "clock rollback", ctx: context.Background(), current: valid, reader: repo, clock: sequenceClockV1(testkit.FixedTime.Add(time.Second), testkit.FixedTime)},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cases = append(cases, struct {
		name    string
		ctx     context.Context
		current toolcontract.ToolSurfaceManifestCurrentProjectionV1
		reader  toolcontract.ToolDefinitionMaterialReaderV1
		clock   func() time.Time
	}{name: "canceled context", ctx: ctx, current: valid, reader: repo, clock: validClock})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if result, err := surface.CompileModelToolsV1(tc.ctx, tc.current, tc.reader, tc.clock); err == nil || len(result.Tools) != 0 {
				t.Fatalf("fail-closed compile returned %#v, %v", result, err)
			}
		})
	}
}

func TestCompileModelToolsV1RejectsDialectAndVisibilityDrift(t *testing.T) {
	repo := surface.NewInMemoryToolDefinitionMaterialRepositoryV1()
	material := modelToolMaterialV1(t, "weather", "Look up weather")
	if _, err := repo.EnsureExactToolDefinitionMaterialV1(context.Background(), material); err != nil {
		t.Fatal(err)
	}
	valid := modelToolSurfaceCurrentV1(t, []toolcontract.ToolDefinitionMaterialV1{material})
	for _, tc := range []struct {
		name   string
		mutate func(*toolcontract.ToolSurfaceManifest)
	}{
		{name: "dialect", mutate: func(m *toolcontract.ToolSurfaceManifest) { m.Dialect = "model/other" }},
		{name: "hidden", mutate: func(m *toolcontract.ToolSurfaceManifest) {
			m.Entries[0].Visibility = toolcontract.SurfaceHidden
			m.Entries[0].Allowed = false
		}},
		{name: "not allowed", mutate: func(m *toolcontract.ToolSurfaceManifest) { m.Entries[0].Allowed = false }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manifest := valid.Manifest
			manifest.Entries = append([]toolcontract.ToolSurfaceEntry(nil), manifest.Entries...)
			tc.mutate(&manifest)
			manifest.Digest = ""
			sealed, err := toolcontract.SealSurface(manifest)
			if err != nil {
				t.Fatal(err)
			}
			current, err := toolcontract.SealToolSurfaceManifestCurrentV1(toolcontract.ToolSurfaceManifestCurrentProjectionV1{
				Manifest: sealed, Owner: sealed.Owner, CheckedUnixNano: valid.CheckedUnixNano, ExpiresUnixNano: sealed.ExpiresUnixNano,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result, err := surface.CompileModelToolsV1(context.Background(), current, repo, func() time.Time { return testkit.FixedTime.Add(time.Second) }); err == nil || len(result.Tools) != 0 {
				t.Fatalf("dialect/visibility drift returned output: %#v %v", result, err)
			}
		})
	}
}

func TestCompileModelToolsV1RejectsNonPortableNameAndSchema(t *testing.T) {
	validMaterial := modelToolMaterialV1(t, "weather", "Look up weather")
	for _, tc := range []struct {
		name      string
		modelName string
		schema    []byte
	}{
		{name: "name starts with digit", modelName: "1weather"},
		{name: "name uses vendor-only punctuation", modelName: "weather.lookup"},
		{name: "schema is not strict", modelName: "weather", schema: []byte(`{"type":"object","properties":{"value":{"type":"string"}}}`)},
		{name: "schema keyword is not portable", modelName: "weather", schema: []byte(`{"type":"object","properties":{},"required":[],"additionalProperties":false,"allOf":[{}]}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := surface.NewInMemoryToolDefinitionMaterialRepositoryV1()
			material := validMaterial
			if tc.schema != nil {
				material.InputSchema = append([]byte(nil), tc.schema...)
				material.Ref.InputSchema.ContentDigest = core.DigestBytes(material.InputSchema)
				ref, err := toolcontract.DeriveToolDefinitionMaterialRefV1(material.Ref.Tool, material.Ref.InputSchema, material.Ref.DescriptionDigest)
				if err != nil {
					t.Fatal(err)
				}
				material.Ref = ref
			}
			if _, err := repo.EnsureExactToolDefinitionMaterialV1(context.Background(), material); err != nil {
				t.Fatal(err)
			}
			current := modelToolSurfaceCurrentV1(t, []toolcontract.ToolDefinitionMaterialV1{material})
			if tc.modelName != "" {
				manifest := current.Manifest
				manifest.Entries = append([]toolcontract.ToolSurfaceEntry(nil), manifest.Entries...)
				manifest.Entries[0].ModelName = tc.modelName
				manifest.Digest = ""
				sealed, err := toolcontract.SealSurface(manifest)
				if err != nil {
					t.Fatal(err)
				}
				current, err = toolcontract.SealToolSurfaceManifestCurrentV1(toolcontract.ToolSurfaceManifestCurrentProjectionV1{
					Manifest: sealed, Owner: sealed.Owner, CheckedUnixNano: current.CheckedUnixNano, ExpiresUnixNano: sealed.ExpiresUnixNano,
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			compiled, err := surface.CompileModelToolsV1(context.Background(), current, repo, func() time.Time { return testkit.FixedTime.Add(time.Second) })
			if err == nil || len(compiled.Tools) != 0 {
				t.Fatalf("non-portable Tool expression returned output: %#v, %v", compiled, err)
			}
		})
	}
}

func modelToolMaterialV1(t *testing.T, name, description string) toolcontract.ToolDefinitionMaterialV1 {
	t.Helper()
	schema := []byte(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"],"additionalProperties":false}`)
	tool := testkit.Tool()
	tool.ID = runtimeports.NamespacedNameV2("tool/" + name)
	tool.Digest = ""
	sealed, err := toolcontract.SealTool(tool)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := toolcontract.DeriveToolDefinitionMaterialRefV1(
		toolcontract.ObjectRef{ID: string(sealed.ID), Revision: sealed.Revision, Digest: sealed.Digest},
		runtimeports.SchemaRefV2{Namespace: "tool", Name: name, Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes(schema)},
		core.DigestBytes([]byte(description)),
	)
	if err != nil {
		t.Fatal(err)
	}
	return toolcontract.ToolDefinitionMaterialV1{Ref: ref, Description: description, InputSchema: schema}
}

func modelToolSurfaceCurrentV1(t *testing.T, values []toolcontract.ToolDefinitionMaterialV1) toolcontract.ToolSurfaceManifestCurrentProjectionV1 {
	t.Helper()
	entries := make([]toolcontract.ToolSurfaceEntry, 0, len(values))
	for _, value := range values {
		entries = append(entries, toolcontract.ToolSurfaceEntry{
			Capability: testkit.Tool().Capability, Tool: value.Ref.Tool, ModelName: value.Ref.InputSchema.Name,
			InputSchema: value.Ref.InputSchema, DescriptionDigest: value.Ref.DescriptionDigest,
			Visibility: toolcontract.SurfaceVisible, Allowed: true, Admission: toolcontract.AdmissionRequired,
			MechanismDigest: testkit.Digest("mechanism:" + value.Ref.ID), EffectKinds: []runtimeports.NamespacedNameV2{"praxis.tool/execute"},
		})
	}
	manifest, err := toolcontract.SealSurface(toolcontract.ToolSurfaceManifest{
		ID: "surface-model-tools", Revision: 1, Owner: testkit.Owner(), ResolvedPlanDigest: testkit.Digest("plan"),
		ProfileDigest: testkit.Digest("profile"), CapabilityGrantDigest: testkit.Digest("grant"), RegistrySnapshotDigest: testkit.Digest("registry"),
		Entries: entries, Dialect: toolcontract.ModelToolDialectFunctionCallingV1,
		CreatedUnixNano: testkit.FixedTime.UnixNano(), ExpiresUnixNano: testkit.FixedTime.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := toolcontract.SealToolSurfaceManifestCurrentV1(toolcontract.ToolSurfaceManifestCurrentProjectionV1{
		Manifest: manifest, Owner: manifest.Owner, CheckedUnixNano: testkit.FixedTime.UnixNano(), ExpiresUnixNano: manifest.ExpiresUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	return projection
}

func driftMaterialRefV1(t *testing.T, source toolcontract.ToolDefinitionMaterialRefV1) toolcontract.ToolDefinitionMaterialRefV1 {
	t.Helper()
	source.DescriptionDigest = testkit.Digest("drift")
	ref, err := toolcontract.DeriveToolDefinitionMaterialRefV1(source.Tool, source.InputSchema, source.DescriptionDigest)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func sequenceClockV1(values ...time.Time) func() time.Time {
	var index atomic.Uint32
	return func() time.Time {
		current := int(index.Add(1)) - 1
		if current >= len(values) {
			return values[len(values)-1]
		}
		return values[current]
	}
}

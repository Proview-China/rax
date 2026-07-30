package sqlite_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/applicationadapter"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
	toolsqlite "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/storage/sqlite"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/surface"
)

type sqliteModelToolCompileFixtureV1 struct {
	Surfaces                *surface.InMemoryToolSurfaceManifestCurrentRepositoryV1
	Definitions             *surface.InMemoryToolDefinitionMaterialRepositoryV1
	Currents                toolcontract.ToolRegistryObjectCurrentReaderV1
	Current                 toolcontract.ToolSurfaceManifestCurrentProjectionV1
	ExpectedExpiresUnixNano int64
}

func sqliteCompileFixtureV1(t *testing.T, clock *testkit.ManualClock) sqliteModelToolCompileFixtureV1 {
	t.Helper()
	schema := []byte(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"],"additionalProperties":false}`)
	capability := testkit.Capability()
	capability.ID = "tool/example-capability"
	capability.InputSchema = runtimeports.SchemaRefV2{
		Namespace: "tool", Name: "example", Version: "1.0.0",
		MediaType: "application/json", ContentDigest: core.DigestBytes(schema),
	}
	capability.Digest = ""
	var err error
	capability, err = toolcontract.SealCapability(capability)
	if err != nil {
		t.Fatal(err)
	}
	tool := testkit.Tool()
	tool.ID = "tool/example"
	tool.Capability = toolcontract.ObjectRef{ID: string(capability.ID), Revision: capability.Revision, Digest: capability.Digest}
	tool.InputSchema = capability.InputSchema
	tool.OutputSchema = capability.OutputSchema
	tool.EffectKinds = append([]runtimeports.NamespacedNameV2(nil), capability.EffectKinds...)
	tool.Digest = ""
	tool, err = toolcontract.SealTool(tool)
	if err != nil {
		t.Fatal(err)
	}

	description := "EXAMPLE tool"
	definitionRef, err := toolcontract.DeriveToolDefinitionMaterialRefV1(
		toolcontract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest},
		tool.InputSchema,
		core.DigestBytes([]byte(description)),
	)
	if err != nil {
		t.Fatal(err)
	}
	definitions := surface.NewInMemoryToolDefinitionMaterialRepositoryV1()
	if _, err = definitions.EnsureExactToolDefinitionMaterialV1(context.Background(), toolcontract.ToolDefinitionMaterialV1{
		Ref: definitionRef, Description: description, InputSchema: schema,
	}); err != nil {
		t.Fatal(err)
	}

	source := registry.New()
	capRecord, err := source.SubmitCapability(capability, testkit.FixedTime.Add(-4*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	capRecord, err = source.Transition("capability", string(capability.ID), capRecord.RegistryRevision, registry.StateAdmitted, testkit.FixedTime.Add(-3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = source.Transition("capability", string(capability.ID), capRecord.RegistryRevision, registry.StateActive, testkit.FixedTime.Add(-2*time.Second)); err != nil {
		t.Fatal(err)
	}
	toolRecord, err := source.SubmitTool(tool, testkit.FixedTime.Add(-2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	toolRecord, err = source.Transition("tool", string(tool.ID), toolRecord.RegistryRevision, registry.StateAdmitted, testkit.FixedTime.Add(-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = source.Transition("tool", string(tool.ID), toolRecord.RegistryRevision, registry.StateActive, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	currents, err := applicationadapter.NewRegistryObjectCurrentReaderV1(source, clock)
	if err != nil {
		t.Fatal(err)
	}

	manifest, err := toolcontract.SealSurface(toolcontract.ToolSurfaceManifest{
		ID: "surface-model-tool-injection-v1", Revision: 1, Owner: testkit.Owner(),
		ResolvedPlanDigest: testkit.Digest("injection-plan"), ProfileDigest: testkit.Digest("injection-profile"),
		CapabilityGrantDigest: testkit.Digest("injection-grant"), RegistrySnapshotDigest: testkit.Digest("injection-registry"),
		Entries: []toolcontract.ToolSurfaceEntry{{
			Capability: tool.Capability,
			Tool:       toolcontract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest},
			ModelName:  "example", InputSchema: tool.InputSchema,
			DescriptionDigest: definitionRef.DescriptionDigest,
			Visibility:        toolcontract.SurfaceVisible, Allowed: true, Admission: toolcontract.AdmissionRequired,
			MechanismDigest: tool.ArtifactDigest,
			EffectKinds:     append([]runtimeports.NamespacedNameV2(nil), tool.EffectKinds...),
		}},
		Dialect:         toolcontract.ModelToolDialectFunctionCallingV1,
		CreatedUnixNano: testkit.FixedTime.Add(-time.Second).UnixNano(),
		ExpiresUnixNano: testkit.FixedTime.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	surfaces, err := surface.NewInMemoryToolSurfaceManifestCurrentRepositoryV1(clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	current, err := surfaces.EnsureExactToolSurfaceManifestCurrentV1(context.Background(), toolcontract.ToolSurfaceManifestCurrentEnsureRequestV1{
		ContractVersion: toolcontract.ToolSurfaceManifestCurrentContractVersionV1,
		Manifest:        manifest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(current.Ref.ID) == "" {
		t.Fatal("Model Tool Injection fixture Surface current is absent")
	}
	return sqliteModelToolCompileFixtureV1{
		Surfaces: surfaces, Definitions: definitions, Currents: currents, Current: current,
		ExpectedExpiresUnixNano: current.Manifest.ExpiresUnixNano,
	}
}

func compileAndPersistV1(t *testing.T, store *toolsqlite.StoreV1, clock *testkit.ManualClock) (surface.CompiledModelToolsV1, toolcontract.ModelToolInjectionMaterialV1) {
	t.Helper()
	fixture := sqliteCompileFixtureV1(t, clock)
	compiled, material, err := store.CompileAndEnsureModelToolInjectionMaterialV1(
		context.Background(), fixture.Current.Ref, fixture.Surfaces, fixture.Definitions, fixture.Currents,
	)
	if err != nil {
		t.Fatal(err)
	}
	return compiled, material
}

package sdk_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/surface"
)

func TestModelToolAssemblySDKV1ExactSurfaceAndMaterial(t *testing.T) {
	clock := func() time.Time { return testkit.FixedTime.Add(time.Second) }
	surfaces, err := surface.NewInMemoryToolSurfaceManifestCurrentRepositoryV1(clock)
	if err != nil {
		t.Fatal(err)
	}
	materials := surface.NewInMemoryToolDefinitionMaterialRepositoryV1()
	client, err := sdk.NewModelToolAssemblyV1(surfaces, materials, clock)
	if err != nil {
		t.Fatal(err)
	}
	material := sdkMaterialV1(t)
	if _, err := client.EnsureToolDefinitionMaterialV1(context.Background(), material); err != nil {
		t.Fatal(err)
	}
	manifest := sdkSurfaceV1(t, material)
	current, err := surfaces.EnsureExactToolSurfaceManifestCurrentV1(context.Background(), toolcontract.ToolSurfaceManifestCurrentEnsureRequestV1{
		ContractVersion: toolcontract.ToolSurfaceManifestCurrentContractVersionV1, Manifest: manifest,
	})
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := client.CompileModelToolsV1(context.Background(), current.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Tools) != 1 || compiled.Tools[0].Name != "weather" || string(compiled.Tools[0].Parameters) != string(material.InputSchema) {
		t.Fatalf("SDK compiled wrong neutral Model Tool: %#v", compiled)
	}
	if inspected, err := client.InspectToolDefinitionMaterialV1(context.Background(), material.Ref); err != nil || inspected.Ref != material.Ref {
		t.Fatalf("SDK exact Material inspect failed: %v", err)
	}
}

func TestModelToolAssemblySDKV1RejectsTypedNilAndMissingMaterial(t *testing.T) {
	clock := func() time.Time { return testkit.FixedTime.Add(time.Second) }
	materials := surface.NewInMemoryToolDefinitionMaterialRepositoryV1()
	var typedNil *surface.InMemoryToolSurfaceManifestCurrentRepositoryV1
	if _, err := sdk.NewModelToolAssemblyV1(typedNil, materials, clock); err == nil {
		t.Fatal("typed-nil Surface Reader was accepted")
	}
	surfaces, err := surface.NewInMemoryToolSurfaceManifestCurrentRepositoryV1(clock)
	if err != nil {
		t.Fatal(err)
	}
	client, err := sdk.NewModelToolAssemblyV1(surfaces, materials, clock)
	if err != nil {
		t.Fatal(err)
	}
	material := sdkMaterialV1(t)
	current, err := surfaces.EnsureExactToolSurfaceManifestCurrentV1(context.Background(), toolcontract.ToolSurfaceManifestCurrentEnsureRequestV1{
		ContractVersion: toolcontract.ToolSurfaceManifestCurrentContractVersionV1, Manifest: sdkSurfaceV1(t, material),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result, err := client.CompileModelToolsV1(context.Background(), current.Ref); err == nil || len(result.Tools) != 0 {
		t.Fatalf("missing exact Material did not fail closed: %#v %v", result, err)
	}
}

func sdkMaterialV1(t *testing.T) toolcontract.ToolDefinitionMaterialV1 {
	t.Helper()
	schema := []byte(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"],"additionalProperties":false}`)
	description := "Look up weather"
	tool := testkit.Tool()
	ref, err := toolcontract.DeriveToolDefinitionMaterialRefV1(
		toolcontract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest},
		runtimeports.SchemaRefV2{Namespace: "tool", Name: "weather", Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes(schema)},
		core.DigestBytes([]byte(description)),
	)
	if err != nil {
		t.Fatal(err)
	}
	return toolcontract.ToolDefinitionMaterialV1{Ref: ref, Description: description, InputSchema: schema}
}

func sdkSurfaceV1(t *testing.T, material toolcontract.ToolDefinitionMaterialV1) toolcontract.ToolSurfaceManifest {
	t.Helper()
	manifest, err := toolcontract.SealSurface(toolcontract.ToolSurfaceManifest{
		ID: "surface-sdk-model-tools", Revision: 1, Owner: testkit.Owner(), ResolvedPlanDigest: testkit.Digest("plan"),
		ProfileDigest: testkit.Digest("profile"), CapabilityGrantDigest: testkit.Digest("grant"), RegistrySnapshotDigest: testkit.Digest("registry"),
		Entries: []toolcontract.ToolSurfaceEntry{{
			Capability: testkit.Tool().Capability, Tool: material.Ref.Tool, ModelName: "weather", InputSchema: material.Ref.InputSchema,
			DescriptionDigest: material.Ref.DescriptionDigest, Visibility: toolcontract.SurfaceVisible, Allowed: true,
			Admission: toolcontract.AdmissionRequired, MechanismDigest: testkit.Digest("mechanism"), EffectKinds: []runtimeports.NamespacedNameV2{"praxis.tool/execute"},
		}},
		Dialect:         toolcontract.ModelToolDialectFunctionCallingV1,
		CreatedUnixNano: testkit.FixedTime.UnixNano(), ExpiresUnixNano: testkit.FixedTime.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

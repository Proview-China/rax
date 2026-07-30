package surface_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/applicationadapter"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/surface"
)

func TestCompileModelToolInjectionMaterialV1ExactDeterministicAndZeroExecution(t *testing.T) {
	fixture := newModelToolInjectionFixtureV1(t)
	compiled, material, err := surface.CompileModelToolInjectionMaterialV1(
		context.Background(), fixture.current.Ref, fixture.surfaces, fixture.definitions, fixture.currents,
		func() time.Time { return testkit.FixedTime.Add(time.Second) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if material.ValidateCurrent(material.Ref, testkit.FixedTime.Add(time.Second)) != nil || material.Surface != fixture.current.Ref ||
		material.ExpectedInjectionDigest != fixture.current.Manifest.ExpectedInjectionDigest || material.CompiledToolsDigest != compiled.Digest {
		t.Fatalf("compiled Material closure drifted: compiled=%#v material=%#v", compiled, material)
	}
	if len(material.Entries) != 2 || material.Entries[0].ModelName != "alpha" || material.Entries[1].ModelName != "zeta" ||
		compiled.Tools[0].Name != "alpha" || compiled.Tools[1].Name != "zeta" {
		t.Fatalf("canonical order drifted: material=%#v compiled=%#v", material.Entries, compiled.Tools)
	}
	for index, entry := range material.Entries {
		expected := fixture.capabilities[entry.ModelName]
		if entry.Order != uint32(index) || !entry.Strict || entry.ReviewProfile != expected.ReviewProfile ||
			entry.AuthorityRequirement != expected.AuthorityRequirement || entry.BudgetRequirement != expected.BudgetRequirement ||
			entry.SandboxRequirement != expected.SandboxRequirement || entry.EvidenceRequirement != expected.EvidenceRequirement {
			t.Fatalf("entry %d governance projection drifted: %#v", index, entry)
		}
	}
	wire, err := json.Marshal(material)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"tool_choice", "verdict", "authorization", "permit"} {
		if strings.Contains(string(wire), forbidden) {
			t.Fatalf("forbidden field %q entered Model Tool Injection Material: %s", forbidden, wire)
		}
	}
	compiledAgain, materialAgain, err := surface.CompileModelToolInjectionMaterialV1(
		context.Background(), fixture.current.Ref, fixture.surfaces, fixture.definitions, fixture.currents,
		func() time.Time { return testkit.FixedTime.Add(time.Second) },
	)
	if err != nil || !reflect.DeepEqual(compiled, compiledAgain) || !reflect.DeepEqual(material, materialAgain) {
		t.Fatalf("same exact closure was not deterministic: compiled=%v material=%v err=%v", reflect.DeepEqual(compiled, compiledAgain), reflect.DeepEqual(material, materialAgain), err)
	}
	if fixture.providerCalls.Load() != 0 || fixture.toolExecutionCalls.Load() != 0 {
		t.Fatalf("pure compilation crossed an execution boundary: provider=%d tool=%d", fixture.providerCalls.Load(), fixture.toolExecutionCalls.Load())
	}
}

func TestCompileModelToolInjectionMaterialV1S1S2FailClosed(t *testing.T) {
	t.Run("surface current drift", func(t *testing.T) {
		fixture := newModelToolInjectionFixtureV1(t)
		reader := &driftingSurfaceReaderV1{base: fixture.surfaces}
		compiled, material, err := surface.CompileModelToolInjectionMaterialV1(
			context.Background(), fixture.current.Ref, reader, fixture.definitions, fixture.currents,
			func() time.Time { return testkit.FixedTime.Add(time.Second) },
		)
		if err == nil || len(compiled.Tools) != 0 || len(material.Entries) != 0 || reader.calls.Load() != 2 {
			t.Fatalf("Surface S1/S2 drift was accepted: compiled=%#v material=%#v calls=%d err=%v", compiled, material, reader.calls.Load(), err)
		}
	})
	t.Run("definition material drift", func(t *testing.T) {
		fixture := newModelToolInjectionFixtureV1(t)
		reader := &driftingDefinitionReaderV1{base: fixture.definitions, driftOn: 3}
		compiled, material, err := surface.CompileModelToolInjectionMaterialV1(
			context.Background(), fixture.current.Ref, fixture.surfaces, reader, fixture.currents,
			func() time.Time { return testkit.FixedTime.Add(time.Second) },
		)
		if err == nil || len(compiled.Tools) != 0 || len(material.Entries) != 0 {
			t.Fatalf("Definition Material S1/S2 drift was accepted: compiled=%#v material=%#v err=%v", compiled, material, err)
		}
	})
	t.Run("capability current closure drift", func(t *testing.T) {
		fixture := newModelToolInjectionFixtureV1(t)
		reader := &driftingRegistryReaderV1{base: fixture.currents, driftCapability: true}
		compiled, material, err := surface.CompileModelToolInjectionMaterialV1(
			context.Background(), fixture.current.Ref, fixture.surfaces, fixture.definitions, reader,
			func() time.Time { return testkit.FixedTime.Add(time.Second) },
		)
		if err == nil || len(compiled.Tools) != 0 || len(material.Entries) != 0 {
			t.Fatalf("Capability S1/S2 drift was accepted: compiled=%#v material=%#v err=%v", compiled, material, err)
		}
	})
	t.Run("tool current closure drift", func(t *testing.T) {
		fixture := newModelToolInjectionFixtureV1(t)
		reader := &driftingRegistryReaderV1{base: fixture.currents, driftTool: true}
		compiled, material, err := surface.CompileModelToolInjectionMaterialV1(
			context.Background(), fixture.current.Ref, fixture.surfaces, fixture.definitions, reader,
			func() time.Time { return testkit.FixedTime.Add(time.Second) },
		)
		if err == nil || len(compiled.Tools) != 0 || len(material.Entries) != 0 {
			t.Fatalf("Tool S1/S2 drift was accepted: compiled=%#v material=%#v err=%v", compiled, material, err)
		}
	})
	t.Run("now equals minimum expiry", func(t *testing.T) {
		fixture := newModelToolInjectionFixtureV1(t)
		expiry := testkit.FixedTime.Add(toolcontract.MaxToolRegistryObjectCurrentTTLV1)
		clock := testkit.NewSequenceClock(testkit.FixedTime.Add(time.Second), testkit.FixedTime.Add(2*time.Second), expiry)
		compiled, material, err := surface.CompileModelToolInjectionMaterialV1(
			context.Background(), fixture.current.Ref, fixture.surfaces, fixture.definitions, fixture.currents, clock.Now,
		)
		if err == nil || len(compiled.Tools) != 0 || len(material.Entries) != 0 {
			t.Fatalf("now==expires was accepted: compiled=%#v material=%#v err=%v", compiled, material, err)
		}
	})
}

type modelToolInjectionFixtureV1 struct {
	surfaces           *surface.InMemoryToolSurfaceManifestCurrentRepositoryV1
	definitions        *surface.InMemoryToolDefinitionMaterialRepositoryV1
	currents           *applicationadapter.RegistryObjectCurrentReaderV1
	current            toolcontract.ToolSurfaceManifestCurrentProjectionV1
	capabilities       map[string]toolcontract.CapabilityDescriptor
	providerCalls      atomic.Int64
	toolExecutionCalls atomic.Int64
}

func newModelToolInjectionFixtureV1(t *testing.T) modelToolInjectionFixtureV1 {
	t.Helper()
	type descriptorSet struct {
		capability toolcontract.CapabilityDescriptor
		tool       toolcontract.ToolDescriptor
		material   toolcontract.ToolDefinitionMaterialV1
	}
	sets := make([]descriptorSet, 0, 2)
	for _, name := range []string{"zeta", "alpha"} {
		schema := []byte(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"],"additionalProperties":false}`)
		capability := testkit.Capability()
		capability.ID = runtimeports.NamespacedNameV2("tool/" + name + "-capability")
		capability.InputSchema = runtimeports.SchemaRefV2{
			Namespace: "tool", Name: name, Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes(schema),
		}
		capability.Digest = ""
		var err error
		capability, err = toolcontract.SealCapability(capability)
		if err != nil {
			t.Fatal(err)
		}
		tool := testkit.Tool()
		tool.ID = runtimeports.NamespacedNameV2("tool/" + name)
		tool.Capability = toolcontract.ObjectRef{ID: string(capability.ID), Revision: capability.Revision, Digest: capability.Digest}
		tool.InputSchema = capability.InputSchema
		tool.OutputSchema = capability.OutputSchema
		tool.EffectKinds = append([]runtimeports.NamespacedNameV2(nil), capability.EffectKinds...)
		tool.Digest = ""
		tool, err = toolcontract.SealTool(tool)
		if err != nil {
			t.Fatal(err)
		}
		description := strings.ToUpper(name) + " tool"
		ref, err := toolcontract.DeriveToolDefinitionMaterialRefV1(
			toolcontract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest},
			tool.InputSchema, core.DigestBytes([]byte(description)),
		)
		if err != nil {
			t.Fatal(err)
		}
		sets = append(sets, descriptorSet{
			capability: capability, tool: tool,
			material: toolcontract.ToolDefinitionMaterialV1{Ref: ref, Description: description, InputSchema: schema},
		})
	}

	source := registry.New()
	definitions := surface.NewInMemoryToolDefinitionMaterialRepositoryV1()
	entries := make([]toolcontract.ToolSurfaceEntry, 0, len(sets))
	capabilities := make(map[string]toolcontract.CapabilityDescriptor, len(sets))
	for _, set := range sets {
		capRecord, err := source.SubmitCapability(set.capability, testkit.FixedTime.Add(-4*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		capRecord, err = source.Transition("capability", string(set.capability.ID), capRecord.RegistryRevision, registry.StateAdmitted, testkit.FixedTime.Add(-3*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = source.Transition("capability", string(set.capability.ID), capRecord.RegistryRevision, registry.StateActive, testkit.FixedTime.Add(-2*time.Second)); err != nil {
			t.Fatal(err)
		}
		toolRecord, err := source.SubmitTool(set.tool, testkit.FixedTime.Add(-2*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		toolRecord, err = source.Transition("tool", string(set.tool.ID), toolRecord.RegistryRevision, registry.StateAdmitted, testkit.FixedTime.Add(-time.Second))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = source.Transition("tool", string(set.tool.ID), toolRecord.RegistryRevision, registry.StateActive, testkit.FixedTime); err != nil {
			t.Fatal(err)
		}
		if _, err = definitions.EnsureExactToolDefinitionMaterialV1(context.Background(), set.material); err != nil {
			t.Fatal(err)
		}
		name := set.material.Ref.InputSchema.Name
		capabilities[name] = set.capability
		entries = append(entries, toolcontract.ToolSurfaceEntry{
			Capability:        set.tool.Capability,
			Tool:              set.material.Ref.Tool,
			ModelName:         name,
			InputSchema:       set.material.Ref.InputSchema,
			DescriptionDigest: set.material.Ref.DescriptionDigest,
			Visibility:        toolcontract.SurfaceVisible,
			Allowed:           true,
			Admission:         toolcontract.AdmissionRequired,
			MechanismDigest:   set.tool.ArtifactDigest,
			EffectKinds:       append([]runtimeports.NamespacedNameV2(nil), set.tool.EffectKinds...),
		})
	}
	manifest, err := toolcontract.SealSurface(toolcontract.ToolSurfaceManifest{
		ID: "surface-model-tool-injection-v1", Revision: 1, Owner: testkit.Owner(),
		ResolvedPlanDigest: testkit.Digest("injection-plan"), ProfileDigest: testkit.Digest("injection-profile"),
		CapabilityGrantDigest: testkit.Digest("injection-grant"), RegistrySnapshotDigest: testkit.Digest("injection-registry"),
		Entries: entries, Dialect: toolcontract.ModelToolDialectFunctionCallingV1,
		CreatedUnixNano: testkit.FixedTime.Add(-time.Second).UnixNano(), ExpiresUnixNano: testkit.FixedTime.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	surfaces, err := surface.NewInMemoryToolSurfaceManifestCurrentRepositoryV1(func() time.Time { return testkit.FixedTime })
	if err != nil {
		t.Fatal(err)
	}
	current, err := surfaces.EnsureExactToolSurfaceManifestCurrentV1(context.Background(), toolcontract.ToolSurfaceManifestCurrentEnsureRequestV1{
		ContractVersion: toolcontract.ToolSurfaceManifestCurrentContractVersionV1, Manifest: manifest,
	})
	if err != nil {
		t.Fatal(err)
	}
	currents, err := applicationadapter.NewRegistryObjectCurrentReaderV1(source, testkit.NewManualClock(testkit.FixedTime))
	if err != nil {
		t.Fatal(err)
	}
	return modelToolInjectionFixtureV1{
		surfaces: surfaces, definitions: definitions, currents: currents, current: current, capabilities: capabilities,
	}
}

type driftingSurfaceReaderV1 struct {
	base  toolcontract.ToolSurfaceManifestCurrentReaderV1
	calls atomic.Int64
}

func (r *driftingSurfaceReaderV1) InspectExactToolSurfaceManifestCurrentV1(ctx context.Context, exact toolcontract.ToolSurfaceManifestCurrentRefV1) (toolcontract.ToolSurfaceManifestCurrentProjectionV1, error) {
	value, err := r.base.InspectExactToolSurfaceManifestCurrentV1(ctx, exact)
	if err == nil && r.calls.Add(1) == 2 {
		value.CheckedUnixNano++
	}
	return value, err
}

type driftingDefinitionReaderV1 struct {
	base    toolcontract.ToolDefinitionMaterialReaderV1
	calls   atomic.Int64
	driftOn int64
}

func (r *driftingDefinitionReaderV1) InspectExactToolDefinitionMaterialV1(ctx context.Context, exact toolcontract.ToolDefinitionMaterialRefV1) (toolcontract.ToolDefinitionMaterialV1, error) {
	value, err := r.base.InspectExactToolDefinitionMaterialV1(ctx, exact)
	if err == nil && r.calls.Add(1) == r.driftOn {
		value.Description += " drift"
	}
	return value, err
}

type driftingRegistryReaderV1 struct {
	base            toolcontract.ToolRegistryObjectCurrentReaderV1
	driftCapability bool
	driftTool       bool
}

func (r *driftingRegistryReaderV1) ResolveExactToolCapabilityCurrentV1(ctx context.Context, object toolcontract.ObjectRef) (toolcontract.CapabilityDescriptor, toolcontract.ToolRegistryObjectCurrentProjectionV1, error) {
	return r.base.ResolveExactToolCapabilityCurrentV1(ctx, object)
}

func (r *driftingRegistryReaderV1) InspectExactToolCapabilityCurrentV1(ctx context.Context, object toolcontract.ObjectRef, expected toolcontract.ToolRegistryObjectCurrentRefV1) (toolcontract.CapabilityDescriptor, toolcontract.ToolRegistryObjectCurrentProjectionV1, error) {
	value, current, err := r.base.InspectExactToolCapabilityCurrentV1(ctx, object, expected)
	if err == nil && r.driftCapability {
		value.ReviewProfile = "review/drifted"
	}
	return value, current, err
}

func (r *driftingRegistryReaderV1) ResolveExactToolDescriptorCurrentV1(ctx context.Context, object toolcontract.ObjectRef) (toolcontract.ToolDescriptor, toolcontract.ToolRegistryObjectCurrentProjectionV1, error) {
	return r.base.ResolveExactToolDescriptorCurrentV1(ctx, object)
}

func (r *driftingRegistryReaderV1) InspectExactToolDescriptorCurrentV1(ctx context.Context, object toolcontract.ObjectRef, expected toolcontract.ToolRegistryObjectCurrentRefV1) (toolcontract.ToolDescriptor, toolcontract.ToolRegistryObjectCurrentProjectionV1, error) {
	value, current, err := r.base.InspectExactToolDescriptorCurrentV1(ctx, object, expected)
	if err == nil && r.driftTool {
		value.Digest = testkit.Digest("drifted-tool")
	}
	return value, current, err
}

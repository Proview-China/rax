package contract_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestModelToolInjectionMaterialV1CanonicalExactAndSplices(t *testing.T) {
	material := contractMaterialV1(t)
	if err := material.ValidateCurrent(material.Ref, testkit.FixedTime.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*toolcontract.ModelToolInjectionMaterialV1)
	}{
		{name: "surface", mutate: func(value *toolcontract.ModelToolInjectionMaterialV1) { value.Surface.Revision++ }},
		{name: "material revision", mutate: func(value *toolcontract.ModelToolInjectionMaterialV1) { value.Ref.Revision++ }},
		{name: "material digest", mutate: func(value *toolcontract.ModelToolInjectionMaterialV1) { value.Digest = testkit.Digest("splice") }},
		{name: "definition material", mutate: func(value *toolcontract.ModelToolInjectionMaterialV1) {
			value.Entries[0].DefinitionMaterialRef.DescriptionDigest = testkit.Digest("splice")
		}},
		{name: "compiled digest", mutate: func(value *toolcontract.ModelToolInjectionMaterialV1) {
			value.CompiledToolsDigest = testkit.Digest("splice")
		}},
		{name: "strict false", mutate: func(value *toolcontract.ModelToolInjectionMaterialV1) { value.Entries[0].Strict = false }},
		{name: "ttl", mutate: func(value *toolcontract.ModelToolInjectionMaterialV1) { value.ExpiresUnixNano++ }},
	} {
		t.Run(test.name, func(t *testing.T) {
			spliced := material.Clone()
			test.mutate(&spliced)
			if err := spliced.Validate(); err == nil {
				t.Fatalf("%s splice was accepted", test.name)
			}
		})
	}
	wrong := material.Ref
	wrong.Digest = testkit.Digest("wrong-exact")
	if err := material.ValidateCurrent(wrong, testkit.FixedTime.Add(time.Second)); err == nil {
		t.Fatal("wrong exact Material Ref was accepted")
	}
	if err := material.ValidateCurrent(material.Ref, time.Unix(0, material.ExpiresUnixNano)); err == nil || !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("now==expires was accepted: %v", err)
	}
}

func TestSealModelToolInjectionMaterialV1ReturnsZeroOnValidationFailure(t *testing.T) {
	material := contractMaterialV1(t)
	material.Ref = toolcontract.ModelToolInjectionMaterialRefV1{}
	material.Digest = ""
	material.Entries[0].Strict = false

	sealed, err := toolcontract.SealModelToolInjectionMaterialV1(material)
	if err == nil {
		t.Fatal("invalid Model Tool Injection Material was sealed")
	}
	if !reflect.DeepEqual(sealed, toolcontract.ModelToolInjectionMaterialV1{}) {
		t.Fatalf("failed seal leaked a non-zero invalid material: %#v", sealed)
	}
}

func TestModelToolInjectionMaterialV1RejectsNonPortableAndDuplicateModelNames(t *testing.T) {
	t.Run("non-portable", func(t *testing.T) {
		material := contractMaterialV1(t)
		material.Ref = toolcontract.ModelToolInjectionMaterialRefV1{}
		material.Digest = ""
		material.Entries[0].ModelName = "not.portable"
		sealed, err := toolcontract.SealModelToolInjectionMaterialV1(material)
		if err == nil || !reflect.DeepEqual(sealed, toolcontract.ModelToolInjectionMaterialV1{}) {
			t.Fatalf("non-portable Model name was not rejected with a zero result: sealed=%#v err=%v", sealed, err)
		}
	})
	t.Run("duplicate", func(t *testing.T) {
		material := contractMaterialV1(t)
		material.Ref = toolcontract.ModelToolInjectionMaterialRefV1{}
		material.Digest = ""
		duplicate := material.Entries[0]
		duplicate.Order = 1
		material.Entries = append(material.Entries, duplicate)
		material.ExpectedInjectionDigest = testkit.Digest("recomputed-later")
		sealed, err := toolcontract.SealModelToolInjectionMaterialV1(material)
		if err == nil || !reflect.DeepEqual(sealed, toolcontract.ModelToolInjectionMaterialV1{}) {
			t.Fatalf("duplicate Model name was not rejected with a zero result: sealed=%#v err=%v", sealed, err)
		}
	})
}

func contractMaterialV1(t *testing.T) toolcontract.ModelToolInjectionMaterialV1 {
	t.Helper()
	capability, tool := testkit.Capability(), testkit.Tool()
	raw := []byte(`{"type":"object","properties":{},"required":[],"additionalProperties":false}`)
	input := tool.InputSchema
	input.ContentDigest = core.DigestBytes(raw)
	description := testkit.Digest("description")
	toolRef := toolcontract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest}
	definition, err := toolcontract.DeriveToolDefinitionMaterialRefV1(toolRef, input, description)
	if err != nil {
		t.Fatal(err)
	}
	capabilityRef := toolcontract.ObjectRef{ID: string(capability.ID), Revision: capability.Revision, Digest: capability.Digest}
	entry := toolcontract.ModelToolInjectionEntryV1{
		ModelName: "example", CapabilityRef: capabilityRef, ToolRef: toolRef, DefinitionMaterialRef: definition,
		InputSchemaRef: input, DescriptionDigest: description, Strict: true, Admission: toolcontract.AdmissionRequired,
		EffectKinds:   append([]runtimeports.NamespacedNameV2(nil), capability.EffectKinds...),
		ReviewProfile: capability.ReviewProfile, AuthorityRequirement: capability.AuthorityRequirement,
		BudgetRequirement: capability.BudgetRequirement, SandboxRequirement: capability.SandboxRequirement,
		EvidenceRequirement: capability.EvidenceRequirement,
	}
	expected, err := toolcontract.ComputeExpectedInjectionDigest([]toolcontract.ToolSurfaceEntry{{
		Capability: capabilityRef, Tool: toolRef, ModelName: entry.ModelName, InputSchema: input,
		DescriptionDigest: description, Visibility: toolcontract.SurfaceVisible, Allowed: true,
		Admission: entry.Admission, MechanismDigest: tool.Digest, EffectKinds: entry.EffectKinds,
	}})
	if err != nil {
		t.Fatal(err)
	}
	material, err := toolcontract.SealModelToolInjectionMaterialV1(toolcontract.ModelToolInjectionMaterialV1{
		Surface: toolcontract.ToolSurfaceManifestCurrentRefV1{
			ContractVersion: toolcontract.ToolSurfaceManifestCurrentContractVersionV1,
			ID:              "surface-contract-material-v1", Revision: 1, Digest: testkit.Digest("surface-contract-material"),
		},
		Entries:                 []toolcontract.ModelToolInjectionEntryV1{entry},
		ExpectedInjectionDigest: expected, CompiledToolsDigest: testkit.Digest("compiled"),
		CreatedUnixNano: testkit.FixedTime.UnixNano(), ExpiresUnixNano: testkit.FixedTime.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return material
}

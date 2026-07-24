package contract_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestToolDefinitionMaterialV1ExactContent(t *testing.T) {
	material := definitionMaterialV1(t)
	if err := material.Validate(); err != nil {
		t.Fatal(err)
	}
	clone := material.Clone()
	clone.InputSchema[0] = '['
	if material.InputSchema[0] != '{' {
		t.Fatal("Tool Definition Material Clone aliased schema bytes")
	}

	cases := []struct {
		name   string
		mutate func(*contract.ToolDefinitionMaterialV1)
	}{
		{name: "description digest", mutate: func(v *contract.ToolDefinitionMaterialV1) { v.Description = "changed" }},
		{name: "schema digest", mutate: func(v *contract.ToolDefinitionMaterialV1) {
			v.InputSchema = []byte(`{"type":"object","properties":{"x":{"type":"string"}}}`)
		}},
		{name: "schema array", mutate: func(v *contract.ToolDefinitionMaterialV1) {
			v.InputSchema = []byte(`[]`)
			v.Ref.InputSchema.ContentDigest = core.DigestBytes(v.InputSchema)
			v.Ref, _ = contract.DeriveToolDefinitionMaterialRefV1(v.Ref.Tool, v.Ref.InputSchema, v.Ref.DescriptionDigest)
		}},
		{name: "schema trailing", mutate: func(v *contract.ToolDefinitionMaterialV1) {
			v.InputSchema = []byte(`{"type":"object"}{}`)
			v.Ref.InputSchema.ContentDigest = core.DigestBytes(v.InputSchema)
			v.Ref, _ = contract.DeriveToolDefinitionMaterialRefV1(v.Ref.Tool, v.Ref.InputSchema, v.Ref.DescriptionDigest)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value := material.Clone()
			tc.mutate(&value)
			if err := value.Validate(); err == nil {
				t.Fatal("invalid Tool Definition Material was accepted")
			}
		})
	}
}

func definitionMaterialV1(t *testing.T) contract.ToolDefinitionMaterialV1 {
	t.Helper()
	schema := []byte(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`)
	description := "Look up weather"
	tool := testkit.Tool()
	ref, err := contract.DeriveToolDefinitionMaterialRefV1(
		contract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest},
		runtimeports.SchemaRefV2{Namespace: "tool", Name: "weather", Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes(schema)},
		core.DigestBytes([]byte(description)),
	)
	if err != nil {
		t.Fatal(err)
	}
	return contract.ToolDefinitionMaterialV1{Ref: ref, Description: description, InputSchema: schema}
}

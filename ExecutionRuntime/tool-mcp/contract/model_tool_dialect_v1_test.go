package contract_test

import (
	"testing"

	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

func TestPortableFunctionToolProfileV1AcceptsStrictPortableSchema(t *testing.T) {
	if err := toolcontract.ValidatePortableFunctionToolNameV1("workspace_edit"); err != nil {
		t.Fatal(err)
	}
	schema := []byte(`{"type":"object","properties":{"path":{"type":"string"},"options":{"type":"object","properties":{},"required":[],"additionalProperties":false}},"required":["path","options"],"additionalProperties":false}`)
	if err := toolcontract.ValidatePortableFunctionToolSchemaV1(schema); err != nil {
		t.Fatal(err)
	}
}

func TestPortableFunctionToolProfileV1RejectsNonPortableExpression(t *testing.T) {
	for _, name := range []string{"1starts_with_digit", "contains.dot", "contains:colon", "contains space", "tool_name_that_is_deliberately_longer_than_the_portable_sixty_four_character_limit"} {
		if err := toolcontract.ValidatePortableFunctionToolNameV1(name); err == nil {
			t.Fatalf("non-portable Tool name %q was accepted", name)
		}
	}
	for _, schema := range [][]byte{
		[]byte(`{"type":"object","properties":{"value":{"type":"string"}}}`),
		[]byte(`{"type":"object","properties":{"value":{"type":"string"}},"required":[],"additionalProperties":false}`),
		[]byte(`{"type":"object","properties":{},"required":[],"additionalProperties":true}`),
		[]byte(`{"type":"object","properties":{},"required":[],"additionalProperties":false,"allOf":[{}]}`),
		[]byte(`{"type":"object","properties":{},"required":[],"additionalProperties":false,"x-vendor":true}`),
	} {
		if err := toolcontract.ValidatePortableFunctionToolSchemaV1(schema); err == nil {
			t.Fatalf("non-portable Tool schema was accepted: %s", schema)
		}
	}
}

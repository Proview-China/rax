package mcp_test

import (
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
)

func TestSchemaValidation(t *testing.T) {
	valid := []byte(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
	if _, err := mcp.ValidateToolSchema(valid); err != nil {
		t.Fatal(err)
	}
	invalid := [][]byte{
		[]byte(`{"type":"array"}`),
		[]byte(`{"type":"object","properties":{},"required":["missing"]}`),
		[]byte(`{"type":"object","type":"object"}`),
	}
	for _, payload := range invalid {
		if _, err := mcp.ValidateToolSchema(payload); err == nil {
			t.Fatalf("invalid schema accepted: %s", payload)
		}
	}
}

func TestSchemaDepthLimit(t *testing.T) {
	payload := `{"type":"object","properties":{"x":`
	payload += strings.Repeat(`{"properties":{"x":`, mcp.MaxSchemaDepth+2)
	payload += `{"type":"string"}`
	payload += strings.Repeat(`}}`, mcp.MaxSchemaDepth+2)
	payload += `}}`
	if _, err := mcp.ValidateToolSchema([]byte(payload)); err == nil {
		t.Fatal("over-deep schema was accepted")
	}
}

func FuzzValidateToolSchema(f *testing.F) {
	f.Add([]byte(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"]}`))
	f.Add([]byte(`{"type":"array"}`))
	f.Fuzz(func(t *testing.T, payload []byte) {
		digest, err := mcp.ValidateToolSchema(payload)
		if err != nil {
			return
		}
		if digest != core.DigestBytes(payload) {
			t.Fatal("accepted MCP schema returned a non-exact digest")
		}
		again, err := mcp.ValidateToolSchema(append([]byte(nil), payload...))
		if err != nil || again != digest {
			t.Fatalf("accepted MCP schema was not deterministic: digest=%s again=%s err=%v", digest, again, err)
		}
	})
}

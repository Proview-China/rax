package api

import (
	"context"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
)

func TestMCPToolMappingReadV1ExactDoubleRead(t *testing.T) {
	f := testkit.MCPToolMappingFixture(testkit.FixedTime)
	store := registry.New()
	if _, err := store.SubmitCapability(f.Capability, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SubmitTool(f.Tool, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SubmitMCPToolMapping(f.Mapping, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	read, err := NewMCPToolMappingReadV1(store)
	if err != nil {
		t.Fatal(err)
	}
	got, err := read.InspectMCPToolMappingManifestV1(context.Background(), f.Mapping.Ref)
	if err != nil || got != f.Mapping {
		t.Fatalf("got=%#v err=%v", got, err)
	}
	if _, err = read.InspectMCPToolMappingManifestV1(nil, f.Mapping.Ref); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil context error=%v", err)
	}
}

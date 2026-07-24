package blackbox_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	toolapi "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/api"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

func TestBlackboxCatalogExactInspectJSONRoundTripV1(t *testing.T) {
	store := registry.New()
	capability := testkit.Capability()
	if _, err := store.SubmitCapability(capability, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	tool := testkit.Tool()
	if _, err := store.SubmitTool(tool, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	client, err := sdk.NewV1(store, func() time.Time { return testkit.FixedTime })
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := toolapi.NewCatalogV1(client)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := catalog.InspectRegistryObjectV1(context.Background(), toolapi.InspectRegistryObjectRequestV1{
		Kind:  "tool",
		Exact: toolcontract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest},
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	var decoded toolapi.RegistryObjectProjectionV1
	if err = json.Unmarshal(payload, &decoded); err != nil || decoded.Validate() != nil || decoded.Tool == nil || decoded.Tool.ID != tool.ID || decoded.Capability != nil || decoded.Package != nil || decoded.ToolAlias != nil {
		t.Fatalf("transport-neutral exact projection drifted: projection=%#v err=%v", decoded, err)
	}
}

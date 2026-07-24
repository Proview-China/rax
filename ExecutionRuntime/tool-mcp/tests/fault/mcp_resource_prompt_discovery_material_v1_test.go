package fault_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

func TestFaultMCPResourcePromptDiscoveryMaterialV1SecondReadDrift(t *testing.T) {
	source := &faultMCPResourcePromptMaterialSourceV1{resource: testkit.MCPResourceDiscoveryMaterialV1(), prompt: testkit.MCPPromptDiscoveryMaterialV1()}
	resources, _ := sdk.NewMCPResourceDiscoveryMaterialV1(source)
	if _, err := resources.InspectExactMCPResourceDiscoveryMaterialV1(context.Background(), source.resource.Ref); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("Resource second-read drift error=%v", err)
	}
	prompts, _ := sdk.NewMCPPromptDiscoveryMaterialV1(source)
	if _, err := prompts.InspectExactMCPPromptDiscoveryMaterialV1(context.Background(), source.prompt.Ref); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("Prompt second-read drift error=%v", err)
	}
}

type faultMCPResourcePromptMaterialSourceV1 struct {
	resource      toolcontract.MCPResourceDiscoveryMaterialV1
	prompt        toolcontract.MCPPromptDiscoveryMaterialV1
	resourceCalls atomic.Uint64
	promptCalls   atomic.Uint64
}

func (s *faultMCPResourcePromptMaterialSourceV1) InspectExactMCPResourceDiscoveryMaterialV1(context.Context, toolcontract.MCPResourceDiscoveryMaterialRefV1) (toolcontract.MCPResourceDiscoveryMaterialV1, error) {
	value := s.resource.Clone()
	if s.resourceCalls.Add(1) > 1 {
		value.CanonicalObject[0] = '['
	}
	return value, nil
}

func (s *faultMCPResourcePromptMaterialSourceV1) InspectExactMCPPromptDiscoveryMaterialV1(context.Context, toolcontract.MCPPromptDiscoveryMaterialRefV1) (toolcontract.MCPPromptDiscoveryMaterialV1, error) {
	value := s.prompt.Clone()
	if s.promptCalls.Add(1) > 1 {
		value.CanonicalObject[0] = '['
	}
	return value, nil
}

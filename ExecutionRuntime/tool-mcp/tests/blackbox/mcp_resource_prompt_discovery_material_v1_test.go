package blackbox_test

import (
	"encoding/json"
	"testing"

	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestBlackboxMCPResourcePromptDiscoveryMaterialV1RoundTrip(t *testing.T) {
	resource := testkit.MCPResourceDiscoveryMaterialV1()
	resourceWire, err := json.Marshal(resource)
	if err != nil {
		t.Fatal(err)
	}
	var decodedResource toolcontract.MCPResourceDiscoveryMaterialV1
	if err = json.Unmarshal(resourceWire, &decodedResource); err != nil || decodedResource.Validate() != nil || decodedResource.Ref != resource.Ref {
		t.Fatalf("Resource round trip=%#v err=%v", decodedResource, err)
	}
	prompt := testkit.MCPPromptDiscoveryMaterialV1()
	promptWire, err := json.Marshal(prompt)
	if err != nil {
		t.Fatal(err)
	}
	var decodedPrompt toolcontract.MCPPromptDiscoveryMaterialV1
	if err = json.Unmarshal(promptWire, &decodedPrompt); err != nil || decodedPrompt.Validate() != nil || decodedPrompt.Ref != prompt.Ref {
		t.Fatalf("Prompt round trip=%#v err=%v", decodedPrompt, err)
	}
	decodedResource.CanonicalObject = []byte(`{"name":"readme","uri":"file:///other"}`)
	if decodedResource.Validate() == nil {
		t.Fatal("Resource canonical tamper was accepted")
	}
	decodedPrompt.CanonicalObject = []byte(`{"name":"other"}`)
	if decodedPrompt.Validate() == nil {
		t.Fatal("Prompt canonical tamper was accepted")
	}
}

func TestBlackboxMCPResourcePromptDiscoveryMaterialSetV1(t *testing.T) {
	resourceSet := testkit.MCPDiscoveryPageResourceMaterialSetV1()
	promptSet := testkit.MCPDiscoveryPagePromptMaterialSetV1()
	if resourceSet.Validate() != nil || promptSet.Validate() != nil || len(resourceSet.Entries) != 1 || len(promptSet.Entries) != 1 {
		t.Fatalf("typed Material Sets are invalid: resource=%#v prompt=%#v", resourceSet, promptSet)
	}
	drift := toolcontract.CloneMCPDiscoveryPageResourceMaterialSetV1(resourceSet)
	drift.Entries[0].Source.URI = "file:///other"
	if drift.Validate() == nil {
		t.Fatal("Resource Material Set source drift was accepted")
	}
	promptDrift := toolcontract.CloneMCPDiscoveryPagePromptMaterialSetV1(promptSet)
	promptDrift.Entries[0].Source.Name = "other"
	if promptDrift.Validate() == nil {
		t.Fatal("Prompt Material Set source drift was accepted")
	}
}

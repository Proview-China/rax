package sdk_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

func TestMCPResourcePromptDiscoveryMaterialSDKV1(t *testing.T) {
	source := newSDKMCPResourcePromptMaterialSourceV1()
	resources, err := sdk.NewMCPResourceDiscoveryMaterialV1(source)
	if err != nil {
		t.Fatal(err)
	}
	prompts, err := sdk.NewMCPPromptDiscoveryMaterialV1(source)
	if err != nil {
		t.Fatal(err)
	}
	resourceSets, err := sdk.NewMCPDiscoveryPageResourceMaterialSetV1(source)
	if err != nil {
		t.Fatal(err)
	}
	promptSets, err := sdk.NewMCPDiscoveryPagePromptMaterialSetV1(source)
	if err != nil {
		t.Fatal(err)
	}
	resource, err := resources.InspectExactMCPResourceDiscoveryMaterialV1(context.Background(), source.resource.Ref)
	if err != nil || resource.Ref != source.resource.Ref {
		t.Fatalf("Resource material=%#v err=%v", resource, err)
	}
	prompt, err := prompts.InspectExactMCPPromptDiscoveryMaterialV1(context.Background(), source.prompt.Ref)
	if err != nil || prompt.Ref != source.prompt.Ref {
		t.Fatalf("Prompt material=%#v err=%v", prompt, err)
	}
	if set, inspectErr := resourceSets.InspectMCPDiscoveryPageResourceMaterialSetV1(context.Background(), source.resourceSet.Receipt); inspectErr != nil || len(set.Entries) != 1 || set.Entries[0].Material != source.resource.Ref {
		t.Fatalf("Resource material set=%#v err=%v", set, inspectErr)
	}
	if set, inspectErr := promptSets.InspectMCPDiscoveryPagePromptMaterialSetV1(context.Background(), source.promptSet.Receipt); inspectErr != nil || len(set.Entries) != 1 || set.Entries[0].Material != source.prompt.Ref {
		t.Fatalf("Prompt material set=%#v err=%v", set, inspectErr)
	}
	resource.CanonicalObject[0] = '['
	prompt.CanonicalObject[0] = '['
	if again, inspectErr := resources.InspectExactMCPResourceDiscoveryMaterialV1(context.Background(), source.resource.Ref); inspectErr != nil || again.Validate() != nil {
		t.Fatalf("Resource material was not deep-cloned: value=%#v err=%v", again, inspectErr)
	}
	if again, inspectErr := prompts.InspectExactMCPPromptDiscoveryMaterialV1(context.Background(), source.prompt.Ref); inspectErr != nil || again.Validate() != nil {
		t.Fatalf("Prompt material was not deep-cloned: value=%#v err=%v", again, inspectErr)
	}
	const workers = 64
	var group sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			_, resourceErr := resources.InspectExactMCPResourceDiscoveryMaterialV1(context.Background(), source.resource.Ref)
			_, promptErr := prompts.InspectExactMCPPromptDiscoveryMaterialV1(context.Background(), source.prompt.Ref)
			if resourceErr != nil {
				errs <- resourceErr
				return
			}
			errs <- promptErr
		}()
	}
	group.Wait()
	close(errs)
	for inspectErr := range errs {
		if inspectErr != nil {
			t.Fatal(inspectErr)
		}
	}
}

func TestMCPResourcePromptDiscoveryMaterialSDKV1FailsClosed(t *testing.T) {
	var typedNil *sdkMCPResourcePromptMaterialSourceV1
	if _, err := sdk.NewMCPResourceDiscoveryMaterialV1(typedNil); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed-nil Resource constructor error=%v", err)
	}
	if _, err := sdk.NewMCPPromptDiscoveryMaterialV1(typedNil); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed-nil Prompt constructor error=%v", err)
	}
	if _, err := sdk.NewMCPDiscoveryPageResourceMaterialSetV1(typedNil); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed-nil Resource set constructor error=%v", err)
	}
	if _, err := sdk.NewMCPDiscoveryPagePromptMaterialSetV1(typedNil); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed-nil Prompt set constructor error=%v", err)
	}
	source := newSDKMCPResourcePromptMaterialSourceV1()
	resources, _ := sdk.NewMCPResourceDiscoveryMaterialV1(source)
	prompts, _ := sdk.NewMCPPromptDiscoveryMaterialV1(source)
	if _, err := resources.InspectExactMCPResourceDiscoveryMaterialV1(nil, source.resource.Ref); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil Resource context error=%v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := prompts.InspectExactMCPPromptDiscoveryMaterialV1(canceled, source.prompt.Ref); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Prompt context error=%v", err)
	}
	source.tamper = true
	if _, err := resources.InspectExactMCPResourceDiscoveryMaterialV1(context.Background(), source.resource.Ref); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("Resource source drift error=%v", err)
	}
	if _, err := prompts.InspectExactMCPPromptDiscoveryMaterialV1(context.Background(), source.prompt.Ref); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("Prompt source drift error=%v", err)
	}
}

type sdkMCPResourcePromptMaterialSourceV1 struct {
	resource    toolcontract.MCPResourceDiscoveryMaterialV1
	prompt      toolcontract.MCPPromptDiscoveryMaterialV1
	resourceSet toolcontract.MCPDiscoveryPageResourceMaterialSetV1
	promptSet   toolcontract.MCPDiscoveryPagePromptMaterialSetV1
	tamper      bool
}

func newSDKMCPResourcePromptMaterialSourceV1() *sdkMCPResourcePromptMaterialSourceV1 {
	return &sdkMCPResourcePromptMaterialSourceV1{resource: testkit.MCPResourceDiscoveryMaterialV1(), prompt: testkit.MCPPromptDiscoveryMaterialV1(), resourceSet: testkit.MCPDiscoveryPageResourceMaterialSetV1(), promptSet: testkit.MCPDiscoveryPagePromptMaterialSetV1()}
}

func (s *sdkMCPResourcePromptMaterialSourceV1) InspectExactMCPResourceDiscoveryMaterialV1(context.Context, toolcontract.MCPResourceDiscoveryMaterialRefV1) (toolcontract.MCPResourceDiscoveryMaterialV1, error) {
	value := s.resource.Clone()
	if s.tamper {
		value.CanonicalObject[0] = '['
	}
	return value, nil
}

func (s *sdkMCPResourcePromptMaterialSourceV1) InspectExactMCPPromptDiscoveryMaterialV1(context.Context, toolcontract.MCPPromptDiscoveryMaterialRefV1) (toolcontract.MCPPromptDiscoveryMaterialV1, error) {
	value := s.prompt.Clone()
	if s.tamper {
		value.CanonicalObject[0] = '['
	}
	return value, nil
}

func (s *sdkMCPResourcePromptMaterialSourceV1) InspectMCPDiscoveryPageResourceMaterialSetV1(context.Context, toolcontract.ObjectRef) (toolcontract.MCPDiscoveryPageResourceMaterialSetV1, error) {
	return toolcontract.CloneMCPDiscoveryPageResourceMaterialSetV1(s.resourceSet), nil
}

func (s *sdkMCPResourcePromptMaterialSourceV1) InspectMCPDiscoveryPagePromptMaterialSetV1(context.Context, toolcontract.ObjectRef) (toolcontract.MCPDiscoveryPagePromptMaterialSetV1, error) {
	return toolcontract.CloneMCPDiscoveryPagePromptMaterialSetV1(s.promptSet), nil
}

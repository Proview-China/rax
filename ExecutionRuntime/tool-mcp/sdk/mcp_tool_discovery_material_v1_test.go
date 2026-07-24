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

func TestMCPToolDiscoveryMaterialSDKV1ExactRead(t *testing.T) {
	source := &sdkMCPToolMaterialSourceV1{value: testkit.MCPToolDiscoveryMaterialV1()}
	client, err := sdk.NewMCPToolDiscoveryMaterialV1(source)
	if err != nil {
		t.Fatal(err)
	}
	value, err := client.InspectExactMCPToolDiscoveryMaterialV1(context.Background(), source.value.Ref)
	if err != nil || value.Ref != source.value.Ref {
		t.Fatalf("exact material=%#v err=%v", value, err)
	}
	value.CanonicalObject[0] = '['
	again, err := client.InspectExactMCPToolDiscoveryMaterialV1(context.Background(), source.value.Ref)
	if err != nil || string(again.CanonicalObject) != string(source.value.CanonicalObject) {
		t.Fatalf("material was not deep-cloned: value=%#v err=%v", again, err)
	}
	const workers = 64
	var group sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			_, inspectErr := client.InspectExactMCPToolDiscoveryMaterialV1(context.Background(), source.value.Ref)
			errs <- inspectErr
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

func TestMCPToolDiscoveryMaterialSDKV1FailsClosed(t *testing.T) {
	var typedNil *sdkMCPToolMaterialSourceV1
	if _, err := sdk.NewMCPToolDiscoveryMaterialV1(typedNil); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed-nil constructor error=%v", err)
	}
	source := &sdkMCPToolMaterialSourceV1{value: testkit.MCPToolDiscoveryMaterialV1()}
	client, _ := sdk.NewMCPToolDiscoveryMaterialV1(source)
	if _, err := client.InspectExactMCPToolDiscoveryMaterialV1(nil, source.value.Ref); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil context error=%v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.InspectExactMCPToolDiscoveryMaterialV1(canceled, source.value.Ref); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context error=%v", err)
	}
	wrong := source.value.Ref
	wrong.Digest = testkit.Digest("wrong-sdk-material")
	if _, err := client.InspectExactMCPToolDiscoveryMaterialV1(context.Background(), wrong); err == nil {
		t.Fatal("wrong exact Ref was accepted")
	}
	source.tamper = true
	if _, err := client.InspectExactMCPToolDiscoveryMaterialV1(context.Background(), source.value.Ref); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("source drift error=%v", err)
	}
}

type sdkMCPToolMaterialSourceV1 struct {
	value  toolcontract.MCPToolDiscoveryMaterialV1
	tamper bool
}

func (s *sdkMCPToolMaterialSourceV1) InspectExactMCPToolDiscoveryMaterialV1(context.Context, toolcontract.MCPToolDiscoveryMaterialRefV1) (toolcontract.MCPToolDiscoveryMaterialV1, error) {
	value := s.value.Clone()
	if s.tamper {
		value.CanonicalObject[0] = '['
	}
	return value, nil
}

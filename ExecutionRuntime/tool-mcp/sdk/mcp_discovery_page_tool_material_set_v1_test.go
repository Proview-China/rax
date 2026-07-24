package sdk_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

func TestMCPDiscoveryPageToolMaterialSetSDKV1(t *testing.T) {
	source := &sdkMCPPageMaterialSetSourceV1{value: testkit.MCPDiscoveryPageToolMaterialSetV1()}
	client, err := sdk.NewMCPDiscoveryPageToolMaterialSetV1(source)
	if err != nil {
		t.Fatal(err)
	}
	value, err := client.InspectMCPDiscoveryPageToolMaterialSetV1(context.Background(), source.value.Receipt)
	if err != nil || value.Ref != source.value.Ref || len(value.Entries) != 1 {
		t.Fatalf("material set=%#v err=%v", value, err)
	}
	value.Entries[0].Source.Name = "mutated"
	again, err := client.InspectMCPDiscoveryPageToolMaterialSetV1(context.Background(), source.value.Receipt)
	if err != nil || again.Entries[0].Source.Name != "echo" {
		t.Fatalf("material set was not deep-cloned: set=%#v err=%v", again, err)
	}
	var typedNil *sdkMCPPageMaterialSetSourceV1
	if _, err = sdk.NewMCPDiscoveryPageToolMaterialSetV1(typedNil); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed nil error=%v", err)
	}
	if _, err = client.InspectMCPDiscoveryPageToolMaterialSetV1(nil, source.value.Receipt); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil context error=%v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = client.InspectMCPDiscoveryPageToolMaterialSetV1(canceled, source.value.Receipt); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context error=%v", err)
	}
	wrong := source.value.Receipt
	wrong.Digest = testkit.Digest("wrong-set-receipt")
	if _, err = client.InspectMCPDiscoveryPageToolMaterialSetV1(context.Background(), wrong); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("wrong exact receipt error=%v", err)
	}
}

type sdkMCPPageMaterialSetSourceV1 struct {
	value toolcontract.MCPDiscoveryPageToolMaterialSetV1
}

func (s *sdkMCPPageMaterialSetSourceV1) InspectMCPDiscoveryPageToolMaterialSetV1(context.Context, toolcontract.ObjectRef) (toolcontract.MCPDiscoveryPageToolMaterialSetV1, error) {
	return toolcontract.CloneMCPDiscoveryPageToolMaterialSetV1(s.value), nil
}

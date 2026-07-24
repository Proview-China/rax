package api_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/api"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestMCPDiscoveryPageToolMaterialSetReadAPIV1(t *testing.T) {
	source := &apiMCPPageMaterialSetSourceV1{value: testkit.MCPDiscoveryPageToolMaterialSetV1()}
	read, err := api.NewMCPDiscoveryPageToolMaterialSetReadV1(source)
	if err != nil {
		t.Fatal(err)
	}
	value, err := read.InspectMCPDiscoveryPageToolMaterialSetV1(context.Background(), source.value.Receipt)
	if err != nil || value.Ref != source.value.Ref || len(value.Entries) != 1 {
		t.Fatalf("material set=%#v err=%v", value, err)
	}
	var typedNil *apiMCPPageMaterialSetSourceV1
	if _, err = api.NewMCPDiscoveryPageToolMaterialSetReadV1(typedNil); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed nil error=%v", err)
	}
	if _, err = read.InspectMCPDiscoveryPageToolMaterialSetV1(nil, source.value.Receipt); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil context error=%v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = read.InspectMCPDiscoveryPageToolMaterialSetV1(canceled, source.value.Receipt); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context error=%v", err)
	}
	wrong := source.value.Receipt
	wrong.Digest = testkit.Digest("wrong-api-set-receipt")
	if _, err = read.InspectMCPDiscoveryPageToolMaterialSetV1(context.Background(), wrong); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("wrong exact receipt error=%v", err)
	}
}

type apiMCPPageMaterialSetSourceV1 struct {
	value toolcontract.MCPDiscoveryPageToolMaterialSetV1
}

func (s *apiMCPPageMaterialSetSourceV1) InspectMCPDiscoveryPageToolMaterialSetV1(context.Context, toolcontract.ObjectRef) (toolcontract.MCPDiscoveryPageToolMaterialSetV1, error) {
	return toolcontract.CloneMCPDiscoveryPageToolMaterialSetV1(s.value), nil
}

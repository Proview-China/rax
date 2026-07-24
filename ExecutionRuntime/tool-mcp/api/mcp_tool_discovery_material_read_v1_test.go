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

func TestMCPToolDiscoveryMaterialReadAPIV1(t *testing.T) {
	source := &apiMCPToolMaterialSourceV1{value: testkit.MCPToolDiscoveryMaterialV1()}
	read, err := api.NewMCPToolDiscoveryMaterialReadV1(source)
	if err != nil {
		t.Fatal(err)
	}
	value, err := read.InspectExactMCPToolDiscoveryMaterialV1(context.Background(), source.value.Ref)
	if err != nil || value.Ref != source.value.Ref {
		t.Fatalf("exact material=%#v err=%v", value, err)
	}
	value.CanonicalObject[0] = '['
	again, err := read.InspectExactMCPToolDiscoveryMaterialV1(context.Background(), source.value.Ref)
	if err != nil || string(again.CanonicalObject) != string(source.value.CanonicalObject) {
		t.Fatalf("material was not deep-cloned: value=%#v err=%v", again, err)
	}
}

func TestMCPToolDiscoveryMaterialReadAPIV1FailsClosed(t *testing.T) {
	var typedNil *apiMCPToolMaterialSourceV1
	if _, err := api.NewMCPToolDiscoveryMaterialReadV1(typedNil); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed-nil constructor error=%v", err)
	}
	source := &apiMCPToolMaterialSourceV1{value: testkit.MCPToolDiscoveryMaterialV1()}
	read, _ := api.NewMCPToolDiscoveryMaterialReadV1(source)
	if _, err := read.InspectExactMCPToolDiscoveryMaterialV1(nil, source.value.Ref); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil context error=%v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := read.InspectExactMCPToolDiscoveryMaterialV1(canceled, source.value.Ref); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context error=%v", err)
	}
	source.tamper = true
	if _, err := read.InspectExactMCPToolDiscoveryMaterialV1(context.Background(), source.value.Ref); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("source drift error=%v", err)
	}
}

type apiMCPToolMaterialSourceV1 struct {
	value  toolcontract.MCPToolDiscoveryMaterialV1
	tamper bool
}

func (s *apiMCPToolMaterialSourceV1) InspectExactMCPToolDiscoveryMaterialV1(context.Context, toolcontract.MCPToolDiscoveryMaterialRefV1) (toolcontract.MCPToolDiscoveryMaterialV1, error) {
	value := s.value.Clone()
	if s.tamper {
		value.CanonicalObject[0] = '['
	}
	return value, nil
}

package conformance_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tooladapter "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/applicationadapter"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

func TestConformanceP42PublicMethodSets(t *testing.T) {
	methodSets := []struct {
		name    string
		typeOf  reflect.Type
		methods []string
	}{
		{"Registry Object Current Reader", reflect.TypeOf((*toolcontract.ToolRegistryObjectCurrentReaderV1)(nil)).Elem(), []string{"ResolveExactToolCapabilityCurrentV1", "InspectExactToolCapabilityCurrentV1", "ResolveExactToolDescriptorCurrentV1", "InspectExactToolDescriptorCurrentV1"}},
		{"Input Contract Reader", reflect.TypeOf((*toolcontract.ToolInputContractCurrentReaderV1)(nil)).Elem(), []string{"ResolveToolInputContractCurrentV1", "InspectToolInputContractCurrentByIssuanceV1", "InspectExactToolInputContractCurrentV1"}},
		{"Input Contract Store", reflect.TypeOf((*toolcontract.ToolInputContractLeaseStoreV1)(nil)).Elem(), []string{"CreateToolInputContractCurrentOnceV1", "InspectToolInputContractCurrentByIssuanceIDV1", "InspectExactToolInputContractCurrentV1"}},
		{"BindingV2 Reader", reflect.TypeOf((*tooladapter.SingleCallToolActionBindingCurrentReaderV2)(nil)).Elem(), []string{"ResolveSingleCallToolActionBindingCurrentV2", "InspectSingleCallToolActionBindingCurrentByIssuanceV2", "InspectExactSingleCallToolActionBindingCurrentV2"}},
		{"BindingV2 Store", reflect.TypeOf((*tooladapter.SingleCallToolActionBindingLeaseStoreV2)(nil)).Elem(), []string{"CreateSingleCallToolActionBindingCurrentOnceV2", "InspectSingleCallToolActionBindingCurrentByIssuanceIDV2", "InspectExactSingleCallToolActionBindingCurrentV2"}},
	}
	for _, set := range methodSets {
		for _, name := range set.methods {
			if _, ok := set.typeOf.MethodByName(name); !ok {
				t.Fatalf("%s is missing %s", set.name, name)
			}
		}
	}
}

func TestConformanceP42ContractImportBoundary(t *testing.T) {
	root := filepath.Join("..", "..", "contract")
	for _, name := range []string{"input_contract_current_v1.go", "action_v3.go", "binding_current_v2.go", "registry_object_current_v1.go"} {
		body, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		for _, forbidden := range []string{"ExecutionRuntime/application/", "ExecutionRuntime/harness/", "/runtime/kernel", "/runtime/fakes", "net/http", "vendor/"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s imports forbidden boundary %q", name, forbidden)
			}
		}
	}
}

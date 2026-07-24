package conformance_test

import (
	"reflect"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/applicationadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/runtimeadapter"
)

func TestControlledProviderV2ImplementsOnlyTheToolNarrowSeam(t *testing.T) {
	var _ applicationadapter.ToolControlledProviderV2 = (*runtimeadapter.ControlledProviderV2)(nil)
	typeOf := reflect.TypeOf(runtimeadapter.ControlledProviderV2{})
	for index := 0; index < typeOf.NumField(); index++ {
		name := typeOf.Field(index).Name
		if name == "provider" || name == "transport" || name == "backend" || name == "raw" {
			t.Fatalf("Tool V2 adapter owns forbidden execution field %q", name)
		}
	}
}

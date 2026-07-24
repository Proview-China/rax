package assemblyadapter_test

import (
	"reflect"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblyadapter"
)

func TestControlledOperationProviderRouteInputsReaderIsPackageSealedV2(t *testing.T) {
	t.Parallel()
	typeOf := reflect.TypeOf((*assemblyadapter.ControlledOperationProviderRouteConformanceInputsReaderV2)(nil)).Elem()
	found := false
	for index := 0; index < typeOf.NumMethod(); index++ {
		method := typeOf.Method(index)
		if method.Name == "controlledOperationProviderRouteConformanceInputsOwnerV2" && method.PkgPath != "" {
			found = true
		}
	}
	if !found {
		t.Fatal("public InputsReader is externally implementable")
	}
}

func TestControlledOperationProviderRouteOwnerSourceIsPackageSealedV2(t *testing.T) {
	t.Parallel()
	typeOf := reflect.TypeOf((*assemblyadapter.ControlledOperationProviderRouteConformanceOwnerSourceV2)(nil)).Elem()
	found := false
	for index := 0; index < typeOf.NumMethod(); index++ {
		method := typeOf.Method(index)
		if method.Name == "controlledOperationProviderRouteConformanceOwnerSourceV2" && method.PkgPath != "" {
			found = true
		}
	}
	if !found {
		t.Fatal("public OwnerSource is externally implementable through FromOwner")
	}
}

package ports_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
)

func TestApplicationRuntimeImportConformanceV3(t *testing.T) {
	allowed := conformance.ApplicationAllowedRuntimeImportsV3()
	if len(allowed) != 2 {
		t.Fatalf("unexpected Application allowlist: %#v", allowed)
	}
	if err := conformance.CheckApplicationRuntimeImportsV3(allowed); err != nil {
		t.Fatalf("public Runtime imports rejected: %v", err)
	}
	for _, forbidden := range []string{
		"github.com/Proview-China/rax/ExecutionRuntime/runtime/control",
		"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel",
		"github.com/Proview-China/rax/ExecutionRuntime/runtime/foundation",
		"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes",
	} {
		if err := conformance.CheckApplicationRuntimeImportsV3([]string{forbidden}); err == nil {
			t.Fatalf("Application import conformance allowed %s", forbidden)
		}
	}
	copy := conformance.ApplicationAllowedRuntimeImportsV3()
	copy[0] = "mutated"
	if conformance.ApplicationAllowedRuntimeImportsV3()[0] == "mutated" {
		t.Fatal("Application import allowlist was externally mutable")
	}
}

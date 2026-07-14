package application_test

import (
	"testing"

	applicationconformance "github.com/Proview-China/rax/ExecutionRuntime/application/conformance"
)

func TestOperationDomainAdapterImportConformanceV3(t *testing.T) {
	allowed := applicationconformance.OperationDomainAdapterAllowedImportsV3()
	if len(allowed) != 4 {
		t.Fatalf("unexpected adapter allowlist: %#v", allowed)
	}
	allowed[0] = "mutated"
	if applicationconformance.OperationDomainAdapterAllowedImportsV3()[0] == "mutated" {
		t.Fatal("adapter import allowlist exposed mutable package state")
	}
	for _, testCase := range []struct {
		name    string
		imports []string
		ok      bool
	}{
		{name: "public-contracts-and-standard-library", imports: []string{"context", "sync", "github.com/Proview-China/rax/ExecutionRuntime/runtime/core", "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports", "github.com/Proview-China/rax/ExecutionRuntime/application/contract", "github.com/Proview-China/rax/ExecutionRuntime/application/ports"}, ok: true},
		{name: "runtime-control", imports: []string{"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"}},
		{name: "runtime-kernel", imports: []string{"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"}},
		{name: "runtime-foundation", imports: []string{"github.com/Proview-China/rax/ExecutionRuntime/runtime/foundation"}},
		{name: "runtime-fakes", imports: []string{"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"}},
		{name: "application-root", imports: []string{"github.com/Proview-China/rax/ExecutionRuntime/application"}},
		{name: "application-fakes", imports: []string{"github.com/Proview-China/rax/ExecutionRuntime/application/fakes"}},
		{name: "harness-internal", imports: []string{"github.com/Proview-China/rax/ExecutionRuntime/harness/kernel"}},
		{name: "other-component-implementation", imports: []string{"github.com/Proview-China/rax/ExecutionRuntime/memory/internal"}},
		{name: "empty", imports: []string{""}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := applicationconformance.CheckOperationDomainAdapterImportsV3(testCase.imports)
			if testCase.ok && err != nil {
				t.Fatal(err)
			}
			if !testCase.ok && err == nil {
				t.Fatal("forbidden adapter import passed conformance")
			}
		})
	}
}

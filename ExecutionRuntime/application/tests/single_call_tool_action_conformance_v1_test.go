package application_test

import (
	"testing"

	applicationconformance "github.com/Proview-China/rax/ExecutionRuntime/application/conformance"
)

func TestSingleCallToolActionAdapterImportConformanceV1(t *testing.T) {
	allowed := applicationconformance.SingleCallToolActionAdapterAllowedImportsV1()
	if len(allowed) != 4 {
		t.Fatalf("unexpected G6A adapter allowlist: %#v", allowed)
	}
	allowed[0] = "mutated"
	if applicationconformance.SingleCallToolActionAdapterAllowedImportsV1()[0] == "mutated" {
		t.Fatal("G6A adapter allowlist exposed mutable package state")
	}
	for _, testCase := range []struct {
		name    string
		imports []string
		ok      bool
	}{
		{name: "public", imports: []string{"context", "sync", "github.com/Proview-China/rax/ExecutionRuntime/runtime/core", "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports", "github.com/Proview-China/rax/ExecutionRuntime/application/contract", "github.com/Proview-China/rax/ExecutionRuntime/application/ports"}, ok: true},
		{name: "runtime-control", imports: []string{"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"}},
		{name: "runtime-fakes", imports: []string{"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"}},
		{name: "tool", imports: []string{"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/kernel"}},
		{name: "harness", imports: []string{"github.com/Proview-China/rax/ExecutionRuntime/harness/kernel"}},
		{name: "context", imports: []string{"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"}},
		{name: "model", imports: []string{"github.com/Proview-China/rax/ExecutionRuntime/model-invoker"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := applicationconformance.CheckSingleCallToolActionAdapterImportsV1(testCase.imports)
			if testCase.ok && err != nil {
				t.Fatal(err)
			}
			if !testCase.ok && err == nil {
				t.Fatal("forbidden G6A adapter import passed")
			}
		})
	}
}

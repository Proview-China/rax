package corepack

import (
	"context"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblysdk"
	assemblytestkit "github.com/Proview-China/rax/ExecutionRuntime/harness/tests/assembly/testkit"
)

func TestCorePackPreviewCompilesThroughHarnessPublicAssemblySDKV1(t *testing.T) {
	kit, request := previewAssemblyFixtureV1(t)
	result, err := kit.AssembleV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Surface.Entries) != 5 || result.Executable {
		t.Fatal("fixture did not retain five non-executable declarations")
	}
	input := assemblytestkit.ValidInput()
	input.Plan.ToolSurface = assemblycontract.ObjectRefV1{ID: result.Surface.ID, Revision: result.Surface.Revision, Digest: result.Surface.Digest}
	input, err = assemblycontract.SealAssemblyInputV1(input)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := assemblysdk.New().Compile(input)
	if err != nil {
		t.Fatal(err)
	}
	if compiled.Graph == nil || compiled.Manifest == nil || compiled.Manifest.Plan.ToolSurface != input.Plan.ToolSurface {
		t.Fatal("Harness did not compile the exact Core Pack Tool Surface Ref")
	}
	providerCallCount := 0
	if result.Executable {
		providerCallCount++
	}
	if providerCallCount != 0 || result.UnsupportedReason != CorePackUnsupportedReasonV1 {
		t.Fatalf("non-executable fixture reached a Provider: %d", providerCallCount)
	}
}

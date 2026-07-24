package assembly_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblysdk"
	assemblytestkit "github.com/Proview-China/rax/ExecutionRuntime/harness/tests/assembly/testkit"
)

func TestBlackBoxCompileInspectExplainDiff(t *testing.T) {
	t.Parallel()
	input := assemblytestkit.ValidInput()
	result, err := assemblysdk.New().Compile(input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Generation == nil || result.Manifest == nil || result.Graph == nil || result.Handoff == nil {
		t.Fatal("public compile omitted a sealed artifact")
	}
	if result.Generation.State != assemblycontract.AssemblyStateSealedV1 {
		t.Fatalf("unexpected state %q", result.Generation.State)
	}
	if explanation := assemblysdk.Explain(result, "praxis.fixture/model-turn"); !explanation.Found {
		t.Fatal("contribution was not explainable through public SDK")
	}
	if diff := assemblysdk.Diff(result, assemblysdk.Inspect(result)); diff.Changed {
		t.Fatalf("defensive inspection changed semantics: %+v", diff)
	}
}

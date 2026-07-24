package assemblysdk

import (
	"testing"

	assemblytestkit "github.com/Proview-China/rax/ExecutionRuntime/harness/tests/assembly/testkit"
)

func TestInspectExplainAndDiffAreReadOnly(t *testing.T) {
	t.Parallel()
	sdk := New()
	input := assemblytestkit.ValidInput()
	result, err := sdk.Compile(input)
	if err != nil {
		t.Fatal(err)
	}
	inspected := Inspect(result)
	var slotIndex int
	for index := range inspected.Graph.Slots {
		if len(inspected.Graph.Slots[index].Contributions) > 0 {
			slotIndex = index
			break
		}
	}
	inspected.Graph.Slots[slotIndex].Contributions[0] = "mutated"
	if result.Graph.Slots[slotIndex].Contributions[0] == "mutated" {
		t.Fatal("Inspect returned aliased graph state")
	}
	explanation := Explain(result, "model.turn")
	if !explanation.Found || explanation.Slot == nil {
		t.Fatal("model.turn explanation missing")
	}
	if diff := Diff(result, result); diff.Changed || len(diff.Changes) != 0 {
		t.Fatalf("identical result reported changes: %+v", diff)
	}
	changed := result
	changed = Inspect(result)
	changed.Graph.Digest = assemblytestkit.Digest("changed-graph")
	if diff := Diff(result, changed); !diff.Changed || len(diff.Changes) != 1 {
		t.Fatalf("graph change not reported: %+v", diff)
	}
}

func TestBuilderUsesPublicCatalogAndReturnsDefensiveInput(t *testing.T) {
	t.Parallel()
	input := assemblytestkit.ValidInput()
	input.Slots, input.HookFaces = nil, nil
	built, err := NewBuilder(input).UsePublicCatalogV1().Build()
	if err != nil {
		t.Fatal(err)
	}
	if len(built.Slots) == 0 || len(built.HookFaces) == 0 {
		t.Fatal("public catalog missing")
	}
}

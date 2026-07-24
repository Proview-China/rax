package kernel

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"sync"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/refstore"
)

func TestCompileDeterministicGolden(t *testing.T) {
	result1 := compileFixture(t, false, "tail-v1")
	result2 := compileFixture(t, true, "tail-v1")
	if !reflect.DeepEqual(result1, result2) {
		t.Fatal("candidate input order changed frozen facts")
	}
	manifestDigest, err := result1.Manifest.DigestValue()
	if err != nil {
		t.Fatal(err)
	}
	frameDigest, err := result1.Frame.DigestValue()
	if err != nil {
		t.Fatal(err)
	}
	golden := struct {
		ManifestDigest contract.Digest `json:"manifest_digest"`
		FrameDigest    contract.Digest `json:"frame_digest"`
		StablePrefix   contract.Digest `json:"stable_prefix_digest"`
		SemiStable     contract.Digest `json:"semi_stable_digest"`
		DynamicTail    contract.Digest `json:"dynamic_tail_digest"`
		Rendered       contract.Digest `json:"rendered_digest"`
	}{manifestDigest, frameDigest, result1.Frame.StablePrefix.Digest, result1.Frame.SemiStable.Digest, result1.Frame.DynamicTail.Digest, result1.Frame.Rendered.Digest}
	got, err := json.MarshalIndent(golden, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	want, err := os.ReadFile("testdata/frame.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("golden mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestDynamicChangeKeepsStablePrefix(t *testing.T) {
	first := compileFixture(t, false, "tail-v1")
	second := compileFixture(t, false, "tail-v2")
	if first.Frame.StablePrefix != second.Frame.StablePrefix {
		t.Fatalf("dynamic tail broke stable prefix: %#v %#v", first.Frame.StablePrefix, second.Frame.StablePrefix)
	}
	if first.Frame.DynamicTail == second.Frame.DynamicTail || first.Frame.Rendered == second.Frame.Rendered {
		t.Fatal("dynamic content change was not reflected")
	}
}

func TestOnlyStableInputCanChangeStablePrefix(t *testing.T) {
	baseline := compileFixture(t, false, "tail-v1")

	store := refstore.NewMemory()
	instruction, _ := store.Put([]byte("You are changed."))
	artifact, _ := store.Put([]byte("artifact-v2"))
	dynamic, _ := store.Put([]byte("tail-v1"))
	changedStable, err := Compile(store, compileRequest(testkit.Recipe(), []contract.ContextCandidate{
		testkit.Candidate("instruction", contract.FragmentInstruction, instruction, 20),
		testkit.Candidate("artifact", contract.FragmentArtifactInline, artifact, 10),
		testkit.Candidate("conversation", contract.FragmentConversation, dynamic, 8),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if baseline.Frame.StablePrefix == changedStable.Frame.StablePrefix {
		t.Fatal("stable instruction change did not change stable prefix")
	}

	store = refstore.NewMemory()
	instruction, _ = store.Put([]byte("You are deterministic."))
	artifact, _ = store.Put([]byte("artifact-v2"))
	dynamic, _ = store.Put([]byte("tail-v1"))
	changedSemi, err := Compile(store, compileRequest(testkit.Recipe(), []contract.ContextCandidate{
		testkit.Candidate("instruction", contract.FragmentInstruction, instruction, 20),
		testkit.Candidate("artifact", contract.FragmentArtifactInline, artifact, 10),
		testkit.Candidate("conversation", contract.FragmentConversation, dynamic, 8),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if baseline.Frame.StablePrefix != changedSemi.Frame.StablePrefix {
		t.Fatal("semi-stable input broke stable prefix")
	}
}

func TestInspectFrameRejectsAnyNonExactReference(t *testing.T) {
	store, result := exactFixture(t)
	if err := InspectFrame(store, result.Manifest, result.Frame); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*contract.ContextManifest, *contract.ContextFrame)
	}{
		{"manifest_digest", func(_ *contract.ContextManifest, frame *contract.ContextFrame) {
			frame.ManifestRef.Digest = testkit.D("wrong-manifest")
		}},
		{"source_set", func(_ *contract.ContextManifest, frame *contract.ContextFrame) {
			frame.SourceSetDigest = testkit.D("wrong-source-set")
		}},
		{"generation", func(_ *contract.ContextManifest, frame *contract.ContextFrame) {
			frame.GenerationID = "generation-drift"
		}},
		{"stable_prefix", func(_ *contract.ContextManifest, frame *contract.ContextFrame) {
			frame.StablePrefix, _ = store.Put([]byte("[]"))
		}},
		{"rendered", func(_ *contract.ContextManifest, frame *contract.ContextFrame) {
			frame.Rendered, _ = store.Put([]byte("{}"))
		}},
		{"fragment_content", func(manifest *contract.ContextManifest, frame *contract.ContextFrame) {
			manifest.Fragments[0].Content, _ = store.Put([]byte("tampered-fragment"))
			digest, _ := manifest.DigestValue()
			frame.ManifestRef.Digest = digest
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := result.Manifest
			manifest.Decisions = append([]contract.AdmissionDecision(nil), result.Manifest.Decisions...)
			manifest.Fragments = append([]contract.ContextFragment(nil), result.Manifest.Fragments...)
			frame := result.Frame
			test.mutate(&manifest, &frame)
			if err := InspectFrame(store, manifest, frame); err == nil {
				t.Fatal("non-exact frame reference was accepted")
			}
		})
	}
}

func TestCompileConcurrentDeterminism(t *testing.T) {
	store := refstore.NewMemory()
	instruction, _ := store.Put([]byte("You are deterministic."))
	artifact, _ := store.Put([]byte("artifact-v1"))
	dynamic, _ := store.Put([]byte("tail-v1"))
	candidates := []contract.ContextCandidate{
		testkit.Candidate("instruction", contract.FragmentInstruction, instruction, 20),
		testkit.Candidate("artifact", contract.FragmentArtifactInline, artifact, 10),
		testkit.Candidate("conversation", contract.FragmentConversation, dynamic, 8),
	}
	baseline, err := Compile(store, compileRequest(testkit.Recipe(), candidates))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for worker := 0; worker < 64; worker++ {
		worker := worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			permuted := append([]contract.ContextCandidate(nil), candidates...)
			shift := worker % len(permuted)
			permuted = append(permuted[shift:], permuted[:shift]...)
			got, compileErr := Compile(store, compileRequest(testkit.Recipe(), permuted))
			if compileErr != nil {
				t.Errorf("worker %d: %v", worker, compileErr)
				return
			}
			if !reflect.DeepEqual(got, baseline) {
				t.Errorf("worker %d produced non-deterministic frame", worker)
			}
		}()
	}
	wg.Wait()
}

func TestOptionalBudgetExclusionPreservesRequired(t *testing.T) {
	store := refstore.NewMemory()
	instruction, _ := store.Put([]byte("system"))
	tail, _ := store.Put([]byte("tail"))
	recipe := testkit.Recipe()
	recipe.Budget.TotalTokens = 100
	recipe.Budget.StablePrefixMax = 100
	recipe.Budget.SemiStableMax = 100
	recipe.Budget.DynamicTailMax = 100
	candidates := []contract.ContextCandidate{
		testkit.Candidate("optional-tail", contract.FragmentConversation, tail, 90),
		testkit.Candidate("required-instruction", contract.FragmentInstruction, instruction, 20),
	}
	result, err := Compile(store, compileRequest(recipe, candidates))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Manifest.Fragments) != 1 || result.Manifest.Fragments[0].Kind != contract.FragmentInstruction {
		t.Fatalf("required source not preserved: %#v", result.Manifest.Fragments)
	}
	if result.Manifest.Decisions[1].Disposition != contract.AdmissionExcluded {
		t.Fatalf("optional candidate not excluded: %#v", result.Manifest.Decisions)
	}
}

func TestRequiredBudgetFailsClosed(t *testing.T) {
	store := refstore.NewMemory()
	content, _ := store.Put([]byte("system"))
	recipe := testkit.Recipe()
	recipe.Budget.TotalTokens = 10
	recipe.Budget.StablePrefixMax = 10
	recipe.Budget.SemiStableMax = 10
	recipe.Budget.DynamicTailMax = 10
	_, err := Compile(store, compileRequest(recipe, []contract.ContextCandidate{testkit.Candidate("required", contract.FragmentInstruction, content, 20)}))
	if err == nil {
		t.Fatal("required over-budget candidate was admitted")
	}
}

func compileFixture(t *testing.T, reverse bool, tail string) CompileResult {
	t.Helper()
	store := refstore.NewMemory()
	instruction, _ := store.Put([]byte("You are deterministic."))
	artifact, _ := store.Put([]byte("artifact-v1"))
	dynamic, _ := store.Put([]byte(tail))
	candidates := []contract.ContextCandidate{
		testkit.Candidate("instruction", contract.FragmentInstruction, instruction, 20),
		testkit.Candidate("artifact", contract.FragmentArtifactInline, artifact, 10),
		testkit.Candidate("conversation", contract.FragmentConversation, dynamic, 8),
	}
	if reverse {
		candidates[0], candidates[2] = candidates[2], candidates[0]
	}
	result, err := Compile(store, compileRequest(testkit.Recipe(), candidates))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func exactFixture(t *testing.T) (*refstore.Memory, CompileResult) {
	t.Helper()
	store := refstore.NewMemory()
	instruction, _ := store.Put([]byte("You are deterministic."))
	artifact, _ := store.Put([]byte("artifact-v1"))
	dynamic, _ := store.Put([]byte("tail-v1"))
	result, err := Compile(store, compileRequest(testkit.Recipe(), []contract.ContextCandidate{
		testkit.Candidate("instruction", contract.FragmentInstruction, instruction, 20),
		testkit.Candidate("artifact", contract.FragmentArtifactInline, artifact, 10),
		testkit.Candidate("conversation", contract.FragmentConversation, dynamic, 8),
	}))
	if err != nil {
		t.Fatal(err)
	}
	return store, result
}

func compileRequest(recipe contract.ContextRecipe, candidates []contract.ContextCandidate) CompileRequest {
	return CompileRequest{
		AttemptID: "attempt-1", ManifestID: "manifest-1", FrameID: "frame-1", GenerationID: "generation-1", Generation: 1,
		Recipe: recipe, Execution: testkit.Execution(), Candidates: candidates,
		CreatedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + 1_000,
	}
}

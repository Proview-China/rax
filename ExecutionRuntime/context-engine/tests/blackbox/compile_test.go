package blackbox_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/refstore"
)

func TestPublicCompileProducesFrozenFrame(t *testing.T) {
	store := refstore.NewMemory()
	instruction, err := store.Put([]byte("authoritative instruction"))
	if err != nil {
		t.Fatal(err)
	}
	tail, err := store.Put([]byte("user turn"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := kernel.Compile(store, kernel.CompileRequest{
		AttemptID: "attempt-blackbox", ManifestID: "manifest-blackbox", FrameID: "frame-blackbox", GenerationID: "generation-blackbox", Generation: 1,
		Recipe: testkit.Recipe(), Execution: testkit.Execution(), Candidates: []contract.ContextCandidate{
			testkit.Candidate("tail", contract.FragmentConversation, tail, 9),
			testkit.Candidate("instruction", contract.FragmentInstruction, instruction, 12),
		}, CreatedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + 1_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := result.Frame.Validate(); err != nil {
		t.Fatal(err)
	}
	manifestDigest, _ := result.Manifest.DigestValue()
	if result.Frame.ManifestRef.Digest != manifestDigest || result.Frame.SourceSetDigest != result.Manifest.SourceSetDigest {
		t.Fatal("frame is not bound to exact manifest/source set")
	}
	stable, err := store.Get(result.Frame.StablePrefix)
	if err != nil || len(stable) == 0 {
		t.Fatalf("stable prefix unavailable: %q %v", stable, err)
	}
}

package framestore

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
)

func TestCommitReceiptRequiresCreatedV1(t *testing.T) {
	state := frameStoreUnitStateV1(t, []byte("unit content"))
	frameRef, _, _, err := stateRefsV1(state)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := sealCommitReceiptV1(CommitReceiptV1{
		OperationID: "unit-receipt", FrameRef: frameRef, Pointer: state.Pointer, Created: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt.Created = false
	if err := receipt.Validate(); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("Created=false accepted: %v", err)
	}
}

func TestDecodeLargeStateChecksCancellationAfterStrictDecodeV1(t *testing.T) {
	state := frameStoreUnitStateV1(t, append([]byte{1}, make([]byte, (4<<20)-1)...))
	payload, digest, err := encodeStateRowV1(state)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &cancelAfterErrChecksV1{cancelAt: 2}
	if _, err := decodeStateRowV1(ctx, payload, string(digest), state.Owner); !errors.Is(err, context.Canceled) {
		t.Fatalf("mid-decode cancellation classification drifted: %v", err)
	}
	if ctx.calls.Load() != 2 {
		t.Fatalf("work continued after cancellation boundary: calls=%d", ctx.calls.Load())
	}
}

type cancelAfterErrChecksV1 struct {
	calls    atomic.Int32
	cancelAt int32
}

func (c *cancelAfterErrChecksV1) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *cancelAfterErrChecksV1) Done() <-chan struct{}       { return nil }
func (c *cancelAfterErrChecksV1) Value(any) any               { return nil }
func (c *cancelAfterErrChecksV1) Err() error {
	if c.calls.Add(1) >= c.cancelAt {
		return context.Canceled
	}
	return nil
}

func frameStoreUnitStateV1(t *testing.T, content []byte) CurrentStateV1 {
	t.Helper()
	now := time.Unix(0, testkit.Now)
	store := testkit.NewMutableReferenceStoreV1()
	contentRef, err := store.Put(content)
	if err != nil {
		t.Fatal(err)
	}
	recipe := testkit.Recipe()
	recipe.ExpiresUnixNano = now.Add(time.Hour).UnixNano()
	compiled, err := kernel.Compile(store, kernel.CompileRequest{
		AttemptID: "unit-attempt", ManifestID: "unit-manifest", FrameID: "unit-frame",
		GenerationID: "unit-generation", Generation: 1, Recipe: recipe,
		Execution: testkit.Execution(), Candidates: []contract.ContextCandidate{
			testkit.Candidate("unit-candidate", contract.FragmentInstruction, contentRef, 10),
		},
		CreatedUnixNano: now.Add(-time.Second).UnixNano(),
		ExpiresUnixNano: now.Add(40 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	frameDigest, err := compiled.Frame.DigestValue()
	if err != nil {
		t.Fatal(err)
	}
	frameRef := contract.FactRef{ID: compiled.Frame.ID, Revision: compiled.Frame.Revision, Digest: frameDigest}
	generation := contract.ContextGeneration{
		ContractVersion: contract.Version, ID: compiled.Frame.GenerationID, Revision: 1,
		Ordinal: 1, RootFrame: frameRef, CreatedUnixNano: now.Add(-500 * time.Millisecond).UnixNano(),
	}
	generationDigest, err := generation.DigestValue()
	if err != nil {
		t.Fatal(err)
	}
	subject := contract.ContextParentFrameApplicabilitySubjectV1{
		ContractVersion: contract.Version, FrameRef: frameRef, ManifestRef: compiled.Frame.ManifestRef,
		GenerationRef:     contract.FactRef{ID: generation.ID, Revision: generation.Revision, Digest: generationDigest},
		GenerationOrdinal: 1, ExecutionScopeDigest: compiled.Frame.Execution.ScopeDigest,
		RunID:      compiled.Frame.Execution.RunID,
		SessionRef: contract.FactRef{ID: "unit-session", Revision: 1, Digest: testkit.D("unit-session")},
		Turn:       1, ParentFrameGenerationBindingDigest: testkit.D("unit-binding"),
		RecipeRef: compiled.Manifest.RecipeRef, AuthorityDigest: compiled.Frame.Execution.AuthorityDigest,
	}
	source, err := contract.SealContextParentFrameApplicabilitySourceCoordinateV1(subject)
	if err != nil {
		t.Fatal(err)
	}
	pointer, err := contract.SealContextGenerationCurrentPointerV1(contract.ContextGenerationCurrentPointerV1{
		ID: "unit-pointer", Revision: 1, ExecutionScopeDigest: subject.ExecutionScopeDigest,
		RunID: subject.RunID, SessionRef: subject.SessionRef, Turn: subject.Turn,
		GenerationRef: subject.GenerationRef, GenerationOrdinal: subject.GenerationOrdinal,
		ParentFrameGenerationBindingDigest: subject.ParentFrameGenerationBindingDigest,
		ExpiresUnixNano:                    now.Add(12 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	state := CurrentStateV1{
		Owner: testkit.Owner(),
		Binding: contract.ContextParentFrameSourceBindingV1{
			Source: source, Subject: subject,
			BindingExpiresUnixNano:   now.Add(25 * time.Second).UnixNano(),
			RecipeExpiresUnixNano:    now.Add(20 * time.Second).UnixNano(),
			AuthorityExpiresUnixNano: now.Add(18 * time.Second).UnixNano(),
		},
		Frame: compiled.Frame, Manifest: compiled.Manifest, Generation: generation, Pointer: pointer,
	}
	for ref := range requiredContentRefsV1(compiled.Manifest, compiled.Frame) {
		value, err := store.Get(ref)
		if err != nil {
			t.Fatal(err)
		}
		state.Contents = append(state.Contents, ContentValueV1{Ref: ref, Value: value})
	}
	state, err = normalizeStateV1(state)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

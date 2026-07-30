package testfixture

import (
	"fmt"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/framestore"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
)

// FrameStoreFixtureV1 is a test-only complete durable-store input. It is not a
// production backend, composition root, or authority source.
type FrameStoreFixtureV1 struct {
	Now    time.Time
	Owner  contract.OwnerRef
	Store  *testkit.MutableReferenceStoreV1
	Recipe contract.ContextRecipe
	State  framestore.CurrentStateV1
}

func NewFrameStoreFixtureV1() (FrameStoreFixtureV1, error) {
	parent, err := NewParentFrameFixtureV1(func() time.Time {
		return time.Unix(0, testkit.Now)
	}, 30*time.Second)
	if err != nil {
		return FrameStoreFixtureV1{}, err
	}
	refs := make(map[contract.ContentRef]struct{})
	for _, fragment := range parent.Manifest.Fragments {
		refs[fragment.Content] = struct{}{}
	}
	refs[parent.Frame.StablePrefix] = struct{}{}
	if parent.Frame.SemiStable != nil {
		refs[*parent.Frame.SemiStable] = struct{}{}
	}
	refs[parent.Frame.DynamicTail] = struct{}{}
	refs[parent.Frame.Rendered] = struct{}{}
	contents := make([]framestore.ContentValueV1, 0, len(refs))
	for ref := range refs {
		value, getErr := parent.Content.Get(ref)
		if getErr != nil {
			return FrameStoreFixtureV1{}, getErr
		}
		contents = append(contents, framestore.ContentValueV1{Ref: ref, Value: value})
	}
	return FrameStoreFixtureV1{
		Now:    parent.Now,
		Owner:  testkit.Owner(),
		Store:  parent.Content,
		Recipe: parent.Recipe,
		State: framestore.CurrentStateV1{
			Owner:      testkit.Owner(),
			Binding:    parent.Binding,
			Frame:      parent.Frame,
			Manifest:   parent.Manifest,
			Generation: parent.Generation,
			Pointer:    parent.Pointer,
			Contents:   contents,
		},
	}, nil
}

func AdvanceFrameStoreFixtureV1(base FrameStoreFixtureV1, suffix string) (FrameStoreFixtureV1, error) {
	if suffix == "" || len(base.State.Manifest.Fragments) == 0 {
		return FrameStoreFixtureV1{}, fmt.Errorf("%w: frame store advance fixture", contract.ErrInvalid)
	}
	parentFrameRef, _, _, err := frameStoreRefsV1(base.State)
	if err != nil {
		return FrameStoreFixtureV1{}, err
	}
	candidate := testkit.Candidate("candidate-frame-store-"+suffix, contract.FragmentInstruction, base.State.Manifest.Fragments[0].Content, 10)
	candidate.Execution = base.State.Frame.Execution
	compiled, err := kernel.Compile(base.Store, kernel.CompileRequest{
		AttemptID:       "attempt-frame-store-" + suffix,
		ManifestID:      "manifest-frame-store-" + suffix,
		FrameID:         "frame-store-" + suffix,
		GenerationID:    "generation-frame-store-" + suffix,
		Generation:      base.State.Generation.Ordinal + 1,
		Recipe:          base.Recipe,
		Execution:       base.State.Frame.Execution,
		ParentFrame:     &parentFrameRef,
		Candidates:      []contract.ContextCandidate{candidate},
		CreatedUnixNano: base.Now.Add(-500 * time.Millisecond).UnixNano(),
		ExpiresUnixNano: base.Now.Add(40 * time.Second).UnixNano(),
	})
	if err != nil {
		return FrameStoreFixtureV1{}, err
	}
	frameDigest, err := compiled.Frame.DigestValue()
	if err != nil {
		return FrameStoreFixtureV1{}, err
	}
	frameRef := contract.FactRef{ID: compiled.Frame.ID, Revision: compiled.Frame.Revision, Digest: frameDigest}
	generation := contract.ContextGeneration{
		ContractVersion: contract.Version,
		ID:              compiled.Frame.GenerationID,
		Revision:        1,
		Ordinal:         compiled.Frame.Generation,
		RootFrame:       frameRef,
		Parent:          &base.State.Generation.RootFrame,
		CreatedUnixNano: base.Now.Add(-400 * time.Millisecond).UnixNano(),
	}
	generationDigest, err := generation.DigestValue()
	if err != nil {
		return FrameStoreFixtureV1{}, err
	}
	generationRef := contract.FactRef{ID: generation.ID, Revision: generation.Revision, Digest: generationDigest}
	subject := contract.ContextParentFrameApplicabilitySubjectV1{
		ContractVersion:                    contract.Version,
		FrameRef:                           frameRef,
		ManifestRef:                        compiled.Frame.ManifestRef,
		GenerationRef:                      generationRef,
		GenerationOrdinal:                  generation.Ordinal,
		ExecutionScopeDigest:               compiled.Frame.Execution.ScopeDigest,
		RunID:                              compiled.Frame.Execution.RunID,
		SessionRef:                         base.State.Pointer.SessionRef,
		Turn:                               compiled.Frame.Execution.Turn,
		ParentFrameGenerationBindingDigest: testkit.D("parent-frame-generation-binding-" + suffix),
		RecipeRef:                          compiled.Manifest.RecipeRef,
		AuthorityDigest:                    compiled.Frame.Execution.AuthorityDigest,
	}
	source, err := contract.SealContextParentFrameApplicabilitySourceCoordinateV1(subject)
	if err != nil {
		return FrameStoreFixtureV1{}, err
	}
	binding := contract.ContextParentFrameSourceBindingV1{
		Source:                   source,
		Subject:                  subject,
		BindingExpiresUnixNano:   base.Now.Add(25 * time.Second).UnixNano(),
		RecipeExpiresUnixNano:    base.Now.Add(20 * time.Second).UnixNano(),
		AuthorityExpiresUnixNano: base.Now.Add(18 * time.Second).UnixNano(),
	}
	pointer, err := contract.SealContextGenerationCurrentPointerV1(contract.ContextGenerationCurrentPointerV1{
		ID:                                 base.State.Pointer.ID,
		Revision:                           base.State.Pointer.Revision + 1,
		ExecutionScopeDigest:               subject.ExecutionScopeDigest,
		RunID:                              subject.RunID,
		SessionRef:                         subject.SessionRef,
		Turn:                               subject.Turn,
		GenerationRef:                      subject.GenerationRef,
		GenerationOrdinal:                  subject.GenerationOrdinal,
		ParentFrameGenerationBindingDigest: subject.ParentFrameGenerationBindingDigest,
		ExpiresUnixNano:                    base.Now.Add(12 * time.Second).UnixNano(),
	})
	if err != nil {
		return FrameStoreFixtureV1{}, err
	}
	state := framestore.CurrentStateV1{
		Owner: base.Owner, Binding: binding, Frame: compiled.Frame,
		Manifest: compiled.Manifest, Generation: generation, Pointer: pointer,
	}
	state.Contents, err = frameStoreContentsV1(base.Store, compiled.Manifest, compiled.Frame)
	if err != nil {
		return FrameStoreFixtureV1{}, err
	}
	return FrameStoreFixtureV1{Now: base.Now, Owner: base.Owner, Store: base.Store, Recipe: base.Recipe, State: state}, nil
}

func CrossScopeSameFrameIDFixtureV1(base FrameStoreFixtureV1, suffix string) (FrameStoreFixtureV1, error) {
	if suffix == "" || len(base.State.Manifest.Fragments) == 0 {
		return FrameStoreFixtureV1{}, fmt.Errorf("%w: frame store cross-scope fixture", contract.ErrInvalid)
	}
	execution := base.State.Frame.Execution
	execution.ScopeDigest = testkit.D("cross-scope-" + suffix)
	candidate := testkit.Candidate("candidate-cross-scope-"+suffix, contract.FragmentInstruction, base.State.Manifest.Fragments[0].Content, 10)
	candidate.Execution = execution
	compiled, err := kernel.Compile(base.Store, kernel.CompileRequest{
		AttemptID:       "attempt-cross-scope-" + suffix,
		ManifestID:      "manifest-cross-scope-" + suffix,
		FrameID:         base.State.Frame.ID,
		GenerationID:    "generation-cross-scope-" + suffix,
		Generation:      1,
		Recipe:          base.Recipe,
		Execution:       execution,
		Candidates:      []contract.ContextCandidate{candidate},
		CreatedUnixNano: base.Now.Add(-500 * time.Millisecond).UnixNano(),
		ExpiresUnixNano: base.Now.Add(40 * time.Second).UnixNano(),
	})
	if err != nil {
		return FrameStoreFixtureV1{}, err
	}
	frameDigest, err := compiled.Frame.DigestValue()
	if err != nil {
		return FrameStoreFixtureV1{}, err
	}
	frameRef := contract.FactRef{ID: compiled.Frame.ID, Revision: compiled.Frame.Revision, Digest: frameDigest}
	generation := contract.ContextGeneration{
		ContractVersion: contract.Version,
		ID:              compiled.Frame.GenerationID,
		Revision:        1,
		Ordinal:         1,
		RootFrame:       frameRef,
		CreatedUnixNano: base.Now.Add(-400 * time.Millisecond).UnixNano(),
	}
	generationDigest, err := generation.DigestValue()
	if err != nil {
		return FrameStoreFixtureV1{}, err
	}
	generationRef := contract.FactRef{ID: generation.ID, Revision: generation.Revision, Digest: generationDigest}
	subject := contract.ContextParentFrameApplicabilitySubjectV1{
		ContractVersion:                    contract.Version,
		FrameRef:                           frameRef,
		ManifestRef:                        compiled.Frame.ManifestRef,
		GenerationRef:                      generationRef,
		GenerationOrdinal:                  generation.Ordinal,
		ExecutionScopeDigest:               execution.ScopeDigest,
		RunID:                              execution.RunID,
		SessionRef:                         base.State.Pointer.SessionRef,
		Turn:                               execution.Turn,
		ParentFrameGenerationBindingDigest: testkit.D("cross-scope-binding-" + suffix),
		RecipeRef:                          compiled.Manifest.RecipeRef,
		AuthorityDigest:                    execution.AuthorityDigest,
	}
	source, err := contract.SealContextParentFrameApplicabilitySourceCoordinateV1(subject)
	if err != nil {
		return FrameStoreFixtureV1{}, err
	}
	pointer, err := contract.SealContextGenerationCurrentPointerV1(contract.ContextGenerationCurrentPointerV1{
		ID:                                 "cross-scope-pointer-" + suffix,
		Revision:                           1,
		ExecutionScopeDigest:               subject.ExecutionScopeDigest,
		RunID:                              subject.RunID,
		SessionRef:                         subject.SessionRef,
		Turn:                               subject.Turn,
		GenerationRef:                      subject.GenerationRef,
		GenerationOrdinal:                  subject.GenerationOrdinal,
		ParentFrameGenerationBindingDigest: subject.ParentFrameGenerationBindingDigest,
		ExpiresUnixNano:                    base.Now.Add(12 * time.Second).UnixNano(),
	})
	if err != nil {
		return FrameStoreFixtureV1{}, err
	}
	state := framestore.CurrentStateV1{
		Owner: base.Owner,
		Binding: contract.ContextParentFrameSourceBindingV1{
			Source: source, Subject: subject,
			BindingExpiresUnixNano:   base.Now.Add(25 * time.Second).UnixNano(),
			RecipeExpiresUnixNano:    base.Now.Add(20 * time.Second).UnixNano(),
			AuthorityExpiresUnixNano: base.Now.Add(18 * time.Second).UnixNano(),
		},
		Frame: compiled.Frame, Manifest: compiled.Manifest, Generation: generation, Pointer: pointer,
	}
	state.Contents, err = frameStoreContentsV1(base.Store, compiled.Manifest, compiled.Frame)
	if err != nil {
		return FrameStoreFixtureV1{}, err
	}
	return FrameStoreFixtureV1{Now: base.Now, Owner: base.Owner, Store: base.Store, Recipe: base.Recipe, State: state}, nil
}

func frameStoreContentsV1(store *testkit.MutableReferenceStoreV1, manifest contract.ContextManifest, frame contract.ContextFrame) ([]framestore.ContentValueV1, error) {
	refs := make(map[contract.ContentRef]struct{})
	for _, fragment := range manifest.Fragments {
		refs[fragment.Content] = struct{}{}
	}
	refs[frame.StablePrefix] = struct{}{}
	if frame.SemiStable != nil {
		refs[*frame.SemiStable] = struct{}{}
	}
	refs[frame.DynamicTail] = struct{}{}
	refs[frame.Rendered] = struct{}{}
	contents := make([]framestore.ContentValueV1, 0, len(refs))
	for ref := range refs {
		value, err := store.Get(ref)
		if err != nil {
			return nil, err
		}
		contents = append(contents, framestore.ContentValueV1{Ref: ref, Value: value})
	}
	return contents, nil
}

func frameStoreRefsV1(state framestore.CurrentStateV1) (contract.FactRef, contract.FactRef, contract.FactRef, error) {
	frameDigest, err := state.Frame.DigestValue()
	if err != nil {
		return contract.FactRef{}, contract.FactRef{}, contract.FactRef{}, err
	}
	manifestDigest, err := state.Manifest.DigestValue()
	if err != nil {
		return contract.FactRef{}, contract.FactRef{}, contract.FactRef{}, err
	}
	generationDigest, err := state.Generation.DigestValue()
	if err != nil {
		return contract.FactRef{}, contract.FactRef{}, contract.FactRef{}, err
	}
	return contract.FactRef{ID: state.Frame.ID, Revision: state.Frame.Revision, Digest: frameDigest},
		contract.FactRef{ID: state.Manifest.ID, Revision: state.Manifest.Revision, Digest: manifestDigest},
		contract.FactRef{ID: state.Generation.ID, Revision: state.Generation.Revision, Digest: generationDigest}, nil
}

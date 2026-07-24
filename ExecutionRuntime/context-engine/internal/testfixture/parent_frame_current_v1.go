package testfixture

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
)

type ParentFrameFixtureV1 struct {
	Now        time.Time
	Content    *testkit.MutableReferenceStoreV1
	Metadata   *testkit.MetadataStoreV1
	Reader     *kernel.ParentFrameCurrentReaderV1
	Recipe     contract.ContextRecipe
	Manifest   contract.ContextManifest
	Frame      contract.ContextFrame
	Generation contract.ContextGeneration
	Pointer    contract.ContextGenerationCurrentPointerV1
	Binding    contract.ContextParentFrameSourceBindingV1
	Source     contract.ContextParentFrameApplicabilitySourceCoordinateV1
}

func NewParentFrameFixtureV1(clock func() time.Time, maxTTL time.Duration) (*ParentFrameFixtureV1, error) {
	return NewParentFrameFixtureWithRecipeV1(clock, maxTTL, testkit.Recipe())
}

func NewParentFrameFixtureWithRecipeV1(clock func() time.Time, maxTTL time.Duration, recipe contract.ContextRecipe) (*ParentFrameFixtureV1, error) {
	now := time.Unix(0, testkit.Now)
	content := testkit.NewMutableReferenceStoreV1()
	instruction, err := content.Put([]byte("stable instruction"))
	if err != nil {
		return nil, err
	}
	recipe.ExpiresUnixNano = now.Add(time.Hour).UnixNano()
	compiled, err := kernel.Compile(content, kernel.CompileRequest{
		AttemptID:       "attempt-parent-current-1",
		ManifestID:      "manifest-parent-current-1",
		FrameID:         "frame-parent-current-1",
		GenerationID:    "generation-parent-current-1",
		Generation:      1,
		Recipe:          recipe,
		Execution:       testkit.Execution(),
		Candidates:      []contract.ContextCandidate{testkit.Candidate("candidate-parent-current-1", contract.FragmentInstruction, instruction, 10)},
		CreatedUnixNano: now.Add(-time.Second).UnixNano(),
		ExpiresUnixNano: now.Add(40 * time.Second).UnixNano(),
	})
	if err != nil {
		return nil, err
	}
	frameDigest, err := compiled.Frame.DigestValue()
	if err != nil {
		return nil, err
	}
	frameRef := contract.FactRef{ID: compiled.Frame.ID, Revision: compiled.Frame.Revision, Digest: frameDigest}
	generation := contract.ContextGeneration{
		ContractVersion: contract.Version,
		ID:              compiled.Frame.GenerationID,
		Revision:        1,
		Ordinal:         compiled.Frame.Generation,
		RootFrame:       frameRef,
		CreatedUnixNano: now.Add(-time.Second).UnixNano(),
	}
	generationDigest, err := generation.DigestValue()
	if err != nil {
		return nil, err
	}
	generationRef := contract.FactRef{ID: generation.ID, Revision: generation.Revision, Digest: generationDigest}
	sessionRef := contract.FactRef{ID: "session-parent-current-1", Revision: 1, Digest: testkit.D("session-parent-current-1")}
	parentBindingDigest := testkit.D("parent-frame-generation-binding-1")
	subject := contract.ContextParentFrameApplicabilitySubjectV1{
		ContractVersion:                    contract.Version,
		FrameRef:                           frameRef,
		ManifestRef:                        compiled.Frame.ManifestRef,
		GenerationRef:                      generationRef,
		GenerationOrdinal:                  generation.Ordinal,
		ExecutionScopeDigest:               compiled.Frame.Execution.ScopeDigest,
		RunID:                              compiled.Frame.Execution.RunID,
		SessionRef:                         sessionRef,
		Turn:                               compiled.Frame.Execution.Turn,
		ParentFrameGenerationBindingDigest: parentBindingDigest,
		RecipeRef:                          compiled.Manifest.RecipeRef,
		AuthorityDigest:                    compiled.Frame.Execution.AuthorityDigest,
	}
	source, err := contract.SealContextParentFrameApplicabilitySourceCoordinateV1(subject)
	if err != nil {
		return nil, err
	}
	binding := contract.ContextParentFrameSourceBindingV1{
		Source:                   source,
		Subject:                  subject,
		BindingExpiresUnixNano:   now.Add(25 * time.Second).UnixNano(),
		RecipeExpiresUnixNano:    now.Add(20 * time.Second).UnixNano(),
		AuthorityExpiresUnixNano: now.Add(18 * time.Second).UnixNano(),
	}
	pointer, err := contract.SealContextGenerationCurrentPointerV1(contract.ContextGenerationCurrentPointerV1{
		ID:                                 "generation-current-parent-1",
		Revision:                           1,
		ExecutionScopeDigest:               subject.ExecutionScopeDigest,
		RunID:                              subject.RunID,
		SessionRef:                         subject.SessionRef,
		Turn:                               subject.Turn,
		GenerationRef:                      subject.GenerationRef,
		GenerationOrdinal:                  subject.GenerationOrdinal,
		ParentFrameGenerationBindingDigest: subject.ParentFrameGenerationBindingDigest,
		ExpiresUnixNano:                    now.Add(12 * time.Second).UnixNano(),
	})
	if err != nil {
		return nil, err
	}
	metadata := testkit.NewMetadataStoreV1()
	for _, put := range []func() error{
		func() error { return metadata.PutSourceBinding(binding) },
		func() error { return metadata.PutFrame(compiled.Frame) },
		func() error { return metadata.PutManifest(compiled.Manifest) },
		func() error { return metadata.PutGeneration(subject.ExecutionScopeDigest, generation) },
		func() error { return metadata.PutCurrentGenerationPointer(pointer) },
	} {
		if err := put(); err != nil {
			return nil, err
		}
	}
	reader, err := kernel.NewParentFrameCurrentReaderV1(metadata, metadata, metadata, metadata, metadata, content, clock, maxTTL)
	if err != nil {
		return nil, err
	}
	return &ParentFrameFixtureV1{
		Now: now, Content: content, Metadata: metadata, Reader: reader, Recipe: recipe,
		Manifest: compiled.Manifest, Frame: compiled.Frame, Generation: generation,
		Pointer: pointer, Binding: binding, Source: source,
	}, nil
}

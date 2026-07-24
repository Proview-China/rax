package kernel

import (
	"context"
	"fmt"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
)

const MaxParentFrameCurrentTTLV1 = 30 * time.Second

type ParentFrameCurrentReaderV1 struct {
	sourceBindings contextports.ContextParentFrameSourceBindingReaderV1
	frames         contextports.ContextFrameMetadataReaderV1
	manifests      contextports.ContextManifestMetadataReaderV1
	generations    contextports.ContextGenerationMetadataReaderV1
	pointers       contextports.ContextGenerationCurrentPointerReaderV1
	content        ReferenceStore
	clock          func() time.Time
	maxTTL         time.Duration
}

func NewParentFrameCurrentReaderV1(
	sourceBindings contextports.ContextParentFrameSourceBindingReaderV1,
	frames contextports.ContextFrameMetadataReaderV1,
	manifests contextports.ContextManifestMetadataReaderV1,
	generations contextports.ContextGenerationMetadataReaderV1,
	pointers contextports.ContextGenerationCurrentPointerReaderV1,
	content ReferenceStore,
	clock func() time.Time,
	maxTTL time.Duration,
) (*ParentFrameCurrentReaderV1, error) {
	if sourceBindings == nil || frames == nil || manifests == nil || generations == nil || pointers == nil || content == nil || clock == nil || maxTTL <= 0 || maxTTL > MaxParentFrameCurrentTTLV1 {
		return nil, fmt.Errorf("%w: parent frame current reader dependencies", contract.ErrInvalid)
	}
	return &ParentFrameCurrentReaderV1{
		sourceBindings: sourceBindings,
		frames:         frames,
		manifests:      manifests,
		generations:    generations,
		pointers:       pointers,
		content:        content,
		clock:          clock,
		maxTTL:         maxTTL,
	}, nil
}

var _ contextports.ContextParentFrameCurrentReaderV1 = (*ParentFrameCurrentReaderV1)(nil)

type parentFrameCurrentSnapshotV1 struct {
	Binding    contract.ContextParentFrameSourceBindingV1 `json:"binding"`
	Frame      contract.ContextFrame                      `json:"frame"`
	Manifest   contract.ContextManifest                   `json:"manifest"`
	Generation contract.ContextGeneration                 `json:"generation"`
	Pointer    contract.ContextGenerationCurrentPointerV1 `json:"pointer"`
}

func (r *ParentFrameCurrentReaderV1) InspectContextParentFrameCurrentV1(ctx context.Context, source contract.ContextParentFrameApplicabilitySourceCoordinateV1) (contract.ContextParentFrameCurrentProjectionV1, error) {
	if ctx == nil {
		return contract.ContextParentFrameCurrentProjectionV1{}, fmt.Errorf("%w: parent frame current inspect request", contract.ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return contract.ContextParentFrameCurrentProjectionV1{}, err
	}
	if r == nil || source.Validate() != nil {
		return contract.ContextParentFrameCurrentProjectionV1{}, fmt.Errorf("%w: parent frame current inspect request", contract.ErrInvalid)
	}
	checked := r.clock()
	if checked.IsZero() {
		return contract.ContextParentFrameCurrentProjectionV1{}, fmt.Errorf("%w: parent frame current clock", contract.ErrInvalid)
	}
	notAfter := checked.Add(r.maxTTL)

	s1, err := r.readSnapshot(ctx, source, checked.UnixNano())
	if err != nil {
		return contract.ContextParentFrameCurrentProjectionV1{}, err
	}
	if err := ctx.Err(); err != nil {
		return contract.ContextParentFrameCurrentProjectionV1{}, err
	}
	request, err := contract.SealContextParentFrameCurrentRequestV1(contract.ContextParentFrameCurrentRequestV1{
		Source:           source,
		Subject:          s1.Binding.Subject,
		CheckedUnixNano:  checked.UnixNano(),
		NotAfterUnixNano: notAfter.UnixNano(),
	})
	if err != nil {
		return contract.ContextParentFrameCurrentProjectionV1{}, err
	}
	expires := snapshotExpiryV1(s1, request.NotAfterUnixNano)
	if checked.UnixNano() >= expires {
		return contract.ContextParentFrameCurrentProjectionV1{}, fmt.Errorf("%w: parent frame current S1 TTL", contract.ErrExpired)
	}

	s2, err := r.readSnapshot(ctx, source, checked.UnixNano())
	if err != nil {
		return contract.ContextParentFrameCurrentProjectionV1{}, err
	}
	if err := ctx.Err(); err != nil {
		return contract.ContextParentFrameCurrentProjectionV1{}, err
	}
	s1Digest, err := contract.DigestJSON(s1)
	if err != nil {
		return contract.ContextParentFrameCurrentProjectionV1{}, err
	}
	s2Digest, err := contract.DigestJSON(s2)
	if err != nil {
		return contract.ContextParentFrameCurrentProjectionV1{}, err
	}
	if s1Digest != s2Digest {
		return contract.ContextParentFrameCurrentProjectionV1{}, fmt.Errorf("%w: parent frame current S1/S2 drift", contract.ErrConflict)
	}
	completed := r.clock()
	if completed.IsZero() || completed.Before(checked) {
		return contract.ContextParentFrameCurrentProjectionV1{}, fmt.Errorf("%w: parent frame current clock regression", contract.ErrConflict)
	}
	if err := ensureSnapshotLiveV1(s2, completed.UnixNano()); err != nil {
		return contract.ContextParentFrameCurrentProjectionV1{}, err
	}
	if completed.UnixNano() >= expires {
		return contract.ContextParentFrameCurrentProjectionV1{}, fmt.Errorf("%w: parent frame current TTL crossing", contract.ErrExpired)
	}
	if err := ctx.Err(); err != nil {
		return contract.ContextParentFrameCurrentProjectionV1{}, err
	}

	subject := s2.Binding.Subject
	return contract.SealContextParentFrameCurrentProjectionV1(contract.ContextParentFrameCurrentProjectionV1{
		Source:               source,
		FrameRef:             subject.FrameRef,
		ManifestRef:          subject.ManifestRef,
		GenerationRef:        subject.GenerationRef,
		GenerationOrdinal:    subject.GenerationOrdinal,
		ExecutionScopeDigest: s2.Frame.Execution.ScopeDigest,
		Current:              true,
		CheckedUnixNano:      request.CheckedUnixNano,
		ExpiresUnixNano:      expires,
	}, completed.UnixNano())
}

func (r *ParentFrameCurrentReaderV1) readSnapshot(ctx context.Context, source contract.ContextParentFrameApplicabilitySourceCoordinateV1, observedUnixNano int64) (parentFrameCurrentSnapshotV1, error) {
	if err := ctx.Err(); err != nil {
		return parentFrameCurrentSnapshotV1{}, err
	}
	binding, err := r.sourceBindings.ResolveExactSourceBinding(ctx, source)
	if err != nil {
		return parentFrameCurrentSnapshotV1{}, fmt.Errorf("parent frame source binding: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return parentFrameCurrentSnapshotV1{}, err
	}
	if err := binding.Validate(); err != nil || binding.Source != source {
		return parentFrameCurrentSnapshotV1{}, fmt.Errorf("%w: parent frame source binding mismatch", contract.ErrConflict)
	}
	subject := binding.Subject
	frame, err := r.frames.FrameByExactRef(ctx, subject.FrameRef, subject.ExecutionScopeDigest)
	if err != nil {
		return parentFrameCurrentSnapshotV1{}, fmt.Errorf("parent frame exact read: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return parentFrameCurrentSnapshotV1{}, err
	}
	manifest, err := r.manifests.ManifestByExactRef(ctx, subject.ManifestRef, subject.ExecutionScopeDigest)
	if err != nil {
		return parentFrameCurrentSnapshotV1{}, fmt.Errorf("parent frame manifest exact read: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return parentFrameCurrentSnapshotV1{}, err
	}
	generation, err := r.generations.GenerationByExactRef(ctx, subject.GenerationRef, subject.ExecutionScopeDigest)
	if err != nil {
		return parentFrameCurrentSnapshotV1{}, fmt.Errorf("parent frame generation exact read: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return parentFrameCurrentSnapshotV1{}, err
	}
	pointer, err := r.pointers.InspectCurrentGenerationPointer(ctx, contract.ContextGenerationCurrentPointerRequestV1{
		ExecutionScopeDigest: subject.ExecutionScopeDigest,
		RunID:                subject.RunID,
		SessionRef:           subject.SessionRef,
		Turn:                 subject.Turn,
	})
	if err != nil {
		return parentFrameCurrentSnapshotV1{}, fmt.Errorf("parent frame generation current pointer: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return parentFrameCurrentSnapshotV1{}, err
	}
	snapshot := parentFrameCurrentSnapshotV1{Binding: binding, Frame: frame, Manifest: manifest, Generation: generation, Pointer: pointer}
	if err := r.validateSnapshot(snapshot, observedUnixNano); err != nil {
		return parentFrameCurrentSnapshotV1{}, err
	}
	return snapshot, nil
}

func (r *ParentFrameCurrentReaderV1) validateSnapshot(snapshot parentFrameCurrentSnapshotV1, observedUnixNano int64) error {
	binding := snapshot.Binding
	subject := binding.Subject
	frame := snapshot.Frame
	manifest := snapshot.Manifest
	generation := snapshot.Generation
	pointer := snapshot.Pointer

	frameDigest, err := frame.DigestValue()
	if err != nil || (contract.FactRef{ID: frame.ID, Revision: frame.Revision, Digest: frameDigest}) != subject.FrameRef {
		return fmt.Errorf("%w: exact parent frame reference", contract.ErrConflict)
	}
	manifestDigest, err := manifest.DigestValue()
	if err != nil || (contract.FactRef{ID: manifest.ID, Revision: manifest.Revision, Digest: manifestDigest}) != subject.ManifestRef {
		return fmt.Errorf("%w: exact parent manifest reference", contract.ErrConflict)
	}
	generationDigest, err := generation.DigestValue()
	if err != nil || (contract.FactRef{ID: generation.ID, Revision: generation.Revision, Digest: generationDigest}) != subject.GenerationRef {
		return fmt.Errorf("%w: exact context generation reference", contract.ErrConflict)
	}

	// ContextFrame carries the Owner-read ExecutionScope as its exact digest.
	// Re-read and validate that digest against the sealed source and all outputs;
	// do not trust the query partition or source ID alone.
	if frame.Execution.ScopeDigest != subject.ExecutionScopeDigest || manifest.Execution.ScopeDigest != subject.ExecutionScopeDigest || frame.Execution.RunID != subject.RunID || manifest.Execution.RunID != subject.RunID || frame.Execution.Turn != subject.Turn || manifest.Execution.Turn != subject.Turn || frame.Execution.AuthorityDigest != subject.AuthorityDigest || manifest.Execution.AuthorityDigest != subject.AuthorityDigest {
		return fmt.Errorf("%w: parent frame execution scope", contract.ErrConflict)
	}
	if frame.ManifestRef != subject.ManifestRef || frame.GenerationID != subject.GenerationRef.ID || frame.Generation != subject.GenerationOrdinal || !sameFactRef(frame.ParentFrame, subject.ParentFrameRef) || manifest.GenerationID != subject.GenerationRef.ID || manifest.RecipeRef != subject.RecipeRef || !sameFactRef(manifest.ParentFrame, subject.ParentFrameRef) {
		return fmt.Errorf("%w: parent frame subject binding", contract.ErrConflict)
	}
	if generation.Ordinal != subject.GenerationOrdinal || generation.RootFrame != subject.FrameRef || !sameFactRef(generation.Parent, subject.ParentGenerationRef) {
		return fmt.Errorf("%w: context generation subject binding", contract.ErrConflict)
	}
	if pointer.Validate() != nil || pointer.ExecutionScopeDigest != subject.ExecutionScopeDigest || pointer.RunID != subject.RunID || pointer.SessionRef != subject.SessionRef || pointer.Turn != subject.Turn || pointer.GenerationRef != subject.GenerationRef || pointer.GenerationOrdinal != subject.GenerationOrdinal || pointer.ParentFrameGenerationBindingDigest != subject.ParentFrameGenerationBindingDigest {
		return fmt.Errorf("%w: generation current pointer drift", contract.ErrConflict)
	}
	if err := InspectFrame(r.content, manifest, frame); err != nil {
		return fmt.Errorf("parent frame content inspection: %w", err)
	}
	return ensureSnapshotLiveV1(snapshot, observedUnixNano)
}

func ensureSnapshotLiveV1(snapshot parentFrameCurrentSnapshotV1, nowUnixNano int64) error {
	if nowUnixNano <= 0 {
		return fmt.Errorf("%w: parent frame observed time", contract.ErrInvalid)
	}
	for _, expires := range []int64{
		snapshot.Binding.BindingExpiresUnixNano,
		snapshot.Binding.RecipeExpiresUnixNano,
		snapshot.Binding.AuthorityExpiresUnixNano,
		snapshot.Frame.ExpiresUnixNano,
		snapshot.Manifest.ExpiresUnixNano,
		snapshot.Pointer.ExpiresUnixNano,
	} {
		if nowUnixNano >= expires {
			return fmt.Errorf("%w: parent frame owner TTL", contract.ErrExpired)
		}
	}
	return nil
}

func snapshotExpiryV1(snapshot parentFrameCurrentSnapshotV1, requestNotAfter int64) int64 {
	expires := requestNotAfter
	for _, candidate := range []int64{
		snapshot.Binding.BindingExpiresUnixNano,
		snapshot.Binding.RecipeExpiresUnixNano,
		snapshot.Binding.AuthorityExpiresUnixNano,
		snapshot.Frame.ExpiresUnixNano,
		snapshot.Manifest.ExpiresUnixNano,
		snapshot.Pointer.ExpiresUnixNano,
	} {
		if candidate < expires {
			expires = candidate
		}
	}
	return expires
}

package kernel

import (
	"bytes"
	"context"
	"fmt"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

func InspectFrame(store ReferenceStore, manifest contract.ContextManifest, frame contract.ContextFrame) error {
	if store == nil {
		return fmt.Errorf("%w: frame inspection", contract.ErrInvalid)
	}
	return InspectFrameStagedV1(context.Background(), legacyReferenceStoreAdapterV1{store: store}, manifest, frame, legacyInspectWorkLimitsV1())
}

func InspectFrameStagedV1(ctx context.Context, store ContextAwareReferenceStoreV1, manifest contract.ContextManifest, frame contract.ContextFrame, limits InspectWorkLimitsV1) error {
	if err := checkContextV1(ctx); err != nil {
		return err
	}
	if limits.Validate() != nil {
		return fmt.Errorf("%w: frame inspection limits", contract.ErrInvalid)
	}
	if len(manifest.Fragments) > int(limits.MaxFragments) {
		return fmt.Errorf("%w: frame inspection limits", contract.ErrLimitExceeded)
	}
	if store == nil || manifest.Validate() != nil || frame.Validate() != nil {
		return fmt.Errorf("%w: frame inspection", contract.ErrInvalid)
	}
	manifestDigest, err := manifest.DigestValue()
	if err != nil {
		return err
	}
	if frame.ManifestRef.ID != manifest.ID || frame.ManifestRef.Revision != manifest.Revision || frame.ManifestRef.Digest != manifestDigest {
		return fmt.Errorf("%w: frame manifest reference", contract.ErrConflict)
	}
	if frame.Execution != manifest.Execution || frame.GenerationID != manifest.GenerationID || !sameFactRef(frame.ParentFrame, manifest.ParentFrame) || frame.SourceSetDigest != manifest.SourceSetDigest || frame.CreatedUnixNano != manifest.CreatedUnixNano || frame.ExpiresUnixNano != manifest.ExpiresUnixNano {
		return fmt.Errorf("%w: frame manifest binding", contract.ErrConflict)
	}
	regions := map[contract.FrameRegion][]renderedFragment{
		contract.RegionStablePrefix: {},
		contract.RegionSemiStable:   {},
		contract.RegionDynamicTail:  {},
	}
	seenContent := make(map[contract.ContentRef]struct{}, len(manifest.Fragments)+4)
	var inspectedItems uint32
	var inspectedBytes uint64
	consume := func(ref contract.ContentRef) error {
		if _, ok := seenContent[ref]; ok {
			return nil
		}
		seenContent[ref] = struct{}{}
		if inspectedItems == limits.MaxContentItems || ref.Length > limits.MaxRawBytes-inspectedBytes {
			return fmt.Errorf("%w: frame inspection aggregate", contract.ErrLimitExceeded)
		}
		inspectedItems++
		inspectedBytes += ref.Length
		return nil
	}
	for _, fragment := range manifest.Fragments {
		if err := checkContextV1(ctx); err != nil {
			return err
		}
		if err := consume(fragment.Content); err != nil {
			return err
		}
		content, readErr := readExactContextV1(ctx, store, fragment.Content, limits.MaxContentItemBytes)
		if readErr != nil {
			return readErr
		}
		regions[fragment.Region] = append(regions[fragment.Region], renderedFragment{Position: fragment.Position, Kind: fragment.Kind, CandidateDigest: fragment.CandidateRef.Digest, Content: content})
	}
	wantStable, wantSemi, wantDynamic, wantRendered, err := renderRegionsContextV1(ctx, regions, limits.MaxRawBytes)
	if err != nil {
		return err
	}
	if err := consume(frame.StablePrefix); err != nil {
		return err
	}
	gotStable, err := readExactContextV1(ctx, store, frame.StablePrefix, limits.MaxContentItemBytes)
	if err != nil || !bytes.Equal(gotStable, wantStable) {
		return fmt.Errorf("%w: stable prefix reference", contract.ErrConflict)
	}
	if len(regions[contract.RegionSemiStable]) == 0 {
		if frame.SemiStable != nil {
			return fmt.Errorf("%w: unexpected semi-stable reference", contract.ErrConflict)
		}
	} else {
		if frame.SemiStable == nil {
			return fmt.Errorf("%w: missing semi-stable reference", contract.ErrConflict)
		}
		if err := consume(*frame.SemiStable); err != nil {
			return err
		}
		gotSemi, readErr := readExactContextV1(ctx, store, *frame.SemiStable, limits.MaxContentItemBytes)
		if readErr != nil || !bytes.Equal(gotSemi, wantSemi) {
			return fmt.Errorf("%w: semi-stable reference", contract.ErrConflict)
		}
	}
	if err := consume(frame.DynamicTail); err != nil {
		return err
	}
	gotDynamic, err := readExactContextV1(ctx, store, frame.DynamicTail, limits.MaxContentItemBytes)
	if err != nil || !bytes.Equal(gotDynamic, wantDynamic) {
		return fmt.Errorf("%w: dynamic tail reference", contract.ErrConflict)
	}
	if err := consume(frame.Rendered); err != nil {
		return err
	}
	gotRendered, err := readExactContextV1(ctx, store, frame.Rendered, limits.MaxContentItemBytes)
	if err != nil || !bytes.Equal(gotRendered, wantRendered) {
		return fmt.Errorf("%w: rendered frame reference", contract.ErrConflict)
	}
	return checkContextV1(ctx)
}

func readExactContextV1(ctx context.Context, store ContextAwareReferenceStoreV1, ref contract.ContentRef, maxBytes uint64) ([]byte, error) {
	if ref.Length > maxBytes {
		return nil, fmt.Errorf("%w: content item limit", contract.ErrLimitExceeded)
	}
	value, err := store.GetContextV1(ctx, ref)
	if err != nil {
		return nil, err
	}
	digest, digestErr := digestBytesContextV1(ctx, value)
	if digestErr != nil {
		return nil, digestErr
	}
	if uint64(len(value)) != ref.Length || digest != ref.Digest {
		return nil, fmt.Errorf("%w: content reference", contract.ErrConflict)
	}
	return value, nil
}

func readExact(store ReferenceStore, ref contract.ContentRef) ([]byte, error) {
	value, err := store.Get(ref)
	if err != nil {
		return nil, err
	}
	if uint64(len(value)) != ref.Length || contract.DigestBytes(value) != ref.Digest {
		return nil, fmt.Errorf("%w: content reference", contract.ErrConflict)
	}
	return value, nil
}

func sameFactRef(left, right *contract.FactRef) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

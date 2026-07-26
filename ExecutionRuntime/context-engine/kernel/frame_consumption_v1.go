package kernel

import (
	"context"
	"fmt"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

type FrameConsumptionCurrentSnapshotV1 struct {
	Manifest                  contract.ContextManifest
	Frame                     contract.ContextFrame
	Generation                contract.ContextGeneration
	GenerationExpiresUnixNano int64
	FragmentSourceExpires     []int64
	PromptExpiresUnixNano     int64
	RecipeExpiresUnixNano     int64
	DisclosureExpiresUnixNano int64
	AuthorityExpiresUnixNano  int64
}

type FrameConsumptionCurrentReaderV1 interface {
	InspectFrameConsumptionCurrentV1(context.Context, contract.ContextFrameConsumptionRequestV1) (FrameConsumptionCurrentSnapshotV1, error)
}

func BuildFrameConsumptionDescriptorV1(
	ctx context.Context,
	reader FrameConsumptionCurrentReaderV1,
	store ContextAwareReferenceStoreV1,
	request contract.ContextFrameConsumptionRequestV1,
) (contract.ContextFrameConsumptionDescriptorV1, error) {
	if err := checkContextV1(ctx); err != nil {
		return contract.ContextFrameConsumptionDescriptorV1{}, err
	}
	if reader == nil || store == nil || request.Validate() != nil {
		return contract.ContextFrameConsumptionDescriptorV1{}, fmt.Errorf("%w: frame consumption dependencies", contract.ErrInvalid)
	}
	s1, err := reader.InspectFrameConsumptionCurrentV1(ctx, request)
	if err != nil {
		return contract.ContextFrameConsumptionDescriptorV1{}, err
	}
	if err = validateFrameConsumptionSnapshotV1(ctx, store, request, s1); err != nil {
		return contract.ContextFrameConsumptionDescriptorV1{}, err
	}
	expires := frameConsumptionExpiresV1(request.NotAfterUnixNano, s1)
	if request.CheckedUnixNano >= expires {
		return contract.ContextFrameConsumptionDescriptorV1{}, fmt.Errorf("%w: frame consumption current window", contract.ErrExpired)
	}
	hint, err := buildContextCacheHintV1(request, s1, expires)
	if err != nil {
		return contract.ContextFrameConsumptionDescriptorV1{}, err
	}
	fragmentRefs := make([]contract.FactRef, len(s1.Manifest.Fragments))
	for index, fragment := range s1.Manifest.Fragments {
		fragmentRefs[index] = fragment.CandidateRef
	}
	descriptor, err := contract.SealContextFrameConsumptionDescriptorV1(contract.ContextFrameConsumptionDescriptorV1{
		ID:                request.DescriptorID,
		FrameRef:          request.FrameRef,
		ManifestRef:       request.ManifestRef,
		GenerationRef:     request.GenerationRef,
		StablePrefix:      s1.Frame.StablePrefix,
		SemiStable:        cloneContentRefPointerV1(s1.Frame.SemiStable),
		DynamicTail:       s1.Frame.DynamicTail,
		Rendered:          s1.Frame.Rendered,
		FragmentRefs:      fragmentRefs,
		TenantScopeDigest: request.TenantScopeDigest,
		AgentInstanceRef:  request.AgentInstanceRef,
		RunID:             request.RunID,
		RunScopeDigest:    request.RunScopeDigest,
		PromptAssetRefs:   append([]contract.PromptAssetRefV1{}, request.PromptAssetRefs...),
		RecipeRef:         request.RecipeRef,
		DisclosureClass:   request.DisclosureClass,
		CacheHint:         hint,
		CheckedUnixNano:   request.CheckedUnixNano,
		ExpiresUnixNano:   expires,
	})
	if err != nil {
		return contract.ContextFrameConsumptionDescriptorV1{}, err
	}
	if err = checkContextV1(ctx); err != nil {
		return contract.ContextFrameConsumptionDescriptorV1{}, err
	}
	s2, err := reader.InspectFrameConsumptionCurrentV1(ctx, request)
	if err != nil {
		return contract.ContextFrameConsumptionDescriptorV1{}, err
	}
	if !reflect.DeepEqual(s1, s2) {
		return contract.ContextFrameConsumptionDescriptorV1{}, fmt.Errorf("%w: frame consumption S2 drift", contract.ErrConflict)
	}
	if err = validateFrameConsumptionSnapshotV1(ctx, store, request, s2); err != nil {
		return contract.ContextFrameConsumptionDescriptorV1{}, err
	}
	if expires != frameConsumptionExpiresV1(request.NotAfterUnixNano, s2) || request.CheckedUnixNano >= expires {
		return contract.ContextFrameConsumptionDescriptorV1{}, fmt.Errorf("%w: frame consumption TTL crossing", contract.ErrExpired)
	}
	return descriptor, checkContextV1(ctx)
}

func validateFrameConsumptionSnapshotV1(ctx context.Context, store ContextAwareReferenceStoreV1, request contract.ContextFrameConsumptionRequestV1, snapshot FrameConsumptionCurrentSnapshotV1) error {
	if snapshot.Manifest.Validate() != nil || snapshot.Frame.Validate() != nil || snapshot.Generation.Validate() != nil || len(snapshot.FragmentSourceExpires) != len(snapshot.Manifest.Fragments) {
		return fmt.Errorf("%w: frame consumption current snapshot", contract.ErrInvalid)
	}
	manifestDigest, err := snapshot.Manifest.DigestValue()
	if err != nil {
		return err
	}
	frameDigest, err := snapshot.Frame.DigestValue()
	if err != nil {
		return err
	}
	generationDigest, err := contract.DigestJSON(snapshot.Generation)
	if err != nil {
		return err
	}
	if request.ManifestRef != (contract.FactRef{ID: snapshot.Manifest.ID, Revision: snapshot.Manifest.Revision, Digest: manifestDigest}) ||
		request.FrameRef != (contract.FactRef{ID: snapshot.Frame.ID, Revision: snapshot.Frame.Revision, Digest: frameDigest}) ||
		request.GenerationRef != (contract.FactRef{ID: snapshot.Generation.ID, Revision: snapshot.Generation.Revision, Digest: generationDigest}) {
		return fmt.Errorf("%w: frame consumption exact references", contract.ErrConflict)
	}
	if snapshot.Generation.RootFrame != request.FrameRef || snapshot.Frame.ManifestRef != request.ManifestRef || snapshot.Frame.GenerationID != snapshot.Generation.ID || snapshot.Frame.Execution.RunID != request.RunID || snapshot.Frame.Execution.ScopeDigest != request.RunScopeDigest {
		return fmt.Errorf("%w: frame consumption binding", contract.ErrConflict)
	}
	for _, expiry := range append([]int64{
		snapshot.GenerationExpiresUnixNano,
		snapshot.PromptExpiresUnixNano,
		snapshot.RecipeExpiresUnixNano,
		snapshot.DisclosureExpiresUnixNano,
		snapshot.AuthorityExpiresUnixNano,
	}, snapshot.FragmentSourceExpires...) {
		if expiry <= request.CheckedUnixNano {
			return fmt.Errorf("%w: frame consumption dependency", contract.ErrExpired)
		}
	}
	return InspectFrameStagedV1(ctx, store, snapshot.Manifest, snapshot.Frame, legacyInspectWorkLimitsV1())
}

func frameConsumptionExpiresV1(notAfter int64, snapshot FrameConsumptionCurrentSnapshotV1) int64 {
	expires := notAfter
	values := append([]int64{
		snapshot.Frame.ExpiresUnixNano,
		snapshot.Manifest.ExpiresUnixNano,
		snapshot.GenerationExpiresUnixNano,
		snapshot.PromptExpiresUnixNano,
		snapshot.RecipeExpiresUnixNano,
		snapshot.DisclosureExpiresUnixNano,
		snapshot.AuthorityExpiresUnixNano,
	}, snapshot.FragmentSourceExpires...)
	for _, value := range values {
		if value < expires {
			expires = value
		}
	}
	return expires
}

func buildContextCacheHintV1(request contract.ContextFrameConsumptionRequestV1, snapshot FrameConsumptionCurrentSnapshotV1, expires int64) (contract.ContextCacheHintV1, error) {
	allRefs := make([]contract.FactRef, 0, len(snapshot.Manifest.Fragments))
	stableRefs := make([]contract.FactRef, 0, len(snapshot.Manifest.Fragments))
	semiRefs := make([]contract.FactRef, 0, len(snapshot.Manifest.Fragments))
	for _, fragment := range snapshot.Manifest.Fragments {
		allRefs = append(allRefs, fragment.CandidateRef)
		switch fragment.Region {
		case contract.RegionStablePrefix:
			stableRefs = append(stableRefs, fragment.CandidateRef)
		case contract.RegionSemiStable:
			semiRefs = append(semiRefs, fragment.CandidateRef)
		}
	}
	common := struct {
		TenantScopeDigest contract.Digest             `json:"tenant_scope_digest"`
		AgentInstanceRef  contract.FactRef            `json:"agent_instance_ref"`
		RunID             string                      `json:"run_id"`
		RunScopeDigest    contract.Digest             `json:"run_scope_digest"`
		PromptAssetRefs   []contract.PromptAssetRefV1 `json:"prompt_asset_refs"`
		RecipeRef         contract.FactRef            `json:"recipe_ref"`
		DisclosureClass   contract.DisclosureClassV1  `json:"disclosure_class"`
		KeyVersion        string                      `json:"key_version"`
	}{
		request.TenantScopeDigest,
		request.AgentInstanceRef,
		request.RunID,
		request.RunScopeDigest,
		request.PromptAssetRefs,
		request.RecipeRef,
		request.DisclosureClass,
		contract.FrameConsumptionKeyV1,
	}
	frameFingerprint, err := contract.DigestJSON(struct {
		Domain        string
		Common        any
		FrameRef      contract.FactRef
		ManifestRef   contract.FactRef
		GenerationRef contract.FactRef
		Stable        contract.ContentRef
		Semi          *contract.ContentRef
		Dynamic       contract.ContentRef
		Rendered      contract.ContentRef
		FragmentRefs  []contract.FactRef
	}{"praxis.context/frame-fingerprint-v1", common, request.FrameRef, request.ManifestRef, request.GenerationRef, snapshot.Frame.StablePrefix, snapshot.Frame.SemiStable, snapshot.Frame.DynamicTail, snapshot.Frame.Rendered, allRefs})
	if err != nil {
		return contract.ContextCacheHintV1{}, err
	}
	stableFingerprint, err := contract.DigestJSON(struct {
		Domain       string
		Common       any
		Content      contract.ContentRef
		FragmentRefs []contract.FactRef
	}{"praxis.context/stable-prefix-fingerprint-v1", common, snapshot.Frame.StablePrefix, stableRefs})
	if err != nil {
		return contract.ContextCacheHintV1{}, err
	}
	var semiFingerprint *contract.Digest
	if snapshot.Frame.SemiStable != nil {
		value, digestErr := contract.DigestJSON(struct {
			Domain       string
			Common       any
			Content      contract.ContentRef
			FragmentRefs []contract.FactRef
		}{"praxis.context/semi-stable-prefix-fingerprint-v1", common, *snapshot.Frame.SemiStable, semiRefs})
		if digestErr != nil {
			return contract.ContextCacheHintV1{}, digestErr
		}
		semiFingerprint = &value
	}
	hint := contract.ContextCacheHintV1{
		StableEligible:              len(stableRefs) > 0,
		SemiStableEligible:          snapshot.Frame.SemiStable != nil,
		FrameFingerprint:            frameFingerprint,
		StablePrefixFingerprint:     stableFingerprint,
		SemiStablePrefixFingerprint: semiFingerprint,
		InvalidationReasons:         []contract.ContextCacheInvalidationReasonV1{},
		ExpiresUnixNano:             expires,
	}
	return hint, hint.Validate()
}

func cloneContentRefPointerV1(value *contract.ContentRef) *contract.ContentRef {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

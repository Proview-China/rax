package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type SettledToolResultCurrentReaderV2 interface {
	InspectSettledToolResultCurrentV2(context.Context, toolcontract.ObjectRef) (toolcontract.ToolResultV2, error)
}

type AppendSettledToolResultRequestV2 struct {
	Projection                toolcontract.SettledToolResultProjectionV1
	ParentManifest            contract.ContextManifest
	ParentFrame               contract.ContextFrame
	ParentGeneration          contract.ContextGeneration
	ParentGenerationExpires   int64
	Recipe                    contract.ContextRecipe
	TenantScopeDigest         contract.Digest
	AgentInstanceRef          contract.FactRef
	PromptAssetRefs           []contract.PromptAssetRefV1
	DisclosureClass           contract.DisclosureClassV1
	PromptExpiresUnixNano     int64
	DisclosureExpiresUnixNano int64
	AuthorityExpiresUnixNano  int64
	IdempotencyKey            string
	CheckedUnixNano           int64
	NotAfterUnixNano          int64
}

type AppendSettledToolResultResultV2 struct {
	Source     contract.SettledToolResultSourceV2
	Manifest   contract.ContextManifest
	Frame      contract.ContextFrame
	Generation contract.ContextGeneration
	Descriptor contract.ContextFrameConsumptionDescriptorV1
}

func AppendSettledToolResultV2(ctx context.Context, reader SettledToolResultCurrentReaderV2, store ContextAwareReferenceStoreV1, request AppendSettledToolResultRequestV2) (AppendSettledToolResultResultV2, error) {
	if err := checkContextV1(ctx); err != nil {
		return AppendSettledToolResultResultV2{}, err
	}
	if reader == nil || store == nil || request.CheckedUnixNano <= 0 || request.NotAfterUnixNano <= request.CheckedUnixNano || request.IdempotencyKey == "" {
		return AppendSettledToolResultResultV2{}, fmt.Errorf("%w: append settled Tool result request", contract.ErrInvalid)
	}
	now := time.Unix(0, request.CheckedUnixNano)
	exactResult, err := reader.InspectSettledToolResultCurrentV2(ctx, request.Projection.Result)
	if err != nil {
		return AppendSettledToolResultResultV2{}, err
	}
	if exactResult.Validate() != nil || request.Projection.Result != (toolcontract.ObjectRef{ID: exactResult.ID, Revision: exactResult.Revision, Digest: exactResult.Digest}) {
		return AppendSettledToolResultResultV2{}, fmt.Errorf("%w: exact Tool result reader drift", contract.ErrConflict)
	}
	source, err := contract.SealSettledToolResultSourceV2(request.Projection, exactResult, now, request.NotAfterUnixNano)
	if err != nil {
		return AppendSettledToolResultResultV2{}, err
	}
	if err = validateAppendParentV2(ctx, store, request); err != nil {
		return AppendSettledToolResultResultV2{}, err
	}
	expires := minimumExpiryV2(source.ExpiresUnixNano, request.NotAfterUnixNano, request.ParentManifest.ExpiresUnixNano, request.ParentFrame.ExpiresUnixNano, request.ParentGenerationExpires, request.Recipe.ExpiresUnixNano, request.PromptExpiresUnixNano, request.DisclosureExpiresUnixNano, request.AuthorityExpiresUnixNano)
	if expires <= request.CheckedUnixNano {
		return AppendSettledToolResultResultV2{}, fmt.Errorf("%w: append settled Tool result lifetime", contract.ErrExpired)
	}

	payload, kind, err := toolResultPayloadV2(source)
	if err != nil {
		return AppendSettledToolResultResultV2{}, err
	}
	content, err := store.PutContextV1(ctx, payload)
	if err != nil {
		return AppendSettledToolResultResultV2{}, err
	}
	if err = checkContextV1(ctx); err != nil {
		return AppendSettledToolResultResultV2{}, err
	}
	tokenEstimate := uint64((len(payload) + 3) / 4)
	if tokenEstimate == 0 {
		tokenEstimate = 1
	}
	if request.ParentManifest.DynamicTokens+tokenEstimate > request.Recipe.Budget.DynamicTailMax || request.ParentManifest.TotalTokens+tokenEstimate > request.Recipe.Budget.TotalTokens {
		return AppendSettledToolResultResultV2{}, fmt.Errorf("%w: settled Tool result budget", contract.ErrConflict)
	}
	candidateRef := contract.FactRef{ID: deterministicIDV2("ctx-tool-result-", source.Digest), Revision: uint64(exactResult.Revision), Digest: source.Digest}
	for _, decision := range request.ParentManifest.Decisions {
		if decision.CandidateRef.ID == candidateRef.ID {
			return AppendSettledToolResultResultV2{}, fmt.Errorf("%w: settled Tool result duplicate", contract.ErrConflict)
		}
	}
	fragments := append([]contract.ContextFragment{}, request.ParentManifest.Fragments...)
	fragments = append(fragments, contract.ContextFragment{CandidateRef: candidateRef, Kind: kind, Region: contract.RegionDynamicTail, Position: uint32(len(fragments) + 1), Content: content, Tokens: tokenEstimate})
	decisions := append([]contract.AdmissionDecision{}, request.ParentManifest.Decisions...)
	decisions = append(decisions, contract.AdmissionDecision{CandidateRef: candidateRef, Disposition: contract.AdmissionAdmitted, Reason: "settled_tool_result_current", Region: contract.RegionDynamicTail, Tokens: tokenEstimate})
	refs := make([]contract.FactRef, len(decisions))
	for i := range decisions {
		refs[i] = decisions[i].CandidateRef
	}
	sourceSet, err := contract.DigestJSON(refs)
	if err != nil {
		return AppendSettledToolResultResultV2{}, err
	}
	seed, err := contract.DigestJSON(struct {
		Parent contract.FactRef
		Source contract.Digest
		Key    string
	}{mustFrameRefV2(request.ParentFrame), source.Digest, request.IdempotencyKey})
	if err != nil {
		return AppendSettledToolResultResultV2{}, err
	}
	generationID := deterministicIDV2("ctx-tool-generation-", seed)
	manifestID := deterministicIDV2("ctx-tool-manifest-", seed)
	frameID := deterministicIDV2("ctx-tool-frame-", seed)

	regions := map[contract.FrameRegion][]renderedFragment{contract.RegionStablePrefix: {}, contract.RegionSemiStable: {}, contract.RegionDynamicTail: {}}
	for _, fragment := range fragments {
		value, readErr := readExactContextV1(ctx, store, fragment.Content, legacyInspectWorkLimitsV1().MaxContentItemBytes)
		if readErr != nil {
			return AppendSettledToolResultResultV2{}, readErr
		}
		regions[fragment.Region] = append(regions[fragment.Region], renderedFragment{Position: fragment.Position, Kind: fragment.Kind, CandidateDigest: fragment.CandidateRef.Digest, Content: value})
	}
	stableBytes, semiBytes, dynamicBytes, renderedBytes, err := renderRegionsContextV1(ctx, regions, legacyCompileWorkLimitsV1().MaxGeneratedRawBytes)
	if err != nil {
		return AppendSettledToolResultResultV2{}, err
	}
	if contract.DigestBytes(stableBytes) != request.ParentFrame.StablePrefix.Digest || uint64(len(stableBytes)) != request.ParentFrame.StablePrefix.Length {
		return AppendSettledToolResultResultV2{}, fmt.Errorf("%w: stable Prefix changed during Tool append", contract.ErrConflict)
	}
	if request.ParentFrame.SemiStable == nil {
		if len(regions[contract.RegionSemiStable]) != 0 {
			return AppendSettledToolResultResultV2{}, fmt.Errorf("%w: semi-stable parent drift", contract.ErrConflict)
		}
	} else if contract.DigestBytes(semiBytes) != request.ParentFrame.SemiStable.Digest || uint64(len(semiBytes)) != request.ParentFrame.SemiStable.Length {
		return AppendSettledToolResultResultV2{}, fmt.Errorf("%w: semi-stable changed during Tool append", contract.ErrConflict)
	}
	dynamicRef, err := store.PutContextV1(ctx, dynamicBytes)
	if err != nil {
		return AppendSettledToolResultResultV2{}, err
	}
	renderedRef, err := store.PutContextV1(ctx, renderedBytes)
	if err != nil {
		return AppendSettledToolResultResultV2{}, err
	}
	parentRef := mustFrameRefV2(request.ParentFrame)
	recipeDigest, err := request.Recipe.DigestValue()
	if err != nil {
		return AppendSettledToolResultResultV2{}, err
	}
	manifest := contract.ContextManifest{ContractVersion: contract.Version, ID: manifestID, Revision: 1, Execution: request.ParentFrame.Execution, RecipeRef: contract.FactRef{ID: request.Recipe.ID, Revision: request.Recipe.Revision, Digest: recipeDigest}, GenerationID: generationID, ParentFrame: &parentRef, Decisions: decisions, Fragments: fragments, StableTokens: request.ParentManifest.StableTokens, SemiStableTokens: request.ParentManifest.SemiStableTokens, DynamicTokens: request.ParentManifest.DynamicTokens + tokenEstimate, TotalTokens: request.ParentManifest.TotalTokens + tokenEstimate, SourceSetDigest: sourceSet, CreatedUnixNano: request.CheckedUnixNano, ExpiresUnixNano: expires}
	manifestDigest, err := manifest.DigestValue()
	if err != nil {
		return AppendSettledToolResultResultV2{}, err
	}
	frame := contract.ContextFrame{ContractVersion: contract.Version, ID: frameID, Revision: 1, Execution: request.ParentFrame.Execution, ManifestRef: contract.FactRef{ID: manifest.ID, Revision: manifest.Revision, Digest: manifestDigest}, ParentFrame: &parentRef, GenerationID: generationID, Generation: request.ParentFrame.Generation + 1, StablePrefix: request.ParentFrame.StablePrefix, SemiStable: cloneContentRefPointerV1(request.ParentFrame.SemiStable), DynamicTail: dynamicRef, Rendered: renderedRef, SourceSetDigest: sourceSet, CreatedUnixNano: request.CheckedUnixNano, ExpiresUnixNano: expires}
	frameDigest, err := frame.DigestValue()
	if err != nil {
		return AppendSettledToolResultResultV2{}, err
	}
	parentGenerationRef, err := factRefForGenerationV2(request.ParentGeneration)
	if err != nil {
		return AppendSettledToolResultResultV2{}, err
	}
	generation := contract.ContextGeneration{ContractVersion: contract.Version, ID: generationID, Revision: 1, Ordinal: request.ParentGeneration.Ordinal + 1, Parent: &parentGenerationRef, RootFrame: contract.FactRef{ID: frame.ID, Revision: frame.Revision, Digest: frameDigest}, RetainedAnchors: []contract.FactRef{}, OpenEffects: []contract.FactRef{}, CreatedUnixNano: request.CheckedUnixNano}
	if err = generation.Validate(); err != nil {
		return AppendSettledToolResultResultV2{}, err
	}
	generationDigest, _ := contract.DigestJSON(generation)
	consumptionRequest, err := contract.SealContextFrameConsumptionRequestV1(contract.ContextFrameConsumptionRequestV1{DescriptorID: deterministicIDV2("ctx-tool-consumption-", seed), FrameRef: generation.RootFrame, ManifestRef: frame.ManifestRef, GenerationRef: contract.FactRef{ID: generation.ID, Revision: generation.Revision, Digest: generationDigest}, TenantScopeDigest: request.TenantScopeDigest, AgentInstanceRef: request.AgentInstanceRef, RunID: frame.Execution.RunID, RunScopeDigest: frame.Execution.ScopeDigest, PromptAssetRefs: request.PromptAssetRefs, RecipeRef: manifest.RecipeRef, DisclosureClass: request.DisclosureClass, CheckedUnixNano: request.CheckedUnixNano, NotAfterUnixNano: expires})
	if err != nil {
		return AppendSettledToolResultResultV2{}, err
	}
	static := staticFrameConsumptionReaderV2{snapshot: FrameConsumptionCurrentSnapshotV1{Manifest: manifest, Frame: frame, Generation: generation, GenerationExpiresUnixNano: expires, FragmentSourceExpires: append(repeatExpiryV2(len(fragments)-1, request.ParentManifest.ExpiresUnixNano), source.ExpiresUnixNano), PromptExpiresUnixNano: request.PromptExpiresUnixNano, RecipeExpiresUnixNano: request.Recipe.ExpiresUnixNano, DisclosureExpiresUnixNano: request.DisclosureExpiresUnixNano, AuthorityExpiresUnixNano: request.AuthorityExpiresUnixNano}}
	descriptor, err := BuildFrameConsumptionDescriptorV1(ctx, static, store, consumptionRequest)
	if err != nil {
		return AppendSettledToolResultResultV2{}, err
	}
	descriptor.CacheHint.InvalidationReasons = []contract.ContextCacheInvalidationReasonV1{contract.CacheInvalidationFrameChangedV1, contract.CacheInvalidationFragmentChangedV1}
	descriptor, err = contract.SealContextFrameConsumptionDescriptorV1(descriptor)
	if err != nil {
		return AppendSettledToolResultResultV2{}, err
	}
	return AppendSettledToolResultResultV2{Source: source, Manifest: manifest, Frame: frame, Generation: generation, Descriptor: descriptor}, checkContextV1(ctx)
}

type staticFrameConsumptionReaderV2 struct {
	snapshot FrameConsumptionCurrentSnapshotV1
}

func (s staticFrameConsumptionReaderV2) InspectFrameConsumptionCurrentV1(context.Context, contract.ContextFrameConsumptionRequestV1) (FrameConsumptionCurrentSnapshotV1, error) {
	return s.snapshot, nil
}

func validateAppendParentV2(ctx context.Context, store ContextAwareReferenceStoreV1, r AppendSettledToolResultRequestV2) error {
	if r.ParentManifest.Validate() != nil || r.ParentFrame.Validate() != nil || r.ParentGeneration.Validate() != nil || r.Recipe.Validate() != nil || r.TenantScopeDigest.Validate() != nil || r.AgentInstanceRef.Validate() != nil || r.PromptExpiresUnixNano <= r.CheckedUnixNano || r.DisclosureExpiresUnixNano <= r.CheckedUnixNano || r.AuthorityExpiresUnixNano <= r.CheckedUnixNano {
		return fmt.Errorf("%w: append parent snapshot", contract.ErrInvalid)
	}
	recipeDigest, err := r.Recipe.DigestValue()
	if err != nil || r.ParentManifest.RecipeRef != (contract.FactRef{ID: r.Recipe.ID, Revision: r.Recipe.Revision, Digest: recipeDigest}) {
		return fmt.Errorf("%w: append recipe binding", contract.ErrConflict)
	}
	if r.ParentFrame.ManifestRef != mustManifestRefV2(r.ParentManifest) || r.ParentGeneration.RootFrame != mustFrameRefV2(r.ParentFrame) || r.ParentFrame.Execution != r.ParentManifest.Execution {
		return fmt.Errorf("%w: append parent exact binding", contract.ErrConflict)
	}
	return InspectFrameStagedV1(ctx, store, r.ParentManifest, r.ParentFrame, legacyInspectWorkLimitsV1())
}
func toolResultPayloadV2(s contract.SettledToolResultSourceV2) ([]byte, contract.FragmentKind, error) {
	if s.Projection.Inline != nil {
		return append([]byte{}, s.Projection.Inline...), contract.FragmentToolResult, nil
	}
	if s.Projection.Artifact == nil {
		return nil, "", fmt.Errorf("%w: settled Tool payload", contract.ErrInvalid)
	}
	b, e := json.Marshal(struct {
		Artifact        toolcontract.ObjectRef `json:"artifact"`
		SchemaDigest    string                 `json:"schema_digest"`
		PayloadRevision uint64                 `json:"payload_revision"`
		Complete        bool                   `json:"complete"`
	}{*s.Projection.Artifact, string(s.Projection.Schema.ContentDigest), uint64(s.Projection.PayloadRevision), s.Projection.Complete})
	return b, contract.FragmentArtifactReference, e
}
func mustManifestRefV2(m contract.ContextManifest) contract.FactRef {
	d, _ := m.DigestValue()
	return contract.FactRef{ID: m.ID, Revision: m.Revision, Digest: d}
}
func mustFrameRefV2(f contract.ContextFrame) contract.FactRef {
	d, _ := f.DigestValue()
	return contract.FactRef{ID: f.ID, Revision: f.Revision, Digest: d}
}
func factRefForGenerationV2(g contract.ContextGeneration) (contract.FactRef, error) {
	if err := g.Validate(); err != nil {
		return contract.FactRef{}, err
	}
	d, e := contract.DigestJSON(g)
	return contract.FactRef{ID: g.ID, Revision: g.Revision, Digest: d}, e
}
func deterministicIDV2(prefix string, d contract.Digest) string {
	return prefix + strings.TrimPrefix(string(d), "sha256:")
}
func minimumExpiryV2(v int64, values ...int64) int64 {
	for _, x := range values {
		if x < v {
			v = x
		}
	}
	return v
}
func repeatExpiryV2(n int, v int64) []int64 {
	out := make([]int64, n)
	for i := range out {
		out[i] = v
	}
	return out
}

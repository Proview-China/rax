package kernel

import (
	"context"
	"fmt"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
)

type ContextPromptServiceV1 struct {
	store contextports.ContextPromptLifecycleStoreV1
}

func NewContextPromptServiceV1(store contextports.ContextPromptLifecycleStoreV1) (*ContextPromptServiceV1, error) {
	if store == nil {
		return nil, fmt.Errorf("%w: prompt store", contract.ErrInvalid)
	}
	return &ContextPromptServiceV1{store: store}, nil
}

func (s *ContextPromptServiceV1) CreateDraft(ctx context.Context, asset contract.PromptAssetV1, draft contract.ContextPromptLifecycleFactV1) (contract.FactRef, error) {
	if err := promptKernelContextErrV1(ctx); err != nil {
		return contract.FactRef{}, err
	}
	if err := draft.Validate(); err != nil {
		return contract.FactRef{}, err
	}
	assetDigest, err := asset.DigestValue()
	if err != nil {
		return contract.FactRef{}, err
	}
	assetRef := contract.PromptAssetRefV1{ID: asset.ID, Revision: asset.Revision, Digest: assetDigest}
	if draft.PromptAssetRef != assetRef {
		return contract.FactRef{}, fmt.Errorf("%w: prompt draft asset binding", contract.ErrConflict)
	}
	storedRef, err := s.store.PutPromptAssetV1(ctx, asset)
	if err != nil {
		return contract.FactRef{}, err
	}
	if storedRef != draft.PromptAssetRef {
		return contract.FactRef{}, fmt.Errorf("%w: prompt draft stored binding", contract.ErrConflict)
	}
	return s.store.CreatePromptDraftV1(ctx, draft)
}

func (s *ContextPromptServiceV1) Advance(ctx context.Context, expected contract.FactRef, next contract.ContextPromptLifecycleFactV1) (contract.FactRef, error) {
	if err := promptKernelContextErrV1(ctx); err != nil {
		return contract.FactRef{}, err
	}
	if _, err := s.store.InspectPromptLifecycleV1(ctx, expected); err != nil {
		return contract.FactRef{}, err
	}
	return s.store.CompareAndSwapPromptLifecycleV1(ctx, expected, next)
}

func (s *ContextPromptServiceV1) InspectAsset(ctx context.Context, ref contract.PromptAssetRefV1) (contract.PromptAssetV1, error) {
	return s.store.InspectPromptAssetV1(ctx, ref)
}

func (s *ContextPromptServiceV1) InspectLifecycle(ctx context.Context, ref contract.FactRef) (contract.ContextPromptLifecycleFactV1, error) {
	return s.store.InspectPromptLifecycleV1(ctx, ref)
}

func (s *ContextPromptServiceV1) InspectHead(ctx context.Context, assetRef contract.PromptAssetRefV1) (contract.ContextPromptLifecycleHeadV1, error) {
	return s.store.InspectPromptLifecycleHeadV1(ctx, assetRef)
}

func (s *ContextPromptServiceV1) BuildCandidates(ctx context.Context, request contract.BuildPromptCandidatesRequestV1) (contract.PromptCandidateSetV1, error) {
	if err := promptKernelContextErrV1(ctx); err != nil {
		return contract.PromptCandidateSetV1{}, err
	}
	if err := request.Validate(); err != nil {
		return contract.PromptCandidateSetV1{}, err
	}
	asset, err := s.store.InspectPromptAssetV1(ctx, request.PromptAssetRef)
	if err != nil {
		return contract.PromptCandidateSetV1{}, err
	}
	return ProjectPromptCandidatesV1(ctx, asset, request)
}

// ProjectPromptCandidatesV1 is the single pure projector shared by the exact
// Store-backed service and the owner-local engineering SDK preview.
func ProjectPromptCandidatesV1(ctx context.Context, asset contract.PromptAssetV1, request contract.BuildPromptCandidatesRequestV1) (contract.PromptCandidateSetV1, error) {
	if err := promptKernelContextErrV1(ctx); err != nil {
		return contract.PromptCandidateSetV1{}, err
	}
	if err := request.Validate(); err != nil {
		return contract.PromptCandidateSetV1{}, err
	}
	assetRef, err := asset.RefV1()
	if err != nil {
		return contract.PromptCandidateSetV1{}, err
	}
	if assetRef != request.PromptAssetRef {
		return contract.PromptCandidateSetV1{}, fmt.Errorf("%w: prompt preview exact asset", contract.ErrConflict)
	}
	if request.Execution.AuthorityDigest != asset.AuthorityDigest {
		return contract.PromptCandidateSetV1{}, fmt.Errorf("%w: prompt asset authority", contract.ErrUnauthorized)
	}
	compatible := false
	for _, ref := range asset.RenderCompatibility {
		if ref == request.RenderCompatibilityRef {
			compatible = true
			break
		}
	}
	if !compatible {
		return contract.PromptCandidateSetV1{}, fmt.Errorf("%w: prompt render compatibility", contract.ErrConflict)
	}
	expires := request.NotAfterUnixNano
	if asset.ExpiresUnixNano < expires {
		expires = asset.ExpiresUnixNano
	}
	if request.CreatedUnixNano < asset.CreatedUnixNano || request.CreatedUnixNano >= expires {
		return contract.PromptCandidateSetV1{}, fmt.Errorf("%w: prompt asset currentness", contract.ErrExpired)
	}
	candidates := make([]contract.ContextCandidate, 0, len(asset.Fragments))
	for index, fragment := range asset.Fragments {
		if err := promptKernelContextErrV1(ctx); err != nil {
			return contract.PromptCandidateSetV1{}, err
		}
		kind, trust, err := fragment.Role.KindAndTrustV1()
		if err != nil {
			return contract.PromptCandidateSetV1{}, err
		}
		digest, err := contract.DigestJSON(struct {
			Domain                 string                    `json:"domain"`
			AssetRef               contract.PromptAssetRefV1 `json:"asset_ref"`
			FragmentID             string                    `json:"fragment_id"`
			Ordinal                uint32                    `json:"ordinal"`
			Execution              contract.ExecutionBinding `json:"execution"`
			RenderCompatibilityRef contract.FactRef          `json:"render_compatibility_ref"`
			CreatedUnixNano        int64                     `json:"created_unix_nano"`
			ExpiresUnixNano        int64                     `json:"expires_unix_nano"`
		}{"praxis.context.prompt-candidate", request.PromptAssetRef, fragment.ID, uint32(index + 1), request.Execution, request.RenderCompatibilityRef, request.CreatedUnixNano, expires})
		if err != nil {
			return contract.PromptCandidateSetV1{}, err
		}
		suffix := strings.TrimPrefix(string(digest), "sha256:")
		candidate := contract.ContextCandidate{
			ContractVersion: contract.Version,
			ID:              fmt.Sprintf("prompt-candidate-%02d-%s", index+1, suffix),
			Revision:        1,
			Kind:            kind,
			Owner:           asset.Owner,
			Execution:       request.Execution,
			SourceRef:       asset.ID,
			SourceRevision:  asset.Revision,
			Content:         fragment.Content,
			Trust:           trust,
			Sensitivity:     asset.Sensitivity,
			Mode:            contract.MaterializationInline,
			Required:        fragment.Required,
			TokenEstimate:   fragment.TokenEstimate,
			EstimatorDigest: fragment.EstimatorDigest,
			CacheStability:  fragment.CacheStability,
			Evidence:        fragment.Evidence,
			IdempotencyKey:  fmt.Sprintf("prompt-candidate-once-%02d-%s", index+1, suffix),
			CreatedUnixNano: request.CreatedUnixNano,
			ExpiresUnixNano: expires,
		}
		if err := candidate.Validate(); err != nil {
			return contract.PromptCandidateSetV1{}, err
		}
		candidates = append(candidates, candidate)
	}
	return contract.SealPromptCandidateSetV1(contract.PromptCandidateSetV1{
		PromptAssetRef: request.PromptAssetRef, Execution: request.Execution,
		RenderCompatibilityRef: request.RenderCompatibilityRef, Candidates: candidates,
		CreatedUnixNano: request.CreatedUnixNano, ExpiresUnixNano: expires,
	})
}

func (s *ContextPromptServiceV1) ProductionAction(ctx context.Context, action contract.ContextPromptProductionActionV1) error {
	if err := promptKernelContextErrV1(ctx); err != nil {
		return err
	}
	if err := action.Validate(); err != nil {
		return err
	}
	return fmt.Errorf("%w: CTX-D07 production prompt action is not available", contract.ErrUnsupported)
}

func promptKernelContextErrV1(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("%w: nil context", contract.ErrInvalid)
	}
	return ctx.Err()
}

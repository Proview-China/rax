package contract

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	MaxFrameConsumptionRefsV1 = 512
	FrameConsumptionKeyV1     = "context-frame-consumption-v1"
)

type DisclosureClassV1 string

const (
	DisclosurePublicV1       DisclosureClassV1 = "public"
	DisclosureInternalV1     DisclosureClassV1 = "internal"
	DisclosureConfidentialV1 DisclosureClassV1 = "confidential"
	DisclosureRestrictedV1   DisclosureClassV1 = "restricted"
)

type ContextCacheInvalidationReasonV1 string

const (
	CacheInvalidationFrameChangedV1        ContextCacheInvalidationReasonV1 = "frame_changed"
	CacheInvalidationManifestChangedV1     ContextCacheInvalidationReasonV1 = "manifest_changed"
	CacheInvalidationGenerationChangedV1   ContextCacheInvalidationReasonV1 = "generation_changed"
	CacheInvalidationFragmentChangedV1     ContextCacheInvalidationReasonV1 = "fragment_changed"
	CacheInvalidationPromptChangedV1       ContextCacheInvalidationReasonV1 = "prompt_revision_changed"
	CacheInvalidationRecipeChangedV1       ContextCacheInvalidationReasonV1 = "recipe_revision_changed"
	CacheInvalidationDisclosureChangedV1   ContextCacheInvalidationReasonV1 = "disclosure_changed"
	CacheInvalidationScopeChangedV1        ContextCacheInvalidationReasonV1 = "scope_changed"
	CacheInvalidationTTLExpiredV1          ContextCacheInvalidationReasonV1 = "ttl_expired"
	CacheInvalidationOwnerCurrentUnknownV1 ContextCacheInvalidationReasonV1 = "owner_current_unknown"
)

type ContextFrameConsumptionDescriptorRefV1 struct {
	ID       string `json:"id"`
	Revision uint64 `json:"revision"`
	Digest   Digest `json:"digest"`
}

func (r ContextFrameConsumptionDescriptorRefV1) Validate() error {
	if validateID(r.ID) != nil || r.Revision != 1 || r.Digest.Validate() != nil {
		return fmt.Errorf("%w: frame consumption descriptor reference", ErrInvalid)
	}
	return nil
}

type ContextFrameConsumptionRequestV1 struct {
	ContractVersion   string             `json:"contract_version"`
	DescriptorID      string             `json:"descriptor_id"`
	FrameRef          FactRef            `json:"frame_ref"`
	ManifestRef       FactRef            `json:"manifest_ref"`
	GenerationRef     FactRef            `json:"generation_ref"`
	TenantScopeDigest Digest             `json:"tenant_scope_digest"`
	AgentInstanceRef  FactRef            `json:"agent_instance_ref"`
	RunID             string             `json:"run_id"`
	RunScopeDigest    Digest             `json:"run_scope_digest"`
	PromptAssetRefs   []PromptAssetRefV1 `json:"prompt_asset_refs"`
	RecipeRef         FactRef            `json:"recipe_ref"`
	DisclosureClass   DisclosureClassV1  `json:"disclosure_class"`
	CheckedUnixNano   int64              `json:"checked_unix_nano"`
	NotAfterUnixNano  int64              `json:"not_after_unix_nano"`
	RequestDigest     Digest             `json:"request_digest"`
}

func (r ContextFrameConsumptionRequestV1) digestValue() (Digest, error) {
	copy := r
	copy.RequestDigest = ""
	return digestDomainV1("praxis.context/frame-consumption-request-v1", copy)
}

func (r ContextFrameConsumptionRequestV1) Validate() error {
	if ValidateContract(r.ContractVersion) != nil || validateID(r.DescriptorID) != nil || r.FrameRef.Validate() != nil || r.ManifestRef.Validate() != nil || r.GenerationRef.Validate() != nil || r.TenantScopeDigest.Validate() != nil || r.AgentInstanceRef.Validate() != nil || validateID(r.RunID) != nil || r.RunScopeDigest.Validate() != nil || r.RecipeRef.Validate() != nil || !validDisclosureClassV1(r.DisclosureClass) || validateTimes(r.CheckedUnixNano, r.NotAfterUnixNano) != nil || r.RequestDigest.Validate() != nil {
		return fmt.Errorf("%w: frame consumption request", ErrInvalid)
	}
	if r.PromptAssetRefs == nil || len(r.PromptAssetRefs) > MaxFrameConsumptionRefsV1 || !canonicalPromptAssetRefsV1(r.PromptAssetRefs) {
		return fmt.Errorf("%w: frame consumption prompt references", ErrConflict)
	}
	want, err := r.digestValue()
	if err != nil || want != r.RequestDigest {
		return fmt.Errorf("%w: frame consumption request digest", ErrConflict)
	}
	return nil
}

func SealContextFrameConsumptionRequestV1(r ContextFrameConsumptionRequestV1) (ContextFrameConsumptionRequestV1, error) {
	r.ContractVersion = Version
	r.PromptAssetRefs = canonicalizePromptAssetRefsV1(r.PromptAssetRefs)
	r.RequestDigest = ""
	digest, err := r.digestValue()
	if err != nil {
		return ContextFrameConsumptionRequestV1{}, err
	}
	r.RequestDigest = digest
	return r, r.Validate()
}

type ContextCacheHintV1 struct {
	StableEligible              bool                               `json:"stable_eligible"`
	SemiStableEligible          bool                               `json:"semi_stable_eligible"`
	FrameFingerprint            Digest                             `json:"frame_fingerprint"`
	StablePrefixFingerprint     Digest                             `json:"stable_prefix_fingerprint"`
	SemiStablePrefixFingerprint *Digest                            `json:"semi_stable_prefix_fingerprint,omitempty"`
	InvalidationReasons         []ContextCacheInvalidationReasonV1 `json:"invalidation_reasons"`
	ExpiresUnixNano             int64                              `json:"expires_unix_nano"`
}

func (h ContextCacheHintV1) Validate() error {
	if h.FrameFingerprint.Validate() != nil || h.StablePrefixFingerprint.Validate() != nil || h.ExpiresUnixNano <= 0 || h.InvalidationReasons == nil || !canonicalInvalidationReasonsV1(h.InvalidationReasons) {
		return fmt.Errorf("%w: context cache hint", ErrInvalid)
	}
	if h.SemiStableEligible != (h.SemiStablePrefixFingerprint != nil) {
		return fmt.Errorf("%w: semi-stable cache hint presence", ErrConflict)
	}
	if h.SemiStablePrefixFingerprint != nil && h.SemiStablePrefixFingerprint.Validate() != nil {
		return fmt.Errorf("%w: semi-stable fingerprint", ErrInvalid)
	}
	return nil
}

type ContextFrameConsumptionDescriptorV1 struct {
	ContractVersion   string             `json:"contract_version"`
	ID                string             `json:"descriptor_id"`
	Revision          uint64             `json:"revision"`
	FrameRef          FactRef            `json:"frame_ref"`
	ManifestRef       FactRef            `json:"manifest_ref"`
	GenerationRef     FactRef            `json:"generation_ref"`
	StablePrefix      ContentRef         `json:"stable_prefix"`
	SemiStable        *ContentRef        `json:"semi_stable,omitempty"`
	DynamicTail       ContentRef         `json:"dynamic_tail"`
	Rendered          ContentRef         `json:"rendered"`
	FragmentRefs      []FactRef          `json:"fragment_refs"`
	TenantScopeDigest Digest             `json:"tenant_scope_digest"`
	AgentInstanceRef  FactRef            `json:"agent_instance_ref"`
	RunID             string             `json:"run_id"`
	RunScopeDigest    Digest             `json:"run_scope_digest"`
	PromptAssetRefs   []PromptAssetRefV1 `json:"prompt_asset_refs"`
	RecipeRef         FactRef            `json:"recipe_ref"`
	DisclosureClass   DisclosureClassV1  `json:"disclosure_class"`
	CacheHint         ContextCacheHintV1 `json:"cache_hint"`
	CheckedUnixNano   int64              `json:"checked_unix_nano"`
	ExpiresUnixNano   int64              `json:"expires_unix_nano"`
	Digest            Digest             `json:"digest"`
}

func (d ContextFrameConsumptionDescriptorV1) digestValue() (Digest, error) {
	copy := d
	copy.Digest = ""
	return digestDomainV1("praxis.context/frame-consumption-v1", copy)
}

func (d ContextFrameConsumptionDescriptorV1) Validate() error {
	if ValidateContract(d.ContractVersion) != nil || validateID(d.ID) != nil || d.Revision != 1 || d.FrameRef.Validate() != nil || d.ManifestRef.Validate() != nil || d.GenerationRef.Validate() != nil || d.StablePrefix.Validate() != nil || d.DynamicTail.Validate() != nil || d.Rendered.Validate() != nil || d.TenantScopeDigest.Validate() != nil || d.AgentInstanceRef.Validate() != nil || validateID(d.RunID) != nil || d.RunScopeDigest.Validate() != nil || d.RecipeRef.Validate() != nil || !validDisclosureClassV1(d.DisclosureClass) || d.CacheHint.Validate() != nil || validateTimes(d.CheckedUnixNano, d.ExpiresUnixNano) != nil || d.CacheHint.ExpiresUnixNano != d.ExpiresUnixNano || d.Digest.Validate() != nil {
		return fmt.Errorf("%w: frame consumption descriptor", ErrInvalid)
	}
	if d.SemiStable != nil && d.SemiStable.Validate() != nil {
		return fmt.Errorf("%w: frame consumption semi-stable reference", ErrInvalid)
	}
	if len(d.FragmentRefs) == 0 || len(d.FragmentRefs) > MaxFrameConsumptionRefsV1 || !validUniqueFactRefsV1(d.FragmentRefs) || d.PromptAssetRefs == nil || len(d.PromptAssetRefs) > MaxFrameConsumptionRefsV1 || !canonicalPromptAssetRefsV1(d.PromptAssetRefs) {
		return fmt.Errorf("%w: frame consumption descriptor references", ErrConflict)
	}
	want, err := d.digestValue()
	if err != nil || want != d.Digest {
		return fmt.Errorf("%w: frame consumption descriptor digest", ErrConflict)
	}
	return nil
}

func SealContextFrameConsumptionDescriptorV1(d ContextFrameConsumptionDescriptorV1) (ContextFrameConsumptionDescriptorV1, error) {
	d.ContractVersion = Version
	d.Revision = 1
	d.FragmentRefs = append([]FactRef{}, d.FragmentRefs...)
	d.PromptAssetRefs = canonicalizePromptAssetRefsV1(d.PromptAssetRefs)
	d.CacheHint.InvalidationReasons = canonicalizeInvalidationReasonsV1(d.CacheHint.InvalidationReasons)
	d.Digest = ""
	digest, err := d.digestValue()
	if err != nil {
		return ContextFrameConsumptionDescriptorV1{}, err
	}
	d.Digest = digest
	return d, d.Validate()
}

func (d ContextFrameConsumptionDescriptorV1) RefV1() (ContextFrameConsumptionDescriptorRefV1, error) {
	if err := d.Validate(); err != nil {
		return ContextFrameConsumptionDescriptorRefV1{}, err
	}
	return ContextFrameConsumptionDescriptorRefV1{ID: d.ID, Revision: d.Revision, Digest: d.Digest}, nil
}

type ContextFragmentCacheKeyV1 struct {
	ContractVersion        string             `json:"contract_version"`
	TenantScopeDigest      Digest             `json:"tenant_scope_digest"`
	AgentInstanceRef       FactRef            `json:"agent_instance_ref"`
	RunID                  string             `json:"run_id"`
	RunScopeDigest         Digest             `json:"run_scope_digest"`
	FragmentRef            FactRef            `json:"fragment_ref"`
	Content                ContentRef         `json:"content"`
	PromptAssetRefs        []PromptAssetRefV1 `json:"prompt_asset_refs"`
	RecipeRef              FactRef            `json:"recipe_ref"`
	DisclosureClass        DisclosureClassV1  `json:"disclosure_class"`
	InvalidationGeneration uint64             `json:"invalidation_generation"`
	KeyVersion             string             `json:"key_version"`
	Digest                 Digest             `json:"digest"`
}

func (k ContextFragmentCacheKeyV1) digestValue() (Digest, error) {
	copy := k
	copy.Digest = ""
	return digestDomainV1("praxis.context/fragment-cache-key-v1", copy)
}

func (k ContextFragmentCacheKeyV1) Validate() error {
	if ValidateContract(k.ContractVersion) != nil || k.TenantScopeDigest.Validate() != nil || k.AgentInstanceRef.Validate() != nil || validateID(k.RunID) != nil || k.RunScopeDigest.Validate() != nil || k.FragmentRef.Validate() != nil || k.Content.Validate() != nil || k.RecipeRef.Validate() != nil || !validDisclosureClassV1(k.DisclosureClass) || k.InvalidationGeneration == 0 || validateID(k.KeyVersion) != nil || k.Digest.Validate() != nil || k.PromptAssetRefs == nil || !canonicalPromptAssetRefsV1(k.PromptAssetRefs) {
		return fmt.Errorf("%w: fragment cache key", ErrInvalid)
	}
	want, err := k.digestValue()
	if err != nil || want != k.Digest {
		return fmt.Errorf("%w: fragment cache key digest", ErrConflict)
	}
	return nil
}

func SealContextFragmentCacheKeyV1(k ContextFragmentCacheKeyV1) (ContextFragmentCacheKeyV1, error) {
	k.ContractVersion = Version
	k.PromptAssetRefs = canonicalizePromptAssetRefsV1(k.PromptAssetRefs)
	k.Digest = ""
	digest, err := k.digestValue()
	if err != nil {
		return ContextFragmentCacheKeyV1{}, err
	}
	k.Digest = digest
	return k, k.Validate()
}

type ContextFrameCacheKeyV1 struct {
	ContractVersion        string             `json:"contract_version"`
	TenantScopeDigest      Digest             `json:"tenant_scope_digest"`
	AgentInstanceRef       FactRef            `json:"agent_instance_ref"`
	RunID                  string             `json:"run_id"`
	RunScopeDigest         Digest             `json:"run_scope_digest"`
	FrameRef               FactRef            `json:"frame_ref"`
	ManifestRef            FactRef            `json:"manifest_ref"`
	GenerationRef          FactRef            `json:"generation_ref"`
	PromptAssetRefs        []PromptAssetRefV1 `json:"prompt_asset_refs"`
	RecipeRef              FactRef            `json:"recipe_ref"`
	DisclosureClass        DisclosureClassV1  `json:"disclosure_class"`
	InvalidationGeneration uint64             `json:"invalidation_generation"`
	KeyVersion             string             `json:"key_version"`
	Digest                 Digest             `json:"digest"`
}

func (k ContextFrameCacheKeyV1) digestValue() (Digest, error) {
	copy := k
	copy.Digest = ""
	return digestDomainV1("praxis.context/frame-cache-key-v1", copy)
}

func (k ContextFrameCacheKeyV1) Validate() error {
	if ValidateContract(k.ContractVersion) != nil || k.TenantScopeDigest.Validate() != nil || k.AgentInstanceRef.Validate() != nil || validateID(k.RunID) != nil || k.RunScopeDigest.Validate() != nil || k.FrameRef.Validate() != nil || k.ManifestRef.Validate() != nil || k.GenerationRef.Validate() != nil || k.RecipeRef.Validate() != nil || !validDisclosureClassV1(k.DisclosureClass) || k.InvalidationGeneration == 0 || validateID(k.KeyVersion) != nil || k.Digest.Validate() != nil || k.PromptAssetRefs == nil || !canonicalPromptAssetRefsV1(k.PromptAssetRefs) {
		return fmt.Errorf("%w: frame cache key", ErrInvalid)
	}
	want, err := k.digestValue()
	if err != nil || want != k.Digest {
		return fmt.Errorf("%w: frame cache key digest", ErrConflict)
	}
	return nil
}

func SealContextFrameCacheKeyV1(k ContextFrameCacheKeyV1) (ContextFrameCacheKeyV1, error) {
	k.ContractVersion = Version
	k.PromptAssetRefs = canonicalizePromptAssetRefsV1(k.PromptAssetRefs)
	k.Digest = ""
	digest, err := k.digestValue()
	if err != nil {
		return ContextFrameCacheKeyV1{}, err
	}
	k.Digest = digest
	return k, k.Validate()
}

type FrameConsumptionErrorClassV1 string

const (
	FrameConsumptionInvalidV1       FrameConsumptionErrorClassV1 = "invalid_argument"
	FrameConsumptionUnauthorizedV1  FrameConsumptionErrorClassV1 = "unauthorized"
	FrameConsumptionNotFoundV1      FrameConsumptionErrorClassV1 = "not_found"
	FrameConsumptionExpiredV1       FrameConsumptionErrorClassV1 = "expired"
	FrameConsumptionConflictV1      FrameConsumptionErrorClassV1 = "conflict"
	FrameConsumptionUnavailableV1   FrameConsumptionErrorClassV1 = "unavailable"
	FrameConsumptionIndeterminateV1 FrameConsumptionErrorClassV1 = "indeterminate"
	FrameConsumptionLimitV1         FrameConsumptionErrorClassV1 = "limit_exceeded"
	FrameConsumptionUnsupportedV1   FrameConsumptionErrorClassV1 = "unsupported"
	FrameConsumptionInspectOnlyV1   FrameConsumptionErrorClassV1 = "inspect_only"
)

func ClassifyFrameConsumptionErrorV1(err error) FrameConsumptionErrorClassV1 {
	switch {
	case errors.Is(err, ErrInspectOnly):
		return FrameConsumptionInspectOnlyV1
	case errors.Is(err, ErrUnauthorized):
		return FrameConsumptionUnauthorizedV1
	case errors.Is(err, ErrNotFound):
		return FrameConsumptionNotFoundV1
	case errors.Is(err, ErrExpired):
		return FrameConsumptionExpiredV1
	case errors.Is(err, ErrConflict):
		return FrameConsumptionConflictV1
	case errors.Is(err, ErrUnavailable):
		return FrameConsumptionUnavailableV1
	case errors.Is(err, ErrUnknown), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return FrameConsumptionIndeterminateV1
	case errors.Is(err, ErrLimitExceeded):
		return FrameConsumptionLimitV1
	case errors.Is(err, ErrUnsupported):
		return FrameConsumptionUnsupportedV1
	default:
		return FrameConsumptionInvalidV1
	}
}

func digestDomainV1(domain string, value any) (Digest, error) {
	return DigestJSON(struct {
		Domain string `json:"domain"`
		Value  any    `json:"value"`
	}{Domain: domain, Value: value})
}

func validDisclosureClassV1(v DisclosureClassV1) bool {
	return v == DisclosurePublicV1 || v == DisclosureInternalV1 || v == DisclosureConfidentialV1 || v == DisclosureRestrictedV1
}

func canonicalizePromptAssetRefsV1(refs []PromptAssetRefV1) []PromptAssetRefV1 {
	result := append([]PromptAssetRefV1{}, refs...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].ID != result[j].ID {
			return result[i].ID < result[j].ID
		}
		if result[i].Revision != result[j].Revision {
			return result[i].Revision < result[j].Revision
		}
		return result[i].Digest < result[j].Digest
	})
	return result
}

func canonicalPromptAssetRefsV1(refs []PromptAssetRefV1) bool {
	for i, ref := range refs {
		if ref.Validate() != nil {
			return false
		}
		if i > 0 && !promptAssetRefLessV1(refs[i-1], ref) {
			return false
		}
	}
	return true
}

func promptAssetRefLessV1(left, right PromptAssetRefV1) bool {
	if left.ID != right.ID {
		return left.ID < right.ID
	}
	if left.Revision != right.Revision {
		return left.Revision < right.Revision
	}
	return left.Digest < right.Digest
}

func canonicalizeInvalidationReasonsV1(values []ContextCacheInvalidationReasonV1) []ContextCacheInvalidationReasonV1 {
	result := append([]ContextCacheInvalidationReasonV1{}, values...)
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func canonicalInvalidationReasonsV1(values []ContextCacheInvalidationReasonV1) bool {
	for i, value := range values {
		if !validInvalidationReasonV1(value) || i > 0 && values[i-1] >= value {
			return false
		}
	}
	return true
}

func validInvalidationReasonV1(value ContextCacheInvalidationReasonV1) bool {
	switch value {
	case CacheInvalidationFrameChangedV1, CacheInvalidationManifestChangedV1, CacheInvalidationGenerationChangedV1, CacheInvalidationFragmentChangedV1, CacheInvalidationPromptChangedV1, CacheInvalidationRecipeChangedV1, CacheInvalidationDisclosureChangedV1, CacheInvalidationScopeChangedV1, CacheInvalidationTTLExpiredV1, CacheInvalidationOwnerCurrentUnknownV1:
		return true
	default:
		return false
	}
}

func canonicalStringsV1(values []string) bool {
	for i, value := range values {
		if strings.TrimSpace(value) == "" || len(value) > 256 || i > 0 && values[i-1] >= value {
			return false
		}
	}
	return true
}

func validUniqueFactRefsV1(values []FactRef) bool {
	seen := make(map[FactRef]struct{}, len(values))
	for _, value := range values {
		if value.Validate() != nil {
			return false
		}
		if _, ok := seen[value]; ok {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

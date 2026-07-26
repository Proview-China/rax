package contract

import "fmt"

// ContextProjectionCacheKeyV1 indexes a provider-neutral frame-consumption
// descriptor. Provider serialization, prompt-cache keys and KV caches remain
// Model Invoker responsibilities.
type ContextProjectionCacheKeyV1 struct {
	ContractVersion        string                                 `json:"contract_version"`
	DescriptorRef          ContextFrameConsumptionDescriptorRefV1 `json:"descriptor_ref"`
	TenantScopeDigest      Digest                                 `json:"tenant_scope_digest"`
	RunScopeDigest         Digest                                 `json:"run_scope_digest"`
	DisclosureClass        DisclosureClassV1                      `json:"disclosure_class"`
	FrameFingerprint       Digest                                 `json:"frame_fingerprint"`
	InvalidationGeneration uint64                                 `json:"invalidation_generation"`
	KeyVersion             string                                 `json:"key_version"`
	Digest                 Digest                                 `json:"digest"`
}

func (k ContextProjectionCacheKeyV1) digestValue() (Digest, error) {
	copy := k
	copy.Digest = ""
	return digestDomainV1("praxis.context/projection-cache-key-v1", copy)
}

func (k ContextProjectionCacheKeyV1) Validate() error {
	if ValidateContract(k.ContractVersion) != nil || k.DescriptorRef.Validate() != nil || k.TenantScopeDigest.Validate() != nil || k.RunScopeDigest.Validate() != nil || !validDisclosureClassV1(k.DisclosureClass) || k.FrameFingerprint.Validate() != nil || k.InvalidationGeneration == 0 || validateID(k.KeyVersion) != nil || k.Digest.Validate() != nil {
		return fmt.Errorf("%w: projection cache key", ErrInvalid)
	}
	want, err := k.digestValue()
	if err != nil || want != k.Digest {
		return fmt.Errorf("%w: projection cache key digest", ErrConflict)
	}
	return nil
}

func SealContextProjectionCacheKeyV1(k ContextProjectionCacheKeyV1) (ContextProjectionCacheKeyV1, error) {
	k.ContractVersion = Version
	k.Digest = ""
	digest, err := k.digestValue()
	if err != nil {
		return ContextProjectionCacheKeyV1{}, err
	}
	k.Digest = digest
	return k, k.Validate()
}

package contract

import (
	"fmt"
	"math/big"
)

type ReuseScopeClass string

const (
	ReuseRun      ReuseScopeClass = "run"
	ReuseInstance ReuseScopeClass = "instance"
	ReuseLineage  ReuseScopeClass = "lineage"
	ReuseIdentity ReuseScopeClass = "identity"
	ReuseTenant   ReuseScopeClass = "tenant"
)

type ProviderCacheProfile struct {
	ContractVersion  string `json:"contract_version"`
	ID               string `json:"profile_id"`
	Revision         uint64 `json:"revision"`
	Provider         string `json:"provider"`
	RouteID          string `json:"route_id"`
	Model            string `json:"model"`
	RequestControl   bool   `json:"request_control"`
	KeyOwnership     bool   `json:"key_ownership"`
	TTLControl       bool   `json:"ttl_control"`
	UsageObservable  bool   `json:"usage_observable"`
	CapabilityDigest Digest `json:"capability_digest"`
	ExpiresUnixNano  int64  `json:"expires_unix_nano"`
}

func (p ProviderCacheProfile) Validate(now int64) error {
	if ValidateContract(p.ContractVersion) != nil || validateID(p.ID) != nil || p.Revision == 0 || validateID(p.Provider) != nil || validateID(p.RouteID) != nil || validateID(p.Model) != nil || p.CapabilityDigest.Validate() != nil || p.ExpiresUnixNano <= 0 || now <= 0 {
		return fmt.Errorf("%w: provider cache profile", ErrInvalid)
	}
	if now >= p.ExpiresUnixNano {
		return fmt.Errorf("%w: provider cache profile", ErrExpired)
	}
	return nil
}

func (p ProviderCacheProfile) DigestValue(now int64) (Digest, error) {
	if err := p.Validate(now); err != nil {
		return "", err
	}
	return DigestJSON(p)
}

type CachePartition struct {
	AuditScopeDigest   Digest          `json:"audit_scope_digest"`
	ReuseScope         ReuseScopeClass `json:"reuse_scope"`
	IsolationDigest    Digest          `json:"isolation_digest"`
	AuthorityDigest    Digest          `json:"authority_digest"`
	Sensitivity        Sensitivity     `json:"sensitivity"`
	SourceSetDigest    Digest          `json:"source_set_digest"`
	RecipeDigest       Digest          `json:"recipe_digest"`
	RenderDigest       Digest          `json:"render_digest"`
	ModelProfileDigest Digest          `json:"model_profile_digest"`
	HarnessDigest      Digest          `json:"harness_digest"`
	ToolSchemaDigest   Digest          `json:"tool_schema_digest"`
	PrefixDigest       Digest          `json:"prefix_digest"`
	ProviderProfileRef FactRef         `json:"provider_profile_ref"`
	KeyVersion         string          `json:"key_version"`
}

func (p CachePartition) Validate() error {
	if !validReuseScope(p.ReuseScope) || !validSensitivity(p.Sensitivity) || validateID(p.KeyVersion) != nil || p.ProviderProfileRef.Validate() != nil {
		return fmt.Errorf("%w: cache partition policy", ErrInvalid)
	}
	for _, d := range []Digest{p.AuditScopeDigest, p.IsolationDigest, p.AuthorityDigest, p.SourceSetDigest, p.RecipeDigest, p.RenderDigest, p.ModelProfileDigest, p.HarnessDigest, p.ToolSchemaDigest, p.PrefixDigest} {
		if d.Validate() != nil {
			return fmt.Errorf("%w: cache partition digest", ErrInvalid)
		}
	}
	return nil
}

func (p CachePartition) DigestValue() (Digest, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	return DigestJSON(p)
}

func (p CachePartition) KeyDigest() (Digest, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	key := struct {
		Namespace string         `json:"namespace"`
		Partition CachePartition `json:"partition"`
	}{Namespace: "praxis.context-cache-key/v1", Partition: p}
	return DigestJSON(key)
}

type CachePlan struct {
	ContractVersion string         `json:"contract_version"`
	ID              string         `json:"plan_id"`
	Revision        uint64         `json:"revision"`
	Partition       CachePartition `json:"partition"`
	EligibleTokens  uint64         `json:"eligible_tokens"`
	PredictedReads  uint64         `json:"predicted_reads"`
	ReadCostPerM    uint64         `json:"read_cost_per_million"`
	WriteCostPerM   uint64         `json:"write_cost_per_million"`
	KeepaliveCost   uint64         `json:"keepalive_cost"`
	TTL             int64          `json:"ttl_nanos"`
	CreatedUnixNano int64          `json:"created_unix_nano"`
	ExpiresUnixNano int64          `json:"expires_unix_nano"`
}

func (p CachePlan) Validate() error {
	if ValidateContract(p.ContractVersion) != nil || validateID(p.ID) != nil || p.Revision != 1 || p.Partition.Validate() != nil || p.EligibleTokens == 0 || p.TTL <= 0 || validateTimes(p.CreatedUnixNano, p.ExpiresUnixNano) != nil || p.ExpiresUnixNano-p.CreatedUnixNano > p.TTL {
		return fmt.Errorf("%w: cache plan", ErrInvalid)
	}
	return nil
}

func (p CachePlan) ValidateCurrent(profile ProviderCacheProfile, now int64) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if now < p.CreatedUnixNano || now >= p.ExpiresUnixNano {
		return fmt.Errorf("%w: cache plan currentness", ErrExpired)
	}
	profileDigest, err := profile.DigestValue(now)
	if err != nil {
		return err
	}
	ref := p.Partition.ProviderProfileRef
	if ref.ID != profile.ID || ref.Revision != profile.Revision || ref.Digest != profileDigest {
		return fmt.Errorf("%w: provider profile reference", ErrConflict)
	}
	if p.ExpiresUnixNano > profile.ExpiresUnixNano {
		return fmt.Errorf("%w: plan outlives provider profile", ErrConflict)
	}
	return nil
}

type CacheEconomicDecision struct {
	WorthCreating bool   `json:"worth_creating"`
	ExpectedRead  uint64 `json:"expected_read_savings"`
	ExpectedCost  uint64 `json:"expected_write_keepalive_cost"`
	Reason        string `json:"reason"`
}

func CompareCacheEconomics(plan CachePlan) (CacheEconomicDecision, error) {
	if err := plan.Validate(); err != nil {
		return CacheEconomicDecision{}, err
	}
	read := saturatingProductDiv([]uint64{plan.EligibleTokens, plan.PredictedReads, plan.ReadCostPerM}, 1_000_000)
	write := saturatingAdd(saturatingProductDiv([]uint64{plan.EligibleTokens, plan.WriteCostPerM}, 1_000_000), plan.KeepaliveCost)
	decision := CacheEconomicDecision{ExpectedRead: read, ExpectedCost: write, WorthCreating: read > write}
	if decision.WorthCreating {
		decision.Reason = "expected_savings_positive"
	} else {
		decision.Reason = "expected_savings_not_positive"
	}
	return decision, nil
}

type CacheEntryState string

const (
	CacheEntryCurrent     CacheEntryState = "current"
	CacheEntryInvalidated CacheEntryState = "invalidated"
	CacheEntryExpired     CacheEntryState = "expired"
)

type CacheEntry struct {
	ContractVersion        string          `json:"contract_version"`
	ID                     string          `json:"entry_id"`
	Revision               uint64          `json:"revision"`
	PartitionDigest        Digest          `json:"partition_digest"`
	KeyDigest              Digest          `json:"key_digest"`
	PrefixDigest           Digest          `json:"prefix_digest"`
	AuthorityDigest        Digest          `json:"authority_digest"`
	State                  CacheEntryState `json:"state"`
	InvalidationGeneration uint64          `json:"invalidation_generation"`
	CreatedUnixNano        int64           `json:"created_unix_nano"`
	ExpiresUnixNano        int64           `json:"expires_unix_nano"`
}

func (e CacheEntry) Validate() error {
	if ValidateContract(e.ContractVersion) != nil || validateID(e.ID) != nil || e.Revision == 0 || e.InvalidationGeneration == 0 || validateTimes(e.CreatedUnixNano, e.ExpiresUnixNano) != nil {
		return fmt.Errorf("%w: cache entry", ErrInvalid)
	}
	for _, d := range []Digest{e.PartitionDigest, e.KeyDigest, e.PrefixDigest, e.AuthorityDigest} {
		if d.Validate() != nil {
			return fmt.Errorf("%w: cache entry digest", ErrInvalid)
		}
	}
	if e.State != CacheEntryCurrent && e.State != CacheEntryInvalidated && e.State != CacheEntryExpired {
		return fmt.Errorf("%w: cache entry state", ErrInvalid)
	}
	return nil
}

type ProviderCacheUsageObservation struct {
	ObservationID    string `json:"observation_id"`
	ReadTokens       uint64 `json:"read_tokens"`
	WriteTokens      uint64 `json:"write_tokens"`
	ObservedUnixNano int64  `json:"observed_unix_nano"`
}

func (o ProviderCacheUsageObservation) Validate() error {
	if validateID(o.ObservationID) != nil || o.ObservedUnixNano <= 0 {
		return fmt.Errorf("%w: cache usage observation", ErrInvalid)
	}
	return nil
}

type CacheAccessFact struct {
	EntryRef          FactRef `json:"entry_ref"`
	PartitionDigest   Digest  `json:"partition_digest"`
	AuthorityDigest   Digest  `json:"authority_digest"`
	InspectedUnixNano int64   `json:"inspected_unix_nano"`
}

func validReuseScope(v ReuseScopeClass) bool {
	return v == ReuseRun || v == ReuseInstance || v == ReuseLineage || v == ReuseIdentity || v == ReuseTenant
}

func saturatingProductDiv(factors []uint64, divisor uint64) uint64 {
	if divisor == 0 {
		return ^uint64(0)
	}
	product := new(big.Int).SetUint64(1)
	for _, factor := range factors {
		if factor == 0 {
			return 0
		}
		product.Mul(product, new(big.Int).SetUint64(factor))
	}
	product.Div(product, new(big.Int).SetUint64(divisor))
	if product.BitLen() > 64 {
		return ^uint64(0)
	}
	return product.Uint64()
}

func saturatingAdd(a, b uint64) uint64 {
	if ^uint64(0)-a < b {
		return ^uint64(0)
	}
	return a + b
}

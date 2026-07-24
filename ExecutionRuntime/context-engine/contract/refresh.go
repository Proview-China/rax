package contract

import (
	"fmt"
	"strings"
)

const (
	ContextTurnRefreshPendingV1        = "pending_domain_result"
	ContextTurnRefreshAppliedV1        = "applied_current"
	MaxContextTurnRefreshSourceBytesV1 = 64 * 1024
)

// ContextTurnRefreshSourceCardinalityV1 keeps one settled Tool result and
// admits at most one independently inspected contribution from each Owner.
type ContextTurnRefreshSourceCardinalityV1 struct {
	Tool       uint32 `json:"tool"`
	Memory     uint32 `json:"memory"`
	Knowledge  uint32 `json:"knowledge"`
	Continuity uint32 `json:"continuity"`
}

func (c ContextTurnRefreshSourceCardinalityV1) Validate() error {
	if c.Tool != 1 || c.Memory > 1 || c.Knowledge > 1 || c.Continuity != 0 {
		return fmt.Errorf("%w: refresh source cardinality", ErrUnsupported)
	}
	return nil
}

// SettledActionContextSourceRequestV1 carries only exact neutral coordinates.
// It does not make Context the owner of Tool, Runtime inspection or association facts.
type SettledActionContextSourceRequestV1 struct {
	ToolResultRef      FactRef          `json:"tool_result_ref"`
	DomainResultRef    FactRef          `json:"domain_result_ref"`
	ApplySettlementRef FactRef          `json:"apply_settlement_ref"`
	InspectionRef      FactRef          `json:"inspection_ref"`
	AssociationRef     FactRef          `json:"association_ref"`
	Execution          ExecutionBinding `json:"execution"`
	ActionID           string           `json:"action_id"`
	AttemptID          string           `json:"attempt_id"`
}

func (r SettledActionContextSourceRequestV1) Validate() error {
	for _, ref := range []FactRef{r.ToolResultRef, r.DomainResultRef, r.ApplySettlementRef, r.InspectionRef, r.AssociationRef} {
		if ref.Validate() != nil {
			return fmt.Errorf("%w: settled tool exact chain", ErrInvalid)
		}
	}
	if r.Execution.Validate() != nil || validateID(r.ActionID) != nil || validateID(r.AttemptID) != nil {
		return fmt.Errorf("%w: settled tool binding", ErrInvalid)
	}
	return nil
}

// SettledActionContextSourceCurrentV1 is an Observation-like read projection.
// Its body is already bounded or is an exact artifact reference serialized by
// its Owner; raw Provider/Receipt output is not representable here.
type SettledActionContextSourceCurrentV1 struct {
	ContractVersion string                              `json:"contract_version"`
	Request         SettledActionContextSourceRequestV1 `json:"request"`
	Content         ContentRef                          `json:"content"`
	TokenEstimate   uint64                              `json:"token_estimate"`
	Sensitivity     Sensitivity                         `json:"sensitivity"`
	CheckedUnixNano int64                               `json:"checked_unix_nano"`
	ExpiresUnixNano int64                               `json:"expires_unix_nano"`
	SourceDigest    Digest                              `json:"source_digest"`
	Digest          Digest                              `json:"digest"`
}

func (p SettledActionContextSourceCurrentV1) sourceDigestValue() (Digest, error) {
	return DigestJSON(struct {
		Request       SettledActionContextSourceRequestV1 `json:"request"`
		Content       ContentRef                          `json:"content"`
		TokenEstimate uint64                              `json:"token_estimate"`
		Sensitivity   Sensitivity                         `json:"sensitivity"`
		Expires       int64                               `json:"expires_unix_nano"`
	}{p.Request, p.Content, p.TokenEstimate, p.Sensitivity, p.ExpiresUnixNano})
}

func (p SettledActionContextSourceCurrentV1) digestValue() (Digest, error) {
	copy := p
	copy.Digest = ""
	return DigestJSON(copy)
}

func (p SettledActionContextSourceCurrentV1) ValidateAt(now int64) error {
	if ValidateContract(p.ContractVersion) != nil || p.Request.Validate() != nil || p.Content.Validate() != nil || p.Content.Length > MaxContextTurnRefreshSourceBytesV1 || p.TokenEstimate == 0 || !validSensitivity(p.Sensitivity) || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || now < p.CheckedUnixNano || now >= p.ExpiresUnixNano || p.SourceDigest.Validate() != nil || p.Digest.Validate() != nil {
		return fmt.Errorf("%w: settled tool current projection", ErrExpired)
	}
	sourceDigest, err := p.sourceDigestValue()
	if err != nil || sourceDigest != p.SourceDigest {
		return fmt.Errorf("%w: settled tool source digest", ErrConflict)
	}
	digest, err := p.digestValue()
	if err != nil || digest != p.Digest {
		return fmt.Errorf("%w: settled tool projection digest", ErrConflict)
	}
	return nil
}

func SealSettledActionContextSourceCurrentV1(p SettledActionContextSourceCurrentV1, now int64) (SettledActionContextSourceCurrentV1, error) {
	p.ContractVersion = Version
	p.SourceDigest = ""
	p.Digest = ""
	var err error
	p.SourceDigest, err = p.sourceDigestValue()
	if err != nil {
		return SettledActionContextSourceCurrentV1{}, err
	}
	p.Digest, err = p.digestValue()
	if err != nil {
		return SettledActionContextSourceCurrentV1{}, err
	}
	return p, p.ValidateAt(now)
}

// ContextStableCacheIdentityV1 excludes DynamicTail by construction.
type ContextStableCacheIdentityV1 struct {
	ReuseScope            string      `json:"reuse_scope"`
	IsolationDigest       Digest      `json:"isolation_digest"`
	AuthorityDigest       Digest      `json:"authority_digest"`
	StableSourceSetDigest Digest      `json:"stable_source_set_digest"`
	RecipeRef             FactRef     `json:"recipe_ref"`
	RenderVersion         string      `json:"render_version"`
	ModelProfileDigest    Digest      `json:"model_profile_digest"`
	HarnessGenerationRef  FactRef     `json:"harness_generation_ref"`
	ToolSchemaDigest      Digest      `json:"tool_schema_digest"`
	StablePrefix          ContentRef  `json:"stable_prefix"`
	SemiStable            *ContentRef `json:"semi_stable,omitempty"`
	PrefixDigest          Digest      `json:"prefix_digest"`
	ProviderProfileDigest Digest      `json:"provider_profile_digest"`
	KeyVersion            string      `json:"key_version"`
	ExpiresUnixNano       int64       `json:"expires_unix_nano"`
	Digest                Digest      `json:"digest"`
}

func (c ContextStableCacheIdentityV1) digestValue() (Digest, error) {
	copy := c
	copy.Digest = ""
	return DigestJSON(copy)
}

func (c ContextStableCacheIdentityV1) ValidateAt(now int64) error {
	if validateID(c.ReuseScope) != nil || c.IsolationDigest.Validate() != nil || c.AuthorityDigest.Validate() != nil || c.StableSourceSetDigest.Validate() != nil || c.RecipeRef.Validate() != nil || validateID(c.RenderVersion) != nil || c.ModelProfileDigest.Validate() != nil || c.HarnessGenerationRef.Validate() != nil || c.ToolSchemaDigest.Validate() != nil || c.StablePrefix.Validate() != nil || c.PrefixDigest.Validate() != nil || c.ProviderProfileDigest.Validate() != nil || validateID(c.KeyVersion) != nil || c.ExpiresUnixNano <= now || c.Digest.Validate() != nil {
		return fmt.Errorf("%w: stable cache identity", ErrInvalid)
	}
	if c.SemiStable != nil && c.SemiStable.Validate() != nil {
		return fmt.Errorf("%w: semi-stable cache identity", ErrInvalid)
	}
	prefixDigest, err := DigestJSON(struct {
		Stable ContentRef  `json:"stable"`
		Semi   *ContentRef `json:"semi,omitempty"`
	}{c.StablePrefix, c.SemiStable})
	if err != nil || prefixDigest != c.PrefixDigest {
		return fmt.Errorf("%w: prefix digest", ErrConflict)
	}
	digest, err := c.digestValue()
	if err != nil || digest != c.Digest {
		return fmt.Errorf("%w: cache identity digest", ErrConflict)
	}
	return nil
}

func SealContextStableCacheIdentityV1(c ContextStableCacheIdentityV1, now int64) (ContextStableCacheIdentityV1, error) {
	c.PrefixDigest, _ = DigestJSON(struct {
		Stable ContentRef  `json:"stable"`
		Semi   *ContentRef `json:"semi,omitempty"`
	}{c.StablePrefix, c.SemiStable})
	c.Digest = ""
	var err error
	c.Digest, err = c.digestValue()
	if err != nil {
		return ContextStableCacheIdentityV1{}, err
	}
	return c, c.ValidateAt(now)
}

type ContextTurnRefreshRequestV1 struct {
	ContractVersion  string                                            `json:"contract_version"`
	RefreshAttemptID string                                            `json:"refresh_attempt_id"`
	ManifestID       string                                            `json:"manifest_id"`
	FrameID          string                                            `json:"frame_id"`
	NextGenerationID string                                            `json:"next_generation_id"`
	IdempotencyKey   string                                            `json:"idempotency_key"`
	ParentSource     ContextParentFrameApplicabilitySourceCoordinateV1 `json:"parent_source"`
	ExpectedCurrent  ContextGenerationCurrentPointerV1                 `json:"expected_current"`
	Recipe           ContextRecipe                                     `json:"recipe"`
	ToolSource       SettledActionContextSourceRequestV1               `json:"tool_source"`
	MemorySource     *ContextOwnerSourceContributionV1                 `json:"memory_source,omitempty"`
	KnowledgeSource  *ContextOwnerSourceContributionV1                 `json:"knowledge_source,omitempty"`
	Cardinality      ContextTurnRefreshSourceCardinalityV1             `json:"cardinality"`
	CacheIdentity    ContextStableCacheIdentityV1                      `json:"cache_identity"`
	CheckedUnixNano  int64                                             `json:"checked_unix_nano"`
	NotAfterUnixNano int64                                             `json:"not_after_unix_nano"`
	Digest           Digest                                            `json:"digest"`
}

func (r ContextTurnRefreshRequestV1) seedDigest() (Digest, error) {
	copy := r
	copy.ContractVersion, copy.RefreshAttemptID, copy.ManifestID, copy.FrameID, copy.NextGenerationID, copy.Digest = "", "", "", "", "", ""
	return DigestJSON(copy)
}

func refreshID(prefix string, digest Digest) string {
	return prefix + strings.TrimPrefix(string(digest), "sha256:")
}

func SealContextTurnRefreshRequestV1(r ContextTurnRefreshRequestV1) (ContextTurnRefreshRequestV1, error) {
	r.ContractVersion = Version
	r.RefreshAttemptID, r.ManifestID, r.FrameID, r.NextGenerationID, r.Digest = "", "", "", "", ""
	seed, err := r.seedDigest()
	if err != nil {
		return ContextTurnRefreshRequestV1{}, err
	}
	r.RefreshAttemptID = refreshID("ctx-refresh-attempt-", seed)
	r.ManifestID = refreshID("ctx-refresh-manifest-", seed)
	r.FrameID = refreshID("ctx-refresh-frame-", seed)
	r.NextGenerationID = refreshID("ctx-refresh-generation-", seed)
	r.Digest, err = r.digestValue()
	if err != nil {
		return ContextTurnRefreshRequestV1{}, err
	}
	return r, r.Validate()
}

func (r ContextTurnRefreshRequestV1) digestValue() (Digest, error) {
	copy := r
	copy.Digest = ""
	return DigestJSON(copy)
}

func (r ContextTurnRefreshRequestV1) Validate() error {
	if ValidateContract(r.ContractVersion) != nil || validateID(r.IdempotencyKey) != nil || r.ParentSource.Validate() != nil || r.ExpectedCurrent.Validate() != nil || r.Recipe.Validate() != nil || r.ToolSource.Validate() != nil || r.Cardinality.Validate() != nil || r.CacheIdentity.ValidateAt(r.CheckedUnixNano) != nil || r.CheckedUnixNano <= 0 || r.NotAfterUnixNano <= r.CheckedUnixNano || r.Digest.Validate() != nil {
		return fmt.Errorf("%w: context turn refresh request", ErrInvalid)
	}
	if (r.MemorySource == nil) != (r.Cardinality.Memory == 0) || (r.KnowledgeSource == nil) != (r.Cardinality.Knowledge == 0) {
		return fmt.Errorf("%w: refresh owner source cardinality", ErrConflict)
	}
	for owner, source := range map[ContextOwnerSourceKindV1]*ContextOwnerSourceContributionV1{ContextOwnerSourceMemoryV1: r.MemorySource, ContextOwnerSourceKnowledgeV1: r.KnowledgeSource} {
		if source == nil {
			continue
		}
		if source.Owner != owner || source.Validate() != nil || source.SourceTurnOrdinal != r.ExpectedCurrent.Turn || source.ExpiresUnixNano > r.NotAfterUnixNano {
			return fmt.Errorf("%w: refresh owner source binding", ErrConflict)
		}
	}
	seed, err := r.seedDigest()
	if err != nil {
		return err
	}
	if r.RefreshAttemptID != refreshID("ctx-refresh-attempt-", seed) || r.ManifestID != refreshID("ctx-refresh-manifest-", seed) || r.FrameID != refreshID("ctx-refresh-frame-", seed) || r.NextGenerationID != refreshID("ctx-refresh-generation-", seed) {
		return fmt.Errorf("%w: deterministic refresh identity", ErrConflict)
	}
	digest, err := r.digestValue()
	if err != nil || digest != r.Digest {
		return fmt.Errorf("%w: refresh request digest", ErrConflict)
	}
	if r.ExpectedCurrent.ExecutionScopeDigest != r.ToolSource.Execution.ScopeDigest || r.ExpectedCurrent.RunID != r.ToolSource.Execution.RunID || r.ExpectedCurrent.Turn != r.ToolSource.Execution.Turn || r.CacheIdentity.AuthorityDigest != r.ToolSource.Execution.AuthorityDigest {
		return fmt.Errorf("%w: refresh execution binding", ErrConflict)
	}
	return nil
}

type ContextTurnRefreshPendingDomainResultV1 struct {
	ContractVersion        string                                            `json:"contract_version"`
	ID                     string                                            `json:"id"`
	Revision               uint64                                            `json:"revision"`
	RequestDigest          Digest                                            `json:"request_digest"`
	ParentProjectionDigest Digest                                            `json:"parent_projection_digest"`
	ToolSourceDigest       Digest                                            `json:"tool_source_digest"`
	ManifestRef            FactRef                                           `json:"manifest_ref"`
	FrameRef               FactRef                                           `json:"frame_ref"`
	GenerationRef          FactRef                                           `json:"generation_ref"`
	ChildSource            ContextParentFrameApplicabilitySourceCoordinateV1 `json:"child_source"`
	ExpectedCurrent        ContextGenerationCurrentPointerV1                 `json:"expected_current"`
	NextCurrent            ContextGenerationCurrentPointerV1                 `json:"next_current"`
	CacheIdentityDigest    Digest                                            `json:"cache_identity_digest"`
	CreatedUnixNano        int64                                             `json:"created_unix_nano"`
	ExpiresUnixNano        int64                                             `json:"expires_unix_nano"`
	Status                 string                                            `json:"status"`
}

func (p ContextTurnRefreshPendingDomainResultV1) Validate() error {
	if ValidateContract(p.ContractVersion) != nil || validateID(p.ID) != nil || p.Revision != 1 || p.RequestDigest.Validate() != nil || p.ParentProjectionDigest.Validate() != nil || p.ToolSourceDigest.Validate() != nil || p.ManifestRef.Validate() != nil || p.FrameRef.Validate() != nil || p.GenerationRef.Validate() != nil || p.ChildSource.Validate() != nil || p.ExpectedCurrent.Validate() != nil || p.NextCurrent.Validate() != nil || p.CacheIdentityDigest.Validate() != nil || validateTimes(p.CreatedUnixNano, p.ExpiresUnixNano) != nil || p.Status != ContextTurnRefreshPendingV1 {
		return fmt.Errorf("%w: pending refresh domain result", ErrInvalid)
	}
	return nil
}

func (p ContextTurnRefreshPendingDomainResultV1) DigestValue() (Digest, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	return DigestJSON(p)
}

type ContextTurnRefreshPendingRecordV1 struct {
	Request          ContextTurnRefreshRequestV1             `json:"request"`
	ParentProjection ContextParentFrameCurrentProjectionV1   `json:"parent_projection"`
	ToolProjection   SettledActionContextSourceCurrentV1     `json:"tool_projection"`
	Manifest         ContextManifest                         `json:"manifest"`
	Frame            ContextFrame                            `json:"frame"`
	Generation       ContextGeneration                       `json:"generation"`
	Binding          ContextParentFrameSourceBindingV1       `json:"binding"`
	Pointer          ContextGenerationCurrentPointerV1       `json:"pointer"`
	Pending          ContextTurnRefreshPendingDomainResultV1 `json:"pending"`
}

type ContextTurnRefreshPreparedV1 struct {
	AttemptRef             FactRef `json:"attempt_ref"`
	PendingDomainResultRef FactRef `json:"pending_domain_result_ref"`
	ManifestRef            FactRef `json:"manifest_ref"`
	FrameRef               FactRef `json:"frame_ref"`
	GenerationRef          FactRef `json:"generation_ref"`
	CheckedUnixNano        int64   `json:"checked_unix_nano"`
	ExpiresUnixNano        int64   `json:"expires_unix_nano"`
	Status                 string  `json:"status"`
	Digest                 Digest  `json:"digest"`
}

func (p ContextTurnRefreshPreparedV1) digestValue() (Digest, error) {
	copy := p
	copy.Digest = ""
	return DigestJSON(copy)
}
func (p ContextTurnRefreshPreparedV1) ValidateAt(now int64) error {
	if p.AttemptRef.Validate() != nil || p.PendingDomainResultRef.Validate() != nil || p.ManifestRef.Validate() != nil || p.FrameRef.Validate() != nil || p.GenerationRef.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || now < p.CheckedUnixNano || now >= p.ExpiresUnixNano || p.Status != ContextTurnRefreshPendingV1 || p.Digest.Validate() != nil {
		return fmt.Errorf("%w: refresh prepared", ErrExpired)
	}
	d, e := p.digestValue()
	if e != nil || d != p.Digest {
		return fmt.Errorf("%w: refresh prepared digest", ErrConflict)
	}
	return nil
}
func SealContextTurnRefreshPreparedV1(p ContextTurnRefreshPreparedV1, now int64) (ContextTurnRefreshPreparedV1, error) {
	p.Digest = ""
	var e error
	p.Digest, e = p.digestValue()
	if e != nil {
		return ContextTurnRefreshPreparedV1{}, e
	}
	return p, p.ValidateAt(now)
}

type ApplyContextTurnRefreshRequestV1 struct {
	ContractVersion        string                            `json:"contract_version"`
	AttemptRef             FactRef                           `json:"attempt_ref"`
	PendingDomainResultRef FactRef                           `json:"pending_domain_result_ref"`
	ExpectedCurrent        ContextGenerationCurrentPointerV1 `json:"expected_current"`
	TransitionProofRef     *FactRef                          `json:"transition_proof_ref,omitempty"`
	StableSourceSetDigest  Digest                            `json:"stable_source_set_digest,omitempty"`
	S2AssociationSetDigest Digest                            `json:"s2_association_set_digest,omitempty"`
	CheckedUnixNano        int64                             `json:"checked_unix_nano"`
	NotAfterUnixNano       int64                             `json:"not_after_unix_nano"`
	Digest                 Digest                            `json:"digest"`
}

func (r ApplyContextTurnRefreshRequestV1) digestValue() (Digest, error) {
	copy := r
	copy.Digest = ""
	return DigestJSON(copy)
}
func (r ApplyContextTurnRefreshRequestV1) ValidateAt(now int64) error {
	if ValidateContract(r.ContractVersion) != nil || r.AttemptRef.Validate() != nil || r.PendingDomainResultRef.Validate() != nil || r.ExpectedCurrent.Validate() != nil || r.CheckedUnixNano <= 0 || r.NotAfterUnixNano <= r.CheckedUnixNano || now < r.CheckedUnixNano || now >= r.NotAfterUnixNano || r.Digest.Validate() != nil {
		return fmt.Errorf("%w: apply refresh request", ErrExpired)
	}
	if !validTransitionApplyBinding(r.TransitionProofRef, r.StableSourceSetDigest, r.S2AssociationSetDigest) {
		return fmt.Errorf("%w: apply refresh transition binding", ErrConflict)
	}
	d, e := r.digestValue()
	if e != nil || d != r.Digest {
		return fmt.Errorf("%w: apply refresh digest", ErrConflict)
	}
	return nil
}
func SealApplyContextTurnRefreshRequestV1(r ApplyContextTurnRefreshRequestV1, now int64) (ApplyContextTurnRefreshRequestV1, error) {
	r.ContractVersion = Version
	r.Digest = ""
	var e error
	r.Digest, e = r.digestValue()
	if e != nil {
		return ApplyContextTurnRefreshRequestV1{}, e
	}
	return r, r.ValidateAt(now)
}

type InspectContextTurnRefreshRequestV1 struct {
	AttemptRef FactRef `json:"attempt_ref"`
}

func (r InspectContextTurnRefreshRequestV1) Validate() error {
	if r.AttemptRef.Validate() != nil {
		return fmt.Errorf("%w: inspect refresh request", ErrInvalid)
	}
	return nil
}

type ContextTurnRefreshApplySettlementV1 struct {
	ContractVersion        string   `json:"contract_version"`
	ID                     string   `json:"id"`
	Revision               uint64   `json:"revision"`
	AttemptRef             FactRef  `json:"attempt_ref"`
	PendingDomainResultRef FactRef  `json:"pending_domain_result_ref"`
	TransitionProofRef     *FactRef `json:"transition_proof_ref,omitempty"`
	StableSourceSetDigest  Digest   `json:"stable_source_set_digest,omitempty"`
	S2AssociationSetDigest Digest   `json:"s2_association_set_digest,omitempty"`
	PreviousCurrentDigest  Digest   `json:"previous_current_digest"`
	CurrentGenerationRef   FactRef  `json:"current_generation_ref"`
	AppliedUnixNano        int64    `json:"applied_unix_nano"`
}

func (s ContextTurnRefreshApplySettlementV1) Validate() error {
	if ValidateContract(s.ContractVersion) != nil || validateID(s.ID) != nil || s.Revision != 1 || s.AttemptRef.Validate() != nil || s.PendingDomainResultRef.Validate() != nil || s.PreviousCurrentDigest.Validate() != nil || s.CurrentGenerationRef.Validate() != nil || s.AppliedUnixNano <= 0 {
		return fmt.Errorf("%w: local refresh apply settlement", ErrInvalid)
	}
	if !validTransitionApplyBinding(s.TransitionProofRef, s.StableSourceSetDigest, s.S2AssociationSetDigest) {
		return fmt.Errorf("%w: local refresh transition settlement binding", ErrConflict)
	}
	return nil
}

func validTransitionApplyBinding(proof *FactRef, stable, s2 Digest) bool {
	if proof == nil {
		return stable == "" && s2 == ""
	}
	return proof.Validate() == nil && stable.Validate() == nil && s2.Validate() == nil
}
func (s ContextTurnRefreshApplySettlementV1) DigestValue() (Digest, error) {
	if e := s.Validate(); e != nil {
		return "", e
	}
	return DigestJSON(s)
}

type ContextTurnRefreshResultV1 struct {
	AttemptRef             FactRef                            `json:"attempt_ref"`
	PendingDomainResultRef FactRef                            `json:"pending_domain_result_ref"`
	ManifestRef            FactRef                            `json:"manifest_ref"`
	FrameRef               FactRef                            `json:"frame_ref"`
	GenerationRef          FactRef                            `json:"generation_ref"`
	TransitionProofRef     *FactRef                           `json:"transition_proof_ref,omitempty"`
	StableSourceSetDigest  Digest                             `json:"stable_source_set_digest,omitempty"`
	S2AssociationSetDigest Digest                             `json:"s2_association_set_digest,omitempty"`
	ApplySettlementRef     *FactRef                           `json:"apply_settlement_ref,omitempty"`
	Current                *ContextGenerationCurrentPointerV1 `json:"current,omitempty"`
	Status                 string                             `json:"status"`
	Digest                 Digest                             `json:"digest"`
}

func (r ContextTurnRefreshResultV1) digestValue() (Digest, error) {
	copy := r
	copy.Digest = ""
	return DigestJSON(copy)
}
func (r ContextTurnRefreshResultV1) Validate() error {
	if r.AttemptRef.Validate() != nil || r.PendingDomainResultRef.Validate() != nil || r.ManifestRef.Validate() != nil || r.FrameRef.Validate() != nil || r.GenerationRef.Validate() != nil || r.Digest.Validate() != nil {
		return fmt.Errorf("%w: refresh result", ErrInvalid)
	}
	switch r.Status {
	case ContextTurnRefreshPendingV1:
		if r.ApplySettlementRef != nil || r.Current != nil || r.TransitionProofRef != nil || r.StableSourceSetDigest != "" || r.S2AssociationSetDigest != "" {
			return fmt.Errorf("%w: pending refresh visibility", ErrConflict)
		}
	case ContextTurnRefreshAppliedV1:
		if r.ApplySettlementRef == nil || r.ApplySettlementRef.Validate() != nil || r.Current == nil || r.Current.Validate() != nil {
			return fmt.Errorf("%w: applied refresh visibility", ErrConflict)
		}
		if !validTransitionApplyBinding(r.TransitionProofRef, r.StableSourceSetDigest, r.S2AssociationSetDigest) {
			return fmt.Errorf("%w: applied refresh transition binding", ErrConflict)
		}
	default:
		return fmt.Errorf("%w: refresh result status", ErrInvalid)
	}
	d, e := r.digestValue()
	if e != nil || d != r.Digest {
		return fmt.Errorf("%w: refresh result digest", ErrConflict)
	}
	return nil
}
func SealContextTurnRefreshResultV1(r ContextTurnRefreshResultV1) (ContextTurnRefreshResultV1, error) {
	r.Digest = ""
	var e error
	r.Digest, e = r.digestValue()
	if e != nil {
		return ContextTurnRefreshResultV1{}, e
	}
	return r, r.Validate()
}

type ContextTurnRefreshCommitV1 struct {
	Apply           ApplyContextTurnRefreshRequestV1    `json:"apply"`
	Settlement      ContextTurnRefreshApplySettlementV1 `json:"settlement"`
	AppliedUnixNano int64                               `json:"applied_unix_nano"`
}

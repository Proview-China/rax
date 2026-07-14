package ports

import (
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	EffectContractVersionV2 = "praxis.runtime.effect/v2"
	MaxDispatchPermitTTL    = 30 * time.Second
)

type EffectKindV2 NamespacedNameV2

type ProviderBindingRefV2 struct {
	BindingSetID       string           `json:"binding_set_id"`
	BindingSetRevision core.Revision    `json:"binding_set_revision"`
	ComponentID        ComponentIDV2    `json:"component_id"`
	ManifestDigest     core.Digest      `json:"manifest_digest"`
	ArtifactDigest     core.Digest      `json:"artifact_digest"`
	Capability         CapabilityNameV2 `json:"capability"`
}

type EffectOwnerRefV2 struct {
	Role           OwnerRoleV2   `json:"role"`
	ComponentID    ComponentIDV2 `json:"component_id"`
	ManifestDigest core.Digest   `json:"manifest_digest"`
}

type AuthorityBindingRefV2 struct {
	Ref      string        `json:"ref"`
	Digest   core.Digest   `json:"digest"`
	Revision core.Revision `json:"revision"`
	Epoch    core.Epoch    `json:"epoch"`
}

type ReviewBindingRefV2 struct {
	Ref          string        `json:"ref"`
	Digest       core.Digest   `json:"digest"`
	Revision     core.Revision `json:"revision"`
	PolicyDigest core.Digest   `json:"policy_digest"`
}

type BudgetBindingRefV2 struct {
	Ref          string        `json:"ref"`
	Digest       core.Digest   `json:"digest"`
	Revision     core.Revision `json:"revision"`
	PolicyDigest core.Digest   `json:"policy_digest"`
}

type DispatchPolicyBindingRefV2 struct {
	Ref      string        `json:"ref"`
	Digest   core.Digest   `json:"digest"`
	Revision core.Revision `json:"revision"`
}

type ExecutionScopeBindingRefV2 struct {
	Ref      string        `json:"ref"`
	Digest   core.Digest   `json:"digest"`
	Revision core.Revision `json:"revision"`
}

type CredentialLeaseRefV2 struct {
	Ref         string      `json:"ref"`
	Class       string      `json:"class"`
	ScopeDigest core.Digest `json:"scope_digest"`
	Epoch       core.Epoch  `json:"epoch"`
}

type IdempotencyBindingV2 struct {
	Key         string                   `json:"key"`
	ScopeClass  EffectStableScopeClassV2 `json:"scope_class"`
	ScopeDigest core.Digest              `json:"scope_digest"`
	Class       core.IdempotencyClass    `json:"class"`
}

type EffectStableScopeClassV2 string

const EffectStableScopeTenantV2 EffectStableScopeClassV2 = "tenant"

type ConflictDomainBindingV2 struct {
	Domain      NamespacedNameV2         `json:"domain"`
	ScopeClass  EffectStableScopeClassV2 `json:"scope_class"`
	ScopeDigest core.Digest              `json:"scope_digest"`
}

type EffectRelationV2 struct {
	CompensatesEffectID       core.EffectIntentID `json:"compensates_effect_id,omitempty"`
	CompensatesEffectRevision core.Revision       `json:"compensates_effect_revision,omitempty"`
	InspectsEffectID          core.EffectIntentID `json:"inspects_effect_id,omitempty"`
	InspectsEffectRevision    core.Revision       `json:"inspects_effect_revision,omitempty"`
	CleansUpEffectID          core.EffectIntentID `json:"cleans_up_effect_id,omitempty"`
	CleansUpEffectRevision    core.Revision       `json:"cleans_up_effect_revision,omitempty"`
	ReviewsCaseID             string              `json:"reviews_case_id,omitempty"`
	ReviewsCandidateRevision  core.Revision       `json:"reviews_candidate_revision,omitempty"`
}

// EffectIntentV2 is a command/intent envelope, not evidence that any external
// action happened. It contains only governance fields plus a bounded opaque
// payload whose business meaning remains component-owned.
type EffectIntentV2 struct {
	ContractVersion        string                     `json:"contract_version"`
	ID                     core.EffectIntentID        `json:"effect_intent_id"`
	Revision               core.Revision              `json:"effect_intent_revision"`
	Scope                  core.ExecutionScope        `json:"scope"`
	RunID                  core.AgentRunID            `json:"run_id"`
	Kind                   EffectKindV2               `json:"effect_kind"`
	RiskClass              NamespacedNameV2           `json:"risk_class"`
	ActionScopeDigest      core.Digest                `json:"action_scope_digest"`
	Payload                OpaquePayloadV2            `json:"payload"`
	PayloadRevision        core.Revision              `json:"payload_revision"`
	Target                 string                     `json:"target"`
	ConflictDomain         ConflictDomainBindingV2    `json:"conflict_domain"`
	Owners                 []EffectOwnerRefV2         `json:"owners"`
	Provider               ProviderBindingRefV2       `json:"provider_binding"`
	Authority              AuthorityBindingRefV2      `json:"authority_binding"`
	Review                 ReviewBindingRefV2         `json:"review_binding"`
	Budget                 BudgetBindingRefV2         `json:"budget_binding"`
	Policy                 DispatchPolicyBindingRefV2 `json:"dispatch_policy_binding"`
	CurrentScope           ExecutionScopeBindingRefV2 `json:"current_scope_binding"`
	Idempotency            IdempotencyBindingV2       `json:"idempotency"`
	CredentialLeases       []CredentialLeaseRefV2     `json:"credential_leases"`
	Relation               EffectRelationV2           `json:"relation"`
	MayLeaveRemoteResidual bool                       `json:"may_leave_remote_residual"`
	RequiresCleanup        bool                       `json:"requires_cleanup"`
	ExpiresUnixNano        int64                      `json:"expires_unix_nano"`
}

func (i EffectIntentV2) Validate() error {
	return i.validateV2(false)
}

func (i EffectIntentV2) validateV2(allowPendingPolicyDigest bool) error {
	if i.ContractVersion != EffectContractVersionV2 || strings.TrimSpace(string(i.ID)) == "" || i.Revision == 0 || strings.TrimSpace(string(i.RunID)) == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEffectIntentMissing, "effect v2 contract, id, revision and run are required")
	}
	if err := i.Scope.Validate(); err != nil {
		return err
	}
	if err := ValidateNamespacedNameV2(NamespacedNameV2(i.Kind)); err != nil {
		return err
	}
	if err := ValidateNamespacedNameV2(i.RiskClass); err != nil {
		return err
	}
	if i.PayloadRevision == 0 || i.ExpiresUnixNano <= 0 || strings.TrimSpace(i.Target) == "" || len(i.Target) > 512 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "effect target, conflict domain, payload revision and expiry are required and bounded")
	}
	if err := i.ConflictDomain.ValidateForScope(i.Scope); err != nil {
		return err
	}
	if err := i.ActionScopeDigest.Validate(); err != nil {
		return err
	}
	if err := i.Payload.Validate(); err != nil {
		return err
	}
	if err := validateEffectOwnersV2(i.Owners); err != nil {
		return err
	}
	if err := i.Provider.Validate(); err != nil {
		return err
	}
	if err := i.Authority.Validate(); err != nil {
		return err
	}
	if err := i.Review.Validate(); err != nil {
		return err
	}
	if err := i.Budget.Validate(); err != nil {
		return err
	}
	if allowPendingPolicyDigest && i.Policy.Digest == "" {
		if strings.TrimSpace(i.Policy.Ref) == "" || i.Policy.Revision == 0 {
			return core.NewError(core.ErrorForbidden, core.ReasonEffectAuthorizationMissing, "dispatch policy ref and revision must be preallocated")
		}
	} else if err := i.Policy.Validate(); err != nil {
		return err
	}
	if err := i.CurrentScope.Validate(); err != nil {
		return err
	}
	if err := i.Idempotency.Validate(); err != nil {
		return err
	}
	if i.Idempotency.ScopeDigest != StableTenantScopeDigestV2(i.Scope.Identity.TenantID) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "idempotency scope must remain stable across run, instance and restore boundaries")
	}
	if len(i.CredentialLeases) > 64 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "effect credential lease set exceeds its bound")
	}
	seen := make(map[string]struct{}, len(i.CredentialLeases))
	var previous string
	for index, credential := range i.CredentialLeases {
		if err := credential.Validate(); err != nil {
			return err
		}
		key := credential.Class + "\x00" + credential.Ref
		if _, exists := seen[key]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "effect credential lease set contains a duplicate")
		}
		if index > 0 && key < previous {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "effect credential lease set must be sorted")
		}
		seen[key] = struct{}{}
		previous = key
	}
	return i.Relation.Validate(i.ID, i.Revision)
}

func (i EffectIntentV2) DigestV2() (core.Digest, error) {
	if err := i.Validate(); err != nil {
		return "", err
	}
	if i.Owners == nil {
		i.Owners = []EffectOwnerRefV2{}
	}
	if i.CredentialLeases == nil {
		i.CredentialLeases = []CredentialLeaseRefV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.effect", EffectContractVersionV2, "EffectIntentV2", i)
}

// PolicyCandidateDigestV2 excludes the policy fact binding itself to avoid a
// circular digest while still binding policy to every governed effect field.
func (i EffectIntentV2) PolicyCandidateDigestV2() (core.Digest, error) {
	if err := i.validateV2(true); err != nil {
		return "", err
	}
	i.Policy = DispatchPolicyBindingRefV2{}
	// The policy binds the stable Review case locator and policy digest, not a
	// later candidate/verdict content digest. This avoids cross-fact cycles.
	i.Review.Digest = ""
	if i.Owners == nil {
		i.Owners = []EffectOwnerRefV2{}
	}
	if i.CredentialLeases == nil {
		i.CredentialLeases = []CredentialLeaseRefV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.effect", EffectContractVersionV2, "EffectPolicyCandidateV2", i)
}

func (r ProviderBindingRefV2) Validate() error {
	if strings.TrimSpace(r.BindingSetID) == "" || r.BindingSetRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonProviderBindingStale, "provider binding set and revision are required")
	}
	if err := ValidateNamespacedNameV2(NamespacedNameV2(r.ComponentID)); err != nil {
		return err
	}
	if err := ValidateNamespacedNameV2(NamespacedNameV2(r.Capability)); err != nil {
		return err
	}
	if err := r.ManifestDigest.Validate(); err != nil {
		return err
	}
	return r.ArtifactDigest.Validate()
}

func (r AuthorityBindingRefV2) Validate() error {
	if strings.TrimSpace(r.Ref) == "" || r.Revision == 0 || r.Epoch == 0 {
		return core.NewError(core.ErrorForbidden, core.ReasonEffectAuthorizationMissing, "authority binding ref, revision and epoch are required")
	}
	return r.Digest.Validate()
}

func (r ReviewBindingRefV2) Validate() error {
	if strings.TrimSpace(r.Ref) == "" || r.Revision == 0 {
		return core.NewError(core.ErrorForbidden, core.ReasonReviewVerdictMissing, "review fact ref and revision are required, including explicit not-required policy")
	}
	if err := r.Digest.Validate(); err != nil {
		return err
	}
	return r.PolicyDigest.Validate()
}

func (r BudgetBindingRefV2) Validate() error {
	if strings.TrimSpace(r.Ref) == "" || r.Revision == 0 {
		return core.NewError(core.ErrorForbidden, core.ReasonBudgetBindingMissing, "budget fact ref and revision are required, including explicit not-required policy")
	}
	if err := r.Digest.Validate(); err != nil {
		return err
	}
	return r.PolicyDigest.Validate()
}

func (r DispatchPolicyBindingRefV2) Validate() error {
	if strings.TrimSpace(r.Ref) == "" || r.Revision == 0 {
		return core.NewError(core.ErrorForbidden, core.ReasonEffectAuthorizationMissing, "dispatch policy fact ref and revision are required")
	}
	return r.Digest.Validate()
}

func (r ExecutionScopeBindingRefV2) Validate() error {
	if strings.TrimSpace(r.Ref) == "" || r.Revision == 0 {
		return core.NewError(core.ErrorForbidden, core.ReasonEffectFenceStale, "current execution scope fact ref and revision are required")
	}
	return r.Digest.Validate()
}

func (r CredentialLeaseRefV2) Validate() error {
	if strings.TrimSpace(r.Ref) == "" || strings.TrimSpace(r.Class) == "" || r.Epoch == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCredentialLeaseMissing, "credential lease ref, class and epoch are required")
	}
	return r.ScopeDigest.Validate()
}

func (i IdempotencyBindingV2) Validate() error {
	if strings.TrimSpace(i.Key) == "" || len(i.Key) > 256 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "bounded effect idempotency key is required")
	}
	if err := i.ScopeDigest.Validate(); err != nil {
		return err
	}
	if i.ScopeClass != EffectStableScopeTenantV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "P0.2 permits only the conservative tenant-stable idempotency scope")
	}
	switch i.Class {
	case core.IdempotencyProviderKey, core.IdempotencyQueryable, core.IdempotencyNonRetryable:
		return nil
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "effect idempotency class is required")
	}
}

func StableTenantScopeDigestV2(tenantID core.TenantID) core.Digest {
	return core.DigestBytes([]byte("praxis.runtime.effect/v2\x00tenant\x00" + string(tenantID)))
}

func (b ConflictDomainBindingV2) ValidateForScope(scope core.ExecutionScope) error {
	if err := ValidateNamespacedNameV2(b.Domain); err != nil {
		return err
	}
	if b.ScopeClass != EffectStableScopeTenantV2 || b.ScopeDigest != StableTenantScopeDigestV2(scope.Identity.TenantID) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectConflictDomainOccupied, "conflict domain must use the tenant-stable scope and cannot narrow itself to a run or instance")
	}
	return nil
}

func (r EffectRelationV2) Validate(selfID core.EffectIntentID, selfRevision core.Revision) error {
	compensation := strings.TrimSpace(string(r.CompensatesEffectID)) != "" || r.CompensatesEffectRevision != 0
	inspection := strings.TrimSpace(string(r.InspectsEffectID)) != "" || r.InspectsEffectRevision != 0
	cleanup := strings.TrimSpace(string(r.CleansUpEffectID)) != "" || r.CleansUpEffectRevision != 0
	review := strings.TrimSpace(r.ReviewsCaseID) != "" || r.ReviewsCandidateRevision != 0
	count := 0
	for _, present := range []bool{compensation, inspection, cleanup, review} {
		if present {
			count++
		}
	}
	if count > 1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "effect relation must have at most one original-effect purpose")
	}
	if compensation && (strings.TrimSpace(string(r.CompensatesEffectID)) == "" || r.CompensatesEffectRevision == 0 || r.CompensatesEffectID == selfID && r.CompensatesEffectRevision == selfRevision) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCompensationIncomplete, "compensation must bind a different original effect revision")
	}
	if inspection && (strings.TrimSpace(string(r.InspectsEffectID)) == "" || r.InspectsEffectRevision == 0 || r.InspectsEffectID == selfID && r.InspectsEffectRevision == selfRevision) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEffectUnknownOutcome, "remote inspection must bind a different original effect revision")
	}
	if cleanup && (strings.TrimSpace(string(r.CleansUpEffectID)) == "" || r.CleansUpEffectRevision == 0 || r.CleansUpEffectID == selfID && r.CleansUpEffectRevision == selfRevision) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCleanupEvidenceIncomplete, "cleanup effect must bind a different original effect revision")
	}
	if review && (strings.TrimSpace(r.ReviewsCaseID) == "" || r.ReviewsCandidateRevision == 0) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewRemoteEffectRequired, "review invocation Effect must bind its exact review candidate revision")
	}
	return nil
}

type DispatchPermitV2 struct {
	ContractVersion            string                     `json:"contract_version"`
	ID                         string                     `json:"permit_id"`
	Revision                   core.Revision              `json:"permit_revision"`
	AttemptID                  string                     `json:"attempt_id"`
	IntentID                   core.EffectIntentID        `json:"intent_id"`
	IntentRevision             core.Revision              `json:"intent_revision"`
	IntentDigest               core.Digest                `json:"intent_digest"`
	PayloadSchema              SchemaRefV2                `json:"payload_schema"`
	PayloadDigest              core.Digest                `json:"payload_digest"`
	PayloadRevision            core.Revision              `json:"payload_revision"`
	Scope                      core.ExecutionScope        `json:"scope"`
	RunID                      core.AgentRunID            `json:"run_id"`
	ConflictDomain             ConflictDomainBindingV2    `json:"conflict_domain"`
	Provider                   ProviderBindingRefV2       `json:"provider_binding"`
	EnforcementPoint           ProviderBindingRefV2       `json:"enforcement_point_binding"`
	Authority                  AuthorityBindingRefV2      `json:"authority_binding"`
	Review                     ReviewBindingRefV2         `json:"review_binding"`
	ReviewVerdictDigest        core.Digest                `json:"review_verdict_digest"`
	ReviewVerdictRevision      core.Revision              `json:"review_verdict_revision"`
	ReviewSatisfactionRef      string                     `json:"review_satisfaction_ref,omitempty"`
	ReviewSatisfactionDigest   core.Digest                `json:"review_satisfaction_digest,omitempty"`
	ReviewSatisfactionRevision core.Revision              `json:"review_satisfaction_revision,omitempty"`
	Budget                     BudgetBindingRefV2         `json:"budget_binding"`
	Policy                     DispatchPolicyBindingRefV2 `json:"dispatch_policy_binding"`
	CurrentScope               ExecutionScopeBindingRefV2 `json:"current_scope_binding"`
	CapabilityGrantDigest      core.Digest                `json:"capability_grant_digest"`
	CredentialGrantDigest      core.Digest                `json:"credential_grant_digest"`
	FenceDigest                core.Digest                `json:"fence_digest"`
	Idempotency                IdempotencyBindingV2       `json:"idempotency"`
	IssuedUnixNano             int64                      `json:"issued_unix_nano"`
	ExpiresUnixNano            int64                      `json:"expires_unix_nano"`
}

func (p DispatchPermitV2) Validate() error {
	if p.ContractVersion != EffectContractVersionV2 || strings.TrimSpace(p.ID) == "" || p.Revision == 0 || strings.TrimSpace(p.AttemptID) == "" || strings.TrimSpace(string(p.IntentID)) == "" || p.IntentRevision == 0 || p.PayloadRevision == 0 || strings.TrimSpace(string(p.RunID)) == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "permit id, revision, single attempt, intent, payload, run and conflict domain are required")
	}
	if err := p.Scope.Validate(); err != nil {
		return err
	}
	if err := p.ConflictDomain.ValidateForScope(p.Scope); err != nil {
		return err
	}
	if err := p.PayloadSchema.Validate(); err != nil {
		return err
	}
	for _, digest := range []core.Digest{p.IntentDigest, p.PayloadDigest, p.ReviewVerdictDigest, p.CapabilityGrantDigest, p.CredentialGrantDigest, p.FenceDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if err := p.Provider.Validate(); err != nil {
		return err
	}
	if err := p.EnforcementPoint.Validate(); err != nil {
		return err
	}
	// P0.2 deliberately supports only provider self-verification. A separately
	// delegated verifier requires a future versioned binding contract.
	if p.EnforcementPoint != p.Provider {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "P0.2 enforcement point must be the exact bound provider")
	}
	if err := p.Authority.Validate(); err != nil {
		return err
	}
	if err := p.Review.Validate(); err != nil {
		return err
	}
	if p.ReviewVerdictRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictMissing, "dispatch permit requires the current verdict revision")
	}
	satisfaction := strings.TrimSpace(p.ReviewSatisfactionRef) != "" || p.ReviewSatisfactionDigest != "" || p.ReviewSatisfactionRevision != 0
	if satisfaction && (strings.TrimSpace(p.ReviewSatisfactionRef) == "" || p.ReviewSatisfactionDigest.Validate() != nil || p.ReviewSatisfactionRevision == 0) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewConditionUnsatisfied, "conditional permit must bind one exact satisfaction fact")
	}
	if err := p.Budget.Validate(); err != nil {
		return err
	}
	if err := p.Policy.Validate(); err != nil {
		return err
	}
	if err := p.CurrentScope.Validate(); err != nil {
		return err
	}
	if err := p.Idempotency.Validate(); err != nil {
		return err
	}
	if p.IssuedUnixNano <= 0 || p.ExpiresUnixNano <= p.IssuedUnixNano || time.Duration(p.ExpiresUnixNano-p.IssuedUnixNano) > MaxDispatchPermitTTL {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "dispatch permit requires a positive bounded TTL")
	}
	return nil
}

func (p DispatchPermitV2) DigestV2() (core.Digest, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.runtime.effect", EffectContractVersionV2, "DispatchPermitV2", p)
}

func DigestExecutionFenceV2(fence core.ExecutionFence) (core.Digest, error) {
	if err := fence.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.runtime.effect", EffectContractVersionV2, "ExecutionFenceV2Binding", fence)
}

type DispatchCurrentFactsV2 struct {
	Scope                      core.ExecutionScope        `json:"scope"`
	CapabilityGrantDigest      core.Digest                `json:"capability_grant_digest"`
	CredentialGrantDigest      core.Digest                `json:"credential_grant_digest"`
	Provider                   ProviderBindingRefV2       `json:"provider_binding"`
	EnforcementPoint           ProviderBindingRefV2       `json:"enforcement_point_binding"`
	Authority                  AuthorityBindingRefV2      `json:"authority_binding"`
	Review                     ReviewBindingRefV2         `json:"review_binding"`
	ReviewVerdictDigest        core.Digest                `json:"review_verdict_digest"`
	ReviewVerdictRevision      core.Revision              `json:"review_verdict_revision"`
	ReviewSatisfactionRef      string                     `json:"review_satisfaction_ref,omitempty"`
	ReviewSatisfactionDigest   core.Digest                `json:"review_satisfaction_digest,omitempty"`
	ReviewSatisfactionRevision core.Revision              `json:"review_satisfaction_revision,omitempty"`
	Budget                     BudgetBindingRefV2         `json:"budget_binding"`
	Policy                     DispatchPolicyBindingRefV2 `json:"dispatch_policy_binding"`
	CurrentScope               ExecutionScopeBindingRefV2 `json:"current_scope_binding"`
	FenceDigest                core.Digest                `json:"fence_digest"`
}

// ValidateDispatchAtExecutionPointV2 is the second gate. The actual execution
// point must independently inspect current facts; a permit bearer alone is not
// sufficient authority.
func ValidateDispatchAtExecutionPointV2(permit DispatchPermitV2, intent EffectIntentV2, fence core.ExecutionFence, current DispatchCurrentFactsV2, now time.Time) error {
	if err := permit.Validate(); err != nil {
		return err
	}
	if err := intent.Validate(); err != nil {
		return err
	}
	intentDigest, err := intent.DigestV2()
	if err != nil {
		return err
	}
	if now.IsZero() || !now.Before(time.Unix(0, permit.ExpiresUnixNano)) || !now.Before(time.Unix(0, intent.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitExpired, "dispatch permit or intent expired")
	}
	if permit.IntentID != intent.ID || permit.IntentRevision != intent.Revision || permit.IntentDigest != intentDigest || permit.PayloadSchema != intent.Payload.Schema || permit.PayloadDigest != intent.Payload.ContentDigest || permit.PayloadRevision != intent.PayloadRevision || permit.RunID != intent.RunID || permit.ConflictDomain != intent.ConflictDomain || permit.Provider != intent.Provider || permit.EnforcementPoint != intent.Provider || permit.Authority != intent.Authority || permit.Review != intent.Review || permit.Budget != intent.Budget || permit.Policy != intent.Policy || permit.CurrentScope != intent.CurrentScope || permit.Idempotency != intent.Idempotency {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "permit drifted from its exact effect intent")
	}
	if current.Provider != permit.Provider || current.EnforcementPoint != permit.EnforcementPoint || current.Authority != permit.Authority || current.Review != permit.Review || current.ReviewVerdictDigest != permit.ReviewVerdictDigest || current.ReviewVerdictRevision != permit.ReviewVerdictRevision || current.ReviewSatisfactionRef != permit.ReviewSatisfactionRef || current.ReviewSatisfactionDigest != permit.ReviewSatisfactionDigest || current.ReviewSatisfactionRevision != permit.ReviewSatisfactionRevision || current.Budget != permit.Budget || current.Policy != permit.Policy || current.CurrentScope != permit.CurrentScope || current.CapabilityGrantDigest != permit.CapabilityGrantDigest || current.CredentialGrantDigest != permit.CredentialGrantDigest || current.FenceDigest != permit.FenceDigest || !SameExecutionScopeV2(current.Scope, permit.Scope) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "execution point facts drifted after permit issuance")
	}
	fenceDigest, err := DigestExecutionFenceV2(fence)
	if err != nil || fenceDigest != permit.FenceDigest || fence.EffectIntentID != intent.ID || fence.EffectIntentRevision != intent.Revision || fence.CanonicalPayloadDigest != intent.Payload.ContentDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "execution point fence drifted from permit and intent")
	}
	return core.CheckFence(fence, core.CurrentFenceFacts{Scope: current.Scope, CapabilityGrantDigest: current.CapabilityGrantDigest}, now)
}

type EnforcementReceiptV2 struct {
	ContractVersion string                 `json:"contract_version"`
	PermitID        string                 `json:"permit_id"`
	PermitRevision  core.Revision          `json:"permit_revision"`
	AttemptID       string                 `json:"attempt_id"`
	PermitDigest    core.Digest            `json:"permit_digest"`
	Verifier        ProviderBindingRefV2   `json:"verifier_binding"`
	ValidatedAt     int64                  `json:"validated_unix_nano"`
	Attestation     *GovernanceExtensionV2 `json:"attestation,omitempty"`
}

func (r EnforcementReceiptV2) Validate() error {
	if r.ContractVersion != EffectContractVersionV2 || strings.TrimSpace(r.PermitID) == "" || r.PermitRevision == 0 || strings.TrimSpace(r.AttemptID) == "" || r.ValidatedAt <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "enforcement receipt identity and validation time are required")
	}
	if err := r.PermitDigest.Validate(); err != nil {
		return err
	}
	if err := r.Verifier.Validate(); err != nil {
		return err
	}
	if r.Attestation != nil {
		return validateExtensionsV2([]GovernanceExtensionV2{*r.Attestation})
	}
	return nil
}

func validateEffectOwnersV2(owners []EffectOwnerRefV2) error {
	if len(owners) != 3 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonOwnerMissing, "effect, settlement and cleanup each require exactly one binding owner")
	}
	seen := make(map[OwnerRoleV2]struct{}, 3)
	for _, owner := range owners {
		if owner.Role != OwnerEffect && owner.Role != OwnerSettlement && owner.Role != OwnerCleanup {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonOwnerConflict, "effect owner role is unknown")
		}
		if _, exists := seen[owner.Role]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "effect owner role has multiple owners")
		}
		if err := ValidateNamespacedNameV2(NamespacedNameV2(owner.ComponentID)); err != nil {
			return err
		}
		if err := owner.ManifestDigest.Validate(); err != nil {
			return err
		}
		seen[owner.Role] = struct{}{}
	}
	for index := 1; index < len(owners); index++ {
		if owners[index].Role < owners[index-1].Role {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "effect owner set must be sorted by role")
		}
	}
	return nil
}

// SameExecutionScopeV2 compares an execution scope by governed value. In
// particular, SandboxLease pointers are references to values, not pointer
// identities, so persistence round-trips cannot invalidate an equal scope.
func SameExecutionScopeV2(left, right core.ExecutionScope) bool {
	if left.Identity != right.Identity || left.Lineage != right.Lineage || left.Instance != right.Instance || left.AuthorityEpoch != right.AuthorityEpoch {
		return false
	}
	if left.SandboxLease == nil || right.SandboxLease == nil {
		return left.SandboxLease == nil && right.SandboxLease == nil
	}
	return *left.SandboxLease == *right.SandboxLease
}

package core

import "time"

type ValidationMode string

const (
	ValidationOnlineStrict  ValidationMode = "online_strict"
	ValidationLeasedOffline ValidationMode = "leased_offline"
)

type RevocationPolicy struct {
	RiskClass            string         `json:"risk_class"`
	ValidationMode       ValidationMode `json:"validation_mode"`
	MaxRevocationLag     time.Duration  `json:"max_revocation_lag"`
	MaxClockSkew         time.Duration  `json:"max_clock_skew"`
	TokenScope           string         `json:"token_scope"`
	ConflictEffectDomain string         `json:"conflict_effect_domain"`
}

func (p RevocationPolicy) Validate() error {
	if blank(p.RiskClass) || blank(p.ConflictEffectDomain) {
		return NewError(ErrorInvalidArgument, ReasonInvalidReference, "risk class and conflict effect domain are required")
	}
	switch p.ValidationMode {
	case ValidationOnlineStrict:
		return nil
	case ValidationLeasedOffline:
		if p.MaxRevocationLag <= 0 || p.MaxClockSkew <= 0 || blank(p.TokenScope) {
			return NewError(ErrorPreconditionFailed, ReasonOfflineRevocationPolicyMissing, "offline effects require explicit lag, clock skew and token scope")
		}
		return nil
	default:
		return NewError(ErrorInvalidArgument, ReasonOfflineRevocationPolicyMissing, "validation mode is required")
	}
}

type ExecutionFence struct {
	BoundaryScope          FenceBoundaryScope `json:"boundary_scope"`
	Scope                  ExecutionScope     `json:"scope"`
	CapabilityGrantDigest  Digest             `json:"capability_grant_digest"`
	EffectIntentID         EffectIntentID     `json:"effect_intent_id"`
	EffectIntentRevision   Revision           `json:"effect_intent_revision"`
	CanonicalPayloadDigest Digest             `json:"canonical_payload_digest"`
	ExpiresAt              time.Time          `json:"expires_at"`
}

type FenceBoundaryScope string

const (
	FenceBoundaryActivation FenceBoundaryScope = "activation"
	FenceBoundaryInstance   FenceBoundaryScope = "instance"
)

func (f ExecutionFence) Validate() error {
	if err := f.Scope.Validate(); err != nil {
		return err
	}
	if f.BoundaryScope != FenceBoundaryActivation && f.BoundaryScope != FenceBoundaryInstance {
		return NewError(ErrorInvalidArgument, ReasonInvalidReference, "fence boundary scope is required")
	}
	if f.BoundaryScope == FenceBoundaryInstance && f.Scope.SandboxLease == nil {
		return NewError(ErrorInvalidArgument, ReasonInvalidReference, "effect fence requires an active sandbox lease reference")
	}
	if err := f.CapabilityGrantDigest.Validate(); err != nil {
		return err
	}
	if blank(string(f.EffectIntentID)) || f.EffectIntentRevision == 0 {
		return NewError(ErrorInvalidArgument, ReasonEffectIntentMissing, "effect intent id and revision are required")
	}
	if err := f.CanonicalPayloadDigest.Validate(); err != nil {
		return err
	}
	if f.ExpiresAt.IsZero() {
		return NewError(ErrorInvalidArgument, ReasonEffectFenceStale, "fence expiry is required")
	}
	return nil
}

type CurrentFenceFacts struct {
	Scope                 ExecutionScope
	CapabilityGrantDigest Digest
}

func CheckFence(fence ExecutionFence, current CurrentFenceFacts, now time.Time) error {
	if err := fence.Validate(); err != nil {
		return err
	}
	if err := current.Scope.Validate(); err != nil {
		return err
	}
	if now.IsZero() || !now.Before(fence.ExpiresAt) {
		return NewError(ErrorPreconditionFailed, ReasonEffectFenceStale, "fence has expired")
	}
	if fence.BoundaryScope == FenceBoundaryInstance && current.Scope.SandboxLease == nil {
		return NewError(ErrorPreconditionFailed, ReasonEffectFenceStale, "current sandbox lease is missing")
	}
	if fence.Scope.Identity != current.Scope.Identity || fence.Scope.Instance != current.Scope.Instance ||
		fence.Scope.AuthorityEpoch != current.Scope.AuthorityEpoch || !sameOptionalLease(fence.Scope.SandboxLease, current.Scope.SandboxLease) ||
		fence.Scope.Lineage.ID != current.Scope.Lineage.ID || fence.Scope.Lineage.PlanDigest != current.Scope.Lineage.PlanDigest ||
		fence.CapabilityGrantDigest != current.CapabilityGrantDigest {
		return NewError(ErrorPreconditionFailed, ReasonEffectFenceStale, "fence does not match current authority facts")
	}
	return nil
}

func sameOptionalLease(left, right *SandboxLeaseRef) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

type ReplacementPermit struct {
	OldInstance               InstanceRef `json:"old_instance"`
	NewInstance               InstanceRef `json:"new_instance"`
	ConflictEffectDomain      string      `json:"conflict_effect_domain"`
	FenceIsolationConfirmed   bool        `json:"fence_isolation_confirmed"`
	NetworkIsolationConfirmed bool        `json:"network_isolation_confirmed"`
	SecretPathRevoked         bool        `json:"secret_path_revoked"`
	RemoteDomainIsolated      bool        `json:"remote_domain_isolated"`
	IssuedAt                  time.Time   `json:"issued_at"`
}

func (p ReplacementPermit) Validate() error {
	if err := p.OldInstance.Validate(); err != nil {
		return err
	}
	if err := p.NewInstance.Validate(); err != nil {
		return err
	}
	if p.NewInstance.Epoch <= p.OldInstance.Epoch || p.NewInstance.ID == p.OldInstance.ID || blank(p.ConflictEffectDomain) {
		return NewError(ErrorPreconditionFailed, ReasonInvalidReference, "replacement requires a new id, higher epoch and conflict domain")
	}
	if !p.FenceIsolationConfirmed || !p.NetworkIsolationConfirmed || !p.SecretPathRevoked || !p.RemoteDomainIsolated || p.IssuedAt.IsZero() {
		return NewError(ErrorPreconditionFailed, ReasonFencedInstance, "replacement isolation evidence is incomplete")
	}
	return nil
}

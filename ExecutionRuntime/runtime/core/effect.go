package core

import "time"

type EffectKind string

const (
	EffectKindDataDisclosure       EffectKind = "data_disclosure"
	EffectKindCostConsumption      EffectKind = "cost_consumption"
	EffectKindResourceLifecycle    EffectKind = "resource_lifecycle"
	EffectKindExternalMutation     EffectKind = "external_mutation"
	EffectKindProviderContinuation EffectKind = "provider_continuation"
	EffectKindHostedExecution      EffectKind = "hosted_execution"
	EffectKindCacheState           EffectKind = "cache_state"
	EffectKindFormalCommit         EffectKind = "formal_commit"
	EffectKindCredentialOperation  EffectKind = "credential_operation"
	EffectKindSafetyControl        EffectKind = "safety_control"
)

type IdempotencyClass string

const (
	IdempotencyProviderKey  IdempotencyClass = "provider_key"
	IdempotencyQueryable    IdempotencyClass = "queryable"
	IdempotencyNonRetryable IdempotencyClass = "non_retryable"
)

type EffectIntent struct {
	ID                     EffectIntentID   `json:"effect_intent_id"`
	Revision               Revision         `json:"effect_intent_revision"`
	Kind                   EffectKind       `json:"effect_kind"`
	RiskClass              string           `json:"risk_class"`
	CanonicalPayloadDigest Digest           `json:"canonical_payload_digest"`
	Target                 string           `json:"target"`
	ConflictEffectDomain   string           `json:"conflict_effect_domain"`
	Ownership              EffectOwnership  `json:"ownership"`
	AuthorizationRef       string           `json:"authorization_ref"`
	BudgetReservationRef   string           `json:"budget_reservation_ref,omitempty"`
	IdempotencyClass       IdempotencyClass `json:"idempotency_class"`
	PersistedAt            time.Time        `json:"persisted_at"`
}

func (i EffectIntent) Validate() error {
	if blank(string(i.ID)) || i.Revision == 0 {
		return NewError(ErrorInvalidArgument, ReasonEffectIntentMissing, "effect intent id and revision are required")
	}
	if !validEffectKind(i.Kind) || blank(i.RiskClass) || blank(i.Target) || blank(i.ConflictEffectDomain) {
		return NewError(ErrorInvalidArgument, ReasonInvalidReference, "effect kind, risk, target and conflict domain are required")
	}
	if err := i.CanonicalPayloadDigest.Validate(); err != nil {
		return err
	}
	if err := i.Ownership.Validate(); err != nil {
		return err
	}
	if blank(i.AuthorizationRef) {
		return NewError(ErrorForbidden, ReasonEffectAuthorizationMissing, "effect authorization is required")
	}
	if i.PersistedAt.IsZero() {
		return NewError(ErrorUnavailable, ReasonEvidenceUnavailable, "effect intent must be durably persisted before dispatch")
	}
	switch i.IdempotencyClass {
	case IdempotencyProviderKey, IdempotencyQueryable, IdempotencyNonRetryable:
		return nil
	default:
		return NewError(ErrorInvalidArgument, ReasonInvalidReference, "idempotency class is required")
	}
}

func ValidateEffectDispatch(intent EffectIntent, fence ExecutionFence, current CurrentFenceFacts, now time.Time) error {
	if err := intent.Validate(); err != nil {
		return err
	}
	if fence.EffectIntentID != intent.ID || fence.EffectIntentRevision != intent.Revision || fence.CanonicalPayloadDigest != intent.CanonicalPayloadDigest {
		return NewError(ErrorPreconditionFailed, ReasonEffectFenceStale, "fence is not bound to the current intent revision and payload")
	}
	return CheckFence(fence, current, now)
}

func validEffectKind(kind EffectKind) bool {
	switch kind {
	case EffectKindDataDisclosure, EffectKindCostConsumption, EffectKindResourceLifecycle, EffectKindExternalMutation,
		EffectKindProviderContinuation, EffectKindHostedExecution, EffectKindCacheState, EffectKindFormalCommit,
		EffectKindCredentialOperation, EffectKindSafetyControl:
		return true
	default:
		return false
	}
}

type RecoveryEffectKind string

const (
	RecoveryInspect            RecoveryEffectKind = "inspect"
	RecoveryOriginalSettlement RecoveryEffectKind = "original_intent_settlement"
	RecoveryEmergencySafety    RecoveryEffectKind = "emergency_safety"
	RecoveryCleanupRelease     RecoveryEffectKind = "cleanup_release"
	RecoveryCompensation       RecoveryEffectKind = "compensation"
)

type RecoveryEffectRequest struct {
	Kind                    RecoveryEffectKind
	ExternalEffect          bool
	ExpandsAuthority        bool
	HasFreshIntent          bool
	HasCurrentFence         bool
	HasCurrentAuthority     bool
	HasApplicableBudget     bool
	HasWriteAheadEvidence   bool
	OriginalEffectConfirmed bool
	CompensationAuthorized  bool
}

func ValidateRecoveryEffect(certainty ExecutionCertainty, request RecoveryEffectRequest) error {
	if certainty == CertaintyConfirmed {
		return nil
	}
	if request.ExpandsAuthority {
		return NewError(ErrorForbidden, ReasonRecoveryEffectNotPermitted, "recovery effect cannot expand authority")
	}
	switch request.Kind {
	case RecoveryInspect, RecoveryOriginalSettlement:
		if !request.ExternalEffect || hasRecoveryGuards(request) {
			return nil
		}
	case RecoveryEmergencySafety:
		return nil
	case RecoveryCleanupRelease:
		if hasRecoveryGuards(request) {
			return nil
		}
	case RecoveryCompensation:
		if request.OriginalEffectConfirmed && request.CompensationAuthorized && hasRecoveryGuards(request) {
			return nil
		}
	}
	return NewError(ErrorForbidden, ReasonRecoveryEffectNotPermitted, "recovery effect does not satisfy the restricted whitelist")
}

func hasRecoveryGuards(request RecoveryEffectRequest) bool {
	return request.HasFreshIntent && request.HasCurrentFence && request.HasCurrentAuthority && request.HasApplicableBudget && request.HasWriteAheadEvidence
}

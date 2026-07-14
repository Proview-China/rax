package core

import (
	"errors"
	"fmt"
)

type ErrorCategory string

const (
	ErrorInvalidArgument       ErrorCategory = "invalid_argument"
	ErrorUnauthenticated       ErrorCategory = "unauthenticated"
	ErrorForbidden             ErrorCategory = "forbidden"
	ErrorNotFound              ErrorCategory = "not_found"
	ErrorConflict              ErrorCategory = "conflict"
	ErrorPreconditionFailed    ErrorCategory = "precondition_failed"
	ErrorCapabilityUnavailable ErrorCategory = "capability_unavailable"
	ErrorIndeterminate         ErrorCategory = "indeterminate"
	ErrorRateLimited           ErrorCategory = "rate_limited"
	ErrorUnavailable           ErrorCategory = "unavailable"
	ErrorInternal              ErrorCategory = "internal"
)

type ReasonCode string

const (
	ReasonInvalidReference                ReasonCode = "invalid_reference"
	ReasonInvalidDigest                   ReasonCode = "invalid_digest"
	ReasonInvalidState                    ReasonCode = "invalid_state"
	ReasonInvalidTransition               ReasonCode = "invalid_transition"
	ReasonTerminalInstance                ReasonCode = "terminal_instance"
	ReasonFencedInstance                  ReasonCode = "fenced_instance"
	ReasonInspectCoverageIncomplete       ReasonCode = "inspect_coverage_incomplete"
	ReasonCleanupEvidenceIncomplete       ReasonCode = "cleanup_evidence_incomplete"
	ReasonCleanupRetractionMissing        ReasonCode = "cleanup_completion_retraction_missing"
	ReasonCleanupRetryUnauthorized        ReasonCode = "cleanup_retry_unauthorized"
	ReasonStaleIdentityEpoch              ReasonCode = "stale_identity_epoch"
	ReasonStaleInstanceEpoch              ReasonCode = "stale_instance_epoch"
	ReasonStaleLeaseEpoch                 ReasonCode = "stale_lease_epoch"
	ReasonStaleAuthorityEpoch             ReasonCode = "stale_authority_epoch"
	ReasonRevisionConflict                ReasonCode = "revision_conflict"
	ReasonAlreadyExists                   ReasonCode = "already_exists"
	ReasonIdempotencyPayloadMismatch      ReasonCode = "idempotency_payload_mismatch"
	ReasonCommandDominated                ReasonCode = "command_dominated"
	ReasonIdentityLeaseConflict           ReasonCode = "identity_lease_conflict"
	ReasonIdentityLeaseStateInvalid       ReasonCode = "identity_lease_state_invalid"
	ReasonStaleLeaseRevision              ReasonCode = "stale_lease_revision"
	ReasonActivationFactDrift             ReasonCode = "activation_fact_drift"
	ReasonActivationAttemptConflict       ReasonCode = "activation_attempt_conflict"
	ReasonActivationQuarantineRequired    ReasonCode = "activation_quarantine_required"
	ReasonSupervisionPolicyMissing        ReasonCode = "supervision_policy_missing"
	ReasonSupervisionPolicyDrift          ReasonCode = "supervision_policy_drift"
	ReasonLateSupervisionSignal           ReasonCode = "late_supervision_signal"
	ReasonEffectIntentMissing             ReasonCode = "effect_intent_missing"
	ReasonEffectAuthorizationMissing      ReasonCode = "effect_authorization_missing"
	ReasonEffectFenceStale                ReasonCode = "effect_fence_stale"
	ReasonEvidenceUnavailable             ReasonCode = "evidence_unavailable"
	ReasonEvidenceConflict                ReasonCode = "evidence_conflict"
	ReasonEvidenceSourceMissing           ReasonCode = "evidence_source_missing"
	ReasonEvidenceSourceStale             ReasonCode = "evidence_source_stale"
	ReasonEvidenceSequenceGap             ReasonCode = "evidence_sequence_gap"
	ReasonEvidenceTrustInvalid            ReasonCode = "evidence_trust_invalid"
	ReasonEvidenceScopeConflict           ReasonCode = "evidence_scope_conflict"
	ReasonEvidenceCursorInvalid           ReasonCode = "evidence_cursor_invalid"
	ReasonEvidenceChainConflict           ReasonCode = "evidence_chain_conflict"
	ReasonOfflineRevocationPolicyMissing  ReasonCode = "offline_revocation_policy_missing"
	ReasonRecoveryEffectNotPermitted      ReasonCode = "recovery_effect_not_permitted"
	ReasonOwnerMissing                    ReasonCode = "owner_missing"
	ReasonPlanInvalid                     ReasonCode = "plan_invalid"
	ReasonComponentMissing                ReasonCode = "component_missing"
	ReasonComponentMismatch               ReasonCode = "component_mismatch"
	ReasonCapabilityExpired               ReasonCode = "capability_expired"
	ReasonDependencyCycle                 ReasonCode = "dependency_cycle"
	ReasonRunConflict                     ReasonCode = "run_conflict"
	ReasonRunClaimConflict                ReasonCode = "run_claim_conflict"
	ReasonRunClaimUnverified              ReasonCode = "run_claim_unverified"
	ReasonCheckpointInconsistent          ReasonCode = "checkpoint_inconsistent"
	ReasonCheckpointUnsupported           ReasonCode = "checkpoint_unsupported"
	ReasonRestoreIncompatible             ReasonCode = "restore_incompatible"
	ReasonReadyEvidenceIncomplete         ReasonCode = "ready_evidence_incomplete"
	ReasonInvalidCanonicalForm            ReasonCode = "invalid_canonical_form"
	ReasonCanonicalLimitExceeded          ReasonCode = "canonical_limit_exceeded"
	ReasonDuplicateCanonicalKey           ReasonCode = "duplicate_canonical_key"
	ReasonInvalidSemanticVersion          ReasonCode = "invalid_semantic_version"
	ReasonInvalidNamespace                ReasonCode = "invalid_namespace"
	ReasonUnknownGovernanceCategory       ReasonCode = "unknown_governance_category"
	ReasonUnknownCapability               ReasonCode = "unknown_capability"
	ReasonUnknownSchema                   ReasonCode = "unknown_schema"
	ReasonUnknownRequiredExtension        ReasonCode = "unknown_required_extension"
	ReasonBindingExpired                  ReasonCode = "binding_expired"
	ReasonBindingDrift                    ReasonCode = "binding_drift"
	ReasonBindingNotCertified             ReasonCode = "binding_not_certified"
	ReasonBindingSetConflict              ReasonCode = "binding_set_conflict"
	ReasonOwnerConflict                   ReasonCode = "owner_conflict"
	ReasonClockRegression                 ReasonCode = "clock_regression"
	ReasonReviewVerdictMissing            ReasonCode = "review_verdict_missing"
	ReasonReviewVerdictStale              ReasonCode = "review_verdict_stale"
	ReasonReviewCandidateConflict         ReasonCode = "review_candidate_conflict"
	ReasonReviewConditionUnsatisfied      ReasonCode = "review_condition_unsatisfied"
	ReasonReviewRemoteEffectRequired      ReasonCode = "review_remote_effect_required"
	ReasonBudgetBindingMissing            ReasonCode = "budget_binding_missing"
	ReasonBudgetBindingStale              ReasonCode = "budget_binding_stale"
	ReasonCredentialLeaseMissing          ReasonCode = "credential_lease_missing"
	ReasonEffectStateConflict             ReasonCode = "effect_state_conflict"
	ReasonEffectConflictDomainOccupied    ReasonCode = "effect_conflict_domain_occupied"
	ReasonEffectUnknownOutcome            ReasonCode = "effect_unknown_outcome"
	ReasonEffectSettlementMissing         ReasonCode = "effect_settlement_missing"
	ReasonSettlementOwnerMismatch         ReasonCode = "settlement_owner_mismatch"
	ReasonDispatchPermitInvalid           ReasonCode = "dispatch_permit_invalid"
	ReasonDispatchPermitExpired           ReasonCode = "dispatch_permit_expired"
	ReasonDispatchPermitConsumed          ReasonCode = "dispatch_permit_consumed"
	ReasonProviderBindingStale            ReasonCode = "provider_binding_stale"
	ReasonCompensationIncomplete          ReasonCode = "compensation_incomplete"
	ReasonRemoteResidualUnresolved        ReasonCode = "remote_residual_unresolved"
	ReasonRunSettlementPlanConflict       ReasonCode = "run_settlement_plan_conflict"
	ReasonRunSettlementRequirementInvalid ReasonCode = "run_settlement_requirement_invalid"
	ReasonRunEffectIndexConflict          ReasonCode = "run_effect_index_conflict"
	ReasonRunEffectSetFrozen              ReasonCode = "run_effect_set_frozen"
	ReasonRunSettlementClosureConflict    ReasonCode = "run_settlement_closure_conflict"
	ReasonRunSettlementParticipantMissing ReasonCode = "run_settlement_participant_missing"
	ReasonRunSettlementParticipantStale   ReasonCode = "run_settlement_participant_stale"
	ReasonExecutionInspectionInvalid      ReasonCode = "execution_inspection_invalid"
	ReasonRunSettlementBlocked            ReasonCode = "run_settlement_blocked"
	ReasonRunCompletionConflict           ReasonCode = "run_completion_conflict"
	ReasonRunCompletionAtomicityBroken    ReasonCode = "run_completion_atomicity_broken"
	ReasonTerminationProgressConflict     ReasonCode = "termination_progress_conflict"
	ReasonTerminationReportIncomplete     ReasonCode = "termination_report_incomplete"
)

// DomainError is transport-neutral. Message must not contain secrets or raw
// provider payloads; callers should branch on Category and Reason.
type DomainError struct {
	Category ErrorCategory
	Reason   ReasonCode
	Message  string
}

func (e *DomainError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Message == "" {
		return fmt.Sprintf("%s: %s", e.Category, e.Reason)
	}
	return fmt.Sprintf("%s: %s: %s", e.Category, e.Reason, e.Message)
}

func NewError(category ErrorCategory, reason ReasonCode, message string) error {
	return &DomainError{Category: category, Reason: reason, Message: message}
}

func HasReason(err error, reason ReasonCode) bool {
	var domainErr *DomainError
	return errors.As(err, &domainErr) && domainErr.Reason == reason
}

func HasCategory(err error, category ErrorCategory) bool {
	var domainErr *DomainError
	return errors.As(err, &domainErr) && domainErr.Category == category
}

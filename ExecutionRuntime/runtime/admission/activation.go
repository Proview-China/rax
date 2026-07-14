// Package admission defines the durable, component-neutral activation
// protocol. It records decisions and recovery state, but owns no Sandbox,
// Budget, Harness, or Provider implementation.
package admission

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type ActivationStage string

const (
	StageProposed              ActivationStage = "proposed"
	StagePreflighting          ActivationStage = "preflighting"
	StagePreflightPassed       ActivationStage = "preflight_passed"
	StageSnapshotFrozen        ActivationStage = "snapshot_frozen"
	StageIdentityLeaseReserved ActivationStage = "identity_lease_reserved"
	StageBudgetResolved        ActivationStage = "budget_resolved"
	StageSandboxReserved       ActivationStage = "sandbox_reserved"
	StageCommitted             ActivationStage = "committed"
	StageSandboxActive         ActivationStage = "sandbox_active"
)

type RecoveryState string

const (
	RecoveryNormal      RecoveryState = "normal"
	RecoveryAborting    RecoveryState = "aborting"
	RecoveryQuarantined RecoveryState = "quarantined"
	RecoveryAborted     RecoveryState = "aborted"
)

type OperationState string

const (
	OperationNotStarted          OperationState = "not_started"
	OperationNotRequired         OperationState = "not_required"
	OperationIntentRecorded      OperationState = "intent_recorded"
	OperationConfirmedApplied    OperationState = "confirmed_applied"
	OperationConfirmedNotApplied OperationState = "confirmed_not_applied"
	OperationUnknownOutcome      OperationState = "unknown_outcome"
	OperationReleased            OperationState = "released"
)

type ActivationOperation struct {
	State          OperationState      `json:"state"`
	IntentID       core.EffectIntentID `json:"effect_intent_id,omitempty"`
	Reference      string              `json:"reference,omitempty"`
	EvidenceDigest core.Digest         `json:"evidence_digest,omitempty"`
}

func (o ActivationOperation) Validate() error {
	switch o.State {
	case OperationNotStarted, OperationNotRequired:
		if o.IntentID != "" || o.Reference != "" || o.EvidenceDigest != "" {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "inactive activation operation cannot retain effect facts")
		}
		return nil
	case OperationIntentRecorded, OperationUnknownOutcome:
		if strings.TrimSpace(string(o.IntentID)) == "" {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonEffectIntentMissing, "recorded or unknown activation operation requires an intent id")
		}
		return nil
	case OperationConfirmedNotApplied:
		if strings.TrimSpace(string(o.IntentID)) == "" {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonEffectIntentMissing, "confirmed activation operation requires an intent id")
		}
		return o.EvidenceDigest.Validate()
	case OperationConfirmedApplied, OperationReleased:
		if strings.TrimSpace(string(o.IntentID)) == "" || strings.TrimSpace(o.Reference) == "" {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "applied activation operation requires intent and external reference")
		}
		return o.EvidenceDigest.Validate()
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "unknown activation operation state")
	}
}

// ActivationSnapshot freezes only pre-allocation facts. Concrete Identity,
// Budget, and Sandbox lease IDs deliberately cannot appear in this schema.
type ActivationSnapshot struct {
	Digest                   core.Digest `json:"digest"`
	AuthorityEpoch           core.Epoch  `json:"authority_epoch"`
	EntitlementDigest        core.Digest `json:"entitlement_digest"`
	RouteDigest              core.Digest `json:"route_digest"`
	CapabilityEvidenceDigest core.Digest `json:"capability_evidence_digest"`
	PolicyDigest             core.Digest `json:"policy_digest"`
	BudgetPolicyDigest       core.Digest `json:"budget_policy_digest"`
	SandboxRequirementDigest core.Digest `json:"sandbox_requirement_digest"`
	RequestedExecutableCap   uint64      `json:"requested_executable_cap"`
	BudgetUnit               string      `json:"budget_unit"`
	EvidenceExpiresAt        time.Time   `json:"evidence_expires_at"`
}

func (s ActivationSnapshot) Validate() error {
	for _, digest := range []core.Digest{s.Digest, s.EntitlementDigest, s.RouteDigest, s.CapabilityEvidenceDigest, s.PolicyDigest, s.BudgetPolicyDigest, s.SandboxRequirementDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if s.AuthorityEpoch == 0 || s.EvidenceExpiresAt.IsZero() {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonActivationFactDrift, "activation snapshot authority or evidence lifetime is invalid")
	}
	if (s.RequestedExecutableCap == 0) != (strings.TrimSpace(s.BudgetUnit) == "") {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "budget cap and unit must either both be set or both be absent")
	}
	return nil
}

func (s ActivationSnapshot) ValidateFresh(now time.Time) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if now.IsZero() || !now.Before(s.EvidenceExpiresAt) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonActivationFactDrift, "activation snapshot evidence has expired")
	}
	return nil
}

type ActivationAttempt struct {
	ID                    string                     `json:"activation_attempt_id"`
	Scope                 core.ExecutionScope        `json:"scope"`
	ExpectedIdentityEpoch core.Epoch                 `json:"expected_identity_epoch"`
	RequirementDigest     core.Digest                `json:"requirement_digest"`
	Stage                 ActivationStage            `json:"stage"`
	Recovery              RecoveryState              `json:"recovery_state"`
	Snapshot              *ActivationSnapshot        `json:"activation_snapshot,omitempty"`
	IdentityLeaseID       string                     `json:"identity_lease_id,omitempty"`
	IdentityLeaseState    control.IdentityLeaseState `json:"identity_lease_state,omitempty"`
	IdentityLeaseRevision core.Revision              `json:"identity_lease_revision,omitempty"`
	Budget                ActivationOperation        `json:"budget"`
	SandboxReservation    ActivationOperation        `json:"sandbox_reservation"`
	SandboxActivation     ActivationOperation        `json:"sandbox_activation"`
	SandboxFenced         bool                       `json:"sandbox_fenced"`
	FailureReason         string                     `json:"failure_reason,omitempty"`
	Revision              core.Revision              `json:"revision"`
	CreatedAt             time.Time                  `json:"created_at"`
	UpdatedAt             time.Time                  `json:"updated_at"`
}

func (a ActivationAttempt) Validate(now time.Time) error {
	if strings.TrimSpace(a.ID) == "" || a.Revision == 0 || a.CreatedAt.IsZero() || a.UpdatedAt.IsZero() || a.UpdatedAt.Before(a.CreatedAt) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "activation attempt identity, revision and timestamps are required")
	}
	if err := a.Scope.Validate(); err != nil {
		return err
	}
	if err := a.RequirementDigest.Validate(); err != nil {
		return err
	}
	if a.Scope.Identity.Epoch != a.ExpectedIdentityEpoch+1 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonStaleIdentityEpoch, "proposed scope must reserve the next identity epoch")
	}
	if !validStage(a.Stage) || !validRecovery(a.Recovery) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "activation stage or recovery state is invalid")
	}
	for _, operation := range []ActivationOperation{a.Budget, a.SandboxReservation, a.SandboxActivation} {
		if err := operation.Validate(); err != nil {
			return err
		}
	}
	if stageIndex(a.Stage) < stageIndex(StageCommitted) && a.Scope.SandboxLease != nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "pre-commit activation attempt cannot bind a sandbox lease")
	}
	if stageIndex(a.Stage) >= stageIndex(StageSnapshotFrozen) {
		if a.Snapshot == nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "frozen activation stage requires a snapshot")
		}
		if err := a.Snapshot.Validate(); err != nil {
			return err
		}
		if a.Snapshot.AuthorityEpoch != a.Scope.AuthorityEpoch || a.Snapshot.SandboxRequirementDigest != a.RequirementDigest {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonActivationFactDrift, "snapshot no longer matches proposed authority or sandbox requirement")
		}
	} else if a.Snapshot != nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "snapshot cannot exist before the frozen stage")
	}
	if stageIndex(a.Stage) >= stageIndex(StageIdentityLeaseReserved) && (strings.TrimSpace(a.IdentityLeaseID) == "" || a.IdentityLeaseRevision == 0 || a.IdentityLeaseState == "") {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonIdentityLeaseStateInvalid, "reserved activation stage requires an identity lease fact")
	}
	if stageIndex(a.Stage) < stageIndex(StageIdentityLeaseReserved) && (a.IdentityLeaseID != "" || a.IdentityLeaseRevision != 0 || a.IdentityLeaseState != "") {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonIdentityLeaseStateInvalid, "identity lease cannot be attached before its reserved stage")
	}
	if stageIndex(a.Stage) >= stageIndex(StageBudgetResolved) && a.Budget.State != OperationNotRequired && a.Budget.State != OperationConfirmedApplied && a.Budget.State != OperationReleased {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "budget must be resolved before activation can continue")
	}
	if stageIndex(a.Stage) >= stageIndex(StageSandboxReserved) && a.SandboxReservation.State != OperationConfirmedApplied && a.SandboxReservation.State != OperationReleased {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "sandbox reservation must be confirmed before activation can continue")
	}
	if stageIndex(a.Stage) >= stageIndex(StageCommitted) {
		if a.Scope.SandboxLease == nil || string(a.Scope.SandboxLease.ID) != a.SandboxReservation.Reference {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "activation commit must bind the confirmed sandbox lease")
		}
		if a.IdentityLeaseState != control.IdentityLeaseActive {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonIdentityLeaseStateInvalid, "committed activation requires an active identity lease")
		}
	}
	if a.Stage == StageSandboxActive && a.SandboxActivation.State != OperationConfirmedApplied {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "active sandbox stage requires confirmed provider activation")
	}
	if hasUnknownOperation(a) && a.Recovery != RecoveryQuarantined {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonActivationQuarantineRequired, "unknown activation effect must remain quarantined")
	}
	if a.Recovery != RecoveryNormal && strings.TrimSpace(a.FailureReason) == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "recovery state requires a reason")
	}
	if (a.Recovery == RecoveryAborting || a.Recovery == RecoveryAborted) && stageIndex(a.Stage) >= stageIndex(StageCommitted) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "committed activation must use normal stop and cleanup rather than activation abort")
	}
	if a.Recovery == RecoveryNormal && (a.Budget.State == OperationReleased || a.SandboxReservation.State == OperationReleased) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "released activation resources cannot resume normal progress")
	}
	if a.Recovery == RecoveryAborted && !activationObligationsSettled(a) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCleanupEvidenceIncomplete, "aborted activation still has unsettled reservations")
	}
	return nil
}

type TransitionContext struct {
	UnknownResolved bool
}

func ValidateTransition(from, to ActivationAttempt, context TransitionContext, now time.Time) error {
	if err := from.Validate(now); err != nil {
		return err
	}
	if err := to.Validate(now); err != nil {
		return err
	}
	if from.ID != to.ID || from.CreatedAt != to.CreatedAt || from.ExpectedIdentityEpoch != to.ExpectedIdentityEpoch ||
		from.RequirementDigest != to.RequirementDigest || from.Scope.Identity != to.Scope.Identity ||
		from.Scope.Lineage != to.Scope.Lineage || from.Scope.Instance != to.Scope.Instance || from.Scope.AuthorityEpoch != to.Scope.AuthorityEpoch {
		return core.NewError(core.ErrorConflict, core.ReasonActivationAttemptConflict, "activation attempt immutable identity changed")
	}
	if to.Revision != from.Revision+1 || to.UpdatedAt.Before(from.UpdatedAt) {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "activation update requires the next revision and monotonic timestamp")
	}
	if stageIndex(to.Stage) < stageIndex(from.Stage) || stageIndex(to.Stage) > stageIndex(from.Stage)+1 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "activation stage must advance exactly once or remain unchanged")
	}
	if to.Recovery == RecoveryNormal && stageIndex(to.Stage) > stageIndex(from.Stage) && to.Snapshot != nil {
		if err := to.Snapshot.ValidateFresh(now); err != nil {
			return err
		}
	}
	if from.Recovery == RecoveryAborted && to.Recovery != RecoveryAborted {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "aborted activation cannot resume")
	}
	if from.Recovery == RecoveryQuarantined && to.Recovery == RecoveryNormal && !context.UnknownResolved {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInspectCoverageIncomplete, "quarantine may clear only after authoritative inspection")
	}
	if from.Recovery == RecoveryAborting && to.Recovery != RecoveryAborting && to.Recovery != RecoveryAborted && to.Recovery != RecoveryQuarantined {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "aborting activation may only finish or quarantine")
	}
	if from.Recovery != RecoveryNormal && to.Stage != from.Stage {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "activation progress is frozen while recovering")
	}
	if from.Recovery == RecoveryNormal && to.Recovery == RecoveryAborted {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "activation must enter aborting before aborted")
	}
	return nil
}

type ActivationCommitRequest struct {
	AttemptID                     string               `json:"activation_attempt_id"`
	ExpectedAttemptRevision       core.Revision        `json:"expected_attempt_revision"`
	IdentityLeaseID               string               `json:"identity_lease_id"`
	ExpectedIdentityLeaseRevision core.Revision        `json:"expected_identity_lease_revision"`
	SandboxLease                  core.SandboxLeaseRef `json:"sandbox_lease"`
	AuthorityEpoch                core.Epoch           `json:"authority_epoch"`
}

type ActivationCommitResult struct {
	Attempt       ActivationAttempt              `json:"attempt"`
	IdentityLease control.IdentityExecutionLease `json:"identity_lease"`
}

// ActivationFactPort is the authoritative activation journal. CommitActivation
// must update the attempt, Instance binding, and Identity Lease in one logical
// commit; implementations that cannot provide that property are non-conformant.
type ActivationFactPort interface {
	CreateActivationAttempt(context.Context, ActivationAttempt) (ActivationAttempt, error)
	InspectActivationAttempt(context.Context, string) (ActivationAttempt, error)
	CompareAndSwapActivation(context.Context, ActivationAttempt, TransitionContext) (ActivationAttempt, error)
	CommitActivation(context.Context, ActivationCommitRequest) (ActivationCommitResult, error)
}

func validStage(stage ActivationStage) bool { return stageIndex(stage) >= 0 }

func stageIndex(stage ActivationStage) int {
	for index, candidate := range activationOrder {
		if candidate == stage {
			return index
		}
	}
	return -1
}

func validRecovery(state RecoveryState) bool {
	switch state {
	case RecoveryNormal, RecoveryAborting, RecoveryQuarantined, RecoveryAborted:
		return true
	default:
		return false
	}
}

func hasUnknownOperation(attempt ActivationAttempt) bool {
	return attempt.Budget.State == OperationUnknownOutcome || attempt.SandboxReservation.State == OperationUnknownOutcome || attempt.SandboxActivation.State == OperationUnknownOutcome
}

func activationObligationsSettled(attempt ActivationAttempt) bool {
	budgetSettled := attempt.Budget.State == OperationNotStarted || attempt.Budget.State == OperationNotRequired ||
		attempt.Budget.State == OperationConfirmedNotApplied || attempt.Budget.State == OperationReleased
	sandboxSettled := attempt.SandboxReservation.State == OperationNotStarted || attempt.SandboxReservation.State == OperationNotRequired ||
		attempt.SandboxReservation.State == OperationConfirmedNotApplied || attempt.SandboxReservation.State == OperationReleased
	identitySettled := attempt.IdentityLeaseID == "" || attempt.IdentityLeaseState == control.IdentityLeaseReleased
	return budgetSettled && sandboxSettled && identitySettled
}

var activationOrder = []ActivationStage{
	StageProposed, StagePreflighting, StagePreflightPassed, StageSnapshotFrozen,
	StageIdentityLeaseReserved, StageBudgetResolved, StageSandboxReserved,
	StageCommitted, StageSandboxActive,
}

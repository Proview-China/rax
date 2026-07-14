package admission

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type RecoveryAction string

const (
	ActionNone                      RecoveryAction = "none"
	ActionResumePreflight           RecoveryAction = "resume_preflight"
	ActionInspectPreflight          RecoveryAction = "inspect_preflight"
	ActionFreezeSnapshot            RecoveryAction = "freeze_activation_snapshot"
	ActionReserveIdentityLease      RecoveryAction = "reserve_identity_lease"
	ActionResolveBudget             RecoveryAction = "resolve_budget"
	ActionReserveSandbox            RecoveryAction = "reserve_sandbox"
	ActionCommitActivation          RecoveryAction = "commit_activation"
	ActionActivateSandbox           RecoveryAction = "activate_sandbox"
	ActionResumeBinding             RecoveryAction = "resume_binding"
	ActionBeginAbort                RecoveryAction = "begin_abort"
	ActionInspectBudget             RecoveryAction = "inspect_budget"
	ActionInspectSandboxReservation RecoveryAction = "inspect_sandbox_reservation"
	ActionInspectSandboxActivation  RecoveryAction = "inspect_sandbox_activation"
	ActionFenceSandbox              RecoveryAction = "fence_sandbox"
	ActionReleaseSandbox            RecoveryAction = "release_sandbox"
	ActionReleaseBudget             RecoveryAction = "release_budget"
	ActionRevokeIdentityLease       RecoveryAction = "revoke_identity_lease"
	ActionReleaseIdentityLease      RecoveryAction = "release_identity_lease"
	ActionMarkAborted               RecoveryAction = "mark_aborted"
	ActionAwaitOperator             RecoveryAction = "await_operator"
)

type RecoveryDecision struct {
	Action        RecoveryAction `json:"action"`
	AutomaticSafe bool           `json:"automatic_safe"`
	Reason        string         `json:"reason"`
}

// PlanRecovery is pure and deterministic. A restarted coordinator can call it
// repeatedly against the latest journal record; it never treats a timeout as a
// failed effect and never proposes a second dispatch while outcome is unknown.
func PlanRecovery(attempt ActivationAttempt, now time.Time) (RecoveryDecision, error) {
	if err := attempt.Validate(now); err != nil {
		return RecoveryDecision{}, err
	}

	switch attempt.Recovery {
	case RecoveryAborted:
		return decision(ActionNone, true, "activation is already durably aborted"), nil
	case RecoveryQuarantined:
		return quarantinedDecision(attempt), nil
	case RecoveryAborting:
		return abortDecision(attempt), nil
	}
	if stageIndex(attempt.Stage) >= stageIndex(StageSnapshotFrozen) && stageIndex(attempt.Stage) < stageIndex(StageCommitted) {
		if err := attempt.Snapshot.ValidateFresh(now); err != nil {
			return decision(ActionBeginAbort, true, "activation snapshot expired or drifted before the commit point"), nil
		}
	}
	if attempt.SandboxActivation.State == OperationIntentRecorded {
		return decision(ActionInspectSandboxActivation, false, "sandbox activation intent exists but dispatch outcome is not durable"), nil
	}
	if attempt.SandboxReservation.State == OperationIntentRecorded {
		return decision(ActionInspectSandboxReservation, false, "sandbox reservation intent exists but dispatch outcome is not durable"), nil
	}
	if attempt.Budget.State == OperationIntentRecorded {
		return decision(ActionInspectBudget, false, "budget intent exists but dispatch outcome is not durable"), nil
	}

	switch attempt.Stage {
	case StageProposed:
		return decision(ActionResumePreflight, true, "proposed identity has no execution authority or external allocation"), nil
	case StagePreflighting:
		return decision(ActionInspectPreflight, false, "preflight may have crossed an external probe boundary"), nil
	case StagePreflightPassed:
		return decision(ActionFreezeSnapshot, true, "preflight is clean and activation facts must be frozen again"), nil
	case StageSnapshotFrozen:
		return decision(ActionReserveIdentityLease, true, "snapshot is frozen and the exclusive identity slot is next"), nil
	case StageIdentityLeaseReserved:
		return decision(ActionResolveBudget, true, "reserved identity lease grants no general execution authority"), nil
	case StageBudgetResolved:
		return decision(ActionReserveSandbox, true, "budget is resolved and sandbox reservation remains quarantined"), nil
	case StageSandboxReserved:
		return decision(ActionCommitActivation, true, "all reservations are confirmed and the logical commit is next"), nil
	case StageCommitted:
		return decision(ActionActivateSandbox, true, "activation is committed but provider sandbox is still quarantined"), nil
	case StageSandboxActive:
		return decision(ActionResumeBinding, true, "activation is complete and binding may reconcile from its own journal"), nil
	default:
		return RecoveryDecision{}, core.NewError(core.ErrorInternal, core.ReasonInvalidState, "validated activation stage has no recovery action")
	}
}

func quarantinedDecision(attempt ActivationAttempt) RecoveryDecision {
	switch {
	case attempt.SandboxActivation.State == OperationUnknownOutcome:
		return decision(ActionInspectSandboxActivation, false, "sandbox activation outcome is unknown; duplicate activation is forbidden")
	case attempt.SandboxReservation.State == OperationUnknownOutcome:
		return decision(ActionInspectSandboxReservation, false, "sandbox reservation outcome is unknown; duplicate allocation is forbidden")
	case attempt.Budget.State == OperationUnknownOutcome:
		return decision(ActionInspectBudget, false, "budget reservation outcome is unknown; a second reservation is forbidden")
	default:
		return decision(ActionAwaitOperator, false, "quarantine has no machine-resolvable unknown operation")
	}
}

func abortDecision(attempt ActivationAttempt) RecoveryDecision {
	if stageIndex(attempt.Stage) >= stageIndex(StageSandboxReserved) && attempt.SandboxReservation.State == OperationConfirmedApplied && !attempt.SandboxFenced {
		return decision(ActionFenceSandbox, true, "reserved sandbox must be fenced before release")
	}
	if attempt.SandboxReservation.State != OperationNotStarted && attempt.SandboxReservation.State != OperationNotRequired && attempt.SandboxReservation.State != OperationReleased && attempt.SandboxReservation.State != OperationConfirmedNotApplied {
		return decision(ActionReleaseSandbox, true, "sandbox cleanup precedes budget and identity release")
	}
	if attempt.Budget.State == OperationConfirmedApplied {
		return decision(ActionReleaseBudget, true, "budget reservation remains held after sandbox cleanup")
	}
	if attempt.IdentityLeaseState == control.IdentityLeaseReserved || attempt.IdentityLeaseState == control.IdentityLeaseActive {
		return decision(ActionRevokeIdentityLease, true, "reserved identity slot must be revoked before release")
	}
	if attempt.IdentityLeaseState == control.IdentityLeaseRevoked || attempt.IdentityLeaseState == control.IdentityLeaseExpired {
		return decision(ActionReleaseIdentityLease, true, "revoked or expired identity slot can now be released")
	}
	return decision(ActionMarkAborted, true, "all activation obligations are settled")
}

func decision(action RecoveryAction, automatic bool, reason string) RecoveryDecision {
	return RecoveryDecision{Action: action, AutomaticSafe: automatic, Reason: reason}
}

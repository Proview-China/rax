package core

type LifecyclePhase string

const (
	PhasePending      LifecyclePhase = "pending"
	PhaseAdmitted     LifecyclePhase = "admitted"
	PhasePreflighting LifecyclePhase = "preflighting"
	PhaseActivating   LifecyclePhase = "activating"
	PhaseProvisioning LifecyclePhase = "provisioning"
	PhaseBinding      LifecyclePhase = "binding"
	PhaseStarting     LifecyclePhase = "starting"
	PhaseReady        LifecyclePhase = "ready"
	PhaseRunning      LifecyclePhase = "running"
	PhaseStopping     LifecyclePhase = "stopping"
	PhaseTerminal     LifecyclePhase = "terminal"
)

type ExecutionCertainty string

const (
	CertaintyConfirmed ExecutionCertainty = "confirmed"
	CertaintyUnknown   ExecutionCertainty = "unknown"
	CertaintyLost      ExecutionCertainty = "lost"
	CertaintyFenced    ExecutionCertainty = "fenced"
)

type CleanupStatus string

const (
	CleanupNotRequired   CleanupStatus = "not_required"
	CleanupPending       CleanupStatus = "pending"
	CleanupComplete      CleanupStatus = "complete"
	CleanupFailed        CleanupStatus = "failed"
	CleanupIndeterminate CleanupStatus = "indeterminate"
)

// InstanceState keeps the three state dimensions orthogonal. The evidence
// flags are validation inputs, not additional lifecycle phases.
type InstanceState struct {
	Phase                   LifecyclePhase     `json:"lifecycle_phase"`
	Certainty               ExecutionCertainty `json:"execution_certainty"`
	Cleanup                 CleanupStatus      `json:"cleanup_status"`
	HasCleanupObligations   bool               `json:"has_cleanup_obligations"`
	CleanupEvidenceComplete bool               `json:"cleanup_evidence_complete"`
}

func (s InstanceState) Validate() error {
	if !validPhase(s.Phase) || !validCertainty(s.Certainty) || !validCleanup(s.Cleanup) {
		return NewError(ErrorInvalidArgument, ReasonInvalidState, "unknown state value")
	}

	phaseIndex := lifecycleIndex(s.Phase)
	if phaseIndex <= lifecycleIndex(PhasePreflighting) {
		if s.Certainty != CertaintyConfirmed || s.Cleanup != CleanupNotRequired || s.HasCleanupObligations {
			return NewError(ErrorPreconditionFailed, ReasonInvalidState, "pre-activation state must be confirmed and cleanup-free")
		}
	}
	if s.Certainty == CertaintyUnknown && phaseIndex < lifecycleIndex(PhaseActivating) {
		return NewError(ErrorPreconditionFailed, ReasonInvalidState, "unknown certainty cannot remain in preflight")
	}
	if s.Certainty == CertaintyLost && phaseIndex < lifecycleIndex(PhaseProvisioning) {
		return NewError(ErrorPreconditionFailed, ReasonInvalidState, "lost certainty requires an external execution phase")
	}
	if s.Certainty == CertaintyFenced && s.Phase != PhaseStopping && s.Phase != PhaseTerminal {
		return NewError(ErrorPreconditionFailed, ReasonInvalidState, "fenced state must be stopping or terminal")
	}

	if s.HasCleanupObligations && s.Cleanup == CleanupNotRequired {
		return NewError(ErrorPreconditionFailed, ReasonInvalidState, "cleanup obligations require a cleanup state")
	}
	if !s.HasCleanupObligations && s.Cleanup != CleanupNotRequired {
		return NewError(ErrorPreconditionFailed, ReasonInvalidState, "cleanup state requires an obligation")
	}
	if s.Cleanup == CleanupComplete {
		if s.Phase != PhaseTerminal || !s.CleanupEvidenceComplete {
			return NewError(ErrorPreconditionFailed, ReasonCleanupEvidenceIncomplete, "cleanup complete requires terminal lifecycle and complete evidence")
		}
	} else if s.CleanupEvidenceComplete {
		return NewError(ErrorPreconditionFailed, ReasonInvalidState, "cleanup evidence completion only accompanies cleanup complete")
	}
	if (s.Cleanup == CleanupFailed || s.Cleanup == CleanupIndeterminate) && s.Phase != PhaseStopping && s.Phase != PhaseTerminal {
		return NewError(ErrorPreconditionFailed, ReasonInvalidState, "failed or indeterminate cleanup requires stopping or terminal lifecycle")
	}
	return nil
}

type TransitionContext struct {
	InspectCoverageComplete    bool
	CleanupRetryAuthorized     bool
	CleanupCompletionRetracted bool
}

func ValidateStateTransition(from, to InstanceState, context TransitionContext) error {
	if err := from.Validate(); err != nil {
		return err
	}
	if err := to.Validate(); err != nil {
		return err
	}

	if from.Phase == PhaseTerminal && to.Phase != PhaseTerminal {
		return NewError(ErrorPreconditionFailed, ReasonTerminalInstance, "terminal lifecycle cannot be revived")
	}
	if !validLifecycleMove(from.Phase, to.Phase) {
		return NewError(ErrorPreconditionFailed, ReasonInvalidTransition, "lifecycle cannot move backwards or skip the normal chain")
	}
	if from.Certainty == CertaintyFenced && to.Certainty != CertaintyFenced {
		return NewError(ErrorPreconditionFailed, ReasonFencedInstance, "fenced instance cannot regain execution authority")
	}
	if (from.Certainty == CertaintyUnknown || from.Certainty == CertaintyLost) && to.Certainty == CertaintyConfirmed && !context.InspectCoverageComplete {
		return NewError(ErrorPreconditionFailed, ReasonInspectCoverageIncomplete, "certainty convergence requires complete authoritative coverage")
	}
	if from.Cleanup == CleanupComplete && to.Cleanup != CleanupComplete && !context.CleanupCompletionRetracted {
		return NewError(ErrorPreconditionFailed, ReasonCleanupRetractionMissing, "cleanup completion may regress only through an explicit retraction")
	}
	if (from.Cleanup == CleanupFailed || from.Cleanup == CleanupIndeterminate) && to.Cleanup == CleanupPending && !context.CleanupRetryAuthorized {
		return NewError(ErrorForbidden, ReasonCleanupRetryUnauthorized, "cleanup retry requires fresh authority")
	}
	return nil
}

func validLifecycleMove(from, to LifecyclePhase) bool {
	if from == to {
		return true
	}
	if to == PhaseStopping && from != PhaseTerminal {
		return true
	}
	if from == PhaseStopping && to == PhaseTerminal {
		return true
	}
	return lifecycleIndex(to) == lifecycleIndex(from)+1
}

func lifecycleIndex(phase LifecyclePhase) int {
	for index, candidate := range lifecycleOrder {
		if candidate == phase {
			return index
		}
	}
	return -1
}

var lifecycleOrder = []LifecyclePhase{
	PhasePending, PhaseAdmitted, PhasePreflighting, PhaseActivating,
	PhaseProvisioning, PhaseBinding, PhaseStarting, PhaseReady,
	PhaseRunning, PhaseStopping, PhaseTerminal,
}

func validPhase(value LifecyclePhase) bool { return lifecycleIndex(value) >= 0 }

func validCertainty(value ExecutionCertainty) bool {
	switch value {
	case CertaintyConfirmed, CertaintyUnknown, CertaintyLost, CertaintyFenced:
		return true
	default:
		return false
	}
}

func validCleanup(value CleanupStatus) bool {
	switch value {
	case CleanupNotRequired, CleanupPending, CleanupComplete, CleanupFailed, CleanupIndeterminate:
		return true
	default:
		return false
	}
}

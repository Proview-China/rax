// Package kernel provides deterministic, component-free Runtime state
// coordination. External effects and facts enter only through ports.
package kernel

import (
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type Aggregate struct {
	mu       sync.RWMutex
	scope    core.ExecutionScope
	state    core.InstanceState
	revision core.Revision
}

type Snapshot struct {
	Scope    core.ExecutionScope `json:"scope"`
	State    core.InstanceState  `json:"state"`
	Revision core.Revision       `json:"revision"`
}

func NewAggregate(scope core.ExecutionScope, state core.InstanceState) (*Aggregate, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	if err := state.Validate(); err != nil {
		return nil, err
	}
	if err := validateScopeState(scope, state); err != nil {
		return nil, err
	}
	return &Aggregate{scope: cloneScope(scope), state: state, revision: 1}, nil
}

func (a *Aggregate) Snapshot() Snapshot {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return Snapshot{Scope: cloneScope(a.scope), State: a.state, Revision: a.revision}
}

type TransitionRequest struct {
	Preconditions core.ExecutionPreconditions
	NextState     core.InstanceState
	Context       core.TransitionContext
}

func (a *Aggregate) Transition(request TransitionRequest) (Snapshot, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := core.CheckExecutionPreconditions(request.Preconditions, core.CurrentExecutionFacts{Scope: a.scope, Revision: a.revision}); err != nil {
		return Snapshot{}, err
	}
	if err := core.ValidateStateTransition(a.state, request.NextState, request.Context); err != nil {
		return Snapshot{}, err
	}
	if err := validateScopeState(a.scope, request.NextState); err != nil {
		return Snapshot{}, err
	}
	a.state = request.NextState
	a.revision++
	return Snapshot{Scope: cloneScope(a.scope), State: a.state, Revision: a.revision}, nil
}

type ActivationCommitRequest struct {
	Preconditions core.ExecutionPreconditions
	SandboxLease  core.SandboxLeaseRef
	NextState     core.InstanceState
}

// CommitActivation is the only aggregate operation that binds the actual
// SandboxLeaseRef while crossing activating -> provisioning. Provider-side
// activation remains a separate effect and is not claimed atomically here.
func (a *Aggregate) CommitActivation(request ActivationCommitRequest) (Snapshot, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := core.CheckExecutionPreconditions(request.Preconditions, core.CurrentExecutionFacts{Scope: a.scope, Revision: a.revision}); err != nil {
		return Snapshot{}, err
	}
	if a.state.Phase != core.PhaseActivating || request.NextState.Phase != core.PhaseProvisioning || a.scope.SandboxLease != nil {
		return Snapshot{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "activation commit requires activating state without a bound lease and targets provisioning")
	}
	if err := request.SandboxLease.Validate(); err != nil {
		return Snapshot{}, err
	}
	if err := core.ValidateStateTransition(a.state, request.NextState, core.TransitionContext{}); err != nil {
		return Snapshot{}, err
	}
	nextScope := cloneScope(a.scope)
	nextScope.SandboxLease = &request.SandboxLease
	if err := validateScopeState(nextScope, request.NextState); err != nil {
		return Snapshot{}, err
	}
	a.scope = nextScope
	a.state = request.NextState
	a.revision++
	return Snapshot{Scope: cloneScope(a.scope), State: a.state, Revision: a.revision}, nil
}

type ObservationDisposition string

const (
	ObservationCurrent ObservationDisposition = "current"
	ObservationLate    ObservationDisposition = "late"
)

type SourceObservation struct {
	Scope          core.ExecutionScope
	SourceID       string
	SourceEpoch    core.Epoch
	SourceSequence uint64
}

// ClassifyObservation never mutates lifecycle. A current observation still
// needs authority/inspect evaluation before another transition is submitted.
func (a *Aggregate) ClassifyObservation(observation SourceObservation) ObservationDisposition {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if !sameExecutionScope(observation.Scope, a.scope) {
		return ObservationLate
	}
	return ObservationCurrent
}

func sameExecutionScope(left, right core.ExecutionScope) bool {
	if left.Identity != right.Identity || left.Lineage != right.Lineage || left.Instance != right.Instance || left.AuthorityEpoch != right.AuthorityEpoch {
		return false
	}
	if left.SandboxLease == nil || right.SandboxLease == nil {
		return left.SandboxLease == nil && right.SandboxLease == nil
	}
	return *left.SandboxLease == *right.SandboxLease
}

func cloneScope(scope core.ExecutionScope) core.ExecutionScope {
	clone := scope
	if scope.SandboxLease != nil {
		lease := *scope.SandboxLease
		clone.SandboxLease = &lease
	}
	return clone
}

func validateScopeState(scope core.ExecutionScope, state core.InstanceState) error {
	phaseNumber := phaseIndex(state.Phase)
	if phaseNumber <= phaseIndex(core.PhaseActivating) && scope.SandboxLease != nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "pre-commit instance cannot bind a sandbox lease")
	}
	if phaseNumber >= phaseIndex(core.PhaseProvisioning) && phaseNumber <= phaseIndex(core.PhaseRunning) && scope.SandboxLease == nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "active execution phase requires a sandbox lease")
	}
	if scope.SandboxLease != nil && !state.HasCleanupObligations {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "bound sandbox lease creates a cleanup obligation")
	}
	return nil
}

func phaseIndex(phase core.LifecyclePhase) int {
	order := []core.LifecyclePhase{
		core.PhasePending, core.PhaseAdmitted, core.PhasePreflighting, core.PhaseActivating,
		core.PhaseProvisioning, core.PhaseBinding, core.PhaseStarting, core.PhaseReady,
		core.PhaseRunning, core.PhaseStopping, core.PhaseTerminal,
	}
	for index, candidate := range order {
		if candidate == phase {
			return index
		}
	}
	return -1
}

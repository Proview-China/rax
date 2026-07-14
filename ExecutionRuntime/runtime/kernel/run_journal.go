package kernel

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// RunJournal coordinates Runtime-owned Run facts through a durable CAS Port.
// It deliberately contains no Harness session state and never derives an
// ExecutionOutcome from an opaque component observation.
type RunJournal struct {
	facts control.RunFactPort
}

func NewRunJournal(facts control.RunFactPort) (*RunJournal, error) {
	if facts == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "run fact port is required")
	}
	return &RunJournal{facts: facts}, nil
}

func (j *RunJournal) Start(ctx context.Context, scope core.ExecutionScope, runID core.AgentRunID, sessionRef string, now time.Time) (core.AgentRunRecord, error) {
	record := core.AgentRunRecord{
		ID: runID, Scope: scope, Status: core.RunRunning, Revision: 1,
		SessionRef: sessionRef, StartedAt: now,
	}
	return j.facts.CreateRun(ctx, record)
}

func (j *RunJournal) BeginStop(ctx context.Context, scope core.ExecutionScope, runID core.AgentRunID) (core.AgentRunRecord, error) {
	current, err := j.facts.InspectRun(ctx, scope, runID)
	if err != nil {
		return core.AgentRunRecord{}, err
	}
	if current.Status == core.RunStopping {
		return current, nil
	}
	if current.Status != core.RunRunning {
		return core.AgentRunRecord{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunConflict, "only a running run may begin stopping")
	}
	next := current
	next.Status = core.RunStopping
	next.Revision++
	return j.facts.CompareAndSwapRun(ctx, control.RunFactCASRequest{ExpectedRevision: current.Revision, Next: next})
}

// RecordCompletionClaim durably associates an already-ingested observation
// with its Run while deliberately leaving Runtime status and outcome unchanged.
func (j *RunJournal) RecordCompletionClaim(ctx context.Context, scope core.ExecutionScope, runID core.AgentRunID, claim core.RunCompletionClaim) (core.AgentRunRecord, error) {
	if err := claim.Validate(); err != nil {
		return core.AgentRunRecord{}, err
	}
	current, err := j.facts.InspectRun(ctx, scope, runID)
	if err != nil {
		return core.AgentRunRecord{}, err
	}
	if current.CompletionClaim != nil {
		if *current.CompletionClaim == claim {
			return current, nil
		}
		return core.AgentRunRecord{}, core.NewError(core.ErrorConflict, core.ReasonRunClaimConflict, "run already carries a different completion claim")
	}
	if current.Status == core.RunPending || current.Status == core.RunTerminal {
		return core.AgentRunRecord{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunClaimConflict, "completion claim cannot attach to pending or terminal run")
	}
	next := current
	next.Revision++
	next.CompletionClaim = &claim
	persisted, err := j.facts.CompareAndSwapRun(ctx, control.RunFactCASRequest{ExpectedRevision: current.Revision, Next: next})
	if err == nil || !core.HasReason(err, core.ReasonRevisionConflict) {
		return persisted, err
	}
	// A concurrent replay of the same source event is idempotent after the
	// winning CAS; any different claim remains a conflict.
	latest, inspectErr := j.facts.InspectRun(ctx, scope, runID)
	if inspectErr != nil {
		return core.AgentRunRecord{}, err
	}
	if latest.CompletionClaim != nil && *latest.CompletionClaim == claim {
		return latest, nil
	}
	return core.AgentRunRecord{}, err
}

// Finish is idempotent only when the already-persisted terminal outcome is the
// same. A conflicting late claim cannot overwrite the first linearized result.
func (j *RunJournal) Finish(ctx context.Context, scope core.ExecutionScope, runID core.AgentRunID, outcome core.ExecutionOutcome, now time.Time) (core.AgentRunRecord, error) {
	current, err := j.facts.InspectRun(ctx, scope, runID)
	if err != nil {
		return core.AgentRunRecord{}, err
	}
	if current.Status == core.RunTerminal {
		if current.Outcome == outcome {
			return current, nil
		}
		return core.AgentRunRecord{}, core.NewError(core.ErrorConflict, core.ReasonRunConflict, "terminal run outcome conflicts with the persisted fact")
	}
	if current.Status != core.RunRunning && current.Status != core.RunStopping {
		return core.AgentRunRecord{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunConflict, "run is not eligible for terminal settlement")
	}
	next := current
	next.Status = core.RunTerminal
	next.Revision++
	next.EndedAt = now
	next.Outcome = outcome
	return j.facts.CompareAndSwapRun(ctx, control.RunFactCASRequest{ExpectedRevision: current.Revision, Next: next})
}

func (j *RunJournal) Inspect(ctx context.Context, scope core.ExecutionScope, runID core.AgentRunID) (core.AgentRunRecord, error) {
	return j.facts.InspectRun(ctx, scope, runID)
}

func (j *RunJournal) RecoverActive(ctx context.Context, scope core.ExecutionScope) (core.AgentRunRecord, error) {
	return j.facts.InspectActiveRun(ctx, scope)
}

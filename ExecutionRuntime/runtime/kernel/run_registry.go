package kernel

import (
	"fmt"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// RunRegistry enforces the V1 one-active-run-per-instance invariant. It owns
// Runtime run records, not Harness session state.
type RunRegistry struct {
	mu      sync.Mutex
	records map[string]core.AgentRunRecord
	active  map[string]core.AgentRunID
}

func NewRunRegistry() *RunRegistry {
	return &RunRegistry{records: make(map[string]core.AgentRunRecord), active: make(map[string]core.AgentRunID)}
}

func (r *RunRegistry) Start(scope core.ExecutionScope, runID core.AgentRunID, sessionRef string, now time.Time) (core.AgentRunRecord, error) {
	if err := scope.Validate(); err != nil {
		return core.AgentRunRecord{}, err
	}
	if runID == "" || now.IsZero() {
		return core.AgentRunRecord{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "run id and start time are required")
	}
	key := runInstanceKey(scope)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.active[key]; exists {
		return core.AgentRunRecord{}, core.NewError(core.ErrorConflict, core.ReasonRunConflict, "instance already has an active run")
	}
	if _, exists := r.records[string(runID)]; exists {
		return core.AgentRunRecord{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "run id already exists")
	}
	record := core.AgentRunRecord{ID: runID, Scope: cloneScope(scope), Status: core.RunRunning, Revision: 1, SessionRef: sessionRef, StartedAt: now}
	if err := record.Validate(); err != nil {
		return core.AgentRunRecord{}, err
	}
	r.records[string(runID)] = record
	r.active[key] = runID
	return record, nil
}

func (r *RunRegistry) Finish(scope core.ExecutionScope, runID core.AgentRunID, outcome core.ExecutionOutcome, now time.Time) (core.AgentRunRecord, error) {
	if err := scope.Validate(); err != nil {
		return core.AgentRunRecord{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	record, exists := r.records[string(runID)]
	if !exists || !sameExecutionScope(record.Scope, scope) {
		return core.AgentRunRecord{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "run record does not exist for the execution scope")
	}
	if active, exists := r.active[runInstanceKey(scope)]; !exists || active != runID || record.Status == core.RunTerminal {
		return core.AgentRunRecord{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunConflict, "run is not active")
	}
	record.Status = core.RunTerminal
	record.Revision++
	record.EndedAt = now
	record.Outcome = outcome
	if err := record.Validate(); err != nil {
		return core.AgentRunRecord{}, err
	}
	r.records[string(runID)] = record
	delete(r.active, runInstanceKey(scope))
	return record, nil
}

func (r *RunRegistry) Inspect(runID core.AgentRunID) (core.AgentRunRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	record, exists := r.records[string(runID)]
	if !exists {
		return core.AgentRunRecord{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "run record does not exist")
	}
	return record, nil
}

func runInstanceKey(scope core.ExecutionScope) string {
	return string(scope.Identity.TenantID) + "/" + string(scope.Identity.ID) + "/" + string(scope.Lineage.ID) + "/" + string(scope.Instance.ID) + "/" + fmt.Sprint(scope.Instance.Epoch)
}

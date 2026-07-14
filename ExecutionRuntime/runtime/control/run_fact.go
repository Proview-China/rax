package control

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// RunFactPort is the linearizable owner of Runtime AgentRunRecord facts.
// Harness may contribute observations and completion claims, but it never
// creates or completes this record directly.
type RunFactPort interface {
	CreateRun(context.Context, core.AgentRunRecord) (core.AgentRunRecord, error)
	InspectRun(context.Context, core.ExecutionScope, core.AgentRunID) (core.AgentRunRecord, error)
	InspectActiveRun(context.Context, core.ExecutionScope) (core.AgentRunRecord, error)
	CompareAndSwapRun(context.Context, RunFactCASRequest) (core.AgentRunRecord, error)
}

type RunFactCASRequest struct {
	ExpectedRevision core.Revision       `json:"expected_revision"`
	Next             core.AgentRunRecord `json:"next"`
}

func (r RunFactCASRequest) Validate() error {
	if r.ExpectedRevision == 0 || r.Next.Revision != r.ExpectedRevision+1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "run CAS requires the next consecutive revision")
	}
	return r.Next.Validate()
}

// ValidateRunFactTransition freezes the persistence-level lifecycle. It does
// not infer a terminal outcome from a Harness claim; callers must establish
// that outcome before submitting the CAS.
func ValidateRunFactTransition(current, next core.AgentRunRecord) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if current.ID != next.ID || !sameRunScope(current.Scope, next.Scope) || current.SessionRef != next.SessionRef {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunConflict, "run identity, scope and session binding are immutable")
	}
	if next.Revision != current.Revision+1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "run revision is stale or skipped")
	}
	claimAttached := current.CompletionClaim == nil && next.CompletionClaim != nil
	if current.CompletionClaim != nil && (next.CompletionClaim == nil || *current.CompletionClaim != *next.CompletionClaim) {
		return core.NewError(core.ErrorConflict, core.ReasonRunClaimConflict, "persisted completion claim is immutable")
	}
	switch current.Status {
	case core.RunPending:
		if next.Status != core.RunRunning {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "pending run may only become running")
		}
	case core.RunRunning:
		if next.Status == core.RunRunning && claimAttached {
			break
		}
		if next.Status != core.RunStopping && next.Status != core.RunTerminal {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "running run may only stop or become terminal")
		}
	case core.RunStopping:
		if next.Status == core.RunStopping && claimAttached {
			break
		}
		if next.Status != core.RunTerminal {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "stopping run may only become terminal")
		}
	case core.RunTerminal:
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonTerminalInstance, "terminal run fact is immutable")
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "unknown current run status")
	}
	if current.Status != core.RunPending && !current.StartedAt.Equal(next.StartedAt) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunConflict, "run start time is immutable after execution begins")
	}
	if next.Status == current.Status && claimAttached && (!next.EndedAt.Equal(current.EndedAt) || next.Outcome != current.Outcome) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunClaimConflict, "claim attachment cannot mutate Runtime outcome fields")
	}
	return nil
}

func sameRunScope(left, right core.ExecutionScope) bool {
	if left.Identity != right.Identity || left.Lineage != right.Lineage || left.Instance != right.Instance || left.AuthorityEpoch != right.AuthorityEpoch {
		return false
	}
	if left.SandboxLease == nil || right.SandboxLease == nil {
		return left.SandboxLease == nil && right.SandboxLease == nil
	}
	return *left.SandboxLease == *right.SandboxLease
}

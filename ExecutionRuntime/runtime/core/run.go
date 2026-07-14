package core

import (
	"strings"
	"time"
)

type RunStatus string

const (
	RunPending  RunStatus = "pending"
	RunRunning  RunStatus = "running"
	RunStopping RunStatus = "stopping"
	RunTerminal RunStatus = "terminal"
)

type ExecutionOutcome string

const (
	OutcomeCompleted           ExecutionOutcome = "completed"
	OutcomeCancelled           ExecutionOutcome = "cancelled"
	OutcomeFailed              ExecutionOutcome = "failed"
	OutcomeLost                ExecutionOutcome = "lost"
	OutcomeIndeterminate       ExecutionOutcome = "indeterminate"
	OutcomeNeedsReconciliation ExecutionOutcome = "needs_reconciliation"
)

type RunCompletionClaimKind string

const (
	RunClaimCompleted     RunCompletionClaimKind = "completed"
	RunClaimCancelled     RunCompletionClaimKind = "cancelled"
	RunClaimFailed        RunCompletionClaimKind = "failed"
	RunClaimIndeterminate RunCompletionClaimKind = "indeterminate"
)

// RunCompletionClaim is durable evidence that an execution component claimed
// its own loop ended. It is not a Runtime ExecutionOutcome and carries no
// authority to complete the Run without independent settlement.
type RunCompletionClaim struct {
	SourceID         string                 `json:"source_id"`
	SourceEpoch      Epoch                  `json:"source_epoch"`
	SourceSequence   uint64                 `json:"source_sequence"`
	Kind             RunCompletionClaimKind `json:"kind"`
	PayloadDigest    Digest                 `json:"payload_digest"`
	EvidenceScope    string                 `json:"evidence_scope"`
	EvidenceSequence uint64                 `json:"evidence_sequence"`
	ObservedAt       time.Time              `json:"observed_at"`
}

func (c RunCompletionClaim) Validate() error {
	if strings.TrimSpace(c.SourceID) == "" || c.SourceEpoch == 0 || c.SourceSequence == 0 || !validRunCompletionClaimKind(c.Kind) || strings.TrimSpace(c.EvidenceScope) == "" || c.EvidenceSequence == 0 || c.ObservedAt.IsZero() {
		return NewError(ErrorInvalidArgument, ReasonRunClaimUnverified, "completion claim requires source, sequence, kind, evidence reference and observation time")
	}
	return c.PayloadDigest.Validate()
}

type AgentRunRecord struct {
	ID              AgentRunID          `json:"run_id"`
	Scope           ExecutionScope      `json:"scope"`
	Status          RunStatus           `json:"status"`
	Revision        Revision            `json:"revision"`
	SessionRef      string              `json:"session_ref,omitempty"`
	StartedAt       time.Time           `json:"started_at,omitempty"`
	EndedAt         time.Time           `json:"ended_at,omitempty"`
	Outcome         ExecutionOutcome    `json:"execution_outcome,omitempty"`
	CompletionClaim *RunCompletionClaim `json:"completion_claim,omitempty"`
}

func (r AgentRunRecord) Validate() error {
	if blank(string(r.ID)) || r.Revision == 0 {
		return NewError(ErrorInvalidArgument, ReasonInvalidReference, "run id and revision are required")
	}
	if err := r.Scope.Validate(); err != nil {
		return err
	}
	switch r.Status {
	case RunPending:
		if !r.StartedAt.IsZero() || !r.EndedAt.IsZero() || r.Outcome != "" || r.CompletionClaim != nil {
			return NewError(ErrorPreconditionFailed, ReasonInvalidState, "pending run cannot have execution timestamps or outcome")
		}
	case RunRunning, RunStopping:
		if r.StartedAt.IsZero() || !r.EndedAt.IsZero() || r.Outcome != "" {
			return NewError(ErrorPreconditionFailed, ReasonInvalidState, "active run requires only a start time")
		}
	case RunTerminal:
		if r.StartedAt.IsZero() || r.EndedAt.IsZero() || r.EndedAt.Before(r.StartedAt) || !validExecutionOutcome(r.Outcome) {
			return NewError(ErrorPreconditionFailed, ReasonInvalidState, "terminal run requires ordered timestamps and an execution outcome")
		}
	default:
		return NewError(ErrorInvalidArgument, ReasonInvalidState, "unknown run status")
	}
	if r.CompletionClaim != nil {
		if err := r.CompletionClaim.Validate(); err != nil {
			return err
		}
		if r.CompletionClaim.SourceEpoch != r.Scope.Instance.Epoch || r.CompletionClaim.ObservedAt.Before(r.StartedAt) {
			return NewError(ErrorPreconditionFailed, ReasonRunClaimUnverified, "completion claim epoch and time must belong to the run")
		}
	}
	return nil
}

type TerminationReport struct {
	Scope                     ExecutionScope   `json:"scope"`
	State                     InstanceState    `json:"instance_state"`
	ExecutionOutcome          ExecutionOutcome `json:"execution_outcome"`
	EffectSettlement          string           `json:"effect_settlement"`
	RemoteContinuationsStatus string           `json:"remote_continuations_status"`
	ProviderRetentionStatus   string           `json:"provider_data_retention_status"`
	CompletedAt               time.Time        `json:"completed_at"`
}

func (r TerminationReport) Validate() error {
	if err := r.Scope.Validate(); err != nil {
		return err
	}
	if err := r.State.Validate(); err != nil {
		return err
	}
	if r.State.Phase != PhaseTerminal || !validExecutionOutcome(r.ExecutionOutcome) || blank(r.EffectSettlement) || blank(r.RemoteContinuationsStatus) || blank(r.ProviderRetentionStatus) || r.CompletedAt.IsZero() {
		return NewError(ErrorPreconditionFailed, ReasonInvalidState, "termination report requires terminal state and every independent outcome dimension")
	}
	return nil
}

func validExecutionOutcome(value ExecutionOutcome) bool {
	switch value {
	case OutcomeCompleted, OutcomeCancelled, OutcomeFailed, OutcomeLost, OutcomeIndeterminate, OutcomeNeedsReconciliation:
		return true
	default:
		return false
	}
}

func validRunCompletionClaimKind(value RunCompletionClaimKind) bool {
	switch value {
	case RunClaimCompleted, RunClaimCancelled, RunClaimFailed, RunClaimIndeterminate:
		return true
	default:
		return false
	}
}

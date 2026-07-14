// Package ports defines narrow dependencies consumed by the Harness kernel.
package ports

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type ContextRequest struct {
	Run               contract.RunRef            `json:"run"`
	ContextPlanDigest core.Digest                `json:"context_plan_digest"`
	Input             runtimeports.OpaquePayload `json:"input"`
}

func (r ContextRequest) Validate() error {
	if err := r.Run.Validate(); err != nil {
		return err
	}
	if err := r.ContextPlanDigest.Validate(); err != nil {
		return err
	}
	return contract.ValidateOpaque(r.Input)
}

type ContextSnapshot struct {
	Ref            string                     `json:"snapshot_ref"`
	Payload        runtimeports.OpaquePayload `json:"payload"`
	EvidenceDigest core.Digest                `json:"evidence_digest"`
	ObservedAt     time.Time                  `json:"observed_at"`
}

func (s ContextSnapshot) Validate() error {
	if strings.TrimSpace(s.Ref) == "" || len(s.Ref) > contract.MaxReferenceBytes || s.ObservedAt.IsZero() {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "context snapshot reference and time are required")
	}
	if err := contract.ValidateOpaque(s.Payload); err != nil {
		return err
	}
	return s.EvidenceDigest.Validate()
}

// ContextPort is a Harness-private materialization seam. Implementations may
// return only a context snapshot whose remote retrieval, disclosure and cache
// effects were already governed and settled outside the interaction loop. A
// ContextPort implementation must not use Prepare as an unjournaled network or
// persistence backdoor.
type ContextPort interface {
	Prepare(context.Context, ContextRequest) (ContextSnapshot, error)
}

type TurnState string

const (
	TurnCompleted      TurnState = "completed"
	TurnActionRequired TurnState = "action_required"
	TurnInputRequired  TurnState = "input_required"
)

type ModelTurnRequest struct {
	Run          contract.RunRef            `json:"run"`
	Input        runtimeports.OpaquePayload `json:"input"`
	Context      ContextSnapshot            `json:"context"`
	ActionResult *contract.ActionResult     `json:"action_result,omitempty"`
	Intent       core.EffectIntent          `json:"intent"`
	Fence        core.ExecutionFence        `json:"fence"`
}

func (r ModelTurnRequest) Validate(now time.Time) error {
	if err := r.Run.Validate(); err != nil {
		return err
	}
	if err := contract.ValidateOpaque(r.Input); err != nil {
		return err
	}
	if err := r.Context.Validate(); err != nil {
		return err
	}
	if r.ActionResult != nil {
		if err := r.ActionResult.Validate(); err != nil {
			return err
		}
	}
	return core.ValidateEffectDispatch(r.Intent, r.Fence, core.CurrentFenceFacts{
		Scope: r.Run.Scope, CapabilityGrantDigest: r.Fence.CapabilityGrantDigest,
	}, now)
}

type ModelTurnResult struct {
	State            TurnState                   `json:"state"`
	Output           *runtimeports.OpaquePayload `json:"output,omitempty"`
	Action           *contract.ActionRequest     `json:"action,omitempty"`
	NativeSessionRef string                      `json:"native_session_ref"`
	EvidenceDigest   core.Digest                 `json:"evidence_digest"`
}

func (r ModelTurnResult) Validate() error {
	if strings.TrimSpace(r.NativeSessionRef) == "" || len(r.NativeSessionRef) > contract.MaxReferenceBytes {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "model turn native session reference is required")
	}
	if err := r.EvidenceDigest.Validate(); err != nil {
		return err
	}
	switch r.State {
	case TurnCompleted:
		if r.Output == nil || r.Action != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "completed model turn requires only output")
		}
		return contract.ValidateOpaque(*r.Output)
	case TurnActionRequired:
		if r.Action == nil || r.Output != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "action model turn requires exactly one action")
		}
		return r.Action.Validate()
	case TurnInputRequired:
		if r.Action != nil || r.Output != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "input model turn cannot claim output or action")
		}
		return nil
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "unknown model turn state")
	}
}

type ModelTurnPort interface {
	Invoke(context.Context, ModelTurnRequest) (ModelTurnResult, error)
}

// EventCandidatePort accepts source-ordered Harness observations. It does not
// assign or imply a Runtime ledger position.
type EventCandidatePort interface {
	AppendCandidate(context.Context, contract.Event) error
}

// EventCandidateJournalPort is the fully controlled Harness source journal
// seam. Append implementations may lose a reply after persistence; callers
// recover only by inspecting the exact immutable source key.
type EventCandidateJournalPort interface {
	EventCandidatePort
	InspectCandidate(context.Context, string, core.Epoch, uint64) (contract.Event, error)
}

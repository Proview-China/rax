package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type AgentActivationPortV2 interface {
	StartOrInspectAgentActivationV2(context.Context, contract.AgentActivationStartRequestV2) (contract.AgentActivationResultV2, error)
	InspectAgentActivationV2(context.Context, contract.AgentActivationStartRequestV2) (contract.AgentActivationResultV2, error)
}

// AgentActivationStepPreparationV2 contains only Application-owned
// coordination coordinates. The Owner adapter reads its authoritative facts
// and returns a sealed request with exact dispatch refs where required.
type AgentActivationStepPreparationV2 struct {
	Coordination              contract.AgentActivationCoordinationRefV2 `json:"coordination_ref"`
	InvocationSequence        uint32                                    `json:"invocation_sequence"`
	InvocationEventDigest     core.Digest                               `json:"invocation_event_digest"`
	Step                      contract.AgentActivationStepV2            `json:"step"`
	Predecessor               *contract.AgentActivationStepResultRefV2  `json:"predecessor,omitempty"`
	Start                     contract.AgentActivationStartRequestV2    `json:"start"`
	RequestedNotAfterUnixNano int64                                     `json:"requested_not_after_unix_nano"`
}

func (p AgentActivationStepPreparationV2) Validate() error {
	if p.Coordination.Validate() != nil || p.InvocationSequence == 0 || p.InvocationEventDigest.Validate() != nil || p.Step.Validate() != nil || p.Start.Validate() != nil || p.Coordination.ActivationID != p.Start.ActivationID || p.Coordination.StartRequestDigest != p.Start.RequestDigest || p.RequestedNotAfterUnixNano != p.Start.RequestedNotAfterUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent activation V2 step preparation is incomplete")
	}
	if p.Step == contract.AgentActivationPreflightV2 {
		if p.Predecessor != nil {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Preflight preparation has a predecessor")
		}
	} else if p.Predecessor == nil || p.Predecessor.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent activation V2 preparation predecessor is missing")
	}
	return nil
}

// AgentActivationStepPortV2 is stable-attempt start-or-inspect. Prepare is
// read-only and may not dispatch the Owner operation.
type AgentActivationStepPortV2 interface {
	PrepareAgentActivationStepV2(context.Context, AgentActivationStepPreparationV2) (contract.AgentActivationStepRequestV2, error)
	StartOrInspectAgentActivationStepV2(context.Context, contract.AgentActivationStepRequestV2) (contract.AgentActivationStepResultV2, error)
	InspectAgentActivationStepV2(context.Context, contract.AgentActivationStepRequestV2) (contract.AgentActivationStepResultV2, error)
}

type AgentActivationStepPortsV2 struct {
	Preflight        AgentActivationStepPortV2
	Snapshot         AgentActivationStepPortV2
	IdentityBudget   AgentActivationStepPortV2
	SandboxAllocate  AgentActivationStepPortV2
	ActivationCommit AgentActivationStepPortV2
	SandboxActivate  AgentActivationStepPortV2
	ExecutionOpen    AgentActivationStepPortV2
	ReadyInspect     AgentActivationStepPortV2
}

func (p AgentActivationStepPortsV2) OrderedV2() []AgentActivationStepPortV2 {
	return []AgentActivationStepPortV2{p.Preflight, p.Snapshot, p.IdentityBudget, p.SandboxAllocate, p.ActivationCommit, p.SandboxActivate, p.ExecutionOpen, p.ReadyInspect}
}

type AgentActivationCoordinationCreateReceiptV2 struct {
	Fact    contract.AgentActivationCoordinationFactV2 `json:"fact"`
	Created bool                                       `json:"created"`
}

type AgentActivationCoordinationCASRequestV2 struct {
	ActivationID     string                                     `json:"activation_id"`
	ExpectedRevision core.Revision                              `json:"expected_revision"`
	ExpectedDigest   core.Digest                                `json:"expected_digest"`
	Next             contract.AgentActivationCoordinationFactV2 `json:"next"`
}

func (r AgentActivationCoordinationCASRequestV2) Validate() error {
	if r.ActivationID == "" || r.ExpectedRevision == 0 || r.ExpectedDigest.Validate() != nil || r.Next.ActivationID != r.ActivationID || r.Next.Revision != r.ExpectedRevision+1 || r.Next.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "Agent activation V2 CAS request is incomplete")
	}
	return nil
}

type AgentActivationCoordinationCASReceiptV2 struct {
	Fact    contract.AgentActivationCoordinationFactV2 `json:"fact"`
	Applied bool                                       `json:"applied"`
}

type AgentActivationCoordinationCurrentReaderV2 interface {
	InspectAgentActivationCoordinationV2(context.Context, string) (contract.AgentActivationCoordinationFactV2, error)
}

type AgentActivationCoordinationFactPortV2 interface {
	AgentActivationCoordinationCurrentReaderV2
	CreateAgentActivationCoordinationV2(context.Context, contract.AgentActivationCoordinationFactV2) (AgentActivationCoordinationCreateReceiptV2, error)
	CompareAndSwapAgentActivationCoordinationV2(context.Context, AgentActivationCoordinationCASRequestV2) (AgentActivationCoordinationCASReceiptV2, error)
}

package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// AgentLifecyclePortV1 is the narrow Application-owned production lifecycle
// seam. Start and Stop are stable-attempt start-or-inspect operations; only the
// explicit Inspect method may recover an unknown Start reply.
type AgentLifecyclePortV1 interface {
	AgentActivationPortV1
	AgentTerminationPortV1
}

type AgentActivationPortV1 interface {
	StartOrInspectAgentActivationV1(context.Context, contract.AgentActivationStartRequestV1) (contract.AgentActivationResultV1, error)
	InspectAgentActivationV1(context.Context, contract.AgentActivationStartRequestV1) (contract.AgentActivationResultV1, error)
}

type AgentTerminationPortV1 interface {
	StopOrInspectAgentV1(context.Context, contract.AgentTerminationRequestV1) (contract.AgentTerminationResultV1, error)
}

// AgentActivationStepPortV1 is a neutral start-or-inspect seam. Each configured
// dependency owns exactly one step and returns only public exact coordinates.
type AgentActivationStepPortV1 interface {
	StartOrInspectAgentActivationStepV1(context.Context, contract.AgentActivationStepRequestV1) (contract.AgentActivationStepResultV1, error)
	InspectAgentActivationStepV1(context.Context, contract.AgentActivationStepRequestV1) (contract.AgentActivationStepResultV1, error)
}

type AgentActivationStepPortsV1 struct {
	Preflight        AgentActivationStepPortV1
	Snapshot         AgentActivationStepPortV1
	IdentityBudget   AgentActivationStepPortV1
	SandboxAllocate  AgentActivationStepPortV1
	ActivationCommit AgentActivationStepPortV1
	SandboxActivate  AgentActivationStepPortV1
	ExecutionOpen    AgentActivationStepPortV1
	ReadyInspect     AgentActivationStepPortV1
}

func (p AgentActivationStepPortsV1) OrderedV1() []AgentActivationStepPortV1 {
	return []AgentActivationStepPortV1{p.Preflight, p.Snapshot, p.IdentityBudget, p.SandboxAllocate, p.ActivationCommit, p.SandboxActivate, p.ExecutionOpen, p.ReadyInspect}
}

type AgentActivationCoordinationCASRequestV1 struct {
	ActivationID     string                                     `json:"activation_id"`
	ExpectedRevision core.Revision                              `json:"expected_revision"`
	ExpectedDigest   core.Digest                                `json:"expected_digest"`
	Next             contract.AgentActivationCoordinationFactV1 `json:"next"`
}

type AgentActivationCoordinationFactPortV1 interface {
	EnsureAgentActivationCoordinationV1(context.Context, contract.AgentActivationCoordinationFactV1) (contract.AgentActivationCoordinationFactV1, error)
	InspectAgentActivationCoordinationV1(context.Context, string) (contract.AgentActivationCoordinationFactV1, error)
	CompareAndSwapAgentActivationCoordinationV1(context.Context, AgentActivationCoordinationCASRequestV1) (contract.AgentActivationCoordinationFactV1, error)
}

type AgentLifecycleFactCASRequestV1 struct {
	LifecycleID      string                        `json:"lifecycle_id"`
	ExpectedRevision core.Revision                 `json:"expected_revision"`
	ExpectedDigest   core.Digest                   `json:"expected_digest"`
	Next             contract.AgentLifecycleFactV1 `json:"next"`
}

// AgentLifecycleFactPortV1 is an additive durable aggregate seam. It does not
// change the existing AgentLifecyclePortV1 service API.
type AgentLifecycleFactPortV1 interface {
	EnsureAgentLifecycleFactV1(context.Context, contract.AgentLifecycleFactV1) (contract.AgentLifecycleFactV1, error)
	InspectAgentLifecycleFactV1(context.Context, string) (contract.AgentLifecycleFactV1, error)
	CompareAndSwapAgentLifecycleFactV1(context.Context, AgentLifecycleFactCASRequestV1) (contract.AgentLifecycleFactV1, error)
}

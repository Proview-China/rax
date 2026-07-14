package ports

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// ExecutionPort is the only Runtime-facing execution seam for future Harness
// routes and the existing model-invoker. An adapter may expose only capabilities
// it can actually control and observe; Runtime does not infer missing behavior.
type ExecutionPort interface {
	Describe(context.Context) (ComponentDescriptor, error)
	Preflight(context.Context, ExecutionPreflightRequest) (ExecutionPreflightReport, error)
	Open(context.Context, ExecutionOpenRequest) (ExecutionEndpointRef, error)
	Inspect(context.Context, ExecutionInspectRequest) (ExecutionObservation, error)
	Control(context.Context, ExecutionControlRequest) (ExecutionObservation, error)
	Close(context.Context, ExecutionCloseRequest) (ExecutionObservation, error)
}

type ExecutionPreflightRequest struct {
	ProposedScope     core.ExecutionScope `json:"proposed_scope"`
	RequirementDigest core.Digest         `json:"requirement_digest"`
	ProbeBudget       ProbeBudget         `json:"probe_budget"`
}

type ProbeBudget struct {
	MaxRequests      uint32        `json:"max_requests"`
	MaxDuration      time.Duration `json:"max_duration"`
	PossibleCharge   bool          `json:"possible_charge"`
	PossibleMutation bool          `json:"possible_mutation"`
	CleanupContract  string        `json:"cleanup_contract,omitempty"`
}

type ExecutionPreflightReport struct {
	Accepted          bool                `json:"accepted"`
	Descriptor        ComponentDescriptor `json:"descriptor"`
	RequirementDigest core.Digest         `json:"requirement_digest"`
	EvidenceDigest    core.Digest         `json:"evidence_digest"`
	EvidenceExpiry    time.Time           `json:"evidence_expiry"`
	PossibleResidual  bool                `json:"possible_residual"`
	ResidualRef       string              `json:"residual_ref,omitempty"`
}

type ExecutionOpenRequest struct {
	Scope             core.ExecutionScope `json:"scope"`
	RequirementDigest core.Digest         `json:"requirement_digest"`
	Intent            core.EffectIntent   `json:"intent"`
	Fence             core.ExecutionFence `json:"fence"`
}

type ExecutionEndpointRef struct {
	ComponentID string      `json:"component_id"`
	EndpointID  string      `json:"endpoint_id"`
	Digest      core.Digest `json:"digest"`
}

type ExecutionInspectRequest struct {
	Scope       core.ExecutionScope  `json:"scope"`
	Endpoint    ExecutionEndpointRef `json:"endpoint"`
	InspectKind string               `json:"inspect_kind"`
}

type ExecutionControlRequest struct {
	Scope       core.ExecutionScope  `json:"scope"`
	Endpoint    ExecutionEndpointRef `json:"endpoint"`
	CommandKind string               `json:"command_kind"`
	Payload     OpaquePayload        `json:"payload"`
	Fence       *core.ExecutionFence `json:"fence,omitempty"`
}

type ExecutionCloseRequest struct {
	Scope    core.ExecutionScope  `json:"scope"`
	Endpoint ExecutionEndpointRef `json:"endpoint"`
	Reason   string               `json:"reason"`
	Intent   core.EffectIntent    `json:"intent"`
	Fence    core.ExecutionFence  `json:"fence"`
}

// ExecutionObservation never becomes an authoritative Runtime fact solely by
// being signed or returned successfully.
type ExecutionObservation struct {
	SourceComponentID string        `json:"source_component_id"`
	SourceEpoch       core.Epoch    `json:"source_epoch"`
	ObservationKind   string        `json:"observation_kind"`
	Payload           OpaquePayload `json:"payload"`
	ObservedAt        time.Time     `json:"observed_at"`
}

package ports

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// ProfilePort and AssemblyPort are upstream compilation seams. Runtime only
// consumes their immutable resolved outputs and never merges profiles itself.
type ProfilePort interface {
	Describe(context.Context) (ComponentDescriptor, error)
	Resolve(context.Context, ProfileResolveRequest) (ProfileResolveResult, error)
}

type ProfileResolveRequest struct {
	DefinitionDigest core.Digest   `json:"definition_digest"`
	Selection        OpaquePayload `json:"selection"`
}

type ProfileResolveResult struct {
	ProfileRef string      `json:"profile_ref"`
	Digest     core.Digest `json:"digest"`
	Residuals  []string    `json:"residuals,omitempty"`
}

type AssemblyPort interface {
	Describe(context.Context) (ComponentDescriptor, error)
	Assemble(context.Context, AssemblyRequest) (ResolvedAgentPlan, error)
}

type AssemblyRequest struct {
	DefinitionDigest core.Digest `json:"definition_digest"`
	ProfileDigest    core.Digest `json:"profile_digest"`
	FactsDigest      core.Digest `json:"facts_digest"`
}

type ActionPort interface {
	Describe(context.Context) (ComponentDescriptor, error)
	Snapshot(context.Context, CapabilitySnapshotRequest) (CapabilitySnapshot, error)
	Execute(context.Context, ActionRequest) (ActionObservation, error)
	Inspect(context.Context, ActionInspectRequest) (ActionObservation, error)
	Cancel(context.Context, ActionCancelRequest) (ActionObservation, error)
}

type ToolPort interface{ ActionPort }
type MCPPort interface{ ActionPort }

type CapabilitySnapshotRequest struct {
	Scope           core.ExecutionScope `json:"scope"`
	AuthorityDigest core.Digest         `json:"authority_digest"`
}

type CapabilitySnapshot struct {
	Ref            string      `json:"ref"`
	SchemaDigest   core.Digest `json:"schema_digest"`
	EvidenceDigest core.Digest `json:"evidence_digest"`
	ExpiresAt      time.Time   `json:"expires_at"`
}

type ActionRequest struct {
	Scope      core.ExecutionScope `json:"scope"`
	ActionRef  string              `json:"action_ref"`
	Capability string              `json:"capability"`
	Payload    OpaquePayload       `json:"payload"`
	Intent     core.EffectIntent   `json:"intent"`
	Fence      core.ExecutionFence `json:"fence"`
}

type ActionInspectRequest struct {
	Scope     core.ExecutionScope `json:"scope"`
	ActionRef string              `json:"action_ref"`
	IntentID  core.EffectIntentID `json:"effect_intent_id"`
}

type ActionCancelRequest struct {
	Scope     core.ExecutionScope `json:"scope"`
	ActionRef string              `json:"action_ref"`
	Intent    core.EffectIntent   `json:"intent"`
	Fence     core.ExecutionFence `json:"fence"`
}

type ActionObservation struct {
	ActionRef      string      `json:"action_ref"`
	State          string      `json:"state"`
	ReceiptRef     string      `json:"receipt_ref,omitempty"`
	EvidenceDigest core.Digest `json:"evidence_digest"`
	ObservedAt     time.Time   `json:"observed_at"`
}

type MemoryPort interface {
	StatePort
	Retrieve(context.Context, StateQueryRequest) (StateQueryResult, error)
}

type AssetPort interface{ StatePort }

type KnowledgePort interface {
	Describe(context.Context) (ComponentDescriptor, error)
	Query(context.Context, StateQueryRequest) (StateQueryResult, error)
}

type StateQueryRequest struct {
	Scope           core.ExecutionScope `json:"scope"`
	QueryDigest     core.Digest         `json:"query_digest"`
	AuthorityDigest core.Digest         `json:"authority_digest"`
}

type StateQueryResult struct {
	SnapshotRef    string      `json:"snapshot_ref"`
	ResultDigest   core.Digest `json:"result_digest"`
	EvidenceDigest core.Digest `json:"evidence_digest"`
}

type OrganizationPort interface {
	Describe(context.Context) (ComponentDescriptor, error)
	InspectAuthority(context.Context, AuthorityRequest) (AuthorityObservation, error)
}

type ReviewPort interface {
	Describe(context.Context) (ComponentDescriptor, error)
	RequestVerdict(context.Context, VerdictRequest) (VerdictObservation, error)
}

type ManagementPort interface {
	Describe(context.Context) (ComponentDescriptor, error)
	Propose(context.Context, ManagementRequest) (ManagementObservation, error)
}

type ManagementRequest struct {
	Scope               core.ExecutionScope `json:"scope"`
	DesiredStateDigest  core.Digest         `json:"desired_state_digest"`
	ObservedStateDigest core.Digest         `json:"observed_state_digest"`
	VerdictRefs         []string            `json:"verdict_refs,omitempty"`
}

type ManagementObservation struct {
	ControlIntentRef string      `json:"control_intent_ref"`
	IntentDigest     core.Digest `json:"intent_digest"`
	ObservedAt       time.Time   `json:"observed_at"`
}

type TimelinePort interface {
	Describe(context.Context) (ComponentDescriptor, error)
	Append(context.Context, TimelineAppendRequest) (core.TimelinePoint, error)
	Read(context.Context, TimelineReadRequest) ([]core.TimelinePoint, error)
}

type TimelineAppendRequest struct {
	Scope          core.ExecutionScope `json:"scope"`
	EventID        string              `json:"event_id"`
	SourceID       string              `json:"source_id"`
	SourceEpoch    core.Epoch          `json:"source_epoch"`
	SourceSequence uint64              `json:"source_sequence"`
	CausationID    string              `json:"causation_id"`
	CorrelationID  string              `json:"correlation_id"`
	ObservedAt     time.Time           `json:"observed_at"`
	PayloadDigest  core.Digest         `json:"payload_digest"`
}

type TimelineReadRequest struct {
	Scope         core.ExecutionScope `json:"scope"`
	AfterSequence uint64              `json:"after_sequence"`
	Limit         uint32              `json:"limit"`
}

type CheckpointParticipantPort interface {
	Describe(context.Context) (ComponentDescriptor, error)
	PrepareCheckpoint(context.Context, CheckpointPrepareRequest) (CheckpointParticipantReport, error)
	CommitCheckpoint(context.Context, CheckpointCommitRequest) (CheckpointParticipantReport, error)
	AbortCheckpoint(context.Context, CheckpointAbortRequest) (CheckpointParticipantReport, error)
	RestoreCheckpoint(context.Context, CheckpointRestoreRequest) (CheckpointParticipantReport, error)
}

type CheckpointPrepareRequest struct {
	BarrierID string                `json:"barrier_id"`
	Epoch     core.Epoch            `json:"checkpoint_epoch"`
	Scope     core.ExecutionScope   `json:"scope"`
	Effects   core.EffectWatermarks `json:"effect_watermarks"`
}

type CheckpointCommitRequest struct {
	BarrierID string     `json:"barrier_id"`
	Epoch     core.Epoch `json:"checkpoint_epoch"`
}

type CheckpointAbortRequest struct {
	BarrierID string     `json:"barrier_id"`
	Epoch     core.Epoch `json:"checkpoint_epoch"`
	Reason    string     `json:"reason"`
}

type CheckpointRestoreRequest struct {
	CheckpointID   string              `json:"checkpoint_id"`
	SnapshotRef    string              `json:"snapshot_ref"`
	SnapshotDigest core.Digest         `json:"snapshot_digest"`
	NewScope       core.ExecutionScope `json:"new_scope"`
}

type CheckpointParticipantReport struct {
	ComponentID    string                          `json:"component_id"`
	ComponentKind  ComponentKind                   `json:"component_kind"`
	State          core.CheckpointParticipantState `json:"state"`
	SnapshotRef    string                          `json:"snapshot_ref,omitempty"`
	SnapshotDigest core.Digest                     `json:"snapshot_digest,omitempty"`
	EvidenceDigest core.Digest                     `json:"evidence_digest"`
	ObservedAt     time.Time                       `json:"observed_at"`
}

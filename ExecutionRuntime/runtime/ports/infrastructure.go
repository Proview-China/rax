package ports

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type EnvironmentPort interface {
	Describe(context.Context) (ComponentDescriptor, error)
	Allocate(context.Context, SandboxAllocateRequest) (SandboxLeaseObservation, error)
	Activate(context.Context, SandboxActivateRequest) (SandboxLeaseObservation, error)
	Inspect(context.Context, core.SandboxLeaseRef) (SandboxLeaseObservation, error)
	Fence(context.Context, SandboxFenceRequest) (SandboxLeaseObservation, error)
	Release(context.Context, SandboxReleaseRequest) (SandboxLeaseObservation, error)
}

type SandboxAllocateRequest struct {
	ProposedInstance  core.InstanceRef    `json:"proposed_instance"`
	RequirementDigest core.Digest         `json:"requirement_digest"`
	FenceEpoch        core.Epoch          `json:"fence_epoch"`
	Intent            core.EffectIntent   `json:"intent"`
	Fence             core.ExecutionFence `json:"fence"`
}

type SandboxActivateRequest struct {
	Scope  core.ExecutionScope `json:"scope"`
	Intent core.EffectIntent   `json:"intent"`
	Fence  core.ExecutionFence `json:"fence"`
}

type SandboxFenceRequest struct {
	Lease  core.SandboxLeaseRef `json:"lease"`
	Reason string               `json:"reason"`
}

type SandboxReleaseRequest struct {
	Lease  core.SandboxLeaseRef `json:"lease"`
	Intent core.EffectIntent    `json:"intent"`
	Fence  core.ExecutionFence  `json:"fence"`
}

type SandboxLeaseObservation struct {
	Lease       core.SandboxLeaseRef `json:"lease"`
	State       string               `json:"state"`
	EvidenceRef string               `json:"evidence_ref"`
	ObservedAt  time.Time            `json:"observed_at"`
}

type BudgetAuthorityPort interface {
	Describe(context.Context) (ComponentDescriptor, error)
	Reserve(context.Context, BudgetReserveRequest) (BudgetObservation, error)
	Commit(context.Context, BudgetCommitRequest) (BudgetObservation, error)
	Release(context.Context, BudgetReleaseRequest) (BudgetObservation, error)
	Inspect(context.Context, string) (BudgetObservation, error)
}

type BudgetReserveRequest struct {
	IntentID      core.EffectIntentID `json:"effect_intent_id"`
	PolicyDigest  core.Digest         `json:"policy_digest"`
	ExecutableCap uint64              `json:"executable_cap"`
	Unit          string              `json:"unit"`
}

type BudgetCommitRequest struct {
	ReservationRef string `json:"reservation_ref"`
	ActualUsage    uint64 `json:"actual_usage"`
}

type BudgetReleaseRequest struct {
	ReservationRef string            `json:"reservation_ref"`
	Intent         core.EffectIntent `json:"intent"`
}

type BudgetObservation struct {
	ReservationRef string      `json:"reservation_ref"`
	Status         string      `json:"status"`
	Reserved       uint64      `json:"reserved"`
	Committed      uint64      `json:"committed"`
	EvidenceDigest core.Digest `json:"evidence_digest"`
}

type EvidencePort interface {
	Describe(context.Context) (ComponentDescriptor, error)
	AppendIntent(context.Context, EvidenceIntentRecord) (EvidenceRecordRef, error)
	AppendObservation(context.Context, EvidenceObservationRecord) (EvidenceRecordRef, error)
	Read(context.Context, EvidenceRecordRef) (EvidenceRecord, error)
}

type EvidenceIntentRecord struct {
	Scope         core.ExecutionScope `json:"scope"`
	Kind          string              `json:"kind"`
	PayloadDigest core.Digest         `json:"payload_digest"`
	CausationID   string              `json:"causation_id"`
}

type EvidenceObservationRecord struct {
	SourceID       string      `json:"source_id"`
	SourceEpoch    core.Epoch  `json:"source_epoch"`
	SourceSequence uint64      `json:"source_sequence"`
	PayloadDigest  core.Digest `json:"payload_digest"`
	CausationID    string      `json:"causation_id"`
}

type EvidenceRecordRef struct {
	Scope    string `json:"scope"`
	Sequence uint64 `json:"sequence"`
}

type EvidenceRecord struct {
	Ref            EvidenceRecordRef `json:"ref"`
	Classification string            `json:"classification"`
	PayloadDigest  core.Digest       `json:"payload_digest"`
	RecordedAt     time.Time         `json:"recorded_at"`
}

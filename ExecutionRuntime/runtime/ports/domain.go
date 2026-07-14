package ports

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type ContextPort interface {
	Describe(context.Context) (ComponentDescriptor, error)
	Prepare(context.Context, ContextRequest) (ContextPlanRef, error)
	Inspect(context.Context, ContextPlanRef) (ContextObservation, error)
}

type ContextRequest struct {
	Scope             core.ExecutionScope `json:"scope"`
	RequirementDigest core.Digest         `json:"requirement_digest"`
	AuthorityDigest   core.Digest         `json:"authority_digest"`
}

type ContextPlanRef struct {
	ContextPackageRef string      `json:"context_package_ref"`
	CachePlanRef      string      `json:"cache_plan_ref,omitempty"`
	Digest            core.Digest `json:"digest"`
}

type ContextObservation struct {
	PlanRef     ContextPlanRef `json:"plan_ref"`
	EvidenceRef string         `json:"evidence_ref"`
	ObservedAt  time.Time      `json:"observed_at"`
}

type EffectPort interface {
	Describe(context.Context) (ComponentDescriptor, error)
	Dispatch(context.Context, EffectDispatchRequest) (EffectDispatchResult, error)
	Inspect(context.Context, EffectInspectRequest) (EffectSettlementObservation, error)
}

type EffectDispatchRequest struct {
	Intent core.EffectIntent   `json:"intent"`
	Fence  core.ExecutionFence `json:"fence"`
}

type EffectDispatchResult struct {
	ProviderOperationRef string      `json:"provider_operation_ref,omitempty"`
	ReceiptRef           string      `json:"receipt_ref,omitempty"`
	Outcome              string      `json:"outcome"`
	EvidenceDigest       core.Digest `json:"evidence_digest"`
}

type EffectInspectRequest struct {
	IntentID             core.EffectIntentID `json:"effect_intent_id"`
	IntentRevision       core.Revision       `json:"effect_intent_revision"`
	ProviderOperationRef string              `json:"provider_operation_ref,omitempty"`
}

type EffectSettlementObservation struct {
	Outcome        string      `json:"outcome"`
	ReceiptRef     string      `json:"receipt_ref,omitempty"`
	EvidenceDigest core.Digest `json:"evidence_digest"`
	ObservedAt     time.Time   `json:"observed_at"`
}

type StatePort interface {
	Describe(context.Context) (ComponentDescriptor, error)
	SubmitCandidate(context.Context, CandidateRequest) (CandidateRef, error)
	Commit(context.Context, CommitRequest) (CommitObservation, error)
	InspectCommit(context.Context, CommitInspectRequest) (CommitObservation, error)
}

type CandidateRequest struct {
	Scope         core.ExecutionScope `json:"scope"`
	Kind          string              `json:"kind"`
	ContentDigest core.Digest         `json:"content_digest"`
	EvidenceRef   string              `json:"evidence_ref"`
}

type CandidateRef struct {
	ID     string      `json:"id"`
	Kind   string      `json:"kind"`
	Digest core.Digest `json:"digest"`
}

type CommitRequest struct {
	Candidate  CandidateRef        `json:"candidate"`
	VerdictRef string              `json:"verdict_ref"`
	Intent     core.EffectIntent   `json:"intent"`
	Fence      core.ExecutionFence `json:"fence"`
}

type CommitInspectRequest struct {
	CandidateID    string              `json:"candidate_id"`
	EffectIntentID core.EffectIntentID `json:"effect_intent_id"`
}

type CommitObservation struct {
	Status         string      `json:"status"`
	ReceiptRef     string      `json:"receipt_ref,omitempty"`
	EvidenceDigest core.Digest `json:"evidence_digest"`
}

type GovernancePort interface {
	Describe(context.Context) (ComponentDescriptor, error)
	InspectAuthority(context.Context, AuthorityRequest) (AuthorityObservation, error)
	RequestVerdict(context.Context, VerdictRequest) (VerdictObservation, error)
}

type AuthorityRequest struct {
	Scope        core.ExecutionScope `json:"scope"`
	ActionDigest core.Digest         `json:"action_digest"`
}

type AuthorityObservation struct {
	Allowed        bool        `json:"allowed"`
	AuthorityEpoch core.Epoch  `json:"authority_epoch"`
	ScopeDigest    core.Digest `json:"scope_digest"`
	ExpiresAt      time.Time   `json:"expires_at"`
}

type VerdictRequest struct {
	Candidate    CandidateRef `json:"candidate"`
	PolicyDigest core.Digest  `json:"policy_digest"`
}

type VerdictObservation struct {
	VerdictRef string      `json:"verdict_ref"`
	Status     string      `json:"status"`
	Digest     core.Digest `json:"digest"`
}

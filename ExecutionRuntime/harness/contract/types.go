// Package contract defines versioned, provider-neutral Harness values.
package contract

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const Version = "praxis.harness/v1alpha1"

const (
	MaxOpaquePayloadBytes = 1 << 20
	MaxOpaqueSchemaBytes  = 256
	MaxReferenceBytes     = 1024
	MaxManifestListItems  = 256
)

type ControlCapabilities struct {
	Cancel              bool `json:"cancel"`
	ProvideInput        bool `json:"provide_input"`
	ProvideActionResult bool `json:"provide_action_result"`
	Checkpoint          bool `json:"checkpoint"`
}

type BootstrapPlan struct {
	ID                              string                        `json:"bootstrap_plan_id"`
	Version                         string                        `json:"version"`
	ResolvedPlanDigest              core.Digest                   `json:"resolved_plan_digest"`
	ProfileDigest                   core.Digest                   `json:"profile_digest"`
	RuntimePolicyDigest             core.Digest                   `json:"runtime_policy_digest"`
	HarnessStackDigest              core.Digest                   `json:"harness_stack_digest"`
	SemanticRouteDigest             core.Digest                   `json:"semantic_route_digest"`
	ExpectedInjectionManifestDigest core.Digest                   `json:"expected_injection_manifest_digest"`
	ContextPlanDigest               core.Digest                   `json:"context_plan_digest"`
	ToolSurfaceDigest               core.Digest                   `json:"tool_surface_digest"`
	CapabilityGrantDigest           core.Digest                   `json:"capability_grant_digest"`
	MinimumConformance              runtimeports.ConformanceLevel `json:"minimum_conformance"`
	Controls                        ControlCapabilities           `json:"controls"`
	AllowedResiduals                []string                      `json:"allowed_residuals,omitempty"`
	EvidenceExpiresAt               time.Time                     `json:"evidence_expires_at"`
}

func (p BootstrapPlan) Validate(now time.Time) error {
	if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.Version) == "" || len(p.ID) > MaxReferenceBytes || len(p.Version) > MaxReferenceBytes {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "harness bootstrap plan identity and version are required")
	}
	for _, digest := range []core.Digest{
		p.ResolvedPlanDigest, p.ProfileDigest, p.RuntimePolicyDigest, p.HarnessStackDigest,
		p.SemanticRouteDigest, p.ExpectedInjectionManifestDigest, p.ContextPlanDigest,
		p.ToolSurfaceDigest, p.CapabilityGrantDigest,
	} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if p.MinimumConformance != runtimeports.ConformanceFullyControlled && p.MinimumConformance != runtimeports.ConformanceRestrictedControlled {
		return core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMismatch, "minimal harness requires a governed conformance level")
	}
	if now.IsZero() || !now.Before(p.EvidenceExpiresAt) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "harness bootstrap evidence has expired")
	}
	if len(p.AllowedResiduals) > MaxManifestListItems {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "harness residual list exceeds its bound")
	}
	seen := make(map[string]struct{}, len(p.AllowedResiduals))
	for _, residual := range p.AllowedResiduals {
		if strings.TrimSpace(residual) == "" || len(residual) > MaxReferenceBytes {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "harness residual cannot be empty")
		}
		if _, exists := seen[residual]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonPlanInvalid, "harness residual is duplicated")
		}
		seen[residual] = struct{}{}
	}
	return nil
}

type Manifest struct {
	ContractVersion   string                        `json:"contract_version"`
	ID                string                        `json:"id"`
	Version           string                        `json:"version"`
	ArtifactDigest    core.Digest                   `json:"artifact_digest"`
	Conformance       runtimeports.ConformanceLevel `json:"conformance"`
	Bootstrap         BootstrapPlan                 `json:"bootstrap"`
	Capabilities      []string                      `json:"capabilities"`
	OpaqueBoundaries  []string                      `json:"opaque_boundaries,omitempty"`
	EvidenceDigest    core.Digest                   `json:"evidence_digest"`
	EvidenceExpiresAt time.Time                     `json:"evidence_expires_at"`
}

// CloneManifest establishes the immutable handoff boundary expected by the
// Harness kernel and Runtime adapter.
func CloneManifest(manifest Manifest) Manifest {
	clone := manifest
	clone.Bootstrap.AllowedResiduals = append([]string(nil), manifest.Bootstrap.AllowedResiduals...)
	clone.Capabilities = append([]string(nil), manifest.Capabilities...)
	clone.OpaqueBoundaries = append([]string(nil), manifest.OpaqueBoundaries...)
	return clone
}

func (m Manifest) Validate(now time.Time) error {
	if m.ContractVersion != Version || strings.TrimSpace(m.ID) == "" || strings.TrimSpace(m.Version) == "" || len(m.ID) > MaxReferenceBytes || len(m.Version) > MaxReferenceBytes {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMismatch, "harness manifest identity and current contract version are required")
	}
	if m.Conformance != runtimeports.ConformanceFullyControlled && m.Conformance != runtimeports.ConformanceRestrictedControlled {
		return core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMismatch, "minimal harness cannot execute at this conformance level")
	}
	if m.Bootstrap.MinimumConformance == runtimeports.ConformanceFullyControlled && m.Conformance != runtimeports.ConformanceFullyControlled {
		return core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMismatch, "harness conformance is below the bootstrap requirement")
	}
	if err := m.Bootstrap.Validate(now); err != nil {
		return err
	}
	for _, digest := range []core.Digest{m.ArtifactDigest, m.EvidenceDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if m.EvidenceExpiresAt.IsZero() || !now.Before(m.EvidenceExpiresAt) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "harness manifest evidence has expired")
	}
	for _, values := range [][]string{m.Capabilities, m.OpaqueBoundaries} {
		if len(values) > MaxManifestListItems {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "harness manifest list exceeds its bound")
		}
		seen := make(map[string]struct{}, len(values))
		for _, value := range values {
			if strings.TrimSpace(value) == "" || len(value) > MaxReferenceBytes {
				return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "harness manifest list value cannot be empty")
			}
			if _, exists := seen[value]; exists {
				return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "harness manifest list value is duplicated")
			}
			seen[value] = struct{}{}
		}
	}
	return nil
}

type RunPhase string

const (
	RunStarting      RunPhase = "starting"
	RunRunning       RunPhase = "running"
	RunWaitingInput  RunPhase = "waiting_input"
	RunWaitingAction RunPhase = "waiting_action"
	RunReconciling   RunPhase = "waiting_reconciliation"
	RunCancelling    RunPhase = "cancelling"
	RunTerminal      RunPhase = "terminal"
)

type CompletionClaim string

const (
	ClaimCompleted     CompletionClaim = "completed"
	ClaimCancelled     CompletionClaim = "cancelled"
	ClaimFailed        CompletionClaim = "failed"
	ClaimIndeterminate CompletionClaim = "indeterminate"
)

type RunRef struct {
	Scope core.ExecutionScope `json:"scope"`
	RunID core.AgentRunID     `json:"run_id"`
}

func (r RunRef) Validate() error {
	if err := r.Scope.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(string(r.RunID)) == "" || len(r.RunID) > MaxReferenceBytes {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "harness run id is required")
	}
	return nil
}

type ActionRequest struct {
	Ref            string                     `json:"action_ref"`
	Capability     string                     `json:"capability"`
	Payload        runtimeports.OpaquePayload `json:"payload"`
	ReviewRequired bool                       `json:"review_required"`
}

func (a ActionRequest) Validate() error {
	if strings.TrimSpace(a.Ref) == "" || strings.TrimSpace(a.Capability) == "" || len(a.Ref) > MaxReferenceBytes || len(a.Capability) > MaxReferenceBytes {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "action reference and capability are required")
	}
	return ValidateOpaque(a.Payload)
}

type ActionResult struct {
	Ref     string                     `json:"action_ref"`
	Payload runtimeports.OpaquePayload `json:"payload"`
}

func (a ActionResult) Validate() error {
	if strings.TrimSpace(a.Ref) == "" || len(a.Ref) > MaxReferenceBytes {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "action result reference is required")
	}
	return ValidateOpaque(a.Payload)
}

type RunState struct {
	Ref            RunRef        `json:"ref"`
	Phase          RunPhase      `json:"phase"`
	Revision       core.Revision `json:"revision"`
	SourceSequence uint64        `json:"source_sequence"`
	// SessionRef is the Harness-stable Run session identity. A model/provider
	// native session is only an observation carried by model_turn_observed and
	// must never replace this value.
	SessionRef      string          `json:"session_ref"`
	PendingAction   *ActionRequest  `json:"pending_action,omitempty"`
	CompletionClaim CompletionClaim `json:"completion_claim,omitempty"`
	StartedAt       time.Time       `json:"started_at"`
	EndedAt         time.Time       `json:"ended_at,omitempty"`
}

func (s RunState) Validate() error {
	if err := s.Ref.Validate(); err != nil {
		return err
	}
	if s.Revision == 0 || strings.TrimSpace(s.SessionRef) == "" || len(s.SessionRef) > MaxReferenceBytes || s.StartedAt.IsZero() {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "harness run revision, session and start time are required")
	}
	switch s.Phase {
	case RunStarting, RunRunning, RunWaitingInput, RunReconciling, RunCancelling:
		if !s.EndedAt.IsZero() || s.CompletionClaim != "" || s.PendingAction != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "active harness run has terminal or action-only fields")
		}
	case RunWaitingAction:
		if !s.EndedAt.IsZero() || s.CompletionClaim != "" || s.PendingAction == nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "waiting action state requires exactly one pending action")
		}
		if err := s.PendingAction.Validate(); err != nil {
			return err
		}
	case RunTerminal:
		if s.EndedAt.IsZero() || s.EndedAt.Before(s.StartedAt) || !validClaim(s.CompletionClaim) || s.PendingAction != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "terminal harness run requires an ordered completion claim")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "unknown harness run phase")
	}
	return nil
}

type EventKind string

const (
	EventRunStarted           EventKind = "run_started"
	EventModelTurnStarted     EventKind = "model_turn_started"
	EventModelTurnObserved    EventKind = "model_turn_observed"
	EventModelTurnUncertain   EventKind = "model_turn_uncertain"
	EventModelOutput          EventKind = "model_output"
	EventActionRequested      EventKind = "action_requested"
	EventActionResultReceived EventKind = "action_result_received"
	EventInputRequested       EventKind = "input_requested"
	EventInputReceived        EventKind = "input_received"
	EventCancelRequested      EventKind = "cancel_requested"
	EventRunCompleted         EventKind = "run_completed"
	EventRunCancelled         EventKind = "run_cancelled"
	EventRunFailed            EventKind = "run_failed"
)

type Event struct {
	SourceComponentID string                     `json:"source_component_id"`
	SourceEpoch       core.Epoch                 `json:"source_epoch"`
	SourceSequence    uint64                     `json:"source_sequence"`
	RunID             core.AgentRunID            `json:"run_id"`
	Kind              EventKind                  `json:"kind"`
	Payload           runtimeports.OpaquePayload `json:"payload"`
	ObservedAt        time.Time                  `json:"observed_at"`
}

func (e Event) Validate() error {
	if strings.TrimSpace(e.SourceComponentID) == "" || len(e.SourceComponentID) > MaxReferenceBytes || e.SourceEpoch == 0 || e.SourceSequence == 0 || strings.TrimSpace(string(e.RunID)) == "" || len(e.RunID) > MaxReferenceBytes || !validEventKind(e.Kind) || e.ObservedAt.IsZero() {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "harness event identity, sequence, kind and time are required")
	}
	return ValidateOpaque(e.Payload)
}

type Snapshot struct {
	State        RunState    `json:"state"`
	EventsDigest core.Digest `json:"events_digest"`
	CapturedAt   time.Time   `json:"captured_at"`
}

func (s Snapshot) Validate() error {
	if err := s.State.Validate(); err != nil {
		return err
	}
	if err := s.EventsDigest.Validate(); err != nil {
		return err
	}
	if s.CapturedAt.IsZero() {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "harness snapshot capture time is required")
	}
	return nil
}

func ValidateOpaque(payload runtimeports.OpaquePayload) error {
	if strings.TrimSpace(payload.Schema) == "" || len(payload.Schema) > MaxOpaqueSchemaBytes {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "opaque payload schema is required")
	}
	if err := payload.Digest.Validate(); err != nil {
		return err
	}
	if len(payload.Payload) == 0 || len(payload.Payload) > MaxOpaquePayloadBytes || !json.Valid(payload.Payload) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "opaque payload must contain valid json")
	}
	digest, err := core.DigestJSON(json.RawMessage(payload.Payload))
	if err != nil {
		return err
	}
	if digest != payload.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "opaque payload digest does not match its content")
	}
	return nil
}

func CloneOpaque(payload runtimeports.OpaquePayload) runtimeports.OpaquePayload {
	clone := payload
	clone.Payload = append(json.RawMessage(nil), payload.Payload...)
	return clone
}

func validClaim(claim CompletionClaim) bool {
	switch claim {
	case ClaimCompleted, ClaimCancelled, ClaimFailed, ClaimIndeterminate:
		return true
	default:
		return false
	}
}

func validEventKind(kind EventKind) bool {
	switch kind {
	case EventRunStarted, EventModelTurnStarted, EventModelTurnObserved, EventModelTurnUncertain, EventModelOutput, EventActionRequested,
		EventActionResultReceived, EventInputRequested, EventInputReceived, EventCancelRequested,
		EventRunCompleted, EventRunCancelled, EventRunFailed:
		return true
	default:
		return false
	}
}

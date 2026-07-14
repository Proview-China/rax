// Package ports defines the versioned seams between the Runtime kernel and
// adjacent Praxis components. It intentionally contains no component logic.
package ports

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const ContractVersion = "praxis.runtime.ports/v1alpha1"

type ComponentKind string

const (
	ComponentHarness            ComponentKind = "harness"
	ComponentModelInvoker       ComponentKind = "model_invoker"
	ComponentContextEngine      ComponentKind = "context_engine"
	ComponentToolMCP            ComponentKind = "tool_mcp"
	ComponentState              ComponentKind = "state"
	ComponentGovernance         ComponentKind = "governance"
	ComponentSandbox            ComponentKind = "sandbox"
	ComponentBudget             ComponentKind = "budget_authority"
	ComponentEvidence           ComponentKind = "evidence"
	ComponentProfileSystem      ComponentKind = "profile_system"
	ComponentAgentAssembler     ComponentKind = "agent_assembler"
	ComponentToolEngine         ComponentKind = "tool_engine"
	ComponentMCPGateway         ComponentKind = "mcp_gateway"
	ComponentMemoryEngine       ComponentKind = "memory_engine"
	ComponentKnowledgeEngine    ComponentKind = "knowledge_engine"
	ComponentAssetManager       ComponentKind = "asset_manager"
	ComponentOrganizationEngine ComponentKind = "organization_engine"
	ComponentReviewEngine       ComponentKind = "review_engine"
	ComponentManagementEngine   ComponentKind = "management_engine"
	ComponentCheckpoint         ComponentKind = "checkpoint"
	ComponentTimeline           ComponentKind = "timeline"
)

type CapabilityState string

const (
	CapabilityDeclared  CapabilityState = "declared"
	CapabilityProbed    CapabilityState = "probed"
	CapabilityCertified CapabilityState = "certified"
	CapabilityBound     CapabilityState = "bound"
	CapabilityRevoked   CapabilityState = "revoked"
)

type ConformanceLevel string

const (
	ConformanceFullyControlled      ConformanceLevel = "fully_controlled"
	ConformanceRestrictedControlled ConformanceLevel = "restricted_controlled"
	ConformanceContainedObserveOnly ConformanceLevel = "contained_observe_only"
	ConformanceRejected             ConformanceLevel = "rejected"
)

type Capability struct {
	Name           string          `json:"name"`
	State          CapabilityState `json:"state"`
	EvidenceDigest core.Digest     `json:"evidence_digest"`
	EvidenceExpiry time.Time       `json:"evidence_expiry"`
}

type ComponentDescriptor struct {
	ID              string           `json:"id"`
	Kind            ComponentKind    `json:"kind"`
	Version         string           `json:"version"`
	ArtifactDigest  core.Digest      `json:"artifact_digest"`
	ContractVersion string           `json:"contract_version"`
	Conformance     ConformanceLevel `json:"conformance,omitempty"`
	Capabilities    []Capability     `json:"capabilities"`
}

func (d ComponentDescriptor) Validate() error {
	if strings.TrimSpace(d.ID) == "" || strings.TrimSpace(d.Version) == "" || d.ContractVersion != ContractVersion || !validComponentKind(d.Kind) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "component id, kind, version and current contract version are required")
	}
	if err := d.ArtifactDigest.Validate(); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(d.Capabilities))
	for _, capability := range d.Capabilities {
		if strings.TrimSpace(capability.Name) == "" || !validCapabilityState(capability.State) {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "capability name and state are required")
		}
		if _, exists := seen[capability.Name]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "duplicate capability")
		}
		seen[capability.Name] = struct{}{}
		if capability.State != CapabilityDeclared && capability.State != CapabilityRevoked {
			if err := capability.EvidenceDigest.Validate(); err != nil {
				return err
			}
			if capability.EvidenceExpiry.IsZero() {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "probed, certified or bound capability requires expiring evidence")
			}
		}
	}
	return nil
}

type OpaquePayload struct {
	Schema  string          `json:"schema"`
	Digest  core.Digest     `json:"digest"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// ValidateOpaquePayload verifies both the envelope and the exact JSON content
// digest without interpreting a component-owned schema.
func ValidateOpaquePayload(payload OpaquePayload) error {
	if strings.TrimSpace(payload.Schema) == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "opaque payload schema is required")
	}
	if err := payload.Digest.Validate(); err != nil {
		return err
	}
	if len(payload.Payload) == 0 || !json.Valid(payload.Payload) {
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

func validComponentKind(kind ComponentKind) bool {
	switch kind {
	case ComponentHarness, ComponentModelInvoker, ComponentContextEngine, ComponentToolMCP,
		ComponentState, ComponentGovernance, ComponentSandbox, ComponentBudget, ComponentEvidence,
		ComponentProfileSystem, ComponentAgentAssembler, ComponentToolEngine, ComponentMCPGateway,
		ComponentMemoryEngine, ComponentKnowledgeEngine, ComponentAssetManager,
		ComponentOrganizationEngine, ComponentReviewEngine, ComponentManagementEngine,
		ComponentCheckpoint, ComponentTimeline:
		return true
	default:
		return false
	}
}

func validCapabilityState(state CapabilityState) bool {
	switch state {
	case CapabilityDeclared, CapabilityProbed, CapabilityCertified, CapabilityBound, CapabilityRevoked:
		return true
	default:
		return false
	}
}

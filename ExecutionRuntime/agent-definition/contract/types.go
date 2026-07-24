package contract

import (
	"encoding/json"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	ContractVersionV1 = "praxis.agent.definition/v1"
	DigestDomainV1    = "praxis.agent.definition"
	DigestVersionV1   = "v1"

	MaxDefinitionEntriesV1 = 256
	MaxExtensionBytesV1    = 256 << 10
)

type ObjectRefV1 struct {
	ID       string        `json:"id" yaml:"id"`
	Revision core.Revision `json:"revision" yaml:"revision"`
	Digest   core.Digest   `json:"digest" yaml:"digest"`
}

type VersionRangeV1 struct {
	MinimumInclusive string `json:"minimum_inclusive" yaml:"minimum_inclusive"`
	MaximumExclusive string `json:"maximum_exclusive" yaml:"maximum_exclusive"`
}

type EffectiveWindowV1 struct {
	NotBeforeUnixNano int64 `json:"not_before_unix_nano" yaml:"not_before_unix_nano"`
	NotAfterUnixNano  int64 `json:"not_after_unix_nano" yaml:"not_after_unix_nano"`
}

type SupportModeV1 string

const SupportModeProductionV1 SupportModeV1 = "production"

type LocalityConstraintV1 string

const (
	LocalityHostControlPlaneV1   LocalityConstraintV1 = "host_control_plane"
	LocalityInstanceDataPlaneV1  LocalityConstraintV1 = "instance_data_plane"
	LocalityExternalStatePlaneV1 LocalityConstraintV1 = "external_state_plane"
	LocalityRemoteProviderV1     LocalityConstraintV1 = "remote_provider"
)

type ResidualPolicyV1 struct {
	Allowed         bool        `json:"allowed" yaml:"allowed"`
	InspectOwnerRef ObjectRefV1 `json:"inspect_owner_ref" yaml:"inspect_owner_ref"`
	CleanupOwnerRef ObjectRefV1 `json:"cleanup_owner_ref" yaml:"cleanup_owner_ref"`
}

type ComponentRequirementV1 struct {
	ComponentID          string               `json:"component_id" yaml:"component_id"`
	Kind                 string               `json:"kind" yaml:"kind"`
	SemanticVersion      VersionRangeV1       `json:"semantic_version" yaml:"semantic_version"`
	ContractName         string               `json:"contract_name" yaml:"contract_name"`
	ContractVersion      VersionRangeV1       `json:"contract_version" yaml:"contract_version"`
	RequiredCapabilities []string             `json:"required_capabilities" yaml:"required_capabilities"`
	Required             bool                 `json:"required" yaml:"required"`
	SupportMode          SupportModeV1        `json:"support_mode" yaml:"support_mode"`
	LocalityConstraint   LocalityConstraintV1 `json:"locality_constraint" yaml:"locality_constraint"`
	ResidualPolicy       ResidualPolicyV1     `json:"residual_policy" yaml:"residual_policy"`
	DependencyIDs        []string             `json:"dependency_ids" yaml:"dependency_ids"`
}

type PolicyRefsV1 struct {
	Runtime         ObjectRefV1 `json:"runtime" yaml:"runtime"`
	Authority       ObjectRefV1 `json:"authority" yaml:"authority"`
	Review          ObjectRefV1 `json:"review" yaml:"review"`
	Budget          ObjectRefV1 `json:"budget" yaml:"budget"`
	Sandbox         ObjectRefV1 `json:"sandbox" yaml:"sandbox"`
	Context         ObjectRefV1 `json:"context" yaml:"context"`
	Continuity      ObjectRefV1 `json:"continuity" yaml:"continuity"`
	ToolMCP         ObjectRefV1 `json:"tool_mcp" yaml:"tool_mcp"`
	MemoryKnowledge ObjectRefV1 `json:"memory_knowledge" yaml:"memory_knowledge"`
}

type SecretRefV1 struct {
	SecretID             string      `json:"secret_id" yaml:"secret_id"`
	Class                string      `json:"class" yaml:"class"`
	RequestedScopeDigest core.Digest `json:"requested_scope_digest" yaml:"requested_scope_digest"`
}

type SchemaRefV1 struct {
	Namespace     string      `json:"namespace" yaml:"namespace"`
	Name          string      `json:"name" yaml:"name"`
	Version       string      `json:"version" yaml:"version"`
	MediaType     string      `json:"media_type" yaml:"media_type"`
	ContentDigest core.Digest `json:"content_digest" yaml:"content_digest"`
}

type ExtensionV1 struct {
	Key           string          `json:"key" yaml:"key"`
	Required      bool            `json:"required" yaml:"required"`
	Schema        SchemaRefV1     `json:"schema" yaml:"schema"`
	ContentDigest core.Digest     `json:"content_digest" yaml:"content_digest"`
	Payload       json.RawMessage `json:"payload" yaml:"payload"`
}

type AgentDefinitionSourceV1 struct {
	ContractVersion     string                   `json:"contract_version" yaml:"contract_version"`
	DefinitionID        string                   `json:"definition_id" yaml:"definition_id"`
	Revision            core.Revision            `json:"revision" yaml:"revision"`
	IdentityRef         ObjectRefV1              `json:"identity_ref" yaml:"identity_ref"`
	ProfileSelectionRef ObjectRefV1              `json:"profile_selection_ref" yaml:"profile_selection_ref"`
	Components          []ComponentRequirementV1 `json:"components" yaml:"components"`
	PolicyRefs          PolicyRefsV1             `json:"policy_refs" yaml:"policy_refs"`
	SecretRefs          []SecretRefV1            `json:"secret_refs" yaml:"secret_refs"`
	ProvenanceRef       ObjectRefV1              `json:"provenance_ref" yaml:"provenance_ref"`
	ApprovalRef         ObjectRefV1              `json:"approval_ref" yaml:"approval_ref"`
	EffectiveWindow     EffectiveWindowV1        `json:"effective_window" yaml:"effective_window"`
	Extensions          []ExtensionV1            `json:"extensions" yaml:"extensions"`
	ChangeReason        string                   `json:"change_reason" yaml:"change_reason"`
}

type AgentDefinitionV1 struct {
	AgentDefinitionSourceV1
	CreatedUnixNano int64       `json:"created_unix_nano"`
	SourceDigest    core.Digest `json:"source_digest"`
	Digest          core.Digest `json:"digest"`
}

type AgentDefinitionRefV1 struct {
	DefinitionID string        `json:"definition_id"`
	Revision     core.Revision `json:"revision"`
	Digest       core.Digest   `json:"digest"`
}

type DefinitionCurrentStateV1 string

const (
	DefinitionCurrentActiveV1  DefinitionCurrentStateV1 = "active"
	DefinitionCurrentRevokedV1 DefinitionCurrentStateV1 = "revoked"
	DefinitionCurrentExpiredV1 DefinitionCurrentStateV1 = "expired"
)

type DefinitionCurrentV1 struct {
	Definition       AgentDefinitionRefV1     `json:"definition"`
	State            DefinitionCurrentStateV1 `json:"state"`
	Revision         core.Revision            `json:"revision"`
	UpdatedUnixNano  int64                    `json:"updated_unix_nano"`
	CheckedUnixNano  int64                    `json:"checked_unix_nano"`
	Reason           string                   `json:"reason,omitempty"`
	ProjectionDigest core.Digest              `json:"projection_digest"`
}

type ValidationCatalogV1 struct {
	Kinds        []string `json:"kinds"`
	Capabilities []string `json:"capabilities"`
	// RegisteredExtensionKeys permits declaration-time required-key recognition
	// only. It is not schema trust or production-resolution authority.
	RegisteredExtensionKeys []string `json:"registered_extension_keys"`
}

var requiredCoreKindsV1 = [...]string{
	"praxis/continuity",
	"praxis/tool-mcp",
	"praxis/memory-knowledge",
	"praxis/sandbox",
	"praxis/review",
	"praxis/context-cache",
	"praxis/harness",
}

func RequiredCoreKindsV1() []string {
	return append([]string(nil), requiredCoreKindsV1[:]...)
}

func (d AgentDefinitionV1) RefV1() AgentDefinitionRefV1 {
	return AgentDefinitionRefV1{DefinitionID: d.DefinitionID, Revision: d.Revision, Digest: d.Digest}
}

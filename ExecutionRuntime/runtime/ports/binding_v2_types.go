package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	BindingContractVersionV2 = "praxis.runtime.binding/v2"
	MaxManifestSetEntries    = 256
	MaxOpaqueInlineBytes     = 256 << 10
	MaxOpaqueReferenceBytes  = 2048
)

type NamespacedNameV2 string
type ComponentIDV2 NamespacedNameV2
type ComponentKindV2 NamespacedNameV2
type GovernanceCategoryV2 NamespacedNameV2
type CapabilityNameV2 NamespacedNameV2

type LocalityV2 string

const (
	LocalityHostControlPlane   LocalityV2 = "host_control_plane"
	LocalityInstanceDataPlane  LocalityV2 = "instance_data_plane"
	LocalityExternalStatePlane LocalityV2 = "external_state_plane"
	LocalityRemoteProvider     LocalityV2 = "remote_provider"
)

type ResidualClassV2 string

const (
	ResidualNone             ResidualClassV2 = "none"
	ResidualInspectable      ResidualClassV2 = "inspectable"
	ResidualCompensatable    ResidualClassV2 = "compensatable"
	ResidualExternallyOwned  ResidualClassV2 = "externally_owned"
	ResidualPotentiallyStale ResidualClassV2 = "potentially_stale"
)

type OfflinePolicyModeV2 string

const (
	OfflineDenied              OfflinePolicyModeV2 = "denied"
	OfflineObserveOnly         OfflinePolicyModeV2 = "observe_only"
	OfflineCachedAuthorityOnly OfflinePolicyModeV2 = "cached_authority_only"
)

type OwnerRoleV2 string

const (
	OwnerEffect     OwnerRoleV2 = "effect"
	OwnerSettlement OwnerRoleV2 = "settlement"
	OwnerCleanup    OwnerRoleV2 = "cleanup"
)

type VersionRangeV2 struct {
	MinimumInclusive string `json:"minimum_inclusive"`
	MaximumExclusive string `json:"maximum_exclusive"`
}

type ContractBindingV2 struct {
	Name       NamespacedNameV2 `json:"name"`
	Version    string           `json:"version"`
	Compatible VersionRangeV2   `json:"compatible_range"`
}

type SchemaRefV2 struct {
	Namespace     string      `json:"namespace"`
	Name          string      `json:"name"`
	Version       string      `json:"version"`
	MediaType     string      `json:"media_type"`
	ContentDigest core.Digest `json:"content_digest"`
}

func (s SchemaRefV2) Key() string {
	return s.Namespace + "/" + s.Name + "@" + s.Version + ";" + s.MediaType + ";" + string(s.ContentDigest)
}

type OpaqueLimitPolicyRefV2 struct {
	Policy NamespacedNameV2 `json:"policy"`
	Digest core.Digest      `json:"digest"`
}

// OpaquePayloadV2 is a bounded envelope. Exactly one of Inline or Ref is set.
// Runtime validates the wrapper and content digest but never interprets Inline.
type OpaquePayloadV2 struct {
	Schema        SchemaRefV2            `json:"schema"`
	ContentDigest core.Digest            `json:"content_digest"`
	Length        uint64                 `json:"length"`
	Inline        []byte                 `json:"inline,omitempty"`
	Ref           string                 `json:"ref,omitempty"`
	LimitPolicy   OpaqueLimitPolicyRefV2 `json:"limit_policy"`
}

type GovernanceExtensionV2 struct {
	Key      NamespacedNameV2 `json:"key"`
	Required bool             `json:"required"`
	Payload  OpaquePayloadV2  `json:"payload"`
}

type DisplayAnnotationV2 struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type ComponentDependencyV2 struct {
	ComponentID ComponentIDV2 `json:"component_id"`
	Optional    bool          `json:"optional"`
}

type CapabilityRequirementV2 struct {
	Capability        CapabilityNameV2 `json:"capability"`
	ProviderComponent ComponentIDV2    `json:"provider_component"`
	Optional          bool             `json:"optional"`
}

type ProvidedCapabilityV2 struct {
	Capability CapabilityNameV2 `json:"capability"`
	TTLSeconds uint64           `json:"ttl_seconds"`
	Schemas    []SchemaRefV2    `json:"schemas"`
}

type OwnerAssignmentV2 struct {
	Role             OwnerRoleV2   `json:"role"`
	OwnerComponentID ComponentIDV2 `json:"owner_component_id"`
}

type CredentialRequirementV2 struct {
	CredentialClass   NamespacedNameV2 `json:"credential_class"`
	ScopeDigest       core.Digest      `json:"scope_digest"`
	MaximumTTLSeconds uint64           `json:"maximum_ttl_seconds"`
}

// ComponentManifestV2 contains governance-relevant metadata. Annotations are
// explicitly display-only and are excluded from BindingDigestV2.
type ComponentManifestV2 struct {
	ContractVersion      string                    `json:"contract_version"`
	ComponentID          ComponentIDV2             `json:"component_id"`
	Kind                 ComponentKindV2           `json:"kind"`
	GovernanceCategory   GovernanceCategoryV2      `json:"governance_category"`
	SemanticVersion      string                    `json:"semantic_version"`
	ArtifactDigest       core.Digest               `json:"artifact_digest"`
	Contract             ContractBindingV2         `json:"contract"`
	Schemas              []SchemaRefV2             `json:"schemas"`
	Locality             LocalityV2                `json:"locality"`
	Dependencies         []ComponentDependencyV2   `json:"dependencies"`
	RequiredCapabilities []CapabilityRequirementV2 `json:"required_capabilities"`
	ProvidedCapabilities []ProvidedCapabilityV2    `json:"provided_capabilities"`
	Conformance          ConformanceLevel          `json:"conformance"`
	ResidualClass        ResidualClassV2           `json:"residual_class"`
	Owners               []OwnerAssignmentV2       `json:"owners"`
	Credentials          []CredentialRequirementV2 `json:"credential_requirements"`
	OfflinePolicy        OfflinePolicyModeV2       `json:"offline_policy"`
	Extensions           []GovernanceExtensionV2   `json:"extensions"`
	Annotations          []DisplayAnnotationV2     `json:"annotations"`
}

type ExtensionPolicyV2 struct {
	Key NamespacedNameV2 `json:"key"`
}

type GovernanceRegistrationV2 struct {
	Kind               ComponentKindV2      `json:"kind"`
	Category           GovernanceCategoryV2 `json:"category"`
	Capabilities       []CapabilityNameV2   `json:"capabilities"`
	Schemas            []SchemaRefV2        `json:"schemas"`
	ExtensionPolicies  []ExtensionPolicyV2  `json:"extension_policies"`
	AllowedLocalities  []LocalityV2         `json:"allowed_localities"`
	AllowedConformance []ConformanceLevel   `json:"allowed_conformance"`
}

type GovernanceCatalogV2 struct {
	Registrations []GovernanceRegistrationV2 `json:"registrations"`
}

type CapabilityGrantV2 struct {
	Capability       CapabilityNameV2 `json:"capability"`
	EvidenceDigest   core.Digest      `json:"evidence_digest"`
	ObservedUnixNano int64            `json:"observed_unix_nano"`
	ExpiresUnixNano  int64            `json:"expires_unix_nano"`
}

type BindingRequirementV2 struct {
	ComponentID          ComponentIDV2      `json:"component_id"`
	Kind                 ComponentKindV2    `json:"kind"`
	SemanticVersion      VersionRangeV2     `json:"semantic_version"`
	ContractName         NamespacedNameV2   `json:"contract_name"`
	Contract             VersionRangeV2     `json:"contract_version"`
	ArtifactDigest       core.Digest        `json:"artifact_digest"`
	RequiredCapabilities []CapabilityNameV2 `json:"required_capabilities"`
	Required             bool               `json:"required"`
	AllowResidual        bool               `json:"allow_residual"`
}

type BindingPlanV2 struct {
	ID               string                 `json:"id"`
	PlanDigest       core.Digest            `json:"plan_digest"`
	GovernanceDigest core.Digest            `json:"governance_digest"`
	Requirements     []BindingRequirementV2 `json:"requirements"`
}

type DescriberV2 interface {
	DescribeV2(context.Context) (ComponentManifestV2, error)
}

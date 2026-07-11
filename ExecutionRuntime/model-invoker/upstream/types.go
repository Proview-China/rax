package upstream

// RouteID is the stable catalog identity of a complete seven-dimensional
// upstream route.
type RouteID string

type ProviderID string
type OfferingID string
type DeploymentID string
type ProtocolID string
type EndpointID string
type CredentialProfileID string

const (
	ProtocolResponses       ProtocolID = "responses"
	ProtocolChatCompletions ProtocolID = "chat_completions"
	ProtocolMessages        ProtocolID = "messages"
	ProtocolGenerateContent ProtocolID = "generate_content"
	ProtocolBedrockConverse ProtocolID = "bedrock_converse"
	ProtocolBedrockInvoke   ProtocolID = "bedrock_invoke_model"
)

// ModelIdentity separates Praxis' canonical model family from the model name
// understood by the selected provider.
type ModelIdentity struct {
	CanonicalFamily  string `json:"canonical_family"`
	ProviderModelRef string `json:"provider_model_ref"`
}

type OfferingKind string

const (
	OfferingPayAsYouGo  OfferingKind = "pay_as_you_go"
	OfferingTokenPlan   OfferingKind = "token_plan"
	OfferingCodingPlan  OfferingKind = "coding_plan"
	OfferingProvisioned OfferingKind = "provisioned"
	OfferingDedicated   OfferingKind = "dedicated"
	OfferingSelfHosted  OfferingKind = "self_hosted"
)

// AllowedUsage is the maximum usage scope permitted by an offering's official
// terms. OfficialClientOnly is never directly callable by Praxis.
type AllowedUsage string

const (
	AllowedUsageGeneralAPI            AllowedUsage = "general_api"
	AllowedUsageInteractiveCodingOnly AllowedUsage = "interactive_coding_only"
	AllowedUsageOfficialClientOnly    AllowedUsage = "official_client_only"
)

// InvocationUsage describes the caller's actual execution context.
type InvocationUsage string

const (
	InvocationGeneralAPI        InvocationUsage = "general_api"
	InvocationInteractiveCoding InvocationUsage = "interactive_coding"
)

// CommercialEntitlement records static product policy. Account-specific
// balances and expiry timestamps belong to runtime state, not the catalog.
type CommercialEntitlement struct {
	AllowedUsage              AllowedUsage     `json:"allowed_usage"`
	RequiresExplicitContext   bool             `json:"requires_explicit_context,omitempty"`
	AllowsAutomaticPAYGSwitch bool             `json:"allows_automatic_payg_switch,omitempty"`
	ClientRestrictions        []string         `json:"client_restrictions,omitempty"`
	SubjectPolicy             SubjectPolicy    `json:"subject_policy,omitempty"`
	TenancyPolicy             TenancyPolicy    `json:"tenancy_policy,omitempty"`
	ExecutionPolicy           ExecutionPolicy  `json:"execution_policy,omitempty"`
	ProductionPolicy          ProductionPolicy `json:"production_policy,omitempty"`
	RequiresClientIdentity    bool             `json:"requires_client_identity,omitempty"`
	AllowedClientNames        []string         `json:"allowed_client_names,omitempty"`
}

type Offering struct {
	ID          OfferingID            `json:"id"`
	Kind        OfferingKind          `json:"kind"`
	Entitlement CommercialEntitlement `json:"entitlement"`
	BillingPlan *BillingPlanReference `json:"billing_plan,omitempty"`
}

type BillingPlanKind string

const BillingPlanSavings BillingPlanKind = "savings_plan"

// BillingPlanReference changes settlement for an existing Offering. It is not
// a Provider, Route, Endpoint, Protocol, or Credential identity dimension.
type BillingPlanReference struct {
	ID                  string          `json:"id"`
	Kind                BillingPlanKind `json:"kind"`
	BillingOwner        string          `json:"billing_owner"`
	AppliesToOfferingID OfferingID      `json:"applies_to_offering_id"`
}

type DeploymentKind string

const (
	DeploymentDirect           DeploymentKind = "direct"
	DeploymentCloudServerless  DeploymentKind = "cloud_serverless"
	DeploymentCloudProvisioned DeploymentKind = "cloud_provisioned"
	DeploymentThirdParty       DeploymentKind = "third_party"
	DeploymentSelfHosted       DeploymentKind = "self_hosted"
)

// Deployment contains references to provider-side runtime scope. Reference
// fields identify configuration; they are not credential values.
type Deployment struct {
	ID             DeploymentID   `json:"id"`
	Kind           DeploymentKind `json:"kind"`
	Region         string         `json:"region,omitempty"`
	ProjectRef     string         `json:"project_ref,omitempty"`
	WorkspaceRef   string         `json:"workspace_ref,omitempty"`
	ResourceRef    string         `json:"resource_ref,omitempty"`
	DeploymentName string         `json:"deployment_name,omitempty"`
}

type ProtocolBinding struct {
	ID         ProtocolID `json:"id"`
	APIVersion string     `json:"api_version,omitempty"`
}

// Endpoint is data-only. HostTemplate may contain named placeholders such as
// {region}; it must not contain credentials or a URL scheme.
type Endpoint struct {
	ID                 EndpointID `json:"id"`
	Scheme             string     `json:"scheme"`
	HostTemplate       string     `json:"host_template"`
	BasePath           string     `json:"base_path,omitempty"`
	CredentialAudience string     `json:"credential_audience"`
}

type CredentialType string

const (
	CredentialAPIKey    CredentialType = "api_key"
	CredentialOAuth     CredentialType = "oauth"
	CredentialADC       CredentialType = "adc"
	CredentialEntraID   CredentialType = "entra_id"
	CredentialSigV4     CredentialType = "sigv4"
	CredentialBearer    CredentialType = "bearer"
	CredentialAnonymous CredentialType = "anonymous"
)

type AuthPlacement string

const (
	AuthPlacementHeader         AuthPlacement = "header"
	AuthPlacementQuery          AuthPlacement = "query"
	AuthPlacementSDK            AuthPlacement = "sdk"
	AuthPlacementRequestSigning AuthPlacement = "request_signing"
	AuthPlacementNone           AuthPlacement = "none"
)

type CredentialLifecycle string

const (
	CredentialLifecycleStatic           CredentialLifecycle = "static"
	CredentialLifecycleRenewable        CredentialLifecycle = "renewable"
	CredentialLifecycleShortLived       CredentialLifecycle = "short_lived"
	CredentialLifecycleWorkloadIdentity CredentialLifecycle = "workload_identity"
	CredentialLifecycleAnonymous        CredentialLifecycle = "anonymous"
)

type CredentialPurpose string

const (
	CredentialPurposeAPIKey           CredentialPurpose = "api_key"
	CredentialPurposeBearerToken      CredentialPurpose = "bearer_token"
	CredentialPurposeClientID         CredentialPurpose = "client_id"
	CredentialPurposeClientSecret     CredentialPurpose = "client_secret"
	CredentialPurposeTenantID         CredentialPurpose = "tenant_id"
	CredentialPurposeAccessKeyID      CredentialPurpose = "access_key_id"
	CredentialPurposeSecretAccessKey  CredentialPurpose = "secret_access_key"
	CredentialPurposeSessionToken     CredentialPurpose = "session_token"
	CredentialPurposeProfile          CredentialPurpose = "profile"
	CredentialPurposeWorkloadIdentity CredentialPurpose = "workload_identity"
	CredentialPurposeCertificate      CredentialPurpose = "certificate"
)

// CredentialReference names a secret-store entry. There is intentionally no
// field capable of holding a plaintext credential value.
type CredentialReference struct {
	Purpose CredentialPurpose `json:"purpose,omitempty"`
	Store   string            `json:"store"`
	Name    string            `json:"name"`
}

// CredentialProfile contains only typed references and routing constraints.
// Secret values are resolved by a later runtime boundary.
type CredentialProfile struct {
	ID                   CredentialProfileID   `json:"id"`
	Type                 CredentialType        `json:"type"`
	References           []CredentialReference `json:"references,omitempty"`
	Audience             string                `json:"audience"`
	AuthPlacement        AuthPlacement         `json:"auth_placement,omitempty"`
	AuthHeader           string                `json:"auth_header,omitempty"`
	AuthParameter        string                `json:"auth_parameter,omitempty"`
	AuthScheme           string                `json:"auth_scheme,omitempty"`
	Scopes               []string              `json:"scopes,omitempty"`
	KeyPrefixes          []string              `json:"key_prefixes,omitempty"`
	DeniedKeyPrefixes    []string              `json:"denied_key_prefixes,omitempty"`
	AllowedProviderIDs   []ProviderID          `json:"allowed_provider_ids,omitempty"`
	AllowedOfferingIDs   []OfferingID          `json:"allowed_offering_ids,omitempty"`
	AllowedDeploymentIDs []DeploymentID        `json:"allowed_deployment_ids,omitempty"`
	AllowedRegions       []string              `json:"allowed_regions,omitempty"`
	AllowedEndpointIDs   []EndpointID          `json:"allowed_endpoint_ids"`
	Lifecycle            CredentialLifecycle   `json:"lifecycle,omitempty"`
	SigV4Service         string                `json:"sigv4_service,omitempty"`
}

// UpstreamRoute is the complete runtime identity. Model, Provider, Offering,
// Deployment, Protocol, Endpoint and Credential are independent dimensions.
type UpstreamRoute struct {
	ID         RouteID           `json:"id"`
	Model      ModelIdentity     `json:"model"`
	Provider   ProviderID        `json:"provider"`
	Offering   Offering          `json:"offering"`
	Deployment Deployment        `json:"deployment"`
	Protocol   ProtocolBinding   `json:"protocol"`
	Endpoint   Endpoint          `json:"endpoint"`
	Credential CredentialProfile `json:"credential"`
}

// MappingReason is a stable, machine-readable explanation of a route decision.
type MappingReason struct {
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}

type CapabilityDecisionAction string

const (
	CapabilityExact       CapabilityDecisionAction = "exact"
	CapabilityTransformed CapabilityDecisionAction = "transformed"
	CapabilityDegraded    CapabilityDecisionAction = "degraded"
	CapabilityRejected    CapabilityDecisionAction = "rejected"
)

type CapabilityDecision struct {
	Capability  string                   `json:"capability"`
	Action      CapabilityDecisionAction `json:"action"`
	ReasonCode  string                   `json:"reason_code"`
	Detail      string                   `json:"detail,omitempty"`
	Limitations []string                 `json:"limitations,omitempty"`
}

// MappingReport is a data-only route selection result. It is intentionally
// independent of provider SDK types.
type MappingReport struct {
	Identity            RouteIdentity        `json:"identity"`
	RouteID             RouteID              `json:"route_id"`
	Provider            ProviderID           `json:"provider"`
	EvidenceDigest      string               `json:"evidence_digest"`
	Reasons             []MappingReason      `json:"reasons,omitempty"`
	CapabilityDecisions []CapabilityDecision `json:"capability_decisions"`
	Degradations        []string             `json:"degradations,omitempty"`
}

// Allows reports whether Praxis may invoke this offering in the stated
// execution context. Official-client-only products always return false.
func (o Offering) Allows(usage InvocationUsage) bool {
	switch o.Entitlement.AllowedUsage {
	case AllowedUsageGeneralAPI:
		return usage == InvocationGeneralAPI || usage == InvocationInteractiveCoding
	case AllowedUsageInteractiveCodingOnly:
		return usage == InvocationInteractiveCoding
	default:
		return false
	}
}

// Clone returns a deep copy safe for independent mutation.
func (r UpstreamRoute) Clone() UpstreamRoute {
	clone := r
	if r.Offering.BillingPlan != nil {
		billingPlan := *r.Offering.BillingPlan
		clone.Offering.BillingPlan = &billingPlan
	}
	clone.Offering.Entitlement.ClientRestrictions = append([]string(nil), r.Offering.Entitlement.ClientRestrictions...)
	clone.Offering.Entitlement.AllowedClientNames = append([]string(nil), r.Offering.Entitlement.AllowedClientNames...)
	clone.Credential.References = append([]CredentialReference(nil), r.Credential.References...)
	clone.Credential.Scopes = append([]string(nil), r.Credential.Scopes...)
	clone.Credential.KeyPrefixes = append([]string(nil), r.Credential.KeyPrefixes...)
	clone.Credential.DeniedKeyPrefixes = append([]string(nil), r.Credential.DeniedKeyPrefixes...)
	clone.Credential.AllowedProviderIDs = append([]ProviderID(nil), r.Credential.AllowedProviderIDs...)
	clone.Credential.AllowedOfferingIDs = append([]OfferingID(nil), r.Credential.AllowedOfferingIDs...)
	clone.Credential.AllowedDeploymentIDs = append([]DeploymentID(nil), r.Credential.AllowedDeploymentIDs...)
	clone.Credential.AllowedRegions = append([]string(nil), r.Credential.AllowedRegions...)
	clone.Credential.AllowedEndpointIDs = append([]EndpointID(nil), r.Credential.AllowedEndpointIDs...)
	return clone
}

// Clone returns a deep copy safe for independent mutation.
func (r MappingReport) Clone() MappingReport {
	clone := r
	clone.Reasons = append([]MappingReason(nil), r.Reasons...)
	clone.Degradations = append([]string(nil), r.Degradations...)
	clone.CapabilityDecisions = make([]CapabilityDecision, len(r.CapabilityDecisions))
	for index, decision := range r.CapabilityDecisions {
		clone.CapabilityDecisions[index] = decision
		clone.CapabilityDecisions[index].Limitations = append([]string(nil), decision.Limitations...)
	}
	return clone
}

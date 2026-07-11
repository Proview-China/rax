package catalog

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

const SchemaVersion = "praxis.upstream-catalog/v1"

type EvidenceStatus string

const (
	EvidenceFresh        EvidenceStatus = "fresh"
	EvidenceStale        EvidenceStatus = "stale"
	EvidenceInvalidated  EvidenceStatus = "invalidated"
	EvidenceUnverified   EvidenceStatus = "unverified"
	EvidenceTermsBlocked EvidenceStatus = "terms_blocked"
	EvidenceDeprecated   EvidenceStatus = "deprecated"
)

type ImplementationStatus string

const (
	ImplementationResearchOnly       ImplementationStatus = "research_only"
	ImplementationDesigned           ImplementationStatus = "designed"
	ImplementationPlanned            ImplementationStatus = "planned"
	ImplementationImplementedOffline ImplementationStatus = "implemented_offline"
	ImplementationLiveVerified       ImplementationStatus = "live_verified"
	ImplementationProductionApproved ImplementationStatus = "production_approved"
)

type SourceKind string

const (
	SourceAPIReference SourceKind = "api_reference"
	SourceSDK          SourceKind = "sdk"
	SourceTerms        SourceKind = "terms"
	SourceProductDocs  SourceKind = "product_docs"
	SourceModelCatalog SourceKind = "model_catalog"
)

// OfficialSource is a provenance assertion made by the catalog author. The
// validator requires a secure URL and publisher identity but does not perform
// network access.
type OfficialSource struct {
	ID        string     `json:"id"`
	Publisher string     `json:"publisher"`
	Kind      SourceKind `json:"kind"`
	URL       string     `json:"url"`
}

type Evidence struct {
	Status                EvidenceStatus   `json:"status"`
	TTLClass              EvidenceTTLClass `json:"ttl_class"`
	CheckedAt             time.Time        `json:"checked_at"`
	ValidUntil            time.Time        `json:"valid_until"`
	InvalidatedBySourceID string           `json:"invalidated_by_source_id,omitempty"`
	Digest                string           `json:"digest"`
}

type EvidenceTTLClass string

const (
	EvidenceTTL7Days  EvidenceTTLClass = "7d"
	EvidenceTTL14Days EvidenceTTLClass = "14d"
	EvidenceTTL30Days EvidenceTTLClass = "30d"
	EvidenceTTL90Days EvidenceTTLClass = "90d"
)

// Duration returns the exact validity window represented by the TTL class.
func (class EvidenceTTLClass) Duration() (time.Duration, bool) {
	switch class {
	case EvidenceTTL7Days:
		return 7 * 24 * time.Hour, true
	case EvidenceTTL14Days:
		return 14 * 24 * time.Hour, true
	case EvidenceTTL30Days:
		return 30 * 24 * time.Hour, true
	case EvidenceTTL90Days:
		return 90 * 24 * time.Hour, true
	default:
		return 0, false
	}
}

type SDKOwnership string

const (
	SDKOwnershipProviderNative   SDKOwnership = "provider_native"
	SDKOwnershipModelVendor      SDKOwnership = "model_vendor"
	SDKOwnershipProtocolUpstream SDKOwnership = "protocol_upstream"
	SDKOwnershipCloudNative      SDKOwnership = "cloud_native"
	SDKOwnershipCommunity        SDKOwnership = "community"
)

type TransportKind string

const (
	TransportSDK       TransportKind = "sdk"
	TransportHTTP      TransportKind = "http"
	TransportSSE       TransportKind = "sse"
	TransportWebSocket TransportKind = "websocket"
	TransportGRPC      TransportKind = "grpc"
	TransportSidecar   TransportKind = "sidecar"
)

type SDKMetadata struct {
	Language  string        `json:"language"`
	Package   string        `json:"package"`
	Owner     string        `json:"owner"`
	Ownership SDKOwnership  `json:"ownership"`
	Transport TransportKind `json:"transport"`
	Version   string        `json:"version"`
	License   string        `json:"license"`
	Official  bool          `json:"official"`
}

type CapabilitySupport string

const (
	CapabilityNative      CapabilitySupport = "native"
	CapabilityCompatible  CapabilitySupport = "compatible"
	CapabilityPartial     CapabilitySupport = "partial"
	CapabilityUnsupported CapabilitySupport = "unsupported"
	CapabilityUnknown     CapabilitySupport = "unknown"
)

type CapabilityMetadata struct {
	ID          string            `json:"id"`
	Support     CapabilitySupport `json:"support"`
	Limitations []string          `json:"limitations,omitempty"`
}

type Implementation struct {
	Status       ImplementationStatus `json:"status"`
	Callable     bool                 `json:"callable"`
	AdapterID    string               `json:"adapter_id,omitempty"`
	CodePaths    []string             `json:"code_paths,omitempty"`
	TestEvidence []string             `json:"test_evidence,omitempty"`
	LiveEvidence []string             `json:"live_evidence,omitempty"`
}

type Maturity string

const (
	MaturityGA           Maturity = "ga"
	MaturityPreview      Maturity = "preview"
	MaturityExperimental Maturity = "experimental"
	MaturityUnknown      Maturity = "unknown"
)

type ModelDiscoveryMethod string

const (
	ModelDiscoveryRuntimeSelected ModelDiscoveryMethod = "runtime_selected"
	ModelDiscoveryStaticCatalog   ModelDiscoveryMethod = "static_catalog"
	ModelDiscoveryProviderAPI     ModelDiscoveryMethod = "provider_api"
)

type ModelAliasPolicy string

const (
	ModelAliasExactProviderID ModelAliasPolicy = "exact_provider_id"
	ModelAliasStable          ModelAliasPolicy = "stable_alias"
	ModelAliasProviderManaged ModelAliasPolicy = "provider_managed_alias"
)

type ModelAlias struct {
	Alias            string `json:"alias"`
	ProviderModelRef string `json:"provider_model_ref"`
	Stable           bool   `json:"stable"`
}

type ModelDiscovery struct {
	Method      ModelDiscoveryMethod `json:"method"`
	AliasPolicy ModelAliasPolicy     `json:"alias_policy"`
	Aliases     []ModelAlias         `json:"aliases,omitempty"`
}

type ErrorDialect struct {
	Envelope         string   `json:"envelope"`
	CodeField        string   `json:"code_field"`
	RequestIDHeaders []string `json:"request_id_headers"`
	RetryHeaders     []string `json:"retry_headers,omitempty"`
}

type ProductionBoundary string

const (
	ProductionRequiresReview ProductionBoundary = "requires_review"
	ProductionAllowed        ProductionBoundary = "allowed"
	ProductionProhibited     ProductionBoundary = "prohibited"
	ProductionUnknown        ProductionBoundary = "unknown"
)

type QuotaBoundary string

const (
	QuotaProviderAccount     QuotaBoundary = "provider_account"
	QuotaSubscriptionWindow  QuotaBoundary = "subscription_window"
	QuotaProvisionedCapacity QuotaBoundary = "provisioned_capacity"
	QuotaSelfHosted          QuotaBoundary = "self_hosted"
	QuotaUnknown             QuotaBoundary = "unknown"
)

type ExpiryBoundary string

const (
	ExpiryCredentialOrAccount ExpiryBoundary = "credential_or_account"
	ExpirySubscriptionPeriod  ExpiryBoundary = "subscription_period"
	ExpiryNone                ExpiryBoundary = "none"
	ExpiryUnknown             ExpiryBoundary = "unknown"
)

type OperationalBoundaries struct {
	Production ProductionBoundary `json:"production"`
	Quota      QuotaBoundary      `json:"quota"`
	Expiry     ExpiryBoundary     `json:"expiry"`
}

type Entry struct {
	ID              upstream.RouteID       `json:"id"`
	Route           upstream.UpstreamRoute `json:"route"`
	Maturity        Maturity               `json:"maturity"`
	ModelDiscovery  ModelDiscovery         `json:"model_discovery"`
	Sources         []OfficialSource       `json:"official_sources"`
	Evidence        Evidence               `json:"evidence"`
	SDKs            []SDKMetadata          `json:"sdks"`
	Capabilities    []CapabilityMetadata   `json:"capabilities"`
	IgnoredFields   []string               `json:"ignored_fields"`
	ExtensionFields []string               `json:"extension_fields"`
	StreamEvents    []string               `json:"stream_events"`
	ErrorDialect    ErrorDialect           `json:"error_dialect"`
	Boundaries      OperationalBoundaries  `json:"boundaries"`
	Implementation  Implementation         `json:"implementation"`
}

type Document struct {
	SchemaVersion string  `json:"schema_version"`
	Entries       []Entry `json:"entries"`
}

func (entry Entry) Clone() Entry {
	clone := entry
	clone.Route = entry.Route.Clone()
	clone.Sources = append([]OfficialSource(nil), entry.Sources...)
	clone.SDKs = append([]SDKMetadata(nil), entry.SDKs...)
	clone.ModelDiscovery.Aliases = append([]ModelAlias(nil), entry.ModelDiscovery.Aliases...)
	clone.Capabilities = make([]CapabilityMetadata, len(entry.Capabilities))
	for index, capability := range entry.Capabilities {
		clone.Capabilities[index] = capability
		clone.Capabilities[index].Limitations = append([]string(nil), capability.Limitations...)
	}
	clone.IgnoredFields = append([]string(nil), entry.IgnoredFields...)
	clone.ExtensionFields = append([]string(nil), entry.ExtensionFields...)
	clone.StreamEvents = append([]string(nil), entry.StreamEvents...)
	clone.ErrorDialect.RequestIDHeaders = append([]string(nil), entry.ErrorDialect.RequestIDHeaders...)
	clone.ErrorDialect.RetryHeaders = append([]string(nil), entry.ErrorDialect.RetryHeaders...)
	clone.Implementation.CodePaths = append([]string(nil), entry.Implementation.CodePaths...)
	clone.Implementation.TestEvidence = append([]string(nil), entry.Implementation.TestEvidence...)
	clone.Implementation.LiveEvidence = append([]string(nil), entry.Implementation.LiveEvidence...)
	return clone
}

func (document Document) Clone() Document {
	clone := Document{SchemaVersion: document.SchemaVersion, Entries: make([]Entry, len(document.Entries))}
	for index, entry := range document.Entries {
		clone.Entries[index] = entry.Clone()
	}
	return clone
}

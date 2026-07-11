package catalog

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

var (
	builtinCheckedAt  = time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	builtinValidUntil = builtinCheckedAt.Add(7 * 24 * time.Hour)
)

var defaultCapabilityIDs = []string{
	"text_generation",
	"streaming",
	"tool_calling",
	"parallel_tool_calling",
	"structured_output",
	"reasoning",
	"reasoning_summary",
	"function_error_result",
	"vision_input",
	"audio_input",
	"video_input",
	"file_input",
	"server_state",
	"provider_continuation",
	"prompt_caching",
	"batch",
	"background_execution",
	"realtime",
	"hosted_tools",
	"usage_reporting",
}

func completeCapabilities(overrides map[string]CapabilityMetadata) []CapabilityMetadata {
	capabilities := make([]CapabilityMetadata, 0, len(defaultCapabilityIDs))
	for _, id := range defaultCapabilityIDs {
		if capability, ok := overrides[id]; ok {
			capability.ID = id
			capabilities = append(capabilities, capability)
			continue
		}
		capabilities = append(capabilities, CapabilityMetadata{
			ID: id, Support: CapabilityUnsupported,
			Limitations: []string{"outside the currently implemented offline semantic slice"},
		})
	}
	return capabilities
}

func finalizeDefaultEntry(entry Entry) Entry {
	digest, err := ComputeEvidenceDigest(entry)
	if err != nil {
		panic("catalog: compute built-in evidence digest: " + err.Error())
	}
	entry.Evidence.Digest = digest
	return entry
}

// DefaultDocument contains the callable direct bindings plus reviewed control-
// plane records for later routes. Credential records contain secret references
// only; planned and research routes remain non-callable.
func DefaultDocument() Document {
	entries := []Entry{
		openAIEntry(upstream.ProtocolResponses),
		openAIEntry(upstream.ProtocolChatCompletions),
		anthropicEntry(),
		geminiEntry(),
	}
	entries = append(entries, directEntries()...)
	entries = append(entries, cloudEntries()...)
	entries = append(entries, subscriptionEntries()...)
	return Document{
		SchemaVersion: SchemaVersion,
		Entries:       entries,
	}
}

func NewDefault(now time.Time) (*Catalog, error) {
	return New(DefaultDocument(), now)
}

func openAIEntry(protocol upstream.ProtocolID) Entry {
	routeID := upstream.RouteID("openai.direct.payg." + string(protocol))
	overrides := map[string]CapabilityMetadata{
		"text_generation":       {Support: CapabilityNative},
		"streaming":             {Support: CapabilityNative},
		"tool_calling":          {Support: CapabilityNative},
		"parallel_tool_calling": {Support: CapabilityNative},
		"structured_output":     {Support: CapabilityNative},
		"reasoning":             {Support: CapabilityNative},
		"function_error_result": {Support: CapabilityPartial, Limitations: []string{"portable is_error is unavailable; explicit degradation preserves output text"}},
		"usage_reporting":       {Support: CapabilityNative},
	}
	if protocol == upstream.ProtocolResponses {
		overrides["reasoning_summary"] = CapabilityMetadata{Support: CapabilityNative}
		overrides["server_state"] = CapabilityMetadata{Support: CapabilityNative}
	} else {
		overrides["reasoning_summary"] = CapabilityMetadata{Support: CapabilityPartial, Limitations: []string{"Responses-style reasoning summaries are unavailable"}}
		overrides["server_state"] = CapabilityMetadata{Support: CapabilityUnsupported, Limitations: []string{"previous_response_id is a Responses-only feature"}}
	}
	streamEvents := []string{"chat.completion.chunk", "done"}
	if protocol == upstream.ProtocolResponses {
		streamEvents = []string{"response.created", "response.output_text.delta", "response.function_call_arguments.delta", "response.completed", "error"}
	}
	entry := Entry{
		ID: routeID,
		Route: upstream.UpstreamRoute{
			ID:       routeID,
			Model:    upstream.ModelIdentity{CanonicalFamily: "openai", ProviderModelRef: "runtime_selected"},
			Provider: "openai",
			Offering: upstream.Offering{
				ID:          "openai.api.payg",
				Kind:        upstream.OfferingPayAsYouGo,
				Entitlement: upstream.CommercialEntitlement{AllowedUsage: upstream.AllowedUsageGeneralAPI},
			},
			Deployment: upstream.Deployment{ID: "openai.direct.global", Kind: upstream.DeploymentDirect, Region: "global"},
			Protocol:   upstream.ProtocolBinding{ID: protocol, APIVersion: "v1"},
			Endpoint: upstream.Endpoint{
				ID:                 "openai.public",
				Scheme:             "https",
				HostTemplate:       "api.openai.com",
				BasePath:           "/v1",
				CredentialAudience: "api.openai.com",
			},
			Credential: upstream.CredentialProfile{
				ID: "openai.default", Type: upstream.CredentialAPIKey,
				References: []upstream.CredentialReference{{Purpose: upstream.CredentialPurposeAPIKey, Store: "env", Name: "OPENAI_API_KEY"}},
				Audience:   "api.openai.com", AuthPlacement: upstream.AuthPlacementHeader,
				AuthHeader: "Authorization", AuthScheme: "Bearer", Lifecycle: upstream.CredentialLifecycleStatic,
				AllowedProviderIDs: []upstream.ProviderID{"openai"}, AllowedOfferingIDs: []upstream.OfferingID{"openai.api.payg"},
				AllowedDeploymentIDs: []upstream.DeploymentID{"openai.direct.global"}, AllowedRegions: []string{"global"},
				AllowedEndpointIDs: []upstream.EndpointID{"openai.public"},
			},
		},
		Maturity:       MaturityUnknown,
		ModelDiscovery: ModelDiscovery{Method: ModelDiscoveryRuntimeSelected, AliasPolicy: ModelAliasExactProviderID},
		Sources: []OfficialSource{
			{ID: "openai.api.protocols", Publisher: "OpenAI", Kind: SourceAPIReference, URL: "https://developers.openai.com/api/docs/guides/migrate-to-responses"},
			{ID: "openai.sdk.go", Publisher: "OpenAI", Kind: SourceSDK, URL: "https://github.com/openai/openai-go"},
		},
		Evidence: Evidence{Status: EvidenceFresh, TTLClass: EvidenceTTL7Days, CheckedAt: builtinCheckedAt, ValidUntil: builtinValidUntil},
		SDKs: []SDKMetadata{{
			Language: "go", Package: "github.com/openai/openai-go/v3", Owner: "OpenAI",
			Ownership: SDKOwnershipProviderNative, Transport: TransportSDK,
			Version: "v3.41.1", License: "Apache-2.0", Official: true,
		}},
		Capabilities:    completeCapabilities(overrides),
		IgnoredFields:   []string{},
		ExtensionFields: []string{},
		StreamEvents:    streamEvents,
		ErrorDialect: ErrorDialect{
			Envelope: "openai.error", CodeField: "error.code",
			RequestIDHeaders: []string{"x-request-id"}, RetryHeaders: []string{"retry-after"},
		},
		Boundaries: OperationalBoundaries{Production: ProductionRequiresReview, Quota: QuotaProviderAccount, Expiry: ExpiryCredentialOrAccount},
		Implementation: Implementation{
			Status:       ImplementationImplementedOffline,
			Callable:     true,
			AdapterID:    "openai",
			CodePaths:    []string{"provider/openai"},
			TestEvidence: []string{"tests/openai", "tests/openai/provider_contract_test.go"},
		},
	}
	return finalizeDefaultEntry(entry)
}

func anthropicEntry() Entry {
	routeID := upstream.RouteID("anthropic.direct.payg.messages")
	entry := Entry{
		ID: routeID,
		Route: upstream.UpstreamRoute{
			ID:       routeID,
			Model:    upstream.ModelIdentity{CanonicalFamily: "claude", ProviderModelRef: "runtime_selected"},
			Provider: "anthropic",
			Offering: upstream.Offering{
				ID:          "anthropic.api.payg",
				Kind:        upstream.OfferingPayAsYouGo,
				Entitlement: upstream.CommercialEntitlement{AllowedUsage: upstream.AllowedUsageGeneralAPI},
			},
			Deployment: upstream.Deployment{ID: "anthropic.direct.global", Kind: upstream.DeploymentDirect, Region: "global"},
			Protocol:   upstream.ProtocolBinding{ID: upstream.ProtocolMessages, APIVersion: "2023-06-01"},
			Endpoint: upstream.Endpoint{
				ID:                 "anthropic.public",
				Scheme:             "https",
				HostTemplate:       "api.anthropic.com",
				BasePath:           "/v1",
				CredentialAudience: "api.anthropic.com",
			},
			Credential: upstream.CredentialProfile{
				ID: "anthropic.default", Type: upstream.CredentialAPIKey,
				References: []upstream.CredentialReference{{Purpose: upstream.CredentialPurposeAPIKey, Store: "env", Name: "ANTHROPIC_API_KEY"}},
				Audience:   "api.anthropic.com", AuthPlacement: upstream.AuthPlacementHeader,
				AuthHeader: "x-api-key", Lifecycle: upstream.CredentialLifecycleStatic,
				AllowedProviderIDs: []upstream.ProviderID{"anthropic"}, AllowedOfferingIDs: []upstream.OfferingID{"anthropic.api.payg"},
				AllowedDeploymentIDs: []upstream.DeploymentID{"anthropic.direct.global"}, AllowedRegions: []string{"global"},
				AllowedEndpointIDs: []upstream.EndpointID{"anthropic.public"},
			},
		},
		Maturity:       MaturityUnknown,
		ModelDiscovery: ModelDiscovery{Method: ModelDiscoveryRuntimeSelected, AliasPolicy: ModelAliasExactProviderID},
		Sources: []OfficialSource{
			{ID: "anthropic.api.messages", Publisher: "Anthropic", Kind: SourceAPIReference, URL: "https://platform.claude.com/docs/en/api/overview"},
			{ID: "anthropic.sdk.go", Publisher: "Anthropic", Kind: SourceSDK, URL: "https://platform.claude.com/docs/en/cli-sdks-libraries/sdks/go"},
		},
		Evidence: Evidence{Status: EvidenceFresh, TTLClass: EvidenceTTL7Days, CheckedAt: builtinCheckedAt, ValidUntil: builtinValidUntil},
		SDKs: []SDKMetadata{{
			Language: "go", Package: "github.com/anthropics/anthropic-sdk-go", Owner: "Anthropic",
			Ownership: SDKOwnershipProviderNative, Transport: TransportSDK,
			Version: "v1.56.0", License: "MIT", Official: true,
		}},
		Capabilities: completeCapabilities(map[string]CapabilityMetadata{
			"text_generation":       {Support: CapabilityNative},
			"streaming":             {Support: CapabilityNative},
			"tool_calling":          {Support: CapabilityNative},
			"parallel_tool_calling": {Support: CapabilityNative},
			"function_error_result": {Support: CapabilityNative},
			"structured_output":     {Support: CapabilityCompatible},
			"reasoning":             {Support: CapabilityCompatible},
			"reasoning_summary":     {Support: CapabilityCompatible},
			"provider_continuation": {Support: CapabilityNative},
			"usage_reporting":       {Support: CapabilityNative},
		}),
		IgnoredFields:   []string{},
		ExtensionFields: []string{},
		StreamEvents: []string{
			"message_start", "content_block_start", "content_block_delta", "content_block_stop",
			"message_delta", "message_stop", "ping", "error",
		},
		ErrorDialect: ErrorDialect{
			Envelope: "anthropic.error", CodeField: "error.type",
			RequestIDHeaders: []string{"request-id"}, RetryHeaders: []string{"retry-after", "retry-after-ms"},
		},
		Boundaries: OperationalBoundaries{Production: ProductionRequiresReview, Quota: QuotaProviderAccount, Expiry: ExpiryCredentialOrAccount},
		Implementation: Implementation{
			Status:       ImplementationImplementedOffline,
			Callable:     true,
			AdapterID:    "anthropic",
			CodePaths:    []string{"provider/anthropic"},
			TestEvidence: []string{"tests/anthropic", "tests/anthropic/provider_contract_test.go"},
		},
	}
	return finalizeDefaultEntry(entry)
}

func geminiEntry() Entry {
	routeID := upstream.RouteID("google.gemini-developer.payg.generate_content")
	entry := Entry{
		ID: routeID,
		Route: upstream.UpstreamRoute{
			ID:       routeID,
			Model:    upstream.ModelIdentity{CanonicalFamily: "gemini", ProviderModelRef: "runtime_selected"},
			Provider: "google.gemini-developer",
			Offering: upstream.Offering{
				ID:          "google.gemini-developer.api.payg",
				Kind:        upstream.OfferingPayAsYouGo,
				Entitlement: upstream.CommercialEntitlement{AllowedUsage: upstream.AllowedUsageGeneralAPI},
			},
			Deployment: upstream.Deployment{ID: "google.gemini-developer.global", Kind: upstream.DeploymentDirect, Region: "global"},
			Protocol:   upstream.ProtocolBinding{ID: upstream.ProtocolGenerateContent, APIVersion: "v1beta"},
			Endpoint: upstream.Endpoint{
				ID:                 "google.gemini-developer.public",
				Scheme:             "https",
				HostTemplate:       "generativelanguage.googleapis.com",
				BasePath:           "/v1beta",
				CredentialAudience: "generativelanguage.googleapis.com",
			},
			Credential: upstream.CredentialProfile{
				ID: "google.gemini-developer.default", Type: upstream.CredentialAPIKey,
				References: []upstream.CredentialReference{{Purpose: upstream.CredentialPurposeAPIKey, Store: "env", Name: "GEMINI_API_KEY"}},
				Audience:   "generativelanguage.googleapis.com", AuthPlacement: upstream.AuthPlacementHeader,
				AuthHeader: "x-goog-api-key", Lifecycle: upstream.CredentialLifecycleStatic,
				AllowedProviderIDs: []upstream.ProviderID{"google.gemini-developer"}, AllowedOfferingIDs: []upstream.OfferingID{"google.gemini-developer.api.payg"},
				AllowedDeploymentIDs: []upstream.DeploymentID{"google.gemini-developer.global"}, AllowedRegions: []string{"global"},
				AllowedEndpointIDs: []upstream.EndpointID{"google.gemini-developer.public"},
			},
		},
		Maturity:       MaturityUnknown,
		ModelDiscovery: ModelDiscovery{Method: ModelDiscoveryRuntimeSelected, AliasPolicy: ModelAliasExactProviderID},
		Sources: []OfficialSource{
			{ID: "google.gemini-developer.api", Publisher: "Google", Kind: SourceAPIReference, URL: "https://ai.google.dev/api"},
			{ID: "google.genai.sdk", Publisher: "Google", Kind: SourceSDK, URL: "https://ai.google.dev/gemini-api/docs/libraries"},
		},
		Evidence: Evidence{Status: EvidenceFresh, TTLClass: EvidenceTTL7Days, CheckedAt: builtinCheckedAt, ValidUntil: builtinValidUntil},
		SDKs: []SDKMetadata{{
			Language: "go", Package: "google.golang.org/genai", Owner: "Google",
			Ownership: SDKOwnershipProviderNative, Transport: TransportSDK,
			Version: "v1.63.0", License: "Apache-2.0", Official: true,
		}},
		Capabilities: completeCapabilities(map[string]CapabilityMetadata{
			"text_generation":       {Support: CapabilityNative},
			"streaming":             {Support: CapabilityNative},
			"tool_calling":          {Support: CapabilityNative},
			"parallel_tool_calling": {Support: CapabilityNative},
			"structured_output":     {Support: CapabilityNative},
			"function_error_result": {Support: CapabilityNative},
			"reasoning":             {Support: CapabilityCompatible},
			"reasoning_summary":     {Support: CapabilityPartial, Limitations: []string{"thought parts are provider-native, not a guaranteed portable summary"}},
			"provider_continuation": {Support: CapabilityNative},
			"usage_reporting":       {Support: CapabilityNative},
		}),
		IgnoredFields:   []string{},
		ExtensionFields: []string{},
		StreamEvents:    []string{"generate_content_response", "stream_terminal", "error"},
		ErrorDialect: ErrorDialect{
			Envelope: "google.rpc.status", CodeField: "error.status",
			RequestIDHeaders: []string{"x-goog-request-id", "x-request-id", "request-id"}, RetryHeaders: []string{"retry-after"},
		},
		Boundaries: OperationalBoundaries{Production: ProductionRequiresReview, Quota: QuotaProviderAccount, Expiry: ExpiryCredentialOrAccount},
		Implementation: Implementation{
			Status:       ImplementationImplementedOffline,
			Callable:     true,
			AdapterID:    "gemini",
			CodePaths:    []string{"provider/gemini"},
			TestEvidence: []string{"tests/gemini", "tests/gemini/provider_contract_test.go"},
		},
	}
	return finalizeDefaultEntry(entry)
}

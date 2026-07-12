package catalog

import (
	"fmt"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

var (
	subscriptionCheckedAt  = time.Date(2026, 7, 11, 8, 8, 0, 0, time.UTC)
	subscriptionValidUntil = subscriptionCheckedAt.Add(7 * 24 * time.Hour)
)

type subscriptionSpec struct {
	provider          upstream.ProviderID
	offering          upstream.OfferingID
	kind              upstream.OfferingKind
	canonicalFamily   string
	modelRef          string
	models            []string
	deployment        upstream.DeploymentID
	region            string
	credential        upstream.CredentialProfileID
	credentialEnv     string
	keyPrefixes       []string
	allowedClients    []string
	allowedUsage      upstream.AllowedUsage
	restrictions      []string
	source            OfficialSource
	modelSource       OfficialSource
	additionalSources []OfficialSource
	evidence          EvidenceStatus
	implementation    ImplementationStatus
	adapterID         string
	hostActivation    HostActivationRequirement
	endpointHost      string
	endpointIDs       []upstream.EndpointID
	endpointBasePaths map[upstream.ProtocolID]string
	authOverrides     map[upstream.ProtocolID]subscriptionAuthOverride
}

type subscriptionAuthOverride struct {
	credential upstream.CredentialProfileID
	header     string
	scheme     string
}

func subscriptionEntries() []Entry {
	var entries []Entry
	addPair := func(spec subscriptionSpec, protocols ...upstream.ProtocolID) {
		for index, protocolID := range protocols {
			entries = append(entries, subscriptionEntry(spec, protocolID, spec.endpointIDs[index], spec.endpointBasePaths[protocolID]))
		}
	}

	addPair(subscriptionSpec{
		provider: "zai", offering: "zai.glm-coding-plan", kind: upstream.OfferingCodingPlan,
		canonicalFamily: "glm", modelRef: "runtime_selected",
		deployment: "zai.glm-coding-plan.cn", region: "cn",
		credential: "zai.glm-coding-plan.cn", credentialEnv: "GLM_CODING_PLAN_API_KEY",
		allowedClients: []string{"claude-code", "claude-for-ide", "cline", "opencode", "roo-code", "kilo-code", "cursor", "crush", "goose", "trae", "qoder", "droid", "eigent", "openclaw", "hermes-agent", "sillytavern"},
		restrictions:   []string{"only the officially listed supported tools and products may use this plan", "Praxis is not listed as a supported product"},
		allowedUsage:   upstream.AllowedUsageOfficialClientOnly,
		source:         OfficialSource{ID: "zai.glm-coding-plan.usage-policy.2026-07-11", Publisher: "Z.AI", Kind: SourceTerms, URL: "https://docs.z.ai/devpack/usage-policy.md"},
		additionalSources: []OfficialSource{
			{ID: "zai.glm-coding-plan.supported-tools.2026-07-11", Publisher: "Z.AI", Kind: SourceTerms, URL: "https://docs.z.ai/devpack/tool/others.md"},
			{ID: "zai.glm-coding-plan.quick-start.2026-07-11", Publisher: "Z.AI", Kind: SourceProductDocs, URL: "https://docs.z.ai/devpack/quick-start.md"},
		},
		evidence: EvidenceUnverified, implementation: ImplementationResearchOnly,
		endpointHost: "open.bigmodel.cn", endpointIDs: []upstream.EndpointID{"zai.glm-coding-plan.cn.openai"},
		endpointBasePaths: map[upstream.ProtocolID]string{upstream.ProtocolChatCompletions: "/api/coding/paas/v4"},
	}, upstream.ProtocolChatCompletions)

	addPair(subscriptionSpec{
		provider: "kimi", offering: "kimi.code-membership", kind: upstream.OfferingCodingPlan,
		canonicalFamily: "kimi", modelRef: "kimi-for-coding", models: []string{"kimi-for-coding"},
		deployment: "kimi.code-membership.global", region: "global",
		credential: "kimi.code-membership.global", credentialEnv: "KIMI_CODE_API_KEY",
		restrictions: []string{"personal interactive coding or agent use only", "preserve the real client User-Agent", "product backends must use the pay-as-you-go Kimi platform"},
		source:       OfficialSource{ID: "kimi.code-membership.third-party-agents.2026-07-11", Publisher: "Kimi", Kind: SourceProductDocs, URL: "https://www.kimi.com/code/docs/third-party-tools/other-coding-agents.html"},
		additionalSources: []OfficialSource{
			{ID: "kimi.code-membership.community-guidelines.2026-07-11", Publisher: "Kimi", Kind: SourceTerms, URL: "https://www.kimi.com/code/docs/kimi-code/community-guidelines.html"},
		},
		evidence: EvidenceFresh, implementation: ImplementationImplementedOffline, adapterID: "kimi-code",
		hostActivation: HostActivationTrustedSubscriptionAuthorizationResolver,
		endpointHost:   "api.kimi.com", endpointIDs: []upstream.EndpointID{"kimi.code-membership.openai", "kimi.code-membership.anthropic"},
		endpointBasePaths: map[upstream.ProtocolID]string{upstream.ProtocolChatCompletions: "/coding/v1", upstream.ProtocolMessages: "/coding"},
		authOverrides: map[upstream.ProtocolID]subscriptionAuthOverride{
			upstream.ProtocolMessages: {credential: "kimi.code-membership.global.messages", header: "x-api-key"},
		},
	}, upstream.ProtocolChatCompletions, upstream.ProtocolMessages)

	addPair(subscriptionSpec{
		provider: "minimax", offering: "minimax.token-plan", kind: upstream.OfferingTokenPlan,
		canonicalFamily: "minimax-m", modelRef: "MiniMax-M3", models: []string{"MiniMax-M3", "MiniMax-M2.7", "MiniMax-M2.7-highspeed"},
		deployment: "minimax.token-plan.global", region: "global",
		credential: "minimax.token-plan.global", credentialEnv: "MINIMAX_TOKEN_PLAN_API_KEY", keyPrefixes: []string{"sk-cp-"},
		restrictions: []string{"interactive AI agent and coding tool use only", "Token Plan key is not interchangeable with pay-as-you-go keys"},
		source:       OfficialSource{ID: "minimax.token-plan.overview.2026-07-11", Publisher: "MiniMax", Kind: SourceProductDocs, URL: "https://platform.minimax.io/docs/token-plan/intro"},
		modelSource:  OfficialSource{ID: "minimax.token-plan.models.2026-07-11", Publisher: "MiniMax", Kind: SourceModelCatalog, URL: "https://platform.minimax.io/docs/guides/text-generation"},
		additionalSources: []OfficialSource{
			{ID: "minimax.token-plan.other-tools.2026-07-11", Publisher: "MiniMax", Kind: SourceProductDocs, URL: "https://platform.minimax.io/docs/token-plan/other-tools"},
		},
		evidence: EvidenceFresh, implementation: ImplementationImplementedOffline, adapterID: "minimax-token-plan",
		hostActivation: HostActivationTrustedSubscriptionAuthorizationResolver,
		endpointHost:   "api.minimax.io", endpointIDs: []upstream.EndpointID{"minimax.token-plan.openai", "minimax.token-plan.anthropic"},
		endpointBasePaths: map[upstream.ProtocolID]string{upstream.ProtocolChatCompletions: "/v1", upstream.ProtocolMessages: "/anthropic"},
		authOverrides: map[upstream.ProtocolID]subscriptionAuthOverride{
			upstream.ProtocolMessages: {credential: "minimax.token-plan.global.messages", header: "x-api-key"},
		},
	}, upstream.ProtocolChatCompletions, upstream.ProtocolMessages)

	for _, region := range []struct {
		id, host string
	}{{"cn", "token-plan-cn.xiaomimimo.com"}, {"sgp", "token-plan-sgp.xiaomimimo.com"}, {"ams", "token-plan-ams.xiaomimimo.com"}} {
		addPair(subscriptionSpec{
			provider: "xiaomi.mimo", offering: "mimo.token-plan", kind: upstream.OfferingTokenPlan,
			canonicalFamily: "mimo", modelRef: "mimo-v2.5", models: []string{"mimo-v2.5", "mimo-v2.5-pro"},
			deployment: upstream.DeploymentID("mimo.token-plan." + region.id), region: region.id,
			credential: upstream.CredentialProfileID("mimo.token-plan." + region.id), credentialEnv: "MIMO_TOKEN_PLAN_API_KEY", keyPrefixes: []string{"tp-"},
			restrictions: []string{"programming tool use only", "automated scripts, custom application backends, and non-coding API use are prohibited"},
			source:       OfficialSource{ID: "mimo.token-plan.subscription.2026-07-11", Publisher: "Xiaomi MiMo", Kind: SourceTerms, URL: "https://platform.xiaomimimo.com/static/docs/price/tokenplan/subscription.md"},
			evidence:     EvidenceFresh, implementation: ImplementationImplementedOffline, adapterID: "mimo-token-plan",
			hostActivation:    HostActivationTrustedSubscriptionAuthorizationResolver,
			endpointHost:      region.host,
			endpointIDs:       []upstream.EndpointID{upstream.EndpointID("mimo.token-plan." + region.id + ".openai"), upstream.EndpointID("mimo.token-plan." + region.id + ".anthropic")},
			endpointBasePaths: map[upstream.ProtocolID]string{upstream.ProtocolChatCompletions: "/v1", upstream.ProtocolMessages: "/anthropic"},
		}, upstream.ProtocolChatCompletions, upstream.ProtocolMessages)
	}

	for _, region := range []struct {
		id, host, sourceURL string
		models              []string
	}{
		{"cn", "coding.dashscope.aliyuncs.com", "https://help.aliyun.com/en/model-studio/coding-plan", alibabaCodingPlanModels("cn")},
		{"intl", "coding-intl.dashscope.aliyuncs.com", "https://www.alibabacloud.com/help/en/model-studio/coding-plan", alibabaCodingPlanModels("intl")},
	} {
		addPair(subscriptionSpec{
			provider: "alibaba.model-studio", offering: "alibaba.coding-plan", kind: upstream.OfferingCodingPlan,
			canonicalFamily: "multi-model", modelRef: region.models[0], models: region.models,
			deployment: upstream.DeploymentID("alibaba.coding-plan." + region.id), region: region.id,
			credential: upstream.CredentialProfileID("alibaba.coding-plan." + region.id), credentialEnv: "ALIBABA_CODING_PLAN_API_KEY", keyPrefixes: []string{"sk-sp-"},
			restrictions: []string{"interactive AI programming tools and OpenClaw-type agents only", "automated scripts, workflow platforms, API test tools, and application backends are prohibited"},
			source:       OfficialSource{ID: "alibaba.coding-plan." + region.id + ".exact-models.2026-07-11", Publisher: "Alibaba Cloud Model Studio", Kind: SourceModelCatalog, URL: region.sourceURL},
			evidence:     EvidenceFresh, implementation: ImplementationImplementedOffline, adapterID: "alibaba-plan",
			hostActivation:    HostActivationTrustedSubscriptionAuthorizationResolver,
			endpointHost:      region.host,
			endpointIDs:       []upstream.EndpointID{upstream.EndpointID("alibaba.coding-plan." + region.id + ".openai"), upstream.EndpointID("alibaba.coding-plan." + region.id + ".anthropic")},
			endpointBasePaths: map[upstream.ProtocolID]string{upstream.ProtocolChatCompletions: "/v1", upstream.ProtocolMessages: "/apps/anthropic"},
		}, upstream.ProtocolChatCompletions, upstream.ProtocolMessages)
	}

	addPair(subscriptionSpec{
		provider: "alibaba.model-studio", offering: "alibaba.token-plan-team", kind: upstream.OfferingTokenPlan,
		canonicalFamily: "multi-model", modelRef: "qwen3.7-max", models: alibabaTokenPlanTeamModels(),
		deployment: "alibaba.token-plan-team.cn-beijing", region: "cn-beijing",
		credential: "alibaba.token-plan-team.cn-beijing", credentialEnv: "ALIBABA_TOKEN_PLAN_API_KEY", keyPrefixes: []string{"sk-sp-"},
		restrictions: []string{"programming tools and OpenClaw-type agents only", "workflow automation, API test tools, custom applications, and non-interactive backends are prohibited"},
		source:       OfficialSource{ID: "alibaba.token-plan-team.quick-start.2026-07-11", Publisher: "Alibaba Cloud Model Studio", Kind: SourceProductDocs, URL: "https://help.aliyun.com/en/model-studio/token-plan-quickstart"},
		modelSource:  OfficialSource{ID: "alibaba.token-plan-team.exact-models.2026-07-11", Publisher: "Alibaba Cloud Model Studio", Kind: SourceModelCatalog, URL: "https://help.aliyun.com/en/model-studio/token-plan-overview"},
		evidence:     EvidenceFresh, implementation: ImplementationImplementedOffline, adapterID: "alibaba-plan",
		hostActivation:    HostActivationTrustedSubscriptionAuthorizationResolver,
		endpointHost:      "token-plan.cn-beijing.maas.aliyuncs.com",
		endpointIDs:       []upstream.EndpointID{"alibaba.token-plan-team.openai", "alibaba.token-plan-team.anthropic"},
		endpointBasePaths: map[upstream.ProtocolID]string{upstream.ProtocolChatCompletions: "/compatible-mode/v1", upstream.ProtocolMessages: "/apps/anthropic"},
	}, upstream.ProtocolChatCompletions, upstream.ProtocolMessages)

	entries = append(entries, xAIConsumerEntry())
	return entries
}

func subscriptionEntry(spec subscriptionSpec, protocolID upstream.ProtocolID, endpointID upstream.EndpointID, basePath string) Entry {
	routeID := upstream.RouteID(string(spec.deployment) + "." + string(protocolID))
	credentialID := spec.credential
	authHeader, authScheme := "Authorization", "Bearer"
	allowedEndpointIDs := append([]upstream.EndpointID(nil), spec.endpointIDs...)
	if override, ok := spec.authOverrides[protocolID]; ok {
		credentialID, authHeader, authScheme = override.credential, override.header, override.scheme
		allowedEndpointIDs = []upstream.EndpointID{endpointID}
	}
	allowedUsage := spec.allowedUsage
	if allowedUsage == "" {
		allowedUsage = upstream.AllowedUsageInteractiveCodingOnly
	}
	entitlement := upstream.CommercialEntitlement{
		AllowedUsage: allowedUsage, RequiresExplicitContext: true,
		AllowsAutomaticPAYGSwitch: false, ClientRestrictions: append([]string(nil), spec.restrictions...),
		SubjectPolicy: upstream.SubjectPolicyPersonalOnly, TenancyPolicy: upstream.TenancyPolicySingleTenantOnly,
		ExecutionPolicy: upstream.ExecutionPolicyForegroundOnly, ProductionPolicy: upstream.ProductionPolicyForbidden,
		RequiresClientIdentity: true, AllowedClientNames: append([]string(nil), spec.allowedClients...),
	}
	entry := Entry{
		ID: routeID,
		Route: upstream.UpstreamRoute{
			ID:         routeID,
			Model:      upstream.ModelIdentity{CanonicalFamily: spec.canonicalFamily, ProviderModelRef: spec.modelRef},
			Provider:   spec.provider,
			Offering:   upstream.Offering{ID: spec.offering, Kind: spec.kind, Entitlement: entitlement},
			Deployment: upstream.Deployment{ID: spec.deployment, Kind: upstream.DeploymentDirect, Region: spec.region},
			Protocol:   upstream.ProtocolBinding{ID: protocolID},
			Endpoint:   upstream.Endpoint{ID: endpointID, Scheme: "https", HostTemplate: spec.endpointHost, BasePath: basePath, CredentialAudience: spec.endpointHost},
			Credential: upstream.CredentialProfile{
				ID: credentialID, Type: upstream.CredentialAPIKey,
				References: []upstream.CredentialReference{{Purpose: upstream.CredentialPurposeAPIKey, Store: "env", Name: spec.credentialEnv}},
				Audience:   spec.endpointHost, AuthPlacement: upstream.AuthPlacementHeader,
				AuthHeader: authHeader, AuthScheme: authScheme, KeyPrefixes: append([]string(nil), spec.keyPrefixes...),
				AllowedProviderIDs: []upstream.ProviderID{spec.provider}, AllowedOfferingIDs: []upstream.OfferingID{spec.offering},
				AllowedDeploymentIDs: []upstream.DeploymentID{spec.deployment}, AllowedRegions: []string{spec.region},
				AllowedEndpointIDs: allowedEndpointIDs, Lifecycle: upstream.CredentialLifecycleStatic,
			},
		},
		Maturity:       MaturityUnknown,
		ModelDiscovery: exactProviderModels(spec.models),
		Sources:        subscriptionSources(spec),
		Evidence:       Evidence{Status: spec.evidence, TTLClass: EvidenceTTL7Days, CheckedAt: subscriptionCheckedAt, ValidUntil: subscriptionValidUntil},
		SDKs:           []SDKMetadata{protocolSDKMetadata(protocolID)},
		Capabilities:   protocolPlanCapabilities(protocolID),
		IgnoredFields:  []string{}, ExtensionFields: []string{},
		StreamEvents: protocolStreamEvents(protocolID), ErrorDialect: protocolErrorDialect(protocolID),
		Boundaries: OperationalBoundaries{Production: ProductionProhibited, Quota: QuotaSubscriptionWindow, Expiry: ExpirySubscriptionPeriod},
		Implementation: Implementation{
			Status: spec.implementation, Callable: spec.adapterID != "" && spec.hostActivation == "", AdapterID: spec.adapterID,
			HostActivationRequirement: spec.hostActivation,
		},
	}
	if spec.adapterID != "" {
		entry.Capabilities = implementedPlanCapabilities(protocolID)
		entry.Implementation.CodePaths = []string{"provider/plancompat", "internal/compatprovider", "internal/protocol/" + protocolPackage(protocolID), "routegateway"}
		entry.Implementation.TestEvidence = []string{"tests/plancompat", "tests/routegateway", "tests/semanticmatrix"}
	}
	return finalizeDefaultEntry(entry)
}

func exactProviderModels(models []string) ModelDiscovery {
	if len(models) == 0 {
		return ModelDiscovery{Method: ModelDiscoveryRuntimeSelected, AliasPolicy: ModelAliasExactProviderID}
	}
	aliases := make([]ModelAlias, 0, len(models))
	for index, model := range models {
		aliases = append(aliases, ModelAlias{Alias: fmt.Sprintf("exact-%03d", index+1), ProviderModelRef: model, Stable: true})
	}
	return ModelDiscovery{Method: ModelDiscoveryStaticCatalog, AliasPolicy: ModelAliasExactProviderID, Aliases: aliases}
}

func subscriptionSources(spec subscriptionSpec) []OfficialSource {
	sources := []OfficialSource{spec.source}
	if spec.modelSource.ID != "" {
		sources = append(sources, spec.modelSource)
	}
	sources = append(sources, spec.additionalSources...)
	return sources
}

func alibabaCodingPlanModels(region string) []string {
	// Keep the region branches separate even while the official 2026-07-11
	// exact-string lists are equal. A later regional drift must update one
	// Route family without silently changing the other.
	switch region {
	case "cn":
		return []string{"qwen3.7-plus", "qwen3.6-plus", "kimi-k2.5", "glm-5", "MiniMax-M2.5", "qwen3.5-plus", "qwen3-max-2026-01-23", "qwen3-coder-next", "qwen3-coder-plus", "glm-4.7"}
	case "intl":
		return []string{"qwen3.7-plus", "qwen3.6-plus", "kimi-k2.5", "glm-5", "MiniMax-M2.5", "qwen3.5-plus", "qwen3-max-2026-01-23", "qwen3-coder-next", "qwen3-coder-plus", "glm-4.7"}
	default:
		return nil
	}
}

func alibabaTokenPlanTeamModels() []string {
	return []string{
		"qwen3.7-max", "qwen3.7-plus", "qwen3.6-plus", "qwen3.6-flash",
		"deepseek-v4-pro", "deepseek-v4-flash", "deepseek-v3.2",
		"kimi-k2.7-code", "kimi-k2.6", "kimi-k2.5",
		"glm-5.2", "glm-5.1", "glm-5", "MiniMax-M2.5",
	}
}

func protocolSDKMetadata(protocolID upstream.ProtocolID) SDKMetadata {
	if protocolID == upstream.ProtocolMessages {
		return SDKMetadata{
			Language: "go", Package: "github.com/anthropics/anthropic-sdk-go", Owner: "Anthropic",
			Ownership: SDKOwnershipProtocolUpstream, Transport: TransportSDK,
			Version: "v1.56.0", License: "MIT", Official: true,
		}
	}
	return SDKMetadata{
		Language: "go", Package: "github.com/openai/openai-go/v3", Owner: "OpenAI",
		Ownership: SDKOwnershipProtocolUpstream, Transport: TransportSDK,
		Version: "v3.41.1", License: "Apache-2.0", Official: true,
	}
}

func protocolPlanCapabilities(protocolID upstream.ProtocolID) []CapabilityMetadata {
	overrides := map[string]CapabilityMetadata{
		"text_generation":       {Support: CapabilityCompatible},
		"streaming":             {Support: CapabilityCompatible},
		"tool_calling":          {Support: CapabilityCompatible},
		"parallel_tool_calling": {Support: CapabilityUnknown, Limitations: []string{"requires route-specific compatibility testing"}},
		"structured_output":     {Support: CapabilityUnknown, Limitations: []string{"requires route-specific compatibility testing"}},
		"reasoning":             {Support: CapabilityPartial, Limitations: []string{"provider-specific reasoning fields require a dialect design"}},
		"usage_reporting":       {Support: CapabilityUnknown, Limitations: []string{"subscription quota accounting is not inferred from protocol usage"}},
	}
	if protocolID == upstream.ProtocolMessages {
		overrides["provider_continuation"] = CapabilityMetadata{Support: CapabilityUnknown, Limitations: []string{"thinking and tool continuation require route-specific fixtures"}}
	}
	return completeCapabilities(overrides)
}

func implementedPlanCapabilities(protocolID upstream.ProtocolID) []CapabilityMetadata {
	overrides := map[string]CapabilityMetadata{
		"text_generation":       {Support: CapabilityCompatible},
		"streaming":             {Support: CapabilityCompatible},
		"tool_calling":          {Support: CapabilityCompatible},
		"parallel_tool_calling": {Support: CapabilityUnsupported, Limitations: []string{"parallel tool control is not approved in the restricted subscription slice"}},
		"structured_output":     {Support: CapabilityUnsupported, Limitations: []string{"structured output is not approved in the restricted subscription slice"}},
		"reasoning":             {Support: CapabilityUnsupported, Limitations: []string{"portable reasoning controls require plan-specific fixtures"}},
		"usage_reporting":       {Support: CapabilityCompatible, Limitations: []string{"token usage is preserved but does not represent subscription quota"}},
	}
	if protocolID == upstream.ProtocolMessages {
		overrides["function_error_result"] = CapabilityMetadata{Support: CapabilityCompatible}
	} else {
		overrides["function_error_result"] = CapabilityMetadata{Support: CapabilityPartial, Limitations: []string{"Chat preserves result text but not the portable is_error marker"}}
	}
	return completeCapabilities(overrides)
}

func protocolStreamEvents(protocolID upstream.ProtocolID) []string {
	if protocolID == upstream.ProtocolMessages {
		return []string{"message_start", "content_block_delta", "message_delta", "message_stop", "error"}
	}
	return []string{"chat.completion.chunk", "done", "error"}
}

func protocolErrorDialect(protocolID upstream.ProtocolID) ErrorDialect {
	if protocolID == upstream.ProtocolMessages {
		return ErrorDialect{Envelope: "anthropic-compatible.error", CodeField: "error.type", RequestIDHeaders: []string{"request-id", "x-request-id"}, RetryHeaders: []string{"retry-after"}}
	}
	return ErrorDialect{Envelope: "openai-compatible.error", CodeField: "error.code", RequestIDHeaders: []string{"x-request-id", "request-id"}, RetryHeaders: []string{"retry-after"}}
}

func xAIConsumerEntry() Entry {
	routeID := upstream.RouteID("xai.consumer-subscription.grok-build.external_agent_acp")
	entry := Entry{
		ID: routeID,
		Route: upstream.UpstreamRoute{
			ID:       routeID,
			Model:    upstream.ModelIdentity{CanonicalFamily: "grok", ProviderModelRef: "grok-build"},
			Provider: "xai.consumer",
			Offering: upstream.Offering{
				ID: "xai.consumer-subscription", Kind: upstream.OfferingCodingPlan,
				Entitlement: upstream.CommercialEntitlement{
					AllowedUsage: upstream.AllowedUsageOfficialClientOnly, RequiresExplicitContext: true,
					ClientRestrictions: []string{"no public subscription API key and Base URL contract for model-invoker"},
					SubjectPolicy:      upstream.SubjectPolicyPersonalOnly, TenancyPolicy: upstream.TenancyPolicySingleTenantOnly,
					ExecutionPolicy: upstream.ExecutionPolicyForegroundOnly, ProductionPolicy: upstream.ProductionPolicyForbidden,
					RequiresClientIdentity: true,
				},
			},
			Deployment: upstream.Deployment{ID: "xai.grok-build.external", Kind: upstream.DeploymentDirect, Region: "global"},
			Protocol:   upstream.ProtocolBinding{ID: "external_agent_acp"},
			Endpoint:   upstream.Endpoint{ID: "xai.grok-build.product", Scheme: "https", HostTemplate: "grok.com", CredentialAudience: "grok.com"},
			Credential: upstream.CredentialProfile{
				ID: "xai.grok-build.product-login", Type: upstream.CredentialAnonymous,
				Audience: "grok.com", AuthPlacement: upstream.AuthPlacementNone, Lifecycle: upstream.CredentialLifecycleAnonymous,
				AllowedProviderIDs: []upstream.ProviderID{"xai.consumer"}, AllowedOfferingIDs: []upstream.OfferingID{"xai.consumer-subscription"},
				AllowedDeploymentIDs: []upstream.DeploymentID{"xai.grok-build.external"}, AllowedRegions: []string{"global"},
				AllowedEndpointIDs: []upstream.EndpointID{"xai.grok-build.product"},
			},
		},
		Maturity:       MaturityUnknown,
		ModelDiscovery: ModelDiscovery{Method: ModelDiscoveryStaticCatalog, AliasPolicy: ModelAliasStable, Aliases: []ModelAlias{{Alias: "grok-build", ProviderModelRef: "grok-build", Stable: true}}},
		Sources:        []OfficialSource{{ID: "xai.grok-build.product.2026-07-11", Publisher: "xAI", Kind: SourceProductDocs, URL: "https://grok.com"}},
		Evidence:       Evidence{Status: EvidenceUnverified, TTLClass: EvidenceTTL7Days, CheckedAt: subscriptionCheckedAt, ValidUntil: subscriptionValidUntil},
		SDKs:           []SDKMetadata{{Language: "external", Package: "grok-build", Owner: "xAI", Ownership: SDKOwnershipProviderNative, Transport: TransportSidecar, Version: "unverified", License: "proprietary", Official: true}},
		Capabilities: completeCapabilities(map[string]CapabilityMetadata{
			"text_generation": {Support: CapabilityUnknown, Limitations: []string{"consumer product is not a model-invoker API contract"}},
			"streaming":       {Support: CapabilityUnknown, Limitations: []string{"consumer product is not a model-invoker API contract"}},
			"tool_calling":    {Support: CapabilityUnknown, Limitations: []string{"external Agent or ACP behavior requires a separate module design"}},
		}),
		IgnoredFields: []string{}, ExtensionFields: []string{}, StreamEvents: []string{"external_agent_event"},
		ErrorDialect:   ErrorDialect{Envelope: "external-agent.error", CodeField: "error.code", RequestIDHeaders: []string{"x-request-id"}},
		Boundaries:     OperationalBoundaries{Production: ProductionProhibited, Quota: QuotaSubscriptionWindow, Expiry: ExpirySubscriptionPeriod},
		Implementation: Implementation{Status: ImplementationResearchOnly, Callable: false},
	}
	return finalizeDefaultEntry(entry)
}

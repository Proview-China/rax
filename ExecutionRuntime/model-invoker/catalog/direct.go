package catalog

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

var (
	directCheckedAt  = time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)
	directValidUntil = directCheckedAt.Add(7 * 24 * time.Hour)
)

func directEntries() []Entry {
	entries := deepSeekEntries()
	entries = append(entries, kimiPlatformEntry())
	entries = append(entries, zaiPlatformEntry())
	entries = append(entries, minimaxPlatformEntries()...)
	entries = append(entries, mimoPlatformEntries()...)
	entries = append(entries, qwenPlatformEntries()...)
	entries = append(entries, xAIAPIEntry())
	return entries
}

func xAIAPIEntry() Entry {
	const providerID upstream.ProviderID = "xai.api"
	offering := cloudPAYGOffering("xai.api.payg")
	deployment := upstream.Deployment{ID: "xai.api.global", Kind: upstream.DeploymentDirect, Region: "global"}
	endpoint := upstream.Endpoint{ID: "xai.api.global.responses", Scheme: "https", HostTemplate: "api.x.ai", BasePath: "/v1", CredentialAudience: "api.x.ai"}
	credential := upstream.CredentialProfile{
		ID: "xai.api.global.responses", Type: upstream.CredentialAPIKey,
		References: []upstream.CredentialReference{{Purpose: upstream.CredentialPurposeAPIKey, Store: "env", Name: "XAI_API_KEY"}},
		Audience:   endpoint.CredentialAudience, AuthPlacement: upstream.AuthPlacementHeader, AuthHeader: "Authorization", AuthScheme: "Bearer", Lifecycle: upstream.CredentialLifecycleStatic,
		AllowedProviderIDs: []upstream.ProviderID{providerID}, AllowedOfferingIDs: []upstream.OfferingID{offering.ID},
		AllowedDeploymentIDs: []upstream.DeploymentID{deployment.ID}, AllowedRegions: []string{"global"}, AllowedEndpointIDs: []upstream.EndpointID{endpoint.ID},
	}
	routeID := upstream.RouteID("xai.api.global.payg.responses")
	entry := Entry{
		ID: routeID,
		Route: upstream.UpstreamRoute{
			ID: routeID, Model: upstream.ModelIdentity{CanonicalFamily: "grok", ProviderModelRef: "grok-4.5"},
			Provider: providerID, Offering: offering, Deployment: deployment,
			Protocol: upstream.ProtocolBinding{ID: upstream.ProtocolResponses, APIVersion: "xai-responses-2026-07-11"}, Endpoint: endpoint, Credential: credential,
		},
		Maturity:       MaturityUnknown,
		ModelDiscovery: ModelDiscovery{Method: ModelDiscoveryStaticCatalog, AliasPolicy: ModelAliasExactProviderID, Aliases: []ModelAlias{{Alias: "grok-4.5", ProviderModelRef: "grok-4.5", Stable: true}}},
		Sources: []OfficialSource{
			{ID: "xai.responses.2026-07-11", Publisher: "xAI", Kind: SourceAPIReference, URL: "https://docs.x.ai/developers/rest-api-reference/inference/chat"},
			{ID: "xai.grok-4.5.2026-07-11", Publisher: "xAI", Kind: SourceProductDocs, URL: "https://docs.x.ai/developers/grok-4-5"},
			{ID: "xai.function-calling.2026-07-11", Publisher: "xAI", Kind: SourceProductDocs, URL: "https://docs.x.ai/developers/tools/function-calling"},
			{ID: "xai.reasoning.2026-07-11", Publisher: "xAI", Kind: SourceProductDocs, URL: "https://docs.x.ai/developers/model-capabilities/text/reasoning"},
			{ID: "xai.errors.2026-07-11", Publisher: "xAI", Kind: SourceAPIReference, URL: "https://docs.x.ai/developers/debugging"},
			{ID: "xai.prompt-cache.2026-07-11", Publisher: "xAI", Kind: SourceProductDocs, URL: "https://docs.x.ai/developers/advanced-api-usage/prompt-caching/maximizing-cache-hits"},
		},
		Evidence: Evidence{Status: EvidenceFresh, TTLClass: EvidenceTTL7Days, CheckedAt: directCheckedAt, ValidUntil: directValidUntil},
		SDKs:     []SDKMetadata{openAISDK()},
		Capabilities: completeCapabilities(map[string]CapabilityMetadata{
			"text_generation":       {Support: CapabilityCompatible},
			"streaming":             {Support: CapabilityCompatible},
			"tool_calling":          {Support: CapabilityCompatible, Limitations: []string{"client-side function tools only; at most 128 tools"}},
			"parallel_tool_calling": {Support: CapabilityCompatible},
			"function_error_result": {Support: CapabilityPartial, Limitations: []string{"portable is_error is unavailable; explicit degradation preserves output text"}},
			"reasoning":             {Support: CapabilityCompatible, Limitations: []string{"grok-4.5 allows low, medium, or high and cannot disable reasoning"}},
			"reasoning_summary":     {Support: CapabilityUnsupported, Limitations: []string{"encrypted reasoning is not exposed as readable portable summary"}},
			"server_state":          {Support: CapabilityCompatible, Limitations: []string{"Responses are stored for 30 days by default and continued with previous_response_id"}},
			"prompt_caching":        {Support: CapabilityCompatible, Limitations: []string{"prompt_cache_key provides sticky routing; cache hits remain best effort"}},
			"usage_reporting":       {Support: CapabilityCompatible, Limitations: []string{"token usage is normalized; cost_in_usd_ticks remains in Raw"}},
		}),
		IgnoredFields:   []string{"metadata", "structured_output", "strict", "hosted_tools", "reasoning.encrypted_content", "cost_in_usd_ticks"},
		ExtensionFields: []string{"prompt_cache_key", "reasoning.effort", "previous_response_id"},
		StreamEvents:    []string{"response.created", "response.in_progress", "response.output_item.added", "response.output_text.delta", "response.reasoning_summary_text.delta", "response.function_call_arguments.delta", "response.completed", "error"},
		ErrorDialect:    ErrorDialect{Envelope: "openai.error", CodeField: "error.code", RequestIDHeaders: []string{"x-request-id", "request-id"}, RetryHeaders: []string{"retry-after"}},
		Boundaries:      OperationalBoundaries{Production: ProductionRequiresReview, Quota: QuotaProviderAccount, Expiry: ExpiryCredentialOrAccount},
		Implementation:  Implementation{Status: ImplementationImplementedOffline, Callable: true, AdapterID: "xai", CodePaths: []string{"provider/xai", "internal/compatprovider", "internal/protocol/openairesponses"}, TestEvidence: []string{"tests/xai"}},
	}
	return finalizeDefaultEntry(entry)
}

func qwenPlatformEntries() []Entry {
	const providerID upstream.ProviderID = "alibaba.model-studio"
	models := []string{"qwen3.7-max", "qwen3.7-plus", "qwen3.6-flash"}
	offering := cloudPAYGOffering("alibaba.model-studio.payg")
	sources := []OfficialSource{
		{ID: "alibaba.qwen.reference.2026-07-11", Publisher: "Alibaba Cloud Model Studio", Kind: SourceProductDocs, URL: "https://www.alibabacloud.com/help/en/model-studio/qwen-api-reference"},
		{ID: "alibaba.qwen.responses.2026-07-11", Publisher: "Alibaba Cloud Model Studio", Kind: SourceAPIReference, URL: "https://www.alibabacloud.com/help/en/model-studio/qwen-api-via-openai-responses"},
		{ID: "alibaba.qwen.chat.2026-07-11", Publisher: "Alibaba Cloud Model Studio", Kind: SourceAPIReference, URL: "https://www.alibabacloud.com/help/en/model-studio/qwen-api-via-openai-chat-completions"},
		{ID: "alibaba.base-url.2026-07-11", Publisher: "Alibaba Cloud Model Studio", Kind: SourceProductDocs, URL: "https://help.aliyun.com/en/model-studio/base-url"},
		{ID: "alibaba.regions.2026-07-11", Publisher: "Alibaba Cloud Model Studio", Kind: SourceProductDocs, URL: "https://help.aliyun.com/en/model-studio/regions/"},
		{ID: "alibaba.api-key.2026-07-11", Publisher: "Alibaba Cloud Model Studio", Kind: SourceProductDocs, URL: "https://www.alibabacloud.com/help/en/model-studio/get-api-key"},
		{ID: "alibaba.errors.2026-07-11", Publisher: "Alibaba Cloud Model Studio", Kind: SourceAPIReference, URL: "https://www.alibabacloud.com/help/en/model-studio/error-code"},
	}
	regions := []struct {
		id string
	}{
		{"cn-beijing"},
		{"ap-southeast-1"},
	}
	protocols := []upstream.ProtocolID{upstream.ProtocolResponses, upstream.ProtocolChatCompletions}
	entries := make([]Entry, 0, len(regions)*len(protocols))
	for _, region := range regions {
		deployment := upstream.Deployment{
			ID: upstream.DeploymentID("alibaba.model-studio." + region.id), Kind: upstream.DeploymentDirect,
			Region: region.id, WorkspaceRef: "runtime-workspace",
		}
		for _, protocolID := range protocols {
			routeID := upstream.RouteID("alibaba.model-studio." + region.id + ".payg." + string(protocolID))
			endpointID := upstream.EndpointID("alibaba.model-studio." + region.id + ".openai")
			host := "{workspace}." + region.id + ".maas.aliyuncs.com"
			endpoint := upstream.Endpoint{ID: endpointID, Scheme: "https", HostTemplate: host, BasePath: "/compatible-mode/v1", CredentialAudience: host}
			credential := upstream.CredentialProfile{
				ID: upstream.CredentialProfileID("alibaba.model-studio." + region.id + "." + string(protocolID)), Type: upstream.CredentialAPIKey,
				References: []upstream.CredentialReference{{Purpose: upstream.CredentialPurposeAPIKey, Store: "env", Name: "DASHSCOPE_API_KEY"}},
				Audience:   endpoint.CredentialAudience, AuthPlacement: upstream.AuthPlacementHeader, AuthHeader: "Authorization", AuthScheme: "Bearer",
				KeyPrefixes: []string{"sk-"}, DeniedKeyPrefixes: []string{"sk-sp-"}, Lifecycle: upstream.CredentialLifecycleStatic,
				AllowedProviderIDs: []upstream.ProviderID{providerID}, AllowedOfferingIDs: []upstream.OfferingID{offering.ID},
				AllowedDeploymentIDs: []upstream.DeploymentID{deployment.ID}, AllowedRegions: []string{region.id}, AllowedEndpointIDs: []upstream.EndpointID{endpoint.ID},
			}
			capabilities := completeCapabilities(map[string]CapabilityMetadata{
				"text_generation": {Support: CapabilityCompatible}, "streaming": {Support: CapabilityCompatible},
				"tool_calling":          {Support: CapabilityCompatible, Limitations: []string{"function tools only; required selection needs exactly one tool"}},
				"parallel_tool_calling": {Support: CapabilityUnsupported, Limitations: []string{"portable parallel-tool control is not approved in this slice"}},
				"reasoning":             {Support: CapabilityCompatible},
				"reasoning_summary":     {Support: CapabilityCompatible, Limitations: []string{"provider reasoning output is preserved without summary-style control"}},
				"function_error_result": {Support: CapabilityPartial, Limitations: []string{"portable is_error is unavailable; explicit degradation preserves output text"}},
				"usage_reporting":       {Support: CapabilityCompatible},
			})
			ignored := []string{"background", "metadata", "parallel_tool_calls", "strict"}
			extensions := []string{"reasoning.effort", "previous_response_id"}
			streamEvents := []string{"response.created", "response.in_progress", "response.output_item.added", "response.output_text.delta", "response.reasoning_summary_text.delta", "response.function_call_arguments.delta", "response.completed", "error"}
			if protocolID == upstream.ProtocolResponses {
				for index := range capabilities {
					switch capabilities[index].ID {
					case "server_state":
						capabilities[index] = CapabilityMetadata{ID: "server_state", Support: CapabilityCompatible, Limitations: []string{"previous_response_id is valid for seven days and remains identity-bound"}}
					case "structured_output":
						capabilities[index] = CapabilityMetadata{ID: "structured_output", Support: CapabilityUnsupported, Limitations: []string{"not approved in the current Responses slice"}}
					}
				}
			} else {
				for index := range capabilities {
					switch capabilities[index].ID {
					case "structured_output":
						capabilities[index] = CapabilityMetadata{ID: "structured_output", Support: CapabilityPartial, Limitations: []string{"JSON Object only and the prompt must explicitly request JSON"}}
					case "provider_continuation":
						capabilities[index] = CapabilityMetadata{ID: "provider_continuation", Support: CapabilityUnsupported, Limitations: []string{"portable Chat input cannot preserve reasoning_content history"}}
					}
				}
				ignored = []string{"metadata", "parallel_tool_calls", "reasoning_effort", "strict"}
				extensions = []string{"enable_thinking", "thinking_budget", "reasoning_content", "response_format.json_object"}
				streamEvents = []string{"chat.completion.chunk", "reasoning_content.delta", "usage", "done"}
			}
			entry := Entry{
				ID:       routeID,
				Route:    upstream.UpstreamRoute{ID: routeID, Model: upstream.ModelIdentity{CanonicalFamily: "qwen", ProviderModelRef: models[0]}, Provider: providerID, Offering: offering, Deployment: deployment, Protocol: upstream.ProtocolBinding{ID: protocolID, APIVersion: "model-studio-2026-07-11"}, Endpoint: endpoint, Credential: credential},
				Maturity: MaturityUnknown, ModelDiscovery: exactProviderModels(models),
				Sources: sources, Evidence: Evidence{Status: EvidenceFresh, TTLClass: EvidenceTTL7Days, CheckedAt: directCheckedAt, ValidUntil: directValidUntil},
				SDKs: []SDKMetadata{openAISDK()}, Capabilities: capabilities, IgnoredFields: ignored, ExtensionFields: extensions,
				StreamEvents: streamEvents, ErrorDialect: ErrorDialect{Envelope: "openai.error", CodeField: "error.code", RequestIDHeaders: []string{"x-request-id", "request-id", "x-dashscope-request-id"}, RetryHeaders: []string{"retry-after"}},
				Boundaries:     OperationalBoundaries{Production: ProductionRequiresReview, Quota: QuotaProviderAccount, Expiry: ExpiryCredentialOrAccount},
				Implementation: Implementation{Status: ImplementationImplementedOffline, Callable: true, AdapterID: "qwen", CodePaths: []string{"provider/qwen", "internal/compatprovider", "internal/protocol/" + protocolPackage(protocolID)}, TestEvidence: []string{"tests/qwen"}},
			}
			entries = append(entries, finalizeDefaultEntry(entry))
		}
	}
	return entries
}

func mimoPlatformEntries() []Entry {
	const providerID upstream.ProviderID = "xiaomi.mimo"
	models := []string{"mimo-v2.5-pro", "mimo-v2.5"}
	offering := cloudPAYGOffering("xiaomi.mimo.payg")
	deployment := upstream.Deployment{ID: "xiaomi.mimo.global", Kind: upstream.DeploymentDirect, Region: "global"}
	sources := []OfficialSource{
		{ID: "mimo.index.2026-07-11", Publisher: "Xiaomi MiMo", Kind: SourceProductDocs, URL: "https://platform.xiaomimimo.com/llms.txt"},
		{ID: "mimo.first-call.2026-07-11", Publisher: "Xiaomi MiMo", Kind: SourceProductDocs, URL: "https://platform.xiaomimimo.com/static/docs/quick-start/first-api-call.md"},
		{ID: "mimo.models.2026-07-11", Publisher: "Xiaomi MiMo", Kind: SourceProductDocs, URL: "https://platform.xiaomimimo.com/static/docs/quick-start/model.md"},
		{ID: "mimo.openai.2026-07-11", Publisher: "Xiaomi MiMo", Kind: SourceAPIReference, URL: "https://platform.xiaomimimo.com/static/docs/api/chat/openai-api.md"},
		{ID: "mimo.anthropic.2026-07-11", Publisher: "Xiaomi MiMo", Kind: SourceAPIReference, URL: "https://platform.xiaomimimo.com/static/docs/api/chat/anthropic-api.md"},
		{ID: "mimo.errors.2026-07-11", Publisher: "Xiaomi MiMo", Kind: SourceAPIReference, URL: "https://platform.xiaomimimo.com/static/docs/quick-start/error-codes.md"},
		{ID: "mimo.deprecation.2026-07-11", Publisher: "Xiaomi MiMo", Kind: SourceProductDocs, URL: "https://platform.xiaomimimo.com/static/docs/updates/deprecate.md"},
		{ID: "mimo.payg.2026-07-11", Publisher: "Xiaomi MiMo", Kind: SourceTerms, URL: "https://platform.xiaomimimo.com/static/docs/price/pay-as-you-go.md"},
		{ID: "mimo.token-plan.2026-07-11", Publisher: "Xiaomi MiMo", Kind: SourceTerms, URL: "https://platform.xiaomimimo.com/static/docs/price/tokenplan/quick-access.md"},
	}
	type definition struct {
		id         upstream.RouteID
		protocol   upstream.ProtocolID
		endpointID upstream.EndpointID
		basePath   string
		sdk        SDKMetadata
	}
	definitions := []definition{
		{"xiaomi.mimo.global.payg.messages", upstream.ProtocolMessages, "xiaomi.mimo.global.anthropic", "/anthropic", deepSeekAnthropicSDK()},
		{"xiaomi.mimo.global.payg.chat_completions", upstream.ProtocolChatCompletions, "xiaomi.mimo.global.chat", "/v1", openAISDK()},
	}
	entries := make([]Entry, 0, len(definitions))
	for _, item := range definitions {
		endpoint := upstream.Endpoint{ID: item.endpointID, Scheme: "https", HostTemplate: "api.xiaomimimo.com", BasePath: item.basePath, CredentialAudience: "api.xiaomimimo.com"}
		credential := upstream.CredentialProfile{
			ID: upstream.CredentialProfileID("xiaomi.mimo.global." + string(item.protocol)), Type: upstream.CredentialAPIKey,
			References: []upstream.CredentialReference{{Purpose: upstream.CredentialPurposeAPIKey, Store: "env", Name: "MIMO_API_KEY"}},
			Audience:   endpoint.CredentialAudience, AuthPlacement: upstream.AuthPlacementHeader, AuthHeader: "Authorization", AuthScheme: "Bearer", KeyPrefixes: []string{"sk-"}, Lifecycle: upstream.CredentialLifecycleStatic,
			AllowedProviderIDs: []upstream.ProviderID{providerID}, AllowedOfferingIDs: []upstream.OfferingID{offering.ID}, AllowedDeploymentIDs: []upstream.DeploymentID{deployment.ID}, AllowedRegions: []string{"global"}, AllowedEndpointIDs: []upstream.EndpointID{endpoint.ID},
		}
		capabilities := completeCapabilities(map[string]CapabilityMetadata{
			"text_generation": {Support: CapabilityCompatible}, "streaming": {Support: CapabilityCompatible},
			"tool_calling":      {Support: CapabilityCompatible, Limitations: []string{"function tools with automatic selection only"}},
			"reasoning":         {Support: CapabilityCompatible, Limitations: []string{"portable effort maps only to an enabled or disabled thinking switch"}},
			"reasoning_summary": {Support: CapabilityCompatible, Limitations: []string{"provider reasoning output is preserved without summary-style control"}},
			"prompt_caching":    {Support: CapabilityPartial, Limitations: []string{"cache usage is preserved; cache creation is outside the portable request"}},
			"usage_reporting":   {Support: CapabilityCompatible},
		})
		ignored := []string{"output_config.effort", "thinking.budget_tokens", "thinking.display", "anthropic-beta"}
		extensions := []string{"thinking", "signature"}
		streamEvents := []string{"message_start", "content_block_start", "content_block_delta", "content_block_stop", "message_delta", "message_stop", "error"}
		errorDialect := ErrorDialect{Envelope: "anthropic.error", CodeField: "error.type", RequestIDHeaders: []string{"request-id", "x-request-id"}, RetryHeaders: []string{"retry-after"}}
		if item.protocol == upstream.ProtocolMessages {
			for index := range capabilities {
				switch capabilities[index].ID {
				case "parallel_tool_calling":
					capabilities[index] = CapabilityMetadata{ID: "parallel_tool_calling", Support: CapabilityCompatible, Limitations: []string{"disable_parallel_tool_use is supported with automatic selection"}}
				case "provider_continuation":
					capabilities[index] = CapabilityMetadata{ID: "provider_continuation", Support: CapabilityCompatible, Limitations: []string{"thinking signatures and tool blocks are preserved"}}
				case "function_error_result":
					capabilities[index] = CapabilityMetadata{ID: "function_error_result", Support: CapabilityCompatible}
				}
			}
		} else {
			for index := range capabilities {
				switch capabilities[index].ID {
				case "parallel_tool_calling":
					capabilities[index] = CapabilityMetadata{ID: "parallel_tool_calling", Support: CapabilityUnsupported, Limitations: []string{"MiMo Chat exposes no portable parallel control"}}
				case "structured_output":
					capabilities[index] = CapabilityMetadata{ID: "structured_output", Support: CapabilityPartial, Limitations: []string{"JSON Object only; strict JSON Schema output is unsupported"}}
				case "function_error_result":
					capabilities[index] = CapabilityMetadata{ID: "function_error_result", Support: CapabilityPartial, Limitations: []string{"portable is_error is unavailable; output text is preserved"}}
				}
			}
			ignored = []string{"reasoning_effort", "parallel_tool_calls"}
			extensions = []string{"thinking", "reasoning_content", "response_format.json_object"}
			streamEvents = []string{"chat.completion.chunk", "reasoning_content.delta", "done"}
			errorDialect = ErrorDialect{Envelope: "openai.error", CodeField: "error.code", RequestIDHeaders: []string{"x-request-id", "request-id"}, RetryHeaders: []string{"retry-after"}}
		}
		entry := Entry{
			ID:       item.id,
			Route:    upstream.UpstreamRoute{ID: item.id, Model: upstream.ModelIdentity{CanonicalFamily: "mimo", ProviderModelRef: models[0]}, Provider: providerID, Offering: offering, Deployment: deployment, Protocol: upstream.ProtocolBinding{ID: item.protocol, APIVersion: "mimo-2026-07-11"}, Endpoint: endpoint, Credential: credential},
			Maturity: MaturityUnknown, ModelDiscovery: exactProviderModels(models),
			Sources: sources, Evidence: Evidence{Status: EvidenceFresh, TTLClass: EvidenceTTL7Days, CheckedAt: directCheckedAt, ValidUntil: directValidUntil},
			SDKs: []SDKMetadata{item.sdk}, Capabilities: capabilities, IgnoredFields: ignored, ExtensionFields: extensions,
			StreamEvents: streamEvents, ErrorDialect: errorDialect,
			Boundaries:     OperationalBoundaries{Production: ProductionRequiresReview, Quota: QuotaProviderAccount, Expiry: ExpiryCredentialOrAccount},
			Implementation: Implementation{Status: ImplementationImplementedOffline, Callable: true, AdapterID: "xiaomi-mimo", CodePaths: []string{"provider/mimo", "internal/compatprovider", "internal/protocol/" + protocolPackage(item.protocol)}, TestEvidence: []string{"tests/mimo"}},
		}
		entries = append(entries, finalizeDefaultEntry(entry))
	}
	return entries
}

func minimaxPlatformEntries() []Entry {
	const providerID upstream.ProviderID = "minimax"
	models := []string{"MiniMax-M3", "MiniMax-M2.7", "MiniMax-M2.7-highspeed", "MiniMax-M2.5", "MiniMax-M2.5-highspeed", "MiniMax-M2.1", "MiniMax-M2.1-highspeed", "MiniMax-M2"}
	offering := cloudPAYGOffering("minimax.platform.payg")
	deployment := upstream.Deployment{ID: "minimax.platform.global", Kind: upstream.DeploymentDirect, Region: "global"}
	sources := []OfficialSource{
		{ID: "minimax.api.overview.2026-07-11", Publisher: "MiniMax", Kind: SourceProductDocs, URL: "https://platform.minimax.io/docs/api-reference/api-overview"},
		{ID: "minimax.anthropic.2026-07-11", Publisher: "MiniMax", Kind: SourceAPIReference, URL: "https://platform.minimax.io/docs/api-reference/text-anthropic-api"},
		{ID: "minimax.openai.2026-07-11", Publisher: "MiniMax", Kind: SourceAPIReference, URL: "https://platform.minimax.io/docs/api-reference/text-openai-api"},
		{ID: "minimax.responses.2026-07-11", Publisher: "MiniMax", Kind: SourceAPIReference, URL: "https://platform.minimax.io/docs/api-reference/responses-create"},
		{ID: "minimax.errors.2026-07-11", Publisher: "MiniMax", Kind: SourceAPIReference, URL: "https://platform.minimax.io/docs/api-reference/errorcode"},
	}
	type definition struct {
		id         upstream.RouteID
		protocol   upstream.ProtocolID
		endpointID upstream.EndpointID
		basePath   string
		authHeader string
		authScheme string
		sdk        SDKMetadata
	}
	definitions := []definition{
		{"minimax.platform.global.payg.messages", upstream.ProtocolMessages, "minimax.platform.global.anthropic", "/anthropic", "x-api-key", "", deepSeekAnthropicSDK()},
		{"minimax.platform.global.payg.chat_completions", upstream.ProtocolChatCompletions, "minimax.platform.global.chat", "/v1", "Authorization", "Bearer", openAISDK()},
		{"minimax.platform.global.payg.responses", upstream.ProtocolResponses, "minimax.platform.global.responses", "/v1", "Authorization", "Bearer", openAISDK()},
	}
	entries := make([]Entry, 0, len(definitions))
	for _, item := range definitions {
		endpoint := upstream.Endpoint{ID: item.endpointID, Scheme: "https", HostTemplate: "api.minimax.io", BasePath: item.basePath, CredentialAudience: "api.minimax.io"}
		credential := upstream.CredentialProfile{
			ID: upstream.CredentialProfileID("minimax.platform.global." + string(item.protocol)), Type: upstream.CredentialAPIKey,
			References: []upstream.CredentialReference{{Purpose: upstream.CredentialPurposeAPIKey, Store: "env", Name: "MINIMAX_API_KEY"}},
			Audience:   endpoint.CredentialAudience, AuthPlacement: upstream.AuthPlacementHeader, AuthHeader: item.authHeader, AuthScheme: item.authScheme, Lifecycle: upstream.CredentialLifecycleStatic,
			AllowedProviderIDs: []upstream.ProviderID{providerID}, AllowedOfferingIDs: []upstream.OfferingID{offering.ID}, AllowedDeploymentIDs: []upstream.DeploymentID{deployment.ID}, AllowedRegions: []string{"global"}, AllowedEndpointIDs: []upstream.EndpointID{endpoint.ID},
		}
		capabilities := completeCapabilities(map[string]CapabilityMetadata{
			"text_generation": {Support: CapabilityCompatible}, "streaming": {Support: CapabilityCompatible},
			"tool_calling":          {Support: CapabilityCompatible, Limitations: []string{"function tools with auto or none selection only"}},
			"parallel_tool_calling": {Support: CapabilityUnsupported, Limitations: []string{"portable parallel-tool control is not implemented"}},
			"structured_output":     {Support: CapabilityUnsupported, Limitations: []string{"only text output is approved in the current compatibility slice"}},
			"reasoning":             {Support: CapabilityCompatible, Limitations: []string{"M3 supports an on/off switch; M2.x thinking is always on"}},
			"reasoning_summary":     {Support: CapabilityCompatible, Limitations: []string{"provider reasoning output is preserved without summary-style control"}},
			"usage_reporting":       {Support: CapabilityCompatible},
		})
		ignored := []string{}
		extensions := []string{"thinking"}
		streamEvents := []string{"message_start", "content_block_start", "content_block_delta", "content_block_stop", "message_delta", "message_stop", "error"}
		errorDialect := ErrorDialect{Envelope: "anthropic.error", CodeField: "error.type", RequestIDHeaders: []string{"request-id", "x-request-id"}, RetryHeaders: []string{"retry-after"}}
		if item.protocol == upstream.ProtocolMessages {
			for index := range capabilities {
				if capabilities[index].ID == "provider_continuation" {
					capabilities[index] = CapabilityMetadata{ID: "provider_continuation", Support: CapabilityCompatible, Limitations: []string{"thinking signatures and tool blocks are preserved"}}
				}
				if capabilities[index].ID == "function_error_result" {
					capabilities[index] = CapabilityMetadata{ID: "function_error_result", Support: CapabilityCompatible}
				}
			}
		} else {
			for index := range capabilities {
				if capabilities[index].ID == "function_error_result" {
					capabilities[index] = CapabilityMetadata{ID: "function_error_result", Support: CapabilityPartial, Limitations: []string{"portable is_error is unavailable; explicit degradation preserves output text"}}
				}
			}
			errorDialect = ErrorDialect{Envelope: "openai.error_or_base_resp", CodeField: "error.code", RequestIDHeaders: []string{"x-request-id", "request-id"}, RetryHeaders: []string{"retry-after"}}
			if item.protocol == upstream.ProtocolChatCompletions {
				extensions = []string{"thinking", "reasoning_split", "reasoning_content", "reasoning_details"}
				streamEvents = []string{"chat.completion.chunk", "cumulative.reasoning_details", "cumulative.content", "done"}
			} else {
				extensions = []string{"reasoning", "store=false"}
				streamEvents = []string{"response.created", "response.output_text.delta", "response.reasoning_summary_text.delta", "response.function_call_arguments.delta", "response.completed", "error"}
			}
		}
		entry := Entry{
			ID:       item.id,
			Route:    upstream.UpstreamRoute{ID: item.id, Model: upstream.ModelIdentity{CanonicalFamily: "minimax-m", ProviderModelRef: models[0]}, Provider: providerID, Offering: offering, Deployment: deployment, Protocol: upstream.ProtocolBinding{ID: item.protocol, APIVersion: "minimax-2026-07-11"}, Endpoint: endpoint, Credential: credential},
			Maturity: MaturityUnknown, ModelDiscovery: exactProviderModels(models),
			Sources: sources, Evidence: Evidence{Status: EvidenceFresh, TTLClass: EvidenceTTL7Days, CheckedAt: directCheckedAt, ValidUntil: directValidUntil},
			SDKs: []SDKMetadata{item.sdk}, Capabilities: capabilities, IgnoredFields: ignored, ExtensionFields: extensions,
			StreamEvents: streamEvents, ErrorDialect: errorDialect,
			Boundaries:     OperationalBoundaries{Production: ProductionRequiresReview, Quota: QuotaProviderAccount, Expiry: ExpiryCredentialOrAccount},
			Implementation: Implementation{Status: ImplementationImplementedOffline, Callable: true, AdapterID: "minimax", CodePaths: []string{"provider/minimax", "internal/compatprovider", "internal/protocol/" + protocolPackage(item.protocol)}, TestEvidence: []string{"tests/minimax"}},
		}
		entries = append(entries, finalizeDefaultEntry(entry))
	}
	return entries
}

func zaiPlatformEntry() Entry {
	models := []string{"glm-5.2"}
	routeID := upstream.RouteID("zai.platform.global.payg.chat_completions")
	providerID := upstream.ProviderID("zai")
	offering := cloudPAYGOffering("zai.platform.payg")
	deployment := upstream.Deployment{ID: "zai.platform.global", Kind: upstream.DeploymentDirect, Region: "global"}
	endpoint := upstream.Endpoint{ID: "zai.platform.global.openai", Scheme: "https", HostTemplate: "api.z.ai", BasePath: "/api/paas/v4", CredentialAudience: "api.z.ai"}
	credential := upstream.CredentialProfile{
		ID: "zai.platform.global", Type: upstream.CredentialAPIKey,
		References: []upstream.CredentialReference{{Purpose: upstream.CredentialPurposeAPIKey, Store: "env", Name: "ZAI_API_KEY"}},
		Audience:   endpoint.CredentialAudience, AuthPlacement: upstream.AuthPlacementHeader, AuthHeader: "Authorization", AuthScheme: "Bearer", Lifecycle: upstream.CredentialLifecycleStatic,
		AllowedProviderIDs: []upstream.ProviderID{providerID}, AllowedOfferingIDs: []upstream.OfferingID{offering.ID}, AllowedDeploymentIDs: []upstream.DeploymentID{deployment.ID}, AllowedRegions: []string{"global"}, AllowedEndpointIDs: []upstream.EndpointID{endpoint.ID},
	}
	entry := Entry{
		ID:       routeID,
		Route:    upstream.UpstreamRoute{ID: routeID, Model: upstream.ModelIdentity{CanonicalFamily: "glm", ProviderModelRef: models[0]}, Provider: providerID, Offering: offering, Deployment: deployment, Protocol: upstream.ProtocolBinding{ID: upstream.ProtocolChatCompletions, APIVersion: "paas-v4"}, Endpoint: endpoint, Credential: credential},
		Maturity: MaturityUnknown, ModelDiscovery: exactProviderModels(models),
		Sources: []OfficialSource{
			{ID: "zai.introduction.2026-07-11", Publisher: "Z.AI", Kind: SourceProductDocs, URL: "https://docs.z.ai/api-reference/introduction"},
			{ID: "zai.chat.2026-07-11", Publisher: "Z.AI", Kind: SourceAPIReference, URL: "https://docs.z.ai/api-reference/llm/chat-completion"},
			{ID: "zai.thinking.2026-07-11", Publisher: "Z.AI", Kind: SourceProductDocs, URL: "https://docs.z.ai/guides/capabilities/thinking-mode"},
			{ID: "zai.function-calling.2026-07-11", Publisher: "Z.AI", Kind: SourceProductDocs, URL: "https://docs.z.ai/guides/capabilities/function-calling"},
			{ID: "zai.streaming.2026-07-11", Publisher: "Z.AI", Kind: SourceProductDocs, URL: "https://docs.z.ai/guides/capabilities/streaming"},
			{ID: "zai.errors.2026-07-11", Publisher: "Z.AI", Kind: SourceAPIReference, URL: "https://docs.z.ai/api-reference/api-code"},
		},
		Evidence: Evidence{Status: EvidenceFresh, TTLClass: EvidenceTTL7Days, CheckedAt: directCheckedAt, ValidUntil: directValidUntil},
		SDKs:     []SDKMetadata{openAISDK()},
		Capabilities: completeCapabilities(map[string]CapabilityMetadata{
			"text_generation": {Support: CapabilityCompatible}, "streaming": {Support: CapabilityCompatible}, "tool_calling": {Support: CapabilityCompatible, Limitations: []string{"function tools with tool_choice=auto only"}},
			"parallel_tool_calling": {Support: CapabilityPartial, Limitations: []string{"exact behavior is model-scoped and non-auto selection is unavailable"}},
			"structured_output":     {Support: CapabilityPartial, Limitations: []string{"JSON Object only; strict JSON Schema is not declared"}},
			"reasoning":             {Support: CapabilityCompatible, Limitations: []string{"GLM-4.5 and newer text models only"}}, "reasoning_summary": {Support: CapabilityCompatible, Limitations: []string{"reasoning_content is preserved as portable reasoning output"}},
			"provider_continuation": {Support: CapabilityUnsupported, Limitations: []string{"preserved thinking is disabled in the current standard API slice"}}, "usage_reporting": {Support: CapabilityCompatible},
		}),
		IgnoredFields: []string{}, ExtensionFields: []string{"thinking", "reasoning_content", "reasoning_effort", "tool_stream", "request_id", "web_search"},
		StreamEvents:   []string{"chat.completion.chunk", "reasoning_content.delta", "done", "sensitive", "model_context_window_exceeded", "network_error"},
		ErrorDialect:   ErrorDialect{Envelope: "zai.error", CodeField: "error.code", RequestIDHeaders: []string{"x-request-id", "request-id"}, RetryHeaders: []string{"retry-after"}},
		Boundaries:     OperationalBoundaries{Production: ProductionRequiresReview, Quota: QuotaProviderAccount, Expiry: ExpiryCredentialOrAccount},
		Implementation: Implementation{Status: ImplementationImplementedOffline, Callable: true, AdapterID: "zai", CodePaths: []string{"provider/zai", "internal/compatprovider", "internal/protocol/openaichat"}, TestEvidence: []string{"tests/zai"}},
	}
	return finalizeDefaultEntry(entry)
}

func kimiPlatformEntry() Entry {
	models := []string{"kimi-k2.7-code", "kimi-k2.7-code-highspeed", "kimi-k2.6", "kimi-k2.5", "moonshot-v1-8k", "moonshot-v1-32k", "moonshot-v1-128k"}
	routeID := upstream.RouteID("kimi.platform.cn.payg.chat_completions")
	providerID := upstream.ProviderID("kimi")
	offering := cloudPAYGOffering("kimi.platform.payg")
	deployment := upstream.Deployment{ID: "kimi.platform.cn", Kind: upstream.DeploymentDirect, Region: "cn"}
	endpoint := upstream.Endpoint{ID: "kimi.platform.cn.openai", Scheme: "https", HostTemplate: "api.moonshot.cn", BasePath: "/v1", CredentialAudience: "api.moonshot.cn"}
	credential := upstream.CredentialProfile{
		ID: "kimi.platform.cn", Type: upstream.CredentialAPIKey,
		References: []upstream.CredentialReference{{Purpose: upstream.CredentialPurposeAPIKey, Store: "env", Name: "MOONSHOT_API_KEY"}},
		Audience:   endpoint.CredentialAudience, AuthPlacement: upstream.AuthPlacementHeader, AuthHeader: "Authorization", AuthScheme: "Bearer", Lifecycle: upstream.CredentialLifecycleStatic,
		AllowedProviderIDs: []upstream.ProviderID{providerID}, AllowedOfferingIDs: []upstream.OfferingID{offering.ID}, AllowedDeploymentIDs: []upstream.DeploymentID{deployment.ID}, AllowedRegions: []string{"cn"}, AllowedEndpointIDs: []upstream.EndpointID{endpoint.ID},
	}
	entry := Entry{
		ID:       routeID,
		Route:    upstream.UpstreamRoute{ID: routeID, Model: upstream.ModelIdentity{CanonicalFamily: "kimi", ProviderModelRef: models[0]}, Provider: providerID, Offering: offering, Deployment: deployment, Protocol: upstream.ProtocolBinding{ID: upstream.ProtocolChatCompletions, APIVersion: "kimi-2026-07-11"}, Endpoint: endpoint, Credential: credential},
		Maturity: MaturityUnknown, ModelDiscovery: exactProviderModels(models),
		Sources: []OfficialSource{
			{ID: "kimi.platform.overview.2026-07-11", Publisher: "Moonshot AI", Kind: SourceProductDocs, URL: "https://platform.kimi.com/docs/api/overview"},
			{ID: "kimi.platform.models.2026-07-11", Publisher: "Moonshot AI", Kind: SourceProductDocs, URL: "https://platform.kimi.com/docs/models"},
			{ID: "kimi.platform.reasoning.2026-07-11", Publisher: "Moonshot AI", Kind: SourceAPIReference, URL: "https://platform.kimi.com/docs/guide/use-kimi-k2-thinking-model"},
			{ID: "kimi.platform.errors.2026-07-11", Publisher: "Moonshot AI", Kind: SourceAPIReference, URL: "https://platform.kimi.com/docs/api/errors"},
		},
		Evidence: Evidence{Status: EvidenceFresh, TTLClass: EvidenceTTL7Days, CheckedAt: directCheckedAt, ValidUntil: directValidUntil},
		SDKs:     []SDKMetadata{openAISDK()},
		Capabilities: completeCapabilities(map[string]CapabilityMetadata{
			"text_generation": {Support: CapabilityCompatible}, "streaming": {Support: CapabilityCompatible},
			"tool_calling":          {Support: CapabilityCompatible, Limitations: []string{"K2 models only in the current binding"}},
			"parallel_tool_calling": {Support: CapabilityCompatible, Limitations: []string{"K2 models only; exact support remains model-scoped"}},
			"structured_output":     {Support: CapabilityPartial, Limitations: []string{"JSON Object only; strict JSON Schema and Partial Mode are not implemented"}},
			"reasoning":             {Support: CapabilityCompatible, Limitations: []string{"K2 models only; effort maps to provider thinking behavior"}},
			"reasoning_summary":     {Support: CapabilityCompatible, Limitations: []string{"reasoning_content is preserved as the portable reasoning output"}},
			"provider_continuation": {Support: CapabilityUnsupported, Limitations: []string{"preserved thinking is not yet representable in portable Chat input"}},
			"usage_reporting":       {Support: CapabilityCompatible},
		}),
		IgnoredFields: []string{}, ExtensionFields: []string{"thinking", "reasoning_content", "messages[].partial"},
		StreamEvents:   []string{"chat.completion.chunk", "reasoning_content.delta", "done"},
		ErrorDialect:   ErrorDialect{Envelope: "openai.error", CodeField: "error.type", RequestIDHeaders: []string{"x-request-id", "request-id"}, RetryHeaders: []string{"retry-after"}},
		Boundaries:     OperationalBoundaries{Production: ProductionRequiresReview, Quota: QuotaProviderAccount, Expiry: ExpiryCredentialOrAccount},
		Implementation: Implementation{Status: ImplementationImplementedOffline, Callable: true, AdapterID: "kimi", CodePaths: []string{"provider/kimi", "internal/compatprovider", "internal/protocol/openaichat"}, TestEvidence: []string{"tests/kimi"}},
	}
	return finalizeDefaultEntry(entry)
}

func deepSeekEntries() []Entry {
	const providerID upstream.ProviderID = "deepseek"
	models := []string{"deepseek-v4-flash", "deepseek-v4-pro"}
	offering := cloudPAYGOffering("deepseek.api.payg")
	deployment := upstream.Deployment{ID: "deepseek.direct.global", Kind: upstream.DeploymentDirect, Region: "global"}
	endpoints := map[upstream.ProtocolID]upstream.Endpoint{
		upstream.ProtocolChatCompletions: {ID: "deepseek.openai", Scheme: "https", HostTemplate: "api.deepseek.com", CredentialAudience: "api.deepseek.com"},
		upstream.ProtocolMessages:        {ID: "deepseek.anthropic", Scheme: "https", HostTemplate: "api.deepseek.com", BasePath: "/anthropic", CredentialAudience: "api.deepseek.com"},
	}
	credentials := map[upstream.ProtocolID]upstream.CredentialProfile{
		upstream.ProtocolChatCompletions: {
			ID: "deepseek.default.openai", Type: upstream.CredentialAPIKey,
			References: []upstream.CredentialReference{{Purpose: upstream.CredentialPurposeAPIKey, Store: "env", Name: "DEEPSEEK_API_KEY"}},
			Audience:   "api.deepseek.com", AuthPlacement: upstream.AuthPlacementHeader, AuthHeader: "Authorization", AuthScheme: "Bearer",
			Lifecycle: upstream.CredentialLifecycleStatic, AllowedProviderIDs: []upstream.ProviderID{providerID},
			AllowedOfferingIDs: []upstream.OfferingID{offering.ID}, AllowedDeploymentIDs: []upstream.DeploymentID{deployment.ID},
			AllowedRegions: []string{"global"}, AllowedEndpointIDs: []upstream.EndpointID{"deepseek.openai"},
		},
		upstream.ProtocolMessages: {
			ID: "deepseek.default.anthropic", Type: upstream.CredentialAPIKey,
			References: []upstream.CredentialReference{{Purpose: upstream.CredentialPurposeAPIKey, Store: "env", Name: "DEEPSEEK_API_KEY"}},
			Audience:   "api.deepseek.com", AuthPlacement: upstream.AuthPlacementHeader, AuthHeader: "x-api-key",
			Lifecycle: upstream.CredentialLifecycleStatic, AllowedProviderIDs: []upstream.ProviderID{providerID},
			AllowedOfferingIDs: []upstream.OfferingID{offering.ID}, AllowedDeploymentIDs: []upstream.DeploymentID{deployment.ID},
			AllowedRegions: []string{"global"}, AllowedEndpointIDs: []upstream.EndpointID{"deepseek.anthropic"},
		},
	}
	sources := []OfficialSource{
		{ID: "deepseek.quickstart.2026-07-11", Publisher: "DeepSeek", Kind: SourceProductDocs, URL: "https://api-docs.deepseek.com/"},
		{ID: "deepseek.chat.reference.2026-07-11", Publisher: "DeepSeek", Kind: SourceAPIReference, URL: "https://api-docs.deepseek.com/api/create-chat-completion/"},
		{ID: "deepseek.anthropic.2026-07-11", Publisher: "DeepSeek", Kind: SourceAPIReference, URL: "https://api-docs.deepseek.com/guides/anthropic_api/"},
	}
	definitions := []struct {
		id       upstream.RouteID
		protocol upstream.ProtocolID
		sdk      SDKMetadata
		version  string
	}{
		{"deepseek.direct.payg.chat_completions", upstream.ProtocolChatCompletions, openAISDK(), "deepseek-2026-07-11"},
		{"deepseek.direct.payg.messages", upstream.ProtocolMessages, deepSeekAnthropicSDK(), "deepseek-2026-07-11"},
	}
	entries := make([]Entry, 0, len(definitions))
	for _, definition := range definitions {
		capabilities := completeCapabilities(map[string]CapabilityMetadata{
			"text_generation":       {Support: CapabilityCompatible},
			"streaming":             {Support: CapabilityCompatible},
			"tool_calling":          {Support: CapabilityCompatible},
			"parallel_tool_calling": {Support: CapabilityCompatible},
			"structured_output":     {Support: CapabilityPartial, Limitations: []string{"Chat Completions supports JSON Object; strict JSON Schema is not declared"}},
			"reasoning":             {Support: CapabilityCompatible},
			"usage_reporting":       {Support: CapabilityCompatible},
		})
		ignored := []string{}
		extensions := []string{"reasoning_content", "thinking", "reasoning_effort"}
		streamEvents := []string{"chat.completion.chunk", "reasoning_content.delta", "done"}
		errorDialect := ErrorDialect{Envelope: "openai.error", CodeField: "error.code", RequestIDHeaders: []string{"x-request-id"}, RetryHeaders: []string{"retry-after"}}
		if definition.protocol == upstream.ProtocolMessages {
			capabilities = completeCapabilities(map[string]CapabilityMetadata{
				"text_generation":       {Support: CapabilityCompatible},
				"streaming":             {Support: CapabilityCompatible},
				"tool_calling":          {Support: CapabilityCompatible},
				"parallel_tool_calling": {Support: CapabilityPartial, Limitations: []string{"disable_parallel_tool_use is ignored"}},
				"reasoning":             {Support: CapabilityCompatible},
				"provider_continuation": {Support: CapabilityCompatible},
				"usage_reporting":       {Support: CapabilityCompatible},
			})
			ignored = []string{"anthropic-version", "anthropic-beta", "thinking.budget_tokens", "tool_result.is_error", "top_k", "service_tier"}
			extensions = []string{"thinking"}
			streamEvents = []string{"message_start", "content_block_start", "content_block_delta", "content_block_stop", "message_delta", "message_stop", "error"}
			errorDialect = ErrorDialect{Envelope: "anthropic.error", CodeField: "error.type", RequestIDHeaders: []string{"request-id", "x-request-id"}, RetryHeaders: []string{"retry-after"}}
		}
		entry := Entry{
			ID:       definition.id,
			Route:    upstream.UpstreamRoute{ID: definition.id, Model: upstream.ModelIdentity{CanonicalFamily: "deepseek", ProviderModelRef: models[0]}, Provider: providerID, Offering: offering, Deployment: deployment, Protocol: upstream.ProtocolBinding{ID: definition.protocol, APIVersion: definition.version}, Endpoint: endpoints[definition.protocol], Credential: credentials[definition.protocol]},
			Maturity: MaturityUnknown, ModelDiscovery: exactProviderModels(models),
			Sources: sources, Evidence: Evidence{Status: EvidenceFresh, TTLClass: EvidenceTTL7Days, CheckedAt: directCheckedAt, ValidUntil: directValidUntil},
			SDKs: []SDKMetadata{definition.sdk}, Capabilities: capabilities, IgnoredFields: ignored, ExtensionFields: extensions,
			StreamEvents: streamEvents, ErrorDialect: errorDialect,
			Boundaries:     OperationalBoundaries{Production: ProductionRequiresReview, Quota: QuotaProviderAccount, Expiry: ExpiryCredentialOrAccount},
			Implementation: Implementation{Status: ImplementationImplementedOffline, Callable: true, AdapterID: "deepseek", CodePaths: []string{"provider/deepseek", "internal/compatprovider", "internal/protocol/" + protocolPackage(definition.protocol)}, TestEvidence: []string{"tests/deepseek"}},
		}
		entries = append(entries, finalizeDefaultEntry(entry))
	}
	return entries
}

func deepSeekAnthropicSDK() SDKMetadata {
	return SDKMetadata{Language: "go", Package: "github.com/anthropics/anthropic-sdk-go", Owner: "Anthropic", Ownership: SDKOwnershipProtocolUpstream, Transport: TransportSDK, Version: "v1.56.0", License: "MIT", Official: true}
}

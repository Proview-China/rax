package catalog

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

var (
	cloudCheckedAt  = time.Date(2026, 7, 10, 17, 0, 0, 0, time.UTC)
	cloudValidUntil = cloudCheckedAt.Add(7 * 24 * time.Hour)
)

type cloudRouteSpec struct {
	id           upstream.RouteID
	provider     upstream.ProviderID
	offering     upstream.Offering
	modelFamily  string
	modelRef     string
	deployment   upstream.Deployment
	protocol     upstream.ProtocolBinding
	endpoint     upstream.Endpoint
	credential   upstream.CredentialProfile
	sources      []OfficialSource
	sdks         []SDKMetadata
	capabilities []CapabilityMetadata
	streamEvents []string
	errorDialect ErrorDialect
	adapterID    string
	codePaths    []string
	testEvidence []string
	status       ImplementationStatus
	callable     bool
	evidence     EvidenceStatus
	boundaries   OperationalBoundaries
}

func cloudEntries() []Entry {
	entries := make([]Entry, 0, 26)
	entries = append(entries, awsMantleEntries()...)
	entries = append(entries, awsRuntimeEntries()...)
	entries = append(entries, vertexEntries()...)
	entries = append(entries, azureEntries()...)
	entries = append(entries, cloudControlEntries()...)
	return entries
}

func awsMantleEntries() []Entry {
	const providerID upstream.ProviderID = "aws.bedrock-mantle"
	offering := cloudPAYGOffering("aws.bedrock-mantle.payg")
	deployment := upstream.Deployment{ID: "aws.bedrock-mantle.us-east-1", Kind: upstream.DeploymentCloudServerless, Region: "us-east-1", ProjectRef: "runtime-project"}
	endpoints := map[upstream.ProtocolID]upstream.Endpoint{
		upstream.ProtocolResponses:       {ID: "aws.bedrock-mantle.openai", Scheme: "https", HostTemplate: "bedrock-mantle.{region}.api.aws", BasePath: "/openai/v1", CredentialAudience: "bedrock-mantle.{region}.api.aws"},
		upstream.ProtocolChatCompletions: {ID: "aws.bedrock-mantle.openai", Scheme: "https", HostTemplate: "bedrock-mantle.{region}.api.aws", BasePath: "/openai/v1", CredentialAudience: "bedrock-mantle.{region}.api.aws"},
		upstream.ProtocolMessages:        {ID: "aws.bedrock-mantle.anthropic", Scheme: "https", HostTemplate: "bedrock-mantle.{region}.api.aws", BasePath: "/anthropic/v1", CredentialAudience: "bedrock-mantle.{region}.api.aws"},
	}
	endpointIDs := []upstream.EndpointID{"aws.bedrock-mantle.openai", "aws.bedrock-mantle.anthropic"}
	mantleSigV4 := cloudSigV4Credential("aws.bedrock-mantle.sigv4", providerID, offering.ID, deployment, "bedrock-mantle.{region}.api.aws", endpointIDs)
	mantleSigV4.SigV4Service = "bedrock-mantle"
	credentials := []upstream.CredentialProfile{
		cloudAPIKeyCredential("aws.bedrock-mantle.api-key", "AWS_BEARER_TOKEN_BEDROCK", providerID, offering.ID, deployment, "bedrock-mantle.{region}.api.aws", endpointIDs, "x-api-key"),
		mantleSigV4,
	}
	sources := []OfficialSource{{ID: "aws.bedrock.mantle.2026-07-11", Publisher: "Amazon Web Services", Kind: SourceProductDocs, URL: "https://docs.aws.amazon.com/bedrock/latest/userguide/model-parameters-openai.html"}, {ID: "anthropic.go.bedrock-mantle.1.56.0", Publisher: "Anthropic", Kind: SourceSDK, URL: "https://github.com/anthropics/anthropic-sdk-go/tree/main/bedrock"}}
	var entries []Entry
	for _, protocolID := range []upstream.ProtocolID{upstream.ProtocolResponses, upstream.ProtocolChatCompletions, upstream.ProtocolMessages} {
		for _, credential := range credentials {
			authID := "api-key"
			if credential.Type == upstream.CredentialSigV4 {
				authID = "sigv4"
			}
			sdk := openAISDK()
			if protocolID == upstream.ProtocolMessages {
				sdk = anthropicSDK()
			}
			spec := cloudRouteSpec{id: upstream.RouteID("aws.bedrock-mantle.us-east-1." + authID + "." + string(protocolID)), provider: providerID, offering: offering, modelFamily: "multi-model", modelRef: "runtime_selected", deployment: deployment, protocol: upstream.ProtocolBinding{ID: protocolID, APIVersion: mantleVersion(protocolID)}, endpoint: endpoints[protocolID], credential: credential, sources: sources, sdks: []SDKMetadata{sdk}, capabilities: cloudProtocolCapabilities(protocolID), streamEvents: cloudStreamEvents(protocolID), errorDialect: cloudErrorDialect(protocolID, "x-amzn-requestid"), adapterID: "aws-bedrock-mantle", codePaths: []string{"provider/bedrockmantle", "internal/protocol/" + protocolPackage(protocolID)}, testEvidence: []string{"tests/bedrockmantle"}, status: ImplementationImplementedOffline, callable: true, evidence: EvidenceFresh, boundaries: OperationalBoundaries{Production: ProductionRequiresReview, Quota: QuotaProviderAccount, Expiry: ExpiryCredentialOrAccount}}
			entries = append(entries, makeCloudEntry(spec))
		}
	}
	return entries
}

func awsRuntimeEntries() []Entry {
	const providerID upstream.ProviderID = "aws.bedrock-runtime"
	offering := cloudPAYGOffering("aws.bedrock-runtime.payg")
	deployment := upstream.Deployment{ID: "aws.bedrock-runtime.us-east-1", Kind: upstream.DeploymentCloudServerless, Region: "us-east-1", ResourceRef: "runtime-model-or-profile"}
	endpoint := upstream.Endpoint{ID: "aws.bedrock-runtime.native", Scheme: "https", HostTemplate: "bedrock-runtime.{region}.amazonaws.com", CredentialAudience: "bedrock-runtime.{region}.amazonaws.com"}
	endpointIDs := []upstream.EndpointID{endpoint.ID}
	credentials := []upstream.CredentialProfile{cloudBearerCredential("aws.bedrock-runtime.bearer", "AWS_BEARER_TOKEN_BEDROCK", providerID, offering.ID, deployment, endpoint.CredentialAudience, endpointIDs), cloudSigV4Credential("aws.bedrock-runtime.sigv4", providerID, offering.ID, deployment, endpoint.CredentialAudience, endpointIDs)}
	sources := []OfficialSource{{ID: "aws.bedrock.runtime.converse.2026-07-11", Publisher: "Amazon Web Services", Kind: SourceAPIReference, URL: "https://docs.aws.amazon.com/bedrock/latest/APIReference/API_runtime_Converse.html"}, {ID: "aws.bedrock.runtime.invoke.2026-07-11", Publisher: "Amazon Web Services", Kind: SourceAPIReference, URL: "https://docs.aws.amazon.com/bedrock/latest/APIReference/API_runtime_InvokeModel.html"}}
	var entries []Entry
	for _, protocolID := range []upstream.ProtocolID{upstream.ProtocolBedrockConverse, upstream.ProtocolBedrockInvoke} {
		for _, credential := range credentials {
			authID := "bearer"
			if credential.Type == upstream.CredentialSigV4 {
				authID = "sigv4"
			}
			capabilities := cloudProtocolCapabilities(protocolID)
			spec := cloudRouteSpec{id: upstream.RouteID("aws.bedrock-runtime.us-east-1." + authID + "." + string(protocolID)), provider: providerID, offering: offering, modelFamily: "multi-model", modelRef: "runtime_selected", deployment: deployment, protocol: upstream.ProtocolBinding{ID: protocolID, APIVersion: "2023-09-30"}, endpoint: endpoint, credential: credential, sources: sources, sdks: []SDKMetadata{awsBedrockSDK()}, capabilities: capabilities, streamEvents: cloudStreamEvents(protocolID), errorDialect: cloudErrorDialect(protocolID, "x-amzn-requestid"), adapterID: "aws-bedrock-runtime", codePaths: []string{"provider/bedrockruntime", "internal/protocol/bedrock"}, testEvidence: []string{"tests/bedrockruntime", "tests/protocol/bedrock"}, status: ImplementationImplementedOffline, callable: true, evidence: EvidenceFresh, boundaries: OperationalBoundaries{Production: ProductionRequiresReview, Quota: QuotaProviderAccount, Expiry: ExpiryCredentialOrAccount}}
			entries = append(entries, makeCloudEntry(spec))
		}
	}
	return entries
}

func vertexEntries() []Entry {
	const providerID upstream.ProviderID = "google.vertex-ai"
	offering := cloudPAYGOffering("google.vertex-ai.payg")
	deployment := upstream.Deployment{ID: "google.vertex-ai.serverless.us-central1", Kind: upstream.DeploymentCloudServerless, Region: "us-central1", ProjectRef: "runtime-project", ResourceRef: "publisher-model"}
	endpoints := map[upstream.ProtocolID]upstream.Endpoint{
		upstream.ProtocolGenerateContent: {ID: "google.vertex-ai.generate", Scheme: "https", HostTemplate: "{region}-aiplatform.googleapis.com", BasePath: "/v1", CredentialAudience: "{region}-aiplatform.googleapis.com"},
		upstream.ProtocolMessages:        {ID: "google.vertex-ai.anthropic", Scheme: "https", HostTemplate: "{region}-aiplatform.googleapis.com", BasePath: "/v1/projects", CredentialAudience: "{region}-aiplatform.googleapis.com"},
		upstream.ProtocolChatCompletions: {ID: "google.vertex-ai.openai-chat", Scheme: "https", HostTemplate: "{region}-aiplatform.googleapis.com", BasePath: "/v1beta1/projects", CredentialAudience: "{region}-aiplatform.googleapis.com"},
	}
	allEndpoints := []upstream.EndpointID{"google.vertex-ai.generate", "google.vertex-ai.anthropic", "google.vertex-ai.openai-chat"}
	apiEndpoints := []upstream.EndpointID{"google.vertex-ai.generate", "google.vertex-ai.openai-chat"}
	adc := cloudADCCredential("google.vertex-ai.adc", providerID, offering.ID, deployment, "{region}-aiplatform.googleapis.com", allEndpoints)
	apiKey := cloudAPIKeyCredential("google.vertex-ai.api-key", "GOOGLE_API_KEY", providerID, offering.ID, deployment, "{region}-aiplatform.googleapis.com", apiEndpoints, "x-goog-api-key")
	sources := []OfficialSource{{ID: "google.vertex-ai.claude.2026-07-11", Publisher: "Google Cloud", Kind: SourceProductDocs, URL: "https://cloud.google.com/vertex-ai/generative-ai/docs/partner-models/use-claude"}, {ID: "google.genai.go.1.63.0", Publisher: "Google", Kind: SourceSDK, URL: "https://pkg.go.dev/google.golang.org/genai"}}
	definitions := []struct {
		protocol    upstream.ProtocolID
		credentials []upstream.CredentialProfile
		sdk         SDKMetadata
		version     string
	}{
		{upstream.ProtocolGenerateContent, []upstream.CredentialProfile{adc, apiKey}, googleGenAISDK(), "v1"},
		{upstream.ProtocolMessages, []upstream.CredentialProfile{adc}, anthropicSDK(), "vertex-2023-10-16"},
		{upstream.ProtocolChatCompletions, []upstream.CredentialProfile{adc, apiKey}, openAISDK(), "v1beta1"},
	}
	var entries []Entry
	for _, definition := range definitions {
		for _, credential := range definition.credentials {
			authID := "adc"
			if credential.Type == upstream.CredentialAPIKey {
				authID = "api-key"
			}
			spec := cloudRouteSpec{id: upstream.RouteID("google.vertex-ai.us-central1." + authID + "." + string(definition.protocol)), provider: providerID, offering: offering, modelFamily: vertexFamily(definition.protocol), modelRef: "runtime_selected", deployment: deployment, protocol: upstream.ProtocolBinding{ID: definition.protocol, APIVersion: definition.version}, endpoint: endpoints[definition.protocol], credential: credential, sources: sources, sdks: []SDKMetadata{definition.sdk}, capabilities: cloudProtocolCapabilities(definition.protocol), streamEvents: cloudStreamEvents(definition.protocol), errorDialect: cloudErrorDialect(definition.protocol, "x-goog-request-id"), adapterID: "google-vertex-ai", codePaths: []string{"provider/vertex", "internal/protocol/" + protocolPackage(definition.protocol)}, testEvidence: []string{"tests/vertex"}, status: ImplementationImplementedOffline, callable: true, evidence: EvidenceFresh, boundaries: OperationalBoundaries{Production: ProductionRequiresReview, Quota: QuotaProviderAccount, Expiry: ExpiryCredentialOrAccount}}
			entries = append(entries, makeCloudEntry(spec))
		}
	}
	return entries
}

func azureEntries() []Entry {
	const providerID upstream.ProviderID = "azure.openai"
	offering := cloudPAYGOffering("azure.openai.payg")
	deployment := upstream.Deployment{ID: "azure.openai.deployment.eastus", Kind: upstream.DeploymentCloudServerless, Region: "eastus", ResourceRef: "runtime-resource", DeploymentName: "runtime-deployment"}
	endpoints := map[string]upstream.Endpoint{"v1": {ID: "azure.openai.v1", Scheme: "https", HostTemplate: "{resource}.openai.azure.com", BasePath: "/openai/v1", CredentialAudience: "{resource}.openai.azure.com"}, "legacy": {ID: "azure.openai.legacy", Scheme: "https", HostTemplate: "{resource}.openai.azure.com", BasePath: "/openai/deployments", CredentialAudience: "{resource}.openai.azure.com"}}
	endpointIDs := []upstream.EndpointID{"azure.openai.v1", "azure.openai.legacy"}
	apiKey := cloudAPIKeyCredential("azure.openai.api-key", "AZURE_OPENAI_API_KEY", providerID, offering.ID, deployment, "{resource}.openai.azure.com", endpointIDs, "api-key")
	entra := cloudEntraCredential("azure.openai.entra", providerID, offering.ID, deployment, "{resource}.openai.azure.com", endpointIDs)
	sources := []OfficialSource{{ID: "azure.openai.v1.lifecycle.2026-07-11", Publisher: "Microsoft", Kind: SourceAPIReference, URL: "https://learn.microsoft.com/en-us/azure/ai-foundry/openai/api-version-lifecycle"}, {ID: "azure.openai.switching-endpoints.2026-07-11", Publisher: "Microsoft", Kind: SourceProductDocs, URL: "https://learn.microsoft.com/en-us/azure/ai-foundry/openai/how-to/switching-endpoints"}}
	definitions := []struct {
		name     string
		protocol upstream.ProtocolID
		version  string
		endpoint upstream.Endpoint
	}{{"v1", upstream.ProtocolResponses, "v1", endpoints["v1"]}, {"v1", upstream.ProtocolChatCompletions, "v1", endpoints["v1"]}, {"legacy", upstream.ProtocolChatCompletions, "2024-10-21", endpoints["legacy"]}}
	var entries []Entry
	for _, definition := range definitions {
		for _, credential := range []upstream.CredentialProfile{apiKey, entra} {
			authID := "api-key"
			if credential.Type == upstream.CredentialEntraID {
				authID = "entra"
			}
			spec := cloudRouteSpec{id: upstream.RouteID("azure.openai.eastus." + definition.name + "." + authID + "." + string(definition.protocol)), provider: providerID, offering: offering, modelFamily: "openai", modelRef: "deployment_selected", deployment: deployment, protocol: upstream.ProtocolBinding{ID: definition.protocol, APIVersion: definition.version}, endpoint: definition.endpoint, credential: credential, sources: sources, sdks: []SDKMetadata{openAISDK()}, capabilities: cloudProtocolCapabilities(definition.protocol), streamEvents: cloudStreamEvents(definition.protocol), errorDialect: cloudErrorDialect(definition.protocol, "apim-request-id"), adapterID: "azure-openai", codePaths: []string{"provider/azureopenai", "internal/protocol/" + protocolPackage(definition.protocol)}, testEvidence: []string{"tests/azureopenai"}, status: ImplementationImplementedOffline, callable: true, evidence: EvidenceFresh, boundaries: OperationalBoundaries{Production: ProductionRequiresReview, Quota: QuotaProviderAccount, Expiry: ExpiryCredentialOrAccount}}
			entries = append(entries, makeCloudEntry(spec))
		}
	}
	return entries
}

func cloudControlEntries() []Entry {
	controls := []cloudRouteSpec{
		controlSpec("google.vertex-ai.provisioned.us-central1.generate_content", "google.vertex-ai", "google.vertex-ai.provisioned", "gemini", "publisher-model", upstream.Deployment{ID: "google.vertex-ai.provisioned.us-central1", Kind: upstream.DeploymentCloudProvisioned, Region: "us-central1", ProjectRef: "runtime-project", ResourceRef: "provisioned-throughput"}, upstream.ProtocolGenerateContent, upstream.Endpoint{ID: "google.vertex-ai.provisioned.generate", Scheme: "https", HostTemplate: "{region}-aiplatform.googleapis.com", BasePath: "/v1", CredentialAudience: "{region}-aiplatform.googleapis.com"}, EvidenceFresh, ImplementationPlanned, "provisioned throughput requires a capacity-specific route"),
		controlSpec("google.vertex-ai.model-garden.self-deployed.generate_content", "google.vertex-ai", "google.vertex-ai.model-garden", "multi-model", "self-deployed-endpoint", upstream.Deployment{ID: "google.vertex-ai.model-garden.self-deployed", Kind: upstream.DeploymentCloudProvisioned, Region: "us-central1", ProjectRef: "runtime-project", ResourceRef: "model-garden-endpoint"}, upstream.ProtocolGenerateContent, upstream.Endpoint{ID: "google.vertex-ai.model-garden.generate", Scheme: "https", HostTemplate: "{region}-aiplatform.googleapis.com", BasePath: "/v1", CredentialAudience: "{region}-aiplatform.googleapis.com"}, EvidenceFresh, ImplementationPlanned, "self-deployed Model Garden endpoints do not inherit serverless capability"),
		controlSpec("azure.ai-foundry.other-models.unverified.chat_completions", "azure.ai-foundry", "azure.ai-foundry.other-models", "multi-model", "runtime_selected", upstream.Deployment{ID: "azure.ai-foundry.other-models", Kind: upstream.DeploymentCloudServerless, Region: "unknown", ResourceRef: "runtime-resource", DeploymentName: "runtime-deployment"}, upstream.ProtocolChatCompletions, upstream.Endpoint{ID: "azure.ai-foundry.other-models", Scheme: "https", HostTemplate: "{resource}.services.ai.azure.com", BasePath: "/models", CredentialAudience: "{resource}.services.ai.azure.com"}, EvidenceUnverified, ImplementationResearchOnly, "Foundry models require per-model protocol and deployment evidence"),
		controlSpec("anthropic.platform-on-aws.unverified.messages", "anthropic.platform-on-aws", "anthropic.platform-on-aws.marketplace", "claude", "runtime_selected", upstream.Deployment{ID: "anthropic.platform-on-aws", Kind: upstream.DeploymentCloudServerless, Region: "unknown", ResourceRef: "anthropic-operated"}, upstream.ProtocolMessages, upstream.Endpoint{ID: "anthropic.platform-on-aws", Scheme: "https", HostTemplate: "platform.claude.com", CredentialAudience: "platform.claude.com"}, EvidenceUnverified, ImplementationResearchOnly, "Anthropic-operated AWS Marketplace product is not Amazon Bedrock"),
		controlSpec("anthropic.consumer-plans.product-login.messages", "anthropic.consumer", "anthropic.consumer-plans", "claude", "claude-product", upstream.Deployment{ID: "anthropic.consumer-plans", Kind: upstream.DeploymentDirect, Region: "global", ResourceRef: "claude-product"}, upstream.ProtocolMessages, upstream.Endpoint{ID: "anthropic.consumer-product", Scheme: "https", HostTemplate: "claude.ai", CredentialAudience: "claude.ai"}, EvidenceFresh, ImplementationResearchOnly, "Claude Pro and Max do not fund the Messages API; Agent SDK subscription use is a separate unimplemented host-wiring contract"),
	}
	entries := make([]Entry, 0, len(controls))
	for _, spec := range controls {
		entry := makeCloudEntry(spec)
		if entry.ID == "anthropic.consumer-plans.product-login.messages" {
			entry.Sources = append(entry.Sources,
				OfficialSource{ID: "anthropic.consumer.pro-no-api.2026-07-11", Publisher: "Anthropic", Kind: SourceTerms, URL: "https://support.claude.com/en/articles/8325606-what-is-the-pro-plan"},
				OfficialSource{ID: "anthropic.consumer.agent-sdk-plan.2026-07-11", Publisher: "Anthropic", Kind: SourceTerms, URL: "https://support.claude.com/en/articles/15036540-use-the-claude-agent-sdk-with-your-claude-plan"},
			)
			entry = finalizeDefaultEntry(entry)
		}
		entries = append(entries, entry)
	}
	return entries
}

func controlSpec(id string, provider upstream.ProviderID, offeringID upstream.OfferingID, family, model string, deployment upstream.Deployment, protocol upstream.ProtocolID, endpoint upstream.Endpoint, evidence EvidenceStatus, status ImplementationStatus, limitation string) cloudRouteSpec {
	offering := cloudPAYGOffering(offeringID)
	offering.Entitlement.AllowedUsage = upstream.AllowedUsageOfficialClientOnly
	offering.Entitlement.ProductionPolicy = upstream.ProductionPolicyForbidden
	credentialID := upstream.CredentialProfileID(string(deployment.ID) + ".research-only")
	credential := upstream.CredentialProfile{ID: credentialID, Type: upstream.CredentialAnonymous, Audience: endpoint.CredentialAudience, AuthPlacement: upstream.AuthPlacementNone, Lifecycle: upstream.CredentialLifecycleAnonymous, AllowedProviderIDs: []upstream.ProviderID{provider}, AllowedOfferingIDs: []upstream.OfferingID{offeringID}, AllowedDeploymentIDs: []upstream.DeploymentID{deployment.ID}, AllowedRegions: []string{deployment.Region}, AllowedEndpointIDs: []upstream.EndpointID{endpoint.ID}}
	return cloudRouteSpec{id: upstream.RouteID(id), provider: provider, offering: offering, modelFamily: family, modelRef: model, deployment: deployment, protocol: upstream.ProtocolBinding{ID: protocol}, endpoint: endpoint, credential: credential, sources: []OfficialSource{{ID: id + ".official-control", Publisher: controlPublisher(provider), Kind: SourceProductDocs, URL: controlURL(provider)}}, sdks: []SDKMetadata{{Language: "control", Package: string(provider), Owner: controlPublisher(provider), Ownership: SDKOwnershipCloudNative, Transport: TransportHTTP, Version: "unverified", License: "proprietary", Official: true}}, capabilities: completeCapabilities(map[string]CapabilityMetadata{"text_generation": {Support: CapabilityUnknown, Limitations: []string{limitation}}, "streaming": {Support: CapabilityUnknown, Limitations: []string{limitation}}}), streamEvents: []string{"unknown"}, errorDialect: ErrorDialect{Envelope: "unverified.error", CodeField: "error.code", RequestIDHeaders: []string{"x-request-id"}}, status: status, callable: false, evidence: evidence, boundaries: OperationalBoundaries{Production: ProductionProhibited, Quota: QuotaUnknown, Expiry: ExpiryUnknown}}
}

func makeCloudEntry(spec cloudRouteSpec) Entry {
	entry := Entry{ID: spec.id, Route: upstream.UpstreamRoute{ID: spec.id, Model: upstream.ModelIdentity{CanonicalFamily: spec.modelFamily, ProviderModelRef: spec.modelRef}, Provider: spec.provider, Offering: spec.offering, Deployment: spec.deployment, Protocol: spec.protocol, Endpoint: spec.endpoint, Credential: spec.credential}, Maturity: MaturityUnknown, ModelDiscovery: ModelDiscovery{Method: ModelDiscoveryRuntimeSelected, AliasPolicy: ModelAliasExactProviderID}, Sources: append([]OfficialSource(nil), spec.sources...), Evidence: Evidence{Status: spec.evidence, TTLClass: EvidenceTTL7Days, CheckedAt: cloudCheckedAt, ValidUntil: cloudValidUntil}, SDKs: append([]SDKMetadata(nil), spec.sdks...), Capabilities: spec.capabilities, IgnoredFields: []string{}, ExtensionFields: []string{}, StreamEvents: append([]string(nil), spec.streamEvents...), ErrorDialect: spec.errorDialect, Boundaries: spec.boundaries, Implementation: Implementation{Status: spec.status, Callable: spec.callable, AdapterID: spec.adapterID, CodePaths: append([]string(nil), spec.codePaths...), TestEvidence: append([]string(nil), spec.testEvidence...)}}
	return finalizeDefaultEntry(entry)
}

func cloudPAYGOffering(id upstream.OfferingID) upstream.Offering {
	return upstream.Offering{ID: id, Kind: upstream.OfferingPayAsYouGo, Entitlement: upstream.CommercialEntitlement{AllowedUsage: upstream.AllowedUsageGeneralAPI, SubjectPolicy: upstream.SubjectPolicyAny, TenancyPolicy: upstream.TenancyPolicyAny, ExecutionPolicy: upstream.ExecutionPolicyAny, ProductionPolicy: upstream.ProductionPolicyAllowed}}
}

func cloudAPIKeyCredential(id upstream.CredentialProfileID, env string, provider upstream.ProviderID, offering upstream.OfferingID, deployment upstream.Deployment, audience string, endpoints []upstream.EndpointID, header string) upstream.CredentialProfile {
	return upstream.CredentialProfile{ID: id, Type: upstream.CredentialAPIKey, References: []upstream.CredentialReference{{Purpose: upstream.CredentialPurposeAPIKey, Store: "env", Name: env}}, Audience: audience, AuthPlacement: upstream.AuthPlacementHeader, AuthHeader: header, Lifecycle: upstream.CredentialLifecycleRenewable, AllowedProviderIDs: []upstream.ProviderID{provider}, AllowedOfferingIDs: []upstream.OfferingID{offering}, AllowedDeploymentIDs: []upstream.DeploymentID{deployment.ID}, AllowedRegions: []string{deployment.Region}, AllowedEndpointIDs: append([]upstream.EndpointID(nil), endpoints...)}
}
func cloudBearerCredential(id upstream.CredentialProfileID, env string, provider upstream.ProviderID, offering upstream.OfferingID, deployment upstream.Deployment, audience string, endpoints []upstream.EndpointID) upstream.CredentialProfile {
	c := cloudAPIKeyCredential(id, env, provider, offering, deployment, audience, endpoints, "Authorization")
	c.Type = upstream.CredentialBearer
	c.References[0].Purpose = upstream.CredentialPurposeBearerToken
	c.AuthScheme = "Bearer"
	return c
}
func cloudSigV4Credential(id upstream.CredentialProfileID, provider upstream.ProviderID, offering upstream.OfferingID, deployment upstream.Deployment, audience string, endpoints []upstream.EndpointID) upstream.CredentialProfile {
	return upstream.CredentialProfile{ID: id, Type: upstream.CredentialSigV4, References: []upstream.CredentialReference{{Purpose: upstream.CredentialPurposeWorkloadIdentity, Store: "workload_identity", Name: "AWS_DEFAULT_CREDENTIAL_CHAIN"}}, Audience: audience, AuthPlacement: upstream.AuthPlacementRequestSigning, Lifecycle: upstream.CredentialLifecycleWorkloadIdentity, SigV4Service: "bedrock", AllowedProviderIDs: []upstream.ProviderID{provider}, AllowedOfferingIDs: []upstream.OfferingID{offering}, AllowedDeploymentIDs: []upstream.DeploymentID{deployment.ID}, AllowedRegions: []string{deployment.Region}, AllowedEndpointIDs: append([]upstream.EndpointID(nil), endpoints...)}
}
func cloudADCCredential(id upstream.CredentialProfileID, provider upstream.ProviderID, offering upstream.OfferingID, deployment upstream.Deployment, audience string, endpoints []upstream.EndpointID) upstream.CredentialProfile {
	return upstream.CredentialProfile{ID: id, Type: upstream.CredentialADC, References: []upstream.CredentialReference{{Purpose: upstream.CredentialPurposeWorkloadIdentity, Store: "workload_identity", Name: "GOOGLE_APPLICATION_DEFAULT_CREDENTIALS"}}, Audience: audience, AuthPlacement: upstream.AuthPlacementSDK, Lifecycle: upstream.CredentialLifecycleWorkloadIdentity, AllowedProviderIDs: []upstream.ProviderID{provider}, AllowedOfferingIDs: []upstream.OfferingID{offering}, AllowedDeploymentIDs: []upstream.DeploymentID{deployment.ID}, AllowedRegions: []string{deployment.Region}, AllowedEndpointIDs: append([]upstream.EndpointID(nil), endpoints...)}
}
func cloudEntraCredential(id upstream.CredentialProfileID, provider upstream.ProviderID, offering upstream.OfferingID, deployment upstream.Deployment, audience string, endpoints []upstream.EndpointID) upstream.CredentialProfile {
	return upstream.CredentialProfile{ID: id, Type: upstream.CredentialEntraID, References: []upstream.CredentialReference{{Purpose: upstream.CredentialPurposeWorkloadIdentity, Store: "workload_identity", Name: "AZURE_DEFAULT_CREDENTIAL"}}, Audience: audience, AuthPlacement: upstream.AuthPlacementSDK, Scopes: []string{"https://cognitiveservices.azure.com/.default"}, Lifecycle: upstream.CredentialLifecycleShortLived, AllowedProviderIDs: []upstream.ProviderID{provider}, AllowedOfferingIDs: []upstream.OfferingID{offering}, AllowedDeploymentIDs: []upstream.DeploymentID{deployment.ID}, AllowedRegions: []string{deployment.Region}, AllowedEndpointIDs: append([]upstream.EndpointID(nil), endpoints...)}
}

func cloudProtocolCapabilities(protocol upstream.ProtocolID) []CapabilityMetadata {
	overrides := map[string]CapabilityMetadata{"text_generation": {Support: CapabilityCompatible}, "streaming": {Support: CapabilityCompatible}, "tool_calling": {Support: CapabilityCompatible}, "usage_reporting": {Support: CapabilityCompatible}}
	switch protocol {
	case upstream.ProtocolResponses:
		overrides["server_state"] = CapabilityMetadata{Support: CapabilityPartial, Limitations: []string{"state retention is cloud route and deployment specific"}}
	case upstream.ProtocolBedrockConverse:
		overrides["function_error_result"] = CapabilityMetadata{Support: CapabilityCompatible}
		overrides["parallel_tool_calling"] = CapabilityMetadata{Support: CapabilityPartial, Limitations: []string{"support is selected by the concrete Bedrock model"}}
	case upstream.ProtocolBedrockInvoke:
		overrides["text_generation"] = CapabilityMetadata{Support: CapabilityPartial, Limitations: []string{"provider-native JSON remains in RawPayload"}}
		overrides["streaming"] = CapabilityMetadata{Support: CapabilityPartial, Limitations: []string{"provider-native chunks remain native events"}}
		overrides["tool_calling"] = CapabilityMetadata{Support: CapabilityUnsupported, Limitations: []string{"InvokeModel raw mode does not infer portable tool semantics"}}
		overrides["usage_reporting"] = CapabilityMetadata{Support: CapabilityUnsupported, Limitations: []string{"response shape is model-specific and is not normalized in raw mode"}}
	}
	return completeCapabilities(overrides)
}
func cloudStreamEvents(protocol upstream.ProtocolID) []string {
	switch protocol {
	case upstream.ProtocolResponses:
		return []string{"response.created", "response.output_text.delta", "response.completed", "error"}
	case upstream.ProtocolChatCompletions:
		return []string{"chat.completion.chunk", "done", "error"}
	case upstream.ProtocolMessages:
		return []string{"message_start", "content_block_delta", "message_stop", "error"}
	case upstream.ProtocolGenerateContent:
		return []string{"generate_content_response", "stream_terminal", "error"}
	case upstream.ProtocolBedrockConverse:
		return []string{"messageStart", "contentBlockDelta", "metadata", "messageStop"}
	case upstream.ProtocolBedrockInvoke:
		return []string{"chunk", "stream_terminal"}
	default:
		return []string{"unknown"}
	}
}
func cloudErrorDialect(protocol upstream.ProtocolID, requestHeader string) ErrorDialect {
	envelope := "cloud.error"
	code := "error.code"
	if protocol == upstream.ProtocolMessages {
		envelope = "anthropic-compatible.error"
		code = "error.type"
	} else if protocol == upstream.ProtocolGenerateContent {
		envelope = "google.rpc.status"
		code = "error.status"
	} else if protocol == upstream.ProtocolBedrockConverse || protocol == upstream.ProtocolBedrockInvoke {
		envelope = "aws.smithy.error"
		code = "__type"
	}
	return ErrorDialect{Envelope: envelope, CodeField: code, RequestIDHeaders: []string{requestHeader, "x-request-id"}, RetryHeaders: []string{"retry-after"}}
}
func protocolPackage(protocol upstream.ProtocolID) string {
	switch protocol {
	case upstream.ProtocolResponses:
		return "openairesponses"
	case upstream.ProtocolChatCompletions:
		return "openaichat"
	case upstream.ProtocolMessages:
		return "anthropicmessages"
	case upstream.ProtocolGenerateContent:
		return "geminigenerate"
	default:
		return "bedrock"
	}
}
func mantleVersion(protocol upstream.ProtocolID) string {
	if protocol == upstream.ProtocolMessages {
		return "2023-06-01"
	}
	return "v1"
}
func vertexFamily(protocol upstream.ProtocolID) string {
	if protocol == upstream.ProtocolMessages {
		return "claude"
	}
	if protocol == upstream.ProtocolGenerateContent {
		return "gemini"
	}
	return "multi-model"
}
func openAISDK() SDKMetadata {
	return SDKMetadata{Language: "go", Package: "github.com/openai/openai-go/v3", Owner: "OpenAI", Ownership: SDKOwnershipProtocolUpstream, Transport: TransportSDK, Version: "v3.41.1", License: "Apache-2.0", Official: true}
}
func anthropicSDK() SDKMetadata {
	return SDKMetadata{Language: "go", Package: "github.com/anthropics/anthropic-sdk-go", Owner: "Anthropic", Ownership: SDKOwnershipModelVendor, Transport: TransportSDK, Version: "v1.56.0", License: "MIT", Official: true}
}
func googleGenAISDK() SDKMetadata {
	return SDKMetadata{Language: "go", Package: "google.golang.org/genai", Owner: "Google", Ownership: SDKOwnershipCloudNative, Transport: TransportSDK, Version: "v1.63.0", License: "Apache-2.0", Official: true}
}
func awsBedrockSDK() SDKMetadata {
	return SDKMetadata{Language: "go", Package: "github.com/aws/aws-sdk-go-v2/service/bedrockruntime", Owner: "Amazon Web Services", Ownership: SDKOwnershipCloudNative, Transport: TransportSDK, Version: "v1.55.0", License: "Apache-2.0", Official: true}
}
func controlPublisher(provider upstream.ProviderID) string {
	switch provider {
	case "google.vertex-ai":
		return "Google Cloud"
	case "azure.ai-foundry":
		return "Microsoft"
	case "anthropic.platform-on-aws", "anthropic.consumer":
		return "Anthropic"
	default:
		return "Cloud provider"
	}
}
func controlURL(provider upstream.ProviderID) string {
	switch provider {
	case "google.vertex-ai":
		return "https://cloud.google.com/vertex-ai/generative-ai/docs/learn/overview"
	case "azure.ai-foundry":
		return "https://learn.microsoft.com/en-us/azure/ai-foundry/model-inference/overview"
	case "anthropic.platform-on-aws":
		return "https://www.anthropic.com/partners/amazon-web-services"
	case "anthropic.consumer":
		return "https://support.claude.com/en/articles/9797557-usage-limit-best-practices"
	default:
		return "https://example.com"
	}
}

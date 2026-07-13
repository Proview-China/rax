package routegateway

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/anthropic"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/azureopenai"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/bedrockmantle"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/bedrockruntime"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/deepseek"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/gemini"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/kimi"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/mimo"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/minimax"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/openai"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/plancompat"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/qwen"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/relaycompat"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/vertex"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/xai"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/zai"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

const builtinFactoryVersion = "v1candidate"

type builtinBuild func(context.Context, FactoryInput) (modelinvoker.Provider, error)

type builtinFactory struct {
	adapterID modelinvoker.ProviderID
	build     builtinBuild
}

func (f builtinFactory) ID() string                         { return "builtin/" + string(f.adapterID) }
func (builtinFactory) Version() string                      { return builtinFactoryVersion }
func (f builtinFactory) AdapterID() modelinvoker.ProviderID { return f.adapterID }
func (f builtinFactory) Build(ctx context.Context, input FactoryInput) (FactoryResult, error) {
	if ctx == nil || ctx.Err() != nil {
		return FactoryResult{}, gatewayError(contextKind(contextError(ctx)), "factory_context_done", "adapter construction context is unavailable", contextError(ctx))
	}
	if input.Entry.Implementation.AdapterID != string(f.adapterID) || input.Binding.RouteID != input.Entry.ID || input.Endpoint == "" {
		return FactoryResult{}, gatewayError(modelinvoker.ErrorMapping, "factory_input_mismatch", "factory input does not match its catalog AdapterID, Route, or endpoint", nil)
	}
	provider, err := f.build(ctx, input)
	candidate, err := adaptercore.FinalizeCandidateBinding(
		ctx, f.adapterID, runtimeProtocolForFactory(input.Entry.Route.Protocol.ID), input.Endpoint, provider, err,
	)
	if err != nil {
		return FactoryResult{}, err
	}
	return FactoryResult{Provider: candidate.Provider, Closer: candidate.Closer, Endpoint: candidate.Endpoint}, nil
}

func NewBuiltinFactoryRegistry() (*FactoryRegistry, error) {
	definitions := []builtinFactory{
		{openai.ProviderID, buildOpenAI},
		{anthropic.ProviderID, buildAnthropic},
		{gemini.ProviderID, buildGemini},
		{bedrockmantle.ProviderID, buildBedrockMantle},
		{bedrockruntime.ProviderID, buildBedrockRuntime},
		{vertex.ProviderID, buildVertex},
		{azureopenai.ProviderID, buildAzureOpenAI},
		{deepseek.ProviderID, buildDeepSeek},
		{kimi.ProviderID, buildKimi},
		{zai.ProviderID, buildZAI},
		{minimax.ProviderID, buildMiniMax},
		{mimo.ProviderID, buildMiMo},
		{qwen.ProviderID, buildQwen},
		{xai.ProviderID, buildXAI},
		{plancompat.KimiCodeProvider, buildPlan},
		{plancompat.MiniMaxTokenProvider, buildPlan},
		{plancompat.MiMoTokenProvider, buildPlan},
		{plancompat.AlibabaPlanProvider, buildPlan},
	}
	factories := make([]AdapterFactory, 0, len(definitions))
	for index := range definitions {
		factories = append(factories, definitions[index])
	}
	return NewFactoryRegistry(factories...)
}

// NewRelayCompatFactory returns the explicit opt-in factory for user-supplied
// third-party relay catalog Routes. It is intentionally excluded from the
// default registry and catalog so no relay is mistaken for an official direct
// upstream or silently enabled by installing Praxis.
func NewRelayCompatFactory() AdapterFactory {
	return builtinFactory{adapterID: relaycompat.ProviderID, build: buildRelayCompat}
}

func buildRelayCompat(_ context.Context, input FactoryInput) (modelinvoker.Provider, error) {
	key, err := requiredSecret(input.Secret, upstream.CredentialPurposeAPIKey)
	if err != nil {
		return nil, err
	}
	protocolID := runtimeProtocolForFactory(input.Entry.Route.Protocol.ID)
	config := relaycompat.Config{
		APIKey: key, BaseURL: input.Endpoint, Protocol: protocolID,
		AllowedModels: []string{input.Entry.Route.Model.ProviderModelRef},
		UserAgent:     input.ClientIdentity.UserAgent, HTTPClient: input.HTTPClient,
	}
	if protocolID == modelinvoker.ProtocolGenerateContent {
		version := input.Entry.Route.Protocol.APIVersion
		if version != "v1" && version != "v1beta" {
			return nil, gatewayError(modelinvoker.ErrorMapping, "relay_generate_version_invalid", "relay GenerateContent Route requires API version v1 or v1beta", nil)
		}
		suffix := "/" + version
		if !strings.HasSuffix(input.Endpoint, suffix) {
			return nil, gatewayError(modelinvoker.ErrorMapping, "relay_generate_endpoint_invalid", "relay GenerateContent endpoint must end with its API version", nil)
		}
		config.BaseURL = strings.TrimSuffix(input.Endpoint, suffix)
		config.APIVersion = version
	}
	return relaycompat.New(config)
}

func buildPlan(_ context.Context, input FactoryInput) (modelinvoker.Provider, error) {
	key, err := requiredSecret(input.Secret, upstream.CredentialPurposeAPIKey)
	if err != nil {
		return nil, err
	}
	var kind plancompat.Kind
	switch modelinvoker.ProviderID(input.Entry.Implementation.AdapterID) {
	case plancompat.KimiCodeProvider:
		kind = plancompat.KimiCode
	case plancompat.MiniMaxTokenProvider:
		kind = plancompat.MiniMaxTokenPlan
	case plancompat.MiMoTokenProvider:
		kind = plancompat.MiMoTokenPlan
	case plancompat.AlibabaPlanProvider:
		kind = plancompat.AlibabaPlan
	default:
		return nil, gatewayError(modelinvoker.ErrorUnknownProvider, "plan_factory_unknown", "unknown subscription plan AdapterID", nil)
	}
	return plancompat.New(plancompat.Config{
		Kind: kind, Profile: planProfile(input.Entry), APIKey: key, BaseURL: input.Endpoint,
		Protocol: runtimeProtocolForFactory(input.Entry.Route.Protocol.ID), UserAgent: input.ClientIdentity.UserAgent,
		HTTPClient: input.HTTPClient,
	})
}

func planProfile(entry catalog.Entry) plancompat.RouteProfile {
	switch entry.Route.Deployment.ID {
	case "kimi.code-membership.global":
		return plancompat.ProfileKimiCodeGlobal
	case "minimax.token-plan.global":
		return plancompat.ProfileMiniMaxTokenGlobal
	case "mimo.token-plan.cn":
		return plancompat.ProfileMiMoTokenCN
	case "mimo.token-plan.sgp":
		return plancompat.ProfileMiMoTokenSGP
	case "mimo.token-plan.ams":
		return plancompat.ProfileMiMoTokenAMS
	case "alibaba.coding-plan.cn":
		return plancompat.ProfileAlibabaCodingCN
	case "alibaba.coding-plan.intl":
		return plancompat.ProfileAlibabaCodingIntl
	case "alibaba.token-plan-team.cn-beijing":
		return plancompat.ProfileAlibabaTokenTeamBeijing
	default:
		return ""
	}
}

func runtimeProtocolForFactory(protocol upstream.ProtocolID) modelinvoker.Protocol {
	switch protocol {
	case upstream.ProtocolResponses:
		return modelinvoker.ProtocolResponses
	case upstream.ProtocolChatCompletions:
		return modelinvoker.ProtocolChatCompletions
	case upstream.ProtocolMessages:
		return modelinvoker.ProtocolMessages
	case upstream.ProtocolGenerateContent:
		return modelinvoker.ProtocolGenerateContent
	case upstream.ProtocolBedrockConverse:
		return modelinvoker.ProtocolBedrockConverse
	case upstream.ProtocolBedrockInvoke:
		return modelinvoker.ProtocolBedrockInvoke
	default:
		return modelinvoker.ProtocolAuto
	}
}

func buildOpenAI(_ context.Context, input FactoryInput) (modelinvoker.Provider, error) {
	key, err := requiredSecret(input.Secret, upstream.CredentialPurposeAPIKey)
	if err != nil {
		return nil, err
	}
	return openai.New(openai.Config{APIKey: key, BaseURL: input.Endpoint, HTTPClient: input.HTTPClient})
}

func buildAnthropic(_ context.Context, input FactoryInput) (modelinvoker.Provider, error) {
	key, err := requiredSecret(input.Secret, upstream.CredentialPurposeAPIKey)
	if err != nil {
		return nil, err
	}
	return anthropic.New(anthropic.Config{APIKey: key, BaseURL: endpointOrigin(input.Endpoint), HTTPClient: input.HTTPClient})
}

func buildGemini(_ context.Context, input FactoryInput) (modelinvoker.Provider, error) {
	key, err := requiredSecret(input.Secret, upstream.CredentialPurposeAPIKey)
	if err != nil {
		return nil, err
	}
	version := input.Entry.Route.Protocol.APIVersion
	if version != "v1" && version != "v1beta" {
		version = "v1beta"
	}
	return gemini.New(gemini.Config{APIKey: key, BaseURL: endpointOrigin(input.Endpoint), APIVersion: version, HTTPClient: input.HTTPClient})
}

func buildDeepSeek(_ context.Context, input FactoryInput) (modelinvoker.Provider, error) {
	key, err := requiredSecret(input.Secret, upstream.CredentialPurposeAPIKey)
	if err != nil {
		return nil, err
	}
	return deepseek.New(deepseek.Config{APIKey: key, BaseURL: endpointOrigin(input.Endpoint), HTTPClient: input.HTTPClient})
}

func buildKimi(_ context.Context, input FactoryInput) (modelinvoker.Provider, error) {
	key, err := requiredSecret(input.Secret, upstream.CredentialPurposeAPIKey)
	if err != nil {
		return nil, err
	}
	return kimi.New(kimi.Config{APIKey: key, BaseURL: input.Endpoint, HTTPClient: input.HTTPClient})
}

func buildZAI(_ context.Context, input FactoryInput) (modelinvoker.Provider, error) {
	key, err := requiredSecret(input.Secret, upstream.CredentialPurposeAPIKey)
	if err != nil {
		return nil, err
	}
	return zai.New(zai.Config{APIKey: key, BaseURL: input.Endpoint, HTTPClient: input.HTTPClient})
}

func buildMiniMax(_ context.Context, input FactoryInput) (modelinvoker.Provider, error) {
	key, err := requiredSecret(input.Secret, upstream.CredentialPurposeAPIKey)
	if err != nil {
		return nil, err
	}
	return minimax.New(minimax.Config{APIKey: key, BaseURL: endpointOrigin(input.Endpoint), HTTPClient: input.HTTPClient})
}

func buildMiMo(_ context.Context, input FactoryInput) (modelinvoker.Provider, error) {
	key, err := requiredSecret(input.Secret, upstream.CredentialPurposeAPIKey)
	if err != nil {
		return nil, err
	}
	return mimo.New(mimo.Config{APIKey: key, BaseURL: endpointOrigin(input.Endpoint), HTTPClient: input.HTTPClient})
}

func buildQwen(_ context.Context, input FactoryInput) (modelinvoker.Provider, error) {
	key, err := requiredSecret(input.Secret, upstream.CredentialPurposeAPIKey)
	if err != nil {
		return nil, err
	}
	return qwen.New(qwen.Config{APIKey: key, Region: qwen.Region(input.Binding.Region), WorkspaceID: input.Binding.Workspace, HTTPClient: input.HTTPClient})
}

func buildXAI(_ context.Context, input FactoryInput) (modelinvoker.Provider, error) {
	key, err := requiredSecret(input.Secret, upstream.CredentialPurposeAPIKey)
	if err != nil {
		return nil, err
	}
	return xai.New(xai.Config{APIKey: key, HTTPClient: input.HTTPClient})
}

func buildBedrockMantle(_ context.Context, input FactoryInput) (modelinvoker.Provider, error) {
	config := bedrockmantle.Config{Region: input.Binding.Region, ProjectRef: input.Binding.Project, BaseURL: endpointOrigin(input.Endpoint), HTTPClient: input.HTTPClient}
	switch input.Entry.Route.Credential.Type {
	case upstream.CredentialAPIKey:
		key, err := requiredSecret(input.Secret, upstream.CredentialPurposeAPIKey)
		if err != nil {
			return nil, err
		}
		config.CredentialMode, config.APIKey = bedrockmantle.CredentialAPIKey, key
	case upstream.CredentialSigV4:
		access, err := requiredSecret(input.Secret, upstream.CredentialPurposeAccessKeyID)
		if err != nil {
			return nil, err
		}
		secret, err := requiredSecret(input.Secret, upstream.CredentialPurposeSecretAccessKey)
		if err != nil {
			return nil, err
		}
		config.CredentialMode, config.AccessKeyID, config.SecretAccessKey = bedrockmantle.CredentialSigV4, access, secret
		config.SessionToken, _ = input.Secret.value(upstream.CredentialPurposeSessionToken)
	default:
		return nil, unsupportedFactoryCredential(input)
	}
	return bedrockmantle.New(config)
}

func buildBedrockRuntime(_ context.Context, input FactoryInput) (modelinvoker.Provider, error) {
	config := bedrockruntime.Config{Region: input.Binding.Region, BaseURL: endpointOrigin(input.Endpoint), HTTPClient: input.HTTPClient}
	switch input.Entry.Route.Credential.Type {
	case upstream.CredentialBearer:
		token, err := requiredSecret(input.Secret, upstream.CredentialPurposeBearerToken)
		if err != nil {
			return nil, err
		}
		config.CredentialMode, config.BearerToken = bedrockruntime.CredentialBearer, token
	case upstream.CredentialSigV4:
		access, err := requiredSecret(input.Secret, upstream.CredentialPurposeAccessKeyID)
		if err != nil {
			return nil, err
		}
		secret, err := requiredSecret(input.Secret, upstream.CredentialPurposeSecretAccessKey)
		if err != nil {
			return nil, err
		}
		config.CredentialMode, config.AccessKeyID, config.SecretAccessKey = bedrockruntime.CredentialSigV4, access, secret
		config.SessionToken, _ = input.Secret.value(upstream.CredentialPurposeSessionToken)
	default:
		return nil, unsupportedFactoryCredential(input)
	}
	return bedrockruntime.New(config)
}

func buildVertex(_ context.Context, input FactoryInput) (modelinvoker.Provider, error) {
	mode := vertex.DeploymentServerless
	switch input.Entry.Route.Deployment.Kind {
	case upstream.DeploymentCloudProvisioned:
		mode = vertex.DeploymentProvisionedThroughput
	case upstream.DeploymentSelfHosted:
		mode = vertex.DeploymentSelfDeployedModelGarden
	}
	config := vertex.Config{Project: input.Binding.Project, Location: input.Binding.Region, DeploymentMode: mode, DeploymentRef: input.Binding.Resource, BaseURL: endpointOrigin(input.Endpoint), HTTPClient: input.HTTPClient}
	switch input.Entry.Route.Credential.Type {
	case upstream.CredentialAPIKey:
		key, err := requiredSecret(input.Secret, upstream.CredentialPurposeAPIKey)
		if err != nil {
			return nil, err
		}
		config.CredentialMode, config.APIKey = vertex.CredentialAPIKey, key
	case upstream.CredentialADC:
		token, err := requiredSecret(input.Secret, upstream.CredentialPurposeBearerToken)
		if err != nil {
			return nil, err
		}
		config.CredentialMode = vertex.CredentialADC
		config.AccessTokenProvider = staticAccessToken{value: token, expires: input.Secret.ExpiresAt}
	default:
		return nil, unsupportedFactoryCredential(input)
	}
	return vertex.New(config)
}

func buildAzureOpenAI(_ context.Context, input FactoryInput) (modelinvoker.Provider, error) {
	legacy := strings.Contains(string(input.Entry.ID), ".legacy.")
	config := azureopenai.Config{ResourceEndpoint: endpointOrigin(input.Endpoint), Region: input.Binding.Region, DeploymentName: input.Binding.Deployment, EnableLegacy: legacy, HTTPClient: input.HTTPClient}
	if legacy {
		config.LegacyAPIVersion = input.Entry.Route.Protocol.APIVersion
	}
	switch input.Entry.Route.Credential.Type {
	case upstream.CredentialAPIKey:
		key, err := requiredSecret(input.Secret, upstream.CredentialPurposeAPIKey)
		if err != nil {
			return nil, err
		}
		config.CredentialMode, config.APIKey = azureopenai.CredentialAPIKey, key
	case upstream.CredentialEntraID:
		token, err := requiredSecret(input.Secret, upstream.CredentialPurposeBearerToken)
		if err != nil {
			return nil, err
		}
		config.CredentialMode = azureopenai.CredentialEntraID
		config.AccessTokenProvider = staticAccessToken{value: token, expires: input.Secret.ExpiresAt}
	default:
		return nil, unsupportedFactoryCredential(input)
	}
	return azureopenai.New(config)
}

type staticAccessToken struct {
	value   string
	expires time.Time
}

func (s staticAccessToken) AccessToken(context.Context) (string, time.Time, error) {
	return s.value, s.expires, nil
}

func requiredSecret(material SecretMaterial, purpose upstream.CredentialPurpose) (string, error) {
	value, ok := material.value(purpose)
	if !ok {
		return "", gatewayError(modelinvoker.ErrorAuthentication, "factory_secret_missing", "adapter factory did not receive a required typed credential value", nil)
	}
	return value, nil
}

func unsupportedFactoryCredential(input FactoryInput) error {
	return gatewayError(modelinvoker.ErrorAuthentication, "factory_credential_unsupported", fmt.Sprintf("built-in AdapterID %q does not support catalog credential type %q", input.Entry.Implementation.AdapterID, input.Entry.Route.Credential.Type), nil)
}

func endpointOrigin(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return raw
	}
	parsed.Path, parsed.RawPath, parsed.RawQuery, parsed.Fragment = "", "", "", ""
	return strings.TrimRight(parsed.String(), "/")
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return context.Canceled
	}
	return ctx.Err()
}

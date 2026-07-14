// Package localcompat implements explicit OpenAI-compatible bindings for
// self-hosted endpoints, Ollama, and llama.cpp. It is intentionally separate
// from third-party relays and official OpenAI routes.
package localcompat

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
)

type Product string

const (
	ProductGeneric  Product = "openai-compatible"
	ProductOllama   Product = "ollama"
	ProductLlamaCPP Product = "llama.cpp"
)

const (
	ProviderGeneric  modelinvoker.ProviderID = "local-openai-compatible"
	ProviderOllama   modelinvoker.ProviderID = "ollama-openai-compatible"
	ProviderLlamaCPP modelinvoker.ProviderID = "llamacpp-openai-compatible"
)

type TrustMode string

const (
	TrustLocal      TrustMode = "local"
	TrustEnterprise TrustMode = "enterprise"
)

type Config struct {
	Product               Product
	Trust                 TrustMode
	BaseURL               string
	Protocol              modelinvoker.Protocol
	APIKey                string
	AllowedModels         []string
	SupportedCapabilities []modelinvoker.Capability
	UserAgent             string
	HTTPClient            *http.Client
}

func (Config) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "localcompat.Config([REDACTED])")
}
func (Config) GoString() string { return "localcompat.Config([REDACTED])" }

func (config Config) providerID() modelinvoker.ProviderID {
	switch config.Product {
	case ProductOllama:
		return ProviderOllama
	case ProductLlamaCPP:
		return ProviderLlamaCPP
	default:
		return ProviderGeneric
	}
}

func (config Config) validate() (string, error) {
	switch config.Product {
	case ProductGeneric, ProductOllama, ProductLlamaCPP:
	default:
		return "", fmt.Errorf("local compatible product is unsupported")
	}
	if config.Protocol != modelinvoker.ProtocolChatCompletions && config.Protocol != modelinvoker.ProtocolResponses {
		return "", fmt.Errorf("local compatible protocol must be Chat Completions or Responses")
	}
	if config.APIKey != "" && (config.APIKey != strings.TrimSpace(config.APIKey) || strings.ContainsAny(config.APIKey, "\r\n\x00")) {
		return "", fmt.Errorf("local compatible API key is invalid")
	}
	u, err := url.Parse(config.BaseURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("local compatible base URL must be absolute and credential-free")
	}
	policy := adaptercore.EndpointPolicy{}
	switch config.Trust {
	case TrustLocal:
		policy.AllowLoopback, policy.LoopbackOnly = true, true
	case TrustEnterprise:
		if u.Scheme != "https" {
			return "", fmt.Errorf("enterprise self-hosted endpoint requires HTTPS")
		}
		policy.OfficialHosts, policy.AllowLoopback = []string{u.Hostname()}, true
	default:
		return "", fmt.Errorf("local compatible trust mode is invalid")
	}
	base, err := adaptercore.ValidateEndpoint(config.BaseURL, policy)
	if err != nil {
		return "", fmt.Errorf("local compatible base URL is invalid: %w", err)
	}
	if len(config.AllowedModels) == 0 {
		return "", fmt.Errorf("local compatible endpoint requires an exact model allowlist")
	}
	seen := make(map[string]struct{}, len(config.AllowedModels))
	for _, model := range config.AllowedModels {
		if model == "" || model != strings.TrimSpace(model) || len(model) > 512 || strings.ContainsAny(model, "\r\n\x00") {
			return "", fmt.Errorf("local compatible model allowlist contains an invalid model")
		}
		if _, exists := seen[model]; exists {
			return "", fmt.Errorf("local compatible model allowlist contains a duplicate")
		}
		seen[model] = struct{}{}
	}
	if !slices.Contains(config.SupportedCapabilities, modelinvoker.CapabilityTextGeneration) {
		return "", fmt.Errorf("local compatible endpoint must explicitly declare text generation")
	}
	seenCapabilities := map[modelinvoker.Capability]struct{}{}
	for _, capability := range config.SupportedCapabilities {
		if capability == "" {
			return "", fmt.Errorf("local compatible capability allowlist contains an empty value")
		}
		if _, exists := seenCapabilities[capability]; exists {
			return "", fmt.Errorf("local compatible capability allowlist contains a duplicate")
		}
		seenCapabilities[capability] = struct{}{}
	}
	if config.UserAgent != "" && (config.UserAgent != strings.TrimSpace(config.UserAgent) || len(config.UserAgent) > 512 || strings.ContainsAny(config.UserAgent, "\r\n")) {
		return "", fmt.Errorf("local compatible user agent is invalid")
	}
	return base, nil
}

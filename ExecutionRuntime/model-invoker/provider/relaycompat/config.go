// Package relaycompat implements an explicit third-party relay compatibility
// boundary. It never reuses or weakens an official provider's endpoint gate.
package relaycompat

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
)

const ProviderID modelinvoker.ProviderID = "third-party-relay"

type Config struct {
	APIKey            string
	BaseURL           string
	Protocol          modelinvoker.Protocol
	APIVersion        string
	AllowedModels     []string
	MessagesAuthToken bool
	UserAgent         string
	HTTPClient        *http.Client
}

func (Config) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "relaycompat.Config([REDACTED])")
}

func (Config) GoString() string { return "relaycompat.Config([REDACTED])" }

func (config Config) validate() error {
	if strings.TrimSpace(config.APIKey) == "" || strings.ContainsAny(config.APIKey, "\r\n") {
		return fmt.Errorf("third-party relay API key is required")
	}
	if config.Protocol != modelinvoker.ProtocolChatCompletions &&
		config.Protocol != modelinvoker.ProtocolResponses &&
		config.Protocol != modelinvoker.ProtocolMessages &&
		config.Protocol != modelinvoker.ProtocolGenerateContent {
		return fmt.Errorf("third-party relay protocol is unsupported")
	}
	if config.Protocol == modelinvoker.ProtocolGenerateContent && config.APIVersion != "v1" && config.APIVersion != "v1beta" {
		return fmt.Errorf("GenerateContent relay API version must be v1 or v1beta")
	}
	if config.Protocol != modelinvoker.ProtocolGenerateContent && config.APIVersion != "" {
		return fmt.Errorf("API version is valid only for GenerateContent relay routes")
	}
	if config.UserAgent != "" && (strings.TrimSpace(config.UserAgent) == "" || len(config.UserAgent) > 512 || strings.ContainsAny(config.UserAgent, "\r\n")) {
		return fmt.Errorf("third-party relay user agent must be bounded and single-line")
	}
	if _, err := trustedRelayBaseURL(config.BaseURL); err != nil {
		return err
	}
	if len(config.AllowedModels) == 0 {
		return fmt.Errorf("third-party relay requires an exact model allowlist")
	}
	seen := make(map[string]struct{}, len(config.AllowedModels))
	for _, model := range config.AllowedModels {
		if strings.TrimSpace(model) == "" || len(model) > 256 || strings.ContainsAny(model, "\r\n") {
			return fmt.Errorf("third-party relay model allowlist contains an invalid model")
		}
		if _, exists := seen[model]; exists {
			return fmt.Errorf("third-party relay model allowlist contains a duplicate")
		}
		seen[model] = struct{}{}
	}
	return nil
}

func trustedRelayBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Opaque != "" || parsed.Host == "" {
		return "", fmt.Errorf("third-party relay base URL must be an absolute HTTP(S) URL")
	}
	paths := []string(nil)
	if path := strings.TrimRight(parsed.Path, "/"); path != "" {
		paths = []string{path}
	}
	trusted, err := adaptercore.ValidateEndpoint(raw, adaptercore.EndpointPolicy{
		OfficialHosts: []string{parsed.Hostname()}, OfficialPaths: paths, AllowLoopback: true,
	})
	if err != nil {
		return "", fmt.Errorf("third-party relay base URL is not a canonical pinned endpoint: %w", err)
	}
	return trusted, nil
}

func (config Config) endpoint() string {
	base, _ := trustedRelayBaseURL(config.BaseURL)
	if config.Protocol == modelinvoker.ProtocolGenerateContent {
		return base + "/" + config.APIVersion
	}
	return base
}

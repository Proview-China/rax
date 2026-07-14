// Package compatprovider composes provider-owned dialects with the concrete
// OpenAI and Anthropic protocol drivers. It is internal so protocol SDK types
// and compatibility mechanics cannot become part of Praxis' public API.
package compatprovider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
)

type CapabilityBuilder func(context.Context, modelinvoker.CapabilityQuery) (modelinvoker.CapabilityContract, error)

type Config struct {
	Provider        modelinvoker.ProviderID
	DefaultProtocol modelinvoker.Protocol
	APIKey          string
	// AllowAnonymous permits an empty API key. Concrete public providers must
	// opt in explicitly; the transport then strips inherited authentication
	// headers so OPENAI_* environment state cannot escape to a local endpoint.
	AllowAnonymous     bool
	HTTPClient         *http.Client
	ChatEndpoint       string
	ResponsesEndpoint  string
	MessagesEndpoint   string
	GenerateEndpoint   string
	GenerateBaseURL    string
	GenerateAPIVersion string
	MessagesAuthToken  bool
	// UserAgent, when non-empty, is forced at the final HTTP transport boundary.
	// Restricted subscription routes use the attested build/runtime identity and
	// never accept a caller-supplied header through Request or ProviderOptions.
	UserAgent        string
	ChatDialect      protocol.Dialect
	ResponsesDialect protocol.Dialect
	MessagesDialect  protocol.Dialect
	GenerateDialect  protocol.Dialect
	Capabilities     CapabilityBuilder
	RequestIDHeaders []string
}

func (c Config) Validate() error {
	if strings.TrimSpace(string(c.Provider)) == "" {
		return fmt.Errorf("compatible provider: provider ID is required")
	}
	if strings.TrimSpace(c.APIKey) == "" && !c.AllowAnonymous {
		return fmt.Errorf("%s: API key is required", c.Provider)
	}
	if c.APIKey != "" && (c.APIKey != strings.TrimSpace(c.APIKey) || strings.ContainsAny(c.APIKey, "\r\n\x00")) {
		return fmt.Errorf("%s: API key is invalid", c.Provider)
	}
	if c.UserAgent != "" && (strings.TrimSpace(c.UserAgent) == "" || len(c.UserAgent) > 512 || strings.ContainsAny(c.UserAgent, "\r\n")) {
		return fmt.Errorf("%s: user agent must be bounded and single-line", c.Provider)
	}
	configured := map[modelinvoker.Protocol]struct{}{}
	checks := []struct {
		protocol modelinvoker.Protocol
		endpoint string
		dialect  protocol.Dialect
	}{
		{modelinvoker.ProtocolChatCompletions, c.ChatEndpoint, c.ChatDialect},
		{modelinvoker.ProtocolResponses, c.ResponsesEndpoint, c.ResponsesDialect},
		{modelinvoker.ProtocolMessages, c.MessagesEndpoint, c.MessagesDialect},
		{modelinvoker.ProtocolGenerateContent, c.GenerateEndpoint, c.GenerateDialect},
	}
	for _, check := range checks {
		if check.endpoint == "" {
			if !protocol.IsNil(check.dialect) {
				return fmt.Errorf("%s: %s dialect requires an endpoint", c.Provider, check.protocol)
			}
			continue
		}
		if protocol.IsNil(check.dialect) {
			return fmt.Errorf("%s: %s endpoint requires a dialect", c.Provider, check.protocol)
		}
		if err := validateEndpoint(c.Provider, check.endpoint); err != nil {
			return err
		}
		configured[check.protocol] = struct{}{}
	}
	if c.GenerateEndpoint != "" {
		if err := validateEndpoint(c.Provider, c.GenerateBaseURL); err != nil {
			return fmt.Errorf("%s: GenerateContent base URL is invalid: %w", c.Provider, err)
		}
		if c.GenerateAPIVersion != "v1" && c.GenerateAPIVersion != "v1beta" {
			return fmt.Errorf("%s: GenerateContent API version must be v1 or v1beta", c.Provider)
		}
	}
	if len(configured) == 0 {
		return fmt.Errorf("%s: at least one protocol endpoint is required", c.Provider)
	}
	if _, ok := configured[c.DefaultProtocol]; !ok {
		return fmt.Errorf("%s: default protocol is not configured", c.Provider)
	}
	if c.Capabilities == nil {
		return fmt.Errorf("%s: capability builder is required", c.Provider)
	}
	for _, header := range c.RequestIDHeaders {
		if strings.TrimSpace(header) == "" || strings.ContainsAny(header, "\r\n") {
			return fmt.Errorf("%s: request ID header is invalid", c.Provider)
		}
	}
	return nil
}

func validateEndpoint(provider modelinvoker.ProviderID, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("%s: endpoint must be an absolute HTTP(S) URL", provider)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("%s: endpoint must be credential-free, query-free, and fragment-free", provider)
	}
	if u.Scheme == "http" && !adaptercore.IsLoopbackHost(u.Hostname()) {
		return fmt.Errorf("%s: plain HTTP is allowed only for loopback tests", provider)
	}
	return nil
}

package anthropic

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
)

// Config contains transport settings for the native Anthropic Messages API.
// APIKey is never copied into audit payloads or public errors.
type Config struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

func (Config) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "anthropic.Config([REDACTED])")
}

func (Config) GoString() string {
	return "anthropic.Config([REDACTED])"
}

func (c Config) validate() error {
	if strings.TrimSpace(c.APIKey) == "" {
		return fmt.Errorf("anthropic: API key is required")
	}
	_, err := c.trustedBaseURL()
	return err
}

func (c Config) trustedBaseURL() (string, error) {
	endpoint := c.BaseURL
	if endpoint == "" {
		endpoint = defaultBaseURL
	}
	trusted, err := adaptercore.ValidateEndpoint(endpoint, adaptercore.EndpointPolicy{
		OfficialHosts: []string{"api.anthropic.com"}, AllowLoopback: true,
	})
	if err != nil {
		return "", fmt.Errorf("anthropic: invalid base URL: %w", err)
	}
	return trusted, nil
}

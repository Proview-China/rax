package openai

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
)

// Config contains the transport settings for the native OpenAI provider.
// APIKey is never included in request audit payloads or returned errors.
type Config struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

func (Config) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "openai.Config([REDACTED])")
}

func (Config) GoString() string {
	return "openai.Config([REDACTED])"
}

func (c Config) validate() error {
	if strings.TrimSpace(c.APIKey) == "" {
		return fmt.Errorf("openai: API key is required")
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
		OfficialHosts: []string{"api.openai.com"}, OfficialPaths: []string{"/v1"}, AllowLoopback: true,
	})
	if err != nil {
		return "", fmt.Errorf("openai: invalid base URL: %w", err)
	}
	return trusted, nil
}

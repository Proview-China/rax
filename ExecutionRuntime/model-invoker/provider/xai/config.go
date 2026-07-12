package xai

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
)

const defaultBaseURL = "https://api.x.ai/v1"

type Config struct {
	APIKey string
	// BaseURL is reserved for loopback protocol tests. Production traffic is
	// fixed to the xAI API so credentials cannot reach an arbitrary compatible
	// host or the Grok consumer product.
	BaseURL    string
	HTTPClient *http.Client
}

func (Config) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "xai.Config([REDACTED])")
}
func (Config) GoString() string { return "xai.Config([REDACTED])" }

func (c Config) validate() error {
	key := strings.TrimSpace(c.APIKey)
	if key == "" {
		return fmt.Errorf("xai: API key is required")
	}
	for _, character := range key {
		if unicode.IsControl(character) {
			return fmt.Errorf("xai: API key must not contain control characters")
		}
	}
	_, err := c.trustedEndpoint()
	return err
}

func (c Config) trustedEndpoint() (string, error) {
	if c.BaseURL == "" {
		return defaultBaseURL, nil
	}
	endpoint, err := adaptercore.ValidateEndpoint(c.BaseURL, adaptercore.EndpointPolicy{AllowLoopback: true, LoopbackOnly: true})
	if err != nil {
		return "", fmt.Errorf("xai: invalid test Base URL: %w", err)
	}
	return endpoint, nil
}

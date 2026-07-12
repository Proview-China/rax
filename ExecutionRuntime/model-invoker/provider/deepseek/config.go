package deepseek

import (
	"fmt"
	"io"
	"net/http"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
)

const defaultBaseURL = "https://api.deepseek.com"

type Config struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

func (Config) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "deepseek.Config([REDACTED])")
}
func (Config) GoString() string { return "deepseek.Config([REDACTED])" }

func (c Config) root() (string, error) {
	raw := c.BaseURL
	if raw == "" {
		raw = defaultBaseURL
	}
	endpoint, err := adaptercore.ValidateEndpoint(raw, adaptercore.EndpointPolicy{
		OfficialHosts: []string{"api.deepseek.com"}, AllowLoopback: true,
	})
	if err != nil {
		return "", fmt.Errorf("deepseek: invalid base URL: %w", err)
	}
	return endpoint, nil
}

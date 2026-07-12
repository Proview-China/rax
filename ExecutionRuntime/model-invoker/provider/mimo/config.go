package mimo

import (
	"fmt"
	"io"
	"net/http"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
)

const defaultRootURL = "https://api.xiaomimimo.com"

type Config struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

func (Config) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "mimo.Config([REDACTED])")
}
func (Config) GoString() string { return "mimo.Config([REDACTED])" }

func (c Config) rootEndpoint() (string, error) {
	raw := c.BaseURL
	if raw == "" {
		raw = defaultRootURL
	}
	endpoint, err := adaptercore.ValidateEndpoint(raw, adaptercore.EndpointPolicy{
		OfficialHosts: []string{"api.xiaomimimo.com"}, AllowLoopback: true,
	})
	if err != nil {
		return "", fmt.Errorf("MiMo invalid base URL: %w", err)
	}
	return endpoint, nil
}

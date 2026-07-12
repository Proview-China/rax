package zai

import (
	"fmt"
	"io"
	"net/http"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
)

const defaultBaseURL = "https://api.z.ai/api/paas/v4"

type Config struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

func (Config) Format(state fmt.State, _ rune) { _, _ = io.WriteString(state, "zai.Config([REDACTED])") }
func (Config) GoString() string               { return "zai.Config([REDACTED])" }
func (c Config) endpoint() (string, error) {
	raw := c.BaseURL
	if raw == "" {
		raw = defaultBaseURL
	}
	endpoint, err := adaptercore.ValidateEndpoint(raw, adaptercore.EndpointPolicy{
		OfficialHosts: []string{"api.z.ai"}, OfficialPaths: []string{"/api/paas/v4"}, AllowLoopback: true,
	})
	if err != nil {
		return "", fmt.Errorf("zai: invalid base URL: %w", err)
	}
	return endpoint, nil
}

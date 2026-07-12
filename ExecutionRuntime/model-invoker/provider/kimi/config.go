package kimi

import (
	"fmt"
	"io"
	"net/http"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
)

const defaultBaseURL = "https://api.moonshot.cn/v1"

type Config struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

func (Config) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "kimi.Config([REDACTED])")
}
func (Config) GoString() string { return "kimi.Config([REDACTED])" }
func (c Config) endpoint() (string, error) {
	raw := c.BaseURL
	if raw == "" {
		raw = defaultBaseURL
	}
	endpoint, err := adaptercore.ValidateEndpoint(raw, adaptercore.EndpointPolicy{
		OfficialHosts: []string{"api.moonshot.cn"}, OfficialPaths: []string{"/v1"}, AllowLoopback: true,
	})
	if err != nil {
		return "", fmt.Errorf("kimi: invalid base URL: %w", err)
	}
	return endpoint, nil
}

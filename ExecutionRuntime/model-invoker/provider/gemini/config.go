package gemini

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
)

const (
	defaultBaseURL    = "https://generativelanguage.googleapis.com/"
	defaultAPIVersion = "v1beta"
)

// Config contains the deterministic transport settings for the Gemini
// Developer API. The adapter never falls back to SDK environment variables.
// APIKey is never included in audit payloads or returned errors.
type Config struct {
	APIKey     string
	BaseURL    string
	APIVersion string
	HTTPClient *http.Client
}

func (Config) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "gemini.Config([REDACTED])")
}

func (Config) GoString() string {
	return "gemini.Config([REDACTED])"
}

func (c Config) validate() error {
	if strings.TrimSpace(c.APIKey) == "" {
		return fmt.Errorf("gemini: API key is required")
	}
	if _, err := c.trustedBaseURL(); err != nil {
		return err
	}
	version := c.APIVersion
	if version == "" {
		version = defaultAPIVersion
	}
	if version != "v1" && version != "v1beta" {
		return fmt.Errorf("gemini: API version must be v1 or v1beta")
	}
	return nil
}

func (c Config) trustedBaseURL() (string, error) {
	baseURL := c.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	trusted, err := adaptercore.ValidateEndpoint(baseURL, adaptercore.EndpointPolicy{
		OfficialHosts: []string{"generativelanguage.googleapis.com"}, AllowLoopback: true,
	})
	if err != nil {
		return "", fmt.Errorf("gemini: invalid base URL: %w", err)
	}
	return trusted, nil
}

func (c Config) effectiveBaseURL() string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/") + "/"
	}
	return defaultBaseURL
}

func (c Config) effectiveAPIVersion() string {
	if c.APIVersion != "" {
		return c.APIVersion
	}
	return defaultAPIVersion
}

func effectiveEndpoint(baseURL, apiVersion string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return strings.TrimRight(baseURL, "/") + "/" + apiVersion
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + apiVersion
	return strings.TrimRight(u.String(), "/")
}

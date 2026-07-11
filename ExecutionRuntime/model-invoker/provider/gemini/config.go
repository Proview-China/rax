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
	baseURL := c.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("gemini: invalid base URL: %w", err)
	}
	if u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("gemini: base URL must be an absolute HTTP(S) URL")
	}
	if u.User != nil {
		return fmt.Errorf("gemini: base URL must not contain credentials")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("gemini: base URL must not contain a query or fragment")
	}
	if isVertexEndpoint(u) {
		return fmt.Errorf("gemini: Vertex AI/aiplatform endpoints are not supported")
	}
	if u.Scheme == "http" && !adaptercore.IsLoopbackHost(u.Hostname()) {
		return fmt.Errorf("gemini: insecure HTTP is allowed only for loopback test servers")
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

func isVertexEndpoint(u *url.URL) bool {
	if u == nil {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "aiplatform.googleapis.com" ||
		strings.HasSuffix(host, "-aiplatform.googleapis.com") ||
		host == "vertexai.googleapis.com" ||
		strings.HasSuffix(host, ".vertexai.googleapis.com") {
		return true
	}
	path := strings.ToLower("/" + strings.Trim(u.Path, "/"))
	return strings.Contains(path, "/projects/") &&
		strings.Contains(path, "/locations/") &&
		(strings.Contains(path, "/publishers/") || strings.Contains(path, "/models/"))
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

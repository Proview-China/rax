package mimo

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

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
	raw := strings.TrimSpace(c.BaseURL)
	if raw == "" {
		raw = defaultRootURL
	}
	raw = strings.TrimRight(raw, "/")
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return "", fmt.Errorf("MiMo base URL must be an absolute HTTP(S) URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("MiMo base URL must not contain credentials, a query, or a fragment")
	}
	loopback := adaptercore.IsLoopbackHost(parsed.Hostname())
	if !loopback && (parsed.Scheme != "https" || !strings.EqualFold(parsed.Hostname(), "api.xiaomimimo.com") || (parsed.Path != "" && parsed.Path != "/")) {
		return "", fmt.Errorf("MiMo pay-as-you-go base URL must use the official host or a loopback test server")
	}
	return raw, nil
}

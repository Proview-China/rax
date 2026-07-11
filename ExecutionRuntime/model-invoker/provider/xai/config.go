package xai

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	if strings.TrimSpace(c.BaseURL) == "" {
		return nil
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("xai: test Base URL must be absolute, credential-free, query-free, and fragment-free")
	}
	if !adaptercore.IsLoopbackHost(u.Hostname()) {
		return fmt.Errorf("xai: Base URL override is allowed only for loopback tests")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("xai: test Base URL must use HTTP or HTTPS")
	}
	return nil
}

func (c Config) endpoint() string {
	if strings.TrimSpace(c.BaseURL) != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return defaultBaseURL
}

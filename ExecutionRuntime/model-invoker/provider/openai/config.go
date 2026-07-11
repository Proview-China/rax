package openai

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	if c.BaseURL == "" {
		return nil
	}

	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return fmt.Errorf("openai: invalid base URL: %w", err)
	}
	if u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("openai: base URL must be an absolute HTTP(S) URL")
	}
	if u.User != nil {
		return fmt.Errorf("openai: base URL must not contain credentials")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("openai: base URL must not contain a query or fragment")
	}
	if u.Scheme == "http" && !adaptercore.IsLoopbackHost(u.Hostname()) {
		return fmt.Errorf("openai: insecure HTTP is allowed only for loopback test servers")
	}
	return nil
}

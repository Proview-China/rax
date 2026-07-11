package anthropic

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// Config contains transport settings for the native Anthropic Messages API.
// APIKey is never copied into audit payloads or public errors.
type Config struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

func (Config) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "anthropic.Config([REDACTED])")
}

func (Config) GoString() string {
	return "anthropic.Config([REDACTED])"
}

func (c Config) validate() error {
	if strings.TrimSpace(c.APIKey) == "" {
		return fmt.Errorf("anthropic: API key is required")
	}
	if c.BaseURL == "" {
		return nil
	}

	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return fmt.Errorf("anthropic: invalid base URL: %w", err)
	}
	if u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("anthropic: base URL must be an absolute HTTP(S) URL")
	}
	if u.User != nil {
		return fmt.Errorf("anthropic: base URL must not contain credentials")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("anthropic: base URL must not contain a query or fragment")
	}
	if u.Scheme == "http" && !isLoopbackHost(u.Hostname()) {
		return fmt.Errorf("anthropic: insecure HTTP is allowed only for loopback test servers")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

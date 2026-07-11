package deepseek

import (
	"fmt"
	"io"
	"net/http"
	"strings"
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

func (c Config) root() string {
	if strings.TrimSpace(c.BaseURL) == "" {
		return defaultBaseURL
	}
	return strings.TrimRight(c.BaseURL, "/")
}

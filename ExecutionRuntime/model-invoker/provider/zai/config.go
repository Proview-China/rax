package zai

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

const defaultBaseURL = "https://api.z.ai/api/paas/v4"

type Config struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

func (Config) Format(state fmt.State, _ rune) { _, _ = io.WriteString(state, "zai.Config([REDACTED])") }
func (Config) GoString() string               { return "zai.Config([REDACTED])" }
func (c Config) endpoint() string {
	if strings.TrimSpace(c.BaseURL) == "" {
		return defaultBaseURL
	}
	return strings.TrimRight(c.BaseURL, "/")
}

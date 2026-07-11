package kimi

import (
	"fmt"
	"io"
	"net/http"
	"strings"
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
func (c Config) endpoint() string {
	if strings.TrimSpace(c.BaseURL) == "" {
		return defaultBaseURL
	}
	return strings.TrimRight(c.BaseURL, "/")
}

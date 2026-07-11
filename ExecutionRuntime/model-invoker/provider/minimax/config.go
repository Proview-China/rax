package minimax

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

const defaultRootURL = "https://api.minimax.io"

type Config struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

func (Config) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "minimax.Config([REDACTED])")
}
func (Config) GoString() string { return "minimax.Config([REDACTED])" }

func (c Config) rootEndpoint() string {
	if strings.TrimSpace(c.BaseURL) == "" {
		return defaultRootURL
	}
	return strings.TrimRight(c.BaseURL, "/")
}

func (c Config) openAIEndpoint() string   { return c.rootEndpoint() + "/v1" }
func (c Config) messagesEndpoint() string { return c.rootEndpoint() + "/anthropic" }

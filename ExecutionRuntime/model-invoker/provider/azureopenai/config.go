package azureopenai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
)

type CredentialMode string

const (
	CredentialAPIKey  CredentialMode = "api_key"
	CredentialEntraID CredentialMode = "entra_id"
)

type APIKeyProvider interface {
	APIKey(context.Context) (string, time.Time, error)
}
type AccessTokenProvider interface {
	AccessToken(context.Context) (string, time.Time, error)
}
type Config struct {
	ResourceEndpoint    string
	Region              string
	DeploymentName      string
	CredentialMode      CredentialMode
	APIKey              string
	APIKeyProvider      APIKeyProvider
	AccessTokenProvider AccessTokenProvider
	EnableLegacy        bool
	LegacyAPIVersion    string
	HTTPClient          *http.Client
}

func (Config) Format(s fmt.State, _ rune) { _, _ = io.WriteString(s, "azureopenai.Config([REDACTED])") }
func (Config) GoString() string           { return "azureopenai.Config([REDACTED])" }
func (c Config) validate() error {
	if strings.TrimSpace(c.Region) == "" || strings.ContainsAny(c.Region, "\r\n/ ") {
		return fmt.Errorf("azure openai: region is required")
	}
	if strings.TrimSpace(c.DeploymentName) == "" || strings.ContainsAny(c.DeploymentName, "\r\n/?#") {
		return fmt.Errorf("azure openai: deployment name is required")
	}
	u, err := url.Parse(c.ResourceEndpoint)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("azure openai: resource endpoint must be absolute, credential-free, and query-free")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && adaptercore.IsLoopbackHost(u.Hostname())) {
		return fmt.Errorf("azure openai: plain HTTP is allowed only for loopback tests")
	}
	switch c.CredentialMode {
	case CredentialAPIKey:
		if (strings.TrimSpace(c.APIKey) == "") == (c.APIKeyProvider == nil) {
			return fmt.Errorf("azure openai: API key mode requires exactly one static or renewable key source")
		}
		if c.AccessTokenProvider != nil {
			return fmt.Errorf("azure openai: API key and Entra ID credentials cannot be mixed")
		}
	case CredentialEntraID:
		if c.AccessTokenProvider == nil {
			return fmt.Errorf("azure openai: Entra ID mode requires a renewable access token provider")
		}
		if c.APIKey != "" || c.APIKeyProvider != nil {
			return fmt.Errorf("azure openai: Entra ID and API key credentials cannot be mixed")
		}
	default:
		return fmt.Errorf("azure openai: credential mode is invalid")
	}
	if c.EnableLegacy {
		if strings.TrimSpace(c.LegacyAPIVersion) == "" || strings.ContainsAny(c.LegacyAPIVersion, "\r\n&=?") {
			return fmt.Errorf("azure openai: legacy binding requires a safe dated API version")
		}
	} else if c.LegacyAPIVersion != "" {
		return fmt.Errorf("azure openai: legacy API version cannot be set when legacy binding is disabled")
	}
	return nil
}
func (c Config) root() string { return adaptercore.NormalizeEndpoint(c.ResourceEndpoint) }

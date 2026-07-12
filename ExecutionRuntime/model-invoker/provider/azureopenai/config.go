package azureopenai

import (
	"context"
	"fmt"
	"io"
	"net/http"
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
	if !adaptercore.IsCloudRegion(c.Region) {
		return fmt.Errorf("azure openai: region must be a canonical cloud region label")
	}
	if !adaptercore.IsPathSegment(c.DeploymentName) {
		return fmt.Errorf("azure openai: deployment name must be one canonical identifier segment")
	}
	if _, err := c.trustedRootEndpoint(); err != nil {
		return err
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
func (c Config) root() string {
	endpoint, _ := c.trustedRootEndpoint()
	return endpoint
}
func (c Config) trustedRootEndpoint() (string, error) {
	endpoint, err := adaptercore.ValidateEndpoint(c.ResourceEndpoint, adaptercore.EndpointPolicy{
		OfficialHostSuffix: "openai.azure.com", AllowLoopback: true,
	})
	if err != nil {
		return "", fmt.Errorf("azure openai: resource endpoint does not match the single-resource credential audience: %w", err)
	}
	return endpoint, nil
}

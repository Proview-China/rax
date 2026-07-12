package bedrockmantle

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
	CredentialAPIKey       CredentialMode = "bedrock_api_key"
	CredentialSigV4        CredentialMode = "sigv4"
	CredentialDefaultChain CredentialMode = "default_chain"
)

type APIKeyProvider interface {
	APIKey(context.Context) (value string, expires time.Time, err error)
}

type Config struct {
	Region          string
	ProjectRef      string
	BaseURL         string
	CredentialMode  CredentialMode
	APIKey          string
	APIKeyProvider  APIKeyProvider
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Profile         string
	StoreResponses  *bool
	HTTPClient      *http.Client
}

func (Config) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "bedrockmantle.Config([REDACTED])")
}
func (Config) GoString() string { return "bedrockmantle.Config([REDACTED])" }

func (c Config) validate() error {
	if !adaptercore.IsCloudRegion(c.Region) {
		return fmt.Errorf("bedrock mantle: region must be a canonical cloud region label")
	}
	if !adaptercore.IsPathSegment(c.ProjectRef) {
		return fmt.Errorf("bedrock mantle: project reference must be one canonical identifier segment")
	}
	if _, err := c.trustedRootEndpoint(); err != nil {
		return err
	}
	switch c.CredentialMode {
	case CredentialAPIKey:
		if (strings.TrimSpace(c.APIKey) == "") == (c.APIKeyProvider == nil) {
			return fmt.Errorf("bedrock mantle: API key mode requires exactly one static or renewable key source")
		}
		if strings.HasPrefix(strings.TrimSpace(c.APIKey), "sk-") {
			return fmt.Errorf("bedrock mantle: OpenAI API keys are not Bedrock credentials")
		}
		if c.AccessKeyID != "" || c.SecretAccessKey != "" || c.SessionToken != "" || c.Profile != "" {
			return fmt.Errorf("bedrock mantle: API key and AWS signing credentials cannot be mixed")
		}
	case CredentialSigV4:
		if c.AccessKeyID == "" || c.SecretAccessKey == "" {
			return fmt.Errorf("bedrock mantle: SigV4 requires access key ID and secret access key")
		}
		if c.APIKey != "" || c.APIKeyProvider != nil || c.Profile != "" {
			return fmt.Errorf("bedrock mantle: SigV4 credentials cannot be mixed")
		}
	case CredentialDefaultChain:
		if c.APIKey != "" || c.APIKeyProvider != nil || c.AccessKeyID != "" || c.SecretAccessKey != "" || c.SessionToken != "" {
			return fmt.Errorf("bedrock mantle: default chain cannot be mixed with explicit credentials")
		}
	default:
		return fmt.Errorf("bedrock mantle: credential mode is invalid")
	}
	return nil
}

func (c Config) rootEndpoint() string {
	endpoint, _ := c.trustedRootEndpoint()
	return endpoint
}
func (c Config) trustedRootEndpoint() (string, error) {
	host := "bedrock-mantle." + c.Region + ".api.aws"
	raw := c.BaseURL
	if raw == "" {
		raw = "https://" + host
	}
	endpoint, err := adaptercore.ValidateEndpoint(raw, adaptercore.EndpointPolicy{OfficialHosts: []string{host}, AllowLoopback: true})
	if err != nil {
		return "", fmt.Errorf("bedrock mantle: endpoint does not match the Region-derived credential audience: %w", err)
	}
	return endpoint, nil
}
func (c Config) storesResponses() bool { return c.StoreResponses == nil || *c.StoreResponses }

package bedrockmantle

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
	if strings.TrimSpace(c.Region) == "" || strings.ContainsAny(c.Region, "\r\n/ ") {
		return fmt.Errorf("bedrock mantle: region is required")
	}
	if strings.TrimSpace(c.ProjectRef) == "" || strings.ContainsAny(c.ProjectRef, "\r\n") {
		return fmt.Errorf("bedrock mantle: stable project reference is required")
	}
	if c.BaseURL != "" {
		u, err := url.Parse(c.BaseURL)
		if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("bedrock mantle: base URL must be absolute and credential-free")
		}
		if u.Scheme != "https" && !(u.Scheme == "http" && adaptercore.IsLoopbackHost(u.Hostname())) {
			return fmt.Errorf("bedrock mantle: plain HTTP is allowed only for loopback tests")
		}
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
	if c.BaseURL != "" {
		return adaptercore.NormalizeEndpoint(c.BaseURL)
	}
	return "https://bedrock-mantle." + c.Region + ".api.aws"
}
func (c Config) storesResponses() bool { return c.StoreResponses == nil || *c.StoreResponses }

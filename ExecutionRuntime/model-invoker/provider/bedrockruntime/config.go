package bedrockruntime

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
	CredentialSigV4        CredentialMode = "sigv4"
	CredentialBearer       CredentialMode = "bearer"
	CredentialDefaultChain CredentialMode = "default_chain"
)

// BearerTokenProvider is SDK-neutral so AWS SDK values never cross the public
// provider configuration boundary. Expiry must be non-zero for renewable keys.
type BearerTokenProvider interface {
	Token(context.Context) (value string, expires time.Time, err error)
}

type Config struct {
	Region          string
	BaseURL         string
	CredentialMode  CredentialMode
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	BearerToken     string
	TokenProvider   BearerTokenProvider
	Profile         string
	HTTPClient      *http.Client
}

func (Config) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "bedrockruntime.Config([REDACTED])")
}
func (Config) GoString() string { return "bedrockruntime.Config([REDACTED])" }

func (c Config) validate() error {
	if !adaptercore.IsCloudRegion(c.Region) {
		return fmt.Errorf("bedrock runtime: region must be a canonical cloud region label")
	}
	if _, err := c.trustedEndpoint(); err != nil {
		return err
	}
	switch c.CredentialMode {
	case CredentialSigV4:
		if strings.TrimSpace(c.AccessKeyID) == "" || strings.TrimSpace(c.SecretAccessKey) == "" {
			return fmt.Errorf("bedrock runtime: SigV4 requires access key ID and secret access key")
		}
		if c.BearerToken != "" || c.TokenProvider != nil || c.Profile != "" {
			return fmt.Errorf("bedrock runtime: SigV4 credentials cannot be mixed with bearer or profile credentials")
		}
	case CredentialBearer:
		if (strings.TrimSpace(c.BearerToken) == "") == (c.TokenProvider == nil) {
			return fmt.Errorf("bedrock runtime: bearer mode requires exactly one static token or renewable token provider")
		}
		if strings.HasPrefix(strings.TrimSpace(c.BearerToken), "sk-") {
			return fmt.Errorf("bedrock runtime: OpenAI API keys are not Bedrock bearer credentials")
		}
		if c.AccessKeyID != "" || c.SecretAccessKey != "" || c.SessionToken != "" || c.Profile != "" {
			return fmt.Errorf("bedrock runtime: bearer credentials cannot be mixed with SigV4 or profile credentials")
		}
	case CredentialDefaultChain:
		if c.AccessKeyID != "" || c.SecretAccessKey != "" || c.SessionToken != "" || c.BearerToken != "" || c.TokenProvider != nil {
			return fmt.Errorf("bedrock runtime: default chain cannot be mixed with explicit credentials")
		}
	default:
		return fmt.Errorf("bedrock runtime: credential mode must be sigv4, bearer, or default_chain")
	}
	return nil
}

func (c Config) endpoint() string {
	endpoint, _ := c.trustedEndpoint()
	return endpoint
}
func (c Config) trustedEndpoint() (string, error) {
	host := "bedrock-runtime." + c.Region + ".amazonaws.com"
	raw := c.BaseURL
	if raw == "" {
		raw = "https://" + host
	}
	endpoint, err := adaptercore.ValidateEndpoint(raw, adaptercore.EndpointPolicy{OfficialHosts: []string{host}, AllowLoopback: true})
	if err != nil {
		return "", fmt.Errorf("bedrock runtime: endpoint does not match the Region-derived credential audience: %w", err)
	}
	return endpoint, nil
}

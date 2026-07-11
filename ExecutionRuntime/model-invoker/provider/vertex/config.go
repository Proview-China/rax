package vertex

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
	CredentialADC    CredentialMode = "adc"
	CredentialAPIKey CredentialMode = "api_key"
)

type DeploymentMode string

const (
	DeploymentServerless              DeploymentMode = "serverless"
	DeploymentProvisionedThroughput   DeploymentMode = "provisioned_throughput"
	DeploymentSelfDeployedModelGarden DeploymentMode = "self_deployed_model_garden"
)

type APIKeyProvider interface {
	APIKey(context.Context) (string, time.Time, error)
}
type AccessTokenProvider interface {
	AccessToken(context.Context) (string, time.Time, error)
}

type Config struct {
	Project             string
	Location            string
	DeploymentMode      DeploymentMode
	DeploymentRef       string
	BaseURL             string
	CredentialMode      CredentialMode
	APIKey              string
	APIKeyProvider      APIKeyProvider
	AccessTokenProvider AccessTokenProvider
	HTTPClient          *http.Client
}

func (Config) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "vertex.Config([REDACTED])")
}
func (Config) GoString() string { return "vertex.Config([REDACTED])" }
func (c Config) validate() error {
	for name, value := range map[string]string{"project": c.Project, "location": c.Location, "deployment reference": c.DeploymentRef} {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n/ ") {
			return fmt.Errorf("vertex: %s is required and must be a stable reference", name)
		}
	}
	switch c.DeploymentMode {
	case DeploymentServerless, DeploymentProvisionedThroughput, DeploymentSelfDeployedModelGarden:
	default:
		return fmt.Errorf("vertex: deployment mode is invalid")
	}
	if c.BaseURL != "" {
		u, err := url.Parse(c.BaseURL)
		if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("vertex: base URL must be absolute and credential-free")
		}
		if u.Scheme != "https" && !(u.Scheme == "http" && adaptercore.IsLoopbackHost(u.Hostname())) {
			return fmt.Errorf("vertex: plain HTTP is allowed only for loopback tests")
		}
	}
	switch c.CredentialMode {
	case CredentialAPIKey:
		if (strings.TrimSpace(c.APIKey) == "") == (c.APIKeyProvider == nil) {
			return fmt.Errorf("vertex: API key mode requires exactly one static or renewable key source")
		}
		if c.AccessTokenProvider != nil {
			return fmt.Errorf("vertex: API key and ADC token sources cannot be mixed")
		}
	case CredentialADC:
		if c.APIKey != "" || c.APIKeyProvider != nil {
			return fmt.Errorf("vertex: ADC cannot be mixed with API key credentials")
		}
	default:
		return fmt.Errorf("vertex: credential mode must be adc or api_key")
	}
	return nil
}
func (c Config) rootEndpoint() string {
	if c.BaseURL != "" {
		return adaptercore.NormalizeEndpoint(c.BaseURL)
	}
	return "https://" + c.Location + "-aiplatform.googleapis.com"
}

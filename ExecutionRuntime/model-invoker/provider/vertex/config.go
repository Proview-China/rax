package vertex

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
	if !adaptercore.IsDNSLabel(c.Project) {
		return fmt.Errorf("vertex: project must be a canonical lowercase DNS label")
	}
	if !adaptercore.IsCloudRegion(c.Location) {
		return fmt.Errorf("vertex: location must be a canonical cloud location label")
	}
	if !adaptercore.IsPathSegment(c.DeploymentRef) {
		return fmt.Errorf("vertex: deployment reference must be one canonical identifier segment")
	}
	switch c.DeploymentMode {
	case DeploymentServerless, DeploymentProvisionedThroughput, DeploymentSelfDeployedModelGarden:
	default:
		return fmt.Errorf("vertex: deployment mode is invalid")
	}
	if _, err := c.trustedRootEndpoint(); err != nil {
		return err
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
	endpoint, _ := c.trustedRootEndpoint()
	return endpoint
}
func (c Config) trustedRootEndpoint() (string, error) {
	host := c.Location + "-aiplatform.googleapis.com"
	raw := c.BaseURL
	if raw == "" {
		raw = "https://" + host
	}
	endpoint, err := adaptercore.ValidateEndpoint(raw, adaptercore.EndpointPolicy{OfficialHosts: []string{host}, AllowLoopback: true})
	if err != nil {
		return "", fmt.Errorf("vertex: endpoint does not match the Location-derived credential audience: %w", err)
	}
	return endpoint, nil
}

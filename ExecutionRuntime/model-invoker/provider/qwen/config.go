package qwen

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
)

type Region string

const (
	RegionChinaBeijing Region = "cn-beijing"
	RegionSingapore    Region = "ap-southeast-1"
)

var workspacePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

type Config struct {
	APIKey      string
	Region      Region
	WorkspaceID string
	// BaseURL is reserved for loopback protocol tests. Production endpoints are
	// derived from Region and WorkspaceID so credentials cannot be sent to an
	// arbitrary compatible host.
	BaseURL    string
	HTTPClient *http.Client
}

func (Config) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "qwen.Config([REDACTED])")
}
func (Config) GoString() string { return "qwen.Config([REDACTED])" }

func (c Config) validate() error {
	key := strings.TrimSpace(c.APIKey)
	if key == "" {
		return fmt.Errorf("qwen: API key is required")
	}
	if strings.HasPrefix(key, "sk-sp-") {
		return fmt.Errorf("qwen: Coding Plan and Token Plan keys are not accepted by the pay-as-you-go adapter")
	}
	if !strings.HasPrefix(key, "sk-") {
		return fmt.Errorf("qwen: pay-as-you-go API key must use the documented sk- or sk-ws- prefix")
	}
	switch c.Region {
	case RegionChinaBeijing, RegionSingapore:
	default:
		return fmt.Errorf("qwen: region is outside the approved cn-beijing/ap-southeast-1 slice")
	}
	if c.WorkspaceID != strings.TrimSpace(c.WorkspaceID) || !workspacePattern.MatchString(c.WorkspaceID) {
		return fmt.Errorf("qwen: workspace ID must be a safe DNS label")
	}
	_, err := c.trustedEndpoint()
	return err
}

func (c Config) trustedEndpoint() (string, error) {
	if c.BaseURL != "" {
		endpoint, err := adaptercore.ValidateEndpoint(c.BaseURL, adaptercore.EndpointPolicy{AllowLoopback: true, LoopbackOnly: true})
		if err != nil {
			return "", fmt.Errorf("qwen: invalid test Base URL: %w", err)
		}
		return endpoint, nil
	}
	official := "https://" + c.WorkspaceID + "." + string(c.Region) + ".maas.aliyuncs.com/compatible-mode/v1"
	endpoint, err := adaptercore.ValidateEndpoint(official, adaptercore.EndpointPolicy{
		OfficialHosts: []string{c.WorkspaceID + "." + string(c.Region) + ".maas.aliyuncs.com"}, OfficialPaths: []string{"/compatible-mode/v1"},
	})
	if err != nil {
		return "", fmt.Errorf("qwen: derived production endpoint is invalid: %w", err)
	}
	return endpoint, nil
}

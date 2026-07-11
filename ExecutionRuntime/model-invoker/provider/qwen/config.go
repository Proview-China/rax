package qwen

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	if !workspacePattern.MatchString(strings.TrimSpace(c.WorkspaceID)) {
		return fmt.Errorf("qwen: workspace ID must be a safe DNS label")
	}
	if strings.TrimSpace(c.BaseURL) == "" {
		return nil
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("qwen: test Base URL must be absolute, credential-free, query-free, and fragment-free")
	}
	if !adaptercore.IsLoopbackHost(u.Hostname()) {
		return fmt.Errorf("qwen: Base URL override is allowed only for loopback tests")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("qwen: test Base URL must use HTTP or HTTPS")
	}
	return nil
}

func (c Config) endpoint() string {
	if strings.TrimSpace(c.BaseURL) != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return "https://" + strings.TrimSpace(c.WorkspaceID) + "." + string(c.Region) + ".maas.aliyuncs.com/compatible-mode/v1"
}

// Package plancompat implements the narrow interactive-coding slice for
// official subscription plans that explicitly allow third-party coding tools.
package plancompat

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
)

type Kind string

const (
	KimiCode         Kind = "kimi_code"
	MiniMaxTokenPlan Kind = "minimax_token_plan"
	MiMoTokenPlan    Kind = "mimo_token_plan"
	AlibabaPlan      Kind = "alibaba_plan"
)

type RouteProfile string

const (
	ProfileKimiCodeGlobal          RouteProfile = "kimi.code-membership.global"
	ProfileMiniMaxTokenGlobal      RouteProfile = "minimax.token-plan.global"
	ProfileMiMoTokenCN             RouteProfile = "mimo.token-plan.cn"
	ProfileMiMoTokenSGP            RouteProfile = "mimo.token-plan.sgp"
	ProfileMiMoTokenAMS            RouteProfile = "mimo.token-plan.ams"
	ProfileAlibabaCodingCN         RouteProfile = "alibaba.coding-plan.cn"
	ProfileAlibabaCodingIntl       RouteProfile = "alibaba.coding-plan.intl"
	ProfileAlibabaTokenTeamBeijing RouteProfile = "alibaba.token-plan-team.cn-beijing"
)

const (
	KimiCodeProvider     modelinvoker.ProviderID = "kimi-code"
	MiniMaxTokenProvider modelinvoker.ProviderID = "minimax-token-plan"
	MiMoTokenProvider    modelinvoker.ProviderID = "mimo-token-plan"
	AlibabaPlanProvider  modelinvoker.ProviderID = "alibaba-plan"
)

type Config struct {
	Kind       Kind
	Profile    RouteProfile
	APIKey     string
	BaseURL    string
	Protocol   modelinvoker.Protocol
	UserAgent  string
	HTTPClient *http.Client
}

func (Config) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "plancompat.Config([REDACTED])")
}
func (Config) GoString() string { return "plancompat.Config([REDACTED])" }

func (config Config) validate() error {
	if providerID(config.Kind) == "" {
		return fmt.Errorf("subscription plan kind is invalid")
	}
	if profileKind(config.Profile) != config.Kind {
		return fmt.Errorf("subscription plan Route profile does not match its adapter kind")
	}
	key := strings.TrimSpace(config.APIKey)
	if key == "" || strings.ContainsAny(key, "\r\n") {
		return fmt.Errorf("subscription plan API key is required")
	}
	switch config.Kind {
	case MiniMaxTokenPlan:
		if !strings.HasPrefix(key, "sk-cp-") {
			return fmt.Errorf("MiniMax Token Plan key must use the documented prefix")
		}
	case MiMoTokenPlan:
		if !strings.HasPrefix(key, "tp-") {
			return fmt.Errorf("MiMo Token Plan key must use the documented prefix")
		}
	case AlibabaPlan:
		if !strings.HasPrefix(key, "sk-sp-") {
			return fmt.Errorf("Alibaba subscription key must use the documented prefix")
		}
	}
	if config.Protocol != modelinvoker.ProtocolChatCompletions && config.Protocol != modelinvoker.ProtocolMessages {
		return fmt.Errorf("subscription plan protocol must be Chat Completions or Messages")
	}
	if strings.TrimSpace(config.UserAgent) == "" || len(config.UserAgent) > 512 || strings.ContainsAny(config.UserAgent, "\r\n") {
		return fmt.Errorf("attested real client User-Agent is required")
	}
	if _, err := config.trustedEndpoint(); err != nil {
		return err
	}
	return nil
}

func (config Config) trustedEndpoint() (string, error) {
	host := officialHost(config.Profile)
	basePath := officialPath(config.Profile, config.Protocol)
	if host == "" || basePath == "" {
		return "", fmt.Errorf("subscription plan endpoint profile is incomplete")
	}
	endpoint, err := adaptercore.ValidateEndpoint(config.BaseURL, adaptercore.EndpointPolicy{
		OfficialHosts: []string{host}, OfficialPaths: []string{basePath}, AllowLoopback: true,
	})
	if err != nil {
		return "", fmt.Errorf("subscription plan base URL is outside the official host/path contract: %w", err)
	}
	return endpoint, nil
}

func providerID(kind Kind) modelinvoker.ProviderID {
	switch kind {
	case KimiCode:
		return KimiCodeProvider
	case MiniMaxTokenPlan:
		return MiniMaxTokenProvider
	case MiMoTokenPlan:
		return MiMoTokenProvider
	case AlibabaPlan:
		return AlibabaPlanProvider
	default:
		return ""
	}
}

func profileKind(profile RouteProfile) Kind {
	switch profile {
	case ProfileKimiCodeGlobal:
		return KimiCode
	case ProfileMiniMaxTokenGlobal:
		return MiniMaxTokenPlan
	case ProfileMiMoTokenCN, ProfileMiMoTokenSGP, ProfileMiMoTokenAMS:
		return MiMoTokenPlan
	case ProfileAlibabaCodingCN, ProfileAlibabaCodingIntl, ProfileAlibabaTokenTeamBeijing:
		return AlibabaPlan
	default:
		return ""
	}
}

func officialHost(profile RouteProfile) string {
	switch profile {
	case ProfileKimiCodeGlobal:
		return "api.kimi.com"
	case ProfileMiniMaxTokenGlobal:
		return "api.minimax.io"
	case ProfileMiMoTokenCN:
		return "token-plan-cn.xiaomimimo.com"
	case ProfileMiMoTokenSGP:
		return "token-plan-sgp.xiaomimimo.com"
	case ProfileMiMoTokenAMS:
		return "token-plan-ams.xiaomimimo.com"
	case ProfileAlibabaCodingCN:
		return "coding.dashscope.aliyuncs.com"
	case ProfileAlibabaCodingIntl:
		return "coding-intl.dashscope.aliyuncs.com"
	case ProfileAlibabaTokenTeamBeijing:
		return "token-plan.cn-beijing.maas.aliyuncs.com"
	default:
		return ""
	}
}

func officialPath(profile RouteProfile, protocol modelinvoker.Protocol) string {
	if protocol == modelinvoker.ProtocolMessages {
		switch profile {
		case ProfileKimiCodeGlobal:
			return "/coding"
		case ProfileMiniMaxTokenGlobal, ProfileMiMoTokenCN, ProfileMiMoTokenSGP, ProfileMiMoTokenAMS:
			return "/anthropic"
		case ProfileAlibabaCodingCN, ProfileAlibabaCodingIntl, ProfileAlibabaTokenTeamBeijing:
			return "/apps/anthropic"
		}
	}
	if protocol == modelinvoker.ProtocolChatCompletions {
		switch profile {
		case ProfileKimiCodeGlobal:
			return "/coding/v1"
		case ProfileMiniMaxTokenGlobal, ProfileMiMoTokenCN, ProfileMiMoTokenSGP, ProfileMiMoTokenAMS, ProfileAlibabaCodingCN, ProfileAlibabaCodingIntl:
			return "/v1"
		case ProfileAlibabaTokenTeamBeijing:
			return "/compatible-mode/v1"
		}
	}
	return ""
}

func exactModels(profile RouteProfile) []string {
	switch profile {
	case ProfileKimiCodeGlobal:
		return []string{"kimi-for-coding"}
	case ProfileMiniMaxTokenGlobal:
		return []string{"MiniMax-M3", "MiniMax-M2.7", "MiniMax-M2.7-highspeed"}
	case ProfileMiMoTokenCN, ProfileMiMoTokenSGP, ProfileMiMoTokenAMS:
		return []string{"mimo-v2.5", "mimo-v2.5-pro"}
	case ProfileAlibabaCodingCN, ProfileAlibabaCodingIntl:
		return []string{"qwen3.7-plus", "qwen3.6-plus", "kimi-k2.5", "glm-5", "MiniMax-M2.5", "qwen3.5-plus", "qwen3-max-2026-01-23", "qwen3-coder-next", "qwen3-coder-plus", "glm-4.7"}
	case ProfileAlibabaTokenTeamBeijing:
		return []string{
			"qwen3.7-max", "qwen3.7-plus", "qwen3.6-plus", "qwen3.6-flash",
			"deepseek-v4-pro", "deepseek-v4-flash", "deepseek-v3.2",
			"kimi-k2.7-code", "kimi-k2.6", "kimi-k2.5",
			"glm-5.2", "glm-5.1", "glm-5", "MiniMax-M2.5",
		}
	default:
		return nil
	}
}

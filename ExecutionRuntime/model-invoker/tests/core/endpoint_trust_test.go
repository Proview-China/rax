package core_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/anthropic"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/deepseek"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/gemini"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/kimi"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/mimo"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/minimax"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/openai"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/qwen"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/xai"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/zai"
)

func TestTrustedEndpointPolicyRejectsCredentialAudienceAndPathEscapes(t *testing.T) {
	policy := adaptercore.EndpointPolicy{OfficialHosts: []string{"api.openai.com"}, OfficialPaths: []string{"/v1"}, AllowLoopback: true}
	tests := []struct {
		name, raw, want string
		valid           bool
	}{
		{"official", "https://api.openai.com/v1", "https://api.openai.com/v1", true},
		{"official canonical host and port", "https://API.OPENAI.COM:443/v1/", "https://api.openai.com/v1", true},
		{"IPv4 loopback", "http://127.0.0.2:8123/v1", "http://127.0.0.2:8123/v1", true},
		{"IPv6 loopback", "http://[::1]:8123/v1", "http://[::1]:8123/v1", true},
		{"localhost", "https://LOCALHOST:8443/test", "https://localhost:8443/test", true},
		{"DNS suffix deception", "https://api.openai.com.evil/v1", "", false},
		{"userinfo", "https://api.openai.com@evil.example/v1", "", false},
		{"production port", "https://api.openai.com:444/v1", "", false},
		{"encoded traversal", "https://api.openai.com/v1/%2e%2e/other", "", false},
		{"leading whitespace", " https://api.openai.com/v1", "", false},
		{"trailing whitespace", "https://api.openai.com/v1 ", "", false},
		{"localhost suffix", "http://localhost.evil/v1", "", false},
		{"query", "https://api.openai.com/v1?q=1", "", false},
		{"empty query", "https://api.openai.com/v1?", "", false},
		{"fragment", "https://api.openai.com/v1#x", "", false},
		{"extra path", "https://api.openai.com/v1/other", "", false},
		{"repeated trailing slash", "https://api.openai.com/v1///", "", false},
		{"dot segment", "https://api.openai.com/v1/../other", "", false},
		{"backslash", "https://api.openai.com/v1\\other", "", false},
		{"wrong scheme", "ftp://api.openai.com/v1", "", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := adaptercore.ValidateEndpoint(test.raw, policy)
			if test.valid {
				if err != nil || got != test.want {
					t.Fatalf("ValidateEndpoint(%q) = %q, %v; want %q", test.raw, got, err, test.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateEndpoint(%q) = %q, nil; want rejection", test.raw, got)
			}
		})
	}
}

func TestDirectOfficialEndpointPoliciesUseExactHostAndBasePath(t *testing.T) {
	tests := []struct {
		name, endpoint string
		policy         adaptercore.EndpointPolicy
	}{
		{"OpenAI", "https://api.openai.com/v1", adaptercore.EndpointPolicy{OfficialHosts: []string{"api.openai.com"}, OfficialPaths: []string{"/v1"}}},
		{"Anthropic", "https://api.anthropic.com", adaptercore.EndpointPolicy{OfficialHosts: []string{"api.anthropic.com"}}},
		{"Gemini", "https://generativelanguage.googleapis.com", adaptercore.EndpointPolicy{OfficialHosts: []string{"generativelanguage.googleapis.com"}}},
		{"ZAI", "https://api.z.ai/api/paas/v4", adaptercore.EndpointPolicy{OfficialHosts: []string{"api.z.ai"}, OfficialPaths: []string{"/api/paas/v4"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := adaptercore.ValidateEndpoint(test.endpoint, test.policy); err != nil {
				t.Fatal(err)
			}
			parsed, _ := url.Parse(test.endpoint)
			for _, malicious := range []string{
				"https://" + parsed.Hostname() + ".evil" + parsed.Path,
				"https://" + parsed.Hostname() + ":444" + parsed.Path,
				"https://" + parsed.Hostname() + parsed.Path + "/escape",
			} {
				if _, err := adaptercore.ValidateEndpoint(malicious, test.policy); err == nil {
					t.Fatalf("accepted %q", malicious)
				}
			}
		})
	}
}

func TestTenDirectPublicConfigsRejectRemoteAudienceAndOfficialPathEscape(t *testing.T) {
	tests := []struct {
		name       string
		pathEscape string
		construct  func(string) error
	}{
		{"OpenAI", "https://api.openai.com/v1/escape", func(endpoint string) error {
			_, err := openai.New(openai.Config{APIKey: "test", BaseURL: endpoint})
			return err
		}},
		{"Anthropic", "https://api.anthropic.com/escape", func(endpoint string) error {
			_, err := anthropic.New(anthropic.Config{APIKey: "test", BaseURL: endpoint})
			return err
		}},
		{"Gemini", "https://generativelanguage.googleapis.com/escape", func(endpoint string) error {
			_, err := gemini.New(gemini.Config{APIKey: "test", BaseURL: endpoint})
			return err
		}},
		{"DeepSeek", "https://api.deepseek.com/escape", func(endpoint string) error {
			_, err := deepseek.New(deepseek.Config{APIKey: "test", BaseURL: endpoint})
			return err
		}},
		{"Kimi", "https://api.moonshot.cn/v1/escape", func(endpoint string) error {
			_, err := kimi.New(kimi.Config{APIKey: "test", BaseURL: endpoint})
			return err
		}},
		{"ZAI", "https://api.z.ai/api/paas/v4/escape", func(endpoint string) error {
			_, err := zai.New(zai.Config{APIKey: "test", BaseURL: endpoint})
			return err
		}},
		{"MiniMax", "https://api.minimax.io/escape", func(endpoint string) error {
			_, err := minimax.New(minimax.Config{APIKey: "payg-test", BaseURL: endpoint})
			return err
		}},
		{"MiMo", "https://api.xiaomimimo.com/escape", func(endpoint string) error {
			_, err := mimo.New(mimo.Config{APIKey: "payg-test", BaseURL: endpoint})
			return err
		}},
		{"Qwen", "https://workspace.cn-beijing.maas.aliyuncs.com/compatible-mode/v1/escape", func(endpoint string) error {
			_, err := qwen.New(qwen.Config{APIKey: "sk-test", Region: qwen.RegionChinaBeijing, WorkspaceID: "workspace", BaseURL: endpoint})
			return err
		}},
		{"xAI", "https://api.x.ai/v1/escape", func(endpoint string) error {
			_, err := xai.New(xai.Config{APIKey: "test", BaseURL: endpoint})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, endpoint := range []string{"https://attacker.invalid/v1", test.pathEscape} {
				if err := test.construct(endpoint); err == nil {
					t.Fatalf("public Config accepted unsafe endpoint %q", endpoint)
				}
			}
		})
	}
}

func TestCloudRegionGrammarIsCrossCloudAndCanonical(t *testing.T) {
	for _, valid := range []string{"us-east-1", "us-central1", "eastus", "eastus2", "westeurope"} {
		if !adaptercore.IsCloudRegion(valid) {
			t.Errorf("IsCloudRegion(%q) = false", valid)
		}
	}
	for _, invalid := range []string{"us.east.1", "eastus.", "us--east-1", "EASTUS", " eastus", "eastus ", "eastus@x", "eastus:443", "eastus?x", "eastus#x", `eastus\\x`, ""} {
		if adaptercore.IsCloudRegion(invalid) {
			t.Errorf("IsCloudRegion(%q) = true", invalid)
		}
	}
}

func FuzzTrustedEndpointNeverWidensOfficialAudience(f *testing.F) {
	for _, seed := range []string{"https://api.openai.com/v1", "https://api.openai.com.evil/v1", "http://[::1]:8080/v1", "https://api.openai.com/v1/%2e%2e/x"} {
		f.Add(seed)
	}
	policy := adaptercore.EndpointPolicy{OfficialHosts: []string{"api.openai.com"}, OfficialPaths: []string{"/v1"}, AllowLoopback: true}
	f.Fuzz(func(t *testing.T, raw string) {
		trusted, err := adaptercore.ValidateEndpoint(raw, policy)
		if err != nil {
			return
		}
		parsed, err := url.Parse(trusted)
		if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.RawPath != "" || strings.Contains(parsed.EscapedPath(), "%") {
			t.Fatalf("accepted non-canonical endpoint %q", trusted)
		}
		if adaptercore.IsLoopbackHost(parsed.Hostname()) {
			if parsed.Scheme != "http" && parsed.Scheme != "https" {
				t.Fatalf("accepted loopback scheme %q", parsed.Scheme)
			}
			return
		}
		if parsed.Scheme != "https" || parsed.Hostname() != "api.openai.com" || parsed.Path != "/v1" || parsed.Port() != "" {
			t.Fatalf("accepted audience widening %q", trusted)
		}
	})
}

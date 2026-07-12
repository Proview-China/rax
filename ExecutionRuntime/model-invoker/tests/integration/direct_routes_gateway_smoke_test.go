//go:build integration

package integration_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/routegateway"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

const directRouteGatewaySmokeMarker = "praxis-route-gateway-ok"

func TestDirectRoutesGatewayLiveSmoke(t *testing.T) {
	tests := []struct {
		name, enableEnv, enableValue string
		keyEnv, modelEnv             string
		routeID                      upstream.RouteID
	}{
		{"openai", "PRAXIS_OPENAI_SMOKE", "confirmed", "OPENAI_API_KEY", "OPENAI_SMOKE_MODEL", "openai.direct.payg.responses"},
		{"anthropic", "PRAXIS_ANTHROPIC_SMOKE", "confirmed", "ANTHROPIC_API_KEY", "ANTHROPIC_SMOKE_MODEL", "anthropic.direct.payg.messages"},
		{"gemini", "PRAXIS_GEMINI_SMOKE", "confirmed", "GEMINI_API_KEY", "GEMINI_SMOKE_MODEL", "google.gemini-developer.payg.generate_content"},
		{"xai", "PRAXIS_XAI_LIVE_TESTS", "1", "XAI_API_KEY", "XAI_SMOKE_MODEL", "xai.api.global.payg.responses"},
		{"zai", "PRAXIS_ZAI_LIVE_TESTS", "1", "ZAI_API_KEY", "ZAI_SMOKE_MODEL", "zai.platform.global.payg.chat_completions"},
		{"deepseek", "PRAXIS_DEEPSEEK_LIVE_TESTS", "1", "DEEPSEEK_API_KEY", "DEEPSEEK_SMOKE_MODEL", "deepseek.direct.payg.chat_completions"},
		{"kimi", "PRAXIS_KIMI_LIVE_TESTS", "1", "MOONSHOT_API_KEY", "KIMI_SMOKE_MODEL", "kimi.platform.global.payg.chat_completions"},
		{"minimax", "PRAXIS_MINIMAX_LIVE_TESTS", "1", "MINIMAX_API_KEY", "MINIMAX_SMOKE_MODEL", "minimax.platform.global.payg.messages"},
		{"mimo", "PRAXIS_MIMO_LIVE_TESTS", "1", "MIMO_API_KEY", "MIMO_SMOKE_MODEL", "xiaomi.mimo.global.payg.messages"},
		{"qwen", "PRAXIS_QWEN_LIVE_TESTS", "1", "DASHSCOPE_API_KEY", "QWEN_SMOKE_MODEL", ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if os.Getenv("PRAXIS_LIVE_TESTS") != "1" || os.Getenv(test.enableEnv) != test.enableValue {
				t.Skip("direct Route Gateway smoke requires global and provider confirmations")
			}
			key, model := os.Getenv(test.keyEnv), os.Getenv(test.modelEnv)
			if key == "" || model == "" {
				t.Fatalf("enabled direct Route Gateway smoke requires %s and %s", test.keyEnv, test.modelEnv)
			}
			routeID, workspace := test.routeID, ""
			if test.name == "qwen" {
				workspace = os.Getenv("QWEN_SMOKE_WORKSPACE_ID")
				switch os.Getenv("QWEN_SMOKE_REGION") {
				case "cn-beijing":
					routeID = "alibaba.model-studio.cn-beijing.payg.responses"
				case "ap-southeast-1":
					routeID = "alibaba.model-studio.ap-southeast-1.payg.responses"
				default:
					t.Fatal("enabled Qwen Gateway smoke requires QWEN_SMOKE_REGION=cn-beijing or ap-southeast-1")
				}
				if workspace == "" {
					t.Fatal("enabled Qwen Gateway smoke requires QWEN_SMOKE_WORKSPACE_ID")
				}
			}
			runDirectRouteGatewaySmoke(t, routeID, model, key, workspace)
		})
	}
}

func runDirectRouteGatewaySmoke(t *testing.T, routeID upstream.RouteID, model, key, workspace string) {
	t.Helper()
	now := time.Now().UTC()
	base, err := catalog.NewDefault(now)
	if err != nil {
		t.Fatalf("construct fresh default catalog: %v", err)
	}
	entry, ok := base.Get(routeID)
	if !ok {
		t.Fatalf("RouteID %q is not present in the default catalog", routeID)
	}
	factories, err := routegateway.NewBuiltinFactoryRegistry()
	if err != nil {
		t.Fatal(err)
	}
	binding := directSmokeBindingResolver{workspace: workspace}
	gateway, report, err := routegateway.NewHost(routegateway.HostConfig{
		BaseCatalog: base, BindingResolver: binding,
		SecretResolver: liveSmokeSecretResolver{now: now, key: key, routeID: entry.ID, identity: entry.Route.Identity(), profile: entry.Route.Credential}, Factories: factories,
		Clock: func() time.Time { return now },
	})
	if err != nil || gateway == nil || !report.Ready {
		t.Fatalf("construct direct Route Gateway: report=%#v err=%v", report, err)
	}
	defer func() {
		if closeErr := gateway.Close(); closeErr != nil {
			t.Errorf("Gateway.Close() error = %v", closeErr)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	result, err := gateway.Invoke(ctx, modelinvoker.RouteCall{
		RouteID: routeID,
		Invocation: upstream.InvocationContext{
			Usage: upstream.InvocationGeneralAPI, Subject: upstream.SubjectService,
			Tenancy: upstream.TenancyMulti, Execution: upstream.ExecutionForeground,
		},
		Request: modelinvoker.Request{
			Model:  model,
			Input:  []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "Reply with exactly: "+directRouteGatewaySmokeMarker)},
			Budget: modelinvoker.Budget{MaxOutputTokens: 32},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasExactDirectRouteGatewaySmokeMarker(result.Response.Text()) || result.Resolution.Route.RouteID != routeID {
		t.Fatalf("direct Route Gateway returned an invalid result for route %q", result.Resolution.Route.RouteID)
	}
}

func hasExactDirectRouteGatewaySmokeMarker(text string) bool {
	return strings.TrimSpace(text) == directRouteGatewaySmokeMarker
}

func TestDirectRouteGatewaySmokeMarkerIsExact(t *testing.T) {
	for _, value := range []string{"not-empty", "prefix " + directRouteGatewaySmokeMarker, directRouteGatewaySmokeMarker + " suffix", strings.ToUpper(directRouteGatewaySmokeMarker)} {
		if hasExactDirectRouteGatewaySmokeMarker(value) {
			t.Fatalf("non-exact marker %q was accepted", value)
		}
	}
	if !hasExactDirectRouteGatewaySmokeMarker(" \n" + directRouteGatewaySmokeMarker + "\t") {
		t.Fatal("exact marker with transport whitespace was rejected")
	}
}

type directSmokeBindingResolver struct {
	workspace string
}

func (resolver directSmokeBindingResolver) ResolveBinding(ctx context.Context, request routegateway.BindingRequest) (routegateway.RuntimeBinding, error) {
	binding, err := (routegateway.CatalogBindingResolver{}).ResolveBinding(ctx, request)
	if err != nil {
		return routegateway.RuntimeBinding{}, err
	}
	if resolver.workspace != "" {
		binding.Workspace = resolver.workspace
		binding.Version = "live-workspace-v1"
	}
	return binding, nil
}

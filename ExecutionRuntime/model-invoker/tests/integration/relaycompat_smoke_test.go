//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/relaycompat"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/routegateway"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

type relaySmokeRoute struct {
	name       string
	model      string
	protocol   modelinvoker.Protocol
	apiVersion string
	keyEnv     string
}

func TestThirdPartyRelayTextAndToolCalls(t *testing.T) {
	if os.Getenv("PRAXIS_LIVE_TESTS") != "1" || os.Getenv("PRAXIS_RELAY_SMOKE") != "confirmed" {
		t.Skip("third-party relay live smoke requires global and relay confirmations")
	}
	baseURL := strings.TrimRight(os.Getenv("PRAXIS_RELAY_BASE_URL"), "/")
	if baseURL == "" {
		t.Fatal("PRAXIS_RELAY_BASE_URL is required")
	}
	routes := []relaySmokeRoute{
		{name: "gemini-native", model: "gemini-3.5-flash", protocol: modelinvoker.ProtocolGenerateContent, apiVersion: "v1beta", keyEnv: "PRAXIS_RELAY_GEMINI_NATIVE_KEY"},
		{name: "gemini-chat", model: "gemini-3.5-flash", protocol: modelinvoker.ProtocolChatCompletions, keyEnv: "PRAXIS_RELAY_GEMINI_CHAT_KEY"},
		{name: "grok-chat", model: "grok-4.5", protocol: modelinvoker.ProtocolChatCompletions, keyEnv: "PRAXIS_RELAY_GROK_KEY"},
		{name: "grok-messages", model: "grok-4.5", protocol: modelinvoker.ProtocolMessages, keyEnv: "PRAXIS_RELAY_GROK_KEY"},
		{name: "gpt-chat", model: "gpt-5.6-luna", protocol: modelinvoker.ProtocolChatCompletions, keyEnv: "PRAXIS_RELAY_GPT_KEY"},
		{name: "gpt-responses", model: "gpt-5.6-luna", protocol: modelinvoker.ProtocolResponses, keyEnv: "PRAXIS_RELAY_GPT_KEY"},
		{name: "claude-messages", model: "claude-sonnet-5", protocol: modelinvoker.ProtocolMessages, keyEnv: "PRAXIS_RELAY_CLAUDE_MESSAGES_KEY"},
		{name: "claude-chat", model: "claude-sonnet-5", protocol: modelinvoker.ProtocolChatCompletions, keyEnv: "PRAXIS_RELAY_CLAUDE_CHAT_KEY"},
	}

	var total modelinvoker.Usage
	for _, route := range routes {
		route := route
		t.Run(route.name, func(t *testing.T) {
			key := os.Getenv(route.keyEnv)
			if key == "" {
				t.Fatalf("%s is required", route.keyEnv)
			}
			invoker, closer, endpoint := relaySmokeInvoker(t, baseURL, key, route)
			defer func() {
				if err := closer.Close(); err != nil {
					t.Errorf("close relay adapter: %v", err)
				}
			}()

			textRequest := relaySmokeRequest(route, endpoint, 16)
			textRequest.Input = []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "Reply with exactly: PRAXIS_OK")}
			text, err := invokeRelaySmoke(context.Background(), invoker, textRequest)
			if err != nil {
				t.Fatalf("text invocation: %s", relayFailureSummary(err, text))
			}
			if strings.TrimSpace(text.Text()) != "PRAXIS_OK" || text.Model != route.model || text.Protocol != route.protocol || text.Provider != relaycompat.ProviderID {
				t.Fatalf("text normalization: model=%q protocol=%q provider=%q text=%q", text.Model, text.Protocol, text.Provider, text.Text())
			}

			toolRequest := relaySmokeRequest(route, endpoint, 64)
			toolRequest.Input = []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "Call lookup once with code PRAXIS_TOOL. Do not answer in text.")}
			toolRequest.Tools = []modelinvoker.Tool{{
				Name: "lookup", Description: "Return a value for a code",
				Parameters: json.RawMessage(`{"type":"object","properties":{"code":{"type":"string"}},"required":["code"],"additionalProperties":false}`),
			}}
			toolRequest.ToolChoice = modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceRequired}
			tool, err := invokeRelaySmoke(context.Background(), invoker, toolRequest)
			if err != nil {
				t.Fatalf("tool invocation: %s", relayFailureSummary(err, tool))
			}
			calls := tool.FunctionCalls()
			if len(calls) != 1 || calls[0].Name != "lookup" || !json.Valid(calls[0].Arguments) {
				t.Fatalf("tool normalization: %#v", calls)
			}
			total.InputTokens += text.Usage.InputTokens + tool.Usage.InputTokens
			total.OutputTokens += text.Usage.OutputTokens + tool.Usage.OutputTokens
			total.ReasoningTokens += text.Usage.ReasoningTokens + tool.Usage.ReasoningTokens
			total.CacheReadTokens += text.Usage.CacheReadTokens + tool.Usage.CacheReadTokens
			total.CacheWriteTokens += text.Usage.CacheWriteTokens + tool.Usage.CacheWriteTokens
			total.TotalTokens += text.Usage.TotalTokens + tool.Usage.TotalTokens
			t.Logf("route=%s text_usage=%+v tool_usage=%+v tool=%s args=%s", route.name, text.Usage, tool.Usage, calls[0].Name, calls[0].Arguments)
		})
	}
	t.Logf("relay cumulative usage=%+v nominal_maximum_output_budget_tokens=640 nominal_requests=16 transient_retries_max=2_per_failed_call", total)
}

func invokeRelaySmoke(ctx context.Context, invoker *modelinvoker.Invoker, request modelinvoker.Request) (modelinvoker.Response, error) {
	var response modelinvoker.Response
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		response, err = invoker.Invoke(ctx, request)
		if err == nil || (modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorRateLimit && modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorProviderUnavailable) {
			return response, err
		}
		if attempt < 2 {
			select {
			case <-ctx.Done():
				return response, ctx.Err()
			case <-time.After(time.Duration(1<<attempt) * time.Second):
			}
		}
	}
	return response, err
}

func relayFailureSummary(err error, response modelinvoker.Response) string {
	summary := err.Error()
	var invocationError *modelinvoker.Error
	if errors.As(err, &invocationError) && invocationError != nil {
		summary += fmt.Sprintf(" kind=%s status=%d code=%q request_id=%q", invocationError.Kind, invocationError.HTTPStatus, invocationError.Code, invocationError.RequestID)
	}
	if raw := response.RawResponse.Bytes(); len(raw) != 0 {
		if len(raw) > 1024 {
			raw = raw[:1024]
		}
		summary += " raw=" + string(raw)
	}
	return summary
}

func relaySmokeInvoker(t *testing.T, root, key string, route relaySmokeRoute) (*modelinvoker.Invoker, interface{ Close() error }, string) {
	t.Helper()
	endpoint := root
	upstreamProtocol := upstream.ProtocolID(route.protocol)
	if route.protocol == modelinvoker.ProtocolChatCompletions || route.protocol == modelinvoker.ProtocolResponses {
		endpoint += "/v1"
	}
	if route.protocol == modelinvoker.ProtocolGenerateContent {
		endpoint += "/" + route.apiVersion
	}
	routeID := upstream.RouteID("live.relay." + route.name)
	entry := catalog.Entry{
		ID: routeID,
		Route: upstream.UpstreamRoute{
			ID: routeID, Model: upstream.ModelIdentity{CanonicalFamily: route.name, ProviderModelRef: route.model},
			Protocol: upstream.ProtocolBinding{ID: upstreamProtocol, APIVersion: route.apiVersion},
		},
		Implementation: catalog.Implementation{AdapterID: string(relaycompat.ProviderID)},
	}
	profileID := upstream.CredentialProfileID("live.relay." + route.name + ".key")
	secret, err := routegateway.NewSecretMaterial(profileID, upstream.CredentialAPIKey, "live-v1", time.Now().Add(5*time.Minute), map[upstream.CredentialPurpose][]byte{
		upstream.CredentialPurposeAPIKey: []byte(key),
	})
	if err != nil {
		t.Fatalf("create secret material: %v", err)
	}
	result, err := routegateway.NewRelayCompatFactory().Build(context.Background(), routegateway.FactoryInput{
		Entry: entry, Binding: routegateway.RuntimeBinding{RouteID: routeID}, Endpoint: endpoint, Secret: secret,
	})
	if err != nil {
		t.Fatalf("build relay factory: %v", err)
	}
	if result.Provider == nil || result.Closer == nil || result.Endpoint != endpoint {
		if result.Closer != nil {
			_ = result.Closer.Close()
		}
		t.Fatalf("incomplete relay factory result: provider=%t closer=%t endpoint=%q want=%q", result.Provider != nil, result.Closer != nil, result.Endpoint, endpoint)
	}
	registry, err := modelinvoker.NewRegistry(result.Provider)
	if err != nil {
		_ = result.Closer.Close()
		t.Fatalf("create relay registry: %v", err)
	}
	invoker, err := modelinvoker.NewInvoker(registry)
	if err != nil {
		_ = result.Closer.Close()
		t.Fatalf("create relay invoker: %v", err)
	}
	return invoker, result.Closer, endpoint
}

func relaySmokeRequest(route relaySmokeRoute, endpoint string, maxOutput int64) modelinvoker.Request {
	return modelinvoker.Request{
		Provider: relaycompat.ProviderID, Protocol: route.protocol, Endpoint: endpoint, Model: route.model,
		Budget: modelinvoker.Budget{MaxOutputTokens: maxOutput, Timeout: 90 * time.Second},
	}
}

package plancompat_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/plancompat"
)

func TestAllApprovedPlanKindsExecuteChatAndMessagesWithAttestedUserAgent(t *testing.T) {
	tests := []struct {
		kind       plancompat.Kind
		profile    plancompat.RouteProfile
		provider   modelinvoker.ProviderID
		key, model string
	}{
		{plancompat.KimiCode, plancompat.ProfileKimiCodeGlobal, plancompat.KimiCodeProvider, "kimi-plan-key", "kimi-for-coding"},
		{plancompat.MiniMaxTokenPlan, plancompat.ProfileMiniMaxTokenGlobal, plancompat.MiniMaxTokenProvider, "sk-cp-offline", "MiniMax-M2.7"},
		{plancompat.MiMoTokenPlan, plancompat.ProfileMiMoTokenCN, plancompat.MiMoTokenProvider, "tp-offline", "mimo-v2.5"},
		{plancompat.AlibabaPlan, plancompat.ProfileAlibabaTokenTeamBeijing, plancompat.AlibabaPlanProvider, "sk-sp-offline", "qwen3.7-max"},
	}
	for _, test := range tests {
		for _, protocol := range []modelinvoker.Protocol{modelinvoker.ProtocolChatCompletions, modelinvoker.ProtocolMessages} {
			t.Run(string(test.kind)+"/"+string(protocol), func(t *testing.T) {
				server := planServer(t, test.kind, protocol, test.key, test.model, "praxis-cli/v1.0.0")
				defer server.Close()
				adapter, err := plancompat.New(plancompat.Config{Kind: test.kind, Profile: test.profile, APIKey: test.key, BaseURL: server.URL, Protocol: protocol, UserAgent: "praxis-cli/v1.0.0", HTTPClient: server.Client()})
				if err != nil {
					t.Fatal(err)
				}
				registry, err := modelinvoker.NewRegistry(adapter)
				if err != nil {
					t.Fatal(err)
				}
				invoker, err := modelinvoker.NewInvoker(registry)
				if err != nil {
					t.Fatal(err)
				}
				response, err := invoker.Invoke(context.Background(), modelinvoker.Request{
					Provider: test.provider, Protocol: protocol, Endpoint: server.URL, Model: test.model,
					Input:  []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")},
					Tools:  []modelinvoker.Tool{{Name: "lookup", Description: "lookup", Parameters: json.RawMessage(`{"type":"object","properties":{}}`)}},
					Budget: modelinvoker.Budget{MaxOutputTokens: 256},
				})
				if err != nil {
					t.Fatal(err)
				}
				if response.Text() != "ok" || response.Provider != test.provider || response.Protocol != protocol {
					t.Fatalf("response = %#v", response)
				}
			})
		}
	}
}

func TestPlanConfigRejectsWrongKeyIdentityAndProductionHost(t *testing.T) {
	tests := []plancompat.Config{
		{Kind: plancompat.MiniMaxTokenPlan, Profile: plancompat.ProfileMiniMaxTokenGlobal, APIKey: "payg-key", BaseURL: "https://api.minimax.io/v1", Protocol: modelinvoker.ProtocolChatCompletions, UserAgent: "praxis/v1"},
		{Kind: plancompat.MiMoTokenPlan, Profile: plancompat.ProfileMiMoTokenCN, APIKey: "tp-key", BaseURL: "https://token-plan-cn.xiaomimimo.com/v1", Protocol: modelinvoker.ProtocolChatCompletions},
		{Kind: plancompat.AlibabaPlan, Profile: plancompat.ProfileAlibabaCodingCN, APIKey: "sk-sp-key", BaseURL: "https://attacker.invalid/v1", Protocol: modelinvoker.ProtocolChatCompletions, UserAgent: "praxis/v1"},
	}
	for index, config := range tests {
		if _, err := plancompat.New(config); err == nil {
			t.Errorf("config %d error = nil", index)
		}
	}
}

func TestApprovedPlanChatAndMessagesStreams(t *testing.T) {
	tests := []struct {
		kind       plancompat.Kind
		profile    plancompat.RouteProfile
		provider   modelinvoker.ProviderID
		protocol   modelinvoker.Protocol
		key, model string
	}{
		{plancompat.KimiCode, plancompat.ProfileKimiCodeGlobal, plancompat.KimiCodeProvider, modelinvoker.ProtocolChatCompletions, "kimi-plan-key", "kimi-for-coding"},
		{plancompat.AlibabaPlan, plancompat.ProfileAlibabaTokenTeamBeijing, plancompat.AlibabaPlanProvider, modelinvoker.ProtocolMessages, "sk-sp-offline", "qwen3.7-max"},
	}
	for _, test := range tests {
		t.Run(string(test.protocol), func(t *testing.T) {
			server := planStreamServer(t, test.protocol, test.key, test.model, "praxis-cli/v1.0.0")
			defer server.Close()
			adapter, err := plancompat.New(plancompat.Config{Kind: test.kind, Profile: test.profile, APIKey: test.key, BaseURL: server.URL, Protocol: test.protocol, UserAgent: "praxis-cli/v1.0.0", HTTPClient: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			registry, _ := modelinvoker.NewRegistry(adapter)
			invoker, _ := modelinvoker.NewInvoker(registry)
			stream, err := invoker.Stream(context.Background(), modelinvoker.Request{Provider: test.provider, Protocol: test.protocol, Endpoint: server.URL, Model: test.model, Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}, Budget: modelinvoker.Budget{MaxOutputTokens: 128}})
			if err != nil {
				t.Fatal(err)
			}
			var text strings.Builder
			for stream.Next() {
				text.WriteString(stream.Event().TextDelta)
			}
			if err := stream.Err(); err != nil {
				t.Fatal(err)
			}
			if err := stream.Close(); err != nil {
				t.Fatal(err)
			}
			if text.String() != "ok" {
				t.Fatalf("stream text = %q", text.String())
			}
		})
	}
}

func TestAlibabaCodingAndTokenPlanModelSetsRemainIndependentAndExact(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	coding, err := plancompat.New(plancompat.Config{
		Kind: plancompat.AlibabaPlan, Profile: plancompat.ProfileAlibabaCodingCN, APIKey: "sk-sp-coding", BaseURL: server.URL,
		Protocol: modelinvoker.ProtocolChatCompletions, UserAgent: "praxis-cli/v1.0.0",
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	token, err := plancompat.New(plancompat.Config{
		Kind: plancompat.AlibabaPlan, Profile: plancompat.ProfileAlibabaTokenTeamBeijing, APIKey: "sk-sp-token", BaseURL: server.URL,
		Protocol: modelinvoker.ProtocolChatCompletions, UserAgent: "praxis-cli/v1.0.0",
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	query := modelinvoker.CapabilityQuery{Protocol: modelinvoker.ProtocolChatCompletions, Model: "glm-5"}
	if _, err := coding.Capabilities(context.Background(), query); err != nil {
		t.Fatalf("Coding Plan exact glm-5 rejected: %v", err)
	}
	query.Model = "glm-5.1"
	if _, err := coding.Capabilities(context.Background(), query); err == nil {
		t.Fatal("Coding Plan accepted unsupported glm-5.1")
	}
	if _, err := token.Capabilities(context.Background(), query); err != nil {
		t.Fatalf("Token Plan Team exact glm-5.1 rejected: %v", err)
	}
	query.Model = "GLM-5.1"
	if _, err := token.Capabilities(context.Background(), query); err == nil {
		t.Fatal("Token Plan Team accepted wrong-case GLM-5.1")
	}
}

func planServer(t *testing.T, kind plancompat.Kind, protocol modelinvoker.Protocol, key, model, userAgent string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.UserAgent() != userAgent {
			t.Errorf("User-Agent = %q, want %q", request.UserAgent(), userAgent)
		}
		if protocol == modelinvoker.ProtocolMessages && (kind == plancompat.KimiCode || kind == plancompat.MiniMaxTokenPlan) {
			if request.Header.Get("x-api-key") != key {
				t.Errorf("x-api-key was not preserved for %s Messages", kind)
			}
		} else if request.Header.Get("Authorization") != "Bearer "+key {
			t.Errorf("Authorization header does not contain the selected plan key")
		}
		if request.Method != http.MethodPost {
			t.Errorf("method = %s", request.Method)
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("x-request-id", "plan-request")
		switch protocol {
		case modelinvoker.ProtocolChatCompletions:
			if !strings.HasSuffix(request.URL.Path, "/chat/completions") {
				t.Errorf("chat path = %q", request.URL.Path)
			}
			_, _ = fmt.Fprintf(writer, `{"id":"chat-plan","model":%q,"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`, model)
		case modelinvoker.ProtocolMessages:
			if !strings.HasSuffix(request.URL.Path, "/messages") {
				t.Errorf("messages path = %q", request.URL.Path)
			}
			_, _ = fmt.Fprintf(writer, `{"id":"msg-plan","type":"message","role":"assistant","model":%q,"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`, model)
		default:
			t.Errorf("unexpected protocol %q", protocol)
		}
	}))
}

func planStreamServer(t *testing.T, protocol modelinvoker.Protocol, key, model, userAgent string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.UserAgent() != userAgent || request.Header.Get("Authorization") != "Bearer "+key {
			t.Errorf("stream identity/auth headers are incorrect")
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := writer.(http.Flusher)
		if protocol == modelinvoker.ProtocolChatCompletions {
			_, _ = fmt.Fprintf(writer, "data: {\"id\":\"chat-stream\",\"object\":\"chat.completion.chunk\",\"model\":%q,\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":null}]}\n\n", model)
			_, _ = fmt.Fprintf(writer, "data: {\"id\":\"chat-stream\",\"object\":\"chat.completion.chunk\",\"model\":%q,\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", model)
		} else {
			_, _ = fmt.Fprintf(writer, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg-stream\",\"type\":\"message\",\"role\":\"assistant\",\"model\":%q,\"content\":[],\"stop_reason\":null,\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n", model)
			_, _ = io.WriteString(writer, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}\n\nevent: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":1}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		}
		if flusher != nil {
			flusher.Flush()
		}
	}))
}

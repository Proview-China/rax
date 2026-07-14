package localcompat_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/localcompat"
)

func TestAllLocalProductsAndProtocolsExecuteAgainstPinnedFakeServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/chat/completions":
			_, _ = fmt.Fprint(w, `{"id":"chat","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"chat-ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
		case "/v1/responses":
			_, _ = fmt.Fprint(w, `{"id":"resp","object":"response","model":"m","status":"completed","output":[{"id":"msg","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"responses-ok","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	products := []struct {
		product localcompat.Product
		id      modelinvoker.ProviderID
	}{
		{localcompat.ProductGeneric, localcompat.ProviderGeneric},
		{localcompat.ProductOllama, localcompat.ProviderOllama},
		{localcompat.ProductLlamaCPP, localcompat.ProviderLlamaCPP},
	}
	protocols := []modelinvoker.Protocol{modelinvoker.ProtocolChatCompletions, modelinvoker.ProtocolResponses}
	for _, product := range products {
		for _, protocolID := range protocols {
			name := string(product.product) + "/" + string(protocolID)
			t.Run(name, func(t *testing.T) {
				adapter, err := localcompat.New(localcompat.Config{
					Product: product.product, Trust: localcompat.TrustLocal, BaseURL: server.URL + "/v1", Protocol: protocolID,
					AllowedModels: []string{"m"}, SupportedCapabilities: []modelinvoker.Capability{modelinvoker.CapabilityTextGeneration, modelinvoker.CapabilityStreaming},
					HTTPClient: server.Client(),
				})
				if err != nil {
					t.Fatal(err)
				}
				if adapter.ID() != product.id || adapter.DefaultProtocol() != protocolID {
					t.Fatalf("identity drift: %s %s", adapter.ID(), adapter.DefaultProtocol())
				}
				if endpoint, ok := adapter.CandidateBindingEndpoint(protocolID, ""); !ok || endpoint != server.URL+"/v1" {
					t.Fatalf("candidate endpoint mismatch: %q %v", endpoint, ok)
				}
				contract, err := adapter.Capabilities(context.Background(), modelinvoker.CapabilityQuery{Protocol: protocolID, Model: "m"})
				if err != nil || contract[modelinvoker.CapabilityTextGeneration].Level != modelinvoker.SupportCompatible {
					t.Fatalf("capability contract mismatch: %+v %v", contract, err)
				}
				request := modelinvoker.Request{
					Provider: product.id, Protocol: protocolID, Endpoint: server.URL + "/v1", Model: "m",
					Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}, Budget: modelinvoker.Budget{MaxOutputTokens: 8},
				}
				response, err := adapter.Invoke(context.Background(), request)
				if err != nil {
					t.Fatal(err)
				}
				want := "chat-ok"
				if protocolID == modelinvoker.ProtocolResponses {
					want = "responses-ok"
				}
				if response.Text() != want || response.Provider != product.id || response.Protocol != protocolID || response.Model != "m" {
					t.Fatalf("normalized response drift: %+v", response)
				}
			})
		}
	}
}

func TestLocalCompatibilityValidationAndNilAdapterMatrix(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	valid := localcompat.Config{
		Product: localcompat.ProductGeneric, Trust: localcompat.TrustLocal, BaseURL: server.URL + "/v1",
		Protocol: modelinvoker.ProtocolChatCompletions, AllowedModels: []string{"m"},
		SupportedCapabilities: []modelinvoker.Capability{modelinvoker.CapabilityTextGeneration}, HTTPClient: server.Client(),
	}
	mutate := func(change func(*localcompat.Config)) localcompat.Config {
		copy := valid
		copy.AllowedModels = append([]string(nil), valid.AllowedModels...)
		copy.SupportedCapabilities = append([]modelinvoker.Capability(nil), valid.SupportedCapabilities...)
		change(&copy)
		return copy
	}
	cases := []localcompat.Config{
		mutate(func(c *localcompat.Config) { c.Product = localcompat.Product("bad") }),
		mutate(func(c *localcompat.Config) { c.APIKey = " bad " }),
		mutate(func(c *localcompat.Config) { c.BaseURL = "http://user:pass@127.0.0.1/v1" }),
		mutate(func(c *localcompat.Config) { c.Trust = localcompat.TrustMode("bad") }),
		mutate(func(c *localcompat.Config) { c.AllowedModels = nil }),
		mutate(func(c *localcompat.Config) { c.AllowedModels = []string{"m", "m"} }),
		mutate(func(c *localcompat.Config) { c.AllowedModels = []string{"bad\nmodel"} }),
		mutate(func(c *localcompat.Config) {
			c.SupportedCapabilities = []modelinvoker.Capability{modelinvoker.CapabilityStreaming}
		}),
		mutate(func(c *localcompat.Config) {
			c.SupportedCapabilities = []modelinvoker.Capability{modelinvoker.CapabilityTextGeneration, modelinvoker.CapabilityTextGeneration}
		}),
		mutate(func(c *localcompat.Config) {
			c.SupportedCapabilities = []modelinvoker.Capability{modelinvoker.CapabilityTextGeneration, ""}
		}),
		mutate(func(c *localcompat.Config) { c.UserAgent = "bad\nagent" }),
	}
	for index, config := range cases {
		if _, err := localcompat.New(config); err == nil {
			t.Fatalf("invalid local config %d accepted", index)
		}
	}

	const secret = "local-format-secret"
	formatted := fmt.Sprintf("%v %#v", localcompat.Config{APIKey: secret}, localcompat.Config{APIKey: secret})
	if strings.Contains(formatted, secret) {
		t.Fatal("local config formatting leaked credential")
	}
	var adapter *localcompat.Adapter
	if adapter.ID() != "" || adapter.DefaultProtocol() != modelinvoker.ProtocolAuto {
		t.Fatal("nil adapter accessors are not safe")
	}
	if endpoint, ok := adapter.CandidateBindingEndpoint(modelinvoker.ProtocolResponses, ""); ok || endpoint != "" {
		t.Fatal("nil adapter returned candidate endpoint")
	}
	if _, err := adapter.Capabilities(context.Background(), modelinvoker.CapabilityQuery{}); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorProviderUnavailable {
		t.Fatalf("nil capabilities error = %v", err)
	}
	if _, err := adapter.Invoke(context.Background(), modelinvoker.Request{}); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorProviderUnavailable {
		t.Fatalf("nil invoke error = %v", err)
	}
	if _, err := adapter.Stream(context.Background(), modelinvoker.Request{}); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorProviderUnavailable {
		t.Fatalf("nil stream error = %v", err)
	}
}

func TestLocalBindingRejectsRequestIdentityModelOptionsAndCapabilityQuery(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	adapter, err := localcompat.New(localcompat.Config{
		Product: localcompat.ProductOllama, Trust: localcompat.TrustLocal, BaseURL: server.URL + "/v1",
		Protocol: modelinvoker.ProtocolChatCompletions, AllowedModels: []string{"m"},
		SupportedCapabilities: []modelinvoker.Capability{modelinvoker.CapabilityTextGeneration}, HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	base := modelinvoker.Request{Provider: localcompat.ProviderOllama, Protocol: modelinvoker.ProtocolChatCompletions, Endpoint: server.URL + "/v1", Model: "m", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "x")}, Budget: modelinvoker.Budget{MaxOutputTokens: 1}}
	cases := []modelinvoker.Request{
		func() modelinvoker.Request { r := base; r.Provider = localcompat.ProviderGeneric; return r }(),
		func() modelinvoker.Request { r := base; r.Protocol = modelinvoker.ProtocolResponses; return r }(),
		func() modelinvoker.Request { r := base; r.Model = "other"; return r }(),
		func() modelinvoker.Request {
			r := base
			r.ProviderOptions = modelinvoker.ProviderOptions{localcompat.ProviderOllama: []byte(`{}`)}
			return r
		}(),
	}
	for index, request := range cases {
		if _, err := adapter.Invoke(context.Background(), request); err == nil {
			t.Fatalf("identity drift request %d reached local transport", index)
		}
	}
	if _, err := adapter.Capabilities(context.Background(), modelinvoker.CapabilityQuery{Protocol: modelinvoker.ProtocolResponses, Model: "m"}); err == nil {
		t.Fatal("wrong protocol capability query succeeded")
	}
	if _, err := adapter.Capabilities(context.Background(), modelinvoker.CapabilityQuery{Protocol: modelinvoker.ProtocolChatCompletions, Model: "other"}); err == nil {
		t.Fatal("wrong model capability query succeeded")
	}
}

func TestLocalCompatibleChatAndResponsesStreaming(t *testing.T) {
	protocols := []modelinvoker.Protocol{modelinvoker.ProtocolChatCompletions, modelinvoker.ProtocolResponses}
	for _, protocolID := range protocols {
		t.Run(string(protocolID), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				flusher := w.(http.Flusher)
				if protocolID == modelinvoker.ProtocolChatCompletions {
					_, _ = io.WriteString(w, "data: {\"id\":\"chat\",\"object\":\"chat.completion.chunk\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"stream-ok\"},\"finish_reason\":null}]}\n\n")
					_, _ = io.WriteString(w, "data: {\"id\":\"chat\",\"object\":\"chat.completion.chunk\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
				} else {
					for _, event := range []string{
						`{"type":"response.created","sequence_number":1,"response":{"id":"resp","model":"m","status":"in_progress","output":[]}}`,
						`{"type":"response.output_text.delta","sequence_number":2,"item_id":"msg","output_index":0,"content_index":0,"delta":"streamed"}`,
						`{"type":"response.completed","sequence_number":3,"response":{"id":"resp","model":"m","status":"completed","output":[{"id":"msg","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"streamed","annotations":[]}]}],"usage":{"input_tokens":5,"input_tokens_details":{"cached_tokens":3},"output_tokens":2,"output_tokens_details":{"reasoning_tokens":1},"total_tokens":7}}}`,
					} {
						_, _ = fmt.Fprintf(w, "data: %s\n\n", event)
					}
					_, _ = io.WriteString(w, "data: [DONE]\n\n")
				}
				flusher.Flush()
			}))
			defer server.Close()
			adapter, err := localcompat.New(localcompat.Config{
				Product: localcompat.ProductGeneric, Trust: localcompat.TrustLocal, BaseURL: server.URL + "/v1", Protocol: protocolID,
				AllowedModels: []string{"m"}, SupportedCapabilities: []modelinvoker.Capability{modelinvoker.CapabilityTextGeneration, modelinvoker.CapabilityStreaming}, HTTPClient: server.Client(),
			})
			if err != nil {
				t.Fatal(err)
			}
			stream, err := adapter.Stream(context.Background(), modelinvoker.Request{
				Provider: localcompat.ProviderGeneric, Protocol: protocolID, Endpoint: server.URL + "/v1", Model: "m", Stream: true,
				Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "x")}, Budget: modelinvoker.Budget{MaxOutputTokens: 8},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			var text string
			var terminal *modelinvoker.Response
			for stream.Next() {
				text += stream.Event().TextDelta
				if stream.Event().Response != nil {
					terminal = stream.Event().Response
				}
			}
			wantText := "stream-ok"
			if protocolID == modelinvoker.ProtocolResponses {
				wantText = "streamed"
			}
			if err := stream.Err(); err != nil || text != wantText || terminal == nil || terminal.Model != "m" {
				var typed *modelinvoker.Error
				_ = errors.As(err, &typed)
				if typed != nil {
					t.Fatalf("text=%q terminal=%+v kind=%s code=%s message=%s", text, terminal, typed.Kind, typed.Code, typed.Message)
				}
				t.Fatalf("text=%q terminal=%+v err=%v", text, terminal, err)
			}
		})
	}
}

func TestLocalCompatibleAdapterAttestsExactResponseModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), `"stream":true`) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"id\":\"chat\",\"object\":\"chat.completion.chunk\",\"model\":\"other\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			return
		}
		_, _ = io.WriteString(w, `{"id":"chat","object":"chat.completion","model":"other","choices":[{"index":0,"finish_reason":"stop","message":{"content":"bad"}}]}`)
	}))
	defer server.Close()
	adapter, err := localcompat.New(localcompat.Config{
		Product: localcompat.ProductGeneric, Trust: localcompat.TrustLocal, BaseURL: server.URL + "/v1",
		Protocol: modelinvoker.ProtocolChatCompletions, AllowedModels: []string{"m"},
		SupportedCapabilities: []modelinvoker.Capability{modelinvoker.CapabilityTextGeneration, modelinvoker.CapabilityStreaming},
		HTTPClient:            server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := modelinvoker.Request{
		Provider: localcompat.ProviderGeneric, Protocol: modelinvoker.ProtocolChatCompletions,
		Endpoint: server.URL + "/v1", Model: "m",
		Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "x")},
	}
	if _, err := adapter.Invoke(context.Background(), request); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorMapping {
		t.Fatalf("local invoke accepted response model drift: %v", err)
	}
	request.Stream = true
	stream, err := adapter.Stream(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	found := false
	for stream.Next() {
		if event := stream.Event(); event.Type == modelinvoker.StreamEventError && event.Error != nil && event.Error.Code == "response_model_mismatch" {
			found = true
		}
	}
	if !found || modelinvoker.ErrorKindOf(stream.Err()) != modelinvoker.ErrorMapping {
		t.Fatalf("local stream accepted response model drift: found=%t err=%v", found, stream.Err())
	}
}

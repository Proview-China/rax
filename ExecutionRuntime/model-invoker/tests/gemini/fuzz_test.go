package gemini_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/gemini"
)

func FuzzGenerateContentInvoke(f *testing.F) {
	f.Add([]byte("hello"))
	f.Add([]byte(`{"json":"text"}`))
	f.Add([]byte{0xff, 0x00, '\n'})

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, []byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP","index":0}],"responseId":"resp_fuzz","usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`))
	}))
	f.Cleanup(server.Close)
	adapter := newAdapter(f, server.URL, server.Client())

	f.Fuzz(func(t *testing.T, input []byte) {
		request := baseRequest()
		request.Input = []modelinvoker.InputItem{
			modelinvoker.MessageInput(modelinvoker.RoleUser, "fuzz:"+string(input)),
		}
		response, err := adapter.Invoke(context.Background(), request)
		if err != nil {
			t.Fatalf("Invoke() error = %v", err)
		}
		if response.ID != "resp_fuzz" || response.Text() != "ok" {
			t.Fatalf("response = %#v", response)
		}
		if strings.Contains(string(response.RawRequest.Bytes()), testAPIKey) {
			t.Fatal("fuzz RawRequest leaked API key")
		}
	})
}

func FuzzContinuationDecoder(f *testing.F) {
	f.Add([]byte(`{"version":1,"contents":[],"calls":{}}`))
	f.Add([]byte(`{"version":1,"contents":[{"role":"model","parts":[{"thoughtSignature":"c2ln"}]}],"calls":{}}`))
	f.Add([]byte(`{"version":1,"contents":[{"role":"model","parts":[{"inlineData":{}}]}],"calls":{}}`))
	f.Add([]byte{0xff, 0x00, '{'})

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, []byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP","index":0}],"responseId":"resp_cont_fuzz"}`))
	}))
	f.Cleanup(server.Close)
	adapter := newAdapter(f, server.URL, server.Client())

	f.Fuzz(func(t *testing.T, payload []byte) {
		request := baseRequest()
		request.State = &modelinvoker.State{
			Kind: modelinvoker.StateProviderContinuation, Provider: provider.ProviderID,
			Protocol: modelinvoker.ProtocolGenerateContent,
			Payload:  modelinvoker.NewRawPayload(payload),
		}
		_, err := adapter.Invoke(context.Background(), request)
		if err != nil && strings.Contains(fmt.Sprintf("%v", err), testAPIKey) {
			t.Fatal("continuation error leaked API key")
		}
	})
}

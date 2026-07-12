package zai_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/zai"
)

func TestZAIDirectAdapterRejectsMissingMismatchedAndCrossChunkModels(t *testing.T) {
	for _, actual := range []string{"glm-5.2", "other-model", ""} {
		name := actual
		if name == "" {
			name = "missing"
		}
		t.Run("invoke/"+name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(writer, `{"id":"zai","model":%q,"choices":[{"index":0,"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`, actual)
			}))
			defer server.Close()
			endpoint := server.URL + "/api/paas/v4"
			adapter, err := provider.New(provider.Config{APIKey: "offline", BaseURL: endpoint, HTTPClient: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			_, err = adapter.Invoke(context.Background(), request(endpoint, "glm-5.2"))
			if actual == "glm-5.2" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			assertZAIModelError(t, err, actual == "")
		})
	}

	t.Run("stream cross chunk", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "text/event-stream")
			for _, event := range []string{
				`{"id":"zai","model":"glm-5.2","choices":[{"index":0,"delta":{"content":"safe"},"finish_reason":null}]}`,
				`{"id":"zai","model":"other-model","choices":[{"index":0,"delta":{"content":"unsafe"},"finish_reason":"stop"}]}`,
			} {
				_, _ = fmt.Fprintf(writer, "data: %s\n\n", event)
			}
		}))
		defer server.Close()
		endpoint := server.URL + "/api/paas/v4"
		adapter, err := provider.New(provider.Config{APIKey: "offline", BaseURL: endpoint, HTTPClient: server.Client()})
		if err != nil {
			t.Fatal(err)
		}
		stream, err := adapter.Stream(context.Background(), request(endpoint, "glm-5.2"))
		if err != nil {
			t.Fatal(err)
		}
		var text string
		for stream.Next() {
			text += stream.Event().TextDelta
		}
		if text != "safe" {
			t.Fatalf("delivered text = %q", text)
		}
		assertZAIModelError(t, stream.Err(), false)
		_ = stream.Close()
		_ = stream.Close()
	})
}

func assertZAIModelError(t *testing.T, err error, missing bool) {
	t.Helper()
	var invocationError *modelinvoker.Error
	if !errors.As(err, &invocationError) || invocationError == nil || invocationError.Kind != modelinvoker.ErrorMapping {
		t.Fatalf("model identity error = %T %v", err, err)
	}
	want := "response_model_mismatch"
	if missing {
		want = "response_model_missing"
	}
	if invocationError.Code != want {
		t.Fatalf("code = %q, want %q", invocationError.Code, want)
	}
}

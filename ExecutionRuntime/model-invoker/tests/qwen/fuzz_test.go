package qwen_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/qwen"
)

func FuzzQwenSelectionNeverLeaksOrCallsUnknownModel(f *testing.F) {
	for _, seed := range []struct{ model, key string }{
		{"qwen3.7-max", "sk-ws-secret-one"},
		{"qwen3.6-plus", "sk-old-secret-two"},
		{"qwen-vl-max", "sk-ws-secret-three"},
		{"unknown", "sk-sp-subscription"},
	} {
		f.Add(seed.model, seed.key)
	}
	f.Fuzz(func(t *testing.T, model, key string) {
		if len(model) > 256 || len(key) > 256 || strings.ContainsAny(key, "\r\n") {
			t.Skip()
		}
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"resp-fuzz","object":"response","created_at":1770000000,"model":"`+model+`","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
		}))
		defer server.Close()
		adapter, err := provider.New(provider.Config{APIKey: key, Region: provider.RegionChinaBeijing, WorkspaceID: "llm-fuzz", BaseURL: server.URL, HTTPClient: server.Client()})
		if err != nil {
			if len(key) >= 8 && strings.Contains(fmt.Sprint(err), key) {
				t.Fatalf("configuration error leaked key")
			}
			return
		}
		r := request(server.URL, model, modelinvoker.ProtocolResponses)
		_, err = adapter.Invoke(context.Background(), r)
		if err != nil && len(key) >= 8 && strings.Contains(fmt.Sprint(err), key) {
			t.Fatalf("invoke error leaked key")
		}
		approved := model == "qwen3.7-max" || model == "qwen3-max" || model == "qwen3.6-plus" || model == "qwen3.6-flash" || model == "qwen-plus" || model == "qwen-flash" || model == "qwen3-coder-plus" || model == "qwen3-coder-flash"
		if !approved && calls != 0 {
			t.Fatalf("unknown model %q made %d calls", model, calls)
		}
	})
}

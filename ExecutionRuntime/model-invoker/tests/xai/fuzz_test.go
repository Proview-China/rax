package xai_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/xai"
)

func FuzzXAISelectionNeverLeaksOrCallsUnknownModel(f *testing.F) {
	for _, seed := range []struct{ model, key string }{
		{"grok-4.5", "xai-secret-one"}, {"grok-4.3", "xai-secret-two"}, {"latest", "consumer-secret"},
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
			_, _ = io.WriteString(w, `{"id":"resp","object":"response","model":"grok-4.5","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":0,"total_tokens":1}}`)
		}))
		defer server.Close()
		adapter, err := provider.New(provider.Config{APIKey: key, BaseURL: server.URL, HTTPClient: server.Client()})
		if err != nil {
			if len(key) >= 8 && strings.Contains(fmt.Sprint(err), key) {
				t.Fatal("configuration error leaked key")
			}
			return
		}
		r := request(server.URL)
		r.Model = model
		_, err = adapter.Invoke(context.Background(), r)
		if err != nil && len(key) >= 8 && strings.Contains(fmt.Sprint(err), key) {
			t.Fatal("invoke error leaked key")
		}
		if model != "grok-4.5" && calls != 0 {
			t.Fatalf("unknown model %q made %d calls", model, calls)
		}
	})
}

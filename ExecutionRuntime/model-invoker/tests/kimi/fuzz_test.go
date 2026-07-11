package kimi_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/kimi"
)

func FuzzKimiSelectionNeverLeaksOrCallsUnknownModel(f *testing.F) {
	for _, seed := range []struct{ key, model, text string }{{"a", "kimi-k2.7-code", "hello"}, {"b", "kimi-for-coding", "x"}, {"c", "kimi-latest", "y"}} {
		f.Add(seed.key, seed.model, seed.text)
	}
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"fuzz","model":"kimi-k2.7-code","choices":[{"index":0,"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	f.Cleanup(server.Close)
	f.Fuzz(func(t *testing.T, key, model, text string) {
		if strings.TrimSpace(text) == "" {
			t.Skip()
		}
		digest := sha256.Sum256([]byte(key))
		credential := "kimi-fuzz-secret-" + hex.EncodeToString(digest[:])
		adapter, err := provider.New(provider.Config{APIKey: credential, BaseURL: server.URL + "/v1", HTTPClient: server.Client()})
		if err != nil {
			t.Fatal(err)
		}
		before := calls.Load()
		response, invokeErr := adapter.Invoke(context.Background(), request(server.URL+"/v1", model))
		if strings.Contains(fmt.Sprintf("%v %#v", invokeErr, response), credential) {
			t.Fatal("public result leaked credential")
		}
		supported := model == "kimi-k2.7-code" || model == "kimi-k2.7-code-highspeed" || model == "kimi-k2.6" || model == "kimi-k2.5" || model == "moonshot-v1-8k" || model == "moonshot-v1-32k" || model == "moonshot-v1-128k"
		if !supported && calls.Load() != before {
			t.Fatalf("unknown model %q reached HTTP", model)
		}
	})
}

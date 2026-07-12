package zai_test

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

	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/zai"
)

func FuzzZAISelectionNeverLeaksOrCallsUnknownModel(f *testing.F) {
	for _, seed := range []struct{ key, model, text string }{{"a", "glm-5.2", "hello"}, {"b", "glm-latest", "x"}, {"c", "glm-coding", "y"}} {
		f.Add(seed.key, seed.model, seed.text)
	}
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"fuzz","model":"glm-5.2","choices":[{"index":0,"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	f.Cleanup(server.Close)
	f.Fuzz(func(t *testing.T, key, model, text string) {
		if strings.TrimSpace(text) == "" {
			t.Skip()
		}
		digest := sha256.Sum256([]byte(key))
		credential := "zai-fuzz-secret-" + hex.EncodeToString(digest[:])
		endpoint := server.URL + "/api/paas/v4"
		adapter, err := provider.New(provider.Config{APIKey: credential, BaseURL: endpoint, HTTPClient: server.Client()})
		if err != nil {
			t.Fatal(err)
		}
		before := calls.Load()
		response, invokeErr := adapter.Invoke(context.Background(), request(endpoint, model))
		if strings.Contains(fmt.Sprintf("%v %#v", invokeErr, response), credential) {
			t.Fatal("public result leaked credential")
		}
		supported := model == "glm-5.2"
		if !supported && calls.Load() != before {
			t.Fatalf("unknown model %q reached HTTP", model)
		}
	})
}

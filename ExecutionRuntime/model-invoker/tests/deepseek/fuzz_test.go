package deepseek_test

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

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/deepseek"
)

func FuzzDeepSeekSelectionNeverLeaksOrCallsUnknownModel(f *testing.F) {
	for _, seed := range []struct{ key, model, text string }{{"secret-a", "deepseek-v4-pro", "hello"}, {"secret-b", "claude-sonnet", "x"}, {"secret-c", "deepseek-chat", "y"}} {
		f.Add(seed.key, seed.model, seed.text)
	}
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"fuzz","model":"deepseek-v4-pro","choices":[{"index":0,"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	f.Cleanup(server.Close)
	f.Fuzz(func(t *testing.T, key, model, text string) {
		if strings.TrimSpace(text) == "" {
			t.Skip()
		}
		digest := sha256.Sum256([]byte(key))
		credential := "deepseek-fuzz-secret-" + hex.EncodeToString(digest[:])
		adapter, err := provider.New(provider.Config{APIKey: credential, BaseURL: server.URL, HTTPClient: server.Client()})
		if err != nil {
			t.Fatal(err)
		}
		before := calls.Load()
		request := modelinvoker.Request{Provider: provider.ProviderID, Protocol: modelinvoker.ProtocolChatCompletions, Endpoint: server.URL, Model: model, Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, text)}, Budget: modelinvoker.Budget{MaxOutputTokens: 8}}
		response, invokeErr := adapter.Invoke(context.Background(), request)
		combined := fmt.Sprintf("%v %#v", invokeErr, response)
		if strings.Contains(combined, credential) {
			t.Fatalf("public result leaked API key")
		}
		supported := model == "deepseek-v4-pro" || model == "deepseek-v4-flash"
		if !supported && calls.Load() != before {
			t.Fatalf("unknown model %q reached HTTP", model)
		}
	})
}

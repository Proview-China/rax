package minimax_test

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

	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/minimax"
)

func FuzzMiniMaxSelectionNeverLeaksOrCallsUnknownModel(f *testing.F) {
	for _, seed := range []struct{ key, model string }{{"a", "MiniMax-M3"}, {"b", "MiniMax-M2.7"}, {"sk-cp-plan", "MiniMax-M3"}, {"c", "unknown"}} {
		f.Add(seed.key, seed.model)
	}
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"fuzz","model":"MiniMax-M3","choices":[{"index":0,"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2},"base_resp":{"status_code":0}}`)
	}))
	f.Cleanup(server.Close)
	f.Fuzz(func(t *testing.T, key, model string) {
		digest := sha256.Sum256([]byte(key))
		credential := "minimax-fuzz-secret-" + hex.EncodeToString(digest[:])
		if strings.HasPrefix(key, "sk-cp-") {
			credential = key
		}
		before := calls.Load()
		adapter, newErr := provider.New(provider.Config{APIKey: credential, BaseURL: server.URL, HTTPClient: server.Client()})
		var response any
		var invokeErr error
		if newErr == nil {
			value, err := adapter.Invoke(context.Background(), request(server.URL+"/v1", model, "chat_completions"))
			response, invokeErr = value, err
		} else {
			invokeErr = newErr
		}
		if strings.Contains(fmt.Sprintf("%v %#v", invokeErr, response), credential) {
			t.Fatal("public result leaked credential")
		}
		supported := model == "MiniMax-M3" || model == "MiniMax-M2.7" || model == "MiniMax-M2.7-highspeed" || model == "MiniMax-M2.5" || model == "MiniMax-M2.5-highspeed" || model == "MiniMax-M2.1" || model == "MiniMax-M2.1-highspeed" || model == "MiniMax-M2"
		if (!supported || strings.HasPrefix(credential, "sk-cp-")) && calls.Load() != before {
			t.Fatalf("rejected selection reached HTTP")
		}
	})
}

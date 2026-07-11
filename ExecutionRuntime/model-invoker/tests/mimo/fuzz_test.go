package mimo_test

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
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/mimo"
)

func FuzzMiMoSelectionNeverLeaksOrCallsUnknownModel(f *testing.F) {
	for _, seed := range []struct{ key, model string }{{"a", "mimo-v2.5-pro"}, {"b", "mimo-v2.5"}, {"tp-plan", "mimo-v2.5-pro"}, {"c", "mimo-v2-pro"}} {
		f.Add(seed.key, seed.model)
	}
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"fuzz","model":"mimo-v2.5-pro","choices":[{"index":0,"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	f.Cleanup(server.Close)
	f.Fuzz(func(t *testing.T, key, model string) {
		digest := sha256.Sum256([]byte(key))
		credential := "sk-mimo-fuzz-secret-" + hex.EncodeToString(digest[:])
		if strings.HasPrefix(key, "tp-") {
			credential = key
		}
		before := calls.Load()
		adapter, newErr := provider.New(provider.Config{APIKey: credential, BaseURL: server.URL, HTTPClient: server.Client()})
		var response any
		var invokeErr error
		if newErr == nil {
			value, err := adapter.Invoke(context.Background(), request(server.URL+"/v1", model, modelinvoker.ProtocolChatCompletions))
			response, invokeErr = value, err
		} else {
			invokeErr = newErr
		}
		if strings.Contains(fmt.Sprintf("%v %#v", invokeErr, response), credential) {
			t.Fatal("public result leaked credential")
		}
		supported := model == "mimo-v2.5-pro" || model == "mimo-v2.5"
		if (!supported || strings.HasPrefix(credential, "tp-")) && calls.Load() != before {
			t.Fatalf("rejected selection reached HTTP")
		}
	})
}

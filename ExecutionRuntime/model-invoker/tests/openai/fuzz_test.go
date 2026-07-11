package openai_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

func FuzzPublicResponsesInvoke(f *testing.F) {
	f.Add([]byte("hello"))
	f.Add([]byte(`{"tool":"arguments"}`))
	f.Add([]byte{0xff, 0x00, '\n'})

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(writer, `{"id":"resp_fuzz","model":"test-model","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
	}))
	f.Cleanup(server.Close)
	adapter := newPublicAdapter(f, server.URL)

	f.Fuzz(func(t *testing.T, input []byte) {
		request := basePublicRequest(modelinvoker.ProtocolResponses)
		request.Input = []modelinvoker.InputItem{
			modelinvoker.MessageInput(modelinvoker.RoleUser, "fuzz:"+string(input)),
		}
		response, err := adapter.Invoke(context.Background(), request)
		if err != nil {
			t.Fatalf("public Responses invoke error = %v", err)
		}
		if response.ID != "resp_fuzz" {
			t.Fatalf("response ID = %q", response.ID)
		}
	})
}

func BenchmarkPublicResponsesInvoke(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(writer, `{"id":"resp_bench","model":"test-model","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
	}))
	b.Cleanup(server.Close)
	adapter := newPublicAdapter(b, server.URL)
	request := basePublicRequest(modelinvoker.ProtocolResponses)

	b.ResetTimer()
	for range b.N {
		if _, err := adapter.Invoke(context.Background(), request); err != nil {
			b.Fatal(err)
		}
	}
}

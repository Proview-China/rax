package bedrockruntime_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/bedrockruntime"
)

func TestConverseUsesAWSSDKSigV4AndNormalizesLocalHTTPFake(t *testing.T) {
	captured := make(chan *http.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured <- r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-amzn-requestid", "aws-request-1")
		_, _ = w.Write([]byte(`{"output":{"message":{"role":"assistant","content":[{"text":"ok"}]}},"stopReason":"end_turn","usage":{"inputTokens":2,"outputTokens":1,"totalTokens":3},"metrics":{"latencyMs":1}}`))
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{Region: "us-east-1", BaseURL: server.URL, CredentialMode: provider.CredentialSigV4, AccessKeyID: "AKIATESTONLY", SecretAccessKey: "test-secret", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	request := modelinvoker.Request{Provider: provider.ProviderID, Protocol: modelinvoker.ProtocolBedrockConverse, Endpoint: server.URL, Model: "anthropic.claude-test", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}, Budget: modelinvoker.Budget{MaxOutputTokens: 32}}
	response, err := adapter.Invoke(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	native := <-captured
	if native.Method != http.MethodPost || !strings.HasSuffix(native.URL.Path, "/model/anthropic.claude-test/converse") {
		t.Fatalf("request = %s %s", native.Method, native.URL.Path)
	}
	if !strings.HasPrefix(native.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
		t.Fatalf("authorization = %q", native.Header.Get("Authorization"))
	}
	if response.Text() != "ok" || response.Usage.TotalTokens != 3 || response.Provider != provider.ProviderID {
		t.Fatalf("response = %#v", response)
	}
}

func TestCredentialAndRouteMismatchesFailBeforeNetwork(t *testing.T) {
	if _, err := provider.New(provider.Config{Region: "us-east-1", CredentialMode: provider.CredentialBearer, BearerToken: "sk-proj-not-bedrock"}); err == nil {
		t.Fatal("OpenAI key accepted as Bedrock credential")
	}
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("unexpected network call") }))
	defer server.Close()
	adapter, err := provider.New(provider.Config{Region: "us-east-1", BaseURL: server.URL, CredentialMode: provider.CredentialSigV4, AccessKeyID: "AKIATEST", SecretAccessKey: "secret", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	request := modelinvoker.Request{Provider: provider.ProviderID, Protocol: modelinvoker.ProtocolBedrockConverse, Endpoint: server.URL + "/wrong", Model: "model", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "x")}}
	if _, err := adapter.Invoke(context.Background(), request); err == nil {
		t.Fatal("endpoint mismatch accepted")
	}
}

func TestBedrockBearerCredentialUsesBearerAuthScheme(t *testing.T) {
	authorization := make(chan string, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization <- r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":{"message":{"role":"assistant","content":[{"text":"ok"}]}},"stopReason":"end_turn","usage":{"inputTokens":1,"outputTokens":1,"totalTokens":2},"metrics":{"latencyMs":1}}`))
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{Region: "us-east-1", BaseURL: server.URL, CredentialMode: provider.CredentialBearer, BearerToken: "ABSK-runtime-test", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	request := modelinvoker.Request{Provider: provider.ProviderID, Protocol: modelinvoker.ProtocolBedrockConverse, Endpoint: server.URL, Model: "model", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}}
	if _, err := adapter.Invoke(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if got := <-authorization; got != "Bearer ABSK-runtime-test" {
		t.Fatalf("Authorization = %q", got)
	}
}

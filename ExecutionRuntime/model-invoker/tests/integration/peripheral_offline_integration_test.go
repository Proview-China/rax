//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/job"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation/nativehttp"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/localcompat"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/realtime"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/realtime/nativews"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/resource"
	"github.com/gorilla/websocket"
)

func TestPeripheralUnionOfflineEndToEndAcrossHTTPResourceJobLocalAndRealtime(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/ops/files" && r.Method == http.MethodPost:
			_, _ = io.WriteString(w, `{"id":"file-1","status":"processed"}`)
		case r.URL.Path == "/ops/images" && r.Method == http.MethodPost:
			_, _ = io.WriteString(w, `{"data":[{"b64_json":"aW1hZ2U="}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`)
		case r.URL.Path == "/ops/batches/batch-1/results" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = io.WriteString(w, "{\"custom_id\":\"one\"}\n{\"custom_id\":\"two\"}\n")
		case r.URL.Path == "/v1/chat/completions" && r.Method == http.MethodPost:
			_, _ = io.WriteString(w, `{"id":"chat-local","object":"chat.completion","created":1,"model":"local-model","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"local-text"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer httpServer.Close()

	peripheral, err := nativehttp.New(nativehttp.Config{
		Provider: "peripheral", BaseURL: httpServer.URL + "/ops", Trust: nativehttp.TrustLocal, Auth: nativehttp.AuthAnonymous,
		Specs: []nativehttp.Spec{
			{Kind: operation.FileCreate, Method: http.MethodPost, Path: "/files", Lifecycle: operation.LifecycleResource, Support: operation.SupportNative, ContentTypes: []string{"application/json"}, ResponseMode: nativehttp.ResponseJSON, IDKeys: []string{"id"}, StatusKeys: []string{"status"}},
			{Kind: operation.ImageGenerate, Method: http.MethodPost, Path: "/images", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, Models: []string{"image-model"}, BodyModelField: "model", ContentTypes: []string{"application/json"}, ResponseMode: nativehttp.ResponseJSON, ArtifactKind: operation.ArtifactImage, ArtifactMIME: "image/png", Base64Keys: []string{"b64_json"}},
			{Kind: operation.BatchResults, Method: http.MethodGet, Path: "/batches/{id}/results", RequiresResourceID: true, Lifecycle: operation.LifecycleJob, Support: operation.SupportNative, ResponseMode: nativehttp.ResponseNDJSON},
		},
		HTTPClient: httpServer.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, _ := operation.NewRegistry(peripheral)
	invoker, _ := operation.NewInvoker(registry)
	resources, _ := resource.NewClient(invoker)
	jobs, _ := job.NewClient(invoker)
	file, err := resources.Create(context.Background(), resource.Request{
		Provider: "peripheral", Kind: resource.File, ContentType: "application/json", Body: modelinvoker.NewRawPayload([]byte(`{"purpose":"assistants"}`)),
	})
	if err != nil || file.Resource == nil || file.Resource.ID != "file-1" {
		t.Fatalf("resource lifecycle failed: %+v %v", file, err)
	}
	image, err := invoker.Invoke(context.Background(), operation.Request{
		Provider: "peripheral", Kind: operation.ImageGenerate, Model: "image-model", ContentType: "application/json",
		Body: modelinvoker.NewRawPayload([]byte(`{"model":"image-model","prompt":"x"}`)),
	})
	if err != nil || len(image.Artifacts) != 1 || string(image.Artifacts[0].Data) != "image" || image.Usage.TotalTokens != 3 {
		t.Fatalf("request lifecycle failed: %+v %v", image, err)
	}
	results, err := jobs.StreamResults(context.Background(), job.Request{Provider: "peripheral", Kind: job.Batch, ID: "batch-1"})
	if err != nil {
		t.Fatal(err)
	}
	defer results.Close()
	var native, completed int
	for results.Next() {
		switch results.Event().Type {
		case operation.StreamNative:
			native++
		case operation.StreamCompleted:
			completed++
		}
	}
	if err := results.Err(); err != nil || native != 2 || completed != 1 {
		t.Fatalf("job lifecycle stream native=%d completed=%d err=%v", native, completed, err)
	}

	local, err := localcompat.New(localcompat.Config{
		Product: localcompat.ProductOllama, Trust: localcompat.TrustLocal, BaseURL: httpServer.URL + "/v1", Protocol: modelinvoker.ProtocolChatCompletions,
		AllowedModels: []string{"local-model"}, SupportedCapabilities: []modelinvoker.Capability{modelinvoker.CapabilityTextGeneration}, HTTPClient: httpServer.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	textResponse, err := local.Invoke(context.Background(), modelinvoker.Request{
		Provider: localcompat.ProviderOllama, Protocol: modelinvoker.ProtocolChatCompletions, Endpoint: httpServer.URL + "/v1", Model: "local-model",
		Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}, Budget: modelinvoker.Budget{MaxOutputTokens: 8},
	})
	if err != nil || textResponse.Text() != "local-text" {
		t.Fatalf("local compatible lifecycle failed: %+v %v", textResponse, err)
	}

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("model") != "voice-model" {
			t.Errorf("realtime model query missing")
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, input, _ := conn.ReadMessage()
		_ = conn.WriteMessage(websocket.TextMessage, input)
	}))
	defer wsServer.Close()
	live, err := nativews.New(nativews.Config{
		Provider: "voice", URL: "ws" + strings.TrimPrefix(wsServer.URL, "http"), Trust: nativews.TrustLocal,
		Auth: nativews.AuthAnonymous, ModelQueryKey: "model", AllowedModels: []string{"voice-model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	session, err := live.Open(context.Background(), realtime.Request{Provider: "voice", Model: "voice-model", Modalities: []realtime.Modality{realtime.Audio}})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err := session.Send(context.Background(), realtime.ClientEvent{Type: "input_audio", Text: "chunk"}); err != nil {
		t.Fatal(err)
	}
	if !session.Next() || session.Event().Type != "input_audio" || session.Event().Text != "chunk" {
		t.Fatalf("realtime lifecycle failed: %+v err=%v", session.Event(), session.Err())
	}

	t.Log(fmt.Sprintf("verified resource=%s artifacts=%d batch_events=%d local=%s realtime=%s", file.Resource.ID, len(image.Artifacts), native+completed, textResponse.Text(), session.Event().Type))
}

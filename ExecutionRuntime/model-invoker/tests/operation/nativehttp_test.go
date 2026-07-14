package operation_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation/nativehttp"
)

func TestNativeHTTPPinsMethodPathModelQueryAndAuth(t *testing.T) {
	const secret = "test-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/models/model-1:embed" || r.URL.Query().Get("dimensions") != "2" {
			t.Errorf("unexpected request target: %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("Authorization") != "Bearer "+secret {
			t.Errorf("missing pinned auth header")
		}
		if r.Header.Get("X-Provider-Version") != "beta" {
			t.Errorf("missing pinned provider header")
		}
		w.Header().Set("x-request-id", "req-1")
		_, _ = io.WriteString(w, `{"data":[{"index":0,"embedding":[0.1,0.2]}],"usage":{"prompt_tokens":3,"total_tokens":3}}`)
	}))
	defer server.Close()

	p, err := nativehttp.New(nativehttp.Config{
		Provider: "p", BaseURL: server.URL, Trust: nativehttp.TrustRelay, Auth: nativehttp.AuthBearer, APIKey: secret,
		StaticHeaders: http.Header{"X-Provider-Version": {"beta"}},
		Specs: []nativehttp.Spec{{
			Kind: operation.EmbeddingCreate, Method: http.MethodPost, Path: "/models/{model}:embed", RequiresModel: true,
			Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, Models: []string{"model-1"},
			ContentTypes: []string{"application/json"}, AllowedQuery: []string{"dimensions"}, ResponseMode: nativehttp.ResponseJSON,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := p.Invoke(context.Background(), operation.Request{
		Provider: "p", Kind: operation.EmbeddingCreate, Model: "model-1", ContentType: "application/json",
		Body: modelinvoker.NewRawPayload([]byte(`{"input":"x"}`)), Query: map[string][]string{"dimensions": {"2"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RequestID != "req-1" || len(result.Vectors) != 1 || len(result.Vectors[0].Values) != 2 || result.Usage.InputTokens != 3 {
		t.Fatalf("unexpected normalized result: %+v", result)
	}
	if strings.Contains(result.RawRequest.String(), secret) || strings.Contains(result.RawResponse.String(), secret) {
		t.Fatal("credential leaked into raw payload")
	}
}

func TestNativeHTTPArtifactSelectorDoesNotDecodeUnrelatedDataFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":"dGVzdA==","candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"aW1hZ2U="}}]}}]}`)
	}))
	defer server.Close()
	p, err := nativehttp.New(nativehttp.Config{
		Provider: "p", BaseURL: server.URL, Trust: nativehttp.TrustLocal, Auth: nativehttp.AuthAnonymous,
		Specs: []nativehttp.Spec{{Kind: operation.ImageGenerate, Method: http.MethodPost, Path: "/image", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ResponseMode: nativehttp.ResponseJSON, ArtifactKind: operation.ArtifactImage, Base64Keys: []string{"inlineData.data"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := p.Invoke(context.Background(), operation.Request{Provider: "p", Kind: operation.ImageGenerate})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || string(result.Artifacts[0].Data) != "image" {
		t.Fatalf("selector produced phantom artifacts: %+v", result.Artifacts)
	}
}

func TestNativeHTTPNormalizesResourcePrefixJobStateAndDoneError(t *testing.T) {
	response := `{"name":"batches/123","state":"JOB_STATE_SUCCEEDED"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, response) }))
	defer server.Close()
	p, err := nativehttp.New(nativehttp.Config{
		Provider: "p", BaseURL: server.URL, Trust: nativehttp.TrustLocal, Auth: nativehttp.AuthAnonymous,
		Specs: []nativehttp.Spec{{Kind: operation.BatchGet, Method: http.MethodGet, Path: "/batches/{id}", RequiresResourceID: true, Lifecycle: operation.LifecycleJob, Support: operation.SupportNative, ResponseMode: nativehttp.ResponseJSON, IDKeys: []string{"name"}, IDPrefix: "batches/", StatusKeys: []string{"state"}, DoneKeys: []string{"done"}, FailureKeys: []string{"error"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := p.Invoke(context.Background(), operation.Request{Provider: "p", Kind: operation.BatchGet, ResourceID: "123"})
	if err != nil || result.Job == nil || result.Job.ID != "123" || result.Job.Status != operation.StatusSucceeded {
		t.Fatalf("normalized job = %+v, %v", result.Job, err)
	}
	response = `{"name":"batches/123","done":true,"error":{"code":13}}`
	result, err = p.Invoke(context.Background(), operation.Request{Provider: "p", Kind: operation.BatchGet, ResourceID: "123"})
	if err != nil || result.Job == nil || result.Job.Status != operation.StatusFailed {
		t.Fatalf("done job with provider error must fail: %+v, %v", result.Job, err)
	}
}

func TestNativeHTTPRejectsUnconfiguredShapesAndRedirects(t *testing.T) {
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, "https://example.com/steal", http.StatusFound)
	}))
	defer redirect.Close()
	p, err := nativehttp.New(nativehttp.Config{
		Provider: "p", BaseURL: redirect.URL, Trust: nativehttp.TrustLocal, Auth: nativehttp.AuthAnonymous,
		Specs: []nativehttp.Spec{{Kind: operation.ImageGenerate, Method: http.MethodPost, Path: "/image", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ContentTypes: []string{"application/json"}, ResponseMode: nativehttp.ResponseJSON}},
	})
	if err != nil {
		t.Fatal(err)
	}
	base := operation.Request{Provider: "p", Kind: operation.ImageGenerate, Model: "m", ContentType: "application/json", Body: modelinvoker.NewRawPayload([]byte(`{}`))}
	bad := base
	bad.Query = map[string][]string{"escape": {"1"}}
	if _, err := p.Invoke(context.Background(), bad); err == nil {
		t.Fatal("unconfigured query must fail before transport")
	}
	_, err = p.Invoke(context.Background(), base)
	var typed *modelinvoker.Error
	if !errors.As(err, &typed) || typed.HTTPStatus != http.StatusFound {
		t.Fatalf("redirect must not be followed: %T %v", err, err)
	}
}

func TestNativeHTTPResponseLimitAndErrorRedaction(t *testing.T) {
	const secret = "never-print-this"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/large" {
			_, _ = io.WriteString(w, "12345")
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"`+secret+`"}}`)
	}))
	defer server.Close()
	makeProvider := func(path string, mode nativehttp.ResponseMode) *nativehttp.Provider {
		p, err := nativehttp.New(nativehttp.Config{Provider: "p", BaseURL: server.URL, Trust: nativehttp.TrustRelay, Auth: nativehttp.AuthBearer, APIKey: secret, Specs: []nativehttp.Spec{{Kind: operation.SpeechGenerate, Method: http.MethodPost, Path: path, Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ResponseMode: mode, ArtifactKind: operation.ArtifactAudio}}})
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	_, err := makeProvider("/large", nativehttp.ResponseBinary).Invoke(context.Background(), operation.Request{Provider: "p", Kind: operation.SpeechGenerate, Budget: operation.Budget{MaxResponseBytes: 4}})
	if err == nil {
		t.Fatal("oversized response must fail")
	}
	_, err = makeProvider("/error", nativehttp.ResponseJSON).Invoke(context.Background(), operation.Request{Provider: "p", Kind: operation.SpeechGenerate})
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("upstream error must be sanitized: %v", err)
	}
}

func TestNativeHTTPConfigTrustGates(t *testing.T) {
	spec := []nativehttp.Spec{{Kind: operation.EmbeddingCreate, Method: http.MethodPost, Path: "/embed", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ResponseMode: nativehttp.ResponseJSON}}
	bad := []nativehttp.Config{
		{Provider: "p", BaseURL: "http://example.com", Trust: nativehttp.TrustLocal, Auth: nativehttp.AuthAnonymous, Specs: spec},
		{Provider: "p", BaseURL: "https://evil.example/v1", Trust: nativehttp.TrustOfficial, OfficialHosts: []string{"api.example.com"}, Auth: nativehttp.AuthAnonymous, Specs: spec},
		{Provider: "p", BaseURL: "https://user:pass@example.com", Trust: nativehttp.TrustRelay, Auth: nativehttp.AuthAnonymous, Specs: spec},
		{Provider: "p", BaseURL: "https://example.com", Trust: nativehttp.TrustRelay, Auth: nativehttp.AuthAnonymous, StaticHeaders: http.Header{"Authorization": {"steal"}}, Specs: spec},
	}
	for index, config := range bad {
		if _, err := nativehttp.New(config); err == nil {
			t.Fatalf("unsafe config %d should fail", index)
		}
	}
}

func TestNativeHTTPBindsDeclaredModelToJSONAndMultipartBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()
	p, err := nativehttp.New(nativehttp.Config{
		Provider: "p", BaseURL: server.URL, Trust: nativehttp.TrustLocal, Auth: nativehttp.AuthAnonymous,
		Specs: []nativehttp.Spec{{Kind: operation.ImageGenerate, Method: http.MethodPost, Path: "/image", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, Models: []string{"m"}, BodyModelField: "model", ContentTypes: []string{"application/json", "multipart/form-data"}, ResponseMode: nativehttp.ResponseJSON}},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := operation.Request{Provider: "p", Kind: operation.ImageGenerate, Model: "m", ContentType: "application/json", Body: modelinvoker.NewRawPayload([]byte(`{"model":"other"}`))}
	if _, err := p.Invoke(context.Background(), request); err == nil {
		t.Fatal("JSON body model drift was accepted")
	}
	request.Body = modelinvoker.NewRawPayload([]byte(`{"model":"m"}`))
	if _, err := p.Invoke(context.Background(), request); err != nil {
		t.Fatalf("matching JSON model failed: %v", err)
	}

	var body strings.Builder
	body.WriteString("--boundary\r\nContent-Disposition: form-data; name=\"model\"\r\n\r\nother\r\n--boundary--\r\n")
	request.ContentType = "multipart/form-data; boundary=boundary"
	request.Body = modelinvoker.NewRawPayload([]byte(body.String()))
	if _, err := p.Invoke(context.Background(), request); err == nil {
		t.Fatal("multipart body model drift was accepted")
	}
}

func TestNativeHTTPStreamsSSEWithoutReorderingNativeEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: progress\ndata: {\"progress\":0.5}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	p, err := nativehttp.New(nativehttp.Config{
		Provider: "p", BaseURL: server.URL, Trust: nativehttp.TrustLocal, Auth: nativehttp.AuthAnonymous,
		Specs: []nativehttp.Spec{{Kind: operation.VideoGet, Method: http.MethodGet, Path: "/jobs/{id}", RequiresResourceID: true, Lifecycle: operation.LifecycleJob, Support: operation.SupportNative, ResponseMode: nativehttp.ResponseSSE}},
	})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := p.Stream(context.Background(), operation.Request{Provider: "p", Kind: operation.VideoGet, ResourceID: "job-1"})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if !stream.Next() || stream.Event().Type != operation.StreamNative || stream.Event().Sequence != 1 || !strings.Contains(string(stream.Event().Raw.Bytes()), "progress") {
		t.Fatalf("unexpected first SSE event: %+v", stream.Event())
	}
	if !stream.Next() || stream.Event().Type != operation.StreamCompleted || stream.Event().Sequence != 2 {
		t.Fatalf("unexpected terminal SSE event: %+v", stream.Event())
	}
}

func TestNativeHTTPRejectsStreamingResponseThroughInvokeBeforeNetwork(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	p, err := nativehttp.New(nativehttp.Config{
		Provider: "p", BaseURL: server.URL, Trust: nativehttp.TrustLocal, Auth: nativehttp.AuthAnonymous,
		Specs: []nativehttp.Spec{{Kind: operation.BatchResults, Method: http.MethodGet, Path: "/batches/{id}/results", RequiresResourceID: true, Lifecycle: operation.LifecycleJob, Support: operation.SupportNative, ResponseMode: nativehttp.ResponseNDJSON}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Invoke(context.Background(), operation.Request{Provider: "p", Kind: operation.BatchResults, ResourceID: "batch-1"}); err == nil {
		t.Fatal("NDJSON response was accepted through synchronous Invoke")
	}
	if calls != 0 {
		t.Fatalf("invalid response mode reached network %d times", calls)
	}
}

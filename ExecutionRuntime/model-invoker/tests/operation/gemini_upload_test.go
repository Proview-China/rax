package operation_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation/geminiupload"
)

func TestGeminiResumableUploadUsesTrustedTwoStepProtocol(t *testing.T) {
	const key = "gemini-key"
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/upload/v1beta/files":
			if r.Header.Get("X-Goog-Api-Key") != key || r.Header.Get("X-Goog-Upload-Command") != "start" || r.Header.Get("X-Goog-Upload-Header-Content-Type") != "text/plain" {
				t.Errorf("invalid upload handshake headers: %+v", r.Header)
			}
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "doc.txt") {
				t.Errorf("display name missing from metadata: %s", body)
			}
			w.Header().Set("X-Goog-Upload-URL", server.URL+"/upload/v1beta/files/session?token=opaque")
			w.WriteHeader(http.StatusOK)
		case "/upload/v1beta/files/session":
			if r.Header.Get("X-Goog-Api-Key") != "" || r.Header.Get("X-Goog-Upload-Command") != "upload, finalize" || r.URL.Query().Get("token") != "opaque" {
				t.Errorf("invalid finalize request: %s %+v", r.URL.String(), r.Header)
			}
			body, _ := io.ReadAll(r.Body)
			if string(body) != "hello" {
				t.Errorf("uploaded bytes = %q", body)
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Goog-Request-Id", "upload-request")
			_, _ = fmt.Fprint(w, `{"file":{"name":"files/abc","uri":"https://example.invalid/file","state":"ACTIVE","mimeType":"text/plain","sizeBytes":"5"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	uploader, err := geminiupload.New(geminiupload.Config{APIKey: key, BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	registry, _ := operation.NewRegistry(uploader)
	invoker, _ := operation.NewInvoker(registry)
	result, err := invoker.Invoke(context.Background(), operation.Request{
		Provider: geminiupload.ProviderID, Kind: operation.FileCreate, ContentType: "text/plain",
		Body: modelinvoker.NewRawPayload([]byte("hello")), Metadata: modelinvoker.Metadata{"display_name": "doc.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Resource == nil || result.Resource.ID != "abc" || result.Status != operation.StatusSucceeded || result.RequestID != "upload-request" || len(result.Artifacts) != 1 || result.Artifacts[0].SizeBytes != 5 {
		t.Fatalf("unexpected upload result: %+v", result)
	}
	if strings.Contains(result.RawRequest.String(), key) || strings.Contains(result.RawResponse.String(), key) {
		t.Fatal("Gemini credential leaked into audit payload")
	}
}

func TestGeminiUploadRejectsCrossOriginOneTimeURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Goog-Upload-URL", "https://evil.example/upload/v1beta/files/steal?token=x")
	}))
	defer server.Close()
	uploader, err := geminiupload.New(geminiupload.Config{APIKey: "key", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = uploader.Invoke(context.Background(), operation.Request{Provider: geminiupload.ProviderID, Kind: operation.FileCreate, ContentType: "text/plain", Body: modelinvoker.NewRawPayload([]byte("x")), Metadata: modelinvoker.Metadata{"display_name": "x.txt"}})
	if err == nil {
		t.Fatal("cross-origin one-time upload URL was accepted")
	}
}

func TestCompositeRejectsOverlappingOperationOwners(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	one, _ := geminiupload.New(geminiupload.Config{APIKey: "key", BaseURL: server.URL, HTTPClient: server.Client()})
	two, _ := geminiupload.New(geminiupload.Config{APIKey: "key", BaseURL: server.URL, HTTPClient: server.Client()})
	if _, err := operation.NewComposite(geminiupload.ProviderID, one, two); err == nil {
		t.Fatal("overlapping composite operation owners were accepted")
	}
}

func TestGeminiUploadConfigurationAndRequestNegativeMatrix(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	configs := []geminiupload.Config{
		{},
		{APIKey: " bad ", BaseURL: server.URL},
		{APIKey: "key", BaseURL: "https://example.com"},
		{APIKey: "key", BaseURL: server.URL, MaxUploadBytes: -1},
		{APIKey: "key", BaseURL: server.URL, MaxUploadBytes: 3 << 30},
	}
	for index, config := range configs {
		if _, err := geminiupload.New(config); err == nil {
			t.Fatalf("invalid upload config %d accepted", index)
		}
	}
	const secret = "upload-format-secret"
	if formatted := fmt.Sprintf("%v %#v", geminiupload.Config{APIKey: secret}, geminiupload.Config{APIKey: secret}); strings.Contains(formatted, secret) {
		t.Fatal("upload config formatting leaked credential")
	}

	uploader, err := geminiupload.New(geminiupload.Config{APIKey: "key", BaseURL: server.URL, MaxUploadBytes: 1, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if uploader.ID() != geminiupload.ProviderID || len(uploader.Kinds()) != 1 || uploader.Kinds()[0] != operation.FileCreate {
		t.Fatal("upload provider identity or kinds drifted")
	}
	if _, err := uploader.Capabilities(nil, operation.Query{}); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorInvalidRequest {
		t.Fatalf("nil capability context error = %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := uploader.Capabilities(cancelled, operation.Query{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled capability context error = %v", err)
	}
	base := operation.Request{Provider: geminiupload.ProviderID, Kind: operation.FileCreate, ContentType: "text/plain", Body: modelinvoker.NewRawPayload([]byte("x")), Metadata: modelinvoker.Metadata{"display_name": "x.txt"}}
	cases := []operation.Request{
		func() operation.Request { r := base; r.Provider = "other"; return r }(),
		func() operation.Request { r := base; r.Kind = operation.FileGet; return r }(),
		func() operation.Request {
			r := base
			r.ProviderOptions = modelinvoker.ProviderOptions{"gemini": []byte(`{}`)}
			return r
		}(),
		func() operation.Request { r := base; r.Body = modelinvoker.RawPayload{}; return r }(),
		func() operation.Request { r := base; r.Body = modelinvoker.NewRawPayload([]byte("xx")); return r }(),
		func() operation.Request { r := base; r.ContentType = "bad type"; return r }(),
		func() operation.Request { r := base; r.Metadata = nil; return r }(),
		func() operation.Request {
			r := base
			r.Metadata = modelinvoker.Metadata{"display_name": " bad "}
			return r
		}(),
	}
	for index, request := range cases {
		if _, err := uploader.Invoke(context.Background(), request); err == nil {
			t.Fatalf("invalid upload request %d succeeded", index)
		}
	}
	if _, err := uploader.Invoke(nil, base); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorInvalidRequest {
		t.Fatalf("nil invoke context error = %v", err)
	}
	if _, err := uploader.Stream(context.Background(), base); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorUnsupportedCapability {
		t.Fatalf("upload stream error = %v", err)
	}
	var nilUploader *geminiupload.Provider
	if _, err := nilUploader.Capabilities(context.Background(), operation.Query{}); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorProviderUnavailable {
		t.Fatalf("nil provider capabilities error = %v", err)
	}
	if _, err := nilUploader.Invoke(context.Background(), base); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorProviderUnavailable {
		t.Fatalf("nil provider invoke error = %v", err)
	}
}

func TestGeminiUploadHTTPStatusDecodeAndLimitMatrix(t *testing.T) {
	wantKinds := map[int]modelinvoker.ErrorKind{
		http.StatusBadRequest:         modelinvoker.ErrorInvalidRequest,
		http.StatusUnauthorized:       modelinvoker.ErrorAuthentication,
		http.StatusForbidden:          modelinvoker.ErrorPermission,
		http.StatusTooManyRequests:    modelinvoker.ErrorRateLimit,
		http.StatusServiceUnavailable: modelinvoker.ErrorProviderUnavailable,
	}
	for status, want := range wantKinds {
		t.Run(fmt.Sprintf("start-%d", status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
			defer server.Close()
			uploader, _ := geminiupload.New(geminiupload.Config{APIKey: "key", BaseURL: server.URL, HTTPClient: server.Client()})
			_, err := uploader.Invoke(context.Background(), operation.Request{Provider: geminiupload.ProviderID, Kind: operation.FileCreate, ContentType: "text/plain", Body: modelinvoker.NewRawPayload([]byte("x")), Metadata: modelinvoker.Metadata{"display_name": "x.txt"}})
			if modelinvoker.ErrorKindOf(err) != want {
				t.Fatalf("status %d kind = %s, want %s: %v", status, modelinvoker.ErrorKindOf(err), want, err)
			}
		})
	}

	tests := []struct {
		name      string
		finalBody string
		status    int
		limit     int64
		wantKind  modelinvoker.ErrorKind
	}{
		{"final-status", `{}`, http.StatusServiceUnavailable, 0, modelinvoker.ErrorProviderUnavailable},
		{"invalid-resource", `{}`, http.StatusOK, 0, modelinvoker.ErrorProvider},
		{"oversized-response", `{"file":{"name":"files/x"}}`, http.StatusOK, 4, modelinvoker.ErrorProvider},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/upload/v1beta/files" {
					w.Header().Set("X-Goog-Upload-URL", server.URL+"/upload/v1beta/session")
					return
				}
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.finalBody)
			}))
			defer server.Close()
			uploader, _ := geminiupload.New(geminiupload.Config{APIKey: "key", BaseURL: server.URL, HTTPClient: server.Client()})
			_, err := uploader.Invoke(context.Background(), operation.Request{Provider: geminiupload.ProviderID, Kind: operation.FileCreate, ContentType: "text/plain", Body: modelinvoker.NewRawPayload([]byte("x")), Metadata: modelinvoker.Metadata{"display_name": "x.txt"}, Budget: operation.Budget{MaxResponseBytes: test.limit}})
			if modelinvoker.ErrorKindOf(err) != test.wantKind {
				t.Fatalf("kind = %s, want %s: %v", modelinvoker.ErrorKindOf(err), test.wantKind, err)
			}
		})
	}
}

package operation_test

import (
	"context"
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

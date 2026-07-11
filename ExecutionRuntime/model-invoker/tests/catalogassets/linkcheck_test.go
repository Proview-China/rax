package catalogassets_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
)

func TestSourceLinkCheckerUsesOnlyExplicitLoopbackClient(t *testing.T) {
	var fallbackGET atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("User-Agent") != "Praxis-Catalog-LinkCheck/1" {
			t.Errorf("User-Agent = %q", request.Header.Get("User-Agent"))
		}
		switch request.URL.Path {
		case "/ok":
			writer.WriteHeader(http.StatusNoContent)
		case "/fallback":
			if request.Method == http.MethodHead {
				writer.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			fallbackGET.Add(1)
			if request.Header.Get("Range") != "bytes=0-0" {
				t.Errorf("fallback Range = %q", request.Header.Get("Range"))
			}
			writer.WriteHeader(http.StatusPartialContent)
		case "/missing":
			writer.WriteHeader(http.StatusNotFound)
		default:
			writer.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	checker, err := catalog.NewSourceLinkChecker(server.Client())
	if err != nil {
		t.Fatalf("NewSourceLinkChecker() error = %v", err)
	}
	sources := []catalog.OfficialSource{
		{ID: "source.ok", URL: server.URL + "/ok"},
		{ID: "source.fallback", URL: server.URL + "/fallback"},
		{ID: "source.missing", URL: server.URL + "/missing"},
	}
	results, err := checker.Check(context.Background(), sources)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(results) != len(sources) {
		t.Fatalf("Check() results = %d, want %d", len(results), len(sources))
	}
	if !results[0].Reachable || results[0].Method != http.MethodHead || results[0].StatusCode != http.StatusNoContent {
		t.Fatalf("HEAD result = %#v", results[0])
	}
	if !results[1].Reachable || results[1].Method != http.MethodGet || results[1].StatusCode != http.StatusPartialContent || fallbackGET.Load() != 1 {
		t.Fatalf("fallback result = %#v, GET calls = %d", results[1], fallbackGET.Load())
	}
	if results[2].Reachable || results[2].StatusCode != http.StatusNotFound || results[2].Problem == "" {
		t.Fatalf("missing result = %#v", results[2])
	}
	for index, result := range results {
		if result.SourceID != sources[index].ID || result.URL != sources[index].URL {
			t.Fatalf("result order/identity changed at %d: %#v", index, result)
		}
	}
}

func TestSourceLinkCheckerRequiresExplicitDependenciesAndHonorsCancellation(t *testing.T) {
	if _, err := catalog.NewSourceLinkChecker(nil); err == nil {
		t.Fatal("NewSourceLinkChecker(nil) unexpectedly succeeded")
	}
	checker, err := catalog.NewSourceLinkChecker(doerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("injected transport failure")
	}))
	if err != nil {
		t.Fatal(err)
	}
	results, err := checker.Check(context.Background(), []catalog.OfficialSource{{ID: "source.failure", URL: "https://docs.example.invalid/source"}})
	if err != nil || len(results) != 1 || results[0].Reachable || results[0].Problem == "" {
		t.Fatalf("injected failure results=%#v error=%v", results, err)
	}
	if _, err := checker.Check(nil, nil); err == nil {
		t.Fatal("Check(nil, nil) unexpectedly succeeded")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := checker.Check(cancelled, []catalog.OfficialSource{{ID: "source.cancelled", URL: "https://docs.example.invalid/source"}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Check() error = %v, want context.Canceled", err)
	}
}

type doerFunc func(*http.Request) (*http.Response, error)

func (function doerFunc) Do(request *http.Request) (*http.Response, error) {
	return function(request)
}

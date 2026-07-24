package cli

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

type transport struct {
	path string
	body []byte
}

func (t *transport) Do(_ context.Context, path string, body []byte) ([]byte, error) {
	t.path = path
	t.body = append([]byte(nil), body...)
	return []byte(`{"ok":true}`), nil
}

func TestDocumentedCommandsMapWithoutChoosingEndpoint(t *testing.T) {
	cases := map[string]string{"memory search": "/v1/memory/query", "memory inspect": "/v1/memory/inspect", "memory forget": "/v1/memory/forget", "knowledge source list": "/v1/knowledge/source/list", "knowledge snapshot build": "/v1/knowledge/snapshot/build", "knowledge query": "/v1/knowledge/query", "index status": "/v1/index/status"}
	for command, want := range cases {
		tr := &transport{}
		var out bytes.Buffer
		if err := Run(context.Background(), strings.Fields(command), bytes.NewBufferString(`{"x":1}`), &out, tr); err != nil {
			t.Fatalf("%s: %v", command, err)
		}
		if tr.path != want || out.String() != `{"ok":true}` {
			t.Fatalf("%s => %s %s", command, tr.path, out.String())
		}
	}
}
func TestUnknownCommandAndBodyBound(t *testing.T) {
	if err := Run(context.Background(), []string{"unknown"}, nil, &bytes.Buffer{}, &transport{}); !errors.Is(err, contract.ErrInvalidArgument) {
		t.Fatalf("unknown: %v", err)
	}
	large := bytes.NewReader(bytes.Repeat([]byte("x"), (1<<20)+1))
	if err := Run(context.Background(), []string{"index", "status"}, large, &bytes.Buffer{}, &transport{}); !errors.Is(err, contract.ErrInvalidArgument) {
		t.Fatalf("large: %v", err)
	}
}

func TestHTTPTransportRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), (1<<20)+1))
	}))
	defer server.Close()
	_, err := (HTTPTransport{BaseURL: server.URL, Client: server.Client()}).Do(context.Background(), "/v1/index/status", []byte("{}"))
	if !errors.Is(err, contract.ErrInvalidArgument) {
		t.Fatalf("oversized response: %v", err)
	}
}

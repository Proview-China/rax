package realtime_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/realtime"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/realtime/nativews"
	"github.com/gorilla/websocket"
)

func TestNativeWebSocketPreservesConfigurationAndFrameOrder(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("model") != "voice-1" || r.Header.Get("X-Feature") != "enabled" {
			t.Errorf("pinned session metadata missing: %s %+v", r.URL.String(), r.Header)
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, configuration, _ := conn.ReadMessage()
		if string(configuration) != `{"type":"session.update"}` {
			t.Errorf("unexpected configuration: %s", configuration)
		}
		_, message, _ := conn.ReadMessage()
		_ = conn.WriteMessage(websocket.TextMessage, message)
		_ = conn.WriteMessage(websocket.BinaryMessage, []byte{1, 2, 3})
	}))
	defer server.Close()

	p, err := nativews.New(nativews.Config{
		Provider: "voice", URL: "ws" + strings.TrimPrefix(server.URL, "http"), Trust: nativews.TrustLocal,
		Auth: nativews.AuthAnonymous, ModelQueryKey: "model", AllowedModels: []string{"voice-1"},
		StaticHeaders: http.Header{"X-Feature": {"enabled"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	session, err := p.Open(context.Background(), realtime.Request{
		Provider: "voice", Model: "voice-1", Modalities: []realtime.Modality{realtime.Audio},
		Configuration: modelinvoker.NewRawPayload([]byte(`{"type":"session.update"}`)),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err := session.Send(context.Background(), realtime.ClientEvent{Type: "input", Text: "hello"}); err != nil {
		t.Fatal(err)
	}
	if !session.Next() || session.Event().Sequence != 1 || session.Event().Type != "input" || session.Event().Text != "hello" {
		t.Fatalf("unexpected first event: %+v err=%v", session.Event(), session.Err())
	}
	if !session.Next() || session.Event().Sequence != 2 || session.Event().Type != "binary" || len(session.Event().Binary) != 3 {
		t.Fatalf("unexpected second event: %+v err=%v", session.Event(), session.Err())
	}
}

func TestNativeWebSocketRejectsUnsafeConfiguration(t *testing.T) {
	cases := []nativews.Config{
		{Provider: "p", URL: "ws://example.com/live", Trust: nativews.TrustLocal, Auth: nativews.AuthAnonymous, AllowedModels: []string{"m"}},
		{Provider: "p", URL: "wss://evil.example/live", Trust: nativews.TrustOfficial, OfficialHosts: []string{"api.example.com"}, Auth: nativews.AuthAnonymous, AllowedModels: []string{"m"}},
		{Provider: "p", URL: "wss://api.example.com/live", Trust: nativews.TrustOfficial, OfficialHosts: []string{"api.example.com"}, Auth: nativews.AuthAnonymous, AllowedModels: []string{"m"}, StaticHeaders: http.Header{"Authorization": {"steal"}}},
		{Provider: "p", URL: "wss://api.example.com/live", Trust: nativews.TrustOfficial, OfficialHosts: []string{"api.example.com"}, Auth: nativews.AuthAnonymous, AllowedModels: []string{"m", "m"}},
	}
	for index, config := range cases {
		if _, err := nativews.New(config); err == nil {
			t.Fatalf("unsafe config %d should fail", index)
		}
	}
	var nilSession realtime.Session
	_ = nilSession
}

func TestNativeWebSocketQueryAuthenticationIsInjectedAndRedacted(t *testing.T) {
	const secret = "query-secret"
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != secret || r.URL.Query().Get("model") != "m" {
			t.Errorf("query authentication/model missing: %s", r.URL.RawQuery)
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer server.Close()
	config := nativews.Config{
		Provider: "p", URL: "ws" + strings.TrimPrefix(server.URL, "http"), Trust: nativews.TrustLocal,
		Auth: nativews.AuthQuery, APIKey: secret, QueryName: "key", ModelQueryKey: "model", AllowedModels: []string{"m"},
	}
	p, err := nativews.New(config)
	if err != nil {
		t.Fatal(err)
	}
	session, err := p.Open(context.Background(), realtime.Request{Provider: "p", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	_ = session.Close()
	if strings.Contains(fmt.Sprintf("%#v", config), secret) {
		t.Fatal("query credential leaked through formatting")
	}
	config.QueryName = "model"
	if _, err := nativews.New(config); err == nil {
		t.Fatal("query credential and model key collision was accepted")
	}
}

func TestNativeWebSocketBindsConfigurationModelBeforeDial(t *testing.T) {
	config := nativews.Config{
		Provider: "gemini", URL: "wss://api.example.com/live", Trust: nativews.TrustOfficial,
		OfficialHosts: []string{"api.example.com"}, Auth: nativews.AuthBearer, APIKey: "key",
		ConfigurationModelPath: "setup.model", ConfigurationModelPrefix: "models/",
		AllowedModels: []string{"gemini-live"},
	}
	p, err := nativews.New(config)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`{"setup":{"model":"models/other"}}`,
		`{"setup":{}}`,
		`{}`,
	} {
		_, err := p.Open(context.Background(), realtime.Request{
			Provider: "gemini", Model: "gemini-live",
			Configuration: modelinvoker.NewRawPayload([]byte(raw)),
		})
		if err == nil || modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorMapping {
			t.Fatalf("configuration %s should fail model binding before dial: %v", raw, err)
		}
	}

	config.ConfigurationModelPath = "setup..model"
	if _, err := nativews.New(config); err == nil {
		t.Fatal("invalid configuration model path was accepted")
	}
	config.ConfigurationModelPath = "setup.model"
	config.ModelQueryKey = "model"
	if _, err := nativews.New(config); err == nil {
		t.Fatal("dual model owners were accepted")
	}
}

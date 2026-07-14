package realtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/realtime"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/realtime/nativews"
	"github.com/gorilla/websocket"
)

func TestConfigurationBoundRealtimeSessionCoversTextBinaryAndCloseWrite(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, setup, err := conn.ReadMessage()
		if err != nil || string(setup) != `{"setup":{"model":"models/live-model"}}` {
			t.Errorf("setup = %s, %v", setup, err)
			return
		}
		messageType, text, _ := conn.ReadMessage()
		var envelope map[string]string
		_ = json.Unmarshal(text, &envelope)
		if messageType != websocket.TextMessage || envelope["type"] != "input" || envelope["text"] != "hello" {
			t.Errorf("text frame = %d %s", messageType, text)
		}
		messageType, binary, _ := conn.ReadMessage()
		if messageType != websocket.BinaryMessage || string(binary) != "audio" {
			t.Errorf("binary frame = %d %s", messageType, binary)
		}
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"transcript","text":"heard"}`))
		_ = conn.WriteMessage(websocket.BinaryMessage, []byte("sound"))
		_, _, _ = conn.ReadMessage()
	}))
	defer server.Close()

	provider, err := nativews.New(nativews.Config{
		Provider: "live", URL: "ws" + strings.TrimPrefix(server.URL, "http"), Trust: nativews.TrustLocal, Auth: nativews.AuthAnonymous,
		ConfigurationModelPath: "setup.model", ConfigurationModelPrefix: "models/", AllowedModels: []string{"live-model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.ID() != "live" {
		t.Fatalf("provider ID = %q", provider.ID())
	}
	session, err := provider.Open(context.Background(), realtime.Request{
		Provider: "live", Model: "live-model", Modalities: []realtime.Modality{realtime.Text, realtime.Audio, realtime.Video},
		Configuration: modelinvoker.NewRawPayload([]byte(`{"setup":{"model":"models/live-model"}}`)),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err := session.Send(context.Background(), realtime.ClientEvent{Type: "input", Text: "hello"}); err != nil {
		t.Fatal(err)
	}
	if err := session.Send(context.Background(), realtime.ClientEvent{Binary: []byte("audio")}); err != nil {
		t.Fatal(err)
	}
	if !session.Next() || session.Event().Type != "transcript" || session.Event().Text != "heard" || session.Event().Sequence != 1 {
		t.Fatalf("text event = %+v err=%v", session.Event(), session.Err())
	}
	if !session.Next() || session.Event().Type != "binary" || string(session.Event().Binary) != "sound" || session.Event().Sequence != 2 {
		t.Fatalf("binary event = %+v err=%v", session.Event(), session.Err())
	}
	if err := session.Send(context.Background(), realtime.ClientEvent{Binary: []byte("x"), Text: "bad"}); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorInvalidRequest {
		t.Fatalf("mixed binary event error = %v", err)
	}
	if err := session.Send(context.Background(), realtime.ClientEvent{Raw: modelinvoker.NewRawPayload([]byte(`{`))}); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorInvalidRequest {
		t.Fatalf("invalid raw event error = %v", err)
	}
	if err := session.Send(nil, realtime.ClientEvent{Type: "x"}); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorInvalidRequest {
		t.Fatalf("nil context send error = %v", err)
	}
	if err := session.CloseWrite(); err != nil {
		t.Fatalf("close write failed: %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("idempotent close failed: %v", err)
	}
	if session.Next() {
		t.Fatal("closed session produced event")
	}
}

func TestRealtimeOpenRejectsRequestDriftBeforeNetwork(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	provider, err := nativews.New(nativews.Config{
		Provider: "p", URL: "ws" + strings.TrimPrefix(server.URL, "http"), Trust: nativews.TrustLocal,
		Auth: nativews.AuthAnonymous, ModelQueryKey: "model", AllowedModels: []string{"m"},
	})
	if err != nil {
		t.Fatal(err)
	}
	base := realtime.Request{Provider: "p", Model: "m", Modalities: []realtime.Modality{realtime.Audio}}
	tests := []struct {
		name    string
		ctx     context.Context
		request realtime.Request
	}{
		{"nil-context", nil, base},
		{"wrong-provider", context.Background(), func() realtime.Request { r := base; r.Provider = "other"; return r }()},
		{"empty-model", context.Background(), func() realtime.Request { r := base; r.Model = ""; return r }()},
		{"wrong-model", context.Background(), func() realtime.Request { r := base; r.Model = "other"; return r }()},
		{"options", context.Background(), func() realtime.Request {
			r := base
			r.ProviderOptions = modelinvoker.ProviderOptions{"p": []byte(`{}`)}
			return r
		}()},
		{"negative-timeout", context.Background(), func() realtime.Request { r := base; r.Timeout = -time.Second; return r }()},
		{"bad-config", context.Background(), func() realtime.Request {
			r := base
			r.Configuration = modelinvoker.NewRawPayload([]byte(`{`))
			return r
		}()},
		{"bad-modality", context.Background(), func() realtime.Request { r := base; r.Modalities = []realtime.Modality{"hologram"}; return r }()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := provider.Open(test.ctx, test.request); err == nil {
				t.Fatal("invalid realtime request succeeded")
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid requests reached network %d times", calls.Load())
	}
	var nilProvider *nativews.Provider
	if nilProvider.ID() != "" {
		t.Fatal("nil provider ID is not safe")
	}
	if _, err := nilProvider.Open(context.Background(), base); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorProviderUnavailable {
		t.Fatalf("nil provider open error = %v", err)
	}
}

func TestRealtimeConfigurationNegativeMatrixAndHandshakeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "no", http.StatusUnauthorized) }))
	defer server.Close()
	base := nativews.Config{
		Provider: "p", URL: "ws" + strings.TrimPrefix(server.URL, "http"), Trust: nativews.TrustLocal,
		Auth: nativews.AuthAnonymous, ModelQueryKey: "model", AllowedModels: []string{"m"},
	}
	mutate := func(change func(*nativews.Config)) nativews.Config {
		copy := base
		copy.AllowedModels = append([]string(nil), base.AllowedModels...)
		change(&copy)
		return copy
	}
	cases := []nativews.Config{
		mutate(func(c *nativews.Config) { c.Provider = "" }),
		mutate(func(c *nativews.Config) { c.URL = "ws://user:pass@127.0.0.1/live" }),
		mutate(func(c *nativews.Config) { c.Trust = nativews.TrustMode("bad") }),
		mutate(func(c *nativews.Config) { c.Auth = nativews.AuthBearer }),
		mutate(func(c *nativews.Config) { c.Auth = nativews.AuthHeader; c.APIKey = "key" }),
		mutate(func(c *nativews.Config) { c.Auth = nativews.AuthQuery; c.APIKey = "key"; c.QueryName = "bad name" }),
		mutate(func(c *nativews.Config) { c.Auth = nativews.AuthMode("bad") }),
		mutate(func(c *nativews.Config) { c.AllowedModels = nil }),
		mutate(func(c *nativews.Config) { c.AllowedModels = []string{"m", "m"} }),
		mutate(func(c *nativews.Config) { c.AllowedModels = []string{"bad\nmodel"} }),
		mutate(func(c *nativews.Config) { c.ModelQueryKey = "bad key" }),
		mutate(func(c *nativews.Config) { c.ModelQueryKey = "" }),
		mutate(func(c *nativews.Config) { c.StaticHeaders = http.Header{"Origin": {"steal"}} }),
		mutate(func(c *nativews.Config) { c.StaticHeaders = http.Header{"X-Test": {"bad\nvalue"}} }),
	}
	for index, config := range cases {
		if _, err := nativews.New(config); err == nil {
			t.Fatalf("invalid realtime config %d accepted", index)
		}
	}
	const secret = "realtime-format-secret"
	if formatted := fmt.Sprintf("%v %#v", nativews.Config{APIKey: secret}, nativews.Config{APIKey: secret}); strings.Contains(formatted, secret) {
		t.Fatal("realtime config formatting leaked credential")
	}
	provider, err := nativews.New(base)
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Open(context.Background(), realtime.Request{Provider: "p", Model: "m"})
	var typed *modelinvoker.Error
	if !errors.As(err, &typed) || typed.HTTPStatus != http.StatusUnauthorized {
		t.Fatalf("handshake error was not normalized: %T %v", err, err)
	}
}

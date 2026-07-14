package realtime_test

import (
	"context"
	"errors"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/realtime"
)

type invokerProvider struct {
	id      modelinvoker.ProviderID
	open    func(context.Context, realtime.Request) (realtime.Session, error)
	session realtime.Session
}

func (provider *invokerProvider) ID() modelinvoker.ProviderID { return provider.id }
func (provider *invokerProvider) Open(ctx context.Context, request realtime.Request) (realtime.Session, error) {
	if provider.open != nil {
		return provider.open(ctx, request)
	}
	return provider.session, nil
}

type invokerSession struct {
	events    []realtime.ServerEvent
	index     int
	err       error
	sent      realtime.ClientEvent
	closed    int
	writeDone bool
}

func (session *invokerSession) Send(_ context.Context, event realtime.ClientEvent) error {
	session.sent = event
	return nil
}
func (session *invokerSession) Next() bool {
	if session.index >= len(session.events) {
		return false
	}
	session.index++
	return true
}
func (session *invokerSession) Event() realtime.ServerEvent {
	return session.events[session.index-1]
}
func (session *invokerSession) Err() error        { return session.err }
func (session *invokerSession) CloseWrite() error { session.writeDone = true; return nil }
func (session *invokerSession) Close() error      { session.closed++; return nil }

func TestRealtimeInvokerOwnsSelectionValidationAndEventProjection(t *testing.T) {
	inner := &invokerSession{events: []realtime.ServerEvent{{Type: "audio.delta", Sequence: 99, Binary: []byte("voice"), Raw: modelinvoker.NewRawPayload([]byte(`{"native":true}`))}}}
	provider := &invokerProvider{id: "voice", session: inner}
	registry, err := realtime.NewRegistry(provider)
	if err != nil {
		t.Fatal(err)
	}
	invoker, _ := realtime.NewInvoker(registry)
	session, err := invoker.Open(context.Background(), realtime.Request{Provider: "voice", Model: "voice-1", Modalities: []realtime.Modality{realtime.Audio}})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("input")
	if err := session.Send(context.Background(), realtime.ClientEvent{Binary: payload}); err != nil {
		t.Fatal(err)
	}
	payload[0] = 'X'
	if string(inner.sent.Binary) != "input" {
		t.Fatal("realtime invoker aliased caller-owned event bytes")
	}
	if !session.Next() {
		t.Fatalf("session did not yield event: %v", session.Err())
	}
	event := session.Event()
	if event.Sequence != 1 || event.Type != "audio.delta" || string(event.Binary) != "voice" {
		t.Fatalf("event was not projected into stable sequence: %+v", event)
	}
	event.Binary[0] = 'X'
	if string(session.Event().Binary) != "voice" {
		t.Fatal("realtime event accessor aliased provider-owned bytes")
	}
	if err := session.CloseWrite(); err != nil || !inner.writeDone {
		t.Fatalf("close-write did not delegate: %v", err)
	}
	if err := session.Close(); err != nil || inner.closed != 1 {
		t.Fatalf("close did not delegate exactly once: %v count=%d", err, inner.closed)
	}
	if err := session.Close(); err != nil || inner.closed != 1 {
		t.Fatalf("close is not idempotent: %v count=%d", err, inner.closed)
	}
	if err := session.Send(context.Background(), realtime.ClientEvent{Binary: []byte("late")}); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorCancelled {
		t.Fatalf("send after close was not rejected: %v", err)
	}
}

func TestRealtimeInvokerFailsClosedOnRegistrySessionAndContextErrors(t *testing.T) {
	var typedNil *invokerProvider
	if _, err := realtime.NewRegistry(typedNil); err == nil {
		t.Fatal("typed nil provider was accepted")
	}
	provider := &invokerProvider{id: "p", session: (*invokerSession)(nil)}
	registry, _ := realtime.NewRegistry(provider)
	if err := registry.Register(provider); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorDuplicateProvider {
		t.Fatalf("duplicate provider was not rejected: %v", err)
	}
	if ids := registry.IDs(); len(ids) != 1 || ids[0] != "p" {
		t.Fatalf("registry IDs drifted: %v", ids)
	}
	invoker, _ := realtime.NewInvoker(registry)
	if _, err := invoker.Open(context.Background(), realtime.Request{Provider: "p", Model: "m"}); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorProviderUnavailable {
		t.Fatalf("typed nil session was not rejected: %v", err)
	}
	if _, err := invoker.Open(context.Background(), realtime.Request{Provider: "unknown", Model: "m"}); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorUnknownProvider {
		t.Fatalf("unknown provider was not rejected: %v", err)
	}
	if _, err := invoker.Open(nil, realtime.Request{Provider: "p", Model: "m"}); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorInvalidRequest {
		t.Fatalf("nil context was not rejected: %v", err)
	}

	provider.session = &invokerSession{}
	provider.open = func(ctx context.Context, _ realtime.Request) (realtime.Session, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if _, err := invoker.Open(context.Background(), realtime.Request{Provider: "p", Model: "m", Timeout: time.Millisecond}); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorTimeout {
		t.Fatalf("timeout was not normalized: %v", err)
	}
	provider.open = func(context.Context, realtime.Request) (realtime.Session, error) {
		return nil, errors.New("opaque")
	}
	if _, err := invoker.Open(context.Background(), realtime.Request{Provider: "p", Model: "m"}); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorProvider {
		t.Fatalf("opaque provider error was not normalized: %v", err)
	}
}

func TestRealtimeRequestAndClientEventValidationAreProviderNeutral(t *testing.T) {
	invalid := []realtime.Request{
		{},
		{Provider: "p", Model: " bad"},
		{Provider: "p", Model: "m", Timeout: -time.Second},
		{Provider: "p", Model: "m", Modalities: []realtime.Modality{realtime.Audio, realtime.Audio}},
		{Provider: "p", Model: "m", Modalities: []realtime.Modality{"hologram"}},
		{Provider: "p", Model: "m", Configuration: modelinvoker.NewRawPayload([]byte(`{`))},
		{Provider: "p", Model: "m", ProviderOptions: modelinvoker.ProviderOptions{"other": []byte(`{}`)}},
	}
	for index, request := range invalid {
		if err := request.Validate(); err == nil {
			t.Fatalf("invalid request %d was accepted", index)
		}
	}
}

func TestRealtimeInvokerRejectsProviderEventSemanticDrift(t *testing.T) {
	events := []realtime.ServerEvent{
		{},
		{Type: "bad\nevent"},
		{Type: "error", Error: &modelinvoker.Error{Provider: "other", Kind: modelinvoker.ErrorProvider}},
		{Type: "usage", Usage: &modelinvoker.Usage{InputTokens: -1}},
	}
	for index, event := range events {
		inner := &invokerSession{events: []realtime.ServerEvent{event}}
		provider := &invokerProvider{id: "p", session: inner}
		registry, _ := realtime.NewRegistry(provider)
		invoker, _ := realtime.NewInvoker(registry)
		session, err := invoker.Open(context.Background(), realtime.Request{Provider: "p", Model: "m"})
		if err != nil {
			t.Fatal(err)
		}
		if session.Next() || modelinvoker.ErrorKindOf(session.Err()) != modelinvoker.ErrorMapping {
			t.Fatalf("provider event drift %d was not rejected: event=%+v err=%v", index, session.Event(), session.Err())
		}
	}
}

func TestRealtimeInvokerIsolatesCallerRequestState(t *testing.T) {
	provider := &invokerProvider{id: "p", session: &invokerSession{}}
	provider.open = func(_ context.Context, request realtime.Request) (realtime.Session, error) {
		request.Modalities[0] = realtime.Video
		request.Metadata["trace"] = "provider-mutated"
		request.ProviderOptions["p"][0] = 'X'
		return provider.session, nil
	}
	registry, _ := realtime.NewRegistry(provider)
	invoker, _ := realtime.NewInvoker(registry)
	modalities := []realtime.Modality{realtime.Audio}
	metadata := modelinvoker.Metadata{"trace": "original"}
	options := modelinvoker.ProviderOptions{"p": []byte(`{"mode":"original"}`)}
	session, err := invoker.Open(context.Background(), realtime.Request{
		Provider: "p", Model: "m", Modalities: modalities, Metadata: metadata, ProviderOptions: options,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if modalities[0] != realtime.Audio || metadata["trace"] != "original" || string(options["p"]) != `{"mode":"original"}` {
		t.Fatal("realtime provider mutated caller-owned request state")
	}
}

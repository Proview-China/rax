package realtime

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

type Invoker struct{ registry *Registry }

func NewInvoker(registry *Registry) (*Invoker, error) {
	if registry == nil {
		return nil, realtimeError("", modelinvoker.ErrorInvalidRequest, "new_invoker", "realtime registry is required", nil)
	}
	return &Invoker{registry: registry}, nil
}

func (invoker *Invoker) Open(ctx context.Context, request Request) (Session, error) {
	if invoker == nil || invoker.registry == nil {
		return nil, realtimeError(request.Provider, modelinvoker.ErrorInvalidRequest, "open", "realtime invoker is not initialized", nil)
	}
	if ctx == nil {
		return nil, realtimeError(request.Provider, modelinvoker.ErrorInvalidRequest, "open", "context is nil", nil)
	}
	if err := request.Validate(); err != nil {
		return nil, realtimeError(request.Provider, modelinvoker.ErrorInvalidRequest, "validate", err.Error(), nil)
	}
	request = cloneRequest(request)
	provider, err := invoker.registry.Get(request.Provider)
	if err != nil {
		return nil, err
	}
	call := ctx
	cancel := func() {}
	if request.Timeout > 0 {
		call, cancel = context.WithTimeout(ctx, request.Timeout)
	}
	session, err := provider.Open(call, request)
	if err != nil {
		normalized := normalizeRealtimeError(call, request.Provider, "open", err)
		cancel()
		return nil, normalized
	}
	cancel()
	if nilInterface(session) {
		return nil, realtimeError(request.Provider, modelinvoker.ErrorProviderUnavailable, "open", "realtime provider returned a nil session", nil)
	}
	return &boundSession{provider: request.Provider, inner: session}, nil
}

type boundSession struct {
	provider modelinvoker.ProviderID
	inner    Session
	event    ServerEvent
	sequence int64
	err      error
	closed   atomic.Bool
}

func (session *boundSession) Send(ctx context.Context, event ClientEvent) error {
	if session == nil || nilInterface(session.inner) {
		return realtimeError("", modelinvoker.ErrorProviderUnavailable, "send", "realtime session is not initialized", nil)
	}
	if session.closed.Load() {
		return realtimeError(session.provider, modelinvoker.ErrorCancelled, "send", "realtime session is closed", nil)
	}
	if ctx == nil {
		return realtimeError(session.provider, modelinvoker.ErrorInvalidRequest, "send", "context is nil", nil)
	}
	if err := validateClientEvent(event); err != nil {
		return realtimeError(session.provider, modelinvoker.ErrorInvalidRequest, "send", err.Error(), nil)
	}
	return normalizeRealtimeError(ctx, session.provider, "send", session.inner.Send(ctx, cloneClientEvent(event)))
}

func (session *boundSession) Next() bool {
	if session == nil || nilInterface(session.inner) || session.closed.Load() || session.err != nil {
		return false
	}
	if !session.inner.Next() {
		session.err = normalizeRealtimeError(context.Background(), session.provider, "receive", session.inner.Err())
		return false
	}
	event := cloneServerEvent(session.inner.Event())
	if err := validateServerEvent(&event, session.provider); err != nil {
		session.err = err
		return false
	}
	session.sequence++
	event.Sequence = session.sequence
	session.event = event
	return true
}

func validateServerEvent(event *ServerEvent, provider modelinvoker.ProviderID) error {
	if event == nil || event.Type == "" || len(event.Type) > 512 || strings.ContainsAny(event.Type, "\r\n\x00") {
		return realtimeError(provider, modelinvoker.ErrorMapping, "receive", "realtime provider emitted an invalid event type", nil)
	}
	if event.Error != nil {
		if event.Error.Provider != "" && event.Error.Provider != provider {
			return realtimeError(provider, modelinvoker.ErrorMapping, "receive", "realtime event error provider does not match the selected provider", nil)
		}
		if event.Error.Provider == "" {
			event.Error.Provider = provider
		}
	}
	if event.Usage != nil && (event.Usage.InputTokens < 0 || event.Usage.OutputTokens < 0 ||
		event.Usage.ReasoningTokens < 0 || event.Usage.CacheReadTokens < 0 ||
		event.Usage.CacheWriteTokens < 0 || event.Usage.TotalTokens < 0) {
		return realtimeError(provider, modelinvoker.ErrorMapping, "receive", "realtime event usage must not be negative", nil)
	}
	return nil
}

func (session *boundSession) Event() ServerEvent {
	if session == nil {
		return ServerEvent{}
	}
	return cloneServerEvent(session.event)
}

func (session *boundSession) Err() error {
	if session == nil {
		return nil
	}
	return session.err
}

func (session *boundSession) CloseWrite() error {
	if session == nil || nilInterface(session.inner) {
		return nil
	}
	if session.closed.Load() {
		return nil
	}
	return normalizeRealtimeError(context.Background(), session.provider, "close_write", session.inner.CloseWrite())
}

func (session *boundSession) Close() error {
	if session == nil {
		return nil
	}
	if !session.closed.CompareAndSwap(false, true) {
		return nil
	}
	if nilInterface(session.inner) {
		return nil
	}
	return normalizeRealtimeError(context.Background(), session.provider, "close", session.inner.Close())
}

func cloneClientEvent(event ClientEvent) ClientEvent {
	event.Binary = append([]byte(nil), event.Binary...)
	event.Raw = modelinvoker.NewRawPayload(event.Raw.Bytes())
	return event
}

func cloneRequest(request Request) Request {
	request.Modalities = append([]Modality(nil), request.Modalities...)
	request.Configuration = modelinvoker.NewRawPayload(request.Configuration.Bytes())
	if request.Metadata != nil {
		metadata := make(modelinvoker.Metadata, len(request.Metadata))
		for key, value := range request.Metadata {
			metadata[key] = value
		}
		request.Metadata = metadata
	}
	if request.ProviderOptions != nil {
		options := make(modelinvoker.ProviderOptions, len(request.ProviderOptions))
		for key, value := range request.ProviderOptions {
			options[key] = append([]byte(nil), value...)
		}
		request.ProviderOptions = options
	}
	return request
}

func cloneServerEvent(event ServerEvent) ServerEvent {
	event.Binary = append([]byte(nil), event.Binary...)
	event.Raw = modelinvoker.NewRawPayload(event.Raw.Bytes())
	if event.Usage != nil {
		usage := *event.Usage
		event.Usage = &usage
	}
	if event.Error != nil {
		typed := *event.Error
		event.Error = &typed
	}
	return event
}

func normalizeRealtimeError(ctx context.Context, provider modelinvoker.ProviderID, operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || (ctx != nil && errors.Is(ctx.Err(), context.DeadlineExceeded)) {
		return realtimeError(provider, modelinvoker.ErrorTimeout, operation, "realtime operation timed out", err)
	}
	if errors.Is(err, context.Canceled) || (ctx != nil && errors.Is(ctx.Err(), context.Canceled)) {
		return realtimeError(provider, modelinvoker.ErrorCancelled, operation, "realtime operation was cancelled", err)
	}
	var typed *modelinvoker.Error
	if errors.As(err, &typed) {
		return typed
	}
	return realtimeError(provider, modelinvoker.ErrorProvider, operation, "realtime provider failed", err)
}

func realtimeError(provider modelinvoker.ProviderID, kind modelinvoker.ErrorKind, operation, message string, err error) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: kind, Provider: provider, Operation: operation, Message: message, Err: err}
}

var _ Session = (*boundSession)(nil)

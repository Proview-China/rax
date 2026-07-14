package localcompat

import (
	"context"
	"errors"
	"fmt"
	"io"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/compatprovider"
)

type Adapter struct{ inner *compatprovider.Adapter }

func (Adapter) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "localcompat.Adapter([REDACTED])")
}
func (Adapter) GoString() string { return "localcompat.Adapter([REDACTED])" }

func New(config Config) (*Adapter, error) {
	redactor := adaptercore.NewRedactor(config.APIKey)
	base, err := config.validate()
	if err != nil {
		return nil, redactor.Error(localError(config.providerID(), modelinvoker.ErrorInvalidRequest, "configure", err.Error()))
	}
	provider := config.providerID()
	dialect := localDialect{provider: provider, protocol: config.Protocol, models: append([]string(nil), config.AllowedModels...)}
	compatible := compatprovider.Config{
		Provider:         provider,
		DefaultProtocol:  config.Protocol,
		APIKey:           config.APIKey,
		AllowAnonymous:   config.APIKey == "",
		HTTPClient:       config.HTTPClient,
		UserAgent:        config.UserAgent,
		Capabilities:     capabilityContract(provider, config.Protocol, config.AllowedModels, config.SupportedCapabilities),
		RequestIDHeaders: []string{"x-request-id", "request-id"},
	}
	switch config.Protocol {
	case modelinvoker.ProtocolChatCompletions:
		compatible.ChatEndpoint, compatible.ChatDialect = base, dialect
	case modelinvoker.ProtocolResponses:
		compatible.ResponsesEndpoint, compatible.ResponsesDialect = base, dialect
	}
	inner, err := compatprovider.New(compatible)
	if err != nil {
		return nil, redactor.Error(err)
	}
	return &Adapter{inner: inner}, nil
}

func (a *Adapter) ID() modelinvoker.ProviderID {
	if a == nil || a.inner == nil {
		return ""
	}
	return a.inner.ID()
}
func (a *Adapter) DefaultProtocol() modelinvoker.Protocol {
	if a == nil || a.inner == nil {
		return modelinvoker.ProtocolAuto
	}
	return a.inner.DefaultProtocol()
}
func (a *Adapter) CandidateBindingEndpoint(protocolID modelinvoker.Protocol, requested string) (string, bool) {
	if a == nil || a.inner == nil {
		return "", false
	}
	return a.inner.CandidateBindingEndpoint(protocolID, requested)
}
func (a *Adapter) Capabilities(ctx context.Context, query modelinvoker.CapabilityQuery) (modelinvoker.CapabilityContract, error) {
	if a == nil || a.inner == nil {
		return nil, localError("", modelinvoker.ErrorProviderUnavailable, "capabilities", "adapter is not initialized")
	}
	return a.inner.Capabilities(ctx, query)
}
func (a *Adapter) Invoke(ctx context.Context, request modelinvoker.Request) (modelinvoker.Response, error) {
	if a == nil || a.inner == nil {
		return modelinvoker.Response{}, localError("", modelinvoker.ErrorProviderUnavailable, "invoke", "adapter is not initialized")
	}
	response, err := a.inner.Invoke(ctx, request)
	if err == nil {
		if modelErr := exactModelError(a.ID(), request.Model, response.Model); modelErr != nil {
			return modelinvoker.Response{}, modelErr
		}
	}
	return response, err
}
func (a *Adapter) Stream(ctx context.Context, request modelinvoker.Request) (modelinvoker.Stream, error) {
	if a == nil || a.inner == nil {
		return nil, localError("", modelinvoker.ErrorProviderUnavailable, "stream", "adapter is not initialized")
	}
	stream, err := a.inner.Stream(ctx, request)
	if err != nil {
		return nil, err
	}
	return &modelStream{inner: stream, provider: a.ID(), requested: request.Model}, nil
}

func localError(provider modelinvoker.ProviderID, kind modelinvoker.ErrorKind, operation, message string) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: kind, Provider: provider, Operation: operation, Message: message}
}

func exactModelError(provider modelinvoker.ProviderID, requested, actual string) *modelinvoker.Error {
	if actual == requested {
		return nil
	}
	code, message := "response_model_mismatch", "local response model does not match the exact selected model"
	if actual == "" {
		code, message = "response_model_missing", "local response is missing the exact selected model"
	}
	return &modelinvoker.Error{Kind: modelinvoker.ErrorMapping, Provider: provider, Operation: "validate_response", Code: code, Message: message}
}

type modelStream struct {
	inner       modelinvoker.Stream
	provider    modelinvoker.ProviderID
	requested   string
	event       modelinvoker.StreamEvent
	err         error
	done        bool
	closed      bool
	innerClosed bool
}

func (stream *modelStream) Next() bool {
	if stream == nil || stream.inner == nil || stream.done || stream.closed {
		return false
	}
	if !stream.inner.Next() {
		stream.done = true
		stream.err = stream.inner.Err()
		return false
	}
	stream.event = stream.inner.Event()
	if stream.event.Response != nil {
		if modelErr := exactModelError(stream.provider, stream.requested, stream.event.Response.Model); modelErr != nil {
			stream.event = modelinvoker.StreamEvent{
				Type: modelinvoker.StreamEventError, Sequence: stream.event.Sequence,
				ResponseID: stream.event.ResponseID, Error: modelErr,
			}
			stream.err = modelErr
			stream.done = true
			if closeErr := stream.inner.Close(); closeErr != nil {
				stream.err = errors.Join(stream.err, closeErr)
			}
			stream.innerClosed = true
			return true
		}
	}
	return true
}
func (stream *modelStream) Event() modelinvoker.StreamEvent {
	if stream == nil {
		return modelinvoker.StreamEvent{}
	}
	return stream.event
}
func (stream *modelStream) Err() error {
	if stream == nil {
		return nil
	}
	return stream.err
}
func (stream *modelStream) Close() error {
	if stream == nil || stream.closed {
		return nil
	}
	stream.closed = true
	if stream.inner == nil || stream.innerClosed {
		return nil
	}
	stream.innerClosed = true
	return stream.inner.Close()
}

var _ modelinvoker.Provider = (*Adapter)(nil)
var _ adaptercore.CandidateBindingReceipt = (*Adapter)(nil)
var _ modelinvoker.Stream = (*modelStream)(nil)

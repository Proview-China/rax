package localcompat

import (
	"context"
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
	return a.inner.Invoke(ctx, request)
}
func (a *Adapter) Stream(ctx context.Context, request modelinvoker.Request) (modelinvoker.Stream, error) {
	if a == nil || a.inner == nil {
		return nil, localError("", modelinvoker.ErrorProviderUnavailable, "stream", "adapter is not initialized")
	}
	return a.inner.Stream(ctx, request)
}

func localError(provider modelinvoker.ProviderID, kind modelinvoker.ErrorKind, operation, message string) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: kind, Provider: provider, Operation: operation, Message: message}
}

var _ modelinvoker.Provider = (*Adapter)(nil)
var _ adaptercore.CandidateBindingReceipt = (*Adapter)(nil)

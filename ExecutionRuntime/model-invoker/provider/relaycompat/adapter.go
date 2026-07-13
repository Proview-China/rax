package relaycompat

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
	_, _ = io.WriteString(state, "relaycompat.Adapter([REDACTED])")
}
func (Adapter) GoString() string { return "relaycompat.Adapter([REDACTED])" }

func New(config Config) (*Adapter, error) {
	redactor := adaptercore.NewRedactor(config.APIKey)
	if err := config.validate(); err != nil {
		return nil, redactor.Error(providerError(modelinvoker.ErrorInvalidRequest, "configure", err.Error()))
	}
	base, _ := trustedRelayBaseURL(config.BaseURL)
	dialect := relayDialect{protocol: config.Protocol, allowedModels: append([]string(nil), config.AllowedModels...)}
	compatible := compatprovider.Config{
		Provider: ProviderID, DefaultProtocol: config.Protocol, APIKey: config.APIKey,
		HTTPClient: config.HTTPClient, UserAgent: config.UserAgent,
		Capabilities:     capabilityContract(config.Protocol, config.AllowedModels),
		RequestIDHeaders: []string{"x-request-id", "request-id", "x-goog-request-id", "cf-ray"},
	}
	switch config.Protocol {
	case modelinvoker.ProtocolChatCompletions:
		compatible.ChatEndpoint, compatible.ChatDialect = base, dialect
	case modelinvoker.ProtocolResponses:
		compatible.ResponsesEndpoint, compatible.ResponsesDialect = base, dialect
	case modelinvoker.ProtocolMessages:
		compatible.MessagesEndpoint, compatible.MessagesDialect = base, dialect
		compatible.MessagesAuthToken = config.MessagesAuthToken
	case modelinvoker.ProtocolGenerateContent:
		compatible.GenerateEndpoint = config.endpoint()
		compatible.GenerateBaseURL = base + "/"
		compatible.GenerateAPIVersion = config.APIVersion
		compatible.GenerateDialect = dialect
	}
	inner, err := compatprovider.New(compatible)
	if err != nil {
		return nil, redactor.Error(err)
	}
	return &Adapter{inner: inner}, nil
}

func (adapter *Adapter) ID() modelinvoker.ProviderID { return ProviderID }
func (adapter *Adapter) DefaultProtocol() modelinvoker.Protocol {
	if adapter == nil || adapter.inner == nil {
		return modelinvoker.ProtocolAuto
	}
	return adapter.inner.DefaultProtocol()
}
func (adapter *Adapter) CandidateBindingEndpoint(protocol modelinvoker.Protocol, requested string) (string, bool) {
	if adapter == nil || adapter.inner == nil {
		return "", false
	}
	return adapter.inner.CandidateBindingEndpoint(protocol, requested)
}
func (adapter *Adapter) Capabilities(ctx context.Context, query modelinvoker.CapabilityQuery) (modelinvoker.CapabilityContract, error) {
	if adapter == nil || adapter.inner == nil {
		return nil, providerError(modelinvoker.ErrorProviderUnavailable, "capabilities", "adapter is not initialized")
	}
	return adapter.inner.Capabilities(ctx, query)
}
func (adapter *Adapter) Invoke(ctx context.Context, request modelinvoker.Request) (modelinvoker.Response, error) {
	if adapter == nil || adapter.inner == nil {
		return modelinvoker.Response{}, providerError(modelinvoker.ErrorProviderUnavailable, "invoke", "adapter is not initialized")
	}
	return adapter.inner.Invoke(ctx, request)
}
func (adapter *Adapter) Stream(ctx context.Context, request modelinvoker.Request) (modelinvoker.Stream, error) {
	if adapter == nil || adapter.inner == nil {
		return nil, providerError(modelinvoker.ErrorProviderUnavailable, "stream", "adapter is not initialized")
	}
	return adapter.inner.Stream(ctx, request)
}

func providerError(kind modelinvoker.ErrorKind, operation, message string) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: kind, Provider: ProviderID, Operation: operation, Message: message}
}

var _ modelinvoker.Provider = (*Adapter)(nil)
var _ adaptercore.CandidateBindingReceipt = (*Adapter)(nil)

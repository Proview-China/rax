package plancompat

import (
	"context"
	"fmt"
	"io"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/compatprovider"
)

type Adapter struct {
	kind  Kind
	inner *compatprovider.Adapter
}

func (Adapter) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "plancompat.Adapter([REDACTED])")
}
func (Adapter) GoString() string { return "plancompat.Adapter([REDACTED])" }

func New(config Config) (*Adapter, error) {
	if err := config.validate(); err != nil {
		return nil, &modelinvoker.Error{Kind: modelinvoker.ErrorInvalidRequest, Provider: providerID(config.Kind), Operation: "configure", Message: err.Error()}
	}
	endpoint, _ := config.trustedEndpoint()
	config.BaseURL = endpoint
	allowedModels := exactModels(config.Profile)
	dialect := planDialect{kind: config.Kind, protocol: config.Protocol, allowedModels: allowedModels}
	compatible := compatprovider.Config{
		Provider: providerID(config.Kind), DefaultProtocol: config.Protocol, APIKey: config.APIKey,
		HTTPClient: config.HTTPClient, Capabilities: capabilityContract(config.Kind, allowedModels), UserAgent: config.UserAgent,
		RequestIDHeaders: []string{"x-request-id", "request-id", "x-dashscope-request-id"},
	}
	switch config.Protocol {
	case modelinvoker.ProtocolChatCompletions:
		compatible.ChatEndpoint, compatible.ChatDialect = config.BaseURL, dialect
	case modelinvoker.ProtocolMessages:
		compatible.MessagesEndpoint, compatible.MessagesDialect = config.BaseURL, dialect
		compatible.MessagesAuthToken = config.Kind == MiMoTokenPlan || config.Kind == AlibabaPlan
	}
	inner, err := compatprovider.New(compatible)
	if err != nil {
		return nil, err
	}
	return &Adapter{kind: config.Kind, inner: inner}, nil
}

func (adapter *Adapter) ID() modelinvoker.ProviderID {
	if adapter == nil {
		return ""
	}
	return providerID(adapter.kind)
}
func (adapter *Adapter) DefaultProtocol() modelinvoker.Protocol {
	if adapter == nil || adapter.inner == nil {
		return modelinvoker.ProtocolAuto
	}
	return adapter.inner.DefaultProtocol()
}
func (adapter *Adapter) CandidateBindingEndpoint(protocolID modelinvoker.Protocol, requested string) (string, bool) {
	if adapter == nil || adapter.inner == nil {
		return "", false
	}
	return adapter.inner.CandidateBindingEndpoint(protocolID, requested)
}
func (adapter *Adapter) Capabilities(ctx context.Context, query modelinvoker.CapabilityQuery) (modelinvoker.CapabilityContract, error) {
	if adapter == nil || adapter.inner == nil {
		return nil, providerError("", modelinvoker.ErrorProviderUnavailable, "capabilities", "adapter is not initialized")
	}
	return adapter.inner.Capabilities(ctx, query)
}
func (adapter *Adapter) Invoke(ctx context.Context, request modelinvoker.Request) (modelinvoker.Response, error) {
	if adapter == nil || adapter.inner == nil {
		return modelinvoker.Response{}, providerError("", modelinvoker.ErrorProviderUnavailable, "invoke", "adapter is not initialized")
	}
	return adapter.inner.Invoke(ctx, request)
}
func (adapter *Adapter) Stream(ctx context.Context, request modelinvoker.Request) (modelinvoker.Stream, error) {
	if adapter == nil || adapter.inner == nil {
		return nil, providerError("", modelinvoker.ErrorProviderUnavailable, "stream", "adapter is not initialized")
	}
	return adapter.inner.Stream(ctx, request)
}

func providerError(provider modelinvoker.ProviderID, kind modelinvoker.ErrorKind, operation, message string) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: kind, Provider: provider, Operation: operation, Message: message}
}

var _ modelinvoker.Provider = (*Adapter)(nil)
var _ adaptercore.CandidateBindingReceipt = (*Adapter)(nil)

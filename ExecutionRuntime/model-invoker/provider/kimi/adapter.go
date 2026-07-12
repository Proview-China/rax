package kimi

import (
	"context"
	"fmt"
	"io"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/compatprovider"
)

const ProviderID modelinvoker.ProviderID = "kimi"

type Adapter struct{ inner *compatprovider.Adapter }

func (Adapter) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "kimi.Adapter([REDACTED])")
}
func (Adapter) GoString() string { return "kimi.Adapter([REDACTED])" }

func New(config Config) (*Adapter, error) {
	endpoint, err := config.endpoint()
	if err != nil {
		return nil, err
	}
	inner, err := compatprovider.New(compatprovider.Config{
		Provider: ProviderID, DefaultProtocol: modelinvoker.ProtocolChatCompletions,
		APIKey: config.APIKey, HTTPClient: config.HTTPClient, ChatEndpoint: endpoint,
		ChatDialect: chatDialect{}, Capabilities: capabilityContract,
		RequestIDHeaders: []string{"x-request-id", "request-id"},
	})
	if err != nil {
		return nil, err
	}
	return &Adapter{inner: inner}, nil
}

func (a *Adapter) ID() modelinvoker.ProviderID { return ProviderID }
func (a *Adapter) DefaultProtocol() modelinvoker.Protocol {
	return modelinvoker.ProtocolChatCompletions
}
func (a *Adapter) CandidateBindingEndpoint(protocolID modelinvoker.Protocol, requested string) (string, bool) {
	if a == nil || a.inner == nil {
		return "", false
	}
	return a.inner.CandidateBindingEndpoint(protocolID, requested)
}
func (a *Adapter) Capabilities(ctx context.Context, q modelinvoker.CapabilityQuery) (modelinvoker.CapabilityContract, error) {
	if a == nil || a.inner == nil {
		return nil, providerError(modelinvoker.ErrorProviderUnavailable, "capabilities", "adapter is not initialized")
	}
	return a.inner.Capabilities(ctx, q)
}
func (a *Adapter) Invoke(ctx context.Context, r modelinvoker.Request) (modelinvoker.Response, error) {
	if a == nil || a.inner == nil {
		return modelinvoker.Response{}, providerError(modelinvoker.ErrorProviderUnavailable, "invoke", "adapter is not initialized")
	}
	return a.inner.Invoke(ctx, r)
}
func (a *Adapter) Stream(ctx context.Context, r modelinvoker.Request) (modelinvoker.Stream, error) {
	if a == nil || a.inner == nil {
		return nil, providerError(modelinvoker.ErrorProviderUnavailable, "stream", "adapter is not initialized")
	}
	return a.inner.Stream(ctx, r)
}

func capabilityContract(_ context.Context, query modelinvoker.CapabilityQuery) (modelinvoker.CapabilityContract, error) {
	kind := modelKind(query.Model)
	if kind == modelUnknown {
		return nil, providerError(modelinvoker.ErrorMapping, "capabilities", "model must be a current approved Kimi Open Platform text model")
	}
	if query.Protocol != modelinvoker.ProtocolChatCompletions {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "capabilities", "Kimi Open Platform only exposes the approved Chat binding")
	}
	contract := adaptercore.UnsupportedContract("outside the current Kimi Open Platform text-only Chat slice")
	adaptercore.SetSupport(contract, query, modelinvoker.SupportCompatible, "mapped through Kimi's documented Chat Completions contract",
		modelinvoker.CapabilityTextGeneration, modelinvoker.CapabilityStreaming, modelinvoker.CapabilityUsageReporting)
	contract[modelinvoker.CapabilityStructuredOutput] = adaptercore.QuerySupport(query, modelinvoker.SupportPartial, "JSON Object is supported; strict JSON Schema and Partial Mode are not")
	if kind == modelK2 {
		adaptercore.SetSupport(contract, query, modelinvoker.SupportCompatible, "supported by the selected Kimi K2 model",
			modelinvoker.CapabilityToolCalling, modelinvoker.CapabilityParallelToolCalling, modelinvoker.CapabilityReasoning, modelinvoker.CapabilityReasoningSummary)
		contract[modelinvoker.CapabilityProviderContinuation] = adaptercore.QuerySupport(query, modelinvoker.SupportUnsupported, "preserved thinking cannot yet be represented in portable Chat input")
	}
	return contract, nil
}

func providerError(kind modelinvoker.ErrorKind, op, message string) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: kind, Provider: ProviderID, Operation: op, Message: message}
}

var _ modelinvoker.Provider = (*Adapter)(nil)
var _ adaptercore.CandidateBindingReceipt = (*Adapter)(nil)

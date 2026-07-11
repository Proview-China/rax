package zai

import (
	"context"
	"fmt"
	"io"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/compatprovider"
)

const ProviderID modelinvoker.ProviderID = "zai"

type Adapter struct{ inner *compatprovider.Adapter }

func (Adapter) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "zai.Adapter([REDACTED])")
}
func (Adapter) GoString() string { return "zai.Adapter([REDACTED])" }

func New(config Config) (*Adapter, error) {
	inner, err := compatprovider.New(compatprovider.Config{
		Provider: ProviderID, DefaultProtocol: modelinvoker.ProtocolChatCompletions,
		APIKey: config.APIKey, HTTPClient: config.HTTPClient, ChatEndpoint: config.endpoint(),
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
	if !supportedModel(query.Model) {
		return nil, providerError(modelinvoker.ErrorMapping, "capabilities", "model must be a current approved Z.AI text model")
	}
	if query.Protocol != modelinvoker.ProtocolChatCompletions {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "capabilities", "Z.AI only exposes the approved Chat binding")
	}
	contract := adaptercore.UnsupportedContract("outside the current Z.AI text-only Chat slice")
	adaptercore.SetSupport(contract, query, modelinvoker.SupportCompatible, "mapped through Z.AI's documented Chat Completions contract", modelinvoker.CapabilityTextGeneration, modelinvoker.CapabilityStreaming, modelinvoker.CapabilityToolCalling, modelinvoker.CapabilityUsageReporting)
	contract[modelinvoker.CapabilityParallelToolCalling] = adaptercore.QuerySupport(query, modelinvoker.SupportPartial, "only tool_choice=auto is supported and exact parallel behavior remains model-scoped")
	contract[modelinvoker.CapabilityStructuredOutput] = adaptercore.QuerySupport(query, modelinvoker.SupportPartial, "JSON Object is supported; strict JSON Schema is not")
	if thinkingModel(query.Model) {
		adaptercore.SetSupport(contract, query, modelinvoker.SupportCompatible, "supported by the selected GLM thinking model", modelinvoker.CapabilityReasoning, modelinvoker.CapabilityReasoningSummary)
		contract[modelinvoker.CapabilityProviderContinuation] = adaptercore.QuerySupport(query, modelinvoker.SupportUnsupported, "preserved thinking is disabled in the current standard API slice")
	}
	return contract, nil
}

func providerError(kind modelinvoker.ErrorKind, op, message string) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: kind, Provider: ProviderID, Operation: op, Message: message}
}

var _ modelinvoker.Provider = (*Adapter)(nil)

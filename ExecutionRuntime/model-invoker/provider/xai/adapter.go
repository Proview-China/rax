package xai

import (
	"context"
	"fmt"
	"io"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/compatprovider"
)

const ProviderID modelinvoker.ProviderID = "xai"

type Adapter struct {
	inner *compatprovider.Adapter
}

func (Adapter) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "xai.Adapter([REDACTED])")
}
func (Adapter) GoString() string { return "xai.Adapter([REDACTED])" }

func New(config Config) (*Adapter, error) {
	if err := config.validate(); err != nil {
		redactor := adaptercore.NewRedactor(config.APIKey)
		return nil, redactor.Error(providerError(modelinvoker.ErrorInvalidRequest, "configure", err.Error()))
	}
	endpoint, _ := config.trustedEndpoint()
	inner, err := compatprovider.New(compatprovider.Config{
		Provider: ProviderID, DefaultProtocol: modelinvoker.ProtocolResponses,
		APIKey: config.APIKey, HTTPClient: config.HTTPClient,
		ResponsesEndpoint: endpoint, ResponsesDialect: responsesDialect{},
		Capabilities:     capabilityContract,
		RequestIDHeaders: []string{"x-request-id", "request-id"},
	})
	if err != nil {
		return nil, err
	}
	return &Adapter{inner: inner}, nil
}

func (a *Adapter) ID() modelinvoker.ProviderID            { return ProviderID }
func (a *Adapter) DefaultProtocol() modelinvoker.Protocol { return modelinvoker.ProtocolResponses }
func (a *Adapter) CandidateBindingEndpoint(protocolID modelinvoker.Protocol, requested string) (string, bool) {
	if a == nil || a.inner == nil {
		return "", false
	}
	return a.inner.CandidateBindingEndpoint(protocolID, requested)
}

func (a *Adapter) Capabilities(ctx context.Context, query modelinvoker.CapabilityQuery) (modelinvoker.CapabilityContract, error) {
	if a == nil || a.inner == nil {
		return nil, providerError(modelinvoker.ErrorProviderUnavailable, "capabilities", "adapter is not initialized")
	}
	return a.inner.Capabilities(ctx, query)
}

func (a *Adapter) Invoke(ctx context.Context, request modelinvoker.Request) (modelinvoker.Response, error) {
	if a == nil || a.inner == nil {
		return modelinvoker.Response{}, providerError(modelinvoker.ErrorProviderUnavailable, "invoke", "adapter is not initialized")
	}
	return a.inner.Invoke(ctx, request)
}

func (a *Adapter) Stream(ctx context.Context, request modelinvoker.Request) (modelinvoker.Stream, error) {
	if a == nil || a.inner == nil {
		return nil, providerError(modelinvoker.ErrorProviderUnavailable, "stream", "adapter is not initialized")
	}
	return a.inner.Stream(ctx, request)
}

func capabilityContract(_ context.Context, query modelinvoker.CapabilityQuery) (modelinvoker.CapabilityContract, error) {
	if query.Model != "grok-4.5" {
		return nil, providerError(modelinvoker.ErrorMapping, "capabilities", "model must be the approved grok-4.5 ID")
	}
	if query.Protocol != modelinvoker.ProtocolResponses {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "capabilities", "only the xAI Responses protocol is approved")
	}
	contract := adaptercore.UnsupportedContract("outside the current xAI Responses text slice")
	adaptercore.SetSupport(contract, query, modelinvoker.SupportCompatible, "mapped through xAI's documented Responses contract",
		modelinvoker.CapabilityTextGeneration, modelinvoker.CapabilityStreaming, modelinvoker.CapabilityToolCalling,
		modelinvoker.CapabilityParallelToolCalling, modelinvoker.CapabilityReasoning, modelinvoker.CapabilityServerState,
		modelinvoker.CapabilityPromptCaching, modelinvoker.CapabilityUsageReporting)
	contract[modelinvoker.CapabilityFunctionErrorResult] = adaptercore.QuerySupport(query, modelinvoker.SupportPartial, "Responses function output cannot preserve the portable is_error marker")
	contract[modelinvoker.CapabilityReasoningSummary] = adaptercore.QuerySupport(query, modelinvoker.SupportUnsupported, "encrypted reasoning is not exposed as a readable portable summary")
	return contract, nil
}

func providerError(kind modelinvoker.ErrorKind, operation, message string) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: kind, Provider: ProviderID, Operation: operation, Message: strings.TrimSpace(message)}
}

var (
	_ modelinvoker.Provider               = (*Adapter)(nil)
	_ adaptercore.CandidateBindingReceipt = (*Adapter)(nil)
)

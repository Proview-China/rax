package qwen

import (
	"context"
	"fmt"
	"io"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/compatprovider"
)

const ProviderID modelinvoker.ProviderID = "qwen"

type Adapter struct {
	inner     *compatprovider.Adapter
	region    Region
	workspace string
}

func (Adapter) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "qwen.Adapter([REDACTED])")
}
func (Adapter) GoString() string { return "qwen.Adapter([REDACTED])" }

func New(config Config) (*Adapter, error) {
	if err := config.validate(); err != nil {
		redactor := adaptercore.NewRedactor(config.APIKey)
		return nil, redactor.Error(providerError(modelinvoker.ErrorInvalidRequest, "configure", err.Error()))
	}
	endpoint, _ := config.trustedEndpoint()
	inner, err := compatprovider.New(compatprovider.Config{
		Provider: ProviderID, DefaultProtocol: modelinvoker.ProtocolResponses,
		APIKey: config.APIKey, HTTPClient: config.HTTPClient,
		ChatEndpoint: endpoint, ResponsesEndpoint: endpoint,
		ChatDialect: chatDialect{}, ResponsesDialect: responsesDialect{},
		Capabilities:     capabilityContract,
		RequestIDHeaders: []string{"x-request-id", "request-id", "x-dashscope-request-id"},
	})
	if err != nil {
		return nil, err
	}
	return &Adapter{inner: inner, region: config.Region, workspace: config.WorkspaceID}, nil
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
	if !supportedModel(query.Model) {
		return nil, providerError(modelinvoker.ErrorMapping, "capabilities", "model must be in the approved Qwen text alias set")
	}
	if query.Protocol != modelinvoker.ProtocolResponses && query.Protocol != modelinvoker.ProtocolChatCompletions {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "capabilities", "unsupported Qwen protocol")
	}
	contract := adaptercore.UnsupportedContract("outside the current Qwen pay-as-you-go text slice")
	adaptercore.SetSupport(contract, query, modelinvoker.SupportCompatible, "mapped through Alibaba Model Studio's documented OpenAI compatibility contract",
		modelinvoker.CapabilityTextGeneration, modelinvoker.CapabilityStreaming, modelinvoker.CapabilityToolCalling,
		modelinvoker.CapabilityReasoning, modelinvoker.CapabilityReasoningSummary, modelinvoker.CapabilityUsageReporting)
	contract[modelinvoker.CapabilityParallelToolCalling] = adaptercore.QuerySupport(query, modelinvoker.SupportUnsupported, "portable parallel-tool control is not approved in this slice")
	contract[modelinvoker.CapabilityFunctionErrorResult] = adaptercore.QuerySupport(query, modelinvoker.SupportPartial, "OpenAI-compatible tool outputs cannot preserve the portable is_error marker")
	if query.Protocol == modelinvoker.ProtocolResponses {
		contract[modelinvoker.CapabilityServerState] = adaptercore.QuerySupport(query, modelinvoker.SupportCompatible, "previous_response_id is supported for seven days and remains identity-bound")
		contract[modelinvoker.CapabilityStructuredOutput] = adaptercore.QuerySupport(query, modelinvoker.SupportUnsupported, "Responses structured output is not approved in this slice")
	} else {
		contract[modelinvoker.CapabilityStructuredOutput] = adaptercore.QuerySupport(query, modelinvoker.SupportPartial, "Chat supports JSON Object only")
		contract[modelinvoker.CapabilityProviderContinuation] = adaptercore.QuerySupport(query, modelinvoker.SupportUnsupported, "portable Chat input cannot preserve Qwen reasoning_content history")
	}
	return contract, nil
}

func providerError(kind modelinvoker.ErrorKind, operation, message string) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: kind, Provider: ProviderID, Operation: operation, Message: strings.TrimSpace(message)}
}

var (
	_ modelinvoker.Provider               = (*Adapter)(nil)
	_ adaptercore.CandidateBindingReceipt = (*Adapter)(nil)
)

package minimax

import (
	"context"
	"fmt"
	"io"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/compatprovider"
)

const ProviderID modelinvoker.ProviderID = "minimax"

type Adapter struct{ inner *compatprovider.Adapter }

func (Adapter) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "minimax.Adapter([REDACTED])")
}
func (Adapter) GoString() string { return "minimax.Adapter([REDACTED])" }

func New(config Config) (*Adapter, error) {
	key := strings.TrimSpace(config.APIKey)
	if strings.HasPrefix(key, "sk-cp-") {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "configure", "MiniMax Token Plan keys are not accepted by the pay-as-you-go adapter")
	}
	root, err := config.rootEndpoint()
	if err != nil {
		return nil, err
	}
	inner, err := compatprovider.New(compatprovider.Config{
		Provider: ProviderID, DefaultProtocol: modelinvoker.ProtocolMessages,
		APIKey: key, HTTPClient: config.HTTPClient,
		ChatEndpoint: root + "/v1", ResponsesEndpoint: root + "/v1", MessagesEndpoint: root + "/anthropic",
		ChatDialect: chatDialect{}, ResponsesDialect: responsesDialect{}, MessagesDialect: messagesDialect{},
		Capabilities: capabilityContract, RequestIDHeaders: []string{"x-request-id", "request-id"},
	})
	if err != nil {
		return nil, err
	}
	return &Adapter{inner: inner}, nil
}

func (a *Adapter) ID() modelinvoker.ProviderID            { return ProviderID }
func (a *Adapter) DefaultProtocol() modelinvoker.Protocol { return modelinvoker.ProtocolMessages }
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
		return nil, providerError(modelinvoker.ErrorMapping, "capabilities", "model must be a current approved MiniMax text model")
	}
	if query.Protocol != modelinvoker.ProtocolMessages && query.Protocol != modelinvoker.ProtocolChatCompletions && query.Protocol != modelinvoker.ProtocolResponses {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "capabilities", "unsupported MiniMax protocol")
	}
	contract := adaptercore.UnsupportedContract("outside the current MiniMax pay-as-you-go text slice")
	adaptercore.SetSupport(contract, query, modelinvoker.SupportCompatible, "mapped through MiniMax's documented compatibility contract",
		modelinvoker.CapabilityTextGeneration, modelinvoker.CapabilityStreaming, modelinvoker.CapabilityToolCalling,
		modelinvoker.CapabilityReasoning, modelinvoker.CapabilityReasoningSummary, modelinvoker.CapabilityUsageReporting)
	contract[modelinvoker.CapabilityParallelToolCalling] = adaptercore.QuerySupport(query, modelinvoker.SupportUnsupported, "MiniMax does not expose portable parallel-tool control in this slice")
	contract[modelinvoker.CapabilityStructuredOutput] = adaptercore.QuerySupport(query, modelinvoker.SupportUnsupported, "only text output is approved in the current MiniMax compatibility slice")
	if query.Protocol == modelinvoker.ProtocolMessages {
		contract[modelinvoker.CapabilityProviderContinuation] = adaptercore.QuerySupport(query, modelinvoker.SupportCompatible, "thinking signatures and tool blocks are preserved through provider continuation state")
		contract[modelinvoker.CapabilityFunctionErrorResult] = adaptercore.QuerySupport(query, modelinvoker.SupportCompatible, "Messages preserves tool_result is_error")
	} else {
		contract[modelinvoker.CapabilityFunctionErrorResult] = adaptercore.QuerySupport(query, modelinvoker.SupportPartial, "the compatible OpenAI formats cannot preserve the portable is_error marker")
	}
	contract[modelinvoker.CapabilityServerState] = adaptercore.QuerySupport(query, modelinvoker.SupportUnsupported, "MiniMax Responses is stateless and does not accept previous_response_id")
	return contract, nil
}

func providerError(kind modelinvoker.ErrorKind, operation, message string) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: kind, Provider: ProviderID, Operation: operation, Message: message}
}

var (
	_ modelinvoker.Provider               = (*Adapter)(nil)
	_ adaptercore.CandidateBindingReceipt = (*Adapter)(nil)
)

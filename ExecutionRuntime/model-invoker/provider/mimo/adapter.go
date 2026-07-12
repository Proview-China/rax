package mimo

import (
	"context"
	"fmt"
	"io"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/compatprovider"
)

const ProviderID modelinvoker.ProviderID = "xiaomi-mimo"

type Adapter struct{ inner *compatprovider.Adapter }

func (Adapter) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "mimo.Adapter([REDACTED])")
}
func (Adapter) GoString() string { return "mimo.Adapter([REDACTED])" }

func New(config Config) (*Adapter, error) {
	key := strings.TrimSpace(config.APIKey)
	if strings.HasPrefix(key, "tp-") {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "configure", "MiMo Token Plan keys are not accepted by the pay-as-you-go adapter")
	}
	root, err := config.rootEndpoint()
	if err != nil {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "configure", err.Error())
	}
	inner, err := compatprovider.New(compatprovider.Config{
		Provider: ProviderID, DefaultProtocol: modelinvoker.ProtocolMessages,
		APIKey: key, HTTPClient: config.HTTPClient,
		ChatEndpoint: root + "/v1", MessagesEndpoint: root + "/anthropic", MessagesAuthToken: true,
		ChatDialect: chatDialect{}, MessagesDialect: messagesDialect{},
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
		return nil, providerError(modelinvoker.ErrorMapping, "capabilities", "model must be a current approved MiMo V2.5 text model")
	}
	if query.Protocol != modelinvoker.ProtocolMessages && query.Protocol != modelinvoker.ProtocolChatCompletions {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "capabilities", "MiMo only exposes the approved Messages and Chat bindings")
	}
	contract := adaptercore.UnsupportedContract("outside the current MiMo pay-as-you-go text slice")
	adaptercore.SetSupport(contract, query, modelinvoker.SupportCompatible, "mapped through MiMo's documented compatibility contract",
		modelinvoker.CapabilityTextGeneration, modelinvoker.CapabilityStreaming, modelinvoker.CapabilityToolCalling,
		modelinvoker.CapabilityReasoning, modelinvoker.CapabilityReasoningSummary, modelinvoker.CapabilityUsageReporting)
	contract[modelinvoker.CapabilityPromptCaching] = adaptercore.QuerySupport(query, modelinvoker.SupportPartial, "cache usage is preserved; cache creation is not exposed by the portable request")
	if query.Protocol == modelinvoker.ProtocolMessages {
		adaptercore.SetSupport(contract, query, modelinvoker.SupportCompatible, "Messages preserves the documented parallel-tool and continuation contracts",
			modelinvoker.CapabilityParallelToolCalling, modelinvoker.CapabilityProviderContinuation, modelinvoker.CapabilityFunctionErrorResult)
	} else {
		contract[modelinvoker.CapabilityParallelToolCalling] = adaptercore.QuerySupport(query, modelinvoker.SupportUnsupported, "MiMo Chat exposes no portable parallel-tool control")
		contract[modelinvoker.CapabilityStructuredOutput] = adaptercore.QuerySupport(query, modelinvoker.SupportPartial, "JSON Object is supported; strict JSON Schema output is not")
		contract[modelinvoker.CapabilityProviderContinuation] = adaptercore.QuerySupport(query, modelinvoker.SupportUnsupported, "thinking tool history cannot be represented losslessly in portable Chat input")
		contract[modelinvoker.CapabilityFunctionErrorResult] = adaptercore.QuerySupport(query, modelinvoker.SupportPartial, "the Chat format cannot preserve the portable is_error marker")
	}
	return contract, nil
}

func providerError(kind modelinvoker.ErrorKind, operation, message string) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: kind, Provider: ProviderID, Operation: operation, Message: message}
}

var (
	_ modelinvoker.Provider               = (*Adapter)(nil)
	_ adaptercore.CandidateBindingReceipt = (*Adapter)(nil)
)

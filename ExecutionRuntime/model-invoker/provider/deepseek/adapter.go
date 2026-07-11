package deepseek

import (
	"context"
	"fmt"
	"io"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/compatprovider"
)

const ProviderID modelinvoker.ProviderID = "deepseek"

type Adapter struct{ inner *compatprovider.Adapter }

func (Adapter) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "deepseek.Adapter([REDACTED])")
}
func (Adapter) GoString() string { return "deepseek.Adapter([REDACTED])" }

func New(config Config) (*Adapter, error) {
	root := config.root()
	inner, err := compatprovider.New(compatprovider.Config{
		Provider: ProviderID, DefaultProtocol: modelinvoker.ProtocolChatCompletions,
		APIKey: config.APIKey, HTTPClient: config.HTTPClient,
		ChatEndpoint: root, MessagesEndpoint: root + "/anthropic",
		ChatDialect: chatDialect{}, MessagesDialect: messagesDialect{},
		Capabilities:     capabilityContract,
		RequestIDHeaders: []string{"x-request-id", "request-id"},
	})
	if err != nil {
		return nil, err
	}
	return &Adapter{inner: inner}, nil
}

func (a *Adapter) ID() modelinvoker.ProviderID {
	if a == nil || a.inner == nil {
		return ProviderID
	}
	return a.inner.ID()
}
func (a *Adapter) DefaultProtocol() modelinvoker.Protocol {
	if a == nil || a.inner == nil {
		return modelinvoker.ProtocolChatCompletions
	}
	return a.inner.DefaultProtocol()
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
		return nil, providerError(modelinvoker.ErrorMapping, "capabilities", "model must be an exact current DeepSeek v4 model ID")
	}
	contract := unsupportedContract()
	switch query.Protocol {
	case modelinvoker.ProtocolChatCompletions:
		setSupport(contract, query, modelinvoker.SupportCompatible, "mapped through DeepSeek's documented Chat Completions contract",
			modelinvoker.CapabilityTextGeneration, modelinvoker.CapabilityStreaming, modelinvoker.CapabilityToolCalling,
			modelinvoker.CapabilityParallelToolCalling, modelinvoker.CapabilityReasoning, modelinvoker.CapabilityUsageReporting)
		contract[modelinvoker.CapabilityStructuredOutput] = querySupport(query, modelinvoker.SupportPartial, "JSON Object is supported; strict JSON Schema is not declared")
	case modelinvoker.ProtocolMessages:
		setSupport(contract, query, modelinvoker.SupportCompatible, "mapped through DeepSeek's documented Anthropic Messages compatibility contract",
			modelinvoker.CapabilityTextGeneration, modelinvoker.CapabilityStreaming, modelinvoker.CapabilityToolCalling,
			modelinvoker.CapabilityReasoning, modelinvoker.CapabilityProviderContinuation, modelinvoker.CapabilityUsageReporting)
		contract[modelinvoker.CapabilityParallelToolCalling] = querySupport(query, modelinvoker.SupportPartial, "disable_parallel_tool_use is ignored by the provider")
	case modelinvoker.ProtocolResponses:
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "capabilities", "DeepSeek Responses is not part of the approved binding")
	default:
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "capabilities", "unsupported protocol")
	}
	return contract, nil
}

func unsupportedContract() modelinvoker.CapabilityContract {
	return map[modelinvoker.Capability]modelinvoker.CapabilitySupport{
		modelinvoker.CapabilityTextGeneration:       {Level: modelinvoker.SupportUnsupported},
		modelinvoker.CapabilityStreaming:            {Level: modelinvoker.SupportUnsupported},
		modelinvoker.CapabilityToolCalling:          {Level: modelinvoker.SupportUnsupported},
		modelinvoker.CapabilityParallelToolCalling:  {Level: modelinvoker.SupportUnsupported},
		modelinvoker.CapabilityStructuredOutput:     {Level: modelinvoker.SupportUnsupported},
		modelinvoker.CapabilityReasoning:            {Level: modelinvoker.SupportUnsupported},
		modelinvoker.CapabilityReasoningSummary:     {Level: modelinvoker.SupportUnsupported},
		modelinvoker.CapabilityFunctionErrorResult:  {Level: modelinvoker.SupportUnsupported},
		modelinvoker.CapabilityServerState:          {Level: modelinvoker.SupportUnsupported},
		modelinvoker.CapabilityProviderContinuation: {Level: modelinvoker.SupportUnsupported},
		modelinvoker.CapabilityPromptCaching:        {Level: modelinvoker.SupportUnsupported},
		modelinvoker.CapabilityUsageReporting:       {Level: modelinvoker.SupportUnsupported},
	}
}

func setSupport(c modelinvoker.CapabilityContract, q modelinvoker.CapabilityQuery, level modelinvoker.SupportLevel, detail string, capabilities ...modelinvoker.Capability) {
	for _, capability := range capabilities {
		c[capability] = querySupport(q, level, detail)
	}
}
func querySupport(q modelinvoker.CapabilityQuery, level modelinvoker.SupportLevel, detail string) modelinvoker.CapabilitySupport {
	return modelinvoker.CapabilitySupport{Level: level, Models: []string{q.Model}, Protocols: []modelinvoker.Protocol{q.Protocol}, Limitations: []string{detail}}
}

func providerError(kind modelinvoker.ErrorKind, op, message string) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: kind, Provider: ProviderID, Operation: op, Message: message}
}

var _ modelinvoker.Provider = (*Adapter)(nil)

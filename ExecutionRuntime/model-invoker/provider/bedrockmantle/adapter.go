package bedrockmantle

import (
	"context"
	"fmt"
	"io"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/anthropicmessages"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openaichat"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openairesponses"
)

const ProviderID modelinvoker.ProviderID = "aws-bedrock-mantle"

type Adapter struct {
	chat      *openaichat.Driver
	responses *openairesponses.Driver
	messages  *anthropicmessages.Driver
	bindings  map[modelinvoker.Protocol]protocol.Binding
	endpoints map[modelinvoker.Protocol]string
	redactor  adaptercore.Redactor
}

func (Adapter) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "bedrockmantle.Adapter([REDACTED])")
}
func (Adapter) GoString() string { return "bedrockmantle.Adapter([REDACTED])" }

func New(config Config) (*Adapter, error) {
	redactor := adaptercore.NewRedactor(config.APIKey, config.AccessKeyID, config.SecretAccessKey, config.SessionToken)
	if err := config.validate(); err != nil {
		return nil, redactor.Error(providerError(modelinvoker.ErrorInvalidRequest, "configure", err.Error()))
	}
	root, _ := config.trustedRootEndpoint()
	config.BaseURL = root
	chatClient, responsesClient, messagesClient, err := newSDKClients(config)
	if err != nil {
		return nil, redactor.Error(providerError(modelinvoker.ErrorProviderUnavailable, "configure", "failed to initialize Bedrock Mantle SDK clients"))
	}
	root = strings.TrimRight(root, "/")
	chatBinding, err := protocol.NewBinding(ProviderID, modelinvoker.ProtocolChatCompletions, root+"/openai/v1", "x-amzn-requestid")
	if err != nil {
		return nil, err
	}
	responsesBinding, err := protocol.NewBinding(ProviderID, modelinvoker.ProtocolResponses, root+"/openai/v1", "x-amzn-requestid")
	if err != nil {
		return nil, err
	}
	messagesBinding, err := protocol.NewBinding(ProviderID, modelinvoker.ProtocolMessages, root+"/anthropic/v1", "x-amzn-requestid")
	if err != nil {
		return nil, err
	}
	chat, err := openaichat.New(chatBinding, cloudDialect{store: config.storesResponses()}, chatClient)
	if err != nil {
		return nil, err
	}
	responsesDriver, err := openairesponses.New(responsesBinding, cloudDialect{store: config.storesResponses()}, responsesClient)
	if err != nil {
		return nil, err
	}
	messages, err := anthropicmessages.New(messagesBinding, cloudDialect{store: config.storesResponses(), messages: true}, messagesClient)
	if err != nil {
		return nil, err
	}
	bindings := map[modelinvoker.Protocol]protocol.Binding{}
	endpoints := map[modelinvoker.Protocol]string{}
	for id, binding := range map[modelinvoker.Protocol]protocol.Binding{modelinvoker.ProtocolChatCompletions: chatBinding, modelinvoker.ProtocolResponses: responsesBinding, modelinvoker.ProtocolMessages: messagesBinding} {
		public := binding.Clone()
		public.Endpoint = redactor.String(public.Endpoint)
		bindings[id] = public
		endpoints[id] = binding.Endpoint
	}
	return &Adapter{chat: chat, responses: responsesDriver, messages: messages, bindings: bindings, endpoints: endpoints, redactor: redactor}, nil
}

func (a *Adapter) ID() modelinvoker.ProviderID            { return ProviderID }
func (a *Adapter) DefaultProtocol() modelinvoker.Protocol { return modelinvoker.ProtocolResponses }
func (a *Adapter) CandidateBindingEndpoint(protocolID modelinvoker.Protocol, _ string) (string, bool) {
	if a == nil {
		return "", false
	}
	endpoint, ok := a.endpoints[protocolID]
	return endpoint, ok && endpoint != ""
}
func (a *Adapter) Capabilities(ctx context.Context, query modelinvoker.CapabilityQuery) (modelinvoker.CapabilityContract, error) {
	if a == nil {
		return nil, providerError(modelinvoker.ErrorProviderUnavailable, "capabilities", "adapter is not initialized")
	}
	if ctx == nil {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "capabilities", "context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, adaptercore.ContextError(ProviderID, "capabilities", err)
	}
	if strings.TrimSpace(query.Model) == "" {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "capabilities", "model is required")
	}
	endpoint, ok := a.endpoints[query.Protocol]
	if !ok {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "capabilities", "unsupported protocol")
	}
	if query.Endpoint != "" && adaptercore.NormalizeEndpoint(query.Endpoint) != endpoint {
		return nil, mappingError("capabilities", "endpoint does not match protocol binding")
	}
	contract := adaptercore.UnsupportedContract("outside the Bedrock Mantle compatibility table for the selected model")
	adaptercore.SetSupport(contract, query, modelinvoker.SupportCompatible, "mapped through a Bedrock Mantle compatibility protocol", modelinvoker.CapabilityTextGeneration, modelinvoker.CapabilityStreaming, modelinvoker.CapabilityToolCalling, modelinvoker.CapabilityUsageReporting)
	if query.Protocol == modelinvoker.ProtocolResponses {
		contract[modelinvoker.CapabilityServerState] = adaptercore.QuerySupport(query, modelinvoker.SupportPartial, "requires store=true and Project-scoped 30-day state")
	}
	return contract, nil
}
func (a *Adapter) selected(id modelinvoker.Protocol) (protocol.Driver, protocol.Binding, bool) {
	if a == nil {
		return nil, protocol.Binding{}, false
	}
	switch id {
	case modelinvoker.ProtocolChatCompletions:
		return a.chat, a.bindings[id], a.chat != nil
	case modelinvoker.ProtocolResponses:
		return a.responses, a.bindings[id], a.responses != nil
	case modelinvoker.ProtocolMessages:
		return a.messages, a.bindings[id], a.messages != nil
	}
	return nil, protocol.Binding{}, false
}
func (a *Adapter) Invoke(ctx context.Context, request modelinvoker.Request) (result modelinvoker.Response, resultErr error) {
	driver, binding, ok := a.selected(request.Protocol)
	redactor := adaptercore.Redactor{}
	if a != nil {
		redactor = a.redactor
	}
	defer func() {
		result = redactor.Response(result)
		resultErr = redactor.Error(resultErr)
		if ok {
			result = binding.StampResponse(request, result)
			resultErr = binding.StampError(ctx, request, resultErr, "invoke")
		}
	}()
	if ctx == nil {
		return modelinvoker.Response{}, providerError(modelinvoker.ErrorInvalidRequest, "invoke", "context is nil")
	}
	if !ok {
		return modelinvoker.Response{}, providerError(modelinvoker.ErrorInvalidRequest, "invoke", "unsupported protocol")
	}
	request.Stream = false
	return driver.Invoke(ctx, request)
}
func (a *Adapter) Stream(ctx context.Context, request modelinvoker.Request) (result modelinvoker.Stream, resultErr error) {
	driver, binding, ok := a.selected(request.Protocol)
	redactor := adaptercore.Redactor{}
	if a != nil {
		redactor = a.redactor
	}
	defer func() {
		result = redactor.Stream(result)
		resultErr = redactor.Error(resultErr)
		if ok {
			result = binding.BindStream(ctx, request, result)
			resultErr = binding.StampError(ctx, request, resultErr, "stream")
		}
	}()
	if ctx == nil {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "stream", "context is nil")
	}
	if !ok {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "stream", "unsupported protocol")
	}
	request.Stream = true
	return driver.Stream(ctx, request)
}
func providerError(kind modelinvoker.ErrorKind, operation, message string) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: kind, Provider: ProviderID, Operation: operation, Message: message}
}

var _ modelinvoker.Provider = (*Adapter)(nil)
var _ adaptercore.CandidateBindingReceipt = (*Adapter)(nil)

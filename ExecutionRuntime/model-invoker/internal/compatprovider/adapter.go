package compatprovider

import (
	"context"
	"fmt"
	"io"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/anthropicmessages"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/geminigenerate"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openaichat"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openairesponses"
)

type Adapter struct {
	provider        modelinvoker.ProviderID
	defaultProtocol modelinvoker.Protocol
	drivers         map[modelinvoker.Protocol]protocol.Driver
	bindings        map[modelinvoker.Protocol]protocol.Binding
	endpoints       map[modelinvoker.Protocol]string
	capabilities    CapabilityBuilder
	redactor        adaptercore.Redactor
}

func (Adapter) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "compatprovider.Adapter([REDACTED])")
}
func (Adapter) GoString() string { return "compatprovider.Adapter([REDACTED])" }

func New(config Config) (*Adapter, error) {
	redactor := adaptercore.NewRedactor(config.APIKey)
	if err := config.Validate(); err != nil {
		return nil, redactor.Error(providerError(config.Provider, modelinvoker.ErrorInvalidRequest, "configure", err.Error()))
	}
	a := &Adapter{
		provider: config.Provider, defaultProtocol: config.DefaultProtocol,
		drivers: make(map[modelinvoker.Protocol]protocol.Driver), bindings: make(map[modelinvoker.Protocol]protocol.Binding),
		endpoints: make(map[modelinvoker.Protocol]string), capabilities: config.Capabilities, redactor: redactor,
	}
	if config.ChatEndpoint != "" {
		if err := a.addChat(config); err != nil {
			return nil, redactor.Error(err)
		}
	}
	if config.ResponsesEndpoint != "" {
		if err := a.addResponses(config); err != nil {
			return nil, redactor.Error(err)
		}
	}
	if config.MessagesEndpoint != "" {
		if err := a.addMessages(config); err != nil {
			return nil, redactor.Error(err)
		}
	}
	if config.GenerateEndpoint != "" {
		if err := a.addGenerate(config); err != nil {
			return nil, redactor.Error(err)
		}
	}
	return a, nil
}

func (a *Adapter) addBinding(id modelinvoker.Protocol, endpoint string, driver protocol.Driver, headers []string) error {
	binding, err := protocol.NewBinding(a.provider, id, adaptercore.NormalizeEndpoint(endpoint), headers...)
	if err != nil {
		return err
	}
	_ = binding
	// Drivers are constructed with an identical private binding in add*; retain
	// a redacted public clone for the final identity stamp.
	private := driver.Binding()
	public := private.Clone()
	public.Endpoint = a.redactor.String(public.Endpoint)
	a.drivers[id] = driver
	a.bindings[id] = public
	a.endpoints[id] = private.Endpoint
	return nil
}

func (a *Adapter) addChat(c Config) error {
	endpoint := adaptercore.NormalizeEndpoint(c.ChatEndpoint)
	binding, err := protocol.NewBinding(c.Provider, modelinvoker.ProtocolChatCompletions, endpoint, c.RequestIDHeaders...)
	if err != nil {
		return err
	}
	driver, err := openaichat.New(binding, c.ChatDialect, newOpenAIClient(c.APIKey, endpoint, c.HTTPClient, c.UserAgent))
	if err != nil {
		return err
	}
	return a.addBinding(modelinvoker.ProtocolChatCompletions, endpoint, driver, c.RequestIDHeaders)
}

func (a *Adapter) addResponses(c Config) error {
	endpoint := adaptercore.NormalizeEndpoint(c.ResponsesEndpoint)
	binding, err := protocol.NewBinding(c.Provider, modelinvoker.ProtocolResponses, endpoint, c.RequestIDHeaders...)
	if err != nil {
		return err
	}
	client := newOpenAIClient(c.APIKey, endpoint, c.HTTPClient, c.UserAgent)
	driver, err := openairesponses.New(binding, c.ResponsesDialect, responsesClient{native: client})
	if err != nil {
		return err
	}
	return a.addBinding(modelinvoker.ProtocolResponses, endpoint, driver, c.RequestIDHeaders)
}

func (a *Adapter) addMessages(c Config) error {
	endpoint := adaptercore.NormalizeEndpoint(c.MessagesEndpoint)
	binding, err := protocol.NewBinding(c.Provider, modelinvoker.ProtocolMessages, endpoint, c.RequestIDHeaders...)
	if err != nil {
		return err
	}
	driver, err := anthropicmessages.New(binding, c.MessagesDialect, newAnthropicClient(c.APIKey, endpoint, c.HTTPClient, c.MessagesAuthToken, c.UserAgent))
	if err != nil {
		return err
	}
	return a.addBinding(modelinvoker.ProtocolMessages, endpoint, driver, c.RequestIDHeaders)
}

func (a *Adapter) addGenerate(c Config) error {
	endpoint := adaptercore.NormalizeEndpoint(c.GenerateEndpoint)
	binding, err := protocol.NewBinding(c.Provider, modelinvoker.ProtocolGenerateContent, endpoint, c.RequestIDHeaders...)
	if err != nil {
		return err
	}
	client, err := newGenerateClient(c.APIKey, c.GenerateBaseURL, c.GenerateAPIVersion, c.HTTPClient, c.UserAgent)
	if err != nil {
		return err
	}
	driver, err := geminigenerate.New(binding, c.GenerateDialect, client)
	if err != nil {
		return err
	}
	return a.addBinding(modelinvoker.ProtocolGenerateContent, endpoint, driver, c.RequestIDHeaders)
}

func (a *Adapter) ID() modelinvoker.ProviderID {
	if a == nil {
		return ""
	}
	return a.provider
}
func (a *Adapter) DefaultProtocol() modelinvoker.Protocol {
	if a == nil {
		return modelinvoker.ProtocolAuto
	}
	return a.defaultProtocol
}

func (a *Adapter) CandidateBindingEndpoint(protocolID modelinvoker.Protocol, _ string) (string, bool) {
	if a == nil {
		return "", false
	}
	endpoint, ok := a.endpoints[protocolID]
	return endpoint, ok && endpoint != ""
}

func (a *Adapter) Capabilities(ctx context.Context, query modelinvoker.CapabilityQuery) (modelinvoker.CapabilityContract, error) {
	if a == nil || a.capabilities == nil {
		return nil, providerError(a.ID(), modelinvoker.ErrorProviderUnavailable, "capabilities", "adapter is not initialized")
	}
	if ctx == nil {
		return nil, providerError(a.provider, modelinvoker.ErrorInvalidRequest, "capabilities", "context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, adaptercore.ContextError(a.provider, "capabilities", err)
	}
	if strings.TrimSpace(query.Model) == "" {
		return nil, providerError(a.provider, modelinvoker.ErrorInvalidRequest, "capabilities", "model is required")
	}
	endpoint, ok := a.endpoints[query.Protocol]
	if !ok {
		return nil, providerError(a.provider, modelinvoker.ErrorInvalidRequest, "capabilities", "unsupported protocol")
	}
	if query.Endpoint != "" && adaptercore.NormalizeEndpoint(query.Endpoint) != endpoint {
		return nil, providerError(a.provider, modelinvoker.ErrorMapping, "capabilities", "endpoint does not match the selected provider binding")
	}
	return a.capabilities(ctx, query)
}

func (a *Adapter) selectDriver(request modelinvoker.Request) (protocol.Driver, protocol.Binding, bool) {
	if a == nil {
		return nil, protocol.Binding{}, false
	}
	driver, ok := a.drivers[request.Protocol]
	return driver, a.bindings[request.Protocol], ok && !protocol.IsNil(driver)
}

func (a *Adapter) Invoke(ctx context.Context, request modelinvoker.Request) (out modelinvoker.Response, err error) {
	driver, binding, ok := a.selectDriver(request)
	redactor := adaptercore.Redactor{}
	if a != nil {
		redactor = a.redactor
	}
	defer func() {
		out = redactor.Response(out)
		err = redactor.Error(err)
		if ok {
			out = binding.StampResponse(request, out)
			err = binding.StampError(ctx, request, err, "invoke")
		}
	}()
	if ctx == nil {
		return out, providerError(a.ID(), modelinvoker.ErrorInvalidRequest, "invoke", "context is nil")
	}
	if !ok {
		return out, providerError(a.ID(), modelinvoker.ErrorInvalidRequest, "invoke", "unsupported protocol")
	}
	if err := a.validateSelection(request); err != nil {
		return out, err
	}
	request.Stream = false
	return driver.Invoke(ctx, request)
}

func (a *Adapter) Stream(ctx context.Context, request modelinvoker.Request) (out modelinvoker.Stream, err error) {
	driver, binding, ok := a.selectDriver(request)
	redactor := adaptercore.Redactor{}
	if a != nil {
		redactor = a.redactor
	}
	defer func() {
		out = redactor.Stream(out)
		err = redactor.Error(err)
		if ok {
			out = binding.BindStream(ctx, request, out)
			err = binding.StampError(ctx, request, err, "stream")
		}
	}()
	if ctx == nil {
		return nil, providerError(a.ID(), modelinvoker.ErrorInvalidRequest, "stream", "context is nil")
	}
	if !ok {
		return nil, providerError(a.ID(), modelinvoker.ErrorInvalidRequest, "stream", "unsupported protocol")
	}
	if err := a.validateSelection(request); err != nil {
		return nil, err
	}
	request.Stream = true
	return driver.Stream(ctx, request)
}

func (a *Adapter) validateSelection(request modelinvoker.Request) error {
	if err := request.Validate(); err != nil {
		return err
	}
	endpoint, ok := a.endpoints[request.Protocol]
	if !ok {
		return providerError(a.provider, modelinvoker.ErrorInvalidRequest, "validate", "unsupported protocol")
	}
	if err := adaptercore.ValidateSelection(request, a.provider, endpoint, request.Protocol); err != nil {
		kind := modelinvoker.ErrorMapping
		if request.Provider != a.provider || request.Protocol == modelinvoker.ProtocolAuto {
			kind = modelinvoker.ErrorInvalidRequest
		}
		return providerError(a.provider, kind, "validate", err.Error())
	}
	return nil
}

func providerError(provider modelinvoker.ProviderID, kind modelinvoker.ErrorKind, operation, message string) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: kind, Provider: provider, Operation: operation, Message: message}
}

var _ modelinvoker.Provider = (*Adapter)(nil)
var _ adaptercore.CandidateBindingReceipt = (*Adapter)(nil)

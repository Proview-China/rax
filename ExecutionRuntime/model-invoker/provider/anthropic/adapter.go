package anthropic

import (
	"context"
	"fmt"
	"io"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/anthropicmessages"
)

const (
	ProviderID      modelinvoker.ProviderID = "anthropic"
	defaultBaseURL                          = "https://api.anthropic.com"
	requestIDHeader                         = "request-id"
)

type Adapter struct {
	client                anthropicmessages.Client
	messagesDriver        *anthropicmessages.Driver
	messagesPublicBinding protocol.Binding
	baseURL               string
	redactor              adaptercore.Redactor
}

func (Adapter) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "anthropic.Adapter([REDACTED])")
}

func (Adapter) GoString() string {
	return "anthropic.Adapter([REDACTED])"
}

func New(config Config) (*Adapter, error) {
	redactor := adaptercore.NewRedactor(config.APIKey)
	if err := config.validate(); err != nil {
		return nil, redactor.Error(&modelinvoker.Error{
			Kind: modelinvoker.ErrorInvalidRequest, Provider: ProviderID, Operation: "configure",
			Message: err.Error(), Err: err,
		})
	}
	baseURL, _ := config.trustedBaseURL()
	config.BaseURL = baseURL
	adapter, err := newWithClient(newSDKClient(config), baseURL, redactor)
	if err != nil {
		return nil, redactor.Error(providerError(modelinvoker.ErrorInvalidRequest, "configure", err.Error(), nil))
	}
	return adapter, nil
}

func newWithClient(client anthropicmessages.Client, baseURL string, redactor adaptercore.Redactor) (*Adapter, error) {
	endpoint := adaptercore.NormalizeEndpoint(baseURL) + "/v1"
	binding, err := protocol.NewBinding(ProviderID, modelinvoker.ProtocolMessages, endpoint, requestIDHeader)
	if err != nil {
		return nil, fmt.Errorf("configure Messages binding: %w", err)
	}
	driver, err := anthropicmessages.New(binding, messagesDialect{}, client)
	if err != nil {
		return nil, err
	}
	publicBinding := binding.Clone()
	publicBinding.Endpoint = redactor.String(binding.Endpoint)
	return &Adapter{
		client: client, messagesDriver: driver, messagesPublicBinding: publicBinding,
		baseURL: endpoint, redactor: redactor,
	}, nil
}

func (a *Adapter) ID() modelinvoker.ProviderID { return ProviderID }

func (a *Adapter) DefaultProtocol() modelinvoker.Protocol { return modelinvoker.ProtocolMessages }

func (a *Adapter) CandidateBindingEndpoint(protocolID modelinvoker.Protocol, _ string) (string, bool) {
	if a == nil || a.baseURL == "" || protocolID != modelinvoker.ProtocolMessages {
		return "", false
	}
	return a.baseURL, true
}

func (a *Adapter) Capabilities(ctx context.Context, query modelinvoker.CapabilityQuery) (modelinvoker.CapabilityContract, error) {
	if a == nil || protocol.IsNil(a.client) || a.messagesDriver == nil {
		return nil, providerError(modelinvoker.ErrorProviderUnavailable, "capabilities", "adapter is not initialized", nil)
	}
	if ctx == nil {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "capabilities", "context is nil", nil)
	}
	if err := ctx.Err(); err != nil {
		return nil, adaptercore.ContextError(ProviderID, "capabilities", err)
	}
	if strings.TrimSpace(query.Model) == "" {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "capabilities", "model is required", nil)
	}
	if query.Protocol != modelinvoker.ProtocolMessages {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "capabilities", fmt.Sprintf("unsupported protocol %q", query.Protocol), nil)
	}
	if query.Endpoint != "" && adaptercore.NormalizeEndpoint(query.Endpoint) != a.baseURL {
		return nil, mappingError("capabilities", "capability endpoint does not match the configured Anthropic endpoint")
	}
	return capabilityContract(query), nil
}

func (a *Adapter) Invoke(ctx context.Context, request modelinvoker.Request) (result modelinvoker.Response, resultErr error) {
	redactor := adaptercore.Redactor{}
	publicBinding := protocol.Binding{}
	bindAfterRedaction := false
	if a != nil {
		redactor = a.redactor
		publicBinding = a.messagesPublicBinding
		bindAfterRedaction = a.messagesDriver != nil
	}
	stampResponseAfterRedaction := false
	defer func() {
		if bindAfterRedaction {
			if stampResponseAfterRedaction {
				result = publicBinding.StampResponse(request, result)
			}
			resultErr = publicBinding.StampError(ctx, request, resultErr, "messages.create")
		}
	}()
	defer func() {
		result = redactor.Response(result)
		resultErr = redactor.Error(resultErr)
	}()
	if ctx == nil {
		return modelinvoker.Response{}, providerError(modelinvoker.ErrorInvalidRequest, "invoke", "context is nil", nil)
	}
	if a == nil || a.messagesDriver == nil || protocol.IsNil(a.client) {
		return modelinvoker.Response{}, providerError(modelinvoker.ErrorProviderUnavailable, "invoke", "adapter is not initialized", nil)
	}
	request.Stream = false
	if err := a.validateSelection(request); err != nil {
		return modelinvoker.Response{}, err
	}
	stampResponseAfterRedaction = true
	result, resultErr = a.messagesDriver.Invoke(ctx, request)
	return result, resultErr
}

func (a *Adapter) Stream(ctx context.Context, request modelinvoker.Request) (result modelinvoker.Stream, resultErr error) {
	redactor := adaptercore.Redactor{}
	publicBinding := protocol.Binding{}
	bindAfterRedaction := false
	if a != nil {
		redactor = a.redactor
		publicBinding = a.messagesPublicBinding
		bindAfterRedaction = a.messagesDriver != nil
	}
	defer func() {
		if bindAfterRedaction {
			result = publicBinding.BindStream(ctx, request, result)
			resultErr = publicBinding.StampError(ctx, request, resultErr, "messages.stream")
		}
	}()
	defer func() {
		result = redactor.Stream(result)
		resultErr = redactor.Error(resultErr)
	}()
	if ctx == nil {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "stream", "context is nil", nil)
	}
	if a == nil || a.messagesDriver == nil || protocol.IsNil(a.client) {
		return nil, providerError(modelinvoker.ErrorProviderUnavailable, "stream", "adapter is not initialized", nil)
	}
	request.Stream = true
	if err := a.validateSelection(request); err != nil {
		return nil, err
	}
	result, resultErr = a.messagesDriver.Stream(ctx, request)
	return result, resultErr
}

func (a *Adapter) validateSelection(request modelinvoker.Request) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if err := adaptercore.ValidateSelection(request, ProviderID, a.baseURL, modelinvoker.ProtocolMessages); err != nil {
		if request.Provider != ProviderID || request.Protocol != modelinvoker.ProtocolMessages {
			return providerError(modelinvoker.ErrorInvalidRequest, "validate", err.Error(), nil)
		}
		if request.Endpoint != "" && adaptercore.NormalizeEndpoint(request.Endpoint) != a.baseURL {
			return mappingError("validate", "request endpoint does not match the configured Anthropic endpoint")
		}
		return mappingError("validate", err.Error())
	}
	return nil
}

var _ modelinvoker.Provider = (*Adapter)(nil)
var _ adaptercore.CandidateBindingReceipt = (*Adapter)(nil)

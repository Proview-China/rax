package gemini

import (
	"context"
	"fmt"
	"io"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/geminigenerate"
)

const ProviderID modelinvoker.ProviderID = "gemini"

type Adapter struct {
	client                geminigenerate.Client
	generateDriver        *geminigenerate.Driver
	generatePublicBinding protocol.Binding
	endpoint              string
	redactor              adaptercore.Redactor
}

func (Adapter) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "gemini.Adapter([REDACTED])")
}

func (Adapter) GoString() string {
	return "gemini.Adapter([REDACTED])"
}

func New(config Config) (*Adapter, error) {
	redactor := adaptercore.NewRedactor(config.APIKey)
	if err := config.validate(); err != nil {
		return nil, redactor.Error(&modelinvoker.Error{
			Kind: modelinvoker.ErrorInvalidRequest, Provider: ProviderID, Operation: "configure",
			Message: err.Error(), Err: err,
		})
	}
	client, err := newSDKClient(config)
	if err != nil {
		return nil, redactor.Error(providerError(modelinvoker.ErrorProviderUnavailable, "configure", "failed to initialize Gemini Developer API client", nil))
	}
	adapter, err := newWithClient(client, effectiveEndpoint(config.effectiveBaseURL(), config.effectiveAPIVersion()), redactor)
	if err != nil {
		return nil, redactor.Error(providerError(modelinvoker.ErrorInvalidRequest, "configure", err.Error(), nil))
	}
	return adapter, nil
}

func newWithClient(client geminigenerate.Client, endpoint string, redactor adaptercore.Redactor) (*Adapter, error) {
	canonical := adaptercore.NormalizeEndpoint(endpoint)
	binding, err := protocol.NewBinding(
		ProviderID, modelinvoker.ProtocolGenerateContent, canonical,
		"x-goog-request-id", "x-request-id", "request-id",
	)
	if err != nil {
		return nil, fmt.Errorf("configure GenerateContent binding: %w", err)
	}
	driver, err := geminigenerate.New(binding, generateDialect{}, client)
	if err != nil {
		return nil, err
	}
	publicBinding := binding.Clone()
	publicBinding.Endpoint = redactor.String(binding.Endpoint)
	return &Adapter{
		client: client, generateDriver: driver, generatePublicBinding: publicBinding,
		endpoint: canonical, redactor: redactor,
	}, nil
}

func (a *Adapter) ID() modelinvoker.ProviderID { return ProviderID }

func (a *Adapter) DefaultProtocol() modelinvoker.Protocol {
	return modelinvoker.ProtocolGenerateContent
}

func (a *Adapter) Capabilities(ctx context.Context, query modelinvoker.CapabilityQuery) (modelinvoker.CapabilityContract, error) {
	if a == nil || protocol.IsNil(a.client) || a.generateDriver == nil {
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
	if query.Protocol != modelinvoker.ProtocolGenerateContent {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "capabilities", fmt.Sprintf("unsupported protocol %q", query.Protocol), nil)
	}
	if query.Endpoint != "" && adaptercore.NormalizeEndpoint(query.Endpoint) != a.endpoint {
		return nil, mappingError("capabilities", "capability endpoint does not match the configured Gemini endpoint")
	}
	return capabilityContract(query), nil
}

func (a *Adapter) Invoke(ctx context.Context, request modelinvoker.Request) (result modelinvoker.Response, resultErr error) {
	redactor := adaptercore.Redactor{}
	publicBinding := protocol.Binding{}
	bindAfterRedaction := false
	if a != nil {
		redactor = a.redactor
		publicBinding = a.generatePublicBinding
		bindAfterRedaction = a.generateDriver != nil
	}
	stampResponseAfterRedaction := false
	defer func() {
		if bindAfterRedaction {
			if stampResponseAfterRedaction {
				result = publicBinding.StampResponse(request, result)
			}
			resultErr = publicBinding.StampError(ctx, request, resultErr, "generate_content.create")
		}
	}()
	defer func() {
		result = redactor.Response(result)
		resultErr = redactor.Error(resultErr)
	}()
	if ctx == nil {
		return modelinvoker.Response{}, providerError(modelinvoker.ErrorInvalidRequest, "invoke", "context is nil", nil)
	}
	if a == nil || a.generateDriver == nil || protocol.IsNil(a.client) {
		return modelinvoker.Response{}, providerError(modelinvoker.ErrorProviderUnavailable, "invoke", "adapter is not initialized", nil)
	}
	request.Stream = false
	if err := a.validateSelection(request); err != nil {
		return modelinvoker.Response{}, err
	}
	stampResponseAfterRedaction = true
	return a.generateDriver.Invoke(ctx, request)
}

func (a *Adapter) Stream(ctx context.Context, request modelinvoker.Request) (result modelinvoker.Stream, resultErr error) {
	redactor := adaptercore.Redactor{}
	publicBinding := protocol.Binding{}
	bindAfterRedaction := false
	if a != nil {
		redactor = a.redactor
		publicBinding = a.generatePublicBinding
		bindAfterRedaction = a.generateDriver != nil
	}
	defer func() {
		if bindAfterRedaction {
			result = publicBinding.BindStream(ctx, request, result)
			resultErr = publicBinding.StampError(ctx, request, resultErr, "generate_content.stream")
		}
	}()
	defer func() {
		result = redactor.Stream(result)
		resultErr = redactor.Error(resultErr)
	}()
	if ctx == nil {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "stream", "context is nil", nil)
	}
	if a == nil || a.generateDriver == nil || protocol.IsNil(a.client) {
		return nil, providerError(modelinvoker.ErrorProviderUnavailable, "stream", "adapter is not initialized", nil)
	}
	request.Stream = true
	if err := a.validateSelection(request); err != nil {
		return nil, err
	}
	return a.generateDriver.Stream(ctx, request)
}

func (a *Adapter) validateSelection(request modelinvoker.Request) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if err := adaptercore.ValidateSelection(request, ProviderID, a.endpoint, modelinvoker.ProtocolGenerateContent); err != nil {
		if request.Provider != ProviderID || request.Protocol != modelinvoker.ProtocolGenerateContent {
			return providerError(modelinvoker.ErrorInvalidRequest, "validate", err.Error(), nil)
		}
		if request.Endpoint != "" && adaptercore.NormalizeEndpoint(request.Endpoint) != a.endpoint {
			return mappingError("validate", "request endpoint does not match the configured Gemini endpoint")
		}
		return mappingError("validate", err.Error())
	}
	return nil
}

var _ modelinvoker.Provider = (*Adapter)(nil)

package bedrockruntime

import (
	"context"
	"fmt"
	"io"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	bedrockprotocol "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/bedrock"
)

const ProviderID modelinvoker.ProviderID = "aws-bedrock-runtime"

type Adapter struct {
	client                         bedrockprotocol.Client
	converse, invoke               *bedrockprotocol.Driver
	converseBinding, invokeBinding protocol.Binding
	endpoint                       string
	redactor                       adaptercore.Redactor
}

func (Adapter) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "bedrockruntime.Adapter([REDACTED])")
}
func (Adapter) GoString() string { return "bedrockruntime.Adapter([REDACTED])" }

func New(config Config) (*Adapter, error) {
	redactor := adaptercore.NewRedactor(config.AccessKeyID, config.SecretAccessKey, config.SessionToken, config.BearerToken)
	if err := config.validate(); err != nil {
		return nil, redactor.Error(providerError(modelinvoker.ErrorInvalidRequest, "configure", err.Error()))
	}
	endpoint, _ := config.trustedEndpoint()
	config.BaseURL = endpoint
	client, err := newSDKClient(config)
	if err != nil {
		return nil, redactor.Error(providerError(modelinvoker.ErrorProviderUnavailable, "configure", "failed to initialize AWS Bedrock Runtime client"))
	}
	return newWithClient(client, endpoint, redactor)
}

func newWithClient(client bedrockprotocol.Client, endpoint string, redactor adaptercore.Redactor) (*Adapter, error) {
	converseBinding, err := protocol.NewBinding(ProviderID, modelinvoker.ProtocolBedrockConverse, endpoint, "x-amzn-requestid")
	if err != nil {
		return nil, err
	}
	converse, err := bedrockprotocol.New(converseBinding, dialect{}, client)
	if err != nil {
		return nil, err
	}
	invokeBinding, err := protocol.NewBinding(ProviderID, modelinvoker.ProtocolBedrockInvoke, endpoint, "x-amzn-requestid")
	if err != nil {
		return nil, err
	}
	invoke, err := bedrockprotocol.New(invokeBinding, dialect{}, client)
	if err != nil {
		return nil, err
	}
	publicConverse, publicInvoke := converseBinding.Clone(), invokeBinding.Clone()
	publicConverse.Endpoint, publicInvoke.Endpoint = redactor.String(publicConverse.Endpoint), redactor.String(publicInvoke.Endpoint)
	return &Adapter{client: client, converse: converse, invoke: invoke, converseBinding: publicConverse, invokeBinding: publicInvoke, endpoint: endpoint, redactor: redactor}, nil
}

func (a *Adapter) ID() modelinvoker.ProviderID { return ProviderID }
func (a *Adapter) DefaultProtocol() modelinvoker.Protocol {
	return modelinvoker.ProtocolBedrockConverse
}
func (a *Adapter) CandidateBindingEndpoint(protocolID modelinvoker.Protocol, _ string) (string, bool) {
	if a == nil || a.endpoint == "" || (protocolID != modelinvoker.ProtocolBedrockConverse && protocolID != modelinvoker.ProtocolBedrockInvoke) {
		return "", false
	}
	return a.endpoint, true
}

func (a *Adapter) Capabilities(ctx context.Context, query modelinvoker.CapabilityQuery) (modelinvoker.CapabilityContract, error) {
	if a == nil || protocol.IsNil(a.client) {
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
	if query.Endpoint != "" && adaptercore.NormalizeEndpoint(query.Endpoint) != a.endpoint {
		return nil, providerError(modelinvoker.ErrorMapping, "capabilities", "endpoint does not match configured Bedrock Runtime")
	}
	contract := adaptercore.UnsupportedContract("outside the implemented Bedrock Runtime offline slice")
	switch query.Protocol {
	case modelinvoker.ProtocolBedrockConverse:
		adaptercore.SetSupport(contract, query, modelinvoker.SupportCompatible, "mapped through Bedrock Converse", modelinvoker.CapabilityTextGeneration, modelinvoker.CapabilityStreaming, modelinvoker.CapabilityToolCalling, modelinvoker.CapabilityFunctionErrorResult, modelinvoker.CapabilityUsageReporting)
		contract[modelinvoker.CapabilityParallelToolCalling] = adaptercore.QuerySupport(query, modelinvoker.SupportPartial, "support is selected by the concrete Bedrock model")
	case modelinvoker.ProtocolBedrockInvoke:
		contract[modelinvoker.CapabilityTextGeneration] = adaptercore.QuerySupport(query, modelinvoker.SupportPartial, "provider-native JSON body and response remain in RawPayload")
		contract[modelinvoker.CapabilityStreaming] = adaptercore.QuerySupport(query, modelinvoker.SupportPartial, "provider-native chunks remain native events")
		contract[modelinvoker.CapabilityUsageReporting] = adaptercore.QuerySupport(query, modelinvoker.SupportUnsupported, "InvokeModel response shape is model-specific")
	default:
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "capabilities", "unsupported protocol")
	}
	return contract, nil
}

func (a *Adapter) driver(protocolID modelinvoker.Protocol) (*bedrockprotocol.Driver, protocol.Binding, bool) {
	if a == nil {
		return nil, protocol.Binding{}, false
	}
	switch protocolID {
	case modelinvoker.ProtocolBedrockConverse:
		return a.converse, a.converseBinding, a.converse != nil
	case modelinvoker.ProtocolBedrockInvoke:
		return a.invoke, a.invokeBinding, a.invoke != nil
	}
	return nil, protocol.Binding{}, false
}

func (a *Adapter) Invoke(ctx context.Context, request modelinvoker.Request) (result modelinvoker.Response, resultErr error) {
	driver, binding, ok := a.driver(request.Protocol)
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
	if !ok || protocol.IsNil(a.client) {
		return modelinvoker.Response{}, providerError(modelinvoker.ErrorInvalidRequest, "invoke", "unsupported or uninitialized Bedrock protocol")
	}
	request.Stream = false
	return driver.Invoke(ctx, request)
}

func (a *Adapter) Stream(ctx context.Context, request modelinvoker.Request) (result modelinvoker.Stream, resultErr error) {
	driver, binding, ok := a.driver(request.Protocol)
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
	if !ok || protocol.IsNil(a.client) {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "stream", "unsupported or uninitialized Bedrock protocol")
	}
	request.Stream = true
	return driver.Stream(ctx, request)
}

func providerError(kind modelinvoker.ErrorKind, operation, message string) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: kind, Provider: ProviderID, Operation: operation, Message: message}
}

var _ modelinvoker.Provider = (*Adapter)(nil)
var _ adaptercore.CandidateBindingReceipt = (*Adapter)(nil)

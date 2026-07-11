package azureopenai

import (
	"context"
	"fmt"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openaichat"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openairesponses"
	"io"
	"strings"
)

const ProviderID modelinvoker.ProviderID = "azure-openai"

type Adapter struct {
	v1Chat      *openaichat.Driver
	v1Responses *openairesponses.Driver
	legacyChat  *openaichat.Driver
	bindings    map[string]protocol.Binding
	endpoints   map[string]string
	deployment  string
	redactor    adaptercore.Redactor
}

const (
	v1ChatKey      = "v1-chat"
	v1ResponsesKey = "v1-responses"
	legacyChatKey  = "legacy-chat"
)

func (Adapter) Format(s fmt.State, _ rune) {
	_, _ = io.WriteString(s, "azureopenai.Adapter([REDACTED])")
}
func (Adapter) GoString() string { return "azureopenai.Adapter([REDACTED])" }
func New(c Config) (*Adapter, error) {
	red := adaptercore.NewRedactor(c.APIKey)
	if err := c.validate(); err != nil {
		return nil, red.Error(providerError(modelinvoker.ErrorInvalidRequest, "configure", err.Error()))
	}
	chatClient, responsesClient := newV1Clients(c)
	root := strings.TrimRight(c.root(), "/")
	v1Endpoint := root + "/openai/v1"
	chatBinding, err := protocol.NewBinding(ProviderID, modelinvoker.ProtocolChatCompletions, v1Endpoint, "x-request-id", "apim-request-id")
	if err != nil {
		return nil, err
	}
	responsesBinding, err := protocol.NewBinding(ProviderID, modelinvoker.ProtocolResponses, v1Endpoint, "x-request-id", "apim-request-id")
	if err != nil {
		return nil, err
	}
	v1Chat, err := openaichat.New(chatBinding, dialect{deployment: c.DeploymentName}, chatClient)
	if err != nil {
		return nil, err
	}
	v1Responses, err := openairesponses.New(responsesBinding, dialect{deployment: c.DeploymentName}, responsesClient)
	if err != nil {
		return nil, err
	}
	a := &Adapter{v1Chat: v1Chat, v1Responses: v1Responses, bindings: map[string]protocol.Binding{}, endpoints: map[string]string{}, deployment: c.DeploymentName, redactor: red}
	a.add(v1ChatKey, chatBinding)
	a.add(v1ResponsesKey, responsesBinding)
	if c.EnableLegacy {
		legacyEndpoint := root + "/openai/deployments/" + urlPathSegment(c.DeploymentName)
		legacyBinding, e := protocol.NewBinding(ProviderID, modelinvoker.ProtocolChatCompletions, legacyEndpoint, "x-request-id", "apim-request-id")
		if e != nil {
			return nil, e
		}
		a.legacyChat, e = openaichat.New(legacyBinding, dialect{deployment: c.DeploymentName, legacy: true}, newLegacyChatClient(c))
		if e != nil {
			return nil, e
		}
		a.add(legacyChatKey, legacyBinding)
	}
	return a, nil
}
func (a *Adapter) add(key string, b protocol.Binding) {
	public := b.Clone()
	public.Endpoint = a.redactor.String(public.Endpoint)
	a.bindings[key] = public
	a.endpoints[key] = b.Endpoint
}
func (a *Adapter) ID() modelinvoker.ProviderID            { return ProviderID }
func (a *Adapter) DefaultProtocol() modelinvoker.Protocol { return modelinvoker.ProtocolResponses }
func (a *Adapter) selectDriver(r modelinvoker.Request) (protocol.Driver, protocol.Binding, bool) {
	if a == nil {
		return nil, protocol.Binding{}, false
	}
	switch r.Protocol {
	case modelinvoker.ProtocolResponses:
		return a.v1Responses, a.bindings[v1ResponsesKey], a.v1Responses != nil
	case modelinvoker.ProtocolChatCompletions:
		if r.Endpoint != "" && a.legacyChat != nil && adaptercore.NormalizeEndpoint(r.Endpoint) == a.endpoints[legacyChatKey] {
			return a.legacyChat, a.bindings[legacyChatKey], true
		}
		return a.v1Chat, a.bindings[v1ChatKey], a.v1Chat != nil
	}
	return nil, protocol.Binding{}, false
}
func (a *Adapter) Capabilities(ctx context.Context, q modelinvoker.CapabilityQuery) (modelinvoker.CapabilityContract, error) {
	if a == nil {
		return nil, providerError(modelinvoker.ErrorProviderUnavailable, "capabilities", "adapter is not initialized")
	}
	if ctx == nil {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "capabilities", "context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, adaptercore.ContextError(ProviderID, "capabilities", err)
	}
	if strings.TrimSpace(q.Model) == "" {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "capabilities", "deployment name is required")
	}
	if q.Model != a.deployment {
		return nil, mappingError("capabilities", "model must equal configured Azure deployment name")
	}
	key := v1ResponsesKey
	if q.Protocol == modelinvoker.ProtocolChatCompletions {
		key = v1ChatKey
		if q.Endpoint != "" && a.legacyChat != nil && adaptercore.NormalizeEndpoint(q.Endpoint) == a.endpoints[legacyChatKey] {
			key = legacyChatKey
		}
	} else if q.Protocol != modelinvoker.ProtocolResponses {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "capabilities", "unsupported protocol")
	}
	if q.Endpoint != "" && adaptercore.NormalizeEndpoint(q.Endpoint) != a.endpoints[key] {
		return nil, mappingError("capabilities", "endpoint does not match Azure binding")
	}
	contract := adaptercore.UnsupportedContract("capability must be verified for the configured Azure deployment, region, and GA/Preview state")
	adaptercore.SetSupport(contract, q, modelinvoker.SupportCompatible, "mapped through Azure OpenAI", modelinvoker.CapabilityTextGeneration, modelinvoker.CapabilityStreaming, modelinvoker.CapabilityToolCalling, modelinvoker.CapabilityUsageReporting)
	if key == v1ResponsesKey {
		contract[modelinvoker.CapabilityServerState] = adaptercore.QuerySupport(q, modelinvoker.SupportCompatible, "Azure OpenAI v1 Responses state is deployment-dependent")
	}
	return contract, nil
}
func (a *Adapter) Invoke(ctx context.Context, r modelinvoker.Request) (out modelinvoker.Response, err error) {
	d, b, ok := a.selectDriver(r)
	red := adaptercore.Redactor{}
	if a != nil {
		red = a.redactor
	}
	defer func() {
		out = red.Response(out)
		err = red.Error(err)
		if ok {
			out = b.StampResponse(r, out)
			err = b.StampError(ctx, r, err, "invoke")
		}
	}()
	if ctx == nil {
		return out, providerError(modelinvoker.ErrorInvalidRequest, "invoke", "context is nil")
	}
	if !ok {
		return out, providerError(modelinvoker.ErrorInvalidRequest, "invoke", "unsupported protocol")
	}
	r.Stream = false
	return d.Invoke(ctx, r)
}
func (a *Adapter) Stream(ctx context.Context, r modelinvoker.Request) (out modelinvoker.Stream, err error) {
	d, b, ok := a.selectDriver(r)
	red := adaptercore.Redactor{}
	if a != nil {
		red = a.redactor
	}
	defer func() {
		out = red.Stream(out)
		err = red.Error(err)
		if ok {
			out = b.BindStream(ctx, r, out)
			err = b.StampError(ctx, r, err, "stream")
		}
	}()
	if ctx == nil {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "stream", "context is nil")
	}
	if !ok {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "stream", "unsupported protocol")
	}
	r.Stream = true
	return d.Stream(ctx, r)
}
func providerError(k modelinvoker.ErrorKind, op, msg string) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: k, Provider: ProviderID, Operation: op, Message: msg}
}

var _ modelinvoker.Provider = (*Adapter)(nil)

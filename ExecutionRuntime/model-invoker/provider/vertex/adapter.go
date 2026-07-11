package vertex

import (
	"context"
	"fmt"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/anthropicmessages"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/geminigenerate"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openaichat"
	"io"
	"strings"
)

const ProviderID modelinvoker.ProviderID = "google-vertex-ai"

type Adapter struct {
	generate  *geminigenerate.Driver
	chat      *openaichat.Driver
	messages  *anthropicmessages.Driver
	bindings  map[modelinvoker.Protocol]protocol.Binding
	endpoints map[modelinvoker.Protocol]string
	redactor  adaptercore.Redactor
}

func (Adapter) Format(s fmt.State, _ rune) { _, _ = io.WriteString(s, "vertex.Adapter([REDACTED])") }
func (Adapter) GoString() string           { return "vertex.Adapter([REDACTED])" }
func New(c Config) (*Adapter, error) {
	redactor := adaptercore.NewRedactor(c.APIKey)
	if err := c.validate(); err != nil {
		return nil, redactor.Error(providerError(modelinvoker.ErrorInvalidRequest, "configure", err.Error()))
	}
	geminiClient, chatClient, messagesClient, err := newSDKClients(c)
	if err != nil {
		return nil, redactor.Error(providerError(modelinvoker.ErrorProviderUnavailable, "configure", "failed to initialize Vertex SDK clients"))
	}
	root := strings.TrimRight(c.rootEndpoint(), "/")
	generateEndpoint := root + "/v1"
	chatEndpoint := root + "/v1beta1/projects/" + c.Project + "/locations/" + c.Location + "/endpoints/openapi"
	messagesEndpoint := root + "/v1/projects/" + c.Project + "/locations/" + c.Location + "/publishers/anthropic"
	gb, err := protocol.NewBinding(ProviderID, modelinvoker.ProtocolGenerateContent, generateEndpoint, "x-goog-request-id", "x-request-id")
	if err != nil {
		return nil, err
	}
	cb, err := protocol.NewBinding(ProviderID, modelinvoker.ProtocolChatCompletions, chatEndpoint, "x-goog-request-id", "x-request-id")
	if err != nil {
		return nil, err
	}
	g, err := geminigenerate.New(gb, dialect{}, geminiClient)
	if err != nil {
		return nil, err
	}
	chat, err := openaichat.New(cb, dialect{}, chatClient)
	if err != nil {
		return nil, err
	}
	bindings := map[modelinvoker.Protocol]protocol.Binding{}
	endpoints := map[modelinvoker.Protocol]string{}
	for id, b := range map[modelinvoker.Protocol]protocol.Binding{modelinvoker.ProtocolGenerateContent: gb, modelinvoker.ProtocolChatCompletions: cb} {
		public := b.Clone()
		public.Endpoint = redactor.String(public.Endpoint)
		bindings[id] = public
		endpoints[id] = b.Endpoint
	}
	var messages *anthropicmessages.Driver
	if messagesClient != nil {
		mb, e := protocol.NewBinding(ProviderID, modelinvoker.ProtocolMessages, messagesEndpoint, "x-goog-request-id", "x-request-id")
		if e != nil {
			return nil, e
		}
		messages, e = anthropicmessages.New(mb, dialect{messages: true}, messagesClient)
		if e != nil {
			return nil, e
		}
		public := mb.Clone()
		public.Endpoint = redactor.String(public.Endpoint)
		bindings[modelinvoker.ProtocolMessages] = public
		endpoints[modelinvoker.ProtocolMessages] = mb.Endpoint
	}
	return &Adapter{generate: g, chat: chat, messages: messages, bindings: bindings, endpoints: endpoints, redactor: redactor}, nil
}
func (a *Adapter) ID() modelinvoker.ProviderID { return ProviderID }
func (a *Adapter) DefaultProtocol() modelinvoker.Protocol {
	return modelinvoker.ProtocolGenerateContent
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
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "capabilities", "model is required")
	}
	endpoint, ok := a.endpoints[q.Protocol]
	if !ok {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "capabilities", "protocol unavailable for configured credential mode")
	}
	if q.Endpoint != "" && adaptercore.NormalizeEndpoint(q.Endpoint) != endpoint {
		return nil, mappingError("capabilities", "endpoint does not match Vertex protocol binding")
	}
	contract := adaptercore.UnsupportedContract("outside the selected Vertex model and deployment capability card")
	adaptercore.SetSupport(contract, q, modelinvoker.SupportCompatible, "mapped through the selected Vertex protocol", modelinvoker.CapabilityTextGeneration, modelinvoker.CapabilityStreaming, modelinvoker.CapabilityToolCalling, modelinvoker.CapabilityUsageReporting)
	return contract, nil
}
func (a *Adapter) selected(id modelinvoker.Protocol) (protocol.Driver, protocol.Binding, bool) {
	if a == nil {
		return nil, protocol.Binding{}, false
	}
	switch id {
	case modelinvoker.ProtocolGenerateContent:
		return a.generate, a.bindings[id], a.generate != nil
	case modelinvoker.ProtocolChatCompletions:
		return a.chat, a.bindings[id], a.chat != nil
	case modelinvoker.ProtocolMessages:
		return a.messages, a.bindings[id], a.messages != nil
	}
	return nil, protocol.Binding{}, false
}
func (a *Adapter) Invoke(ctx context.Context, r modelinvoker.Request) (out modelinvoker.Response, err error) {
	d, b, ok := a.selected(r.Protocol)
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
		return out, providerError(modelinvoker.ErrorInvalidRequest, "invoke", "protocol unavailable")
	}
	r.Stream = false
	return d.Invoke(ctx, r)
}
func (a *Adapter) Stream(ctx context.Context, r modelinvoker.Request) (out modelinvoker.Stream, err error) {
	d, b, ok := a.selected(r.Protocol)
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
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "stream", "protocol unavailable")
	}
	r.Stream = true
	return d.Stream(ctx, r)
}
func providerError(k modelinvoker.ErrorKind, op, msg string) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: k, Provider: ProviderID, Operation: op, Message: msg}
}

var _ modelinvoker.Provider = (*Adapter)(nil)

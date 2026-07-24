package mcp

import (
	"context"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type LocalHandler func(context.Context, Message) (Message, error)

// LocalTransport is an in-memory, test-only transport. It performs no network,
// process, credential or filesystem access and is not a production backend.
type LocalTransport struct {
	mu      sync.RWMutex
	handler LocalHandler
	closed  bool
}

func NewLocalTransport(handler LocalHandler) (*LocalTransport, error) {
	if handler == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "local MCP handler is required")
	}
	return &LocalTransport{handler: handler}, nil
}

func (t *LocalTransport) RoundTrip(ctx context.Context, request Message) (Message, error) {
	if err := request.Validate(); err != nil {
		return Message{}, err
	}
	t.mu.RLock()
	if t.closed {
		t.mu.RUnlock()
		return Message{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "local MCP transport is closed")
	}
	handler := t.handler
	t.mu.RUnlock()
	response, err := handler(ctx, request)
	if err != nil {
		return Message{}, err
	}
	if err := response.Validate(); err != nil {
		return Message{}, err
	}
	if len(request.ID) != 0 && string(request.ID) != string(response.ID) {
		return Message{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "local MCP response id differs from request")
	}
	return response, nil
}

func (t *LocalTransport) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
}

func ConnectExternal(context.Context, contract.MCPServerDescriptor) error {
	return contract.ErrExternalEffectUnsupported
}

func DiscoverExternal(context.Context, contract.MCPConnectionRef) error {
	return contract.ErrExternalEffectUnsupported
}

func InvokeExternal(context.Context, contract.MCPConnectionRef, Message) error {
	return contract.ErrExternalEffectUnsupported
}

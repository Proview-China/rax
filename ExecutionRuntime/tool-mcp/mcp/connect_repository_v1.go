package mcp

import (
	"context"
	"reflect"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type EnsureMCPTransportConfigRequestV1 struct {
	Config          toolcontract.MCPTransportConfigV1     `json:"config"`
	ExpectedCurrent *toolcontract.MCPTransportConfigRefV1 `json:"expected_current,omitempty"`
}

func (r EnsureMCPTransportConfigRequestV1) Validate() error {
	if r.Config.Validate() != nil {
		return invalid("MCP Transport Config Ensure request is invalid")
	}
	if r.ExpectedCurrent == nil {
		if r.Config.Ref.Revision != 1 {
			return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "MCP Transport Config create must start at revision 1")
		}
		return nil
	}
	if r.ExpectedCurrent.Validate() != nil || r.ExpectedCurrent.ID != r.Config.Ref.ID || r.Config.Ref.Revision != r.ExpectedCurrent.Revision+1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "MCP Transport Config successor request drifted")
	}
	return nil
}

type MCPTransportConfigReaderV1 interface {
	InspectMCPTransportConfigV1(context.Context, toolcontract.MCPTransportConfigRefV1) (toolcontract.MCPTransportConfigV1, error)
	InspectCurrentMCPTransportConfigV1(context.Context, string) (toolcontract.MCPTransportConfigV1, error)
}

type MCPTransportConfigRepositoryV1 interface {
	MCPTransportConfigReaderV1
	EnsureMCPTransportConfigV1(context.Context, EnsureMCPTransportConfigRequestV1) (toolcontract.MCPTransportConfigV1, error)
}

type InMemoryMCPTransportConfigRepositoryV1 struct {
	mu      sync.RWMutex
	history map[string]toolcontract.MCPTransportConfigV1
	current map[string]toolcontract.MCPTransportConfigRefV1
}

func NewInMemoryMCPTransportConfigRepositoryV1() *InMemoryMCPTransportConfigRepositoryV1 {
	return &InMemoryMCPTransportConfigRepositoryV1{history: make(map[string]toolcontract.MCPTransportConfigV1), current: make(map[string]toolcontract.MCPTransportConfigRefV1)}
}

func (r *InMemoryMCPTransportConfigRepositoryV1) EnsureMCPTransportConfigV1(ctx context.Context, request EnsureMCPTransportConfigRequestV1) (toolcontract.MCPTransportConfigV1, error) {
	if err := connectRepositoryContextV1(ctx); err != nil {
		return toolcontract.MCPTransportConfigV1{}, err
	}
	if r == nil {
		return toolcontract.MCPTransportConfigV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Transport Config repository is unavailable")
	}
	if err := request.Validate(); err != nil {
		return toolcontract.MCPTransportConfigV1{}, err
	}
	config := cloneMCPTransportConfigV1(request.Config)
	key := connectHistoryKeyV1(config.Ref.ObjectRef())
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return toolcontract.MCPTransportConfigV1{}, err
	}
	current, exists := r.current[config.Ref.ID]
	if winner, ok := r.history[key]; ok {
		if current != config.Ref || !reflect.DeepEqual(winner, config) {
			return toolcontract.MCPTransportConfigV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "MCP Transport Config history cannot replace current")
		}
		return cloneMCPTransportConfigV1(winner), nil
	}
	if !exists {
		if request.ExpectedCurrent != nil || config.Ref.Revision != 1 {
			return toolcontract.MCPTransportConfigV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "MCP Transport Config current is absent")
		}
	} else if request.ExpectedCurrent == nil || current != *request.ExpectedCurrent || config.Ref.Revision != current.Revision+1 {
		return toolcontract.MCPTransportConfigV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "MCP Transport Config current changed")
	}
	r.history[key] = config
	r.current[config.Ref.ID] = config.Ref
	return cloneMCPTransportConfigV1(config), nil
}

func (r *InMemoryMCPTransportConfigRepositoryV1) InspectMCPTransportConfigV1(ctx context.Context, exact toolcontract.MCPTransportConfigRefV1) (toolcontract.MCPTransportConfigV1, error) {
	if err := connectRepositoryContextV1(ctx); err != nil {
		return toolcontract.MCPTransportConfigV1{}, err
	}
	if r == nil || exact.Validate() != nil {
		return toolcontract.MCPTransportConfigV1{}, invalid("MCP Transport Config exact Inspect is invalid")
	}
	r.mu.RLock()
	config, ok := r.history[connectHistoryKeyV1(exact.ObjectRef())]
	r.mu.RUnlock()
	if !ok {
		return toolcontract.MCPTransportConfigV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Transport Config not found")
	}
	if config.Ref != exact {
		return toolcontract.MCPTransportConfigV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Transport Config exact Ref drifted")
	}
	return cloneMCPTransportConfigV1(config), nil
}

func (r *InMemoryMCPTransportConfigRepositoryV1) InspectCurrentMCPTransportConfigV1(ctx context.Context, id string) (toolcontract.MCPTransportConfigV1, error) {
	if err := connectRepositoryContextV1(ctx); err != nil {
		return toolcontract.MCPTransportConfigV1{}, err
	}
	if r == nil || toolcontract.ValidateStableID(id) != nil {
		return toolcontract.MCPTransportConfigV1{}, invalid("MCP Transport Config current Inspect is invalid")
	}
	r.mu.RLock()
	exact, ok := r.current[id]
	config := r.history[connectHistoryKeyV1(exact.ObjectRef())]
	r.mu.RUnlock()
	if !ok {
		return toolcontract.MCPTransportConfigV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "current MCP Transport Config not found")
	}
	if config.Ref != exact {
		return toolcontract.MCPTransportConfigV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "current MCP Transport Config index drifted")
	}
	return cloneMCPTransportConfigV1(config), nil
}

type EnsureMCPConnectIntentRequestV1 struct {
	Intent          toolcontract.MCPConnectIntentV1 `json:"intent"`
	ExpectedCurrent *toolcontract.ObjectRef         `json:"expected_current,omitempty"`
}

func (r EnsureMCPConnectIntentRequestV1) Validate() error {
	if r.Intent.Validate() != nil {
		return invalid("MCP Connect Intent Ensure request is invalid")
	}
	if r.ExpectedCurrent == nil {
		if r.Intent.Ref.Revision != 1 {
			return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "MCP Connect Intent create must start at revision 1")
		}
		return nil
	}
	if r.ExpectedCurrent.Validate() != nil || r.ExpectedCurrent.ID != r.Intent.Ref.ID || r.Intent.Ref.Revision != r.ExpectedCurrent.Revision+1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "MCP Connect Intent successor request drifted")
	}
	return nil
}

type MCPConnectIntentReaderV1 interface {
	InspectMCPConnectIntentV1(context.Context, toolcontract.ObjectRef) (toolcontract.MCPConnectIntentV1, error)
	InspectCurrentMCPConnectIntentV1(context.Context, string) (toolcontract.MCPConnectIntentV1, error)
}

type MCPConnectIntentRepositoryV1 interface {
	MCPConnectIntentReaderV1
	EnsureMCPConnectIntentV1(context.Context, EnsureMCPConnectIntentRequestV1) (toolcontract.MCPConnectIntentV1, error)
}

type InMemoryMCPConnectIntentRepositoryV1 struct {
	mu      sync.RWMutex
	history map[string]toolcontract.MCPConnectIntentV1
	current map[string]toolcontract.ObjectRef
}

func NewInMemoryMCPConnectIntentRepositoryV1() *InMemoryMCPConnectIntentRepositoryV1 {
	return &InMemoryMCPConnectIntentRepositoryV1{history: make(map[string]toolcontract.MCPConnectIntentV1), current: make(map[string]toolcontract.ObjectRef)}
}

func (r *InMemoryMCPConnectIntentRepositoryV1) EnsureMCPConnectIntentV1(ctx context.Context, request EnsureMCPConnectIntentRequestV1) (toolcontract.MCPConnectIntentV1, error) {
	if err := connectRepositoryContextV1(ctx); err != nil {
		return toolcontract.MCPConnectIntentV1{}, err
	}
	if r == nil {
		return toolcontract.MCPConnectIntentV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Connect Intent repository is unavailable")
	}
	if err := request.Validate(); err != nil {
		return toolcontract.MCPConnectIntentV1{}, err
	}
	intent := cloneMCPConnectIntentV1(request.Intent)
	key := connectHistoryKeyV1(intent.Ref)
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return toolcontract.MCPConnectIntentV1{}, err
	}
	current, exists := r.current[intent.Ref.ID]
	if winner, ok := r.history[key]; ok {
		if current != intent.Ref || !reflect.DeepEqual(winner, intent) {
			return toolcontract.MCPConnectIntentV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "MCP Connect Intent history cannot replace current")
		}
		return cloneMCPConnectIntentV1(winner), nil
	}
	if !exists {
		if request.ExpectedCurrent != nil || intent.Ref.Revision != 1 {
			return toolcontract.MCPConnectIntentV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "MCP Connect Intent current is absent")
		}
	} else if request.ExpectedCurrent == nil || current != *request.ExpectedCurrent || intent.Ref.Revision != current.Revision+1 {
		return toolcontract.MCPConnectIntentV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "MCP Connect Intent current changed")
	}
	r.history[key] = intent
	r.current[intent.Ref.ID] = intent.Ref
	return cloneMCPConnectIntentV1(intent), nil
}

func (r *InMemoryMCPConnectIntentRepositoryV1) InspectMCPConnectIntentV1(ctx context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPConnectIntentV1, error) {
	if err := connectRepositoryContextV1(ctx); err != nil {
		return toolcontract.MCPConnectIntentV1{}, err
	}
	if r == nil || exact.Validate() != nil {
		return toolcontract.MCPConnectIntentV1{}, invalid("MCP Connect Intent exact Inspect is invalid")
	}
	r.mu.RLock()
	intent, ok := r.history[connectHistoryKeyV1(exact)]
	r.mu.RUnlock()
	if !ok {
		return toolcontract.MCPConnectIntentV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Connect Intent not found")
	}
	if intent.Ref != exact {
		return toolcontract.MCPConnectIntentV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Connect Intent exact Ref drifted")
	}
	return cloneMCPConnectIntentV1(intent), nil
}

func (r *InMemoryMCPConnectIntentRepositoryV1) InspectCurrentMCPConnectIntentV1(ctx context.Context, id string) (toolcontract.MCPConnectIntentV1, error) {
	if err := connectRepositoryContextV1(ctx); err != nil {
		return toolcontract.MCPConnectIntentV1{}, err
	}
	if r == nil || toolcontract.ValidateStableID(id) != nil {
		return toolcontract.MCPConnectIntentV1{}, invalid("MCP Connect Intent current Inspect is invalid")
	}
	r.mu.RLock()
	exact, ok := r.current[id]
	intent := r.history[connectHistoryKeyV1(exact)]
	r.mu.RUnlock()
	if !ok {
		return toolcontract.MCPConnectIntentV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "current MCP Connect Intent not found")
	}
	if intent.Ref != exact {
		return toolcontract.MCPConnectIntentV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "current MCP Connect Intent index drifted")
	}
	return cloneMCPConnectIntentV1(intent), nil
}

func cloneMCPTransportConfigV1(config toolcontract.MCPTransportConfigV1) toolcontract.MCPTransportConfigV1 {
	if config.Stdio != nil {
		value := *config.Stdio
		value.Arguments = append([]string(nil), value.Arguments...)
		value.CredentialPlaceholders = append([]string(nil), value.CredentialPlaceholders...)
		config.Stdio = &value
	}
	if config.StreamableHTTP != nil {
		value := *config.StreamableHTTP
		config.StreamableHTTP = &value
	}
	return config
}

func cloneMCPConnectIntentV1(intent toolcontract.MCPConnectIntentV1) toolcontract.MCPConnectIntentV1 {
	intent.CredentialLeases = append([]runtimeports.CredentialLeaseRefV2(nil), intent.CredentialLeases...)
	return intent
}

func connectHistoryKeyV1(exact toolcontract.ObjectRef) string {
	return exact.ID + "\x00" + string(exact.Digest)
}

func connectRepositoryContextV1(ctx context.Context) error {
	if ctx == nil {
		return invalid("MCP Connect repository context is required")
	}
	return ctx.Err()
}

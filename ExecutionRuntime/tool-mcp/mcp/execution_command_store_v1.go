package mcp

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type InMemoryMCPExecutionCommandRepositoryV1 struct {
	mu       sync.RWMutex
	facts    map[string]toolcontract.MCPExecutionCommandFactV1
	attempts map[string]toolcontract.MCPExecutionCommandRefV1
	clock    func() time.Time
}

func NewInMemoryMCPExecutionCommandRepositoryV1(clock func() time.Time) (*InMemoryMCPExecutionCommandRepositoryV1, error) {
	if clock == nil {
		return nil, invalid("MCP execution command repository clock is missing")
	}
	return &InMemoryMCPExecutionCommandRepositoryV1{facts: make(map[string]toolcontract.MCPExecutionCommandFactV1), attempts: make(map[string]toolcontract.MCPExecutionCommandRefV1), clock: clock}, nil
}

func (r *InMemoryMCPExecutionCommandRepositoryV1) CreateMCPExecutionCommandV1(ctx context.Context, fact toolcontract.MCPExecutionCommandFactV1) (toolcontract.MCPExecutionCommandFactV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPExecutionCommandFactV1{}, err
	}
	if r == nil || r.clock == nil {
		return toolcontract.MCPExecutionCommandFactV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP execution command repository is unavailable")
	}
	if err := fact.Validate(); err != nil {
		return toolcontract.MCPExecutionCommandFactV1{}, err
	}
	now := r.clock()
	if err := fact.ValidateCurrent(now); err != nil {
		return toolcontract.MCPExecutionCommandFactV1{}, err
	}
	fact = toolcontract.CloneMCPExecutionCommandFactV1(fact)
	attemptKey, err := mcpExecutionAttemptKeyV1(fact.Attempt)
	if err != nil {
		return toolcontract.MCPExecutionCommandFactV1{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.facts[fact.Ref.ID]; ok {
		if existing.Ref != fact.Ref || !reflect.DeepEqual(existing, fact) {
			return toolcontract.MCPExecutionCommandFactV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "MCP execution command ID already binds another command")
		}
		return toolcontract.CloneMCPExecutionCommandFactV1(existing), nil
	}
	if existingRef, ok := r.attempts[attemptKey]; ok && existingRef != fact.Ref {
		return toolcontract.MCPExecutionCommandFactV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Runtime Attempt already binds another MCP execution command")
	}
	r.facts[fact.Ref.ID] = fact
	r.attempts[attemptKey] = fact.Ref
	return toolcontract.CloneMCPExecutionCommandFactV1(fact), nil
}

func (r *InMemoryMCPExecutionCommandRepositoryV1) InspectMCPExecutionCommandV1(ctx context.Context, exact toolcontract.MCPExecutionCommandRefV1) (toolcontract.MCPExecutionCommandFactV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPExecutionCommandFactV1{}, err
	}
	if r == nil {
		return toolcontract.MCPExecutionCommandFactV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP execution command repository is unavailable")
	}
	if err := exact.Validate(); err != nil {
		return toolcontract.MCPExecutionCommandFactV1{}, err
	}
	r.mu.RLock()
	fact, ok := r.facts[exact.ID]
	r.mu.RUnlock()
	if !ok {
		return toolcontract.MCPExecutionCommandFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP execution command not found")
	}
	if fact.Ref != exact {
		return toolcontract.MCPExecutionCommandFactV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "MCP execution command exact Ref drifted")
	}
	return toolcontract.CloneMCPExecutionCommandFactV1(fact), nil
}

func (r *InMemoryMCPExecutionCommandRepositoryV1) InspectCurrentMCPExecutionCommandV1(ctx context.Context, exact toolcontract.MCPExecutionCommandRefV1) (toolcontract.MCPExecutionCommandCurrentProjectionV1, error) {
	fact, err := r.InspectMCPExecutionCommandV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPExecutionCommandCurrentProjectionV1{}, err
	}
	if r == nil || r.clock == nil {
		return toolcontract.MCPExecutionCommandCurrentProjectionV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP execution command current clock is unavailable")
	}
	now := r.clock()
	if err = fact.ValidateCurrent(now); err != nil {
		return toolcontract.MCPExecutionCommandCurrentProjectionV1{}, err
	}
	return toolcontract.SealMCPExecutionCommandCurrentProjectionV1(toolcontract.MCPExecutionCommandCurrentProjectionV1{Fact: fact, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: fact.NotAfterUnixNano})
}

func (r *InMemoryMCPExecutionCommandRepositoryV1) InspectMCPExecutionCommandByAttemptV1(ctx context.Context, attempt runtimeports.OperationDispatchAttemptRefV3) (toolcontract.MCPExecutionCommandFactV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPExecutionCommandFactV1{}, err
	}
	if r == nil || attempt.Validate() != nil {
		return toolcontract.MCPExecutionCommandFactV1{}, invalid("MCP execution command Attempt lookup is invalid")
	}
	key, err := mcpExecutionAttemptKeyV1(attempt)
	if err != nil {
		return toolcontract.MCPExecutionCommandFactV1{}, err
	}
	r.mu.RLock()
	ref, ok := r.attempts[key]
	fact := r.facts[ref.ID]
	r.mu.RUnlock()
	if !ok {
		return toolcontract.MCPExecutionCommandFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP execution command Attempt not found")
	}
	if fact.Ref != ref || fact.Attempt != attempt {
		return toolcontract.MCPExecutionCommandFactV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP execution command Attempt index drifted")
	}
	return toolcontract.CloneMCPExecutionCommandFactV1(fact), nil
}

func mcpExecutionAttemptKeyV1(attempt runtimeports.OperationDispatchAttemptRefV3) (string, error) {
	if attempt.Validate() != nil {
		return "", invalid("MCP execution command Attempt is invalid")
	}
	digest, err := core.CanonicalJSONDigest("praxis.tool-mcp.mcp-execution", toolcontract.MCPExecutionCommandContractVersionV1, "MCPExecutionCommandAttemptIndexV1", attempt)
	if err != nil {
		return "", err
	}
	return string(digest), nil
}

func requireMCPExecutionContextV1(ctx context.Context) error {
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP execution context is nil")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

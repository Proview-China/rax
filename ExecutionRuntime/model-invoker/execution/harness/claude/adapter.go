package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/streamjson"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

type preparedExecution struct {
	client        *streamjson.Client
	init          InitMessage
	manifest      union.ContextManifestSummary
	planDigest    string
	requestDigest string
}

type Adapter struct {
	config   Config
	mu       sync.Mutex
	prepared map[union.ExecutionID]*preparedExecution
}

func New(config Config) (*Adapter, error) {
	config = config.clone()
	if err := config.normalize(); err != nil {
		return nil, err
	}
	return &Adapter{config: config, prepared: make(map[union.ExecutionID]*preparedExecution)}, nil
}

func (adapter *Adapter) Describe(_ context.Context) (execution.AdapterDescriptor, error) {
	if adapter == nil {
		return execution.AdapterDescriptor{}, ErrInvalidConfig
	}
	return execution.AdapterDescriptor{
		Identity: adapter.config.Identity, Origin: union.EventOriginHarness,
		ExecutionKinds: []union.ExecutionKind{union.ExecutionKindAgent},
	}, nil
}

// Preflight starts the exact pinned sidecar, completes the SDK initialize
// control request, consumes SystemMessage(init), and retains that same process
// for Open. No prompt is sent before the Actual Manifest is accepted.
func (adapter *Adapter) Preflight(ctx context.Context, invocation execution.Invocation) (execution.PreflightReport, error) {
	if adapter == nil || ctx == nil {
		return execution.PreflightReport{}, ErrInvalidConfig
	}
	if err := invocation.Validate(); err != nil {
		return execution.PreflightReport{}, err
	}
	if invocation.Plan.ExecutionKind != union.ExecutionKindAgent || invocation.Plan.Route != adapter.config.Route {
		return rejected("claude_route_mismatch"), nil
	}
	adapter.mu.Lock()
	_, exists := adapter.prepared[invocation.Request.ExecutionID]
	adapter.mu.Unlock()
	if exists {
		return execution.PreflightReport{}, ErrAlreadyPrepared
	}
	client, err := streamjson.Start(ctx, adapter.config.Process)
	if err != nil {
		return execution.PreflightReport{}, err
	}
	keep := false
	defer func() {
		if !keep {
			_ = client.Close()
		}
	}()
	var initialize map[string]any
	if err := json.Unmarshal(adapter.config.InitializeRequest, &initialize); err != nil {
		return execution.PreflightReport{}, err
	}
	if _, err := client.Call(ctx, initialize); err != nil {
		return execution.PreflightReport{}, fmt.Errorf("%w: initialize: %v", ErrProtocol, err)
	}
	message, err := client.Receive(ctx)
	if err != nil {
		return execution.PreflightReport{}, fmt.Errorf("%w: init event: %v", ErrProtocol, err)
	}
	init, err := decodeInit(message.Raw)
	if err != nil {
		return execution.PreflightReport{}, err
	}
	if err := validateInit(adapter.config.ExpectedInit, init); err != nil {
		return rejected("claude_manifest_drift"), nil
	}
	manifest, err := buildActualManifest(invocation.Plan.ExpectedManifest, init, client.Evidence())
	if err != nil {
		return execution.PreflightReport{}, err
	}
	planDigest, err := invocation.Plan.ComputeDigest()
	if err != nil {
		return execution.PreflightReport{}, err
	}
	requestDigest, err := invocation.Request.Digest()
	if err != nil {
		return execution.PreflightReport{}, err
	}
	adapter.mu.Lock()
	if _, duplicate := adapter.prepared[invocation.Request.ExecutionID]; duplicate {
		adapter.mu.Unlock()
		return execution.PreflightReport{}, ErrAlreadyPrepared
	}
	adapter.prepared[invocation.Request.ExecutionID] = &preparedExecution{
		client: client, init: init, manifest: manifest, planDigest: planDigest, requestDigest: requestDigest,
	}
	adapter.mu.Unlock()
	keep = true
	return execution.PreflightReport{Accepted: true, ActualManifest: manifest}, nil
}

func (adapter *Adapter) Open(ctx context.Context, invocation execution.Invocation) (execution.Session, error) {
	if adapter == nil || ctx == nil {
		return nil, ErrInvalidConfig
	}
	if err := invocation.Validate(); err != nil {
		return nil, err
	}
	adapter.mu.Lock()
	prepared := adapter.prepared[invocation.Request.ExecutionID]
	delete(adapter.prepared, invocation.Request.ExecutionID)
	adapter.mu.Unlock()
	if prepared == nil {
		return nil, ErrPreparedNotFound
	}
	planDigest, planErr := invocation.Plan.ComputeDigest()
	requestDigest, requestErr := invocation.Request.Digest()
	if planErr != nil || requestErr != nil || planDigest != prepared.planDigest || requestDigest != prepared.requestDigest {
		_ = prepared.client.Close()
		return nil, fmt.Errorf("%w: invocation changed after preflight", ErrRouteMismatch)
	}
	message := buildUserMessage(invocation.Request, prepared.init.SessionID)
	if err := prepared.client.Send(message); err != nil {
		_ = prepared.client.Close()
		return nil, err
	}
	return newSession(prepared.client, prepared.init, prepared.manifest, invocation.Plan, adapter.config.Clock, adapter.config.ApprovalTTL), nil
}

// ClosePrepared releases preflight processes when a caller elects not to Open.
func (adapter *Adapter) ClosePrepared(executionID union.ExecutionID) error {
	if adapter == nil {
		return nil
	}
	adapter.mu.Lock()
	prepared := adapter.prepared[executionID]
	delete(adapter.prepared, executionID)
	adapter.mu.Unlock()
	if prepared == nil {
		return nil
	}
	err := prepared.client.Close()
	if errors.Is(err, streamjson.ErrClosed) {
		return nil
	}
	return err
}

func buildUserMessage(request union.UnifiedExecutionRequest, sessionID string) map[string]any {
	blocks := make([]map[string]any, 0)
	for _, instruction := range request.Instructions {
		for _, part := range instruction.Content {
			if text := contentText(part); text != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": text})
			}
		}
	}
	for _, input := range request.Input {
		for _, part := range input.Content {
			if text := contentText(part); text != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": text})
			}
		}
	}
	if len(blocks) == 0 {
		blocks = append(blocks, map[string]any{"type": "text", "text": "Execute the prepared Praxis intent graph."})
	}
	return map[string]any{
		"type": "user", "session_id": sessionID, "parent_tool_use_id": nil,
		"message": map[string]any{"role": "user", "content": blocks},
	}
}

func contentText(part union.ContentPart) string {
	if part.Text != "" {
		return part.Text
	}
	if len(part.JSON) != 0 {
		return string(part.JSON)
	}
	if part.Reference != "" {
		return part.Reference
	}
	return ""
}

var _ execution.Adapter = (*Adapter)(nil)

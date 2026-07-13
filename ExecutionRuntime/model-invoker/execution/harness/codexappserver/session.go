package codexappserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

type attemptBinding struct {
	intentID  union.IntentID
	planID    union.MechanismPlanID
	attemptID union.MechanismAttemptID
	plan      union.MechanismPlan
}

type executionSession struct {
	client     *Client
	mapper     *Mapper
	invocation execution.Invocation
	bindings   []attemptBinding
	clock      func() time.Time

	readMu           sync.Mutex
	mu               sync.Mutex
	queue            []union.UnifiedExecutionEvent
	closed           bool
	terminal         bool
	cancelRequested  bool
	pendingApprovals map[union.ApprovalID]NativeEvent
	pendingTools     map[union.ActionID]NativeEvent
	sourceSequence   atomic.Uint64
	closeOnce        sync.Once
	closeErr         error
}

func newExecutionSession(client *Client, mapper *Mapper, invocation execution.Invocation, bindings []attemptBinding, clock func() time.Time) *executionSession {
	session := &executionSession{
		client: client, mapper: mapper, invocation: invocation, bindings: append([]attemptBinding(nil), bindings...), clock: clock,
		pendingApprovals: make(map[union.ApprovalID]NativeEvent), pendingTools: make(map[union.ActionID]NativeEvent),
	}
	for _, binding := range bindings {
		session.queue = append(session.queue, session.attemptEvent(binding, union.AttemptStatusRunning, union.SideEffectNone, "attempt_started"))
	}
	return session
}

func (session *executionSession) Receive(ctx context.Context) (union.UnifiedExecutionEvent, error) {
	if session == nil {
		return union.UnifiedExecutionEvent{}, execution.ErrSessionClosed
	}
	if ctx == nil {
		return union.UnifiedExecutionEvent{}, context.Canceled
	}
	session.readMu.Lock()
	defer session.readMu.Unlock()
	for {
		if event, ok, err := session.pop(); ok || err != nil {
			return event, err
		}
		native, err := session.client.Receive(ctx)
		if err != nil {
			return union.UnifiedExecutionEvent{}, err
		}
		mapped, err := session.mapper.Map(native)
		if err != nil {
			return union.UnifiedExecutionEvent{}, err
		}
		switch native.Kind {
		case NativeApprovalRequest:
			if mapped.Control == nil {
				return union.UnifiedExecutionEvent{}, fmt.Errorf("%w: approval mapping lacks control payload", ErrProtocol)
			}
			session.mu.Lock()
			session.pendingApprovals[mapped.Control.ApprovalID] = native
			session.mu.Unlock()
		case NativeDynamicToolRequest:
			actionID := mapped.Header.ActionID
			if actionID == "" {
				return union.UnifiedExecutionEvent{}, fmt.Errorf("%w: dynamic tool mapping lacks action identity", ErrProtocol)
			}
			session.mu.Lock()
			session.pendingTools[actionID] = native
			session.mu.Unlock()
		case NativeTerminalCandidate:
			if native.Terminal == nil {
				return union.UnifiedExecutionEvent{}, fmt.Errorf("%w: terminal candidate is missing", ErrProtocol)
			}
			session.mu.Lock()
			if session.cancelRequested && native.Terminal.Status == union.ExecutionStatusCancelled {
				session.queue = append(session.queue, session.controlEvent(execution.ControlCancellationQuiesced))
			}
			for _, binding := range session.bindings {
				session.queue = append(session.queue, session.attemptEvent(binding, terminalAttemptStatus(native.Terminal.Status), native.Terminal.SideEffectState, "attempt_completed"))
			}
			mapped.Header.SourceSequence = session.nextSourceSequence()
			session.queue = append(session.queue, mapped)
			session.terminal = true
			session.mu.Unlock()
			continue
		}
		mapped.Header.SourceSequence = session.nextSourceSequence()
		return mapped, nil
	}
}

func (session *executionSession) Command(ctx context.Context, command union.ExecutionCommand) error {
	if session == nil {
		return execution.ErrSessionClosed
	}
	if ctx == nil {
		return context.Canceled
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if command.ExecutionID != session.invocation.Request.ExecutionID {
		return fmt.Errorf("%w: execution identity differs", ErrUnsupportedCommand)
	}
	switch command.Kind {
	case union.CommandCancelExecution:
		// Hold the queue lock across interrupt dispatch so a concurrently
		// delivered turn/completed cannot overtake acknowledgement evidence.
		session.mu.Lock()
		session.cancelRequested = true
		if err := session.client.Interrupt(ctx); err != nil {
			session.cancelRequested = false
			session.mu.Unlock()
			return err
		}
		session.queue = append(session.queue, session.controlEvent(execution.ControlCancelAcknowledged))
		session.mu.Unlock()
		return nil
	case union.CommandInterrupt:
		return session.client.Interrupt(ctx)
	case union.CommandApproveAction, union.CommandDenyAction, union.CommandProvideInput:
		return session.resolveApproval(command)
	case union.CommandProvideToolResult:
		return session.resolveDynamicTool(command)
	default:
		return ErrUnsupportedCommand
	}
}

func (session *executionSession) resolveApproval(command union.ExecutionCommand) error {
	session.mu.Lock()
	native, exists := session.pendingApprovals[command.ApprovalID]
	session.mu.Unlock()
	if !exists || command.ActionID == "" || command.MechanismAttemptID != session.bindings[0].attemptID {
		return ErrReverseRequest
	}
	result, err := codexApprovalResult(native.Method, command)
	if err != nil {
		return err
	}
	if err := session.client.RespondApproval(native, result); err != nil {
		return err
	}
	session.mu.Lock()
	delete(session.pendingApprovals, command.ApprovalID)
	session.mu.Unlock()
	return nil
}

func (session *executionSession) resolveDynamicTool(command union.ExecutionCommand) error {
	session.mu.Lock()
	native, exists := session.pendingTools[command.ActionID]
	session.mu.Unlock()
	if !exists || command.MechanismAttemptID != session.bindings[0].attemptID {
		return ErrReverseRequest
	}
	result, err := codexDynamicToolResult(command.Payload)
	if err != nil {
		return err
	}
	if err := session.client.RespondDynamicTool(native, result); err != nil {
		return err
	}
	session.mu.Lock()
	delete(session.pendingTools, command.ActionID)
	session.mu.Unlock()
	return nil
}

type nativeCommandPayload struct {
	NativeResult json.RawMessage `json:"native_result,omitempty"`
	Output       string          `json:"output,omitempty"`
	IsError      bool            `json:"is_error,omitempty"`
}

func codexApprovalResult(method string, command union.ExecutionCommand) (json.RawMessage, error) {
	var payload nativeCommandPayload
	if len(command.Payload) != 0 && json.Unmarshal(command.Payload, &payload) != nil {
		return nil, fmt.Errorf("%w: command payload is invalid", ErrUnsupportedCommand)
	}
	if len(payload.NativeResult) != 0 {
		if !validJSONObject(payload.NativeResult) {
			return nil, fmt.Errorf("%w: native_result must be an object", ErrUnsupportedCommand)
		}
		return cloneRaw(payload.NativeResult), nil
	}
	if command.Kind == union.CommandProvideInput || method == "item/tool/requestUserInput" {
		return nil, fmt.Errorf("%w: user input requires payload.native_result", ErrUnsupportedCommand)
	}
	if method == "item/permissions/requestApproval" {
		if command.Kind == union.CommandDenyAction {
			return json.RawMessage(`{"permissions":{}}`), nil
		}
		return nil, fmt.Errorf("%w: permission approval requires an explicit granted subset in payload.native_result", ErrUnsupportedCommand)
	}
	decision := "accept"
	if command.Kind == union.CommandDenyAction {
		decision = "decline"
	}
	return json.Marshal(map[string]string{"decision": decision})
}

func codexDynamicToolResult(raw json.RawMessage) (json.RawMessage, error) {
	var payload nativeCommandPayload
	if len(raw) == 0 || json.Unmarshal(raw, &payload) != nil {
		return nil, fmt.Errorf("%w: tool result payload is invalid", ErrUnsupportedCommand)
	}
	if len(payload.NativeResult) != 0 {
		if !validJSONObject(payload.NativeResult) {
			return nil, fmt.Errorf("%w: native_result must be an object", ErrUnsupportedCommand)
		}
		return cloneRaw(payload.NativeResult), nil
	}
	if strings.TrimSpace(payload.Output) == "" {
		return nil, fmt.Errorf("%w: tool output is required", ErrUnsupportedCommand)
	}
	return json.Marshal(map[string]any{
		"contentItems": []any{map[string]string{"type": "inputText", "text": payload.Output}}, "success": !payload.IsError,
	})
}

func (session *executionSession) pop() (union.UnifiedExecutionEvent, bool, error) {
	session.mu.Lock()
	defer session.mu.Unlock()
	if len(session.queue) != 0 {
		event := session.queue[0]
		session.queue[0] = union.UnifiedExecutionEvent{}
		session.queue = session.queue[1:]
		return event, true, nil
	}
	if session.closed {
		return union.UnifiedExecutionEvent{}, false, execution.ErrSessionClosed
	}
	if session.terminal {
		return union.UnifiedExecutionEvent{}, false, io.EOF
	}
	return union.UnifiedExecutionEvent{}, false, nil
}

func (session *executionSession) attemptEvent(binding attemptBinding, status union.AttemptStatus, sideEffects union.SideEffectState, kind string) union.UnifiedExecutionEvent {
	sequence := session.nextSourceSequence()
	now := session.clock().UTC()
	attempt := union.MechanismAttempt{
		ID: binding.attemptID, MechanismPlanID: binding.planID, Authoritative: true,
		ActualKind: binding.plan.Kind, ActualOrigin: union.CapabilityOriginHarnessHosted, ActualOwner: union.ExecutionOwnerHarness,
		NativeToolIdentity: &union.NativeIdentity{Namespace: "openai.codex.app-server", Kind: "turn", Value: "turn/start"},
		Status:             status, SideEffectState: sideEffects,
	}
	if status == union.AttemptStatusRunning {
		attempt.StartedAt = now
	} else {
		attempt.EndedAt = now
	}
	return union.UnifiedExecutionEvent{
		Header: union.EventHeader{
			EventID: union.EventID(fmt.Sprintf("%s:codex:attempt:%d", session.invocation.Request.ExecutionID, sequence)), SemanticVersion: union.SemanticVersionV1,
			ExecutionID: session.invocation.Request.ExecutionID, IntentID: binding.intentID, MechanismPlanID: binding.planID, MechanismAttemptID: binding.attemptID,
			Sequence: sequence, SourceSequence: sequence, Timestamp: now, IngestedAt: now, Origin: union.EventOriginHarness,
			Family: union.EventFamilyMechanism, Visibility: union.VisibilityAuditOnly, SecurityClassification: union.SecurityInternal,
			ExecutionKind: union.ExecutionKindAgent, Profile: session.invocation.Plan.Profile, Route: session.invocation.Plan.Route,
		},
		Mechanism: &union.MechanismEvent{Kind: kind, Attempt: &attempt},
	}
}

func (session *executionSession) controlEvent(kind string) union.UnifiedExecutionEvent {
	sequence := session.nextSourceSequence()
	now := session.clock().UTC()
	primary := session.bindings[0]
	return union.UnifiedExecutionEvent{
		Header: union.EventHeader{
			EventID: union.EventID(fmt.Sprintf("%s:codex:control:%d", session.invocation.Request.ExecutionID, sequence)), SemanticVersion: union.SemanticVersionV1,
			ExecutionID: session.invocation.Request.ExecutionID, IntentID: primary.intentID, MechanismPlanID: primary.planID, MechanismAttemptID: primary.attemptID,
			Sequence: sequence, SourceSequence: sequence, Timestamp: now, IngestedAt: now, Origin: union.EventOriginHarness,
			Family: union.EventFamilyControl, Visibility: union.VisibilityAuditOnly, SecurityClassification: union.SecurityInternal,
			ExecutionKind: union.ExecutionKindAgent, Profile: session.invocation.Plan.Profile, Route: session.invocation.Plan.Route,
		},
		Control: &union.ControlEvent{Kind: kind},
	}
}

func (session *executionSession) nextSourceSequence() uint64 { return session.sourceSequence.Add(1) }

func (session *executionSession) Close() error {
	if session == nil {
		return nil
	}
	session.closeOnce.Do(func() {
		session.mu.Lock()
		session.closed = true
		session.mu.Unlock()
		session.closeErr = session.client.Close()
	})
	return session.closeErr
}

func selectAttemptBindings(invocation execution.Invocation) ([]attemptBinding, error) {
	selected := make(map[union.IntentID]union.MechanismPlan)
	for _, plan := range invocation.Plan.Mechanisms {
		current, exists := selected[plan.IntentID]
		if !exists || plan.PreferredRank < current.PreferredRank || (plan.PreferredRank == current.PreferredRank && plan.ID < current.ID) {
			selected[plan.IntentID] = plan
		}
	}
	bindings := make([]attemptBinding, 0, len(invocation.Plan.IntentGraph.Nodes))
	for _, intent := range invocation.Plan.IntentGraph.Nodes {
		plan, exists := selected[intent.ID]
		if !exists {
			return nil, fmt.Errorf("%w: intent %s has no selected mechanism", ErrMapping, intent.ID)
		}
		bindings = append(bindings, attemptBinding{
			intentID: intent.ID, planID: plan.ID,
			attemptID: union.MechanismAttemptID(fmt.Sprintf("%s:%s:attempt:1", invocation.Request.ExecutionID, plan.ID)), plan: plan,
		})
	}
	if len(bindings) == 0 {
		return nil, fmt.Errorf("%w: no mechanism attempts were selected", ErrMapping)
	}
	return bindings, nil
}

func terminalAttemptStatus(status union.ExecutionStatus) union.AttemptStatus {
	switch status {
	case union.ExecutionStatusSucceeded:
		return union.AttemptStatusCompleted
	case union.ExecutionStatusCancelled:
		return union.AttemptStatusCancelled
	case union.ExecutionStatusFailed:
		return union.AttemptStatusFailed
	default:
		return union.AttemptStatusIndeterminate
	}
}

var _ execution.Session = (*executionSession)(nil)

package qwen

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/streamjson"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

type pendingPermission struct {
	requestID   string
	toolUseID   string
	toolName    string
	input       json.RawMessage
	inputDigest string
	attemptID   union.MechanismAttemptID
}

type partialTool struct {
	id   string
	name string
}

type toolAttempt struct {
	plan    union.MechanismPlan
	attempt union.MechanismAttempt
}

type session struct {
	client      *streamjson.Client
	init        InitMessage
	manifest    union.ContextManifestSummary
	plan        union.PreparedExecutionPlan
	clock       func() time.Time
	approvalTTL time.Duration

	readMu         sync.Mutex
	mu             sync.Mutex
	queue          []union.UnifiedExecutionEvent
	permissions    map[string]pendingPermission
	pendingTools   map[string]string
	partialTools   map[int]partialTool
	attempts       map[string]toolAttempt
	selectedPlans  map[union.MechanismPlanID]bool
	itemStatuses   map[string]union.ItemStatus
	cancelPending  *streamjson.PendingCall
	cancelAcked    bool
	resultSeen     bool
	terminal       bool
	eofSynthesized bool
	toolObserved   bool
	sequence       atomic.Uint64

	closeOnce sync.Once
	closeErr  error
}

func newSession(client *streamjson.Client, init InitMessage, manifest union.ContextManifestSummary, plan union.PreparedExecutionPlan, clock func() time.Time, approvalTTL time.Duration) *session {
	session := &session{
		client: client, init: init, manifest: manifest, plan: plan, clock: clock, approvalTTL: approvalTTL,
		permissions: make(map[string]pendingPermission), pendingTools: make(map[string]string),
		partialTools: make(map[int]partialTool), attempts: make(map[string]toolAttempt),
		selectedPlans: make(map[union.MechanismPlanID]bool), itemStatuses: make(map[string]union.ItemStatus),
	}
	session.queue = append(session.queue, manifestDraft(session.nextSequence(), init, manifest))
	return session
}

func (session *session) nextSequence() uint64 { return session.sequence.Add(1) }

func (session *session) Receive(ctx context.Context) (union.UnifiedExecutionEvent, error) {
	if session == nil || ctx == nil {
		return union.UnifiedExecutionEvent{}, execution.ErrSessionClosed
	}
	session.readMu.Lock()
	defer session.readMu.Unlock()
	for {
		if event, ok := session.pop(); ok {
			return event, nil
		}
		session.mu.Lock()
		terminal := session.terminal
		session.mu.Unlock()
		if terminal {
			return union.UnifiedExecutionEvent{}, io.EOF
		}
		message, err := session.client.Receive(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				cancellation, cancellationErr := session.finishCancellation(ctx)
				if cancellationErr != nil {
					return union.UnifiedExecutionEvent{}, cancellationErr
				}
				session.mu.Lock()
				session.queue = append(session.queue, cancellation...)
				session.mu.Unlock()
				session.synthesizeMissingResult()
				continue
			}
			return union.UnifiedExecutionEvent{}, err
		}
		var prefix []union.UnifiedExecutionEvent
		if message.Type == "result" {
			prefix, err = session.finishCancellation(ctx)
			if err != nil {
				return union.UnifiedExecutionEvent{}, err
			}
		}
		events, err := session.decode(message)
		if err != nil {
			return union.UnifiedExecutionEvent{}, err
		}
		session.mu.Lock()
		session.queue = append(session.queue, prefix...)
		session.queue = append(session.queue, events...)
		session.mu.Unlock()
	}
}

func (session *session) Command(ctx context.Context, command union.ExecutionCommand) error {
	if session == nil || ctx == nil {
		return execution.ErrSessionClosed
	}
	switch command.Kind {
	case union.CommandApproveAction, union.CommandDenyAction:
		return session.resolvePermission(command)
	case union.CommandCancelExecution:
		session.mu.Lock()
		duplicate := session.cancelPending != nil || session.cancelAcked
		if duplicate {
			session.mu.Unlock()
			return fmt.Errorf("%w: duplicate cancellation", ErrProtocol)
		}
		// Keep the Session lock from the duplicate check through publication of
		// cancelPending. A very fast Harness may emit control_response + result
		// before BeginCall returns; Receive must not observe that result while
		// cancelPending is still nil or it would commit a terminal without the
		// required acknowledgement/quiescence evidence.
		pending, err := session.client.BeginCall(map[string]any{"subtype": "interrupt"})
		if err != nil {
			session.mu.Unlock()
			return fmt.Errorf("%w: interrupt dispatch: %v", ErrProtocol, err)
		}
		session.cancelPending = pending
		session.mu.Unlock()
		return nil
	case union.CommandInterrupt:
		_, err := session.client.Call(ctx, map[string]any{"subtype": "interrupt"})
		if err != nil {
			return fmt.Errorf("%w: interrupt: %v", ErrProtocol, err)
		}
		return nil
	default:
		return ErrUnsupportedCommand
	}
}

func (session *session) Close() error {
	if session == nil {
		return nil
	}
	session.closeOnce.Do(func() { session.closeErr = session.client.Close() })
	return session.closeErr
}

func (session *session) pop() (union.UnifiedExecutionEvent, bool) {
	session.mu.Lock()
	defer session.mu.Unlock()
	if len(session.queue) == 0 {
		return union.UnifiedExecutionEvent{}, false
	}
	event := session.queue[0]
	session.queue[0] = union.UnifiedExecutionEvent{}
	session.queue = session.queue[1:]
	return event, true
}

func (session *session) finishCancellation(ctx context.Context) ([]union.UnifiedExecutionEvent, error) {
	session.mu.Lock()
	pending := session.cancelPending
	acked := session.cancelAcked
	session.mu.Unlock()
	if pending == nil || acked {
		return nil, nil
	}
	response, err := pending.Await(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: interrupt acknowledgement: %v", ErrProtocol, err)
	}
	events := []union.UnifiedExecutionEvent{
		cancellationDraft(session.nextSequence(), session.init.SessionID, execution.ControlCancelAcknowledged, response),
	}
	_ = session.client.CloseInput()
	result, waitErr := session.client.Wait()
	if waitErr == nil && result.Quiesced {
		payload, _ := json.Marshal(map[string]any{
			"executable": result.ActualExecutablePath, "digest": result.ActualExecutableDigest,
			"exit_code": result.ExitCode, "quiesced": result.Quiesced,
		})
		events = append(events, cancellationDraft(session.nextSequence(), session.init.SessionID, execution.ControlCancellationQuiesced, payload))
	} else {
		payload, _ := json.Marshal(map[string]any{"quiesced": result.Quiesced, "error": fmt.Sprint(waitErr)})
		events = append(events, diagnosticDraft(session.nextSequence(), session.init.SessionID, "process/wait", "cancellation_not_quiesced", "process_not_quiesced", "", payload))
	}
	session.mu.Lock()
	session.cancelPending = nil
	session.cancelAcked = true
	session.mu.Unlock()
	return events, nil
}

func (session *session) synthesizeMissingResult() {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.resultSeen || session.eofSynthesized {
		session.terminal = true
		return
	}
	session.eofSynthesized = true
	session.terminal = true
	payload, _ := json.Marshal(map[string]string{"reason": "eof_without_sdk_result"})
	session.queue = append(session.queue,
		diagnosticDraft(session.nextSequence(), session.init.SessionID, "eof", "protocol_violation", "missing_result", ErrMissingResult.Error(), payload),
		terminalDraft(session.nextSequence(), session.init.SessionID, execution.RouteTerminalCandidate{
			Status: union.ExecutionStatusIndeterminate, StopReason: "eof_without_sdk_result",
			PendingBackgroundWork: len(session.pendingTools), SideEffectState: union.SideEffectUnknown,
		}),
	)
}

func (session *session) decode(message streamjson.Message) ([]union.UnifiedExecutionEvent, error) {
	switch message.Type {
	case "stream_event":
		return session.decodeStreamEvent(message.Raw)
	case "assistant":
		return session.decodeAssistant(message.Raw)
	case "user":
		return session.decodeUser(message.Raw)
	case "system":
		return session.decodeSystem(message.Raw)
	case "control_request":
		return session.decodeControlRequest(message.Raw)
	case "control_cancel_request":
		return []union.UnifiedExecutionEvent{diagnosticDraft(session.nextSequence(), session.init.SessionID, message.Type, "native_control_cancel", message.RequestID, "", unknownPayload(message.Raw, message.Type, message.Subtype))}, nil
	case "result":
		return session.decodeResult(message.Raw)
	case "error":
		return []union.UnifiedExecutionEvent{diagnosticDraft(session.nextSequence(), session.init.SessionID, message.Type, "native_error", "sdk_error", objectString(message.Raw, "error"), unknownPayload(message.Raw, message.Type, message.Subtype))}, nil
	default:
		return []union.UnifiedExecutionEvent{diagnosticDraft(session.nextSequence(), session.init.SessionID, message.Type, "unknown_native_event", "event_unrecognized", "", unknownPayload(message.Raw, message.Type, message.Subtype))}, nil
	}
}

func (session *session) decodeSystem(raw json.RawMessage) ([]union.UnifiedExecutionEvent, error) {
	subtype := objectString(raw, "subtype")
	sessionID := objectString(raw, "session_id")
	if sessionID == "" {
		sessionID = session.init.SessionID
	}
	if subtype == "init" {
		return []union.UnifiedExecutionEvent{diagnosticDraft(session.nextSequence(), sessionID, "system/init", "protocol_violation", "duplicate_init", "duplicate SDKSystemMessage after preflight", unknownPayload(raw, "system", subtype))}, nil
	}
	itemID := objectString(raw, "uuid")
	if itemID == "" {
		itemID = "qwen-system-" + subtype
	}
	if subtype == "compact_boundary" || subtype == "status" {
		return []union.UnifiedExecutionEvent{itemDraft(session.nextSequence(), sessionID, "system/"+subtype, itemID, subtype, union.ItemStatusCompleted, union.SideEffectNone, raw)}, nil
	}
	return []union.UnifiedExecutionEvent{diagnosticDraft(session.nextSequence(), sessionID, "system/"+subtype, "native_system", subtype, "", unknownPayload(raw, "system", subtype))}, nil
}

func (session *session) decodeStreamEvent(raw json.RawMessage) ([]union.UnifiedExecutionEvent, error) {
	sessionID := objectString(raw, "session_id")
	if sessionID == "" {
		sessionID = session.init.SessionID
	}
	eventRaw := objectRaw(raw, "event")
	eventType := objectString(eventRaw, "type")
	switch eventType {
	case "message_start":
		return []union.UnifiedExecutionEvent{modelDraft(session.nextSequence(), sessionID, "stream_event/message_start", "model_step_started", nil, "provider_exposed_output", "", eventRaw)}, nil
	case "message_stop":
		return []union.UnifiedExecutionEvent{modelDraft(session.nextSequence(), sessionID, "stream_event/message_stop", "model_step_completed", nil, "provider_exposed_output", "", eventRaw)}, nil
	case "content_block_start":
		return session.decodeBlock(sessionID, "stream_event/content_block_start", objectRaw(eventRaw, "content_block"), objectInt(eventRaw, "index"), true)
	case "content_block_delta":
		return session.decodeDelta(sessionID, eventRaw)
	case "content_block_stop":
		index := objectInt(eventRaw, "index")
		session.mu.Lock()
		tool := session.partialTools[index]
		delete(session.partialTools, index)
		session.mu.Unlock()
		payload, _ := json.Marshal(map[string]any{"index": index, "tool_use_id": tool.id})
		return []union.UnifiedExecutionEvent{modelDraft(session.nextSequence(), sessionID, "stream_event/content_block_stop", "content_block_completed", nil, "provider_exposed_output", union.ActionID(tool.id), payload)}, nil
	default:
		return []union.UnifiedExecutionEvent{diagnosticDraft(session.nextSequence(), sessionID, "stream_event/"+eventType, "unknown_native_event", "stream_event_unrecognized", "", unknownPayload(eventRaw, "stream_event", eventType))}, nil
	}
}

func (session *session) decodeDelta(sessionID string, eventRaw json.RawMessage) ([]union.UnifiedExecutionEvent, error) {
	index := objectInt(eventRaw, "index")
	delta := objectRaw(eventRaw, "delta")
	switch objectString(delta, "type") {
	case "text_delta":
		return []union.UnifiedExecutionEvent{modelDraft(session.nextSequence(), sessionID, "stream_event/text_delta", "content_delta", textPart(objectString(delta, "text")), "provider_exposed_output", "", delta)}, nil
	case "thinking_delta":
		return []union.UnifiedExecutionEvent{modelDraft(session.nextSequence(), sessionID, "stream_event/thinking_delta", "reasoning_delta", textPart(objectString(delta, "thinking")), "provider_exposed_reasoning", "", delta)}, nil
	case "input_json_delta":
		session.mu.Lock()
		tool := session.partialTools[index]
		state := session.attempts[tool.id]
		session.mu.Unlock()
		payload, _ := json.Marshal(map[string]any{"partial_json": objectString(delta, "partial_json"), "index": index, "tool_use_id": tool.id, "name": tool.name})
		event := modelDraft(session.nextSequence(), sessionID, "stream_event/input_json_delta", "tool_input_delta", nil, "provider_exposed_output", union.ActionID(tool.id), payload)
		bindAttempt(&event, state)
		return []union.UnifiedExecutionEvent{event}, nil
	default:
		return []union.UnifiedExecutionEvent{diagnosticDraft(session.nextSequence(), sessionID, "stream_event/content_block_delta", "unknown_native_event", "delta_unrecognized", "", unknownPayload(delta, "content_block_delta", objectString(delta, "type")))}, nil
	}
}

func (session *session) decodeAssistant(raw json.RawMessage) ([]union.UnifiedExecutionEvent, error) {
	sessionID := objectString(raw, "session_id")
	if sessionID == "" {
		sessionID = session.init.SessionID
	}
	messageRaw := objectRaw(raw, "message")
	var blocks []json.RawMessage
	if err := json.Unmarshal(objectRaw(messageRaw, "content"), &blocks); err != nil {
		return nil, fmt.Errorf("%w: assistant content must be an array", ErrProtocol)
	}
	events := make([]union.UnifiedExecutionEvent, 0, len(blocks)+1)
	for index, block := range blocks {
		mapped, err := session.decodeBlock(sessionID, "assistant", block, index, false)
		if err != nil {
			return nil, err
		}
		events = append(events, mapped...)
	}
	return events, nil
}

func (session *session) decodeUser(raw json.RawMessage) ([]union.UnifiedExecutionEvent, error) {
	sessionID := objectString(raw, "session_id")
	if sessionID == "" {
		sessionID = session.init.SessionID
	}
	messageRaw := objectRaw(raw, "message")
	var blocks []json.RawMessage
	if err := json.Unmarshal(objectRaw(messageRaw, "content"), &blocks); err != nil {
		return []union.UnifiedExecutionEvent{diagnosticDraft(session.nextSequence(), sessionID, "user", "native_user_message", "non_block_content", "", unknownPayload(raw, "user", ""))}, nil
	}
	events := make([]union.UnifiedExecutionEvent, 0, len(blocks))
	for index, block := range blocks {
		if objectString(block, "type") != "tool_result" {
			continue
		}
		mapped, err := session.decodeBlock(sessionID, "user", block, index, false)
		if err != nil {
			return nil, err
		}
		events = append(events, mapped...)
	}
	if len(events) == 0 {
		events = append(events, diagnosticDraft(session.nextSequence(), sessionID, "user", "native_user_message", "retained", "", unknownPayload(raw, "user", "")))
	}
	return events, nil
}

func (session *session) selectPlan(toolName string) (union.MechanismPlan, error) {
	needle := strings.ToLower(toolName)
	aliases := []string{needle}
	switch needle {
	case "run_shell_command":
		aliases = append(aliases, "shell", "execute", "code", "bash")
	case "edit", "notebook_edit":
		aliases = append(aliases, "modify", "rewrite", "create", "patch")
	case "read_file":
		aliases = append(aliases, "read", "inspect")
	}
	for _, plan := range session.plan.Mechanisms {
		kind := strings.ToLower(plan.Kind)
		for _, alias := range aliases {
			if strings.Contains(kind, alias) {
				return plan, nil
			}
		}
	}
	if len(session.plan.Mechanisms) == 0 {
		return union.MechanismPlan{}, fmt.Errorf("%w: prepared plan has no mechanism", ErrProtocol)
	}
	return session.plan.Mechanisms[0], nil
}

// structuredOutputEvents binds an SDKResult carrying output to the prepared
// ProduceStructured mechanism when no native tool call occurred. The Adapter
// records mechanism evidence only; Praxis still owns schema verification and
// final Effect authority.
func (session *session) structuredOutputEvents(status union.ExecutionStatus, hasOutput bool) []union.UnifiedExecutionEvent {
	if !hasOutput {
		return nil
	}
	var intentID union.IntentID
	for _, intent := range session.plan.IntentGraph.Nodes {
		if intent.Kind == union.IntentProduceStructured {
			intentID = intent.ID
			break
		}
	}
	if intentID == "" {
		return nil
	}
	var selected union.MechanismPlan
	for _, candidate := range session.plan.Mechanisms {
		if candidate.IntentID != intentID {
			continue
		}
		if selected.ID == "" || candidate.PreferredRank < selected.PreferredRank ||
			(candidate.PreferredRank == selected.PreferredRank && candidate.ID < selected.ID) {
			selected = candidate
		}
	}
	if selected.ID == "" {
		return nil
	}
	const outputKey = "__praxis_structured_output__"
	session.mu.Lock()
	if _, exists := session.attempts[outputKey]; exists {
		session.mu.Unlock()
		return nil
	}
	for _, existing := range session.attempts {
		if existing.plan.ID == selected.ID {
			session.mu.Unlock()
			return nil
		}
	}
	selectPlan := !session.selectedPlans[selected.ID]
	session.selectedPlans[selected.ID] = true
	started := session.clock().UTC()
	attempt := union.MechanismAttempt{
		ID: union.MechanismAttemptID("qwen-output-attempt-" + string(session.plan.ExecutionID) + "-" + string(selected.ID)), MechanismPlanID: selected.ID,
		Authoritative: true, ActualKind: selected.Kind, ActualOrigin: union.CapabilityOriginHarnessHosted,
		ActualOwner:        union.ExecutionOwnerHarness,
		NativeToolIdentity: &union.NativeIdentity{Namespace: "alibaba.qwen-code-sdk", Kind: "output", Value: "SDKResult"},
		StartedAt:          started, Status: union.AttemptStatusRunning, SideEffectState: union.SideEffectNone,
	}
	session.attempts[outputKey] = toolAttempt{plan: selected, attempt: attempt}
	session.mu.Unlock()

	events := make([]union.UnifiedExecutionEvent, 0, 3)
	if selectPlan {
		events = append(events, selectedPlanDraft(session.nextSequence(), session.init.SessionID, selected))
	}
	events = append(events, attemptDraft(session.nextSequence(), session.init.SessionID, "output_attempt_started", selected, attempt))
	completed := attempt
	completed.EndedAt = session.clock().UTC()
	switch status {
	case union.ExecutionStatusSucceeded:
		completed.Status = union.AttemptStatusCompleted
	case union.ExecutionStatusFailed:
		completed.Status = union.AttemptStatusFailed
	case union.ExecutionStatusCancelled:
		completed.Status = union.AttemptStatusCancelled
	default:
		completed.Status = union.AttemptStatusIndeterminate
	}
	session.mu.Lock()
	session.attempts[outputKey] = toolAttempt{plan: selected, attempt: completed}
	session.mu.Unlock()
	events = append(events, attemptDraft(session.nextSequence(), session.init.SessionID, "output_attempt_completed", selected, completed))
	return events
}

func (session *session) ensureAttempt(toolUseID, toolName string, input json.RawMessage, itemStatus union.ItemStatus) (toolAttempt, []union.UnifiedExecutionEvent, error) {
	session.mu.Lock()
	if existing, ok := session.attempts[toolUseID]; ok {
		current := session.itemStatuses[toolUseID]
		if current == union.ItemStatusPending && itemStatus == union.ItemStatusInProgress {
			session.itemStatuses[toolUseID] = itemStatus
			session.mu.Unlock()
			payload := actionPayload(toolUseID, toolName, input)
			return existing, []union.UnifiedExecutionEvent{toolItemDraft(
				session.nextSequence(), session.init.SessionID, "tool_action/in_progress", toolUseID,
				existing.plan, existing.attempt, itemStatus, payload,
			)}, nil
		}
		session.mu.Unlock()
		return existing, nil, nil
	}
	session.mu.Unlock()
	plan, err := session.selectPlan(toolName)
	if err != nil {
		return toolAttempt{}, nil, err
	}
	if len(input) == 0 {
		input = json.RawMessage(`{}`)
	}
	attempt := union.MechanismAttempt{
		ID: union.MechanismAttemptID("qwen-attempt-" + toolUseID), MechanismPlanID: plan.ID,
		Authoritative: true, ActualKind: toolName, ActualOrigin: union.CapabilityOriginHarnessHosted,
		ActualOwner:        union.ExecutionOwnerHarness,
		NativeToolIdentity: &union.NativeIdentity{Namespace: "alibaba.qwen-code-sdk", Kind: "tool", Value: toolName},
		StartedAt:          session.clock().UTC(), Status: union.AttemptStatusRunning,
		SanitizedInput: append(json.RawMessage(nil), input...), SideEffectState: toolSideEffects(toolName),
	}
	state := toolAttempt{plan: plan, attempt: attempt}
	session.mu.Lock()
	if existing, duplicate := session.attempts[toolUseID]; duplicate {
		session.mu.Unlock()
		return existing, nil, nil
	}
	selectPlan := !session.selectedPlans[plan.ID]
	session.selectedPlans[plan.ID] = true
	session.attempts[toolUseID] = state
	session.itemStatuses[toolUseID] = itemStatus
	session.pendingTools[toolUseID] = toolName
	session.toolObserved = true
	session.mu.Unlock()
	events := make([]union.UnifiedExecutionEvent, 0, 2)
	if selectPlan {
		events = append(events, selectedPlanDraft(session.nextSequence(), session.init.SessionID, plan))
	}
	events = append(events, attemptDraft(session.nextSequence(), session.init.SessionID, "attempt_started", plan, attempt))
	events = append(events, toolItemDraft(
		session.nextSequence(), session.init.SessionID, "tool_action/started", toolUseID,
		plan, attempt, itemStatus, actionPayload(toolUseID, toolName, input),
	))
	return state, events, nil
}

func (session *session) completeAttempt(toolUseID string, failed bool, payload json.RawMessage) ([]union.UnifiedExecutionEvent, toolAttempt, error) {
	session.mu.Lock()
	state, exists := session.attempts[toolUseID]
	if !exists {
		session.mu.Unlock()
		return nil, toolAttempt{}, fmt.Errorf("%w: tool_result %s has no observed tool_use", ErrProtocol, toolUseID)
	}
	state.attempt.EndedAt = session.clock().UTC()
	state.attempt.Status = union.AttemptStatusCompleted
	if failed {
		state.attempt.Status = union.AttemptStatusFailed
	}
	itemStatus := union.ItemStatusCompleted
	if failed {
		itemStatus = union.ItemStatusFailed
	}
	session.attempts[toolUseID] = state
	session.itemStatuses[toolUseID] = itemStatus
	delete(session.pendingTools, toolUseID)
	session.mu.Unlock()
	events := []union.UnifiedExecutionEvent{
		attemptDraft(session.nextSequence(), session.init.SessionID, "attempt_completed", state.plan, state.attempt),
		toolItemDraft(session.nextSequence(), session.init.SessionID, "tool_action/completed", toolUseID, state.plan, state.attempt, itemStatus, payload),
	}
	return events, state, nil
}

func toolSideEffects(toolName string) union.SideEffectState {
	switch strings.ToLower(toolName) {
	case "read_file", "glob", "grep", "list_directory", "web_search":
		return union.SideEffectNone
	default:
		return union.SideEffectPossible
	}
}

func bindAttempt(event *union.UnifiedExecutionEvent, state toolAttempt) {
	if event == nil || state.attempt.ID == "" {
		return
	}
	event.Header.IntentID = state.plan.IntentID
	event.Header.MechanismPlanID = state.plan.ID
	event.Header.MechanismAttemptID = state.attempt.ID
}

func (session *session) decodeBlock(sessionID, nativeType string, block json.RawMessage, index int, partial bool) ([]union.UnifiedExecutionEvent, error) {
	switch blockType := objectString(block, "type"); blockType {
	case "text":
		kind := "content_completed"
		if partial {
			kind = "content_started"
		}
		return []union.UnifiedExecutionEvent{modelDraft(session.nextSequence(), sessionID, nativeType+"/text", kind, textPart(objectString(block, "text")), "provider_exposed_output", "", block)}, nil
	case "thinking":
		kind := "reasoning_completed"
		if partial {
			kind = "reasoning_started"
		}
		return []union.UnifiedExecutionEvent{modelDraft(session.nextSequence(), sessionID, nativeType+"/thinking", kind, textPart(objectString(block, "thinking")), "provider_exposed_reasoning", "", block)}, nil
	case "tool_use":
		id, err := requireString(block, "id", "tool_use")
		if err != nil {
			return nil, err
		}
		name, err := requireString(block, "name", "tool_use")
		if err != nil {
			return nil, err
		}
		input := objectRaw(block, "input")
		state, attemptEvents, err := session.ensureAttempt(id, name, input, union.ItemStatusInProgress)
		if err != nil {
			return nil, err
		}
		session.mu.Lock()
		session.partialTools[index] = partialTool{id: id, name: name}
		session.mu.Unlock()
		kind := "model_tool_call"
		if partial {
			kind = "tool_input_started"
		}
		model := modelDraft(session.nextSequence(), sessionID, nativeType+"/tool_use", kind, nil, "provider_exposed_output", union.ActionID(id), actionPayload(id, name, input))
		bindAttempt(&model, state)
		events := make([]union.UnifiedExecutionEvent, 0, 2)
		events = append(events, attemptEvents...)
		events = append(events, model)
		return events, nil
	case "tool_result":
		id, err := requireString(block, "tool_use_id", "tool_result")
		if err != nil {
			return nil, err
		}
		failed := false
		_ = json.Unmarshal(objectRaw(block, "is_error"), &failed)
		completionEvents, state, err := session.completeAttempt(id, failed, block)
		if err != nil {
			return nil, err
		}
		result := toolResultDraft(session.nextSequence(), sessionID, nativeType+"/tool_result", id, block)
		bindAttempt(&result, state)
		events := make([]union.UnifiedExecutionEvent, 0, 2)
		events = append(events, completionEvents...)
		events = append(events, result)
		return events, nil
	default:
		return []union.UnifiedExecutionEvent{diagnosticDraft(session.nextSequence(), sessionID, nativeType+"/"+blockType, "unknown_native_event", "content_block_unrecognized", "", unknownPayload(block, "content_block", blockType))}, nil
	}
}

func (session *session) decodeControlRequest(raw json.RawMessage) ([]union.UnifiedExecutionEvent, error) {
	requestID, err := requireString(raw, "request_id", "control_request")
	if err != nil {
		return nil, err
	}
	request := objectRaw(raw, "request")
	subtype := objectString(request, "subtype")
	if subtype != "can_use_tool" {
		_ = session.client.RespondError(requestID, "unsupported control request subtype")
		return []union.UnifiedExecutionEvent{diagnosticDraft(session.nextSequence(), session.init.SessionID, "control_request/"+subtype, "unknown_native_event", "control_request_unrecognized", "", unknownPayload(raw, "control_request", subtype))}, nil
	}
	toolName, err := requireString(request, "tool_name", "can_use_tool")
	if err != nil {
		return nil, err
	}
	toolUseID, err := requireString(request, "tool_use_id", "can_use_tool")
	if err != nil {
		return nil, err
	}
	input := objectRaw(request, "input")
	if len(input) == 0 || !json.Valid(input) {
		return nil, fmt.Errorf("%w: can_use_tool input is invalid", ErrProtocol)
	}
	state, attemptEvents, err := session.ensureAttempt(toolUseID, toolName, input, union.ItemStatusPending)
	if err != nil {
		return nil, err
	}
	digest, _ := union.StableDigest(input)
	permission := pendingPermission{
		requestID: requestID, toolUseID: toolUseID, toolName: toolName, input: input,
		inputDigest: digest, attemptID: state.attempt.ID,
	}
	session.mu.Lock()
	if _, duplicate := session.permissions[requestID]; duplicate {
		session.mu.Unlock()
		return nil, fmt.Errorf("%w: duplicate permission request %s", ErrProtocol, requestID)
	}
	session.permissions[requestID] = permission
	session.mu.Unlock()
	events := make([]union.UnifiedExecutionEvent, 0, 2)
	events = append(events, attemptEvents...)
	events = append(events, approvalDraft(
		session.nextSequence(), session.init.SessionID, requestID, toolUseID, toolName, digest,
		state.attempt.ID, session.clock().UTC().Add(session.approvalTTL), request,
	))
	return events, nil
}

func (session *session) resolvePermission(command union.ExecutionCommand) error {
	requestID := string(command.ApprovalID)
	session.mu.Lock()
	permission, exists := session.permissions[requestID]
	session.mu.Unlock()
	if !exists || string(command.ActionID) != permission.toolUseID || command.InputDigest != permission.inputDigest || command.MechanismAttemptID != permission.attemptID {
		return fmt.Errorf("%w: permission correlation failed", ErrUnsupportedCommand)
	}
	response := map[string]any{}
	if command.Kind == union.CommandApproveAction {
		updatedInput := json.RawMessage(permission.input)
		var options struct {
			UpdatedInput json.RawMessage `json:"updated_input"`
		}
		if len(command.Payload) != 0 && json.Unmarshal(command.Payload, &options) == nil && len(options.UpdatedInput) != 0 {
			updatedInput = options.UpdatedInput
		}
		response["behavior"] = "allow"
		response["updatedInput"] = updatedInput
	} else {
		message := "Denied by Praxis RuntimePolicy"
		interrupt := false
		var options struct {
			Message   string `json:"message"`
			Interrupt bool   `json:"interrupt"`
		}
		if len(command.Payload) != 0 && json.Unmarshal(command.Payload, &options) == nil {
			if strings.TrimSpace(options.Message) != "" {
				message = options.Message
			}
			interrupt = options.Interrupt
		}
		response["behavior"] = "deny"
		response["message"] = message
		response["interrupt"] = interrupt
	}
	if err := session.client.Respond(requestID, response); err != nil {
		return err
	}
	session.mu.Lock()
	delete(session.permissions, requestID)
	session.mu.Unlock()
	return nil
}

func (session *session) decodeResult(raw json.RawMessage) ([]union.UnifiedExecutionEvent, error) {
	var result struct {
		Subtype           string          `json:"subtype"`
		IsError           *bool           `json:"is_error"`
		SessionID         string          `json:"session_id"`
		Result            string          `json:"result"`
		Usage             json.RawMessage `json:"usage"`
		PermissionDenials json.RawMessage `json:"permission_denials"`
		Error             json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(raw, &result); err != nil || result.Subtype == "" || result.IsError == nil {
		return nil, fmt.Errorf("%w: malformed SDKResult", ErrProtocol)
	}
	sessionID := result.SessionID
	if sessionID == "" {
		sessionID = session.init.SessionID
	}
	status := union.ExecutionStatusIndeterminate
	if !*result.IsError && result.Subtype == "success" {
		status = union.ExecutionStatusSucceeded
	} else if *result.IsError && (result.Subtype == "error_max_turns" || result.Subtype == "error_during_execution") {
		status = union.ExecutionStatusFailed
	} else if result.Subtype == "interrupted" || result.Subtype == "cancelled" {
		status = union.ExecutionStatusCancelled
	}
	outputEvents := session.structuredOutputEvents(status, result.Result != "")
	session.mu.Lock()
	pending := len(session.pendingTools)
	sideEffects := union.SideEffectNone
	if session.toolObserved {
		sideEffects = union.SideEffectPossible
	}
	if pending != 0 {
		sideEffects = union.SideEffectUnknown
	}
	session.resultSeen = true
	session.terminal = true
	session.mu.Unlock()
	resultPayload, _ := json.Marshal(map[string]any{
		"subtype": result.Subtype, "is_error": *result.IsError, "result": result.Result,
		"usage": result.Usage, "permission_denials": result.PermissionDenials, "error": result.Error,
	})
	return append(outputEvents,
		modelDraft(session.nextSequence(), sessionID, "SDKResult", "route_result", nil, "provider_exposed_output", "", resultPayload),
		terminalDraft(session.nextSequence(), sessionID, execution.RouteTerminalCandidate{
			Status: status, StopReason: result.Subtype, PendingBackgroundWork: pending, SideEffectState: sideEffects,
		}),
	), nil
}

var _ execution.Session = (*session)(nil)

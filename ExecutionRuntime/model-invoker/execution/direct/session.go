package direct

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"
	"sync/atomic"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type session struct {
	ctx      context.Context
	cancel   context.CancelFunc
	backend  Backend
	call     modelinvoker.RouteCall
	request  modelinvoker.Request
	plans    []union.MechanismPlan
	attempt  map[union.MechanismPlanID]union.MechanismAttemptID
	toolPlan map[string]union.MechanismPlan
	toolSet  map[string]struct{}
	callPlan map[string]union.MechanismPlan

	readMu               sync.Mutex
	mu                   sync.Mutex
	queue                []union.UnifiedExecutionEvent
	notify               chan struct{}
	stream               ModelStream
	streamFinalizer      *modelinvoker.ToolCallCandidateStreamFinalizerV1
	invocationID         string
	invocationDigest     core.Digest
	projectionRepository modelinvoker.ToolCallCandidateObservationProjectionRepositoryV1
	pending              map[string]modelinvoker.FunctionCall
	completedCalls       map[string]modelinvoker.FunctionCall
	pendingResults       []modelinvoker.InputItem
	responseState        *modelinvoker.State
	terminal             bool
	attemptsClosed       bool
	closed               bool
	continuing           bool
	err                  error
	sourceSequence       atomic.Uint64

	closeOnce sync.Once
	closeErr  error
}

func newSession(
	ctx context.Context,
	cancel context.CancelFunc,
	backend Backend,
	call modelinvoker.RouteCall,
	request modelinvoker.Request,
	unifiedRequest union.UnifiedExecutionRequest,
	plan union.PreparedExecutionPlan,
	projectionRepository modelinvoker.ToolCallCandidateObservationProjectionRepositoryV1,
) *session {
	selected := selectedMechanisms(plan)
	created := &session{
		ctx: ctx, cancel: cancel, backend: backend, call: call, request: request,
		notify: make(chan struct{}, 1), invocationID: string(unifiedRequest.ExecutionID), invocationDigest: core.Digest(plan.Metadata["request_digest"]),
		projectionRepository: projectionRepository,
		pending:              make(map[string]modelinvoker.FunctionCall), completedCalls: make(map[string]modelinvoker.FunctionCall),
		plans: selected, attempt: make(map[union.MechanismPlanID]union.MechanismAttemptID, len(selected)),
		toolPlan: mapToolPlans(plan, selected, unifiedRequest), toolSet: directToolNames(unifiedRequest), callPlan: make(map[string]union.MechanismPlan),
	}
	for _, mechanism := range selected {
		attemptID := union.MechanismAttemptID("direct:" + string(mechanism.ID) + ":attempt:1")
		created.attempt[mechanism.ID] = attemptID
		created.queue = append(created.queue, mechanismAttemptDraft(mechanism, attemptID, union.AttemptStatusRunning, created.nextSourceSequence()))
	}
	return created
}

func mapToolPlans(plan union.PreparedExecutionPlan, selected []union.MechanismPlan, request union.UnifiedExecutionRequest) map[string]union.MechanismPlan {
	result := make(map[string]union.MechanismPlan)
	byIntent := make(map[union.IntentID]union.MechanismPlan, len(selected))
	toolNames := make(map[string]string, len(request.Tools)*2)
	for _, tool := range request.Tools {
		toolNames[tool.ID] = tool.Name
		toolNames[tool.Name] = tool.Name
	}
	for _, mechanism := range selected {
		byIntent[mechanism.IntentID] = mechanism
	}
	for _, intent := range plan.IntentGraph.Nodes {
		if intent.Kind == union.IntentCallTool {
			if name := toolNames[intent.Target]; name != "" {
				result[name] = byIntent[intent.ID]
			}
		}
	}
	return result
}

func directToolNames(request union.UnifiedExecutionRequest) map[string]struct{} {
	allowedIDs := make(map[string]struct{}, len(request.ToolPolicy.AllowedToolIDs))
	for _, toolID := range request.ToolPolicy.AllowedToolIDs {
		allowedIDs[toolID] = struct{}{}
	}
	result := make(map[string]struct{}, len(request.Tools))
	for _, tool := range request.Tools {
		if len(allowedIDs) != 0 {
			if _, allowed := allowedIDs[tool.ID]; !allowed {
				continue
			}
		}
		result[tool.Name] = struct{}{}
	}
	return result
}

func selectedMechanisms(plan union.PreparedExecutionPlan) []union.MechanismPlan {
	byIntent := make(map[union.IntentID]union.MechanismPlan, len(plan.IntentGraph.Nodes))
	for _, mechanism := range plan.Mechanisms {
		current, exists := byIntent[mechanism.IntentID]
		if !exists || mechanism.PreferredRank < current.PreferredRank ||
			(mechanism.PreferredRank == current.PreferredRank && mechanism.ID < current.ID) {
			byIntent[mechanism.IntentID] = mechanism
		}
	}
	selected := make([]union.MechanismPlan, 0, len(byIntent))
	for _, intent := range plan.IntentGraph.Nodes {
		selected = append(selected, byIntent[intent.ID])
	}
	return selected
}

func (session *session) nextSourceSequence() uint64 {
	return session.sourceSequence.Add(1)
}

func (session *session) observeSourceSequence(sequence uint64) {
	for sequence != 0 {
		current := session.sourceSequence.Load()
		if current >= sequence || session.sourceSequence.CompareAndSwap(current, sequence) {
			return
		}
	}
}

func (session *session) acceptResponse(response modelinvoker.Response) {
	finalized, observation, err := session.finalizeToolResponse(response)
	if err != nil {
		session.mu.Lock()
		events := toolCallProtocolViolation(session.nextSourceSequence)
		events = session.closeAttemptsLocked(events, union.ExecutionStatusIndeterminate)
		session.queue = append(session.queue, events...)
		session.terminal = true
		session.signalLocked()
		session.mu.Unlock()
		return
	}
	response = finalized
	if observation != nil {
		calls := pendingToolCallsFromObservationV1(*observation)
		session.mu.Lock()
		code, batchErr := session.validateToolCallBatchLocked(calls, true)
		if batchErr != nil {
			events := toolCallBatchViolation(code, session.nextSourceSequence)
			events = session.closeAttemptsLocked(events, union.ExecutionStatusIndeterminate)
			session.queue = append(session.queue, events...)
			session.terminal = true
			if code == "unattributed_tool_call" {
				session.err = batchErr
			}
			session.signalLocked()
			session.mu.Unlock()
			return
		}
		session.mu.Unlock()
	}

	stepStartedSequence := session.nextSourceSequence()
	var projection *modelinvoker.ToolCallCandidateObservationProjectionV1
	if observation != nil {
		inspected, publishErr := ensureToolCallObservationProjectionV1(
			session.ctx, session.projectionRepository,
			session.invocationID, response.ID, session.nextSourceSequence(), *observation,
		)
		if publishErr != nil {
			session.mu.Lock()
			events := toolCallBatchViolation(projectionFailureCodeV1(publishErr), session.nextSourceSequence)
			events = session.closeAttemptsLocked(events, union.ExecutionStatusIndeterminate)
			session.queue = append(session.queue, events...)
			session.terminal = true
			session.signalLocked()
			session.mu.Unlock()
			return
		}
		projection = &inspected
	}
	events, pending, terminal, err := responseEvents(response, projection, stepStartedSequence, session.nextSourceSequence)
	if err != nil {
		session.mu.Lock()
		events := toolCallProtocolViolation(session.nextSourceSequence)
		events = session.closeAttemptsLocked(events, union.ExecutionStatusIndeterminate)
		session.queue = append(session.queue, events...)
		session.terminal = true
		session.signalLocked()
		session.mu.Unlock()
		return
	}
	session.mu.Lock()
	terminalStatus := responseStatus(response.Status)
	if code, batchErr := session.validateToolCallBatchLocked(pending, observation != nil); batchErr != nil {
		events = toolCallBatchViolation(code, session.nextSourceSequence)
		pending = make(map[string]modelinvoker.FunctionCall)
		terminal = true
		terminalStatus = union.ExecutionStatusIndeterminate
		if code == "unattributed_tool_call" {
			session.err = batchErr
		}
	}
	if terminal {
		events = session.closeAttemptsLocked(events, terminalStatus)
	}
	for id, call := range pending {
		session.pending[id] = call
	}
	events = session.bindToolCallsLocked(events, pending)
	session.queue = append(session.queue, events...)
	if response.State != nil {
		state := *response.State
		session.responseState = &state
	} else {
		session.responseState = nil
	}
	session.terminal = terminal
	session.signalLocked()
	session.mu.Unlock()
}

func (session *session) finalizeToolResponse(response modelinvoker.Response) (modelinvoker.Response, *modelinvoker.ToolCallCandidateObservationV1, error) {
	if !responseContainsToolCall(response) {
		return response, nil, nil
	}
	observation, err := modelinvoker.FinalizeToolCallCandidateObservationV1(session.invocationDigest, response)
	if err != nil {
		return modelinvoker.Response{}, nil, err
	}
	return responseWithFinalizedToolCalls(response, observation), &observation, nil
}

func responseContainsToolCall(response modelinvoker.Response) bool {
	if response.StopReason == modelinvoker.StopReasonToolCall {
		return true
	}
	for _, output := range response.Output {
		if output.Type == modelinvoker.OutputItemFunctionCall {
			return true
		}
	}
	return false
}

func responseWithFinalizedToolCalls(response modelinvoker.Response, observation modelinvoker.ToolCallCandidateObservationV1) modelinvoker.Response {
	response.Output = append([]modelinvoker.OutputItem(nil), response.Output...)
	callIndex := 0
	for index := range response.Output {
		if response.Output[index].Type != modelinvoker.OutputItemFunctionCall {
			continue
		}
		call := observation.Calls[callIndex]
		response.Output[index].FunctionCall = &modelinvoker.FunctionCall{
			ID: call.CallID, Name: call.Name, Arguments: append(json.RawMessage(nil), call.CanonicalArguments...),
		}
		callIndex++
	}
	return response
}

func (session *session) acceptStreamEventLocked(native modelinvoker.StreamEvent) ([]union.UnifiedExecutionEvent, map[string]modelinvoker.FunctionCall, bool, union.ExecutionStatus) {
	terminalStatus := union.ExecutionStatusIndeterminate
	if native.Response != nil {
		terminalStatus = responseStatus(native.Response.Status)
	} else if native.Type == modelinvoker.StreamEventError {
		terminalStatus = union.ExecutionStatusFailed
	}

	switch native.Type {
	case modelinvoker.StreamEventResponseStarted:
		finalizer, err := session.ensureStreamFinalizerLocked()
		if err == nil {
			_, err = finalizer.Observe(native)
		}
		if err != nil {
			return toolCallProtocolViolation(session.nextSourceSequence), nil, true, union.ExecutionStatusIndeterminate
		}
		events, calls, terminal := streamEventDraft(native, session.nextSourceSequence)
		return events, calls, terminal, terminalStatus

	case modelinvoker.StreamEventFunctionCallStarted,
		modelinvoker.StreamEventFunctionArgumentsDelta,
		modelinvoker.StreamEventFunctionCallCompleted:
		finalizer, err := session.ensureStreamFinalizerLocked()
		if err == nil {
			_, err = finalizer.Observe(native)
		}
		if err != nil {
			return toolCallProtocolViolation(session.nextSourceSequence), nil, true, union.ExecutionStatusIndeterminate
		}
		// Tool-call stream frames are provisional evidence. They are never
		// externally observable before the completed/tool_call terminal snapshot
		// validates the entire batch.
		return nil, nil, false, terminalStatus

	case modelinvoker.StreamEventResponseCompleted:
		if native.Response == nil {
			events, calls, terminal := streamEventDraft(native, session.nextSourceSequence)
			return events, calls, terminal, union.ExecutionStatusIndeterminate
		}
		if !responseContainsToolCall(*native.Response) {
			events, calls, terminal := streamEventDraft(native, session.nextSourceSequence)
			return events, calls, terminal, terminalStatus
		}
		finalizer, err := session.ensureStreamFinalizerLocked()
		var observation *modelinvoker.ToolCallCandidateObservationV1
		if err == nil {
			observation, err = finalizer.Observe(native)
		}
		if err != nil || observation == nil {
			return toolCallProtocolViolation(session.nextSourceSequence), nil, true, union.ExecutionStatusIndeterminate
		}
		response := responseWithFinalizedToolCalls(*native.Response, *observation)
		response.ID, err = finalizer.FinalizedResponseID()
		if err != nil {
			return toolCallProtocolViolation(session.nextSourceSequence), nil, true, union.ExecutionStatusIndeterminate
		}
		calls := pendingToolCallsFromObservationV1(*observation)
		if code, batchErr := session.validateToolCallBatchLocked(calls, true); batchErr != nil {
			if code == "unattributed_tool_call" {
				session.err = batchErr
			}
			return toolCallBatchViolation(code, session.nextSourceSequence), nil, true, union.ExecutionStatusIndeterminate
		}
		projection, publishErr := ensureToolCallObservationProjectionV1(
			session.ctx, session.projectionRepository,
			session.invocationID, response.ID, session.nextSourceSequence(), *observation,
		)
		if publishErr != nil {
			return toolCallBatchViolation(projectionFailureCodeV1(publishErr), session.nextSourceSequence), nil, true, union.ExecutionStatusIndeterminate
		}
		events, calls, err := streamFinalToolCallEvents(response, projection, session.nextSourceSequence)
		if err != nil {
			return toolCallProtocolViolation(session.nextSourceSequence), nil, true, union.ExecutionStatusIndeterminate
		}
		if code, batchErr := session.validateToolCallBatchLocked(calls, true); batchErr != nil {
			if code == "unattributed_tool_call" {
				session.err = batchErr
			}
			return toolCallBatchViolation(code, session.nextSourceSequence), nil, true, union.ExecutionStatusIndeterminate
		}
		return events, calls, false, terminalStatus

	case modelinvoker.StreamEventError:
		// Provider failure discards any provisional tool-call batch. The ordinary
		// stream error remains the only public terminal evidence.
		events, calls, terminal := streamEventDraft(native, session.nextSourceSequence)
		return events, calls, terminal, terminalStatus
	default:
		events, calls, terminal := streamEventDraft(native, session.nextSourceSequence)
		return events, calls, terminal, terminalStatus
	}
}

func (session *session) ensureStreamFinalizerLocked() (*modelinvoker.ToolCallCandidateStreamFinalizerV1, error) {
	if session.streamFinalizer != nil {
		return session.streamFinalizer, nil
	}
	finalizer, err := modelinvoker.NewToolCallCandidateStreamFinalizerV1(session.invocationDigest)
	if err != nil {
		return nil, err
	}
	session.streamFinalizer = finalizer
	return finalizer, nil
}

func streamFinalToolCallEvents(response modelinvoker.Response, projection modelinvoker.ToolCallCandidateObservationProjectionV1, nextSequence func() uint64) ([]union.UnifiedExecutionEvent, map[string]modelinvoker.FunctionCall, error) {
	observationEvent, err := toolCallObservationDraft(projection)
	if err != nil {
		return nil, nil, err
	}
	events := make([]union.UnifiedExecutionEvent, 0, len(response.Output)+2)
	events = append(events, observationEvent)
	calls := make(map[string]modelinvoker.FunctionCall)
	callOrdinal := uint32(0)
	for _, output := range response.Output {
		if output.Type != modelinvoker.OutputItemFunctionCall || output.FunctionCall == nil {
			continue
		}
		call := *output.FunctionCall
		call.Arguments = append(json.RawMessage(nil), call.Arguments...)
		calls[call.ID] = call
		payload := compatibleToolCallPayload(call, callOrdinal, &projection)
		events = append(events, modelDraft("model_tool_call", nextSequence(), &union.ModelEvent{Kind: "model_tool_call", ActionID: union.ActionID(call.ID), Payload: payload}))
		callOrdinal++
	}
	stepPayload, _ := json.Marshal(map[string]any{"status": response.Status, "stop_reason": response.StopReason, "model": response.Model})
	events = append(events, modelDraft("model_step_completed", nextSequence(), &union.ModelEvent{Kind: "model_step_completed", Payload: stepPayload}))
	return events, calls, nil
}

func pendingToolCallsFromObservationV1(observation modelinvoker.ToolCallCandidateObservationV1) map[string]modelinvoker.FunctionCall {
	calls := make(map[string]modelinvoker.FunctionCall, len(observation.Calls))
	for _, candidate := range observation.Calls {
		calls[candidate.CallID] = modelinvoker.FunctionCall{
			ID: candidate.CallID, Name: candidate.Name,
			Arguments: append(json.RawMessage(nil), candidate.CanonicalArguments...),
		}
	}
	return calls
}

func (session *session) validateToolCallBatchLocked(calls map[string]modelinvoker.FunctionCall, required bool) (string, error) {
	if err := validatePendingToolCalls(calls); err != nil || required && len(calls) == 0 {
		if err == nil {
			err = fmt.Errorf("%w: completed tool-call response contains no calls", ErrProtocolTerminal)
		}
		return "invalid_tool_call", err
	}
	for id, call := range calls {
		if _, exists := session.pending[id]; exists {
			return "duplicate_tool_call", fmt.Errorf("%w: provider repeated a pending tool call identity", ErrProtocolTerminal)
		}
		if _, exists := session.completedCalls[id]; exists {
			return "duplicate_tool_call", fmt.Errorf("%w: provider reused a completed tool call identity", ErrProtocolTerminal)
		}
		if _, exists := session.callPlan[id]; exists {
			return "duplicate_tool_call", fmt.Errorf("%w: provider reused a bound tool call identity", ErrProtocolTerminal)
		}
		if _, ok := session.planForTool(call.Name); !ok {
			return "unattributed_tool_call", fmt.Errorf("%w: tool call cannot be attributed to an allowed planned mechanism", ErrProtocolTerminal)
		}
	}
	return "", nil
}

func (session *session) Receive(ctx context.Context) (union.UnifiedExecutionEvent, error) {
	if session == nil {
		return union.UnifiedExecutionEvent{}, execution.ErrSessionClosed
	}
	if ctx == nil {
		return union.UnifiedExecutionEvent{}, context.Canceled
	}
	session.readMu.Lock()
	defer session.readMu.Unlock()
	for {
		session.mu.Lock()
		if len(session.queue) > 0 {
			event := session.queue[0]
			session.queue[0] = union.UnifiedExecutionEvent{}
			session.queue = session.queue[1:]
			session.mu.Unlock()
			return event, nil
		}
		if session.err != nil {
			err := session.err
			session.mu.Unlock()
			return union.UnifiedExecutionEvent{}, err
		}
		if session.terminal || session.closed {
			session.mu.Unlock()
			return union.UnifiedExecutionEvent{}, io.EOF
		}
		stream := session.stream
		pending := len(session.pending)
		notify := session.notify
		session.mu.Unlock()

		if stream != nil {
			if stream.Next() {
				native := stream.Event()
				session.observeSourceSequence(uint64(native.Sequence))
				session.mu.Lock()
				events, calls, terminal, terminalStatus := session.acceptStreamEventLocked(native)
				if terminal {
					events = session.closeAttemptsLocked(events, terminalStatus)
				}
				for id, call := range calls {
					session.pending[id] = call
				}
				events = session.bindToolCallsLocked(events, calls)
				session.queue = append(session.queue, events...)
				if native.Response != nil {
					if native.Response.State != nil {
						state := *native.Response.State
						session.responseState = &state
					}
					_ = stream.Close()
					session.stream = nil
					session.streamFinalizer = nil
				} else if terminal {
					_ = stream.Close()
					session.stream = nil
					session.streamFinalizer = nil
				}
				if terminal {
					session.terminal = true
				}
				session.signalLocked()
				session.mu.Unlock()
				continue
			}
			streamErr := stream.Err()
			closeErr := stream.Close()
			session.mu.Lock()
			session.stream = nil
			session.streamFinalizer = nil
			if streamErr != nil || closeErr != nil {
				session.err = mapError(errors.Join(streamErr, closeErr))
			} else if !session.terminal && len(session.pending) == 0 {
				violations := protocolViolation("stream_eof", session.nextSourceSequence)
				violations = session.closeAttemptsLocked(violations, union.ExecutionStatusIndeterminate)
				session.queue = append(session.queue, violations...)
				session.terminal = true
			}
			session.signalLocked()
			session.mu.Unlock()
			continue
		}
		if pending > 0 {
			select {
			case <-ctx.Done():
				return union.UnifiedExecutionEvent{}, ctx.Err()
			case <-session.ctx.Done():
				return union.UnifiedExecutionEvent{}, session.ctx.Err()
			case <-notify:
				continue
			}
		}
		return union.UnifiedExecutionEvent{}, ErrProtocolTerminal
	}
}

func (session *session) Command(ctx context.Context, command union.ExecutionCommand) error {
	if session == nil {
		return execution.ErrSessionClosed
	}
	if ctx == nil {
		return context.Canceled
	}
	switch command.Kind {
	case union.CommandCancelExecution, union.CommandInterrupt:
		session.cancel()
		session.mu.Lock()
		if session.stream != nil {
			_ = session.stream.Close()
			session.stream = nil
			session.streamFinalizer = nil
		}
		if !session.terminal {
			events := []union.UnifiedExecutionEvent{
				controlDraft(execution.ControlCancelAcknowledged, session.nextSourceSequence()),
				controlDraft(execution.ControlCancellationQuiesced, session.nextSourceSequence()),
				terminalCandidate(union.ExecutionStatusCancelled, "cancelled", session.nextSourceSequence(), union.SideEffectNone),
			}
			events = session.closeAttemptsLocked(events, union.ExecutionStatusCancelled)
			session.queue = append(session.queue, events...)
			session.terminal = true
		}
		session.signalLocked()
		session.mu.Unlock()
		return nil
	case union.CommandProvideToolResult:
		return session.provideToolResult(ctx, command)
	default:
		return ErrUnsupportedCommand
	}
}

func (session *session) closeAttemptsLocked(events []union.UnifiedExecutionEvent, status union.ExecutionStatus) []union.UnifiedExecutionEvent {
	if session.attemptsClosed {
		return events
	}
	closed := make([]union.UnifiedExecutionEvent, 0, len(events)+len(session.plans))
	insertAt := len(events)
	if len(events) > 0 && events[len(events)-1].Diagnostic != nil && events[len(events)-1].Diagnostic.Kind == execution.EventKindRouteTerminalCandidate {
		insertAt--
	}
	closed = append(closed, events[:insertAt]...)
	for _, mechanism := range session.plans {
		closed = append(closed, mechanismAttemptDraft(mechanism, session.attempt[mechanism.ID], attemptStatusForExecution(status), session.nextSourceSequence()))
	}
	if insertAt < len(events) {
		// The terminal draft was created before the completion attempts but is
		// committed after them. Reassign only its source order so both source
		// and Praxis-global order remain monotonic.
		events[insertAt].Header.SourceSequence = session.nextSourceSequence()
	}
	closed = append(closed, events[insertAt:]...)
	session.attemptsClosed = true
	return closed
}

func (session *session) provideToolResult(ctx context.Context, command union.ExecutionCommand) error {
	var result toolResultPayload
	if len(command.Payload) == 0 || json.Unmarshal(command.Payload, &result) != nil || result.CallID == "" || union.ActionID(result.CallID) != command.ActionID || result.Executed == nil {
		return fmt.Errorf("%w: tool result payload is invalid", ErrUnsupportedCommand)
	}
	if *result.Executed {
		if result.ResultOrigin != union.EventOriginExternal && result.ResultOrigin != union.EventOriginPraxis {
			return fmt.Errorf("%w: executed caller result needs an external or Praxis origin", ErrUnsupportedCommand)
		}
		if !validToolResultSideEffectState(result.SideEffectState) {
			return fmt.Errorf("%w: executed caller result needs a valid explicit side-effect state", ErrUnsupportedCommand)
		}
	} else if result.SyntheticReason == "" || result.SideEffectState != union.SideEffectNone {
		return fmt.Errorf("%w: synthetic caller result needs a reason and side_effect_state=none", ErrUnsupportedCommand)
	}
	session.mu.Lock()
	if session.closed || session.terminal {
		session.mu.Unlock()
		return execution.ErrSessionClosed
	}
	call, exists := session.pending[result.CallID]
	if !exists {
		session.mu.Unlock()
		return fmt.Errorf("%w: tool call is not pending", ErrUnsupportedCommand)
	}
	if result.Name != "" && result.Name != call.Name {
		session.mu.Unlock()
		return fmt.Errorf("%w: tool result name differs from the pending call", ErrUnsupportedCommand)
	}
	result.Name = call.Name
	delete(session.pending, result.CallID)
	session.completedCalls[result.CallID] = call
	plan := session.callPlan[result.CallID]
	attemptID := session.attempt[plan.ID]
	resultEvent := toolResultDraft(plan, attemptID, call, result, session.nextSourceSequence())
	itemStatus := union.ItemStatusCompleted
	if !*result.Executed {
		itemStatus = union.ItemStatusIncomplete
	} else if result.IsError {
		itemStatus = union.ItemStatusFailed
	}
	itemSummary := append(json.RawMessage(nil), resultEvent.Model.Payload...)
	session.queue = append(session.queue, resultEvent, toolItemDraft(plan, attemptID, call, itemStatus, result.SideEffectState, session.nextSourceSequence(), itemSummary))
	session.pendingResults = append(session.pendingResults, modelinvoker.NamedFunctionResultInput(result.CallID, result.Name, result.Output, result.IsError))
	if len(session.pending) > 0 {
		session.signalLocked()
		session.mu.Unlock()
		return nil
	}
	if session.continuing {
		session.mu.Unlock()
		return fmt.Errorf("%w: continuation is already running", ErrUnsupportedCommand)
	}
	session.continuing = true
	next := session.nextRequestLocked()
	session.pendingResults = nil
	session.mu.Unlock()

	callRequest := session.call
	callRequest.Request = next
	if next.Stream {
		stream, err := session.backend.OpenStream(ctx, callRequest)
		session.mu.Lock()
		session.continuing = false
		if err != nil {
			session.err = mapError(err)
		} else {
			session.request = next
			session.call = callRequest
			session.stream = stream
			session.streamFinalizer = nil
		}
		session.signalLocked()
		session.mu.Unlock()
		return err
	}
	invoked, err := session.backend.Invoke(ctx, callRequest)
	session.mu.Lock()
	session.continuing = false
	if err != nil {
		session.err = mapError(err)
		session.signalLocked()
		session.mu.Unlock()
		return err
	}
	session.request = next
	session.call = callRequest
	session.mu.Unlock()
	session.acceptResponse(invoked.Response)
	return nil
}

func (session *session) bindToolCallsLocked(events []union.UnifiedExecutionEvent, calls map[string]modelinvoker.FunctionCall) []union.UnifiedExecutionEvent {
	if len(calls) == 0 {
		return events
	}
	bound := make([]union.UnifiedExecutionEvent, 0, len(events)+len(calls))
	items := make([]union.UnifiedExecutionEvent, 0, len(calls))
	boundCalls := make(map[string]struct{}, len(calls))
	for _, event := range events {
		if event.Model == nil || event.Model.Kind != "model_tool_call" || event.Model.ActionID == "" {
			bound = append(bound, event)
			continue
		}
		call, exists := calls[string(event.Model.ActionID)]
		if !exists {
			bound = append(bound, event)
			continue
		}
		if _, duplicate := boundCalls[call.ID]; duplicate {
			session.err = fmt.Errorf("%w: provider repeated a tool call identity", ErrProtocolTerminal)
			bound = append(bound, diagnosticDraft("protocol_violation", "duplicate_tool_call", session.nextSourceSequence(), nil))
			continue
		}
		boundCalls[call.ID] = struct{}{}
		plan, ok := session.planForTool(call.Name)
		if !ok {
			session.err = fmt.Errorf("%w: tool call cannot be attributed to an allowed planned mechanism", ErrProtocolTerminal)
			bound = append(bound, event, diagnosticDraft("protocol_violation", "unattributed_tool_call", session.nextSourceSequence(), nil))
			continue
		}
		attemptID := session.attempt[plan.ID]
		event.Header.IntentID, event.Header.MechanismPlanID, event.Header.MechanismAttemptID = plan.IntentID, plan.ID, attemptID
		itemID := union.ItemID("direct-tool:" + call.ID)
		event.Header.ItemID, event.Header.ActionID = itemID, event.Model.ActionID
		event.Model.ExecutionItemID = itemID
		session.callPlan[call.ID] = plan
		bound = append(bound, event)
		items = append(items, toolItemDraft(plan, attemptID, call, union.ItemStatusPending, union.SideEffectNone, event.Header.SourceSequence, actionPayloadSummary(call)))
	}
	// Some provider streams report tool calls only in the final Response and do
	// not emit a separate function-call-completed frame. Materialize those calls
	// here so their Action/Item/Attempt identities are still explicit before a
	// caller result can continue the route.
	callIDs := make([]string, 0, len(calls))
	for callID := range calls {
		if _, alreadyBound := session.callPlan[callID]; alreadyBound {
			continue
		}
		if _, emitted := boundCalls[callID]; emitted {
			continue
		}
		callIDs = append(callIDs, callID)
	}
	sort.Strings(callIDs)
	for _, callID := range callIDs {
		call := calls[callID]
		plan, ok := session.planForTool(call.Name)
		if !ok {
			session.err = fmt.Errorf("%w: tool call cannot be attributed to an allowed planned mechanism", ErrProtocolTerminal)
			bound = append(bound, diagnosticDraft("protocol_violation", "unattributed_tool_call", session.nextSourceSequence(), nil))
			continue
		}
		payload, _ := json.Marshal(map[string]any{"call_id": call.ID, "name": call.Name, "arguments": json.RawMessage(call.Arguments)})
		event := modelDraft("model_tool_call", session.nextSourceSequence(), &union.ModelEvent{Kind: "model_tool_call", ActionID: union.ActionID(call.ID), Payload: payload})
		attemptID := session.attempt[plan.ID]
		itemID := union.ItemID("direct-tool:" + call.ID)
		event.Header.IntentID, event.Header.MechanismPlanID, event.Header.MechanismAttemptID = plan.IntentID, plan.ID, attemptID
		event.Header.ItemID, event.Header.ActionID = itemID, event.Model.ActionID
		event.Model.ExecutionItemID = itemID
		session.callPlan[call.ID] = plan
		bound = append(bound, event)
		items = append(items, toolItemDraft(plan, attemptID, call, union.ItemStatusPending, union.SideEffectNone, event.Header.SourceSequence, actionPayloadSummary(call)))
	}
	return append(bound, items...)
}

func validatePendingToolCalls(calls map[string]modelinvoker.FunctionCall) error {
	for id, call := range calls {
		if id == "" || call.ID != id || call.Name == "" || !jsonObject(call.Arguments) {
			return fmt.Errorf("%w: provider tool call identity or arguments are invalid", ErrProtocolTerminal)
		}
	}
	return nil
}

func toolCallProtocolViolation(nextSequence func() uint64) []union.UnifiedExecutionEvent {
	return toolCallBatchViolation("invalid_tool_call", nextSequence)
}

func toolCallBatchViolation(code string, nextSequence func() uint64) []union.UnifiedExecutionEvent {
	if code == "" {
		code = "invalid_tool_call"
	}
	payload, _ := json.Marshal(map[string]string{"reason": code})
	return []union.UnifiedExecutionEvent{
		diagnosticDraft("protocol_violation", code, nextSequence(), payload),
		terminalCandidate(union.ExecutionStatusIndeterminate, code, nextSequence(), union.SideEffectUnknown),
	}
}

func validToolResultSideEffectState(state union.SideEffectState) bool {
	switch state {
	case union.SideEffectNone, union.SideEffectPossible, union.SideEffectObserved, union.SideEffectReconciled, union.SideEffectUnknown:
		return true
	default:
		return false
	}
}

func (session *session) planForTool(name string) (union.MechanismPlan, bool) {
	if _, allowed := session.toolSet[name]; !allowed {
		return union.MechanismPlan{}, false
	}
	if plan, exists := session.toolPlan[name]; exists {
		return plan, true
	}
	if len(session.plans) == 1 {
		return session.plans[0], true
	}
	return union.MechanismPlan{}, false
}

func actionPayloadSummary(call modelinvoker.FunctionCall) json.RawMessage {
	digest, _ := union.StableDigest(call.Arguments)
	payload, _ := json.Marshal(map[string]any{"call_id": call.ID, "name": call.Name, "input_digest": digest, "input_bytes": len(call.Arguments)})
	return payload
}

func (session *session) nextRequestLocked() modelinvoker.Request {
	next := session.request
	results := append([]modelinvoker.InputItem(nil), session.pendingResults...)
	if session.responseState != nil {
		state := *session.responseState
		next.State = &state
		next.Input = results
		return next
	}
	ids := make([]string, 0, len(session.pendingResults))
	resultByID := make(map[string]modelinvoker.InputItem, len(results))
	for _, result := range results {
		if result.FunctionResult != nil {
			ids = append(ids, result.FunctionResult.CallID)
			resultByID[result.FunctionResult.CallID] = result
		}
	}
	sort.Strings(ids)
	next.Input = append([]modelinvoker.InputItem(nil), session.request.Input...)
	for _, id := range ids {
		if call, exists := session.completedCalls[id]; exists {
			next.Input = append(next.Input, modelinvoker.FunctionCallInput(call.ID, call.Name, call.Arguments))
		}
		if result, exists := resultByID[id]; exists {
			next.Input = append(next.Input, result)
		}
		delete(session.completedCalls, id)
	}
	return next
}

func (session *session) signalLocked() {
	select {
	case session.notify <- struct{}{}:
	default:
	}
}

func (session *session) Close() error {
	if session == nil {
		return nil
	}
	session.closeOnce.Do(func() {
		session.cancel()
		session.mu.Lock()
		session.closed = true
		if session.stream != nil {
			session.closeErr = session.stream.Close()
			session.stream = nil
		}
		session.signalLocked()
		session.mu.Unlock()
	})
	return session.closeErr
}

var _ execution.Session = (*session)(nil)

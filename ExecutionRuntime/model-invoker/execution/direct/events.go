package direct

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func modelDraft(kind string, sourceSequence uint64, payload *union.ModelEvent) union.UnifiedExecutionEvent {
	return union.UnifiedExecutionEvent{
		Header: union.EventHeader{
			SourceSequence: sourceSequence, Origin: union.EventOriginProvider, Family: union.EventFamilyModel,
			Visibility: union.VisibilityUserVisible, SecurityClassification: union.SecurityInternal,
		},
		Model: payload,
	}
}

func toolCallObservationDraft(projection modelinvoker.ToolCallCandidateObservationProjectionV1) (union.UnifiedExecutionEvent, error) {
	payload, err := json.Marshal(projection)
	if err != nil {
		return union.UnifiedExecutionEvent{}, err
	}
	event := union.UnifiedExecutionEvent{
		Header: union.EventHeader{
			SourceSequence: projection.Ref.Source.SourceSequence, Origin: union.EventOriginProvider, Family: union.EventFamilyModel,
			Visibility: union.VisibilityAuditOnly, SecurityClassification: union.SecurityInternal,
		},
		Model: &union.ModelEvent{Kind: modelinvoker.ToolCallCandidateObservationModelEventKindV1, Payload: payload},
	}
	return event, nil
}

func diagnosticDraft(kind, code string, sourceSequence uint64, payload json.RawMessage) union.UnifiedExecutionEvent {
	return union.UnifiedExecutionEvent{
		Header: union.EventHeader{
			SourceSequence: sourceSequence, Origin: union.EventOriginProvider, Family: union.EventFamilyDiagnostic,
			Visibility: union.VisibilityAuditOnly, SecurityClassification: union.SecurityInternal,
		},
		Diagnostic: &union.DiagnosticEvent{Kind: kind, Code: code, Payload: payload},
	}
}

func controlDraft(kind string, sourceSequence uint64) union.UnifiedExecutionEvent {
	return union.UnifiedExecutionEvent{
		Header: union.EventHeader{
			SourceSequence: sourceSequence, Origin: union.EventOriginProvider, Family: union.EventFamilyControl,
			Visibility: union.VisibilityAuditOnly, SecurityClassification: union.SecurityInternal,
		},
		Control: &union.ControlEvent{Kind: kind},
	}
}

func mechanismAttemptDraft(plan union.MechanismPlan, attemptID union.MechanismAttemptID, status union.AttemptStatus, sourceSequence uint64) union.UnifiedExecutionEvent {
	return union.UnifiedExecutionEvent{
		Header: union.EventHeader{
			SourceSequence: sourceSequence, Origin: union.EventOriginProvider, Family: union.EventFamilyMechanism,
			Visibility: union.VisibilityAuditOnly, SecurityClassification: union.SecurityInternal,
			IntentID: plan.IntentID, MechanismPlanID: plan.ID, MechanismAttemptID: attemptID,
		},
		Mechanism: &union.MechanismEvent{Kind: "attempt_" + string(status), Attempt: &union.MechanismAttempt{
			ID: attemptID, MechanismPlanID: plan.ID, Authoritative: true,
			ActualKind: plan.Kind, ActualOrigin: plan.Origin, ActualOwner: plan.Owner,
			Status: status, SideEffectState: union.SideEffectNone,
		}},
	}
}

func toolItemDraft(plan union.MechanismPlan, attemptID union.MechanismAttemptID, call modelinvoker.FunctionCall, status union.ItemStatus, sideEffects union.SideEffectState, sourceSequence uint64, payload json.RawMessage) union.UnifiedExecutionEvent {
	itemID := union.ItemID("direct-tool:" + call.ID)
	header := union.EventHeader{
		SourceSequence: sourceSequence, Origin: union.EventOriginProvider, Family: union.EventFamilyItem,
		Visibility: union.VisibilityProgressOnly, SecurityClassification: union.SecurityInternal,
		IntentID: plan.IntentID, MechanismPlanID: plan.ID, MechanismAttemptID: attemptID,
		ItemID: itemID, ActionID: union.ActionID(call.ID),
	}
	return union.UnifiedExecutionEvent{Header: header, Item: &union.ItemEvent{
		Kind: "caller_tool_execution", Item: union.ExecutionItem{
			ID: itemID, Kind: "caller_tool", Status: status, ActionID: union.ActionID(call.ID), AttemptID: attemptID,
			SideEffectState: sideEffects, Payload: append(json.RawMessage(nil), payload...),
		},
	}}
}

func toolResultDraft(plan union.MechanismPlan, attemptID union.MechanismAttemptID, call modelinvoker.FunctionCall, result toolResultPayload, sourceSequence uint64) union.UnifiedExecutionEvent {
	itemID := union.ItemID("direct-tool:" + call.ID)
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(result.Output)))
	payload, _ := json.Marshal(map[string]any{
		"call_id": result.CallID, "name": result.Name, "output_digest": digest, "output_bytes": len(result.Output),
		"is_error": result.IsError, "synthetic_reason": result.SyntheticReason,
	})
	header := union.EventHeader{
		SourceSequence: sourceSequence, Origin: union.EventOriginProvider, Family: union.EventFamilyModel,
		Visibility: union.VisibilityAuditOnly, SecurityClassification: union.SecurityInternal,
		IntentID: plan.IntentID, MechanismPlanID: plan.ID, MechanismAttemptID: attemptID,
		ItemID: itemID, ActionID: union.ActionID(call.ID),
	}
	return union.UnifiedExecutionEvent{Header: header, Model: &union.ModelEvent{
		Kind: "tool_result_provided", ActionID: union.ActionID(call.ID), ExecutionItemID: itemID,
		ResultOrigin: result.ResultOrigin, Executed: result.Executed, SyntheticReason: result.SyntheticReason, Payload: payload,
	}}
}

func attemptStatusForExecution(status union.ExecutionStatus) union.AttemptStatus {
	switch status {
	case union.ExecutionStatusSucceeded:
		return union.AttemptStatusCompleted
	case union.ExecutionStatusFailed:
		return union.AttemptStatusFailed
	case union.ExecutionStatusCancelled:
		return union.AttemptStatusCancelled
	default:
		return union.AttemptStatusIndeterminate
	}
}

func responseEvents(response modelinvoker.Response, projection *modelinvoker.ToolCallCandidateObservationProjectionV1, stepStartedSequence uint64, nextSequence func() uint64) ([]union.UnifiedExecutionEvent, map[string]modelinvoker.FunctionCall, bool, error) {
	if stepStartedSequence == 0 {
		stepStartedSequence = nextSequence()
	}
	events := []union.UnifiedExecutionEvent{modelDraft("model_step_started", stepStartedSequence, &union.ModelEvent{Kind: "model_step_started"})}
	if projection != nil {
		event, err := toolCallObservationDraft(*projection)
		if err != nil {
			return nil, nil, true, err
		}
		events = append(events, event)
	}
	pending := make(map[string]modelinvoker.FunctionCall)
	callOrdinal := uint32(0)
	for _, output := range response.Output {
		switch output.Type {
		case modelinvoker.OutputItemText:
			if output.Text != "" {
				events = append(events, modelDraft("content_completed", nextSequence(), &union.ModelEvent{Kind: "content_completed", Content: []union.ContentPart{{Kind: "text", Text: output.Text}}}))
			}
		case modelinvoker.OutputItemReasoningSummary:
			if output.ReasoningSummary != "" {
				events = append(events, modelDraft("reasoning_summary_completed", nextSequence(), &union.ModelEvent{Kind: "reasoning_summary_completed", DisclosureClass: "public_summary", Content: []union.ContentPart{{Kind: "text", Text: output.ReasoningSummary}}}))
			}
		case modelinvoker.OutputItemFunctionCall:
			if output.FunctionCall != nil {
				call := *output.FunctionCall
				call.Arguments = append(json.RawMessage(nil), call.Arguments...)
				pending[call.ID] = call
				payload := compatibleToolCallPayload(call, callOrdinal, projection)
				events = append(events, modelDraft("model_tool_call", nextSequence(), &union.ModelEvent{Kind: "model_tool_call", ActionID: union.ActionID(call.ID), Payload: payload}))
				callOrdinal++
			}
		}
	}
	usage, _ := json.Marshal(response.Usage)
	events = append(events, modelDraft("model_usage", nextSequence(), &union.ModelEvent{Kind: "model_usage", Payload: usage, Usage: directUsageMetrics(response.Usage)}))
	stepPayload, _ := json.Marshal(map[string]any{"status": response.Status, "stop_reason": response.StopReason, "model": response.Model})
	events = append(events, modelDraft("model_step_completed", nextSequence(), &union.ModelEvent{Kind: "model_step_completed", Payload: stepPayload}))
	terminal := len(pending) == 0 && response.StopReason != modelinvoker.StopReasonToolCall
	if terminal {
		events = append(events, terminalCandidate(responseStatus(response.Status), string(response.StopReason), nextSequence(), union.SideEffectNone))
	}
	return events, pending, terminal, nil
}

func compatibleToolCallPayload(call modelinvoker.FunctionCall, ordinal uint32, projection *modelinvoker.ToolCallCandidateObservationProjectionV1) json.RawMessage {
	payload := map[string]any{"call_id": call.ID, "name": call.Name, "arguments": json.RawMessage(call.Arguments)}
	if projection != nil {
		payload["authority"] = modelinvoker.ToolCallCompatibilityAuthorityV1
		payload["gateway_authoritative"] = false
		payload["observation_ref"] = projection.Ref
		payload["ordinal"] = ordinal
	}
	encoded, _ := json.Marshal(payload)
	return encoded
}

// validateResponseToolCallSequence runs before responseEvents builds its
// call-ID map. This prevents duplicate provider identities from being silently
// collapsed by map assignment.
func validateResponseToolCallSequence(response modelinvoker.Response) error {
	seen := make(map[string]struct{})
	for _, output := range response.Output {
		if output.Type != modelinvoker.OutputItemFunctionCall {
			continue
		}
		if output.FunctionCall == nil || output.FunctionCall.ID == "" {
			return fmt.Errorf("%w: provider tool call identity is missing", ErrProtocolTerminal)
		}
		if _, duplicate := seen[output.FunctionCall.ID]; duplicate {
			return fmt.Errorf("%w: provider repeated a tool call identity", ErrProtocolTerminal)
		}
		seen[output.FunctionCall.ID] = struct{}{}
	}
	return nil
}

func streamEventDraft(event modelinvoker.StreamEvent, nextSequence func() uint64) ([]union.UnifiedExecutionEvent, map[string]modelinvoker.FunctionCall, bool) {
	sequence := uint64(event.Sequence)
	if sequence == 0 {
		sequence = nextSequence()
	}
	switch event.Type {
	case modelinvoker.StreamEventResponseStarted:
		return []union.UnifiedExecutionEvent{modelDraft("model_step_started", sequence, &union.ModelEvent{Kind: "model_step_started"})}, nil, false
	case modelinvoker.StreamEventTextDelta:
		return []union.UnifiedExecutionEvent{modelDraft("content_delta", sequence, &union.ModelEvent{Kind: "content_delta", Content: []union.ContentPart{{Kind: "text", Text: event.TextDelta}}})}, nil, false
	case modelinvoker.StreamEventReasoningDelta:
		return []union.UnifiedExecutionEvent{modelDraft("reasoning_summary_delta", sequence, &union.ModelEvent{Kind: "reasoning_summary_delta", DisclosureClass: "provider_exposed", Content: []union.ContentPart{{Kind: "text", Text: event.ReasoningDelta}}})}, nil, false
	case modelinvoker.StreamEventFunctionCallStarted, modelinvoker.StreamEventFunctionArgumentsDelta, modelinvoker.StreamEventFunctionCallCompleted:
		kind := map[modelinvoker.StreamEventType]string{
			modelinvoker.StreamEventFunctionCallStarted:    "tool_input_started",
			modelinvoker.StreamEventFunctionArgumentsDelta: "tool_input_delta",
			modelinvoker.StreamEventFunctionCallCompleted:  "model_tool_call",
		}[event.Type]
		payload, _ := json.Marshal(map[string]any{"arguments_delta": event.ArgumentsDelta, "function_call": event.FunctionCall})
		modelEvent := &union.ModelEvent{Kind: kind, Payload: payload}
		pending := map[string]modelinvoker.FunctionCall(nil)
		if event.FunctionCall != nil {
			modelEvent.ActionID = union.ActionID(event.FunctionCall.ID)
			if event.Type == modelinvoker.StreamEventFunctionCallCompleted {
				call := *event.FunctionCall
				call.Arguments = append(json.RawMessage(nil), call.Arguments...)
				pending = map[string]modelinvoker.FunctionCall{call.ID: call}
			}
		}
		return []union.UnifiedExecutionEvent{modelDraft(kind, sequence, modelEvent)}, pending, false
	case modelinvoker.StreamEventUsage:
		payload, _ := json.Marshal(event.Usage)
		var metrics []union.UsageMetric
		if event.Usage != nil {
			metrics = directUsageMetrics(*event.Usage)
		}
		return []union.UnifiedExecutionEvent{modelDraft("model_usage", sequence, &union.ModelEvent{Kind: "model_usage", Payload: payload, Usage: metrics})}, nil, false
	case modelinvoker.StreamEventResponseCompleted:
		if event.Response == nil {
			return []union.UnifiedExecutionEvent{diagnosticDraft("protocol_violation", "response_missing", sequence, nil), terminalCandidate(union.ExecutionStatusIndeterminate, "response_missing", nextSequence(), union.SideEffectUnknown)}, nil, true
		}
		pending := make(map[string]modelinvoker.FunctionCall)
		for _, call := range event.Response.FunctionCalls() {
			pending[call.ID] = call
		}
		stepPayload, _ := json.Marshal(map[string]any{"status": event.Response.Status, "stop_reason": event.Response.StopReason, "model": event.Response.Model})
		events := []union.UnifiedExecutionEvent{modelDraft("model_step_completed", sequence, &union.ModelEvent{Kind: "model_step_completed", Payload: stepPayload})}
		terminal := len(pending) == 0 && event.Response.StopReason != modelinvoker.StopReasonToolCall
		if terminal {
			events = append(events, terminalCandidate(responseStatus(event.Response.Status), string(event.Response.StopReason), nextSequence(), union.SideEffectNone))
		}
		return events, pending, terminal
	case modelinvoker.StreamEventError:
		message := "provider stream error"
		if event.Error != nil {
			message = event.Error.Error()
		}
		payload, _ := json.Marshal(map[string]string{"message": message})
		return []union.UnifiedExecutionEvent{diagnosticDraft("provider_error", "stream_error", sequence, payload), terminalCandidate(union.ExecutionStatusFailed, "stream_error", nextSequence(), union.SideEffectNone)}, nil, true
	case modelinvoker.StreamEventNative:
		payload := event.Raw.Bytes()
		digest := fmt.Sprintf("sha256:%x", sha256.Sum256(payload))
		summary, _ := json.Marshal(map[string]any{"digest": digest, "bytes": len(payload), "json": json.Valid(payload)})
		return []union.UnifiedExecutionEvent{diagnosticDraft("native_event", "retained_extension", sequence, summary)}, nil, false
	default:
		payload, _ := json.Marshal(map[string]string{"native_type": string(event.Type)})
		return []union.UnifiedExecutionEvent{diagnosticDraft("unknown_native_event", "event_unrecognized", sequence, payload)}, nil, false
	}
}

func directUsageMetrics(usage modelinvoker.Usage) []union.UsageMetric {
	values := []struct {
		kind  string
		value int64
	}{
		{"input_tokens", usage.InputTokens},
		{"output_tokens", usage.OutputTokens},
		{"reasoning_tokens", usage.ReasoningTokens},
		{"cache_read_tokens", usage.CacheReadTokens},
		{"cache_write_tokens", usage.CacheWriteTokens},
		{"total_tokens", usage.TotalTokens},
	}
	metrics := make([]union.UsageMetric, 0, len(values))
	for _, value := range values {
		if value.value == 0 {
			continue
		}
		metrics = append(metrics, union.UsageMetric{
			Kind: value.kind, Value: float64(value.value), Unit: "tokens", Scope: "model_step",
			Source: "provider", Quality: "reported",
		})
	}
	return metrics
}

func terminalCandidate(status union.ExecutionStatus, stopReason string, sourceSequence uint64, sideEffects union.SideEffectState) union.UnifiedExecutionEvent {
	payload, _ := json.Marshal(execution.RouteTerminalCandidate{Status: status, StopReason: stopReason, SideEffectState: sideEffects})
	return diagnosticDraft(execution.EventKindRouteTerminalCandidate, "", sourceSequence, payload)
}

func responseStatus(status modelinvoker.ResponseStatus) union.ExecutionStatus {
	switch status {
	case modelinvoker.ResponseStatusCompleted:
		return union.ExecutionStatusSucceeded
	case modelinvoker.ResponseStatusIncomplete, modelinvoker.ResponseStatusInProgress:
		return union.ExecutionStatusPartial
	case modelinvoker.ResponseStatusCancelled:
		return union.ExecutionStatusCancelled
	default:
		return union.ExecutionStatusFailed
	}
}

func protocolViolation(reason string, nextSequence func() uint64) []union.UnifiedExecutionEvent {
	payload, _ := json.Marshal(map[string]string{"reason": reason})
	return []union.UnifiedExecutionEvent{
		diagnosticDraft("protocol_violation", "missing_terminal", nextSequence(), payload),
		terminalCandidate(union.ExecutionStatusIndeterminate, reason, nextSequence(), union.SideEffectUnknown),
	}
}

func mapError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("direct model route: %w", err)
}

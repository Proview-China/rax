package claude

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func eventHeader(family union.EventFamily, sequence uint64, sessionID, nativeType string) union.EventHeader {
	header := execution.CandidateHeader(union.EventOriginHarness, family)
	header.SourceSequence = sequence
	header.SessionID = union.SessionID(sessionID)
	header.NativeIdentity = &union.NativeIdentity{Namespace: "anthropic.claude-agent-sdk", Kind: "message", Value: nativeType}
	return header
}

func manifestDraft(sequence uint64, init InitMessage, manifest union.ContextManifestSummary) union.UnifiedExecutionEvent {
	clone, _ := manifest.Clone()
	return union.UnifiedExecutionEvent{
		Header:     eventHeader(union.EventFamilyDiagnostic, sequence, init.SessionID, "system/init"),
		Diagnostic: &union.DiagnosticEvent{Kind: "actual_manifest", Code: "system_init", Manifest: &clone, Payload: append(json.RawMessage(nil), init.Raw...)},
	}
}

func modelDraft(sequence uint64, sessionID, nativeType, kind string, content []union.ContentPart, disclosure string, actionID union.ActionID, payload json.RawMessage) union.UnifiedExecutionEvent {
	header := eventHeader(union.EventFamilyModel, sequence, sessionID, nativeType)
	header.ActionID = actionID
	header.Visibility = union.VisibilityUserVisible
	if disclosure == "provider_exposed_reasoning" {
		header.Visibility = union.VisibilityAuditOnly
	}
	return union.UnifiedExecutionEvent{
		Header: header,
		Model:  &union.ModelEvent{Kind: kind, Content: content, DisclosureClass: disclosure, ActionID: actionID, Payload: append(json.RawMessage(nil), payload...)},
	}
}

func toolResultDraft(sequence uint64, sessionID, nativeType, toolUseID string, payload json.RawMessage) union.UnifiedExecutionEvent {
	executed := true
	event := modelDraft(sequence, sessionID, nativeType, "model_tool_result", nil, "provider_exposed_output", union.ActionID(toolUseID), payload)
	event.Model.ResultOrigin = union.EventOriginHarness
	event.Model.Executed = &executed
	event.Header.ItemID = union.ItemID(toolUseID)
	event.Model.ExecutionItemID = union.ItemID(toolUseID)
	return event
}

func itemDraft(sequence uint64, sessionID, nativeType, itemID, kind string, status union.ItemStatus, sideEffects union.SideEffectState, payload json.RawMessage) union.UnifiedExecutionEvent {
	header := eventHeader(union.EventFamilyItem, sequence, sessionID, nativeType)
	header.ItemID = union.ItemID(itemID)
	return union.UnifiedExecutionEvent{
		Header: header,
		Item: &union.ItemEvent{Kind: kind, Item: union.ExecutionItem{
			ID: union.ItemID(itemID), Kind: kind, Status: status, SideEffectState: sideEffects,
			Payload: append(json.RawMessage(nil), payload...),
		}},
	}
}

func toolItemDraft(sequence uint64, sessionID, nativeType, toolUseID string, plan union.MechanismPlan, attempt union.MechanismAttempt, status union.ItemStatus, payload json.RawMessage) union.UnifiedExecutionEvent {
	header := eventHeader(union.EventFamilyItem, sequence, sessionID, nativeType)
	header.ItemID = union.ItemID(toolUseID)
	header.ActionID = union.ActionID(toolUseID)
	header.IntentID = plan.IntentID
	header.MechanismPlanID = plan.ID
	header.MechanismAttemptID = attempt.ID
	return union.UnifiedExecutionEvent{
		Header: header,
		Item: &union.ItemEvent{Kind: "tool_action", Item: union.ExecutionItem{
			ID: union.ItemID(toolUseID), Kind: "tool_action", Status: status,
			ActionID: union.ActionID(toolUseID), AttemptID: attempt.ID,
			SideEffectState: attempt.SideEffectState, Payload: append(json.RawMessage(nil), payload...),
		}},
	}
}

func approvalDraft(sequence uint64, sessionID, requestID, toolUseID, toolName, inputDigest string, attemptID union.MechanismAttemptID, expiresAt time.Time, payload json.RawMessage) union.UnifiedExecutionEvent {
	actionID := union.ActionID(toolUseID)
	header := eventHeader(union.EventFamilyControl, sequence, sessionID, "control_request/can_use_tool")
	header.ApprovalID = union.ApprovalID(requestID)
	header.ActionID = actionID
	header.MechanismAttemptID = attemptID
	header.Visibility = union.VisibilityUserVisible
	return union.UnifiedExecutionEvent{
		Header: header,
		Control: &union.ControlEvent{
			Kind: "approval_requested", ApprovalID: union.ApprovalID(requestID), ActionID: actionID,
			MechanismAttemptID: attemptID, InputDigest: inputDigest, ActionRevision: 1,
			Scope: toolName, Authority: "runtime", ExpiresAt: expiresAt, Payload: append(json.RawMessage(nil), payload...),
		},
	}
}

func selectedPlanDraft(sequence uint64, sessionID string, plan union.MechanismPlan) union.UnifiedExecutionEvent {
	header := eventHeader(union.EventFamilyMechanism, sequence, sessionID, "mechanism/selected")
	header.IntentID = plan.IntentID
	header.MechanismPlanID = plan.ID
	return union.UnifiedExecutionEvent{Header: header, Mechanism: &union.MechanismEvent{Kind: "selected", Plan: &plan}}
}

func attemptDraft(sequence uint64, sessionID, nativeType string, plan union.MechanismPlan, attempt union.MechanismAttempt) union.UnifiedExecutionEvent {
	header := eventHeader(union.EventFamilyMechanism, sequence, sessionID, nativeType)
	header.IntentID = plan.IntentID
	header.MechanismPlanID = plan.ID
	header.MechanismAttemptID = attempt.ID
	return union.UnifiedExecutionEvent{Header: header, Mechanism: &union.MechanismEvent{Kind: nativeType, Attempt: &attempt}}
}

func cancellationDraft(sequence uint64, sessionID, kind string, payload json.RawMessage) union.UnifiedExecutionEvent {
	header := eventHeader(union.EventFamilyControl, sequence, sessionID, "control/"+kind)
	return union.UnifiedExecutionEvent{Header: header, Control: &union.ControlEvent{Kind: kind, Payload: append(json.RawMessage(nil), payload...)}}
}

func diagnosticDraft(sequence uint64, sessionID, nativeType, kind, code, message string, payload json.RawMessage) union.UnifiedExecutionEvent {
	return union.UnifiedExecutionEvent{
		Header:     eventHeader(union.EventFamilyDiagnostic, sequence, sessionID, nativeType),
		Diagnostic: &union.DiagnosticEvent{Kind: kind, Code: code, Message: message, Payload: append(json.RawMessage(nil), payload...)},
	}
}

func terminalDraft(sequence uint64, sessionID string, candidate execution.RouteTerminalCandidate) union.UnifiedExecutionEvent {
	payload, _ := json.Marshal(candidate)
	return diagnosticDraft(sequence, sessionID, "result", execution.EventKindRouteTerminalCandidate, candidate.StopReason, "", payload)
}

func unknownPayload(raw json.RawMessage, nativeType, subtype string) json.RawMessage {
	digest, _ := union.StableDigest(raw)
	payload, _ := json.Marshal(map[string]any{
		"type": nativeType, "subtype": subtype, "digest": digest, "bytes": len(raw),
	})
	return payload
}

func textPart(text string) []union.ContentPart {
	if text == "" {
		return nil
	}
	return []union.ContentPart{{Kind: "text", Text: text}}
}

func actionPayload(id, name string, input json.RawMessage) json.RawMessage {
	payload, _ := json.Marshal(map[string]any{"tool_use_id": id, "name": name, "input": input})
	return payload
}

func requireString(raw json.RawMessage, key, context string) (string, error) {
	value := objectString(raw, key)
	if value == "" {
		return "", fmt.Errorf("%w: %s lacks %s", ErrProtocol, context, key)
	}
	return value, nil
}

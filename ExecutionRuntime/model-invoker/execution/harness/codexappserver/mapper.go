package codexappserver

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

type MappingContext struct {
	ExecutionID        union.ExecutionID
	Profile            union.VersionedIdentity
	Route              union.VersionedIdentity
	IntentID           union.IntentID
	MechanismPlanID    union.MechanismPlanID
	MechanismAttemptID union.MechanismAttemptID
	ApprovalTTL        time.Duration
	Clock              func() time.Time
}

type Mapper struct {
	context MappingContext
	mu      sync.Mutex
	seq     uint64
}

func NewMapper(context MappingContext) (*Mapper, error) {
	if context.ExecutionID == "" || context.Profile.Validate("profile") != nil || context.Route.Validate("route") != nil ||
		context.IntentID == "" || context.MechanismPlanID == "" || context.MechanismAttemptID == "" || context.ApprovalTTL <= 0 {
		return nil, ErrInvalidConfig
	}
	if context.Clock == nil {
		context.Clock = time.Now
	}
	return &Mapper{context: context}, nil
}

// Map projects one native app-server event without claiming any Effect.
// File-change and diff payloads remain Item evidence for an external observer.
func (m *Mapper) Map(native NativeEvent) (union.UnifiedExecutionEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	now := m.context.Clock()
	header := union.EventHeader{
		EventID: union.EventID(fmt.Sprintf("%s:codex:%d", m.context.ExecutionID, m.seq)), SemanticVersion: union.SemanticVersionV1,
		ExecutionID: m.context.ExecutionID, SessionID: union.SessionID(native.ThreadID), TurnID: union.TurnID(native.TurnID), ItemID: union.ItemID(native.ItemID),
		IntentID: m.context.IntentID, MechanismPlanID: m.context.MechanismPlanID, MechanismAttemptID: m.context.MechanismAttemptID,
		Sequence: m.seq, SourceSequence: m.seq, Timestamp: now, IngestedAt: now, Origin: union.EventOriginHarness,
		Visibility: union.VisibilityProgressOnly, SecurityClassification: union.SecurityInternal,
		ExecutionKind: union.ExecutionKindAgent, Profile: m.context.Profile, Route: m.context.Route,
		NativeIdentity: &union.NativeIdentity{Namespace: "openai.codex.app-server", Kind: "method", Value: native.Method},
	}
	event := union.UnifiedExecutionEvent{Header: header}
	switch native.Kind {
	case NativeThreadStarted, NativeTurnStarted:
		event.Header.Family = union.EventFamilyLifecycle
		event.Lifecycle = &union.LifecycleEvent{Kind: string(native.Kind)}
	case NativeTerminalCandidate:
		if native.Terminal == nil {
			return union.UnifiedExecutionEvent{}, fmt.Errorf("%w: terminal event lacks candidate", ErrProtocol)
		}
		payload, _ := json.Marshal(struct {
			Status          union.ExecutionStatus `json:"status"`
			StopReason      string                `json:"stop_reason,omitempty"`
			SideEffectState union.SideEffectState `json:"side_effect_state"`
		}{
			Status: native.Terminal.Status, StopReason: native.Terminal.StopReason,
			SideEffectState: native.Terminal.SideEffectState,
		})
		event.Header.Family = union.EventFamilyDiagnostic
		event.Header.Visibility = union.VisibilityAuditOnly
		event.Diagnostic = &union.DiagnosticEvent{Kind: "route_terminal_candidate", Code: native.Terminal.NativeStatus, Payload: payload}
	case NativeApprovalRequest:
		approvalID := nativeRequestID(native.RequestID)
		actionID := native.ItemID
		if actionID == "" {
			actionID = "request-" + approvalID
		}
		event.Header.Family = union.EventFamilyControl
		event.Header.ApprovalID = union.ApprovalID(approvalID)
		event.Header.ActionID = union.ActionID(actionID)
		event.Header.Visibility = union.VisibilityUserVisible
		event.Control = &union.ControlEvent{
			Kind: "approval_requested", ApprovalID: union.ApprovalID(approvalID), ActionID: union.ActionID(actionID),
			MechanismAttemptID: m.context.MechanismAttemptID, InputDigest: nativeInputDigest(native.Params), ActionRevision: 1,
			Scope: native.Method, Authority: "runtime", ExpiresAt: now.Add(m.context.ApprovalTTL), Payload: cloneRaw(native.Params),
		}
	case NativeItemUpdated:
		modelKind, disclosure, visible := codexModelDelta(native.Method)
		if modelKind != "" {
			event.Header.Family = union.EventFamilyModel
			event.Header.Visibility = visible
			event.Model = &union.ModelEvent{
				Kind:            modelKind,
				Content:         codexTextContent(native.Delta),
				DisclosureClass: disclosure,
				Payload:         cloneRaw(native.Params),
			}
			break
		}
		fallthrough
	case NativeItemStarted, NativeItemCompleted, NativeDynamicToolRequest, NativeProvisionalDiff:
		itemID := native.ItemID
		if itemID == "" {
			itemID = strings.ReplaceAll(string(native.Kind), "_", "-") + "-" + string(header.TurnID)
		}
		itemType := native.ItemType
		if itemType == "" {
			itemType = string(native.Kind)
		}
		status := mapItemStatus(native.Kind, native.ItemStatus)
		sideEffects := union.SideEffectNone
		if sideEffectItem(itemType) || native.Kind == NativeProvisionalDiff {
			sideEffects = union.SideEffectPossible
		}
		event.Header.Family = union.EventFamilyItem
		event.Header.ItemID = union.ItemID(itemID)
		event.Header.ActionID = union.ActionID(itemID)
		event.Item = &union.ItemEvent{
			Kind: string(native.Kind),
			Item: union.ExecutionItem{ID: union.ItemID(itemID), Kind: itemType, Status: status, ActionID: union.ActionID(itemID), AttemptID: m.context.MechanismAttemptID, SideEffectState: sideEffects, Payload: cloneRaw(native.Params)},
		}
		if native.Delta != "" {
			delta, _ := json.Marshal(map[string]string{"text": native.Delta})
			event.Item.Delta = delta
		}
		if native.Kind == NativeProvisionalDiff {
			event.Item.Item.Kind = "provisional_diff"
		}
	case NativeError:
		event.Header.Family = union.EventFamilyDiagnostic
		event.Header.Visibility = union.VisibilityAuditOnly
		event.Diagnostic = &union.DiagnosticEvent{Kind: "native_error", Code: native.Method, Payload: cloneRaw(native.Params)}
	case NativeExtension:
		event.Header.Family = union.EventFamilyDiagnostic
		event.Header.Visibility = union.VisibilityAuditOnly
		event.Diagnostic = &union.DiagnosticEvent{Kind: "native_extension", Code: native.Method, Payload: cloneRaw(native.Raw)}
	default:
		return union.UnifiedExecutionEvent{}, fmt.Errorf("%w: unknown native event kind %q", ErrProtocol, native.Kind)
	}
	if err := event.Validate(); err != nil {
		return union.UnifiedExecutionEvent{}, fmt.Errorf("%w: mapped event: %v", ErrProtocol, err)
	}
	return event, nil
}

func nativeInputDigest(raw json.RawMessage) string {
	digest := sha256.Sum256(raw)
	return fmt.Sprintf("sha256:%x", digest)
}

func codexModelDelta(method string) (string, string, union.Visibility) {
	switch method {
	case "item/agentMessage/delta":
		return "agent_message_delta", "provider_exposed_output", union.VisibilityUserVisible
	case "item/reasoning/summaryTextDelta", "item/reasoning/textDelta":
		return "reasoning_delta", "provider_exposed_reasoning", union.VisibilityAuditOnly
	default:
		return "", "", union.VisibilityProgressOnly
	}
}

func codexTextContent(value string) []union.ContentPart {
	if value == "" {
		return nil
	}
	return []union.ContentPart{{Kind: "text", Text: value}}
}

func mapItemStatus(kind NativeEventKind, native string) union.ItemStatus {
	if kind == NativeItemStarted || kind == NativeItemUpdated || kind == NativeProvisionalDiff || kind == NativeDynamicToolRequest {
		return union.ItemStatusInProgress
	}
	switch native {
	case "completed":
		return union.ItemStatusCompleted
	case "failed":
		return union.ItemStatusFailed
	case "declined":
		return union.ItemStatusCancelled
	case "inProgress":
		return union.ItemStatusInProgress
	default:
		if kind == NativeItemCompleted {
			return union.ItemStatusIndeterminate
		}
		return union.ItemStatusPending
	}
}

func nativeRequestID(raw json.RawMessage) string {
	value := strings.TrimSpace(string(raw))
	return strings.Trim(value, `"`)
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), raw...)
}

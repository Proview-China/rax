package acp

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

// Map projects one ACP event into the union stream. Tool updates, including
// diffs reported by the Agent, are execution evidence rather than observed
// Effects; only an external observer may emit an Effect.
func (m *Mapper) Map(native NativeEvent) (union.UnifiedExecutionEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.seq++
	now := m.context.Clock()
	header := union.EventHeader{
		EventID:                union.EventID(fmt.Sprintf("%s:acp:%d", m.context.ExecutionID, m.seq)),
		SemanticVersion:        union.SemanticVersionV1,
		ExecutionID:            m.context.ExecutionID,
		SessionID:              union.SessionID(native.SessionID),
		ItemID:                 union.ItemID(native.ToolCallID),
		IntentID:               m.context.IntentID,
		MechanismPlanID:        m.context.MechanismPlanID,
		MechanismAttemptID:     m.context.MechanismAttemptID,
		Sequence:               m.seq,
		SourceSequence:         m.seq,
		Timestamp:              now,
		IngestedAt:             now,
		Origin:                 union.EventOriginHarness,
		Visibility:             union.VisibilityProgressOnly,
		SecurityClassification: union.SecurityInternal,
		ExecutionKind:          union.ExecutionKindAgent,
		Profile:                m.context.Profile,
		Route:                  m.context.Route,
		NativeIdentity:         &union.NativeIdentity{Namespace: "agentclientprotocol", Kind: "method", Value: native.Method},
	}
	event := union.UnifiedExecutionEvent{Header: header}

	switch native.Kind {
	case NativeAgentMessageChunk:
		event.Header.Family = union.EventFamilyModel
		event.Header.Visibility = union.VisibilityUserVisible
		event.Model = &union.ModelEvent{
			Kind:            string(native.Kind),
			Content:         textContent(native.Text),
			DisclosureClass: "provider_exposed_output",
			Payload:         cloneRaw(native.Params),
		}
	case NativeAgentThoughtChunk:
		event.Header.Family = union.EventFamilyModel
		event.Header.Visibility = union.VisibilityAuditOnly
		event.Model = &union.ModelEvent{
			Kind:            string(native.Kind),
			Content:         textContent(native.Text),
			DisclosureClass: "provider_exposed_reasoning",
			Payload:         cloneRaw(native.Params),
		}
	case NativeToolCall, NativeToolCallUpdate, NativePlanUpdate:
		itemID := native.ToolCallID
		if itemID == "" {
			itemID = strings.ReplaceAll(string(native.Kind), "_", "-") + "-" + native.SessionID
		}
		itemKind := native.ToolKind
		if itemKind == "" {
			itemKind = string(native.Kind)
		}
		state := union.SideEffectNone
		if native.Kind == NativeToolCall || native.Kind == NativeToolCallUpdate {
			state = union.SideEffectPossible
		}
		event.Header.Family = union.EventFamilyItem
		event.Header.ItemID = union.ItemID(itemID)
		event.Header.ActionID = union.ActionID(itemID)
		event.Item = &union.ItemEvent{
			Kind: string(native.Kind),
			Item: union.ExecutionItem{
				ID:              union.ItemID(itemID),
				Kind:            itemKind,
				Status:          mapToolStatus(native.Kind, native.ToolStatus),
				ActionID:        union.ActionID(itemID),
				AttemptID:       m.context.MechanismAttemptID,
				SideEffectState: state,
				Payload:         cloneRaw(native.Params),
			},
		}
		if native.Text != "" {
			event.Item.Delta, _ = json.Marshal(map[string]string{"text": native.Text})
		}
	case NativeApprovalRequest:
		approvalID := nativeRequestID(native.RequestID)
		actionID := native.ToolCallID
		if actionID == "" {
			actionID = "request-" + approvalID
		}
		event.Header.Family = union.EventFamilyControl
		event.Header.Visibility = union.VisibilityUserVisible
		event.Header.ApprovalID = union.ApprovalID(approvalID)
		event.Header.ActionID = union.ActionID(actionID)
		event.Control = &union.ControlEvent{
			Kind:               "approval_requested",
			ApprovalID:         union.ApprovalID(approvalID),
			ActionID:           union.ActionID(actionID),
			MechanismAttemptID: m.context.MechanismAttemptID,
			InputDigest:        acpNativeInputDigest(native.Params),
			ActionRevision:     1,
			Scope:              native.Method,
			Authority:          "runtime",
			ExpiresAt:          now.Add(m.context.ApprovalTTL),
			Payload:            cloneRaw(native.Params),
		}
	case NativeTerminalCandidate:
		if native.Terminal == nil {
			return union.UnifiedExecutionEvent{}, fmt.Errorf("%w: terminal event lacks candidate", ErrProtocol)
		}
		payload, _ := json.Marshal(struct {
			Status          union.ExecutionStatus `json:"status"`
			StopReason      string                `json:"stop_reason"`
			SideEffectState union.SideEffectState `json:"side_effect_state"`
		}{
			Status:          native.Terminal.Status,
			StopReason:      native.Terminal.StopReason,
			SideEffectState: native.Terminal.SideEffectState,
		})
		event.Header.Family = union.EventFamilyDiagnostic
		event.Header.Visibility = union.VisibilityAuditOnly
		event.Diagnostic = &union.DiagnosticEvent{Kind: "route_terminal_candidate", Code: native.Terminal.StopReason, Payload: payload}
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

func acpNativeInputDigest(raw json.RawMessage) string {
	digest := sha256.Sum256(raw)
	return fmt.Sprintf("sha256:%x", digest)
}

func mapToolStatus(kind NativeEventKind, native string) union.ItemStatus {
	switch native {
	case "pending":
		return union.ItemStatusPending
	case "in_progress":
		return union.ItemStatusInProgress
	case "completed":
		return union.ItemStatusCompleted
	case "failed":
		return union.ItemStatusFailed
	case "cancelled":
		return union.ItemStatusCancelled
	}
	if kind == NativeToolCallUpdate {
		return union.ItemStatusIndeterminate
	}
	return union.ItemStatusInProgress
}

func textContent(value string) []union.ContentPart {
	if value == "" {
		return nil
	}
	return []union.ContentPart{{Kind: "text", Text: value}}
}

func nativeRequestID(raw json.RawMessage) string {
	value := strings.TrimSpace(string(raw))
	return strings.Trim(value, `"`)
}

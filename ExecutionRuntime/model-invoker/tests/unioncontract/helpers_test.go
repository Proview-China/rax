package unioncontract_test

import (
	"encoding/json"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

var contractTime = time.Date(2026, 7, 13, 2, 0, 0, 0, time.UTC)

func identity(id string) union.VersionedIdentity {
	return union.VersionedIdentity{ID: id, Version: "v1"}
}

func validIntentGraph() union.IntentGraph {
	return union.IntentGraph{Nodes: []union.IntentNode{{
		ID:               "intent-1",
		Kind:             union.IntentMoveFile,
		Target:           "/workspace/source.txt",
		Specification:    json.RawMessage(`{"destination":"/workspace/destination.txt"}`),
		Postconditions:   []union.Condition{{Kind: "path_exists", Target: "/workspace/destination.txt"}},
		Required:         true,
		Idempotency:      "safe_with_precondition",
		AcceptedFidelity: []union.SemanticFidelity{union.SemanticFidelityExact},
		Metadata:         map[string]string{"fixture": "union-contract"},
	}}}
}

func validRequest() union.UnifiedExecutionRequest {
	profile := identity("profile-contract")
	return union.UnifiedExecutionRequest{
		SemanticVersion: union.SemanticVersionV1,
		ExecutionID:     "execution-1",
		ProfileSelector: union.ProfileSelector{Exact: &profile},
		ExecutionKind:   union.ExecutionKindModel,
		Input: []union.InputItem{{
			ID:   "input-1",
			Kind: "message",
			Role: "user",
			Content: []union.ContentPart{{
				Kind:     "text",
				Text:     "move the file",
				Metadata: map[string]string{"language": "en"},
			}},
			Payload: json.RawMessage(`{"priority":"normal"}`),
		}},
		Instructions: []union.Instruction{{
			ID:             "instruction-1",
			Authority:      "task",
			Scope:          "execution",
			ConflictPolicy: "higher_authority_wins",
			Content:        []union.ContentPart{{Kind: "text", Text: "preserve content"}},
		}},
		Tools: []union.ToolDefinition{{
			ID:             "tool-1",
			Name:           "move_file",
			Kind:           "function",
			InputSchema:    json.RawMessage(`{"type":"object"}`),
			ExecutionOwner: union.ExecutionOwnerPraxis,
		}},
		ToolPolicy: union.ToolPolicy{
			AllowedToolIDs:  []string{"tool-1"},
			DefaultApproval: "on_side_effect",
			Parallelism:     1,
		},
		OutputContract: union.OutputContract{AcceptedContentKinds: []string{"text"}},
		SessionIntent:  union.SessionIntent{Mode: "new", SessionID: "session-1", TurnID: "turn-1"},
		ExecutionPolicy: union.ExecutionPolicy{
			Stream:         true,
			Sandbox:        "workspace_write",
			MaxConcurrency: 1,
		},
		Budget: union.Budget{MaxSteps: 8, MaxToolActions: 4, MaxWallTime: time.Minute},
		DegradationPolicy: union.DegradationPolicy{
			Default:           union.DegradationDefaultReject,
			AllowedFidelities: []union.SemanticFidelity{union.SemanticFidelityExact},
		},
		IntentGraph: validIntentGraph(),
		Metadata:    map[string]string{"trace": "contract"},
		Extensions:  map[string]json.RawMessage{"fixture": json.RawMessage(`{"enabled":true}`)},
	}
}

func validManifest(id string) union.ContextManifestSummary {
	return union.ContextManifestSummary{
		ID:      id,
		Version: "v1",
		Mode:    "test",
		Components: []union.ManifestComponent{{
			Kind: "runtime", Name: "fixture", Version: "1", State: "ready",
			Owner: union.ExecutionOwnerPraxis, ModelVisible: true, Executable: true,
		}},
		Tools: union.ToolSurfaceManifest{Entries: []union.ToolSurfaceEntry{{
			ID:             "tool-1",
			NativeName:     "move_file",
			Discovered:     true,
			Registered:     true,
			ModelVisible:   true,
			Executable:     true,
			PermissionMode: "approval_required",
			Owner:          union.ExecutionOwnerPraxis,
			Probe: union.ToolSurfaceProbe{
				Status:         union.ToolProbeObserved,
				EvidenceDigest: "sha256:probe",
				ObservedAt:     contractTime,
			},
		}}},
	}
}

func validMechanismPlan() union.MechanismPlan {
	return union.MechanismPlan{
		ID:                 "mechanism-plan-1",
		IntentID:           "intent-1",
		Kind:               "caller_move_file",
		Origin:             union.CapabilityOriginCallerHosted,
		Owner:              union.ExecutionOwnerPraxis,
		SelectionAuthority: union.SelectionAuthorityRuntime,
		CapabilityRef:      "tool-1",
		ExpectedEffects:    []string{"file_moved"},
		SemanticFidelity:   union.SemanticFidelityExact,
	}
}

func validPlan() union.PreparedExecutionPlan {
	return union.PreparedExecutionPlan{
		SemanticVersion:  union.SemanticVersionV1,
		ExecutionID:      "execution-1",
		Profile:          identity("profile-contract"),
		Route:            identity("route-contract"),
		ProfileKeyDigest: "sha256:profile-key",
		ExecutionKind:    union.ExecutionKindModel,
		IntentGraph:      validIntentGraph(),
		Mechanisms:       []union.MechanismPlan{validMechanismPlan()},
		ExpectedManifest: validManifest("expected-manifest"),
		MappingReport: union.MappingReport{Decisions: []union.MappingDecision{{
			Path: "intent_graph.nodes[0]", Fidelity: union.SemanticFidelityExact,
			Origin: union.CapabilityOriginCallerHosted,
		}}},
		RouteFingerprint: "sha256:route",
		Metadata:         map[string]string{"fixture": "plan"},
	}
}

func presentFile(path, hash string) *union.FileStateSnapshot {
	return &union.FileStateSnapshot{
		Path: path, Exists: true, Type: union.FileStateRegular,
		Hash: hash, Size: 12, Mode: 0o644, ModifiedAt: contractTime,
	}
}

func absentFile(path string) *union.FileStateSnapshot {
	return &union.FileStateSnapshot{Path: path, Exists: false, Type: union.FileStateAbsent}
}

func validMoveChange() union.WorkspaceChange {
	return union.WorkspaceChange{
		Kind:              "file_moved",
		Path:              "/workspace/source.txt",
		Destination:       "/workspace/destination.txt",
		Before:            presentFile("/workspace/source.txt", "sha256:before"),
		After:             absentFile("/workspace/source.txt"),
		DestinationBefore: absentFile("/workspace/destination.txt"),
		DestinationAfter:  presentFile("/workspace/destination.txt", "sha256:before"),
		UnifiedDiff:       "rename from source.txt\nrename to destination.txt\n",
	}
}

func validEffect() union.EffectRecord {
	change := validMoveChange()
	return union.EffectRecord{
		ID:                 "effect-1",
		IntentIDs:          []union.IntentID{"intent-1"},
		MechanismAttemptID: "mechanism-attempt-1",
		Kind:               "file_moved",
		Target:             "/workspace/source.txt",
		Payload:            union.EffectPayload{WorkspaceChange: &change},
		ObservationSource:  "workspace_observer",
		VerificationStatus: union.VerificationVerified,
		VerificationRefs:   []union.VerificationID{"verification-1"},
		Confidence:         "observed",
		OccurredAt:         contractTime.Add(time.Second),
	}
}

func validVerification() union.VerificationRecord {
	return union.VerificationRecord{
		ID:          "verification-1",
		EffectIDs:   []union.EffectID{"effect-1"},
		IntentIDs:   []union.IntentID{"intent-1"},
		Kind:        "workspace_state",
		Status:      union.VerificationVerified,
		Verifier:    identity("workspace-verifier"),
		CompletedAt: contractTime.Add(2 * time.Second),
	}
}

func validAttempt() union.MechanismAttempt {
	return union.MechanismAttempt{
		ID:              "mechanism-attempt-1",
		MechanismPlanID: "mechanism-plan-1",
		Authoritative:   true,
		ActualKind:      "caller_move_file",
		ActualOrigin:    union.CapabilityOriginCallerHosted,
		ActualOwner:     union.ExecutionOwnerPraxis,
		StartedAt:       contractTime,
		EndedAt:         contractTime.Add(time.Second),
		Status:          union.AttemptStatusCompleted,
		SideEffectState: union.SideEffectReconciled,
		SanitizedInput:  json.RawMessage(`{"source":"source.txt","destination":"destination.txt"}`),
	}
}

func validHeader(family union.EventFamily) union.EventHeader {
	return union.EventHeader{
		EventID:                "event-1",
		SemanticVersion:        union.SemanticVersionV1,
		ExecutionID:            "execution-1",
		SessionID:              "session-1",
		TurnID:                 "turn-1",
		Sequence:               1,
		Timestamp:              contractTime,
		IngestedAt:             contractTime.Add(time.Millisecond),
		CorrelationID:          "correlation-1",
		Origin:                 union.EventOriginPraxis,
		Family:                 family,
		Visibility:             union.VisibilityAuditOnly,
		SecurityClassification: union.SecurityInternal,
		ExecutionKind:          union.ExecutionKindModel,
		Profile:                identity("profile-contract"),
		Route:                  identity("route-contract"),
	}
}

func validEffectEvent() union.UnifiedExecutionEvent {
	effect := validEffect()
	header := validHeader(union.EventFamilyEffect)
	header.IntentID = "intent-1"
	header.MechanismPlanID = "mechanism-plan-1"
	header.MechanismAttemptID = effect.MechanismAttemptID
	header.EffectID = effect.ID
	return union.UnifiedExecutionEvent{
		Header: header,
		Effect: &union.EffectEvent{Kind: "effect_observed", Effect: &effect},
	}
}

func validApprovalCommand() union.ExecutionCommand {
	return union.ExecutionCommand{
		SemanticVersion:         union.SemanticVersionV1,
		ExecutionID:             "execution-1",
		SessionID:               "session-1",
		TurnID:                  "turn-1",
		Kind:                    union.CommandApproveAction,
		ExpectedExecutionStatus: "awaiting_approval",
		IdempotencyKey:          "approval-command-1",
		ApprovalID:              "approval-1",
		ActionID:                "action-1",
		MechanismAttemptID:      "mechanism-attempt-1",
		InputDigest:             "sha256:input",
		ActionRevision:          1,
		Payload:                 json.RawMessage(`{"decision":"approve"}`),
	}
}

func validResult() union.UnifiedExecutionResult {
	effect := validEffect()
	return union.UnifiedExecutionResult{
		SemanticVersion:    union.SemanticVersionV1,
		ExecutionID:        "execution-1",
		SessionID:          "session-1",
		TurnID:             "turn-1",
		TerminalEventID:    "event-terminal",
		Status:             union.ExecutionStatusSucceeded,
		VerificationStatus: union.VerificationVerified,
		StopReason:         "verified",
		IntentSatisfaction: []union.IntentSatisfaction{{
			IntentID: "intent-1", Status: union.IntentSatisfied, EffectIDs: []union.EffectID{"effect-1"},
		}},
		MechanismTrace:   []union.MechanismAttempt{validAttempt()},
		Effects:          []union.EffectRecord{effect},
		Verifications:    []union.VerificationRecord{validVerification()},
		FinalContent:     []union.ContentPart{{Kind: "text", Text: "moved"}},
		Actions:          []union.ExecutionItem{{ID: "item-1", Kind: "tool_call", Status: union.ItemStatusCompleted, ActionID: "action-1", AttemptID: "mechanism-attempt-1", SideEffectState: union.SideEffectReconciled}},
		WorkspaceChanges: []union.WorkspaceChange{validMoveChange()},
		UsageMetrics:     []union.UsageMetric{{Kind: "steps", Value: 1, Unit: "count", Scope: "execution", Source: "runtime", Quality: "observed"}},
		MappingReport: union.MappingReport{Decisions: []union.MappingDecision{{
			Path: "intent_graph.nodes[0]", Fidelity: union.SemanticFidelityExact,
			Origin: union.CapabilityOriginCallerHosted,
		}}},
		ContextManifest: validManifest("actual-manifest"),
	}
}

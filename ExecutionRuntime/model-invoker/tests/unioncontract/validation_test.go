package unioncontract_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func requireValid(t *testing.T, name string, validate func() error) {
	t.Helper()
	if err := validate(); err != nil {
		t.Fatalf("%s should be valid: %v", name, err)
	}
}

func requireInvalid(t *testing.T, name, contains string, validate func() error) {
	t.Helper()
	err := validate()
	if err == nil {
		t.Fatalf("%s should be invalid", name)
	}
	if contains != "" && !strings.Contains(err.Error(), contains) {
		t.Fatalf("%s error = %q, want substring %q", name, err, contains)
	}
}

func TestTopLevelContractsAcceptCompleteValues(t *testing.T) {
	request := validRequest()
	plan := validPlan()
	event := validEffectEvent()
	command := validApprovalCommand()
	result := validResult()

	requireValid(t, "request", request.Validate)
	requireValid(t, "plan", plan.Validate)
	requireValid(t, "event", event.Validate)
	requireValid(t, "command", command.Validate)
	requireValid(t, "result", result.Validate)
}

func TestTopLevelContractsRejectBrokenRequiredInvariants(t *testing.T) {
	t.Run("request semantic version", func(t *testing.T) {
		value := validRequest()
		value.SemanticVersion = "v0"
		requireInvalid(t, "request", "semantic_version", value.Validate)
	})

	t.Run("plan covers every intent", func(t *testing.T) {
		value := validPlan()
		value.Mechanisms = nil
		requireInvalid(t, "plan", "at least one mechanism for every intent", value.Validate)
	})

	t.Run("tool names are unique", func(t *testing.T) {
		value := validRequest()
		duplicate := value.Tools[0]
		duplicate.ID = "tool-2"
		value.Tools = append(value.Tools, duplicate)
		requireInvalid(t, "request", "tools.name", value.Validate)
	})

	t.Run("event family matches payload", func(t *testing.T) {
		value := validEffectEvent()
		value.Header.Family = union.EventFamilyModel
		requireInvalid(t, "event", "must match the tagged event payload", value.Validate)
	})

	t.Run("approval revision is mandatory", func(t *testing.T) {
		value := validApprovalCommand()
		value.ActionRevision = 0
		requireInvalid(t, "command", "action_revision", value.Validate)
	})

	t.Run("verification completion is observable", func(t *testing.T) {
		value := validResult()
		value.Verifications[0].CompletedAt = time.Time{}
		requireInvalid(t, "result", "verification.completed_at", value.Validate)
	})

	t.Run("verification must be associated", func(t *testing.T) {
		value := validVerification()
		value.EffectIDs = nil
		value.IntentIDs = nil
		requireInvalid(t, "verification", "at least one Effect or intent", value.Validate)
	})

	t.Run("Effect attempt must be in result trace", func(t *testing.T) {
		value := validResult()
		value.Effects[0].MechanismAttemptID = "unknown-attempt"
		requireInvalid(t, "result", "result mechanism attempt", value.Validate)
	})

	t.Run("Effect supersession must be causal", func(t *testing.T) {
		value := validResult()
		later := validEffect()
		later.ID = "effect-2"
		later.VerificationRefs = nil
		later.OccurredAt = contractTime.Add(3 * time.Second)
		value.Effects[0].SupersedesEffectIDs = []union.EffectID{later.ID}
		value.Effects = append(value.Effects, later)
		requireInvalid(t, "result", "earlier result Effect", value.Validate)
	})

	t.Run("conclusive Effect needs verification reference", func(t *testing.T) {
		value := validEffect()
		value.VerificationRefs = nil
		requireInvalid(t, "Effect", "conclusive verification status", value.Validate)
	})

	t.Run("succeeded result cannot contain unsatisfied intent", func(t *testing.T) {
		value := validResult()
		value.IntentSatisfaction[0].Status = union.IntentUnsatisfied
		value.IntentSatisfaction[0].EffectIDs = nil
		requireInvalid(t, "result", "satisfied when execution succeeded", value.Validate)
	})
}

func TestIntentGraphRejectsCycles(t *testing.T) {
	graph := union.IntentGraph{Nodes: []union.IntentNode{
		{ID: "a", Kind: union.IntentModifyFile, Target: "a.txt", Required: true, DependsOn: []union.IntentID{"c"}},
		{ID: "b", Kind: union.IntentModifyFile, Target: "b.txt", Required: true, DependsOn: []union.IntentID{"a"}},
		{ID: "c", Kind: union.IntentModifyFile, Target: "c.txt", Required: true, DependsOn: []union.IntentID{"b"}},
	}}
	requireInvalid(t, "cyclic graph", "acyclic", graph.Validate)

	graph.Nodes[0].DependsOn = nil
	requireValid(t, "acyclic graph", graph.Validate)
}

func TestPreparedPlanFallbackGraphIsSameIntentExistingUniqueAndAcyclic(t *testing.T) {
	t.Run("valid same-intent DAG", func(t *testing.T) {
		value := validPlan()
		fallback := validMechanismPlan()
		fallback.ID = "mechanism-plan-2"
		fallback.PreferredRank = 2
		value.Mechanisms[0].FallbackPlanIDs = []union.MechanismPlanID{fallback.ID}
		value.Mechanisms = append(value.Mechanisms, fallback)
		requireValid(t, "same-intent fallback", value.Validate)
	})

	tests := []struct {
		name     string
		contains string
		mutate   func(*union.PreparedExecutionPlan)
	}{
		{
			name: "missing",
			mutate: func(value *union.PreparedExecutionPlan) {
				value.Mechanisms[0].FallbackPlanIDs = []union.MechanismPlanID{"missing-plan"}
			},
			contains: "existing mechanism",
		},
		{
			name: "duplicate",
			mutate: func(value *union.PreparedExecutionPlan) {
				value.Mechanisms[0].FallbackPlanIDs = []union.MechanismPlanID{"missing-plan", "missing-plan"}
			},
			contains: "duplicates",
		},
		{
			name: "self",
			mutate: func(value *union.PreparedExecutionPlan) {
				value.Mechanisms[0].FallbackPlanIDs = []union.MechanismPlanID{value.Mechanisms[0].ID}
			},
			contains: "same mechanism",
		},
		{
			name: "cross intent",
			mutate: func(value *union.PreparedExecutionPlan) {
				secondIntent := value.IntentGraph.Nodes[0]
				secondIntent.ID = "intent-2"
				secondIntent.Target = "/workspace/other.txt"
				value.IntentGraph.Nodes = append(value.IntentGraph.Nodes, secondIntent)
				fallback := validMechanismPlan()
				fallback.ID = "mechanism-plan-2"
				fallback.IntentID = secondIntent.ID
				value.Mechanisms[0].FallbackPlanIDs = []union.MechanismPlanID{fallback.ID}
				value.Mechanisms = append(value.Mechanisms, fallback)
			},
			contains: "same intent",
		},
		{
			name: "cycle",
			mutate: func(value *union.PreparedExecutionPlan) {
				fallback := validMechanismPlan()
				fallback.ID = "mechanism-plan-2"
				value.Mechanisms[0].FallbackPlanIDs = []union.MechanismPlanID{fallback.ID}
				fallback.FallbackPlanIDs = []union.MechanismPlanID{value.Mechanisms[0].ID}
				value.Mechanisms = append(value.Mechanisms, fallback)
			},
			contains: "acyclic",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := validPlan()
			test.mutate(&value)
			requireInvalid(t, test.name, test.contains, value.Validate)
		})
	}
}

func TestCredentialLikeKeysAreRejectedFromStringAndExtensionMaps(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*union.UnifiedExecutionRequest)
	}{
		{
			name: "request metadata",
			mutate: func(value *union.UnifiedExecutionRequest) {
				value.Metadata[" API_KEY "] = "redacted"
			},
		},
		{
			name: "extensions",
			mutate: func(value *union.UnifiedExecutionRequest) {
				value.Extensions["refresh_token"] = json.RawMessage(`"redacted"`)
			},
		},
		{
			name: "profile constraints",
			mutate: func(value *union.UnifiedExecutionRequest) {
				value.ProfileSelector.Exact = nil
				value.ProfileSelector.Constraints = map[string]string{"Authorization": "redacted"}
			},
		},
		{
			name: "content metadata",
			mutate: func(value *union.UnifiedExecutionRequest) {
				value.Input[0].Content[0].Metadata["password"] = "redacted"
			},
		},
		{
			name: "intent metadata",
			mutate: func(value *union.UnifiedExecutionRequest) {
				value.IntentGraph.Nodes[0].Metadata["client_secret"] = "redacted"
			},
		},
		{
			name: "camel case provider api key",
			mutate: func(value *union.UnifiedExecutionRequest) {
				value.Metadata["openaiApiKey"] = "redacted"
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := validRequest()
			test.mutate(&value)
			requireInvalid(t, test.name, "credential material", value.Validate)
		})
	}

	plan := validPlan()
	plan.Metadata["oauth_token"] = "redacted"
	requireInvalid(t, "plan metadata", "credential material", plan.Validate)
}

func TestTaggedUnionsRejectZeroOrMultipleArms(t *testing.T) {
	t.Run("effect payload zero arms", func(t *testing.T) {
		payload := union.EffectPayload{}
		requireInvalid(t, "effect payload", "exactly one tagged payload", payload.Validate)
	})

	t.Run("effect payload multiple arms", func(t *testing.T) {
		change := validMoveChange()
		payload := union.EffectPayload{
			WorkspaceChange: &change,
			Extension:       json.RawMessage(`{"also":true}`),
		}
		requireInvalid(t, "effect payload", "exactly one tagged payload", payload.Validate)
	})

	t.Run("event zero arms", func(t *testing.T) {
		event := union.UnifiedExecutionEvent{Header: validHeader(union.EventFamilyEffect)}
		requireInvalid(t, "event", "exactly one tagged event payload", event.Validate)
	})

	t.Run("event multiple arms", func(t *testing.T) {
		event := validEffectEvent()
		event.Lifecycle = &union.LifecycleEvent{Kind: "started"}
		requireInvalid(t, "event", "exactly one tagged event payload", event.Validate)
	})

	t.Run("effect event multiple arms", func(t *testing.T) {
		event := validEffectEvent()
		verification := validVerification()
		event.Header.VerificationID = verification.ID
		event.Effect.Verification = &verification
		requireInvalid(t, "effect event", "exactly one tagged value", event.Validate)
	})

	t.Run("mechanism event multiple arms", func(t *testing.T) {
		plan := validMechanismPlan()
		attempt := validAttempt()
		header := validHeader(union.EventFamilyMechanism)
		header.IntentID = plan.IntentID
		header.MechanismPlanID = plan.ID
		header.MechanismAttemptID = attempt.ID
		event := union.UnifiedExecutionEvent{
			Header:    header,
			Mechanism: &union.MechanismEvent{Kind: "invalid_combination", Plan: &plan, Attempt: &attempt},
		}
		requireInvalid(t, "mechanism event", "multiple tagged values", event.Validate)
	})
}

func TestModelResultRequiresCoherentAssociationsAndProvenance(t *testing.T) {
	base := validHeader(union.EventFamilyModel)
	base.ActionID = "action-1"
	base.ItemID = "item-1"
	executed := true
	valid := union.UnifiedExecutionEvent{Header: base, Model: &union.ModelEvent{
		Kind: "tool_result", ActionID: "action-1", ExecutionItemID: "item-1",
		Executed: &executed, ResultOrigin: union.EventOriginHarness,
	}}
	requireValid(t, "executed model result", valid.Validate)

	tests := []struct {
		name   string
		mutate func(*union.UnifiedExecutionEvent)
		field  string
	}{
		{
			name: "action identity mismatch",
			mutate: func(event *union.UnifiedExecutionEvent) {
				event.Header.ActionID = "other-action"
			},
			field: "header.action_id",
		},
		{
			name: "execution item identity mismatch",
			mutate: func(event *union.UnifiedExecutionEvent) {
				event.Header.ItemID = "other-item"
			},
			field: "header.item_id",
		},
		{
			name: "executed result without origin",
			mutate: func(event *union.UnifiedExecutionEvent) {
				event.Model.ResultOrigin = ""
			},
			field: "result_origin",
		},
		{
			name: "synthetic result without reason",
			mutate: func(event *union.UnifiedExecutionEvent) {
				value := false
				event.Model.Executed = &value
				event.Model.ResultOrigin = ""
			},
			field: "synthetic_reason",
		},
		{
			name: "synthetic result claims execution origin",
			mutate: func(event *union.UnifiedExecutionEvent) {
				value := false
				event.Model.Executed = &value
				event.Model.SyntheticReason = "protocol_pairing_only"
			},
			field: "result_origin",
		},
		{
			name: "result association without executed flag",
			mutate: func(event *union.UnifiedExecutionEvent) {
				event.Model.Executed = nil
				event.Model.ResultOrigin = ""
			},
			field: "model.executed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event, err := valid.Clone()
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(&event)
			requireInvalid(t, test.name, test.field, event.Validate)
		})
	}

	synthetic := valid
	wasExecuted := false
	synthetic.Model.Executed = &wasExecuted
	synthetic.Model.ResultOrigin = ""
	synthetic.Model.SyntheticReason = "protocol_pairing_only"
	requireValid(t, "synthetic model result", synthetic.Validate)
}

func TestWorkspaceMoveRequiresCoherentSourceAndDestinationSnapshots(t *testing.T) {
	valid := validMoveChange()
	requireValid(t, "move change", func() error {
		return union.EffectPayload{WorkspaceChange: &valid}.Validate()
	})

	tests := []struct {
		name   string
		mutate func(*union.WorkspaceChange)
		field  string
	}{
		{
			name: "missing destination after",
			mutate: func(change *union.WorkspaceChange) {
				change.DestinationAfter = nil
			},
			field: "both destination snapshots",
		},
		{
			name: "destination path mismatch",
			mutate: func(change *union.WorkspaceChange) {
				change.DestinationAfter.Path = "/workspace/other.txt"
			},
			field: "destination snapshot paths",
		},
		{
			name: "source path mismatch",
			mutate: func(change *union.WorkspaceChange) {
				change.Before.Path = "/workspace/other.txt"
			},
			field: "before.path",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			change := validMoveChange()
			test.mutate(&change)
			requireInvalid(t, test.name, test.field, func() error {
				return union.EffectPayload{WorkspaceChange: &change}.Validate()
			})
		})
	}
}

func TestEffectCapabilityFieldsAreValidated(t *testing.T) {
	structured := union.EffectPayload{StructuredOutput: &union.StructuredOutputEffect{
		Mechanism: union.StructuredStrictJSONSchema, Origin: union.CapabilityOriginNative,
		Fidelity: union.SemanticFidelityExact, Transport: "response_format",
		Parsed: json.RawMessage(`{"ok":true}`), SchemaDigest: "sha256:schema",
		JSONValid: true, SchemaValid: true, FinalDigest: "sha256:output",
	}}
	requireValid(t, "structured output", structured.Validate)

	missingMechanism := structured
	structuredCopy := *structured.StructuredOutput
	missingMechanism.StructuredOutput = &structuredCopy
	missingMechanism.StructuredOutput.Mechanism = ""
	requireInvalid(t, "structured mechanism", "mechanism", missingMechanism.Validate)

	exitCode := 0
	code := union.EffectPayload{CodeExecution: &union.CodeExecutionEffect{
		Mechanism: "caller_process", Origin: union.CapabilityOriginCallerHosted,
		Argv: []string{"go", "test", "./..."}, RuntimeIdentity: "go1.25", ExitCode: &exitCode,
	}}
	requireValid(t, "code execution", code.Validate)
	code.CodeExecution.Origin = "unknown"
	requireInvalid(t, "code execution origin", "origin", code.Validate)

	computer := union.EffectPayload{ComputerUse: &union.ComputerUseEffect{
		Mechanism: "provider_computer", Origin: union.CapabilityOriginProviderHosted,
		Action: "click", Target: "button#submit",
	}}
	requireValid(t, "computer use", computer.Validate)
	computer.ComputerUse.Mechanism = ""
	requireInvalid(t, "computer mechanism", "mechanism", computer.Validate)
}

func TestEventHeaderIdentityAssociations(t *testing.T) {
	tests := []struct {
		name   string
		valid  func() union.UnifiedExecutionEvent
		mutate func(*union.UnifiedExecutionEvent)
	}{
		{
			name: "mechanism plan",
			valid: func() union.UnifiedExecutionEvent {
				plan := validMechanismPlan()
				header := validHeader(union.EventFamilyMechanism)
				header.IntentID = plan.IntentID
				header.MechanismPlanID = plan.ID
				return union.UnifiedExecutionEvent{Header: header, Mechanism: &union.MechanismEvent{Kind: "planned", Plan: &plan}}
			},
			mutate: func(event *union.UnifiedExecutionEvent) { event.Header.IntentID = "other-intent" },
		},
		{
			name: "mechanism attempt",
			valid: func() union.UnifiedExecutionEvent {
				attempt := validAttempt()
				header := validHeader(union.EventFamilyMechanism)
				header.MechanismPlanID = attempt.MechanismPlanID
				header.MechanismAttemptID = attempt.ID
				return union.UnifiedExecutionEvent{Header: header, Mechanism: &union.MechanismEvent{Kind: "attempted", Attempt: &attempt}}
			},
			mutate: func(event *union.UnifiedExecutionEvent) { event.Header.MechanismAttemptID = "other-attempt" },
		},
		{
			name:  "effect",
			valid: validEffectEvent,
			mutate: func(event *union.UnifiedExecutionEvent) {
				event.Header.MechanismAttemptID = "other-attempt"
			},
		},
		{
			name: "verification",
			valid: func() union.UnifiedExecutionEvent {
				verification := validVerification()
				header := validHeader(union.EventFamilyEffect)
				header.VerificationID = verification.ID
				header.IntentID = verification.IntentIDs[0]
				header.EffectID = verification.EffectIDs[0]
				return union.UnifiedExecutionEvent{Header: header, Effect: &union.EffectEvent{Kind: "verified", Verification: &verification}}
			},
			mutate: func(event *union.UnifiedExecutionEvent) { event.Header.VerificationID = "other-verification" },
		},
		{
			name: "control",
			valid: func() union.UnifiedExecutionEvent {
				header := validHeader(union.EventFamilyControl)
				header.ApprovalID, header.ActionID, header.MechanismAttemptID = "approval-1", "action-1", "mechanism-attempt-1"
				return union.UnifiedExecutionEvent{Header: header, Control: &union.ControlEvent{
					Kind: "approval_requested", ApprovalID: "approval-1", ActionID: "action-1",
					MechanismAttemptID: "mechanism-attempt-1", InputDigest: "sha256:input", ActionRevision: 1,
					ExpiresAt: contractTime.Add(time.Hour),
				}}
			},
			mutate: func(event *union.UnifiedExecutionEvent) { event.Header.ActionID = "other-action" },
		},
		{
			name: "item",
			valid: func() union.UnifiedExecutionEvent {
				item := union.ExecutionItem{ID: "item-1", Kind: "message", Status: union.ItemStatusCompleted, SideEffectState: union.SideEffectNone}
				header := validHeader(union.EventFamilyItem)
				header.ItemID = item.ID
				return union.UnifiedExecutionEvent{Header: header, Item: &union.ItemEvent{Kind: "completed", Item: item}}
			},
			mutate: func(event *union.UnifiedExecutionEvent) { event.Header.ItemID = "other-item" },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			valid := test.valid()
			requireValid(t, test.name, valid.Validate)
			test.mutate(&valid)
			requireInvalid(t, test.name, "must match", valid.Validate)
		})
	}
}

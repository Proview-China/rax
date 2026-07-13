package conformance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/profile"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

// TestSixRoutesConvergeOnTheSameUnionSemantics is the cross-route integration
// contract. Route-specific suites under tests/executiondirect and
// tests/harnesslocal cover the real wire codecs. This test deliberately keeps
// provider contact out of scope: it sends every compiled plan through the same
// local Runtime boundary, then makes the Praxis-owned reconciler and verifier
// derive Effects from observed attempts rather than trusting adapter claims.
func TestSixRoutesConvergeOnTheSameUnionSemantics(t *testing.T) {
	profiles, err := profile.RepresentativeProfiles(semanticRouteTestTime)
	if err != nil {
		t.Fatalf("RepresentativeProfiles: %v", err)
	}
	profileRegistry, err := profile.NewRegistry(semanticRouteTestTime, profiles...)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	compiler, err := profile.NewCompiler(profileRegistry, semanticRouteTestTime)
	if err != nil {
		t.Fatalf("NewCompiler: %v", err)
	}

	profilesByID := make(map[profile.ProfileID]profile.SemanticRouteProfile, len(profiles))
	for _, candidate := range profiles {
		profilesByID[candidate.ID] = candidate
	}

	routes := semanticRouteCases()
	semanticOutcomes := make(map[profile.ProfileID]routeSemanticOutcome, len(routes))
	selectedCapabilities := make(map[union.IntentID]map[string]struct{}, 4)
	for _, route := range routes {
		route := route
		t.Run(string(route.profileID), func(t *testing.T) {
			selected, ok := profilesByID[route.profileID]
			if !ok {
				t.Fatalf("representative Profile %q is missing", route.profileID)
			}
			request := semanticRouteRequest(route.profileID)
			compiled, err := compiler.Compile(profile.CompileInput{
				Request:        request,
				ActualManifest: observedManifestFixture(selected.HarnessCapability.ExpectedManifest),
			})
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			if !compiled.ManifestEvaluation.Allowed || len(compiled.ManifestEvaluation.Differences) != 0 {
				t.Fatalf("offline observed manifest did not bind exactly: %#v", compiled.ManifestEvaluation)
			}
			if err := compiled.Plan.Validate(); err != nil {
				t.Fatalf("PreparedExecutionPlan.Validate: %v", err)
			}
			if compiled.Plan.ExecutionKind != route.executionKind {
				t.Fatalf("execution kind = %q, want %q", compiled.Plan.ExecutionKind, route.executionKind)
			}
			if gotHarness := len(compiled.Profile.Selection.HarnessStack) != 0; gotHarness != route.harnessed {
				t.Fatalf("harnessed = %v, want %v", gotHarness, route.harnessed)
			}

			primary := primaryMechanisms(compiled.Plan)
			for intentID, want := range route.capabilities {
				got, ok := primary[intentID]
				if !ok {
					t.Fatalf("primary mechanism for %q is missing", intentID)
				}
				if got.CapabilityRef != want {
					t.Errorf("%s capability = %q, want %q", intentID, got.CapabilityRef, want)
				}
				if selectedCapabilities[intentID] == nil {
					selectedCapabilities[intentID] = make(map[string]struct{})
				}
				selectedCapabilities[intentID][got.CapabilityRef] = struct{}{}
			}

			result := executeSemanticRoute(t, route, execution.Invocation{Request: request, Plan: compiled.Plan})
			if err := result.Validate(); err != nil {
				t.Fatalf("UnifiedExecutionResult.Validate: %v", err)
			}
			if result.Status != union.ExecutionStatusSucceeded || result.VerificationStatus != union.VerificationVerified {
				t.Fatalf("terminal status = %q/%q", result.Status, result.VerificationStatus)
			}
			assertRouteProvenance(t, result, primary)
			outcome := projectRouteSemanticOutcome(t, result)
			semanticOutcomes[route.profileID] = outcome
		})
	}

	baseline := semanticOutcomes[profile.ProfileOpenAIDirect]
	for _, route := range routes[1:] {
		if got := semanticOutcomes[route.profileID]; !reflect.DeepEqual(got, baseline) {
			t.Errorf("%s semantic outcome diverged\n got: %#v\nwant: %#v", route.profileID, got, baseline)
		}
	}
	for _, intentID := range []union.IntentID{"i1", "i2", "i3", "i4"} {
		if got := len(selectedCapabilities[intentID]); got != len(routes) {
			t.Errorf("%s compiled to %d distinct route mechanisms, want %d: %#v", intentID, got, len(routes), selectedCapabilities[intentID])
		}
	}
}

var semanticRouteTestTime = time.Date(2026, 7, 13, 3, 30, 0, 0, time.UTC)

type semanticRouteCase struct {
	profileID     profile.ProfileID
	adapterID     string
	origin        union.EventOrigin
	executionKind union.ExecutionKind
	harnessed     bool
	capabilities  map[union.IntentID]string
}

func semanticRouteCases() []semanticRouteCase {
	return []semanticRouteCase{
		{
			profileID: profile.ProfileOpenAIDirect, adapterID: "offline.openai.direct", origin: union.EventOriginProvider,
			executionKind: union.ExecutionKindModel,
			capabilities: map[union.IntentID]string{
				"i1": "openai.caller.apply_patch", "i2": "openai.caller.function",
				"i3": "openai.caller.process", "i4": "openai.responses.json_schema",
			},
		},
		{
			profileID: profile.ProfileCodex, adapterID: "offline.codex.app-server", origin: union.EventOriginHarness,
			executionKind: union.ExecutionKindAgent, harnessed: true,
			capabilities: map[union.IntentID]string{
				"i1": "codex.apply_patch", "i2": "codex.dynamic_tool",
				"i3": "codex.shell", "i4": "codex.output_schema",
			},
		},
		{
			profileID: profile.ProfileClaudeSDK, adapterID: "offline.claude.agent-sdk", origin: union.EventOriginHarness,
			executionKind: union.ExecutionKindAgent, harnessed: true,
			capabilities: map[union.IntentID]string{
				"i1": "claude.edit", "i2": "claude.tool_use",
				"i3": "claude.bash", "i4": "claude.output_format",
			},
		},
		{
			profileID: profile.ProfileGeminiCLI, adapterID: "offline.gemini.cli-acp", origin: union.EventOriginHarness,
			executionKind: union.ExecutionKindAgent, harnessed: true,
			capabilities: map[union.IntentID]string{
				"i1": "gemini.replace", "i2": "gemini.tool_call",
				"i3": "gemini.run_shell_command", "i4": "praxis.gemini.schema_repair",
			},
		},
		{
			profileID: profile.ProfileKimiCLI, adapterID: "offline.kimi.cli-acp", origin: union.EventOriginHarness,
			executionKind: union.ExecutionKindAgent, harnessed: true,
			capabilities: map[union.IntentID]string{
				"i1": "kimi.edit", "i2": "kimi.tool_call",
				"i3": "kimi.shell", "i4": "praxis.kimi.schema_repair",
			},
		},
		{
			profileID: profile.ProfileQwenSDK, adapterID: "offline.qwen.code-sdk", origin: union.EventOriginHarness,
			executionKind: union.ExecutionKindAgent, harnessed: true,
			capabilities: map[union.IntentID]string{
				"i1": "qwen.edit", "i2": "qwen.tool_call",
				"i3": "qwen.bash", "i4": "praxis.qwen.schema_repair",
			},
		},
	}
}

func semanticRouteRequest(profileID profile.ProfileID) union.UnifiedExecutionRequest {
	return union.UnifiedExecutionRequest{
		SemanticVersion: union.SemanticVersionV1,
		ExecutionID:     union.ExecutionID("exec.semantic." + string(profileID)),
		ProfileSelector: union.ProfileSelector{Exact: &union.VersionedIdentity{ID: string(profileID), Version: "v1candidate"}},
		ExecutionKind:   union.ExecutionKindAuto,
		Input: []union.InputItem{{
			ID: "input.semantic", Kind: "message", Role: "user",
			Content: []union.ContentPart{{Kind: "text", Text: "apply the canonical four-intent offline fixture"}},
		}},
		Tools: []union.ToolDefinition{{
			ID: "workspace.inspect", Name: "workspace.inspect", Kind: "function",
			InputSchema:    json.RawMessage(`{"type":"object","additionalProperties":false}`),
			OutputSchema:   json.RawMessage(`{"type":"object","required":["ok"],"properties":{"ok":{"const":true}},"additionalProperties":false}`),
			ExecutionOwner: union.ExecutionOwnerPraxis,
		}},
		ToolPolicy: union.ToolPolicy{
			AllowedToolIDs: []string{"workspace.inspect"}, DefaultApproval: "on_side_effect", Parallelism: 1, MaxActions: 8,
		},
		OutputContract: union.OutputContract{
			AcceptedContentKinds: []string{"json"}, CompletionMode: "final",
			JSONSchema: json.RawMessage(`{"type":"object","required":["ok"],"properties":{"ok":{"const":true}},"additionalProperties":false}`),
		},
		SessionIntent: union.SessionIntent{Mode: "new"},
		ExecutionPolicy: union.ExecutionPolicy{
			Sandbox: "workspace_write", CWDReference: "/workspace", NetworkPolicy: "denied",
			UserPresence: "present", Foreground: "required", InteractionMode: "interactive", MaxConcurrency: 1,
		},
		Budget:            union.Budget{MaxWallTime: 120 * time.Second, MaxToolActions: 8},
		DegradationPolicy: union.DegradationPolicy{Default: union.DegradationDefaultReject},
		IntentGraph: union.IntentGraph{Nodes: []union.IntentNode{
			{
				ID: "i1", Kind: union.IntentModifyFile, Target: "/workspace/internal/config/config.go", Required: true,
				Specification:    json.RawMessage(`{"before_hash":"sha256:before","replace":["legacy","strict"]}`),
				AcceptedFidelity: []union.SemanticFidelity{union.SemanticFidelityExact},
			},
			{
				ID: "i2", Kind: union.IntentCallTool, Target: "workspace.inspect", DependsOn: []union.IntentID{"i1"}, Required: true,
				Specification: json.RawMessage(`{"arguments":{}}`), AcceptedFidelity: []union.SemanticFidelity{union.SemanticFidelityExact},
			},
			{
				ID: "i3", Kind: union.IntentExecuteCode, Target: "./internal/config", DependsOn: []union.IntentID{"i1"}, Required: true,
				Specification:    json.RawMessage(`{"argv":["go","test","./internal/config"],"cwd":"/workspace","network":false,"timeout_ms":120000}`),
				AcceptedFidelity: []union.SemanticFidelity{union.SemanticFidelityExact},
			},
			{
				ID: "i4", Kind: union.IntentProduceStructured, Target: "summary", DependsOn: []union.IntentID{"i2", "i3"}, Required: true,
				Postconditions:   []union.Condition{{Kind: "json_schema_valid"}},
				AcceptedFidelity: []union.SemanticFidelity{union.SemanticFidelityExact, union.SemanticFidelityTransformed},
			},
		}},
	}
}

func observedManifestFixture(expected profile.InjectionManifest) profile.InjectionManifest {
	actual := expected.Clone()
	actual.ProbeStatus = profile.ManifestProbeObserved
	for index := range actual.Fields {
		actual.Fields[index].Evidence = profile.ManifestEvidence{
			Source: profile.ManifestEvidenceObserved, Confidence: 100,
			Reference: "fixture://semantic-route/" + actual.Fields[index].Path,
		}
	}
	return actual
}

func primaryMechanisms(plan union.PreparedExecutionPlan) map[union.IntentID]union.MechanismPlan {
	result := make(map[union.IntentID]union.MechanismPlan, len(plan.IntentGraph.Nodes))
	for _, mechanism := range plan.Mechanisms {
		current, exists := result[mechanism.IntentID]
		if !exists || mechanism.PreferredRank < current.PreferredRank ||
			(mechanism.PreferredRank == current.PreferredRank && mechanism.ID < current.ID) {
			result[mechanism.IntentID] = mechanism
		}
	}
	return result
}

func executeSemanticRoute(t *testing.T, route semanticRouteCase, invocation execution.Invocation) union.UnifiedExecutionResult {
	t.Helper()
	registry := execution.NewRegistry()
	adapter := semanticRouteAdapter{id: route.adapterID, origin: route.origin}
	if err := registry.Register(context.Background(), adapter); err != nil {
		t.Fatalf("Register adapter: %v", err)
	}
	var tick atomic.Int64
	runtime, err := execution.NewRuntime(execution.RuntimeConfig{
		Registry: registry, Reconciler: semanticRouteReconciler{}, Verifier: semanticRouteVerifier{},
		Clock: func() time.Time {
			return semanticRouteTestTime.Add(time.Duration(tick.Add(1)) * time.Millisecond)
		},
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	result, err := runtime.Execute(context.Background(), route.adapterID, invocation)
	if err != nil {
		t.Fatalf("Runtime.Execute: %v", err)
	}
	return result
}

type semanticRouteAdapter struct {
	id     string
	origin union.EventOrigin
}

func (adapter semanticRouteAdapter) Describe(context.Context) (execution.AdapterDescriptor, error) {
	return execution.AdapterDescriptor{
		Identity: union.VersionedIdentity{ID: adapter.id, Version: "v1"}, Origin: adapter.origin,
		ExecutionKinds: []union.ExecutionKind{union.ExecutionKindModel, union.ExecutionKindAgent},
	}, nil
}

func (semanticRouteAdapter) Preflight(_ context.Context, invocation execution.Invocation) (execution.PreflightReport, error) {
	actual, err := invocation.Plan.ExpectedManifest.Clone()
	if err != nil {
		return execution.PreflightReport{}, err
	}
	return execution.PreflightReport{Accepted: true, ActualManifest: actual}, nil
}

func (adapter semanticRouteAdapter) Open(_ context.Context, invocation execution.Invocation) (execution.Session, error) {
	primary := primaryMechanisms(invocation.Plan)
	intents := append([]union.IntentNode(nil), invocation.Plan.IntentGraph.Nodes...)
	sort.Slice(intents, func(i, j int) bool { return intents[i].ID < intents[j].ID })
	events := make([]union.UnifiedExecutionEvent, 0, len(intents)+1)
	for index, intent := range intents {
		plan, ok := primary[intent.ID]
		if !ok {
			return nil, fmt.Errorf("primary mechanism for %s is missing", intent.ID)
		}
		attemptID := union.MechanismAttemptID("attempt." + string(intent.ID))
		header := execution.CandidateHeader(adapter.origin, union.EventFamilyMechanism)
		header.Sequence = uint64(index + 1)
		header.Timestamp = semanticRouteTestTime.Add(time.Duration(index+1) * time.Second)
		header.IntentID, header.MechanismPlanID, header.MechanismAttemptID = intent.ID, plan.ID, attemptID
		events = append(events, union.UnifiedExecutionEvent{
			Header: header,
			Mechanism: &union.MechanismEvent{Kind: "attempt_completed", Attempt: &union.MechanismAttempt{
				ID: attemptID, MechanismPlanID: plan.ID, Authoritative: true,
				ActualKind: plan.Kind, ActualOrigin: plan.Origin, ActualOwner: plan.Owner,
				StartedAt: semanticRouteTestTime, EndedAt: semanticRouteTestTime.Add(time.Second),
				Status: union.AttemptStatusCompleted, SideEffectState: union.SideEffectObserved,
			}},
		})
	}
	header := execution.CandidateHeader(adapter.origin, union.EventFamilyLifecycle)
	header.Sequence = uint64(len(events) + 1)
	header.Timestamp = semanticRouteTestTime.Add(time.Duration(len(events)+1) * time.Second)
	events = append(events, union.UnifiedExecutionEvent{
		Header: header,
		Lifecycle: &union.LifecycleEvent{
			Kind: "route_terminal", Status: union.ExecutionStatusSucceeded, StopReason: "offline_semantic_fixture_complete",
		},
	})
	return &semanticRouteSession{events: events}, nil
}

type semanticRouteSession struct {
	events []union.UnifiedExecutionEvent
	next   int
}

func (session *semanticRouteSession) Receive(ctx context.Context) (union.UnifiedExecutionEvent, error) {
	if err := ctx.Err(); err != nil {
		return union.UnifiedExecutionEvent{}, err
	}
	if session.next >= len(session.events) {
		return union.UnifiedExecutionEvent{}, io.EOF
	}
	event := session.events[session.next]
	session.next++
	return event, nil
}

func (*semanticRouteSession) Command(context.Context, union.ExecutionCommand) error { return nil }
func (*semanticRouteSession) Close() error                                          { return nil }

type semanticRouteReconciler struct{}

func (semanticRouteReconciler) Reconcile(_ context.Context, input execution.ReconcileInput) (execution.ReconcileReport, error) {
	attemptByPlan := make(map[union.MechanismPlanID]union.MechanismAttempt)
	for _, event := range input.Events {
		if event.Mechanism != nil && event.Mechanism.Attempt != nil {
			attemptByPlan[event.Mechanism.Attempt.MechanismPlanID] = *event.Mechanism.Attempt
		}
	}
	primary := primaryMechanisms(input.Invocation.Plan)
	effects := make([]union.EffectRecord, 0, len(input.Invocation.Plan.IntentGraph.Nodes))
	for _, intent := range input.Invocation.Plan.IntentGraph.Nodes {
		plan, exists := primary[intent.ID]
		if !exists {
			return execution.ReconcileReport{}, fmt.Errorf("primary mechanism for %s is missing", intent.ID)
		}
		attempt, exists := attemptByPlan[plan.ID]
		if !exists || attempt.Status != union.AttemptStatusCompleted {
			return execution.ReconcileReport{}, fmt.Errorf("completed attempt for %s is missing", plan.ID)
		}
		effectRecord, err := semanticEffectFixture(intent, plan, attempt.ID)
		if err != nil {
			return execution.ReconcileReport{}, err
		}
		effects = append(effects, effectRecord)
	}
	return execution.ReconcileReport{Effects: effects, SideEffectState: union.SideEffectObserved, Quiesced: true}, nil
}

func semanticEffectFixture(intent union.IntentNode, mechanism union.MechanismPlan, attemptID union.MechanismAttemptID) (union.EffectRecord, error) {
	observed := union.EffectRecord{
		ID: union.EffectID("effect." + string(intent.ID)), IntentIDs: []union.IntentID{intent.ID}, MechanismAttemptID: attemptID,
		Target: intent.Target, ObservationSource: "offline.semantic.observer",
		VerificationStatus: union.VerificationUnverified, OccurredAt: semanticRouteTestTime.Add(10 * time.Second),
	}
	switch intent.Kind {
	case union.IntentModifyFile:
		observed.Kind = "file_changed"
		observed.Payload.WorkspaceChange = &union.WorkspaceChange{
			Kind: "file_changed", Path: intent.Target,
			Before:      &union.FileStateSnapshot{Path: intent.Target, Exists: true, Type: union.FileStateRegular, Hash: "sha256:before", Size: 7, Mode: 0o644},
			After:       &union.FileStateSnapshot{Path: intent.Target, Exists: true, Type: union.FileStateRegular, Hash: "sha256:after", Size: 7, Mode: 0o644},
			UnifiedDiff: "--- a/config.go\n+++ b/config.go\n@@ -1 +1 @@\n-legacy\n+strict\n",
		}
	case union.IntentCallTool:
		observed.Kind = "tool_call_completed"
		observed.Payload.ToolCall = &union.ToolCallEffect{
			ToolID: intent.Target, ActionID: union.ActionID("action." + string(intent.ID)),
			Mechanism: mechanism.CapabilityRef, Origin: mechanism.Origin, Owner: mechanism.Owner, Executed: true,
			InputDigest: "sha256:tool-input", OutputDigest: "sha256:tool-output",
			ResultOrigin: resultOrigin(mechanism.Owner), SideEffectState: union.SideEffectObserved,
		}
	case union.IntentExecuteCode:
		zero := 0
		observed.Kind = "code_execution_completed"
		observed.Payload.CodeExecution = &union.CodeExecutionEffect{
			Mechanism: mechanism.CapabilityRef, Origin: mechanism.Origin,
			Argv: []string{"go", "test", "./internal/config"}, RuntimeIdentity: "offline-fixture-go",
			ExitCode: &zero, StdoutRef: "sha256:test-output", StderrRef: "sha256:empty", Duration: time.Second,
		}
	case union.IntentProduceStructured:
		structuredMechanism := union.StructuredStrictJSONSchema
		repairAttempts := 0
		if mechanism.Origin == union.CapabilityOriginEmulated {
			structuredMechanism = union.StructuredEmulatedSchema
			repairAttempts = 1
		}
		observed.Kind = "structured_output_produced"
		observed.Payload.StructuredOutput = &union.StructuredOutputEffect{
			Mechanism: structuredMechanism, Origin: mechanism.Origin, Fidelity: mechanism.SemanticFidelity,
			Parsed: json.RawMessage(`{"ok":true}`), SchemaDigest: "sha256:semantic-schema",
			JSONValid: true, SchemaValid: true, RepairAttempts: repairAttempts, FinalDigest: "sha256:semantic-output",
		}
	default:
		return union.EffectRecord{}, fmt.Errorf("unsupported semantic fixture intent %q", intent.Kind)
	}
	return observed, nil
}

func resultOrigin(owner union.ExecutionOwner) union.EventOrigin {
	switch owner {
	case union.ExecutionOwnerPraxis:
		return union.EventOriginPraxis
	case union.ExecutionOwnerProvider:
		return union.EventOriginProvider
	case union.ExecutionOwnerHarness:
		return union.EventOriginHarness
	case union.ExecutionOwnerModel:
		return union.EventOriginModel
	default:
		return union.EventOriginExternal
	}
}

type semanticRouteVerifier struct{}

func (semanticRouteVerifier) Verify(_ context.Context, input execution.VerifyInput) (execution.VerificationReport, error) {
	verifications := make([]union.VerificationRecord, 0, len(input.Effects))
	for _, observed := range input.Effects {
		verifications = append(verifications, union.VerificationRecord{
			ID:        union.VerificationID("verification." + strings.TrimPrefix(string(observed.ID), "effect.")),
			EffectIDs: []union.EffectID{observed.ID}, IntentIDs: append([]union.IntentID(nil), observed.IntentIDs...),
			Kind: "semantic_postcondition", Status: union.VerificationVerified,
			Verifier:    union.VersionedIdentity{ID: "offline.semantic.verifier", Version: "v1"},
			CompletedAt: semanticRouteTestTime.Add(20 * time.Second),
		})
	}
	return execution.VerificationReport{Verifications: verifications}, nil
}

func assertRouteProvenance(t *testing.T, result union.UnifiedExecutionResult, primary map[union.IntentID]union.MechanismPlan) {
	t.Helper()
	attempts := make(map[union.MechanismAttemptID]union.MechanismAttempt, len(result.MechanismTrace))
	for _, attempt := range result.MechanismTrace {
		attempts[attempt.ID] = attempt
	}
	for _, observed := range result.Effects {
		if len(observed.IntentIDs) != 1 {
			t.Fatalf("Effect %s intent identities = %#v", observed.ID, observed.IntentIDs)
		}
		plan := primary[observed.IntentIDs[0]]
		attempt, exists := attempts[observed.MechanismAttemptID]
		if !exists {
			t.Errorf("Effect %s attempt %q is absent from the mechanism trace", observed.ID, observed.MechanismAttemptID)
			continue
		}
		if attempt.MechanismPlanID != plan.ID || attempt.ActualKind != plan.Kind ||
			attempt.ActualOrigin != plan.Origin || attempt.ActualOwner != plan.Owner {
			t.Errorf("Effect %s attempt provenance = %q/%q/%q/%q, want %q/%q/%q/%q",
				observed.ID, attempt.MechanismPlanID, attempt.ActualKind, attempt.ActualOrigin, attempt.ActualOwner,
				plan.ID, plan.Kind, plan.Origin, plan.Owner)
		}
		switch {
		case observed.Payload.StructuredOutput != nil:
			if observed.Payload.StructuredOutput.Origin != plan.Origin || observed.Payload.StructuredOutput.Fidelity != plan.SemanticFidelity {
				t.Errorf("structured provenance = %q/%q, want %q/%q", observed.Payload.StructuredOutput.Origin, observed.Payload.StructuredOutput.Fidelity, plan.Origin, plan.SemanticFidelity)
			}
		case observed.Payload.ToolCall != nil:
			tool := observed.Payload.ToolCall
			if tool.Mechanism != plan.CapabilityRef || tool.Origin != plan.Origin || tool.Owner != plan.Owner {
				t.Errorf("tool provenance = %q/%q/%q, want %q/%q/%q", tool.Mechanism, tool.Origin, tool.Owner, plan.CapabilityRef, plan.Origin, plan.Owner)
			}
		case observed.Payload.CodeExecution != nil:
			code := observed.Payload.CodeExecution
			if code.Mechanism != plan.CapabilityRef || code.Origin != plan.Origin {
				t.Errorf("code provenance = %q/%q, want %q/%q", code.Mechanism, code.Origin, plan.CapabilityRef, plan.Origin)
			}
		}
	}
}

type routeSemanticOutcome struct {
	Status       union.ExecutionStatus
	Verification union.VerificationStatus
	Effects      []string
	Verified     []string
	Satisfied    []string
}

func projectRouteSemanticOutcome(t *testing.T, result union.UnifiedExecutionResult) routeSemanticOutcome {
	t.Helper()
	outcome := routeSemanticOutcome{Status: result.Status, Verification: result.VerificationStatus}
	for _, observed := range result.Effects {
		semanticPayload := ""
		switch {
		case observed.Payload.WorkspaceChange != nil:
			change := observed.Payload.WorkspaceChange
			semanticPayload = fmt.Sprintf("%s|%s|%s|%s", change.Kind, change.Before.Hash, change.After.Hash, change.UnifiedDiff)
		case observed.Payload.StructuredOutput != nil:
			structured := observed.Payload.StructuredOutput
			semanticPayload = fmt.Sprintf("json=%t|schema=%t|parsed=%s", structured.JSONValid, structured.SchemaValid, structured.Parsed)
		case observed.Payload.ToolCall != nil:
			tool := observed.Payload.ToolCall
			semanticPayload = fmt.Sprintf("tool=%s|executed=%t|side_effect=%s", tool.ToolID, tool.Executed, tool.SideEffectState)
		case observed.Payload.CodeExecution != nil:
			code := observed.Payload.CodeExecution
			exitCode := -1
			if code.ExitCode != nil {
				exitCode = *code.ExitCode
			}
			semanticPayload = fmt.Sprintf("argv=%s|exit=%d", strings.Join(code.Argv, "\x00"), exitCode)
		default:
			t.Fatalf("Effect %s has no known semantic payload", observed.ID)
		}
		outcome.Effects = append(outcome.Effects, fmt.Sprintf("%s|%s|%s|%s|%s", observed.ID, observed.IntentIDs[0], observed.Kind, observed.Target, semanticPayload))
	}
	for _, verification := range result.Verifications {
		outcome.Verified = append(outcome.Verified, fmt.Sprintf("%s|%s|%s|%s|%s", verification.ID, verification.EffectIDs[0], verification.IntentIDs[0], verification.Kind, verification.Status))
	}
	for _, satisfaction := range result.IntentSatisfaction {
		effectIDs := make([]string, len(satisfaction.EffectIDs))
		for index, effectID := range satisfaction.EffectIDs {
			effectIDs[index] = string(effectID)
		}
		sort.Strings(effectIDs)
		outcome.Satisfied = append(outcome.Satisfied, fmt.Sprintf("%s|%s|%s", satisfaction.IntentID, satisfaction.Status, strings.Join(effectIDs, ",")))
	}
	sort.Strings(outcome.Effects)
	sort.Strings(outcome.Verified)
	sort.Strings(outcome.Satisfied)
	return outcome
}

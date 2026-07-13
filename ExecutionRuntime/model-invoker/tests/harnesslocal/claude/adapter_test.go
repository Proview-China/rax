package claude_test

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/claude"
	harnessprocess "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/process"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func TestClaudeAdapterRunsThroughRuntimeWithApprovalAndNativeEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	adapter, invocation := newClaudeAdapter(t, "lifecycle", "exec-claude-lifecycle")
	registry := execution.NewRegistry()
	if err := registry.Register(ctx, adapter); err != nil {
		t.Fatal(err)
	}
	runtime, err := execution.NewRuntime(execution.RuntimeConfig{Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	running, err := runtime.Start(ctx, "claude-test-adapter", invocation)
	if err != nil {
		t.Fatal(err)
	}
	approval := waitForApproval(t, ctx, running)
	command := union.ExecutionCommand{
		SemanticVersion: union.SemanticVersionV1, ExecutionID: invocation.Request.ExecutionID,
		Kind: union.CommandApproveAction, ExpectedExecutionStatus: running.State().ExecutionStatus(),
		IdempotencyKey: "approve-claude-edit", ApprovalID: approval.Control.ApprovalID,
		ActionID: approval.Control.ActionID, MechanismAttemptID: approval.Control.MechanismAttemptID,
		InputDigest: approval.Control.InputDigest, ActionRevision: approval.Control.ActionRevision,
	}
	if err := running.Command(ctx, command); err != nil {
		t.Fatalf("approve: %v", err)
	}
	result, err := running.Wait(ctx)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if result.ContextManifest.ID != "expected-claude.actual.claude" {
		t.Fatalf("actual manifest id=%q", result.ContextManifest.ID)
	}
	if !hasComponent(result.ContextManifest, "expected", "profile-baseline") || !hasComponent(result.ContextManifest, "launch_probe", "actual_executable") {
		t.Fatalf("actual manifest lost baseline or launch evidence: %#v", result.ContextManifest.Components)
	}
	events := running.Events()
	for _, required := range []string{
		"selected", "attempt_started", "approval_requested", "content_delta", "reasoning_delta",
		"model_tool_call", "tool_action", "model_tool_result", "unknown_native_event", execution.EventKindRouteTerminalCandidate,
	} {
		if !hasEventKind(events, required) {
			t.Errorf("missing event kind %q", required)
		}
	}
	if state := running.State(); state.Attempts[approval.Control.MechanismAttemptID] != union.AttemptStatusCompleted {
		t.Fatalf("attempt state=%q", state.Attempts[approval.Control.MechanismAttemptID])
	}
	assertToolItemLifecycle(t, events, approval, union.ItemStatusCompleted)
}

func TestClaudeEOFWithoutResultBecomesIndeterminate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	adapter, invocation := newClaudeAdapter(t, "eof", "exec-claude-eof")
	report, err := adapter.Preflight(ctx, invocation)
	if err != nil || !report.Accepted {
		t.Fatalf("preflight=%#v err=%v", report, err)
	}
	session, err := adapter.Open(ctx, invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	var terminal execution.RouteTerminalCandidate
	for {
		event, receiveErr := session.Receive(ctx)
		if receiveErr != nil {
			if errors.Is(receiveErr, io.EOF) {
				break
			}
			t.Fatal(receiveErr)
		}
		if event.Diagnostic != nil && event.Diagnostic.Kind == execution.EventKindRouteTerminalCandidate {
			if err := json.Unmarshal(event.Diagnostic.Payload, &terminal); err != nil {
				t.Fatal(err)
			}
		}
	}
	if terminal.Status != union.ExecutionStatusIndeterminate || terminal.StopReason != "eof_without_result" {
		t.Fatalf("terminal=%#v", terminal)
	}
}

func TestClaudeCancellationProducesAckQuiescenceAndCancelledRuntime(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	adapter, invocation := newClaudeAdapter(t, "interrupt", "exec-claude-cancel")
	registry := execution.NewRegistry()
	if err := registry.Register(ctx, adapter); err != nil {
		t.Fatal(err)
	}
	runtime, _ := execution.NewRuntime(execution.RuntimeConfig{Registry: registry})
	running, err := runtime.Start(ctx, "claude-test-adapter", invocation)
	if err != nil {
		t.Fatal(err)
	}
	command := union.ExecutionCommand{
		SemanticVersion: union.SemanticVersionV1, ExecutionID: invocation.Request.ExecutionID,
		Kind: union.CommandCancelExecution, ExpectedExecutionStatus: running.State().ExecutionStatus(),
		IdempotencyKey: "cancel-claude",
	}
	if err := running.Command(ctx, command); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	result, err := running.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != union.ExecutionStatusCancelled {
		t.Fatalf("result status=%q", result.Status)
	}
	state := running.State()
	if state.Cancellation != execution.CancellationReconciled {
		t.Fatalf("cancellation phase=%q", state.Cancellation)
	}
	if !hasEventKind(running.Events(), execution.ControlCancelAcknowledged) || !hasEventKind(running.Events(), execution.ControlCancellationQuiesced) {
		t.Fatal("missing native cancellation acknowledgement/quiescence")
	}
}

func TestClaudeManifestDriftRejectsBeforePrompt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	adapter, invocation := newClaudeAdapter(t, "drift", "exec-claude-drift")
	report, err := adapter.Preflight(ctx, invocation)
	if err != nil {
		t.Fatal(err)
	}
	if report.Accepted || report.RejectionCode != "claude_manifest_drift" {
		t.Fatalf("report=%#v", report)
	}
}

func TestClaudeStructuredResultProducesExecutionScopedCausalAttempt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	config := claudeConfig(t, "structured")
	adapter, err := claude.New(config)
	if err != nil {
		t.Fatal(err)
	}
	invocation := structuredInvocation("exec-claude-structured", config.Route, "claude.output_format")
	if report, err := adapter.Preflight(ctx, invocation); err != nil || !report.Accepted {
		t.Fatalf("preflight=%#v err=%v", report, err)
	}
	session, err := adapter.Open(ctx, invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	var started, completed bool
	for {
		event, receiveErr := session.Receive(ctx)
		if errors.Is(receiveErr, io.EOF) {
			break
		}
		if receiveErr != nil {
			t.Fatal(receiveErr)
		}
		if event.Mechanism == nil || event.Mechanism.Attempt == nil {
			continue
		}
		attempt := event.Mechanism.Attempt
		if attempt.MechanismPlanID != "plan-structured" || !strings.Contains(string(attempt.ID), string(invocation.Request.ExecutionID)) {
			t.Fatalf("structured attempt is not bound to this execution/plan: %#v", attempt)
		}
		started = started || attempt.Status == union.AttemptStatusRunning
		completed = completed || attempt.Status == union.AttemptStatusCompleted
	}
	if !started || !completed {
		t.Fatalf("structured output attempt lifecycle started=%v completed=%v", started, completed)
	}
}

func TestClaudeFailedToolResultClosesAttemptAndExecutionItem(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	adapter, invocation := newClaudeAdapter(t, "tool-failure", "exec-claude-tool-failure")
	registry := execution.NewRegistry()
	if err := registry.Register(ctx, adapter); err != nil {
		t.Fatal(err)
	}
	runtime, _ := execution.NewRuntime(execution.RuntimeConfig{Registry: registry})
	running, err := runtime.Start(ctx, "claude-test-adapter", invocation)
	if err != nil {
		t.Fatal(err)
	}
	approval := waitForApproval(t, ctx, running)
	if err := running.Command(ctx, approvalCommand(invocation.Request.ExecutionID, running, approval, "approve-claude-failure")); err != nil {
		t.Fatal(err)
	}
	if _, err := running.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if running.State().Attempts[approval.Control.MechanismAttemptID] != union.AttemptStatusFailed {
		t.Fatalf("attempt status=%q", running.State().Attempts[approval.Control.MechanismAttemptID])
	}
	assertToolItemLifecycle(t, running.Events(), approval, union.ItemStatusFailed)
}

func TestClaudeRejectsBareAndConflictingEnvironmentBeforeStart(t *testing.T) {
	_, invocation := newClaudeAdapter(t, "eof", "exec-claude-config-template")
	_ = invocation
	config := claudeConfig(t, "eof")
	config.Process.Arguments = append(config.Process.Arguments, "--bare")
	if _, err := claude.New(config); !errors.Is(err, claude.ErrInvalidConfig) {
		t.Fatalf("bare error=%v", err)
	}
	config = claudeConfig(t, "eof")
	config.Process.Environment = map[string]string{"ANTHROPIC_BASE_URL": "https://example.invalid"}
	config.Process.AllowedEnvironment = []string{"ANTHROPIC_BASE_URL"}
	if _, err := claude.New(config); !errors.Is(err, claude.ErrInvalidConfig) {
		t.Fatalf("environment error=%v", err)
	}
}

func newClaudeAdapter(t *testing.T, mode string, executionID union.ExecutionID) (*claude.Adapter, execution.Invocation) {
	t.Helper()
	config := claudeConfig(t, mode)
	adapter, err := claude.New(config)
	if err != nil {
		t.Fatal(err)
	}
	return adapter, validInvocation(executionID, config.Route)
}

func claudeConfig(t *testing.T, mode string) claude.Config {
	t.Helper()
	processConfig, directory := helperProcessConfig(t, mode)
	return claude.Config{
		Identity: union.VersionedIdentity{ID: "claude-test-adapter", Version: "v1"},
		Route:    union.VersionedIdentity{ID: "route-claude-test", Version: "v1"},
		Process:  processConfig,
		ExpectedInit: claude.ExpectedInit{
			Model: "claude-test-model", CWD: directory,
			Tools: []string{"Read", "Edit", "Write", "Bash"}, PermissionMode: "default",
			CLIVersion: "2.1.0", APIKeySource: "none",
		},
	}
}

func validInvocation(executionID union.ExecutionID, route union.VersionedIdentity) execution.Invocation {
	profileID := union.VersionedIdentity{ID: "profile-claude-test", Version: "v1"}
	graph := union.IntentGraph{Nodes: []union.IntentNode{{
		ID: "intent-edit", Kind: union.IntentModifyFile, Target: "/workspace/a.txt", Required: true,
	}}}
	expected := union.ContextManifestSummary{
		ID: "expected-claude", Version: "v1", Mode: "semantic_stable",
		Components: []union.ManifestComponent{{Kind: "expected", Name: "profile-baseline", State: "required", Owner: union.ExecutionOwnerHarness}},
		Tools: union.ToolSurfaceManifest{Entries: []union.ToolSurfaceEntry{{
			ID: "Edit", NativeName: "Edit", Discovered: true, Registered: true, ModelVisible: true, Executable: true,
			PermissionMode: "default", Owner: union.ExecutionOwnerHarness, Probe: union.ToolSurfaceProbe{Status: union.ToolProbeNotRun},
		}}},
		OpaqueFields: []string{"expected.opaque"},
	}
	request := union.UnifiedExecutionRequest{
		SemanticVersion: union.SemanticVersionV1, ExecutionID: executionID,
		ProfileSelector: union.ProfileSelector{Exact: &profileID}, ExecutionKind: union.ExecutionKindAgent,
		Input:             []union.InputItem{{ID: "input-1", Kind: "message", Role: "user", Content: []union.ContentPart{{Kind: "text", Text: "edit a.txt"}}}},
		SessionIntent:     union.SessionIntent{Mode: "new", TurnID: "turn-1"},
		DegradationPolicy: union.DegradationPolicy{Default: union.DegradationDefaultReject}, IntentGraph: graph,
	}
	plan := union.PreparedExecutionPlan{
		SemanticVersion: union.SemanticVersionV1, ExecutionID: executionID, Profile: profileID, Route: route,
		ProfileKeyDigest: "sha256:profile-key", ExecutionKind: union.ExecutionKindAgent, IntentGraph: graph,
		Mechanisms: []union.MechanismPlan{{
			ID: "plan-edit", IntentID: "intent-edit", Kind: "claude_edit", Origin: union.CapabilityOriginHarnessHosted,
			Owner: union.ExecutionOwnerHarness, SelectionAuthority: union.SelectionAuthorityHarness,
			SemanticFidelity: union.SemanticFidelityExact,
		}},
		ExpectedManifest: expected, RouteFingerprint: "sha256:route-claude-test",
	}
	invocation, err := execution.NewInvocation(request, plan)
	if err != nil {
		panic(err)
	}
	return invocation
}

func structuredInvocation(executionID union.ExecutionID, route union.VersionedIdentity, mechanismKind string) execution.Invocation {
	base := validInvocation(executionID, route)
	graph := union.IntentGraph{Nodes: []union.IntentNode{{
		ID: "intent-structured", Kind: union.IntentProduceStructured, Target: "summary", Required: true,
		Postconditions: []union.Condition{{Kind: "json_schema_valid"}}, AcceptedFidelity: []union.SemanticFidelity{union.SemanticFidelityExact},
	}}}
	base.Request.IntentGraph = graph
	base.Request.OutputContract = union.OutputContract{
		AcceptedContentKinds: []string{"json"}, CompletionMode: "final",
		JSONSchema: json.RawMessage(`{"type":"object","required":["marker"],"properties":{"marker":{"const":"ok"}},"additionalProperties":false}`),
	}
	base.Plan.IntentGraph = graph
	base.Plan.Mechanisms = []union.MechanismPlan{{
		ID: "plan-structured", IntentID: "intent-structured", Kind: mechanismKind,
		Origin: union.CapabilityOriginNative, Owner: union.ExecutionOwnerProvider,
		SelectionAuthority: union.SelectionAuthorityProvider, SemanticFidelity: union.SemanticFidelityExact,
	}}
	base.Plan.Digest = ""
	delete(base.Plan.Metadata, "request_digest")
	invocation, err := execution.NewInvocation(base.Request, base.Plan)
	if err != nil {
		panic(err)
	}
	return invocation
}

func helperProcessConfig(t *testing.T, mode string) (harnessprocess.Config, string) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	return harnessprocess.Config{
		Executable: executable, ExpectedExecutableDigest: fmt.Sprintf("sha256:%x", sha256.Sum256(data)),
		Arguments:        []string{"-test.run=^TestClaudeFakeProcess$", "--", mode},
		WorkingDirectory: directory, AllowedWorkingDirectories: []string{directory}, Protocol: harnessprocess.ProtocolJSONL,
		TerminationGrace: 100 * time.Millisecond, KillWait: time.Second,
	}, directory
}

func waitForApproval(t *testing.T, ctx context.Context, running *execution.Execution) union.UnifiedExecutionEvent {
	t.Helper()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		for _, event := range running.Events() {
			if event.Control != nil && event.Control.Kind == execution.ControlApprovalRequested {
				return event
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for approval; state=%#v events=%#v", running.State(), running.Events())
		case <-ticker.C:
		}
	}
}

func approvalCommand(executionID union.ExecutionID, running *execution.Execution, approval union.UnifiedExecutionEvent, key string) union.ExecutionCommand {
	return union.ExecutionCommand{
		SemanticVersion: union.SemanticVersionV1, ExecutionID: executionID,
		Kind: union.CommandApproveAction, ExpectedExecutionStatus: running.State().ExecutionStatus(),
		IdempotencyKey: key, ApprovalID: approval.Control.ApprovalID, ActionID: approval.Control.ActionID,
		MechanismAttemptID: approval.Control.MechanismAttemptID, InputDigest: approval.Control.InputDigest,
		ActionRevision: approval.Control.ActionRevision,
	}
}

func hasComponent(manifest union.ContextManifestSummary, kind, name string) bool {
	for _, component := range manifest.Components {
		if component.Kind == kind && component.Name == name {
			return true
		}
	}
	return false
}

func hasEventKind(events []union.UnifiedExecutionEvent, kind string) bool {
	for _, event := range events {
		switch {
		case event.Mechanism != nil && event.Mechanism.Kind == kind:
			return true
		case event.Control != nil && event.Control.Kind == kind:
			return true
		case event.Model != nil && event.Model.Kind == kind:
			return true
		case event.Item != nil && event.Item.Kind == kind:
			return true
		case event.Diagnostic != nil && event.Diagnostic.Kind == kind:
			return true
		}
	}
	return false
}

func assertToolItemLifecycle(t *testing.T, events []union.UnifiedExecutionEvent, approval union.UnifiedExecutionEvent, terminalStatus union.ItemStatus) {
	t.Helper()
	initial, terminal, modelCall, modelResult := false, false, false, false
	for _, event := range events {
		if event.Model != nil && event.Model.ActionID == approval.Control.ActionID {
			if event.Header.ActionID != event.Model.ActionID || event.Header.MechanismAttemptID != approval.Control.MechanismAttemptID {
				t.Fatalf("Model action correlation mismatch: %#v", event)
			}
			if event.Model.Kind == "model_tool_call" {
				modelCall = true
			}
			if event.Model.Kind == "model_tool_result" {
				modelResult = true
				if event.Header.ItemID != union.ItemID(approval.Control.ActionID) || event.Model.ExecutionItemID != union.ItemID(approval.Control.ActionID) {
					t.Fatalf("tool result Item correlation mismatch: %#v", event)
				}
			}
		}
		if event.Item == nil || event.Item.Kind != "tool_action" {
			continue
		}
		item := event.Item.Item
		if event.Header.ActionID != approval.Control.ActionID || item.ActionID != approval.Control.ActionID ||
			event.Header.MechanismAttemptID != approval.Control.MechanismAttemptID || item.AttemptID != approval.Control.MechanismAttemptID ||
			event.Header.IntentID != "intent-edit" || event.Header.MechanismPlanID != "plan-edit" {
			t.Fatalf("tool Item correlation mismatch: %#v", event)
		}
		if item.Status == union.ItemStatusPending || item.Status == union.ItemStatusInProgress {
			initial = true
		}
		if item.Status == terminalStatus {
			terminal = true
		}
	}
	if !initial || !terminal || !modelCall || !modelResult {
		t.Fatalf("tool evidence lifecycle initial=%v terminal=%v model_call=%v model_result=%v", initial, terminal, modelCall, modelResult)
	}
}

func TestClaudeFakeProcess(t *testing.T) {
	mode := helperMode(os.Args)
	if mode == "" {
		return
	}
	os.Exit(runClaudeFake(mode, os.Stdin, os.Stdout))
}

func runClaudeFake(mode string, input io.Reader, output io.Writer) int {
	reader := bufio.NewReader(input)
	encoder := json.NewEncoder(output)
	cwd, _ := os.Getwd()
	tools := []string{"Read", "Edit", "Write", "Bash"}
	if mode == "drift" {
		tools = []string{"Read", "Write", "Bash"}
	}
	if err := encoder.Encode(map[string]any{
		"type": "system", "subtype": "init", "session_id": "claude-session", "uuid": "init-1",
		"model": "claude-test-model", "cwd": cwd, "tools": tools, "mcp_servers": []any{},
		"permissionMode": "default", "claude_code_version": "2.1.0", "apiKeySource": "none",
	}); err != nil {
		return 2
	}
	initialize, err := readFrame(reader)
	if err != nil || nestedString(initialize, "request", "subtype") != "initialize" {
		return 3
	}
	if err := respondControl(encoder, objectString(initialize, "request_id"), map[string]any{"commands": []string{"interrupt"}}); err != nil {
		return 4
	}
	if mode == "drift" {
		for {
			if _, err := reader.ReadByte(); err != nil {
				return 0
			}
		}
	}
	user, err := readFrame(reader)
	if err != nil || objectString(user, "type") != "user" {
		return 5
	}
	switch mode {
	case "eof":
		return 0
	case "interrupt":
		request, err := readFrame(reader)
		if err != nil || nestedString(request, "request", "subtype") != "interrupt" {
			return 6
		}
		if err := respondControl(encoder, objectString(request, "request_id"), map[string]any{"interrupted": true}); err != nil {
			return 7
		}
		if err := encoder.Encode(resultMessage("interrupted", false)); err != nil {
			return 8
		}
		_, _ = reader.ReadByte()
		return 0
	case "lifecycle", "tool-failure":
		return runClaudeLifecycle(reader, encoder, mode == "tool-failure")
	case "structured":
		if err := encoder.Encode(map[string]any{"type": "stream_event", "uuid": "structured-1", "session_id": "claude-session", "parent_tool_use_id": nil, "event": map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": `{"marker":"ok"}`}}}); err != nil {
			return 14
		}
		result := resultMessage("success", false)
		result["result"] = `{"marker":"ok"}`
		if err := encoder.Encode(result); err != nil {
			return 15
		}
		return 0
	default:
		return 9
	}
}

func runClaudeLifecycle(reader *bufio.Reader, encoder *json.Encoder, failed bool) int {
	frames := []any{
		map[string]any{"type": "stream_event", "uuid": "s1", "session_id": "claude-session", "parent_tool_use_id": nil, "event": map[string]any{"type": "message_start", "message": map[string]any{"id": "m1", "model": "claude-test-model"}}},
		map[string]any{"type": "stream_event", "uuid": "s2", "session_id": "claude-session", "parent_tool_use_id": nil, "event": map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": "editing"}}},
		map[string]any{"type": "stream_event", "uuid": "s3", "session_id": "claude-session", "parent_tool_use_id": nil, "event": map[string]any{"type": "content_block_delta", "index": 1, "delta": map[string]any{"type": "thinking_delta", "thinking": "checking"}}},
		map[string]any{"type": "stream_event", "uuid": "s4", "session_id": "claude-session", "parent_tool_use_id": nil, "event": map[string]any{"type": "content_block_start", "index": 2, "content_block": map[string]any{"type": "tool_use", "id": "tool-1", "name": "Edit", "input": map[string]any{"file_path": "a.txt"}}}},
	}
	for _, frame := range frames {
		if err := encoder.Encode(frame); err != nil {
			return 10
		}
	}
	if err := encoder.Encode(map[string]any{
		"type": "control_request", "request_id": "permission-1",
		"request": map[string]any{"subtype": "can_use_tool", "tool_name": "Edit", "tool_use_id": "tool-1", "input": map[string]any{"file_path": "a.txt"}, "permission_suggestions": []any{}},
	}); err != nil {
		return 11
	}
	permission, err := readFrame(reader)
	if err != nil || nestedString(permission, "response", "subtype") != "success" || !strings.Contains(string(permission), `"behavior":"allow"`) {
		return 12
	}
	remaining := []any{
		map[string]any{"type": "assistant", "uuid": "a1", "session_id": "claude-session", "parent_tool_use_id": nil, "message": map[string]any{"id": "m1", "role": "assistant", "model": "claude-test-model", "content": []any{map[string]any{"type": "tool_use", "id": "tool-1", "name": "Edit", "input": map[string]any{"file_path": "a.txt"}}}}},
		map[string]any{"type": "user", "uuid": "u1", "session_id": "claude-session", "parent_tool_use_id": nil, "message": map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "tool-1", "content": "tool result", "is_error": failed}}}},
		map[string]any{"type": "future_claude_event", "subtype": "v9", "opaque": true},
		resultMessage(map[bool]string{false: "success", true: "error_during_execution"}[failed], failed),
	}
	for _, frame := range remaining {
		if err := encoder.Encode(frame); err != nil {
			return 13
		}
	}
	return 0
}

func resultMessage(subtype string, isError bool) map[string]any {
	return map[string]any{
		"type": "result", "subtype": subtype, "is_error": isError, "session_id": "claude-session",
		"duration_ms": 10, "duration_api_ms": 5, "num_turns": 1, "result": "done",
		"usage": map[string]any{"input_tokens": 1, "output_tokens": 1}, "permission_denials": []any{},
	}
}

func respondControl(encoder *json.Encoder, requestID string, response any) error {
	return encoder.Encode(map[string]any{
		"type":     "control_response",
		"response": map[string]any{"subtype": "success", "request_id": requestID, "response": response},
	})
}

func readFrame(reader *bufio.Reader) (json.RawMessage, error) {
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	if !json.Valid(line) {
		return nil, fmt.Errorf("invalid json")
	}
	return json.RawMessage(line), nil
}

func objectString(raw json.RawMessage, key string) string {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return ""
	}
	var value string
	_ = json.Unmarshal(object[key], &value)
	return value
}

func nestedString(raw json.RawMessage, objectKey, key string) string {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return ""
	}
	return objectString(object[objectKey], key)
}

func helperMode(arguments []string) string {
	for index, argument := range arguments {
		if argument == "--" && index+1 < len(arguments) {
			return arguments[index+1]
		}
	}
	return ""
}

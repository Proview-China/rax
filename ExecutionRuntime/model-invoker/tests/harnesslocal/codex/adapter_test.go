package codex_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	codex "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/codexappserver"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func TestCodexAdapterOfflineContractAndAttemptBindings(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	invocation := codexAdapterInvocation()
	adapter := newCodexExecutionAdapter(t, "lifecycle")

	descriptor, err := adapter.Describe(ctx)
	if err != nil || descriptor.Origin != union.EventOriginHarness || !descriptor.Supports(union.ExecutionKindAgent) {
		t.Fatalf("descriptor = %#v, %v", descriptor, err)
	}
	report, err := adapter.Preflight(ctx, invocation)
	if err != nil || !report.Accepted {
		t.Fatalf("preflight = %#v, %v", report, err)
	}
	if !manifestHasComponent(report.ActualManifest, "launch_probe", "actual_executable") || !manifestHasComponent(report.ActualManifest, "native_surface", "initialize") {
		t.Fatalf("actual manifest lacks probe evidence: %#v", report.ActualManifest)
	}

	session, err := adapter.Open(ctx, invocation)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer session.Close()
	var attemptID union.MechanismAttemptID
	var sawApproval, sawTool, sawAttemptCompleted, sawTerminal bool
	var lastSource uint64
	for {
		event, receiveErr := session.Receive(ctx)
		if errors.Is(receiveErr, io.EOF) {
			break
		}
		if receiveErr != nil {
			t.Fatalf("receive: %v", receiveErr)
		}
		if event.Header.SourceSequence <= lastSource {
			t.Fatalf("source sequence regressed: %d <= %d", event.Header.SourceSequence, lastSource)
		}
		lastSource = event.Header.SourceSequence
		if event.Mechanism != nil && event.Mechanism.Attempt != nil {
			attemptID = event.Mechanism.Attempt.ID
			if event.Mechanism.Attempt.Status == union.AttemptStatusCompleted {
				sawAttemptCompleted = true
			}
		}
		if event.Control != nil && event.Control.Kind == execution.ControlApprovalRequested {
			sawApproval = true
			if event.Header.IntentID == "" || event.Header.MechanismPlanID == "" || event.Control.MechanismAttemptID == "" || event.Control.ExpiresAt.IsZero() {
				t.Fatalf("approval is not bound to a live attempt: %#v", event)
			}
			if err := session.Command(ctx, union.ExecutionCommand{
				ExecutionID: invocation.Request.ExecutionID, Kind: union.CommandApproveAction,
				ApprovalID: event.Control.ApprovalID, ActionID: event.Control.ActionID, MechanismAttemptID: event.Control.MechanismAttemptID,
			}); err != nil {
				t.Fatalf("approve: %v", err)
			}
		}
		if event.Item != nil && event.Item.Kind == string(codex.NativeDynamicToolRequest) {
			sawTool = true
			if event.Item.Item.AttemptID == "" || event.Item.Item.ActionID == "" {
				t.Fatalf("dynamic tool is not bound to an attempt/action: %#v", event)
			}
			payload := json.RawMessage(`{"output":"ok"}`)
			if err := session.Command(ctx, union.ExecutionCommand{
				ExecutionID: invocation.Request.ExecutionID, Kind: union.CommandProvideToolResult,
				ActionID: event.Item.Item.ActionID, MechanismAttemptID: event.Item.Item.AttemptID, Payload: payload,
			}); err != nil {
				t.Fatalf("tool result: %v", err)
			}
		}
		if event.Diagnostic != nil && event.Diagnostic.Kind == execution.EventKindRouteTerminalCandidate {
			sawTerminal = true
		}
	}
	if attemptID == "" || !sawApproval || !sawTool || !sawAttemptCompleted || !sawTerminal {
		t.Fatalf("missing contract evidence attempt=%q approval=%v tool=%v completed=%v terminal=%v", attemptID, sawApproval, sawTool, sawAttemptCompleted, sawTerminal)
	}
}

func TestCodexAdapterRunsThroughExecutionRuntimeOffline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	invocation := codexAdapterInvocation()
	adapter := newCodexExecutionAdapter(t, "lifecycle")
	registry := execution.NewRegistry()
	if err := registry.Register(ctx, adapter); err != nil {
		t.Fatal(err)
	}
	runtime, err := execution.NewRuntime(execution.RuntimeConfig{Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	running, err := runtime.Start(ctx, "codex-adapter-test", invocation)
	if err != nil {
		t.Fatalf("runtime start: %v", err)
	}

	approval := waitCodexEvent(t, ctx, running, func(event union.UnifiedExecutionEvent) bool {
		return event.Control != nil && event.Control.Kind == execution.ControlApprovalRequested
	})
	if err := running.Command(ctx, union.ExecutionCommand{
		SemanticVersion: union.SemanticVersionV1, ExecutionID: invocation.Request.ExecutionID, Kind: union.CommandApproveAction,
		ExpectedExecutionStatus: "running", IdempotencyKey: "approve-1", ApprovalID: approval.Control.ApprovalID,
		ActionID: approval.Control.ActionID, MechanismAttemptID: approval.Control.MechanismAttemptID,
		InputDigest: approval.Control.InputDigest, ActionRevision: approval.Control.ActionRevision,
	}); err != nil {
		t.Fatalf("runtime approve: %v", err)
	}
	tool := waitCodexEvent(t, ctx, running, func(event union.UnifiedExecutionEvent) bool {
		return event.Item != nil && event.Item.Kind == string(codex.NativeDynamicToolRequest)
	})
	if err := running.Command(ctx, union.ExecutionCommand{
		SemanticVersion: union.SemanticVersionV1, ExecutionID: invocation.Request.ExecutionID, Kind: union.CommandProvideToolResult,
		ExpectedExecutionStatus: "running", IdempotencyKey: "tool-1", ActionID: tool.Item.Item.ActionID,
		MechanismAttemptID: tool.Item.Item.AttemptID, Payload: json.RawMessage(`{"output":"ok"}`),
	}); err != nil {
		t.Fatalf("runtime tool result: %v", err)
	}
	result, err := running.Wait(ctx)
	if err != nil {
		t.Fatalf("runtime wait: %v", err)
	}
	if result.Status != union.ExecutionStatusIndeterminate {
		t.Fatalf("unreconciled Harness side effects must remain indeterminate, got %s", result.Status)
	}
	var attempt, resolved, terminal bool
	for _, event := range running.Events() {
		attempt = attempt || event.Mechanism != nil && event.Mechanism.Attempt != nil
		resolved = resolved || event.Control != nil && event.Control.Kind == execution.ControlApprovalResolved
		terminal = terminal || event.Diagnostic != nil && event.Diagnostic.Kind == execution.EventKindRouteTerminalCandidate
	}
	if !attempt || !resolved || !terminal {
		t.Fatalf("runtime ledger evidence attempt=%v resolved=%v terminal=%v", attempt, resolved, terminal)
	}
}

func TestCodexAdapterCancelEmitsStateMachineEvidence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	invocation := codexAdapterInvocation()
	adapter := newCodexExecutionAdapter(t, "interrupt")
	if report, err := adapter.Preflight(ctx, invocation); err != nil || !report.Accepted {
		t.Fatalf("preflight = %#v, %v", report, err)
	}
	session, err := adapter.Open(ctx, invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if _, err := session.Receive(ctx); err != nil { // attempt_started
		t.Fatal(err)
	}
	if err := session.Command(ctx, union.ExecutionCommand{ExecutionID: invocation.Request.ExecutionID, Kind: union.CommandCancelExecution}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	var acknowledged, quiesced, cancelledAttempt, terminal bool
	for {
		event, receiveErr := session.Receive(ctx)
		if errors.Is(receiveErr, io.EOF) {
			break
		}
		if receiveErr != nil {
			t.Fatal(receiveErr)
		}
		if event.Control != nil {
			acknowledged = acknowledged || event.Control.Kind == execution.ControlCancelAcknowledged
			quiesced = quiesced || event.Control.Kind == execution.ControlCancellationQuiesced
		}
		cancelledAttempt = cancelledAttempt || event.Mechanism != nil && event.Mechanism.Attempt != nil && event.Mechanism.Attempt.Status == union.AttemptStatusCancelled
		terminal = terminal || event.Diagnostic != nil && event.Diagnostic.Kind == execution.EventKindRouteTerminalCandidate
	}
	if !acknowledged || !quiesced || !cancelledAttempt || !terminal {
		t.Fatalf("cancel evidence ack=%v quiesced=%v attempt=%v terminal=%v", acknowledged, quiesced, cancelledAttempt, terminal)
	}
}

func manifestHasComponent(manifest union.ContextManifestSummary, kind, name string) bool {
	for _, component := range manifest.Components {
		if component.Kind == kind && component.Name == name {
			return true
		}
	}
	return false
}

func TestCodexAdapterCancellationRunsThroughRuntime(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	invocation := codexAdapterInvocation()
	adapter := newCodexExecutionAdapter(t, "interrupt")
	registry := execution.NewRegistry()
	if err := registry.Register(ctx, adapter); err != nil {
		t.Fatal(err)
	}
	runtime, err := execution.NewRuntime(execution.RuntimeConfig{Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	running, err := runtime.Start(ctx, "codex-adapter-test", invocation)
	if err != nil {
		t.Fatal(err)
	}
	waitCodexEvent(t, ctx, running, func(event union.UnifiedExecutionEvent) bool {
		return event.Mechanism != nil && event.Mechanism.Attempt != nil && event.Mechanism.Attempt.Status == union.AttemptStatusRunning
	})
	if err := running.Command(ctx, union.ExecutionCommand{
		SemanticVersion: union.SemanticVersionV1, ExecutionID: invocation.Request.ExecutionID, Kind: union.CommandCancelExecution,
		ExpectedExecutionStatus: "running", IdempotencyKey: "cancel-1",
	}); err != nil {
		t.Fatalf("runtime cancel: %v", err)
	}
	result, err := running.Wait(ctx)
	if err != nil {
		t.Fatalf("runtime wait: %v", err)
	}
	if result.Status != union.ExecutionStatusCancelled {
		for _, event := range running.Events() {
			t.Logf("event seq=%d family=%s mechanism=%#v control=%#v diagnostic=%#v", event.Header.Sequence, event.Header.Family, event.Mechanism, event.Control, event.Diagnostic)
		}
		t.Logf("state=%#v", running.State())
		t.Fatalf("cancel result = %s", result.Status)
	}
}

func waitCodexEvent(t *testing.T, ctx context.Context, running *execution.Execution, match func(union.UnifiedExecutionEvent) bool) union.UnifiedExecutionEvent {
	t.Helper()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		for _, event := range running.Events() {
			if match(event) {
				return event
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for runtime event: %v", ctx.Err())
		case <-ticker.C:
		}
	}
}

func codexAdapterInvocation() execution.Invocation {
	profile := union.VersionedIdentity{ID: "profile-codex-test", Version: "v1"}
	route := union.VersionedIdentity{ID: "route-codex-test", Version: "v1"}
	graph := union.IntentGraph{Nodes: []union.IntentNode{{
		ID: "intent-codex-test", Kind: union.IntentModifyFile, Target: "/workspace/a.txt", Required: true,
		AcceptedFidelity: []union.SemanticFidelity{union.SemanticFidelityTransformed},
	}}}
	request := union.UnifiedExecutionRequest{
		SemanticVersion: union.SemanticVersionV1, ExecutionID: "exec-codex-adapter", ExecutionKind: union.ExecutionKindAgent,
		ProfileSelector: union.ProfileSelector{Exact: &profile},
		Input:           []union.InputItem{{ID: "message-1", Kind: "message", Role: "user", Content: []union.ContentPart{{Kind: "text", Text: "edit a.txt"}}}},
		Instructions:    []union.Instruction{{ID: "instruction-1", Authority: "developer", Scope: "execution", ConflictPolicy: "higher_authority_wins", Content: []union.ContentPart{{Kind: "text", Text: "Use the registered tools."}}}},
		Tools: []union.ToolDefinition{{
			ID: "lookup", Name: "lookup", Kind: "function", ExecutionOwner: union.ExecutionOwnerPraxis,
			InputSchema: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}},"additionalProperties":false}`),
		}},
		ToolPolicy:        union.ToolPolicy{DefaultApproval: "on_side_effect", Parallelism: 1, MaxActions: 4},
		OutputContract:    union.OutputContract{AcceptedContentKinds: []string{"text"}, CompletionMode: "final"},
		SessionIntent:     union.SessionIntent{Mode: "new"},
		ExecutionPolicy:   union.ExecutionPolicy{Stream: true, UserPresence: "present", Foreground: "required", InteractionMode: "interactive", MaxConcurrency: 1},
		Budget:            union.Budget{MaxWallTime: time.Minute, MaxToolActions: 4},
		DegradationPolicy: union.DegradationPolicy{Default: union.DegradationDefaultReject}, IntentGraph: graph,
	}
	plan := union.PreparedExecutionPlan{
		SemanticVersion: union.SemanticVersionV1, ExecutionID: request.ExecutionID, Profile: profile, Route: route,
		ProfileKeyDigest: "sha256:profile", ExecutionKind: union.ExecutionKindAgent, IntentGraph: graph,
		Mechanisms: []union.MechanismPlan{{
			ID: "plan-codex-test", IntentID: "intent-codex-test", Kind: "codex_app_server_turn",
			Origin: union.CapabilityOriginHarnessHosted, Owner: union.ExecutionOwnerHarness, SelectionAuthority: union.SelectionAuthorityHarness,
			SemanticFidelity: union.SemanticFidelityTransformed,
		}},
		ExpectedManifest: union.ContextManifestSummary{ID: "expected-codex-manifest", Version: "v1", Mode: "harness"},
		RouteFingerprint: "sha256:codex-route",
	}
	invocation, err := execution.NewInvocation(request, plan)
	if err != nil {
		panic(err)
	}
	return invocation
}

func newCodexExecutionAdapter(t *testing.T, mode string) *codex.Adapter {
	t.Helper()
	adapter, err := codex.NewAdapter(codex.AdapterConfig{
		Identity: union.VersionedIdentity{ID: "codex-adapter-test", Version: "v1"}, RouteID: "route-codex-test",
		Client: codex.Config{
			Process: helperProcessConfig(t, mode), ClientInfo: codex.ClientInfo{Name: "praxis-test", Version: "v1"},
			Capabilities: json.RawMessage(`{"experimentalApi":true}`),
		},
		Model: "gpt-test", ApprovalPolicy: "on-request", Sandbox: "workspace-write", Ephemeral: true, ApprovalTTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

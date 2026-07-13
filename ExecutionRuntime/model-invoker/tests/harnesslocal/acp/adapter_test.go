package acp_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	acp "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/acp"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func TestACPAdapterOfflineContractAndAttemptBindings(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	invocation := acpAdapterInvocation()
	adapter := newACPExecutionAdapter(t, "lifecycle")

	descriptor, err := adapter.Describe(ctx)
	if err != nil || descriptor.Origin != union.EventOriginHarness || !descriptor.Supports(union.ExecutionKindAgent) {
		t.Fatalf("descriptor = %#v, %v", descriptor, err)
	}
	report, err := adapter.Preflight(ctx, invocation)
	if err != nil || !report.Accepted {
		t.Fatalf("preflight = %#v, %v", report, err)
	}
	if !acpManifestHasComponent(report.ActualManifest, "launch_probe", "actual_executable") || !acpManifestHasComponent(report.ActualManifest, "native_surface", "initialize") {
		t.Fatalf("actual manifest lacks probe evidence: %#v", report.ActualManifest)
	}

	session, err := adapter.Open(ctx, invocation)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer session.Close()
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
		if event.Mechanism != nil && event.Mechanism.Attempt != nil && event.Mechanism.Attempt.Status == union.AttemptStatusCompleted {
			sawAttemptCompleted = true
		}
		if event.Item != nil && (event.Item.Kind == string(acp.NativeToolCall) || event.Item.Kind == string(acp.NativeToolCallUpdate)) {
			sawTool = true
			if event.Item.Item.AttemptID == "" || event.Item.Item.ActionID == "" || event.Effect != nil {
				t.Fatalf("tool update is not bound Item-only evidence: %#v", event)
			}
		}
		if event.Control != nil && event.Control.Kind == execution.ControlApprovalRequested {
			sawApproval = true
			if event.Header.IntentID == "" || event.Header.MechanismPlanID == "" || event.Control.MechanismAttemptID == "" || event.Control.ExpiresAt.IsZero() {
				t.Fatalf("permission is not bound to a live attempt: %#v", event)
			}
			payload := json.RawMessage(`{"native_result":{"outcome":{"outcome":"selected","optionId":"allow_once"}}}`)
			if err := session.Command(ctx, union.ExecutionCommand{
				ExecutionID: invocation.Request.ExecutionID, Kind: union.CommandApproveAction,
				ApprovalID: event.Control.ApprovalID, ActionID: event.Control.ActionID, MechanismAttemptID: event.Control.MechanismAttemptID,
				Payload: payload,
			}); err != nil {
				t.Fatalf("approve: %v", err)
			}
		}
		if event.Diagnostic != nil && event.Diagnostic.Kind == execution.EventKindRouteTerminalCandidate {
			sawTerminal = true
		}
	}
	if !sawApproval || !sawTool || !sawAttemptCompleted || !sawTerminal {
		t.Fatalf("missing contract evidence approval=%v tool=%v completed=%v terminal=%v", sawApproval, sawTool, sawAttemptCompleted, sawTerminal)
	}
}

func TestACPAdapterRunsThroughExecutionRuntimeOffline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	invocation := acpAdapterInvocation()
	adapter := newACPExecutionAdapter(t, "lifecycle")
	registry := execution.NewRegistry()
	if err := registry.Register(ctx, adapter); err != nil {
		t.Fatal(err)
	}
	runtime, err := execution.NewRuntime(execution.RuntimeConfig{Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	running, err := runtime.Start(ctx, "acp-adapter-test", invocation)
	if err != nil {
		t.Fatalf("runtime start: %v", err)
	}
	approval := waitACPEvent(t, ctx, running, func(event union.UnifiedExecutionEvent) bool {
		return event.Control != nil && event.Control.Kind == execution.ControlApprovalRequested
	})
	payload := json.RawMessage(`{"native_result":{"outcome":{"outcome":"selected","optionId":"allow_once"}}}`)
	if err := running.Command(ctx, union.ExecutionCommand{
		SemanticVersion: union.SemanticVersionV1, ExecutionID: invocation.Request.ExecutionID, Kind: union.CommandApproveAction,
		ExpectedExecutionStatus: "running", IdempotencyKey: "approve-1", ApprovalID: approval.Control.ApprovalID,
		ActionID: approval.Control.ActionID, MechanismAttemptID: approval.Control.MechanismAttemptID,
		InputDigest: approval.Control.InputDigest, ActionRevision: approval.Control.ActionRevision, Payload: payload,
	}); err != nil {
		t.Fatalf("runtime approve: %v", err)
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

func waitACPEvent(t *testing.T, ctx context.Context, running *execution.Execution, match func(union.UnifiedExecutionEvent) bool) union.UnifiedExecutionEvent {
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

func TestACPAdapterCancelEmitsStateMachineEvidence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	invocation := acpAdapterInvocation()
	adapter := newACPExecutionAdapter(t, "cancel")
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
	ready, err := session.Receive(ctx)
	if err != nil || ready.Diagnostic == nil || ready.Diagnostic.Kind != "native_extension" {
		t.Fatalf("ready event = %#v, %v", ready, err)
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

func acpManifestHasComponent(manifest union.ContextManifestSummary, kind, name string) bool {
	for _, component := range manifest.Components {
		if component.Kind == kind && component.Name == name {
			return true
		}
	}
	return false
}

func TestACPAdapterCancellationRunsThroughRuntime(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	invocation := acpAdapterInvocation()
	adapter := newACPExecutionAdapter(t, "cancel")
	registry := execution.NewRegistry()
	if err := registry.Register(ctx, adapter); err != nil {
		t.Fatal(err)
	}
	runtime, err := execution.NewRuntime(execution.RuntimeConfig{Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	running, err := runtime.Start(ctx, "acp-adapter-test", invocation)
	if err != nil {
		t.Fatal(err)
	}
	waitACPEvent(t, ctx, running, func(event union.UnifiedExecutionEvent) bool {
		return event.Diagnostic != nil && event.Diagnostic.Kind == "native_extension" && event.Diagnostic.Code == "test/prompt_received"
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
		t.Fatalf("cancel result = %s", result.Status)
	}
}

func TestACPPreflightRejectsAgentIdentityDrift(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	invocation := acpAdapterInvocation()
	config := acp.AdapterConfig{
		Identity: union.VersionedIdentity{ID: "acp-identity-test", Version: "v1"}, RouteID: "route-acp-test",
		Client: acp.Config{
			Process:          acpHelperProcessConfig(t, "lifecycle"),
			InitializeParams: json.RawMessage(`{"protocolVersion":1,"clientCapabilities":{},"clientInfo":{"name":"praxis-test","version":"v1"}}`),
		},
		ExpectedAgentName: "not-the-fake-agent", SessionOptions: json.RawMessage(`{"mcpServers":[]}`), ApprovalTTL: time.Minute,
	}
	adapter, err := acp.NewAdapter(config)
	if err != nil {
		t.Fatal(err)
	}
	report, err := adapter.Preflight(ctx, invocation)
	if err != nil || report.Accepted || report.RejectionCode != "acp_agent_identity_drift" {
		t.Fatalf("identity drift report = %#v, %v", report, err)
	}
	if _, err := adapter.Open(ctx, invocation); !errors.Is(err, acp.ErrPreparedNotFound) {
		t.Fatalf("rejected probe remained prepared: %v", err)
	}
}

func acpAdapterInvocation() execution.Invocation {
	profile := union.VersionedIdentity{ID: "profile-acp-test", Version: "v1"}
	route := union.VersionedIdentity{ID: "route-acp-test", Version: "v1"}
	graph := union.IntentGraph{Nodes: []union.IntentNode{{
		ID: "intent-acp-test", Kind: union.IntentModifyFile, Target: "/workspace/a.txt", Required: true,
		AcceptedFidelity: []union.SemanticFidelity{union.SemanticFidelityTransformed},
	}}}
	request := union.UnifiedExecutionRequest{
		SemanticVersion: union.SemanticVersionV1, ExecutionID: "exec-acp-adapter", ExecutionKind: union.ExecutionKindAgent,
		ProfileSelector:   union.ProfileSelector{Exact: &profile},
		Input:             []union.InputItem{{ID: "message-1", Kind: "message", Role: "user", Content: []union.ContentPart{{Kind: "text", Text: "edit a.txt"}}}},
		Instructions:      []union.Instruction{{ID: "instruction-1", Authority: "developer", Scope: "execution", ConflictPolicy: "higher_authority_wins", Content: []union.ContentPart{{Kind: "text", Text: "Use the Harness tools."}}}},
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
			ID: "plan-acp-test", IntentID: "intent-acp-test", Kind: "acp_session_prompt",
			Origin: union.CapabilityOriginHarnessHosted, Owner: union.ExecutionOwnerHarness, SelectionAuthority: union.SelectionAuthorityHarness,
			SemanticFidelity: union.SemanticFidelityTransformed,
		}},
		ExpectedManifest: union.ContextManifestSummary{ID: "expected-acp-manifest", Version: "v1", Mode: "harness"},
		RouteFingerprint: "sha256:acp-route",
	}
	invocation, err := execution.NewInvocation(request, plan)
	if err != nil {
		panic(err)
	}
	return invocation
}

func newACPExecutionAdapter(t *testing.T, mode string) *acp.Adapter {
	t.Helper()
	adapter, err := acp.NewAdapter(acp.AdapterConfig{
		Identity: union.VersionedIdentity{ID: "acp-adapter-test", Version: "v1"}, RouteID: "route-acp-test",
		Client: acp.Config{
			Process:          acpHelperProcessConfig(t, mode),
			InitializeParams: json.RawMessage(`{"protocolVersion":1,"clientCapabilities":{"fs":{"readTextFile":true,"writeTextFile":true}},"clientInfo":{"name":"praxis-test","version":"v1"}}`),
		},
		ExpectedAgentName: "fake-acp", SessionOptions: json.RawMessage(`{"mcpServers":[]}`), ApprovalTTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

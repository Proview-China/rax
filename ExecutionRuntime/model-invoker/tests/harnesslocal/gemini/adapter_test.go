package gemini_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/acp"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/gemini"
	harnessprocess "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/process"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func TestGeminiRuntimeEndTurnIsCandidateAndEffectsStayObserverOwned(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	invocation := geminiInvocation()
	adapter := newGeminiAdapter(t, "end-turn")
	registry := execution.NewRegistry()
	if err := registry.Register(ctx, adapter); err != nil {
		t.Fatal(err)
	}
	runtime, err := execution.NewRuntime(execution.RuntimeConfig{Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	running, err := runtime.Start(ctx, "gemini-acp-test", invocation)
	if err != nil {
		t.Fatal(err)
	}
	result, err := running.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != union.ExecutionStatusIndeterminate {
		t.Fatalf("unobserved Gemini Harness edit must remain indeterminate, got %s", result.Status)
	}
	if !geminiManifestComponent(result.ContextManifest, "gemini_first_user_session_context") || !containsString(result.ContextManifest.OpaqueFields, "instructions.gemini_first_user_session_context") {
		t.Fatalf("Gemini first-user context is absent from ActualManifest: %#v", result.ContextManifest)
	}
	var terminalCandidate bool
	for _, event := range running.Events() {
		if event.Effect != nil {
			t.Fatalf("Harness event crossed Effect authority: %#v", event)
		}
		terminalCandidate = terminalCandidate || event.Diagnostic != nil && event.Diagnostic.Kind == execution.EventKindRouteTerminalCandidate
	}
	if !terminalCandidate {
		t.Fatal("Gemini end_turn did not produce a route terminal candidate")
	}
}

func TestGeminiRuntimeCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	invocation := geminiInvocation()
	adapter := newGeminiAdapter(t, "cancel")
	registry := execution.NewRegistry()
	if err := registry.Register(ctx, adapter); err != nil {
		t.Fatal(err)
	}
	runtime, _ := execution.NewRuntime(execution.RuntimeConfig{Registry: registry})
	running, err := runtime.Start(ctx, "gemini-acp-test", invocation)
	if err != nil {
		t.Fatal(err)
	}
	waitGeminiEvent(t, ctx, running, func(event union.UnifiedExecutionEvent) bool {
		return event.Diagnostic != nil && event.Diagnostic.Code == "test/prompt_received"
	})
	if err := running.Command(ctx, union.ExecutionCommand{
		SemanticVersion: union.SemanticVersionV1, ExecutionID: invocation.Request.ExecutionID, Kind: union.CommandCancelExecution,
		ExpectedExecutionStatus: "running", IdempotencyKey: "cancel-gemini",
	}); err != nil {
		t.Fatal(err)
	}
	result, err := running.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != union.ExecutionStatusCancelled {
		t.Fatalf("cancel result = %s", result.Status)
	}
}

func TestGeminiRequiresExplicitACPAndFirstUserContext(t *testing.T) {
	config := geminiConfig(t, "end-turn")
	config.FirstUserSessionContext = false
	if _, err := gemini.New(config); err == nil {
		t.Fatal("adapter accepted an unacknowledged first-user session context")
	}
	config = geminiConfig(t, "end-turn")
	config.ACP.Client.Process.Arguments = []string{"-test.run=^TestGeminiACPProcessHelper$", "--", "end-turn"}
	if _, err := gemini.New(config); err == nil {
		t.Fatal("adapter accepted a process without --acp")
	}
}

func newGeminiAdapter(t *testing.T, mode string) *gemini.Adapter {
	t.Helper()
	adapter, err := gemini.New(geminiConfig(t, mode))
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func geminiConfig(t *testing.T, mode string) gemini.Config {
	t.Helper()
	return gemini.Config{
		ACP: acp.AdapterConfig{
			Identity: union.VersionedIdentity{ID: "gemini-acp-test", Version: "v1"}, RouteID: "google.gemini-code-assist.cli-acp",
			Client: acp.Config{
				Process:          geminiHelperProcessConfig(t, mode),
				InitializeParams: json.RawMessage(`{"protocolVersion":1,"clientCapabilities":{"fs":{"readTextFile":true,"writeTextFile":true}},"clientInfo":{"name":"praxis-test","version":"v1"}}`),
			},
			ExpectedAgentName: "gemini-cli", SessionOptions: json.RawMessage(`{"mcpServers":[]}`), ApprovalTTL: time.Minute,
		},
		FirstUserSessionContext: true,
	}
}

func geminiInvocation() execution.Invocation {
	profile := union.VersionedIdentity{ID: "google.gemini.cli-acp.semantic-stable", Version: "v1"}
	route := union.VersionedIdentity{ID: "google.gemini-code-assist.cli-acp", Version: "v1"}
	graph := union.IntentGraph{Nodes: []union.IntentNode{{
		ID: "intent-gemini", Kind: union.IntentModifyFile, Target: "/workspace/a.txt", Required: true,
		AcceptedFidelity: []union.SemanticFidelity{union.SemanticFidelityExact},
	}}}
	request := union.UnifiedExecutionRequest{
		SemanticVersion: union.SemanticVersionV1, ExecutionID: "exec-gemini-acp", ExecutionKind: union.ExecutionKindAgent,
		ProfileSelector: union.ProfileSelector{Exact: &profile},
		Input:           []union.InputItem{{ID: "message-1", Kind: "message", Role: "user", Content: []union.ContentPart{{Kind: "text", Text: "edit a.txt"}}}},
		Instructions:    []union.Instruction{{ID: "instruction-1", Authority: "developer", Scope: "execution", ConflictPolicy: "higher_authority_wins", Content: []union.ContentPart{{Kind: "text", Text: "Use Gemini CLI tools."}}}},
		ToolPolicy:      union.ToolPolicy{DefaultApproval: "on_side_effect", Parallelism: 1, MaxActions: 4},
		OutputContract:  union.OutputContract{AcceptedContentKinds: []string{"text"}, CompletionMode: "final"},
		SessionIntent:   union.SessionIntent{Mode: "new"}, ExecutionPolicy: union.ExecutionPolicy{Stream: true, UserPresence: "present", MaxConcurrency: 1},
		Budget: union.Budget{MaxWallTime: time.Minute, MaxToolActions: 4}, DegradationPolicy: union.DegradationPolicy{Default: union.DegradationDefaultReject}, IntentGraph: graph,
	}
	plan := union.PreparedExecutionPlan{
		SemanticVersion: union.SemanticVersionV1, ExecutionID: request.ExecutionID, Profile: profile, Route: route,
		ProfileKeyDigest: "sha256:gemini-profile", ExecutionKind: union.ExecutionKindAgent, IntentGraph: graph,
		Mechanisms: []union.MechanismPlan{{
			ID: "plan-gemini", IntentID: "intent-gemini", Kind: "gemini.replace", Origin: union.CapabilityOriginHarnessHosted,
			Owner: union.ExecutionOwnerHarness, SelectionAuthority: union.SelectionAuthorityHarness, SemanticFidelity: union.SemanticFidelityExact,
		}},
		ExpectedManifest: union.ContextManifestSummary{ID: "expected-gemini", Version: "v1", Mode: "harness"}, RouteFingerprint: "sha256:gemini-route",
	}
	invocation, err := execution.NewInvocation(request, plan)
	if err != nil {
		panic(err)
	}
	return invocation
}

func geminiHelperProcessConfig(t *testing.T, mode string) harnessprocess.Config {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	return harnessprocess.Config{
		Executable: executable, Arguments: []string{"-test.run=^TestGeminiACPProcessHelper$", "--", mode, "--acp"},
		WorkingDirectory: directory, AllowedWorkingDirectories: []string{directory}, Protocol: harnessprocess.ProtocolJSONRPCNDJSON,
		TerminationGrace: 100 * time.Millisecond, KillWait: time.Second,
	}
}

func waitGeminiEvent(t *testing.T, ctx context.Context, running *execution.Execution, match func(union.UnifiedExecutionEvent) bool) union.UnifiedExecutionEvent {
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
			t.Fatalf("event timeout: %v", ctx.Err())
		case <-ticker.C:
		}
	}
}

func geminiManifestComponent(manifest union.ContextManifestSummary, name string) bool {
	for _, component := range manifest.Components {
		if component.Name == name {
			return true
		}
	}
	return false
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func TestGeminiACPProcessHelper(t *testing.T) {
	mode := helperMode(os.Args)
	if mode == "" {
		return
	}
	os.Exit(runGeminiHelper(mode))
}

type rpcFrame struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
}

func runGeminiHelper(mode string) int {
	reader := bufio.NewReader(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	initialize, err := readMethod(reader, "initialize")
	if err != nil {
		return helperError(err)
	}
	if err := respond(encoder, initialize.ID, map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{}, "agentInfo": map[string]any{"name": "gemini-cli", "version": "test"}}); err != nil {
		return helperError(err)
	}
	newSession, err := readMethod(reader, "session/new")
	if err != nil {
		return helperError(err)
	}
	if err := respond(encoder, newSession.ID, map[string]any{"sessionId": "gemini-session"}); err != nil {
		return helperError(err)
	}
	prompt, err := readMethod(reader, "session/prompt")
	if err != nil {
		return helperError(err)
	}
	if mode == "cancel" {
		if err := notify(encoder, "test/prompt_received", map[string]any{"sessionId": "gemini-session"}); err != nil {
			return helperError(err)
		}
		if _, err := readMethod(reader, "session/cancel"); err != nil {
			return helperError(err)
		}
		if err := respond(encoder, prompt.ID, map[string]any{"stopReason": "cancelled"}); err != nil {
			return helperError(err)
		}
		return 0
	}
	updates := []any{
		map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "done"}},
		map[string]any{"sessionUpdate": "tool_call", "toolCallId": "gemini-edit-1", "kind": "edit", "status": "in_progress"},
		map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "gemini-edit-1", "kind": "edit", "status": "completed", "content": map[string]any{"type": "diff", "text": "diff"}},
	}
	for _, update := range updates {
		if err := notify(encoder, "session/update", map[string]any{"sessionId": "gemini-session", "update": update}); err != nil {
			return helperError(err)
		}
	}
	if err := respond(encoder, prompt.ID, map[string]any{"stopReason": "end_turn"}); err != nil {
		return helperError(err)
	}
	return 0
}

func helperMode(arguments []string) string {
	for index, argument := range arguments {
		if argument == "--" && index+1 < len(arguments) {
			return arguments[index+1]
		}
	}
	return ""
}

func readMethod(reader *bufio.Reader, expected string) (rpcFrame, error) {
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return rpcFrame{}, err
	}
	var frame rpcFrame
	if json.Unmarshal(line, &frame) != nil || frame.Method != expected {
		return rpcFrame{}, fmt.Errorf("method=%q, want %q", frame.Method, expected)
	}
	return frame, nil
}

func respond(encoder *json.Encoder, id json.RawMessage, result any) error {
	return encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func notify(encoder *json.Encoder, method string, params any) error {
	return encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func helperError(err error) int {
	_, _ = fmt.Fprintln(os.Stderr, err)
	return 51
}

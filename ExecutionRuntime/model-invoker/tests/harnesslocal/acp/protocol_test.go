package acp_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	acp "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/acp"
	harnessprocess "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/process"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func TestACPPromptStreamsPermissionAndMapping(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := startACP(t, ctx, "lifecycle")
	defer func() {
		if err := client.Close(); err != nil {
			t.Errorf("close client: %v", err)
		}
	}()
	session, info, err := client.NewSession(ctx, json.RawMessage(`{"cwd":"/workspace","mcpServers":[]}`))
	if err != nil || info.ID != "session-1" || session.ID() != "session-1" {
		t.Fatalf("new session = %#v, %v", info, err)
	}

	type promptOutcome struct {
		result acp.PromptResult
		err    error
	}
	outcome := make(chan promptOutcome, 1)
	go func() {
		result, promptErr := session.Prompt(ctx, json.RawMessage(`[{"type":"text","text":"edit"}]`))
		outcome <- promptOutcome{result: result, err: promptErr}
	}()

	mapper := newACPMapper(t)
	var sawMessage, sawThought, sawTool, sawUpdate, sawExtension, sawApproval, sawTerminal bool
	for index := 0; index < 7; index++ {
		native, receiveErr := session.Receive(ctx)
		if receiveErr != nil {
			t.Fatalf("receive %d: %v", index, receiveErr)
		}
		switch native.Kind {
		case acp.NativeAgentMessageChunk:
			sawMessage = true
		case acp.NativeAgentThoughtChunk:
			sawThought = true
		case acp.NativeToolCall:
			sawTool = true
		case acp.NativeToolCallUpdate:
			sawUpdate = true
		case acp.NativeExtension:
			sawExtension = true
		case acp.NativeApprovalRequest:
			sawApproval = true
			if err := session.RespondPermission(native, json.RawMessage(`{"outcome":{"outcome":"selected","optionId":"allow_once"}}`)); err != nil {
				t.Fatalf("respond permission: %v", err)
			}
		case acp.NativeTerminalCandidate:
			sawTerminal = true
			if native.Terminal == nil || native.Terminal.Status != union.ExecutionStatusSucceeded || native.Terminal.SideEffectState != union.SideEffectPossible {
				t.Fatalf("terminal candidate = %#v", native.Terminal)
			}
		}

		mapped, mapErr := mapper.Map(native)
		if mapErr != nil {
			t.Fatalf("map %s: %v", native.Kind, mapErr)
		}
		if mapped.Effect != nil {
			t.Fatalf("native event %s was incorrectly promoted to an Effect", native.Kind)
		}
		if native.Kind == acp.NativeToolCallUpdate && mapped.Item == nil {
			t.Fatalf("tool update, including its provisional diff, must remain Item evidence: %#v", mapped)
		}
		if native.Kind == acp.NativeAgentThoughtChunk && (mapped.Model == nil || mapped.Model.DisclosureClass != "provider_exposed_reasoning") {
			t.Fatalf("thought disclosure mapping = %#v", mapped)
		}
		if native.Kind == acp.NativeExtension && (mapped.Diagnostic == nil || mapped.Diagnostic.Kind != "native_extension") {
			t.Fatalf("unknown native update was not preserved: %#v", mapped)
		}
	}
	result := <-outcome
	if result.err != nil || result.result.StopReason != "end_turn" {
		t.Fatalf("prompt result = %#v, %v", result.result, result.err)
	}
	if !sawMessage || !sawThought || !sawTool || !sawUpdate || !sawExtension || !sawApproval || !sawTerminal {
		t.Fatalf("missing events: message=%v thought=%v tool=%v update=%v extension=%v approval=%v terminal=%v", sawMessage, sawThought, sawTool, sawUpdate, sawExtension, sawApproval, sawTerminal)
	}
}

func TestACPCancelProducesTerminalCandidate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := startACP(t, ctx, "cancel")
	defer client.Close()
	session, _, err := client.NewSession(ctx, json.RawMessage(`{"cwd":"/workspace"}`))
	if err != nil {
		t.Fatal(err)
	}
	type promptOutcome struct {
		result acp.PromptResult
		err    error
	}
	outcome := make(chan promptOutcome, 1)
	go func() {
		result, promptErr := session.Prompt(ctx, json.RawMessage(`[{"type":"text","text":"wait"}]`))
		outcome <- promptOutcome{result: result, err: promptErr}
	}()

	ready, err := session.Receive(ctx)
	if err != nil || ready.Kind != acp.NativeExtension || ready.Method != "test/prompt_received" {
		t.Fatalf("prompt readiness event = %#v, %v", ready, err)
	}
	if err := session.Cancel(); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	result := <-outcome
	if result.err != nil || result.result.StopReason != "cancelled" {
		t.Fatalf("cancel prompt result = %#v, %v", result.result, result.err)
	}
	terminal, err := session.Receive(ctx)
	if err != nil {
		t.Fatalf("receive terminal: %v", err)
	}
	if terminal.Kind != acp.NativeTerminalCandidate || terminal.Terminal == nil || terminal.Terminal.Status != union.ExecutionStatusCancelled {
		t.Fatalf("cancel terminal = %#v", terminal)
	}
}

func TestACPEOFWithoutStopReasonIsViolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := startACP(t, ctx, "eof")
	defer client.Close()
	session, _, err := client.NewSession(ctx, json.RawMessage(`{"cwd":"/workspace"}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = session.Prompt(ctx, json.RawMessage(`[{"type":"text","text":"wait"}]`))
	if !errors.Is(err, acp.ErrMissingTerminal) {
		t.Fatalf("EOF error = %v, want ErrMissingTerminal", err)
	}
}

func TestACPFailsClosedOnInvalidFrameOrResponseID(t *testing.T) {
	for _, mode := range []string{"bad-frame", "bad-id"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, err := acp.Start(ctx, acp.Config{
				Process:          acpHelperProcessConfig(t, mode),
				InitializeParams: json.RawMessage(`{"protocolVersion":1,"clientCapabilities":{},"clientInfo":{"name":"praxis-test","version":"v1"}}`),
			})
			if err == nil {
				t.Fatal("Start unexpectedly accepted an invalid protocol stream")
			}
		})
	}
}

func startACP(t *testing.T, ctx context.Context, mode string) *acp.Client {
	t.Helper()
	client, err := acp.Start(ctx, acp.Config{
		Process:          acpHelperProcessConfig(t, mode),
		InitializeParams: json.RawMessage(`{"protocolVersion":1,"clientCapabilities":{"fs":{"readTextFile":true,"writeTextFile":true}},"clientInfo":{"name":"praxis-test","version":"v1"}}`),
	})
	if err != nil {
		t.Fatalf("start ACP client: %v", err)
	}
	return client
}

func acpHelperProcessConfig(t *testing.T, mode string) harnessprocess.Config {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	return harnessprocess.Config{
		Executable:                executable,
		Arguments:                 []string{"-test.run=^TestACPProcessHelper$", "--", mode},
		WorkingDirectory:          directory,
		AllowedWorkingDirectories: []string{directory},
		Protocol:                  harnessprocess.ProtocolJSONRPCNDJSON,
		TerminationGrace:          100 * time.Millisecond,
		KillWait:                  time.Second,
	}
}

func newACPMapper(t *testing.T) *acp.Mapper {
	t.Helper()
	mapper, err := acp.NewMapper(acp.MappingContext{
		ExecutionID:        "exec-acp-test",
		Profile:            union.VersionedIdentity{ID: "profile-acp-test", Version: "v1"},
		Route:              union.VersionedIdentity{ID: "route-acp-test", Version: "v1"},
		IntentID:           "intent-acp-test",
		MechanismPlanID:    "plan-acp-test",
		MechanismAttemptID: "attempt-acp-test",
		ApprovalTTL:        time.Minute,
		Clock:              func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
	})
	if err != nil {
		t.Fatal(err)
	}
	return mapper
}

// TestACPProcessHelper is the explicitly configured fake ACP Agent process.
func TestACPProcessHelper(t *testing.T) {
	mode := acpProcessHelperMode(os.Args)
	if mode == "" {
		return
	}
	os.Exit(runACPProcessHelper(mode))
}

type rpcFrame struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Result  json.RawMessage `json:"result"`
}

func runACPProcessHelper(mode string) int {
	reader := bufio.NewReader(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	initialize, err := acpReadMethod(reader, "initialize")
	if err != nil {
		return acpHelperFailure(err)
	}
	if mode == "bad-frame" {
		_, _ = fmt.Fprintln(os.Stdout, `{not-json`)
		return 0
	}
	if mode == "bad-id" {
		_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": 999, "result": map[string]any{}})
		return 0
	}
	if err := acpRespond(encoder, initialize.ID, map[string]any{
		"protocolVersion": 1, "agentCapabilities": map[string]any{"promptCapabilities": map[string]any{"image": false}}, "agentInfo": map[string]any{"name": "fake-acp", "version": "v1"},
	}); err != nil {
		return acpHelperFailure(err)
	}
	newSession, err := acpReadMethod(reader, "session/new")
	if err != nil {
		return acpHelperFailure(err)
	}
	if err := acpRespond(encoder, newSession.ID, map[string]any{"sessionId": "session-1"}); err != nil {
		return acpHelperFailure(err)
	}
	prompt, err := acpReadMethod(reader, "session/prompt")
	if err != nil {
		return acpHelperFailure(err)
	}

	switch mode {
	case "eof":
		return 0
	case "cancel":
		if err := acpNotify(encoder, "test/prompt_received", map[string]any{"sessionId": "session-1"}); err != nil {
			return acpHelperFailure(err)
		}
		cancel, readErr := acpReadMethod(reader, "session/cancel")
		if readErr != nil {
			return acpHelperFailure(readErr)
		}
		if len(cancel.ID) != 0 {
			return acpHelperFailure(fmt.Errorf("session/cancel must be a notification, got id=%s", cancel.ID))
		}
		if err := acpRespond(encoder, prompt.ID, map[string]any{"stopReason": "cancelled"}); err != nil {
			return acpHelperFailure(err)
		}
		return 0
	case "lifecycle":
		return runACPLifecycle(reader, encoder, prompt.ID)
	default:
		return acpHelperFailure(fmt.Errorf("unknown helper mode %q", mode))
	}
}

func runACPLifecycle(reader *bufio.Reader, encoder *json.Encoder, promptID json.RawMessage) int {
	updates := []any{
		map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "hello"}},
		map[string]any{"sessionUpdate": "agent_thought_chunk", "content": map[string]any{"type": "text", "text": "provider summary"}},
		map[string]any{"sessionUpdate": "tool_call", "toolCallId": "tool-1", "title": "edit", "kind": "edit", "status": "in_progress", "rawInput": map[string]any{"path": "a.txt"}},
		map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "tool-1", "kind": "edit", "status": "completed", "content": map[string]any{"type": "diff", "text": "--- a/a.txt\n+++ b/a.txt\n@@ -0,0 +1 @@\n+hello"}},
		map[string]any{"sessionUpdate": "future_update", "opaque": map[string]any{"version": 2}},
	}
	for _, update := range updates {
		if err := acpNotify(encoder, "session/update", map[string]any{"sessionId": "session-1", "update": update}); err != nil {
			return acpHelperFailure(err)
		}
	}
	if err := acpRequest(encoder, json.RawMessage(`"permission-1"`), "session/request_permission", map[string]any{
		"sessionId": "session-1", "toolCallId": "tool-1", "options": []any{map[string]any{"optionId": "allow_once", "name": "Allow once", "kind": "allow_once"}},
	}); err != nil {
		return acpHelperFailure(err)
	}
	permission, err := acpReadResponse(reader, `"permission-1"`)
	if err != nil || !strings.Contains(string(permission.Result), "allow_once") {
		return acpHelperFailure(fmt.Errorf("permission response: %w; result=%s", err, permission.Result))
	}
	if err := acpRespond(encoder, promptID, map[string]any{"stopReason": "end_turn"}); err != nil {
		return acpHelperFailure(err)
	}
	return 0
}

func acpProcessHelperMode(arguments []string) string {
	for index, argument := range arguments {
		if argument == "--" && index+1 < len(arguments) {
			return arguments[index+1]
		}
	}
	return ""
}

func acpReadMethod(reader *bufio.Reader, expected string) (rpcFrame, error) {
	frame, err := acpReadFrame(reader)
	if err != nil {
		return rpcFrame{}, err
	}
	if frame.Method != expected {
		return rpcFrame{}, fmt.Errorf("method = %q, want %q", frame.Method, expected)
	}
	return frame, nil
}

func acpReadResponse(reader *bufio.Reader, expectedID string) (rpcFrame, error) {
	frame, err := acpReadFrame(reader)
	if err != nil {
		return rpcFrame{}, err
	}
	if frame.Method != "" || string(frame.ID) != expectedID {
		return rpcFrame{}, fmt.Errorf("response id=%s method=%q, want id=%s", frame.ID, frame.Method, expectedID)
	}
	return frame, nil
}

func acpReadFrame(reader *bufio.Reader) (rpcFrame, error) {
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return rpcFrame{}, err
	}
	var frame rpcFrame
	if err := json.Unmarshal(line, &frame); err != nil {
		return rpcFrame{}, err
	}
	return frame, nil
}

func acpRespond(encoder *json.Encoder, id json.RawMessage, result any) error {
	return encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func acpNotify(encoder *json.Encoder, method string, params any) error {
	return encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func acpRequest(encoder *json.Encoder, id json.RawMessage, method string, params any) error {
	return encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
}

func acpHelperFailure(err error) int {
	_, _ = fmt.Fprintln(os.Stderr, err)
	return 41
}

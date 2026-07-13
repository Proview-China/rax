package codex_test

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

	codex "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/codexappserver"
	harnessprocess "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/process"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func TestCodexLifecycleApprovalToolAndMapping(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := startCodex(t, ctx, "lifecycle")
	defer func() {
		if err := client.Close(); err != nil {
			t.Errorf("close client: %v", err)
		}
	}()

	thread, err := client.StartThread(ctx, json.RawMessage(`{"cwd":"/workspace","approvalPolicy":"on-request"}`))
	if err != nil || thread.ID != "thread-1" {
		t.Fatalf("start thread = %#v, %v", thread, err)
	}
	turn, err := client.StartTurn(ctx, json.RawMessage(`{"threadId":"thread-1","input":[{"type":"text","text":"edit"}]}`))
	if err != nil || turn.ID != "turn-1" {
		t.Fatalf("start turn = %#v, %v", turn, err)
	}

	mapper := newCodexMapper(t)
	var sawMessage, sawReasoning, sawDiff, sawApproval, sawTool, sawExtension, sawTerminal bool
	for index := 0; index < 11; index++ {
		native, receiveErr := client.Receive(ctx)
		if receiveErr != nil {
			t.Fatalf("receive %d: %v", index, receiveErr)
		}
		switch native.Kind {
		case codex.NativeApprovalRequest:
			sawApproval = true
			if err := client.RespondApproval(native, json.RawMessage(`{"decision":"accept"}`)); err != nil {
				t.Fatalf("respond approval: %v", err)
			}
		case codex.NativeDynamicToolRequest:
			sawTool = true
			if err := client.RespondDynamicTool(native, json.RawMessage(`{"success":true,"contentItems":[{"type":"inputText","text":"ok"}]}`)); err != nil {
				t.Fatalf("respond tool: %v", err)
			}
		case codex.NativeProvisionalDiff:
			sawDiff = true
		case codex.NativeExtension:
			sawExtension = true
		case codex.NativeTerminalCandidate:
			sawTerminal = true
			if native.Terminal == nil || native.Terminal.SideEffectState != union.SideEffectPossible {
				t.Fatalf("terminal candidate did not preserve possible side effects: %#v", native.Terminal)
			}
		}

		mapped, mapErr := mapper.Map(native)
		if mapErr != nil {
			t.Fatalf("map %s: %v", native.Kind, mapErr)
		}
		if mapped.Effect != nil {
			t.Fatalf("native event %s was incorrectly promoted to an Effect", native.Kind)
		}
		if native.Kind == codex.NativeProvisionalDiff {
			if mapped.Item == nil || mapped.Item.Item.Kind != "provisional_diff" {
				t.Fatalf("provisional diff mapping = %#v", mapped)
			}
		}
		if native.Kind == codex.NativeExtension && (mapped.Diagnostic == nil || mapped.Diagnostic.Kind != "native_extension") {
			t.Fatalf("unknown native event not preserved: %#v", mapped)
		}
		if native.Method == "item/agentMessage/delta" {
			sawMessage = mapped.Model != nil && mapped.Header.Visibility == union.VisibilityUserVisible
		}
		if native.Method == "item/reasoning/summaryTextDelta" {
			sawReasoning = mapped.Model != nil && mapped.Model.DisclosureClass == "provider_exposed_reasoning"
		}
	}
	if !sawMessage || !sawReasoning || !sawDiff || !sawApproval || !sawTool || !sawExtension || !sawTerminal {
		t.Fatalf("missing expected events: message=%v reasoning=%v diff=%v approval=%v tool=%v extension=%v terminal=%v", sawMessage, sawReasoning, sawDiff, sawApproval, sawTool, sawExtension, sawTerminal)
	}
}

func TestCodexInterruptProducesTerminalCandidate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := startCodex(t, ctx, "interrupt")
	defer client.Close()
	if _, err := client.StartThread(ctx, json.RawMessage(`{"cwd":"/workspace"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := client.StartTurn(ctx, json.RawMessage(`{"threadId":"thread-1","input":[]}`)); err != nil {
		t.Fatal(err)
	}
	if err := client.Interrupt(ctx); err != nil {
		t.Fatalf("interrupt: %v", err)
	}
	for index := 0; index < 2; index++ {
		event, err := client.Receive(ctx)
		if err != nil {
			t.Fatalf("receive terminal: %v", err)
		}
		if event.Kind == codex.NativeTerminalCandidate {
			if event.Terminal == nil || event.Terminal.Status != union.ExecutionStatusCancelled {
				t.Fatalf("interrupt terminal = %#v", event)
			}
			return
		}
	}
	t.Fatal("interrupt did not produce a terminal candidate")
}

func TestCodexEOFWithoutTurnCompletedIsViolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := startCodex(t, ctx, "eof")
	defer client.Close()
	if _, err := client.StartThread(ctx, json.RawMessage(`{"cwd":"/workspace"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := client.StartTurn(ctx, json.RawMessage(`{"threadId":"thread-1","input":[]}`)); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 2; index++ {
		_, err := client.Receive(ctx)
		if errors.Is(err, codex.ErrMissingTerminal) {
			return
		}
		if err != nil {
			t.Fatalf("receive before EOF = %v", err)
		}
	}
	t.Fatal("stream ended without reporting ErrMissingTerminal")
}

func TestCodexFailsClosedOnInvalidFrameOrResponseID(t *testing.T) {
	for _, mode := range []string{"bad-frame", "bad-id"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, err := codex.Start(ctx, codex.Config{
				Process:    helperProcessConfig(t, mode),
				ClientInfo: codex.ClientInfo{Name: "praxis-test", Version: "v1"},
			})
			if err == nil {
				t.Fatal("Start unexpectedly accepted an invalid protocol stream")
			}
		})
	}
}

func startCodex(t *testing.T, ctx context.Context, mode string) *codex.Client {
	t.Helper()
	client, err := codex.Start(ctx, codex.Config{
		Process:      helperProcessConfig(t, mode),
		ClientInfo:   codex.ClientInfo{Name: "praxis-test", Title: "Praxis Test", Version: "v1"},
		Capabilities: json.RawMessage(`{"experimentalApi":true}`),
	})
	if err != nil {
		t.Fatalf("start codex client: %v", err)
	}
	return client
}

func helperProcessConfig(t *testing.T, mode string) harnessprocess.Config {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	return harnessprocess.Config{
		Executable:                executable,
		Arguments:                 []string{"-test.run=^TestCodexProcessHelper$", "--", mode},
		WorkingDirectory:          directory,
		AllowedWorkingDirectories: []string{directory},
		Protocol:                  harnessprocess.ProtocolCodexAppServer,
		TerminationGrace:          100 * time.Millisecond,
		KillWait:                  time.Second,
	}
}

func newCodexMapper(t *testing.T) *codex.Mapper {
	t.Helper()
	mapper, err := codex.NewMapper(codex.MappingContext{
		ExecutionID:        "exec-codex-test",
		Profile:            union.VersionedIdentity{ID: "profile-codex-test", Version: "v1"},
		Route:              union.VersionedIdentity{ID: "route-codex-test", Version: "v1"},
		IntentID:           "intent-codex-test",
		MechanismPlanID:    "plan-codex-test",
		MechanismAttemptID: "attempt-codex-test",
		ApprovalTTL:        time.Minute,
		Clock:              func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
	})
	if err != nil {
		t.Fatal(err)
	}
	return mapper
}

// TestCodexProcessHelper is executed as the explicitly configured Harness
// executable. It never discovers or invokes a real Codex binary.
func TestCodexProcessHelper(t *testing.T) {
	mode := processHelperMode(os.Args)
	if mode == "" {
		return
	}
	os.Exit(runCodexProcessHelper(mode))
}

type rpcFrame struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Result  json.RawMessage `json:"result"`
}

func runCodexProcessHelper(mode string) int {
	reader := bufio.NewReader(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	initialize, err := readMethod(reader, "initialize")
	if err != nil {
		return helperFailure(err)
	}
	if mode == "bad-frame" {
		_, _ = fmt.Fprintln(os.Stdout, `{not-json`)
		return 0
	}
	if mode == "bad-id" {
		_ = encoder.Encode(map[string]any{"id": 999, "result": map[string]any{}})
		return 0
	}
	if err := respond(encoder, initialize.ID, map[string]any{"userAgent": "fake-codex"}); err != nil {
		return helperFailure(err)
	}
	if _, err := readMethod(reader, "initialized"); err != nil {
		return helperFailure(err)
	}
	threadStart, err := readMethod(reader, "thread/start")
	if err != nil {
		return helperFailure(err)
	}
	if err := respond(encoder, threadStart.ID, map[string]any{"thread": map[string]any{"id": "thread-1"}}); err != nil {
		return helperFailure(err)
	}
	if err := notify(encoder, "thread/started", map[string]any{"thread": map[string]any{"id": "thread-1"}}); err != nil {
		return helperFailure(err)
	}
	turnStart, err := readMethod(reader, "turn/start")
	if err != nil {
		return helperFailure(err)
	}
	if err := respond(encoder, turnStart.ID, map[string]any{"turn": map[string]any{"id": "turn-1", "status": "inProgress"}}); err != nil {
		return helperFailure(err)
	}

	switch mode {
	case "eof":
		return 0
	case "interrupt":
		interrupt, readErr := readMethod(reader, "turn/interrupt")
		if readErr != nil {
			return helperFailure(readErr)
		}
		if err := respond(encoder, interrupt.ID, map[string]any{}); err != nil {
			return helperFailure(err)
		}
		if err := notify(encoder, "turn/completed", map[string]any{"threadId": "thread-1", "turn": map[string]any{"id": "turn-1", "status": "interrupted"}}); err != nil {
			return helperFailure(err)
		}
		return 0
	case "lifecycle":
		return runCodexLifecycle(reader, encoder)
	default:
		return helperFailure(fmt.Errorf("unknown helper mode %q", mode))
	}
}

func runCodexLifecycle(reader *bufio.Reader, encoder *json.Encoder) int {
	frames := []struct {
		method string
		params any
	}{
		{"turn/started", map[string]any{"threadId": "thread-1", "turn": map[string]any{"id": "turn-1", "status": "inProgress"}}},
		{"item/agentMessage/delta", map[string]any{"threadId": "thread-1", "turnId": "turn-1", "itemId": "message-1", "delta": "hello"}},
		{"item/reasoning/summaryTextDelta", map[string]any{"threadId": "thread-1", "turnId": "turn-1", "itemId": "reasoning-1", "delta": "provider summary"}},
		{"item/started", map[string]any{"threadId": "thread-1", "turnId": "turn-1", "item": map[string]any{"id": "item-file-1", "type": "fileChange", "status": "inProgress"}}},
		{"turn/diff/updated", map[string]any{"threadId": "thread-1", "turnId": "turn-1", "diff": "--- a/a.txt\n+++ b/a.txt\n@@ -0,0 +1 @@\n+hello"}},
	}
	for _, frame := range frames {
		if err := notify(encoder, frame.method, frame.params); err != nil {
			return helperFailure(err)
		}
	}
	if err := request(encoder, json.RawMessage(`"approval-1"`), "item/fileChange/requestApproval", map[string]any{
		"threadId": "thread-1", "turnId": "turn-1", "itemId": "item-file-1", "reason": "write",
	}); err != nil {
		return helperFailure(err)
	}
	approval, err := readResponse(reader, `"approval-1"`)
	if err != nil || !strings.Contains(string(approval.Result), "accept") {
		return helperFailure(fmt.Errorf("approval response: %w; result=%s", err, approval.Result))
	}
	if err := request(encoder, json.RawMessage(`"tool-1"`), "item/tool/call", map[string]any{
		"threadId": "thread-1", "turnId": "turn-1", "callId": "dynamic-1", "tool": "lookup", "arguments": map[string]any{"q": "x"},
	}); err != nil {
		return helperFailure(err)
	}
	tool, err := readResponse(reader, `"tool-1"`)
	if err != nil || !strings.Contains(string(tool.Result), "success") {
		return helperFailure(fmt.Errorf("tool response: %w; result=%s", err, tool.Result))
	}
	remaining := []struct {
		method string
		params any
	}{
		{"item/completed", map[string]any{"threadId": "thread-1", "turnId": "turn-1", "item": map[string]any{"id": "item-file-1", "type": "fileChange", "status": "completed"}}},
		{"future/nativeEvent", map[string]any{"threadId": "thread-1", "turnId": "turn-1", "opaque": map[string]any{"v": 1}}},
		{"turn/completed", map[string]any{"threadId": "thread-1", "turn": map[string]any{"id": "turn-1", "status": "completed"}}},
	}
	for _, frame := range remaining {
		if err := notify(encoder, frame.method, frame.params); err != nil {
			return helperFailure(err)
		}
	}
	return 0
}

func processHelperMode(arguments []string) string {
	for index, argument := range arguments {
		if argument == "--" && index+1 < len(arguments) {
			return arguments[index+1]
		}
	}
	return ""
}

func readMethod(reader *bufio.Reader, expected string) (rpcFrame, error) {
	frame, err := readFrame(reader)
	if err != nil {
		return rpcFrame{}, err
	}
	if frame.Method != expected {
		return rpcFrame{}, fmt.Errorf("method = %q, want %q", frame.Method, expected)
	}
	return frame, nil
}

func readResponse(reader *bufio.Reader, expectedID string) (rpcFrame, error) {
	frame, err := readFrame(reader)
	if err != nil {
		return rpcFrame{}, err
	}
	if frame.Method != "" || string(frame.ID) != expectedID {
		return rpcFrame{}, fmt.Errorf("response id=%s method=%q, want id=%s", frame.ID, frame.Method, expectedID)
	}
	return frame, nil
}

func readFrame(reader *bufio.Reader) (rpcFrame, error) {
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return rpcFrame{}, err
	}
	var frame rpcFrame
	if err := json.Unmarshal(line, &frame); err != nil {
		return rpcFrame{}, err
	}
	if frame.JSONRPC != "" {
		return rpcFrame{}, fmt.Errorf("Codex App Server frame unexpectedly declares jsonrpc=%q", frame.JSONRPC)
	}
	return frame, nil
}

func respond(encoder *json.Encoder, id json.RawMessage, result any) error {
	return encoder.Encode(map[string]any{"id": id, "result": result})
}

func notify(encoder *json.Encoder, method string, params any) error {
	return encoder.Encode(map[string]any{"method": method, "params": params})
}

func request(encoder *json.Encoder, id json.RawMessage, method string, params any) error {
	return encoder.Encode(map[string]any{"id": id, "method": method, "params": params})
}

func helperFailure(err error) int {
	_, _ = fmt.Fprintln(os.Stderr, err)
	return 31
}

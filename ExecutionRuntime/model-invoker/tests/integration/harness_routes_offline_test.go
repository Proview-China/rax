//go:build integration

package integration_test

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/profile"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

const offlineHarnessMarker = `{"marker":"praxis-offline-harness"}`

// TestOfficialHarnessAdaptersOfflineEndToEnd is the shared black-box contract
// for every official Harness route. Each case uses the production Adapter and
// process boundary, while this test binary acts as the pinned protocol peer.
// Two executions run concurrently through the same Adapter so prepared
// sessions, native session identities, manifests, attempts and terminal state
// cannot accidentally bleed across ExecutionIDs.
func TestOfficialHarnessAdaptersOfflineEndToEnd(t *testing.T) {
	now := time.Date(2026, 7, 13, 5, 0, 0, 0, time.UTC)
	profiles, err := profile.RepresentativeProfiles(now)
	if err != nil {
		t.Fatal(err)
	}
	profilesByID := make(map[profile.ProfileID]profile.SemanticRouteProfile, len(profiles))
	for _, selected := range profiles {
		profilesByID[selected.ID] = selected
	}
	setOfflineHarnessBuilderEnvironment(t)

	cases := []struct {
		liveHarnessCase
		arguments []string
	}{
		{liveHarnessCase: liveHarnessCase{name: "codex_app_server", prefix: "PRAXIS_CODEX_HARNESS_", profileID: profile.ProfileCodex}, arguments: []string{"-test.run=^TestOfficialHarnessOfflineProcess$", "--", "codex", "app-server"}},
		{liveHarnessCase: liveHarnessCase{name: "claude_sdk_cli", prefix: "PRAXIS_CLAUDE_HARNESS_", profileID: profile.ProfileClaudeSDK}, arguments: []string{"-test.run=^TestOfficialHarnessOfflineProcess$", "--", "claude", "--output-format", "stream-json"}},
		{liveHarnessCase: liveHarnessCase{name: "gemini_acp", prefix: "PRAXIS_GEMINI_HARNESS_", profileID: profile.ProfileGeminiCLI}, arguments: []string{"-test.run=^TestOfficialHarnessOfflineProcess$", "--", "gemini", "--acp"}},
		{liveHarnessCase: liveHarnessCase{name: "kimi_current_acp", prefix: "PRAXIS_KIMI_HARNESS_", profileID: profile.ProfileKimiCLI}, arguments: []string{"-test.run=^TestOfficialHarnessOfflineProcess$", "--", "kimi", "acp"}},
		{liveHarnessCase: liveHarnessCase{name: "qwen_sdk_cli", prefix: "PRAXIS_QWEN_HARNESS_", profileID: profile.ProfileQwenSDK}, arguments: []string{"-test.run=^TestOfficialHarnessOfflineProcess$", "--", "qwen", "--bare"}},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			selected := profilesByID[test.profileID]
			input := offlineHarnessInput(t, selected, test.arguments)
			adapter, _, err := buildOfficialHarnessAdapter(test.liveHarnessCase, input, selected)
			if err != nil {
				t.Fatalf("construct production Adapter: %v", err)
			}
			descriptor, err := adapter.Describe(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			registry := execution.NewRegistry()
			if err := registry.Register(context.Background(), adapter); err != nil {
				t.Fatal(err)
			}
			runtime, err := execution.NewRuntime(execution.RuntimeConfig{
				Registry: registry, Reconciler: offlineHarnessReconciler{}, Verifier: offlineHarnessVerifier{},
			})
			if err != nil {
				t.Fatal(err)
			}

			base, err := buildHarnessSmokeInvocation(now, selected, input)
			if err != nil {
				t.Fatal(err)
			}
			invocations := make([]execution.Invocation, 2)
			for index := range invocations {
				invocations[index], err = cloneOfflineHarnessInvocation(base, index+1)
				if err != nil {
					t.Fatal(err)
				}
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			results := make([]union.UnifiedExecutionResult, len(invocations))
			errorsByIndex := make([]error, len(invocations))
			executions := make([]*execution.Execution, len(invocations))
			var wait sync.WaitGroup
			for index := range invocations {
				index := index
				wait.Add(1)
				go func() {
					defer wait.Done()
					executions[index], errorsByIndex[index] = runtime.Start(ctx, descriptor.Identity.ID, invocations[index])
					if errorsByIndex[index] == nil {
						results[index], errorsByIndex[index] = executions[index].Wait(ctx)
					}
				}()
			}
			wait.Wait()

			attemptIDs := make(map[union.MechanismAttemptID]struct{}, len(results))
			nativeSessions := make(map[union.SessionID]struct{}, len(results))
			for index, result := range results {
				if errorsByIndex[index] != nil {
					t.Fatalf("execution %d: %v; events=%s", index, errorsByIndex[index], offlineHarnessEventSummary(executions[index]))
				}
				if err := result.Validate(); err != nil {
					t.Fatalf("result %d: %v", index, err)
				}
				if result.ExecutionID != invocations[index].Request.ExecutionID {
					t.Fatalf("execution %d identity crossed sessions: %#v", index, result)
				}
				if result.Status != union.ExecutionStatusSucceeded || result.VerificationStatus != union.VerificationVerified || len(result.Effects) != 1 || len(result.MechanismTrace) == 0 {
					t.Fatalf("execution %d did not complete the full semantic lifecycle: status=%s verification=%s effects=%d attempts=%d satisfaction=%#v residuals=%#v error=%#v events=%s", index, result.Status, result.VerificationStatus, len(result.Effects), len(result.MechanismTrace), result.IntentSatisfaction, result.Residuals, result.Error, offlineHarnessEventSummary(executions[index]))
				}
				attemptID := result.Effects[0].MechanismAttemptID
				if _, duplicate := attemptIDs[attemptID]; duplicate {
					t.Fatalf("execution %d reused a mechanism attempt from another session", index)
				}
				attemptIDs[attemptID] = struct{}{}
				nativeSession := offlineHarnessNativeSession(executions[index])
				if nativeSession == "" {
					t.Fatalf("execution %d lost its native Harness session identity", index)
				}
				if _, duplicate := nativeSessions[nativeSession]; duplicate {
					t.Fatalf("execution %d reused a native Harness session identity", index)
				}
				nativeSessions[nativeSession] = struct{}{}
				if !strings.Contains(result.ContextManifest.ID, string(test.profileID)) {
					t.Fatalf("execution %d manifest lost its selected Profile: %q", index, result.ContextManifest.ID)
				}
			}
		})
	}
}

func setOfflineHarnessBuilderEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("PRAXIS_CLAUDE_HARNESS_INITIALIZE_JSON", `{"subtype":"initialize","hooks":null,"agents":{}}`)
	t.Setenv("PRAXIS_CLAUDE_HARNESS_EXPECTED_INIT_JSON", `{"tools":["Bash","Edit","Read","Write"],"mcp_servers":[],"permission_mode":"default","api_key_source":"none"}`)
	t.Setenv("PRAXIS_GEMINI_HARNESS_INITIALIZE_JSON", `{"protocolVersion":1,"clientCapabilities":{},"clientInfo":{"name":"praxis-offline","version":"v1"}}`)
	t.Setenv("PRAXIS_GEMINI_HARNESS_SESSION_JSON", `{"model":"gemini-3.5-flash","mcpServers":[]}`)
	t.Setenv("PRAXIS_GEMINI_HARNESS_AGENT_NAME", "gemini-cli")
	t.Setenv("PRAXIS_KIMI_HARNESS_INITIALIZE_JSON", `{"protocolVersion":1,"clientCapabilities":{},"clientInfo":{"name":"praxis-offline","version":"v1"}}`)
	t.Setenv("PRAXIS_KIMI_HARNESS_SESSION_JSON", `{"model":"kimi-for-coding","mcpServers":[]}`)
	t.Setenv("PRAXIS_KIMI_HARNESS_AGENT_NAME", "kimi-code")
	t.Setenv("PRAXIS_QWEN_HARNESS_INITIALIZE_JSON", `{"subtype":"initialize","hooks":null,"mcpServers":{},"agents":[]}`)
	t.Setenv("PRAXIS_QWEN_HARNESS_EXPECTED_INIT_JSON", `{"tools":["edit","read_file","run_shell_command"],"mcp_servers":[],"permission_mode":"default","agents":[],"skills":[],"surface_mode":"bare_fixed","core_tools":[],"exclude_tools":["notebook_edit"]}`)
}

func offlineHarnessInput(t *testing.T, selected profile.SemanticRouteProfile, arguments []string) commonHarnessInput {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	digest, err := digestOfflineHarnessFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	return commonHarnessInput{
		Executable: executable, ResolvedExecutable: executable, ExecutableSHA256: digest,
		CWD: directory, ResolvedCWD: directory, Home: directory,
		Model: selected.Selection.ModelID, Version: "offline-fake-v1", Arguments: append([]string(nil), arguments...),
	}
}

func digestOfflineHarnessFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data)), nil
}

func cloneOfflineHarnessInvocation(base execution.Invocation, serial int) (execution.Invocation, error) {
	clone, err := base.Clone()
	if err != nil {
		return execution.Invocation{}, err
	}
	executionID := union.ExecutionID(fmt.Sprintf("offline.harness.%s.%d", clone.Plan.Profile.ID, serial))
	clone.Request.ExecutionID = executionID
	clone.Plan.ExecutionID = executionID
	clone.Plan.Digest = ""
	delete(clone.Plan.Metadata, "request_digest")
	return execution.NewInvocation(clone.Request, clone.Plan)
}

func offlineHarnessEventSummary(running *execution.Execution) string {
	if running == nil {
		return "<not-started>"
	}
	parts := make([]string, 0)
	for _, event := range running.Events() {
		kind := ""
		switch {
		case event.Lifecycle != nil:
			kind = event.Lifecycle.Kind + ":" + string(event.Lifecycle.Status)
		case event.Mechanism != nil:
			kind = event.Mechanism.Kind
		case event.Model != nil:
			kind = event.Model.Kind
		case event.Diagnostic != nil:
			kind = event.Diagnostic.Kind + ":" + event.Diagnostic.Code
		case event.Effect != nil:
			kind = event.Effect.Kind
		}
		parts = append(parts, fmt.Sprintf("%d/%s/%s", event.Header.Sequence, event.Header.Family, kind))
	}
	return strings.Join(parts, ",")
}

func offlineHarnessNativeSession(running *execution.Execution) union.SessionID {
	if running == nil {
		return ""
	}
	for _, event := range running.Events() {
		if event.Header.Origin == union.EventOriginHarness && event.Header.SessionID != "" {
			return event.Header.SessionID
		}
	}
	return ""
}

type offlineHarnessReconciler struct{}

func (offlineHarnessReconciler) Reconcile(_ context.Context, input execution.ReconcileInput) (execution.ReconcileReport, error) {
	var output strings.Builder
	var completed *union.MechanismAttempt
	for _, event := range input.Events {
		if event.Model != nil {
			for _, part := range event.Model.Content {
				if part.Kind == "text" {
					output.WriteString(part.Text)
				}
			}
		}
		if event.Mechanism != nil && event.Mechanism.Attempt != nil && event.Mechanism.Attempt.Status == union.AttemptStatusCompleted {
			candidate := *event.Mechanism.Attempt
			completed = &candidate
		}
	}
	if strings.TrimSpace(output.String()) != offlineHarnessMarker {
		return execution.ReconcileReport{}, fmt.Errorf("offline Harness marker was not preserved")
	}
	if completed == nil {
		return execution.ReconcileReport{}, fmt.Errorf("completed Harness attempt is missing")
	}
	intent := input.Invocation.Plan.IntentGraph.Nodes[0]
	mechanism := primaryOfflineHarnessMechanism(input.Invocation.Plan, intent.ID)
	structuredMechanism := union.StructuredStrictJSONSchema
	if mechanism.Origin == union.CapabilityOriginEmulated {
		structuredMechanism = union.StructuredEmulatedSchema
	}
	effect := union.EffectRecord{
		ID: union.EffectID("effect." + string(input.Invocation.Request.ExecutionID)), IntentIDs: []union.IntentID{intent.ID},
		MechanismAttemptID: completed.ID, Kind: "structured_output_produced", Target: intent.Target,
		ObservationSource: "offline.harness.protocol-observer", VerificationStatus: union.VerificationUnverified,
		VerificationRefs: []union.VerificationID{union.VerificationID("verify." + string(input.Invocation.Request.ExecutionID))},
		OccurredAt:       time.Date(2026, 7, 13, 5, 0, 1, 0, time.UTC),
		Payload: union.EffectPayload{StructuredOutput: &union.StructuredOutputEffect{
			Mechanism: structuredMechanism, Origin: mechanism.Origin, Fidelity: mechanism.SemanticFidelity,
			Parsed: json.RawMessage(offlineHarnessMarker), SchemaDigest: "sha256:offline-harness-schema",
			JSONValid: true, SchemaValid: true, FinalDigest: "sha256:offline-harness-output",
		}},
	}
	return execution.ReconcileReport{Effects: []union.EffectRecord{effect}, SideEffectState: union.SideEffectObserved, Quiesced: true}, nil
}

func primaryOfflineHarnessMechanism(plan union.PreparedExecutionPlan, intentID union.IntentID) union.MechanismPlan {
	var selected union.MechanismPlan
	for _, candidate := range plan.Mechanisms {
		if candidate.IntentID != intentID {
			continue
		}
		if selected.ID == "" || candidate.PreferredRank < selected.PreferredRank || candidate.PreferredRank == selected.PreferredRank && candidate.ID < selected.ID {
			selected = candidate
		}
	}
	return selected
}

type offlineHarnessVerifier struct{}

func (offlineHarnessVerifier) Verify(_ context.Context, input execution.VerifyInput) (execution.VerificationReport, error) {
	if len(input.Effects) != 1 || input.Effects[0].Payload.StructuredOutput == nil || string(input.Effects[0].Payload.StructuredOutput.Parsed) != offlineHarnessMarker {
		return execution.VerificationReport{}, fmt.Errorf("offline Harness Effect is not the reviewed structured marker")
	}
	return execution.VerificationReport{Verifications: []union.VerificationRecord{{
		ID:        union.VerificationID("verify." + string(input.Invocation.Request.ExecutionID)),
		EffectIDs: []union.EffectID{input.Effects[0].ID}, IntentIDs: append([]union.IntentID(nil), input.Effects[0].IntentIDs...),
		Kind: "offline_harness_marker", Status: union.VerificationVerified,
		Verifier:    union.VersionedIdentity{ID: "offline.harness.verifier", Version: "v1"},
		CompletedAt: time.Date(2026, 7, 13, 5, 0, 2, 0, time.UTC),
	}}}, nil
}

// TestOfficialHarnessOfflineProcess is re-executed as the exact fake binary.
// No real CLI, login state, Provider endpoint or inherited environment is used.
func TestOfficialHarnessOfflineProcess(t *testing.T) {
	route := offlineHarnessRoute(os.Args)
	if route == "" {
		return
	}
	var code int
	switch route {
	case "codex":
		code = runOfflineCodex(os.Stdin, os.Stdout)
	case "claude", "qwen":
		code = runOfflineStreamJSON(route, os.Stdin, os.Stdout)
	case "gemini", "kimi":
		code = runOfflineACP(route, os.Stdin, os.Stdout)
	default:
		code = 91
	}
	os.Exit(code)
}

func offlineHarnessRoute(arguments []string) string {
	for index, argument := range arguments {
		if argument == "--" && index+1 < len(arguments) {
			return arguments[index+1]
		}
	}
	return ""
}

type offlineRPCFrame struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
}

func runOfflineCodex(input io.Reader, output io.Writer) int {
	reader := bufio.NewReader(input)
	encoder := json.NewEncoder(output)
	threadID := fmt.Sprintf("offline-thread-%d", os.Getpid())
	turnID := fmt.Sprintf("offline-turn-%d", os.Getpid())
	initialize, err := readOfflineRPCMethod(reader, "initialize")
	if err != nil || offlineCodexRPCRespond(encoder, initialize.ID, map[string]any{"userAgent": "offline-codex-v1"}) != nil {
		return 11
	}
	if _, err := readOfflineRPCMethod(reader, "initialized"); err != nil {
		return 12
	}
	thread, err := readOfflineRPCMethod(reader, "thread/start")
	if err != nil || offlineCodexRPCRespond(encoder, thread.ID, map[string]any{"thread": map[string]any{"id": threadID}}) != nil {
		return 13
	}
	turn, err := readOfflineRPCMethod(reader, "turn/start")
	if err != nil || offlineCodexRPCRespond(encoder, turn.ID, map[string]any{"turn": map[string]any{"id": turnID, "status": "inProgress"}}) != nil {
		return 14
	}
	if offlineCodexRPCNotify(encoder, "turn/started", map[string]any{"threadId": threadID, "turn": map[string]any{"id": turnID, "status": "inProgress"}}) != nil ||
		offlineCodexRPCNotify(encoder, "item/agentMessage/delta", map[string]any{"threadId": threadID, "turnId": turnID, "itemId": "offline-message", "delta": offlineHarnessMarker}) != nil ||
		offlineCodexRPCNotify(encoder, "turn/completed", map[string]any{"threadId": threadID, "turn": map[string]any{"id": turnID, "status": "completed"}}) != nil {
		return 15
	}
	return 0
}

func runOfflineACP(route string, input io.Reader, output io.Writer) int {
	reader := bufio.NewReader(input)
	encoder := json.NewEncoder(output)
	initialize, err := readOfflineRPCMethod(reader, "initialize")
	agent := map[string]string{"gemini": "gemini-cli", "kimi": "kimi-code"}[route]
	sessionID := fmt.Sprintf("offline-%s-session-%d", route, os.Getpid())
	if err != nil || offlineRPCRespond(encoder, initialize.ID, map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{}, "agentInfo": map[string]any{"name": agent, "version": "offline-fake-v1"}}) != nil {
		return 21
	}
	newSession, err := readOfflineRPCMethod(reader, "session/new")
	if err != nil || offlineRPCRespond(encoder, newSession.ID, map[string]any{"sessionId": sessionID}) != nil {
		return 22
	}
	prompt, err := readOfflineRPCMethod(reader, "session/prompt")
	if err != nil {
		return 23
	}
	if offlineRPCNotify(encoder, "session/update", map[string]any{"sessionId": sessionID, "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": offlineHarnessMarker}}}) != nil ||
		offlineRPCRespond(encoder, prompt.ID, map[string]any{"stopReason": "end_turn"}) != nil {
		return 24
	}
	return 0
}

func runOfflineStreamJSON(route string, input io.Reader, output io.Writer) int {
	reader := bufio.NewReader(input)
	encoder := json.NewEncoder(output)
	cwd, _ := os.Getwd()
	model := map[string]string{"claude": "claude-fable-5", "qwen": "qwen3.7-max"}[route]
	tools := map[string][]string{"claude": {"Bash", "Edit", "Read", "Write"}, "qwen": {"edit", "read_file", "run_shell_command"}}[route]
	sessionID := fmt.Sprintf("offline-%s-session-%d", route, os.Getpid())
	init := map[string]any{
		"type": "system", "subtype": "init", "session_id": sessionID, "uuid": "offline-init",
		"model": model, "cwd": cwd, "tools": tools, "mcp_servers": []any{}, "permission_mode": "default",
	}
	if route == "claude" {
		init["permissionMode"] = "default"
		delete(init, "permission_mode")
		init["claude_code_version"] = "offline-fake-v1"
		init["apiKeySource"] = "none"
	} else {
		init["qwen_code_version"] = "offline-fake-v1"
		init["agents"] = []any{}
		init["skills"] = []any{}
	}
	if encoder.Encode(init) != nil {
		return 31
	}
	initialize, err := readOfflineJSONLine(reader)
	if err != nil || nestedOfflineString(initialize, "request", "subtype") != "initialize" {
		return 32
	}
	if encoder.Encode(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": objectOfflineString(initialize, "request_id"), "response": map[string]any{"commands": []string{"interrupt"}}}}) != nil {
		return 33
	}
	if _, err := readOfflineJSONLine(reader); err != nil {
		return 34
	}
	if encoder.Encode(map[string]any{"type": "stream_event", "uuid": "offline-stream", "session_id": sessionID, "parent_tool_use_id": nil, "event": map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": offlineHarnessMarker}}}) != nil {
		return 35
	}
	if encoder.Encode(map[string]any{
		"type": "result", "subtype": "success", "is_error": false, "session_id": sessionID,
		"duration_ms": 1, "duration_api_ms": 1, "num_turns": 1, "result": offlineHarnessMarker,
		"usage": map[string]any{"input_tokens": 1, "output_tokens": 1}, "permission_denials": []any{},
	}) != nil {
		return 36
	}
	return 0
}

func readOfflineRPCMethod(reader *bufio.Reader, expected string) (offlineRPCFrame, error) {
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return offlineRPCFrame{}, err
	}
	var frame offlineRPCFrame
	if json.Unmarshal(line, &frame) != nil || frame.Method != expected {
		return offlineRPCFrame{}, fmt.Errorf("unexpected RPC method")
	}
	return frame, nil
}

func offlineRPCRespond(encoder *json.Encoder, id json.RawMessage, result any) error {
	return encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func offlineRPCNotify(encoder *json.Encoder, method string, params any) error {
	return encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func offlineCodexRPCRespond(encoder *json.Encoder, id json.RawMessage, result any) error {
	return encoder.Encode(map[string]any{"id": id, "result": result})
}

func offlineCodexRPCNotify(encoder *json.Encoder, method string, params any) error {
	return encoder.Encode(map[string]any{"method": method, "params": params})
}

func readOfflineJSONLine(reader *bufio.Reader) (json.RawMessage, error) {
	line, err := reader.ReadBytes('\n')
	if err != nil || !json.Valid(line) {
		return nil, fmt.Errorf("invalid JSONL frame")
	}
	return json.RawMessage(line), nil
}

func objectOfflineString(raw json.RawMessage, key string) string {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return ""
	}
	var value string
	_ = json.Unmarshal(object[key], &value)
	return value
}

func nestedOfflineString(raw json.RawMessage, objectKey, key string) string {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return ""
	}
	return objectOfflineString(object[objectKey], key)
}

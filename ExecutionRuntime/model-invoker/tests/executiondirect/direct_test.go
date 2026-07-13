package executiondirect_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/effect"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/direct"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/profile"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/routegateway"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

var directTestNow = time.Date(2026, 7, 13, 2, 0, 0, 0, time.UTC)

func TestDirectAdapterPreflightMapsRequestWithoutProviderContact(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID}
	adapter := newDirectAdapter(t, backend, selected)
	report, err := adapter.Preflight(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Accepted || report.ActualManifest.Digest == "" || backend.resolveCalls != 1 || backend.invokeCalls != 0 || backend.streamCalls != 0 {
		t.Fatalf("preflight = %#v, backend = %#v", report, backend)
	}
	if backend.lastCall.Request.Provider != "" || backend.lastCall.Request.Protocol != modelinvoker.ProtocolAuto || backend.lastCall.Request.Endpoint != "" {
		t.Fatalf("Route-owned selectors leaked into request: %#v", backend.lastCall.Request)
	}
	if backend.lastCall.Request.Output.Type != modelinvoker.OutputJSONSchema || len(backend.lastCall.Request.Tools) != 1 || backend.lastCall.Request.ToolChoice.Mode != modelinvoker.ToolChoiceAuto {
		t.Fatalf("mapped request = %#v", backend.lastCall.Request)
	}
}

func TestDirectToolPolicyFiltersDeclaredToolsAndRejectsUnknownIDs(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	invocation.Request.Tools = append(invocation.Request.Tools, union.ToolDefinition{
		ID: "write-config", Name: "write_config", Kind: "function", ExecutionOwner: union.ExecutionOwnerPraxis,
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	})
	invocation.Request.ToolPolicy.AllowedToolIDs = []string{"read-config"}
	invocation.Plan.Digest = ""
	delete(invocation.Plan.Metadata, "request_digest")
	invocation, err := execution.NewInvocation(invocation.Request, invocation.Plan)
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID}
	report, err := newDirectAdapter(t, backend, selected).Preflight(context.Background(), invocation)
	if err != nil || !report.Accepted {
		t.Fatalf("Preflight() = %#v, %v", report, err)
	}
	if got := backend.lastCall.Request.Tools; len(got) != 1 || got[0].Name != "read_config" {
		t.Fatalf("filtered tools = %#v", got)
	}

	invocation.Request.ToolPolicy.AllowedToolIDs = []string{"missing-tool"}
	invocation.Plan.Digest = ""
	delete(invocation.Plan.Metadata, "request_digest")
	if _, err = execution.NewInvocation(invocation.Request, invocation.Plan); !errors.Is(err, execution.ErrInvalidInvocation) {
		t.Fatalf("unknown allowed ToolID invocation error=%v", err)
	}
}

func TestDirectUndeclaredProviderToolCallFailsClosed(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, invokeResponses: []modelinvoker.Response{{
		ID: "response-unknown-tool", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted,
		StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &modelinvoker.FunctionCall{
			ID: "call-unknown", Name: "undeclared_tool", Arguments: json.RawMessage(`{}`),
		}}},
	}}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	seenViolation := false
	for {
		event, receiveErr := session.Receive(context.Background())
		if receiveErr != nil {
			if !errors.Is(receiveErr, direct.ErrProtocolTerminal) || !seenViolation {
				t.Fatalf("Receive() error=%v violation=%v", receiveErr, seenViolation)
			}
			break
		}
		seenViolation = seenViolation || event.Diagnostic != nil && event.Diagnostic.Code == "unattributed_tool_call"
	}
}

func TestDirectNonStreamToolLoopPairsCallResultThenTerminates(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID}
	backend.invokeResponses = []modelinvoker.Response{
		{
			ID: "response-1", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
			Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &modelinvoker.FunctionCall{ID: "call-1", Name: "read_config", Arguments: json.RawMessage(`{"path":"config.go"}`)}}},
		},
		{
			ID: "response-2", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonEndTurn,
			Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemText, Text: "done"}},
		},
	}
	adapter := newDirectAdapter(t, backend, selected)
	session, err := adapter.Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	seenToolCall := false
	runningAttempts := 0
	for !seenToolCall {
		event, err := session.Receive(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		seenToolCall = event.Model != nil && event.Model.Kind == "model_tool_call"
		if event.Mechanism != nil && event.Mechanism.Attempt != nil && event.Mechanism.Attempt.Status == union.AttemptStatusRunning {
			runningAttempts++
		}
	}
	resultPayload := json.RawMessage(`{"call_id":"call-1","name":"read_config","output":"strict","is_error":false,"executed":true,"result_origin":"external","side_effect_state":"none"}`)
	if err := session.Command(context.Background(), union.ExecutionCommand{Kind: union.CommandProvideToolResult, ActionID: "call-1", Payload: resultPayload}); err != nil {
		t.Fatal(err)
	}
	seenFinal, seenTerminal, seenPendingItem, seenCompletedItem, seenToolResult, completedAttempts := false, false, false, false, false, 0
	for !seenTerminal {
		event, err := session.Receive(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if event.Model != nil && event.Model.Kind == "content_completed" && len(event.Model.Content) == 1 && event.Model.Content[0].Text == "done" {
			seenFinal = true
		}
		if event.Mechanism != nil && event.Mechanism.Attempt != nil && event.Mechanism.Attempt.Status == union.AttemptStatusCompleted {
			completedAttempts++
		}
		if event.Item != nil && event.Item.Item.ActionID == "call-1" {
			seenPendingItem = seenPendingItem || event.Item.Item.Status == union.ItemStatusPending
			seenCompletedItem = seenCompletedItem || event.Item.Item.Status == union.ItemStatusCompleted
		}
		if event.Model != nil && event.Model.Kind == "tool_result_provided" {
			seenToolResult = event.Model.Executed != nil && *event.Model.Executed && event.Model.ResultOrigin == union.EventOriginExternal
			if strings.Contains(string(event.Model.Payload), "strict") {
				t.Fatalf("tool output leaked into audit payload: %s", event.Model.Payload)
			}
		}
		seenTerminal = event.Diagnostic != nil && event.Diagnostic.Kind == execution.EventKindRouteTerminalCandidate
	}
	if !seenFinal || !seenPendingItem || !seenCompletedItem || !seenToolResult || backend.invokeCalls != 2 || runningAttempts == 0 || runningAttempts != completedAttempts {
		t.Fatalf("final=%v items=%v/%v result=%v invokeCalls=%d attempts=%d/%d", seenFinal, seenPendingItem, seenCompletedItem, seenToolResult, backend.invokeCalls, runningAttempts, completedAttempts)
	}
	second := backend.calls[1].Request.Input
	if len(second) < 3 || second[len(second)-2].Type != modelinvoker.InputTypeFunctionCall || second[len(second)-1].Type != modelinvoker.InputTypeFunctionResult {
		t.Fatalf("continuation did not preserve call/result pairing: %#v", second)
	}
	if _, err := session.Receive(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("terminal Receive() error = %v", err)
	}
}

func TestDirectToolResultRequiresExplicitExecutionProvenance(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, invokeResponses: []modelinvoker.Response{{
		ID: "response-1", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &modelinvoker.FunctionCall{ID: "call-provenance", Name: "read_config", Arguments: json.RawMessage(`{}`)}}},
	}}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	for {
		event, receiveErr := session.Receive(context.Background())
		if receiveErr != nil {
			t.Fatal(receiveErr)
		}
		if event.Model != nil && event.Model.Kind == "model_tool_call" {
			break
		}
	}
	err = session.Command(context.Background(), union.ExecutionCommand{
		Kind: union.CommandProvideToolResult, ActionID: "call-provenance",
		Payload: json.RawMessage(`{"call_id":"call-provenance","output":"unproven"}`),
	})
	if !errors.Is(err, direct.ErrUnsupportedCommand) {
		t.Fatalf("Command() error = %v", err)
	}
	if backend.invokeCalls != 1 {
		t.Fatalf("unproven result reached backend; invoke calls = %d", backend.invokeCalls)
	}
}

func TestDirectToolLoopUsesProviderContinuationState(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	state := &modelinvoker.State{Kind: modelinvoker.StateServerContinuation, Provider: "openai", Protocol: modelinvoker.ProtocolResponses, ID: "response-state-1"}
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, invokeResponses: []modelinvoker.Response{
		{
			ID: "response-1", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall, State: state,
			Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &modelinvoker.FunctionCall{ID: "call-state", Name: "read_config", Arguments: json.RawMessage(`{"path":"config.go"}`)}}},
		},
		{ID: "response-2", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonEndTurn},
	}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	for {
		event, receiveErr := session.Receive(context.Background())
		if receiveErr != nil {
			t.Fatal(receiveErr)
		}
		if event.Model != nil && event.Model.Kind == "model_tool_call" {
			break
		}
	}
	payload := json.RawMessage(`{"call_id":"call-state","name":"read_config","output":"ok","executed":true,"result_origin":"external","side_effect_state":"none"}`)
	if err := session.Command(context.Background(), union.ExecutionCommand{Kind: union.CommandProvideToolResult, ActionID: "call-state", Payload: payload}); err != nil {
		t.Fatal(err)
	}
	if backend.invokeCalls != 2 {
		t.Fatalf("invoke calls = %d", backend.invokeCalls)
	}
	next := backend.calls[1].Request
	if next.State == nil || next.State.ID != state.ID || len(next.Input) != 1 || next.Input[0].Type != modelinvoker.InputTypeFunctionResult {
		t.Fatalf("state continuation = %#v", next)
	}
}

func TestDirectRejectsToolResultNameMutationWithoutConsumingPendingCall(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, invokeResponses: []modelinvoker.Response{
		{
			ID: "response-name", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
			Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &modelinvoker.FunctionCall{ID: "call-name", Name: "read_config", Arguments: json.RawMessage(`{"path":"config.go"}`)}}},
		},
		{ID: "response-final", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonEndTurn},
	}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	for {
		event, receiveErr := session.Receive(context.Background())
		if receiveErr != nil {
			t.Fatal(receiveErr)
		}
		if event.Model != nil && event.Model.Kind == "model_tool_call" {
			break
		}
	}
	mutated := json.RawMessage(`{"call_id":"call-name","name":"write_config","output":"bad","executed":true,"result_origin":"external","side_effect_state":"none"}`)
	if err := session.Command(context.Background(), union.ExecutionCommand{Kind: union.CommandProvideToolResult, ActionID: "call-name", Payload: mutated}); !errors.Is(err, direct.ErrUnsupportedCommand) {
		t.Fatalf("mutated name error = %v", err)
	}
	if backend.invokeCalls != 1 {
		t.Fatalf("mutated result reached backend: %d calls", backend.invokeCalls)
	}
	valid := json.RawMessage(`{"call_id":"call-name","name":"read_config","output":"ok","executed":true,"result_origin":"external","side_effect_state":"none"}`)
	if err := session.Command(context.Background(), union.ExecutionCommand{Kind: union.CommandProvideToolResult, ActionID: "call-name", Payload: valid}); err != nil {
		t.Fatalf("valid result after rejection: %v", err)
	}
	if backend.invokeCalls != 2 {
		t.Fatalf("valid result did not continue: %d calls", backend.invokeCalls)
	}
}

func TestDirectConcurrentDuplicateToolResultContinuesExactlyOnce(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, invokeResponses: []modelinvoker.Response{
		{
			ID: "response-concurrent", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
			Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &modelinvoker.FunctionCall{ID: "call-concurrent", Name: "read_config", Arguments: json.RawMessage(`{}`)}}},
		},
		{ID: "response-final", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonEndTurn},
	}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	for {
		event, receiveErr := session.Receive(context.Background())
		if receiveErr != nil {
			t.Fatal(receiveErr)
		}
		if event.Model != nil && event.Model.Kind == "model_tool_call" {
			break
		}
	}
	payload := json.RawMessage(`{"call_id":"call-concurrent","name":"read_config","output":"ok","executed":true,"result_origin":"external","side_effect_state":"none"}`)
	start := make(chan struct{})
	errorsByCall := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			errorsByCall <- session.Command(context.Background(), union.ExecutionCommand{Kind: union.CommandProvideToolResult, ActionID: "call-concurrent", Payload: payload})
		}()
	}
	close(start)
	successes, rejections := 0, 0
	for range 2 {
		err := <-errorsByCall
		switch {
		case err == nil:
			successes++
		case errors.Is(err, direct.ErrUnsupportedCommand), errors.Is(err, execution.ErrSessionClosed):
			rejections++
		default:
			t.Fatalf("concurrent result error = %v", err)
		}
	}
	if successes != 1 || rejections != 1 || backend.invokeCalls != 2 {
		t.Fatalf("success/rejection/invoke = %d/%d/%d", successes, rejections, backend.invokeCalls)
	}
}

func TestDirectParallelToolResultsContinueOnlyAfterAllCalls(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, invokeResponses: []modelinvoker.Response{
		{
			ID: "response-parallel", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
			Output: []modelinvoker.OutputItem{
				{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &modelinvoker.FunctionCall{ID: "call-b", Name: "read_config", Arguments: json.RawMessage(`{"path":"b"}`)}},
				{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &modelinvoker.FunctionCall{ID: "call-a", Name: "read_config", Arguments: json.RawMessage(`{"path":"a"}`)}},
			},
		},
		{ID: "response-final", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonEndTurn},
	}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	seen := map[union.ActionID]bool{}
	for len(seen) != 2 {
		event, receiveErr := session.Receive(context.Background())
		if receiveErr != nil {
			t.Fatal(receiveErr)
		}
		if event.Model != nil && event.Model.Kind == "model_tool_call" {
			seen[event.Model.ActionID] = true
		}
	}
	for index, callID := range []string{"call-b", "call-a"} {
		payload, _ := json.Marshal(map[string]any{"call_id": callID, "name": "read_config", "output": callID, "executed": true, "result_origin": "external", "side_effect_state": "none"})
		if err := session.Command(context.Background(), union.ExecutionCommand{Kind: union.CommandProvideToolResult, ActionID: union.ActionID(callID), Payload: payload}); err != nil {
			t.Fatal(err)
		}
		if got := backend.invokeCalls; got != index+1 {
			t.Fatalf("invoke calls after result %d = %d", index, got)
		}
	}
	input := backend.calls[1].Request.Input
	if len(input) < 5 {
		t.Fatalf("parallel continuation input = %#v", input)
	}
	if input[len(input)-4].FunctionCall.ID != "call-a" || input[len(input)-2].FunctionCall.ID != "call-b" {
		t.Fatalf("parallel call/result order is not deterministic: %#v", input)
	}
}

func TestDirectCancelReportsAcknowledgedQuiescedAndCancelledAttempt(t *testing.T) {
	invocation, selected := directInvocation(t, true, false)
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, streams: []*fakeStream{{}}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err := session.Command(context.Background(), union.ExecutionCommand{Kind: union.CommandCancelExecution}); err != nil {
		t.Fatal(err)
	}
	var ack, quiesced, cancelledAttempt, terminal bool
	var lastSource uint64
	for {
		event, receiveErr := session.Receive(context.Background())
		if errors.Is(receiveErr, io.EOF) {
			break
		}
		if receiveErr != nil {
			t.Fatal(receiveErr)
		}
		if event.Header.SourceSequence <= lastSource {
			t.Fatalf("source sequence regressed: %d <= %d", event.Header.SourceSequence, lastSource)
		}
		lastSource = event.Header.SourceSequence
		if event.Control != nil {
			ack = ack || event.Control.Kind == execution.ControlCancelAcknowledged
			quiesced = quiesced || event.Control.Kind == execution.ControlCancellationQuiesced
		}
		cancelledAttempt = cancelledAttempt || event.Mechanism != nil && event.Mechanism.Attempt != nil && event.Mechanism.Attempt.Status == union.AttemptStatusCancelled
		terminal = terminal || event.Diagnostic != nil && event.Diagnostic.Kind == execution.EventKindRouteTerminalCandidate
	}
	if !ack || !quiesced || !cancelledAttempt || !terminal {
		t.Fatalf("cancel evidence ack=%v quiesced=%v attempt=%v terminal=%v", ack, quiesced, cancelledAttempt, terminal)
	}
}

func TestDirectCleanStreamEOFBecomesProtocolViolationAndIndeterminate(t *testing.T) {
	invocation, selected := directInvocation(t, true, false)
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, streams: []*fakeStream{{}}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	var violation, terminal bool
	for {
		event, receiveErr := session.Receive(context.Background())
		if errors.Is(receiveErr, io.EOF) {
			break
		}
		if receiveErr != nil {
			t.Fatal(receiveErr)
		}
		violation = violation || event.Diagnostic != nil && event.Diagnostic.Kind == "protocol_violation"
		if event.Diagnostic != nil && event.Diagnostic.Kind == execution.EventKindRouteTerminalCandidate {
			var candidate execution.RouteTerminalCandidate
			if json.Unmarshal(event.Diagnostic.Payload, &candidate) != nil || candidate.Status != union.ExecutionStatusIndeterminate {
				t.Fatalf("terminal candidate = %#v", event.Diagnostic)
			}
			terminal = true
		}
	}
	if !violation || !terminal {
		t.Fatalf("violation=%v terminal=%v", violation, terminal)
	}
}

func TestDirectStreamPreservesDeltasAndEmitsCandidateOnlyAtCompletion(t *testing.T) {
	invocation, selected := directInvocation(t, true, false)
	response := modelinvoker.Response{ID: "response-stream", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonEndTurn}
	backend := &fakeBackend{
		routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID,
		streams: []*fakeStream{{events: []modelinvoker.StreamEvent{
			{Type: modelinvoker.StreamEventResponseStarted, Sequence: 1},
			{Type: modelinvoker.StreamEventTextDelta, Sequence: 2, TextDelta: "hello"},
			{Type: modelinvoker.StreamEventResponseCompleted, Sequence: 3, Response: &response},
		}}},
	}
	adapter := newDirectAdapter(t, backend, selected)
	session, err := adapter.Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	var kinds []string
	for {
		event, err := session.Receive(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if event.Model != nil {
			kinds = append(kinds, event.Model.Kind)
		}
		if event.Diagnostic != nil {
			kinds = append(kinds, event.Diagnostic.Kind)
		}
	}
	want := []string{"model_step_started", "content_delta", "model_step_completed", execution.EventKindRouteTerminalCandidate}
	if !equalStrings(kinds, want) {
		t.Fatalf("event kinds = %#v, want %#v", kinds, want)
	}
}

func TestDirectStreamMaterializesToolCallFoundOnlyInFinalResponse(t *testing.T) {
	invocation, selected := directInvocation(t, true, true)
	toolResponse := modelinvoker.Response{
		ID: "response-stream-tool", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &modelinvoker.FunctionCall{ID: "call-final-only", Name: "read_config", Arguments: json.RawMessage(`{"path":"config.go"}`)}}},
	}
	finalResponse := modelinvoker.Response{ID: "response-stream-final", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonEndTurn}
	backend := &fakeBackend{
		routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID,
		streams: []*fakeStream{
			{events: []modelinvoker.StreamEvent{{Type: modelinvoker.StreamEventResponseStarted, Sequence: 1}, {Type: modelinvoker.StreamEventResponseCompleted, Sequence: 2, Response: &toolResponse}}},
			{events: []modelinvoker.StreamEvent{{Type: modelinvoker.StreamEventResponseStarted, Sequence: 3}, {Type: modelinvoker.StreamEventResponseCompleted, Sequence: 4, Response: &finalResponse}}},
		},
	}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	seenCall, seenPending := false, false
	for !seenCall || !seenPending {
		event, receiveErr := session.Receive(context.Background())
		if receiveErr != nil {
			t.Fatal(receiveErr)
		}
		seenCall = seenCall || event.Model != nil && event.Model.Kind == "model_tool_call" && event.Model.ActionID == "call-final-only"
		seenPending = seenPending || event.Item != nil && event.Item.Item.ActionID == "call-final-only" && event.Item.Item.Status == union.ItemStatusPending
	}
	payload := json.RawMessage(`{"call_id":"call-final-only","name":"read_config","output":"ok","executed":true,"result_origin":"external","side_effect_state":"none"}`)
	if err := session.Command(context.Background(), union.ExecutionCommand{Kind: union.CommandProvideToolResult, ActionID: "call-final-only", Payload: payload}); err != nil {
		t.Fatal(err)
	}
	seenTerminal := false
	for !seenTerminal {
		event, receiveErr := session.Receive(context.Background())
		if receiveErr != nil {
			t.Fatal(receiveErr)
		}
		seenTerminal = event.Diagnostic != nil && event.Diagnostic.Kind == execution.EventKindRouteTerminalCandidate
	}
	if backend.streamCalls != 2 || len(backend.calls) != 2 {
		t.Fatalf("stream continuation calls = %d/%d", backend.streamCalls, len(backend.calls))
	}
	continuation := backend.calls[1].Request.Input
	if len(continuation) < 3 || continuation[len(continuation)-2].FunctionCall == nil || continuation[len(continuation)-2].FunctionCall.ID != "call-final-only" || continuation[len(continuation)-1].FunctionResult == nil || continuation[len(continuation)-1].FunctionResult.CallID != "call-final-only" {
		t.Fatalf("final-only call was not paired in continuation: %#v", continuation)
	}
}

func TestDirectInvalidProviderToolCallTerminatesWithoutWaitingForResult(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, invokeResponses: []modelinvoker.Response{{
		ID: "response-invalid-tool", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &modelinvoker.FunctionCall{ID: "", Name: "read_config", Arguments: json.RawMessage(`{}`)}}},
	}}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	seenViolation, seenTerminal := false, false
	for {
		event, receiveErr := session.Receive(ctx)
		if errors.Is(receiveErr, io.EOF) {
			break
		}
		if receiveErr != nil {
			t.Fatalf("Receive() = %v", receiveErr)
		}
		seenViolation = seenViolation || event.Diagnostic != nil && event.Diagnostic.Code == "invalid_tool_call"
		seenTerminal = seenTerminal || event.Diagnostic != nil && event.Diagnostic.Kind == execution.EventKindRouteTerminalCandidate
	}
	if !seenViolation || !seenTerminal {
		t.Fatalf("invalid tool evidence violation=%v terminal=%v", seenViolation, seenTerminal)
	}
}

func TestDirectMapperRejectsHarnessOwnedToolBeforeBackend(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	invocation.Request.Tools[0].ExecutionOwner = union.ExecutionOwnerHarness
	invocation.Plan.Digest = ""
	delete(invocation.Plan.Metadata, "request_digest")
	invocation, err := execution.NewInvocation(invocation.Request, invocation.Plan)
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID}
	adapter := newDirectAdapter(t, backend, selected)
	report, err := adapter.Preflight(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	if report.Accepted || report.RejectionCode != "direct_mapping_rejected" || backend.resolveCalls != 0 {
		t.Fatalf("report = %#v backend = %#v", report, backend)
	}
}

func TestDirectAdapterRunsThroughUnifiedRuntimeWithVerifiedStructuredEffect(t *testing.T) {
	invocation, selected := directInvocation(t, false, false)
	backend := &fakeBackend{
		routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID,
		invokeResponses: []modelinvoker.Response{{
			ID: "response-runtime", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted,
			StopReason: modelinvoker.StopReasonEndTurn,
			Output:     []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemText, Text: `{}`}},
			Usage:      modelinvoker.Usage{InputTokens: 10, OutputTokens: 4, TotalTokens: 14},
		}},
	}
	adapter := newDirectAdapter(t, backend, selected)
	registry := execution.NewRegistry()
	if err := registry.Register(context.Background(), adapter); err != nil {
		t.Fatal(err)
	}
	mechanism := invocation.Plan.Mechanisms[0]
	for _, candidate := range invocation.Plan.Mechanisms {
		if candidate.IntentID == "structured" && candidate.PreferredRank < mechanism.PreferredRank {
			mechanism = candidate
		}
	}
	attemptID := union.MechanismAttemptID("direct:" + string(mechanism.ID) + ":attempt:1")
	validated, err := effect.ValidateStructuredOutputWithMechanism(
		"effect-runtime", "verify-runtime", "structured", attemptID,
		[]byte(`{}`), invocation.Request.OutputContract.JSONSchema,
		effect.StructuredMechanism{Kind: union.StructuredStrictJSONSchema, Origin: mechanism.Origin, Fidelity: mechanism.SemanticFidelity, Transport: "responses"},
		0, directTestNow.Add(time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := execution.NewRuntime(execution.RuntimeConfig{
		Registry: registry,
		Reconciler: directReconcilerFunc(func(_ context.Context, input execution.ReconcileInput) (execution.ReconcileReport, error) {
			if _, exists := input.State.Attempts[attemptID]; !exists {
				t.Fatalf("attempt %q was not observed", attemptID)
			}
			return execution.ReconcileReport{Effects: []union.EffectRecord{validated.Effect}, SideEffectState: union.SideEffectObserved, Quiesced: true}, nil
		}),
		Verifier: directVerifierFunc(func(context.Context, execution.VerifyInput) (execution.VerificationReport, error) {
			return execution.VerificationReport{Verifications: []union.VerificationRecord{validated.Verification}}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runtime.Execute(context.Background(), adapterIdentity(adapter), invocation)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != union.ExecutionStatusSucceeded || result.VerificationStatus != union.VerificationVerified || len(result.Effects) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if len(result.FinalContent) != 1 || result.FinalContent[0].Text != `{}` || len(result.UsageMetrics) != 3 {
		t.Fatalf("content/usage = %#v / %#v", result.FinalContent, result.UsageMetrics)
	}
}

type directReconcilerFunc func(context.Context, execution.ReconcileInput) (execution.ReconcileReport, error)

func (function directReconcilerFunc) Reconcile(ctx context.Context, input execution.ReconcileInput) (execution.ReconcileReport, error) {
	return function(ctx, input)
}

type directVerifierFunc func(context.Context, execution.VerifyInput) (execution.VerificationReport, error)

func (function directVerifierFunc) Verify(ctx context.Context, input execution.VerifyInput) (execution.VerificationReport, error) {
	return function(ctx, input)
}

func adapterIdentity(adapter *direct.Adapter) string {
	descriptor, _ := adapter.Describe(context.Background())
	return descriptor.Identity.ID
}

func directInvocation(t *testing.T, stream, withTool bool) (execution.Invocation, profile.SemanticRouteProfile) {
	t.Helper()
	profiles, err := profile.RepresentativeProfiles(directTestNow)
	if err != nil {
		t.Fatal(err)
	}
	var selected profile.SemanticRouteProfile
	for _, candidate := range profiles {
		if candidate.ID == profile.ProfileOpenAIDirect {
			selected = candidate
			break
		}
	}
	registry, err := profile.NewRegistry(directTestNow, profiles...)
	if err != nil {
		t.Fatal(err)
	}
	compiler, err := profile.NewCompiler(registry, directTestNow)
	if err != nil {
		t.Fatal(err)
	}
	request := union.UnifiedExecutionRequest{
		SemanticVersion: union.SemanticVersionV1, ExecutionID: "exec.direct", ExecutionKind: union.ExecutionKindModel,
		ProfileSelector:   union.ProfileSelector{Exact: &union.VersionedIdentity{ID: string(profile.ProfileOpenAIDirect), Version: "v1candidate"}},
		Input:             []union.InputItem{{ID: "message-1", Kind: "message", Role: "user", Content: []union.ContentPart{{Kind: "text", Text: "return a result"}}}},
		Instructions:      []union.Instruction{{ID: "instruction-1", Authority: "developer", Scope: "execution", ConflictPolicy: "higher_authority_wins", Content: []union.ContentPart{{Kind: "text", Text: "Use the allowed tools only."}}}},
		ToolPolicy:        union.ToolPolicy{DefaultApproval: "on_side_effect", Parallelism: 1, MaxActions: 2},
		OutputContract:    union.OutputContract{AcceptedContentKinds: []string{"json"}, CompletionMode: "final", JSONSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`)},
		SessionIntent:     union.SessionIntent{Mode: "new"},
		ExecutionPolicy:   union.ExecutionPolicy{Stream: stream, Sandbox: "workspace_write", CWDReference: "/workspace", NetworkPolicy: "denied", UserPresence: "present", Foreground: "required", InteractionMode: "interactive", MaxConcurrency: 1},
		Budget:            union.Budget{MaxWallTime: time.Minute, MaxToolActions: 2},
		DegradationPolicy: union.DegradationPolicy{Default: union.DegradationDefaultReject},
		IntentGraph:       union.IntentGraph{Nodes: []union.IntentNode{{ID: "structured", Kind: union.IntentProduceStructured, Target: "summary", Required: true, AcceptedFidelity: []union.SemanticFidelity{union.SemanticFidelityExact}}}},
	}
	if withTool {
		request.Tools = []union.ToolDefinition{{
			ID: "read-config", Name: "read_config", Kind: "function", ExecutionOwner: union.ExecutionOwnerPraxis,
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`),
		}}
	}
	compiled, err := compiler.Compile(profile.CompileInput{
		Request: request, PaperOnly: true,
		ActualManifest: profile.InjectionManifest{SchemaVersion: "v1candidate", ProbeStatus: profile.ManifestProbeNotRun},
	})
	if err != nil {
		t.Fatal(err)
	}
	return execution.Invocation{Request: request, Plan: compiled.Plan}, selected
}

func newDirectAdapter(t *testing.T, backend direct.Backend, selected profile.SemanticRouteProfile) *direct.Adapter {
	t.Helper()
	adapter, err := direct.New(direct.Config{
		Identity: union.VersionedIdentity{ID: "direct-test", Version: "v1"}, Backend: backend,
		RouteID: selected.Selection.BaseRouteID, Model: selected.Selection.ModelID,
		Invocation: upstream.InvocationContext{Explicit: true, Usage: upstream.InvocationGeneralAPI, Subject: upstream.SubjectService, Tenancy: upstream.TenancyMulti, Execution: upstream.ExecutionForeground},
	})
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

type fakeBackend struct {
	mu              sync.Mutex
	routeID         upstream.RouteID
	model           string
	resolveCalls    int
	invokeCalls     int
	streamCalls     int
	lastCall        modelinvoker.RouteCall
	calls           []modelinvoker.RouteCall
	invokeResponses []modelinvoker.Response
	streams         []*fakeStream
}

func (backend *fakeBackend) Resolve(_ context.Context, call modelinvoker.RouteCall) (routegateway.Resolution, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.resolveCalls++
	backend.lastCall = call
	return routegateway.Resolution{Route: modelinvoker.RouteSelection{RouteID: backend.routeID, Model: backend.model}}, nil
}

func (backend *fakeBackend) Invoke(_ context.Context, call modelinvoker.RouteCall) (routegateway.InvokeResult, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.invokeCalls++
	backend.lastCall = call
	backend.calls = append(backend.calls, call)
	if len(backend.invokeResponses) == 0 {
		return routegateway.InvokeResult{}, errors.New("unexpected Invoke")
	}
	response := backend.invokeResponses[0]
	backend.invokeResponses = backend.invokeResponses[1:]
	return routegateway.InvokeResult{Response: response}, nil
}

func (backend *fakeBackend) OpenStream(_ context.Context, call modelinvoker.RouteCall) (direct.ModelStream, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.streamCalls++
	backend.lastCall = call
	backend.calls = append(backend.calls, call)
	if len(backend.streams) == 0 {
		return nil, errors.New("unexpected OpenStream")
	}
	stream := backend.streams[0]
	backend.streams = backend.streams[1:]
	return stream, nil
}

type fakeStream struct {
	events []modelinvoker.StreamEvent
	index  int
	closed bool
}

func (stream *fakeStream) Next() bool {
	if stream.closed || stream.index >= len(stream.events) {
		return false
	}
	stream.index++
	return true
}
func (stream *fakeStream) Event() modelinvoker.StreamEvent { return stream.events[stream.index-1] }
func (stream *fakeStream) Err() error                      { return nil }
func (stream *fakeStream) Close() error {
	stream.closed = true
	return nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

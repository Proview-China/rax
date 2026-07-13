package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	harnessprocess "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/process"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

type Config struct {
	Process          harnessprocess.Config
	InitializeParams json.RawMessage
}

type InitializeResult struct {
	Raw json.RawMessage
}

type SessionInfo struct {
	ID  string
	Raw json.RawMessage
}

type PromptResult struct {
	StopReason string
	Raw        json.RawMessage
}

type NativeEventKind string

const (
	NativeAgentMessageChunk NativeEventKind = "agent_message_chunk"
	NativeAgentThoughtChunk NativeEventKind = "agent_thought_chunk"
	NativeToolCall          NativeEventKind = "tool_call"
	NativeToolCallUpdate    NativeEventKind = "tool_call_update"
	NativePlanUpdate        NativeEventKind = "plan"
	NativeApprovalRequest   NativeEventKind = "approval_request"
	NativeTerminalCandidate NativeEventKind = "terminal_candidate"
	NativeExtension         NativeEventKind = "extension"
)

type TerminalCandidate struct {
	StopReason      string
	Status          union.ExecutionStatus
	SideEffectState union.SideEffectState
}

type NativeEvent struct {
	Kind       NativeEventKind
	Method     string
	RequestID  json.RawMessage
	SessionID  string
	ToolCallID string
	ToolStatus string
	ToolKind   string
	Text       string
	Params     json.RawMessage
	Raw        json.RawMessage
	Terminal   *TerminalCandidate
}

type Client struct {
	wire       *rpcWire
	initialize InitializeResult
}

type Session struct {
	client *Client
	id     string
	mu     sync.Mutex

	active             bool
	terminalObserved   bool
	sideEffectPossible bool
	terminal           chan NativeEvent
	promptDone         chan struct{}
}

func Start(ctx context.Context, config Config) (*Client, error) {
	if !validJSONObject(config.InitializeParams) {
		return nil, fmt.Errorf("%w: initialize params must be an object", ErrInvalidConfig)
	}
	wire, err := startRPCWire(ctx, config.Process)
	if err != nil {
		return nil, err
	}
	result, err := wire.call(ctx, "initialize", config.InitializeParams)
	if err != nil {
		_ = wire.close()
		return nil, fmt.Errorf("%w: initialize: %v", ErrProtocol, err)
	}
	return &Client{wire: wire, initialize: InitializeResult{Raw: cloneRaw(result)}}, nil
}

func (c *Client) InitializeResult() InitializeResult {
	return InitializeResult{Raw: cloneRaw(c.initialize.Raw)}
}

func (c *Client) NewSession(ctx context.Context, params json.RawMessage) (*Session, SessionInfo, error) {
	if !validJSONObject(params) {
		return nil, SessionInfo{}, fmt.Errorf("%w: session/new params must be an object", ErrInvalidConfig)
	}
	result, err := c.wire.call(ctx, "session/new", params)
	if err != nil {
		return nil, SessionInfo{}, err
	}
	sessionID := objectString(result, "sessionId")
	if sessionID == "" {
		c.wire.fail(fmt.Errorf("%w: session/new response has no sessionId", ErrProtocol))
		return nil, SessionInfo{}, fmt.Errorf("%w: session/new response has no sessionId", ErrProtocol)
	}
	session := &Session{client: c, id: sessionID, terminal: make(chan NativeEvent, 1)}
	return session, SessionInfo{ID: sessionID, Raw: cloneRaw(result)}, nil
}

func (s *Session) ID() string { return s.id }

func (s *Session) Prompt(ctx context.Context, prompt json.RawMessage) (PromptResult, error) {
	if !validJSONArray(prompt) {
		return PromptResult{}, fmt.Errorf("%w: prompt must be a JSON array of ACP content blocks", ErrInvalidConfig)
	}
	s.mu.Lock()
	if s.active {
		s.mu.Unlock()
		return PromptResult{}, fmt.Errorf("%w: prompt already active", ErrProtocol)
	}
	promptDone := make(chan struct{})
	s.active, s.terminalObserved, s.sideEffectPossible = true, false, false
	s.promptDone = promptDone
	s.mu.Unlock()
	defer close(promptDone)
	params, _ := json.Marshal(map[string]any{"sessionId": s.id, "prompt": prompt})
	result, err := s.client.wire.call(ctx, "session/prompt", params)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return PromptResult{}, fmt.Errorf("%w: %v", ErrMissingTerminal, err)
		}
		return PromptResult{}, err
	}
	stopReason := objectString(result, "stopReason")
	if stopReason == "" {
		s.client.wire.fail(fmt.Errorf("%w: session/prompt response has no stopReason", ErrProtocol))
		return PromptResult{}, fmt.Errorf("%w: session/prompt response has no stopReason", ErrProtocol)
	}
	event := s.finish(stopReason, result)
	s.terminal <- event
	return PromptResult{StopReason: stopReason, Raw: cloneRaw(result)}, nil
}

func (s *Session) Cancel() error {
	params, _ := json.Marshal(map[string]string{"sessionId": s.id})
	return s.client.wire.notify("session/cancel", params)
}

func (s *Session) Receive(ctx context.Context) (NativeEvent, error) {
	select {
	case envelope := <-s.client.wire.inbound:
		return s.decode(envelope)
	default:
	}
	select {
	case envelope := <-s.client.wire.inbound:
		return s.decode(envelope)
	case terminal := <-s.terminal:
		return s.refreshTerminal(terminal), nil
	case <-ctx.Done():
		return NativeEvent{}, ctx.Err()
	case <-s.client.wire.done:
		select {
		case envelope := <-s.client.wire.inbound:
			return s.decode(envelope)
		case terminal := <-s.terminal:
			return s.refreshTerminal(terminal), nil
		default:
			err := s.client.wire.failure()
			s.mu.Lock()
			active := s.active && !s.terminalObserved
			promptDone := s.promptDone
			s.mu.Unlock()
			if active && errors.Is(err, io.EOF) && promptDone != nil {
				// A prompt response can be dispatched immediately before process
				// EOF. Wait for Prompt to either enqueue its terminal candidate or
				// confirm that the response itself was missing.
				select {
				case <-promptDone:
				case <-ctx.Done():
					return NativeEvent{}, ctx.Err()
				}
				select {
				case terminal := <-s.terminal:
					return s.refreshTerminal(terminal), nil
				default:
				}
				return NativeEvent{}, fmt.Errorf("%w: %v", ErrMissingTerminal, err)
			}
			return NativeEvent{}, err
		}
	}
}

func (s *Session) refreshTerminal(event NativeEvent) NativeEvent {
	if event.Terminal == nil {
		return event
	}
	s.mu.Lock()
	if s.sideEffectPossible {
		event.Terminal.SideEffectState = union.SideEffectPossible
	}
	s.mu.Unlock()
	return event
}

func (s *Session) RespondPermission(event NativeEvent, result json.RawMessage) error {
	if event.Kind != NativeApprovalRequest || !validJSONObject(result) {
		return ErrReverseRequest
	}
	return s.client.wire.respond(event.RequestID, result, nil)
}

func (s *Session) RespondError(event NativeEvent, code int, message string) error {
	if len(event.RequestID) == 0 || strings.TrimSpace(message) == "" {
		return ErrReverseRequest
	}
	fault, _ := json.Marshal(map[string]any{"code": code, "message": message})
	return s.client.wire.respond(event.RequestID, nil, fault)
}

func (s *Session) Close() error { return s.client.Close() }
func (c *Client) Close() error  { return c.wire.close() }

func (s *Session) decode(envelope wireEnvelope) (NativeEvent, error) {
	message := envelope.message
	event := NativeEvent{Method: message.Method, RequestID: cloneRaw(message.ID), Params: cloneRaw(message.Params), Raw: envelope.raw}
	if message.Kind == harnessprocess.JSONRPCRequest {
		if message.Method == "session/request_permission" {
			event.Kind = NativeApprovalRequest
			event.SessionID = objectString(message.Params, "sessionId")
			event.ToolCallID = objectString(message.Params, "toolCallId")
			return event, nil
		}
		event.Kind = NativeExtension
		return event, nil
	}
	if message.Kind != harnessprocess.JSONRPCNotification {
		return NativeEvent{}, fmt.Errorf("%w: unexpected message kind %q", ErrProtocol, message.Kind)
	}
	if message.Method != "session/update" {
		event.Kind = NativeExtension
		return event, nil
	}
	event.SessionID = objectString(message.Params, "sessionId")
	update := objectRaw(message.Params, "update")
	updateKind := objectString(update, "sessionUpdate")
	event.ToolCallID = objectString(update, "toolCallId")
	event.ToolStatus = objectString(update, "status")
	event.ToolKind = objectString(update, "kind")
	event.Text = nestedString(update, "content", "text")
	switch updateKind {
	case "agent_message_chunk":
		event.Kind = NativeAgentMessageChunk
	case "agent_thought_chunk":
		event.Kind = NativeAgentThoughtChunk
	case "tool_call":
		event.Kind = NativeToolCall
		s.markSideEffectPossible()
	case "tool_call_update":
		event.Kind = NativeToolCallUpdate
		s.markSideEffectPossible()
	case "plan":
		event.Kind = NativePlanUpdate
	default:
		event.Kind = NativeExtension
	}
	return event, nil
}

func (s *Session) markSideEffectPossible() {
	s.mu.Lock()
	s.sideEffectPossible = true
	s.mu.Unlock()
}

func (s *Session) finish(stopReason string, raw json.RawMessage) NativeEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := union.SideEffectNone
	if s.sideEffectPossible {
		state = union.SideEffectPossible
	}
	status := union.ExecutionStatusSucceeded
	switch stopReason {
	case "cancelled":
		status = union.ExecutionStatusCancelled
	case "refusal":
		status = union.ExecutionStatusFailed
	case "max_tokens", "max_turns":
		status = union.ExecutionStatusPartial
	case "end_turn":
		status = union.ExecutionStatusSucceeded
	default:
		status = union.ExecutionStatusIndeterminate
	}
	s.active, s.terminalObserved = false, true
	return NativeEvent{
		Kind: NativeTerminalCandidate, Method: "session/prompt", SessionID: s.id, Raw: cloneRaw(raw),
		Terminal: &TerminalCandidate{StopReason: stopReason, Status: status, SideEffectState: state},
	}
}

func objectRaw(raw json.RawMessage, key string) json.RawMessage {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return nil
	}
	return cloneRaw(object[key])
}

func objectString(raw json.RawMessage, key string) string {
	var value string
	_ = json.Unmarshal(objectRaw(raw, key), &value)
	return value
}

func nestedString(raw json.RawMessage, objectKey, key string) string {
	return objectString(objectRaw(raw, objectKey), key)
}

func validJSONArray(raw json.RawMessage) bool {
	if len(raw) == 0 || !json.Valid(raw) {
		return false
	}
	var value []json.RawMessage
	return json.Unmarshal(raw, &value) == nil
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), raw...)
}

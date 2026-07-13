package codexappserver

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

type ClientInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version"`
}

type Config struct {
	Process      harnessprocess.Config
	ClientInfo   ClientInfo
	Capabilities json.RawMessage
}

type Thread struct {
	ID  string
	Raw json.RawMessage
}

type Turn struct {
	ID     string
	Status string
	Raw    json.RawMessage
}

type InitializeResult struct {
	Raw json.RawMessage
}

type NativeEventKind string

const (
	NativeThreadStarted      NativeEventKind = "thread_started"
	NativeTurnStarted        NativeEventKind = "turn_started"
	NativeItemStarted        NativeEventKind = "item_started"
	NativeItemUpdated        NativeEventKind = "item_updated"
	NativeItemCompleted      NativeEventKind = "item_completed"
	NativeApprovalRequest    NativeEventKind = "approval_request"
	NativeDynamicToolRequest NativeEventKind = "dynamic_tool_request"
	NativeProvisionalDiff    NativeEventKind = "provisional_diff"
	NativeTerminalCandidate  NativeEventKind = "terminal_candidate"
	NativeError              NativeEventKind = "error"
	NativeExtension          NativeEventKind = "extension"
)

type TerminalCandidate struct {
	NativeStatus    string
	Status          union.ExecutionStatus
	StopReason      string
	SideEffectState union.SideEffectState
}

type NativeEvent struct {
	Kind       NativeEventKind
	Method     string
	RequestID  json.RawMessage
	ThreadID   string
	TurnID     string
	ItemID     string
	ItemType   string
	ItemStatus string
	Delta      string
	Params     json.RawMessage
	Raw        json.RawMessage
	Terminal   *TerminalCandidate
}

type Client struct {
	wire       *rpcWire
	initialize InitializeResult
	mu         sync.Mutex

	initialized        bool
	threadID           string
	turnID             string
	activeTurn         bool
	terminalObserved   bool
	sideEffectPossible bool
}

func Start(ctx context.Context, config Config) (*Client, error) {
	if strings.TrimSpace(config.ClientInfo.Name) == "" || strings.TrimSpace(config.ClientInfo.Version) == "" {
		return nil, fmt.Errorf("%w: clientInfo name and version are required", ErrInvalidConfig)
	}
	if len(config.Capabilities) == 0 {
		config.Capabilities = json.RawMessage(`{}`)
	}
	if !validJSONObject(config.Capabilities) {
		return nil, fmt.Errorf("%w: capabilities must be an object", ErrInvalidConfig)
	}
	wire, err := startRPCWire(ctx, config.Process)
	if err != nil {
		return nil, err
	}
	client := &Client{wire: wire}
	params, _ := json.Marshal(map[string]any{"clientInfo": config.ClientInfo, "capabilities": config.Capabilities})
	initialize, err := wire.call(ctx, "initialize", params)
	if err != nil {
		_ = wire.close()
		return nil, fmt.Errorf("%w: initialize: %v", ErrProtocol, err)
	}
	if err := wire.notify("initialized", json.RawMessage(`{}`)); err != nil {
		_ = wire.close()
		return nil, fmt.Errorf("%w: initialized notification: %v", ErrProtocol, err)
	}
	client.initialized = true
	client.initialize = InitializeResult{Raw: append(json.RawMessage(nil), initialize...)}
	return client, nil
}

func (c *Client) InitializeResult() InitializeResult {
	if c == nil {
		return InitializeResult{}
	}
	return InitializeResult{Raw: append(json.RawMessage(nil), c.initialize.Raw...)}
}

func (c *Client) StartThread(ctx context.Context, params json.RawMessage) (Thread, error) {
	if !validJSONObject(params) {
		return Thread{}, fmt.Errorf("%w: thread/start params must be an object", ErrInvalidConfig)
	}
	result, err := c.wire.call(ctx, "thread/start", params)
	if err != nil {
		return Thread{}, err
	}
	threadID := nestedString(result, "thread", "id")
	if threadID == "" {
		c.wire.fail(fmt.Errorf("%w: thread/start response has no thread.id", ErrProtocol))
		return Thread{}, fmt.Errorf("%w: thread/start response has no thread.id", ErrProtocol)
	}
	c.mu.Lock()
	c.threadID = threadID
	c.mu.Unlock()
	return Thread{ID: threadID, Raw: append(json.RawMessage(nil), result...)}, nil
}

func (c *Client) StartTurn(ctx context.Context, params json.RawMessage) (Turn, error) {
	if !validJSONObject(params) {
		return Turn{}, fmt.Errorf("%w: turn/start params must be an object", ErrInvalidConfig)
	}
	threadID := objectString(params, "threadId")
	if threadID == "" {
		return Turn{}, fmt.Errorf("%w: turn/start requires threadId", ErrInvalidConfig)
	}
	result, err := c.wire.call(ctx, "turn/start", params)
	if err != nil {
		return Turn{}, err
	}
	turnID := nestedString(result, "turn", "id")
	status := nestedString(result, "turn", "status")
	if turnID == "" {
		c.wire.fail(fmt.Errorf("%w: turn/start response has no turn.id", ErrProtocol))
		return Turn{}, fmt.Errorf("%w: turn/start response has no turn.id", ErrProtocol)
	}
	c.mu.Lock()
	c.threadID, c.turnID = threadID, turnID
	c.activeTurn, c.terminalObserved, c.sideEffectPossible = true, false, false
	c.mu.Unlock()
	return Turn{ID: turnID, Status: status, Raw: append(json.RawMessage(nil), result...)}, nil
}

func (c *Client) Interrupt(ctx context.Context) error {
	c.mu.Lock()
	threadID, turnID, active := c.threadID, c.turnID, c.activeTurn
	c.mu.Unlock()
	if !active || threadID == "" || turnID == "" {
		return ErrNoActiveTurn
	}
	params, _ := json.Marshal(map[string]string{"threadId": threadID, "turnId": turnID})
	_, err := c.wire.call(ctx, "turn/interrupt", params)
	return err
}

func (c *Client) Receive(ctx context.Context) (NativeEvent, error) {
	envelope, err := c.wire.receive(ctx)
	if err != nil {
		c.mu.Lock()
		missing := c.activeTurn && !c.terminalObserved && errors.Is(err, io.EOF)
		c.mu.Unlock()
		if missing {
			return NativeEvent{}, fmt.Errorf("%w: %v", ErrMissingTerminal, err)
		}
		return NativeEvent{}, err
	}
	event, err := c.decode(envelope)
	if err != nil {
		c.wire.fail(err)
		return NativeEvent{}, err
	}
	return event, nil
}

func (c *Client) RespondApproval(event NativeEvent, result json.RawMessage) error {
	if event.Kind != NativeApprovalRequest || !validJSONObject(result) {
		return ErrReverseRequest
	}
	return c.wire.respond(event.RequestID, result, nil)
}

func (c *Client) RespondDynamicTool(event NativeEvent, result json.RawMessage) error {
	if event.Kind != NativeDynamicToolRequest || !validJSONObject(result) {
		return ErrReverseRequest
	}
	return c.wire.respond(event.RequestID, result, nil)
}

func (c *Client) RespondError(event NativeEvent, code int, message string) error {
	if len(event.RequestID) == 0 || strings.TrimSpace(message) == "" {
		return ErrReverseRequest
	}
	fault, _ := json.Marshal(map[string]any{"code": code, "message": message})
	return c.wire.respond(event.RequestID, nil, fault)
}

func (c *Client) Close() error { return c.wire.close() }

func (c *Client) decode(envelope wireEnvelope) (NativeEvent, error) {
	message := envelope.message
	event := NativeEvent{Method: message.Method, RequestID: append(json.RawMessage(nil), message.ID...), Params: append(json.RawMessage(nil), message.Params...), Raw: envelope.raw}
	if message.Kind == harnessprocess.JSONRPCRequest {
		event.ThreadID = objectString(message.Params, "threadId")
		event.TurnID = objectString(message.Params, "turnId")
		event.ItemID = objectString(message.Params, "itemId")
		switch message.Method {
		case "item/commandExecution/requestApproval", "item/fileChange/requestApproval", "item/permissions/requestApproval", "item/tool/requestUserInput":
			event.Kind = NativeApprovalRequest
		case "item/tool/call":
			event.Kind = NativeDynamicToolRequest
			event.ItemID = objectString(message.Params, "callId")
			event.ItemType = "dynamicToolCall"
			c.markSideEffectPossible()
		default:
			event.Kind = NativeExtension
		}
		return event, nil
	}
	if message.Kind != harnessprocess.JSONRPCNotification {
		return NativeEvent{}, fmt.Errorf("%w: unexpected message kind %q", ErrProtocol, message.Kind)
	}
	event.ThreadID = objectString(message.Params, "threadId")
	event.TurnID = objectString(message.Params, "turnId")
	switch message.Method {
	case "thread/started":
		event.Kind = NativeThreadStarted
		event.ThreadID = nestedString(message.Params, "thread", "id")
	case "turn/started":
		event.Kind = NativeTurnStarted
		event.TurnID = nestedString(message.Params, "turn", "id")
	case "turn/completed":
		event.Kind = NativeTerminalCandidate
		event.TurnID = nestedString(message.Params, "turn", "id")
		status := nestedString(message.Params, "turn", "status")
		event.Terminal = c.terminal(status)
	case "turn/diff/updated":
		event.Kind = NativeProvisionalDiff
		event.Delta = objectString(message.Params, "diff")
		c.markSideEffectPossible()
	case "item/started", "item/completed":
		if message.Method == "item/started" {
			event.Kind = NativeItemStarted
		} else {
			event.Kind = NativeItemCompleted
		}
		event.ItemID = nestedString(message.Params, "item", "id")
		event.ItemType = nestedString(message.Params, "item", "type")
		event.ItemStatus = nestedString(message.Params, "item", "status")
		if event.ItemID == "" || event.ItemType == "" {
			return NativeEvent{}, fmt.Errorf("%w: %s lacks item id/type", ErrProtocol, message.Method)
		}
		if sideEffectItem(event.ItemType) {
			c.markSideEffectPossible()
		}
	case "error":
		event.Kind = NativeError
	default:
		if strings.HasPrefix(message.Method, "item/") {
			event.Kind = NativeItemUpdated
			event.ItemID = objectString(message.Params, "itemId")
			event.Delta = objectString(message.Params, "delta")
		} else {
			event.Kind = NativeExtension
		}
	}
	return event, nil
}

func (c *Client) markSideEffectPossible() {
	c.mu.Lock()
	c.sideEffectPossible = true
	c.mu.Unlock()
}

func (c *Client) terminal(native string) *TerminalCandidate {
	c.mu.Lock()
	defer c.mu.Unlock()
	state := union.SideEffectNone
	if c.sideEffectPossible {
		state = union.SideEffectPossible
	}
	status := union.ExecutionStatusFailed
	switch native {
	case "completed":
		status = union.ExecutionStatusSucceeded
	case "interrupted":
		status = union.ExecutionStatusCancelled
	case "failed":
		status = union.ExecutionStatusFailed
	default:
		status = union.ExecutionStatusIndeterminate
	}
	c.activeTurn, c.terminalObserved = false, true
	return &TerminalCandidate{NativeStatus: native, Status: status, StopReason: native, SideEffectState: state}
}

func sideEffectItem(kind string) bool {
	switch kind {
	case "commandExecution", "fileChange", "mcpToolCall", "dynamicToolCall", "collabToolCall":
		return true
	default:
		return false
	}
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

package streamjson

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	harnessprocess "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/process"
)

// Message is one validated JSON object received from the SDK sidecar.
type Message struct {
	Type      string
	Subtype   string
	RequestID string
	Raw       json.RawMessage
}

func (message Message) Clone() Message {
	message.Raw = append(json.RawMessage(nil), message.Raw...)
	return message
}

type controlReply struct {
	subtype string
	result  json.RawMessage
	fault   json.RawMessage
}

// PendingCall is a control request that has been written successfully but may
// be awaited later. Harness Sessions use this split for cancellation: Command
// can return after dispatch, allowing Runtime to record cancel_dispatched,
// while Receive correlates the native interrupt acknowledgement afterwards.
type PendingCall struct {
	client    *Client
	requestID string
	reply     <-chan controlReply
	once      sync.Once
	result    json.RawMessage
	err       error
}

func (pending *PendingCall) RequestID() string {
	if pending == nil {
		return ""
	}
	return pending.requestID
}

func (pending *PendingCall) Await(ctx context.Context) (json.RawMessage, error) {
	if pending == nil || pending.client == nil || ctx == nil {
		return nil, ErrClosed
	}
	pending.once.Do(func() {
		select {
		case response := <-pending.reply:
			if response.subtype != "success" {
				pending.err = fmt.Errorf("%w: %s", ErrControl, compactFault(response.fault))
				return
			}
			pending.result = append(json.RawMessage(nil), response.result...)
		case <-ctx.Done():
			pending.client.removePending(pending.requestID)
			pending.err = ctx.Err()
		case <-pending.client.done:
			pending.client.removePending(pending.requestID)
			pending.err = pending.client.currentError()
		}
	})
	return append(json.RawMessage(nil), pending.result...), pending.err
}

type Client struct {
	process  *harnessprocess.Session
	evidence LaunchEvidence
	nextID   atomic.Uint64

	mu       sync.Mutex
	pending  map[string]chan controlReply
	err      error
	inbound  chan Message
	done     chan struct{}
	failOnce sync.Once

	closeOnce sync.Once
	closeErr  error
}

func Start(ctx context.Context, config harnessprocess.Config) (*Client, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: context is nil", ErrInvalidConfig)
	}
	evidence, err := ProbeLaunch(config)
	if err != nil {
		return nil, err
	}
	processSession, err := harnessprocess.Start(ctx, config)
	if err != nil {
		return nil, err
	}
	client := &Client{
		process:  processSession,
		evidence: evidence,
		pending:  make(map[string]chan controlReply),
		inbound:  make(chan Message, 128),
		done:     make(chan struct{}),
	}
	go client.readLoop()
	return client, nil
}

func (client *Client) Evidence() LaunchEvidence {
	if client == nil {
		return LaunchEvidence{}
	}
	return client.evidence.Clone()
}

// Send writes one arbitrary JSON object. Route adapters should use Call for
// SDK control requests so response IDs remain correlated.
func (client *Client) Send(value any) error {
	raw, err := marshalObject(value)
	if err != nil {
		return err
	}
	return client.SendRaw(raw)
}

func (client *Client) SendRaw(raw json.RawMessage) error {
	if client == nil {
		return ErrClosed
	}
	if !validObject(raw) {
		return fmt.Errorf("%w: outbound frame must be a JSON object", ErrProtocol)
	}
	if err := client.currentError(); err != nil {
		return err
	}
	if err := client.process.WriteFrame(raw); err != nil {
		client.fail(err)
		return err
	}
	return nil
}

// Call sends the SDK control_request envelope and waits for its correlated
// control_response.
func (client *Client) Call(ctx context.Context, request any) (json.RawMessage, error) {
	if client == nil || ctx == nil {
		return nil, ErrClosed
	}
	pending, err := client.BeginCall(request)
	if err != nil {
		return nil, err
	}
	return pending.Await(ctx)
}

// BeginCall writes a correlated control_request and returns without waiting
// for the control_response.
func (client *Client) BeginCall(request any) (*PendingCall, error) {
	if client == nil {
		return nil, ErrClosed
	}
	requestRaw, err := marshalObject(request)
	if err != nil {
		return nil, err
	}
	requestID := "praxis-" + strconv.FormatUint(client.nextID.Add(1), 10)
	reply := make(chan controlReply, 1)
	client.mu.Lock()
	if client.err != nil {
		err := client.err
		client.mu.Unlock()
		return nil, err
	}
	client.pending[requestID] = reply
	client.mu.Unlock()

	frame, err := json.Marshal(struct {
		Type      string          `json:"type"`
		RequestID string          `json:"request_id"`
		Request   json.RawMessage `json:"request"`
	}{Type: "control_request", RequestID: requestID, Request: requestRaw})
	if err != nil {
		client.removePending(requestID)
		return nil, err
	}
	if err := client.SendRaw(frame); err != nil {
		client.removePending(requestID)
		return nil, err
	}
	return &PendingCall{client: client, requestID: requestID, reply: reply}, nil
}

func (client *Client) Respond(requestID string, response any) error {
	if strings.TrimSpace(requestID) == "" {
		return fmt.Errorf("%w: control request id is required", ErrProtocol)
	}
	responseRaw, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("%w: encode control response: %v", ErrProtocol, err)
	}
	if len(responseRaw) == 0 || !json.Valid(responseRaw) {
		return fmt.Errorf("%w: control response is invalid", ErrProtocol)
	}
	frame, err := json.Marshal(map[string]any{
		"type": "control_response",
		"response": map[string]any{
			"subtype": "success", "request_id": requestID,
			"response": json.RawMessage(responseRaw),
		},
	})
	if err != nil {
		return err
	}
	return client.SendRaw(frame)
}

func (client *Client) RespondError(requestID, message string) error {
	if strings.TrimSpace(requestID) == "" || strings.TrimSpace(message) == "" {
		return fmt.Errorf("%w: control request id and error are required", ErrProtocol)
	}
	frame, err := json.Marshal(map[string]any{
		"type": "control_response",
		"response": map[string]any{
			"subtype": "error", "request_id": requestID, "error": message,
		},
	})
	if err != nil {
		return err
	}
	return client.SendRaw(frame)
}

func (client *Client) Receive(ctx context.Context) (Message, error) {
	if client == nil || ctx == nil {
		return Message{}, ErrClosed
	}
	select {
	case message := <-client.inbound:
		return message.Clone(), nil
	default:
	}
	select {
	case message := <-client.inbound:
		return message.Clone(), nil
	case <-ctx.Done():
		return Message{}, ctx.Err()
	case <-client.done:
		select {
		case message := <-client.inbound:
			return message.Clone(), nil
		default:
			return Message{}, client.currentError()
		}
	}
}

func (client *Client) CloseInput() error {
	if client == nil {
		return ErrClosed
	}
	return client.process.CloseInput()
}

func (client *Client) Wait() (harnessprocess.Result, error) {
	if client == nil {
		return harnessprocess.Result{}, ErrClosed
	}
	return client.process.Wait()
}

func (client *Client) Close() error {
	if client == nil {
		return nil
	}
	client.closeOnce.Do(func() {
		client.closeErr = client.process.Close()
		client.fail(ErrClosed)
	})
	return client.closeErr
}

func (client *Client) readLoop() {
	for {
		frame, err := client.process.ReadFrame()
		if err != nil {
			client.fail(err)
			return
		}
		message, err := decodeMessage(frame.Raw)
		if err != nil {
			client.fail(err)
			go func() { _ = client.process.Close() }()
			return
		}
		if message.Type == "control_response" {
			if err := client.acceptControlResponse(message.Raw); err != nil {
				client.fail(err)
				go func() { _ = client.process.Close() }()
				return
			}
			continue
		}
		select {
		case client.inbound <- message:
		case <-client.done:
			return
		}
	}
}

func (client *Client) acceptControlResponse(raw json.RawMessage) error {
	var envelope struct {
		Response struct {
			Subtype   string          `json:"subtype"`
			RequestID string          `json:"request_id"`
			Response  json.RawMessage `json:"response"`
			Error     json.RawMessage `json:"error"`
		} `json:"response"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || strings.TrimSpace(envelope.Response.RequestID) == "" {
		return fmt.Errorf("%w: malformed control_response", ErrProtocol)
	}
	if envelope.Response.Subtype != "success" && envelope.Response.Subtype != "error" {
		return fmt.Errorf("%w: unknown control response subtype %q", ErrProtocol, envelope.Response.Subtype)
	}
	client.mu.Lock()
	reply := client.pending[envelope.Response.RequestID]
	if reply != nil {
		delete(client.pending, envelope.Response.RequestID)
	}
	client.mu.Unlock()
	if reply == nil {
		return fmt.Errorf("%w: %s", ErrUnexpectedResponse, envelope.Response.RequestID)
	}
	reply <- controlReply{
		subtype: envelope.Response.Subtype,
		result:  append(json.RawMessage(nil), envelope.Response.Response...),
		fault:   append(json.RawMessage(nil), envelope.Response.Error...),
	}
	return nil
}

func (client *Client) fail(err error) {
	if err == nil {
		err = io.EOF
	}
	client.failOnce.Do(func() {
		client.mu.Lock()
		client.err = err
		client.mu.Unlock()
		close(client.done)
	})
}

func (client *Client) currentError() error {
	if client == nil {
		return ErrClosed
	}
	client.mu.Lock()
	err := client.err
	client.mu.Unlock()
	return err
}

func (client *Client) removePending(requestID string) {
	client.mu.Lock()
	delete(client.pending, requestID)
	client.mu.Unlock()
}

func decodeMessage(raw json.RawMessage) (Message, error) {
	if !validObject(raw) {
		return Message{}, fmt.Errorf("%w: inbound frame must be a JSON object", ErrProtocol)
	}
	var envelope struct {
		Type      string `json:"type"`
		Subtype   string `json:"subtype"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return Message{}, fmt.Errorf("%w: decode envelope: %v", ErrProtocol, err)
	}
	return Message{
		Type: envelope.Type, Subtype: envelope.Subtype, RequestID: envelope.RequestID,
		Raw: append(json.RawMessage(nil), raw...),
	}, nil
}

func marshalObject(value any) (json.RawMessage, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("%w: encode object: %v", ErrProtocol, err)
	}
	if !validObject(raw) {
		return nil, fmt.Errorf("%w: value must encode to a JSON object", ErrProtocol)
	}
	return raw, nil
}

func validObject(raw json.RawMessage) bool {
	if len(raw) == 0 || !json.Valid(raw) {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(raw, &object) == nil && object != nil
}

func compactFault(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "unknown control error"
	}
	var text string
	if json.Unmarshal(raw, &text) == nil && text != "" {
		return text
	}
	var object struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &object) == nil && object.Message != "" {
		return object.Message
	}
	if errors.Is(json.Unmarshal(raw, &object), nil) {
		return "control request rejected"
	}
	return "malformed control error"
}

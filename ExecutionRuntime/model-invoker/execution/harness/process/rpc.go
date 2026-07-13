package process

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
)

// JSONRPCMessageKind identifies the three JSON-RPC 2.0 envelope forms.
type JSONRPCMessageKind string

const (
	JSONRPCRequest      JSONRPCMessageKind = "request"
	JSONRPCNotification JSONRPCMessageKind = "notification"
	JSONRPCResponse     JSONRPCMessageKind = "response"
)

// JSONRPCMessage is a validated projection of one correlated RPC object.
type JSONRPCMessage struct {
	Kind   JSONRPCMessageKind
	ID     json.RawMessage
	Method string
	Params json.RawMessage
	Result json.RawMessage
	Error  json.RawMessage
}

func parseRPC(raw []byte, allowMissingVersion bool) (JSONRPCMessage, error) {
	object, err := decodeJSONObject(raw)
	if err != nil {
		return JSONRPCMessage{}, ErrInvalidJSONRPC
	}
	versionRaw, hasVersion := object["jsonrpc"]
	if !hasVersion && !allowMissingVersion {
		return JSONRPCMessage{}, ErrInvalidJSONRPC
	}
	if hasVersion {
		var version string
		if err := json.Unmarshal(versionRaw, &version); err != nil || version != "2.0" {
			return JSONRPCMessage{}, ErrInvalidJSONRPC
		}
	}
	methodRaw, hasMethod := object["method"]
	id, hasID := object["id"]
	result, hasResult := object["result"]
	errorValue, hasError := object["error"]
	if hasID {
		if _, err := rpcIDKey(id); err != nil {
			return JSONRPCMessage{}, err
		}
	}
	if hasMethod {
		if hasResult || hasError {
			return JSONRPCMessage{}, ErrInvalidJSONRPC
		}
		var method string
		if err := json.Unmarshal(methodRaw, &method); err != nil || method == "" {
			return JSONRPCMessage{}, ErrInvalidJSONRPC
		}
		params := object["params"]
		if len(params) > 0 {
			trimmed := bytes.TrimSpace(params)
			if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
				return JSONRPCMessage{}, ErrInvalidJSONRPC
			}
		}
		kind := JSONRPCNotification
		if hasID {
			kind = JSONRPCRequest
		}
		return JSONRPCMessage{Kind: kind, ID: append(json.RawMessage(nil), id...), Method: method, Params: append(json.RawMessage(nil), params...)}, nil
	}
	if !hasID || hasResult == hasError {
		return JSONRPCMessage{}, ErrInvalidJSONRPC
	}
	if hasError {
		var failure struct {
			Code    json.RawMessage `json:"code"`
			Message string          `json:"message"`
		}
		if err := json.Unmarshal(errorValue, &failure); err != nil || len(failure.Code) == 0 || failure.Message == "" {
			return JSONRPCMessage{}, ErrInvalidJSONRPC
		}
		if _, err := strconv.ParseFloat(string(bytes.TrimSpace(failure.Code)), 64); err != nil {
			return JSONRPCMessage{}, ErrInvalidJSONRPC
		}
	}
	return JSONRPCMessage{
		Kind: JSONRPCResponse, ID: append(json.RawMessage(nil), id...),
		Result: append(json.RawMessage(nil), result...), Error: append(json.RawMessage(nil), errorValue...),
	}, nil
}

func decodeJSONObject(raw []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, ErrInvalidJSONRPC
	}
	object := make(map[string]json.RawMessage)
	for decoder.More() {
		keyToken, err := decoder.Token()
		key, ok := keyToken.(string)
		if err != nil || !ok {
			return nil, ErrInvalidJSONRPC
		}
		if _, duplicate := object[key]; duplicate {
			return nil, ErrInvalidJSONRPC
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, ErrInvalidJSONRPC
		}
		object[key] = value
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, ErrInvalidJSONRPC
	}
	if decoder.More() {
		return nil, ErrInvalidJSONRPC
	}
	return object, nil
}

func rpcIDKey(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "", ErrInvalidJSONRPC
	}
	if trimmed[0] == '"' {
		var value string
		if err := json.Unmarshal(trimmed, &value); err != nil {
			return "", ErrInvalidJSONRPC
		}
		return "s:" + value, nil
	}
	if _, err := strconv.ParseFloat(string(trimmed), 64); err != nil {
		return "", ErrInvalidJSONRPC
	}
	return "n:" + string(trimmed), nil
}

type rpcTracker struct {
	mu                sync.Mutex
	outboundPending   map[string]struct{}
	outboundCompleted map[string]struct{}
	inboundPending    map[string]struct{}
	inboundCompleted  map[string]struct{}
}

func newRPCTracker() *rpcTracker {
	return &rpcTracker{
		outboundPending: make(map[string]struct{}), outboundCompleted: make(map[string]struct{}),
		inboundPending: make(map[string]struct{}), inboundCompleted: make(map[string]struct{}),
	}
}

func (t *rpcTracker) prepareOutgoing(message JSONRPCMessage) (func(), error) {
	if message.Kind == JSONRPCNotification {
		return func() {}, nil
	}
	key, err := rpcIDKey(message.ID)
	if err != nil {
		return nil, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	switch message.Kind {
	case JSONRPCRequest:
		if _, ok := t.outboundPending[key]; ok {
			return nil, fmt.Errorf("%w: %s", ErrDuplicateRequestID, key)
		}
		if _, ok := t.outboundCompleted[key]; ok {
			return nil, fmt.Errorf("%w: %s", ErrDuplicateRequestID, key)
		}
		t.outboundPending[key] = struct{}{}
		return func() {
			t.mu.Lock()
			delete(t.outboundPending, key)
			t.mu.Unlock()
		}, nil
	case JSONRPCResponse:
		if _, ok := t.inboundCompleted[key]; ok {
			return nil, fmt.Errorf("%w: %s", ErrDuplicateResponseID, key)
		}
		if _, ok := t.inboundPending[key]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrUnknownResponseID, key)
		}
		delete(t.inboundPending, key)
		t.inboundCompleted[key] = struct{}{}
		return func() {
			t.mu.Lock()
			delete(t.inboundCompleted, key)
			t.inboundPending[key] = struct{}{}
			t.mu.Unlock()
		}, nil
	default:
		return nil, ErrInvalidJSONRPC
	}
}

func (t *rpcTracker) acceptIncoming(message JSONRPCMessage) error {
	if message.Kind == JSONRPCNotification {
		return nil
	}
	key, err := rpcIDKey(message.ID)
	if err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	switch message.Kind {
	case JSONRPCRequest:
		if _, ok := t.inboundPending[key]; ok {
			return fmt.Errorf("%w: %s", ErrDuplicateRequestID, key)
		}
		if _, ok := t.inboundCompleted[key]; ok {
			return fmt.Errorf("%w: %s", ErrDuplicateRequestID, key)
		}
		t.inboundPending[key] = struct{}{}
		return nil
	case JSONRPCResponse:
		if _, ok := t.outboundCompleted[key]; ok {
			return fmt.Errorf("%w: %s", ErrDuplicateResponseID, key)
		}
		if _, ok := t.outboundPending[key]; !ok {
			return fmt.Errorf("%w: %s", ErrUnknownResponseID, key)
		}
		delete(t.outboundPending, key)
		t.outboundCompleted[key] = struct{}{}
		return nil
	default:
		return ErrInvalidJSONRPC
	}
}

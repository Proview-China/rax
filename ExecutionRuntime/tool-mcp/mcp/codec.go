package mcp

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const MaxMessageBytes = 1 << 20

type ErrorObject struct {
	Code    int64           `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ErrorObject    `json:"error,omitempty"`
}

func DecodeMessage(payload []byte) (Message, error) {
	if len(payload) == 0 || len(payload) > MaxMessageBytes {
		return Message{}, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "MCP message is empty or exceeds limit")
	}
	var message Message
	if err := core.DecodeStrictJSON(payload, &message); err != nil {
		return Message{}, err
	}
	if err := message.Validate(); err != nil {
		return Message{}, err
	}
	return message, nil
}

func EncodeMessage(message Message) ([]byte, error) {
	if err := message.Validate(); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(message)
	if err != nil || len(payload) > MaxMessageBytes {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "MCP message cannot be encoded within limit")
	}
	return payload, nil
}

func (m Message) Validate() error {
	if m.JSONRPC != "2.0" {
		return invalid("MCP JSON-RPC version must be 2.0")
	}
	hasID := len(m.ID) != 0
	if hasID {
		if err := validateID(m.ID); err != nil {
			return err
		}
	}
	if m.Method != "" {
		if strings.TrimSpace(m.Method) != m.Method || len(m.Method) > 128 || len(m.Params) > MaxMessageBytes || m.Result != nil || m.Error != nil {
			return invalid("MCP request or notification shape is invalid")
		}
		if len(m.Params) > 0 && !validJSON(m.Params) {
			return invalid("MCP params are invalid JSON")
		}
		return nil
	}
	if !hasID || (m.Result == nil) == (m.Error == nil) || m.Params != nil {
		return invalid("MCP response requires id and exactly one result or error")
	}
	if len(m.Result) > MaxMessageBytes || m.Result != nil && !validJSON(m.Result) {
		return invalid("MCP result is invalid JSON")
	}
	if m.Error != nil {
		if strings.TrimSpace(m.Error.Message) == "" || len(m.Error.Message) > 4096 || len(m.Error.Data) > MaxMessageBytes || len(m.Error.Data) > 0 && !validJSON(m.Error.Data) {
			return invalid("MCP error object is invalid")
		}
	}
	return nil
}

func validateID(raw json.RawMessage) error {
	if bytes.Equal(raw, []byte("null")) {
		return invalid("MCP id must not be null")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return invalid("MCP id is invalid")
	}
	switch id := value.(type) {
	case string:
		if strings.TrimSpace(id) == "" || len(id) > 128 {
			return invalid("MCP string id is blank or unbounded")
		}
	case json.Number:
		if strings.ContainsAny(id.String(), ".eE") {
			return invalid("MCP numeric id must be an integer")
		}
		if _, err := id.Int64(); err != nil {
			return invalid("MCP numeric id exceeds int64")
		}
	default:
		return invalid("MCP id must be a string or integer")
	}
	return nil
}

func validJSON(payload []byte) bool {
	var value any
	return core.DecodeStrictJSON(payload, &value) == nil
}

func invalid(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, message)
}

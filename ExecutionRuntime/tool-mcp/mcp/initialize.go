package mcp

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type ClientCapabilities struct {
	Roots        map[string]json.RawMessage `json:"roots,omitempty"`
	Sampling     map[string]json.RawMessage `json:"sampling,omitempty"`
	Elicitation  map[string]json.RawMessage `json:"elicitation,omitempty"`
	Experimental map[string]json.RawMessage `json:"experimental,omitempty"`
}

type ServerCapabilities struct {
	Tools        map[string]json.RawMessage `json:"tools,omitempty"`
	Resources    map[string]json.RawMessage `json:"resources,omitempty"`
	Prompts      map[string]json.RawMessage `json:"prompts,omitempty"`
	Logging      map[string]json.RawMessage `json:"logging,omitempty"`
	Tasks        map[string]json.RawMessage `json:"tasks,omitempty"`
	Experimental map[string]json.RawMessage `json:"experimental,omitempty"`
}

type InitializeParams struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
	ClientInfo      Implementation     `json:"clientInfo"`
}

type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      Implementation     `json:"serverInfo"`
	Instructions    string             `json:"instructions,omitempty"`
}

func DecodeInitializeRequest(message Message) (InitializeParams, error) {
	if message.Method != "initialize" || len(message.ID) == 0 {
		return InitializeParams{}, invalid("initialize must be a JSON-RPC request")
	}
	var params InitializeParams
	if err := core.DecodeStrictJSON(message.Params, &params); err != nil {
		return InitializeParams{}, err
	}
	if err := validateImplementation(params.ClientInfo); err != nil || !validDateVersion(params.ProtocolVersion) {
		return InitializeParams{}, invalid("initialize client info or protocol version is invalid")
	}
	return params, nil
}

func EncodeInitializeResult(id json.RawMessage, result InitializeResult) (Message, error) {
	if err := validateImplementation(result.ServerInfo); err != nil || !validDateVersion(result.ProtocolVersion) || len(result.Instructions) > 16<<10 {
		return Message{}, invalid("initialize result is invalid")
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return Message{}, err
	}
	message := Message{JSONRPC: "2.0", ID: append(json.RawMessage(nil), id...), Result: payload}
	return message, message.Validate()
}

func NegotiateProtocol(clientRequested string, serverSupported []string) (string, error) {
	if !validDateVersion(clientRequested) || len(serverSupported) == 0 {
		return "", invalid("protocol negotiation input is invalid")
	}
	versions := append([]string(nil), serverSupported...)
	sort.Sort(sort.Reverse(sort.StringSlice(versions)))
	for _, version := range versions {
		if !validDateVersion(version) {
			return "", invalid("server protocol list contains an invalid version")
		}
		if version <= clientRequested && version <= contract.MCPStableProtocolVersion {
			return version, nil
		}
	}
	return "", core.NewError(core.ErrorPreconditionFailed, core.ReasonRestoreIncompatible, "no stable MCP protocol version overlaps")
}

func validateImplementation(value Implementation) error {
	if strings.TrimSpace(value.Name) == "" || strings.TrimSpace(value.Version) == "" || len(value.Name) > 128 || len(value.Version) > 64 {
		return invalid("MCP implementation info is incomplete")
	}
	return nil
}

func validDateVersion(value string) bool {
	if len(value) != 10 || value[4] != '-' || value[7] != '-' {
		return false
	}
	for i, c := range []byte(value) {
		if i == 4 || i == 7 {
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

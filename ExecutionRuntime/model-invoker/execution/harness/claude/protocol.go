package claude

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type MCPServerState struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// InitMessage is the provider-reported SystemMessage(init) evidence used for
// the Actual Manifest. Both current snake_case and historical camelCase keys
// are accepted where the official CLI has emitted both shapes.
type InitMessage struct {
	Type              string           `json:"type"`
	Subtype           string           `json:"subtype"`
	UUID              string           `json:"uuid,omitempty"`
	SessionID         string           `json:"session_id"`
	Model             string           `json:"model"`
	CWD               string           `json:"cwd"`
	Tools             []string         `json:"tools"`
	MCPServers        []MCPServerState `json:"mcp_servers"`
	PermissionMode    string           `json:"permissionMode"`
	PermissionModeAlt string           `json:"permission_mode"`
	CLIVersion        string           `json:"claude_code_version"`
	CLIVersionAlt     string           `json:"claudeCodeVersion"`
	APIKeySource      string           `json:"apiKeySource"`
	APIKeySourceAlt   string           `json:"api_key_source"`
	Raw               json.RawMessage  `json:"-"`
}

func (message InitMessage) EffectivePermissionMode() string {
	if message.PermissionMode != "" {
		return message.PermissionMode
	}
	return message.PermissionModeAlt
}

func (message InitMessage) EffectiveCLIVersion() string {
	if message.CLIVersion != "" {
		return message.CLIVersion
	}
	return message.CLIVersionAlt
}

func (message InitMessage) EffectiveAPIKeySource() string {
	if message.APIKeySource != "" {
		return message.APIKeySource
	}
	return message.APIKeySourceAlt
}

type ExpectedInit struct {
	Model          string
	CWD            string
	Tools          []string
	MCPServers     []string
	PermissionMode string
	CLIVersion     string
	APIKeySource   string
}

func decodeInit(raw json.RawMessage) (InitMessage, error) {
	var message InitMessage
	if err := json.Unmarshal(raw, &message); err != nil {
		return InitMessage{}, fmt.Errorf("%w: decode init: %v", ErrProtocol, err)
	}
	message.Raw = append(json.RawMessage(nil), raw...)
	if message.Type != "system" || message.Subtype != "init" || strings.TrimSpace(message.SessionID) == "" {
		return InitMessage{}, fmt.Errorf("%w: first SDK event must be system/init with session_id", ErrProtocol)
	}
	if strings.TrimSpace(message.Model) == "" || strings.TrimSpace(message.CWD) == "" {
		return InitMessage{}, fmt.Errorf("%w: init must report model and cwd", ErrProtocol)
	}
	return message, nil
}

func validateInit(expected ExpectedInit, actual InitMessage) error {
	differences := make([]string, 0, 8)
	compare := func(name, want, got string) {
		if want != "" && want != got {
			differences = append(differences, fmt.Sprintf("%s=%q want %q", name, got, want))
		}
	}
	compare("model", expected.Model, actual.Model)
	compare("cwd", expected.CWD, actual.CWD)
	compare("permission_mode", expected.PermissionMode, actual.EffectivePermissionMode())
	compare("claude_code_version", expected.CLIVersion, actual.EffectiveCLIVersion())
	compare("api_key_source", expected.APIKeySource, actual.EffectiveAPIKeySource())
	if !sameStrings(expected.Tools, actual.Tools) {
		differences = append(differences, fmt.Sprintf("tools=%v want %v", sortedStrings(actual.Tools), sortedStrings(expected.Tools)))
	}
	actualMCP := make([]string, 0, len(actual.MCPServers))
	for _, server := range actual.MCPServers {
		actualMCP = append(actualMCP, server.Name+":"+server.Status)
	}
	if !sameStrings(expected.MCPServers, actualMCP) {
		differences = append(differences, fmt.Sprintf("mcp_servers=%v want %v", sortedStrings(actualMCP), sortedStrings(expected.MCPServers)))
	}
	if len(differences) != 0 {
		return fmt.Errorf("%w: %s", ErrManifestDrift, strings.Join(differences, "; "))
	}
	return nil
}

func sameStrings(left, right []string) bool {
	left = sortedStrings(left)
	right = sortedStrings(right)
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

func sortedStrings(values []string) []string {
	copyValues := append([]string(nil), values...)
	sort.Strings(copyValues)
	return copyValues
}

func objectRaw(raw json.RawMessage, key string) json.RawMessage {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return nil
	}
	return append(json.RawMessage(nil), object[key]...)
}

func objectString(raw json.RawMessage, key string) string {
	var value string
	_ = json.Unmarshal(objectRaw(raw, key), &value)
	return value
}

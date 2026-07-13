package qwen

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

type InitMessage struct {
	Type           string           `json:"type"`
	Subtype        string           `json:"subtype"`
	UUID           string           `json:"uuid"`
	SessionID      string           `json:"session_id"`
	CWD            string           `json:"cwd"`
	Tools          []string         `json:"tools"`
	MCPServers     []MCPServerState `json:"mcp_servers"`
	Model          string           `json:"model"`
	PermissionMode string           `json:"permission_mode"`
	PermissionAlt  string           `json:"permissionMode"`
	QwenVersion    string           `json:"qwen_code_version"`
	QwenVersionAlt string           `json:"qwenCodeVersion"`
	Agents         []string         `json:"agents"`
	Skills         []string         `json:"skills"`
	Capabilities   json.RawMessage  `json:"capabilities"`
	Data           json.RawMessage  `json:"data"`
	Raw            json.RawMessage  `json:"-"`
}

func (message InitMessage) EffectivePermissionMode() string {
	if message.PermissionMode != "" {
		return message.PermissionMode
	}
	return message.PermissionAlt
}

func (message InitMessage) EffectiveVersion() string {
	if message.QwenVersion != "" {
		return message.QwenVersion
	}
	return message.QwenVersionAlt
}

type ExpectedInit struct {
	Model          string
	CWD            string
	Tools          []string
	MCPServers     []string
	PermissionMode string
	QwenVersion    string
	Agents         []string
	Skills         []string
}

func decodeInit(raw json.RawMessage) (InitMessage, error) {
	var message InitMessage
	if err := json.Unmarshal(raw, &message); err != nil {
		return InitMessage{}, fmt.Errorf("%w: decode SDKSystemMessage: %v", ErrProtocol, err)
	}
	message.Raw = append(json.RawMessage(nil), raw...)
	if message.Type != "system" || message.Subtype != "init" || strings.TrimSpace(message.SessionID) == "" {
		return InitMessage{}, fmt.Errorf("%w: first SDK event must be system/init with session_id", ErrProtocol)
	}
	if strings.TrimSpace(message.Model) == "" || strings.TrimSpace(message.CWD) == "" {
		return InitMessage{}, fmt.Errorf("%w: SDKSystemMessage must report model and cwd", ErrProtocol)
	}
	return message, nil
}

func validateInit(expected ExpectedInit, actual InitMessage) error {
	differences := make([]string, 0, 10)
	compare := func(name, want, got string) {
		if want != "" && want != got {
			differences = append(differences, fmt.Sprintf("%s=%q want %q", name, got, want))
		}
	}
	compare("model", expected.Model, actual.Model)
	compare("cwd", expected.CWD, actual.CWD)
	compare("permission_mode", expected.PermissionMode, actual.EffectivePermissionMode())
	compare("qwen_code_version", expected.QwenVersion, actual.EffectiveVersion())
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
	if !sameStrings(expected.Agents, actual.Agents) {
		differences = append(differences, fmt.Sprintf("agents=%v want %v", sortedStrings(actual.Agents), sortedStrings(expected.Agents)))
	}
	if !sameStrings(expected.Skills, actual.Skills) {
		differences = append(differences, fmt.Sprintf("skills=%v want %v", sortedStrings(actual.Skills), sortedStrings(expected.Skills)))
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

func objectInt(raw json.RawMessage, key string) int {
	var value int
	_ = json.Unmarshal(objectRaw(raw, key), &value)
	return value
}

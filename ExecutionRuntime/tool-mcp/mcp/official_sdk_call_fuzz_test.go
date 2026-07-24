package mcp

import (
	"encoding/json"
	"testing"
)

func FuzzDecodeMCPCallArgumentsV1(f *testing.F) {
	f.Add([]byte(`{"value":1}`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`{"value":1}{"value":2}`))
	f.Fuzz(func(t *testing.T, payload []byte) {
		arguments, err := decodeMCPCallArgumentsV1(payload)
		if err != nil {
			return
		}
		if arguments == nil {
			t.Fatal("accepted MCP tools/call arguments produced a nil object")
		}
		if _, err := json.Marshal(arguments); err != nil {
			t.Fatalf("accepted MCP tools/call arguments were not JSON encodable: %v", err)
		}
	})
}

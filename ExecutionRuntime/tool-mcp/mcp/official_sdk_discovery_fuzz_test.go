package mcp

import (
	"context"
	"testing"
	"time"

	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	officialmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func FuzzOfficialSDKDiscoveryPaginationV1(f *testing.F) {
	f.Add("cursor-a", "cursor-b", false)
	f.Add("same", "same", true)
	f.Fuzz(func(t *testing.T, first, second string, forceCycle bool) {
		first = boundedFuzzCursorV1(first, "cursor-a")
		second = boundedFuzzCursorV1(second, "cursor-b")
		session := completeFakeOfficialSDKSessionV1()
		session.initialize.Capabilities.Resources = nil
		session.initialize.Capabilities.Prompts = nil
		session.tools = map[string]*officialmcp.ListToolsResult{
			"": {Tools: []*officialmcp.Tool{{Name: "first", InputSchema: map[string]any{"type": "object"}}}, NextCursor: first},
		}
		if forceCycle {
			session.tools[first] = &officialmcp.ListToolsResult{NextCursor: first}
		} else {
			session.tools[first] = &officialmcp.ListToolsResult{Tools: []*officialmcp.Tool{{Name: "second", InputSchema: map[string]any{"type": "object"}}}, NextCursor: second}
			session.tools[second] = &officialmcp.ListToolsResult{}
		}
		discovery, err := newOfficialSDKDiscoveryV1(session, func() time.Time { return testkit.FixedTime }, DefaultOfficialSDKDiscoveryLimitsV1())
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := discovery.DiscoverV1(context.Background(), officialSDKDiscoveryRequestV1())
		if err == nil {
			if err := snapshot.ValidateCurrent(testkit.FixedTime); err != nil {
				t.Fatalf("successful fuzz discovery returned an invalid Snapshot: %v", err)
			}
		}
		if session.toolCalls > DefaultOfficialSDKDiscoveryLimitsV1().MaxPages+1 {
			t.Fatalf("pagination exceeded the configured page bound: %d", session.toolCalls)
		}
	})
}

func boundedFuzzCursorV1(value, fallback string) string {
	if value == "" {
		return fallback
	}
	if len(value) > toolcontract.MaxMCPDiscoveryCursorBytesV1 {
		return value[:toolcontract.MaxMCPDiscoveryCursorBytesV1]
	}
	return value
}

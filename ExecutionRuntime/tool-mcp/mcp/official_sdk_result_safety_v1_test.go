package mcp

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	officialmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCanonicalizeOfficialSDKCallResultV1(t *testing.T) {
	result := &officialmcp.CallToolResult{
		Content:           []officialmcp.Content{&officialmcp.TextContent{Text: "ok"}},
		StructuredContent: map[string]any{"ok": true},
	}
	first, err := canonicalizeOfficialSDKCallResultV1(result, 4096)
	if err != nil {
		t.Fatal(err)
	}
	second, err := canonicalizeOfficialSDKCallResultV1(result, 4096)
	if err != nil || string(first) != string(second) || strings.Contains(string(first), "\n") {
		t.Fatalf("canonical result drifted: first=%q second=%q err=%v", first, second, err)
	}
	if !strings.Contains(string(first), `"content":[{"type":"text","text":"ok"}]`) || !strings.Contains(string(first), `"structuredContent":{"ok":true}`) {
		t.Fatalf("official MCP wire shape was not preserved: %s", first)
	}
}

func TestCanonicalizeOfficialSDKCallResultV1FailsClosed(t *testing.T) {
	var nilText *officialmcp.TextContent
	cycle := map[string]any{}
	cycle["cycle"] = cycle
	for name, result := range map[string]*officialmcp.CallToolResult{
		"nil content slice":  {},
		"typed nil content":  {Content: []officialmcp.Content{nilText}},
		"sampling content":   {Content: []officialmcp.Content{&officialmcp.ToolUseContent{ID: "nested", Name: "bad"}}},
		"structured array":   {Content: []officialmcp.Content{}, StructuredContent: []string{"not", "object"}},
		"cyclic structured":  {Content: []officialmcp.Content{}, StructuredContent: cycle},
		"nonfinite metadata": {Content: []officialmcp.Content{&officialmcp.TextContent{Text: "bad", Meta: officialmcp.Meta{"value": math.Inf(1)}}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := canonicalizeOfficialSDKCallResultV1(result, 4096); err == nil {
				t.Fatal("unsafe official MCP result was accepted")
			}
		})
	}
	oversized := &officialmcp.CallToolResult{Content: []officialmcp.Content{&officialmcp.TextContent{Text: strings.Repeat("x", 256)}}}
	if _, err := canonicalizeOfficialSDKCallResultV1(oversized, 64); err == nil || !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("oversized result error=%v", err)
	}
}

func TestOfficialSDKPhysicalExecutorV1UnsafeResultRemainsUnknownAndInspectOnly(t *testing.T) {
	fixture := newMCPCallFixtureV1(t)
	fixture.session.result = &officialmcp.CallToolResult{
		Content:           []officialmcp.Content{&officialmcp.TextContent{Text: "provider returned"}},
		StructuredContent: []string{"not", "an", "object"},
	}
	receipt, err := fixture.executor.ExecuteControlledOperationPhysicalV3(context.Background(), fixture.authorization)
	if err == nil || !core.HasCategory(err, core.ErrorIndeterminate) || !receipt.Admitted || fixture.session.calls.Load() != 1 {
		t.Fatalf("unsafe result receipt=%+v err=%v calls=%d", receipt, err, fixture.session.calls.Load())
	}
	entry, inspectErr := fixture.executor.InspectMCPPhysicalExecutionV1(context.Background(), fixture.authorization.StableKeyDigest)
	if inspectErr != nil || entry.State != MCPPhysicalExecutionUnknownV1 || entry.ProtocolReceipt != nil {
		t.Fatalf("unsafe result entry=%+v err=%v", entry, inspectErr)
	}
	if _, err = fixture.executor.ExecuteControlledOperationPhysicalV3(context.Background(), fixture.authorization); err != nil || fixture.session.calls.Load() != 1 {
		t.Fatalf("unsafe result was redispatched: err=%v calls=%d", err, fixture.session.calls.Load())
	}
}

func FuzzCanonicalizeOfficialSDKCallResultV1(f *testing.F) {
	f.Add([]byte("ok"), uint16(128))
	f.Add([]byte("\x00\xff\n"), uint16(32))
	f.Fuzz(func(t *testing.T, text []byte, rawLimit uint16) {
		limit := uint64(rawLimit) + 1
		result := &officialmcp.CallToolResult{Content: []officialmcp.Content{&officialmcp.TextContent{Text: string(text)}}}
		encoded, err := canonicalizeOfficialSDKCallResultV1(result, limit)
		if err != nil {
			return
		}
		if uint64(len(encoded)) > limit || !json.Valid(encoded) {
			t.Fatalf("successful bounded result is invalid: length=%d limit=%d", len(encoded), limit)
		}
	})
}

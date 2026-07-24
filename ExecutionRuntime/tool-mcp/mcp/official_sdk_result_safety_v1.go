package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	officialmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

var errMCPResultLimitExceededV1 = errors.New("MCP result exceeds its governed output limit")

// canonicalizeOfficialSDKCallResultV1 preserves the official MCP wire shape
// while bounding the bytes that may enter the Tool-owned Protocol Receipt.
// It does not truncate, summarize, or upgrade Provider content into a domain
// result. A result that cannot be recorded exactly remains Unknown.
func canonicalizeOfficialSDKCallResultV1(result *officialmcp.CallToolResult, resultLimit uint64) ([]byte, error) {
	if result == nil || result.Content == nil || resultLimit == 0 {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "official MCP tools/call result is incomplete")
	}
	for _, content := range result.Content {
		if nilLikeOfficialSDKResultContentV1(content) {
			return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "official MCP tools/call result contains nil content")
		}
		switch content.(type) {
		case *officialmcp.TextContent, *officialmcp.ImageContent, *officialmcp.AudioContent, *officialmcp.ResourceLink, *officialmcp.EmbeddedResource:
		default:
			return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "official MCP tools/call result contains a sampling-only content type")
		}
	}

	limit := resultLimit
	if maximum := uint64(toolcontract.MaxMCPProtocolReceiptBytesV1); limit > maximum {
		limit = maximum
	}
	if result.StructuredContent != nil {
		structured, err := marshalMCPResultWithinLimitV1(result.StructuredContent, limit)
		if err != nil {
			return nil, err
		}
		trimmed := bytes.TrimSpace(structured)
		if len(trimmed) == 0 || trimmed[0] != '{' {
			return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "official MCP structuredContent is not one JSON object")
		}
	}
	return marshalMCPResultWithinLimitV1(result, limit)
}

func marshalMCPResultWithinLimitV1(value any, limit uint64) ([]byte, error) {
	if limit == 0 || limit > uint64(^uint(0)>>1) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "MCP result output limit is invalid")
	}
	buffer := &boundedMCPResultBufferV1{limit: int(limit)}
	encoder := json.NewEncoder(buffer)
	if err := encoder.Encode(value); err != nil {
		if errors.Is(err, errMCPResultLimitExceededV1) {
			return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonCanonicalLimitExceeded, "official MCP tools/call result exceeds its governed output limit")
		}
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "official MCP tools/call result is not canonical JSON")
	}
	encoded := bytes.TrimSuffix(buffer.Bytes(), []byte{'\n'})
	if len(encoded) == 0 {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "official MCP tools/call result encoded no bytes")
	}
	return append([]byte(nil), encoded...), nil
}

type boundedMCPResultBufferV1 struct {
	bytes.Buffer
	limit int
}

func (b *boundedMCPResultBufferV1) Write(value []byte) (int, error) {
	if len(value) > b.limit-b.Len() {
		return 0, errMCPResultLimitExceededV1
	}
	return b.Buffer.Write(value)
}

func nilLikeOfficialSDKResultContentV1(value officialmcp.Content) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	return v.Kind() == reflect.Pointer && v.IsNil()
}

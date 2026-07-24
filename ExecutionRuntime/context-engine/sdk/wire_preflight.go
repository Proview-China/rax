package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

func preflightWireMetaV1(ctx context.Context, payload []byte, op OfflineSDKOperationV1, hardCap uint64) (OfflineRequestMetaV1, error) {
	if err := validateContextV1(ctx, op); err != nil {
		return OfflineRequestMetaV1{}, err
	}
	if uint64(len(payload)) > hardCap {
		return OfflineRequestMetaV1{}, sdkErrorV1(OfflineErrorLimitExceededV1, op, "payload", "request wire hard cap exceeded", contract.ErrLimitExceeded)
	}
	start, end, err := findTopLevelJSONFieldV1(ctx, payload, "meta")
	if err != nil {
		return OfflineRequestMetaV1{}, mapWireSyntaxErrorV1(op, "meta", err)
	}
	if start < 0 {
		return OfflineRequestMetaV1{}, sdkErrorV1(OfflineErrorInvalidArgumentV1, op, "meta", "missing request metadata", contract.ErrInvalid)
	}
	var meta OfflineRequestMetaV1
	decoder := json.NewDecoder(&contextChunkReaderV1{ctx: ctx, reader: bytes.NewReader(payload[start:end])})
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&meta); err != nil {
		if ctx.Err() != nil {
			return OfflineRequestMetaV1{}, mapErrorV1(op, "meta", ctx.Err())
		}
		return OfflineRequestMetaV1{}, mapWireSyntaxErrorV1(op, "meta", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if ctx.Err() != nil {
			return OfflineRequestMetaV1{}, mapErrorV1(op, "meta", ctx.Err())
		}
		return OfflineRequestMetaV1{}, sdkErrorV1(OfflineErrorInvalidArgumentV1, op, "meta", "trailing metadata value", contract.ErrInvalid)
	}
	if err := validateRequestMetaV1(meta, op); err != nil {
		return OfflineRequestMetaV1{}, err
	}
	if uint64(len(payload)) > meta.Limits.MaxWireRequestBytes {
		return OfflineRequestMetaV1{}, sdkErrorV1(OfflineErrorLimitExceededV1, op, "payload", "request wire limit exceeded", contract.ErrLimitExceeded)
	}
	return meta, nil
}

func mapWireSyntaxErrorV1(op OfflineSDKOperationV1, path string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return mapErrorV1(op, path, err)
	}
	return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, path, err.Error(), contract.ErrInvalid)
}

func findTopLevelJSONFieldV1(ctx context.Context, payload []byte, field string) (int, int, error) {
	index, err := skipJSONSpaceContextV1(ctx, payload, 0)
	if err != nil {
		return -1, -1, err
	}
	if index >= len(payload) || payload[index] != '{' {
		return -1, -1, fmt.Errorf("top-level JSON object required")
	}
	index++
	for {
		if index%wireChunkBytesV1 == 0 {
			if err := ctx.Err(); err != nil {
				return -1, -1, err
			}
		}
		index, err = skipJSONSpaceContextV1(ctx, payload, index)
		if err != nil {
			return -1, -1, err
		}
		if index >= len(payload) {
			return -1, -1, fmt.Errorf("unterminated top-level object")
		}
		if payload[index] == '}' {
			return -1, -1, nil
		}
		keyStart := index
		keyEnd, err := skipJSONStringBytesV1(ctx, payload, index)
		if err != nil {
			return -1, -1, err
		}
		var key string
		if err := json.Unmarshal(payload[keyStart:keyEnd], &key); err != nil {
			return -1, -1, err
		}
		index, err = skipJSONSpaceContextV1(ctx, payload, keyEnd)
		if err != nil {
			return -1, -1, err
		}
		if index >= len(payload) || payload[index] != ':' {
			return -1, -1, fmt.Errorf("missing object colon")
		}
		valueStart, err := skipJSONSpaceContextV1(ctx, payload, index+1)
		if err != nil {
			return -1, -1, err
		}
		valueEnd, err := skipJSONValueBytesV1(ctx, payload, valueStart)
		if err != nil {
			return -1, -1, err
		}
		if key == field {
			return valueStart, valueEnd, nil
		}
		index, err = skipJSONSpaceContextV1(ctx, payload, valueEnd)
		if err != nil {
			return -1, -1, err
		}
		if index >= len(payload) {
			return -1, -1, fmt.Errorf("unterminated top-level object")
		}
		if payload[index] == ',' {
			index++
			continue
		}
		if payload[index] == '}' {
			return -1, -1, nil
		}
		return -1, -1, fmt.Errorf("invalid top-level separator")
	}
}

func skipJSONValueBytesV1(ctx context.Context, payload []byte, index int) (int, error) {
	var err error
	index, err = skipJSONSpaceContextV1(ctx, payload, index)
	if err != nil {
		return index, err
	}
	if index >= len(payload) {
		return index, fmt.Errorf("missing JSON value")
	}
	switch payload[index] {
	case '"':
		return skipJSONStringBytesV1(ctx, payload, index)
	case '{', '[':
		stack := []byte{payload[index]}
		for index++; index < len(payload); index++ {
			if index%wireChunkBytesV1 == 0 {
				if err := ctx.Err(); err != nil {
					return index, err
				}
			}
			if payload[index] == '"' {
				end, err := skipJSONStringBytesV1(ctx, payload, index)
				if err != nil {
					return index, err
				}
				index = end - 1
				continue
			}
			switch payload[index] {
			case '{', '[':
				stack = append(stack, payload[index])
			case '}', ']':
				open := stack[len(stack)-1]
				if (open == '{' && payload[index] != '}') || (open == '[' && payload[index] != ']') {
					return index, fmt.Errorf("mismatched JSON container")
				}
				stack = stack[:len(stack)-1]
				if len(stack) == 0 {
					return index + 1, nil
				}
			}
		}
		return index, fmt.Errorf("unterminated JSON container")
	default:
		end := index
		for end < len(payload) && !strings.ContainsRune(" \t\r\n,}]", rune(payload[end])) {
			end++
		}
		if end == index {
			return end, fmt.Errorf("invalid JSON scalar")
		}
		return end, nil
	}
}

func skipJSONStringBytesV1(ctx context.Context, payload []byte, index int) (int, error) {
	if index >= len(payload) || payload[index] != '"' {
		return index, fmt.Errorf("JSON string required")
	}
	for index++; index < len(payload); index++ {
		if index%wireChunkBytesV1 == 0 {
			if err := ctx.Err(); err != nil {
				return index, err
			}
		}
		switch payload[index] {
		case '\\':
			index++
			if index >= len(payload) {
				return index, fmt.Errorf("unterminated JSON escape")
			}
		case '"':
			return index + 1, nil
		case 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31:
			return index, fmt.Errorf("unescaped control character")
		}
	}
	return index, fmt.Errorf("unterminated JSON string")
}

func skipJSONSpaceV1(payload []byte, index int) int {
	for index < len(payload) {
		switch payload[index] {
		case ' ', '\t', '\r', '\n':
			index++
		default:
			return index
		}
	}
	return index
}

func skipJSONSpaceContextV1(ctx context.Context, payload []byte, index int) (int, error) {
	for index < len(payload) {
		if index%wireChunkBytesV1 == 0 {
			if err := ctx.Err(); err != nil {
				return index, err
			}
		}
		switch payload[index] {
		case ' ', '\t', '\r', '\n':
			index++
		default:
			return index, nil
		}
	}
	return index, ctx.Err()
}

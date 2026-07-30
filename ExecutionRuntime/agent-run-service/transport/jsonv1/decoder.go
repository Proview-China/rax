package jsonv1

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/contract"
)

const DefaultMaxPayloadBytesV1 = 1 << 20

type DecoderV1 struct {
	MaxPayloadBytes int
}

func NewDecoderV1(maxPayloadBytes int) DecoderV1 {
	if maxPayloadBytes <= 0 {
		maxPayloadBytes = DefaultMaxPayloadBytesV1
	}
	return DecoderV1{MaxPayloadBytes: maxPayloadBytes}
}

// DecodeStrictV1 rejects ambiguous JSON before DTO validation: bounded size,
// duplicate keys at any depth, unknown fields and trailing documents.
func (d DecoderV1) DecodeStrictV1(payload []byte, target any) error {
	maximum := d.MaxPayloadBytes
	if maximum <= 0 {
		maximum = DefaultMaxPayloadBytesV1
	}
	if len(payload) == 0 || len(payload) > maximum {
		return contract.NewError(contract.FaultInvalidArgumentV1, "json_payload_size_invalid", "JSON payload is empty or exceeds the configured size limit")
	}
	if target == nil || reflect.ValueOf(target).Kind() != reflect.Pointer || reflect.ValueOf(target).IsNil() {
		return contract.NewError(contract.FaultInvalidArgumentV1, "json_target_invalid", "strict JSON target must be a non-nil pointer")
	}
	if err := rejectDuplicateKeysV1(payload); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return contract.NewError(contract.FaultInvalidArgumentV1, "json_decode_failed", err.Error())
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return contract.NewError(contract.FaultInvalidArgumentV1, "json_trailing_document", "JSON payload contains a trailing document")
	}
	return nil
}

func rejectDuplicateKeysV1(payload []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := scanJSONValueV1(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return contract.NewError(contract.FaultInvalidArgumentV1, "json_trailing_document", "JSON payload contains a trailing document")
	}
	return nil
}

func scanJSONValueV1(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return contract.NewError(contract.FaultInvalidArgumentV1, "json_syntax_invalid", err.Error())
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return contract.NewError(contract.FaultInvalidArgumentV1, "json_syntax_invalid", err.Error())
			}
			key, ok := keyToken.(string)
			if !ok {
				return contract.NewError(contract.FaultInvalidArgumentV1, "json_object_key_invalid", "JSON object key must be a string")
			}
			if _, exists := seen[key]; exists {
				return contract.NewError(contract.FaultInvalidArgumentV1, "json_duplicate_key", "JSON object contains duplicate key: "+key)
			}
			seen[key] = struct{}{}
			if err := scanJSONValueV1(decoder); err != nil {
				return err
			}
		}
		if closing, err := decoder.Token(); err != nil || closing != json.Delim('}') {
			return contract.NewError(contract.FaultInvalidArgumentV1, "json_syntax_invalid", "JSON object is not closed")
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValueV1(decoder); err != nil {
				return err
			}
		}
		if closing, err := decoder.Token(); err != nil || closing != json.Delim(']') {
			return contract.NewError(contract.FaultInvalidArgumentV1, "json_syntax_invalid", "JSON array is not closed")
		}
	default:
		return contract.NewError(contract.FaultInvalidArgumentV1, "json_syntax_invalid", "unexpected JSON delimiter")
	}
	return nil
}

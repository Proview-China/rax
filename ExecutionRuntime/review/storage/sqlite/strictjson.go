package sqlite

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// decodeSnapshotStrictV1 mirrors the public strict JSON rules without the
// public 1 MiB document limit; tenant snapshots have their own 64 MiB bound.
func decodeSnapshotStrictV1(payload []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := scanSnapshotJSONV1(decoder); err != nil {
		return err
	}
	decoder = json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "review sqlite snapshot does not match its schema")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "review sqlite snapshot contains trailing data")
	}
	return nil
}

func scanSnapshotJSONV1(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "review sqlite snapshot JSON is invalid")
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
				return core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "review sqlite snapshot object key is invalid")
			}
			key, ok := keyToken.(string)
			if !ok {
				return core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "review sqlite snapshot object key is not a string")
			}
			if _, exists := seen[key]; exists {
				return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "review sqlite snapshot contains a duplicate key")
			}
			seen[key] = struct{}{}
			if err := scanSnapshotJSONV1(decoder); err != nil {
				return err
			}
		}
		if _, err := decoder.Token(); err != nil {
			return core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "review sqlite snapshot object is unterminated")
		}
	case '[':
		for decoder.More() {
			if err := scanSnapshotJSONV1(decoder); err != nil {
				return err
			}
		}
		if _, err := decoder.Token(); err != nil {
			return core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "review sqlite snapshot array is unterminated")
		}
	default:
		return core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "review sqlite snapshot delimiter is invalid")
	}
	return nil
}

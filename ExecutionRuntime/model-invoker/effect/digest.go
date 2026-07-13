package effect

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
)

func digestBytes(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func digestValue(value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return digestBytes(payload), nil
}

func decodeStrictJSON(payload []byte) (any, error) {
	if len(bytes.TrimSpace(payload)) == 0 {
		return nil, ErrInvalidJSON
	}
	if err := rejectDuplicateKeys(payload); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("%w: multiple JSON values", ErrInvalidJSON)
		}
		return nil, fmt.Errorf("%w: trailing data: %v", ErrInvalidJSON, err)
	}
	return value, nil
}

func rejectDuplicateKeys(payload []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := scanJSONValue(decoder, "$"); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("%w: trailing token", ErrInvalidJSON)
		}
		return fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder, location string) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return fmt.Errorf("%w: %v", ErrInvalidJSON, err)
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("%w: object key at %s is not a string", ErrInvalidJSON, location)
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("%w: duplicate key %q at %s", ErrInvalidJSON, key, location)
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(decoder, location+"."+key); err != nil {
				return err
			}
		}
		if closing, err := decoder.Token(); err != nil || closing != json.Delim('}') {
			return fmt.Errorf("%w: object at %s is not closed", ErrInvalidJSON, location)
		}
	case '[':
		for index := 0; decoder.More(); index++ {
			if err := scanJSONValue(decoder, fmt.Sprintf("%s[%d]", location, index)); err != nil {
				return err
			}
		}
		if closing, err := decoder.Token(); err != nil || closing != json.Delim(']') {
			return fmt.Errorf("%w: array at %s is not closed", ErrInvalidJSON, location)
		}
	default:
		return fmt.Errorf("%w: unexpected delimiter %q", ErrInvalidJSON, delimiter)
	}
	return nil
}

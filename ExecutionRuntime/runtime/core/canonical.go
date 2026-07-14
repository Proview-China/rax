package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const MaxCanonicalDocumentBytes = 1 << 20

// CanonicalJSONDigest hashes normalized, struct-backed JSON with explicit
// domain separation. Callers must pass a value whose collection ordering and
// nil/empty semantics have already been normalized by its owning contract.
func CanonicalJSONDigest(domain, version, discriminator string, normalized any) (Digest, error) {
	if !validCanonicalLabel(domain) || !validCanonicalLabel(version) || !validCanonicalLabel(discriminator) {
		return "", NewError(ErrorInvalidArgument, ReasonInvalidCanonicalForm, "canonical digest domain, version and discriminator are required ASCII labels")
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return "", NewError(ErrorInvalidArgument, ReasonInvalidCanonicalForm, "canonical value is not JSON serializable")
	}
	if len(payload) > MaxCanonicalDocumentBytes {
		return "", NewError(ErrorInvalidArgument, ReasonCanonicalLimitExceeded, "canonical document exceeds the bounded size")
	}
	hash := sha256.New()
	writeCanonicalFrame(hash, []byte("praxis-canonical-v1"))
	writeCanonicalFrame(hash, []byte(domain))
	writeCanonicalFrame(hash, []byte(version))
	writeCanonicalFrame(hash, []byte(discriminator))
	writeCanonicalFrame(hash, payload)
	return Digest("sha256:" + hex.EncodeToString(hash.Sum(nil))), nil
}

// DecodeStrictJSON rejects duplicate object keys, unknown struct fields,
// trailing documents and unbounded inputs. Forward-compatible data must use a
// contract-defined opaque extension envelope instead of undeclared fields.
func DecodeStrictJSON(payload []byte, target any) error {
	if len(payload) == 0 || len(payload) > MaxCanonicalDocumentBytes {
		return NewError(ErrorInvalidArgument, ReasonCanonicalLimitExceeded, "JSON document is empty or exceeds the bounded size")
	}
	if err := rejectDuplicateJSONKeys(payload); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return NewError(ErrorInvalidArgument, ReasonInvalidCanonicalForm, "JSON document does not match the declared contract")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return NewError(ErrorInvalidArgument, ReasonInvalidCanonicalForm, "JSON document contains trailing data")
	}
	return nil
}

func DigestBytes(payload []byte) Digest {
	sum := sha256.Sum256(payload)
	return Digest("sha256:" + hex.EncodeToString(sum[:]))
}

func writeCanonicalFrame(writer io.Writer, payload []byte) {
	// Fixed-width hexadecimal length avoids signedness, platform width and
	// varint-canonicality ambiguity. The canonical document itself is bounded.
	_, _ = fmt.Fprintf(writer, "%016x:", uint64(len(payload)))
	_, _ = writer.Write(payload)
}

func validCanonicalLabel(value string) bool {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range []byte(value) {
		if character < 0x21 || character > 0x7e {
			return false
		}
	}
	return true
}

func rejectDuplicateJSONKeys(payload []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	return scanJSONValue(decoder)
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return NewError(ErrorInvalidArgument, ReasonInvalidCanonicalForm, "invalid JSON document")
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return NewError(ErrorInvalidArgument, ReasonInvalidCanonicalForm, "invalid JSON object key")
			}
			key, ok := keyToken.(string)
			if !ok {
				return NewError(ErrorInvalidArgument, ReasonInvalidCanonicalForm, "JSON object key must be a string")
			}
			if _, exists := seen[key]; exists {
				return NewError(ErrorConflict, ReasonDuplicateCanonicalKey, "JSON object contains a duplicate key")
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		if _, err := decoder.Token(); err != nil {
			return NewError(ErrorInvalidArgument, ReasonInvalidCanonicalForm, "unterminated JSON object")
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		if _, err := decoder.Token(); err != nil {
			return NewError(ErrorInvalidArgument, ReasonInvalidCanonicalForm, "unterminated JSON array")
		}
	default:
		return NewError(ErrorInvalidArgument, ReasonInvalidCanonicalForm, "unexpected JSON delimiter")
	}
	return nil
}

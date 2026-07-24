package contract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const Version = "praxis.context-engine/v1"

var (
	ErrInvalid       = errors.New("context contract: invalid")
	ErrConflict      = errors.New("context contract: conflict")
	ErrExpired       = errors.New("context contract: expired")
	ErrUnauthorized  = errors.New("context contract: unauthorized")
	ErrUnsupported   = errors.New("context contract: unsupported")
	ErrUnknown       = errors.New("context contract: unknown outcome")
	ErrNotFound      = errors.New("context contract: not found")
	ErrUnavailable   = errors.New("context contract: unavailable")
	ErrInspectOnly   = errors.New("context contract: inspect only")
	ErrLimitExceeded = errors.New("context contract: limit exceeded")
)

type Digest string

func DigestBytes(value []byte) Digest {
	sum := sha256.Sum256(value)
	return Digest("sha256:" + hex.EncodeToString(sum[:]))
}

func DigestJSON(value any) (Digest, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("%w: canonical json: %v", ErrInvalid, err)
	}
	return DigestBytes(payload), nil
}

func DecodeStrict[T any](payload []byte) (T, error) {
	var value T
	if err := rejectDuplicateJSONKeys(payload); err != nil {
		return value, err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, fmt.Errorf("%w: strict json: %v", ErrInvalid, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return value, fmt.Errorf("%w: trailing json", ErrInvalid)
	}
	return value, nil
}

func rejectDuplicateJSONKeys(payload []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := scanJSONValue(decoder); err != nil {
		return fmt.Errorf("%w: duplicate-key scan: %v", ErrInvalid, err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: trailing json", ErrInvalid)
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
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
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate key %q", key)
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return errors.New("unterminated object")
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return errors.New("unterminated array")
		}
	default:
		return errors.New("unexpected delimiter")
	}
	return nil
}

func (d Digest) Validate() error {
	value := string(d)
	if !strings.HasPrefix(value, "sha256:") {
		return fmt.Errorf("%w: digest prefix", ErrInvalid)
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	if err != nil || len(raw) != sha256.Size {
		return fmt.Errorf("%w: digest width", ErrInvalid)
	}
	return nil
}

type FactRef struct {
	ID       string `json:"id"`
	Revision uint64 `json:"revision"`
	Digest   Digest `json:"digest"`
}

func (r FactRef) Validate() error {
	if err := validateID(r.ID); err != nil || r.Revision == 0 || r.Digest.Validate() != nil {
		return fmt.Errorf("%w: fact reference", ErrInvalid)
	}
	return nil
}

type OwnerRef struct {
	ComponentID   string `json:"component_id"`
	BindingDigest Digest `json:"binding_digest"`
}

func (r OwnerRef) Validate() error {
	if err := validateName(r.ComponentID); err != nil || r.BindingDigest.Validate() != nil {
		return fmt.Errorf("%w: owner reference", ErrInvalid)
	}
	return nil
}

type EvidenceRef struct {
	ID     string `json:"id"`
	Digest Digest `json:"digest"`
}

func (r EvidenceRef) Validate() error {
	if err := validateID(r.ID); err != nil || r.Digest.Validate() != nil {
		return fmt.Errorf("%w: evidence reference", ErrInvalid)
	}
	return nil
}

// ExecutionBinding references Runtime-owned facts by digest without copying
// Runtime's Identity, Scope, Authority or Run ownership into this module.
type ExecutionBinding struct {
	ScopeDigest     Digest `json:"scope_digest"`
	RunID           string `json:"run_id"`
	Turn            uint32 `json:"turn"`
	AuthorityDigest Digest `json:"authority_digest"`
}

func (b ExecutionBinding) Validate() error {
	if b.ScopeDigest.Validate() != nil || b.AuthorityDigest.Validate() != nil || validateID(b.RunID) != nil || b.Turn == 0 {
		return fmt.Errorf("%w: execution binding", ErrInvalid)
	}
	return nil
}

type ContentRef struct {
	Ref    string `json:"ref"`
	Digest Digest `json:"digest"`
	Length uint64 `json:"length"`
}

func (r ContentRef) Validate() error {
	if err := validateID(r.Ref); err != nil || r.Digest.Validate() != nil || r.Length == 0 {
		return fmt.Errorf("%w: content reference", ErrInvalid)
	}
	return nil
}

func ValidateContract(contract string) error {
	if contract != Version {
		return fmt.Errorf("%w: contract version", ErrUnsupported)
	}
	return nil
}

func validateID(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 256 {
		return fmt.Errorf("%w: bounded id", ErrInvalid)
	}
	for _, r := range value {
		if r < 0x21 || r > 0x7e {
			return fmt.Errorf("%w: id must be visible ascii", ErrInvalid)
		}
	}
	return nil
}

func validateName(value string) error {
	if err := validateID(value); err != nil || !strings.Contains(value, "/") {
		return fmt.Errorf("%w: namespaced name", ErrInvalid)
	}
	return nil
}

func validateTimes(created, expires int64) error {
	if created <= 0 || expires <= created {
		return fmt.Errorf("%w: lifetime", ErrInvalid)
	}
	return nil
}

package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

const (
	ContractVersionV1 = "praxis.agent-host/v1"
	ContractVersionV2 = "praxis.agent-host/v2"
)

type ErrorCode string

const (
	ErrorInvalidArgument ErrorCode = "invalid_argument"
	ErrorConflict        ErrorCode = "conflict"
	ErrorNotFound        ErrorCode = "not_found"
	ErrorUnavailable     ErrorCode = "unavailable"
	ErrorPrecondition    ErrorCode = "precondition_failed"
	ErrorUnknownOutcome  ErrorCode = "unknown_outcome"
)

type Error struct {
	Code    ErrorCode
	Reason  string
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("agent-host: %s: %s: %s", e.Code, e.Reason, e.Message)
}

func NewError(code ErrorCode, reason, message string) error {
	return &Error{Code: code, Reason: reason, Message: message}
}

func HasCode(err error, code ErrorCode) bool {
	var target *Error
	return errors.As(err, &target) && target.Code == code
}

var identifierV1 = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@-]{0,255}$`)

func ValidateIdentifierV1(field, value string) error {
	if value != strings.TrimSpace(value) || !identifierV1.MatchString(value) {
		return NewError(ErrorInvalidArgument, "invalid_reference", field+" must be a canonical non-empty identifier")
	}
	return nil
}

type DigestV1 string

func (d DigestV1) Validate() error {
	if !strings.HasPrefix(string(d), "sha256:") {
		return NewError(ErrorInvalidArgument, "invalid_digest", "digest must be sha256:<64 lowercase hex>")
	}
	value := strings.TrimPrefix(string(d), "sha256:")
	if len(value) != sha256.Size*2 {
		return NewError(ErrorInvalidArgument, "invalid_digest", "digest must be sha256:<64 lowercase hex>")
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size || value != strings.ToLower(value) {
		return NewError(ErrorInvalidArgument, "invalid_digest", "digest must be sha256:<64 lowercase hex>")
	}
	return nil
}

func DigestJSONV1(value any) (DigestV1, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", NewError(ErrorInvalidArgument, "canonical_encoding_failed", err.Error())
	}
	sum := sha256.Sum256(encoded)
	return DigestV1("sha256:" + hex.EncodeToString(sum[:])), nil
}

func IsTypedNilV1(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

type ExactRefV1 struct {
	Kind     string   `json:"kind"`
	ID       string   `json:"id"`
	Revision uint64   `json:"revision"`
	Digest   DigestV1 `json:"digest"`
}

func (r ExactRefV1) Validate() error {
	if err := ValidateIdentifierV1("ref kind", r.Kind); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("ref id", r.ID); err != nil {
		return err
	}
	if r.Revision == 0 {
		return NewError(ErrorInvalidArgument, "invalid_revision", "ref revision must be positive")
	}
	return r.Digest.Validate()
}

func SameExactRefV1(a, b ExactRefV1) bool {
	return a == b
}

package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

type FaultCodeV1 string

const (
	FaultInvalidArgumentV1       FaultCodeV1 = "INVALID_ARGUMENT"
	FaultUnauthenticatedV1       FaultCodeV1 = "UNAUTHENTICATED"
	FaultForbiddenV1             FaultCodeV1 = "FORBIDDEN"
	FaultNotFoundV1              FaultCodeV1 = "NOT_FOUND"
	FaultRevisionConflictV1      FaultCodeV1 = "REVISION_CONFLICT"
	FaultIdempotencyConflictV1   FaultCodeV1 = "IDEMPOTENCY_CONFLICT"
	FaultPreconditionFailedV1    FaultCodeV1 = "PRECONDITION_FAILED"
	FaultCapabilityUnavailableV1 FaultCodeV1 = "CAPABILITY_UNAVAILABLE"
	FaultUnavailableV1           FaultCodeV1 = "UNAVAILABLE"
	FaultRateLimitedV1           FaultCodeV1 = "RATE_LIMITED"
	FaultUnknownOutcomeV1        FaultCodeV1 = "UNKNOWN_OUTCOME"
	FaultIndeterminateV1         FaultCodeV1 = "INDETERMINATE"
	FaultInternalV1              FaultCodeV1 = "INTERNAL"
)

func validFaultCodeV1(code FaultCodeV1) bool {
	switch code {
	case FaultInvalidArgumentV1, FaultUnauthenticatedV1, FaultForbiddenV1, FaultNotFoundV1,
		FaultRevisionConflictV1, FaultIdempotencyConflictV1, FaultPreconditionFailedV1,
		FaultCapabilityUnavailableV1, FaultUnavailableV1, FaultRateLimitedV1,
		FaultUnknownOutcomeV1, FaultIndeterminateV1, FaultInternalV1:
		return true
	default:
		return false
	}
}

type Error struct {
	Code    FaultCodeV1
	Reason  string
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("agent-run-service: %s: %s: %s", e.Code, e.Reason, e.Message)
}

func NewError(code FaultCodeV1, reason, message string) error {
	return &Error{Code: code, Reason: reason, Message: message}
}

func HasCode(err error, code FaultCodeV1) bool {
	var target *Error
	return errors.As(err, &target) && target.Code == code
}

var identifierV1 = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@-]{0,255}$`)

func ValidateIdentifierV1(field, value string) error {
	if value != strings.TrimSpace(value) || !identifierV1.MatchString(value) {
		return NewError(FaultInvalidArgumentV1, "invalid_reference", field+" must be a canonical non-empty identifier")
	}
	return nil
}

func ValidateNamespacedIdentifierV1(field, value string) error {
	if err := ValidateIdentifierV1(field, value); err != nil {
		return err
	}
	separator := strings.IndexAny(value, "./:")
	if separator <= 0 || separator == len(value)-1 {
		return NewError(FaultInvalidArgumentV1, "identifier_namespace_missing", field+" must contain a non-empty namespace and local name")
	}
	return nil
}

func validateOptionalContractVersionV1(value, expected string) error {
	if value != "" && value != expected {
		return NewError(FaultRevisionConflictV1, "contract_version_mismatch", "supplied contract version drifted")
	}
	return nil
}

type DigestV1 string

func (d DigestV1) Validate() error {
	if !strings.HasPrefix(string(d), "sha256:") {
		return NewError(FaultInvalidArgumentV1, "invalid_digest", "digest must be sha256:<64 lowercase hex>")
	}
	value := strings.TrimPrefix(string(d), "sha256:")
	decoded, err := hex.DecodeString(value)
	if err != nil || len(value) != sha256.Size*2 || len(decoded) != sha256.Size || value != strings.ToLower(value) {
		return NewError(FaultInvalidArgumentV1, "invalid_digest", "digest must be sha256:<64 lowercase hex>")
	}
	return nil
}

func DigestJSONV1(value any) (DigestV1, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", NewError(FaultInvalidArgumentV1, "canonical_encoding_failed", err.Error())
	}
	sum := sha256.Sum256(encoded)
	return DigestV1("sha256:" + hex.EncodeToString(sum[:])), nil
}

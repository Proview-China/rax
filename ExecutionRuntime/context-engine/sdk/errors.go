package sdk

import (
	"context"
	"errors"
	"fmt"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

type OfflineSDKErrorCodeV1 string

const (
	OfflineErrorInvalidArgumentV1  OfflineSDKErrorCodeV1 = "invalid_argument"
	OfflineErrorLimitExceededV1    OfflineSDKErrorCodeV1 = "limit_exceeded"
	OfflineErrorNotFoundV1         OfflineSDKErrorCodeV1 = "not_found"
	OfflineErrorConflictV1         OfflineSDKErrorCodeV1 = "conflict"
	OfflineErrorExpiredV1          OfflineSDKErrorCodeV1 = "expired"
	OfflineErrorUnauthorizedV1     OfflineSDKErrorCodeV1 = "unauthorized"
	OfflineErrorUnsupportedV1      OfflineSDKErrorCodeV1 = "unsupported"
	OfflineErrorCanceledV1         OfflineSDKErrorCodeV1 = "canceled"
	OfflineErrorDeadlineExceededV1 OfflineSDKErrorCodeV1 = "deadline_exceeded"
	OfflineErrorInternalFailureV1  OfflineSDKErrorCodeV1 = "internal_failure"
)

type OfflineSDKErrorV1 struct {
	Code      OfflineSDKErrorCodeV1 `json:"code"`
	Operation OfflineSDKOperationV1 `json:"operation"`
	FieldPath string                `json:"field_path"`
	Message   string                `json:"message"`
	cause     error
}

func (e *OfflineSDKErrorV1) Error() string {
	return fmt.Sprintf("context offline sdk %s: %s", e.Code, e.Message)
}

func (e *OfflineSDKErrorV1) Unwrap() error { return e.cause }

func sdkErrorV1(code OfflineSDKErrorCodeV1, op OfflineSDKOperationV1, path, message string, cause error) error {
	return &OfflineSDKErrorV1{Code: code, Operation: op, FieldPath: path, Message: message, cause: cause}
}

// UnsupportedOperationErrorV1 returns the stable typed error used by
// owner-local dispatch surfaces when the requested offline operation is not in
// the V1 closed set.
func UnsupportedOperationErrorV1(op OfflineSDKOperationV1) error {
	return sdkErrorV1(OfflineErrorUnsupportedV1, op, "operation", "unsupported offline operation", contract.ErrUnsupported)
}

func mapErrorV1(op OfflineSDKOperationV1, path string, err error) error {
	if err == nil {
		return nil
	}
	var sdkErr *OfflineSDKErrorV1
	if errors.As(err, &sdkErr) {
		return err
	}
	switch {
	case errors.Is(err, context.Canceled):
		return sdkErrorV1(OfflineErrorCanceledV1, op, path, "operation canceled", context.Canceled)
	case errors.Is(err, context.DeadlineExceeded):
		return sdkErrorV1(OfflineErrorDeadlineExceededV1, op, path, "deadline exceeded", context.DeadlineExceeded)
	case errors.Is(err, contract.ErrLimitExceeded):
		return sdkErrorV1(OfflineErrorLimitExceededV1, op, path, "limit exceeded", err)
	case errors.Is(err, contract.ErrNotFound):
		return sdkErrorV1(OfflineErrorNotFoundV1, op, path, "required content not found", err)
	case errors.Is(err, contract.ErrConflict):
		return sdkErrorV1(OfflineErrorConflictV1, op, path, "content or binding conflict", err)
	case errors.Is(err, contract.ErrExpired):
		return sdkErrorV1(OfflineErrorExpiredV1, op, path, "content or binding expired", err)
	case errors.Is(err, contract.ErrUnauthorized):
		return sdkErrorV1(OfflineErrorUnauthorizedV1, op, path, "content or binding unauthorized", err)
	case errors.Is(err, contract.ErrUnsupported):
		return sdkErrorV1(OfflineErrorUnsupportedV1, op, path, "unsupported operation or mode", err)
	case errors.Is(err, contract.ErrInvalid):
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, path, "invalid argument", err)
	default:
		return sdkErrorV1(OfflineErrorInternalFailureV1, op, path, "internal failure", err)
	}
}

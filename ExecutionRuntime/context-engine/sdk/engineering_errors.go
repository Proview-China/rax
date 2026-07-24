package sdk

import (
	"context"
	"errors"
	"fmt"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

type ContextEngineeringErrorCodeV1 string

const (
	EngineeringErrorInvalidArgumentV1  ContextEngineeringErrorCodeV1 = "invalid_argument"
	EngineeringErrorLimitExceededV1    ContextEngineeringErrorCodeV1 = "limit_exceeded"
	EngineeringErrorConflictV1         ContextEngineeringErrorCodeV1 = "conflict"
	EngineeringErrorExpiredV1          ContextEngineeringErrorCodeV1 = "expired"
	EngineeringErrorUnauthorizedV1     ContextEngineeringErrorCodeV1 = "unauthorized"
	EngineeringErrorNotFoundV1         ContextEngineeringErrorCodeV1 = "not_found"
	EngineeringErrorUnavailableV1      ContextEngineeringErrorCodeV1 = "unavailable"
	EngineeringErrorUnknownV1          ContextEngineeringErrorCodeV1 = "unknown"
	EngineeringErrorCanceledV1         ContextEngineeringErrorCodeV1 = "canceled"
	EngineeringErrorDeadlineExceededV1 ContextEngineeringErrorCodeV1 = "deadline_exceeded"
	EngineeringErrorUnsupportedV1      ContextEngineeringErrorCodeV1 = "unsupported"
	EngineeringErrorInternalFailureV1  ContextEngineeringErrorCodeV1 = "internal_failure"
)

type ContextEngineeringErrorV1 struct {
	Code      ContextEngineeringErrorCodeV1 `json:"code"`
	Operation ContextEngineeringOperationV1 `json:"operation"`
	FieldPath string                        `json:"field_path"`
	Message   string                        `json:"message"`
	cause     error
}

func (e *ContextEngineeringErrorV1) Error() string {
	return fmt.Sprintf("context engineering sdk %s: %s", e.Code, e.Message)
}

func (e *ContextEngineeringErrorV1) Unwrap() error { return e.cause }

func engineeringErrorV1(code ContextEngineeringErrorCodeV1, op ContextEngineeringOperationV1, path, message string, cause error) error {
	return &ContextEngineeringErrorV1{Code: code, Operation: op, FieldPath: path, Message: message, cause: cause}
}

// UnsupportedEngineeringOperationErrorV1 preserves the engineering SDK's
// nominal error closure for transport-neutral dispatchers.
func UnsupportedEngineeringOperationErrorV1(op ContextEngineeringOperationV1) error {
	return engineeringErrorV1(EngineeringErrorUnsupportedV1, op, "operation", "unsupported operation or mode", contract.ErrUnsupported)
}

func mapEngineeringErrorV1(op ContextEngineeringOperationV1, path string, err error) error {
	if err == nil {
		return nil
	}
	var typed *ContextEngineeringErrorV1
	if errors.As(err, &typed) {
		return err
	}
	switch {
	case errors.Is(err, context.Canceled):
		return engineeringErrorV1(EngineeringErrorCanceledV1, op, path, "operation canceled", context.Canceled)
	case errors.Is(err, context.DeadlineExceeded):
		return engineeringErrorV1(EngineeringErrorDeadlineExceededV1, op, path, "deadline exceeded", context.DeadlineExceeded)
	case errors.Is(err, contract.ErrLimitExceeded):
		return engineeringErrorV1(EngineeringErrorLimitExceededV1, op, path, "limit exceeded", err)
	case errors.Is(err, contract.ErrConflict):
		return engineeringErrorV1(EngineeringErrorConflictV1, op, path, "content or binding conflict", err)
	case errors.Is(err, contract.ErrExpired):
		return engineeringErrorV1(EngineeringErrorExpiredV1, op, path, "content or binding expired", err)
	case errors.Is(err, contract.ErrUnauthorized):
		return engineeringErrorV1(EngineeringErrorUnauthorizedV1, op, path, "content or binding unauthorized", err)
	case errors.Is(err, contract.ErrNotFound):
		return engineeringErrorV1(EngineeringErrorNotFoundV1, op, path, "required exact input not found", err)
	case errors.Is(err, contract.ErrUnavailable):
		return engineeringErrorV1(EngineeringErrorUnavailableV1, op, path, "evaluator or input unavailable", err)
	case errors.Is(err, contract.ErrUnknown):
		return engineeringErrorV1(EngineeringErrorUnknownV1, op, path, "unknown evaluator outcome", err)
	case errors.Is(err, contract.ErrUnsupported):
		return engineeringErrorV1(EngineeringErrorUnsupportedV1, op, path, "unsupported operation or mode", err)
	case errors.Is(err, contract.ErrInvalid):
		return engineeringErrorV1(EngineeringErrorInvalidArgumentV1, op, path, "invalid argument", err)
	default:
		return engineeringErrorV1(EngineeringErrorInternalFailureV1, op, path, "internal failure", err)
	}
}

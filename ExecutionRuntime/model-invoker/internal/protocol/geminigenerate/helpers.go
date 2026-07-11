package geminigenerate

import (
	"regexp"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

var nativeToolNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.:-]{0,127}$`)

func driverError(kind modelinvoker.ErrorKind, operation, message string) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: kind, Operation: operation, Message: message}
}

func mappingError(operation, message string) *modelinvoker.Error {
	return driverError(modelinvoker.ErrorMapping, operation, message)
}

func mappingErrorWithRequestID(operation, message, requestID string) *modelinvoker.Error {
	err := mappingError(operation, message)
	err.RequestID = requestID
	return err
}

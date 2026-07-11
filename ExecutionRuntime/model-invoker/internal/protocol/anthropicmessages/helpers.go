package anthropicmessages

import modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"

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

func transformed(capability modelinvoker.Capability, detail string) modelinvoker.MappingDecision {
	return modelinvoker.MappingDecision{Capability: capability, Action: modelinvoker.MappingTransformed, Detail: detail}
}

func degradation(capability modelinvoker.Capability, detail string) modelinvoker.MappingDecision {
	return modelinvoker.MappingDecision{Capability: capability, Action: modelinvoker.MappingDegraded, Detail: detail}
}

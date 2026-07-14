package conformance

import (
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

var operationDomainAdapterAllowedImportsV3 = [...]string{
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core",
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports",
	"github.com/Proview-China/rax/ExecutionRuntime/application/contract",
	"github.com/Proview-China/rax/ExecutionRuntime/application/ports",
}

func OperationDomainAdapterAllowedImportsV3() []string {
	result := make([]string, len(operationDomainAdapterAllowedImportsV3))
	copy(result, operationDomainAdapterAllowedImportsV3[:])
	return result
}

// CheckOperationDomainAdapterImportsV3 is a build/test hygiene check only. A
// passing result grants no Binding, production, dispatch or commit authority.
func CheckOperationDomainAdapterImportsV3(imports []string) error {
	allowed := OperationDomainAdapterAllowedImportsV3()
	for _, candidate := range imports {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "operation domain adapter import is empty")
		}
		if !strings.HasPrefix(candidate, "github.com/Proview-China/rax/ExecutionRuntime/") {
			continue
		}
		permitted := false
		for _, prefix := range allowed {
			if candidate == prefix || strings.HasPrefix(candidate, prefix+"/") {
				permitted = true
				break
			}
		}
		if !permitted {
			return core.NewError(core.ErrorForbidden, core.ReasonComponentMismatch, "operation domain adapter imports a forbidden implementation package")
		}
	}
	return nil
}

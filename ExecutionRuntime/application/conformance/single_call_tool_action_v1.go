package conformance

import (
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

var singleCallToolActionAdapterAllowedImportsV1 = [...]string{
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core",
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports",
	"github.com/Proview-China/rax/ExecutionRuntime/application/contract",
	"github.com/Proview-China/rax/ExecutionRuntime/application/ports",
}

func SingleCallToolActionAdapterAllowedImportsV1() []string {
	result := make([]string, len(singleCallToolActionAdapterAllowedImportsV1))
	copy(result, singleCallToolActionAdapterAllowedImportsV1[:])
	return result
}

// CheckSingleCallToolActionAdapterImportsV1 is build hygiene only. Passing it
// grants no Binding, Tool execution, Provider access or production claim.
func CheckSingleCallToolActionAdapterImportsV1(imports []string) error {
	allowed := SingleCallToolActionAdapterAllowedImportsV1()
	for _, candidate := range imports {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call adapter import is empty")
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
			return core.NewError(core.ErrorForbidden, core.ReasonComponentMismatch, "single-call adapter imports an Owner implementation package")
		}
	}
	return nil
}

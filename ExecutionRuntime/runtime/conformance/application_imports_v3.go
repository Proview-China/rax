package conformance

import (
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

var applicationAllowedRuntimeImportsV3 = [...]string{
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core",
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports",
}

func ApplicationAllowedRuntimeImportsV3() []string {
	return append([]string(nil), applicationAllowedRuntimeImportsV3[:]...)
}

// CheckApplicationRuntimeImportsV3 is the build/conformance boundary. The
// control/kernel/foundation/fakes packages are Runtime implementation detail;
// aliases in control exist only for legacy tests and migrations.
func CheckApplicationRuntimeImportsV3(imports []string) error {
	for _, path := range imports {
		if !strings.Contains(path, "/ExecutionRuntime/runtime/") {
			continue
		}
		allowed := false
		for _, candidate := range applicationAllowedRuntimeImportsV3 {
			if path == candidate {
				allowed = true
				break
			}
		}
		if !allowed {
			return core.NewError(core.ErrorForbidden, core.ReasonEffectAuthorizationMissing, "Application imports a Runtime implementation package")
		}
	}
	return nil
}

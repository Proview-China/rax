package operation

import (
	"fmt"
	"slices"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

func Evaluate(request Request, contract CapabilityContract) (MappingReport, error) {
	report := MappingReport{Provider: request.Provider, Kind: request.Kind, Model: request.Model}
	support, ok := contract[request.Kind]
	if !ok {
		support = Capability{Level: SupportUnsupported, Limitations: []string{"operation is not declared"}}
	}
	if len(support.Models) > 0 && !slices.Contains(support.Models, request.Model) {
		support.Level = SupportUnsupported
		support.Limitations = append(append([]string(nil), support.Limitations...), "model is outside the operation allowlist")
	}
	report.Detail = strings.Join(support.Limitations, "; ")
	switch support.Level {
	case SupportNative:
		report.Action = MappingExact
	case SupportCompatible:
		report.Action = MappingTransformed
	case SupportPartial:
		if request.AllowDegradation {
			report.Action = MappingDegraded
			return report, nil
		}
		report.Action = MappingRejected
		return report, operationError(request.Provider, modelinvoker.ErrorUnsupportedCapability, "capability_check", string(request.Kind), "partial operation support requires explicit degradation permission")
	case SupportUnsupported, "":
		report.Action = MappingRejected
		return report, operationError(request.Provider, modelinvoker.ErrorUnsupportedCapability, "capability_check", string(request.Kind), "operation is unsupported")
	default:
		report.Action = MappingRejected
		return report, operationError(request.Provider, modelinvoker.ErrorMapping, "capability_check", string(request.Kind), fmt.Sprintf("unknown support level %q", support.Level))
	}
	return report, nil
}

func operationError(provider modelinvoker.ProviderID, kind modelinvoker.ErrorKind, operation, code, message string) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: kind, Provider: provider, Operation: operation, Code: code, Message: message}
}

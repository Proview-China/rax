package adaptercore

import modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"

// KnownCapabilities returns the complete capability vocabulary understood by
// the current public contract.
func KnownCapabilities() []modelinvoker.Capability {
	return modelinvoker.AllCapabilities()
}

// UnsupportedContract creates a complete contract so an omitted declaration
// can never be mistaken for support.
func UnsupportedContract(reason string) modelinvoker.CapabilityContract {
	capabilities := modelinvoker.AllCapabilities()
	contract := make(modelinvoker.CapabilityContract, len(capabilities))
	for _, capability := range capabilities {
		contract[capability] = modelinvoker.CapabilitySupport{
			Level:       modelinvoker.SupportUnsupported,
			Limitations: []string{reason},
		}
	}
	return contract
}

// QuerySupport binds a support declaration to the exact protocol and model
// used for the capability query.
func QuerySupport(query modelinvoker.CapabilityQuery, level modelinvoker.SupportLevel, limitation string) modelinvoker.CapabilitySupport {
	return modelinvoker.CapabilitySupport{
		Level:       level,
		Limitations: []string{limitation},
		Protocols:   []modelinvoker.Protocol{query.Protocol},
		Models:      []string{query.Model},
	}
}

// SetSupport applies the same query-scoped declaration to each capability.
func SetSupport(
	contract modelinvoker.CapabilityContract,
	query modelinvoker.CapabilityQuery,
	level modelinvoker.SupportLevel,
	limitation string,
	capabilities ...modelinvoker.Capability,
) {
	for _, capability := range capabilities {
		contract[capability] = QuerySupport(query, level, limitation)
	}
}

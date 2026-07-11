package gemini

import (
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
)

func capabilityContract(query modelinvoker.CapabilityQuery) modelinvoker.CapabilityContract {
	contract := adaptercore.UnsupportedContract("outside the implemented Gemini Developer API GenerateContent semantic slice")
	adaptercore.SetSupport(contract, query, modelinvoker.SupportNative,
		"GenerateContent maps this capability natively; Gemini validates model-specific availability at invocation time",
		modelinvoker.CapabilityTextGeneration,
		modelinvoker.CapabilityStreaming,
		modelinvoker.CapabilityToolCalling,
		modelinvoker.CapabilityParallelToolCalling,
		modelinvoker.CapabilityStructuredOutput,
		modelinvoker.CapabilityFunctionErrorResult,
		modelinvoker.CapabilityProviderContinuation,
		modelinvoker.CapabilityUsageReporting,
	)
	adaptercore.SetSupport(contract, query, modelinvoker.SupportCompatible,
		"unified effort or budget maps to Gemini ThinkingConfig; model-family compatibility is validated at invocation time",
		modelinvoker.CapabilityReasoning,
	)
	contract[modelinvoker.CapabilityReasoningSummary] = adaptercore.QuerySupport(query, modelinvoker.SupportPartial,
		"Gemini thought parts are provider-native reasoning, not a guaranteed portable reasoning summary; includeThoughts requires explicit degradation")
	contract[modelinvoker.CapabilityServerState] = adaptercore.QuerySupport(query, modelinvoker.SupportUnsupported,
		"GenerateContent response IDs identify responses but do not continue server-held conversation state")
	return contract
}

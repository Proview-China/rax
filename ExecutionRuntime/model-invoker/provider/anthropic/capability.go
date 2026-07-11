package anthropic

import (
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
)

func capabilityContract(query modelinvoker.CapabilityQuery) modelinvoker.CapabilityContract {
	contract := adaptercore.UnsupportedContract("outside the implemented Anthropic Messages semantic slice")
	adaptercore.SetSupport(contract, query, modelinvoker.SupportNative,
		"adapter mapping is native; Anthropic validates model availability and feature support at invocation time",
		modelinvoker.CapabilityTextGeneration,
		modelinvoker.CapabilityStreaming,
		modelinvoker.CapabilityToolCalling,
		modelinvoker.CapabilityParallelToolCalling,
		modelinvoker.CapabilityFunctionErrorResult,
		modelinvoker.CapabilityProviderContinuation,
		modelinvoker.CapabilityUsageReporting,
	)
	adaptercore.SetSupport(contract, query, modelinvoker.SupportCompatible,
		"Anthropic uses output_config.format; json_object and explicit non-strict schema output remain field-level rejections",
		modelinvoker.CapabilityStructuredOutput,
	)
	adaptercore.SetSupport(contract, query, modelinvoker.SupportCompatible,
		"unified reasoning maps to adaptive or budgeted thinking and output_config.effort with field-level combination checks",
		modelinvoker.CapabilityReasoning,
	)
	adaptercore.SetSupport(contract, query, modelinvoker.SupportCompatible,
		"Anthropic exposes summarized thinking but does not control concise or detailed summary style",
		modelinvoker.CapabilityReasoningSummary,
	)
	contract[modelinvoker.CapabilityServerState] = adaptercore.QuerySupport(query, modelinvoker.SupportUnsupported,
		"Anthropic Messages is stateless and does not accept a server response ID continuation")
	return contract
}

func transformed(capability modelinvoker.Capability, detail string) modelinvoker.MappingDecision {
	return modelinvoker.MappingDecision{Capability: capability, Action: modelinvoker.MappingTransformed, Detail: detail}
}

func degradation(capability modelinvoker.Capability, detail string) modelinvoker.MappingDecision {
	return modelinvoker.MappingDecision{Capability: capability, Action: modelinvoker.MappingDegraded, Detail: detail}
}

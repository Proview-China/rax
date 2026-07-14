package localcompat

import modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"

type Definition struct {
	Product  Product
	Provider modelinvoker.ProviderID
}

// Definitions is the closed list of caller-configured local and enterprise
// OpenAI-compatible LLM products supported by this adapter.
func Definitions() []Definition {
	return []Definition{
		{Product: ProductGeneric, Provider: ProviderGeneric},
		{Product: ProductOllama, Provider: ProviderOllama},
		{Product: ProductLlamaCPP, Provider: ProviderLlamaCPP},
	}
}

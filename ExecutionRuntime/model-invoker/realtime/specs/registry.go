package specs

import (
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/realtime/nativews"
)

type Definition struct {
	ID       string
	Provider modelinvoker.ProviderID
	build    func(string, []string) nativews.Config
}

func (definition Definition) Config(apiKey string, models []string) nativews.Config {
	if definition.build == nil {
		return nativews.Config{}
	}
	return definition.build(apiKey, models)
}

// Definitions returns every audited official realtime surface. Parameterized
// local WebSocket surfaces remain explicit host configuration and are covered
// separately by Local.
func Definitions() []Definition {
	return []Definition{
		{ID: "openai.realtime", Provider: "openai", build: OpenAI},
		{ID: "google.gemini-live", Provider: "gemini", build: Gemini},
		{ID: "xai.voice", Provider: "xai", build: XAI},
	}
}

package specs

import (
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation/nativehttp"
)

type SurfaceKind string

const (
	SurfaceOfficial   SurfaceKind = "official"
	SurfaceCompatible SurfaceKind = "compatible"
	SurfaceManagement SurfaceKind = "management"
	SurfaceLocal      SurfaceKind = "local"
)

// Definition is the canonical machine-readable list of peripheral HTTP
// surfaces. Keeping constructors here lets conformance checks cover every
// published upstream without duplicating a test-only provider list.
type Definition struct {
	ID       string
	Provider modelinvoker.ProviderID
	Kind     SurfaceKind
	build    func(map[operation.Kind][]string) []nativehttp.Spec
}

func (definition Definition) Specs(models map[operation.Kind][]string) []nativehttp.Spec {
	if definition.build == nil {
		return nil
	}
	return definition.build(models)
}

func Definitions() []Definition {
	return []Definition{
		{ID: "openai.platform", Provider: "openai", Kind: SurfaceOfficial, build: OpenAI},
		{ID: "anthropic.platform", Provider: "anthropic", Kind: SurfaceOfficial, build: Anthropic},
		{ID: "google.gemini-developer", Provider: "gemini", Kind: SurfaceOfficial, build: Gemini},
		{ID: "xai.inference", Provider: "xai", Kind: SurfaceOfficial, build: XAI},
		{ID: "xai.management", Provider: "xai-management", Kind: SurfaceManagement, build: func(map[operation.Kind][]string) []nativehttp.Spec { return XAIManagement() }},
		{ID: "kimi.platform", Provider: "kimi", Kind: SurfaceCompatible, build: Kimi},
		{ID: "zai.platform", Provider: "zai", Kind: SurfaceOfficial, build: ZAI},
		{ID: "xiaomi.mimo", Provider: "xiaomi-mimo", Kind: SurfaceCompatible, build: MiMo},
		{ID: "minimax.platform", Provider: "minimax", Kind: SurfaceOfficial, build: MiniMax},
		{ID: "alibaba.qwen.dashscope", Provider: "qwen", Kind: SurfaceOfficial, build: Qwen},
		{ID: "alibaba.qwen.openai-compatible-batch", Provider: "qwen", Kind: SurfaceCompatible, build: func(map[operation.Kind][]string) []nativehttp.Spec { return QwenOpenAICompatibleBatch() }},
		{ID: "ollama.native", Provider: "ollama-native", Kind: SurfaceLocal, build: Ollama},
		{ID: "llamacpp.native", Provider: "llamacpp-native", Kind: SurfaceLocal, build: LlamaCPP},
		{ID: "self-hosted.openai-compatible", Provider: "local-openai-compatible", Kind: SurfaceLocal, build: GenericOpenAICompatible},
	}
}

// Package specs contains audited WebSocket endpoint descriptors for provider
// realtime APIs. Callers still supply exact model allowlists and credentials;
// session-specific setup remains a provider-native JSON configuration event.
package specs

import (
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/realtime/nativews"
)

func OpenAI(apiKey string, models []string) nativews.Config {
	return nativews.Config{
		Provider: "openai", URL: "wss://api.openai.com/v1/realtime", Trust: nativews.TrustOfficial,
		OfficialHosts: []string{"api.openai.com"}, Auth: nativews.AuthBearer, APIKey: apiKey,
		ModelQueryKey: "model", AllowedModels: append([]string(nil), models...),
	}
}

func Gemini(apiKey string, models []string) nativews.Config {
	return nativews.Config{
		Provider: "gemini", URL: "wss://generativelanguage.googleapis.com/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContent",
		Trust: nativews.TrustOfficial, OfficialHosts: []string{"generativelanguage.googleapis.com"},
		Auth: nativews.AuthQuery, QueryName: "key", APIKey: apiKey,
		ConfigurationModelPath: "setup.model", ConfigurationModelPrefix: "models/",
		AllowedModels: append([]string(nil), models...),
	}
}

func XAI(apiKey string, models []string) nativews.Config {
	return nativews.Config{
		Provider: "xai", URL: "wss://api.x.ai/v1/realtime", Trust: nativews.TrustOfficial,
		OfficialHosts: []string{"api.x.ai"}, Auth: nativews.AuthBearer, APIKey: apiKey,
		ModelQueryKey: "model",
		AllowedModels: append([]string(nil), models...),
	}
}

func Local(provider modelinvoker.ProviderID, url string, models []string) nativews.Config {
	return nativews.Config{
		Provider: provider, URL: url, Trust: nativews.TrustLocal, Auth: nativews.AuthAnonymous,
		ModelQueryKey: "model", AllowedModels: append([]string(nil), models...),
	}
}

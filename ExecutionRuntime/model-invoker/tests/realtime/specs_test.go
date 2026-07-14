package realtime_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/realtime/nativews"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/realtime/specs"
)

func TestOfficialRealtimeSpecsPassPinnedValidation(t *testing.T) {
	configs := []nativews.Config{
		specs.OpenAI("key", []string{"gpt-realtime"}),
		specs.Gemini("key", []string{"gemini-live"}),
		specs.XAI("key", []string{"grok-voice"}),
	}
	for _, config := range configs {
		if _, err := nativews.New(config); err != nil {
			t.Fatalf("%s spec failed validation: %v", config.Provider, err)
		}
	}
}

func TestOfficialRealtimeSpecsBindModelAtProviderNativeLocation(t *testing.T) {
	openAI := specs.OpenAI("key", []string{"gpt-realtime"})
	if openAI.ModelQueryKey != "model" || openAI.ConfigurationModelPath != "" {
		t.Fatalf("unexpected OpenAI model binding: %+v", openAI)
	}
	gemini := specs.Gemini("key", []string{"gemini-live"})
	if gemini.ModelQueryKey != "" || gemini.ConfigurationModelPath != "setup.model" || gemini.ConfigurationModelPrefix != "models/" {
		t.Fatalf("unexpected Gemini model binding: %+v", gemini)
	}
	xai := specs.XAI("key", []string{"grok-voice"})
	if xai.ModelQueryKey != "model" || xai.ConfigurationModelPath != "" {
		t.Fatalf("unexpected xAI model binding: %+v", xai)
	}
}

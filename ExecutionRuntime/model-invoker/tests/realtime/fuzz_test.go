package realtime_test

import (
	"fmt"
	"strings"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/realtime/nativews"
)

func FuzzRealtimeConfigurationNeverPanicsOrLeaksCredential(f *testing.F) {
	f.Add("provider", "model", "secret", "model", "")
	f.Add("", "bad\nmodel", "bad\nsecret", "bad key", "setup.model")
	f.Fuzz(func(t *testing.T, provider, model, key, queryKey, configPath string) {
		credential := "fuzz-realtime-secret:" + key
		config := nativews.Config{
			Provider:      modelinvoker.ProviderID(provider),
			URL:           "ws://127.0.0.1:1/live",
			Trust:         nativews.TrustLocal,
			Auth:          nativews.AuthQuery,
			APIKey:        credential,
			QueryName:     "key",
			ModelQueryKey: queryKey,
			AllowedModels: []string{model},
		}
		if configPath != "" {
			config.ModelQueryKey = ""
			config.ConfigurationModelPath = configPath
		}
		_, err := nativews.New(config)
		if strings.Contains(fmt.Sprint(err), credential) || strings.Contains(fmt.Sprintf("%#v", config), credential) {
			t.Fatal("credential leaked through realtime validation")
		}
	})
}

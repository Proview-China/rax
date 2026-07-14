package localcompat_test

import (
	"fmt"
	"strings"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/localcompat"
)

func FuzzLocalCompatibleConfigurationNeverPanicsOrLeaksCredential(f *testing.F) {
	f.Add("openai-compatible", "m", "", "agent")
	f.Add("bad", "bad\nmodel", "secret", "bad\nagent")
	f.Fuzz(func(t *testing.T, product, model, key, agent string) {
		credential := "fuzz-local-secret:" + key
		config := localcompat.Config{
			Product: localcompat.Product(product), Trust: localcompat.TrustLocal, BaseURL: "http://127.0.0.1:1/v1",
			Protocol: modelinvoker.ProtocolChatCompletions, APIKey: credential, AllowedModels: []string{model},
			SupportedCapabilities: []modelinvoker.Capability{modelinvoker.CapabilityTextGeneration}, UserAgent: agent,
		}
		_, err := localcompat.New(config)
		if strings.Contains(fmt.Sprint(err), credential) || strings.Contains(fmt.Sprintf("%#v", config), credential) {
			t.Fatal("credential leaked through local compatible validation")
		}
	})
}

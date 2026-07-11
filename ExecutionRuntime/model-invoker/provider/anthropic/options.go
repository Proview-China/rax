package anthropic

import (
	"encoding/json"
	"fmt"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

func validateProviderOptions(request modelinvoker.Request) error {
	for namespace, raw := range request.ProviderOptions {
		if namespace != ProviderID {
			return mappingError("options", fmt.Sprintf("provider options namespace %q cannot be consumed by Anthropic", namespace))
		}
		var options map[string]json.RawMessage
		if err := json.Unmarshal(raw, &options); err != nil || options == nil {
			return mappingError("options", "Anthropic provider options must be a JSON object")
		}
		if len(options) != 0 {
			return mappingError("options", "Anthropic provider options are not defined for the Messages slice")
		}
	}
	return nil
}

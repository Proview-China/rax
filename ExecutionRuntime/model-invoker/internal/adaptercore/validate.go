package adaptercore

import (
	"fmt"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

// ValidateSelection checks invariants shared by every concrete adapter after
// Request.Validate has checked the public structure.
func ValidateSelection(
	request modelinvoker.Request,
	provider modelinvoker.ProviderID,
	configuredEndpoint string,
	protocols ...modelinvoker.Protocol,
) error {
	if request.Provider != provider {
		return fmt.Errorf("request provider %q does not match %q", request.Provider, provider)
	}
	protocolSupported := false
	for _, protocol := range protocols {
		protocolSupported = protocolSupported || request.Protocol == protocol
	}
	if !protocolSupported {
		return fmt.Errorf("unsupported protocol %q", request.Protocol)
	}
	if request.Endpoint != "" && NormalizeEndpoint(request.Endpoint) != NormalizeEndpoint(configuredEndpoint) {
		return fmt.Errorf("request endpoint does not match the configured provider endpoint")
	}
	for namespace := range request.ProviderOptions {
		if namespace != provider {
			return fmt.Errorf("provider options namespace %q does not match %q", namespace, provider)
		}
	}
	return nil
}

package catalog_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
)

func TestXAIRouteKeepsAPIAndConsumerOfferingSeparate(t *testing.T) {
	document := catalog.DefaultDocument()
	api, consumer := 0, 0
	for _, entry := range document.Entries {
		switch entry.Route.Offering.ID {
		case "xai.api.payg":
			api++
			if entry.ID != "xai.api.global.payg.responses" || entry.Route.Provider != "xai.api" ||
				entry.Route.Model.ProviderModelRef != "grok-4.5" || entry.Route.Credential.References[0].Name != "XAI_API_KEY" ||
				!entry.Implementation.Callable || entry.Implementation.AdapterID != "xai" || entry.Implementation.Status != catalog.ImplementationImplementedOffline {
				t.Errorf("xAI API route=%#v", entry)
			}
		case "xai.consumer-subscription":
			consumer++
			if entry.Implementation.Callable || entry.Implementation.AdapterID != "" {
				t.Errorf("xAI consumer record became callable: %#v", entry.Implementation)
			}
		}
	}
	if api != 1 || consumer != 1 {
		t.Fatalf("xAI route counts: api=%d consumer=%d", api, consumer)
	}
}

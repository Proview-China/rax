package catalog_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

func TestQwenRoutesBindRegionWorkspaceCredentialAndProtocol(t *testing.T) {
	document := catalog.DefaultDocument()
	want := map[upstream.RouteID]struct{}{
		"alibaba.model-studio.cn-beijing.payg.responses":            {},
		"alibaba.model-studio.cn-beijing.payg.chat_completions":     {},
		"alibaba.model-studio.ap-southeast-1.payg.responses":        {},
		"alibaba.model-studio.ap-southeast-1.payg.chat_completions": {},
	}
	for _, entry := range document.Entries {
		if entry.Route.Provider != "alibaba.model-studio" || entry.Route.Offering.ID != "alibaba.model-studio.payg" {
			continue
		}
		delete(want, entry.ID)
		if !entry.Implementation.Callable || entry.Implementation.AdapterID != "qwen" || entry.Implementation.Status != catalog.ImplementationImplementedOffline {
			t.Errorf("route %q implementation=%#v", entry.ID, entry.Implementation)
		}
		if entry.Route.Deployment.WorkspaceRef != "runtime-workspace" || entry.Route.Deployment.Region == "" {
			t.Errorf("route %q deployment=%#v", entry.ID, entry.Route.Deployment)
		}
		if entry.Route.Credential.References[0].Name != "DASHSCOPE_API_KEY" || len(entry.Route.Credential.DeniedKeyPrefixes) != 1 || entry.Route.Credential.DeniedKeyPrefixes[0] != "sk-sp-" {
			t.Errorf("route %q credential boundary=%#v", entry.ID, entry.Route.Credential)
		}
		if entry.Route.Offering.Entitlement.AllowsAutomaticPAYGSwitch {
			t.Errorf("route %q allows automatic billing switch", entry.ID)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing Qwen routes: %#v", want)
	}
}

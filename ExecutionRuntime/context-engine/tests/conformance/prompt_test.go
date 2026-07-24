package conformance_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/promptstore"
)

func TestPromptAssetDoesNotOwnProviderOrFrameLayoutV1(t *testing.T) {
	asset := testkit.PromptAssetV1()
	payload, err := json.Marshal(asset)
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(payload))
	for _, forbidden := range []string{"provider_message", "provider_role", "system_message", "frame_region", "cache_placement", "runtime_settlement", "continuation"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("cross-owner field %q escaped into PromptAsset: %s", forbidden, payload)
		}
	}
}

func TestPromptRoleMappingAndProductionNoGoV1(t *testing.T) {
	want := map[contract.PromptFragmentRoleV1]struct {
		kind  contract.FragmentKind
		trust contract.TrustClass
	}{
		contract.PromptFragmentInstructionV1: {contract.FragmentInstruction, contract.TrustAuthoritativeInstruction},
		contract.PromptFragmentExampleV1:     {contract.FragmentConversation, contract.TrustRestrictedMaterial},
		contract.PromptFragmentPolicyV1:      {contract.FragmentPolicySnapshot, contract.TrustRestrictedMaterial},
	}
	for role, expected := range want {
		kind, trust, err := role.KindAndTrustV1()
		if err != nil || kind != expected.kind || trust != expected.trust {
			t.Fatalf("role=%q kind=%q trust=%q err=%v", role, kind, trust, err)
		}
	}
	service, _ := kernel.NewContextPromptServiceV1(promptstore.NewMemory())
	for _, action := range []contract.ContextPromptProductionActionV1{contract.ContextPromptPublishV1, contract.ContextPromptRollbackV1, contract.ContextPromptRevokeV1} {
		if err := service.ProductionAction(context.Background(), action); !errors.Is(err, contract.ErrUnsupported) {
			t.Fatalf("production prompt action %q escaped CTX-D07: %v", action, err)
		}
	}
}

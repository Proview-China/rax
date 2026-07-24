package failure_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/promptstore"
)

type promptInspectFaultStoreV1 struct {
	contextports.ContextPromptLifecycleStoreV1
	err error
}

func (s *promptInspectFaultStoreV1) InspectPromptAssetV1(context.Context, contract.PromptAssetRefV1) (contract.PromptAssetV1, error) {
	return contract.PromptAssetV1{}, s.err
}

func TestPromptBuildInspectFaultsReturnZeroV1(t *testing.T) {
	asset := testkit.PromptAssetV1()
	for name, injected := range map[string]error{"unknown": contract.ErrUnknown, "unavailable": contract.ErrUnavailable} {
		t.Run(name, func(t *testing.T) {
			base := promptstore.NewMemory()
			if _, err := base.PutPromptAssetV1(context.Background(), asset); err != nil {
				t.Fatal(err)
			}
			service, err := kernel.NewContextPromptServiceV1(&promptInspectFaultStoreV1{ContextPromptLifecycleStoreV1: base, err: injected})
			if err != nil {
				t.Fatal(err)
			}
			got, err := service.BuildCandidates(context.Background(), testkit.PromptBuildRequestV1(asset))
			if !errors.Is(err, injected) || !reflect.DeepEqual(got, contract.PromptCandidateSetV1{}) {
				t.Fatalf("injected=%v got=%#v err=%v", injected, got, err)
			}
		})
	}
}

func TestPromptCanceledBuildReturnsZeroV1(t *testing.T) {
	store := promptstore.NewMemory()
	asset := testkit.PromptAssetV1()
	if _, err := store.PutPromptAssetV1(context.Background(), asset); err != nil {
		t.Fatal(err)
	}
	service, _ := kernel.NewContextPromptServiceV1(store)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := service.BuildCandidates(ctx, testkit.PromptBuildRequestV1(asset))
	if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, contract.PromptCandidateSetV1{}) {
		t.Fatalf("canceled build produced state: %#v %v", got, err)
	}
}

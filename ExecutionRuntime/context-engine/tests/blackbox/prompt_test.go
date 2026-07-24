package blackbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/promptstore"
)

func TestPromptOwnerLocalBlackBoxV1(t *testing.T) {
	service, err := kernel.NewContextPromptServiceV1(promptstore.NewMemory())
	if err != nil {
		t.Fatal(err)
	}
	asset := testkit.PromptAssetV1()
	assetRef := testkit.PromptAssetRefV1(asset)
	draft := promptBlackBoxLifecycleV1("prompt-blackbox-draft", assetRef, nil, contract.ContextPromptDraftV1)
	draftRef, err := service.CreateDraft(context.Background(), asset, draft)
	if err != nil {
		t.Fatal(err)
	}
	set, err := service.BuildCandidates(context.Background(), testkit.PromptBuildRequestV1(asset))
	if err != nil {
		t.Fatal(err)
	}
	if set.PromptAssetRef != assetRef || len(set.Candidates) != len(asset.Fragments) {
		t.Fatalf("prompt projection drift: %#v", set)
	}
	validated := promptBlackBoxLifecycleV1("prompt-blackbox-validated", assetRef, &draftRef, contract.ContextPromptValidatedV1)
	validated.ValidationReportRef = promptBlackBoxRefV1("prompt-blackbox-validation")
	validatedRef, err := service.Advance(context.Background(), draftRef, validated)
	if err != nil {
		t.Fatal(err)
	}
	head, err := service.InspectHead(context.Background(), assetRef)
	if err != nil || head.LifecycleRef != validatedRef {
		t.Fatalf("prompt head drift: %#v %v", head, err)
	}
	inspected, err := service.InspectAsset(context.Background(), assetRef)
	if err != nil || inspected.ContentDigest != asset.ContentDigest {
		t.Fatalf("prompt exact inspect drift: %#v %v", inspected, err)
	}
}

func promptBlackBoxLifecycleV1(id string, asset contract.PromptAssetRefV1, previous *contract.FactRef, state contract.ContextPromptLifecycleStateV1) contract.ContextPromptLifecycleFactV1 {
	return contract.ContextPromptLifecycleFactV1{
		ContractVersion: contract.Version, ID: id, Revision: 1, PromptAssetRef: asset,
		PreviousLifecycleRef: previous, State: state, Evidence: []contract.EvidenceRef{},
		CreatedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + int64(time.Minute),
	}
}

func promptBlackBoxRefV1(id string) *contract.FactRef {
	ref := contract.FactRef{ID: id, Revision: 1, Digest: testkit.D(id)}
	return &ref
}

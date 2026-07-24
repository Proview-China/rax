package kernel_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/promptstore"
)

func TestPromptAssetBuildCandidatesExactDeterministicV1(t *testing.T) {
	service, _ := kernel.NewContextPromptServiceV1(promptstore.NewMemory())
	asset := testkit.PromptAssetV1()
	assetRef := testkit.PromptAssetRefV1(asset)
	draft := promptKernelLifecycleV1("prompt-draft-build", assetRef, nil, contract.ContextPromptDraftV1)
	if _, err := service.CreateDraft(context.Background(), asset, draft); err != nil {
		t.Fatal(err)
	}
	request := testkit.PromptBuildRequestV1(asset)
	first, err := service.BuildCandidates(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.BuildCandidates(context.Background(), request)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("prompt candidate projection drift: %#v %#v %v", first, second, err)
	}
	if len(first.Candidates) != 3 || first.ExpiresUnixNano != request.NotAfterUnixNano {
		t.Fatalf("candidate set drift: %#v", first)
	}
	wantKinds := []contract.FragmentKind{contract.FragmentInstruction, contract.FragmentConversation, contract.FragmentPolicySnapshot}
	wantTrust := []contract.TrustClass{contract.TrustAuthoritativeInstruction, contract.TrustRestrictedMaterial, contract.TrustRestrictedMaterial}
	for index, candidate := range first.Candidates {
		if candidate.Kind != wantKinds[index] || candidate.Trust != wantTrust[index] || candidate.Owner != asset.Owner || candidate.Execution != request.Execution || candidate.Sensitivity != asset.Sensitivity || candidate.Mode != contract.MaterializationInline || candidate.SourceRef != asset.ID || candidate.SourceRevision != asset.Revision || candidate.Content != asset.Fragments[index].Content || candidate.Evidence != asset.Fragments[index].Evidence {
			t.Fatalf("candidate %d mapping drift: %#v", index, candidate)
		}
	}
	mutated := first
	mutated.Candidates[0].ID = "caller-mutated"
	again, err := service.BuildCandidates(context.Background(), request)
	if err != nil || again.Candidates[0].ID == "caller-mutated" {
		t.Fatalf("candidate projection alias escaped: %#v %v", again, err)
	}
	inspected, err := service.InspectAsset(context.Background(), assetRef)
	if err != nil {
		t.Fatal(err)
	}
	inspected.Fragments[0].ID = "caller-mutated"
	againAsset, err := service.InspectAsset(context.Background(), assetRef)
	if err != nil || againAsset.Fragments[0].ID == "caller-mutated" {
		t.Fatalf("asset alias escaped: %#v %v", againAsset, err)
	}
}

func TestPromptAssetBuildCandidatesFailClosedV1(t *testing.T) {
	service, _ := kernel.NewContextPromptServiceV1(promptstore.NewMemory())
	asset := testkit.PromptAssetV1()
	assetRef := testkit.PromptAssetRefV1(asset)
	draft := promptKernelLifecycleV1("prompt-draft-fail", assetRef, nil, contract.ContextPromptDraftV1)
	if _, err := service.CreateDraft(context.Background(), asset, draft); err != nil {
		t.Fatal(err)
	}
	base := testkit.PromptBuildRequestV1(asset)
	tests := map[string]struct {
		mutate func(*contract.BuildPromptCandidatesRequestV1)
		want   error
	}{
		"authority": {func(v *contract.BuildPromptCandidatesRequestV1) {
			v.Execution.AuthorityDigest = testkit.D("other-authority")
		}, contract.ErrUnauthorized},
		"render": {func(v *contract.BuildPromptCandidatesRequestV1) {
			v.RenderCompatibilityRef = testkit.PromptRenderRefV1("other-render")
		}, contract.ErrConflict},
		"before_asset": {func(v *contract.BuildPromptCandidatesRequestV1) { v.CreatedUnixNano = asset.CreatedUnixNano - 1 }, contract.ErrExpired},
		"asset_expiry": {func(v *contract.BuildPromptCandidatesRequestV1) {
			v.CreatedUnixNano = asset.ExpiresUnixNano
			v.NotAfterUnixNano = asset.ExpiresUnixNano + int64(time.Second)
		}, contract.ErrExpired},
		"asset_ref": {func(v *contract.BuildPromptCandidatesRequestV1) { v.PromptAssetRef.Digest = testkit.D("other-asset") }, contract.ErrConflict},
	}
	for name, item := range tests {
		t.Run(name, func(t *testing.T) {
			request := base
			item.mutate(&request)
			request.RequestDigest = ""
			sealed, err := contract.SealBuildPromptCandidatesRequestV1(request)
			if err != nil {
				t.Fatal(err)
			}
			result, err := service.BuildCandidates(context.Background(), sealed)
			if !errors.Is(err, item.want) || !reflect.DeepEqual(result, contract.PromptCandidateSetV1{}) {
				t.Fatalf("want %v with zero result, got %#v %v", item.want, result, err)
			}
		})
	}
	tampered := base
	tampered.CreatedUnixNano++
	if result, err := service.BuildCandidates(context.Background(), tampered); !errors.Is(err, contract.ErrConflict) || !reflect.DeepEqual(result, contract.PromptCandidateSetV1{}) {
		t.Fatalf("request digest drift accepted: %#v %v", result, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result, err := service.BuildCandidates(ctx, base); !errors.Is(err, context.Canceled) || !reflect.DeepEqual(result, contract.PromptCandidateSetV1{}) {
		t.Fatalf("cancel did not return zero: %#v %v", result, err)
	}
}

func TestPromptPreReleaseLifecycleLostReplyAndProductionNoGoV1(t *testing.T) {
	service, _ := kernel.NewContextPromptServiceV1(promptstore.NewMemory())
	asset := testkit.PromptAssetV1()
	assetRef := testkit.PromptAssetRefV1(asset)
	draft := promptKernelLifecycleV1("prompt-draft-life", assetRef, nil, contract.ContextPromptDraftV1)
	draftRef, err := service.CreateDraft(context.Background(), asset, draft)
	if err != nil {
		t.Fatal(err)
	}
	validated := promptKernelLifecycleV1("prompt-validated", assetRef, &draftRef, contract.ContextPromptValidatedV1)
	validated.ValidationReportRef = promptKernelRefPtrV1("prompt-validation")
	validatedRef, err := service.Advance(context.Background(), draftRef, validated)
	if err != nil {
		t.Fatal(err)
	}
	evaluated := promptKernelLifecycleV1("prompt-evaluated", assetRef, &validatedRef, contract.ContextPromptEvaluatedV1)
	evaluated.ValidationReportRef = validated.ValidationReportRef
	evaluated.EvaluationRef = promptKernelRefPtrV1("prompt-evaluation")
	evaluated.FeedbackCandidateRef = promptKernelRefPtrV1("prompt-feedback")
	evaluatedRef, err := service.Advance(context.Background(), validatedRef, evaluated)
	if err != nil {
		t.Fatal(err)
	}
	review := promptKernelLifecycleV1("prompt-review", assetRef, &evaluatedRef, contract.ContextPromptReviewPendingV1)
	review.ValidationReportRef = evaluated.ValidationReportRef
	review.EvaluationRef = evaluated.EvaluationRef
	review.FeedbackCandidateRef = evaluated.FeedbackCandidateRef
	review.ReviewCaseRef = promptKernelRefPtrV1("prompt-review-case")
	reviewRef, err := service.Advance(context.Background(), evaluatedRef, review)
	if err != nil {
		t.Fatal(err)
	}
	head, err := service.InspectHead(context.Background(), assetRef)
	if err != nil || head.LifecycleRef != reviewRef {
		t.Fatalf("lost reply head inspect drift: %#v %v", head, err)
	}
	if _, err := service.Advance(context.Background(), evaluatedRef, review); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("replayed prompt successor did not conflict: %v", err)
	}
	for _, action := range []contract.ContextPromptProductionActionV1{contract.ContextPromptPublishV1, contract.ContextPromptRollbackV1, contract.ContextPromptRevokeV1} {
		if err := service.ProductionAction(context.Background(), action); !errors.Is(err, contract.ErrUnsupported) {
			t.Fatalf("production action %q accepted: %v", action, err)
		}
	}
}

func TestPromptLifecycle64ConcurrentSingleSuccessorV1(t *testing.T) {
	service, _ := kernel.NewContextPromptServiceV1(promptstore.NewMemory())
	asset := testkit.PromptAssetV1()
	assetRef := testkit.PromptAssetRefV1(asset)
	draft := promptKernelLifecycleV1("prompt-draft-concurrent", assetRef, nil, contract.ContextPromptDraftV1)
	draftRef, err := service.CreateDraft(context.Background(), asset, draft)
	if err != nil {
		t.Fatal(err)
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for index := range 64 {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			next := promptKernelLifecycleV1(fmt.Sprintf("prompt-validated-%02d", index), assetRef, &draftRef, contract.ContextPromptValidatedV1)
			next.ValidationReportRef = promptKernelRefPtrV1("prompt-validation")
			if _, err := service.Advance(context.Background(), draftRef, next); err == nil {
				successes.Add(1)
			} else if !errors.Is(err, contract.ErrConflict) {
				t.Errorf("unexpected CAS error: %v", err)
			}
		}(index)
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("successes=%d want=1", successes.Load())
	}
}

func TestPromptCancelAndImmutableIdentityV1(t *testing.T) {
	store := promptstore.NewMemory()
	service, _ := kernel.NewContextPromptServiceV1(store)
	asset := testkit.PromptAssetV1()
	assetRef := testkit.PromptAssetRefV1(asset)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.CreateDraft(ctx, asset, promptKernelLifecycleV1("prompt-cancel", assetRef, nil, contract.ContextPromptDraftV1)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel not preserved: %v", err)
	}
	if _, err := service.InspectHead(context.Background(), assetRef); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("canceled create produced a head: %v", err)
	}
	if _, err := store.PutPromptAssetV1(context.Background(), asset); err != nil {
		t.Fatal(err)
	}
	changed := asset
	changed.Fragments = append([]contract.PromptFragmentSpecV1(nil), asset.Fragments...)
	changed.Fragments[0].TokenEstimate++
	changed, err := contract.SealPromptAssetV1(changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutPromptAssetV1(context.Background(), changed); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("same-ID changed prompt accepted: %v", err)
	}
}

func TestPromptCandidateProjection64ConcurrentDeterministicV1(t *testing.T) {
	service, _ := kernel.NewContextPromptServiceV1(promptstore.NewMemory())
	asset := testkit.PromptAssetV1()
	assetRef := testkit.PromptAssetRefV1(asset)
	if _, err := service.CreateDraft(context.Background(), asset, promptKernelLifecycleV1("prompt-draft-projection-concurrent", assetRef, nil, contract.ContextPromptDraftV1)); err != nil {
		t.Fatal(err)
	}
	request := testkit.PromptBuildRequestV1(asset)
	want, err := service.BuildCandidates(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 64)
	for range 64 {
		go func() {
			got, err := service.BuildCandidates(context.Background(), request)
			if err == nil && !reflect.DeepEqual(got, want) {
				err = errors.New("prompt projection drift")
			}
			errs <- err
		}()
	}
	for range 64 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
}

func promptKernelLifecycleV1(id string, asset contract.PromptAssetRefV1, previous *contract.FactRef, state contract.ContextPromptLifecycleStateV1) contract.ContextPromptLifecycleFactV1 {
	return contract.ContextPromptLifecycleFactV1{ContractVersion: contract.Version, ID: id, Revision: 1, PromptAssetRef: asset, PreviousLifecycleRef: previous, State: state, Evidence: []contract.EvidenceRef{}, CreatedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + int64(time.Minute)}
}

func promptKernelRefPtrV1(id string) *contract.FactRef {
	ref := contract.FactRef{ID: id, Revision: 1, Digest: testkit.D(id)}
	return &ref
}

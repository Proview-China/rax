package kernel_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/releasestore"
)

func TestRecipePreReleaseLifecycleAndLostReplyInspectV1(t *testing.T) {
	service, _ := kernel.NewContextRecipeLifecycleServiceV1(releasestore.NewMemory())
	recipe := testkit.Recipe()
	recipeDigest, _ := recipe.DigestValue()
	recipeRef := contract.FactRef{ID: recipe.ID, Revision: recipe.Revision, Digest: recipeDigest}
	draft := releaseKernelFactV1("draft", recipeRef, nil, contract.ContextRecipeDraftV1)
	draftRef, err := service.CreateDraft(context.Background(), recipe, draft)
	if err != nil {
		t.Fatal(err)
	}
	validated := releaseKernelFactV1("validated", recipeRef, &draftRef, contract.ContextRecipeValidatedV1)
	validated.ValidationReportRef = releaseKernelRefPtrV1("validation")
	validatedRef, err := service.Advance(context.Background(), draftRef, validated)
	if err != nil {
		t.Fatal(err)
	}
	evaluated := releaseKernelFactV1("evaluated", recipeRef, &validatedRef, contract.ContextRecipeEvaluatedV1)
	evaluated.ValidationReportRef = validated.ValidationReportRef
	evaluated.EvaluationRef = releaseKernelRefPtrV1("evaluation")
	evaluatedRef, err := service.Advance(context.Background(), validatedRef, evaluated)
	if err != nil {
		t.Fatal(err)
	}
	review := releaseKernelFactV1("review-pending", recipeRef, &evaluatedRef, contract.ContextRecipeReviewPendingV1)
	review.ValidationReportRef = validated.ValidationReportRef
	review.EvaluationRef = evaluated.EvaluationRef
	review.ReviewCaseRef = releaseKernelRefPtrV1("review-case")
	reviewRef, err := service.Advance(context.Background(), evaluatedRef, review)
	if err != nil {
		t.Fatal(err)
	}
	head, err := service.InspectHead(context.Background(), recipeRef)
	if err != nil || head.LifecycleRef != reviewRef {
		t.Fatalf("lost reply head inspect: %#v %v", head, err)
	}
	if _, err := service.Advance(context.Background(), evaluatedRef, review); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("replayed write did not conflict/inspect: %v", err)
	}
	for _, action := range []contract.ContextRecipeProductionActionV1{contract.ContextRecipePublishV1, contract.ContextRecipeRollbackV1, contract.ContextRecipeRevokeV1} {
		if err := service.ProductionAction(context.Background(), action); !errors.Is(err, contract.ErrUnsupported) {
			t.Fatalf("production action %q accepted: %v", action, err)
		}
	}
}

func TestRecipeLifecycle64ConcurrentSuccessorsSingleWinnerV1(t *testing.T) {
	service, _ := kernel.NewContextRecipeLifecycleServiceV1(releasestore.NewMemory())
	recipe := testkit.Recipe()
	digest, _ := recipe.DigestValue()
	recipeRef := contract.FactRef{ID: recipe.ID, Revision: recipe.Revision, Digest: digest}
	draft := releaseKernelFactV1("draft-concurrent", recipeRef, nil, contract.ContextRecipeDraftV1)
	draftRef, err := service.CreateDraft(context.Background(), recipe, draft)
	if err != nil {
		t.Fatal(err)
	}
	var success atomic.Int32
	var wg sync.WaitGroup
	for i := range 64 {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			next := releaseKernelFactV1(fmt.Sprintf("validated-%02d", index), recipeRef, &draftRef, contract.ContextRecipeValidatedV1)
			next.ValidationReportRef = releaseKernelRefPtrV1("validation")
			if _, err := service.Advance(context.Background(), draftRef, next); err == nil {
				success.Add(1)
			} else if !errors.Is(err, contract.ErrConflict) {
				t.Errorf("unexpected CAS error: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if success.Load() != 1 {
		t.Fatalf("successes=%d want=1", success.Load())
	}
}

func TestRecipeLifecycleCancelCreatesNoHeadV1(t *testing.T) {
	service, _ := kernel.NewContextRecipeLifecycleServiceV1(releasestore.NewMemory())
	recipe := testkit.Recipe()
	digest, _ := recipe.DigestValue()
	recipeRef := contract.FactRef{ID: recipe.ID, Revision: recipe.Revision, Digest: digest}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.CreateDraft(ctx, recipe, releaseKernelFactV1("draft-cancel", recipeRef, nil, contract.ContextRecipeDraftV1)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel not preserved: %v", err)
	}
	if _, err := service.InspectHead(context.Background(), recipeRef); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("canceled draft created head: %v", err)
	}
}

func releaseKernelFactV1(id string, recipe contract.FactRef, previous *contract.FactRef, state contract.ContextRecipeLifecycleStateV1) contract.ContextRecipeLifecycleFactV1 {
	return contract.ContextRecipeLifecycleFactV1{ContractVersion: contract.Version, ID: id, Revision: 1, RecipeRef: recipe, PreviousLifecycleRef: previous, State: state, Evidence: []contract.EvidenceRef{}, CreatedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + int64(time.Minute)}
}
func releaseKernelRefPtrV1(id string) *contract.FactRef {
	value := outcomeKernelRefV1(id)
	return &value
}

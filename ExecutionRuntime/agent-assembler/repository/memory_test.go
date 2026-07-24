package repository_test

import (
	"context"
	"sync"
	"testing"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/repository"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestPlanRepositoryConcurrentCreateOnce(t *testing.T) {
	fixture := testkit.NewFixture()
	result, err := fixture.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	store := repository.NewMemory()
	const workers = 64
	var wait sync.WaitGroup
	errors := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			stored, err := store.EnsureExactResolvedAgentPlanV1(context.Background(), result.Plan)
			if err == nil && stored.RefV1() != result.Plan.RefV1() {
				t.Error("wrong exact plan")
			}
			errors <- err
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestPlanRepositoryConcurrentDifferentContentHasOneExactWinner(t *testing.T) {
	fixture := testkit.NewFixture()
	result, err := fixture.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	first := result.Plan
	second := first
	second.EvidenceRefs = append(second.EvidenceRefs, testkit.Ref("evidence/conflicting-plan"))
	second, err = assemblercontract.SealResolvedAgentPlanV1(second)
	if err != nil {
		t.Fatal(err)
	}
	store := repository.NewMemory()
	type outcome struct {
		digest core.Digest
		err    error
	}
	const workers = 64
	outcomes := make(chan outcome, workers)
	var wait sync.WaitGroup
	for i := 0; i < workers; i++ {
		candidate := first
		if i%2 == 1 {
			candidate = second
		}
		wait.Add(1)
		go func() {
			defer wait.Done()
			stored, createErr := store.EnsureExactResolvedAgentPlanV1(context.Background(), candidate)
			outcomes <- outcome{digest: stored.Digest, err: createErr}
		}()
	}
	wait.Wait()
	close(outcomes)
	winners := map[core.Digest]struct{}{}
	conflicts := 0
	for result := range outcomes {
		if result.err == nil {
			winners[result.digest] = struct{}{}
			continue
		}
		if !core.HasCategory(result.err, core.ErrorConflict) {
			t.Fatalf("unexpected create-once error: %v", result.err)
		}
		conflicts++
	}
	if len(winners) != 1 || conflicts == 0 {
		t.Fatalf("create-once did not select one exact winner: winners=%d conflicts=%d", len(winners), conflicts)
	}
}

func TestCurrentProjectionRejectsABAAndStaleExpected(t *testing.T) {
	fixture := testkit.NewFixture()
	result, err := fixture.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	planA := result.Plan
	planB := planA
	planB.PlanID = "resolved/plan-b"
	planB.BindingPlan.ID = planB.PlanID + "-binding"
	planB.BindingPlan, err = runtimeports.SealBindingPlanV2(planB.BindingPlan)
	if err != nil {
		t.Fatal(err)
	}
	planB, err = assemblercontract.SealResolvedAgentPlanV1(planB)
	if err != nil {
		t.Fatal(err)
	}
	store := repository.NewMemory()
	for _, plan := range []assemblercontract.ResolvedAgentPlanV1{planA, planB} {
		if _, err = store.EnsureExactResolvedAgentPlanV1(context.Background(), plan); err != nil {
			t.Fatal(err)
		}
	}
	checked := testkit.Now.UnixNano()
	currentA := mustSealCurrent(t, assemblercontract.CurrentResolvedPlanV1{
		DefinitionID: fixture.Definition.DefinitionID, Revision: 1, PlanRef: planA.RefV1(),
		UpdatedUnixNano: checked, CheckedUnixNano: checked, ExpiresUnixNano: planA.ValidUntilUnixNano,
	})
	storedA, err := store.CompareAndSwapCurrentResolvedAgentPlanV1(context.Background(), nil, currentA)
	if err != nil {
		t.Fatal(err)
	}
	refA := storedA.RefV1()
	currentB := mustSealCurrent(t, assemblercontract.CurrentResolvedPlanV1{
		DefinitionID: fixture.Definition.DefinitionID, Revision: 2, PlanRef: planB.RefV1(), PreviousRef: &refA,
		UpdatedUnixNano: checked + 1, CheckedUnixNano: checked + 1, ExpiresUnixNano: planB.ValidUntilUnixNano,
	})
	storedB, err := store.CompareAndSwapCurrentResolvedAgentPlanV1(context.Background(), &refA, currentB)
	if err != nil {
		t.Fatal(err)
	}
	refB := storedB.RefV1()
	rollbackA := mustSealCurrent(t, assemblercontract.CurrentResolvedPlanV1{
		DefinitionID: fixture.Definition.DefinitionID, Revision: 3, PlanRef: planA.RefV1(), PreviousRef: &refB,
		UpdatedUnixNano: checked + 2, CheckedUnixNano: checked + 2, ExpiresUnixNano: planA.ValidUntilUnixNano,
	})
	if _, err = store.CompareAndSwapCurrentResolvedAgentPlanV1(context.Background(), &refB, rollbackA); !core.HasReason(err, core.ReasonRevisionConflict) {
		t.Fatalf("ABA rollback accepted: %v", err)
	}
	if _, err = store.CompareAndSwapCurrentResolvedAgentPlanV1(context.Background(), &refA, rollbackA); !core.HasReason(err, core.ReasonRevisionConflict) {
		t.Fatalf("stale expected projection accepted: %v", err)
	}
}

func mustSealCurrent(t *testing.T, value assemblercontract.CurrentResolvedPlanV1) assemblercontract.CurrentResolvedPlanV1 {
	t.Helper()
	sealed, err := assemblercontract.SealCurrentResolvedPlanV1(value)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

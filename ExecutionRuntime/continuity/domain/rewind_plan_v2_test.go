package domain_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

func TestRewindPlanControllerV2HistoryCASAndNoABA(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_752_577_200, 0)
	backend := memory.NewWithClock(func() time.Time { return now })
	controller, err := domain.NewRewindPlanControllerV2(backend, restorePlanClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	plan := testkit.RewindPlanV2(now)
	created, replay, err := controller.CreateRewindPlanV2(ctx, ports.CreateRewindPlanRequestV2{Candidate: plan, ExpectAbsent: true})
	if err != nil || replay {
		t.Fatalf("create = (%v,%v)", replay, err)
	}
	now = now.Add(time.Second)
	next, replay, err := controller.CompareAndSwapRewindPlanV2(ctx, ports.CompareAndSwapRewindPlanRequestV2{Expected: created.Ref(), NextState: contract.RewindPlanWorkspaceInspectedV2})
	if err != nil || replay || next.Revision != 2 {
		t.Fatalf("CAS = (%d,%v,%v)", next.Revision, replay, err)
	}
	recovered, replay, err := controller.CompareAndSwapRewindPlanV2(ctx, ports.CompareAndSwapRewindPlanRequestV2{Expected: created.Ref(), NextState: contract.RewindPlanWorkspaceInspectedV2})
	if err != nil || !replay || recovered.Ref() != next.Ref() {
		t.Fatalf("replay = (%v,%v,%v)", recovered.Ref(), replay, err)
	}
	if _, _, err := controller.CompareAndSwapRewindPlanV2(ctx, ports.CompareAndSwapRewindPlanRequestV2{Expected: created.Ref(), NextState: contract.RewindPlanRejectedV2}); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("ABA stale CAS = %v", err)
	}
	historical, err := controller.InspectRewindPlanV2(ctx, ports.InspectRewindPlanRequestV2{Ref: created.Ref()})
	if err != nil || historical.State != contract.RewindPlanDraftV2 {
		t.Fatalf("history = (%s,%v)", historical.State, err)
	}
}

func TestRewindPlanControllerV2ConcurrentCASSingleWinner(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_752_577_200, 0)
	backend := memory.NewWithClock(func() time.Time { return now })
	controller, _ := domain.NewRewindPlanControllerV2(backend, restorePlanClock(func() time.Time { return now }))
	created, _, err := controller.CreateRewindPlanV2(ctx, ports.CreateRewindPlanRequestV2{Candidate: testkit.RewindPlanV2(now), ExpectAbsent: true})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	var wg sync.WaitGroup
	var mu sync.Mutex
	winners := 0
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, replay, err := controller.CompareAndSwapRewindPlanV2(ctx, ports.CompareAndSwapRewindPlanRequestV2{Expected: created.Ref(), NextState: contract.RewindPlanWorkspaceInspectedV2})
			if err == nil && !replay {
				mu.Lock()
				winners++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if winners != 1 {
		t.Fatalf("CAS winners = %d", winners)
	}
}

func TestRewindPlanControllerV2RejectsTypedNil(t *testing.T) {
	var backend *memory.Backend
	if _, err := domain.NewRewindPlanControllerV2(backend, restorePlanClock(time.Now)); err == nil {
		t.Fatal("typed-nil repository accepted")
	}
}

func TestRewindPlanControllerV2SameIDIsIndependentAcrossTenants(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_752_577_200, 0)
	backend := memory.NewWithClock(func() time.Time { return now })
	controller, _ := domain.NewRewindPlanControllerV2(backend, restorePlanClock(func() time.Time { return now }))
	first := testkit.RewindPlanV2(now)
	second := first.Clone()
	second.Scope.TenantID = "tenant-2"
	second.Scope.ExecutionScopeDigest = "execution-scope-tenant-2-digest"
	second.ConflictDomain = "tenant/tenant-2/sandbox/workspace"
	second.IdempotencyKey = "rewind-plan-request-tenant-2"
	refs := []*contract.ExactFactRefV2{
		&second.CheckpointConsistencyRef, &second.SourceWorkspaceViewRef, &second.PlannedChangeSetRef,
	}
	for i := range second.KeepChangeSetRefs {
		refs = append(refs, &second.KeepChangeSetRefs[i])
	}
	for i := range second.DropChangeSetRefs {
		refs = append(refs, &second.DropChangeSetRefs[i])
	}
	for i := range second.DependencyInspectionRefs {
		refs = append(refs, &second.DependencyInspectionRefs[i])
	}
	for i := range second.ReviewRequirementRefs {
		refs = append(refs, &second.ReviewRequirementRefs[i])
	}
	for i := range second.IrreversibleEffectRefs {
		refs = append(refs, &second.IrreversibleEffectRefs[i])
	}
	for _, ref := range refs {
		ref.TenantID = second.Scope.TenantID
		ref.ScopeDigest = second.Scope.ExecutionScopeDigest
	}
	seal := second.ManifestSealRef.Exact()
	seal.TenantID = second.Scope.TenantID
	seal.ScopeDigest = second.Scope.ExecutionScopeDigest
	second.ManifestSealRef = contract.CheckpointManifestSealRefV2(seal)
	testkit.RefreshRewindPlanV2(&second)
	if _, _, err := controller.CreateRewindPlanV2(ctx, ports.CreateRewindPlanRequestV2{Candidate: first, ExpectAbsent: true}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := controller.CreateRewindPlanV2(ctx, ports.CreateRewindPlanRequestV2{Candidate: second, ExpectAbsent: true}); err != nil {
		t.Fatal(err)
	}
	firstCurrent, err := controller.InspectCurrentRewindPlanV2(ctx, ports.InspectCurrentRewindPlanRequestV2{TenantID: first.Scope.TenantID, ScopeDigest: first.Scope.ExecutionScopeDigest, PlanID: first.PlanID, Owner: first.Owner})
	if err != nil || firstCurrent.Scope.TenantID != "tenant-1" {
		t.Fatalf("tenant 1 current = (%s,%v)", firstCurrent.Scope.TenantID, err)
	}
	secondCurrent, err := controller.InspectCurrentRewindPlanV2(ctx, ports.InspectCurrentRewindPlanRequestV2{TenantID: second.Scope.TenantID, ScopeDigest: second.Scope.ExecutionScopeDigest, PlanID: second.PlanID, Owner: second.Owner})
	if err != nil || secondCurrent.Scope.TenantID != "tenant-2" {
		t.Fatalf("tenant 2 current = (%s,%v)", secondCurrent.Scope.TenantID, err)
	}
}

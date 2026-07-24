package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	continuitysqlite "github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/sqlite"
)

type rewindSQLiteClock func() time.Time

func (c rewindSQLiteClock) Now() time.Time { return c() }

func TestStoreRewindPlanV2DurableHistoryCASAndExpiry(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_752_577_200, 0)
	path := filepath.Join(t.TempDir(), "continuity.db")
	store, err := continuitysqlite.OpenWithClock(ctx, path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	controller, err := domain.NewRewindPlanControllerV2(store, rewindSQLiteClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	plan := testkit.RewindPlanV2(now)
	created, replay, err := controller.CreateRewindPlanV2(ctx, ports.CreateRewindPlanRequestV2{Candidate: plan, ExpectAbsent: true})
	if err != nil || replay {
		t.Fatalf("create = (%v,%v)", replay, err)
	}
	now = now.Add(time.Second)
	advanced, replay, err := controller.CompareAndSwapRewindPlanV2(ctx, ports.CompareAndSwapRewindPlanRequestV2{Expected: created.Ref(), NextState: contract.RewindPlanWorkspaceInspectedV2})
	if err != nil || replay {
		t.Fatalf("CAS = (%v,%v)", replay, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = continuitysqlite.OpenWithClock(ctx, path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	controller, _ = domain.NewRewindPlanControllerV2(store, rewindSQLiteClock(func() time.Time { return now }))
	historical, err := controller.InspectRewindPlanV2(ctx, ports.InspectRewindPlanRequestV2{Ref: created.Ref()})
	if err != nil || historical.State != contract.RewindPlanDraftV2 {
		t.Fatalf("history = (%s,%v)", historical.State, err)
	}
	current, err := controller.InspectCurrentRewindPlanV2(ctx, ports.InspectCurrentRewindPlanRequestV2{TenantID: plan.Scope.TenantID, ScopeDigest: plan.Scope.ExecutionScopeDigest, PlanID: plan.PlanID, Owner: plan.Owner})
	if err != nil || current.Ref() != advanced.Ref() {
		t.Fatalf("current = (%v,%v)", current.Ref(), err)
	}
	recovered, replay, err := controller.CompareAndSwapRewindPlanV2(ctx, ports.CompareAndSwapRewindPlanRequestV2{Expected: created.Ref(), NextState: contract.RewindPlanWorkspaceInspectedV2})
	if err != nil || !replay || recovered.Ref() != advanced.Ref() {
		t.Fatalf("lost-reply recovery = (%v,%v,%v)", recovered.Ref(), replay, err)
	}
	now = time.Unix(0, plan.ExpiresUnixNano)
	if _, err := controller.InspectCurrentRewindPlanV2(ctx, ports.InspectCurrentRewindPlanRequestV2{TenantID: plan.Scope.TenantID, ScopeDigest: plan.Scope.ExecutionScopeDigest, PlanID: plan.PlanID, Owner: plan.Owner}); !contract.HasCode(err, contract.ErrRewindConflict) {
		t.Fatalf("expiry boundary = %v", err)
	}
}

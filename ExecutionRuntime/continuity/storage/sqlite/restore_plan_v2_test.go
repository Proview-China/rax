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

type restoreSQLiteClock func() time.Time

func (c restoreSQLiteClock) Now() time.Time { return c() }

func TestStoreRestorePlanV2DurableHistoryCASAndExpiry(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_752_577_200, 0)
	path := filepath.Join(t.TempDir(), "continuity.db")
	store, err := continuitysqlite.OpenWithClock(ctx, path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	controller, err := domain.NewRestorePlanControllerV2(store, restoreSQLiteClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	plan := testkit.RestorePlanV2(now)
	created, replay, err := controller.CreateRestorePlanV2(ctx, ports.CreateRestorePlanRequestV2{Candidate: plan, ExpectAbsent: true})
	if err != nil || replay {
		t.Fatalf("create = (%v,%v)", replay, err)
	}
	now = now.Add(time.Second)
	advanced, replay, err := controller.CompareAndSwapRestorePlanV2(ctx, ports.CompareAndSwapRestorePlanRequestV2{Expected: created.Ref(), NextState: contract.RestorePlanCheckpointInspectedV2})
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
	controller, err = domain.NewRestorePlanControllerV2(store, restoreSQLiteClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	historical, err := controller.InspectRestorePlanV2(ctx, ports.InspectRestorePlanRequestV2{Ref: created.Ref()})
	if err != nil || historical.State != contract.RestorePlanDraftV2 {
		t.Fatalf("history = (%s,%v)", historical.State, err)
	}
	current, err := controller.InspectCurrentRestorePlanV2(ctx, ports.InspectCurrentRestorePlanRequestV2{
		TenantID: plan.Scope.TenantID, ScopeDigest: plan.Scope.ExecutionScopeDigest, PlanID: plan.PlanID, Owner: plan.Owner,
	})
	if err != nil || current.Ref() != advanced.Ref() {
		t.Fatalf("current = (%v,%v)", current.Ref(), err)
	}
	// A replay after a durable write/lost reply resolves to the same exact fact.
	recovered, replay, err := controller.CompareAndSwapRestorePlanV2(ctx, ports.CompareAndSwapRestorePlanRequestV2{Expected: created.Ref(), NextState: contract.RestorePlanCheckpointInspectedV2})
	if err != nil || !replay || recovered.Ref() != advanced.Ref() {
		t.Fatalf("recovery = (%v,%v,%v)", recovered.Ref(), replay, err)
	}
	now = time.Unix(0, plan.ExpiresUnixNano)
	if _, err := controller.InspectCurrentRestorePlanV2(ctx, ports.InspectCurrentRestorePlanRequestV2{
		TenantID: plan.Scope.TenantID, ScopeDigest: plan.Scope.ExecutionScopeDigest, PlanID: plan.PlanID, Owner: plan.Owner,
	}); !contract.HasCode(err, contract.ErrRestoreIncompatible) {
		t.Fatalf("expired current error = %v", err)
	}
}

package memory_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

func TestContentDeltaRepositoryConcurrentDifferentContentSingleWinner(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	now := time.Date(2026, 7, 17, 18, 0, 0, 0, time.UTC)
	var winners atomic.Int32
	var conflicts atomic.Int32
	var unexpected atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			source := testkit.ContentDeltaSourceV1(testkit.Scope())
			source.Target.ContentDigest = "target-content-" + decimalArtifactV1(i)
			fact, err := contract.NewContentDeltaFactV1("delta-race", "request-race", "request-digest-race", testkit.Scope(), testkit.ContentDeltaOwnerV1(), source, now)
			if err != nil {
				unexpected.Add(1)
				return
			}
			_, replay, err := backend.CreateContentDeltaFactV1(ctx, fact)
			switch {
			case err == nil && !replay:
				winners.Add(1)
			case contract.HasCode(err, contract.ErrRevisionConflict):
				conflicts.Add(1)
			default:
				unexpected.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if winners.Load() != 1 || conflicts.Load() != 63 || unexpected.Load() != 0 {
		t.Fatalf("create-once closure winners=%d conflicts=%d unexpected=%d", winners.Load(), conflicts.Load(), unexpected.Load())
	}
}

func TestContentDeltaRepositoryTenantIsolationExactAndNoAlias(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	now := time.Date(2026, 7, 17, 18, 0, 0, 0, time.UTC)
	base := contentDeltaFactV1(t, testkit.Scope(), "same-delta", "same-request", now)
	if _, _, err := backend.CreateContentDeltaFactV1(ctx, base); err != nil {
		t.Fatal(err)
	}
	otherScope := testkit.Scope()
	otherScope.TenantID = "tenant-2"
	otherScope.ExecutionScopeDigest = "tenant-2-scope"
	other := contentDeltaFactV1(t, otherScope, "same-delta", "same-request", now)
	if _, _, err := backend.CreateContentDeltaFactV1(ctx, other); err != nil {
		t.Fatalf("cross-tenant same ID must be independent: %v", err)
	}
	inspected, err := backend.InspectContentDeltaV1(ctx, ports.InspectContentDeltaRequestV1{Ref: base.Ref()})
	if err != nil {
		t.Fatal(err)
	}
	inspected.TargetRecipe[0].Kind = contract.ContentDeltaAdd
	again, err := backend.InspectContentDeltaV1(ctx, ports.InspectContentDeltaRequestV1{Ref: base.Ref()})
	if err != nil || again.TargetRecipe[0].Kind == contract.ContentDeltaAdd {
		t.Fatal("historical Content Delta aliases caller memory")
	}
}

func contentDeltaFactV1(t *testing.T, scope contract.Scope, deltaID, requestID string, now time.Time) contract.ContentDeltaFactV1 {
	t.Helper()
	fact, err := contract.NewContentDeltaFactV1(deltaID, requestID, "request-digest-1", scope, testkit.ContentDeltaOwnerV1(), testkit.ContentDeltaSourceV1(scope), now)
	if err != nil {
		t.Fatal(err)
	}
	return fact
}

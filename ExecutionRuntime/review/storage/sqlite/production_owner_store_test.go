package sqlite_test

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	reviewsqlite "github.com/Proview-China/rax/ExecutionRuntime/review/storage/sqlite"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestSQLiteOwnerStoreRejectsTypedNilAndNilContextV1(t *testing.T) {
	var store *reviewsqlite.Store
	var owner reviewport.StoreV1 = store
	if _, err := owner.InspectCaseV1(context.Background(), "tenant-typed-nil", "case-typed-nil"); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed-nil Review Store did not fail closed: %v", err)
	}

	path := filepath.Join(t.TempDir(), "review.sqlite")
	now := time.Unix(1_790_000_000, 0)
	live, err := reviewsqlite.Open(context.Background(), reviewsqlite.Config{Path: path, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if _, err = live.InspectCaseV1(nil, "tenant-nil-context", "case-nil-context"); !core.HasReason(err, core.ReasonInvalidReference) {
		t.Fatalf("nil Review Store context did not fail closed: %v", err)
	}
	if err = live.IntegrityCheckV1(nil); !core.HasReason(err, core.ReasonInvalidReference) {
		t.Fatalf("nil integrity context did not fail closed: %v", err)
	}
}

func TestSQLiteOwnerStore64IndependentProcessesHaveOneCASWinnerV1(t *testing.T) {
	const workers = 64
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "review.sqlite")
	now := time.Unix(1_791_000_000, 0)
	clock := func() time.Time { return now }
	stores := make([]*reviewsqlite.Store, workers)
	for index := range stores {
		store, err := reviewsqlite.Open(ctx, reviewsqlite.Config{Path: path, Clock: clock, BusyTimeout: 30 * time.Second, MaxOpenConns: 1})
		if err != nil {
			t.Fatal(err)
		}
		stores[index] = store
		defer store.Close()
	}

	target := testkit.Target(now)
	base, err := contract.SealReviewCaseV1(contract.ReviewCaseV1{
		FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: target.TenantID, ID: "case-64-stores", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()},
		TargetID:       target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest,
		State: contract.CaseRequestedV1, ExpiresUnixNano: now.Add(time.Hour).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	trace := testkit.TraceForTarget(now, base.ID, target, contract.TraceRequestedV1, 1, target.ID)
	if _, err = stores[0].CreateTargetCaseV1(ctx, reviewport.CreateTargetCaseMutationV1{Target: target, Case: base, Trace: trace}); err != nil {
		t.Fatal(err)
	}

	var successes atomic.Int64
	var conflicts atomic.Int64
	errs := make(chan error, workers)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index, store := range stores {
		index, store := index, store
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			next := base
			next.Revision = 2
			next.State = contract.CaseAdmittedV1
			next.UpdatedUnixNano = now.Add(time.Duration(index+1) * time.Nanosecond).UnixNano()
			next.Digest = ""
			sealed, sealErr := contract.SealReviewCaseV1(next)
			if sealErr != nil {
				errs <- sealErr
				return
			}
			trace := testkit.TransitionTrace(now.Add(time.Duration(index+1)*time.Nanosecond), base, contract.CaseAdmittedV1)
			_, casErr := store.TransitionCaseWithTraceV2(ctx, reviewport.TransitionCaseWithTraceMutationV2{Expected: reviewport.ExpectedV1(base.Revision, base.Digest), Next: sealed, Trace: trace})
			switch {
			case casErr == nil:
				successes.Add(1)
			case core.HasCategory(casErr, core.ErrorConflict):
				conflicts.Add(1)
			default:
				errs <- casErr
			}
		}()
	}
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if successes.Load() != 1 || conflicts.Load() != workers-1 {
		t.Fatalf("64 Store CAS linearization drifted: successes=%d conflicts=%d", successes.Load(), conflicts.Load())
	}

	current, err := stores[workers-1].InspectCaseV1(ctx, base.TenantID, base.ID)
	if err != nil || current.Revision != 2 || current.State != contract.CaseAdmittedV1 {
		t.Fatalf("64 Store winner is not current: %+v err=%v", current, err)
	}
	historical, err := stores[1].InspectCaseExactV1(ctx, base.TenantID, reviewport.ExactV1(base.ID, base.Revision, base.Digest))
	if err != nil || historical.Digest != base.Digest {
		t.Fatalf("64 Store CAS damaged exact history: %+v err=%v", historical, err)
	}
	if err := stores[2].IntegrityCheckV1(ctx); err != nil {
		t.Fatal(err)
	}
}

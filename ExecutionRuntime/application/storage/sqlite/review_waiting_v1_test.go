package sqlite

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestSQLiteReviewWaitingRestartLostReplyHistoryAndABAClosedV1(t *testing.T) {
	now := time.Unix(2_910_000_000, 0)
	request, initial, claimed := sqliteReviewWaitingFactsV1(t, now, "restart")
	path := t.TempDir() + "/application.db"
	store := openTestStoreV1(t, path, now.Add(5*time.Second))
	store.LoseNextReviewWaitingCreateReplyV1()
	if _, err := store.CreateReviewWaitingCoordinationV1(context.Background(), initial); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("create lost reply missing: %v", err)
	}
	_ = store.Close()
	store = openTestStoreV1(t, path, now.Add(5*time.Second))
	if got, err := store.InspectCurrentReviewWaitingCoordinationV1(context.Background(), request.ExecutionScope, request.ID); err != nil || got.RefV1() != initial.RefV1() {
		t.Fatalf("restart create Inspect: %+v %v", got, err)
	}
	store.LoseNextReviewWaitingCASReplyV1()
	cas := applicationports.ReviewWaitingCoordinationCASRequestV1{Scope: request.ExecutionScope, Expected: initial.RefV1(), Next: claimed}
	if _, err := store.CompareAndSwapReviewWaitingCoordinationV1(context.Background(), cas); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("CAS lost reply missing: %v", err)
	}
	_ = store.Close()
	store = openTestStoreV1(t, path, now.Add(5*time.Second))
	defer store.Close()
	if got, err := store.InspectCurrentReviewWaitingCoordinationV1(context.Background(), request.ExecutionScope, request.ID); err != nil || got.RefV1() != claimed.RefV1() {
		t.Fatalf("restart CAS Inspect: %+v %v", got, err)
	}
	if old, err := store.InspectHistoricalReviewWaitingCoordinationV1(context.Background(), request.ExecutionScope, initial.RefV1()); err != nil || old.RefV1() != initial.RefV1() {
		t.Fatalf("historical exact lost: %+v %v", old, err)
	}
	if replay, err := store.CompareAndSwapReviewWaitingCoordinationV1(context.Background(), cas); err != nil || replay.Applied || replay.Fact.RefV1() != claimed.RefV1() {
		t.Fatalf("idempotent CAS receipt drift: %+v %v", replay, err)
	}
	if _, err := store.db.Exec(`UPDATE application_fact_current_v1 SET revision=(SELECT MIN(revision) FROM application_fact_history_v1 WHERE fact_type=? AND fact_id=?),digest=(SELECT digest FROM application_fact_history_v1 WHERE fact_type=? AND fact_id=? ORDER BY revision LIMIT 1),row_digest=(SELECT row_digest FROM application_fact_history_v1 WHERE fact_type=? AND fact_id=? ORDER BY revision LIMIT 1) WHERE fact_type=? AND fact_id=?`, reviewWaitingCoordinationKindV1, request.ID, reviewWaitingCoordinationKindV1, request.ID, reviewWaitingCoordinationKindV1, request.ID, reviewWaitingCoordinationKindV1, request.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InspectCurrentReviewWaitingCoordinationV1(context.Background(), request.ExecutionScope, request.ID); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("ABA current accepted: %v", err)
	}
}

func TestSQLiteReviewWaitingStagedFailureZeroWritesAndCrossScopeV1(t *testing.T) {
	now := time.Unix(2_910_000_000, 0)
	request, initial, _ := sqliteReviewWaitingFactsV1(t, now, "stage")
	store := openTestStoreV1(t, t.TempDir()+"/application.db", now.Add(5*time.Second))
	defer store.Close()
	store.FailNextReviewWaitingCreateBeforeCommitV1(core.ErrorUnavailable)
	if _, err := store.CreateReviewWaitingCoordinationV1(context.Background(), initial); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("staged create fault missing: %v", err)
	}
	var history, current int
	if err := store.db.QueryRow(`SELECT COUNT(1) FROM application_fact_history_v1 WHERE fact_type=? AND fact_id=?`, reviewWaitingCoordinationKindV1, request.ID).Scan(&history); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(1) FROM application_fact_current_v1 WHERE fact_type=? AND fact_id=?`, reviewWaitingCoordinationKindV1, request.ID).Scan(&current); err != nil {
		t.Fatal(err)
	}
	if history != 0 || current != 0 {
		t.Fatalf("staged create leaked history=%d current=%d", history, current)
	}
	if _, err := store.CreateReviewWaitingCoordinationV1(context.Background(), initial); err != nil {
		t.Fatal(err)
	}
	other := request.ExecutionScope
	other.Identity.TenantID = "other-tenant"
	if _, err := store.InspectCurrentReviewWaitingCoordinationV1(context.Background(), other, request.ID); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("cross-scope Inspect accepted: %v", err)
	}
}

func TestSQLiteReviewWaiting64StoresOneAppliedCASV1(t *testing.T) {
	now := time.Unix(2_910_000_000, 0)
	request, initial, claimed := sqliteReviewWaitingFactsV1(t, now, "race")
	path := t.TempDir() + "/application.db"
	const workers = 64
	stores := make([]*StoreV1, workers)
	for i := range stores {
		stores[i] = openTestStoreV1(t, path, now.Add(5*time.Second))
		defer stores[i].Close()
	}
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := range stores {
		wg.Add(1)
		go func(store *StoreV1) {
			defer wg.Done()
			receipt, err := store.CreateReviewWaitingCoordinationV1(context.Background(), initial)
			if err == nil && receipt.Fact.RefV1() != initial.RefV1() {
				err = fmt.Errorf("create ref drift")
			}
			errs <- err
		}(stores[i])
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var applied atomic.Int32
	errs = make(chan error, workers)
	cas := applicationports.ReviewWaitingCoordinationCASRequestV1{Scope: request.ExecutionScope, Expected: initial.RefV1(), Next: claimed}
	for i := range stores {
		wg.Add(1)
		go func(store *StoreV1) {
			defer wg.Done()
			receipt, err := store.CompareAndSwapReviewWaitingCoordinationV1(context.Background(), cas)
			if err == nil && receipt.Applied {
				applied.Add(1)
			}
			errs <- err
		}(stores[i])
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if applied.Load() != 1 {
		t.Fatalf("applied CAS winners=%d want=1", applied.Load())
	}
	if got, err := stores[0].InspectCurrentReviewWaitingCoordinationV1(context.Background(), request.ExecutionScope, request.ID); err != nil || got.RefV1() != claimed.RefV1() {
		t.Fatalf("concurrent current drift: %+v %v", got, err)
	}
}

func sqliteReviewWaitingFactsV1(t *testing.T, now time.Time, suffix string) (contract.ReviewWaitingRequestV1, contract.ReviewWaitingCoordinationFactV1, contract.ReviewWaitingCoordinationFactV1) {
	t.Helper()
	tenant := core.TenantID("tenant-sqlite-review-" + suffix)
	digest := func(value string) core.Digest {
		d, err := core.CanonicalJSONDigest("sqlite-review-waiting", "v1", "value", value+suffix)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: tenant, ID: "agent", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage", PlanDigest: digest("plan")}, Instance: core.InstanceRef{ID: "instance", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "sandbox", Epoch: 1}, AuthorityEpoch: 1}
	scopeDigest, _ := runtimeports.ExecutionScopeDigestV2(scope)
	target := contract.ReviewWaitingTargetCoordinateV1{TenantID: tenant, ID: "target-" + suffix, Revision: 1, Digest: digest("target"), RunID: core.AgentRunID("run-" + suffix), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	request, err := contract.SealReviewWaitingRequestV1(contract.ReviewWaitingRequestV1{Delivery: contract.ReviewWaitingInlineV1, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, Phase: contract.ReviewPhasePointCoordinateV1{Kind: contract.ReviewPhaseActionV1, ID: "phase-" + suffix, Revision: 1, Digest: digest("phase"), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}, Target: target, ReviewRequest: contract.ReviewRequestCoordinateV1{TenantID: tenant, ID: "review-request-" + suffix, Revision: 1, Digest: digest("request"), CaseID: "case-" + suffix}, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(45 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	initial, err := contract.NewReviewWaitingCoordinationFactV1(request, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := contract.ClaimReviewWaitingStartV1(initial, "review-start-claim/"+suffix, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return request, initial, claimed
}

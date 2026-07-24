package sqlite

import (
	"context"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestSQLiteDispatchAuthorityV3FaultHistoryCASAndABA(t *testing.T) {
	now := time.Unix(2_830_000_000, 0)
	store := openTestStore(t, testDBPath(t), func() time.Time { return now })
	first := sqliteDispatchAuthorityV3(t, now)
	store.failNextStageForTest()
	if _, err := store.CommitAuthorityFactV3(context.Background(), ports.DispatchAuthorityFactPublishRequestV3{Value: first}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("stage failure missing: %v", err)
	}
	assertAuthorityRowsV3(t, store, reviewGovernanceProjectionDispatchAuthorityFactV3, 0, 0)
	store.loseNextReplyForTest()
	if _, err := store.CommitAuthorityFactV3(context.Background(), ports.DispatchAuthorityFactPublishRequestV3{Value: first}); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost reply missing: %v", err)
	}
	old, err := store.InspectHistoricalAuthorityFactV3(context.Background(), first.Ref)
	if err != nil || !reflect.DeepEqual(old, first) {
		t.Fatalf("lost reply exact history failed: %+v %v", old, err)
	}
	next := sqliteNextDispatchAuthorityV3(t, first, now.Add(time.Second), true)
	if _, err := store.CommitAuthorityFactV3(context.Background(), ports.DispatchAuthorityFactPublishRequestV3{Previous: &first.Ref, Value: next}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InspectCurrentAuthorityFactV3(context.Background(), first.Ref); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("stale authority Ref accepted: %v", err)
	}
	if old, err := store.InspectHistoricalAuthorityFactV3(context.Background(), first.Ref); err != nil || !reflect.DeepEqual(old, first) {
		t.Fatalf("authority history rewritten: %+v %v", old, err)
	}
	if _, err := store.db.Exec(`UPDATE runtime_review_governance_projection_current SET highest_revision=highest_revision+1 WHERE kind=?`, reviewGovernanceProjectionDispatchAuthorityFactV3); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InspectCurrentAuthorityFactV3(context.Background(), next.Ref); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("bad highest/ABA accepted: %v", err)
	}
}
func TestSQLiteDispatchAuthorityV3ConcurrentCanonicalCreateOnce(t *testing.T) {
	now := time.Unix(2_830_000_000, 0)
	store := openTestStore(t, testDBPath(t), func() time.Time { return now })
	value := sqliteDispatchAuthorityV3(t, now)
	const workers = 32
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.CommitAuthorityFactV3(context.Background(), ports.DispatchAuthorityFactPublishRequestV3{Value: value})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("canonical concurrent create failed: %v", err)
		}
	}
	assertAuthorityRowsV3(t, store, reviewGovernanceProjectionDispatchAuthorityFactV3, 1, 1)
}
func TestSQLiteReviewActorAuthorityV2FaultHistoryAndConcurrentCreateOnce(t *testing.T) {
	now := time.Unix(2_830_000_000, 0)
	store := openTestStore(t, testDBPath(t), func() time.Time { return now })
	value := sqliteActorProjectionV2(t, now)
	store.failNextStageForTest()
	if _, err := store.CommitActorAuthorityV2(context.Background(), ports.ReviewActorAuthorityCurrentPublishRequestV2{Value: value}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("actor stage failure missing: %v", err)
	}
	assertAuthorityRowsV3(t, store, reviewGovernanceProjectionReviewActorAuthorityV2, 0, 0)
	const workers = 32
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.CommitActorAuthorityV2(context.Background(), ports.ReviewActorAuthorityCurrentPublishRequestV2{Value: value})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("actor canonical concurrent create failed: %v", err)
		}
	}
	next := sqliteNextActorProjectionV2(t, value, now.Add(time.Second), true)
	store.loseNextReplyForTest()
	if _, err := store.CommitActorAuthorityV2(context.Background(), ports.ReviewActorAuthorityCurrentPublishRequestV2{Previous: &value.Ref, Value: next}); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("actor lost reply missing: %v", err)
	}
	if old, err := store.InspectHistoricalActorAuthorityV2(context.Background(), value.Ref); err != nil || !reflect.DeepEqual(old, value) {
		t.Fatalf("actor history lost: %+v %v", old, err)
	}
	if _, err := store.ResolveActorAuthorityV2(context.Background(), value.Subject); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("actor stale subject resolved: %v", err)
	}
	if _, err := store.db.Exec(`UPDATE runtime_review_governance_projection_current SET highest_revision=highest_revision+1 WHERE kind=?`, reviewGovernanceProjectionReviewActorAuthorityV2); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InspectCurrentActorAuthorityV2(context.Background(), next.Subject, next.Ref); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("actor bad highest/ABA accepted: %v", err)
	}
}
func sqliteDispatchAuthorityV3(t *testing.T, now time.Time) ports.DispatchAuthorityFactV3 {
	t.Helper()
	d := func(v string) core.Digest {
		x, e := core.CanonicalJSONDigest("sqlite-authority-v3", "v1", "value", v)
		if e != nil {
			t.Fatal(e)
		}
		return x
	}
	tenant := core.TenantID("tenant-sqlite-actor")
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: tenant, ID: "agent", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage", PlanDigest: d("plan")}, Instance: core.InstanceRef{ID: "instance", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "sandbox", Epoch: 1}, AuthorityEpoch: 1}
	f, e := ports.SealDispatchAuthorityFactV3(ports.DispatchAuthorityFactV3{Ref: ports.AuthorityBindingRefV2{Ref: "authority", Revision: 1, Epoch: 1}, Scope: scope, RunID: "run", ActionScopeDigest: d("action"), State: ports.AuthorityFactActive, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano()})
	if e != nil {
		t.Fatal(e)
	}
	return f
}
func sqliteNextDispatchAuthorityV3(t *testing.T, f ports.DispatchAuthorityFactV3, now time.Time, active bool) ports.DispatchAuthorityFactV3 {
	t.Helper()
	f = f.Clone()
	f.Ref.Revision++
	f.Ref.Epoch++
	f.Scope.AuthorityEpoch = f.Ref.Epoch
	f.CheckedUnixNano = now.UnixNano()
	f.ExpiresUnixNano = now.Add(30 * time.Second).UnixNano()
	f.State = ports.AuthorityFactActive
	if !active {
		f.State = ports.AuthorityFactRevoked
	}
	f, e := ports.SealDispatchAuthorityFactV3(f)
	if e != nil {
		t.Fatal(e)
	}
	return f
}
func sqliteActorProjectionV2(t *testing.T, now time.Time) ports.ReviewActorAuthorityCurrentProjectionV2 {
	t.Helper()
	f := sqliteDispatchAuthorityV3(t, now)
	d, _ := core.CanonicalJSONDigest("sqlite-actor-v2", "v1", "value", "target")
	s := ports.ReviewActorAuthorityCurrentSubjectV2{Target: ports.ReviewDecisionTargetRefV1{TenantID: f.Scope.Identity.TenantID, ID: "target", Revision: 1, Digest: d, RunID: f.RunID}, ActorAuthority: f.Ref, ActionScopeDigest: f.ActionScopeDigest}
	p, e := ports.SealReviewActorAuthorityCurrentProjectionV2(ports.ReviewActorAuthorityCurrentProjectionV2{Ref: ports.ReviewActorAuthorityCurrentProjectionRefV2{Revision: 1}, Subject: s, Fact: f, State: ports.ReviewDecisionGovernanceProjectionActiveV1, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano()})
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func sqliteNextActorProjectionV2(t *testing.T, p ports.ReviewActorAuthorityCurrentProjectionV2, now time.Time, active bool) ports.ReviewActorAuthorityCurrentProjectionV2 {
	t.Helper()
	p = p.Clone()
	p.Ref.Revision++
	p.Fact = sqliteNextDispatchAuthorityV3(t, p.Fact, now, active)
	p.Subject.ActorAuthority = p.Fact.Ref
	p.CheckedUnixNano = now.UnixNano()
	p.ExpiresUnixNano = now.Add(20 * time.Second).UnixNano()
	p.State = ports.ReviewDecisionGovernanceProjectionActiveV1
	p.Current = active
	if !active {
		p.State = ports.ReviewDecisionGovernanceProjectionRevokedV1
	}
	p, e := ports.SealReviewActorAuthorityCurrentProjectionV2(p)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func assertAuthorityRowsV3(t *testing.T, store *Store, kind string, history, current int) {
	t.Helper()
	var got int
	if err := store.db.QueryRow(`SELECT COUNT(1) FROM runtime_review_governance_projection_history WHERE kind=?`, kind).Scan(&got); err != nil || got != history {
		t.Fatalf("%s history rows=%d want=%d err=%v", kind, got, history, err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(1) FROM runtime_review_governance_projection_current WHERE kind=?`, kind).Scan(&got); err != nil || got != current {
		t.Fatalf("%s current rows=%d want=%d err=%v", kind, got, current, err)
	}
}

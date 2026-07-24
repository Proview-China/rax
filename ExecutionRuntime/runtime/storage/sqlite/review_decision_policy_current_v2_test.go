package sqlite

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestSQLiteReviewDecisionPolicyCurrentV2HistoryCASFaultsAndRecovery(t *testing.T) {
	now := time.Unix(2_720_000_000, 0)
	store := openTestStore(t, testDBPath(t), func() time.Time { return now })
	first := sqliteReviewDecisionPolicyV2(t, now, 1, true)
	store.failNextStageForTest()
	if _, err := store.CommitPolicyV2(context.Background(), ports.ReviewDecisionPolicyCurrentPublishRequestV2{Value: first}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("staged failure not surfaced: %v", err)
	}
	if _, err := store.ResolvePolicyV2(context.Background(), first.Subject); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("staged failure leaked current/history: %v", err)
	}
	store.loseNextReplyForTest()
	if _, err := store.CommitPolicyV2(context.Background(), ports.ReviewDecisionPolicyCurrentPublishRequestV2{Value: first}); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost reply not surfaced: %v", err)
	}
	historical, err := store.InspectHistoricalPolicyV2(context.Background(), first.Ref)
	if err != nil || !reflect.DeepEqual(historical, first) {
		t.Fatalf("lost reply exact Inspect failed: %+v %v", historical, err)
	}
	next := sqliteNextReviewDecisionPolicyV2(t, first, now.Add(time.Second), true)
	if _, err := store.CommitPolicyV2(context.Background(), ports.ReviewDecisionPolicyCurrentPublishRequestV2{Previous: &first.Ref, Value: next}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InspectCurrentPolicyV2(context.Background(), first.Subject, first.Ref); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("stale full Ref accepted: %v", err)
	}
	if _, err := store.ResolvePolicyV2(context.Background(), first.Subject); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("stale exact subject resolved: %v", err)
	}
	if _, err := store.ResolvePolicyV2(context.Background(), first.Subject); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("stale exact subject resolved: %v", err)
	}
	if old, err := store.InspectHistoricalPolicyV2(context.Background(), first.Ref); err != nil || !reflect.DeepEqual(old, first) {
		t.Fatalf("append-only history lost: %+v %v", old, err)
	}
	changed := next
	changed.ProjectionDigest = first.ProjectionDigest
	if _, err := store.CommitPolicyV2(context.Background(), ports.ReviewDecisionPolicyCurrentPublishRequestV2{Previous: &first.Ref, Value: changed}); err == nil {
		t.Fatal("changed replay accepted")
	}
}

func TestSQLiteReviewDecisionPolicyCurrentV2ConcurrentCreateOnce(t *testing.T) {
	now := time.Unix(2_720_000_000, 0)
	store := openTestStore(t, testDBPath(t), func() time.Time { return now })
	base := sqliteReviewDecisionPolicyV2(t, now, 1, true)
	const workers = 32
	var wg sync.WaitGroup
	results := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.CommitPolicyV2(context.Background(), ports.ReviewDecisionPolicyCurrentPublishRequestV2{Value: base})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("same canonical concurrent replay failed: %v", err)
		}
	}
	ref, err := store.ResolvePolicyV2(context.Background(), base.Subject)
	if err != nil || ref != base.Ref {
		t.Fatalf("concurrent create current drifted: %+v %v", ref, err)
	}
}

func sqliteReviewDecisionPolicyV2(t *testing.T, now time.Time, revision core.Revision, active bool) ports.ReviewDecisionPolicyCurrentProjectionV2 {
	t.Helper()
	tenant := core.TenantID("tenant-sqlite-v2")
	d := func(v string) core.Digest {
		x, e := core.CanonicalJSONDigest("sqlite-review-policy-v2", "v1", "value", v)
		if e != nil {
			t.Fatal(e)
		}
		return x
	}
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: tenant, ID: "agent", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage", PlanDigest: d("plan")}, Instance: core.InstanceRef{ID: "instance", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "sandbox", Epoch: 1}, AuthorityEpoch: 1}
	subject := ports.ReviewDecisionPolicyApplicabilitySubjectV2{TenantID: tenant, TargetID: "target", TargetRevision: 1, IntentID: "intent", IntentRevision: 1, IntentSubjectDigest: d("subject"), PayloadRevision: 1, PayloadDigest: d("payload"), RunID: "run", Scope: scope, CurrentScope: ports.ExecutionScopeBindingRefV2{Ref: "scope", Revision: 1, Digest: d("scope")}, ActionScopeDigest: d("action"), ActorAuthority: ports.AuthorityBindingRefV2{Ref: "actor", Revision: 1, Digest: d("actor"), Epoch: 1}, Policy: ports.ReviewPolicyBindingRefV2{Ref: "policy", Revision: revision, Digest: d("placeholder")}}
	fact := ports.ReviewPolicyFactV2{Ref: "policy", Revision: revision, SubjectDigest: subject.IntentSubjectDigest, Scope: scope, RunID: "run", CurrentScope: subject.CurrentScope, RiskClass: "review.test/controlled", ActorAuthorityRef: "actor", ReviewerAuthorityRef: "reviewer", PolicyDecisionRef: "decision", Active: active, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	fd, e := fact.DigestV2()
	if e != nil {
		t.Fatal(e)
	}
	fact.Digest = fd
	subject.Policy.Digest = fd
	state := ports.ReviewDecisionGovernanceProjectionActiveV1
	if !active {
		state = ports.ReviewDecisionGovernanceProjectionRevokedV1
	}
	p, e := ports.SealReviewDecisionPolicyCurrentProjectionV2(ports.ReviewDecisionPolicyCurrentProjectionV2{Ref: ports.ReviewDecisionPolicyCurrentProjectionRefV2{Revision: revision}, Subject: subject, Fact: fact, State: state, Current: active, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano()})
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func sqliteNextReviewDecisionPolicyV2(t *testing.T, current ports.ReviewDecisionPolicyCurrentProjectionV2, now time.Time, active bool) ports.ReviewDecisionPolicyCurrentProjectionV2 {
	t.Helper()
	next := current
	next.Ref.Revision++
	next.Subject.Policy.Revision++
	next.CheckedUnixNano = now.UnixNano()
	next.ExpiresUnixNano = now.Add(30 * time.Second).UnixNano()
	next.Fact.Revision = next.Subject.Policy.Revision
	next.Fact.Active = active
	next.Fact.ExpiresUnixNano = now.Add(time.Minute).UnixNano()
	next.Fact.Digest = ""
	fd, e := next.Fact.DigestV2()
	if e != nil {
		t.Fatal(e)
	}
	next.Fact.Digest = fd
	next.Subject.Policy.Digest = fd
	next.State = ports.ReviewDecisionGovernanceProjectionActiveV1
	next.Current = active
	if !active {
		next.State = ports.ReviewDecisionGovernanceProjectionRevokedV1
	}
	next, e = ports.SealReviewDecisionPolicyCurrentProjectionV2(next)
	if e != nil {
		t.Fatal(e)
	}
	return next
}

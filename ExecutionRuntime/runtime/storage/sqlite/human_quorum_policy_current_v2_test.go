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

func TestSQLiteHumanQuorumPolicyV2RestartHistoryCurrentDeepCloneAndABA(t *testing.T) {
	now := time.Unix(2_600_000_000, 0)
	path := testDBPath(t)
	clockNow := now
	store := openTestStore(t, path, func() time.Time { return clockNow })
	first := sqliteHumanQuorumPolicyV2(t, now, "tenant-restart", "production/release", 1)
	receipt, err := store.PublishHumanQuorumPolicyCurrentV2(context.Background(), ports.HumanQuorumPolicyCurrentPublishRequestV2{Value: first})
	if err != nil || !receipt.Created || receipt.Ref != first.Ref {
		t.Fatalf("initial publish failed: %+v %v", receipt, err)
	}
	resolved, err := store.ResolveCurrentHumanQuorumPolicyV2(context.Background(), ports.HumanQuorumPolicyCurrentResolveRequestV2{Subject: first.Subject})
	if err != nil || resolved != first.Ref {
		t.Fatalf("Resolve did not return exact current Ref: %+v %v", resolved, err)
	}
	aliased, err := store.InspectCurrentHumanQuorumPolicyV2(context.Background(), first.Subject, first.Ref)
	if err != nil {
		t.Fatal(err)
	}
	aliased.RoleRequirements[0].Role = "mutated"
	aliased.RejectVetoRoles[0] = "mutated"
	clean, err := store.InspectHistoricalHumanQuorumPolicyV2(context.Background(), first.Ref)
	if err != nil || !reflect.DeepEqual(clean, first) {
		t.Fatalf("stored policy leaked a mutable alias: %+v %v", clean, err)
	}

	clockNow = now.Add(time.Second)
	next := sqliteNextHumanQuorumPolicyV2(t, first, clockNow, ports.HumanQuorumPolicyProjectionActiveV2)
	next.AcceptThreshold = 3
	next, err = ports.SealHumanQuorumPolicyCurrentProjectionV2(next)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PublishHumanQuorumPolicyCurrentV2(context.Background(), ports.HumanQuorumPolicyCurrentPublishRequestV2{Previous: &first.Ref, Value: next}); err != nil {
		t.Fatal(err)
	}
	if old, err := store.InspectHistoricalHumanQuorumPolicyV2(context.Background(), first.Ref); err != nil || old.ProjectionDigest != first.ProjectionDigest {
		t.Fatalf("revision two overwrote historical revision one: %+v %v", old, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTestStore(t, path, func() time.Time { return clockNow })
	if ref, err := reopened.ResolveCurrentHumanQuorumPolicyV2(context.Background(), ports.HumanQuorumPolicyCurrentResolveRequestV2{Subject: first.Subject}); err != nil || ref != next.Ref {
		t.Fatalf("restart lost current policy: %+v %v", ref, err)
	}
	if old, err := reopened.InspectHistoricalHumanQuorumPolicyV2(context.Background(), first.Ref); err != nil || !reflect.DeepEqual(old, first) {
		t.Fatalf("restart lost exact history: %+v %v", old, err)
	}

	if _, err := reopened.db.Exec(`UPDATE runtime_review_governance_projection_current SET highest_revision=? WHERE kind=? AND tenant_id=? AND projection_id=?`, next.Ref.Revision+1, reviewGovernanceProjectionHumanQuorumPolicyV2, string(first.Subject.TenantID), first.Ref.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.ResolveCurrentHumanQuorumPolicyV2(context.Background(), ports.HumanQuorumPolicyCurrentResolveRequestV2{Subject: first.Subject}); !core.HasReason(err, core.ReasonRevisionConflict) {
		t.Fatalf("highest-revision corruption was accepted: %v", err)
	}
	if _, err := reopened.db.Exec(`UPDATE runtime_review_governance_projection_current SET revision=?,projection_digest=?,highest_revision=? WHERE kind=? AND tenant_id=? AND projection_id=?`, first.Ref.Revision, string(first.Ref.Digest), first.Ref.Revision, reviewGovernanceProjectionHumanQuorumPolicyV2, string(first.Subject.TenantID), first.Ref.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.ResolveCurrentHumanQuorumPolicyV2(context.Background(), ports.HumanQuorumPolicyCurrentResolveRequestV2{Subject: first.Subject}); !core.HasReason(err, core.ReasonRevisionConflict) {
		t.Fatalf("full current/highest ABA rollback was accepted despite newer history: %v", err)
	}
	if old, err := reopened.InspectHistoricalHumanQuorumPolicyV2(context.Background(), first.Ref); err != nil || old.Ref != first.Ref {
		t.Fatalf("bad current index contaminated exact history: %+v %v", old, err)
	}
}

func TestSQLiteHumanQuorumPolicyV2FaultsLostReplyTTLAndClockRollback(t *testing.T) {
	now := time.Unix(2_610_000_000, 0)
	clockNow := now
	store := openTestStore(t, testDBPath(t), func() time.Time { return clockNow })
	staged := sqliteHumanQuorumPolicyV2(t, now, "tenant-fault", "finance/payment", 1)
	store.failNextStageForTest()
	if _, err := store.PublishHumanQuorumPolicyCurrentV2(context.Background(), ports.HumanQuorumPolicyCurrentPublishRequestV2{Value: staged}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("staged publish did not fail unavailable: %v", err)
	}
	assertSQLiteHumanQuorumPolicyRowsV2(t, store, 0, 0)
	if _, err := store.ResolveCurrentHumanQuorumPolicyV2(context.Background(), ports.HumanQuorumPolicyCurrentResolveRequestV2{Subject: staged.Subject}); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("missing current index did not return authoritative NotFound: %v", err)
	}
	if _, err := store.InspectHistoricalHumanQuorumPolicyV2(context.Background(), staged.Ref); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("staged publish leaked exact history: %v", err)
	}

	store.loseNextReplyForTest()
	if _, err := store.PublishHumanQuorumPolicyCurrentV2(context.Background(), ports.HumanQuorumPolicyCurrentPublishRequestV2{Value: staged}); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost publish reply was not indeterminate: %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if recovered, err := store.InspectHistoricalHumanQuorumPolicyV2(context.WithoutCancel(canceled), staged.Ref); err != nil || recovered.Ref != staged.Ref {
		t.Fatalf("lost publish did not recover through detached exact Inspect: %+v %v", recovered, err)
	}
	assertSQLiteHumanQuorumPolicyRowsV2(t, store, 1, 1)

	clockNow = now.Add(time.Second)
	next := sqliteNextHumanQuorumPolicyV2(t, staged, clockNow, ports.HumanQuorumPolicyProjectionActiveV2)
	store.failNextStageForTest()
	if _, err := store.PublishHumanQuorumPolicyCurrentV2(context.Background(), ports.HumanQuorumPolicyCurrentPublishRequestV2{Previous: &staged.Ref, Value: next}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("staged CAS publish did not fail unavailable: %v", err)
	}
	if current, err := store.InspectCurrentHumanQuorumPolicyV2(context.Background(), staged.Subject, staged.Ref); err != nil || current.Ref != staged.Ref {
		t.Fatalf("staged CAS changed current: %+v %v", current, err)
	}
	if _, err := store.InspectHistoricalHumanQuorumPolicyV2(context.Background(), next.Ref); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("staged CAS leaked history: %v", err)
	}

	store.loseNextReplyForTest()
	if _, err := store.PublishHumanQuorumPolicyCurrentV2(context.Background(), ports.HumanQuorumPolicyCurrentPublishRequestV2{Previous: &staged.Ref, Value: next}); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost CAS reply was not indeterminate: %v", err)
	}
	if recovered, err := store.InspectHistoricalHumanQuorumPolicyV2(context.WithoutCancel(canceled), next.Ref); err != nil || recovered.Ref != next.Ref {
		t.Fatalf("lost CAS did not recover exact next revision: %+v %v", recovered, err)
	}
	assertSQLiteHumanQuorumPolicyRowsV2(t, store, 2, 1)

	clockNow = time.Unix(0, next.ExpiresUnixNano)
	if _, err := store.ResolveCurrentHumanQuorumPolicyV2(context.Background(), ports.HumanQuorumPolicyCurrentResolveRequestV2{Subject: next.Subject}); !core.HasReason(err, core.ReasonReviewVerdictStale) {
		t.Fatalf("TTL crossing did not fail closed: %v", err)
	}
	assertSQLiteHumanQuorumPolicyRowsV2(t, store, 2, 1)
	if historical, err := store.InspectHistoricalHumanQuorumPolicyV2(context.Background(), next.Ref); err != nil || !reflect.DeepEqual(historical, next) {
		t.Fatalf("passage of time rewrote the sealed projection: %+v %v", historical, err)
	}

	store.clock = sequenceClockV2(now.Add(2*time.Second), now.Add(time.Second))
	if _, err := store.InspectCurrentHumanQuorumPolicyV2(context.Background(), next.Subject, next.Ref); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock rollback across Inspect was accepted: %v", err)
	}
	if _, err := store.InspectHistoricalHumanQuorumPolicyV2(canceled, next.Ref); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("canceled exact Inspect did not preserve Indeterminate: %v", err)
	}
}

func TestSQLiteHumanQuorumPolicyV2ConcurrentCreateOnceAndIsolation(t *testing.T) {
	now := time.Unix(2_620_000_000, 0)
	store := openTestStore(t, testDBPath(t), func() time.Time { return now })
	const workers = 64
	values := make([]ports.HumanQuorumPolicyCurrentProjectionV2, workers)
	for i := range values {
		values[i] = sqliteHumanQuorumPolicyV2(t, now, "tenant-race", "production/deploy", 1)
		values[i].MaxVoteTTLNanos += int64(i)
		var err error
		values[i], err = ports.SealHumanQuorumPolicyCurrentProjectionV2(values[i])
		if err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	created := make(chan ports.HumanQuorumPolicyCurrentProjectionV2, workers)
	errs := make(chan error, workers)
	for i := range values {
		wg.Add(1)
		go func(value ports.HumanQuorumPolicyCurrentProjectionV2) {
			defer wg.Done()
			receipt, err := store.PublishHumanQuorumPolicyCurrentV2(context.Background(), ports.HumanQuorumPolicyCurrentPublishRequestV2{Value: value})
			if err != nil {
				errs <- err
				return
			}
			if receipt.Created {
				created <- value
			}
		}(values[i])
	}
	wg.Wait()
	close(created)
	close(errs)
	if len(created) != 1 {
		t.Fatalf("64 concurrent changed publishes created %d policies", len(created))
	}
	for err := range errs {
		if !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("unexpected concurrent loser: %v", err)
		}
	}
	winner := <-created
	if replay, err := store.PublishHumanQuorumPolicyCurrentV2(context.Background(), ports.HumanQuorumPolicyCurrentPublishRequestV2{Value: winner}); err != nil || replay.Created {
		t.Fatalf("same canonical replay was not idempotent: %+v %v", replay, err)
	}
	assertSQLiteHumanQuorumPolicyRowsV2(t, store, 1, 1)

	isolated := []ports.HumanQuorumPolicyCurrentProjectionV2{
		sqliteHumanQuorumPolicyV2(t, now, "tenant-other", "production/deploy", 1),
		sqliteHumanQuorumPolicyV2(t, now, "tenant-race", "production/release", 1),
	}
	for _, value := range isolated {
		if _, err := store.PublishHumanQuorumPolicyCurrentV2(context.Background(), ports.HumanQuorumPolicyCurrentPublishRequestV2{Value: value}); err != nil {
			t.Fatalf("tenant/domain-isolated policy conflicted: %v", err)
		}
	}
	assertSQLiteHumanQuorumPolicyRowsV2(t, store, 3, 3)
}

func TestSQLiteHumanQuorumPolicyV2PublishActualPointClockAndTTL(t *testing.T) {
	now := time.Unix(2_625_000_000, 0)
	store := openTestStore(t, testDBPath(t), func() time.Time { return now })
	value := sqliteHumanQuorumPolicyV2(t, now, "tenant-actual-point", "security/incident", 1)
	store.clock = sequenceClockV2(now.Add(2*time.Second), now.Add(time.Second))
	if _, err := store.PublishHumanQuorumPolicyCurrentV2(context.Background(), ports.HumanQuorumPolicyCurrentPublishRequestV2{Value: value}); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("publish clock rollback was accepted: %v", err)
	}
	assertSQLiteHumanQuorumPolicyRowsV2(t, store, 0, 0)

	store.clock = sequenceClockV2(now, time.Unix(0, value.ExpiresUnixNano))
	if _, err := store.PublishHumanQuorumPolicyCurrentV2(context.Background(), ports.HumanQuorumPolicyCurrentPublishRequestV2{Value: value}); !core.HasReason(err, core.ReasonReviewVerdictStale) {
		t.Fatalf("publish actual-point TTL crossing was accepted: %v", err)
	}
	assertSQLiteHumanQuorumPolicyRowsV2(t, store, 0, 0)
}

func TestSQLiteHumanQuorumPolicyV2S1S2DriftTerminalAndHistoricalIndependence(t *testing.T) {
	now := time.Unix(2_630_000_000, 0)
	clockNow := now
	store := openTestStore(t, testDBPath(t), func() time.Time { return clockNow })
	first := sqliteHumanQuorumPolicyV2(t, now, "tenant-s1s2", "legal/opinion", 1)
	if _, err := store.PublishHumanQuorumPolicyCurrentV2(context.Background(), ports.HumanQuorumPolicyCurrentPublishRequestV2{Value: first}); err != nil {
		t.Fatal(err)
	}
	s1, err := store.ResolveCurrentHumanQuorumPolicyV2(context.Background(), ports.HumanQuorumPolicyCurrentResolveRequestV2{Subject: first.Subject})
	if err != nil {
		t.Fatal(err)
	}
	clockNow = now.Add(time.Second)
	terminal := sqliteNextHumanQuorumPolicyV2(t, first, clockNow, ports.HumanQuorumPolicyProjectionRevokedV2)
	if _, err := store.PublishHumanQuorumPolicyCurrentV2(context.Background(), ports.HumanQuorumPolicyCurrentPublishRequestV2{Previous: &first.Ref, Value: terminal}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InspectCurrentHumanQuorumPolicyV2(context.Background(), first.Subject, s1); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("S2 accepted a drifted current index: %v", err)
	}
	if _, err := store.ResolveCurrentHumanQuorumPolicyV2(context.Background(), ports.HumanQuorumPolicyCurrentResolveRequestV2{Subject: first.Subject}); !core.HasReason(err, core.ReasonReviewVerdictStale) {
		t.Fatalf("terminal current policy was resolved as active: %v", err)
	}
	if old, err := store.InspectHistoricalHumanQuorumPolicyV2(context.Background(), first.Ref); err != nil || old.Ref != first.Ref {
		t.Fatalf("terminal current contaminated old history: %+v %v", old, err)
	}
	if exactTerminal, err := store.InspectHistoricalHumanQuorumPolicyV2(context.Background(), terminal.Ref); err != nil || exactTerminal.Ref != terminal.Ref {
		t.Fatalf("terminal history is not exact-readable: %+v %v", exactTerminal, err)
	}
	assertSQLiteHumanQuorumPolicyRowsV2(t, store, 2, 1)
}

func sqliteHumanQuorumPolicyV2(t *testing.T, checked time.Time, tenant core.TenantID, domain string, revision core.Revision) ports.HumanQuorumPolicyCurrentProjectionV2 {
	t.Helper()
	value, err := ports.SealHumanQuorumPolicyCurrentProjectionV2(ports.HumanQuorumPolicyCurrentProjectionV2{
		Ref:                         ports.HumanQuorumPolicyCurrentProjectionRefV2{Revision: revision},
		Subject:                     ports.HumanQuorumPolicyCurrentSubjectV2{TenantID: tenant, Domain: domain},
		State:                       ports.HumanQuorumPolicyProjectionActiveV2,
		Current:                     true,
		AcceptThreshold:             2,
		MaximumPanelSize:            3,
		RoleRequirements:            []ports.HumanQuorumRoleRequirementV2{{Role: "security", Minimum: 1}, {Role: "legal", Minimum: 1}},
		RejectVetoRoles:             []string{"security", "legal"},
		DelegationRequired:          true,
		ProductionSelfReviewAllowed: false,
		MaxPanelDurationNanos:       (24 * time.Hour).Nanoseconds(),
		MaxVoteTTLNanos:             time.Hour.Nanoseconds(),
		CheckedUnixNano:             checked.UnixNano(),
		ExpiresUnixNano:             checked.Add(30 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func sqliteNextHumanQuorumPolicyV2(t *testing.T, current ports.HumanQuorumPolicyCurrentProjectionV2, checked time.Time, state ports.HumanQuorumPolicyProjectionStateV2) ports.HumanQuorumPolicyCurrentProjectionV2 {
	t.Helper()
	next := current.Clone()
	next.Ref.Revision++
	next.State = state
	next.Current = state == ports.HumanQuorumPolicyProjectionActiveV2
	next.CheckedUnixNano = checked.UnixNano()
	next.ExpiresUnixNano = checked.Add(30 * time.Second).UnixNano()
	next, err := ports.SealHumanQuorumPolicyCurrentProjectionV2(next)
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func assertSQLiteHumanQuorumPolicyRowsV2(t *testing.T, store *Store, history, current int) {
	t.Helper()
	queries := map[string]string{
		"history": `SELECT COUNT(1) FROM runtime_review_governance_projection_history WHERE kind='` + reviewGovernanceProjectionHumanQuorumPolicyV2 + `'`,
		"current": `SELECT COUNT(1) FROM runtime_review_governance_projection_current WHERE kind='` + reviewGovernanceProjectionHumanQuorumPolicyV2 + `'`,
	}
	for name, query := range queries {
		var got int
		if err := store.db.QueryRow(query).Scan(&got); err != nil {
			t.Fatal(err)
		}
		want := history
		if name == "current" {
			want = current
		}
		if got != want {
			t.Fatalf("%s rows=%d want=%d", name, got, want)
		}
	}
}

func sequenceClockV2(values ...time.Time) func() time.Time {
	var mu sync.Mutex
	index := 0
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		if len(values) == 0 {
			return time.Time{}
		}
		if index >= len(values) {
			return values[len(values)-1]
		}
		value := values[index]
		index++
		return value
	}
}

func TestSQLiteHumanQuorumPolicyV2CompileShape(t *testing.T) {
	var _ ports.HumanQuorumPolicyCurrentReaderV2 = (*Store)(nil)
	var _ ports.HumanQuorumPolicyCurrentPublisherV2 = (*Store)(nil)
}

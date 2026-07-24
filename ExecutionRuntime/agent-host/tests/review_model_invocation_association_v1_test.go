package tests_test

import (
	"context"
	"database/sql"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/journal"
	hostports "github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
	hostsqlite "github.com/Proview-China/rax/ExecutionRuntime/agent-host/storage/sqlite"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestReviewModelInvocationAssociationMemoryAndSQLiteConformanceV1(t *testing.T) {
	now := time.Unix(1_900_400_000, 0)
	fixture := associationFixture(t, now)
	memory := journal.NewMemoryReviewModelInvocationAssociationStoreV1(func() time.Time { return now })
	report, err := conformance.RunReviewModelInvocationAssociationV1(context.Background(), memory, fixture)
	if err != nil || !report.DeepClone || report.ProductionEligible {
		t.Fatalf("memory report=%+v err=%v", report, err)
	}
	path := t.TempDir() + "/host.sqlite"
	store, err := hostsqlite.Open(context.Background(), hostsqlite.Config{Path: path, Owner: ownerRef("agent-host", "owner"), Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	report, err = conformance.RunReviewModelInvocationAssociationV1(context.Background(), store, fixture)
	if err != nil || !report.HistoricalExact || report.ProductionEligible {
		t.Fatalf("sqlite report=%+v err=%v", report, err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := hostsqlite.Open(context.Background(), hostsqlite.Config{Path: path, Owner: ownerRef("agent-host", "owner"), Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.InspectHistoricalReviewModelInvocationAssociationV1(context.Background(), fixture.Initial.RefV1())
	if err != nil || !reflect.DeepEqual(got, fixture.Initial) {
		t.Fatalf("restart history=%+v err=%v", got, err)
	}
}

func TestReviewModelInvocationAssociationLostReplyExactRecoveryV1(t *testing.T) {
	now := time.Unix(1_900_400_000, 0)
	fixture := associationFixture(t, now)
	store := journal.NewMemoryReviewModelInvocationAssociationStoreV1(func() time.Time { return now })
	store.LoseNextCreateReplyV1()
	if _, err := store.CreateReviewModelInvocationAssociationV1(context.Background(), fixture.Initial); !contract.HasCode(err, contract.ErrorUnknownOutcome) {
		t.Fatalf("lost create=%v", err)
	}
	got, err := store.InspectHistoricalReviewModelInvocationAssociationV1(context.Background(), fixture.Initial.RefV1())
	if err != nil || !reflect.DeepEqual(got, fixture.Initial) {
		t.Fatalf("create recovery=%+v %v", got, err)
	}
	store.LoseNextCASReplyV1()
	request := hostports.ReviewModelInvocationAssociationCASRequestV1{Expected: fixture.Initial.RefV1(), Next: fixture.Terminal}
	if _, err = store.CompareAndSwapReviewModelInvocationAssociationV1(context.Background(), request); !contract.HasCode(err, contract.ErrorUnknownOutcome) {
		t.Fatalf("lost CAS=%v", err)
	}
	got, err = store.InspectHistoricalReviewModelInvocationAssociationV1(context.Background(), fixture.Terminal.RefV1())
	if err != nil || !reflect.DeepEqual(got, fixture.Terminal) {
		t.Fatalf("CAS recovery=%+v %v", got, err)
	}
}

func TestReviewModelInvocationAssociationTTLClockABAV1(t *testing.T) {
	base := time.Unix(1_900_400_000, 0)
	now := base
	fixture := associationFixture(t, base)
	store := journal.NewMemoryReviewModelInvocationAssociationStoreV1(func() time.Time { return now })
	if _, err := store.CreateReviewModelInvocationAssociationV1(context.Background(), fixture.Initial); err != nil {
		t.Fatal(err)
	}
	ref, err := store.ResolveCurrentReviewModelInvocationAssociationV1(context.Background(), fixture.Initial.Subject)
	if err != nil {
		t.Fatal(err)
	}
	now = time.Unix(0, fixture.Initial.CheckedUnixNano-1)
	if _, err = store.InspectCurrentReviewModelInvocationAssociationV1(context.Background(), fixture.Initial.Subject, ref); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("rollback=%v", err)
	}
	now = time.Unix(0, fixture.Initial.ExpiresUnixNano)
	if _, err = store.InspectCurrentReviewModelInvocationAssociationV1(context.Background(), fixture.Initial.Subject, ref); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("expiry=%v", err)
	}
	now = base
	if _, err = store.CompareAndSwapReviewModelInvocationAssociationV1(context.Background(), hostports.ReviewModelInvocationAssociationCASRequestV1{Expected: ref, Next: fixture.Terminal}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.InspectCurrentReviewModelInvocationAssociationV1(context.Background(), fixture.Initial.Subject, ref); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("stale full Ref=%v", err)
	}
}

func TestReviewModelInvocationAssociationConcurrentCreateV1(t *testing.T) {
	now := time.Unix(1_900_400_000, 0)
	fixture := associationFixture(t, now)
	store := journal.NewMemoryReviewModelInvocationAssociationStoreV1(func() time.Time { return now })
	const n = 64
	var wg sync.WaitGroup
	var mu sync.Mutex
	created, failed := 0, 0
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			receipt, err := store.CreateReviewModelInvocationAssociationV1(context.Background(), fixture.Initial)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed++
			} else if receipt.Created {
				created++
			}
		}()
	}
	wg.Wait()
	if created != 1 || failed != 0 {
		t.Fatalf("created=%d failed=%d", created, failed)
	}
}

func TestReviewModelInvocationAssociationChangedPayloadAndReadTimeRollbackV1(t *testing.T) {
	base := time.Unix(1_900_400_000, 0)
	rollback := false
	reads := 0
	clock := func() time.Time {
		if !rollback {
			return base
		}
		reads++
		if reads == 1 {
			return base.Add(2 * time.Second)
		}
		return base.Add(time.Second)
	}
	fixture := associationFixture(t, base)
	store := journal.NewMemoryReviewModelInvocationAssociationStoreV1(clock)
	if _, err := store.CreateReviewModelInvocationAssociationV1(context.Background(), fixture.Initial); err != nil {
		t.Fatal(err)
	}
	changed := fixture.Initial
	changed.Command.DispatchSequence = 2
	changed.ID, changed.Digest, changed.CommandDigest = "", "", ""
	changed, err := contract.SealReviewModelInvocationAssociationV1(changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateReviewModelInvocationAssociationV1(context.Background(), changed); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("changed payload=%v", err)
	}
	if got, err := store.InspectHistoricalReviewModelInvocationAssociationV1(context.Background(), fixture.Initial.RefV1()); err != nil || !reflect.DeepEqual(got, fixture.Initial) {
		t.Fatalf("changed payload damaged history: %+v %v", got, err)
	}
	rollback = true
	reads = 0
	if _, err = store.ResolveCurrentReviewModelInvocationAssociationV1(context.Background(), fixture.Initial.Subject); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("read-time rollback=%v", err)
	}
}

func TestReviewModelInvocationAssociationCheckedAndModelTimeBoundsV1(t *testing.T) {
	base := time.Unix(1_900_400_000, 0)
	fixture := associationFixture(t, base)
	beforeModelCurrent := fixture.Initial
	beforeModelCurrent.CheckedUnixNano = beforeModelCurrent.Command.CurrentRef.CheckedUnixNano - 1
	beforeModelCurrent.ID, beforeModelCurrent.Digest, beforeModelCurrent.CommandDigest = "", "", ""
	if _, err := contract.SealReviewModelInvocationAssociationV1(beforeModelCurrent); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("Checked before Model current=%v", err)
	}

	for _, tc := range []struct {
		name             string
		currentExpires   time.Time
		modelNotAfter    time.Time
		associationUntil time.Time
	}{
		{name: "model_expires", currentExpires: base.Add(4 * time.Minute), modelNotAfter: base.Add(10 * time.Minute), associationUntil: base.Add(4 * time.Minute)},
		{name: "model_not_after", currentExpires: base.Add(4 * time.Minute), modelNotAfter: base.Add(4 * time.Minute), associationUntil: base.Add(4 * time.Minute)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bounded := associationFixtureWithBounds(t, base, tc.currentExpires, tc.modelNotAfter, tc.associationUntil)
			if bounded.Initial.ExpiresUnixNano != tc.associationUntil.UnixNano() {
				t.Fatalf("association expiry=%d want=%d", bounded.Initial.ExpiresUnixNano, tc.associationUntil.UnixNano())
			}
			outlives := bounded.Initial
			outlives.ExpiresUnixNano++
			outlives.ID, outlives.Digest, outlives.CommandDigest = "", "", ""
			if _, err := contract.SealReviewModelInvocationAssociationV1(outlives); !contract.HasCode(err, contract.ErrorConflict) {
				t.Fatalf("outlives %s=%v", tc.name, err)
			}
		})
	}
}

func TestReviewModelInvocationAssociationCreateActualPointFailClosedV1(t *testing.T) {
	base := time.Unix(1_900_400_000, 0)
	fixture := associationFixture(t, base)
	tests := []struct {
		name       string
		values     []time.Time
		sqliteOnly bool
	}{
		{name: "rollback", values: []time.Time{base.Add(time.Second), base}},
		{name: "ttl_crossing", values: []time.Time{base, time.Unix(0, fixture.Initial.ExpiresUnixNano)}},
		{name: "commit_ttl_crossing", values: []time.Time{base, base.Add(time.Second), time.Unix(0, fixture.Initial.ExpiresUnixNano)}, sqliteOnly: true},
	}
	for _, tc := range tests {
		for _, backend := range []string{"memory", "sqlite"} {
			if tc.sqliteOnly && backend != "sqlite" {
				continue
			}
			t.Run(backend+"/"+tc.name, func(t *testing.T) {
				clock := newControlledClockV1(base)
				var store hostports.ReviewModelInvocationAssociationPortV1
				var closeStore func()
				if backend == "memory" {
					store = journal.NewMemoryReviewModelInvocationAssociationStoreV1(clock.Now)
					closeStore = func() {}
				} else {
					sqliteStore, err := hostsqlite.Open(context.Background(), hostsqlite.Config{Path: t.TempDir() + "/host.sqlite", Owner: ownerRef("agent-host", "owner"), Clock: clock.Now})
					if err != nil {
						t.Fatal(err)
					}
					store = sqliteStore
					closeStore = func() { _ = sqliteStore.Close() }
				}
				defer closeStore()
				clock.Reset(tc.values...)
				if _, err := store.CreateReviewModelInvocationAssociationV1(context.Background(), fixture.Initial); !contract.HasCode(err, contract.ErrorPrecondition) {
					t.Fatalf("actual-point failure=%v", err)
				}
				if _, err := store.InspectHistoricalReviewModelInvocationAssociationV1(context.Background(), fixture.Initial.RefV1()); !contract.HasCode(err, contract.ErrorNotFound) {
					t.Fatalf("staged history leaked=%v", err)
				}
			})
		}
	}
}

func TestReviewModelInvocationAssociationIdempotentReplayActualPointFailClosedV1(t *testing.T) {
	base := time.Unix(1_900_400_000, 0)
	fixture := associationFixture(t, base)
	tests := []struct {
		name       string
		values     []time.Time
		sqliteOnly bool
	}{
		{name: "rollback", values: []time.Time{base.Add(time.Second), base}},
		{name: "ttl_crossing", values: []time.Time{base, time.Unix(0, fixture.Initial.ExpiresUnixNano)}},
		{name: "commit_ttl_crossing", values: []time.Time{base, base.Add(time.Second), time.Unix(0, fixture.Initial.ExpiresUnixNano)}, sqliteOnly: true},
	}
	for _, tc := range tests {
		for _, backend := range []string{"memory", "sqlite"} {
			if tc.sqliteOnly && backend != "sqlite" {
				continue
			}
			t.Run(backend+"/"+tc.name, func(t *testing.T) {
				clock := newControlledClockV1(base)
				var store hostports.ReviewModelInvocationAssociationPortV1
				var closeStore func()
				if backend == "memory" {
					store = journal.NewMemoryReviewModelInvocationAssociationStoreV1(clock.Now)
					closeStore = func() {}
				} else {
					sqliteStore, err := hostsqlite.Open(context.Background(), hostsqlite.Config{Path: t.TempDir() + "/host.sqlite", Owner: ownerRef("agent-host", "owner"), Clock: clock.Now})
					if err != nil {
						t.Fatal(err)
					}
					store = sqliteStore
					closeStore = func() { _ = sqliteStore.Close() }
				}
				defer closeStore()
				clock.Reset(base, base, base)
				if receipt, err := store.CreateReviewModelInvocationAssociationV1(context.Background(), fixture.Initial); err != nil || !receipt.Created {
					t.Fatalf("initial Create=%+v %v", receipt, err)
				}
				clock.Reset(tc.values...)
				if _, err := store.CreateReviewModelInvocationAssociationV1(context.Background(), fixture.Initial); !contract.HasCode(err, contract.ErrorPrecondition) {
					t.Fatalf("replay actual-point failure=%v", err)
				}
				got, err := store.InspectHistoricalReviewModelInvocationAssociationV1(context.Background(), fixture.Initial.RefV1())
				if err != nil || !reflect.DeepEqual(got, fixture.Initial) {
					t.Fatalf("replay failure altered history: %+v %v", got, err)
				}
			})
		}
	}
}

func TestReviewModelInvocationAssociationMemoryStagedFailureAndLateTerminalV1(t *testing.T) {
	base := time.Unix(1_900_400_000, 0)
	fixture := associationFixture(t, base)
	store := journal.NewMemoryReviewModelInvocationAssociationStoreV1(func() time.Time { return base })
	store.FailNextCreateBeforeCommitV1(contract.ErrorUnavailable)
	if _, err := store.CreateReviewModelInvocationAssociationV1(context.Background(), fixture.Initial); !contract.HasCode(err, contract.ErrorUnavailable) {
		t.Fatalf("staged create=%v", err)
	}
	if _, err := store.InspectHistoricalReviewModelInvocationAssociationV1(context.Background(), fixture.Initial.RefV1()); !contract.HasCode(err, contract.ErrorNotFound) {
		t.Fatalf("staged create leaked=%v", err)
	}
	if _, err := store.CreateReviewModelInvocationAssociationV1(context.Background(), fixture.Initial); err != nil {
		t.Fatal(err)
	}
	store.FailNextCASBeforeCommitV1(contract.ErrorUnavailable)
	request := hostports.ReviewModelInvocationAssociationCASRequestV1{Expected: fixture.Initial.RefV1(), Next: fixture.Terminal}
	if _, err := store.CompareAndSwapReviewModelInvocationAssociationV1(context.Background(), request); !contract.HasCode(err, contract.ErrorUnavailable) {
		t.Fatalf("staged CAS=%v", err)
	}
	if _, err := store.InspectHistoricalReviewModelInvocationAssociationV1(context.Background(), fixture.Terminal.RefV1()); !contract.HasCode(err, contract.ErrorNotFound) {
		t.Fatalf("staged CAS leaked=%v", err)
	}
	late := time.Unix(0, fixture.Initial.ExpiresUnixNano).Add(time.Hour)
	// Seed through a live clock, then prove terminal truth is recordable after expiry.
	clock := newControlledClockV1(base)
	liveStore := journal.NewMemoryReviewModelInvocationAssociationStoreV1(clock.Now)
	if _, err := liveStore.CreateReviewModelInvocationAssociationV1(context.Background(), fixture.Initial); err != nil {
		t.Fatal(err)
	}
	clock.Reset(late)
	if _, err := liveStore.CompareAndSwapReviewModelInvocationAssociationV1(context.Background(), request); err != nil {
		t.Fatalf("truthful terminal CAS after active TTL=%v", err)
	}
}

func TestReviewModelInvocationAssociationTerminalCASTruthfulAfterActiveTTLV1(t *testing.T) {
	base := time.Unix(1_900_400_000, 0)
	fixture := associationFixture(t, base)
	request := hostports.ReviewModelInvocationAssociationCASRequestV1{Expected: fixture.Initial.RefV1(), Next: fixture.Terminal}
	late := time.Unix(0, fixture.Initial.ExpiresUnixNano).Add(time.Hour)
	for _, backend := range []string{"memory", "sqlite"} {
		t.Run(backend, func(t *testing.T) {
			clock := newControlledClockV1(base)
			var store hostports.ReviewModelInvocationAssociationPortV1
			var closeStore func()
			if backend == "memory" {
				store = journal.NewMemoryReviewModelInvocationAssociationStoreV1(clock.Now)
				closeStore = func() {}
			} else {
				sqliteStore, err := hostsqlite.Open(context.Background(), hostsqlite.Config{Path: t.TempDir() + "/host.sqlite", Owner: ownerRef("agent-host", "owner"), Clock: clock.Now})
				if err != nil {
					t.Fatal(err)
				}
				store = sqliteStore
				closeStore = func() { _ = sqliteStore.Close() }
			}
			defer closeStore()
			clock.Reset(base, base)
			if _, err := store.CreateReviewModelInvocationAssociationV1(context.Background(), fixture.Initial); err != nil {
				t.Fatal(err)
			}
			clock.Reset(late)
			receipt, err := store.CompareAndSwapReviewModelInvocationAssociationV1(context.Background(), request)
			if err != nil || !receipt.Applied || !reflect.DeepEqual(receipt.Fact, fixture.Terminal) {
				t.Fatalf("truthful terminal after TTL=%+v %v", receipt, err)
			}
		})
	}
}

func TestReviewModelInvocationAssociationSQLiteConcurrentAndCorruptRowsV1(t *testing.T) {
	base := time.Unix(1_900_400_000, 0)
	fixture := associationFixture(t, base)
	path := t.TempDir() + "/host.sqlite"
	store, err := hostsqlite.Open(context.Background(), hostsqlite.Config{Path: path, Owner: ownerRef("agent-host", "owner"), Clock: func() time.Time { return base }})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	const n = 64
	var wg sync.WaitGroup
	var mu sync.Mutex
	created, failed := 0, 0
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			receipt, createErr := store.CreateReviewModelInvocationAssociationV1(context.Background(), fixture.Initial)
			mu.Lock()
			defer mu.Unlock()
			if createErr != nil {
				failed++
			} else if receipt.Created {
				created++
			}
		}()
	}
	wg.Wait()
	if created != 1 || failed != 0 {
		t.Fatalf("SQLite concurrent Create created=%d failed=%d", created, failed)
	}

	request := hostports.ReviewModelInvocationAssociationCASRequestV1{Expected: fixture.Initial.RefV1(), Next: fixture.Terminal}
	applied, failed := 0, 0
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			receipt, casErr := store.CompareAndSwapReviewModelInvocationAssociationV1(context.Background(), request)
			mu.Lock()
			defer mu.Unlock()
			if casErr != nil {
				failed++
			} else if receipt.Applied {
				applied++
			}
		}()
	}
	wg.Wait()
	if applied != 1 || failed != 0 {
		t.Fatalf("SQLite concurrent CAS applied=%d failed=%d", applied, failed)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rogueDigest := string(core.DigestBytes([]byte("rogue-higher-history")))
	if _, err = db.Exec(`INSERT INTO agent_host_review_model_association_history_v1(id,revision,digest,previous_digest,checked_unix_nano,expires_unix_nano,row_digest,canonical_json) SELECT id,revision+1,?,digest,checked_unix_nano,expires_unix_nano,row_digest,canonical_json FROM agent_host_review_model_association_history_v1 WHERE id=? AND revision=?`, rogueDigest, fixture.Terminal.ID, uint64(fixture.Terminal.Revision)); err != nil {
		t.Fatal(err)
	}
	if _, err = store.ResolveCurrentReviewModelInvocationAssociationV1(context.Background(), fixture.Initial.Subject); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("regressed current index=%v", err)
	}
	if _, err = db.Exec(`DELETE FROM agent_host_review_model_association_history_v1 WHERE id=? AND revision=? AND digest=?`, fixture.Terminal.ID, uint64(fixture.Terminal.Revision+1), rogueDigest); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE agent_host_review_model_association_history_v1 SET canonical_json=? WHERE id=? AND revision=?`, []byte(`{}`), fixture.Terminal.ID, uint64(fixture.Terminal.Revision)); err != nil {
		t.Fatal(err)
	}
	if _, err = store.ResolveCurrentReviewModelInvocationAssociationV1(context.Background(), fixture.Initial.Subject); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("corrupt current payload=%v", err)
	}
}

func associationFixture(t *testing.T, now time.Time) conformance.ReviewModelInvocationAssociationFixtureV1 {
	return associationFixtureWithBounds(t, now, now.Add(8*time.Minute), now.Add(10*time.Minute), now.Add(5*time.Minute))
}

func associationFixtureWithBounds(t *testing.T, now, currentExpires, modelNotAfter, associationExpires time.Time) conformance.ReviewModelInvocationAssociationFixtureV1 {
	t.Helper()
	digest := func(v string) core.Digest { return core.DigestBytes([]byte(v)) }
	requestDigest := digest("review-command")
	prepared, err := modelinvoker.SealPreparedModelInvocationFactV1(modelinvoker.PreparedModelInvocationFactV1{InvocationID: "review-invocation", InvocationDigest: requestDigest, UnifiedRequestDigest: requestDigest, RequestToolsDigest: digest("tools"), PreparedPlanDigest: digest("plan"), RouteDigest: digest("route"), ProfileDigest: digest("profile"), ActualToolSurfaceDigest: digest("surface"), ActualProviderInjectionDigest: digest("provider"), CapabilitySnapshotRef: modelinvoker.PreparedModelInvocationCapabilitySnapshotRefV1{ContractVersion: "1.0.0", ID: "capability", Revision: 1, Digest: digest("capability")}, RegistrySnapshotRef: runtimeports.RegistrySnapshotRefV1{Owner: ownerRef("registry", "owner"), ContractVersion: "1.0.0", ID: "registry", Revision: 1, Digest: digest("registry")}, CreatedUnixNano: now.Add(-time.Minute).UnixNano(), NotAfterUnixNano: modelNotAfter.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	current, err := modelinvoker.SealPreparedModelInvocationCurrentV1(modelinvoker.PreparedModelInvocationCurrentProjectionV1{Prepared: prepared.Ref(), CapabilitySnapshotRef: prepared.CapabilitySnapshotRef, RegistrySnapshotRef: prepared.RegistrySnapshotRef, ActualToolSurfaceDigest: prepared.ActualToolSurfaceDigest, ActualProviderInjectionDigest: prepared.ActualProviderInjectionDigest, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: currentExpires.UnixNano(), NotAfterUnixNano: prepared.NotAfterUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	strict := true
	call := modelinvoker.RouteCall{RouteID: "openai.direct.payg.responses", Invocation: upstream.InvocationContext{Explicit: true, Usage: upstream.InvocationGeneralAPI, Subject: upstream.SubjectService, Tenancy: upstream.TenancyMulti, Execution: upstream.ExecutionForeground}, Request: modelinvoker.Request{Model: "gpt-5.5", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "review")}, ToolChoice: modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceNone}, Output: modelinvoker.OutputConstraint{Type: modelinvoker.OutputJSONSchema, Name: "review", Schema: []byte(`{"type":"object"}`), Strict: &strict}}}
	subject := contract.ReviewModelInvocationAssociationSubjectV1{TenantID: "tenant-a", ReviewAttempt: contract.ReviewAttemptExactCoordinateV1{ID: "attempt-a", Revision: 1, Digest: digest("attempt")}}
	initial, err := contract.SealReviewModelInvocationAssociationV1(contract.ReviewModelInvocationAssociationFactV1{Subject: subject, Command: modelinvoker.GovernedModelInvocationCommandV1{PreparedRef: prepared.Ref(), CurrentRef: current.Ref(), AttemptRequestDigest: requestDigest, DispatchSequence: 1, ProviderAttemptOrdinal: 1, Call: call}, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: associationExpires.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := contract.SealReviewModelInvocationAssociationSuccessorV1(initial, contract.ReviewModelInvocationAssociationFactV1{State: contract.ReviewModelInvocationAssociationRevokedV1, CheckedUnixNano: now.Add(time.Second).UnixNano(), ExpiresUnixNano: initial.ExpiresUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	return conformance.ReviewModelInvocationAssociationFixtureV1{Initial: initial, Terminal: terminal}
}

type controlledClockV1 struct {
	mu     sync.Mutex
	values []time.Time
	index  int
}

func newControlledClockV1(initial time.Time) *controlledClockV1 {
	return &controlledClockV1{values: []time.Time{initial}}
}
func (c *controlledClockV1) Reset(values ...time.Time) {
	c.mu.Lock()
	c.values = append([]time.Time(nil), values...)
	c.index = 0
	c.mu.Unlock()
}
func (c *controlledClockV1) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.values) == 0 {
		return time.Time{}
	}
	if c.index >= len(c.values) {
		return c.values[len(c.values)-1]
	}
	value := c.values[c.index]
	c.index++
	return value
}
func ownerRef(domain, id string) core.OwnerRef {
	return core.OwnerRef{Domain: domain, ID: core.OwnerID(id)}
}

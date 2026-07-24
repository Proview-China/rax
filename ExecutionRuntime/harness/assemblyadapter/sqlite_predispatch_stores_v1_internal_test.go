package assemblyadapter

import (
	"context"
	"reflect"
	"testing"
	"time"

	assemblytestkit "github.com/Proview-China/rax/ExecutionRuntime/harness/tests/assembly/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestSQLiteModelPreDispatchAssemblyCurrentStoreRestartCASAndLostReplyV1(t *testing.T) {
	ctx := context.Background()
	firstFixture := newModelPreDispatchFixtureV1(t, 1)
	now := firstFixture.now
	config := SQLiteModelPreDispatchStoreConfigV1{Path: t.TempDir() + "/predispatch.db", Clock: func() time.Time { return now }}
	store, err := OpenSQLiteModelPreDispatchAssemblyCurrentStoreV1(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	publisher, _, _, _, _, _ := newModelPreDispatchPublisherV1(t, firstFixture, store, config.Clock)
	store.LoseNextReplyForTestingV1()
	if _, err = publisher.PublishModelPreDispatchAssemblyCurrentV1(ctx, firstFixture.request); err != nil {
		t.Fatalf("publisher did not recover exact lost reply: %v", err)
	}
	first, err := store.InspectCurrentModelPreDispatchAssemblyV1(ctx, mustSQLiteAssemblyCurrentRefV1(t, store))
	if err != nil {
		t.Fatal(err)
	}
	now = time.Unix(0, first.ExpiresUnixNano)
	if _, err := store.InspectCurrentModelPreDispatchAssemblyV1(ctx, first.Ref); !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("expired M2 current = %v", err)
	}
	now = firstFixture.now
	if err := store.IntegrityCheckV1(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenSQLiteModelPreDispatchAssemblyCurrentStoreV1(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	recovered, err := reopened.InspectCurrentModelPreDispatchAssemblyV1(ctx, first.Ref)
	if err != nil || recovered != first {
		t.Fatalf("restart current = %#v, %v", recovered, err)
	}
	secondFixture := newModelPreDispatchFixtureV1(t, 2)
	now = secondFixture.now
	secondFixture.request.ExpectedCurrent = first.Ref
	secondPublisher, _, _, _, _, _ := newModelPreDispatchPublisherV1(t, secondFixture, reopened, config.Clock)
	second, err := secondPublisher.PublishModelPreDispatchAssemblyCurrentV1(ctx, secondFixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if historical, err := reopened.InspectHistoricalModelPreDispatchAssemblyV1(ctx, first.Ref); err != nil || historical != first {
		t.Fatalf("historical = %#v, %v", historical, err)
	}
	if _, err := reopened.InspectCurrentModelPreDispatchAssemblyV1(ctx, first.Ref); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("old current = %v", err)
	}
	if _, err := reopened.CompareAndSwapModelPreDispatchAssemblyCurrentV1(ctx, second.Ref, first); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("ABA = %v", err)
	}
}

func mustSQLiteAssemblyCurrentRefV1(t *testing.T, store *SQLiteModelPreDispatchAssemblyCurrentStoreV1) runtimeports.ModelPreDispatchAssemblyCurrentRefV1 {
	t.Helper()
	var id string
	if err := store.state.db.QueryRow(`SELECT id FROM harness_model_predispatch_assembly_current_v1 LIMIT 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	var revision int64
	if err := store.state.db.QueryRow(`SELECT revision FROM harness_model_predispatch_assembly_current_v1 WHERE id=?`, id).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	stored, exists, err := inspectSQLiteAssemblyHistoryDBV1(context.Background(), store.state.db, id, core.Revision(revision))
	if err != nil || !exists {
		t.Fatalf("stored = %#v, %v", stored, err)
	}
	return stored.Ref
}

func TestSQLiteModelPreDispatchVerifiedStoreDeepCloneRestartAndConflictV1(t *testing.T) {
	ctx := context.Background()
	now := assemblytestkit.Now
	config := SQLiteModelPreDispatchStoreConfigV1{Path: t.TempDir() + "/verified.db", Clock: func() time.Time { return now }}
	store, err := OpenSQLiteModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	first := modelPreDispatchVerifiedAssemblyProjectionV1(t, now, 1)
	store.LoseNextReplyForTestingV1()
	if _, err := store.EnsureModelPreDispatchVerifiedAssemblyOwnerCurrentV1(ctx, first); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost reply = %v", err)
	}
	recovered, err := store.InspectCurrentModelPreDispatchVerifiedAssemblyOwnerV1(ctx, first.Ref)
	if err != nil || !reflect.DeepEqual(recovered, first) {
		t.Fatalf("recovered = %#v, %v", recovered, err)
	}
	recovered.Compile.Manifest.CurrentFacts[0].ID = "mutated"
	again, err := store.InspectCurrentModelPreDispatchVerifiedAssemblyOwnerV1(ctx, first.Ref)
	if err != nil || again.Compile.Manifest.CurrentFacts[0].ID == "mutated" {
		t.Fatalf("SQLite Store aliased returned projection: %v", err)
	}
	now = time.Unix(0, first.ExpiresUnixNano)
	if _, err := store.InspectCurrentModelPreDispatchVerifiedAssemblyOwnerV1(ctx, first.Ref); !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("expired A2 current = %v", err)
	}
	now = assemblytestkit.Now
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSQLiteModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got, err := reopened.InspectCurrentModelPreDispatchVerifiedAssemblyOwnerV1(ctx, first.Ref); err != nil || !reflect.DeepEqual(got, first) {
		t.Fatalf("restart = %#v, %v", got, err)
	}
	drift := cloneVerifiedAssemblyForTestV1(t, first)
	drift.Compile.Diagnostics = append(drift.Compile.Diagnostics, drift.Compile.Diagnostics...)
	drift.CompileDigest, _ = ModelPreDispatchVerifiedAssemblyOwnerCurrentCompileDigestV1(drift.Compile)
	drift.Ref.Digest = ""
	drift.ProjectionDigest = ""
	drift.ProjectionDigest, _ = ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionDigestV1(drift)
	drift.Ref.Digest = drift.ProjectionDigest
	if _, err := reopened.EnsureModelPreDispatchVerifiedAssemblyOwnerCurrentV1(ctx, drift); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same revision drift = %v", err)
	}
	if err := reopened.IntegrityCheckV1(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteModelPreDispatchStoresRejectCanceledAndCorruptRowsV1(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := OpenSQLiteModelPreDispatchAssemblyCurrentStoreV1(ctx, SQLiteModelPreDispatchStoreConfigV1{Path: t.TempDir() + "/canceled.db"}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("canceled open = %v", err)
	}
	now := assemblytestkit.Now
	config := SQLiteModelPreDispatchStoreConfigV1{Path: t.TempDir() + "/corrupt.db", Clock: func() time.Time { return now }}
	store, err := OpenSQLiteModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	value := modelPreDispatchVerifiedAssemblyProjectionV1(t, now, 1)
	if _, err := store.EnsureModelPreDispatchVerifiedAssemblyOwnerCurrentV1(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	if _, err := store.state.db.Exec(`UPDATE harness_model_predispatch_verified_history_v1 SET row_digest='sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' WHERE id=?`, value.Ref.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InspectHistoricalModelPreDispatchVerifiedAssemblyOwnerV1(context.Background(), value.Ref); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("corrupt row = %v", err)
	}
}

func TestSQLiteModelPreDispatchStoresRejectIndexDriftAndPostS2CancellationV1(t *testing.T) {
	t.Run("M2", func(t *testing.T) {
		ctx := context.Background()
		fixture := newModelPreDispatchFixtureV1(t, 1)
		now := fixture.now
		store, err := OpenSQLiteModelPreDispatchAssemblyCurrentStoreV1(ctx, SQLiteModelPreDispatchStoreConfigV1{Path: t.TempDir() + "/m2.db", Clock: func() time.Time { return now }})
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		publisher, _, _, _, _, _ := newModelPreDispatchPublisherV1(t, fixture, store, func() time.Time { return now })
		value, err := publisher.PublishModelPreDispatchAssemblyCurrentV1(ctx, fixture.request)
		if err != nil {
			t.Fatal(err)
		}
		readCtx, cancel := context.WithCancel(context.Background())
		calls := 0
		store.state.clock = func() time.Time {
			calls++
			if calls == 2 {
				cancel()
			}
			return now
		}
		if _, err := store.InspectCurrentModelPreDispatchAssemblyV1(readCtx, value.Ref); !core.HasCategory(err, core.ErrorUnavailable) {
			t.Fatalf("post-S2 cancellation = %v", err)
		}
		store.state.clock = func() time.Time { return now }
		if _, err := store.state.db.Exec(`UPDATE harness_model_predispatch_assembly_history_v1 SET watermark_digest=? WHERE id=? AND revision=?`, string(assemblytestkit.Digest("tampered-watermark")), value.Ref.ID, value.Ref.Revision); err != nil {
			t.Fatal(err)
		}
		if _, err := store.InspectHistoricalModelPreDispatchAssemblyV1(ctx, value.Ref); !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("watermark index drift = %v", err)
		}
	})

	t.Run("A2", func(t *testing.T) {
		ctx := context.Background()
		now := assemblytestkit.Now
		store, err := OpenSQLiteModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1(ctx, SQLiteModelPreDispatchStoreConfigV1{Path: t.TempDir() + "/a2.db", Clock: func() time.Time { return now }})
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		value := modelPreDispatchVerifiedAssemblyProjectionV1(t, now, 1)
		if _, err := store.EnsureModelPreDispatchVerifiedAssemblyOwnerCurrentV1(ctx, value); err != nil {
			t.Fatal(err)
		}
		readCtx, cancel := context.WithCancel(context.Background())
		calls := 0
		store.state.clock = func() time.Time {
			calls++
			if calls == 2 {
				cancel()
			}
			return now
		}
		if _, err := store.InspectCurrentModelPreDispatchVerifiedAssemblyOwnerV1(readCtx, value.Ref); !core.HasCategory(err, core.ErrorUnavailable) {
			t.Fatalf("post-S2 cancellation = %v", err)
		}
		store.state.clock = func() time.Time { return now }
		if _, err := store.state.db.Exec(`UPDATE harness_model_predispatch_verified_history_v1 SET ref_digest=? WHERE id=? AND revision=?`, string(assemblytestkit.Digest("tampered-ref")), value.Ref.ID, value.Ref.Revision); err != nil {
			t.Fatal(err)
		}
		if _, err := store.InspectHistoricalModelPreDispatchVerifiedAssemblyOwnerV1(ctx, value.Ref); !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("ref index drift = %v", err)
		}
	})
}

func TestSQLiteModelPreDispatchStoresRecheckTTLImmediatelyBeforeCommitV1(t *testing.T) {
	t.Run("M2", func(t *testing.T) {
		ctx := context.Background()
		fixture := newModelPreDispatchFixtureV1(t, 1)
		now := fixture.now
		memory, _ := NewInMemoryModelPreDispatchAssemblyCurrentStoreV1(func() time.Time { return now })
		publisher, _, _, _, _, _ := newModelPreDispatchPublisherV1(t, fixture, memory, func() time.Time { return now })
		next, err := publisher.PublishModelPreDispatchAssemblyCurrentV1(ctx, fixture.request)
		if err != nil {
			t.Fatal(err)
		}
		store, err := OpenSQLiteModelPreDispatchAssemblyCurrentStoreV1(ctx, SQLiteModelPreDispatchStoreConfigV1{Path: t.TempDir() + "/m2-ttl.db", Clock: func() time.Time { return now }})
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		calls := 0
		store.state.clock = func() time.Time {
			calls++
			if calls >= 3 {
				return time.Unix(0, next.ExpiresUnixNano)
			}
			return now
		}
		if _, err := store.CompareAndSwapModelPreDispatchAssemblyCurrentV1(ctx, runtimeports.ModelPreDispatchAssemblyCurrentRefV1{}, next); !core.HasCategory(err, core.ErrorIndeterminate) || !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
			t.Fatalf("post-mutation TTL crossing = %v", err)
		}
		var history, current int
		_ = store.state.db.QueryRow(`SELECT COUNT(*) FROM harness_model_predispatch_assembly_history_v1`).Scan(&history)
		_ = store.state.db.QueryRow(`SELECT COUNT(*) FROM harness_model_predispatch_assembly_current_v1`).Scan(&current)
		if history != 0 || current != 0 {
			t.Fatalf("rolled-back TTL crossing persisted %d/%d rows", history, current)
		}
	})

	t.Run("A2", func(t *testing.T) {
		ctx := context.Background()
		now := assemblytestkit.Now
		next := modelPreDispatchVerifiedAssemblyProjectionV1(t, now, 1)
		store, err := OpenSQLiteModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1(ctx, SQLiteModelPreDispatchStoreConfigV1{Path: t.TempDir() + "/a2-ttl.db", Clock: func() time.Time { return now }})
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		calls := 0
		store.state.clock = func() time.Time {
			calls++
			if calls >= 3 {
				return time.Unix(0, next.ExpiresUnixNano)
			}
			return now
		}
		if _, err := store.EnsureModelPreDispatchVerifiedAssemblyOwnerCurrentV1(ctx, next); !core.HasCategory(err, core.ErrorIndeterminate) || !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
			t.Fatalf("post-mutation TTL crossing = %v", err)
		}
		var history, current int
		_ = store.state.db.QueryRow(`SELECT COUNT(*) FROM harness_model_predispatch_verified_history_v1`).Scan(&history)
		_ = store.state.db.QueryRow(`SELECT COUNT(*) FROM harness_model_predispatch_verified_current_v1`).Scan(&current)
		if history != 0 || current != 0 {
			t.Fatalf("rolled-back TTL crossing persisted %d/%d rows", history, current)
		}
	})
}

func TestSQLiteModelPreDispatchStoresRecheckTTLOnIdempotentReplayV1(t *testing.T) {
	t.Run("M2", func(t *testing.T) {
		ctx := context.Background()
		fixture := newModelPreDispatchFixtureV1(t, 1)
		now := fixture.now
		store, err := OpenSQLiteModelPreDispatchAssemblyCurrentStoreV1(ctx, SQLiteModelPreDispatchStoreConfigV1{Path: t.TempDir() + "/m2-replay.db", Clock: func() time.Time { return now }})
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		publisher, _, _, _, _, _ := newModelPreDispatchPublisherV1(t, fixture, store, func() time.Time { return now })
		value, err := publisher.PublishModelPreDispatchAssemblyCurrentV1(ctx, fixture.request)
		if err != nil {
			t.Fatal(err)
		}
		calls := 0
		store.state.clock = func() time.Time {
			calls++
			if calls >= 2 {
				return time.Unix(0, value.ExpiresUnixNano)
			}
			return now
		}
		if _, err := store.CompareAndSwapModelPreDispatchAssemblyCurrentV1(ctx, runtimeports.ModelPreDispatchAssemblyCurrentRefV1{}, value); !core.HasReason(err, core.ReasonBindingExpired) {
			t.Fatalf("expired idempotent replay = %v", err)
		}
	})

	t.Run("A2", func(t *testing.T) {
		ctx := context.Background()
		now := assemblytestkit.Now
		value := modelPreDispatchVerifiedAssemblyProjectionV1(t, now, 1)
		store, err := OpenSQLiteModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1(ctx, SQLiteModelPreDispatchStoreConfigV1{Path: t.TempDir() + "/a2-replay.db", Clock: func() time.Time { return now }})
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		if _, err := store.EnsureModelPreDispatchVerifiedAssemblyOwnerCurrentV1(ctx, value); err != nil {
			t.Fatal(err)
		}
		calls := 0
		store.state.clock = func() time.Time {
			calls++
			if calls >= 2 {
				return time.Unix(0, value.ExpiresUnixNano)
			}
			return now
		}
		if _, err := store.EnsureModelPreDispatchVerifiedAssemblyOwnerCurrentV1(ctx, value); !core.HasReason(err, core.ReasonBindingExpired) {
			t.Fatalf("expired idempotent replay = %v", err)
		}
	})
}

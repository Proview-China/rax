package blackbox_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/framestore"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	_ "modernc.org/sqlite"
)

func TestFrameStoreExactCommitReadRestartAndLostReplyV1(t *testing.T) {
	fixture, err := testfixture.NewFrameStoreFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/frame.db"
	store, err := framestore.OpenSQLiteV1(context.Background(), framestore.SQLiteConfigV1{
		Path: path, Owner: fixture.Owner, Clock: func() time.Time { return fixture.Now },
	})
	if err != nil {
		t.Fatal(err)
	}
	store.LoseNextReplyForTestingV1()
	if _, err := store.CommitCurrentV1(context.Background(), "commit-frame-1", fixture.State, nil, fixture.Now.UnixNano()); err == nil {
		t.Fatal("lost reply must not report success")
	}
	receipt, err := store.InspectCommitV1(context.Background(), "commit-frame-1")
	if err != nil {
		t.Fatal(err)
	}
	if receipt.FrameRef.ID != fixture.State.Frame.ID || !receipt.Created {
		t.Fatalf("unexpected receipt: %+v", receipt)
	}
	projection, err := store.InspectContextFrameExactCurrentV1(context.Background(), receipt.FrameRef, fixture.Now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	if projection.FrameRef != receipt.FrameRef || !projection.Current {
		t.Fatalf("unexpected projection: %+v", projection)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = framestore.OpenSQLiteV1(context.Background(), framestore.SQLiteConfigV1{
		Path: path, Owner: fixture.Owner, Clock: func() time.Time { return fixture.Now },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.InspectCommitV1(context.Background(), "commit-frame-1"); err != nil {
		t.Fatal(err)
	}
}

func TestFrameStoreAdvanceSupersedesHistoryAndReplaysInspectOnlyV1(t *testing.T) {
	fixture, next, store, path := openCommittedFrameStoreV1(t)
	firstFrame, err := fixture.State.Frame.DigestValue()
	if err != nil {
		t.Fatal(err)
	}
	firstRef := contract.FactRef{ID: fixture.State.Frame.ID, Revision: fixture.State.Frame.Revision, Digest: firstFrame}
	receipt, err := store.CommitCurrentV1(context.Background(), "commit-frame-2", next.State, &fixture.State.Pointer, fixture.Now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.InspectContextFrameExactCurrentV1(context.Background(), firstRef, fixture.Now.UnixNano()); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("superseded frame must fail closed: %v", err)
	}
	request := contract.ContextGenerationCurrentPointerRequestV1{
		ExecutionScopeDigest: next.State.Pointer.ExecutionScopeDigest,
		RunID:                next.State.Pointer.RunID, SessionRef: next.State.Pointer.SessionRef, Turn: next.State.Pointer.Turn,
	}
	current, err := store.InspectCurrentGenerationPointer(context.Background(), request)
	if err != nil || current != next.State.Pointer {
		t.Fatalf("current pointer drift: %+v %v", current, err)
	}
	if _, err := store.CommitCurrentV1(context.Background(), "commit-frame-2", next.State, &fixture.State.Pointer, fixture.Now.UnixNano()); !errors.Is(err, contract.ErrInspectOnly) {
		t.Fatalf("same operation must be inspect-only: %v", err)
	}
	other, err := testfixture.AdvanceFrameStoreFixtureV1(fixture, "other")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitCurrentV1(context.Background(), "commit-frame-2", other.State, &fixture.State.Pointer, fixture.Now.UnixNano()); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("same operation with different state must conflict: %v", err)
	}
	inspected, err := store.InspectCommitV1(context.Background(), "commit-frame-2")
	if err != nil || inspected.FrameRef != receipt.FrameRef {
		t.Fatalf("inspect returned wrong committed result: %+v %v", inspected, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = framestore.OpenSQLiteV1(context.Background(), framestore.SQLiteConfigV1{
		Path: path, Owner: fixture.Owner, Clock: func() time.Time { return fixture.Now },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.InspectCommitV1(context.Background(), "commit-frame-2"); err != nil {
		t.Fatalf("restart could not recover full predecessor coordinate: %v", err)
	}
}

func TestFrameStoreCommitClockAndCancelFailClosedV1(t *testing.T) {
	fixture, err := testfixture.NewFrameStoreFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		clock    func() time.Time
		observed int64
		want     error
	}{
		{"now_equals_expiry", func() time.Time { return time.Unix(0, fixture.State.Pointer.ExpiresUnixNano) }, fixture.Now.UnixNano(), contract.ErrExpired},
		{"clock_rollback", func() time.Time { return fixture.Now.Add(-time.Second) }, fixture.Now.UnixNano(), contract.ErrConflict},
		{"future_created", func() time.Time { return fixture.Now.Add(-2 * time.Second) }, fixture.Now.Add(-3 * time.Second).UnixNano(), contract.ErrConflict},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, openErr := framestore.OpenSQLiteV1(context.Background(), framestore.SQLiteConfigV1{
				Path: t.TempDir() + "/frame.db", Owner: fixture.Owner, Clock: test.clock,
			})
			if openErr != nil {
				t.Fatal(openErr)
			}
			defer store.Close()
			if _, commitErr := store.CommitCurrentV1(context.Background(), "commit-clock", fixture.State, nil, test.observed); !errors.Is(commitErr, test.want) {
				t.Fatalf("got %v want %v", commitErr, test.want)
			}
			if _, inspectErr := store.InspectCommitV1(context.Background(), "commit-clock"); !errors.Is(inspectErr, contract.ErrNotFound) {
				t.Fatalf("failed commit became visible: %v", inspectErr)
			}
		})
	}
	t.Run("cross_ttl", func(t *testing.T) {
		var calls atomic.Int32
		clock := func() time.Time {
			if calls.Add(1) == 1 {
				return fixture.Now
			}
			return time.Unix(0, fixture.State.Pointer.ExpiresUnixNano)
		}
		store, openErr := framestore.OpenSQLiteV1(context.Background(), framestore.SQLiteConfigV1{
			Path: t.TempDir() + "/frame.db", Owner: fixture.Owner, Clock: clock,
		})
		if openErr != nil {
			t.Fatal(openErr)
		}
		defer store.Close()
		if _, commitErr := store.CommitCurrentV1(context.Background(), "commit-cross-ttl", fixture.State, nil, fixture.Now.UnixNano()); !errors.Is(commitErr, contract.ErrExpired) {
			t.Fatalf("TTL crossing must fail: %v", commitErr)
		}
		if _, inspectErr := store.InspectCommitV1(context.Background(), "commit-cross-ttl"); !errors.Is(inspectErr, contract.ErrNotFound) {
			t.Fatalf("TTL crossing wrote ledger: %v", inspectErr)
		}
	})
	t.Run("cancel_and_typed_nil", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		store, openErr := framestore.OpenSQLiteV1(context.Background(), framestore.SQLiteConfigV1{
			Path: t.TempDir() + "/frame.db", Owner: fixture.Owner, Clock: func() time.Time { return fixture.Now },
		})
		if openErr != nil {
			t.Fatal(openErr)
		}
		defer store.Close()
		if _, commitErr := store.CommitCurrentV1(ctx, "commit-canceled", fixture.State, nil, fixture.Now.UnixNano()); !errors.Is(commitErr, context.Canceled) {
			t.Fatalf("cancel classification drifted: %v", commitErr)
		}
		var nilStore *framestore.SQLiteV1
		if _, readErr := nilStore.InspectContextFrameExactCurrentV1(context.Background(), contract.FactRef{}, fixture.Now.UnixNano()); !errors.Is(readErr, contract.ErrUnavailable) {
			t.Fatalf("typed nil classification drifted: %v", readErr)
		}
	})
}

func TestFrameStoreCASAllowsExactlyOneOf64WritersV1(t *testing.T) {
	fixture, next, store, _ := openCommittedFrameStoreV1(t)
	defer store.Close()
	var success atomic.Int32
	var conflicts atomic.Int32
	var wait sync.WaitGroup
	for index := 0; index < 64; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, err := store.CommitCurrentV1(context.Background(), fmt.Sprintf("commit-race-%02d", index), next.State, &fixture.State.Pointer, fixture.Now.UnixNano())
			switch {
			case err == nil:
				success.Add(1)
			case errors.Is(err, contract.ErrConflict):
				conflicts.Add(1)
			default:
				t.Errorf("unexpected concurrent result: %v", err)
			}
		}(index)
	}
	wait.Wait()
	if success.Load() != 1 || conflicts.Load() != 63 {
		t.Fatalf("CAS results success=%d conflict=%d", success.Load(), conflicts.Load())
	}
}

func TestFrameStoreRejectsSamePublicFrameCoordinateAcrossScopesV1(t *testing.T) {
	fixture, err := testfixture.NewFrameStoreFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	crossScope, err := testfixture.CrossScopeSameFrameIDFixtureV1(fixture, "other")
	if err != nil {
		t.Fatal(err)
	}
	if fixture.State.Frame.ID != crossScope.State.Frame.ID ||
		fixture.State.Frame.Revision != crossScope.State.Frame.Revision ||
		fixture.State.Frame.Execution.ScopeDigest == crossScope.State.Frame.Execution.ScopeDigest {
		t.Fatal("cross-scope fixture does not exercise the public exact-coordinate collision")
	}
	store, err := framestore.OpenSQLiteV1(context.Background(), framestore.SQLiteConfigV1{
		Path: t.TempDir() + "/frame.db", Owner: fixture.Owner, Clock: func() time.Time { return fixture.Now },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.CommitCurrentV1(context.Background(), "commit-public-coordinate-first", fixture.State, nil, fixture.Now.UnixNano()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitCurrentV1(context.Background(), "commit-public-coordinate-cross-scope", crossScope.State, nil, fixture.Now.UnixNano()); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("same Frame ID/revision across scopes was accepted: %v", err)
	}
	request := contract.ContextGenerationCurrentPointerRequestV1{
		ExecutionScopeDigest: crossScope.State.Pointer.ExecutionScopeDigest,
		RunID:                crossScope.State.Pointer.RunID, SessionRef: crossScope.State.Pointer.SessionRef,
		Turn: crossScope.State.Pointer.Turn,
	}
	if pointer, err := store.InspectCurrentGenerationPointer(context.Background(), request); !errors.Is(err, contract.ErrNotFound) ||
		pointer != (contract.ContextGenerationCurrentPointerV1{}) {
		t.Fatalf("rejected cross-scope state became current: pointer=%+v err=%v", pointer, err)
	}
}

func TestFrameStoreExactCurrentS2RejectsConcurrentAdvanceV1(t *testing.T) {
	fixture, next, store, _ := openCommittedFrameStoreV1(t)
	defer store.Close()
	frameDigest, err := fixture.State.Frame.DigestValue()
	if err != nil {
		t.Fatal(err)
	}
	var advanceErr error
	store.BeforeExactCurrentS2ForTestingV1(func() {
		_, advanceErr = store.CommitCurrentV1(context.Background(), "commit-between-s1-s2", next.State, &fixture.State.Pointer, fixture.Now.UnixNano())
	})
	_, err = store.InspectContextFrameExactCurrentV1(context.Background(), contract.FactRef{
		ID: fixture.State.Frame.ID, Revision: fixture.State.Frame.Revision, Digest: frameDigest,
	}, fixture.Now.UnixNano())
	if advanceErr != nil {
		t.Fatalf("legal concurrent advance failed: %v", advanceErr)
	}
	if !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("S1/S2 drift was accepted: %v", err)
	}
}

func TestFrameStoreGenerationCurrentS2RejectsSecondConnectionAdvanceV1(t *testing.T) {
	fixture, next, first, path := openCommittedFrameStoreV1(t)
	defer first.Close()
	second, err := framestore.OpenSQLiteV1(context.Background(), framestore.SQLiteConfigV1{
		Path: path, Owner: fixture.Owner, Clock: func() time.Time { return fixture.Now },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	request := contract.ContextGenerationCurrentPointerRequestV1{
		ExecutionScopeDigest: fixture.State.Pointer.ExecutionScopeDigest,
		RunID:                fixture.State.Pointer.RunID, SessionRef: fixture.State.Pointer.SessionRef,
		Turn: fixture.State.Pointer.Turn,
	}
	var advanceErr error
	first.BeforeGenerationCurrentS2ForTestingV1(func() {
		_, advanceErr = second.CommitCurrentV1(
			context.Background(), "commit-second-connection",
			next.State, &fixture.State.Pointer, fixture.Now.UnixNano(),
		)
	})
	pointer, err := first.InspectCurrentGenerationPointer(context.Background(), request)
	if advanceErr != nil {
		t.Fatalf("second connection legal advance failed: %v", advanceErr)
	}
	if !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("cross-connection S1/S2 drift was accepted: pointer=%+v err=%v", pointer, err)
	}
	if pointer != (contract.ContextGenerationCurrentPointerV1{}) {
		t.Fatalf("drift returned stale pointer: %+v", pointer)
	}
}

func TestFrameStoreRejectsLedgerAndCurrentRowCorruptionV1(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *sql.DB)
		check  func(*testing.T, *framestore.SQLiteV1, testfixture.FrameStoreFixtureV1)
	}{
		{
			name: "state_row_digest",
			mutate: func(t *testing.T, db *sql.DB) {
				replaceLedgerTriggerV1(t, db, `UPDATE context_frame_store_ledger SET state_row_digest=?`, string(contract.DigestBytes([]byte("tampered-state-row"))))
			},
			check: func(t *testing.T, store *framestore.SQLiteV1, _ testfixture.FrameStoreFixtureV1) {
				if _, err := store.InspectCommitV1(context.Background(), "commit-frame-1"); !errors.Is(err, contract.ErrConflict) {
					t.Fatalf("tampered state row digest accepted: %v", err)
				}
			},
		},
		{
			name: "noncanonical_receipt_payload",
			mutate: func(t *testing.T, db *sql.DB) {
				replaceLedgerTriggerV1(t, db, `UPDATE context_frame_store_ledger SET payload=CAST(char(32)||CAST(payload AS TEXT) AS BLOB)`)
			},
			check: func(t *testing.T, store *framestore.SQLiteV1, _ testfixture.FrameStoreFixtureV1) {
				if _, err := store.InspectCommitV1(context.Background(), "commit-frame-1"); !errors.Is(err, contract.ErrConflict) {
					t.Fatalf("noncanonical receipt accepted: %v", err)
				}
			},
		},
		{
			name: "current_pointer_splice",
			mutate: func(t *testing.T, db *sql.DB) {
				if _, err := db.Exec(`UPDATE context_frame_store_current SET pointer_digest=?`, string(contract.DigestBytes([]byte("spliced-pointer")))); err != nil {
					t.Fatal(err)
				}
			},
			check: func(t *testing.T, store *framestore.SQLiteV1, fixture testfixture.FrameStoreFixtureV1) {
				request := contract.ContextGenerationCurrentPointerRequestV1{
					ExecutionScopeDigest: fixture.State.Pointer.ExecutionScopeDigest,
					RunID:                fixture.State.Pointer.RunID, SessionRef: fixture.State.Pointer.SessionRef, Turn: fixture.State.Pointer.Turn,
				}
				if _, err := store.InspectCurrentGenerationPointer(context.Background(), request); !errors.Is(err, contract.ErrConflict) {
					t.Fatalf("spliced current pointer accepted: %v", err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture, _, store, path := openCommittedFrameStoreV1(t)
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(t, db)
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			store, err = framestore.OpenSQLiteV1(context.Background(), framestore.SQLiteConfigV1{
				Path: path, Owner: fixture.Owner, Clock: func() time.Time { return fixture.Now },
			})
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			test.check(t, store, fixture)
		})
	}
}

func TestFrameStorePhysicalVerifierRejectsIndexOnWrongTableV1(t *testing.T) {
	fixture, _, store, path := openCommittedFrameStoreV1(t)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
DROP INDEX context_frame_store_history_frame_public_exact;
CREATE TABLE context_frame_store_fake(
 owner_component_id TEXT NOT NULL,owner_binding_digest TEXT NOT NULL,
 frame_id TEXT NOT NULL,frame_revision INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX context_frame_store_history_frame_public_exact
 ON context_frame_store_fake(owner_component_id,owner_binding_digest,frame_id,frame_revision)`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if reopened, err := framestore.OpenSQLiteV1(context.Background(), framestore.SQLiteConfigV1{
		Path: path, Owner: fixture.Owner, Clock: func() time.Time { return fixture.Now },
	}); !errors.Is(err, contract.ErrConflict) {
		if reopened != nil {
			reopened.Close()
		}
		t.Fatalf("wrong-table exact index passed physical proof: %v", err)
	}
}

func TestFrameStorePhysicalVerifierRejectsCommentWeakTriggerAndFKDriftV1(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *sql.DB)
	}{
		{
			name: "comment_is_not_check",
			mutate: func(t *testing.T, db *sql.DB) {
				rewriteSchemaSQLV1(t, db, "context_frame_store_history",
					"turn INTEGER NOT NULL CHECK(turn > 0)",
					"turn INTEGER NOT NULL /* CHECK(turn > 0) */")
			},
		},
		{
			name: "weak_check",
			mutate: func(t *testing.T, db *sql.DB) {
				rewriteSchemaSQLV1(t, db, "context_frame_store_history",
					"turn INTEGER NOT NULL CHECK(turn > 0)",
					"turn INTEGER NOT NULL CHECK(turn >= 0)")
			},
		},
		{
			name: "no_op_trigger",
			mutate: func(t *testing.T, db *sql.DB) {
				if _, err := db.Exec(`
DROP TRIGGER context_frame_store_history_no_update;
CREATE TRIGGER context_frame_store_history_no_update
BEFORE UPDATE ON context_frame_store_history BEGIN SELECT 1; END`); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "foreign_key_action",
			mutate: func(t *testing.T, db *sql.DB) {
				rewriteSchemaSQLV1(t, db, "context_frame_store_current",
					"REFERENCES context_frame_store_history(owner_component_id,owner_binding_digest,execution_scope_digest,frame_id,frame_revision)",
					"REFERENCES context_frame_store_history(owner_component_id,owner_binding_digest,execution_scope_digest,frame_id,frame_revision) ON DELETE CAS")
			},
		},
		{
			name: "foreign_key_column",
			mutate: func(t *testing.T, db *sql.DB) {
				rewriteSchemaSQLV1(t, db, "context_frame_store_current",
					"execution_scope_digest,frame_id,frame_revision)",
					"execution_scope_digest,frame_id,pointer_revision)")
			},
		},
		{
			name: "foreign_key_sequence",
			mutate: func(t *testing.T, db *sql.DB) {
				rewriteSchemaSQLV1(t, db, "context_frame_store_current",
					"execution_scope_digest,frame_id,frame_revision)\n    REFERENCES",
					"execution_scope_digest,frame_revision,frame_id)\n    REFERENCES")
			},
		},
		{
			name: "trigger_wrong_table",
			mutate: func(t *testing.T, db *sql.DB) {
				if _, err := db.Exec(`
DROP TRIGGER context_frame_store_history_no_update;
CREATE TRIGGER context_frame_store_history_no_update
BEFORE UPDATE ON context_frame_store_current BEGIN SELECT RAISE(ABORT,'context frame history is append-only'); END`); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "generated_column",
			mutate: func(t *testing.T, db *sql.DB) {
				if _, err := db.Exec(`ALTER TABLE context_frame_store_history
ADD COLUMN generated_probe TEXT GENERATED ALWAYS AS (owner_component_id) VIRTUAL`); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "extra_table",
			mutate: func(t *testing.T, db *sql.DB) {
				if _, err := db.Exec(`CREATE TABLE context_frame_store_extra(id TEXT NOT NULL PRIMARY KEY) STRICT`); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "extra_index",
			mutate: func(t *testing.T, db *sql.DB) {
				if _, err := db.Exec(`CREATE INDEX unrelated_name_on_context_store ON context_frame_store_history(run_id)`); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "extra_trigger",
			mutate: func(t *testing.T, db *sql.DB) {
				if _, err := db.Exec(`CREATE TRIGGER unrelated_name_on_context_store
BEFORE INSERT ON context_frame_store_history BEGIN SELECT 1; END`); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "index_nocase_collation",
			mutate: func(t *testing.T, db *sql.DB) {
				if _, err := db.Exec(`
DROP INDEX context_frame_store_history_frame_public_exact;
CREATE UNIQUE INDEX context_frame_store_history_frame_public_exact
ON context_frame_store_history(
 owner_component_id COLLATE NOCASE,owner_binding_digest,frame_id,frame_revision
)`); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "extra_table_check",
			mutate: func(t *testing.T, db *sql.DB) {
				rewriteSchemaSQLV1(t, db, "context_frame_store_history",
					"PRIMARY KEY(owner_component_id,owner_binding_digest,execution_scope_digest,frame_id,frame_revision)\n) STRICT",
					"PRIMARY KEY(owner_component_id,owner_binding_digest,execution_scope_digest,frame_id,frame_revision),\n  CHECK(run_id <> 'forbidden')\n) STRICT")
			},
		},
		{
			name: "non_index_column_nocase",
			mutate: func(t *testing.T, db *sql.DB) {
				rewriteSchemaSQLV1(t, db, "context_frame_store_history",
					"run_id TEXT NOT NULL",
					"run_id TEXT NOT NULL COLLATE NOCASE")
			},
		},
		{
			name: "trigger_quoted_comment_literal",
			mutate: func(t *testing.T, db *sql.DB) {
				rewriteSchemaSQLV1(t, db, "context_frame_store_history_no_update",
					"'context frame history is append-only'",
					"'context frame history /*not a comment*/ is append-only'")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture, _, store, path := openCommittedFrameStoreV1(t)
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", "file:"+path)
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(t, db)
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if reopened, err := framestore.OpenSQLiteV1(context.Background(), framestore.SQLiteConfigV1{
				Path: path, Owner: fixture.Owner, Clock: func() time.Time { return fixture.Now },
			}); err == nil {
				if reopened != nil {
					reopened.Close()
				}
				t.Fatalf("physical drift passed proof: %v", err)
			}
		})
	}
}

func TestFrameStorePhysicalVerifierAllowsSemanticCommentsAndRollsBackPositiveChainV1(t *testing.T) {
	fixture, _, store, path := openCommittedFrameStoreV1(t)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	rewriteSchemaSQLV1(t, db, "context_frame_store_history_no_update",
		"BEFORE UPDATE ON context_frame_store_history",
		"BEFORE /* harmless schema comment */ UPDATE ON context_frame_store_history")
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = framestore.OpenSQLiteV1(context.Background(), framestore.SQLiteConfigV1{
		Path: path, Owner: fixture.Owner, Clock: func() time.Time { return fixture.Now },
	})
	if err != nil {
		t.Fatalf("semantic comment changed DDL meaning: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, table := range []string{
		"context_frame_store_history",
		"context_frame_store_current",
		"context_frame_store_ledger",
	} {
		var count int
		if err := db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM "%s" WHERE owner_component_id LIKE 'physical-%%'`, table)).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("positive schema probe leaked rows in %s: %d", table, count)
		}
	}
}

func TestFrameStoreRejectsOwnerAndMetadataSpliceV1(t *testing.T) {
	fixture, _, store, path := openCommittedFrameStoreV1(t)
	receipt, err := store.InspectCommitV1(context.Background(), "commit-frame-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	otherOwner := fixture.Owner
	otherOwner.BindingDigest = contract.DigestBytes([]byte("other-owner"))
	other, err := framestore.OpenSQLiteV1(context.Background(), framestore.SQLiteConfigV1{
		Path: path, Owner: otherOwner, Clock: func() time.Time { return fixture.Now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.InspectCommitV1(context.Background(), "commit-frame-1"); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("cross-Owner ledger lookup accepted: %v", err)
	}
	if _, err := other.InspectContextFrameExactCurrentV1(context.Background(), receipt.FrameRef, fixture.Now.UnixNano()); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("cross-Owner frame lookup accepted: %v", err)
	}
	if err := other.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TRIGGER context_frame_store_history_no_update`); err != nil {
		t.Fatal(err)
	}
	spliced := contract.DigestBytes([]byte("spliced-manifest"))
	if _, err := db.Exec(`UPDATE context_frame_store_history SET manifest_digest=?`, string(spliced)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER context_frame_store_history_no_update
BEFORE UPDATE ON context_frame_store_history BEGIN SELECT RAISE(ABORT,'context frame history is append-only'); END`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = framestore.OpenSQLiteV1(context.Background(), framestore.SQLiteConfigV1{
		Path: path, Owner: fixture.Owner, Clock: func() time.Time { return fixture.Now },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.ManifestByExactRef(context.Background(), contract.FactRef{
		ID: fixture.State.Manifest.ID, Revision: fixture.State.Manifest.Revision, Digest: spliced,
	}, fixture.State.Frame.Execution.ScopeDigest); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("manifest denormalized splice accepted: %v", err)
	}
}

func openCommittedFrameStoreV1(t *testing.T) (testfixture.FrameStoreFixtureV1, testfixture.FrameStoreFixtureV1, *framestore.SQLiteV1, string) {
	t.Helper()
	fixture, err := testfixture.NewFrameStoreFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	next, err := testfixture.AdvanceFrameStoreFixtureV1(fixture, "next")
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/frame.db"
	store, err := framestore.OpenSQLiteV1(context.Background(), framestore.SQLiteConfigV1{
		Path: path, Owner: fixture.Owner, Clock: func() time.Time { return fixture.Now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitCurrentV1(context.Background(), "commit-frame-1", fixture.State, nil, fixture.Now.UnixNano()); err != nil {
		store.Close()
		t.Fatal(err)
	}
	return fixture, next, store, path
}

func replaceLedgerTriggerV1(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(`DROP TRIGGER context_frame_store_ledger_no_update`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER context_frame_store_ledger_no_update
BEFORE UPDATE ON context_frame_store_ledger BEGIN SELECT RAISE(ABORT,'context frame ledger is append-only'); END`); err != nil {
		t.Fatal(err)
	}
}

func rewriteSchemaSQLV1(t *testing.T, db *sql.DB, object, old, replacement string) {
	t.Helper()
	if _, err := db.Exec(`PRAGMA writable_schema=ON`); err != nil {
		t.Fatal(err)
	}
	result, err := db.Exec(`UPDATE sqlite_master SET sql=replace(sql,?,?) WHERE name=?`, old, replacement, object)
	if err != nil {
		t.Fatal(err)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		t.Fatalf("schema rewrite affected=%d err=%v", affected, err)
	}
	var version int
	if err := db.QueryRow(`PRAGMA schema_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(fmt.Sprintf(`PRAGMA schema_version=%d`, version+1)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA writable_schema=OFF`); err != nil {
		t.Fatal(err)
	}
}

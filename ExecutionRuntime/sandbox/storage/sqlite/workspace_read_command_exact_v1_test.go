package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestWorkspaceReadCommandExactReaderV1RejectsCanonicalTimestampSpliceAfterRestart(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*contract.WorkspaceReadCommandV1)
	}{
		{
			name: "created timestamp",
			mutate: func(command *contract.WorkspaceReadCommandV1) {
				command.Meta.CreatedUnixNano--
			},
		},
		{
			name: "updated timestamp",
			mutate: func(command *contract.WorkspaceReadCommandV1) {
				command.Meta.UpdatedUnixNano++
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			now := time.Unix(1_949_999_900, 0)
			expires := now.Add(time.Minute)
			database := filepath.Join(t.TempDir(), "sandbox.db")
			store, err := OpenWithClock(ctx, database, func() time.Time { return now })
			if err != nil {
				t.Fatal(err)
			}
			_, command := workspaceReadCompletionInputFixtureV1(t, now, expires)
			if _, err = store.CreateWorkspaceReadCommandV1(ctx, command); err != nil {
				t.Fatal(err)
			}
			exact := command.Meta.Ref()
			test.mutate(&command)
			if err = command.ValidateShape(); err != nil || command.Meta.Ref() != exact {
				t.Fatalf("timestamp splice precondition failed: command=%#v err=%v", command, err)
			}
			body, err := encode(command)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = store.db.ExecContext(
				ctx,
				`UPDATE workspace_read_command_current SET body=? WHERE command_id=?`,
				body,
				exact.ID,
			); err != nil {
				t.Fatal(err)
			}
			if err = store.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := OpenWithClock(ctx, database, func() time.Time {
				panic("timestamp splice Inspect consulted the current clock")
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = reopened.Close() })
			if _, err = reopened.InspectWorkspaceReadCommandExactV1(
				ctx,
				exact,
			); !errors.Is(err, ports.ErrConflict) {
				t.Fatalf("canonical %s splice error=%v", test.name, err)
			}
		})
	}
}

func TestWorkspaceReadCommandExactReaderV1V17MigrationDoesNotSelfSealLegacyBody(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_949_999_950, 0)
	expires := now.Add(time.Minute)
	database := filepath.Join(t.TempDir(), "sandbox.db")
	store, err := OpenWithClock(ctx, database, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	_, command := workspaceReadCompletionInputFixtureV1(t, now, expires)
	if _, err = store.CreateWorkspaceReadCommandV1(ctx, command); err != nil {
		t.Fatal(err)
	}
	if _, err = store.db.ExecContext(
		ctx,
		`DROP TABLE workspace_read_command_body_seal;
		 DROP TABLE workspace_read_runtime_attempt_admission_binding_v2`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = store.db.ExecContext(ctx, `PRAGMA user_version=16`); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenWithClock(ctx, database, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if _, err = reopened.InspectWorkspaceReadCommandExactV1(
		ctx,
		command.Meta.Ref(),
	); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("legacy Command without proof error=%v", err)
	}
	if _, err = reopened.CreateWorkspaceReadCommandV1(
		ctx,
		command,
	); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("legacy Command retry self-sealed body: %v", err)
	}
	var seals int
	if err = reopened.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM workspace_read_command_body_seal WHERE command_id=?`,
		command.Meta.ID,
	).Scan(&seals); err != nil {
		t.Fatal(err)
	}
	if seals != 0 {
		t.Fatalf("legacy migration created body seals=%d", seals)
	}
	var version int
	if err = reopened.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != schemaVersion {
		t.Fatalf("migrated schema version=%d want=%d", version, schemaVersion)
	}
}

func TestWorkspaceReadCommandExactReaderV1CommandAndSealCreateAreAtomic(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_949_999_975, 0)
	expires := now.Add(time.Minute)
	store, err := OpenWithClock(
		ctx,
		filepath.Join(t.TempDir(), "sandbox.db"),
		func() time.Time { return now },
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err = store.db.ExecContext(
		ctx,
		`CREATE TRIGGER reject_workspace_read_command_body_seal
		 BEFORE INSERT ON workspace_read_command_body_seal
		 BEGIN
		   SELECT RAISE(ABORT, 'body seal insert failed');
		 END`,
	); err != nil {
		t.Fatal(err)
	}
	_, command := workspaceReadCompletionInputFixtureV1(t, now, expires)
	if _, err = store.CreateWorkspaceReadCommandV1(ctx, command); err == nil {
		t.Fatal("Command creation succeeded after body seal failure")
	}
	for _, table := range []string{
		"workspace_read_command_current",
		"workspace_read_command_body_seal",
	} {
		var rows int
		if err = store.db.QueryRowContext(
			ctx,
			`SELECT COUNT(*) FROM `+table,
		).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if rows != 0 {
			t.Fatalf("%s rows=%d after body seal failure", table, rows)
		}
	}
}

func TestWorkspaceReadCommandExactReaderV1RejectsIncompatibleBodySealSchema(t *testing.T) {
	for _, test := range []struct {
		name    string
		columns string
	}{
		{
			name: "extra normal column",
			columns: `command_id TEXT NOT NULL PRIMARY KEY,
				revision INTEGER NOT NULL,
				digest TEXT NOT NULL,
				canonical_body_digest TEXT NOT NULL,
				unapproved_column TEXT`,
		},
		{
			name: "extra hidden generated column",
			columns: `command_id TEXT NOT NULL PRIMARY KEY,
				revision INTEGER NOT NULL,
				digest TEXT NOT NULL,
				canonical_body_digest TEXT NOT NULL,
				unapproved_generated TEXT GENERATED ALWAYS AS (digest) VIRTUAL`,
		},
		{
			name: "wrong type",
			columns: `command_id TEXT NOT NULL PRIMARY KEY,
				revision TEXT NOT NULL,
				digest TEXT NOT NULL,
				canonical_body_digest TEXT NOT NULL`,
		},
		{
			name: "nullable column",
			columns: `command_id TEXT NOT NULL PRIMARY KEY,
				revision INTEGER NOT NULL,
				digest TEXT,
				canonical_body_digest TEXT NOT NULL`,
		},
		{
			name: "default value",
			columns: `command_id TEXT NOT NULL PRIMARY KEY,
				revision INTEGER NOT NULL,
				digest TEXT NOT NULL,
				canonical_body_digest TEXT NOT NULL DEFAULT ''`,
		},
		{
			name: "wrong primary key",
			columns: `command_id TEXT NOT NULL,
				revision INTEGER NOT NULL,
				digest TEXT NOT NULL,
				canonical_body_digest TEXT NOT NULL,
				PRIMARY KEY(command_id,revision)`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			database := filepath.Join(t.TempDir(), "sandbox.db")
			raw, err := sql.Open("sqlite", dataSource(database))
			if err != nil {
				t.Fatal(err)
			}
			if _, err = raw.ExecContext(
				ctx,
				`CREATE TABLE workspace_read_command_body_seal (`+test.columns+`)`,
			); err != nil {
				_ = raw.Close()
				t.Fatal(err)
			}
			if _, err = raw.ExecContext(ctx, `PRAGMA user_version=16`); err != nil {
				_ = raw.Close()
				t.Fatal(err)
			}
			if err = raw.Close(); err != nil {
				t.Fatal(err)
			}
			if store, openErr := OpenWithClock(ctx, database, time.Now); openErr == nil {
				_ = store.Close()
				t.Fatal("incompatible workspace read Command body seal schema was accepted")
			}
		})
	}
}

func TestWorkspaceReadCommandExactReaderV1ReadsExpiredFactAcrossRestart(t *testing.T) {
	ctx := context.Background()
	created := time.Unix(1_950_000_000, 0)
	expires := created.Add(time.Minute)
	current := created
	database := filepath.Join(t.TempDir(), "sandbox.db")
	store, err := OpenWithClock(ctx, database, func() time.Time { return current })
	if err != nil {
		t.Fatal(err)
	}
	_, command := workspaceReadCompletionInputFixtureV1(t, created, expires)
	if _, err = store.CreateWorkspaceReadCommandV1(ctx, command); err != nil {
		t.Fatal(err)
	}

	current = expires
	if _, err = store.InspectWorkspaceReadCommandCurrentV1(ctx, command.Meta.Ref()); err == nil {
		t.Fatal("expired workspace.read Command remained current")
	}
	store.db.SetMaxOpenConns(1)
	store.db.SetMaxIdleConns(1)
	var changesBefore int64
	if err = store.db.QueryRowContext(ctx, `SELECT total_changes()`).Scan(&changesBefore); err != nil {
		t.Fatal(err)
	}
	store.clock = func() time.Time {
		panic("historical exact Command reader consulted the current clock")
	}
	got, err := store.InspectWorkspaceReadCommandExactV1(ctx, command.Meta.Ref())
	if err != nil || !reflect.DeepEqual(got, command) {
		t.Fatalf("expired exact Command was not readable: got=%#v err=%v", got, err)
	}
	var changesAfter int64
	if err = store.db.QueryRowContext(ctx, `SELECT total_changes()`).Scan(&changesAfter); err != nil {
		t.Fatal(err)
	}
	if changesAfter != changesBefore {
		t.Fatalf("historical exact read wrote SQLite state: before=%d after=%d", changesBefore, changesAfter)
	}
	if got.Meta.ExpiresUnixNano != expires.UnixNano() ||
		got.RequestedNotAfterUnixNano != expires.UnixNano() {
		t.Fatalf("historical read renewed Command lifetime: %#v", got.Meta)
	}

	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenWithClock(ctx, database, func() time.Time { return expires.Add(time.Hour) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	reopened.clock = func() time.Time {
		panic("restart historical exact Command reader consulted the current clock")
	}
	got, err = reopened.InspectWorkspaceReadCommandExactV1(ctx, command.Meta.Ref())
	if err != nil || !reflect.DeepEqual(got, command) {
		t.Fatalf("restart historical Command drifted: got=%#v err=%v", got, err)
	}
}

func TestWorkspaceReadCommandExactReaderV1SixtyFourConcurrentReadsAreImmutableAndWriteFree(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_950_000_100, 0)
	expires := now.Add(time.Minute)
	store, err := OpenWithClock(ctx, filepath.Join(t.TempDir(), "sandbox.db"), func() time.Time {
		return expires.Add(time.Hour)
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	_, command := workspaceReadCompletionInputFixtureV1(t, now, expires)
	// Creation requires a current clock. Temporarily use the creation time,
	// then move beyond expiry before every historical read.
	store.clock = func() time.Time { return now }
	if _, err = store.CreateWorkspaceReadCommandV1(ctx, command); err != nil {
		t.Fatal(err)
	}
	store.clock = func() time.Time { return expires.Add(time.Hour) }

	var beforeBody []byte
	if err = store.db.QueryRowContext(
		ctx,
		`SELECT body FROM workspace_read_command_current WHERE command_id=?`,
		command.Meta.ID,
	).Scan(&beforeBody); err != nil {
		t.Fatal(err)
	}
	const readers = 64
	var group sync.WaitGroup
	failures := make(chan error, readers)
	for range readers {
		group.Add(1)
		go func() {
			defer group.Done()
			got, inspectErr := store.InspectWorkspaceReadCommandExactV1(ctx, command.Meta.Ref())
			if inspectErr == nil && !reflect.DeepEqual(got, command) {
				inspectErr = errors.New("historical Command winner drifted")
			}
			failures <- inspectErr
		}()
	}
	group.Wait()
	close(failures)
	for inspectErr := range failures {
		if inspectErr != nil {
			t.Fatal(inspectErr)
		}
	}
	var afterBody []byte
	if err = store.db.QueryRowContext(
		ctx,
		`SELECT body FROM workspace_read_command_current WHERE command_id=?`,
		command.Meta.ID,
	).Scan(&afterBody); err != nil {
		t.Fatal(err)
	}
	if string(afterBody) != string(beforeBody) {
		t.Fatal("historical readers mutated the immutable Command row")
	}
}

func TestWorkspaceReadCommandExactReaderV1RejectsUnavailableCancellationAndInvalidRef(t *testing.T) {
	ctx := context.Background()
	valid := contract.Ref{
		ID: "command", Revision: 1,
		Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	var unavailable *Store
	if _, err := unavailable.InspectWorkspaceReadCommandExactV1(ctx, valid); err == nil {
		t.Fatal("typed-nil exact Command reader was accepted")
	}

	store, err := OpenWithClock(ctx, filepath.Join(t.TempDir(), "sandbox.db"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err = store.InspectWorkspaceReadCommandExactV1(nil, valid); err == nil {
		t.Fatal("nil context was accepted")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = store.InspectWorkspaceReadCommandExactV1(cancelled, valid); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context was not preserved: %v", err)
	}
	if _, err = store.InspectWorkspaceReadCommandExactV1(ctx, contract.Ref{}); err == nil {
		t.Fatal("invalid exact Command Ref was accepted")
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = store.InspectWorkspaceReadCommandExactV1(ctx, valid); err == nil {
		t.Fatal("closed exact Command reader was accepted")
	}
}

func TestWorkspaceReadCommandExactReaderV1RejectsCoordinateAndBodySplice(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, *Store, contract.WorkspaceReadCommandV1)
	}{
		{
			name: "revision column",
			mutate: func(t *testing.T, store *Store, command contract.WorkspaceReadCommandV1) {
				t.Helper()
				if _, err := store.db.Exec(
					`UPDATE workspace_read_command_current SET revision=? WHERE command_id=?`,
					command.Meta.Revision+1,
					command.Meta.ID,
				); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "digest column",
			mutate: func(t *testing.T, store *Store, command contract.WorkspaceReadCommandV1) {
				t.Helper()
				if _, err := store.db.Exec(
					`UPDATE workspace_read_command_current SET digest=? WHERE command_id=?`,
					mustWorkspaceReadDigest(t, "spliced-command-digest"),
					command.Meta.ID,
				); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "body source command",
			mutate: func(t *testing.T, store *Store, command contract.WorkspaceReadCommandV1) {
				t.Helper()
				command.SourceToolCommand.ID = "spliced-tool-command"
				body, err := encode(command)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = store.db.Exec(
					`UPDATE workspace_read_command_current SET body=? WHERE command_id=?`,
					body,
					command.Meta.ID,
				); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "body exact ref",
			mutate: func(t *testing.T, store *Store, command contract.WorkspaceReadCommandV1) {
				t.Helper()
				command.Meta.Revision++
				body, err := encode(command)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = store.db.Exec(
					`UPDATE workspace_read_command_current SET body=? WHERE command_id=?`,
					body,
					command.Meta.ID,
				); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "seal revision",
			mutate: func(t *testing.T, store *Store, command contract.WorkspaceReadCommandV1) {
				t.Helper()
				if _, err := store.db.Exec(
					`UPDATE workspace_read_command_body_seal SET revision=? WHERE command_id=?`,
					command.Meta.Revision+1,
					command.Meta.ID,
				); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "seal digest",
			mutate: func(t *testing.T, store *Store, command contract.WorkspaceReadCommandV1) {
				t.Helper()
				if _, err := store.db.Exec(
					`UPDATE workspace_read_command_body_seal SET digest=? WHERE command_id=?`,
					mustWorkspaceReadDigest(t, "spliced-seal-digest"),
					command.Meta.ID,
				); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "seal body digest",
			mutate: func(t *testing.T, store *Store, command contract.WorkspaceReadCommandV1) {
				t.Helper()
				if _, err := store.db.Exec(
					`UPDATE workspace_read_command_body_seal
						    SET canonical_body_digest=?
						  WHERE command_id=?`,
					mustWorkspaceReadDigest(t, "spliced-seal-body"),
					command.Meta.ID,
				); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "missing seal",
			mutate: func(t *testing.T, store *Store, command contract.WorkspaceReadCommandV1) {
				t.Helper()
				if _, err := store.db.Exec(
					`DELETE FROM workspace_read_command_body_seal WHERE command_id=?`,
					command.Meta.ID,
				); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			now := time.Unix(1_950_000_200, 0)
			expires := now.Add(time.Minute)
			store, err := OpenWithClock(ctx, filepath.Join(t.TempDir(), "sandbox.db"), func() time.Time { return now })
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			_, command := workspaceReadCompletionInputFixtureV1(t, now, expires)
			if _, err = store.CreateWorkspaceReadCommandV1(ctx, command); err != nil {
				t.Fatal(err)
			}
			test.mutate(t, store, command)
			if _, err = store.InspectWorkspaceReadCommandExactV1(ctx, command.Meta.Ref()); !errors.Is(err, ports.ErrConflict) {
				t.Fatalf("spliced exact Command error=%v", err)
			}
			var rows int
			if err = store.db.QueryRow(
				`SELECT COUNT(*) FROM workspace_read_command_current WHERE command_id=?`,
				command.Meta.ID,
			).Scan(&rows); err != nil {
				t.Fatal(err)
			}
			if rows != 1 {
				t.Fatalf("read path changed Command row count=%d", rows)
			}
		})
	}
}

func TestWorkspaceReadCommandExactReaderV1RejectsNonCanonicalAndDuplicateJSONAfterRestart(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{
			name: "noncanonical whitespace",
			mutate: func(body []byte) []byte {
				return append([]byte(" \n\t"), body...)
			},
		},
		{
			name: "duplicate top-level key",
			mutate: func(body []byte) []byte {
				// Preserve valid JSON while appending a duplicate tenant_id.
				mutated := append([]byte(nil), body[:len(body)-1]...)
				return append(mutated, []byte(`,"tenant_id":"tenant"}`)...)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			now := time.Unix(1_950_000_250, 0)
			expires := now.Add(time.Minute)
			database := filepath.Join(t.TempDir(), "sandbox.db")
			store, err := OpenWithClock(ctx, database, func() time.Time { return now })
			if err != nil {
				t.Fatal(err)
			}
			_, command := workspaceReadCompletionInputFixtureV1(t, now, expires)
			if _, err = store.CreateWorkspaceReadCommandV1(ctx, command); err != nil {
				t.Fatal(err)
			}
			var body []byte
			if err = store.db.QueryRow(
				`SELECT body FROM workspace_read_command_current WHERE command_id=?`,
				command.Meta.ID,
			).Scan(&body); err != nil {
				t.Fatal(err)
			}
			if _, err = store.db.Exec(
				`UPDATE workspace_read_command_current SET body=? WHERE command_id=?`,
				test.mutate(body),
				command.Meta.ID,
			); err != nil {
				t.Fatal(err)
			}
			if err = store.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := OpenWithClock(ctx, database, func() time.Time { return expires.Add(time.Hour) })
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = reopened.Close() })
			if _, err = reopened.InspectWorkspaceReadCommandExactV1(ctx, command.Meta.Ref()); !errors.Is(err, ports.ErrConflict) {
				t.Fatalf("noncanonical stored Command error=%v", err)
			}
		})
	}
}

func TestWorkspaceReadCommandExactReaderV1MissingAndCoordinateMismatch(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_950_000_300, 0)
	expires := now.Add(time.Minute)
	store, err := OpenWithClock(ctx, filepath.Join(t.TempDir(), "sandbox.db"), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	_, command := workspaceReadCompletionInputFixtureV1(t, now, expires)
	if _, err = store.CreateWorkspaceReadCommandV1(ctx, command); err != nil {
		t.Fatal(err)
	}
	missing := command.Meta.Ref()
	missing.ID = "missing-command"
	if _, err = store.InspectWorkspaceReadCommandExactV1(ctx, missing); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("missing exact Command error=%v", err)
	}
	mismatch := command.Meta.Ref()
	mismatch.Revision++
	if _, err = store.InspectWorkspaceReadCommandExactV1(ctx, mismatch); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("revision mismatch error=%v", err)
	}
	mismatch = command.Meta.Ref()
	mismatch.Digest = mustWorkspaceReadDigest(t, "other-exact-command")
	if _, err = store.InspectWorkspaceReadCommandExactV1(ctx, mismatch); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("digest mismatch error=%v", err)
	}
}

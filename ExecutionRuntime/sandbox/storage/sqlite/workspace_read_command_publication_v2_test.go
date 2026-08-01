package sqlite

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	ownerworkspaceread "github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/owner/workspaceread"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type workspaceReadPublicationClockV2 struct {
	nano atomic.Int64
	step int64
}

type workspaceReadPublicationCommitterV2 struct {
	err   error
	calls int
}

type workspaceReadPublicationCodedErrorV2 struct {
	code int
}

func (e workspaceReadPublicationCodedErrorV2) Error() string { return "coded sqlite failure" }
func (e workspaceReadPublicationCodedErrorV2) Code() int     { return e.code }

func (c *workspaceReadPublicationCommitterV2) Commit() error {
	c.calls++
	return c.err
}

func newWorkspaceReadPublicationClockV2(now time.Time, step time.Duration) *workspaceReadPublicationClockV2 {
	clock := &workspaceReadPublicationClockV2{step: int64(step)}
	clock.nano.Store(now.UnixNano())
	return clock
}

func (c *workspaceReadPublicationClockV2) Now() time.Time {
	if c.step == 0 {
		return time.Unix(0, c.nano.Load())
	}
	return time.Unix(0, c.nano.Add(c.step))
}

func (c *workspaceReadPublicationClockV2) Set(now time.Time) {
	c.nano.Store(now.UnixNano())
}

type workspaceReadPublicationClosureV2 struct {
	fixture     testkit.WorkspaceReadCommandPublicationFixtureV2
	command     contract.WorkspaceReadCommandV1
	publication contract.WorkspaceReadCommandPublicationV2
	current     contract.WorkspaceReadCommandOwnerCurrentV2
	capability  ownerworkspaceread.AuthorizedCommandPublicationV2
}

func newWorkspaceReadPublicationClosureV2(
	t *testing.T,
	fixture testkit.WorkspaceReadCommandPublicationFixtureV2,
	commitNow time.Time,
) workspaceReadPublicationClosureV2 {
	t.Helper()
	semantic, err := contract.SealWorkspaceReadCommandPublicationSemanticV2(
		fixture.Source,
		fixture.Effect,
		fixture.Prepared,
		fixture.Workspace,
		commitNow,
	)
	if err != nil {
		t.Fatalf("seal publication semantic: %v", err)
	}
	command, err := contract.SealWorkspaceReadPublishedCommandV2(semantic, commitNow)
	if err != nil {
		t.Fatalf("seal published Command: %v", err)
	}
	publication, err := contract.SealWorkspaceReadCommandPublicationV2(
		contract.WorkspaceReadCommandPublicationV2{Semantic: semantic},
		command,
		commitNow,
	)
	if err != nil {
		t.Fatalf("seal publication: %v", err)
	}
	current, err := contract.SealInitialWorkspaceReadCommandOwnerCurrentV2(
		workspaceReadPublicationCurrentBodyV2(
			command,
			publication,
			fixture,
			commitNow,
		),
		commitNow,
	)
	if err != nil {
		t.Fatalf("seal initial current: %v", err)
	}
	capability, err := ownerworkspaceread.NewInitialCommandPublicationV2(
		command,
		publication,
		current,
		fixture.Source,
		fixture.Effect,
		fixture.Prepared,
		fixture.Workspace,
		commitNow,
	)
	if err != nil {
		t.Fatalf("seal initial capability: %v", err)
	}
	return workspaceReadPublicationClosureV2{
		fixture: fixture, command: command, publication: publication,
		current: current, capability: capability,
	}
}

func workspaceReadPublicationCurrentBodyV2(
	command contract.WorkspaceReadCommandV1,
	publication contract.WorkspaceReadCommandPublicationV2,
	fixture testkit.WorkspaceReadCommandPublicationFixtureV2,
	checked time.Time,
) contract.WorkspaceReadCommandOwnerCurrentV2 {
	return contract.WorkspaceReadCommandOwnerCurrentV2{
		Command:                         command.Meta.Ref(),
		Publication:                     publication.Meta.Ref(),
		PublicationSemanticDigest:       publication.Semantic.Digest,
		SourceCommand:                   publication.Semantic.SourceCommand,
		SourceSemanticDigest:            publication.Semantic.SourceSemanticDigest,
		SourceProjectionDigest:          fixture.Source.ProjectionDigest,
		SourceCheckedUnixNano:           fixture.Source.CheckedUnixNano,
		SourceExpiresUnixNano:           fixture.Source.ExpiresUnixNano,
		RuntimeEffectProjectionDigest:   fixture.Effect.Digest,
		RuntimeEffectCheckedUnixNano:    fixture.Effect.CheckedUnixNano,
		RuntimeEffectExpiresUnixNano:    fixture.Effect.ExpiresUnixNano,
		RuntimePreparedProjectionDigest: fixture.Prepared.ProjectionDigest,
		RuntimePreparedCheckedUnixNano:  fixture.Prepared.CheckedUnixNano,
		RuntimePreparedExpiresUnixNano:  fixture.Prepared.ExpiresUnixNano,
		WorkspaceView:                   fixture.Workspace.Meta.Ref(),
		WorkspaceSemanticDigest:         publication.Semantic.WorkspaceSemanticDigest,
		WorkspaceCheckedUnixNano:        checked.UnixNano(),
		WorkspaceExpiresUnixNano:        fixture.Workspace.Meta.ExpiresUnixNano,
		WorkspaceLeaseExpiresUnixNano:   fixture.Workspace.Lease.ExpiresUnixNano,
		SemanticNotAfterUnixNano:        publication.Semantic.SemanticNotAfterUnixNano,
	}
}

func newWorkspaceReadPublicationRefreshV2(
	t *testing.T,
	closure workspaceReadPublicationClosureV2,
	expected contract.WorkspaceReadCommandOwnerCurrentV2,
	checked time.Time,
) (
	contract.WorkspaceReadCommandOwnerCurrentV2,
	ownerworkspaceread.AuthorizedCommandPublicationV2,
) {
	t.Helper()
	body := workspaceReadPublicationCurrentBodyV2(
		closure.command,
		closure.publication,
		closure.fixture,
		checked,
	)
	next, err := contract.SealNextWorkspaceReadCommandOwnerCurrentV2(body, expected, checked)
	if err != nil {
		t.Fatalf("seal next current: %v", err)
	}
	capability, err := ownerworkspaceread.NewRefreshCommandPublicationV2(
		closure.command,
		closure.publication,
		expected,
		next,
		closure.fixture.Source,
		closure.fixture.Effect,
		closure.fixture.Prepared,
		closure.fixture.Workspace,
		checked,
	)
	if err != nil {
		t.Fatalf("seal refresh capability: %v", err)
	}
	return next, capability
}

func openWorkspaceReadPublicationStoreV2(
	t *testing.T,
	clock func() time.Time,
) (*Store, string) {
	t.Helper()
	database := filepath.Join(t.TempDir(), "sandbox.db")
	store, err := OpenWithClock(context.Background(), database, clock)
	if err != nil {
		t.Fatalf("open publication store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, database
}

func TestWorkspaceReadCommandPublicationV2EightIndependentOpenHandlesNaturalClockUseOneWinner(t *testing.T) {
	ctx := context.Background()
	database := filepath.Join(t.TempDir(), "sandbox.db")
	stores := make([]*Store, 8)
	for index := range stores {
		var err error
		stores[index], err = Open(ctx, database)
		if err != nil {
			t.Fatalf("open publication store handle %d: %v", index, err)
		}
		store := stores[index]
		t.Cleanup(func() { _ = store.Close() })
	}
	// Open/verify all handles before starting the bounded current window. Under
	// -race, schema verification is intentionally expensive and must not consume
	// most of the Source current's ten-second authority before Apply begins.
	base := time.Now().UTC().Add(-100 * time.Millisecond)
	fixture := testkit.WorkspaceReadCommandPublicationV2(base, "natural-clock")
	closures := make([]workspaceReadPublicationClosureV2, 64)
	for index := range closures {
		closures[index] = newWorkspaceReadPublicationClosureV2(
			t,
			fixture,
			base.Add(50*time.Millisecond+time.Duration(index)*time.Nanosecond),
		)
	}
	var created atomic.Int64
	results := make(chan contract.Ref, len(closures))
	errs := make(chan error, len(closures))
	var wait sync.WaitGroup
	for index := range closures {
		wait.Add(1)
		go func(store *Store, closure workspaceReadPublicationClosureV2) {
			defer wait.Done()
			current, won, err := store.ApplyWorkspaceReadCommandPublicationV2(ctx, closure.capability)
			if err != nil {
				errs <- err
				return
			}
			if won {
				created.Add(1)
			}
			results <- current.Meta.Ref()
		}(stores[index%len(stores)], closures[index])
	}
	wait.Wait()
	close(errs)
	close(results)
	for err := range errs {
		t.Errorf("concurrent initial Apply: %v", err)
	}
	if created.Load() != 1 {
		t.Fatalf("created winners=%d want=1", created.Load())
	}
	var winner contract.Ref
	for result := range results {
		if winner.ID == "" {
			winner = result
		}
		if result != winner {
			t.Fatalf("concurrent initial returned sibling current: got=%#v winner=%#v", result, winner)
		}
	}
}

func TestWorkspaceReadCommandPublicationV2CommitFailureClassificationIsWriteSensitive(t *testing.T) {
	injected := errors.New("injected transaction end failure")
	readOnly := &workspaceReadPublicationCommitterV2{err: injected}
	if err := commitWorkspaceReadCommandPublicationV2(readOnly, false); err == nil ||
		errors.Is(err, ports.ErrUnknownOutcome) {
		t.Fatalf("read-only commit failure=%v", err)
	}
	if readOnly.calls != 1 {
		t.Fatalf("read-only commit calls=%d", readOnly.calls)
	}
	write := &workspaceReadPublicationCommitterV2{err: injected}
	if err := commitWorkspaceReadCommandPublicationV2(write, true); !errors.Is(err, ports.ErrUnknownOutcome) {
		t.Fatalf("write commit failure=%v", err)
	}
	if write.calls != 1 {
		t.Fatalf("write commit calls=%d", write.calls)
	}
}

func TestWorkspaceReadCommandPublicationV2SQLiteRetryClassificationIsClosed(t *testing.T) {
	for _, code := range []int{5, 517, 6, 262} { // BUSY, BUSY_SNAPSHOT, LOCKED, LOCKED_SHAREDCACHE.
		err := fmt.Errorf("wrapped: %w", workspaceReadPublicationCodedErrorV2{code: code})
		if !workspaceReadCommandPublicationSQLiteBusyV2(err) {
			t.Fatalf("retryable SQLite code %d was not recognized", code)
		}
	}
	for _, err := range []error{
		workspaceReadPublicationCodedErrorV2{code: 1},
		workspaceReadPublicationCodedErrorV2{code: 19},
		errors.New("sql: database is closed"),
	} {
		if workspaceReadCommandPublicationSQLiteBusyV2(err) {
			t.Fatalf("non-retryable storage failure was accepted: %v", err)
		}
	}
	busy := workspaceReadPublicationCodedErrorV2{code: 5}
	readOnly := &workspaceReadPublicationCommitterV2{err: busy}
	if err := commitWorkspaceReadCommandPublicationV2(readOnly, false); !workspaceReadCommandPublicationSQLiteBusyV2(err) {
		t.Fatalf("read-only BUSY must remain retryable: %v", err)
	}
	write := &workspaceReadPublicationCommitterV2{err: busy}
	err := commitWorkspaceReadCommandPublicationV2(write, true)
	if !errors.Is(err, ports.ErrUnknownOutcome) || workspaceReadCommandPublicationSQLiteBusyV2(err) {
		t.Fatalf("write-commit BUSY must be Unknown and non-retryable: %v", err)
	}
}

func TestWorkspaceReadCommandPublicationV2EightHandleRefreshCASRestartAndLostReply(t *testing.T) {
	ctx := context.Background()
	base := time.Now().UTC().Add(-4 * time.Second)
	fixture := testkit.WorkspaceReadCommandPublicationV2(base, "refresh-cas")
	database := filepath.Join(t.TempDir(), "sandbox.db")
	stores := make([]*Store, 8)
	for index := range stores {
		var err error
		stores[index], err = Open(ctx, database)
		if err != nil {
			t.Fatalf("open refresh handle %d: %v", index, err)
		}
	}
	defer func() {
		for _, store := range stores {
			_ = store.Close()
		}
	}()
	closure := newWorkspaceReadPublicationClosureV2(t, fixture, base.Add(time.Second))
	first, created, err := stores[0].ApplyWorkspaceReadCommandPublicationV2(ctx, closure.capability)
	if err != nil || !created {
		t.Fatalf("initial publication: created=%v err=%v", created, err)
	}
	beforeReplay := snapshotWorkspaceReadPublicationRowsV2(t, stores[1])
	if replay, replayCreated, replayErr := stores[1].ApplyWorkspaceReadCommandPublicationV2(
		ctx, closure.capability,
	); replayErr != nil || replayCreated || replay.Meta.Ref() != first.Meta.Ref() {
		t.Fatalf("initial lost-reply replay: created=%v current=%#v err=%v", replayCreated, replay, replayErr)
	}
	if afterReplay := snapshotWorkspaceReadPublicationRowsV2(t, stores[1]); fmt.Sprint(afterReplay) != fmt.Sprint(beforeReplay) {
		t.Fatalf("initial replay wrote rows: before=%v after=%v", beforeReplay, afterReplay)
	}

	type refreshCandidate struct {
		current    contract.WorkspaceReadCommandOwnerCurrentV2
		capability ownerworkspaceread.AuthorizedCommandPublicationV2
	}
	candidates := make([]refreshCandidate, len(stores))
	for index := range candidates {
		current, capability := newWorkspaceReadPublicationRefreshV2(
			t, closure, first, base.Add(2*time.Second+time.Duration(index)*time.Nanosecond),
		)
		candidates[index] = refreshCandidate{current: current, capability: capability}
	}
	var winners atomic.Int64
	var conflicts atomic.Int64
	winnerRefs := make(chan contract.Ref, 1)
	var wait sync.WaitGroup
	for index := range candidates {
		wait.Add(1)
		go func(store *Store, candidate refreshCandidate) {
			defer wait.Done()
			current, won, applyErr := store.ApplyWorkspaceReadCommandPublicationV2(ctx, candidate.capability)
			switch {
			case applyErr == nil && won:
				winners.Add(1)
				winnerRefs <- current.Meta.Ref()
			case errors.Is(applyErr, ports.ErrConflict):
				conflicts.Add(1)
			default:
				t.Errorf("refresh CAS result: created=%v current=%#v err=%v", won, current, applyErr)
			}
		}(stores[index], candidates[index])
	}
	wait.Wait()
	if winners.Load() != 1 || conflicts.Load() != int64(len(candidates)-1) {
		t.Fatalf("refresh winners=%d conflicts=%d", winners.Load(), conflicts.Load())
	}
	winnerRef := <-winnerRefs
	for _, store := range stores {
		if err = store.Close(); err != nil {
			t.Fatal(err)
		}
	}
	reopened, err := Open(ctx, database)
	if err != nil {
		t.Fatalf("restart after refresh CAS: %v", err)
	}
	defer reopened.Close()
	pointer, err := reopened.InspectStoredWorkspaceReadCommandOwnerCurrentByCommandV2(ctx, closure.command.Meta.Ref())
	if err != nil || pointer.Meta.Ref() != winnerRef {
		t.Fatalf("restart pointer=%#v winner=%#v err=%v", pointer.Meta.Ref(), winnerRef, err)
	}
	var winnerCapability ownerworkspaceread.AuthorizedCommandPublicationV2
	for _, candidate := range candidates {
		if candidate.current.Meta.Ref() == winnerRef {
			winnerCapability = candidate.capability
			break
		}
	}
	beforeReplay = snapshotWorkspaceReadPublicationRowsV2(t, reopened)
	if replay, replayCreated, replayErr := reopened.ApplyWorkspaceReadCommandPublicationV2(
		ctx, winnerCapability,
	); replayErr != nil || replayCreated || replay.Meta.Ref() != winnerRef {
		t.Fatalf("refresh lost-reply replay: created=%v current=%#v err=%v", replayCreated, replay, replayErr)
	}
	if afterReplay := snapshotWorkspaceReadPublicationRowsV2(t, reopened); fmt.Sprint(afterReplay) != fmt.Sprint(beforeReplay) {
		t.Fatalf("refresh replay wrote rows: before=%v after=%v", beforeReplay, afterReplay)
	}
}

func TestWorkspaceReadCommandPublicationV2RuntimeAttemptIsDurablyUnique(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 7, 31, 15, 10, 0, 0, time.UTC)
	fixture := testkit.WorkspaceReadCommandPublicationV2(base, "attempt-unique")
	clock := newWorkspaceReadPublicationClockV2(base.Add(2*time.Second), time.Nanosecond)
	store, database := openWorkspaceReadPublicationStoreV2(t, clock.Now)

	closures := make([]workspaceReadPublicationClosureV2, 64)
	for index := range closures {
		distinct := fixture
		distinct.Source = contract.CloneWorkspaceReadSourceCurrentProjectionV2(fixture.Source)
		distinct.Source.SourceCommand.ID = fmt.Sprintf("tool-workspace-read-command-attempt-unique-%02d", index)
		distinct.Source.SourceCommand.Digest = testkit.Ref(fmt.Sprintf("attempt-unique-source-%02d", index)).Digest
		distinct.Source.ProjectionDigest = ""
		var err error
		distinct.Source, err = contract.SealWorkspaceReadSourceCurrentProjectionV2(distinct.Source)
		if err != nil {
			t.Fatalf("seal distinct Source %d: %v", index, err)
		}
		closures[index] = newWorkspaceReadPublicationClosureV2(
			t,
			distinct,
			base.Add(time.Second+time.Duration(index)*time.Nanosecond),
		)
	}
	var winners atomic.Int64
	var conflicts atomic.Int64
	winnerRefs := make(chan contract.Ref, 1)
	var wait sync.WaitGroup
	for index := range closures {
		wait.Add(1)
		go func(closure workspaceReadPublicationClosureV2) {
			defer wait.Done()
			current, _, err := store.ApplyWorkspaceReadCommandPublicationV2(ctx, closure.capability)
			switch {
			case err == nil:
				winners.Add(1)
				select {
				case winnerRefs <- current.Publication:
				default:
				}
			case errors.Is(err, ports.ErrConflict):
				conflicts.Add(1)
			default:
				t.Errorf("same RuntimeAttempt Apply error=%v", err)
			}
		}(closures[index])
	}
	wait.Wait()
	if winners.Load() != 1 || conflicts.Load() != 63 {
		t.Fatalf("same RuntimeAttempt winners=%d conflicts=%d", winners.Load(), conflicts.Load())
	}
	winner := <-winnerRefs
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenWithClock(ctx, database, clock.Now)
	if err != nil {
		t.Fatalf("restart publication store: %v", err)
	}
	defer reopened.Close()
	if publication, inspectErr := reopened.InspectStoredWorkspaceReadCommandPublicationExactV2(
		ctx,
		winner,
	); inspectErr != nil || publication.Meta.Ref() != winner {
		t.Fatalf("restart winner Inspect: publication=%#v err=%v", publication, inspectErr)
	}
	for _, table := range []string{
		"workspace_read_command_current",
		"workspace_read_command_body_seal",
		"workspace_read_command_publication_v2",
		"workspace_read_command_owner_current_history_v2",
		"workspace_read_command_owner_current_pointer_v2",
	} {
		var rows int
		if err = reopened.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if rows != 1 {
			t.Fatalf("%s rows=%d want=1", table, rows)
		}
	}
}

func TestWorkspaceReadCommandPublicationV2PartialClosureNeverRepairs(t *testing.T) {
	mutations := map[string]string{
		"command":     `DELETE FROM workspace_read_command_current`,
		"publication": `DELETE FROM workspace_read_command_publication_v2`,
		"history":     `DELETE FROM workspace_read_command_owner_current_history_v2`,
		"pointer":     `DELETE FROM workspace_read_command_owner_current_pointer_v2`,
	}
	for name, mutation := range mutations {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			base := time.Date(2026, 7, 31, 15, 20, 0, 0, time.UTC)
			fixture := testkit.WorkspaceReadCommandPublicationV2(base, "partial-"+name)
			clock := newWorkspaceReadPublicationClockV2(base.Add(4*time.Second), 0)
			store, _ := openWorkspaceReadPublicationStoreV2(t, clock.Now)
			closure := newWorkspaceReadPublicationClosureV2(t, fixture, base.Add(time.Second))
			winner, _, err := store.ApplyWorkspaceReadCommandPublicationV2(ctx, closure.capability)
			if err != nil {
				t.Fatal(err)
			}
			_, refresh := newWorkspaceReadPublicationRefreshV2(
				t,
				closure,
				winner,
				base.Add(3*time.Second),
			)
			if _, err = store.db.ExecContext(ctx, mutation); err != nil {
				t.Fatal(err)
			}
			before := snapshotWorkspaceReadPublicationRowsV2(t, store)
			if _, _, err = store.ApplyWorkspaceReadCommandPublicationV2(
				ctx,
				refresh,
			); !errors.Is(err, ports.ErrConflict) {
				t.Fatalf("partial refresh error=%v", err)
			}
			if _, _, _, err = store.InspectStoredWorkspaceReadCommandTripleV2(
				ctx,
				closure.command.Meta.Ref(),
			); !errors.Is(err, ports.ErrConflict) {
				t.Fatalf("partial triple error=%v", err)
			}
			after := snapshotWorkspaceReadPublicationRowsV2(t, store)
			if fmt.Sprint(before) != fmt.Sprint(after) {
				t.Fatalf("partial closure was repaired: before=%v after=%v", before, after)
			}
		})
	}
}

func TestWorkspaceReadCommandPublicationV2RefreshRequiresCompleteHistory(t *testing.T) {
	mutations := map[string]func(*testing.T, *Store, contract.WorkspaceReadCommandOwnerCurrentV2, contract.WorkspaceReadCommandOwnerCurrentV2, workspaceReadPublicationClosureV2){
		"missing predecessor": func(t *testing.T, store *Store, first, _ contract.WorkspaceReadCommandOwnerCurrentV2, _ workspaceReadPublicationClosureV2) {
			_, err := store.db.Exec(`DELETE FROM workspace_read_command_owner_current_history_v2 WHERE revision=?`, first.Meta.Revision)
			if err != nil {
				t.Fatal(err)
			}
		},
		"corrupt predecessor": func(t *testing.T, store *Store, first, _ contract.WorkspaceReadCommandOwnerCurrentV2, _ workspaceReadPublicationClosureV2) {
			_, err := store.db.Exec(
				`UPDATE workspace_read_command_owner_current_history_v2 SET body=x'00'
				  WHERE revision=?`,
				first.Meta.Revision,
			)
			if err != nil {
				t.Fatal(err)
			}
		},
		"future orphan": func(t *testing.T, store *Store, _, second contract.WorkspaceReadCommandOwnerCurrentV2, closure workspaceReadPublicationClosureV2) {
			third, _ := newWorkspaceReadPublicationRefreshV2(
				t,
				closure,
				second,
				time.Unix(0, second.CheckedUnixNano).Add(time.Second),
			)
			tx, err := store.db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			if err = insertWorkspaceReadCommandOwnerCurrentTxV2(context.Background(), tx, third); err != nil {
				_ = tx.Rollback()
				t.Fatal(err)
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
		},
		"gap": func(t *testing.T, store *Store, _, second contract.WorkspaceReadCommandOwnerCurrentV2, _ workspaceReadPublicationClosureV2) {
			_, err := store.db.Exec(`DELETE FROM workspace_read_command_owner_current_history_v2 WHERE revision=?`, second.Meta.Revision)
			if err != nil {
				t.Fatal(err)
			}
		},
		"pointer rollback": func(t *testing.T, store *Store, first, _ contract.WorkspaceReadCommandOwnerCurrentV2, _ workspaceReadPublicationClosureV2) {
			_, err := store.db.Exec(
				`UPDATE workspace_read_command_owner_current_pointer_v2
				    SET current_id=?,current_revision=?,current_digest=?`,
				first.Meta.ID,
				first.Meta.Revision,
				first.Meta.Digest,
			)
			if err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			base := time.Date(2026, 7, 31, 15, 30, 0, 0, time.UTC)
			suffix := "history-" + strings.NewReplacer(" ", "-", "_", "-").Replace(name)
			fixture := testkit.WorkspaceReadCommandPublicationV2(base, suffix)
			clock := newWorkspaceReadPublicationClockV2(base.Add(4*time.Second), 0)
			store, _ := openWorkspaceReadPublicationStoreV2(t, clock.Now)
			closure := newWorkspaceReadPublicationClosureV2(t, fixture, base.Add(time.Second))
			first, _, err := store.ApplyWorkspaceReadCommandPublicationV2(ctx, closure.capability)
			if err != nil {
				t.Fatal(err)
			}
			second, refresh := newWorkspaceReadPublicationRefreshV2(
				t,
				closure,
				first,
				base.Add(2*time.Second),
			)
			if got, _, applyErr := store.ApplyWorkspaceReadCommandPublicationV2(
				ctx,
				refresh,
			); applyErr != nil || got.Meta.Ref() != second.Meta.Ref() {
				t.Fatalf("apply second current: got=%#v err=%v", got, applyErr)
			}
			mutate(t, store, first, second, closure)
			before := snapshotWorkspaceReadPublicationRowsV2(t, store)
			if _, _, err = store.ApplyWorkspaceReadCommandPublicationV2(
				ctx,
				refresh,
			); !errors.Is(err, ports.ErrConflict) {
				t.Fatalf("history drift replay error=%v", err)
			}
			after := snapshotWorkspaceReadPublicationRowsV2(t, store)
			if fmt.Sprint(before) != fmt.Sprint(after) {
				t.Fatalf("history drift was repaired: before=%v after=%v", before, after)
			}
		})
	}
}

func snapshotWorkspaceReadPublicationRowsV2(t *testing.T, store *Store) map[string]int {
	t.Helper()
	result := make(map[string]int)
	for _, table := range []string{
		"workspace_read_command_current",
		"workspace_read_command_body_seal",
		"workspace_read_command_publication_v2",
		"workspace_read_command_owner_current_history_v2",
		"workspace_read_command_owner_current_pointer_v2",
	} {
		var rows int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		result[table] = rows
	}
	return result
}

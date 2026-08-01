package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	ownerworkspaceread "github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/owner/workspaceread"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestWorkspaceReadPostActualV20QualificationTerminalRestartAndExpiry(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Unix(1_970_000_000, 0)
	clock := newPostActualClockV20(now)
	path := filepath.Join(t.TempDir(), "sandbox.db")
	store, err := OpenWithClock(ctx, path, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	fixture := postActualSQLiteQualificationV20(t, store, now, "restart")
	qualification, authorized := fixture.qualification, fixture.authorized
	winner, created, err := store.EnsureAuthorizedExecutionQualificationV2(ctx, authorized)
	if err != nil || !created || !reflect.DeepEqual(winner, qualification) {
		t.Fatalf("create qualification: created=%v err=%v\nwant=%#v\ngot=%#v", created, err, qualification, winner)
	}
	clock.Set(time.Unix(0, qualification.ExpiresUnixNano))
	if _, _, err = store.EnsureAuthorizedExecutionQualificationV2(ctx, authorized); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("expired qualification regained execution authority: %v", err)
	}
	if exact, inspectErr := store.InspectWorkspaceReadExecutionQualificationExactV2(ctx, qualification.Ref); inspectErr != nil || !reflect.DeepEqual(exact, qualification) {
		t.Fatalf("expired historical qualification disappeared: %#v err=%v", exact, inspectErr)
	}
	terminal := postActualSQLiteIndeterminateV20(t, qualification)
	terminalWinner, terminalCreated, err := createOrInspectWorkspaceReadTerminalForTestV20(ctx, store, terminal)
	if err != nil || !terminalCreated || !reflect.DeepEqual(terminalWinner, terminal) {
		t.Fatalf("create terminal after expiry: created=%v err=%v\nwant=%#v\ngot=%#v", terminalCreated, err, terminal, terminalWinner)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenWithClock(ctx, path, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if exact, inspectErr := reopened.InspectWorkspaceReadTerminalExactV2(ctx, terminal.Ref); inspectErr != nil || !reflect.DeepEqual(exact, terminal) {
		t.Fatalf("restart exact terminal: %#v err=%v", exact, inspectErr)
	}
	if byOrigin, inspectErr := reopened.InspectWorkspaceReadTerminalByOriginV2(ctx, qualification.OriginAttempt); inspectErr != nil || !reflect.DeepEqual(byOrigin, terminal) {
		t.Fatalf("restart origin terminal: %#v err=%v", byOrigin, inspectErr)
	}
	if replay, replayCreated, replayErr := createOrInspectWorkspaceReadTerminalForTestV20(ctx, reopened, terminal); replayErr != nil || replayCreated || !reflect.DeepEqual(replay, terminal) {
		t.Fatalf("restart terminal replay: created=%v err=%v got=%#v", replayCreated, replayErr, replay)
	}
}

func TestWorkspaceReadPostActualV20QualificationByOriginRecoveryIsExactAndDurable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Unix(1_970_000_050, 0)
	path := filepath.Join(t.TempDir(), "sandbox.db")
	store, err := OpenWithClock(ctx, path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	fixture := postActualSQLiteQualificationV20(t, store, now, "origin-recovery")
	qualification := fixture.qualification
	if _, created, createErr := store.EnsureAuthorizedExecutionQualificationV2(ctx, fixture.authorized); createErr != nil || !created {
		t.Fatalf("seed qualification: created=%v err=%v", created, createErr)
	}
	if got, inspectErr := store.InspectWorkspaceReadExecutionQualificationByOriginV2(ctx, qualification.OriginAttempt); inspectErr != nil || !reflect.DeepEqual(got, qualification) {
		t.Fatalf("live origin recovery: got=%#v err=%v", got, inspectErr)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenWithClock(ctx, path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got, inspectErr := reopened.InspectWorkspaceReadExecutionQualificationByOriginV2(ctx, qualification.OriginAttempt); inspectErr != nil || !reflect.DeepEqual(got, qualification) {
		t.Fatalf("restart origin recovery: got=%#v err=%v", got, inspectErr)
	}
	spliced := qualification.OriginAttempt
	spliced.Revision++
	spliced.Digest = postActualSQLiteDigestV20(t, "origin-coordinate-splice")
	if _, inspectErr := reopened.InspectWorkspaceReadExecutionQualificationByOriginV2(ctx, spliced); !errors.Is(inspectErr, ports.ErrConflict) {
		t.Fatalf("same ID spliced origin was not Conflict: %v", inspectErr)
	}
	missing := contract.WorkspaceReadAttemptRefV1{
		ID: "missing-origin", Revision: 1, Digest: postActualSQLiteDigestV20(t, "missing-origin"),
	}
	if _, inspectErr := reopened.InspectWorkspaceReadExecutionQualificationByOriginV2(ctx, missing); !errors.Is(inspectErr, ports.ErrNotFound) {
		t.Fatalf("absent origin was not NotFound: %v", inspectErr)
	}
	if _, err = reopened.db.ExecContext(ctx,
		`UPDATE workspace_read_execution_qualification_history_v2 SET body=x'00' WHERE qualification_id=?`,
		qualification.Ref.ID,
	); err != nil {
		t.Fatal(err)
	}
	if _, inspectErr := reopened.InspectWorkspaceReadExecutionQualificationByOriginV2(ctx, qualification.OriginAttempt); !errors.Is(inspectErr, ports.ErrConflict) {
		t.Fatalf("corrupt origin winner was not Conflict: %v", inspectErr)
	}
}

func TestWorkspaceReadPostActualV20QualificationByOriginEightHandles64Concurrent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Unix(1_970_000_075, 0)
	path := filepath.Join(t.TempDir(), "sandbox.db")
	seed, err := OpenWithClock(ctx, path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	fixture := postActualSQLiteQualificationV20(t, seed, now, "origin-concurrent")
	if _, created, createErr := seed.EnsureAuthorizedExecutionQualificationV2(ctx, fixture.authorized); createErr != nil || !created {
		t.Fatalf("seed qualification: created=%v err=%v", created, createErr)
	}
	if err = seed.Close(); err != nil {
		t.Fatal(err)
	}
	stores := make([]*Store, 8)
	for index := range stores {
		stores[index], err = OpenWithClock(ctx, path, func() time.Time { return now })
		if err != nil {
			t.Fatal(err)
		}
		defer stores[index].Close()
	}
	errs := make(chan error, 64)
	var wait sync.WaitGroup
	for index := 0; index < 64; index++ {
		wait.Add(1)
		go func(store *Store) {
			defer wait.Done()
			winner, inspectErr := store.InspectWorkspaceReadExecutionQualificationByOriginV2(ctx, fixture.qualification.OriginAttempt)
			if inspectErr != nil {
				errs <- inspectErr
				return
			}
			if !reflect.DeepEqual(winner, fixture.qualification) {
				errs <- ports.ErrConflict
			}
		}(stores[index%len(stores)])
	}
	wait.Wait()
	close(errs)
	for inspectErr := range errs {
		t.Fatal(inspectErr)
	}
}

func TestWorkspaceReadPostActualV20EightHandles64ConcurrentSingleWinner(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Unix(1_970_000_100, 0)
	clock := newPostActualClockV20(now)
	path := filepath.Join(t.TempDir(), "sandbox.db")
	seed, err := OpenWithClock(ctx, path, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	fixture := postActualSQLiteQualificationV20(t, seed, now, "concurrent")
	qualification, authorized := fixture.qualification, fixture.authorized
	if err = seed.Close(); err != nil {
		t.Fatal(err)
	}
	stores := make([]*Store, 8)
	for index := range stores {
		stores[index], err = OpenWithClock(ctx, path, clock.Now)
		if err != nil {
			t.Fatal(err)
		}
		defer stores[index].Close()
	}
	var created atomic.Int64
	errorsOut := make(chan error, 64)
	var wg sync.WaitGroup
	for index := 0; index < 64; index++ {
		wg.Add(1)
		go func(store *Store) {
			defer wg.Done()
			winner, didCreate, createErr := store.EnsureAuthorizedExecutionQualificationV2(ctx, authorized)
			if createErr != nil {
				errorsOut <- createErr
				return
			}
			if !reflect.DeepEqual(winner, qualification) {
				errorsOut <- errors.New("qualification winner drifted")
				return
			}
			if didCreate {
				created.Add(1)
			}
		}(stores[index%len(stores)])
	}
	wg.Wait()
	close(errorsOut)
	for createErr := range errorsOut {
		t.Fatal(createErr)
	}
	if got := created.Load(); got != 1 {
		t.Fatalf("qualification creators=%d want=1", got)
	}
	var rows int
	if err = stores[0].db.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspace_read_execution_qualification_history_v2`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("qualification rows=%d err=%v", rows, err)
	}
	terminal := postActualSQLiteIndeterminateV20(t, qualification)
	created.Store(0)
	errorsOut = make(chan error, 64)
	for index := 0; index < 64; index++ {
		wg.Add(1)
		go func(store *Store) {
			defer wg.Done()
			winner, didCreate, createErr := createOrInspectWorkspaceReadTerminalForTestV20(ctx, store, terminal)
			if createErr != nil {
				errorsOut <- createErr
				return
			}
			if !reflect.DeepEqual(winner, terminal) {
				errorsOut <- errors.New("terminal winner drifted")
				return
			}
			if didCreate {
				created.Add(1)
			}
		}(stores[index%len(stores)])
	}
	wg.Wait()
	close(errorsOut)
	for createErr := range errorsOut {
		t.Fatal(createErr)
	}
	if got := created.Load(); got != 1 {
		t.Fatalf("terminal creators=%d want=1", got)
	}
	if err = stores[0].db.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspace_read_terminal_history_v2`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("terminal rows=%d err=%v", rows, err)
	}
}

func TestWorkspaceReadPostActualV20BindingAndRowSplicesFailClosed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Unix(1_970_000_200, 0)
	path := filepath.Join(t.TempDir(), "sandbox.db")
	store, err := OpenWithClock(ctx, path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	fixture := postActualSQLiteQualificationV20(t, store, now, "splice")
	qualification, authorized := fixture.qualification, fixture.authorized
	bindingLookupDigest, err := ports.WorkspaceReadRuntimeAttemptDigestV2(qualification.RuntimeAttempt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.db.ExecContext(ctx, `UPDATE workspace_read_runtime_attempt_admission_binding_v2 SET authorization_digest=? WHERE runtime_attempt_digest=?`, runtimecore.DigestBytes([]byte("spliced-authorization")), bindingLookupDigest); err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.EnsureAuthorizedExecutionQualificationV2(ctx, authorized); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("binding splice reached qualification history: %v", err)
	}
	var rows int
	if err = store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspace_read_execution_qualification_history_v2`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("binding splice writes=%d err=%v", rows, err)
	}
}

func TestWorkspaceReadPostActualV20QualificationRereadsEverySandboxOwnedSource(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*testing.T, *Store, postActualSQLiteQualificationFixtureV20){
		"publication": func(t *testing.T, store *Store, fixture postActualSQLiteQualificationFixtureV20) {
			if _, err := store.db.Exec(`UPDATE workspace_read_command_publication_v2 SET semantic_digest=? WHERE publication_id=?`, postActualSQLiteDigestV20(t, "publication-splice"), fixture.publication.Meta.ID); err != nil {
				t.Fatal(err)
			}
		},
		"owner_current": func(t *testing.T, store *Store, fixture postActualSQLiteQualificationFixtureV20) {
			if _, err := store.db.Exec(`UPDATE workspace_read_command_owner_current_history_v2 SET body=x'00' WHERE current_id=?`, fixture.ownerCurrent.Meta.ID); err != nil {
				t.Fatal(err)
			}
		},
		"workspace": func(t *testing.T, store *Store, fixture postActualSQLiteQualificationFixtureV20) {
			if _, err := store.db.Exec(`UPDATE workspace_view_history SET body=x'00' WHERE view_id=?`, fixture.workspace.Meta.ID); err != nil {
				t.Fatal(err)
			}
		},
		"reservation": func(t *testing.T, store *Store, fixture postActualSQLiteQualificationFixtureV20) {
			if _, err := store.db.Exec(`UPDATE workspace_read_reservation SET body=x'00' WHERE reservation_id=?`, fixture.qualification.Reservation.ID); err != nil {
				t.Fatal(err)
			}
		},
		"origin_attempt": func(t *testing.T, store *Store, fixture postActualSQLiteQualificationFixtureV20) {
			if _, err := store.db.Exec(`UPDATE workspace_read_attempt_origin SET body=x'00' WHERE attempt_id=?`, fixture.qualification.OriginAttempt.ID); err != nil {
				t.Fatal(err)
			}
		},
	} {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			now := time.Unix(1_970_000_250, 0)
			store, err := OpenWithClock(ctx, filepath.Join(t.TempDir(), "sandbox.db"), func() time.Time { return now })
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			fixture := postActualSQLiteQualificationV20(t, store, now, "source-splice-"+name)
			mutate(t, store, fixture)
			if _, _, err = store.EnsureAuthorizedExecutionQualificationV2(ctx, fixture.authorized); !errors.Is(err, ports.ErrConflict) {
				t.Fatalf("%s splice reached Qualification history: %v", name, err)
			}
			var rows int
			if err = store.db.QueryRow(`SELECT COUNT(*) FROM workspace_read_execution_qualification_history_v2`).Scan(&rows); err != nil || rows != 0 {
				t.Fatalf("%s splice writes=%d err=%v", name, rows, err)
			}
		})
	}
}

func TestWorkspaceReadPostActualV20ExistingSchemaNeverRepairsDrift(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sandbox.db")
	now := time.Unix(1_970_000_300, 0)
	store, err := OpenWithClock(ctx, path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = raw.Exec(`DROP INDEX workspace_read_terminal_journal_v2`); err != nil {
		t.Fatal(err)
	}
	if err = raw.Close(); err != nil {
		t.Fatal(err)
	}
	if reopened, openErr := OpenWithClock(ctx, path, func() time.Time { return now }); openErr == nil {
		reopened.Close()
		t.Fatal("existing v20 drift was silently repaired")
	}
	raw, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var indexes int
	if err = raw.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='workspace_read_terminal_journal_v2'`).Scan(&indexes); err != nil || indexes != 0 {
		t.Fatalf("missing index was repaired: count=%d err=%v", indexes, err)
	}
}

func TestWorkspaceReadPostActualV20MigratesExactV19AndRejectsPartialNamespace(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Unix(1_970_000_400, 0)
	for _, testCase := range []struct {
		name    string
		partial bool
	}{
		{name: "exact-v19"},
		{name: "partial-v20-without-ledger", partial: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "sandbox.db")
			store, err := OpenWithClock(ctx, path, func() time.Time { return now })
			if err != nil {
				t.Fatal(err)
			}
			if err = store.Close(); err != nil {
				t.Fatal(err)
			}
			raw, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if testCase.partial {
				_, err = raw.Exec(`DROP TABLE workspace_read_post_actual_schema_v20; PRAGMA user_version=19`)
			} else {
				_, err = raw.Exec(`
					DROP TABLE workspace_read_terminal_history_v2;
					DROP TABLE workspace_read_execution_qualification_history_v2;
					DROP TABLE workspace_read_post_actual_schema_v20;
					PRAGMA user_version=19`)
			}
			if err != nil {
				raw.Close()
				t.Fatal(err)
			}
			if err = raw.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, openErr := OpenWithClock(ctx, path, func() time.Time { return now })
			if testCase.partial {
				if openErr == nil {
					reopened.Close()
					t.Fatal("partial v20 namespace was repaired during v19 upgrade")
				}
				return
			}
			if openErr != nil {
				t.Fatal(openErr)
			}
			defer reopened.Close()
			var version int
			if err = reopened.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != schemaVersion {
				t.Fatalf("user_version=%d err=%v", version, err)
			}
		})
	}
}

func TestWorkspaceReadPostActualV20DenormalizedRowDriftFailsClosedAfterRestart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Unix(1_970_000_500, 0)
	path := filepath.Join(t.TempDir(), "sandbox.db")
	store, err := OpenWithClock(ctx, path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	fixture := postActualSQLiteQualificationV20(t, store, now, "row-drift")
	qualification, authorized := fixture.qualification, fixture.authorized
	if _, _, err = store.EnsureAuthorizedExecutionQualificationV2(ctx, authorized); err != nil {
		t.Fatal(err)
	}
	if _, err = store.db.ExecContext(ctx, `UPDATE workspace_read_execution_qualification_history_v2 SET expected_runtime_current_digest=? WHERE qualification_id=?`, postActualSQLiteDigestV20(t, "spliced-runtime-current"), qualification.Ref.ID); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenWithClock(ctx, path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err = reopened.InspectWorkspaceReadExecutionQualificationExactV2(ctx, qualification.Ref); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("denormalized row drift remained readable: %v", err)
	}
}

func TestWorkspaceReadPostActualV20PublicInspectErrorClosure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Unix(1_970_000_600, 0)
	store, err := OpenWithClock(ctx, filepath.Join(t.TempDir(), "sandbox.db"), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	fixture := postActualSQLiteQualificationV20(t, store, now, "inspect-errors")
	if _, _, err = store.EnsureAuthorizedExecutionQualificationV2(ctx, fixture.authorized); err != nil {
		t.Fatal(err)
	}
	terminal := postActualSQLiteIndeterminateV20(t, fixture.qualification)
	if _, _, err = createOrInspectWorkspaceReadTerminalForTestV20(ctx, store, terminal); err != nil {
		t.Fatal(err)
	}
	splicedOrigin := terminal.OriginAttempt
	splicedOrigin.Digest = postActualSQLiteDigestV20(t, "spliced-origin")
	if _, err = store.InspectWorkspaceReadTerminalByOriginV2(ctx, splicedOrigin); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("same-ID origin splice did not fail Conflict: %v", err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = store.InspectWorkspaceReadTerminalExactV2(ctx, terminal.Ref); err == nil {
		t.Fatal("closed database terminal Inspect unexpectedly succeeded")
	} else {
		var domainErr *runtimecore.DomainError
		if !errors.As(err, &domainErr) || domainErr.Category != runtimecore.ErrorUnavailable {
			t.Fatalf("closed database error escaped public Unavailable closure: %T %v", err, err)
		}
	}
}

func TestWorkspaceReadPostActualV20ObservedTerminalUsesExactS2Closure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Unix(1_970_000_700, 0)
	store, err := OpenWithClock(ctx, filepath.Join(t.TempDir(), "sandbox.db"), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	fixture := postActualSQLiteQualificationV20(t, store, now, "observed")
	if _, _, err = store.EnsureAuthorizedExecutionQualificationV2(ctx, fixture.authorized); err != nil {
		t.Fatal(err)
	}
	terminal := postActualSQLiteObservedV20(t, store, fixture, now)
	winner, created, err := createOrInspectWorkspaceReadTerminalForTestV20(ctx, store, terminal)
	if err != nil || !created || !reflect.DeepEqual(winner, terminal) {
		t.Fatalf("create observed terminal: created=%v err=%v\nwant=%#v\ngot=%#v", created, err, terminal, winner)
	}
	if terminal.Observed.S2Proof.Observation.Digest == terminal.Observed.S2Proof.ProviderReceipt.ObservationDigest {
		t.Fatal("Sandbox Observation exact digest collapsed into Provider Observation digest domain")
	}
}

type postActualClockV20 struct {
	nano atomic.Int64
}

func newPostActualClockV20(now time.Time) *postActualClockV20 {
	clock := &postActualClockV20{}
	clock.nano.Store(now.UnixNano())
	return clock
}

func (c *postActualClockV20) Now() time.Time    { return time.Unix(0, c.nano.Load()) }
func (c *postActualClockV20) Set(now time.Time) { c.nano.Store(now.UnixNano()) }

type postActualSQLiteQualificationFixtureV20 struct {
	qualification  contract.WorkspaceReadExecutionQualificationV2
	binding        ports.WorkspaceReadAdmissionAttemptBindingV2
	authorized     ownerworkspaceread.AuthorizedExecutionQualificationV2
	publication    contract.WorkspaceReadCommandPublicationV2
	ownerCurrent   contract.WorkspaceReadCommandOwnerCurrentV2
	workspace      contract.WorkspaceView
	runtimeCurrent runtimeports.CurrentOperationDispatchEnforcementV4
	association    runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1
}

func postActualSQLiteQualificationV20(t *testing.T, store *Store, now time.Time, suffix string) postActualSQLiteQualificationFixtureV20 {
	t.Helper()
	authorityFixture := testkit.WorkspaceReadPostActualV2(now, "post-actual-"+suffix)
	closure := newWorkspaceReadPublicationClosureV2(t, authorityFixture.Publication, now)
	expires := time.Unix(0, minimumWorkspaceReadPostActualExpiryV20(
		closure.current.ExpiresUnixNano,
		authorityFixture.RuntimeCurrent.ExpiresUnixNano,
	))
	if _, err := store.CreateWorkspaceViewV1(context.Background(), closure.fixture.Workspace); err != nil {
		t.Fatalf("seed Workspace View: %v", err)
	}
	if _, _, err := store.ApplyWorkspaceReadCommandPublicationV2(context.Background(), closure.capability); err != nil {
		t.Fatalf("seed Command publication: %v", err)
	}
	runtimeAttempt := closure.fixture.Source.RuntimeAttempt
	queryBase := authorityFixture.QueryBaseV2(closure.command, closure.current.Meta.Ref())
	authorizationDigest := queryBase.Query.AuthorizationDigest
	association := queryBase.Association.Ref
	domain := queryBase.Domain
	stable := queryBase.Query.StableKeyDigest
	admission, err := runtimeports.SealControlledOperationProviderAdmissionReceiptRefV2(runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{
		ID: "admission-" + suffix, Revision: 1, StableKeyDigest: stable, Admitted: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	ttl, err := contract.SealWorkspaceReadTTLClosureV1(contract.WorkspaceReadTTLClosureV1{
		UnifiedNotAfterUnixNano: expires.UnixNano(), RuntimeEnforcementExpiresNano: expires.UnixNano(),
		AssociationExpiresUnixNano: expires.UnixNano(), CommandRequestedNotAfterNano: expires.UnixNano(),
		CommandExpiresUnixNano: expires.UnixNano(), WorkspaceViewExpiresUnixNano: expires.UnixNano(),
		WorkspaceLeaseExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	reservation, err := contract.SealWorkspaceReadReservationV1(contract.WorkspaceReadReservationV1{
		StableKeyDigest: string(stable), AuthorizationDigest: string(authorizationDigest),
		RequestDigest: closure.command.Meta.Digest, PayloadDigest: closure.command.SourceToolPayloadDigest,
		Command: closure.command.Meta.Ref(), WorkspaceView: closure.fixture.Workspace.Meta.Ref(),
		AttemptID: "workspace-attempt-" + suffix, TTLClosure: ttl,
	}, "reservation-"+suffix, now, expires)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := contract.SealWorkspaceReadAttemptV1(contract.WorkspaceReadAttemptV1{
		StableKeyDigest: string(stable), RequestDigest: reservation.RequestDigest,
		PayloadDigest: reservation.PayloadDigest, Reservation: reservation.Meta.Ref(),
		AdmissionReceipt: contract.WorkspaceReadReceiptBindingV1{
			ID: admission.ID, Revision: uint64(admission.Revision), Digest: string(admission.Digest),
			StableKeyDigest: string(admission.StableKeyDigest), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
		}, State: contract.WorkspaceReadStartedV1,
	}, reservation.AttemptID, 1, now, expires)
	if err != nil {
		t.Fatal(err)
	}
	origin := contract.WorkspaceReadAttemptRefV1{ID: attempt.Meta.ID, Revision: attempt.Meta.Revision, Digest: attempt.Meta.Digest}
	query := authorityFixture.QueryV2(queryBase, reservation.Meta.Ref(), origin, attempt.AdmissionReceipt)
	bindingV1, err := ports.SealWorkspaceReadAdmissionAttemptBindingV1(ports.WorkspaceReadAdmissionAttemptBindingV1{
		AdmissionReceipt: admission, Attempt: origin, Command: closure.command.Meta.Ref(),
		AuthorizationDigest: authorizationDigest, StableKeyDigest: stable,
		Association: association, DomainCommand: domain,
		CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, created, reserveErr := store.reserveWorkspaceReadV1(context.Background(), reservation, attempt, bindingV1, nil); reserveErr != nil || !created {
		t.Fatalf("seed workspace-read origin: created=%v err=%v", created, reserveErr)
	}
	binding, err := ports.SealWorkspaceReadAdmissionAttemptBindingV2(ports.WorkspaceReadAdmissionAttemptBindingV2{
		RuntimeAttempt: runtimeAttempt, AdmissionBinding: bindingV1,
		AuthorizationDigest: authorizationDigest, Association: association, DomainCommand: domain,
		WorkspaceReadCommand: closure.command, AdmissionReceipt: admission, WorkspaceReadAttempt: origin,
	})
	if err != nil {
		t.Fatal(err)
	}
	seedPostActualBindingV20(t, store, binding)
	tx, err := store.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	storedBinding, inspectErr := inspectWorkspaceReadAdmissionClosureForRuntimeAttemptTxV2(context.Background(), tx, runtimeAttempt)
	_ = tx.Rollback()
	if inspectErr != nil || !reflect.DeepEqual(storedBinding, binding) {
		t.Fatalf("seeded V2 binding is not exact-readable: err=%v\nwant=%#v\ngot=%#v", inspectErr, binding, storedBinding)
	}
	runtimeAttemptDigest, err := contract.WorkspaceReadSourceRuntimeAttemptDigestV2(runtimeAttempt)
	if err != nil {
		t.Fatal(err)
	}
	leaseDigest, err := contract.WorkspaceReadRuntimeLeaseDigestV2(closure.fixture.Workspace.Lease)
	if err != nil {
		t.Fatal(err)
	}
	qualification, err := contract.SealWorkspaceReadExecutionQualificationV2(contract.WorkspaceReadExecutionQualificationV2{
		OriginAttempt: origin,
		Reservation:   reservation.Meta.Ref(),
		AdmissionReceipt: contract.WorkspaceReadReceiptBindingV1{
			ID: admission.ID, Revision: uint64(admission.Revision), Digest: string(admission.Digest),
			StableKeyDigest: string(admission.StableKeyDigest), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
		},
		RuntimeAdmissionReceipt: admission, AdmissionAttemptBindingDigest: binding.Digest,
		RuntimeAttempt: runtimeAttempt, RuntimeAttemptDigest: runtimeAttemptDigest,
		AuthorizationDigest: authorizationDigest, Association: association,
		Command: closure.command.Meta.Ref(), CommandPublication: closure.publication.Meta.Ref(),
		CommandOwnerCurrent: closure.current.Meta.Ref(), WorkspaceView: closure.fixture.Workspace.Meta.Ref(),
		WorkspaceLeaseDigest:         leaseDigest,
		CurrentQueryDigest:           query.Digest,
		ExpectedRuntimeCurrentDigest: authorityFixture.RuntimeCurrent.Digest,
		ActualRequestDigest:          postActualSQLiteDigestV20(t, "actual-request-"+suffix),
		PayloadDigest:                postActualSQLiteDigestV20(t, "actual-provider-payload-"+suffix),
		S1CheckedUnixNano:            now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if qualification.PayloadDigest == closure.command.SourceToolPayloadDigest {
		t.Fatal("actual Provider payload digest collapsed into the Source Tool payload digest domain")
	}
	prepared, err := ownerworkspaceread.AuthorizePreparedWorkspaceReadRequestProofV2(
		qualification.RuntimeAttempt.AttemptID,
		qualification.ActualRequestDigest,
		qualification.PayloadDigest,
		qualification.ExpiresUnixNano,
	)
	if err != nil {
		t.Fatal(err)
	}
	authorized, err := ownerworkspaceread.AuthorizeExecutionQualificationV2(
		qualification,
		binding,
		reservation,
		attempt,
		closure.publication,
		closure.current,
		closure.fixture.Workspace,
		query,
		authorityFixture.RuntimeCurrent,
		prepared,
	)
	if err != nil {
		t.Fatal(err)
	}
	return postActualSQLiteQualificationFixtureV20{
		qualification:  qualification,
		binding:        binding,
		authorized:     authorized,
		publication:    closure.publication,
		ownerCurrent:   closure.current,
		workspace:      closure.fixture.Workspace,
		runtimeCurrent: authorityFixture.RuntimeCurrent,
		association:    queryBase.Association,
	}
}

func minimumWorkspaceReadPostActualExpiryV20(values ...int64) int64 {
	minimum := int64(0)
	for _, value := range values {
		if minimum == 0 || value < minimum {
			minimum = value
		}
	}
	return minimum
}

func seedPostActualBindingV20(t *testing.T, store *Store, binding ports.WorkspaceReadAdmissionAttemptBindingV2) {
	t.Helper()
	body, err := encode(binding)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := ports.WorkspaceReadRuntimeAttemptDigestV2(binding.RuntimeAttempt)
	if err != nil {
		t.Fatal(err)
	}
	delegationPresent, delegationID, delegationRevision, delegationDigest := 0, "", uint64(0), ""
	if binding.RuntimeAttempt.Delegation != nil {
		delegationPresent, delegationID = 1, binding.RuntimeAttempt.Delegation.ID
		delegationRevision, delegationDigest = uint64(binding.RuntimeAttempt.Delegation.Revision), string(binding.RuntimeAttempt.Delegation.Digest)
	}
	_, err = store.db.Exec(`INSERT INTO workspace_read_runtime_attempt_admission_binding_v2(
		runtime_attempt_digest,operation_digest,effect_id,intent_revision,intent_digest,
		permit_id,permit_revision,permit_digest,runtime_attempt_id,
		delegation_present,delegation_id,delegation_revision,delegation_digest,
		authorization_digest,association_id,association_revision,association_digest,
		domain_command_id,domain_command_revision,domain_command_digest,
		command_id,command_revision,command_digest,admission_id,admission_revision,admission_digest,
		workspace_attempt_id,workspace_attempt_revision,workspace_attempt_digest,binding_digest,body)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		digest, binding.RuntimeAttempt.OperationDigest, binding.RuntimeAttempt.EffectID,
		binding.RuntimeAttempt.IntentRevision, binding.RuntimeAttempt.IntentDigest,
		binding.RuntimeAttempt.PermitID, binding.RuntimeAttempt.PermitRevision, binding.RuntimeAttempt.PermitDigest,
		binding.RuntimeAttempt.AttemptID, delegationPresent, delegationID, delegationRevision, delegationDigest,
		binding.AuthorizationDigest, binding.Association.ID, binding.Association.Revision, binding.Association.Digest,
		binding.DomainCommand.ID, binding.DomainCommand.Revision, binding.DomainCommand.Digest,
		binding.WorkspaceReadCommand.Meta.ID, binding.WorkspaceReadCommand.Meta.Revision, binding.WorkspaceReadCommand.Meta.Digest,
		binding.AdmissionReceipt.ID, binding.AdmissionReceipt.Revision, binding.AdmissionReceipt.Digest,
		binding.WorkspaceReadAttempt.ID, binding.WorkspaceReadAttempt.Revision, binding.WorkspaceReadAttempt.Digest,
		binding.Digest, body)
	if err != nil {
		t.Fatal(err)
	}
}

func postActualSQLiteIndeterminateV20(t *testing.T, qualification contract.WorkspaceReadExecutionQualificationV2) contract.WorkspaceReadTerminalFactV2 {
	t.Helper()
	journalTime := time.Unix(0, qualification.ExpiresUnixNano).Add(time.Second)
	journal := contract.WorkspaceReadPhysicalJournalRefV2{
		AttemptID: qualification.RuntimeAttempt.AttemptID, RequestDigest: qualification.ActualRequestDigest,
		PayloadDigest: qualification.PayloadDigest, Phase: contract.WorkspaceReadPhysicalJournalExecuteV2,
		State: contract.WorkspaceReadPhysicalJournalStartedV2, Revision: 1,
		RecordedUnixNano: journalTime.UnixNano(), RecordDigest: postActualSQLiteDigestV20(t, "journal-"+qualification.Ref.ID),
	}
	unknown, err := contract.SealWorkspaceReadIndeterminateEvidenceV2(
		qualification.Ref,
		qualification.OriginAttempt,
		journal,
		contract.WorkspaceReadIndeterminateErrorActualPointUnknownV2,
		postActualSQLiteDigestV20(t, "error-"+qualification.Ref.ID),
		journalTime.Add(time.Second).UnixNano(),
	)
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := contract.SealWorkspaceReadTerminalFactV2(contract.WorkspaceReadTerminalFactV2{
		Qualification: qualification.Ref, OriginAttempt: qualification.OriginAttempt,
		RuntimeAttempt: qualification.RuntimeAttempt, RuntimeAttemptDigest: qualification.RuntimeAttemptDigest,
		ActualRequestDigest: qualification.ActualRequestDigest, Journal: journal,
		Outcome:                      contract.WorkspaceReadTerminalIndeterminateV2,
		Indeterminate:                &contract.WorkspaceReadIndeterminateTerminalV2{Evidence: unknown},
		OutcomeCheckedUnixNano:       journalTime.Add(time.Second).UnixNano(),
		RecordedUnixNano:             journalTime.Add(2 * time.Second).UnixNano(),
		QualificationExpiresUnixNano: qualification.ExpiresUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	return terminal
}

func postActualSQLiteObservedV20(
	t *testing.T,
	store *Store,
	fixture postActualSQLiteQualificationFixtureV20,
	now time.Time,
) contract.WorkspaceReadTerminalFactV2 {
	t.Helper()
	qualification := fixture.qualification
	providerReceipt := contract.WorkspaceReadReceiptBindingV1{
		ID: "provider-receipt-" + qualification.Ref.ID, Revision: 1,
		Digest:            postActualSQLiteDigestV20(t, "provider-receipt-"+qualification.Ref.ID),
		ObservationDigest: postActualSQLiteDigestV20(t, "provider-observation-"+qualification.Ref.ID),
		StableKeyDigest:   qualification.AdmissionReceipt.StableKeyDigest,
		CheckedUnixNano:   now.Add(time.Second).UnixNano(), ExpiresUnixNano: qualification.ExpiresUnixNano,
	}
	content := "hello"
	fileID, err := contract.WorkspaceReadFileIDV1(fixture.workspace.Meta.ID, fixture.publication.Semantic.RelativePath)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := contract.SealWorkspaceReadObservationV1(contract.WorkspaceReadObservationV1{
		Reservation: qualification.Reservation, Command: qualification.Command, WorkspaceView: qualification.WorkspaceView,
		File:         contract.Ref{ID: fileID, Revision: fixture.workspace.Meta.Revision, Digest: postActualSQLiteDigestV20(t, "whole-file-"+qualification.Ref.ID)},
		RelativePath: fixture.publication.Semantic.RelativePath, StartByte: fixture.publication.Semantic.StartByte,
		ReturnedBytes: uint64(len(content)), TotalBytes: fixture.publication.Semantic.StartByte + uint64(len(content)), Complete: true,
		Content: content, ContentDigest: contract.WorkspaceReadContentDigestV1([]byte(content), fixture.publication.Semantic.StartByte, fixture.publication.Semantic.StartByte+uint64(len(content)), true),
		S1CheckedUnixNano: now.UnixNano(), S2CheckedUnixNano: now.Add(time.Second).UnixNano(),
		AdmissionReceipt: qualification.AdmissionReceipt, ProviderReceipt: providerReceipt,
	}, "observation-"+qualification.Ref.ID, now.Add(time.Second), time.Unix(0, qualification.ExpiresUnixNano))
	if err != nil {
		t.Fatal(err)
	}
	body, err := encode(observation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.db.Exec(`INSERT INTO workspace_read_observation(observation_id,stable_digest,body) VALUES(?,?,?)`, observation.Meta.ID, "post-actual-"+qualification.Ref.ID, body); err != nil {
		t.Fatal(err)
	}
	journal := contract.WorkspaceReadPhysicalJournalRefV2{
		AttemptID: qualification.RuntimeAttempt.AttemptID, RequestDigest: qualification.ActualRequestDigest,
		PayloadDigest: qualification.PayloadDigest, Phase: contract.WorkspaceReadPhysicalJournalExecuteV2,
		State: contract.WorkspaceReadPhysicalJournalCompletedV2, Revision: 2,
		RecordedUnixNano: now.Add(time.Second).UnixNano(), RecordDigest: postActualSQLiteDigestV20(t, "completed-journal-"+qualification.Ref.ID),
	}
	leaseDigest, err := contract.WorkspaceReadRuntimeLeaseDigestV2(fixture.workspace.Lease)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := contract.SealWorkspaceReadOutcomeS2ProofV2(contract.WorkspaceReadOutcomeS2ProofV2{
		Qualification: qualification.Ref, OriginAttempt: qualification.OriginAttempt,
		Association: fixture.association.Ref, CommandPublication: fixture.publication.Meta.Ref(),
		CommandOwnerCurrent: fixture.ownerCurrent.Meta.Ref(), WorkspaceView: fixture.workspace.Meta.Ref(),
		WorkspaceLeaseDigest: leaseDigest, RuntimeCurrentDigest: fixture.runtimeCurrent.Digest,
		Journal: journal, Observation: observation.Meta.Ref(), ProviderReceipt: providerReceipt,
		S2CheckedUnixNano: now.Add(2 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := contract.SealWorkspaceReadTerminalFactV2(contract.WorkspaceReadTerminalFactV2{
		Qualification: qualification.Ref, OriginAttempt: qualification.OriginAttempt,
		RuntimeAttempt: qualification.RuntimeAttempt, RuntimeAttemptDigest: qualification.RuntimeAttemptDigest,
		ActualRequestDigest: qualification.ActualRequestDigest, Journal: journal,
		Outcome:                      contract.WorkspaceReadTerminalObservedV2,
		Observed:                     &contract.WorkspaceReadObservedTerminalV2{S2Proof: s2},
		OutcomeCheckedUnixNano:       now.Add(2 * time.Second).UnixNano(),
		RecordedUnixNano:             now.Add(3 * time.Second).UnixNano(),
		QualificationExpiresUnixNano: qualification.ExpiresUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	return terminal
}

func createOrInspectWorkspaceReadTerminalForTestV20(
	ctx context.Context,
	store *Store,
	fact contract.WorkspaceReadTerminalFactV2,
) (contract.WorkspaceReadTerminalFactV2, bool, error) {
	for attempt := 0; attempt < 64; attempt++ {
		winner, created, err := store.createOrInspectWorkspaceReadTerminalOnceV2(ctx, fact)
		if err == nil || !workspaceReadCommandPublicationSQLiteBusyV2(err) {
			return winner, created, err
		}
		if err = waitWorkspaceReadPostActualRetryV20(ctx, attempt); err != nil {
			return contract.WorkspaceReadTerminalFactV2{}, false, err
		}
	}
	return contract.WorkspaceReadTerminalFactV2{}, false, errors.New("test terminal retry limit reached")
}

func postActualSQLiteDigestV20(t *testing.T, value string) string {
	t.Helper()
	digest, err := contract.Digest("workspace-read-post-actual-sqlite-v20-test", value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

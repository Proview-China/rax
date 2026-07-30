package routegateway_test

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/routegateway"
	modelsqlite "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/storage/sqlite"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestGovernedModelTurnV2ToolCallIsAtomicAndRestartReadable(t *testing.T) {
	path := t.TempDir() + "/turn-v2.db"
	fixture := newGovernedFixtureV2(t, path, nil)
	result, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
	if err != nil || result.State != modelinvoker.GovernedModelTurnObservedV2 || result.Observation == nil || result.Observation.ToolCallProjection == nil {
		t.Fatalf("governed model turn = %#v, %v", result, err)
	}
	if fixture.state.invoke.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1", fixture.state.invoke.Load())
	}
	projectionRef := result.Observation.ToolCallProjection.Ref
	if projectionRef.InvocationID != fixture.prepared.InvocationID || projectionRef.InvocationDigest != fixture.prepared.InvocationDigest {
		t.Fatalf("projection lineage = %#v, prepared = %#v", projectionRef, fixture.prepared.Ref())
	}
	if err := fixture.gateway.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := modelsqlite.Open(context.Background(), modelsqlite.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exact, err := store.InspectExactGovernedModelTurnV2(context.Background(), result.RefV2())
	if err != nil || exact.RefV2() != result.RefV2() {
		t.Fatalf("restart exact turn = %#v, %v", exact, err)
	}
	projection, err := store.InspectExactGovernedModelTurnToolCallProjectionV2(context.Background(), projectionRef)
	if err != nil || projection.Ref != projectionRef {
		t.Fatalf("restart exact projection = %#v, %v", projection, err)
	}
}

func TestGovernedModelTurnV2IdempotentCASRequiresExactExpectedHistory(t *testing.T) {
	fixture := newGovernedFixtureV2(t, t.TempDir()+"/turn-v2.db", nil)
	defer fixture.close(t)
	current, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
	if err != nil || current.State != modelinvoker.GovernedModelTurnObservedV2 || current.Observation == nil || current.Observation.ToolCallProjection == nil {
		t.Fatalf("governed model turn = %#v, %v", current, err)
	}
	forgedExpected := current.Observation.TurnRef
	forgedExpected.Digest = core.DigestBytes([]byte("forged-never-existing-expected"))
	if err := forgedExpected.Validate(); err != nil {
		t.Fatalf("forged syntactically valid Expected = %#v, %v", forgedExpected, err)
	}
	mutation, err := fixture.store.CompareAndSwapObservedGovernedModelTurnV2(context.Background(), modelinvoker.GovernedModelTurnCASV2{
		Expected: forgedExpected,
		Next:     current,
	})
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorConflict || mutation.Applied {
		t.Fatalf("forged idempotent CAS = %#v, %v", mutation, err)
	}
}

func TestGovernedModelTurnV2ReadsRejectSplicedOwnerIndexes(t *testing.T) {
	tests := []struct {
		name          string
		completedText bool
		mutate        func(*sql.DB, modelinvoker.GovernedModelTurnOutcomeV2) error
	}{
		{
			name: "history_gap",
			mutate: func(db *sql.DB, outcome modelinvoker.GovernedModelTurnOutcomeV2) error {
				_, err := db.ExecContext(context.Background(), `DELETE FROM governed_model_turn_history WHERE turn_id=? AND revision=2`, outcome.ID)
				return err
			},
		},
		{
			name: "attempt_guard_loss",
			mutate: func(db *sql.DB, outcome modelinvoker.GovernedModelTurnOutcomeV2) error {
				_, err := db.ExecContext(context.Background(), `DELETE FROM governed_model_turn_attempt_guard WHERE turn_id=?`, outcome.ID)
				return err
			},
		},
		{
			name: "attempt_guard_splice",
			mutate: func(db *sql.DB, outcome modelinvoker.GovernedModelTurnOutcomeV2) error {
				_, err := db.ExecContext(context.Background(), `UPDATE governed_model_turn_attempt_guard SET attempt_digest=? WHERE turn_id=?`, string(core.DigestBytes([]byte("spliced-attempt"))), outcome.ID)
				return err
			},
		},
		{
			name: "tool_call_projection_loss",
			mutate: func(db *sql.DB, outcome modelinvoker.GovernedModelTurnOutcomeV2) error {
				_, err := db.ExecContext(context.Background(), `DELETE FROM governed_model_turn_tool_call_projection WHERE turn_id=?`, outcome.ID)
				return err
			},
		},
		{
			name:          "completed_text_extra_projection",
			completedText: true,
			mutate: func(db *sql.DB, outcome modelinvoker.GovernedModelTurnOutcomeV2) error {
				digest := string(core.DigestBytes([]byte("extra-projection")))
				_, err := db.ExecContext(context.Background(), `INSERT INTO governed_model_turn_tool_call_projection(turn_id,turn_revision,projection_id,projection_revision,projection_digest,observation_digest,canonical_json) VALUES(?,?,?,?,?,?,?)`, outcome.ID, 2, "extra-completed-text-projection", 1, digest, digest, []byte(`{}`))
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := t.TempDir() + "/turn-v2.db"
			fixture := newGovernedFixtureV2(t, path, nil)
			closed := false
			defer func() {
				if !closed {
					fixture.close(t)
				}
			}()
			if test.completedText {
				fixture.state.governedV2CompletedText.Store(true)
			}
			outcome, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
			if err != nil || outcome.State != modelinvoker.GovernedModelTurnObservedV2 {
				t.Fatalf("governed model turn = %#v, %v", outcome, err)
			}
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if err := test.mutate(db, outcome); err != nil {
				_ = db.Close()
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			_, err = fixture.store.InspectExactGovernedModelTurnV2(context.Background(), outcome.RefV2())
			if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorConflict {
				t.Fatalf("live exact spliced read error = %v", err)
			}
			if err := fixture.gateway.Close(); err != nil {
				t.Fatal(err)
			}
			if err := fixture.store.Close(); err != nil {
				t.Fatal(err)
			}
			closed = true
			restarted, err := modelsqlite.Open(context.Background(), modelsqlite.Config{Path: path})
			if err != nil {
				t.Fatal(err)
			}
			defer restarted.Close()
			_, err = restarted.InspectExactGovernedModelTurnV2(context.Background(), outcome.RefV2())
			if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorConflict {
				t.Fatalf("restart exact spliced read error = %v", err)
			}
		})
	}
}

func TestGovernedModelTurnV2AllReadsRejectNonCanonicalHistoryBytes(t *testing.T) {
	for _, revision := range []int{1, 2, 3} {
		t.Run(fmt.Sprintf("revision_%d", revision), func(t *testing.T) {
			path := t.TempDir() + "/turn-v2.db"
			fixture := newGovernedFixtureV2(t, path, nil)
			outcome, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
			if err != nil || outcome.Observation == nil || outcome.Observation.ToolCallProjection == nil {
				t.Fatalf("governed model turn = %#v, %v", outcome, err)
			}
			attemptRef, err := modelinvoker.DeriveGovernedModelTurnAttemptRefV2(fixture.command)
			if err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			var payload []byte
			if err := db.QueryRowContext(context.Background(), `SELECT canonical_json FROM governed_model_turn_history WHERE turn_id=? AND revision=?`, outcome.ID, revision).Scan(&payload); err != nil {
				t.Fatal(err)
			}
			nonCanonical := append([]byte(" \n"), payload...)
			if _, err := db.ExecContext(context.Background(), `UPDATE governed_model_turn_history SET canonical_json=? WHERE turn_id=? AND revision=?`, nonCanonical, outcome.ID, revision); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			assertGovernedTurnCanonicalClosureRejectedV2(t, fixture.store, fixture.initial, attemptRef, outcome)
			if err := fixture.gateway.Close(); err != nil {
				t.Fatal(err)
			}
			if err := fixture.store.Close(); err != nil {
				t.Fatal(err)
			}
			restarted, err := modelsqlite.Open(context.Background(), modelsqlite.Config{Path: path})
			if err != nil {
				t.Fatal(err)
			}
			defer restarted.Close()
			assertGovernedTurnCanonicalClosureRejectedV2(t, restarted, fixture.initial, attemptRef, outcome)
		})
	}
}

func assertGovernedTurnCanonicalClosureRejectedV2(t *testing.T, store *modelsqlite.Store, initial modelinvoker.GovernedModelTurnOutcomeV2, attemptRef modelinvoker.GovernedModelTurnAttemptRefV2, outcome modelinvoker.GovernedModelTurnOutcomeV2) {
	t.Helper()
	ctx := context.Background()
	checks := []struct {
		name string
		call func() error
	}{
		{"exact", func() error { _, err := store.InspectExactGovernedModelTurnV2(ctx, outcome.RefV2()); return err }},
		{"current", func() error { _, err := store.InspectCurrentGovernedModelTurnV2(ctx, outcome.ID); return err }},
		{"attempt", func() error { _, err := store.InspectGovernedModelTurnAttemptV2(ctx, attemptRef); return err }},
		{"create", func() error { _, err := store.CreateGovernedModelTurnV2(ctx, initial); return err }},
		{"cas", func() error {
			_, err := store.CompareAndSwapObservedGovernedModelTurnV2(ctx, modelinvoker.GovernedModelTurnCASV2{Expected: outcome.Observation.TurnRef, Next: outcome})
			return err
		}},
		{"projection", func() error {
			_, err := store.InspectExactGovernedModelTurnToolCallProjectionV2(ctx, outcome.Observation.ToolCallProjection.Ref)
			return err
		}},
	}
	for _, check := range checks {
		if err := check.call(); modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorConflict {
			t.Fatalf("%s accepted non-canonical history: %v", check.name, err)
		}
	}
}

func TestGovernedModelTurnV2ConcurrentCallersOnlyOneProvider(t *testing.T) {
	fixture := newGovernedFixtureV2(t, t.TempDir()+"/turn-v2.db", nil)
	defer fixture.close(t)
	const workers = 64
	start := make(chan struct{})
	var wait sync.WaitGroup
	var observed atomic.Uint64
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			result, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
			if err == nil && result.State == modelinvoker.GovernedModelTurnObservedV2 {
				observed.Add(1)
			}
		}()
	}
	close(start)
	wait.Wait()
	if fixture.state.invoke.Load() != 1 || observed.Load() == 0 {
		t.Fatalf("provider calls/observed callers = %d/%d, want 1/>0", fixture.state.invoke.Load(), observed.Load())
	}
	current, err := fixture.store.InspectCurrentGovernedModelTurnV2(context.Background(), fixture.turnID)
	if err != nil || current.State != modelinvoker.GovernedModelTurnObservedV2 {
		t.Fatalf("current = %#v, %v", current, err)
	}
}

func TestGovernedModelTurnV2StableAttemptReplaySurvivesTTLRestartAndRejectsDifferentPayload(t *testing.T) {
	var clock atomic.Int64
	clock.Store(gatewayNow.UnixNano())
	path := t.TempDir() + "/turn-v2.db"
	fixture := newGovernedFixtureV2(t, path, []routegateway.Option{
		routegateway.WithClock(func() time.Time { return time.Unix(0, clock.Load()) }),
	})
	outcome, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
	if err != nil || outcome.State != modelinvoker.GovernedModelTurnObservedV2 || fixture.state.invoke.Load() != 1 {
		t.Fatalf("first stable attempt = %#v, %v provider=%d", outcome, err, fixture.state.invoke.Load())
	}
	attemptRef, err := modelinvoker.DeriveGovernedModelTurnAttemptRefV2(fixture.command)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := modelinvoker.DeriveGovernedModelTurnAttemptRefV2(fixture.command); err != nil || second != attemptRef {
		t.Fatalf("stable AttemptRef = %#v/%#v, %v", attemptRef, second, err)
	}
	clock.Store(gatewayNow.Add(24 * time.Hour).UnixNano())
	replayed, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
	if err != nil || replayed.RefV2() != outcome.RefV2() || fixture.state.invoke.Load() != 1 {
		t.Fatalf("expired same-command replay = %#v, %v provider=%d", replayed, err, fixture.state.invoke.Load())
	}
	inspected, err := fixture.gateway.InspectGovernedModelTurnAttemptV2(context.Background(), attemptRef)
	if err != nil || inspected.RefV2() != outcome.RefV2() {
		t.Fatalf("expired attempt Inspect = %#v, %v", inspected, err)
	}
	different := fixture.command
	different.CurrentRef.CheckedUnixNano++
	if _, err := modelinvoker.DeriveGovernedModelTurnAttemptRefV2(different); err != nil {
		t.Fatalf("different payload is not syntactically valid: %v", err)
	}
	if _, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), different); modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorConflict || fixture.state.invoke.Load() != 1 {
		t.Fatalf("different payload replay = %v provider=%d", err, fixture.state.invoke.Load())
	}
	if err := fixture.gateway.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.Close(); err != nil {
		t.Fatal(err)
	}
	restartedStore, err := modelsqlite.Open(context.Background(), modelsqlite.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer restartedStore.Close()
	restartedState := &callState{}
	restartedState.governedV2.Store(true)
	restartedGate := &governedGateV1{ack: fixture.ack}
	restartedGateway := fakeGateway(
		t, defaultCatalog(t),
		countingBinding{state: restartedState},
		countingSecret{state: restartedState, version: "v1"},
		restartedState,
		routegateway.WithClock(func() time.Time { return time.Unix(0, clock.Load()) }),
		routegateway.WithGovernedModelTurnsV2(routegateway.GovernedModelTurnDependenciesV2{
			PreparedHistory: restartedStore, PreparedCurrent: restartedStore, CommitGate: restartedGate, Materials: restartedStore, Turns: restartedStore,
		}),
	)
	defer restartedGateway.Close()
	restarted, err := restartedGateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
	if err != nil || restarted.RefV2() != outcome.RefV2() || restartedState.invoke.Load() != 0 {
		t.Fatalf("restart stable replay = %#v, %v provider=%d", restarted, err, restartedState.invoke.Load())
	}
}

func TestGovernedModelTurnV2ReadSnapshotDoesNotReportHealthyObservedCASAsConflict(t *testing.T) {
	path := t.TempDir() + "/turn-v2.db"
	base, err := modelsqlite.Open(context.Background(), modelsqlite.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	repository := &blockingObservedCASRepositoryV2{inner: base, ready: make(chan struct{}), release: make(chan struct{})}
	fixture := newGovernedFixtureV2WithStore(t, base, repository, nil)
	defer fixture.close(t)
	result := make(chan error, 1)
	go func() {
		_, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
		result <- err
	}()
	<-repository.ready
	boundary, err := base.InspectCurrentGovernedModelTurnV2(context.Background(), fixture.turnID)
	if err != nil || boundary.State != modelinvoker.GovernedModelTurnProviderBoundaryCrossedV2 {
		t.Fatalf("boundary before observed CAS = %#v, %v", boundary, err)
	}
	const readers = 128
	start := make(chan struct{})
	errorsSeen := make(chan error, readers*2)
	var wait sync.WaitGroup
	for index := 0; index < readers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, exactErr := base.InspectExactGovernedModelTurnV2(context.Background(), boundary.RefV2())
			errorsSeen <- exactErr
			_, currentErr := base.InspectCurrentGovernedModelTurnV2(context.Background(), boundary.ID)
			errorsSeen <- currentErr
		}()
	}
	close(start)
	close(repository.release)
	wait.Wait()
	close(errorsSeen)
	if err := <-result; err != nil {
		t.Fatalf("observed CAS = %v", err)
	}
	for err := range errorsSeen {
		if err != nil {
			t.Fatalf("healthy concurrent journal read = %v", err)
		}
	}
}

func TestGovernedModelTurnV2RequestRouteDigestSpliceCallsNoProvider(t *testing.T) {
	fixture := newGovernedFixtureV2(t, t.TempDir()+"/turn-v2.db", nil)
	defer fixture.close(t)
	spliced := fixture.command
	spliced.AttemptRequestDigest, spliced.RouteCallDigest = spliced.RouteCallDigest, spliced.AttemptRequestDigest
	if _, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), spliced); err == nil {
		t.Fatal("request/route digest splice was accepted")
	}
	if fixture.state.invoke.Load() != 0 {
		t.Fatalf("provider calls after digest splice = %d, want 0", fixture.state.invoke.Load())
	}
}

func TestGovernedModelTurnV2OrdinaryCASCannotBypassAtomicToolProjection(t *testing.T) {
	path := t.TempDir() + "/turn-v2.db"
	base, err := modelsqlite.Open(context.Background(), modelsqlite.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	repository := &ordinaryObservedCASRepositoryV2{inner: base}
	fixture := newGovernedFixtureV2WithStore(t, base, repository, nil)
	defer fixture.close(t)
	result, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
	if err == nil || result.State != modelinvoker.GovernedModelTurnProviderBoundaryCrossedV2 || fixture.state.invoke.Load() != 1 {
		t.Fatalf("ordinary observed CAS bypass = %#v, %v provider=%d", result, err, fixture.state.invoke.Load())
	}
	current, inspectErr := base.InspectCurrentGovernedModelTurnV2(context.Background(), fixture.turnID)
	if inspectErr != nil || current.State != modelinvoker.GovernedModelTurnProviderBoundaryCrossedV2 {
		t.Fatalf("current after rejected bypass = %#v, %v", current, inspectErr)
	}
}

func TestGovernedModelTurnV2BoundaryLostReplyIsInspectOnly(t *testing.T) {
	path := t.TempDir() + "/turn-v2.db"
	base, err := modelsqlite.Open(context.Background(), modelsqlite.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	repository := &lostBoundaryReplyRepositoryV2{inner: base}
	fixture := newGovernedFixtureV2WithStore(t, base, repository, nil)
	defer fixture.close(t)
	result, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorIndeterminate || result.State != modelinvoker.GovernedModelTurnProviderBoundaryCrossedV2 || fixture.state.invoke.Load() != 0 {
		t.Fatalf("lost boundary reply = %#v, %v provider=%d", result, err, fixture.state.invoke.Load())
	}
	replayed, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorIndeterminate || replayed.RefV2() != result.RefV2() || fixture.state.invoke.Load() != 0 {
		t.Fatalf("lost boundary replay = %#v, %v provider=%d", replayed, err, fixture.state.invoke.Load())
	}
}

func TestGovernedModelTurnV2CreateLostReplyRecoversStableAttempt(t *testing.T) {
	path := t.TempDir() + "/turn-v2.db"
	base, err := modelsqlite.Open(context.Background(), modelsqlite.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	repository := &lostCreateReplyRepositoryV2{inner: base}
	fixture := newGovernedFixtureV2WithStore(t, base, repository, nil)
	defer fixture.close(t)
	result, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
	if err != nil || result.State != modelinvoker.GovernedModelTurnObservedV2 || fixture.state.invoke.Load() != 1 {
		t.Fatalf("lost create reply = %#v, %v provider=%d", result, err, fixture.state.invoke.Load())
	}
	replayed, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
	if err != nil || replayed.RefV2() != result.RefV2() || fixture.state.invoke.Load() != 1 {
		t.Fatalf("lost create replay = %#v, %v provider=%d", replayed, err, fixture.state.invoke.Load())
	}
}

func TestGovernedModelTurnV2ObservedLostReplyRecoversExact(t *testing.T) {
	path := t.TempDir() + "/turn-v2.db"
	base, err := modelsqlite.Open(context.Background(), modelsqlite.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	repository := &lostObservedReplyRepositoryV2{inner: base}
	fixture := newGovernedFixtureV2WithStore(t, base, repository, nil)
	defer fixture.close(t)
	result, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
	if err != nil || result.State != modelinvoker.GovernedModelTurnObservedV2 || fixture.state.invoke.Load() != 1 {
		t.Fatalf("lost observed reply = %#v, %v provider=%d", result, err, fixture.state.invoke.Load())
	}
	replayed, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
	if err != nil || replayed.RefV2() != result.RefV2() || fixture.state.invoke.Load() != 1 {
		t.Fatalf("observed replay = %#v, %v provider=%d", replayed, err, fixture.state.invoke.Load())
	}
}

func TestGovernedModelTurnV2ProviderInvalidOutputsNeverPublishCandidate(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*callState)
	}{
		{"unknown tool", func(state *callState) { state.governedV2UnknownTool.Store(true) }},
		{"multiple calls", func(state *callState) { state.governedV2MultipleCalls.Store(true) }},
		{"text and call", func(state *callState) { state.governedV2TextAndCall.Store(true) }},
		{"invalid arguments", func(state *callState) { state.governedV2InvalidArguments.Store(true) }},
		{"model drift", func(state *callState) { state.wrongModel.Store(true) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newGovernedFixtureV2(t, t.TempDir()+"/turn-v2.db", nil)
			defer fixture.close(t)
			test.mutate(fixture.state)
			result, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
			if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorIndeterminate || result.State != modelinvoker.GovernedModelTurnUnknownV2 || result.Observation != nil || fixture.state.invoke.Load() != 1 {
				t.Fatalf("invalid provider output = %#v, %v provider=%d", result, err, fixture.state.invoke.Load())
			}
			current, inspectErr := fixture.store.InspectCurrentGovernedModelTurnV2(context.Background(), fixture.turnID)
			if inspectErr != nil || current.State != modelinvoker.GovernedModelTurnUnknownV2 {
				t.Fatalf("inspect unknown = %#v, %v", current, inspectErr)
			}
		})
	}
}

func TestGovernedModelTurnV2ExpiryAndClockRegressionCallNoProvider(t *testing.T) {
	var clock atomic.Int64
	clock.Store(gatewayNow.UnixNano())
	fixture := newGovernedFixtureV2(t, t.TempDir()+"/turn-v2.db", []routegateway.Option{
		routegateway.WithClock(func() time.Time { return time.Unix(0, clock.Load()) }),
	})
	defer fixture.close(t)
	fixture.gate.afterInspect = func() {
		if fixture.gate.inspect.Load() == 1 {
			clock.Store(fixture.ack.ExpiresUnixNano)
		}
	}
	result, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
	if err == nil || result.State != modelinvoker.GovernedModelTurnRejectedNoEffectV2 || fixture.state.invoke.Load() != 0 {
		t.Fatalf("expired S2 = %#v, %v provider=%d", result, err, fixture.state.invoke.Load())
	}
}

func TestGovernedModelTurnV2S3ReadsCrossTTLCallNoProvider(t *testing.T) {
	var clock atomic.Int64
	clock.Store(gatewayNow.UnixNano())
	fixture := newGovernedFixtureV2Configured(t, t.TempDir()+"/turn-v2.db", []routegateway.Option{
		routegateway.WithClock(func() time.Time { return time.Unix(0, clock.Load()) }),
	}, func(dependencies *routegateway.GovernedModelTurnDependenciesV2) {
		dependencies.PreparedCurrent = &advancingPreparedCurrentReaderV2{
			inner: dependencies.PreparedCurrent,
			at:    4,
			advance: func() {
				clock.Store(gatewayNow.Add(8 * time.Minute).UnixNano())
			},
		}
	})
	defer fixture.close(t)
	result, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorIndeterminate || result.State != modelinvoker.GovernedModelTurnUnknownV2 || fixture.state.invoke.Load() != 0 {
		t.Fatalf("S3 TTL crossing = %#v, %v provider=%d", result, err, fixture.state.invoke.Load())
	}
}

func TestGovernedModelTurnV2ShorterAckNarrowsExpiry(t *testing.T) {
	fixture := newGovernedFixtureV2(t, t.TempDir()+"/turn-v2.db", nil)
	defer fixture.close(t)
	result, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
	if err != nil || result.State != modelinvoker.GovernedModelTurnObservedV2 {
		t.Fatalf("shorter ACK turn = %#v, %v", result, err)
	}
	if result.ExpiresUnixNano != fixture.ack.ExpiresUnixNano {
		t.Fatalf("turn expiry = %d, want ACK expiry %d", result.ExpiresUnixNano, fixture.ack.ExpiresUnixNano)
	}
}

func TestGovernedModelTurnV2SealersAreAtomicAndIdempotent(t *testing.T) {
	fixture := newGovernedFixtureV2(t, t.TempDir()+"/turn-v2.db", nil)
	defer fixture.close(t)
	outcome, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
	if err != nil || outcome.Observation == nil {
		t.Fatalf("observed turn = %#v, %v", outcome, err)
	}

	resealedOutcome, err := modelinvoker.SealGovernedModelTurnOutcomeV2(outcome)
	if err != nil || !reflect.DeepEqual(resealedOutcome, outcome) {
		t.Fatalf("re-sealed outcome = %#v, %v", resealedOutcome, err)
	}
	invalidOutcome := outcome
	invalidOutcome.Digest = ""
	invalidOutcome.Revision = 0
	if sealed, sealErr := modelinvoker.SealGovernedModelTurnOutcomeV2(invalidOutcome); sealErr == nil || sealed != (modelinvoker.GovernedModelTurnOutcomeV2{}) {
		t.Fatalf("invalid outcome sealer leaked partial value = %#v, %v", sealed, sealErr)
	}

	observation := *outcome.Observation
	resealedObservation, err := modelinvoker.SealGovernedModelTurnObservationV2(observation)
	if err != nil || !reflect.DeepEqual(resealedObservation, observation) {
		t.Fatalf("re-sealed observation = %#v, %v", resealedObservation, err)
	}
	invalidObservation := observation
	invalidObservation.Digest = ""
	invalidObservation.Provider = ""
	if sealed, sealErr := modelinvoker.SealGovernedModelTurnObservationV2(invalidObservation); sealErr == nil || sealed != (modelinvoker.GovernedModelTurnObservationV2{}) {
		t.Fatalf("invalid observation sealer leaked partial value = %#v, %v", sealed, sealErr)
	}

	material, err := fixture.store.InspectExactInvocationMaterialV1(context.Background(), fixture.command.MaterialRef)
	if err != nil {
		t.Fatal(err)
	}
	resealedMaterial, err := modelinvoker.SealInvocationMaterialV1(material)
	if err != nil || !reflect.DeepEqual(resealedMaterial, material) {
		t.Fatalf("re-sealed material = %#v, %v", resealedMaterial, err)
	}
	invalidMaterial := material
	invalidMaterial.Digest = ""
	invalidMaterial.ExpiresUnixNano = invalidMaterial.CreatedUnixNano
	if sealed, sealErr := modelinvoker.SealInvocationMaterialV1(invalidMaterial); sealErr == nil || !reflect.DeepEqual(sealed, modelinvoker.InvocationMaterialV1{}) {
		t.Fatalf("invalid material sealer leaked partial value = %#v, %v", sealed, sealErr)
	}

	authorization := material.Authorization
	resealedAuthorization, err := modelinvoker.SealInvocationMaterialAuthorizationV1(authorization)
	if err != nil || !reflect.DeepEqual(resealedAuthorization, authorization) {
		t.Fatalf("re-sealed authorization = %#v, %v", resealedAuthorization, err)
	}
	invalidAuthorization := authorization
	invalidAuthorization.Digest = ""
	invalidAuthorization.ExpiresUnixNano = invalidAuthorization.AuthorizedUnixNano
	if sealed, sealErr := modelinvoker.SealInvocationMaterialAuthorizationV1(invalidAuthorization); sealErr == nil || sealed != (modelinvoker.InvocationMaterialAuthorizationV1{}) {
		t.Fatalf("invalid authorization sealer leaked partial value = %#v, %v", sealed, sealErr)
	}
}

func TestInvocationMaterialOwnerS1S2BindsEveryFullExactSourceRef(t *testing.T) {
	fixture := newGovernedFixtureV2(t, t.TempDir()+"/turn-v2.db", nil)
	defer fixture.close(t)
	material, err := fixture.store.InspectExactInvocationMaterialV1(context.Background(), fixture.command.MaterialRef)
	if err != nil {
		t.Fatal(err)
	}
	current, err := fixture.store.InspectExactPreparedModelInvocationCurrentV1(context.Background(), fixture.command.CurrentRef)
	if err != nil {
		t.Fatal(err)
	}
	closure := modelinvoker.InvocationMaterialExactClosureV1{
		ContextFrame:      material.Authorization.ContextFrameRef,
		ToolSurface:       material.Authorization.ToolSurfaceRef,
		ProviderInjection: material.Authorization.ProviderInjectionRef,
		Route:             material.Authorization.RouteRef,
		Profile:           material.Authorization.ProfileRef,
	}
	for _, name := range []string{"context", "tool", "provider", "route", "profile"} {
		t.Run(name, func(t *testing.T) {
			readers := &invocationMaterialExactReadersV2{driftRole: name}
			authorizer, createErr := modelinvoker.NewInvocationMaterialAuthorizerV1(modelinvoker.InvocationMaterialAuthorizerConfigV1{
				ContextFrame: readers, ToolSurface: readers, ProviderInjection: readers, Route: readers, Profile: readers,
			})
			if createErr != nil {
				t.Fatal(createErr)
			}
			got, ensureErr := modelinvoker.AuthorizeAndEnsureInvocationMaterialV1(context.Background(), authorizer, fixture.store, fixture.prepared, current, material.Call, closure, func() time.Time { return gatewayNow })
			if ensureErr == nil || !reflect.DeepEqual(got, modelinvoker.InvocationMaterialV1{}) {
				t.Fatalf("S1/S2 exact %s drift accepted: %#v, %v", name, got, ensureErr)
			}
		})
	}
	if _, err := fixture.store.EnsureAuthorizedInvocationMaterialV1(context.Background(), modelinvoker.InvocationMaterialPersistRequestV1{}); err == nil {
		t.Fatal("zero opaque persist request was accepted")
	}
}

type governedFixtureV2 struct {
	gateway  *routegateway.Gateway
	store    *modelsqlite.Store
	state    *callState
	gate     *governedGateV1
	command  modelinvoker.GovernedModelTurnCommandV2
	initial  modelinvoker.GovernedModelTurnOutcomeV2
	prepared modelinvoker.PreparedModelInvocationFactV1
	ack      modelinvoker.PreparedModelInvocationCommitAckV1
	turnID   string
}

func newGovernedFixtureV2(t *testing.T, path string, options []routegateway.Option) governedFixtureV2 {
	return newGovernedFixtureV2Configured(t, path, options, nil)
}

func newGovernedFixtureV2Configured(t *testing.T, path string, options []routegateway.Option, configure func(*routegateway.GovernedModelTurnDependenciesV2)) governedFixtureV2 {
	t.Helper()
	store, err := modelsqlite.Open(context.Background(), modelsqlite.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	return newGovernedFixtureV2WithStoreConfigured(t, store, store, options, configure)
}

func newGovernedFixtureV2WithStore(t *testing.T, store *modelsqlite.Store, turns modelinvoker.GovernedModelTurnRepositoryV2, options []routegateway.Option) governedFixtureV2 {
	return newGovernedFixtureV2WithStoreConfigured(t, store, turns, options, nil)
}

func newGovernedFixtureV2WithStoreConfigured(t *testing.T, store *modelsqlite.Store, turns modelinvoker.GovernedModelTurnRepositoryV2, options []routegateway.Option, configure func(*routegateway.GovernedModelTurnDependenciesV2)) governedFixtureV2 {
	t.Helper()
	call := governedCallV2()
	tempState := &callState{}
	temp := fakeGateway(t, defaultCatalog(t), countingBinding{state: tempState}, countingSecret{state: tempState, version: "v1"}, tempState)
	resolution, err := temp.Resolve(context.Background(), call)
	if err != nil {
		t.Fatal(err)
	}
	if err := temp.Close(); err != nil {
		t.Fatal(err)
	}
	routeDigest, err := modelinvoker.DigestGovernedRouteSelectionV1(resolution.Route)
	if err != nil {
		t.Fatal(err)
	}
	requestDigest := digestV2(t, "unified request", call)
	toolsDigest := digestV2(t, "request tools", call.Request.Tools)
	prepared, err := modelinvoker.SealPreparedModelInvocationFactV1(modelinvoker.PreparedModelInvocationFactV1{
		InvocationID: "execution-governed-v2", InvocationDigest: requestDigest, UnifiedRequestDigest: requestDigest,
		RequestToolsDigest: toolsDigest, PreparedPlanDigest: digestV2(t, "prepared plan", call.Request.Input),
		RouteDigest: routeDigest, ProfileDigest: digestV2(t, "profile", call.Request.Model),
		ActualToolSurfaceDigest:       digestV2(t, "tool surface", call.Request.Tools),
		ActualProviderInjectionDigest: digestV2(t, "provider injection", call.RouteID),
		CapabilitySnapshotRef:         modelinvoker.PreparedModelInvocationCapabilitySnapshotRefV1{ContractVersion: "1.0.0", ID: "capability-v2", Revision: 1, Digest: digestV2(t, "capability", call.RouteID)},
		RegistrySnapshotRef:           runtimeports.RegistrySnapshotRefV1{Owner: ownerV2("registry", "model-v2"), ContractVersion: "1.0.0", ID: "registry-v2", Revision: 1, Digest: digestV2(t, "registry", call.RouteID)},
		CreatedUnixNano:               gatewayNow.Add(-2 * time.Minute).UnixNano(), NotAfterUnixNano: gatewayNow.Add(10 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err := modelinvoker.SealPreparedModelInvocationCurrentV1(modelinvoker.PreparedModelInvocationCurrentProjectionV1{
		Prepared: prepared.Ref(), CapabilitySnapshotRef: prepared.CapabilitySnapshotRef, RegistrySnapshotRef: prepared.RegistrySnapshotRef,
		ActualToolSurfaceDigest: prepared.ActualToolSurfaceDigest, ActualProviderInjectionDigest: prepared.ActualProviderInjectionDigest,
		CheckedUnixNano: gatewayNow.Add(-time.Minute).UnixNano(), ExpiresUnixNano: gatewayNow.Add(8 * time.Minute).UnixNano(), NotAfterUnixNano: prepared.NotAfterUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	ack, err := modelinvoker.SealPreparedModelInvocationCommitAckV1(modelinvoker.PreparedModelInvocationCommitAckV1{
		PreparedRef: prepared.Ref(), CurrentRef: current.Ref(),
		GateImplementationRef: modelinvoker.PreparedModelInvocationGateImplementationRefV1{Owner: ownerV2("host", "gate-v2"), ContractVersion: "1.0.0", ID: "gate-v2", Revision: 1, Digest: digestV2(t, "gate", prepared.Ref())},
		SurfaceBindingRef:     modelinvoker.PreparedModelInvocationSurfaceBindingRefV1{Owner: ownerV2("tool", "surface-v2"), ContractVersion: "1.0.0", ID: "surface-v2", Revision: 1, Digest: prepared.ActualToolSurfaceDigest},
		CheckedUnixNano:       gatewayNow.Add(-30 * time.Second).UnixNano(), ExpiresUnixNano: gatewayNow.Add(6 * time.Minute).UnixNano(), NotAfterUnixNano: prepared.NotAfterUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	closure := modelinvoker.InvocationMaterialExactClosureV1{
		ContextFrame:      exactSourceV2("context", "frame-v2", digestV2(t, "context", call.Request.Input)),
		ToolSurface:       exactSourceV2("tool", "surface-v2", prepared.ActualToolSurfaceDigest),
		ProviderInjection: exactSourceV2("model", "provider-v2", prepared.ActualProviderInjectionDigest),
		Route:             exactSourceV2("model", "route-v2", prepared.RouteDigest),
		Profile:           exactSourceV2("model", "profile-v2", prepared.ProfileDigest),
	}
	readers := &invocationMaterialExactReadersV2{}
	authorizer, err := modelinvoker.NewInvocationMaterialAuthorizerV1(modelinvoker.InvocationMaterialAuthorizerConfigV1{
		ContextFrame: readers, ToolSurface: readers, ProviderInjection: readers, Route: readers, Profile: readers,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.EnsurePreparedModelInvocationV1(context.Background(), prepared); err != nil {
		t.Fatal(err)
	}
	if _, err = store.EnsurePreparedModelInvocationCurrentV1(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	material, err := modelinvoker.AuthorizeAndEnsureInvocationMaterialV1(context.Background(), authorizer, store, prepared, current, call, closure, func() time.Time { return gatewayNow })
	if err != nil {
		t.Fatal(err)
	}
	state := &callState{}
	state.governedV2.Store(true)
	gate := &governedGateV1{ack: ack}
	dependencies := routegateway.GovernedModelTurnDependenciesV2{
		PreparedHistory: store, PreparedCurrent: store, CommitGate: gate, Materials: store, Turns: turns,
	}
	if configure != nil {
		configure(&dependencies)
	}
	allOptions := append([]routegateway.Option{}, options...)
	allOptions = append(allOptions, routegateway.WithGovernedModelTurnsV2(dependencies))
	gateway := fakeGateway(t, defaultCatalog(t), countingBinding{state: state}, countingSecret{state: state, version: "v1"}, state, allOptions...)
	command := modelinvoker.GovernedModelTurnCommandV2{
		PreparedRef: prepared.Ref(), CurrentRef: current.Ref(), MaterialRef: material.RefV1(),
		AttemptRequestDigest: prepared.UnifiedRequestDigest, RouteCallDigest: material.RouteCallDigest,
		DispatchSequence: 1, ProviderAttemptOrdinal: 1,
	}
	initial, err := modelinvoker.NewPreparedGovernedModelTurnV2(command, gatewayNow)
	if err != nil {
		t.Fatal(err)
	}
	return governedFixtureV2{gateway: gateway, store: store, state: state, gate: gate, command: command, initial: initial, prepared: prepared, ack: ack, turnID: initial.ID}
}

func (fixture governedFixtureV2) close(t *testing.T) {
	t.Helper()
	if err := fixture.gateway.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.Close(); err != nil {
		t.Fatal(err)
	}
}

func governedCallV2() modelinvoker.RouteCall {
	strict := true
	parallel := false
	return modelinvoker.RouteCall{
		RouteID: "openai.direct.payg.responses", Invocation: generalInvocation(),
		Request: modelinvoker.Request{
			Model: "gpt-5.5", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "read README")},
			Tools: []modelinvoker.Tool{{
				Name: "workspace.read", Description: "read one file",
				Parameters: []byte(`{"type":"object","required":["path"],"properties":{"path":{"type":"string"}},"additionalProperties":false}`),
				Strict:     &strict,
			}},
			ToolChoice:        modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceRequired},
			ParallelToolCalls: &parallel,
			Budget:            modelinvoker.Budget{MaxOutputTokens: 256, Timeout: time.Minute},
		},
	}
}

type invocationMaterialExactReadersV2 struct {
	driftRole                               string
	context, tool, provider, route, profile atomic.Int64
}

type advancingPreparedCurrentReaderV2 struct {
	inner   modelinvoker.PreparedModelInvocationCurrentReaderV1
	reads   atomic.Int64
	at      int64
	advance func()
}

func (reader *advancingPreparedCurrentReaderV2) InspectExactPreparedModelInvocationCurrentV1(ctx context.Context, ref modelinvoker.PreparedModelInvocationCurrentRefV1) (modelinvoker.PreparedModelInvocationCurrentProjectionV1, error) {
	current, err := reader.inner.InspectExactPreparedModelInvocationCurrentV1(ctx, ref)
	if err == nil && reader.reads.Add(1) == reader.at && reader.advance != nil {
		reader.advance()
	}
	return current, err
}

func (readers *invocationMaterialExactReadersV2) exact(role string, calls *atomic.Int64, ref modelinvoker.InvocationMaterialExactSourceRefV1) modelinvoker.InvocationMaterialExactSourceRefV1 {
	if calls.Add(1) == 2 && readers.driftRole == role {
		switch role {
		case "context":
			ref.ID += "-drift"
		case "tool":
			ref.Owner.ID = core.OwnerID(string(ref.Owner.ID) + "-drift")
		case "provider":
			ref.Kind += "-drift"
		case "route":
			ref.Revision++
		case "profile":
			ref.ID += "-drift"
		}
	}
	return ref
}

func (readers *invocationMaterialExactReadersV2) projection(role string, calls *atomic.Int64, ref modelinvoker.InvocationMaterialExactSourceRefV1) modelinvoker.InvocationMaterialExactSourceProjectionV1 {
	return modelinvoker.InvocationMaterialExactSourceProjectionV1{
		Ref:             readers.exact(role, calls, ref),
		CheckedUnixNano: gatewayNow.Add(-time.Minute).UnixNano(),
		ExpiresUnixNano: gatewayNow.Add(7 * time.Minute).UnixNano(),
	}
}

func (readers *invocationMaterialExactReadersV2) InspectExactInvocationContextFrameV1(_ context.Context, ref modelinvoker.InvocationMaterialExactSourceRefV1) (modelinvoker.InvocationMaterialExactSourceProjectionV1, error) {
	return readers.projection("context", &readers.context, ref), nil
}
func (readers *invocationMaterialExactReadersV2) InspectExactInvocationToolSurfaceV1(_ context.Context, ref modelinvoker.InvocationMaterialExactSourceRefV1) (modelinvoker.InvocationMaterialExactSourceProjectionV1, error) {
	return readers.projection("tool", &readers.tool, ref), nil
}
func (readers *invocationMaterialExactReadersV2) InspectExactInvocationProviderInjectionV1(_ context.Context, ref modelinvoker.InvocationMaterialExactSourceRefV1) (modelinvoker.InvocationMaterialExactSourceProjectionV1, error) {
	return readers.projection("provider", &readers.provider, ref), nil
}
func (readers *invocationMaterialExactReadersV2) InspectExactInvocationRouteV1(_ context.Context, ref modelinvoker.InvocationMaterialExactSourceRefV1) (modelinvoker.InvocationMaterialExactSourceProjectionV1, error) {
	return readers.projection("route", &readers.route, ref), nil
}
func (readers *invocationMaterialExactReadersV2) InspectExactInvocationProfileV1(_ context.Context, ref modelinvoker.InvocationMaterialExactSourceRefV1) (modelinvoker.InvocationMaterialExactSourceProjectionV1, error) {
	return readers.projection("profile", &readers.profile, ref), nil
}

func exactSourceV2(domain, id string, digest core.Digest) modelinvoker.InvocationMaterialExactSourceRefV1 {
	return modelinvoker.InvocationMaterialExactSourceRefV1{Owner: ownerV2(domain, id+"-owner"), Kind: domain + "-material", ID: id, Revision: 1, Digest: digest}
}

func ownerV2(domain, id string) core.OwnerRef {
	return core.OwnerRef{Domain: domain, ID: core.OwnerID(id)}
}

func digestV2(t *testing.T, label string, value any) core.Digest {
	t.Helper()
	label = strings.ReplaceAll(label, " ", "-")
	digest, err := core.CanonicalJSONDigest("praxis.model-invoker.tests", "v2", label, value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

type lostBoundaryReplyRepositoryV2 struct {
	inner modelinvoker.GovernedModelTurnRepositoryV2
	once  atomic.Bool
}

type lostCreateReplyRepositoryV2 struct {
	inner modelinvoker.GovernedModelTurnRepositoryV2
	once  atomic.Bool
}

type ordinaryObservedCASRepositoryV2 struct {
	inner modelinvoker.GovernedModelTurnRepositoryV2
}

func (repository *lostCreateReplyRepositoryV2) CreateGovernedModelTurnV2(ctx context.Context, outcome modelinvoker.GovernedModelTurnOutcomeV2) (modelinvoker.GovernedModelTurnMutationV2, error) {
	mutation, err := repository.inner.CreateGovernedModelTurnV2(ctx, outcome)
	if err == nil && repository.once.CompareAndSwap(false, true) {
		return modelinvoker.GovernedModelTurnMutationV2{}, &modelinvoker.GovernedModelInvocationErrorV1{Kind: modelinvoker.GovernedModelInvocationErrorIndeterminate, Operation: "create_turn", Message: "lost create reply"}
	}
	return mutation, err
}
func (repository *lostCreateReplyRepositoryV2) CompareAndSwapGovernedModelTurnV2(ctx context.Context, request modelinvoker.GovernedModelTurnCASV2) (modelinvoker.GovernedModelTurnMutationV2, error) {
	return repository.inner.CompareAndSwapGovernedModelTurnV2(ctx, request)
}
func (repository *lostCreateReplyRepositoryV2) CompareAndSwapObservedGovernedModelTurnV2(ctx context.Context, request modelinvoker.GovernedModelTurnCASV2) (modelinvoker.GovernedModelTurnMutationV2, error) {
	return repository.inner.CompareAndSwapObservedGovernedModelTurnV2(ctx, request)
}
func (repository *lostCreateReplyRepositoryV2) InspectGovernedModelTurnAttemptV2(ctx context.Context, ref modelinvoker.GovernedModelTurnAttemptRefV2) (modelinvoker.GovernedModelTurnOutcomeV2, error) {
	return repository.inner.InspectGovernedModelTurnAttemptV2(ctx, ref)
}
func (repository *lostCreateReplyRepositoryV2) InspectExactGovernedModelTurnV2(ctx context.Context, ref modelinvoker.GovernedModelTurnRefV2) (modelinvoker.GovernedModelTurnOutcomeV2, error) {
	return repository.inner.InspectExactGovernedModelTurnV2(ctx, ref)
}
func (repository *lostCreateReplyRepositoryV2) InspectCurrentGovernedModelTurnV2(ctx context.Context, id string) (modelinvoker.GovernedModelTurnOutcomeV2, error) {
	return repository.inner.InspectCurrentGovernedModelTurnV2(ctx, id)
}
func (repository *lostCreateReplyRepositoryV2) InspectExactGovernedModelTurnToolCallProjectionV2(ctx context.Context, ref modelinvoker.ToolCallCandidateObservationRefV1) (modelinvoker.ToolCallCandidateObservationProjectionV1, error) {
	return repository.inner.InspectExactGovernedModelTurnToolCallProjectionV2(ctx, ref)
}

type blockingObservedCASRepositoryV2 struct {
	inner   modelinvoker.GovernedModelTurnRepositoryV2
	ready   chan struct{}
	release chan struct{}
	once    sync.Once
}

func (repository *blockingObservedCASRepositoryV2) CreateGovernedModelTurnV2(ctx context.Context, outcome modelinvoker.GovernedModelTurnOutcomeV2) (modelinvoker.GovernedModelTurnMutationV2, error) {
	return repository.inner.CreateGovernedModelTurnV2(ctx, outcome)
}
func (repository *blockingObservedCASRepositoryV2) CompareAndSwapGovernedModelTurnV2(ctx context.Context, request modelinvoker.GovernedModelTurnCASV2) (modelinvoker.GovernedModelTurnMutationV2, error) {
	return repository.inner.CompareAndSwapGovernedModelTurnV2(ctx, request)
}
func (repository *blockingObservedCASRepositoryV2) CompareAndSwapObservedGovernedModelTurnV2(ctx context.Context, request modelinvoker.GovernedModelTurnCASV2) (modelinvoker.GovernedModelTurnMutationV2, error) {
	repository.once.Do(func() { close(repository.ready) })
	<-repository.release
	return repository.inner.CompareAndSwapObservedGovernedModelTurnV2(ctx, request)
}
func (repository *blockingObservedCASRepositoryV2) InspectGovernedModelTurnAttemptV2(ctx context.Context, ref modelinvoker.GovernedModelTurnAttemptRefV2) (modelinvoker.GovernedModelTurnOutcomeV2, error) {
	return repository.inner.InspectGovernedModelTurnAttemptV2(ctx, ref)
}
func (repository *blockingObservedCASRepositoryV2) InspectExactGovernedModelTurnV2(ctx context.Context, ref modelinvoker.GovernedModelTurnRefV2) (modelinvoker.GovernedModelTurnOutcomeV2, error) {
	return repository.inner.InspectExactGovernedModelTurnV2(ctx, ref)
}
func (repository *blockingObservedCASRepositoryV2) InspectCurrentGovernedModelTurnV2(ctx context.Context, id string) (modelinvoker.GovernedModelTurnOutcomeV2, error) {
	return repository.inner.InspectCurrentGovernedModelTurnV2(ctx, id)
}
func (repository *blockingObservedCASRepositoryV2) InspectExactGovernedModelTurnToolCallProjectionV2(ctx context.Context, ref modelinvoker.ToolCallCandidateObservationRefV1) (modelinvoker.ToolCallCandidateObservationProjectionV1, error) {
	return repository.inner.InspectExactGovernedModelTurnToolCallProjectionV2(ctx, ref)
}

func (repository *ordinaryObservedCASRepositoryV2) CreateGovernedModelTurnV2(ctx context.Context, outcome modelinvoker.GovernedModelTurnOutcomeV2) (modelinvoker.GovernedModelTurnMutationV2, error) {
	return repository.inner.CreateGovernedModelTurnV2(ctx, outcome)
}
func (repository *ordinaryObservedCASRepositoryV2) CompareAndSwapGovernedModelTurnV2(ctx context.Context, request modelinvoker.GovernedModelTurnCASV2) (modelinvoker.GovernedModelTurnMutationV2, error) {
	return repository.inner.CompareAndSwapGovernedModelTurnV2(ctx, request)
}
func (repository *ordinaryObservedCASRepositoryV2) CompareAndSwapObservedGovernedModelTurnV2(ctx context.Context, request modelinvoker.GovernedModelTurnCASV2) (modelinvoker.GovernedModelTurnMutationV2, error) {
	return repository.inner.CompareAndSwapGovernedModelTurnV2(ctx, request)
}
func (repository *ordinaryObservedCASRepositoryV2) InspectGovernedModelTurnAttemptV2(ctx context.Context, ref modelinvoker.GovernedModelTurnAttemptRefV2) (modelinvoker.GovernedModelTurnOutcomeV2, error) {
	return repository.inner.InspectGovernedModelTurnAttemptV2(ctx, ref)
}
func (repository *ordinaryObservedCASRepositoryV2) InspectExactGovernedModelTurnV2(ctx context.Context, ref modelinvoker.GovernedModelTurnRefV2) (modelinvoker.GovernedModelTurnOutcomeV2, error) {
	return repository.inner.InspectExactGovernedModelTurnV2(ctx, ref)
}
func (repository *ordinaryObservedCASRepositoryV2) InspectCurrentGovernedModelTurnV2(ctx context.Context, id string) (modelinvoker.GovernedModelTurnOutcomeV2, error) {
	return repository.inner.InspectCurrentGovernedModelTurnV2(ctx, id)
}
func (repository *ordinaryObservedCASRepositoryV2) InspectExactGovernedModelTurnToolCallProjectionV2(ctx context.Context, ref modelinvoker.ToolCallCandidateObservationRefV1) (modelinvoker.ToolCallCandidateObservationProjectionV1, error) {
	return repository.inner.InspectExactGovernedModelTurnToolCallProjectionV2(ctx, ref)
}

func (repository *lostBoundaryReplyRepositoryV2) CreateGovernedModelTurnV2(ctx context.Context, outcome modelinvoker.GovernedModelTurnOutcomeV2) (modelinvoker.GovernedModelTurnMutationV2, error) {
	return repository.inner.CreateGovernedModelTurnV2(ctx, outcome)
}
func (repository *lostBoundaryReplyRepositoryV2) CompareAndSwapGovernedModelTurnV2(ctx context.Context, request modelinvoker.GovernedModelTurnCASV2) (modelinvoker.GovernedModelTurnMutationV2, error) {
	mutation, err := repository.inner.CompareAndSwapGovernedModelTurnV2(ctx, request)
	if err == nil && request.Next.State == modelinvoker.GovernedModelTurnProviderBoundaryCrossedV2 && repository.once.CompareAndSwap(false, true) {
		return modelinvoker.GovernedModelTurnMutationV2{}, &modelinvoker.GovernedModelInvocationErrorV1{Kind: modelinvoker.GovernedModelInvocationErrorIndeterminate, Operation: "cas_boundary", Message: "lost boundary reply"}
	}
	return mutation, err
}
func (repository *lostBoundaryReplyRepositoryV2) CompareAndSwapObservedGovernedModelTurnV2(ctx context.Context, request modelinvoker.GovernedModelTurnCASV2) (modelinvoker.GovernedModelTurnMutationV2, error) {
	return repository.inner.CompareAndSwapObservedGovernedModelTurnV2(ctx, request)
}
func (repository *lostBoundaryReplyRepositoryV2) InspectGovernedModelTurnAttemptV2(ctx context.Context, ref modelinvoker.GovernedModelTurnAttemptRefV2) (modelinvoker.GovernedModelTurnOutcomeV2, error) {
	return repository.inner.InspectGovernedModelTurnAttemptV2(ctx, ref)
}
func (repository *lostBoundaryReplyRepositoryV2) InspectExactGovernedModelTurnV2(ctx context.Context, ref modelinvoker.GovernedModelTurnRefV2) (modelinvoker.GovernedModelTurnOutcomeV2, error) {
	return repository.inner.InspectExactGovernedModelTurnV2(ctx, ref)
}
func (repository *lostBoundaryReplyRepositoryV2) InspectCurrentGovernedModelTurnV2(ctx context.Context, id string) (modelinvoker.GovernedModelTurnOutcomeV2, error) {
	return repository.inner.InspectCurrentGovernedModelTurnV2(ctx, id)
}
func (repository *lostBoundaryReplyRepositoryV2) InspectExactGovernedModelTurnToolCallProjectionV2(ctx context.Context, ref modelinvoker.ToolCallCandidateObservationRefV1) (modelinvoker.ToolCallCandidateObservationProjectionV1, error) {
	return repository.inner.InspectExactGovernedModelTurnToolCallProjectionV2(ctx, ref)
}

type lostObservedReplyRepositoryV2 struct {
	inner modelinvoker.GovernedModelTurnRepositoryV2
	once  atomic.Bool
}

func (repository *lostObservedReplyRepositoryV2) CreateGovernedModelTurnV2(ctx context.Context, outcome modelinvoker.GovernedModelTurnOutcomeV2) (modelinvoker.GovernedModelTurnMutationV2, error) {
	return repository.inner.CreateGovernedModelTurnV2(ctx, outcome)
}
func (repository *lostObservedReplyRepositoryV2) CompareAndSwapGovernedModelTurnV2(ctx context.Context, request modelinvoker.GovernedModelTurnCASV2) (modelinvoker.GovernedModelTurnMutationV2, error) {
	return repository.inner.CompareAndSwapGovernedModelTurnV2(ctx, request)
}
func (repository *lostObservedReplyRepositoryV2) CompareAndSwapObservedGovernedModelTurnV2(ctx context.Context, request modelinvoker.GovernedModelTurnCASV2) (modelinvoker.GovernedModelTurnMutationV2, error) {
	mutation, err := repository.inner.CompareAndSwapObservedGovernedModelTurnV2(ctx, request)
	if err == nil && repository.once.CompareAndSwap(false, true) {
		return modelinvoker.GovernedModelTurnMutationV2{}, &modelinvoker.GovernedModelInvocationErrorV1{Kind: modelinvoker.GovernedModelInvocationErrorIndeterminate, Operation: "cas_observed", Message: "lost observed reply"}
	}
	return mutation, err
}
func (repository *lostObservedReplyRepositoryV2) InspectGovernedModelTurnAttemptV2(ctx context.Context, ref modelinvoker.GovernedModelTurnAttemptRefV2) (modelinvoker.GovernedModelTurnOutcomeV2, error) {
	return repository.inner.InspectGovernedModelTurnAttemptV2(ctx, ref)
}
func (repository *lostObservedReplyRepositoryV2) InspectExactGovernedModelTurnV2(ctx context.Context, ref modelinvoker.GovernedModelTurnRefV2) (modelinvoker.GovernedModelTurnOutcomeV2, error) {
	return repository.inner.InspectExactGovernedModelTurnV2(ctx, ref)
}
func (repository *lostObservedReplyRepositoryV2) InspectCurrentGovernedModelTurnV2(ctx context.Context, id string) (modelinvoker.GovernedModelTurnOutcomeV2, error) {
	return repository.inner.InspectCurrentGovernedModelTurnV2(ctx, id)
}
func (repository *lostObservedReplyRepositoryV2) InspectExactGovernedModelTurnToolCallProjectionV2(ctx context.Context, ref modelinvoker.ToolCallCandidateObservationRefV1) (modelinvoker.ToolCallCandidateObservationProjectionV1, error) {
	return repository.inner.InspectExactGovernedModelTurnToolCallProjectionV2(ctx, ref)
}

var _ modelinvoker.InvocationMaterialContextFrameExactReaderV1 = (*invocationMaterialExactReadersV2)(nil)
var _ modelinvoker.InvocationMaterialToolSurfaceExactReaderV1 = (*invocationMaterialExactReadersV2)(nil)
var _ modelinvoker.InvocationMaterialProviderInjectionExactReaderV1 = (*invocationMaterialExactReadersV2)(nil)
var _ modelinvoker.InvocationMaterialRouteExactReaderV1 = (*invocationMaterialExactReadersV2)(nil)
var _ modelinvoker.InvocationMaterialProfileExactReaderV1 = (*invocationMaterialExactReadersV2)(nil)
var _ modelinvoker.GovernedModelTurnRepositoryV2 = (*lostBoundaryReplyRepositoryV2)(nil)
var _ modelinvoker.GovernedModelTurnRepositoryV2 = (*lostObservedReplyRepositoryV2)(nil)
var _ modelinvoker.GovernedModelTurnRepositoryV2 = (*ordinaryObservedCASRepositoryV2)(nil)

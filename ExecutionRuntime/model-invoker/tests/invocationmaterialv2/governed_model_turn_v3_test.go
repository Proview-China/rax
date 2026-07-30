package invocationmaterialv2_test

import (
	"bytes"
	"context"
	"database/sql"
	"reflect"
	"sync"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	modelsqlite "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/storage/sqlite"
)

type lostTurnReplyRepositoryV3 struct {
	inner        *modelsqlite.Store
	mu           sync.Mutex
	ensureCalls  int
	inspectCalls int
	ensured      modelinvoker.GovernedModelTurnAttemptRefV3
	inspected    modelinvoker.GovernedModelTurnAttemptRefV3
	deadline     time.Time
	hasDeadline  bool
}

type typedNilTurnRepositoryV3 struct{}

func (*typedNilTurnRepositoryV3) EnsurePreparedGovernedModelTurnV3(
	context.Context,
	modelinvoker.GovernedModelTurnOutcomeV3,
) (modelinvoker.GovernedModelTurnMutationV3, error) {
	panic("typed nil V3 repository must not be called")
}

func (*typedNilTurnRepositoryV3) InspectGovernedModelTurnAttemptV3(
	context.Context,
	modelinvoker.GovernedModelTurnAttemptRefV3,
) (modelinvoker.GovernedModelTurnOutcomeV3, error) {
	panic("typed nil V3 repository must not be called")
}

func (*typedNilTurnRepositoryV3) InspectExactGovernedModelTurnV3(
	context.Context,
	modelinvoker.GovernedModelTurnRefV3,
) (modelinvoker.GovernedModelTurnOutcomeV3, error) {
	panic("typed nil V3 repository must not be called")
}

func (r *lostTurnReplyRepositoryV3) EnsurePreparedGovernedModelTurnV3(
	ctx context.Context,
	outcome modelinvoker.GovernedModelTurnOutcomeV3,
) (modelinvoker.GovernedModelTurnMutationV3, error) {
	r.mu.Lock()
	r.ensureCalls++
	r.ensured = outcome.AttemptRefV3()
	r.mu.Unlock()
	if _, err := r.inner.EnsurePreparedGovernedModelTurnV3(ctx, outcome); err != nil {
		return modelinvoker.GovernedModelTurnMutationV3{}, err
	}
	return modelinvoker.GovernedModelTurnMutationV3{},
		&modelinvoker.GovernedModelInvocationErrorV1{
			Kind:      modelinvoker.GovernedModelInvocationErrorIndeterminate,
			Operation: "ensure_turn_v3",
			Message:   "simulated lost commit reply",
		}
}

func (r *lostTurnReplyRepositoryV3) InspectGovernedModelTurnAttemptV3(
	ctx context.Context,
	ref modelinvoker.GovernedModelTurnAttemptRefV3,
) (modelinvoker.GovernedModelTurnOutcomeV3, error) {
	r.mu.Lock()
	r.inspectCalls++
	r.inspected = ref
	if r.inspectCalls > 1 {
		r.deadline, r.hasDeadline = ctx.Deadline()
	}
	r.mu.Unlock()
	return r.inner.InspectGovernedModelTurnAttemptV3(ctx, ref)
}

func (r *lostTurnReplyRepositoryV3) InspectExactGovernedModelTurnV3(
	ctx context.Context,
	ref modelinvoker.GovernedModelTurnRefV3,
) (modelinvoker.GovernedModelTurnOutcomeV3, error) {
	return r.inner.InspectExactGovernedModelTurnV3(ctx, ref)
}

func TestGovernedModelTurnV3NativelyCarriesInvocationMaterialRefV2(t *testing.T) {
	for _, value := range []any{
		modelinvoker.GovernedModelTurnCommandV3{},
		modelinvoker.GovernedModelTurnAttemptRefV3{},
		modelinvoker.GovernedModelTurnRefV3{},
		modelinvoker.GovernedModelTurnOutcomeV3{},
	} {
		field, ok := reflect.TypeOf(value).FieldByName("MaterialRef")
		if !ok {
			t.Fatalf("%T has no MaterialRef", value)
		}
		if field.Type != reflect.TypeOf(modelinvoker.InvocationMaterialRefV2{}) {
			t.Fatalf("%T carries %v, want InvocationMaterialRefV2", value, field.Type)
		}
	}
}

func TestGovernedModelTurnV3RejectsTypedNilRepository(t *testing.T) {
	var repository *typedNilTurnRepositoryV3
	if _, err := modelinvoker.NewGovernedModelTurnOwnerPortV3(
		modelinvoker.GovernedModelTurnOwnerPortConfigV3{
			Repository: repository,
			Clock:      func() time.Time { return fixtureNow },
		},
	); err == nil {
		t.Fatal("typed nil V3 repository was accepted")
	}
}

func TestGovernedModelTurnV3StrictCanonicalDecode(t *testing.T) {
	command := governedModelTurnCommandV3(t, newFixtureV2(t))
	outcome, err := modelinvoker.NewPreparedGovernedModelTurnV3(
		command,
		fixtureNow.Add(2*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := modelinvoker.EncodeGovernedModelTurnOutcomeV3(outcome)
	if err != nil {
		t.Fatal(err)
	}
	version := []byte(
		`"contract_version":"` + modelinvoker.GovernedModelTurnContractVersionV3 + `"`,
	)
	duplicate := bytes.Replace(
		wire,
		version,
		append(append([]byte(nil), version...), append([]byte(","), version...)...),
		1,
	)
	unknown := append([]byte(`{"unknown":true,`), wire[1:]...)
	trailing := append(append([]byte(nil), wire...), []byte(` {}`)...)
	for name, payload := range map[string][]byte{
		"duplicate": duplicate,
		"unknown":   unknown,
		"trailing":  trailing,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := modelinvoker.DecodeGovernedModelTurnOutcomeV3(payload); err == nil {
				t.Fatal("non-strict V3 JSON was accepted")
			}
		})
	}
}

func TestGovernedModelTurnV3ReplayUsesHistoricalWinnerAcrossClocks(t *testing.T) {
	command := governedModelTurnCommandV3(t, newFixtureV2(t))
	path := filepathV3(t)
	store := openStoreV2(t, path)
	defer store.Close()

	firstPort := ownerPortV3(t, store, fixtureNow.Add(2*time.Minute))
	first, err := firstPort.StartOrInspectGovernedModelTurnV3(
		context.Background(),
		command,
	)
	if err != nil {
		t.Fatal(err)
	}
	secondPort := ownerPortV3(t, store, fixtureNow.Add(3*time.Minute))
	second, err := secondPort.StartOrInspectGovernedModelTurnV3(
		context.Background(),
		command,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || first.RefV3() != second.RefV3() {
		t.Fatal("same command replay did not return the exact historical winner")
	}
	assertTurnHistoryCountV3(t, path, 1)
}

func TestGovernedModelTurnV3ConcurrentCreateOnce(t *testing.T) {
	command := governedModelTurnCommandV3(t, newFixtureV2(t))
	path := filepathV3(t)
	store := openStoreV2(t, path)
	defer store.Close()
	port := ownerPortV3(t, store, fixtureNow.Add(2*time.Minute))

	const workers = 64
	results := make(chan modelinvoker.GovernedModelTurnOutcomeV3, workers)
	failures := make(chan error, workers)
	var group sync.WaitGroup
	for index := 0; index < workers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			outcome, err := port.StartOrInspectGovernedModelTurnV3(
				context.Background(),
				command,
			)
			if err != nil {
				failures <- err
				return
			}
			results <- outcome
		}()
	}
	group.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	var winner modelinvoker.GovernedModelTurnRefV3
	for outcome := range results {
		if winner.ID == "" {
			winner = outcome.RefV3()
			continue
		}
		if outcome.RefV3() != winner {
			t.Fatal("concurrent create returned multiple V3 winners")
		}
	}
	assertTurnHistoryCountV3(t, path, 1)
}

func TestGovernedModelTurnV3LostReplyInspectsOriginalAttempt(t *testing.T) {
	command := governedModelTurnCommandV3(t, newFixtureV2(t))
	path := filepathV3(t)
	store := openStoreV2(t, path)
	defer store.Close()
	repository := &lostTurnReplyRepositoryV3{inner: store}
	port := ownerPortV3(t, repository, fixtureNow.Add(2*time.Minute))
	expected, err := modelinvoker.DeriveGovernedModelTurnAttemptRefV3(command)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := port.StartOrInspectGovernedModelTurnV3(
		context.Background(),
		command,
	)
	if err != nil {
		t.Fatal(err)
	}
	repository.mu.Lock()
	ensureCalls := repository.ensureCalls
	inspectCalls := repository.inspectCalls
	ensured := repository.ensured
	inspected := repository.inspected
	deadline := repository.deadline
	hasDeadline := repository.hasDeadline
	repository.mu.Unlock()
	if ensureCalls != 1 || inspectCalls != 2 {
		t.Fatalf("calls ensure=%d inspect=%d, want 1/2", ensureCalls, inspectCalls)
	}
	if ensured != expected || inspected != expected ||
		outcome.AttemptRefV3() != expected {
		t.Fatal("lost reply recovery changed the stable AttemptRef")
	}
	wantDeadline := time.Unix(0, minInt64V3(
		expected.CurrentRef.ExpiresUnixNano,
		expected.CurrentRef.NotAfterUnixNano,
		expected.MaterialRef.ExpiresUnixNano,
	))
	if !hasDeadline || !deadline.Equal(wantDeadline) {
		t.Fatalf("recovery deadline=%v/%v want %v", deadline, hasDeadline, wantDeadline)
	}
	assertTurnHistoryCountV3(t, path, 1)
}

func TestGovernedModelTurnV3RestartInspect(t *testing.T) {
	command := governedModelTurnCommandV3(t, newFixtureV2(t))
	path := filepathV3(t)
	first := openStoreV2(t, path)
	outcome, err := ownerPortV3(
		t,
		first,
		fixtureNow.Add(2*time.Minute),
	).StartOrInspectGovernedModelTurnV3(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openStoreV2(t, path)
	defer reopened.Close()
	recovered, err := reopened.InspectGovernedModelTurnAttemptV3(
		context.Background(),
		outcome.AttemptRefV3(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(recovered, outcome) {
		t.Fatal("restart Inspect did not return the exact durable V3 outcome")
	}
}

func TestGovernedModelTurnV3UsesCurrentNotAfterAsTTL(t *testing.T) {
	fixture := newFixtureV2(t)
	fixture.current.NotAfterUnixNano = fixtureNow.Add(5 * time.Minute).UnixNano()
	fixture.current.ExpiresUnixNano = fixture.current.NotAfterUnixNano
	fixture.current.Digest = ""
	current, err := modelinvoker.SealPreparedModelInvocationCurrentV1(fixture.current)
	if err != nil {
		t.Fatal(err)
	}
	fixture.current = current
	command := governedModelTurnCommandV3(t, fixture)
	outcome, err := modelinvoker.NewPreparedGovernedModelTurnV3(
		command,
		fixtureNow.Add(2*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.ExpiresUnixNano != fixture.current.NotAfterUnixNano {
		t.Fatalf(
			"expiry=%d want Current.NotAfter=%d",
			outcome.ExpiresUnixNano,
			fixture.current.NotAfterUnixNano,
		)
	}
	if _, err := modelinvoker.NewPreparedGovernedModelTurnV3(
		command,
		time.Unix(0, fixture.current.NotAfterUnixNano),
	); err == nil {
		t.Fatal("clock at Current.NotAfter was accepted")
	}
	if _, err := modelinvoker.NewPreparedGovernedModelTurnV3(
		command,
		time.Unix(0, fixture.current.CheckedUnixNano-1),
	); err == nil {
		t.Fatal("clock regression before Current.Checked was accepted")
	}
}

func TestGovernedModelTurnV3ReplayRejectsClockRegression(t *testing.T) {
	command := governedModelTurnCommandV3(t, newFixtureV2(t))
	path := filepathV3(t)
	store := openStoreV2(t, path)
	defer store.Close()
	if _, err := ownerPortV3(
		t,
		store,
		fixtureNow.Add(2*time.Minute),
	).StartOrInspectGovernedModelTurnV3(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	_, err := ownerPortV3(
		t,
		store,
		time.Unix(0, command.CurrentRef.CheckedUnixNano-1),
	).StartOrInspectGovernedModelTurnV3(context.Background(), command)
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) !=
		modelinvoker.GovernedModelInvocationErrorConflict {
		t.Fatalf("error=%v, want Conflict", err)
	}
	assertTurnHistoryCountV3(t, path, 1)
}

func TestGovernedModelTurnV3EnsureSuccessRechecksFreshClock(t *testing.T) {
	command := governedModelTurnCommandV3(t, newFixtureV2(t))
	cases := []struct {
		name   string
		second time.Time
	}{
		{
			name:   "ttl crossing",
			second: time.Unix(0, command.MaterialRef.ExpiresUnixNano),
		},
		{
			name:   "clock regression",
			second: time.Unix(0, command.CurrentRef.CheckedUnixNano-1),
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			path := filepathV3(t)
			store := openStoreV2(t, path)
			defer store.Close()
			port := ownerPortClockV3(
				t,
				store,
				clockSequenceV2(fixtureNow.Add(2*time.Minute), test.second),
			)
			outcome, err := port.StartOrInspectGovernedModelTurnV3(
				context.Background(),
				command,
			)
			if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) !=
				modelinvoker.GovernedModelInvocationErrorConflict {
				t.Fatalf("error=%v, want Conflict", err)
			}
			if !reflect.DeepEqual(outcome, modelinvoker.GovernedModelTurnOutcomeV3{}) {
				t.Fatal("failed fresh-clock check returned a non-zero current outcome")
			}
			assertTurnHistoryCountV3(t, path, 1)
		})
	}
}

func TestGovernedModelTurnV3StoredCoordinatesFailClosed(t *testing.T) {
	command := governedModelTurnCommandV3(t, newFixtureV2(t))
	cases := []struct {
		name   string
		column string
		value  any
	}{
		{name: "fact digest", column: "fact_digest", value: digestV2("tampered-fact")},
		{name: "attempt digest", column: "attempt_digest", value: digestV2("tampered-attempt")},
		{name: "current digest", column: "current_digest", value: digestV2("tampered-current")},
		{name: "material digest", column: "material_digest", value: digestV2("tampered-material")},
		{name: "dispatch", column: "dispatch_sequence", value: 99},
		{name: "ordinal", column: "provider_attempt_ordinal", value: 99},
		{name: "expiry", column: "expires_unix_nano", value: fixtureNow.Add(9 * time.Minute).UnixNano()},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			path := filepathV3(t)
			store := openStoreV2(t, path)
			outcome, err := ownerPortV3(
				t,
				store,
				fixtureNow.Add(2*time.Minute),
			).StartOrInspectGovernedModelTurnV3(context.Background(), command)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(
				`UPDATE governed_model_turn_v3_history SET `+test.column+`=?`,
				test.value,
			); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			reopened := openStoreV2(t, path)
			defer reopened.Close()
			_, err = reopened.InspectExactGovernedModelTurnV3(
				context.Background(),
				outcome.RefV3(),
			)
			if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) !=
				modelinvoker.GovernedModelInvocationErrorConflict {
				t.Fatalf("error=%v, want Conflict", err)
			}
		})
	}
}

func TestGovernedModelTurnV3SchemaForbidsCurrentTable(t *testing.T) {
	path := filepathV3(t)
	store := openStoreV2(t, path)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`CREATE TABLE governed_model_turn_v3_current(turn_id TEXT PRIMARY KEY)`,
	); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := modelsqlite.Open(
		context.Background(),
		modelsqlite.Config{Path: path},
	)
	if reopened != nil {
		_ = reopened.Close()
	}
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) !=
		modelinvoker.GovernedModelInvocationErrorConflict {
		t.Fatalf("error=%v, want Conflict", err)
	}
}

func governedModelTurnCommandV3(
	t *testing.T,
	fixture fixtureV2,
) modelinvoker.GovernedModelTurnCommandV3 {
	t.Helper()
	authorization := directAuthorizationV2(t, fixture, fixture.closure.SourceLineage)
	material := directMaterialV2(t, fixture, authorization)
	return modelinvoker.GovernedModelTurnCommandV3{
		PreparedRef:            fixture.prepared.Ref(),
		CurrentRef:             fixture.current.Ref(),
		MaterialRef:            material.RefV2(),
		AttemptRequestDigest:   fixture.prepared.UnifiedRequestDigest,
		RouteCallDigest:        material.RouteCallDigest,
		DispatchSequence:       1,
		ProviderAttemptOrdinal: 1,
	}
}

func ownerPortV3(
	t *testing.T,
	repository modelinvoker.GovernedModelTurnRepositoryV3,
	now time.Time,
) *modelinvoker.GovernedModelTurnOwnerPortV3 {
	t.Helper()
	return ownerPortClockV3(t, repository, func() time.Time { return now })
}

func ownerPortClockV3(
	t *testing.T,
	repository modelinvoker.GovernedModelTurnRepositoryV3,
	clock func() time.Time,
) *modelinvoker.GovernedModelTurnOwnerPortV3 {
	t.Helper()
	port, err := modelinvoker.NewGovernedModelTurnOwnerPortV3(
		modelinvoker.GovernedModelTurnOwnerPortConfigV3{
			Repository: repository,
			Clock:      clock,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func filepathV3(t *testing.T) string {
	t.Helper()
	return t.TempDir() + "/model-invoker-v3.sqlite"
}

func assertTurnHistoryCountV3(t *testing.T, path string, expected int) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM governed_model_turn_v3_history`,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != expected {
		t.Fatalf("history count=%d want %d", count, expected)
	}
}

func minInt64V3(values ...int64) int64 {
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

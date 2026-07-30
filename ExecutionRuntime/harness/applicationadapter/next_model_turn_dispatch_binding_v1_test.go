package applicationadapter

import (
	"context"
	"database/sql"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	applicationconformance "github.com/Proview-China/rax/ExecutionRuntime/application/conformance"
	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type nextModelTurnDispatchReaderV1 struct {
	mu       sync.Mutex
	current  applicationcontract.TurnContinuationCurrentV1
	second   *applicationcontract.TurnContinuationCurrentV1
	err      error
	cancelAt int
	cancel   context.CancelFunc
	calls    int
}

func (r *nextModelTurnDispatchReaderV1) InspectTurnContinuationV1(
	_ context.Context,
	_ applicationcontract.TurnContinuationInspectRequestV1,
) (applicationcontract.TurnContinuationCurrentV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.cancelAt == r.calls && r.cancel != nil {
		r.cancel()
	}
	if r.err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, r.err
	}
	if r.calls == 2 && r.second != nil {
		return r.second.Clone(), nil
	}
	return r.current.Clone(), nil
}

type nextModelTurnDispatchClockV1 struct{ value atomic.Int64 }

func newNextModelTurnDispatchClockV1(now time.Time) *nextModelTurnDispatchClockV1 {
	clock := &nextModelTurnDispatchClockV1{}
	clock.Set(now)
	return clock
}

func (c *nextModelTurnDispatchClockV1) Now() time.Time {
	return time.Unix(0, c.value.Load())
}

func (c *nextModelTurnDispatchClockV1) Set(now time.Time) {
	c.value.Store(now.UnixNano())
}

type nextModelTurnDispatchFixtureV1 struct {
	now     time.Time
	request applicationcontract.NextModelTurnDispatchRequestV1
	current applicationcontract.TurnContinuationCurrentV1
	pending applicationcontract.TurnContinuationCurrentV1
}

func newNextModelTurnDispatchFixtureV1(t *testing.T) nextModelTurnDispatchFixtureV1 {
	t.Helper()
	now := time.Unix(1_910_000_000, 0).UTC()
	digest := func(value string) core.Digest { return core.DigestBytes([]byte(value)) }
	scope := core.ExecutionScope{
		Identity:       core.AgentIdentityRef{TenantID: "tenant-dispatch", ID: "agent-dispatch", Epoch: 1},
		Lineage:        core.LineageRef{ID: "lineage-dispatch", PlanDigest: digest("plan")},
		Instance:       core.InstanceRef{ID: "instance-dispatch", Epoch: 2},
		SandboxLease:   &core.SandboxLeaseRef{ID: "lease-dispatch", Epoch: 2},
		AuthorityEpoch: 1,
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	mustNextModelTurnDispatchV1(t, err)
	run := applicationcontract.SingleCallRunCoordinateV1{
		RunID: "run-dispatch", Revision: 4, Digest: digest("run-current"),
	}
	session := applicationcontract.SingleCallSessionCoordinateV1{
		ID: "session-dispatch", Revision: 3, Digest: digest("session"),
		Phase:           applicationcontract.SingleCallSessionWaitingActionV1,
		CheckedUnixNano: now.Add(-time.Second).UnixNano(),
		ExpiresUnixNano: now.Add(30 * time.Second).UnixNano(),
	}
	sourceTurn := applicationcontract.SingleCallTurnCoordinateV1{
		ID: "turn:" + string(digest("turn-3")), Ordinal: 3, Revision: 3, Digest: digest("turn-3"),
	}
	source, err := applicationcontract.SealContextTurnSourceCurrentV1(
		applicationcontract.ContextTurnSourceCurrentV1{
			ExecutionScopeDigest: scopeDigest,
			RunID:                run.RunID,
			Session:              session,
			SessionApplicability: applicationcontract.SingleCallSessionApplicabilitySourceCoordinateV1{
				Kind: applicationcontract.SingleCallSessionSourceKindV1,
				ID:   "session:" + string(session.Digest), Revision: session.Revision, Digest: session.Digest,
			},
			Turn: sourceTurn,
			TurnApplicability: applicationcontract.SingleCallTurnApplicabilitySourceCoordinateV1{
				Kind: applicationcontract.SingleCallTurnSourceKindV1,
				ID:   sourceTurn.ID, Revision: sourceTurn.Revision, Digest: sourceTurn.Digest,
			},
			CheckedUnixNano: session.CheckedUnixNano,
			ExpiresUnixNano: session.ExpiresUnixNano,
		},
	)
	mustNextModelTurnDispatchV1(t, err)
	contextRef := func(kind, id string) applicationcontract.ContextRefreshExactRefV1 {
		return applicationcontract.ContextRefreshExactRefV1{
			Kind: runtimeports.NamespacedNameV2(kind), ID: id, Revision: 1, Digest: digest(id),
		}
	}
	oldActive, err := applicationcontract.SealHarnessActiveContextRefV1(
		applicationcontract.HarnessActiveContextRefV1{
			Revision: 7, ExecutionScopeDigest: scopeDigest, RunID: run.RunID,
			SessionID: session.ID, TurnOrdinal: sourceTurn.Ordinal,
			ManifestRef:              contextRef("context/manifest", "manifest-3"),
			FrameRef:                 contextRef("context/frame", "frame-3"),
			GenerationRef:            contextRef("context/generation", "generation-3"),
			ContextCurrentPointerRef: contextRef("context/generation-current", "current-3"),
			UpdatedUnixNano:          now.Add(-2 * time.Second).UnixNano(),
		},
	)
	mustNextModelTurnDispatchV1(t, err)
	startInput := applicationcontract.TurnContinuationStartRequestV1{
		ExecutionScopeDigest: scopeDigest,
		RunID:                run.RunID,
		Source:               source,
		SettledToolResult: applicationcontract.SingleCallToolActionResultRefV2{
			ID: "result-dispatch", Revision: 1, Digest: digest("result"),
			RequestID: "request-dispatch", RequestRevision: 1, RequestDigest: digest("request"),
			ActionCoordinateDigest: digest("action"),
			ToolResultID:           "tool-result-dispatch", ToolResultRevision: 1, ToolResultDigest: digest("tool-result"),
		},
		ExpectedActiveContext:     oldActive,
		TargetTurn:                sourceTurn.Ordinal + 1,
		RequestedNotAfterUnixNano: now.Add(25 * time.Second).UnixNano(),
	}
	attemptID, err := applicationcontract.DeriveTurnContinuationAttemptIDV1(startInput)
	mustNextModelTurnDispatchV1(t, err)
	startInput.ExpectedContextRefreshAttempt = applicationcontract.ContextRefreshExactRefV1{
		Kind: applicationcontract.ContextTurnRefreshApplicationAttemptKindV1,
		ID:   attemptID, Revision: 1, Digest: digest("context-attempt"),
	}
	start, err := applicationcontract.SealTurnContinuationStartRequestV1(startInput)
	mustNextModelTurnDispatchV1(t, err)
	pending, err := applicationcontract.SealTurnContinuationPendingV1(
		start,
		now.UnixNano(),
		now.Add(20*time.Second).UnixNano(),
	)
	mustNextModelTurnDispatchV1(t, err)
	settlement := contextRef("context/apply-settlement", "apply-4")
	pointer := contextRef("context/generation-current", "current-4")
	refresh, err := applicationcontract.SealContextTurnRefreshResultV1(
		applicationcontract.ContextTurnRefreshResultV1{
			AttemptRef:             start.ExpectedContextRefreshAttempt,
			PendingDomainResultRef: contextRef("context/pending-result", "pending-4"),
			TransitionProofRef:     contextRef("context/transition-proof", "proof-4"),
			ManifestRef:            contextRef("context/manifest", "manifest-4"),
			FrameRef:               contextRef("context/frame", "frame-4"),
			GenerationRef:          contextRef("context/generation", "generation-4"),
			StableSourceSetDigest:  digest("stable"),
			S1AssociationSetDigest: digest("s1"),
			S2AssociationSetDigest: digest("s2"),
			ApplySettlementRef:     &settlement,
			CurrentPointerRef:      &pointer,
			CheckedUnixNano:        now.UnixNano(),
			ExpiresUnixNano:        now.Add(18 * time.Second).UnixNano(),
			State:                  applicationcontract.ContextTurnRefreshAppliedStateV1,
		},
	)
	mustNextModelTurnDispatchV1(t, err)
	commit, err := applicationcontract.SealTurnContinuationCommitRequestV1(
		applicationcontract.TurnContinuationCommitRequestV1{
			Pending: pending, AppliedContextRefresh: refresh,
			RequestedNotAfterUnixNano: now.Add(17 * time.Second).UnixNano(),
		},
		now,
	)
	mustNextModelTurnDispatchV1(t, err)
	current, err := applicationcontract.SealTurnContinuationContextCurrentV1(
		commit,
		now.UnixNano(),
		now.Add(17*time.Second).UnixNano(),
	)
	mustNextModelTurnDispatchV1(t, err)

	operation := runtimeports.OperationSubjectV3{
		Kind: runtimeports.OperationScopeRunV3, ExecutionScope: scope,
		ExecutionScopeDigest: scopeDigest, RunID: run.RunID,
		SubjectRevision: 1, CurrentProjectionRef: "run-current-dispatch",
		CurrentProjectionRevision: run.Revision, CurrentProjectionDigest: run.Digest,
	}
	operationDigest, err := operation.DigestV3()
	mustNextModelTurnDispatchV1(t, err)
	runtimeAttempt := runtimeports.OperationDispatchAttemptRefV3{
		OperationDigest: operationDigest, EffectID: "effect-dispatch",
		IntentRevision: 1, IntentDigest: digest("intent"),
		PermitID: "permit-dispatch", PermitRevision: 1, PermitDigest: digest("permit"),
		AttemptID: "runtime-attempt-dispatch",
	}
	boundary, err := runtimeports.SealModelProviderBoundaryCurrentRefV1(
		runtimeports.ModelProviderBoundaryCurrentRefV1{
			Owner: core.OwnerRef{Domain: "praxis.model", ID: "model-invoker"},
			ID:    "boundary-dispatch", Revision: 1,
			OperationDigest: operationDigest, EffectID: runtimeAttempt.EffectID,
			RuntimeAttempt: runtimeAttempt, DispatchSequence: 5,
			ProviderAttemptOrdinal: 1, AttemptRequestDigest: digest("attempt-request"),
			AcknowledgementDigest: digest("ack"),
			ExpiresUnixNano:       now.Add(16 * time.Second).UnixNano(),
		},
	)
	mustNextModelTurnDispatchV1(t, err)
	actualRequest := runtimeports.InspectCurrentModelProviderActualPointRequestV1{
		Operation: operation, EffectID: runtimeAttempt.EffectID, ExpectedEffectRevision: 3,
		PermitID: runtimeAttempt.PermitID, ExpectedPermitFactRevision: 2,
		PermitDigest: runtimeAttempt.PermitDigest, AdmissionDigest: digest("admission"),
		ReviewAuthorization: runtimeports.OperationReviewAuthorizationRefV4{
			ID: "review-dispatch", Revision: 1, Digest: digest("review"),
		},
		Attempt: runtimeAttempt,
		Verifier: runtimeports.ProviderBindingRefV2{
			BindingSetID: "binding-dispatch", BindingSetRevision: 1,
			ComponentID: "praxis.model/provider", ManifestDigest: digest("manifest"),
			ArtifactDigest: digest("artifact"), Capability: runtimeports.ModelInvokeCapabilityV1,
		},
		FenceDigest: digest("fence"), ModelBoundary: boundary,
		RequestedNotAfterUnixNano: now.Add(15 * time.Second).UnixNano(),
	}
	eligibilityRequest, err := applicationcontract.SealNextModelTurnEligibilityRequestV1(
		applicationcontract.NextModelTurnEligibilityRequestV1{
			ContinuationAttempt:       current.Start.AttemptRefV1(),
			ContinuationCurrentDigest: current.Digest,
			ActiveContext:             current.ActiveContext, Run: run, Session: session,
			TargetTurn: current.Start.TargetTurn, RuntimeActualPoint: actualRequest,
			RequestedNotAfterUnixNano: actualRequest.RequestedNotAfterUnixNano,
		},
	)
	mustNextModelTurnDispatchV1(t, err)
	eligibilityProjection, err := applicationcontract.SealNextModelTurnEligibilityProjectionV1(
		eligibilityRequest,
		current.ExpiresUnixNano,
		now,
	)
	mustNextModelTurnDispatchV1(t, err)
	request, err := applicationcontract.SealNextModelTurnDispatchRequestV1(
		applicationcontract.NextModelTurnDispatchRequestV1{
			EligibilityRequest:        eligibilityRequest,
			EligibilityProjection:     eligibilityProjection,
			RequestedNotAfterUnixNano: now.Add(11 * time.Second).UnixNano(),
		},
	)
	mustNextModelTurnDispatchV1(t, err)
	return nextModelTurnDispatchFixtureV1{
		now: now, request: request, current: current, pending: pending,
	}
}

func openNextModelTurnDispatchStoreV1(
	t *testing.T,
	path string,
	clock *nextModelTurnDispatchClockV1,
) *SQLiteNextModelTurnDispatchV1 {
	t.Helper()
	assertNextModelTurnDispatchNoForbiddenProductionImportsV1(t)
	store, err := OpenSQLiteNextModelTurnDispatchV1(
		context.Background(),
		SQLiteNextModelTurnDispatchConfigV1{Path: path, Clock: clock.Now},
	)
	mustNextModelTurnDispatchV1(t, err)
	return store
}

func newNextModelTurnDispatchBindingV1(
	t *testing.T,
	reader *nextModelTurnDispatchReaderV1,
	store nextModelTurnDispatchBindingRepositoryV1,
	clock *nextModelTurnDispatchClockV1,
) *NextModelTurnDispatchBindingV1 {
	t.Helper()
	binding, err := NewNextModelTurnDispatchBindingV1(
		NextModelTurnDispatchBindingConfigV1{
			Continuations: reader,
			Bindings:      store,
			Clock:         clock.Now,
		},
	)
	mustNextModelTurnDispatchV1(t, err)
	return binding
}

func TestNextModelTurnDispatchBindingLostReplyRestartAndNoForbiddenCallSurfaceV1(t *testing.T) {
	fixture := newNextModelTurnDispatchFixtureV1(t)
	clock := newNextModelTurnDispatchClockV1(fixture.now)
	path := filepath.Join(t.TempDir(), "dispatch.db")
	store := openNextModelTurnDispatchStoreV1(t, path, clock)
	reader := &nextModelTurnDispatchReaderV1{current: fixture.current}
	binding := newNextModelTurnDispatchBindingV1(t, reader, store, clock)

	store.LoseNextReplyForTestingV1()
	first, err := binding.StartOrInspectNextModelTurnV1(context.Background(), fixture.request)
	mustNextModelTurnDispatchV1(t, err)
	if first.State != applicationcontract.NextModelTurnDispatchAttemptBoundV1 ||
		first.DerivedDispatch.ID != fixture.request.EligibilityProjection.DerivedDispatch.ID ||
		reader.calls != 2 {
		t.Fatalf("first binding did not preserve the exact attempt-bound closure: %+v calls=%d", first, reader.calls)
	}
	replayed, err := binding.StartOrInspectNextModelTurnV1(context.Background(), fixture.request)
	mustNextModelTurnDispatchV1(t, err)
	if replayed != first || reader.calls != 2 {
		t.Fatalf("idempotent replay re-read or changed the binding: calls=%d", reader.calls)
	}
	inspect, err := applicationcontract.NewNextModelTurnDispatchInspectRequestV1(fixture.request)
	mustNextModelTurnDispatchV1(t, err)
	inspected, err := binding.InspectNextModelTurnV1(context.Background(), inspect)
	mustNextModelTurnDispatchV1(t, err)
	if inspected != first {
		t.Fatal("Inspect did not recover the original exact binding")
	}
	mustNextModelTurnDispatchV1(t, store.Close())
	restarted := openNextModelTurnDispatchStoreV1(t, path, clock)
	defer restarted.Close()
	restartedBinding := newNextModelTurnDispatchBindingV1(t, reader, restarted, clock)
	recovered, err := restartedBinding.InspectNextModelTurnV1(context.Background(), inspect)
	mustNextModelTurnDispatchV1(t, err)
	if recovered != first {
		t.Fatal("restart did not recover the exact durable binding")
	}
	if err = restarted.IntegrityCheckV1(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestNextModelTurnDispatchBindingFailClosedZeroWriteMatrixV1(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*nextModelTurnDispatchFixtureV1, *nextModelTurnDispatchReaderV1, *nextModelTurnDispatchClockV1)
	}{
		{
			name: "continuation pending",
			prepare: func(f *nextModelTurnDispatchFixtureV1, r *nextModelTurnDispatchReaderV1, _ *nextModelTurnDispatchClockV1) {
				r.current = f.pending
			},
		},
		{
			name: "expired at exact boundary",
			prepare: func(f *nextModelTurnDispatchFixtureV1, _ *nextModelTurnDispatchReaderV1, c *nextModelTurnDispatchClockV1) {
				c.Set(time.Unix(0, f.request.RequestedNotAfterUnixNano))
			},
		},
		{
			name: "continuation current splice",
			prepare: func(f *nextModelTurnDispatchFixtureV1, r *nextModelTurnDispatchReaderV1, _ *nextModelTurnDispatchClockV1) {
				spliced := f.current.Clone()
				spliced.Digest = core.DigestBytes([]byte("splice"))
				r.current = spliced
			},
		},
		{
			name: "current reader unavailable",
			prepare: func(_ *nextModelTurnDispatchFixtureV1, r *nextModelTurnDispatchReaderV1, _ *nextModelTurnDispatchClockV1) {
				r.err = core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "reader unavailable")
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newNextModelTurnDispatchFixtureV1(t)
			clock := newNextModelTurnDispatchClockV1(fixture.now)
			store := openNextModelTurnDispatchStoreV1(t, filepath.Join(t.TempDir(), "dispatch.db"), clock)
			defer store.Close()
			reader := &nextModelTurnDispatchReaderV1{current: fixture.current}
			tc.prepare(&fixture, reader, clock)
			binding := newNextModelTurnDispatchBindingV1(t, reader, store, clock)
			if _, err := binding.StartOrInspectNextModelTurnV1(context.Background(), fixture.request); err == nil {
				t.Fatal("fail-closed case was accepted")
			}
			var history, current int
			mustNextModelTurnDispatchV1(t, store.db.QueryRow(
				`SELECT COUNT(*) FROM harness_next_model_turn_dispatch_history_v1`,
			).Scan(&history))
			mustNextModelTurnDispatchV1(t, store.db.QueryRow(
				`SELECT COUNT(*) FROM harness_next_model_turn_dispatch_current_v1`,
			).Scan(&current))
			if history != 0 || current != 0 {
				t.Fatalf("fail-closed case wrote history/current: %d/%d", history, current)
			}
		})
	}
	var typedNil *nextModelTurnDispatchReaderV1
	fixture := newNextModelTurnDispatchFixtureV1(t)
	clock := newNextModelTurnDispatchClockV1(fixture.now)
	store := openNextModelTurnDispatchStoreV1(t, filepath.Join(t.TempDir(), "typed-nil.db"), clock)
	defer store.Close()
	if value, err := NewNextModelTurnDispatchBindingV1(
		NextModelTurnDispatchBindingConfigV1{
			Continuations: typedNil, Bindings: store, Clock: clock.Now,
		},
	); value != nil || !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed nil dependency was accepted: value=%v err=%v", value, err)
	}
}

func TestNextModelTurnDispatchBindingS1S2DriftClockAndCancelWindowsV1(t *testing.T) {
	t.Run("S1 S2 drift", func(t *testing.T) {
		fixture := newNextModelTurnDispatchFixtureV1(t)
		clock := newNextModelTurnDispatchClockV1(fixture.now)
		store := openNextModelTurnDispatchStoreV1(t, filepath.Join(t.TempDir(), "drift.db"), clock)
		defer store.Close()
		drifted := fixture.current.Clone()
		drifted.Digest = core.DigestBytes([]byte("s2-drift"))
		reader := &nextModelTurnDispatchReaderV1{current: fixture.current, second: &drifted}
		binding := newNextModelTurnDispatchBindingV1(t, reader, store, clock)
		if _, err := binding.StartOrInspectNextModelTurnV1(context.Background(), fixture.request); err == nil {
			t.Fatal("S1/S2 drift was accepted")
		}
		assertNextModelTurnDispatchEmptyV1(t, store)
	})
	t.Run("clock rollback", func(t *testing.T) {
		fixture := newNextModelTurnDispatchFixtureV1(t)
		sequence := []time.Time{fixture.now, fixture.now, fixture.now.Add(-time.Nanosecond)}
		var mu sync.Mutex
		index := 0
		clockFn := func() time.Time {
			mu.Lock()
			defer mu.Unlock()
			value := sequence[index]
			if index < len(sequence)-1 {
				index++
			}
			return value
		}
		storeClock := newNextModelTurnDispatchClockV1(fixture.now)
		store := openNextModelTurnDispatchStoreV1(t, filepath.Join(t.TempDir(), "rollback.db"), storeClock)
		defer store.Close()
		binding, err := NewNextModelTurnDispatchBindingV1(
			NextModelTurnDispatchBindingConfigV1{
				Continuations: &nextModelTurnDispatchReaderV1{current: fixture.current},
				Bindings:      store, Clock: clockFn,
			},
		)
		mustNextModelTurnDispatchV1(t, err)
		if _, err = binding.StartOrInspectNextModelTurnV1(context.Background(), fixture.request); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("clock rollback was accepted: %v", err)
		}
		assertNextModelTurnDispatchEmptyV1(t, store)
	})
	for _, cancelAt := range []int{1, 2} {
		t.Run("cancel after owner read", func(t *testing.T) {
			fixture := newNextModelTurnDispatchFixtureV1(t)
			clock := newNextModelTurnDispatchClockV1(fixture.now)
			store := openNextModelTurnDispatchStoreV1(t, filepath.Join(t.TempDir(), "cancel.db"), clock)
			defer store.Close()
			ctx, cancel := context.WithCancel(context.Background())
			reader := &nextModelTurnDispatchReaderV1{
				current: fixture.current, cancelAt: cancelAt, cancel: cancel,
			}
			binding := newNextModelTurnDispatchBindingV1(t, reader, store, clock)
			if _, err := binding.StartOrInspectNextModelTurnV1(ctx, fixture.request); err == nil {
				t.Fatal("canceled request was accepted")
			}
			assertNextModelTurnDispatchEmptyV1(t, store)
		})
	}
	t.Run("cancel after SQLite mutation rolls back", func(t *testing.T) {
		fixture := newNextModelTurnDispatchFixtureV1(t)
		clock := newNextModelTurnDispatchClockV1(fixture.now)
		store := openNextModelTurnDispatchStoreV1(t, filepath.Join(t.TempDir(), "mutation-cancel.db"), clock)
		defer store.Close()
		ctx, cancel := context.WithCancel(context.Background())
		store.afterMutationForTesting = cancel
		binding := newNextModelTurnDispatchBindingV1(
			t,
			&nextModelTurnDispatchReaderV1{current: fixture.current},
			store,
			clock,
		)
		if _, err := binding.StartOrInspectNextModelTurnV1(ctx, fixture.request); err == nil {
			t.Fatal("post-mutation cancellation was accepted")
		}
		assertNextModelTurnDispatchEmptyV1(t, store)
	})
}

func TestNextModelTurnDispatchSQLiteConcurrent64SameAndDifferentPayloadV1(t *testing.T) {
	fixture := newNextModelTurnDispatchFixtureV1(t)
	clock := newNextModelTurnDispatchClockV1(fixture.now)
	store := openNextModelTurnDispatchStoreV1(t, filepath.Join(t.TempDir(), "concurrent.db"), clock)
	defer store.Close()
	binding := newNextModelTurnDispatchBindingV1(
		t,
		&nextModelTurnDispatchReaderV1{current: fixture.current},
		store,
		clock,
	)
	const workers = 64
	values := make(chan applicationcontract.NextModelTurnDispatchCurrentV1, workers)
	errs := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			value, err := binding.StartOrInspectNextModelTurnV1(context.Background(), fixture.request)
			if err != nil {
				errs <- err
				return
			}
			values <- value
		}()
	}
	wait.Wait()
	close(values)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	var winner applicationcontract.NextModelTurnDispatchCurrentV1
	for value := range values {
		if winner.Digest == "" {
			winner = value
		}
		if value != winner {
			t.Fatal("64 same-payload callers observed more than one winner")
		}
	}
	var history, current int
	mustNextModelTurnDispatchV1(t, store.db.QueryRow(
		`SELECT COUNT(*) FROM harness_next_model_turn_dispatch_history_v1`,
	).Scan(&history))
	mustNextModelTurnDispatchV1(t, store.db.QueryRow(
		`SELECT COUNT(*) FROM harness_next_model_turn_dispatch_current_v1`,
	).Scan(&current))
	if history != 1 || current != 1 {
		t.Fatalf("same-payload race did not leave one ledger/current row: %d/%d", history, current)
	}

	conflict := fixture.request
	conflict.RequestedNotAfterUnixNano--
	conflict.RequestDigest = ""
	var err error
	conflict, err = applicationcontract.SealNextModelTurnDispatchRequestV1(conflict)
	mustNextModelTurnDispatchV1(t, err)
	if conflict.EligibilityProjection.DerivedDispatch.ID != fixture.request.EligibilityProjection.DerivedDispatch.ID ||
		conflict.RequestDigest == fixture.request.RequestDigest {
		t.Fatal("conflict fixture did not preserve derived ID with another payload")
	}
	var conflicts atomic.Int64
	raceErrors := make(chan error, workers)
	for index := range workers {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			request := fixture.request
			if index%2 == 1 {
				request = conflict
			}
			value, callErr := binding.StartOrInspectNextModelTurnV1(context.Background(), request)
			if callErr != nil {
				if core.HasCategory(callErr, core.ErrorConflict) {
					conflicts.Add(1)
					return
				}
				raceErrors <- callErr
				return
			}
			if value != winner {
				raceErrors <- core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "different-payload race observed another winner")
			}
		}(index)
	}
	wait.Wait()
	close(raceErrors)
	for err := range raceErrors {
		t.Fatal(err)
	}
	if conflicts.Load() != workers/2 {
		t.Fatalf("different-payload race did not reject every loser: %d", conflicts.Load())
	}
}

func TestNextModelTurnDispatchSQLiteConcurrent64MixedPayloadFreshCreateV1(t *testing.T) {
	fixture := newNextModelTurnDispatchFixtureV1(t)
	conflicting := fixture.request
	conflicting.RequestedNotAfterUnixNano--
	conflicting.RequestDigest = ""
	var err error
	conflicting, err = applicationcontract.SealNextModelTurnDispatchRequestV1(conflicting)
	mustNextModelTurnDispatchV1(t, err)
	if conflicting.EligibilityProjection.DerivedDispatch.ID !=
		fixture.request.EligibilityProjection.DerivedDispatch.ID ||
		conflicting.RequestDigest == fixture.request.RequestDigest {
		t.Fatal("fresh mixed-payload fixture does not share one stable identity")
	}
	clock := newNextModelTurnDispatchClockV1(fixture.now)
	store := openNextModelTurnDispatchStoreV1(t, filepath.Join(t.TempDir(), "fresh-mixed.db"), clock)
	defer store.Close()
	assertNextModelTurnDispatchEmptyV1(t, store)
	binding := newNextModelTurnDispatchBindingV1(
		t,
		&nextModelTurnDispatchReaderV1{current: fixture.current},
		store,
		clock,
	)

	const workers = 64
	start := make(chan struct{})
	values := make(chan applicationcontract.NextModelTurnDispatchCurrentV1, workers)
	errs := make(chan error, workers)
	var wait sync.WaitGroup
	for index := range workers {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			request := fixture.request
			if index%2 == 1 {
				request = conflicting
			}
			value, callErr := binding.StartOrInspectNextModelTurnV1(context.Background(), request)
			if callErr != nil {
				errs <- callErr
				return
			}
			values <- value
		}(index)
	}
	close(start)
	wait.Wait()
	close(values)
	close(errs)

	var winner applicationcontract.NextModelTurnDispatchCurrentV1
	successes := 0
	for value := range values {
		successes++
		if winner.Digest == "" {
			winner = value
		}
		if value != winner {
			t.Fatal("fresh mixed-payload race returned more than one durable winner")
		}
	}
	if successes == 0 {
		t.Fatal("fresh mixed-payload race produced no winner")
	}
	conflicts := 0
	for callErr := range errs {
		if !core.HasCategory(callErr, core.ErrorConflict) {
			t.Fatalf("fresh mixed-payload loser returned a non-Conflict error: %v", callErr)
		}
		conflicts++
	}
	if successes+conflicts != workers {
		t.Fatalf("fresh mixed-payload race lost caller outcomes: success=%d conflict=%d", successes, conflicts)
	}
	if winner.RequestDigest != fixture.request.RequestDigest &&
		winner.RequestDigest != conflicting.RequestDigest {
		t.Fatalf("fresh mixed-payload race persisted an unknown payload: %+v", winner)
	}
	var history, current int
	mustNextModelTurnDispatchV1(t, store.db.QueryRow(
		`SELECT COUNT(*) FROM harness_next_model_turn_dispatch_history_v1`,
	).Scan(&history))
	mustNextModelTurnDispatchV1(t, store.db.QueryRow(
		`SELECT COUNT(*) FROM harness_next_model_turn_dispatch_current_v1`,
	).Scan(&current))
	if history != 1 || current != 1 {
		t.Fatalf("fresh mixed-payload race did not create exactly one winner/current: %d/%d", history, current)
	}
}

func TestNextModelTurnDispatchSQLiteCanonicalCorruptionAndExactSchemaV1(t *testing.T) {
	fixture := newNextModelTurnDispatchFixtureV1(t)
	clock := newNextModelTurnDispatchClockV1(fixture.now)
	store := openNextModelTurnDispatchStoreV1(t, filepath.Join(t.TempDir(), "corrupt.db"), clock)
	defer store.Close()
	binding := newNextModelTurnDispatchBindingV1(
		t,
		&nextModelTurnDispatchReaderV1{current: fixture.current},
		store,
		clock,
	)
	_, err := binding.StartOrInspectNextModelTurnV1(context.Background(), fixture.request)
	mustNextModelTurnDispatchV1(t, err)
	var tables int
	mustNextModelTurnDispatchV1(t, store.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('harness_next_model_turn_dispatch_schema_v1','harness_next_model_turn_dispatch_history_v1','harness_next_model_turn_dispatch_current_v1')`,
	).Scan(&tables))
	if tables != 3 {
		t.Fatalf("exact schema tables missing: %d", tables)
	}
	_, err = store.db.Exec(
		`UPDATE harness_next_model_turn_dispatch_current_v1 SET canonical_json=?`,
		[]byte(`{"spliced":true}`),
	)
	mustNextModelTurnDispatchV1(t, err)
	inspect, err := applicationcontract.NewNextModelTurnDispatchInspectRequestV1(fixture.request)
	mustNextModelTurnDispatchV1(t, err)
	if _, err = binding.InspectNextModelTurnV1(context.Background(), inspect); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("canonical corruption was accepted: %v", err)
	}
}

func TestNextModelTurnDispatchSQLiteOpenRejectsPartialWeakAndUnexpectedClosureV1(t *testing.T) {
	tests := []struct {
		name       string
		startFresh bool
		mutate     []string
	}{
		{
			name:       "correct ledger missing both data tables",
			startFresh: true,
			mutate: []string{
				`DROP TABLE harness_next_model_turn_dispatch_current_v1`,
				`DROP TABLE harness_next_model_turn_dispatch_history_v1`,
			},
		},
		{
			name:       "correct ledger missing history attempt table",
			startFresh: true,
			mutate: []string{
				`DROP TABLE harness_next_model_turn_dispatch_history_v1`,
			},
		},
		{
			name:       "correct ledger missing current fact table",
			startFresh: true,
			mutate: []string{
				`DROP TABLE harness_next_model_turn_dispatch_current_v1`,
			},
		},
		{
			name:       "correct ledger missing exact index",
			startFresh: true,
			mutate: []string{
				`DROP INDEX harness_next_model_turn_dispatch_current_exact_v1`,
			},
		},
		{
			name:       "partial data table without ledger",
			startFresh: false,
			mutate: []string{
				sqliteNextModelTurnDispatchHistoryTableV1,
			},
		},
		{
			name:       "correct ledger weak current schema",
			startFresh: true,
			mutate: []string{
				`DROP TABLE harness_next_model_turn_dispatch_current_v1`,
				`CREATE TABLE harness_next_model_turn_dispatch_current_v1 (
				   dispatch_id TEXT PRIMARY KEY,
				   revision INTEGER,
				   fact_digest TEXT,
				   request_digest TEXT,
				   canonical_json BLOB,
				   updated_unix_nano INTEGER
				 )`,
				sqliteNextModelTurnDispatchCurrentIndexV1,
			},
		},
		{
			name:       "unexpected trigger in owner closure",
			startFresh: true,
			mutate: []string{
				`CREATE TRIGGER adversarial_next_model_turn_trigger
				   AFTER INSERT ON harness_next_model_turn_dispatch_history_v1
				   BEGIN SELECT 1; END`,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newNextModelTurnDispatchFixtureV1(t)
			clock := newNextModelTurnDispatchClockV1(fixture.now)
			path := filepath.Join(t.TempDir(), "closure.db")
			if tc.startFresh {
				store := openNextModelTurnDispatchStoreV1(t, path, clock)
				mustNextModelTurnDispatchV1(t, store.Close())
			}
			mutateNextModelTurnDispatchSQLiteV1(t, path, tc.mutate...)
			before := snapshotNextModelTurnDispatchSQLiteObjectsV1(t, path)
			reopened, err := OpenSQLiteNextModelTurnDispatchV1(
				context.Background(),
				SQLiteNextModelTurnDispatchConfigV1{Path: path, Clock: clock.Now},
			)
			if reopened != nil {
				_ = reopened.Close()
				t.Fatal("partial or weak physical closure reopened")
			}
			if !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("partial or weak physical closure did not Conflict: %v", err)
			}
			after := snapshotNextModelTurnDispatchSQLiteObjectsV1(t, path)
			if before != after {
				t.Fatalf("failed open silently repaired physical objects:\nbefore=%s\nafter=%s", before, after)
			}
		})
	}
}

func TestNextModelTurnDispatchSQLiteRejectsForgedOutcomeAndUnknownStateV1(t *testing.T) {
	for _, state := range []applicationcontract.NextModelTurnDispatchStateV1{
		"outcome_bound",
		"future_unknown_state",
	} {
		t.Run(string(state), func(t *testing.T) {
			fixture := newNextModelTurnDispatchFixtureV1(t)
			clock := newNextModelTurnDispatchClockV1(fixture.now)
			store := openNextModelTurnDispatchStoreV1(t, filepath.Join(t.TempDir(), "forged.db"), clock)
			defer store.Close()
			binding := newNextModelTurnDispatchBindingV1(
				t,
				&nextModelTurnDispatchReaderV1{current: fixture.current},
				store,
				clock,
			)
			current, err := binding.StartOrInspectNextModelTurnV1(context.Background(), fixture.request)
			mustNextModelTurnDispatchV1(t, err)
			forged := current
			forged.State = state
			forged.Digest = ""
			forged.Digest, err = forged.DigestV1()
			mustNextModelTurnDispatchV1(t, err)
			payload, err := applicationcontract.EncodeNextModelTurnDispatchCurrentV1(forged)
			mustNextModelTurnDispatchV1(t, err)
			for _, table := range []string{
				"harness_next_model_turn_dispatch_history_v1",
				"harness_next_model_turn_dispatch_current_v1",
			} {
				_, err = store.db.Exec(
					`UPDATE `+table+` SET fact_digest=?,request_digest=?,canonical_json=? WHERE dispatch_id=?`,
					string(forged.Digest),
					string(forged.RequestDigest),
					payload,
					forged.DerivedDispatch.ID,
				)
				mustNextModelTurnDispatchV1(t, err)
			}
			if err = store.IntegrityCheckV1(context.Background()); !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("IntegrityCheck accepted forged state %q: %v", state, err)
			}
			inspect, err := applicationcontract.NewNextModelTurnDispatchInspectRequestV1(fixture.request)
			mustNextModelTurnDispatchV1(t, err)
			if _, err = binding.InspectNextModelTurnV1(context.Background(), inspect); !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("public Inspect accepted forged state %q: %v", state, err)
			}
		})
	}
}

func TestNextModelTurnDispatchRejectsStructurallyForgedDerivedRefLiveAndRestartV1(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*applicationcontract.NextModelTurnDerivedDispatchRefV1)
	}{
		{
			name: "attacker selected stable ID",
			mutate: func(ref *applicationcontract.NextModelTurnDerivedDispatchRefV1) {
				ref.ID = "attacker-selected-derived-id"
			},
		},
		{
			name: "invalid continuation attempt",
			mutate: func(ref *applicationcontract.NextModelTurnDerivedDispatchRefV1) {
				ref.ContinuationAttempt.SourceTurn = 0
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newNextModelTurnDispatchFixtureV1(t)
			clock := newNextModelTurnDispatchClockV1(fixture.now)
			path := filepath.Join(t.TempDir(), "forged-derived.db")
			store := openNextModelTurnDispatchStoreV1(t, path, clock)
			binding := newNextModelTurnDispatchBindingV1(
				t,
				&nextModelTurnDispatchReaderV1{current: fixture.current},
				store,
				clock,
			)
			current, err := applicationcontract.SealNextModelTurnDispatchAttemptBoundV1(
				fixture.request,
				fixture.request.EligibilityProjection.CheckedUnixNano,
				fixture.request.RequestedNotAfterUnixNano,
			)
			mustNextModelTurnDispatchV1(t, err)
			forged := current
			tc.mutate(&forged.DerivedDispatch)
			forged.DerivedDispatch.Digest = ""
			forged.DerivedDispatch.Digest, err = forged.DerivedDispatch.DigestV1()
			mustNextModelTurnDispatchV1(t, err)
			forged.Digest = ""
			forged.Digest, err = forged.DigestV1()
			mustNextModelTurnDispatchV1(t, err)
			if _, err = store.ensureNextModelTurnDispatchBindingV1(
				context.Background(),
				fixture.request,
				forged,
			); !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("store write accepted structurally forged derived ref: %v", err)
			}
			assertNextModelTurnDispatchEmptyV1(t, store)

			current, err = binding.StartOrInspectNextModelTurnV1(context.Background(), fixture.request)
			mustNextModelTurnDispatchV1(t, err)
			originalID := current.DerivedDispatch.ID
			overwriteNextModelTurnDispatchSQLitePairV1(t, path, originalID, forged)

			if err = store.IntegrityCheckV1(context.Background()); !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("live IntegrityCheck accepted structurally forged derived ref: %v", err)
			}
			inspect := applicationcontract.NextModelTurnDispatchInspectRequestV1{
				ContractVersion: applicationcontract.NextModelTurnDispatchContractVersionV1,
				DerivedDispatch: forged.DerivedDispatch,
				RequestDigest:   forged.RequestDigest,
			}
			inspect.Digest, err = inspect.DigestV1()
			mustNextModelTurnDispatchV1(t, err)
			if _, err = binding.InspectNextModelTurnV1(context.Background(), inspect); err == nil {
				t.Fatal("public Inspect accepted structurally forged derived ref")
			}

			mustNextModelTurnDispatchV1(t, store.Close())
			reopened, err := OpenSQLiteNextModelTurnDispatchV1(
				context.Background(),
				SQLiteNextModelTurnDispatchConfigV1{Path: path, Clock: clock.Now},
			)
			if reopened != nil {
				_ = reopened.Close()
				t.Fatal("restart accepted structurally forged durable ref")
			}
			if !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("restart did not Conflict on structurally forged durable ref: %v", err)
			}
		})
	}
}

func TestNextModelTurnDispatchTTLBoundedMutationPointsV1(t *testing.T) {
	t.Run("owner lock wait crosses exact expiry", func(t *testing.T) {
		fixture := newNextModelTurnDispatchFixtureV1(t)
		clock := newNextModelTurnDispatchClockV1(fixture.now)
		store := openNextModelTurnDispatchStoreV1(t, filepath.Join(t.TempDir(), "lock-expiry.db"), clock)
		defer store.Close()
		repository := &nextModelTurnDispatchEnsureGateV1{
			store:          store,
			inspectDone:    make(chan struct{}),
			releaseInspect: make(chan struct{}),
			ensureStarted:  make(chan struct{}),
		}
		binding := newNextModelTurnDispatchBindingV1(
			t,
			&nextModelTurnDispatchReaderV1{current: fixture.current},
			repository,
			clock,
		)
		result := make(chan error, 1)
		go func() {
			_, err := binding.StartOrInspectNextModelTurnV1(context.Background(), fixture.request)
			result <- err
		}()
		<-repository.inspectDone
		store.mu.Lock()
		close(repository.releaseInspect)
		<-repository.ensureStarted
		clock.Set(time.Unix(0, fixture.request.RequestedNotAfterUnixNano))
		store.mu.Unlock()
		if err := <-result; !core.HasReason(err, core.ReasonBindingExpired) {
			t.Fatalf("lock-wait TTL crossing was not rejected at exact expiry: %v", err)
		}
		assertNextModelTurnDispatchEmptyV1(t, store)
	})

	t.Run("transaction mutation crosses exact expiry", func(t *testing.T) {
		fixture := newNextModelTurnDispatchFixtureV1(t)
		clock := newNextModelTurnDispatchClockV1(fixture.now)
		store := openNextModelTurnDispatchStoreV1(t, filepath.Join(t.TempDir(), "transaction-expiry.db"), clock)
		defer store.Close()
		store.afterMutationForTesting = func() {
			clock.Set(time.Unix(0, fixture.request.RequestedNotAfterUnixNano))
		}
		binding := newNextModelTurnDispatchBindingV1(
			t,
			&nextModelTurnDispatchReaderV1{current: fixture.current},
			store,
			clock,
		)
		if _, err := binding.StartOrInspectNextModelTurnV1(context.Background(), fixture.request); !core.HasReason(err, core.ReasonBindingExpired) {
			t.Fatalf("transaction TTL crossing was not rejected at exact expiry: %v", err)
		}
		assertNextModelTurnDispatchEmptyV1(t, store)
	})

	t.Run("transaction clock rolls back before commit", func(t *testing.T) {
		fixture := newNextModelTurnDispatchFixtureV1(t)
		clock := newNextModelTurnDispatchClockV1(fixture.now)
		store := openNextModelTurnDispatchStoreV1(t, filepath.Join(t.TempDir(), "transaction-rollback.db"), clock)
		defer store.Close()
		store.afterMutationForTesting = func() {
			clock.Set(fixture.now.Add(-time.Nanosecond))
		}
		binding := newNextModelTurnDispatchBindingV1(
			t,
			&nextModelTurnDispatchReaderV1{current: fixture.current},
			store,
			clock,
		)
		if _, err := binding.StartOrInspectNextModelTurnV1(context.Background(), fixture.request); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("pre-commit clock rollback was accepted: %v", err)
		}
		assertNextModelTurnDispatchEmptyV1(t, store)
	})
}

func TestNextModelTurnDispatchReusableConformanceAndZeroExecutionCountsV1(t *testing.T) {
	fixture := newNextModelTurnDispatchFixtureV1(t)
	conflict := fixture.request
	conflict.RequestedNotAfterUnixNano--
	conflict.RequestDigest = ""
	var err error
	conflict, err = applicationcontract.SealNextModelTurnDispatchRequestV1(conflict)
	mustNextModelTurnDispatchV1(t, err)
	clock := newNextModelTurnDispatchClockV1(fixture.now)
	store := openNextModelTurnDispatchStoreV1(t, filepath.Join(t.TempDir(), "conformance.db"), clock)
	defer store.Close()
	binding := newNextModelTurnDispatchBindingV1(
		t,
		&nextModelTurnDispatchReaderV1{current: fixture.current},
		store,
		clock,
	)
	report, err := applicationconformance.CheckNextModelTurnDispatchPortV1(
		context.Background(),
		applicationconformance.NextModelTurnDispatchCaseV1{
			NewPort: func() applicationports.NextModelTurnDispatchPortV1 {
				return binding
			},
			Request: fixture.request, Conflicting: conflict,
		},
	)
	mustNextModelTurnDispatchV1(t, err)
	if !report.ExactInspect || !report.IdempotentReplay ||
		!report.ConcurrentSingleWinner || !report.ChangedPayloadRejected ||
		!report.AttemptBoundOnly ||
		report.ProductionEligible ||
		report.OutcomeBindingEligible {
		t.Fatalf("unexpected next-turn conformance report: %+v", report)
	}
}

type nextModelTurnDispatchEnsureGateV1 struct {
	store          *SQLiteNextModelTurnDispatchV1
	inspectDone    chan struct{}
	releaseInspect chan struct{}
	ensureStarted  chan struct{}
	inspectOnce    sync.Once
	ensureOnce     sync.Once
}

func (r *nextModelTurnDispatchEnsureGateV1) inspectNextModelTurnDispatchBindingV1(
	ctx context.Context,
	request applicationcontract.NextModelTurnDispatchInspectRequestV1,
) (applicationcontract.NextModelTurnDispatchCurrentV1, error) {
	current, err := r.store.inspectNextModelTurnDispatchBindingV1(ctx, request)
	r.inspectOnce.Do(func() {
		close(r.inspectDone)
		<-r.releaseInspect
	})
	return current, err
}

func (r *nextModelTurnDispatchEnsureGateV1) ensureNextModelTurnDispatchBindingV1(
	ctx context.Context,
	request applicationcontract.NextModelTurnDispatchRequestV1,
	current applicationcontract.NextModelTurnDispatchCurrentV1,
) (applicationcontract.NextModelTurnDispatchCurrentV1, error) {
	r.ensureOnce.Do(func() { close(r.ensureStarted) })
	return r.store.ensureNextModelTurnDispatchBindingV1(ctx, request, current)
}

func mutateNextModelTurnDispatchSQLiteV1(t *testing.T, path string, statements ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	mustNextModelTurnDispatchV1(t, err)
	defer db.Close()
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA foreign_keys=OFF`)
	mustNextModelTurnDispatchV1(t, err)
	for _, statement := range statements {
		_, err = db.Exec(statement)
		mustNextModelTurnDispatchV1(t, err)
	}
}

func overwriteNextModelTurnDispatchSQLitePairV1(
	t *testing.T,
	path string,
	originalID string,
	current applicationcontract.NextModelTurnDispatchCurrentV1,
) {
	t.Helper()
	payload, err := applicationcontract.EncodeNextModelTurnDispatchCurrentV1(current)
	mustNextModelTurnDispatchV1(t, err)
	db, err := sql.Open("sqlite", path)
	mustNextModelTurnDispatchV1(t, err)
	defer db.Close()
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA foreign_keys=OFF`)
	mustNextModelTurnDispatchV1(t, err)
	tx, err := db.Begin()
	mustNextModelTurnDispatchV1(t, err)
	defer tx.Rollback()
	_, err = tx.Exec(
		`UPDATE harness_next_model_turn_dispatch_history_v1
		    SET dispatch_id=?,revision=?,fact_digest=?,request_digest=?,canonical_json=?,created_unix_nano=?
		  WHERE dispatch_id=?`,
		current.DerivedDispatch.ID,
		current.Revision,
		string(current.Digest),
		string(current.RequestDigest),
		payload,
		current.CheckedUnixNano,
		originalID,
	)
	mustNextModelTurnDispatchV1(t, err)
	_, err = tx.Exec(
		`UPDATE harness_next_model_turn_dispatch_current_v1
		    SET dispatch_id=?,revision=?,fact_digest=?,request_digest=?,canonical_json=?,updated_unix_nano=?
		  WHERE dispatch_id=?`,
		current.DerivedDispatch.ID,
		current.Revision,
		string(current.Digest),
		string(current.RequestDigest),
		payload,
		current.CheckedUnixNano,
		originalID,
	)
	mustNextModelTurnDispatchV1(t, err)
	mustNextModelTurnDispatchV1(t, tx.Commit())
}

func snapshotNextModelTurnDispatchSQLiteObjectsV1(t *testing.T, path string) string {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	mustNextModelTurnDispatchV1(t, err)
	defer db.Close()
	rows, err := db.Query(
		`SELECT type,name,tbl_name,sql
		   FROM sqlite_master
		  WHERE sql IS NOT NULL
		    AND (
		      name LIKE 'harness_next_model_turn_dispatch_%'
		      OR tbl_name IN (
		        'harness_next_model_turn_dispatch_schema_v1',
		        'harness_next_model_turn_dispatch_history_v1',
		        'harness_next_model_turn_dispatch_current_v1'
		      )
		    )
		  ORDER BY type,name`,
	)
	mustNextModelTurnDispatchV1(t, err)
	defer rows.Close()
	var snapshot string
	for rows.Next() {
		var kind, name, tableName, ddl string
		mustNextModelTurnDispatchV1(t, rows.Scan(&kind, &name, &tableName, &ddl))
		snapshot += kind + "\x00" + name + "\x00" + tableName + "\x00" + ddl + "\x00"
	}
	mustNextModelTurnDispatchV1(t, rows.Err())
	return snapshot
}

func assertNextModelTurnDispatchEmptyV1(
	t *testing.T,
	store *SQLiteNextModelTurnDispatchV1,
) {
	t.Helper()
	assertNextModelTurnDispatchNoForbiddenProductionImportsV1(t)
	var history, current int
	mustNextModelTurnDispatchV1(t, store.db.QueryRow(
		`SELECT COUNT(*) FROM harness_next_model_turn_dispatch_history_v1`,
	).Scan(&history))
	mustNextModelTurnDispatchV1(t, store.db.QueryRow(
		`SELECT COUNT(*) FROM harness_next_model_turn_dispatch_current_v1`,
	).Scan(&current))
	if history != 0 || current != 0 {
		t.Fatalf("unexpected next-turn rows: history=%d current=%d", history, current)
	}
}

func assertNextModelTurnDispatchNoForbiddenProductionImportsV1(t *testing.T) {
	t.Helper()
	nextModelTurnDispatchProductionImportScanV1.once.Do(func() {
		_, sourceFile, _, ok := runtime.Caller(0)
		if !ok {
			nextModelTurnDispatchProductionImportScanV1.err = core.NewError(
				core.ErrorUnavailable,
				core.ReasonEvidenceUnavailable,
				"cannot resolve next-turn Harness production import scanner root",
			)
			return
		}
		executionRuntimeRoot := filepath.Dir(filepath.Dir(filepath.Dir(sourceFile)))
		var productionImports []string
		for _, relative := range []string{
			"application/contract/next_model_turn_dispatch_v1.go",
			"application/ports/next_model_turn_dispatch_v1.go",
			"application/conformance/next_model_turn_dispatch_v1.go",
			"harness/applicationadapter/next_model_turn_dispatch_binding_v1.go",
			"harness/applicationadapter/sqlite_next_model_turn_dispatch_v1.go",
		} {
			file, err := parser.ParseFile(
				token.NewFileSet(),
				filepath.Join(executionRuntimeRoot, relative),
				nil,
				parser.ImportsOnly,
			)
			if err != nil {
				nextModelTurnDispatchProductionImportScanV1.err = err
				return
			}
			for _, imported := range file.Imports {
				path, err := strconv.Unquote(imported.Path.Value)
				if err != nil {
					nextModelTurnDispatchProductionImportScanV1.err = err
					return
				}
				productionImports = append(productionImports, path)
			}
		}
		nextModelTurnDispatchProductionImportScanV1.err =
			applicationconformance.CheckNextModelTurnDispatchImportsV1(productionImports)
	})
	if nextModelTurnDispatchProductionImportScanV1.err != nil {
		t.Fatalf("production import scanner found a forbidden execution surface: %v", nextModelTurnDispatchProductionImportScanV1.err)
	}
}

var nextModelTurnDispatchProductionImportScanV1 struct {
	once sync.Once
	err  error
}

func mustNextModelTurnDispatchV1(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

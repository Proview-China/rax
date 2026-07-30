package application_test

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	application "github.com/Proview-China/rax/ExecutionRuntime/application"
	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

var (
	_ applicationports.TurnContinuationCurrentReaderV1 = (*nextTurnContinuationReaderV1)(nil)
	_ applicationports.NextModelTurnEligibilityPortV1  = (*application.NextModelTurnEligibilityV1)(nil)
)

type nextTurnContinuationReaderV1 struct {
	mu        sync.Mutex
	current   applicationcontract.TurnContinuationCurrentV1
	err       error
	loseFirst bool
	calls     int
	requests  []applicationcontract.TurnContinuationInspectRequestV1
}

func (r *nextTurnContinuationReaderV1) InspectTurnContinuationV1(
	_ context.Context,
	request applicationcontract.TurnContinuationInspectRequestV1,
) (applicationcontract.TurnContinuationCurrentV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.requests = append(r.requests, request)
	if r.loseFirst && r.calls == 1 {
		return applicationcontract.TurnContinuationCurrentV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEvidenceUnavailable, "lost read-only Inspect reply")
	}
	if r.err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, r.err
	}
	return r.current.Clone(), nil
}

type nextTurnEligibilityFixtureV1 struct {
	now          time.Time
	request      applicationcontract.NextModelTurnEligibilityRequestV1
	current      applicationcontract.TurnContinuationCurrentV1
	pending      applicationcontract.TurnContinuationCurrentV1
	continuation *nextTurnContinuationReaderV1
}

func TestNextModelTurnEligibilityV1StableDerivedRefAndNoActualPointCallSurface(t *testing.T) {
	fixture := newNextTurnEligibilityFixtureV1(t)
	inspector := newNextTurnEligibilityInspectorV1(t, fixture, func() time.Time { return fixture.now })

	first, err := inspector.InspectNextModelTurnEligibilityV1(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := inspector.InspectNextModelTurnEligibilityV1(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if first.DerivedDispatch != second.DerivedDispatch {
		t.Fatalf("same exact request derived unstable dispatch refs:\nfirst=%+v\nsecond=%+v", first.DerivedDispatch, second.DerivedDispatch)
	}
	if err := first.ValidateFor(fixture.request, fixture.now); err != nil {
		t.Fatal(err)
	}
	if first.RuntimeActualPointRequestDigest != fixture.request.RuntimeActualPointRequestDigest ||
		first.NotAfterUnixNano != fixture.request.RequestedNotAfterUnixNano {
		t.Fatalf("eligibility lost the future actual-point coordinate or Continuation TTL: %+v", first)
	}

	projectionType := reflect.TypeOf(first)
	for index := 0; index < projectionType.NumField(); index++ {
		field := projectionType.Field(index)
		name := strings.ToLower(field.Name)
		if strings.Contains(name, "actualpointprojection") || strings.Contains(name, "allowed") {
			t.Fatalf("eligibility projection exposed forbidden field %s", field.Name)
		}
	}
	configType := reflect.TypeOf(application.NextModelTurnEligibilityConfigV1{})
	if configType.NumField() != 2 {
		t.Fatalf("eligibility config grew an unexpected surface: %d", configType.NumField())
	}
	for index := 0; index < configType.NumField(); index++ {
		field := configType.Field(index)
		boundary := strings.ToLower(field.Name + " " + field.Type.String())
		for _, forbidden := range []string{"actualpoint", "model", "provider", "dispatch"} {
			if strings.Contains(boundary, forbidden) {
				t.Fatalf("eligibility config gained forbidden %s boundary: %s", forbidden, boundary)
			}
		}
	}
}

func TestNextModelTurnEligibilityV1FailClosedMatrixHasZeroDownstreamExecution(t *testing.T) {
	tests := []struct {
		name                 string
		prepare              func(*nextTurnEligibilityFixtureV1) (context.Context, func() time.Time)
		wantContinuationRead int
		wantReason           core.ReasonCode
	}{
		{
			name: "continuation pending",
			prepare: func(f *nextTurnEligibilityFixtureV1) (context.Context, func() time.Time) {
				f.continuation.current = f.pending
				return context.Background(), func() time.Time { return f.now }
			},
			wantContinuationRead: 1,
			wantReason:           core.ReasonInvalidState,
		},
		{
			name: "expired at exclusive boundary",
			prepare: func(f *nextTurnEligibilityFixtureV1) (context.Context, func() time.Time) {
				return context.Background(), func() time.Time {
					return time.Unix(0, f.request.RequestedNotAfterUnixNano)
				}
			},
			wantReason: core.ReasonBindingExpired,
		},
		{
			name: "continuation current digest splice",
			prepare: func(f *nextTurnEligibilityFixtureV1) (context.Context, func() time.Time) {
				f.request.ContinuationCurrentDigest = nextTurnDigestV1("spliced-continuation-current")
				f.request.Digest = ""
				var err error
				f.request, err = applicationcontract.SealNextModelTurnEligibilityRequestV1(f.request)
				if err != nil {
					panic(err)
				}
				return context.Background(), func() time.Time { return f.now }
			},
			wantContinuationRead: 1,
			wantReason:           core.ReasonEvidenceConflict,
		},
		{
			name: "cancelled before current read",
			prepare: func(f *nextTurnEligibilityFixtureV1) (context.Context, func() time.Time) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, func() time.Time { return f.now }
			},
		},
		{
			name: "invalid future fence coordinate",
			prepare: func(f *nextTurnEligibilityFixtureV1) (context.Context, func() time.Time) {
				f.request.RuntimeActualPoint.FenceDigest = ""
				return context.Background(), func() time.Time { return f.now }
			},
			wantReason: core.ReasonInvalidReference,
		},
		{
			name: "continuation current reader unavailable",
			prepare: func(f *nextTurnEligibilityFixtureV1) (context.Context, func() time.Time) {
				f.continuation.err = core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "continuation current unavailable")
				return context.Background(), func() time.Time { return f.now }
			},
			wantContinuationRead: 1,
			wantReason:           core.ReasonComponentMissing,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newNextTurnEligibilityFixtureV1(t)
			ctx, clock := testCase.prepare(fixture)
			inspector := newNextTurnEligibilityInspectorV1(t, fixture, clock)
			projection, err := inspector.InspectNextModelTurnEligibilityV1(ctx, fixture.request)
			if err == nil {
				t.Fatal("negative eligibility path returned a projection")
			}
			if projection != (applicationcontract.NextModelTurnEligibilityProjectionV1{}) {
				t.Fatalf("negative eligibility path leaked a projection: %+v", projection)
			}
			if testCase.wantReason != "" && !core.HasReason(err, testCase.wantReason) {
				t.Fatalf("error reason = %v, want %s", err, testCase.wantReason)
			}
			if fixture.continuation.calls != testCase.wantContinuationRead {
				t.Fatalf("unexpected Continuation read count: %d", fixture.continuation.calls)
			}
		})
	}
}

func TestNextModelTurnEligibilityV1TypedNilDependenciesFailBeforeReads(t *testing.T) {
	var continuation *nextTurnContinuationReaderV1
	clock := func() time.Time { return time.Unix(1_900_000_000, 0) }
	for _, config := range []application.NextModelTurnEligibilityConfigV1{
		{Continuation: continuation, Clock: clock},
		{Continuation: &nextTurnContinuationReaderV1{}},
	} {
		if inspector, err := application.NewNextModelTurnEligibilityV1(config); inspector != nil || !core.HasReason(err, core.ReasonComponentMissing) {
			t.Fatalf("typed nil dependency was accepted: inspector=%v err=%v", inspector, err)
		}
	}
}

func TestNextModelTurnEligibilityV1LostInspectReplyUsesOriginalExactCoordinates(t *testing.T) {
	fixture := newNextTurnEligibilityFixtureV1(t)
	fixture.continuation.loseFirst = true
	inspector := newNextTurnEligibilityInspectorV1(t, fixture, func() time.Time { return fixture.now })

	if _, err := inspector.InspectNextModelTurnEligibilityV1(context.Background(), fixture.request); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost Inspect reply was not preserved: %v", err)
	}
	if fixture.continuation.calls != 1 {
		t.Fatalf("lost Inspect reply was retried internally: %d", fixture.continuation.calls)
	}
	projection, err := inspector.InspectNextModelTurnEligibilityV1(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if projection.DerivedDispatch.ValidateFor(fixture.request) != nil ||
		len(fixture.continuation.requests) != 2 ||
		fixture.continuation.requests[0] != fixture.continuation.requests[1] ||
		fixture.continuation.requests[0].AttemptRef != fixture.request.ContinuationAttempt {
		t.Fatalf("recovery changed the original exact coordinates: %+v", fixture.continuation.requests)
	}
}

func TestNextModelTurnEligibilityV1Concurrent64IsReadOnlyAndStable(t *testing.T) {
	fixture := newNextTurnEligibilityFixtureV1(t)
	inspector := newNextTurnEligibilityInspectorV1(t, fixture, func() time.Time { return fixture.now })
	const workers = 64
	refs := make(chan applicationcontract.NextModelTurnDerivedDispatchRefV1, workers)
	errs := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			projection, err := inspector.InspectNextModelTurnEligibilityV1(context.Background(), fixture.request)
			if err != nil {
				errs <- err
				return
			}
			refs <- projection.DerivedDispatch
		}()
	}
	group.Wait()
	close(refs)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	var stable applicationcontract.NextModelTurnDerivedDispatchRefV1
	for ref := range refs {
		if stable.ID == "" {
			stable = ref
		}
		if ref != stable {
			t.Fatalf("64 read-only inspections derived multiple refs:\nfirst=%+v\nnext=%+v", stable, ref)
		}
	}
	if fixture.continuation.calls != workers {
		t.Fatalf("concurrent inspection was not exactly read-only: continuation=%d", fixture.continuation.calls)
	}
}

func TestNextModelTurnEligibilityV1TTLAndClockBoundaries(t *testing.T) {
	t.Run("ModelBoundary is the minimum projection TTL", func(t *testing.T) {
		fixture := newNextTurnEligibilityFixtureV1(t)
		boundaryExpiry := fixture.now.Add(5 * time.Second)
		setNextTurnModelBoundaryExpiryV1(t, fixture, boundaryExpiry)
		inspector := newNextTurnEligibilityInspectorV1(t, fixture, func() time.Time { return fixture.now })

		projection, err := inspector.InspectNextModelTurnEligibilityV1(context.Background(), fixture.request)
		if err != nil {
			t.Fatal(err)
		}
		if projection.NotAfterUnixNano != boundaryExpiry.UnixNano() {
			t.Fatalf("projection widened ModelBoundary TTL: got %d want %d", projection.NotAfterUnixNano, boundaryExpiry.UnixNano())
		}
	})

	t.Run("boundary expires before others", func(t *testing.T) {
		fixture := newNextTurnEligibilityFixtureV1(t)
		setNextTurnModelBoundaryExpiryV1(t, fixture, fixture.now.Add(-time.Nanosecond))
		inspector := newNextTurnEligibilityInspectorV1(t, fixture, func() time.Time { return fixture.now })

		projection, err := inspector.InspectNextModelTurnEligibilityV1(context.Background(), fixture.request)
		if !core.HasReason(err, core.ReasonBindingExpired) {
			t.Fatalf("expired ModelBoundary was accepted: %v", err)
		}
		if projection != (applicationcontract.NextModelTurnEligibilityProjectionV1{}) {
			t.Fatalf("expired ModelBoundary leaked a projection: %+v", projection)
		}
		if fixture.continuation.calls != 0 {
			t.Fatalf("expired ModelBoundary reached Continuation: %d", fixture.continuation.calls)
		}
	})

	t.Run("now equals boundary expiry", func(t *testing.T) {
		fixture := newNextTurnEligibilityFixtureV1(t)
		setNextTurnModelBoundaryExpiryV1(t, fixture, fixture.now)
		inspector := newNextTurnEligibilityInspectorV1(t, fixture, func() time.Time { return fixture.now })

		projection, err := inspector.InspectNextModelTurnEligibilityV1(context.Background(), fixture.request)
		if !core.HasReason(err, core.ReasonBindingExpired) {
			t.Fatalf("exclusive ModelBoundary expiry was accepted: %v", err)
		}
		if projection != (applicationcontract.NextModelTurnEligibilityProjectionV1{}) {
			t.Fatalf("exclusive ModelBoundary expiry leaked a projection: %+v", projection)
		}
		if fixture.continuation.calls != 0 {
			t.Fatalf("exclusive ModelBoundary expiry reached Continuation: %d", fixture.continuation.calls)
		}
	})

	t.Run("sealed request crosses boundary TTL after Continuation Inspect", func(t *testing.T) {
		fixture := newNextTurnEligibilityFixtureV1(t)
		boundaryExpiry := fixture.now.Add(time.Second)
		setNextTurnModelBoundaryExpiryV1(t, fixture, boundaryExpiry)
		clock := nextTurnClockSequenceV1(t, fixture.now, fixture.now, boundaryExpiry)
		inspector := newNextTurnEligibilityInspectorV1(t, fixture, clock)

		projection, err := inspector.InspectNextModelTurnEligibilityV1(context.Background(), fixture.request)
		if !core.HasReason(err, core.ReasonBindingExpired) {
			t.Fatalf("post-seal ModelBoundary TTL crossing was accepted: %v", err)
		}
		if projection != (applicationcontract.NextModelTurnEligibilityProjectionV1{}) {
			t.Fatalf("post-seal ModelBoundary TTL crossing leaked a projection: %+v", projection)
		}
		if fixture.continuation.calls != 1 {
			t.Fatalf("ModelBoundary TTL crossing repeated Continuation read: %d", fixture.continuation.calls)
		}
	})

	t.Run("projection cannot widen ModelBoundary TTL", func(t *testing.T) {
		fixture := newNextTurnEligibilityFixtureV1(t)
		boundaryExpiry := fixture.now.Add(5 * time.Second)
		setNextTurnModelBoundaryExpiryV1(t, fixture, boundaryExpiry)
		inspector := newNextTurnEligibilityInspectorV1(t, fixture, func() time.Time { return fixture.now })
		projection, err := inspector.InspectNextModelTurnEligibilityV1(context.Background(), fixture.request)
		if err != nil {
			t.Fatal(err)
		}

		projection.NotAfterUnixNano = fixture.request.RequestedNotAfterUnixNano
		projection.Digest = ""
		projection.Digest, err = projection.DigestV1()
		if err != nil {
			t.Fatal(err)
		}
		if err = projection.ValidateFor(fixture.request, fixture.now); !core.HasReason(err, core.ReasonInvalidReference) {
			t.Fatalf("projection widened past ModelBoundary expiry: %v", err)
		}
	})

	t.Run("TTL crosses after Continuation Inspect", func(t *testing.T) {
		fixture := newNextTurnEligibilityFixtureV1(t)
		expiry := time.Unix(0, fixture.request.RequestedNotAfterUnixNano)
		clock := nextTurnClockSequenceV1(t, fixture.now, fixture.now, expiry)
		inspector := newNextTurnEligibilityInspectorV1(t, fixture, clock)
		if _, err := inspector.InspectNextModelTurnEligibilityV1(context.Background(), fixture.request); !core.HasReason(err, core.ReasonBindingExpired) {
			t.Fatalf("TTL crossing was accepted: %v", err)
		}
		if fixture.continuation.calls != 1 {
			t.Fatalf("TTL crossing repeated current reads: %d", fixture.continuation.calls)
		}
	})

	t.Run("clock rolls back after Continuation Inspect", func(t *testing.T) {
		fixture := newNextTurnEligibilityFixtureV1(t)
		clock := nextTurnClockSequenceV1(t, fixture.now, fixture.now.Add(-time.Nanosecond))
		inspector := newNextTurnEligibilityInspectorV1(t, fixture, clock)
		if _, err := inspector.InspectNextModelTurnEligibilityV1(context.Background(), fixture.request); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("clock rollback was accepted: %v", err)
		}
		if fixture.continuation.calls != 1 {
			t.Fatalf("clock rollback repeated current reads: %d", fixture.continuation.calls)
		}
	})
}

func setNextTurnModelBoundaryExpiryV1(
	t *testing.T,
	fixture *nextTurnEligibilityFixtureV1,
	expiry time.Time,
) {
	t.Helper()
	boundary := fixture.request.RuntimeActualPoint.ModelBoundary
	boundary.ExpiresUnixNano = expiry.UnixNano()
	var err error
	boundary, err = runtimeports.SealModelProviderBoundaryCurrentRefV1(boundary)
	mustNextTurnV1(t, err)
	fixture.request.RuntimeActualPoint.ModelBoundary = boundary
	fixture.request.RuntimeActualPointRequestDigest = ""
	fixture.request.Digest = ""
	fixture.request, err = applicationcontract.SealNextModelTurnEligibilityRequestV1(fixture.request)
	mustNextTurnV1(t, err)
}

func newNextTurnEligibilityInspectorV1(
	t *testing.T,
	fixture *nextTurnEligibilityFixtureV1,
	clock func() time.Time,
) *application.NextModelTurnEligibilityV1 {
	t.Helper()
	inspector, err := application.NewNextModelTurnEligibilityV1(application.NextModelTurnEligibilityConfigV1{
		Continuation: fixture.continuation,
		Clock:        clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	return inspector
}

func newNextTurnEligibilityFixtureV1(t *testing.T) *nextTurnEligibilityFixtureV1 {
	t.Helper()
	now := time.Unix(1_900_000_000, 0).UTC()
	scope := core.ExecutionScope{
		Identity:       core.AgentIdentityRef{TenantID: "tenant-next-turn", ID: "agent-next-turn", Epoch: 1},
		Lineage:        core.LineageRef{ID: "lineage-next-turn", PlanDigest: nextTurnDigestV1("plan")},
		Instance:       core.InstanceRef{ID: "instance-next-turn", Epoch: 2},
		SandboxLease:   &core.SandboxLeaseRef{ID: "lease-next-turn", Epoch: 2},
		AuthorityEpoch: 1,
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	mustNextTurnV1(t, err)
	run := applicationcontract.SingleCallRunCoordinateV1{RunID: "run-next-turn", Revision: 4, Digest: nextTurnDigestV1("run-current")}
	session := applicationcontract.SingleCallSessionCoordinateV1{
		ID: "session-next-turn", Revision: 3, Digest: nextTurnDigestV1("session-next-turn"),
		Phase:           applicationcontract.SingleCallSessionWaitingActionV1,
		CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano(),
	}
	sourceTurn := applicationcontract.SingleCallTurnCoordinateV1{
		ID: "turn:" + string(nextTurnDigestV1("turn-3")), Ordinal: 3, Revision: 3, Digest: nextTurnDigestV1("turn-3"),
	}
	source, err := applicationcontract.SealContextTurnSourceCurrentV1(applicationcontract.ContextTurnSourceCurrentV1{
		ExecutionScopeDigest: scopeDigest,
		RunID:                run.RunID,
		Session:              session,
		SessionApplicability: applicationcontract.SingleCallSessionApplicabilitySourceCoordinateV1{
			Kind: applicationcontract.SingleCallSessionSourceKindV1, ID: "session:" + string(session.Digest), Revision: session.Revision, Digest: session.Digest,
		},
		Turn: sourceTurn,
		TurnApplicability: applicationcontract.SingleCallTurnApplicabilitySourceCoordinateV1{
			Kind: applicationcontract.SingleCallTurnSourceKindV1, ID: sourceTurn.ID, Revision: sourceTurn.Revision, Digest: sourceTurn.Digest,
		},
		CheckedUnixNano: session.CheckedUnixNano,
		ExpiresUnixNano: session.ExpiresUnixNano,
	})
	mustNextTurnV1(t, err)
	oldActive, err := applicationcontract.SealHarnessActiveContextRefV1(applicationcontract.HarnessActiveContextRefV1{
		Revision: 7, ExecutionScopeDigest: scopeDigest, RunID: run.RunID, SessionID: session.ID, TurnOrdinal: sourceTurn.Ordinal,
		ManifestRef:              nextTurnContextExactV1("context/manifest", "manifest-3"),
		FrameRef:                 nextTurnContextExactV1("context/frame", "frame-3"),
		GenerationRef:            nextTurnContextExactV1("context/generation", "generation-3"),
		ContextCurrentPointerRef: nextTurnContextExactV1("context/generation-current", "current-3"),
		UpdatedUnixNano:          now.Add(-2 * time.Second).UnixNano(),
	})
	mustNextTurnV1(t, err)
	startInput := applicationcontract.TurnContinuationStartRequestV1{
		ExecutionScopeDigest: scopeDigest,
		RunID:                run.RunID,
		Source:               source,
		SettledToolResult: applicationcontract.SingleCallToolActionResultRefV2{
			ID: "result-next-turn", Revision: 1, Digest: nextTurnDigestV1("result"),
			RequestID: "request-next-turn", RequestRevision: 1, RequestDigest: nextTurnDigestV1("request"),
			ActionCoordinateDigest: nextTurnDigestV1("action"),
			ToolResultID:           "tool-result-next-turn", ToolResultRevision: 1, ToolResultDigest: nextTurnDigestV1("tool-result"),
		},
		ExpectedActiveContext:     oldActive,
		TargetTurn:                sourceTurn.Ordinal + 1,
		RequestedNotAfterUnixNano: now.Add(15 * time.Second).UnixNano(),
	}
	attemptID, err := applicationcontract.DeriveTurnContinuationAttemptIDV1(startInput)
	mustNextTurnV1(t, err)
	startInput.ExpectedContextRefreshAttempt = applicationcontract.ContextRefreshExactRefV1{
		Kind: applicationcontract.ContextTurnRefreshApplicationAttemptKindV1, ID: attemptID, Revision: 1, Digest: nextTurnDigestV1("context-attempt"),
	}
	start, err := applicationcontract.SealTurnContinuationStartRequestV1(startInput)
	mustNextTurnV1(t, err)
	pending, err := applicationcontract.SealTurnContinuationPendingV1(start, now.UnixNano(), now.Add(10*time.Second).UnixNano())
	mustNextTurnV1(t, err)
	settlement := nextTurnContextExactV1("context/apply-settlement", "apply-4")
	pointer := nextTurnContextExactV1("context/generation-current", "current-4")
	refresh, err := applicationcontract.SealContextTurnRefreshResultV1(applicationcontract.ContextTurnRefreshResultV1{
		AttemptRef:             start.ExpectedContextRefreshAttempt,
		PendingDomainResultRef: nextTurnContextExactV1("context/pending-result", "pending-4"),
		TransitionProofRef:     nextTurnContextExactV1("context/transition-proof", "proof-4"),
		ManifestRef:            nextTurnContextExactV1("context/manifest", "manifest-4"),
		FrameRef:               nextTurnContextExactV1("context/frame", "frame-4"),
		GenerationRef:          nextTurnContextExactV1("context/generation", "generation-4"),
		StableSourceSetDigest:  nextTurnDigestV1("stable"),
		S1AssociationSetDigest: nextTurnDigestV1("s1"),
		S2AssociationSetDigest: nextTurnDigestV1("s2"),
		ApplySettlementRef:     &settlement,
		CurrentPointerRef:      &pointer,
		CheckedUnixNano:        now.UnixNano(), ExpiresUnixNano: now.Add(9 * time.Second).UnixNano(),
		State: applicationcontract.ContextTurnRefreshAppliedStateV1,
	})
	mustNextTurnV1(t, err)
	commit, err := applicationcontract.SealTurnContinuationCommitRequestV1(applicationcontract.TurnContinuationCommitRequestV1{
		Pending: pending, AppliedContextRefresh: refresh, RequestedNotAfterUnixNano: now.Add(8 * time.Second).UnixNano(),
	}, now)
	mustNextTurnV1(t, err)
	current, err := applicationcontract.SealTurnContinuationContextCurrentV1(commit, now.UnixNano(), now.Add(8*time.Second).UnixNano())
	mustNextTurnV1(t, err)

	operation := runtimeports.OperationSubjectV3{
		Kind: runtimeports.OperationScopeRunV3, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, RunID: run.RunID,
		SubjectRevision: 1, CurrentProjectionRef: "run-current-next-turn",
		CurrentProjectionRevision: run.Revision, CurrentProjectionDigest: run.Digest,
	}
	operationDigest, err := operation.DigestV3()
	mustNextTurnV1(t, err)
	runtimeAttempt := runtimeports.OperationDispatchAttemptRefV3{
		OperationDigest: operationDigest, EffectID: "effect-next-turn", IntentRevision: 1, IntentDigest: nextTurnDigestV1("intent"),
		PermitID: "permit-next-turn", PermitRevision: 1, PermitDigest: nextTurnDigestV1("permit"), AttemptID: "runtime-attempt-next-turn",
	}
	boundary, err := runtimeports.SealModelProviderBoundaryCurrentRefV1(runtimeports.ModelProviderBoundaryCurrentRefV1{
		Owner: core.OwnerRef{Domain: "praxis.model", ID: "model-invoker"}, ID: "boundary-next-turn", Revision: 1,
		OperationDigest: operationDigest, EffectID: runtimeAttempt.EffectID, RuntimeAttempt: runtimeAttempt,
		DispatchSequence: 5, ProviderAttemptOrdinal: 1, AttemptRequestDigest: nextTurnDigestV1("attempt-request"),
		AcknowledgementDigest: nextTurnDigestV1("ack"), ExpiresUnixNano: now.Add(9 * time.Second).UnixNano(),
	})
	mustNextTurnV1(t, err)
	actualRequest := runtimeports.InspectCurrentModelProviderActualPointRequestV1{
		Operation: operation, EffectID: runtimeAttempt.EffectID, ExpectedEffectRevision: 3,
		PermitID: runtimeAttempt.PermitID, ExpectedPermitFactRevision: 2, PermitDigest: runtimeAttempt.PermitDigest,
		AdmissionDigest:     nextTurnDigestV1("admission"),
		ReviewAuthorization: runtimeports.OperationReviewAuthorizationRefV4{ID: "review-next-turn", Revision: 1, Digest: nextTurnDigestV1("review")},
		Attempt:             runtimeAttempt, Verifier: nextTurnProviderBindingV1(), FenceDigest: nextTurnDigestV1("fence"),
		ModelBoundary: boundary, RequestedNotAfterUnixNano: now.Add(7 * time.Second).UnixNano(),
	}
	request, err := applicationcontract.SealNextModelTurnEligibilityRequestV1(applicationcontract.NextModelTurnEligibilityRequestV1{
		ContinuationAttempt: current.Start.AttemptRefV1(), ContinuationCurrentDigest: current.Digest,
		ActiveContext: current.ActiveContext, Run: run, Session: session, TargetTurn: current.Start.TargetTurn,
		RuntimeActualPoint: actualRequest, RequestedNotAfterUnixNano: actualRequest.RequestedNotAfterUnixNano,
	})
	mustNextTurnV1(t, err)
	return &nextTurnEligibilityFixtureV1{
		now: now, request: request, current: current, pending: pending,
		continuation: &nextTurnContinuationReaderV1{current: current},
	}
}

func nextTurnProviderBindingV1() runtimeports.ProviderBindingRefV2 {
	return runtimeports.ProviderBindingRefV2{
		BindingSetID: "binding-next-turn", BindingSetRevision: 1, ComponentID: "praxis.model/provider",
		ManifestDigest: nextTurnDigestV1("manifest"), ArtifactDigest: nextTurnDigestV1("artifact"),
		Capability: runtimeports.ModelInvokeCapabilityV1,
	}
}

func nextTurnContextExactV1(kind, id string) applicationcontract.ContextRefreshExactRefV1 {
	return applicationcontract.ContextRefreshExactRefV1{
		Kind: runtimeports.NamespacedNameV2(kind), ID: id, Revision: 1, Digest: nextTurnDigestV1(id),
	}
}

func nextTurnDigestV1(value string) core.Digest {
	return core.DigestBytes([]byte(value))
}

func nextTurnClockSequenceV1(t *testing.T, values ...time.Time) func() time.Time {
	t.Helper()
	var mu sync.Mutex
	index := 0
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		if index >= len(values) {
			t.Fatalf("clock called more than %d times", len(values))
		}
		value := values[index]
		index++
		return value
	}
}

func mustNextTurnV1(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

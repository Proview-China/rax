package review_test

import (
	"context"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/caseengine"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/storetestkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/service"
	reviewsqlite "github.com/Proview-China/rax/ExecutionRuntime/review/storage/sqlite"
	"github.com/Proview-China/rax/ExecutionRuntime/review/verdictowner"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func assertCaseRevisionAbsentV2(t *testing.T, store reviewport.StoreV1, value contract.ReviewCaseV1, revision core.Revision) {
	t.Helper()
	if _, err := store.InspectCaseExactV1(context.Background(), value.TenantID, reviewport.ExactV1(value.ID, revision, testkit.Digest("absent-case-revision"))); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("unexpected Case revision %d became visible: %v", revision, err)
	}
}

func newRequestedCaseV2(t *testing.T, at time.Time, id string) (*memory.Store, *caseengine.Engine, *testkit.ManualClock, contract.TargetSnapshotV1, contract.ReviewCaseV1) {
	t.Helper()
	clock := testkit.NewClock(at)
	store := storetestkit.NewMemoryStoreV1(clock.Now)
	engine, err := caseengine.New(store, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	target := testkit.Target(clock.Now())
	testkit.PublishRubric(context.Background(), store, clock.Now(), target.TenantID)
	request := testkit.Request(clock.Now(), target, id)
	created, err := engine.CreateCaseV1(context.Background(), caseengine.CreateCaseCommandV1{CaseID: id, Request: &request, Target: target, ExpiresUnixNano: request.ExpiresUnixNano, Trace: testkit.TraceForTarget(clock.Now(), id, target, contract.TraceRequestedV1, 1, request.ID)})
	if err != nil {
		t.Fatal(err)
	}
	return store, engine, clock, target, created
}

func TestPublicMutationMatrixRequestedIsMandatoryAndExactV2(t *testing.T) {
	now := time.Unix(1_960_000_000, 0)
	t.Run("direct CreateCase empty and wrong event are zero-write", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			trace func(contract.TargetSnapshotV1) contract.TraceFactV1
		}{
			{name: "empty", trace: func(contract.TargetSnapshotV1) contract.TraceFactV1 { return contract.TraceFactV1{} }},
			{name: "wrong-event", trace: func(target contract.TargetSnapshotV1) contract.TraceFactV1 {
				return testkit.TraceForTarget(now, "case-requested-wrong", target, contract.TraceRoutedV1, 9, target.ID)
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				store := memory.NewStore()
				engine, _ := caseengine.New(store, func() time.Time { return now })
				target := testkit.Target(now)
				_, err := engine.CreateCaseV1(context.Background(), caseengine.CreateCaseCommandV1{CaseID: "case-requested-wrong", Target: target, ExpiresUnixNano: now.Add(time.Hour).UnixNano(), Trace: tc.trace(target)})
				if err == nil {
					t.Fatal("CreateCase accepted a missing/wrong Requested Trace")
				}
				if _, inspectErr := store.InspectCaseV1(context.Background(), target.TenantID, "case-requested-wrong"); !core.HasCategory(inspectErr, core.ErrorNotFound) {
					t.Fatalf("failed CreateCase leaked Case: %v", inspectErr)
				}
				if _, inspectErr := store.InspectTargetExactV1(context.Background(), target.TenantID, reviewport.ExactV1(target.ID, target.Revision, target.Digest)); !core.HasCategory(inspectErr, core.ErrorNotFound) {
					t.Fatalf("failed CreateCase leaked Target: %v", inspectErr)
				}
			})
		}
	})
	t.Run("Submit empty Requested is zero-write", func(t *testing.T) {
		store := storetestkit.NewMemoryStoreV1(func() time.Time { return time.Unix(1_900_000_010, 0) })
		testkit.PublishRubric(context.Background(), store, now, "tenant-a")
		owner, _ := service.New(store, func() time.Time { return now })
		target := testkit.Target(now)
		request := testkit.Request(now, target, "case-submit-empty-requested")
		if _, err := owner.SubmitV1(context.Background(), service.SubmitCommandV1{Request: request, Target: target}); err == nil {
			t.Fatal("Submit accepted an empty Requested Trace")
		}
		if _, err := store.InspectRequestExactV1(context.Background(), request.TenantID, reviewport.ExactV1(request.ID, request.Revision, request.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("failed Submit leaked Request: %v", err)
		}
	})
}

type lostTransitionReplyStoreV2 struct {
	*memory.Store
	cancel context.CancelFunc
	calls  atomic.Int64
}

func (s *lostTransitionReplyStoreV2) TransitionCaseWithTraceV2(ctx context.Context, mutation reviewport.TransitionCaseWithTraceMutationV2) (contract.ReviewCaseV1, error) {
	s.calls.Add(1)
	value, err := s.Store.TransitionCaseWithTraceV2(ctx, mutation)
	if err == nil {
		s.cancel()
		return contract.ReviewCaseV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected transition reply loss")
	}
	return value, err
}

func TestPublicMutationMatrixCaseTransitionIsCompoundOnlyV2(t *testing.T) {
	t.Run("legacy V1 fails closed without a read or write", func(t *testing.T) {
		store, engine, _, _, current := newRequestedCaseV2(t, time.Unix(1_961_000_000, 0), "case-transition-v1")
		_, err := engine.TransitionV1(context.Background(), caseengine.TransitionCommandV1{TenantID: current.TenantID, CaseID: current.ID, Expected: reviewport.ExpectedV1(current.Revision, current.Digest), Next: contract.CaseAdmittedV1})
		if !core.HasCategory(err, core.ErrorPreconditionFailed) {
			t.Fatalf("eventless TransitionV1 did not fail closed: %v", err)
		}
		got, inspectErr := store.InspectCaseV1(context.Background(), current.TenantID, current.ID)
		if inspectErr != nil || got.Digest != current.Digest {
			t.Fatalf("TransitionV1 changed Case: %+v err=%v", got, inspectErr)
		}
		assertCaseRevisionAbsentV2(t, store, current, current.Revision+1)
	})
	t.Run("Trace source conflict stages neither Case nor Trace", func(t *testing.T) {
		store, engine, clock, _, current := newRequestedCaseV2(t, time.Unix(1_962_000_000, 0), "case-transition-conflict")
		clock.Advance(time.Second)
		intended := testkit.TransitionTrace(clock.Now(), current, contract.CaseAdmittedV1)
		occupied := intended
		occupied.ID = "trace-transition-source-occupied"
		occupied.Event = contract.TraceCancelledV1
		occupied.Digest = ""
		occupied, _ = contract.SealTraceFactV1(occupied)
		if _, err := store.InjectTraceForTestV1(context.Background(), occupied); err != nil {
			t.Fatal(err)
		}
		_, err := engine.TransitionWithTraceV2(context.Background(), caseengine.TransitionWithTraceCommandV2{TransitionCommandV1: caseengine.TransitionCommandV1{TenantID: current.TenantID, CaseID: current.ID, Expected: reviewport.ExpectedV1(current.Revision, current.Digest), Next: contract.CaseAdmittedV1}, Trace: intended})
		if !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("transition source conflict was accepted: %v", err)
		}
		got, inspectErr := store.InspectCaseV1(context.Background(), current.TenantID, current.ID)
		if inspectErr != nil || got.Digest != current.Digest {
			t.Fatalf("failed compound transition changed Case: %+v err=%v", got, inspectErr)
		}
		if _, inspectErr = store.InspectTraceExactV1(context.Background(), intended.TenantID, reviewport.ExactV1(intended.ID, intended.Revision, intended.Digest)); !core.HasCategory(inspectErr, core.ErrorNotFound) {
			t.Fatalf("failed compound transition leaked intended Trace: %v", inspectErr)
		}
	})
	t.Run("lost reply recovers Case and Trace by detached exact Inspect", func(t *testing.T) {
		base, _, clock, _, current := newRequestedCaseV2(t, time.Unix(1_963_000_000, 0), "case-transition-lost")
		ctx, cancel := context.WithCancel(context.Background())
		wrapper := &lostTransitionReplyStoreV2{Store: base, cancel: cancel}
		engine, _ := caseengine.New(wrapper, clock.Now)
		clock.Advance(time.Second)
		trace := testkit.TransitionTrace(clock.Now(), current, contract.CaseAdmittedV1)
		got, err := engine.TransitionWithTraceV2(ctx, caseengine.TransitionWithTraceCommandV2{TransitionCommandV1: caseengine.TransitionCommandV1{TenantID: current.TenantID, CaseID: current.ID, Expected: reviewport.ExpectedV1(current.Revision, current.Digest), Next: contract.CaseAdmittedV1}, Trace: trace})
		if err != nil || got.State != contract.CaseAdmittedV1 || wrapper.calls.Load() != 1 || ctx.Err() != context.Canceled {
			t.Fatalf("detached exact transition recovery failed: case=%+v calls=%d ctx=%v err=%v", got, wrapper.calls.Load(), ctx.Err(), err)
		}
		if _, err := base.InspectTraceExactV1(context.Background(), trace.TenantID, reviewport.ExactV1(trace.ID, trace.Revision, trace.Digest)); err != nil {
			t.Fatalf("lost-reply recovery missed exact Trace: %v", err)
		}
	})
}

func TestPublicMutationMatrixHumanEventsFailClosedV2(t *testing.T) {
	t.Run("Open requires exactly one Assigned", func(t *testing.T) {
		store := storetestkit.NewMemoryStoreV1(func() time.Time { return time.Unix(1_900_000_010, 0) })
		fixture := prepareFixture(t, store)
		bad := fixture.create
		bad.Trace = contract.TraceFactV1{}
		if _, err := store.CreateHumanPanelV2(context.Background(), bad); err == nil {
			t.Fatal("OpenPanel accepted an empty Assigned Trace")
		}
		if _, err := store.InspectHumanPanelCurrentV2(context.Background(), fixture.create.OpenPanel.TenantID, fixture.create.OpenPanel.ID); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("failed OpenPanel leaked Panel: %v", err)
		}
	})
	t.Run("Open Trace conflict stages no Panel Assignment or Assigned", func(t *testing.T) {
		store := storetestkit.NewMemoryStoreV1(func() time.Time { return time.Unix(1_900_000_010, 0) })
		fixture := prepareFixture(t, store)
		intended := fixture.create.Trace
		occupied := intended
		occupied.ID = "trace-human-assigned-occupied"
		occupied.Event = contract.TraceCancelledV1
		occupied.Digest = ""
		occupied, _ = contract.SealTraceFactV1(occupied)
		if _, err := store.InjectTraceForTestV1(context.Background(), occupied); err != nil {
			t.Fatal(err)
		}
		if _, err := store.CreateHumanPanelV2(context.Background(), fixture.create); !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("OpenPanel Trace conflict was accepted: %v", err)
		}
		if _, err := store.InspectHumanPanelCurrentV2(context.Background(), fixture.create.OpenPanel.TenantID, fixture.create.OpenPanel.ID); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("failed OpenPanel leaked Panel: %v", err)
		}
		for _, assignment := range fixture.create.Assignments {
			if _, err := store.InspectHumanPanelAssignmentExactV2(context.Background(), assignment.ExactRef()); !core.HasCategory(err, core.ErrorNotFound) {
				t.Fatalf("failed OpenPanel leaked Assignment %s: %v", assignment.ID, err)
			}
		}
		if _, err := store.InspectTraceExactV1(context.Background(), intended.TenantID, reviewport.ExactV1(intended.ID, intended.Revision, intended.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("failed OpenPanel leaked Assigned Trace: %v", err)
		}
	})
	t.Run("Record requires Attested and escalation requires Escalated", func(t *testing.T) {
		store := storetestkit.NewMemoryStoreV1(func() time.Time { return time.Unix(1_900_000_010, 0) })
		fixture := prepareFixture(t, store)
		if _, err := store.CreateHumanPanelV2(context.Background(), fixture.create); err != nil {
			t.Fatal(err)
		}
		missing := fixture.vote1
		missing.Trace = contract.TraceFactV1{}
		if _, err := store.RecordHumanAttestationV2(context.Background(), missing); err == nil {
			t.Fatal("RecordAttestation accepted an empty Attested Trace")
		}
		escalating := fixture.vote1
		attestation := escalating.Attestation.Clone()
		attestation.Resolution = contract.ResolutionEscalateHumanV1
		attestation.Digest = ""
		attestation, _ = contract.SealHumanAttestationV2(attestation)
		escalating.Attestation = attestation
		escalating.AdditionalTraces = nil
		if _, err := store.RecordHumanAttestationV2(context.Background(), escalating); err == nil {
			t.Fatal("escalating Attestation accepted without Escalated Trace")
		}
		if _, err := store.InspectHumanAttestationExactV2(context.Background(), fixture.vote1.Attestation.ExactRef()); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("failed human vote leaked Attestation: %v", err)
		}
	})
	t.Run("Decide requires Verdict plus exactly one Resolved", func(t *testing.T) {
		store := storetestkit.NewMemoryStoreV1(func() time.Time { return time.Unix(1_900_000_010, 0) })
		fixture := prepareFixture(t, store)
		if _, err := store.CreateHumanPanelV2(context.Background(), fixture.create); err != nil {
			t.Fatal(err)
		}
		if _, err := store.RecordHumanAttestationV2(context.Background(), fixture.vote1); err != nil {
			t.Fatal(err)
		}
		if _, err := store.RecordHumanAttestationV2(context.Background(), fixture.vote2); err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.BeginHumanPanelDecisionV2(context.Background(), fixture.begin); err != nil {
			t.Fatal(err)
		}
		bad := fixture.decide
		bad.AdditionalTraces = nil
		if _, err := store.DecideHumanPanelV2(context.Background(), bad); err == nil {
			t.Fatal("human Decide accepted without Resolved Trace")
		}
		if _, err := store.InspectHumanVerdictExactV2(context.Background(), fixture.decide.Verdict.ExactRef()); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("failed human Decide leaked Verdict: %v", err)
		}
		current, err := store.InspectHumanPanelCurrentV2(context.Background(), fixture.decide.ExpectedPanel.TenantID, fixture.decide.ExpectedPanel.ID)
		if err != nil || current.Digest != fixture.decide.ExpectedPanel.Digest {
			t.Fatalf("failed human Decide advanced Panel: %+v err=%v", current, err)
		}
	})
	t.Run("Resolved source conflict stages no human Verdict or terminal revisions", func(t *testing.T) {
		store := storetestkit.NewMemoryStoreV1(func() time.Time { return time.Unix(1_900_000_010, 0) })
		fixture := prepareFixture(t, store)
		if _, err := store.CreateHumanPanelV2(context.Background(), fixture.create); err != nil {
			t.Fatal(err)
		}
		if _, err := store.RecordHumanAttestationV2(context.Background(), fixture.vote1); err != nil {
			t.Fatal(err)
		}
		if _, err := store.RecordHumanAttestationV2(context.Background(), fixture.vote2); err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.BeginHumanPanelDecisionV2(context.Background(), fixture.begin); err != nil {
			t.Fatal(err)
		}
		resolved := fixture.decide.AdditionalTraces[0]
		occupied := resolved
		occupied.ID = "trace-human-resolved-occupied"
		occupied.Event = contract.TraceCancelledV1
		occupied.Digest = ""
		occupied, _ = contract.SealTraceFactV1(occupied)
		if _, err := store.InjectTraceForTestV1(context.Background(), occupied); err != nil {
			t.Fatal(err)
		}
		if _, err := store.DecideHumanPanelV2(context.Background(), fixture.decide); !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("human Resolved source conflict was accepted: %v", err)
		}
		if _, err := store.InspectHumanVerdictExactV2(context.Background(), fixture.decide.Verdict.ExactRef()); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("failed human Decide leaked Verdict: %v", err)
		}
		current, err := store.InspectHumanPanelCurrentV2(context.Background(), fixture.decide.ExpectedPanel.TenantID, fixture.decide.ExpectedPanel.ID)
		if err != nil || current.Digest != fixture.decide.ExpectedPanel.Digest {
			t.Fatalf("failed human Decide advanced Panel: %+v err=%v", current, err)
		}
		caseCurrent, err := store.InspectCaseExactV1(context.Background(), fixture.decide.ExpectedCase.TenantID, reviewport.ExactV1(fixture.decide.ExpectedCase.ID, fixture.decide.ExpectedCase.Revision, fixture.decide.ExpectedCase.Digest))
		if err != nil || caseCurrent.Digest != fixture.decide.ExpectedCase.Digest {
			t.Fatalf("failed human Decide changed Case: %+v err=%v", caseCurrent, err)
		}
		for _, event := range append([]contract.TraceFactV1{fixture.decide.Trace}, fixture.decide.AdditionalTraces...) {
			if _, err := store.InspectTraceExactV1(context.Background(), event.TenantID, reviewport.ExactV1(event.ID, event.Revision, event.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
				t.Fatalf("failed human Decide leaked %s: %v", event.Event, err)
			}
		}
	})
}

func TestPublicStoreV1HasNoEventlessMutationSurfaceV2(t *testing.T) {
	for name, typ := range map[string]reflect.Type{
		"StoreV1":      reflect.TypeOf((*reviewport.StoreV1)(nil)).Elem(),
		"memory.Store": reflect.TypeOf((*memory.Store)(nil)),
		"sqlite.Store": reflect.TypeOf((*reviewsqlite.Store)(nil)),
	} {
		for _, method := range []string{"CompareAndSwapCaseV1", "CreateFindingV1", "AppendTraceV1"} {
			if _, ok := typ.MethodByName(method); ok {
				t.Fatalf("%s exposes forbidden eventless mutation %s", name, method)
			}
		}
	}
}

func TestStoreV1DirectClaimRequiresExactlyOneStartedV2(t *testing.T) {
	for _, tc := range []struct {
		name   string
		traces func(time.Time, contract.ReviewCaseV1, contract.ReviewerAssignmentV1) []contract.TraceFactV1
	}{
		{name: "empty"},
		{name: "wrong-event", traces: func(now time.Time, current contract.ReviewCaseV1, assignment contract.ReviewerAssignmentV1) []contract.TraceFactV1 {
			successor := current
			successor.Revision++
			return []contract.TraceFactV1{testkit.Trace(now, successor, contract.TraceAttestedV1, 70, assignment.ID)}
		}},
		{name: "wrong-assignment", traces: func(now time.Time, current contract.ReviewCaseV1, _ contract.ReviewerAssignmentV1) []contract.TraceFactV1 {
			return []contract.TraceFactV1{testkit.StartedTrace(now, current, "assignment-other")}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newWaitingReviewerFlowV2(t)
			f.clock.Advance(time.Second)
			var traces []contract.TraceFactV1
			if tc.traces != nil {
				traces = tc.traces(f.clock.Now(), f.caseValue, f.assignment)
			}
			_, _, err := f.store.ClaimAssignmentV1(f.ctx, reviewport.ClaimAssignmentMutationV1{
				TenantID: f.caseValue.TenantID, CaseID: f.caseValue.ID, AssignmentID: f.assignment.ID,
				ExpectedCase: reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), ExpectedAssignment: reviewport.ExpectedV1(f.assignment.Revision, f.assignment.Digest),
				LeaseHolder: f.assignment.ReviewerID, LeaseExpiresUnixNano: f.clock.Now().Add(10 * time.Minute).UnixNano(), UpdatedUnixNano: f.clock.Now().UnixNano(), Traces: traces,
			})
			if err == nil {
				t.Fatal("Store Claim accepted a missing or drifted Started Trace")
			}
			assertCurrentCaseExact(t, f.store, f.caseValue)
			assignment, inspectErr := f.store.InspectAssignmentExactV1(f.ctx, f.assignment.TenantID, reviewport.ExactV1(f.assignment.ID, f.assignment.Revision, f.assignment.Digest))
			if inspectErr != nil || assignment.Digest != f.assignment.Digest {
				t.Fatalf("failed Claim changed Assignment: %+v err=%v", assignment, inspectErr)
			}
			for _, event := range traces {
				if _, inspectErr := f.store.InspectTraceExactV1(f.ctx, event.TenantID, reviewport.ExactV1(event.ID, event.Revision, event.Digest)); !core.HasCategory(inspectErr, core.ErrorNotFound) {
					t.Fatalf("failed Claim leaked Trace: %v", inspectErr)
				}
			}
		})
	}
}

func TestStoreV1DirectAttestationRequiresExactEventClosureV2(t *testing.T) {
	t.Run("empty Attested", func(t *testing.T) {
		f := newReviewingFlow(t, contract.RouteHumanV1)
		f.clock.Advance(time.Second)
		attestation := testkit.HumanAttestation(f.clock.Now(), f.caseValue, f.round, f.assignment, contract.ResolutionAcceptV1, "direct-attestation-empty")
		_, _, err := f.store.RecordAttestationV1(f.ctx, reviewport.RecordAttestationMutationV1{Expected: reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), Attestation: attestation, NextState: contract.CaseAttestedV1})
		if err == nil {
			t.Fatal("Store RecordAttestation accepted an empty Attested Trace")
		}
		assertCurrentCaseExact(t, f.store, f.caseValue)
		if _, inspectErr := f.store.InspectAttestationExactV1(f.ctx, attestation.TenantID, reviewport.ExactV1(attestation.ID, attestation.Revision, attestation.Digest)); !core.HasCategory(inspectErr, core.ErrorNotFound) {
			t.Fatalf("failed attestation leaked fact: %v", inspectErr)
		}
	})
	for _, tc := range []struct {
		name       string
		additional func(time.Time, contract.ReviewCaseV1, contract.AttestationV1) []contract.TraceFactV1
	}{
		{name: "missing Escalated"},
		{name: "wrong Escalated event", additional: func(now time.Time, current contract.ReviewCaseV1, attestation contract.AttestationV1) []contract.TraceFactV1 {
			successor := current
			successor.Revision++
			return []contract.TraceFactV1{testkit.Trace(now, successor, contract.TraceCancelledV1, 72, attestation.ID)}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newReviewingFlow(t, contract.RouteHumanV1)
			f.clock.Advance(time.Second)
			attestation := testkit.HumanAttestation(f.clock.Now(), f.caseValue, f.round, f.assignment, contract.ResolutionEscalateHumanV1, "direct-attestation-escalate-"+tc.name)
			var additional []contract.TraceFactV1
			if tc.additional != nil {
				additional = tc.additional(f.clock.Now(), f.caseValue, attestation)
			}
			primary := testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceAttestedV1, 71, attestation.ID)
			_, _, err := f.store.RecordAttestationV1(f.ctx, reviewport.RecordAttestationMutationV1{Expected: reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), Attestation: attestation, NextState: contract.CaseWaitingHumanV1, Trace: primary, AdditionalTraces: additional})
			if err == nil {
				t.Fatal("Store RecordAttestation accepted an incomplete escalation event closure")
			}
			assertCurrentCaseExact(t, f.store, f.caseValue)
			if _, inspectErr := f.store.InspectAttestationExactV1(f.ctx, attestation.TenantID, reviewport.ExactV1(attestation.ID, attestation.Revision, attestation.Digest)); !core.HasCategory(inspectErr, core.ErrorNotFound) {
				t.Fatalf("failed escalation leaked Attestation: %v", inspectErr)
			}
			for _, event := range append([]contract.TraceFactV1{primary}, additional...) {
				if _, inspectErr := f.store.InspectTraceExactV1(f.ctx, event.TenantID, reviewport.ExactV1(event.ID, event.Revision, event.Digest)); !core.HasCategory(inspectErr, core.ErrorNotFound) {
					t.Fatalf("failed escalation leaked Trace: %v", inspectErr)
				}
			}
		})
	}
}

type captureDecideMutationStoreV2 struct {
	reviewport.StoreV1
	mutation reviewport.DecideMutationV1
}

func (s *captureDecideMutationStoreV2) DecideV1(_ context.Context, mutation reviewport.DecideMutationV1) (contract.ReviewCaseV1, contract.VerdictV1, error) {
	s.mutation = mutation
	return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "capture only")
}

func validDirectDecideMutationV2(t *testing.T) (*flow, reviewport.DecideMutationV1) {
	t.Helper()
	f := newReviewingFlow(t, contract.RouteHumanV1)
	f.clock.Advance(time.Second)
	attestation := testkit.HumanAttestation(f.clock.Now(), f.caseValue, f.round, f.assignment, contract.ResolutionAcceptV1, "direct-decide-capture")
	attested, _, err := f.engine.RecordAttestationV1(f.ctx, reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), attestation, testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceAttestedV1, 73, attestation.ID))
	if err != nil {
		t.Fatal(err)
	}
	f.caseValue = attested
	f.clock.Advance(time.Second)
	source, err := memory.NewDecisionCurrentSourceV1(f.store, &testkit.ExternalCurrentReader{}, f.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	capture := &captureDecideMutationStoreV2{StoreV1: f.store}
	owner, err := verdictowner.New(capture, source, f.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _ = owner.DecideV1(f.ctx, verdictowner.DecideCommandV1{TenantID: attested.TenantID, CaseID: attested.ID, Expected: reviewport.ExpectedV1(attested.Revision, attested.Digest), AttestationID: attestation.ID, VerdictID: "verdict-direct-decide", Trace: testkit.Trace(f.clock.Now(), attested, contract.TraceVerdictV1, 74, "verdict-direct-decide")})
	if capture.mutation.Verdict.ID == "" {
		t.Fatal("failed to capture valid Decide mutation")
	}
	return f, capture.mutation
}

func TestStoreV1DirectDecideRequiresExactlyOneResolvedV2(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*reviewport.DecideMutationV1)
	}{
		{name: "empty", mutate: func(m *reviewport.DecideMutationV1) { m.AdditionalTraces = nil }},
		{name: "wrong-event", mutate: func(m *reviewport.DecideMutationV1) {
			wrong := m.AdditionalTraces[0]
			wrong.Event = contract.TraceCancelledV1
			wrong.Digest = ""
			wrong, _ = contract.SealTraceFactV1(wrong)
			m.AdditionalTraces = []contract.TraceFactV1{wrong}
		}},
		{name: "wrong-verdict", mutate: func(m *reviewport.DecideMutationV1) {
			wrong := m.AdditionalTraces[0]
			wrong.FactRefs = []string{"verdict-other"}
			wrong.Digest = ""
			wrong, _ = contract.SealTraceFactV1(wrong)
			m.AdditionalTraces = []contract.TraceFactV1{wrong}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, mutation := validDirectDecideMutationV2(t)
			tc.mutate(&mutation)
			_, _, err := f.store.DecideV1(f.ctx, mutation)
			if err == nil {
				t.Fatal("Store Decide accepted a missing or drifted Resolved Trace")
			}
			assertCurrentCaseExact(t, f.store, f.caseValue)
			if _, inspectErr := f.store.InspectVerdictExactV1(f.ctx, mutation.Verdict.TenantID, reviewport.ExactV1(mutation.Verdict.ID, mutation.Verdict.Revision, mutation.Verdict.Digest)); !core.HasCategory(inspectErr, core.ErrorNotFound) {
				t.Fatalf("failed Decide leaked Verdict: %v", inspectErr)
			}
			for _, event := range append([]contract.TraceFactV1{mutation.Trace}, mutation.AdditionalTraces...) {
				if _, inspectErr := f.store.InspectTraceExactV1(f.ctx, event.TenantID, reviewport.ExactV1(event.ID, event.Revision, event.Digest)); !core.HasCategory(inspectErr, core.ErrorNotFound) {
					t.Fatalf("failed Decide leaked Trace: %v", inspectErr)
				}
			}
		})
	}
}

func TestStoreV1DirectDecideReplayRequiresExactHistoricalClosureV2(t *testing.T) {
	f, mutation := validDirectDecideMutationV2(t)
	resolved, verdict, err := f.store.DecideV1(f.ctx, mutation)
	if err != nil {
		t.Fatal(err)
	}
	replayCase, replayVerdict, err := f.store.DecideV1(f.ctx, mutation)
	if err != nil || replayCase.Digest != resolved.Digest || replayVerdict.Digest != verdict.Digest {
		t.Fatalf("canonical Decide replay did not return the exact historical closure: case=%+v verdict=%+v err=%v", replayCase, replayVerdict, err)
	}

	tests := []struct {
		name   string
		mutate func(*reviewport.DecideMutationV1)
	}{
		{"missing_resolved", func(m *reviewport.DecideMutationV1) { m.AdditionalTraces = nil }},
		{"changed_primary", func(m *reviewport.DecideMutationV1) {
			m.Trace.CorrelationID = "different-case"
			m.Trace.Digest = ""
			m.Trace, _ = contract.SealTraceFactV1(m.Trace)
		}},
		{"changed_resolved", func(m *reviewport.DecideMutationV1) {
			m.AdditionalTraces[0].CausationID = "different-verdict"
			m.AdditionalTraces[0].Digest = ""
			m.AdditionalTraces[0], _ = contract.SealTraceFactV1(m.AdditionalTraces[0])
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			changed := mutation
			changed.AdditionalTraces = append([]contract.TraceFactV1(nil), mutation.AdditionalTraces...)
			tc.mutate(&changed)
			if _, _, err := f.store.DecideV1(f.ctx, changed); !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("changed Decide replay did not conflict: %v", err)
			}
			for _, event := range append([]contract.TraceFactV1{mutation.Trace}, mutation.AdditionalTraces...) {
				if _, err := f.store.InspectTraceExactV1(f.ctx, event.TenantID, reviewport.ExactV1(event.ID, event.Revision, event.Digest)); err != nil {
					t.Fatalf("changed replay damaged original %s Trace: %v", event.Event, err)
				}
			}
		})
	}
}

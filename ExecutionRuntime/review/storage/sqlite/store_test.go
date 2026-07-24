package sqlite_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/caseengine"
	"github.com/Proview-China/rax/ExecutionRuntime/review/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/decisioncurrent"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/service"
	reviewsqlite "github.com/Proview-China/rax/ExecutionRuntime/review/storage/sqlite"
	"github.com/Proview-China/rax/ExecutionRuntime/review/verdictowner"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestSQLiteStoreConformanceAndRestartV1(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "review.sqlite")
	now := time.Unix(1_750_000_000, 0)
	clock := func() time.Time { return now }
	store, err := reviewsqlite.Open(ctx, reviewsqlite.Config{Path: path, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	target, base, next, trace := conformanceFixture(t, now)
	if err := conformance.CheckStoreV1(ctx, store, conformance.StoreFixtureV1{Target: target, Case: base, Trace: trace, Next: next, NextTrace: testkit.TransitionTrace(now.Add(time.Second), base, contract.CaseAdmittedV1)}); err != nil {
		t.Fatal(err)
	}
	if err := store.IntegrityCheckV1(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := reviewsqlite.Open(ctx, reviewsqlite.Config{Path: path, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	got, err := restarted.InspectCaseExactV1(ctx, base.TenantID, reviewport.ExactV1(base.ID, base.Revision, base.Digest))
	if err != nil || got.Digest != base.Digest {
		t.Fatalf("historical case did not survive restart: %+v err=%v", got, err)
	}
	gotTrace, err := restarted.InspectTraceExactV1(ctx, trace.TenantID, reviewport.ExactV1(trace.ID, trace.Revision, trace.Digest))
	if err != nil || gotTrace.Digest != trace.Digest {
		t.Fatalf("creation trace did not survive restart: %+v err=%v", gotTrace, err)
	}
}

func TestConditionV2SQLiteExactAttestationSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "review-condition-v2.sqlite")
	clock := testkit.NewClock(time.Unix(1_981_000_000, 0))
	store, err := reviewsqlite.Open(ctx, reviewsqlite.Config{Path: path, Clock: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := caseengine.New(store, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	target := testkit.Target(clock.Now())
	testkit.PublishRubric(ctx, store, clock.Now(), target.TenantID)
	request := testkit.Request(clock.Now(), target, "case-condition-v2")
	clock.Advance(time.Second)
	caseFact, err := engine.CreateCaseV1(ctx, caseengine.CreateCaseCommandV1{CaseID: "case-condition-v2", Request: &request, Target: target, ExpiresUnixNano: request.ExpiresUnixNano, Trace: testkit.TraceForTarget(clock.Now(), "case-condition-v2", target, contract.TraceRequestedV1, 1, request.ID)})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []contract.CaseStateV1{contract.CaseAdmittedV1, contract.CaseRoutedV1} {
		clock.Advance(time.Second)
		caseFact, err = engine.TransitionWithTraceV2(ctx, caseengine.TransitionWithTraceCommandV2{TransitionCommandV1: caseengine.TransitionCommandV1{TenantID: caseFact.TenantID, CaseID: caseFact.ID, Expected: reviewport.ExpectedV1(caseFact.Revision, caseFact.Digest), Next: state}, Trace: testkit.TransitionTrace(clock.Now(), caseFact, state)})
		if err != nil {
			t.Fatal(err)
		}
	}
	clock.Advance(time.Second)
	round := testkit.Round(clock.Now(), caseFact, contract.RouteHumanV1)
	assignment := testkit.Assignment(clock.Now(), caseFact, round, contract.RouteHumanV1)
	caseFact, round, assignment, err = engine.StartRoundV1(ctx, reviewport.StartRoundMutationV1{Expected: reviewport.ExpectedV1(caseFact.Revision, caseFact.Digest), Round: round, Assignment: assignment, Trace: testkit.Trace(clock.Now(), caseFact, contract.TraceAssignedV1, 2, round.ID, assignment.ID)})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	caseFact, assignment, err = engine.ClaimAssignmentV1(ctx, reviewport.ClaimAssignmentMutationV1{TenantID: caseFact.TenantID, ExpectedCase: reviewport.ExpectedV1(caseFact.Revision, caseFact.Digest), ExpectedAssignment: reviewport.ExpectedV1(assignment.Revision, assignment.Digest), CaseID: caseFact.ID, AssignmentID: assignment.ID, LeaseHolder: "worker-condition-v2", LeaseExpiresUnixNano: clock.Now().Add(15 * time.Minute).UnixNano(), UpdatedUnixNano: clock.Now().UnixNano(), Traces: []contract.TraceFactV1{testkit.StartedTrace(clock.Now(), caseFact, assignment.ID)}})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	attestation := testkit.HumanAttestation(clock.Now(), caseFact, round, assignment, contract.ResolutionConditionalV1, "condition-v2-sqlite")
	caseFact, attestation, err = engine.RecordAttestationV1(ctx, reviewport.ExpectedV1(caseFact.Revision, caseFact.Digest), attestation, testkit.Trace(clock.Now(), caseFact, contract.TraceAttestedV1, 3, attestation.ID))
	if err != nil {
		t.Fatal(err)
	}
	if len(attestation.Conditions) != 1 || attestation.ConditionsDigest == "" {
		t.Fatal("conditional Attestation lost exact condition set before persistence")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := reviewsqlite.Open(ctx, reviewsqlite.Config{Path: path, Clock: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	stored, err := restarted.InspectAttestationExactV1(ctx, attestation.TenantID, reviewport.ExactV1(attestation.ID, attestation.Revision, attestation.Digest))
	if err != nil || stored.ConditionsDigest != attestation.ConditionsDigest || len(stored.Conditions) != 1 || stored.Conditions[0] != attestation.Conditions[0] {
		t.Fatalf("SQLite restart lost exact condition set: %+v err=%v", stored, err)
	}
	current, err := restarted.InspectCaseExactV1(ctx, caseFact.TenantID, reviewport.ExactV1(caseFact.ID, caseFact.Revision, caseFact.Digest))
	if err != nil || current.State != contract.CaseAttestedV1 {
		t.Fatalf("SQLite restart lost atomic attested Case: %+v err=%v", current, err)
	}
}

func TestSQLiteRequestTargetCaseTraceAtomicRestartV1(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "review.sqlite")
	now := time.Unix(1_755_000_000, 0)
	store, err := reviewsqlite.Open(ctx, reviewsqlite.Config{Path: path, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	testkit.PublishRubric(ctx, store, now, "tenant-a")
	owner, err := service.New(store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	target := testkit.Target(now)
	bundle := testkit.ResultBundle(now, target.TenantID, "bundle-request-sqlite")
	request := testkit.Request(now, target, "case-request-sqlite")
	request.ResultBundle = &contract.ExactResourceRefV1{ID: bundle.ID, Revision: bundle.Revision, Digest: bundle.Digest}
	request.Digest = ""
	request, _ = contract.SealReviewRequestV1(request)
	trace := testkit.TraceForTarget(now, request.CaseID, target, contract.TraceRequestedV1, 1, request.ID)
	if _, err := owner.SubmitV1(ctx, service.SubmitCommandV1{Request: request, ResultBundle: &bundle, Target: target, Trace: trace}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := reviewsqlite.Open(ctx, reviewsqlite.Config{Path: path, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	stored, err := restarted.InspectRequestByIdempotencyV1(ctx, request.TenantID, request.IdempotencyKey)
	if err != nil || stored.Digest != request.Digest {
		t.Fatalf("request did not survive restart: %+v %v", stored, err)
	}
	byCase, err := restarted.InspectRequestByCaseV1(ctx, request.TenantID, request.CaseID)
	if err != nil || byCase.Digest != request.Digest {
		t.Fatalf("Case request index did not survive restart: %+v %v", byCase, err)
	}
	storedBundle, err := restarted.InspectResultBundleExactV1(ctx, bundle.TenantID, reviewport.ExactV1(bundle.ID, bundle.Revision, bundle.Digest))
	if err != nil || storedBundle.Digest != bundle.Digest {
		t.Fatalf("Result Bundle did not survive restart: %+v %v", storedBundle, err)
	}
}

func TestSQLiteTraceEventV2CompoundFindingAndPageSurviveRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "review-trace-v2.sqlite")
	clock := testkit.NewClock(time.Unix(1_955_000_000, 0))
	store, err := reviewsqlite.Open(ctx, reviewsqlite.Config{Path: path, Clock: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	testkit.PublishRubric(ctx, store, clock.Now(), "tenant-a")
	owner, err := service.New(store, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	target := testkit.Target(clock.Now())
	request := testkit.Request(clock.Now(), target, "case-trace-restart-v2")
	created, err := owner.SubmitV1(ctx, service.SubmitCommandV1{Request: request, Target: target, Trace: testkit.TraceForTarget(clock.Now(), request.CaseID, target, contract.TraceRequestedV1, 1, request.ID)})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := caseengine.New(store, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	c := created.Case
	for _, state := range []contract.CaseStateV1{contract.CaseAdmittedV1, contract.CaseRoutedV1} {
		clock.Advance(time.Second)
		c, err = engine.TransitionWithTraceV2(ctx, caseengine.TransitionWithTraceCommandV2{TransitionCommandV1: caseengine.TransitionCommandV1{TenantID: c.TenantID, CaseID: c.ID, Expected: reviewport.ExpectedV1(c.Revision, c.Digest), Next: state}, Trace: testkit.TransitionTrace(clock.Now(), c, state)})
		if err != nil {
			t.Fatal(err)
		}
	}
	clock.Advance(time.Second)
	round := testkit.Round(clock.Now(), c, contract.RouteHumanV1)
	assignment := testkit.Assignment(clock.Now(), c, round, contract.RouteHumanV1)
	c, round, assignment, err = engine.StartRoundV1(ctx, reviewport.StartRoundMutationV1{Expected: reviewport.ExpectedV1(c.Revision, c.Digest), Round: round, Assignment: assignment, Trace: testkit.Trace(clock.Now(), c, contract.TraceAssignedV1, 2, round.ID, assignment.ID)})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	successor := c
	successor.Revision++
	started := testkit.Trace(clock.Now(), successor, contract.TraceStartedV1, 3, assignment.ID)
	c, assignment, err = engine.ClaimAssignmentV1(ctx, reviewport.ClaimAssignmentMutationV1{TenantID: c.TenantID, CaseID: c.ID, AssignmentID: assignment.ID, ExpectedCase: reviewport.ExpectedV1(c.Revision, c.Digest), ExpectedAssignment: reviewport.ExpectedV1(assignment.Revision, assignment.Digest), LeaseHolder: "reviewer-a", LeaseExpiresUnixNano: clock.Now().Add(10 * time.Minute).UnixNano(), UpdatedUnixNano: clock.Now().UnixNano(), Traces: []contract.TraceFactV1{started}})
	if err != nil {
		t.Fatal(err)
	}
	evidence := []runtimeports.ReviewEvidenceRefV2{testkit.Evidence("sqlite-finding-v2")}
	finding, err := contract.SealFindingV1(contract.FindingV1{FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: c.TenantID, ID: "finding-sqlite-v2", Revision: 1, CreatedUnixNano: clock.Now().UnixNano(), UpdatedUnixNano: clock.Now().UnixNano()}, CaseID: c.ID, CaseRevision: c.Revision, RoundID: round.ID, RoundRevision: round.Revision, RoundDigest: round.Digest, TargetID: c.TargetID, TargetRevision: c.TargetRevision, TargetDigest: c.TargetDigest, Category: "review.test/sqlite", Priority: "high", Anchor: "sqlite", Claim: "compound persistence", Impact: "restart must preserve it", Evidence: evidence, Status: contract.FindingOpenV1, ExpiresUnixNano: clock.Now().Add(5 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	findingTrace := testkit.Trace(clock.Now(), c, contract.TraceFindingV1, 4, finding.ID)
	if _, err := store.CreateFindingWithTraceV2(ctx, reviewport.CreateFindingWithTraceMutationV2{Finding: finding, Trace: findingTrace}); err != nil {
		t.Fatal(err)
	}
	if err := conformance.CheckTraceEventStoreV2(ctx, store, conformance.TraceEventStoreFixtureV2{Mutation: reviewport.CreateFindingWithTraceMutationV2{Finding: finding, Trace: findingTrace}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := reviewsqlite.Open(ctx, reviewsqlite.Config{Path: path, Clock: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	page, err := restarted.ListTracePageV2(ctx, reviewport.ListTracePageRequestV2{TenantID: c.TenantID, CaseID: c.ID, Limit: reviewport.MaxTracePageV2})
	if err != nil {
		t.Fatal(err)
	}
	foundStarted, foundFinding := false, false
	for _, event := range page.Events {
		foundStarted = foundStarted || event.Digest == started.Digest
		foundFinding = foundFinding || event.Digest == findingTrace.Digest
	}
	if !foundStarted || !foundFinding {
		t.Fatalf("restart lost compound events: started=%v finding=%v events=%d", foundStarted, foundFinding, len(page.Events))
	}
	storedFinding, err := restarted.InspectFindingExactV1(ctx, finding.TenantID, reviewport.ExactV1(finding.ID, finding.Revision, finding.Digest))
	if err != nil || storedFinding.Digest != finding.Digest {
		t.Fatalf("restart lost exact Finding: %+v err=%v", storedFinding, err)
	}
}

func TestSQLiteInvalidateTraceFailureLeaksNoSnapshotOrHistoryV2(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(contract.TraceFactV1, contract.TraceFactV1) contract.TraceFactV1
	}{
		{name: "malformed exact target binding", mutate: func(intended, _ contract.TraceFactV1) contract.TraceFactV1 {
			intended.TargetDigest = testkit.Digest("different-target")
			intended.Digest = ""
			intended, _ = contract.SealTraceFactV1(intended)
			return intended
		}},
		{name: "staged source conflict", mutate: func(intended, occupied contract.TraceFactV1) contract.TraceFactV1 {
			intended.SourceID = occupied.SourceID
			intended.SourceEpoch = occupied.SourceEpoch
			intended.SourceSequence = occupied.SourceSequence
			intended.Digest = ""
			intended, _ = contract.SealTraceFactV1(intended)
			return intended
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "review-invalidate.sqlite")
			baseTime := time.Unix(1_957_000_000, 0)
			store, err := reviewsqlite.Open(ctx, reviewsqlite.Config{Path: path, Clock: func() time.Time { return baseTime }})
			if err != nil {
				t.Fatal(err)
			}
			target, requested, _, requestedTrace := conformanceFixture(t, baseTime)
			if _, err = store.CreateTargetCaseV1(ctx, reviewport.CreateTargetCaseMutationV1{Target: target, Case: requested, Trace: requestedTrace}); err != nil {
				t.Fatal(err)
			}
			admitted := nextCase(t, requested, contract.CaseAdmittedV1, baseTime.Add(time.Second), "")
			admittedTrace := testkit.TransitionTrace(baseTime.Add(time.Second), requested, contract.CaseAdmittedV1)
			if _, err = store.TransitionCaseWithTraceV2(ctx, reviewport.TransitionCaseWithTraceMutationV2{Expected: reviewport.ExpectedV1(requested.Revision, requested.Digest), Next: admitted, Trace: admittedTrace}); err != nil {
				t.Fatal(err)
			}
			at := baseTime.Add(2 * time.Second)
			intended := tc.mutate(testkit.Trace(at, admitted, contract.TraceCancelledV1, 99, admitted.ID), admittedTrace)
			expected := nextCase(t, admitted, contract.CaseCancelledV1, at, core.ReasonInvalidState)
			_, _, err = store.InvalidateV1(ctx, reviewport.InvalidateMutationV1{TenantID: admitted.TenantID, Expected: reviewport.ExpectedV1(admitted.Revision, admitted.Digest), CaseID: admitted.ID, CaseState: contract.CaseCancelledV1, VerdictState: contract.VerdictRevokedV1, Reason: core.ReasonInvalidState, UpdatedUnixNano: at.UnixNano(), Trace: intended})
			if !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("SQLite Invalidate accepted invalid Trace closure: %v", err)
			}
			if err = store.Close(); err != nil {
				t.Fatal(err)
			}
			restarted, err := reviewsqlite.Open(ctx, reviewsqlite.Config{Path: path, Clock: func() time.Time { return at }})
			if err != nil {
				t.Fatal(err)
			}
			defer restarted.Close()
			current, err := restarted.InspectCaseV1(ctx, admitted.TenantID, admitted.ID)
			if err != nil || current.Digest != admitted.Digest || current.Revision != admitted.Revision {
				t.Fatalf("failed SQLite Invalidate changed durable current Case: %+v err=%v", current, err)
			}
			if _, err = restarted.InspectCaseExactV1(ctx, expected.TenantID, reviewport.ExactV1(expected.ID, expected.Revision, expected.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
				t.Fatalf("failed SQLite Invalidate leaked successor Case history: %v", err)
			}
			if _, err = restarted.InspectTraceExactV1(ctx, intended.TenantID, reviewport.ExactV1(intended.ID, intended.Revision, intended.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
				t.Fatalf("failed SQLite Invalidate leaked intended Trace: %v", err)
			}
		})
	}
}

func TestSQLiteStoreConcurrentGenerationCASV1(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "review.sqlite")
	now := time.Unix(1_760_000_000, 0)
	clock := func() time.Time { return now }
	left, err := reviewsqlite.Open(ctx, reviewsqlite.Config{Path: path, Clock: clock, BusyTimeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer left.Close()
	right, err := reviewsqlite.Open(ctx, reviewsqlite.Config{Path: path, Clock: clock, BusyTimeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer right.Close()
	target, base, _, trace := conformanceFixture(t, now)
	if _, err := left.CreateTargetCaseV1(ctx, reviewport.CreateTargetCaseMutationV1{Target: target, Case: base, Trace: trace}); err != nil {
		t.Fatal(err)
	}
	nextA := nextCase(t, base, contract.CaseAdmittedV1, now.Add(time.Second), "")
	nextB := nextCase(t, base, contract.CaseAdmittedV1, now.Add(2*time.Second), "")

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wait sync.WaitGroup
	for _, item := range []struct {
		store *reviewsqlite.Store
		next  contract.ReviewCaseV1
		trace contract.TraceFactV1
	}{{left, nextA, testkit.TransitionTrace(now.Add(time.Second), base, contract.CaseAdmittedV1)}, {right, nextB, testkit.TransitionTrace(now.Add(2*time.Second), base, contract.CaseAdmittedV1)}} {
		wait.Add(1)
		go func(item struct {
			store *reviewsqlite.Store
			next  contract.ReviewCaseV1
			trace contract.TraceFactV1
		}) {
			defer wait.Done()
			<-start
			_, mutationErr := item.store.TransitionCaseWithTraceV2(ctx, reviewport.TransitionCaseWithTraceMutationV2{Expected: reviewport.ExpectedV1(base.Revision, base.Digest), Next: item.next, Trace: item.trace})
			errs <- mutationErr
		}(item)
	}
	close(start)
	wait.Wait()
	close(errs)
	success, conflict := 0, 0
	for mutationErr := range errs {
		if mutationErr == nil {
			success++
		} else if core.HasCategory(mutationErr, core.ErrorConflict) {
			conflict++
		} else {
			t.Fatalf("unexpected concurrent error: %v", mutationErr)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("expected one winner and one conflict, success=%d conflict=%d", success, conflict)
	}
}

func TestSQLiteStoreRejectsCorruptSnapshotV1(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "review.sqlite")
	now := time.Unix(1_770_000_000, 0)
	store, err := reviewsqlite.Open(ctx, reviewsqlite.Config{Path: path, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	target, base, _, trace := conformanceFixture(t, now)
	if _, err := store.CreateTargetCaseV1(ctx, reviewport.CreateTargetCaseMutationV1{Target: target, Case: base, Trace: trace}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", (&urlForTest{path: path}).dsn())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE review_owner_state SET canonical_json=? WHERE tenant_id=?`, []byte(`{"contract_version":"corrupt"}`), string(base.TenantID)); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	restarted, err := reviewsqlite.Open(ctx, reviewsqlite.Config{Path: path, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if _, err := restarted.InspectCaseV1(ctx, base.TenantID, base.ID); err == nil || (!core.HasCategory(err, core.ErrorConflict) && !core.HasCategory(err, core.ErrorInternal)) {
		t.Fatalf("corrupt snapshot did not fail closed: %v", err)
	}
}

func TestSQLiteOpenRejectsMissingDirectoryV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "review.sqlite")
	if _, err := reviewsqlite.Open(context.Background(), reviewsqlite.Config{Path: path}); err == nil {
		t.Fatal("missing parent directory was accepted")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("failed open unexpectedly created database: %v", err)
	}
}

func TestSQLiteDecisionOwnerInputsUseOneDurableSnapshotAndDeepCloneV1(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "review.sqlite")
	now := time.Unix(1_780_000_000, 0)
	current := now
	store, err := reviewsqlite.Open(ctx, reviewsqlite.Config{Path: path, Clock: func() time.Time { return current }})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := caseengine.New(store, func() time.Time { return current })
	if err != nil {
		t.Fatal(err)
	}
	target := testkit.Target(now)
	testkit.PublishRubric(ctx, store, now, target.TenantID)
	admissionRequest := testkit.Request(now, target, "case-decision-inputs")
	caseValue, err := engine.CreateCaseV1(ctx, caseengine.CreateCaseCommandV1{CaseID: "case-decision-inputs", Request: &admissionRequest, Target: target, ExpiresUnixNano: admissionRequest.ExpiresUnixNano, Trace: testkit.TraceForTarget(current, "case-decision-inputs", target, contract.TraceRequestedV1, 1, admissionRequest.ID)})
	if err != nil {
		t.Fatal(err)
	}
	current = now.Add(time.Second)
	caseValue, err = engine.TransitionWithTraceV2(ctx, caseengine.TransitionWithTraceCommandV2{TransitionCommandV1: caseengine.TransitionCommandV1{TenantID: caseValue.TenantID, CaseID: caseValue.ID, Expected: reviewport.ExpectedV1(caseValue.Revision, caseValue.Digest), Next: contract.CaseAdmittedV1}, Trace: testkit.TransitionTrace(current, caseValue, contract.CaseAdmittedV1)})
	if err != nil {
		t.Fatal(err)
	}
	current = now.Add(2 * time.Second)
	caseValue, err = engine.TransitionWithTraceV2(ctx, caseengine.TransitionWithTraceCommandV2{TransitionCommandV1: caseengine.TransitionCommandV1{TenantID: caseValue.TenantID, CaseID: caseValue.ID, Expected: reviewport.ExpectedV1(caseValue.Revision, caseValue.Digest), Next: contract.CaseRoutedV1}, Trace: testkit.TransitionTrace(current, caseValue, contract.CaseRoutedV1)})
	if err != nil {
		t.Fatal(err)
	}
	current = now.Add(3 * time.Second)
	round := testkit.Round(current, caseValue, contract.RouteHumanV1)
	assignment := testkit.Assignment(current, caseValue, round, contract.RouteHumanV1)
	assignedTrace := testkit.Trace(current, caseValue, contract.TraceAssignedV1, 1, round.ID, assignment.ID)
	caseValue, round, assignment, err = engine.StartRoundV1(ctx, reviewport.StartRoundMutationV1{Expected: reviewport.ExpectedV1(caseValue.Revision, caseValue.Digest), Round: round, Assignment: assignment, Trace: assignedTrace})
	if err != nil {
		t.Fatal(err)
	}
	current = now.Add(4 * time.Second)
	caseValue, assignment, err = engine.ClaimAssignmentV1(ctx, reviewport.ClaimAssignmentMutationV1{TenantID: caseValue.TenantID, ExpectedCase: reviewport.ExpectedV1(caseValue.Revision, caseValue.Digest), ExpectedAssignment: reviewport.ExpectedV1(assignment.Revision, assignment.Digest), CaseID: caseValue.ID, AssignmentID: assignment.ID, LeaseHolder: assignment.ReviewerID, LeaseExpiresUnixNano: current.Add(15 * time.Minute).UnixNano(), UpdatedUnixNano: current.UnixNano(), Traces: []contract.TraceFactV1{testkit.StartedTrace(current, caseValue, assignment.ID)}})
	if err != nil {
		t.Fatal(err)
	}
	current = now.Add(5 * time.Second)
	attestation := testkit.HumanAttestation(current, caseValue, round, assignment, contract.ResolutionAcceptV1, "sqlite-decision-inputs")
	attestedTrace := testkit.Trace(current, caseValue, contract.TraceAttestedV1, 2, attestation.ID)
	caseValue, attestation, err = engine.RecordAttestationV1(ctx, reviewport.ExpectedV1(caseValue.Revision, caseValue.Digest), attestation, attestedTrace)
	if err != nil {
		t.Fatal(err)
	}
	request := reviewport.DecisionCurrentRequestV1{TenantID: caseValue.TenantID, CaseID: caseValue.ID, ExpectedCase: reviewport.ExpectedV1(caseValue.Revision, caseValue.Digest), AttestationID: attestation.ID}
	resolvedRequest, err := store.ResolveDecisionCurrentRequestV1(ctx, reviewport.DecisionCurrentResolveRequestV1{TenantID: caseValue.TenantID, CaseID: caseValue.ID, ExpectedCase: reviewport.ExpectedV1(caseValue.Revision, caseValue.Digest)})
	if err != nil || resolvedRequest != request {
		t.Fatalf("durable current Attestation resolution failed: %+v err=%v", resolvedRequest, err)
	}
	first, err := store.InspectDecisionOwnerInputsV1(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Case.Digest != caseValue.Digest || first.Target.Digest != target.Digest || first.Round.Digest != round.Digest || first.Assignment.Digest != assignment.Digest || first.Attestation.Digest != attestation.Digest || len(first.Evidence) == 0 {
		t.Fatalf("durable Decision inputs were incomplete: %+v", first)
	}
	first.Evidence[0].Ref = "mutated-by-caller"
	if len(first.Findings) != 0 {
		first.Findings[0].ID = "mutated-by-caller"
	}
	second, err := store.InspectDecisionOwnerInputsV1(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if second.Evidence[0].Ref == "mutated-by-caller" {
		t.Fatal("Decision inputs leaked a mutable alias")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := reviewsqlite.Open(ctx, reviewsqlite.Config{Path: path, Clock: func() time.Time { return current }})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	afterRestart, err := restarted.InspectDecisionOwnerInputsV1(ctx, request)
	if err != nil || afterRestart.Case.Digest != second.Case.Digest || afterRestart.Attestation.Digest != second.Attestation.Digest {
		t.Fatalf("Decision inputs did not survive restart: %+v err=%v", afterRestart, err)
	}
}

func TestSQLiteVerdictOwnerUsesDurableDecisionSnapshotAndSurvivesRestartV1(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "review.sqlite")
	base := time.Unix(1_785_000_000, 0)
	current := base
	store, err := reviewsqlite.Open(ctx, reviewsqlite.Config{Path: path, Clock: func() time.Time { return current }})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := caseengine.New(store, func() time.Time { return current })
	if err != nil {
		t.Fatal(err)
	}
	target := testkit.Target(base)
	testkit.PublishRubric(ctx, store, base, target.TenantID)
	request := testkit.Request(base, target, "case-durable-verdict")
	caseFact, err := engine.CreateCaseV1(ctx, caseengine.CreateCaseCommandV1{CaseID: "case-durable-verdict", Request: &request, Target: target, ExpiresUnixNano: request.ExpiresUnixNano, Trace: testkit.TraceForTarget(current, "case-durable-verdict", target, contract.TraceRequestedV1, 1, request.ID)})
	if err != nil {
		t.Fatal(err)
	}
	current = base.Add(time.Second)
	caseFact, err = engine.TransitionWithTraceV2(ctx, caseengine.TransitionWithTraceCommandV2{TransitionCommandV1: caseengine.TransitionCommandV1{TenantID: caseFact.TenantID, CaseID: caseFact.ID, Expected: reviewport.ExpectedV1(caseFact.Revision, caseFact.Digest), Next: contract.CaseAdmittedV1}, Trace: testkit.TransitionTrace(current, caseFact, contract.CaseAdmittedV1)})
	if err != nil {
		t.Fatal(err)
	}
	current = base.Add(2 * time.Second)
	caseFact, err = engine.TransitionWithTraceV2(ctx, caseengine.TransitionWithTraceCommandV2{TransitionCommandV1: caseengine.TransitionCommandV1{TenantID: caseFact.TenantID, CaseID: caseFact.ID, Expected: reviewport.ExpectedV1(caseFact.Revision, caseFact.Digest), Next: contract.CaseRoutedV1}, Trace: testkit.TransitionTrace(current, caseFact, contract.CaseRoutedV1)})
	if err != nil {
		t.Fatal(err)
	}
	current = base.Add(3 * time.Second)
	round := testkit.Round(current, caseFact, contract.RouteHumanV1)
	assignment := testkit.Assignment(current, caseFact, round, contract.RouteHumanV1)
	caseFact, round, assignment, err = engine.StartRoundV1(ctx, reviewport.StartRoundMutationV1{
		Expected:   reviewport.ExpectedV1(caseFact.Revision, caseFact.Digest),
		Round:      round,
		Assignment: assignment,
		Trace:      testkit.Trace(current, caseFact, contract.TraceAssignedV1, 1, round.ID, assignment.ID),
	})
	if err != nil {
		t.Fatal(err)
	}
	current = base.Add(4 * time.Second)
	caseFact, assignment, err = engine.ClaimAssignmentV1(ctx, reviewport.ClaimAssignmentMutationV1{
		TenantID:             caseFact.TenantID,
		ExpectedCase:         reviewport.ExpectedV1(caseFact.Revision, caseFact.Digest),
		ExpectedAssignment:   reviewport.ExpectedV1(assignment.Revision, assignment.Digest),
		CaseID:               caseFact.ID,
		AssignmentID:         assignment.ID,
		LeaseHolder:          assignment.ReviewerID,
		LeaseExpiresUnixNano: current.Add(15 * time.Minute).UnixNano(),
		UpdatedUnixNano:      current.UnixNano(),
		Traces:               []contract.TraceFactV1{testkit.StartedTrace(current, caseFact, assignment.ID)},
	})
	if err != nil {
		t.Fatal(err)
	}
	current = base.Add(5 * time.Second)
	attestation := testkit.HumanAttestation(current, caseFact, round, assignment, contract.ResolutionAcceptV1, "durable-verdict")
	caseFact, attestation, err = engine.RecordAttestationV1(ctx, reviewport.ExpectedV1(caseFact.Revision, caseFact.Digest), attestation, testkit.Trace(current, caseFact, contract.TraceAttestedV1, 2, attestation.ID))
	if err != nil {
		t.Fatal(err)
	}
	currentSource, err := decisioncurrent.NewSourceV1(store, &testkit.ExternalCurrentReader{}, func() time.Time { return current })
	if err != nil {
		t.Fatal(err)
	}
	owner, err := verdictowner.New(store, currentSource, func() time.Time { return current })
	if err != nil {
		t.Fatal(err)
	}
	current = base.Add(6 * time.Second)
	resolved, verdict, err := owner.DecideV1(ctx, verdictowner.DecideCommandV1{
		TenantID:      caseFact.TenantID,
		CaseID:        caseFact.ID,
		Expected:      reviewport.ExpectedV1(caseFact.Revision, caseFact.Digest),
		AttestationID: attestation.ID,
		VerdictID:     "verdict-durable-sqlite",
		Trace:         testkit.Trace(current, caseFact, contract.TraceVerdictV1, 3, "verdict-durable-sqlite"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.State != contract.CaseResolvedV1 || verdict.State != contract.VerdictAcceptedV1 {
		t.Fatalf("durable Verdict did not resolve the Case: case=%+v verdict=%+v", resolved, verdict)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := reviewsqlite.Open(ctx, reviewsqlite.Config{Path: path, Clock: func() time.Time { return current }})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	storedCase, err := restarted.InspectCaseExactV1(ctx, resolved.TenantID, reviewport.ExactV1(resolved.ID, resolved.Revision, resolved.Digest))
	if err != nil || storedCase.Digest != resolved.Digest {
		t.Fatalf("resolved Case did not survive restart: %+v err=%v", storedCase, err)
	}
	storedVerdict, err := restarted.InspectVerdictExactV1(ctx, verdict.TenantID, reviewport.ExactV1(verdict.ID, verdict.Revision, verdict.Digest))
	if err != nil || storedVerdict.Digest != verdict.Digest {
		t.Fatalf("Verdict did not survive restart: %+v err=%v", storedVerdict, err)
	}
}

type urlForTest struct{ path string }

func (u *urlForTest) dsn() string { return "file:" + filepath.ToSlash(u.path) }

func conformanceFixture(t *testing.T, now time.Time) (contract.TargetSnapshotV1, contract.ReviewCaseV1, contract.ReviewCaseV1, contract.TraceFactV1) {
	t.Helper()
	target := testkit.Target(now)
	base, err := contract.SealReviewCaseV1(contract.ReviewCaseV1{FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: target.TenantID, ID: "case-sqlite", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest, State: contract.CaseRequestedV1, ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	next := nextCase(t, base, contract.CaseAdmittedV1, now.Add(time.Second), "")
	trace := testkit.TraceForTarget(now, base.ID, target, contract.TraceRequestedV1, 1)
	return target, base, next, trace
}

func nextCase(t *testing.T, base contract.ReviewCaseV1, state contract.CaseStateV1, at time.Time, reason core.ReasonCode) contract.ReviewCaseV1 {
	t.Helper()
	next := base
	next.Revision++
	next.State = state
	next.UpdatedUnixNano = at.UnixNano()
	next.InvalidationReason = reason
	next.Digest = ""
	next, err := contract.SealReviewCaseV1(next)
	if err != nil {
		t.Fatal(err)
	}
	return next
}

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
	"github.com/Proview-China/rax/ExecutionRuntime/review/verdictowner"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type atomicTargetCaseCreatorV1 interface {
	CreateTargetCaseV1(context.Context, reviewport.CreateTargetCaseMutationV1) (contract.ReviewCaseV1, error)
}

var _ atomicTargetCaseCreatorV1 = (reviewport.StoreV1)(nil)

func TestConformanceStoreShapeHasNoCaseCreationBypass(t *testing.T) {
	storePort := reflect.TypeOf((*reviewport.StoreV1)(nil)).Elem()
	if _, ok := storePort.MethodByName("CreateTargetCaseV1"); !ok {
		t.Fatal("StoreV1 lost its atomic Target+Case creation entrypoint")
	}
	if _, ok := storePort.MethodByName("CreateCaseV1"); ok {
		t.Fatal("StoreV1 exposes a Case-only creation bypass")
	}
	memoryStore := reflect.TypeOf(memory.NewStore())
	if _, ok := memoryStore.MethodByName("CreateCaseV1"); ok {
		t.Fatal("memory Store exposes a Case-only creation bypass")
	}
}

func TestP0AutoAttestationRejectsNonAppliedAndCrossSubjectResult(t *testing.T) {
	tests := []struct {
		name              string
		disposition       runtimeports.OperationSettlementDispositionV3
		mutateResult      func(*contract.ReviewerInvocationResultFactV1)
		mutateAttestation func(*contract.AttestationV1)
	}{
		{name: "not_applied", disposition: runtimeports.OperationSettlementNotAppliedV3},
		{name: "failed", disposition: runtimeports.OperationSettlementFailedV3},
		{name: "cross_tenant", mutateResult: func(v *contract.ReviewerInvocationResultFactV1) { v.TenantID = "tenant-other" }},
		{name: "cross_case", mutateResult: func(v *contract.ReviewerInvocationResultFactV1) { v.CaseID = "case-other" }},
		{name: "cross_round", mutateResult: func(v *contract.ReviewerInvocationResultFactV1) { v.RoundID = "round-other" }},
		{name: "cross_assignment", mutateResult: func(v *contract.ReviewerInvocationResultFactV1) { v.AssignmentID = "assignment-other" }},
		{name: "cross_target_id", mutateResult: func(v *contract.ReviewerInvocationResultFactV1) { v.TargetID = "target-other" }},
		{name: "cross_target_revision", mutateResult: func(v *contract.ReviewerInvocationResultFactV1) { v.TargetRevision++ }},
		{name: "cross_target_digest", mutateResult: func(v *contract.ReviewerInvocationResultFactV1) { v.TargetDigest = testkit.Digest("target-other") }},
		{name: "attempt_drift", mutateAttestation: func(v *contract.AttestationV1) { v.ReviewerAttemptID = "attempt-other" }},
		{name: "result_digest_drift", mutateAttestation: func(v *contract.AttestationV1) { v.ReviewerResultDigest = testkit.Digest("result-other") }},
		{name: "runtime_inspection_drift", mutateAttestation: func(v *contract.AttestationV1) {
			v.DomainApplySettlement.RuntimeContractVersion = runtimeports.OperationSettlementContractVersionV4
			v.DomainApplySettlement.RuntimeInspectionDigest = testkit.Digest("another-runtime-inspection")
			v.AutoProvenance = &contract.AutoReviewerAttestationProvenanceV1{
				Attempt:     contract.ExactResourceRefV1{ID: "auto-attempt-drift", Revision: 2, Digest: testkit.Digest("auto-attempt-drift")},
				Observation: contract.AutoReviewerInvocationObservationRefV1{ID: "auto-observation-drift", Revision: 1, Digest: testkit.Digest("auto-observation-drift")},
				Rubric:      contract.ExactResourceRefV1{ID: "rubric-drift", Revision: 1, Digest: testkit.Digest("rubric-drift")},
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newReviewingFlow(t, contract.RouteAutoV1)
			f.clock.Advance(time.Second)
			result := testkit.DomainResult(f.clock.Now(), f.caseValue, f.round, f.assignment)
			if test.mutateResult != nil {
				test.mutateResult(&result)
				result.Digest = ""
				var err error
				result, err = contract.SealReviewerInvocationResultFactV1(result)
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := f.store.CreateDomainResultV1(f.ctx, result); err != nil {
				t.Fatal(err)
			}
			settlement := testkit.RuntimeSettlement(result)
			if test.disposition != "" {
				settlement.Disposition = test.disposition
			}
			apply, err := contract.ApplyRuntimeSettlementV1("apply-a", result, settlement, f.clock.Now().Add(time.Nanosecond).UnixNano())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.store.CreateApplySettlementV1(f.ctx, apply); err != nil {
				t.Fatal(err)
			}
			attestationApply := apply
			if attestationApply.State != contract.DomainApplyAppliedV1 {
				attestationApply.State = contract.DomainApplyAppliedV1
			}
			attestation := testkit.AutoAttestation(f.clock.Now(), f.caseValue, f.round, f.assignment, result, attestationApply)
			if test.mutateAttestation != nil {
				test.mutateAttestation(&attestation)
			}
			attestation.Digest = ""
			attestation, err = contract.SealAttestationV1(attestation)
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = f.engine.RecordAttestationV1(f.ctx, reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), attestation, testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceAttestedV1, 3, attestation.ID))
			if err == nil {
				t.Fatal("invalid auto settlement/result chain reached attested state")
			}
			assertCurrentCaseExact(t, f.store, f.caseValue)
			if _, inspectErr := f.store.InspectAttestationExactV1(f.ctx, attestation.TenantID, reviewport.ExactV1(attestation.ID, attestation.Revision, attestation.Digest)); !core.HasCategory(inspectErr, core.ErrorNotFound) {
				t.Fatalf("invalid auto chain leaked exact Attestation: %v", inspectErr)
			}
			verdictRef := reviewport.ExactV1("verdict-invalid-"+test.name, 1, testkit.Digest("verdict-invalid-"+test.name))
			if _, inspectErr := f.store.InspectVerdictExactV1(f.ctx, f.caseValue.TenantID, verdictRef); !core.HasCategory(inspectErr, core.ErrorNotFound) {
				t.Fatalf("invalid auto chain leaked exact Verdict: %v", inspectErr)
			}
		})
	}
}

type mutatingCurrentReader struct {
	base   reviewport.DecisionCurrentReaderV1
	mutate func(*contract.DecisionCurrentSnapshotV1)
}

func (r mutatingCurrentReader) InspectDecisionCurrentV1(ctx context.Context, request reviewport.DecisionCurrentRequestV1) (contract.DecisionCurrentSnapshotV1, error) {
	value, err := r.base.InspectDecisionCurrentV1(ctx, request)
	if err != nil {
		return contract.DecisionCurrentSnapshotV1{}, err
	}
	if value.DomainResult != nil {
		copyValue := *value.DomainResult
		value.DomainResult = &copyValue
	}
	r.mutate(&value)
	value.Digest = ""
	return contract.SealDecisionCurrentSnapshotV1(value)
}

func TestP0AutoVerdictRechecksStoredSettlementResultChain(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*contract.DecisionCurrentSnapshotV1)
	}{
		{name: "case", mutate: func(v *contract.DecisionCurrentSnapshotV1) {
			v.DomainResult.CaseID = "case-other"
			v.DomainResult.Digest = ""
			*v.DomainResult, _ = contract.SealReviewerInvocationResultFactV1(*v.DomainResult)
		}},
		{name: "round", mutate: func(v *contract.DecisionCurrentSnapshotV1) {
			v.DomainResult.RoundID = "round-other"
			v.DomainResult.Digest = ""
			*v.DomainResult, _ = contract.SealReviewerInvocationResultFactV1(*v.DomainResult)
		}},
		{name: "assignment", mutate: func(v *contract.DecisionCurrentSnapshotV1) {
			v.DomainResult.AssignmentID = "assignment-other"
			v.DomainResult.Digest = ""
			*v.DomainResult, _ = contract.SealReviewerInvocationResultFactV1(*v.DomainResult)
		}},
		{name: "target", mutate: func(v *contract.DecisionCurrentSnapshotV1) {
			v.DomainResult.TargetDigest = testkit.Digest("target-other")
			v.DomainResult.Digest = ""
			*v.DomainResult, _ = contract.SealReviewerInvocationResultFactV1(*v.DomainResult)
		}},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			baseTime := time.Unix(1_902_100_000, 0)
			storeClock := testkit.NewClock(baseTime.Add(6 * time.Second))
			store := storetestkit.NewMemoryStoreV1(storeClock.Now)
			fixture := newAutoReviewerFixtureV1(t, store, baseTime, "verdict-chain-"+test.name)
			prepared, err := store.BeginAutoReviewerAttemptV1(fixture.ctx, reviewport.BeginAutoReviewerAttemptMutationV1{Attempt: fixture.attempt})
			if err != nil {
				t.Fatal(err)
			}
			waiting := applyAutoReviewerStartClaimV1(t, store, fixture, prepared)
			observationMutation := autoReviewerObservedMutationV1(t, fixture, waiting, "verdict-chain-"+test.name)
			observed, result, err := store.RecordAutoReviewerObservationV1(fixture.ctx, observationMutation)
			if err != nil {
				t.Fatal(err)
			}
			checked := fixture.now.Add(6 * time.Second)
			settlement := testkit.RuntimeSettlement(result)
			settlement.Attempt.AttemptID = result.AttemptID
			apply, err := contract.ApplyRuntimeSettlementV1("apply-a-"+test.name, result, settlement, checked.Add(-time.Nanosecond).UnixNano())
			if err != nil {
				t.Fatal(err)
			}
			attestation := testkit.AutoAttestation(checked, fixture.caseValue, fixture.round, fixture.assignment, result, apply)
			apply.RuntimeContractVersion = runtimeports.OperationSettlementContractVersionV4
			apply.RuntimeInspectionDigest = testkit.Digest("runtime-v4-inspection-" + test.name)
			apply.Digest = ""
			apply, err = contract.SealDomainApplySettlementFactV1(apply)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = store.CreateApplySettlementV1(fixture.ctx, apply); err != nil {
				t.Fatal(err)
			}
			applyRef := apply.Ref()
			attestation.DomainApplySettlement = &applyRef
			attestation.AutoProvenance = &contract.AutoReviewerAttestationProvenanceV1{Attempt: observed.ExactRef(), Observation: observationMutation.Observation.Ref(), Rubric: fixture.rubric.ExactRef()}
			attestation.Resolution = observationMutation.Observation.Output.Resolution
			attestation.ReasonCodes = append([]string(nil), observationMutation.Observation.Output.ReasonCodes...)
			attestation.Evidence = append([]runtimeports.ReviewEvidenceRefV2(nil), observationMutation.Observation.Output.Evidence...)
			attestation.EvidenceDigest, err = contract.ComputeReviewEvidenceDigestV1(attestation.Evidence)
			if err != nil {
				t.Fatal(err)
			}
			attestation.Conditions = append([]runtimeports.ReviewConditionV2(nil), observationMutation.Observation.Output.Conditions...)
			attestation.ConditionsDigest = observationMutation.Observation.Output.ConditionsDigest
			attestation.ExpiresUnixNano = checked.Add(time.Minute).UnixNano()
			attestation.Digest = ""
			attestation, err = contract.SealAttestationV1(attestation)
			if err != nil {
				t.Fatal(err)
			}
			termination, err := store.InspectAutoReviewTerminationCurrentV1(fixture.ctx, reviewport.AutoReviewTerminationCurrentRequestV1{TenantID: observed.TenantID, Target: observed.Target, Case: observed.Case, Rubric: observed.Rubric, ExpectedRound: observed.Round, CheckedUnixNano: checked.Add(-time.Nanosecond).UnixNano()})
			if err != nil {
				t.Fatal(err)
			}
			storeClock.Set(checked.Add(time.Nanosecond))
			trace := testkit.AttestedTrace(checked, fixture.caseValue, 3, attestation.ID, attestation.ID, observed.ID, observationMutation.Observation.ID, result.ID, apply.ID, fixture.rubric.ID)
			caseFact, _, err := store.RecordAttestationV1(fixture.ctx, reviewport.RecordAttestationMutationV1{Expected: reviewport.ExpectedV1(fixture.caseValue.Revision, fixture.caseValue.Digest), Attestation: attestation, NextState: contract.CaseAttestedV1, Trace: trace, AutoTerminationCurrent: &termination, AutoCheckedUnixNano: checked.UnixNano()})
			if err != nil {
				t.Fatal(err)
			}
			base, _ := memory.NewDecisionCurrentSourceV1(store, &testkit.ExternalCurrentReader{}, storeClock.Now)
			owner, _ := verdictowner.New(store, mutatingCurrentReader{base: base, mutate: test.mutate}, storeClock.Now)
			_, _, err = owner.DecideV1(fixture.ctx, verdictowner.DecideCommandV1{TenantID: caseFact.TenantID, CaseID: caseFact.ID, Expected: reviewport.ExpectedV1(caseFact.Revision, caseFact.Digest), AttestationID: attestation.ID, VerdictID: "verdict-auto-" + test.name, Trace: testkit.Trace(storeClock.Now(), caseFact, contract.TraceVerdictV1, 4, "verdict-auto-"+test.name)})
			if err == nil {
				t.Fatal("drifted auto result reached Verdict CAS")
			}
		})
	}
}

type rollbackDuringDecisionCurrentReader struct {
	base       reviewport.DecisionCurrentReaderV1
	clock      *testkit.ManualClock
	rollbackTo time.Time
}

func (r rollbackDuringDecisionCurrentReader) InspectDecisionCurrentV1(ctx context.Context, request reviewport.DecisionCurrentRequestV1) (contract.DecisionCurrentSnapshotV1, error) {
	value, err := r.base.InspectDecisionCurrentV1(ctx, request)
	r.clock.Set(r.rollbackTo)
	return value, err
}

type decideCallCountingStore struct {
	reviewport.StoreV1
	decideCalls atomic.Int32
}

func (s *decideCallCountingStore) DecideV1(ctx context.Context, mutation reviewport.DecideMutationV1) (contract.ReviewCaseV1, contract.VerdictV1, error) {
	s.decideCalls.Add(1)
	return s.StoreV1.DecideV1(ctx, mutation)
}

func TestP1VerdictDecideClockRollbackDuringCurrentInspectHasZeroCAS(t *testing.T) {
	f := newReviewingFlow(t, contract.RouteHumanV1)
	f.clock.Advance(time.Second)
	attestation := testkit.HumanAttestation(f.clock.Now(), f.caseValue, f.round, f.assignment, contract.ResolutionAcceptV1, "idem-clock-rollback")
	attested, _, err := f.engine.RecordAttestationV1(f.ctx, reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), attestation, testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceAttestedV1, 3, attestation.ID))
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(3 * time.Second)
	baseline := f.clock.Now()
	rollbackTo := baseline.Add(-time.Second)
	if !rollbackTo.After(time.Unix(0, attested.UpdatedUnixNano)) {
		t.Fatal("test rollback clock must remain later than all inspected Review facts")
	}
	base, err := memory.NewDecisionCurrentSourceV1(f.store, &testkit.ExternalCurrentReader{}, f.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	countingStore := &decideCallCountingStore{StoreV1: f.store}
	owner, err := verdictowner.New(countingStore, rollbackDuringDecisionCurrentReader{base: base, clock: f.clock, rollbackTo: rollbackTo}, f.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = owner.DecideV1(f.ctx, verdictowner.DecideCommandV1{TenantID: attested.TenantID, CaseID: attested.ID, Expected: reviewport.ExpectedV1(attested.Revision, attested.Digest), AttestationID: attestation.ID, VerdictID: "verdict-clock-rollback", Trace: testkit.Trace(baseline, attested, contract.TraceVerdictV1, 4, "verdict-clock-rollback")})
	if !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("Decide accepted a clock rollback across current Inspect: %v", err)
	}
	if countingStore.decideCalls.Load() != 0 {
		t.Fatalf("clock rollback reached Verdict CAS: calls=%d", countingStore.decideCalls.Load())
	}
	assertCurrentCaseExact(t, f.store, attested)
	if _, err := f.store.InspectVerdictV1(f.ctx, attested.TenantID, "verdict-clock-rollback"); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("clock rollback leaked a Verdict: %v", err)
	}
}

func TestP2ReviewerIdentityAuthorityAndBindingDriftHaveZeroVerdictCAS(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*contract.AttestationV1)
	}{
		{name: "reviewer_id", mutate: func(value *contract.AttestationV1) { value.ReviewerID = "reviewer-other" }},
		{name: "reviewer_authority", mutate: func(value *contract.AttestationV1) {
			value.ReviewerAuthority = testkit.Authority("reviewer-authority-other")
		}},
		{name: "reviewer_binding", mutate: func(value *contract.AttestationV1) {
			value.ReviewerBinding = testkit.ReviewerBinding()
			value.ReviewerBinding.ComponentID = "review.test/reviewer-other"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newReviewingFlow(t, contract.RouteHumanV1)
			f.clock.Advance(time.Second)
			attestation := testkit.HumanAttestation(f.clock.Now(), f.caseValue, f.round, f.assignment, contract.ResolutionAcceptV1, "idem-reviewer-drift-"+test.name)
			attested, _, err := f.engine.RecordAttestationV1(f.ctx, reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), attestation, testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceAttestedV1, 3, attestation.ID))
			if err != nil {
				t.Fatal(err)
			}
			base, err := memory.NewDecisionCurrentSourceV1(f.store, &testkit.ExternalCurrentReader{}, f.clock.Now)
			if err != nil {
				t.Fatal(err)
			}
			reader := mutatingCurrentReader{base: base, mutate: func(snapshot *contract.DecisionCurrentSnapshotV1) {
				value := snapshot.Attestation
				test.mutate(&value)
				value.Digest = ""
				value, _ = contract.SealAttestationV1(value)
				snapshot.Attestation = value
			}}
			countingStore := &decideCallCountingStore{StoreV1: f.store}
			owner, err := verdictowner.New(countingStore, reader, f.clock.Now)
			if err != nil {
				t.Fatal(err)
			}
			verdictID := "verdict-reviewer-drift-" + test.name
			_, _, err = owner.DecideV1(f.ctx, verdictowner.DecideCommandV1{TenantID: attested.TenantID, CaseID: attested.ID, Expected: reviewport.ExpectedV1(attested.Revision, attested.Digest), AttestationID: attestation.ID, VerdictID: verdictID, Trace: testkit.Trace(f.clock.Now(), attested, contract.TraceVerdictV1, 4, verdictID)})
			if err == nil {
				t.Fatal("reviewer drift reached Verdict")
			}
			if countingStore.decideCalls.Load() != 0 {
				t.Fatalf("reviewer drift reached Verdict CAS: calls=%d", countingStore.decideCalls.Load())
			}
			if _, inspectErr := f.store.InspectVerdictV1(f.ctx, attested.TenantID, verdictID); !core.HasCategory(inspectErr, core.ErrorNotFound) {
				t.Fatalf("reviewer drift leaked Verdict: %v", inspectErr)
			}
			assertCurrentCaseExact(t, f.store, attested)
		})
	}
}

func TestWhiteboxTargetCaseUniquenessSupersedeAndHistory(t *testing.T) {
	ctx := context.Background()
	clock := testkit.NewClock(time.Unix(1_750_100_000, 0))
	store := memory.NewStore()
	engine, _ := caseengine.New(store, clock.Now)
	target := testkit.Target(clock.Now())
	clock.Advance(time.Second)
	requested := testkit.TraceForTarget(clock.Now(), "case-original", target, contract.TraceRequestedV1, 1)
	created, err := engine.CreateCaseV1(ctx, caseengine.CreateCaseCommandV1{CaseID: "case-original", Target: target, ExpiresUnixNano: clock.Now().Add(20 * time.Minute).UnixNano(), Trace: requested})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := engine.CreateCaseV1(ctx, caseengine.CreateCaseCommandV1{CaseID: "case-original", Target: target, ExpiresUnixNano: created.ExpiresUnixNano, Trace: requested})
	if err != nil || replayed.ID != created.ID {
		t.Fatalf("same target did not return same Case: %v", err)
	}
	if _, err := engine.CreateCaseV1(ctx, caseengine.CreateCaseCommandV1{CaseID: "case-other", Target: target, ExpiresUnixNano: created.ExpiresUnixNano, Trace: testkit.TraceForTarget(clock.Now(), "case-other", target, contract.TraceRequestedV1, 11)}); err == nil {
		t.Fatal("same target accepted another Case ID")
	}
	// The exact Target identity is a create-once key. Reusing both that key and
	// the Case ID with different Case payload must conflict instead of returning
	// whichever current revision happens to be indexed for the Target.
	drift := created
	drift.ExpiresUnixNano--
	drift.Digest = ""
	drift, _ = contract.SealReviewCaseV1(drift)
	if _, err := store.CreateTargetCaseV1(ctx, reviewport.CreateTargetCaseMutationV1{Target: target, Case: drift}); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("same Case ID drift was not Conflict: %v", err)
	}

	source, _ := memory.NewDecisionCurrentSourceV1(store, &testkit.ExternalCurrentReader{}, clock.Now)
	owner, _ := verdictowner.New(store, source, clock.Now)
	clock.Advance(time.Second)
	superseded, _, err := owner.SupersedeV1(ctx, created.TenantID, created.ID, reviewport.ExpectedV1(created.Revision, created.Digest), core.ReasonReviewCandidateConflict, testkit.Trace(clock.Now(), created, contract.TraceSupersededV1, 2, created.ID))
	if err != nil {
		t.Fatal(err)
	}
	sameRevision := target
	sameRevision.PayloadDigest = testkit.Digest("same-revision-new-payload")
	sameRevision.UpdatedUnixNano = clock.Now().UnixNano()
	sameRevision.Digest = ""
	sameRevision, err = contract.SealTargetSnapshotV1(sameRevision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.CreateCaseV1(ctx, caseengine.CreateCaseCommandV1{CaseID: "case-same-revision-drift", Target: sameRevision, ExpiresUnixNano: clock.Now().Add(15 * time.Minute).UnixNano(), Trace: testkit.TraceForTarget(clock.Now(), "case-same-revision-drift", sameRevision, contract.TraceRequestedV1, 12)}); !core.HasReason(err, core.ReasonRevisionConflict) {
		t.Fatalf("same Target revision with a new digest was not rejected: %v", err)
	}
	if _, err := store.InspectTargetExactV1(ctx, target.TenantID, reviewport.ExactV1(target.ID, target.Revision, target.Digest)); err != nil {
		t.Fatalf("same-revision conflict overwrote old exact Target: %v", err)
	}
	if _, err := store.InspectCaseV1(ctx, target.TenantID, "case-same-revision-drift"); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("same-revision conflict leaked a Case: %v", err)
	}
	nextTarget := target
	nextTarget.Revision++
	nextTarget.PayloadRevision++
	nextTarget.PayloadDigest = testkit.Digest("target-payload-v2")
	nextTarget.UpdatedUnixNano = clock.Now().UnixNano()
	nextTarget.Digest = ""
	nextTarget, err = contract.SealTargetSnapshotV1(nextTarget)
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	nextCase, err := engine.CreateCaseV1(ctx, caseengine.CreateCaseCommandV1{CaseID: "case-next", Target: nextTarget, ExpiresUnixNano: clock.Now().Add(15 * time.Minute).UnixNano(), Trace: testkit.TraceForTarget(clock.Now(), "case-next", nextTarget, contract.TraceRequestedV1, 3)})
	if err != nil {
		t.Fatal(err)
	}
	if nextCase.ID == superseded.ID || nextCase.TargetRevision == created.TargetRevision {
		t.Fatal("new Target revision did not create a distinct current Case")
	}
	clock.Advance(time.Second)
	if _, _, err := owner.SupersedeV1(ctx, nextCase.TenantID, nextCase.ID, reviewport.ExpectedV1(nextCase.Revision, nextCase.Digest), core.ReasonReviewCandidateConflict, testkit.Trace(clock.Now(), nextCase, contract.TraceSupersededV1, 4, nextCase.ID)); err != nil {
		t.Fatal(err)
	}
	rollbackTarget := target
	rollbackTarget.Revision--
	rollbackTarget.PayloadDigest = testkit.Digest("target-revision-rollback")
	rollbackTarget.UpdatedUnixNano = clock.Now().UnixNano()
	rollbackTarget.Digest = ""
	rollbackTarget, err = contract.SealTargetSnapshotV1(rollbackTarget)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.CreateCaseV1(ctx, caseengine.CreateCaseCommandV1{CaseID: "case-revision-rollback", Target: rollbackTarget, ExpiresUnixNano: clock.Now().Add(10 * time.Minute).UnixNano(), Trace: testkit.TraceForTarget(clock.Now(), "case-revision-rollback", rollbackTarget, contract.TraceRequestedV1, 13)}); !core.HasReason(err, core.ReasonRevisionConflict) {
		t.Fatalf("Target revision rollback was not rejected: %v", err)
	}
	for _, historical := range []contract.TargetSnapshotV1{target, nextTarget} {
		if _, err := store.InspectTargetExactV1(ctx, historical.TenantID, reviewport.ExactV1(historical.ID, historical.Revision, historical.Digest)); err != nil {
			t.Fatalf("revision rollback damaged Target history revision %d: %v", historical.Revision, err)
		}
	}
	if _, err := store.InspectCaseV1(ctx, target.TenantID, "case-revision-rollback"); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("revision rollback leaked a Case: %v", err)
	}
	if _, err := store.InspectCaseExactV1(ctx, created.TenantID, reviewport.ExactV1(created.ID, created.Revision, created.Digest)); err != nil {
		t.Fatalf("initial Case history lost: %v", err)
	}
	if _, err := store.InspectCaseExactV1(ctx, superseded.TenantID, reviewport.ExactV1(superseded.ID, superseded.Revision, superseded.Digest)); err != nil {
		t.Fatalf("superseded Case history lost: %v", err)
	}
}

type createReplyLostStore struct {
	reviewport.StoreV1
	createCalls atomic.Int32
}

func (s *createReplyLostStore) CreateTargetCaseV1(ctx context.Context, mutation reviewport.CreateTargetCaseMutationV1) (contract.ReviewCaseV1, error) {
	s.createCalls.Add(1)
	value, err := s.StoreV1.CreateTargetCaseV1(ctx, mutation)
	if err == nil {
		return contract.ReviewCaseV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEffectUnknownOutcome, "lost Target+Case+Trace create reply")
	}
	return value, err
}

func TestFaultCreateTargetCaseTraceAtomicityAndLostReply(t *testing.T) {
	t.Run("trace_conflict_zero_write", func(t *testing.T) {
		ctx := context.Background()
		clock := testkit.NewClock(time.Unix(1_750_200_000, 0))
		store := memory.NewStore()
		engine, _ := caseengine.New(store, clock.Now)
		target := testkit.Target(clock.Now())
		clock.Advance(time.Second)
		conflict := testkit.TraceForTarget(clock.Now(), "case-create-conflict", target, contract.TraceRoutedV1, 1, "conflict")
		conflict.SourceID = "review.test/request"
		conflict.Digest = ""
		conflict, _ = contract.SealTraceFactV1(conflict)
		if _, err := store.InjectTraceForTestV1(ctx, conflict); err != nil {
			t.Fatal(err)
		}
		requested := testkit.TraceForTarget(clock.Now(), "case-create-conflict", target, contract.TraceRequestedV1, 1, target.ID)
		_, err := engine.CreateCaseV1(ctx, caseengine.CreateCaseCommandV1{CaseID: "case-create-conflict", Target: target, ExpiresUnixNano: clock.Now().Add(20 * time.Minute).UnixNano(), Trace: requested})
		if !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("creation Trace conflict did not fail compound mutation: %v", err)
		}
		if _, err := store.InspectTargetExactV1(ctx, target.TenantID, reviewport.ExactV1(target.ID, target.Revision, target.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("Trace conflict leaked Target history: %v", err)
		}
		if _, err := store.InspectCaseV1(ctx, target.TenantID, "case-create-conflict"); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("Trace conflict leaked Case: %v", err)
		}
		if _, err := store.InspectTraceExactV1(ctx, requested.TenantID, reviewport.ExactV1(requested.ID, requested.Revision, requested.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("Trace conflict leaked requested Trace: %v", err)
		}
	})

	t.Run("lost_reply_exact_inspect", func(t *testing.T) {
		ctx := context.Background()
		clock := testkit.NewClock(time.Unix(1_750_300_000, 0))
		store := memory.NewStore()
		lost := &createReplyLostStore{StoreV1: store}
		engine, _ := caseengine.New(lost, clock.Now)
		target := testkit.Target(clock.Now())
		clock.Advance(time.Second)
		trace := testkit.TraceForTarget(clock.Now(), "case-create-lost", target, contract.TraceRequestedV1, 1, target.ID)
		created, err := engine.CreateCaseV1(ctx, caseengine.CreateCaseCommandV1{CaseID: "case-create-lost", Target: target, ExpiresUnixNano: clock.Now().Add(20 * time.Minute).UnixNano(), Trace: trace})
		if err != nil || lost.createCalls.Load() != 1 {
			t.Fatalf("lost create reply was not exact-recovered once: calls=%d err=%v", lost.createCalls.Load(), err)
		}
		if _, err := store.InspectTargetExactV1(ctx, target.TenantID, reviewport.ExactV1(target.ID, target.Revision, target.Digest)); err != nil {
			t.Fatalf("lost reply recovery missed exact Target: %v", err)
		}
		if _, err := store.InspectCaseExactV1(ctx, created.TenantID, reviewport.ExactV1(created.ID, created.Revision, created.Digest)); err != nil {
			t.Fatalf("lost reply recovery missed exact Case: %v", err)
		}
		if _, err := store.InspectTraceExactV1(ctx, trace.TenantID, reviewport.ExactV1(trace.ID, trace.Revision, trace.Digest)); err != nil {
			t.Fatalf("lost reply recovery missed exact Trace: %v", err)
		}
	})
}

func TestFaultStagedMutationFailureLeaksNoCurrentOrHistory(t *testing.T) {
	t.Run("start_round", func(t *testing.T) {
		ctx := context.Background()
		clock := testkit.NewClock(time.Unix(1_750_250_000, 0))
		store, err := memory.NewStoreWithClockV1(clock.Now)
		if err != nil {
			t.Fatal(err)
		}
		engine, err := caseengine.New(store, clock.Now)
		if err != nil {
			t.Fatal(err)
		}
		target := testkit.Target(clock.Now())
		testkit.PublishRubric(ctx, store, clock.Now(), target.TenantID)
		request := testkit.Request(clock.Now(), target, "case-staged-round")
		clock.Advance(time.Second)
		current, err := engine.CreateCaseV1(ctx, caseengine.CreateCaseCommandV1{CaseID: "case-staged-round", Request: &request, Target: target, ExpiresUnixNano: request.ExpiresUnixNano, Trace: testkit.TraceForTarget(clock.Now(), "case-staged-round", target, contract.TraceRequestedV1, 1, request.ID)})
		if err != nil {
			t.Fatal(err)
		}
		for _, state := range []contract.CaseStateV1{contract.CaseAdmittedV1, contract.CaseRoutedV1} {
			clock.Advance(time.Second)
			current, err = engine.TransitionWithTraceV2(ctx, caseengine.TransitionWithTraceCommandV2{TransitionCommandV1: caseengine.TransitionCommandV1{TenantID: current.TenantID, CaseID: current.ID, Expected: reviewport.ExpectedV1(current.Revision, current.Digest), Next: state}, Trace: testkit.TransitionTrace(clock.Now(), current, state)})
			if err != nil {
				t.Fatal(err)
			}
		}
		clock.Advance(time.Second)
		round := testkit.Round(clock.Now(), current, contract.RouteHumanV1)
		assignment := testkit.Assignment(clock.Now(), current, round, contract.RouteHumanV1)
		conflict := testkit.Trace(clock.Now(), current, contract.TraceRoutedV1, 2, "conflict")
		if _, err := store.InjectTraceForTestV1(ctx, conflict); err != nil {
			t.Fatal(err)
		}
		_, _, _, err = engine.StartRoundV1(ctx, reviewport.StartRoundMutationV1{Expected: reviewport.ExpectedV1(current.Revision, current.Digest), Round: round, Assignment: assignment, Trace: testkit.Trace(clock.Now(), current, contract.TraceAssignedV1, 2, round.ID, assignment.ID)})
		if !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("staged StartRound did not fail at trace conflict: %v", err)
		}
		assertCurrentCaseExact(t, store, current)
		assertExactCaseRevisionMissing(t, store, current, current.Revision+1)
		if _, err := store.InspectRoundV1(ctx, current.TenantID, round.ID); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("failed StartRound leaked Round: %v", err)
		}
		if _, err := store.InspectAssignmentV1(ctx, current.TenantID, assignment.ID); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("failed StartRound leaked Assignment/history: %v", err)
		}
	})

	t.Run("record_attestation", func(t *testing.T) {
		f := newReviewingFlow(t, contract.RouteHumanV1)
		f.clock.Advance(time.Second)
		attestation := testkit.HumanAttestation(f.clock.Now(), f.caseValue, f.round, f.assignment, contract.ResolutionAcceptV1, "idem-staged-attestation")
		conflict := testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceRoutedV1, 30, "conflict")
		if _, err := f.store.InjectTraceForTestV1(f.ctx, conflict); err != nil {
			t.Fatal(err)
		}
		_, _, err := f.engine.RecordAttestationV1(f.ctx, reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), attestation, testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceAttestedV1, 30, attestation.ID))
		if !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("staged attestation did not fail at trace conflict: %v", err)
		}
		assertCurrentCaseExact(t, f.store, f.caseValue)
		assertExactCaseRevisionMissing(t, f.store, f.caseValue, f.caseValue.Revision+1)
		if _, err := f.store.InspectAttestationV1(f.ctx, attestation.TenantID, attestation.ID); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("failed attestation leaked current fact: %v", err)
		}
	})

	t.Run("decide", func(t *testing.T) {
		f := newReviewingFlow(t, contract.RouteHumanV1)
		f.clock.Advance(time.Second)
		attestation := testkit.HumanAttestation(f.clock.Now(), f.caseValue, f.round, f.assignment, contract.ResolutionAcceptV1, "idem-staged-decide")
		attested, _, err := f.engine.RecordAttestationV1(f.ctx, reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), attestation, testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceAttestedV1, 3, attestation.ID))
		if err != nil {
			t.Fatal(err)
		}
		f.clock.Advance(time.Second)
		conflict := testkit.Trace(f.clock.Now(), attested, contract.TraceRoutedV1, 4, "conflict")
		if _, err := f.store.InjectTraceForTestV1(f.ctx, conflict); err != nil {
			t.Fatal(err)
		}
		owner := newVerdictOwner(t, f, nil)
		_, _, err = owner.DecideV1(f.ctx, verdictowner.DecideCommandV1{TenantID: attested.TenantID, CaseID: attested.ID, Expected: reviewport.ExpectedV1(attested.Revision, attested.Digest), AttestationID: attestation.ID, VerdictID: "verdict-staged", Trace: testkit.Trace(f.clock.Now(), attested, contract.TraceVerdictV1, 4, "verdict-staged")})
		if !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("staged Decide did not fail at trace conflict: %v", err)
		}
		assertCurrentCaseExact(t, f.store, attested)
		assertExactCaseRevisionMissing(t, f.store, attested, attested.Revision+1)
		if _, err := f.store.InspectVerdictV1(f.ctx, attested.TenantID, "verdict-staged"); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("failed Decide leaked Verdict current/history: %v", err)
		}
	})

	t.Run("invalidate", func(t *testing.T) {
		f := newResolvedFlow(t, 10*time.Minute)
		f.clock.Advance(time.Second)
		expectedCase, expectedVerdict := expectedInvalidationFactsV2(t, f.resolved, &f.verdict, f.clock.Now(), contract.CaseRevokedV1, contract.VerdictRevokedV1, core.ReasonReviewVerdictStale)
		conflict := testkit.Trace(f.clock.Now(), f.resolved, contract.TraceRoutedV1, 5, "conflict")
		if _, err := f.store.InjectTraceForTestV1(f.ctx, conflict); err != nil {
			t.Fatal(err)
		}
		_, _, err := f.owner.RevokeV1(f.ctx, f.resolved.TenantID, f.resolved.ID, reviewport.ExpectedV1(f.resolved.Revision, f.resolved.Digest), core.ReasonReviewVerdictStale, testkit.Trace(f.clock.Now(), f.resolved, contract.TraceRevokedV1, 5, f.verdict.ID))
		if !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("staged Invalidate did not fail at trace conflict: %v", err)
		}
		assertInvalidationFactsAbsentV2(t, f.store, f.resolved, &f.verdict, expectedCase, expectedVerdict)
	})
}

func TestRecordAttestationStoreRejectsCallerSelectedCaseState(t *testing.T) {
	f := newReviewingFlow(t, contract.RouteHumanV1)
	f.clock.Advance(time.Second)
	attestation := testkit.HumanAttestation(f.clock.Now(), f.caseValue, f.round, f.assignment, contract.ResolutionAcceptV1, "idem-state-mismatch")
	trace := testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceAttestedV1, 3, attestation.ID)
	_, _, err := f.store.RecordAttestationV1(f.ctx, reviewport.RecordAttestationMutationV1{
		Expected:    reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest),
		Attestation: attestation,
		NextState:   contract.CaseWaitingHumanV1,
		Trace:       trace,
	})
	if !core.HasReason(err, core.ReasonInvalidTransition) {
		t.Fatalf("Store accepted caller-selected state that disagrees with Resolution: %v", err)
	}
	assertCurrentCaseExact(t, f.store, f.caseValue)
	assertExactCaseRevisionMissing(t, f.store, f.caseValue, f.caseValue.Revision+1)
	if _, err := f.store.InspectAttestationV1(f.ctx, attestation.TenantID, attestation.ID); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("state mismatch leaked Attestation: %v", err)
	}
	if _, err := f.store.InspectTraceExactV1(f.ctx, trace.TenantID, reviewport.ExactV1(trace.ID, trace.Revision, trace.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("state mismatch leaked Trace: %v", err)
	}
}

func assertCurrentCaseExact(t *testing.T, store *memory.Store, expected contract.ReviewCaseV1) {
	t.Helper()
	current, err := store.InspectCaseV1(context.Background(), expected.TenantID, expected.ID)
	if err != nil || current.Revision != expected.Revision || current.Digest != expected.Digest {
		t.Fatalf("failed staged mutation changed current Case: value=%+v err=%v", current, err)
	}
	if _, err := store.InspectCaseExactV1(context.Background(), expected.TenantID, reviewport.ExactV1(expected.ID, expected.Revision, expected.Digest)); err != nil {
		t.Fatalf("pre-mutation Case history was lost: %v", err)
	}
}

func assertExactCaseRevisionMissing(t *testing.T, store *memory.Store, previous contract.ReviewCaseV1, revision core.Revision) {
	t.Helper()
	if _, err := store.InspectCaseExactV1(context.Background(), previous.TenantID, reviewport.ExactV1(previous.ID, revision, previous.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("failed staged mutation leaked Case history revision %d: %v", revision, err)
	}
}

func expectedInvalidationFactsV2(t *testing.T, current contract.ReviewCaseV1, verdict *contract.VerdictV1, at time.Time, caseState contract.CaseStateV1, verdictState contract.VerdictStateV1, reason core.ReasonCode) (contract.ReviewCaseV1, *contract.VerdictV1) {
	t.Helper()
	nextCase := current
	nextCase.Revision++
	nextCase.State = caseState
	nextCase.VerdictID, nextCase.VerdictRevision, nextCase.VerdictDigest = "", 0, ""
	nextCase.UpdatedUnixNano = at.UnixNano()
	nextCase.InvalidationReason = reason
	nextCase.Digest = ""
	nextCase, err := contract.SealReviewCaseV1(nextCase)
	if err != nil {
		t.Fatal(err)
	}
	if verdict == nil {
		return nextCase, nil
	}
	nextVerdict := *verdict
	nextVerdict.Revision++
	nextVerdict.State = verdictState
	nextVerdict.UpdatedUnixNano = at.UnixNano()
	nextVerdict.InvalidationReason = reason
	nextVerdict.Digest = ""
	nextVerdict, err = contract.SealVerdictV1(nextVerdict)
	if err != nil {
		t.Fatal(err)
	}
	return nextCase, &nextVerdict
}

func assertInvalidationFactsAbsentV2(t *testing.T, store reviewport.StoreV1, current contract.ReviewCaseV1, verdict *contract.VerdictV1, expectedCase contract.ReviewCaseV1, expectedVerdict *contract.VerdictV1) {
	t.Helper()
	got, err := store.InspectCaseV1(context.Background(), current.TenantID, current.ID)
	if err != nil || got.Digest != current.Digest || got.Revision != current.Revision {
		t.Fatalf("failed Invalidate changed current Case: %+v err=%v", got, err)
	}
	if _, err = store.InspectCaseExactV1(context.Background(), expectedCase.TenantID, reviewport.ExactV1(expectedCase.ID, expectedCase.Revision, expectedCase.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("failed Invalidate leaked successor Case history: %v", err)
	}
	if verdict == nil {
		return
	}
	gotVerdict, err := store.InspectVerdictV1(context.Background(), verdict.TenantID, verdict.ID)
	if err != nil || gotVerdict.Digest != verdict.Digest || gotVerdict.Revision != verdict.Revision {
		t.Fatalf("failed Invalidate changed current Verdict: %+v err=%v", gotVerdict, err)
	}
	if expectedVerdict == nil {
		t.Fatal("test expected a successor Verdict")
	}
	if _, err = store.InspectVerdictExactV1(context.Background(), expectedVerdict.TenantID, reviewport.ExactV1(expectedVerdict.ID, expectedVerdict.Revision, expectedVerdict.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("failed Invalidate leaked successor Verdict history: %v", err)
	}
}

func TestInvalidateMalformedTraceStagesNoCaseVerdictOrHistoryV2(t *testing.T) {
	f := newResolvedFlow(t, 10*time.Minute)
	f.clock.Advance(time.Second)
	reason := core.ReasonReviewVerdictStale
	expectedCase, expectedVerdict := expectedInvalidationFactsV2(t, f.resolved, &f.verdict, f.clock.Now(), contract.CaseRevokedV1, contract.VerdictRevokedV1, reason)
	trace := testkit.Trace(f.clock.Now(), f.resolved, contract.TraceRevokedV1, 85, f.verdict.ID)
	trace.TargetDigest = testkit.Digest("different-target")
	trace.Digest = ""
	trace, err := contract.SealTraceFactV1(trace)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = f.store.InvalidateV1(f.ctx, reviewport.InvalidateMutationV1{TenantID: f.resolved.TenantID, Expected: reviewport.ExpectedV1(f.resolved.Revision, f.resolved.Digest), CaseID: f.resolved.ID, CaseState: contract.CaseRevokedV1, VerdictState: contract.VerdictRevokedV1, Reason: reason, UpdatedUnixNano: f.clock.Now().UnixNano(), Trace: trace})
	if !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("Invalidate accepted malformed exact Trace binding: %v", err)
	}
	assertInvalidationFactsAbsentV2(t, f.store, f.resolved, &f.verdict, expectedCase, expectedVerdict)
	if _, inspectErr := f.store.InspectTraceExactV1(f.ctx, trace.TenantID, reviewport.ExactV1(trace.ID, trace.Revision, trace.Digest)); !core.HasCategory(inspectErr, core.ErrorNotFound) {
		t.Fatalf("failed Invalidate leaked malformed Trace: %v", inspectErr)
	}
}

type mutationReplyLostStore struct {
	reviewport.StoreV1
	decideCalls     atomic.Int32
	invalidateCalls atomic.Int32
}

type lostInvalidateTraceInspectStoreV2 struct {
	*mutationReplyLostStore
	traceID string
}

func (s *lostInvalidateTraceInspectStoreV2) InspectTraceExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (contract.TraceFactV1, error) {
	if ref.ID == s.traceID {
		return contract.TraceFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "injected lost Trace visibility")
	}
	return s.mutationReplyLostStore.InspectTraceExactV1(ctx, tenant, ref)
}

func (s *mutationReplyLostStore) DecideV1(ctx context.Context, mutation reviewport.DecideMutationV1) (contract.ReviewCaseV1, contract.VerdictV1, error) {
	s.decideCalls.Add(1)
	c, v, err := s.StoreV1.DecideV1(ctx, mutation)
	if err == nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEffectUnknownOutcome, "lost Decide reply")
	}
	return c, v, err
}

func (s *mutationReplyLostStore) InvalidateV1(ctx context.Context, mutation reviewport.InvalidateMutationV1) (contract.ReviewCaseV1, *contract.VerdictV1, error) {
	s.invalidateCalls.Add(1)
	c, v, err := s.StoreV1.InvalidateV1(ctx, mutation)
	if err == nil {
		return contract.ReviewCaseV1{}, nil, core.NewError(core.ErrorUnavailable, core.ReasonEffectUnknownOutcome, "lost Invalidate reply")
	}
	return c, v, err
}

func TestFaultOwnerLostRepliesInspectExactWithoutMutationReplay(t *testing.T) {
	f := newReviewingFlow(t, contract.RouteHumanV1)
	f.clock.Advance(time.Second)
	attestation := testkit.HumanAttestation(f.clock.Now(), f.caseValue, f.round, f.assignment, contract.ResolutionAcceptV1, "idem-owner-loss")
	caseFact, _, err := f.engine.RecordAttestationV1(f.ctx, reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), attestation, testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceAttestedV1, 3, attestation.ID))
	if err != nil {
		t.Fatal(err)
	}
	baseSource, _ := memory.NewDecisionCurrentSourceV1(f.store, &testkit.ExternalCurrentReader{}, f.clock.Now)
	lost := &mutationReplyLostStore{StoreV1: f.store}
	owner, _ := verdictowner.New(lost, baseSource, f.clock.Now)
	f.clock.Advance(time.Second)
	resolved, verdict, err := owner.DecideV1(f.ctx, verdictowner.DecideCommandV1{TenantID: caseFact.TenantID, CaseID: caseFact.ID, Expected: reviewport.ExpectedV1(caseFact.Revision, caseFact.Digest), AttestationID: attestation.ID, VerdictID: "verdict-owner-loss", Trace: testkit.Trace(f.clock.Now(), caseFact, contract.TraceVerdictV1, 4, "verdict-owner-loss")})
	if err != nil || lost.decideCalls.Load() != 1 {
		t.Fatalf("Decide lost reply did not exact-recover once: calls=%d err=%v", lost.decideCalls.Load(), err)
	}
	f.clock.Advance(time.Second)
	revoked, historicalVerdict, err := owner.RevokeV1(f.ctx, resolved.TenantID, resolved.ID, reviewport.ExpectedV1(resolved.Revision, resolved.Digest), core.ReasonReviewVerdictStale, testkit.Trace(f.clock.Now(), resolved, contract.TraceRevokedV1, 5, verdict.ID))
	if err != nil || lost.invalidateCalls.Load() != 1 || historicalVerdict == nil {
		t.Fatalf("Invalidate lost reply did not exact-recover once: calls=%d err=%v", lost.invalidateCalls.Load(), err)
	}
	if _, err := f.store.InspectVerdictExactV1(f.ctx, verdict.TenantID, reviewport.ExactV1(verdict.ID, verdict.Revision, verdict.Digest)); err != nil {
		t.Fatalf("original Verdict history lost: %v", err)
	}
	if _, err := f.store.InspectCaseExactV1(f.ctx, resolved.TenantID, reviewport.ExactV1(resolved.ID, resolved.Revision, resolved.Digest)); err != nil {
		t.Fatalf("resolved Case history lost: %v", err)
	}
	if revoked.State != contract.CaseRevokedV1 || historicalVerdict.State != contract.VerdictRevokedV1 {
		t.Fatal("lost-reply recovery returned wrong terminal facts")
	}
}

func TestFaultInvalidateLostReplyRequiresExactTraceRecoveryV2(t *testing.T) {
	f := newResolvedFlow(t, 10*time.Minute)
	baseSource, err := memory.NewDecisionCurrentSourceV1(f.store, &testkit.ExternalCurrentReader{}, f.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	lost := &mutationReplyLostStore{StoreV1: f.store}
	f.clock.Advance(time.Second)
	trace := testkit.Trace(f.clock.Now(), f.resolved, contract.TraceRevokedV1, 86, f.verdict.ID)
	wrapper := &lostInvalidateTraceInspectStoreV2{mutationReplyLostStore: lost, traceID: trace.ID}
	owner, err := verdictowner.New(wrapper, baseSource, f.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = owner.RevokeV1(f.ctx, f.resolved.TenantID, f.resolved.ID, reviewport.ExpectedV1(f.resolved.Revision, f.resolved.Digest), core.ReasonReviewVerdictStale, trace)
	if !core.HasCategory(err, core.ErrorUnavailable) || !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("missing exact Trace did not preserve the original unknown outcome: %v", err)
	}
	if lost.invalidateCalls.Load() != 1 {
		t.Fatalf("lost Invalidate reply retried mutation: calls=%d", lost.invalidateCalls.Load())
	}
	if _, inspectErr := f.store.InspectTraceExactV1(context.Background(), trace.TenantID, reviewport.ExactV1(trace.ID, trace.Revision, trace.Digest)); inspectErr != nil {
		t.Fatalf("underlying atomic mutation did not commit Trace: %v", inspectErr)
	}
}

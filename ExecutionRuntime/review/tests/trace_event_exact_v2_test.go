package review_test

import (
	"sort"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/verdictowner"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type traceDriftV2 struct {
	name   string
	mutate func(*contract.TraceFactV1)
}

func traceDriftsV2() []traceDriftV2 {
	return []traceDriftV2{
		{name: "extra_fact_ref", mutate: func(v *contract.TraceFactV1) {
			v.FactRefs = append(v.FactRefs, "unexpected-fact")
			sort.Strings(v.FactRefs)
		}},
		{name: "missing_fact_ref", mutate: func(v *contract.TraceFactV1) { v.FactRefs = nil }},
		{name: "causation", mutate: func(v *contract.TraceFactV1) { v.CausationID = "wrong-causation" }},
		{name: "correlation", mutate: func(v *contract.TraceFactV1) { v.CorrelationID = "wrong-correlation" }},
		{name: "timestamp", mutate: func(v *contract.TraceFactV1) { v.CreatedUnixNano++; v.UpdatedUnixNano++ }},
	}
}

func driftTraceV2(t *testing.T, value contract.TraceFactV1, drift traceDriftV2) contract.TraceFactV1 {
	t.Helper()
	value.FactRefs = append([]string(nil), value.FactRefs...)
	drift.mutate(&value)
	value.Digest = ""
	sealed, err := contract.SealTraceFactV1(value)
	if err != nil {
		// A structurally invalid negative is still a valid fail-closed oracle.
		return value
	}
	return sealed
}

func assertTraceAbsentV2(t *testing.T, f *flow, trace contract.TraceFactV1) {
	t.Helper()
	if trace.Digest == "" {
		return
	}
	if _, err := f.store.InspectTraceExactV1(f.ctx, trace.TenantID, reviewport.ExactV1(trace.ID, trace.Revision, trace.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("failed mutation leaked Trace: %v", err)
	}
}

func TestTraceEventV2ClaimExactEnvelopeDriftIsZeroWrite(t *testing.T) {
	for _, drift := range traceDriftsV2() {
		t.Run(drift.name, func(t *testing.T) {
			f := newWaitingReviewerFlowV2(t)
			f.clock.Advance(time.Second)
			successor := f.caseValue
			successor.Revision++
			trace := driftTraceV2(t, testkit.Trace(f.clock.Now(), successor, contract.TraceStartedV1, 801, f.assignment.ID), drift)
			_, _, err := f.engine.ClaimAssignmentV1(f.ctx, reviewport.ClaimAssignmentMutationV1{TenantID: f.caseValue.TenantID, CaseID: f.caseValue.ID, AssignmentID: f.assignment.ID, ExpectedCase: reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), ExpectedAssignment: reviewport.ExpectedV1(f.assignment.Revision, f.assignment.Digest), LeaseHolder: "reviewer-a", LeaseExpiresUnixNano: f.clock.Now().Add(10 * time.Minute).UnixNano(), UpdatedUnixNano: f.clock.Now().UnixNano(), Traces: []contract.TraceFactV1{trace}})
			if err == nil {
				t.Fatal("drifted Claim Trace committed")
			}
			gotCase, _ := f.store.InspectCaseV1(f.ctx, f.caseValue.TenantID, f.caseValue.ID)
			gotAssignment, _ := f.store.InspectAssignmentV1(f.ctx, f.assignment.TenantID, f.assignment.ID)
			if gotCase.Digest != f.caseValue.Digest || gotAssignment.Digest != f.assignment.Digest {
				t.Fatal("drifted Claim Trace changed Case or Assignment")
			}
			assertTraceAbsentV2(t, f, trace)
		})
	}
}

func TestTraceEventV2AttestationExactEnvelopeDriftIsZeroWrite(t *testing.T) {
	for _, drift := range traceDriftsV2() {
		t.Run(drift.name, func(t *testing.T) {
			f := newReviewingFlow(t, contract.RouteHumanV1)
			f.clock.Advance(time.Second)
			attestation := testkit.HumanAttestation(f.clock.Now(), f.caseValue, f.round, f.assignment, contract.ResolutionAcceptV1, "idem-trace-attest-"+drift.name)
			trace := driftTraceV2(t, testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceAttestedV1, 802, attestation.ID), drift)
			if _, _, err := f.engine.RecordAttestationV1(f.ctx, reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), attestation, trace); err == nil {
				t.Fatal("drifted Attestation Trace committed")
			}
			gotCase, _ := f.store.InspectCaseV1(f.ctx, f.caseValue.TenantID, f.caseValue.ID)
			if gotCase.Digest != f.caseValue.Digest {
				t.Fatal("drifted Attestation Trace changed Case")
			}
			if _, err := f.store.InspectAttestationExactV1(f.ctx, attestation.TenantID, reviewport.ExactV1(attestation.ID, attestation.Revision, attestation.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
				t.Fatalf("drifted Trace leaked Attestation: %v", err)
			}
			assertTraceAbsentV2(t, f, trace)
		})
	}
}

func TestTraceEventV2DecisionExactEnvelopeDriftIsZeroWrite(t *testing.T) {
	for _, drift := range traceDriftsV2() {
		t.Run(drift.name, func(t *testing.T) {
			f := newReviewingFlow(t, contract.RouteHumanV1)
			f.clock.Advance(time.Second)
			attestation := testkit.HumanAttestation(f.clock.Now(), f.caseValue, f.round, f.assignment, contract.ResolutionAcceptV1, "idem-trace-decide-"+drift.name)
			attested, _, err := f.engine.RecordAttestationV1(f.ctx, reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), attestation, testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceAttestedV1, 803, attestation.ID))
			if err != nil {
				t.Fatal(err)
			}
			f.clock.Advance(time.Second)
			verdictID := "verdict-trace-drift-" + drift.name
			trace := driftTraceV2(t, testkit.Trace(f.clock.Now(), attested, contract.TraceVerdictV1, 804, verdictID), drift)
			owner := newVerdictOwner(t, f, nil)
			if _, _, err = owner.DecideV1(f.ctx, verdictowner.DecideCommandV1{TenantID: attested.TenantID, CaseID: attested.ID, Expected: reviewport.ExpectedV1(attested.Revision, attested.Digest), AttestationID: attestation.ID, VerdictID: verdictID, Trace: trace}); err == nil {
				t.Fatal("drifted Verdict Trace committed")
			}
			gotCase, _ := f.store.InspectCaseV1(f.ctx, attested.TenantID, attested.ID)
			if gotCase.Digest != attested.Digest {
				t.Fatal("drifted Verdict Trace changed Case")
			}
			if _, err := f.store.InspectVerdictV1(f.ctx, attested.TenantID, verdictID); !core.HasCategory(err, core.ErrorNotFound) {
				t.Fatalf("drifted Trace leaked Verdict: %v", err)
			}
			assertTraceAbsentV2(t, f, trace)
		})
	}
}

func TestTraceEventV2InvalidationExactEnvelopeDriftIsZeroWrite(t *testing.T) {
	for _, drift := range traceDriftsV2() {
		t.Run(drift.name, func(t *testing.T) {
			f := newResolvedFlow(t, 10*time.Minute)
			f.clock.Advance(time.Second)
			trace := driftTraceV2(t, testkit.Trace(f.clock.Now(), f.resolved, contract.TraceRevokedV1, 805, f.verdict.ID), drift)
			if _, _, err := f.owner.RevokeV1(f.ctx, f.resolved.TenantID, f.resolved.ID, reviewport.ExpectedV1(f.resolved.Revision, f.resolved.Digest), core.ReasonReviewVerdictStale, trace); err == nil {
				t.Fatal("drifted invalidation Trace committed")
			}
			gotCase, _ := f.store.InspectCaseV1(f.ctx, f.resolved.TenantID, f.resolved.ID)
			gotVerdict, _ := f.store.InspectVerdictExactV1(f.ctx, f.verdict.TenantID, reviewport.ExactV1(f.verdict.ID, f.verdict.Revision, f.verdict.Digest))
			if gotCase.Digest != f.resolved.Digest || gotVerdict.Digest != f.verdict.Digest {
				t.Fatal("drifted invalidation Trace changed Case or Verdict")
			}
			assertTraceAbsentV2(t, f.flow, trace)
		})
	}
}

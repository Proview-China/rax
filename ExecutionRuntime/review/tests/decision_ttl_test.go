package review_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/verdictowner"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type clockAtDecisionCurrentReturn struct {
	base  reviewport.DecisionCurrentReaderV1
	clock *testkit.ManualClock
	at    time.Time
}

func (r clockAtDecisionCurrentReturn) InspectDecisionCurrentV1(ctx context.Context, request reviewport.DecisionCurrentRequestV1) (contract.DecisionCurrentSnapshotV1, error) {
	value, err := r.base.InspectDecisionCurrentV1(ctx, request)
	r.clock.Set(r.at)
	return value, err
}

func TestP2DecisionOwnerEachExternalInputIsIndependentlyShortestAndExpiresAtActualPoint(t *testing.T) {
	tests := []struct {
		name          string
		evidenceIndex int
	}{
		{name: "policy", evidenceIndex: -1},
		{name: "actor_authority", evidenceIndex: -1},
		{name: "reviewer_authority", evidenceIndex: -1},
		{name: "scope", evidenceIndex: -1},
		{name: "binding", evidenceIndex: -1},
		{name: "evidence_attestation", evidenceIndex: 0},
		{name: "evidence_target", evidenceIndex: 1},
	}

	setup := func(t *testing.T, test struct {
		name          string
		evidenceIndex int
	}) (*flow, contract.ReviewCaseV1, contract.AttestationV1, reviewport.DecisionCurrentReaderV1, int64) {
		t.Helper()
		start := time.Unix(1_750_000_000, 0)
		selectedExpiry := start.Add(5 * time.Minute).UnixNano()
		longExternalExpiry := start.Add(9 * time.Minute).UnixNano()
		policyExpiry := start.Add(time.Hour).UnixNano()
		if test.name == "policy" {
			policyExpiry = selectedExpiry
		}
		f := newReviewingFlowWithPolicyExpiry(t, contract.RouteHumanV1, policyExpiry)
		f.clock.Advance(time.Second)
		attestation := testkit.HumanAttestation(f.clock.Now(), f.caseValue, f.round, f.assignment, contract.ResolutionAcceptV1, "idem-ttl-"+test.name)
		attested, _, err := f.engine.RecordAttestationV1(f.ctx, reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), attestation, testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceAttestedV1, 3, attestation.ID))
		if err != nil {
			t.Fatal(err)
		}
		f.clock.Advance(time.Second)
		external := &testkit.ExternalCurrentReader{Mutate: func(value *reviewport.DecisionExternalCurrentProjectionV1) {
			value.Current = true
			value.ExpiresUnixNano = longExternalExpiry
			value.Policy.ExpiresUnixNano = policyExpiry
			value.Policy.Digest = ""
			digest, err := value.Policy.DigestV2()
			if err != nil {
				panic(err)
			}
			value.Policy.Digest = digest
			value.ActorAuthority.ExpiresUnixNano = longExternalExpiry
			value.ReviewerAuthority.ExpiresUnixNano = longExternalExpiry
			value.Scope.ExpiresUnixNano = longExternalExpiry
			value.Binding.ExpiresUnixNano = longExternalExpiry
			for i := range value.Evidence {
				value.Evidence[i].ExpiresUnixNano = longExternalExpiry
			}
			switch test.name {
			case "policy":
			case "actor_authority":
				value.ActorAuthority.ExpiresUnixNano = selectedExpiry
			case "reviewer_authority":
				value.ReviewerAuthority.ExpiresUnixNano = selectedExpiry
			case "scope":
				value.Scope.ExpiresUnixNano = selectedExpiry
			case "binding":
				value.Binding.ExpiresUnixNano = selectedExpiry
			case "evidence_attestation", "evidence_target":
				if len(value.Evidence) != 2 {
					panic("Decision TTL fixture expected exactly two Evidence refs")
				}
				value.Evidence[test.evidenceIndex].ExpiresUnixNano = selectedExpiry
			}
			// The external Owner projection carries its aggregate shortest TTL,
			// while the independently selected field proves which input produced it.
			value.ExpiresUnixNano = selectedExpiry
		}}
		source, err := memory.NewDecisionCurrentSourceV1(f.store, external, f.clock.Now)
		if err != nil {
			t.Fatal(err)
		}
		return f, attested, attestation, source, selectedExpiry
	}

	for _, test := range tests {
		t.Run(test.name+"/minimum", func(t *testing.T) {
			f, attested, attestation, source, selectedExpiry := setup(t, test)
			owner, err := verdictowner.New(f.store, source, f.clock.Now)
			if err != nil {
				t.Fatal(err)
			}
			verdictID := "verdict-ttl-minimum-" + test.name
			_, verdict, err := owner.DecideV1(f.ctx, verdictowner.DecideCommandV1{TenantID: attested.TenantID, CaseID: attested.ID, Expected: reviewport.ExpectedV1(attested.Revision, attested.Digest), AttestationID: attestation.ID, VerdictID: verdictID, Trace: testkit.Trace(f.clock.Now(), attested, contract.TraceVerdictV1, 4, verdictID)})
			if err != nil {
				t.Fatal(err)
			}
			if verdict.ExpiresUnixNano != selectedExpiry {
				t.Fatalf("%s was not the unique minimum Verdict TTL: got=%d want=%d", test.name, verdict.ExpiresUnixNano, selectedExpiry)
			}
		})

		t.Run(test.name+"/actual_point_expired", func(t *testing.T) {
			f, attested, attestation, source, selectedExpiry := setup(t, test)
			countingStore := &decideCallCountingStore{StoreV1: f.store}
			reader := clockAtDecisionCurrentReturn{base: source, clock: f.clock, at: time.Unix(0, selectedExpiry)}
			owner, err := verdictowner.New(countingStore, reader, f.clock.Now)
			if err != nil {
				t.Fatal(err)
			}
			verdictID := "verdict-ttl-expired-" + test.name
			_, _, err = owner.DecideV1(f.ctx, verdictowner.DecideCommandV1{TenantID: attested.TenantID, CaseID: attested.ID, Expected: reviewport.ExpectedV1(attested.Revision, attested.Digest), AttestationID: attestation.ID, VerdictID: verdictID, Trace: testkit.Trace(f.clock.Now(), attested, contract.TraceVerdictV1, 4, verdictID)})
			if err == nil {
				t.Fatalf("%s expiry at Decide actual point reached Verdict", test.name)
			}
			if countingStore.decideCalls.Load() != 0 {
				t.Fatalf("%s expiry reached Verdict CAS: calls=%d", test.name, countingStore.decideCalls.Load())
			}
			if _, inspectErr := f.store.InspectVerdictV1(f.ctx, attested.TenantID, verdictID); !core.HasCategory(inspectErr, core.ErrorNotFound) {
				t.Fatalf("%s expiry leaked Verdict: %v", test.name, inspectErr)
			}
			assertCurrentCaseExact(t, f.store, attested)
		})
	}
}

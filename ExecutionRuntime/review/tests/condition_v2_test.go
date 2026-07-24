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
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestConditionV2SingleOwnerExactChainAndShortestTTL(t *testing.T) {
	f := newReviewingFlow(t, contract.RouteHumanV1)
	f.clock.Advance(time.Second)
	attestation := testkit.HumanAttestation(f.clock.Now(), f.caseValue, f.round, f.assignment, contract.ResolutionConditionalV1, "condition-v2-owner")
	attested, stored, err := f.engine.RecordAttestationV1(f.ctx, reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), attestation, testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceAttestedV1, 3, attestation.ID))
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Conditions) != 1 || stored.Conditions[0] != attestation.Conditions[0] {
		t.Fatal("Store did not preserve exact Attestation conditions")
	}
	f.clock.Advance(time.Second)
	source, err := memory.NewDecisionCurrentSourceV1(f.store, &testkit.ExternalCurrentReader{}, f.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := verdictowner.New(f.store, source, f.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	_, verdict, err := owner.DecideV1(f.ctx, verdictowner.DecideCommandV1{TenantID: attested.TenantID, CaseID: attested.ID, Expected: reviewport.ExpectedV1(attested.Revision, attested.Digest), AttestationID: attestation.ID, VerdictID: "verdict-condition-v2", Trace: testkit.Trace(f.clock.Now(), attested, contract.TraceVerdictV1, 4, "verdict-condition-v2")})
	if err != nil {
		t.Fatal(err)
	}
	if verdict.State != contract.VerdictConditionalV1 || verdict.ConditionsDigest != attestation.ConditionsDigest || len(verdict.Conditions) != 1 || verdict.Conditions[0] != attestation.Conditions[0] || verdict.ExpiresUnixNano != attestation.ExpiresUnixNano {
		t.Fatalf("Verdict did not preserve exact shortest-TTL condition chain: %+v", verdict)
	}
}

func TestConditionV2StoreFailClosedZeroWriteMatrix(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*contract.AttestationV1) error
	}{
		{name: "scope drift", mutate: func(a *contract.AttestationV1) error {
			a.Conditions[0].ScopeDigest = testkit.Digest("different-scope")
			a.ConditionsDigest, _ = runtimeports.DigestReviewConditionsV2(a.Conditions)
			a.Digest = ""
			var err error
			*a, err = contract.SealAttestationV1(*a)
			return err
		}},
		{name: "condition expired at observation", mutate: func(a *contract.AttestationV1) error {
			a.Conditions[0].ExpiresUnixNano = a.ObservedUnixNano
			a.ConditionsDigest, _ = runtimeports.DigestReviewConditionsV2(a.Conditions)
			a.Digest = ""
			_, err := contract.SealAttestationV1(*a)
			return err
		}},
		{name: "legacy digest only", mutate: func(a *contract.AttestationV1) error {
			a.Conditions = nil
			a.Digest = ""
			a.Digest, _ = core.CanonicalJSONDigest("praxis.review", contract.ContractVersionV1, "AttestationV1", *a)
			return nil
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newReviewingFlow(t, contract.RouteHumanV1)
			f.clock.Advance(time.Second)
			attestation := testkit.HumanAttestation(f.clock.Now(), f.caseValue, f.round, f.assignment, contract.ResolutionConditionalV1, "condition-v2-zero")
			idempotencyKey := attestation.IdempotencyKey
			if mutationErr := tc.mutate(&attestation); mutationErr == nil {
				_, _, err := f.engine.RecordAttestationV1(f.ctx, reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), attestation, testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceAttestedV1, 3, attestation.ID))
				if err == nil {
					t.Fatal("invalid conditional Attestation was committed")
				}
			} else if !core.HasReason(mutationErr, core.ReasonReviewConditionUnsatisfied) {
				t.Fatalf("invalid condition failed with wrong reason: %v", mutationErr)
			}
			if _, inspectErr := f.store.InspectAttestationByIdempotencyV1(context.Background(), f.caseValue.TenantID, idempotencyKey); !core.HasCategory(inspectErr, core.ErrorNotFound) {
				t.Fatalf("failed conditional mutation leaked Attestation: %v", inspectErr)
			}
			current, inspectErr := f.store.InspectCaseV1(context.Background(), f.caseValue.TenantID, f.caseValue.ID)
			if inspectErr != nil || current.Digest != f.caseValue.Digest {
				t.Fatalf("failed conditional mutation changed Case: %+v err=%v", current, inspectErr)
			}
		})
	}
}

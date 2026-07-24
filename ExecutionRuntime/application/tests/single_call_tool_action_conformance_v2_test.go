package application_test

import (
	"context"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/application/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/application/fakes"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestSingleCallToolActionCoordinationConformanceV2(t *testing.T) {
	fx := newSingleCallFixtureV2(t)
	prepared := mustPreparedFactV2(t, fx.request)
	dispatch := mustNextFactV2(t, prepared, contract.SingleCallToolActionDispatchIntentV2, nil, fx.now)
	claimID, err := contract.DeriveSingleCallToolActionStartClaimIDV2(fx.request)
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := contract.ClaimSingleCallToolActionStartV2(dispatch, claimID, fx.now)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := contract.CompleteSingleCallToolActionCoordinationFactV2(waiting, fx.result, fx.now)
	if err != nil {
		t.Fatal(err)
	}
	changedRequest := fx.request
	changedRequest.ExpiresUnixNano--
	changedRequest, err = contract.SealSingleCallToolActionRequestV2(changedRequest)
	if err != nil {
		t.Fatal(err)
	}
	conflicting := mustPreparedFactV2(t, changedRequest)
	report, err := conformance.CheckSingleCallToolActionCoordinationPortV2(context.Background(), conformance.SingleCallToolActionCoordinationCaseV2{
		NewPort: func() applicationports.SingleCallToolActionCoordinationFactPortV2 {
			return fakes.NewSingleCallToolActionCoordinationStoreV2()
		},
		Prepared:    prepared,
		Dispatch:    mustCASV2(t, prepared, dispatch),
		StartClaim:  mustCASV2(t, dispatch, waiting),
		Completed:   mustCompletedSuccessorCASV2(t, waiting, completed),
		Conflicting: conflicting,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.AtomicInitialClaim || !report.ExactInspect || !report.IdempotentReplay || !report.ConcurrentLinearization || !report.StateMachine || !report.ChangedContentRejected || !report.CertificationCandidate {
		t.Fatalf("incomplete conformance report: %+v", report)
	}
	if report.BindingEligible || report.ProductionEligible || report.DispatchEligible || report.SystemG6AEligible {
		t.Fatalf("fixture overstated production/system eligibility: %+v", report)
	}
}

func TestSingleCallToolActionCoordinationConformanceV2RejectsTypedNilPort(t *testing.T) {
	fx := newSingleCallFixtureV2(t)
	prepared := mustPreparedFactV2(t, fx.request)
	dispatch := mustNextFactV2(t, prepared, contract.SingleCallToolActionDispatchIntentV2, nil, fx.now)
	claimID, err := contract.DeriveSingleCallToolActionStartClaimIDV2(fx.request)
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := contract.ClaimSingleCallToolActionStartV2(dispatch, claimID, fx.now)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := contract.CompleteSingleCallToolActionCoordinationFactV2(waiting, fx.result, fx.now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = conformance.CheckSingleCallToolActionCoordinationPortV2(context.Background(), conformance.SingleCallToolActionCoordinationCaseV2{
		NewPort: func() applicationports.SingleCallToolActionCoordinationFactPortV2 {
			var store *fakes.SingleCallToolActionCoordinationStoreV2
			return store
		},
		Prepared:   prepared,
		Dispatch:   mustCASV2(t, prepared, dispatch),
		StartClaim: mustCASV2(t, dispatch, waiting),
		Completed:  mustCompletedSuccessorCASV2(t, waiting, completed),
	})
	if !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("typed-nil conformance port accepted: %v", err)
	}
}

func TestSingleCallToolActionCoordinationConformanceV2RejectsNilContext(t *testing.T) {
	_, err := conformance.CheckSingleCallToolActionCoordinationPortV2(nil, conformance.SingleCallToolActionCoordinationCaseV2{})
	if !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil conformance context accepted: %v", err)
	}
}

func TestSingleCallToolActionCoordinationConformanceV2WithoutConflictIsNotCandidate(t *testing.T) {
	fx := newSingleCallFixtureV2(t)
	prepared := mustPreparedFactV2(t, fx.request)
	dispatch := mustNextFactV2(t, prepared, contract.SingleCallToolActionDispatchIntentV2, nil, fx.now)
	claimID, err := contract.DeriveSingleCallToolActionStartClaimIDV2(fx.request)
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := contract.ClaimSingleCallToolActionStartV2(dispatch, claimID, fx.now)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := contract.CompleteSingleCallToolActionCoordinationFactV2(waiting, fx.result, fx.now)
	if err != nil {
		t.Fatal(err)
	}
	report, err := conformance.CheckSingleCallToolActionCoordinationPortV2(context.Background(), conformance.SingleCallToolActionCoordinationCaseV2{
		NewPort: func() applicationports.SingleCallToolActionCoordinationFactPortV2 {
			return fakes.NewSingleCallToolActionCoordinationStoreV2()
		},
		Prepared: prepared, Dispatch: mustCASV2(t, prepared, dispatch), StartClaim: mustCASV2(t, dispatch, waiting), Completed: mustCompletedSuccessorCASV2(t, waiting, completed),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.AtomicInitialClaim || report.ChangedContentRejected || report.CertificationCandidate {
		t.Fatalf("conformance without changed-content case overstated certification: %+v", report)
	}
}

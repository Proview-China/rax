package application_test

import (
	"context"
	"testing"
	"time"

	application "github.com/Proview-China/rax/ExecutionRuntime/application"
	applicationconformance "github.com/Proview-China/rax/ExecutionRuntime/application/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/application/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestAgentActivationV2ConformanceCandidateNeverClaimsProduction(t *testing.T) {
	fx := newActivationFixtureV2(t)
	if _, err := fx.coordinator.StartOrInspectAgentActivationV2(context.Background(), fx.request); err != nil {
		t.Fatal(err)
	}
	fact, err := fx.store.InspectAgentActivationCoordinationV2(context.Background(), fx.request.ActivationID)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := applicationconformance.CheckAgentActivationCoordinationV2(fact, fx.now)
	if err != nil {
		t.Fatal(err)
	}
	if !candidate.EightStepOrderClosed || !candidate.VersionClaimAtomicPayload || !candidate.AppendOnlyHistory || !candidate.InvocationWriteAhead || !candidate.UnknownInspectOnly || !candidate.CommittedScopeExact || candidate.ProductionEligible {
		t.Fatalf("unexpected candidate: %+v", candidate)
	}
}

func TestAgentActivationV2CanonicalSplicesFailClosed(t *testing.T) {
	fx := newActivationFixtureV2(t)
	if _, err := fx.coordinator.StartOrInspectAgentActivationV2(context.Background(), fx.request); err != nil {
		t.Fatal(err)
	}
	fact, err := fx.store.InspectAgentActivationCoordinationV2(context.Background(), fx.request.ActivationID)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*contract.AgentActivationCoordinationFactV2){
		"claim-version": func(value *contract.AgentActivationCoordinationFactV2) {
			value.Claim.ClaimedVersion = "praxis.application.agent-activation/v1"
		},
		"event-order":        func(value *contract.AgentActivationCoordinationFactV2) { value.Events[2].Sequence++ },
		"committed-instance": func(value *contract.AgentActivationCoordinationFactV2) { value.Result.ExecutionScope.Instance.Epoch++ },
		"result-current": func(value *contract.AgentActivationCoordinationFactV2) {
			value.Result.ExecutionReadyCurrent.Digest = core.DigestBytes([]byte("spliced"))
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			copy := fact
			copy.Events = append([]contract.AgentActivationStepEventV2{}, fact.Events...)
			result := *fact.Result
			copy.Result = &result
			mutate(&copy)
			if copy.Validate() == nil {
				t.Fatal("canonical splice was accepted")
			}
		})
	}
}

func TestAgentActivationV2TypedNilDependenciesFailClosed(t *testing.T) {
	fx := newActivationFixtureV2(t)
	var nilStore *fakes.AgentActivationCoordinationStoreV2
	if _, err := application.NewAgentActivationCoordinatorV2(nilStore, fx.ports, func() time.Time { return fx.now }); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("typed nil store accepted: %v", err)
	}
}

package conformance_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
)

func TestRefreshApplyContractHasNoRuntimeSettlementOrContinuation(t *testing.T) {
	fixture, err := testfixture.NewRefreshFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(contract.ApplyContextTurnRefreshRequestV1{ExpectedCurrent: fixture.Request.ExpectedCurrent})
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	for _, forbidden := range []string{"runtime_settlement", "operation_settlement", "continuation", "harness", "turn_cas"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("forbidden cross-owner field %q in %s", forbidden, text)
		}
	}
}

func TestRefreshSourceCardinalityConformance(t *testing.T) {
	for _, cardinality := range []contract.ContextTurnRefreshSourceCardinalityV1{{}, {Tool: 2}, {Tool: 1, Memory: 2}, {Tool: 1, Knowledge: 2}, {Tool: 1, Continuity: 1}} {
		if cardinality.Validate() == nil {
			t.Fatalf("unsupported source cardinality accepted: %#v", cardinality)
		}
	}
	for _, cardinality := range []contract.ContextTurnRefreshSourceCardinalityV1{{Tool: 1}, {Tool: 1, Memory: 1}, {Tool: 1, Knowledge: 1}, {Tool: 1, Memory: 1, Knowledge: 1}} {
		if err := cardinality.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}

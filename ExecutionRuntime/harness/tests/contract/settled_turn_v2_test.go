package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestSettledTurnResultV2RoundTripsActionAndRejectsCandidateSwap(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	_, candidate := testkit.GovernedFactsV2(now)
	ref, _ := candidate.RefV2()
	action, err := contract.NewPendingActionV2("action-1", "custom.tool/execute", candidate.Input, ref)
	if err != nil {
		t.Fatal(err)
	}
	result := contract.SettledTurnResultV2{ContractVersion: contract.SettledTurnResultContractV2, Candidate: ref, State: contract.SettledTurnActionRequiredV2, Action: &action}
	payload, err := contract.NewSettledTurnDomainResultV2(result)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := contract.DecodeSettledTurnDomainResultV2(payload)
	if err != nil || decoded.Action == nil || decoded.Action.RequestDigest != action.RequestDigest {
		t.Fatalf("settled action did not round trip: %#v err=%v", decoded, err)
	}
	changed := result
	changed.Candidate.ID = "candidate-other"
	if _, err := contract.NewSettledTurnDomainResultV2(changed); !core.HasReason(err, core.ReasonInvalidState) {
		t.Fatalf("settled action accepted another source candidate: %v", err)
	}
}

func TestSettledTurnResultV2RejectsProviderStyleUnsettledShapes(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	_, candidate := testkit.GovernedFactsV2(now)
	ref, _ := candidate.RefV2()
	output := candidate.Input
	for name, value := range map[string]contract.SettledTurnResultV2{
		"completed without output":     {ContractVersion: contract.SettledTurnResultContractV2, Candidate: ref, State: contract.SettledTurnCompletedV2},
		"failed without exact failure": {ContractVersion: contract.SettledTurnResultContractV2, Candidate: ref, State: contract.SettledTurnFailedV2},
		"action with provider output":  {ContractVersion: contract.SettledTurnResultContractV2, Candidate: ref, State: contract.SettledTurnActionRequiredV2, Output: &output},
	} {
		t.Run(name, func(t *testing.T) {
			if err := value.Validate(); err == nil {
				t.Fatal("unsettled/ambiguous turn shape was accepted")
			}
		})
	}
	failure := contract.NewSettledTurnFailureV2(ref, "custom.model/provider-failed", []byte("provider failed"))
	if err := failure.Validate(); err != nil || failure.IsTerminalClaimV2() != contract.ClaimFailed {
		t.Fatalf("exact failure result is invalid: %v", err)
	}
}

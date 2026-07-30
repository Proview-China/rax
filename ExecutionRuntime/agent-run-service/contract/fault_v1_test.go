package contract_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/contract"
)

func TestFaultV1PreservesFrozenPublicTaxonomy(t *testing.T) {
	commandRef := exactRefV1(t, contract.AgentRunCommandRefKindV1, "command-1", 1)
	codes := []contract.FaultCodeV1{
		contract.FaultInvalidArgumentV1, contract.FaultUnauthenticatedV1, contract.FaultForbiddenV1,
		contract.FaultNotFoundV1, contract.FaultRevisionConflictV1, contract.FaultIdempotencyConflictV1,
		contract.FaultPreconditionFailedV1, contract.FaultCapabilityUnavailableV1, contract.FaultUnavailableV1,
		contract.FaultRateLimitedV1, contract.FaultUnknownOutcomeV1, contract.FaultIndeterminateV1,
		contract.FaultInternalV1,
	}
	seen := map[contract.FaultCodeV1]struct{}{}
	for _, code := range codes {
		fault := contract.FaultV1{Code: code, Reason: "reason-1", Message: "typed failure", TraceID: "trace-1", RetryDirective: contract.RetryNoneV1}
		if code == contract.FaultUnknownOutcomeV1 || code == contract.FaultIndeterminateV1 {
			fault.CommandRef = &commandRef
			fault.RetryDirective = contract.RetryInspectV1
		}
		if code == contract.FaultIdempotencyConflictV1 {
			fault.CommandRef = &commandRef
		}
		if err := fault.Validate(); err != nil {
			t.Fatalf("code=%s err=%v", code, err)
		}
		seen[code] = struct{}{}
	}
	if len(seen) != 13 {
		t.Fatalf("fault taxonomy count=%d", len(seen))
	}
}

func TestFaultV1UnknownRequiresOneInspectableOriginal(t *testing.T) {
	commandRef := exactRefV1(t, contract.AgentRunCommandRefKindV1, "command-1", 1)
	attemptRef := exactRefV1(t, "praxis.runtime/attempt", "attempt-1", 1)
	base := contract.FaultV1{Code: contract.FaultUnknownOutcomeV1, Reason: "lost_reply", Message: "reply was lost", TraceID: "trace-1", RetryDirective: contract.RetryInspectV1}
	if err := base.Validate(); err == nil {
		t.Fatal("accepted unknown outcome without original subject")
	}
	base.AttemptRef = &attemptRef
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	base.CommandRef = &commandRef
	if err := base.Validate(); err == nil {
		t.Fatal("accepted ambiguous command+attempt subject without association")
	}
}

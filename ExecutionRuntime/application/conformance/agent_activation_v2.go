package conformance

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type AgentActivationCertificationCandidateV2 struct {
	ContractVersion           string `json:"contract_version"`
	EightStepOrderClosed      bool   `json:"eight_step_order_closed"`
	VersionClaimAtomicPayload bool   `json:"version_claim_atomic_payload"`
	AppendOnlyHistory         bool   `json:"append_only_history"`
	InvocationWriteAhead      bool   `json:"invocation_write_ahead"`
	UnknownInspectOnly        bool   `json:"unknown_inspect_only"`
	CommittedScopeExact       bool   `json:"committed_scope_exact"`
	ProductionEligible        bool   `json:"production_eligible"`
}

// CheckAgentActivationCoordinationV2 validates evidence already present in one
// aggregate. It intentionally cannot certify durability, Owner Readers or a
// production composition root.
func CheckAgentActivationCoordinationV2(fact contract.AgentActivationCoordinationFactV2, now time.Time) (AgentActivationCertificationCandidateV2, error) {
	if err := fact.Validate(); err != nil {
		return AgentActivationCertificationCandidateV2{}, err
	}
	if now.IsZero() || now.UnixNano() < fact.UpdatedUnixNano {
		return AgentActivationCertificationCandidateV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Agent activation V2 conformance clock regressed")
	}
	completed := 0
	invocationWriteAhead := true
	unknownInspectOnly := true
	committedScopeExact := false
	seenInvocation := map[contract.AgentActivationStepV2]core.Digest{}
	for index, event := range fact.Events {
		switch event.State {
		case contract.AgentActivationStepInvocationRecordedV2:
			seenInvocation[event.Step] = event.RequestDigest
		case contract.AgentActivationStepOutcomeUnknownV2:
			if seenInvocation[event.Step] != event.RequestDigest || index+1 >= len(fact.Events) {
				unknownInspectOnly = false
			}
		case contract.AgentActivationStepResultRecordedV2:
			if seenInvocation[event.Step] != event.RequestDigest {
				invocationWriteAhead = false
			}
			if index > 0 && fact.Events[index-1].State == contract.AgentActivationStepOutcomeUnknownV2 && fact.Events[index-1].RequestDigest != event.RequestDigest {
				unknownInspectOnly = false
			}
		}
		if event.State == contract.AgentActivationStepResultRecordedV2 {
			completed++
			if event.Step == contract.AgentActivationCommitV2 && event.Result != nil && event.Result.Proof.CommittedScope != nil && fact.Result != nil {
				committedScopeExact = *event.Result.Proof.CommittedScope == fact.Result.ExecutionScope
			}
		}
	}
	return AgentActivationCertificationCandidateV2{
		ContractVersion:           contract.AgentActivationContractVersionV2,
		EightStepOrderClosed:      completed == len(contract.AgentActivationStepOrderV2()),
		// A single aggregate proves exact payload binding, not the store's atomic
		// Create or append-only CAS behavior. Those remain store-test evidence.
		VersionClaimAtomicPayload: false,
		AppendOnlyHistory:         false,
		InvocationWriteAhead:      invocationWriteAhead,
		UnknownInspectOnly:        unknownInspectOnly,
		CommittedScopeExact:       committedScopeExact,
		ProductionEligible:        false,
	}, nil
}

package kernel

import (
	"fmt"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

func ApplySettlement(id string, result contract.DomainResultFact, operation contract.OperationSettlementRef, appliedUnixNano int64) (contract.DomainSettlementFact, error) {
	if err := result.Validate(); err != nil {
		return contract.DomainSettlementFact{}, err
	}
	if err := operation.Validate(); err != nil {
		return contract.DomainSettlementFact{}, err
	}
	resultDigest, err := contract.DigestJSON(result)
	if err != nil {
		return contract.DomainSettlementFact{}, err
	}
	if operation.DomainResultDigest != resultDigest {
		return contract.DomainSettlementFact{}, fmt.Errorf("%w: operation settlement references different domain result", contract.ErrConflict)
	}
	fact := contract.DomainSettlementFact{
		ContractVersion: contract.Version, ID: id, Revision: 1,
		DomainResultRef: contract.FactRef{ID: result.ID, Revision: result.Revision, Digest: resultDigest},
		OperationRef:    operation, AppliedUnixNano: appliedUnixNano,
	}
	return fact, fact.Validate()
}

func RecoveryAction(result contract.DomainResultFact) (string, error) {
	if err := result.Validate(); err != nil {
		return "", err
	}
	if result.State == contract.DomainResultUnknown {
		return "inspect_original_attempt", nil
	}
	return "apply_runtime_settlement_ref", nil
}

package domain

import "github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"

// ApplySettlement enforces the Wave 1 layering:
// DomainResultFact -> Runtime Operation Settlement (ref only) -> Domain apply.
// It does not inspect or mutate Runtime state and does not execute an effect.
func ApplySettlement(fact contract.DomainResultFact, settlement contract.OperationSettlementRef) (contract.AppliedSettlement, error) {
	if err := fact.Validate(); err != nil {
		return contract.AppliedSettlement{}, err
	}
	if err := settlement.Validate(); err != nil {
		return contract.AppliedSettlement{}, err
	}
	if settlement.OperationID != fact.OperationID ||
		settlement.OperationDigest != fact.OperationDigest ||
		settlement.DomainResultFactID != fact.FactRef.ID ||
		settlement.DomainResultDigest != fact.FactRef.Digest {
		return contract.AppliedSettlement{}, contract.NewError(contract.ErrRevisionConflict, "settlement_ref", "settlement does not bind the exact domain result fact")
	}
	return contract.AppliedSettlement{DomainResultFactRef: fact.FactRef, RuntimeSettlement: settlement}, nil
}

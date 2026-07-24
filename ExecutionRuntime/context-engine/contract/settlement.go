package contract

import "fmt"

type DomainResultState string

const (
	DomainResultSucceeded DomainResultState = "succeeded"
	DomainResultFailed    DomainResultState = "failed"
	DomainResultUnknown   DomainResultState = "unknown"
)

type DomainResultFact struct {
	ContractVersion string            `json:"contract_version"`
	ID              string            `json:"fact_id"`
	Revision        uint64            `json:"revision"`
	AttemptID       string            `json:"attempt_id"`
	IntentDigest    Digest            `json:"intent_digest"`
	ResultDigest    Digest            `json:"result_digest"`
	State           DomainResultState `json:"state"`
	CreatedUnixNano int64             `json:"created_unix_nano"`
}

func (f DomainResultFact) Validate() error {
	if ValidateContract(f.ContractVersion) != nil || validateID(f.ID) != nil || f.Revision != 1 || validateID(f.AttemptID) != nil || f.IntentDigest.Validate() != nil || f.ResultDigest.Validate() != nil || f.CreatedUnixNano <= 0 {
		return fmt.Errorf("%w: domain result fact", ErrInvalid)
	}
	if f.State != DomainResultSucceeded && f.State != DomainResultFailed && f.State != DomainResultUnknown {
		return fmt.Errorf("%w: domain result state", ErrInvalid)
	}
	return nil
}

// OperationSettlementRef is an opaque, ref-only projection of the Runtime-owned
// settlement. This module neither creates nor interprets Runtime outcome facts.
type OperationSettlementRef struct {
	OperationID        string `json:"operation_id"`
	OperationRevision  uint64 `json:"operation_revision"`
	SettlementDigest   Digest `json:"settlement_digest"`
	DomainResultDigest Digest `json:"domain_result_digest"`
}

func (r OperationSettlementRef) Validate() error {
	if validateID(r.OperationID) != nil || r.OperationRevision == 0 || r.SettlementDigest.Validate() != nil || r.DomainResultDigest.Validate() != nil {
		return fmt.Errorf("%w: operation settlement reference", ErrInvalid)
	}
	return nil
}

type DomainSettlementFact struct {
	ContractVersion string                 `json:"contract_version"`
	ID              string                 `json:"fact_id"`
	Revision        uint64                 `json:"revision"`
	DomainResultRef FactRef                `json:"domain_result_ref"`
	OperationRef    OperationSettlementRef `json:"operation_settlement_ref"`
	AppliedUnixNano int64                  `json:"applied_unix_nano"`
}

func (f DomainSettlementFact) Validate() error {
	if ValidateContract(f.ContractVersion) != nil || validateID(f.ID) != nil || f.Revision != 1 || f.DomainResultRef.Validate() != nil || f.OperationRef.Validate() != nil || f.AppliedUnixNano <= 0 {
		return fmt.Errorf("%w: domain settlement fact", ErrInvalid)
	}
	if f.DomainResultRef.Digest != f.OperationRef.DomainResultDigest {
		return fmt.Errorf("%w: settlement result digest", ErrConflict)
	}
	return nil
}

package contract

import (
	"bytes"
	"encoding/json"
	"io"
)

type Outcome string

const (
	OutcomeSucceeded Outcome = "succeeded"
	OutcomeFailed    Outcome = "failed"
	OutcomeUnknown   Outcome = "unknown"
)

type DomainResultFact struct {
	FactRef                 FactRef  `json:"fact_ref"`
	OperationID             string   `json:"operation_id"`
	OperationDigest         string   `json:"operation_digest"`
	EffectKind              string   `json:"effect_kind"`
	Outcome                 Outcome  `json:"outcome"`
	ObservationEvidenceRefs []string `json:"observation_evidence_refs"`
}

func (f DomainResultFact) Validate() error {
	if err := f.FactRef.Validate(); err != nil {
		return err
	}
	for field, value := range map[string]string{
		"operation_id": f.OperationID, "operation_digest": f.OperationDigest, "effect_kind": f.EffectKind,
	} {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	if len(f.ObservationEvidenceRefs) == 0 {
		return NewError(ErrEvidenceNotInspectable, "observation_evidence_refs", "domain result requires inspected evidence")
	}
	switch f.Outcome {
	case OutcomeSucceeded, OutcomeFailed, OutcomeUnknown:
		return nil
	default:
		return NewError(ErrInvalidArgument, "outcome", "unknown outcome")
	}
}

// OperationSettlementRef is deliberately ref-only. Continuity does not own or
// persist the Runtime settlement object represented by this reference. It
// carries identity and digest bindings only; Runtime outcome, disposition,
// status, or other semantics must never be mirrored here.
type OperationSettlementRef struct {
	SettlementID       string `json:"settlement_id"`
	SettlementDigest   string `json:"settlement_digest"`
	OperationID        string `json:"operation_id"`
	OperationDigest    string `json:"operation_digest"`
	DomainResultFactID string `json:"domain_result_fact_id"`
	DomainResultDigest string `json:"domain_result_digest"`
}

func (r OperationSettlementRef) Validate() error {
	for field, value := range map[string]string{
		"settlement_id": r.SettlementID, "settlement_digest": r.SettlementDigest,
		"operation_id": r.OperationID, "operation_digest": r.OperationDigest,
		"domain_result_fact_id": r.DomainResultFactID, "domain_result_digest": r.DomainResultDigest,
	} {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	return nil
}

// UnmarshalJSON rejects semantic copies instead of silently ignoring them.
// The only accepted wire fields are the opaque identity/digest bindings above.
func (r *OperationSettlementRef) UnmarshalJSON(data []byte) error {
	type wire OperationSettlementRef
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var decoded wire
	if err := decoder.Decode(&decoded); err != nil {
		return NewError(ErrInvalidArgument, "operation_settlement_ref", "contains an unknown or malformed field")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return NewError(ErrInvalidArgument, "operation_settlement_ref", "contains trailing data")
	}
	value := OperationSettlementRef(decoded)
	if err := value.Validate(); err != nil {
		return err
	}
	*r = value
	return nil
}

type AppliedSettlement struct {
	DomainResultFactRef FactRef                `json:"domain_result_fact_ref"`
	RuntimeSettlement   OperationSettlementRef `json:"runtime_settlement_ref"`
}

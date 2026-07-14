package contract

import (
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// UndispatchedSettlementBindingV2 is the only terminal sidecar allowed when a
// model-turn Effect settled before any provider Prepare existed.
type UndispatchedSettlementBindingV2 struct {
	Candidate          CandidateRefV2                        `json:"candidate"`
	Settlement         runtimeports.OperationSettlementRefV3 `json:"settlement"`
	DomainResultSchema runtimeports.SchemaRefV2              `json:"domain_result_schema"`
	DomainResultDigest core.Digest                           `json:"domain_result_digest"`
}

func (b UndispatchedSettlementBindingV2) Validate() error {
	if err := b.Candidate.Validate(); err != nil {
		return err
	}
	if err := b.Settlement.Validate(); err != nil {
		return err
	}
	if b.Settlement.Observation != nil || b.Settlement.Attempt.Delegation != nil || b.Settlement.Disposition == runtimeports.OperationSettlementAppliedV3 {
		return core.NewError(core.ErrorConflict, core.ReasonEffectUnknownOutcome, "undispatched settlement cannot claim provider execution")
	}
	if err := b.DomainResultSchema.Validate(); err != nil {
		return err
	}
	if err := b.DomainResultDigest.Validate(); err != nil {
		return err
	}
	if b.Settlement.DomainResultSchema == nil || *b.Settlement.DomainResultSchema != b.DomainResultSchema || b.Settlement.DomainResultDigest != b.DomainResultDigest || b.DomainResultSchema != SettledTurnResultSchemaV2() {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "undispatched settlement binds another DomainResult")
	}
	return nil
}

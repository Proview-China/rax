package contract

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ReviewerInvocationResultFactV1 is the Review domain result. It is not a
// Runtime settlement and cannot by itself authorize an Attestation or Verdict.
type ReviewerInvocationResultFactV1 struct {
	FactIdentityV1
	CaseID             string                   `json:"case_id"`
	CaseRevision       core.Revision            `json:"case_revision"`
	RoundID            string                   `json:"round_id"`
	RoundRevision      core.Revision            `json:"round_revision"`
	RoundDigest        core.Digest              `json:"round_digest"`
	AssignmentID       string                   `json:"assignment_id"`
	AssignmentRevision core.Revision            `json:"assignment_revision"`
	AssignmentDigest   core.Digest              `json:"assignment_digest"`
	TargetID           string                   `json:"target_id"`
	TargetRevision     core.Revision            `json:"target_revision"`
	TargetDigest       core.Digest              `json:"target_digest"`
	AttemptID          string                   `json:"attempt_id"`
	ResultSchema       runtimeports.SchemaRefV2 `json:"result_schema"`
	ResultDigest       core.Digest              `json:"result_digest"`
	ObservationRefs    []string                 `json:"observation_refs"`
}

func (f ReviewerInvocationResultFactV1) ExactRef() ExactResourceRefV1 {
	return ExactResourceRefV1{ID: f.ID, Revision: f.Revision, Digest: f.Digest}
}

func (f ReviewerInvocationResultFactV1) digestValue() ReviewerInvocationResultFactV1 {
	f.Digest = ""
	return f
}
func (f ReviewerInvocationResultFactV1) validateShape() error {
	if err := f.FactIdentityV1.ValidateShape(); err != nil {
		return err
	}
	if f.Revision != 1 || invalidID(f.CaseID) || f.CaseRevision == 0 || invalidID(f.RoundID) || f.RoundRevision == 0 || f.RoundDigest.Validate() != nil || invalidID(f.AssignmentID) || f.AssignmentRevision == 0 || f.AssignmentDigest.Validate() != nil || invalidID(f.TargetID) || f.TargetRevision == 0 || f.TargetDigest.Validate() != nil || invalidID(f.AttemptID) || f.ResultDigest.Validate() != nil || len(f.ObservationRefs) == 0 || len(f.ObservationRefs) > MaxListItemsV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "reviewer invocation domain result is incomplete")
	}
	return f.ResultSchema.Validate()
}
func SealReviewerInvocationResultFactV1(f ReviewerInvocationResultFactV1) (ReviewerInvocationResultFactV1, error) {
	f.ContractVersion = ContractVersionV1
	f.Digest = ""
	if err := f.validateShape(); err != nil {
		return ReviewerInvocationResultFactV1{}, err
	}
	d, err := seal("ReviewerInvocationResultFactV1", f.digestValue())
	if err != nil {
		return ReviewerInvocationResultFactV1{}, err
	}
	f.Digest = d
	return f, f.Validate()
}
func (f ReviewerInvocationResultFactV1) Validate() error {
	if err := f.validateShape(); err != nil {
		return err
	}
	return validateSealed("ReviewerInvocationResultFactV1", f.digestValue(), f.Digest)
}

type DomainApplyStateV1 string

const (
	DomainApplyAppliedV1    DomainApplyStateV1 = "applied"
	DomainApplyNotAppliedV1 DomainApplyStateV1 = "not_applied"
	DomainApplyFailedV1     DomainApplyStateV1 = "failed"
)

type DomainApplySettlementRefV1 struct {
	ID                        string             `json:"id"`
	Revision                  core.Revision      `json:"revision"`
	Digest                    core.Digest        `json:"digest"`
	DomainResultID            string             `json:"domain_result_id"`
	DomainResultDigest        core.Digest        `json:"domain_result_digest"`
	RuntimeSettlementID       string             `json:"runtime_settlement_id"`
	RuntimeSettlementRevision core.Revision      `json:"runtime_settlement_revision"`
	RuntimeSettlementDigest   core.Digest        `json:"runtime_settlement_digest"`
	RuntimeContractVersion    string             `json:"runtime_contract_version,omitempty"`
	RuntimeInspectionDigest   core.Digest        `json:"runtime_inspection_digest,omitempty"`
	State                     DomainApplyStateV1 `json:"state"`
}

func (r DomainApplySettlementRefV1) Validate() error {
	if invalidID(r.ID) || r.Revision == 0 || r.Digest.Validate() != nil || invalidID(r.DomainResultID) || r.DomainResultDigest.Validate() != nil || invalidID(r.RuntimeSettlementID) || r.RuntimeSettlementRevision == 0 || r.RuntimeSettlementDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonSettlementOwnerMismatch, "domain ApplySettlement ref is incomplete")
	}
	if (r.RuntimeContractVersion == "") != (r.RuntimeInspectionDigest == "") {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonSettlementOwnerMismatch, "domain ApplySettlement Runtime inspection binding is partial")
	}
	if r.RuntimeContractVersion != "" && (r.RuntimeContractVersion != runtimeports.OperationSettlementContractVersionV4 || r.RuntimeInspectionDigest.Validate() != nil) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonSettlementOwnerMismatch, "domain ApplySettlement Runtime inspection binding is invalid")
	}
	switch r.State {
	case DomainApplyAppliedV1, DomainApplyNotAppliedV1, DomainApplyFailedV1:
		return nil
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "domain ApplySettlement state is unsupported")
	}
}

type DomainApplySettlementFactV1 struct {
	FactIdentityV1
	DomainResultID            string             `json:"domain_result_id"`
	DomainResultDigest        core.Digest        `json:"domain_result_digest"`
	RuntimeSettlementID       string             `json:"runtime_settlement_id"`
	RuntimeSettlementRevision core.Revision      `json:"runtime_settlement_revision"`
	RuntimeSettlementDigest   core.Digest        `json:"runtime_settlement_digest"`
	RuntimeContractVersion    string             `json:"runtime_contract_version,omitempty"`
	RuntimeInspectionDigest   core.Digest        `json:"runtime_inspection_digest,omitempty"`
	State                     DomainApplyStateV1 `json:"state"`
}

func (f DomainApplySettlementFactV1) digestValue() DomainApplySettlementFactV1 {
	f.Digest = ""
	return f
}
func (f DomainApplySettlementFactV1) validateShape() error {
	if err := f.FactIdentityV1.ValidateShape(); err != nil {
		return err
	}
	r := f.Ref()
	r.Digest = core.Digest("sha256:0000000000000000000000000000000000000000000000000000000000000000")
	return r.Validate()
}
func (f DomainApplySettlementFactV1) Ref() DomainApplySettlementRefV1 {
	return DomainApplySettlementRefV1{ID: f.ID, Revision: f.Revision, Digest: f.Digest, DomainResultID: f.DomainResultID, DomainResultDigest: f.DomainResultDigest, RuntimeSettlementID: f.RuntimeSettlementID, RuntimeSettlementRevision: f.RuntimeSettlementRevision, RuntimeSettlementDigest: f.RuntimeSettlementDigest, RuntimeContractVersion: f.RuntimeContractVersion, RuntimeInspectionDigest: f.RuntimeInspectionDigest, State: f.State}
}
func SealDomainApplySettlementFactV1(f DomainApplySettlementFactV1) (DomainApplySettlementFactV1, error) {
	f.ContractVersion = ContractVersionV1
	f.Digest = ""
	if err := f.validateShape(); err != nil {
		return DomainApplySettlementFactV1{}, err
	}
	d, err := seal("DomainApplySettlementFactV1", f.digestValue())
	if err != nil {
		return DomainApplySettlementFactV1{}, err
	}
	f.Digest = d
	return f, f.Validate()
}
func (f DomainApplySettlementFactV1) Validate() error {
	if err := f.validateShape(); err != nil {
		return err
	}
	return validateSealed("DomainApplySettlementFactV1", f.digestValue(), f.Digest)
}

// ApplyRuntimeSettlementV1 accepts only a Runtime-owned settlement reference.
// It never creates or mutates Runtime settlement or outcome state.
func ApplyRuntimeSettlementV1(id string, result ReviewerInvocationResultFactV1, settlement runtimeports.OperationSettlementRefV3, nowUnixNano int64) (DomainApplySettlementFactV1, error) {
	if err := result.Validate(); err != nil {
		return DomainApplySettlementFactV1{}, err
	}
	if err := settlement.Validate(); err != nil {
		return DomainApplySettlementFactV1{}, err
	}
	if nowUnixNano <= 0 {
		return DomainApplySettlementFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "ApplySettlement requires a current timestamp")
	}
	if settlement.DomainResultDigest != result.Digest || settlement.Attempt.AttemptID != result.AttemptID {
		return DomainApplySettlementFactV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Runtime settlement does not reference the exact Review domain result")
	}
	state := DomainApplyFailedV1
	switch settlement.Disposition {
	case runtimeports.OperationSettlementAppliedV3:
		state = DomainApplyAppliedV1
	case runtimeports.OperationSettlementNotAppliedV3:
		state = DomainApplyNotAppliedV1
	case runtimeports.OperationSettlementFailedV3:
		state = DomainApplyFailedV1
	}
	f := DomainApplySettlementFactV1{FactIdentityV1: FactIdentityV1{ContractVersion: ContractVersionV1, TenantID: result.TenantID, ID: id, Revision: 1, CreatedUnixNano: nowUnixNano, UpdatedUnixNano: nowUnixNano}, DomainResultID: result.ID, DomainResultDigest: result.Digest, RuntimeSettlementID: settlement.ID, RuntimeSettlementRevision: settlement.Revision, RuntimeSettlementDigest: settlement.Digest, State: state}
	return SealDomainApplySettlementFactV1(f)
}

// ApplyRuntimeSettlementV4 consumes only Runtime's current four-object
// inspection. V4 settlement of the exact Review DomainResult means the result
// fact is durably accepted; Provider observations still do not become a
// Verdict. The inspection digest is retained for later exact audit.
func ApplyRuntimeSettlementV4(id string, result ReviewerInvocationResultFactV1, inspection runtimeports.OperationInspectionSettlementRefV4, now time.Time) (DomainApplySettlementFactV1, error) {
	if err := result.Validate(); err != nil {
		return DomainApplySettlementFactV1{}, err
	}
	if err := inspection.Validate(now); err != nil {
		return DomainApplySettlementFactV1{}, err
	}
	fact := inspection.DomainResult
	if fact.TenantID != result.TenantID || fact.ID != result.ID || fact.Revision != result.Revision || fact.Digest != result.Digest || fact.Attempt.AttemptID != result.AttemptID || fact.Schema != result.ResultSchema || fact.PayloadDigest != result.ResultDigest || inspection.Settlement.DomainResult.ID != result.ID || inspection.Settlement.DomainResult.Digest != result.Digest {
		return DomainApplySettlementFactV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Runtime V4 settlement does not bind the exact Review DomainResult")
	}
	value := DomainApplySettlementFactV1{
		FactIdentityV1:            FactIdentityV1{ContractVersion: ContractVersionV1, TenantID: result.TenantID, ID: id, Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()},
		DomainResultID:            result.ID,
		DomainResultDigest:        result.Digest,
		RuntimeSettlementID:       inspection.Settlement.ID,
		RuntimeSettlementRevision: inspection.Settlement.Revision,
		RuntimeSettlementDigest:   inspection.Settlement.Digest,
		RuntimeContractVersion:    runtimeports.OperationSettlementContractVersionV4,
		RuntimeInspectionDigest:   inspection.Digest,
		State:                     DomainApplyAppliedV1,
	}
	return SealDomainApplySettlementFactV1(value)
}

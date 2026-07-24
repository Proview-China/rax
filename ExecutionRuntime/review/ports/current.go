package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// DecisionCurrentRequestV1 contains only exact Review identities. Callers
// cannot inject current values into Verdict Decide.
type DecisionCurrentRequestV1 struct {
	TenantID      core.TenantID
	CaseID        string
	ExpectedCase  ExpectedFactV1
	AttestationID string
}

// DecisionCurrentResolveRequestV1 identifies the exact current Case without
// asking a caller to remember the Attestation ID. The Review Store resolves
// that ID only from its committed Case/Attestation facts.
type DecisionCurrentResolveRequestV1 struct {
	TenantID     core.TenantID
	CaseID       string
	ExpectedCase ExpectedFactV1
}

// DecisionOwnerInputsV1 is one Review-Owner snapshot of the exact facts used
// by Verdict Decide. It deliberately excludes Policy/Authority/Scope/Binding
// and Evidence currentness, which remain independently owned and are read by
// DecisionExternalCurrentReaderV1. Durable Store implementations must produce
// this bundle from one committed tenant snapshot.
type DecisionOwnerInputsV1 struct {
	Target          contract.TargetSnapshotV1
	Case            contract.ReviewCaseV1
	Round           contract.ReviewRoundV1
	Rubric          contract.RubricDefinitionV1
	Assignment      contract.ReviewerAssignmentV1
	Attestation     contract.AttestationV1
	Findings        []contract.FindingV1
	ApplySettlement *contract.DomainApplySettlementFactV1
	DomainResult    *contract.ReviewerInvocationResultFactV1
	Evidence        []runtimeports.ReviewEvidenceRefV2
}

// DecisionCurrentReaderV1 must exact-Inspect all inputs and return one
// linearized immutable snapshot. It is read-only. There is intentionally no
// fallback to caller-supplied current values.
type DecisionCurrentReaderV1 interface {
	InspectDecisionCurrentV1(context.Context, DecisionCurrentRequestV1) (contract.DecisionCurrentSnapshotV1, error)
}

// DecisionExternalCurrentRequestV1 is assembled only from exact stored Review
// facts. It is the read-only request passed to public Owner adapters.
type DecisionExternalCurrentRequestV1 struct {
	Target      contract.TargetSnapshotV1
	Assignment  contract.ReviewerAssignmentV1
	Attestation contract.AttestationV1
	Evidence    []runtimeports.ReviewEvidenceRefV2
}

type DecisionExternalCurrentProjectionV1 struct {
	Policy            runtimeports.ReviewPolicyFactV2
	ActorAuthority    runtimeports.OperationGovernanceFactRefV3
	ReviewerAuthority runtimeports.OperationGovernanceFactRefV3
	Scope             runtimeports.OperationGovernanceFactRefV3
	Binding           contract.ReviewerBindingCurrentV1
	Evidence          []contract.DecisionEvidenceCurrentV1
	ExternalProof     *contract.DecisionExternalCurrentProofV1
	Current           bool
	ExpiresUnixNano   int64
}

// DecisionExternalCurrentReaderV1 is intentionally an adapter seam. No
// production implementation is supplied until Binding and Review-Evidence
// exact-current public contracts are jointly frozen.
type DecisionExternalCurrentReaderV1 interface {
	InspectDecisionExternalCurrentV1(context.Context, DecisionExternalCurrentRequestV1) (DecisionExternalCurrentProjectionV1, error)
}

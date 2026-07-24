package ports

import (
	"context"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// RestoreStageActionPortV1 is the existing host Action Gateway's dedicated
// Restore route. Execute is start-or-inspect and must perform Admission,
// Review/Authorization, Permit/Fence, Begin, Sandbox Prepare, actual-point
// Enforcement, Stage, Evidence, Runtime Settlement and Sandbox ApplySettlement.
// A retry with the same key must never call the Provider twice.
type RestoreStageActionPortV1 interface {
	ExecuteRestoreStageActionV1(context.Context, applicationcontract.RestoreStageActionRequestV1) (applicationcontract.RestoreStageActionResultV1, error)
	InspectRestoreStageActionV1(context.Context, applicationcontract.RestoreStageActionInspectKeyV1) (applicationcontract.RestoreStageActionResultV1, error)
}

// RestoreStageAuthorizationPortV1 is implemented by the existing host Action
// Gateway's Restore route. The returned value is already Admission -> Review
// Authorization -> Permit/Fence -> Begin current; this port never invokes the
// Sandbox Provider.
type RestoreStageAuthorizationPortV1 interface {
	AuthorizeRestoreStageV1(context.Context, applicationcontract.RestoreStageActionRequestV1) (applicationcontract.RestoreStageAuthorizedDispatchV1, error)
	InspectRestoreStageAuthorizationV1(context.Context, applicationcontract.RestoreStageActionInspectKeyV1) (applicationcontract.RestoreStageAuthorizedDispatchV1, error)
}

// RestoreStageAuthorizationInputCurrentReaderV1 is supplied by the trusted
// host Assembler. It returns a sealed immutable Intent and exact routing refs;
// caller candidates never become current merely by reaching Application.
type RestoreStageAuthorizationInputCurrentReaderV1 interface {
	InspectRestoreStageAuthorizationInputCurrentV1(context.Context, applicationcontract.RestoreStageActionInspectKeyV1) (applicationcontract.RestoreStageAuthorizationInputCurrentProjectionV1, error)
}

// RestoreStageParticipantPortV1 is the narrow cross-owner Sandbox seam. It
// owns Prepare, Stage/Inspect and ApplySettlement, but not Runtime Permit,
// Enforcement, Evidence, Settlement or Activation.
type RestoreStageParticipantPortV1 interface {
	PrepareRestoreStageV1(context.Context, applicationcontract.RestoreStageActionRequestV1, applicationcontract.RestoreStageAuthorizedDispatchV1) (runtimeports.RestoreStageSandboxCurrentProjectionV1, error)
	ExecuteRestoreStageV1(context.Context, applicationcontract.RestoreStageActionRequestV1, applicationcontract.RestoreStageAuthorizedDispatchV1, runtimeports.OperationDispatchEnforcementPhaseRefV4) (runtimeports.RestoreStageDomainResultCurrentProjectionV1, error)
	ApplyRestoreStageSettlementV1(context.Context, runtimeports.RestoreStageSettlementRefV1, runtimeports.RestoreStageDomainResultCurrentProjectionV1) (runtimeports.RestoreStageApplySettlementCurrentProjectionV1, error)
}

type RestoreStageEvidencePublisherV1 interface {
	PublishRestoreStageEvidenceV1(context.Context, applicationcontract.RestoreStageEvidenceRequestV1) (runtimeports.EvidenceRecordRefV2, error)
}

type RestoreStageActionResultFactPortV1 interface {
	CreateRestoreStageActionResultV1(context.Context, applicationcontract.RestoreStageActionResultFactV1) (applicationcontract.RestoreStageActionResultFactV1, error)
	InspectRestoreStageActionResultV1(context.Context, core.TenantID, string) (applicationcontract.RestoreStageActionResultFactV1, error)
}

type RestoreExecutionPortV1 interface {
	ExecuteRestoreV1(context.Context, applicationcontract.RestoreExecutionRequestV1) (applicationcontract.RestoreExecutionResultV1, error)
}

type RestoreExecutionIntentFactPortV1 interface {
	CreateRestoreExecutionIntentV1(context.Context, applicationcontract.RestoreExecutionIntentFactV1) (applicationcontract.RestoreExecutionIntentFactV1, error)
	InspectRestoreExecutionIntentV1(context.Context, core.TenantID, string) (applicationcontract.RestoreExecutionIntentFactV1, error)
}

type RestoreExecutionResultFactPortV1 interface {
	CreateRestoreExecutionResultV1(context.Context, applicationcontract.RestoreExecutionResultFactV1) (applicationcontract.RestoreExecutionResultFactV1, error)
	InspectRestoreExecutionResultV1(context.Context, core.TenantID, string) (applicationcontract.RestoreExecutionResultFactV1, error)
}

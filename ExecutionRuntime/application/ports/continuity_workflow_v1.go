package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
)

// ContinuityWorkflowAssemblerV1 is supplied by the trusted Application /
// Agent-Assembler composition root. Continuity and external callers cannot
// construct an authoritative SubmissionBundle themselves.
type ContinuityWorkflowAssemblerV1 interface {
	AssembleContinuityWorkflowV1(context.Context, contract.ContinuityWorkflowRequestV1) (contract.ContinuityWorkflowAssemblyV1, error)
}

// ContinuityWorkflowSubmissionGatewayV1 is the only public Application write
// seam exposed to Continuity CLI/API adapters.
type ContinuityWorkflowSubmissionGatewayV1 interface {
	SubmitContinuityWorkflowV1(context.Context, contract.ContinuityWorkflowRequestV1) (contract.ContinuityWorkflowInspectionV1, error)
	InspectContinuityWorkflowV1(context.Context, contract.ContinuityWorkflowRequestV1) (contract.ContinuityWorkflowInspectionV1, error)
}

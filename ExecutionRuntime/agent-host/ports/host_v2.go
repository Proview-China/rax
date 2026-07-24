package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// HostV2 is additive. HostV1 remains byte-for-byte and behaviorally separate.
type HostV2 interface {
	StartV2(context.Context, contract.StartRequestV2) (contract.StartResultV2, error)
	InspectV2(context.Context, contract.InspectRequestV2) (contract.InspectResultV2, error)
	StopV2(context.Context, contract.StopRequestV2) (contract.StopResultV2, error)
}

type CleanupPlanCurrentReaderV2 interface {
	InspectCleanupPlanV2(context.Context, contract.ExactRefV1) (contract.CleanupPlanV2, error)
}

// CleanupNodeOperationV2 is one exact cleanup endpoint. It is deliberately
// resolved from the plan's nominal InspectPortBinding, never from an Owner
// name or a generic lifecycle hook.
type CleanupNodeOperationV2 interface {
	StartOrInspectCleanupNodeV2(context.Context, contract.CleanupNodeRequestV2) (contract.CleanupNodeResultV2, error)
	InspectCleanupNodeV2(context.Context, contract.CleanupNodeRequestV2) (contract.CleanupNodeResultV2, error)
}

type CleanupNodeOperationRegistryV2 interface {
	ResolveCleanupNodeOperationV2(
		context.Context,
		contract.ExactRefV1,
		contract.ExactRefV1,
		contract.DigestV1,
		contract.DigestV1,
	) (CleanupNodeOperationV2, error)
}

// The first three operations are Host-local adapters over immutable Owner
// outputs. Production adapters must persist or resolve exact outputs so that
// Inspect never re-runs compilation after an uncertain reply.
type DefinitionOperationV2 interface {
	StartOrInspectDefinitionV2(context.Context, contract.StartRequestV2) (contract.DecodedDefinitionV1, error)
	InspectDefinitionV2(context.Context, contract.StartRequestV2) (contract.DecodedDefinitionV1, error)
}

type AssemblyPlanOperationV2 interface {
	StartOrInspectAssemblyPlanV2(context.Context, contract.StartRequestV2, contract.DecodedDefinitionV1) (contract.ResolvedAssemblyV1, error)
	InspectAssemblyPlanV2(context.Context, contract.StartRequestV2, contract.DecodedDefinitionV1) (contract.ResolvedAssemblyV1, error)
}

type HarnessCompileOperationV2 interface {
	StartOrInspectHarnessCompileV2(context.Context, contract.StartRequestV2, contract.ResolvedAssemblyV1) (contract.CompiledAssemblyArtifactsV2, error)
	InspectHarnessCompileV2(context.Context, contract.StartRequestV2, contract.ResolvedAssemblyV1) (contract.CompiledAssemblyArtifactsV2, error)
}

// AssemblyPublicationInspectorV2 is separate from the mutation surface so a
// Ready/restarted Host can reconstruct the exact publication without receiving
// a second publish capability.
type AssemblyPublicationInspectorV2 interface {
	InspectAssemblyPublicationV2(context.Context, contract.AssemblyPublicationRequestV2) (contract.AssemblyPublicationResultV2, error)
}

// HostV2StageInputsAssemblerV2 is a pure, injectable request-building seam.
// It may read Owner public current ports in its own adapter, but it cannot
// write Owner facts or substitute generic refs for nominal public requests.
type HostV2StageInputsAssemblerV2 interface {
	BuildAssemblyPublicationRequestV2(context.Context, contract.StartRequestV2, contract.CompiledAssemblyArtifactsV2) (contract.AssemblyPublicationRequestV2, error)
	BuildBindingAdmissionRequestV2(context.Context, contract.StartRequestV2, contract.DecodedDefinitionV1, contract.ResolvedAssemblyV1, contract.AssemblyPublicationResultV2) (runtimeports.BindingAdmissionRequestV1, error)
	BuildControlAdapterRequestsV2(context.Context, contract.StartRequestV2, contract.AssemblyPublicationResultV2, runtimeports.BindingAdmissionResultV1) ([]contract.ControlAdapterConstructRequestV2, error)
	BuildAgentActivationRequestV2(context.Context, contract.StartRequestV2, contract.AssemblyPublicationResultV2, runtimeports.BindingAdmissionResultV1, []contract.ControlAdapterInstanceV2) (applicationcontract.AgentActivationStartRequestV1, error)
	BuildGenerationAssociationCandidateV2(context.Context, contract.StartRequestV2, contract.AssemblyPublicationResultV2, runtimeports.BindingAdmissionResultV1, applicationcontract.AgentActivationResultV1) (runtimeports.GenerationBindingAssociationCandidateV1, error)
	BuildSystemReadyRequestV2(context.Context, contract.StartRequestV2, contract.HostStartClaimV1, contract.DecodedDefinitionV1, contract.ResolvedAssemblyV1, contract.AssemblyPublicationResultV2, runtimeports.BindingAdmissionResultV1, []contract.ControlAdapterInstanceV2, applicationcontract.AgentActivationResultV1, runtimeports.GenerationBindingAssociationFactV1) (contract.SystemReadyEnsureRequestV2, error)
}

// Compile-time aliases document the exact public Owner capabilities consumed
// by the reference HostV2 coordinator without widening their authority.
type BindingAdmissionGovernancePortV2 interface {
	runtimeports.BindingAdmissionGovernancePortV1
}

type AgentActivationGovernancePortV2 interface {
	applicationports.AgentActivationPortV1
}

type GenerationAssociationGovernancePortV2 interface {
	runtimeports.GenerationBindingAssociationGovernancePortV1
}

package ports

import (
	"context"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// Nominal methods prevent a generic OwnerCurrent projection from being used in
// another readiness position.
type SystemReadyCoreCurrentReadersV2 interface {
	InspectDefinitionCurrentV2(context.Context, runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error)
	InspectPlanCurrentV2(context.Context, runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error)
	InspectAssemblyCurrentV2(context.Context, runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error)
	InspectBindingSetCurrentV2(context.Context, runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error)
	InspectActivationCurrentV2(context.Context, runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error)
	InspectGenerationBindingCurrentV2(context.Context, runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error)
	InspectApplicationStartCurrentV2(context.Context, runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error)
	InspectSandboxLeaseCurrentV2(context.Context, runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error)
	InspectSandboxActiveCurrentV2(context.Context, runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error)
	InspectExecutionReadyCurrentV2(context.Context, runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error)
	InspectSupervisionPolicyCurrentV2(context.Context, runtimeports.OwnerCurrentRefV1) (contract.SystemReadySupervisionPolicyCurrentV2, error)
}
type ComponentProductionCurrentReaderV2 interface {
	InspectComponentProductionCurrentV2(context.Context, contract.ComponentProductionCurrentV2) (contract.ComponentProductionCurrentV2, error)
}
type ComponentProductionCurrentReaderRegistryV2 interface {
	ReaderForComponentProductionCurrentV2(runtimeports.NamespacedNameV2) (ComponentProductionCurrentReaderV2, error)
}
type SystemReadyGovernancePortV2 interface {
	StartOrInspectSystemReadyV2(context.Context, contract.SystemReadyEnsureRequestV2) (contract.SystemReadyGatewayResultV2, error)
	InspectSystemReadyV2(context.Context, contract.SystemReadyInspectRequestV2) (contract.SystemReadyGatewayResultV2, error)
}

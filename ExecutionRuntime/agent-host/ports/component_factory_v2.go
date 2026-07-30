package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
)

// ComponentFactoryV2 is the Owner-declared executable interface. Its descriptor
// and conformance metadata do not prove implementation source or provenance.
// Agent Host only invokes this interface after a successful preflight and never
// receives a raw provider, database, resource implementation or cleanup
// function.
type ComponentFactoryV2 interface {
	DescriptorV2() contract.ComponentFactoryDescriptorV2
	StartOrInspectComponentV2(context.Context, contract.ComponentStartRequestV2) (ComponentHandleV2, error)
	// Inspect accepts only the original exact Attempt sealed by the Start
	// request; altered coordinates fail contract validation.
	InspectComponentV2(context.Context, contract.ComponentFactoryAttemptRefV2) (ComponentHandleV2, error)
}

type ComponentHandleV2 interface {
	InstanceV2() contract.ComponentInstanceV2
	InspectBindingV2() contract.ExactRefV1
	CleanupBindingV2() contract.ExactRefV1
}

// ComponentFactoryConformanceCurrentReaderV2 is neutral. Each Component Owner
// publishes its own current; the Host can only inspect an exact Ref.
type ComponentFactoryConformanceCurrentReaderV2 interface {
	InspectComponentFactoryConformanceCurrentV2(
		context.Context,
		contract.ComponentFactoryConformanceCurrentRefV2,
	) (contract.ComponentFactoryConformanceCurrentV2, error)
}

type ComponentDependencyCurrentReaderV2 interface {
	InspectComponentDependencyCurrentV2(
		context.Context,
		contract.ComponentInstanceRefV2,
	) (contract.ComponentDependencyCurrentV2, error)
}

// ComponentFactoryRegistryReaderV2 deliberately exposes metadata only.
// Preflight therefore cannot call a factory or cross an effect boundary.
type ComponentFactoryRegistryReaderV2 interface {
	InspectComponentFactoryRegistrationV2(
		context.Context,
		contract.ComponentFactoryRegistryKeyV2,
	) (contract.ComponentFactoryRegistrationV2, error)
}

type ComponentFactoryRegistryV2 interface {
	ComponentFactoryRegistryReaderV2
	RegisterComponentFactoryV2(context.Context, ComponentFactoryV2, contract.ComponentFactoryConformanceCurrentV2) (contract.ComponentFactoryRegistrationV2, error)
	SealComponentFactoryRegistryV2(context.Context) error
	ResolveComponentFactoryV2(context.Context, contract.ComponentFactoryRegistryKeyV2) (ComponentFactoryV2, error)
}

type ComponentFactoryPreflightV2 interface {
	PreflightComponentFactoryV2(
		context.Context,
		contract.ComponentFactoryPreflightRequestV2,
	) (contract.ComponentFactoryPreflightReceiptV2, error)
}

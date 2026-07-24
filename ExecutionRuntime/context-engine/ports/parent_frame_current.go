package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

type ContextParentFrameSourceBindingReaderV1 interface {
	ResolveExactSourceBinding(context.Context, contract.ContextParentFrameApplicabilitySourceCoordinateV1) (contract.ContextParentFrameSourceBindingV1, error)
}

type ContextFrameMetadataReaderV1 interface {
	FrameByExactRef(context.Context, contract.FactRef, contract.Digest) (contract.ContextFrame, error)
}

type ContextManifestMetadataReaderV1 interface {
	ManifestByExactRef(context.Context, contract.FactRef, contract.Digest) (contract.ContextManifest, error)
}

type ContextGenerationMetadataReaderV1 interface {
	GenerationByExactRef(context.Context, contract.FactRef, contract.Digest) (contract.ContextGeneration, error)
}

type ContextGenerationCurrentPointerReaderV1 interface {
	InspectCurrentGenerationPointer(context.Context, contract.ContextGenerationCurrentPointerRequestV1) (contract.ContextGenerationCurrentPointerV1, error)
}

// ContextParentFrameCurrentReaderV1 accepts only the nominal four-field source
// coordinate. Exact metadata is resolved and reread inside the Context Owner.
type ContextParentFrameCurrentReaderV1 interface {
	InspectContextParentFrameCurrentV1(context.Context, contract.ContextParentFrameApplicabilitySourceCoordinateV1) (contract.ContextParentFrameCurrentProjectionV1, error)
}

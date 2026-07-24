package sdk

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/consolidation"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/knowledge"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/observability"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/projection"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/retrieval"
)

// ChannelRetriever returns provider observations only. The Owner/Context path
// still validates, filters, merges, and materializes exact refs.
type ChannelRetriever interface {
	RetrieveChannel(context.Context, retrieval.HybridRequestV1, retrieval.ChannelBudgetV1) (retrieval.ChannelObservationV1, error)
}

// Indexer produces a build observation, never an authoritative Projection or
// IndexDescriptor. Only the relevant Owner may inspect and publish those.
type Indexer interface {
	BuildIndex(context.Context, projection.BuildRequestV1) (projection.BuildObservationV1, error)
}

// Consolidator produces proposal material. It cannot Commit Memory facts.
type Consolidator interface {
	Consolidate(context.Context, consolidation.InputV1) (consolidation.BatchV1, error)
}

// AdmissionPolicyAdapter returns non-authoritative advice. The Memory or
// Knowledge Owner still creates and CASes the formal Admission fact.
type AdmissionPolicyAdapter interface {
	AdviseAdmission(context.Context, contract.AdmissionPolicyRequestV1) (contract.AdmissionAdviceV1, error)
}

// TelemetrySink accepts bounded diagnostic observations. It must not turn a
// metric into a Memory/Knowledge fact or task outcome.
type TelemetrySink interface {
	ObserveMetrics(context.Context, observability.SnapshotV1) error
}

// SourceConnector performs only the already-governed acquire attempt and can
// inspect that same attempt after a lost reply. Its output is never a Source.
type SourceConnector interface {
	Acquire(context.Context, knowledge.AcquireRequestV1) (knowledge.AcquireObservationV1, error)
	InspectAcquire(context.Context, contract.Ref, contract.Ref) (knowledge.AcquireInspectionV1, *knowledge.AcquireObservationV1, error)
}

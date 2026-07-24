// Package conformance validates custom Memory/Knowledge extension providers.
// It deliberately tests Observation/proposal boundaries, not production SLA.
package conformance

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/consolidation"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/knowledge"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/observability"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/projection"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/retrieval"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/sdk"
)

func CheckChannelRetrieverV1(ctx context.Context, now time.Time, provider sdk.ChannelRetriever, request retrieval.HybridRequestV1, budget retrieval.ChannelBudgetV1) error {
	if provider == nil || request.Validate() != nil {
		return contract.ErrInvalidArgument
	}
	first, err := provider.RetrieveChannel(ctx, request, budget)
	if err != nil {
		return err
	}
	if err := first.Validate(now); err != nil || first.Kind != budget.Kind || !contract.SameRef(first.ViewRef, request.Query.ViewRef) || len(first.Hits) > budget.Limit {
		return fmt.Errorf("%w: retriever observation", contract.ErrEvidenceConflict)
	}
	second, err := provider.RetrieveChannel(ctx, request, budget)
	if err != nil || second.Digest != first.Digest {
		return fmt.Errorf("%w: nondeterministic retriever", contract.ErrEvidenceConflict)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := provider.RetrieveChannel(cancelled, request, budget); !errors.Is(err, context.Canceled) {
		return fmt.Errorf("%w: retriever ignored cancellation", contract.ErrInvalidArgument)
	}
	return nil
}

func CheckIndexerV1(ctx context.Context, now time.Time, provider sdk.Indexer, request projection.BuildRequestV1) error {
	if provider == nil || request.Validate(now) != nil {
		return contract.ErrInvalidArgument
	}
	first, err := provider.BuildIndex(ctx, request)
	if err != nil {
		return err
	}
	if err := first.Validate(now); err != nil || first.Owner != request.Owner || first.Kind != request.Kind || !contract.SameRef(first.ViewRef, request.ViewRef) || !contract.SameRef(first.BoundaryRef, request.BoundaryRef) || first.BuilderVersion != request.BuilderVersion || first.IndexVersion != request.IndexVersion {
		return fmt.Errorf("%w: index observation drift", contract.ErrEvidenceConflict)
	}
	second, err := provider.BuildIndex(ctx, request)
	if err != nil || second.Digest != first.Digest {
		return fmt.Errorf("%w: nondeterministic indexer", contract.ErrEvidenceConflict)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := provider.BuildIndex(cancelled, request); !errors.Is(err, context.Canceled) {
		return fmt.Errorf("%w: indexer ignored cancellation", contract.ErrInvalidArgument)
	}
	return nil
}

func CheckConsolidatorV1(ctx context.Context, now time.Time, provider sdk.Consolidator, input consolidation.InputV1) error {
	if provider == nil || input.Validate(now) != nil {
		return contract.ErrInvalidArgument
	}
	first, err := provider.Consolidate(ctx, input)
	if err != nil {
		return err
	}
	if err := first.Validate(now); err != nil || !contract.SameRef(first.InputRef, input.Ref) || first.Owner != contract.OwnerMemory {
		return fmt.Errorf("%w: consolidation proposal drift", contract.ErrEvidenceConflict)
	}
	second, err := provider.Consolidate(ctx, input)
	if err != nil || second.Digest != first.Digest {
		return fmt.Errorf("%w: nondeterministic consolidator", contract.ErrEvidenceConflict)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := provider.Consolidate(cancelled, input); !errors.Is(err, context.Canceled) {
		return fmt.Errorf("%w: consolidator ignored cancellation", contract.ErrInvalidArgument)
	}
	return nil
}

func CheckAdmissionPolicyAdapterV1(ctx context.Context, now time.Time, provider sdk.AdmissionPolicyAdapter, request contract.AdmissionPolicyRequestV1) error {
	if provider == nil || request.Validate(now) != nil {
		return contract.ErrInvalidArgument
	}
	first, err := provider.AdviseAdmission(ctx, request)
	if err != nil {
		return err
	}
	if err := first.Validate(now); err != nil || first.RequestDigest != request.Digest {
		return fmt.Errorf("%w: admission advice binding", contract.ErrEvidenceConflict)
	}
	second, err := provider.AdviseAdmission(ctx, request)
	if err != nil || second.Digest != first.Digest {
		return fmt.Errorf("%w: nondeterministic admission advice", contract.ErrEvidenceConflict)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := provider.AdviseAdmission(cancelled, request); !errors.Is(err, context.Canceled) {
		return fmt.Errorf("%w: admission policy ignored cancellation", contract.ErrInvalidArgument)
	}
	return nil
}

func CheckTelemetrySinkV1(ctx context.Context, now time.Time, sink sdk.TelemetrySink, snapshot observability.SnapshotV1) error {
	if sink == nil || snapshot.Validate(now) != nil {
		return contract.ErrInvalidArgument
	}
	if err := sink.ObserveMetrics(ctx, snapshot); err != nil {
		return err
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := sink.ObserveMetrics(cancelled, snapshot); !errors.Is(err, context.Canceled) {
		return fmt.Errorf("%w: telemetry sink ignored cancellation", contract.ErrInvalidArgument)
	}
	return nil
}

func CheckSourceConnectorV1(ctx context.Context, now time.Time, connector sdk.SourceConnector, request knowledge.AcquireRequestV1) error {
	if connector == nil || request.Validate(now) != nil {
		return contract.ErrInvalidArgument
	}
	observation, err := connector.Acquire(ctx, request)
	if err != nil {
		return err
	}
	if err := observation.Validate(now); err != nil || !contract.SameRef(observation.RequestRef, request.Ref) || !contract.SameRef(observation.AttemptRef, request.AttemptRef) {
		return fmt.Errorf("%w: connector observation binding", contract.ErrEvidenceConflict)
	}
	inspection, inspected, err := connector.InspectAcquire(ctx, request.Ref, request.AttemptRef)
	if err != nil {
		return err
	}
	if err := inspection.Validate(now); err != nil || !contract.SameRef(inspection.RequestRef, request.Ref) || !contract.SameRef(inspection.AttemptRef, request.AttemptRef) || inspection.Outcome != knowledge.AcquireObserved || inspected == nil || !contract.SameRef(inspection.ObservationRef, observation.Ref) || !contract.SameRef(inspected.Ref, observation.Ref) {
		return fmt.Errorf("%w: connector inspect binding", contract.ErrEvidenceConflict)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := connector.Acquire(cancelled, request); !errors.Is(err, context.Canceled) {
		return fmt.Errorf("%w: connector ignored cancellation", contract.ErrInvalidArgument)
	}
	return nil
}

package assemblypublication

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type PublisherV2 struct {
	compiler CompilerV1
	store    OwnerStoreV2
	clock    func() time.Time
}

var _ CompileAndPublisherV2 = (*PublisherV2)(nil)
var _ HistoricalReaderV2 = (*PublisherV2)(nil)
var _ CurrentReaderV2 = (*PublisherV2)(nil)

func NewPublisherV2(compiler CompilerV1, store OwnerStoreV2, clock func() time.Time) (*PublisherV2, error) {
	if nilLike(compiler) || nilLike(store) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Assembly Publication V2 compiler, owner store and clock are required")
	}
	return &PublisherV2{compiler: compiler, store: store, clock: clock}, nil
}

func (p *PublisherV2) CompileAndPublishAssemblyV2(ctx context.Context, request assemblycontract.CompileAndPublishAssemblyRequestV2) (assemblycontract.CompileAndPublishAssemblyResultV2, error) {
	if p == nil || nilLike(p.compiler) || nilLike(p.store) || p.clock == nil || ctx == nil {
		return assemblycontract.CompileAndPublishAssemblyResultV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Assembly Publication V2 publisher is incomplete")
	}
	if err := validateRequestV2(request); err != nil {
		return assemblycontract.CompileAndPublishAssemblyResultV2{}, err
	}
	now := p.clock()
	if now.IsZero() || request.RequestedExpiresUnixNano <= now.UnixNano() {
		return assemblycontract.CompileAndPublishAssemblyResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Assembly publication requested current window is unavailable")
	}
	predecessor, predecessorExists, err := p.inspectExpected(ctx, request.Input.ScopeRef, request.ExpectedCurrent)
	if err != nil {
		return assemblycontract.CompileAndPublishAssemblyResultV2{}, err
	}
	if err := validatePreviousGenerationV2(request.Input, predecessor, predecessorExists); err != nil {
		return assemblycontract.CompileAndPublishAssemblyResultV2{}, err
	}

	compiled, err := p.compiler.Compile(request.Input)
	if err != nil {
		return assemblycontract.CompileAndPublishAssemblyResultV2{}, err
	}
	bundle, err := assemblycontract.NewAssemblyPublicationBundleV2(request.Input.ScopeRef, compiled)
	if err != nil {
		return assemblycontract.CompileAndPublishAssemblyResultV2{}, err
	}
	nextRevision := core.Revision(1)
	if predecessorExists {
		nextRevision = predecessor.Revision + 1
		if nextRevision <= predecessor.Revision {
			return assemblycontract.CompileAndPublishAssemblyResultV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Assembly publication current revision overflowed")
		}
	}
	current, err := assemblycontract.NewAssemblyPublicationCurrentV2(bundle, request.AttemptID, nextRevision, now, request.RequestedExpiresUnixNano)
	if err != nil {
		return assemblycontract.CompileAndPublishAssemblyResultV2{}, err
	}

	recovered := false
	stages := []struct {
		write func() error
		match func(StagedPublicationInspectionV2) bool
	}{
		{func() error {
			return p.store.StageGenerationV2(ctx, bundle.Publication.PublicationID, bundle.Generation)
		}, func(value StagedPublicationInspectionV2) bool {
			return value.GenerationDigest == bundle.Generation.Digest
		}},
		{func() error { return p.store.StageManifestV2(ctx, bundle.Publication.PublicationID, bundle.Manifest) }, func(value StagedPublicationInspectionV2) bool { return value.ManifestDigest == bundle.Manifest.Digest }},
		{func() error { return p.store.StageGraphV2(ctx, bundle.Publication.PublicationID, bundle.Graph) }, func(value StagedPublicationInspectionV2) bool { return value.GraphDigest == bundle.Graph.Digest }},
		{func() error { return p.store.StageHandoffV2(ctx, bundle.Publication.PublicationID, bundle.Handoff) }, func(value StagedPublicationInspectionV2) bool { return value.HandoffDigest == bundle.Handoff.Digest }},
	}
	for _, stage := range stages {
		if stageErr := stage.write(); stageErr != nil {
			if !recoverable(stageErr) {
				return assemblycontract.CompileAndPublishAssemblyResultV2{}, stageErr
			}
			inspection, inspectErr := p.store.InspectStagedPublicationV2(context.WithoutCancel(ctx), bundle.Publication.PublicationID)
			if inspectErr != nil || !stage.match(inspection) {
				return assemblycontract.CompileAndPublishAssemblyResultV2{}, stageErr
			}
			recovered = true
		}
	}

	committed, commitErr := p.store.CommitPublicationCurrentV2(ctx, CommitPublicationCurrentRequestV2{Expected: request.ExpectedCurrent, Bundle: bundle, Current: current})
	if commitErr != nil {
		if !recoverable(commitErr) {
			return assemblycontract.CompileAndPublishAssemblyResultV2{}, commitErr
		}
		recoveredCurrent, recoveryErr := p.store.InspectCommittedPublicationCurrentV2(context.WithoutCancel(ctx), assemblycontract.AssemblyPublicationRefV2{PublicationID: bundle.Publication.PublicationID, Revision: bundle.Publication.Revision, Digest: bundle.Publication.Digest})
		if recoveryErr != nil || recoveredCurrent.Digest != current.Digest || recoveredCurrent.CommitAttemptID != request.AttemptID {
			return assemblycontract.CompileAndPublishAssemblyResultV2{}, commitErr
		}
		committed = recoveredCurrent
		recovered = true
	}
	if committed.Digest != current.Digest || committed.CommitAttemptID != request.AttemptID {
		return assemblycontract.CompileAndPublishAssemblyResultV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "publication commit returned a different current")
	}
	return p.finish(ctx, bundle, committed, recovered)
}

func (p *PublisherV2) EnsureAssemblyPublicationV2(ctx context.Context, request assemblycontract.CompileAndPublishAssemblyRequestV2) (assemblycontract.CompileAndPublishAssemblyResultV2, error) {
	return p.CompileAndPublishAssemblyV2(ctx, request)
}

func (p *PublisherV2) InspectAssemblyPublicationHistoricalV2(ctx context.Context, ref assemblycontract.AssemblyPublicationRefV2) (assemblycontract.AssemblyPublicationBundleV2, error) {
	if p == nil || nilLike(p.store) || ctx == nil {
		return assemblycontract.AssemblyPublicationBundleV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Assembly publication historical reader is incomplete")
	}
	bundle, err := p.store.InspectHistoricalPublicationV2(ctx, ref)
	if err != nil {
		return assemblycontract.AssemblyPublicationBundleV2{}, err
	}
	if err := bundle.Validate(); err != nil {
		return assemblycontract.AssemblyPublicationBundleV2{}, err
	}
	return clone(bundle), nil
}

func (p *PublisherV2) InspectAssemblyPublicationCurrentV2(ctx context.Context, scopeRef string) (assemblycontract.AssemblyPublicationCurrentV2, error) {
	if p == nil || nilLike(p.store) || p.clock == nil || ctx == nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Assembly publication current reader is incomplete")
	}
	current, err := p.store.InspectCurrentPublicationV2(ctx, scopeRef)
	if err != nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, err
	}
	if err := current.ValidateAt(p.clock()); err != nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, err
	}
	return clone(current), nil
}

func (p *PublisherV2) finish(ctx context.Context, expected assemblycontract.AssemblyPublicationBundleV2, current assemblycontract.AssemblyPublicationCurrentV2, recovered bool) (assemblycontract.CompileAndPublishAssemblyResultV2, error) {
	publicationRef := assemblycontract.AssemblyPublicationRefV2{PublicationID: expected.Publication.PublicationID, Revision: expected.Publication.Revision, Digest: expected.Publication.Digest}
	observed, err := p.store.InspectCommittedPublicationCurrentV2(context.WithoutCancel(ctx), publicationRef)
	if err != nil {
		return assemblycontract.CompileAndPublishAssemblyResultV2{}, err
	}
	if err := observed.ValidateAt(p.clock()); err != nil {
		return assemblycontract.CompileAndPublishAssemblyResultV2{}, err
	}
	if observed.Digest != current.Digest || observed.Publication != publicationRef {
		return assemblycontract.CompileAndPublishAssemblyResultV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "post-commit publication current drifted")
	}
	historical, err := p.InspectAssemblyPublicationHistoricalV2(context.WithoutCancel(ctx), publicationRef)
	if err != nil {
		return assemblycontract.CompileAndPublishAssemblyResultV2{}, err
	}
	if historical.Publication.Digest != expected.Publication.Digest {
		return assemblycontract.CompileAndPublishAssemblyResultV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "post-commit historical publication drifted")
	}
	return assemblycontract.CompileAndPublishAssemblyResultV2{Publication: clone(historical.Publication), Current: clone(observed), RecoveredByInspect: recovered}, nil
}

func (p *PublisherV2) inspectExpected(ctx context.Context, scopeRef string, expected assemblycontract.AssemblyPublicationCurrentExpectationV2) (assemblycontract.AssemblyPublicationCurrentV2, bool, error) {
	current, err := p.store.InspectCurrentPublicationV2(ctx, scopeRef)
	if err != nil {
		if core.HasCategory(err, core.ErrorNotFound) && !expected.Exists {
			return assemblycontract.AssemblyPublicationCurrentV2{}, false, nil
		}
		if core.HasCategory(err, core.ErrorNotFound) {
			return assemblycontract.AssemblyPublicationCurrentV2{}, false, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "expected Assembly publication current is absent")
		}
		return assemblycontract.AssemblyPublicationCurrentV2{}, false, err
	}
	if !expected.Exists || current.Revision != expected.Revision || current.Digest != expected.Digest {
		return assemblycontract.AssemblyPublicationCurrentV2{}, false, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Assembly publication current predecessor mismatch")
	}
	// Validate integrity at the original observation point; an expired current
	// remains an exact legal predecessor for a fresh successor.
	if err := current.ValidateAt(time.Unix(0, current.CheckedUnixNano)); err != nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, false, err
	}
	return current, true, nil
}

func validateRequestV2(request assemblycontract.CompileAndPublishAssemblyRequestV2) error {
	return request.Validate()
}

func validatePreviousGenerationV2(input assemblycontract.AssemblyInputV1, current assemblycontract.AssemblyPublicationCurrentV2, exists bool) error {
	if !exists {
		if input.PreviousGenerationRef != nil {
			return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "initial Assembly publication cannot name a previous Generation")
		}
		return nil
	}
	if input.PreviousGenerationRef == nil || *input.PreviousGenerationRef != current.Artifacts.Generation {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Assembly input previous Generation is not the exact current predecessor")
	}
	return nil
}

func recoverable(err error) bool {
	return core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate)
}

func nilLike(value any) bool {
	if value == nil {
		return true
	}
	ref := reflect.ValueOf(value)
	switch ref.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return ref.IsNil()
	default:
		return false
	}
}

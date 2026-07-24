package modelinvokeradapter

import (
	"context"
	"reflect"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

// PreparedModelInvocationSurfaceCommitGateV1 is the production concrete
// Harness/host implementation of Model's two-method CommitGate. It owns only
// the ordering and the Model ACK repository; Tool owns Binding creation.
type PreparedModelInvocationSurfaceCommitGateV1 struct {
	assemblyRef runtimeports.ModelPreDispatchAssemblyCurrentRefV1
	gateRef     modelinvoker.PreparedModelInvocationGateImplementationRefV1
	prepared    modelinvoker.PreparedModelInvocationReaderV1
	current     modelinvoker.PreparedModelInvocationCurrentReaderV1
	assemblies  runtimeports.ModelPreDispatchAssemblyCurrentReaderV1
	surfaces    toolcontract.ToolSurfaceManifestCurrentReaderV1
	bindings    toolcontract.ToolSurfaceInvocationBindingRepositoryV1
	acks        *InMemoryPreparedModelInvocationAckRepositoryV1
	clock       func() time.Time
}

func NewPreparedModelInvocationSurfaceCommitGateV1(
	assemblyRef runtimeports.ModelPreDispatchAssemblyCurrentRefV1,
	gateRef modelinvoker.PreparedModelInvocationGateImplementationRefV1,
	prepared modelinvoker.PreparedModelInvocationReaderV1,
	current modelinvoker.PreparedModelInvocationCurrentReaderV1,
	assemblies runtimeports.ModelPreDispatchAssemblyCurrentReaderV1,
	surfaces toolcontract.ToolSurfaceManifestCurrentReaderV1,
	bindings toolcontract.ToolSurfaceInvocationBindingRepositoryV1,
	acks *InMemoryPreparedModelInvocationAckRepositoryV1,
	clock func() time.Time,
) (*PreparedModelInvocationSurfaceCommitGateV1, error) {
	if err := assemblyRef.Validate(); err != nil {
		return nil, err
	}
	if err := gateRef.Validate(); err != nil {
		return nil, err
	}
	for _, dependency := range []any{prepared, current, assemblies, surfaces, bindings, acks} {
		if nilLikeSurfaceCommitGateV1(dependency) {
			return nil, surfaceCommitGateUnavailableV1("Surface CommitGate dependency is unavailable")
		}
	}
	if clock == nil {
		return nil, surfaceCommitGateUnavailableV1("Surface CommitGate clock is unavailable")
	}
	return &PreparedModelInvocationSurfaceCommitGateV1{
		assemblyRef: assemblyRef,
		gateRef:     gateRef,
		prepared:    prepared,
		current:     current,
		assemblies:  assemblies,
		surfaces:    surfaces,
		bindings:    bindings,
		acks:        acks,
		clock:       clock,
	}, nil
}

func (g *PreparedModelInvocationSurfaceCommitGateV1) Commit(
	ctx context.Context,
	preparedRef modelinvoker.PreparedModelInvocationRefV1,
	currentRef modelinvoker.PreparedModelInvocationCurrentRefV1,
) (modelinvoker.PreparedModelInvocationCommitAckV1, error) {
	if err := g.preflightV1(ctx, preparedRef, currentRef); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}

	// The ACK repository is always the first Owner call. A stored winner is
	// recovered before taking a new clock sample or touching Tool.
	stored, storedErr := g.acks.inspectByPreparedCurrent(ctx, preparedRef, currentRef)
	if storedErr == nil {
		projection, err := g.current.InspectExactPreparedModelInvocationCurrentV1(ctx, currentRef)
		if err != nil {
			return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
		}
		now, err := freshSurfaceCommitGateTimeV1(g.clock, time.Time{})
		if err != nil {
			return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
		}
		if stored.PreparedRef != preparedRef || stored.CurrentRef != currentRef {
			return modelinvoker.PreparedModelInvocationCommitAckV1{}, surfaceCommitGateConflictV1("stored Model ACK lineage drifted")
		}
		if err := stored.ValidateCurrent(projection, now); err != nil {
			return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
		}
		return stored.Clone(), nil
	}
	if !core.HasCategory(storedErr, core.ErrorNotFound) {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, storedErr
	}

	nowS1, err := freshSurfaceCommitGateTimeV1(g.clock, time.Time{})
	if err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	s1, err := g.inspectOwnerCurrentV1(ctx, preparedRef, currentRef, nowS1)
	if err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	request := toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1{
		Invocation: toolcontract.ToolSurfaceInvocationCoordinateV1{
			InvocationID: preparedRef.InvocationID, InvocationDigest: preparedRef.InvocationDigest,
		},
		PreparedFactRef:           preparedRef,
		PreparedHistoricalFact:    s1.fact,
		PreparedCurrentRef:        currentRef,
		PreparedCurrent:           s1.current,
		SurfaceCurrent:            s1.surface,
		AssemblyCurrentRef:        g.assemblyRef,
		AssemblyRegistrySnapshot:  s1.assembly.RegistrySnapshot,
		AssemblyCurrent:           s1.assembly,
		RequestedNotAfterUnixNano: requestedNotAfterSurfaceCommitGateV1(ctx, s1),
	}
	if err := request.Validate(); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}

	binding, toolAck, readErr := g.bindings.InspectToolSurfaceInvocationBindingByInvocationV1(ctx, request.Invocation)
	if readErr != nil {
		if !core.HasCategory(readErr, core.ErrorNotFound) {
			return modelinvoker.PreparedModelInvocationCommitAckV1{}, readErr
		}
		binding, toolAck, err = g.bindings.EnsureToolSurfaceInvocationBindingV1(ctx, request)
		if err != nil {
			if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorIndeterminate) {
				return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
			}
			binding, toolAck, readErr = g.bindings.InspectToolSurfaceInvocationBindingByInvocationV1(ctx, request.Invocation)
			if readErr != nil {
				return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
			}
		}
	}
	if err := request.ValidateAgainst(binding); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	if err := toolAck.ValidateAgainst(binding, nowS1); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}

	nowS2, err := freshSurfaceCommitGateTimeV1(g.clock, nowS1)
	if err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	s2, err := g.inspectOwnerCurrentV1(ctx, preparedRef, currentRef, nowS2)
	if err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	if !reflect.DeepEqual(s1, s2) {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, surfaceCommitGateConflictV1("Surface CommitGate Owner current changed between S1 and S2")
	}

	nowAck, err := freshSurfaceCommitGateTimeV1(g.clock, nowS2)
	if err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	if err := binding.ValidateCurrent(nowAck); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	if err := toolAck.ValidateAgainst(binding, nowAck); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	surfaceRef := modelinvoker.PreparedModelInvocationSurfaceBindingRefV1{
		Owner: binding.Ref.Owner, ContractVersion: binding.Ref.ContractVersion,
		ID: binding.Ref.ID, Revision: binding.Ref.Revision, Digest: binding.Ref.Digest,
	}
	expires := minSurfaceCommitGateV1(currentRef.ExpiresUnixNano, s2.assembly.ExpiresUnixNano, s2.surface.ExpiresUnixNano, binding.NotAfterUnixNano, toolAck.NotAfterUnixNano)
	if expires <= nowAck.UnixNano() || expires > currentRef.NotAfterUnixNano {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, surfaceCommitGateExpiredV1("Surface CommitGate common window expired")
	}
	sealed, err := modelinvoker.SealPreparedModelInvocationCommitAckV1(modelinvoker.PreparedModelInvocationCommitAckV1{
		PreparedRef:           preparedRef,
		CurrentRef:            currentRef,
		GateImplementationRef: g.gateRef,
		SurfaceBindingRef:     surfaceRef,
		CheckedUnixNano:       nowAck.UnixNano(),
		ExpiresUnixNano:       expires,
		NotAfterUnixNano:      currentRef.NotAfterUnixNano,
	})
	if err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	stored, ensureErr := g.acks.EnsureAck(ctx, sealed)
	if ensureErr != nil {
		if !core.HasCategory(ensureErr, core.ErrorUnavailable) && !core.HasCategory(ensureErr, core.ErrorIndeterminate) {
			return modelinvoker.PreparedModelInvocationCommitAckV1{}, ensureErr
		}
		stored, err = g.acks.inspectByPreparedCurrent(ctx, preparedRef, currentRef)
		if err != nil {
			return modelinvoker.PreparedModelInvocationCommitAckV1{}, ensureErr
		}
	}
	if stored != sealed {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, surfaceCommitGateConflictV1("Model ACK Repository returned another canonical ACK")
	}
	finalNow, err := freshSurfaceCommitGateTimeV1(g.clock, nowAck)
	if err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	if err := stored.ValidateCurrent(s2.current, finalNow); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	return stored.Clone(), nil
}

func (g *PreparedModelInvocationSurfaceCommitGateV1) InspectExactAck(ctx context.Context, ref modelinvoker.PreparedModelInvocationCommitAckRefV1) (modelinvoker.PreparedModelInvocationCommitAckV1, error) {
	if g == nil || g.acks == nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, surfaceCommitGateUnavailableV1("Surface CommitGate ACK Repository is unavailable")
	}
	if err := surfaceCommitGateContextErrorV1(ctx); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	return g.acks.InspectExactAck(ctx, ref)
}

type surfaceCommitGateOwnerSnapshotV1 struct {
	fact     modelinvoker.PreparedModelInvocationFactV1
	current  modelinvoker.PreparedModelInvocationCurrentProjectionV1
	assembly runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1
	surface  toolcontract.ToolSurfaceManifestCurrentProjectionV1
}

func (g *PreparedModelInvocationSurfaceCommitGateV1) inspectOwnerCurrentV1(ctx context.Context, preparedRef modelinvoker.PreparedModelInvocationRefV1, currentRef modelinvoker.PreparedModelInvocationCurrentRefV1, now time.Time) (surfaceCommitGateOwnerSnapshotV1, error) {
	fact, err := g.prepared.InspectExactPreparedModelInvocationV1(ctx, preparedRef)
	if err != nil {
		return surfaceCommitGateOwnerSnapshotV1{}, err
	}
	if err := fact.Validate(); err != nil || fact.Ref() != preparedRef {
		if err != nil {
			return surfaceCommitGateOwnerSnapshotV1{}, err
		}
		return surfaceCommitGateOwnerSnapshotV1{}, surfaceCommitGateConflictV1("Prepared Historical exact Ref drifted")
	}
	current, err := g.current.InspectExactPreparedModelInvocationCurrentV1(ctx, currentRef)
	if err != nil {
		return surfaceCommitGateOwnerSnapshotV1{}, err
	}
	if err := current.ValidateAgainstFact(fact); err != nil {
		return surfaceCommitGateOwnerSnapshotV1{}, err
	}
	if err := current.ValidateCurrent(currentRef, now); err != nil {
		return surfaceCommitGateOwnerSnapshotV1{}, err
	}
	assembly, err := g.assemblies.InspectCurrentModelPreDispatchAssemblyV1(ctx, g.assemblyRef)
	if err != nil {
		return surfaceCommitGateOwnerSnapshotV1{}, err
	}
	if err := assembly.ValidateCurrent(g.assemblyRef, now); err != nil {
		return surfaceCommitGateOwnerSnapshotV1{}, err
	}
	surfaceRef := toolcontract.ToolSurfaceManifestCurrentRefV1{
		ContractVersion: toolcontract.ToolSurfaceManifestCurrentContractVersionV1,
		ID:              assembly.ToolSurface.ID, Revision: assembly.ToolSurface.Revision, Digest: assembly.ToolSurface.Digest,
	}
	surface, err := g.surfaces.InspectExactToolSurfaceManifestCurrentV1(ctx, surfaceRef)
	if err != nil {
		return surfaceCommitGateOwnerSnapshotV1{}, err
	}
	if err := surface.ValidateCurrent(surfaceRef, now); err != nil {
		return surfaceCommitGateOwnerSnapshotV1{}, err
	}
	if assembly.ProfileDigest != fact.ProfileDigest || surface.Manifest.ProfileDigest != fact.ProfileDigest ||
		assembly.RegistrySnapshot != fact.RegistrySnapshotRef || surface.Manifest.RegistrySnapshotDigest != fact.RegistrySnapshotRef.Digest ||
		surface.Manifest.ExpectedInjectionDigest != fact.ActualToolSurfaceDigest || current.ActualToolSurfaceDigest != fact.ActualToolSurfaceDigest ||
		current.ActualProviderInjectionDigest != fact.ActualProviderInjectionDigest || current.NotAfterUnixNano != fact.NotAfterUnixNano {
		return surfaceCommitGateOwnerSnapshotV1{}, surfaceCommitGateConflictV1("Prepared, Assembly, Surface or Registry Owner current lineage drifted")
	}
	return surfaceCommitGateOwnerSnapshotV1{fact: fact, current: current, assembly: assembly, surface: surface}, nil
}

func (g *PreparedModelInvocationSurfaceCommitGateV1) preflightV1(ctx context.Context, preparedRef modelinvoker.PreparedModelInvocationRefV1, currentRef modelinvoker.PreparedModelInvocationCurrentRefV1) error {
	if g == nil || nilLikeSurfaceCommitGateV1(g.prepared) || nilLikeSurfaceCommitGateV1(g.current) || nilLikeSurfaceCommitGateV1(g.assemblies) || nilLikeSurfaceCommitGateV1(g.surfaces) || nilLikeSurfaceCommitGateV1(g.bindings) || g.acks == nil || g.clock == nil {
		return surfaceCommitGateUnavailableV1("Surface CommitGate is unavailable")
	}
	if err := surfaceCommitGateContextErrorV1(ctx); err != nil {
		return err
	}
	if err := preparedRef.Validate(); err != nil {
		return err
	}
	if err := currentRef.Validate(); err != nil {
		return err
	}
	if currentRef.Prepared != preparedRef {
		return surfaceCommitGateConflictV1("Prepared and Current Gate inputs drifted")
	}
	return nil
}

func requestedNotAfterSurfaceCommitGateV1(ctx context.Context, snapshot surfaceCommitGateOwnerSnapshotV1) int64 {
	result := minSurfaceCommitGateV1(snapshot.fact.NotAfterUnixNano, snapshot.current.ExpiresUnixNano, snapshot.assembly.ExpiresUnixNano, snapshot.surface.ExpiresUnixNano)
	if deadline, ok := ctx.Deadline(); ok && deadline.UnixNano() < result {
		result = deadline.UnixNano()
	}
	return result
}

func freshSurfaceCommitGateTimeV1(clock func() time.Time, previous time.Time) (time.Time, error) {
	if clock == nil {
		return time.Time{}, surfaceCommitGateUnavailableV1("Surface CommitGate clock is unavailable")
	}
	now := clock()
	if now.IsZero() {
		return time.Time{}, surfaceCommitGateUnavailableV1("Surface CommitGate clock returned zero time")
	}
	if !previous.IsZero() && now.Before(previous) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Surface CommitGate clock regressed")
	}
	return now, nil
}

func minSurfaceCommitGateV1(values ...int64) int64 {
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

func nilLikeSurfaceCommitGateV1(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func surfaceCommitGateContextErrorV1(ctx context.Context) error {
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Surface CommitGate context is required")
	}
	if ctx.Err() != nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonInvalidState, "Surface CommitGate context is canceled")
	}
	return nil
}

func surfaceCommitGateUnavailableV1(message string) error {
	return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, message)
}

func surfaceCommitGateConflictV1(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, message)
}

func surfaceCommitGateExpiredV1(message string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, message)
}

var _ modelinvoker.PreparedModelInvocationCommitGateV1 = (*PreparedModelInvocationSurfaceCommitGateV1)(nil)

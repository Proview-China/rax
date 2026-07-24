package applicationadapter

import (
	"context"
	"errors"
	"reflect"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type BindingCurrentReaderV2 struct {
	application    applicationports.SingleCallToolActionInputCurrentReaderV2
	model          modelinvoker.ToolCallCandidateObservationProjectionReaderV1
	surfaceBinding toolcontract.ToolSurfaceInvocationBindingReaderV1
	surface        toolcontract.ToolSurfaceManifestCurrentReaderV1
	registry       toolcontract.ToolRegistryObjectCurrentReaderV1
	inputContract  toolcontract.ToolInputContractCurrentReaderV1
	candidate      *CandidateBuilderV3
	association    runtimeports.GenerationBindingAssociationCurrentReaderV1
	generation     runtimeports.GenerationCurrentReaderV1
	route          runtimeports.ControlledOperationProviderRouteCurrentReaderV2
	provider       runtimeports.ProviderBindingCurrentnessPortV2
	store          SingleCallToolActionBindingLeaseStoreV2
	clock          ClockV1
}

func NewBindingCurrentReaderV2(application applicationports.SingleCallToolActionInputCurrentReaderV2, model modelinvoker.ToolCallCandidateObservationProjectionReaderV1, surfaceBinding toolcontract.ToolSurfaceInvocationBindingReaderV1, surface toolcontract.ToolSurfaceManifestCurrentReaderV1, registry toolcontract.ToolRegistryObjectCurrentReaderV1, inputContract toolcontract.ToolInputContractCurrentReaderV1, candidate *CandidateBuilderV3, association runtimeports.GenerationBindingAssociationCurrentReaderV1, generation runtimeports.GenerationCurrentReaderV1, route runtimeports.ControlledOperationProviderRouteCurrentReaderV2, provider runtimeports.ProviderBindingCurrentnessPortV2, store SingleCallToolActionBindingLeaseStoreV2, clock ClockV1) (*BindingCurrentReaderV2, error) {
	for name, dependency := range map[string]any{
		"Application input Reader": application, "Model projection Reader": model, "Surface Invocation Binding Reader": surfaceBinding,
		"Surface current Reader": surface, "Registry current Reader": registry, "Input Contract Reader": inputContract, "CandidateV3 Builder": candidate,
		"Association Reader": association, "Generation Reader": generation, "Route Reader": route, "Provider Reader": provider, "BindingV2 Store": store, "clock": clock,
	} {
		if isNilFlowDependencyV1(dependency) {
			return nil, bindingInvalidV1(name + " is required")
		}
	}
	return &BindingCurrentReaderV2{application: application, model: model, surfaceBinding: surfaceBinding, surface: surface, registry: registry, inputContract: inputContract, candidate: candidate, association: association, generation: generation, route: route, provider: provider, store: store, clock: clock}, nil
}

func (r *BindingCurrentReaderV2) ResolveSingleCallToolActionBindingCurrentV2(ctx context.Context, request SingleCallToolActionBindingResolveRequestV2) (SingleCallToolActionBindingCurrentProjectionV2, error) {
	now, err := r.readyBindingV2(ctx, time.Time{})
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	if err = request.Validate(now); err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	_, issuance, err := sealBindingIssuanceV2(request)
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	id, err := toolcontract.DeriveSingleCallToolActionBindingCurrentIDV2(issuance)
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	existing, inspectErr := r.store.InspectSingleCallToolActionBindingCurrentByIssuanceIDV2(ctx, id)
	if inspectErr == nil {
		return r.validateStoredBindingV2(ctx, request, existing, now)
	}
	if !core.HasCategory(inspectErr, core.ErrorNotFound) {
		return SingleCallToolActionBindingCurrentProjectionV2{}, inspectErr
	}
	s1, last, err := r.readBindingSnapshotV2(ctx, request, nil, now)
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	s2, last, err := r.readBindingSnapshotV2(ctx, request, &s1, last)
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	checked, err := r.readyBindingV2(ctx, last)
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	closure, err := SealSingleCallToolActionCandidateClosureV2(SingleCallToolActionCandidateClosureV2{
		ApplicationInput: s1.input, ModelProjection: s1.model, SurfaceInvocationBinding: s1.surfaceBinding,
		Association: s1.association, Generation: s1.generation, Route: s1.route, ProviderCurrent: s1.provider,
		SurfaceCurrent: s1.surface, CapabilityCurrent: s1.capabilityCurrent, ToolCurrent: s1.toolCurrent, InputContract: s1.inputContract, Candidate: s1.candidate,
	})
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	s2Expires := minimumPositiveUnixNanoV1(s2.bounds...)
	s2Snapshot, err := SealSingleCallToolActionBindingS2SnapshotV2(SingleCallToolActionBindingS2SnapshotV2{
		ApplicationInput: s2.input, SurfaceInvocationBinding: s2.surfaceBinding, Association: s2.association, Generation: s2.generation,
		Route: s2.route, ProviderCurrent: s2.provider, SurfaceCurrent: s2.surface, CapabilityCurrent: s2.capabilityCurrent, ToolCurrent: s2.toolCurrent,
		CheckedUnixNano: last.UnixNano(), ExpiresUnixNano: s2Expires,
	})
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	bounds := append(append([]int64{}, s1.bounds...), s2.bounds...)
	bounds = append(bounds, request.ApplicationRequest.ExpiresUnixNano, s1.inputContract.ExpiresUnixNano, s1.candidate.RequestedExpiresUnixNano, checked.Add(toolcontract.MaxSingleCallToolActionBindingCurrentTTLV2).UnixNano())
	if request.RequestedExpiresUnixNano > 0 {
		bounds = append(bounds, request.RequestedExpiresUnixNano)
	}
	expires := minimumPositiveUnixNanoV1(bounds...)
	if expires <= checked.UnixNano() {
		return SingleCallToolActionBindingCurrentProjectionV2{}, bindingExpiredV1("BindingV2 current window crossed before durable create")
	}
	projection, err := SealSingleCallToolActionBindingCurrentProjectionV2(SingleCallToolActionBindingCurrentProjectionV2{
		IssuanceSubject: issuance, CandidateRef: s1.candidate.ObjectRef(), InputContractCurrentRef: s1.inputContract.Ref,
		CandidateClosure: closure, S2Snapshot: s2Snapshot, RequestedExpiresUnixNano: request.RequestedExpiresUnixNano,
		CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: expires,
	})
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	final, err := r.readyBindingV2(ctx, checked)
	if err != nil || !final.Before(time.Unix(0, projection.ExpiresUnixNano)) {
		if err != nil {
			return SingleCallToolActionBindingCurrentProjectionV2{}, err
		}
		return SingleCallToolActionBindingCurrentProjectionV2{}, bindingExpiredV1("BindingV2 current window crossed at durable create")
	}
	winner, createErr := r.store.CreateSingleCallToolActionBindingCurrentOnceV2(ctx, projection)
	if createErr != nil {
		recovered, recoveryErr := r.store.InspectSingleCallToolActionBindingCurrentByIssuanceIDV2(context.WithoutCancel(ctx), id)
		if recoveryErr != nil {
			return SingleCallToolActionBindingCurrentProjectionV2{}, errors.Join(createErr, recoveryErr)
		}
		winner = recovered
	}
	return r.validateStoredBindingV2(ctx, request, winner, final)
}

func (r *BindingCurrentReaderV2) InspectSingleCallToolActionBindingCurrentByIssuanceV2(ctx context.Context, lookup SingleCallToolActionBindingIssuanceLookupRequestV2) (SingleCallToolActionBindingCurrentProjectionV2, error) {
	now, err := r.readyBindingV2(ctx, time.Time{})
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	request := lookup.resolveV2()
	if err = request.Validate(now); err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	_, issuance, err := sealBindingIssuanceV2(request)
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	id, err := toolcontract.DeriveSingleCallToolActionBindingCurrentIDV2(issuance)
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	winner, err := r.store.InspectSingleCallToolActionBindingCurrentByIssuanceIDV2(ctx, id)
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	return r.validateStoredBindingV2(ctx, request, winner, now)
}

func (r *BindingCurrentReaderV2) InspectExactSingleCallToolActionBindingCurrentV2(ctx context.Context, exact SingleCallToolActionBindingInspectExactRequestV2) (SingleCallToolActionBindingCurrentProjectionV2, error) {
	now, err := r.readyBindingV2(ctx, time.Time{})
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	request := exact.resolveV2()
	if err = request.Validate(now); err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	if err = exact.Expected.Validate(); err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	_, issuance, err := sealBindingIssuanceV2(request)
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	id, err := toolcontract.DeriveSingleCallToolActionBindingCurrentIDV2(issuance)
	if err != nil || exact.Expected.ID != id {
		return SingleCallToolActionBindingCurrentProjectionV2{}, bindingConflictV1("BindingV2 exact Ref names another issuance")
	}
	winner, err := r.store.InspectExactSingleCallToolActionBindingCurrentV2(ctx, exact.Expected)
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	return r.validateStoredBindingV2(ctx, request, winner, now)
}

type bindingSnapshotV2 struct {
	input             applicationcontract.SingleCallToolActionInputCurrentProjectionV2
	model             modelinvoker.ToolCallCandidateObservationProjectionV1
	surfaceBinding    toolcontract.ToolSurfaceInvocationBindingV1
	surfaceBindingAck toolcontract.ToolSurfaceInvocationBindingAckV1
	association       runtimeports.GenerationBindingAssociationFactV1
	generation        runtimeports.GenerationCurrentProjectionV1
	route             runtimeports.ControlledOperationProviderRouteCurrentProjectionV2
	provider          runtimeports.ProviderBindingCurrentProjectionV2
	surface           toolcontract.ToolSurfaceManifestCurrentProjectionV1
	capabilityCurrent toolcontract.ToolRegistryObjectCurrentProjectionV1
	toolCurrent       toolcontract.ToolRegistryObjectCurrentProjectionV1
	inputContract     toolcontract.ToolInputContractCurrentProjectionV1
	candidate         toolcontract.ActionCandidateV3
	bounds            []int64
}

func (r *BindingCurrentReaderV2) readBindingSnapshotV2(ctx context.Context, request SingleCallToolActionBindingResolveRequestV2, first *bindingSnapshotV2, previous time.Time) (bindingSnapshotV2, time.Time, error) {
	var out bindingSnapshotV2
	now, err := r.readyBindingV2(ctx, previous)
	if err != nil {
		return out, previous, err
	}
	input, err := r.application.InspectSingleCallToolActionInputCurrentV2(ctx, request.ApplicationRequest)
	if err != nil {
		return out, now, err
	}
	now, err = r.readyBindingV2(ctx, now)
	if err != nil || input.ValidateFor(request.ApplicationRequest, now) != nil {
		if err == nil {
			err = input.ValidateFor(request.ApplicationRequest, now)
		}
		return out, now, err
	}
	out.input = applicationcontract.CloneSingleCallToolActionInputCurrentProjectionV2(input)
	out.bounds = append(out.bounds, input.ExpiresUnixNano)

	identity := request.SourceSubject.Identity
	modelRef := modelinvoker.ToolCallCandidateObservationRefV1{ID: identity.ModelProjectionID, Revision: identity.ModelProjectionRevision, Digest: identity.ModelProjectionDigest, InvocationID: identity.ModelInvocationID, InvocationDigest: identity.ModelInvocationDigest, ObservationDigest: identity.ModelObservationDigest, Source: modelinvoker.ToolCallCandidateObservationSourceCoordinateV1{SourceSequence: identity.ModelSourceSequence, ResponseID: identity.ModelSourceResponseID}}
	if first == nil {
		model, modelErr := r.model.InspectExactProjectionV1(ctx, modelRef)
		if modelErr != nil {
			return out, now, modelErr
		}
		now, err = r.readyBindingV2(ctx, now)
		if err != nil || validateModelForBindingV1(model, modelRef, input, request.SourceSubject) != nil {
			if err == nil {
				err = validateModelForBindingV1(model, modelRef, input, request.SourceSubject)
			}
			return out, now, err
		}
		out.model = model.Clone()
	} else {
		out.model = first.model.Clone()
	}

	invocation := toolcontract.ToolSurfaceInvocationCoordinateV1{InvocationID: modelRef.InvocationID, InvocationDigest: modelRef.InvocationDigest}
	if first == nil {
		out.surfaceBinding, out.surfaceBindingAck, err = r.surfaceBinding.InspectToolSurfaceInvocationBindingByInvocationV1(ctx, invocation)
	} else {
		out.surfaceBinding, out.surfaceBindingAck, err = r.surfaceBinding.InspectExactToolSurfaceInvocationBindingV1(ctx, first.surfaceBinding.Ref)
	}
	if err != nil {
		return out, now, err
	}
	now, err = r.readyBindingV2(ctx, now)
	if err != nil || out.surfaceBinding.ValidateCurrent(now) != nil || out.surfaceBindingAck.ValidateAgainst(out.surfaceBinding, now) != nil || out.surfaceBinding.Subject.Invocation != invocation {
		if err == nil {
			if err = out.surfaceBinding.ValidateCurrent(now); err == nil {
				err = out.surfaceBindingAck.ValidateAgainst(out.surfaceBinding, now)
			}
			if err == nil {
				err = bindingConflictV1("Surface Invocation Binding differs from Model invocation")
			}
		}
		return out, now, err
	}
	out.bounds = append(out.bounds, out.surfaceBinding.NotAfterUnixNano)

	associationRef := request.SourceSubject.Binding.OwnerInputs.GenerationBindingAssociation
	out.association, err = r.association.InspectCurrentGenerationBindingAssociationV1(ctx, associationRef.ID)
	if err != nil {
		return out, now, err
	}
	now, err = r.readyBindingV2(ctx, now)
	if err != nil || validateAssociationCurrentV1(out.association, associationRef, now) != nil {
		if err == nil {
			err = validateAssociationCurrentV1(out.association, associationRef, now)
		}
		return out, now, err
	}
	out.bounds = append(out.bounds, out.association.ExpiresUnixNano)
	out.generation, err = r.generation.InspectGenerationCurrentV1(ctx, out.association.Candidate.Generation.Generation)
	if err != nil {
		return out, now, err
	}
	now, err = r.readyBindingV2(ctx, now)
	if err != nil || out.generation.ValidateCurrent(out.association.Candidate.Generation.Generation, now) != nil || !reflect.DeepEqual(out.generation, out.association.Candidate.Generation) {
		if err == nil {
			err = out.generation.ValidateCurrent(out.association.Candidate.Generation.Generation, now)
		}
		if err == nil {
			err = bindingConflictV1("Generation current differs from Association")
		}
		return out, now, err
	}
	out.bounds = append(out.bounds, out.generation.ExpiresUnixNano)
	matrix := request.SourceSubject.Binding.OwnerInputs.RouteMatrix
	out.route, err = r.route.InspectCurrentControlledOperationProviderRouteV2(ctx, request.SourceSubject.Binding.OwnerInputs.RouteCurrent, matrix)
	if err != nil {
		return out, now, err
	}
	now, err = r.readyBindingV2(ctx, now)
	if err != nil || out.route.ValidateCurrent(request.SourceSubject.Binding.OwnerInputs.RouteCurrent, matrix, now) != nil || validateRouteBindingV1(out.route, out.association, out.generation) != nil {
		if err == nil {
			err = out.route.ValidateCurrent(request.SourceSubject.Binding.OwnerInputs.RouteCurrent, matrix, now)
		}
		if err == nil {
			err = validateRouteBindingV1(out.route, out.association, out.generation)
		}
		return out, now, err
	}
	out.bounds = append(out.bounds, out.route.ExpiresUnixNano)
	out.provider, err = r.provider.InspectProviderBindingCurrentV2(ctx, out.route.ProviderBinding)
	if err != nil {
		return out, now, err
	}
	now, err = r.readyBindingV2(ctx, now)
	if err != nil || out.provider.ValidateCurrent(out.route.ProviderBinding, now) != nil || out.provider.BindingSetDigest != out.route.BindingSetDigest || out.provider.BindingSetSemanticDigest != out.route.BindingSetSemanticDigest {
		if err == nil {
			err = out.provider.ValidateCurrent(out.route.ProviderBinding, now)
		}
		if err == nil {
			err = bindingConflictV1("Provider current differs from Route")
		}
		return out, now, err
	}
	out.bounds = append(out.bounds, out.provider.ExpiresUnixNano)

	surfaceRef := out.surfaceBinding.Subject.SurfaceCurrent.Ref
	out.surface, err = r.surface.InspectExactToolSurfaceManifestCurrentV1(ctx, surfaceRef)
	if err != nil {
		return out, now, err
	}
	now, err = r.readyBindingV2(ctx, now)
	if err != nil || out.surface.ValidateCurrent(surfaceRef, now) != nil {
		if err == nil {
			err = out.surface.ValidateCurrent(surfaceRef, now)
		}
		return out, now, err
	}
	out.bounds = append(out.bounds, out.surface.ExpiresUnixNano)
	entry, err := exactEntryForCallV2(out.surface, identity.CallName)
	if err != nil {
		return out, now, err
	}
	if first == nil {
		_, out.capabilityCurrent, err = r.registry.ResolveExactToolCapabilityCurrentV1(ctx, entry.Capability)
	} else {
		_, out.capabilityCurrent, err = r.registry.InspectExactToolCapabilityCurrentV1(ctx, entry.Capability, first.capabilityCurrent.Ref)
	}
	if err != nil {
		return out, now, err
	}
	if first == nil {
		_, out.toolCurrent, err = r.registry.ResolveExactToolDescriptorCurrentV1(ctx, entry.Tool)
	} else {
		_, out.toolCurrent, err = r.registry.InspectExactToolDescriptorCurrentV1(ctx, entry.Tool, first.toolCurrent.Ref)
	}
	if err != nil {
		return out, now, err
	}
	now, err = r.readyBindingV2(ctx, now)
	if err != nil || out.capabilityCurrent.ValidateCurrent(now) != nil || out.toolCurrent.ValidateCurrent(now) != nil {
		if err == nil {
			err = out.capabilityCurrent.ValidateCurrent(now)
		}
		if err == nil {
			err = out.toolCurrent.ValidateCurrent(now)
		}
		return out, now, err
	}
	out.bounds = append(out.bounds, out.capabilityCurrent.ExpiresUnixNano, out.toolCurrent.ExpiresUnixNano)

	inputRequest := inputContractRequestForBindingV2(request, input, out.surface, entry, out.route)
	if first == nil {
		out.inputContract, err = r.inputContract.ResolveToolInputContractCurrentV1(ctx, inputRequest)
	} else {
		out.inputContract, err = r.inputContract.InspectExactToolInputContractCurrentV1(ctx, toolcontract.ToolInputContractInspectExactRequestV1{ResolveRequest: inputRequest, Expected: first.inputContract.Ref})
	}
	if err != nil {
		return out, now, err
	}
	now, err = r.readyBindingV2(ctx, now)
	if err != nil || out.inputContract.ValidateCurrent(now) != nil {
		if err == nil {
			err = out.inputContract.ValidateCurrent(now)
		}
		return out, now, err
	}
	out.bounds = append(out.bounds, out.inputContract.ExpiresUnixNano)

	if first == nil {
		out.candidate, err = r.candidate.BuildSingleCallToolActionCandidateV3(ctx, SingleCallToolActionCandidateBuildRequestV3{
			ApplicationRequest: request.ApplicationRequest, ApplicationInput: input, ModelProjection: out.model,
			SurfaceBinding: out.surfaceBinding, SurfaceBindingAck: out.surfaceBindingAck, InputContract: out.inputContract, Route: out.route, ProviderCurrent: out.provider,
		})
		if err != nil {
			return out, now, err
		}
		out.bounds = append(out.bounds, out.candidate.RequestedExpiresUnixNano)
		return cloneBindingSnapshotV2(out), now, nil
	}
	out.candidate = toolcontract.CloneActionCandidateV3(first.candidate)
	out.bounds = append(out.bounds, out.candidate.RequestedExpiresUnixNano)
	if err := compareBindingSnapshotsV2(*first, out); err != nil {
		return out, now, err
	}
	return cloneBindingSnapshotV2(out), now, nil
}

func (r *BindingCurrentReaderV2) validateStoredBindingV2(ctx context.Context, request SingleCallToolActionBindingResolveRequestV2, winner SingleCallToolActionBindingCurrentProjectionV2, previous time.Time) (SingleCallToolActionBindingCurrentProjectionV2, error) {
	now, err := r.readyBindingV2(ctx, previous)
	if err != nil || winner.ValidateAgainst(request, now) != nil {
		if err == nil {
			err = winner.ValidateAgainst(request, now)
		}
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	first := snapshotFromBindingWinnerV2(winner)
	if _, _, err = r.readBindingSnapshotV2(ctx, request, &first, now); err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	final, err := r.readyBindingV2(ctx, now)
	if err != nil || winner.ValidateCurrent(final) != nil {
		if err == nil {
			err = winner.ValidateCurrent(final)
		}
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	return CloneSingleCallToolActionBindingCurrentProjectionV2(winner), nil
}

func (r *BindingCurrentReaderV2) readyBindingV2(ctx context.Context, previous time.Time) (time.Time, error) {
	if ctx == nil {
		return time.Time{}, bindingInvalidV1("BindingV2 context is required")
	}
	if err := ctx.Err(); err != nil {
		return time.Time{}, err
	}
	if r == nil || isNilFlowDependencyV1(r.application) || isNilFlowDependencyV1(r.model) || isNilFlowDependencyV1(r.surfaceBinding) || isNilFlowDependencyV1(r.surface) || isNilFlowDependencyV1(r.registry) || isNilFlowDependencyV1(r.inputContract) || isNilFlowDependencyV1(r.candidate) || isNilFlowDependencyV1(r.association) || isNilFlowDependencyV1(r.generation) || isNilFlowDependencyV1(r.route) || isNilFlowDependencyV1(r.provider) || isNilFlowDependencyV1(r.store) || isNilFlowDependencyV1(r.clock) {
		return time.Time{}, bindingUnavailableV1("BindingV2 Reader dependency is unavailable")
	}
	now := r.clock.Now()
	if now.IsZero() || (!previous.IsZero() && now.Before(previous)) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "BindingV2 clock regressed")
	}
	return now, nil
}

func inputContractRequestForBindingV2(request SingleCallToolActionBindingResolveRequestV2, input applicationcontract.SingleCallToolActionInputCurrentProjectionV2, surface toolcontract.ToolSurfaceManifestCurrentProjectionV1, entry toolcontract.ToolSurfaceEntry, route runtimeports.ControlledOperationProviderRouteCurrentProjectionV2) toolcontract.ToolInputContractResolveRequestV1 {
	requested := minimumPositiveUnixNanoV1(request.ApplicationRequest.ExpiresUnixNano, input.ExpiresUnixNano, request.RequestedExpiresUnixNano)
	return toolcontract.ToolInputContractResolveRequestV1{
		ApplicationRequestID: request.ApplicationRequest.ID, ApplicationRequestRevision: request.ApplicationRequest.Revision, ApplicationRequestDigest: request.ApplicationRequest.Digest,
		PendingAction:        toolcontract.PendingActionExactRefV2{ID: request.SourceSubject.PendingActionRef, Revision: 1, RequestDigest: request.SourceSubject.PendingActionDigest},
		OperationScopeDigest: request.ApplicationRequest.Action.ExecutionScopeDigest, ProviderBinding: route.ProviderBinding, ExpectedOwner: effectOwnerFromRouteV1(route),
		Surface: toolcontract.ObjectRef{ID: surface.Ref.ID, Revision: surface.Ref.Revision, Digest: surface.Ref.Digest}, CallName: entry.ModelName,
		Capability: entry.Capability, Tool: entry.Tool, InputSchema: entry.InputSchema, RequestedExpiresUnixNano: requested,
	}
}

func exactEntryForCallV2(surface toolcontract.ToolSurfaceManifestCurrentProjectionV1, callName string) (toolcontract.ToolSurfaceEntry, error) {
	var result toolcontract.ToolSurfaceEntry
	count := 0
	for _, entry := range surface.Manifest.Entries {
		if entry.ModelName == callName {
			result = entry
			count++
		}
	}
	if count != 1 {
		return toolcontract.ToolSurfaceEntry{}, bindingConflictV1("Model CallName does not select exactly one Tool Surface entry")
	}
	result.EffectKinds = append([]runtimeports.NamespacedNameV2(nil), result.EffectKinds...)
	return result, nil
}

func compareBindingSnapshotsV2(s1, s2 bindingSnapshotV2) error {
	if s2.input.CheckedUnixNano < s1.input.CheckedUnixNano || s2.model.Ref != s1.model.Ref || s2.surfaceBinding.Ref != s1.surfaceBinding.Ref || s2.association.RefV1() != s1.association.RefV1() || s2.generation.Generation != s1.generation.Generation || s2.route.Ref != s1.route.Ref || s2.provider.Ref != s1.provider.Ref || s2.surface.Ref != s1.surface.Ref || s2.capabilityCurrent.Ref != s1.capabilityCurrent.Ref || s2.toolCurrent.Ref != s1.toolCurrent.Ref || s2.inputContract.Ref != s1.inputContract.Ref || s2.candidate.ObjectRef() != s1.candidate.ObjectRef() {
		return bindingConflictV1("BindingV2 S2 stable exact coordinates drifted from S1")
	}
	a, b := s1.input.HarnessCurrent.Subject, s2.input.HarnessCurrent.Subject
	if a.Digest != b.Digest || a.PendingActionRef != b.PendingActionRef || a.PendingActionDigest != b.PendingActionDigest || a.Binding.Digest != b.Binding.Digest || a.Identity.Digest != b.Identity.Digest {
		return bindingConflictV1("BindingV2 S2 PendingAction identity drifted from S1")
	}
	return nil
}

func snapshotFromBindingWinnerV2(p SingleCallToolActionBindingCurrentProjectionV2) bindingSnapshotV2 {
	c := p.CandidateClosure
	return bindingSnapshotV2{input: c.ApplicationInput, model: c.ModelProjection, surfaceBinding: c.SurfaceInvocationBinding, association: c.Association, generation: c.Generation, route: c.Route, provider: c.ProviderCurrent, surface: c.SurfaceCurrent, capabilityCurrent: c.CapabilityCurrent, toolCurrent: c.ToolCurrent, inputContract: c.InputContract, candidate: c.Candidate}
}

func cloneBindingSnapshotV2(s bindingSnapshotV2) bindingSnapshotV2 {
	s.input = applicationcontract.CloneSingleCallToolActionInputCurrentProjectionV2(s.input)
	s.model = s.model.Clone()
	s.surfaceBinding = cloneSurfaceInvocationBindingV2(s.surfaceBinding)
	s.association = cloneAssociationV1(s.association)
	s.generation = cloneGenerationV1(s.generation)
	s.surface = cloneSurfaceCurrentProjectionV2(s.surface)
	s.inputContract = toolcontract.CloneToolInputContractCurrentProjectionV1(s.inputContract)
	s.candidate = toolcontract.CloneActionCandidateV3(s.candidate)
	s.bounds = append([]int64(nil), s.bounds...)
	return s
}

var _ SingleCallToolActionBindingCurrentReaderV2 = (*BindingCurrentReaderV2)(nil)

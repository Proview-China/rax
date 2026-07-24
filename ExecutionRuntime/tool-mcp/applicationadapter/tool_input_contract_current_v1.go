package applicationadapter

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type ToolInputContractCurrentResolverV1 struct {
	surface  toolcontract.ToolSurfaceManifestCurrentReaderV1
	registry toolcontract.ToolRegistryObjectCurrentReaderV1
	store    toolcontract.ToolInputContractLeaseStoreV1
	clock    ClockV1
}

func NewToolInputContractCurrentResolverV1(surface toolcontract.ToolSurfaceManifestCurrentReaderV1, registry toolcontract.ToolRegistryObjectCurrentReaderV1, store toolcontract.ToolInputContractLeaseStoreV1, clock ClockV1) (*ToolInputContractCurrentResolverV1, error) {
	if isNilFlowDependencyV1(surface) || isNilFlowDependencyV1(registry) || isNilFlowDependencyV1(store) || isNilFlowDependencyV1(clock) {
		return nil, bindingInvalidV1("Tool Input Contract Resolver dependencies are required")
	}
	return &ToolInputContractCurrentResolverV1{surface: surface, registry: registry, store: store, clock: clock}, nil
}

func (r *ToolInputContractCurrentResolverV1) ResolveToolInputContractCurrentV1(ctx context.Context, request toolcontract.ToolInputContractResolveRequestV1) (toolcontract.ToolInputContractCurrentProjectionV1, error) {
	now, err := r.readyInputContractV1(ctx, time.Time{})
	if err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	if err = request.Validate(now); err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	issuance, id, err := inputContractIssuanceIDV1(request)
	if err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	existing, inspectErr := r.store.InspectToolInputContractCurrentByIssuanceIDV1(ctx, id)
	if inspectErr == nil {
		return r.validateInputContractWinnerV1(ctx, request, existing, now)
	}
	if !core.HasCategory(inspectErr, core.ErrorNotFound) {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, inspectErr
	}

	surface, capability, capabilityCurrent, tool, toolCurrent, entry, sourceNow, err := r.readInputContractSourcesV1(ctx, request, now, toolcontract.ToolInputContractCurrentProjectionV1{})
	if err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	policy, err := toolcontract.DeriveToolInputLimitPolicyV1(request.Surface, entry.Order, entry, request.Capability, request.Tool, request.InputSchema)
	if err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	subject, err := toolcontract.SealToolInputContractBindingSubjectV1(toolcontract.ToolInputContractBindingSubjectV1{
		ApplicationRequestID: request.ApplicationRequestID, ApplicationRequestRevision: request.ApplicationRequestRevision, ApplicationRequestDigest: request.ApplicationRequestDigest,
		PendingAction: request.PendingAction, OperationScopeDigest: request.OperationScopeDigest, ProviderBinding: request.ProviderBinding, ExpectedOwner: request.ExpectedOwner,
		SurfaceOwner: surface.Owner, CapabilityRegistryOwner: capability.Owner, ToolRegistryOwner: tool.Owner,
		Surface: request.Surface, SurfaceEntryOrdinal: entry.Order, SurfaceEntry: entry, Capability: request.Capability, Tool: request.Tool,
		ToolArtifactDigest: tool.ArtifactDigest, InputSchema: request.InputSchema, LimitPolicy: policy,
	})
	if err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	expires := minimumPositiveUnixNanoV1(
		surface.ExpiresUnixNano,
		capabilityCurrent.ExpiresUnixNano,
		toolCurrent.ExpiresUnixNano,
		sourceNow.Add(toolcontract.MaxToolInputContractCurrentTTLV1).UnixNano(),
		request.RequestedExpiresUnixNano,
	)
	if expires <= sourceNow.UnixNano() {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Tool Input Contract has no current window")
	}
	schemaCurrent, err := toolcontract.SealToolInputSchemaCurrentRefV1(toolcontract.ToolInputSchemaCurrentRefV1{
		InputSchema: request.InputSchema, Authority: toolCurrent.Ref, RegistryOwner: tool.Owner,
		CheckedUnixNano: sourceNow.UnixNano(), ExpiresUnixNano: expires,
	})
	if err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	sealed, err := toolcontract.SealToolInputContractCurrentV1(toolcontract.ToolInputContractCurrentProjectionV1{
		IssuanceSubject: issuance, BindingSubject: subject, SurfaceCurrent: surface, CapabilityCurrent: capabilityCurrent, ToolCurrent: toolCurrent,
		InputSchemaCurrent: schemaCurrent, RequestedExpiresUnixNano: request.RequestedExpiresUnixNano, CheckedUnixNano: sourceNow.UnixNano(), ExpiresUnixNano: expires,
	})
	if err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	winner, createErr := r.store.CreateToolInputContractCurrentOnceV1(ctx, sealed)
	if createErr != nil {
		recoveryContext := context.WithoutCancel(ctx)
		recovered, recoveryErr := r.store.InspectToolInputContractCurrentByIssuanceIDV1(recoveryContext, id)
		if recoveryErr != nil {
			return toolcontract.ToolInputContractCurrentProjectionV1{}, errors.Join(createErr, recoveryErr)
		}
		winner = recovered
	}
	return r.validateInputContractWinnerV1(ctx, request, winner, sourceNow)
}

func (r *ToolInputContractCurrentResolverV1) InspectToolInputContractCurrentByIssuanceV1(ctx context.Context, request toolcontract.ToolInputContractInspectByIssuanceRequestV1) (toolcontract.ToolInputContractCurrentProjectionV1, error) {
	now, err := r.readyInputContractV1(ctx, time.Time{})
	if err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	if err = request.ResolveRequest.Validate(now); err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	_, id, err := inputContractIssuanceIDV1(request.ResolveRequest)
	if err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	winner, err := r.store.InspectToolInputContractCurrentByIssuanceIDV1(ctx, id)
	if err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	return r.validateInputContractWinnerV1(ctx, request.ResolveRequest, winner, now)
}

func (r *ToolInputContractCurrentResolverV1) InspectExactToolInputContractCurrentV1(ctx context.Context, request toolcontract.ToolInputContractInspectExactRequestV1) (toolcontract.ToolInputContractCurrentProjectionV1, error) {
	now, err := r.readyInputContractV1(ctx, time.Time{})
	if err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	if err = request.Expected.Validate(); err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	if err = request.ResolveRequest.Validate(now); err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	_, id, err := inputContractIssuanceIDV1(request.ResolveRequest)
	if err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	if request.Expected.ID != id {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, bindingConflictV1("Tool Input Contract exact Ref names another issuance")
	}
	winner, err := r.store.InspectExactToolInputContractCurrentV1(ctx, request.Expected)
	if err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	return r.validateInputContractWinnerV1(ctx, request.ResolveRequest, winner, now)
}

func (r *ToolInputContractCurrentResolverV1) validateInputContractWinnerV1(ctx context.Context, request toolcontract.ToolInputContractResolveRequestV1, winner toolcontract.ToolInputContractCurrentProjectionV1, previous time.Time) (toolcontract.ToolInputContractCurrentProjectionV1, error) {
	now, err := r.readyInputContractV1(ctx, previous)
	if err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	if err = winner.ValidateAgainst(request, now); err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	_, _, _, _, _, _, sourceNow, err := r.readInputContractSourcesV1(ctx, request, now, winner)
	if err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	final, err := r.readyInputContractV1(ctx, sourceNow)
	if err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	if err = winner.ValidateCurrent(final); err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	return toolcontract.CloneToolInputContractCurrentProjectionV1(winner), nil
}

func (r *ToolInputContractCurrentResolverV1) readInputContractSourcesV1(ctx context.Context, request toolcontract.ToolInputContractResolveRequestV1, previous time.Time, expected toolcontract.ToolInputContractCurrentProjectionV1) (toolcontract.ToolSurfaceManifestCurrentProjectionV1, toolcontract.CapabilityDescriptor, toolcontract.ToolRegistryObjectCurrentProjectionV1, toolcontract.ToolDescriptor, toolcontract.ToolRegistryObjectCurrentProjectionV1, toolcontract.ToolSurfaceEntry, time.Time, error) {
	now, err := r.readyInputContractV1(ctx, previous)
	if err != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, toolcontract.CapabilityDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, toolcontract.ToolDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, toolcontract.ToolSurfaceEntry{}, time.Time{}, err
	}
	surfaceRef := toolcontract.ToolSurfaceManifestCurrentRefV1{ContractVersion: toolcontract.ToolSurfaceManifestCurrentContractVersionV1, ID: request.Surface.ID, Revision: request.Surface.Revision, Digest: request.Surface.Digest}
	surface, err := r.surface.InspectExactToolSurfaceManifestCurrentV1(ctx, surfaceRef)
	if err != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, toolcontract.CapabilityDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, toolcontract.ToolDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, toolcontract.ToolSurfaceEntry{}, time.Time{}, err
	}
	now, err = r.readyInputContractV1(ctx, now)
	if err != nil || surface.ValidateCurrent(surfaceRef, now) != nil {
		if err == nil {
			err = surface.ValidateCurrent(surfaceRef, now)
		}
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, toolcontract.CapabilityDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, toolcontract.ToolDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, toolcontract.ToolSurfaceEntry{}, time.Time{}, err
	}
	entry, err := exactSurfaceEntryV1(surface, request)
	if err != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, toolcontract.CapabilityDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, toolcontract.ToolDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, toolcontract.ToolSurfaceEntry{}, time.Time{}, err
	}
	var capability toolcontract.CapabilityDescriptor
	var capabilityCurrent toolcontract.ToolRegistryObjectCurrentProjectionV1
	if expected.Ref.ID == "" {
		capability, capabilityCurrent, err = r.registry.ResolveExactToolCapabilityCurrentV1(ctx, request.Capability)
	} else {
		capability, capabilityCurrent, err = r.registry.InspectExactToolCapabilityCurrentV1(ctx, request.Capability, expected.CapabilityCurrent.Ref)
	}
	if err != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, toolcontract.CapabilityDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, toolcontract.ToolDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, toolcontract.ToolSurfaceEntry{}, time.Time{}, err
	}
	var tool toolcontract.ToolDescriptor
	var toolCurrent toolcontract.ToolRegistryObjectCurrentProjectionV1
	if expected.Ref.ID == "" {
		tool, toolCurrent, err = r.registry.ResolveExactToolDescriptorCurrentV1(ctx, request.Tool)
	} else {
		tool, toolCurrent, err = r.registry.InspectExactToolDescriptorCurrentV1(ctx, request.Tool, expected.ToolCurrent.Ref)
	}
	if err != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, toolcontract.CapabilityDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, toolcontract.ToolDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, toolcontract.ToolSurfaceEntry{}, time.Time{}, err
	}
	now, err = r.readyInputContractV1(ctx, now)
	if err != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, toolcontract.CapabilityDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, toolcontract.ToolDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, toolcontract.ToolSurfaceEntry{}, time.Time{}, err
	}
	if err = capabilityCurrent.ValidateCurrent(now); err != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, toolcontract.CapabilityDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, toolcontract.ToolDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, toolcontract.ToolSurfaceEntry{}, time.Time{}, err
	}
	if err = toolCurrent.ValidateCurrent(now); err != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, toolcontract.CapabilityDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, toolcontract.ToolDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, toolcontract.ToolSurfaceEntry{}, time.Time{}, err
	}
	if capability.InputSchema != request.InputSchema || tool.InputSchema != request.InputSchema || tool.Capability != request.Capability || tool.ArtifactDigest != request.ProviderBinding.ArtifactDigest || request.ProviderBinding.Capability != runtimeports.CapabilityNameV2(runtimeports.OperationScopeEvidenceActionEffectKindV3) || request.ProviderBinding.ComponentID != request.ExpectedOwner.ComponentID || request.ProviderBinding.ManifestDigest != request.ExpectedOwner.ManifestDigest || request.ExpectedOwner.Role != runtimeports.OwnerSettlement || !toolcontract.ContainsName(capability.EffectKinds, runtimeports.NamespacedNameV2(runtimeports.OperationScopeEvidenceActionEffectKindV3)) || !toolcontract.ContainsName(tool.EffectKinds, runtimeports.NamespacedNameV2(runtimeports.OperationScopeEvidenceActionEffectKindV3)) {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, toolcontract.CapabilityDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, toolcontract.ToolDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, toolcontract.ToolSurfaceEntry{}, time.Time{}, bindingConflictV1("Tool Input Contract descriptor/provider bindings drifted")
	}
	if expected.Ref.ID != "" {
		if !reflect.DeepEqual(surface.Manifest, expected.SurfaceCurrent.Manifest) || surface.Owner != expected.BindingSubject.SurfaceOwner || capability.Owner != expected.BindingSubject.CapabilityRegistryOwner || tool.Owner != expected.BindingSubject.ToolRegistryOwner || tool.ArtifactDigest != expected.BindingSubject.ToolArtifactDigest || !reflect.DeepEqual(entry, expected.BindingSubject.SurfaceEntry) {
			return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, toolcontract.CapabilityDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, toolcontract.ToolDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, toolcontract.ToolSurfaceEntry{}, time.Time{}, bindingConflictV1("Tool Input Contract exact source closure drifted")
		}
	}
	return surface, capability, capabilityCurrent, tool, toolCurrent, entry, now, nil
}

func (r *ToolInputContractCurrentResolverV1) readyInputContractV1(ctx context.Context, previous time.Time) (time.Time, error) {
	if ctx == nil {
		return time.Time{}, bindingInvalidV1("Tool Input Contract context is required")
	}
	if err := ctx.Err(); err != nil {
		return time.Time{}, err
	}
	if r == nil || isNilFlowDependencyV1(r.surface) || isNilFlowDependencyV1(r.registry) || isNilFlowDependencyV1(r.store) || isNilFlowDependencyV1(r.clock) {
		return time.Time{}, bindingUnavailableV1("Tool Input Contract Resolver is unavailable")
	}
	now := r.clock.Now()
	if now.IsZero() || (!previous.IsZero() && now.Before(previous)) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Input Contract clock regressed")
	}
	return now, nil
}

func inputContractIssuanceIDV1(request toolcontract.ToolInputContractResolveRequestV1) (toolcontract.ToolInputContractIssuanceSubjectV1, string, error) {
	issuance, err := toolcontract.ToolInputContractIssuanceFromResolveRequestV1(request)
	if err != nil {
		return toolcontract.ToolInputContractIssuanceSubjectV1{}, "", err
	}
	id, err := toolcontract.DeriveToolInputContractCurrentIDV1(issuance)
	return issuance, id, err
}

func exactSurfaceEntryV1(surface toolcontract.ToolSurfaceManifestCurrentProjectionV1, request toolcontract.ToolInputContractResolveRequestV1) (toolcontract.ToolSurfaceEntry, error) {
	var result toolcontract.ToolSurfaceEntry
	found := false
	for _, entry := range surface.Manifest.Entries {
		if entry.ModelName != request.CallName {
			continue
		}
		if found {
			return toolcontract.ToolSurfaceEntry{}, bindingConflictV1("Tool Surface contains duplicate call names")
		}
		result, found = entry, true
	}
	if !found {
		return toolcontract.ToolSurfaceEntry{}, bindingNotFoundV1("Tool Surface call name is absent")
	}
	if result.Capability != request.Capability || result.Tool != request.Tool || result.InputSchema != request.InputSchema || !result.Allowed || result.Visibility != toolcontract.SurfaceVisible || !toolcontract.ContainsName(result.EffectKinds, runtimeports.NamespacedNameV2(runtimeports.OperationScopeEvidenceActionEffectKindV3)) {
		return toolcontract.ToolSurfaceEntry{}, bindingConflictV1("Tool Surface entry differs from exact request")
	}
	result.EffectKinds = append([]runtimeports.NamespacedNameV2(nil), result.EffectKinds...)
	return result, nil
}

func minimumPositiveUnixNanoV1(values ...int64) int64 {
	minimum := int64(0)
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if minimum == 0 || value < minimum {
			minimum = value
		}
	}
	return minimum
}

var _ toolcontract.ToolInputContractCurrentReaderV1 = (*ToolInputContractCurrentResolverV1)(nil)

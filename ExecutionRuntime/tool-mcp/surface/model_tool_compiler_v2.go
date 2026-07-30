package surface

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type modelToolInjectionClosureV1 struct {
	entry             toolcontract.ToolSurfaceEntry
	definition        toolcontract.ToolDefinitionMaterialV1
	capability        toolcontract.CapabilityDescriptor
	capabilityCurrent toolcontract.ToolRegistryObjectCurrentProjectionV1
	tool              toolcontract.ToolDescriptor
	toolCurrent       toolcontract.ToolRegistryObjectCurrentProjectionV1
}

// CompileModelToolInjectionMaterialV1 performs Tool Owner-local S1/S2 exact
// reads and seals both the neutral Model tools and their immutable injection
// material. It does not select ToolChoice, invoke a provider, execute a Tool,
// or issue Review/Authority/Permit state.
func CompileModelToolInjectionMaterialV1(
	ctx context.Context,
	exactSurface toolcontract.ToolSurfaceManifestCurrentRefV1,
	surfaces toolcontract.ToolSurfaceManifestCurrentReaderV1,
	definitions toolcontract.ToolDefinitionMaterialReaderV1,
	registry toolcontract.ToolRegistryObjectCurrentReaderV1,
	clock func() time.Time,
) (CompiledModelToolsV1, toolcontract.ModelToolInjectionMaterialV1, error) {
	zero := func(err error) (CompiledModelToolsV1, toolcontract.ModelToolInjectionMaterialV1, error) {
		return CompiledModelToolsV1{}, toolcontract.ModelToolInjectionMaterialV1{}, err
	}
	if ctx == nil {
		return zero(core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "Model Tool Injection compile context is required"))
	}
	if err := ctx.Err(); err != nil {
		return zero(err)
	}
	if exactSurface.Validate() != nil {
		return zero(core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Model Tool Injection exact Surface Ref is invalid"))
	}
	if dependencyUnavailableV2(surfaces) || dependencyUnavailableV2(definitions) || dependencyUnavailableV2(registry) {
		return zero(core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Model Tool Injection exact Reader is nil or typed-nil"))
	}
	if clock == nil {
		return zero(core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "Model Tool Injection compile clock is required"))
	}
	nowS1 := clock()
	if nowS1.IsZero() {
		return zero(core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Model Tool Injection S1 clock is unavailable"))
	}
	surfaceS1, err := surfaces.InspectExactToolSurfaceManifestCurrentV1(ctx, exactSurface)
	if err != nil {
		return zero(err)
	}
	surfaceNowS1, err := freshCompileTimeV2(clock, nowS1, "Model Tool Injection clock regressed after Surface S1")
	if err != nil {
		return zero(err)
	}
	if err := surfaceS1.ValidateCurrent(exactSurface, surfaceNowS1); err != nil {
		return zero(err)
	}
	if surfaceS1.Manifest.Dialect != toolcontract.ModelToolDialectFunctionCallingV1 {
		return zero(core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownCapability, "Tool Surface Dialect is not supported by neutral function-tool injection"))
	}

	closures := make([]modelToolInjectionClosureV1, 0, len(surfaceS1.Manifest.Entries))
	expires := surfaceS1.ExpiresUnixNano
	created := surfaceS1.Manifest.CreatedUnixNano
	for _, entry := range surfaceS1.Manifest.Entries {
		if err := ctx.Err(); err != nil {
			return zero(err)
		}
		if entry.Visibility != toolcontract.SurfaceVisible || !entry.Allowed {
			return zero(core.NewError(core.ErrorPreconditionFailed, core.ReasonPlanInvalid, "Model Tool Injection requires every Surface entry to be visible and allowed"))
		}
		if err := toolcontract.ValidatePortableFunctionToolNameV1(entry.ModelName); err != nil {
			return zero(err)
		}
		definitionRef, err := toolcontract.DeriveToolDefinitionMaterialRefV1(entry.Tool, entry.InputSchema, entry.DescriptionDigest)
		if err != nil {
			return zero(err)
		}
		definition, err := definitions.InspectExactToolDefinitionMaterialV1(ctx, definitionRef)
		if err != nil {
			return zero(err)
		}
		if err := definition.Validate(); err != nil || definition.Ref != definitionRef {
			return zero(core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Tool Definition Material S1 does not bind its Surface entry"))
		}
		if err := toolcontract.ValidatePortableFunctionToolSchemaV1(definition.InputSchema); err != nil {
			return zero(err)
		}
		capability, capabilityCurrent, err := registry.ResolveExactToolCapabilityCurrentV1(ctx, entry.Capability)
		if err != nil {
			return zero(err)
		}
		capabilityNowS1, err := freshCompileTimeV2(clock, surfaceNowS1, "Model Tool Injection clock regressed after Capability S1")
		if err != nil {
			return zero(err)
		}
		if err := capabilityCurrent.ValidateAgainst(entry.Capability, capabilityCurrent.Ref, capabilityNowS1); err != nil {
			return zero(err)
		}
		tool, toolCurrent, err := registry.ResolveExactToolDescriptorCurrentV1(ctx, entry.Tool)
		if err != nil {
			return zero(err)
		}
		toolNowS1, err := freshCompileTimeV2(clock, capabilityNowS1, "Model Tool Injection clock regressed after Tool S1")
		if err != nil {
			return zero(err)
		}
		if err := toolCurrent.ValidateAgainst(entry.Tool, toolCurrent.Ref, toolNowS1); err != nil {
			return zero(err)
		}
		surfaceNowS1 = toolNowS1
		if err := validateModelToolInjectionDescriptorClosureV1(entry, capability, tool); err != nil {
			return zero(err)
		}
		expires = minimumUnixNanoV2(
			expires,
			capabilityCurrent.ExpiresUnixNano,
			capabilityCurrent.Source.UpdatedUnixNano+int64(toolcontract.MaxToolRegistryObjectCurrentTTLV1),
			toolCurrent.ExpiresUnixNano,
			toolCurrent.Source.UpdatedUnixNano+int64(toolcontract.MaxToolRegistryObjectCurrentTTLV1),
		)
		created = maximumUnixNanoV2(created, capabilityCurrent.Source.UpdatedUnixNano, toolCurrent.Source.UpdatedUnixNano)
		closures = append(closures, modelToolInjectionClosureV1{
			entry: entry, definition: definition, capability: capability, capabilityCurrent: capabilityCurrent,
			tool: tool, toolCurrent: toolCurrent,
		})
	}

	compiled := CompiledModelToolsV1{
		ContractVersion: CompiledModelToolsContractVersionV1,
		Surface:         exactSurface,
		Dialect:         string(surfaceS1.Manifest.Dialect),
		Tools:           make([]modelinvoker.Tool, 0, len(closures)),
	}
	for _, closure := range closures {
		strict := true
		compiled.Tools = append(compiled.Tools, modelinvoker.Tool{
			Name:        closure.entry.ModelName,
			Description: closure.definition.Description,
			Parameters:  append(json.RawMessage(nil), closure.definition.InputSchema...),
			Strict:      &strict,
		})
	}
	compiled.Digest, err = digestCompiledModelToolsV1(compiled)
	if err != nil {
		return zero(err)
	}

	surfaceS2, err := surfaces.InspectExactToolSurfaceManifestCurrentV1(ctx, exactSurface)
	if err != nil {
		return zero(err)
	}
	if !reflect.DeepEqual(surfaceS1, surfaceS2) {
		return zero(core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Tool Surface current drifted between S1 and S2"))
	}
	nowS2, err := freshCompileTimeV2(clock, surfaceNowS1, "Model Tool Injection clock regressed after Surface S2")
	if err != nil {
		return zero(err)
	}
	if err := surfaceS2.ValidateCurrent(exactSurface, nowS2); err != nil {
		return zero(err)
	}
	for index, s1 := range closures {
		definitionS2, err := definitions.InspectExactToolDefinitionMaterialV1(ctx, s1.definition.Ref)
		if err != nil {
			return zero(err)
		}
		if !reflect.DeepEqual(s1.definition, definitionS2) {
			return zero(core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Tool Definition Material drifted between S1 and S2"))
		}
		capabilityS2, capabilityCurrentS2, err := registry.InspectExactToolCapabilityCurrentV1(ctx, s1.entry.Capability, s1.capabilityCurrent.Ref)
		if err != nil {
			return zero(err)
		}
		capabilityNowS2, err := freshCompileTimeV2(clock, nowS2, "Model Tool Injection clock regressed after Capability S2")
		if err != nil {
			return zero(err)
		}
		toolS2, toolCurrentS2, err := registry.InspectExactToolDescriptorCurrentV1(ctx, s1.entry.Tool, s1.toolCurrent.Ref)
		if err != nil {
			return zero(err)
		}
		toolNowS2, err := freshCompileTimeV2(clock, capabilityNowS2, "Model Tool Injection clock regressed after Tool S2")
		if err != nil {
			return zero(err)
		}
		if !reflect.DeepEqual(s1.capability, capabilityS2) || !sameRegistryCurrentStableV2(s1.capabilityCurrent, capabilityCurrentS2) || !reflect.DeepEqual(s1.tool, toolS2) || !sameRegistryCurrentStableV2(s1.toolCurrent, toolCurrentS2) {
			return zero(core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Tool Registry current closure drifted between S1 and S2"))
		}
		if err := capabilityCurrentS2.ValidateAgainst(s1.entry.Capability, s1.capabilityCurrent.Ref, capabilityNowS2); err != nil {
			return zero(err)
		}
		if err := toolCurrentS2.ValidateAgainst(s1.entry.Tool, s1.toolCurrent.Ref, toolNowS2); err != nil {
			return zero(err)
		}
		nowS2 = toolNowS2
		expires = minimumUnixNanoV2(
			expires,
			capabilityCurrentS2.ExpiresUnixNano,
			capabilityCurrentS2.Source.UpdatedUnixNano+int64(toolcontract.MaxToolRegistryObjectCurrentTTLV1),
			toolCurrentS2.ExpiresUnixNano,
			toolCurrentS2.Source.UpdatedUnixNano+int64(toolcontract.MaxToolRegistryObjectCurrentTTLV1),
		)
		created = maximumUnixNanoV2(created, capabilityCurrentS2.Source.UpdatedUnixNano, toolCurrentS2.Source.UpdatedUnixNano)
		closures[index].definition = definitionS2
	}
	if err := ctx.Err(); err != nil {
		return zero(err)
	}
	final := clock()
	if final.IsZero() || final.Before(nowS2) {
		return zero(core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Model Tool Injection final clock regressed"))
	}
	if final.UnixNano() >= expires {
		return zero(core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Model Tool Injection closure expired before seal"))
	}
	if err := surfaceS2.ValidateCurrent(exactSurface, final); err != nil {
		return zero(err)
	}

	entries := make([]toolcontract.ModelToolInjectionEntryV1, 0, len(closures))
	for _, closure := range closures {
		entries = append(entries, toolcontract.ModelToolInjectionEntryV1{
			Order:                 closure.entry.Order,
			ModelName:             closure.entry.ModelName,
			CapabilityRef:         closure.entry.Capability,
			ToolRef:               closure.entry.Tool,
			DefinitionMaterialRef: closure.definition.Ref,
			InputSchemaRef:        closure.entry.InputSchema,
			DescriptionDigest:     closure.entry.DescriptionDigest,
			Strict:                true,
			Admission:             closure.entry.Admission,
			EffectKinds:           append([]runtimeports.NamespacedNameV2(nil), closure.entry.EffectKinds...),
			ReviewProfile:         closure.capability.ReviewProfile,
			AuthorityRequirement:  closure.capability.AuthorityRequirement,
			BudgetRequirement:     closure.capability.BudgetRequirement,
			SandboxRequirement:    closure.capability.SandboxRequirement,
			EvidenceRequirement:   closure.capability.EvidenceRequirement,
		})
	}
	material, err := toolcontract.SealModelToolInjectionMaterialV1(toolcontract.ModelToolInjectionMaterialV1{
		Surface:                 exactSurface,
		Entries:                 entries,
		ExpectedInjectionDigest: surfaceS2.Manifest.ExpectedInjectionDigest,
		CompiledToolsDigest:     compiled.Digest,
		CreatedUnixNano:         created,
		ExpiresUnixNano:         expires,
	})
	if err != nil {
		return zero(err)
	}
	if err := compiled.ValidateAgainstMaterialV1(material); err != nil {
		return zero(err)
	}
	return cloneCompiledModelToolsV1(compiled), material.Clone(), nil
}

func validateModelToolInjectionDescriptorClosureV1(entry toolcontract.ToolSurfaceEntry, capability toolcontract.CapabilityDescriptor, tool toolcontract.ToolDescriptor) error {
	capabilityRef := toolcontract.ObjectRef{ID: string(capability.ID), Revision: capability.Revision, Digest: capability.Digest}
	toolRef := toolcontract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest}
	if capabilityRef != entry.Capability || toolRef != entry.Tool || tool.ValidateAgainst(capability) != nil {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Tool Registry descriptors do not bind the exact Surface entry")
	}
	if tool.InputSchema != entry.InputSchema || !slices.Equal(tool.EffectKinds, entry.EffectKinds) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Tool Descriptor schema or Effects drifted from the Surface entry")
	}
	return nil
}

func dependencyUnavailableV2(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func minimumUnixNanoV2(values ...int64) int64 {
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

func maximumUnixNanoV2(values ...int64) int64 {
	maximum := values[0]
	for _, value := range values[1:] {
		if value > maximum {
			maximum = value
		}
	}
	return maximum
}

func sameRegistryCurrentStableV2(left, right toolcontract.ToolRegistryObjectCurrentProjectionV1) bool {
	return left.ContractVersion == right.ContractVersion &&
		left.Ref == right.Ref &&
		left.Source == right.Source &&
		left.Object == right.Object &&
		left.RegistryOwner == right.RegistryOwner
}

func freshCompileTimeV2(clock func() time.Time, previous time.Time, message string) (time.Time, error) {
	now := clock()
	if now.IsZero() || now.Before(previous) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, message)
	}
	return now, nil
}

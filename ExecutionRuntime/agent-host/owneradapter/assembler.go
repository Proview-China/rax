package owneradapter

import (
	"context"
	"reflect"
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	assemblerports "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/ports"
	definitioncontract "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	definitionports "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/ports"
	hostcontract "github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	hostports "github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
)

type AssemblerAdapterV1 struct {
	definitions definitionports.DefinitionCurrentReaderV1
	sources     hostports.DefinitionSourceCurrentReaderV1
	inputs      hostports.ResolutionInputsCurrentReaderV1
	assembler   assemblerports.AgentAssemblerPortV1
	catalog     definitioncontract.ValidationCatalogV1
	clock       func() time.Time
}

func NewAssemblerAdapterV1(definitions definitionports.DefinitionCurrentReaderV1, sources hostports.DefinitionSourceCurrentReaderV1, inputs hostports.ResolutionInputsCurrentReaderV1, assembler assemblerports.AgentAssemblerPortV1, catalog definitioncontract.ValidationCatalogV1, clock func() time.Time) *AssemblerAdapterV1 {
	return &AssemblerAdapterV1{definitions: definitions, sources: sources, inputs: inputs, assembler: assembler, catalog: cloneValidationCatalogV1(catalog), clock: clock}
}

func (a *AssemblerAdapterV1) ResolveAgentV1(ctx context.Context, config hostcontract.HostConfigV1, decoded hostcontract.DecodedDefinitionV1) (hostcontract.ResolvedAssemblyV1, error) {
	if a == nil {
		return hostcontract.ResolvedAssemblyV1{}, hostcontract.NewError(hostcontract.ErrorUnavailable, "assembler_adapter_unavailable", "assembler adapter is unavailable")
	}
	if ctx == nil {
		return hostcontract.ResolvedAssemblyV1{}, hostcontract.NewError(hostcontract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	if err := config.Validate(); err != nil {
		return hostcontract.ResolvedAssemblyV1{}, err
	}
	if err := decoded.Validate(); err != nil {
		return hostcontract.ResolvedAssemblyV1{}, err
	}
	dependencies := []struct {
		value  any
		reason string
	}{{a.definitions, "definition_reader_unavailable"}, {a.sources, "definition_source_reader_unavailable"}, {a.inputs, "resolution_inputs_reader_unavailable"}, {a.assembler, "assembler_unavailable"}}
	for _, dependency := range dependencies {
		if err := unavailableV1(dependency.value, dependency.reason); err != nil {
			return hostcontract.ResolvedAssemblyV1{}, err
		}
	}
	definitionRef, err := ownerDefinitionRefV1(decoded.Ref)
	if err != nil {
		return hostcontract.ResolvedAssemblyV1{}, err
	}
	now1, err := nowUnixNanoV1(a.clock)
	if err != nil {
		return hostcontract.ResolvedAssemblyV1{}, err
	}
	definitionCurrent1, err := a.sources.InspectDefinitionSourceCurrentV1(ctx, config.DefinitionSourceRef)
	if err != nil {
		return hostcontract.ResolvedAssemblyV1{}, ownerErrorV1(err, "definition_current_s1_failed")
	}
	if err := validateDefinitionSourceV1(definitionCurrent1, config.DefinitionSourceRef, now1); err != nil || definitionCurrent1.DefinitionExactRef != decoded.Ref {
		if err != nil {
			return hostcontract.ResolvedAssemblyV1{}, err
		}
		return hostcontract.ResolvedAssemblyV1{}, hostcontract.NewError(hostcontract.ErrorConflict, "definition_current_drift", "decoded definition is no longer current")
	}
	ownerDefinitionCurrent1, err := a.definitions.InspectCurrentDefinitionV1(ctx, definitionRef.DefinitionID, now1)
	if err != nil {
		return hostcontract.ResolvedAssemblyV1{}, ownerErrorV1(err, "definition_owner_current_s1_failed")
	}
	if err := validateDefinitionCurrentV1(ownerDefinitionCurrent1); err != nil {
		return hostcontract.ResolvedAssemblyV1{}, err
	}
	if ownerDefinitionCurrent1.Definition != definitionRef {
		return hostcontract.ResolvedAssemblyV1{}, hostcontract.NewError(hostcontract.ErrorConflict, "definition_owner_current_mismatch", "decoded definition is not the active owner current revision")
	}
	definition, err := a.definitions.InspectExactDefinitionV1(ctx, definitionRef)
	if err != nil {
		return hostcontract.ResolvedAssemblyV1{}, ownerErrorV1(err, "definition_exact_failed")
	}
	if definition.RefV1() != definitionRef {
		return hostcontract.ResolvedAssemblyV1{}, hostcontract.NewError(hostcontract.ErrorConflict, "definition_exact_splice", "definition exact reader returned another revision")
	}
	if err := definition.Validate(cloneValidationCatalogV1(a.catalog)); err != nil {
		return hostcontract.ResolvedAssemblyV1{}, ownerErrorV1(err, "definition_invalid")
	}
	inputs1, err := a.inputs.InspectResolutionInputsCurrentV1(ctx, config.CatalogRef, config.ResolutionFactsRef)
	if err != nil {
		return hostcontract.ResolvedAssemblyV1{}, ownerErrorV1(err, "resolution_inputs_current_s1_failed")
	}
	if err := validateResolutionInputsV1(inputs1, config, now1); err != nil {
		return hostcontract.ResolvedAssemblyV1{}, err
	}
	factsRef, err := exactFactsRefV1(inputs1.ResolutionFactsExactRef)
	if err != nil {
		return hostcontract.ResolvedAssemblyV1{}, err
	}
	catalogRef, err := exactCatalogRefV1(inputs1.CatalogExactRef)
	if err != nil {
		return hostcontract.ResolvedAssemblyV1{}, err
	}
	result, err := a.assembler.Resolve(ctx, assemblercontract.ResolveRequestV1{Definition: definition, FactsRef: factsRef, CatalogRef: catalogRef})
	if err != nil {
		return hostcontract.ResolvedAssemblyV1{}, ownerErrorV1(err, "assembler_resolve_failed")
	}
	now2, err := nowUnixNanoV1(a.clock)
	if err != nil {
		return hostcontract.ResolvedAssemblyV1{}, err
	}
	if now2 < now1 {
		return hostcontract.ResolvedAssemblyV1{}, hostcontract.NewError(hostcontract.ErrorConflict, "clock_regressed", "clock regressed during resolution")
	}
	inputs2, err := a.inputs.InspectResolutionInputsCurrentV1(ctx, config.CatalogRef, config.ResolutionFactsRef)
	if err != nil {
		return hostcontract.ResolvedAssemblyV1{}, ownerErrorV1(err, "resolution_inputs_current_s2_failed")
	}
	if err := validateResolutionInputsV1(inputs2, config, now2); err != nil {
		return hostcontract.ResolvedAssemblyV1{}, err
	}
	if inputs1 != inputs2 {
		return hostcontract.ResolvedAssemblyV1{}, hostcontract.NewError(hostcontract.ErrorConflict, "resolution_inputs_current_drift", "resolution inputs current projection changed during resolve")
	}
	definitionCurrent2, err := a.sources.InspectDefinitionSourceCurrentV1(ctx, config.DefinitionSourceRef)
	if err != nil {
		return hostcontract.ResolvedAssemblyV1{}, ownerErrorV1(err, "definition_current_s2_failed")
	}
	if err := validateDefinitionSourceV1(definitionCurrent2, config.DefinitionSourceRef, now2); err != nil {
		return hostcontract.ResolvedAssemblyV1{}, err
	}
	if definitionCurrent1 != definitionCurrent2 {
		return hostcontract.ResolvedAssemblyV1{}, hostcontract.NewError(hostcontract.ErrorConflict, "definition_current_drift", "definition current projection changed during resolve")
	}
	ownerDefinitionCurrent2, err := a.definitions.InspectCurrentDefinitionV1(ctx, definitionRef.DefinitionID, now2)
	if err != nil {
		return hostcontract.ResolvedAssemblyV1{}, ownerErrorV1(err, "definition_owner_current_s2_failed")
	}
	if err := validateDefinitionCurrentV1(ownerDefinitionCurrent2); err != nil {
		return hostcontract.ResolvedAssemblyV1{}, err
	}
	if ownerDefinitionCurrent1 != ownerDefinitionCurrent2 || ownerDefinitionCurrent2.Definition != definitionRef {
		return hostcontract.ResolvedAssemblyV1{}, hostcontract.NewError(hostcontract.ErrorConflict, "definition_owner_current_drift", "definition owner current projection changed during resolve")
	}
	finalNow, err := nowUnixNanoV1(a.clock)
	if err != nil {
		return hostcontract.ResolvedAssemblyV1{}, err
	}
	if finalNow < now2 || finalNow < definition.EffectiveWindow.NotBeforeUnixNano || finalNow >= definition.EffectiveWindow.NotAfterUnixNano {
		return hostcontract.ResolvedAssemblyV1{}, hostcontract.NewError(hostcontract.ErrorPrecondition, "definition_not_effective", "definition is not effective after resolution")
	}
	if err := validateDefinitionSourceV1(definitionCurrent2, config.DefinitionSourceRef, finalNow); err != nil {
		return hostcontract.ResolvedAssemblyV1{}, err
	}
	if err := validateResolutionInputsV1(inputs2, config, finalNow); err != nil {
		return hostcontract.ResolvedAssemblyV1{}, err
	}
	if err := result.Plan.Validate(); err != nil {
		return hostcontract.ResolvedAssemblyV1{}, ownerErrorV1(err, "resolved_plan_invalid")
	}
	if err := result.AssemblyInput.Validate(); err != nil {
		return hostcontract.ResolvedAssemblyV1{}, ownerErrorV1(err, "assembly_input_invalid")
	}
	if result.Plan.DefinitionRef != definitionRef || result.Plan.ResolutionFactsRef != factsRef || result.Plan.CatalogRef != catalogRef || !reflect.DeepEqual(result.BindingPlan, result.Plan.BindingPlan) || result.AssemblyInput.Plan.ResolvedAgentPlan.ID != result.Plan.PlanID || result.AssemblyInput.Plan.ResolvedAgentPlan.Revision != result.Plan.Revision || result.AssemblyInput.Plan.ResolvedAgentPlan.Digest != result.Plan.Digest {
		return hostcontract.ResolvedAssemblyV1{}, hostcontract.NewError(hostcontract.ErrorConflict, "assembler_result_splice", "assembler result does not preserve exact owner inputs")
	}
	return hostcontract.ResolvedAssemblyV1{PlanRef: planRefV1(result.Plan.RefV1()), InputRef: inputRefV1(result.AssemblyInput)}, nil
}

func validateResolutionInputsV1(value hostcontract.ResolutionInputsCurrentV1, config hostcontract.HostConfigV1, now int64) error {
	if err := value.Validate(now); err != nil {
		return err
	}
	if value.CatalogStableID != config.CatalogRef || value.ResolutionFactsStableID != config.ResolutionFactsRef {
		return hostcontract.NewError(hostcontract.ErrorConflict, "resolution_inputs_alias_drift", "resolution inputs projection stable ids differ from configuration")
	}
	return nil
}

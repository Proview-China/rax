package owneradapter

import (
	"context"
	"reflect"
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/mapper"
	assemblerports "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/ports"
	hostcontract "github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	hostports "github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycompiler"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
)

type CompilerAdapterV1 struct {
	plans    assemblerports.ResolvedAgentPlanExactReaderV1
	facts    assemblerports.ResolutionFactsReaderV1
	catalogs assemblerports.ComponentReleaseCatalogReaderV1
	inputs   hostports.ResolutionInputsCurrentReaderV1
	clock    func() time.Time
}

func NewCompilerAdapterV1(plans assemblerports.ResolvedAgentPlanExactReaderV1, facts assemblerports.ResolutionFactsReaderV1, catalogs assemblerports.ComponentReleaseCatalogReaderV1, inputs hostports.ResolutionInputsCurrentReaderV1, clock func() time.Time) *CompilerAdapterV1 {
	return &CompilerAdapterV1{plans: plans, facts: facts, catalogs: catalogs, inputs: inputs, clock: clock}
}

func (a *CompilerAdapterV1) CompileHarnessV1(ctx context.Context, config hostcontract.HostConfigV1, resolved hostcontract.ResolvedAssemblyV1) (hostcontract.CompiledAssemblyV1, error) {
	return a.compileHarnessV2(ctx, config, resolved, nil, nil)
}

func (a *CompilerAdapterV1) CompileHarnessArtifactsV2(ctx context.Context, config hostcontract.HostConfigV1, resolved hostcontract.ResolvedAssemblyV1) (hostcontract.CompiledAssemblyArtifactsV2, error) {
	var captured assemblycontract.CompileResultV1
	var capturedInput assemblycontract.AssemblyInputV1
	var checked, expires int64
	compiled, err := a.compileHarnessV2(ctx, config, resolved, func(input assemblycontract.AssemblyInputV1, value assemblycontract.CompileResultV1) {
		capturedInput, captured = input, value
	}, func(c, e int64) { checked, expires = c, e })
	if err != nil {
		return hostcontract.CompiledAssemblyArtifactsV2{}, err
	}
	return hostcontract.SealCompiledAssemblyArtifactsV2(hostcontract.CompiledAssemblyArtifactsV2{ScopeRef: capturedInput.ScopeRef, InputRef: resolved.InputRef, Compiled: compiled, Harness: captured, CheckedUnixNano: checked, ExpiresUnixNano: expires})
}

func (a *CompilerAdapterV1) compileHarnessV2(ctx context.Context, config hostcontract.HostConfigV1, resolved hostcontract.ResolvedAssemblyV1, capture func(assemblycontract.AssemblyInputV1, assemblycontract.CompileResultV1), window func(int64, int64)) (hostcontract.CompiledAssemblyV1, error) {
	if a == nil {
		return hostcontract.CompiledAssemblyV1{}, hostcontract.NewError(hostcontract.ErrorUnavailable, "compiler_adapter_unavailable", "compiler adapter is unavailable")
	}
	if ctx == nil {
		return hostcontract.CompiledAssemblyV1{}, hostcontract.NewError(hostcontract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	if err := config.Validate(); err != nil {
		return hostcontract.CompiledAssemblyV1{}, err
	}
	if err := resolved.Validate(); err != nil {
		return hostcontract.CompiledAssemblyV1{}, err
	}
	dependencies := []struct {
		value  any
		reason string
	}{{a.plans, "plan_reader_unavailable"}, {a.facts, "facts_reader_unavailable"}, {a.catalogs, "catalog_reader_unavailable"}, {a.inputs, "resolution_inputs_reader_unavailable"}}
	for _, dependency := range dependencies {
		if err := unavailableV1(dependency.value, dependency.reason); err != nil {
			return hostcontract.CompiledAssemblyV1{}, err
		}
	}
	planRef, err := ownerPlanRefV1(resolved.PlanRef)
	if err != nil {
		return hostcontract.CompiledAssemblyV1{}, err
	}
	now1, err := nowUnixNanoV1(a.clock)
	if err != nil {
		return hostcontract.CompiledAssemblyV1{}, err
	}
	inputs1, err := a.inputs.InspectResolutionInputsCurrentV1(ctx, config.CatalogRef, config.ResolutionFactsRef)
	if err != nil {
		return hostcontract.CompiledAssemblyV1{}, ownerErrorV1(err, "resolution_inputs_current_s1_failed")
	}
	if err := validateResolutionInputsV1(inputs1, config, now1); err != nil {
		return hostcontract.CompiledAssemblyV1{}, err
	}
	plan, err := a.plans.InspectExactResolvedAgentPlanV1(ctx, planRef)
	if err != nil {
		return hostcontract.CompiledAssemblyV1{}, ownerErrorV1(err, "plan_exact_failed")
	}
	if plan.RefV1() != planRef {
		return hostcontract.CompiledAssemblyV1{}, hostcontract.NewError(hostcontract.ErrorConflict, "plan_exact_splice", "plan reader returned another exact plan")
	}
	if err := plan.Validate(); err != nil {
		return hostcontract.CompiledAssemblyV1{}, ownerErrorV1(err, "plan_invalid")
	}
	factsRef, err := exactFactsRefV1(inputs1.ResolutionFactsExactRef)
	if err != nil {
		return hostcontract.CompiledAssemblyV1{}, err
	}
	catalogRef, err := exactCatalogRefV1(inputs1.CatalogExactRef)
	if err != nil {
		return hostcontract.CompiledAssemblyV1{}, err
	}
	if plan.ResolutionFactsRef != factsRef || plan.CatalogRef != catalogRef {
		return hostcontract.CompiledAssemblyV1{}, hostcontract.NewError(hostcontract.ErrorConflict, "plan_input_splice", "plan does not bind the current exact resolution inputs")
	}
	facts, err := a.facts.InspectExactResolutionFactsV1(ctx, factsRef)
	if err != nil {
		return hostcontract.CompiledAssemblyV1{}, ownerErrorV1(err, "facts_exact_failed")
	}
	if err := facts.Validate(); err != nil {
		return hostcontract.CompiledAssemblyV1{}, ownerErrorV1(err, "facts_invalid")
	}
	if facts.RefV1() != factsRef || facts.DefinitionRef != plan.DefinitionRef {
		return hostcontract.CompiledAssemblyV1{}, hostcontract.NewError(hostcontract.ErrorConflict, "facts_exact_splice", "resolution facts differ from plan identity")
	}
	catalog, err := a.catalogs.InspectExactComponentReleaseCatalogV1(ctx, catalogRef)
	if err != nil {
		return hostcontract.CompiledAssemblyV1{}, ownerErrorV1(err, "catalog_exact_failed")
	}
	if err := catalog.Validate(); err != nil {
		return hostcontract.CompiledAssemblyV1{}, ownerErrorV1(err, "catalog_invalid")
	}
	if catalog.RefV1() != catalogRef {
		return hostcontract.CompiledAssemblyV1{}, hostcontract.NewError(hostcontract.ErrorConflict, "catalog_exact_splice", "catalog reader returned another exact catalog")
	}
	if now1 < plan.CreatedUnixNano || now1 < facts.FrozenUnixNano || now1 < catalog.CheckedUnixNano || now1 >= plan.ValidUntilUnixNano || now1 >= facts.ExpiresUnixNano || now1 >= catalog.ExpiresUnixNano {
		return hostcontract.CompiledAssemblyV1{}, hostcontract.NewError(hostcontract.ErrorPrecondition, "assembly_owner_input_stale", "plan, facts, or catalog is stale or observed in the future")
	}
	releases, err := selectedReleasesV1(plan, catalog)
	if err != nil {
		return hostcontract.CompiledAssemblyV1{}, err
	}
	input, err := mapper.AssemblyInputV1(plan, facts, releases)
	if err != nil {
		return hostcontract.CompiledAssemblyV1{}, ownerErrorV1(err, "assembly_input_rebuild_failed")
	}
	if inputRefV1(input) != resolved.InputRef {
		return hostcontract.CompiledAssemblyV1{}, hostcontract.NewError(hostcontract.ErrorConflict, "assembly_input_splice", "rebuilt assembly input differs from resolved exact input")
	}
	compileResult, err := assemblycompiler.New().Compile(input)
	if err != nil {
		return hostcontract.CompiledAssemblyV1{}, ownerErrorV1(err, "harness_compile_failed")
	}
	if compileResult.Generation == nil || compileResult.Manifest == nil || compileResult.Graph == nil || compileResult.Handoff == nil {
		return hostcontract.CompiledAssemblyV1{}, hostcontract.NewError(hostcontract.ErrorPrecondition, "harness_compile_incomplete", "real Harness compiler did not seal every artifact")
	}
	generation, manifest, graph, handoff := *compileResult.Generation, *compileResult.Manifest, *compileResult.Graph, *compileResult.Handoff
	if err := validateCompileResultV1(input, generation, manifest, graph, handoff); err != nil {
		return hostcontract.CompiledAssemblyV1{}, err
	}
	hostGraph, err := constructionGraphV1(manifest, graph, generation)
	if err != nil {
		return hostcontract.CompiledAssemblyV1{}, err
	}
	now2, err := nowUnixNanoV1(a.clock)
	if err != nil {
		return hostcontract.CompiledAssemblyV1{}, err
	}
	if now2 < now1 || now2 >= plan.ValidUntilUnixNano || now2 >= facts.ExpiresUnixNano || now2 >= catalog.ExpiresUnixNano {
		return hostcontract.CompiledAssemblyV1{}, hostcontract.NewError(hostcontract.ErrorPrecondition, "assembly_owner_input_expired", "owner input expired or clock regressed during compilation")
	}
	inputs2, err := a.inputs.InspectResolutionInputsCurrentV1(ctx, config.CatalogRef, config.ResolutionFactsRef)
	if err != nil {
		return hostcontract.CompiledAssemblyV1{}, ownerErrorV1(err, "resolution_inputs_current_s2_failed")
	}
	if err := validateResolutionInputsV1(inputs2, config, now2); err != nil {
		return hostcontract.CompiledAssemblyV1{}, err
	}
	if inputs1 != inputs2 {
		return hostcontract.CompiledAssemblyV1{}, hostcontract.NewError(hostcontract.ErrorConflict, "resolution_inputs_current_drift", "resolution inputs current projection changed during compilation")
	}
	finalNow, err := nowUnixNanoV1(a.clock)
	if err != nil {
		return hostcontract.CompiledAssemblyV1{}, err
	}
	if finalNow < now2 || finalNow >= plan.ValidUntilUnixNano || finalNow >= facts.ExpiresUnixNano || finalNow >= catalog.ExpiresUnixNano {
		return hostcontract.CompiledAssemblyV1{}, hostcontract.NewError(hostcontract.ErrorPrecondition, "assembly_owner_input_expired", "owner input expired or clock regressed after compilation")
	}
	if err := validateResolutionInputsV1(inputs2, config, finalNow); err != nil {
		return hostcontract.CompiledAssemblyV1{}, err
	}
	result := hostcontract.CompiledAssemblyV1{GenerationRef: generationRefV1(generation), ManifestRef: artifactRefV1(ManifestKindV1, generation.GenerationID+"/manifest", uint64(generation.Revision), manifest.Digest), Graph: hostGraph, HandoffRef: artifactRefV1(HandoffKindV1, generation.GenerationID+"/handoff", uint64(generation.Revision), handoff.Digest)}
	if err := result.Validate(); err != nil {
		return hostcontract.CompiledAssemblyV1{}, err
	}
	if capture != nil {
		capture(input, compileResult)
	}
	if window != nil {
		expires := plan.ValidUntilUnixNano
		if facts.ExpiresUnixNano < expires {
			expires = facts.ExpiresUnixNano
		}
		if catalog.ExpiresUnixNano < expires {
			expires = catalog.ExpiresUnixNano
		}
		if inputs2.ExpiresUnixNano < expires {
			expires = inputs2.ExpiresUnixNano
		}
		window(finalNow, expires)
	}
	return result, nil
}

func selectedReleasesV1(plan assemblercontract.ResolvedAgentPlanV1, catalog assemblercontract.ComponentReleaseCatalogSnapshotV1) ([]assemblercontract.ComponentReleaseV1, error) {
	byRef := make(map[assemblercontract.ComponentReleaseRefV1]assemblercontract.ComponentReleaseV1, len(catalog.Releases))
	for _, release := range catalog.Releases {
		if _, ok := byRef[release.RefV1()]; ok {
			return nil, hostcontract.NewError(hostcontract.ErrorConflict, "catalog_release_alias", "catalog contains duplicate exact release")
		}
		byRef[release.RefV1()] = release
	}
	result := make([]assemblercontract.ComponentReleaseV1, 0, len(plan.ComponentReleases))
	for _, selected := range plan.ComponentReleases {
		release, ok := byRef[selected.ReleaseRef]
		if !ok {
			return nil, hostcontract.NewError(hostcontract.ErrorPrecondition, "selected_release_missing", "plan selected release is absent from exact catalog")
		}
		if !reflect.DeepEqual(release.ComponentManifest, selected.Manifest) {
			return nil, hostcontract.NewError(hostcontract.ErrorConflict, "selected_release_manifest_splice", "plan manifest differs from exact catalog release")
		}
		result = append(result, release)
	}
	return result, nil
}

func validateCompileResultV1(input assemblycontract.AssemblyInputV1, generation assemblycontract.AssemblyGenerationV1, manifest assemblycontract.AssemblyManifestV1, graph assemblycontract.CompiledHarnessGraphV1, handoff assemblycontract.AssemblyHandoffV1) error {
	manifestDigest, err := assemblycontract.ManifestDigestV1(manifest)
	if err != nil || manifestDigest != manifest.Digest {
		return hostcontract.NewError(hostcontract.ErrorConflict, "manifest_digest_drift", "Harness manifest digest drifted")
	}
	graphDigest, err := assemblycontract.GraphDigestV1(graph)
	if err != nil || graphDigest != graph.Digest {
		return hostcontract.NewError(hostcontract.ErrorConflict, "graph_digest_drift", "Harness graph digest drifted")
	}
	generationDigest, err := assemblycontract.GenerationDigestV1(generation)
	if err != nil || generationDigest != generation.Digest {
		return hostcontract.NewError(hostcontract.ErrorConflict, "generation_digest_drift", "Harness generation digest drifted")
	}
	handoffDigest, err := assemblycontract.HandoffDigestV1(handoff)
	if err != nil || handoffDigest != handoff.Digest {
		return hostcontract.NewError(hostcontract.ErrorConflict, "handoff_digest_drift", "Harness handoff digest drifted")
	}
	if generation.State != assemblycontract.AssemblyStateSealedV1 || generation.InputDigest != input.Digest || manifest.InputDigest != input.Digest || graph.InputDigest != input.Digest || generation.ManifestDigest != manifest.Digest || generation.GraphDigest != graph.Digest || handoff.GenerationRef.ID != generation.GenerationID || handoff.GenerationRef.Revision != generation.Revision || handoff.GenerationRef.Digest != generation.Digest || handoff.ManifestDigest != manifest.Digest || handoff.GraphDigest != graph.Digest {
		return hostcontract.NewError(hostcontract.ErrorConflict, "harness_artifact_splice", "Harness sealed artifact linkage drifted")
	}
	if err := handoff.Validate(); err != nil {
		return ownerErrorV1(err, "handoff_invalid")
	}
	return nil
}

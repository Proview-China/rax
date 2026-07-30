package compiler

import (
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/conformance"
	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	packagecontract "github.com/Proview-China/rax/ExecutionRuntime/agent-builder/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycompiler"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type CompilerV1 struct{ harness assemblycompiler.Compiler }

func NewV1() CompilerV1 { return CompilerV1{harness: assemblycompiler.New()} }

// Compile consumes only the existing sealed Assembler result.
// Every lock coordinate is derived from that validated closure; callers cannot
// inject a parallel list of releases, facts, catalog or binding digests.
func (c CompilerV1) Compile(result assemblercontract.ResolveResultV1) (packagecontract.AgentPackageV1, error) {
	if _, err := conformance.CheckResolveResultV1(result); err != nil {
		return packagecontract.AgentPackageV1{}, err
	}
	if !reflect.DeepEqual(result.BindingPlan, result.Plan.BindingPlan) || !reflect.DeepEqual(result.AssemblyInput.Plan, result.Plan.AssemblyPlanRefs) {
		return packagecontract.AgentPackageV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "resolve result is not one exact Definition-to-Assembly closure")
	}
	planRef := result.Plan.RefV1()
	assemblyPlanRef := result.AssemblyInput.Plan.ResolvedAgentPlan
	if assemblyPlanRef.ID != planRef.PlanID || assemblyPlanRef.Revision != planRef.Revision || assemblyPlanRef.Digest != planRef.Digest {
		return packagecontract.AgentPackageV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "assembly input does not bind the exact resolved plan")
	}
	if result.AssemblyInput.CreatedUnixNano != result.Plan.CreatedUnixNano {
		return packagecontract.AgentPackageV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "assembly input frozen time differs from the resolved plan")
	}
	refs := make([]assemblercontract.ComponentReleaseRefV1, 0, len(result.Plan.ComponentReleases))
	for _, component := range result.Plan.ComponentReleases {
		refs = append(refs, component.ReleaseRef)
	}
	compiled, err := c.harness.Compile(result.AssemblyInput)
	if err != nil {
		return packagecontract.AgentPackageV1{}, err
	}
	if compiled.Generation == nil || compiled.Manifest == nil || compiled.Graph == nil || compiled.Handoff == nil {
		return packagecontract.AgentPackageV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonPlanInvalid, "Harness compiler did not return a complete sealed artifact set")
	}
	publicationBundle, err := assemblycontract.NewAssemblyPublicationBundleV2(result.AssemblyInput.ScopeRef, compiled)
	if err != nil {
		return packagecontract.AgentPackageV1{}, err
	}
	publication := publicationBundle.Publication
	lock, err := packagecontract.SealLockManifestV1(packagecontract.AgentPackageLockManifestV1{
		DefinitionRef: result.Plan.DefinitionRef, ResolvedPlanRef: planRef, ResolutionFactsRef: result.Plan.ResolutionFactsRef, CatalogRef: result.Plan.CatalogRef,
		ComponentReleaseRefs: refs, BindingPlanDigest: result.BindingPlan.PlanDigest, AssemblyInputDigest: result.AssemblyInput.Digest, FrozenUnixNano: result.AssemblyInput.CreatedUnixNano,
		HarnessCompilerVersion: assemblycontract.CompilerVersionV1,
		PublicationRef:         assemblycontract.AssemblyPublicationRefV2{PublicationID: publication.PublicationID, Revision: publication.Revision, Digest: publication.Digest},
		GenerationRef:          publication.Artifacts.Generation,
		ManifestRef:            publication.Artifacts.Manifest,
		GraphRef:               publication.Artifacts.Graph,
		HandoffRef:             publication.Artifacts.Handoff,
	})
	if err != nil {
		return packagecontract.AgentPackageV1{}, err
	}
	return packagecontract.SealPackageV1(packagecontract.AgentPackageV1{Lock: lock})
}

package owneradapter

import (
	"errors"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	definitioncontract "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	DefinitionKindV1    = "praxis.agent-definition/definition"
	PlanKindV1          = "praxis.agent-assembler/resolved-plan"
	FactsKindV1         = "praxis.agent-assembler/resolution-facts"
	CatalogKindV1       = "praxis.agent-assembler/component-release-catalog"
	AssemblyInputKindV1 = "praxis.harness/assembly-input"
	GenerationKindV1    = "praxis.harness/assembly-generation"
	ManifestKindV1      = "praxis.harness/assembly-manifest"
	GraphKindV1         = "praxis.harness/compiled-graph"
	HandoffKindV1       = "praxis.harness/assembly-handoff"
)

func definitionRefV1(value definitioncontract.AgentDefinitionRefV1) contract.ExactRefV1 {
	return contract.ExactRefV1{Kind: DefinitionKindV1, ID: value.DefinitionID, Revision: uint64(value.Revision), Digest: contract.DigestV1(value.Digest)}
}
func planRefV1(value assemblercontract.ResolvedAgentPlanRefV1) contract.ExactRefV1 {
	return contract.ExactRefV1{Kind: PlanKindV1, ID: value.PlanID, Revision: uint64(value.Revision), Digest: contract.DigestV1(value.Digest)}
}
func inputRefV1(value assemblycontract.AssemblyInputV1) contract.ExactRefV1 {
	return contract.ExactRefV1{Kind: AssemblyInputKindV1, ID: value.InputID, Revision: uint64(value.Revision), Digest: contract.DigestV1(value.Digest)}
}
func generationRefV1(value assemblycontract.AssemblyGenerationV1) contract.ExactRefV1 {
	return contract.ExactRefV1{Kind: GenerationKindV1, ID: value.GenerationID, Revision: uint64(value.Revision), Digest: contract.DigestV1(value.Digest)}
}
func artifactRefV1(kind, id string, revision uint64, digest core.Digest) contract.ExactRefV1 {
	return contract.ExactRefV1{Kind: kind, ID: id, Revision: revision, Digest: contract.DigestV1(digest)}
}

func exactFactsRefV1(ref contract.ExactRefV1) (assemblercontract.ResolutionFactsRefV1, error) {
	if ref.Kind != FactsKindV1 {
		return assemblercontract.ResolutionFactsRefV1{}, contract.NewError(contract.ErrorPrecondition, "facts_exact_ref_kind_invalid", "resolution facts exact ref kind is unsupported")
	}
	if err := ref.Validate(); err != nil {
		return assemblercontract.ResolutionFactsRefV1{}, err
	}
	value := assemblercontract.ResolutionFactsRefV1{FactsID: ref.ID, Revision: core.Revision(ref.Revision), Digest: core.Digest(ref.Digest)}
	if err := value.Validate(); err != nil {
		return assemblercontract.ResolutionFactsRefV1{}, ownerErrorV1(err, "facts_ref_invalid")
	}
	return value, nil
}
func exactCatalogRefV1(ref contract.ExactRefV1) (assemblercontract.ComponentReleaseCatalogRefV1, error) {
	if ref.Kind != CatalogKindV1 {
		return assemblercontract.ComponentReleaseCatalogRefV1{}, contract.NewError(contract.ErrorPrecondition, "catalog_exact_ref_kind_invalid", "component catalog exact ref kind is unsupported")
	}
	if err := ref.Validate(); err != nil {
		return assemblercontract.ComponentReleaseCatalogRefV1{}, err
	}
	value := assemblercontract.ComponentReleaseCatalogRefV1{CatalogID: ref.ID, Revision: core.Revision(ref.Revision), Digest: core.Digest(ref.Digest)}
	if err := value.Validate(); err != nil {
		return assemblercontract.ComponentReleaseCatalogRefV1{}, ownerErrorV1(err, "catalog_ref_invalid")
	}
	return value, nil
}
func ownerDefinitionRefV1(value contract.ExactRefV1) (definitioncontract.AgentDefinitionRefV1, error) {
	if value.Kind != DefinitionKindV1 {
		return definitioncontract.AgentDefinitionRefV1{}, contract.NewError(contract.ErrorConflict, "definition_ref_kind_drift", "definition exact ref kind is unsupported")
	}
	result := definitioncontract.AgentDefinitionRefV1{DefinitionID: value.ID, Revision: core.Revision(value.Revision), Digest: core.Digest(value.Digest)}
	if err := result.Validate(); err != nil {
		return definitioncontract.AgentDefinitionRefV1{}, ownerErrorV1(err, "definition_ref_invalid")
	}
	return result, nil
}
func ownerPlanRefV1(value contract.ExactRefV1) (assemblercontract.ResolvedAgentPlanRefV1, error) {
	if value.Kind != PlanKindV1 {
		return assemblercontract.ResolvedAgentPlanRefV1{}, contract.NewError(contract.ErrorConflict, "plan_ref_kind_drift", "plan exact ref kind is unsupported")
	}
	result := assemblercontract.ResolvedAgentPlanRefV1{PlanID: value.ID, Revision: core.Revision(value.Revision), Digest: core.Digest(value.Digest)}
	if err := result.Validate(); err != nil {
		return assemblercontract.ResolvedAgentPlanRefV1{}, ownerErrorV1(err, "plan_ref_invalid")
	}
	return result, nil
}

func ownerErrorV1(err error, reason string) error {
	if err == nil {
		return nil
	}
	code := contract.ErrorPrecondition
	switch {
	case core.HasCategory(err, core.ErrorInvalidArgument):
		code = contract.ErrorInvalidArgument
	case core.HasCategory(err, core.ErrorNotFound):
		code = contract.ErrorNotFound
	case core.HasCategory(err, core.ErrorConflict):
		code = contract.ErrorConflict
	case core.HasCategory(err, core.ErrorUnavailable), core.HasCategory(err, core.ErrorCapabilityUnavailable), core.HasCategory(err, core.ErrorRateLimited):
		code = contract.ErrorUnavailable
	case core.HasCategory(err, core.ErrorIndeterminate):
		code = contract.ErrorUnknownOutcome
	}
	return errors.Join(contract.NewError(code, reason, "owner public contract rejected the exact adapter operation"), err)
}

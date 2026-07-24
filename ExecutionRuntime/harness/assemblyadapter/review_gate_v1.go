package assemblyadapter

import (
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	ReviewGateModuleRefV1       = "praxis.review/gate-v1"
	ReviewGatePortSpecRefV1     = "praxis.review/gate-v1/read-current"
	ReviewGateContributionRefV1 = "praxis.review/gate-v1/slot-owner"
	ReviewGatePhaseRefV1        = "praxis.review/gate-v1/action-review"
)

// ReviewGateAssemblyContributionV1 reuses the live review.gate Slot and
// action.review HookFace. It does not register a new Slot, Phase, or root.
func ReviewGateAssemblyContributionV1() (assemblycontract.PortSpecV1, assemblycontract.SlotContributionV1, assemblycontract.PhaseContributionV1, error) {
	port := assemblycontract.PortSpecV1{
		ContractVersion:  assemblycontract.ContractVersionV1,
		PortID:           ReviewGatePortSpecRefV1,
		OwnerCapability:  runtimeports.CapabilityNameV2("praxis.review/gate"),
		RequestSchema:    contract.ReviewGateRequestSchemaV1(),
		ResponseSchema:   contract.ReviewGateResultSchemaV1(),
		OperationClass:   "read-only-gate",
		Idempotency:      "exact-read-s1-s2",
		FailureSemantics: "fail-closed-defer",
		Compatibility:    assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"},
	}
	if err := port.Validate(); err != nil {
		return assemblycontract.PortSpecV1{}, assemblycontract.SlotContributionV1{}, assemblycontract.PhaseContributionV1{}, err
	}
	slot := assemblycontract.SlotContributionV1{
		ContractVersion: assemblycontract.ContractVersionV1,
		ContributionID:  ReviewGateContributionRefV1,
		ModuleRef:       ReviewGateModuleRefV1,
		SlotRef:         "review.gate",
		Kind:            assemblycontract.SlotContributionOwnerV1,
		CapabilityRef:   runtimeports.CapabilityNameV2("praxis.review/gate"),
		PortSpecRef:     ReviewGatePortSpecRefV1,
	}
	var err error
	slot.Digest, err = assemblycontract.SlotContributionDigestV1(slot)
	if err != nil {
		return assemblycontract.PortSpecV1{}, assemblycontract.SlotContributionV1{}, assemblycontract.PhaseContributionV1{}, err
	}
	if err := slot.Validate(); err != nil {
		return assemblycontract.PortSpecV1{}, assemblycontract.SlotContributionV1{}, assemblycontract.PhaseContributionV1{}, err
	}
	hook, found := reviewGateHookFaceV1()
	if !found {
		return assemblycontract.PortSpecV1{}, assemblycontract.SlotContributionV1{}, assemblycontract.PhaseContributionV1{}, core.NewError(core.ErrorNotFound, core.ReasonUnknownCapability, "action.review Gate HookFace is absent from the live catalog")
	}
	phase := assemblycontract.PhaseContributionV1{
		ContractVersion:      assemblycontract.ContractVersionV1,
		ContributionID:       ReviewGatePhaseRefV1,
		HookFaceRef:          hook.HookFaceID,
		HandlerDescriptorRef: assemblycontract.ObjectRefV1{ID: "praxis.review/gate-v1/handler", Revision: 1, Digest: core.DigestBytes([]byte("praxis.review/gate-v1/handler"))},
		ModuleRef:            ReviewGateModuleRefV1,
		Capability:           assemblycontract.PhaseGateV1,
		Dependencies:         []string{ReviewGateContributionRefV1},
		Async:                false,
	}
	phase.Digest, err = assemblycontract.PhaseContributionDigestV1(phase)
	if err != nil {
		return assemblycontract.PortSpecV1{}, assemblycontract.SlotContributionV1{}, assemblycontract.PhaseContributionV1{}, err
	}
	if err := phase.Validate(); err != nil {
		return assemblycontract.PortSpecV1{}, assemblycontract.SlotContributionV1{}, assemblycontract.PhaseContributionV1{}, err
	}
	return port, slot, phase, nil
}

func reviewGateHookFaceV1() (assemblycontract.HookFaceSpecV1, bool) {
	for _, hook := range assemblycontract.HookFaceCatalogV1() {
		if hook.PhaseID == contract.ReviewGatePhaseIDV1 && hook.Kind == assemblycontract.PhaseGateV1 {
			return hook, true
		}
	}
	return assemblycontract.HookFaceSpecV1{}, false
}

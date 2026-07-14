package conformance

import (
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type RunAssemblerSubjectV3 string

const (
	RunAssemblerHostControlPlaneV3 RunAssemblerSubjectV3 = "host_control_plane"
	RunAssemblerApplicationV3      RunAssemblerSubjectV3 = "application"
	RunAssemblerComponentAdapterV3 RunAssemblerSubjectV3 = "component_adapter"
	RunAssemblerTestFixtureV3      RunAssemblerSubjectV3 = "test_fixture"
)

type RunAssemblerAccessReportV3 struct {
	HostInjected                       bool `json:"host_injected"`
	AssemblerEligible                  bool `json:"assembler_eligible"`
	ProductionPlanEligible             bool `json:"production_plan_eligible"`
	AssemblerCarriesCertificationProof bool `json:"assembler_carries_certification_proof"`
}

// CheckTrustedRunAssemblerAccessV3 documents the assembler-only boundary.
// Plan Admission exists as a separate Runtime owner; this report never carries
// or manufactures that owner's certification proof.
func CheckTrustedRunAssemblerAccessV3(subject RunAssemblerSubjectV3, hostInjected bool) (RunAssemblerAccessReportV3, error) {
	report := RunAssemblerAccessReportV3{HostInjected: hostInjected, AssemblerCarriesCertificationProof: false}
	if subject != RunAssemblerHostControlPlaneV3 || !hostInjected {
		return report, core.NewError(core.ErrorForbidden, core.ReasonEffectAuthorizationMissing, "trusted Run assembler is restricted to host-control-plane injection")
	}
	report.AssemblerEligible = true
	// Assembly alone is never production eligibility; the caller must obtain a
	// separate current Plan Admission certification.
	report.ProductionPlanEligible = false
	return report, nil
}

package releasecandidate

import (
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
)

type ConformanceReportV1 struct {
	ReleaseValid            bool
	DescriptorClosureValid  bool
	ReadinessValid          bool
	SupportMode             assemblercontract.SupportModeV1
	ProductionClaimEligible bool
	MissingProductionProofs []ProofRequirementV1
}

func InspectConformanceV1(candidate CandidateV1, now time.Time) (ConformanceReportV1, error) {
	if err := candidate.ValidateCurrentV1(now); err != nil {
		return ConformanceReportV1{}, err
	}
	report := ConformanceReportV1{ReleaseValid: true, DescriptorClosureValid: len(candidate.Release.ModuleDescriptors) == 1 && len(candidate.Release.CapabilityDescriptors) == 1 && len(candidate.Release.PortSpecs) == 1 && len(candidate.Release.FactoryDescriptors) == 1, ReadinessValid: true, SupportMode: candidate.Release.SupportMode, MissingProductionProofs: append([]ProofRequirementV1(nil), candidate.Readiness.MissingProductionProofs...)}
	report.ProductionClaimEligible = candidate.Readiness.ProductionEligible && len(report.MissingProductionProofs) == 0 && report.SupportMode == assemblercontract.SupportProductionV1
	return report, nil
}

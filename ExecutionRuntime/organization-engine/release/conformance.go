package release

import (
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type ConformanceCandidateV1 struct {
	ReleaseValid             bool
	ConstructionClosureValid bool
	LocalReadinessExact      bool
	ProductionReadinessExact bool
	SupportMode              assemblercontract.SupportModeV1
	ProductionClaimEligible  bool
	DescriptorOnly           bool
}

func EvaluateConformanceCandidateV1(result PublicationResultV1, now time.Time) (ConformanceCandidateV1, error) {
	if now.IsZero() {
		return ConformanceCandidateV1{}, invalid("Organization conformance clock is required")
	}
	if err := result.Release.Validate(); err != nil {
		return ConformanceCandidateV1{}, err
	}
	if !now.Before(time.Unix(0, result.Release.ExpiresUnixNano)) {
		return ConformanceCandidateV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Organization release expired")
	}
	report := ConformanceCandidateV1{
		ReleaseValid:             true,
		ConstructionClosureValid: len(result.Release.ModuleDescriptors) == 1 && len(result.Release.CapabilityDescriptors) == 1 && len(result.Release.PortSpecs) == 1 && len(result.Release.FactoryDescriptors) == 1,
		SupportMode:              result.Release.SupportMode,
		DescriptorOnly:           true,
	}
	if result.LocalReadiness != nil {
		if err := result.LocalReadiness.ValidateCurrent(now); err != nil {
			return ConformanceCandidateV1{}, err
		}
		report.LocalReadinessExact = true
	}
	if result.ProductionReadiness != nil {
		if err := result.ProductionReadiness.ValidateCurrent(now); err != nil {
			return ConformanceCandidateV1{}, err
		}
		report.ProductionReadinessExact = true
	}
	expected := supportMode(report.LocalReadinessExact, report.ProductionReadinessExact)
	if result.Release.SupportMode != expected || result.LocalReady != report.LocalReadinessExact || result.ProductionReady != report.ProductionReadinessExact {
		return ConformanceCandidateV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingNotCertified, "Organization release support mode lacks exact readiness")
	}
	report.ProductionClaimEligible = result.ProductionReady && result.LocalReady && report.LocalReadinessExact && report.ProductionReadinessExact && result.Release.SupportMode == assemblercontract.SupportProductionV1
	return report, nil
}

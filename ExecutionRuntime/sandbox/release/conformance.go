package release

import (
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// ConformanceCandidateV1 is review input, not a certification fact. It makes
// the local candidate/production distinction visible to assemblers and tests.
type ConformanceCandidateV1 struct {
	ReleaseValid             bool
	ConstructionClosureValid bool
	ReadinessExactAndCurrent bool
	CatalogEligible          bool
	ProductionClaimEligible  bool
}

func EvaluateConformanceCandidateV1(result PublicationResultV1, now time.Time) (ConformanceCandidateV1, error) {
	if now.IsZero() {
		return ConformanceCandidateV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "sandbox release conformance requires a clock")
	}
	if err := result.Release.Validate(); err != nil {
		return ConformanceCandidateV1{}, err
	}
	if !now.Before(time.Unix(0, result.Release.ExpiresUnixNano)) {
		return ConformanceCandidateV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "sandbox release conformance candidate expired")
	}
	report := ConformanceCandidateV1{ReleaseValid: true, ConstructionClosureValid: len(result.Release.ModuleDescriptors) > 0 && len(result.Release.CapabilityDescriptors) > 0 && len(result.Release.PortSpecs) > 0 && len(result.Release.FactoryDescriptors) > 0, CatalogEligible: true}
	if result.Readiness != nil {
		if err := result.Readiness.ValidateCurrent(now); err != nil {
			return ConformanceCandidateV1{}, err
		}
		report.ReadinessExactAndCurrent = true
	}
	report.ProductionClaimEligible = result.ProductionReady && report.ReadinessExactAndCurrent && result.Release.SupportMode == assemblercontract.SupportProductionV1
	if result.ProductionReady != report.ProductionClaimEligible {
		return ConformanceCandidateV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingNotCertified, "sandbox release production status is not backed by current readiness")
	}
	return report, nil
}

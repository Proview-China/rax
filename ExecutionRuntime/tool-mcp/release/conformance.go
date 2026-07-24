package release

import (
	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"time"
)

type ConformanceCandidateV1 struct {
	ReleaseValid             bool
	ConstructionClosureValid bool
	LocalReadinessExact      bool
	ProductionReadinessExact bool
	SupportMode              assemblercontract.SupportModeV1
	ProductionClaimEligible  bool
}

func EvaluateConformanceCandidateV1(r PublicationResultV1, now time.Time) (ConformanceCandidateV1, error) {
	if now.IsZero() {
		return ConformanceCandidateV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "tool/MCP conformance clock is required")
	}
	if e := r.Release.Validate(); e != nil {
		return ConformanceCandidateV1{}, e
	}
	if !now.Before(time.Unix(0, r.Release.ExpiresUnixNano)) {
		return ConformanceCandidateV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "tool/MCP release expired")
	}
	out := ConformanceCandidateV1{ReleaseValid: true, ConstructionClosureValid: len(r.Release.ModuleDescriptors) > 0 && len(r.Release.CapabilityDescriptors) == 2 && len(r.Release.PortSpecs) == 2 && len(r.Release.FactoryDescriptors) == 2, SupportMode: r.Release.SupportMode}
	if r.LocalReadiness != nil {
		if e := r.LocalReadiness.ValidateCurrent(now); e != nil {
			return ConformanceCandidateV1{}, e
		}
		out.LocalReadinessExact = true
	}
	if r.ProductionReadiness != nil {
		if e := r.ProductionReadiness.ValidateCurrent(now); e != nil {
			return ConformanceCandidateV1{}, e
		}
		out.ProductionReadinessExact = true
	}
	out.ProductionClaimEligible = r.ProductionReady && r.LocalReady && out.LocalReadinessExact && out.ProductionReadinessExact && r.Release.SupportMode == assemblercontract.SupportProductionV1
	if r.ProductionReady != out.ProductionClaimEligible {
		return ConformanceCandidateV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingNotCertified, "tool/MCP production status lacks exact current readiness")
	}
	return out, nil
}

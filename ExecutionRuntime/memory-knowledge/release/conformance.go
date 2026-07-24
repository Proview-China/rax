package release

import (
	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"time"
)

type ConformanceCandidateV1 struct {
	ReleaseValid, ConstructionValid, LocalExact, ProductionExact, ProductionEligible bool
	SupportMode                                                                      assemblercontract.SupportModeV1
}

func EvaluateV1(r PublicationResultV1, now time.Time) (ConformanceCandidateV1, error) {
	if now.IsZero() {
		return ConformanceCandidateV1{}, invalid("conformance clock missing")
	}
	if e := r.Release.Validate(); e != nil {
		return ConformanceCandidateV1{}, e
	}
	if !now.Before(time.Unix(0, r.Release.ExpiresUnixNano)) {
		return ConformanceCandidateV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "release expired")
	}
	o := ConformanceCandidateV1{ReleaseValid: true, ConstructionValid: len(r.Release.ModuleDescriptors) == 1 && len(r.Release.CapabilityDescriptors) == 2 && len(r.Release.PortSpecs) == 2 && len(r.Release.FactoryDescriptors) == 2, SupportMode: r.Release.SupportMode}
	if r.Local != nil {
		if e := r.Local.ValidateCurrent(now); e != nil {
			return ConformanceCandidateV1{}, e
		}
		o.LocalExact = true
	}
	if r.Production != nil {
		if e := r.Production.ValidateCurrent(now); e != nil {
			return ConformanceCandidateV1{}, e
		}
		o.ProductionExact = true
	}
	o.ProductionEligible = r.ProductionReady && r.LocalReady && o.LocalExact && o.ProductionExact && r.Release.SupportMode == assemblercontract.SupportProductionV1
	if r.ProductionReady != o.ProductionEligible {
		return ConformanceCandidateV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingNotCertified, "production claim not proven")
	}
	return o, nil
}

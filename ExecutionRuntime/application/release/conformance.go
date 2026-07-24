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

func EvaluateV1(r PublicationResultV1, n time.Time) (ConformanceCandidateV1, error) {
	if n.IsZero() {
		return ConformanceCandidateV1{}, invalid("clock missing")
	}
	if e := r.Release.Validate(); e != nil {
		return ConformanceCandidateV1{}, e
	}
	if !n.Before(time.Unix(0, r.Release.ExpiresUnixNano)) {
		return ConformanceCandidateV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "release expired")
	}
	o := ConformanceCandidateV1{ReleaseValid: true, ConstructionValid: len(r.Release.CapabilityDescriptors) == len(capabilities)+1 && len(r.Release.PortSpecs) == len(capabilities) && len(r.Release.FactoryDescriptors) == len(capabilities), SupportMode: r.Release.SupportMode}
	if r.Local != nil {
		if e := r.Local.ValidateCurrent(n); e != nil {
			return ConformanceCandidateV1{}, e
		}
		o.LocalExact = true
	}
	if r.Production != nil {
		if e := r.Production.ValidateCurrent(n); e != nil {
			return ConformanceCandidateV1{}, e
		}
		o.ProductionExact = true
	}
	o.ProductionEligible = r.ProductionReady && r.LocalReady && o.LocalExact && o.ProductionExact && r.Release.SupportMode == assemblercontract.SupportProductionV1
	if r.ProductionReady != o.ProductionEligible {
		return ConformanceCandidateV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingNotCertified, "production claim unproven")
	}
	return o, nil
}

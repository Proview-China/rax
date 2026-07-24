package conformance

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type OperationScopeEvidenceActionConformanceCaseV3 struct {
	Router        ports.OperationScopeEvidenceActionApplicabilityCurrentRouterV3
	Provider      ports.ControlledOperationProviderPortV1
	Sources       []ports.OperationScopeEvidenceActionApplicabilitySourceV3
	ScopeDigest   core.Digest
	Call          ports.ControlledOperationProviderCallRequestV1
	ProviderCalls func() int
}

type OperationScopeEvidenceActionConformanceReportV3 struct {
	ClosedMatrixExact       bool
	AllFiveOwnerRoutesExact bool
	BoundaryCurrentExact    bool
	ReplayDidNotRecall      bool
	ProductionClaimEligible bool
}

// RunOperationScopeEvidenceActionConformanceV3 is safe for isolated fixtures.
// It never invokes a production Provider repeatedly and does not attest
// physical exactly-once, durability, availability or SLA.
func RunOperationScopeEvidenceActionConformanceV3(ctx context.Context, test OperationScopeEvidenceActionConformanceCaseV3) (OperationScopeEvidenceActionConformanceReportV3, error) {
	report := OperationScopeEvidenceActionConformanceReportV3{}
	if test.Router == nil || test.Provider == nil || test.ProviderCalls == nil {
		return report, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Action Evidence conformance dependencies are incomplete")
	}
	if err := ports.OperationScopeEvidenceActionMatrixV3().Validate(); err != nil {
		return report, err
	}
	report.ClosedMatrixExact = true
	if len(test.Sources) != 5 {
		return report, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "Action Evidence conformance requires five Owner sources")
	}
	seen := map[ports.OperationScopeEvidenceApplicabilityDimensionV3]bool{}
	for _, source := range test.Sources {
		ref, err := ports.ProjectOperationScopeEvidenceActionApplicabilityRefV3(source)
		if err != nil {
			return report, err
		}
		if seen[source.Route.Dimension] {
			return report, core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "Action Evidence conformance source dimension is duplicated")
		}
		seen[source.Route.Dimension] = true
		if _, err := test.Router.InspectOperationScopeEvidenceActionApplicabilityCurrentV3(ctx, source.Route.Dimension, ref, test.ScopeDigest); err != nil {
			return report, err
		}
	}
	report.AllFiveOwnerRoutesExact = len(seen) == 5
	before := test.ProviderCalls()
	if err := test.Provider.CallControlledOperationProviderV1(ctx, test.Call); err != nil {
		return report, err
	}
	after := test.ProviderCalls()
	if after != before+1 {
		return report, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "controlled Provider seam did not make exactly one logical fixture call")
	}
	report.BoundaryCurrentExact = true
	_ = test.Provider.CallControlledOperationProviderV1(ctx, test.Call)
	report.ReplayDidNotRecall = test.ProviderCalls() == after
	if !report.ReplayDidNotRecall {
		return report, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "controlled Provider replay invoked the fixture Provider again")
	}
	report.ProductionClaimEligible = false
	return report, nil
}

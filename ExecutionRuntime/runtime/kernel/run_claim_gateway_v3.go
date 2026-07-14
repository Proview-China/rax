package kernel

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// RunClaimGatewayV3 adds certified-Run preflight before the legacy V2 ledger
// mutation. It never appends Evidence for an uncertified Run.
type RunClaimGatewayV3 struct {
	Legacy         *RunClaimGatewayV2
	Bundles        control.RunBundleFactPortV3
	PlanAdmissions ports.RunSettlementPlanAdmissionPortV3
}

func (g RunClaimGatewayV3) IngestRunClaimV3(ctx context.Context, request ports.RunClaimIngestRequestV2) (ports.RunClaimIngestResultV3, error) {
	if err := request.Validate(); err != nil {
		return ports.RunClaimIngestResultV3{}, err
	}
	bundle, err := g.preflight(ctx, request.Candidate.ExecutionScope, request.Candidate.LedgerScope.RunID)
	if err != nil {
		return ports.RunClaimIngestResultV3{}, err
	}
	if (bundle.Run.Status != core.RunRunning && bundle.Run.Status != core.RunStopping) || bundle.Run.Revision != request.ExpectedRunRevision {
		return ports.RunClaimIngestResultV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunClaimUnverified, "claim requires the exact certified active Run")
	}
	legacy, err := g.Legacy.IngestRunClaimV2(ctx, request)
	if err != nil {
		return ports.RunClaimIngestResultV3{}, err
	}
	result := ports.RunClaimIngestResultV3{Certification: bundle.Certification, Plan: claimPlanRefV3(bundle.Plan), Run: legacy.Run, Evidence: legacy.Evidence, Association: legacy.Association}
	return result, result.Validate()
}

func (g RunClaimGatewayV3) InspectRunClaimV3(ctx context.Context, scope core.ExecutionScope, runID core.AgentRunID) (ports.RunClaimIngestResultV3, error) {
	if err := (ports.RunTerminationRequestV3{ExecutionScope: scope, RunID: runID}).Validate(); err != nil {
		return ports.RunClaimIngestResultV3{}, err
	}
	bundle, err := g.preflight(ctx, scope, runID)
	if err != nil {
		return ports.RunClaimIngestResultV3{}, err
	}
	legacy, err := g.Legacy.InspectRunClaimV2(ctx, scope, runID)
	if err != nil {
		return ports.RunClaimIngestResultV3{}, err
	}
	result := ports.RunClaimIngestResultV3{Certification: bundle.Certification, Plan: claimPlanRefV3(bundle.Plan), Run: legacy.Run, Evidence: legacy.Evidence, Association: legacy.Association}
	return result, result.Validate()
}

func (g RunClaimGatewayV3) preflight(ctx context.Context, scope core.ExecutionScope, runID core.AgentRunID) (control.RunBundleV3, error) {
	if g.Legacy == nil || g.Bundles == nil || g.PlanAdmissions == nil {
		return control.RunBundleV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "certified claim ingest requires V2 gateway, bundle and Plan admission readers")
	}
	bundle, err := g.Bundles.InspectRunBundleV3(ctx, scope, runID)
	if err != nil {
		return control.RunBundleV3{}, err
	}
	fact, err := g.PlanAdmissions.InspectCertifiedRunSettlementPlanV3(ctx, scope, runID)
	if err != nil {
		return control.RunBundleV3{}, err
	}
	ref, refErr := fact.RefV3()
	planRef, planErr := bundle.Plan.RefV2()
	expectedAssociation, associationErr := ports.NewRunSettlementPlanCertificationAssociationV3(bundle.Run, bundle.Plan, ref)
	if bundle.Run.Validate() != nil || bundle.Plan.Validate() != nil || fact.Validate() != nil || refErr != nil || planErr != nil || associationErr != nil || expectedAssociation != bundle.Certification || ref != bundle.Certification.Certification || fact.RunID != bundle.Run.ID || fact.RunIdentityDigest != bundle.Certification.RunIdentityDigest || fact.ExecutionScopeDigest != bundle.Certification.ExecutionScopeDigest || fact.Plan != planRef {
		return control.RunBundleV3{}, core.NewError(core.ErrorConflict, core.ReasonRunClaimUnverified, "claim ingest requires the exact historical Plan certification")
	}
	return bundle, nil
}

func claimPlanRefV3(plan ports.RunSettlementPlanFactV2) ports.RunSettlementPlanLifecycleRefV3 {
	ref, _ := plan.RefV2()
	return ports.RunSettlementPlanLifecycleRefV3{RunSettlementPlanRefV2: ref, RunID: plan.RunID, RunIdentityDigest: plan.RunIdentityDigest, ExecutionScopeDigest: plan.ExecutionScopeDigest}
}

var _ ports.RunClaimIngestGovernancePortV3 = RunClaimGatewayV3{}

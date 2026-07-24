package conformance

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type OperationSettlementConformanceCaseV4 struct {
	Governance ports.OperationSettlementGovernancePortV4
	// Facts is retained only for source compatibility. The V4 conformance
	// contract never reads it; consumers need only the Governance read surface.
	Facts      ports.OperationSettlementFactPortV4
	Submission ports.OperationSettlementSubmissionV4
	Changed    ports.OperationSettlementSubmissionV4
}

type OperationSettlementConformanceReportV4 struct {
	SameContentReplayIdempotent bool
	ChangedContentConflicts     bool
	HistoricalClosureExact      bool
	CurrentClosureExact         bool
	ProviderCalled              bool
	EvidenceReconsumed          bool
	ProductionClaimEligible     bool
}

// CheckOperationSettlementV4 exercises only public Runtime ports. It proves
// logical create-once/Inspect behavior for a supplied fixture; it makes no
// production durability, physical exactly-once, Provider or SLA claim.
func CheckOperationSettlementV4(ctx context.Context, testCase OperationSettlementConformanceCaseV4) (OperationSettlementConformanceReportV4, error) {
	if testCase.Governance == nil {
		return OperationSettlementConformanceReportV4{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "V4 settlement conformance requires the public governance surface")
	}
	if err := testCase.Submission.Validate(); err != nil {
		return OperationSettlementConformanceReportV4{}, err
	}
	if err := testCase.Changed.Validate(); err != nil {
		return OperationSettlementConformanceReportV4{}, core.NewError(core.ErrorInvalidArgument, core.ReasonIdempotencyPayloadMismatch, "V4 settlement conformance requires a valid changed-content submission")
	}
	first, err := testCase.Governance.SettleOperationV4(ctx, testCase.Submission)
	if err != nil {
		return OperationSettlementConformanceReportV4{}, err
	}
	replayed, err := testCase.Governance.SettleOperationV4(ctx, testCase.Submission)
	if err != nil {
		return OperationSettlementConformanceReportV4{}, err
	}
	historical, err := testCase.Governance.InspectOperationSettlementV4(ctx, ports.InspectOperationSettlementRequestV4{Operation: testCase.Submission.Operation, SettlementID: testCase.Submission.ID})
	if err != nil {
		return OperationSettlementConformanceReportV4{}, err
	}
	historicalClosure, err := testCase.Governance.InspectOperationSettlementClosureV4(ctx, ports.InspectOperationSettlementRequestV4{Operation: testCase.Submission.Operation, SettlementID: testCase.Submission.ID})
	if err != nil {
		return OperationSettlementConformanceReportV4{}, err
	}
	current, err := testCase.Governance.InspectCurrentOperationSettlementV4(ctx, ports.InspectCurrentOperationSettlementRequestV4{Operation: testCase.Submission.Operation, EffectID: testCase.Submission.EffectID})
	if err != nil {
		return OperationSettlementConformanceReportV4{}, err
	}
	association, err := testCase.Governance.InspectOperationSettlementEvidenceAssociationV4(ctx, testCase.Submission.Operation, current.Association)
	if err != nil {
		return OperationSettlementConformanceReportV4{}, err
	}
	guard, err := testCase.Governance.InspectOperationSettlementTerminalGuardV4(ctx, testCase.Submission.Operation, current.Guard)
	if err != nil {
		return OperationSettlementConformanceReportV4{}, err
	}
	projection, err := testCase.Governance.InspectOperationSettlementTerminalProjectionV4(ctx, testCase.Submission.Operation, current.Projection)
	if err != nil {
		return OperationSettlementConformanceReportV4{}, err
	}
	_, changedErr := testCase.Governance.SettleOperationV4(ctx, testCase.Changed)
	changedConflicts := core.HasCategory(changedErr, core.ErrorConflict)
	historicalExact := historicalClosure.Validate() == nil && ports.SameOperationSettlementRefV4(historical.RefV4(), first) && ports.SameOperationSettlementRefV4(historicalClosure.Settlement.RefV4(), first) && ports.SameOperationSettlementEvidenceAssociationRefV4(historicalClosure.Association.RefV4(), historicalClosure.Projection.Association) && ports.SameOperationSettlementTerminalGuardRefV4(historicalClosure.Guard.RefV4(), historicalClosure.Projection.Guard)
	currentExact := ports.SameOperationSettlementRefV4(current.Settlement, first) && ports.SameOperationSettlementEvidenceAssociationRefV4(current.Association, association.RefV4()) && ports.SameOperationSettlementTerminalGuardRefV4(current.Guard, guard.RefV4()) && ports.SameOperationSettlementTerminalProjectionRefV4(current.Projection, projection.RefV4()) && ports.SameOperationSettlementDomainResultFactRefV4(current.DomainResult, historical.Submission.DomainResult) && ports.SameOperationSettlementEvidenceAssociationRefV4(projection.Association, association.RefV4()) && ports.SameOperationSettlementTerminalGuardRefV4(projection.Guard, guard.RefV4())
	return OperationSettlementConformanceReportV4{
		SameContentReplayIdempotent: ports.SameOperationSettlementRefV4(first, replayed),
		ChangedContentConflicts:     changedConflicts,
		HistoricalClosureExact:      historicalExact,
		CurrentClosureExact:         currentExact,
		ProviderCalled:              false,
		EvidenceReconsumed:          false,
		ProductionClaimEligible:     false,
	}, nil
}

package conformance

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// CheckpointEvidenceSettlementFixtureV5 is fixture-only. It never calls a
// Provider and cannot establish production durability, availability, or SLA.
type CheckpointEvidenceSettlementFixtureV5 struct {
	Qualification ports.IssueCheckpointPhaseQualificationRequestV1
	Handoff       ports.CreateCheckpointPhaseProviderHandoffRequestV1
	Consumption   ports.ConsumeCheckpointPhaseEvidenceRequestV1
	Settlement    ports.OperationCheckpointRestoreSettlementSubmissionV5
}

type CheckpointEvidenceSettlementReportV5 struct {
	IssueDoesNotSettle      bool `json:"issue_does_not_settle"`
	QualificationOwnerExact bool `json:"qualification_owner_exact"`
	QualificationTTLBounded bool `json:"qualification_ttl_bounded"`
	ConsumedCurrentExact    bool `json:"consumed_current_exact"`
	HistoricalClosureExact  bool `json:"historical_closure_exact"`
	CurrentClosureExact     bool `json:"current_closure_exact"`
	AssociationClosureExact bool `json:"association_closure_exact"`
	ProviderCalls           int  `json:"provider_calls"`
	ProductionClaimEligible bool `json:"production_claim_eligible"`
}

func RunCheckpointEvidenceSettlementConformanceV5(ctx context.Context, evidence ports.CheckpointRestoreEvidenceGovernancePortV1, settlement ports.OperationCheckpointRestoreSettlementGovernancePortV5, fixture CheckpointEvidenceSettlementFixtureV5) (CheckpointEvidenceSettlementReportV5, error) {
	if evidence == nil || settlement == nil {
		return CheckpointEvidenceSettlementReportV5{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "checkpoint Evidence and Settlement governance ports are required")
	}
	qualification, err := evidence.IssueCheckpointPhaseQualificationV1(ctx, fixture.Qualification)
	if err != nil {
		return CheckpointEvidenceSettlementReportV5{}, err
	}
	historicalQualification, err := evidence.InspectCheckpointPhaseQualificationHistoricalV1(ctx, qualification)
	if err != nil || historicalQualification.Ref != qualification {
		return CheckpointEvidenceSettlementReportV5{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint qualification historical closure drifted")
	}
	handoffRequest := fixture.Handoff
	handoffRequest.Qualification = qualification
	handoff, err := evidence.CreateCheckpointPhaseProviderHandoffV1(ctx, handoffRequest)
	if err != nil {
		return CheckpointEvidenceSettlementReportV5{}, err
	}
	consumeRequest := fixture.Consumption
	consumeRequest.Qualification = qualification
	consumeRequest.Handoff = handoff
	consumption, err := evidence.ConsumeCheckpointPhaseEvidenceCurrentV1(ctx, consumeRequest)
	if err != nil {
		return CheckpointEvidenceSettlementReportV5{}, err
	}
	submission := fixture.Settlement
	submission.Evidence = consumption
	submission.Handoff = handoff
	ref, err := settlement.SettleCheckpointPhaseV5(ctx, submission)
	if err != nil {
		return CheckpointEvidenceSettlementReportV5{}, err
	}
	historical, err := settlement.InspectCheckpointPhaseSettlementHistoricalV5(ctx, ports.InspectOperationCheckpointRestoreSettlementRequestV5{Operation: submission.Operation, SettlementID: ref.ID})
	if err != nil {
		return CheckpointEvidenceSettlementReportV5{}, err
	}
	current, err := settlement.InspectCheckpointPhaseSettlementCurrentV5(ctx, ports.InspectCurrentOperationCheckpointRestoreSettlementRequestV5{Operation: submission.Operation, EffectID: submission.EffectID})
	if err != nil {
		return CheckpointEvidenceSettlementReportV5{}, err
	}
	association, associationErr := settlement.InspectCheckpointPhaseSettlementAssociationV5(ctx, submission.Operation, historical.Association.Ref)
	guard, guardErr := settlement.InspectCheckpointPhaseTerminalGuardV5(ctx, submission.Operation, historical.Guard.Ref)
	projection, projectionErr := settlement.InspectCheckpointPhaseTerminalProjectionV5(ctx, submission.Operation, historical.Projection.Ref)
	report := CheckpointEvidenceSettlementReportV5{
		IssueDoesNotSettle:      historicalQualification.Ref == qualification,
		QualificationOwnerExact: qualification.Barrier == fixture.Qualification.Barrier && qualification.EffectCut == fixture.Qualification.EffectCut && qualification.Reservation == fixture.Qualification.Reservation,
		QualificationTTLBounded: qualification.ExpiresUnixNano > historicalQualification.CreatedUnixNano && qualification.ExpiresUnixNano-historicalQualification.CreatedUnixNano <= int64(30*time.Second),
		ConsumedCurrentExact:    consumption.State == ports.CheckpointEvidenceConsumedCurrentV1 && consumption.Handoff == handoff,
		HistoricalClosureExact:  historical.Settlement == ref && historical.Validate() == nil,
		CurrentClosureExact:     current.Bundle.Settlement == ref && current.Validate() == nil,
		AssociationClosureExact: associationErr == nil && guardErr == nil && projectionErr == nil && association.Ref == historical.Association.Ref && guard.Ref == historical.Guard.Ref && projection.Ref == historical.Projection.Ref,
		ProviderCalls:           0,
		ProductionClaimEligible: false,
	}
	return report, nil
}

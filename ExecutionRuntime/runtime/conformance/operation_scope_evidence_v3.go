package conformance

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// OperationScopeEvidenceConformanceCaseV3 is intentionally destructive only
// against an isolated test fixture. It is not a production certification or
// backend/SLA claim.
type OperationScopeEvidenceConformanceCaseV3 struct {
	Governance              ports.OperationScopeEvidenceGovernancePortV3
	Facts                   ports.OperationScopeEvidenceFactPortV3
	Issue                   ports.IssueOperationScopeEvidenceRequestV3
	HandoffID               string
	ConsumptionID           string
	Candidate               func(ports.OperationScopeEvidenceQualificationFactV3, ports.OperationScopeEvidenceProviderHandoffFactV3) ports.OperationScopeEvidenceCandidateV3
	ForbiddenMutationCounts func() (providerCalls int, domainFactWrites int, settlementWrites int)
}

type OperationScopeEvidenceConformanceReportV3 struct {
	IssueDidNotAdvanceCursor             bool
	HandoffIsProofOnly                   bool
	ConsumeAdvancedAtomically            bool
	SameContentReplayIdempotent          bool
	ChangedContentConflicts              bool
	NoProviderDomainOrSettlementMutation bool
	ProductionClaimEligible              bool
}

func CheckOperationScopeEvidenceV3(ctx context.Context, c OperationScopeEvidenceConformanceCaseV3) (OperationScopeEvidenceConformanceReportV3, error) {
	if c.Governance == nil || c.Facts == nil || c.Candidate == nil || c.ForbiddenMutationCounts == nil || c.HandoffID == "" || c.ConsumptionID == "" {
		return OperationScopeEvidenceConformanceReportV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "isolated Operation Evidence conformance fixture is incomplete")
	}
	before, err := c.Facts.InspectOperationScopeEvidenceSourceV3(ctx, c.Issue.Reservation.Source.RegistrationID)
	if err != nil {
		return OperationScopeEvidenceConformanceReportV3{}, err
	}
	qualification, err := c.Governance.IssueOperationScopeEvidenceV3(ctx, c.Issue)
	if err != nil {
		return OperationScopeEvidenceConformanceReportV3{}, err
	}
	afterIssue, err := c.Facts.InspectOperationScopeEvidenceSourceV3(ctx, before.ID)
	if err != nil {
		return OperationScopeEvidenceConformanceReportV3{}, err
	}
	handoff, err := c.Governance.HandoffOperationScopeEvidenceV3(ctx, ports.HandoffOperationScopeEvidenceRequestV3{HandoffID: c.HandoffID, Qualification: qualification.RefV3()})
	if err != nil {
		return OperationScopeEvidenceConformanceReportV3{}, err
	}
	afterHandoff, err := c.Facts.InspectOperationScopeEvidenceSourceV3(ctx, before.ID)
	if err != nil {
		return OperationScopeEvidenceConformanceReportV3{}, err
	}
	candidate := c.Candidate(qualification, handoff)
	first, err := c.Governance.ConsumeOperationScopeEvidenceV3(ctx, ports.ConsumeOperationScopeEvidenceRequestV3{ConsumptionID: c.ConsumptionID, Handoff: handoff.RefV3(), Candidate: candidate})
	if err != nil {
		return OperationScopeEvidenceConformanceReportV3{}, err
	}
	replayed, err := c.Governance.ConsumeOperationScopeEvidenceV3(ctx, ports.ConsumeOperationScopeEvidenceRequestV3{ConsumptionID: c.ConsumptionID, Handoff: handoff.RefV3(), Candidate: candidate})
	if err != nil {
		return OperationScopeEvidenceConformanceReportV3{}, err
	}
	changed := candidate
	changed.Payload.ContentDigest = core.DigestBytes([]byte("changed-conformance-payload"))
	_, changedErr := c.Governance.ConsumeOperationScopeEvidenceV3(ctx, ports.ConsumeOperationScopeEvidenceRequestV3{ConsumptionID: c.ConsumptionID, Handoff: handoff.RefV3(), Candidate: changed})
	provider, domain, settlement := c.ForbiddenMutationCounts()
	report := OperationScopeEvidenceConformanceReportV3{
		IssueDidNotAdvanceCursor:             afterIssue.Revision == before.Revision && afterIssue.NextSequence == before.NextSequence,
		HandoffIsProofOnly:                   afterHandoff.Revision == before.Revision && afterHandoff.NextSequence == before.NextSequence,
		ConsumeAdvancedAtomically:            first.Source.Revision == before.Revision+1 && first.Source.NextSequence == before.NextSequence+1 && first.Qualification.Consumption != nil && first.Record.Ref == first.Consumption.Record,
		SameContentReplayIdempotent:          replayed.Consumption.Digest == first.Consumption.Digest && replayed.Record.Ref == first.Record.Ref,
		ChangedContentConflicts:              core.HasCategory(changedErr, core.ErrorConflict),
		NoProviderDomainOrSettlementMutation: provider == 0 && domain == 0 && settlement == 0,
		ProductionClaimEligible:              false,
	}
	if !report.IssueDidNotAdvanceCursor || !report.HandoffIsProofOnly || !report.ConsumeAdvancedAtomically || !report.SameContentReplayIdempotent || !report.ChangedContentConflicts || !report.NoProviderDomainOrSettlementMutation {
		return report, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Operation Evidence conformance invariant failed")
	}
	return report, nil
}

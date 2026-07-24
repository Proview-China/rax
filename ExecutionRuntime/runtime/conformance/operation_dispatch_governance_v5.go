package conformance

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type OperationDispatchGovernanceCaseV5 struct {
	Gateway ports.OperationGovernancePortV5
	Issue   ports.IssueGovernedOperationDispatchRequestV5
}
type OperationDispatchGovernanceReportV5 struct {
	AtomicIssueObserved       bool
	HistoricalInspectObserved bool
	CurrentInspectObserved    bool
	BeginObserved             bool
	ProviderCalled            bool
	V5MasqueradesAsV4         bool
	ProductionClaimEligible   bool
}

func CheckOperationDispatchGovernanceV5(ctx context.Context, c OperationDispatchGovernanceCaseV5) (OperationDispatchGovernanceReportV5, error) {
	if c.Gateway == nil {
		return OperationDispatchGovernanceReportV5{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "V5 governance gateway required")
	}
	if err := c.Issue.Validate(); err != nil {
		return OperationDispatchGovernanceReportV5{}, err
	}
	issued, err := c.Gateway.IssueOperationDispatchV5(ctx, c.Issue)
	if err != nil {
		return OperationDispatchGovernanceReportV5{}, err
	}
	inspect := ports.InspectOperationDispatchRecordRequestV5{Operation: c.Issue.Operation, EffectID: c.Issue.EffectID, PermitID: c.Issue.PermitID}
	historical, err := c.Gateway.InspectOperationDispatchRecordV5(ctx, inspect)
	if err != nil {
		return OperationDispatchGovernanceReportV5{}, err
	}
	current, err := c.Gateway.InspectCurrentOperationDispatchV5(ctx, ports.InspectCurrentOperationDispatchRequestV5{Inspect: inspect, AdmissionDigest: issued.Record.Permit.Admission.Digest, ReviewAuthorization: issued.ReviewAuthorization, AuthorizationBasis: issued.AuthorizationBasis})
	if err != nil {
		return OperationDispatchGovernanceReportV5{}, err
	}
	begun, err := c.Gateway.BeginOperationDispatchV5(ctx, ports.BeginGovernedOperationDispatchRequestV5{Operation: c.Issue.Operation, EffectID: c.Issue.EffectID, ExpectedEffectRevision: issued.Record.EffectFactRevision, PermitID: c.Issue.PermitID, ExpectedPermitFactRevision: issued.Record.Revision, AdmissionDigest: issued.Record.Permit.Admission.Digest, ReviewAuthorization: issued.ReviewAuthorization, AuthorizationBasis: issued.AuthorizationBasis})
	if err != nil {
		return OperationDispatchGovernanceReportV5{}, err
	}
	report := OperationDispatchGovernanceReportV5{AtomicIssueObserved: issued.Record.State == ports.OperationPermitIssuedV5, HistoricalInspectObserved: historical.Digest == issued.Record.Digest, CurrentInspectObserved: current.Record.Digest == issued.Record.Digest, BeginObserved: begun.Record.State == ports.OperationPermitBegunV5}
	if !report.AtomicIssueObserved || !report.HistoricalInspectObserved || !report.CurrentInspectObserved || !report.BeginObserved {
		return OperationDispatchGovernanceReportV5{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "V5 governance conformance incomplete")
	}
	return report, nil
}

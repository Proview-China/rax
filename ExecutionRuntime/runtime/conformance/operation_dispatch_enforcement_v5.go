package conformance

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type OperationDispatchEnforcementCaseV5 struct {
	Gateway         ports.OperationDispatchEnforcementGovernancePortV5
	Prepare         ports.EnforceCurrentOperationDispatchRequestV5
	PreparedAttempt ports.PreparedProviderAttemptRefV2
}
type OperationDispatchEnforcementReportV5 struct {
	PreparePersisted        bool
	HistoricalInspect       bool
	CurrentInspect          bool
	ExecutePersisted        bool
	ExactReplayIdempotent   bool
	ChangedExecuteConflict  bool
	ProviderCalled          bool
	ProductionClaimEligible bool
}

func CheckOperationDispatchEnforcementV5(ctx context.Context, c OperationDispatchEnforcementCaseV5) (OperationDispatchEnforcementReportV5, error) {
	if c.Gateway == nil {
		return OperationDispatchEnforcementReportV5{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "V5 enforcement gateway required")
	}
	if err := c.Prepare.Validate(); err != nil {
		return OperationDispatchEnforcementReportV5{}, err
	}
	if err := c.PreparedAttempt.Validate(); err != nil {
		return OperationDispatchEnforcementReportV5{}, err
	}
	prepared, err := c.Gateway.EnforceCurrentOperationDispatchV5(ctx, c.Prepare)
	if err != nil {
		return OperationDispatchEnforcementReportV5{}, err
	}
	inspect := ports.InspectOperationDispatchEnforcementRequestV5{Operation: c.Prepare.Operation, EffectID: c.Prepare.EffectID, PermitID: c.Prepare.PermitID, Phase: ports.OperationDispatchEnforcementPrepareV4}
	historical, err := c.Gateway.InspectOperationDispatchEnforcementV5(ctx, inspect)
	if err != nil {
		return OperationDispatchEnforcementReportV5{}, err
	}
	current, err := c.Gateway.InspectCurrentOperationDispatchEnforcementV5(ctx, ports.InspectCurrentOperationDispatchEnforcementRequestV5{Inspect: inspect, PermitDigest: c.Prepare.PermitDigest, AdmissionDigest: c.Prepare.AdmissionDigest, ReviewAuthorization: c.Prepare.ReviewAuthorization, AuthorizationBasis: c.Prepare.AuthorizationBasis, SandboxAttempt: c.Prepare.SandboxAttempt, SandboxProjectionDigest: c.Prepare.SandboxProjectionDigest})
	if err != nil {
		return OperationDispatchEnforcementReportV5{}, err
	}
	replayed, err := c.Gateway.EnforceCurrentOperationDispatchV5(ctx, c.Prepare)
	if err != nil {
		return OperationDispatchEnforcementReportV5{}, err
	}
	execute := c.Prepare
	execute.Phase = ports.OperationDispatchEnforcementExecuteV4
	execute.ExpectedJournalRevision = 1
	execute.Prepare = &prepared.Phase
	execute.PreparedAttempt = &c.PreparedAttempt
	executed, err := c.Gateway.EnforceCurrentOperationDispatchV5(ctx, execute)
	if err != nil {
		return OperationDispatchEnforcementReportV5{}, err
	}
	changed := c.PreparedAttempt
	changed.PreparedUnixNano++
	changed, err = ports.SealPreparedProviderAttemptRefV2(changed)
	if err != nil {
		return OperationDispatchEnforcementReportV5{}, err
	}
	execute.PreparedAttempt = &changed
	_, changedErr := c.Gateway.EnforceCurrentOperationDispatchV5(ctx, execute)
	report := OperationDispatchEnforcementReportV5{PreparePersisted: prepared.Journal.Revision == 1, HistoricalInspect: historical.Digest == prepared.Journal.Digest, CurrentInspect: current.Phase.ReceiptDigest == prepared.Phase.ReceiptDigest, ExecutePersisted: executed.Journal.Revision == 2, ExactReplayIdempotent: replayed.Journal.Digest == prepared.Journal.Digest, ChangedExecuteConflict: core.HasCategory(changedErr, core.ErrorConflict) || core.HasCategory(changedErr, core.ErrorPreconditionFailed)}
	if !report.PreparePersisted || !report.HistoricalInspect || !report.CurrentInspect || !report.ExecutePersisted || !report.ExactReplayIdempotent || !report.ChangedExecuteConflict {
		return OperationDispatchEnforcementReportV5{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "V5 enforcement conformance incomplete")
	}
	return report, nil
}

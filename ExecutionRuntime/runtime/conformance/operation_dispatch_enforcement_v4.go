package conformance

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type OperationDispatchEnforcementCaseV4 struct {
	Gateway         ports.OperationDispatchEnforcementGovernancePortV4
	Prepare         ports.EnforceCurrentOperationDispatchRequestV4
	PreparedAttempt ports.PreparedProviderAttemptRefV2
}

type OperationDispatchEnforcementReportV4 struct {
	PreparePersisted        bool
	HistoricalInspect       bool
	CurrentInspect          bool
	ExecutePersisted        bool
	PrepareSlotPreserved    bool
	ExactReplayIdempotent   bool
	ChangedExecuteConflict  bool
	ProviderCalled          bool
	BeginIsExecution        bool
	ProductionClaimEligible bool
}

func CheckOperationDispatchEnforcementV4(ctx context.Context, testCase OperationDispatchEnforcementCaseV4) (OperationDispatchEnforcementReportV4, error) {
	if testCase.Gateway == nil {
		return OperationDispatchEnforcementReportV4{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "enforcement conformance requires the public Gateway")
	}
	if err := testCase.Prepare.Validate(); err != nil {
		return OperationDispatchEnforcementReportV4{}, err
	}
	if err := testCase.PreparedAttempt.Validate(); err != nil {
		return OperationDispatchEnforcementReportV4{}, err
	}
	prepared, err := testCase.Gateway.EnforceCurrentOperationDispatchV4(ctx, testCase.Prepare)
	if err != nil {
		return OperationDispatchEnforcementReportV4{}, err
	}
	prepareInspect := ports.InspectOperationDispatchEnforcementRequestV4{
		Operation: testCase.Prepare.Operation, EffectID: testCase.Prepare.EffectID,
		PermitID: testCase.Prepare.PermitID, Phase: ports.OperationDispatchEnforcementPrepareV4,
	}
	historical, err := testCase.Gateway.InspectOperationDispatchEnforcementV4(ctx, prepareInspect)
	if err != nil {
		return OperationDispatchEnforcementReportV4{}, err
	}
	current, err := testCase.Gateway.InspectCurrentOperationDispatchEnforcementV4(ctx, ports.InspectCurrentOperationDispatchEnforcementRequestV4{
		Inspect: prepareInspect, PermitDigest: testCase.Prepare.PermitDigest,
		AdmissionDigest: testCase.Prepare.AdmissionDigest, ReviewAuthorization: testCase.Prepare.ReviewAuthorization,
		SandboxAttempt: testCase.Prepare.SandboxAttempt, SandboxProjectionDigest: testCase.Prepare.SandboxProjectionDigest,
	})
	if err != nil {
		return OperationDispatchEnforcementReportV4{}, err
	}
	replayed, err := testCase.Gateway.EnforceCurrentOperationDispatchV4(ctx, testCase.Prepare)
	if err != nil {
		return OperationDispatchEnforcementReportV4{}, err
	}
	execute := testCase.Prepare
	execute.Phase = ports.OperationDispatchEnforcementExecuteV4
	execute.ExpectedJournalRevision = 1
	execute.Prepare = &prepared.Phase
	execute.PreparedAttempt = &testCase.PreparedAttempt
	executed, err := testCase.Gateway.EnforceCurrentOperationDispatchV4(ctx, execute)
	if err != nil {
		return OperationDispatchEnforcementReportV4{}, err
	}
	changed := testCase.PreparedAttempt
	changed.PreparedUnixNano++
	changed, err = ports.SealPreparedProviderAttemptRefV2(changed)
	if err != nil {
		return OperationDispatchEnforcementReportV4{}, err
	}
	execute.PreparedAttempt = &changed
	_, changedErr := testCase.Gateway.EnforceCurrentOperationDispatchV4(ctx, execute)
	report := OperationDispatchEnforcementReportV4{
		PreparePersisted:       prepared.Phase.Phase == ports.OperationDispatchEnforcementPrepareV4 && prepared.Journal.Revision == 1,
		HistoricalInspect:      historical.Digest == prepared.Journal.Digest,
		CurrentInspect:         current.Phase.ReceiptDigest == prepared.Phase.ReceiptDigest,
		ExecutePersisted:       executed.Phase.Phase == ports.OperationDispatchEnforcementExecuteV4 && executed.Journal.Revision == 2,
		PrepareSlotPreserved:   executed.Journal.Prepare != nil && executed.Journal.Prepare.Digest == prepared.Phase.ReceiptDigest,
		ExactReplayIdempotent:  replayed.Journal.Digest == prepared.Journal.Digest,
		ChangedExecuteConflict: core.HasCategory(changedErr, core.ErrorConflict) || core.HasCategory(changedErr, core.ErrorPreconditionFailed),
	}
	if !report.PreparePersisted || !report.HistoricalInspect || !report.CurrentInspect || !report.ExecutePersisted || !report.PrepareSlotPreserved || !report.ExactReplayIdempotent || !report.ChangedExecuteConflict {
		return OperationDispatchEnforcementReportV4{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "enforcement conformance did not observe the frozen two-phase contract")
	}
	return report, nil
}

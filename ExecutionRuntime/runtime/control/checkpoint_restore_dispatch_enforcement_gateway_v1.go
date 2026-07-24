package control

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// CheckpointRestoreDispatchEnforcementGatewayV1 performs the checkpoint-only
// third current read and persists prepare/execute Enforcement receipts in the
// Runtime Effect Owner journal. It never calls a Provider.
type CheckpointRestoreDispatchEnforcementGatewayV1 struct {
	Dispatch ports.OperationGovernancePortV4
	Sandbox  ports.CheckpointRestoreDispatchSandboxCurrentReaderV1
	Facts    OperationDispatchEnforcementFactPortV4
	Clock    func() time.Time
}

func (g CheckpointRestoreDispatchEnforcementGatewayV1) EnforceCurrentCheckpointRestoreDispatchV1(ctx context.Context, request ports.EnforceCurrentCheckpointRestoreDispatchRequestV1) (ports.CurrentCheckpointRestoreDispatchEnforcementV1, error) {
	if err := g.validate(); err != nil {
		return ports.CurrentCheckpointRestoreDispatchEnforcementV1{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.CurrentCheckpointRestoreDispatchEnforcementV1{}, err
	}
	now := g.Clock()
	if now.IsZero() {
		return ports.CurrentCheckpointRestoreDispatchEnforcementV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "checkpoint enforcement clock returned zero")
	}
	dispatch, err := g.inspectDispatchCurrent(ctx, request.Operation, request.EffectID, request.PermitID, request.AdmissionDigest, request.ReviewAuthorization)
	if err != nil {
		return ports.CurrentCheckpointRestoreDispatchEnforcementV1{}, err
	}
	if err := validateCheckpointEnforcementDispatchV1(request, dispatch, now); err != nil {
		return ports.CurrentCheckpointRestoreDispatchEnforcementV1{}, err
	}
	sandbox, err := g.Sandbox.InspectCheckpointRestoreDispatchSandboxCurrentV1(ctx, request.Operation, request.EffectID, request.Reservation)
	if err != nil {
		return ports.CurrentCheckpointRestoreDispatchEnforcementV1{}, err
	}
	stage := ports.CheckpointRestoreDispatchSandboxPrePrepareV1
	if request.Phase == ports.OperationDispatchEnforcementExecuteV4 {
		stage = ports.CheckpointRestoreDispatchSandboxPreExecuteV1
	}
	if err := sandbox.ValidateCurrent(request.Operation, request.EffectID, request.Reservation, stage, now); err != nil {
		return ports.CurrentCheckpointRestoreDispatchEnforcementV1{}, err
	}
	legacy := dispatch.Record.Permit.LegacyPermit
	if sandbox.ProjectionDigest != request.SandboxProjectionDigest || sandbox.IntentRevision != legacy.IntentRevision || sandbox.IntentDigest != legacy.IntentDigest || sandbox.DispatchAttempt.ID != request.AttemptID || sandbox.Verifier != request.Verifier {
		return ports.CurrentCheckpointRestoreDispatchEnforcementV1{}, core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "checkpoint enforcement expected another Sandbox current projection")
	}
	if err := validateCheckpointPreparedAttemptV1(request, dispatch, sandbox); err != nil {
		return ports.CurrentCheckpointRestoreDispatchEnforcementV1{}, err
	}
	expires := minCheckpointEnforcementExpiryV1(legacy.ExpiresUnixNano, sandbox.ExpiresUnixNano)
	if request.PreparedAttempt != nil {
		expires = minCheckpointEnforcementExpiryV1(expires, request.PreparedAttempt.ExpiresUnixNano)
	}
	if now.UnixNano() >= expires {
		return ports.CurrentCheckpointRestoreDispatchEnforcementV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitExpired, "checkpoint enforcement has no current TTL")
	}
	receipt, err := ports.SealOperationDispatchEnforcementPhaseReceiptV4(ports.OperationDispatchEnforcementPhaseReceiptV4{
		Phase: request.Phase, Operation: request.Operation, OperationDigest: sandbox.OperationDigest,
		EffectID: request.EffectID, IntentRevision: legacy.IntentRevision, IntentDigest: legacy.IntentDigest,
		PermitID: request.PermitID, PermitFactRevision: dispatch.Record.Revision, PermitDigest: dispatch.Record.PermitDigest,
		AdmissionDigest: request.AdmissionDigest, ReviewAuthorization: request.ReviewAuthorization,
		AttemptID: request.AttemptID, SandboxAttempt: sandbox.DispatchAttempt, Verifier: request.Verifier,
		CheckpointSandbox: &sandbox, Prepare: request.Prepare, PreparedAttempt: request.PreparedAttempt,
		ValidatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
	})
	if err != nil {
		return ports.CurrentCheckpointRestoreDispatchEnforcementV1{}, err
	}
	journal, err := g.Facts.AppendOperationDispatchEnforcementV4(ctx, AppendOperationDispatchEnforcementRequestV4{
		Operation: request.Operation, EffectID: request.EffectID, PermitID: request.PermitID,
		ExpectedJournalRevision: request.ExpectedJournalRevision, Receipt: receipt,
	})
	if err != nil {
		if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorIndeterminate) && !core.HasCategory(err, core.ErrorConflict) {
			return ports.CurrentCheckpointRestoreDispatchEnforcementV1{}, err
		}
		recovered, inspectErr := g.Facts.InspectOperationDispatchEnforcementV4(context.WithoutCancel(ctx), request.Operation, request.EffectID, request.PermitID)
		if inspectErr != nil || !journalContainsReceiptV4(recovered, receipt) {
			return ports.CurrentCheckpointRestoreDispatchEnforcementV1{}, err
		}
		journal = recovered
	}
	if !journalContainsReceiptV4(journal, receipt) {
		return ports.CurrentCheckpointRestoreDispatchEnforcementV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Effect Owner returned another checkpoint enforcement receipt")
	}
	return g.currentEnvelope(dispatch, sandbox, journal, request.Phase, now)
}

func (g CheckpointRestoreDispatchEnforcementGatewayV1) InspectCurrentCheckpointRestoreDispatchV1(ctx context.Context, request ports.InspectCurrentCheckpointRestoreDispatchRequestV1) (ports.CurrentCheckpointRestoreDispatchEnforcementV1, error) {
	if err := g.validate(); err != nil {
		return ports.CurrentCheckpointRestoreDispatchEnforcementV1{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.CurrentCheckpointRestoreDispatchEnforcementV1{}, err
	}
	now := g.Clock()
	if now.IsZero() {
		return ports.CurrentCheckpointRestoreDispatchEnforcementV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "checkpoint enforcement clock returned zero")
	}
	journal, err := g.Facts.InspectOperationDispatchEnforcementV4(ctx, request.Operation, request.EffectID, request.PermitID)
	if err != nil {
		return ports.CurrentCheckpointRestoreDispatchEnforcementV1{}, err
	}
	if err := journal.Validate(); err != nil {
		return ports.CurrentCheckpointRestoreDispatchEnforcementV1{}, err
	}
	receipt := phaseReceiptV4(journal, request.Phase)
	if receipt == nil || receipt.CheckpointSandbox == nil || receipt.PermitDigest != request.PermitDigest || receipt.AdmissionDigest != request.AdmissionDigest || receipt.ReviewAuthorization != request.ReviewAuthorization || receipt.CheckpointSandbox.Reservation.Ref != request.Reservation || receipt.CheckpointSandbox.ProjectionDigest != request.SandboxProjectionDigest {
		return ports.CurrentCheckpointRestoreDispatchEnforcementV1{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "checkpoint enforcement Inspect expected other immutable facts")
	}
	dispatch, err := g.inspectDispatchCurrent(ctx, request.Operation, request.EffectID, request.PermitID, request.AdmissionDigest, request.ReviewAuthorization)
	if err != nil {
		return ports.CurrentCheckpointRestoreDispatchEnforcementV1{}, err
	}
	sandbox, err := g.Sandbox.InspectCheckpointRestoreDispatchSandboxCurrentV1(ctx, request.Operation, request.EffectID, request.Reservation)
	if err != nil {
		return ports.CurrentCheckpointRestoreDispatchEnforcementV1{}, err
	}
	stage := ports.CheckpointRestoreDispatchSandboxPrePrepareV1
	if request.Phase == ports.OperationDispatchEnforcementExecuteV4 {
		stage = ports.CheckpointRestoreDispatchSandboxPreExecuteV1
	}
	if err := sandbox.ValidateCurrent(request.Operation, request.EffectID, request.Reservation, stage, now); err != nil {
		return ports.CurrentCheckpointRestoreDispatchEnforcementV1{}, err
	}
	if sandbox.ProjectionDigest != request.SandboxProjectionDigest || dispatch.Record.Revision != receipt.PermitFactRevision || dispatch.Record.PermitDigest != receipt.PermitDigest || sandbox.DispatchAttempt != receipt.SandboxAttempt {
		return ports.CurrentCheckpointRestoreDispatchEnforcementV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "checkpoint dispatch or Sandbox current watermarks drifted")
	}
	return g.currentEnvelope(dispatch, sandbox, journal, request.Phase, now)
}

func (g CheckpointRestoreDispatchEnforcementGatewayV1) inspectDispatchCurrent(ctx context.Context, operation ports.OperationSubjectV3, effectID core.EffectIntentID, permitID string, admissionDigest core.Digest, authorization ports.OperationReviewAuthorizationRefV4) (ports.CurrentOperationDispatchAuthorizationV4, error) {
	current, err := g.Dispatch.InspectCurrentOperationDispatchV4(ctx, ports.InspectCurrentOperationDispatchRequestV4{
		Inspect:         ports.InspectOperationDispatchRecordRequestV4{Operation: operation, EffectID: effectID, PermitID: permitID},
		AdmissionDigest: admissionDigest, ReviewAuthorization: authorization,
	})
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	if err := current.Validate(); err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	return current, nil
}

func (g CheckpointRestoreDispatchEnforcementGatewayV1) currentEnvelope(dispatch ports.CurrentOperationDispatchAuthorizationV4, sandbox ports.CheckpointRestoreDispatchSandboxCurrentProjectionV1, journal ports.OperationDispatchEnforcementJournalV4, phase ports.OperationDispatchEnforcementPhaseV4, now time.Time) (ports.CurrentCheckpointRestoreDispatchEnforcementV1, error) {
	ref, err := journal.PhaseRefV4(phase)
	if err != nil {
		return ports.CurrentCheckpointRestoreDispatchEnforcementV1{}, err
	}
	expires := minCheckpointEnforcementExpiryV1(ref.ExpiresUnixNano, dispatch.Record.Permit.LegacyPermit.ExpiresUnixNano)
	expires = minCheckpointEnforcementExpiryV1(expires, sandbox.ExpiresUnixNano)
	return ports.SealCurrentCheckpointRestoreDispatchEnforcementV1(ports.CurrentCheckpointRestoreDispatchEnforcementV1{
		Dispatch: dispatch, Sandbox: sandbox, Journal: journal, Phase: ref,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
	}, now)
}

func validateCheckpointEnforcementDispatchV1(request ports.EnforceCurrentCheckpointRestoreDispatchRequestV1, current ports.CurrentOperationDispatchAuthorizationV4, now time.Time) error {
	record := current.Record
	legacy := record.Permit.LegacyPermit
	if record.State != ports.OperationPermitBegunV4 || record.Revision != request.ExpectedPermitFactRevision || record.PermitDigest != request.PermitDigest || record.Permit.Admission.Digest != request.AdmissionDigest || record.Permit.Admission.Authorization != request.ReviewAuthorization || legacy.IntentID != request.EffectID || !ports.SameOperationSubjectV3(legacy.Operation, request.Operation) || legacy.ID != request.PermitID || legacy.AttemptID != request.AttemptID || legacy.EnforcementPoint != request.Verifier || now.IsZero() || !now.Before(time.Unix(0, legacy.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "checkpoint enforcement requires the exact current begun V4 Permit")
	}
	return nil
}

func validateCheckpointPreparedAttemptV1(request ports.EnforceCurrentCheckpointRestoreDispatchRequestV1, current ports.CurrentOperationDispatchAuthorizationV4, sandbox ports.CheckpointRestoreDispatchSandboxCurrentProjectionV1) error {
	if request.Phase == ports.OperationDispatchEnforcementPrepareV4 {
		return nil
	}
	legacy := current.Record.Permit.LegacyPermit
	legacyDigest, err := legacy.DigestV3()
	if err != nil {
		return err
	}
	prepared := request.PreparedAttempt
	if prepared.PermitID != legacy.ID || prepared.PermitRevision != legacy.Revision || prepared.PermitDigest != legacyDigest || prepared.AttemptID != legacy.AttemptID || prepared.IntentID != legacy.IntentID || prepared.IntentRevision != legacy.IntentRevision || prepared.IntentDigest != legacy.IntentDigest || prepared.OperationDigest != mustOperationSubjectDigestV4(legacy.Operation) || prepared.Provider != legacy.EnforcementPoint || request.Prepare.PermitFactRevision != current.Record.Revision || request.Prepare.PermitDigest != current.Record.PermitDigest || request.Prepare.AdmissionDigest != current.Record.Permit.Admission.Digest || request.Prepare.ReviewAuthorization != current.ReviewAuthorization || request.Prepare.SandboxAttempt != sandbox.DispatchAttempt || sandbox.PrepareEnforcement == nil || sandbox.PreparedAttempt == nil || *sandbox.PrepareEnforcement != *request.Prepare || *sandbox.PreparedAttempt != *request.PreparedAttempt {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "checkpoint execute enforcement changed prepare or prepared Provider attempt")
	}
	return nil
}

func minCheckpointEnforcementExpiryV1(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func (g CheckpointRestoreDispatchEnforcementGatewayV1) validate() error {
	if g.Dispatch == nil || g.Sandbox == nil || g.Facts == nil || g.Clock == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "dispatch, checkpoint Sandbox current Owner, Effect Owner and clock are required")
	}
	return nil
}

var _ ports.CheckpointRestoreDispatchEnforcementGovernancePortV1 = CheckpointRestoreDispatchEnforcementGatewayV1{}

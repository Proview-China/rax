package control

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// OperationDispatchEnforcementGatewayV5 persists prepare/execute current-check
// receipts. It never invokes a Provider.
type OperationDispatchEnforcementGatewayV5 struct {
	Dispatch ports.OperationGovernancePortV5
	Sandbox  ports.OperationDispatchSandboxCurrentReaderV4
	Facts    OperationDispatchEnforcementFactPortV5
	Clock    func() time.Time
}

func (g OperationDispatchEnforcementGatewayV5) EnforceCurrentOperationDispatchV5(ctx context.Context, request ports.EnforceCurrentOperationDispatchRequestV5) (ports.CurrentOperationDispatchEnforcementV5, error) {
	if err := g.validateV5(); err != nil {
		return ports.CurrentOperationDispatchEnforcementV5{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.CurrentOperationDispatchEnforcementV5{}, err
	}
	baseline, err := g.baselineV5()
	if err != nil {
		return ports.CurrentOperationDispatchEnforcementV5{}, err
	}
	dispatch, sandbox, now, err := g.actualPointV5(ctx, request.Operation, request.EffectID, request.PermitID, request.AdmissionDigest, request.ReviewAuthorization, request.AuthorizationBasis, request.SandboxAttempt, baseline)
	if err != nil {
		return ports.CurrentOperationDispatchEnforcementV5{}, err
	}
	if err := validateEnforcementRequestAgainstDispatchV5(request, dispatch, now); err != nil {
		return ports.CurrentOperationDispatchEnforcementV5{}, err
	}
	permit := dispatch.Record.Permit
	if err := sandbox.ValidateCurrent(request.Operation, request.EffectID, permit.IntentRevision, permit.IntentDigest, request.AttemptID, request.Verifier, now); err != nil {
		return ports.CurrentOperationDispatchEnforcementV5{}, err
	}
	if sandbox.Attempt != request.SandboxAttempt || sandbox.Reservation != request.SandboxReservation || sandbox.ProjectionDigest != request.SandboxProjectionDigest {
		return ports.CurrentOperationDispatchEnforcementV5{}, core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "V5 enforcement expected another Sandbox projection")
	}
	if err := validatePreparedAttemptForEnforcementV5(request, dispatch); err != nil {
		return ports.CurrentOperationDispatchEnforcementV5{}, err
	}
	expires := minimumDispatchTimeV5(time.Unix(0, dispatch.ExpiresUnixNano), time.Unix(0, sandbox.ExpiresUnixNano))
	if request.PreparedAttempt != nil {
		expires = minimumDispatchTimeV5(expires, time.Unix(0, request.PreparedAttempt.ExpiresUnixNano))
	}
	if !expires.After(now) {
		return ports.CurrentOperationDispatchEnforcementV5{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitExpired, "V5 enforcement has no current TTL")
	}
	receipt, err := ports.SealOperationDispatchEnforcementPhaseReceiptV5(ports.OperationDispatchEnforcementPhaseReceiptV5{Phase: request.Phase, Operation: request.Operation, OperationDigest: sandbox.OperationDigest, EffectID: request.EffectID, IntentRevision: permit.IntentRevision, IntentDigest: permit.IntentDigest, PermitID: request.PermitID, PermitFactRevision: dispatch.Record.Revision, PermitDigest: dispatch.Record.PermitDigest, AdmissionDigest: request.AdmissionDigest, ReviewAuthorization: request.ReviewAuthorization, AuthorizationBasis: request.AuthorizationBasis, AttemptID: request.AttemptID, SandboxAttempt: request.SandboxAttempt, Verifier: request.Verifier, Sandbox: sandbox, Prepare: request.Prepare, PreparedAttempt: request.PreparedAttempt, ValidatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano()})
	if err != nil {
		return ports.CurrentOperationDispatchEnforcementV5{}, err
	}
	journal, err := g.Facts.AppendOperationDispatchEnforcementV5(ctx, AppendOperationDispatchEnforcementRequestV5{Operation: request.Operation, EffectID: request.EffectID, PermitID: request.PermitID, ExpectedJournalRevision: request.ExpectedJournalRevision, Receipt: receipt})
	if err != nil {
		if !unknownOrConflictDispatchV5(err) {
			return ports.CurrentOperationDispatchEnforcementV5{}, err
		}
		recovered, inspectErr := g.Facts.InspectOperationDispatchEnforcementV5(context.WithoutCancel(ctx), request.Operation, request.EffectID, request.PermitID)
		if inspectErr != nil || !journalContainsReceiptV5(recovered, receipt) {
			return ports.CurrentOperationDispatchEnforcementV5{}, err
		}
		journal = recovered
	}
	if !journalContainsReceiptV5(journal, receipt) {
		return ports.CurrentOperationDispatchEnforcementV5{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "V5 Effect Owner returned another enforcement receipt")
	}
	return g.currentEnvelopeV5(context.WithoutCancel(ctx), request.Operation, request.EffectID, request.PermitID, request.AdmissionDigest, request.ReviewAuthorization, request.AuthorizationBasis, request.SandboxAttempt, request.SandboxProjectionDigest, journal, request.Phase, now)
}

func (g OperationDispatchEnforcementGatewayV5) InspectOperationDispatchEnforcementV5(ctx context.Context, request ports.InspectOperationDispatchEnforcementRequestV5) (ports.OperationDispatchEnforcementJournalV5, error) {
	if nilDispatchV5(g.Facts) {
		return ports.OperationDispatchEnforcementJournalV5{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "V5 Effect Owner is required")
	}
	if err := request.Validate(); err != nil {
		return ports.OperationDispatchEnforcementJournalV5{}, err
	}
	journal, err := g.Facts.InspectOperationDispatchEnforcementV5(ctx, request.Operation, request.EffectID, request.PermitID)
	if err != nil {
		return ports.OperationDispatchEnforcementJournalV5{}, err
	}
	if err := journal.Validate(); err != nil {
		return ports.OperationDispatchEnforcementJournalV5{}, err
	}
	operationDigest, _ := request.Operation.DigestV3()
	if journal.OperationDigest != operationDigest || journal.EffectID != request.EffectID || journal.PermitID != request.PermitID || phaseReceiptV5(journal, request.Phase) == nil {
		return ports.OperationDispatchEnforcementJournalV5{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V5 enforcement history does not bind exact phase")
	}
	return journal, nil
}
func (g OperationDispatchEnforcementGatewayV5) InspectCurrentOperationDispatchEnforcementV5(ctx context.Context, request ports.InspectCurrentOperationDispatchEnforcementRequestV5) (ports.CurrentOperationDispatchEnforcementV5, error) {
	if err := g.validateV5(); err != nil {
		return ports.CurrentOperationDispatchEnforcementV5{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.CurrentOperationDispatchEnforcementV5{}, err
	}
	baseline, err := g.baselineV5()
	if err != nil {
		return ports.CurrentOperationDispatchEnforcementV5{}, err
	}
	journal, err := g.InspectOperationDispatchEnforcementV5(ctx, request.Inspect)
	if err != nil {
		return ports.CurrentOperationDispatchEnforcementV5{}, err
	}
	receipt := phaseReceiptV5(journal, request.Inspect.Phase)
	if receipt.PermitDigest != request.PermitDigest || receipt.AdmissionDigest != request.AdmissionDigest || receipt.ReviewAuthorization != request.ReviewAuthorization || receipt.AuthorizationBasis != request.AuthorizationBasis || receipt.SandboxAttempt != request.SandboxAttempt || receipt.Sandbox.ProjectionDigest != request.SandboxProjectionDigest {
		return ports.CurrentOperationDispatchEnforcementV5{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V5 current enforcement Inspect expected other immutable facts")
	}
	return g.currentEnvelopeV5(ctx, request.Inspect.Operation, request.Inspect.EffectID, request.Inspect.PermitID, request.AdmissionDigest, request.ReviewAuthorization, request.AuthorizationBasis, request.SandboxAttempt, request.SandboxProjectionDigest, journal, request.Inspect.Phase, baseline)
}

func (g OperationDispatchEnforcementGatewayV5) actualPointV5(ctx context.Context, operation ports.OperationSubjectV3, effectID core.EffectIntentID, permitID string, admission core.Digest, authorization ports.OperationReviewAuthorizationRefV5, basis ports.OperationReviewAuthorizationBasisV5, sandboxRef ports.OperationDispatchSandboxFactRefV4, previous time.Time) (ports.CurrentOperationDispatchAuthorizationV5, ports.OperationDispatchSandboxCurrentProjectionV4, time.Time, error) {
	request := ports.InspectCurrentOperationDispatchRequestV5{Inspect: ports.InspectOperationDispatchRecordRequestV5{Operation: operation, EffectID: effectID, PermitID: permitID}, AdmissionDigest: admission, ReviewAuthorization: authorization, AuthorizationBasis: basis}
	first, err := g.inspectDispatchV5(ctx, request)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, ports.OperationDispatchSandboxCurrentProjectionV4{}, time.Time{}, err
	}
	sandbox, err := g.inspectSandboxV5(ctx, operation, effectID, sandboxRef)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, ports.OperationDispatchSandboxCurrentProjectionV4{}, time.Time{}, err
	}
	second, err := g.inspectDispatchV5(ctx, request)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, ports.OperationDispatchSandboxCurrentProjectionV4{}, time.Time{}, err
	}
	now := g.Clock()
	if now.IsZero() || now.Before(previous) {
		return ports.CurrentOperationDispatchAuthorizationV5{}, ports.OperationDispatchSandboxCurrentProjectionV4{}, time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "V5 enforcement clock regressed across actual-point reads")
	}
	if first.Digest != second.Digest || first.Record.Digest != second.Record.Digest || first.ReviewAuthorization != authorization || second.ReviewAuthorization != authorization || second.AuthorizationBasis != basis || !now.Before(time.Unix(0, second.ExpiresUnixNano)) {
		return ports.CurrentOperationDispatchAuthorizationV5{}, ports.OperationDispatchSandboxCurrentProjectionV4{}, time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "V5 dispatch current changed across enforcement S1/S2")
	}
	return second, sandbox, now, nil
}

func (g OperationDispatchEnforcementGatewayV5) currentEnvelopeV5(ctx context.Context, operation ports.OperationSubjectV3, effectID core.EffectIntentID, permitID string, admission core.Digest, authorization ports.OperationReviewAuthorizationRefV5, basis ports.OperationReviewAuthorizationBasisV5, sandboxRef ports.OperationDispatchSandboxFactRefV4, sandboxDigest core.Digest, journal ports.OperationDispatchEnforcementJournalV5, phase ports.OperationDispatchEnforcementPhaseV4, previous time.Time) (ports.CurrentOperationDispatchEnforcementV5, error) {
	dispatch, sandbox, now, err := g.actualPointV5(ctx, operation, effectID, permitID, admission, authorization, basis, sandboxRef, previous)
	if err != nil {
		return ports.CurrentOperationDispatchEnforcementV5{}, err
	}
	receipt := phaseReceiptV5(journal, phase)
	if receipt == nil {
		return ports.CurrentOperationDispatchEnforcementV5{}, core.NewError(core.ErrorNotFound, core.ReasonDispatchPermitInvalid, "V5 enforcement phase absent")
	}
	if sandbox.ProjectionDigest != sandboxDigest || receipt.Sandbox.ProjectionDigest != sandboxDigest || receipt.PermitFactRevision != dispatch.Record.Revision || receipt.PermitDigest != dispatch.Record.PermitDigest {
		return ports.CurrentOperationDispatchEnforcementV5{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "V5 dispatch or Sandbox drifted from receipt")
	}
	if err := sandbox.ValidateCurrent(operation, effectID, dispatch.Record.Permit.IntentRevision, dispatch.Record.Permit.IntentDigest, receipt.AttemptID, receipt.Verifier, now); err != nil {
		return ports.CurrentOperationDispatchEnforcementV5{}, err
	}
	ref, err := journal.PhaseRefV5(phase)
	if err != nil {
		return ports.CurrentOperationDispatchEnforcementV5{}, err
	}
	expires := minimumDispatchTimeV5(time.Unix(0, receipt.ExpiresUnixNano), time.Unix(0, dispatch.ExpiresUnixNano), time.Unix(0, sandbox.ExpiresUnixNano))
	return ports.SealCurrentOperationDispatchEnforcementV5(ports.CurrentOperationDispatchEnforcementV5{Dispatch: dispatch, Sandbox: sandbox, Journal: journal, Phase: ref, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano()})
}

func validateEnforcementRequestAgainstDispatchV5(r ports.EnforceCurrentOperationDispatchRequestV5, current ports.CurrentOperationDispatchAuthorizationV5, now time.Time) error {
	record := current.Record
	permit := record.Permit
	if record.State != ports.OperationPermitBegunV5 || record.Revision != r.ExpectedPermitFactRevision || record.PermitDigest != r.PermitDigest || permit.Admission.Digest != r.AdmissionDigest || permit.Authorization != r.ReviewAuthorization || permit.AuthorizationBasis != r.AuthorizationBasis || permit.IntentID != r.EffectID || !ports.SameOperationSubjectV3(permit.Operation, r.Operation) || permit.ID != r.PermitID || permit.AttemptID != r.AttemptID || permit.EnforcementPoint != r.Verifier || now.IsZero() || !now.Before(time.Unix(0, permit.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "V5 enforcement requires exact begun Permit")
	}
	return nil
}
func validatePreparedAttemptForEnforcementV5(r ports.EnforceCurrentOperationDispatchRequestV5, current ports.CurrentOperationDispatchAuthorizationV5) error {
	if r.Phase == ports.OperationDispatchEnforcementPrepareV4 {
		return nil
	}
	permit := current.Record.Permit
	prepared := r.PreparedAttempt
	if prepared.PermitID != permit.ID || prepared.PermitRevision != permit.Revision || prepared.PermitDigest != permit.Digest || prepared.AttemptID != permit.AttemptID || prepared.IntentID != permit.IntentID || prepared.IntentRevision != permit.IntentRevision || prepared.IntentDigest != permit.IntentDigest || prepared.OperationDigest != mustOperationSubjectDigestV5(permit.Operation) || prepared.Provider != permit.EnforcementPoint || r.Prepare.PermitFactRevision != current.Record.Revision || r.Prepare.PermitDigest != current.Record.PermitDigest || r.Prepare.AdmissionDigest != permit.Admission.Digest || r.Prepare.ReviewAuthorization != current.ReviewAuthorization || r.Prepare.AuthorizationBasis != current.AuthorizationBasis || r.Prepare.SandboxAttempt != r.SandboxAttempt {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V5 execute changed prepare or prepared attempt")
	}
	return nil
}
func journalContainsReceiptV5(j ports.OperationDispatchEnforcementJournalV5, r ports.OperationDispatchEnforcementPhaseReceiptV5) bool {
	if j.Validate() != nil {
		return false
	}
	stored := phaseReceiptV5(j, r.Phase)
	return stored != nil && stored.Digest == r.Digest
}
func phaseReceiptV5(j ports.OperationDispatchEnforcementJournalV5, p ports.OperationDispatchEnforcementPhaseV4) *ports.OperationDispatchEnforcementPhaseReceiptV5 {
	if p == ports.OperationDispatchEnforcementPrepareV4 {
		return j.Prepare
	}
	if p == ports.OperationDispatchEnforcementExecuteV4 {
		return j.Execute
	}
	return nil
}
func (g OperationDispatchEnforcementGatewayV5) inspectDispatchV5(ctx context.Context, r ports.InspectCurrentOperationDispatchRequestV5) (ports.CurrentOperationDispatchAuthorizationV5, error) {
	v, e := g.Dispatch.InspectCurrentOperationDispatchV5(ctx, r)
	if unknownDispatchV5(e) {
		return g.Dispatch.InspectCurrentOperationDispatchV5(context.WithoutCancel(ctx), r)
	}
	return v, e
}
func (g OperationDispatchEnforcementGatewayV5) inspectSandboxV5(ctx context.Context, o ports.OperationSubjectV3, id core.EffectIntentID, ref ports.OperationDispatchSandboxFactRefV4) (ports.OperationDispatchSandboxCurrentProjectionV4, error) {
	v, e := g.Sandbox.InspectOperationDispatchSandboxCurrentV4(ctx, o, id, ref)
	if unknownDispatchV5(e) {
		return g.Sandbox.InspectOperationDispatchSandboxCurrentV4(context.WithoutCancel(ctx), o, id, ref)
	}
	return v, e
}
func (g OperationDispatchEnforcementGatewayV5) baselineV5() (time.Time, error) {
	now := g.Clock()
	if now.IsZero() {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "V5 enforcement baseline clock is zero")
	}
	return now, nil
}
func (g OperationDispatchEnforcementGatewayV5) validateV5() error {
	if nilDispatchV5(g.Dispatch) || nilDispatchV5(g.Sandbox) || nilDispatchV5(g.Facts) || g.Clock == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "V5 dispatch, Sandbox, Effect Owner and clock are required")
	}
	return nil
}
func mustOperationSubjectDigestV5(o ports.OperationSubjectV3) core.Digest {
	d, _ := o.DigestV3()
	return d
}

var _ ports.OperationDispatchEnforcementGovernancePortV5 = OperationDispatchEnforcementGatewayV5{}

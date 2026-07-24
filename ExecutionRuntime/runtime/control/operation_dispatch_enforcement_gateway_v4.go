package control

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// OperationDispatchEnforcementGatewayV4 performs the third independent
// governance check and persists a phase receipt. It never calls a Provider.
type OperationDispatchEnforcementGatewayV4 struct {
	Dispatch ports.OperationGovernancePortV4
	Sandbox  ports.OperationDispatchSandboxCurrentReaderV4
	Facts    OperationDispatchEnforcementFactPortV4
	Clock    func() time.Time
}

func (g OperationDispatchEnforcementGatewayV4) EnforceCurrentOperationDispatchV4(ctx context.Context, request ports.EnforceCurrentOperationDispatchRequestV4) (ports.CurrentOperationDispatchEnforcementV4, error) {
	if err := g.validate(); err != nil {
		return ports.CurrentOperationDispatchEnforcementV4{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.CurrentOperationDispatchEnforcementV4{}, err
	}
	now := g.Clock()
	if now.IsZero() {
		return ports.CurrentOperationDispatchEnforcementV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "enforcement gateway clock returned zero")
	}
	dispatch, err := g.inspectDispatchCurrent(ctx, request.Operation, request.EffectID, request.PermitID, request.AdmissionDigest, request.ReviewAuthorization)
	if err != nil {
		return ports.CurrentOperationDispatchEnforcementV4{}, err
	}
	if err := validateEnforcementRequestAgainstDispatchV4(request, dispatch, now); err != nil {
		return ports.CurrentOperationDispatchEnforcementV4{}, err
	}
	sandbox, err := g.Sandbox.InspectOperationDispatchSandboxCurrentV4(ctx, request.Operation, request.EffectID, request.SandboxAttempt)
	if err != nil {
		return ports.CurrentOperationDispatchEnforcementV4{}, err
	}
	legacy := dispatch.Record.Permit.LegacyPermit
	if err := sandbox.ValidateCurrent(request.Operation, request.EffectID, legacy.IntentRevision, legacy.IntentDigest, request.AttemptID, request.Verifier, now); err != nil {
		return ports.CurrentOperationDispatchEnforcementV4{}, err
	}
	if sandbox.Attempt != request.SandboxAttempt || sandbox.Reservation != request.SandboxReservation || sandbox.ProjectionDigest != request.SandboxProjectionDigest {
		return ports.CurrentOperationDispatchEnforcementV4{}, core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "enforcement request expected another sandbox attempt")
	}
	if err := validatePreparedAttemptForEnforcementV4(request, dispatch); err != nil {
		return ports.CurrentOperationDispatchEnforcementV4{}, err
	}
	expires := dispatch.Record.Permit.LegacyPermit.ExpiresUnixNano
	if sandbox.ExpiresUnixNano < expires {
		expires = sandbox.ExpiresUnixNano
	}
	if request.PreparedAttempt != nil && request.PreparedAttempt.ExpiresUnixNano < expires {
		expires = request.PreparedAttempt.ExpiresUnixNano
	}
	if now.UnixNano() >= expires {
		return ports.CurrentOperationDispatchEnforcementV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitExpired, "enforcement phase has no current TTL")
	}
	receipt, err := ports.SealOperationDispatchEnforcementPhaseReceiptV4(ports.OperationDispatchEnforcementPhaseReceiptV4{
		Phase: request.Phase, Operation: request.Operation, OperationDigest: sandbox.OperationDigest,
		EffectID: request.EffectID, IntentRevision: legacy.IntentRevision, IntentDigest: legacy.IntentDigest,
		PermitID: request.PermitID, PermitFactRevision: dispatch.Record.Revision, PermitDigest: dispatch.Record.PermitDigest,
		AdmissionDigest: request.AdmissionDigest, ReviewAuthorization: request.ReviewAuthorization,
		AttemptID: request.AttemptID, SandboxAttempt: request.SandboxAttempt, Verifier: request.Verifier, Sandbox: sandbox,
		Prepare: request.Prepare, PreparedAttempt: request.PreparedAttempt,
		ValidatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
	})
	if err != nil {
		return ports.CurrentOperationDispatchEnforcementV4{}, err
	}
	journal, err := g.Facts.AppendOperationDispatchEnforcementV4(ctx, AppendOperationDispatchEnforcementRequestV4{
		Operation: request.Operation, EffectID: request.EffectID, PermitID: request.PermitID,
		ExpectedJournalRevision: request.ExpectedJournalRevision, Receipt: receipt,
	})
	if err != nil {
		if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorIndeterminate) && !core.HasCategory(err, core.ErrorConflict) {
			return ports.CurrentOperationDispatchEnforcementV4{}, err
		}
		recovered, inspectErr := g.Facts.InspectOperationDispatchEnforcementV4(context.WithoutCancel(ctx), request.Operation, request.EffectID, request.PermitID)
		if inspectErr != nil || !journalContainsReceiptV4(recovered, receipt) {
			return ports.CurrentOperationDispatchEnforcementV4{}, err
		}
		journal = recovered
	}
	if !journalContainsReceiptV4(journal, receipt) {
		return ports.CurrentOperationDispatchEnforcementV4{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Effect Owner returned another enforcement receipt")
	}
	return g.currentEnvelope(ctx, dispatch, sandbox, journal, request.Phase, now)
}

func (g OperationDispatchEnforcementGatewayV4) InspectOperationDispatchEnforcementV4(ctx context.Context, request ports.InspectOperationDispatchEnforcementRequestV4) (ports.OperationDispatchEnforcementJournalV4, error) {
	if g.Facts == nil {
		return ports.OperationDispatchEnforcementJournalV4{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Effect Owner is required for historical enforcement Inspect")
	}
	if err := request.Validate(); err != nil {
		return ports.OperationDispatchEnforcementJournalV4{}, err
	}
	journal, err := g.Facts.InspectOperationDispatchEnforcementV4(ctx, request.Operation, request.EffectID, request.PermitID)
	if err != nil {
		return ports.OperationDispatchEnforcementJournalV4{}, err
	}
	if err := journal.Validate(); err != nil {
		return ports.OperationDispatchEnforcementJournalV4{}, err
	}
	operationDigest, _ := request.Operation.DigestV3()
	if journal.OperationDigest != operationDigest || journal.EffectID != request.EffectID || journal.PermitID != request.PermitID || phaseReceiptV4(journal, request.Phase) == nil {
		return ports.OperationDispatchEnforcementJournalV4{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "historical enforcement journal does not bind the exact dispatch phase")
	}
	return journal, nil
}

func (g OperationDispatchEnforcementGatewayV4) InspectCurrentOperationDispatchEnforcementV4(ctx context.Context, request ports.InspectCurrentOperationDispatchEnforcementRequestV4) (ports.CurrentOperationDispatchEnforcementV4, error) {
	if err := g.validate(); err != nil {
		return ports.CurrentOperationDispatchEnforcementV4{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.CurrentOperationDispatchEnforcementV4{}, err
	}
	now := g.Clock()
	if now.IsZero() {
		return ports.CurrentOperationDispatchEnforcementV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "enforcement gateway clock returned zero")
	}
	journal, err := g.InspectOperationDispatchEnforcementV4(ctx, request.Inspect)
	if err != nil {
		return ports.CurrentOperationDispatchEnforcementV4{}, err
	}
	receipt := phaseReceiptV4(journal, request.Inspect.Phase)
	if receipt.PermitDigest != request.PermitDigest || receipt.AdmissionDigest != request.AdmissionDigest || receipt.ReviewAuthorization != request.ReviewAuthorization || receipt.SandboxAttempt != request.SandboxAttempt || receipt.Sandbox.ProjectionDigest != request.SandboxProjectionDigest {
		return ports.CurrentOperationDispatchEnforcementV4{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "current enforcement Inspect expected other immutable facts")
	}
	dispatch, err := g.inspectDispatchCurrent(ctx, request.Inspect.Operation, request.Inspect.EffectID, request.Inspect.PermitID, request.AdmissionDigest, request.ReviewAuthorization)
	if err != nil {
		return ports.CurrentOperationDispatchEnforcementV4{}, err
	}
	sandbox, err := g.Sandbox.InspectOperationDispatchSandboxCurrentV4(ctx, request.Inspect.Operation, request.Inspect.EffectID, request.SandboxAttempt)
	if err != nil {
		return ports.CurrentOperationDispatchEnforcementV4{}, err
	}
	legacy := dispatch.Record.Permit.LegacyPermit
	if err := sandbox.ValidateCurrent(request.Inspect.Operation, request.Inspect.EffectID, legacy.IntentRevision, legacy.IntentDigest, receipt.AttemptID, receipt.Verifier, now); err != nil {
		return ports.CurrentOperationDispatchEnforcementV4{}, err
	}
	if sandbox.Attempt != request.SandboxAttempt || receipt.SandboxAttempt != request.SandboxAttempt || sandbox.ProjectionDigest != request.SandboxProjectionDigest || receipt.Sandbox.ProjectionDigest != sandbox.ProjectionDigest || dispatch.Record.Revision != receipt.PermitFactRevision || dispatch.Record.PermitDigest != receipt.PermitDigest {
		return ports.CurrentOperationDispatchEnforcementV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "current dispatch or sandbox watermarks drifted from enforcement receipt")
	}
	return g.currentEnvelope(ctx, dispatch, sandbox, journal, request.Inspect.Phase, now)
}

func (g OperationDispatchEnforcementGatewayV4) inspectDispatchCurrent(ctx context.Context, operation ports.OperationSubjectV3, effectID core.EffectIntentID, permitID string, admissionDigest core.Digest, authorization ports.OperationReviewAuthorizationRefV4) (ports.CurrentOperationDispatchAuthorizationV4, error) {
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

func (g OperationDispatchEnforcementGatewayV4) currentEnvelope(ctx context.Context, dispatch ports.CurrentOperationDispatchAuthorizationV4, sandbox ports.OperationDispatchSandboxCurrentProjectionV4, journal ports.OperationDispatchEnforcementJournalV4, phase ports.OperationDispatchEnforcementPhaseV4, now time.Time) (ports.CurrentOperationDispatchEnforcementV4, error) {
	receipt := phaseReceiptV4(journal, phase)
	if receipt == nil {
		return ports.CurrentOperationDispatchEnforcementV4{}, core.NewError(core.ErrorNotFound, core.ReasonDispatchPermitInvalid, "enforcement phase was not persisted")
	}
	ref, err := journal.PhaseRefV4(phase)
	if err != nil {
		return ports.CurrentOperationDispatchEnforcementV4{}, err
	}
	expires := receipt.ExpiresUnixNano
	if dispatch.Record.Permit.LegacyPermit.ExpiresUnixNano < expires {
		expires = dispatch.Record.Permit.LegacyPermit.ExpiresUnixNano
	}
	if sandbox.ExpiresUnixNano < expires {
		expires = sandbox.ExpiresUnixNano
	}
	return ports.SealCurrentOperationDispatchEnforcementV4(ports.CurrentOperationDispatchEnforcementV4{
		Dispatch: dispatch, Sandbox: sandbox, Journal: journal, Phase: ref,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
	})
}

func validateEnforcementRequestAgainstDispatchV4(request ports.EnforceCurrentOperationDispatchRequestV4, current ports.CurrentOperationDispatchAuthorizationV4, now time.Time) error {
	record := current.Record
	legacy := record.Permit.LegacyPermit
	if record.State != ports.OperationPermitBegunV4 || record.Revision != request.ExpectedPermitFactRevision || record.PermitDigest != request.PermitDigest || record.Permit.Admission.Digest != request.AdmissionDigest || record.Permit.Admission.Authorization != request.ReviewAuthorization || legacy.IntentID != request.EffectID || !ports.SameOperationSubjectV3(legacy.Operation, request.Operation) || legacy.ID != request.PermitID || legacy.AttemptID != request.AttemptID || legacy.EnforcementPoint != request.Verifier || now.IsZero() || !now.Before(time.Unix(0, legacy.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "enforcement requires the exact current begun V4 Permit and verifier")
	}
	return nil
}

func validatePreparedAttemptForEnforcementV4(request ports.EnforceCurrentOperationDispatchRequestV4, current ports.CurrentOperationDispatchAuthorizationV4) error {
	if request.Phase == ports.OperationDispatchEnforcementPrepareV4 {
		return nil
	}
	legacy := current.Record.Permit.LegacyPermit
	legacyDigest, err := legacy.DigestV3()
	if err != nil {
		return err
	}
	prepared := request.PreparedAttempt
	if prepared.PermitID != legacy.ID || prepared.PermitRevision != legacy.Revision || prepared.PermitDigest != legacyDigest || prepared.AttemptID != legacy.AttemptID || prepared.IntentID != legacy.IntentID || prepared.IntentRevision != legacy.IntentRevision || prepared.IntentDigest != legacy.IntentDigest || prepared.OperationDigest != mustOperationSubjectDigestV4(legacy.Operation) || prepared.Provider != legacy.EnforcementPoint || request.Prepare.PermitFactRevision != current.Record.Revision || request.Prepare.PermitDigest != current.Record.PermitDigest || request.Prepare.AdmissionDigest != current.Record.Permit.Admission.Digest || request.Prepare.ReviewAuthorization != current.ReviewAuthorization || request.Prepare.SandboxAttempt != request.SandboxAttempt {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "execute enforcement changed prepare or prepared provider attempt")
	}
	return nil
}

func journalContainsReceiptV4(journal ports.OperationDispatchEnforcementJournalV4, receipt ports.OperationDispatchEnforcementPhaseReceiptV4) bool {
	if err := journal.Validate(); err != nil {
		return false
	}
	stored := phaseReceiptV4(journal, receipt.Phase)
	return stored != nil && stored.Digest == receipt.Digest
}

func phaseReceiptV4(journal ports.OperationDispatchEnforcementJournalV4, phase ports.OperationDispatchEnforcementPhaseV4) *ports.OperationDispatchEnforcementPhaseReceiptV4 {
	if phase == ports.OperationDispatchEnforcementPrepareV4 {
		return journal.Prepare
	}
	if phase == ports.OperationDispatchEnforcementExecuteV4 {
		return journal.Execute
	}
	return nil
}

func mustOperationSubjectDigestV4(subject ports.OperationSubjectV3) core.Digest {
	digest, _ := subject.DigestV3()
	return digest
}

func (g OperationDispatchEnforcementGatewayV4) validate() error {
	if g.Dispatch == nil || g.Sandbox == nil || g.Facts == nil || g.Clock == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "dispatch, sandbox, Effect Owner and clock are required for enforcement")
	}
	return nil
}

package control

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type AppendRestoreStageEnforcementRequestV1 struct {
	ExpectedRevision core.Revision
	Next             ports.RestoreStageEnforcementJournalV1
}

type RestoreStageEnforcementFactPortV1 interface {
	AppendRestoreStageEnforcementV1(context.Context, AppendRestoreStageEnforcementRequestV1) (ports.RestoreStageEnforcementJournalV1, error)
	InspectRestoreStageEnforcementV1(context.Context, ports.OperationSubjectV3, core.EffectIntentID, string) (ports.RestoreStageEnforcementJournalV1, error)
}

// RestoreStageEnforcementGatewayV1 is the restore-specific actual-point gate.
// It persists governance receipts only and never invokes a Provider.
type RestoreStageEnforcementGatewayV1 struct {
	Dispatch ports.OperationGovernancePortV4
	Sandbox  ports.RestoreStageSandboxCurrentReaderV1
	Facts    RestoreStageEnforcementFactPortV1
	Clock    func() time.Time
}

func (g RestoreStageEnforcementGatewayV1) EnforceRestoreStageDispatchV1(ctx context.Context, request ports.EnforceRestoreStageDispatchRequestV1) (ports.OperationDispatchEnforcementPhaseRefV4, error) {
	if err := g.validate(); err != nil {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, err
	}
	now := g.Clock()
	if now.IsZero() {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Restore Stage enforcement clock returned zero")
	}
	dispatch, err := g.inspectDispatchV1(ctx, request.Operation, request.EffectID, request.PermitID, request.AdmissionDigest, request.ReviewAuthorization)
	if err != nil {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, err
	}
	if err := validateRestoreStageDispatchV1(request, dispatch, now); err != nil {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, err
	}
	sandboxRequest := ports.InspectRestoreStageSandboxCurrentRequestV1{Operation: request.Operation, EffectID: request.EffectID, IntentRevision: request.DispatchAttempt.IntentRevision, IntentDigest: request.DispatchAttempt.IntentDigest, DispatchAttempt: request.DispatchAttempt, SandboxAttempt: request.SandboxAttempt, RestoreAttempt: request.RestoreAttempt, Eligibility: request.Eligibility, Identity: request.Identity, SnapshotArtifact: request.SnapshotArtifact, Provider: request.Verifier}
	sandbox, err := g.Sandbox.InspectRestoreStageSandboxCurrentV1(ctx, sandboxRequest)
	if err != nil {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, err
	}
	if err := sandbox.ValidateCurrent(now); err != nil {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, err
	}
	if sandbox.SandboxAttempt != request.SandboxAttempt || sandbox.ProjectionDigest != request.SandboxProjectionDigest || sandbox.Prepared.Provider != request.Verifier {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "Restore Stage enforcement expected another Sandbox prepared attempt")
	}
	if request.Phase == ports.OperationDispatchEnforcementExecuteV4 && (*request.Prepared != sandbox.Prepared || request.Prepare.SandboxAttempt != request.SandboxAttempt) {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "Restore Stage execute changed the prepared closure")
	}
	expires := minRestoreStageEnforcementTimeV1(dispatch.Record.Permit.LegacyPermit.ExpiresUnixNano, sandbox.ExpiresUnixNano)
	if request.Prepared != nil {
		expires = minRestoreStageEnforcementTimeV1(expires, request.Prepared.ExpiresUnixNano)
	}
	if now.UnixNano() >= expires {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitExpired, "Restore Stage enforcement has no current TTL")
	}
	receiptDigest, err := restoreStageEnforcementReceiptDigestV1(request, dispatch.Record.Digest, sandbox.ProjectionDigest)
	if err != nil {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, err
	}
	ref := ports.OperationDispatchEnforcementPhaseRefV4{OperationDigest: request.DispatchAttempt.OperationDigest, EffectID: request.EffectID, PermitID: request.PermitID, PermitFactRevision: dispatch.Record.Revision, PermitDigest: dispatch.Record.PermitDigest, AdmissionDigest: request.AdmissionDigest, ReviewAuthorization: request.ReviewAuthorization, AttemptID: request.DispatchAttempt.AttemptID, SandboxAttempt: request.SandboxAttempt, Phase: request.Phase, ReceiptDigest: receiptDigest, JournalRevision: request.ExpectedJournalRevision + 1, ValidatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires}
	if request.Phase == ports.OperationDispatchEnforcementExecuteV4 {
		ref.PrepareReceiptDigest = request.Prepare.ReceiptDigest
		ref.PreparedAttemptDigest = request.Prepared.Digest
	}
	if err := ref.Validate(); err != nil {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, err
	}
	journal := ports.RestoreStageEnforcementJournalV1{Operation: request.Operation, OperationDigest: ref.OperationDigest, EffectID: request.EffectID, PermitID: request.PermitID, SandboxAttempt: request.SandboxAttempt, SandboxProjectionDigest: sandbox.ProjectionDigest, Sandbox: sandbox, Revision: ref.JournalRevision}
	if request.Phase == ports.OperationDispatchEnforcementPrepareV4 {
		journal.Prepare = &ref
	} else {
		prepare := *request.Prepare
		journal.Prepare = &prepare
		journal.Execute = &ref
	}
	journal, err = ports.SealRestoreStageEnforcementJournalV1(journal)
	if err != nil {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, err
	}
	committed, appendErr := g.Facts.AppendRestoreStageEnforcementV1(ctx, AppendRestoreStageEnforcementRequestV1{ExpectedRevision: request.ExpectedJournalRevision, Next: journal})
	if appendErr != nil {
		committed, err = g.Facts.InspectRestoreStageEnforcementV1(context.WithoutCancel(ctx), request.Operation, request.EffectID, request.PermitID)
		if err != nil {
			return ports.OperationDispatchEnforcementPhaseRefV4{}, appendErr
		}
	}
	if committed.Validate() != nil || committed.Digest != journal.Digest {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Restore Stage enforcement append winner differs")
	}
	return ref, nil
}

func (g RestoreStageEnforcementGatewayV1) InspectCurrentRestoreStageDispatchEnforcementV1(ctx context.Context, operation ports.OperationSubjectV3, expected ports.OperationDispatchEnforcementPhaseRefV4) (ports.OperationDispatchEnforcementPhaseRefV4, error) {
	if err := g.validate(); err != nil {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, err
	}
	if operation.Validate() != nil || expected.Validate() != nil {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "Restore Stage enforcement current Inspect is invalid")
	}
	journal, err := g.Facts.InspectRestoreStageEnforcementV1(ctx, operation, expected.EffectID, expected.PermitID)
	if err != nil {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, err
	}
	if journal.Validate() != nil || !ports.SameOperationSubjectV3(journal.Operation, operation) {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "Restore Stage enforcement journal drifted")
	}
	actual := journal.Prepare
	if expected.Phase == ports.OperationDispatchEnforcementExecuteV4 {
		actual = journal.Execute
	}
	if actual == nil || *actual != expected {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "Restore Stage enforcement exact ref is absent")
	}
	now := g.Clock()
	if err := journal.Sandbox.ValidateCurrent(now); err != nil {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, err
	}
	dispatch, err := g.inspectDispatchV1(ctx, operation, expected.EffectID, expected.PermitID, expected.AdmissionDigest, expected.ReviewAuthorization)
	if err != nil {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, err
	}
	if dispatch.Record.State != ports.OperationPermitBegunV4 || dispatch.Record.Revision != expected.PermitFactRevision || dispatch.Record.PermitDigest != expected.PermitDigest || now.UnixNano() >= dispatch.Record.Permit.LegacyPermit.ExpiresUnixNano || now.UnixNano() >= expected.ExpiresUnixNano {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "Restore Stage enforcement upstream current drifted")
	}
	request := ports.InspectRestoreStageSandboxCurrentRequestV1{Operation: operation, EffectID: expected.EffectID, IntentRevision: journal.Sandbox.IntentRevision, IntentDigest: journal.Sandbox.IntentDigest, DispatchAttempt: journal.Sandbox.DispatchAttempt, SandboxAttempt: expected.SandboxAttempt, RestoreAttempt: journal.Sandbox.RestoreAttempt, Eligibility: journal.Sandbox.Eligibility, Identity: journal.Sandbox.Identity, SnapshotArtifact: journal.Sandbox.SnapshotArtifact, Provider: journal.Sandbox.Provider}
	sandbox, err := g.Sandbox.InspectRestoreStageSandboxCurrentV1(ctx, request)
	if err != nil || sandbox.ProjectionDigest != journal.SandboxProjectionDigest {
		if err != nil {
			return ports.OperationDispatchEnforcementPhaseRefV4{}, err
		}
		return ports.OperationDispatchEnforcementPhaseRefV4{}, core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "Restore Stage Sandbox current moved")
	}
	return expected, nil
}

func (g RestoreStageEnforcementGatewayV1) InspectRestoreStageDispatchEnforcementByRequestV1(ctx context.Context, request ports.EnforceRestoreStageDispatchRequestV1) (ports.OperationDispatchEnforcementPhaseRefV4, error) {
	if err := g.validate(); err != nil {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, err
	}
	journal, err := g.Facts.InspectRestoreStageEnforcementV1(ctx, request.Operation, request.EffectID, request.PermitID)
	if err != nil {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, err
	}
	if journal.Validate() != nil || !ports.SameOperationSubjectV3(journal.Operation, request.Operation) || journal.OperationDigest != request.DispatchAttempt.OperationDigest || journal.EffectID != request.EffectID || journal.PermitID != request.PermitID || journal.SandboxAttempt != request.SandboxAttempt || journal.SandboxProjectionDigest != request.SandboxProjectionDigest || journal.Sandbox.DispatchAttempt != request.DispatchAttempt || journal.Sandbox.RestoreAttempt != request.RestoreAttempt || journal.Sandbox.Eligibility != request.Eligibility || journal.Sandbox.Identity != request.Identity || journal.Sandbox.SnapshotArtifact != request.SnapshotArtifact || journal.Sandbox.Provider != request.Verifier {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "Restore Stage enforcement recovery request changed the exact closure")
	}
	actual := journal.Prepare
	if request.Phase == ports.OperationDispatchEnforcementExecuteV4 {
		actual = journal.Execute
	}
	if actual == nil || actual.OperationDigest != request.DispatchAttempt.OperationDigest || actual.EffectID != request.EffectID || actual.PermitID != request.PermitID || actual.PermitFactRevision != request.ExpectedPermitFactRevision || actual.PermitDigest != request.PermitDigest || actual.AdmissionDigest != request.AdmissionDigest || actual.ReviewAuthorization != request.ReviewAuthorization || actual.AttemptID != request.DispatchAttempt.AttemptID || actual.SandboxAttempt != request.SandboxAttempt || actual.Phase != request.Phase || actual.JournalRevision != request.ExpectedJournalRevision+1 {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "Restore Stage enforcement recovery found another request")
	}
	if request.Phase == ports.OperationDispatchEnforcementExecuteV4 && (actual.PrepareReceiptDigest != request.Prepare.ReceiptDigest || actual.PreparedAttemptDigest != request.Prepared.Digest) {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "Restore Stage execute recovery changed prepared refs")
	}
	return g.InspectCurrentRestoreStageDispatchEnforcementV1(ctx, request.Operation, *actual)
}

func (g RestoreStageEnforcementGatewayV1) InspectCurrentOperationProviderExecuteEnforcementV1(ctx context.Context, operation ports.OperationSubjectV3, expected ports.OperationDispatchEnforcementPhaseRefV4) (ports.OperationDispatchEnforcementPhaseRefV4, error) {
	if expected.Phase != ports.OperationDispatchEnforcementExecuteV4 {
		return ports.OperationDispatchEnforcementPhaseRefV4{}, core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "Restore Stage Provider current Reader requires execute Enforcement")
	}
	return g.InspectCurrentRestoreStageDispatchEnforcementV1(ctx, operation, expected)
}

func (g RestoreStageEnforcementGatewayV1) inspectDispatchV1(ctx context.Context, operation ports.OperationSubjectV3, effectID core.EffectIntentID, permitID string, admission core.Digest, authorization ports.OperationReviewAuthorizationRefV4) (ports.CurrentOperationDispatchAuthorizationV4, error) {
	return g.Dispatch.InspectCurrentOperationDispatchV4(ctx, ports.InspectCurrentOperationDispatchRequestV4{Inspect: ports.InspectOperationDispatchRecordRequestV4{Operation: operation, EffectID: effectID, PermitID: permitID}, AdmissionDigest: admission, ReviewAuthorization: authorization})
}

func validateRestoreStageDispatchV1(request ports.EnforceRestoreStageDispatchRequestV1, current ports.CurrentOperationDispatchAuthorizationV4, now time.Time) error {
	if current.Validate() != nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "Restore Stage dispatch current is invalid")
	}
	record := current.Record
	legacy := record.Permit.LegacyPermit
	if record.State != ports.OperationPermitBegunV4 || record.Revision != request.ExpectedPermitFactRevision || record.PermitDigest != request.PermitDigest || record.Permit.Admission.Digest != request.AdmissionDigest || record.Permit.Admission.Authorization != request.ReviewAuthorization || legacy.IntentID != request.EffectID || !ports.SameOperationSubjectV3(legacy.Operation, request.Operation) || legacy.ID != request.PermitID || legacy.AttemptID != request.DispatchAttempt.AttemptID || legacy.EnforcementPoint != request.Verifier || request.DispatchAttempt.PermitRevision != record.Revision || request.DispatchAttempt.PermitDigest != record.PermitDigest || now.IsZero() || now.UnixNano() >= legacy.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "Restore Stage enforcement requires exact current begun Permit")
	}
	return nil
}

func restoreStageEnforcementReceiptDigestV1(request ports.EnforceRestoreStageDispatchRequestV1, dispatch, sandbox core.Digest) (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.runtime.restore-stage-enforcement", ports.RestoreStageEnforcementContractVersionV1, "RestoreStageEnforcementReceiptV1", struct {
		Request  ports.EnforceRestoreStageDispatchRequestV1 `json:"request"`
		Dispatch core.Digest                                `json:"dispatch_digest"`
		Sandbox  core.Digest                                `json:"sandbox_projection_digest"`
	}{request, dispatch, sandbox})
}

func minRestoreStageEnforcementTimeV1(values ...int64) int64 {
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

func (g RestoreStageEnforcementGatewayV1) validate() error {
	for _, dependency := range []any{g.Dispatch, g.Sandbox, g.Facts, g.Clock} {
		if dependency == nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Restore Stage enforcement dependencies are required")
		}
		value := reflect.ValueOf(dependency)
		if value.Kind() == reflect.Pointer && value.IsNil() {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Restore Stage enforcement typed-nil dependency is forbidden")
		}
	}
	return nil
}

var _ ports.RestoreStageEnforcementGovernancePortV1 = RestoreStageEnforcementGatewayV1{}
var _ ports.OperationProviderExecuteEnforcementCurrentReaderV1 = RestoreStageEnforcementGatewayV1{}

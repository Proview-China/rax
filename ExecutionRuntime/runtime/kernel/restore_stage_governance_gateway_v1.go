package kernel

import (
	"context"
	"fmt"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// RestoreStageGovernanceGatewayV1 is a typed read-only join over the existing
// Restore V2 and Operation V3/V4 governance chains. It never issues Admission,
// Review, Permit, Begin, Enforcement, Evidence, Settlement, or Provider calls.
type RestoreStageGovernanceGatewayV1 struct {
	Restore         ports.RestoreGovernancePortV2
	Materialization ports.RestoreMaterializationCurrentReaderV1
	Effects         ports.ControlledOperationEffectCurrentReaderV2
	Admissions      ports.OperationEffectAdmissionPortV3
	Reviews         ports.OperationReviewAuthorizationGovernancePortV4
	Dispatch        ports.OperationGovernancePortV4
	Enforcement     ports.OperationProviderExecuteEnforcementCurrentReaderV1
	Clock           func() time.Time
}

func (g RestoreStageGovernanceGatewayV1) InspectRestoreStageGovernanceCurrentV1(ctx context.Context, request ports.InspectRestoreStageGovernanceCurrentRequestV1) (ports.RestoreStageGovernanceCurrentProjectionV1, error) {
	if err := request.Validate(); err != nil {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, err
	}
	dependencies := []struct {
		value any
		name  string
	}{{g.Restore, "Restore Governance V2"}, {g.Materialization, "Restore materialization current Reader"}, {g.Effects, "Operation Effect current Reader"}, {g.Admissions, "Operation Admission"}, {g.Reviews, "Review Authorization current"}, {g.Dispatch, "Operation Dispatch V4"}, {g.Enforcement, "execute Enforcement current Reader"}}
	for _, dependency := range dependencies {
		if err := requireCheckpointDependencyV2(dependency.value, dependency.name); err != nil {
			return ports.RestoreStageGovernanceCurrentProjectionV1{}, err
		}
	}
	if g.Clock == nil {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, core.NewError(core.ErrorUnavailable, core.ReasonRestoreIncompatible, "Restore Stage current clock is unavailable")
	}
	now := g.Clock()
	if now.IsZero() {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, core.NewError(core.ErrorUnavailable, core.ReasonRestoreIncompatible, "Restore Stage current clock is invalid")
	}

	attempt, err := g.Restore.InspectRestoreAttemptV2(ctx, ports.InspectRestoreAttemptRequestV2{TenantID: request.RestoreAttempt.TenantID, AttemptID: request.RestoreAttempt.ID})
	if err != nil {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, err
	}
	if attempt.Ref != request.RestoreAttempt || attempt.State != ports.RestoreAttemptEligibilityBoundV2 || attempt.Eligibility == nil || *attempt.Eligibility != request.Eligibility {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, restoreStageConflictV1("Restore Attempt or Eligibility binding is no longer exact current")
	}
	eligibility, err := g.Restore.InspectCurrentRestoreEligibilityV2(ctx, ports.InspectRestoreEligibilityCurrentRequestV2{Attempt: attempt.Ref, ExpectedEligibility: request.Eligibility})
	if err != nil {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, err
	}
	if eligibility.Ref != request.Eligibility || eligibility.Identity != attempt.OperationScope.Identity || eligibility.RestorePlan != attempt.OperationScope.RestorePlan {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, restoreStageConflictV1("Restore Eligibility current projection drifted")
	}
	materialization, err := g.Materialization.InspectRestoreMaterializationCurrentV1(ctx, ports.InspectRestoreMaterializationCurrentRequestV1{Attempt: request.RestoreAttempt, Eligibility: request.Eligibility})
	if err != nil {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, err
	}
	if err := materialization.ValidateCurrent(now); err != nil {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, err
	}
	if materialization.Attempt != request.RestoreAttempt || materialization.Eligibility != request.Eligibility || materialization.RestorePlan != eligibility.RestorePlan || materialization.Consistency != eligibility.CheckpointConsistency || materialization.Identity != eligibility.Identity || !materialization.ContainsSnapshotV1(request.SnapshotArtifact) {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, restoreStageConflictV1("Snapshot is not a member of the exact current materialization closure")
	}

	effect, err := g.Effects.InspectCurrentControlledOperationEffectV2(ctx, request.Operation, request.EffectID)
	if err != nil {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, err
	}
	if err := effect.Validate(now); err != nil {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, err
	}
	intent := effect.Intent
	if intent.Kind != ports.RestoreStageEffectKindV1 || !ports.SameOperationSubjectV3(intent.Operation, request.Operation) || intent.Operation.Kind != ports.RestoreStageOperationKindV1 || intent.Operation.CustomOperationID != request.RestoreAttempt.ID || intent.Payload.Ref != "" || intent.Payload.Inline == nil || intent.Payload.Schema.Namespace != ports.RestoreStagePayloadSchemaNamespaceV1 || intent.Payload.Schema.Name != ports.RestoreStagePayloadSchemaNameV1 || intent.Payload.Schema.Version != ports.RestoreStagePayloadSchemaVersionV1 {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, restoreStageConflictV1("Operation Effect is not the typed Restore Stage Intent")
	}
	payload, err := ports.DecodeRestoreStageOperationPayloadV1(intent.Payload.Inline)
	if err != nil {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, err
	}
	if payload.RestoreAttempt != request.RestoreAttempt || payload.Eligibility != request.Eligibility || payload.Identity != eligibility.Identity || payload.SnapshotArtifact != request.SnapshotArtifact || payload.SnapshotArtifact.TenantID != string(request.RestoreAttempt.TenantID) || payload.SnapshotArtifact.ScopeDigest != string(attempt.OperationScope.SourceScopeDigest) {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, restoreStageConflictV1("Restore Stage typed payload crosses Attempt, Eligibility, Identity, or Snapshot")
	}
	if request.Operation.ExecutionScope.Instance != eligibility.Identity.TargetInstance || request.Operation.ExecutionScope.SandboxLease == nil || *request.Operation.ExecutionScope.SandboxLease != eligibility.Identity.TargetLease || request.Operation.ExecutionScope.AuthorityEpoch != eligibility.Identity.TargetFenceEpoch {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, restoreStageConflictV1("Restore Stage operation is not fenced to reserved target Instance/Lease")
	}

	admission, err := g.Admissions.InspectAcceptedOperationEffectV3(ctx, request.Operation, request.EffectID)
	if err != nil {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, err
	}
	if admission != request.Admission || admission.IntentRevision != intent.Revision || admission.IntentDigest != effect.IntentDigest {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, restoreStageConflictV1("Restore Stage Action Admission is not exact current")
	}
	authorization, err := g.Reviews.InspectCurrentOperationReviewAuthorizationV4(ctx, request.Operation, request.EffectID, request.Authorization.ID)
	if err != nil {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, err
	}
	if authorization.Validate() != nil || authorization.RefV4() != request.Authorization || authorization.State != ports.OperationReviewAuthorizationActiveV4 || now.UnixNano() >= authorization.ExpiresUnixNano {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, restoreStageConflictV1("Restore Stage Review Authorization is not exact active current")
	}

	dispatch, err := g.Dispatch.InspectCurrentOperationDispatchV4(ctx, ports.InspectCurrentOperationDispatchRequestV4{Inspect: ports.InspectOperationDispatchRecordRequestV4{Operation: request.Operation, EffectID: request.EffectID, PermitID: request.PermitID}, AdmissionDigest: request.ExecuteEnforcement.AdmissionDigest, ReviewAuthorization: request.Authorization})
	if err != nil {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, err
	}
	if err := dispatch.Validate(); err != nil || dispatch.Record.State != ports.OperationPermitBegunV4 || dispatch.Record.Permit.Admission.Admission != request.Admission || dispatch.Record.Permit.Admission.Authorization != request.Authorization || dispatch.Record.Permit.Digest != request.DispatchAttempt.PermitDigest || dispatch.Record.Revision != request.DispatchAttempt.PermitRevision || dispatch.Record.Permit.LegacyPermit.ID != request.PermitID || dispatch.Record.Permit.LegacyPermit.AttemptID != request.DispatchAttempt.AttemptID {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, restoreStageConflictV1("Restore Stage Permit/Begin current record drifted")
	}
	enforcement, err := g.Enforcement.InspectCurrentOperationProviderExecuteEnforcementV1(ctx, request.Operation, request.ExecuteEnforcement)
	if err != nil {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, err
	}
	if enforcement != request.ExecuteEnforcement || enforcement.Phase != ports.OperationDispatchEnforcementExecuteV4 || enforcement.OperationDigest != request.DispatchAttempt.OperationDigest || enforcement.EffectID != request.DispatchAttempt.EffectID || enforcement.PermitID != request.DispatchAttempt.PermitID || enforcement.PermitFactRevision != request.DispatchAttempt.PermitRevision || enforcement.PermitDigest != request.DispatchAttempt.PermitDigest || enforcement.AttemptID != request.DispatchAttempt.AttemptID || enforcement.ReviewAuthorization != request.Authorization || enforcement.AdmissionDigest != dispatch.Record.Permit.Admission.Digest {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, restoreStageConflictV1("Restore Stage execute Enforcement current ref drifted")
	}

	checked := enforcement.ValidatedUnixNano
	expires := minimumRestoreStageTimeV1(eligibility.Ref.ExpiresUnixNano, materialization.ExpiresUnixNano, effect.ExpiresUnixNano, authorization.ExpiresUnixNano, dispatch.Record.Permit.LegacyPermit.ExpiresUnixNano, enforcement.ExpiresUnixNano)
	projection, err := ports.SealRestoreStageGovernanceCurrentProjectionV1(ports.RestoreStageGovernanceCurrentProjectionV1{
		RestoreAttempt: request.RestoreAttempt, Eligibility: request.Eligibility, Identity: eligibility.Identity, Operation: request.Operation, EffectID: request.EffectID, EffectRevision: intent.Revision, IntentDigest: effect.IntentDigest,
		Admission: request.Admission, DispatchAdmissionDigest: dispatch.Record.Permit.Admission.Digest, Authorization: request.Authorization, PermitID: request.PermitID, PermitFactRevision: dispatch.Record.Revision, PermitDigest: dispatch.Record.Permit.Digest, BeginRecordRevision: dispatch.Record.Revision, BeginRecordDigest: dispatch.Record.Digest,
		DispatchAttempt: request.DispatchAttempt, ExecuteEnforcement: enforcement, MaterializationDigest: materialization.ProjectionDigest, SnapshotArtifact: request.SnapshotArtifact, CheckedUnixNano: checked, ExpiresUnixNano: expires,
	}, now)
	if err != nil {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, err
	}
	return projection, nil
}

func minimumRestoreStageTimeV1(values ...int64) int64 {
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

func restoreStageConflictV1(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, fmt.Sprintf("Restore Stage: %s", message))
}

var _ ports.RestoreStageGovernanceCurrentPortV1 = RestoreStageGovernanceGatewayV1{}

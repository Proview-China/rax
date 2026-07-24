package control

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// OperationGovernanceGatewayV4 is the only Application-facing Issue/Inspect/
// Begin path for V4 dispatch. Begin remains a host gate, not final Provider
// execution authorization.
type OperationGovernanceGatewayV4 struct {
	Effects    OperationEffectDispatchFactPortV4
	Admissions ports.OperationEffectAdmissionPortV3
	Reviews    ports.OperationReviewAuthorizationGovernancePortV4
	Current    ports.OperationGovernanceCurrentReaderV3
	Clock      func() time.Time
}

func (g OperationGovernanceGatewayV4) IssueOperationDispatchV4(ctx context.Context, request ports.IssueGovernedOperationDispatchRequestV4) (ports.CurrentOperationDispatchAuthorizationV4, error) {
	if err := g.validate(); err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	now := g.Clock()
	if now.IsZero() {
		return ports.CurrentOperationDispatchAuthorizationV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "V4 operation gateway clock returned zero")
	}
	effect, err := g.Effects.InspectOperationEffectV3(ctx, request.Operation, request.EffectID)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	if effect.State != OperationEffectAcceptedV3 || effect.Revision != request.ExpectedEffectRevision || !ports.SameOperationSubjectV3(effect.Intent.Operation, request.Operation) || !now.Before(time.Unix(0, effect.Intent.ExpiresUnixNano)) {
		return ports.CurrentOperationDispatchAuthorizationV4{}, core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "V4 Issue requires an exact accepted unexpired Effect")
	}
	admission, err := g.Admissions.InspectAcceptedOperationEffectV3(ctx, request.Operation, request.EffectID)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	if admission != request.Admission || admission.FactRevision != effect.Revision || admission.IntentDigest != effect.IntentDigest {
		return ports.CurrentOperationDispatchAuthorizationV4{}, core.NewError(core.ErrorConflict, core.ReasonEffectAuthorizationMissing, "V4 Issue admission does not bind the current accepted Effect")
	}
	authorization, err := g.Reviews.InspectCurrentOperationReviewAuthorizationV4(ctx, request.Operation, request.EffectID, request.ReviewAuthorization.ID)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	if authorization.RefV4() != request.ReviewAuthorization {
		return ports.CurrentOperationDispatchAuthorizationV4{}, core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "V4 Issue requested another Review Authorization revision or digest")
	}
	current, err := g.inspectCurrentGovernance(ctx, effect.Intent, authorization, now)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	credentialDigest, err := ports.DigestOperationCredentialFactsV3(current.Credentials, now)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	legacyReview, err := authorization.CompatibilityProjectionV3(now)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	if !ports.OperationReviewAuthorizationV3Covers(current.Review, legacyReview) {
		return ports.CurrentOperationDispatchAuthorizationV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "current V3 review projection does not cover the V4 Authorization")
	}
	legacyReview = current.Review
	legacyReviewDigest, err := ports.DigestOperationReviewAuthorizationV3(legacyReview)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	expires := minimumOperationDispatchExpiryV4(now.Add(request.PermitTTL), effect.Intent, current, authorization)
	if !expires.After(now) {
		return ports.CurrentOperationDispatchAuthorizationV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitExpired, "a V4 operation governance fact expires at Issue")
	}
	fence := operationDispatchFenceV4(effect.Intent, current, expires)
	fenceDigest, err := ports.DigestOperationExecutionFenceV3(fence, effect.Intent.Operation)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	authorizedAdmission, err := ports.SealOperationAuthorizedAdmissionV4(ports.OperationAuthorizedAdmissionV4{
		Admission: request.Admission, Authorization: authorization.RefV4(),
		PayloadSchema: effect.Intent.Payload.Schema, PayloadDigest: effect.Intent.Payload.ContentDigest, PayloadRevision: effect.Intent.PayloadRevision,
		ReviewProjectionDigest: authorization.Review.ProjectionDigest, ReviewCurrentnessDigest: authorization.Review.CurrentnessDigest,
		LegacyReviewProjectionDigest: legacyReviewDigest, GovernanceSnapshotDigest: authorization.Governance.SnapshotDigest,
		AuthorizationFenceDigest: authorization.FenceDigest, ExpiresUnixNano: authorization.ExpiresUnixNano,
	})
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	if err := authorizedAdmission.ValidateAgainstAuthorization(authorization, now); err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	intentDigest, err := effect.Intent.DigestV3()
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	snapshotDigest, err := current.DigestV3(now)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	legacyPermit := ports.OperationDispatchPermitV3{
		ContractVersion: ports.OperationEffectContractVersionV3, ID: request.PermitID, Revision: 1, AttemptID: request.AttemptID,
		IntentID: effect.Intent.ID, IntentRevision: effect.Intent.Revision, IntentDigest: intentDigest, Operation: effect.Intent.Operation,
		PayloadSchema: effect.Intent.Payload.Schema, PayloadDigest: effect.Intent.Payload.ContentDigest, PayloadRevision: effect.Intent.PayloadRevision,
		ConflictDomain: effect.Intent.ConflictDomain, Provider: current.Provider, EnforcementPoint: current.EnforcementPoint,
		Authority: effect.Intent.Authority, Review: effect.Intent.Review, ReviewAuthorization: legacyReview,
		Budget: effect.Intent.Budget, Policy: effect.Intent.Policy, CapabilityGrantDigest: current.CapabilityGrantDigest,
		CredentialGrantDigest: credentialDigest, GovernanceSnapshotDigest: snapshotDigest, FenceDigest: fenceDigest,
		Idempotency: effect.Intent.Idempotency, IssuedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	}
	permit, err := ports.SealOperationDispatchPermitV4(ports.OperationDispatchPermitV4{LegacyPermit: legacyPermit, Admission: authorizedAdmission})
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	if err := ports.ValidateOperationAtExecutionPointV3(legacyPermit, effect.Intent, fence, current, now); err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	if err := permit.ValidateAgainstAuthorization(authorization, fence, now); err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	result, err := g.Effects.IssueOperationDispatchPermitV4(ctx, IssueOperationPermitRequestV4{
		Operation: request.Operation, EffectID: request.EffectID, ExpectedEffectRevision: request.ExpectedEffectRevision,
		Permit: permit, Fence: fence, ReviewAuthorization: authorization,
	})
	if err != nil {
		if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorIndeterminate) && !core.HasCategory(err, core.ErrorConflict) {
			return ports.CurrentOperationDispatchAuthorizationV4{}, err
		}
		recoveryCtx := context.WithoutCancel(ctx)
		recovered, inspectErr := g.inspectHistorical(recoveryCtx, ports.InspectOperationDispatchRecordRequestV4{Operation: request.Operation, EffectID: request.EffectID, PermitID: request.PermitID})
		if inspectErr != nil || recovered.PermitDigest != permit.Digest || recovered.Permit.Admission.Digest != authorizedAdmission.Digest || recovered.Permit.Admission.Authorization != request.ReviewAuthorization {
			return ports.CurrentOperationDispatchAuthorizationV4{}, err
		}
		result.Permit = recovered
		result.Effect, inspectErr = g.Effects.InspectOperationEffectV3(recoveryCtx, request.Operation, request.EffectID)
		if inspectErr != nil {
			return ports.CurrentOperationDispatchAuthorizationV4{}, err
		}
	}
	if result.Permit.PermitDigest != permit.Digest || result.Permit.Permit.Admission.Digest != authorizedAdmission.Digest || result.Effect.State != OperationEffectDispatchIntentV3 || result.Effect.DispatchPermitID != request.PermitID || result.Effect.DispatchPermitDigest != permit.Digest {
		return ports.CurrentOperationDispatchAuthorizationV4{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "V4 Issue owner returned another Effect or Permit")
	}
	return g.currentEnvelope(ctx, result.Effect, result.Permit, now)
}

func (g OperationGovernanceGatewayV4) InspectOperationDispatchRecordV4(ctx context.Context, request ports.InspectOperationDispatchRecordRequestV4) (ports.OperationDispatchRecordV4, error) {
	if err := g.validate(); err != nil {
		return ports.OperationDispatchRecordV4{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.OperationDispatchRecordV4{}, err
	}
	return g.inspectHistorical(ctx, request)
}

func (g OperationGovernanceGatewayV4) InspectCurrentOperationDispatchV4(ctx context.Context, request ports.InspectCurrentOperationDispatchRequestV4) (ports.CurrentOperationDispatchAuthorizationV4, error) {
	if err := g.validate(); err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	record, err := g.inspectHistorical(ctx, request.Inspect)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	if record.Permit.Admission.Digest != request.AdmissionDigest || record.Permit.Admission.Authorization != request.ReviewAuthorization {
		return ports.CurrentOperationDispatchAuthorizationV4{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V4 current Inspect expected another admission or Authorization")
	}
	effect, err := g.Effects.InspectOperationEffectV3(ctx, request.Inspect.Operation, request.Inspect.EffectID)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	return g.currentEnvelope(ctx, effect, record, g.Clock())
}

func (g OperationGovernanceGatewayV4) BeginOperationDispatchV4(ctx context.Context, request ports.BeginGovernedOperationDispatchRequestV4) (ports.CurrentOperationDispatchAuthorizationV4, error) {
	if err := g.validate(); err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	now := g.Clock()
	if now.IsZero() {
		return ports.CurrentOperationDispatchAuthorizationV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "V4 operation gateway clock returned zero")
	}
	inspect := ports.InspectOperationDispatchRecordRequestV4{Operation: request.Operation, EffectID: request.EffectID, PermitID: request.PermitID}
	record, err := g.inspectHistorical(ctx, inspect)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	effect, err := g.Effects.InspectOperationEffectV3(ctx, request.Operation, request.EffectID)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	if effect.State != OperationEffectDispatchIntentV3 || effect.Revision != request.ExpectedEffectRevision || record.State != ports.OperationPermitIssuedV4 || record.Revision != request.ExpectedPermitFactRevision || record.EffectFactRevision != effect.Revision || record.Permit.Admission.Digest != request.AdmissionDigest || record.Permit.Admission.Authorization != request.ReviewAuthorization {
		return ports.CurrentOperationDispatchAuthorizationV4{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V4 Begin requires exact issued Permit, admission and Authorization watermarks")
	}
	if _, err := g.currentEnvelope(ctx, effect, record, now); err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	stored, err := g.Effects.BeginOperationDispatchV4(ctx, BeginOperationDispatchRequestV4{
		Operation: request.Operation, EffectID: request.EffectID, ExpectedEffectRevision: request.ExpectedEffectRevision,
		PermitID: request.PermitID, ExpectedPermitFactRevision: request.ExpectedPermitFactRevision,
		AdmissionDigest: request.AdmissionDigest, ReviewAuthorization: request.ReviewAuthorization,
	})
	if err != nil {
		if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorIndeterminate) && !core.HasCategory(err, core.ErrorConflict) {
			return ports.CurrentOperationDispatchAuthorizationV4{}, err
		}
		recovered, inspectErr := g.inspectHistorical(context.WithoutCancel(ctx), inspect)
		if inspectErr != nil || recovered.State != ports.OperationPermitBegunV4 || recovered.Revision != request.ExpectedPermitFactRevision+1 || recovered.PermitDigest != record.PermitDigest || recovered.Permit.Admission.Digest != request.AdmissionDigest || recovered.Permit.Admission.Authorization != request.ReviewAuthorization {
			return ports.CurrentOperationDispatchAuthorizationV4{}, err
		}
		stored = recovered
	}
	if stored.State != ports.OperationPermitBegunV4 || stored.PermitDigest != record.PermitDigest || stored.Permit.Admission.Authorization != request.ReviewAuthorization {
		return ports.CurrentOperationDispatchAuthorizationV4{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "V4 Begin owner returned another Permit")
	}
	return g.currentEnvelope(ctx, effect, stored, now)
}

func (g OperationGovernanceGatewayV4) inspectHistorical(ctx context.Context, request ports.InspectOperationDispatchRecordRequestV4) (ports.OperationDispatchRecordV4, error) {
	record, err := g.Effects.InspectOperationDispatchPermitV4(ctx, request.Operation, request.PermitID)
	if err != nil {
		return ports.OperationDispatchRecordV4{}, err
	}
	if err := record.Validate(); err != nil {
		return ports.OperationDispatchRecordV4{}, err
	}
	effect, err := g.Effects.InspectOperationEffectV3(ctx, request.Operation, request.EffectID)
	if err != nil {
		return ports.OperationDispatchRecordV4{}, err
	}
	legacy := record.Permit.LegacyPermit
	if effect.Intent.ID != request.EffectID || legacy.IntentID != request.EffectID || !ports.SameOperationSubjectV3(effect.Intent.Operation, request.Operation) || effect.IntentDigest != legacy.IntentDigest || effect.DispatchPermitID != request.PermitID || effect.DispatchPermitDigest != record.PermitDigest || record.EffectFactRevision > effect.Revision {
		return ports.OperationDispatchRecordV4{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V4 historical record does not bind the exact Effect")
	}
	return record, nil
}

func (g OperationGovernanceGatewayV4) currentEnvelope(ctx context.Context, effect OperationEffectFactV3, record ports.OperationDispatchRecordV4, now time.Time) (ports.CurrentOperationDispatchAuthorizationV4, error) {
	if now.IsZero() || !now.Before(time.Unix(0, record.Permit.LegacyPermit.ExpiresUnixNano)) {
		return ports.CurrentOperationDispatchAuthorizationV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitExpired, "V4 Permit is expired")
	}
	authorizationRef := record.Permit.Admission.Authorization
	authorization, err := g.Reviews.InspectCurrentOperationReviewAuthorizationV4(ctx, effect.Intent.Operation, effect.Intent.ID, authorizationRef.ID)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	if authorization.RefV4() != authorizationRef {
		return ports.CurrentOperationDispatchAuthorizationV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "persisted V4 Permit binds a stale Authorization")
	}
	current, err := g.inspectCurrentGovernance(ctx, effect.Intent, authorization, now)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	if err := record.Permit.ValidateAgainstAuthorization(authorization, record.Fence, now); err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	if err := ports.ValidateOperationAtExecutionPointV3(record.Permit.LegacyPermit, effect.Intent, record.Fence, current, now); err != nil {
		return ports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	envelope := ports.CurrentOperationDispatchAuthorizationV4{
		Record: record, ReviewAuthorization: authorization.RefV4(),
		ReviewProjectionDigest: authorization.Review.ProjectionDigest, ReviewCurrentnessDigest: authorization.Review.CurrentnessDigest,
		GovernanceSnapshotDigest: authorization.Governance.SnapshotDigest, CheckedUnixNano: now.UnixNano(),
	}
	return envelope, envelope.Validate()
}

func (g OperationGovernanceGatewayV4) inspectCurrentGovernance(ctx context.Context, intent ports.OperationEffectIntentV3, authorization ports.OperationReviewAuthorizationFactV4, now time.Time) (ports.OperationGovernanceSnapshotV3, error) {
	current, err := g.Current.InspectOperationGovernance(ctx, intent.Operation)
	if err != nil {
		return ports.OperationGovernanceSnapshotV3{}, err
	}
	if err := current.ValidateCurrent(intent, now); err != nil {
		return ports.OperationGovernanceSnapshotV3{}, err
	}
	snapshotDigest, err := current.DigestV3(now)
	if err != nil {
		return ports.OperationGovernanceSnapshotV3{}, err
	}
	credentialDigest, err := ports.DigestOperationCredentialFactsV3(current.Credentials, now)
	if err != nil {
		return ports.OperationGovernanceSnapshotV3{}, err
	}
	legacyReview, err := authorization.CompatibilityProjectionV3(now)
	if err != nil {
		return ports.OperationGovernanceSnapshotV3{}, err
	}
	if snapshotDigest != authorization.Governance.SnapshotDigest || current.ProjectionWatermark != authorization.Governance.ProjectionWatermark || current.CapabilityGrantDigest != authorization.Governance.CapabilityGrantDigest || credentialDigest != authorization.Governance.CredentialGrantDigest || !ports.OperationReviewAuthorizationV3Covers(current.Review, legacyReview) {
		return ports.OperationGovernanceSnapshotV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "current V3 governance projection drifted from V4 Authorization")
	}
	return current, nil
}

func minimumOperationDispatchExpiryV4(initial time.Time, intent ports.OperationEffectIntentV3, current ports.OperationGovernanceSnapshotV3, authorization ports.OperationReviewAuthorizationFactV4) time.Time {
	expires := initial
	limits := []int64{
		intent.ExpiresUnixNano, authorization.ExpiresUnixNano, current.ExpiresUnixNano,
		current.Identity.ExpiresUnixNano, current.Binding.ExpiresUnixNano, current.CurrentScope.ExpiresUnixNano,
		current.Authority.ExpiresUnixNano, current.Review.ExpiresUnixNano, current.Review.Case.ExpiresUnixNano,
		current.Review.Verdict.ExpiresUnixNano, current.Review.ReviewerAuthority.ExpiresUnixNano,
		current.Budget.ExpiresUnixNano, current.Policy.ExpiresUnixNano,
	}
	if current.Review.Satisfaction != nil {
		limits = append(limits, current.Review.Satisfaction.ExpiresUnixNano)
	}
	for _, credential := range current.Credentials {
		limits = append(limits, credential.ExpiresUnixNano)
	}
	for _, limit := range limits {
		value := time.Unix(0, limit)
		if value.Before(expires) {
			expires = value
		}
	}
	return expires
}

func operationDispatchFenceV4(intent ports.OperationEffectIntentV3, current ports.OperationGovernanceSnapshotV3, expires time.Time) core.ExecutionFence {
	boundary := core.FenceBoundaryActivation
	if current.Operation.ExecutionScope.SandboxLease != nil {
		boundary = core.FenceBoundaryInstance
	}
	return core.ExecutionFence{
		BoundaryScope: boundary, Scope: current.Operation.ExecutionScope, CapabilityGrantDigest: current.CapabilityGrantDigest,
		EffectIntentID: intent.ID, EffectIntentRevision: intent.Revision, CanonicalPayloadDigest: intent.Payload.ContentDigest, ExpiresAt: expires,
	}
}

func (g OperationGovernanceGatewayV4) validate() error {
	if g.Effects == nil || g.Admissions == nil || g.Reviews == nil || g.Current == nil || g.Clock == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "V4 operation gateway requires Fact Owner, admission, Review Authorization, current governance and clock")
	}
	return nil
}

var _ ports.OperationGovernancePortV4 = OperationGovernanceGatewayV4{}

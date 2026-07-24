package control

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// OperationGovernanceGatewayV5 is the nominal quorum/not-required dispatch
// route. It never calls a Provider and never constructs Review-owned facts.
type OperationGovernanceGatewayV5 struct {
	Effects    OperationEffectDispatchFactPortV5
	Admissions ports.OperationEffectAdmissionPortV3
	Reviews    ports.OperationReviewAuthorizationGovernancePortV5
	Current    ports.OperationGovernanceCurrentReaderV3
	Clock      func() time.Time
}

func (g OperationGovernanceGatewayV5) IssueOperationDispatchV5(ctx context.Context, request ports.IssueGovernedOperationDispatchRequestV5) (ports.CurrentOperationDispatchAuthorizationV5, error) {
	if err := g.validate(); err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, err
	}
	baseline, err := g.baseline()
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, err
	}
	inspect := ports.InspectOperationDispatchRecordRequestV5{Operation: request.Operation, EffectID: request.EffectID, PermitID: request.PermitID}
	if existing, inspectErr := g.inspectHistorical(ctx, inspect); inspectErr == nil {
		if existing.Permit.Authorization != request.ReviewAuthorization || existing.Permit.AuthorizationBasis != request.AuthorizationBasis || existing.Permit.AttemptID != request.AttemptID || existing.Permit.Admission.Admission != request.Admission || existing.Permit.ExpiresUnixNano-existing.Permit.IssuedUnixNano > request.PermitTTL.Nanoseconds() {
			return ports.CurrentOperationDispatchAuthorizationV5{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "V5 Permit ID already binds another Issue request")
		}
		effect, e := g.inspectEffect(ctx, request.Operation, request.EffectID)
		if e != nil {
			return ports.CurrentOperationDispatchAuthorizationV5{}, e
		}
		return g.currentEnvelope(ctx, effect, existing, baseline)
	} else if !core.HasCategory(inspectErr, core.ErrorNotFound) {
		return ports.CurrentOperationDispatchAuthorizationV5{}, inspectErr
	}
	effect, err := g.inspectEffect(ctx, request.Operation, request.EffectID)
	if err != nil {
		if recovered, ok := g.recoverIssueV5(ctx, inspect, request, baseline); ok {
			return recovered, nil
		}
		return ports.CurrentOperationDispatchAuthorizationV5{}, err
	}
	if effect.State != OperationEffectAcceptedV3 || effect.Revision != request.ExpectedEffectRevision || !ports.SameOperationSubjectV3(effect.Intent.Operation, request.Operation) {
		if recovered, ok := g.recoverIssueV5(ctx, inspect, request, baseline); ok {
			return recovered, nil
		}
		return ports.CurrentOperationDispatchAuthorizationV5{}, core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "V5 Issue requires exact accepted Effect")
	}
	admission, err := g.inspectAdmission(ctx, request.Operation, request.EffectID)
	if err != nil {
		if recovered, ok := g.recoverIssueV5(ctx, inspect, request, baseline); ok {
			return recovered, nil
		}
		return ports.CurrentOperationDispatchAuthorizationV5{}, err
	}
	if admission != request.Admission || admission.FactRevision != effect.Revision || admission.IntentDigest != effect.IntentDigest {
		if recovered, ok := g.recoverIssueV5(ctx, inspect, request, baseline); ok {
			return recovered, nil
		}
		return ports.CurrentOperationDispatchAuthorizationV5{}, core.NewError(core.ErrorConflict, core.ReasonEffectAuthorizationMissing, "V5 Issue admission drifted")
	}
	authorization, current, now, err := g.inspectActualPoint(ctx, effect.Intent, request.ReviewAuthorization, request.AuthorizationBasis, baseline)
	if err != nil {
		if recovered, ok := g.recoverIssueV5(ctx, inspect, request, baseline); ok {
			return recovered, nil
		}
		return ports.CurrentOperationDispatchAuthorizationV5{}, err
	}
	credentialDigest, err := ports.DigestOperationCredentialFactsV3(current.Credentials, now)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, err
	}
	currentness, err := ports.OperationReviewCurrentnessDigestV5(authorization.Review)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, err
	}
	expires := minimumDispatchTimeV5(now.Add(request.PermitTTL), time.Unix(0, effect.Intent.ExpiresUnixNano), time.Unix(0, authorization.ExpiresUnixNano), time.Unix(0, current.ExpiresUnixNano))
	if !expires.After(now) {
		return ports.CurrentOperationDispatchAuthorizationV5{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitExpired, "V5 Issue has no current TTL")
	}
	fence := operationDispatchFenceV5(effect.Intent, current, expires)
	fenceDigest, err := ports.DigestOperationExecutionFenceV3(fence, effect.Intent.Operation)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, err
	}
	authorizedAdmission, err := ports.SealOperationAuthorizedAdmissionV5(ports.OperationAuthorizedAdmissionV5{Admission: request.Admission, Authorization: authorization.RefV5(), AuthorizationBasis: authorization.Review.Basis, PayloadSchema: effect.Intent.Payload.Schema, PayloadDigest: effect.Intent.Payload.ContentDigest, PayloadRevision: effect.Intent.PayloadRevision, ReviewProjectionDigest: authorization.Review.ProjectionDigest, ReviewCurrentnessDigest: currentness, GovernanceSnapshotDigest: authorization.Governance.SnapshotDigest, AuthorizationFenceDigest: authorization.FenceDigest, ExpiresUnixNano: authorization.ExpiresUnixNano})
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, err
	}
	intentDigest, err := effect.Intent.DigestV3()
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, err
	}
	permit, err := ports.SealOperationDispatchPermitV5(ports.OperationDispatchPermitV5{ID: request.PermitID, Revision: 1, AttemptID: request.AttemptID, IntentID: effect.Intent.ID, IntentRevision: effect.Intent.Revision, IntentDigest: intentDigest, Operation: effect.Intent.Operation, PayloadSchema: effect.Intent.Payload.Schema, PayloadDigest: effect.Intent.Payload.ContentDigest, PayloadRevision: effect.Intent.PayloadRevision, ConflictDomain: effect.Intent.ConflictDomain, Provider: current.Provider, EnforcementPoint: current.EnforcementPoint, Authority: effect.Intent.Authority, Review: effect.Intent.Review, Budget: effect.Intent.Budget, Policy: effect.Intent.Policy, Authorization: authorization.RefV5(), AuthorizationBasis: authorization.Review.Basis, CapabilityGrantDigest: current.CapabilityGrantDigest, CredentialGrantDigest: credentialDigest, GovernanceSnapshotDigest: authorization.Governance.SnapshotDigest, FenceDigest: fenceDigest, Idempotency: effect.Intent.Idempotency, IssuedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(), Admission: authorizedAdmission})
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, err
	}
	if err := permit.ValidateAgainstAuthorization(authorization, fence, now); err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, err
	}
	result, err := g.Effects.IssueOperationDispatchPermitV5(ctx, IssueOperationPermitRequestV5{Operation: request.Operation, EffectID: request.EffectID, ExpectedEffectRevision: request.ExpectedEffectRevision, Permit: permit, Fence: fence, ReviewAuthorization: authorization})
	if err != nil {
		if !unknownOrConflictDispatchV5(err) {
			return ports.CurrentOperationDispatchAuthorizationV5{}, err
		}
		recovery := context.WithoutCancel(ctx)
		stored, e := g.inspectHistorical(recovery, inspect)
		if e != nil || stored.PermitDigest != permit.Digest || stored.Permit.Admission.Digest != authorizedAdmission.Digest || stored.Permit.Authorization != request.ReviewAuthorization {
			return ports.CurrentOperationDispatchAuthorizationV5{}, err
		}
		result.Permit = stored
		result.Effect, e = g.inspectEffect(recovery, request.Operation, request.EffectID)
		if e != nil {
			return ports.CurrentOperationDispatchAuthorizationV5{}, err
		}
	}
	if result.Permit.PermitDigest != permit.Digest || result.Effect.State != OperationEffectDispatchIntentV3 || result.Effect.DispatchPermitID != request.PermitID || result.Effect.DispatchPermitDigest != permit.Digest {
		return ports.CurrentOperationDispatchAuthorizationV5{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "V5 Issue owner returned another Effect or Permit")
	}
	return g.currentEnvelope(context.WithoutCancel(ctx), result.Effect, result.Permit, now)
}

func (g OperationGovernanceGatewayV5) InspectOperationDispatchRecordV5(ctx context.Context, request ports.InspectOperationDispatchRecordRequestV5) (ports.OperationDispatchRecordV5, error) {
	if err := g.validate(); err != nil {
		return ports.OperationDispatchRecordV5{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.OperationDispatchRecordV5{}, err
	}
	return g.inspectHistorical(ctx, request)
}
func (g OperationGovernanceGatewayV5) InspectCurrentOperationDispatchV5(ctx context.Context, request ports.InspectCurrentOperationDispatchRequestV5) (ports.CurrentOperationDispatchAuthorizationV5, error) {
	if err := g.validate(); err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, err
	}
	baseline, err := g.baseline()
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, err
	}
	record, err := g.inspectHistorical(ctx, request.Inspect)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, err
	}
	if record.Permit.Admission.Digest != request.AdmissionDigest || record.Permit.Authorization != request.ReviewAuthorization || record.Permit.AuthorizationBasis != request.AuthorizationBasis {
		return ports.CurrentOperationDispatchAuthorizationV5{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V5 current Inspect expected another immutable binding")
	}
	effect, err := g.inspectEffect(ctx, request.Inspect.Operation, request.Inspect.EffectID)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, err
	}
	return g.currentEnvelope(ctx, effect, record, baseline)
}

func (g OperationGovernanceGatewayV5) BeginOperationDispatchV5(ctx context.Context, request ports.BeginGovernedOperationDispatchRequestV5) (ports.CurrentOperationDispatchAuthorizationV5, error) {
	if err := g.validate(); err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, err
	}
	baseline, err := g.baseline()
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, err
	}
	inspect := ports.InspectOperationDispatchRecordRequestV5{Operation: request.Operation, EffectID: request.EffectID, PermitID: request.PermitID}
	record, err := g.inspectHistorical(ctx, inspect)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, err
	}
	effect, err := g.inspectEffect(ctx, request.Operation, request.EffectID)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, err
	}
	if effect.State != OperationEffectDispatchIntentV3 || effect.Revision != request.ExpectedEffectRevision || record.State != ports.OperationPermitIssuedV5 || record.Revision != request.ExpectedPermitFactRevision || record.EffectFactRevision != effect.Revision || record.Permit.Admission.Digest != request.AdmissionDigest || record.Permit.Authorization != request.ReviewAuthorization || record.Permit.AuthorizationBasis != request.AuthorizationBasis {
		return ports.CurrentOperationDispatchAuthorizationV5{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V5 Begin requires exact issued Permit")
	}
	if _, err := g.currentEnvelope(ctx, effect, record, baseline); err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, err
	}
	stored, err := g.Effects.BeginOperationDispatchV5(ctx, BeginOperationDispatchRequestV5{Operation: request.Operation, EffectID: request.EffectID, ExpectedEffectRevision: request.ExpectedEffectRevision, PermitID: request.PermitID, ExpectedPermitFactRevision: request.ExpectedPermitFactRevision, AdmissionDigest: request.AdmissionDigest, ReviewAuthorization: request.ReviewAuthorization, AuthorizationBasis: request.AuthorizationBasis})
	if err != nil {
		if !unknownOrConflictDispatchV5(err) {
			return ports.CurrentOperationDispatchAuthorizationV5{}, err
		}
		recovered, recoverErr := g.inspectHistorical(context.WithoutCancel(ctx), inspect)
		if recoverErr != nil || recovered.State != ports.OperationPermitBegunV5 || recovered.Revision != request.ExpectedPermitFactRevision+1 || recovered.PermitDigest != record.PermitDigest || recovered.Permit.Authorization != request.ReviewAuthorization {
			return ports.CurrentOperationDispatchAuthorizationV5{}, err
		}
		stored = recovered
	}
	return g.currentEnvelope(context.WithoutCancel(ctx), effect, stored, baseline)
}

func sameOperationDispatchIssueV5(record ports.OperationDispatchRecordV5, request ports.IssueGovernedOperationDispatchRequestV5) bool {
	return record.Permit.ID == request.PermitID &&
		record.Permit.AttemptID == request.AttemptID &&
		record.Permit.IntentID == request.EffectID &&
		record.Permit.Admission.Admission == request.Admission &&
		record.Permit.Authorization == request.ReviewAuthorization &&
		record.Permit.AuthorizationBasis == request.AuthorizationBasis &&
		record.Permit.ExpiresUnixNano-record.Permit.IssuedUnixNano <= request.PermitTTL.Nanoseconds()
}

func (g OperationGovernanceGatewayV5) recoverIssueV5(ctx context.Context, inspect ports.InspectOperationDispatchRecordRequestV5, request ports.IssueGovernedOperationDispatchRequestV5, baseline time.Time) (ports.CurrentOperationDispatchAuthorizationV5, bool) {
	recovery := context.WithoutCancel(ctx)
	record, err := g.inspectHistorical(recovery, inspect)
	if err != nil || !sameOperationDispatchIssueV5(record, request) {
		return ports.CurrentOperationDispatchAuthorizationV5{}, false
	}
	effect, err := g.inspectEffect(recovery, request.Operation, request.EffectID)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, false
	}
	current, err := g.currentEnvelope(recovery, effect, record, baseline)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, false
	}
	return current, true
}

func (g OperationGovernanceGatewayV5) currentEnvelope(ctx context.Context, effect OperationEffectFactV3, record ports.OperationDispatchRecordV5, previous time.Time) (ports.CurrentOperationDispatchAuthorizationV5, error) {
	authorization, current, now, err := g.inspectActualPoint(ctx, effect.Intent, record.Permit.Authorization, record.Permit.AuthorizationBasis, previous)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, err
	}
	if err := record.Permit.ValidateAgainstAuthorization(authorization, record.Fence, now); err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, err
	}
	if err := validateOperationAtExecutionPointV5(record.Permit, effect.Intent, record.Fence, current, authorization, now); err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, err
	}
	currentness, err := ports.OperationReviewCurrentnessDigestV5(authorization.Review)
	if err != nil {
		return ports.CurrentOperationDispatchAuthorizationV5{}, err
	}
	expires := minimumDispatchTimeV5(time.Unix(0, record.Permit.ExpiresUnixNano), time.Unix(0, authorization.ExpiresUnixNano), time.Unix(0, current.ExpiresUnixNano))
	return ports.SealCurrentOperationDispatchAuthorizationV5(ports.CurrentOperationDispatchAuthorizationV5{Record: record, ReviewAuthorization: authorization.RefV5(), AuthorizationBasis: authorization.Review.Basis, ReviewProjectionDigest: authorization.Review.ProjectionDigest, ReviewCurrentnessDigest: currentness, GovernanceSnapshotDigest: authorization.Governance.SnapshotDigest, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano()})
}

func (g OperationGovernanceGatewayV5) inspectActualPoint(ctx context.Context, intent ports.OperationEffectIntentV3, expected ports.OperationReviewAuthorizationRefV5, basis ports.OperationReviewAuthorizationBasisV5, previous time.Time) (ports.OperationReviewAuthorizationFactV5, ports.OperationGovernanceSnapshotV3, time.Time, error) {
	first, err := g.inspectReview(ctx, intent.Operation, intent.ID, expected.ID)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, ports.OperationGovernanceSnapshotV3{}, time.Time{}, err
	}
	current, err := g.inspectGovernance(ctx, intent.Operation)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, ports.OperationGovernanceSnapshotV3{}, time.Time{}, err
	}
	second, err := g.inspectReview(ctx, intent.Operation, intent.ID, expected.ID)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, ports.OperationGovernanceSnapshotV3{}, time.Time{}, err
	}
	now := g.Clock()
	if now.IsZero() || now.Before(previous) {
		return ports.OperationReviewAuthorizationFactV5{}, ports.OperationGovernanceSnapshotV3{}, time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "V5 dispatch clock regressed across current reads")
	}
	if first.RefV5() != expected || second.RefV5() != expected || first.Digest != second.Digest || first.State != ports.OperationReviewAuthorizationActiveV5 || second.Review.Basis != basis {
		return ports.OperationReviewAuthorizationFactV5{}, ports.OperationGovernanceSnapshotV3{}, time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "V5 Review Authorization changed across actual-point S1/S2")
	}
	if err := validateGovernanceCurrentV5(current, intent, second, now); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, ports.OperationGovernanceSnapshotV3{}, time.Time{}, err
	}
	return second, current, now, nil
}

func validateGovernanceCurrentV5(s ports.OperationGovernanceSnapshotV3, intent ports.OperationEffectIntentV3, authorization ports.OperationReviewAuthorizationFactV5, now time.Time) error {
	if !s.Active || s.ProjectionWatermark == 0 || !ports.SameOperationSubjectV3(s.Operation, intent.Operation) || now.IsZero() || !now.Before(time.Unix(0, s.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "V5 governance is inactive or expired")
	}
	for _, ref := range []ports.OperationGovernanceFactRefV3{s.Identity, s.Binding, s.CurrentScope, s.Authority, s.Budget, s.Policy} {
		if err := ref.Validate(now); err != nil {
			return err
		}
	}
	if s.CurrentScope.Ref != intent.Operation.CurrentProjectionRef || s.CurrentScope.Revision != intent.Operation.CurrentProjectionRevision || s.CurrentScope.Digest != intent.Operation.CurrentProjectionDigest || s.Provider != intent.Provider || s.EnforcementPoint != intent.Provider || s.Binding.Ref != intent.Provider.BindingSetID || s.Binding.Revision != intent.Provider.BindingSetRevision || s.Authority.Ref != intent.Authority.Ref || s.Authority.Revision != intent.Authority.Revision || s.Authority.Digest != intent.Authority.Digest || s.Budget.Ref != intent.Budget.Ref || s.Budget.Revision != intent.Budget.Revision || s.Budget.Digest != intent.Budget.Digest || s.Policy.Ref != intent.Policy.Ref || s.Policy.Revision != intent.Policy.Revision || s.Policy.Digest != intent.Policy.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectAuthorizationMissing, "V5 governance drifted from Intent")
	}
	credentialDigest, err := ports.DigestOperationCredentialFactsV3(s.Credentials, now)
	if err != nil {
		return err
	}
	snapshotDigest, err := ports.DigestOperationGovernanceForReviewAuthorizationV5(s)
	if err != nil {
		return err
	}
	if snapshotDigest != authorization.Governance.SnapshotDigest || s.ProjectionWatermark != authorization.Governance.ProjectionWatermark || s.Identity != authorization.Governance.Identity || s.Binding != authorization.Governance.Binding || s.CurrentScope != authorization.Governance.CurrentScope || s.Authority != authorization.Governance.Authority || s.Budget != authorization.Governance.Budget || s.Policy != authorization.Governance.Policy || s.CapabilityGrantDigest != authorization.Governance.CapabilityGrantDigest || credentialDigest != authorization.Governance.CredentialGrantDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "V5 governance drifted from Authorization")
	}
	return nil
}

func validateOperationAtExecutionPointV5(p ports.OperationDispatchPermitV5, intent ports.OperationEffectIntentV3, fence core.ExecutionFence, current ports.OperationGovernanceSnapshotV3, authorization ports.OperationReviewAuthorizationFactV5, now time.Time) error {
	if err := p.ValidateAgainstAuthorization(authorization, fence, now); err != nil {
		return err
	}
	intentDigest, err := intent.DigestV3()
	if err != nil {
		return err
	}
	credentialDigest, err := ports.DigestOperationCredentialFactsV3(current.Credentials, now)
	if err != nil {
		return err
	}
	snapshotDigest, err := ports.DigestOperationGovernanceForReviewAuthorizationV5(current)
	if err != nil {
		return err
	}
	fenceDigest, err := ports.DigestOperationExecutionFenceV3(fence, intent.Operation)
	if err != nil || p.IntentID != intent.ID || p.IntentRevision != intent.Revision || p.IntentDigest != intentDigest || !ports.SameOperationSubjectV3(p.Operation, intent.Operation) || p.PayloadSchema != intent.Payload.Schema || p.PayloadDigest != intent.Payload.ContentDigest || p.PayloadRevision != intent.PayloadRevision || p.Provider != intent.Provider || p.EnforcementPoint != current.EnforcementPoint || p.Authority != intent.Authority || p.Review != intent.Review || p.Budget != intent.Budget || p.Policy != intent.Policy || p.CapabilityGrantDigest != current.CapabilityGrantDigest || p.CredentialGrantDigest != credentialDigest || p.GovernanceSnapshotDigest != snapshotDigest || p.FenceDigest != fenceDigest || !now.Before(time.Unix(0, p.ExpiresUnixNano)) || !now.Before(time.Unix(0, intent.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "V5 Permit or governance drifted before provider contact")
	}
	return core.CheckFence(fence, core.CurrentFenceFacts{Scope: current.Operation.ExecutionScope, CapabilityGrantDigest: current.CapabilityGrantDigest}, now)
}

func (g OperationGovernanceGatewayV5) inspectHistorical(ctx context.Context, r ports.InspectOperationDispatchRecordRequestV5) (ports.OperationDispatchRecordV5, error) {
	record, err := g.Effects.InspectOperationDispatchPermitV5(ctx, r.Operation, r.PermitID)
	if unknownDispatchV5(err) {
		record, err = g.Effects.InspectOperationDispatchPermitV5(context.WithoutCancel(ctx), r.Operation, r.PermitID)
	}
	if err != nil {
		return ports.OperationDispatchRecordV5{}, err
	}
	if err := record.Validate(); err != nil {
		return ports.OperationDispatchRecordV5{}, err
	}
	effect, err := g.inspectEffect(ctx, r.Operation, r.EffectID)
	if err != nil {
		return ports.OperationDispatchRecordV5{}, err
	}
	if effect.Intent.ID != r.EffectID || record.Permit.IntentID != r.EffectID || !ports.SameOperationSubjectV3(effect.Intent.Operation, r.Operation) || effect.IntentDigest != record.Permit.IntentDigest || effect.DispatchPermitID != r.PermitID || effect.DispatchPermitDigest != record.PermitDigest || record.EffectFactRevision > effect.Revision {
		return ports.OperationDispatchRecordV5{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V5 historical record does not bind exact Effect")
	}
	return record, nil
}
func (g OperationGovernanceGatewayV5) inspectEffect(ctx context.Context, o ports.OperationSubjectV3, id core.EffectIntentID) (OperationEffectFactV3, error) {
	v, e := g.Effects.InspectOperationEffectV3(ctx, o, id)
	if unknownDispatchV5(e) {
		return g.Effects.InspectOperationEffectV3(context.WithoutCancel(ctx), o, id)
	}
	return v, e
}
func (g OperationGovernanceGatewayV5) inspectAdmission(ctx context.Context, o ports.OperationSubjectV3, id core.EffectIntentID) (ports.OperationEffectAdmissionReceiptV3, error) {
	v, e := g.Admissions.InspectAcceptedOperationEffectV3(ctx, o, id)
	if unknownDispatchV5(e) {
		return g.Admissions.InspectAcceptedOperationEffectV3(context.WithoutCancel(ctx), o, id)
	}
	return v, e
}
func (g OperationGovernanceGatewayV5) inspectReview(ctx context.Context, o ports.OperationSubjectV3, id core.EffectIntentID, a string) (ports.OperationReviewAuthorizationFactV5, error) {
	v, e := g.Reviews.InspectCurrentOperationReviewAuthorizationV5(ctx, o, id, a)
	if unknownDispatchV5(e) {
		return g.Reviews.InspectCurrentOperationReviewAuthorizationV5(context.WithoutCancel(ctx), o, id, a)
	}
	return v, e
}
func (g OperationGovernanceGatewayV5) inspectGovernance(ctx context.Context, o ports.OperationSubjectV3) (ports.OperationGovernanceSnapshotV3, error) {
	v, e := g.Current.InspectOperationGovernance(ctx, o)
	if unknownDispatchV5(e) {
		return g.Current.InspectOperationGovernance(context.WithoutCancel(ctx), o)
	}
	return v, e
}
func (g OperationGovernanceGatewayV5) baseline() (time.Time, error) {
	now := g.Clock()
	if now.IsZero() {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "V5 dispatch baseline clock is zero")
	}
	return now, nil
}
func (g OperationGovernanceGatewayV5) validate() error {
	if nilDispatchV5(g.Effects) || nilDispatchV5(g.Admissions) || nilDispatchV5(g.Reviews) || nilDispatchV5(g.Current) || g.Clock == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "V5 operation gateway dependencies are required")
	}
	return nil
}
func minimumDispatchTimeV5(values ...time.Time) time.Time {
	m := values[0]
	for _, v := range values[1:] {
		if v.Before(m) {
			m = v
		}
	}
	return m
}
func operationDispatchFenceV5(intent ports.OperationEffectIntentV3, current ports.OperationGovernanceSnapshotV3, expires time.Time) core.ExecutionFence {
	boundary := core.FenceBoundaryActivation
	if current.Operation.ExecutionScope.SandboxLease != nil {
		boundary = core.FenceBoundaryInstance
	}
	return core.ExecutionFence{BoundaryScope: boundary, Scope: current.Operation.ExecutionScope, CapabilityGrantDigest: current.CapabilityGrantDigest, EffectIntentID: intent.ID, EffectIntentRevision: intent.Revision, CanonicalPayloadDigest: intent.Payload.ContentDigest, ExpiresAt: expires}
}
func unknownDispatchV5(err error) bool {
	return core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate)
}
func unknownOrConflictDispatchV5(err error) bool {
	return unknownDispatchV5(err) || core.HasCategory(err, core.ErrorConflict)
}
func nilDispatchV5(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return r.IsNil()
	}
	return false
}

var _ ports.OperationGovernancePortV5 = OperationGovernanceGatewayV5{}

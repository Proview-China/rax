package kernel

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// OperationReviewAuthorizationGatewayV5 owns only the Runtime Authorization
// fact. It neither counts Review votes nor creates Review/Policy facts.
type OperationReviewAuthorizationGatewayV5 struct {
	Facts      ports.OperationReviewAuthorizationFactPortV5
	Effects    control.OperationEffectFactPortV3
	Governance ports.OperationGovernanceCurrentReaderV3
	Reviews    ports.OperationReviewCurrentReaderV5
	Clock      func() time.Time
}

func (g OperationReviewAuthorizationGatewayV5) validate() error {
	if nilInterfaceV5(g.Facts) || nilInterfaceV5(g.Effects) || nilInterfaceV5(g.Governance) || nilInterfaceV5(g.Reviews) || g.Clock == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review Authorization V5 owners, readers and clock are required")
	}
	return nil
}

func (g OperationReviewAuthorizationGatewayV5) CreateOperationReviewAuthorizationV5(ctx context.Context, request ports.CreateOperationReviewAuthorizationRequestV5) (ports.OperationReviewAuthorizationFactV5, error) {
	if err := g.validate(); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	baseline := g.Clock()
	if baseline.IsZero() {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review Authorization V5 baseline clock is zero")
	}
	existing, err := g.inspectFactV5(ctx, request.AuthorizationID)
	if err == nil {
		return exactOperationReviewAuthorizationRequestV5(existing, request)
	}
	if !core.HasCategory(err, core.ErrorNotFound) {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	effect, err := g.inspectEffectV5(ctx, request.Operation, request.EffectID)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if effect.State != control.OperationEffectAcceptedV3 || effect.Revision != request.ExpectedEffectRevision || !ports.SameOperationSubjectV3(effect.Intent.Operation, request.Operation) {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "Review Authorization V5 requires the exact accepted Effect")
	}
	_, review, governance, now, err := g.inspectCurrentV5(ctx, effect.Intent, request.Basis, baseline)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if !now.Before(time.Unix(0, effect.Intent.ExpiresUnixNano)) {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review Authorization V5 Effect expired during current reads")
	}
	intentDigest, err := effect.Intent.DigestV3()
	if err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	intentBinding := ports.OperationReviewIntentBindingV4{
		Operation: effect.Intent.Operation, IntentID: effect.Intent.ID, IntentRevision: effect.Intent.Revision,
		IntentDigest: intentDigest, EffectFactRevision: effect.Revision, Target: effect.Intent.Target,
		PayloadSchema: effect.Intent.Payload.Schema, PayloadDigest: effect.Intent.Payload.ContentDigest,
		PayloadRevision: effect.Intent.PayloadRevision, Provider: effect.Intent.Provider, Authority: effect.Intent.Authority,
		ReviewBinding: effect.Intent.Review, DispatchPolicy: effect.Intent.Policy, IntentExpires: effect.Intent.ExpiresUnixNano,
	}
	expires := minimumTimeV5(now.Add(request.RequestedTTL), time.Unix(0, effect.Intent.ExpiresUnixNano), time.Unix(0, review.ExpiresUnixNano), time.Unix(0, governance.ExpiresUnixNano))
	if !expires.After(now) {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review Authorization V5 has no positive current TTL")
	}
	boundary := core.FenceBoundaryActivation
	if effect.Intent.Operation.ExecutionScope.SandboxLease != nil {
		boundary = core.FenceBoundaryInstance
	}
	fence := core.ExecutionFence{
		BoundaryScope: boundary, Scope: effect.Intent.Operation.ExecutionScope,
		CapabilityGrantDigest: governance.CapabilityGrantDigest,
		EffectIntentID:        effect.Intent.ID, EffectIntentRevision: effect.Intent.Revision,
		CanonicalPayloadDigest: effect.Intent.Payload.ContentDigest, ExpiresAt: expires,
	}
	fenceDigest, err := ports.DigestOperationExecutionFenceV3(fence, effect.Intent.Operation)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	fact, err := ports.SealOperationReviewAuthorizationFactV5(ports.OperationReviewAuthorizationFactV5{
		ID: request.AuthorizationID, Revision: 1, State: ports.OperationReviewAuthorizationActiveV5,
		Intent: intentBinding, Review: review, Governance: governance, Fence: fence, FenceDigest: fenceDigest,
		RequestedTTLUnixNano: request.RequestedTTL.Nanoseconds(), CreatedUnixNano: now.UnixNano(),
		UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	created, err := g.Facts.CreateOperationReviewAuthorizationV5(ctx, fact)
	if err == nil {
		if created.Digest != fact.Digest || created.Validate() != nil {
			return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review Authorization V5 owner returned different content")
		}
		return created, nil
	}
	if !isUnknownOrConflictV5(err) {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	recovered, inspectErr := g.inspectFactV5(context.WithoutCancel(ctx), request.AuthorizationID)
	if inspectErr != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if recovered.Digest != fact.Digest {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Review Authorization V5 ID contains different content")
	}
	return recovered, recovered.Validate()
}

func (g OperationReviewAuthorizationGatewayV5) InspectCurrentOperationReviewAuthorizationV5(ctx context.Context, operation ports.OperationSubjectV3, effectID core.EffectIntentID, authorizationID string) (ports.OperationReviewAuthorizationFactV5, error) {
	if err := g.validate(); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if err := operation.Validate(); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if effectID == "" || authorizationID == "" {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictMissing, "current Review Authorization V5 inspection requires exact identities")
	}
	baseline := g.Clock()
	if baseline.IsZero() {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review Authorization V5 baseline clock is zero")
	}
	fact, err := g.inspectFactV5(ctx, authorizationID)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if err := fact.Validate(); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if fact.State != ports.OperationReviewAuthorizationActiveV5 || fact.Intent.IntentID != effectID || !ports.SameOperationSubjectV3(fact.Intent.Operation, operation) {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review Authorization V5 is inactive or belongs to another operation")
	}
	effect, err := g.inspectEffectV5(ctx, operation, effectID)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if effect.State != control.OperationEffectAcceptedV3 && effect.State != control.OperationEffectDispatchIntentV3 {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "Review Authorization V5 is current only before provider contact")
	}
	intentDigest, err := effect.Intent.DigestV3()
	if err != nil || intentDigest != fact.Intent.IntentDigest || effect.Intent.Revision != fact.Intent.IntentRevision || effect.Intent.Payload.ContentDigest != fact.Intent.PayloadDigest {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewCandidateConflict, "authorized V5 Intent or payload drifted")
	}
	_, review, governance, now, err := g.inspectCurrentV5(ctx, effect.Intent, fact.Review.Basis, baseline)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if !now.Before(time.Unix(0, fact.ExpiresUnixNano)) || review.ProjectionDigest != fact.Review.ProjectionDigest || governance.SnapshotDigest != fact.Governance.SnapshotDigest || governance.CapabilityGrantDigest != fact.Governance.CapabilityGrantDigest {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review V5 or operation governance drifted")
	}
	return fact, nil
}

func (g OperationReviewAuthorizationGatewayV5) CompareAndSwapOperationReviewAuthorizationV5(ctx context.Context, request ports.OperationReviewAuthorizationCASRequestV5) (ports.OperationReviewAuthorizationFactV5, error) {
	if err := g.validate(); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if request.ExpectedRevision == 0 {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "Review Authorization V5 CAS requires an expected revision")
	}
	if err := request.Next.Validate(); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	updated, err := g.Facts.CompareAndSwapOperationReviewAuthorizationV5(ctx, request)
	if err == nil {
		if updated.Digest != request.Next.Digest {
			return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review Authorization V5 CAS owner returned different content")
		}
		return updated, nil
	}
	if !isUnknownOrConflictV5(err) {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	recovered, inspectErr := g.inspectFactExactV5(context.WithoutCancel(ctx), request.Next.RefV5())
	if inspectErr != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if recovered.Digest != request.Next.Digest {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Review Authorization V5 CAS resolved to different content")
	}
	return recovered, nil
}

func (g OperationReviewAuthorizationGatewayV5) inspectCurrentV5(ctx context.Context, intent ports.OperationEffectIntentV3, basis ports.OperationReviewAuthorizationBasisV5, baseline time.Time) (ports.OperationGovernanceSnapshotV3, ports.OperationReviewCurrentProjectionV5, ports.OperationReviewGovernanceBindingV4, time.Time, error) {
	first, err := g.inspectGovernanceV5(ctx, intent.Operation)
	if err != nil {
		return ports.OperationGovernanceSnapshotV3{}, ports.OperationReviewCurrentProjectionV5{}, ports.OperationReviewGovernanceBindingV4{}, time.Time{}, err
	}
	if err := validateOperationGovernanceCurrentForReviewV5(first, intent, baseline); err != nil {
		return ports.OperationGovernanceSnapshotV3{}, ports.OperationReviewCurrentProjectionV5{}, ports.OperationReviewGovernanceBindingV4{}, time.Time{}, err
	}
	review, err := g.inspectReviewV5(ctx, ports.OperationReviewCurrentRequestV5{Intent: intent, Basis: basis})
	if err != nil {
		return ports.OperationGovernanceSnapshotV3{}, ports.OperationReviewCurrentProjectionV5{}, ports.OperationReviewGovernanceBindingV4{}, time.Time{}, err
	}
	second, err := g.inspectGovernanceV5(ctx, intent.Operation)
	if err != nil {
		return ports.OperationGovernanceSnapshotV3{}, ports.OperationReviewCurrentProjectionV5{}, ports.OperationReviewGovernanceBindingV4{}, time.Time{}, err
	}
	now := g.Clock()
	if now.IsZero() || now.Before(baseline) {
		return ports.OperationGovernanceSnapshotV3{}, ports.OperationReviewCurrentProjectionV5{}, ports.OperationReviewGovernanceBindingV4{}, time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review Authorization V5 clock regressed during current reads")
	}
	if err := validateOperationGovernanceCurrentForReviewV5(first, intent, now); err != nil {
		return ports.OperationGovernanceSnapshotV3{}, ports.OperationReviewCurrentProjectionV5{}, ports.OperationReviewGovernanceBindingV4{}, time.Time{}, err
	}
	if err := validateOperationGovernanceCurrentForReviewV5(second, intent, now); err != nil {
		return ports.OperationGovernanceSnapshotV3{}, ports.OperationReviewCurrentProjectionV5{}, ports.OperationReviewGovernanceBindingV4{}, time.Time{}, err
	}
	if err := review.ValidateAgainstIntent(intent, second, now); err != nil {
		return ports.OperationGovernanceSnapshotV3{}, ports.OperationReviewCurrentProjectionV5{}, ports.OperationReviewGovernanceBindingV4{}, time.Time{}, err
	}
	firstDigest, err := ports.DigestOperationGovernanceForReviewAuthorizationV5(first)
	if err != nil {
		return ports.OperationGovernanceSnapshotV3{}, ports.OperationReviewCurrentProjectionV5{}, ports.OperationReviewGovernanceBindingV4{}, time.Time{}, err
	}
	secondDigest, err := ports.DigestOperationGovernanceForReviewAuthorizationV5(second)
	if err != nil || secondDigest != firstDigest {
		return ports.OperationGovernanceSnapshotV3{}, ports.OperationReviewCurrentProjectionV5{}, ports.OperationReviewGovernanceBindingV4{}, time.Time{}, core.NewError(core.ErrorConflict, core.ReasonEffectFenceStale, "operation governance changed across Review V5 S1/S2")
	}
	credentialDigest, err := ports.DigestOperationCredentialFactsV3(second.Credentials, now)
	if err != nil {
		return ports.OperationGovernanceSnapshotV3{}, ports.OperationReviewCurrentProjectionV5{}, ports.OperationReviewGovernanceBindingV4{}, time.Time{}, err
	}
	governance := ports.OperationReviewGovernanceBindingV4{
		SnapshotDigest: secondDigest, ProjectionWatermark: second.ProjectionWatermark,
		Identity: second.Identity, Binding: second.Binding, CurrentScope: second.CurrentScope,
		Authority: second.Authority, Policy: second.Policy, Budget: second.Budget,
		CapabilityGrantDigest: second.CapabilityGrantDigest, CredentialGrantDigest: credentialDigest,
		ExpiresUnixNano: second.ExpiresUnixNano,
	}
	if err := governance.Validate(now); err != nil {
		return ports.OperationGovernanceSnapshotV3{}, ports.OperationReviewCurrentProjectionV5{}, ports.OperationReviewGovernanceBindingV4{}, time.Time{}, err
	}
	return second, review, governance, now, nil
}

func validateOperationGovernanceCurrentForReviewV5(s ports.OperationGovernanceSnapshotV3, intent ports.OperationEffectIntentV3, now time.Time) error {
	if !s.Active || s.ProjectionWatermark == 0 || !ports.SameOperationSubjectV3(s.Operation, intent.Operation) || now.IsZero() || !now.Before(time.Unix(0, s.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "operation governance is inactive, expired or drifted")
	}
	for _, ref := range []ports.OperationGovernanceFactRefV3{s.Identity, s.Binding, s.CurrentScope, s.Authority, s.Budget, s.Policy} {
		if err := ref.Validate(now); err != nil {
			return err
		}
	}
	if s.CurrentScope.Ref != intent.Operation.CurrentProjectionRef || s.CurrentScope.Revision != intent.Operation.CurrentProjectionRevision || s.CurrentScope.Digest != intent.Operation.CurrentProjectionDigest || s.Provider != intent.Provider || s.EnforcementPoint != intent.Provider || s.Binding.Ref != intent.Provider.BindingSetID || s.Binding.Revision != intent.Provider.BindingSetRevision || s.Authority.Ref != intent.Authority.Ref || s.Authority.Revision != intent.Authority.Revision || s.Authority.Digest != intent.Authority.Digest || s.Budget.Ref != intent.Budget.Ref || s.Budget.Revision != intent.Budget.Revision || s.Budget.Digest != intent.Budget.Digest || s.Policy.Ref != intent.Policy.Ref || s.Policy.Revision != intent.Policy.Revision || s.Policy.Digest != intent.Policy.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectAuthorizationMissing, "operation governance facts drifted from exact V5 Intent bindings")
	}
	if err := s.CapabilityGrantDigest.Validate(); err != nil {
		return err
	}
	if len(s.Credentials) != len(intent.CredentialLeases) || len(s.Credentials) > 64 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCredentialLeaseMissing, "operation credential set drifted")
	}
	for index, credential := range s.Credentials {
		if err := credential.Validate(now); err != nil {
			return err
		}
		if credential.Lease != intent.CredentialLeases[index] {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonCredentialLeaseMissing, "operation credential lease drifted")
		}
	}
	return nil
}

func (g OperationReviewAuthorizationGatewayV5) inspectFactV5(ctx context.Context, id string) (ports.OperationReviewAuthorizationFactV5, error) {
	fact, err := g.Facts.InspectOperationReviewAuthorizationV5(ctx, id)
	if !isUnknownV5(err) {
		return fact, err
	}
	return g.Facts.InspectOperationReviewAuthorizationV5(context.WithoutCancel(ctx), id)
}

func (g OperationReviewAuthorizationGatewayV5) inspectFactExactV5(ctx context.Context, ref ports.OperationReviewAuthorizationRefV5) (ports.OperationReviewAuthorizationFactV5, error) {
	fact, err := g.Facts.InspectOperationReviewAuthorizationExactV5(ctx, ref)
	if !isUnknownV5(err) {
		return fact, err
	}
	return g.Facts.InspectOperationReviewAuthorizationExactV5(context.WithoutCancel(ctx), ref)
}

func (g OperationReviewAuthorizationGatewayV5) inspectEffectV5(ctx context.Context, operation ports.OperationSubjectV3, id core.EffectIntentID) (control.OperationEffectFactV3, error) {
	fact, err := g.Effects.InspectOperationEffectV3(ctx, operation, id)
	if !isUnknownV5(err) {
		return fact, err
	}
	return g.Effects.InspectOperationEffectV3(context.WithoutCancel(ctx), operation, id)
}

func (g OperationReviewAuthorizationGatewayV5) inspectGovernanceV5(ctx context.Context, operation ports.OperationSubjectV3) (ports.OperationGovernanceSnapshotV3, error) {
	value, err := g.Governance.InspectOperationGovernance(ctx, operation)
	if !isUnknownV5(err) {
		return value, err
	}
	return g.Governance.InspectOperationGovernance(context.WithoutCancel(ctx), operation)
}

func (g OperationReviewAuthorizationGatewayV5) inspectReviewV5(ctx context.Context, request ports.OperationReviewCurrentRequestV5) (ports.OperationReviewCurrentProjectionV5, error) {
	value, err := g.Reviews.InspectOperationReviewCurrentV5(ctx, request)
	if !isUnknownV5(err) {
		return value, err
	}
	return g.Reviews.InspectOperationReviewCurrentV5(context.WithoutCancel(ctx), request)
}

func exactOperationReviewAuthorizationRequestV5(fact ports.OperationReviewAuthorizationFactV5, request ports.CreateOperationReviewAuthorizationRequestV5) (ports.OperationReviewAuthorizationFactV5, error) {
	if err := fact.Validate(); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if fact.ID != request.AuthorizationID || fact.Intent.IntentID != request.EffectID || fact.Intent.EffectFactRevision != request.ExpectedEffectRevision || !ports.SameOperationSubjectV3(fact.Intent.Operation, request.Operation) || fact.Review.Basis != request.Basis || fact.RequestedTTLUnixNano != request.RequestedTTL.Nanoseconds() {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Review Authorization V5 ID already binds another request")
	}
	return fact, nil
}

func isUnknownV5(err error) bool {
	return core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate)
}

func isUnknownOrConflictV5(err error) bool {
	return isUnknownV5(err) || core.HasCategory(err, core.ErrorConflict)
}

func nilInterfaceV5(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

func minimumTimeV5(values ...time.Time) time.Time {
	minimum := values[0]
	for _, value := range values[1:] {
		if value.Before(minimum) {
			minimum = value
		}
	}
	return minimum
}

var _ ports.OperationReviewAuthorizationGovernancePortV5 = OperationReviewAuthorizationGatewayV5{}

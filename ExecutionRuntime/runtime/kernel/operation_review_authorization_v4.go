package kernel

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// OperationReviewAuthorizationGatewayV4 is the sole Runtime authorization
// owner. Review supplies current Verdict references; it never creates this
// Fact, a Permit, an Enforcement receipt, or a settlement.
type OperationReviewAuthorizationGatewayV4 struct {
	Facts      ports.OperationReviewAuthorizationFactPortV4
	Effects    control.OperationEffectFactPortV3
	Governance ports.OperationGovernanceCurrentReaderV3
	Reviews    ports.OperationReviewCurrentReaderV4
	Clock      func() time.Time
}

func (g OperationReviewAuthorizationGatewayV4) validate() error {
	if g.Facts == nil || g.Effects == nil || g.Governance == nil || g.Reviews == nil || g.Clock == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review Authorization Fact owner, Effect owner, current readers and clock are required")
	}
	return nil
}

func (g OperationReviewAuthorizationGatewayV4) CreateOperationReviewAuthorizationV4(ctx context.Context, request ports.CreateOperationReviewAuthorizationRequestV4) (ports.OperationReviewAuthorizationFactV4, error) {
	if err := g.validate(); err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	existing, err := g.Facts.InspectOperationReviewAuthorizationV4(ctx, request.AuthorizationID)
	if err == nil {
		return exactOperationReviewAuthorizationRequestV4(existing, request)
	}
	if !core.HasCategory(err, core.ErrorNotFound) {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	now := g.Clock()
	if now.IsZero() {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review Authorization clock returned zero")
	}
	effect, err := g.Effects.InspectOperationEffectV3(ctx, request.Operation, request.EffectID)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	if effect.State != control.OperationEffectAcceptedV3 || effect.Revision != request.ExpectedEffectRevision || !ports.SameOperationSubjectV3(effect.Intent.Operation, request.Operation) || !now.Before(time.Unix(0, effect.Intent.ExpiresUnixNano)) {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "Review Authorization requires an exact accepted Effect")
	}
	_, review, governance, err := g.inspectCurrent(ctx, effect.Intent, now)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	intentDigest, _ := effect.Intent.DigestV3()
	intentBinding := ports.OperationReviewIntentBindingV4{
		Operation: effect.Intent.Operation, IntentID: effect.Intent.ID, IntentRevision: effect.Intent.Revision,
		IntentDigest: intentDigest, EffectFactRevision: effect.Revision,
		Target:        effect.Intent.Target,
		PayloadSchema: effect.Intent.Payload.Schema, PayloadDigest: effect.Intent.Payload.ContentDigest,
		PayloadRevision: effect.Intent.PayloadRevision, Provider: effect.Intent.Provider,
		Authority: effect.Intent.Authority, ReviewBinding: effect.Intent.Review, DispatchPolicy: effect.Intent.Policy,
		IntentExpires: effect.Intent.ExpiresUnixNano,
	}
	expires := now.Add(request.RequestedTTL)
	for _, limit := range []int64{effect.Intent.ExpiresUnixNano, review.ExpiresUnixNano, governance.ExpiresUnixNano} {
		value := time.Unix(0, limit)
		if value.Before(expires) {
			expires = value
		}
	}
	if !expires.After(now) {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review Authorization has no positive current TTL")
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
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	fact, err := ports.SealOperationReviewAuthorizationFactV4(ports.OperationReviewAuthorizationFactV4{
		ID: request.AuthorizationID, Revision: 1, State: ports.OperationReviewAuthorizationActiveV4,
		Intent: intentBinding, Review: review, Governance: governance,
		Fence: fence, FenceDigest: fenceDigest, RequestedTTLUnixNano: request.RequestedTTL.Nanoseconds(),
		CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	created, err := g.Facts.CreateOperationReviewAuthorizationV4(ctx, fact)
	if err == nil {
		if err := created.Validate(); err != nil {
			return ports.OperationReviewAuthorizationFactV4{}, err
		}
		if created.Digest != fact.Digest {
			return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review Authorization owner returned different content")
		}
		return created, nil
	}
	if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorIndeterminate) && !core.HasCategory(err, core.ErrorConflict) {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	recovered, inspectErr := g.Facts.InspectOperationReviewAuthorizationV4(context.WithoutCancel(ctx), request.AuthorizationID)
	if inspectErr != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	if recovered.Digest != fact.Digest {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Review Authorization ID contains different content")
	}
	return recovered, recovered.Validate()
}

func (g OperationReviewAuthorizationGatewayV4) InspectCurrentOperationReviewAuthorizationV4(ctx context.Context, operation ports.OperationSubjectV3, effectID core.EffectIntentID, authorizationID string) (ports.OperationReviewAuthorizationFactV4, error) {
	if err := g.validate(); err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	if err := operation.Validate(); err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	if effectID == "" || authorizationID == "" {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictMissing, "current Review Authorization inspection requires exact identities")
	}
	fact, err := g.Facts.InspectOperationReviewAuthorizationV4(ctx, authorizationID)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	if err := fact.Validate(); err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	now := g.Clock()
	if fact.State != ports.OperationReviewAuthorizationActiveV4 || now.IsZero() || !now.Before(time.Unix(0, fact.ExpiresUnixNano)) || fact.Intent.IntentID != effectID || !ports.SameOperationSubjectV3(fact.Intent.Operation, operation) {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review Authorization is inactive, expired or belongs to another operation")
	}
	effect, err := g.Effects.InspectOperationEffectV3(ctx, operation, effectID)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	if effect.State != control.OperationEffectAcceptedV3 && effect.State != control.OperationEffectDispatchIntentV3 {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "Review Authorization is only current before provider contact")
	}
	intentDigest, err := effect.Intent.DigestV3()
	if err != nil || intentDigest != fact.Intent.IntentDigest || effect.Intent.Revision != fact.Intent.IntentRevision || effect.Intent.Payload.ContentDigest != fact.Intent.PayloadDigest {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewCandidateConflict, "authorized operation Intent or payload drifted")
	}
	_, review, governance, err := g.inspectCurrent(ctx, effect.Intent, now)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	if review.ProjectionDigest != fact.Review.ProjectionDigest || governance.SnapshotDigest != fact.Governance.SnapshotDigest || governance.CapabilityGrantDigest != fact.Governance.CapabilityGrantDigest {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review verdict or operation governance drifted; a new Authorization is required")
	}
	return fact, nil
}

func (g OperationReviewAuthorizationGatewayV4) inspectCurrent(ctx context.Context, intent ports.OperationEffectIntentV3, now time.Time) (ports.OperationGovernanceSnapshotV3, ports.OperationReviewCurrentProjectionV4, ports.OperationReviewGovernanceBindingV4, error) {
	current, err := g.Governance.InspectOperationGovernance(ctx, intent.Operation)
	if err != nil {
		return ports.OperationGovernanceSnapshotV3{}, ports.OperationReviewCurrentProjectionV4{}, ports.OperationReviewGovernanceBindingV4{}, err
	}
	if err := current.ValidateCurrent(intent, now); err != nil {
		return ports.OperationGovernanceSnapshotV3{}, ports.OperationReviewCurrentProjectionV4{}, ports.OperationReviewGovernanceBindingV4{}, err
	}
	review, err := g.Reviews.InspectOperationReviewCurrentV4(ctx, intent)
	if err != nil {
		return ports.OperationGovernanceSnapshotV3{}, ports.OperationReviewCurrentProjectionV4{}, ports.OperationReviewGovernanceBindingV4{}, err
	}
	if err := review.ValidateAgainstIntent(intent, current, now); err != nil {
		return ports.OperationGovernanceSnapshotV3{}, ports.OperationReviewCurrentProjectionV4{}, ports.OperationReviewGovernanceBindingV4{}, err
	}
	snapshotDigest, err := current.DigestV3(now)
	if err != nil {
		return ports.OperationGovernanceSnapshotV3{}, ports.OperationReviewCurrentProjectionV4{}, ports.OperationReviewGovernanceBindingV4{}, err
	}
	credentialDigest, err := ports.DigestOperationCredentialFactsV3(current.Credentials, now)
	if err != nil {
		return ports.OperationGovernanceSnapshotV3{}, ports.OperationReviewCurrentProjectionV4{}, ports.OperationReviewGovernanceBindingV4{}, err
	}
	governance := ports.OperationReviewGovernanceBindingV4{
		SnapshotDigest: snapshotDigest, ProjectionWatermark: current.ProjectionWatermark,
		Identity: current.Identity, Binding: current.Binding, CurrentScope: current.CurrentScope,
		Authority: current.Authority, Policy: current.Policy, Budget: current.Budget,
		CapabilityGrantDigest: current.CapabilityGrantDigest, CredentialGrantDigest: credentialDigest,
		ExpiresUnixNano: current.ExpiresUnixNano,
	}
	if err := governance.Validate(now); err != nil {
		return ports.OperationGovernanceSnapshotV3{}, ports.OperationReviewCurrentProjectionV4{}, ports.OperationReviewGovernanceBindingV4{}, err
	}
	return current, review, governance, nil
}

func exactOperationReviewAuthorizationRequestV4(fact ports.OperationReviewAuthorizationFactV4, request ports.CreateOperationReviewAuthorizationRequestV4) (ports.OperationReviewAuthorizationFactV4, error) {
	if err := fact.Validate(); err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	if fact.ID != request.AuthorizationID || fact.Intent.IntentID != request.EffectID || fact.Intent.EffectFactRevision != request.ExpectedEffectRevision || !ports.SameOperationSubjectV3(fact.Intent.Operation, request.Operation) || fact.RequestedTTLUnixNano != request.RequestedTTL.Nanoseconds() {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Review Authorization ID already binds another request")
	}
	return fact, nil
}

var _ ports.OperationReviewAuthorizationGovernancePortV4 = OperationReviewAuthorizationGatewayV4{}

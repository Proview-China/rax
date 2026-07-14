package control

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type IssueGovernedOperationDispatchRequestV3 = ports.IssueGovernedOperationDispatchRequestV3
type BeginGovernedOperationDispatchRequestV3 = ports.BeginGovernedOperationDispatchRequestV3

// OperationGovernanceGatewayV3 is the only public Application-facing Issue
// and Begin gate for Operation Effects. OperationEffectFactPortV3 remains the
// raw Fact Owner primitive and must not be used by Application orchestration.
type OperationGovernanceGatewayV3 struct {
	Effects OperationEffectFactPortV3
	Current ports.OperationGovernanceCurrentReaderV3
	Clock   func() time.Time
}

func (g OperationGovernanceGatewayV3) Issue(ctx context.Context, request IssueGovernedOperationDispatchRequestV3) (IssueOperationPermitResultV3, error) {
	if err := g.validate(); err != nil {
		return IssueOperationPermitResultV3{}, err
	}
	if err := request.Operation.Validate(); err != nil {
		return IssueOperationPermitResultV3{}, err
	}
	if strings.TrimSpace(string(request.EffectID)) == "" || request.ExpectedEffectRevision == 0 || strings.TrimSpace(request.PermitID) == "" || strings.TrimSpace(request.AttemptID) == "" || request.PermitTTL <= 0 || request.PermitTTL > ports.MaxDispatchPermitTTL {
		return IssueOperationPermitResultV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "operation Issue requires exact Effect, Permit, attempt and bounded TTL")
	}
	now := g.Clock()
	if now.IsZero() {
		return IssueOperationPermitResultV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "operation gateway clock returned zero")
	}
	effect, err := g.Effects.InspectOperationEffectV3(ctx, request.Operation, request.EffectID)
	if err != nil {
		return IssueOperationPermitResultV3{}, err
	}
	if effect.State != OperationEffectAcceptedV3 || effect.Revision != request.ExpectedEffectRevision || !ports.SameOperationSubjectV3(effect.Intent.Operation, request.Operation) || !now.Before(time.Unix(0, effect.Intent.ExpiresUnixNano)) {
		return IssueOperationPermitResultV3{}, core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "operation Issue requires current accepted unexpired Effect")
	}
	current, err := g.Current.InspectOperationGovernance(ctx, request.Operation)
	if err != nil {
		return IssueOperationPermitResultV3{}, err
	}
	if err := current.ValidateCurrent(effect.Intent, now); err != nil {
		return IssueOperationPermitResultV3{}, err
	}
	credentialDigest, err := ports.DigestOperationCredentialFactsV3(current.Credentials, now)
	if err != nil {
		return IssueOperationPermitResultV3{}, err
	}
	snapshotDigest, err := current.DigestV3(now)
	if err != nil {
		return IssueOperationPermitResultV3{}, err
	}
	expires := now.Add(request.PermitTTL)
	limits := []int64{
		effect.Intent.ExpiresUnixNano,
		current.ExpiresUnixNano,
		current.Identity.ExpiresUnixNano,
		current.Binding.ExpiresUnixNano,
		current.CurrentScope.ExpiresUnixNano,
		current.Authority.ExpiresUnixNano,
		current.Review.ExpiresUnixNano,
		current.Review.Case.ExpiresUnixNano,
		current.Review.Verdict.ExpiresUnixNano,
		current.Review.ReviewerAuthority.ExpiresUnixNano,
		current.Budget.ExpiresUnixNano,
		current.Policy.ExpiresUnixNano,
	}
	for _, credential := range current.Credentials {
		limits = append(limits, credential.ExpiresUnixNano)
	}
	if current.Review.Satisfaction != nil {
		limits = append(limits, current.Review.Satisfaction.ExpiresUnixNano)
	}
	for _, limit := range limits {
		value := time.Unix(0, limit)
		if value.Before(expires) {
			expires = value
		}
	}
	if !expires.After(now) {
		return IssueOperationPermitResultV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitExpired, "a current operation governance fact expires at Issue")
	}
	boundary := core.FenceBoundaryActivation
	if current.Operation.ExecutionScope.SandboxLease != nil {
		boundary = core.FenceBoundaryInstance
	}
	fence := core.ExecutionFence{
		BoundaryScope:          boundary,
		Scope:                  current.Operation.ExecutionScope,
		CapabilityGrantDigest:  current.CapabilityGrantDigest,
		EffectIntentID:         effect.Intent.ID,
		EffectIntentRevision:   effect.Intent.Revision,
		CanonicalPayloadDigest: effect.Intent.Payload.ContentDigest,
		ExpiresAt:              expires,
	}
	fenceDigest, err := ports.DigestOperationExecutionFenceV3(fence, effect.Intent.Operation)
	if err != nil {
		return IssueOperationPermitResultV3{}, err
	}
	intentDigest, _ := effect.Intent.DigestV3()
	permit := ports.OperationDispatchPermitV3{
		ContractVersion:          ports.OperationEffectContractVersionV3,
		ID:                       request.PermitID,
		Revision:                 1,
		AttemptID:                request.AttemptID,
		IntentID:                 effect.Intent.ID,
		IntentRevision:           effect.Intent.Revision,
		IntentDigest:             intentDigest,
		Operation:                effect.Intent.Operation,
		PayloadSchema:            effect.Intent.Payload.Schema,
		PayloadDigest:            effect.Intent.Payload.ContentDigest,
		PayloadRevision:          effect.Intent.PayloadRevision,
		ConflictDomain:           effect.Intent.ConflictDomain,
		Provider:                 current.Provider,
		EnforcementPoint:         current.EnforcementPoint,
		Authority:                effect.Intent.Authority,
		Review:                   effect.Intent.Review,
		ReviewAuthorization:      current.Review,
		Budget:                   effect.Intent.Budget,
		Policy:                   effect.Intent.Policy,
		CapabilityGrantDigest:    current.CapabilityGrantDigest,
		CredentialGrantDigest:    credentialDigest,
		GovernanceSnapshotDigest: snapshotDigest,
		FenceDigest:              fenceDigest,
		Idempotency:              effect.Intent.Idempotency,
		IssuedUnixNano:           now.UnixNano(),
		ExpiresUnixNano:          expires.UnixNano(),
	}
	if err := ports.ValidateOperationAtExecutionPointV3(permit, effect.Intent, fence, current, now); err != nil {
		return IssueOperationPermitResultV3{}, err
	}
	result, err := g.Effects.IssueOperationDispatchPermitV3(ctx, IssueOperationPermitRequestV3{
		Operation:              request.Operation,
		EffectID:               request.EffectID,
		ExpectedEffectRevision: request.ExpectedEffectRevision,
		Permit:                 permit,
		Fence:                  fence,
	})
	if err == nil {
		return result, nil
	}
	if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorIndeterminate) && !core.HasCategory(err, core.ErrorConflict) {
		return IssueOperationPermitResultV3{}, err
	}
	recoveryCtx := context.WithoutCancel(ctx)
	inspectedPermit, permitErr := g.Effects.InspectOperationDispatchPermitV3(recoveryCtx, request.Operation, request.PermitID)
	inspectedEffect, effectErr := g.Effects.InspectOperationEffectV3(recoveryCtx, request.Operation, request.EffectID)
	permitDigest, _ := permit.DigestV3()
	if permitErr != nil || effectErr != nil || inspectedPermit.PermitDigest != permitDigest || inspectedPermit.Fence != fence || inspectedEffect.State != OperationEffectDispatchIntentV3 || inspectedEffect.DispatchPermitID != permit.ID || inspectedEffect.DispatchPermitDigest != permitDigest || inspectedPermit.EffectFactRevision != inspectedEffect.Revision {
		return IssueOperationPermitResultV3{}, err
	}
	return IssueOperationPermitResultV3{Effect: inspectedEffect, Permit: inspectedPermit}, nil
}

// Begin is the final host gate. It independently re-reads every current
// governance source after Issue and before the actual provider Prepare gate.
func (g OperationGovernanceGatewayV3) Begin(ctx context.Context, request BeginGovernedOperationDispatchRequestV3) (OperationDispatchPermitFactV3, error) {
	if err := g.validate(); err != nil {
		return OperationDispatchPermitFactV3{}, err
	}
	if err := request.Validate(); err != nil {
		return OperationDispatchPermitFactV3{}, err
	}
	now := g.Clock()
	if now.IsZero() {
		return OperationDispatchPermitFactV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "operation gateway clock returned zero")
	}
	effect, err := g.Effects.InspectOperationEffectV3(ctx, request.Operation, request.EffectID)
	if err != nil {
		return OperationDispatchPermitFactV3{}, err
	}
	permit, err := g.Effects.InspectOperationDispatchPermitV3(ctx, request.Operation, request.PermitID)
	if err != nil {
		return OperationDispatchPermitFactV3{}, err
	}
	if effect.State != OperationEffectDispatchIntentV3 || effect.Revision != request.ExpectedEffectRevision || permit.State != OperationPermitIssuedV3 || permit.Revision != request.ExpectedPermitRevision || permit.EffectFactRevision != effect.Revision || effect.DispatchPermitID != permit.Permit.ID {
		return OperationDispatchPermitFactV3{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "operation Begin requires current dispatch intent and issued Permit revisions")
	}
	current, err := g.Current.InspectOperationGovernance(ctx, request.Operation)
	if err != nil {
		return OperationDispatchPermitFactV3{}, err
	}
	if err := ports.ValidateOperationAtExecutionPointV3(permit.Permit, effect.Intent, permit.Fence, current, now); err != nil {
		return OperationDispatchPermitFactV3{}, err
	}
	result, err := g.Effects.BeginOperationDispatchV3(ctx, BeginOperationDispatchRequestV3{
		Operation:              request.Operation,
		EffectID:               request.EffectID,
		ExpectedEffectRevision: request.ExpectedEffectRevision,
		PermitID:               request.PermitID,
		ExpectedPermitRevision: request.ExpectedPermitRevision,
	})
	if err == nil {
		return result, nil
	}
	if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorIndeterminate) && !core.HasCategory(err, core.ErrorConflict) {
		return OperationDispatchPermitFactV3{}, err
	}
	inspected, inspectErr := g.Effects.InspectOperationDispatchPermitV3(context.WithoutCancel(ctx), request.Operation, request.PermitID)
	if inspectErr != nil || inspected.State != OperationPermitBegunV3 || inspected.PermitDigest != permit.PermitDigest || inspected.Permit.ID != request.PermitID || inspected.EffectFactRevision != effect.Revision {
		return OperationDispatchPermitFactV3{}, err
	}
	return inspected, nil
}

func (g OperationGovernanceGatewayV3) IssueOperationDispatchV3(ctx context.Context, request ports.IssueGovernedOperationDispatchRequestV3) (ports.OperationDispatchAuthorizationV3, error) {
	result, err := g.Issue(ctx, request)
	if err != nil {
		return ports.OperationDispatchAuthorizationV3{}, err
	}
	return operationDispatchAuthorizationV3(result.Effect, result.Permit)
}

func (g OperationGovernanceGatewayV3) BeginOperationDispatchV3(ctx context.Context, request ports.BeginGovernedOperationDispatchRequestV3) (ports.OperationDispatchAuthorizationV3, error) {
	permit, err := g.Begin(ctx, request)
	if err != nil {
		return ports.OperationDispatchAuthorizationV3{}, err
	}
	effect, err := g.Effects.InspectOperationEffectV3(context.WithoutCancel(ctx), request.Operation, request.EffectID)
	if err != nil {
		return ports.OperationDispatchAuthorizationV3{}, err
	}
	return operationDispatchAuthorizationV3(effect, permit)
}

// MarkOperationDispatchUnknownV3 is the only Application-facing recovery
// transition after a Permit has begun but Provider contact cannot be proved.
// It never rejects or reissues the attempt.
func (g OperationGovernanceGatewayV3) MarkOperationDispatchUnknownV3(ctx context.Context, request ports.MarkOperationDispatchUnknownRequestV3) (ports.OperationDispatchAuthorizationV3, error) {
	if err := g.validate(); err != nil {
		return ports.OperationDispatchAuthorizationV3{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.OperationDispatchAuthorizationV3{}, err
	}
	now := g.Clock()
	if now.IsZero() {
		return ports.OperationDispatchAuthorizationV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "operation gateway clock returned zero")
	}
	effect, err := g.Effects.InspectOperationEffectV3(ctx, request.Operation, request.EffectID)
	if err != nil {
		return ports.OperationDispatchAuthorizationV3{}, err
	}
	permit, err := g.Effects.InspectOperationDispatchPermitV3(ctx, request.Operation, request.Permit.PermitID)
	if err != nil {
		return ports.OperationDispatchAuthorizationV3{}, err
	}
	expected, err := operationDispatchAuthorizationV3(effect, permit)
	if err != nil || !sameOperationDispatchAttemptV3(request.Permit, expected.Attempt) || permit.State != OperationPermitBegunV3 {
		return ports.OperationDispatchAuthorizationV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "unknown outcome requires the exact begun Permit and attempt")
	}
	if effect.State == OperationEffectUnknownOutcomeV3 {
		expected.State = ports.OperationDispatchAuthorizationUnknownV3
		return expected, nil
	}
	if effect.State != OperationEffectDispatchIntentV3 || effect.Revision != request.ExpectedEffectRevision {
		return ports.OperationDispatchAuthorizationV3{}, core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "only the exact begun dispatch intent can become unknown")
	}
	next := effect
	next.State = OperationEffectUnknownOutcomeV3
	next.Revision++
	next.UpdatedUnixNano = now.UnixNano()
	stored, err := g.Effects.CompareAndSwapOperationEffectV3(ctx, request.Operation, OperationEffectCASRequestV3{ExpectedRevision: effect.Revision, Next: next})
	if err != nil {
		if !recoverableOperationWriteErrorV3(err) {
			return ports.OperationDispatchAuthorizationV3{}, err
		}
		stored, err = g.Effects.InspectOperationEffectV3(context.WithoutCancel(ctx), request.Operation, request.EffectID)
		if err != nil || stored.State != OperationEffectUnknownOutcomeV3 || stored.IntentDigest != effect.IntentDigest || stored.DispatchPermitID != permit.Permit.ID || stored.DispatchPermitDigest != permit.PermitDigest {
			return ports.OperationDispatchAuthorizationV3{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "cannot prove operation unknown-outcome transition")
		}
	}
	result, err := operationDispatchAuthorizationV3(stored, permit)
	if err != nil {
		return ports.OperationDispatchAuthorizationV3{}, err
	}
	result.State = ports.OperationDispatchAuthorizationUnknownV3
	return result, nil
}

func (g OperationGovernanceGatewayV3) InspectOperationDispatchAuthorizationV3(ctx context.Context, subject ports.OperationSubjectV3, effectID core.EffectIntentID, permitID string) (ports.OperationDispatchAuthorizationV3, error) {
	if err := g.validate(); err != nil {
		return ports.OperationDispatchAuthorizationV3{}, err
	}
	if err := subject.Validate(); err != nil {
		return ports.OperationDispatchAuthorizationV3{}, err
	}
	if strings.TrimSpace(string(effectID)) == "" || strings.TrimSpace(permitID) == "" {
		return ports.OperationDispatchAuthorizationV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "operation authorization inspection requires Effect and Permit identities")
	}
	effect, err := g.Effects.InspectOperationEffectV3(ctx, subject, effectID)
	if err != nil {
		return ports.OperationDispatchAuthorizationV3{}, err
	}
	permit, err := g.Effects.InspectOperationDispatchPermitV3(ctx, subject, permitID)
	if err != nil {
		return ports.OperationDispatchAuthorizationV3{}, err
	}
	return operationDispatchAuthorizationV3(effect, permit)
}

func operationDispatchAuthorizationV3(effect OperationEffectFactV3, permit OperationDispatchPermitFactV3) (ports.OperationDispatchAuthorizationV3, error) {
	permitDigest, err := permit.Permit.DigestV3()
	if err != nil || permitDigest != permit.PermitDigest || effect.IntentDigest != permit.Permit.IntentDigest || effect.DispatchPermitID != permit.Permit.ID || effect.DispatchPermitDigest != permit.PermitDigest || permit.EffectFactRevision > effect.Revision {
		return ports.OperationDispatchAuthorizationV3{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "Effect and Permit facts do not form one exact authorization")
	}
	operationDigest, _ := effect.Intent.Operation.DigestV3()
	attempt := ports.OperationDispatchAttemptRefV3{OperationDigest: operationDigest, EffectID: effect.Intent.ID, IntentRevision: effect.Intent.Revision, IntentDigest: effect.IntentDigest, PermitID: permit.Permit.ID, PermitRevision: permit.Permit.Revision, PermitDigest: permit.PermitDigest, AttemptID: permit.Permit.AttemptID}
	state := ports.OperationDispatchAuthorizationIssuedV3
	if permit.State == OperationPermitBegunV3 {
		state = ports.OperationDispatchAuthorizationBegunV3
	}
	if effect.State == OperationEffectUnknownOutcomeV3 {
		state = ports.OperationDispatchAuthorizationUnknownV3
	}
	result := ports.OperationDispatchAuthorizationV3{Attempt: attempt, Permit: permit.Permit, EffectFactRevision: effect.Revision, PermitFactRevision: permit.Revision, State: state, Fence: permit.Fence, ExpiresUnixNano: permit.Permit.ExpiresUnixNano}
	return result, result.Validate()
}

func (g OperationGovernanceGatewayV3) validate() error {
	if g.Effects == nil || g.Current == nil || g.Clock == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "operation gateway requires Fact Owner, current governance reader and injected clock")
	}
	return nil
}

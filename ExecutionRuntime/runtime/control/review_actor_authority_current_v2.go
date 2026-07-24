package control

import (
	"context"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"reflect"
	"time"
)

type ReviewActorAuthorityCurrentFactPortV2 interface {
	ResolveActorAuthorityV2(context.Context, ports.ReviewActorAuthorityCurrentSubjectV2) (ports.ReviewActorAuthorityCurrentProjectionRefV2, error)
	InspectCurrentActorAuthorityV2(context.Context, ports.ReviewActorAuthorityCurrentSubjectV2, ports.ReviewActorAuthorityCurrentProjectionRefV2) (ports.ReviewActorAuthorityCurrentProjectionV2, error)
	InspectHistoricalActorAuthorityV2(context.Context, ports.ReviewActorAuthorityCurrentProjectionRefV2) (ports.ReviewActorAuthorityCurrentProjectionV2, error)
	CommitActorAuthorityV2(context.Context, ports.ReviewActorAuthorityCurrentPublishRequestV2) (ports.ReviewActorAuthorityCurrentPublishReceiptV2, error)
}

type ReviewActorAuthorityCurrentGatewayV2 struct {
	facts     ReviewActorAuthorityCurrentFactPortV2
	targets   ReviewDecisionSubjectProofReaderV1
	authority ports.DispatchAuthorityCurrentReaderV3
	clock     func() time.Time
}

func NewReviewActorAuthorityCurrentGatewayV2(facts ReviewActorAuthorityCurrentFactPortV2, targets ReviewDecisionSubjectProofReaderV1, authority ports.DispatchAuthorityCurrentReaderV3, clock func() time.Time) (*ReviewActorAuthorityCurrentGatewayV2, error) {
	for _, v := range []any{facts, targets, authority, clock} {
		if reviewDecisionGovernanceNilV1(v) {
			return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review actor authority V2 dependency is missing")
		}
	}
	if clock().IsZero() {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review actor authority V2 clock returned zero")
	}
	return &ReviewActorAuthorityCurrentGatewayV2{facts: facts, targets: targets, authority: authority, clock: clock}, nil
}

func (g *ReviewActorAuthorityCurrentGatewayV2) ResolveCurrentReviewActorAuthorityV2(ctx context.Context, request ports.ReviewActorAuthorityCurrentResolveRequestV2) (ports.ReviewActorAuthorityCurrentProjectionRefV2, error) {
	baseline, err := g.nowV2(time.Time{})
	if err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionRefV2{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionRefV2{}, err
	}
	if err := g.proveTargetV2(ctx, request.Subject.Target); err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionRefV2{}, err
	}
	ref, err := g.facts.ResolveActorAuthorityV2(ctx, request.Subject)
	if err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionRefV2{}, err
	}
	if err := g.proveTargetV2(ctx, request.Subject.Target); err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionRefV2{}, err
	}
	checkpoint, err := g.nowV2(baseline)
	if err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionRefV2{}, err
	}
	if _, err := g.inspectCurrentV2(ctx, request.Subject, ref, checkpoint); err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionRefV2{}, err
	}
	return ref, nil
}
func (g *ReviewActorAuthorityCurrentGatewayV2) InspectCurrentReviewActorAuthorityV2(ctx context.Context, subject ports.ReviewActorAuthorityCurrentSubjectV2, expected ports.ReviewActorAuthorityCurrentProjectionRefV2) (ports.ReviewActorAuthorityCurrentProjectionV2, error) {
	baseline, err := g.nowV2(time.Time{})
	if err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	return g.inspectCurrentV2(ctx, subject, expected, baseline)
}
func (g *ReviewActorAuthorityCurrentGatewayV2) inspectCurrentV2(ctx context.Context, subject ports.ReviewActorAuthorityCurrentSubjectV2, expected ports.ReviewActorAuthorityCurrentProjectionRefV2, baseline time.Time) (ports.ReviewActorAuthorityCurrentProjectionV2, error) {
	if err := subject.Validate(); err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	if err := expected.Validate(); err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	if err := g.proveTargetV2(ctx, subject.Target); err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	s1, err := g.authority.InspectCurrentDispatchAuthorityV3(ctx, subject.ActorAuthority)
	if err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	p1, err := g.facts.InspectCurrentActorAuthorityV2(ctx, subject, expected)
	if err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	if !reflect.DeepEqual(s1, p1.Fact) {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, reviewDecisionGovernanceDriftV1("actor Authority S1 drifted from projection")
	}
	if err := g.proveTargetV2(ctx, subject.Target); err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	s2, err := g.authority.InspectCurrentDispatchAuthorityV3(ctx, subject.ActorAuthority)
	if err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	checkpoint, err := g.nowV2(baseline)
	if err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	if !reflect.DeepEqual(s1, s2) {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, reviewDecisionGovernanceDriftV1("actor Authority source changed across S1/S2")
	}
	p2, err := g.facts.InspectCurrentActorAuthorityV2(ctx, subject, expected)
	if err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	if !reflect.DeepEqual(p1, p2) {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, reviewDecisionGovernanceDriftV1("actor Authority projection changed across S1/S2")
	}
	final, err := g.nowV2(checkpoint)
	if err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	if err := p1.ValidateCurrent(expected, subject, final); err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	return p1.Clone(), nil
}
func (g *ReviewActorAuthorityCurrentGatewayV2) InspectHistoricalReviewActorAuthorityV2(ctx context.Context, ref ports.ReviewActorAuthorityCurrentProjectionRefV2) (ports.ReviewActorAuthorityCurrentProjectionV2, error) {
	return g.facts.InspectHistoricalActorAuthorityV2(ctx, ref)
}
func (g *ReviewActorAuthorityCurrentGatewayV2) PublishReviewActorAuthorityCurrentV2(ctx context.Context, request ports.ReviewActorAuthorityCurrentPublishRequestV2) (ports.ReviewActorAuthorityCurrentPublishReceiptV2, error) {
	baseline, err := g.nowV2(time.Time{})
	if err != nil {
		return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, err
	}
	if err := g.proveTargetV2(ctx, request.Value.Subject.Target); err != nil {
		return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, err
	}
	s1, err := g.authority.InspectCurrentDispatchAuthorityV3(ctx, request.Value.Subject.ActorAuthority)
	if err != nil {
		return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, err
	}
	if !reflect.DeepEqual(s1, request.Value.Fact) {
		return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, reviewDecisionGovernanceDriftV1("actor Authority source drifted from publish value")
	}
	if err := g.proveTargetV2(ctx, request.Value.Subject.Target); err != nil {
		return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, err
	}
	s2, err := g.authority.InspectCurrentDispatchAuthorityV3(ctx, request.Value.Subject.ActorAuthority)
	if err != nil {
		return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, err
	}
	now, err := g.nowV2(baseline)
	if err != nil {
		return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, err
	}
	if !reflect.DeepEqual(s1, s2) {
		return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, reviewDecisionGovernanceDriftV1("actor Authority source changed across publish S1/S2")
	}
	if request.Value.Current {
		if err := request.Value.ValidateCurrent(request.Value.Ref, request.Value.Subject, now); err != nil {
			return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, err
		}
	} else if now.UnixNano() < request.Value.CheckedUnixNano {
		return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review actor authority V2 checked time is in the future")
	}
	receipt, err := g.facts.CommitActorAuthorityV2(ctx, request)
	if err == nil {
		return receipt, nil
	}
	if !core.HasCategory(err, core.ErrorIndeterminate) {
		return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, err
	}
	stored, inspectErr := g.facts.InspectHistoricalActorAuthorityV2(context.WithoutCancel(ctx), request.Value.Ref)
	if inspectErr != nil || !reflect.DeepEqual(stored, request.Value) {
		return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, err
	}
	final, clockErr := g.nowV2(now)
	if clockErr != nil {
		return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, clockErr
	}
	if request.Value.Current {
		if currentErr := request.Value.ValidateCurrent(request.Value.Ref, request.Value.Subject, final); currentErr != nil {
			return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, currentErr
		}
	}
	return ports.ReviewActorAuthorityCurrentPublishReceiptV2{Ref: request.Value.Ref, Created: false}, nil
}
func (g *ReviewActorAuthorityCurrentGatewayV2) proveTargetV2(ctx context.Context, expected ports.ReviewDecisionTargetRefV1) error {
	actual, err := g.targets.InspectReviewDecisionTargetProofV1(ctx, expected)
	if err != nil {
		return err
	}
	if actual != expected {
		return reviewDecisionGovernanceDriftV1("Review actor authority Target proof drifted")
	}
	return nil
}
func (g *ReviewActorAuthorityCurrentGatewayV2) nowV2(baseline time.Time) (time.Time, error) {
	now := g.clock()
	if now.IsZero() || (!baseline.IsZero() && now.Before(baseline)) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review actor authority V2 clock regressed")
	}
	return now, nil
}

var _ ports.ReviewActorAuthorityCurrentReaderV2 = (*ReviewActorAuthorityCurrentGatewayV2)(nil)
var _ ports.ReviewActorAuthorityCurrentPublisherV2 = (*ReviewActorAuthorityCurrentGatewayV2)(nil)

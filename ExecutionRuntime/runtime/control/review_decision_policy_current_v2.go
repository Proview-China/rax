package control

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ReviewDecisionPolicyCurrentFactPortV2 is the Runtime Policy Owner's narrow
// append-only journal. It is intentionally separate from V1.
type ReviewDecisionPolicyCurrentFactPortV2 interface {
	ResolvePolicyV2(context.Context, ports.ReviewDecisionPolicyApplicabilitySubjectV2) (ports.ReviewDecisionPolicyCurrentProjectionRefV2, error)
	InspectCurrentPolicyV2(context.Context, ports.ReviewDecisionPolicyApplicabilitySubjectV2, ports.ReviewDecisionPolicyCurrentProjectionRefV2) (ports.ReviewDecisionPolicyCurrentProjectionV2, error)
	InspectHistoricalPolicyV2(context.Context, ports.ReviewDecisionPolicyCurrentProjectionRefV2) (ports.ReviewDecisionPolicyCurrentProjectionV2, error)
	CommitPolicyV2(context.Context, ports.ReviewDecisionPolicyCurrentPublishRequestV2) (ports.ReviewDecisionPolicyCurrentPublishReceiptV2, error)
}

// ReviewDecisionPolicyCurrentGatewayV2 cross-checks the immutable projection
// against the exact ReviewPolicyFactV2 source before and after each current
// read or publish. It dispatches nothing and is not a production root.
type ReviewDecisionPolicyCurrentGatewayV2 struct {
	facts    ReviewDecisionPolicyCurrentFactPortV2
	policies ports.ReviewPolicyFactReaderV2
	clock    func() time.Time
}

func NewReviewDecisionPolicyCurrentGatewayV2(facts ReviewDecisionPolicyCurrentFactPortV2, policies ports.ReviewPolicyFactReaderV2, clock func() time.Time) (*ReviewDecisionPolicyCurrentGatewayV2, error) {
	for _, dependency := range []any{facts, policies, clock} {
		if reviewDecisionPolicyNilV2(dependency) {
			return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review decision policy V2 dependency is missing")
		}
	}
	if clock().IsZero() {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review decision policy V2 clock returned zero")
	}
	return &ReviewDecisionPolicyCurrentGatewayV2{facts: facts, policies: policies, clock: clock}, nil
}

func (g *ReviewDecisionPolicyCurrentGatewayV2) ResolveCurrentReviewDecisionPolicyV2(ctx context.Context, request ports.ReviewDecisionPolicyCurrentResolveRequestV2) (ports.ReviewDecisionPolicyCurrentProjectionRefV2, error) {
	baseline, err := g.nowV2(time.Time{})
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV2{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV2{}, err
	}
	ref, err := g.facts.ResolvePolicyV2(ctx, request.Subject)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV2{}, err
	}
	checkpoint, err := g.nowV2(baseline)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV2{}, err
	}
	if _, err := g.inspectCurrentV2(ctx, request.Subject, ref, checkpoint); err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV2{}, err
	}
	return ref, nil
}

func (g *ReviewDecisionPolicyCurrentGatewayV2) InspectCurrentReviewDecisionPolicyV2(ctx context.Context, subject ports.ReviewDecisionPolicyApplicabilitySubjectV2, expected ports.ReviewDecisionPolicyCurrentProjectionRefV2) (ports.ReviewDecisionPolicyCurrentProjectionV2, error) {
	baseline, err := g.nowV2(time.Time{})
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, err
	}
	return g.inspectCurrentV2(ctx, subject, expected, baseline)
}

func (g *ReviewDecisionPolicyCurrentGatewayV2) inspectCurrentV2(ctx context.Context, subject ports.ReviewDecisionPolicyApplicabilitySubjectV2, expected ports.ReviewDecisionPolicyCurrentProjectionRefV2, baseline time.Time) (ports.ReviewDecisionPolicyCurrentProjectionV2, error) {
	if err := subject.Validate(); err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, err
	}
	if err := expected.Validate(); err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, err
	}
	s1, err := g.policies.InspectReviewPolicy(ctx, subject.Policy.Ref)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, err
	}
	p1, err := g.facts.InspectCurrentPolicyV2(ctx, subject, expected)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, err
	}
	if !reflect.DeepEqual(s1, p1.Fact) {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, reviewDecisionPolicyDriftV2("Policy S1 drifted from projection")
	}
	s2, err := g.policies.InspectReviewPolicy(ctx, subject.Policy.Ref)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, err
	}
	checkpoint, err := g.nowV2(baseline)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, err
	}
	if !reflect.DeepEqual(s1, s2) {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, reviewDecisionPolicyDriftV2("Policy source changed across S1/S2")
	}
	p2, err := g.facts.InspectCurrentPolicyV2(ctx, subject, expected)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, err
	}
	if !reflect.DeepEqual(p1, p2) {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, reviewDecisionPolicyDriftV2("Policy current projection changed across S1/S2")
	}
	final, err := g.nowV2(checkpoint)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, err
	}
	if err := p1.ValidateCurrent(expected, subject, final); err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, err
	}
	return p1.Clone(), nil
}

func (g *ReviewDecisionPolicyCurrentGatewayV2) InspectHistoricalReviewDecisionPolicyV2(ctx context.Context, ref ports.ReviewDecisionPolicyCurrentProjectionRefV2) (ports.ReviewDecisionPolicyCurrentProjectionV2, error) {
	return g.facts.InspectHistoricalPolicyV2(ctx, ref)
}

func (g *ReviewDecisionPolicyCurrentGatewayV2) PublishReviewDecisionPolicyCurrentV2(ctx context.Context, request ports.ReviewDecisionPolicyCurrentPublishRequestV2) (ports.ReviewDecisionPolicyCurrentPublishReceiptV2, error) {
	baseline, err := g.nowV2(time.Time{})
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, err
	}
	s1, err := g.policies.InspectReviewPolicy(ctx, request.Value.Subject.Policy.Ref)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, err
	}
	if !reflect.DeepEqual(s1, request.Value.Fact) {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, reviewDecisionPolicyDriftV2("Policy source drifted from publish value")
	}
	s2, err := g.policies.InspectReviewPolicy(ctx, request.Value.Subject.Policy.Ref)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, err
	}
	now, err := g.nowV2(baseline)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, err
	}
	if !reflect.DeepEqual(s1, s2) {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, reviewDecisionPolicyDriftV2("Policy source changed across publish S1/S2")
	}
	if request.Value.Current {
		if err := request.Value.ValidateCurrent(request.Value.Ref, request.Value.Subject, now); err != nil {
			return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, err
		}
	} else if now.UnixNano() < request.Value.CheckedUnixNano {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review decision policy V2 checked time is in the future")
	}
	receipt, err := g.facts.CommitPolicyV2(ctx, request)
	if err == nil {
		return receipt, nil
	}
	if !core.HasCategory(err, core.ErrorIndeterminate) {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, err
	}
	stored, inspectErr := g.facts.InspectHistoricalPolicyV2(context.WithoutCancel(ctx), request.Value.Ref)
	if inspectErr != nil || !reflect.DeepEqual(stored, request.Value) {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, err
	}
	final, clockErr := g.nowV2(now)
	if clockErr != nil {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, clockErr
	}
	if request.Value.Current {
		if currentErr := request.Value.ValidateCurrent(request.Value.Ref, request.Value.Subject, final); currentErr != nil {
			return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, currentErr
		}
	}
	return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{Ref: request.Value.Ref, Created: false}, nil
}

func (g *ReviewDecisionPolicyCurrentGatewayV2) nowV2(baseline time.Time) (time.Time, error) {
	now := g.clock()
	if now.IsZero() || (!baseline.IsZero() && now.Before(baseline)) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review decision policy V2 clock regressed")
	}
	return now, nil
}
func reviewDecisionPolicyDriftV2(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, message)
}
func reviewDecisionPolicyNilV2(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}
	return false
}

var _ ports.ReviewDecisionPolicyCurrentReaderV2 = (*ReviewDecisionPolicyCurrentGatewayV2)(nil)
var _ ports.ReviewDecisionPolicyCurrentPublisherV2 = (*ReviewDecisionPolicyCurrentGatewayV2)(nil)

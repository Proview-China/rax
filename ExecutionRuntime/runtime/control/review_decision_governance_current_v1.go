package control

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ReviewDecisionSubjectProofReaderV1 is the only accepted bridge to the Review
// Owner's exact Target and Assignment facts. Governance never trusts nominal
// values supplied by a publisher or reconstructs Review facts.
type ReviewDecisionSubjectProofReaderV1 interface {
	InspectReviewDecisionTargetProofV1(context.Context, ports.ReviewDecisionTargetRefV1) (ports.ReviewDecisionTargetRefV1, error)
	InspectReviewDecisionAssignmentProofV1(context.Context, ports.ReviewDecisionAssignmentRefV1) (ports.ReviewDecisionAssignmentRefV1, error)
}

// ReviewDecisionGovernanceCurrentFactPortV1 is the Governance Owner's narrow
// atomic journal. Runtime storage/sqlite provides the single-node durable
// implementation; the interface itself makes no HA, topology or SLA claim.
type ReviewDecisionGovernanceCurrentFactPortV1 interface {
	ResolvePolicyV1(context.Context, ports.ReviewDecisionPolicyCurrentSubjectV1) (ports.ReviewDecisionPolicyCurrentProjectionRefV1, error)
	InspectCurrentPolicyV1(context.Context, ports.ReviewDecisionPolicyCurrentSubjectV1, ports.ReviewDecisionPolicyCurrentProjectionRefV1) (ports.ReviewDecisionPolicyCurrentProjectionV1, error)
	InspectHistoricalPolicyV1(context.Context, ports.ReviewDecisionPolicyCurrentProjectionRefV1) (ports.ReviewDecisionPolicyCurrentProjectionV1, error)
	CommitPolicyV1(context.Context, ports.ReviewDecisionPolicyCurrentPublishRequestV1) (ports.ReviewDecisionPolicyCurrentPublishReceiptV1, error)

	ResolveAuthorityV1(context.Context, ports.ReviewDecisionAuthorityCurrentSubjectV1) (ports.ReviewDecisionAuthorityCurrentProjectionRefV1, error)
	InspectCurrentAuthorityV1(context.Context, ports.ReviewDecisionAuthorityCurrentSubjectV1, ports.ReviewDecisionAuthorityCurrentProjectionRefV1) (ports.ReviewDecisionAuthorityCurrentProjectionV1, error)
	InspectHistoricalAuthorityV1(context.Context, ports.ReviewDecisionAuthorityCurrentProjectionRefV1) (ports.ReviewDecisionAuthorityCurrentProjectionV1, error)
	CommitAuthorityV1(context.Context, ports.ReviewDecisionAuthorityCurrentPublishRequestV1) (ports.ReviewDecisionAuthorityCurrentPublishReceiptV1, error)

	ResolveScopeV1(context.Context, ports.ReviewDecisionScopeCurrentSubjectV1) (ports.ReviewDecisionScopeCurrentProjectionRefV1, error)
	InspectCurrentScopeV1(context.Context, ports.ReviewDecisionScopeCurrentSubjectV1, ports.ReviewDecisionScopeCurrentProjectionRefV1) (ports.ReviewDecisionScopeCurrentProjectionV1, error)
	InspectHistoricalScopeV1(context.Context, ports.ReviewDecisionScopeCurrentProjectionRefV1) (ports.ReviewDecisionScopeCurrentProjectionV1, error)
	CommitScopeV1(context.Context, ports.ReviewDecisionScopeCurrentPublishRequestV1) (ports.ReviewDecisionScopeCurrentPublishReceiptV1, error)
}

// ReviewDecisionGovernanceCurrentGatewayV1 is an Owner-local reference
// projector. Every current read and publish performs proof/source S1 and S2
// around the journal operation. It dispatches nothing; production persistence
// may be injected from storage/sqlite, while composition remains host-owned.
type ReviewDecisionGovernanceCurrentGatewayV1 struct {
	facts     ReviewDecisionGovernanceCurrentFactPortV1
	proofs    ReviewDecisionSubjectProofReaderV1
	policies  ports.ReviewPolicyFactReaderV2
	authority ports.AuthorityFactReaderV2
	scopes    ports.ExecutionScopeFactReaderV2
	clock     func() time.Time
}

func NewReviewDecisionGovernanceCurrentGatewayV1(facts ReviewDecisionGovernanceCurrentFactPortV1, proofs ReviewDecisionSubjectProofReaderV1, policies ports.ReviewPolicyFactReaderV2, authority ports.AuthorityFactReaderV2, scopes ports.ExecutionScopeFactReaderV2, clock func() time.Time) (*ReviewDecisionGovernanceCurrentGatewayV1, error) {
	values := []any{facts, proofs, policies, authority, scopes, clock}
	for _, value := range values {
		if reviewDecisionGovernanceNilV1(value) {
			return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review decision Governance current dependency is missing")
		}
	}
	if clock().IsZero() {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review decision Governance current clock returned zero")
	}
	return &ReviewDecisionGovernanceCurrentGatewayV1{facts: facts, proofs: proofs, policies: policies, authority: authority, scopes: scopes, clock: clock}, nil
}

func (g *ReviewDecisionGovernanceCurrentGatewayV1) ResolveCurrentReviewDecisionPolicyV1(ctx context.Context, request ports.ReviewDecisionPolicyCurrentResolveRequestV1) (ports.ReviewDecisionPolicyCurrentProjectionRefV1, error) {
	if err := request.Subject.Validate(); err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV1{}, err
	}
	if err := g.proveTargetV1(ctx, request.Subject.Target); err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV1{}, err
	}
	return g.facts.ResolvePolicyV1(ctx, request.Subject)
}

func (g *ReviewDecisionGovernanceCurrentGatewayV1) InspectCurrentReviewDecisionPolicyV1(ctx context.Context, subject ports.ReviewDecisionPolicyCurrentSubjectV1, expected ports.ReviewDecisionPolicyCurrentProjectionRefV1) (ports.ReviewDecisionPolicyCurrentProjectionV1, error) {
	baseline, err := g.baselineV1()
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, err
	}
	if err := g.proveTargetV1(ctx, subject.Target); err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, err
	}
	s1, err := g.policies.InspectReviewPolicy(ctx, subject.Policy.Ref)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, err
	}
	p, err := g.facts.InspectCurrentPolicyV1(ctx, subject, expected)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, err
	}
	if !reflect.DeepEqual(s1, p.Fact) {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, reviewDecisionGovernanceDriftV1("Policy S1 drifted from projection")
	}
	if err := g.proveTargetV1(ctx, subject.Target); err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, err
	}
	s2, err := g.policies.InspectReviewPolicy(ctx, subject.Policy.Ref)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, err
	}
	now, err := g.freshV1(baseline)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, err
	}
	if !reflect.DeepEqual(s1, s2) {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, reviewDecisionGovernanceDriftV1("Policy source changed across S1/S2")
	}
	p2, err := g.facts.InspectCurrentPolicyV1(ctx, subject, expected)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, err
	}
	if !reflect.DeepEqual(p, p2) {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, reviewDecisionGovernanceDriftV1("Policy current index changed across S1/S2")
	}
	final, err := g.freshV1(now)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, err
	}
	if err := p.ValidateCurrent(expected, subject, final); err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, err
	}
	return p, nil
}

func (g *ReviewDecisionGovernanceCurrentGatewayV1) InspectHistoricalReviewDecisionPolicyV1(ctx context.Context, ref ports.ReviewDecisionPolicyCurrentProjectionRefV1) (ports.ReviewDecisionPolicyCurrentProjectionV1, error) {
	return g.facts.InspectHistoricalPolicyV1(ctx, ref)
}

func (g *ReviewDecisionGovernanceCurrentGatewayV1) PublishReviewDecisionPolicyCurrentV1(ctx context.Context, request ports.ReviewDecisionPolicyCurrentPublishRequestV1) (ports.ReviewDecisionPolicyCurrentPublishReceiptV1, error) {
	baseline, err := g.baselineV1()
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV1{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV1{}, err
	}
	if err := g.proveTargetV1(ctx, request.Value.Subject.Target); err != nil {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV1{}, err
	}
	s1, err := g.policies.InspectReviewPolicy(ctx, request.Value.Subject.Policy.Ref)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV1{}, err
	}
	if !reflect.DeepEqual(s1, request.Value.Fact) {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV1{}, reviewDecisionGovernanceDriftV1("Policy source drifted from publish value")
	}
	if err := g.proveTargetV1(ctx, request.Value.Subject.Target); err != nil {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV1{}, err
	}
	s2, err := g.policies.InspectReviewPolicy(ctx, request.Value.Subject.Policy.Ref)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV1{}, err
	}
	now, err := g.freshV1(baseline)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV1{}, err
	}
	if !reflect.DeepEqual(s1, s2) {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV1{}, reviewDecisionGovernanceDriftV1("Policy source changed across S1/S2")
	}
	if now.UnixNano() < request.Value.CheckedUnixNano {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Policy projection Checked time is in the future")
	}
	if request.Value.Current {
		if err := request.Value.ValidateCurrent(request.Value.Ref, request.Value.Subject, now); err != nil {
			return ports.ReviewDecisionPolicyCurrentPublishReceiptV1{}, err
		}
	}
	return g.commitPolicyV1(ctx, request)
}

func (g *ReviewDecisionGovernanceCurrentGatewayV1) ResolveCurrentReviewDecisionAuthorityV1(ctx context.Context, request ports.ReviewDecisionAuthorityCurrentResolveRequestV1) (ports.ReviewDecisionAuthorityCurrentProjectionRefV1, error) {
	if err := request.Subject.Validate(); err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionRefV1{}, err
	}
	if err := g.proveAuthoritySubjectV1(ctx, request.Subject); err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionRefV1{}, err
	}
	return g.facts.ResolveAuthorityV1(ctx, request.Subject)
}

func (g *ReviewDecisionGovernanceCurrentGatewayV1) InspectCurrentReviewDecisionAuthorityV1(ctx context.Context, subject ports.ReviewDecisionAuthorityCurrentSubjectV1, expected ports.ReviewDecisionAuthorityCurrentProjectionRefV1) (ports.ReviewDecisionAuthorityCurrentProjectionV1, error) {
	baseline, err := g.baselineV1()
	if err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, err
	}
	if err := g.proveAuthoritySubjectV1(ctx, subject); err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, err
	}
	s1, err := g.authority.InspectDispatchAuthority(ctx, subject.Authority.Ref)
	if err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, err
	}
	p, err := g.facts.InspectCurrentAuthorityV1(ctx, subject, expected)
	if err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, err
	}
	if !reflect.DeepEqual(s1, p.Fact) {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, reviewDecisionGovernanceDriftV1("Authority S1 drifted from projection")
	}
	if err := g.proveAuthoritySubjectV1(ctx, subject); err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, err
	}
	s2, err := g.authority.InspectDispatchAuthority(ctx, subject.Authority.Ref)
	if err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, err
	}
	now, err := g.freshV1(baseline)
	if err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, err
	}
	if !reflect.DeepEqual(s1, s2) {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, reviewDecisionGovernanceDriftV1("Authority source changed across S1/S2")
	}
	p2, err := g.facts.InspectCurrentAuthorityV1(ctx, subject, expected)
	if err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, err
	}
	if !reflect.DeepEqual(p, p2) {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, reviewDecisionGovernanceDriftV1("Authority current index changed across S1/S2")
	}
	final, err := g.freshV1(now)
	if err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, err
	}
	if err := p.ValidateCurrent(expected, subject, final); err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, err
	}
	return p, nil
}

func (g *ReviewDecisionGovernanceCurrentGatewayV1) InspectHistoricalReviewDecisionAuthorityV1(ctx context.Context, ref ports.ReviewDecisionAuthorityCurrentProjectionRefV1) (ports.ReviewDecisionAuthorityCurrentProjectionV1, error) {
	return g.facts.InspectHistoricalAuthorityV1(ctx, ref)
}

func (g *ReviewDecisionGovernanceCurrentGatewayV1) PublishReviewDecisionAuthorityCurrentV1(ctx context.Context, request ports.ReviewDecisionAuthorityCurrentPublishRequestV1) (ports.ReviewDecisionAuthorityCurrentPublishReceiptV1, error) {
	baseline, err := g.baselineV1()
	if err != nil {
		return ports.ReviewDecisionAuthorityCurrentPublishReceiptV1{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.ReviewDecisionAuthorityCurrentPublishReceiptV1{}, err
	}
	if err := g.proveAuthoritySubjectV1(ctx, request.Value.Subject); err != nil {
		return ports.ReviewDecisionAuthorityCurrentPublishReceiptV1{}, err
	}
	s1, err := g.authority.InspectDispatchAuthority(ctx, request.Value.Subject.Authority.Ref)
	if err != nil {
		return ports.ReviewDecisionAuthorityCurrentPublishReceiptV1{}, err
	}
	if !reflect.DeepEqual(s1, request.Value.Fact) {
		return ports.ReviewDecisionAuthorityCurrentPublishReceiptV1{}, reviewDecisionGovernanceDriftV1("Authority source drifted from publish value")
	}
	if err := g.proveAuthoritySubjectV1(ctx, request.Value.Subject); err != nil {
		return ports.ReviewDecisionAuthorityCurrentPublishReceiptV1{}, err
	}
	s2, err := g.authority.InspectDispatchAuthority(ctx, request.Value.Subject.Authority.Ref)
	if err != nil {
		return ports.ReviewDecisionAuthorityCurrentPublishReceiptV1{}, err
	}
	now, err := g.freshV1(baseline)
	if err != nil {
		return ports.ReviewDecisionAuthorityCurrentPublishReceiptV1{}, err
	}
	if !reflect.DeepEqual(s1, s2) {
		return ports.ReviewDecisionAuthorityCurrentPublishReceiptV1{}, reviewDecisionGovernanceDriftV1("Authority source changed across S1/S2")
	}
	if now.UnixNano() < request.Value.CheckedUnixNano {
		return ports.ReviewDecisionAuthorityCurrentPublishReceiptV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Authority projection Checked time is in the future")
	}
	if request.Value.Current {
		if err := request.Value.ValidateCurrent(request.Value.Ref, request.Value.Subject, now); err != nil {
			return ports.ReviewDecisionAuthorityCurrentPublishReceiptV1{}, err
		}
	}
	return g.commitAuthorityV1(ctx, request)
}

func (g *ReviewDecisionGovernanceCurrentGatewayV1) ResolveCurrentReviewDecisionScopeV1(ctx context.Context, request ports.ReviewDecisionScopeCurrentResolveRequestV1) (ports.ReviewDecisionScopeCurrentProjectionRefV1, error) {
	if err := request.Subject.Validate(); err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionRefV1{}, err
	}
	if err := g.proveTargetV1(ctx, request.Subject.Target); err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionRefV1{}, err
	}
	return g.facts.ResolveScopeV1(ctx, request.Subject)
}

func (g *ReviewDecisionGovernanceCurrentGatewayV1) InspectCurrentReviewDecisionScopeV1(ctx context.Context, subject ports.ReviewDecisionScopeCurrentSubjectV1, expected ports.ReviewDecisionScopeCurrentProjectionRefV1) (ports.ReviewDecisionScopeCurrentProjectionV1, error) {
	baseline, err := g.baselineV1()
	if err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, err
	}
	if err := g.proveTargetV1(ctx, subject.Target); err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, err
	}
	s1, err := g.scopes.InspectCurrentExecutionScope(ctx, subject.CurrentScope.Ref)
	if err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, err
	}
	p, err := g.facts.InspectCurrentScopeV1(ctx, subject, expected)
	if err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, err
	}
	if !reflect.DeepEqual(s1, p.Fact) {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, reviewDecisionGovernanceDriftV1("Scope S1 drifted from projection")
	}
	if err := g.proveTargetV1(ctx, subject.Target); err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, err
	}
	s2, err := g.scopes.InspectCurrentExecutionScope(ctx, subject.CurrentScope.Ref)
	if err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, err
	}
	now, err := g.freshV1(baseline)
	if err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, err
	}
	if !reflect.DeepEqual(s1, s2) {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, reviewDecisionGovernanceDriftV1("Scope source changed across S1/S2")
	}
	p2, err := g.facts.InspectCurrentScopeV1(ctx, subject, expected)
	if err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, err
	}
	if !reflect.DeepEqual(p, p2) {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, reviewDecisionGovernanceDriftV1("Scope current index changed across S1/S2")
	}
	if !reflect.DeepEqual(subject, p.Subject) {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, reviewDecisionGovernanceDriftV1("Scope exact subject drifted")
	}
	// ExecutionScope carries optional pointer-valued leases. The public value
	// validator compares its own sealed subject; the Gateway has already made
	// the required semantic deep comparison against the caller's exact subject.
	final, err := g.freshV1(now)
	if err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, err
	}
	if err := p.ValidateCurrent(expected, p.Subject, final); err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, err
	}
	return p, nil
}

func (g *ReviewDecisionGovernanceCurrentGatewayV1) InspectHistoricalReviewDecisionScopeV1(ctx context.Context, ref ports.ReviewDecisionScopeCurrentProjectionRefV1) (ports.ReviewDecisionScopeCurrentProjectionV1, error) {
	return g.facts.InspectHistoricalScopeV1(ctx, ref)
}

func (g *ReviewDecisionGovernanceCurrentGatewayV1) PublishReviewDecisionScopeCurrentV1(ctx context.Context, request ports.ReviewDecisionScopeCurrentPublishRequestV1) (ports.ReviewDecisionScopeCurrentPublishReceiptV1, error) {
	baseline, err := g.baselineV1()
	if err != nil {
		return ports.ReviewDecisionScopeCurrentPublishReceiptV1{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.ReviewDecisionScopeCurrentPublishReceiptV1{}, err
	}
	if err := g.proveTargetV1(ctx, request.Value.Subject.Target); err != nil {
		return ports.ReviewDecisionScopeCurrentPublishReceiptV1{}, err
	}
	s1, err := g.scopes.InspectCurrentExecutionScope(ctx, request.Value.Subject.CurrentScope.Ref)
	if err != nil {
		return ports.ReviewDecisionScopeCurrentPublishReceiptV1{}, err
	}
	if !reflect.DeepEqual(s1, request.Value.Fact) {
		return ports.ReviewDecisionScopeCurrentPublishReceiptV1{}, reviewDecisionGovernanceDriftV1("Scope source drifted from publish value")
	}
	if err := g.proveTargetV1(ctx, request.Value.Subject.Target); err != nil {
		return ports.ReviewDecisionScopeCurrentPublishReceiptV1{}, err
	}
	s2, err := g.scopes.InspectCurrentExecutionScope(ctx, request.Value.Subject.CurrentScope.Ref)
	if err != nil {
		return ports.ReviewDecisionScopeCurrentPublishReceiptV1{}, err
	}
	now, err := g.freshV1(baseline)
	if err != nil {
		return ports.ReviewDecisionScopeCurrentPublishReceiptV1{}, err
	}
	if !reflect.DeepEqual(s1, s2) {
		return ports.ReviewDecisionScopeCurrentPublishReceiptV1{}, reviewDecisionGovernanceDriftV1("Scope source changed across S1/S2")
	}
	if now.UnixNano() < request.Value.CheckedUnixNano {
		return ports.ReviewDecisionScopeCurrentPublishReceiptV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Scope projection Checked time is in the future")
	}
	if request.Value.Current {
		if err := request.Value.ValidateCurrent(request.Value.Ref, request.Value.Subject, now); err != nil {
			return ports.ReviewDecisionScopeCurrentPublishReceiptV1{}, err
		}
	}
	return g.commitScopeV1(ctx, request)
}

func (g *ReviewDecisionGovernanceCurrentGatewayV1) proveTargetV1(ctx context.Context, expected ports.ReviewDecisionTargetRefV1) error {
	actual, err := g.proofs.InspectReviewDecisionTargetProofV1(ctx, expected)
	if err != nil {
		return err
	}
	if actual != expected {
		return reviewDecisionGovernanceDriftV1("Review Target proof drifted")
	}
	return nil
}
func (g *ReviewDecisionGovernanceCurrentGatewayV1) proveAuthoritySubjectV1(ctx context.Context, subject ports.ReviewDecisionAuthorityCurrentSubjectV1) error {
	if err := g.proveTargetV1(ctx, subject.Target); err != nil {
		return err
	}
	actual, err := g.proofs.InspectReviewDecisionAssignmentProofV1(ctx, subject.Assignment)
	if err != nil {
		return err
	}
	if actual != subject.Assignment {
		return reviewDecisionGovernanceDriftV1("Review Assignment proof drifted")
	}
	return nil
}
func (g *ReviewDecisionGovernanceCurrentGatewayV1) baselineV1() (time.Time, error) {
	now := g.clock()
	if now.IsZero() {
		return time.Time{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review decision Governance current clock returned zero")
	}
	return now, nil
}
func (g *ReviewDecisionGovernanceCurrentGatewayV1) freshV1(baseline time.Time) (time.Time, error) {
	now := g.clock()
	if now.IsZero() || now.Before(baseline) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review decision Governance current clock regressed")
	}
	return now, nil
}

func (g *ReviewDecisionGovernanceCurrentGatewayV1) commitPolicyV1(ctx context.Context, request ports.ReviewDecisionPolicyCurrentPublishRequestV1) (ports.ReviewDecisionPolicyCurrentPublishReceiptV1, error) {
	receipt, err := g.facts.CommitPolicyV1(ctx, request)
	if err == nil {
		return receipt, nil
	}
	if !core.HasCategory(err, core.ErrorIndeterminate) {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV1{}, err
	}
	stored, inspectErr := g.facts.InspectHistoricalPolicyV1(context.WithoutCancel(ctx), request.Value.Ref)
	if inspectErr != nil || !reflect.DeepEqual(stored, request.Value) {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV1{}, err
	}
	return ports.ReviewDecisionPolicyCurrentPublishReceiptV1{Ref: request.Value.Ref, Created: false}, nil
}
func (g *ReviewDecisionGovernanceCurrentGatewayV1) commitAuthorityV1(ctx context.Context, request ports.ReviewDecisionAuthorityCurrentPublishRequestV1) (ports.ReviewDecisionAuthorityCurrentPublishReceiptV1, error) {
	receipt, err := g.facts.CommitAuthorityV1(ctx, request)
	if err == nil {
		return receipt, nil
	}
	if !core.HasCategory(err, core.ErrorIndeterminate) {
		return ports.ReviewDecisionAuthorityCurrentPublishReceiptV1{}, err
	}
	stored, inspectErr := g.facts.InspectHistoricalAuthorityV1(context.WithoutCancel(ctx), request.Value.Ref)
	if inspectErr != nil || !reflect.DeepEqual(stored, request.Value) {
		return ports.ReviewDecisionAuthorityCurrentPublishReceiptV1{}, err
	}
	return ports.ReviewDecisionAuthorityCurrentPublishReceiptV1{Ref: request.Value.Ref, Created: false}, nil
}
func (g *ReviewDecisionGovernanceCurrentGatewayV1) commitScopeV1(ctx context.Context, request ports.ReviewDecisionScopeCurrentPublishRequestV1) (ports.ReviewDecisionScopeCurrentPublishReceiptV1, error) {
	receipt, err := g.facts.CommitScopeV1(ctx, request)
	if err == nil {
		return receipt, nil
	}
	if !core.HasCategory(err, core.ErrorIndeterminate) {
		return ports.ReviewDecisionScopeCurrentPublishReceiptV1{}, err
	}
	stored, inspectErr := g.facts.InspectHistoricalScopeV1(context.WithoutCancel(ctx), request.Value.Ref)
	if inspectErr != nil || !reflect.DeepEqual(stored, request.Value) {
		return ports.ReviewDecisionScopeCurrentPublishReceiptV1{}, err
	}
	return ports.ReviewDecisionScopeCurrentPublishReceiptV1{Ref: request.Value.Ref, Created: false}, nil
}

func reviewDecisionGovernanceDriftV1(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, message)
}
func reviewDecisionGovernanceNilV1(value any) bool {
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

var _ ports.ReviewDecisionPolicyCurrentReaderV1 = (*ReviewDecisionGovernanceCurrentGatewayV1)(nil)
var _ ports.ReviewDecisionPolicyCurrentPublisherV1 = (*ReviewDecisionGovernanceCurrentGatewayV1)(nil)
var _ ports.ReviewDecisionAuthorityCurrentReaderV1 = (*ReviewDecisionGovernanceCurrentGatewayV1)(nil)
var _ ports.ReviewDecisionAuthorityCurrentPublisherV1 = (*ReviewDecisionGovernanceCurrentGatewayV1)(nil)
var _ ports.ReviewDecisionScopeCurrentReaderV1 = (*ReviewDecisionGovernanceCurrentGatewayV1)(nil)
var _ ports.ReviewDecisionScopeCurrentPublisherV1 = (*ReviewDecisionGovernanceCurrentGatewayV1)(nil)

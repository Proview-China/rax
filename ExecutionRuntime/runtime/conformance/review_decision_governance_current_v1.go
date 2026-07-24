package conformance

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type ReviewDecisionGovernanceCurrentCaseV1 struct {
	PolicyReader       ports.ReviewDecisionPolicyCurrentReaderV1
	PolicyPublisher    ports.ReviewDecisionPolicyCurrentPublisherV1
	Policy             ports.ReviewDecisionPolicyCurrentPublishRequestV1
	AuthorityReader    ports.ReviewDecisionAuthorityCurrentReaderV1
	AuthorityPublisher ports.ReviewDecisionAuthorityCurrentPublisherV1
	Authority          ports.ReviewDecisionAuthorityCurrentPublishRequestV1
	ScopeReader        ports.ReviewDecisionScopeCurrentReaderV1
	ScopePublisher     ports.ReviewDecisionScopeCurrentPublisherV1
	Scope              ports.ReviewDecisionScopeCurrentPublishRequestV1
}

type ReviewDecisionGovernanceCurrentReportV1 struct {
	PolicyExact        bool `json:"policy_exact"`
	AuthorityExact     bool `json:"authority_exact"`
	ScopeExact         bool `json:"scope_exact"`
	HistoricalExact    bool `json:"historical_exact"`
	ProductionEligible bool `json:"production_eligible"`
}

// RunReviewDecisionGovernanceCurrentV1 proves only public reference behavior.
// Passing never certifies durability, a State Plane, production composition or
// availability. Callers must use isolated fixtures without external effects.
func RunReviewDecisionGovernanceCurrentV1(ctx context.Context, test ReviewDecisionGovernanceCurrentCaseV1) (ReviewDecisionGovernanceCurrentReportV1, error) {
	if reviewDecisionConformanceNilV1(test.PolicyReader) || reviewDecisionConformanceNilV1(test.PolicyPublisher) || reviewDecisionConformanceNilV1(test.AuthorityReader) || reviewDecisionConformanceNilV1(test.AuthorityPublisher) || reviewDecisionConformanceNilV1(test.ScopeReader) || reviewDecisionConformanceNilV1(test.ScopePublisher) {
		return ReviewDecisionGovernanceCurrentReportV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review decision Governance conformance dependencies are missing")
	}
	if _, err := test.PolicyPublisher.PublishReviewDecisionPolicyCurrentV1(ctx, test.Policy); err != nil {
		return ReviewDecisionGovernanceCurrentReportV1{}, err
	}
	policyRef, err := test.PolicyReader.ResolveCurrentReviewDecisionPolicyV1(ctx, ports.ReviewDecisionPolicyCurrentResolveRequestV1{Subject: test.Policy.Value.Subject})
	if err != nil {
		return ReviewDecisionGovernanceCurrentReportV1{}, err
	}
	policy, err := test.PolicyReader.InspectCurrentReviewDecisionPolicyV1(ctx, test.Policy.Value.Subject, policyRef)
	if err != nil {
		return ReviewDecisionGovernanceCurrentReportV1{}, err
	}
	if !reflect.DeepEqual(policy, test.Policy.Value) {
		return ReviewDecisionGovernanceCurrentReportV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Policy conformance exact read drifted")
	}
	policyHistory, err := test.PolicyReader.InspectHistoricalReviewDecisionPolicyV1(ctx, policyRef)
	if err != nil {
		return ReviewDecisionGovernanceCurrentReportV1{}, err
	}
	if !reflect.DeepEqual(policyHistory, policy) {
		return ReviewDecisionGovernanceCurrentReportV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Policy conformance history drifted")
	}

	if _, err := test.AuthorityPublisher.PublishReviewDecisionAuthorityCurrentV1(ctx, test.Authority); err != nil {
		return ReviewDecisionGovernanceCurrentReportV1{}, err
	}
	authorityRef, err := test.AuthorityReader.ResolveCurrentReviewDecisionAuthorityV1(ctx, ports.ReviewDecisionAuthorityCurrentResolveRequestV1{Subject: test.Authority.Value.Subject})
	if err != nil {
		return ReviewDecisionGovernanceCurrentReportV1{}, err
	}
	authority, err := test.AuthorityReader.InspectCurrentReviewDecisionAuthorityV1(ctx, test.Authority.Value.Subject, authorityRef)
	if err != nil {
		return ReviewDecisionGovernanceCurrentReportV1{}, err
	}
	if !reflect.DeepEqual(authority, test.Authority.Value) {
		return ReviewDecisionGovernanceCurrentReportV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Authority conformance exact read drifted")
	}
	authorityHistory, err := test.AuthorityReader.InspectHistoricalReviewDecisionAuthorityV1(ctx, authorityRef)
	if err != nil {
		return ReviewDecisionGovernanceCurrentReportV1{}, err
	}
	if !reflect.DeepEqual(authorityHistory, authority) {
		return ReviewDecisionGovernanceCurrentReportV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Authority conformance history drifted")
	}

	if _, err := test.ScopePublisher.PublishReviewDecisionScopeCurrentV1(ctx, test.Scope); err != nil {
		return ReviewDecisionGovernanceCurrentReportV1{}, err
	}
	scopeRef, err := test.ScopeReader.ResolveCurrentReviewDecisionScopeV1(ctx, ports.ReviewDecisionScopeCurrentResolveRequestV1{Subject: test.Scope.Value.Subject})
	if err != nil {
		return ReviewDecisionGovernanceCurrentReportV1{}, err
	}
	scope, err := test.ScopeReader.InspectCurrentReviewDecisionScopeV1(ctx, test.Scope.Value.Subject, scopeRef)
	if err != nil {
		return ReviewDecisionGovernanceCurrentReportV1{}, err
	}
	if !reflect.DeepEqual(scope, test.Scope.Value) {
		return ReviewDecisionGovernanceCurrentReportV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Scope conformance exact read drifted")
	}
	scopeHistory, err := test.ScopeReader.InspectHistoricalReviewDecisionScopeV1(ctx, scopeRef)
	if err != nil {
		return ReviewDecisionGovernanceCurrentReportV1{}, err
	}
	if !reflect.DeepEqual(scopeHistory, scope) {
		return ReviewDecisionGovernanceCurrentReportV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Scope conformance history drifted")
	}

	return ReviewDecisionGovernanceCurrentReportV1{PolicyExact: true, AuthorityExact: true, ScopeExact: true, HistoricalExact: true, ProductionEligible: false}, nil
}

func reviewDecisionConformanceNilV1(value any) bool {
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

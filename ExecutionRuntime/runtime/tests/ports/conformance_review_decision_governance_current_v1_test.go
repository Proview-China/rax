package ports_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/tests/testsupport"
)

func TestReviewDecisionGovernanceCurrentV1ConformanceIsReferenceOnly(t *testing.T) {
	t.Parallel()
	fixture := testsupport.ReviewDecisionGovernanceFixture()
	store := fakes.NewReviewDecisionGovernanceCurrentStoreV1()
	source := fakes.NewReviewDecisionGovernanceSourceStoreV1()
	testsupport.SeedReviewDecisionGovernanceSourcesV1(source, fixture)
	gateway, err := control.NewReviewDecisionGovernanceCurrentGatewayV1(store, source, source, source, source, func() time.Time { return fixture.Now.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	report, err := conformance.RunReviewDecisionGovernanceCurrentV1(context.Background(), conformance.ReviewDecisionGovernanceCurrentCaseV1{
		PolicyReader: gateway, PolicyPublisher: gateway, Policy: ports.ReviewDecisionPolicyCurrentPublishRequestV1{Value: fixture.Policy},
		AuthorityReader: gateway, AuthorityPublisher: gateway, Authority: ports.ReviewDecisionAuthorityCurrentPublishRequestV1{Value: fixture.Authority},
		ScopeReader: gateway, ScopePublisher: gateway, Scope: ports.ReviewDecisionScopeCurrentPublishRequestV1{Value: fixture.Scope},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.PolicyExact || !report.AuthorityExact || !report.ScopeExact || !report.HistoricalExact || report.ProductionEligible {
		t.Fatalf("unexpected conformance report: %+v", report)
	}
}

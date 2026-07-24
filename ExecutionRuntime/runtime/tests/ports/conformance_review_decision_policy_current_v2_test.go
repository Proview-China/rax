package ports_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type conformanceReviewPolicySourceV2 struct{ fact ports.ReviewPolicyFactV2 }

func (s conformanceReviewPolicySourceV2) InspectReviewPolicy(context.Context, string) (ports.ReviewPolicyFactV2, error) {
	return s.fact, nil
}

type conformancePolicyPublisherV2 struct {
	gateway *control.ReviewDecisionPolicyCurrentGatewayV2
	source  *conformanceReviewPolicySourceV2
}

func (p conformancePolicyPublisherV2) PublishReviewDecisionPolicyCurrentV2(ctx context.Context, request ports.ReviewDecisionPolicyCurrentPublishRequestV2) (ports.ReviewDecisionPolicyCurrentPublishReceiptV2, error) {
	p.source.fact = request.Value.Fact
	return p.gateway.PublishReviewDecisionPolicyCurrentV2(ctx, request)
}

func TestConformanceReviewDecisionPolicyCurrentV2(t *testing.T) {
	now := time.Unix(2_730_000_000, 0)
	value := reviewDecisionPolicyProjectionV2(t, now, 1)
	facts := fakes.NewReviewDecisionPolicyCurrentStoreV2()
	source := &conformanceReviewPolicySourceV2{fact: value.Fact}
	gateway, err := control.NewReviewDecisionPolicyCurrentGatewayV2(facts, source, func() time.Time { return now.Add(2 * time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	next := value.Clone()
	next.Ref.Revision = 2
	next.Subject.Policy.Revision = 2
	next.Fact = reviewPolicyFactV2(t, next.Subject, now.Add(time.Second), true)
	next.Subject.Policy.Digest = next.Fact.Digest
	next.CheckedUnixNano = now.Add(time.Second).UnixNano()
	next.ExpiresUnixNano = now.Add(31 * time.Second).UnixNano()
	next, err = ports.SealReviewDecisionPolicyCurrentProjectionV2(next)
	if err != nil {
		t.Fatal(err)
	}
	requestNext := ports.ReviewDecisionPolicyCurrentPublishRequestV2{Previous: &value.Ref, Value: next}
	report, err := conformance.RunReviewDecisionPolicyCurrentV2(context.Background(), conformance.ReviewDecisionPolicyCurrentCaseV2{Reader: gateway, Publisher: conformancePolicyPublisherV2{gateway: gateway, source: source}, Initial: ports.ReviewDecisionPolicyCurrentPublishRequestV2{Value: value}, Next: &requestNext})
	if err != nil {
		t.Fatal(err)
	}
	if !report.ExactCurrent || !report.AppendOnlyHistory || !report.FullRefCAS || report.ProductionEligible {
		t.Fatalf("unexpected report: %+v", report)
	}
}

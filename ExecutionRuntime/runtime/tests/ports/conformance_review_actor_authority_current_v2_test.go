package ports_test

import (
	"context"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"testing"
	"time"
)

type actorConformanceTargetProofV2 struct {
	target ports.ReviewDecisionTargetRefV1
}

func (p actorConformanceTargetProofV2) InspectReviewDecisionTargetProofV1(context.Context, ports.ReviewDecisionTargetRefV1) (ports.ReviewDecisionTargetRefV1, error) {
	return p.target, nil
}
func (p actorConformanceTargetProofV2) InspectReviewDecisionAssignmentProofV1(context.Context, ports.ReviewDecisionAssignmentRefV1) (ports.ReviewDecisionAssignmentRefV1, error) {
	panic("actor conformance must never inspect Assignment")
}

type actorConformancePublisherV2 struct {
	actor     *control.ReviewActorAuthorityCurrentGatewayV2
	authority *control.DispatchAuthorityCurrentGatewayV3
	last      *ports.AuthorityBindingRefV2
}

func (p *actorConformancePublisherV2) PublishReviewActorAuthorityCurrentV2(ctx context.Context, request ports.ReviewActorAuthorityCurrentPublishRequestV2) (ports.ReviewActorAuthorityCurrentPublishReceiptV2, error) {
	if p.last == nil || *p.last != request.Value.Fact.Ref {
		_, err := p.authority.PublishDispatchAuthorityFactV3(ctx, ports.DispatchAuthorityFactPublishRequestV3{Previous: p.last, Value: request.Value.Fact})
		if err != nil {
			return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, err
		}
		ref := request.Value.Fact.Ref
		p.last = &ref
	}
	return p.actor.PublishReviewActorAuthorityCurrentV2(ctx, request)
}
func TestConformanceDispatchAuthorityCurrentV3(t *testing.T) {
	now := time.Unix(2_840_000_000, 0)
	first := dispatchAuthorityFactV3(t, now, 1, 1, "run-a", true)
	next := nextDispatchAuthorityFactV3(t, first, now.Add(time.Second), true)
	store := fakes.NewDispatchAuthorityCurrentStoreV3()
	gateway, err := control.NewDispatchAuthorityCurrentGatewayV3(store, func() time.Time { return now.Add(2 * time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	requestNext := ports.DispatchAuthorityFactPublishRequestV3{Previous: &first.Ref, Value: next}
	report, err := conformance.RunDispatchAuthorityCurrentV3(context.Background(), conformance.DispatchAuthorityCurrentCaseV3{Reader: gateway, Publisher: gateway, Initial: ports.DispatchAuthorityFactPublishRequestV3{Value: first}, Next: &requestNext})
	if err != nil {
		t.Fatal(err)
	}
	if !report.ExactCurrent || !report.AppendOnlyHistory || !report.FullRefCAS || report.ProductionEligible {
		t.Fatalf("unexpected V3 report: %+v", report)
	}
}
func TestConformanceReviewActorAuthorityCurrentV2(t *testing.T) {
	now := time.Unix(2_840_000_000, 0)
	first := reviewActorAuthorityProjectionV2(t, now)
	next := first.Clone()
	next.Ref.Revision = 2
	next.Fact = nextDispatchAuthorityFactV3(t, first.Fact, now.Add(time.Second), true)
	next.Subject.ActorAuthority = next.Fact.Ref
	next.CheckedUnixNano = now.Add(time.Second).UnixNano()
	next.ExpiresUnixNano = now.Add(20 * time.Second).UnixNano()
	var err error
	next, err = ports.SealReviewActorAuthorityCurrentProjectionV2(next)
	if err != nil {
		t.Fatal(err)
	}
	authorityStore := fakes.NewDispatchAuthorityCurrentStoreV3()
	authorityGateway, _ := control.NewDispatchAuthorityCurrentGatewayV3(authorityStore, func() time.Time { return now.Add(2 * time.Second) })
	actorStore := fakes.NewReviewActorAuthorityCurrentStoreV2()
	actorGateway, _ := control.NewReviewActorAuthorityCurrentGatewayV2(actorStore, actorConformanceTargetProofV2{target: first.Subject.Target}, authorityGateway, func() time.Time { return now.Add(2 * time.Second) })
	publisher := &actorConformancePublisherV2{actor: actorGateway, authority: authorityGateway}
	requestNext := ports.ReviewActorAuthorityCurrentPublishRequestV2{Previous: &first.Ref, Value: next}
	report, err := conformance.RunReviewActorAuthorityCurrentV2(context.Background(), conformance.ReviewActorAuthorityCurrentCaseV2{Reader: actorGateway, Publisher: publisher, Initial: ports.ReviewActorAuthorityCurrentPublishRequestV2{Value: first}, Next: &requestNext})
	if err != nil {
		t.Fatal(err)
	}
	if !report.ExactCurrent || !report.AppendOnlyHistory || !report.FullRefCAS || !report.ActorOnly || report.ProductionEligible {
		t.Fatalf("unexpected actor report: %+v", report)
	}
}

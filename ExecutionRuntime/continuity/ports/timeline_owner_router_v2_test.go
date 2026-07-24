package ports_test

import (
	"context"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

func TestClosedTimelineTypedOwnerRouterV2ExactRouteAndConstructorCopy(t *testing.T) {
	request := ownerRouteRequestV2()
	reader := &ownerRouteReaderV2{}
	routes := []ports.TimelineTypedOwnerRouteV2{{
		OwnerComponentID: request.Fact.Owner.ComponentID,
		Capability:       request.Fact.Owner.Capability,
		FactKind:         request.Fact.FactKind,
		PayloadSchema:    request.Fact.PayloadSchema,
		Reader:           reader,
	}}
	router, err := ports.NewClosedTimelineTypedOwnerRouterV2(routes)
	if err != nil {
		t.Fatal(err)
	}
	routes[0].OwnerComponentID = "mutated/component"
	routes[0].Capability = "mutated/capability"
	routes[0].FactKind = "mutated/fact"
	routes[0].PayloadSchema = "mutated/schema"
	routes[0].Reader = nil
	got, err := router.ReaderForTimelineOwnerV2(request)
	if err != nil || got != reader {
		t.Fatalf("constructor input mutation changed closed route: got=%T err=%v", got, err)
	}
}

func TestClosedTimelineTypedOwnerRouterV2RejectsDuplicateTypedNilAndUnknown(t *testing.T) {
	request := ownerRouteRequestV2()
	route := ports.TimelineTypedOwnerRouteV2{
		OwnerComponentID: request.Fact.Owner.ComponentID,
		Capability:       request.Fact.Owner.Capability,
		FactKind:         request.Fact.FactKind,
		PayloadSchema:    request.Fact.PayloadSchema,
		Reader:           &ownerRouteReaderV2{},
	}
	if _, err := ports.NewClosedTimelineTypedOwnerRouterV2([]ports.TimelineTypedOwnerRouteV2{route, route}); !contract.HasCode(err, contract.ErrProjectionConflict) {
		t.Fatalf("duplicate exact route must conflict: %v", err)
	}
	var typedNil *ownerRouteReaderV2
	route.Reader = typedNil
	if _, err := ports.NewClosedTimelineTypedOwnerRouterV2([]ports.TimelineTypedOwnerRouteV2{route}); !contract.HasCode(err, contract.ErrInvalidArgument) {
		t.Fatalf("typed-nil route must fail closed: %v", err)
	}
	if _, err := ports.NewClosedTimelineTypedOwnerRouterV2(nil); !contract.HasCode(err, contract.ErrInvalidArgument) {
		t.Fatalf("empty closed router must fail closed: %v", err)
	}

	route.Reader = &ownerRouteReaderV2{}
	router, err := ports.NewClosedTimelineTypedOwnerRouterV2([]ports.TimelineTypedOwnerRouteV2{route})
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*ports.TimelineOwnerCurrentInspectRequestV2){
		func(v *ports.TimelineOwnerCurrentInspectRequestV2) { v.Fact.Owner.ComponentID = "unknown/component" },
		func(v *ports.TimelineOwnerCurrentInspectRequestV2) { v.Fact.Owner.Capability = "unknown/capability" },
		func(v *ports.TimelineOwnerCurrentInspectRequestV2) {
			v.Fact.FactKind, v.Fact.Owner.FactKind = "unknown/fact", "unknown/fact"
		},
		func(v *ports.TimelineOwnerCurrentInspectRequestV2) { v.Fact.PayloadSchema = "unknown/schema" },
	} {
		changed := request
		mutate(&changed)
		if _, err := router.ReaderForTimelineOwnerV2(changed); !contract.HasCode(err, contract.ErrUnsupported) {
			t.Fatalf("unknown exact route must fail closed: request=%+v err=%v", changed, err)
		}
	}
}

type ownerRouteReaderV2 struct{}

func (*ownerRouteReaderV2) InspectTimelineOwnerCurrentV2(context.Context, ports.TimelineOwnerCurrentInspectRequestV2) (ports.TimelineOwnerCurrentProjectionV1, error) {
	return ports.TimelineOwnerCurrentProjectionV1{}, nil
}

func (*ownerRouteReaderV2) ValidateTimelineOwnerCurrentV2(context.Context, ports.TimelineOwnerCurrentInspectRequestV2, ports.TimelineOwnerCurrentProjectionV1) error {
	return nil
}

func ownerRouteRequestV2() ports.TimelineOwnerCurrentInspectRequestV2 {
	return ports.TimelineOwnerCurrentInspectRequestV2{
		TenantID: "tenant-a",
		Fact: contract.TimelineOwnerFactRefV1{
			Owner: contract.OwnerBinding{
				BindingSetID: "binding-a", BindingRevision: 1, ComponentID: "component-a",
				ManifestDigest: "manifest-a", ArtifactDigest: "artifact-a",
				Capability: "custom/current", FactKind: "custom/fact",
			},
			FactKind: "custom/fact", FactID: "fact-a", Revision: 1,
			FactDigest: "fact-digest", PayloadSchema: "custom/fact@1.0.0",
			PayloadDigest: "payload-digest", PayloadRevision: 1, ScopeDigest: "scope-digest",
		},
	}
}

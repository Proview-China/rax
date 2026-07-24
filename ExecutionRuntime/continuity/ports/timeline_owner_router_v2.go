package ports

import (
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
)

// TimelineTypedOwnerRouteV2 is one immutable production routing declaration.
// PayloadSchema is the complete canonical schema identity, including its
// content digest when the schema format carries one.
type TimelineTypedOwnerRouteV2 struct {
	OwnerComponentID string                            `json:"owner_component_id"`
	Capability       string                            `json:"capability"`
	FactKind         string                            `json:"fact_kind"`
	PayloadSchema    string                            `json:"payload_schema"`
	Reader           TimelineTypedOwnerCurrentReaderV2 `json:"-"`
}

func (r TimelineTypedOwnerRouteV2) Validate() error {
	for field, value := range map[string]string{
		"owner_component_id": r.OwnerComponentID,
		"capability":         r.Capability,
		"fact_kind":          r.FactKind,
		"payload_schema":     r.PayloadSchema,
	} {
		if err := contract.ValidateToken(field, value); err != nil {
			return err
		}
	}
	if nilOrTypedNilTimelineOwnerV2(r.Reader) {
		return contract.NewError(contract.ErrInvalidArgument, "reader", "typed Owner current Reader is required")
	}
	return nil
}

type timelineTypedOwnerRouteKeyV2 struct {
	OwnerComponentID string
	Capability       string
	FactKind         string
	PayloadSchema    string
}

// ClosedTimelineTypedOwnerRouterV2 has no registration, replacement, or
// deletion API. NewClosedTimelineTypedOwnerRouterV2 copies every declaration
// into its own map before publication, so caller slice mutation cannot alter
// routing after construction.
type ClosedTimelineTypedOwnerRouterV2 struct {
	routes map[timelineTypedOwnerRouteKeyV2]TimelineTypedOwnerCurrentReaderV2
}

func NewClosedTimelineTypedOwnerRouterV2(routes []TimelineTypedOwnerRouteV2) (*ClosedTimelineTypedOwnerRouterV2, error) {
	if len(routes) == 0 {
		return nil, contract.NewError(contract.ErrInvalidArgument, "routes", "at least one typed Owner route is required")
	}
	copied := append([]TimelineTypedOwnerRouteV2(nil), routes...)
	index := make(map[timelineTypedOwnerRouteKeyV2]TimelineTypedOwnerCurrentReaderV2, len(copied))
	for _, route := range copied {
		if err := route.Validate(); err != nil {
			return nil, err
		}
		key := timelineTypedOwnerRouteKeyV2{
			OwnerComponentID: route.OwnerComponentID,
			Capability:       route.Capability,
			FactKind:         route.FactKind,
			PayloadSchema:    route.PayloadSchema,
		}
		if _, exists := index[key]; exists {
			return nil, contract.NewError(contract.ErrProjectionConflict, "routes", "duplicate typed Owner route")
		}
		index[key] = route.Reader
	}
	return &ClosedTimelineTypedOwnerRouterV2{routes: index}, nil
}

func (r *ClosedTimelineTypedOwnerRouterV2) ReaderForTimelineOwnerV2(request TimelineOwnerCurrentInspectRequestV2) (TimelineTypedOwnerCurrentReaderV2, error) {
	if r == nil || r.routes == nil {
		return nil, contract.NewError(contract.ErrUnavailable, "owner_router", "typed Owner router is unavailable")
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	key := timelineTypedOwnerRouteKeyV2{
		OwnerComponentID: request.Fact.Owner.ComponentID,
		Capability:       request.Fact.Owner.Capability,
		FactKind:         request.Fact.FactKind,
		PayloadSchema:    request.Fact.PayloadSchema,
	}
	reader, ok := r.routes[key]
	if !ok || nilOrTypedNilTimelineOwnerV2(reader) {
		return nil, contract.NewError(contract.ErrUnsupported, "owner_route", "no exact typed Owner route is declared")
	}
	return reader, nil
}

func nilOrTypedNilTimelineOwnerV2(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

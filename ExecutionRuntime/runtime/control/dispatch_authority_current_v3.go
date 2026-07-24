package control

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type DispatchAuthorityCurrentFactPortV3 interface {
	InspectCurrentAuthorityFactV3(context.Context, ports.AuthorityBindingRefV2) (ports.DispatchAuthorityFactV3, error)
	InspectHistoricalAuthorityFactV3(context.Context, ports.AuthorityBindingRefV2) (ports.DispatchAuthorityFactV3, error)
	CommitAuthorityFactV3(context.Context, ports.DispatchAuthorityFactPublishRequestV3) (ports.DispatchAuthorityFactPublishReceiptV3, error)
}

// DispatchAuthorityCurrentGatewayV3 is the Authority Owner's public exact
// Reader/Publisher. It performs no Review decision and grants no permit.
type DispatchAuthorityCurrentGatewayV3 struct {
	facts DispatchAuthorityCurrentFactPortV3
	clock func() time.Time
}

func NewDispatchAuthorityCurrentGatewayV3(facts DispatchAuthorityCurrentFactPortV3, clock func() time.Time) (*DispatchAuthorityCurrentGatewayV3, error) {
	if dispatchAuthorityNilV3(facts) || dispatchAuthorityNilV3(clock) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "dispatch authority V3 dependency is missing")
	}
	if clock().IsZero() {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "dispatch authority V3 clock returned zero")
	}
	return &DispatchAuthorityCurrentGatewayV3{facts: facts, clock: clock}, nil
}

func (g *DispatchAuthorityCurrentGatewayV3) InspectCurrentDispatchAuthorityV3(ctx context.Context, expected ports.AuthorityBindingRefV2) (ports.DispatchAuthorityFactV3, error) {
	baseline, err := g.nowV3(time.Time{})
	if err != nil {
		return ports.DispatchAuthorityFactV3{}, err
	}
	if err := expected.Validate(); err != nil {
		return ports.DispatchAuthorityFactV3{}, err
	}
	fact, err := g.facts.InspectCurrentAuthorityFactV3(ctx, expected)
	if err != nil {
		return ports.DispatchAuthorityFactV3{}, err
	}
	now, err := g.nowV3(baseline)
	if err != nil {
		return ports.DispatchAuthorityFactV3{}, err
	}
	if err := fact.ValidateCurrent(expected, fact.Scope, fact.RunID, fact.ActionScopeDigest, now); err != nil {
		return ports.DispatchAuthorityFactV3{}, err
	}
	return fact.Clone(), nil
}

func (g *DispatchAuthorityCurrentGatewayV3) InspectHistoricalDispatchAuthorityV3(ctx context.Context, expected ports.AuthorityBindingRefV2) (ports.DispatchAuthorityFactV3, error) {
	if err := expected.Validate(); err != nil {
		return ports.DispatchAuthorityFactV3{}, err
	}
	return g.facts.InspectHistoricalAuthorityFactV3(ctx, expected)
}

func (g *DispatchAuthorityCurrentGatewayV3) PublishDispatchAuthorityFactV3(ctx context.Context, request ports.DispatchAuthorityFactPublishRequestV3) (ports.DispatchAuthorityFactPublishReceiptV3, error) {
	baseline, err := g.nowV3(time.Time{})
	if err != nil {
		return ports.DispatchAuthorityFactPublishReceiptV3{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.DispatchAuthorityFactPublishReceiptV3{}, err
	}
	now, err := g.nowV3(baseline)
	if err != nil {
		return ports.DispatchAuthorityFactPublishReceiptV3{}, err
	}
	if request.Value.State == ports.AuthorityFactActive {
		if err := request.Value.ValidateCurrent(request.Value.Ref, request.Value.Scope, request.Value.RunID, request.Value.ActionScopeDigest, now); err != nil {
			return ports.DispatchAuthorityFactPublishReceiptV3{}, err
		}
	} else if now.UnixNano() < request.Value.CheckedUnixNano {
		return ports.DispatchAuthorityFactPublishReceiptV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "dispatch authority V3 checked time is in the future")
	}
	receipt, err := g.facts.CommitAuthorityFactV3(ctx, request)
	if err == nil {
		return receipt, nil
	}
	if !core.HasCategory(err, core.ErrorIndeterminate) {
		return ports.DispatchAuthorityFactPublishReceiptV3{}, err
	}
	stored, inspectErr := g.facts.InspectHistoricalAuthorityFactV3(context.WithoutCancel(ctx), request.Value.Ref)
	if inspectErr != nil || !reflect.DeepEqual(stored, request.Value) {
		return ports.DispatchAuthorityFactPublishReceiptV3{}, err
	}
	final, clockErr := g.nowV3(now)
	if clockErr != nil {
		return ports.DispatchAuthorityFactPublishReceiptV3{}, clockErr
	}
	if request.Value.State == ports.AuthorityFactActive {
		if currentErr := request.Value.ValidateCurrent(request.Value.Ref, request.Value.Scope, request.Value.RunID, request.Value.ActionScopeDigest, final); currentErr != nil {
			return ports.DispatchAuthorityFactPublishReceiptV3{}, currentErr
		}
	}
	return ports.DispatchAuthorityFactPublishReceiptV3{Ref: request.Value.Ref, Created: false}, nil
}

func (g *DispatchAuthorityCurrentGatewayV3) nowV3(baseline time.Time) (time.Time, error) {
	now := g.clock()
	if now.IsZero() || (!baseline.IsZero() && now.Before(baseline)) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "dispatch authority V3 clock regressed")
	}
	return now, nil
}

func dispatchAuthorityNilV3(value any) bool {
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

var _ ports.DispatchAuthorityCurrentReaderV3 = (*DispatchAuthorityCurrentGatewayV3)(nil)
var _ ports.DispatchAuthorityCurrentPublisherV3 = (*DispatchAuthorityCurrentGatewayV3)(nil)

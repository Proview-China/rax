package kernel

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// PreparedDomainCommandAssociationGatewayV1 owns the immutable Runtime link
// between one Prepared attempt and one domain command. The link carries no
// execution authority and cannot be renewed after its first lease expires.
type PreparedDomainCommandAssociationGatewayV1 struct {
	store ports.PreparedDomainCommandAssociationStoreV1
	clock func() time.Time
}

func NewPreparedDomainCommandAssociationGatewayV1(store ports.PreparedDomainCommandAssociationStoreV1, clock func() time.Time) (*PreparedDomainCommandAssociationGatewayV1, error) {
	if nilPreparedDomainCommandAssociationDependencyV1(store) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "prepared domain command association dependencies are incomplete")
	}
	return &PreparedDomainCommandAssociationGatewayV1{store: store, clock: clock}, nil
}

func (g *PreparedDomainCommandAssociationGatewayV1) EnsurePreparedDomainCommandAssociationV1(ctx context.Context, request ports.EnsurePreparedDomainCommandAssociationRequestV1) (ports.PreparedDomainCommandAssociationCurrentProjectionV1, error) {
	if err := preparedDomainCommandAssociationContextV1(ctx); err != nil {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, err
	}
	if err := g.validateDependencies(); err != nil {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, err
	}
	refID, err := ports.DerivePreparedDomainCommandAssociationIDV1(request.Prepared, request.Attempt, request.DomainCommand)
	if err != nil {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, err
	}
	existing, inspectErr := g.store.InspectPreparedDomainCommandAssociationByIDV1(ctx, refID)
	if inspectErr == nil {
		return g.validateWinner(request, existing)
	}
	if !core.HasCategory(inspectErr, core.ErrorNotFound) {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, inspectErr
	}

	now := g.clock()
	if now.IsZero() {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "prepared domain command association clock is zero")
	}
	expires := minControlledProviderTimeV2(request.RequestedNotAfterNano, request.Prepared.ExpiresUnixNano)
	if !now.Before(time.Unix(0, expires)) {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "prepared domain command association requested window expired")
	}
	projection, err := ports.SealPreparedDomainCommandAssociationCurrentProjectionV1(ports.PreparedDomainCommandAssociationCurrentProjectionV1{
		Operation: request.Operation, OperationDigest: request.OperationDigest,
		EffectID: request.EffectID, EffectRevision: request.IntentRevision, IntentDigest: request.IntentDigest,
		Prepared: request.Prepared, Attempt: request.Attempt, Provider: request.Provider,
		PayloadSchema: request.PayloadSchema, PayloadDigest: request.PayloadDigest, PayloadRevision: request.PayloadRevision,
		DomainCommand: request.DomainCommand, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
	})
	if err != nil {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, err
	}
	created, err := g.store.CreatePreparedDomainCommandAssociationV1(ctx, projection)
	if err == nil {
		return g.validateWinner(request, created)
	}
	if !core.HasCategory(err, core.ErrorConflict) && !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorIndeterminate) {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, err
	}
	// A lost create reply or concurrent winner is recovered only by the stable
	// derived ID, then validated against the full canonical request.
	winner, inspectErr := g.store.InspectPreparedDomainCommandAssociationByIDV1(context.WithoutCancel(ctx), refID)
	if inspectErr != nil {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, err
	}
	return g.validateWinner(request, winner)
}

func (g *PreparedDomainCommandAssociationGatewayV1) InspectCurrentPreparedDomainCommandAssociationV1(ctx context.Context, exact ports.PreparedDomainCommandAssociationRefV1) (ports.PreparedDomainCommandAssociationCurrentProjectionV1, error) {
	if err := preparedDomainCommandAssociationContextV1(ctx); err != nil {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, err
	}
	if err := g.validateDependencies(); err != nil {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, err
	}
	if exact.Validate() != nil {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "prepared domain command association exact Ref is invalid")
	}
	projection, err := g.store.InspectPreparedDomainCommandAssociationV1(ctx, exact)
	if err != nil {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, err
	}
	now := g.clock()
	if err := projection.ValidateCurrent(exact, now); err != nil {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, err
	}
	return projection, nil
}

func (g *PreparedDomainCommandAssociationGatewayV1) validateWinner(request ports.EnsurePreparedDomainCommandAssociationRequestV1, winner ports.PreparedDomainCommandAssociationCurrentProjectionV1) (ports.PreparedDomainCommandAssociationCurrentProjectionV1, error) {
	if err := winner.Validate(); err != nil {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, err
	}
	if !samePreparedDomainCommandAssociationRequestV1(request, winner) {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "prepared domain command association ID already binds another request")
	}
	now := g.clock()
	if err := winner.ValidateCurrent(winner.Ref, now); err != nil {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, err
	}
	if winner.ExpiresUnixNano > request.RequestedNotAfterNano || winner.ExpiresUnixNano > request.Prepared.ExpiresUnixNano {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "prepared domain command association winner exceeds the requested window")
	}
	return winner, nil
}

func (g *PreparedDomainCommandAssociationGatewayV1) validateDependencies() error {
	if g == nil || nilPreparedDomainCommandAssociationDependencyV1(g.store) || g.clock == nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "prepared domain command association gateway is unavailable")
	}
	return nil
}

func samePreparedDomainCommandAssociationRequestV1(request ports.EnsurePreparedDomainCommandAssociationRequestV1, projection ports.PreparedDomainCommandAssociationCurrentProjectionV1) bool {
	return ports.SameOperationSubjectV3(request.Operation, projection.Operation) && request.OperationDigest == projection.OperationDigest && request.EffectID == projection.EffectID && request.IntentRevision == projection.EffectRevision && request.IntentDigest == projection.IntentDigest && request.Prepared == projection.Prepared && request.Attempt == projection.Attempt && request.Provider == projection.Provider && request.PayloadSchema == projection.PayloadSchema && request.PayloadDigest == projection.PayloadDigest && request.PayloadRevision == projection.PayloadRevision && request.DomainCommand == projection.DomainCommand
}

func preparedDomainCommandAssociationContextV1(ctx context.Context) error {
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "prepared domain command association context is nil")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func nilPreparedDomainCommandAssociationDependencyV1(value any) bool {
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

var _ ports.PreparedDomainCommandAssociationPortV1 = (*PreparedDomainCommandAssociationGatewayV1)(nil)

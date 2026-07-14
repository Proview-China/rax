package application

import (
	"context"
	"time"

	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// RuntimeBindingCurrentnessAdapterV3 narrows Runtime's read-only Binding
// projection into the short-lived Application authorization consumed by the
// operation-domain router. It cannot bind, renew, revoke or dispatch.
type RuntimeBindingCurrentnessAdapterV3 struct {
	runtime runtimeports.ProviderBindingCurrentnessPortV2
	clock   func() time.Time
}

func NewRuntimeBindingCurrentnessAdapterV3(runtime runtimeports.ProviderBindingCurrentnessPortV2, clock func() time.Time) (*RuntimeBindingCurrentnessAdapterV3, error) {
	if runtime == nil || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Runtime Binding currentness Port and Clock are required")
	}
	return &RuntimeBindingCurrentnessAdapterV3{runtime: runtime, clock: clock}, nil
}

func (a *RuntimeBindingCurrentnessAdapterV3) InspectOperationDomainAdapterCurrentV3(ctx context.Context, expected runtimeports.ProviderBindingRefV2) (applicationports.OperationDomainAdapterAuthorizationV3, error) {
	if err := expected.Validate(); err != nil {
		return applicationports.OperationDomainAdapterAuthorizationV3{}, err
	}
	now := a.clock()
	projection, err := a.runtime.InspectProviderBindingCurrentV2(ctx, expected)
	if err != nil {
		return applicationports.OperationDomainAdapterAuthorizationV3{}, err
	}
	if err := projection.ValidateCurrent(expected, now); err != nil {
		return applicationports.OperationDomainAdapterAuthorizationV3{}, err
	}

	expires := now.Add(applicationports.MaxOperationDomainAdapterAuthorizationTTLV3)
	projectionExpiry := time.Unix(0, projection.ExpiresUnixNano)
	if projectionExpiry.Before(expires) {
		expires = projectionExpiry
	}
	authorization, err := applicationports.SealOperationDomainAdapterAuthorizationV3(applicationports.OperationDomainAdapterAuthorizationV3{
		ContractVersion: applicationports.OperationDomainContractVersionV3,
		Adapter:         expected,
		Revision:        projection.BindingRevision,
		State:           applicationports.OperationDomainAdapterAuthorizedV3,
		IssuedUnixNano:  now.UnixNano(),
		ExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil {
		return applicationports.OperationDomainAdapterAuthorizationV3{}, err
	}
	if err := authorization.ValidateCurrentFor(expected, now); err != nil {
		return applicationports.OperationDomainAdapterAuthorizationV3{}, err
	}
	return authorization, nil
}

var _ applicationports.OperationDomainAdapterCurrentnessPortV3 = (*RuntimeBindingCurrentnessAdapterV3)(nil)

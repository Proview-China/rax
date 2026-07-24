package runtimeadapter

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type ClockV1 interface{ Now() time.Time }

type ToolBoundarySourceCurrentReaderV1 interface {
	InspectBoundarySourceCurrentV1(context.Context, contract.ToolProviderBoundarySourceRefV1, time.Time) (contract.SingleCallToolActionCoordinationWatermarkV1, error)
}

type ProviderBoundaryCurrentAdapterV1 struct {
	source ToolBoundarySourceCurrentReaderV1
	clock  ClockV1
}

func NewProviderBoundaryCurrentAdapterV1(source ToolBoundarySourceCurrentReaderV1, clock ClockV1) *ProviderBoundaryCurrentAdapterV1 {
	return &ProviderBoundaryCurrentAdapterV1{source: source, clock: clock}
}

func (a *ProviderBoundaryCurrentAdapterV1) InspectCurrentOperationProviderBoundaryV1(ctx context.Context, exact runtimeports.OperationProviderBoundaryRefV1) (runtimeports.OperationProviderBoundaryCurrentProjectionV1, error) {
	if err := exact.Validate(); err != nil {
		return runtimeports.OperationProviderBoundaryCurrentProjectionV1{}, err
	}
	if a == nil || a.source == nil || a.clock == nil {
		return runtimeports.OperationProviderBoundaryCurrentProjectionV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "Tool provider boundary reader dependencies are unavailable")
	}
	now := a.clock.Now()
	if now.IsZero() {
		return runtimeports.OperationProviderBoundaryCurrentProjectionV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "Tool provider boundary current read requires current time")
	}
	source := contract.ToolProviderBoundarySourceRefV1{WatermarkID: exact.ID, WatermarkRevision: exact.Revision, WatermarkDigest: exact.Digest}
	runtimeRef, err := source.RuntimeRefV1()
	if err != nil || runtimeRef != exact {
		return runtimeports.OperationProviderBoundaryCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "Runtime boundary ref cannot be losslessly mapped to Tool source ref")
	}
	w, err := a.source.InspectBoundarySourceCurrentV1(ctx, source, now)
	if err != nil {
		return runtimeports.OperationProviderBoundaryCurrentProjectionV1{}, err
	}
	if w.ID != exact.ID || w.Revision != exact.Revision || w.Digest != exact.Digest || w.Stage != contract.CoordinationProviderBoundaryV1 || w.Operation == nil || w.RuntimeAttempt == nil || w.ExecuteEnforcement == nil || w.ExecuteHandoff == nil {
		return runtimeports.OperationProviderBoundaryCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "Tool boundary source projection drifted")
	}
	expires := contract.MinUnixNanoV1(w.ExpiresUnixNano, w.ExecuteEnforcement.ExpiresUnixNano, runtimeports.OperationScopeEvidenceFactRefV3(*w.ExecuteHandoff).ExpiresUnixNano)
	projection, err := runtimeports.SealOperationProviderBoundaryCurrentProjectionV1(runtimeports.OperationProviderBoundaryCurrentProjectionV1{Ref: exact, Operation: *w.Operation, OperationDigest: w.OperationDigest, OperationScopeDigest: w.OperationScopeDigest, Attempt: *w.RuntimeAttempt, ExecuteEnforcement: *w.ExecuteEnforcement, ExecuteEvidenceHandoff: *w.ExecuteHandoff, Stage: runtimeports.OperationProviderBoundaryCrossedV1, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires})
	if err != nil {
		return runtimeports.OperationProviderBoundaryCurrentProjectionV1{}, err
	}
	if err = projection.ValidateCurrent(exact, *w.Operation, w.OperationScopeDigest, *w.RuntimeAttempt, *w.ExecuteEnforcement, *w.ExecuteHandoff, now); err != nil {
		return runtimeports.OperationProviderBoundaryCurrentProjectionV1{}, err
	}
	return projection, nil
}

var _ runtimeports.OperationProviderBoundaryCurrentReaderV1 = (*ProviderBoundaryCurrentAdapterV1)(nil)

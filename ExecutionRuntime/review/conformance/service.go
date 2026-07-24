package conformance

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/service"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type ServiceV1 interface {
	SubmitV1(context.Context, service.SubmitCommandV1) (service.ReviewViewV1, error)
	InspectV1(context.Context, core.TenantID, string) (service.ReviewViewV1, error)
	ListV1(context.Context, reviewport.ListCasesRequestV1) (reviewport.ListCasesResultV1, error)
	EventsV1(context.Context, core.TenantID, string) ([]contract.TraceFactV1, error)
}

type ServiceFixtureV1 struct{ Submit service.SubmitCommandV1 }

func CheckServiceV1(ctx context.Context, owner ServiceV1, fixture ServiceFixtureV1) error {
	if owner == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review service conformance owner is missing")
	}
	first, err := owner.SubmitV1(ctx, fixture.Submit)
	if err != nil {
		return err
	}
	replay, err := owner.SubmitV1(ctx, fixture.Submit)
	if err != nil {
		return err
	}
	if first.Case.Digest != replay.Case.Digest || first.Request == nil || replay.Request == nil || first.Request.Digest != replay.Request.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Review service canonical replay drifted")
	}
	if fixture.Submit.ResultBundle != nil && (first.ResultBundle == nil || replay.ResultBundle == nil || first.ResultBundle.Digest != fixture.Submit.ResultBundle.Digest || replay.ResultBundle.Digest != fixture.Submit.ResultBundle.Digest) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review service Result Bundle replay drifted")
	}
	inspected, err := owner.InspectV1(ctx, first.Case.TenantID, first.Case.ID)
	if err != nil {
		return err
	}
	if inspected.Case.Digest != first.Case.Digest || inspected.Target.Digest != first.Target.Digest || inspected.Request == nil || inspected.Request.Digest != first.Request.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review service Inspect drifted")
	}
	if fixture.Submit.ResultBundle != nil && (inspected.ResultBundle == nil || inspected.ResultBundle.Digest != fixture.Submit.ResultBundle.Digest) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review service Result Bundle Inspect drifted")
	}
	page, err := owner.ListV1(ctx, reviewport.ListCasesRequestV1{TenantID: first.Case.TenantID, Limit: 1})
	if err != nil {
		return err
	}
	if len(page.Cases) != 1 || page.Cases[0].Digest != first.Case.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review service list snapshot drifted")
	}
	events, err := owner.EventsV1(ctx, first.Case.TenantID, first.Case.ID)
	if err != nil {
		return err
	}
	if fixture.Submit.Trace.ID != "" && (len(events) != 1 || events[0].Digest != fixture.Submit.Trace.Digest) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review service trace history drifted")
	}
	return nil
}

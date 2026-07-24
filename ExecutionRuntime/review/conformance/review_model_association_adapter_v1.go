package conformance

import (
	"context"
	"reflect"

	hostcontract "github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/hostadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type ReviewModelAssociationAdapterPortV1 interface {
	StartOrInspectAssociationV1(context.Context, hostadapter.ReviewModelAssociationRequestV1) (hostcontract.ReviewModelInvocationAssociationFactV1, error)
	InspectCurrentAssociationV1(context.Context, contract.AutoReviewerAttemptV1, hostcontract.ReviewModelInvocationAssociationRefV1) (hostcontract.ReviewModelInvocationAssociationFactV1, error)
}
type ReviewModelAssociationAdapterReportV1 struct {
	ExactMapping, S1S2Current, IdempotentReplay bool
	ProductionEligible                          bool
}

func RunReviewModelAssociationAdapterV1(ctx context.Context, adapter ReviewModelAssociationAdapterPortV1, request hostadapter.ReviewModelAssociationRequestV1) (ReviewModelAssociationAdapterReportV1, error) {
	report := ReviewModelAssociationAdapterReportV1{ProductionEligible: false}
	if nilcheck.IsNil(adapter) {
		return report, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review Host association conformance adapter is nil")
	}
	first, err := adapter.StartOrInspectAssociationV1(ctx, request)
	if err != nil {
		return report, err
	}
	attemptRef := request.Attempt.ExactRef()
	if first.Subject.TenantID != request.Attempt.TenantID || first.Subject.ReviewAttempt.ID != attemptRef.ID || first.Subject.ReviewAttempt.Revision != attemptRef.Revision || first.Subject.ReviewAttempt.Digest != attemptRef.Digest {
		return report, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review Host association neutral mapping drifted")
	}
	report.ExactMapping = true
	inspected, err := adapter.InspectCurrentAssociationV1(ctx, request.Attempt, first.RefV1())
	if err != nil || !reflect.DeepEqual(inspected, first) {
		if err != nil {
			return report, err
		}
		return report, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Host association S1/S2 Inspect drifted")
	}
	report.S1S2Current = true
	replayed, err := adapter.StartOrInspectAssociationV1(ctx, request)
	if err != nil || !reflect.DeepEqual(replayed, first) {
		if err != nil {
			return report, err
		}
		return report, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Review Host association replay drifted")
	}
	report.IdempotentReplay = true
	return report, nil
}

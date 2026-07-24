package multisigowner

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// ClaimOwnerV2 owns only the Review Assignment lease transition. The injected
// Organization reader is read-only; a successful lease never grants Authority.
type ClaimOwnerV2 struct {
	store        reviewport.StoreV2
	organization reviewport.HumanOrganizationCurrentReaderV2
	clock        Clock
}

func NewClaimOwnerV2(store reviewport.StoreV2, organization reviewport.HumanOrganizationCurrentReaderV2, clock Clock) (*ClaimOwnerV2, error) {
	if nilcheck.IsNil(store) || nilcheck.IsNil(organization) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "human claim Owner requires Store, Organization current Reader and clock")
	}
	return &ClaimOwnerV2{store: store, organization: organization, clock: clock}, nil
}

func (o *ClaimOwnerV2) ClaimAssignmentV2(ctx context.Context, mutation reviewport.ClaimHumanAssignmentMutationV2, request reviewport.HumanOrganizationCurrentRequestV2) (reviewport.ClaimHumanAssignmentResultV2, error) {
	if err := reviewport.ValidateClaimHumanAssignmentTraceV2(mutation); err != nil {
		return reviewport.ClaimHumanAssignmentResultV2{}, err
	}
	baseline := o.clock()
	if baseline.IsZero() || baseline.UnixNano() <= 0 {
		return reviewport.ClaimHumanAssignmentResultV2{}, clockError()
	}
	if recovered, ok, err := o.inspectCanonicalClaimV2(ctx, mutation); err != nil || ok {
		return recovered, err
	}
	panel, err := o.store.InspectHumanPanelCurrentV2(ctx, mutation.ExpectedPanel.TenantID, mutation.ExpectedPanel.ID)
	if err != nil {
		return reviewport.ClaimHumanAssignmentResultV2{}, err
	}
	assignment, err := o.store.InspectHumanPanelAssignmentCurrentV2(ctx, mutation.ExpectedAssignment.TenantID, mutation.ExpectedAssignment.ID)
	if err != nil {
		return reviewport.ClaimHumanAssignmentResultV2{}, err
	}
	if panel.ExactRef() != mutation.ExpectedPanel || assignment.ExactRef() != mutation.ExpectedAssignment || !reflect.DeepEqual(request.Panel, panel) || !reflect.DeepEqual(request.Assignment, assignment) {
		return reviewport.ClaimHumanAssignmentResultV2{}, core.NewError(core.ErrorConflict, core.ReasonStaleLeaseRevision, "human claim current Panel, Assignment or Organization request drifted")
	}
	if request.ReviewerSubjectID != mutation.NextAssignment.LeaseHolder {
		return reviewport.ClaimHumanAssignmentResultV2{}, core.NewError(core.ErrorForbidden, core.ReasonOwnerConflict, "human claim principal does not match the exact reviewer subject")
	}
	if err := panel.ValidateCurrent(mutation.ExpectedPanel, baseline); err != nil {
		return reviewport.ClaimHumanAssignmentResultV2{}, err
	}
	if err := assignment.ValidateCurrent(mutation.ExpectedAssignment, baseline); err != nil {
		return reviewport.ClaimHumanAssignmentResultV2{}, err
	}
	cut, err := o.organization.InspectHumanOrganizationCurrentV2(ctx, []reviewport.HumanOrganizationCurrentRequestV2{request})
	if err != nil {
		return reviewport.ClaimHumanAssignmentResultV2{}, err
	}
	if err := cut.Validate(baseline); err != nil {
		return reviewport.ClaimHumanAssignmentResultV2{}, err
	}
	if len(cut.Items) != 1 {
		return reviewport.ClaimHumanAssignmentResultV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "human claim Organization cut must contain exactly the requested Assignment")
	}
	if err := cut.Items[0].Validate(request, baseline); err != nil {
		return reviewport.ClaimHumanAssignmentResultV2{}, err
	}
	wantLeaseExpiry := minExpires(panel.ExpiresUnixNano, assignment.ExpiresUnixNano, cut.ExpiresUnixNano, mutation.NextAssignment.UpdatedUnixNano+panel.MaxVoteTTLNanos)
	if mutation.NextAssignment.LeaseExpiresUnixNano != wantLeaseExpiry {
		return reviewport.ClaimHumanAssignmentResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonStaleLeaseRevision, "human claim lease expiry is not the exact current-input minimum")
	}
	now := o.clock()
	if now.IsZero() || now.Before(baseline) {
		return reviewport.ClaimHumanAssignmentResultV2{}, clockError()
	}
	if mutation.NextAssignment.UpdatedUnixNano > now.UnixNano() || mutation.NextPanel.UpdatedUnixNano > now.UnixNano() {
		return reviewport.ClaimHumanAssignmentResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "human claim facts are dated after the actual point")
	}
	if err := cut.Validate(now); err != nil {
		return reviewport.ClaimHumanAssignmentResultV2{}, err
	}
	if err := cut.Items[0].Validate(request, now); err != nil {
		return reviewport.ClaimHumanAssignmentResultV2{}, err
	}
	if err := panel.ValidateCurrent(mutation.ExpectedPanel, now); err != nil {
		return reviewport.ClaimHumanAssignmentResultV2{}, err
	}
	if err := assignment.ValidateCurrent(mutation.ExpectedAssignment, now); err != nil {
		return reviewport.ClaimHumanAssignmentResultV2{}, err
	}
	if err := mutation.NextPanel.ValidateCurrent(mutation.NextPanel.ExactRef(), now); err != nil {
		return reviewport.ClaimHumanAssignmentResultV2{}, err
	}
	if err := mutation.NextAssignment.ValidateCurrent(mutation.NextAssignment.ExactRef(), now); err != nil {
		return reviewport.ClaimHumanAssignmentResultV2{}, err
	}
	result, err := o.store.ClaimHumanAssignmentV2(ctx, mutation)
	if err == nil || (!core.HasCategory(err, core.ErrorIndeterminate) && !core.HasCategory(err, core.ErrorUnavailable)) {
		return result, err
	}
	actualPoint := now
	recovery, cancel, ok := boundedRecoveryContextV2(ctx, o.clock, actualPoint,
		mutation.NextPanel.ExpiresUnixNano,
		mutation.NextAssignment.ExpiresUnixNano,
		mutation.NextAssignment.LeaseExpiresUnixNano,
		cut.ExpiresUnixNano,
	)
	if !ok {
		return reviewport.ClaimHumanAssignmentResultV2{}, err
	}
	defer cancel()
	nextPanel, panelErr := o.store.InspectHumanPanelExactV2(recovery, mutation.NextPanel.ExactRef())
	if panelErr != nil {
		return reviewport.ClaimHumanAssignmentResultV2{}, err
	}
	nextAssignment, assignmentErr := o.store.InspectHumanPanelAssignmentExactV2(recovery, mutation.NextAssignment.ExactRef())
	if assignmentErr != nil {
		return reviewport.ClaimHumanAssignmentResultV2{}, err
	}
	trace, traceErr := o.store.InspectTraceExactV1(recovery, mutation.Trace.TenantID, reviewport.ExactV1(mutation.Trace.ID, mutation.Trace.Revision, mutation.Trace.Digest))
	if traceErr != nil {
		return reviewport.ClaimHumanAssignmentResultV2{}, err
	}
	now = o.clock()
	if now.IsZero() || now.Before(actualPoint) {
		return reviewport.ClaimHumanAssignmentResultV2{}, err
	}
	if cut.Validate(now) != nil {
		return reviewport.ClaimHumanAssignmentResultV2{}, err
	}
	if cut.Items[0].Validate(request, now) != nil {
		return reviewport.ClaimHumanAssignmentResultV2{}, err
	}
	if nextPanel.ValidateCurrent(mutation.NextPanel.ExactRef(), now) != nil {
		return reviewport.ClaimHumanAssignmentResultV2{}, err
	}
	if nextAssignment.ValidateCurrent(mutation.NextAssignment.ExactRef(), now) != nil {
		return reviewport.ClaimHumanAssignmentResultV2{}, err
	}
	if !reflect.DeepEqual(nextPanel, mutation.NextPanel) || !reflect.DeepEqual(nextAssignment, mutation.NextAssignment) || !reflect.DeepEqual(trace, mutation.Trace) {
		return reviewport.ClaimHumanAssignmentResultV2{}, err
	}
	return reviewport.ClaimHumanAssignmentResultV2{Panel: nextPanel, Assignment: nextAssignment}, nil
}

// inspectCanonicalClaimV2 is read-only recovery. Authoritative NotFound for the
// exact next Assignment means the mutation has not been observed; any partial
// closure or non-NotFound read error fails closed and never grants a retry by
// itself.
func (o *ClaimOwnerV2) inspectCanonicalClaimV2(ctx context.Context, mutation reviewport.ClaimHumanAssignmentMutationV2) (reviewport.ClaimHumanAssignmentResultV2, bool, error) {
	assignment, err := o.store.InspectHumanPanelAssignmentExactV2(ctx, mutation.NextAssignment.ExactRef())
	if err != nil {
		if core.HasCategory(err, core.ErrorNotFound) {
			return reviewport.ClaimHumanAssignmentResultV2{}, false, nil
		}
		return reviewport.ClaimHumanAssignmentResultV2{}, false, err
	}
	panel, err := o.store.InspectHumanPanelExactV2(ctx, mutation.NextPanel.ExactRef())
	if err != nil {
		return reviewport.ClaimHumanAssignmentResultV2{}, false, core.NewError(core.ErrorConflict, core.ReasonEffectUnknownOutcome, "human claim exact recovery found a partial Panel/Assignment closure")
	}
	if _, err = o.store.InspectHumanPanelExactV2(ctx, mutation.ExpectedPanel); err != nil {
		return reviewport.ClaimHumanAssignmentResultV2{}, false, core.NewError(core.ErrorConflict, core.ReasonEffectUnknownOutcome, "human claim exact recovery lost its expected Panel history")
	}
	if _, err = o.store.InspectHumanPanelAssignmentExactV2(ctx, mutation.ExpectedAssignment); err != nil {
		return reviewport.ClaimHumanAssignmentResultV2{}, false, core.NewError(core.ErrorConflict, core.ReasonEffectUnknownOutcome, "human claim exact recovery lost its expected Assignment history")
	}
	if _, err = o.store.InspectTraceExactV1(ctx, mutation.Trace.TenantID, reviewport.ExactV1(mutation.Trace.ID, mutation.Trace.Revision, mutation.Trace.Digest)); err != nil {
		return reviewport.ClaimHumanAssignmentResultV2{}, false, core.NewError(core.ErrorConflict, core.ReasonEffectUnknownOutcome, "human claim exact recovery found a partial Trace closure")
	}
	if panel.Digest != mutation.NextPanel.Digest || assignment.Digest != mutation.NextAssignment.Digest {
		return reviewport.ClaimHumanAssignmentResultV2{}, false, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "human claim exact recovery found different content")
	}
	return reviewport.ClaimHumanAssignmentResultV2{Panel: panel, Assignment: assignment}, true, nil
}

package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/journal"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
)

type ConfigV3 struct {
	Claims     ports.HostStartClaimPortV3
	Journal    *journal.CoordinatorV2
	Deployment ports.HostDeploymentCurrentReaderV1
	Pipeline   ports.HostV3OwnerPipeline
	Clock      func() time.Time
}

// HostV3 is a reference-executable lifecycle coordinator. It owns no domain
// Current beyond the existing Host Claim/Journal Owners and makes no
// production composition, provider, HA or Model Loop claim.
type HostV3 struct {
	claims     ports.HostStartClaimPortV3
	admission  *journal.HostStartAdmissionV3
	journal    *journal.CoordinatorV2
	deployment ports.HostDeploymentCurrentReaderV1
	pipeline   ports.HostV3OwnerPipeline
	clock      func() time.Time
}

var _ ports.HostV3 = (*HostV3)(nil)

func NewHostV3(config ConfigV3) (*HostV3, error) {
	for name, value := range map[string]any{"claims": config.Claims, "journal": config.Journal, "deployment": config.Deployment, "pipeline": config.Pipeline} {
		if contract.IsTypedNilV1(value) {
			return nil, contract.NewError(contract.ErrorInvalidArgument, "host_v3_dependency_missing", name+" is required")
		}
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	admission, err := journal.NewHostStartAdmissionV3(config.Claims, config.Clock)
	if err != nil {
		return nil, err
	}
	return &HostV3{claims: config.Claims, admission: admission, journal: config.Journal, deployment: config.Deployment, pipeline: config.Pipeline, clock: config.Clock}, nil
}

func (h *HostV3) StartV3(ctx context.Context, request contract.StartRequestV3) (contract.StartResultV3, error) {
	if h == nil {
		return contract.StartResultV3{}, contract.NewError(contract.ErrorUnavailable, "host_v3_missing", "HostV3 is unavailable")
	}
	if ctx == nil {
		return contract.StartResultV3{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	now, err := h.nowV3(time.Time{})
	if err != nil {
		return contract.StartResultV3{}, err
	}
	if err = request.ValidateCurrent(now); err != nil {
		return contract.StartResultV3{}, err
	}
	deployment, err := safeDeploymentCurrentV3(ctx, h.deployment, request.DeploymentCurrentRef)
	if err != nil {
		return contract.StartResultV3{}, err
	}
	if err = deployment.ValidateCurrentV1(request.DeploymentCurrentRef, now); err != nil {
		return contract.StartResultV3{}, err
	}
	input, err := request.ClaimInputV3()
	if err != nil {
		return contract.StartResultV3{}, err
	}
	binding, err := h.admission.ClaimV3(ctx, input)
	if err != nil {
		return contract.StartResultV3{}, err
	}
	claim, err := binding.Input.ClaimV1()
	if err != nil {
		return contract.StartResultV3{}, err
	}
	current, err := h.journal.EnsureAcceptedV3(ctx, claim)
	if err != nil {
		return contract.StartResultV3{}, err
	}
	projection, startErr := safeStartPipelineV3(ctx, h.pipeline, request, binding, current)
	if startErr != nil {
		if !isReconcileOnlyErrorV3(startErr) {
			return contract.StartResultV3{}, startErr
		}
		reconciledJournal, journalErr := h.journal.InspectV2(context.WithoutCancel(ctx), request.Config.HostID, request.StartID)
		if journalErr != nil {
			return contract.StartResultV3{}, errors.Join(startErr, journalErr)
		}
		projection, err = safeInspectPipelineV3(context.WithoutCancel(ctx), h.pipeline, binding, reconciledJournal)
		if err != nil {
			return contract.StartResultV3{}, errors.Join(contract.NewError(contract.ErrorUnknownOutcome, "host_v3_start_reconciliation_required", "HostV3 Owner pipeline may have started and is Inspect-only until exact results are visible"), startErr, err)
		}
	}
	latest, err := h.journal.InspectV2(context.WithoutCancel(ctx), request.Config.HostID, request.StartID)
	if err != nil {
		return contract.StartResultV3{}, err
	}
	journalRef, err := latest.RefV2()
	if err != nil {
		return contract.StartResultV3{}, err
	}
	now, err = h.nowV3(time.Unix(0, latest.UpdatedUnixNano))
	if err != nil {
		return contract.StartResultV3{}, err
	}
	if err = projection.ValidateFor(request, binding.ClaimRef, journalRef, now); err != nil {
		return contract.StartResultV3{}, err
	}
	expires := request.RequestedNotAfterUnixNano
	for _, v := range []int64{binding.ClaimRef.ExpiresUnixNano, projection.Ready.ExpiresUnixNano, projection.Availability.ExpiresUnixNano} {
		if v < expires {
			expires = v
		}
	}
	result, err := contract.SealStartResultV3(contract.StartResultV3{HostID: request.Config.HostID, StartID: request.StartID, RequestDigest: request.RequestDigest, RequestNotAfterUnixNano: request.RequestedNotAfterUnixNano, StartClaim: binding.ClaimRef, Journal: journalRef, CleanupClosure: projection.CleanupClosure, Ready: projection.Ready, Availability: projection.Availability, CheckedUnixNano: projection.CheckedUnixNano, ExpiresUnixNano: expires})
	if err != nil {
		return contract.StartResultV3{}, err
	}
	if err = result.ValidateFor(request, now); err != nil {
		return contract.StartResultV3{}, err
	}
	return result, nil
}

func (h *HostV3) InspectV3(ctx context.Context, request contract.InspectRequestV3) (contract.InspectResultV3, error) {
	if h == nil {
		return contract.InspectResultV3{}, contract.NewError(contract.ErrorUnavailable, "host_v3_missing", "HostV3 is unavailable")
	}
	if ctx == nil {
		return contract.InspectResultV3{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	now, err := h.nowV3(time.Time{})
	if err != nil {
		return contract.InspectResultV3{}, err
	}
	if err = request.ValidateCurrent(now); err != nil {
		return contract.InspectResultV3{}, err
	}
	claim, binding, current, journalRef, err := h.inspectOriginV3(ctx, request.StartClaim)
	if err != nil {
		return contract.InspectResultV3{}, err
	}
	projection, inspectErr := safeInspectPipelineV3(ctx, h.pipeline, binding, current)
	result := contract.InspectResultV3{RequestDigest: request.RequestDigest, RequestNotAfterUnixNano: request.RequestedNotAfterUnixNano, StartClaim: claim, Journal: journalRef, Phase: inspectPhaseV3(current.Phase), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: request.RequestedNotAfterUnixNano}
	if inspectErr == nil {
		if err = projection.ValidateForOrigin(binding, journalRef, now); err != nil {
			return contract.InspectResultV3{}, err
		}
		result.HasReady, result.Ready = true, projection.Ready
		result.HasAvailability, result.Availability = true, projection.Availability
		result.HasCleanupClosure, result.CleanupClosure = true, projection.CleanupClosure
		if projection.ExpiresUnixNano < result.ExpiresUnixNano {
			result.ExpiresUnixNano = projection.ExpiresUnixNano
		}
	} else if isReconcileOnlyErrorV3(inspectErr) {
		result.Phase = contract.HostInspectIndeterminateV3
	} else if !contract.HasCode(inspectErr, contract.ErrorNotFound) {
		return contract.InspectResultV3{}, inspectErr
	}
	result, err = contract.SealInspectResultV3(result)
	if err != nil {
		return contract.InspectResultV3{}, err
	}
	if err = result.ValidateFor(request, now); err != nil {
		return contract.InspectResultV3{}, err
	}
	return result, nil
}

func (h *HostV3) StopV3(ctx context.Context, request contract.StopRequestV3) (contract.StopResultV3, error) {
	if h == nil {
		return contract.StopResultV3{}, contract.NewError(contract.ErrorUnavailable, "host_v3_missing", "HostV3 is unavailable")
	}
	if ctx == nil {
		return contract.StopResultV3{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	now, err := h.nowV3(time.Time{})
	if err != nil {
		return contract.StopResultV3{}, err
	}
	if err = request.ValidateCurrent(now); err != nil {
		return contract.StopResultV3{}, err
	}
	_, binding, current, _, err := h.inspectOriginV3(ctx, request.StartClaim)
	if err != nil {
		return contract.StopResultV3{}, err
	}
	startProjection, err := safeInspectPipelineV3(ctx, h.pipeline, binding, current)
	if err != nil {
		return contract.StopResultV3{}, err
	}
	journalRef, _ := current.RefV2()
	if err = startProjection.ValidateForOrigin(binding, journalRef, now); err != nil {
		return contract.StopResultV3{}, err
	}
	if startProjection.CleanupClosure != request.CleanupClosure {
		return contract.StopResultV3{}, contract.NewError(contract.ErrorConflict, "host_v3_stop_cleanup_closure_drift", "HostV3 Stop names another Cleanup Closure")
	}
	projection, stopErr := safeStopPipelineV3(ctx, h.pipeline, request, binding, current)
	if stopErr != nil {
		if !isReconcileOnlyErrorV3(stopErr) {
			return contract.StopResultV3{}, stopErr
		}
		reconciledJournal, journalErr := h.journal.InspectV2(context.WithoutCancel(ctx), request.HostID, request.StartID)
		if journalErr != nil {
			return contract.StopResultV3{}, errors.Join(stopErr, journalErr)
		}
		projection, err = safeInspectStopPipelineV3(context.WithoutCancel(ctx), h.pipeline, request, binding, reconciledJournal)
		if err != nil {
			return contract.StopResultV3{}, errors.Join(contract.NewError(contract.ErrorUnknownOutcome, "host_v3_stop_reconciliation_required", "HostV3 Stop may have started and only the original Cleanup Closure may be inspected"), stopErr, err)
		}
	}
	if err = projection.ValidateFor(request); err != nil {
		return contract.StopResultV3{}, err
	}
	result, err := contract.SealStopResultV3(contract.StopResultV3{RequestDigest: request.RequestDigest, Journal: projection.Journal, CleanupClosure: projection.CleanupClosure, CleanupResult: projection.CleanupResult, State: projection.State, CheckedUnixNano: projection.CheckedUnixNano})
	if err != nil {
		return contract.StopResultV3{}, err
	}
	if err = result.ValidateFor(request); err != nil {
		return contract.StopResultV3{}, err
	}
	return result, nil
}

func isReconcileOnlyErrorV3(err error) bool {
	return contract.HasCode(err, contract.ErrorUnknownOutcome) || contract.HasCode(err, contract.ErrorUnavailable)
}

func (h *HostV3) inspectOriginV3(ctx context.Context, expected contract.HostStartClaimRefV1) (contract.HostStartClaimV1, contract.HostStartClaimInputBindingV3, contract.HostJournalV2, contract.ExactRefV1, error) {
	claim, err := h.claims.InspectHostStartClaimCurrentV1(ctx, expected)
	if err != nil {
		return contract.HostStartClaimV1{}, contract.HostStartClaimInputBindingV3{}, contract.HostJournalV2{}, contract.ExactRefV1{}, err
	}
	binding, err := h.claims.InspectHostStartClaimInputV3(ctx, expected)
	if err != nil {
		return contract.HostStartClaimV1{}, contract.HostStartClaimInputBindingV3{}, contract.HostJournalV2{}, contract.ExactRefV1{}, err
	}
	current, err := h.journal.InspectV2(ctx, expected.HostID, expected.StartID)
	if err != nil {
		return contract.HostStartClaimV1{}, contract.HostStartClaimInputBindingV3{}, contract.HostJournalV2{}, contract.ExactRefV1{}, err
	}
	ref, err := current.RefV2()
	if err != nil {
		return contract.HostStartClaimV1{}, contract.HostStartClaimInputBindingV3{}, contract.HostJournalV2{}, contract.ExactRefV1{}, err
	}
	claimRef, refErr := claim.RefV1()
	if refErr != nil {
		return contract.HostStartClaimV1{}, contract.HostStartClaimInputBindingV3{}, contract.HostJournalV2{}, contract.ExactRefV1{}, refErr
	}
	if current.StartClaimRef != claimRef {
		return contract.HostStartClaimV1{}, contract.HostStartClaimInputBindingV3{}, contract.HostJournalV2{}, contract.ExactRefV1{}, contract.NewError(contract.ErrorConflict, "host_v3_journal_claim_drift", "HostV3 Journal binds another exact Start Claim")
	}
	return claim, binding, current, ref, nil
}

func inspectPhaseV3(p contract.HostPhaseV2) contract.HostInspectPhaseV3 {
	switch p {
	case contract.HostAcceptedV2:
		return contract.HostInspectClaimedV3
	case contract.HostReadyV2:
		return contract.HostInspectReadyV3
	case contract.HostDrainingV2, contract.HostReconcilingV2:
		return contract.HostInspectStoppingV3
	case contract.HostClosedV2:
		return contract.HostInspectClosedV3
	case contract.HostIndeterminateV2:
		return contract.HostInspectIndeterminateV3
	default:
		return contract.HostInspectStartingV3
	}
}
func (h *HostV3) nowV3(previous time.Time) (time.Time, error) {
	n := h.clock()
	if n.IsZero() || (!previous.IsZero() && n.Before(previous)) {
		return time.Time{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "HostV3 clock regressed")
	}
	return n, nil
}

func safeDeploymentCurrentV3(ctx context.Context, r ports.HostDeploymentCurrentReaderV1, ref contract.HostDeploymentCurrentRefV1) (v contract.HostDeploymentCurrentV1, err error) {
	defer func() {
		if x := recover(); x != nil {
			err = contract.NewError(contract.ErrorUnavailable, "host_v3_deployment_inspect_panic", fmt.Sprintf("HostV3 deployment Inspect panicked: %v", x))
		}
	}()
	return r.InspectHostDeploymentCurrentV1(ctx, ref)
}
func safeStartPipelineV3(ctx context.Context, p ports.HostV3OwnerPipeline, r contract.StartRequestV3, b contract.HostStartClaimInputBindingV3, j contract.HostJournalV2) (v contract.HostV3OwnerStartProjectionV1, err error) {
	defer func() {
		if x := recover(); x != nil {
			err = contract.NewError(contract.ErrorUnknownOutcome, "host_v3_pipeline_start_panic", fmt.Sprintf("HostV3 pipeline Start panicked: %v", x))
		}
	}()
	return p.StartOrInspectHostV3(ctx, r, b, j)
}
func safeInspectPipelineV3(ctx context.Context, p ports.HostV3OwnerPipeline, b contract.HostStartClaimInputBindingV3, j contract.HostJournalV2) (v contract.HostV3OwnerStartProjectionV1, err error) {
	defer func() {
		if x := recover(); x != nil {
			err = contract.NewError(contract.ErrorUnavailable, "host_v3_pipeline_inspect_panic", fmt.Sprintf("HostV3 pipeline Inspect panicked: %v", x))
		}
	}()
	return p.InspectHostV3(ctx, b, j)
}
func safeStopPipelineV3(ctx context.Context, p ports.HostV3OwnerPipeline, r contract.StopRequestV3, b contract.HostStartClaimInputBindingV3, j contract.HostJournalV2) (v contract.HostV3OwnerStopProjectionV1, err error) {
	defer func() {
		if x := recover(); x != nil {
			err = contract.NewError(contract.ErrorUnknownOutcome, "host_v3_pipeline_stop_panic", fmt.Sprintf("HostV3 pipeline Stop panicked: %v", x))
		}
	}()
	return p.StopOrInspectHostV3(ctx, r, b, j)
}
func safeInspectStopPipelineV3(ctx context.Context, p ports.HostV3OwnerPipeline, r contract.StopRequestV3, b contract.HostStartClaimInputBindingV3, j contract.HostJournalV2) (v contract.HostV3OwnerStopProjectionV1, err error) {
	defer func() {
		if x := recover(); x != nil {
			err = contract.NewError(contract.ErrorUnavailable, "host_v3_pipeline_stop_inspect_panic", fmt.Sprintf("HostV3 pipeline Stop Inspect panicked: %v", x))
		}
	}()
	return p.InspectStopHostV3(ctx, r, b, j)
}

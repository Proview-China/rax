package lifecycle

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
)

func (h *HostV2) StopV2(ctx context.Context, request contract.StopRequestV2) (contract.StopResultV2, error) {
	if h == nil {
		return contract.StopResultV2{}, contract.NewError(contract.ErrorUnavailable, "host_v2_missing", "HostV2 is unavailable")
	}
	if ctx == nil {
		return contract.StopResultV2{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	if contract.IsTypedNilV1(h.config.CleanupPlans) || contract.IsTypedNilV1(h.config.CleanupAttempts) || contract.IsTypedNilV1(h.config.CleanupNodes) {
		return contract.StopResultV2{}, contract.NewError(contract.ErrorUnavailable, "host_v2_cleanup_dependency_missing", "HostV2 cleanup Plan, Fact and Node ports are required")
	}
	now, err := h.freshNowV2(time.Time{})
	if err != nil {
		return contract.StopResultV2{}, err
	}
	if err = request.ValidateCurrent(now); err != nil {
		return contract.StopResultV2{}, err
	}
	current, err := h.config.Journal.InspectV2(ctx, request.HostID, request.StartID)
	if err != nil {
		return contract.StopResultV2{}, err
	}
	if current.StartClaimRef != request.StartClaimRef {
		return contract.StopResultV2{}, contract.NewError(contract.ErrorConflict, "host_v2_stop_claim_drift", "HostV2 Stop names another exact Start claim")
	}
	plan, err := h.config.CleanupPlans.InspectCleanupPlanV2(ctx, request.CleanupPlanRef)
	if err != nil {
		return contract.StopResultV2{}, err
	}
	planRef, err := plan.RefV2()
	if err != nil {
		return contract.StopResultV2{}, err
	}
	if planRef != request.CleanupPlanRef || plan.HostID != request.HostID || plan.StartID != request.StartID {
		return contract.StopResultV2{}, contract.NewError(contract.ErrorConflict, "host_v2_cleanup_plan_splice", "cleanup Plan drifted from exact Host/Start")
	}
	order, err := cleanupOrderV2(plan)
	if err != nil {
		return contract.StopResultV2{}, err
	}
	switch current.Phase {
	case contract.HostReadyV2:
		current, err = h.advanceStopPhaseV2(ctx, current, contract.HostDrainingV2)
	case contract.HostIndeterminateV2:
		current, err = h.advanceStopPhaseV2(ctx, current, contract.HostReconcilingV2)
	case contract.HostDrainingV2, contract.HostReconcilingV2, contract.HostClosedV2:
	default:
		return contract.StopResultV2{}, contract.NewError(contract.ErrorPrecondition, "host_v2_stop_not_ready", "HostV2 Stop requires Ready or an existing cleanup phase")
	}
	if err != nil {
		return contract.StopResultV2{}, err
	}

	completed := make(map[string]contract.CleanupAttemptV2, len(order))
	residuals := []contract.ExactRefV1{}
	for index, node := range order {
		barriers := make([]contract.ExactRefV1, 0, len(node.RequiredBarrierIDs))
		for _, dependency := range node.RequiredBarrierIDs {
			attempt, exists := completed[dependency]
			if !exists || attempt.State != contract.CleanupResultRecordedV2 || attempt.ResultRef == nil {
				return contract.StopResultV2{}, contract.NewError(contract.ErrorPrecondition, "cleanup_dependency_unsettled", "cleanup dependency is not exactly settled")
			}
			if attempt.ResultDisposition == contract.CleanupDispositionResidualV2 {
				return h.finishStopResidualV2(ctx, request, current, completed, residuals)
			}
			barriers = append(barriers, *attempt.ResultRef)
		}
		nodeRequest, sealErr := contract.SealCleanupNodeRequestV2(contract.CleanupNodeRequestV2{HostID: request.HostID, StartID: request.StartID, AttemptID: cleanupAttemptIDV2(plan, node), PlanRef: planRef, Node: node, PredecessorRevision: uint64(index + 1), BarrierCurrentRefs: barriers, RequestedNotAfterUnixNano: request.RequestedNotAfterUnixNano})
		if sealErr != nil {
			return contract.StopResultV2{}, sealErr
		}
		attempt, runErr := h.runCleanupNodeV2(ctx, current, nodeRequest)
		if runErr != nil {
			latest, inspectErr := h.config.Journal.InspectV2(context.WithoutCancel(ctx), request.HostID, request.StartID)
			if inspectErr == nil && latest.Phase != contract.HostIndeterminateV2 {
				_, _ = h.advanceStopPhaseV2(context.WithoutCancel(ctx), latest, contract.HostIndeterminateV2)
			}
			return contract.StopResultV2{}, runErr
		}
		completed[node.NodeID] = attempt
		if attempt.ResultDisposition == contract.CleanupDispositionResidualV2 {
			residuals = append(residuals, *attempt.ResultRef)
			return h.finishStopResidualV2(ctx, request, current, completed, residuals)
		}
	}
	current, err = h.config.Journal.InspectV2(ctx, request.HostID, request.StartID)
	if err != nil {
		return contract.StopResultV2{}, err
	}
	if current.Phase == contract.HostDrainingV2 {
		current, err = h.advanceStopPhaseV2(ctx, current, contract.HostReconcilingV2)
		if err != nil {
			return contract.StopResultV2{}, err
		}
	}
	if current.Phase == contract.HostReconcilingV2 {
		current, err = h.advanceStopPhaseV2(ctx, current, contract.HostClosedV2)
		if err != nil {
			return contract.StopResultV2{}, err
		}
	}
	return h.sealStopResultV2(request, current, completed, residuals)
}

func (h *HostV2) runCleanupNodeV2(ctx context.Context, journalValue contract.HostJournalV2, request contract.CleanupNodeRequestV2) (contract.CleanupAttemptV2, error) {
	operation, err := h.config.CleanupNodes.ResolveCleanupNodeOperationV2(
		ctx,
		request.Node.InspectPortBinding,
		request.Node.CleanupContractRef,
		request.Node.RequestSchemaDigest,
		request.Node.ResultSchemaDigest,
	)
	if err != nil {
		return contract.CleanupAttemptV2{}, err
	}
	if contract.IsTypedNilV1(operation) {
		return contract.CleanupAttemptV2{}, contract.NewError(contract.ErrorUnavailable, "cleanup_node_operation_missing", "cleanup node registry returned no typed operation")
	}
	current, inspectErr := h.config.CleanupAttempts.InspectCleanupAttemptV2(ctx, request.AttemptID)
	owned := false
	if inspectErr != nil {
		if !contract.HasCode(inspectErr, contract.ErrorNotFound) {
			return contract.CleanupAttemptV2{}, inspectErr
		}
		now, err := h.freshNowV2(time.Unix(0, journalValue.UpdatedUnixNano))
		if err != nil {
			return contract.CleanupAttemptV2{}, err
		}
		intent, err := contract.SealCleanupAttemptV2(contract.CleanupAttemptV2{ContractVersion: contract.CleanupContractVersionV2, AttemptID: request.AttemptID, Revision: 1, HostID: request.HostID, StartID: request.StartID, PlanRef: request.PlanRef, NodeID: request.Node.NodeID, RequestDigest: request.RequestDigest, PredecessorRevision: request.PredecessorRevision, BarrierCurrentRefs: request.BarrierCurrentRefs, State: contract.CleanupIntentRecordedV2, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()})
		if err != nil {
			return contract.CleanupAttemptV2{}, err
		}
		actual, createErr := safeCreateCleanupAttemptV2(ctx, h, intent)
		if createErr == nil && actual.Digest == intent.Digest {
			current, owned = actual, true
		} else {
			current, inspectErr = h.config.CleanupAttempts.InspectCleanupAttemptV2(context.WithoutCancel(ctx), request.AttemptID)
			if inspectErr != nil {
				return contract.CleanupAttemptV2{}, errors.Join(contract.NewError(contract.ErrorUnknownOutcome, "cleanup_intent_unknown", "cleanup intent outcome requires Inspect"), createErr, inspectErr)
			}
		}
	}
	if err := exactCleanupAttemptRequestV2(current, request); err != nil {
		return contract.CleanupAttemptV2{}, err
	}
	if current.State == contract.CleanupResultRecordedV2 {
		result, err := operation.InspectCleanupNodeV2(ctx, request)
		if err != nil {
			return contract.CleanupAttemptV2{}, err
		}
		if err = result.ValidateCurrent(request, h.config.Clock()); err != nil || current.ResultRef == nil || *current.ResultRef != result.ResultRef || current.ResultDisposition != result.Disposition {
			if err != nil {
				return contract.CleanupAttemptV2{}, err
			}
			return contract.CleanupAttemptV2{}, contract.NewError(contract.ErrorConflict, "cleanup_result_splice", "cleanup Owner result drifted from recorded attempt")
		}
		return current, nil
	}
	if !owned {
		result, err := operation.InspectCleanupNodeV2(context.WithoutCancel(ctx), request)
		if err != nil {
			_, markErr := h.markCleanupUnknownV2(context.WithoutCancel(ctx), current)
			return contract.CleanupAttemptV2{}, errors.Join(contract.NewError(contract.ErrorUnknownOutcome, "cleanup_reconciliation_required", "cleanup intent is permanently Inspect-only"), err, markErr)
		}
		return h.settleCleanupAttemptV2(context.WithoutCancel(ctx), current, request, result)
	}
	result, startErr := operation.StartOrInspectCleanupNodeV2(ctx, request)
	if startErr != nil {
		result, inspectErr = operation.InspectCleanupNodeV2(context.WithoutCancel(ctx), request)
		if inspectErr != nil {
			_, markErr := h.markCleanupUnknownV2(context.WithoutCancel(ctx), current)
			return contract.CleanupAttemptV2{}, errors.Join(contract.NewError(contract.ErrorUnknownOutcome, "cleanup_reconciliation_required", "cleanup Owner may have started"), startErr, inspectErr, markErr)
		}
	}
	return h.settleCleanupAttemptV2(context.WithoutCancel(ctx), current, request, result)
}

func (h *HostV2) settleCleanupAttemptV2(ctx context.Context, current contract.CleanupAttemptV2, request contract.CleanupNodeRequestV2, result contract.CleanupNodeResultV2) (contract.CleanupAttemptV2, error) {
	now, err := h.freshNowV2(time.Unix(0, current.UpdatedUnixNano))
	if err != nil {
		return contract.CleanupAttemptV2{}, err
	}
	if err = result.ValidateCurrent(request, now); err != nil {
		return contract.CleanupAttemptV2{}, err
	}
	next := current
	next.Revision++
	next.State = contract.CleanupResultRecordedV2
	next.ResultRef = &result.ResultRef
	next.ResultDisposition = result.Disposition
	next.UpdatedUnixNano = now.UnixNano()
	next.Digest = ""
	next, err = contract.SealCleanupAttemptV2(next)
	if err != nil {
		return contract.CleanupAttemptV2{}, err
	}
	if err = contract.ValidateCleanupAttemptSuccessorV2(current, next); err != nil {
		return contract.CleanupAttemptV2{}, err
	}
	expected := cleanupAttemptRefV2(current)
	actual, writeErr := safeCASSCleanupAttemptV2(ctx, h, expected, next)
	if writeErr == nil && actual.Digest == next.Digest {
		return actual, nil
	}
	inspected, inspectErr := h.config.CleanupAttempts.InspectCleanupAttemptV2(context.WithoutCancel(ctx), current.AttemptID)
	if inspectErr == nil && inspected.Digest == next.Digest {
		return inspected, nil
	}
	return contract.CleanupAttemptV2{}, errors.Join(contract.NewError(contract.ErrorUnknownOutcome, "cleanup_result_commit_unknown", "cleanup result CAS outcome is unknown"), writeErr, inspectErr)
}

func (h *HostV2) markCleanupUnknownV2(ctx context.Context, current contract.CleanupAttemptV2) (contract.CleanupAttemptV2, error) {
	for _, state := range []contract.CleanupAttemptStateV2{contract.CleanupOutcomeUnknownV2, contract.CleanupReconciliationRequiredV2} {
		if current.State == state {
			continue
		}
		now, err := h.freshNowV2(time.Unix(0, current.UpdatedUnixNano))
		if err != nil {
			return contract.CleanupAttemptV2{}, err
		}
		next := current
		next.Revision++
		next.State = state
		next.UpdatedUnixNano = now.UnixNano()
		next.Digest = ""
		next, err = contract.SealCleanupAttemptV2(next)
		if err != nil {
			return contract.CleanupAttemptV2{}, err
		}
		actual, writeErr := safeCASSCleanupAttemptV2(ctx, h, cleanupAttemptRefV2(current), next)
		if writeErr != nil {
			inspected, inspectErr := h.config.CleanupAttempts.InspectCleanupAttemptV2(ctx, current.AttemptID)
			if inspectErr != nil || inspected.Digest != next.Digest {
				return contract.CleanupAttemptV2{}, errors.Join(writeErr, inspectErr)
			}
		} else {
			current = actual
			continue
		}
		current = next
	}
	return current, nil
}

func (h *HostV2) advanceStopPhaseV2(ctx context.Context, current contract.HostJournalV2, phase contract.HostPhaseV2) (contract.HostJournalV2, error) {
	if current.Phase == phase {
		return current, nil
	}
	now, err := h.freshNowV2(time.Unix(0, current.UpdatedUnixNano))
	if err != nil {
		return contract.HostJournalV2{}, err
	}
	next := current
	next.Revision++
	next.Phase = phase
	next.UpdatedUnixNano = now.UnixNano()
	next.Digest = ""
	next, err = contract.SealHostJournalV2(next)
	if err != nil {
		return contract.HostJournalV2{}, err
	}
	return h.config.Journal.AdvanceV2(ctx, current, next)
}
func (h *HostV2) finishStopResidualV2(ctx context.Context, request contract.StopRequestV2, current contract.HostJournalV2, completed map[string]contract.CleanupAttemptV2, residuals []contract.ExactRefV1) (contract.StopResultV2, error) {
	latest, err := h.config.Journal.InspectV2(ctx, request.HostID, request.StartID)
	if err != nil {
		return contract.StopResultV2{}, err
	}
	if latest.Phase == contract.HostDrainingV2 {
		latest, err = h.advanceStopPhaseV2(ctx, latest, contract.HostReconcilingV2)
		if err != nil {
			return contract.StopResultV2{}, err
		}
	}
	return h.sealStopResultV2(request, latest, completed, residuals)
}
func (h *HostV2) sealStopResultV2(request contract.StopRequestV2, current contract.HostJournalV2, completed map[string]contract.CleanupAttemptV2, residuals []contract.ExactRefV1) (contract.StopResultV2, error) {
	now, err := h.freshNowV2(time.Unix(0, current.UpdatedUnixNano))
	if err != nil {
		return contract.StopResultV2{}, err
	}
	ref, err := current.RefV2()
	if err != nil {
		return contract.StopResultV2{}, err
	}
	attempts := make([]contract.CleanupAttemptV2, 0, len(completed))
	for _, attempt := range completed {
		attempts = append(attempts, attempt)
	}
	return contract.SealStopResultV2(contract.StopResultV2{HostID: request.HostID, StartID: request.StartID, RequestDigest: request.RequestDigest, Journal: ref, Phase: current.Phase, Attempts: attempts, Residuals: residuals, CheckedUnixNano: now.UnixNano()})
}

func cleanupOrderV2(plan contract.CleanupPlanV2) ([]contract.CleanupNodeV2, error) {
	nodes := map[string]contract.CleanupNodeV2{}
	indegree := map[string]int{}
	children := map[string][]string{}
	for _, node := range plan.Nodes {
		nodes[node.NodeID] = node
		indegree[node.NodeID] = len(node.RequiredBarrierIDs)
		for _, dep := range node.RequiredBarrierIDs {
			children[dep] = append(children[dep], node.NodeID)
		}
	}
	ready := []string{}
	for id, degree := range indegree {
		if degree == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)
	result := []contract.CleanupNodeV2{}
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		result = append(result, nodes[id])
		sort.Strings(children[id])
		for _, child := range children[id] {
			indegree[child]--
			if indegree[child] == 0 {
				ready = append(ready, child)
				sort.Strings(ready)
			}
		}
	}
	if len(result) != len(nodes) {
		return nil, contract.NewError(contract.ErrorConflict, "cleanup_dependency_cycle", "cleanup Plan contains a cycle")
	}
	return result, nil
}
func cleanupAttemptIDV2(plan contract.CleanupPlanV2, node contract.CleanupNodeV2) string {
	return "cleanup/" + plan.PlanID + "/" + node.NodeID
}
func cleanupAttemptRefV2(value contract.CleanupAttemptV2) contract.ExactRefV1 {
	return contract.ExactRefV1{Kind: "praxis.agent-host/cleanup-attempt-v2", ID: value.AttemptID, Revision: value.Revision, Digest: value.Digest}
}
func exactCleanupAttemptRequestV2(value contract.CleanupAttemptV2, request contract.CleanupNodeRequestV2) error {
	if err := value.Validate(); err != nil {
		return err
	}
	if value.HostID != request.HostID || value.StartID != request.StartID || value.PlanRef != request.PlanRef || value.NodeID != request.Node.NodeID || value.RequestDigest != request.RequestDigest || value.PredecessorRevision != request.PredecessorRevision || len(value.BarrierCurrentRefs) != len(request.BarrierCurrentRefs) {
		return contract.NewError(contract.ErrorConflict, "cleanup_attempt_request_splice", "cleanup attempt drifted from exact node request")
	}
	for i := range value.BarrierCurrentRefs {
		if value.BarrierCurrentRefs[i] != request.BarrierCurrentRefs[i] {
			return contract.NewError(contract.ErrorConflict, "cleanup_attempt_barrier_splice", "cleanup attempt barrier refs drifted")
		}
	}
	return nil
}
func safeCreateCleanupAttemptV2(ctx context.Context, h *HostV2, value contract.CleanupAttemptV2) (result contract.CleanupAttemptV2, err error) {
	defer func() {
		if recover() != nil {
			result = contract.CleanupAttemptV2{}
			err = contract.NewError(contract.ErrorUnknownOutcome, "cleanup_attempt_create_panic", "cleanup attempt create outcome is unknown")
		}
	}()
	return h.config.CleanupAttempts.CreateCleanupAttemptV2(ctx, value)
}
func safeCASSCleanupAttemptV2(ctx context.Context, h *HostV2, expected contract.ExactRefV1, next contract.CleanupAttemptV2) (result contract.CleanupAttemptV2, err error) {
	defer func() {
		if recover() != nil {
			result = contract.CleanupAttemptV2{}
			err = contract.NewError(contract.ErrorUnknownOutcome, "cleanup_attempt_cas_panic", "cleanup attempt CAS outcome is unknown")
		}
	}()
	return h.config.CleanupAttempts.CompareAndSwapCleanupAttemptV2(ctx, expected, next)
}

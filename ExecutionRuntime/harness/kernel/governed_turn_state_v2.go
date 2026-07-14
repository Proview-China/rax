package kernel

import (
	"context"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type GovernedTurnStateCoordinatorV2 struct {
	Sessions harnessports.SessionFactPortV2
}

var _ harnessports.GovernedTurnStatePortV2 = (*GovernedTurnStateCoordinatorV2)(nil)

func (c *GovernedTurnStateCoordinatorV2) AttachPreparedAttemptV2(ctx context.Context, request harnessports.AttachPreparedAttemptRequestV2) (contract.GovernedSessionV2, error) {
	if err := c.validate(); err != nil {
		return contract.GovernedSessionV2{}, err
	}
	if request.ExpectedSessionRevision == 0 || request.UpdatedUnixNano <= 0 || strings.TrimSpace(request.SessionID) == "" || request.Attempt.Observation != nil {
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "prepared attempt attachment is incomplete or already observed")
	}
	if err := request.Candidate.Validate(); err != nil {
		return contract.GovernedSessionV2{}, err
	}
	if err := request.Attempt.ValidatePrepared(); err != nil {
		return contract.GovernedSessionV2{}, err
	}
	current, err := c.Sessions.InspectSessionV2(ctx, request.Run, request.SessionID)
	if err != nil {
		return contract.GovernedSessionV2{}, err
	}
	if current.Revision == request.ExpectedSessionRevision+1 && current.Phase == contract.SessionModelInFlightV2 && current.Candidate != nil && *current.Candidate == request.Candidate && sameDomainReservationRefV3(current.DomainReservation, &request.Reservation) && sameExecutionAttemptRefsV2(current.Execution, &request.Attempt) && current.UpdatedUnixNano == request.UpdatedUnixNano {
		return current, nil
	}
	if current.Revision != request.ExpectedSessionRevision || current.Phase != contract.SessionModelDispatchReservedV2 || current.Candidate == nil || *current.Candidate != request.Candidate || !sameDomainReservationRefV3(current.DomainReservation, &request.Reservation) {
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidState, "session is not at the exact reserved candidate revision")
	}
	next := current
	next.Revision++
	next.Phase = contract.SessionModelInFlightV2
	attempt := request.Attempt
	next.Execution = &attempt
	next.UpdatedUnixNano = request.UpdatedUnixNano
	return c.commitExactV2(ctx, request.Run, current, next)
}

func (c *GovernedTurnStateCoordinatorV2) MarkAttemptReconcilingV2(ctx context.Context, request harnessports.MarkAttemptReconcilingRequestV2) (contract.GovernedSessionV2, error) {
	if err := c.validate(); err != nil {
		return contract.GovernedSessionV2{}, err
	}
	if err := request.Run.Validate(); err != nil {
		return contract.GovernedSessionV2{}, err
	}
	if strings.TrimSpace(request.SessionID) == "" || len(request.SessionID) > contract.MaxReferenceBytes || request.ExpectedSessionRevision == 0 || request.UpdatedUnixNano <= 0 {
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "reconciling attempt identity, revision and update time are required")
	}
	current, err := c.Sessions.InspectSessionV2(ctx, request.Run, request.SessionID)
	if err != nil {
		return contract.GovernedSessionV2{}, err
	}
	if current.Revision == request.ExpectedSessionRevision+1 && current.Phase == contract.SessionReconcilingV2 && current.UpdatedUnixNano == request.UpdatedUnixNano {
		return current, nil
	}
	if current.Revision != request.ExpectedSessionRevision || current.Phase != contract.SessionModelInFlightV2 || current.Execution == nil || current.Execution.Observation != nil {
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidState, "only an exact unresolved in-flight attempt may reconcile")
	}
	next := current
	next.Revision++
	next.Phase = contract.SessionReconcilingV2
	next.UpdatedUnixNano = request.UpdatedUnixNano
	return c.commitExactV2(ctx, request.Run, current, next)
}

func (c *GovernedTurnStateCoordinatorV2) AttachObservedAttemptV2(ctx context.Context, request harnessports.AttachObservedAttemptRequestV2) (contract.GovernedSessionV2, error) {
	if err := c.validate(); err != nil {
		return contract.GovernedSessionV2{}, err
	}
	if request.ExpectedSessionRevision == 0 || request.UpdatedUnixNano <= 0 || request.Attempt.Observation == nil || request.Attempt.Settlement != nil {
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "observed attempt attachment is incomplete")
	}
	if err := request.Attempt.ValidatePrepared(); err != nil {
		return contract.GovernedSessionV2{}, err
	}
	current, err := c.Sessions.InspectSessionV2(ctx, request.Run, request.SessionID)
	if err != nil {
		return contract.GovernedSessionV2{}, err
	}
	if current.Revision == request.ExpectedSessionRevision+1 && observedReplayMatchesV2(current, request) {
		return current, nil
	}
	if current.Revision != request.ExpectedSessionRevision || current.Phase != contract.SessionModelInFlightV2 && current.Phase != contract.SessionReconcilingV2 || current.Execution == nil || !samePreparedAttemptRefsV2(*current.Execution, request.Attempt) {
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "provider Observation does not descend from the current exact attempt")
	}
	next := current
	next.Revision++
	next.Phase = contract.SessionWaitingSettlementV2
	attempt := request.Attempt
	next.Execution = &attempt
	next.UpdatedUnixNano = request.UpdatedUnixNano
	return c.commitExactV2(ctx, request.Run, current, next)
}

func observedReplayMatchesV2(current contract.GovernedSessionV2, request harnessports.AttachObservedAttemptRequestV2) bool {
	return current.Phase == contract.SessionWaitingSettlementV2 && sameExecutionAttemptRefsV2(current.Execution, &request.Attempt) && current.UpdatedUnixNano == request.UpdatedUnixNano
}

// ApplySettledTurnV2 is the only non-cancellation path from an observed model
// turn to Action/Input/terminal. Provider observations and Application
// callers cannot choose the target state; the exact Runtime Settlement and
// its schema-bound DomainResult determine it.
func (c *GovernedTurnStateCoordinatorV2) ApplySettledTurnV2(ctx context.Context, request harnessports.ApplySettledTurnRequestV2) (contract.GovernedSessionV2, error) {
	if err := c.validate(); err != nil {
		return contract.GovernedSessionV2{}, err
	}
	if request.ExpectedSessionRevision == 0 || request.UpdatedUnixNano <= 0 || request.Attempt.Settlement == nil {
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonSettlementOwnerMismatch, "settled turn attachment is incomplete")
	}
	if err := request.Attempt.ValidatePrepared(); err != nil {
		return contract.GovernedSessionV2{}, err
	}
	settlement := request.Attempt.Settlement
	if settlement.DomainResultSchema == nil || *settlement.DomainResultSchema != request.DomainResult.Schema || settlement.DomainResultDigest != request.DomainResult.ContentDigest {
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "settled turn DomainResult differs from the exact Runtime Settlement")
	}
	result, err := contract.DecodeSettledTurnDomainResultV2(request.DomainResult)
	if err != nil {
		return contract.GovernedSessionV2{}, err
	}
	if settlement.Disposition != runtimeports.OperationSettlementAppliedV3 && result.State != contract.SettledTurnFailedV2 {
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonSettlementOwnerMismatch, "not-applied or failed settlement must produce an explicit failed turn result")
	}
	current, err := c.Sessions.InspectSessionV2(ctx, request.Run, request.SessionID)
	if err != nil {
		return contract.GovernedSessionV2{}, err
	}
	if current.Revision == request.ExpectedSessionRevision+1 {
		expected, buildErr := settledTurnTargetV2(current, request, result)
		if buildErr == nil && sameGovernedSessionV2(current, expected) {
			return current, nil
		}
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "settled turn replay differs from the exact committed session")
	}
	if current.Revision != request.ExpectedSessionRevision || current.Candidate == nil || *current.Candidate != result.Candidate || current.Execution == nil || !settlementDescendsFromSessionV2(current, request.Attempt) {
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "settled turn does not descend from the current exact observation")
	}
	next, err := settledTurnTargetV2(current, request, result)
	if err != nil {
		return contract.GovernedSessionV2{}, err
	}
	return c.commitExactV2(ctx, request.Run, current, next)
}

// ApplyUndispatchedSettlementV2 closes a model turn that Runtime proved never
// reached provider Prepare. It cannot produce completion, action or input.
func (c *GovernedTurnStateCoordinatorV2) ApplyUndispatchedSettlementV2(ctx context.Context, request harnessports.ApplyUndispatchedSettlementRequestV2) (contract.GovernedSessionV2, error) {
	if err := c.validate(); err != nil {
		return contract.GovernedSessionV2{}, err
	}
	if request.ExpectedSessionRevision == 0 || request.UpdatedUnixNano <= 0 || strings.TrimSpace(request.SessionID) == "" {
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "undispatched settlement attachment is incomplete")
	}
	if err := request.Candidate.Validate(); err != nil {
		return contract.GovernedSessionV2{}, err
	}
	if err := request.Settlement.Validate(); err != nil {
		return contract.GovernedSessionV2{}, err
	}
	result, err := contract.DecodeSettledTurnDomainResultV2(request.DomainResult)
	if err != nil {
		return contract.GovernedSessionV2{}, err
	}
	if result.Candidate != request.Candidate || result.State != contract.SettledTurnFailedV2 || request.Settlement.Observation != nil || request.Settlement.Attempt.Delegation != nil || request.Settlement.Disposition == runtimeports.OperationSettlementAppliedV3 || request.Settlement.DomainResultSchema == nil || *request.Settlement.DomainResultSchema != request.DomainResult.Schema || request.Settlement.DomainResultDigest != request.DomainResult.ContentDigest {
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorConflict, core.ReasonEffectUnknownOutcome, "undispatched settlement must be an exact failed result without provider sidecars")
	}
	current, err := c.Sessions.InspectSessionV2(ctx, request.Run, request.SessionID)
	if err != nil {
		return contract.GovernedSessionV2{}, err
	}
	next := current
	next.Revision = request.ExpectedSessionRevision + 1
	next.Phase = contract.SessionTerminalV2
	next.Candidate = nil
	next.DomainReservation = nil
	next.Execution = nil
	next.PendingAction, next.PendingInput = nil, nil
	next.UndispatchedSettlement = &contract.UndispatchedSettlementBindingV2{
		Candidate: request.Candidate, Settlement: request.Settlement,
		DomainResultSchema: request.DomainResult.Schema, DomainResultDigest: request.DomainResult.ContentDigest,
	}
	next.CompletionClaim = contract.ClaimFailed
	next.UpdatedUnixNano = request.UpdatedUnixNano
	if current.Revision == request.ExpectedSessionRevision+1 {
		if sameGovernedSessionV2(current, next) {
			return current, nil
		}
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "undispatched settlement replay differs from committed Session")
	}
	if current.Revision != request.ExpectedSessionRevision || current.Phase != contract.SessionModelDispatchReservedV2 || current.Candidate == nil || *current.Candidate != request.Candidate || current.DomainReservation == nil || current.Execution != nil || current.UndispatchedSettlement != nil {
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorConflict, core.ReasonEffectUnknownOutcome, "undispatched settlement does not descend from the exact reserved candidate")
	}
	if err := next.Validate(); err != nil {
		return contract.GovernedSessionV2{}, err
	}
	return c.commitExactV2(ctx, request.Run, current, next)
}

func settlementDescendsFromSessionV2(current contract.GovernedSessionV2, attempt runtimeports.GovernedExecutionAttemptRefsV2) bool {
	switch current.Phase {
	case contract.SessionWaitingSettlementV2:
		return attempt.Observation != nil && sameObservedAttemptRefsV2(*current.Execution, attempt)
	case contract.SessionReconcilingV2:
		// A separately governed Inspect may settle an unknown attempt without
		// recreating the missing provider Observation.
		return attempt.Observation == nil && attempt.Settlement != nil && attempt.Settlement.Observation == nil && samePreparedAttemptRefsV2(*current.Execution, attempt)
	default:
		return false
	}
}

func settledTurnTargetV2(current contract.GovernedSessionV2, request harnessports.ApplySettledTurnRequestV2, result contract.SettledTurnResultV2) (contract.GovernedSessionV2, error) {
	next := current
	next.Revision = request.ExpectedSessionRevision + 1
	next.Candidate = nil
	next.DomainReservation = nil
	attempt := request.Attempt
	next.Execution = &attempt
	next.PendingAction, next.PendingInput, next.CompletionClaim = nil, nil, ""
	next.UpdatedUnixNano = request.UpdatedUnixNano
	switch result.State {
	case contract.SettledTurnActionRequiredV2:
		next.Phase, next.PendingAction = contract.SessionWaitingActionV2, result.Action
	case contract.SettledTurnInputRequiredV2:
		next.Phase, next.PendingInput = contract.SessionWaitingInputV2, result.Input
	case contract.SettledTurnCompletedV2:
		next.Phase, next.CompletionClaim = contract.SessionTerminalV2, contract.ClaimCompleted
	case contract.SettledTurnFailedV2:
		next.Phase, next.CompletionClaim = contract.SessionTerminalV2, contract.ClaimFailed
	default:
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "settled turn result state is unknown")
	}
	if err := next.Validate(); err != nil {
		return contract.GovernedSessionV2{}, err
	}
	return next, nil
}

func (c *GovernedTurnStateCoordinatorV2) commitExactV2(ctx context.Context, run contract.RunRef, current, next contract.GovernedSessionV2) (contract.GovernedSessionV2, error) {
	stored, err := c.Sessions.CompareAndSwapSessionV2(ctx, harnessports.SessionCASRequestV2{ExpectedRevision: current.Revision, Next: next})
	if err != nil {
		if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorConflict) {
			return contract.GovernedSessionV2{}, err
		}
		stored, err = c.Sessions.InspectSessionV2(context.WithoutCancel(ctx), run, current.ID)
		if err != nil {
			return contract.GovernedSessionV2{}, err
		}
	}
	if !sameGovernedSessionV2(stored, next) {
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Harness turn CAS recovery differs from requested exact state")
	}
	return stored, nil
}

func (c *GovernedTurnStateCoordinatorV2) validate() error {
	if c == nil || c.Sessions == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Harness Session Fact Port is required")
	}
	return nil
}

func sameDomainReservationRefV3(left, right *contract.ModelDispatchReservationRefV2) bool {
	return left != nil && right != nil && *left == *right
}

func samePreparedAttemptRefsV2(left, right runtimeports.GovernedExecutionAttemptRefsV2) bool {
	left.Observation, right.Observation = nil, nil
	left.Settlement, right.Settlement = nil, nil
	ld, le := core.CanonicalJSONDigest("praxis.harness.governed", contract.GovernedContractVersionV2, "PreparedExecutionAttemptRefsV2", left)
	rd, re := core.CanonicalJSONDigest("praxis.harness.governed", contract.GovernedContractVersionV2, "PreparedExecutionAttemptRefsV2", right)
	return le == nil && re == nil && ld == rd
}

func sameObservedAttemptRefsV2(left, right runtimeports.GovernedExecutionAttemptRefsV2) bool {
	left.Settlement, right.Settlement = nil, nil
	ld, le := core.CanonicalJSONDigest("praxis.harness.governed", contract.GovernedContractVersionV2, "ObservedExecutionAttemptRefsV2", left)
	rd, re := core.CanonicalJSONDigest("praxis.harness.governed", contract.GovernedContractVersionV2, "ObservedExecutionAttemptRefsV2", right)
	return le == nil && re == nil && ld == rd
}

func sameExecutionAttemptRefsV2(left, right *runtimeports.GovernedExecutionAttemptRefsV2) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	ld, le := core.CanonicalJSONDigest("praxis.harness.governed", contract.GovernedContractVersionV2, "ExecutionAttemptRefsV2", left)
	rd, re := core.CanonicalJSONDigest("praxis.harness.governed", contract.GovernedContractVersionV2, "ExecutionAttemptRefsV2", right)
	return le == nil && re == nil && ld == rd
}

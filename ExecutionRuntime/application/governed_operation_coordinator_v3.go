package application

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type GovernedOperationCoordinatorConfigV3 struct {
	Attempts       applicationports.GovernedOperationAttemptFactPortV3
	Journals       applicationports.WorkflowJournalFactPortV2
	Admission      runtimeports.OperationEffectAdmissionPortV3
	Governance     runtimeports.OperationGovernancePortV3
	Delegations    runtimeports.ExecutionDelegationGovernancePortV2
	Execution      runtimeports.GovernedExecutionPortV2
	Observations   runtimeports.OperationObservationGovernancePortV3
	Settlements    runtimeports.OperationSettlementGovernancePortV3
	DomainResolver applicationports.OperationDomainResolverV3
	Clock          func() time.Time
}

type GovernedOperationCoordinatorV3 struct {
	config     GovernedOperationCoordinatorConfigV3
	journal    *JournalCoordinatorV2
	completion *GovernedJournalCompletionV3
}

func NewGovernedOperationCoordinatorV3(config GovernedOperationCoordinatorConfigV3) (*GovernedOperationCoordinatorV3, error) {
	if config.Attempts == nil || config.Journals == nil || config.Admission == nil || config.Governance == nil || config.Delegations == nil || config.Execution == nil || config.Observations == nil || config.Settlements == nil || config.DomainResolver == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "governed operation coordinator requires attempt, journal, governance, execution, observation, settlement and domain ports")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	journal, err := NewJournalCoordinatorV2(JournalCoordinatorConfigV2{Facts: config.Journals, Clock: config.Clock})
	if err != nil {
		return nil, err
	}
	completion, err := NewGovernedJournalCompletionV3(GovernedJournalCompletionConfigV3{Attempts: config.Attempts, Journals: config.Journals, Clock: config.Clock})
	if err != nil {
		return nil, err
	}
	return &GovernedOperationCoordinatorV3{config: config, journal: journal, completion: completion}, nil
}

type StartGovernedOperationRequestV3 struct {
	Plan    contract.WorkflowPlanV2                 `json:"plan"`
	Attempt contract.GovernedOperationAttemptFactV3 `json:"attempt"`
}

type ResumeGovernedOperationRequestV3 struct {
	Plan      contract.WorkflowPlanV2 `json:"plan"`
	AttemptID string                  `json:"attempt_id"`
}

type SettleGovernedOperationRequestV3 struct {
	Plan       contract.WorkflowPlanV2                      `json:"plan"`
	AttemptID  string                                       `json:"attempt_id"`
	Submission runtimeports.OperationSettlementSubmissionV3 `json:"submission"`
}

type GovernedOperationResultV3 struct {
	Attempt contract.GovernedOperationAttemptFactV3     `json:"attempt"`
	Journal contract.WorkflowJournalV2                  `json:"journal"`
	Domain  *applicationports.OperationDomainStateRefV3 `json:"domain,omitempty"`
}

// StartGovernedOperationV3 persists the immutable attempt and the workflow
// write-ahead reference before any Runtime gateway or provider may be called.
func (c *GovernedOperationCoordinatorV3) StartGovernedOperationV3(ctx context.Context, request StartGovernedOperationRequestV3) (GovernedOperationResultV3, error) {
	if err := request.Plan.Validate(c.config.Clock()); err != nil {
		return GovernedOperationResultV3{}, err
	}
	if err := request.Attempt.Validate(); err != nil {
		return GovernedOperationResultV3{}, err
	}
	if request.Attempt.State != contract.OperationIntentRecordedV3 || request.Attempt.Revision != 1 || !runtimeports.SameExecutionScopeV2(request.Plan.Target, request.Attempt.Scope) {
		return GovernedOperationResultV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "new governed operation must be intent_recorded revision one in the workflow scope")
	}
	current, err := c.inspectOrCreateAttemptV3(ctx, request.Attempt)
	if err != nil {
		return GovernedOperationResultV3{}, err
	}
	if !sameAttemptOriginV3(current, request.Attempt) {
		return GovernedOperationResultV3{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "operation attempt recovery binds different immutable input")
	}
	if _, err := c.ensureJournalDispatchIntentV3(ctx, request.Plan, request.Attempt, current); err != nil {
		return GovernedOperationResultV3{}, err
	}
	return c.resumeV3(ctx, request.Plan, current.ID)
}

func (c *GovernedOperationCoordinatorV3) ResumeGovernedOperationV3(ctx context.Context, request ResumeGovernedOperationRequestV3) (GovernedOperationResultV3, error) {
	if err := request.Plan.Validate(time.Time{}); err != nil {
		return GovernedOperationResultV3{}, err
	}
	return c.resumeV3(ctx, request.Plan, request.AttemptID)
}

func (c *GovernedOperationCoordinatorV3) resumeV3(ctx context.Context, plan contract.WorkflowPlanV2, attemptID string) (GovernedOperationResultV3, error) {
	anchored, err := c.config.Attempts.InspectGovernedOperationAttemptV3(ctx, plan.Target, attemptID)
	if err != nil {
		return GovernedOperationResultV3{}, err
	}
	if err := validateOperationPlanBindingV3(plan, anchored); err != nil {
		return GovernedOperationResultV3{}, err
	}
	if _, err := c.verifyJournalDispatchIntentV3(ctx, plan, initialOperationAttemptV3(anchored), anchored); err != nil {
		return GovernedOperationResultV3{}, err
	}
	for range 20 {
		current, err := c.config.Attempts.InspectGovernedOperationAttemptV3(ctx, plan.Target, attemptID)
		if err != nil {
			return GovernedOperationResultV3{}, err
		}
		if err := current.Validate(); err != nil {
			return GovernedOperationResultV3{}, err
		}
		if err := validateOperationPlanBindingV3(plan, current); err != nil {
			return GovernedOperationResultV3{}, err
		}
		switch current.State {
		case contract.OperationIntentRecordedV3:
			if err := plan.Validate(c.config.Clock()); err != nil {
				return GovernedOperationResultV3{}, err
			}
			reservation, err := c.reserveDomainIntentV3(ctx, current)
			if err != nil {
				return GovernedOperationResultV3{}, err
			}
			next := nextOperationAttemptV3(current, contract.OperationDomainReservedV3, c.config.Clock())
			next.DomainReservation = &reservation
			if _, err := c.commitAttemptV3(ctx, current, next); err != nil {
				return GovernedOperationResultV3{}, err
			}

		case contract.OperationDomainReservedV3:
			if err := plan.Validate(c.config.Clock()); err != nil {
				return GovernedOperationResultV3{}, err
			}
			if err := c.verifyDomainReservationV3(ctx, current); err != nil {
				return GovernedOperationResultV3{}, err
			}
			receipt, err := c.config.Admission.InspectAcceptedOperationEffectV3(ctx, current.Operation, current.Intent.EffectID)
			if core.HasCategory(err, core.ErrorNotFound) {
				receipt, err = c.config.Admission.AdmitOperationEffectV3(ctx, current.IntentValue)
				if err != nil {
					receipt, err = c.config.Admission.InspectAcceptedOperationEffectV3(context.WithoutCancel(ctx), current.Operation, current.Intent.EffectID)
				}
			}
			if err != nil {
				return GovernedOperationResultV3{}, err
			}
			intentDigest, _ := current.IntentValue.DigestV3()
			if receipt.OperationDigest != current.Intent.OperationDigest || receipt.EffectID != current.Intent.EffectID || receipt.IntentRevision != current.Intent.IntentRevision || receipt.IntentDigest != intentDigest {
				return GovernedOperationResultV3{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "admission recovery returned another operation Intent")
			}
			next := nextOperationAttemptV3(current, contract.OperationEffectAdmittedV3, c.config.Clock())
			next.Admission = &receipt
			if _, err := c.commitAttemptV3(ctx, current, next); err != nil {
				return GovernedOperationResultV3{}, err
			}

		case contract.OperationEffectAdmittedV3:
			if err := plan.Validate(c.config.Clock()); err != nil {
				return GovernedOperationResultV3{}, err
			}
			if err := c.verifyDomainReservationV3(ctx, current); err != nil {
				return GovernedOperationResultV3{}, err
			}
			authorization, err := c.inspectAuthorizationV3(ctx, current)
			if core.HasCategory(err, core.ErrorNotFound) {
				authorization, err = c.config.Governance.IssueOperationDispatchV3(ctx, runtimeports.IssueGovernedOperationDispatchRequestV3{
					Operation: current.Operation, EffectID: current.Intent.EffectID, ExpectedEffectRevision: current.Admission.FactRevision,
					PermitID: current.DispatchPlan.PermitID, AttemptID: current.DispatchPlan.AttemptID, PermitTTL: time.Duration(current.DispatchPlan.PermitTTLNanos),
				})
				if err != nil {
					authorization, err = c.inspectAuthorizationV3(context.WithoutCancel(ctx), current)
				}
			}
			if err != nil {
				return GovernedOperationResultV3{}, err
			}
			issued, err := issuedAuthorizationProjectionV3(authorization)
			if err != nil {
				return GovernedOperationResultV3{}, err
			}
			next := nextOperationAttemptV3(current, contract.OperationPermitIssuedV3, c.config.Clock())
			next.IssuedAuthorization = &issued
			if _, err := c.commitAttemptV3(ctx, current, next); err != nil {
				return GovernedOperationResultV3{}, err
			}

		case contract.OperationPermitIssuedV3:
			if err := plan.Validate(c.config.Clock()); err != nil {
				return GovernedOperationResultV3{}, err
			}
			if err := c.verifyDomainReservationV3(ctx, current); err != nil {
				return GovernedOperationResultV3{}, err
			}
			authorization, err := c.inspectAuthorizationV3(ctx, current)
			if err == nil && authorization.State == runtimeports.OperationDispatchAuthorizationIssuedV3 {
				authorization, err = c.config.Governance.BeginOperationDispatchV3(ctx, runtimeports.BeginGovernedOperationDispatchRequestV3{
					Operation: current.Operation, EffectID: current.Intent.EffectID, ExpectedEffectRevision: current.IssuedAuthorization.EffectFactRevision,
					PermitID: current.DispatchPlan.PermitID, ExpectedPermitRevision: current.IssuedAuthorization.PermitFactRevision,
				})
				if err != nil {
					authorization, err = c.inspectAuthorizationV3(context.WithoutCancel(ctx), current)
				}
			}
			if err != nil {
				return GovernedOperationResultV3{}, err
			}
			if authorization.State == runtimeports.OperationDispatchAuthorizationUnknownV3 {
				next := nextOperationAttemptV3(current, contract.OperationDispatchUnknownV3, c.config.Clock())
				next.UnknownAuthorization = &authorization
				stored, commitErr := c.commitAttemptV3(ctx, current, next)
				if commitErr != nil {
					return GovernedOperationResultV3{}, commitErr
				}
				return c.finishUnknownV3(ctx, plan, stored)
			}
			if authorization.State != runtimeports.OperationDispatchAuthorizationBegunV3 {
				return GovernedOperationResultV3{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "operation Begin recovery is not begun")
			}
			next := nextOperationAttemptV3(current, contract.OperationPermitBegunV3, c.config.Clock())
			next.BegunAuthorization = &authorization
			if _, err := c.commitAttemptV3(ctx, current, next); err != nil {
				return GovernedOperationResultV3{}, err
			}

		case contract.OperationPermitBegunV3:
			if err := c.verifyDomainReservationV3(ctx, current); err != nil {
				return GovernedOperationResultV3{}, err
			}
			delegation, err := deriveExecutionDelegationV3(current, c.config.Clock())
			if err != nil {
				return GovernedOperationResultV3{}, err
			}
			declared, err := c.config.Delegations.InspectDeclaredExecutionV2(ctx, current.Operation, current.DelegationPlan.DelegationID)
			if core.HasCategory(err, core.ErrorNotFound) {
				declared, err = c.config.Delegations.DeclareExecutionDelegationV2(ctx, runtimeports.DeclareExecutionDelegationRequestV2{Delegation: delegation, Intent: current.IntentValue, Permit: current.BegunAuthorization.Permit, Fence: current.BegunAuthorization.Fence})
				if err != nil {
					declared, err = c.config.Delegations.InspectDeclaredExecutionV2(context.WithoutCancel(ctx), current.Operation, current.DelegationPlan.DelegationID)
				}
			}
			if err != nil {
				return GovernedOperationResultV3{}, err
			}
			expected, _ := delegation.RefV2()
			if declared != expected {
				return GovernedOperationResultV3{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "delegation gateway returned another declaration")
			}
			next := nextOperationAttemptV3(current, contract.OperationDelegationDeclaredV3, c.config.Clock())
			next.DelegationFact, next.DeclaredDelegation = &delegation, &declared
			if _, err := c.commitAttemptV3(ctx, current, next); err != nil {
				return GovernedOperationResultV3{}, err
			}

		case contract.OperationDelegationDeclaredV3:
			if err := c.verifyDomainReservationV3(ctx, current); err != nil {
				return GovernedOperationResultV3{}, err
			}
			prepared, uncertain, err := c.prepareAndCommitV3(ctx, current)
			if uncertain {
				unknown, unknownErr := c.markUnknownV3(ctx, current)
				if unknownErr != nil {
					return GovernedOperationResultV3{}, unknownErr
				}
				if unknown.State != contract.OperationDispatchUnknownV3 {
					continue
				}
				return c.finishUnknownV3(ctx, plan, unknown)
			}
			if err != nil {
				return GovernedOperationResultV3{}, err
			}
			next := nextOperationAttemptV3(current, contract.OperationExecutionPreparedV3, c.config.Clock())
			next.PreparedDelegation, next.Prepared, next.Enforcement = &prepared.Delegation, &prepared.Prepared, &prepared.Enforcement
			if _, err := c.commitAttemptV3(ctx, current, next); err != nil {
				return GovernedOperationResultV3{}, err
			}

		case contract.OperationExecutionPreparedV3:
			if err := c.verifyDomainReservationV3(ctx, current); err != nil {
				return GovernedOperationResultV3{}, err
			}
			domain, err := c.ensureDomainPreparedV3(ctx, current)
			if err != nil {
				return GovernedOperationResultV3{}, err
			}
			if domain.State != applicationports.OperationDomainPreparedV3 {
				// A concurrent coordinator has already crossed the domain
				// dispatch boundary. Re-read the Application attempt; never
				// issue another provider Execute from this stale watermark.
				continue
			}
			observation, uncertain, err := c.executeOrInspectV3(ctx, current)
			if err != nil && !uncertain {
				return GovernedOperationResultV3{}, err
			}
			if uncertain || observation.State != runtimeports.ProviderAttemptObservedV2 {
				unknown, unknownErr := c.markUnknownV3(ctx, current)
				if unknownErr != nil {
					return GovernedOperationResultV3{}, unknownErr
				}
				if unknown.State != contract.OperationDispatchUnknownV3 {
					continue
				}
				return c.finishUnknownV3(ctx, plan, unknown)
			}
			observed, err := c.recordObservationV3(ctx, current, observation)
			if err != nil {
				return GovernedOperationResultV3{}, err
			}
			next := nextOperationAttemptV3(current, contract.OperationProviderObservedV3, c.config.Clock())
			next.Observation = &observed
			stored, err := c.commitAttemptV3(ctx, current, next)
			if err != nil {
				return GovernedOperationResultV3{}, err
			}
			if stored.State != contract.OperationProviderObservedV3 {
				continue
			}
			_ = domain
			return c.finishObservedV3(ctx, plan, stored)

		case contract.OperationProviderObservedV3:
			return c.finishObservedV3(ctx, plan, current)

		case contract.OperationDispatchUnknownV3:
			return c.finishUnknownV3(ctx, plan, current)

		case contract.OperationSettledV3:
			return c.finishSettlementV3(ctx, plan, current)
		}
	}
	return GovernedOperationResultV3{}, core.NewError(core.ErrorInternal, core.ReasonInvalidState, "governed operation exceeded bounded recovery stages")
}

func (c *GovernedOperationCoordinatorV3) SettleGovernedOperationV3(ctx context.Context, request SettleGovernedOperationRequestV3) (GovernedOperationResultV3, error) {
	if err := request.Plan.Validate(time.Time{}); err != nil {
		return GovernedOperationResultV3{}, err
	}
	if err := request.Submission.Validate(); err != nil {
		return GovernedOperationResultV3{}, err
	}
	current, err := c.config.Attempts.InspectGovernedOperationAttemptV3(ctx, request.Plan.Target, request.AttemptID)
	if err != nil {
		return GovernedOperationResultV3{}, err
	}
	if err := validateOperationPlanBindingV3(request.Plan, current); err != nil {
		return GovernedOperationResultV3{}, err
	}
	if _, err := c.verifyJournalDispatchIntentV3(ctx, request.Plan, initialOperationAttemptV3(current), current); err != nil {
		return GovernedOperationResultV3{}, err
	}
	if current.State != contract.OperationProviderObservedV3 && current.State != contract.OperationDispatchUnknownV3 && current.State != contract.OperationSettledV3 {
		return GovernedOperationResultV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectStateConflict, "operation is not ready for independent settlement")
	}
	if current.State == contract.OperationDispatchUnknownV3 && current.Prepared == nil && request.Submission.Disposition != runtimeports.OperationSettlementFailedV3 {
		return GovernedOperationResultV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectUnknownOutcome, "pre-prepared unknown may only be closed as confirmed_failed")
	}
	settlement, err := c.config.Settlements.InspectOperationSettlementV3(ctx, current.Operation, current.Intent.EffectID)
	if core.HasCategory(err, core.ErrorNotFound) {
		settlement, err = c.config.Settlements.SettleOperationEffectV3(ctx, current.IntentValue, request.Submission)
		if err != nil {
			settlement, err = c.config.Settlements.InspectOperationSettlementV3(context.WithoutCancel(ctx), current.Operation, current.Intent.EffectID)
		}
	} else if err == nil {
		// Inspect refs intentionally omit some owner-private submission fields.
		// Re-entering the create-once Fact gateway is the only public way to
		// prove that a replay did not change those hidden fields.
		verified, verifyErr := c.config.Settlements.SettleOperationEffectV3(ctx, current.IntentValue, request.Submission)
		if verifyErr != nil {
			return GovernedOperationResultV3{}, verifyErr
		}
		if !sameSettlementRefV3(settlement, verified) {
			return GovernedOperationResultV3{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "settlement gateway replay returned another fact")
		}
	}
	if err != nil {
		return GovernedOperationResultV3{}, err
	}
	if err := validateSettlementSubmissionMatchV3(settlement, request.Submission); err != nil {
		return GovernedOperationResultV3{}, err
	}
	if current.State != contract.OperationSettledV3 {
		next := nextOperationAttemptV3(current, contract.OperationSettledV3, c.config.Clock())
		next.Settlement = &settlement
		next.SettlementDomainResult = cloneOpaquePayloadV3(request.Submission.DomainResult)
		current, err = c.commitAttemptV3(ctx, current, next)
		if err != nil {
			return GovernedOperationResultV3{}, err
		}
	} else {
		if _, err := current.RefV3(); err != nil {
			return GovernedOperationResultV3{}, err
		}
		if !sameOpaquePayloadV3(current.SettlementDomainResult, request.Submission.DomainResult) {
			return GovernedOperationResultV3{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "settlement replay changed DomainResult")
		}
	}
	return c.finishSettlementV3(ctx, request.Plan, current)
}

func (c *GovernedOperationCoordinatorV3) inspectOrCreateAttemptV3(ctx context.Context, requested contract.GovernedOperationAttemptFactV3) (contract.GovernedOperationAttemptFactV3, error) {
	current, err := c.config.Attempts.InspectGovernedOperationAttemptV3(ctx, requested.Scope, requested.ID)
	if err == nil {
		if validateErr := current.Validate(); validateErr != nil {
			return contract.GovernedOperationAttemptFactV3{}, validateErr
		}
		return current, nil
	}
	if !core.HasCategory(err, core.ErrorNotFound) {
		return contract.GovernedOperationAttemptFactV3{}, err
	}
	created, createErr := c.config.Attempts.CreateGovernedOperationAttemptV3(ctx, requested)
	if createErr == nil {
		if validateErr := created.Validate(); validateErr != nil {
			return contract.GovernedOperationAttemptFactV3{}, validateErr
		}
		return created, nil
	}
	if !recoverableApplicationWriteErrorV3(createErr) {
		return contract.GovernedOperationAttemptFactV3{}, createErr
	}
	inspected, inspectErr := c.config.Attempts.InspectGovernedOperationAttemptV3(context.WithoutCancel(ctx), requested.Scope, requested.ID)
	if inspectErr != nil {
		return contract.GovernedOperationAttemptFactV3{}, createErr
	}
	if validateErr := inspected.Validate(); validateErr != nil {
		return contract.GovernedOperationAttemptFactV3{}, validateErr
	}
	return inspected, nil
}

func (c *GovernedOperationCoordinatorV3) ensureJournalDispatchIntentV3(ctx context.Context, plan contract.WorkflowPlanV2, initial, current contract.GovernedOperationAttemptFactV3) (contract.WorkflowJournalV2, error) {
	journal, err := c.config.Journals.InspectWorkflowJournalV2(ctx, plan.Target, initial.JournalID)
	if err != nil {
		return contract.WorkflowJournalV2{}, err
	}
	if err := journal.ValidateFor(plan); err != nil {
		return contract.WorkflowJournalV2{}, err
	}
	ref, err := initial.RefV3()
	if err != nil {
		return contract.WorkflowJournalV2{}, err
	}
	effect := &contract.ApplicationFactRefV2{Ref: ref.ID, Revision: ref.Revision, Digest: ref.Digest}
	for _, step := range journal.Steps {
		if step.StepID != initial.StepID {
			continue
		}
		if step.State == contract.StepReadyV2 {
			return c.journal.AdvanceStepV2(ctx, AdvanceStepRequestV2{Plan: plan, JournalID: journal.ID, StepID: step.StepID, Target: contract.StepDispatchIntentV2, Effect: effect})
		}
		if step.Effect == nil || *step.Effect != *effect || step.Attempt != initial.WorkflowAttempt {
			return contract.WorkflowJournalV2{}, core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "workflow journal does not bind the exact governed attempt")
		}
		if step.State == contract.StepCompletedV2 && current.State != contract.OperationSettledV3 {
			return contract.WorkflowJournalV2{}, core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "workflow completed before governed settlement")
		}
		return journal, nil
	}
	return contract.WorkflowJournalV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "governed workflow step not found")
}

func (c *GovernedOperationCoordinatorV3) verifyJournalDispatchIntentV3(ctx context.Context, plan contract.WorkflowPlanV2, initial, current contract.GovernedOperationAttemptFactV3) (contract.WorkflowJournalV2, error) {
	journal, err := c.config.Journals.InspectWorkflowJournalV2(ctx, plan.Target, initial.JournalID)
	if err != nil {
		return contract.WorkflowJournalV2{}, err
	}
	if err := journal.ValidateFor(plan); err != nil {
		return contract.WorkflowJournalV2{}, err
	}
	ref, err := initial.RefV3()
	if err != nil {
		return contract.WorkflowJournalV2{}, err
	}
	effect := contract.ApplicationFactRefV2{Ref: ref.ID, Revision: ref.Revision, Digest: ref.Digest}
	for _, step := range journal.Steps {
		if step.StepID != initial.StepID {
			continue
		}
		if step.State == contract.StepReadyV2 || step.Effect == nil || *step.Effect != effect || step.Attempt != initial.WorkflowAttempt {
			return contract.WorkflowJournalV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectStateConflict, "Resume requires the exact persisted dispatch_intent write-ahead")
		}
		if step.State == contract.StepCompletedV2 && current.State != contract.OperationSettledV3 {
			return contract.WorkflowJournalV2{}, core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "workflow completed before governed settlement")
		}
		return journal, nil
	}
	return contract.WorkflowJournalV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "governed workflow step not found")
}

func (c *GovernedOperationCoordinatorV3) inspectAuthorizationV3(ctx context.Context, fact contract.GovernedOperationAttemptFactV3) (runtimeports.OperationDispatchAuthorizationV3, error) {
	return c.config.Governance.InspectOperationDispatchAuthorizationV3(ctx, fact.Operation, fact.Intent.EffectID, fact.DispatchPlan.PermitID)
}

func (c *GovernedOperationCoordinatorV3) prepareAndCommitV3(ctx context.Context, fact contract.GovernedOperationAttemptFactV3) (runtimeports.PreparedExecutionGovernanceResultV2, bool, error) {
	inspect := runtimeports.InspectPreparedProviderRequestV2{DeclaredDelegation: *fact.DeclaredDelegation, PreparedAttemptID: fact.DelegationFact.PreparedAttemptID, PermitID: fact.DispatchPlan.PermitID, AttemptID: fact.DispatchPlan.AttemptID}
	attestation, inspectErr := c.config.Execution.RelayInspectPrepared(ctx, inspect)
	prepare := runtimeports.PrepareGovernedExecutionRequestV2{Delegation: *fact.DeclaredDelegation, Intent: fact.IntentValue, Permit: fact.BegunAuthorization.Permit, Fence: fact.BegunAuthorization.Fence}
	if core.HasCategory(inspectErr, core.ErrorNotFound) {
		attestation, inspectErr = c.config.Execution.RelayPrepare(ctx, prepare)
		if inspectErr != nil {
			attestation, inspectErr = c.config.Execution.RelayInspectPrepared(context.WithoutCancel(ctx), inspect)
			if inspectErr != nil {
				// RelayPrepare crossed the provider boundary. An absent or
				// unavailable inspection cannot authorize another Prepare.
				return runtimeports.PreparedExecutionGovernanceResultV2{}, true, inspectErr
			}
		}
	}
	if inspectErr != nil {
		return runtimeports.PreparedExecutionGovernanceResultV2{}, false, inspectErr
	}
	if err := attestation.ValidateAgainstPrepare(prepare, c.config.Clock()); err != nil {
		return runtimeports.PreparedExecutionGovernanceResultV2{}, false, err
	}
	prepared, err := c.config.Delegations.InspectPreparedExecutionV2(ctx, fact.Operation, fact.DelegationPlan.DelegationID, fact.DispatchPlan.PermitID)
	if core.HasCategory(err, core.ErrorNotFound) {
		prepared, err = c.config.Delegations.CommitPreparedExecutionV2(ctx, runtimeports.CommitPreparedExecutionRequestV2{Declared: *fact.DeclaredDelegation, Intent: fact.IntentValue, Permit: fact.BegunAuthorization.Permit, Fence: fact.BegunAuthorization.Fence, Preparation: attestation})
		if err != nil {
			prepared, err = c.config.Delegations.InspectPreparedExecutionV2(context.WithoutCancel(ctx), fact.Operation, fact.DelegationPlan.DelegationID, fact.DispatchPlan.PermitID)
		}
	}
	if err != nil {
		return runtimeports.PreparedExecutionGovernanceResultV2{}, false, err
	}
	if prepared.Prepared != attestation.Prepared {
		return runtimeports.PreparedExecutionGovernanceResultV2{}, false, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "prepared execution differs from provider attestation")
	}
	return prepared, false, prepared.Validate()
}

func (c *GovernedOperationCoordinatorV3) executeOrInspectV3(ctx context.Context, fact contract.GovernedOperationAttemptFactV3) (runtimeports.ProviderAttemptObservationV2, bool, error) {
	inspect := runtimeports.InspectLocalProviderAttemptRequestV2{Delegation: *fact.PreparedDelegation, Prepared: *fact.Prepared}
	observation, err := c.config.Execution.RelayInspectLocalAttempt(ctx, inspect)
	if err == nil {
		if observation.State == runtimeports.ProviderAttemptPreparedV2 {
			// A provider-owned prepared watermark proves Execute has not begun.
		} else {
			return observation, observation.State != runtimeports.ProviderAttemptObservedV2, nil
		}
	} else if !core.HasCategory(err, core.ErrorNotFound) {
		return runtimeports.ProviderAttemptObservationV2{}, false, err
	}
	execute := runtimeports.ExecutePreparedRequestV2{Delegation: *fact.PreparedDelegation, Prepared: *fact.Prepared, Enforcement: *fact.Enforcement, Intent: fact.IntentValue, Permit: fact.BegunAuthorization.Permit, Fence: fact.BegunAuthorization.Fence}
	observation, err = c.config.Execution.RelayExecutePrepared(ctx, execute)
	if err == nil {
		return observation, observation.State != runtimeports.ProviderAttemptObservedV2, nil
	}
	inspected, inspectErr := c.config.Execution.RelayInspectLocalAttempt(context.WithoutCancel(ctx), inspect)
	if inspectErr == nil && inspected.State == runtimeports.ProviderAttemptObservedV2 {
		return inspected, false, nil
	}
	// Once Execute was called, absence, executing or unknown are all unknown
	// outcome. Never call ExecutePrepared again for this attempt.
	return runtimeports.ProviderAttemptObservationV2{}, true, err
}

func (c *GovernedOperationCoordinatorV3) recordObservationV3(ctx context.Context, fact contract.GovernedOperationAttemptFactV3, observation runtimeports.ProviderAttemptObservationV2) (runtimeports.ProviderAttemptObservationRefV2, error) {
	stored, err := c.config.Observations.InspectGovernedProviderObservationV3(ctx, *fact.PreparedDelegation, fact.Prepared.ID)
	if err == nil {
		want, refErr := observation.RefV2()
		if refErr != nil || stored != want {
			return runtimeports.ProviderAttemptObservationRefV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "persisted provider observation differs from local observation")
		}
		return stored, nil
	}
	if !core.HasCategory(err, core.ErrorNotFound) {
		return runtimeports.ProviderAttemptObservationRefV2{}, err
	}
	refs := governedAttemptRefsV3(fact)
	stored, err = c.config.Observations.RecordGovernedProviderObservationV3(ctx, runtimeports.RecordGovernedProviderObservationRequestV2{Intent: fact.IntentValue, Permit: fact.BegunAuthorization.Permit, Fence: fact.BegunAuthorization.Fence, Attempt: refs, Observation: observation})
	if err != nil {
		stored, err = c.config.Observations.InspectGovernedProviderObservationV3(context.WithoutCancel(ctx), *fact.PreparedDelegation, fact.Prepared.ID)
	}
	return stored, err
}

func (c *GovernedOperationCoordinatorV3) markUnknownV3(ctx context.Context, fact contract.GovernedOperationAttemptFactV3) (contract.GovernedOperationAttemptFactV3, error) {
	authorization, err := c.inspectAuthorizationV3(ctx, fact)
	if err == nil && authorization.State == runtimeports.OperationDispatchAuthorizationBegunV3 {
		authorization, err = c.config.Governance.MarkOperationDispatchUnknownV3(ctx, runtimeports.MarkOperationDispatchUnknownRequestV3{Operation: fact.Operation, EffectID: fact.Intent.EffectID, ExpectedEffectRevision: fact.BegunAuthorization.EffectFactRevision, Permit: fact.BegunAuthorization.Attempt})
		if err != nil {
			authorization, err = c.inspectAuthorizationV3(context.WithoutCancel(ctx), fact)
		}
	}
	if err != nil {
		return contract.GovernedOperationAttemptFactV3{}, err
	}
	if authorization.State != runtimeports.OperationDispatchAuthorizationUnknownV3 {
		return contract.GovernedOperationAttemptFactV3{}, core.NewError(core.ErrorConflict, core.ReasonEffectUnknownOutcome, "provider uncertainty was not persisted as Runtime unknown")
	}
	next := nextOperationAttemptV3(fact, contract.OperationDispatchUnknownV3, c.config.Clock())
	next.UnknownAuthorization = &authorization
	return c.commitAttemptV3(ctx, fact, next)
}

func (c *GovernedOperationCoordinatorV3) finishObservedV3(ctx context.Context, plan contract.WorkflowPlanV2, fact contract.GovernedOperationAttemptFactV3) (GovernedOperationResultV3, error) {
	domainPort, err := c.resolveDomainV3(ctx, fact)
	if err != nil {
		return GovernedOperationResultV3{}, err
	}
	ref, _ := fact.RefV3()
	runtimeAttempt := governedAttemptRefsV3(fact)
	basis := struct {
		RuntimeAttempt runtimeports.GovernedExecutionAttemptRefsV2  `json:"runtime_attempt"`
		Observation    runtimeports.ProviderAttemptObservationRefV2 `json:"observation"`
	}{RuntimeAttempt: runtimeAttempt, Observation: *fact.Observation}
	domain, err := c.ensureDomainV3(ctx, domainPort, fact, applicationports.OperationDomainObservedV3, basis, func() (applicationports.OperationDomainStateRefV3, error) {
		request := applicationports.BindObservedOperationRequestV3{StepKind: fact.StepKind, Attempt: ref, Intent: fact.IntentValue, RuntimeAttempt: runtimeAttempt, Observation: basis.Observation}
		if err := request.Validate(); err != nil {
			return applicationports.OperationDomainStateRefV3{}, err
		}
		return domainPort.BindObserved(ctx, request)
	})
	if err != nil {
		return GovernedOperationResultV3{}, err
	}
	journal, err := c.ensureJournalWaitingV3(ctx, plan, fact)
	return GovernedOperationResultV3{Attempt: fact, Journal: journal, Domain: &domain}, err
}

func (c *GovernedOperationCoordinatorV3) finishUnknownV3(ctx context.Context, plan contract.WorkflowPlanV2, fact contract.GovernedOperationAttemptFactV3) (GovernedOperationResultV3, error) {
	// Begin and RelayPrepare can become unknown before a domain owner has
	// received exact Prepared refs. In that case the Application attempt and
	// journal are the recovery barrier; fabricating a domain unknown state
	// would let a permissive adapter hide a pre-prepare authority violation.
	if fact.Prepared == nil {
		journal, err := c.ensureJournalWaitingV3(ctx, plan, fact)
		return GovernedOperationResultV3{Attempt: fact, Journal: journal}, err
	}
	domainPort, err := c.resolveDomainV3(ctx, fact)
	if err != nil {
		return GovernedOperationResultV3{}, err
	}
	ref, _ := fact.RefV3()
	runtimeAttempt := governedAttemptRefsV3(fact)
	basis := struct {
		RuntimeAttempt runtimeports.GovernedExecutionAttemptRefsV2   `json:"runtime_attempt"`
		Authorization  runtimeports.OperationDispatchAuthorizationV3 `json:"authorization"`
	}{RuntimeAttempt: runtimeAttempt, Authorization: *fact.UnknownAuthorization}
	domain, err := c.ensureDomainV3(ctx, domainPort, fact, applicationports.OperationDomainUnknownV3, basis, func() (applicationports.OperationDomainStateRefV3, error) {
		request := applicationports.MarkUnknownOperationRequestV3{StepKind: fact.StepKind, Attempt: ref, Intent: fact.IntentValue, RuntimeAttempt: runtimeAttempt, Authorization: basis.Authorization}
		if err := request.Validate(); err != nil {
			return applicationports.OperationDomainStateRefV3{}, err
		}
		return domainPort.MarkUnknown(ctx, request)
	})
	if err != nil {
		return GovernedOperationResultV3{}, err
	}
	journal, err := c.ensureJournalWaitingV3(ctx, plan, fact)
	return GovernedOperationResultV3{Attempt: fact, Journal: journal, Domain: &domain}, err
}

func (c *GovernedOperationCoordinatorV3) finishSettlementV3(ctx context.Context, plan contract.WorkflowPlanV2, fact contract.GovernedOperationAttemptFactV3) (GovernedOperationResultV3, error) {
	domainPort, err := c.resolveDomainV3(ctx, fact)
	if err != nil {
		return GovernedOperationResultV3{}, err
	}
	ref, _ := fact.RefV3()
	var runtimeAttempt *runtimeports.GovernedExecutionAttemptRefsV2
	if fact.Prepared != nil {
		refs := governedAttemptRefsV3(fact)
		runtimeAttempt = &refs
	}
	basis := struct {
		RuntimeAttempt *runtimeports.GovernedExecutionAttemptRefsV2 `json:"runtime_attempt,omitempty"`
		Settlement     runtimeports.OperationSettlementRefV3        `json:"settlement"`
		DomainResult   *runtimeports.OpaquePayloadV2                `json:"domain_result,omitempty"`
	}{RuntimeAttempt: runtimeAttempt, Settlement: *fact.Settlement, DomainResult: cloneOpaquePayloadV3(fact.SettlementDomainResult)}
	domain, err := c.ensureDomainV3(ctx, domainPort, fact, applicationports.OperationDomainSettledV3, basis, func() (applicationports.OperationDomainStateRefV3, error) {
		request := applicationports.ApplyOperationSettlementRequestV3{StepKind: fact.StepKind, Attempt: ref, Intent: fact.IntentValue, RuntimeAttempt: runtimeAttempt, Settlement: basis.Settlement, DomainResult: cloneOpaquePayloadV3(fact.SettlementDomainResult)}
		if err := request.Validate(); err != nil {
			return applicationports.OperationDomainStateRefV3{}, err
		}
		return domainPort.ApplySettlement(ctx, request)
	})
	if err != nil {
		return GovernedOperationResultV3{}, err
	}
	journal, err := c.completion.Complete(ctx, plan, fact.ID)
	if err != nil {
		return GovernedOperationResultV3{}, err
	}
	return GovernedOperationResultV3{Attempt: fact, Journal: journal, Domain: &domain}, nil
}

func (c *GovernedOperationCoordinatorV3) ensureDomainPreparedV3(ctx context.Context, fact contract.GovernedOperationAttemptFactV3) (applicationports.OperationDomainStateRefV3, error) {
	domainPort, err := c.resolveDomainV3(ctx, fact)
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	ref, _ := fact.RefV3()
	runtimeAttempt := governedAttemptRefsV3(fact)
	basis := struct {
		RuntimeAttempt runtimeports.GovernedExecutionAttemptRefsV2      `json:"runtime_attempt"`
		DelegationFact runtimeports.ExecutionDelegationFactV2           `json:"delegation_fact"`
		Prepared       runtimeports.PreparedExecutionGovernanceResultV2 `json:"prepared"`
	}{RuntimeAttempt: runtimeAttempt, DelegationFact: *fact.DelegationFact, Prepared: runtimeports.PreparedExecutionGovernanceResultV2{Delegation: *fact.PreparedDelegation, Prepared: *fact.Prepared, Enforcement: *fact.Enforcement}}
	return c.ensureDomainV3(ctx, domainPort, fact, applicationports.OperationDomainPreparedV3, basis, func() (applicationports.OperationDomainStateRefV3, error) {
		request := applicationports.BindPreparedOperationRequestV3{StepKind: fact.StepKind, Attempt: ref, Intent: fact.IntentValue, RuntimeAttempt: runtimeAttempt, DelegationFact: basis.DelegationFact, Prepared: basis.Prepared}
		if err := request.Validate(); err != nil {
			return applicationports.OperationDomainStateRefV3{}, err
		}
		return domainPort.BindPrepared(ctx, request)
	})
}

func (c *GovernedOperationCoordinatorV3) ensureDomainV3(ctx context.Context, domainPort applicationports.OperationDomainStatePortV3, fact contract.GovernedOperationAttemptFactV3, state applicationports.OperationDomainStateV3, basis any, apply func() (applicationports.OperationDomainStateRefV3, error)) (applicationports.OperationDomainStateRefV3, error) {
	basisDigest, err := applicationports.OperationDomainBasisDigestV3(basis)
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	inspect := applicationports.OperationDomainInspectRequestV3{Scope: fact.Scope, StepKind: fact.StepKind, AttemptID: fact.ID}
	current, inspectErr := domainPort.InspectOperationDomainStateV3(ctx, inspect)
	if inspectErr == nil {
		ref, _ := fact.RefV3()
		if current.State == state {
			if current.StepKind != fact.StepKind || !sameOperationAttemptRefV3(current.Attempt, ref) || current.BasisDigest != basisDigest {
				return applicationports.OperationDomainStateRefV3{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "domain state differs from exact governed attempt basis")
			}
			return current, nil
		}
		if current.StepKind != fact.StepKind || !sameOperationAttemptRoutingV3(current.Attempt, ref) || !domainTransitionAllowedV3(current.State, state) {
			if current.StepKind == fact.StepKind && sameOperationAttemptRoutingV3(current.Attempt, ref) && domainStateSuccessorV3(current.State, state) {
				return current, nil
			}
			return applicationports.OperationDomainStateRefV3{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "domain state differs from exact governed attempt basis")
		}
	} else if !core.HasCategory(inspectErr, core.ErrorNotFound) {
		return applicationports.OperationDomainStateRefV3{}, inspectErr
	}
	stored, err := apply()
	if err != nil {
		stored, err = domainPort.InspectOperationDomainStateV3(context.WithoutCancel(ctx), inspect)
	}
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	ref, _ := fact.RefV3()
	if stored.State != state || stored.StepKind != fact.StepKind || !sameOperationAttemptRefV3(stored.Attempt, ref) || stored.BasisDigest != basisDigest {
		return applicationports.OperationDomainStateRefV3{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "domain owner returned another operation state")
	}
	return stored, stored.Validate()
}

func (c *GovernedOperationCoordinatorV3) resolveDomainV3(ctx context.Context, fact contract.GovernedOperationAttemptFactV3) (applicationports.OperationDomainStatePortV3, error) {
	return c.config.DomainResolver.ResolveOperationDomainV3(ctx, applicationports.OperationDomainResolveRequestV3{StepKind: fact.StepKind, Descriptor: fact.Descriptor, DomainAdapter: fact.DomainAdapter})
}

func (c *GovernedOperationCoordinatorV3) reserveDomainIntentV3(ctx context.Context, fact contract.GovernedOperationAttemptFactV3) (contract.OperationDomainReservationRefV3, error) {
	domain, err := c.resolveDomainV3(ctx, fact)
	if err != nil {
		return contract.OperationDomainReservationRefV3{}, err
	}
	ref, err := fact.RefV3()
	if err != nil {
		return contract.OperationDomainReservationRefV3{}, err
	}
	request := applicationports.ReserveOperationIntentRequestV3{StepKind: fact.StepKind, Descriptor: fact.Descriptor, DomainAdapter: fact.DomainAdapter, Attempt: ref, Intent: fact.IntentValue}
	if err := request.Validate(); err != nil {
		return contract.OperationDomainReservationRefV3{}, err
	}
	inspect := applicationports.InspectOperationIntentReservationRequestV3{Scope: fact.Scope, StepKind: fact.StepKind, DomainAdapter: fact.DomainAdapter, AttemptID: fact.ID}
	reservation, err := domain.InspectOperationIntentReservationV3(ctx, inspect)
	if core.HasCategory(err, core.ErrorNotFound) {
		reservation, err = domain.ReserveOperationIntentV3(ctx, request)
		if err != nil && recoverableApplicationWriteErrorV3(err) {
			reservation, err = domain.InspectOperationIntentReservationV3(context.WithoutCancel(ctx), inspect)
		}
	}
	if err != nil {
		return contract.OperationDomainReservationRefV3{}, err
	}
	if err := applicationports.ValidateOperationDomainReservationForV3(reservation, request); err != nil {
		return contract.OperationDomainReservationRefV3{}, err
	}
	return reservation, nil
}

func (c *GovernedOperationCoordinatorV3) verifyDomainReservationV3(ctx context.Context, fact contract.GovernedOperationAttemptFactV3) error {
	if fact.DomainReservation == nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "domain_reserved attempt lacks its reservation")
	}
	domain, err := c.resolveDomainV3(ctx, fact)
	if err != nil {
		return err
	}
	initial := initialOperationAttemptV3(fact)
	initialRef, err := initial.RefV3()
	if err != nil {
		return err
	}
	request := applicationports.ReserveOperationIntentRequestV3{StepKind: fact.StepKind, Descriptor: fact.Descriptor, DomainAdapter: fact.DomainAdapter, Attempt: initialRef, Intent: fact.IntentValue}
	inspect := applicationports.InspectOperationIntentReservationRequestV3{Scope: fact.Scope, StepKind: fact.StepKind, DomainAdapter: fact.DomainAdapter, AttemptID: fact.ID}
	reservation, err := domain.InspectOperationIntentReservationV3(ctx, inspect)
	if err != nil {
		return err
	}
	if err := applicationports.ValidateOperationDomainReservationForV3(reservation, request); err != nil {
		return err
	}
	if reservation.Digest != fact.DomainReservation.Digest || reservation.ID != fact.DomainReservation.ID {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "persisted Application reservation differs from domain owner reservation")
	}
	if c.config.Clock().UnixNano() >= reservation.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "operation domain reservation expired before Runtime mutation")
	}
	return nil
}

func (c *GovernedOperationCoordinatorV3) ensureJournalWaitingV3(ctx context.Context, plan contract.WorkflowPlanV2, fact contract.GovernedOperationAttemptFactV3) (contract.WorkflowJournalV2, error) {
	return c.journal.AdvanceStepV2(ctx, AdvanceStepRequestV2{Plan: plan, JournalID: fact.JournalID, StepID: fact.StepID, Target: contract.StepWaitingInspectV2})
}

func (c *GovernedOperationCoordinatorV3) commitAttemptV3(ctx context.Context, current, next contract.GovernedOperationAttemptFactV3) (contract.GovernedOperationAttemptFactV3, error) {
	stored, err := c.config.Attempts.CompareAndSwapGovernedOperationAttemptV3(ctx, applicationports.GovernedOperationAttemptCASRequestV3{Scope: current.Scope, ID: current.ID, ExpectedRevision: current.Revision, Next: next})
	if err == nil {
		if validateErr := stored.Validate(); validateErr != nil {
			return contract.GovernedOperationAttemptFactV3{}, validateErr
		}
		if sameOperationAttemptV3(stored, next) || operationAttemptSuccessorV3(next, stored) {
			return stored, nil
		}
		return contract.GovernedOperationAttemptFactV3{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "operation CAS successful reply is neither the requested fact nor its proven successor")
	}
	if !recoverableApplicationWriteErrorV3(err) {
		return contract.GovernedOperationAttemptFactV3{}, err
	}
	inspected, inspectErr := c.config.Attempts.InspectGovernedOperationAttemptV3(context.WithoutCancel(ctx), current.Scope, current.ID)
	if inspectErr != nil {
		return contract.GovernedOperationAttemptFactV3{}, err
	}
	if validateErr := inspected.Validate(); validateErr != nil {
		return contract.GovernedOperationAttemptFactV3{}, validateErr
	}
	// Another coordinator may win a sibling branch from current and progress
	// several more stages before this recovery Inspect. Accept only a strict,
	// append-only successor proven from current (or from our requested next).
	if sameOperationAttemptV3(inspected, next) || operationAttemptSuccessorV3(current, inspected) || operationAttemptSuccessorV3(next, inspected) {
		return inspected, nil
	}
	return contract.GovernedOperationAttemptFactV3{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "operation CAS recovery returned an unrelated or regressed fact")
}

func operationAttemptSuccessorV3(expected, actual contract.GovernedOperationAttemptFactV3) bool {
	if actual.Revision <= expected.Revision || !sameAttemptOriginV3(actual, initialOperationAttemptV3(expected)) || !operationAttemptStateReachableV3(expected.State, actual.State) {
		return false
	}
	for _, pair := range [][2]any{
		{expected.DomainReservation, actual.DomainReservation}, {expected.Admission, actual.Admission},
		{expected.IssuedAuthorization, actual.IssuedAuthorization}, {expected.BegunAuthorization, actual.BegunAuthorization},
		{expected.DelegationFact, actual.DelegationFact}, {expected.DeclaredDelegation, actual.DeclaredDelegation},
		{expected.PreparedDelegation, actual.PreparedDelegation}, {expected.Prepared, actual.Prepared},
		{expected.Enforcement, actual.Enforcement}, {expected.Observation, actual.Observation},
		{expected.UnknownAuthorization, actual.UnknownAuthorization}, {expected.Settlement, actual.Settlement},
		{expected.SettlementDomainResult, actual.SettlementDomainResult},
	} {
		if !nilApplicationValueV3(pair[0]) && (nilApplicationValueV3(pair[1]) || !sameApplicationValueV3("operation-successor-sidecar", pair[0], pair[1])) {
			return false
		}
	}
	return true
}

func nilApplicationValueV3(value any) bool {
	if value == nil {
		return true
	}
	switch v := value.(type) {
	case *contract.OperationDomainReservationRefV3:
		return v == nil
	case *runtimeports.OperationEffectAdmissionReceiptV3:
		return v == nil
	case *runtimeports.OperationDispatchAuthorizationV3:
		return v == nil
	case *runtimeports.ExecutionDelegationFactV2:
		return v == nil
	case *runtimeports.ExecutionDelegationRefV2:
		return v == nil
	case *runtimeports.PreparedProviderAttemptRefV2:
		return v == nil
	case *runtimeports.PersistedOperationEnforcementRefV3:
		return v == nil
	case *runtimeports.ProviderAttemptObservationRefV2:
		return v == nil
	case *runtimeports.OperationSettlementRefV3:
		return v == nil
	case *runtimeports.OpaquePayloadV2:
		return v == nil
	}
	return false
}

func operationAttemptStateReachableV3(from, to contract.GovernedOperationAttemptStateV3) bool {
	if from == to {
		return true
	}
	seen := map[contract.GovernedOperationAttemptStateV3]bool{from: true}
	queue := []contract.GovernedOperationAttemptStateV3{from}
	states := []contract.GovernedOperationAttemptStateV3{contract.OperationDomainReservedV3, contract.OperationEffectAdmittedV3, contract.OperationPermitIssuedV3, contract.OperationPermitBegunV3, contract.OperationDelegationDeclaredV3, contract.OperationExecutionPreparedV3, contract.OperationProviderObservedV3, contract.OperationDispatchUnknownV3, contract.OperationSettledV3}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, candidate := range states {
			if seen[candidate] {
				continue
			}
			if operationAttemptDirectSuccessorV3(current, candidate) {
				if candidate == to {
					return true
				}
				seen[candidate] = true
				queue = append(queue, candidate)
			}
		}
	}
	return false
}

func operationAttemptDirectSuccessorV3(from, to contract.GovernedOperationAttemptStateV3) bool {
	switch from {
	case contract.OperationIntentRecordedV3:
		return to == contract.OperationDomainReservedV3
	case contract.OperationDomainReservedV3:
		return to == contract.OperationEffectAdmittedV3
	case contract.OperationEffectAdmittedV3:
		return to == contract.OperationPermitIssuedV3
	case contract.OperationPermitIssuedV3:
		return to == contract.OperationPermitBegunV3 || to == contract.OperationDispatchUnknownV3
	case contract.OperationPermitBegunV3:
		return to == contract.OperationDelegationDeclaredV3 || to == contract.OperationDispatchUnknownV3
	case contract.OperationDelegationDeclaredV3:
		return to == contract.OperationExecutionPreparedV3 || to == contract.OperationDispatchUnknownV3
	case contract.OperationExecutionPreparedV3:
		return to == contract.OperationProviderObservedV3 || to == contract.OperationDispatchUnknownV3
	case contract.OperationProviderObservedV3, contract.OperationDispatchUnknownV3:
		return to == contract.OperationSettledV3
	}
	return false
}

func nextOperationAttemptV3(current contract.GovernedOperationAttemptFactV3, state contract.GovernedOperationAttemptStateV3, now time.Time) contract.GovernedOperationAttemptFactV3 {
	next := current
	next.Revision++
	next.State = state
	next.UpdatedUnixNano = now.UnixNano()
	return next
}

func deriveExecutionDelegationV3(fact contract.GovernedOperationAttemptFactV3, now time.Time) (runtimeports.ExecutionDelegationFactV2, error) {
	if now.IsZero() {
		return runtimeports.ExecutionDelegationFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "delegation clock returned zero")
	}
	p := fact.DelegationPlan
	a := fact.BegunAuthorization.Attempt
	preparedID, err := runtimeports.DerivePreparedProviderAttemptIDV2(p.DelegationID, a.PermitID, a.AttemptID)
	if err != nil {
		return runtimeports.ExecutionDelegationFactV2{}, err
	}
	created := time.Unix(0, fact.UpdatedUnixNano)
	expires := created.Add(time.Duration(p.DelegationTTLNanos)).UnixNano()
	for _, limit := range []int64{fact.IntentValue.ExpiresUnixNano, fact.BegunAuthorization.Permit.ExpiresUnixNano, p.HostBindingExpiresUnixNano, p.ProviderBindingExpiresUnixNano} {
		if limit < expires {
			expires = limit
		}
	}
	provider := fact.IntentValue.Provider
	delegation := runtimeports.ExecutionDelegationFactV2{ContractVersion: runtimeports.ExecutionGovernanceContractVersionV2, ID: p.DelegationID, Revision: 1, State: runtimeports.ExecutionDelegationDeclaredV2, BindingSetID: provider.BindingSetID, BindingSetRevision: provider.BindingSetRevision, Operation: fact.Operation, HostAdapter: p.HostAdapter, DataProvider: provider, RelayHops: append([]runtimeports.ExecutionRelayHopV2(nil), p.RelayHops...), EndpointID: p.EndpointID, RuntimeSessionRef: p.RuntimeSessionRef, PayloadSchema: fact.IntentValue.Payload.Schema, PayloadDigest: fact.IntentValue.Payload.ContentDigest, PayloadRevision: fact.IntentValue.PayloadRevision, IntentID: a.EffectID, IntentRevision: a.IntentRevision, IntentDigest: a.IntentDigest, ProviderPermitID: a.PermitID, ProviderPermitRevision: a.PermitRevision, ProviderPermitDigest: a.PermitDigest, ProviderAttemptID: a.AttemptID, PreparedAttemptID: preparedID, OperationExpiresUnixNano: fact.IntentValue.ExpiresUnixNano, PermitExpiresUnixNano: fact.BegunAuthorization.Permit.ExpiresUnixNano, HostBindingExpiresUnixNano: p.HostBindingExpiresUnixNano, ProviderBindingExpiresUnixNano: p.ProviderBindingExpiresUnixNano, CreatedUnixNano: created.UnixNano(), ExpiresUnixNano: expires}
	return delegation, delegation.Validate()
}

func governedAttemptRefsV3(fact contract.GovernedOperationAttemptFactV3) runtimeports.GovernedExecutionAttemptRefsV2 {
	a := fact.BegunAuthorization.Attempt
	refs := runtimeports.GovernedExecutionAttemptRefsV2{Admission: *fact.Admission, PermitID: a.PermitID, PermitRevision: a.PermitRevision, PermitDigest: a.PermitDigest, AttemptID: a.AttemptID, Delegation: *fact.PreparedDelegation, Prepared: *fact.Prepared, Enforcement: *fact.Enforcement}
	if fact.Observation != nil {
		o := *fact.Observation
		refs.Observation = &o
	}
	if fact.Settlement != nil {
		s := *fact.Settlement
		refs.Settlement = &s
	}
	return refs
}

func domainTransitionAllowedV3(current, target applicationports.OperationDomainStateV3) bool {
	switch current {
	case applicationports.OperationDomainPreparedV3:
		return target == applicationports.OperationDomainObservedV3 || target == applicationports.OperationDomainUnknownV3
	case applicationports.OperationDomainObservedV3, applicationports.OperationDomainUnknownV3:
		return target == applicationports.OperationDomainSettledV3
	default:
		return false
	}
}

func domainStateSuccessorV3(current, requested applicationports.OperationDomainStateV3) bool {
	if current == applicationports.OperationDomainSettledV3 {
		return requested == applicationports.OperationDomainPreparedV3 || requested == applicationports.OperationDomainObservedV3 || requested == applicationports.OperationDomainUnknownV3
	}
	return requested == applicationports.OperationDomainPreparedV3 && (current == applicationports.OperationDomainObservedV3 || current == applicationports.OperationDomainUnknownV3)
}

func cloneOpaquePayloadV3(value *runtimeports.OpaquePayloadV2) *runtimeports.OpaquePayloadV2 {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.Inline = append([]byte(nil), value.Inline...)
	return &cloned
}

func sameOpaquePayloadV3(left, right *runtimeports.OpaquePayloadV2) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	ld, le := core.CanonicalJSONDigest("praxis.application.governed-operation", contract.GovernedOperationAttemptContractVersionV3, "settlement-domain-result", left)
	rd, re := core.CanonicalJSONDigest("praxis.application.governed-operation", contract.GovernedOperationAttemptContractVersionV3, "settlement-domain-result", right)
	return le == nil && re == nil && ld == rd
}

func validateSettlementSubmissionMatchV3(ref runtimeports.OperationSettlementRefV3, submission runtimeports.OperationSettlementSubmissionV3) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	if ref.ID != submission.ID || ref.Revision != submission.Revision || !sameDispatchAttemptRefV3(ref.Attempt, submission.Attempt) || ref.Owner != submission.Owner || ref.Disposition != submission.Disposition || !sameObservationRefV3(ref.Observation, submission.Observation) || !sameDispatchAttemptRefPtrV3(ref.InspectionEffect, submission.InspectionEffect) || !sameInspectionSettlementRefPtrV3(ref.InspectionSettlement, submission.InspectionSettlement) || len(ref.Evidence) != len(submission.Evidence) {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "settlement fact differs from exact submission")
	}
	for i := range ref.Evidence {
		if ref.Evidence[i] != submission.Evidence[i] {
			return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "settlement evidence differs from exact submission")
		}
	}
	if submission.DomainResult == nil {
		if ref.DomainResultSchema != nil || ref.DomainResultDigest != "" {
			return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "settlement DomainResult presence differs from submission")
		}
		return nil
	}
	if ref.DomainResultSchema == nil || *ref.DomainResultSchema != submission.DomainResult.Schema || ref.DomainResultDigest != submission.DomainResult.ContentDigest {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "settlement DomainResult differs from exact submission")
	}
	return nil
}

func sameObservationRefV3(left, right *runtimeports.ProviderAttemptObservationRefV2) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func sameSettlementRefV3(left, right runtimeports.OperationSettlementRefV3) bool {
	ld, le := core.CanonicalJSONDigest("praxis.application.governed-operation", contract.GovernedOperationAttemptContractVersionV3, "operation-settlement-ref", left)
	rd, re := core.CanonicalJSONDigest("praxis.application.governed-operation", contract.GovernedOperationAttemptContractVersionV3, "operation-settlement-ref", right)
	return le == nil && re == nil && ld == rd
}

func issuedAuthorizationProjectionV3(current runtimeports.OperationDispatchAuthorizationV3) (runtimeports.OperationDispatchAuthorizationV3, error) {
	issued := current
	switch current.State {
	case runtimeports.OperationDispatchAuthorizationIssuedV3:
	case runtimeports.OperationDispatchAuthorizationBegunV3:
		if issued.PermitFactRevision <= 1 {
			return runtimeports.OperationDispatchAuthorizationV3{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "begun authorization has no issued predecessor")
		}
		issued.State = runtimeports.OperationDispatchAuthorizationIssuedV3
		issued.PermitFactRevision--
	case runtimeports.OperationDispatchAuthorizationUnknownV3:
		if issued.PermitFactRevision <= 1 || issued.EffectFactRevision <= 2 {
			return runtimeports.OperationDispatchAuthorizationV3{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "unknown authorization has no issued predecessor")
		}
		issued.State = runtimeports.OperationDispatchAuthorizationIssuedV3
		issued.PermitFactRevision--
		issued.EffectFactRevision -= 2
	default:
		return runtimeports.OperationDispatchAuthorizationV3{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitConsumed, "operation Issue recovery is outside its monotonic authorization states")
	}
	if err := issued.Validate(); err != nil {
		return runtimeports.OperationDispatchAuthorizationV3{}, err
	}
	return issued, nil
}

func sameDispatchAttemptRefPtrV3(left, right *runtimeports.OperationDispatchAttemptRefV3) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return sameDispatchAttemptRefV3(*left, *right)
}

func sameDispatchAttemptRefV3(left, right runtimeports.OperationDispatchAttemptRefV3) bool {
	ld, le := core.CanonicalJSONDigest("praxis.application.governed-operation", contract.GovernedOperationAttemptContractVersionV3, "operation-dispatch-attempt-ref", left)
	rd, re := core.CanonicalJSONDigest("praxis.application.governed-operation", contract.GovernedOperationAttemptContractVersionV3, "operation-dispatch-attempt-ref", right)
	return le == nil && re == nil && ld == rd
}

func sameInspectionSettlementRefPtrV3(left, right *runtimeports.OperationInspectionSettlementRefV3) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	ld, le := core.CanonicalJSONDigest("praxis.application.governed-operation", contract.GovernedOperationAttemptContractVersionV3, "operation-inspection-settlement-ref", left)
	rd, re := core.CanonicalJSONDigest("praxis.application.governed-operation", contract.GovernedOperationAttemptContractVersionV3, "operation-inspection-settlement-ref", right)
	return le == nil && re == nil && ld == rd
}

func sameOperationAttemptRefV3(left, right contract.GovernedOperationAttemptRefV3) bool {
	ld, le := core.CanonicalJSONDigest("praxis.application.governed-operation", contract.GovernedOperationAttemptContractVersionV3, "operation-attempt-ref", left)
	rd, re := core.CanonicalJSONDigest("praxis.application.governed-operation", contract.GovernedOperationAttemptContractVersionV3, "operation-attempt-ref", right)
	return le == nil && re == nil && ld == rd
}

func sameOperationAttemptRoutingV3(left, right contract.GovernedOperationAttemptRefV3) bool {
	return left.ID == right.ID && left.ScopeDigest == right.ScopeDigest && left.JournalID == right.JournalID && left.StepID == right.StepID && left.StepKind == right.StepKind && left.Descriptor == right.Descriptor && left.PlannedProvider == right.PlannedProvider && left.DomainAdapter == right.DomainAdapter && left.PlanAuthority == right.PlanAuthority && left.RoutingDigest == right.RoutingDigest && left.WorkflowAttempt == right.WorkflowAttempt && left.OperationDigest == right.OperationDigest && left.EffectID == right.EffectID
}

func validateOperationPlanBindingV3(plan contract.WorkflowPlanV2, fact contract.GovernedOperationAttemptFactV3) error {
	digest, err := plan.DigestV2()
	if err != nil {
		return err
	}
	if fact.PlanID != plan.ID || fact.PlanRevision != plan.Revision || fact.PlanDigest != digest || !runtimeports.SameExecutionScopeV2(plan.Target, fact.Scope) || plan.Authority != fact.PlanAuthority {
		return core.NewError(core.ErrorConflict, core.ReasonPlanInvalid, "operation attempt belongs to another workflow plan projection")
	}
	for _, step := range plan.Steps {
		if step.ID != fact.StepID {
			continue
		}
		if step.ExecutionClass != contract.StepGovernedEffectV2 || step.Kind != fact.StepKind || step.Descriptor != fact.Descriptor || step.Provider == nil || *step.Provider != fact.PlannedProvider || step.DomainAdapter == nil || *step.DomainAdapter != fact.DomainAdapter || step.Payload.ContentDigest != fact.IntentValue.Payload.ContentDigest || step.Payload.Schema != fact.IntentValue.Payload.Schema {
			return core.NewError(core.ErrorConflict, core.ReasonPlanInvalid, "operation step drifted from its persisted governed plan")
		}
		return nil
	}
	return core.NewError(core.ErrorNotFound, core.ReasonPlanInvalid, "operation step is absent from its workflow plan")
}

func initialOperationAttemptV3(fact contract.GovernedOperationAttemptFactV3) contract.GovernedOperationAttemptFactV3 {
	fact.Revision = 1
	fact.State = contract.OperationIntentRecordedV3
	fact.DomainReservation = nil
	fact.Admission, fact.IssuedAuthorization, fact.BegunAuthorization = nil, nil, nil
	fact.DelegationFact, fact.DeclaredDelegation, fact.PreparedDelegation = nil, nil, nil
	fact.Prepared, fact.Enforcement, fact.Observation, fact.UnknownAuthorization, fact.Settlement = nil, nil, nil, nil, nil
	fact.SettlementDomainResult = nil
	fact.UpdatedUnixNano = fact.CreatedUnixNano
	return fact
}

func sameAttemptOriginV3(current, initial contract.GovernedOperationAttemptFactV3) bool {
	return sameOperationAttemptV3(initialOperationAttemptV3(current), initial)
}

func sameOperationAttemptV3(left, right contract.GovernedOperationAttemptFactV3) bool {
	ld, le := left.DigestV3()
	rd, re := right.DigestV3()
	return le == nil && re == nil && ld == rd
}

func recoverableApplicationWriteErrorV3(err error) bool {
	return core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate) || core.HasCategory(err, core.ErrorConflict)
}

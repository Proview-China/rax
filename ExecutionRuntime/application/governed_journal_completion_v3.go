package application

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type GovernedJournalCompletionConfigV3 struct {
	Attempts applicationports.GovernedOperationAttemptFactPortV3
	Journals applicationports.WorkflowJournalFactPortV2
	Clock    func() time.Time
}

type GovernedJournalCompletionV3 struct {
	config GovernedJournalCompletionConfigV3
}

func NewGovernedJournalCompletionV3(config GovernedJournalCompletionConfigV3) (*GovernedJournalCompletionV3, error) {
	if config.Attempts == nil || config.Journals == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "governed Journal completion requires Attempt and Journal Fact owners")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	return &GovernedJournalCompletionV3{config: config}, nil
}

// Complete re-reads the exact settled Attempt before the Journal Owner CAS.
// It is the only production V3 completion path for governed_effect steps.
func (g *GovernedJournalCompletionV3) Complete(ctx context.Context, plan contract.WorkflowPlanV2, attemptID string) (contract.WorkflowJournalV2, error) {
	if err := plan.Validate(time.Time{}); err != nil {
		return contract.WorkflowJournalV2{}, err
	}
	if attemptID != strings.TrimSpace(attemptID) || attemptID == "" || len(attemptID) > 512 {
		return contract.WorkflowJournalV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "governed completion Attempt ID is invalid")
	}
	attempt, err := g.config.Attempts.InspectGovernedOperationAttemptV3(ctx, plan.Target, attemptID)
	if err != nil {
		return contract.WorkflowJournalV2{}, err
	}
	if err := validateOperationPlanBindingV3(plan, attempt); err != nil {
		return contract.WorkflowJournalV2{}, err
	}
	if attempt.State != contract.OperationSettledV3 || attempt.Settlement == nil {
		return contract.WorkflowJournalV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectStateConflict, "governed Journal completion requires an exact settled Attempt")
	}
	current, err := g.config.Journals.InspectWorkflowJournalV2(ctx, plan.Target, attempt.JournalID)
	if err != nil {
		return contract.WorkflowJournalV2{}, err
	}
	if err := current.ValidateFor(plan); err != nil {
		return contract.WorkflowJournalV2{}, err
	}
	index := -1
	for i := range current.Steps {
		if current.Steps[i].StepID == attempt.StepID {
			index = i
			break
		}
	}
	if index < 0 {
		return contract.WorkflowJournalV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "governed completion step not found")
	}
	initial := initialOperationAttemptV3(attempt)
	initialRef, _ := initial.RefV3()
	settledRef, _ := attempt.RefV3()
	effect := contract.ApplicationFactRefV2{Ref: initialRef.ID, Revision: initialRef.Revision, Digest: initialRef.Digest}
	settlement := contract.ApplicationFactRefV2{Ref: settledRef.ID, Revision: settledRef.Revision, Digest: settledRef.Digest}
	progress := current.Steps[index]
	if progress.State == contract.StepCompletedV2 {
		if progress.Effect != nil && progress.Settlement != nil && *progress.Effect == effect && *progress.Settlement == settlement && progress.Attempt == attempt.WorkflowAttempt {
			return current, nil
		}
		return contract.WorkflowJournalV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "completed governed Journal binds another Attempt")
	}
	if progress.State != contract.StepWaitingInspectV2 || progress.Effect == nil || *progress.Effect != effect || progress.Attempt != attempt.WorkflowAttempt {
		return contract.WorkflowJournalV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectStateConflict, "governed Journal is not waiting on the exact Attempt")
	}
	now := g.config.Clock().UnixNano()
	if now < current.UpdatedUnixNano {
		return contract.WorkflowJournalV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "governed completion clock regressed")
	}
	next := current
	next.Revision++
	next.UpdatedUnixNano = now
	next.Steps = append([]contract.WorkflowStepProgressV2(nil), current.Steps...)
	next.Steps[index].State = contract.StepCompletedV2
	next.Steps[index].Effect = &effect
	next.Steps[index].Settlement = &settlement
	next.Steps[index].LastError = ""
	next.Steps[index].UpdatedUnixNano = now
	next.Status = contract.DeriveWorkflowStatusV2(next.Steps)
	committed, err := g.config.Journals.CompareAndSwapWorkflowJournalV2(ctx, plan, applicationports.WorkflowJournalCASRequestV2{ExpectedRevision: current.Revision, Next: next})
	if err != nil && (core.HasCategory(err, core.ErrorConflict) || core.HasCategory(err, core.ErrorUnavailable)) {
		committed, err = g.config.Journals.InspectWorkflowJournalV2(context.WithoutCancel(ctx), plan.Target, attempt.JournalID)
	}
	if err != nil {
		return contract.WorkflowJournalV2{}, err
	}
	if !sameJournalV2(committed, next, plan) {
		return contract.WorkflowJournalV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "governed Journal completion CAS recovered another Attempt")
	}
	return committed, nil
}

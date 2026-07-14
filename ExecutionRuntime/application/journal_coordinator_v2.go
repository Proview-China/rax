package application

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type JournalCoordinatorConfigV2 struct {
	Facts applicationports.WorkflowJournalFactPortV2
	Clock func() time.Time
}

type JournalCoordinatorV2 struct{ config JournalCoordinatorConfigV2 }

func NewJournalCoordinatorV2(config JournalCoordinatorConfigV2) (*JournalCoordinatorV2, error) {
	if config.Facts == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "workflow journal Fact Port is required")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	return &JournalCoordinatorV2{config: config}, nil
}

type AdvanceStepRequestV2 struct {
	Plan       contract.WorkflowPlanV2        `json:"plan"`
	JournalID  string                         `json:"journal_id"`
	StepID     string                         `json:"step_id"`
	Target     contract.WorkflowStepStateV2   `json:"target"`
	Effect     *contract.ApplicationFactRefV2 `json:"effect,omitempty"`
	Settlement *contract.ApplicationFactRefV2 `json:"settlement,omitempty"`
	Reason     string                         `json:"reason,omitempty"`
}

// AdvanceStepV2 performs exactly one persistent step transition. A lost CAS
// reply is recovered by exact Journal Inspect; it never repeats provider work.
func (c *JournalCoordinatorV2) AdvanceStepV2(ctx context.Context, request AdvanceStepRequestV2) (contract.WorkflowJournalV2, error) {
	if strings.TrimSpace(request.JournalID) == "" || strings.TrimSpace(request.StepID) == "" {
		return contract.WorkflowJournalV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "journal and step ids are required")
	}
	if err := request.Plan.Validate(time.Time{}); err != nil {
		return contract.WorkflowJournalV2{}, err
	}
	current, err := c.config.Facts.InspectWorkflowJournalV2(ctx, request.Plan.Target, request.JournalID)
	if err != nil {
		return contract.WorkflowJournalV2{}, err
	}
	index := -1
	for candidate := range current.Steps {
		if current.Steps[candidate].StepID == request.StepID {
			index = candidate
			break
		}
	}
	if index < 0 {
		return contract.WorkflowJournalV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "workflow step not found")
	}
	if request.Target == contract.StepCompletedV2 {
		for _, planned := range request.Plan.Steps {
			if planned.ID == request.StepID && planned.ExecutionClass == contract.StepGovernedEffectV2 {
				return contract.WorkflowJournalV2{}, core.NewError(core.ErrorForbidden, core.ReasonEffectAuthorizationMissing, "legacy raw Journal completion cannot complete a governed Effect")
			}
		}
	}
	if request.Target == contract.StepReadyV2 || request.Target == contract.StepDispatchIntentV2 || request.Target == contract.StepCompletedV2 && current.Steps[index].State == contract.StepReadyV2 {
		if err := request.Plan.Validate(c.config.Clock()); err != nil {
			return contract.WorkflowJournalV2{}, err
		}
	}
	if stepAlreadyMatchesV2(current.Steps[index], request) {
		return current, nil
	}
	next := current
	next.Steps = append([]contract.WorkflowStepProgressV2(nil), current.Steps...)
	next.Revision++
	now := c.config.Clock().UnixNano()
	if now < current.UpdatedUnixNano {
		return contract.WorkflowJournalV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "workflow coordinator clock regressed")
	}
	next.UpdatedUnixNano = now
	progress := &next.Steps[index]
	progress.State, progress.UpdatedUnixNano = request.Target, now
	progress.Effect, progress.Settlement, progress.LastError = request.Effect, request.Settlement, request.Reason
	switch request.Target {
	case contract.StepReadyV2:
		progress.Attempt, progress.Effect, progress.Settlement, progress.LastError = 0, nil, nil, ""
	case contract.StepDispatchIntentV2:
		progress.Attempt = current.Steps[index].Attempt + 1
		if progress.Attempt == 0 {
			progress.Attempt = 1
		}
	case contract.StepWaitingInspectV2:
		progress.Attempt = current.Steps[index].Attempt
		progress.Effect = current.Steps[index].Effect
		progress.Settlement = nil
	case contract.StepCompletedV2:
		progress.Attempt = current.Steps[index].Attempt
		if progress.Attempt == 0 {
			progress.Attempt = 1
		}
		if progress.Effect == nil {
			progress.Effect = current.Steps[index].Effect
		}
		progress.LastError = ""
	case contract.StepSkippedV2:
		progress.Attempt, progress.Effect, progress.Settlement = 0, nil, nil
	default:
		return contract.WorkflowJournalV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "unsupported workflow step target")
	}
	next.Status = contract.DeriveWorkflowStatusV2(next.Steps)
	committed, err := c.config.Facts.CompareAndSwapWorkflowJournalV2(ctx, request.Plan, applicationports.WorkflowJournalCASRequestV2{ExpectedRevision: current.Revision, Next: next})
	if err != nil {
		if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorConflict) {
			return contract.WorkflowJournalV2{}, err
		}
		committed, err = c.config.Facts.InspectWorkflowJournalV2(context.WithoutCancel(ctx), request.Plan.Target, request.JournalID)
		if err != nil {
			return contract.WorkflowJournalV2{}, err
		}
	}
	if !sameJournalV2(committed, next, request.Plan) {
		return contract.WorkflowJournalV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "workflow step CAS recovery differs from requested transition")
	}
	return committed, nil
}

func stepAlreadyMatchesV2(progress contract.WorkflowStepProgressV2, request AdvanceStepRequestV2) bool {
	if progress.State != request.Target {
		return false
	}
	switch request.Target {
	case contract.StepReadyV2:
		return true
	case contract.StepDispatchIntentV2:
		return sameApplicationFactRefV2(progress.Effect, request.Effect)
	case contract.StepWaitingInspectV2:
		return request.Effect == nil || sameApplicationFactRefV2(progress.Effect, request.Effect)
	case contract.StepCompletedV2:
		return sameApplicationFactRefV2(progress.Settlement, request.Settlement)
	case contract.StepSkippedV2:
		return progress.LastError == request.Reason
	default:
		return false
	}
}

func sameApplicationFactRefV2(left, right *contract.ApplicationFactRefV2) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

// RefreshReadyStepsV2 advances only dependency-satisfied pending steps. Each
// ready transition is separately journaled so restart can resume the scan.
func (c *JournalCoordinatorV2) RefreshReadyStepsV2(ctx context.Context, plan contract.WorkflowPlanV2, journalID string) (contract.WorkflowJournalV2, error) {
	if err := plan.Validate(c.config.Clock()); err != nil {
		return contract.WorkflowJournalV2{}, err
	}
	current, err := c.config.Facts.InspectWorkflowJournalV2(ctx, plan.Target, journalID)
	if err != nil {
		return contract.WorkflowJournalV2{}, err
	}
	for {
		advanced := false
		byID := make(map[string]contract.WorkflowStepProgressV2, len(current.Steps))
		for _, progress := range current.Steps {
			byID[progress.StepID] = progress
		}
		for index, step := range plan.Steps {
			if current.Steps[index].State != contract.StepPendingV2 {
				continue
			}
			ready := true
			for _, dependency := range step.Dependencies {
				ready = ready && contract.DependencySatisfiedV2(byID[dependency].State)
			}
			if !ready {
				continue
			}
			current, err = c.AdvanceStepV2(ctx, AdvanceStepRequestV2{Plan: plan, JournalID: journalID, StepID: step.ID, Target: contract.StepReadyV2})
			if err != nil {
				return contract.WorkflowJournalV2{}, err
			}
			advanced = true
			break
		}
		if !advanced {
			return current, nil
		}
	}
}

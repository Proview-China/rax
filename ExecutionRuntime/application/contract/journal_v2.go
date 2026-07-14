package contract

import (
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type WorkflowStatusV2 string

const (
	WorkflowAcceptedV2       WorkflowStatusV2 = "accepted"
	WorkflowDispatchingV2    WorkflowStatusV2 = "dispatching"
	WorkflowWaitingInspectV2 WorkflowStatusV2 = "waiting_inspect"
	WorkflowCompletedV2      WorkflowStatusV2 = "completed"
	WorkflowIndeterminateV2  WorkflowStatusV2 = "indeterminate"
)

type WorkflowStepStateV2 string

const (
	StepPendingV2        WorkflowStepStateV2 = "pending"
	StepReadyV2          WorkflowStepStateV2 = "ready"
	StepDispatchIntentV2 WorkflowStepStateV2 = "dispatch_intent"
	StepWaitingInspectV2 WorkflowStepStateV2 = "waiting_inspect"
	StepCompletedV2      WorkflowStepStateV2 = "completed"
	StepSkippedV2        WorkflowStepStateV2 = "skipped"
	StepIndeterminateV2  WorkflowStepStateV2 = "indeterminate"
	StepBlockedV2        WorkflowStepStateV2 = "blocked"
)

type ApplicationFactRefV2 struct {
	Ref      string        `json:"ref"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r ApplicationFactRefV2) Validate() error {
	if strings.TrimSpace(r.Ref) == "" || len(r.Ref) > 512 || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "application fact ref and revision are required")
	}
	return r.Digest.Validate()
}

type WorkflowStepProgressV2 struct {
	StepID          string                `json:"step_id"`
	State           WorkflowStepStateV2   `json:"state"`
	Attempt         uint32                `json:"attempt"`
	Effect          *ApplicationFactRefV2 `json:"effect,omitempty"`
	Settlement      *ApplicationFactRefV2 `json:"settlement,omitempty"`
	LastError       string                `json:"last_error,omitempty"`
	UpdatedUnixNano int64                 `json:"updated_unix_nano"`
}

func (p WorkflowStepProgressV2) Validate() error {
	if strings.TrimSpace(p.StepID) == "" || len(p.StepID) > 256 || p.UpdatedUnixNano <= 0 || len(p.LastError) > 1024 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "workflow step progress identity and time are required")
	}
	if p.Effect != nil {
		if err := p.Effect.Validate(); err != nil {
			return err
		}
	}
	if p.Settlement != nil {
		if err := p.Settlement.Validate(); err != nil {
			return err
		}
	}
	switch p.State {
	case StepPendingV2, StepReadyV2:
		if p.Attempt != 0 || p.Effect != nil || p.Settlement != nil || p.LastError != "" {
			return invalidProgressFieldsV2()
		}
	case StepDispatchIntentV2:
		if p.Attempt == 0 || p.Effect == nil || p.Settlement != nil || p.LastError != "" {
			return invalidProgressFieldsV2()
		}
	case StepWaitingInspectV2:
		if p.Attempt == 0 || p.Effect == nil || p.Settlement != nil {
			return invalidProgressFieldsV2()
		}
	case StepCompletedV2:
		if p.Attempt == 0 || p.Settlement == nil || p.LastError != "" {
			return invalidProgressFieldsV2()
		}
	case StepSkippedV2:
		if p.Attempt != 0 || p.Effect != nil || p.Settlement != nil || strings.TrimSpace(p.LastError) == "" {
			return invalidProgressFieldsV2()
		}
	case StepIndeterminateV2:
		if p.Attempt == 0 || p.Effect == nil || p.Settlement != nil || strings.TrimSpace(p.LastError) == "" {
			return invalidProgressFieldsV2()
		}
	case StepBlockedV2:
		if p.Attempt != 0 || p.Effect != nil || strings.TrimSpace(p.LastError) == "" || p.Settlement != nil {
			return invalidProgressFieldsV2()
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "unknown workflow step state")
	}
	return nil
}

func invalidProgressFieldsV2() error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "workflow step state fields are inconsistent")
}

type WorkflowJournalV2 struct {
	ContractVersion string                   `json:"contract_version"`
	ID              string                   `json:"id"`
	Revision        core.Revision            `json:"revision"`
	CommandID       string                   `json:"command_id"`
	PlanID          string                   `json:"plan_id"`
	PlanRevision    core.Revision            `json:"plan_revision"`
	PlanDigest      core.Digest              `json:"plan_digest"`
	Status          WorkflowStatusV2         `json:"status"`
	Steps           []WorkflowStepProgressV2 `json:"steps"`
	CreatedUnixNano int64                    `json:"created_unix_nano"`
	UpdatedUnixNano int64                    `json:"updated_unix_nano"`
}

func NewWorkflowJournalV2(id string, plan WorkflowPlanV2, nowUnixNano int64) (WorkflowJournalV2, error) {
	digest, err := plan.DigestV2()
	if err != nil {
		return WorkflowJournalV2{}, err
	}
	steps := make([]WorkflowStepProgressV2, len(plan.Steps))
	for index, step := range plan.Steps {
		state := StepPendingV2
		if len(step.Dependencies) == 0 {
			state = StepReadyV2
		}
		steps[index] = WorkflowStepProgressV2{StepID: step.ID, State: state, UpdatedUnixNano: nowUnixNano}
	}
	journal := WorkflowJournalV2{ContractVersion: WorkflowContractVersionV2, ID: id, Revision: 1, CommandID: plan.CommandID, PlanID: plan.ID, PlanRevision: plan.Revision, PlanDigest: digest, Status: WorkflowAcceptedV2, Steps: steps, CreatedUnixNano: nowUnixNano, UpdatedUnixNano: nowUnixNano}
	return journal, journal.ValidateFor(plan)
}

func (j WorkflowJournalV2) ValidateFor(plan WorkflowPlanV2) error {
	if j.ContractVersion != WorkflowContractVersionV2 || strings.TrimSpace(j.ID) == "" || len(j.ID) > 512 || j.Revision == 0 || j.CommandID != plan.CommandID || j.PlanID != plan.ID || j.PlanRevision != plan.Revision || j.CreatedUnixNano <= 0 || j.UpdatedUnixNano < j.CreatedUnixNano || len(j.Steps) != len(plan.Steps) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "workflow journal identity, plan and timestamps are incomplete")
	}
	planDigest, err := plan.DigestV2()
	if err != nil || planDigest != j.PlanDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonPlanInvalid, "workflow journal plan digest drifted")
	}
	for index, progress := range j.Steps {
		if err := progress.Validate(); err != nil {
			return err
		}
		if progress.StepID != plan.Steps[index].ID {
			return core.NewError(core.ErrorConflict, core.ReasonPlanInvalid, "workflow journal step order differs from plan")
		}
	}
	if j.Status != DeriveWorkflowStatusV2(j.Steps) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "workflow journal status differs from step facts")
	}
	return nil
}

func (j WorkflowJournalV2) DigestV2(plan WorkflowPlanV2) (core.Digest, error) {
	if err := j.ValidateFor(plan); err != nil {
		return "", err
	}
	if j.Steps == nil {
		j.Steps = []WorkflowStepProgressV2{}
	}
	return core.CanonicalJSONDigest("praxis.application.workflow", WorkflowContractVersionV2, "WorkflowJournalV2", j)
}

func ValidateWorkflowJournalTransitionV2(plan WorkflowPlanV2, current, next WorkflowJournalV2) error {
	if err := current.ValidateFor(plan); err != nil {
		return err
	}
	if err := next.ValidateFor(plan); err != nil {
		return err
	}
	if current.ID != next.ID || current.CommandID != next.CommandID || current.PlanID != next.PlanID || current.PlanRevision != next.PlanRevision || current.PlanDigest != next.PlanDigest || current.CreatedUnixNano != next.CreatedUnixNano || next.Revision != current.Revision+1 || next.UpdatedUnixNano < current.UpdatedUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "workflow journal immutable identity or revision changed")
	}
	changed := -1
	for index := range current.Steps {
		if sameStepProgressV2(current.Steps[index], next.Steps[index]) {
			continue
		}
		if changed >= 0 {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "one journal CAS may change only one step")
		}
		changed = index
	}
	if changed < 0 {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "workflow journal CAS cannot create a fake revision")
	}
	before, after := current.Steps[changed], next.Steps[changed]
	if before.StepID != after.StepID || after.UpdatedUnixNano < before.UpdatedUnixNano || !allowedStepTransitionV2(before.State, after.State) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "workflow step transition is not monotonic")
	}
	if before.State == StepReadyV2 {
		switch plan.Steps[changed].ExecutionClass {
		case StepCoordinationV2:
			if after.State == StepDispatchIntentV2 {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "coordination step cannot enter provider dispatch")
			}
		case StepGovernedEffectV2:
			if after.State == StepCompletedV2 {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "governed effect cannot complete before dispatch and settlement")
			}
		}
	}
	if after.State == StepCompletedV2 {
		switch plan.Steps[changed].ExecutionClass {
		case StepGovernedEffectV2:
			if before.Effect == nil || after.Effect == nil || !sameApplicationFactRefValueV2(before.Effect, after.Effect) {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectStateConflict, "governed step completion must preserve its exact write-ahead Effect")
			}
		case StepCoordinationV2:
			if after.Effect != nil {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectStateConflict, "coordination step completion cannot claim a provider Effect")
			}
		}
	}
	if after.State == StepReadyV2 {
		byID := make(map[string]WorkflowStepProgressV2, len(current.Steps))
		for _, progress := range current.Steps {
			byID[progress.StepID] = progress
		}
		for _, dependency := range plan.Steps[changed].Dependencies {
			if !DependencySatisfiedV2(byID[dependency].State) {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "workflow step became ready before dependencies completed")
			}
		}
	}
	return nil
}

// DependencySatisfiedV2 treats an explicitly skipped optional step as a
// resolved no-op. This keeps future optional component kinds from stranding
// unrelated required work when no compatible implementation is installed.
func DependencySatisfiedV2(state WorkflowStepStateV2) bool {
	return state == StepCompletedV2 || state == StepSkippedV2
}

func allowedStepTransitionV2(current, next WorkflowStepStateV2) bool {
	switch current {
	case StepPendingV2:
		return next == StepReadyV2 || next == StepSkippedV2
	case StepReadyV2:
		return next == StepDispatchIntentV2 || next == StepCompletedV2 || next == StepSkippedV2
	case StepDispatchIntentV2:
		return next == StepWaitingInspectV2 || next == StepCompletedV2
	case StepWaitingInspectV2:
		return next == StepCompletedV2
	default:
		return false
	}
}

func sameApplicationFactRefValueV2(left, right *ApplicationFactRefV2) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func DeriveWorkflowStatusV2(steps []WorkflowStepProgressV2) WorkflowStatusV2 {
	allCompleted := len(steps) > 0
	hasDispatch, hasWait, hasIndeterminate := false, false, false
	for _, step := range steps {
		allCompleted = allCompleted && (step.State == StepCompletedV2 || step.State == StepSkippedV2)
		hasDispatch = hasDispatch || step.State == StepDispatchIntentV2
		hasWait = hasWait || step.State == StepWaitingInspectV2
		hasIndeterminate = hasIndeterminate || step.State == StepIndeterminateV2 || step.State == StepBlockedV2
	}
	if allCompleted {
		return WorkflowCompletedV2
	}
	if hasIndeterminate {
		return WorkflowIndeterminateV2
	}
	if hasWait {
		return WorkflowWaitingInspectV2
	}
	if hasDispatch {
		return WorkflowDispatchingV2
	}
	return WorkflowAcceptedV2
}

func sameStepProgressV2(a, b WorkflowStepProgressV2) bool {
	ad, _ := core.CanonicalJSONDigest("praxis.application.workflow", WorkflowContractVersionV2, "WorkflowStepProgressV2", a)
	bd, _ := core.CanonicalJSONDigest("praxis.application.workflow", WorkflowContractVersionV2, "WorkflowStepProgressV2", b)
	return ad == bd
}

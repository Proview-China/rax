package application

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type OutboxDispatcherConfigV2 struct {
	Commands    runtimeports.ApplicationCommandFactPortV2
	Submissions applicationports.SubmissionFactPortV2
	Journals    applicationports.WorkflowJournalFactPortV2
	StepCatalog applicationports.StepCatalogV2
	Clock       func() time.Time
}

type OutboxDispatcherV2 struct{ config OutboxDispatcherConfigV2 }

func NewOutboxDispatcherV2(config OutboxDispatcherConfigV2) (*OutboxDispatcherV2, error) {
	if config.Commands == nil || config.Submissions == nil || config.Journals == nil || config.StepCatalog == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "command, submission, journal and step catalog ports are required")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	return &OutboxDispatcherV2{config: config}, nil
}

type OutboxDispatchResultV2 struct {
	Outbox  runtimeports.ApplicationOutboxRecordV2 `json:"outbox"`
	Journal contract.WorkflowJournalV2             `json:"journal"`
}

// DispatchCommandV2 hands one accepted Outbox record to a persistent Journal.
// MarkOutboxDispatched is deliberately last; it never means a provider call or
// domain operation occurred.
func (d *OutboxDispatcherV2) DispatchCommandV2(ctx context.Context, scope core.ExecutionScope, commandID string) (OutboxDispatchResultV2, error) {
	if err := scope.Validate(); err != nil {
		return OutboxDispatchResultV2{}, err
	}
	if strings.TrimSpace(commandID) == "" {
		return OutboxDispatchResultV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "command id is required")
	}
	bundle, err := d.config.Submissions.InspectSubmissionBundleV2(ctx, scope, commandID)
	if err != nil {
		return OutboxDispatchResultV2{}, err
	}
	if err := bundle.Validate(d.config.Clock()); err != nil {
		return OutboxDispatchResultV2{}, err
	}
	if !runtimeports.SameExecutionScopeV2(bundle.Command.Target, scope) {
		return OutboxDispatchResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "dispatcher scope differs from submission")
	}
	commands, err := d.config.Commands.ListCommands(ctx, scope)
	if err != nil {
		return OutboxDispatchResultV2{}, err
	}
	accepted, found := findCommandV2(commands, commandID)
	if !found || (accepted.Status != runtimeports.ApplicationCommandAcceptedV2 && accepted.Status != runtimeports.ApplicationCommandExecutingV2) || !sameCommandEnvelopeV2(accepted.Envelope, bundle.Command) {
		return OutboxDispatchResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "accepted command fact differs from submission")
	}
	outboxes, err := d.config.Commands.ListOutbox(ctx, scope)
	if err != nil {
		return OutboxDispatchResultV2{}, err
	}
	outbox, found := findOutboxV2(outboxes, commandID)
	if !found || outbox.PayloadDigest != bundle.Payload.Payload.ContentDigest {
		return OutboxDispatchResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceConflict, "outbox record differs from immutable command payload")
	}

	unknownOptional, err := d.validateStepCatalogV2(ctx, bundle.Plan)
	if err != nil {
		return OutboxDispatchResultV2{}, err
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		return OutboxDispatchResultV2{}, err
	}
	journalID := "workflow-journal:" + string(scopeDigest) + ":" + commandID
	requested, err := contract.NewWorkflowJournalV2(journalID, bundle.Plan, outbox.RecordedAt.UnixNano())
	if err != nil {
		return OutboxDispatchResultV2{}, err
	}
	journal, err := d.config.Journals.CreateWorkflowJournalV2(ctx, bundle.Plan, requested)
	if err != nil {
		if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorConflict) {
			return OutboxDispatchResultV2{}, err
		}
		journal, err = d.config.Journals.InspectWorkflowJournalV2(context.WithoutCancel(ctx), scope, journalID)
		if err != nil {
			return OutboxDispatchResultV2{}, err
		}
	}
	if err := journal.ValidateFor(bundle.Plan); err != nil {
		return OutboxDispatchResultV2{}, err
	}
	if journal.ID != journalID {
		return OutboxDispatchResultV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "workflow journal store returned another journal identity")
	}
	if journal.Revision == 1 && !sameJournalV2(journal, requested, bundle.Plan) {
		return OutboxDispatchResultV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "existing workflow journal differs from outbox handoff")
	}
	for _, index := range unknownOptional {
		journal, err = d.skipOptionalStepV2(ctx, bundle.Plan, journal, index)
		if err != nil {
			return OutboxDispatchResultV2{}, err
		}
	}
	if outbox.Dispatched {
		return OutboxDispatchResultV2{Outbox: outbox, Journal: journal}, nil
	}
	marked, err := d.config.Commands.MarkOutboxDispatched(ctx, scope, commandID, outbox.Revision)
	if err != nil {
		if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorConflict) {
			return OutboxDispatchResultV2{}, err
		}
		outboxes, err = d.config.Commands.ListOutbox(context.WithoutCancel(ctx), scope)
		if err != nil {
			return OutboxDispatchResultV2{}, err
		}
		marked, found = findOutboxV2(outboxes, commandID)
		if !found {
			return OutboxDispatchResultV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "outbox disappeared during dispatch recovery")
		}
	}
	if !marked.Dispatched || marked.CommandID != outbox.CommandID || marked.PayloadDigest != outbox.PayloadDigest || marked.RecordedAt != outbox.RecordedAt {
		return OutboxDispatchResultV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "outbox dispatch recovery differs from handed-off record")
	}
	return OutboxDispatchResultV2{Outbox: marked, Journal: journal}, nil
}

func (d *OutboxDispatcherV2) validateStepCatalogV2(ctx context.Context, plan contract.WorkflowPlanV2) ([]int, error) {
	unknownOptional := []int{}
	for index, step := range plan.Steps {
		descriptor, err := d.config.StepCatalog.ResolveStepKindV2(ctx, step.Kind)
		if err != nil {
			// Only an authoritative "unknown kind" result may turn an optional
			// step into a no-op. A timeout, unavailable catalog or corrupt
			// descriptor remains recoverable and must never be mistaken for an
			// intentionally absent future module.
			if !core.HasReason(err, core.ReasonUnknownCapability) {
				return nil, err
			}
			if step.Required {
				return nil, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonUnknownCapability, "required workflow step kind is unavailable")
			}
			unknownOptional = append(unknownOptional, index)
			continue
		}
		if err := descriptor.ValidateCurrent(d.config.Clock()); err != nil {
			return nil, err
		}
		descriptorRef, err := descriptor.RefV2()
		if err != nil {
			return nil, err
		}
		if descriptorRef != step.Descriptor || descriptor.Kind != step.Kind || descriptor.ExecutionClass != step.ExecutionClass || !descriptorAcceptsSchemaV2(descriptor, step.Payload.Schema) {
			return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMismatch, "workflow step descriptor or schema drifted")
		}
		if step.ExecutionClass == contract.StepGovernedEffectV2 && (step.Provider == nil || step.Provider.Capability != descriptor.RequiredCapability) {
			return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMismatch, "workflow provider capability does not match the registered step kind")
		}
	}
	return unknownOptional, nil
}

func (d *OutboxDispatcherV2) skipOptionalStepV2(ctx context.Context, plan contract.WorkflowPlanV2, current contract.WorkflowJournalV2, index int) (contract.WorkflowJournalV2, error) {
	if current.Steps[index].State == contract.StepSkippedV2 {
		return current, nil
	}
	if current.Steps[index].State != contract.StepPendingV2 && current.Steps[index].State != contract.StepReadyV2 {
		return contract.WorkflowJournalV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidState, "optional unknown step was already advanced")
	}
	next := current
	next.Steps = append([]contract.WorkflowStepProgressV2(nil), current.Steps...)
	next.Revision++
	nowUnixNano := d.config.Clock().UnixNano()
	if nowUnixNano < current.UpdatedUnixNano {
		return contract.WorkflowJournalV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "workflow dispatcher clock regressed")
	}
	next.UpdatedUnixNano = nowUnixNano
	next.Steps[index].State = contract.StepSkippedV2
	next.Steps[index].LastError = "optional_step_kind_unavailable"
	next.Steps[index].UpdatedUnixNano = next.UpdatedUnixNano
	next.Status = contract.DeriveWorkflowStatusV2(next.Steps)
	committed, err := d.config.Journals.CompareAndSwapWorkflowJournalV2(ctx, plan, applicationports.WorkflowJournalCASRequestV2{ExpectedRevision: current.Revision, Next: next})
	if err != nil {
		if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorConflict) {
			return contract.WorkflowJournalV2{}, err
		}
		committed, err = d.config.Journals.InspectWorkflowJournalV2(context.WithoutCancel(ctx), plan.Target, current.ID)
		if err != nil {
			return contract.WorkflowJournalV2{}, err
		}
	}
	if !sameJournalV2(committed, next, plan) {
		return contract.WorkflowJournalV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "optional step CAS recovery differs")
	}
	return committed, nil
}

func descriptorAcceptsSchemaV2(descriptor applicationports.StepKindDescriptorV2, schema runtimeports.SchemaRefV2) bool {
	for _, allowed := range descriptor.Schemas {
		if allowed == schema {
			return true
		}
	}
	return false
}

func findCommandV2(records []runtimeports.ApplicationCommandRecordV2, id string) (runtimeports.ApplicationCommandRecordV2, bool) {
	for _, record := range records {
		if record.Envelope.ID == id {
			return record, true
		}
	}
	return runtimeports.ApplicationCommandRecordV2{}, false
}
func findOutboxV2(records []runtimeports.ApplicationOutboxRecordV2, id string) (runtimeports.ApplicationOutboxRecordV2, bool) {
	for _, record := range records {
		if record.CommandID == id {
			return record, true
		}
	}
	return runtimeports.ApplicationOutboxRecordV2{}, false
}

func sameCommandEnvelopeV2(a, b runtimeports.ApplicationCommandEnvelopeV2) bool {
	ad, _ := core.CanonicalJSONDigest("praxis.application.workflow", contract.WorkflowContractVersionV2, "CommandEnvelope", a)
	bd, _ := core.CanonicalJSONDigest("praxis.application.workflow", contract.WorkflowContractVersionV2, "CommandEnvelope", b)
	return ad == bd
}
func sameJournalV2(a, b contract.WorkflowJournalV2, plan contract.WorkflowPlanV2) bool {
	ad, err := a.DigestV2(plan)
	if err != nil {
		return false
	}
	bd, err := b.DigestV2(plan)
	return err == nil && ad == bd
}

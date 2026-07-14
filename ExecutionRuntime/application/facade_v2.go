package application

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type FacadeConfigV2 struct {
	Commands    runtimeports.ApplicationCommandFactPortV2
	Submissions applicationports.SubmissionFactPortV2
	Clock       func() time.Time
}

type FacadeV2 struct{ config FacadeConfigV2 }

func NewFacadeV2(config FacadeConfigV2) (*FacadeV2, error) {
	if config.Commands == nil || config.Submissions == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "command and submission fact ports are required")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	return &FacadeV2{config: config}, nil
}

type SubmitWorkflowRequestV2 struct {
	Bundle   contract.SubmissionBundleV2         `json:"bundle"`
	Mutation runtimeports.DesiredStateMutationV2 `json:"mutation"`
}

type SubmitWorkflowResultV2 struct {
	Bundle  contract.SubmissionBundleV2             `json:"bundle"`
	Command runtimeports.ApplicationCommandRecordV2 `json:"command"`
	Outbox  runtimeports.ApplicationOutboxRecordV2  `json:"outbox"`
}

// SubmitWorkflowV2 persists immutable payload/plan before accepting the
// Runtime command. A rejected command may leave an inert submission fact; it
// can never reach a provider because no accepted Outbox exists.
func (f *FacadeV2) SubmitWorkflowV2(ctx context.Context, request SubmitWorkflowRequestV2) (SubmitWorkflowResultV2, error) {
	if err := request.Bundle.Validate(f.config.Clock()); err != nil {
		return SubmitWorkflowResultV2{}, err
	}
	if err := request.Mutation.ValidateFor(request.Bundle.Command.Kind); err != nil {
		return SubmitWorkflowResultV2{}, err
	}
	created, err := f.config.Submissions.CreateSubmissionBundleV2(ctx, request.Bundle)
	if err != nil {
		if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorConflict) {
			return SubmitWorkflowResultV2{}, err
		}
		created, err = f.config.Submissions.InspectSubmissionBundleV2(context.WithoutCancel(ctx), request.Bundle.Command.Target, request.Bundle.Command.ID)
		if err != nil {
			return SubmitWorkflowResultV2{}, err
		}
	}
	if !sameSubmissionV2(created, request.Bundle) {
		return SubmitWorkflowResultV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "submission recovery differs from request")
	}
	acceptance, err := f.config.Commands.AcceptCommand(ctx, runtimeports.ApplicationCommandIntentV2{Envelope: request.Bundle.Command, Mutation: request.Mutation})
	if err == nil {
		return SubmitWorkflowResultV2{Bundle: created, Command: acceptance.Record, Outbox: acceptance.Outbox}, nil
	}
	if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorConflict) {
		return SubmitWorkflowResultV2{}, err
	}
	commands, inspectErr := f.config.Commands.ListCommands(context.WithoutCancel(ctx), request.Bundle.Command.Target)
	if inspectErr != nil {
		return SubmitWorkflowResultV2{}, err
	}
	record, found := findCommandV2(commands, request.Bundle.Command.ID)
	if !found || !sameCommandEnvelopeV2(record.Envelope, request.Bundle.Command) {
		return SubmitWorkflowResultV2{}, err
	}
	outboxes, inspectErr := f.config.Commands.ListOutbox(context.WithoutCancel(ctx), request.Bundle.Command.Target)
	if inspectErr != nil {
		return SubmitWorkflowResultV2{}, err
	}
	outbox, found := findOutboxV2(outboxes, request.Bundle.Command.ID)
	if !found || outbox.PayloadDigest != request.Bundle.Command.CanonicalPayloadDigest || outbox.Revision != record.Revision {
		return SubmitWorkflowResultV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "accepted command recovery lacks exact outbox")
	}
	return SubmitWorkflowResultV2{Bundle: created, Command: record, Outbox: outbox}, nil
}

func sameSubmissionV2(a, b contract.SubmissionBundleV2) bool {
	if !sameCommandEnvelopeV2(a.Command, b.Command) {
		return false
	}
	ap, err := a.Payload.DigestV2()
	if err != nil {
		return false
	}
	bp, err := b.Payload.DigestV2()
	if err != nil || ap != bp {
		return false
	}
	aPlan, err := a.Plan.DigestV2()
	if err != nil {
		return false
	}
	bPlan, err := b.Plan.DigestV2()
	return err == nil && aPlan == bPlan
}

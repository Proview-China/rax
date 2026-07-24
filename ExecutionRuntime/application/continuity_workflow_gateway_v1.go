package application

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type ContinuityWorkflowGatewayConfigV1 struct {
	Facade      *FacadeV2
	Assembler   applicationports.ContinuityWorkflowAssemblerV1
	Submissions applicationports.SubmissionFactPortV2
	Commands    runtimeports.ApplicationCommandFactPortV2
	Journals    applicationports.WorkflowJournalFactPortV2
	Clock       func() time.Time
}

type ContinuityWorkflowGatewayV1 struct {
	config ContinuityWorkflowGatewayConfigV1
}

var _ applicationports.ContinuityWorkflowSubmissionGatewayV1 = (*ContinuityWorkflowGatewayV1)(nil)

func NewContinuityWorkflowGatewayV1(config ContinuityWorkflowGatewayConfigV1) (*ContinuityWorkflowGatewayV1, error) {
	if isNilContinuityGatewayValueV1(config.Facade) || isNilContinuityGatewayValueV1(config.Assembler) || isNilContinuityGatewayValueV1(config.Submissions) || isNilContinuityGatewayValueV1(config.Commands) || isNilContinuityGatewayValueV1(config.Journals) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "facade, assembler and exact Application readers are required")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	return &ContinuityWorkflowGatewayV1{config: config}, nil
}

func (g *ContinuityWorkflowGatewayV1) SubmitContinuityWorkflowV1(ctx context.Context, request contract.ContinuityWorkflowRequestV1) (contract.ContinuityWorkflowInspectionV1, error) {
	if isNilContinuityGatewayValueV1(ctx) {
		return contract.ContinuityWorkflowInspectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "context is required")
	}
	if err := request.Validate(g.config.Clock()); err != nil {
		return contract.ContinuityWorkflowInspectionV1{}, err
	}
	assembly, err := g.config.Assembler.AssembleContinuityWorkflowV1(ctx, request)
	if err != nil {
		return contract.ContinuityWorkflowInspectionV1{}, err
	}
	if err := assembly.ValidateFor(request, g.config.Clock()); err != nil {
		return contract.ContinuityWorkflowInspectionV1{}, err
	}
	_, err = g.config.Facade.SubmitWorkflowV2(ctx, SubmitWorkflowRequestV2{Bundle: assembly.Bundle, Mutation: assembly.Mutation})
	if err != nil {
		return contract.ContinuityWorkflowInspectionV1{}, err
	}
	return g.inspectV1(context.WithoutCancel(ctx), request, assembly.RootStepID)
}

func (g *ContinuityWorkflowGatewayV1) InspectContinuityWorkflowV1(ctx context.Context, request contract.ContinuityWorkflowRequestV1) (contract.ContinuityWorkflowInspectionV1, error) {
	if isNilContinuityGatewayValueV1(ctx) {
		return contract.ContinuityWorkflowInspectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "context is required")
	}
	if err := request.Validate(time.Time{}); err != nil {
		return contract.ContinuityWorkflowInspectionV1{}, err
	}
	return g.inspectV1(ctx, request, "")
}

func (g *ContinuityWorkflowGatewayV1) inspectV1(ctx context.Context, request contract.ContinuityWorkflowRequestV1, expectedRootStepID string) (contract.ContinuityWorkflowInspectionV1, error) {
	bundle, err := g.config.Submissions.InspectSubmissionBundleV2(ctx, request.Target, request.RequestID)
	if err != nil {
		return contract.ContinuityWorkflowInspectionV1{}, err
	}
	rootStepID := expectedRootStepID
	if rootStepID == "" {
		rootStepID, err = continuityRootStepIDV1(bundle, request)
		if err != nil {
			return contract.ContinuityWorkflowInspectionV1{}, err
		}
	}
	if err := contract.ValidateContinuitySubmissionV1(request, bundle, rootStepID, time.Time{}); err != nil {
		return contract.ContinuityWorkflowInspectionV1{}, err
	}
	commands, err := g.config.Commands.ListCommands(ctx, request.Target)
	if err != nil {
		return contract.ContinuityWorkflowInspectionV1{}, err
	}
	command, found := findCommandV2(commands, request.RequestID)
	if !found || !sameCommandEnvelopeV2(command.Envelope, bundle.Command) {
		return contract.ContinuityWorkflowInspectionV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "workflow inspection lacks the exact accepted command")
	}
	outboxes, err := g.config.Commands.ListOutbox(ctx, request.Target)
	if err != nil {
		return contract.ContinuityWorkflowInspectionV1{}, err
	}
	outbox, found := findOutboxV2(outboxes, request.RequestID)
	if !found || outbox.Revision != command.Revision || outbox.PayloadDigest != bundle.Command.CanonicalPayloadDigest {
		return contract.ContinuityWorkflowInspectionV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "workflow inspection lacks the exact outbox")
	}
	result, err := continuityInspectionV1(request, bundle, command, outbox)
	if err != nil {
		return contract.ContinuityWorkflowInspectionV1{}, err
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(request.Target)
	if err != nil {
		return contract.ContinuityWorkflowInspectionV1{}, err
	}
	journalID := "workflow-journal:" + string(scopeDigest) + ":" + request.RequestID
	journal, err := g.config.Journals.InspectWorkflowJournalV2(ctx, request.Target, journalID)
	if err != nil {
		if core.HasCategory(err, core.ErrorNotFound) {
			return result, nil
		}
		return contract.ContinuityWorkflowInspectionV1{}, err
	}
	if err := journal.ValidateFor(bundle.Plan); err != nil {
		return contract.ContinuityWorkflowInspectionV1{}, err
	}
	journalDigest, err := journal.DigestV2(bundle.Plan)
	if err != nil {
		return contract.ContinuityWorkflowInspectionV1{}, err
	}
	journalRef := contract.ApplicationFactRefV2{Ref: journal.ID, Revision: journal.Revision, Digest: journalDigest}
	result.Journal = &journalRef
	result.Status = journal.Status
	return result, nil
}

func continuityRootStepIDV1(bundle contract.SubmissionBundleV2, request contract.ContinuityWorkflowRequestV1) (string, error) {
	body, err := request.CanonicalBodyV1()
	if err != nil {
		return "", err
	}
	root := ""
	for _, step := range bundle.Plan.Steps {
		if contract.ContinuityWorkflowKindV1(step.Kind) != request.Kind || step.Payload.Ref != "" || string(step.Payload.Inline) != string(body) {
			continue
		}
		if root != "" {
			return "", core.NewError(core.ErrorConflict, core.ReasonPlanInvalid, "workflow has multiple matching Continuity root steps")
		}
		root = step.ID
	}
	if root == "" {
		return "", core.NewError(core.ErrorPreconditionFailed, core.ReasonPlanInvalid, "workflow Continuity root step is absent")
	}
	return root, nil
}

func continuityInspectionV1(request contract.ContinuityWorkflowRequestV1, bundle contract.SubmissionBundleV2, command runtimeports.ApplicationCommandRecordV2, outbox runtimeports.ApplicationOutboxRecordV2) (contract.ContinuityWorkflowInspectionV1, error) {
	requestDigest, err := request.DigestV1()
	if err != nil {
		return contract.ContinuityWorkflowInspectionV1{}, err
	}
	submissionDigest, err := contract.SubmissionBundleDigestV1(bundle)
	if err != nil {
		return contract.ContinuityWorkflowInspectionV1{}, err
	}
	planDigest, err := bundle.Plan.DigestV2()
	if err != nil {
		return contract.ContinuityWorkflowInspectionV1{}, err
	}
	commandDigest, err := core.CanonicalJSONDigest("praxis.application.continuity-workflow", contract.ContinuityWorkflowContractVersionV1, "ApplicationCommandRecordV2", command)
	if err != nil {
		return contract.ContinuityWorkflowInspectionV1{}, err
	}
	outboxDigest, err := core.CanonicalJSONDigest("praxis.application.continuity-workflow", contract.ContinuityWorkflowContractVersionV1, "ApplicationOutboxRecordV2", outbox)
	if err != nil {
		return contract.ContinuityWorkflowInspectionV1{}, err
	}
	steps := make([]contract.ContinuityWorkflowStepRefV1, len(bundle.Plan.Steps))
	for index, step := range bundle.Plan.Steps {
		steps[index] = contract.ContinuityWorkflowStepRefV1{StepID: step.ID, Kind: step.Kind, Descriptor: step.Descriptor}
	}
	return contract.ContinuityWorkflowInspectionV1{
		RequestDigest: requestDigest,
		Submission:    contract.ApplicationFactRefV2{Ref: request.RequestID, Revision: 1, Digest: submissionDigest},
		Command:       contract.ApplicationFactRefV2{Ref: request.RequestID, Revision: command.Revision, Digest: commandDigest},
		Outbox:        contract.ApplicationFactRefV2{Ref: request.RequestID, Revision: outbox.Revision, Digest: outboxDigest},
		Plan:          contract.ApplicationFactRefV2{Ref: bundle.Plan.ID, Revision: bundle.Plan.Revision, Digest: planDigest},
		Status:        contract.WorkflowAcceptedV2,
		Steps:         steps,
	}, nil
}

func isNilContinuityGatewayValueV1(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

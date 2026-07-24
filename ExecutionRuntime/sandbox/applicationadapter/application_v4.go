package applicationadapter

import (
	"context"
	"errors"
	"strings"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

const sandboxLifecyclePlanVersionV4 = "praxis.sandbox/lifecycle-plan/v4"

type LifecyclePlanEnvelopeV4 struct {
	Ref  applicationcontract.SandboxLifecyclePlanRefV4
	Plan LifecyclePlanV4
}

func SealLifecyclePlanEnvelopeV4(id string, revision runtimecore.Revision, expires time.Time, plan LifecyclePlanV4) (LifecyclePlanEnvelopeV4, error) {
	if id == "" || revision == 0 || expires.IsZero() {
		return LifecyclePlanEnvelopeV4{}, errors.New("lifecycle plan identity is incomplete")
	}
	digest, err := lifecyclePlanDigestV4(plan)
	if err != nil {
		return LifecyclePlanEnvelopeV4{}, err
	}
	return LifecyclePlanEnvelopeV4{Ref: applicationcontract.SandboxLifecyclePlanRefV4{ID: id, Revision: revision, Digest: digest, ExpiresUnixNano: expires.UnixNano()}, Plan: plan}, nil
}

func (e LifecyclePlanEnvelopeV4) ValidateCurrent(expected applicationcontract.SandboxLifecyclePlanRefV4, now time.Time) error {
	if e.Ref != expected || e.Ref.ValidateCurrent(now) != nil {
		return errors.New("Sandbox lifecycle plan reader returned another exact ref")
	}
	digest, err := lifecyclePlanDigestV4(e.Plan)
	if err != nil || digest != e.Ref.Digest {
		return errors.New("Sandbox lifecycle plan content drifted")
	}
	return nil
}

type LifecyclePlanReaderV4 interface {
	InspectLifecyclePlanV4(context.Context, applicationcontract.SandboxLifecyclePlanRefV4) (LifecyclePlanEnvelopeV4, error)
}

type LifecycleApplicationResultStoreV4 interface {
	CreateLifecycleApplicationResultV4(context.Context, applicationcontract.SandboxLifecycleResultV4) (applicationcontract.SandboxLifecycleResultV4, error)
	InspectLifecycleApplicationResultV4(context.Context, string) (applicationcontract.SandboxLifecycleResultV4, error)
}

type ApplicationLifecycleV4 struct {
	flow      *LifecycleFlowV4
	workspace *WorkspaceCommitFlowV1
	plans     LifecyclePlanReaderV4
	results   LifecycleApplicationResultStoreV4
	now       func() time.Time
}

// WithWorkspaceCommitV1 installs the dedicated workspace actual-point closure.
// Without it, workspace-commit remains fail-closed while other lifecycle
// Effects retain the frozen V4 behavior.
func (a *ApplicationLifecycleV4) WithWorkspaceCommitV1(flow *WorkspaceCommitFlowV1) (*ApplicationLifecycleV4, error) {
	if a == nil || flow == nil {
		return nil, errors.New("Application workspace commit flow is required")
	}
	a.workspace = flow
	return a, nil
}

func NewApplicationLifecycleV4(flow *LifecycleFlowV4, plans LifecyclePlanReaderV4, results LifecycleApplicationResultStoreV4, now func() time.Time) (*ApplicationLifecycleV4, error) {
	if flow == nil || nilLike(plans) || nilLike(results) || nilLike(now) {
		return nil, errors.New("Application lifecycle adapter requires flow, plan reader, result store, and clock")
	}
	return &ApplicationLifecycleV4{flow: flow, plans: plans, results: results, now: now}, nil
}

var _ applicationports.SandboxLifecyclePortV4 = (*ApplicationLifecycleV4)(nil)

func (a *ApplicationLifecycleV4) StartOrInspectSandboxLifecycleV4(ctx context.Context, request applicationcontract.SandboxLifecycleRequestV4) (applicationcontract.SandboxLifecycleResultV4, error) {
	if a == nil || nilLike(ctx) {
		return applicationcontract.SandboxLifecycleResultV4{}, errors.New("Application lifecycle adapter or context is nil")
	}
	if err := request.ValidateCurrent(a.now()); err != nil {
		return applicationcontract.SandboxLifecycleResultV4{}, err
	}
	if existing, err := a.results.InspectLifecycleApplicationResultV4(ctx, request.ID); err == nil {
		return existing, validateApplicationResultV4(existing, request, a.now())
	} else if !errors.Is(err, sandboxports.ErrNotFound) {
		return applicationcontract.SandboxLifecycleResultV4{}, err
	}
	envelope, err := a.plans.InspectLifecyclePlanV4(ctx, request.Plan)
	if err != nil {
		return applicationcontract.SandboxLifecycleResultV4{}, err
	}
	if err := envelope.ValidateCurrent(request.Plan, a.now()); err != nil {
		return applicationcontract.SandboxLifecycleResultV4{}, err
	}
	if !runtimeports.SameOperationSubjectV3(envelope.Plan.Prepare.Enforcement.Operation, request.Operation) || envelope.Plan.Prepare.Enforcement.EffectID != request.EffectID || envelope.Plan.Prepare.Enforcement.AttemptID != request.AttemptID {
		return applicationcontract.SandboxLifecycleResultV4{}, errors.New("Application request binds another Sandbox lifecycle plan")
	}
	var completed LifecycleResultV4
	if envelope.Plan.Prepare.EffectKind == "praxis.sandbox/workspace-commit" {
		if a.workspace == nil {
			return applicationcontract.SandboxLifecycleResultV4{}, errors.New("workspace commit production closure is not configured")
		}
		payload, payloadErr := workspaceCommitPayloadFromPlanV1(envelope.Plan.Prepare)
		if payloadErr != nil {
			return applicationcontract.SandboxLifecycleResultV4{}, payloadErr
		}
		workspaceResult, workspaceErr := a.workspace.StartOrInspectWorkspaceCommitV1(ctx, WorkspaceCommitPlanV1{
			Lifecycle:         envelope.Plan,
			ExpectedView:      contract.Ref{ID: payload.View.ID, Revision: payload.View.Revision, Digest: strings.TrimPrefix(payload.View.Digest, "sha256:")},
			ExpectedChangeSet: contract.Ref{ID: payload.ChangeSet.ID, Revision: payload.ChangeSet.Revision, Digest: strings.TrimPrefix(payload.ChangeSet.Digest, "sha256:")},
		})
		if workspaceErr != nil {
			return applicationcontract.SandboxLifecycleResultV4{}, workspaceErr
		}
		completed = workspaceResult.Lifecycle
	} else {
		completed, err = a.flow.StartOrInspectLifecycleV4(ctx, envelope.Plan)
	}
	if err != nil {
		return applicationcontract.SandboxLifecycleResultV4{}, err
	}
	now := a.now()
	expires := minimumInt64(request.Plan.ExpiresUnixNano, completed.Settlement.ExpiresUnixNano, completed.DomainResult.Meta.ExpiresUnixNano)
	result, err := applicationcontract.SealSandboxLifecycleResultV4(applicationcontract.SandboxLifecycleResultV4{
		ID: request.ID, RequestDigest: request.Digest, Plan: request.Plan,
		DomainResult: completed.RuntimeFact, Settlement: completed.Settlement,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
	}, now)
	if err != nil {
		return applicationcontract.SandboxLifecycleResultV4{}, err
	}
	stored, err := a.results.CreateLifecycleApplicationResultV4(ctx, result)
	if err != nil {
		recovered, inspectErr := a.results.InspectLifecycleApplicationResultV4(context.WithoutCancel(ctx), request.ID)
		if inspectErr != nil || validateApplicationResultV4(recovered, request, a.now()) != nil {
			return applicationcontract.SandboxLifecycleResultV4{}, err
		}
		stored = recovered
	}
	return stored, validateApplicationResultV4(stored, request, a.now())
}

func (a *ApplicationLifecycleV4) InspectSandboxLifecycleV4(ctx context.Context, request applicationcontract.SandboxLifecycleRequestV4) (applicationcontract.SandboxLifecycleResultV4, error) {
	if a == nil || nilLike(ctx) {
		return applicationcontract.SandboxLifecycleResultV4{}, errors.New("Application lifecycle adapter or context is nil")
	}
	if err := request.ValidateCurrent(a.now()); err != nil {
		return applicationcontract.SandboxLifecycleResultV4{}, err
	}
	result, err := a.results.InspectLifecycleApplicationResultV4(ctx, request.ID)
	if err != nil {
		return applicationcontract.SandboxLifecycleResultV4{}, err
	}
	return result, validateApplicationResultV4(result, request, a.now())
}

func validateApplicationResultV4(result applicationcontract.SandboxLifecycleResultV4, request applicationcontract.SandboxLifecycleRequestV4, now time.Time) error {
	if err := result.ValidateCurrent(now); err != nil {
		return err
	}
	if result.ID != request.ID || result.RequestDigest != request.Digest || result.Plan != request.Plan {
		return errors.New("Application lifecycle result binds another request")
	}
	return nil
}

func lifecyclePlanDigestV4(plan LifecyclePlanV4) (runtimecore.Digest, error) {
	return runtimecore.CanonicalJSONDigest("praxis.sandbox.lifecycle-plan", sandboxLifecyclePlanVersionV4, "LifecyclePlanV4", plan)
}

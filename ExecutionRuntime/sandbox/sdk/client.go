// Package sdk provides a governed in-process Sandbox client. It never exposes
// Provider handles or raw FactStore mutation methods.
package sdk

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"sort"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type FactReader interface {
	GetReservation(context.Context, string) (contract.DomainReservation, error)
	GetInspection(context.Context, string) (contract.InspectionFact, error)
	GetDomainResult(context.Context, string) (contract.SandboxDomainResultFact, error)
	GetProjection(context.Context, string) (contract.EnvironmentProjection, error)
}

type Config struct {
	Backends         []contract.BackendDescriptor
	Facts            FactReader
	Lifecycle        applicationports.SandboxLifecyclePortV4
	WorkspaceCapture ports.WorkspaceChangeSetCapturePortV1
	Checkpoint       applicationports.CheckpointParticipantDriverV1
	SnapshotArtifact ports.SnapshotArtifactOwnerPortV2
	WorkspaceRestore ports.WorkspaceRestoreOwnerPortV1
	WorkspaceRewind  ports.WorkspaceRewindCompositionPortV1
	Clock            func() time.Time
}

type Client struct {
	backends         []contract.BackendDescriptor
	facts            FactReader
	lifecycle        applicationports.SandboxLifecyclePortV4
	workspaceCapture ports.WorkspaceChangeSetCapturePortV1
	checkpoint       applicationports.CheckpointParticipantDriverV1
	snapshotArtifact ports.SnapshotArtifactOwnerPortV2
	workspaceRestore ports.WorkspaceRestoreOwnerPortV1
	workspaceRewind  ports.WorkspaceRewindCompositionPortV1
	clock            func() time.Time
}

func New(config Config) (*Client, error) {
	if len(config.Backends) == 0 || nilLike(config.Facts) || nilLike(config.Lifecycle) || nilLike(config.WorkspaceCapture) || config.Clock == nil {
		return nil, errors.New("sandbox SDK dependencies are required")
	}
	now := config.Clock()
	backends := make([]contract.BackendDescriptor, len(config.Backends))
	seen := make(map[string]struct{}, len(config.Backends))
	for index, backend := range config.Backends {
		if err := backend.ValidateCurrent(now); err != nil {
			return nil, err
		}
		if _, exists := seen[backend.Meta.ID]; exists {
			return nil, errors.New("sandbox SDK backend ID is duplicated")
		}
		seen[backend.Meta.ID] = struct{}{}
		backends[index] = cloneBackend(backend)
	}
	sort.Slice(backends, func(i, j int) bool { return backends[i].Meta.ID < backends[j].Meta.ID })
	return &Client{
		backends: backends, facts: config.Facts, lifecycle: config.Lifecycle,
		workspaceCapture: config.WorkspaceCapture, checkpoint: config.Checkpoint,
		snapshotArtifact: config.SnapshotArtifact, workspaceRestore: config.WorkspaceRestore,
		workspaceRewind: config.WorkspaceRewind,
		clock:           config.Clock,
	}, nil
}

func (c *Client) DescribeBackends() ([]contract.BackendDescriptor, error) {
	now := c.clock()
	result := make([]contract.BackendDescriptor, len(c.backends))
	for index, backend := range c.backends {
		if err := backend.ValidateCurrent(now); err != nil {
			return nil, err
		}
		result[index] = cloneBackend(backend)
	}
	return result, nil
}

type PlacementInput struct {
	Backend   contract.BackendDescriptor
	Candidate contract.PlacementCandidate
}

type PlacementResult struct {
	BackendRef   contract.Ref
	CandidateRef contract.Ref
	Decision     kernel.AdmissionDecision
}

func (c *Client) MatchRequirement(requirement contract.ExecutionRequirement, policy contract.PolicyProjection, inputs []PlacementInput) ([]PlacementResult, error) {
	if len(inputs) == 0 {
		return nil, errors.New("placement candidates are required")
	}
	now := c.clock()
	result := make([]PlacementResult, 0, len(inputs))
	for _, input := range inputs {
		decision, err := kernel.EvaluatePlacement(now, requirement, policy, input.Backend, input.Candidate)
		if err != nil {
			return nil, err
		}
		result = append(result, PlacementResult{BackendRef: input.Backend.Meta.Ref(), CandidateRef: input.Candidate.Meta.Ref(), Decision: kernel.AdmissionDecision{Admitted: decision.Admitted, Reasons: slices.Clone(decision.Reasons)}})
	}
	return result, nil
}

func (c *Client) StartLifecycle(ctx context.Context, request applicationcontract.SandboxLifecycleRequestV4) (applicationcontract.SandboxLifecycleResultV4, error) {
	return c.lifecycle.StartOrInspectSandboxLifecycleV4(ctx, request)
}

func (c *Client) InspectLifecycle(ctx context.Context, request applicationcontract.SandboxLifecycleRequestV4) (applicationcontract.SandboxLifecycleResultV4, error) {
	return c.lifecycle.InspectSandboxLifecycleV4(ctx, request)
}

func (c *Client) CaptureWorkspaceChangeSet(ctx context.Context, request ports.CaptureWorkspaceChangeSetRequest) (contract.WorkspaceChangeSet, error) {
	return c.workspaceCapture.CaptureWorkspaceChangeSetV1(ctx, request)
}

func (c *Client) CompleteCheckpointParticipant(ctx context.Context, request applicationcontract.CheckpointParticipantWorkRequestV1) (applicationcontract.CheckpointParticipantCommitV1, error) {
	if nilLike(c.checkpoint) {
		return applicationcontract.CheckpointParticipantCommitV1{}, ports.ErrUnsupported
	}
	return c.checkpoint.CompleteCheckpointParticipantV1(ctx, request)
}

func (c *Client) InspectCheckpointParticipant(ctx context.Context, attempt runtimeports.CheckpointAttemptRefV2, participant runtimeports.CheckpointParticipantRefV2) (applicationcontract.CheckpointParticipantCommitV1, error) {
	if nilLike(c.checkpoint) {
		return applicationcontract.CheckpointParticipantCommitV1{}, ports.ErrUnsupported
	}
	return c.checkpoint.InspectCheckpointParticipantV1(ctx, attempt, participant)
}

func (c *Client) ReserveSnapshotArtifact(ctx context.Context, request *contract.ReserveArtifactRequestV2) (contract.ReserveArtifactResultV2, error) {
	if nilLike(c.snapshotArtifact) {
		return contract.ReserveArtifactResultV2{}, ports.ErrUnsupported
	}
	return c.snapshotArtifact.ReserveArtifact(ctx, request)
}

func (c *Client) CommitSnapshotArtifact(ctx context.Context, request *contract.CommitSnapshotArtifactRequestV2) (contract.CommitSnapshotArtifactResultV2, error) {
	if nilLike(c.snapshotArtifact) {
		return contract.CommitSnapshotArtifactResultV2{}, ports.ErrUnsupported
	}
	return c.snapshotArtifact.CommitArtifact(ctx, request)
}

func (c *Client) InspectSnapshotArtifact(ctx context.Context, request *contract.InspectSnapshotArtifactFactRequestV2) (contract.SnapshotArtifactFactV2, error) {
	if nilLike(c.snapshotArtifact) {
		return contract.SnapshotArtifactFactV2{}, ports.ErrUnsupported
	}
	return c.snapshotArtifact.InspectArtifactFact(ctx, request)
}

func (c *Client) StageWorkspaceRestore(ctx context.Context, request *contract.WorkspaceRestoreStageRequestV1) (contract.WorkspaceRestoreStageFactV1, error) {
	if nilLike(c.workspaceRestore) {
		return contract.WorkspaceRestoreStageFactV1{}, ports.ErrUnsupported
	}
	return c.workspaceRestore.StageWorkspaceV1(ctx, request)
}

func (c *Client) ReconcileWorkspaceRestore(ctx context.Context, request *contract.WorkspaceRestoreStageRequestV1) (contract.WorkspaceRestoreStageFactV1, error) {
	if nilLike(c.workspaceRestore) {
		return contract.WorkspaceRestoreStageFactV1{}, ports.ErrUnsupported
	}
	return c.workspaceRestore.ReconcileWorkspaceV1(ctx, request)
}

func (c *Client) ComposeWorkspaceRewindV1(ctx context.Context, request contract.ComposeWorkspaceRewindRequestV1) (ports.WorkspaceRewindCompositionResultV1, error) {
	if nilLike(c.workspaceRewind) {
		return ports.WorkspaceRewindCompositionResultV1{}, ports.ErrUnsupported
	}
	return c.workspaceRewind.ComposeWorkspaceRewindV1(ctx, request)
}

func (c *Client) InspectWorkspaceRewindV1(ctx context.Context, request contract.ComposeWorkspaceRewindRequestV1) (ports.WorkspaceRewindCompositionResultV1, error) {
	if nilLike(c.workspaceRewind) {
		return ports.WorkspaceRewindCompositionResultV1{}, ports.ErrUnsupported
	}
	return c.workspaceRewind.InspectWorkspaceRewindV1(ctx, request)
}

func (c *Client) InspectReservation(ctx context.Context, id string) (contract.DomainReservation, error) {
	return c.facts.GetReservation(ctx, id)
}

func (c *Client) InspectDomainResult(ctx context.Context, id string) (contract.SandboxDomainResultFact, error) {
	return c.facts.GetDomainResult(ctx, id)
}

func (c *Client) InspectEnvironment(ctx context.Context, leaseID string) (contract.EnvironmentProjection, error) {
	return c.facts.GetProjection(ctx, leaseID)
}

func (c *Client) InspectOperation(ctx context.Context, inspectionID string) (contract.InspectionFact, error) {
	return c.facts.GetInspection(ctx, inspectionID)
}

func (c *Client) InspectCleanup(ctx context.Context, inspectionID string) (contract.CleanupReport, error) {
	inspection, err := c.facts.GetInspection(ctx, inspectionID)
	if err != nil {
		return contract.CleanupReport{}, err
	}
	if inspection.Cleanup == nil {
		return contract.CleanupReport{}, ports.ErrNotFound
	}
	result := *inspection.Cleanup
	result.EvidenceRefs = slices.Clone(inspection.Cleanup.EvidenceRefs)
	return result, result.ValidateShape()
}

func (c *Client) InspectResiduals(ctx context.Context, inspectionID string) ([]contract.Residual, error) {
	inspection, err := c.facts.GetInspection(ctx, inspectionID)
	if err != nil {
		return nil, err
	}
	result := make([]contract.Residual, len(inspection.Residuals))
	for index, residual := range inspection.Residuals {
		result[index] = residual
		result[index].EvidenceRefs = slices.Clone(residual.EvidenceRefs)
	}
	return result, nil
}

func cloneBackend(value contract.BackendDescriptor) contract.BackendDescriptor {
	capabilities := value.Capabilities
	value.Capabilities = make(map[contract.BackendCapability]contract.CapabilityLevel, len(value.Capabilities))
	for key, level := range capabilities {
		value.Capabilities[key] = level
	}
	value.ResidualClasses = slices.Clone(value.ResidualClasses)
	return value
}

func nilLike(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

var _ ports.WorkspaceRewindCompositionPortV1 = (*Client)(nil)

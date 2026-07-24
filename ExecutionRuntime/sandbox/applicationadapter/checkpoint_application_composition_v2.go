package applicationadapter

import (
	"errors"
	"time"

	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type CheckpointWorkspaceApplicationConfigV2 struct {
	Production        *CheckpointProductionCompositionV2
	SnapshotStore     ports.SnapshotArtifactStoreV2
	WorkspaceStore    ports.WorkspaceCheckpointParticipantStoreV2
	CaptureBindings   CheckpointSnapshotCaptureBindingStoreV2
	CheckpointContent ports.CheckpointWorkspaceArtifactReaderV2
	SnapshotContent   ports.SnapshotContentStoreV2
	Plans             CheckpointPhaseExecutionPlanReaderV2
	SnapshotLimits    kernel.SnapshotArtifactOwnerLimits
	WorkspaceMaxTTL   time.Duration
	ParticipantID     string
	ParticipantOwner  runtimeports.ProviderBindingRefV2
	SnapshotOwner     runtimeports.ProviderBindingRefV2
	CoverageOwner     runtimeports.ProviderBindingRefV2
	ParticipantSchema runtimeports.SchemaRefV2
	SnapshotSchema    runtimeports.SchemaRefV2
	CoverageSchema    runtimeports.SchemaRefV2
}

// CheckpointWorkspaceApplicationCompositionV2 is the host wiring closure for
// one Sandbox workspace Participant. It composes only public Runtime gateways
// and Sandbox Owner ports. It does not own Runtime coordination, Continuity
// Manifest/consistency, or Provider authority.
type CheckpointWorkspaceApplicationCompositionV2 struct {
	SnapshotCommitCurrent *CheckpointSnapshotArtifactCurrentReaderV2
	SnapshotOwner         *kernel.SnapshotArtifactOwner
	WorkspaceCurrent      *WorkspaceCheckpointPreparationCurrentReaderV2
	WorkspaceOwner        *kernel.WorkspaceCheckpointParticipantOwnerV2
	Capture               *CheckpointSnapshotCaptureV2
	Lifecycle             *GovernedCheckpointParticipantPhaseLifecycleV2
	OwnerCurrent          *WorkspaceCheckpointOwnerCurrentAdapterV1
	Driver                *GovernedCheckpointParticipantApplicationAdapterV1
}

func NewCheckpointWorkspaceApplicationCompositionV2(config CheckpointWorkspaceApplicationConfigV2) (*CheckpointWorkspaceApplicationCompositionV2, error) {
	if config.Production == nil || config.Production.Clock == nil || nilLike(config.SnapshotStore) || nilLike(config.WorkspaceStore) || nilLike(config.CaptureBindings) || nilLike(config.CheckpointContent) || nilLike(config.SnapshotContent) || nilLike(config.Plans) || config.ParticipantID == "" || config.WorkspaceMaxTTL <= 0 {
		return nil, errors.New("checkpoint workspace Application composition dependencies are required")
	}
	clock := config.Production.Clock
	snapshotCurrent, err := NewCheckpointSnapshotArtifactCurrentReaderV2(config.Production, config.CaptureBindings, config.SnapshotStore, config.CheckpointContent, config.SnapshotContent, clock)
	if err != nil {
		return nil, err
	}
	snapshotOwner, err := kernel.NewSnapshotArtifactOwnerWithCommitCurrent(config.SnapshotStore, snapshotCurrent, clock, config.SnapshotLimits)
	if err != nil {
		return nil, err
	}
	workspaceCurrent, err := NewWorkspaceCheckpointPreparationCurrentReaderV2(config.Production, config.CaptureBindings, config.SnapshotStore, clock)
	if err != nil {
		return nil, err
	}
	workspaceOwner, err := kernel.NewWorkspaceCheckpointParticipantOwnerV2(config.WorkspaceStore, workspaceCurrent, clock, config.WorkspaceMaxTTL)
	if err != nil {
		return nil, err
	}
	capture, err := NewCheckpointSnapshotCaptureV2(config.Production, config.CaptureBindings, snapshotOwner, config.CheckpointContent, config.SnapshotContent, clock)
	if err != nil {
		return nil, err
	}
	lifecycle, err := NewGovernedCheckpointParticipantPhaseLifecycleV2(GovernedCheckpointParticipantPhaseLifecycleConfigV2{Composition: config.Production, Plans: config.Plans, Capture: capture, Workspace: workspaceOwner})
	if err != nil {
		return nil, err
	}
	ownerCurrent, err := NewWorkspaceCheckpointOwnerCurrentAdapterV1(WorkspaceCheckpointOwnerCurrentAdapterConfigV1{
		Prepared: workspaceOwner, Artifacts: snapshotOwner, ParticipantOwner: config.ParticipantOwner, SnapshotOwner: config.SnapshotOwner, CoverageOwner: config.CoverageOwner,
		ParticipantSchema: config.ParticipantSchema, SnapshotSchema: config.SnapshotSchema, CoverageSchema: config.CoverageSchema, Clock: clock,
	})
	if err != nil {
		return nil, err
	}
	driver, err := NewGovernedCheckpointParticipantApplicationAdapterV1(GovernedCheckpointParticipantApplicationAdapterConfigV1{ParticipantID: config.ParticipantID, Current: ownerCurrent, Lifecycle: lifecycle, Clock: clock})
	if err != nil {
		return nil, err
	}
	return &CheckpointWorkspaceApplicationCompositionV2{SnapshotCommitCurrent: snapshotCurrent, SnapshotOwner: snapshotOwner, WorkspaceCurrent: workspaceCurrent, WorkspaceOwner: workspaceOwner, Capture: capture, Lifecycle: lifecycle, OwnerCurrent: ownerCurrent, Driver: driver}, nil
}

var _ applicationports.CheckpointParticipantDriverV1 = (*GovernedCheckpointParticipantApplicationAdapterV1)(nil)

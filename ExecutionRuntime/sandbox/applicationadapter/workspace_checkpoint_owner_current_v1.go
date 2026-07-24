package applicationadapter

import (
	"context"
	"errors"
	"reflect"
	"time"

	appcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type WorkspaceCheckpointOwnerCurrentAdapterConfigV1 struct {
	Prepared          sandboxports.WorkspaceCheckpointParticipantOwnerPortV2
	Artifacts         sandboxports.SnapshotArtifactOwnerPortV2
	ParticipantOwner  runtimeports.ProviderBindingRefV2
	SnapshotOwner     runtimeports.ProviderBindingRefV2
	CoverageOwner     runtimeports.ProviderBindingRefV2
	ParticipantSchema runtimeports.SchemaRefV2
	SnapshotSchema    runtimeports.SchemaRefV2
	CoverageSchema    runtimeports.SchemaRefV2
	Clock             func() time.Time
}

type WorkspaceCheckpointOwnerCurrentAdapterV1 struct {
	config WorkspaceCheckpointOwnerCurrentAdapterConfigV1
}

func NewWorkspaceCheckpointOwnerCurrentAdapterV1(config WorkspaceCheckpointOwnerCurrentAdapterConfigV1) (*WorkspaceCheckpointOwnerCurrentAdapterV1, error) {
	if checkpointOwnerCurrentNilV1(config.Prepared) || checkpointOwnerCurrentNilV1(config.Artifacts) || config.ParticipantOwner.Validate() != nil || config.SnapshotOwner.Validate() != nil || config.CoverageOwner.Validate() != nil || config.ParticipantSchema.Validate() != nil || config.SnapshotSchema.Validate() != nil || config.CoverageSchema.Validate() != nil || config.Clock == nil {
		return nil, errors.New("workspace checkpoint Owner current adapter dependencies are required")
	}
	return &WorkspaceCheckpointOwnerCurrentAdapterV1{config: config}, nil
}

func (a *WorkspaceCheckpointOwnerCurrentAdapterV1) InspectCheckpointParticipantOwnerCurrentV1(ctx context.Context, work appcontract.CheckpointParticipantWorkRequestV1) (appcontract.CheckpointParticipantOwnerCandidateV1, error) {
	now := a.config.Clock()
	if err := work.Validate(now); err != nil {
		return appcontract.CheckpointParticipantOwnerCandidateV1{}, err
	}
	query := contract.InspectWorkspaceCheckpointPreparedRequestV2{TenantID: string(work.Gate.TenantID), ScopeDigest: string(work.Gate.ScopeDigest), CheckpointAttemptID: work.Attempt.ID, ParticipantID: work.Participant.ID}
	bundle, err := a.config.Prepared.InspectWorkspaceCheckpointPreparedV2(ctx, &query)
	if err != nil {
		return appcontract.CheckpointParticipantOwnerCandidateV1{}, err
	}
	if err := bundle.ValidateShape(); err != nil || bundle.Participant.ValidateCurrent(now) != nil || bundle.Coverage.ValidateCurrent(now) != nil || !workspaceCheckpointBundleMatchesWorkV1(bundle, work) {
		return appcontract.CheckpointParticipantOwnerCandidateV1{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "workspace checkpoint Owner facts do not match Application work")
	}
	artifact, err := a.config.Artifacts.InspectArtifactFact(ctx, &contract.InspectSnapshotArtifactFactRequestV2{ExpectedRef: bundle.Participant.SnapshotArtifactFactRef})
	if err != nil {
		return appcontract.CheckpointParticipantOwnerCandidateV1{}, err
	}
	if artifact.ValidateCurrent(now) != nil || artifact.TenantID != bundle.Participant.TenantID || artifact.DataDomain != contract.WorkspaceSnapshotDataDomain || !contract.SameSnapshotArtifactExactRef(artifact.ExactRef(), bundle.Participant.SnapshotArtifactFactRef) {
		return appcontract.CheckpointParticipantOwnerCandidateV1{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "workspace checkpoint Snapshot Artifact is not exact current workspace data")
	}
	aggregate, err := a.config.Artifacts.InspectAggregateCurrent(ctx, &contract.InspectSnapshotArtifactAggregateCurrentRequestV2{ArtifactAggregateID: artifact.ArtifactSubjectRef.ArtifactAggregateID, ExpectedAggregateRef: contract.SnapshotArtifactOptionalAggregateRefV2{Presence: contract.SnapshotArtifactPresent, Ref: &bundle.Participant.SnapshotAggregateRef}, RequestedNotAfter: work.NotAfter})
	if err != nil {
		return appcontract.CheckpointParticipantOwnerCandidateV1{}, err
	}
	if aggregate.ValidateCurrent(now) != nil || !contract.SameSnapshotArtifactAggregateRef(aggregate.HeadAggregateEnvelopeRef, bundle.Participant.SnapshotAggregateRef) || aggregate.ArtifactFactRef.Ref == nil || !contract.SameSnapshotArtifactExactRef(*aggregate.ArtifactFactRef.Ref, artifact.ExactRef()) {
		return appcontract.CheckpointParticipantOwnerCandidateV1{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "workspace checkpoint Snapshot Artifact is no longer Owner current")
	}
	participant := workspaceCheckpointAppExactRefV1(bundle.Participant.ExactRef(), bundle.Participant, a.config.ParticipantOwner, a.config.ParticipantSchema, "workspace_checkpoint_participant_fact_v2")
	snapshot := workspaceCheckpointAppExactRefV1(artifact.ExactRef(), bundle.Participant, a.config.SnapshotOwner, a.config.SnapshotSchema, "snapshot_artifact_fact_v2")
	coverage := workspaceCheckpointAppExactRefV1(bundle.Coverage.ExactRef(), bundle.Participant, a.config.CoverageOwner, a.config.CoverageSchema, "workspace_checkpoint_coverage_fact_v2")
	expires := minimumCheckpointNanosV1(work.NotAfter, bundle.Participant.Meta.ExpiresUnixNano, bundle.Coverage.Meta.ExpiresUnixNano, artifact.Meta.ExpiresUnixNano, aggregate.ExpiresUnixNano)
	candidate := appcontract.CheckpointParticipantOwnerCandidateV1{Participant: work.Participant, ParticipantFact: participant, Snapshot: snapshot, Coverage: coverage, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires}
	return appcontract.SealCheckpointParticipantOwnerCandidateV1(candidate, work, now)
}

func workspaceCheckpointBundleMatchesWorkV1(bundle contract.WorkspaceCheckpointPreparedBundleV2, work appcontract.CheckpointParticipantWorkRequestV1) bool {
	p := bundle.Participant
	return p.TenantID == string(work.Gate.TenantID) && p.ScopeDigest == string(work.Gate.ScopeDigest) && p.RunID == string(work.Gate.RunID) &&
		p.CheckpointAttemptRef.ID == work.Attempt.ID && p.CheckpointAttemptRef.Revision == uint64(work.Attempt.Revision) && p.CheckpointAttemptRef.Digest == string(work.Attempt.Digest) &&
		p.BarrierRef.ID == work.Barrier.ID && p.BarrierRef.Revision == uint64(work.Barrier.Revision) && p.BarrierRef.Digest == string(work.Barrier.Digest) &&
		p.EffectCutRef.ID == work.EffectCut.ID && p.EffectCutRef.Revision == uint64(work.EffectCut.Revision) && p.EffectCutRef.Digest == string(work.EffectCut.Digest) &&
		p.ParticipantID == work.Participant.ID && p.ParticipantDigest == string(work.Participant.Digest)
}

func workspaceCheckpointAppExactRefV1(ref contract.SnapshotArtifactExactRefV2, participant contract.WorkspaceCheckpointParticipantFactV2, owner runtimeports.ProviderBindingRefV2, schema runtimeports.SchemaRefV2, kind string) appcontract.CheckpointExternalExactRefV1 {
	return appcontract.CheckpointExternalExactRefV1{ContractVersion: contract.ContractFamily, ExactSchemaRef: ref.TypeURL, FactKind: kind, Schema: schema, Owner: owner, TenantID: core.TenantID(participant.TenantID), ScopeDigest: core.Digest(participant.ScopeDigest), RunID: core.AgentRunID(participant.RunID), ID: ref.ID, Revision: core.Revision(ref.Revision), Digest: core.Digest(contract.SnapshotArtifactDigestSHA256 + ":" + ref.Digest)}
}

func minimumCheckpointNanosV1(values ...int64) int64 {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

func checkpointOwnerCurrentNilV1(value any) bool {
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

var _ applicationports.CheckpointParticipantOwnerCurrentReaderV1 = (*WorkspaceCheckpointOwnerCurrentAdapterV1)(nil)

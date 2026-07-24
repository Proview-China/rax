package kernel

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type WorkspaceCheckpointParticipantOwnerV2 struct {
	store   ports.WorkspaceCheckpointParticipantStoreV2
	current ports.WorkspaceCheckpointPreparationCurrentReaderV2
	clock   func() time.Time
	maxTTL  time.Duration
}

func NewWorkspaceCheckpointParticipantOwnerV2(store ports.WorkspaceCheckpointParticipantStoreV2, current ports.WorkspaceCheckpointPreparationCurrentReaderV2, clock func() time.Time, maxTTL time.Duration) (*WorkspaceCheckpointParticipantOwnerV2, error) {
	if workspaceCheckpointNilV2(store) || workspaceCheckpointNilV2(current) || clock == nil || maxTTL <= 0 {
		return nil, errors.New("workspace checkpoint Participant Owner dependencies are required")
	}
	return &WorkspaceCheckpointParticipantOwnerV2{store: store, current: current, clock: clock, maxTTL: maxTTL}, nil
}

func (o *WorkspaceCheckpointParticipantOwnerV2) PrepareWorkspaceCheckpointParticipantV2(ctx context.Context, input *contract.PrepareWorkspaceCheckpointParticipantRequestV2) (contract.WorkspaceCheckpointPreparedBundleV2, error) {
	if input == nil {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, errors.New("prepare workspace checkpoint request is required")
	}
	request := *input
	now := o.clock()
	if err := request.ValidateCurrent(now); err != nil {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, err
	}
	if existing, err := o.inspectByRequest(ctx, request); err == nil {
		return existing, nil
	} else if !errors.Is(err, ports.ErrNotFound) {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, err
	}

	s1, err := o.current.InspectWorkspaceCheckpointPreparationCurrentV2(ctx, request)
	if err != nil {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, err
	}
	if err := validateWorkspaceCheckpointPreparationV2(s1, request, now); err != nil {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, err
	}
	if len(s1.ResidualRefs) != 0 {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, fmt.Errorf("%w: workspace checkpoint remains partial and diagnostic-only", ports.ErrConflict)
	}
	bundle, err := o.buildWorkspaceCheckpointPreparedV2(request, s1, now)
	if err != nil {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, err
	}

	fresh := o.clock()
	s2, err := o.current.InspectWorkspaceCheckpointPreparationCurrentV2(ctx, request)
	if err != nil {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, err
	}
	if err := validateWorkspaceCheckpointPreparationV2(s2, request, fresh); err != nil {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, err
	}
	if s1.ProjectionDigest != s2.ProjectionDigest {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, fmt.Errorf("%w: workspace checkpoint current changed between S1 and S2", ports.ErrConflict)
	}
	if fresh.UnixNano() >= bundle.Participant.Meta.ExpiresUnixNano {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, fmt.Errorf("%w: workspace checkpoint expired before commit", ports.ErrStale)
	}
	created, err := o.store.CommitWorkspaceCheckpointPreparedV2(ctx, bundle)
	if err == nil && created {
		return bundle.Clone(), nil
	}
	recovered, inspectErr := o.inspectByRequest(context.WithoutCancel(ctx), request)
	if inspectErr == nil && sameWorkspaceCheckpointPreparedV2(recovered, bundle) {
		return recovered, nil
	}
	if err != nil {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, err
	}
	if inspectErr != nil {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, inspectErr
	}
	return contract.WorkspaceCheckpointPreparedBundleV2{}, fmt.Errorf("%w: workspace checkpoint create-once winner differs", ports.ErrConflict)
}

func (o *WorkspaceCheckpointParticipantOwnerV2) InspectWorkspaceCheckpointPreparedV2(ctx context.Context, input *contract.InspectWorkspaceCheckpointPreparedRequestV2) (contract.WorkspaceCheckpointPreparedBundleV2, error) {
	if input == nil {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, errors.New("workspace checkpoint prepared inspect request is required")
	}
	if err := input.Validate(); err != nil {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, err
	}
	bundle, err := o.store.InspectWorkspaceCheckpointPreparedV2(ctx, *input)
	if err != nil {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, err
	}
	if err := bundle.ValidateShape(); err != nil || bundle.Participant.TenantID != input.TenantID || bundle.Participant.ScopeDigest != input.ScopeDigest || bundle.Participant.CheckpointAttemptRef.ID != input.CheckpointAttemptID || bundle.Participant.ParticipantID != input.ParticipantID {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, fmt.Errorf("%w: workspace checkpoint prepared Inspect returned another identity", ports.ErrConflict)
	}
	return bundle.Clone(), nil
}

func (o *WorkspaceCheckpointParticipantOwnerV2) inspectByRequest(ctx context.Context, request contract.PrepareWorkspaceCheckpointParticipantRequestV2) (contract.WorkspaceCheckpointPreparedBundleV2, error) {
	query := contract.InspectWorkspaceCheckpointPreparedRequestV2{TenantID: request.TenantID, ScopeDigest: request.ScopeDigest, CheckpointAttemptID: request.CheckpointAttemptRef.ID, ParticipantID: request.ParticipantID}
	bundle, err := o.InspectWorkspaceCheckpointPreparedV2(ctx, &query)
	if err != nil {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, err
	}
	if !workspaceCheckpointBundleMatchesRequestV2(bundle, request) {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, fmt.Errorf("%w: workspace checkpoint identity already binds different content", ports.ErrConflict)
	}
	return bundle, nil
}

func (o *WorkspaceCheckpointParticipantOwnerV2) buildWorkspaceCheckpointPreparedV2(request contract.PrepareWorkspaceCheckpointParticipantRequestV2, current contract.WorkspaceCheckpointPreparationCurrentProjectionV2, now time.Time) (contract.WorkspaceCheckpointPreparedBundleV2, error) {
	expires := minimumWorkspaceCheckpointTimeV2(request.RequestedNotAfter, current.ExpiresUnixNano, request.SnapshotArtifactFactRef.ExpiresUnixNano, now.Add(o.maxTTL).UnixNano())
	if now.UnixNano() >= expires {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, fmt.Errorf("%w: workspace checkpoint TTL is exhausted", ports.ErrStale)
	}
	coverage, err := contract.SealWorkspaceCheckpointCoverageFactV2(contract.WorkspaceCheckpointCoverageFactV2{
		Meta:     contract.Meta{ContractVersion: contract.ContractFamily, ID: request.StableID + "-coverage", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires},
		TenantID: request.TenantID, ScopeDigest: request.ScopeDigest, RunID: request.RunID, CheckpointAttemptRef: request.CheckpointAttemptRef, BarrierRef: request.BarrierRef, EffectCutRef: request.EffectCutRef,
		ParticipantID: request.ParticipantID, ParticipantDigest: request.ParticipantDigest, PreparedPhaseFactRef: request.PreparedPhaseFactRef, SnapshotArtifactFactRef: request.SnapshotArtifactFactRef,
		SnapshotAggregateRef: current.SnapshotAggregateRef,
		CoveragePolicyRef:    request.CoveragePolicyRef, Included: append([]string(nil), current.Included...), DeclaredExcluded: append([]string(nil), current.DeclaredExcluded...), ResidualRefs: append([]contract.Ref(nil), current.ResidualRefs...), State: contract.WorkspaceCheckpointCoverageComplete, RequestedNotAfter: request.RequestedNotAfter,
	})
	if err != nil {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, err
	}
	participant, err := contract.SealWorkspaceCheckpointParticipantFactV2(contract.WorkspaceCheckpointParticipantFactV2{
		Meta:     contract.Meta{ContractVersion: contract.ContractFamily, ID: request.StableID + "-participant", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires},
		TenantID: request.TenantID, ScopeDigest: request.ScopeDigest, RunID: request.RunID, CheckpointAttemptRef: request.CheckpointAttemptRef, BarrierRef: request.BarrierRef, EffectCutRef: request.EffectCutRef,
		ParticipantID: request.ParticipantID, ParticipantDigest: request.ParticipantDigest, PreparedPhaseFactRef: request.PreparedPhaseFactRef, SnapshotArtifactFactRef: request.SnapshotArtifactFactRef,
		SnapshotAggregateRef: current.SnapshotAggregateRef,
		CoverageFactRef:      coverage.ExactRef(), State: contract.WorkspaceCheckpointParticipantPrepared, RequestedNotAfter: request.RequestedNotAfter,
	})
	if err != nil {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, err
	}
	bundle := contract.WorkspaceCheckpointPreparedBundleV2{Coverage: coverage, Participant: participant}
	return bundle, bundle.ValidateShape()
}

func validateWorkspaceCheckpointPreparationV2(projection contract.WorkspaceCheckpointPreparationCurrentProjectionV2, request contract.PrepareWorkspaceCheckpointParticipantRequestV2, now time.Time) error {
	if err := projection.ValidateCurrent(now); err != nil {
		return err
	}
	if !projection.MatchesRequest(request) {
		return fmt.Errorf("%w: workspace checkpoint preparation projection drifted", ports.ErrConflict)
	}
	return nil
}

func workspaceCheckpointBundleMatchesRequestV2(bundle contract.WorkspaceCheckpointPreparedBundleV2, request contract.PrepareWorkspaceCheckpointParticipantRequestV2) bool {
	p := bundle.Participant
	return p.Meta.ID == request.StableID+"-participant" && bundle.Coverage.Meta.ID == request.StableID+"-coverage" &&
		p.TenantID == request.TenantID && p.ScopeDigest == request.ScopeDigest && p.RunID == request.RunID &&
		contract.SameRef(p.CheckpointAttemptRef, request.CheckpointAttemptRef) && contract.SameRef(p.BarrierRef, request.BarrierRef) && contract.SameRef(p.EffectCutRef, request.EffectCutRef) &&
		p.ParticipantID == request.ParticipantID && p.ParticipantDigest == request.ParticipantDigest && contract.SameRef(p.PreparedPhaseFactRef, request.PreparedPhaseFactRef) &&
		contract.SameSnapshotArtifactExactRef(p.SnapshotArtifactFactRef, request.SnapshotArtifactFactRef) && contract.SameRef(bundle.Coverage.CoveragePolicyRef, request.CoveragePolicyRef) && p.RequestedNotAfter == request.RequestedNotAfter
}

func sameWorkspaceCheckpointPreparedV2(left, right contract.WorkspaceCheckpointPreparedBundleV2) bool {
	return contract.SameSnapshotArtifactExactRef(left.Coverage.ExactRef(), right.Coverage.ExactRef()) && contract.SameSnapshotArtifactExactRef(left.Participant.ExactRef(), right.Participant.ExactRef())
}

func minimumWorkspaceCheckpointTimeV2(values ...int64) int64 {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

func workspaceCheckpointNilV2(value any) bool {
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

var _ ports.WorkspaceCheckpointParticipantOwnerPortV2 = (*WorkspaceCheckpointParticipantOwnerV2)(nil)

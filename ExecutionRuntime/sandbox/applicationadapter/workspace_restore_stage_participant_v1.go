package applicationadapter

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/runtimeadapter"
)

// RestoreStageCoordinateStoreV1 is a Sandbox-owned exact handoff between the
// host coordinator and the Sandbox actual-point governance reader. A durable
// implementation must be create-once; changed content for one stable request
// is a conflict.
type RestoreStageCoordinateStoreV1 interface {
	runtimeadapter.RestoreStageCoordinateReaderV1
	PutRestoreStageCoordinatesV1(context.Context, contract.WorkspaceRestoreStageRequestV1, runtimeports.InspectRestoreStageGovernanceCurrentRequestV1) error
}

type MemoryRestoreStageCoordinateStoreV1 struct {
	mu     sync.Mutex
	values map[string]runtimeports.InspectRestoreStageGovernanceCurrentRequestV1
}

func NewMemoryRestoreStageCoordinateStoreV1() *MemoryRestoreStageCoordinateStoreV1 {
	return &MemoryRestoreStageCoordinateStoreV1{values: make(map[string]runtimeports.InspectRestoreStageGovernanceCurrentRequestV1)}
}

func (s *MemoryRestoreStageCoordinateStoreV1) PutRestoreStageCoordinatesV1(ctx context.Context, request contract.WorkspaceRestoreStageRequestV1, value runtimeports.InspectRestoreStageGovernanceCurrentRequestV1) error {
	if ctx == nil || request.ValidateShape() != nil || value.Validate() != nil {
		return errors.New("workspace Restore Stage coordinate write is invalid")
	}
	key, err := request.StableKeyDigest()
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.values[key]; ok && existing != value {
		return errors.New("workspace Restore Stage stable coordinate changed")
	}
	s.values[key] = value
	return nil
}

func (s *MemoryRestoreStageCoordinateStoreV1) ReadRestoreStageCoordinatesV1(ctx context.Context, request contract.WorkspaceRestoreStageRequestV1) (runtimeports.InspectRestoreStageGovernanceCurrentRequestV1, error) {
	if ctx == nil {
		return runtimeports.InspectRestoreStageGovernanceCurrentRequestV1{}, errors.New("workspace Restore Stage coordinate context is nil")
	}
	key, err := request.StableKeyDigest()
	if err != nil {
		return runtimeports.InspectRestoreStageGovernanceCurrentRequestV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[key]
	if !ok {
		return runtimeports.InspectRestoreStageGovernanceCurrentRequestV1{}, errors.New("workspace Restore Stage coordinate is absent")
	}
	return value, nil
}

type WorkspaceRestoreStageParticipantAdapterV1 struct {
	composition *WorkspaceRestoreProductionCompositionV1
	coordinates RestoreStageCoordinateStoreV1
	clock       func() time.Time
}

func NewWorkspaceRestoreStageParticipantAdapterV1(composition *WorkspaceRestoreProductionCompositionV1, coordinates RestoreStageCoordinateStoreV1, clock func() time.Time) (*WorkspaceRestoreStageParticipantAdapterV1, error) {
	if composition == nil || composition.Restore == nil || composition.PreparedCurrent == nil || composition.DomainResultCurrent == nil || composition.ApplySettlementCurrent == nil || nilLike(coordinates) || clock == nil {
		return nil, errors.New("workspace Restore production composition, coordinate store and clock are required")
	}
	return &WorkspaceRestoreStageParticipantAdapterV1{composition: composition, coordinates: coordinates, clock: clock}, nil
}

func (a *WorkspaceRestoreStageParticipantAdapterV1) PrepareRestoreStageV1(ctx context.Context, request applicationcontract.RestoreStageActionRequestV1, authorized applicationcontract.RestoreStageAuthorizedDispatchV1) (runtimeports.RestoreStageSandboxCurrentProjectionV1, error) {
	now := a.clock()
	workspace, err := workspaceRestoreRequestFromApplicationV1(request, authorized, now)
	if err != nil {
		return runtimeports.RestoreStageSandboxCurrentProjectionV1{}, err
	}
	prepared, err := a.composition.Restore.PrepareWorkspaceV1(ctx, &workspace)
	if err != nil {
		return runtimeports.RestoreStageSandboxCurrentProjectionV1{}, err
	}
	currentRequest, err := workspaceRestoreSandboxCurrentRequestV1(request, workspace, authorized, prepared)
	if err != nil {
		return runtimeports.RestoreStageSandboxCurrentProjectionV1{}, err
	}
	return a.composition.PreparedCurrent.BindWorkspaceRestorePreparedRuntimeV1(ctx, prepared.ExactRef(), currentRequest)
}

func (a *WorkspaceRestoreStageParticipantAdapterV1) ExecuteRestoreStageV1(ctx context.Context, request applicationcontract.RestoreStageActionRequestV1, authorized applicationcontract.RestoreStageAuthorizedDispatchV1, execute runtimeports.OperationDispatchEnforcementPhaseRefV4) (runtimeports.RestoreStageDomainResultCurrentProjectionV1, error) {
	now := a.clock()
	workspace, err := workspaceRestoreRequestFromApplicationV1(request, authorized, now)
	if err != nil {
		return runtimeports.RestoreStageDomainResultCurrentProjectionV1{}, err
	}
	coordinates, err := workspaceRestoreGovernanceCoordinatesV1(request, authorized, execute)
	if err != nil {
		return runtimeports.RestoreStageDomainResultCurrentProjectionV1{}, err
	}
	if err := a.coordinates.PutRestoreStageCoordinatesV1(ctx, workspace, coordinates); err != nil {
		return runtimeports.RestoreStageDomainResultCurrentProjectionV1{}, err
	}
	fact, err := a.composition.Restore.StageWorkspaceV1(ctx, &workspace)
	if err != nil {
		stageErr := err
		// Unknown provider outcomes are never retried with a new identity. The
		// Sandbox Owner's Reconcile method only Inspects the original attempt.
		fact, err = a.composition.Restore.ReconcileWorkspaceV1(context.WithoutCancel(ctx), &workspace)
		if err != nil {
			return runtimeports.RestoreStageDomainResultCurrentProjectionV1{}, errors.Join(stageErr, err)
		}
	}
	ref, err := a.composition.DomainResultCurrent.BindWorkspaceRestoreStageRuntimeV1(ctx, runtimeadapter.BindWorkspaceRestoreStageRuntimeV1Request{StageFactRef: fact.ExactRef(), Governance: coordinates})
	if err != nil {
		return runtimeports.RestoreStageDomainResultCurrentProjectionV1{}, err
	}
	return a.composition.DomainResultCurrent.InspectRestoreStageDomainResultCurrentV1(ctx, ref)
}

func (a *WorkspaceRestoreStageParticipantAdapterV1) ApplyRestoreStageSettlementV1(ctx context.Context, settlement runtimeports.RestoreStageSettlementRefV1, stage runtimeports.RestoreStageDomainResultCurrentProjectionV1) (runtimeports.RestoreStageApplySettlementCurrentProjectionV1, error) {
	if stage.Validate(a.clock()) != nil || !runtimeports.SameRestoreStageDomainResultFactRefV1(settlement.DomainResult, stage.Fact) {
		return runtimeports.RestoreStageApplySettlementCurrentProjectionV1{}, errors.New("workspace Restore ApplySettlement exact closure is invalid")
	}
	factRef, err := a.composition.DomainResultCurrent.InspectWorkspaceRestoreStageFactRefV1(ctx, stage.Fact)
	if err != nil {
		return runtimeports.RestoreStageApplySettlementCurrentProjectionV1{}, err
	}
	return a.composition.ApplySettlementCurrent.ApplyWorkspaceRestoreStageSettlementCurrentV1(ctx, settlement, factRef)
}

func workspaceRestoreRequestFromApplicationV1(request applicationcontract.RestoreStageActionRequestV1, authorized applicationcontract.RestoreStageAuthorizedDispatchV1, now time.Time) (contract.WorkspaceRestoreStageRequestV1, error) {
	if err := authorized.ValidateFor(request, now); err != nil {
		return contract.WorkspaceRestoreStageRequestV1{}, err
	}
	legacy := authorized.Dispatch.Record.Permit.LegacyPermit
	expires := minimumWorkspaceRestoreAdapterTimeV1(request.NotAfterUnixNano, authorized.ExpiresUnixNano, request.Eligibility.ExpiresUnixNano, request.Materialization.ExpiresUnixNano)
	exact := func(typeURL, domain, id string, version uint32, revision uint64, digest string, exactExpires int64) contract.SnapshotArtifactExactRefV2 {
		return contract.SnapshotArtifactExactRefV2{TypeURL: typeURL, Version: version, ID: id, Revision: revision, DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: domain, Digest: strings.TrimPrefix(digest, "sha256:"), ExpiresUnixNano: exactExpires}
	}
	identity := request.Materialization.Identity
	value := contract.WorkspaceRestoreStageRequestV1{
		TenantID: string(request.Attempt.TenantID), DispatchAttemptID: legacy.AttemptID,
		RuntimeRestoreAttempt:   exact("praxis.runtime/restore-attempt/v2", "praxis.runtime/restore-attempt/body/v2", request.Attempt.ID, 2, uint64(request.Attempt.Revision), string(request.Attempt.Digest), expires),
		RestoreEligibility:      exact("praxis.runtime/restore-eligibility/v2", "praxis.runtime/restore-eligibility/body/v2", request.Eligibility.ID, 2, uint64(request.Eligibility.Revision), string(request.Eligibility.Digest), request.Eligibility.ExpiresUnixNano),
		Target:                  contract.RuntimeLeaseBinding{TenantID: string(request.Attempt.TenantID), InstanceID: string(identity.TargetInstance.ID), InstanceEpoch: uint64(identity.TargetInstance.Epoch), LeaseID: string(identity.TargetLease.ID), LeaseEpoch: uint64(identity.TargetLease.Epoch), FenceEpoch: uint64(identity.TargetFenceEpoch), ScopeDigest: string(legacy.Operation.ExecutionScopeDigest), ObservedRevision: uint64(legacy.Operation.CurrentProjectionRevision), ExpiresUnixNano: expires},
		SnapshotArtifactFactRef: exact(contract.SnapshotArtifactFactTypeURL, contract.SnapshotArtifactFactDomain, authorized.SnapshotArtifact.ID, 2, uint64(authorized.SnapshotArtifact.Revision), authorized.SnapshotArtifact.Digest, expires), RequestedNotAfter: expires,
	}
	return value, value.ValidateCurrent(now)
}

func workspaceRestoreSandboxCurrentRequestV1(request applicationcontract.RestoreStageActionRequestV1, workspace contract.WorkspaceRestoreStageRequestV1, authorized applicationcontract.RestoreStageAuthorizedDispatchV1, prepared contract.WorkspaceRestoreAttemptV1) (runtimeports.InspectRestoreStageSandboxCurrentRequestV1, error) {
	legacy := authorized.Dispatch.Record.Permit.LegacyPermit
	opDigest, err := legacy.Operation.DigestV3()
	if err != nil {
		return runtimeports.InspectRestoreStageSandboxCurrentRequestV1{}, err
	}
	dispatch := runtimeports.OperationDispatchAttemptRefV3{OperationDigest: opDigest, EffectID: legacy.IntentID, IntentRevision: legacy.IntentRevision, IntentDigest: legacy.IntentDigest, PermitID: legacy.ID, PermitRevision: authorized.Dispatch.Record.Revision, PermitDigest: authorized.Dispatch.Record.PermitDigest, AttemptID: legacy.AttemptID}
	sandboxAttempt := runtimeports.OperationDispatchSandboxFactRefV4{ID: prepared.Meta.ID, Revision: runtimecore.Revision(prepared.Meta.Revision), Digest: runtimecore.Digest("sha256:" + strings.TrimPrefix(prepared.Meta.Digest, "sha256:")), ExpiresUnixNano: prepared.Meta.ExpiresUnixNano}
	value := runtimeports.InspectRestoreStageSandboxCurrentRequestV1{Operation: legacy.Operation, EffectID: legacy.IntentID, IntentRevision: legacy.IntentRevision, IntentDigest: legacy.IntentDigest, DispatchAttempt: dispatch, SandboxAttempt: sandboxAttempt, RestoreAttempt: request.Attempt, Eligibility: request.Eligibility, Identity: request.Materialization.Identity, SnapshotArtifact: authorized.SnapshotArtifact, Provider: legacy.EnforcementPoint}
	return value, value.Validate()
}

func workspaceRestoreGovernanceCoordinatesV1(request applicationcontract.RestoreStageActionRequestV1, authorized applicationcontract.RestoreStageAuthorizedDispatchV1, execute runtimeports.OperationDispatchEnforcementPhaseRefV4) (runtimeports.InspectRestoreStageGovernanceCurrentRequestV1, error) {
	legacy := authorized.Dispatch.Record.Permit.LegacyPermit
	opDigest, err := legacy.Operation.DigestV3()
	if err != nil {
		return runtimeports.InspectRestoreStageGovernanceCurrentRequestV1{}, err
	}
	dispatch := runtimeports.OperationDispatchAttemptRefV3{OperationDigest: opDigest, EffectID: legacy.IntentID, IntentRevision: legacy.IntentRevision, IntentDigest: legacy.IntentDigest, PermitID: legacy.ID, PermitRevision: authorized.Dispatch.Record.Revision, PermitDigest: authorized.Dispatch.Record.PermitDigest, AttemptID: legacy.AttemptID}
	value := runtimeports.InspectRestoreStageGovernanceCurrentRequestV1{RestoreAttempt: request.Attempt, Eligibility: request.Eligibility, Operation: legacy.Operation, EffectID: legacy.IntentID, Admission: authorized.Dispatch.Record.Permit.Admission.Admission, Authorization: authorized.Dispatch.ReviewAuthorization, PermitID: legacy.ID, DispatchAttempt: dispatch, ExecuteEnforcement: execute, SnapshotArtifact: authorized.SnapshotArtifact}
	return value, value.Validate()
}

func minimumWorkspaceRestoreAdapterTimeV1(values ...int64) int64 {
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

var _ applicationports.RestoreStageParticipantPortV1 = (*WorkspaceRestoreStageParticipantAdapterV1)(nil)
var _ RestoreStageCoordinateStoreV1 = (*MemoryRestoreStageCoordinateStoreV1)(nil)

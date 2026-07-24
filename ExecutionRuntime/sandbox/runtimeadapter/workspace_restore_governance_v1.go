package runtimeadapter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

const (
	restoreAttemptDigestDomainV2     = "praxis.runtime/restore-attempt/body/v2"
	restoreEligibilityDigestDomainV2 = "praxis.runtime/restore-eligibility/body/v2"
	restoreAdmissionTypeURLV1        = "praxis.runtime/operation-effect-admission/v3"
	restoreAdmissionDigestDomainV1   = "praxis.runtime/operation-effect-admission/body/v3"
	restoreReviewTypeURLV1           = "praxis.runtime/operation-review-authorization/v4"
	restoreReviewDigestDomainV1      = "praxis.runtime/operation-review-authorization/body/v4"
	restorePermitTypeURLV1           = "praxis.runtime/operation-dispatch-permit/v3"
	restorePermitDigestDomainV1      = "praxis.runtime/operation-dispatch-permit/body/v3"
	restoreBeginTypeURLV1            = "praxis.runtime/operation-dispatch-begin/v3"
	restoreBeginDigestDomainV1       = "praxis.runtime/operation-dispatch-begin/body/v3"
	restoreEnforcementTypeURLV1      = "praxis.runtime/operation-dispatch-enforcement/v4"
	restoreEnforcementDigestDomainV1 = "praxis.runtime/operation-dispatch-enforcement/body/v4"
)

// RestoreStageCoordinateReaderV1 resolves only exact Runtime request
// coordinates. It grants no execution authority; the Runtime current Gateway
// independently validates all coordinates and returns the authoritative join.
type RestoreStageCoordinateReaderV1 interface {
	ReadRestoreStageCoordinatesV1(context.Context, contract.WorkspaceRestoreStageRequestV1) (runtimeports.InspectRestoreStageGovernanceCurrentRequestV1, error)
}

type WorkspaceRestoreGovernanceReaderV1 struct {
	coordinates RestoreStageCoordinateReaderV1
	current     runtimeports.RestoreStageGovernanceCurrentPortV1
	clock       func() time.Time
}

func NewWorkspaceRestoreGovernanceReaderV1(coordinates RestoreStageCoordinateReaderV1, current runtimeports.RestoreStageGovernanceCurrentPortV1, clock func() time.Time) (*WorkspaceRestoreGovernanceReaderV1, error) {
	if nilLikeV4(coordinates) || nilLikeV4(current) || clock == nil {
		return nil, errors.New("workspace restore Runtime coordinates, current Gateway, and clock are required")
	}
	return &WorkspaceRestoreGovernanceReaderV1{coordinates: coordinates, current: current, clock: clock}, nil
}

func (r *WorkspaceRestoreGovernanceReaderV1) InspectWorkspaceRestoreGovernanceCurrentV1(ctx context.Context, request contract.WorkspaceRestoreStageRequestV1) (contract.WorkspaceRestoreGovernanceCurrentProjectionV1, error) {
	now := r.clock()
	if err := request.ValidateCurrent(now); err != nil {
		return contract.WorkspaceRestoreGovernanceCurrentProjectionV1{}, err
	}
	coordinates, err := r.coordinates.ReadRestoreStageCoordinatesV1(ctx, request)
	if err != nil {
		return contract.WorkspaceRestoreGovernanceCurrentProjectionV1{}, err
	}
	if err := coordinates.Validate(); err != nil {
		return contract.WorkspaceRestoreGovernanceCurrentProjectionV1{}, err
	}
	projection, err := r.current.InspectRestoreStageGovernanceCurrentV1(ctx, coordinates)
	if err != nil {
		return contract.WorkspaceRestoreGovernanceCurrentProjectionV1{}, err
	}
	fresh := r.clock()
	if err := projection.Validate(fresh); err != nil {
		return contract.WorkspaceRestoreGovernanceCurrentProjectionV1{}, err
	}
	if err := validateRestoreStageRuntimeCoordinatesV1(request, coordinates, projection); err != nil {
		return contract.WorkspaceRestoreGovernanceCurrentProjectionV1{}, err
	}
	return mapRestoreStageProjectionV1(request, projection, fresh)
}

func validateRestoreStageRuntimeCoordinatesV1(request contract.WorkspaceRestoreStageRequestV1, coordinates runtimeports.InspectRestoreStageGovernanceCurrentRequestV1, projection runtimeports.RestoreStageGovernanceCurrentProjectionV1) error {
	if coordinates.RestoreAttempt != projection.RestoreAttempt || coordinates.Eligibility != projection.Eligibility || coordinates.Operation != projection.Operation || coordinates.EffectID != projection.EffectID || coordinates.Admission != projection.Admission || coordinates.Authorization != projection.Authorization || coordinates.PermitID != projection.PermitID || coordinates.DispatchAttempt != projection.DispatchAttempt || coordinates.ExecuteEnforcement != projection.ExecuteEnforcement || coordinates.SnapshotArtifact != projection.SnapshotArtifact {
		return fmt.Errorf("%w: Restore Stage Runtime current differs from requested exact coordinates", ports.ErrConflict)
	}
	if string(projection.RestoreAttempt.TenantID) != request.TenantID || projection.RestoreAttempt.ID != request.RuntimeRestoreAttempt.ID || uint64(projection.RestoreAttempt.Revision) != request.RuntimeRestoreAttempt.Revision || sameDigestV1(string(projection.RestoreAttempt.Digest), request.RuntimeRestoreAttempt.Digest) == false {
		return fmt.Errorf("%w: Runtime Restore Attempt crosses Sandbox request", ports.ErrConflict)
	}
	if string(projection.Eligibility.TenantID) != request.TenantID || projection.Eligibility.ID != request.RestoreEligibility.ID || uint64(projection.Eligibility.Revision) != request.RestoreEligibility.Revision || sameDigestV1(string(projection.Eligibility.Digest), request.RestoreEligibility.Digest) == false || projection.Eligibility.ExpiresUnixNano != request.RestoreEligibility.ExpiresUnixNano {
		return fmt.Errorf("%w: Runtime Restore Eligibility crosses Sandbox request", ports.ErrConflict)
	}
	if projection.SnapshotArtifact.Owner.ComponentID != "praxis/sandbox" || projection.SnapshotArtifact.Owner.FactKind != "snapshot-artifact-fact" || projection.SnapshotArtifact.TenantID != request.TenantID || projection.SnapshotArtifact.ID != request.SnapshotArtifactFactRef.ID || uint64(projection.SnapshotArtifact.Revision) != request.SnapshotArtifactFactRef.Revision || sameDigestV1(projection.SnapshotArtifact.Digest, request.SnapshotArtifactFactRef.Digest) == false {
		return fmt.Errorf("%w: Runtime Snapshot Artifact binding crosses Sandbox request", ports.ErrConflict)
	}
	target := projection.Identity
	if request.Target.InstanceID != string(target.TargetInstance.ID) || request.Target.InstanceEpoch != uint64(target.TargetInstance.Epoch) || request.Target.LeaseID != string(target.TargetLease.ID) || request.Target.LeaseEpoch != uint64(target.TargetLease.Epoch) || request.Target.FenceEpoch != uint64(target.TargetFenceEpoch) || request.Target.ScopeDigest != string(projection.Operation.ExecutionScopeDigest) || request.Target.ObservedRevision != uint64(projection.Operation.CurrentProjectionRevision) || request.Target.ExpiresUnixNano > projection.ExpiresUnixNano {
		return fmt.Errorf("%w: Runtime fresh Instance/Lease/Fence binding crosses Sandbox target", ports.ErrConflict)
	}
	if request.RuntimeRestoreAttempt.ExpiresUnixNano > projection.ExpiresUnixNano || request.SnapshotArtifactFactRef.ExpiresUnixNano > projection.ExpiresUnixNano {
		return fmt.Errorf("%w: Sandbox request extends Runtime Restore current TTL", ports.ErrConflict)
	}
	return nil
}

func mapRestoreStageProjectionV1(request contract.WorkspaceRestoreStageRequestV1, source runtimeports.RestoreStageGovernanceCurrentProjectionV1, now time.Time) (contract.WorkspaceRestoreGovernanceCurrentProjectionV1, error) {
	expires := minimumRestoreStageTTLRuntimeAdapterV1(source.ExpiresUnixNano, request.RequestedNotAfter, request.RuntimeRestoreAttempt.ExpiresUnixNano, request.RestoreEligibility.ExpiresUnixNano, request.SnapshotArtifactFactRef.ExpiresUnixNano, request.Target.ExpiresUnixNano, source.ExecuteEnforcement.ExpiresUnixNano)
	if now.IsZero() || now.UnixNano() >= expires {
		return contract.WorkspaceRestoreGovernanceCurrentProjectionV1{}, fmt.Errorf("%w: Restore Stage mapped current TTL is exhausted", ports.ErrStale)
	}
	ref := func(typeURL, domain, id string, revision uint64, digest string, expiry int64) contract.SnapshotArtifactExactRefV2 {
		return contract.SnapshotArtifactExactRefV2{TypeURL: typeURL, Version: 1, ID: id, Revision: revision, DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: domain, Digest: strings.TrimPrefix(digest, "sha256:"), ExpiresUnixNano: expiry}
	}
	value := contract.WorkspaceRestoreGovernanceCurrentProjectionV1{
		TenantID:               request.TenantID,
		RuntimeRestoreAttempt:  request.RuntimeRestoreAttempt,
		RestoreEligibility:     request.RestoreEligibility,
		Target:                 request.Target,
		ActionAdmissionRef:     ref(restoreAdmissionTypeURLV1, restoreAdmissionDigestDomainV1, string(source.EffectID), uint64(source.Admission.FactRevision), string(source.DispatchAdmissionDigest), expires),
		ReviewAuthorizationRef: ref(restoreReviewTypeURLV1, restoreReviewDigestDomainV1, source.Authorization.ID, uint64(source.Authorization.Revision), string(source.Authorization.Digest), expires),
		DispatchPermitRef:      ref(restorePermitTypeURLV1, restorePermitDigestDomainV1, source.PermitID, uint64(source.PermitFactRevision), string(source.PermitDigest), expires),
		BeginRef:               ref(restoreBeginTypeURLV1, restoreBeginDigestDomainV1, source.PermitID, uint64(source.BeginRecordRevision), string(source.BeginRecordDigest), expires),
		EnforcementRef:         ref(restoreEnforcementTypeURLV1, restoreEnforcementDigestDomainV1, source.ExecuteEnforcement.AttemptID, uint64(source.ExecuteEnforcement.JournalRevision), string(source.ExecuteEnforcement.ReceiptDigest), minimumRestoreStageTTLRuntimeAdapterV1(expires, source.ExecuteEnforcement.ExpiresUnixNano)),
		CheckedUnixNano:        source.CheckedUnixNano,
		ExpiresUnixNano:        expires,
	}
	return contract.SealWorkspaceRestoreGovernanceCurrentProjectionV1(value)
}

func sameDigestV1(left, right string) bool {
	return strings.TrimPrefix(left, "sha256:") == strings.TrimPrefix(right, "sha256:")
}

func minimumRestoreStageTTLRuntimeAdapterV1(values ...int64) int64 {
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

var _ ports.WorkspaceRestoreGovernanceCurrentReaderV1 = (*WorkspaceRestoreGovernanceReaderV1)(nil)

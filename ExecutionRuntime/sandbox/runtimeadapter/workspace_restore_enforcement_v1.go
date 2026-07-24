package runtimeadapter

import (
	"context"
	"errors"
	"fmt"
	"time"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type WorkspaceRestorePreparedAttemptReaderV1 interface {
	InspectWorkspaceRestoreAttemptV1(context.Context, contract.SnapshotArtifactExactRefV2) (contract.WorkspaceRestoreAttemptV1, error)
}

type WorkspaceRestorePreparedRuntimeBindingV1 struct {
	TenantID string                                                  `json:"tenant_id"`
	Attempt  contract.SnapshotArtifactExactRefV2                     `json:"sandbox_attempt"`
	Runtime  runtimeports.InspectRestoreStageSandboxCurrentRequestV1 `json:"runtime_request"`
}

func (v WorkspaceRestorePreparedRuntimeBindingV1) Validate() error {
	if v.TenantID == "" || v.Attempt.ValidateShape("workspace restore prepared attempt") != nil || v.Attempt.TypeURL != contract.WorkspaceRestoreAttemptTypeURLV1 || v.Runtime.Validate() != nil || string(v.Runtime.RestoreAttempt.TenantID) != v.TenantID || v.Runtime.SandboxAttempt.ID != v.Attempt.ID || uint64(v.Runtime.SandboxAttempt.Revision) != v.Attempt.Revision || !sameDigestV1(string(v.Runtime.SandboxAttempt.Digest), v.Attempt.Digest) || v.Runtime.SandboxAttempt.ExpiresUnixNano != v.Attempt.ExpiresUnixNano {
		return errors.New("workspace restore prepared Runtime binding is incomplete or crosses Attempt")
	}
	return nil
}

type WorkspaceRestorePreparedRuntimeBindingStoreV1 interface {
	CreateWorkspaceRestorePreparedRuntimeBindingV1(context.Context, WorkspaceRestorePreparedRuntimeBindingV1) (WorkspaceRestorePreparedRuntimeBindingV1, error)
	InspectWorkspaceRestorePreparedRuntimeBindingV1(context.Context, string, string) (WorkspaceRestorePreparedRuntimeBindingV1, error)
}

type WorkspaceRestorePreparedCurrentAdapterV1 struct {
	attempts WorkspaceRestorePreparedAttemptReaderV1
	bindings WorkspaceRestorePreparedRuntimeBindingStoreV1
	clock    func() time.Time
}

func NewWorkspaceRestorePreparedCurrentAdapterV1(attempts WorkspaceRestorePreparedAttemptReaderV1, bindings WorkspaceRestorePreparedRuntimeBindingStoreV1, clock func() time.Time) (*WorkspaceRestorePreparedCurrentAdapterV1, error) {
	if nilLikeV4(attempts) || nilLikeV4(bindings) || nilLikeV4(clock) {
		return nil, errors.New("workspace restore prepared Attempt Reader, binding Owner, and clock are required")
	}
	return &WorkspaceRestorePreparedCurrentAdapterV1{attempts: attempts, bindings: bindings, clock: clock}, nil
}

func (a *WorkspaceRestorePreparedCurrentAdapterV1) BindWorkspaceRestorePreparedRuntimeV1(ctx context.Context, attemptRef contract.SnapshotArtifactExactRefV2, request runtimeports.InspectRestoreStageSandboxCurrentRequestV1) (runtimeports.RestoreStageSandboxCurrentProjectionV1, error) {
	if a == nil || nilLikeV4(ctx) {
		return runtimeports.RestoreStageSandboxCurrentProjectionV1{}, errors.New("workspace restore prepared adapter or context is nil")
	}
	now := a.clock()
	projection, err := a.derive(ctx, attemptRef, request, now)
	if err != nil {
		return runtimeports.RestoreStageSandboxCurrentProjectionV1{}, err
	}
	binding := WorkspaceRestorePreparedRuntimeBindingV1{TenantID: string(request.RestoreAttempt.TenantID), Attempt: attemptRef, Runtime: request}
	stored, createErr := a.bindings.CreateWorkspaceRestorePreparedRuntimeBindingV1(ctx, binding)
	if createErr != nil {
		stored, err = a.bindings.InspectWorkspaceRestorePreparedRuntimeBindingV1(context.WithoutCancel(ctx), binding.TenantID, attemptRef.ID)
		if err != nil {
			return runtimeports.RestoreStageSandboxCurrentProjectionV1{}, createErr
		}
	}
	if stored != binding {
		return runtimeports.RestoreStageSandboxCurrentProjectionV1{}, fmt.Errorf("%w: workspace restore prepared Runtime binding winner differs", ports.ErrConflict)
	}
	return projection, nil
}

func (a *WorkspaceRestorePreparedCurrentAdapterV1) InspectRestoreStageSandboxCurrentV1(ctx context.Context, request runtimeports.InspectRestoreStageSandboxCurrentRequestV1) (runtimeports.RestoreStageSandboxCurrentProjectionV1, error) {
	if a == nil || nilLikeV4(ctx) || request.Validate() != nil {
		return runtimeports.RestoreStageSandboxCurrentProjectionV1{}, errors.New("workspace restore prepared current request is invalid")
	}
	binding, err := a.bindings.InspectWorkspaceRestorePreparedRuntimeBindingV1(ctx, string(request.RestoreAttempt.TenantID), request.SandboxAttempt.ID)
	if err != nil {
		return runtimeports.RestoreStageSandboxCurrentProjectionV1{}, err
	}
	if binding.Validate() != nil || binding.Runtime != request {
		return runtimeports.RestoreStageSandboxCurrentProjectionV1{}, fmt.Errorf("%w: workspace restore prepared current binding drifted", ports.ErrConflict)
	}
	return a.derive(ctx, binding.Attempt, request, a.clock())
}

func (a *WorkspaceRestorePreparedCurrentAdapterV1) derive(ctx context.Context, attemptRef contract.SnapshotArtifactExactRefV2, request runtimeports.InspectRestoreStageSandboxCurrentRequestV1, now time.Time) (runtimeports.RestoreStageSandboxCurrentProjectionV1, error) {
	if now.IsZero() || request.Validate() != nil || attemptRef.ValidateCurrent("workspace restore prepared attempt", now) != nil {
		return runtimeports.RestoreStageSandboxCurrentProjectionV1{}, errors.New("workspace restore prepared derive coordinates are incomplete or stale")
	}
	attempt, err := a.attempts.InspectWorkspaceRestoreAttemptV1(ctx, attemptRef)
	if err != nil {
		return runtimeports.RestoreStageSandboxCurrentProjectionV1{}, err
	}
	if attempt.ValidateShape() != nil || attempt.ExactRef() != attemptRef || attempt.State != contract.WorkspaceRestoreAttemptPreparedV1 || attempt.Request.DispatchAttemptID != request.DispatchAttempt.AttemptID || attempt.Request.RuntimeRestoreAttempt.ID != request.RestoreAttempt.ID || attempt.Request.RuntimeRestoreAttempt.Revision != uint64(request.RestoreAttempt.Revision) || !sameDigestV1(attempt.Request.RuntimeRestoreAttempt.Digest, string(request.RestoreAttempt.Digest)) || attempt.Request.RestoreEligibility.ID != request.Eligibility.ID || attempt.Request.RestoreEligibility.Revision != uint64(request.Eligibility.Revision) || !sameDigestV1(attempt.Request.RestoreEligibility.Digest, string(request.Eligibility.Digest)) || attempt.Request.SnapshotArtifactFactRef.ID != request.SnapshotArtifact.ID || attempt.Request.SnapshotArtifactFactRef.Revision != uint64(request.SnapshotArtifact.Revision) || !sameDigestV1(attempt.Request.SnapshotArtifactFactRef.Digest, request.SnapshotArtifact.Digest) {
		return runtimeports.RestoreStageSandboxCurrentProjectionV1{}, fmt.Errorf("%w: workspace restore prepared Attempt crosses Runtime request", ports.ErrConflict)
	}
	if attempt.Request.Target.InstanceID != string(request.Operation.ExecutionScope.Instance.ID) || attempt.Request.Target.InstanceEpoch != uint64(request.Operation.ExecutionScope.Instance.Epoch) || attempt.Request.Target.LeaseID != string(request.Operation.ExecutionScope.SandboxLease.ID) || attempt.Request.Target.LeaseEpoch != uint64(request.Operation.ExecutionScope.SandboxLease.Epoch) || attempt.Request.Target.FenceEpoch != uint64(request.Operation.ExecutionScope.AuthorityEpoch) || attempt.Request.Target.ScopeDigest != string(request.Operation.ExecutionScopeDigest) {
		return runtimeports.RestoreStageSandboxCurrentProjectionV1{}, fmt.Errorf("%w: workspace restore prepared target crosses Runtime scope", ports.ErrConflict)
	}
	prepared, err := runtimeports.SealRestoreStagePreparedAttemptRefV1(runtimeports.RestoreStagePreparedAttemptRefV1{SandboxAttempt: request.SandboxAttempt, OperationDigest: request.DispatchAttempt.OperationDigest, EffectID: request.EffectID, IntentRevision: request.IntentRevision, IntentDigest: request.IntentDigest, DispatchAttempt: request.DispatchAttempt, Provider: request.Provider, BundleDigest: runtimeDigest(attempt.BundleDigest), PreparedUnixNano: attempt.Meta.UpdatedUnixNano, ExpiresUnixNano: attempt.Meta.ExpiresUnixNano})
	if err != nil {
		return runtimeports.RestoreStageSandboxCurrentProjectionV1{}, err
	}
	projection := runtimeports.RestoreStageSandboxCurrentProjectionV1{Operation: request.Operation, OperationDigest: request.DispatchAttempt.OperationDigest, EffectID: request.EffectID, IntentRevision: request.IntentRevision, IntentDigest: request.IntentDigest, DispatchAttempt: request.DispatchAttempt, SandboxAttempt: request.SandboxAttempt, RestoreAttempt: request.RestoreAttempt, Eligibility: request.Eligibility, Identity: request.Identity, SnapshotArtifact: request.SnapshotArtifact, BundleProjectionDigest: runtimeDigest(attempt.BundleProjectionDigest), BundleDigest: runtimeDigest(attempt.BundleDigest), Provider: request.Provider, Prepared: prepared, Current: true, CheckedUnixNano: attempt.Meta.UpdatedUnixNano, ExpiresUnixNano: attempt.Meta.ExpiresUnixNano}
	return runtimeports.SealRestoreStageSandboxCurrentProjectionV1(projection, now)
}

var _ runtimeports.RestoreStageSandboxCurrentReaderV1 = (*WorkspaceRestorePreparedCurrentAdapterV1)(nil)

package runtimeadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type WorkspaceRestoreStageFactReaderV1 interface {
	InspectWorkspaceRestoreAttemptByStableKeyV1(context.Context, string) (contract.WorkspaceRestoreAttemptV1, error)
	InspectWorkspaceRestoreStageFactV1(context.Context, contract.SnapshotArtifactExactRefV2) (contract.WorkspaceRestoreStageFactV1, error)
}

type WorkspaceRestoreStageRuntimeBindingV1 struct {
	TenantID   string                                                     `json:"tenant_id"`
	FactRef    contract.SnapshotArtifactExactRefV2                        `json:"sandbox_fact_ref"`
	Governance runtimeports.InspectRestoreStageGovernanceCurrentRequestV1 `json:"governance_request"`
	Runtime    runtimeports.RestoreStageDomainResultFactRefV1             `json:"runtime_ref"`
}

func (v WorkspaceRestoreStageRuntimeBindingV1) Validate() error {
	if strings.TrimSpace(v.TenantID) == "" || v.FactRef.ValidateShape("workspace restore stage fact") != nil || v.FactRef.TypeURL != contract.WorkspaceRestoreFactTypeURLV1 || v.Governance.Validate() != nil || v.Runtime.Validate() != nil || string(v.Runtime.TenantID) != v.TenantID || v.Runtime.ID != v.FactRef.ID || uint64(v.Runtime.Revision) != v.FactRef.Revision || !sameDigestV1(string(v.Runtime.Digest), v.FactRef.Digest) || !runtimeports.SameOperationSubjectV3(v.Governance.Operation, v.Runtime.Operation) || v.Governance.EffectID != v.Runtime.EffectID || v.Governance.DispatchAttempt != v.Runtime.Attempt || v.Governance.RestoreAttempt != v.Runtime.RestoreAttempt || v.Governance.Eligibility != v.Runtime.Eligibility {
		return errors.New("workspace restore Stage Runtime binding is incomplete or crosses facts")
	}
	return nil
}

type WorkspaceRestoreStageRuntimeBindingStoreV1 interface {
	CreateWorkspaceRestoreStageRuntimeBindingV1(context.Context, WorkspaceRestoreStageRuntimeBindingV1) (WorkspaceRestoreStageRuntimeBindingV1, error)
	InspectWorkspaceRestoreStageRuntimeBindingV1(context.Context, string, string) (WorkspaceRestoreStageRuntimeBindingV1, error)
}

type BindWorkspaceRestoreStageRuntimeV1Request struct {
	StageFactRef contract.SnapshotArtifactExactRefV2                        `json:"stage_fact_ref"`
	Governance   runtimeports.InspectRestoreStageGovernanceCurrentRequestV1 `json:"governance"`
}

type WorkspaceRestoreStageDomainResultAdapterV1 struct {
	facts      WorkspaceRestoreStageFactReaderV1
	bindings   WorkspaceRestoreStageRuntimeBindingStoreV1
	governance runtimeports.RestoreStageGovernanceCurrentPortV1
	owner      runtimeports.ProviderBindingRefV2
	schema     runtimeports.SchemaRefV2
	clock      func() time.Time
}

func NewWorkspaceRestoreStageDomainResultAdapterV1(facts WorkspaceRestoreStageFactReaderV1, bindings WorkspaceRestoreStageRuntimeBindingStoreV1, governance runtimeports.RestoreStageGovernanceCurrentPortV1, owner runtimeports.ProviderBindingRefV2, schema runtimeports.SchemaRefV2, clock func() time.Time) (*WorkspaceRestoreStageDomainResultAdapterV1, error) {
	if nilLikeV4(facts) || nilLikeV4(bindings) || nilLikeV4(governance) || nilLikeV4(clock) || owner.Validate() != nil || schema.Validate() != nil {
		return nil, errors.New("workspace restore Stage facts, binding Owner, Runtime current Reader, owner, schema, and clock are required")
	}
	return &WorkspaceRestoreStageDomainResultAdapterV1{facts: facts, bindings: bindings, governance: governance, owner: owner, schema: schema, clock: clock}, nil
}

func (a *WorkspaceRestoreStageDomainResultAdapterV1) BindWorkspaceRestoreStageRuntimeV1(ctx context.Context, request BindWorkspaceRestoreStageRuntimeV1Request) (runtimeports.RestoreStageDomainResultFactRefV1, error) {
	if a == nil || nilLikeV4(ctx) {
		return runtimeports.RestoreStageDomainResultFactRefV1{}, errors.New("workspace restore Stage adapter or context is nil")
	}
	now := a.clock()
	ref, factRef, _, err := a.derive(ctx, request.StageFactRef, request.Governance, now)
	if err != nil {
		return runtimeports.RestoreStageDomainResultFactRefV1{}, err
	}
	binding := WorkspaceRestoreStageRuntimeBindingV1{TenantID: string(ref.TenantID), FactRef: factRef, Governance: request.Governance, Runtime: ref}
	stored, createErr := a.bindings.CreateWorkspaceRestoreStageRuntimeBindingV1(ctx, binding)
	if createErr != nil {
		stored, err = a.bindings.InspectWorkspaceRestoreStageRuntimeBindingV1(context.WithoutCancel(ctx), binding.TenantID, ref.ID)
		if err != nil {
			return runtimeports.RestoreStageDomainResultFactRefV1{}, createErr
		}
	}
	if stored != binding {
		return runtimeports.RestoreStageDomainResultFactRefV1{}, fmt.Errorf("%w: workspace restore Stage binding Owner returned different content", ports.ErrConflict)
	}
	return stored.Runtime, nil
}

func (a *WorkspaceRestoreStageDomainResultAdapterV1) InspectRestoreStageDomainResultCurrentV1(ctx context.Context, expected runtimeports.RestoreStageDomainResultFactRefV1) (runtimeports.RestoreStageDomainResultCurrentProjectionV1, error) {
	if a == nil || nilLikeV4(ctx) {
		return runtimeports.RestoreStageDomainResultCurrentProjectionV1{}, errors.New("workspace restore Stage adapter or context is nil")
	}
	if err := expected.Validate(); err != nil {
		return runtimeports.RestoreStageDomainResultCurrentProjectionV1{}, err
	}
	binding, err := a.bindings.InspectWorkspaceRestoreStageRuntimeBindingV1(ctx, string(expected.TenantID), expected.ID)
	if err != nil {
		return runtimeports.RestoreStageDomainResultCurrentProjectionV1{}, err
	}
	if binding.Validate() != nil || !runtimeports.SameRestoreStageDomainResultFactRefV1(binding.Runtime, expected) {
		return runtimeports.RestoreStageDomainResultCurrentProjectionV1{}, fmt.Errorf("%w: workspace restore Stage current binding drifted", ports.ErrConflict)
	}
	now := a.clock()
	derived, _, expires, err := a.derive(ctx, binding.FactRef, binding.Governance, now)
	if err != nil {
		return runtimeports.RestoreStageDomainResultCurrentProjectionV1{}, err
	}
	if !runtimeports.SameRestoreStageDomainResultFactRefV1(derived, expected) {
		return runtimeports.RestoreStageDomainResultCurrentProjectionV1{}, fmt.Errorf("%w: workspace restore Stage Fact or Runtime coordinates drifted", ports.ErrConflict)
	}
	return runtimeports.SealRestoreStageDomainResultCurrentProjectionV1(runtimeports.RestoreStageDomainResultCurrentProjectionV1{Fact: expected, CheckedUnixNano: expected.AuthoritativeTime, ExpiresUnixNano: expires}, now)
}

func (a *WorkspaceRestoreStageDomainResultAdapterV1) InspectRestoreStageDomainEvidenceCurrentV1(ctx context.Context, expected runtimeports.RestoreStageDomainResultFactRefV1) (runtimeports.RestoreStageDomainEvidenceCurrentProjectionV1, error) {
	if a == nil || nilLikeV4(ctx) || expected.Validate() != nil {
		return runtimeports.RestoreStageDomainEvidenceCurrentProjectionV1{}, errors.New("workspace restore Stage Evidence current lookup is invalid")
	}
	current, err := a.InspectRestoreStageDomainResultCurrentV1(ctx, expected)
	if err != nil {
		return runtimeports.RestoreStageDomainEvidenceCurrentProjectionV1{}, err
	}
	factRef, err := a.InspectWorkspaceRestoreStageFactRefV1(ctx, expected)
	if err != nil {
		return runtimeports.RestoreStageDomainEvidenceCurrentProjectionV1{}, err
	}
	fact, err := a.facts.InspectWorkspaceRestoreStageFactV1(ctx, factRef)
	if err != nil {
		return runtimeports.RestoreStageDomainEvidenceCurrentProjectionV1{}, err
	}
	payload, err := json.Marshal(fact)
	if err != nil || len(payload) == 0 || fact.ValidateShape() != nil || fact.ExactRef() != factRef {
		return runtimeports.RestoreStageDomainEvidenceCurrentProjectionV1{}, fmt.Errorf("%w: workspace restore Stage Evidence payload is not exact", ports.ErrConflict)
	}
	projection := runtimeports.RestoreStageDomainEvidenceCurrentProjectionV1{
		Domain: current,
		Payload: runtimeports.EvidencePayloadRefV2{
			Schema: expected.PayloadSchema, ContentDigest: expected.PayloadDigest, Revision: expected.PayloadRevision,
			Length: uint64(len(payload)), Ref: "sandbox-fact://workspace-restore-stage/" + fact.Meta.ID,
		},
	}
	return runtimeports.SealRestoreStageDomainEvidenceCurrentProjectionV1(projection, a.clock())
}

// InspectWorkspaceRestoreStageFactRefV1 resolves the Sandbox Owner's exact
// historical Fact ref from the durable Runtime binding. Runtime projections
// may shorten current TTL, so callers must never reconstruct this exact ref
// from a projection expiry.
func (a *WorkspaceRestoreStageDomainResultAdapterV1) InspectWorkspaceRestoreStageFactRefV1(ctx context.Context, expected runtimeports.RestoreStageDomainResultFactRefV1) (contract.SnapshotArtifactExactRefV2, error) {
	if a == nil || nilLikeV4(ctx) || expected.Validate() != nil {
		return contract.SnapshotArtifactExactRefV2{}, errors.New("workspace restore Stage exact Fact lookup is invalid")
	}
	binding, err := a.bindings.InspectWorkspaceRestoreStageRuntimeBindingV1(ctx, string(expected.TenantID), expected.ID)
	if err != nil {
		return contract.SnapshotArtifactExactRefV2{}, err
	}
	if binding.Validate() != nil || !runtimeports.SameRestoreStageDomainResultFactRefV1(binding.Runtime, expected) || binding.FactRef.ValidateCurrent("workspace restore stage fact", a.clock()) != nil {
		return contract.SnapshotArtifactExactRefV2{}, fmt.Errorf("%w: workspace restore Stage exact Fact binding drifted or is stale", ports.ErrConflict)
	}
	return binding.FactRef, nil
}

func (a *WorkspaceRestoreStageDomainResultAdapterV1) derive(ctx context.Context, factRef contract.SnapshotArtifactExactRefV2, request runtimeports.InspectRestoreStageGovernanceCurrentRequestV1, now time.Time) (runtimeports.RestoreStageDomainResultFactRefV1, contract.SnapshotArtifactExactRefV2, int64, error) {
	zero := runtimeports.RestoreStageDomainResultFactRefV1{}
	if now.IsZero() || factRef.ValidateCurrent("workspace restore stage fact", now) != nil || request.Validate() != nil {
		return zero, contract.SnapshotArtifactExactRefV2{}, 0, errors.New("workspace restore Stage derive coordinates are incomplete or stale")
	}
	fact, err := a.facts.InspectWorkspaceRestoreStageFactV1(ctx, factRef)
	if err != nil {
		return zero, contract.SnapshotArtifactExactRefV2{}, 0, err
	}
	if fact.ValidateShape() != nil || fact.ExactRef() != factRef {
		return zero, contract.SnapshotArtifactExactRefV2{}, 0, fmt.Errorf("%w: workspace restore Stage exact Fact drifted", ports.ErrConflict)
	}
	stable, err := workspaceRestoreStableKeyFromFactV1(fact)
	if err != nil {
		return zero, contract.SnapshotArtifactExactRefV2{}, 0, err
	}
	attempt, err := a.facts.InspectWorkspaceRestoreAttemptByStableKeyV1(ctx, stable)
	if err != nil {
		return zero, contract.SnapshotArtifactExactRefV2{}, 0, err
	}
	if attempt.ValidateShape() != nil || attempt.StageFactRef == nil || *attempt.StageFactRef != factRef || attempt.ProviderStageAttemptRef == nil || *attempt.ProviderStageAttemptRef != fact.AttemptRef || attempt.RootRef == nil || *attempt.RootRef != fact.RootRef || attempt.Request.RuntimeRestoreAttempt != fact.RuntimeRestoreAttempt || attempt.Request.RestoreEligibility != fact.RestoreEligibility || attempt.Request.Target != fact.Target || attempt.Governance == nil || *attempt.Governance != fact.Governance {
		return zero, contract.SnapshotArtifactExactRefV2{}, 0, fmt.Errorf("%w: workspace restore final Attempt and Stage Fact do not close", ports.ErrConflict)
	}
	governance, err := a.governance.InspectRestoreStageGovernanceCurrentV1(ctx, request)
	if err != nil {
		return zero, contract.SnapshotArtifactExactRefV2{}, 0, err
	}
	if err := governance.Validate(now); err != nil {
		return zero, contract.SnapshotArtifactExactRefV2{}, 0, err
	}
	mapped, err := mapRestoreStageProjectionV1(attempt.Request, governance, now)
	if err != nil || mapped != fact.Governance {
		return zero, contract.SnapshotArtifactExactRefV2{}, 0, fmt.Errorf("%w: Sandbox Stage Fact governance is not exact Runtime current", ports.ErrConflict)
	}
	if err := validateRestoreStageFactRuntimeClosureV1(fact, governance); err != nil {
		return zero, contract.SnapshotArtifactExactRefV2{}, 0, err
	}
	opDigest, err := governance.Operation.DigestV3()
	if err != nil {
		return zero, contract.SnapshotArtifactExactRefV2{}, 0, err
	}
	ref := runtimeports.RestoreStageDomainResultFactRefV1{Owner: a.owner, Kind: runtimeports.RestoreStageDomainResultKindV1, ID: fact.Meta.ID, Revision: runtimecore.Revision(fact.Meta.Revision), Digest: runtimeDigest(fact.Meta.Digest), TenantID: runtimecore.TenantID(fact.TenantID), Operation: governance.Operation, OperationDigest: opDigest, EffectID: governance.EffectID, EffectRevision: governance.EffectRevision, Attempt: governance.DispatchAttempt, RestoreAttempt: governance.RestoreAttempt, Eligibility: governance.Eligibility, PayloadSchema: a.schema, PayloadDigest: runtimeDigest(fact.Meta.Digest), PayloadRevision: runtimecore.Revision(fact.Meta.Revision), AuthoritativeTime: fact.Meta.UpdatedUnixNano}
	if err := ref.Validate(); err != nil {
		return zero, contract.SnapshotArtifactExactRefV2{}, 0, err
	}
	expires := minimumRestoreStageTTLRuntimeAdapterV1(fact.Meta.ExpiresUnixNano, attempt.Meta.ExpiresUnixNano, governance.ExpiresUnixNano)
	if now.UnixNano() >= expires {
		return zero, contract.SnapshotArtifactExactRefV2{}, 0, fmt.Errorf("%w: workspace restore Stage owner closure is stale", ports.ErrStale)
	}
	return ref, factRef, expires, nil
}

func workspaceRestoreStableKeyFromFactV1(fact contract.WorkspaceRestoreStageFactV1) (string, error) {
	return contract.Digest("praxis.sandbox/workspace-restore-stage-stable-key/v1", struct {
		TenantID          string
		DispatchAttemptID string
		Attempt           contract.SnapshotArtifactExactRefV2
		Target            contract.RuntimeLeaseBinding
	}{fact.TenantID, fact.AttemptRef.ID, fact.RuntimeRestoreAttempt, fact.Target})
}

func validateRestoreStageFactRuntimeClosureV1(fact contract.WorkspaceRestoreStageFactV1, governance runtimeports.RestoreStageGovernanceCurrentProjectionV1) error {
	if fact.TenantID != string(governance.RestoreAttempt.TenantID) || fact.RuntimeRestoreAttempt.ID != governance.RestoreAttempt.ID || fact.RuntimeRestoreAttempt.Revision != uint64(governance.RestoreAttempt.Revision) || !sameDigestV1(fact.RuntimeRestoreAttempt.Digest, string(governance.RestoreAttempt.Digest)) || fact.RestoreEligibility.ID != governance.Eligibility.ID || fact.RestoreEligibility.Revision != uint64(governance.Eligibility.Revision) || !sameDigestV1(fact.RestoreEligibility.Digest, string(governance.Eligibility.Digest)) || fact.Target.InstanceID != string(governance.Identity.TargetInstance.ID) || fact.Target.InstanceEpoch != uint64(governance.Identity.TargetInstance.Epoch) || fact.Target.LeaseID != string(governance.Identity.TargetLease.ID) || fact.Target.LeaseEpoch != uint64(governance.Identity.TargetLease.Epoch) || fact.Target.FenceEpoch != uint64(governance.Identity.TargetFenceEpoch) || fact.Target.ScopeDigest != string(governance.Operation.ExecutionScopeDigest) {
		return fmt.Errorf("%w: workspace restore Stage Fact crosses Runtime Attempt, Eligibility, Instance, Lease, or Fence", ports.ErrConflict)
	}
	return nil
}

var _ runtimeports.RestoreStageDomainResultCurrentReaderV1 = (*WorkspaceRestoreStageDomainResultAdapterV1)(nil)
var _ runtimeports.RestoreStageDomainEvidenceCurrentReaderV1 = (*WorkspaceRestoreStageDomainResultAdapterV1)(nil)

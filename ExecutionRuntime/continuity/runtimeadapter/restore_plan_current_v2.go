package runtimeadapter

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	continuityports "github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// RuntimeCheckpointConsistencyReaderV2 is the narrow public Runtime fact
// reader consumed by this Adapter. It grants no Checkpoint or Restore writes.
type RuntimeCheckpointConsistencyReaderV2 interface {
	InspectCheckpointConsistencyV2(context.Context, runtimeports.CheckpointConsistencyRefV2) (runtimeports.CheckpointConsistencyFactV2, error)
}

// RestorePlanCurrentReaderV2 projects an exact, Continuity-owned current Plan
// into Runtime's neutral reservation input. It does not create a Restore
// Attempt, grant Eligibility, or call a Provider.
type RestorePlanCurrentReaderV2 struct {
	Plans         continuityports.RestorePlanReaderV2
	ManifestSeals continuityports.CheckpointManifestReaderV2
	Consistency   RuntimeCheckpointConsistencyReaderV2
	Clock         func() time.Time
}

func (a RestorePlanCurrentReaderV2) InspectRestorePlanCurrentV2(ctx context.Context, expected runtimeports.CheckpointExternalExactFactRefV2) (runtimeports.RestorePlanCurrentProjectionV2, error) {
	if err := expected.Validate(); err != nil {
		return runtimeports.RestorePlanCurrentProjectionV2{}, err
	}
	if nilOrTypedNil(a.Plans) || nilOrTypedNil(a.ManifestSeals) || nilOrTypedNil(a.Consistency) || a.Clock == nil {
		return runtimeports.RestorePlanCurrentProjectionV2{}, runtimecore.NewError(runtimecore.ErrorUnavailable, runtimecore.ReasonComponentMissing, "Continuity Restore Plan current dependencies are unavailable")
	}
	now := a.Clock()
	if now.IsZero() {
		return runtimeports.RestorePlanCurrentProjectionV2{}, runtimecore.NewError(runtimecore.ErrorPreconditionFailed, runtimecore.ReasonClockRegression, "Restore Plan current clock is zero")
	}
	exact := externalExactRefToContinuityV2(expected)
	if err := contract.RestorePlanRefV2(exact).Validate(); err != nil {
		return runtimeports.RestorePlanCurrentProjectionV2{}, checkpointContinuityErrorV2(err)
	}
	plan, err := a.Plans.InspectCurrentRestorePlanV2(ctx, continuityports.InspectCurrentRestorePlanRequestV2{TenantID: exact.TenantID, ScopeDigest: exact.ScopeDigest, PlanID: exact.ID, Owner: exact.Owner})
	if err != nil {
		return runtimeports.RestorePlanCurrentProjectionV2{}, checkpointContinuityErrorV2(err)
	}
	if err := plan.ValidateCurrent(now); err != nil {
		return runtimeports.RestorePlanCurrentProjectionV2{}, checkpointContinuityErrorV2(err)
	}
	if !plan.Ref().Exact().Equal(exact) || plan.State != contract.RestorePlanSubmittedV2 {
		return runtimeports.RestorePlanCurrentProjectionV2{}, restorePlanAdapterConflictV2("Continuity Plan is not the exact submitted current revision")
	}
	seal, err := a.ManifestSeals.InspectCheckpointManifestSealV2(ctx, continuityports.InspectCheckpointManifestSealRequestV2{Ref: plan.ManifestSealRef})
	if err != nil {
		return runtimeports.RestorePlanCurrentProjectionV2{}, checkpointContinuityErrorV2(err)
	}
	if err := seal.Validate(); err != nil {
		return runtimeports.RestorePlanCurrentProjectionV2{}, checkpointContinuityErrorV2(err)
	}
	if seal.Ref() != plan.ManifestSealRef || seal.TenantID != plan.Scope.TenantID || seal.ScopeDigest != plan.Scope.ExecutionScopeDigest || seal.FrozenRefSetDigest != plan.FrozenRefSetDigest || seal.RequiredParticipantSetDigest != plan.RequiredParticipantSetDigest {
		return runtimeports.RestorePlanCurrentProjectionV2{}, restorePlanAdapterConflictV2("Restore Plan and immutable Manifest Seal closure drifted")
	}
	attempt, err := runtimeCheckpointAttemptRefFromExactV2(seal.CheckpointAttemptRef)
	if err != nil {
		return runtimeports.RestorePlanCurrentProjectionV2{}, err
	}
	consistencyDigest, err := runtimeports.NormalizeCheckpointExternalSHA256DigestV2(plan.CheckpointConsistencyRef.Digest)
	if err != nil {
		return runtimeports.RestorePlanCurrentProjectionV2{}, err
	}
	consistencyRef := runtimeports.CheckpointConsistencyRefV2{ID: plan.CheckpointConsistencyRef.ID, Revision: runtimecore.Revision(plan.CheckpointConsistencyRef.Revision), Attempt: attempt, Digest: consistencyDigest}
	if err := consistencyRef.Validate(); err != nil {
		return runtimeports.RestorePlanCurrentProjectionV2{}, err
	}
	consistency, err := a.Consistency.InspectCheckpointConsistencyV2(ctx, consistencyRef)
	if err != nil {
		return runtimeports.RestorePlanCurrentProjectionV2{}, err
	}
	if err := consistency.Validate(); err != nil {
		return runtimeports.RestorePlanCurrentProjectionV2{}, err
	}
	if consistency.Ref != consistencyRef || externalExactRefToContinuityV2(consistency.ManifestSeal.ExactLookup) != plan.ManifestSealRef.Exact() {
		return runtimeports.RestorePlanCurrentProjectionV2{}, restorePlanAdapterConflictV2("Runtime Consistency or Manifest Seal exact ref drifted")
	}
	frozenDigest, err := runtimeports.NormalizeCheckpointExternalSHA256DigestV2(plan.FrozenRefSetDigest)
	if err != nil {
		return runtimeports.RestorePlanCurrentProjectionV2{}, err
	}
	participantSetDigest, err := runtimeports.NormalizeCheckpointExternalSHA256DigestV2(seal.RuntimeParticipantSetDigest)
	if err != nil {
		return runtimeports.RestorePlanCurrentProjectionV2{}, err
	}
	if frozenDigest != consistency.FrozenRefSetDigest || participantSetDigest != consistency.ParticipantSetDigest {
		return runtimeports.RestorePlanCurrentProjectionV2{}, restorePlanAdapterConflictV2("Runtime Consistency digests drifted from Continuity Plan")
	}
	if plan.SourceInstanceRef.ID != plan.Scope.InstanceID || plan.SourceInstanceEpoch != plan.Scope.InstanceEpoch || plan.SourceInstanceRef.ID == plan.ProposedInstance.InstanceID {
		return runtimeports.RestorePlanCurrentProjectionV2{}, restorePlanAdapterConflictV2("Restore Plan source Instance is not exact or is reused")
	}
	sourceScopeDigest := runtimecore.Digest(plan.Scope.ExecutionScopeDigest)
	if err := sourceScopeDigest.Validate(); err != nil {
		return runtimeports.RestorePlanCurrentProjectionV2{}, err
	}
	projection := runtimeports.RestorePlanCurrentProjectionV2{
		RestorePlan: expected, State: runtimeports.RestorePlanSubmittedStateV2, CheckpointConsistency: consistency,
		ManifestSeal: consistency.ManifestSeal, SourceScopeDigest: sourceScopeDigest,
		IdentityProposal: runtimeports.RestoreIdentityReservationV2{
			SourceInstance:   runtimecore.InstanceRef{ID: runtimecore.AgentInstanceID(plan.SourceInstanceRef.ID), Epoch: runtimecore.Epoch(plan.SourceInstanceEpoch)},
			TargetInstance:   runtimecore.InstanceRef{ID: runtimecore.AgentInstanceID(plan.ProposedInstance.InstanceID), Epoch: runtimecore.Epoch(plan.ProposedInstance.Epoch)},
			TargetLease:      runtimecore.SandboxLeaseRef{ID: runtimecore.SandboxLeaseID(plan.ProposedInstance.LeaseID), Epoch: runtimecore.Epoch(plan.ProposedInstance.LeaseEpoch)},
			TargetFenceEpoch: runtimecore.Epoch(plan.ProposedInstance.FenceEpoch),
		},
		ConflictDomain: plan.ConflictDomain, RequiredParticipantSetDigest: participantSetDigest,
		CheckedUnixNano: plan.UpdatedUnixNano, ExpiresUnixNano: plan.ExpiresUnixNano,
	}
	return runtimeports.SealRestorePlanCurrentProjectionV2(projection, now)
}

func runtimeCheckpointAttemptRefFromExactV2(ref contract.ExactFactRefV2) (runtimeports.CheckpointAttemptRefV2, error) {
	if err := ref.Validate(); err != nil {
		return runtimeports.CheckpointAttemptRefV2{}, checkpointContinuityErrorV2(err)
	}
	if ref.Owner.ComponentID != "praxis/runtime" || ref.Owner.FactKind != "checkpoint_attempt_fact_v2" {
		return runtimeports.CheckpointAttemptRefV2{}, restorePlanAdapterConflictV2("Manifest Seal Attempt ref has the wrong Owner")
	}
	digest, err := runtimeports.NormalizeCheckpointExternalSHA256DigestV2(ref.Digest)
	if err != nil {
		return runtimeports.CheckpointAttemptRefV2{}, err
	}
	result := runtimeports.CheckpointAttemptRefV2{TenantID: runtimecore.TenantID(ref.TenantID), ID: ref.ID, Revision: runtimecore.Revision(ref.Revision), Digest: digest}
	return result, result.Validate()
}

func restorePlanAdapterConflictV2(message string) error {
	return runtimecore.NewError(runtimecore.ErrorConflict, runtimecore.ReasonRestoreIncompatible, message)
}

var _ runtimeports.RestorePlanCurrentReaderV2 = RestorePlanCurrentReaderV2{}

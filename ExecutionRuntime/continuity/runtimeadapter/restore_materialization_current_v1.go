package runtimeadapter

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	continuityports "github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// RestoreMaterializationCurrentReaderV1 joins only exact, already-owned facts:
// Runtime's current Attempt/Eligibility and Plan projection with Continuity's
// immutable Manifest closure. It neither reads artifact payloads nor grants a
// Restore Stage, Context write, Activation, or Provider call.
type RestoreMaterializationCurrentReaderV1 struct {
	Restore     runtimeports.RestoreGovernancePortV2
	PlanCurrent runtimeports.RestorePlanCurrentReaderV2
	Plans       continuityports.RestorePlanReaderV2
	Manifests   continuityports.CheckpointManifestReaderV2
	Clock       func() time.Time
}

func (a RestoreMaterializationCurrentReaderV1) InspectRestoreMaterializationCurrentV1(ctx context.Context, request runtimeports.InspectRestoreMaterializationCurrentRequestV1) (runtimeports.RestoreMaterializationCurrentProjectionV1, error) {
	if err := request.Validate(); err != nil {
		return runtimeports.RestoreMaterializationCurrentProjectionV1{}, err
	}
	if nilOrTypedNil(a.Restore) || nilOrTypedNil(a.PlanCurrent) || nilOrTypedNil(a.Plans) || nilOrTypedNil(a.Manifests) || a.Clock == nil {
		return runtimeports.RestoreMaterializationCurrentProjectionV1{}, runtimecore.NewError(runtimecore.ErrorUnavailable, runtimecore.ReasonComponentMissing, "Restore materialization exact readers are unavailable")
	}
	now := a.Clock()
	if now.IsZero() {
		return runtimeports.RestoreMaterializationCurrentProjectionV1{}, runtimecore.NewError(runtimecore.ErrorPreconditionFailed, runtimecore.ReasonClockRegression, "Restore materialization clock is zero")
	}

	attempt, err := a.Restore.InspectRestoreAttemptV2(ctx, runtimeports.InspectRestoreAttemptRequestV2{TenantID: request.Attempt.TenantID, AttemptID: request.Attempt.ID})
	if err != nil {
		return runtimeports.RestoreMaterializationCurrentProjectionV1{}, err
	}
	if err := attempt.Validate(); err != nil {
		return runtimeports.RestoreMaterializationCurrentProjectionV1{}, err
	}
	if attempt.Ref != request.Attempt || attempt.Eligibility == nil || *attempt.Eligibility != request.Eligibility {
		return runtimeports.RestoreMaterializationCurrentProjectionV1{}, restorePlanAdapterConflictV2("Restore materialization Attempt or Eligibility current ref drifted")
	}
	eligibility, err := a.Restore.InspectCurrentRestoreEligibilityV2(ctx, runtimeports.InspectRestoreEligibilityCurrentRequestV2{Attempt: attempt.Ref, ExpectedEligibility: request.Eligibility})
	if err != nil {
		return runtimeports.RestoreMaterializationCurrentProjectionV1{}, err
	}
	if err := eligibility.ValidateCurrent(now); err != nil {
		return runtimeports.RestoreMaterializationCurrentProjectionV1{}, err
	}
	if eligibility.Ref != request.Eligibility || eligibility.RestorePlan != attempt.OperationScope.RestorePlan || eligibility.CheckpointConsistency != attempt.OperationScope.Consistency || eligibility.Identity != attempt.OperationScope.Identity {
		return runtimeports.RestoreMaterializationCurrentProjectionV1{}, restorePlanAdapterConflictV2("Restore materialization Eligibility closure drifted")
	}

	planCurrent, err := a.PlanCurrent.InspectRestorePlanCurrentV2(ctx, attempt.OperationScope.RestorePlan)
	if err != nil {
		return runtimeports.RestoreMaterializationCurrentProjectionV1{}, err
	}
	if err := planCurrent.Validate(now); err != nil {
		return runtimeports.RestoreMaterializationCurrentProjectionV1{}, err
	}
	if planCurrent.RestorePlan != eligibility.RestorePlan || planCurrent.CheckpointConsistency.Ref != eligibility.CheckpointConsistency || planCurrent.IdentityProposal != eligibility.Identity || planCurrent.SourceScopeDigest != attempt.OperationScope.SourceScopeDigest {
		return runtimeports.RestoreMaterializationCurrentProjectionV1{}, restorePlanAdapterConflictV2("Restore materialization Plan current closure drifted")
	}

	planExpected := externalExactRefToContinuityV2(planCurrent.RestorePlan)
	plan, err := a.Plans.InspectCurrentRestorePlanV2(ctx, continuityports.InspectCurrentRestorePlanRequestV2{TenantID: planExpected.TenantID, ScopeDigest: planExpected.ScopeDigest, PlanID: planExpected.ID, Owner: planExpected.Owner})
	if err != nil {
		return runtimeports.RestoreMaterializationCurrentProjectionV1{}, checkpointContinuityErrorV2(err)
	}
	if err := plan.ValidateCurrent(now); err != nil {
		return runtimeports.RestoreMaterializationCurrentProjectionV1{}, checkpointContinuityErrorV2(err)
	}
	if !plan.Ref().Exact().Equal(planExpected) || plan.State != contract.RestorePlanSubmittedV2 {
		return runtimeports.RestoreMaterializationCurrentProjectionV1{}, restorePlanAdapterConflictV2("Restore materialization Plan body is not exact submitted current")
	}

	seal, err := a.Manifests.InspectCheckpointManifestSealV2(ctx, continuityports.InspectCheckpointManifestSealRequestV2{Ref: plan.ManifestSealRef})
	if err != nil {
		return runtimeports.RestoreMaterializationCurrentProjectionV1{}, checkpointContinuityErrorV2(err)
	}
	if err := seal.Validate(); err != nil {
		return runtimeports.RestoreMaterializationCurrentProjectionV1{}, checkpointContinuityErrorV2(err)
	}
	manifestDigest, err := runtimeports.NormalizeCheckpointExternalSHA256DigestV2(seal.ManifestRef.Exact().Digest)
	if err != nil {
		return runtimeports.RestoreMaterializationCurrentProjectionV1{}, err
	}
	if !seal.Ref().Exact().Equal(externalExactRefToContinuityV2(planCurrent.ManifestSeal.ExactLookup)) || seal.ManifestRef.Exact().ID != planCurrent.ManifestSeal.ManifestID || seal.ManifestRef.Exact().Revision != uint64(planCurrent.ManifestSeal.ManifestRevision) || manifestDigest != planCurrent.ManifestSeal.ManifestDigest {
		return runtimeports.RestoreMaterializationCurrentProjectionV1{}, restorePlanAdapterConflictV2("Restore materialization immutable Seal or Manifest ref drifted")
	}
	manifest, err := a.Manifests.InspectCheckpointManifestV2(ctx, continuityports.InspectCheckpointManifestRequestV2{Ref: seal.ManifestRef})
	if err != nil {
		return runtimeports.RestoreMaterializationCurrentProjectionV1{}, checkpointContinuityErrorV2(err)
	}
	if err := manifest.Validate(); err != nil {
		return runtimeports.RestoreMaterializationCurrentProjectionV1{}, checkpointContinuityErrorV2(err)
	}
	if manifest.Ref() != seal.ManifestRef || manifest.State != contract.ManifestVerifiedCandidate || !manifest.ContextGenerationRef.Equal(plan.ContextGenerationRef) || !sameContinuityExactRefSetV1(manifest.ContextFrameRefs, plan.ContextFrameRefs) {
		return runtimeports.RestoreMaterializationCurrentProjectionV1{}, restorePlanAdapterConflictV2("Restore materialization Manifest or Context closure drifted")
	}
	contextDigest, err := contract.ContextClosureDigestV2(manifest)
	if err != nil {
		return runtimeports.RestoreMaterializationCurrentProjectionV1{}, checkpointContinuityErrorV2(err)
	}
	artifactDigest, err := contract.ArtifactClosureDigestV2(manifest)
	if err != nil {
		return runtimeports.RestoreMaterializationCurrentProjectionV1{}, checkpointContinuityErrorV2(err)
	}
	participantDigest, err := contract.ParticipantClosuresDigestV2(manifest.ParticipantClosures)
	if err != nil {
		return runtimeports.RestoreMaterializationCurrentProjectionV1{}, checkpointContinuityErrorV2(err)
	}
	sealedParticipantDigest, err := contract.ParticipantClosuresDigestV2(seal.ParticipantClosures)
	if err != nil {
		return runtimeports.RestoreMaterializationCurrentProjectionV1{}, checkpointContinuityErrorV2(err)
	}
	if contextDigest != seal.ContextClosureDigest || artifactDigest != seal.ArtifactClosureDigest || participantDigest != sealedParticipantDigest {
		return runtimeports.RestoreMaterializationCurrentProjectionV1{}, restorePlanAdapterConflictV2("Restore materialization sealed closure digest drifted")
	}

	snapshots := make([]runtimeports.CheckpointExternalExactFactRefV2, 0, len(manifest.ParticipantClosures))
	for _, participant := range manifest.ParticipantClosures {
		if participant.SnapshotRef != nil {
			snapshots = append(snapshots, continuityExactRefToExternalV1(*participant.SnapshotRef))
		}
	}
	checked := maxInt64V1(planCurrent.CheckedUnixNano, plan.UpdatedUnixNano, manifest.UpdatedUnixNano, eligibility.UpdatedUnixNano)
	expires := minInt64V1(planCurrent.ExpiresUnixNano, plan.ExpiresUnixNano, eligibility.Ref.ExpiresUnixNano)
	projection := runtimeports.RestoreMaterializationCurrentProjectionV1{
		Attempt: request.Attempt, Eligibility: request.Eligibility,
		RestorePlan: planCurrent.RestorePlan, Consistency: planCurrent.CheckpointConsistency.Ref,
		ManifestSeal: planCurrent.ManifestSeal, SourceScopeDigest: planCurrent.SourceScopeDigest,
		Identity:          planCurrent.IdentityProposal,
		ContextGeneration: continuityExactRefToExternalV1(manifest.ContextGenerationRef),
		ContextFrames:     continuityExactRefsToExternalV1(manifest.ContextFrameRefs),
		Memory:            continuityExactRefsToExternalV1(manifest.MemoryRefs),
		Knowledge:         continuityExactRefsToExternalV1(manifest.KnowledgeRefs),
		Snapshots:         snapshots,
		CheckedUnixNano:   checked, ExpiresUnixNano: expires,
	}
	return runtimeports.SealRestoreMaterializationCurrentProjectionV1(projection, now)
}

func continuityExactRefToExternalV1(ref contract.ExactFactRefV2) runtimeports.CheckpointExternalExactFactRefV2 {
	return runtimeports.CheckpointExternalExactFactRefV2{
		ContractVersion: ref.ContractVersion, SchemaRef: ref.SchemaRef,
		Owner: runtimeports.CheckpointManifestSealOwnerBindingV2{
			BindingSetID: ref.Owner.BindingSetID, BindingRevision: runtimecore.Revision(ref.Owner.BindingRevision),
			ComponentID: ref.Owner.ComponentID, ManifestDigest: ref.Owner.ManifestDigest,
			ArtifactDigest: ref.Owner.ArtifactDigest, Capability: ref.Owner.Capability, FactKind: ref.Owner.FactKind,
		},
		TenantID: ref.TenantID, ID: ref.ID, Revision: runtimecore.Revision(ref.Revision), Digest: ref.Digest, ScopeDigest: ref.ScopeDigest,
	}
}

func continuityExactRefsToExternalV1(refs []contract.ExactFactRefV2) []runtimeports.CheckpointExternalExactFactRefV2 {
	result := make([]runtimeports.CheckpointExternalExactFactRefV2, len(refs))
	for index := range refs {
		result[index] = continuityExactRefToExternalV1(refs[index])
	}
	return result
}

func sameContinuityExactRefSetV1(left, right []contract.ExactFactRefV2) bool {
	if len(left) != len(right) {
		return false
	}
	values := make(map[contract.ExactFactIdentityKeyV2]struct{}, len(left))
	for _, ref := range left {
		values[ref.IdentityKey()] = struct{}{}
	}
	if len(values) != len(left) {
		return false
	}
	for _, ref := range right {
		if _, ok := values[ref.IdentityKey()]; !ok {
			return false
		}
	}
	return true
}

func maxInt64V1(values ...int64) int64 {
	result := values[0]
	for _, value := range values[1:] {
		if value > result {
			result = value
		}
	}
	return result
}

func minInt64V1(values ...int64) int64 {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

var _ runtimeports.RestoreMaterializationCurrentReaderV1 = RestoreMaterializationCurrentReaderV1{}

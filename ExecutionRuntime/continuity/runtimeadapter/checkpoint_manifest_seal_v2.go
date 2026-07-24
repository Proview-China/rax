package runtimeadapter

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	continuityports "github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// CheckpointManifestSealReaderV2 is a one-way public adapter. It only inspects
// an immutable Continuity Seal and projects exact references required by the
// Runtime consistency gateway. It owns no Barrier, Participant or Runtime Fact.
type CheckpointManifestSealReaderV2 struct {
	Manifests continuityports.CheckpointManifestReaderV2
}

func (a CheckpointManifestSealReaderV2) InspectCheckpointManifestSealV2(
	ctx context.Context,
	request runtimeports.InspectCheckpointManifestSealRequestV2,
) (runtimeports.CheckpointManifestSealProjectionV2, error) {
	if err := request.Validate(); err != nil {
		return runtimeports.CheckpointManifestSealProjectionV2{}, err
	}
	if nilOrTypedNil(a.Manifests) {
		return runtimeports.CheckpointManifestSealProjectionV2{}, runtimecore.NewError(runtimecore.ErrorUnavailable, runtimecore.ReasonComponentMissing, "Continuity Manifest Seal Reader is unavailable")
	}
	exact := externalExactRefToContinuityV2(request.Ref.ExactLookup)
	seal, err := a.Manifests.InspectCheckpointManifestSealV2(ctx, continuityports.InspectCheckpointManifestSealRequestV2{Ref: contract.CheckpointManifestSealRefV2(exact)})
	if err != nil {
		return runtimeports.CheckpointManifestSealProjectionV2{}, checkpointContinuityErrorV2(err)
	}
	if err := seal.Validate(); err != nil {
		return runtimeports.CheckpointManifestSealProjectionV2{}, checkpointContinuityErrorV2(err)
	}
	if !seal.Ref().Exact().Equal(exact) {
		return runtimeports.CheckpointManifestSealProjectionV2{}, checkpointAdapterConflictV2("Continuity Seal Reader returned another exact ref")
	}
	if err := validateCheckpointSealRuntimeBindingsV2(seal, request); err != nil {
		return runtimeports.CheckpointManifestSealProjectionV2{}, err
	}
	contextDigest, err := runtimeports.NormalizeCheckpointExternalSHA256DigestV2(seal.ContextClosureDigest)
	if err != nil {
		return runtimeports.CheckpointManifestSealProjectionV2{}, err
	}
	artifactDigest, err := runtimeports.NormalizeCheckpointExternalSHA256DigestV2(seal.ArtifactClosureDigest)
	if err != nil {
		return runtimeports.CheckpointManifestSealProjectionV2{}, err
	}
	projection := runtimeports.CheckpointManifestSealProjectionV2{
		ContractVersion:       runtimeports.CheckpointManifestSealContractVersionV2,
		Ref:                   request.Ref,
		ParticipantSetDigest:  request.ExpectedParticipantSetDigest,
		ParticipantClosures:   append([]runtimeports.CheckpointParticipantClosureRefV2{}, request.ExpectedParticipantClosures...),
		ContextClosureDigest:  contextDigest,
		ArtifactClosureDigest: artifactDigest,
	}
	projection.SealDigest, err = projection.DigestV2()
	if err != nil {
		return runtimeports.CheckpointManifestSealProjectionV2{}, err
	}
	if err := projection.Validate(); err != nil {
		return runtimeports.CheckpointManifestSealProjectionV2{}, err
	}
	return projection, nil
}

func validateCheckpointSealRuntimeBindingsV2(seal contract.CheckpointManifestSealFactV2, request runtimeports.InspectCheckpointManifestSealRequestV2) error {
	ref := request.Ref
	manifestDigest, err := runtimeports.NormalizeCheckpointExternalSHA256DigestV2(seal.ManifestRef.Exact().Digest)
	if err != nil {
		return err
	}
	frozenDigest, err := runtimeports.NormalizeCheckpointExternalSHA256DigestV2(seal.FrozenRefSetDigest)
	if err != nil {
		return err
	}
	participantSetDigest, err := runtimeports.NormalizeCheckpointExternalSHA256DigestV2(seal.RuntimeParticipantSetDigest)
	if err != nil {
		return err
	}
	if seal.ManifestRef.Exact().ID != ref.ManifestID || seal.ManifestRef.Exact().Revision != uint64(ref.ManifestRevision) || manifestDigest != ref.ManifestDigest || frozenDigest != ref.FrozenRefSetDigest || participantSetDigest != request.ExpectedParticipantSetDigest {
		return checkpointAdapterConflictV2("Continuity Seal changed Manifest, frozen refs, or Participant Set")
	}
	if err := sameRuntimeExactRefV2(seal.CheckpointAttemptRef, string(ref.Attempt.TenantID), ref.ExactLookup.ScopeDigest, ref.Attempt.ID, ref.Attempt.Revision, ref.Attempt.Digest); err != nil {
		return err
	}
	if err := sameRuntimeExactRefV2(seal.BarrierRef, string(ref.Barrier.TenantID), ref.ExactLookup.ScopeDigest, ref.Barrier.ID, ref.Barrier.Revision, ref.Barrier.Digest); err != nil {
		return err
	}
	if err := sameRuntimeExactRefV2(seal.EffectCutRef, string(ref.Attempt.TenantID), ref.ExactLookup.ScopeDigest, ref.EffectCut.ID, ref.EffectCut.Revision, ref.EffectCut.Digest); err != nil {
		return err
	}
	if len(seal.ParticipantClosures) != len(request.ExpectedParticipantClosures) {
		return checkpointAdapterConflictV2("Continuity Seal Participant closure cardinality drifted")
	}
	sealedByRuntimeID := make(map[string]contract.ParticipantClosureRefV2, len(seal.ParticipantClosures))
	for _, closure := range seal.ParticipantClosures {
		if closure.ParticipantID == "" || closure.RuntimeClosureRef.ID == "" {
			return checkpointAdapterConflictV2("Continuity Seal has an incomplete Runtime Participant mapping")
		}
		if _, exists := sealedByRuntimeID[closure.RuntimeClosureRef.ID]; exists {
			return checkpointAdapterConflictV2("Continuity Seal repeats a Runtime Participant closure")
		}
		sealedByRuntimeID[closure.RuntimeClosureRef.ID] = closure
	}
	for _, closure := range request.ExpectedParticipantClosures {
		expectedExternal, err := runtimeports.DeriveCheckpointParticipantClosureExactRefV2(ref.Attempt.TenantID, ref.ExactLookup.ScopeDigest, closure)
		if err != nil {
			return err
		}
		sealed, ok := sealedByRuntimeID[closure.ID]
		if !ok || sealed.ParticipantID != closure.Participant.ID || !sealed.RuntimeClosureRef.Equal(externalExactRefToContinuityV2(expectedExternal)) {
			return checkpointAdapterConflictV2("Continuity Seal does not exactly bind the Runtime Participant closure")
		}
	}
	return nil
}

func sameRuntimeExactRefV2(actual contract.ExactFactRefV2, tenantID, scopeDigest, id string, revision runtimecore.Revision, digest runtimecore.Digest) error {
	if err := actual.Validate(); err != nil {
		return checkpointContinuityErrorV2(err)
	}
	normalized, err := runtimeports.NormalizeCheckpointExternalSHA256DigestV2(actual.Digest)
	if err != nil {
		return err
	}
	if actual.Owner.ComponentID != "praxis/runtime" || actual.TenantID != tenantID || actual.ScopeDigest != scopeDigest || actual.ID != id || actual.Revision != uint64(revision) || normalized != digest {
		return checkpointAdapterConflictV2("Continuity Seal contains a drifted Runtime exact ref")
	}
	return nil
}

func externalExactRefToContinuityV2(ref runtimeports.CheckpointExternalExactFactRefV2) contract.ExactFactRefV2 {
	return contract.ExactFactRefV2{
		ContractVersion: ref.ContractVersion,
		SchemaRef:       ref.SchemaRef,
		Owner: contract.OwnerBinding{
			BindingSetID: ref.Owner.BindingSetID, BindingRevision: uint64(ref.Owner.BindingRevision),
			ComponentID: ref.Owner.ComponentID, ManifestDigest: ref.Owner.ManifestDigest,
			ArtifactDigest: ref.Owner.ArtifactDigest, Capability: ref.Owner.Capability, FactKind: ref.Owner.FactKind,
		},
		TenantID: ref.TenantID, ID: ref.ID, Revision: uint64(ref.Revision), Digest: ref.Digest, ScopeDigest: ref.ScopeDigest,
	}
}

func checkpointAdapterConflictV2(message string) error {
	return runtimecore.NewError(runtimecore.ErrorConflict, runtimecore.ReasonCheckpointInconsistent, message)
}

func checkpointContinuityErrorV2(err error) error {
	switch {
	case contract.HasCode(err, contract.ErrNotFound):
		return runtimecore.NewError(runtimecore.ErrorNotFound, runtimecore.ReasonCheckpointInconsistent, err.Error())
	case contract.HasCode(err, contract.ErrUnavailable):
		return runtimecore.NewError(runtimecore.ErrorUnavailable, runtimecore.ReasonComponentMissing, err.Error())
	case contract.HasCode(err, contract.ErrIndeterminate), contract.HasCode(err, contract.ErrCheckpointIndeterminate):
		return runtimecore.NewError(runtimecore.ErrorIndeterminate, runtimecore.ReasonCheckpointInconsistent, err.Error())
	case contract.HasCode(err, contract.ErrRevisionConflict), contract.HasCode(err, contract.ErrProjectionConflict), contract.HasCode(err, contract.ErrCheckpointPartial), contract.HasCode(err, contract.ErrPreconditionFailed):
		return checkpointAdapterConflictV2(err.Error())
	default:
		return runtimecore.NewError(runtimecore.ErrorInvalidArgument, runtimecore.ReasonCheckpointInconsistent, err.Error())
	}
}

var _ runtimeports.CheckpointManifestSealReaderV2 = CheckpointManifestSealReaderV2{}

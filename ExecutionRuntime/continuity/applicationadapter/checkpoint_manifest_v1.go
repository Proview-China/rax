package applicationadapter

import (
	"context"
	"reflect"
	"time"

	appcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	continuityports "github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type CheckpointManifestApplicationAdapterConfigV1 struct {
	Manifests     continuityports.CheckpointManifestGovernancePortV2
	ManifestOwner contract.OwnerBinding
	SealOwner     contract.OwnerBinding
	RuntimeOwner  contract.OwnerBinding
	Clock         func() time.Time
}

type CheckpointManifestApplicationAdapterV1 struct {
	config CheckpointManifestApplicationAdapterConfigV1
}

func NewCheckpointManifestApplicationAdapterV1(config CheckpointManifestApplicationAdapterConfigV1) (*CheckpointManifestApplicationAdapterV1, error) {
	if checkpointAppNilV1(config.Manifests) || config.Clock == nil || config.ManifestOwner.Validate() != nil || config.SealOwner.Validate() != nil || config.RuntimeOwner.Validate() != nil || config.ManifestOwner.ComponentID != contract.ContinuityComponentID || config.ManifestOwner.Capability != contract.CheckpointManifestCapabilityV2 || config.ManifestOwner.FactKind != "checkpoint_manifest_fact_v2" || config.SealOwner.ComponentID != contract.ContinuityComponentID || config.SealOwner.Capability != contract.CheckpointManifestCapabilityV2 || config.SealOwner.FactKind != "checkpoint_manifest_seal_fact_v2" || config.RuntimeOwner.ComponentID != "praxis/runtime" {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Continuity checkpoint Manifest Application adapter dependencies or Owner bindings are invalid")
	}
	return &CheckpointManifestApplicationAdapterV1{config: config}, nil
}

func (a *CheckpointManifestApplicationAdapterV1) CreateCheckpointManifestSealV1(ctx context.Context, request appcontract.CreateCheckpointManifestSealRequestV1) (runtimeports.CheckpointManifestSealRefV2, error) {
	now := a.config.Clock()
	if err := request.Validate(now); err != nil {
		return runtimeports.CheckpointManifestSealRefV2{}, err
	}
	manifest, err := a.buildManifestV1(request, contract.ManifestCollecting, 1, now.UnixNano(), now.UnixNano())
	if err != nil {
		return runtimeports.CheckpointManifestSealRefV2{}, err
	}
	created, _, err := a.config.Manifests.CreateCheckpointManifestV2(ctx, continuityports.CreateCheckpointManifestRequestV2{Candidate: manifest, ExpectAbsent: true})
	if err != nil {
		var inspectErr error
		created, inspectErr = a.config.Manifests.InspectCheckpointManifestV2(context.WithoutCancel(ctx), continuityports.InspectCheckpointManifestRequestV2{Ref: manifest.Ref()})
		if inspectErr != nil {
			if checkpointDefinitiveContinuityErrorV1(inspectErr) {
				return runtimeports.CheckpointManifestSealRefV2{}, checkpointAppErrorV1(inspectErr)
			}
			return runtimeports.CheckpointManifestSealRefV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonCheckpointInconsistent, "Continuity Manifest create outcome cannot be inspected")
		}
		if !created.Ref().Exact().Equal(manifest.Ref().Exact()) {
			return runtimeports.CheckpointManifestSealRefV2{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "Continuity Manifest create recovery changed exact content")
		}
	}
	verified, err := a.buildManifestV1(request, contract.ManifestVerifiedCandidate, created.Revision+1, created.CreatedUnixNano, maxCheckpointTimeV1(created.UpdatedUnixNano, a.config.Clock().UnixNano()))
	if err != nil {
		return runtimeports.CheckpointManifestSealRefV2{}, err
	}
	expectedVerified := verified.Clone()
	verified, _, err = a.config.Manifests.CompareAndSwapCheckpointManifestV2(ctx, continuityports.CompareAndSwapCheckpointManifestRequestV2{Expected: created.Ref(), Next: expectedVerified})
	if err != nil {
		var inspectErr error
		verified, inspectErr = a.config.Manifests.InspectCheckpointManifestV2(context.WithoutCancel(ctx), continuityports.InspectCheckpointManifestRequestV2{Ref: expectedVerified.Ref()})
		if inspectErr != nil {
			if checkpointDefinitiveContinuityErrorV1(inspectErr) {
				return runtimeports.CheckpointManifestSealRefV2{}, checkpointAppErrorV1(inspectErr)
			}
			return runtimeports.CheckpointManifestSealRefV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonCheckpointInconsistent, "Continuity Manifest CAS outcome cannot be inspected")
		}
		if !verified.Ref().Exact().Equal(expectedVerified.Ref().Exact()) {
			return runtimeports.CheckpointManifestSealRefV2{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "Continuity Manifest CAS recovery changed exact content")
		}
	}
	seal, err := a.buildSealV1(request, verified, maxCheckpointTimeV1(verified.UpdatedUnixNano+1, a.config.Clock().UnixNano()))
	if err != nil {
		return runtimeports.CheckpointManifestSealRefV2{}, err
	}
	stored, _, err := a.config.Manifests.CreateCheckpointManifestSealV2(ctx, continuityports.CreateCheckpointManifestSealRequestV2{Seal: seal})
	if err != nil {
		stored, err = a.config.Manifests.InspectCheckpointManifestSealV2(context.WithoutCancel(ctx), continuityports.InspectCheckpointManifestSealRequestV2{Ref: seal.Ref()})
		if err != nil {
			return runtimeports.CheckpointManifestSealRefV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonCheckpointInconsistent, "Continuity Manifest Seal outcome cannot be inspected")
		}
	}
	if !stored.Ref().Exact().Equal(seal.Ref().Exact()) {
		return runtimeports.CheckpointManifestSealRefV2{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "Continuity Manifest Seal recovery changed exact content")
	}
	return runtimeSealRefV1(stored, request)
}

func (a *CheckpointManifestApplicationAdapterV1) InspectCheckpointManifestSealV1(ctx context.Context, request appcontract.InspectCheckpointManifestSealRequestV1) (runtimeports.CheckpointManifestSealRefV2, error) {
	if err := request.Validate(); err != nil {
		return runtimeports.CheckpointManifestSealRefV2{}, err
	}
	exact := runtimeExactToContinuityV1(request.Ref.ExactLookup)
	seal, err := a.config.Manifests.InspectCheckpointManifestSealV2(ctx, continuityports.InspectCheckpointManifestSealRequestV2{Ref: contract.CheckpointManifestSealRefV2(exact)})
	if err != nil {
		return runtimeports.CheckpointManifestSealRefV2{}, checkpointAppErrorV1(err)
	}
	if !seal.Ref().Exact().Equal(exact) {
		return runtimeports.CheckpointManifestSealRefV2{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "Continuity Manifest Seal Inspect returned another exact ref")
	}
	return request.Ref, nil
}

func (a *CheckpointManifestApplicationAdapterV1) buildManifestV1(request appcontract.CreateCheckpointManifestSealRequestV1, state contract.ManifestState, revision uint64, created, updated int64) (contract.CheckpointManifestFactV2, error) {
	scopeDigest := string(request.Gate.ScopeDigest)
	manifest := contract.CheckpointManifestFactV2{ContractVersion: contract.CheckpointManifestGovernanceContractV2, SchemaRef: contract.CheckpointManifestFactSchemaV2, ManifestID: request.StableID, Revision: revision, Owner: a.config.ManifestOwner, Scope: continuityScopeV1(request, scopeDigest), State: state, IdempotencyKey: request.IdempotencyKey, CheckpointAttemptRef: runtimeAttemptExactV1(request.Attempt, scopeDigest, a.config.RuntimeOwner), BarrierRef: runtimeBarrierExactV1(request.Barrier, scopeDigest, a.config.RuntimeOwner), EffectCutRef: runtimeCutExactV1(request.EffectCut, scopeDigest, a.config.RuntimeOwner), TimelineCut: contract.TimelineCutV2{LedgerScopeDigest: string(request.Input.Timeline.LedgerScopeDigest), LedgerSequence: uint64(request.Input.Timeline.LedgerSequence), EvidenceRecordRef: appExactToContinuityV1(request.Input.Timeline.EvidenceRecord)}, ContextGenerationRef: appExactToContinuityV1(request.Input.ContextGeneration), RuntimeParticipantSetDigest: string(request.ParticipantSet.Certification.Digest), CreatedUnixNano: created, UpdatedUnixNano: updated}
	for _, ref := range request.Input.ContextFrames {
		manifest.ContextFrameRefs = append(manifest.ContextFrameRefs, appExactToContinuityV1(ref))
	}
	for _, ref := range request.Input.Memory {
		manifest.MemoryRefs = append(manifest.MemoryRefs, appExactToContinuityV1(ref))
	}
	for _, ref := range request.Input.Knowledge {
		manifest.KnowledgeRefs = append(manifest.KnowledgeRefs, appExactToContinuityV1(ref))
	}
	for _, value := range request.Input.AttemptSettlements {
		closure := contract.AttemptSettlementClosureV2{AttemptRef: appExactToContinuityV1(value.Attempt), Begun: value.Begun}
		if value.Settlement != nil {
			ref := appExactToContinuityV1(*value.Settlement)
			closure.SettlementRef = &ref
		}
		if value.Inspection != nil {
			ref := appExactToContinuityV1(*value.Inspection)
			closure.InspectionRef = &ref
		}
		for _, ref := range value.Residuals {
			closure.ResidualRefs = append(closure.ResidualRefs, appExactToContinuityV1(ref))
		}
		manifest.AttemptSettlementClosures = append(manifest.AttemptSettlementClosures, closure)
	}
	for _, value := range request.Participants {
		exact, err := runtimeports.DeriveCheckpointParticipantClosureExactRefV2(request.Attempt.TenantID, scopeDigest, value.RuntimeClosure)
		if err != nil {
			return contract.CheckpointManifestFactV2{}, err
		}
		participant := contract.ParticipantClosureRefV2{ParticipantID: value.RuntimeClosure.Participant.ID, Required: true, RuntimeClosureRef: runtimeExactToContinuityV1(exact), ParticipantFactRef: appExactToContinuityV1(value.ParticipantFact)}
		snapshot, coverage := appExactToContinuityV1(value.Snapshot), appExactToContinuityV1(value.Coverage)
		participant.SnapshotRef, participant.CoverageRef = &snapshot, &coverage
		for _, ref := range value.Evidence {
			participant.EvidenceRefs = append(participant.EvidenceRefs, appExactToContinuityV1(ref))
		}
		for _, ref := range value.Residuals {
			participant.ResidualRefs = append(participant.ResidualRefs, appExactToContinuityV1(ref))
		}
		manifest.ParticipantClosures = append(manifest.ParticipantClosures, participant)
	}
	var err error
	manifest.RequiredParticipantSetDigest, err = contract.RequiredParticipantSetDigestV2(manifest.ParticipantClosures)
	if err != nil {
		return contract.CheckpointManifestFactV2{}, err
	}
	manifest.FrozenRefSetDigest, err = contract.FrozenRefSetDigestV2(manifest)
	if err != nil {
		return contract.CheckpointManifestFactV2{}, err
	}
	manifest.Digest, err = manifest.CanonicalDigest()
	if err != nil {
		return contract.CheckpointManifestFactV2{}, err
	}
	return manifest, manifest.Validate()
}

func (a *CheckpointManifestApplicationAdapterV1) buildSealV1(request appcontract.CreateCheckpointManifestSealRequestV1, manifest contract.CheckpointManifestFactV2, created int64) (contract.CheckpointManifestSealFactV2, error) {
	seal := contract.CheckpointManifestSealFactV2{ContractVersion: contract.CheckpointManifestGovernanceContractV2, SchemaRef: contract.CheckpointManifestSealSchemaV2, SealID: request.SealID, Revision: 1, Owner: a.config.SealOwner, TenantID: manifest.Scope.TenantID, ScopeDigest: manifest.Scope.ExecutionScopeDigest, IdempotencyKey: request.IdempotencyKey, ManifestRef: manifest.Ref(), CheckpointAttemptRef: manifest.CheckpointAttemptRef, BarrierRef: manifest.BarrierRef, EffectCutRef: manifest.EffectCutRef, FrozenRefSetDigest: manifest.FrozenRefSetDigest, RequiredParticipantSetDigest: manifest.RequiredParticipantSetDigest, RuntimeParticipantSetDigest: manifest.RuntimeParticipantSetDigest, ParticipantClosures: manifest.Clone().ParticipantClosures, CreatedUnixNano: created}
	var err error
	seal.ContextClosureDigest, err = contract.ContextClosureDigestV2(manifest)
	if err != nil {
		return contract.CheckpointManifestSealFactV2{}, err
	}
	seal.ArtifactClosureDigest, err = contract.ArtifactClosureDigestV2(manifest)
	if err != nil {
		return contract.CheckpointManifestSealFactV2{}, err
	}
	seal.Digest, err = seal.CanonicalDigest()
	if err != nil {
		return contract.CheckpointManifestSealFactV2{}, err
	}
	return seal, seal.Validate()
}

func continuityScopeV1(request appcontract.CreateCheckpointManifestSealRequestV1, digest string) contract.Scope {
	scope := request.Scope
	result := contract.Scope{TenantID: string(scope.Identity.TenantID), IdentityID: string(scope.Identity.ID), IdentityEpoch: uint64(scope.Identity.Epoch), LineageID: string(scope.Lineage.ID), PlanDigest: string(scope.Lineage.PlanDigest), InstanceID: string(scope.Instance.ID), InstanceEpoch: uint64(scope.Instance.Epoch), RunID: string(request.Gate.RunID), RunIdentityDigest: string(request.RunStableIdentityDigest), AuthorityEpoch: uint64(scope.AuthorityEpoch), ExecutionScopeDigest: digest}
	if scope.SandboxLease != nil {
		result.SandboxLeaseID, result.SandboxLeaseEpoch = string(scope.SandboxLease.ID), uint64(scope.SandboxLease.Epoch)
	}
	return result
}

func appExactToContinuityV1(ref appcontract.CheckpointExternalExactRefV1) contract.ExactFactRefV2 {
	return contract.ExactFactRefV2{ContractVersion: ref.ContractVersion, SchemaRef: ref.ExactSchemaRef, Owner: contract.OwnerBinding{BindingSetID: ref.Owner.BindingSetID, BindingRevision: uint64(ref.Owner.BindingSetRevision), ComponentID: string(ref.Owner.ComponentID), ManifestDigest: string(ref.Owner.ManifestDigest), ArtifactDigest: string(ref.Owner.ArtifactDigest), Capability: string(ref.Owner.Capability), FactKind: ref.FactKind}, TenantID: string(ref.TenantID), ID: ref.ID, Revision: uint64(ref.Revision), Digest: string(ref.Digest), ScopeDigest: string(ref.ScopeDigest)}
}

func runtimeAttemptExactV1(ref runtimeports.CheckpointAttemptRefV2, scope string, owner contract.OwnerBinding) contract.ExactFactRefV2 {
	return runtimeOwnedExactV1("praxis.runtime/checkpoint-attempt-fact/v2", "checkpoint_attempt_fact_v2", ref.TenantID, scope, ref.ID, ref.Revision, ref.Digest, owner)
}
func runtimeBarrierExactV1(ref runtimeports.CheckpointBarrierLeaseRefV2, scope string, owner contract.OwnerBinding) contract.ExactFactRefV2 {
	return runtimeOwnedExactV1("praxis.runtime/checkpoint-barrier-lease-fact/v2", "checkpoint_barrier_lease_fact_v2", ref.TenantID, scope, ref.ID, ref.Revision, ref.Digest, owner)
}
func runtimeCutExactV1(ref runtimeports.EffectCutRefV2, scope string, owner contract.OwnerBinding) contract.ExactFactRefV2 {
	return runtimeOwnedExactV1("praxis.runtime/checkpoint-effect-cut-fact/v2", "checkpoint_effect_cut_fact_v2", ref.Attempt.TenantID, scope, ref.ID, ref.Revision, ref.Digest, owner)
}
func runtimeOwnedExactV1(schema, kind string, tenant core.TenantID, scope, id string, revision core.Revision, digest core.Digest, owner contract.OwnerBinding) contract.ExactFactRefV2 {
	owner.FactKind = kind
	return contract.ExactFactRefV2{ContractVersion: runtimeports.CheckpointGovernanceContractVersionV2, SchemaRef: schema, Owner: owner, TenantID: string(tenant), ID: id, Revision: uint64(revision), Digest: string(digest), ScopeDigest: scope}
}

func runtimeSealRefV1(seal contract.CheckpointManifestSealFactV2, request appcontract.CreateCheckpointManifestSealRequestV1) (runtimeports.CheckpointManifestSealRefV2, error) {
	exact := continuityExactToRuntimeV1(seal.Ref().Exact())
	digest, err := runtimeports.NormalizeCheckpointExternalSHA256DigestV2(seal.Digest)
	if err != nil {
		return runtimeports.CheckpointManifestSealRefV2{}, err
	}
	manifestDigest, err := runtimeports.NormalizeCheckpointExternalSHA256DigestV2(seal.ManifestRef.Exact().Digest)
	if err != nil {
		return runtimeports.CheckpointManifestSealRefV2{}, err
	}
	frozen, err := runtimeports.NormalizeCheckpointExternalSHA256DigestV2(seal.FrozenRefSetDigest)
	if err != nil {
		return runtimeports.CheckpointManifestSealRefV2{}, err
	}
	result := runtimeports.CheckpointManifestSealRefV2{ExactLookup: exact, ID: seal.SealID, Revision: 1, Digest: digest, ManifestID: seal.ManifestRef.Exact().ID, ManifestRevision: core.Revision(seal.ManifestRef.Exact().Revision), ManifestDigest: manifestDigest, Attempt: request.Attempt, Barrier: request.Barrier, EffectCut: request.EffectCut, FrozenRefSetDigest: frozen}
	return result, result.Validate()
}

func continuityExactToRuntimeV1(ref contract.ExactFactRefV2) runtimeports.CheckpointExternalExactFactRefV2 {
	return runtimeports.CheckpointExternalExactFactRefV2{ContractVersion: ref.ContractVersion, SchemaRef: ref.SchemaRef, Owner: runtimeports.CheckpointManifestSealOwnerBindingV2{BindingSetID: ref.Owner.BindingSetID, BindingRevision: core.Revision(ref.Owner.BindingRevision), ComponentID: ref.Owner.ComponentID, ManifestDigest: ref.Owner.ManifestDigest, ArtifactDigest: ref.Owner.ArtifactDigest, Capability: ref.Owner.Capability, FactKind: ref.Owner.FactKind}, TenantID: ref.TenantID, ID: ref.ID, Revision: core.Revision(ref.Revision), Digest: ref.Digest, ScopeDigest: ref.ScopeDigest}
}
func runtimeExactToContinuityV1(ref runtimeports.CheckpointExternalExactFactRefV2) contract.ExactFactRefV2 {
	return contract.ExactFactRefV2{ContractVersion: ref.ContractVersion, SchemaRef: ref.SchemaRef, Owner: contract.OwnerBinding{BindingSetID: ref.Owner.BindingSetID, BindingRevision: uint64(ref.Owner.BindingRevision), ComponentID: ref.Owner.ComponentID, ManifestDigest: ref.Owner.ManifestDigest, ArtifactDigest: ref.Owner.ArtifactDigest, Capability: ref.Owner.Capability, FactKind: ref.Owner.FactKind}, TenantID: ref.TenantID, ID: ref.ID, Revision: uint64(ref.Revision), Digest: ref.Digest, ScopeDigest: ref.ScopeDigest}
}
func maxCheckpointTimeV1(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
func checkpointAppNilV1(value any) bool {
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
func checkpointAppErrorV1(err error) error {
	switch {
	case contract.HasCode(err, contract.ErrNotFound):
		return core.NewError(core.ErrorNotFound, core.ReasonCheckpointInconsistent, err.Error())
	case contract.HasCode(err, contract.ErrUnavailable):
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, err.Error())
	case contract.HasCode(err, contract.ErrIndeterminate), contract.HasCode(err, contract.ErrCheckpointIndeterminate):
		return core.NewError(core.ErrorIndeterminate, core.ReasonCheckpointInconsistent, err.Error())
	case contract.HasCode(err, contract.ErrRevisionConflict), contract.HasCode(err, contract.ErrProjectionConflict), contract.HasCode(err, contract.ErrCheckpointPartial), contract.HasCode(err, contract.ErrPreconditionFailed):
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, err.Error())
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCheckpointInconsistent, err.Error())
	}
}

func checkpointDefinitiveContinuityErrorV1(err error) bool {
	return contract.HasCode(err, contract.ErrInvalidArgument) || contract.HasCode(err, contract.ErrRevisionConflict) || contract.HasCode(err, contract.ErrProjectionConflict) || contract.HasCode(err, contract.ErrEvidenceConflict) || contract.HasCode(err, contract.ErrPreconditionFailed) || contract.HasCode(err, contract.ErrCheckpointPartial)
}

var _ applicationports.CheckpointManifestPortV1 = (*CheckpointManifestApplicationAdapterV1)(nil)

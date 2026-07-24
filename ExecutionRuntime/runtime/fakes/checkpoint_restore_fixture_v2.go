package fakes

import (
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// BuildRestorePlanCurrentFixtureV2 builds a structurally valid Restore Plan
// current projection for reference tests. It creates no Runtime, Continuity,
// Sandbox, Provider, or external-world state.
func BuildRestorePlanCurrentFixtureV2(suffix string, now time.Time) (ports.RestorePlanCurrentProjectionV2, error) {
	tenant := core.TenantID("tenant-restore-fixture")
	scope := core.ExecutionScope{
		Identity:       core.AgentIdentityRef{TenantID: tenant, ID: "identity-restore-fixture", Epoch: 1},
		Lineage:        core.LineageRef{ID: "lineage-restore-fixture", PlanDigest: restoreFixtureDigestV2("lineage")},
		Instance:       core.InstanceRef{ID: "source-instance-" + core.AgentInstanceID(suffix), Epoch: 1},
		AuthorityEpoch: 1,
	}
	scopeDigest, err := ports.ExecutionScopeDigestV2(scope)
	if err != nil {
		return ports.RestorePlanCurrentProjectionV2{}, err
	}
	attempt := ports.CheckpointAttemptRefV2{TenantID: tenant, ID: "checkpoint-attempt-" + suffix, Revision: 1, Digest: restoreFixtureDigestV2("attempt-" + suffix)}
	barrier := ports.CheckpointBarrierLeaseRefV2{TenantID: tenant, ID: "checkpoint-barrier-" + suffix, AttemptID: attempt.ID, Revision: 1, Digest: restoreFixtureDigestV2("barrier-" + suffix), ExpiresUnixNano: now.Add(20 * time.Minute).UnixNano()}
	cut := ports.EffectCutRefV2{ID: "checkpoint-cut-" + suffix, Revision: 1, Attempt: attempt, RootDigest: restoreFixtureDigestV2("effect-root-" + suffix), Watermark: 1, Digest: restoreFixtureDigestV2("cut-" + suffix)}
	participant := ports.CheckpointParticipantRefV2{
		ID:     "checkpoint-participant-" + suffix,
		Owner:  ports.ProviderBindingRefV2{BindingSetID: "binding-set-" + suffix, BindingSetRevision: 1, ComponentID: "praxis/continuity", ManifestDigest: restoreFixtureDigestV2("participant-manifest-" + suffix), ArtifactDigest: restoreFixtureDigestV2("participant-artifact-" + suffix), Capability: "checkpoint/participant"},
		Digest: restoreFixtureDigestV2("participant-" + suffix),
	}
	closure, _, err := BuildCommittedCheckpointParticipantClosureV2(scope, "run-restore-fixture", attempt, barrier, cut, participant, suffix, now)
	if err != nil {
		return ports.RestorePlanCurrentProjectionV2{}, err
	}
	participantSetDigest := restoreFixtureDigestV2("participant-set-" + suffix)
	frozenRefSetDigest := restoreFixtureDigestV2("frozen-ref-set-" + suffix)
	sealDigest := restoreFixtureDigestV2("manifest-seal-" + suffix)
	exactSeal := restoreFixtureExternalRefV2(tenant, scopeDigest, "checkpoint-seal-"+suffix, ports.CheckpointManifestSealOwnerFactKindV2)
	exactSeal.ContractVersion = ports.CheckpointManifestSealOwnerContractV2
	exactSeal.SchemaRef = ports.CheckpointManifestSealExactSchemaV2
	exactSeal.Owner.ComponentID = ports.CheckpointManifestSealOwnerComponentV2
	exactSeal.Owner.Capability = ports.CheckpointManifestSealOwnerCapabilityV2
	exactSeal.Owner.FactKind = ports.CheckpointManifestSealOwnerFactKindV2
	exactSeal.Digest = strings.TrimPrefix(string(sealDigest), "sha256:")
	seal := ports.CheckpointManifestSealRefV2{
		ExactLookup: exactSeal, ID: exactSeal.ID, Revision: 1, Digest: sealDigest,
		ManifestID: "checkpoint-manifest-" + suffix, ManifestRevision: 1, ManifestDigest: restoreFixtureDigestV2("manifest-" + suffix),
		Attempt: attempt, Barrier: barrier, EffectCut: cut, FrozenRefSetDigest: frozenRefSetDigest,
	}
	if err := seal.Validate(); err != nil {
		return ports.RestorePlanCurrentProjectionV2{}, err
	}
	consistency, err := ports.SealCheckpointConsistencyFactV2(ports.CheckpointConsistencyFactV2{
		Ref:     ports.CheckpointConsistencyRefV2{ID: "checkpoint-consistency-" + suffix, Revision: 1, Attempt: attempt},
		Barrier: barrier, EffectCut: cut, ManifestSeal: seal, ParticipantClosures: []ports.CheckpointParticipantClosureRefV2{closure},
		ParticipantSetDigest: participantSetDigest, ParticipantRootDigest: restoreFixtureDigestV2("participant-root-" + suffix),
		ParticipantWatermark: 1, ParticipantCount: 1, FrozenRefSetDigest: frozenRefSetDigest, CreatedUnixNano: now.UnixNano(),
	})
	if err != nil {
		return ports.RestorePlanCurrentProjectionV2{}, err
	}
	planRef := restoreFixtureExternalRefV2(tenant, scopeDigest, "restore-plan-"+suffix, "restore-plan-fact")
	return ports.SealRestorePlanCurrentProjectionV2(ports.RestorePlanCurrentProjectionV2{
		RestorePlan: planRef, State: ports.RestorePlanSubmittedStateV2, CheckpointConsistency: consistency, ManifestSeal: seal,
		SourceScopeDigest: scopeDigest,
		IdentityProposal: ports.RestoreIdentityReservationV2{
			SourceInstance:   scope.Instance,
			TargetInstance:   core.InstanceRef{ID: "target-instance-" + core.AgentInstanceID(suffix), Epoch: 2},
			TargetLease:      core.SandboxLeaseRef{ID: "target-lease-" + core.SandboxLeaseID(suffix), Epoch: 2},
			TargetFenceEpoch: 2,
		},
		ConflictDomain:               "tenant/" + string(tenant) + "/restore-" + suffix,
		RequiredParticipantSetDigest: participantSetDigest,
		CheckedUnixNano:              now.UnixNano(), ExpiresUnixNano: now.Add(10 * time.Minute).UnixNano(),
	}, now)
}

func restoreFixtureExternalRefV2(tenant core.TenantID, scope core.Digest, id, kind string) ports.CheckpointExternalExactFactRefV2 {
	digest := restoreFixtureDigestV2(id + "-" + kind)
	return ports.CheckpointExternalExactFactRefV2{
		ContractVersion: "praxis.test/" + kind + "/v1", SchemaRef: "praxis.test/" + kind + "-schema/v1",
		Owner:    ports.CheckpointManifestSealOwnerBindingV2{BindingSetID: "binding-set-" + kind, BindingRevision: 1, ComponentID: "praxis/" + kind, ManifestDigest: string(restoreFixtureDigestV2("manifest-" + kind)), ArtifactDigest: string(restoreFixtureDigestV2("artifact-" + kind)), Capability: kind + "-current", FactKind: kind},
		TenantID: string(tenant), ID: id, Revision: 1, Digest: string(digest), ScopeDigest: string(scope),
	}
}

func restoreFixtureDigestV2(value string) core.Digest { return core.DigestBytes([]byte(value)) }

package contract

import (
	"testing"
	"time"
)

func TestWorkspaceCheckpointPreparedBundleCanonicalAndExact(t *testing.T) {
	bundle := workspaceCheckpointBundleFixtureV2(t, "canonical")
	if err := bundle.ValidateShape(); err != nil {
		t.Fatal(err)
	}
	copy := bundle.Clone()
	copy.Coverage.Included[0] = "workspace/other"
	if copy.Coverage.ValidateShape() == nil {
		t.Fatal("coverage content drift retained canonical digest")
	}
	if bundle.Coverage.Included[0] != "workspace/content" {
		t.Fatal("fixture alias changed original coverage")
	}
}

func TestWorkspaceCheckpointCompleteRejectsResidualAndCrossScopeSplice(t *testing.T) {
	bundle := workspaceCheckpointBundleFixtureV2(t, "negative")
	withResidual := bundle.Coverage
	withResidual.ResidualRefs = []Ref{workspaceCheckpointRefV2("residual")}
	if _, err := SealWorkspaceCheckpointCoverageFactV2(withResidual); err == nil {
		t.Fatal("complete coverage accepted an unresolved residual")
	}
	splice := bundle
	splice.Participant.ScopeDigest = workspaceCheckpointDigestV2("other-scope")
	splice.Participant, _ = SealWorkspaceCheckpointParticipantFactV2(splice.Participant)
	if splice.ValidateShape() == nil {
		t.Fatal("cross-scope coverage/participant splice was accepted")
	}
}

func TestWorkspaceCheckpointTTLBoundaryIsExclusive(t *testing.T) {
	bundle := workspaceCheckpointBundleFixtureV2(t, "ttl")
	now := time.Unix(0, bundle.Participant.Meta.ExpiresUnixNano)
	if bundle.Participant.ValidateCurrent(now) == nil || bundle.Coverage.ValidateCurrent(now) == nil {
		t.Fatal("workspace checkpoint facts remained current at expiry")
	}
}

func workspaceCheckpointBundleFixtureV2(t *testing.T, suffix string) WorkspaceCheckpointPreparedBundleV2 {
	t.Helper()
	now := time.Unix(1_900_400_000, 0).UTC()
	expires := now.Add(time.Minute)
	artifactRef := SnapshotArtifactExactRefV2{TypeURL: SnapshotArtifactFactTypeURL, Version: SnapshotArtifactVersion, ID: "snapshot-artifact-" + suffix, Revision: 1, DigestAlgorithm: SnapshotArtifactDigestSHA256, DigestDomain: SnapshotArtifactFactDomain, Digest: workspaceCheckpointDigestV2("snapshot-artifact-" + suffix), ExpiresUnixNano: expires.UnixNano()}
	aggregateRef := workspaceCheckpointAggregateRefV2("tenant-"+suffix, suffix, expires.UnixNano())
	coverage := WorkspaceCheckpointCoverageFactV2{
		Meta:     Meta{ContractVersion: ContractFamily, ID: "workspace-coverage-" + suffix, Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano()},
		TenantID: "tenant-" + suffix, ScopeDigest: workspaceCheckpointDigestV2("scope-" + suffix), RunID: "run-" + suffix,
		CheckpointAttemptRef: workspaceCheckpointRefV2("attempt-" + suffix), BarrierRef: workspaceCheckpointRefV2("barrier-" + suffix), EffectCutRef: workspaceCheckpointRefV2("cut-" + suffix),
		ParticipantID: "participant-" + suffix, ParticipantDigest: workspaceCheckpointDigestV2("participant-" + suffix), PreparedPhaseFactRef: workspaceCheckpointRefV2("prepared-" + suffix),
		SnapshotArtifactFactRef: artifactRef, SnapshotAggregateRef: aggregateRef, CoveragePolicyRef: workspaceCheckpointRefV2("coverage-policy-" + suffix),
		Included: []string{"workspace/metadata", "workspace/content"}, DeclaredExcluded: []string{"device_state", "network_session", "process_state", "secret_material"}, ResidualRefs: []Ref{},
		State: WorkspaceCheckpointCoverageComplete, RequestedNotAfter: expires.UnixNano(),
	}
	var err error
	coverage, err = SealWorkspaceCheckpointCoverageFactV2(coverage)
	if err != nil {
		t.Fatal(err)
	}
	participant := WorkspaceCheckpointParticipantFactV2{
		Meta:     Meta{ContractVersion: ContractFamily, ID: "workspace-participant-" + suffix, Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano()},
		TenantID: coverage.TenantID, ScopeDigest: coverage.ScopeDigest, RunID: coverage.RunID, CheckpointAttemptRef: coverage.CheckpointAttemptRef, BarrierRef: coverage.BarrierRef, EffectCutRef: coverage.EffectCutRef,
		ParticipantID: coverage.ParticipantID, ParticipantDigest: coverage.ParticipantDigest, PreparedPhaseFactRef: coverage.PreparedPhaseFactRef,
		SnapshotArtifactFactRef: artifactRef, SnapshotAggregateRef: aggregateRef, CoverageFactRef: coverage.ExactRef(), State: WorkspaceCheckpointParticipantPrepared, RequestedNotAfter: expires.UnixNano(),
	}
	participant, err = SealWorkspaceCheckpointParticipantFactV2(participant)
	if err != nil {
		t.Fatal(err)
	}
	return WorkspaceCheckpointPreparedBundleV2{Coverage: coverage, Participant: participant}
}

func workspaceCheckpointRefV2(value string) Ref {
	return Ref{ID: value, Revision: 1, Digest: workspaceCheckpointDigestV2(value)}
}

func workspaceCheckpointDigestV2(value string) string {
	digest, err := Digest("workspace-checkpoint-test", value)
	if err != nil {
		panic(err)
	}
	return digest
}

func workspaceCheckpointAggregateRefV2(tenantID, suffix string, expires int64) SnapshotArtifactAggregateRefV2 {
	return SnapshotArtifactAggregateRefV2{
		TypeURL: SnapshotArtifactAggregateRefTypeURL, Version: SnapshotArtifactVersion, Owner: SnapshotArtifactOwnerV2, Kind: SnapshotArtifactAggregateKind,
		AggregateID: "snapshot-aggregate-" + suffix, Revision: 2, TenantID: tenantID, DataDomain: WorkspaceSnapshotDataDomain,
		SchemaRef: workspaceCheckpointRefV2("snapshot-schema-" + suffix), DigestAlgorithm: SnapshotArtifactDigestSHA256, DigestDomain: SnapshotArtifactAggregateRefDomain,
		Digest: workspaceCheckpointDigestV2("snapshot-aggregate-" + suffix), ExpiresUnixNano: expires,
	}
}

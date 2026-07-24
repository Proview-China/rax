package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
)

func TestObservationProjectionDoesNotRequireOwnerFactOrUpgradeTrust(t *testing.T) {
	candidate := testkit.Candidate(1, 1, contract.TrustObservation)
	if candidate.OwnerFactRef != nil {
		t.Fatal("observation fixture unexpectedly has owner fact")
	}
	if err := candidate.Validate(); err != nil {
		t.Fatalf("valid admitted observation rejected: %v", err)
	}
	unadmitted := candidate
	unadmitted.Evidence.AdmittedByLedger = false
	unadmitted.Digest, _ = unadmitted.CanonicalDigest()
	if err := unadmitted.Validate(); !contract.HasCode(err, contract.ErrEvidenceNotInspectable) {
		t.Fatalf("unadmitted observation should fail, got %v", err)
	}
	candidate.Evidence.TrustClass = contract.TrustAuthoritativeFact
	digest, _ := candidate.CanonicalDigest()
	candidate.Digest = digest
	if err := candidate.Validate(); !contract.HasCode(err, contract.ErrEvidenceNotInspectable) {
		t.Fatalf("authoritative candidate without owner fact should fail, got %v", err)
	}
}

func TestCandidateCanonicalDigestNormalizesNilAndEmptySets(t *testing.T) {
	a := testkit.Candidate(1, 1, contract.TrustObservation)
	a.ObjectRefs = nil
	a.ParentRefs = nil
	a.CausationRefs = nil
	b := a.Clone()
	b.ObjectRefs = []string{}
	b.ParentRefs = []string{}
	b.CausationRefs = []string{}
	da, err := a.CanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	db, err := b.CanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	if da != db {
		t.Fatalf("nil/empty drift: %s != %s", da, db)
	}
}

func TestCursorRejectsTamperAndAuthorityDrift(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	query := contract.TimelineQuery{
		LedgerScopeDigest: "ledger-scope-1", AuthorityWatermark: "authority-1",
		PolicyWatermark: "policy-1", PageLimit: 10,
	}
	queryDigest, _ := query.Digest()
	cursor := contract.TimelineCursor{
		LedgerScopeDigest: query.LedgerScopeDigest, QueryDigest: queryDigest,
		AuthorityWatermark: query.AuthorityWatermark, PolicyWatermark: query.PolicyWatermark,
		ProjectionSchema: contract.ProjectionSchema, PageLimit: query.PageLimit,
		IssuedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(), State: "active",
	}
	token, err := cursor.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := contract.DecodeTimelineCursor(token + "x"); !contract.HasCode(err, contract.ErrCursorInvalidated) {
		t.Fatalf("tampered token should be invalidated, got %v", err)
	}
	decoded, err := contract.DecodeTimelineCursor(token)
	if err != nil {
		t.Fatal(err)
	}
	query.AuthorityWatermark = "authority-2"
	if err := decoded.ValidateFor(query, now); !contract.HasCode(err, contract.ErrCursorInvalidated) {
		t.Fatalf("authority drift should invalidate cursor, got %v", err)
	}
	query.AuthorityWatermark = "authority-1"
	if err := decoded.ValidateFor(query, now.Add(2*time.Minute)); !contract.HasCode(err, contract.ErrCursorExpired) {
		t.Fatalf("expired cursor should fail, got %v", err)
	}
}

func TestCheckpointManifestClassification(t *testing.T) {
	manifest := validManifest(t)
	state, err := contract.ValidateCheckpointManifest(manifest)
	if err != nil || state != contract.ManifestVerifiedCandidate {
		t.Fatalf("verified manifest: state=%s err=%v", state, err)
	}
	partial := manifest
	partial.Participants = append([]contract.SnapshotBinding{}, manifest.Participants...)
	partial.Participants[0].State = contract.ParticipantPartial
	partial.Digest = manifestDigest(t, partial)
	state, err = contract.ValidateCheckpointManifest(partial)
	if err != nil || state != contract.ManifestDiagnosticPartial {
		t.Fatalf("partial manifest: state=%s err=%v", state, err)
	}
	unknown := manifest
	unknown.Participants = append([]contract.SnapshotBinding{}, manifest.Participants...)
	unknown.Participants[0].State = contract.ParticipantUnknown
	unknown.Digest = manifestDigest(t, unknown)
	state, err = contract.ValidateCheckpointManifest(unknown)
	if err != nil || state != contract.ManifestDiagnosticIndeterminate {
		t.Fatalf("unknown manifest: state=%s err=%v", state, err)
	}
	receiptOnly := manifest
	receiptOnly.Participants = append([]contract.SnapshotBinding{}, manifest.Participants...)
	receiptOnly.Participants[0].InspectFactRef = ""
	receiptOnly.Digest = manifestDigest(t, receiptOnly)
	if _, err := contract.ValidateCheckpointManifest(receiptOnly); err == nil {
		t.Fatal("provider receipt without owner inspect fact was accepted")
	}
}

func TestPlanValidationNewInstanceAndNoConflictedRewind(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	fork := contract.ForkPlan{
		PlanID: "fork-1", SourceNodeRef: "event-1", ParentLineageID: "lineage-1",
		NewLineageID: "lineage-2", NewSessionIntent: "review-session",
		ContextGeneration: "context-generation-2", AuthorityCeilingDigest: "authority-ceiling",
		RequiredRevalidations: []string{"binding", "tool", "sandbox"},
		ExpiresUnixNano:       now.Add(time.Hour).UnixNano(),
	}
	fork.Digest = digestFork(t, fork)
	if err := fork.Validate(now); err != nil {
		t.Fatalf("valid fork plan rejected: %v", err)
	}
	fork.NewLineageID = fork.ParentLineageID
	fork.Digest = digestFork(t, fork)
	if err := fork.Validate(now); err == nil {
		t.Fatal("fork reusing parent lineage was accepted")
	}
	restore := contract.RestorePlan{
		PlanID: "restore-1", RuntimeCheckpointFactRef: "runtime-checkpoint-1",
		ContinuityManifestRef: "manifest-1", SourceInstanceID: "instance-1",
		SourceInstanceEpoch: 3, NewInstanceID: "instance-2", NewInstanceEpoch: 4,
		NewSandboxLeaseRef: "lease-2", RequiredParticipantIDs: []string{"sandbox"},
		CompatibilityFactRefs: []string{"binding-current"}, ContextMaterialized: true,
		RecoveryCredentialRef: "credential-1", ExpiresUnixNano: now.Add(time.Hour).UnixNano(),
	}
	restore.Digest = digestRestore(t, restore)
	if err := restore.Validate(now); err != nil {
		t.Fatalf("valid restore plan rejected: %v", err)
	}
	restore.NewInstanceID = restore.SourceInstanceID
	restore.Digest = digestRestore(t, restore)
	if err := restore.Validate(now); !contract.HasCode(err, contract.ErrRestoreIncompatible) {
		t.Fatalf("same instance restore should fail, got %v", err)
	}
	rewind := contract.RewindPlan{
		PlanID: "rewind-1", TargetCheckpointRef: "checkpoint-1",
		KeepChangeSetRefs: []string{"change-1"}, DropChangeSetRefs: []string{"change-2"},
		DependencyConflictRefs: []string{"conflict-1"}, Approved: true,
		ExpiresUnixNano: now.Add(time.Hour).UnixNano(),
	}
	rewind.Digest = digestRewind(t, rewind)
	if err := rewind.Validate(now); err == nil {
		t.Fatal("approved conflicted rewind was accepted")
	}
}

func TestRetentionHoldBlocksMutationUntilExplicitRelease(t *testing.T) {
	fact := contract.RetentionFact{
		ObjectID: "object-1", PolicyRef: "policy-1", Classification: "sensitive",
		State: contract.RetentionActive, Revision: 1, UpdatedUnixNano: 1,
	}
	held, err := contract.AdvanceRetention(fact, contract.RetentionLegalHold, "hold-fact-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := contract.AdvanceRetention(held, contract.RetentionTombstoned, "tombstone-1"); !contract.HasCode(err, contract.ErrRetentionBlocked) {
		t.Fatalf("hold should block tombstone, got %v", err)
	}
	released, err := contract.AdvanceRetention(held, contract.RetentionActive, "release-fact-1")
	if err != nil || released.HoldRef != "" {
		t.Fatalf("explicit release failed: %#v %v", released, err)
	}
	tombstoned, err := contract.AdvanceRetention(fact, contract.RetentionTombstoned, "tombstone-1")
	if err != nil {
		t.Fatal(err)
	}
	heldTombstone, err := contract.AdvanceRetention(tombstoned, contract.RetentionLegalHold, "hold-fact-2")
	if err != nil {
		t.Fatal(err)
	}
	restoredTombstone, err := contract.AdvanceRetention(heldTombstone, contract.RetentionTombstoned, "release-fact-2")
	if err != nil || restoredTombstone.State != contract.RetentionTombstoned {
		t.Fatalf("hold release changed prior visibility: %#v %v", restoredTombstone, err)
	}
}

func validManifest(t *testing.T) contract.CheckpointManifest {
	t.Helper()
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC).UnixNano()
	m := contract.CheckpointManifest{
		ContractVersion: contract.ContractVersion, CheckpointID: "checkpoint-1", Epoch: 1,
		BarrierID: "barrier-1", BarrierRevision: 1, Scope: testkit.Scope(),
		RuntimeStateRef: "runtime-state-1", RunSessionRef: "session-1",
		PlanDigest: "plan-digest", ProfileDigest: "profile-digest", BindingDigest: "binding-digest",
		ContextGeneration: "context-generation-1", ContextMaterialized: true,
		ToolSurfaceDigest: "tool-surface-digest", AuthorityDigest: "authority-digest",
		EvidenceWatermark: 10, EffectCutRef: "effect-cut-1", EffectCutAccepted: true,
		Participants: []contract.SnapshotBinding{{
			ParticipantID: "sandbox", Required: true, State: contract.ParticipantPrepared,
			SnapshotRef: "snapshot-1", SnapshotRevision: 1, SnapshotDigest: "snapshot-digest",
			CoverageSchema: "coverage/v1", CoverageDigest: "coverage-digest", StorageRef: "storage-1",
			EvidenceRef: "evidence-1", InspectFactRef: "participant-fact-1",
		}},
		Revision: 1, CreatedUnixNano: now,
	}
	m.Digest = manifestDigest(t, m)
	return m
}

func manifestDigest(t *testing.T, m contract.CheckpointManifest) string {
	t.Helper()
	digest, err := m.CanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func digestRestore(t *testing.T, p contract.RestorePlan) string {
	t.Helper()
	digest, err := p.CanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func digestRewind(t *testing.T, p contract.RewindPlan) string {
	t.Helper()
	digest, err := p.CanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func digestFork(t *testing.T, p contract.ForkPlan) string {
	t.Helper()
	digest, err := p.CanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

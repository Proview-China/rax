package contract_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
)

func TestQualityTimelineCandidateRejectsCanonicalTamper(t *testing.T) {
	base := testkit.Candidate(7, 7, contract.TrustObservation)
	tests := map[string]func(*contract.TimelineProjectionCandidate){
		"ledger sequence":  func(c *contract.TimelineProjectionCandidate) { c.Evidence.LedgerSequence++ },
		"record digest":    func(c *contract.TimelineProjectionCandidate) { c.Evidence.RecordDigest = "changed" },
		"payload digest":   func(c *contract.TimelineProjectionCandidate) { c.Evidence.PayloadDigest = "changed" },
		"instance epoch":   func(c *contract.TimelineProjectionCandidate) { c.Scope.InstanceEpoch++ },
		"binding revision": func(c *contract.TimelineProjectionCandidate) { c.Owner.BindingRevision++ },
		"semantic kind":    func(c *contract.TimelineProjectionCandidate) { c.SemanticKind = "praxis/control" },
		"object refs":      func(c *contract.TimelineProjectionCandidate) { c.ObjectRefs = append(c.ObjectRefs, "object-2") },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := base.Clone()
			mutate(&candidate)
			if err := candidate.Validate(); !contract.HasCode(err, contract.ErrProjectionConflict) {
				t.Fatalf("stale canonical digest accepted: %v", err)
			}
		})
	}
}

func TestQualityTimelineRecordRejectsEvidenceMirrorTamper(t *testing.T) {
	candidate := testkit.Candidate(7, 7, contract.TrustObservation)
	base := contract.TimelineEventRecord{
		Candidate: candidate, EvidenceRecordRef: candidate.Evidence.RecordRef,
		LedgerScopeDigest:    candidate.Evidence.LedgerScopeDigest,
		LedgerSequence:       candidate.Evidence.LedgerSequence,
		EvidenceRecordDigest: candidate.Evidence.RecordDigest,
		TrustClass:           candidate.Evidence.TrustClass, ProjectionRevision: 1, Visibility: "visible",
	}
	tests := map[string]func(*contract.TimelineEventRecord){
		"record ref":      func(r *contract.TimelineEventRecord) { r.EvidenceRecordRef = "evidence-other" },
		"ledger scope":    func(r *contract.TimelineEventRecord) { r.LedgerScopeDigest = "scope-other" },
		"ledger sequence": func(r *contract.TimelineEventRecord) { r.LedgerSequence++ },
		"record digest":   func(r *contract.TimelineEventRecord) { r.EvidenceRecordDigest = "changed" },
		"trust class":     func(r *contract.TimelineEventRecord) { r.TrustClass = contract.TrustAuthoritativeFact },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			record := base.Clone()
			mutate(&record)
			if err := record.Validate(); !contract.HasCode(err, contract.ErrProjectionConflict) {
				t.Fatalf("evidence-owned mirror tamper accepted: %v", err)
			}
		})
	}
}

func TestQualityCursorRejectsNonCanonicalDigestCase(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	query := contract.TimelineQuery{
		LedgerScopeDigest: "ledger-scope-1", AuthorityWatermark: "authority-1",
		PolicyWatermark: "policy-1", PageLimit: 10,
	}
	queryDigest, _ := query.Digest()
	token, err := (contract.TimelineCursor{
		LedgerScopeDigest: query.LedgerScopeDigest, QueryDigest: queryDigest,
		AuthorityWatermark: query.AuthorityWatermark, PolicyWatermark: query.PolicyWatermark,
		ProjectionSchema: contract.ProjectionSchema, PageLimit: query.PageLimit,
		IssuedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(), State: "active",
	}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := base64.RawURLEncoding.DecodeString(token)
	var wire contract.TimelineCursor
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	wire.Digest = strings.ToUpper(wire.Digest)
	raw, _ = json.Marshal(wire)
	tampered := base64.RawURLEncoding.EncodeToString(raw)
	if _, err := contract.DecodeTimelineCursor(tampered); !contract.HasCode(err, contract.ErrCursorInvalidated) {
		t.Fatalf("noncanonical digest case accepted: %v", err)
	}
}

func TestQualityCursorRejectsNonCanonicalWireBytes(t *testing.T) {
	now := time.Date(2026, 7, 17, 6, 0, 0, 0, time.UTC)
	query := contract.TimelineQuery{
		LedgerScopeDigest: "ledger-scope-1", AuthorityWatermark: "authority-1",
		PolicyWatermark: "policy-1", PageLimit: 10,
	}
	queryDigest, _ := query.Digest()
	token, err := (contract.TimelineCursor{
		LedgerScopeDigest: query.LedgerScopeDigest, QueryDigest: queryDigest,
		AuthorityWatermark: query.AuthorityWatermark, PolicyWatermark: query.PolicyWatermark,
		ProjectionSchema: contract.ProjectionSchema, PageLimit: query.PageLimit,
		IssuedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(), State: "active",
	}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatal(err)
	}
	var indented bytes.Buffer
	if err := json.Indent(&indented, raw, "", "  "); err != nil {
		t.Fatal(err)
	}
	nonCanonical := base64.RawURLEncoding.EncodeToString([]byte(indented.String()))
	if _, err := contract.DecodeTimelineCursor(nonCanonical); !contract.HasCode(err, contract.ErrCursorInvalidated) {
		t.Fatalf("noncanonical cursor wire bytes accepted: %v", err)
	}
}

func TestQualityPlanCanonicalDigestIgnoresSetOrdering(t *testing.T) {
	forkA := contract.ForkPlan{
		PlanID: "fork-1", SourceNodeRef: "event-1", ParentLineageID: "lineage-1",
		NewLineageID: "lineage-2", NewSessionIntent: "session-2", ContextGeneration: "context-2",
		AuthorityCeilingDigest: "authority-1", RequiredRevalidations: []string{"tool", "binding", "sandbox"},
		InheritedEffectRefs: []string{"effect-2", "effect-1"}, ExpiresUnixNano: 10,
	}
	forkB := forkA
	forkB.RequiredRevalidations = []string{"sandbox", "tool", "binding"}
	forkB.InheritedEffectRefs = []string{"effect-1", "effect-2"}
	assertSameDigest(t, forkA.CanonicalDigest, forkB.CanonicalDigest)

	rewindA := contract.RewindPlan{
		PlanID: "rewind-1", TargetCheckpointRef: "checkpoint-1",
		KeepChangeSetRefs: []string{"change-2", "change-1"}, DropChangeSetRefs: []string{"change-4", "change-3"},
		IrreversibleEffectRefs: []string{"effect-2", "effect-1"}, RequiredReviewRefs: []string{"review-2", "review-1"},
		ExpiresUnixNano: 10,
	}
	rewindB := rewindA
	rewindB.KeepChangeSetRefs = []string{"change-1", "change-2"}
	rewindB.DropChangeSetRefs = []string{"change-3", "change-4"}
	rewindB.IrreversibleEffectRefs = []string{"effect-1", "effect-2"}
	rewindB.RequiredReviewRefs = []string{"review-1", "review-2"}
	assertSameDigest(t, rewindA.CanonicalDigest, rewindB.CanonicalDigest)

	restoreA := contract.RestorePlan{
		PlanID: "restore-1", RuntimeCheckpointFactRef: "runtime-checkpoint-1",
		ContinuityManifestRef: "manifest-1", SourceInstanceID: "instance-1", SourceInstanceEpoch: 1,
		NewInstanceID: "instance-2", NewInstanceEpoch: 2, NewSandboxLeaseRef: "lease-2",
		RequiredParticipantIDs: []string{"sandbox", "context"}, CompatibilityFactRefs: []string{"profile", "binding"},
		ContextMaterialized: true, RecoveryCredentialRef: "credential-1", ExpiresUnixNano: 10,
	}
	restoreB := restoreA
	restoreB.RequiredParticipantIDs = []string{"context", "sandbox"}
	restoreB.CompatibilityFactRefs = []string{"binding", "profile"}
	assertSameDigest(t, restoreA.CanonicalDigest, restoreB.CanonicalDigest)
}

func TestQualityRewindApprovedTTLCurrentnessAndReplay(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	plan := contract.RewindPlan{
		PlanID: "rewind-approved-1", TargetCheckpointRef: "checkpoint-1",
		KeepChangeSetRefs: []string{"change-1"}, DropChangeSetRefs: []string{"change-2"},
		RequiredReviewRefs: []string{"review-1"}, Approved: true,
		ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	}
	plan.Digest, _ = plan.CanonicalDigest()

	if err := plan.Validate(time.Unix(0, plan.ExpiresUnixNano-1)); err != nil {
		t.Fatalf("approved plan rejected one nanosecond before expiry: %v", err)
	}
	if err := plan.Validate(time.Unix(0, plan.ExpiresUnixNano)); !contract.HasCode(err, contract.ErrInvalidArgument) {
		t.Fatalf("approved plan accepted at exact expiry boundary: %v", err)
	}
	if err := plan.Validate(time.Unix(0, plan.ExpiresUnixNano+1)); !contract.HasCode(err, contract.ErrInvalidArgument) {
		t.Fatalf("expired approved plan replay was accepted: %v", err)
	}
}

func TestQualityRewindExpiryExtensionRequiresNewCanonicalDigest(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	plan := contract.RewindPlan{
		PlanID: "rewind-approved-1", TargetCheckpointRef: "checkpoint-1",
		KeepChangeSetRefs: []string{"change-1"}, DropChangeSetRefs: []string{"change-2"},
		Approved: true, ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	}
	plan.Digest, _ = plan.CanonicalDigest()
	plan.ExpiresUnixNano = now.Add(time.Hour).UnixNano()
	if err := plan.Validate(now); !contract.HasCode(err, contract.ErrInvalidArgument) {
		t.Fatalf("expiry extension with stale owner digest was accepted: %v", err)
	}
}

func TestQualityCheckpointCanonicalDigestAndTamper(t *testing.T) {
	manifest := validManifest(t)
	second := manifest.Participants[0]
	second.ParticipantID = "context"
	second.SnapshotRef = "snapshot-2"
	second.SnapshotDigest = "snapshot-digest-2"
	second.EvidenceRef = "evidence-2"
	second.InspectFactRef = "participant-fact-2"
	manifest.Participants = append(manifest.Participants, second)
	a := manifest
	a.Digest = ""
	b := manifest
	b.Participants = []contract.SnapshotBinding{second, manifest.Participants[0]}
	b.Digest = ""
	assertSameDigest(t, a.CanonicalDigest, b.CanonicalDigest)
	manifest.Digest, _ = manifest.CanonicalDigest()
	manifest.Participants[0].SnapshotDigest = "tampered"
	if _, err := contract.ValidateCheckpointManifest(manifest); err == nil {
		t.Fatal("checkpoint manifest tamper with stale digest was accepted")
	}
}

func TestQualityObjectManifestRejectsOrderedChunkTamper(t *testing.T) {
	first := []byte("ab")
	second := []byte("cd")
	manifest := contract.ObjectManifest{
		ContractVersion: contract.ContractVersion, ObjectID: "object-1", SchemaVersion: "content/v1",
		ContentDigest: contract.DigestBytes(append(append([]byte{}, first...), second...)), TotalLength: 4,
		Chunks: []contract.ChunkRef{
			{SchemaVersion: "content/v1", Digest: contract.DigestBytes(first), Length: 2},
			{SchemaVersion: "content/v1", Digest: contract.DigestBytes(second), Length: 2},
		},
		Compression: "identity", Classification: "internal", OwnerID: "continuity",
		ScopeDigest: "scope-1", RetentionPolicyRef: "retention-1", CreatedUnixNano: 1,
	}
	manifest.Digest, _ = manifest.CanonicalDigest()
	if err := manifest.Validate(); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
	manifest.Chunks[0], manifest.Chunks[1] = manifest.Chunks[1], manifest.Chunks[0]
	if err := manifest.Validate(); !contract.HasCode(err, contract.ErrContentDigestMismatch) {
		t.Fatalf("ordered chunk tamper accepted: %v", err)
	}
}

func assertSameDigest(t *testing.T, a, b func() (string, error)) {
	t.Helper()
	da, err := a()
	if err != nil {
		t.Fatal(err)
	}
	db, err := b()
	if err != nil {
		t.Fatal(err)
	}
	if da != db {
		t.Fatalf("equivalent set ordering drifted digest: %s != %s", da, db)
	}
}

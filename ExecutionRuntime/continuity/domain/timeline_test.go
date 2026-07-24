package domain_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

func TestTimelineDedupeConflictAndObservationTrust(t *testing.T) {
	ctx := context.Background()
	clock := &testkit.Clock{Time: time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)}
	service, err := domain.NewReferenceTimeline(memory.New(), clock, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	candidate := testkit.Candidate(1, 1, contract.TrustObservation)
	record, duplicate, err := service.Project(ctx, candidate)
	if err != nil || duplicate {
		t.Fatalf("initial projection: duplicate=%v err=%v", duplicate, err)
	}
	if record.TrustClass != contract.TrustObservation || record.Candidate.OwnerFactRef != nil {
		t.Fatalf("observation was upgraded: %#v", record)
	}
	_, duplicate, err = service.Project(ctx, candidate)
	if err != nil || !duplicate {
		t.Fatalf("same candidate should be idempotent: duplicate=%v err=%v", duplicate, err)
	}
	semanticConflict := testkit.Candidate(2, 1, contract.TrustObservation)
	semanticConflict.Evidence.RecordDigest = candidate.Evidence.RecordDigest
	semanticConflict.Evidence.PayloadDigest = candidate.Evidence.PayloadDigest
	semanticConflict.SemanticKind = "praxis/control"
	semanticConflict.Digest, _ = semanticConflict.CanonicalDigest()
	if _, _, err := service.Project(ctx, semanticConflict); !contract.HasCode(err, contract.ErrProjectionConflict) {
		t.Fatalf("same admitted evidence with changed projection semantics should conflict, got %v", err)
	}
	conflict := testkit.Candidate(2, 1, contract.TrustObservation)
	if _, _, err := service.Project(ctx, conflict); !contract.HasCode(err, contract.ErrEvidenceConflict) {
		t.Fatalf("same source sequence with changed evidence should conflict, got %v", err)
	}
}

func TestTimelineCursorDriftAndWatchGap(t *testing.T) {
	ctx := context.Background()
	clock := &testkit.Clock{Time: time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)}
	service, _ := domain.NewReferenceTimeline(memory.New(), clock, time.Minute)
	if _, _, err := service.Project(ctx, testkit.Candidate(1, 1, contract.TrustObservation)); err != nil {
		t.Fatal(err)
	}
	query := contract.TimelineQuery{
		LedgerScopeDigest: "ledger-scope-1", AuthorityWatermark: "authority-1",
		PolicyWatermark: "policy-1", PageLimit: 1,
	}
	page, err := service.Query(ctx, query)
	if err != nil || len(page.Records) != 1 || page.NextCursor == "" {
		t.Fatalf("initial page: %#v err=%v", page, err)
	}
	drifted := query
	drifted.Cursor = page.NextCursor
	drifted.PolicyWatermark = "policy-2"
	if _, err := service.Query(ctx, drifted); !contract.HasCode(err, contract.ErrCursorInvalidated) {
		t.Fatalf("policy drift should invalidate cursor, got %v", err)
	}
	if _, _, err := service.Project(ctx, testkit.Candidate(3, 3, contract.TrustObservation)); err != nil {
		t.Fatal(err)
	}
	watch := query
	watch.Cursor = page.NextCursor
	if _, err := service.Watch(ctx, watch); !contract.HasCode(err, contract.ErrWatchGap) {
		t.Fatalf("missing ledger sequence should produce watch gap, got %v", err)
	}
}

func TestTimelineQueryTypedObjectDimensionsAndCursorDrift(t *testing.T) {
	ctx := context.Background()
	clock := &testkit.Clock{Time: time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)}
	service, _ := domain.NewReferenceTimeline(memory.New(), clock, time.Minute)
	matching := testkit.Candidate(1, 1, contract.TrustObservation)
	matching.ObjectRefs = []string{
		"action-1", "artifact-1", "checkpoint-1", "effect-1", "review-case-1", "step-1", "turn-1",
	}
	matching.Digest, _ = matching.CanonicalDigest()
	nonMatching := testkit.Candidate(2, 2, contract.TrustObservation)
	nonMatching.ObjectRefs = []string{
		"action-2", "artifact-2", "checkpoint-2", "effect-2", "review-case-2", "step-2", "turn-2",
	}
	nonMatching.Digest, _ = nonMatching.CanonicalDigest()
	for _, candidate := range []contract.TimelineProjectionCandidate{matching, nonMatching} {
		if _, _, err := service.Project(ctx, candidate); err != nil {
			t.Fatal(err)
		}
	}
	queries := []contract.TimelineQuery{
		{TurnRef: "turn-1"}, {StepRef: "step-1"}, {ActionRef: "action-1"},
		{ArtifactRef: "artifact-1"}, {EffectRef: "effect-1"},
		{ReviewCaseRef: "review-case-1"}, {CheckpointRef: "checkpoint-1"},
	}
	for _, query := range queries {
		query.LedgerScopeDigest = "ledger-scope-1"
		query.AuthorityWatermark = "authority-1"
		query.PolicyWatermark = "policy-1"
		query.PageLimit = 1
		page, err := service.Query(ctx, query)
		if err != nil || len(page.Records) != 1 || page.Records[0].EvidenceRecordRef != matching.Evidence.RecordRef {
			t.Fatalf("typed query mismatch: query=%+v page=%+v err=%v", query, page, err)
		}
		drifted := query
		drifted.Cursor = page.NextCursor
		drifted.CheckpointRef = "checkpoint-drift"
		if _, err := service.Query(ctx, drifted); !contract.HasCode(err, contract.ErrCursorInvalidated) {
			t.Fatalf("typed query cursor drift was accepted: %v", err)
		}
	}
	all := queries[0]
	all.LedgerScopeDigest = "ledger-scope-1"
	all.AuthorityWatermark = "authority-1"
	all.PolicyWatermark = "policy-1"
	all.PageLimit = 10
	all.StepRef, all.ActionRef, all.ArtifactRef = "step-1", "action-1", "artifact-1"
	all.EffectRef, all.ReviewCaseRef, all.CheckpointRef = "effect-1", "review-case-1", "checkpoint-1"
	page, err := service.Query(ctx, all)
	if err != nil || len(page.Records) != 1 {
		t.Fatalf("typed dimensions must compose with AND semantics: page=%+v err=%v", page, err)
	}
}

func TestWatchDoesNotMistakeFilteredRecordForGap(t *testing.T) {
	ctx := context.Background()
	clock := &testkit.Clock{Time: time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)}
	service, _ := domain.NewReferenceTimeline(memory.New(), clock, time.Minute)
	first := testkit.Candidate(1, 1, contract.TrustObservation)
	second := testkit.Candidate(2, 2, contract.TrustObservation)
	second.SemanticKind = "praxis/control"
	second.Digest, _ = second.CanonicalDigest()
	third := testkit.Candidate(3, 3, contract.TrustObservation)
	for _, candidate := range []contract.TimelineProjectionCandidate{first, second, third} {
		if _, _, err := service.Project(ctx, candidate); err != nil {
			t.Fatal(err)
		}
	}
	query := contract.TimelineQuery{
		LedgerScopeDigest: "ledger-scope-1", SemanticKinds: []string{"praxis/observation"},
		AuthorityWatermark: "authority-1", PolicyWatermark: "policy-1", PageLimit: 1,
	}
	page, err := service.Query(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	query.Cursor = page.NextCursor
	page, err = service.Watch(ctx, query)
	if err != nil || len(page.Records) != 1 || page.Records[0].LedgerSequence != 3 {
		t.Fatalf("filtered sequence should not be a gap: page=%#v err=%v", page, err)
	}
}

func TestProjectionRebuildRejectsCycleAndNeverReplacesHistory(t *testing.T) {
	ctx := context.Background()
	clock := &testkit.Clock{Time: time.Now()}
	backend := memory.New()
	service, _ := domain.NewReferenceTimeline(backend, clock, time.Minute)
	a := testkit.Candidate(1, 1, contract.TrustObservation)
	b := testkit.Candidate(2, 2, contract.TrustObservation)
	a.ParentRefs = []string{b.CandidateID}
	b.ParentRefs = []string{a.CandidateID}
	a.Digest, _ = a.CanonicalDigest()
	b.Digest, _ = b.CanonicalDigest()
	if err := service.Rebuild(ctx, "ledger-scope-1", []contract.TimelineProjectionCandidate{a, b}); !contract.HasCode(err, contract.ErrProjectionConflict) {
		t.Fatalf("cycle should be rejected, got %v", err)
	}
	a.ParentRefs = nil
	b.ParentRefs = []string{a.CandidateID}
	a.Digest, _ = a.CanonicalDigest()
	b.Digest, _ = b.CanonicalDigest()
	if err := service.Rebuild(ctx, "ledger-scope-1", []contract.TimelineProjectionCandidate{a, b}); err != nil {
		t.Fatalf("valid rebuild failed: %v", err)
	}
	if err := service.Rebuild(ctx, "ledger-scope-1", []contract.TimelineProjectionCandidate{a}); err != nil {
		t.Fatalf("idempotent partial rebuild failed: %v", err)
	}
	page, err := service.Query(ctx, contract.TimelineQuery{
		LedgerScopeDigest: "ledger-scope-1", AuthorityWatermark: "authority-1",
		PolicyWatermark: "policy-1", PageLimit: 10,
	})
	if err != nil || len(page.Records) != 2 {
		t.Fatalf("partial rebuild replaced immutable history: %#v err=%v", page, err)
	}
}

func TestIncrementalProjectionRejectsParentCycle(t *testing.T) {
	ctx := context.Background()
	clock := &testkit.Clock{Time: time.Now()}
	service, _ := domain.NewReferenceTimeline(memory.New(), clock, time.Minute)
	a := testkit.Candidate(1, 1, contract.TrustObservation)
	b := testkit.Candidate(2, 2, contract.TrustObservation)
	a.ParentRefs = []string{b.CandidateID}
	b.ParentRefs = []string{a.CandidateID}
	a.Digest, _ = a.CanonicalDigest()
	b.Digest, _ = b.CanonicalDigest()
	if _, _, err := service.Project(ctx, a); err != nil {
		t.Fatalf("forward parent reference should remain admissible: %v", err)
	}
	if _, _, err := service.Project(ctx, b); !contract.HasCode(err, contract.ErrProjectionConflict) {
		t.Fatalf("cycle should be rejected atomically, got %v", err)
	}
}

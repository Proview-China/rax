package contract_test

import (
	"errors"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
)

func TestContextCompactionSummaryRequiresCanonicalBoundedRefsV1(t *testing.T) {
	summary := compactionSummaryFixtureV1()
	if err := summary.Validate(); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name   string
		mutate func(*contract.ContextCompactionSummaryV1)
	}{
		{"duplicate_anchor", func(s *contract.ContextCompactionSummaryV1) {
			s.RetainedAnchorRefs = append(s.RetainedAnchorRefs, s.RetainedAnchorRefs[0])
		}},
		{"unsorted_open_effects", func(s *contract.ContextCompactionSummaryV1) {
			s.OpenEffectRefs = []contract.FactRef{factRefV1("z"), factRefV1("a")}
		}},
		{"nil_outstanding_work", func(s *contract.ContextCompactionSummaryV1) { s.OutstandingWorkRefs = nil }},
		{"no_token_reduction", func(s *contract.ContextCompactionSummaryV1) { s.TokensAfter = s.TokensBefore }},
		{"singleton_range_drift", func(s *contract.ContextCompactionSummaryV1) { s.SourceRange.FrameCount = 1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := summary
			value.RetainedAnchorRefs = append([]contract.FactRef(nil), summary.RetainedAnchorRefs...)
			value.OpenEffectRefs = append([]contract.FactRef(nil), summary.OpenEffectRefs...)
			value.OutstandingWorkRefs = append([]contract.FactRef(nil), summary.OutstandingWorkRefs...)
			value.UncompressibleRefs = append([]contract.FactRef(nil), summary.UncompressibleRefs...)
			test.mutate(&value)
			if err := value.Validate(); err == nil {
				t.Fatal("invalid compaction summary accepted")
			}
		})
	}
}

func TestContextCompactionPlanSealsExactCurrentWindowV1(t *testing.T) {
	summary := compactionSummaryFixtureV1()
	summaryDigest, _ := summary.DigestValue()
	current := currentPointerV1(t, summary.SourceGenerationRef)
	plan, err := contract.SealContextCompactionPlanV1(contract.ContextCompactionPlanV1{
		AttemptID: "compaction-attempt-1", IdempotencyKey: "compact-once", ExpectedCurrent: current,
		SummaryRef:         contract.FactRef{ID: summary.ID, Revision: summary.Revision, Digest: summaryDigest},
		TargetGenerationID: "generation-2", TargetRootFrameRef: factRefV1("frame-root-2"),
		CheckedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	bad := plan
	bad.ExpiresUnixNano = current.ExpiresUnixNano + 1
	bad.Digest = testkit.D("tampered")
	if err := bad.Validate(); err == nil || (!errors.Is(err, contract.ErrExpired) && !errors.Is(err, contract.ErrConflict)) {
		t.Fatalf("plan beyond owner window accepted: %v", err)
	}
}

func compactionSummaryFixtureV1() contract.ContextCompactionSummaryV1 {
	return contract.ContextCompactionSummaryV1{
		ContractVersion: contract.Version, ID: "compaction-summary-1", Revision: 1,
		SourceGenerationRef: factRefV1("generation-1"),
		SourceRange:         contract.ContextCompactionSourceRangeV1{FirstFrameRef: factRefV1("frame-1"), LastFrameRef: factRefV1("frame-9"), FrameCount: 9},
		AlgorithmID:         "bounded-summary", AlgorithmVersion: "v1", SourceDigest: testkit.D("source-frames"),
		Summary:            contract.ContentRef{Ref: "summary-content", Digest: testkit.D("summary"), Length: 128},
		RetainedAnchorRefs: []contract.FactRef{factRefV1("anchor-a")},
		OpenEffectRefs:     []contract.FactRef{factRefV1("effect-a")}, OutstandingWorkRefs: []contract.FactRef{factRefV1("work-a")}, UncompressibleRefs: []contract.FactRef{factRefV1("precise-a")},
		TokensBefore: 4000, TokensAfter: 800, Evidence: testkit.Evidence("compaction-evidence"), CreatedUnixNano: testkit.Now - 10, ExpiresUnixNano: testkit.Now + 100,
	}
}

func factRefV1(id string) contract.FactRef {
	return contract.FactRef{ID: id, Revision: 1, Digest: testkit.D(id)}
}

func currentPointerV1(t *testing.T, generation contract.FactRef) contract.ContextGenerationCurrentPointerV1 {
	t.Helper()
	pointer, err := contract.SealContextGenerationCurrentPointerV1(contract.ContextGenerationCurrentPointerV1{
		ID: "generation-current-1", Revision: 1, ExecutionScopeDigest: testkit.D("scope"), RunID: "run-1", SessionRef: factRefV1("session-1"), Turn: 9,
		GenerationRef: generation, GenerationOrdinal: 1, ParentFrameGenerationBindingDigest: testkit.D("binding"), ExpiresUnixNano: testkit.Now + 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	return pointer
}

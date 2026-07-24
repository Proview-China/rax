package kernel_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
)

func TestPrepareContextCompactionV1DeterministicAndNotCurrent(t *testing.T) {
	plan, summary := compactionFixtureV1(t)
	left, err := kernel.PrepareContextCompactionV1(context.Background(), plan, summary)
	if err != nil {
		t.Fatal(err)
	}
	right, err := kernel.PrepareContextCompactionV1(context.Background(), plan, summary)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(left, right) || left.Current || left.Generation.Ordinal != plan.ExpectedCurrent.GenerationOrdinal+1 || left.Generation.Parent == nil || *left.Generation.Parent != plan.ExpectedCurrent.GenerationRef || left.Generation.Summary == nil || *left.Generation.Summary != summary.Summary {
		t.Fatalf("non-deterministic or incorrectly bound compaction: %#v %#v", left, right)
	}
	if left.GenerationRef.Digest == plan.ExpectedCurrent.GenerationRef.Digest {
		t.Fatal("compaction reused parent generation digest")
	}
}

func TestPrepareContextCompactionV1FailsClosedOnDriftExpiryAndCancel(t *testing.T) {
	plan, summary := compactionFixtureV1(t)
	for _, test := range []struct {
		name string
		ctx  context.Context
		plan func(contract.ContextCompactionPlanV1) contract.ContextCompactionPlanV1
		sum  func(contract.ContextCompactionSummaryV1) contract.ContextCompactionSummaryV1
	}{
		{"canceled", canceledContextV1(), identityPlanV1, identitySummaryV1},
		{"source_generation_drift", context.Background(), identityPlanV1, func(s contract.ContextCompactionSummaryV1) contract.ContextCompactionSummaryV1 {
			s.SourceGenerationRef = refV1("other-generation")
			return s
		}},
		{"summary_expired", context.Background(), func(p contract.ContextCompactionPlanV1) contract.ContextCompactionPlanV1 {
			p.CheckedUnixNano = summary.ExpiresUnixNano
			p.ExpiresUnixNano = p.CheckedUnixNano + 1
			p.Digest = ""
			p, _ = contract.SealContextCompactionPlanV1(p)
			return p
		}, identitySummaryV1},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := kernel.PrepareContextCompactionV1(test.ctx, test.plan(plan), test.sum(summary))
			if err == nil || !reflect.DeepEqual(got, contract.ContextCompactionPreparedV1{}) {
				t.Fatalf("expected zero fail-closed result, got=%#v err=%v", got, err)
			}
		})
	}
}

func TestPreparedCompactionDropsUnretainedAnchorEvenWhenSummaryExists(t *testing.T) {
	plan, summary := compactionFixtureV1(t)
	retained := refV1("anchor-retained")
	summary.RetainedAnchorRefs = []contract.FactRef{retained}
	summaryDigest, _ := summary.DigestValue()
	plan.SummaryRef = contract.FactRef{ID: summary.ID, Revision: 1, Digest: summaryDigest}
	plan, _ = contract.SealContextCompactionPlanV1(plan)
	prepared, err := kernel.PrepareContextCompactionV1(context.Background(), plan, summary)
	if err != nil {
		t.Fatal(err)
	}
	anchor := contract.ArtifactAnchor{ContractVersion: contract.Version, ID: "anchor-dropped", Revision: 1, ArtifactOwner: testkit.Owner(), ArtifactRef: "artifact-1", ArtifactVersion: "v1", ArtifactDigest: testkit.D("artifact-v1"), Range: contract.ArtifactRange{Start: 1, End: 2}, FrameRef: refV1("frame-1"), GenerationID: "generation-1", Evidence: testkit.Evidence("anchor"), CreatedUnixNano: testkit.Now - int64(time.Minute), ExpiresUnixNano: testkit.Now + int64(time.Minute)}
	mode, err := contract.PlanArtifactReadAfterCompaction(anchor, prepared.Generation, "v1", anchor.ArtifactDigest, anchor.Range, 1, nil, testkit.Now)
	if err != nil || mode != contract.ArtifactRematerialize {
		t.Fatalf("unretained anchor survived compaction: mode=%q err=%v", mode, err)
	}
}

func compactionFixtureV1(t *testing.T) (contract.ContextCompactionPlanV1, contract.ContextCompactionSummaryV1) {
	t.Helper()
	summary := contract.ContextCompactionSummaryV1{ContractVersion: contract.Version, ID: "summary-1", Revision: 1, SourceGenerationRef: refV1("generation-1"), SourceRange: contract.ContextCompactionSourceRangeV1{FirstFrameRef: refV1("frame-1"), LastFrameRef: refV1("frame-2"), FrameCount: 2}, AlgorithmID: "summary", AlgorithmVersion: "v1", SourceDigest: testkit.D("source"), Summary: contract.ContentRef{Ref: "summary-content", Digest: testkit.D("summary-content"), Length: 20}, RetainedAnchorRefs: []contract.FactRef{refV1("anchor-a")}, OpenEffectRefs: []contract.FactRef{}, OutstandingWorkRefs: []contract.FactRef{refV1("work-a")}, UncompressibleRefs: []contract.FactRef{refV1("precise-a")}, TokensBefore: 1000, TokensAfter: 200, Evidence: testkit.Evidence("summary"), CreatedUnixNano: testkit.Now - 10, ExpiresUnixNano: testkit.Now + 100}
	current, err := contract.SealContextGenerationCurrentPointerV1(contract.ContextGenerationCurrentPointerV1{ID: "current-1", Revision: 1, ExecutionScopeDigest: testkit.D("scope"), RunID: "run-1", SessionRef: refV1("session-1"), Turn: 2, GenerationRef: summary.SourceGenerationRef, GenerationOrdinal: 1, ParentFrameGenerationBindingDigest: testkit.D("binding"), ExpiresUnixNano: testkit.Now + 100})
	if err != nil {
		t.Fatal(err)
	}
	summaryDigest, _ := summary.DigestValue()
	plan, err := contract.SealContextCompactionPlanV1(contract.ContextCompactionPlanV1{AttemptID: "attempt-1", IdempotencyKey: "once", ExpectedCurrent: current, SummaryRef: contract.FactRef{ID: summary.ID, Revision: 1, Digest: summaryDigest}, TargetGenerationID: "generation-2", TargetRootFrameRef: refV1("frame-root-2"), CheckedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + 50})
	if err != nil {
		t.Fatal(err)
	}
	return plan, summary
}

func refV1(id string) contract.FactRef {
	return contract.FactRef{ID: id, Revision: 1, Digest: testkit.D(id)}
}
func identityPlanV1(value contract.ContextCompactionPlanV1) contract.ContextCompactionPlanV1 {
	return value
}
func identitySummaryV1(value contract.ContextCompactionSummaryV1) contract.ContextCompactionSummaryV1 {
	return value
}
func canceledContextV1() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

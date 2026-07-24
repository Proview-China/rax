package kernel_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/refreshstore"
)

type compactionServiceFixtureV1 struct {
	now      time.Time
	parent   *testfixture.ParentFrameFixtureV1
	store    *refreshstore.Memory
	service  *kernel.ContextCompactionServiceV1
	plan     contract.ContextCompactionPlanV1
	summary  contract.ContextCompactionSummaryV1
	manifest contract.ContextManifest
	frame    contract.ContextFrame
}

func TestContextCompactionPrepareApplyInspectAtomicV1(t *testing.T) {
	f := newCompactionServiceFixtureV1(t, "atomic")
	prepared, err := f.service.Prepare(context.Background(), f.plan, f.summary, f.manifest, f.frame)
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.service.Inspect(context.Background(), contract.InspectContextCompactionRequestV1{PlanRef: prepared.PlanRef})
	if err != nil || result.Status != contract.ContextCompactionPendingV1 || result.Current != nil {
		t.Fatalf("pending inspect: result=%#v err=%v", result, err)
	}
	if _, err := f.store.GenerationByExactRef(context.Background(), prepared.GenerationRef, f.frame.Execution.ScopeDigest); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("pending generation became visible: %v", err)
	}
	apply := sealCompactionApplyV1(t, f, prepared)
	result, err = f.service.Apply(context.Background(), apply)
	if err != nil || result.Status != contract.ContextCompactionAppliedV1 || result.Current == nil {
		t.Fatalf("apply: result=%#v err=%v", result, err)
	}
	if _, err := f.store.GenerationByExactRef(context.Background(), prepared.GenerationRef, f.frame.Execution.ScopeDigest); err != nil {
		t.Fatalf("applied generation not visible: %v", err)
	}
	inspected, err := f.service.Inspect(context.Background(), contract.InspectContextCompactionRequestV1{PlanRef: prepared.PlanRef})
	if err != nil || inspected.Digest != result.Digest {
		t.Fatalf("exact inspect after apply: %#v %v", inspected, err)
	}
	if _, err := f.service.Apply(context.Background(), apply); !errors.Is(err, contract.ErrInspectOnly) || !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("repeat apply did not require inspect: %v", err)
	}
	if _, err := f.service.Prepare(context.Background(), f.plan, f.summary, f.manifest, f.frame); !errors.Is(err, contract.ErrInspectOnly) {
		t.Fatalf("repeat prepare did not require inspect: %v", err)
	}
}

func TestContextCompactionS2DriftKeepsCandidateInvisibleV1(t *testing.T) {
	f := newCompactionServiceFixtureV1(t, "drift")
	prepared, err := f.service.Prepare(context.Background(), f.plan, f.summary, f.manifest, f.frame)
	if err != nil {
		t.Fatal(err)
	}
	drift := f.plan.ExpectedCurrent
	drift.Revision++
	drift.GenerationRef = refV1("legitimate-other-generation")
	drift.GenerationOrdinal++
	drift.ParentFrameGenerationBindingDigest = testkit.D("legitimate-other-binding")
	drift, err = contract.SealContextGenerationCurrentPointerV1(drift)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.CompareAndSwapGenerationCurrentV1(context.Background(), f.plan.ExpectedCurrent, drift); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Apply(context.Background(), sealCompactionApplyV1(t, f, prepared)); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("S2 drift accepted: %v", err)
	}
	if _, err := f.store.GenerationByExactRef(context.Background(), prepared.GenerationRef, f.frame.Execution.ScopeDigest); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("drifted candidate became visible: %v", err)
	}
	result, err := f.service.Inspect(context.Background(), contract.InspectContextCompactionRequestV1{PlanRef: prepared.PlanRef})
	if err != nil || result.Status != contract.ContextCompactionPendingV1 {
		t.Fatalf("pending not inspectable after drift: %#v %v", result, err)
	}
}

func TestContextCompaction64ConcurrentApplySingleCurrentV1(t *testing.T) {
	f := newCompactionServiceFixtureV1(t, "concurrent")
	prepared, err := f.service.Prepare(context.Background(), f.plan, f.summary, f.manifest, f.frame)
	if err != nil {
		t.Fatal(err)
	}
	apply := sealCompactionApplyV1(t, f, prepared)
	var success atomic.Int32
	var wg sync.WaitGroup
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := f.service.Apply(context.Background(), apply); err == nil {
				success.Add(1)
			} else if !errors.Is(err, contract.ErrInspectOnly) && !errors.Is(err, contract.ErrConflict) {
				t.Errorf("unexpected concurrent error: %v", err)
			}
		}()
	}
	wg.Wait()
	if success.Load() != 1 {
		t.Fatalf("successes=%d want=1", success.Load())
	}
}

func TestContextCompactionLostReplyAndTTLRequireInspectV1(t *testing.T) {
	t.Run("lost_apply_reply", func(t *testing.T) {
		f := newCompactionServiceFixtureV1(t, "lost-reply")
		prepared, err := f.service.Prepare(context.Background(), f.plan, f.summary, f.manifest, f.frame)
		if err != nil {
			t.Fatal(err)
		}
		apply := sealCompactionApplyV1(t, f, prepared)
		applied, err := f.store.ApplyContextCompactionCurrentCASV1(context.Background(), apply)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.service.Apply(context.Background(), apply); !errors.Is(err, contract.ErrInspectOnly) {
			t.Fatalf("lost reply replay was not inspect-only: %v", err)
		}
		inspected, err := f.service.Inspect(context.Background(), contract.InspectContextCompactionRequestV1{PlanRef: prepared.PlanRef})
		if err != nil || inspected.Digest != applied.Digest {
			t.Fatalf("lost reply exact inspect failed: %#v %v", inspected, err)
		}
	})
	t.Run("ttl_crossing", func(t *testing.T) {
		f := newCompactionServiceFixtureV1(t, "ttl")
		prepared, err := f.service.Prepare(context.Background(), f.plan, f.summary, f.manifest, f.frame)
		if err != nil {
			t.Fatal(err)
		}
		expiredService, err := kernel.NewContextCompactionServiceV1(f.store, f.parent.Content, func() time.Time { return time.Unix(0, f.plan.ExpiresUnixNano) })
		if err != nil {
			t.Fatal(err)
		}
		if _, err := expiredService.Apply(context.Background(), sealCompactionApplyV1(t, f, prepared)); !errors.Is(err, contract.ErrExpired) {
			t.Fatalf("TTL crossing accepted: %v", err)
		}
		if _, err := f.store.GenerationByExactRef(context.Background(), prepared.GenerationRef, f.frame.Execution.ScopeDigest); !errors.Is(err, contract.ErrNotFound) {
			t.Fatalf("expired candidate became visible: %v", err)
		}
	})
}

func newCompactionServiceFixtureV1(t *testing.T, suffix string) *compactionServiceFixtureV1 {
	t.Helper()
	now := time.Unix(0, testkit.Now)
	parent, err := testfixture.NewParentFrameFixtureV1(func() time.Time { return now }, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	store, err := refreshstore.NewMemoryWithCurrentV1(refreshstore.CurrentStateV1{Binding: parent.Binding, Frame: parent.Frame, Manifest: parent.Manifest, Generation: parent.Generation, Pointer: parent.Pointer})
	if err != nil {
		t.Fatal(err)
	}
	summaryContent, err := parent.Content.Put([]byte("bounded compaction summary " + suffix))
	if err != nil {
		t.Fatal(err)
	}
	recipe := testkit.Recipe()
	recipe.Rules = append(recipe.Rules, contract.FragmentRule{Kind: contract.FragmentCompactionSummary, Region: contract.RegionDynamicTail, Required: true, MaxTokens: 100, Degradation: contract.DegradeReject})
	parentRef, _ := parent.Frame.DigestValue()
	compiled, err := kernel.Compile(parent.Content, kernel.CompileRequest{AttemptID: "compact-frame-" + suffix, ManifestID: "compact-manifest-" + suffix, FrameID: "compact-root-" + suffix, GenerationID: "compact-generation-" + suffix, Generation: parent.Generation.Ordinal + 1, Recipe: recipe, Execution: parent.Frame.Execution, Candidates: []contract.ContextCandidate{testkit.Candidate("compact-instruction-candidate-"+suffix, contract.FragmentInstruction, parent.Manifest.Fragments[0].Content, 10), testkit.Candidate("compact-summary-candidate-"+suffix, contract.FragmentCompactionSummary, summaryContent, 20)}, ParentFrame: &contract.FactRef{ID: parent.Frame.ID, Revision: parent.Frame.Revision, Digest: parentRef}, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	frameDigest, _ := compiled.Frame.DigestValue()
	summary := contract.ContextCompactionSummaryV1{ContractVersion: contract.Version, ID: "summary-" + suffix, Revision: 1, SourceGenerationRef: parent.Pointer.GenerationRef, SourceRange: contract.ContextCompactionSourceRangeV1{FirstFrameRef: parent.Generation.RootFrame, LastFrameRef: parent.Generation.RootFrame, FrameCount: 1}, AlgorithmID: "bounded-summary", AlgorithmVersion: "v1", SourceDigest: testkit.D("source-" + suffix), Summary: summaryContent, RetainedAnchorRefs: []contract.FactRef{}, OpenEffectRefs: []contract.FactRef{}, OutstandingWorkRefs: []contract.FactRef{}, UncompressibleRefs: []contract.FactRef{}, TokensBefore: 1000, TokensAfter: 20, Evidence: testkit.Evidence("summary-" + suffix), CreatedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()}
	summaryDigest, _ := summary.DigestValue()
	plan, err := contract.SealContextCompactionPlanV1(contract.ContextCompactionPlanV1{AttemptID: "compaction-attempt-" + suffix, IdempotencyKey: "compaction-once-" + suffix, ExpectedCurrent: parent.Pointer, SummaryRef: contract.FactRef{ID: summary.ID, Revision: 1, Digest: summaryDigest}, TargetGenerationID: "compact-generation-" + suffix, TargetRootFrameRef: contract.FactRef{ID: compiled.Frame.ID, Revision: 1, Digest: frameDigest}, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(9 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	service, err := kernel.NewContextCompactionServiceV1(store, parent.Content, func() time.Time { return now.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	return &compactionServiceFixtureV1{now: now, parent: parent, store: store, service: service, plan: plan, summary: summary, manifest: compiled.Manifest, frame: compiled.Frame}
}

func sealCompactionApplyV1(t *testing.T, f *compactionServiceFixtureV1, prepared contract.ContextCompactionPreparedV1) contract.ApplyContextCompactionRequestV1 {
	t.Helper()
	request, err := contract.SealApplyContextCompactionRequestV1(contract.ApplyContextCompactionRequestV1{PlanRef: prepared.PlanRef, PreparedDigest: prepared.Digest, ExpectedCurrent: f.plan.ExpectedCurrent, CheckedUnixNano: f.now.Add(time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

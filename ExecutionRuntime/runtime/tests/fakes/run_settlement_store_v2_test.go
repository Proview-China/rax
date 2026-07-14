package fakes_test

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func persistRunningRunBundleV2(t *testing.T, store control.RunSettlementFactPortV2, desired core.AgentRunRecord, plan ports.RunSettlementPlanFactV2) core.AgentRunRecord {
	t.Helper()
	pending := desired
	pending.Status = core.RunPending
	pending.Revision = 1
	pending.StartedAt = time.Time{}
	pending.EndedAt = time.Time{}
	pending.Outcome = ""
	pending.CompletionClaim = nil
	if _, err := store.CreateRunBundleV2(context.Background(), control.RunBundleCreateRequestV2{Run: pending, Plan: plan}); err != nil {
		t.Fatal(err)
	}
	running := desired
	running.Status = core.RunRunning
	running.Revision = 2
	if running.StartedAt.IsZero() {
		running.StartedAt = planTimeV2(plan)
	}
	stored, err := store.CompareAndSwapRun(context.Background(), control.RunFactCASRequest{ExpectedRevision: pending.Revision, Next: running})
	if err != nil {
		t.Fatal(err)
	}
	return stored
}

func planTimeV2(plan ports.RunSettlementPlanFactV2) time.Time {
	return time.Unix(0, plan.CreatedUnixNano)
}

func TestRunSettlementStoreV2AtomicBundleLostReplyAndConflict(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 14, 0, 0, 0, time.UTC)
	store := fakes.NewRunSettlementStoreV2(func() time.Time { return now })
	run := runningRecord(runScope(t), "run-settlement-bundle", now)
	plan := runSettlementPlanFixtureV2(t, run, now)
	run.Status = core.RunPending
	run.StartedAt = time.Time{}
	store.LoseNextBundleReply()
	if _, err := store.CreateRunBundleV2(context.Background(), control.RunBundleCreateRequestV2{Run: run, Plan: plan}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("expected injected lost bundle reply, got %v", err)
	}
	inspectedRun, err := store.InspectRun(context.Background(), run.Scope, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	inspectedPlan, err := store.InspectRunSettlementPlanV2(context.Background(), run.Scope, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if inspectedRun.ID != run.ID || inspectedPlan.ID != plan.ID {
		t.Fatalf("atomic bundle was not fully visible: run=%+v plan=%+v", inspectedRun, inspectedPlan)
	}
	if _, err := store.CreateRunBundleV2(context.Background(), control.RunBundleCreateRequestV2{Run: run, Plan: plan}); err != nil {
		t.Fatalf("exact lost-reply replay must be idempotent: %v", err)
	}
	changed := plan
	changed.Execution.EndpointDigest = runSettlementDigestV2(t, "changed-endpoint")
	changed.Execution.SubjectDigest, _ = changed.Execution.DigestV2()
	for index := range changed.Requirements {
		if changed.Requirements[index].Kind == ports.RunRequirementExecutionTruth {
			changed.Requirements[index].SubjectDigest = changed.Execution.SubjectDigest
		}
	}
	if _, err := store.CreateRunBundleV2(context.Background(), control.RunBundleCreateRequestV2{Run: run, Plan: changed}); !core.HasReason(err, core.ReasonRunSettlementPlanConflict) {
		t.Fatalf("same Run with a different Plan must conflict: %v", err)
	}
}

func TestRunSettlementPlanV2AllowsMultipleCustomOwnersOfSameKindButReservedKindsExactlyOnce(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 14, 0, 30, 0, time.UTC)
	run := runningRecord(runScope(t), "run-custom-settlement", now)
	plan := runSettlementPlanFixtureV2(t, run, now)
	base := plan.Requirements[0]
	base.Kind = "custom/domain-commit"
	base.Phase = ports.RunSettlementPhaseCompletion
	for index, id := range []ports.NamespacedNameV2{"custom/module-a", "custom/module-b"} {
		item := base
		item.ID = id
		item.Owner.ComponentID = ports.ComponentIDV2(fmt.Sprintf("custom/component-%d", index))
		item.Owner.ManifestDigest = runSettlementDigestV2(t, fmt.Sprintf("custom-manifest-%d", index))
		item.Owner.ArtifactDigest = runSettlementDigestV2(t, fmt.Sprintf("custom-artifact-%d", index))
		item.SubjectDigest = runSettlementDigestV2(t, string(id))
		item.Policy.Ref = "policy-" + string(id)
		item.Policy.Digest = runSettlementDigestV2(t, "policy-"+string(id))
		item.Policy.SemanticDigest = runSettlementDigestV2(t, "policy-semantic-"+string(id))
		plan.Requirements = append(plan.Requirements, item)
	}
	ports.SortRunSettlementRequirementsV2(plan.Requirements)
	if err := plan.Validate(); err != nil {
		t.Fatalf("same custom kind from two owners was rejected: %v", err)
	}
	duplicate := plan.Requirements[0]
	duplicate.ID = "custom/duplicate-execution"
	duplicate.Kind = ports.RunRequirementExecutionTruth
	plan.Requirements = append(plan.Requirements, duplicate)
	ports.SortRunSettlementRequirementsV2(plan.Requirements)
	if err := plan.Validate(); !core.HasReason(err, core.ReasonRunSettlementRequirementInvalid) {
		t.Fatalf("duplicate reserved Runtime kind was accepted: %v", err)
	}
}

func TestEffectStoreV2AtomicallyIndexesAndFreezesRunEffects(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 14, 1, 0, 0, time.UTC)
	store := fakes.NewEffectStoreV2(func() time.Time { return now })
	intent := effectIntentV2(t, now, "effect-indexed", "idem-indexed", "domain/indexed")
	run := runningRecord(intent.Scope, intent.RunID, now)
	runIdentity, _ := ports.RunIdentityDigestV2(run)
	scopeDigest, _ := ports.ExecutionScopeDigestV2(intent.Scope)
	facts := fakes.NewRunSettlementStoreV2(func() time.Time { return now })
	plan := runSettlementPlanFixtureV2(t, run, now)
	run = persistRunningRunBundleV2(t, facts, run, plan)
	store.SetRunFacts(facts)
	index := control.RunEffectIndexFactV2{ContractVersion: ports.RunSettlementContractVersionV2, ID: "effect-index-run-1", Revision: 1, RunID: run.ID, RunIdentityDigest: runIdentity, ExecutionScope: run.Scope, ExecutionScopeDigest: scopeDigest, State: control.RunEffectIndexOpen, SegmentCount: 0, EffectCount: 0, HeadSegmentDigest: ports.EvidenceGenesisDigestV2, Watermark: 1, CreatedUnixNano: now.UnixNano()}
	if _, err := store.CreateRunEffectIndexV2(context.Background(), index); err != nil {
		t.Fatal(err)
	}
	proposed, err := control.NewProposedEffectFactV2(intent, now)
	if err != nil {
		t.Fatal(err)
	}
	store.LoseNextRunEffectReply()
	if _, err := store.CreateEffectForRunV2(context.Background(), control.CreateRunEffectRequestV2{Partition: index.PartitionV2(), ExpectedIndexRevision: 1, Effect: proposed}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("expected lost atomic effect+index reply, got %v", err)
	}
	result, err := store.CreateEffectForRunV2(context.Background(), control.CreateRunEffectRequestV2{Partition: index.PartitionV2(), ExpectedIndexRevision: 1, Effect: proposed})
	if err != nil || result.Index.EffectCount != 1 || result.Index.Revision != 2 {
		t.Fatalf("lost reply must recover exact Effect and index: result=%+v err=%v", result, err)
	}
	stopping := run
	stopping.Status, stopping.Revision = core.RunStopping, run.Revision+1
	if _, err := facts.CompareAndSwapRun(context.Background(), control.RunFactCASRequest{ExpectedRevision: run.Revision, Next: stopping}); err != nil {
		t.Fatal(err)
	}
	frozen, err := store.FreezeRunEffectSetV2(context.Background(), control.FreezeRunEffectSetRequestV2{Partition: index.PartitionV2(), ExpectedIndexRevision: result.Index.Revision, ExpectedRunRevision: stopping.Revision})
	if err != nil || frozen.State != control.RunEffectIndexFrozen {
		t.Fatalf("freeze failed: %+v %v", frozen, err)
	}
	secondIntent := effectIntentV2(t, now, "effect-after-freeze", "idem-after-freeze", "domain/after-freeze")
	second, _ := control.NewProposedEffectFactV2(secondIntent, now)
	if _, err := store.CreateEffectForRunV2(context.Background(), control.CreateRunEffectRequestV2{Partition: index.PartitionV2(), ExpectedIndexRevision: frozen.Revision, Effect: second}); !core.HasReason(err, core.ReasonRunEffectSetFrozen) {
		t.Fatalf("frozen index accepted a new Effect: %v", err)
	}
}

func TestEffectStoreV2RunPartitionIsolationDeepCloneAndLongRunSegments(t *testing.T) {
	now := time.Date(2026, 7, 14, 14, 1, 30, 0, time.UTC)
	facts := fakes.NewRunSettlementStoreV2(func() time.Time { return now })
	effects := fakes.NewEffectStoreV2(func() time.Time { return now })
	effects.SetRunFacts(facts)
	baseIntent := effectIntentV2(t, now, "effect-shared", "idem-shared", "domain/shared")
	scopes := []core.ExecutionScope{baseIntent.Scope, cloneTestExecutionScopeV2(baseIntent.Scope)}
	scopes[1].Identity.TenantID = "tenant-2"
	scopes[1].Identity.ID = "agent-2"
	scopes[1].Lineage.ID = "lineage-2"
	scopes[1].Instance.ID = "instance-2"
	scopes[1].SandboxLease.ID = "sandbox-2"
	partitions := make([]control.RunEffectPartitionV2, 0, 2)
	for _, scope := range scopes {
		run := runningRecord(scope, "run-1", now)
		plan := runSettlementPlanFixtureV2(t, run, now)
		run = persistRunningRunBundleV2(t, facts, run, plan)
		runIdentity, _ := ports.RunIdentityDigestV2(run)
		scopeDigest, _ := ports.ExecutionScopeDigestV2(scope)
		index := control.RunEffectIndexFactV2{ContractVersion: ports.RunSettlementContractVersionV2, ID: "shared-index-id", Revision: 1, RunID: run.ID, RunIdentityDigest: runIdentity, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, State: control.RunEffectIndexOpen, HeadSegmentDigest: ports.EvidenceGenesisDigestV2, Watermark: 1, CreatedUnixNano: now.UnixNano()}
		partition := index.PartitionV2()
		if _, err := effects.CreateRunEffectIndexV2(context.Background(), index); err != nil {
			t.Fatal(err)
		}
		intent := baseIntent
		intent.Scope = scope
		intent.RunID = run.ID
		stable := ports.StableTenantScopeDigestV2(scope.Identity.TenantID)
		intent.ConflictDomain.ScopeDigest = stable
		intent.Idempotency.ScopeDigest = stable
		proposed, err := control.NewProposedEffectFactV2(intent, now)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := effects.CreateEffectForRunV2(context.Background(), control.CreateRunEffectRequestV2{Partition: partition, ExpectedIndexRevision: 1, Effect: proposed}); err != nil {
			t.Fatal(err)
		}
		partitions = append(partitions, partition)
	}
	for index, partition := range partitions {
		fact, err := effects.InspectRunEffectV2(context.Background(), partition, "effect-shared")
		if err != nil || fact.Intent.Scope.Identity.TenantID != scopes[index].Identity.TenantID {
			t.Fatalf("partitioned Effect lookup crossed tenants: fact=%+v err=%v", fact, err)
		}
		wrong := partitions[1-index]
		wrong.RunIdentityDigest = partition.RunIdentityDigest
		if _, err := effects.InspectRunEffectV2(context.Background(), wrong, "effect-shared"); err == nil {
			t.Fatal("wrong partition disclosed a different tenant Effect")
		}
	}

	partition := partitions[0]
	read, err := effects.InspectRunEffectIndexV2(context.Background(), partition)
	if err != nil {
		t.Fatal(err)
	}
	originalLease := *read.ExecutionScope.SandboxLease
	read.ExecutionScope.SandboxLease.ID = "mutated-by-reader"
	again, _ := effects.InspectRunEffectIndexV2(context.Background(), partition)
	if *again.ExecutionScope.SandboxLease != originalLease {
		t.Fatal("Run effect root leaked a mutable SandboxLease pointer")
	}

	current := again
	for number := 1; number < 10_000; number++ {
		intent := baseIntent
		intent.ID = core.EffectIntentID(fmt.Sprintf("effect-long-%05d", number))
		intent.Idempotency.Key = fmt.Sprintf("idem-long-%05d", number)
		proposed, err := control.NewProposedEffectFactV2(intent, now)
		if err != nil {
			t.Fatal(err)
		}
		result, err := effects.CreateEffectForRunV2(context.Background(), control.CreateRunEffectRequestV2{Partition: partition, ExpectedIndexRevision: current.Revision, Effect: proposed})
		if err != nil {
			t.Fatalf("long Run Effect %d failed: %v", number, err)
		}
		current = result.Index
	}
	if current.EffectCount != 10_000 || current.SegmentCount < 2 {
		t.Fatalf("long Run did not roll over bounded segments: effects=%d segments=%d", current.EffectCount, current.SegmentCount)
	}
	seen := uint64(0)
	after := uint64(0)
	for {
		page, err := effects.ListRunEffectSegmentsV2(context.Background(), partition, after, 17)
		if err != nil {
			t.Fatal(err)
		}
		for _, segment := range page.Segments {
			seen += uint64(len(segment.Effects))
			after = segment.Number
		}
		if page.NextNumber == 0 {
			break
		}
	}
	if seen != current.EffectCount {
		t.Fatalf("segmented enumeration lost Effects: got=%d want=%d", seen, current.EffectCount)
	}
}

func cloneTestExecutionScopeV2(scope core.ExecutionScope) core.ExecutionScope {
	copy := scope
	if scope.SandboxLease != nil {
		lease := *scope.SandboxLease
		copy.SandboxLease = &lease
	}
	return copy
}

func TestRunSettlementStoreV2AtomicCommitLostReplyAndFirstCASWins(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 14, 2, 0, 0, time.UTC)
	store := fakes.NewRunSettlementStoreV2(func() time.Time { return now })
	run := runningRecord(runScope(t), "run-settlement-commit", now)
	plan := runSettlementPlanFixtureV2(t, run, now)
	run = persistRunningRunBundleV2(t, store, run, plan)
	stopping := run
	stopping.Status, stopping.Revision = core.RunStopping, run.Revision+1
	if _, err := store.CompareAndSwapRun(context.Background(), control.RunFactCASRequest{ExpectedRevision: run.Revision, Next: stopping}); err != nil {
		t.Fatal(err)
	}
	closure := runSettlementClosureFixtureV2(t, stopping, plan, now)
	attemptResult, err := store.CreateRunSettlementClosureAttemptV2(context.Background(), closure)
	if err != nil {
		t.Fatal(err)
	}
	decision, progress := runSettlementDecisionFixtureV2(t, stopping, plan, closure, attemptResult.Pointer.Revision, core.OutcomeCompleted, now.Add(time.Second))
	for _, testCase := range []struct {
		name   string
		mutate func(*control.RunSettlementDecisionFactV2)
	}{
		{
			name: "execution_ref",
			mutate: func(next *control.RunSettlementDecisionFactV2) {
				next.Execution.Digest = runSettlementDigestV2(t, "forged-execution-ref")
			},
		},
		{
			name: "claim",
			mutate: func(next *control.RunSettlementDecisionFactV2) {
				claim := runClaimAssociationFactV2(t, "forged-settlement", 1, core.RunClaimCompleted)
				next.Claim = &claim
			},
		},
		{
			name: "outcome",
			mutate: func(next *control.RunSettlementDecisionFactV2) {
				next.Outcome = core.OutcomeFailed
			},
		},
		{
			name: "participant_resolution",
			mutate: func(next *control.RunSettlementDecisionFactV2) {
				for index := range next.Resolutions {
					if next.Resolutions[index].Participant != nil {
						ref := *next.Resolutions[index].Participant
						ref.Digest = runSettlementDigestV2(t, "forged-participant-ref")
						next.Resolutions[index].Participant = &ref
						return
					}
				}
			},
		},
		{
			name: "participant_disposition_and_outcome",
			mutate: func(next *control.RunSettlementDecisionFactV2) {
				for index := range next.Resolutions {
					if next.Resolutions[index].Participant != nil {
						next.Resolutions[index].Disposition = ports.RunSettlementConfirmedFailed
						next.Outcome = core.OutcomeNeedsReconciliation
						return
					}
				}
			},
		},
	} {
		t.Run("reject_forged_"+testCase.name, func(t *testing.T) {
			forged := decision
			forged.Resolutions = append([]control.RunSettlementResolutionV2{}, decision.Resolutions...)
			testCase.mutate(&forged)
			forgedProgress := progress
			forgedRef, refErr := forged.RefV2()
			if refErr != nil {
				// Structurally invalid forgeries are also required to fail closed.
				if _, err := store.CommitRunCompletionV2(context.Background(), control.CommitRunCompletionRequestV2{ExecutionScope: run.Scope, ExpectedRunRevision: stopping.Revision, Decision: forged, InitialProgress: forgedProgress}); err == nil {
					t.Fatal("structurally invalid forged Decision was accepted")
				}
				return
			}
			forgedProgress.Decision = forgedRef
			if _, err := store.CommitRunCompletionV2(context.Background(), control.CommitRunCompletionRequestV2{ExecutionScope: run.Scope, ExpectedRunRevision: stopping.Revision, Decision: forged, InitialProgress: forgedProgress}); err == nil {
				t.Fatal("raw FactPort accepted a forged Decision")
			}
			current, err := store.InspectRun(context.Background(), run.Scope, run.ID)
			if err != nil || current.Status != core.RunStopping {
				t.Fatalf("forged Decision mutated Run: %+v %v", current, err)
			}
		})
	}
	store.LoseNextCommitReply()
	if _, err := store.CommitRunCompletionV2(context.Background(), control.CommitRunCompletionRequestV2{ExecutionScope: run.Scope, ExpectedRunRevision: stopping.Revision, Decision: decision, InitialProgress: progress}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("expected lost atomic commit reply, got %v", err)
	}
	terminal, err := store.InspectRun(context.Background(), run.Scope, run.ID)
	if err != nil || terminal.Status != core.RunTerminal || terminal.Outcome != core.OutcomeCompleted {
		t.Fatalf("terminal Run did not atomically survive lost reply: %+v %v", terminal, err)
	}
	if _, err := store.InspectRunSettlementDecisionV2(context.Background(), run.Scope, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InspectRunTerminationProgressV2(context.Background(), run.Scope, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitRunCompletionV2(context.Background(), control.CommitRunCompletionRequestV2{ExecutionScope: run.Scope, ExpectedRunRevision: stopping.Revision, Decision: decision, InitialProgress: progress}); err != nil {
		t.Fatalf("exact atomic commit replay must be idempotent: %v", err)
	}
	expectedReport, err := control.BuildRunTerminationReportV2(terminal, decision, progress)
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		name   string
		mutate func(*control.RunTerminationReportV2)
	}{
		{"run_identity", func(report *control.RunTerminationReportV2) {
			report.RunIdentityDigest = runSettlementDigestV2(t, "forged-report-run")
		}},
		{"outcome", func(report *control.RunTerminationReportV2) { report.Outcome = core.OutcomeFailed }},
		{"progress", func(report *control.RunTerminationReportV2) {
			report.Progress.Digest = runSettlementDigestV2(t, "forged-report-progress")
		}},
		{"items", func(report *control.RunTerminationReportV2) {
			report.Items[0].Disposition = ports.RunSettlementConfirmedFailed
		}},
	} {
		forged := expectedReport
		forged.Items = append([]control.RunSettlementResolutionV2{}, expectedReport.Items...)
		testCase.mutate(&forged)
		if _, err := store.CreateRunTerminationReportV2(context.Background(), forged); err == nil {
			t.Fatalf("raw FactPort accepted forged termination Report field %s", testCase.name)
		}
	}
	if _, err := store.CreateRunTerminationReportV2(context.Background(), expectedReport); err != nil {
		t.Fatalf("Fact Owner rejected exact reconstructed termination Report: %v", err)
	}
}

func TestRunSettlementStoreV2ConcurrentDifferentDecisionsLinearizeOnce(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 14, 3, 0, 0, time.UTC)
	store := fakes.NewRunSettlementStoreV2(func() time.Time { return now })
	run := runningRecord(runScope(t), "run-settlement-race", now)
	plan := runSettlementPlanFixtureV2(t, run, now)
	run = persistRunningRunBundleV2(t, store, run, plan)
	stopping := run
	stopping.Status, stopping.Revision = core.RunStopping, run.Revision+1
	_, _ = store.CompareAndSwapRun(context.Background(), control.RunFactCASRequest{ExpectedRevision: run.Revision, Next: stopping})
	closure := runSettlementClosureFixtureV2(t, stopping, plan, now)
	attemptResult, _ := store.CreateRunSettlementClosureAttemptV2(context.Background(), closure)
	completed, completedProgress := runSettlementDecisionFixtureV2(t, stopping, plan, closure, attemptResult.Pointer.Revision, core.OutcomeCompleted, now.Add(time.Second))
	failed, failedProgress := runSettlementDecisionFixtureV2(t, stopping, plan, closure, attemptResult.Pointer.Revision, core.OutcomeFailed, now.Add(time.Second))
	var success atomic.Int32
	var conflict atomic.Int32
	var wg sync.WaitGroup
	for _, pair := range []struct {
		d control.RunSettlementDecisionFactV2
		p control.RunTerminationProgressFactV2
	}{{completed, completedProgress}, {failed, failedProgress}} {
		pair := pair
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.CommitRunCompletionV2(context.Background(), control.CommitRunCompletionRequestV2{ExecutionScope: run.Scope, ExpectedRunRevision: stopping.Revision, Decision: pair.d, InitialProgress: pair.p})
			if err == nil {
				success.Add(1)
			} else if core.HasReason(err, core.ReasonRunCompletionConflict) {
				conflict.Add(1)
			} else {
				t.Errorf("unexpected commit race result: %v", err)
			}
		}()
	}
	wg.Wait()
	if success.Load() != 1 || conflict.Load() != 1 {
		t.Fatalf("expected one immutable Decision winner: success=%d conflict=%d", success.Load(), conflict.Load())
	}
}

func TestRunSettlementStoreV2ClosureAttemptChainLostReplyAndStaleDecisionRejected(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 14, 4, 0, 0, time.UTC)
	store := fakes.NewRunSettlementStoreV2(func() time.Time { return now })
	run := runningRecord(runScope(t), "run-settlement-attempt", now)
	plan := runSettlementPlanFixtureV2(t, run, now)
	run = persistRunningRunBundleV2(t, store, run, plan)
	stopping := run
	stopping.Status, stopping.Revision = core.RunStopping, run.Revision+1
	if _, err := store.CompareAndSwapRun(context.Background(), control.RunFactCASRequest{ExpectedRevision: run.Revision, Next: stopping}); err != nil {
		t.Fatal(err)
	}
	first := runSettlementClosureFixtureV2(t, stopping, plan, now)
	firstResult, err := store.CreateRunSettlementClosureAttemptV2(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.Attempt = 2
	second.PreviousClosureDigest, _ = first.DigestV2()
	second.ID += "-attempt-2"
	second.CreatedUnixNano = now.Add(time.Second).UnixNano()
	store.LoseNextClosureReply()
	if _, err := store.CreateRunSettlementClosureAttemptV2(context.Background(), second); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("expected lost attempt reply: %v", err)
	}
	current, err := store.InspectCurrentRunSettlementClosureV2(context.Background(), run.Scope, run.ID)
	if err != nil || current.Closure.Attempt != 2 || current.Pointer.Revision != firstResult.Pointer.Revision+1 {
		t.Fatalf("attempt chain did not survive reply loss: %+v %v", current, err)
	}
	if replay, err := store.CreateRunSettlementClosureAttemptV2(context.Background(), second); err != nil || replay.Pointer != current.Pointer {
		t.Fatalf("attempt replay is not idempotent: %+v %v", replay, err)
	}
	staleDecision, staleProgress := runSettlementDecisionFixtureV2(t, stopping, plan, first, firstResult.Pointer.Revision, core.OutcomeCompleted, now.Add(2*time.Second))
	if _, err := store.CommitRunCompletionV2(context.Background(), control.CommitRunCompletionRequestV2{ExecutionScope: run.Scope, ExpectedRunRevision: stopping.Revision, Decision: staleDecision, InitialProgress: staleProgress}); !core.HasReason(err, core.ReasonRunSettlementClosureConflict) {
		t.Fatalf("old attempt Decision was accepted: %v", err)
	}
}

func runSettlementPlanFixtureV2(t *testing.T, run core.AgentRunRecord, now time.Time) ports.RunSettlementPlanFactV2 {
	t.Helper()
	runIdentity, _ := ports.RunIdentityDigestV2(run)
	scopeDigest, _ := ports.ExecutionScopeDigestV2(run.Scope)
	manifest := runSettlementDigestV2(t, "manifest")
	artifact := runSettlementDigestV2(t, "artifact")
	owner := ports.EvidenceProducerBindingRefV2{BindingSetID: "binding-set-run", BindingSetRevision: 1, ComponentID: "runtime/settlement-owner", ManifestDigest: manifest, ArtifactDigest: artifact, Capability: "runtime/settle-run"}
	schema := ports.SchemaRefV2{Namespace: "runtime", Name: "settlement", Version: "2.0.0", MediaType: "application/octet-stream", ContentDigest: runSettlementDigestV2(t, "schema")}
	kinds := []struct {
		kind  ports.NamespacedNameV2
		phase ports.RunSettlementRequirementPhaseV2
	}{{ports.RunRequirementExecutionTruth, ports.RunSettlementPhaseCompletion}, {ports.RunRequirementEffects, ports.RunSettlementPhaseCompletion}, {ports.RunRequirementRemoteContinuations, ports.RunSettlementPhaseCompletion}, {ports.RunRequirementDomainCommits, ports.RunSettlementPhaseCompletion}, {ports.RunRequirementBudget, ports.RunSettlementPhaseCompletion}, {ports.RunRequirementCleanup, ports.RunSettlementPhaseTerminationReport}, {ports.RunRequirementResidual, ports.RunSettlementPhaseTerminationReport}, {ports.RunRequirementProviderRetention, ports.RunSettlementPhaseTerminationReport}}
	requirements := make([]ports.RunSettlementRequirementV2, 0, len(kinds))
	for _, item := range kinds {
		id := item.kind
		requirements = append(requirements, ports.RunSettlementRequirementV2{ID: id, Kind: item.kind, Phase: item.phase, Owner: owner, Schema: schema, SubjectSelector: "runtime/run", SubjectDigest: runSettlementDigestV2(t, "subject-"+string(id)), EvidenceTrust: ports.EvidenceTrustAttestation, EvidenceKind: "runtime/settlement-attestation"})
	}
	execution := ports.RunExecutionSubjectV2{EndpointID: "endpoint-run", EndpointDigest: runSettlementDigestV2(t, "endpoint"), SessionRef: run.SessionRef, Binding: owner}
	execution.SubjectDigest, _ = execution.DigestV2()
	for index := range requirements {
		if requirements[index].Kind == ports.RunRequirementExecutionTruth {
			requirements[index].SubjectDigest = execution.SubjectDigest
		}
	}
	ports.SortRunSettlementRequirementsV2(requirements)
	plan := ports.RunSettlementPlanFactV2{ContractVersion: ports.RunSettlementContractVersionV2, ID: "settlement-plan-" + string(run.ID), Revision: 1, RunID: run.ID, RunIdentityDigest: runIdentity, ExecutionScope: run.Scope, ExecutionScopeDigest: scopeDigest, SessionRef: run.SessionRef, LineagePlanDigest: run.Scope.Lineage.PlanDigest, BindingSet: ports.RunBindingSetRefV2{ID: owner.BindingSetID, Revision: 1, Digest: runSettlementDigestV2(t, "binding-set"), SemanticDigest: runSettlementDigestV2(t, "binding-set-semantic")}, Execution: execution, Claim: ports.RunClaimRequirementV2{Mode: ports.RunClaimOptionalByPolicyV2}, Requirements: requirements, CreatedUnixNano: now.UnixNano()}
	for index := range plan.Requirements {
		policy := settlementPolicyFixtureV2(t, plan, plan.Requirements[index], now, false)
		plan.Requirements[index].Policy = ports.RunSettlementPolicyBindingRefV2{Ref: policy.Ref, Revision: policy.Revision, Digest: policy.Digest, SemanticDigest: policy.SemanticDigest}
	}
	claimRequirement := plan.Requirements[0]
	claimRequirement.ID = ports.RunRequirementClaimAssociation
	claimPolicy := settlementPolicyFixtureV2(t, plan, claimRequirement, now, true)
	plan.Claim.OmissionPolicy = &ports.RunSettlementPolicyBindingRefV2{Ref: claimPolicy.Ref, Revision: claimPolicy.Revision, Digest: claimPolicy.Digest, SemanticDigest: claimPolicy.SemanticDigest}
	return plan
}

func runSettlementClosureFixtureV2(t *testing.T, run core.AgentRunRecord, plan ports.RunSettlementPlanFactV2, now time.Time) control.RunSettlementClosureFactV2 {
	t.Helper()
	planRef, _ := plan.RefV2()
	evidence := ports.EvidenceRecordRefV2{LedgerScopeDigest: runSettlementDigestV2(t, "ledger"), Sequence: 1, RecordDigest: runSettlementDigestV2(t, "record")}
	execution := ports.ExecutionSettlementInspectionV2{ContractVersion: ports.RunSettlementContractVersionV2, ID: "inspection-" + string(run.ID), Revision: 1, RunID: run.ID, RunIdentityDigest: plan.RunIdentityDigest, RunRevision: run.Revision, ExecutionScope: run.Scope, ExecutionScopeDigest: plan.ExecutionScopeDigest, Subject: plan.Execution, Truth: ports.RunExecutionTerminalCompleted, SourceEpoch: run.Scope.Instance.Epoch, SourceSequence: 1, Evidence: evidence, InspectedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	execution.PayloadDigest, _ = execution.EvidenceSubjectDigestV2()
	participants := []control.RunSettlementClosureParticipantV2{}
	for _, requirement := range plan.Requirements {
		if requirement.Kind == ports.RunRequirementExecutionTruth || requirement.Kind == ports.RunRequirementEffects {
			continue
		}
		requirementDigest, _ := requirement.DigestV2()
		participantFact := ports.RunSettlementParticipantFactV2{
			ContractVersion:      ports.RunSettlementContractVersionV2,
			ID:                   "participant-" + string(requirement.ID[8:]),
			Revision:             1,
			RunID:                run.ID,
			RunIdentityDigest:    plan.RunIdentityDigest,
			ExecutionScope:       run.Scope,
			ExecutionScopeDigest: plan.ExecutionScopeDigest,
			Plan:                 planRef,
			RequirementID:        requirement.ID,
			RequirementDigest:    requirementDigest,
			SubjectDigest:        requirement.SubjectDigest,
			Owner:                requirement.Owner,
			Disposition:          ports.RunSettlementConfirmedSatisfied,
			Evidence:             []ports.EvidenceRecordRefV2{evidence},
			CreatedUnixNano:      now.UnixNano(),
			ExpiresUnixNano:      now.Add(time.Minute).UnixNano(),
		}
		participantRef, err := participantFact.RefV2()
		if err != nil {
			t.Fatal(err)
		}
		policy := settlementPolicyFixtureV2(t, plan, requirement, now, false)
		participants = append(participants, control.RunSettlementClosureParticipantV2{RequirementID: requirement.ID, RequirementDigest: requirementDigest, Participant: participantRef, ParticipantFact: participantFact, PolicyFact: policy})
	}
	sort.Slice(participants, func(i, j int) bool { return participants[i].RequirementID < participants[j].RequirementID })
	return control.RunSettlementClosureFactV2{ContractVersion: ports.RunSettlementContractVersionV2, ID: "closure-" + string(run.ID), Revision: 1, RunID: run.ID, RunIdentityDigest: plan.RunIdentityDigest, RunRevision: run.Revision, ExecutionScope: run.Scope, ExecutionScopeDigest: plan.ExecutionScopeDigest, Attempt: 1, PreviousClosureDigest: ports.EvidenceGenesisDigestV2, Plan: planRef, Execution: execution, EffectSet: control.RunEffectSetRefV2{IndexID: "effect-index-" + string(run.ID), Revision: 2, Digest: runSettlementDigestV2(t, "effect-set"), Watermark: 2, HeadSegmentDigest: ports.EvidenceGenesisDigestV2}, Participants: participants, CreatedUnixNano: now.UnixNano()}
}

func runSettlementDecisionFixtureV2(t *testing.T, run core.AgentRunRecord, plan ports.RunSettlementPlanFactV2, closure control.RunSettlementClosureFactV2, pointerRevision core.Revision, outcome core.ExecutionOutcome, now time.Time) (control.RunSettlementDecisionFactV2, control.RunTerminationProgressFactV2) {
	t.Helper()
	planRef, _ := plan.RefV2()
	closureRef, _ := closure.RefV2()
	executionRef, _ := closure.Execution.RefV2()
	completion := []control.RunSettlementResolutionV2{}
	termination := []control.RunSettlementResolutionV2{}
	for _, requirement := range plan.Requirements {
		resolution := control.RunSettlementResolutionV2{RequirementID: requirement.ID, Kind: requirement.Kind, Phase: requirement.Phase, Disposition: ports.RunSettlementConfirmedSatisfied, Policy: requirement.Policy, EvidenceDigest: runSettlementDigestV2(t, "resolution-"+string(requirement.ID))}
		switch requirement.Kind {
		case ports.RunRequirementExecutionTruth:
			resolution.EvidenceDigest = closure.Execution.PayloadDigest
		case ports.RunRequirementEffects:
			resolution.EvidenceDigest = closure.EffectSet.Digest
		default:
			for _, participant := range closure.Participants {
				if participant.RequirementID == requirement.ID {
					participantRef := participant.Participant
					resolution.Participant = &participantRef
					resolution.EvidenceDigest = participantRef.Digest
					break
				}
			}
		}
		if requirement.Phase == ports.RunSettlementPhaseCompletion {
			completion = append(completion, resolution)
		} else {
			termination = append(termination, resolution)
		}
	}
	decision := control.RunSettlementDecisionFactV2{ContractVersion: ports.RunSettlementContractVersionV2, ID: "decision-" + string(outcome) + "-" + string(run.ID), Revision: 1, RunID: run.ID, RunIdentityDigest: plan.RunIdentityDigest, ExpectedRunRevision: run.Revision, ExecutionScopeDigest: plan.ExecutionScopeDigest, Plan: planRef, Closure: closureRef, ClosurePointerRevision: pointerRevision, Execution: executionRef, Resolutions: completion, Outcome: outcome, CreatedUnixNano: now.UnixNano()}
	decisionRef, _ := decision.RefV2()
	progress := control.RunTerminationProgressFactV2{ContractVersion: ports.RunSettlementContractVersionV2, ID: "progress-" + string(outcome) + "-" + string(run.ID), Revision: 1, RunID: run.ID, ExecutionScope: run.Scope, ExecutionScopeDigest: plan.ExecutionScopeDigest, Decision: decisionRef, Items: termination, UpdatedUnixNano: now.UnixNano()}
	return decision, progress
}

func runSettlementDigestV2(t *testing.T, value any) core.Digest {
	t.Helper()
	digest, err := core.DigestJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

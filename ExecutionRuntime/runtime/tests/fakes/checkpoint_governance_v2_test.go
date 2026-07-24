package fakes_test

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestCheckpointGovernanceV2AtomicCreateLostReplyAndConformance(t *testing.T) {
	fixture := newCheckpointFixtureV2(t, "atomic")
	fixture.store.FailNextCheckpointAtomicStageV2()
	if _, err := fixture.gateway.CreateCheckpointAttemptV2(context.Background(), fixture.create); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("staged failure should remain indeterminate after exact NotFound inspect: %v", err)
	}
	if _, err := fixture.store.InspectCheckpointAttemptBundleV2(context.Background(), ports.InspectCheckpointAttemptRequestV2{TenantID: fixture.create.Scope.Identity.TenantID, AttemptID: fixture.create.AttemptID}); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("atomic staged failure published a half bundle: %v", err)
	}
	fixture.store.LoseNextCheckpointReplyV2()
	bundle, err := fixture.gateway.CreateCheckpointAttemptV2(context.Background(), fixture.create)
	if err != nil {
		t.Fatal(err)
	}
	if err := bundle.Validate(); err != nil {
		t.Fatal(err)
	}
	report, err := conformance.RunCheckpointGovernanceConformanceV2(context.Background(), fixture.gateway, conformance.CheckpointGovernanceConformanceFixtureV2{Create: fixture.create})
	if err != nil {
		t.Fatal(err)
	}
	if !report.AtomicAttemptBarrier || !report.HistoricalInspectExact || !report.TerminalCurrentIsSeparate || report.ProviderCalls != 0 || report.ProductionClaimEligible {
		t.Fatalf("unexpected checkpoint conformance report: %#v", report)
	}
}

func TestCheckpointGovernanceV2ConcurrentCreateSameAndChangedContent(t *testing.T) {
	fixture := newCheckpointFixtureV2(t, "concurrent")
	const workers = 64
	var success atomic.Int64
	var conflicts atomic.Int64
	var wg sync.WaitGroup
	for index := 0; index < workers; index++ {
		wg.Add(1)
		go func(changed bool) {
			defer wg.Done()
			request := fixture.create
			if changed {
				request.BarrierID = "checkpoint-barrier-conflicting"
			}
			_, err := fixture.gateway.CreateCheckpointAttemptV2(context.Background(), request)
			switch {
			case err == nil:
				success.Add(1)
			case core.HasCategory(err, core.ErrorConflict):
				conflicts.Add(1)
			default:
				t.Errorf("unexpected concurrent create error: %v", err)
			}
		}(index%2 == 1)
	}
	wg.Wait()
	if success.Load() != workers/2 || conflicts.Load() != workers/2 {
		t.Fatalf("same request must be idempotent and changed content must conflict: success=%d conflict=%d", success.Load(), conflicts.Load())
	}
}

func TestCheckpointGovernanceV2AttemptTTLUsesEarliestCurrentOwnerExpiry(t *testing.T) {
	fixture := newCheckpointFixtureV2(t, "owner-ttl")
	fixture.currents.inputTTL = 2 * time.Minute
	bundle, err := fixture.gateway.CreateCheckpointAttemptV2(context.Background(), fixture.create)
	if err != nil {
		t.Fatal(err)
	}
	want := fixture.currents.now.Add(2 * time.Minute).UnixNano()
	if bundle.Barrier.ExpiresUnixNano != want || bundle.Attempt.ReconciliationDeadlineUnixNano != want {
		t.Fatalf("Owner minimum TTL was not frozen exactly: expires=%d deadline=%d want=%d", bundle.Barrier.ExpiresUnixNano, bundle.Attempt.ReconciliationDeadlineUnixNano, want)
	}
}

func TestCheckpointGovernanceV2CreateRequiresExactRunCurrentRevisionAndDigest(t *testing.T) {
	for name, mutate := range map[string]func(*ports.CheckpointRunCurrentProjectionV2){
		"revision": func(value *ports.CheckpointRunCurrentProjectionV2) { value.Revision++ },
		"digest": func(value *ports.CheckpointRunCurrentProjectionV2) {
			value.RunStableIdentityDigest = digestCheckpointV2("wrong-run-stable-identity")
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newCheckpointFixtureV2(t, "run-current-"+name)
			fixture.gateway.Runs = checkpointRunCurrentDriftReaderV2{base: fixture.currents, mutate: mutate}
			if _, err := fixture.gateway.CreateCheckpointAttemptV2(context.Background(), fixture.create); err == nil {
				t.Fatal("wrong Run current projection reached Attempt create")
			}
			if _, err := fixture.store.InspectCheckpointAttemptBundleV2(context.Background(), ports.InspectCheckpointAttemptRequestV2{TenantID: fixture.create.Scope.Identity.TenantID, AttemptID: fixture.create.AttemptID}); !core.HasCategory(err, core.ErrorNotFound) {
				t.Fatalf("wrong Run current projection wrote an Attempt: %v", err)
			}
		})
	}
}

type checkpointRunCurrentDriftReaderV2 struct {
	base   *checkpointCurrentOwnersV2
	mutate func(*ports.CheckpointRunCurrentProjectionV2)
}

func (r checkpointRunCurrentDriftReaderV2) InspectCheckpointRunCurrentV2(ctx context.Context, scope core.ExecutionScope, runID core.AgentRunID) (ports.CheckpointRunCurrentProjectionV2, error) {
	projection, err := r.base.InspectCheckpointRunCurrentV2(ctx, scope, runID)
	if err != nil {
		return projection, err
	}
	r.mutate(&projection)
	projection.ProjectionDigest = ""
	return ports.SealCheckpointRunCurrentProjectionV2(projection, r.base.now)
}

func TestCheckpointGovernanceV2CreateLostReplyAcceptsOnlyExactProgressedSuccessorCKP2R01(t *testing.T) {
	fixture := newCheckpointFixtureV2(t, "create-progressed-r01")
	initial, err := fixture.gateway.CreateCheckpointAttemptV2(context.Background(), fixture.create)
	if err != nil {
		t.Fatal(err)
	}
	fixture.currents.effectRoot = digestCheckpointV2("create-progressed-r01-root")
	if _, err := fixture.gateway.FreezeCheckpointEffectCutV2(context.Background(), ports.FreezeCheckpointEffectCutRequestV2{Attempt: initial.Attempt.RefV2(), Barrier: initial.Barrier.RefV2(), ExpectedAttemptRevision: initial.Attempt.Revision, ExpectedBarrierRevision: initial.Barrier.Revision, EffectInventoryRoot: fixture.currents.effectRoot, EffectInventoryWatermark: 1, ExpectedEffectCount: 0, IdempotencyKey: "freeze-create-progressed-r01"}); err != nil {
		t.Fatal(err)
	}
	fixture.gateway.Facts = checkpointCreateReplyLostPortV2{CheckpointFactPortV2: fixture.store}
	recovered, err := fixture.gateway.CreateCheckpointAttemptV2(context.Background(), fixture.create)
	if err != nil || recovered.Attempt.RefV2() != initial.Attempt.RefV2() || recovered.Barrier.RefV2() != initial.Barrier.RefV2() {
		t.Fatalf("lost create reply did not recover exact immutable initial bundle from legal successor: %+v err=%v", recovered, err)
	}

	fixture.gateway.Facts = checkpointCreateReplyLostPortV2{CheckpointFactPortV2: fixture.store, driftCurrent: true}
	if _, err := fixture.gateway.CreateCheckpointAttemptV2(context.Background(), fixture.create); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("progressed create recovery accepted ABA/immutable identity drift: %v", err)
	}

	fixture.gateway.Facts = checkpointCreateReplyLostPortV2{CheckpointFactPortV2: fixture.store, illegalLineage: true}
	if _, err := fixture.gateway.CreateCheckpointAttemptV2(context.Background(), fixture.create); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("progressed create recovery accepted an unproven transition lineage: %v", err)
	}
}

type checkpointCreateReplyLostPortV2 struct {
	ports.CheckpointFactPortV2
	driftCurrent   bool
	illegalLineage bool
}

func (p checkpointCreateReplyLostPortV2) InspectCheckpointAttemptLineageV2(ctx context.Context, request ports.InspectCheckpointAttemptLineageRequestV2) (ports.CheckpointAttemptLineageV2, error) {
	lineage, err := p.CheckpointFactPortV2.InspectCheckpointAttemptLineageV2(ctx, request)
	if err == nil && p.illegalLineage && len(lineage.Attempts) > 1 {
		lineage.Attempts = lineage.Attempts[1:]
	}
	return lineage, err
}

func (p checkpointCreateReplyLostPortV2) CreateCheckpointAttemptBundleV2(context.Context, ports.CheckpointAttemptBarrierBundleV2) (ports.CheckpointAttemptBarrierBundleV2, error) {
	return ports.CheckpointAttemptBarrierBundleV2{}, core.NewError(core.ErrorUnavailable, core.ReasonCheckpointInconsistent, "injected lost create reply")
}

func (p checkpointCreateReplyLostPortV2) InspectCheckpointAttemptBundleV2(ctx context.Context, request ports.InspectCheckpointAttemptRequestV2) (ports.CheckpointAttemptBarrierBundleV2, error) {
	bundle, err := p.CheckpointFactPortV2.InspectCheckpointAttemptBundleV2(ctx, request)
	if err != nil || !p.driftCurrent {
		return bundle, err
	}
	bundle.Attempt.Workflow.ID += "-aba"
	bundle.Attempt.Workflow.Digest = digestCheckpointV2("workflow-aba")
	bundle.Attempt, err = ports.SealCheckpointAttemptFactV2(bundle.Attempt)
	return bundle, err
}

func TestCheckpointParticipantBranchGuardV2CommitAbortRaceLinearizesOneBranch(t *testing.T) {
	now := time.Unix(1_780_000_000, 0).UTC()
	fixture := newCheckpointFixtureV2(t, "branch-race")
	bundle, err := fixture.gateway.CreateCheckpointAttemptV2(context.Background(), fixture.create)
	if err != nil {
		t.Fatal(err)
	}
	operationFixture := newOperationSettlementFixtureV4(t, "checkpoint-branch-race")
	operation := operationFixture.submission.Operation
	operationDigest := operationFixture.submission.OperationDigest
	participant := ports.CheckpointParticipantRefV2{
		ID:     "checkpoint-branch-participant",
		Owner:  checkpointSettlementOwnerBindingV5(t, operationFixture.effect.effect.intent),
		Digest: digestCheckpointV2("checkpoint-branch-participant"),
	}
	commit := checkpointParticipantPhaseClosureV2(t, bundle.Attempt.RefV2(), participant, operationFixture, operation, operationDigest, ports.CheckpointPhaseCommitV2, ports.CheckpointParticipantCommittedV2, "branch-commit")
	abort := checkpointParticipantPhaseClosureV2(t, bundle.Attempt.RefV2(), participant, operationFixture, operation, operationDigest, ports.CheckpointPhaseAbortV2, ports.CheckpointParticipantAbortedV2, "branch-abort")
	store := fakes.NewCheckpointParticipantBranchStoreV2()
	requests := []ports.SelectCheckpointParticipantBranchRequestV2{
		{Attempt: bundle.Attempt.RefV2(), Participant: participant, Terminal: commit, SelectedAt: now.UnixNano()},
		{Attempt: bundle.Attempt.RefV2(), Participant: participant, Terminal: abort, SelectedAt: now.UnixNano()},
	}
	var successes atomic.Int64
	var conflicts atomic.Int64
	var winner atomic.Value
	var wait sync.WaitGroup
	for index := 0; index < 64; index++ {
		request := requests[index%len(requests)]
		wait.Add(1)
		go func() {
			defer wait.Done()
			fact, selectErr := store.SelectCheckpointParticipantBranchV2(context.Background(), request)
			if selectErr == nil {
				successes.Add(1)
				winner.Store(fact.Ref)
				return
			}
			if core.HasCategory(selectErr, core.ErrorConflict) {
				conflicts.Add(1)
				return
			}
			t.Errorf("unexpected branch selection error: %v", selectErr)
		}()
	}
	wait.Wait()
	if successes.Load() == 0 || conflicts.Load() == 0 {
		t.Fatalf("one branch must win and the opposite branch must conflict: success=%d conflict=%d", successes.Load(), conflicts.Load())
	}
	selected := winner.Load().(ports.CheckpointParticipantBranchGuardRefV2)
	fact, err := store.InspectCheckpointParticipantBranchV2(context.Background(), selected)
	if err != nil || fact.Ref != selected {
		t.Fatalf("selected branch must be exactly inspectable: fact=%+v err=%v", fact, err)
	}
	losing := requests[0]
	if selected.SelectedPhase == ports.CheckpointPhaseCommitV2 {
		losing = requests[1]
	}
	if _, err := store.SelectCheckpointParticipantBranchV2(context.Background(), losing); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("opposite branch must remain permanently rejected: %v", err)
	}
}

func TestCheckpointParticipantBranchAndCurrentProjectionRejectCrossAttemptSplice(t *testing.T) {
	now := time.Unix(1_780_000_000, 0).UTC()
	fixture := newCheckpointFixtureV2(t, "branch-attempt-splice")
	bundle, err := fixture.gateway.CreateCheckpointAttemptV2(context.Background(), fixture.create)
	if err != nil {
		t.Fatal(err)
	}
	participant := ports.CheckpointParticipantRefV2{ID: "checkpoint-splice-participant", Owner: checkpointOwnerBindingV2("splice"), Digest: digestCheckpointV2("checkpoint-splice-participant")}
	closure, guard, err := fakes.BuildCommittedCheckpointParticipantClosureV2(fixture.create.Scope, fixture.create.RunID, bundle.Attempt.RefV2(), bundle.Barrier.RefV2(), ports.EffectCutRefV2{ID: "splice-cut", Revision: 1, Attempt: bundle.Attempt.RefV2(), RootDigest: digestCheckpointV2("splice-root"), Watermark: 1, Digest: digestCheckpointV2("splice-cut")}, participant, "attempt-splice", now)
	if err != nil {
		t.Fatal(err)
	}
	otherAttempt := bundle.Attempt.RefV2()
	otherAttempt.ID = "another-checkpoint-attempt"
	otherAttempt.Digest = digestCheckpointV2("another-checkpoint-attempt")
	if _, err := fakes.NewCheckpointParticipantBranchStoreV2().SelectCheckpointParticipantBranchV2(context.Background(), ports.SelectCheckpointParticipantBranchRequestV2{Attempt: otherAttempt, Participant: participant, Terminal: *closure.Terminal, SelectedAt: now.UnixNano()}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("cross-Attempt terminal branch was accepted: %v", err)
	}
	projection := ports.CheckpointParticipantClosureCurrentProjectionV2{ContractVersion: ports.CheckpointGovernanceContractVersionV2, Attempt: otherAttempt, Participant: participant, Closure: closure, BranchGuard: guard, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	projection.ProjectionDigest, _ = projection.DigestV2()
	if err := projection.Validate(now); err == nil {
		t.Fatal("cross-Attempt closure current projection was accepted")
	}
}

func TestCheckpointGovernanceV2FreezeFinalizeAndTerminalCurrent(t *testing.T) {
	fixture := newCheckpointFixtureV2(t, "finalize")
	bundle, err := fixture.gateway.CreateCheckpointAttemptV2(context.Background(), fixture.create)
	if err != nil {
		t.Fatal(err)
	}
	fixture.currents.effectRoot = digestCheckpointV2("inventory")
	cut, err := fixture.gateway.FreezeCheckpointEffectCutV2(context.Background(), ports.FreezeCheckpointEffectCutRequestV2{
		Attempt: bundle.Attempt.RefV2(), Barrier: bundle.Barrier.RefV2(), ExpectedAttemptRevision: bundle.Attempt.Revision,
		ExpectedBarrierRevision: bundle.Barrier.Revision, EffectInventoryRoot: digestCheckpointV2("inventory"),
		EffectInventoryWatermark: 1, ExpectedEffectCount: 0, IdempotencyKey: "freeze-finalize",
	})
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := fixture.gateway.PrepareCheckpointFinalizationInputsV2(context.Background(), ports.PrepareCheckpointFinalizationInputsRequestV2{
		Attempt: cut.Attempt.RefV2(), Barrier: bundle.Barrier.RefV2(), EffectCut: cut.Cut.Ref,
		ExpectedAttemptRevision: cut.Attempt.Revision, ExpectedBarrierRevision: bundle.Barrier.Revision,
		IdempotencyKey: "prepare-finalize",
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err := fixture.gateway.InspectCheckpointAttemptV2(context.Background(), ports.InspectCheckpointAttemptRequestV2{TenantID: bundle.Attempt.TenantID, AttemptID: bundle.Attempt.ID})
	if err != nil {
		t.Fatal(err)
	}
	finalizeRequest := ports.FinalizeCheckpointAttemptRequestV2{
		Attempt: current.Attempt.RefV2(), Barrier: current.Barrier.RefV2(), ExpectedAttemptRevision: current.Attempt.Revision,
		ExpectedBarrierRevision: current.Barrier.Revision, Inputs: inputs, IdempotencyKey: "finalize-attempt",
	}
	terminal, err := fixture.gateway.FinalizeCheckpointAttemptAndCloseBarrierV2(context.Background(), finalizeRequest)
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Attempt.State != ports.CheckpointAttemptIncompleteV2 || terminal.Barrier.State != ports.CheckpointBarrierClosedV2 {
		t.Fatalf("unexpected terminal checkpoint bundle: %#v", terminal)
	}
	projection, err := fixture.gateway.InspectCheckpointAttemptTerminalCurrentV2(context.Background(), terminal.Attempt.RefV2())
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.Validate(); err != nil {
		t.Fatal(err)
	}
	replayedTerminal, err := fixture.gateway.FinalizeCheckpointAttemptAndCloseBarrierV2(context.Background(), finalizeRequest)
	if err != nil || replayedTerminal.Attempt.RefV2() != terminal.Attempt.RefV2() || replayedTerminal.Barrier.RefV2() != terminal.Barrier.RefV2() {
		t.Fatalf("terminal Finalization replay did not return the persisted successor: bundle=%+v err=%v", replayedTerminal, err)
	}
	changedFinalize := finalizeRequest
	changedFinalize.Inputs.ID += "-changed"
	changedFinalize.Inputs.Digest = ""
	changedFinalize.Inputs.Digest, err = changedFinalize.Inputs.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.gateway.FinalizeCheckpointAttemptAndCloseBarrierV2(context.Background(), changedFinalize); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("terminal Finalization accepted changed history: %v", err)
	}
	historicalAttempt, err := fixture.gateway.InspectCheckpointAttemptHistoricalV2(context.Background(), bundle.Attempt.RefV2())
	if err != nil || historicalAttempt.RefV2() != bundle.Attempt.RefV2() || historicalAttempt.State != ports.CheckpointAttemptBarrierAcquiredV2 {
		t.Fatalf("append-only Attempt history lost its initial revision: fact=%+v err=%v", historicalAttempt, err)
	}
	historicalBarrier, err := fixture.gateway.InspectCheckpointBarrierHistoricalV2(context.Background(), bundle.Barrier.RefV2())
	if err != nil || historicalBarrier.RefV2() != bundle.Barrier.RefV2() || historicalBarrier.State != ports.CheckpointBarrierActiveV2 {
		t.Fatalf("append-only Barrier history lost its active revision: fact=%+v err=%v", historicalBarrier, err)
	}
	replayedCreate, err := fixture.gateway.CreateCheckpointAttemptV2(context.Background(), fixture.create)
	if err != nil || replayedCreate.Attempt.RefV2() != bundle.Attempt.RefV2() || replayedCreate.Barrier.RefV2() != bundle.Barrier.RefV2() {
		t.Fatalf("progressed terminal Attempt did not recover the immutable create result: bundle=%+v err=%v", replayedCreate, err)
	}
	changedCreate := fixture.create
	changedCreate.BarrierID += "-changed"
	if _, err := fixture.gateway.CreateCheckpointAttemptV2(context.Background(), changedCreate); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("progressed Attempt accepted changed create content: %v", err)
	}
	fixture.diagnostics.drift.Store(true)
	if _, err := fixture.gateway.InspectCheckpointAttemptTerminalCurrentV2(context.Background(), terminal.Attempt.RefV2()); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("historical terminal should become current-invisible after Owner Seal drift: %v", err)
	}
	historical, err := fixture.gateway.InspectCheckpointAttemptV2(context.Background(), ports.InspectCheckpointAttemptRequestV2{TenantID: terminal.Attempt.TenantID, AttemptID: terminal.Attempt.ID})
	if err != nil || historical.Attempt.State != ports.CheckpointAttemptIncompleteV2 {
		t.Fatalf("historical terminal must remain inspectable: state=%s err=%v", historical.Attempt.State, err)
	}
}

func TestCheckpointGovernanceV2TypedFinalizationClassificationDeterminesTerminalState(t *testing.T) {
	cases := []struct {
		name            string
		classification  *ports.CheckpointFinalizationClassificationEntryV2
		allowNotApplied bool
		advanceDeadline bool
		wantState       ports.CheckpointAttemptStateV2
		wantBeforeError bool
	}{
		{name: "empty-is-incomplete", wantState: ports.CheckpointAttemptIncompleteV2},
		{name: "confirmed-not-applied-policy-denied", classification: checkpointFinalizationClassificationV2("not-applied-denied", ports.CheckpointClassificationConfirmedNotAppliedV2), wantState: ports.CheckpointAttemptIncompleteV2},
		{name: "confirmed-not-applied-policy-allowed", classification: checkpointFinalizationClassificationV2("not-applied-allowed", ports.CheckpointClassificationConfirmedNotAppliedV2), allowNotApplied: true, wantState: ports.CheckpointAttemptAbortedV2},
		{name: "unknown-before-deadline", classification: checkpointFinalizationClassificationV2("unknown-before", ports.CheckpointClassificationUnknownV2), wantBeforeError: true},
		{name: "unknown-at-deadline", classification: checkpointFinalizationClassificationV2("unknown-deadline", ports.CheckpointClassificationUnknownV2), advanceDeadline: true, wantState: ports.CheckpointAttemptIndeterminateV2},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newCheckpointFixtureV2(t, "classification-"+testCase.name)
			if testCase.classification != nil {
				fixture.diagnostics.classifications = []ports.CheckpointFinalizationClassificationEntryV2{*testCase.classification}
			}
			if testCase.allowNotApplied {
				projection := fixture.policy.projection
				projection.AllowConfirmedNotAppliedAbort = true
				projection.ProjectionDigest = ""
				sealed, err := ports.SealCheckpointBarrierPolicyCurrentProjectionV2(projection, fixture.currents.now)
				if err != nil {
					t.Fatal(err)
				}
				fixture.policy = checkpointPolicyReaderV2{projection: sealed}
				fixture.gateway.Policies = fixture.policy
			}
			bundle, inputs := prepareCheckpointFinalizationFixtureV2(t, &fixture, "classification-"+testCase.name)
			// Non-success closure is governed only by the semantics frozen on the
			// Attempt. A subsequently unavailable/revoked Policy cannot deadlock
			// deadline terminalization.
			fixture.gateway.Policies = nil
			if testCase.advanceDeadline {
				atDeadline := time.Unix(0, bundle.Attempt.ReconciliationDeadlineUnixNano)
				fixture.gateway.Clock = func() time.Time { return atDeadline }
			}
			terminal, err := fixture.gateway.FinalizeCheckpointAttemptAndCloseBarrierV2(context.Background(), ports.FinalizeCheckpointAttemptRequestV2{
				Attempt: bundle.Attempt.RefV2(), Barrier: bundle.Barrier.RefV2(), ExpectedAttemptRevision: bundle.Attempt.Revision,
				ExpectedBarrierRevision: bundle.Barrier.Revision, Inputs: inputs, IdempotencyKey: "finalize-classification-" + testCase.name,
			})
			if testCase.wantBeforeError {
				if !core.HasCategory(err, core.ErrorPreconditionFailed) {
					t.Fatalf("unknown before deadline must remain inspect-only: %v", err)
				}
				current, inspectErr := fixture.gateway.InspectCheckpointAttemptV2(context.Background(), ports.InspectCheckpointAttemptRequestV2{TenantID: bundle.Attempt.TenantID, AttemptID: bundle.Attempt.ID})
				if inspectErr != nil || terminalCheckpointStateForTestV2(current.Attempt.State) {
					t.Fatalf("unknown before deadline mutated terminal state: state=%s err=%v", current.Attempt.State, inspectErr)
				}
				return
			}
			if err != nil || terminal.Attempt.State != testCase.wantState {
				t.Fatalf("typed classification chose wrong terminal state: got=%s want=%s err=%v", terminal.Attempt.State, testCase.wantState, err)
			}
		})
	}
}

func TestCheckpointGovernanceV2TerminalCurrentRederivesTypedNonSuccessState(t *testing.T) {
	fixture := newCheckpointFixtureV2(t, "terminal-current-derived")
	bundle, inputs := prepareCheckpointFinalizationFixtureV2(t, &fixture, "terminal-current-derived")
	terminal, err := fixture.gateway.FinalizeCheckpointAttemptAndCloseBarrierV2(context.Background(), ports.FinalizeCheckpointAttemptRequestV2{Attempt: bundle.Attempt.RefV2(), Barrier: bundle.Barrier.RefV2(), ExpectedAttemptRevision: bundle.Attempt.Revision, ExpectedBarrierRevision: bundle.Barrier.Revision, Inputs: inputs, IdempotencyKey: "finalize-terminal-current-derived"})
	if err != nil || terminal.Attempt.State != ports.CheckpointAttemptIncompleteV2 {
		t.Fatalf("fixture did not create incomplete terminal: %+v err=%v", terminal, err)
	}
	owner := checkpointTerminalStateDriftPortV2{CheckpointFactPortV2: fixture.store}
	mutated, err := owner.InspectCheckpointAttemptBundleV2(context.Background(), ports.InspectCheckpointAttemptRequestV2{TenantID: terminal.Attempt.TenantID, AttemptID: terminal.Attempt.ID})
	if err != nil {
		t.Fatal(err)
	}
	fixture.gateway.Facts = owner
	if _, err := fixture.gateway.InspectCheckpointAttemptTerminalCurrentV2(context.Background(), mutated.Attempt.RefV2()); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("typed classification mismatch remained terminal-current visible: %v", err)
	}
}

type checkpointTerminalStateDriftPortV2 struct{ ports.CheckpointFactPortV2 }

func (p checkpointTerminalStateDriftPortV2) InspectCheckpointAttemptBundleV2(ctx context.Context, request ports.InspectCheckpointAttemptRequestV2) (ports.CheckpointAttemptBarrierBundleV2, error) {
	bundle, err := p.CheckpointFactPortV2.InspectCheckpointAttemptBundleV2(ctx, request)
	if err != nil {
		return bundle, err
	}
	bundle.Attempt.State = ports.CheckpointAttemptAbortedV2
	bundle.Attempt, err = ports.SealCheckpointAttemptFactV2(bundle.Attempt)
	return bundle, err
}

func TestCheckpointGovernanceV2ConsistencyRereadsFrozenEffectInventory(t *testing.T) {
	fixture := newCheckpointFixtureV2(t, "consistency-effect-reread")
	bundle, err := fixture.gateway.CreateCheckpointAttemptV2(context.Background(), fixture.create)
	if err != nil {
		t.Fatal(err)
	}
	fixture.currents.effectRoot = digestCheckpointV2("consistency-effect-reread-root")
	cut, err := fixture.gateway.FreezeCheckpointEffectCutV2(context.Background(), ports.FreezeCheckpointEffectCutRequestV2{Attempt: bundle.Attempt.RefV2(), Barrier: bundle.Barrier.RefV2(), ExpectedAttemptRevision: bundle.Attempt.Revision, ExpectedBarrierRevision: bundle.Barrier.Revision, EffectInventoryRoot: fixture.currents.effectRoot, EffectInventoryWatermark: 1, ExpectedEffectCount: 0, IdempotencyKey: "freeze-consistency-effect-reread"})
	if err != nil {
		t.Fatal(err)
	}
	closure := checkpointCommittedParticipantClosureV2(t, cut.Attempt.RefV2(), "consistency-effect-reread")
	branch, err := fixture.branches.SelectCheckpointParticipantBranchV2(context.Background(), ports.SelectCheckpointParticipantBranchRequestV2{Attempt: cut.Attempt.RefV2(), Participant: closure.Participant, Terminal: *closure.Terminal, SelectedAt: fixture.currents.now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	fixture.currents.branches[closure.Participant.ID] = branch.Ref
	fixture.currents.closures = []ports.CheckpointParticipantClosureRefV2{closure}
	fixture.currents.participantRoot = digestCheckpointV2("participants-consistency-effect-reread")
	manifest := checkpointManifestProjectionV2(t, cut.Attempt, bundle.Barrier, cut.Cut, fixture.currents.closures)
	fixture.gateway.Manifests = &checkpointManifestReaderV2{projection: manifest}
	fixture.gateway.Effects = &checkpointEffectInventoryDriftReaderV2{base: fixture.currents}
	request := ports.CommitCheckpointConsistencyRequestV2{Attempt: cut.Attempt.RefV2(), Barrier: bundle.Barrier.RefV2(), ExpectedAttemptRevision: cut.Attempt.Revision, ExpectedBarrierRevision: bundle.Barrier.Revision, EffectCut: cut.Cut.Ref, ManifestSeal: manifest.Ref, ExpectedParticipantRoot: fixture.currents.participantRoot, ExpectedParticipantWatermark: 1, ExpectedParticipantCount: 1, IdempotencyKey: "commit-consistency-effect-reread"}
	if _, err := fixture.gateway.CommitCheckpointConsistencyAndCloseBarrierV2(context.Background(), request); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("post-Freeze Effect inventory drift must fail before final CAS: %v", err)
	}
	current, err := fixture.gateway.InspectCheckpointAttemptV2(context.Background(), ports.InspectCheckpointAttemptRequestV2{TenantID: cut.Attempt.TenantID, AttemptID: cut.Attempt.ID})
	if err != nil || current.Attempt.State != ports.CheckpointAttemptCutFrozenV2 || current.Barrier.State != ports.CheckpointBarrierActiveV2 || current.Attempt.Consistency != nil {
		t.Fatalf("Effect inventory drift published Consistency or closed Barrier: %+v err=%v", current, err)
	}
}

func TestCheckpointEffectCutV4TerminalBindsExactDispatchAttempt(t *testing.T) {
	fixture := newOperationSettlementFixtureV4(t, "checkpoint-cut-v4-attempt")
	settlement, err := fixture.gateway.SettleOperationV4(context.Background(), fixture.submission)
	if err != nil {
		t.Fatal(err)
	}
	attempt := fixture.submission.DomainResult.Attempt
	base := ports.EffectCutEntryV2{EffectID: attempt.EffectID, IntentRevision: attempt.IntentRevision, IntentDigest: attempt.IntentDigest, Attempt: attempt, Phase: "checkpoint-prepare", Disposition: ports.EffectCutSettledV2, Terminal: ports.RuntimeOperationTerminalRefV2{Kind: ports.RuntimeTerminalOperationSettlementV4V2, OperationSettlementV4: &settlement}}
	if err := base.Validate(); err != nil {
		t.Fatalf("exact V4 terminal fixture is invalid: %v", err)
	}
	mutations := map[string]func(*ports.OperationDispatchAttemptRefV3){
		"intent-digest": func(value *ports.OperationDispatchAttemptRefV3) {
			value.IntentDigest = digestCheckpointV2("v4-spliced-intent")
		},
		"permit-id": func(value *ports.OperationDispatchAttemptRefV3) { value.PermitID += "-spliced" },
		"permit-revision": func(value *ports.OperationDispatchAttemptRefV3) {
			value.PermitRevision++
		},
		"permit-digest": func(value *ports.OperationDispatchAttemptRefV3) {
			value.PermitDigest = digestCheckpointV2("v4-spliced-permit")
		},
		"attempt-id": func(value *ports.OperationDispatchAttemptRefV3) { value.AttemptID += "-spliced" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			spliced := base
			terminal := settlement
			mutate(&terminal.DomainResult.Attempt)
			spliced.Terminal.OperationSettlementV4 = &terminal
			if err := spliced.Terminal.Validate(); err != nil {
				t.Fatalf("spliced terminal must remain structurally valid: %v", err)
			}
			if err := spliced.Validate(); !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("V4 terminal Attempt splice was accepted: %v", err)
			}
		})
	}
}

func TestCheckpointGovernanceV2ConsistencyRejectsAttemptBarrierSpliceAndWrongOwnerReturn(t *testing.T) {
	for name, mutate := range map[string]func(*ports.CheckpointEffectInventoryCurrentProjectionV2){
		"attempt-revision": func(value *ports.CheckpointEffectInventoryCurrentProjectionV2) {
			value.Attempt.Revision++
			value.Attempt.Digest = digestCheckpointV2("spliced-attempt-revision")
		},
		"attempt-digest": func(value *ports.CheckpointEffectInventoryCurrentProjectionV2) {
			value.Attempt.Digest = digestCheckpointV2("spliced-attempt-digest")
		},
		"barrier-revision": func(value *ports.CheckpointEffectInventoryCurrentProjectionV2) {
			value.Barrier.Revision++
			value.Barrier.Digest = digestCheckpointV2("spliced-barrier-revision")
		},
		"barrier-digest": func(value *ports.CheckpointEffectInventoryCurrentProjectionV2) {
			value.Barrier.Digest = digestCheckpointV2("spliced-barrier-digest")
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture, request := prepareCheckpointConsistencyCommitV2(t, "inventory-splice-"+name)
			fixture.gateway.Effects = checkpointEffectInventorySpliceReaderV2{base: fixture.currents, mutate: mutate}
			if _, err := fixture.gateway.CommitCheckpointConsistencyAndCloseBarrierV2(context.Background(), request); !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("spliced current inventory reached Consistency commit: %v", err)
			}
			current, err := fixture.gateway.InspectCheckpointAttemptV2(context.Background(), ports.InspectCheckpointAttemptRequestV2{TenantID: request.Attempt.TenantID, AttemptID: request.Attempt.ID})
			if err != nil || current.Attempt.State != ports.CheckpointAttemptCutFrozenV2 || current.Attempt.Consistency != nil || current.Barrier.State != ports.CheckpointBarrierActiveV2 {
				t.Fatalf("spliced inventory changed terminal state: %+v err=%v", current, err)
			}
		})
	}

	for _, lostReply := range []bool{false, true} {
		name := "normal-return"
		if lostReply {
			name = "lost-reply-inspect"
		}
		t.Run(name, func(t *testing.T) {
			fixture, request := prepareCheckpointConsistencyCommitV2(t, "wrong-owner-"+name)
			fixture.gateway.Facts = checkpointConsistencyReturnDriftPortV2{CheckpointFactPortV2: fixture.store, lostReply: lostReply}
			if _, err := fixture.gateway.CommitCheckpointConsistencyAndCloseBarrierV2(context.Background(), request); err == nil {
				t.Fatal("non-canonical Consistency Owner return was accepted")
			}
		})
	}
}

type checkpointEffectInventorySpliceReaderV2 struct {
	base   *checkpointCurrentOwnersV2
	mutate func(*ports.CheckpointEffectInventoryCurrentProjectionV2)
}

func (r checkpointEffectInventorySpliceReaderV2) InspectCheckpointEffectInventoryCurrentV2(ctx context.Context, attempt ports.CheckpointAttemptRefV2, barrier ports.CheckpointBarrierLeaseRefV2) (ports.CheckpointEffectInventoryCurrentProjectionV2, error) {
	projection, err := r.base.InspectCheckpointEffectInventoryCurrentV2(ctx, attempt, barrier)
	if err != nil {
		return projection, err
	}
	r.mutate(&projection)
	projection.ProjectionDigest = ""
	return ports.SealCheckpointEffectInventoryCurrentProjectionV2(projection, r.base.now)
}

type checkpointConsistencyReturnDriftPortV2 struct {
	ports.CheckpointFactPortV2
	lostReply bool
}

func (p checkpointConsistencyReturnDriftPortV2) CommitCheckpointConsistencyV2(ctx context.Context, request ports.CheckpointConsistencyOwnerCommitRequestV2) (ports.CheckpointConsistencyCommitBundleV2, error) {
	bundle, err := p.CheckpointFactPortV2.CommitCheckpointConsistencyV2(ctx, request)
	if err != nil {
		return bundle, err
	}
	if p.lostReply {
		return ports.CheckpointConsistencyCommitBundleV2{}, core.NewError(core.ErrorUnavailable, core.ReasonCheckpointInconsistent, "injected Consistency reply loss")
	}
	return driftCheckpointConsistencyBundleV2(bundle)
}

func (p checkpointConsistencyReturnDriftPortV2) InspectCheckpointConsistencyV2(ctx context.Context, ref ports.CheckpointConsistencyRefV2) (ports.CheckpointConsistencyFactV2, error) {
	fact, err := p.CheckpointFactPortV2.InspectCheckpointConsistencyV2(ctx, ref)
	if err != nil || !p.lostReply {
		return fact, err
	}
	fact.ManifestSeal.Digest = digestCheckpointV2("wrong-return-manifest")
	return ports.SealCheckpointConsistencyFactV2(fact)
}

func driftCheckpointConsistencyBundleV2(bundle ports.CheckpointConsistencyCommitBundleV2) (ports.CheckpointConsistencyCommitBundleV2, error) {
	bundle.Consistency.ManifestSeal.Digest = digestCheckpointV2("wrong-return-manifest")
	consistency, err := ports.SealCheckpointConsistencyFactV2(bundle.Consistency)
	if err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	bundle.Consistency = consistency
	bundle.Attempt.Consistency = &consistency.Ref
	bundle.Attempt, err = ports.SealCheckpointAttemptFactV2(bundle.Attempt)
	return bundle, err
}

func checkpointFinalizationClassificationV2(id string, classification ports.CheckpointFinalizationClassificationV2) *ports.CheckpointFinalizationClassificationEntryV2 {
	return &ports.CheckpointFinalizationClassificationEntryV2{ID: id, Kind: "praxis.runtime/checkpoint-finalization", Classification: classification, SourceRevision: 1, SourceDigest: digestCheckpointV2("classification-" + id)}
}

func prepareCheckpointFinalizationFixtureV2(t *testing.T, fixture *checkpointFixtureV2, suffix string) (ports.CheckpointAttemptBarrierBundleV2, ports.CheckpointFinalizationInputClosureRefV2) {
	t.Helper()
	bundle, err := fixture.gateway.CreateCheckpointAttemptV2(context.Background(), fixture.create)
	if err != nil {
		t.Fatal(err)
	}
	fixture.currents.effectRoot = digestCheckpointV2("inventory-" + suffix)
	cut, err := fixture.gateway.FreezeCheckpointEffectCutV2(context.Background(), ports.FreezeCheckpointEffectCutRequestV2{
		Attempt: bundle.Attempt.RefV2(), Barrier: bundle.Barrier.RefV2(), ExpectedAttemptRevision: bundle.Attempt.Revision,
		ExpectedBarrierRevision: bundle.Barrier.Revision, EffectInventoryRoot: fixture.currents.effectRoot,
		EffectInventoryWatermark: 1, ExpectedEffectCount: 0, IdempotencyKey: "freeze-" + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := fixture.gateway.PrepareCheckpointFinalizationInputsV2(context.Background(), ports.PrepareCheckpointFinalizationInputsRequestV2{
		Attempt: cut.Attempt.RefV2(), Barrier: bundle.Barrier.RefV2(), EffectCut: cut.Cut.Ref,
		ExpectedAttemptRevision: cut.Attempt.Revision, ExpectedBarrierRevision: bundle.Barrier.Revision, IdempotencyKey: "inputs-" + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err := fixture.gateway.InspectCheckpointAttemptV2(context.Background(), ports.InspectCheckpointAttemptRequestV2{TenantID: bundle.Attempt.TenantID, AttemptID: bundle.Attempt.ID})
	if err != nil {
		t.Fatal(err)
	}
	return current, inputs
}

func terminalCheckpointStateForTestV2(state ports.CheckpointAttemptStateV2) bool {
	return state == ports.CheckpointAttemptConsistentV2 || state == ports.CheckpointAttemptIncompleteV2 || state == ports.CheckpointAttemptAbortedV2 || state == ports.CheckpointAttemptIndeterminateV2
}

func TestCheckpointGovernanceV2ConsistencyAndCloseBarrierAreAtomic(t *testing.T) {
	fixture := newCheckpointFixtureV2(t, "consistent")
	bundle, err := fixture.gateway.CreateCheckpointAttemptV2(context.Background(), fixture.create)
	if err != nil {
		t.Fatal(err)
	}
	fixture.currents.effectRoot = digestCheckpointV2("consistent-inventory")
	cut, err := fixture.gateway.FreezeCheckpointEffectCutV2(context.Background(), ports.FreezeCheckpointEffectCutRequestV2{
		Attempt: bundle.Attempt.RefV2(), Barrier: bundle.Barrier.RefV2(), ExpectedAttemptRevision: bundle.Attempt.Revision,
		ExpectedBarrierRevision: bundle.Barrier.Revision, EffectInventoryRoot: digestCheckpointV2("consistent-inventory"),
		EffectInventoryWatermark: 1, ExpectedEffectCount: 0, IdempotencyKey: "freeze-consistent",
	})
	if err != nil {
		t.Fatal(err)
	}
	closure := checkpointCommittedParticipantClosureV2(t, cut.Attempt.RefV2(), "consistent")
	branch, err := fixture.branches.SelectCheckpointParticipantBranchV2(context.Background(), ports.SelectCheckpointParticipantBranchRequestV2{Attempt: cut.Attempt.RefV2(), Participant: closure.Participant, Terminal: *closure.Terminal, SelectedAt: fixture.currents.now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	fixture.currents.branches[closure.Participant.ID] = branch.Ref
	fixture.currents.closures = []ports.CheckpointParticipantClosureRefV2{closure}
	fixture.currents.participantRoot = digestCheckpointV2("participants-consistent")
	manifest := checkpointManifestProjectionV2(t, cut.Attempt, bundle.Barrier, cut.Cut, []ports.CheckpointParticipantClosureRefV2{closure})
	manifestReader := &checkpointManifestReaderV2{projection: manifest}
	fixture.gateway.Manifests = manifestReader
	request := ports.CommitCheckpointConsistencyRequestV2{
		Attempt: cut.Attempt.RefV2(), Barrier: bundle.Barrier.RefV2(), ExpectedAttemptRevision: cut.Attempt.Revision,
		ExpectedBarrierRevision: bundle.Barrier.Revision, EffectCut: cut.Cut.Ref, ManifestSeal: manifest.Ref,
		ExpectedParticipantRoot: fixture.currents.participantRoot, ExpectedParticipantWatermark: 1,
		ExpectedParticipantCount: 1, IdempotencyKey: "commit-consistent",
	}
	manifestReader.driftSecond = true
	if _, err := fixture.gateway.CommitCheckpointConsistencyAndCloseBarrierV2(context.Background(), request); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("Manifest Seal S1/S2 drift must fail closed: %v", err)
	}
	manifestReader.driftSecond = false
	manifestReader.calls.Store(0)
	fixture.store.FailNextCheckpointAtomicStageV2()
	if _, err := fixture.gateway.CommitCheckpointConsistencyAndCloseBarrierV2(context.Background(), request); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("staged Consistency failure should remain indeterminate: %v", err)
	}
	historical, err := fixture.gateway.InspectCheckpointAttemptV2(context.Background(), ports.InspectCheckpointAttemptRequestV2{TenantID: cut.Attempt.TenantID, AttemptID: cut.Attempt.ID})
	if err != nil || historical.Attempt.State != ports.CheckpointAttemptCutFrozenV2 || historical.Barrier.State != ports.CheckpointBarrierActiveV2 {
		t.Fatalf("failed Consistency published Attempt or closed Barrier: %#v err=%v", historical, err)
	}
	fixture.store.LoseNextCheckpointReplyV2()
	consistent, err := fixture.gateway.CommitCheckpointConsistencyAndCloseBarrierV2(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if consistent.Attempt.State != ports.CheckpointAttemptConsistentV2 || consistent.Barrier.State != ports.CheckpointBarrierClosedV2 {
		t.Fatalf("Consistency and closed Barrier were not committed together: %#v", consistent)
	}
	current, err := fixture.gateway.InspectCheckpointAttemptTerminalCurrentV2(context.Background(), consistent.Attempt.RefV2())
	if err != nil || current.Consistency == nil || *current.Consistency != consistent.Consistency.Ref {
		t.Fatalf("consistent terminal current projection is incomplete: %#v err=%v", current, err)
	}
	replayed, err := fixture.gateway.CommitCheckpointConsistencyAndCloseBarrierV2(context.Background(), request)
	if err != nil || replayed.Attempt.RefV2() != consistent.Attempt.RefV2() || replayed.Barrier.RefV2() != consistent.Barrier.RefV2() || replayed.Consistency.Ref != consistent.Consistency.Ref {
		t.Fatalf("terminal Consistency replay did not return persisted closure: bundle=%+v err=%v", replayed, err)
	}
	changed := request
	changed.ManifestSeal.ManifestID += "-changed"
	if _, err := fixture.gateway.CommitCheckpointConsistencyAndCloseBarrierV2(context.Background(), changed); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("terminal Consistency accepted changed immutable history: %v", err)
	}
	fixture.currents.closures = nil
	if _, err := fixture.gateway.InspectCheckpointAttemptTerminalCurrentV2(context.Background(), consistent.Attempt.RefV2()); err == nil {
		t.Fatal("historical Consistency remained current after Participant Owner closure drift")
	}
	if historicalConsistency, err := fixture.gateway.InspectCheckpointConsistencyV2(context.Background(), consistent.Consistency.Ref); err != nil || historicalConsistency.Ref != consistent.Consistency.Ref {
		t.Fatalf("Participant current drift destroyed immutable Consistency history: fact=%+v err=%v", historicalConsistency, err)
	}
}

func TestCheckpointGovernanceV2TypedNilFailsBeforeBackend(t *testing.T) {
	fixture := newCheckpointFixtureV2(t, "typed-nil")
	var store *fakes.CheckpointStoreV2
	fixture.gateway.Facts = store
	if _, err := fixture.gateway.CreateCheckpointAttemptV2(context.Background(), fixture.create); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed-nil Fact Owner should fail closed: %v", err)
	}
}

func TestCheckpointGovernanceV2TerminalCurrentPreflightsEveryBranchDependencyBeforeBackend(t *testing.T) {
	setters := map[string]func(*kernel.CheckpointGovernanceGatewayV2){
		"manifest": func(gateway *kernel.CheckpointGovernanceGatewayV2) {
			var value *checkpointManifestReaderV2
			gateway.Manifests = value
		},
		"participants": func(gateway *kernel.CheckpointGovernanceGatewayV2) {
			var value *checkpointCurrentOwnersV2
			gateway.Participants = value
		},
		"closures": func(gateway *kernel.CheckpointGovernanceGatewayV2) {
			var value *checkpointCurrentOwnersV2
			gateway.Closures = value
		},
		"branches": func(gateway *kernel.CheckpointGovernanceGatewayV2) {
			var value *fakes.CheckpointParticipantBranchStoreV2
			gateway.Branches = value
		},
		"diagnostics": func(gateway *kernel.CheckpointGovernanceGatewayV2) {
			var value *checkpointDiagnosticsOwnerV2
			gateway.Diagnostics = value
		},
		"residuals": func(gateway *kernel.CheckpointGovernanceGatewayV2) {
			var value *checkpointResidualsOwnerV2
			gateway.Residuals = value
		},
	}
	for name, setNil := range setters {
		t.Run(name, func(t *testing.T) {
			fixture := newCheckpointFixtureV2(t, "terminal-preflight-"+name)
			bundle, err := fixture.gateway.CreateCheckpointAttemptV2(context.Background(), fixture.create)
			if err != nil {
				t.Fatal(err)
			}
			spy := &checkpointFactReadSpyV2{CheckpointFactPortV2: fixture.store}
			fixture.gateway.Facts = spy
			setNil(&fixture.gateway)
			if _, err := fixture.gateway.InspectCheckpointAttemptTerminalCurrentV2(context.Background(), bundle.Attempt.RefV2()); !core.HasReason(err, core.ReasonComponentMissing) {
				t.Fatalf("typed-nil %s dependency did not fail closed: %v", name, err)
			}
			if spy.reads.Load() != 0 {
				t.Fatalf("typed-nil %s dependency was discovered after %d backend reads", name, spy.reads.Load())
			}
		})
	}
}

type checkpointFactReadSpyV2 struct {
	ports.CheckpointFactPortV2
	reads atomic.Int64
}

func (p *checkpointFactReadSpyV2) InspectCheckpointAttemptBundleV2(ctx context.Context, request ports.InspectCheckpointAttemptRequestV2) (ports.CheckpointAttemptBarrierBundleV2, error) {
	p.reads.Add(1)
	return p.CheckpointFactPortV2.InspectCheckpointAttemptBundleV2(ctx, request)
}

func TestCheckpointGovernanceV2OwnerCurrentDriftFailsBeforeMutation(t *testing.T) {
	t.Run("attempt-inputs", func(t *testing.T) {
		fixture := newCheckpointFixtureV2(t, "inputs-drift")
		fixture.gateway.Inputs = &checkpointAttemptInputsDriftReaderV2{base: fixture.currents}
		if _, err := fixture.gateway.CreateCheckpointAttemptV2(context.Background(), fixture.create); !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("Attempt input S1/S2 drift did not fail closed: %v", err)
		}
		if _, err := fixture.store.InspectCheckpointAttemptBundleV2(context.Background(), ports.InspectCheckpointAttemptRequestV2{TenantID: fixture.create.Scope.Identity.TenantID, AttemptID: fixture.create.AttemptID}); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("Attempt input drift published an Attempt+Barrier bundle: %v", err)
		}
	})

	t.Run("effect-inventory", func(t *testing.T) {
		fixture := newCheckpointFixtureV2(t, "effect-drift")
		bundle, err := fixture.gateway.CreateCheckpointAttemptV2(context.Background(), fixture.create)
		if err != nil {
			t.Fatal(err)
		}
		fixture.gateway.Effects = &checkpointEffectInventoryDriftReaderV2{base: fixture.currents}
		if _, err := fixture.gateway.FreezeCheckpointEffectCutV2(context.Background(), ports.FreezeCheckpointEffectCutRequestV2{
			Attempt: bundle.Attempt.RefV2(), Barrier: bundle.Barrier.RefV2(), ExpectedAttemptRevision: bundle.Attempt.Revision,
			ExpectedBarrierRevision: bundle.Barrier.Revision, EffectInventoryRoot: fixture.currents.effectRoot,
			EffectInventoryWatermark: 1, ExpectedEffectCount: 0, IdempotencyKey: "freeze-effect-drift",
		}); !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("Effect inventory S1/S2 drift did not fail closed: %v", err)
		}
		current, err := fixture.gateway.InspectCheckpointAttemptV2(context.Background(), ports.InspectCheckpointAttemptRequestV2{TenantID: bundle.Attempt.TenantID, AttemptID: bundle.Attempt.ID})
		if err != nil || current.Attempt.State != ports.CheckpointAttemptBarrierAcquiredV2 || current.Attempt.EffectCut != nil {
			t.Fatalf("Effect inventory drift changed Attempt state: bundle=%+v err=%v", current, err)
		}
	})

	t.Run("open-terminal-kind", func(t *testing.T) {
		fixture := newCheckpointFixtureV2(t, "open-terminal-kind")
		bundle, err := fixture.gateway.CreateCheckpointAttemptV2(context.Background(), fixture.create)
		if err != nil {
			t.Fatal(err)
		}
		attempt := ports.OperationDispatchAttemptRefV3{OperationDigest: digestCheckpointV2("open-terminal-operation"), EffectID: "effect-open-terminal", IntentRevision: 1, IntentDigest: digestCheckpointV2("open-terminal-intent"), PermitID: "permit-open-terminal", PermitRevision: 1, PermitDigest: digestCheckpointV2("open-terminal-permit"), AttemptID: "attempt-open-terminal"}
		entry := ports.EffectCutEntryV2{EffectID: attempt.EffectID, IntentRevision: 1, IntentDigest: attempt.IntentDigest, Attempt: attempt, Phase: "checkpoint-prepare", Disposition: ports.EffectCutUnknownV2, Terminal: ports.RuntimeOperationTerminalRefV2{Kind: ports.RuntimeOperationTerminalKindV2("custom/unknown")}}
		fixture.gateway.Effects = checkpointMalformedEffectInventoryReaderV2{projection: ports.CheckpointEffectInventoryCurrentProjectionV2{ContractVersion: ports.CheckpointGovernanceContractVersionV2, Attempt: bundle.Attempt.RefV2(), Barrier: bundle.Barrier.RefV2(), RootDigest: digestCheckpointV2("open-terminal-root"), Watermark: 1, Entries: []ports.EffectCutEntryV2{entry}, CheckedUnixNano: fixture.currents.now.UnixNano(), ExpiresUnixNano: fixture.currents.now.Add(time.Minute).UnixNano(), ProjectionDigest: digestCheckpointV2("malformed-open-terminal-projection")}}
		if _, err := fixture.gateway.FreezeCheckpointEffectCutV2(context.Background(), ports.FreezeCheckpointEffectCutRequestV2{Attempt: bundle.Attempt.RefV2(), Barrier: bundle.Barrier.RefV2(), ExpectedAttemptRevision: bundle.Attempt.Revision, ExpectedBarrierRevision: bundle.Barrier.Revision, EffectInventoryRoot: digestCheckpointV2("open-terminal-root"), EffectInventoryWatermark: 1, ExpectedEffectCount: 1, IdempotencyKey: "freeze-open-terminal"}); err == nil {
			t.Fatal("open terminal kind reached Effect Cut Owner")
		}
		current, inspectErr := fixture.gateway.InspectCheckpointAttemptV2(context.Background(), ports.InspectCheckpointAttemptRequestV2{TenantID: bundle.Attempt.TenantID, AttemptID: bundle.Attempt.ID})
		if inspectErr != nil || current.Attempt.EffectCut != nil || current.Attempt.State != ports.CheckpointAttemptBarrierAcquiredV2 {
			t.Fatalf("open terminal kind changed checkpoint state: %+v err=%v", current, inspectErr)
		}
	})

	t.Run("v5-terminal-without-exact-dispatch-proof", func(t *testing.T) {
		fixture := newCheckpointFixtureV2(t, "v5-terminal-unsupported")
		bundle, err := fixture.gateway.CreateCheckpointAttemptV2(context.Background(), fixture.create)
		if err != nil {
			t.Fatal(err)
		}
		dispatch := ports.OperationDispatchAttemptRefV3{OperationDigest: digestCheckpointV2("v5-terminal-operation"), EffectID: "effect-v5-terminal", IntentRevision: 1, IntentDigest: digestCheckpointV2("v5-terminal-intent"), PermitID: "permit-v5-terminal", PermitRevision: 1, PermitDigest: digestCheckpointV2("v5-terminal-permit"), AttemptID: "attempt-v5-terminal"}
		settlement := ports.OperationCheckpointRestoreSettlementRefV5{ID: "checkpoint-settlement-v5-terminal", Revision: 1, TenantID: bundle.Attempt.TenantID, EffectID: dispatch.EffectID, Attempt: bundle.Attempt.RefV2(), Phase: ports.CheckpointPhasePrepareV2, OperationDigest: dispatch.OperationDigest, Digest: digestCheckpointV2("checkpoint-settlement-v5-terminal")}
		entry := ports.EffectCutEntryV2{EffectID: dispatch.EffectID, IntentRevision: dispatch.IntentRevision, IntentDigest: dispatch.IntentDigest, Attempt: dispatch, Phase: "checkpoint-prepare", Disposition: ports.EffectCutSettledV2, Terminal: ports.RuntimeOperationTerminalRefV2{Kind: ports.RuntimeTerminalCheckpointSettlementV5V2, CheckpointSettlementV5: &settlement}}
		root := digestCheckpointV2("v5-terminal-unsupported-root")
		fixture.gateway.Effects = checkpointMalformedEffectInventoryReaderV2{projection: ports.CheckpointEffectInventoryCurrentProjectionV2{ContractVersion: ports.CheckpointGovernanceContractVersionV2, Attempt: bundle.Attempt.RefV2(), Barrier: bundle.Barrier.RefV2(), RootDigest: root, Watermark: 1, Entries: []ports.EffectCutEntryV2{entry}, CheckedUnixNano: fixture.currents.now.UnixNano(), ExpiresUnixNano: fixture.currents.now.Add(time.Minute).UnixNano(), ProjectionDigest: digestCheckpointV2("v5-terminal-unsupported-projection")}}
		if _, err := fixture.gateway.FreezeCheckpointEffectCutV2(context.Background(), ports.FreezeCheckpointEffectCutRequestV2{Attempt: bundle.Attempt.RefV2(), Barrier: bundle.Barrier.RefV2(), ExpectedAttemptRevision: bundle.Attempt.Revision, ExpectedBarrierRevision: bundle.Barrier.Revision, EffectInventoryRoot: root, EffectInventoryWatermark: 1, ExpectedEffectCount: 1, IdempotencyKey: "freeze-v5-terminal-unsupported"}); !core.HasCategory(err, core.ErrorPreconditionFailed) && !core.HasCategory(err, core.ErrorForbidden) {
			t.Fatalf("V5 terminal without exact dispatch proof reached Effect Cut Owner: %v", err)
		}
		current, inspectErr := fixture.gateway.InspectCheckpointAttemptV2(context.Background(), ports.InspectCheckpointAttemptRequestV2{TenantID: bundle.Attempt.TenantID, AttemptID: bundle.Attempt.ID})
		if inspectErr != nil || current.Attempt.EffectCut != nil || current.Attempt.State != ports.CheckpointAttemptBarrierAcquiredV2 {
			t.Fatalf("unsupported V5 terminal changed checkpoint state: %+v err=%v", current, inspectErr)
		}
	})
}

type checkpointMalformedEffectInventoryReaderV2 struct {
	projection ports.CheckpointEffectInventoryCurrentProjectionV2
}

func (r checkpointMalformedEffectInventoryReaderV2) InspectCheckpointEffectInventoryCurrentV2(context.Context, ports.CheckpointAttemptRefV2, ports.CheckpointBarrierLeaseRefV2) (ports.CheckpointEffectInventoryCurrentProjectionV2, error) {
	return r.projection, nil
}

type checkpointAttemptInputsDriftReaderV2 struct {
	base  *checkpointCurrentOwnersV2
	calls atomic.Int64
}

func (r *checkpointAttemptInputsDriftReaderV2) InspectCheckpointAttemptInputsCurrentV2(ctx context.Context, attempt ports.CheckpointAttemptRefV2) (ports.CheckpointAttemptInputsCurrentProjectionV2, error) {
	projection, err := r.base.InspectCheckpointAttemptInputsCurrentV2(ctx, attempt)
	if err != nil || r.calls.Add(1) == 1 {
		return projection, err
	}
	projection.Run.Revision++
	projection.Run.Digest = digestCheckpointV2("run-current-drift")
	projection.ProjectionDigest = ""
	return ports.SealCheckpointAttemptInputsCurrentProjectionV2(projection, r.base.now)
}

type checkpointEffectInventoryDriftReaderV2 struct {
	base  *checkpointCurrentOwnersV2
	calls atomic.Int64
}

func (r *checkpointEffectInventoryDriftReaderV2) InspectCheckpointEffectInventoryCurrentV2(ctx context.Context, attempt ports.CheckpointAttemptRefV2, barrier ports.CheckpointBarrierLeaseRefV2) (ports.CheckpointEffectInventoryCurrentProjectionV2, error) {
	projection, err := r.base.InspectCheckpointEffectInventoryCurrentV2(ctx, attempt, barrier)
	if err != nil || r.calls.Add(1) == 1 {
		return projection, err
	}
	projection.RootDigest = digestCheckpointV2("effect-inventory-drift")
	projection.ProjectionDigest = ""
	return ports.SealCheckpointEffectInventoryCurrentProjectionV2(projection, r.base.now)
}

type checkpointFixtureV2 struct {
	store       *fakes.CheckpointStoreV2
	branches    *fakes.CheckpointParticipantBranchStoreV2
	policy      checkpointPolicyReaderV2
	diagnostics *checkpointDiagnosticsOwnerV2
	residuals   *checkpointResidualsOwnerV2
	gateway     kernel.CheckpointGovernanceGatewayV2
	create      ports.CreateCheckpointAttemptRequestV2
	currents    *checkpointCurrentOwnersV2
}

func newCheckpointFixtureV2(t *testing.T, suffix string) checkpointFixtureV2 {
	t.Helper()
	now := time.Unix(1_780_000_000, 0).UTC()
	scope := core.ExecutionScope{
		Identity: core.AgentIdentityRef{TenantID: "tenant-checkpoint", ID: "identity-checkpoint", Epoch: 1},
		Lineage:  core.LineageRef{ID: "lineage-checkpoint", PlanDigest: digestCheckpointV2("lineage")},
		Instance: core.InstanceRef{ID: "instance-checkpoint", Epoch: 1}, AuthorityEpoch: 1,
	}
	scopeDigest, err := ports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	policyRef := ports.CheckpointBarrierPolicyRefV2{ID: "checkpoint-policy-" + suffix, Revision: 1, Digest: digestCheckpointV2("policy-" + suffix), SemanticDigest: digestCheckpointV2("policy-semantic")}
	projection, err := ports.SealCheckpointBarrierPolicyCurrentProjectionV2(ports.CheckpointBarrierPolicyCurrentProjectionV2{
		Ref: policyRef, MaxBarrierTTLUnixNano: int64(10 * time.Minute), MaxReconciliationTTLUnixNano: int64(5 * time.Minute),
		UnknownAtDeadlineMode: ports.CheckpointUnknownAtDeadlineIndeterminateV2, AbsoluteNotAfterUnixNano: now.Add(20 * time.Minute).UnixNano(),
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(15 * time.Minute).UnixNano(),
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	request := ports.CreateCheckpointAttemptRequestV2{
		AttemptID: "checkpoint-attempt-" + suffix, BarrierID: "checkpoint-barrier-" + suffix, IdempotencyKey: "create-" + suffix,
		Scope: scope, ScopeDigest: scopeDigest, RunID: "run-checkpoint-" + core.AgentRunID(suffix), RunStableIdentityDigest: digestCheckpointV2("run-identity-" + suffix),
		Generation:                  ports.GenerationArtifactRefV1{ID: "generation-" + suffix, Revision: 1, Digest: digestCheckpointV2("generation"), InputDigest: digestCheckpointV2("input"), ManifestDigest: digestCheckpointV2("manifest"), GraphDigest: digestCheckpointV2("graph"), CatalogDigest: digestCheckpointV2("catalog")},
		GenerationBinding:           ports.GenerationBindingAssociationRefV1{ID: "generation-binding-" + suffix, Revision: 1, Digest: digestCheckpointV2("generation-binding")},
		BindingSet:                  ports.RunBindingSetRefV2{ID: "binding-set-" + suffix, Revision: 1, Digest: digestCheckpointV2("binding-set"), SemanticDigest: digestCheckpointV2("binding-set-semantic")},
		ParticipantSetCertification: ports.CheckpointParticipantSetCertificationRefV2{ID: "participant-set-" + suffix, Revision: 1, Digest: digestCheckpointV2("participant-set")},
		Workflow:                    ports.CheckpointWorkflowRefV2{ID: "workflow-" + suffix, Revision: 1, Digest: digestCheckpointV2("workflow"), NotAfter: now.Add(12 * time.Minute).UnixNano()},
		BarrierPolicy:               policyRef, ExpectedRunRevision: 1, AcquiredDispatchWatermark: 1,
	}
	store := fakes.NewCheckpointStoreV2()
	branches := fakes.NewCheckpointParticipantBranchStoreV2()
	diagnostics := &checkpointDiagnosticsOwnerV2{}
	residuals := &checkpointResidualsOwnerV2{}
	currents := &checkpointCurrentOwnersV2{now: now, create: request, effectRoot: digestCheckpointV2("inventory"), participantRoot: digestCheckpointV2("participants"), branches: map[string]ports.CheckpointParticipantBranchGuardRefV2{}}
	gateway := kernel.CheckpointGovernanceGatewayV2{Facts: store, Policies: checkpointPolicyReaderV2{projection: projection}, Runs: currents, Inputs: currents, Effects: currents, Participants: currents, Closures: currents, Branches: branches, Manifests: &checkpointManifestReaderV2{}, Diagnostics: diagnostics, Residuals: residuals, Clock: func() time.Time { return now }}
	return checkpointFixtureV2{store: store, branches: branches, policy: checkpointPolicyReaderV2{projection: projection}, diagnostics: diagnostics, residuals: residuals, gateway: gateway, create: request, currents: currents}
}

func prepareCheckpointConsistencyCommitV2(t *testing.T, suffix string) (checkpointFixtureV2, ports.CommitCheckpointConsistencyRequestV2) {
	t.Helper()
	fixture := newCheckpointFixtureV2(t, suffix)
	bundle, err := fixture.gateway.CreateCheckpointAttemptV2(context.Background(), fixture.create)
	if err != nil {
		t.Fatal(err)
	}
	fixture.currents.effectRoot = digestCheckpointV2("inventory-" + suffix)
	cut, err := fixture.gateway.FreezeCheckpointEffectCutV2(context.Background(), ports.FreezeCheckpointEffectCutRequestV2{Attempt: bundle.Attempt.RefV2(), Barrier: bundle.Barrier.RefV2(), ExpectedAttemptRevision: bundle.Attempt.Revision, ExpectedBarrierRevision: bundle.Barrier.Revision, EffectInventoryRoot: fixture.currents.effectRoot, EffectInventoryWatermark: 1, ExpectedEffectCount: 0, IdempotencyKey: "freeze-" + suffix})
	if err != nil {
		t.Fatal(err)
	}
	closure := checkpointCommittedParticipantClosureV2(t, cut.Attempt.RefV2(), suffix)
	branch, err := fixture.branches.SelectCheckpointParticipantBranchV2(context.Background(), ports.SelectCheckpointParticipantBranchRequestV2{Attempt: cut.Attempt.RefV2(), Participant: closure.Participant, Terminal: *closure.Terminal, SelectedAt: fixture.currents.now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	fixture.currents.branches[closure.Participant.ID] = branch.Ref
	fixture.currents.closures = []ports.CheckpointParticipantClosureRefV2{closure}
	fixture.currents.participantRoot = digestCheckpointV2("participants-" + suffix)
	manifest := checkpointManifestProjectionV2(t, cut.Attempt, bundle.Barrier, cut.Cut, fixture.currents.closures)
	fixture.gateway.Manifests = &checkpointManifestReaderV2{projection: manifest}
	request := ports.CommitCheckpointConsistencyRequestV2{Attempt: cut.Attempt.RefV2(), Barrier: bundle.Barrier.RefV2(), ExpectedAttemptRevision: cut.Attempt.Revision, ExpectedBarrierRevision: bundle.Barrier.Revision, EffectCut: cut.Cut.Ref, ManifestSeal: manifest.Ref, ExpectedParticipantRoot: fixture.currents.participantRoot, ExpectedParticipantWatermark: 1, ExpectedParticipantCount: 1, IdempotencyKey: "commit-" + suffix}
	return fixture, request
}

type checkpointCurrentOwnersV2 struct {
	now             time.Time
	inputTTL        time.Duration
	create          ports.CreateCheckpointAttemptRequestV2
	effectRoot      core.Digest
	participantRoot core.Digest
	closures        []ports.CheckpointParticipantClosureRefV2
	branches        map[string]ports.CheckpointParticipantBranchGuardRefV2
}

func checkpointCurrentInputRefV2(kind, id string, now time.Time) ports.CheckpointCurrentInputRefV2 {
	return ports.CheckpointCurrentInputRefV2{Kind: ports.NamespacedNameV2(kind), ID: id, Revision: 1, Digest: digestCheckpointV2(kind + id), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(10 * time.Minute).UnixNano()}
}

func (o *checkpointCurrentOwnersV2) InspectCheckpointRunCurrentV2(_ context.Context, scope core.ExecutionScope, runID core.AgentRunID) (ports.CheckpointRunCurrentProjectionV2, error) {
	scopeDigest, err := ports.ExecutionScopeDigestV2(scope)
	if err != nil {
		return ports.CheckpointRunCurrentProjectionV2{}, err
	}
	return ports.SealCheckpointRunCurrentProjectionV2(ports.CheckpointRunCurrentProjectionV2{RunID: runID, Revision: o.create.ExpectedRunRevision, Status: core.RunRunning, RunStableIdentityDigest: o.create.RunStableIdentityDigest, ExecutionScopeDigest: scopeDigest, CheckedUnixNano: o.now.UnixNano(), ExpiresUnixNano: o.now.Add(10 * time.Minute).UnixNano()}, o.now)
}

func (o *checkpointCurrentOwnersV2) InspectCheckpointAttemptInputsCurrentV2(_ context.Context, attempt ports.CheckpointAttemptRefV2) (ports.CheckpointAttemptInputsCurrentProjectionV2, error) {
	ttl := o.inputTTL
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	authority := ports.AuthorityBindingRefV2{Ref: "authority:" + o.create.AttemptID, Revision: 1, Epoch: 1, Digest: digestCheckpointV2("authority")}
	projection := ports.CheckpointAttemptInputsCurrentProjectionV2{AttemptID: attempt.ID, TenantID: attempt.TenantID, Run: checkpointCurrentInputRefV2("praxis.runtime/run-current", string(o.create.RunID), o.now), RunID: o.create.RunID, RunStableIdentityDigest: o.create.RunStableIdentityDigest, Generation: checkpointCurrentInputRefV2("praxis.runtime/generation-current", o.create.Generation.ID, o.now), GenerationArtifact: o.create.Generation, GenerationBinding: o.create.GenerationBinding, Binding: checkpointCurrentInputRefV2("praxis.runtime/binding-current", o.create.BindingSet.ID, o.now), BindingSet: o.create.BindingSet, ParticipantCertification: checkpointCurrentInputRefV2("praxis.runtime/participant-certification-current", o.create.ParticipantSetCertification.ID, o.now), ParticipantSetCertification: o.create.ParticipantSetCertification, WorkflowCurrent: checkpointCurrentInputRefV2("praxis.runtime/workflow-current", o.create.Workflow.ID, o.now), Workflow: o.create.Workflow, Authority: checkpointCurrentInputRefV2("praxis.runtime/authority-current", authority.Ref, o.now), AuthorityRef: authority, CheckedUnixNano: o.now.UnixNano(), ExpiresUnixNano: o.now.Add(10 * time.Minute).UnixNano()}
	projection.ExpiresUnixNano = o.now.Add(ttl).UnixNano()
	for _, ref := range []*ports.CheckpointCurrentInputRefV2{&projection.Run, &projection.Generation, &projection.Binding, &projection.ParticipantCertification, &projection.WorkflowCurrent, &projection.Authority} {
		ref.ExpiresUnixNano = projection.ExpiresUnixNano
	}
	return ports.SealCheckpointAttemptInputsCurrentProjectionV2(projection, o.now)
}

func (o *checkpointCurrentOwnersV2) InspectCheckpointEffectInventoryCurrentV2(_ context.Context, attempt ports.CheckpointAttemptRefV2, barrier ports.CheckpointBarrierLeaseRefV2) (ports.CheckpointEffectInventoryCurrentProjectionV2, error) {
	return ports.SealCheckpointEffectInventoryCurrentProjectionV2(ports.CheckpointEffectInventoryCurrentProjectionV2{Attempt: attempt, Barrier: barrier, RootDigest: o.effectRoot, Watermark: 1, Entries: []ports.EffectCutEntryV2{}, CheckedUnixNano: o.now.UnixNano(), ExpiresUnixNano: o.now.Add(10 * time.Minute).UnixNano()}, o.now)
}

func (o *checkpointCurrentOwnersV2) InspectCheckpointParticipantSetCurrentV2(_ context.Context, attempt ports.CheckpointAttemptRefV2, certification ports.CheckpointParticipantSetCertificationRefV2) (ports.CheckpointParticipantSetCurrentProjectionV2, error) {
	participants := make([]ports.CheckpointParticipantRefV2, 0, len(o.closures))
	for _, closure := range o.closures {
		participants = append(participants, closure.Participant)
	}
	projection := ports.CheckpointParticipantSetCurrentProjectionV2{ContractVersion: ports.CheckpointGovernanceContractVersionV2, Attempt: attempt, Certification: certification, RootDigest: o.participantRoot, Watermark: 1, Participants: participants, CheckedUnixNano: o.now.UnixNano(), ExpiresUnixNano: o.now.Add(10 * time.Minute).UnixNano()}
	digest, _ := projection.DigestV2()
	projection.ProjectionDigest = digest
	return projection, projection.Validate(o.now)
}

func (o *checkpointCurrentOwnersV2) InspectCheckpointParticipantClosureCurrentV2(_ context.Context, attempt ports.CheckpointAttemptRefV2, participant ports.CheckpointParticipantRefV2) (ports.CheckpointParticipantClosureCurrentProjectionV2, error) {
	for _, closure := range o.closures {
		if closure.Participant.ID != participant.ID {
			continue
		}
		guard := o.branches[participant.ID]
		projection := ports.CheckpointParticipantClosureCurrentProjectionV2{ContractVersion: ports.CheckpointGovernanceContractVersionV2, Attempt: attempt, Participant: participant, Closure: closure, BranchGuard: guard, CheckedUnixNano: o.now.UnixNano(), ExpiresUnixNano: o.now.Add(10 * time.Minute).UnixNano()}
		digest, _ := projection.DigestV2()
		projection.ProjectionDigest = digest
		return projection, projection.Validate(o.now)
	}
	return ports.CheckpointParticipantClosureCurrentProjectionV2{}, core.NewError(core.ErrorNotFound, core.ReasonCheckpointInconsistent, "participant closure not found")
}

type checkpointPolicyReaderV2 struct {
	projection ports.CheckpointBarrierPolicyCurrentProjectionV2
}

func (r checkpointPolicyReaderV2) InspectCheckpointBarrierPolicyCurrentV2(context.Context, ports.CheckpointBarrierPolicyRefV2) (ports.CheckpointBarrierPolicyCurrentProjectionV2, error) {
	return r.projection, nil
}

type checkpointDiagnosticsOwnerV2 struct {
	ref             ports.CheckpointDiagnosticsFinalizationSealRefV2
	classifications []ports.CheckpointFinalizationClassificationEntryV2
	drift           atomic.Bool
}

func (o *checkpointDiagnosticsOwnerV2) SealCheckpointDiagnosticsForFinalizationV2(_ context.Context, attempt ports.CheckpointAttemptRefV2, _ ports.EffectCutRefV2, cut ports.CheckpointFinalizationCutRefV2) (ports.CheckpointDiagnosticsFinalizationSealRefV2, error) {
	if o.ref.ID == "" {
		set := ports.CheckpointDiagnosticSetRefV2{AttemptID: attempt.ID, Revision: 1, SetDigest: digestCheckpointV2("diagnostics-empty")}
		classifications, _ := ports.SealCheckpointFinalizationClassificationSetV2(ports.CheckpointFinalizationClassificationSetV2{Entries: append([]ports.CheckpointFinalizationClassificationEntryV2{}, o.classifications...)})
		set.Count = uint32(len(classifications.Entries))
		o.ref = ports.CheckpointDiagnosticsFinalizationSealRefV2{ID: "diagnostics-seal-" + attempt.ID, Revision: 1, Attempt: attempt, FinalizationCut: cut, Owner: checkpointOwnerBindingV2("diagnostics"), SourceEpoch: 1, SourceSequence: 1, LedgerRootDigest: digestCheckpointV2("diagnostics-root"), CompleteSet: set, CompleteSetDigest: set.SetDigest, Classifications: classifications, Digest: digestCheckpointV2("diagnostics-seal")}
	}
	return o.ref, nil
}
func (o *checkpointDiagnosticsOwnerV2) InspectCheckpointDiagnosticsFinalizationSealCurrentV2(_ context.Context, ref ports.CheckpointDiagnosticsFinalizationSealRefV2) (ports.CheckpointDiagnosticsFinalizationSealProjectionV2, error) {
	if o.drift.Load() {
		return ports.CheckpointDiagnosticsFinalizationSealProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "injected diagnostics root drift")
	}
	projection := ports.CheckpointDiagnosticsFinalizationSealProjectionV2{Ref: ref, Current: true, CheckedUnixNano: time.Unix(1_780_000_000, 0).UnixNano()}
	digest, _ := projection.DigestV2()
	projection.ProjectionDigest = digest
	return projection, nil
}

type checkpointResidualsOwnerV2 struct {
	ref             ports.CheckpointResidualsFinalizationSealRefV2
	classifications []ports.CheckpointFinalizationClassificationEntryV2
}

type checkpointManifestReaderV2 struct {
	projection  ports.CheckpointManifestSealProjectionV2
	calls       atomic.Uint32
	driftSecond bool
}

func (r *checkpointManifestReaderV2) InspectCheckpointManifestSealV2(_ context.Context, request ports.InspectCheckpointManifestSealRequestV2) (ports.CheckpointManifestSealProjectionV2, error) {
	if err := request.Validate(); err != nil {
		return ports.CheckpointManifestSealProjectionV2{}, err
	}
	if r.projection.Ref != request.Ref || r.projection.ParticipantSetDigest != request.ExpectedParticipantSetDigest || !reflect.DeepEqual(r.projection.ParticipantClosures, request.ExpectedParticipantClosures) {
		return ports.CheckpointManifestSealProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "Manifest Reader expected closure drifted")
	}
	projection := r.projection
	if r.driftSecond && r.calls.Add(1) == 2 {
		projection.ContextClosureDigest = digestCheckpointV2("checkpoint-context-closure-drift")
		projection.SealDigest, _ = projection.DigestV2()
	}
	return projection, nil
}

func checkpointCommittedParticipantClosureV2(t *testing.T, attempt ports.CheckpointAttemptRefV2, suffix string) ports.CheckpointParticipantClosureRefV2 {
	t.Helper()
	operationFixture := newOperationSettlementFixtureV4(t, "checkpoint-closure-"+suffix)
	operation := operationFixture.submission.Operation
	operationDigest := operationFixture.submission.OperationDigest
	owner := checkpointSettlementOwnerBindingV5(t, operationFixture.effect.effect.intent)
	participant := ports.CheckpointParticipantRefV2{ID: "checkpoint-closure-participant-" + suffix, Owner: owner, Digest: digestCheckpointV2("checkpoint-closure-participant-" + suffix)}
	prepare := checkpointParticipantPhaseClosureV2(t, attempt, participant, operationFixture, operation, operationDigest, ports.CheckpointPhasePrepareV2, ports.CheckpointParticipantPreparedV2, suffix)
	commit := checkpointParticipantPhaseClosureV2(t, attempt, participant, operationFixture, operation, operationDigest, ports.CheckpointPhaseCommitV2, ports.CheckpointParticipantCommittedV2, suffix)
	closure := ports.CheckpointParticipantClosureRefV2{ID: "checkpoint-aggregate-closure-" + suffix, Participant: participant, Prepare: prepare, Terminal: &commit}
	digest, err := closure.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	closure.Digest = digest
	if err := closure.Validate(); err != nil {
		t.Fatal(err)
	}
	return closure
}

func checkpointParticipantPhaseClosureV2(t *testing.T, attempt ports.CheckpointAttemptRefV2, participant ports.CheckpointParticipantRefV2, operationFixture *operationSettlementFixtureV4, operation ports.OperationSubjectV3, operationDigest core.Digest, phase ports.CheckpointParticipantPhaseV2, state ports.CheckpointParticipantPhaseStateV2, suffix string) ports.CheckpointParticipantPhaseClosureRefV2 {
	t.Helper()
	phaseSuffix := string(phase) + "-" + suffix
	reservation := ports.CheckpointParticipantPhaseReservationRefV2{ID: "checkpoint-reservation-" + phaseSuffix, Revision: 1, Digest: digestCheckpointV2("checkpoint-reservation-" + phaseSuffix), ExpiresUnixNano: time.Unix(1_780_000_000, 0).Add(time.Minute).UnixNano()}
	phaseFact := ports.CheckpointParticipantPhaseRefV2{ID: "checkpoint-phase-" + phaseSuffix, Revision: 1, Phase: phase, State: state, Digest: digestCheckpointV2("checkpoint-phase-" + phaseSuffix)}
	domain := ports.CheckpointParticipantDomainResultRefV2{ID: "checkpoint-domain-" + phaseSuffix, Revision: 1, Kind: "praxis.sandbox/checkpoint-domain-result", Attempt: attempt, Participant: participant, Phase: phase, Operation: operation, OperationDigest: operationDigest, Digest: digestCheckpointV2("checkpoint-domain-" + phaseSuffix)}
	dispatch := operationFixture.submission.DomainResult.Attempt
	evidenceScope := checkpointEvidenceScopeV1(t, operationFixture, dispatch, phaseSuffix)
	scopeDigest := checkpointEvidenceScopeDigestV1(t, evidenceScope)
	barrier := ports.CheckpointBarrierLeaseRefV2{TenantID: attempt.TenantID, ID: "checkpoint-barrier-" + phaseSuffix, AttemptID: attempt.ID, Revision: 1, Digest: digestCheckpointV2("checkpoint-barrier-" + phaseSuffix), ExpiresUnixNano: time.Unix(1_780_000_000, 0).Add(time.Minute).UnixNano()}
	cut := ports.EffectCutRefV2{ID: "checkpoint-cut-" + phaseSuffix, Revision: 1, Attempt: attempt, RootDigest: digestCheckpointV2("checkpoint-cut-root-" + phaseSuffix), Watermark: 1, Digest: digestCheckpointV2("checkpoint-cut-" + phaseSuffix)}
	qualification := ports.CheckpointRestoreEvidenceQualificationRefV1{ID: "checkpoint-qualification-" + phaseSuffix, Revision: 1, Attempt: attempt, Barrier: barrier, EffectCut: cut, Reservation: reservation, Phase: phase, ScopeDigest: scopeDigest, ExpiresUnixNano: time.Unix(1_780_000_000, 0).Add(time.Minute).UnixNano()}
	qualification.Digest, _ = qualification.DigestV1()
	handoff := ports.CheckpointRestoreEvidenceProviderHandoffRefV1{ID: "checkpoint-handoff-" + phaseSuffix, Revision: 1, Qualification: qualification, Attempt: dispatch, Phase: phase, ScopeDigest: scopeDigest}
	handoff.Digest, _ = handoff.DigestV1()
	evidence := ports.CheckpointRestoreEvidenceConsumptionRefV1{ID: "checkpoint-consumption-" + phaseSuffix, Revision: 1, Qualification: qualification, Handoff: handoff, Record: checkpointEvidenceRecordRefV1(phaseSuffix), Attempt: attempt, Phase: phase, State: ports.CheckpointEvidenceConsumedCurrentV1, ScopeDigest: scopeDigest, Source: evidenceScope.Source}
	evidence.Digest, _ = evidence.DigestV1()
	settlement := ports.OperationCheckpointRestoreSettlementRefV5{ID: "checkpoint-settlement-" + phaseSuffix, Revision: 1, TenantID: attempt.TenantID, EffectID: dispatch.EffectID, Attempt: attempt, Phase: phase, OperationDigest: operationDigest, Digest: digestCheckpointV2("checkpoint-settlement-" + phaseSuffix)}
	apply := ports.CheckpointParticipantApplySettlementRefV2{ID: "checkpoint-apply-" + phaseSuffix, Revision: 1, Participant: participant, Phase: phase, SettlementID: settlement.ID, Digest: digestCheckpointV2("checkpoint-apply-" + phaseSuffix)}
	closure := ports.CheckpointParticipantPhaseClosureRefV2{ID: "checkpoint-phase-closure-" + phaseSuffix, Phase: phase, Reservation: reservation, PhaseFact: phaseFact, DomainResult: domain, Evidence: evidence, Settlement: settlement, ApplySettlement: apply}
	digest, err := closure.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	closure.Digest = digest
	return closure
}

func operationFixtureAttemptV3(t *testing.T, operation ports.OperationSubjectV3, suffix string) ports.OperationDispatchAttemptRefV3 {
	t.Helper()
	operationDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	return ports.OperationDispatchAttemptRefV3{OperationDigest: operationDigest, EffectID: core.EffectIntentID("checkpoint-effect-" + suffix), IntentRevision: 1, IntentDigest: digestCheckpointV2("checkpoint-intent-" + suffix), PermitID: "checkpoint-permit-" + suffix, PermitRevision: 1, PermitDigest: digestCheckpointV2("checkpoint-permit-" + suffix), AttemptID: "checkpoint-operation-attempt-" + suffix}
}

func checkpointManifestProjectionV2(t *testing.T, attempt ports.CheckpointAttemptFactV2, barrier ports.CheckpointBarrierLeaseFactV2, cut ports.EffectCutFactV2, closures []ports.CheckpointParticipantClosureRefV2) ports.CheckpointManifestSealProjectionV2 {
	t.Helper()
	sealDigest := digestCheckpointV2("checkpoint-manifest-seal-" + attempt.ID)
	ref := ports.CheckpointManifestSealRefV2{
		ExactLookup: ports.CheckpointExternalExactFactRefV2{
			ContractVersion: "praxis.continuity/checkpoint-manifest-governance/v2",
			SchemaRef:       "praxis.continuity/checkpoint-manifest-seal-fact/v2",
			Owner: ports.CheckpointManifestSealOwnerBindingV2{
				BindingSetID: "binding-set-continuity", BindingRevision: 1,
				ComponentID: "praxis/continuity", ManifestDigest: string(digestCheckpointV2("continuity-manifest")),
				ArtifactDigest: string(digestCheckpointV2("continuity-artifact")), Capability: "checkpoint-manifest-governance-v2",
				FactKind: "checkpoint_manifest_seal_fact_v2",
			},
			TenantID: string(attempt.TenantID), ID: "checkpoint-manifest-seal-" + attempt.ID,
			Revision: 1, Digest: string(sealDigest), ScopeDigest: string(attempt.ScopeDigest),
		},
		ID: "checkpoint-manifest-seal-" + attempt.ID, Revision: 1, Digest: sealDigest,
		ManifestID: "checkpoint-manifest-" + attempt.ID, ManifestRevision: 1,
		ManifestDigest: digestCheckpointV2("checkpoint-manifest-" + attempt.ID), Attempt: attempt.RefV2(),
		Barrier: barrier.RefV2(), EffectCut: cut.Ref, FrozenRefSetDigest: digestCheckpointV2("checkpoint-frozen-refs-" + attempt.ID),
	}
	projection := ports.CheckpointManifestSealProjectionV2{ContractVersion: ports.CheckpointManifestSealContractVersionV2, Ref: ref, ParticipantSetDigest: attempt.ParticipantSetCertification.Digest, ParticipantClosures: closures, ContextClosureDigest: digestCheckpointV2("checkpoint-context-closure"), ArtifactClosureDigest: digestCheckpointV2("checkpoint-artifact-closure")}
	digest, err := projection.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	projection.SealDigest = digest
	if err := projection.Validate(); err != nil {
		t.Fatal(err)
	}
	return projection
}

func (o *checkpointResidualsOwnerV2) SealCheckpointResidualsForFinalizationV2(_ context.Context, attempt ports.CheckpointAttemptRefV2, _ ports.EffectCutRefV2, cut ports.CheckpointFinalizationCutRefV2) (ports.CheckpointResidualsFinalizationSealRefV2, error) {
	if o.ref.ID == "" {
		set := ports.CheckpointResidualSetRefV2{AttemptID: attempt.ID, Revision: 1, SetDigest: digestCheckpointV2("residuals-empty")}
		classifications, _ := ports.SealCheckpointFinalizationClassificationSetV2(ports.CheckpointFinalizationClassificationSetV2{Entries: append([]ports.CheckpointFinalizationClassificationEntryV2{}, o.classifications...)})
		set.Count = uint32(len(classifications.Entries))
		o.ref = ports.CheckpointResidualsFinalizationSealRefV2{ID: "residuals-seal-" + attempt.ID, Revision: 1, Attempt: attempt, FinalizationCut: cut, Owner: checkpointOwnerBindingV2("residuals"), SourceEpoch: 1, SourceSequence: 1, LedgerRootDigest: digestCheckpointV2("residuals-root"), CompleteSet: set, CompleteSetDigest: set.SetDigest, Classifications: classifications, Digest: digestCheckpointV2("residuals-seal")}
	}
	return o.ref, nil
}
func (o *checkpointResidualsOwnerV2) InspectCheckpointResidualsFinalizationSealCurrentV2(_ context.Context, ref ports.CheckpointResidualsFinalizationSealRefV2) (ports.CheckpointResidualsFinalizationSealProjectionV2, error) {
	projection := ports.CheckpointResidualsFinalizationSealProjectionV2{Ref: ref, Current: true, CheckedUnixNano: time.Unix(1_780_000_000, 0).UnixNano()}
	digest, _ := projection.DigestV2()
	projection.ProjectionDigest = digest
	return projection, nil
}

func checkpointOwnerBindingV2(kind string) ports.ProviderBindingRefV2 {
	return ports.ProviderBindingRefV2{BindingSetID: "binding-set-owner", BindingSetRevision: 1, ComponentID: ports.ComponentIDV2("praxis.runtime/" + kind), ManifestDigest: digestCheckpointV2(kind + "-manifest"), ArtifactDigest: digestCheckpointV2(kind + "-artifact"), Capability: ports.CapabilityNameV2("praxis.runtime/checkpoint-" + kind)}
}
func digestCheckpointV2(value string) core.Digest { return core.DigestBytes([]byte(value)) }

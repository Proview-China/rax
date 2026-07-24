package checkpoint_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/kernel"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/reference"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestCheckpointGateFreezesExactSessionUntilRuntimeTerminal(t *testing.T) {
	fixture := newGateFixture(t, "happy")
	gate, snapshot, err := fixture.controller.AcquireCheckpointGateV1(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if gate.State != contract.CheckpointGateAcquiredV1 || snapshot.Session.Digest != fixture.session.Digest || snapshot.Session.Revision != fixture.session.Revision {
		t.Fatalf("gate/snapshot did not freeze exact Session: %#v / %#v", gate, snapshot)
	}

	guarded, err := kernel.NewCheckpointGuardedSessionFactPortV4(fixture.sessions, fixture.store, fixture.clock)
	if err != nil {
		t.Fatal(err)
	}
	newSession := fixture.creating.Clone()
	newSession.ID = "session-after-gate"
	newSession, err = contract.SealGovernedSessionV4(newSession)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := guarded.CreateSessionV4(context.Background(), newSession); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("current checkpoint gate did not fence actual Session write: %v", err)
	}
	bound, err := fixture.controller.BindCheckpointGateRuntimeV1(context.Background(), contract.BindCheckpointGateRuntimeRequestV1{Expected: gate.Ref, Runtime: fixture.runtime})
	if err != nil || bound.State != contract.CheckpointGateBoundV1 || bound.Ref.Revision != 2 {
		t.Fatalf("Runtime bind=%#v err=%v", bound, err)
	}
	if _, err := guarded.CreateSessionV4(context.Background(), newSession); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("Runtime-bound checkpoint gate did not retain Session fence: %v", err)
	}

	released, err := fixture.controller.ReleaseCheckpointGateV1(context.Background(), contract.ReleaseCheckpointGateRequestV1{Expected: bound.Ref, TerminalAttempt: fixture.runtime.Attempt})
	if err != nil || released.State != contract.CheckpointGateReleasedV1 || released.Ref.Revision != 3 {
		t.Fatalf("release=%#v err=%v", released, err)
	}
	if _, err := guarded.CreateSessionV4(context.Background(), newSession); err != nil {
		t.Fatalf("terminal Runtime checkpoint did not release Session write: %v", err)
	}
	history, err := fixture.store.InspectCheckpointGateV1(context.Background(), gate.Ref)
	if err != nil || !reflect.DeepEqual(history, gate) {
		t.Fatalf("gate history was overwritten: %#v / %v", history, err)
	}
}

func TestCheckpointGateRejectsUnsafeOrDriftedSessionBeforeMutation(t *testing.T) {
	fixture := newGateFixture(t, "drift")
	bad := fixture.request
	bad.ExpectedSessionDigest = core.DigestBytes([]byte("other-session"))
	if _, _, err := fixture.controller.AcquireCheckpointGateV1(context.Background(), bad); err == nil {
		t.Fatal("caller Session digest drift was accepted")
	}
	if _, err := fixture.store.InspectCheckpointGateCurrentV1(context.Background(), fixture.request.Run); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("invalid request mutated gate store: %v", err)
	}

	unsafe := fixture.session.Clone()
	unsafe.Phase = contract.SessionReconcilingV2
	reader := &sequenceSessionReaderV1{values: []contract.GovernedSessionV4{unsafe}}
	controller, err := kernel.NewCheckpointGateControllerV1(reader, fixture.store, fixture.terminals, fixture.clock)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := controller.AcquireCheckpointGateV1(context.Background(), fixture.request); err == nil {
		t.Fatal("reconciling Session was treated as checkpoint-safe")
	}
}

func TestCheckpointGateS1S2DriftInvalidatesGateAndLostReplyInspects(t *testing.T) {
	fixture := newGateFixture(t, "s1s2")
	drift := fixture.session.Clone()
	drift.Revision++
	drift.UpdatedUnixNano++
	drift.Digest = core.DigestBytes([]byte("drift"))
	reader := &sequenceSessionReaderV1{values: []contract.GovernedSessionV4{fixture.session, drift}}
	controller, err := kernel.NewCheckpointGateControllerV1(reader, fixture.store, fixture.terminals, fixture.clock)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := controller.AcquireCheckpointGateV1(context.Background(), fixture.request); err == nil {
		t.Fatal("S1/S2 Session drift was accepted")
	}
	current, err := fixture.store.InspectCheckpointGateCurrentV1(context.Background(), fixture.request.Run)
	if err != nil || current.State != contract.CheckpointGateInvalidatedV1 {
		t.Fatalf("failed S1/S2 did not invalidate the gate: %#v / %v", current, err)
	}

	lost := newGateFixture(t, "lost")
	lostStore := &lostReplyGateStoreV1{CheckpointGateStoreV1: lost.store}
	lostController, err := kernel.NewCheckpointGateControllerV1(lost.sessions, lostStore, lost.terminals, lost.clock)
	if err != nil {
		t.Fatal(err)
	}
	gate, snapshot, err := lostController.AcquireCheckpointGateV1(context.Background(), lost.request)
	if err != nil || gate.State != contract.CheckpointGateAcquiredV1 || snapshot.Ref != gate.Snapshot || lostStore.calls.Load() != 1 {
		t.Fatalf("lost create reply was not recovered by exact Inspect: %#v %#v %v", gate, snapshot, err)
	}
}

func TestCheckpointGateCAS64DifferentContentsSingleWinner(t *testing.T) {
	fixture := newGateFixture(t, "cas64")
	var winners atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			request := fixture.request
			request.StableID = request.StableID + "-" + string(rune('a'+i))
			controller, err := kernel.NewCheckpointGateControllerV1(fixture.sessions, fixture.store, fixture.terminals, fixture.clock)
			if err != nil {
				return
			}
			if _, _, err := controller.AcquireCheckpointGateV1(context.Background(), request); err == nil {
				winners.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("checkpoint Gate CAS winners=%d, want 1", winners.Load())
	}
}

func TestCheckpointGateTypedNilAndCrossAttemptReleaseFailClosed(t *testing.T) {
	var sessions *fakes.GovernedStoreV2
	if _, err := kernel.NewCheckpointGateControllerV1(sessions, reference.NewCheckpointGateStoreV1(), terminalReaderV1{}, time.Now); err == nil {
		t.Fatal("typed-nil Session reader was accepted")
	}
	fixture := newGateFixture(t, "release-drift")
	gate, _, err := fixture.controller.AcquireCheckpointGateV1(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := fixture.controller.BindCheckpointGateRuntimeV1(context.Background(), contract.BindCheckpointGateRuntimeRequestV1{Expected: gate.Ref, Runtime: fixture.runtime})
	if err != nil {
		t.Fatal(err)
	}
	other := fixture.runtime.Attempt
	other.ID = "another-attempt"
	other.Digest = core.DigestBytes([]byte("another-attempt"))
	if _, err := fixture.controller.ReleaseCheckpointGateV1(context.Background(), contract.ReleaseCheckpointGateRequestV1{Expected: bound.Ref, TerminalAttempt: other}); err == nil {
		t.Fatal("another Runtime Attempt released the Harness gate")
	}
	current, err := fixture.controller.InspectCheckpointGateCurrentV1(context.Background(), fixture.request.Run)
	if err != nil || current.Ref != bound.Ref {
		t.Fatalf("failed release changed current gate: %#v / %v", current, err)
	}
}

func TestCheckpointGateRuntimeCutPredecessorAndProgressedRetryNoABA(t *testing.T) {
	fixture := newGateFixture(t, "progressed-retry")
	gate, snapshot, err := fixture.controller.AcquireCheckpointGateV1(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := fixture.controller.BindCheckpointGateRuntimeV1(context.Background(), contract.BindCheckpointGateRuntimeRequestV1{Expected: gate.Ref, Runtime: fixture.runtime})
	if err != nil {
		t.Fatal(err)
	}
	replayedBound, replayedSnapshot, err := fixture.controller.AcquireCheckpointGateV1(context.Background(), fixture.request)
	if err != nil || !reflect.DeepEqual(replayedBound, bound) || !reflect.DeepEqual(replayedSnapshot, snapshot) {
		t.Fatalf("progressed bind retry resurrected revision 1: gate=%+v snapshot=%+v err=%v", replayedBound, replayedSnapshot, err)
	}
	released, err := fixture.controller.ReleaseCheckpointGateV1(context.Background(), contract.ReleaseCheckpointGateRequestV1{Expected: bound.Ref, TerminalAttempt: fixture.runtime.Attempt})
	if err != nil {
		t.Fatal(err)
	}
	replayedReleased, _, err := fixture.controller.AcquireCheckpointGateV1(context.Background(), fixture.request)
	if err != nil || !reflect.DeepEqual(replayedReleased, released) {
		t.Fatalf("progressed release retry resurrected earlier Gate: gate=%+v err=%v", replayedReleased, err)
	}

	invalid := fixture.runtime
	invalid.EffectCut.Attempt = invalid.Attempt
	if invalid.Validate(fixture.request.Run) == nil {
		t.Fatal("EffectCut anchored to the post-cut Attempt was accepted")
	}
	invalid = fixture.runtime
	invalid.Attempt.Revision++
	invalid.Attempt.Digest = core.DigestBytes([]byte("too-new-current-attempt"))
	if invalid.Validate(fixture.request.Run) == nil {
		t.Fatal("non-adjacent EffectCut Attempt revision was accepted")
	}
}

type gateFixtureV1 struct {
	now        time.Time
	clock      func() time.Time
	sessions   *fakes.GovernedStoreV2
	store      *reference.CheckpointGateStoreV1
	terminals  terminalReaderV1
	controller *kernel.CheckpointGateControllerV1
	creating   contract.GovernedSessionV4
	session    contract.GovernedSessionV4
	request    contract.AcquireCheckpointGateRequestV1
	runtime    contract.CheckpointRuntimeBindingV1
}

func newGateFixture(t *testing.T, suffix string) gateFixtureV1 {
	t.Helper()
	now := time.Unix(1_900_000_000, 0)
	v2, _ := testkit.GovernedFactsV2(now)
	creating := mustSealSessionV4(t, contract.GovernedSessionV4{ID: "session-" + suffix, Revision: 1, Run: v2.Run, Endpoint: v2.Endpoint, Phase: contract.SessionCreatingV2, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()})
	sessions := fakes.NewGovernedStoreV2()
	if _, err := sessions.CreateSessionV4(context.Background(), creating); err != nil {
		t.Fatal(err)
	}
	terminal := creating.Clone()
	terminal.Revision = 2
	terminal.Phase = contract.SessionTerminalV2
	terminal.CompletionClaim = contract.ClaimCancelled
	terminal.UpdatedUnixNano++
	terminal = mustSealSessionV4(t, terminal)
	cas, err := contract.SealSessionCASRequestV4(contract.SessionCASRequestV4{Run: creating.Run, SessionID: creating.ID, ExpectedRevision: creating.Revision, ExpectedDigest: creating.Digest, Next: terminal})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.CompareAndSwapSessionV4(context.Background(), cas); err != nil {
		t.Fatal(err)
	}
	attempt := runtimeports.CheckpointAttemptRefV2{TenantID: creating.Run.Scope.Identity.TenantID, ID: "checkpoint-attempt-" + suffix, Revision: 2, Digest: core.DigestBytes([]byte("attempt-" + suffix))}
	barrier := runtimeports.CheckpointBarrierLeaseRefV2{TenantID: attempt.TenantID, ID: "checkpoint-barrier-" + suffix, AttemptID: attempt.ID, Revision: 1, Digest: core.DigestBytes([]byte("barrier-" + suffix)), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	cutAttempt := attempt
	cutAttempt.Revision = 1
	cutAttempt.Digest = core.DigestBytes([]byte("attempt-before-cut-" + suffix))
	cut := runtimeports.EffectCutRefV2{ID: "effect-cut-" + suffix, Revision: 1, Attempt: cutAttempt, RootDigest: core.DigestBytes([]byte("cut-root-" + suffix)), Watermark: 1, Count: 0, Digest: core.DigestBytes([]byte("cut-" + suffix))}
	request := contract.AcquireCheckpointGateRequestV1{StableID: "harness-gate-" + suffix, IntentDigest: core.DigestBytes([]byte("intent-" + suffix)), Run: creating.Run, SessionID: terminal.ID, ExpectedSessionRevision: terminal.Revision, ExpectedSessionDigest: terminal.Digest, RequestedNotAfter: now.Add(time.Minute).UnixNano()}
	runtimeBinding := contract.CheckpointRuntimeBindingV1{Attempt: attempt, Barrier: barrier, EffectCut: cut}
	consistency := runtimeports.CheckpointConsistencyRefV2{ID: "consistency-" + suffix, Revision: 1, Attempt: attempt, Digest: core.DigestBytes([]byte("consistency-" + suffix))}
	terminalProjection, err := runtimeports.SealCheckpointAttemptTerminalCurrentProjectionV2(runtimeports.CheckpointAttemptTerminalCurrentProjectionV2{Attempt: attempt, Barrier: barrier, TerminalState: runtimeports.CheckpointAttemptConsistentV2, Consistency: &consistency, CheckedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	terminals := terminalReaderV1{projection: terminalProjection}
	store := reference.NewCheckpointGateStoreV1()
	clock := func() time.Time { return now }
	controller, err := kernel.NewCheckpointGateControllerV1(sessions, store, terminals, clock)
	if err != nil {
		t.Fatal(err)
	}
	return gateFixtureV1{now: now, clock: clock, sessions: sessions, store: store, terminals: terminals, controller: controller, creating: creating, session: terminal, request: request, runtime: runtimeBinding}
}

func mustSealSessionV4(t *testing.T, value contract.GovernedSessionV4) contract.GovernedSessionV4 {
	t.Helper()
	sealed, err := contract.SealGovernedSessionV4(value)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

type terminalReaderV1 struct {
	projection runtimeports.CheckpointAttemptTerminalCurrentProjectionV2
	err        error
}

func (r terminalReaderV1) InspectCheckpointAttemptTerminalCurrentV2(context.Context, runtimeports.CheckpointAttemptRefV2) (runtimeports.CheckpointAttemptTerminalCurrentProjectionV2, error) {
	return r.projection, r.err
}

type sequenceSessionReaderV1 struct {
	mu     sync.Mutex
	values []contract.GovernedSessionV4
}

func (r *sequenceSessionReaderV1) InspectSessionV4(context.Context, contract.RunRef, string) (contract.GovernedSessionV4, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.values) == 0 {
		return contract.GovernedSessionV4{}, errors.New("session sequence exhausted")
	}
	value := r.values[0]
	if len(r.values) > 1 {
		r.values = r.values[1:]
	}
	return value.Clone(), nil
}

type lostReplyGateStoreV1 struct {
	harnessports.CheckpointGateStoreV1
	calls atomic.Int64
}

func (s *lostReplyGateStoreV1) CreateCheckpointGateAndSnapshotV1(ctx context.Context, gate contract.CheckpointGateFactV1, snapshot contract.HarnessCheckpointSnapshotFactV1) (contract.CheckpointGateFactV1, contract.HarnessCheckpointSnapshotFactV1, error) {
	createdGate, createdSnapshot, err := s.CheckpointGateStoreV1.CreateCheckpointGateAndSnapshotV1(ctx, gate, snapshot)
	if err == nil && s.calls.Add(1) == 1 {
		return contract.CheckpointGateFactV1{}, contract.HarnessCheckpointSnapshotFactV1{}, errors.New("injected lost reply")
	}
	return createdGate, createdSnapshot, err
}

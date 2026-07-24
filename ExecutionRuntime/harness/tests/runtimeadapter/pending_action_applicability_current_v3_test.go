package runtimeadapter_test

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/kernel"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/runtimeadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestCommittedPendingActionApplicabilityCurrentReaderV3ConformanceExactLookup(t *testing.T) {
	fixture := newPendingActionApplicabilityFixtureV1(t)
	for name, ref := range map[string]runtimeports.OperationScopeEvidenceApplicabilityFactRefV3{
		"session": sessionApplicabilityRefV3ForTest(fixture.projection.SessionApplicability),
		"turn":    turnApplicabilityRefV3ForTest(fixture.projection.TurnApplicability),
	} {
		t.Run(name, func(t *testing.T) {
			current, err := fixture.adapter.InspectOperationScopeEvidenceApplicabilityCurrentV3(context.Background(), ref)
			if err != nil {
				t.Fatal(err)
			}
			if err := current.Validate(ref, fixture.request.ExecutionScopeDigest, fixture.now); err != nil {
				t.Fatalf("public current projection did not validate: %v", err)
			}
			if current.Fact != ref || current.ExecutionScopeDigest != fixture.request.ExecutionScopeDigest || current.ExpiresUnixNano != fixture.projection.ExpiresUnixNano {
				t.Fatalf("public current projection drifted: %#v", current)
			}
		})
	}
	if fixture.store.inspectCalls() != 6 || fixture.store.writeCalls() != 0 {
		t.Fatalf("binding creation plus two current reads must remain S1/S2 and zero-write: inspect=%d write=%d", fixture.store.inspectCalls(), fixture.store.writeCalls())
	}
}

func TestCommittedPendingActionApplicabilityCurrentReaderV3ConstructorIsImmutableAndConflictAware(t *testing.T) {
	fixture := newPendingActionApplicabilityFixtureV1(t)
	adapter, err := runtimeadapter.NewCommittedPendingActionApplicabilityCurrentReaderV3(fixture.reader, []contract.CommittedPendingActionApplicabilityBindingV1{fixture.binding, fixture.binding}, func() time.Time { return fixture.now })
	if err != nil {
		t.Fatalf("same exact Binding was not idempotent: %v", err)
	}
	ref := sessionApplicabilityRefV3ForTest(fixture.projection.SessionApplicability)
	originalLeaseEpoch := fixture.binding.Subject.Run.Scope.SandboxLease.Epoch
	fixture.binding.Subject.Run.Scope.SandboxLease.Epoch++
	fixture.binding.ExpectedSessionCoordinate.ID = "session:mutated"
	if _, err := adapter.InspectOperationScopeEvidenceApplicabilityCurrentV3(context.Background(), ref); err != nil {
		t.Fatalf("caller mutation escaped the constructor deep clone: %v", err)
	}
	if fixture.binding.Subject.Run.Scope.SandboxLease.Epoch == originalLeaseEpoch {
		t.Fatal("test did not mutate the caller-owned Binding")
	}

	laterRequest := fixture.request.Clone()
	laterRequest.CheckedAtUnixNano += int64(time.Hour)
	laterBinding, err := contract.SealCommittedPendingActionApplicabilityBindingV1(laterRequest, fixture.projection.SessionApplicability, fixture.projection.TurnApplicability)
	if err != nil {
		t.Fatal(err)
	}
	if laterBinding.Digest != fixture.bindingFromProjection.Digest || !reflect.DeepEqual(laterBinding.Subject, fixture.bindingFromProjection.Subject) {
		t.Fatal("observation time leaked into immutable Binding identity")
	}

	conflictingRequest := fixture.request.Clone()
	conflictingRequest.ExpectedPendingActionRef = "action-conflict"
	conflict, err := contract.SealCommittedPendingActionApplicabilityBindingV1(conflictingRequest, fixture.projection.SessionApplicability, fixture.projection.TurnApplicability)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeadapter.NewCommittedPendingActionApplicabilityCurrentReaderV3(fixture.reader, []contract.CommittedPendingActionApplicabilityBindingV1{fixture.bindingFromProjection, conflict}, func() time.Time { return fixture.now }); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same key with different request was not rejected as Conflict: %v", err)
	}

	drifted := fixture.bindingFromProjection.Clone()
	drifted.Digest = fixture.projection.Digest
	if _, err := runtimeadapter.NewCommittedPendingActionApplicabilityCurrentReaderV3(fixture.reader, []contract.CommittedPendingActionApplicabilityBindingV1{drifted}, func() time.Time { return fixture.now }); !core.HasReason(err, core.ReasonInvalidDigest) {
		t.Fatalf("drifted Binding digest was accepted: %v", err)
	}
}

func TestCommittedPendingActionApplicabilityCurrentReaderV3FaultsFailClosed(t *testing.T) {
	fixture := newPendingActionApplicabilityFixtureV1(t)
	sessionRef := sessionApplicabilityRefV3ForTest(fixture.projection.SessionApplicability)
	tests := []struct {
		name string
		ref  runtimeports.OperationScopeEvidenceApplicabilityFactRefV3
	}{
		{"unknown-id", func() runtimeports.OperationScopeEvidenceApplicabilityFactRefV3 {
			changed := sessionRef
			changed.ID = "session:unknown"
			return changed
		}()},
		{"wrong-revision", func() runtimeports.OperationScopeEvidenceApplicabilityFactRefV3 {
			changed := sessionRef
			changed.Revision++
			return changed
		}()},
		{"wrong-digest", func() runtimeports.OperationScopeEvidenceApplicabilityFactRefV3 {
			changed := sessionRef
			changed.Digest = fixture.projection.TurnApplicability.Digest
			return changed
		}()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := fixture.adapter.InspectOperationScopeEvidenceApplicabilityCurrentV3(context.Background(), test.ref); !core.HasCategory(err, core.ErrorNotFound) {
				t.Fatalf("unknown or drifted ref did not Fail Closed: %v", err)
			}
		})
	}

	driftReader := &driftingCommittedPendingActionReaderV1{base: fixture.reader}
	adapter, err := runtimeadapter.NewCommittedPendingActionApplicabilityCurrentReaderV3(driftReader, []contract.CommittedPendingActionApplicabilityBindingV1{fixture.bindingFromProjection}, func() time.Time { return fixture.now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.InspectOperationScopeEvidenceApplicabilityCurrentV3(context.Background(), sessionRef); err == nil {
		t.Fatal("Reader source drift was accepted")
	}

	rollbackClock := newPendingActionClockSequenceV1(fixture.now, fixture.now.Add(-time.Nanosecond))
	rollback, err := runtimeadapter.NewCommittedPendingActionApplicabilityCurrentReaderV3(fixture.reader, []contract.CommittedPendingActionApplicabilityBindingV1{fixture.bindingFromProjection}, rollbackClock.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rollback.InspectOperationScopeEvidenceApplicabilityCurrentV3(context.Background(), sessionRef); !core.HasReason(err, core.ReasonEffectFenceStale) {
		t.Fatalf("verification clock rollback did not Fail Closed: %v", err)
	}

	ttlCrossingClock := newPendingActionClockSequenceV1(fixture.now, time.Unix(0, fixture.projection.ExpiresUnixNano))
	ttlCrossing, err := runtimeadapter.NewCommittedPendingActionApplicabilityCurrentReaderV3(fixture.reader, []contract.CommittedPendingActionApplicabilityBindingV1{fixture.bindingFromProjection}, ttlCrossingClock.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ttlCrossing.InspectOperationScopeEvidenceApplicabilityCurrentV3(context.Background(), sessionRef); !core.HasReason(err, core.ReasonEffectFenceStale) {
		t.Fatalf("TTL crossing between Reader and Adapter verification did not Fail Closed: %v", err)
	}

	if _, err := runtimeadapter.NewCommittedPendingActionApplicabilityCurrentReaderV3(nil, []contract.CommittedPendingActionApplicabilityBindingV1{fixture.bindingFromProjection}, func() time.Time { return fixture.now }); err == nil {
		t.Fatal("nil underlying Reader was accepted")
	}
	if _, err := runtimeadapter.NewCommittedPendingActionApplicabilityCurrentReaderV3(fixture.reader, nil, func() time.Time { return fixture.now }); err == nil {
		t.Fatal("empty Binding set was accepted")
	}
}

func TestCommittedPendingActionApplicabilityCurrentReaderV3DelayedCallUsesFreshRealClock(t *testing.T) {
	fixture := newPendingActionApplicabilityFixtureV1(t)
	realStore := &pendingActionApplicabilitySessionStoreV1{session: fixture.store.session}
	realReader, err := kernel.NewCommittedPendingActionReaderV1(realStore, time.Now, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := runtimeadapter.NewCommittedPendingActionApplicabilityCurrentReaderV3(
		realReader,
		[]contract.CommittedPendingActionApplicabilityBindingV1{fixture.bindingFromProjection},
		time.Now,
	)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	ref := sessionApplicabilityRefV3ForTest(fixture.projection.SessionApplicability)
	current, err := adapter.InspectOperationScopeEvidenceApplicabilityCurrentV3(context.Background(), ref)
	if err != nil {
		t.Fatalf("constructor delay with a real increasing clock became stale: %v", err)
	}
	if current.ExpiresUnixNano <= time.Now().UnixNano() || realStore.inspectCalls() != 2 || realStore.writeCalls() != 0 {
		t.Fatalf("fresh delayed read did not preserve a live read-only lease: current=%#v reads=%d writes=%d", current, realStore.inspectCalls(), realStore.writeCalls())
	}
}

func TestCommittedPendingActionApplicabilityCurrentReaderV3ConcurrentReads(t *testing.T) {
	fixture := newPendingActionApplicabilityFixtureV1(t)
	ref := turnApplicabilityRefV3ForTest(fixture.projection.TurnApplicability)
	const workers = 64
	start := make(chan struct{})
	errs := make(chan error, workers)
	digests := make(chan core.Digest, workers)
	var wait sync.WaitGroup
	wait.Add(workers)
	for range workers {
		go func() {
			defer wait.Done()
			<-start
			current, err := fixture.adapter.InspectOperationScopeEvidenceApplicabilityCurrentV3(context.Background(), ref)
			errs <- err
			digests <- current.Digest
		}()
	}
	close(start)
	wait.Wait()
	close(errs)
	close(digests)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var digest core.Digest
	for current := range digests {
		if digest == "" {
			digest = current
		}
		if current != digest {
			t.Fatal("concurrent immutable Binding reads returned different digests")
		}
	}
	if fixture.store.inspectCalls() != 2+workers*2 || fixture.store.writeCalls() != 0 {
		t.Fatalf("concurrent adapter reads were not read-only S1/S2: inspect=%d write=%d", fixture.store.inspectCalls(), fixture.store.writeCalls())
	}
	methodType := reflect.TypeOf(fixture.adapter)
	for _, forbidden := range []string{"Register", "Delete", "Replace", "CompareAndSwap"} {
		if _, exists := methodType.MethodByName(forbidden); exists {
			t.Fatalf("runtime mutation method %q leaked from immutable Adapter", forbidden)
		}
	}
}

func TestCommittedPendingActionApplicabilityCurrentReaderV3ConcurrentConstructionConflicts(t *testing.T) {
	fixture := newPendingActionApplicabilityFixtureV1(t)
	conflictingRequest := fixture.request.Clone()
	conflictingRequest.ExpectedPendingActionRef = "action-concurrent-conflict"
	conflict, err := contract.SealCommittedPendingActionApplicabilityBindingV1(conflictingRequest, fixture.projection.SessionApplicability, fixture.projection.TurnApplicability)
	if err != nil {
		t.Fatal(err)
	}
	const workers = 32
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wait sync.WaitGroup
	wait.Add(workers)
	for range workers {
		go func() {
			defer wait.Done()
			<-start
			_, err := runtimeadapter.NewCommittedPendingActionApplicabilityCurrentReaderV3(
				fixture.reader,
				[]contract.CommittedPendingActionApplicabilityBindingV1{fixture.bindingFromProjection, conflict},
				func() time.Time { return fixture.now },
			)
			errs <- err
		}()
	}
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		if !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("concurrent conflicting construction did not deterministically fail: %v", err)
		}
	}
}

type pendingActionApplicabilityFixtureV1 struct {
	now                   time.Time
	request               contract.InspectCommittedPendingActionCurrentRequestV1
	projection            contract.CommittedPendingActionCurrentV1
	binding               contract.CommittedPendingActionApplicabilityBindingV1
	bindingFromProjection contract.CommittedPendingActionApplicabilityBindingV1
	store                 *pendingActionApplicabilitySessionStoreV1
	reader                harnessports.CommittedPendingActionReaderV1
	adapter               *runtimeadapter.CommittedPendingActionApplicabilityCurrentReaderV3
}

func newPendingActionApplicabilityFixtureV1(t *testing.T) pendingActionApplicabilityFixtureV1 {
	t.Helper()
	now := time.Unix(2_000_000_300, 0)
	session, candidate := testkit.GovernedFactsV2(now.Add(-time.Minute))
	candidateRef, err := candidate.RefV2()
	if err != nil {
		t.Fatal(err)
	}
	pending, err := contract.NewPendingActionV2("action-applicability", "custom.tool/execute", candidate.Input, candidateRef)
	if err != nil {
		t.Fatal(err)
	}
	turn := contract.SettledTurnResultV2{ContractVersion: contract.SettledTurnResultContractV2, Candidate: candidateRef, State: contract.SettledTurnActionRequiredV2, Action: &pending}
	settled, _ := testkit.GovernedSettledAttemptRefsV2(now.Add(-time.Minute), candidate, turn, runtimeports.OperationSettlementAppliedV3)
	session.Revision = 9
	session.Phase = contract.SessionWaitingActionV2
	session.Turn = 1
	session.Candidate = nil
	session.DomainReservation = nil
	session.Execution = &settled
	session.PendingAction = &pending
	session.UpdatedUnixNano = now.Add(-time.Second).UnixNano()
	if err := session.Validate(); err != nil {
		t.Fatal(err)
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(session.Run.Scope)
	if err != nil {
		t.Fatal(err)
	}
	request := contract.InspectCommittedPendingActionCurrentRequestV1{
		ContractVersion: contract.CommittedPendingActionReaderContractVersionV1,
		Run:             session.Run, ExecutionScopeDigest: scopeDigest, SessionID: session.ID,
		ExpectedSessionRevision: session.Revision, ExpectedTurn: session.Turn,
		ExpectedPendingActionRef: pending.Ref, ExpectedPendingActionDigest: pending.RequestDigest,
		CheckedAtUnixNano: now.UnixNano(),
	}
	store := &pendingActionApplicabilitySessionStoreV1{session: session}
	reader, err := kernel.NewCommittedPendingActionReaderV1(store, func() time.Time { return now }, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := reader.InspectCommittedPendingActionCurrentV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := contract.SealCommittedPendingActionApplicabilityBindingV1(request, projection.SessionApplicability, projection.TurnApplicability)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := runtimeadapter.NewCommittedPendingActionApplicabilityCurrentReaderV3(reader, []contract.CommittedPendingActionApplicabilityBindingV1{binding}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return pendingActionApplicabilityFixtureV1{now: now, request: request, projection: projection, binding: binding.Clone(), bindingFromProjection: binding, store: store, reader: reader, adapter: adapter}
}

type pendingActionApplicabilitySessionStoreV1 struct {
	mu      sync.Mutex
	session contract.GovernedSessionV2
	reads   int
	writes  int
}

func (s *pendingActionApplicabilitySessionStoreV1) CreateSessionV2(context.Context, contract.GovernedSessionV2) (contract.GovernedSessionV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writes++
	return contract.GovernedSessionV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "unexpected Session write")
}

func (s *pendingActionApplicabilitySessionStoreV1) InspectSessionV2(context.Context, contract.RunRef, string) (contract.GovernedSessionV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reads++
	return s.session, nil
}

func (s *pendingActionApplicabilitySessionStoreV1) CompareAndSwapSessionV2(context.Context, harnessports.SessionCASRequestV2) (contract.GovernedSessionV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writes++
	return contract.GovernedSessionV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "unexpected Session CAS")
}

func (s *pendingActionApplicabilitySessionStoreV1) inspectCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reads
}

func (s *pendingActionApplicabilitySessionStoreV1) writeCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writes
}

type driftingCommittedPendingActionReaderV1 struct {
	base harnessports.CommittedPendingActionReaderV1
}

type pendingActionClockSequenceV1 struct {
	mu    sync.Mutex
	times []time.Time
	index int
}

func newPendingActionClockSequenceV1(times ...time.Time) *pendingActionClockSequenceV1 {
	return &pendingActionClockSequenceV1{times: append([]time.Time(nil), times...)}
}

func (c *pendingActionClockSequenceV1) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.times) == 0 {
		return time.Time{}
	}
	index := c.index
	if index >= len(c.times) {
		index = len(c.times) - 1
	}
	c.index++
	return c.times[index]
}

func (r *driftingCommittedPendingActionReaderV1) InspectCommittedPendingActionCurrentV1(ctx context.Context, request contract.InspectCommittedPendingActionCurrentRequestV1) (contract.CommittedPendingActionCurrentV1, error) {
	current, err := r.base.InspectCommittedPendingActionCurrentV1(ctx, request)
	if err != nil {
		return contract.CommittedPendingActionCurrentV1{}, err
	}
	current.SessionApplicability.Digest = current.TurnApplicability.Digest
	return current, nil
}

func sessionApplicabilityRefV3ForTest(coordinate contract.CommittedPendingActionSessionApplicabilityCoordinateV1) runtimeports.OperationScopeEvidenceApplicabilityFactRefV3 {
	return runtimeports.OperationScopeEvidenceApplicabilityFactRefV3{Kind: coordinate.Kind, ID: coordinate.ID, Revision: coordinate.Revision, Digest: coordinate.Digest}
}

func turnApplicabilityRefV3ForTest(coordinate contract.CommittedPendingActionTurnApplicabilityCoordinateV1) runtimeports.OperationScopeEvidenceApplicabilityFactRefV3 {
	return runtimeports.OperationScopeEvidenceApplicabilityFactRefV3{Kind: coordinate.Kind, ID: coordinate.ID, Revision: coordinate.Revision, Digest: coordinate.Digest}
}

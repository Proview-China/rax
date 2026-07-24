package application_test

import (
	"context"
	"sync"
	"testing"
	"time"

	application "github.com/Proview-China/rax/ExecutionRuntime/application"
	applicationconformance "github.com/Proview-China/rax/ExecutionRuntime/application/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/application/fakes"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestAgentActivationCoordinationFixedSequenceAndAppendOnlyFactV1(t *testing.T) {
	now := time.Unix(1_800_110_000, 0)
	coordinator, store, steps, start := newAgentActivationCoordinatorFixtureV1(t, now)
	result, err := coordinator.StartOrInspectAgentActivationV1(context.Background(), start)
	if err != nil {
		t.Fatal(err)
	}
	if err := result.ValidateFor(start, now); err != nil {
		t.Fatal(err)
	}
	fact, err := store.InspectAgentActivationCoordinationV1(context.Background(), start.ActivationID)
	if err != nil || fact.Validate() != nil || fact.Result == nil || fact.Result.ResultDigest != result.ResultDigest {
		t.Fatalf("completed Agent activation Fact is invalid: %#v %v", fact, err)
	}
	if err := applicationconformance.CheckAgentActivationCoordinationV1(start, fact, now); err != nil {
		t.Fatal(err)
	}
	if len(fact.Events) != len(contract.AgentActivationStepOrderV1())*3 {
		t.Fatalf("Agent activation event count=%d", len(fact.Events))
	}
	for index, step := range contract.AgentActivationStepOrderV1() {
		events := fact.Events[index*3 : index*3+3]
		if events[0].Step != step || events[0].State != contract.AgentActivationStepIntentRecordedV1 || events[1].State != contract.AgentActivationStepInvocationRecordedV1 || events[2].State != contract.AgentActivationStepResultRecordedV1 {
			t.Fatalf("Agent activation step %s events drifted: %#v", step, events)
		}
		startCalls, commits, inspects := steps[index].CountsV1()
		if startCalls != 1 || commits != 1 || inspects != 0 {
			t.Fatalf("Agent activation step %s calls=%d commits=%d inspects=%d", step, startCalls, commits, inspects)
		}
	}
	inspected, err := coordinator.InspectAgentActivationV1(context.Background(), start)
	if err != nil || inspected.ResultDigest != result.ResultDigest {
		t.Fatalf("Agent activation coordinator exact Inspect failed: %#v %v", inspected, err)
	}
}

func TestAgentActivationCoordinationLostRepliesInspectOriginalAttemptsV1(t *testing.T) {
	now := time.Unix(1_800_110_100, 0)
	coordinator, store, steps, start := newAgentActivationCoordinatorFixtureV1(t, now)
	store.LoseNextEnsureReplyV1()
	steps[4].LoseNextStartReplyV1()
	result, err := coordinator.StartOrInspectAgentActivationV1(context.Background(), start)
	if err != nil {
		t.Fatal(err)
	}
	if err := result.ValidateFor(start, now); err != nil {
		t.Fatal(err)
	}
	startCalls, commits, inspects := steps[4].CountsV1()
	if startCalls != 1 || commits != 1 || inspects != 1 {
		t.Fatalf("ActivationCommit lost reply was redispatched: calls=%d commits=%d inspects=%d", startCalls, commits, inspects)
	}
	fact, err := store.InspectAgentActivationCoordinationV1(context.Background(), start.ActivationID)
	if err != nil {
		t.Fatal(err)
	}
	foundUnknown := false
	for _, event := range fact.Events {
		if event.Step == contract.AgentActivationCommitV1 && event.State == contract.AgentActivationStepOutcomeUnknownV1 {
			foundUnknown = true
		}
	}
	if !foundUnknown {
		t.Fatal("ActivationCommit unknown outcome was not persisted before Inspect recovery")
	}
	ensureCommits, casCommits := store.CountsV1()
	if ensureCommits != 1 || casCommits != uint64(len(fact.Events)-1) {
		t.Fatalf("coordination lost reply changed commit cardinality: ensure=%d cas=%d events=%d", ensureCommits, casCommits, len(fact.Events))
	}
}

func TestAgentActivationCoordinationStrictCASGatesDispatchAcross64CoordinatorsV1(t *testing.T) {
	now := time.Unix(1_800_110_150, 0)
	first, store, steps, start := newAgentActivationCoordinatorFixtureV1(t, now)
	configured := activationStepPortsFromFakesV1(steps)
	const workers = 64
	coordinators := make([]*application.AgentActivationCoordinatorV1, workers)
	coordinators[0] = first
	for index := 1; index < workers; index++ {
		var err error
		coordinators[index], err = application.NewAgentActivationCoordinatorV1(store, configured, func() time.Time { return now })
		if err != nil {
			t.Fatal(err)
		}
	}
	var wait sync.WaitGroup
	for index := range coordinators {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, _ = coordinators[index].StartOrInspectAgentActivationV1(context.Background(), start)
		}(index)
	}
	wait.Wait()
	fact, err := store.InspectAgentActivationCoordinationV1(context.Background(), start.ActivationID)
	if err != nil || fact.Result == nil {
		t.Fatalf("64 coordinators did not complete one activation: %#v %v", fact, err)
	}
	for index, step := range steps {
		starts, commits, _ := step.CountsV1()
		if starts != 1 || commits != 1 {
			t.Fatalf("step %d repeated across coordinators: starts=%d commits=%d", index, starts, commits)
		}
	}
}

func TestAgentActivationCoordinationCASLostReplyIsInspectOnlyV1(t *testing.T) {
	now := time.Unix(1_800_110_175, 0)
	coordinator, store, steps, start := newAgentActivationCoordinatorFixtureV1(t, now)
	store.LoseNextCASReplyV1()
	if _, err := coordinator.StartOrInspectAgentActivationV1(context.Background(), start); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("lost CAS reply did not stop at Inspect-only: %v", err)
	}
	starts, commits, inspects := steps[0].CountsV1()
	if starts != 0 || commits != 0 || inspects != 1 {
		t.Fatalf("lost CAS reply dispatched: starts=%d commits=%d inspects=%d", starts, commits, inspects)
	}
	restarted, err := application.NewAgentActivationCoordinatorV1(store, activationStepPortsFromFakesV1(steps), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.StartOrInspectAgentActivationV1(context.Background(), start); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("restart did not remain Inspect-only: %v", err)
	}
	starts, commits, _ = steps[0].CountsV1()
	if starts != 0 || commits != 0 {
		t.Fatalf("restart redispatched persisted invocation: starts=%d commits=%d", starts, commits)
	}
}

func TestAgentActivationCoordinationSameNextCASReplayConflictsV1(t *testing.T) {
	now := time.Unix(1_800_110_190, 0)
	_, store, _, start := newAgentActivationCoordinatorFixtureV1(t, now)
	stepRequest, err := contract.SealAgentActivationStepRequestV1(contract.AgentActivationStepRequestV1{ActivationID: start.ActivationID, StartRequestDigest: start.RequestDigest, Step: contract.AgentActivationPreflightV1, RequestedNotAfterUnixNano: start.RequestedNotAfterUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	event1, err := contract.SealAgentActivationStepEventV1(contract.AgentActivationStepEventV1{Sequence: 1, Step: stepRequest.Step, State: contract.AgentActivationStepIntentRecordedV1, AttemptID: stepRequest.AttemptID, RequestDigest: stepRequest.RequestDigest, RecordedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	current, err := contract.SealAgentActivationCoordinationFactV1(contract.AgentActivationCoordinationFactV1{ActivationID: start.ActivationID, Revision: 1, Request: start, Events: []contract.AgentActivationStepEventV1{event1}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.EnsureAgentActivationCoordinationV1(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	event2, err := contract.SealAgentActivationStepEventV1(contract.AgentActivationStepEventV1{Sequence: 2, Step: stepRequest.Step, State: contract.AgentActivationStepInvocationRecordedV1, AttemptID: stepRequest.AttemptID, RequestDigest: stepRequest.RequestDigest, RecordedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	next, err := contract.SealAgentActivationCoordinationFactV1(contract.AgentActivationCoordinationFactV1{ActivationID: start.ActivationID, Revision: 2, Request: start, Events: []contract.AgentActivationStepEventV1{event1, event2}})
	if err != nil {
		t.Fatal(err)
	}
	request := applicationports.AgentActivationCoordinationCASRequestV1{ActivationID: start.ActivationID, ExpectedRevision: current.Revision, ExpectedDigest: current.Digest, Next: next}
	if _, err = store.CompareAndSwapAgentActivationCoordinationV1(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CompareAndSwapAgentActivationCoordinationV1(context.Background(), request); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same-next CAS replay succeeded: %v", err)
	}
}

func TestAgentActivationCoordinationNoCommitCASFailuresExecuteZeroV1(t *testing.T) {
	for index, category := range []core.ErrorCategory{core.ErrorConflict, core.ErrorUnavailable, core.ErrorIndeterminate} {
		now := time.Unix(1_800_110_195+int64(index), 0)
		_, base, steps, start := newAgentActivationCoordinatorFixtureV1(t, now)
		fault := &activationCASNoCommitPortV1{base: base, category: category, fail: true}
		coordinator, err := application.NewAgentActivationCoordinatorV1(fault, activationStepPortsFromFakesV1(steps), func() time.Time { return now })
		if err != nil {
			t.Fatal(err)
		}
		if _, err = coordinator.StartOrInspectAgentActivationV1(context.Background(), start); !core.HasCategory(err, category) {
			t.Fatalf("no-commit %s: %v", category, err)
		}
		starts, commits, _ := steps[0].CountsV1()
		if starts != 0 || commits != 0 {
			t.Fatalf("no-commit %s dispatched: starts=%d commits=%d", category, starts, commits)
		}
		persisted, err := base.InspectAgentActivationCoordinationV1(context.Background(), start.ActivationID)
		if err != nil || persisted.Revision != 1 || persisted.Events[len(persisted.Events)-1].State != contract.AgentActivationStepIntentRecordedV1 {
			t.Fatalf("no-commit %s persisted mutation: %#v %v", category, persisted, err)
		}
		restarted, err := application.NewAgentActivationCoordinatorV1(base, activationStepPortsFromFakesV1(steps), func() time.Time { return now })
		if err != nil {
			t.Fatal(err)
		}
		if _, err = restarted.InspectAgentActivationV1(context.Background(), start); !core.HasCategory(err, core.ErrorIndeterminate) {
			t.Fatalf("restart Inspect %s: %v", category, err)
		}
		starts, commits, _ = steps[0].CountsV1()
		if starts != 0 || commits != 0 {
			t.Fatalf("restart Inspect %s executed: starts=%d commits=%d", category, starts, commits)
		}
	}
}

func TestAgentActivationCoordinationPersistedInvocationNeverBlindRedispatchesV1(t *testing.T) {
	now := time.Unix(1_800_110_200, 0)
	coordinator, store, steps, start := newAgentActivationCoordinatorFixtureV1(t, now)
	stepRequest, err := contract.SealAgentActivationStepRequestV1(contract.AgentActivationStepRequestV1{
		ActivationID: start.ActivationID, StartRequestDigest: start.RequestDigest, Step: contract.AgentActivationPreflightV1,
		RequestedNotAfterUnixNano: start.RequestedNotAfterUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	events := make([]contract.AgentActivationStepEventV1, 0, 2)
	for index, state := range []contract.AgentActivationStepEventStateV1{contract.AgentActivationStepIntentRecordedV1, contract.AgentActivationStepInvocationRecordedV1} {
		event, err := contract.SealAgentActivationStepEventV1(contract.AgentActivationStepEventV1{
			Sequence: uint32(index + 1), Step: stepRequest.Step, State: state, AttemptID: stepRequest.AttemptID,
			RequestDigest: stepRequest.RequestDigest, RecordedUnixNano: now.UnixNano(),
		})
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	fact, err := contract.SealAgentActivationCoordinationFactV1(contract.AgentActivationCoordinationFactV1{
		ActivationID: start.ActivationID, Revision: 2, Request: start, Events: events,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsureAgentActivationCoordinationV1(context.Background(), fact); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.StartOrInspectAgentActivationV1(context.Background(), start); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("persisted invocation without result was blindly redispatched: %v", err)
	}
	startCalls, commits, inspects := steps[0].CountsV1()
	if startCalls != 0 || commits != 0 || inspects != 1 {
		t.Fatalf("persisted invocation recovery calls=%d commits=%d inspects=%d", startCalls, commits, inspects)
	}
}

func TestAgentActivationCoordinationLinearizes64WorkersV1(t *testing.T) {
	now := time.Unix(1_800_110_300, 0)
	coordinator, store, steps, start := newAgentActivationCoordinatorFixtureV1(t, now)
	const workers = 64
	var wait sync.WaitGroup
	errs := make(chan error, workers)
	digests := make(chan core.Digest, workers)
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := coordinator.StartOrInspectAgentActivationV1(context.Background(), start)
			if err != nil {
				errs <- err
				return
			}
			digests <- result.ResultDigest
		}()
	}
	wait.Wait()
	close(errs)
	close(digests)
	for err := range errs {
		t.Fatal(err)
	}
	var expected core.Digest
	for digest := range digests {
		if expected == "" {
			expected = digest
		}
		if digest != expected {
			t.Fatalf("64 Agent activation workers returned different results: %s != %s", digest, expected)
		}
	}
	fact, err := store.InspectAgentActivationCoordinationV1(context.Background(), start.ActivationID)
	if err != nil || fact.Result == nil || fact.Result.ResultDigest != expected {
		t.Fatalf("64 Agent activation workers did not publish one result: %#v %v", fact.Result, err)
	}
	for index, step := range steps {
		startCalls, commits, _ := step.CountsV1()
		if startCalls != 1 || commits != 1 {
			t.Fatalf("64 workers repeated step %d: calls=%d commits=%d", index, startCalls, commits)
		}
	}
}

func newAgentActivationCoordinatorFixtureV1(t *testing.T, now time.Time) (*application.AgentActivationCoordinatorV1, *fakes.AgentActivationCoordinationStoreV1, []*fakes.AgentActivationStepV1, contract.AgentActivationStartRequestV1) {
	t.Helper()
	_, start, activation, _, _ := newAgentLifecycleFixtureV1(t, now)
	stepOrder := contract.AgentActivationStepOrderV1()
	steps := make([]*fakes.AgentActivationStepV1, len(stepOrder))
	ports := make([]applicationports.AgentActivationStepPortV1, len(stepOrder))
	for index, step := range stepOrder {
		step := step
		port, err := fakes.NewAgentActivationStepV1(step, func(request contract.AgentActivationStepRequestV1) (contract.AgentActivationStepResultV1, error) {
			result := contract.AgentActivationStepResultV1{
				ActivationID: request.ActivationID, Step: request.Step, AttemptID: request.AttemptID, RequestDigest: request.RequestDigest,
				Current:         lifecycleOwnerCurrentV1("step-"+string(step), "step-current-"+string(step), activation.ExpiresUnixNano),
				CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: activation.ExpiresUnixNano,
			}
			switch step {
			case contract.AgentActivationSandboxAllocateV1:
				lease := activation.SandboxLease
				leaseCurrent := activation.SandboxLeaseCurrent
				result.SandboxState, result.SandboxLease, result.SandboxLeaseCurrent = contract.AgentActivationSandboxReservedQuarantinedV1, &lease, &leaseCurrent
			case contract.AgentActivationCommitV1:
				lease := activation.SandboxLease
				scope := activation.ExecutionScope
				current := activation.ActivationCurrent
				result.SandboxLease, result.ExecutionScope, result.ActivationCurrent = &lease, &scope, &current
			case contract.AgentActivationSandboxActivateV1:
				lease := activation.SandboxLease
				current := activation.SandboxActiveCurrent
				result.SandboxState, result.SandboxLease, result.SandboxActiveCurrent = "active", &lease, &current
			case contract.AgentActivationReadyInspectV1:
				current := activation.ExecutionReadyCurrent
				result.ExecutionReadyCurrent = &current
			}
			return contract.SealAgentActivationStepResultV1(result)
		})
		if err != nil {
			t.Fatal(err)
		}
		steps[index], ports[index] = port, port
	}
	configured := applicationports.AgentActivationStepPortsV1{
		Preflight: ports[0], Snapshot: ports[1], IdentityBudget: ports[2], SandboxAllocate: ports[3],
		ActivationCommit: ports[4], SandboxActivate: ports[5], ExecutionOpen: ports[6], ReadyInspect: ports[7],
	}
	store := fakes.NewAgentActivationCoordinationStoreV1()
	coordinator, err := application.NewAgentActivationCoordinatorV1(store, configured, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return coordinator, store, steps, start
}

func activationStepPortsFromFakesV1(steps []*fakes.AgentActivationStepV1) applicationports.AgentActivationStepPortsV1 {
	return applicationports.AgentActivationStepPortsV1{Preflight: steps[0], Snapshot: steps[1], IdentityBudget: steps[2], SandboxAllocate: steps[3], ActivationCommit: steps[4], SandboxActivate: steps[5], ExecutionOpen: steps[6], ReadyInspect: steps[7]}
}

type activationCASNoCommitPortV1 struct {
	base     *fakes.AgentActivationCoordinationStoreV1
	category core.ErrorCategory
	fail     bool
}

func (p *activationCASNoCommitPortV1) EnsureAgentActivationCoordinationV1(ctx context.Context, fact contract.AgentActivationCoordinationFactV1) (contract.AgentActivationCoordinationFactV1, error) {
	return p.base.EnsureAgentActivationCoordinationV1(ctx, fact)
}
func (p *activationCASNoCommitPortV1) InspectAgentActivationCoordinationV1(ctx context.Context, id string) (contract.AgentActivationCoordinationFactV1, error) {
	return p.base.InspectAgentActivationCoordinationV1(ctx, id)
}
func (p *activationCASNoCommitPortV1) CompareAndSwapAgentActivationCoordinationV1(ctx context.Context, request applicationports.AgentActivationCoordinationCASRequestV1) (contract.AgentActivationCoordinationFactV1, error) {
	if p.fail {
		p.fail = false
		reason := core.ReasonRevisionConflict
		if p.category == core.ErrorIndeterminate {
			reason = core.ReasonEffectUnknownOutcome
		} else if p.category == core.ErrorUnavailable {
			reason = core.ReasonEvidenceUnavailable
		}
		return contract.AgentActivationCoordinationFactV1{}, core.NewError(p.category, reason, "injected no-commit CAS failure")
	}
	return p.base.CompareAndSwapAgentActivationCoordinationV1(ctx, request)
}

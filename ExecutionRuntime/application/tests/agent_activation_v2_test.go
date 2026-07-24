package application_test

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	application "github.com/Proview-China/rax/ExecutionRuntime/application"
	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/application/fakes"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	applicationsqlite "github.com/Proview-China/rax/ExecutionRuntime/application/storage/sqlite"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestAgentActivationV2CompletesEightExactSteps(t *testing.T) {
	fx := newActivationFixtureV2(t)
	result, err := fx.coordinator.StartOrInspectAgentActivationV2(context.Background(), fx.request)
	if err != nil {
		t.Fatal(err)
	}
	if err = result.ValidateFor(fx.request, fx.now); err != nil {
		t.Fatal(err)
	}
	fact, err := fx.store.InspectAgentActivationCoordinationV2(context.Background(), fx.request.ActivationID)
	if err != nil {
		t.Fatal(err)
	}
	if fact.Result == nil || len(fact.Events) != 24 || fact.Revision != 24 {
		t.Fatalf("unexpected completed Fact: revision=%d events=%d", fact.Revision, len(fact.Events))
	}
	for index, step := range contract.AgentActivationStepOrderV2() {
		events := fact.Events[index*3 : index*3+3]
		if events[0].Step != step || events[0].State != contract.AgentActivationStepIntentRecordedV2 || events[1].State != contract.AgentActivationStepInvocationRecordedV2 || events[2].State != contract.AgentActivationStepResultRecordedV2 {
			t.Fatalf("step %s event sequence drifted", step)
		}
		_, starts, commits, _ := fx.steps[index].CountsV2()
		if starts != 1 || commits != 1 {
			t.Fatalf("step %s start/commit=%d/%d", step, starts, commits)
		}
	}
}

func TestAgentActivationV2LostOwnerReplyInspectsOriginalAttempt(t *testing.T) {
	fx := newActivationFixtureV2(t)
	fx.steps[3].LoseNextStartReplyV2()
	result, err := fx.coordinator.StartOrInspectAgentActivationV2(context.Background(), fx.request)
	if err != nil {
		t.Fatal(err)
	}
	if err = result.ValidateFor(fx.request, fx.now); err != nil {
		t.Fatal(err)
	}
	_, starts, commits, inspects := fx.steps[3].CountsV2()
	if starts != 1 || commits != 1 || inspects != 1 {
		t.Fatalf("allocate calls=%d/%d/%d", starts, commits, inspects)
	}
	fact, _ := fx.store.InspectAgentActivationCoordinationV2(context.Background(), fx.request.ActivationID)
	foundUnknown := false
	for _, event := range fact.Events {
		if event.Step == contract.AgentActivationSandboxAllocateV2 && event.State == contract.AgentActivationStepOutcomeUnknownV2 {
			foundUnknown = true
		}
	}
	if !foundUnknown {
		t.Fatal("lost reply did not persist outcome_unknown")
	}
}

func TestAgentActivationV2InvocationCASLostReplyIsInspectOnly(t *testing.T) {
	fx := newActivationFixtureV2(t)
	// First CAS is intent->invocation for Preflight.
	fx.store.LoseNextCASReplyV2()
	_, err := fx.coordinator.StartOrInspectAgentActivationV2(context.Background(), fx.request)
	if !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("expected inspect-only NotFound, got %v", err)
	}
	_, starts, commits, inspects := fx.steps[0].CountsV2()
	if starts != 0 || commits != 0 || inspects != 1 {
		t.Fatalf("CAS recovery dispatched: %d/%d/%d", starts, commits, inspects)
	}
	restarted, err := application.NewAgentActivationCoordinatorV2(fx.store, fx.ports, func() time.Time { return fx.now })
	if err != nil {
		t.Fatal(err)
	}
	_, err = restarted.StartOrInspectAgentActivationV2(context.Background(), fx.request)
	if !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("restart should remain inspect-only, got %v", err)
	}
	_, starts, _, inspects = fx.steps[0].CountsV2()
	if starts != 0 || inspects != 2 {
		t.Fatalf("restart dispatched: starts=%d inspects=%d", starts, inspects)
	}
}

func TestAgentActivationV2CASNoCommitFailuresNeverDispatch(t *testing.T) {
	for _, category := range []core.ErrorCategory{core.ErrorConflict, core.ErrorUnavailable, core.ErrorIndeterminate} {
		t.Run(string(category), func(t *testing.T) {
			fx := newActivationFixtureV2(t)
			fx.store.FailNextCASBeforeCommitV2(category)
			_, err := fx.coordinator.StartOrInspectAgentActivationV2(context.Background(), fx.request)
			if !core.HasCategory(err, category) {
				t.Fatalf("expected %s, got %v", category, err)
			}
			_, starts, _, _ := fx.steps[0].CountsV2()
			if starts != 0 {
				t.Fatalf("failed CAS dispatched %d starts", starts)
			}
			fact, inspectErr := fx.store.InspectAgentActivationCoordinationV2(context.Background(), fx.request.ActivationID)
			if inspectErr != nil || fact.Events[len(fact.Events)-1].State != contract.AgentActivationStepIntentRecordedV2 {
				t.Fatalf("failed CAS changed state: %v", inspectErr)
			}
		})
	}
}

func TestAgentActivationV2SixtyFourCoordinatorsShareOneStartPerStep(t *testing.T) {
	fx := newActivationFixtureV2(t)
	const workers = 64
	coordinators := make([]*application.AgentActivationCoordinatorV2, workers)
	for index := range coordinators {
		var err error
		coordinators[index], err = application.NewAgentActivationCoordinatorV2(fx.store, fx.ports, func() time.Time { return fx.now })
		if err != nil {
			t.Fatal(err)
		}
	}
	var successes atomic.Uint64
	var wg sync.WaitGroup
	for index := range coordinators {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			if _, err := coordinators[index].StartOrInspectAgentActivationV2(context.Background(), fx.request); err == nil {
				successes.Add(1)
			}
		}(index)
	}
	wg.Wait()
	if successes.Load() == 0 {
		t.Fatal("no coordinator completed")
	}
	for index, step := range fx.steps {
		_, starts, commits, _ := step.CountsV2()
		if starts != 1 || commits != 1 {
			t.Fatalf("step %d starts/commits=%d/%d", index, starts, commits)
		}
	}
}

func TestAgentActivationV2RejectsProposedCommittedScopeTypePun(t *testing.T) {
	fx := newActivationFixtureV2(t)
	request := fx.request
	request.ProposedScope.Instance.Epoch++
	request.RequestDigest = ""
	request, err := contract.SealAgentActivationStartRequestV2(request)
	if err != nil {
		t.Fatal(err)
	}
	result, err := fx.coordinator.StartOrInspectAgentActivationV2(context.Background(), request)
	if err == nil || result.ResultDigest != "" {
		t.Fatalf("instance-spliced request unexpectedly completed: %v", err)
	}
}

func TestAgentActivationV2FactRejectsInvocationCoordinationAndIntentSplice(t *testing.T) {
	fx := newActivationFixtureV2(t)
	if _, err := fx.coordinator.StartOrInspectAgentActivationV2(context.Background(), fx.request); err != nil {
		t.Fatal(err)
	}
	fact, err := fx.store.InspectAgentActivationCoordinationV2(context.Background(), fx.request.ActivationID)
	if err != nil {
		t.Fatal(err)
	}
	invocationIndex := -1
	for index := range fact.Events {
		if fact.Events[index].State == contract.AgentActivationStepInvocationRecordedV2 {
			invocationIndex = index
			break
		}
	}
	if invocationIndex <= 0 {
		t.Fatal("invocation event missing")
	}
	for name, mutate := range map[string]func(*contract.AgentActivationStepRequestV2){
		"coordination_revision": func(r *contract.AgentActivationStepRequestV2) { r.Coordination.Revision++ },
		"coordination_digest": func(r *contract.AgentActivationStepRequestV2) {
			r.Coordination.Digest = core.DigestBytes([]byte("other-coordination"))
		},
		"invocation_sequence": func(r *contract.AgentActivationStepRequestV2) { r.InvocationSequence++ },
		"intent_event_digest": func(r *contract.AgentActivationStepRequestV2) {
			r.InvocationEventDigest = core.DigestBytes([]byte("other-intent"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			forged := fact
			forged.Events = append([]contract.AgentActivationStepEventV2{}, fact.Events...)
			event := forged.Events[invocationIndex]
			request := *event.Request
			mutate(&request)
			request.RequestDigest = ""
			request, err = contract.SealAgentActivationStepRequestV2(request)
			if err != nil {
				t.Fatal("request must remain internally valid to prove aggregate splice rejection:", err)
			}
			event.Request = &request
			event.RequestDigest = request.RequestDigest
			event.Digest = ""
			event, err = contract.SealAgentActivationStepEventV2(event)
			if err != nil {
				t.Fatal("event must remain internally valid to prove aggregate splice rejection:", err)
			}
			forged.Events[invocationIndex] = event
			forged.Digest = ""
			if _, err = contract.SealAgentActivationCoordinationFactV2(forged); err == nil {
				t.Fatal("aggregate accepted invocation/intent splice")
			}
		})
	}
}

func TestAgentActivationV2DispatchWindowAndBudgetUnionFailClosed(t *testing.T) {
	fx := newActivationFixtureV2(t)
	if _, err := fx.coordinator.StartOrInspectAgentActivationV2(context.Background(), fx.request); err != nil {
		t.Fatal(err)
	}
	fact, err := fx.store.InspectAgentActivationCoordinationV2(context.Background(), fx.request.ActivationID)
	if err != nil {
		t.Fatal(err)
	}
	var dispatchRequest contract.AgentActivationStepRequestV2
	var budgetRequest contract.AgentActivationStepRequestV2
	var budgetResult contract.AgentActivationStepResultV2
	for _, event := range fact.Events {
		if event.State != contract.AgentActivationStepResultRecordedV2 || event.Request == nil || event.Result == nil {
			continue
		}
		if event.Step == contract.AgentActivationSandboxAllocateV2 {
			dispatchRequest = *event.Request
		}
		if event.Step == contract.AgentActivationIdentityBudgetV2 {
			budgetRequest, budgetResult = *event.Request, *event.Result
		}
	}
	if dispatchRequest.RequestDigest == "" || budgetResult.ResultDigest == "" {
		t.Fatal("fixture did not persist dispatch and budget results")
	}
	dispatchRequest.Inputs.Dispatch.IntentCurrent.ExpiresUnixNano = dispatchRequest.RequestedNotAfterUnixNano - 1
	dispatchRequest.Inputs.InputDigest = ""
	dispatchRequest.Inputs, err = contract.SealAgentActivationStepInputsV2(dispatchRequest.Inputs, dispatchRequest.Step)
	if err != nil {
		t.Fatal(err)
	}
	dispatchRequest.RequestDigest = ""
	if _, err = contract.SealAgentActivationStepRequestV2(dispatchRequest); err == nil {
		t.Fatal("dispatch request exceeded exact Intent current TTL")
	}

	other := activationCurrentV2("not-required-policy", fx.request.RequestedNotAfterUnixNano)
	forgedProof := budgetResult.Proof
	forgedProof.Budget.NotRequiredPolicy = &other
	forgedProof.ProofDigest = ""
	if _, err = contract.SealAgentActivationStepProofV2(forgedProof, budgetRequest.Step, budgetRequest.Inputs.ProposedScope); err == nil {
		t.Fatal("budget proof accepted two union branches")
	}
}

func TestAgentActivationV2SQLiteFullCoordinationSurvivesRestart(t *testing.T) {
	fx := newActivationFixtureV2(t)
	path := filepath.Join(t.TempDir(), "application.db")
	store, err := applicationsqlite.OpenV1(context.Background(), applicationsqlite.ConfigV1{Path: path, Clock: func() time.Time { return fx.now }})
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := application.NewAgentActivationCoordinatorV2(store, fx.ports, func() time.Time { return fx.now })
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.StartOrInspectAgentActivationV2(context.Background(), fx.request)
	if err != nil {
		t.Fatal(err)
	}
	if err = result.ValidateFor(fx.request, fx.now); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = applicationsqlite.OpenV1(context.Background(), applicationsqlite.ConfigV1{Path: path, Clock: func() time.Time { return fx.now }})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	fact, err := store.InspectAgentActivationCoordinationV2(context.Background(), fx.request.ActivationID)
	if err != nil || fact.Result == nil || fact.Revision != 24 {
		t.Fatalf("restart lost completed activation: %v", err)
	}
}

func TestAgentActivationV2ClockRollbackFailsBeforeOwnerStart(t *testing.T) {
	fx := newActivationFixtureV2(t)
	var reads atomic.Uint64
	coordinator, err := application.NewAgentActivationCoordinatorV2(fx.store, fx.ports, func() time.Time {
		if reads.Add(1) == 1 {
			return fx.now
		}
		return fx.now.Add(-time.Nanosecond)
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = coordinator.StartOrInspectAgentActivationV2(context.Background(), fx.request)
	if !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("expected clock regression, got %v", err)
	}
	_, starts, _, _ := fx.steps[0].CountsV2()
	if starts != 0 {
		t.Fatalf("clock regression dispatched %d starts", starts)
	}
}

func TestAgentActivationV2TTLCrossingStopsBeforeNextStep(t *testing.T) {
	fx := newActivationFixtureV2(t)
	var reads atomic.Uint64
	coordinator, err := application.NewAgentActivationCoordinatorV2(fx.store, fx.ports, func() time.Time {
		if reads.Add(1) <= 2 {
			return fx.now
		}
		return fx.now.Add(2 * time.Hour)
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = coordinator.StartOrInspectAgentActivationV2(context.Background(), fx.request)
	if !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("expected TTL crossing, got %v", err)
	}
	_, preflightStarts, _, _ := fx.steps[0].CountsV2()
	_, snapshotStarts, _, _ := fx.steps[1].CountsV2()
	if preflightStarts != 1 || snapshotStarts != 0 {
		t.Fatalf("TTL crossing order starts=%d/%d", preflightStarts, snapshotStarts)
	}
}

func TestAgentActivationV2FakeReturnsDeepClones(t *testing.T) {
	fx := newActivationFixtureV2(t)
	if _, err := fx.coordinator.StartOrInspectAgentActivationV2(context.Background(), fx.request); err != nil {
		t.Fatal(err)
	}
	first, err := fx.store.InspectAgentActivationCoordinationV2(context.Background(), fx.request.ActivationID)
	if err != nil {
		t.Fatal(err)
	}
	first.Events[1].Request.Inputs.ProposedScope.Instance.Epoch++
	first.Result.ExecutionScope.SandboxLease.Epoch++
	second, err := fx.store.InspectAgentActivationCoordinationV2(context.Background(), fx.request.ActivationID)
	if err != nil {
		t.Fatal(err)
	}
	if second.Validate() != nil || second.Events[1].Request.Inputs.ProposedScope != fx.request.ProposedScope || second.Result.ExecutionScope.SandboxLease.Epoch != 1 {
		t.Fatal("caller mutation contaminated fake store")
	}
}

type activationFixtureV2 struct {
	now         time.Time
	request     contract.AgentActivationStartRequestV2
	store       *fakes.AgentActivationCoordinationStoreV2
	steps       []*fakes.AgentActivationStepV2
	ports       applicationports.AgentActivationStepPortsV2
	coordinator *application.AgentActivationCoordinatorV2
}

func newActivationFixtureV2(t *testing.T) activationFixtureV2 {
	t.Helper()
	now := time.Unix(1_900_000_000, 0)
	expires := now.Add(time.Hour).UnixNano()
	request, err := contract.SealAgentActivationStartRequestV2(contract.AgentActivationStartRequestV2{
		ActivationID: "activation-v2-test", IdempotencyKey: "activation-v2-idempotency",
		ProposedScope:     contract.ProposedActivationScopeV2{Identity: core.AgentIdentityRef{TenantID: "tenant", ID: "agent", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage", PlanDigest: core.DigestBytes([]byte("plan"))}, Instance: core.InstanceRef{ID: "instance", Epoch: 1}, AuthorityEpoch: 1},
		DefinitionCurrent: activationCurrentV2("definition", expires), PlanCurrent: activationCurrentV2("plan", expires), AssemblyCurrent: activationCurrentV2("assembly", expires), BindingSetCurrent: activationCurrentV2("binding", expires), AuthorityCurrent: activationCurrentV2("authority", expires), PolicyCurrent: activationCurrentV2("policy", expires),
		RequirementDigest: core.DigestBytes([]byte("requirements")), ProbeBudget: 8, RequestedNotAfterUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	lease := core.SandboxLeaseRef{ID: "lease", Epoch: 1}
	committed := core.ExecutionScope{Identity: request.ProposedScope.Identity, Lineage: request.ProposedScope.Lineage, Instance: request.ProposedScope.Instance, AuthorityEpoch: request.ProposedScope.AuthorityEpoch, SandboxLease: &lease}
	steps := make([]*fakes.AgentActivationStepV2, len(contract.AgentActivationStepOrderV2()))
	configured := make([]applicationports.AgentActivationStepPortV2, len(steps))
	for index, step := range contract.AgentActivationStepOrderV2() {
		stepCopy := step
		port, createErr := fakes.NewAgentActivationStepV2(step, func(p applicationports.AgentActivationStepPreparationV2) (contract.AgentActivationStepRequestV2, error) {
			inputs := contract.AgentActivationStepInputsV2{ProposedScope: p.Start.ProposedScope, Predecessor: p.Predecessor}
			if stepCopy == contract.AgentActivationIdentityBudgetV2 {
				inputs.Authority = &p.Start.AuthorityCurrent
				inputs.Policy = &p.Start.PolicyCurrent
			}
			if stepCopy == contract.AgentActivationSandboxAllocateV2 || stepCopy == contract.AgentActivationSandboxActivateV2 || stepCopy == contract.AgentActivationExecutionOpenV2 {
				inputs.Dispatch = &contract.AgentActivationDispatchBindingV2{IntentCurrent: activationCurrentV2("intent-"+string(stepCopy), expires), FenceCurrent: activationCurrentV2("fence-"+string(stepCopy), expires)}
			}
			inputs, err = contract.SealAgentActivationStepInputsV2(inputs, stepCopy)
			if err != nil {
				return contract.AgentActivationStepRequestV2{}, err
			}
			return contract.SealAgentActivationStepRequestV2(contract.AgentActivationStepRequestV2{Coordination: p.Coordination, InvocationSequence: p.InvocationSequence, InvocationEventDigest: p.InvocationEventDigest, Step: stepCopy, Inputs: inputs, RequestedNotAfterUnixNano: p.RequestedNotAfterUnixNano})
		}, func(stepRequest contract.AgentActivationStepRequestV2) (contract.AgentActivationStepResultV2, error) {
			proof := contract.AgentActivationStepProofV2{PrimaryCurrent: activationCurrentV2("primary-"+string(stepCopy), expires), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires}
			switch stepCopy {
			case contract.AgentActivationIdentityBudgetV2:
				secondary := activationCurrentV2("identity-lease", expires)
				budget := activationCurrentV2("budget", expires)
				proof.SecondaryCurrent = &secondary
				proof.Budget = &contract.AgentActivationBudgetProofV2{Disposition: contract.AgentActivationBudgetCurrentV2, BudgetCurrent: &budget}
			case contract.AgentActivationSandboxAllocateV2:
				secondary := activationCurrentV2("lease-current", expires)
				proof.SecondaryCurrent, proof.Lease = &secondary, &lease
			case contract.AgentActivationCommitV2:
				secondary := activationCurrentV2("identity-lease-current", expires)
				proof.PrimaryCurrent = activationCurrentV2("primary-activation_commit", expires)
				proof.SecondaryCurrent, proof.Lease, proof.CommittedScope = &secondary, &lease, &committed
			case contract.AgentActivationSandboxActivateV2:
				secondary := activationCurrentV2("lease-current", expires)
				proof.SecondaryCurrent, proof.Lease = &secondary, &lease
			case contract.AgentActivationExecutionOpenV2:
				secondary, endpoint := activationCurrentV2("primary-sandbox_activate", expires), activationCurrentV2("endpoint", expires)
				proof.SecondaryCurrent, proof.Lease, proof.EndpointCurrent = &secondary, &lease, &endpoint
			case contract.AgentActivationReadyInspectV2:
				secondary, ready := activationCurrentV2("primary-sandbox_activate", expires), activationCurrentV2("execution-ready", expires)
				proof.PrimaryCurrent = activationCurrentV2("primary-activation_commit", expires)
				proof.SecondaryCurrent, proof.Lease, proof.EndpointCurrent = &secondary, &lease, &ready
			}
			proof, err = contract.SealAgentActivationStepProofV2(proof, stepCopy, stepRequest.Inputs.ProposedScope)
			if err != nil {
				return contract.AgentActivationStepResultV2{}, err
			}
			result, err := contract.SealAgentActivationStepResultV2(contract.AgentActivationStepResultV2{Proof: proof}, stepRequest)
			if err != nil {
				return contract.AgentActivationStepResultV2{}, err
			}
			return result, result.ValidateFor(stepRequest, now)
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		steps[index], configured[index] = port, port
	}
	ports := applicationports.AgentActivationStepPortsV2{Preflight: configured[0], Snapshot: configured[1], IdentityBudget: configured[2], SandboxAllocate: configured[3], ActivationCommit: configured[4], SandboxActivate: configured[5], ExecutionOpen: configured[6], ReadyInspect: configured[7]}
	store := fakes.NewAgentActivationCoordinationStoreV2()
	coordinator, err := application.NewAgentActivationCoordinatorV2(store, ports, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return activationFixtureV2{now, request, store, steps, ports, coordinator}
}

func activationCurrentV2(id string, expires int64) runtimeports.OwnerCurrentRefV1 {
	return runtimeports.OwnerCurrentRefV1{Owner: core.OwnerRef{Domain: "praxis.test", ID: core.OwnerID("owner-" + id)}, ContractVersion: "praxis.test/current/v1", ID: fmt.Sprintf("%s-current", id), Revision: 1, Digest: core.DigestBytes([]byte(id)), ExpiresUnixNano: expires}
}

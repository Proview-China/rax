package application_test

import (
	"context"
	"sync"
	"testing"
	"time"

	applicationconformance "github.com/Proview-China/rax/ExecutionRuntime/application/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/application/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestAgentLifecycleContractBindsExactStartAndStopClosureV1(t *testing.T) {
	now := time.Unix(1_800_100_000, 0)
	_, start, activation, stop, termination := newAgentLifecycleFixtureV1(t, now)
	if err := activation.ValidateFor(start, now); err != nil {
		t.Fatal(err)
	}
	if err := termination.ValidateFor(stop, now); err != nil {
		t.Fatal(err)
	}
	tamperedStart := start
	tamperedStart.BindingSetCurrent.ID = "binding-set-drift"
	if err := tamperedStart.Validate(); err == nil {
		t.Fatal("Agent activation request accepted an exact input splice")
	}
	tamperedActivation := activation
	tamperedActivation.ExecutionScope.Instance.Epoch++
	if err := tamperedActivation.Validate(); err == nil {
		t.Fatal("Agent activation result accepted ExecutionScope drift")
	}
	tamperedStop := stop
	tamperedStop.SandboxLease.Epoch++
	if err := tamperedStop.Validate(); err == nil {
		t.Fatal("Agent termination request accepted Sandbox lease drift")
	}
	withResidual := termination
	withResidual.Residuals = []contract.AgentTerminationResidualV1{{ResidualID: "residual-one"}}
	withResidual.ResultDigest = ""
	withResidual.Ref.Digest = ""
	if _, err := contract.SealAgentTerminationResultV1(withResidual); err == nil {
		t.Fatal("stopped Agent termination accepted a residual")
	}
	indeterminate := termination
	indeterminate.State = contract.AgentTerminationIndeterminateV1
	indeterminate.ResultDigest = ""
	indeterminate.Ref.Digest = ""
	if _, err := contract.SealAgentTerminationResultV1(indeterminate); err == nil {
		t.Fatal("indeterminate Agent termination accepted no inspectable residual")
	}
}

func TestAgentLifecycleReferencePortConformanceAndLostRepliesV1(t *testing.T) {
	now := time.Unix(1_800_100_100, 0)
	port, start, _, stop, _ := newAgentLifecycleFixtureV1(t, now)
	applicationconformance.RunAgentLifecyclePortConformanceV1(t, port, start, stop, now)
	counts := port.CountsV1()
	if counts.StartCommits != 1 || counts.InspectCalls != 1 || counts.StopCommits != 1 {
		t.Fatalf("strict lifecycle counts drifted: %#v", counts)
	}

	port, start, activation, stop, termination := newAgentLifecycleFixtureV1(t, now)
	port.LoseNextStartReplyV1()
	if _, err := port.StartOrInspectAgentActivationV1(context.Background(), start); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("injected Start lost reply was not surfaced: %v", err)
	}
	recovered, err := port.InspectAgentActivationV1(context.Background(), start)
	if err != nil || recovered.ResultDigest != activation.ResultDigest {
		t.Fatalf("Start lost reply did not recover only by exact Inspect: %#v %v", recovered, err)
	}
	port.LoseNextStopReplyV1()
	if _, err = port.StopOrInspectAgentV1(context.Background(), stop); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("injected Stop lost reply was not surfaced: %v", err)
	}
	stopRecovered, err := port.StopOrInspectAgentV1(context.Background(), stop)
	if err != nil || stopRecovered.ResultDigest != termination.ResultDigest {
		t.Fatalf("Stop lost reply did not inspect the stable attempt: %#v %v", stopRecovered, err)
	}
	counts = port.CountsV1()
	if counts.StartCommits != 1 || counts.InspectCalls != 1 || counts.StopCommits != 1 {
		t.Fatalf("lost reply repeated an Owner attempt: %#v", counts)
	}
}

func TestAgentLifecycleReferencePortLinearizes64StartWorkersV1(t *testing.T) {
	now := time.Unix(1_800_100_200, 0)
	port, start, expected, _, _ := newAgentLifecycleFixtureV1(t, now)
	const workers = 64
	var wait sync.WaitGroup
	errs := make(chan error, workers)
	digests := make(chan core.Digest, workers)
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := port.StartOrInspectAgentActivationV1(context.Background(), start)
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
	for digest := range digests {
		if digest != expected.ResultDigest {
			t.Fatalf("concurrent Agent activation returned %s", digest)
		}
	}
	counts := port.CountsV1()
	if counts.StartCalls != workers || counts.StartCommits != 1 {
		t.Fatalf("64 Agent activation workers did not linearize: %#v", counts)
	}
}

func TestAgentLifecycleReferencePortDeepClonesResultsV1(t *testing.T) {
	now := time.Unix(1_800_100_300, 0)
	port, start, expected, _, _ := newAgentLifecycleFixtureV1(t, now)
	result, err := port.StartOrInspectAgentActivationV1(context.Background(), start)
	if err != nil {
		t.Fatal(err)
	}
	result.ExecutionScope.SandboxLease.Epoch++
	result.ActivationCurrent.ID = "mutated"
	inspected, err := port.InspectAgentActivationV1(context.Background(), start)
	if err != nil || inspected.ResultDigest != expected.ResultDigest || inspected.ExecutionScope.SandboxLease.Epoch != expected.SandboxLease.Epoch {
		t.Fatalf("reference Agent lifecycle exposed mutable result state: %#v %v", inspected, err)
	}
}

func TestAgentLifecycleAdapterImportConformanceV1(t *testing.T) {
	allowed := applicationconformance.AgentLifecycleAdapterAllowedImportsV1()
	allowed[0] = "mutated"
	if applicationconformance.AgentLifecycleAdapterAllowedImportsV1()[0] == "mutated" {
		t.Fatal("Agent lifecycle import allowlist exposed mutable state")
	}
	if err := applicationconformance.CheckAgentLifecycleAdapterImportsV1([]string{
		"context", "sync",
		"github.com/Proview-China/rax/ExecutionRuntime/runtime/core",
		"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports",
		"github.com/Proview-China/rax/ExecutionRuntime/application/contract",
		"github.com/Proview-China/rax/ExecutionRuntime/application/ports",
	}); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"github.com/Proview-China/rax/ExecutionRuntime/agent-host/lifecycle",
		"github.com/Proview-China/rax/ExecutionRuntime/harness/compiler",
		"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel",
		"github.com/Proview-China/rax/ExecutionRuntime/runtime/control",
	} {
		if err := applicationconformance.CheckAgentLifecycleAdapterImportsV1([]string{forbidden}); err == nil {
			t.Fatalf("forbidden Agent lifecycle import passed: %s", forbidden)
		}
	}
}

func newAgentLifecycleFixtureV1(t *testing.T, now time.Time) (*fakes.AgentLifecycleV1, contract.AgentActivationStartRequestV1, contract.AgentActivationResultV1, contract.AgentTerminationRequestV1, contract.AgentTerminationResultV1) {
	t.Helper()
	inputExpiry := now.Add(time.Hour).UnixNano()
	start, err := contract.SealAgentActivationStartRequestV1(contract.AgentActivationStartRequestV1{
		ActivationID: "activation-one", AttemptID: "activation-attempt-one", IdempotencyKey: "activation-idempotency-one",
		DefinitionCurrent:         lifecycleOwnerCurrentV1("definition", "definition-current", inputExpiry),
		PlanCurrent:               lifecycleOwnerCurrentV1("assembler", "plan-current", inputExpiry),
		AssemblyCurrent:           lifecycleOwnerCurrentV1("harness", "assembly-current", inputExpiry),
		BindingSetCurrent:         lifecycleOwnerCurrentV1("runtime", "binding-set-current", inputExpiry),
		AuthorityCurrent:          lifecycleOwnerCurrentV1("runtime", "authority-current", inputExpiry),
		PolicyCurrent:             lifecycleOwnerCurrentV1("policy", "policy-current", inputExpiry),
		BudgetCurrent:             lifecycleOwnerCurrentV1("budget", "budget-current", inputExpiry),
		CredentialCurrent:         lifecycleOwnerCurrentV1("credential", "credential-current", inputExpiry),
		SandboxAdapterBinding:     lifecycleOwnerCurrentV1("sandbox", "sandbox-adapter-binding", inputExpiry),
		ExecutionAdapterBinding:   lifecycleOwnerCurrentV1("harness", "execution-adapter-binding", inputExpiry),
		RequestedNotAfterUnixNano: now.Add(45 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	lease := core.SandboxLeaseRef{ID: "sandbox-lease-one", Epoch: 1}
	scope := core.ExecutionScope{
		Identity: core.AgentIdentityRef{TenantID: "tenant-one", ID: "agent-one", Epoch: 1},
		Lineage:  core.LineageRef{ID: "lineage-one", PlanDigest: start.PlanCurrent.Digest},
		Instance: core.InstanceRef{ID: "instance-one", Epoch: 1}, SandboxLease: &lease, AuthorityEpoch: 1,
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	resultExpiry := now.Add(30 * time.Minute).UnixNano()
	activation, err := contract.SealAgentActivationResultV1(contract.AgentActivationResultV1{
		ActivationID: start.ActivationID, AttemptID: start.AttemptID, RequestDigest: start.RequestDigest,
		ExecutionScope: scope, ExecutionScopeDigest: scopeDigest,
		ActivationCurrent:     lifecycleOwnerCurrentV1("runtime", "activation-current", resultExpiry),
		SandboxLease:          lease,
		SandboxLeaseCurrent:   lifecycleOwnerCurrentV1("sandbox", "sandbox-lease-current", resultExpiry),
		SandboxActiveCurrent:  lifecycleOwnerCurrentV1("sandbox", "sandbox-active-current", resultExpiry),
		ExecutionReadyCurrent: lifecycleOwnerCurrentV1("harness", "execution-ready-current", resultExpiry),
		CheckedUnixNano:       now.UnixNano(), ExpiresUnixNano: resultExpiry,
	})
	if err != nil {
		t.Fatal(err)
	}
	stop, err := contract.SealAgentTerminationRequestV1(contract.AgentTerminationRequestV1{
		StopID: "stop-one", AttemptID: "stop-attempt-one", IdempotencyKey: "stop-idempotency-one",
		ActivationResult: activation.Ref, ActivationCurrent: activation.ActivationCurrent,
		StopPolicyCurrent: lifecycleOwnerCurrentV1("policy", "stop-policy-current", resultExpiry),
		ExecutionScope:    scope, ExecutionScopeDigest: scopeDigest, SandboxLease: lease,
		RequestedNotAfterUnixNano: now.Add(20 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	terminationExpiry := now.Add(15 * time.Minute).UnixNano()
	termination, err := contract.SealAgentTerminationResultV1(contract.AgentTerminationResultV1{
		StopID: stop.StopID, AttemptID: stop.AttemptID, RequestDigest: stop.RequestDigest,
		ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, ActivationCurrent: activation.ActivationCurrent,
		SandboxLease: lease, TerminationCurrent: lifecycleOwnerCurrentV1("runtime", "termination-current", terminationExpiry),
		State: contract.AgentTerminationStoppedV1, Residuals: []contract.AgentTerminationResidualV1{},
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: terminationExpiry,
	})
	if err != nil {
		t.Fatal(err)
	}
	port, err := fakes.NewAgentLifecycleV1(
		func(request contract.AgentActivationStartRequestV1) (contract.AgentActivationResultV1, error) {
			if request.RequestDigest != start.RequestDigest {
				return contract.AgentActivationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "unexpected activation fixture request")
			}
			return activation, nil
		},
		func(request contract.AgentTerminationRequestV1) (contract.AgentTerminationResultV1, error) {
			if request.RequestDigest != stop.RequestDigest {
				return contract.AgentTerminationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "unexpected termination fixture request")
			}
			return termination, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return port, start, activation, stop, termination
}

func lifecycleOwnerCurrentV1(domain, id string, expires int64) runtimeports.OwnerCurrentRefV1 {
	return runtimeports.OwnerCurrentRefV1{
		Owner: core.OwnerRef{Domain: "praxis." + domain, ID: core.OwnerID(domain)}, ContractVersion: "praxis." + domain + "/current-v1",
		ID: id, Revision: 1, Digest: core.DigestBytes([]byte(domain + "\x00" + id)), ExpiresUnixNano: expires,
	}
}

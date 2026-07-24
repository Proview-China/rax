package applicationadapter

import (
	"context"
	"errors"
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type lifecyclePlanReaderStubV4 struct {
	calls int
	err   error
}

func (r *lifecyclePlanReaderStubV4) InspectLifecyclePlanV4(context.Context, applicationcontract.SandboxLifecyclePlanRefV4) (LifecyclePlanEnvelopeV4, error) {
	r.calls++
	return LifecyclePlanEnvelopeV4{}, r.err
}

type lifecycleResultStoreStubV4 struct {
	inspectErr   error
	inspectCalls int
	createCalls  int
}

func (s *lifecycleResultStoreStubV4) CreateLifecycleApplicationResultV4(context.Context, applicationcontract.SandboxLifecycleResultV4) (applicationcontract.SandboxLifecycleResultV4, error) {
	s.createCalls++
	return applicationcontract.SandboxLifecycleResultV4{}, errors.New("unexpected create")
}

func (s *lifecycleResultStoreStubV4) InspectLifecycleApplicationResultV4(context.Context, string) (applicationcontract.SandboxLifecycleResultV4, error) {
	s.inspectCalls++
	return applicationcontract.SandboxLifecycleResultV4{}, s.inspectErr
}

func TestApplicationLifecycleV4DoesNotTreatUnavailableAsAbsent(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	request := applicationRequestFixtureV4(t, now)
	plans := &lifecyclePlanReaderStubV4{err: errors.New("plan should not be read")}
	results := &lifecycleResultStoreStubV4{inspectErr: errors.New("state plane unavailable")}
	adapter, err := NewApplicationLifecycleV4(&LifecycleFlowV4{}, plans, results, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.StartOrInspectSandboxLifecycleV4(context.Background(), request); err == nil {
		t.Fatal("unavailable result Inspect was treated as NotFound")
	}
	if results.inspectCalls != 1 || results.createCalls != 0 || plans.calls != 0 {
		t.Fatalf("unavailable Inspect advanced flow: inspect=%d create=%d plan=%d", results.inspectCalls, results.createCalls, plans.calls)
	}
}

func TestApplicationLifecycleV4OnlyExplicitNotFoundMayReadPlan(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	request := applicationRequestFixtureV4(t, now)
	plans := &lifecyclePlanReaderStubV4{err: errors.New("injected plan stop")}
	results := &lifecycleResultStoreStubV4{inspectErr: ports.ErrNotFound}
	adapter, err := NewApplicationLifecycleV4(&LifecycleFlowV4{}, plans, results, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.StartOrInspectSandboxLifecycleV4(context.Background(), request); err == nil {
		t.Fatal("injected plan stop was ignored")
	}
	if plans.calls != 1 || results.createCalls != 0 {
		t.Fatalf("NotFound path plan=%d create=%d", plans.calls, results.createCalls)
	}
}

func TestApplicationLifecycleV4RejectsTypedNilDependencies(t *testing.T) {
	var plans *lifecyclePlanReaderStubV4
	var results *lifecycleResultStoreStubV4
	if _, err := NewApplicationLifecycleV4(&LifecycleFlowV4{}, plans, &lifecycleResultStoreStubV4{}, time.Now); err == nil {
		t.Fatal("typed-nil plan reader was accepted")
	}
	if _, err := NewApplicationLifecycleV4(&LifecycleFlowV4{}, &lifecyclePlanReaderStubV4{}, results, time.Now); err == nil {
		t.Fatal("typed-nil result store was accepted")
	}
}

func applicationRequestFixtureV4(t *testing.T, now time.Time) applicationcontract.SandboxLifecycleRequestV4 {
	t.Helper()
	digest := func(value string) runtimecore.Digest { return runtimecore.DigestBytes([]byte(value)) }
	lease := runtimecore.SandboxLeaseRef{ID: "lease-1", Epoch: 1}
	scope := runtimecore.ExecutionScope{
		Identity: runtimecore.AgentIdentityRef{TenantID: "tenant-1", ID: "identity-1", Epoch: 1},
		Lineage:  runtimecore.LineageRef{ID: "lineage-1", PlanDigest: digest("plan")},
		Instance: runtimecore.InstanceRef{ID: "instance-1", Epoch: 1}, SandboxLease: &lease, AuthorityEpoch: 1,
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	operation := runtimeports.OperationSubjectV3{
		Kind: runtimeports.OperationScopeActivationV3, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest,
		ActivationAttemptID: "activation-1", SubjectRevision: 1, CurrentProjectionRef: "activation-current-1",
		CurrentProjectionDigest: digest("activation-current"), CurrentProjectionRevision: 1,
	}
	request, err := applicationcontract.SealSandboxLifecycleRequestV4(applicationcontract.SandboxLifecycleRequestV4{
		ID: "request-1", Plan: applicationcontract.SandboxLifecyclePlanRefV4{ID: "plan-1", Revision: 1, Digest: digest("lifecycle-plan"), ExpiresUnixNano: now.Add(time.Minute).UnixNano()},
		Operation: operation, EffectID: "effect-1", AttemptID: "attempt-1", RequestedUnixNano: now.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

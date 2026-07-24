package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type sandboxLifecyclePortV4Stub struct {
	mu            sync.Mutex
	startResult   contract.SandboxLifecycleResultV4
	startErr      error
	inspectResult contract.SandboxLifecycleResultV4
	inspectErr    error
	startCalls    int
	inspectCalls  int
}

func (p *sandboxLifecyclePortV4Stub) StartOrInspectSandboxLifecycleV4(context.Context, contract.SandboxLifecycleRequestV4) (contract.SandboxLifecycleResultV4, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.startCalls++
	return p.startResult, p.startErr
}

func (p *sandboxLifecyclePortV4Stub) InspectSandboxLifecycleV4(context.Context, contract.SandboxLifecycleRequestV4) (contract.SandboxLifecycleResultV4, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inspectCalls++
	return p.inspectResult, p.inspectErr
}

func TestSandboxLifecycleCoordinatorV4RecoversLostReplyOnlyByInspect(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	request, result := sandboxLifecycleFixtureV4(t, now)
	port := &sandboxLifecyclePortV4Stub{startErr: errors.New("lost reply"), inspectResult: result}
	coordinator, err := NewSandboxLifecycleCoordinatorV4(port, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	got, err := coordinator.CoordinateSandboxLifecycleV4(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if got.Digest != result.Digest || port.startCalls != 1 || port.inspectCalls != 1 {
		t.Fatalf("lost reply recovery got=%#v start=%d inspect=%d", got, port.startCalls, port.inspectCalls)
	}
}

func TestSandboxLifecycleCoordinatorV4FailsBeforePortOnInvalidOrTypedNil(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	request, _ := sandboxLifecycleFixtureV4(t, now)
	var typedNil *sandboxLifecyclePortV4Stub
	if _, err := NewSandboxLifecycleCoordinatorV4(typedNil, func() time.Time { return now }); err == nil {
		t.Fatal("typed-nil lifecycle port was accepted")
	}
	port := &sandboxLifecyclePortV4Stub{}
	coordinator, err := NewSandboxLifecycleCoordinatorV4(port, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	request.Digest = core.DigestBytes([]byte("tampered"))
	if _, err := coordinator.CoordinateSandboxLifecycleV4(context.Background(), request); err == nil {
		t.Fatal("tampered request reached the lifecycle port")
	}
	if port.startCalls != 0 || port.inspectCalls != 0 {
		t.Fatalf("invalid request called port start=%d inspect=%d", port.startCalls, port.inspectCalls)
	}
}

func TestSandboxLifecycleCoordinatorV4RejectsAnotherRecoveredResult(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	request, result := sandboxLifecycleFixtureV4(t, now)
	result.ID = "another-result"
	result, err := contract.SealSandboxLifecycleResultV4(result, now)
	if err != nil {
		t.Fatal(err)
	}
	port := &sandboxLifecyclePortV4Stub{startErr: errors.New("lost reply"), inspectResult: result}
	coordinator, err := NewSandboxLifecycleCoordinatorV4(port, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.CoordinateSandboxLifecycleV4(context.Background(), request); err == nil {
		t.Fatal("another request result was accepted")
	}
}

func sandboxLifecycleFixtureV4(t *testing.T, now time.Time) (contract.SandboxLifecycleRequestV4, contract.SandboxLifecycleResultV4) {
	t.Helper()
	digest := func(value string) core.Digest { return core.DigestBytes([]byte(value)) }
	lease := core.SandboxLeaseRef{ID: "lease-1", Epoch: 1}
	scope := core.ExecutionScope{
		Identity: core.AgentIdentityRef{TenantID: "tenant-1", ID: "identity-1", Epoch: 1},
		Lineage:  core.LineageRef{ID: "lineage-1", PlanDigest: digest("plan")},
		Instance: core.InstanceRef{ID: "instance-1", Epoch: 1}, SandboxLease: &lease, AuthorityEpoch: 1,
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
	operationDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	effectID := core.EffectIntentID("effect-1")
	attempt := runtimeports.OperationDispatchAttemptRefV3{
		OperationDigest: operationDigest, EffectID: effectID, IntentRevision: 1, IntentDigest: digest("intent"),
		PermitID: "permit-1", PermitRevision: 1, PermitDigest: digest("permit"), AttemptID: "attempt-1",
	}
	domain := runtimeports.OperationSettlementDomainResultFactRefV4{
		Owner: runtimeports.ProviderBindingRefV2{BindingSetID: "bindings-1", BindingSetRevision: 1, ComponentID: "praxis.sandbox/provider", ManifestDigest: digest("manifest"), ArtifactDigest: digest("artifact"), Capability: "praxis.sandbox/execute"},
		Kind:  "praxis.sandbox/domain-result", ID: "domain-result-1", Revision: 1, Digest: digest("domain-result"), TenantID: "tenant-1",
		EffectID: effectID, EffectRevision: 1, Operation: operation, OperationDigest: operationDigest, Attempt: attempt,
		Schema:        runtimeports.SchemaRefV2{Namespace: "praxis.sandbox", Name: "domain-result", Version: "1.0.0", MediaType: "application/json", ContentDigest: digest("schema")},
		PayloadDigest: digest("payload"), PayloadRevision: 1, AuthoritativeTime: now.UnixNano(),
	}
	if err := domain.Validate(); err != nil {
		t.Fatal(err)
	}
	settlement := runtimeports.OperationSettlementRefV4{ID: "settlement-1", Revision: 1, Digest: digest("settlement"), OperationDigest: operationDigest, EffectID: effectID, DomainResult: domain}
	association := runtimeports.OperationSettlementEvidenceAssociationRefV4{ID: "association-1", Revision: 1, Digest: digest("association"), Settlement: settlement, OperationDigest: operationDigest, EffectID: effectID}
	guard := runtimeports.OperationSettlementTerminalGuardRefV4{ID: "guard-1", TenantID: "tenant-1", EffectID: effectID, OperationDigest: operationDigest, Revision: 1, Digest: digest("guard"), Settlement: settlement}
	projection := runtimeports.OperationSettlementTerminalProjectionRefV4{ID: "projection-1", Revision: 1, Digest: digest("projection"), TenantID: "tenant-1", OperationDigest: operationDigest, EffectID: effectID, Settlement: settlement, Association: association, Guard: guard}
	inspection, err := runtimeports.SealOperationInspectionSettlementRefV4(runtimeports.OperationInspectionSettlementRefV4{
		Settlement: settlement, Association: association, Guard: guard, Projection: projection, DomainResult: domain,
		EffectFactRevision: 2, Owner: runtimeports.EffectOwnerRefV2{Role: runtimeports.OwnerSettlement, ComponentID: "praxis.sandbox/controller", ManifestDigest: digest("controller")},
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	plan := contract.SandboxLifecyclePlanRefV4{ID: "plan-1", Revision: 1, Digest: digest("lifecycle-plan"), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	request, err := contract.SealSandboxLifecycleRequestV4(contract.SandboxLifecycleRequestV4{ID: "request-1", Plan: plan, Operation: operation, EffectID: effectID, AttemptID: attempt.AttemptID, RequestedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	result, err := contract.SealSandboxLifecycleResultV4(contract.SandboxLifecycleResultV4{ID: request.ID, RequestDigest: request.Digest, Plan: plan, DomainResult: domain, Settlement: inspection, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}, now)
	if err != nil {
		t.Fatal(err)
	}
	return request, result
}

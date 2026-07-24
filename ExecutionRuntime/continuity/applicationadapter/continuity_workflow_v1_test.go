package applicationadapter_test

import (
	"context"
	"testing"
	"time"

	appcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/applicationadapter"
	continuitycontract "github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestGovernedWorkflowAdapterV1SubmitInspectExactAndClone(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	request := governedWorkflowRequestV1(now)
	gateway := &governedWorkflowGatewayFakeV1{}
	adapter, err := applicationadapter.NewGovernedWorkflowAdapterV1(gateway)
	if err != nil {
		t.Fatal(err)
	}
	first, err := adapter.Submit(context.Background(), request)
	if err != nil || gateway.submitCalls != 1 || first.RequestDigest == "" {
		t.Fatalf("Submit = %#v calls=%d err=%v", first, gateway.submitCalls, err)
	}
	first.Steps[0].StepID = "mutated"
	second, err := adapter.Inspect(context.Background(), request)
	if err != nil || gateway.inspectCalls != 1 || second.Steps[0].StepID != "continuity-root" {
		t.Fatalf("Inspect aliases result or missed gateway: %#v calls=%d err=%v", second, gateway.inspectCalls, err)
	}
}

func TestGovernedWorkflowAdapterV1RejectsWrongOwnerAndTypedNil(t *testing.T) {
	var typedNil *governedWorkflowGatewayFakeV1
	if _, err := applicationadapter.NewGovernedWorkflowAdapterV1(typedNil); err == nil {
		t.Fatal("typed-nil gateway was accepted")
	}
	now := time.Unix(1_900_000_000, 0)
	request := governedWorkflowRequestV1(now)
	request.DomainRequest.Owner.ComponentID = "praxis.tool/owner"
	gateway := &governedWorkflowGatewayFakeV1{}
	adapter, err := applicationadapter.NewGovernedWorkflowAdapterV1(gateway)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Submit(context.Background(), request); !core.HasCategory(err, core.ErrorForbidden) || gateway.submitCalls != 0 {
		t.Fatalf("wrong owner reached public gateway: calls=%d err=%v", gateway.submitCalls, err)
	}
}

type governedWorkflowGatewayFakeV1 struct {
	result       appcontract.ContinuityWorkflowInspectionV1
	submitCalls  int
	inspectCalls int
}

func (g *governedWorkflowGatewayFakeV1) SubmitContinuityWorkflowV1(_ context.Context, request appcontract.ContinuityWorkflowRequestV1) (appcontract.ContinuityWorkflowInspectionV1, error) {
	g.submitCalls++
	if g.result.RequestDigest == "" {
		g.result = governedWorkflowInspectionV1(request)
	}
	return g.result, nil
}

func (g *governedWorkflowGatewayFakeV1) InspectContinuityWorkflowV1(_ context.Context, request appcontract.ContinuityWorkflowRequestV1) (appcontract.ContinuityWorkflowInspectionV1, error) {
	g.inspectCalls++
	if g.result.RequestDigest == "" {
		g.result = governedWorkflowInspectionV1(request)
	}
	return g.result, nil
}

func governedWorkflowInspectionV1(request appcontract.ContinuityWorkflowRequestV1) appcontract.ContinuityWorkflowInspectionV1 {
	digest, _ := request.DigestV1()
	ref := func(id string) appcontract.ApplicationFactRefV2 {
		return appcontract.ApplicationFactRefV2{Ref: id, Revision: 1, Digest: core.DigestBytes([]byte(id))}
	}
	return appcontract.ContinuityWorkflowInspectionV1{
		RequestDigest: digest,
		Submission:    ref(request.RequestID), Command: ref(request.RequestID), Outbox: ref(request.RequestID), Plan: ref("plan-1"),
		Status: appcontract.WorkflowAcceptedV2,
		Steps:  []appcontract.ContinuityWorkflowStepRefV1{{StepID: "continuity-root", Kind: runtimeports.NamespacedNameV2(request.Kind), Descriptor: appcontract.StepDescriptorRefV2{Kind: runtimeports.NamespacedNameV2(request.Kind), Revision: 1, Digest: core.DigestBytes([]byte("descriptor")), ExpiresUnixNano: request.NotAfterUnixNano}}},
	}
}

func governedWorkflowRequestV1(now time.Time) appcontract.ContinuityWorkflowRequestV1 {
	digest := func(value string) core.Digest { return core.DigestBytes([]byte(value)) }
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-1", ID: "agent-1", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-1", PlanDigest: digest("plan")}, Instance: core.InstanceRef{ID: "instance-1", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "lease-1", Epoch: 1}, AuthorityEpoch: 1}
	scopeDigest, _ := runtimeports.ExecutionScopeDigestV2(scope)
	ref := func(id string) appcontract.ApplicationFactRefV2 {
		return appcontract.ApplicationFactRefV2{Ref: id, Revision: 1, Digest: digest(id)}
	}
	return appcontract.ContinuityWorkflowRequestV1{
		ContractVersion: appcontract.ContinuityWorkflowContractVersionV1,
		RequestID:       "request-1", IdempotencyKey: "idempotency-1", Kind: appcontract.ContinuityTimelineProjectV1, Target: scope,
		DomainRequest: appcontract.ExternalFactRefV1{ContractVersion: "praxis.continuity/request/v1", SchemaRef: "praxis.continuity/request/v1", Owner: appcontract.ExternalOwnerBindingV1{BindingSetID: "binding-set-1", BindingSetRevision: 1, ComponentID: runtimeports.ComponentIDV2(continuitycontract.ContinuityComponentID), ManifestDigest: digest("manifest"), ArtifactDigest: digest("artifact"), Capability: "praxis.continuity/governed-workflow", FactKind: "praxis.continuity/request"}, TenantID: scope.Identity.TenantID, ScopeDigest: scopeDigest, ID: "domain-request-1", Revision: 1, Digest: digest("domain-request")},
		CompiledGraph: ref("compiled-graph"), Binding: ref("binding"), Consumer: ref("consumer"), RequestedUnixNano: now.Add(-time.Second).UnixNano(), NotAfterUnixNano: now.Add(time.Hour).UnixNano(),
	}
}

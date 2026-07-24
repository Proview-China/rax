package sdk_test

import (
	"context"
	"testing"
	"time"

	appcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/sdk"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestClientGovernedWorkflowUsesGatewayAndClones(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	request := sdkWorkflowRequestV1(now)
	gateway := &sdkWorkflowGatewayV1{}
	client, err := sdk.New(sdk.Config{Workflows: gateway, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := client.SubmitGovernedWorkflow(context.Background(), request)
	if err != nil || gateway.submitCalls != 1 {
		t.Fatalf("SubmitGovernedWorkflow = %#v calls=%d err=%v", accepted, gateway.submitCalls, err)
	}
	accepted.Steps[0].StepID = "mutated"
	inspected, err := client.InspectGovernedWorkflow(context.Background(), request)
	if err != nil || gateway.inspectCalls != 1 || inspected.Steps[0].StepID != "continuity-root" {
		t.Fatalf("InspectGovernedWorkflow aliases result: %#v calls=%d err=%v", inspected, gateway.inspectCalls, err)
	}
}

type sdkWorkflowGatewayV1 struct {
	result       appcontract.ContinuityWorkflowInspectionV1
	submitCalls  int
	inspectCalls int
}

func (g *sdkWorkflowGatewayV1) Submit(_ context.Context, request appcontract.ContinuityWorkflowRequestV1) (appcontract.ContinuityWorkflowInspectionV1, error) {
	g.submitCalls++
	if g.result.RequestDigest == "" {
		g.result = sdkWorkflowInspectionV1(request)
	}
	return g.result, nil
}

func (g *sdkWorkflowGatewayV1) Inspect(_ context.Context, request appcontract.ContinuityWorkflowRequestV1) (appcontract.ContinuityWorkflowInspectionV1, error) {
	g.inspectCalls++
	if g.result.RequestDigest == "" {
		g.result = sdkWorkflowInspectionV1(request)
	}
	return g.result, nil
}

func sdkWorkflowInspectionV1(request appcontract.ContinuityWorkflowRequestV1) appcontract.ContinuityWorkflowInspectionV1 {
	digest, _ := request.DigestV1()
	ref := func(id string) appcontract.ApplicationFactRefV2 {
		return appcontract.ApplicationFactRefV2{Ref: id, Revision: 1, Digest: core.DigestBytes([]byte(id))}
	}
	return appcontract.ContinuityWorkflowInspectionV1{RequestDigest: digest, Submission: ref(request.RequestID), Command: ref(request.RequestID), Outbox: ref(request.RequestID), Plan: ref("plan-1"), Status: appcontract.WorkflowAcceptedV2, Steps: []appcontract.ContinuityWorkflowStepRefV1{{StepID: "continuity-root", Kind: runtimeports.NamespacedNameV2(request.Kind), Descriptor: appcontract.StepDescriptorRefV2{Kind: runtimeports.NamespacedNameV2(request.Kind), Revision: 1, Digest: core.DigestBytes([]byte("descriptor")), ExpiresUnixNano: request.NotAfterUnixNano}}}}
}

func sdkWorkflowRequestV1(now time.Time) appcontract.ContinuityWorkflowRequestV1 {
	digest := func(value string) core.Digest { return core.DigestBytes([]byte(value)) }
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-1", ID: "agent-1", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-1", PlanDigest: digest("plan")}, Instance: core.InstanceRef{ID: "instance-1", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "lease-1", Epoch: 1}, AuthorityEpoch: 1}
	scopeDigest, _ := runtimeports.ExecutionScopeDigestV2(scope)
	ref := func(id string) appcontract.ApplicationFactRefV2 {
		return appcontract.ApplicationFactRefV2{Ref: id, Revision: 1, Digest: digest(id)}
	}
	return appcontract.ContinuityWorkflowRequestV1{ContractVersion: appcontract.ContinuityWorkflowContractVersionV1, RequestID: "request-1", IdempotencyKey: "idempotency-1", Kind: appcontract.ContinuityTimelineProjectV1, Target: scope, DomainRequest: appcontract.ExternalFactRefV1{ContractVersion: "praxis.continuity/request/v1", SchemaRef: "praxis.continuity/request/v1", Owner: appcontract.ExternalOwnerBindingV1{BindingSetID: "binding-set-1", BindingSetRevision: 1, ComponentID: "praxis/continuity", ManifestDigest: digest("manifest"), ArtifactDigest: digest("artifact"), Capability: "praxis.continuity/governed-workflow", FactKind: "praxis.continuity/request"}, TenantID: scope.Identity.TenantID, ScopeDigest: scopeDigest, ID: "domain-request-1", Revision: 1, Digest: digest("domain-request")}, CompiledGraph: ref("compiled-graph"), Binding: ref("binding"), Consumer: ref("consumer"), RequestedUnixNano: now.Add(-time.Second).UnixNano(), NotAfterUnixNano: now.Add(time.Hour).UnixNano()}
}

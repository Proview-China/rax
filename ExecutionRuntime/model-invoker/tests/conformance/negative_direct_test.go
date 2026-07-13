package conformance_test

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/direct"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/routegateway"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

func TestN13HarnessOwnedToolCannotMapToDirectRoute(t *testing.T) {
	invocation := negativeInvocation("exec-N13")
	invocation.Plan.Route = union.VersionedIdentity{ID: "route-direct-negative", Version: "v1"}
	invocation.Request.Tools = []union.ToolDefinition{{
		ID: "harness-tool", Name: "harness_tool", Kind: "function", ExecutionOwner: union.ExecutionOwnerHarness,
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	}}
	invocation.Plan.Digest = ""
	delete(invocation.Plan.Metadata, "request_digest")
	invocation, err := execution.NewInvocation(invocation.Request, invocation.Plan)
	if err != nil {
		t.Fatal(err)
	}
	backend := &negativeDirectBackend{routeID: "route-direct-negative", model: "gpt-negative"}
	adapter, err := direct.New(direct.Config{
		Identity: union.VersionedIdentity{ID: "direct-negative", Version: "v1"}, Backend: backend,
		RouteID: backend.routeID, Model: backend.model,
		Invocation: upstream.InvocationContext{
			Explicit: true, Usage: upstream.InvocationGeneralAPI, Subject: upstream.SubjectService,
			Tenancy: upstream.TenancyMulti, Execution: upstream.ExecutionForeground,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	report, err := adapter.Preflight(context.Background(), invocation)
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	if report.Accepted || report.RejectionCode != "direct_mapping_rejected" {
		t.Fatalf("direct preflight = %#v", report)
	}
	if backend.resolveCalls.Load() != 0 {
		t.Fatalf("harness-owned tool reached Direct backend %d time(s)", backend.resolveCalls.Load())
	}
}

type negativeDirectBackend struct {
	routeID      upstream.RouteID
	model        string
	resolveCalls atomic.Int64
}

func (backend *negativeDirectBackend) Resolve(context.Context, modelinvoker.RouteCall) (routegateway.Resolution, error) {
	backend.resolveCalls.Add(1)
	return routegateway.Resolution{Route: modelinvoker.RouteSelection{RouteID: backend.routeID, Model: backend.model}}, nil
}

func (*negativeDirectBackend) Invoke(context.Context, modelinvoker.RouteCall) (routegateway.InvokeResult, error) {
	return routegateway.InvokeResult{}, nil
}

func (*negativeDirectBackend) OpenStream(context.Context, modelinvoker.RouteCall) (direct.ModelStream, error) {
	return nil, nil
}

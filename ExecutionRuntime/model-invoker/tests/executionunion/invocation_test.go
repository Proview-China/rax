package executionunion_test

import (
	"errors"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func TestInvocationSealRejectsRequestAndPlanMutation(t *testing.T) {
	invocation := validInvocation("exec-seal-mutation")
	if err := invocation.Validate(); err != nil {
		t.Fatalf("sealed invocation: %v", err)
	}

	requestMutation, err := invocation.Clone()
	if err != nil {
		t.Fatal(err)
	}
	requestMutation.Request.Metadata = map[string]string{"trace": "changed-after-profile-compile"}
	if err := requestMutation.Validate(); !errors.Is(err, execution.ErrInvalidInvocation) {
		t.Fatalf("request mutation error = %v", err)
	}

	planMutation, err := invocation.Clone()
	if err != nil {
		t.Fatal(err)
	}
	planMutation.Plan.RouteFingerprint = "sha256:mutated"
	if err := planMutation.Validate(); !errors.Is(err, execution.ErrInvalidInvocation) {
		t.Fatalf("plan mutation error = %v", err)
	}
}

func TestNewInvocationRejectsCrossGraphAndExpectedIdentityMismatch(t *testing.T) {
	base := validInvocation("exec-seal-cross-graph")
	plan := base.Plan
	plan.Digest = ""
	delete(plan.Metadata, "request_digest")
	plan.IntentGraph.Nodes[0].Target = "/workspace/other.txt"
	if _, err := execution.NewInvocation(base.Request, plan); !errors.Is(err, execution.ErrInvalidInvocation) {
		t.Fatalf("cross-graph error = %v", err)
	}

	request := base.Request
	request.SessionIntent.ExpectedRoute = union.VersionedIdentity{ID: "different-route", Version: "v1"}
	plan = base.Plan
	plan.Digest = ""
	delete(plan.Metadata, "request_digest")
	if _, err := execution.NewInvocation(request, plan); !errors.Is(err, execution.ErrInvalidInvocation) {
		t.Fatalf("expected Route mismatch error = %v", err)
	}
}

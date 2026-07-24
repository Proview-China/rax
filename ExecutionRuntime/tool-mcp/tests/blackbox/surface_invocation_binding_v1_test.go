package blackbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/owner/surfacebinding"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestToolSurfaceInvocationBindingV1PublicCreateAndLostReplyInspect(t *testing.T) {
	clock := testkit.NewManualClock(testkit.FixedTime.Add(time.Second))
	repository, err := surfacebinding.NewInMemoryRepositoryV1(testkit.Owner(), clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	request := testkit.ToolSurfaceInvocationBindingRequestV1()
	binding, ack, err := repository.EnsureToolSurfaceInvocationBindingV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	read, readAck, err := repository.InspectToolSurfaceInvocationBindingByInvocationV1(context.Background(), request.Invocation)
	if err != nil || read.Ref != binding.Ref || readAck.Ref != ack.Ref {
		t.Fatalf("public lost-reply recovery failed: binding=%+v ack=%+v err=%v", read.Ref, readAck.Ref, err)
	}
	clock.Set(testkit.FixedTime.Add(11 * time.Minute))
	if err := binding.ValidateCurrent(clock.Now()); err == nil {
		t.Fatal("expired Binding remained current")
	}
	if _, _, err := repository.InspectExactToolSurfaceInvocationBindingV1(context.Background(), binding.Ref); err != nil {
		t.Fatalf("immutable historical Binding became uninspectable after expiry: %v", err)
	}
}

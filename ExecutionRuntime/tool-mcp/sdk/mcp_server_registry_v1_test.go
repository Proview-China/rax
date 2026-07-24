package sdk

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
)

func mcpServerRefV1(value toolcontract.MCPServerDescriptor) toolcontract.ObjectRef {
	return toolcontract.ObjectRef{ID: value.ID, Revision: value.Revision, Digest: value.Digest}
}

func successorMCPServerV1(t *testing.T, current toolcontract.MCPServerDescriptor) toolcontract.MCPServerDescriptor {
	t.Helper()
	current.Revision++
	current.ConfigDigest = testkit.Digest("mcp-server-config-v2")
	current.Digest = ""
	value, err := toolcontract.SealMCPServer(current)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestMCPServerRegistrySDKV1CreateSuccessorInspectAndDeepClone(t *testing.T) {
	repository := mcp.NewInMemoryMCPServerDescriptorRepositoryV1()
	client, err := NewMCPServerRegistryV1(repository, func() time.Time { return testkit.FixedTime })
	if err != nil {
		t.Fatal(err)
	}
	first := testkit.MCPServer()
	registered, err := client.RegisterMCPServerV1(context.Background(), first, nil)
	if err != nil {
		t.Fatal(err)
	}
	registered.Transports[0] = "praxis.test/tampered"
	inspected, err := client.InspectMCPServerV1(context.Background(), mcpServerRefV1(first))
	if err != nil || inspected.Transports[0] != first.Transports[0] {
		t.Fatalf("deep clone/exact Inspect=%+v err=%v", inspected, err)
	}
	second := successorMCPServerV1(t, first)
	expected := mcpServerRefV1(first)
	registered, err = client.RegisterMCPServerV1(context.Background(), second, &expected)
	if err != nil {
		t.Fatal(err)
	}
	current, err := client.InspectCurrentMCPServerV1(context.Background(), first.ID)
	if err != nil || current.Digest != second.Digest || registered.Digest != second.Digest {
		t.Fatalf("current=%+v registered=%+v err=%v", current, registered, err)
	}
	if _, err = client.RegisterMCPServerV1(context.Background(), first, nil); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("revision rollback error=%v", err)
	}
}

func TestMCPServerRegistrySDKV1ConcurrentSameSuccessorSingleCurrent(t *testing.T) {
	repository := mcp.NewInMemoryMCPServerDescriptorRepositoryV1()
	client, _ := NewMCPServerRegistryV1(repository, func() time.Time { return testkit.FixedTime })
	first := testkit.MCPServer()
	if _, err := client.RegisterMCPServerV1(context.Background(), first, nil); err != nil {
		t.Fatal(err)
	}
	second := successorMCPServerV1(t, first)
	expected := mcpServerRefV1(first)
	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, err := client.RegisterMCPServerV1(context.Background(), second, &expected)
			if err == nil && value.Digest != second.Digest {
				err = core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "concurrent MCP Server winner drifted")
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	current, err := client.InspectCurrentMCPServerV1(context.Background(), first.ID)
	if err != nil || current.Digest != second.Digest {
		t.Fatalf("current=%+v err=%v", current, err)
	}
}

type lostReplyMCPServerRepositoryV1 struct {
	inner *mcp.InMemoryMCPServerDescriptorRepositoryV1
}

func (r *lostReplyMCPServerRepositoryV1) EnsureMCPServerDescriptorV1(ctx context.Context, request mcp.EnsureMCPServerDescriptorRequestV1) (toolcontract.MCPServerDescriptor, error) {
	if _, err := r.inner.EnsureMCPServerDescriptorV1(ctx, request); err != nil {
		return toolcontract.MCPServerDescriptor{}, err
	}
	return toolcontract.MCPServerDescriptor{}, core.NewError(core.ErrorUnavailable, core.ReasonEffectUnknownOutcome, "lost MCP Server registration reply")
}
func (r *lostReplyMCPServerRepositoryV1) InspectMCPServerDescriptorV1(ctx context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPServerDescriptor, error) {
	return r.inner.InspectMCPServerDescriptorV1(ctx, exact)
}
func (r *lostReplyMCPServerRepositoryV1) InspectCurrentMCPServerDescriptorV1(ctx context.Context, id string) (toolcontract.MCPServerDescriptor, error) {
	return r.inner.InspectCurrentMCPServerDescriptorV1(ctx, id)
}

func TestMCPServerRegistrySDKV1LostReplyAndFailClosedBoundaries(t *testing.T) {
	inner := mcp.NewInMemoryMCPServerDescriptorRepositoryV1()
	lost := &lostReplyMCPServerRepositoryV1{inner: inner}
	client, err := NewMCPServerRegistryV1(lost, func() time.Time { return testkit.FixedTime })
	if err != nil {
		t.Fatal(err)
	}
	descriptor := testkit.MCPServer()
	if _, err = client.RegisterMCPServerV1(context.Background(), descriptor, nil); !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("lost reply error=%v", err)
	}
	inspected, err := client.InspectMCPServerV1(context.Background(), mcpServerRefV1(descriptor))
	if err != nil || inspected.Digest != descriptor.Digest {
		t.Fatalf("lost reply Inspect=%+v err=%v", inspected, err)
	}

	var typedNil *mcp.InMemoryMCPServerDescriptorRepositoryV1
	if _, err = NewMCPServerRegistryV1(typedNil, time.Now); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed nil error=%v", err)
	}
	valid, _ := NewMCPServerRegistryV1(inner, func() time.Time { return testkit.FixedTime })
	if _, err = valid.RegisterMCPServerV1(nil, descriptor, nil); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil context error=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = valid.RegisterMCPServerV1(ctx, descriptor, nil); err != context.Canceled {
		t.Fatalf("canceled context error=%v", err)
	}
	future := descriptor
	future.CreatedUnixNano = testkit.FixedTime.Add(time.Second).UnixNano()
	future.Digest = ""
	future, err = toolcontract.SealMCPServer(future)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = valid.RegisterMCPServerV1(context.Background(), future, nil); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("future Descriptor error=%v", err)
	}
}

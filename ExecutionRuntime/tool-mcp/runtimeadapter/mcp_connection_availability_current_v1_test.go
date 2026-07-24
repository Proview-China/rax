package runtimeadapter_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/runtimeadapter"
)

func TestMCPConnectionAvailabilityCurrentAdapterV1ExactProjection(t *testing.T) {
	now := time.Now().UTC()
	connection, availability := mcpConnectionAvailabilityAdapterFixtureV1(t, now)
	source := &mcpConnectionAvailabilitySourceV1{connection: connection, availability: availability}
	adapter, err := runtimeadapter.NewMCPConnectionAvailabilityCurrentAdapterV1(source, func() time.Time { return now }, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	exact := runtimeadapter.MCPConnectionAvailabilityRuntimeRefV1(availability)
	projection, err := adapter.InspectCurrentMCPConnectionAvailabilityNeutralV1(context.Background(), exact)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Ref != exact || projection.TenantID != core.TenantID(connection.Coordinate.TenantID) || projection.RunID != connection.Coordinate.RunID || projection.SessionID != connection.Coordinate.Session.ID || projection.ConnectionEpoch != connection.Coordinate.Epoch || projection.Provider != connection.Provider || projection.ProviderTransport != connection.ProviderTransport {
		t.Fatalf("Runtime projection lost Tool exact closure: %#v", projection)
	}
	if source.availabilityCalls.Load() != 1 || source.connectionCalls.Load() != 1 {
		t.Fatalf("adapter did not read both Tool owners exactly once: availability=%d connection=%d", source.availabilityCalls.Load(), source.connectionCalls.Load())
	}
}

func TestMCPConnectionAvailabilityCurrentAdapterV1FailsClosed(t *testing.T) {
	now := time.Now().UTC()
	connection, availability := mcpConnectionAvailabilityAdapterFixtureV1(t, now)
	exact := runtimeadapter.MCPConnectionAvailabilityRuntimeRefV1(availability)

	t.Run("typed_nil", func(t *testing.T) {
		var source *mcpConnectionAvailabilitySourceV1
		if _, err := runtimeadapter.NewMCPConnectionAvailabilityCurrentAdapterV1(source, func() time.Time { return now }, time.Second); !core.HasReason(err, core.ReasonComponentMissing) {
			t.Fatalf("typed-nil source error=%v", err)
		}
	})
	t.Run("nil_context", func(t *testing.T) {
		adapter, _ := runtimeadapter.NewMCPConnectionAvailabilityCurrentAdapterV1(&mcpConnectionAvailabilitySourceV1{connection: connection, availability: availability}, func() time.Time { return now }, time.Second)
		if _, err := adapter.InspectCurrentMCPConnectionAvailabilityNeutralV1(nil, exact); !core.HasCategory(err, core.ErrorInvalidArgument) {
			t.Fatalf("nil context error=%v", err)
		}
	})
	t.Run("canceled_context", func(t *testing.T) {
		adapter, _ := runtimeadapter.NewMCPConnectionAvailabilityCurrentAdapterV1(&mcpConnectionAvailabilitySourceV1{connection: connection, availability: availability}, func() time.Time { return now }, time.Second)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := adapter.InspectCurrentMCPConnectionAvailabilityNeutralV1(ctx, exact); err != context.Canceled {
			t.Fatalf("canceled context error=%v", err)
		}
	})
	t.Run("clock_rollback", func(t *testing.T) {
		values := []time.Time{now, now.Add(-time.Nanosecond)}
		var index atomic.Int64
		adapter, _ := runtimeadapter.NewMCPConnectionAvailabilityCurrentAdapterV1(&mcpConnectionAvailabilitySourceV1{connection: connection, availability: availability}, func() time.Time { return values[min(int(index.Add(1)-1), len(values)-1)] }, time.Second)
		if _, err := adapter.InspectCurrentMCPConnectionAvailabilityNeutralV1(context.Background(), exact); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("clock rollback error=%v", err)
		}
	})
	t.Run("source_digest_drift", func(t *testing.T) {
		drifted := availability
		drifted.ApplySettlement.Digest = testkit.Digest("another-apply")
		drifted.Digest = ""
		drifted, _ = toolcontract.SealMCPConnectionAvailabilityCurrentProjectionV1(drifted, now)
		adapter, _ := runtimeadapter.NewMCPConnectionAvailabilityCurrentAdapterV1(&mcpConnectionAvailabilitySourceV1{connection: connection, availability: drifted}, func() time.Time { return now }, time.Second)
		if _, err := adapter.InspectCurrentMCPConnectionAvailabilityNeutralV1(context.Background(), exact); !core.HasReason(err, core.ReasonBindingDrift) {
			t.Fatalf("source drift error=%v", err)
		}
	})
}

func TestMCPConnectionAvailabilityCurrentAdapterV1ConcurrentReads(t *testing.T) {
	now := time.Now().UTC()
	connection, availability := mcpConnectionAvailabilityAdapterFixtureV1(t, now)
	source := &mcpConnectionAvailabilitySourceV1{connection: connection, availability: availability}
	adapter, _ := runtimeadapter.NewMCPConnectionAvailabilityCurrentAdapterV1(source, func() time.Time { return now }, time.Second)
	exact := runtimeadapter.MCPConnectionAvailabilityRuntimeRefV1(availability)
	const workers = 64
	values := make(chan runtimeports.MCPConnectionAvailabilityNeutralProjectionV1, workers)
	errs := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			value, err := adapter.InspectCurrentMCPConnectionAvailabilityNeutralV1(context.Background(), exact)
			values <- value
			errs <- err
		}()
	}
	group.Wait()
	close(values)
	close(errs)
	var winner runtimeports.MCPConnectionAvailabilityNeutralProjectionV1
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for value := range values {
		if winner.ProjectionDigest == "" {
			winner = value
		} else if value != winner {
			t.Fatal("concurrent exact reads produced different projections")
		}
	}
}

func mcpConnectionAvailabilityAdapterFixtureV1(t *testing.T, now time.Time) (toolcontract.MCPConnectionFactV2, toolcontract.MCPConnectionAvailabilityCurrentProjectionV1) {
	t.Helper()
	fixture := testkit.MCPConnectControlledV1(now, toolcontract.MCPTransportStreamableHTTPV1)
	receipt := testkit.MCPConnectReceiptV1(fixture, []byte(`{"protocolVersion":"2025-03-26"}`), now)
	connection, err := toolcontract.SealMCPConnectionFactV2(toolcontract.MCPConnectionFactV2{
		Owner: fixture.Connect.Intent.Owner, Coordinate: fixture.Connect.Intent.Coordinate, Intent: fixture.Connect.Intent.Ref,
		TransportConfig: fixture.Connect.Config.Ref, Server: fixture.Connect.Intent.Server, ProtocolReceipt: receipt.Ref,
		ProviderTransport: fixture.Connect.Intent.ProviderTransport, Provider: fixture.Connect.Intent.Provider,
		NegotiatedProtocol: receipt.NegotiatedProtocol, ProviderSessionID: receipt.ProviderSessionID,
		CreatedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	availability, err := toolcontract.SealMCPConnectionAvailabilityCurrentProjectionV1(toolcontract.MCPConnectionAvailabilityCurrentProjectionV1{
		Connection:      connection.Ref,
		ApplySettlement: toolcontract.ObjectRef{ID: "mcp-connect-apply-adapter", Revision: 1, Digest: testkit.Digest("mcp-connect-apply-adapter")},
		DomainResult:    toolcontract.ObjectRef{ID: "mcp-connect-result-adapter", Revision: 1, Digest: testkit.Digest("mcp-connect-result-adapter")},
		Owner:           connection.Owner, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano(),
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	return connection, availability
}

type mcpConnectionAvailabilitySourceV1 struct {
	connection        toolcontract.MCPConnectionFactV2
	availability      toolcontract.MCPConnectionAvailabilityCurrentProjectionV1
	connectionCalls   atomic.Int64
	availabilityCalls atomic.Int64
}

func (s *mcpConnectionAvailabilitySourceV1) InspectCurrentMCPConnectionFactV2(_ context.Context, exact toolcontract.MCPConnectionFactRefV2) (toolcontract.MCPConnectionFactV2, error) {
	s.connectionCalls.Add(1)
	if exact != s.connection.Ref {
		return toolcontract.MCPConnectionFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "connection not found")
	}
	return s.connection, nil
}

func (s *mcpConnectionAvailabilitySourceV1) InspectCurrentMCPConnectionAvailabilityV1(_ context.Context, exact toolcontract.MCPConnectionFactRefV2, _ time.Duration) (toolcontract.MCPConnectionAvailabilityCurrentProjectionV1, error) {
	s.availabilityCalls.Add(1)
	if exact != s.availability.Connection {
		return toolcontract.MCPConnectionAvailabilityCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "availability not found")
	}
	return s.availability, nil
}

var _ runtimeports.MCPConnectionAvailabilityNeutralCurrentReaderV1 = (*runtimeadapter.MCPConnectionAvailabilityCurrentAdapterV1)(nil)

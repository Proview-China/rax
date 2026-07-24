package mcp_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
)

func TestMCPDiscoveryPageCommandRepositoryV1CreateOnceAndInspect(t *testing.T) {
	command := mcpDiscoveryPageCommandFixtureV1(t, "create", []byte("cursor-a"), 1)
	repository := mcp.NewInMemoryMCPDiscoveryPageCommandRepositoryV1()
	winner, err := repository.EnsureMCPDiscoveryPageCommandV1(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	winner.Cursor[0] = 'X'
	inspected, err := repository.InspectMCPDiscoveryPageCommandV1(context.Background(), command.Ref)
	if err != nil || string(inspected.Cursor) != "cursor-a" {
		t.Fatalf("exact Inspect/deep copy failed: value=%#v err=%v", inspected, err)
	}
	if _, err = repository.EnsureMCPDiscoveryPageCommandV1(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	drifted := command
	drifted.Cursor = []byte("cursor-b")
	drifted.CursorDigest = core.DigestBytes(drifted.Cursor)
	if _, err = repository.EnsureMCPDiscoveryPageCommandV1(context.Background(), drifted); err == nil {
		t.Fatal("same ID with another cursor was accepted")
	}
}

func TestMCPDiscoveryPageCommandRepositoryV1ConcurrentSingleWinner(t *testing.T) {
	command := mcpDiscoveryPageCommandFixtureV1(t, "concurrent", nil, 0)
	repository := mcp.NewInMemoryMCPDiscoveryPageCommandRepositoryV1()
	const workers = 64
	values := make(chan toolcontract.MCPDiscoveryPageCommandV1, workers)
	errs := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			value, err := repository.EnsureMCPDiscoveryPageCommandV1(context.Background(), command)
			values <- value
			errs <- err
		}()
	}
	group.Wait()
	close(values)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for value := range values {
		if value.Ref != command.Ref {
			t.Fatal("concurrent Ensure returned another winner")
		}
	}
}

func TestMCPDiscoveryPageCommandV1FailClosedBindings(t *testing.T) {
	command := mcpDiscoveryPageCommandFixtureV1(t, "drift", nil, 0)
	for _, mutate := range []func(*toolcontract.MCPDiscoveryPageCommandV1){
		func(value *toolcontract.MCPDiscoveryPageCommandV1) { value.Namespace = "praxis.mcp/unknown" },
		func(value *toolcontract.MCPDiscoveryPageCommandV1) { value.Cursor = []byte("different") },
		func(value *toolcontract.MCPDiscoveryPageCommandV1) {
			value.Availability.ApplyDigest = testkit.Digest("different")
		},
		func(value *toolcontract.MCPDiscoveryPageCommandV1) { value.Provider.ComponentID = "praxis.mcp/other" },
	} {
		drifted := command
		mutate(&drifted)
		if err := drifted.Validate(); err == nil {
			t.Fatal("drifted command validated")
		}
	}
}

func mcpDiscoveryPageCommandFixtureV1(t *testing.T, suffix string, cursor []byte, ordinal uint32) toolcontract.MCPDiscoveryPageCommandV1 {
	t.Helper()
	now := testkit.FixedTime
	fixture := testkit.MCPConnectControlledV1(now, toolcontract.MCPTransportStreamableHTTPV1)
	provider := fixture.Connect.Intent.Provider
	provider.Capability = runtimeports.CapabilityNameV2(runtimeports.OperationScopeEvidenceMCPDiscoveryPageEffectKindV1)
	prepared := fixture.Authorization.Prepared
	prepared.Provider = provider
	prepared.Digest = ""
	prepared, err := runtimeports.SealPreparedProviderAttemptRefV2(prepared)
	if err != nil {
		t.Fatal(err)
	}
	owner := runtimeports.EffectOwnerRefV2{Role: runtimeports.OwnerSettlement, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest}
	connection := toolcontract.MCPConnectionFactRefV2{ID: "mcp-discovery-page-connection-" + suffix, Revision: 1, Digest: testkit.Digest("mcp-discovery-page-connection-" + suffix)}
	availability := runtimeports.MCPConnectionAvailabilityNeutralRefV1{
		Owner: owner, ConnectionID: connection.ID, ConnectionRevision: connection.Revision, ConnectionDigest: connection.Digest,
		ApplyID: "mcp-discovery-page-apply-" + suffix, ApplyRevision: 1, ApplyDigest: testkit.Digest("mcp-discovery-page-apply-" + suffix),
		DomainResultID: "mcp-discovery-page-result-" + suffix, DomainResultRevision: 1, DomainResultDigest: testkit.Digest("mcp-discovery-page-result-" + suffix), SourceProjectionDigest: testkit.Digest("mcp-discovery-page-current-" + suffix),
	}
	if err := availability.Validate(); err != nil {
		t.Fatalf("availability fixture invalid: %v owner=%#v", err, owner)
	}
	if err := prepared.Validate(); err != nil {
		t.Fatalf("prepared fixture invalid after Provider capability bind: %v", err)
	}
	if err := provider.Validate(); err != nil {
		t.Fatalf("provider fixture invalid: %v", err)
	}
	command, err := toolcontract.SealMCPDiscoveryPageCommandV1(toolcontract.MCPDiscoveryPageCommandV1{
		Owner: owner, Connection: connection, Availability: availability, Namespace: runtimeports.MCPDiscoveryPageToolsNamespaceV1, Cursor: cursor, PageOrdinal: ordinal,
		Operation: fixture.Connect.Intent.Operation, OperationDigest: fixture.Connect.Intent.OperationDigest,
		EffectID: fixture.Authorization.EffectID, EffectRevision: fixture.Authorization.EffectRevision,
		EffectKind: runtimeports.OperationScopeEvidenceMCPDiscoveryPageEffectKindV1, PolicyProfile: runtimeports.OperationScopeEvidenceMCPDiscoveryPagePolicyProfileV1,
		IntentDigest: fixture.Authorization.IntentDigest, Prepared: prepared, Attempt: fixture.Authorization.Attempt, Provider: provider,
		CreatedUnixNano: now.Add(-time.Second).UnixNano(), NotAfterUnixNano: now.Add(5 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return command
}

package mcp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	officialmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestOfficialSDKDiscoveryPageExecutorV1ExactOnePage(t *testing.T) {
	f := newDiscoveryPageExecutorFixtureV1(t, "positive")
	receipt, err := f.executor.DiscoverControlledMCPPageV1(context.Background(), f.authorization)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := f.entries.InspectMCPDiscoveryPagePhysicalV1(context.Background(), f.authorization.StableKeyDigest)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Admitted || entry.State != MCPDiscoveryPagePhysicalObservedV1 || entry.ProtocolReceipt == nil || entry.Observation == nil || f.session.calls.Load() != 1 || len(entry.Observation.Tools) != 1 || len(entry.ToolMaterials) != 1 {
		t.Fatalf("single-page closure drifted: entry=%#v calls=%d", entry, f.session.calls.Load())
	}
	material, err := f.entries.InspectExactMCPToolDiscoveryMaterialV1(context.Background(), entry.ToolMaterials[0].Ref)
	if err != nil || material.Source != entry.Observation.Tools[0] || string(material.CanonicalObject) != `{"inputSchema":{"type":"object"},"name":"echo"}` {
		t.Fatalf("exact Tool material drifted: material=%#v err=%v", material, err)
	}
	material.CanonicalObject[0] = '['
	again, err := f.entries.InspectExactMCPToolDiscoveryMaterialV1(context.Background(), entry.ToolMaterials[0].Ref)
	if err != nil || string(again.CanonicalObject) != `{"inputSchema":{"type":"object"},"name":"echo"}` {
		t.Fatalf("Tool material was not deep-cloned: material=%#v err=%v", again, err)
	}
	set, err := f.entries.InspectMCPDiscoveryPageToolMaterialSetV1(context.Background(), entry.ProtocolReceipt.Ref)
	if err != nil || set.Receipt != entry.ProtocolReceipt.Ref || set.Command != entry.Command || set.Connection != entry.Connection || set.ResponsePageDigest != entry.ProtocolReceipt.ResponsePageDigest || len(set.Entries) != 1 || set.Entries[0].Material != entry.ToolMaterials[0].Ref || set.Entries[0].Source != entry.Observation.Tools[0] {
		t.Fatalf("Tool material set drifted: set=%#v err=%v", set, err)
	}
	set.Entries[0].Source.Name = "mutated"
	againSet, err := f.entries.InspectMCPDiscoveryPageToolMaterialSetV1(context.Background(), entry.ProtocolReceipt.Ref)
	if err != nil || againSet.Entries[0].Source.Name != "echo" {
		t.Fatalf("Tool material set was not deep-cloned: set=%#v err=%v", againSet, err)
	}
	second, err := f.executor.DiscoverControlledMCPPageV1(context.Background(), f.authorization)
	if err != nil || second != receipt || f.session.calls.Load() != 1 {
		t.Fatalf("observed retry was not inspect-only: receipt=%#v err=%v calls=%d", second, err, f.session.calls.Load())
	}
}

func TestOfficialSDKDiscoveryPageExecutorV1ResourceAndPromptMaterials(t *testing.T) {
	t.Run("resource", func(t *testing.T) {
		f := newDiscoveryPageExecutorFixtureForNamespaceV1(t, "resource-material", runtimeports.MCPDiscoveryPageResourcesNamespaceV1)
		if _, err := f.executor.DiscoverControlledMCPPageV1(context.Background(), f.authorization); err != nil {
			t.Fatal(err)
		}
		entry, err := f.entries.InspectMCPDiscoveryPagePhysicalV1(context.Background(), f.authorization.StableKeyDigest)
		if err != nil || entry.State != MCPDiscoveryPagePhysicalObservedV1 || len(entry.ResourceMaterials) != 1 || len(entry.ToolMaterials) != 0 || len(entry.PromptMaterials) != 0 || len(entry.Observation.Resources) != 1 {
			t.Fatalf("Resource material closure drifted: entry=%#v err=%v", entry, err)
		}
		material, err := f.entries.InspectExactMCPResourceDiscoveryMaterialV1(context.Background(), entry.ResourceMaterials[0].Ref)
		if err != nil || material.Source != entry.Observation.Resources[0] || string(material.CanonicalObject) != `{"description":"workspace readme","mimeType":"text/markdown","name":"readme","size":128,"title":"README","uri":"file:///workspace/readme.md"}` {
			t.Fatalf("exact Resource material drifted: material=%#v err=%v", material, err)
		}
		material.CanonicalObject[0] = '['
		again, err := f.entries.InspectExactMCPResourceDiscoveryMaterialV1(context.Background(), entry.ResourceMaterials[0].Ref)
		if err != nil || again.Validate() != nil {
			t.Fatalf("Resource material was not deep-cloned: material=%#v err=%v", again, err)
		}
		wrong := entry.ResourceMaterials[0].Ref
		wrong.Digest = testkit.Digest("wrong-resource-material")
		if _, err = f.entries.InspectExactMCPResourceDiscoveryMaterialV1(context.Background(), wrong); !core.HasReason(err, core.ReasonBindingDrift) {
			t.Fatalf("wrong Resource material Ref error=%v", err)
		}
		set, err := f.entries.InspectMCPDiscoveryPageResourceMaterialSetV1(context.Background(), entry.ProtocolReceipt.Ref)
		if err != nil || set.Receipt != entry.ProtocolReceipt.Ref || len(set.Entries) != 1 || set.Entries[0].Material != entry.ResourceMaterials[0].Ref || set.Entries[0].Source != entry.Observation.Resources[0] {
			t.Fatalf("Resource material set drifted: set=%#v err=%v", set, err)
		}
		tampered := again.Clone()
		tampered.CanonicalObject = []byte(`{"name":"readme","uri":"file:///other"}`)
		if err = tampered.Validate(); err == nil {
			t.Fatal("tampered Resource material was accepted")
		}
	})

	t.Run("prompt", func(t *testing.T) {
		f := newDiscoveryPageExecutorFixtureForNamespaceV1(t, "prompt-material", runtimeports.MCPDiscoveryPagePromptsNamespaceV1)
		if _, err := f.executor.DiscoverControlledMCPPageV1(context.Background(), f.authorization); err != nil {
			t.Fatal(err)
		}
		entry, err := f.entries.InspectMCPDiscoveryPagePhysicalV1(context.Background(), f.authorization.StableKeyDigest)
		if err != nil || entry.State != MCPDiscoveryPagePhysicalObservedV1 || len(entry.PromptMaterials) != 1 || len(entry.ToolMaterials) != 0 || len(entry.ResourceMaterials) != 0 || len(entry.Observation.Prompts) != 1 {
			t.Fatalf("Prompt material closure drifted: entry=%#v err=%v", entry, err)
		}
		material, err := f.entries.InspectExactMCPPromptDiscoveryMaterialV1(context.Background(), entry.PromptMaterials[0].Ref)
		if err != nil || material.Source != entry.Observation.Prompts[0] || string(material.CanonicalObject) != `{"arguments":[{"name":"scope","required":true}],"description":"review changes","name":"review","title":"Review"}` {
			t.Fatalf("exact Prompt material drifted: material=%#v err=%v", material, err)
		}
		material.CanonicalObject[0] = '['
		again, err := f.entries.InspectExactMCPPromptDiscoveryMaterialV1(context.Background(), entry.PromptMaterials[0].Ref)
		if err != nil || again.Validate() != nil {
			t.Fatalf("Prompt material was not deep-cloned: material=%#v err=%v", again, err)
		}
		wrong := entry.PromptMaterials[0].Ref
		wrong.Digest = testkit.Digest("wrong-prompt-material")
		if _, err = f.entries.InspectExactMCPPromptDiscoveryMaterialV1(context.Background(), wrong); !core.HasReason(err, core.ReasonBindingDrift) {
			t.Fatalf("wrong Prompt material Ref error=%v", err)
		}
		set, err := f.entries.InspectMCPDiscoveryPagePromptMaterialSetV1(context.Background(), entry.ProtocolReceipt.Ref)
		if err != nil || set.Receipt != entry.ProtocolReceipt.Ref || len(set.Entries) != 1 || set.Entries[0].Material != entry.PromptMaterials[0].Ref || set.Entries[0].Source != entry.Observation.Prompts[0] {
			t.Fatalf("Prompt material set drifted: set=%#v err=%v", set, err)
		}
		tampered := again.Clone()
		tampered.CanonicalObject = []byte(`{"name":"other"}`)
		if err = tampered.Validate(); err == nil {
			t.Fatal("tampered Prompt material was accepted")
		}
	})
}

func TestOfficialSDKDiscoveryPageResourcePromptMaterialsConcurrentExactInspect(t *testing.T) {
	for _, item := range []struct {
		name      string
		namespace runtimeports.NamespacedNameV2
	}{{"resources", runtimeports.MCPDiscoveryPageResourcesNamespaceV1}, {"prompts", runtimeports.MCPDiscoveryPagePromptsNamespaceV1}} {
		t.Run(item.name, func(t *testing.T) {
			namespace := item.namespace
			f := newDiscoveryPageExecutorFixtureForNamespaceV1(t, "concurrent-"+item.name, namespace)
			if _, err := f.executor.DiscoverControlledMCPPageV1(context.Background(), f.authorization); err != nil {
				t.Fatal(err)
			}
			entry, _ := f.entries.InspectMCPDiscoveryPagePhysicalV1(context.Background(), f.authorization.StableKeyDigest)
			const workers = 64
			var group sync.WaitGroup
			errs := make(chan error, workers)
			for range workers {
				group.Add(1)
				go func() {
					defer group.Done()
					var err error
					if namespace == runtimeports.MCPDiscoveryPageResourcesNamespaceV1 {
						_, err = f.entries.InspectExactMCPResourceDiscoveryMaterialV1(context.Background(), entry.ResourceMaterials[0].Ref)
					} else {
						_, err = f.entries.InspectExactMCPPromptDiscoveryMaterialV1(context.Background(), entry.PromptMaterials[0].Ref)
					}
					errs <- err
				}()
			}
			group.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestOfficialSDKDiscoveryPageResourcePromptMaterialsFailClosed(t *testing.T) {
	resource := newDiscoveryPageExecutorFixtureForNamespaceV1(t, "resource-fail-closed", runtimeports.MCPDiscoveryPageResourcesNamespaceV1)
	if _, err := resource.executor.DiscoverControlledMCPPageV1(context.Background(), resource.authorization); err != nil {
		t.Fatal(err)
	}
	resourceEntry, _ := resource.entries.InspectMCPDiscoveryPagePhysicalV1(context.Background(), resource.authorization.StableKeyDigest)
	if _, err := resource.entries.InspectExactMCPResourceDiscoveryMaterialV1(nil, resourceEntry.ResourceMaterials[0].Ref); err == nil {
		t.Fatal("nil Resource material context was accepted")
	}
	prompt := newDiscoveryPageExecutorFixtureForNamespaceV1(t, "prompt-fail-closed", runtimeports.MCPDiscoveryPagePromptsNamespaceV1)
	if _, err := prompt.executor.DiscoverControlledMCPPageV1(context.Background(), prompt.authorization); err != nil {
		t.Fatal(err)
	}
	promptEntry, _ := prompt.entries.InspectMCPDiscoveryPagePhysicalV1(context.Background(), prompt.authorization.StableKeyDigest)
	if _, err := prompt.entries.InspectExactMCPPromptDiscoveryMaterialV1(nil, promptEntry.PromptMaterials[0].Ref); err == nil {
		t.Fatal("nil Prompt material context was accepted")
	}
	var unavailable *InMemoryMCPDiscoveryPagePhysicalRepositoryV1
	if _, err := unavailable.InspectExactMCPResourceDiscoveryMaterialV1(context.Background(), resourceEntry.ResourceMaterials[0].Ref); err == nil {
		t.Fatal("typed-nil Resource material repository was accepted")
	}
	if _, err := unavailable.InspectExactMCPPromptDiscoveryMaterialV1(context.Background(), promptEntry.PromptMaterials[0].Ref); err == nil {
		t.Fatal("typed-nil Prompt material repository was accepted")
	}
}

func TestOfficialSDKDiscoveryPageToolMaterialFailsClosed(t *testing.T) {
	f := newDiscoveryPageExecutorFixtureV1(t, "material-fail-closed")
	if _, err := f.executor.DiscoverControlledMCPPageV1(context.Background(), f.authorization); err != nil {
		t.Fatal(err)
	}
	entry, err := f.entries.InspectMCPDiscoveryPagePhysicalV1(context.Background(), f.authorization.StableKeyDigest)
	if err != nil {
		t.Fatal(err)
	}
	wrong := entry.ToolMaterials[0].Ref
	wrong.Digest = testkit.Digest("wrong-tool-material")
	if _, err = f.entries.InspectExactMCPToolDiscoveryMaterialV1(context.Background(), wrong); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("wrong exact material digest error=%v", err)
	}
	if _, err = f.entries.InspectExactMCPToolDiscoveryMaterialV1(nil, entry.ToolMaterials[0].Ref); err == nil {
		t.Fatal("nil context was accepted")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = f.entries.InspectExactMCPToolDiscoveryMaterialV1(canceled, entry.ToolMaterials[0].Ref); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context error=%v", err)
	}
	var unavailable *InMemoryMCPDiscoveryPagePhysicalRepositoryV1
	if _, err = unavailable.InspectExactMCPToolDiscoveryMaterialV1(context.Background(), entry.ToolMaterials[0].Ref); err == nil {
		t.Fatal("typed-nil material repository was accepted")
	}
	wrongReceipt := entry.ProtocolReceipt.Ref
	wrongReceipt.Digest = testkit.Digest("wrong-page-receipt")
	if _, err = f.entries.InspectMCPDiscoveryPageToolMaterialSetV1(context.Background(), wrongReceipt); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("wrong material set receipt error=%v", err)
	}
	if _, err = f.entries.InspectMCPDiscoveryPageToolMaterialSetV1(nil, entry.ProtocolReceipt.Ref); err == nil {
		t.Fatal("nil material set context was accepted")
	}
	tampered := cloneMCPToolDiscoveryMaterialsV1(entry.ToolMaterials)
	tampered[0].CanonicalObject = []byte(`{"inputSchema":{"type":"object"},"name":"other"}`)
	if err = f.entries.completeV1(context.Background(), f.authorization.StableKeyDigest, *entry.ProtocolReceipt, *entry.Observation, tampered, nil, nil, f.now); !core.HasReason(err, core.ReasonEvidenceConflict) {
		t.Fatalf("tampered material completion error=%v", err)
	}
}

func TestOfficialSDKDiscoveryPageToolMaterialConcurrentExactInspect(t *testing.T) {
	f := newDiscoveryPageExecutorFixtureV1(t, "material-read-concurrent")
	if _, err := f.executor.DiscoverControlledMCPPageV1(context.Background(), f.authorization); err != nil {
		t.Fatal(err)
	}
	entry, _ := f.entries.InspectMCPDiscoveryPagePhysicalV1(context.Background(), f.authorization.StableKeyDigest)
	const workers = 64
	var group sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			material, err := f.entries.InspectExactMCPToolDiscoveryMaterialV1(context.Background(), entry.ToolMaterials[0].Ref)
			if err == nil && material.Ref != entry.ToolMaterials[0].Ref {
				err = errors.New("material Ref drifted")
			}
			if err == nil {
				set, setErr := f.entries.InspectMCPDiscoveryPageToolMaterialSetV1(context.Background(), entry.ProtocolReceipt.Ref)
				if setErr != nil || len(set.Entries) != 1 || set.Entries[0].Material != material.Ref {
					err = errors.New("material set drifted")
				}
			}
			errs <- err
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestOfficialSDKDiscoveryPageExecutorV1LostReplyInspectOnly(t *testing.T) {
	f := newDiscoveryPageExecutorFixtureV1(t, "lost-reply")
	f.session.err = errors.New("provider reply lost")
	if _, err := f.executor.DiscoverControlledMCPPageV1(context.Background(), f.authorization); !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("lost reply error=%v", err)
	}
	f.session.err = nil
	if _, err := f.executor.DiscoverControlledMCPPageV1(context.Background(), f.authorization); !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("unknown retry error=%v", err)
	}
	entry, _ := f.entries.InspectMCPDiscoveryPagePhysicalV1(context.Background(), f.authorization.StableKeyDigest)
	if entry.State != MCPDiscoveryPagePhysicalUnknownV1 || f.session.calls.Load() != 1 {
		t.Fatalf("lost reply redispatched: state=%s calls=%d", entry.State, f.session.calls.Load())
	}
}

func TestOfficialSDKDiscoveryPageExecutorV1ConcurrentSingleAdmission(t *testing.T) {
	f := newDiscoveryPageExecutorFixtureV1(t, "concurrent")
	const workers = 64
	start := make(chan struct{})
	f.session.start = start
	var group sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := f.executor.DiscoverControlledMCPPageV1(context.Background(), f.authorization)
			errs <- err
		}()
	}
	for f.session.calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	close(start)
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil && !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
			t.Fatalf("concurrent error=%v", err)
		}
	}
	if f.session.calls.Load() != 1 {
		t.Fatalf("same canonical page called Provider %d times", f.session.calls.Load())
	}
}

func TestOfficialSDKDiscoveryPageExecutorV1TypedNil(t *testing.T) {
	f := newDiscoveryPageExecutorFixtureV1(t, "typed-nil")
	var commands *InMemoryMCPDiscoveryPageCommandRepositoryV1
	if _, err := NewOfficialSDKDiscoveryPageExecutorV1(commands, f.source, f.source, f.source, f.entries, func() time.Time { return f.now }); err == nil {
		t.Fatal("typed-nil command reader was accepted")
	}
}

type discoveryPageExecutorFixtureV1 struct {
	now            time.Time
	authorization  runtimeports.ControlledMCPDiscoveryPagePhysicalAuthorizationV1
	connectReceipt toolcontract.MCPConnectProtocolReceiptV1
	executor       *OfficialSDKDiscoveryPageExecutorV1
	entries        *InMemoryMCPDiscoveryPagePhysicalRepositoryV1
	session        *discoveryPageSessionV1
	source         *discoveryPageExecutorSourceV1
}

func newDiscoveryPageExecutorFixtureV1(t *testing.T, suffix string) discoveryPageExecutorFixtureV1 {
	return newDiscoveryPageExecutorFixtureForNamespaceV1(t, suffix, runtimeports.MCPDiscoveryPageToolsNamespaceV1)
}

func newDiscoveryPageExecutorFixtureForNamespaceV1(t *testing.T, suffix string, namespace runtimeports.NamespacedNameV2) discoveryPageExecutorFixtureV1 {
	t.Helper()
	now := testkit.FixedTime
	base := testkit.MCPConnectControlledV1(now, toolcontract.MCPTransportStreamableHTTPV1)
	provider := base.Connect.Intent.Provider
	provider.Capability = runtimeports.CapabilityNameV2(runtimeports.OperationScopeEvidenceMCPDiscoveryPageEffectKindV1)
	transport := base.Connect.Intent.ProviderTransport
	transport.Capability = runtimeports.ControlledMCPDiscoveryPageProviderTransportCapabilityV1
	owner := runtimeports.EffectOwnerRefV2{Role: runtimeports.OwnerSettlement, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest}
	capabilities := `{"tools":{}}`
	switch namespace {
	case runtimeports.MCPDiscoveryPageResourcesNamespaceV1:
		capabilities = `{"resources":{}}`
	case runtimeports.MCPDiscoveryPagePromptsNamespaceV1:
		capabilities = `{"prompts":{}}`
	}
	connectReceipt := testkit.MCPConnectReceiptV1(base, []byte(fmt.Sprintf(`{"protocolVersion":%q,"serverInfo":{"name":"discovery-test-server","version":"1.0.0"},"capabilities":%s,"instructions":"Use the discovered capabilities."}`, toolcontract.MCPStableProtocolVersion, capabilities)), now)
	connection, err := toolcontract.SealMCPConnectionFactV2(toolcontract.MCPConnectionFactV2{Owner: owner, Coordinate: base.Connect.Intent.Coordinate, Intent: base.Connect.Intent.Ref, TransportConfig: base.Connect.Config.Ref, Server: base.Connect.Intent.Server, ProtocolReceipt: connectReceipt.Ref, ProviderTransport: transport, Provider: provider, NegotiatedProtocol: connectReceipt.NegotiatedProtocol, ProviderSessionID: "discovery-session", CreatedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	availability, err := toolcontract.SealMCPConnectionAvailabilityCurrentProjectionV1(toolcontract.MCPConnectionAvailabilityCurrentProjectionV1{Connection: connection.Ref, ApplySettlement: toolcontract.ObjectRef{ID: "discovery-apply-" + suffix, Revision: 1, Digest: testkit.Digest("discovery-apply-" + suffix)}, DomainResult: toolcontract.ObjectRef{ID: "discovery-connect-result-" + suffix, Revision: 1, Digest: testkit.Digest("discovery-connect-result-" + suffix)}, Owner: owner, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano()}, now)
	if err != nil {
		t.Fatal(err)
	}
	availabilityRef := runtimeports.MCPConnectionAvailabilityNeutralRefV1{Owner: availability.Owner, ConnectionID: availability.Connection.ID, ConnectionRevision: availability.Connection.Revision, ConnectionDigest: availability.Connection.Digest, ApplyID: availability.ApplySettlement.ID, ApplyRevision: availability.ApplySettlement.Revision, ApplyDigest: availability.ApplySettlement.Digest, DomainResultID: availability.DomainResult.ID, DomainResultRevision: availability.DomainResult.Revision, DomainResultDigest: availability.DomainResult.Digest, SourceProjectionDigest: availability.Digest}
	availabilityCurrent, err := runtimeports.SealMCPConnectionAvailabilityNeutralProjectionV1(runtimeports.MCPConnectionAvailabilityNeutralProjectionV1{Ref: availabilityRef, TenantID: core.TenantID(connection.Coordinate.TenantID), RunID: connection.Coordinate.RunID, SessionID: connection.Coordinate.Session.ID, SessionRevision: connection.Coordinate.Session.Revision, SessionDigest: connection.Coordinate.Session.Digest, ConnectionEpoch: connection.Coordinate.Epoch, ProviderTransport: transport, Provider: provider, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	prepared := base.Authorization.Prepared
	prepared.Provider, prepared.Digest = provider, ""
	prepared, err = runtimeports.SealPreparedProviderAttemptRefV2(prepared)
	if err != nil {
		t.Fatal(err)
	}
	command, err := toolcontract.SealMCPDiscoveryPageCommandV1(toolcontract.MCPDiscoveryPageCommandV1{Owner: owner, Connection: connection.Ref, Availability: availabilityRef, Namespace: namespace, Operation: base.Authorization.Operation, OperationDigest: base.Authorization.OperationDigest, EffectID: base.Authorization.EffectID, EffectRevision: base.Authorization.EffectRevision, EffectKind: runtimeports.OperationScopeEvidenceMCPDiscoveryPageEffectKindV1, PolicyProfile: runtimeports.OperationScopeEvidenceMCPDiscoveryPagePolicyProfileV1, IntentDigest: base.Authorization.IntentDigest, Prepared: prepared, Attempt: base.Authorization.Attempt, Provider: provider, CreatedUnixNano: now.Add(-time.Second).UnixNano(), NotAfterUnixNano: now.Add(4 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	declaration := base.Authorization.Route.DeclarationRef
	conformance := base.Authorization.Route.ConformanceRef
	matrix, _ := runtimeports.DigestOperationScopeEvidenceMCPDiscoveryPageMatrixV1()
	route := runtimeports.ControlledMCPDiscoveryPageRouteCurrentRefV1{Revision: 1, DeclarationRef: declaration, ConformanceRef: conformance, MatrixDigest: matrix}
	route.CurrentID, _ = runtimeports.DeriveControlledMCPDiscoveryPageRouteCurrentIDV1(declaration.RouteID, matrix)
	route.Digest, _ = route.DigestV1()
	authorization, err := runtimeports.SealControlledMCPDiscoveryPagePhysicalAuthorizationV1(runtimeports.ControlledMCPDiscoveryPagePhysicalAuthorizationV1{UnifiedNotAfterUnixNano: now.Add(4 * time.Second).UnixNano(), Route: route, ProviderTransport: transport, Provider: provider, Operation: command.Operation, OperationDigest: command.OperationDigest, OperationScopeDigest: command.Operation.ExecutionScopeDigest, EffectID: command.EffectID, EffectRevision: command.EffectRevision, EffectFactRevision: base.Authorization.EffectFactRevision, IntentDigest: command.IntentDigest, Prepared: prepared, Attempt: command.Attempt, ExecuteEnforcement: base.Authorization.ExecuteEnforcement, PrepareConsumption: base.Authorization.PrepareConsumption, ExecuteHandoff: base.Authorization.ExecuteHandoff, SandboxProjectionDigest: base.Authorization.SandboxProjectionDigest, CredentialFactsDigest: base.Authorization.CredentialFactsDigest, Association: base.Authorization.Association, DomainCommand: command.RuntimeDomainCommandRefV1(), ConnectionAvailability: availabilityRef, Namespace: command.Namespace, CursorDigest: command.CursorDigest, PageOrdinal: command.PageOrdinal, IssuedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	commands := NewInMemoryMCPDiscoveryPageCommandRepositoryV1()
	if _, err = commands.EnsureMCPDiscoveryPageCommandV1(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	session := &discoveryPageSessionV1{namespace: namespace}
	source := &discoveryPageExecutorSourceV1{connection: connection, connectReceipt: connectReceipt, availability: availabilityCurrent, session: session}
	entries := NewInMemoryMCPDiscoveryPagePhysicalRepositoryV1()
	executor, err := NewOfficialSDKDiscoveryPageExecutorV1(commands, source, source, source, entries, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return discoveryPageExecutorFixtureV1{now: now, authorization: authorization, connectReceipt: connectReceipt, executor: executor, entries: entries, session: session, source: source}
}

type discoveryPageSessionV1 struct {
	calls     atomic.Int64
	err       error
	start     <-chan struct{}
	namespace runtimeports.NamespacedNameV2
}

func (s *discoveryPageSessionV1) InitializeResult() *officialmcp.InitializeResult {
	return &officialmcp.InitializeResult{}
}
func (s *discoveryPageSessionV1) ID() string   { return "discovery-session" }
func (s *discoveryPageSessionV1) Close() error { return nil }
func (s *discoveryPageSessionV1) ListTools(context.Context, *officialmcp.ListToolsParams) (*officialmcp.ListToolsResult, error) {
	if s.namespace != runtimeports.MCPDiscoveryPageToolsNamespaceV1 {
		return nil, errors.New("unexpected tools/list")
	}
	s.calls.Add(1)
	if s.start != nil {
		<-s.start
	}
	if s.err != nil {
		return nil, s.err
	}
	return &officialmcp.ListToolsResult{Tools: []*officialmcp.Tool{{Name: "echo", InputSchema: map[string]any{"type": "object"}}}}, nil
}
func (s *discoveryPageSessionV1) ListResources(context.Context, *officialmcp.ListResourcesParams) (*officialmcp.ListResourcesResult, error) {
	if s.namespace != runtimeports.MCPDiscoveryPageResourcesNamespaceV1 {
		return nil, errors.New("unexpected resources/list")
	}
	s.calls.Add(1)
	if s.start != nil {
		<-s.start
	}
	if s.err != nil {
		return nil, s.err
	}
	return &officialmcp.ListResourcesResult{Resources: []*officialmcp.Resource{{URI: "file:///workspace/readme.md", Name: "readme", Title: "README", Description: "workspace readme", MIMEType: "text/markdown", Size: 128}}}, nil
}
func (s *discoveryPageSessionV1) ListPrompts(context.Context, *officialmcp.ListPromptsParams) (*officialmcp.ListPromptsResult, error) {
	if s.namespace != runtimeports.MCPDiscoveryPagePromptsNamespaceV1 {
		return nil, errors.New("unexpected prompts/list")
	}
	s.calls.Add(1)
	if s.start != nil {
		<-s.start
	}
	if s.err != nil {
		return nil, s.err
	}
	return &officialmcp.ListPromptsResult{Prompts: []*officialmcp.Prompt{{Name: "review", Title: "Review", Description: "review changes", Arguments: []*officialmcp.PromptArgument{{Name: "scope", Required: true}}}}}, nil
}

type discoveryPageExecutorSourceV1 struct {
	connection     toolcontract.MCPConnectionFactV2
	connectReceipt toolcontract.MCPConnectProtocolReceiptV1
	availability   runtimeports.MCPConnectionAvailabilityNeutralProjectionV1
	session        OfficialSDKDiscoveryPageSessionV1
}

func (s *discoveryPageExecutorSourceV1) InspectCurrentMCPConnectionFactV2(context.Context, toolcontract.MCPConnectionFactRefV2) (toolcontract.MCPConnectionFactV2, error) {
	return s.connection, nil
}
func (s *discoveryPageExecutorSourceV1) InspectMCPConnectProtocolReceiptV1(context.Context, toolcontract.MCPConnectProtocolReceiptRefV1) (toolcontract.MCPConnectProtocolReceiptV1, error) {
	return s.connectReceipt, nil
}
func (s *discoveryPageExecutorSourceV1) InspectCurrentMCPConnectionAvailabilityNeutralV1(context.Context, runtimeports.MCPConnectionAvailabilityNeutralRefV1) (runtimeports.MCPConnectionAvailabilityNeutralProjectionV1, error) {
	return s.availability, nil
}
func (s *discoveryPageExecutorSourceV1) InspectMCPDiscoveryPageSessionV1(context.Context, toolcontract.MCPConnectProtocolReceiptRefV1) (OfficialSDKDiscoveryPageSessionV1, error) {
	return s.session, nil
}

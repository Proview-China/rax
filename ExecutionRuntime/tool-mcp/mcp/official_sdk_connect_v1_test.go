package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	officialmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestOfficialSDKConnectExecutorV1ObservedAndIdempotent(t *testing.T) {
	f := newMCPConnectExecutorFixtureV1(t, toolcontract.MCPTransportStreamableHTTPV1, "http://127.0.0.1:8123/mcp")
	driver := &fakeMCPConnectDriverV1{session: newFakeConnectSessionV1()}
	executor := f.executor(t, driver, f.clock)
	first, err := executor.ConnectControlledMCPV1(context.Background(), f.authorization)
	if err != nil {
		t.Fatal(err)
	}
	second, err := executor.ConnectControlledMCPV1(context.Background(), f.authorization)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || driver.calls.Load() != 1 {
		t.Fatalf("same canonical Connect was not idempotent: first=%#v second=%#v calls=%d", first, second, driver.calls.Load())
	}
	entry, err := executor.InspectMCPConnectPhysicalV1(context.Background(), f.authorization.StableKeyDigest)
	if err != nil || entry.State != MCPConnectPhysicalObservedV1 || entry.ProtocolReceipt == nil || entry.ProtocolReceipt.Validate() != nil {
		t.Fatalf("observed Connect receipt is not inspectable: entry=%#v err=%v", entry, err)
	}
	if _, err := f.entries.InspectMCPConnectProtocolReceiptV1(context.Background(), entry.ProtocolReceipt.Ref); err != nil {
		t.Fatal(err)
	}
}

func TestOfficialSDKConnectExecutorV1ConcurrentSingleAdmission(t *testing.T) {
	f := newMCPConnectExecutorFixtureV1(t, toolcontract.MCPTransportStreamableHTTPV1, "http://127.0.0.1:8123/mcp")
	driver := &fakeMCPConnectDriverV1{session: newFakeConnectSessionV1(), release: make(chan struct{})}
	executor := f.executor(t, driver, f.clock)
	const workers = 64
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := executor.ConnectControlledMCPV1(context.Background(), f.authorization)
			errs <- err
		}()
	}
	close(start)
	for driver.calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	close(driver.release)
	wg.Wait()
	close(errs)
	if driver.calls.Load() != 1 {
		t.Fatalf("same stable key reached Provider %d times", driver.calls.Load())
	}
	for err := range errs {
		if err != nil && !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
			t.Fatalf("concurrent duplicate returned unexpected error: %v", err)
		}
	}
}

func TestOfficialSDKConnectExecutorV1UnknownIsInspectOnly(t *testing.T) {
	f := newMCPConnectExecutorFixtureV1(t, toolcontract.MCPTransportStreamableHTTPV1, "http://127.0.0.1:8123/mcp")
	driver := &fakeMCPConnectDriverV1{err: context.DeadlineExceeded}
	executor := f.executor(t, driver, f.clock)
	if _, err := executor.ConnectControlledMCPV1(context.Background(), f.authorization); !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("lost Provider reply error=%v", err)
	}
	if _, err := executor.ConnectControlledMCPV1(context.Background(), f.authorization); !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("unknown retry error=%v", err)
	}
	if driver.calls.Load() != 1 {
		t.Fatalf("unknown Connect was re-dispatched: calls=%d", driver.calls.Load())
	}
	entry, err := executor.InspectMCPConnectPhysicalV1(context.Background(), f.authorization.StableKeyDigest)
	if err != nil || entry.State != MCPConnectPhysicalUnknownV1 || entry.ProtocolReceipt != nil {
		t.Fatalf("unknown Connect exact Inspect mismatch: %#v err=%v", entry, err)
	}
}

func TestOfficialSDKConnectExecutorV1NegotiatedProtocolOutsideDescriptorIsInspectOnly(t *testing.T) {
	f := newMCPConnectExecutorFixtureV1(t, toolcontract.MCPTransportStreamableHTTPV1, "http://127.0.0.1:8123/mcp")
	session := newFakeConnectSessionV1()
	session.initialize.ProtocolVersion = "2025-06-18"
	driver := &fakeMCPConnectDriverV1{session: session}
	executor := f.executor(t, driver, f.clock)

	if _, err := executor.ConnectControlledMCPV1(context.Background(), f.authorization); !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("out-of-range negotiated protocol error=%v", err)
	}
	if _, err := executor.ConnectControlledMCPV1(context.Background(), f.authorization); !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("out-of-range negotiated protocol retry error=%v", err)
	}
	if driver.calls.Load() != 1 {
		t.Fatalf("out-of-range negotiated protocol was re-dispatched: calls=%d", driver.calls.Load())
	}
	entry, err := executor.InspectMCPConnectPhysicalV1(context.Background(), f.authorization.StableKeyDigest)
	if err != nil || entry.State != MCPConnectPhysicalUnknownV1 || entry.ProtocolReceipt != nil || entry.session != session || entry.UnknownReasonDigest.Validate() != nil {
		t.Fatalf("out-of-range negotiated protocol residual mismatch: entry=%#v err=%v", entry, err)
	}
}

func TestOfficialSDKConnectExecutorV1InitializeResponseDriftIsInspectOnly(t *testing.T) {
	f := newMCPConnectExecutorFixtureV1(t, toolcontract.MCPTransportStreamableHTTPV1, "http://127.0.0.1:8123/mcp")
	session := newFakeConnectSessionV1()
	driver := &fakeMCPConnectDriverV1{session: session, response: []byte(`{"protocolVersion":"2025-11-25","serverInfo":{"name":"different","version":"1.0.0"},"capabilities":{}}`)}
	executor := f.executor(t, driver, f.clock)

	if _, err := executor.ConnectControlledMCPV1(context.Background(), f.authorization); !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("initialize response drift error=%v", err)
	}
	if _, err := executor.ConnectControlledMCPV1(context.Background(), f.authorization); !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("initialize response drift retry error=%v", err)
	}
	if driver.calls.Load() != 1 {
		t.Fatalf("initialize response drift was re-dispatched: calls=%d", driver.calls.Load())
	}
	entry, err := executor.InspectMCPConnectPhysicalV1(context.Background(), f.authorization.StableKeyDigest)
	if err != nil || entry.State != MCPConnectPhysicalUnknownV1 || entry.ProtocolReceipt != nil || entry.session != session {
		t.Fatalf("initialize response drift residual mismatch: entry=%#v err=%v", entry, err)
	}
}

func TestOfficialSDKConnectExecutorV1LostAdmissionReplyIsInspectOnly(t *testing.T) {
	f := newMCPConnectExecutorFixtureV1(t, toolcontract.MCPTransportStreamableHTTPV1, "http://127.0.0.1:8123/mcp")
	driver := &fakeMCPConnectDriverV1{session: newFakeConnectSessionV1()}
	lost := &lostMCPConnectStoreV1{delegate: f.entries, loseBegin: true}
	executor, err := newOfficialSDKConnectExecutorV1(f.intents, f.configs, f.servers, f.credentials, lost, driver, f.clock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = executor.ConnectControlledMCPV1(context.Background(), f.authorization); err == nil {
		t.Fatal("lost admission reply was reported as success")
	}
	lost.loseBegin = false
	if _, err = executor.ConnectControlledMCPV1(context.Background(), f.authorization); !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("lost admission retry error=%v", err)
	}
	if driver.calls.Load() != 0 {
		t.Fatalf("lost admission reply reached Provider: calls=%d", driver.calls.Load())
	}
}

func TestOfficialSDKConnectExecutorV1LostReceiptReplyRecoversByInspect(t *testing.T) {
	f := newMCPConnectExecutorFixtureV1(t, toolcontract.MCPTransportStreamableHTTPV1, "http://127.0.0.1:8123/mcp")
	driver := &fakeMCPConnectDriverV1{session: newFakeConnectSessionV1()}
	lost := &lostMCPConnectStoreV1{delegate: f.entries, loseComplete: true}
	executor, err := newOfficialSDKConnectExecutorV1(f.intents, f.configs, f.servers, f.credentials, lost, driver, f.clock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = executor.ConnectControlledMCPV1(context.Background(), f.authorization); !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("lost Receipt reply error=%v", err)
	}
	lost.loseComplete = false
	if _, err = executor.ConnectControlledMCPV1(context.Background(), f.authorization); err != nil {
		t.Fatalf("exact Inspect recovery failed: %v", err)
	}
	if driver.calls.Load() != 1 {
		t.Fatalf("lost Receipt reply re-dispatched Provider: calls=%d", driver.calls.Load())
	}
}

func TestOfficialSDKConnectExecutorV1RejectsCurrentDriftBeforeAdmission(t *testing.T) {
	f := newMCPConnectExecutorFixtureV1(t, toolcontract.MCPTransportStreamableHTTPV1, "http://127.0.0.1:8123/mcp")
	driver := &fakeMCPConnectDriverV1{session: newFakeConnectSessionV1()}
	intent := f.mustIntent(t)
	current, err := f.configs.InspectCurrentMCPTransportConfigV1(context.Background(), intent.TransportConfig.ID)
	if err != nil {
		t.Fatal(err)
	}
	current.Ref.Revision++
	current.CreatedUnixNano++
	current.Ref.Digest = ""
	current, err = toolcontract.SealMCPTransportConfigV1(current)
	if err != nil {
		t.Fatal(err)
	}
	previous := intent.TransportConfig
	if _, err = f.configs.EnsureMCPTransportConfigV1(context.Background(), EnsureMCPTransportConfigRequestV1{Config: current, ExpectedCurrent: &previous}); err != nil {
		t.Fatal(err)
	}
	executor := f.executor(t, driver, f.clock)
	if _, err = executor.ConnectControlledMCPV1(context.Background(), f.authorization); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("current Config drift error=%v", err)
	}
	if driver.calls.Load() != 0 {
		t.Fatal("current Config drift reached Provider")
	}
}

func TestOfficialSDKConnectExecutorV1FailClosedBoundaries(t *testing.T) {
	f := newMCPConnectExecutorFixtureV1(t, toolcontract.MCPTransportStreamableHTTPV1, "http://127.0.0.1:8123/mcp")
	driver := &fakeMCPConnectDriverV1{session: newFakeConnectSessionV1()}

	t.Run("nil_context", func(t *testing.T) {
		executor := f.executor(t, driver, f.clock)
		if _, err := executor.ConnectControlledMCPV1(nil, f.authorization); !core.HasCategory(err, core.ErrorInvalidArgument) {
			t.Fatalf("nil context error=%v", err)
		}
	})

	t.Run("typed_nil", func(t *testing.T) {
		var intents *InMemoryMCPConnectIntentRepositoryV1
		if _, err := newOfficialSDKConnectExecutorV1(intents, f.configs, f.servers, f.credentials, f.entries, driver, f.clock); err == nil {
			t.Fatal("typed-nil dependency was accepted")
		}
	})

	t.Run("clock_rollback", func(t *testing.T) {
		values := []time.Time{f.now, f.now.Add(-time.Nanosecond)}
		var index atomic.Int64
		executor := f.executor(t, driver, func() time.Time { return values[min(int(index.Add(1)-1), len(values)-1)] })
		if _, err := executor.ConnectControlledMCPV1(context.Background(), f.authorization); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("clock rollback error=%v", err)
		}
		if driver.calls.Load() != 0 {
			t.Fatal("clock rollback reached Provider")
		}
	})

	t.Run("expired_at_actual_point", func(t *testing.T) {
		late := time.Unix(0, f.authorization.UnifiedNotAfterUnixNano)
		executor := f.executor(t, driver, func() time.Time { return late })
		if _, err := executor.ConnectControlledMCPV1(context.Background(), f.authorization); !core.HasReason(err, core.ReasonBindingExpired) {
			t.Fatalf("expired actual point error=%v", err)
		}
		if driver.calls.Load() != 0 {
			t.Fatal("expired authorization reached Provider")
		}
	})
}

func TestOfficialMCPConnectDriverV1StreamableHTTP(t *testing.T) {
	server := officialmcp.NewServer(&officialmcp.Implementation{Name: "praxis-test-server", Version: "1.0.0"}, nil)
	handler := officialmcp.NewStreamableHTTPHandler(func(*http.Request) *officialmcp.Server { return server }, nil)
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()
	f := newMCPConnectExecutorFixtureV1(t, toolcontract.MCPTransportStreamableHTTPV1, httpServer.URL+"/mcp")
	executor := f.executor(t, officialMCPConnectDriverV1{}, f.clock)
	if _, err := executor.ConnectControlledMCPV1(context.Background(), f.authorization); err != nil {
		t.Fatal(err)
	}
	entry, err := executor.InspectMCPConnectPhysicalV1(context.Background(), f.authorization.StableKeyDigest)
	if err != nil || entry.ProtocolReceipt == nil || entry.ProtocolReceipt.NegotiatedProtocol != toolcontract.MCPStableProtocolVersion {
		t.Fatalf("official Streamable HTTP handshake mismatch: %#v err=%v", entry, err)
	}
	if entry.session != nil {
		defer entry.session.Close()
	}
}

func TestOfficialMCPConnectDriverV1Stdio(t *testing.T) {
	if os.Getenv("PRAXIS_MCP_STDIO_HELPER") == "1" {
		server := officialmcp.NewServer(&officialmcp.Implementation{Name: "praxis-stdio-server", Version: "1.0.0"}, nil)
		if err := server.Run(context.Background(), &officialmcp.StdioTransport{}); err != nil {
			os.Exit(2)
		}
		return
	}
	now := time.Now().UTC()
	config, _, _ := newMCPConnectFactsV1(t, now, toolcontract.MCPTransportStdioV1, "")
	config.Stdio.Executable = os.Args[0]
	config.Stdio.Arguments = []string{"-test.run=TestOfficialMCPConnectDriverV1Stdio"}
	config.Stdio.CredentialPlaceholders = []string{"PRAXIS_MCP_STDIO_HELPER"}
	config.Ref.Digest = ""
	var err error
	config, err = toolcontract.SealMCPTransportConfigV1(config)
	if err != nil {
		t.Fatal(err)
	}
	material := MCPConnectCredentialMaterialV1{CredentialFactsDigest: testDigestV1("stdio-credentials"), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano(), Environment: map[string]string{"PRAXIS_MCP_STDIO_HELPER": "1"}, Headers: map[string]string{}}
	session, response, err := (officialMCPConnectDriverV1{}).Connect(context.Background(), config, material)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if session.InitializeResult() == nil || len(response) == 0 {
		t.Fatal("official stdio handshake returned no initialize observation")
	}
}

func TestMCPConnectReceiptInspectorV2CreatesAuthoritativeFact(t *testing.T) {
	f := newMCPConnectExecutorFixtureV1(t, toolcontract.MCPTransportStreamableHTTPV1, "http://127.0.0.1:8123/mcp")
	driver := &fakeMCPConnectDriverV1{session: newFakeConnectSessionV1()}
	executor := f.executor(t, driver, f.clock)
	if _, err := executor.ConnectControlledMCPV1(context.Background(), f.authorization); err != nil {
		t.Fatal(err)
	}
	entry, err := executor.InspectMCPConnectPhysicalV1(context.Background(), f.authorization.StableKeyDigest)
	if err != nil || entry.ProtocolReceipt == nil {
		t.Fatalf("receipt missing: %#v err=%v", entry, err)
	}
	facts, err := NewInMemoryMCPConnectionFactRepositoryV2(f.clock)
	if err != nil {
		t.Fatal(err)
	}
	inspector, err := NewMCPConnectReceiptInspectorV2(f.entries, f.intents, f.configs, f.servers, facts, f.clock)
	if err != nil {
		t.Fatal(err)
	}
	request := InspectMCPConnectReceiptRequestV2{Receipt: entry.ProtocolReceipt.Ref, RequestedExpiresUnixNano: f.now.Add(7 * time.Second).UnixNano()}
	fact, err := inspector.InspectAndCreateMCPConnectionFactV2(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	again, err := inspector.InspectAndCreateMCPConnectionFactV2(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if fact.Ref != again.Ref || fact.Coordinate.Session.ID != "session-connect" || fact.ProviderSessionID != "provider-session-connect" || fact.ProviderSessionID == fact.Coordinate.Session.ID {
		t.Fatalf("Connection Fact lost Praxis/Provider Session separation: %#v", fact)
	}
	if _, err := facts.InspectCurrentMCPConnectionFactV2(context.Background(), fact.Ref); err != nil {
		t.Fatal(err)
	}
}

func TestMCPConnectReceiptInspectorV2ConcurrentSingleFact(t *testing.T) {
	f := newMCPConnectExecutorFixtureV1(t, toolcontract.MCPTransportStreamableHTTPV1, "http://127.0.0.1:8123/mcp")
	executor := f.executor(t, &fakeMCPConnectDriverV1{session: newFakeConnectSessionV1()}, f.clock)
	if _, err := executor.ConnectControlledMCPV1(context.Background(), f.authorization); err != nil {
		t.Fatal(err)
	}
	entry, _ := executor.InspectMCPConnectPhysicalV1(context.Background(), f.authorization.StableKeyDigest)
	facts, _ := NewInMemoryMCPConnectionFactRepositoryV2(f.clock)
	inspector, _ := NewMCPConnectReceiptInspectorV2(f.entries, f.intents, f.configs, f.servers, facts, f.clock)
	request := InspectMCPConnectReceiptRequestV2{Receipt: entry.ProtocolReceipt.Ref, RequestedExpiresUnixNano: f.now.Add(7 * time.Second).UnixNano()}
	const workers = 64
	refs := make(chan toolcontract.MCPConnectionFactRefV2, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fact, err := inspector.InspectAndCreateMCPConnectionFactV2(context.Background(), request)
			refs <- fact.Ref
			errs <- err
		}()
	}
	wg.Wait()
	close(refs)
	close(errs)
	var winner toolcontract.MCPConnectionFactRefV2
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for ref := range refs {
		if winner.ID == "" {
			winner = ref
		} else if ref != winner {
			t.Fatalf("concurrent Connection Fact winners drifted: %#v != %#v", ref, winner)
		}
	}
}

func TestMCPConnectReceiptInspectorV2FailsClosed(t *testing.T) {
	f := newMCPConnectExecutorFixtureV1(t, toolcontract.MCPTransportStreamableHTTPV1, "http://127.0.0.1:8123/mcp")
	executor := f.executor(t, &fakeMCPConnectDriverV1{session: newFakeConnectSessionV1()}, f.clock)
	if _, err := executor.ConnectControlledMCPV1(context.Background(), f.authorization); err != nil {
		t.Fatal(err)
	}
	entry, _ := executor.InspectMCPConnectPhysicalV1(context.Background(), f.authorization.StableKeyDigest)
	facts, _ := NewInMemoryMCPConnectionFactRepositoryV2(f.clock)
	inspector, _ := NewMCPConnectReceiptInspectorV2(f.entries, f.intents, f.configs, f.servers, facts, f.clock)
	request := InspectMCPConnectReceiptRequestV2{Receipt: entry.ProtocolReceipt.Ref, RequestedExpiresUnixNano: f.now.Add(7 * time.Second).UnixNano()}

	t.Run("nil_context", func(t *testing.T) {
		if _, err := inspector.InspectAndCreateMCPConnectionFactV2(nil, request); !core.HasCategory(err, core.ErrorInvalidArgument) {
			t.Fatalf("nil context error=%v", err)
		}
	})

	t.Run("provider_session_drift", func(t *testing.T) {
		f.entries.mu.Lock()
		key := "mcp-connect-entry-" + strings.TrimPrefix(string(f.authorization.StableKeyDigest), "sha256:")
		value := f.entries.entries[key]
		value.session = &fakeConnectSessionV1{initialize: newFakeConnectSessionV1().initialize}
		value.ProtocolReceipt.ProviderSessionID = "another-provider-session"
		f.entries.entries[key] = value
		f.entries.mu.Unlock()
		if _, err := inspector.InspectAndCreateMCPConnectionFactV2(context.Background(), request); err == nil {
			t.Fatal("Provider Session drift created a Connection Fact")
		}
	})
}

func TestMCPConnectReceiptInspectorV2RejectsNegotiatedProtocolOutsideDescriptor(t *testing.T) {
	f := newMCPConnectExecutorFixtureV1(t, toolcontract.MCPTransportStreamableHTTPV1, "http://127.0.0.1:8123/mcp")
	executor := f.executor(t, &fakeMCPConnectDriverV1{session: newFakeConnectSessionV1()}, f.clock)
	if _, err := executor.ConnectControlledMCPV1(context.Background(), f.authorization); err != nil {
		t.Fatal(err)
	}
	entry, err := executor.InspectMCPConnectPhysicalV1(context.Background(), f.authorization.StableKeyDigest)
	if err != nil || entry.ProtocolReceipt == nil {
		t.Fatalf("receipt missing: entry=%#v err=%v", entry, err)
	}
	intent := f.mustIntent(t)
	config, err := f.configs.InspectMCPTransportConfigV1(context.Background(), intent.TransportConfig)
	if err != nil {
		t.Fatal(err)
	}
	server, err := f.servers.InspectMCPServerDescriptorV1(context.Background(), intent.Server)
	if err != nil {
		t.Fatal(err)
	}
	session := newFakeConnectSessionV1()
	session.initialize.ProtocolVersion = "2025-06-18"
	response, err := json.Marshal(session.initialize)
	if err != nil {
		t.Fatal(err)
	}
	receipt := *entry.ProtocolReceipt
	receipt.NegotiatedProtocol = session.initialize.ProtocolVersion
	receipt.InitializeResponse = response
	receipt.Ref = toolcontract.MCPConnectProtocolReceiptRefV1{}
	receipt.ResponseDigest = ""
	receipt, err = toolcontract.SealMCPConnectProtocolReceiptV1(receipt)
	if err != nil {
		t.Fatal(err)
	}
	entry.ProtocolReceipt = &receipt
	entry.session = session
	if err = validateMCPConnectReceiptClosureV2(entry, session, receipt, intent, config, server, f.now); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("out-of-range negotiated protocol inspection error=%v", err)
	}
}

type mcpConnectExecutorFixtureV1 struct {
	now           time.Time
	authorization runtimeports.ControlledMCPConnectPhysicalAuthorizationV1
	intents       *InMemoryMCPConnectIntentRepositoryV1
	configs       *InMemoryMCPTransportConfigRepositoryV1
	servers       *InMemoryMCPServerDescriptorRepositoryV1
	credentials   *fakeMCPConnectCredentialMaterializerV1
	entries       *InMemoryMCPConnectPhysicalRepositoryV1
	clock         func() time.Time
}

func (f mcpConnectExecutorFixtureV1) mustIntent(t *testing.T) toolcontract.MCPConnectIntentV1 {
	t.Helper()
	intent, err := f.intents.InspectMCPConnectIntentV1(context.Background(), toolcontract.ObjectRef{ID: f.authorization.DomainCommand.ID, Revision: f.authorization.DomainCommand.Revision, Digest: f.authorization.DomainCommand.Digest})
	if err != nil {
		t.Fatal(err)
	}
	return intent
}

func newMCPConnectExecutorFixtureV1(t *testing.T, kind runtimeports.NamespacedNameV2, endpoint string) mcpConnectExecutorFixtureV1 {
	t.Helper()
	now := time.Now().UTC()
	config, intent, server := newMCPConnectFactsV1(t, now, kind, endpoint)
	authorization := newMCPConnectAuthorizationV1(t, now, intent, config)
	intents := NewInMemoryMCPConnectIntentRepositoryV1()
	configs := NewInMemoryMCPTransportConfigRepositoryV1()
	servers := NewInMemoryMCPServerDescriptorRepositoryV1()
	if _, err := servers.EnsureMCPServerDescriptorV1(context.Background(), EnsureMCPServerDescriptorRequestV1{Descriptor: server}); err != nil {
		t.Fatal(err)
	}
	if _, err := configs.EnsureMCPTransportConfigV1(context.Background(), EnsureMCPTransportConfigRequestV1{Config: config}); err != nil {
		t.Fatal(err)
	}
	if _, err := intents.EnsureMCPConnectIntentV1(context.Background(), EnsureMCPConnectIntentRequestV1{Intent: intent}); err != nil {
		t.Fatal(err)
	}
	credentials := &fakeMCPConnectCredentialMaterializerV1{material: MCPConnectCredentialMaterialV1{
		CredentialFactsDigest: authorization.CredentialFactsDigest,
		CheckedUnixNano:       now.Add(-time.Second).UnixNano(), ExpiresUnixNano: authorization.UnifiedNotAfterUnixNano,
		Environment: map[string]string{}, Headers: map[string]string{},
	}}
	return mcpConnectExecutorFixtureV1{now: now, authorization: authorization, intents: intents, configs: configs, servers: servers, credentials: credentials, entries: NewInMemoryMCPConnectPhysicalRepositoryV1(), clock: func() time.Time { return now }}
}

func (f mcpConnectExecutorFixtureV1) executor(t *testing.T, driver mcpOfficialConnectDriverV1, clock func() time.Time) *OfficialSDKConnectExecutorV1 {
	t.Helper()
	executor, err := newOfficialSDKConnectExecutorV1(f.intents, f.configs, f.servers, f.credentials, f.entries, driver, clock)
	if err != nil {
		t.Fatal(err)
	}
	return executor
}

func newMCPConnectFactsV1(t *testing.T, now time.Time, kind runtimeports.NamespacedNameV2, endpoint string) (toolcontract.MCPTransportConfigV1, toolcontract.MCPConnectIntentV1, toolcontract.MCPServerDescriptor) {
	t.Helper()
	provider := runtimeports.ProviderBindingRefV2{BindingSetID: "mcp-binding-v1", BindingSetRevision: 1, ComponentID: "praxis.mcp/owner", ManifestDigest: testDigestV1("provider-manifest"), ArtifactDigest: testDigestV1("provider-artifact"), Capability: runtimeports.CapabilityNameV2(toolcontract.MCPConnectEffectKindV1)}
	transport := runtimeports.ProviderBindingRefV2{BindingSetID: provider.BindingSetID, BindingSetRevision: provider.BindingSetRevision, ComponentID: "praxis.mcp/transport", ManifestDigest: testDigestV1("transport-manifest"), ArtifactDigest: testDigestV1("transport-artifact"), Capability: runtimeports.ControlledMCPConnectProviderTransportCapabilityV1}
	server, err := toolcontract.SealMCPServer(toolcontract.MCPServerDescriptor{ID: "mcp-server-connect-v1", Revision: 1, Owner: core.OwnerRef{Domain: "praxis.tool-mcp", ID: "registry"}, Source: "praxis.mcp/source-test", MinimumProtocol: toolcontract.MCPStableProtocolVersion, MaximumProtocol: toolcontract.MCPStableProtocolVersion, Transports: []runtimeports.NamespacedNameV2{kind}, AuthRequirement: "praxis.mcp.auth/none", TrustClass: "praxis.mcp.trust/test", NetworkScopeDigest: testDigestV1("network"), ArtifactDigest: testDigestV1("artifact"), ConfigDigest: testDigestV1("config"), Conformance: "praxis.mcp.conformance/official-go-sdk", CreatedUnixNano: now.Add(-time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	serverRef := toolcontract.ObjectRef{ID: server.ID, Revision: server.Revision, Digest: server.Digest}
	config := toolcontract.MCPTransportConfigV1{Ref: toolcontract.MCPTransportConfigRefV1{Revision: 1}, Owner: server.Owner, Server: serverRef, Kind: kind, ProviderTransport: transport, ArtifactDigest: server.ArtifactDigest, ConfigDigest: server.ConfigDigest, NetworkScopeDigest: server.NetworkScopeDigest, SandboxRequirementDigest: testDigestV1("sandbox"), CreatedUnixNano: now.Add(-time.Second).UnixNano()}
	if kind == toolcontract.MCPTransportStreamableHTTPV1 {
		config.StreamableHTTP = &toolcontract.MCPStreamableHTTPTransportConfigV1{Endpoint: endpoint, DisableStandaloneSSE: true}
	} else {
		config.Stdio = &toolcontract.MCPStdioTransportConfigV1{Executable: "/bin/false"}
	}
	config, err = toolcontract.SealMCPTransportConfigV1(config)
	if err != nil {
		t.Fatal(err)
	}
	execution := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-connect", ID: "identity-connect", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-connect", PlanDigest: testDigestV1("plan")}, Instance: core.InstanceRef{ID: "instance-connect", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "sandbox-connect", Epoch: 1}, AuthorityEpoch: 1}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(execution)
	if err != nil {
		t.Fatal(err)
	}
	operation := runtimeports.OperationSubjectV3{Kind: runtimeports.OperationScopeRunV3, ExecutionScope: execution, ExecutionScopeDigest: scopeDigest, RunID: "run-connect", SubjectRevision: 1, CurrentProjectionRef: "run-current-connect", CurrentProjectionDigest: testDigestV1("run-current"), CurrentProjectionRevision: 1}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	delegation := runtimeports.ExecutionDelegationRefV2{ID: "delegation-connect", Revision: 2, Digest: testDigestV1("delegation")}
	attempt := runtimeports.OperationDispatchAttemptRefV3{OperationDigest: operationDigest, EffectID: "effect-connect", IntentRevision: 1, IntentDigest: testDigestV1("intent"), PermitID: "permit-connect", PermitRevision: 1, PermitDigest: testDigestV1("permit"), AttemptID: "attempt-connect", Delegation: &delegation}
	coordinate, err := toolcontract.SealMCPConnectionCoordinateV1(toolcontract.MCPConnectionCoordinateV1{TenantID: string(execution.Identity.TenantID), IdentityID: string(execution.Identity.ID), IdentityEpoch: execution.Identity.Epoch, PlanDigest: execution.Lineage.PlanDigest, InstanceID: string(execution.Instance.ID), InstanceEpoch: execution.Instance.Epoch, RunID: string(operation.RunID), Session: toolcontract.ObjectRef{ID: "session-connect", Revision: 1, Digest: testDigestV1("session")}, Server: serverRef, Epoch: 1})
	if err != nil {
		t.Fatal(err)
	}
	intent, err := toolcontract.SealMCPConnectIntentV1(toolcontract.MCPConnectIntentV1{Ref: toolcontract.ObjectRef{Revision: 1}, Owner: runtimeports.EffectOwnerRefV2{Role: runtimeports.OwnerSettlement, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest}, Coordinate: coordinate, Server: serverRef, TransportConfig: config.Ref, Operation: operation, OperationDigest: operationDigest, EffectID: attempt.EffectID, EffectRevision: 1, EffectKind: toolcontract.MCPConnectEffectKindV1, PolicyProfile: toolcontract.MCPConnectPolicyProfileV1, IntentDigest: attempt.IntentDigest, Attempt: attempt, Provider: provider, ProviderTransport: transport, NetworkScopeDigest: config.NetworkScopeDigest, SandboxRequirementDigest: config.SandboxRequirementDigest, CreatedUnixNano: now.Add(-time.Second).UnixNano(), NotAfterUnixNano: now.Add(10 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return config, intent, server
}

func newMCPConnectAuthorizationV1(t *testing.T, now time.Time, intent toolcontract.MCPConnectIntentV1, config toolcontract.MCPTransportConfigV1) runtimeports.ControlledMCPConnectPhysicalAuthorizationV1 {
	t.Helper()
	declared := runtimeports.ExecutionDelegationRefV2{ID: intent.Attempt.Delegation.ID, Revision: intent.Attempt.Delegation.Revision - 1, Digest: testDigestV1("declared-delegation")}
	preparedID, err := runtimeports.DerivePreparedProviderAttemptIDV2(declared.ID, intent.Attempt.PermitID, intent.Attempt.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := runtimeports.SealPreparedProviderAttemptRefV2(runtimeports.PreparedProviderAttemptRefV2{ID: preparedID, Revision: 1, DeclaredDelegation: declared, OperationDigest: intent.OperationDigest, IntentID: intent.EffectID, IntentRevision: intent.Attempt.IntentRevision, IntentDigest: intent.IntentDigest, PermitID: intent.Attempt.PermitID, PermitRevision: intent.Attempt.PermitRevision, PermitDigest: intent.Attempt.PermitDigest, AttemptID: intent.Attempt.AttemptID, Provider: intent.Provider, PayloadSchema: runtimeports.SchemaRefV2{Namespace: "praxis.mcp", Name: "connect-intent", Version: "1.0.0", MediaType: "application/json", ContentDigest: intent.Ref.Digest}, PayloadDigest: intent.Ref.Digest, PayloadRevision: 1, PreparedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	enforcement := runtimeports.OperationDispatchEnforcementPhaseRefV4{OperationDigest: intent.OperationDigest, EffectID: intent.EffectID, PermitID: intent.Attempt.PermitID, PermitFactRevision: intent.Attempt.PermitRevision, PermitDigest: intent.Attempt.PermitDigest, AdmissionDigest: testDigestV1("admission"), ReviewAuthorization: runtimeports.OperationReviewAuthorizationRefV4{ID: "review-connect", Revision: 1, Digest: testDigestV1("review")}, AttemptID: intent.Attempt.AttemptID, SandboxAttempt: runtimeports.OperationDispatchSandboxFactRefV4{ID: intent.Attempt.AttemptID, Revision: 1, Digest: testDigestV1("sandbox-attempt"), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()}, Phase: runtimeports.OperationDispatchEnforcementExecuteV4, ReceiptDigest: testDigestV1("execute-receipt"), JournalRevision: 2, ValidatedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano(), PrepareReceiptDigest: testDigestV1("prepare-receipt"), PreparedAttemptDigest: prepared.Digest}
	declaration := runtimeports.ControlledOperationProviderRouteDeclarationRefV2{RouteID: "mcp-connect-route", Revision: 1, PublisherComponentID: "praxis.harness/assembly", DeclarationDigest: testDigestV1("declaration")}
	conformance := runtimeports.ControlledOperationProviderRouteConformanceRefV2{ConformanceID: "mcp-connect-conformance", Revision: 1, DeclarationRef: declaration, ConformanceDigest: testDigestV1("conformance")}
	route, err := runtimeports.SealControlledMCPConnectRouteCurrentProjectionV1(runtimeports.ControlledMCPConnectRouteCurrentProjectionV1{Ref: runtimeports.ControlledMCPConnectRouteCurrentRefV1{Revision: 1, DeclarationRef: declaration, ConformanceRef: conformance}, Generation: runtimeports.GenerationArtifactRefV1{ID: "generation-connect", Revision: 1, Digest: testDigestV1("generation"), InputDigest: testDigestV1("generation-input"), ManifestDigest: testDigestV1("generation-manifest"), GraphDigest: testDigestV1("generation-graph"), CatalogDigest: testDigestV1("generation-catalog")}, Assembly: runtimeports.GenerationBindingAssociationRefV1{ID: "assembly-connect", Revision: 1, Digest: testDigestV1("assembly")}, HandoffID: "route-handoff", HandoffRevision: 1, HandoffDigest: testDigestV1("route-handoff"), BindingSetID: intent.Provider.BindingSetID, BindingSetRevision: intent.Provider.BindingSetRevision, BindingSetDigest: testDigestV1("binding"), BindingSetSemanticDigest: testDigestV1("binding-semantic"), BindingSetCurrentnessDigest: testDigestV1("binding-current"), ActiveRouteID: "active-route-connect", ActiveRouteRevision: 1, ActiveRouteDigest: testDigestV1("active-route"), ProviderTransport: intent.ProviderTransport, Provider: intent.Provider, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	association, err := runtimeports.SealPreparedDomainCommandAssociationCurrentProjectionV1(runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1{Operation: intent.Operation, OperationDigest: intent.OperationDigest, EffectID: intent.EffectID, EffectRevision: intent.Attempt.IntentRevision, IntentDigest: intent.IntentDigest, Prepared: prepared, Attempt: intent.Attempt, Provider: intent.Provider, PayloadSchema: prepared.PayloadSchema, PayloadDigest: prepared.PayloadDigest, PayloadRevision: prepared.PayloadRevision, DomainCommand: intent.RuntimeDomainCommandRefV1(), CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	record := runtimeports.OperationScopeEvidenceRecordRefV3{LedgerScopeDigest: testDigestV1("ledger"), Sequence: 1, RecordDigest: testDigestV1("record")}
	prepareConsumption := runtimeports.OperationScopeEvidenceConsumptionRefV3{ID: "prepare-consumption-connect", Revision: 1, Digest: testDigestV1("prepare-consumption"), Record: record}
	executeQualification := runtimeports.OperationScopeEvidenceQualificationRefV3{ID: "execute-qualification-connect", Revision: 1, Digest: testDigestV1("execute-qualification"), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()}
	executeHandoff, err := runtimeports.SealOperationScopeEvidenceProviderHandoffFactV3(runtimeports.OperationScopeEvidenceProviderHandoffFactV3{ID: "execute-handoff-connect", Revision: 1, Qualification: executeQualification, Phase: enforcement, CheckedUnixNano: now.Add(-time.Second).UnixNano(), NotAfterUnixNano: now.Add(10 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := runtimeports.SealControlledMCPConnectPhysicalAuthorizationV1(runtimeports.ControlledMCPConnectPhysicalAuthorizationV1{UnifiedNotAfterUnixNano: now.Add(8 * time.Second).UnixNano(), Route: route.Ref, ProviderTransport: intent.ProviderTransport, Provider: intent.Provider, Operation: intent.Operation, OperationDigest: intent.OperationDigest, OperationScopeDigest: intent.Operation.ExecutionScopeDigest, EffectID: intent.EffectID, EffectRevision: intent.Attempt.IntentRevision, EffectFactRevision: intent.EffectRevision, IntentDigest: intent.IntentDigest, Prepared: prepared, Attempt: intent.Attempt, ExecuteEnforcement: enforcement, PrepareConsumption: prepareConsumption, ExecuteHandoff: executeHandoff.RefV3(), SandboxProjectionDigest: testDigestV1("sandbox-projection"), CredentialFactsDigest: testDigestV1("credential-facts"), Association: association.Ref, DomainCommand: intent.RuntimeDomainCommandRefV1(), IssuedUnixNano: now.Add(-time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	_ = config
	return authorization
}

type fakeMCPConnectCredentialMaterializerV1 struct {
	material MCPConnectCredentialMaterialV1
}

func (f *fakeMCPConnectCredentialMaterializerV1) MaterializeCurrentMCPConnectCredentialsV1(context.Context, MCPConnectCredentialMaterialRequestV1) (MCPConnectCredentialMaterialV1, error) {
	return f.material, nil
}

type fakeMCPConnectDriverV1 struct {
	calls    atomic.Int64
	session  OfficialSDKConnectSessionV1
	response []byte
	err      error
	release  chan struct{}
}

type lostMCPConnectStoreV1 struct {
	delegate     *InMemoryMCPConnectPhysicalRepositoryV1
	loseBegin    bool
	loseComplete bool
}

func (s *lostMCPConnectStoreV1) beginMCPConnectV1(ctx context.Context, authorization runtimeports.ControlledMCPConnectPhysicalAuthorizationV1, intent toolcontract.MCPConnectIntentV1, config toolcontract.MCPTransportConfigV1, now time.Time) (MCPConnectPhysicalEntryV1, bool, error) {
	entry, created, err := s.delegate.beginMCPConnectV1(ctx, authorization, intent, config, now)
	if err == nil && s.loseBegin {
		return MCPConnectPhysicalEntryV1{}, false, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "simulated lost admission reply")
	}
	return entry, created, err
}

func (s *lostMCPConnectStoreV1) completeMCPConnectV1(ctx context.Context, stable core.Digest, receipt toolcontract.MCPConnectProtocolReceiptV1, session OfficialSDKConnectSessionV1, now time.Time) (MCPConnectPhysicalEntryV1, error) {
	entry, err := s.delegate.completeMCPConnectV1(ctx, stable, receipt, session, now)
	if err == nil && s.loseComplete {
		return MCPConnectPhysicalEntryV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "simulated lost Receipt reply")
	}
	return entry, err
}

func (s *lostMCPConnectStoreV1) markMCPConnectUnknownV1(stable core.Digest, reason core.Digest, session OfficialSDKConnectSessionV1, now time.Time) {
	s.delegate.markMCPConnectUnknownV1(stable, reason, session, now)
}

func (s *lostMCPConnectStoreV1) InspectMCPConnectPhysicalV1(ctx context.Context, stable core.Digest) (MCPConnectPhysicalEntryV1, error) {
	return s.delegate.InspectMCPConnectPhysicalV1(ctx, stable)
}

func (d *fakeMCPConnectDriverV1) Connect(context.Context, toolcontract.MCPTransportConfigV1, MCPConnectCredentialMaterialV1) (OfficialSDKConnectSessionV1, []byte, error) {
	d.calls.Add(1)
	if d.release != nil {
		<-d.release
	}
	if d.err != nil {
		return nil, nil, d.err
	}
	response := append([]byte(nil), d.response...)
	if response == nil {
		response, _ = json.Marshal(d.session.InitializeResult())
	}
	return d.session, response, nil
}

type fakeConnectSessionV1 struct{ initialize *officialmcp.InitializeResult }

func newFakeConnectSessionV1() *fakeConnectSessionV1 {
	return &fakeConnectSessionV1{initialize: &officialmcp.InitializeResult{ProtocolVersion: toolcontract.MCPStableProtocolVersion, Capabilities: &officialmcp.ServerCapabilities{}, ServerInfo: &officialmcp.Implementation{Name: "fake", Version: "1.0.0"}}}
}

func (s *fakeConnectSessionV1) InitializeResult() *officialmcp.InitializeResult { return s.initialize }
func (*fakeConnectSessionV1) ID() string                                        { return "provider-session-connect" }
func (*fakeConnectSessionV1) Close() error                                      { return nil }

func testDigestV1(label string) core.Digest { return core.DigestBytes([]byte(label)) }

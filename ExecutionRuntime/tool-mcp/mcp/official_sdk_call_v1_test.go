package mcp

import (
	"context"
	"errors"
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

func TestOfficialSDKPhysicalExecutorV1ObservedAndInspectOnlyRetry(t *testing.T) {
	fixture := newMCPCallFixtureV1(t)
	receipt, err := fixture.executor.ExecuteControlledOperationPhysicalV3(context.Background(), fixture.authorization)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Admitted || fixture.session.calls.Load() != 1 {
		t.Fatalf("admission=%+v calls=%d", receipt, fixture.session.calls.Load())
	}
	entry, err := fixture.executor.InspectMCPPhysicalExecutionV1(context.Background(), fixture.authorization.StableKeyDigest)
	if err != nil || entry.State != MCPPhysicalExecutionObservedV1 || entry.ProtocolReceipt == nil || entry.ProtocolReceipt.ToolError {
		t.Fatalf("entry=%+v err=%v", entry, err)
	}
	byAttempt, err := fixture.commands.InspectMCPExecutionCommandByAttemptV1(context.Background(), fixture.command.Attempt)
	if err != nil || byAttempt.Ref != fixture.command.Ref {
		t.Fatalf("command Attempt index=%+v err=%v", byAttempt.Ref, err)
	}
	byID, err := fixture.entries.InspectMCPProtocolReceiptByIDV1(context.Background(), entry.ProtocolReceipt.Ref.ID)
	if err != nil || byID.Ref != entry.ProtocolReceipt.Ref {
		t.Fatalf("receipt ID index=%+v err=%v", byID.Ref, err)
	}
	second, err := fixture.executor.ExecuteControlledOperationPhysicalV3(context.Background(), fixture.authorization)
	if err != nil || second != receipt || fixture.session.calls.Load() != 1 {
		t.Fatalf("same command redispatched: receipt=%+v err=%v calls=%d", second, err, fixture.session.calls.Load())
	}
}

func TestOfficialSDKPhysicalExecutorV1LostProviderReplyIsInspectOnly(t *testing.T) {
	fixture := newMCPCallFixtureV1(t)
	fixture.session.err = errors.New("lost provider reply")
	receipt, err := fixture.executor.ExecuteControlledOperationPhysicalV3(context.Background(), fixture.authorization)
	if err == nil || !core.HasCategory(err, core.ErrorIndeterminate) || !receipt.Admitted || fixture.session.calls.Load() != 1 {
		t.Fatalf("lost reply receipt=%+v err=%v calls=%d", receipt, err, fixture.session.calls.Load())
	}
	fixture.session.err = nil
	second, err := fixture.executor.ExecuteControlledOperationPhysicalV3(context.Background(), fixture.authorization)
	if err != nil || second != receipt || fixture.session.calls.Load() != 1 {
		t.Fatalf("unknown retry redispatched: receipt=%+v err=%v calls=%d", second, err, fixture.session.calls.Load())
	}
	entry, err := fixture.executor.InspectMCPPhysicalExecutionV1(context.Background(), fixture.authorization.StableKeyDigest)
	if err != nil || entry.State != MCPPhysicalExecutionUnknownV1 || entry.UnknownReasonDigest.Validate() != nil || entry.ProtocolReceipt != nil {
		t.Fatalf("unknown entry=%+v err=%v", entry, err)
	}
}

func TestOfficialSDKPhysicalExecutorV1ConcurrentSameCommandSingleEffect(t *testing.T) {
	fixture := newMCPCallFixtureV1(t)
	fixture.session.block = make(chan struct{})
	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := fixture.executor.ExecuteControlledOperationPhysicalV3(context.Background(), fixture.authorization)
			errs <- err
		}()
	}
	for fixture.session.calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	close(fixture.session.block)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if fixture.session.calls.Load() != 1 {
		t.Fatalf("same canonical command called Provider %d times", fixture.session.calls.Load())
	}
}

func TestOfficialSDKPhysicalExecutorV1FailsClosedBeforeProvider(t *testing.T) {
	for name, mutate := range map[string]func(*mcpCallFixtureV1){
		"association drift": func(f *mcpCallFixtureV1) {
			f.association.projection.DomainCommand.Digest = testkit.Digest("other-command")
		},
		"session provider drift": func(f *mcpCallFixtureV1) {
			f.sessions.bindings[f.command.Connection.ID] = func() OfficialSDKCallSessionBindingV1 {
				b := f.sessions.bindings[f.command.Connection.ID]
				b.Provider.ArtifactDigest = testkit.Digest("other-provider")
				return b
			}()
		},
		"domain command drift": func(f *mcpCallFixtureV1) {
			f.authorization.DomainCommand.Digest = testkit.Digest("other-command")
			f.authorization.AuthorizationDigest = ""
			f.authorization, _ = runtimeports.SealControlledOperationPhysicalExecutionAuthorizationV3(f.authorization)
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newMCPCallFixtureV1(t)
			mutate(fixture)
			if _, err := fixture.executor.ExecuteControlledOperationPhysicalV3(context.Background(), fixture.authorization); err == nil {
				t.Fatal("drift was accepted")
			}
			if fixture.session.calls.Load() != 0 {
				t.Fatalf("drift reached Provider calls=%d", fixture.session.calls.Load())
			}
		})
	}
}

func TestOfficialSDKPhysicalExecutorV1TTLAndClockAtActualPoint(t *testing.T) {
	fixture := newMCPCallFixtureV1(t)
	sequence := []time.Time{fixture.now, fixture.now, fixture.now, fixture.now, time.Unix(0, fixture.authorization.UnifiedNotAfterUnixNano)}
	var index atomic.Uint64
	fixture.executor.clock = func() time.Time {
		value := index.Add(1) - 1
		if int(value) >= len(sequence) {
			return sequence[len(sequence)-1]
		}
		return sequence[value]
	}
	if _, err := fixture.executor.ExecuteControlledOperationPhysicalV3(context.Background(), fixture.authorization); err == nil || !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("actual-point expiry error=%v", err)
	}
	if fixture.session.calls.Load() != 0 {
		t.Fatalf("expired actual point called Provider %d times", fixture.session.calls.Load())
	}

	rollback := newMCPCallFixtureV1(t)
	sequence = []time.Time{rollback.now, rollback.now.Add(-time.Nanosecond)}
	index.Store(0)
	rollback.executor.clock = func() time.Time { return sequence[min(int(index.Add(1)-1), len(sequence)-1)] }
	if _, err := rollback.executor.ExecuteControlledOperationPhysicalV3(context.Background(), rollback.authorization); err == nil || !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("clock rollback error=%v", err)
	}
	if rollback.session.calls.Load() != 0 {
		t.Fatal("clock rollback reached Provider")
	}
}

func TestOfficialSDKPhysicalExecutorV1NilAndCanceledBoundaries(t *testing.T) {
	fixture := newMCPCallFixtureV1(t)
	var nilCommands *InMemoryMCPExecutionCommandRepositoryV1
	if _, err := NewOfficialSDKPhysicalExecutorV1(nilCommands, fixture.association, fixture.sessions, fixture.entries, func() time.Time { return fixture.now }); err == nil {
		t.Fatal("typed-nil command reader was accepted")
	}
	if _, err := fixture.executor.ExecuteControlledOperationPhysicalV3(nil, fixture.authorization); err == nil {
		t.Fatal("nil context was accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fixture.executor.ExecuteControlledOperationPhysicalV3(ctx, fixture.authorization); err != context.Canceled {
		t.Fatalf("canceled context was not preserved before effect: %v", err)
	}
	if fixture.session.calls.Load() != 0 {
		t.Fatal("canceled context reached Provider")
	}
}

func TestConformanceOfficialSDKPhysicalExecutorV1OverInMemoryTransport(t *testing.T) {
	fixture := newMCPCallFixtureV1(t)
	ctx := context.Background()
	server := officialmcp.NewServer(&officialmcp.Implementation{Name: "praxis-call-conformance", Version: "1.0.0"}, nil)
	var handlerCalls atomic.Uint64
	server.AddTool(&officialmcp.Tool{Name: fixture.command.SnapshotTool.Name, InputSchema: map[string]any{"type": "object"}}, func(_ context.Context, request *officialmcp.CallToolRequest) (*officialmcp.CallToolResult, error) {
		handlerCalls.Add(1)
		if string(request.Params.Arguments) != string(fixture.command.Params.Inline) {
			return nil, errors.New("canonical arguments drifted")
		}
		return &officialmcp.CallToolResult{Content: []officialmcp.Content{&officialmcp.TextContent{Text: "official-ok"}}, StructuredContent: map[string]any{"ok": true}}, nil
	})
	serverTransport, clientTransport := officialmcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := officialmcp.NewClient(&officialmcp.Implementation{Name: "praxis-tool-mcp", Version: "1.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	sessions, _ := NewInMemoryOfficialSDKCallSessionRepositoryV1(func() time.Time { return fixture.now })
	if _, err = sessions.BindInitializedOfficialSDKSessionV1(ctx, OfficialSDKCallSessionBindingV1{Connection: fixture.command.Connection, Snapshot: fixture.command.Snapshot, ProviderTransport: fixture.authorization.ProviderTransport, Provider: fixture.authorization.Provider, CheckedUnixNano: fixture.now.UnixNano(), ExpiresUnixNano: fixture.command.NotAfterUnixNano, Session: clientSession}); err != nil {
		t.Fatal(err)
	}
	fixture.executor.sessions = sessions
	if _, err = fixture.executor.ExecuteControlledOperationPhysicalV3(ctx, fixture.authorization); err != nil {
		t.Fatal(err)
	}
	entry, err := fixture.executor.InspectMCPPhysicalExecutionV1(ctx, fixture.authorization.StableKeyDigest)
	if err != nil || handlerCalls.Load() != 1 || entry.ProtocolReceipt == nil || entry.State != MCPPhysicalExecutionObservedV1 {
		t.Fatalf("official SDK execution calls=%d entry=%+v err=%v", handlerCalls.Load(), entry, err)
	}
}

func TestConformanceOfficialSDKPhysicalExecutorV1CanceledAfterAdmissionIsUnknownInspectOnly(t *testing.T) {
	fixture := newMCPCallFixtureV1(t)
	serverTransport, clientTransport := officialmcp.NewInMemoryTransports()
	server := officialmcp.NewServer(&officialmcp.Implementation{Name: "praxis-call-cancel-conformance", Version: "1.0.0"}, nil)
	started := make(chan struct{})
	var calls atomic.Uint64
	server.AddTool(&officialmcp.Tool{Name: fixture.command.SnapshotTool.Name, InputSchema: map[string]any{"type": "object"}}, func(ctx context.Context, _ *officialmcp.CallToolRequest) (*officialmcp.CallToolResult, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-ctx.Done()
		return nil, ctx.Err()
	})
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := officialmcp.NewClient(&officialmcp.Implementation{Name: "praxis-tool-mcp", Version: "1.0.0"}, nil)
	clientSession, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	sessions, err := NewInMemoryOfficialSDKCallSessionRepositoryV1(func() time.Time { return fixture.now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = sessions.BindInitializedOfficialSDKSessionV1(context.Background(), OfficialSDKCallSessionBindingV1{
		Connection: fixture.command.Connection, Snapshot: fixture.command.Snapshot,
		ProviderTransport: fixture.authorization.ProviderTransport, Provider: fixture.authorization.Provider,
		CheckedUnixNano: fixture.now.UnixNano(), ExpiresUnixNano: fixture.command.NotAfterUnixNano, Session: clientSession,
	}); err != nil {
		t.Fatal(err)
	}
	fixture.executor.sessions = sessions
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, executeErr := fixture.executor.ExecuteControlledOperationPhysicalV3(ctx, fixture.authorization)
		result <- executeErr
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("official MCP Tool handler did not start")
	}
	cancel()
	select {
	case executeErr := <-result:
		if executeErr == nil || !core.HasCategory(executeErr, core.ErrorIndeterminate) {
			t.Fatalf("canceled admitted call error=%v", executeErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled official MCP call did not return")
	}
	entry, err := fixture.executor.InspectMCPPhysicalExecutionV1(context.Background(), fixture.authorization.StableKeyDigest)
	if err != nil || entry.State != MCPPhysicalExecutionUnknownV1 || entry.ProtocolReceipt != nil || calls.Load() != 1 {
		t.Fatalf("canceled entry=%+v calls=%d err=%v", entry, calls.Load(), err)
	}
	if _, err = fixture.executor.ExecuteControlledOperationPhysicalV3(context.Background(), fixture.authorization); err != nil {
		t.Fatalf("inspect-only redelivery failed: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("canceled unknown call was redispatched %d times", calls.Load())
	}
}

func TestMCPExecutionCommandV1RejectsSplicedSnapshotAndPayload(t *testing.T) {
	fixture := newMCPCallFixtureV1(t)
	for name, mutate := range map[string]func(*toolcontract.MCPExecutionCommandFactV1){
		"snapshot tool": func(c *toolcontract.MCPExecutionCommandFactV1) {
			c.SnapshotTool.ObjectDigest = testkit.Digest("other-snapshot-tool")
		},
		"connection epoch":          func(c *toolcontract.MCPExecutionCommandFactV1) { c.Connection.Epoch++ },
		"same digest other payload": func(c *toolcontract.MCPExecutionCommandFactV1) { c.Params.Inline = []byte(`{"value":2}`) },
		"tool mechanism":            func(c *toolcontract.MCPExecutionCommandFactV1) { c.Tool.Mechanism = toolcontract.MechanismLocal },
	} {
		t.Run(name, func(t *testing.T) {
			changed := toolcontract.CloneMCPExecutionCommandFactV1(fixture.command)
			mutate(&changed)
			changed.Ref.Digest = ""
			if _, err := toolcontract.SealMCPExecutionCommandFactV1(changed); err == nil {
				t.Fatal("spliced MCP execution command was accepted")
			}
		})
	}
}

func TestMCPExecutionCommandRepositoryV1CreateOnceDeepCloneAndConcurrent(t *testing.T) {
	fixture := newMCPCallFixtureV1(t)
	repository, err := NewInMemoryMCPExecutionCommandRepositoryV1(func() time.Time { return fixture.now })
	if err != nil {
		t.Fatal(err)
	}
	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	refs := make(chan toolcontract.MCPExecutionCommandRefV1, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			winner, createErr := repository.CreateMCPExecutionCommandV1(context.Background(), fixture.command)
			if createErr == nil {
				refs <- winner.Ref
			}
			errs <- createErr
		}()
	}
	wg.Wait()
	close(errs)
	close(refs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for ref := range refs {
		if ref != fixture.command.Ref {
			t.Fatalf("create-once returned another Ref: %+v", ref)
		}
	}
	first, err := repository.InspectMCPExecutionCommandV1(context.Background(), fixture.command.Ref)
	if err != nil {
		t.Fatal(err)
	}
	first.Params.Inline[0] ^= 0xff
	second, err := repository.InspectMCPExecutionCommandV1(context.Background(), fixture.command.Ref)
	if err != nil || string(second.Params.Inline) != string(fixture.command.Params.Inline) {
		t.Fatalf("repository leaked mutable Params: %q err=%v", second.Params.Inline, err)
	}
	changed := toolcontract.CloneMCPExecutionCommandFactV1(fixture.command)
	changed.CreatedUnixNano--
	changed.Ref.Digest = ""
	changed, err = toolcontract.SealMCPExecutionCommandFactV1(changed)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Ref.ID != fixture.command.Ref.ID || changed.Ref.Digest == fixture.command.Ref.Digest {
		t.Fatal("test does not exercise same-ID different-content conflict")
	}
	if _, err = repository.CreateMCPExecutionCommandV1(context.Background(), changed); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same ID different content error=%v", err)
	}
}

func TestOfficialSDKCallSessionRepositoryV1RejectsAliasAndTypedNil(t *testing.T) {
	fixture := newMCPCallFixtureV1(t)
	var nilSession *fakeOfficialCallSessionV1
	binding := OfficialSDKCallSessionBindingV1{Connection: fixture.command.Connection, Snapshot: fixture.command.Snapshot, ProviderTransport: fixture.authorization.ProviderTransport, Provider: fixture.authorization.Provider, CheckedUnixNano: fixture.now.UnixNano(), ExpiresUnixNano: fixture.command.NotAfterUnixNano, Session: nilSession}
	repository, _ := NewInMemoryOfficialSDKCallSessionRepositoryV1(func() time.Time { return fixture.now })
	if _, err := repository.BindInitializedOfficialSDKSessionV1(context.Background(), binding); err == nil {
		t.Fatal("typed-nil official Session was accepted")
	}
	binding.Session = fixture.session
	binding.ProviderTransport = binding.Provider
	if _, err := repository.BindInitializedOfficialSDKSessionV1(context.Background(), binding); err == nil {
		t.Fatal("Provider/Transport alias was accepted")
	}
}

type fakeOfficialCallSessionV1 struct {
	initialize *officialmcp.InitializeResult
	id         string
	result     *officialmcp.CallToolResult
	err        error
	block      chan struct{}
	calls      atomic.Uint64
}

func (s *fakeOfficialCallSessionV1) InitializeResult() *officialmcp.InitializeResult {
	return s.initialize
}
func (s *fakeOfficialCallSessionV1) ID() string { return s.id }
func (s *fakeOfficialCallSessionV1) CallTool(context.Context, *officialmcp.CallToolParams) (*officialmcp.CallToolResult, error) {
	s.calls.Add(1)
	if s.block != nil {
		<-s.block
	}
	return s.result, s.err
}

type fixedAssociationReaderV1 struct {
	projection runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1
}

func (r *fixedAssociationReaderV1) InspectCurrentPreparedDomainCommandAssociationV1(_ context.Context, _ runtimeports.PreparedDomainCommandAssociationRefV1) (runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1, error) {
	return r.projection, nil
}

type mcpCallFixtureV1 struct {
	now           time.Time
	command       toolcontract.MCPExecutionCommandFactV1
	authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3
	association   *fixedAssociationReaderV1
	sessions      *InMemoryOfficialSDKCallSessionRepositoryV1
	entries       *InMemoryMCPPhysicalExecutionStoreV1
	commands      *InMemoryMCPExecutionCommandRepositoryV1
	session       *fakeOfficialCallSessionV1
	executor      *OfficialSDKPhysicalExecutorV1
}

func newMCPCallFixtureV1(t *testing.T) *mcpCallFixtureV1 {
	t.Helper()
	now := testkit.FixedTime.Add(2 * time.Second)
	provider := testkit.ProviderBinding()
	transport := provider
	transport.ComponentID, transport.ManifestDigest, transport.ArtifactDigest, transport.Capability = "tool-mcp/transport", testkit.Digest("transport-manifest"), testkit.Digest("transport-artifact"), runtimeports.ControlledOperationProviderTransportCapabilityV2
	boundary := testkit.BoundaryFixture(now)
	projection := testkit.ModelProjection(1)
	source, err := toolcontract.SealModelSourceCandidateHistoricalRefV1(projection)
	if err != nil {
		t.Fatal(err)
	}
	capability := testkit.Capability()
	tool := testkit.Tool()
	tool.Mechanism, tool.ArtifactDigest, tool.Digest = toolcontract.MechanismMCP, provider.ArtifactDigest, ""
	tool, err = toolcontract.SealTool(tool)
	if err != nil {
		t.Fatal(err)
	}
	capRef := toolcontract.ObjectRef{ID: string(capability.ID), Revision: capability.Revision, Digest: capability.Digest}
	toolRef := toolcontract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest}
	surfaceRef := toolcontract.ToolSurfaceManifestCurrentRefV1{ContractVersion: toolcontract.ToolSurfaceManifestCurrentContractVersionV1, ID: "surface-mcp-call-v1", Revision: 1, Digest: testkit.Digest("surface-mcp-call-v1")}
	capCurrentID, _ := toolcontract.DeriveToolRegistryObjectCurrentIDV1(toolcontract.ToolRegistryCapabilityCurrentKindV1, capRef, capability.Owner)
	toolCurrentID, _ := toolcontract.DeriveToolRegistryObjectCurrentIDV1(toolcontract.ToolRegistryDescriptorCurrentKindV1, toolRef, tool.Owner)
	capCurrent := toolcontract.ToolRegistryObjectCurrentRefV1{Kind: toolcontract.ToolRegistryCapabilityCurrentKindV1, ID: capCurrentID, Revision: 1, Digest: testkit.Digest("cap-current")}
	toolCurrent := toolcontract.ToolRegistryObjectCurrentRefV1{Kind: toolcontract.ToolRegistryDescriptorCurrentKindV1, ID: toolCurrentID, Revision: 1, Digest: testkit.Digest("tool-current")}
	schemaCurrent, err := toolcontract.SealToolInputSchemaCurrentRefV1(toolcontract.ToolInputSchemaCurrentRefV1{InputSchema: tool.InputSchema, Authority: toolCurrent, RegistryOwner: tool.Owner, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(12 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	inputRef := toolcontract.ToolInputContractCurrentRefV1{ID: "input-contract-mcp-call-v1", Revision: 1, Digest: testkit.Digest("input-contract")}
	arguments := append([]byte(nil), projection.Observation.Calls[0].CanonicalArguments...)
	payload := runtimeports.OpaquePayloadV2{Schema: tool.InputSchema, ContentDigest: core.DigestBytes(arguments), Length: uint64(len(arguments)), Inline: arguments, LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: toolcontract.ToolInputLimitPolicyV1, Digest: testkit.Digest("input-limit")}}
	candidate, err := toolcontract.SealActionCandidateV3(toolcontract.ActionCandidateV3{
		TenantID: "tenant-v2", RunID: "run-v2", SessionID: "session-v2", TurnID: "1", PendingAction: toolcontract.PendingActionExactRefV2{ID: "pending-mcp-call-v1", Revision: 1, RequestDigest: testkit.Digest("pending")}, SourceCandidate: source,
		Surface: toolcontract.ObjectRef{ID: surfaceRef.ID, Revision: surfaceRef.Revision, Digest: surfaceRef.Digest}, Capability: capRef, Tool: toolRef, InputSchema: tool.InputSchema, Payload: payload, PayloadRevision: 1, LimitPolicy: payload.LimitPolicy,
		InputContractCurrentRef: inputRef, SurfaceCurrent: surfaceRef, CapabilityCurrent: capCurrent, ToolCurrent: toolCurrent, InputSchemaCurrent: schemaCurrent,
		OperationScopeDigest: boundary.Operation.ExecutionScopeDigest, EffectKind: runtimeports.OperationScopeEvidenceActionEffectKindV3, ExpectedOwner: testkit.SettlementOwner(), ConflictDomain: "tenant/tenant-v2/tool/example", IdempotencyKey: "mcp-call-v1", CreatedUnixNano: now.UnixNano(), RequestedExpiresUnixNano: now.Add(10 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	server := testkit.MCPServer()
	connectionID, _ := toolcontract.StableID("mcp-connection", server.ID, "tenant-v2", "session-v2")
	connection, err := toolcontract.SealMCPConnection(toolcontract.MCPConnectionRef{ID: connectionID, Revision: 1, Epoch: 1, Server: toolcontract.ObjectRef{ID: server.ID, Revision: server.Revision, Digest: server.Digest}, TenantID: "tenant-v2", IdentityID: "identity-v2", PlanDigest: testkit.Digest("plan-v2"), InstanceID: "instance-v2", RunID: "run-v2", NegotiatedProtocol: toolcontract.MCPStableProtocolVersion, SessionID: "mcp-session-call-v1", CreatedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	snapshotTool := toolcontract.MCPToolObservationV2{Name: source.CallName, ObjectDigest: testkit.Digest("snapshot-tool"), DescriptionDigest: testkit.Digest("description"), InputSchemaDigest: tool.InputSchema.ContentDigest, OutputSchemaDigest: tool.OutputSchema.ContentDigest, AnnotationsDigest: testkit.Digest("annotations"), MetaDigest: testkit.Digest("meta")}
	snapshot, err := toolcontract.SealMCPCapabilitySnapshotV2(toolcontract.MCPCapabilitySnapshotV2{Revision: 1, Server: connection.Server, Connection: toolcontract.ObjectRef{ID: connection.ID, Revision: connection.Revision, Digest: connection.Digest}, ConnectionEpoch: connection.Epoch, ProtocolVersion: connection.NegotiatedProtocol, ServerInfoDigest: testkit.Digest("server-info"), ServerCapabilitiesDigest: testkit.Digest("server-capabilities"), InstructionsDigest: testkit.Digest("instructions"), Tools: []toolcontract.MCPToolObservationV2{snapshotTool}, SourceDigest: testkit.Digest("source"), Conformance: "mcp/official-go-sdk-v1", CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(15 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	prepared := testkit.PreparedAttemptFor(now, boundary, provider, payload.Schema, payload.ContentDigest, 1)
	bindingRef := toolcontract.SingleCallToolActionBindingCurrentRefV2{ID: "binding-mcp-call-v1", Revision: 1, Digest: testkit.Digest("binding")}
	command, err := toolcontract.SealMCPExecutionCommandFactV1(toolcontract.MCPExecutionCommandFactV1{Owner: testkit.SettlementOwner(), BindingCurrent: bindingRef, Candidate: candidate, InputContractCurrent: inputRef, Capability: capability, Tool: tool, Server: server, Connection: connection, Snapshot: snapshot, SnapshotTool: snapshotTool, Method: toolcontract.MCPToolsCallMethodV1, Params: payload, ParamsRevision: 1, Operation: boundary.Operation, OperationDigest: boundary.Attempt.OperationDigest, EffectID: boundary.Attempt.EffectID, EffectRevision: boundary.Attempt.IntentRevision, IntentDigest: boundary.Attempt.IntentDigest, Prepared: prepared, Attempt: boundary.Attempt, Provider: provider, CreatedUnixNano: now.UnixNano(), NotAfterUnixNano: now.Add(10 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	associationProjection, err := runtimeports.SealPreparedDomainCommandAssociationCurrentProjectionV1(runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1{Operation: boundary.Operation, OperationDigest: boundary.Attempt.OperationDigest, EffectID: boundary.Attempt.EffectID, EffectRevision: boundary.Attempt.IntentRevision, IntentDigest: boundary.Attempt.IntentDigest, Prepared: prepared, Attempt: boundary.Attempt, Provider: provider, PayloadSchema: payload.Schema, PayloadDigest: payload.ContentDigest, PayloadRevision: 1, DomainCommand: command.RuntimeDomainCommandRefV1(), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := runtimeports.SealControlledOperationPhysicalExecutionAuthorizationV3(runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3{UnifiedNotAfterUnixNano: now.Add(10 * time.Second).UnixNano(), ProviderTransport: transport, Provider: provider, Operation: boundary.Operation, OperationDigest: boundary.Attempt.OperationDigest, OperationScopeDigest: boundary.Operation.ExecutionScopeDigest, EffectKind: runtimeports.OperationScopeEvidenceActionEffectKindV3, Prepared: prepared, Attempt: boundary.Attempt, ExecuteEnforcement: boundary.Enforcement, ExecuteEvidenceHandoff: boundary.Handoff.RefV3(), Boundary: runtimeports.OperationProviderBoundaryRefV1{ID: "boundary-mcp-call-v1", Revision: 1, Digest: testkit.Digest("boundary")}, Association: associationProjection.Ref, DomainCommand: command.RuntimeDomainCommandRefV1()})
	if err != nil {
		t.Fatal(err)
	}
	commandRepository, _ := NewInMemoryMCPExecutionCommandRepositoryV1(func() time.Time { return now })
	if _, err = commandRepository.CreateMCPExecutionCommandV1(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	session := &fakeOfficialCallSessionV1{initialize: &officialmcp.InitializeResult{ProtocolVersion: toolcontract.MCPStableProtocolVersion, ServerInfo: &officialmcp.Implementation{Name: "test", Version: "1.0.0"}, Capabilities: &officialmcp.ServerCapabilities{Tools: &officialmcp.ToolCapabilities{}}}, id: connection.SessionID, result: &officialmcp.CallToolResult{Content: []officialmcp.Content{&officialmcp.TextContent{Text: "ok"}}}}
	sessions, _ := NewInMemoryOfficialSDKCallSessionRepositoryV1(func() time.Time { return now })
	if _, err = sessions.BindInitializedOfficialSDKSessionV1(context.Background(), OfficialSDKCallSessionBindingV1{Connection: connection, Snapshot: snapshot, ProviderTransport: transport, Provider: provider, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano(), Session: session}); err != nil {
		t.Fatal(err)
	}
	association := &fixedAssociationReaderV1{projection: associationProjection}
	entries := NewInMemoryMCPPhysicalExecutionStoreV1()
	executor, err := NewOfficialSDKPhysicalExecutorV1(commandRepository, association, sessions, entries, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return &mcpCallFixtureV1{now: now, command: command, authorization: authorization, association: association, sessions: sessions, entries: entries, commands: commandRepository, session: session, executor: executor}
}

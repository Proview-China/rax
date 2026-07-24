package mcp

import (
	"context"
	"sync"
	"testing"
	"time"

	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	officialmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeOfficialSDKSessionV1 struct {
	mu            sync.Mutex
	initialize    *officialmcp.InitializeResult
	sessionID     string
	tools         map[string]*officialmcp.ListToolsResult
	resources     map[string]*officialmcp.ListResourcesResult
	prompts       map[string]*officialmcp.ListPromptsResult
	toolCalls     int
	resourceCalls int
	promptCalls   int
	err           error
}

func (s *fakeOfficialSDKSessionV1) InitializeResult() *officialmcp.InitializeResult {
	return s.initialize
}
func (s *fakeOfficialSDKSessionV1) ID() string   { return s.sessionID }
func (s *fakeOfficialSDKSessionV1) Close() error { return nil }

func (s *fakeOfficialSDKSessionV1) ListTools(_ context.Context, p *officialmcp.ListToolsParams) (*officialmcp.ListToolsResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.toolCalls++
	if s.err != nil {
		return nil, s.err
	}
	return s.tools[p.Cursor], nil
}

func (s *fakeOfficialSDKSessionV1) ListResources(_ context.Context, p *officialmcp.ListResourcesParams) (*officialmcp.ListResourcesResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resourceCalls++
	if s.err != nil {
		return nil, s.err
	}
	return s.resources[p.Cursor], nil
}

func (s *fakeOfficialSDKSessionV1) ListPrompts(_ context.Context, p *officialmcp.ListPromptsParams) (*officialmcp.ListPromptsResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.promptCalls++
	if s.err != nil {
		return nil, s.err
	}
	return s.prompts[p.Cursor], nil
}

func completeFakeOfficialSDKSessionV1() *fakeOfficialSDKSessionV1 {
	return &fakeOfficialSDKSessionV1{
		initialize: &officialmcp.InitializeResult{
			ProtocolVersion: toolcontract.MCPStableProtocolVersion,
			ServerInfo:      &officialmcp.Implementation{Name: "official-test-server", Version: "1.0.0"},
			Capabilities: &officialmcp.ServerCapabilities{
				Tools: &officialmcp.ToolCapabilities{}, Resources: &officialmcp.ResourceCapabilities{}, Prompts: &officialmcp.PromptCapabilities{},
			},
			Instructions: "Use exact MCP objects.",
		},
		sessionID: "mcp-session-1",
		tools: map[string]*officialmcp.ListToolsResult{
			"":        {Tools: []*officialmcp.Tool{{Name: "zeta", Description: "zeta", InputSchema: map[string]any{"type": "object"}}}, NextCursor: "tools-2"},
			"tools-2": {Tools: []*officialmcp.Tool{{Name: "alpha", Description: "alpha", InputSchema: map[string]any{"type": "object"}}}},
		},
		resources: map[string]*officialmcp.ListResourcesResult{
			"": {Resources: []*officialmcp.Resource{{URI: "file:///zeta", Name: "zeta"}, {URI: "file:///alpha", Name: "alpha"}}},
		},
		prompts: map[string]*officialmcp.ListPromptsResult{
			"": {Prompts: []*officialmcp.Prompt{{Name: "zeta"}, {Name: "alpha"}}},
		},
	}
}

func officialSDKDiscoveryRequestV1() OfficialSDKDiscoveryRequestV1 {
	server, connection := testkit.MCPServer(), testkit.MCPConnection()
	return OfficialSDKDiscoveryRequestV1{
		Server:                   toolcontract.ObjectRef{ID: server.ID, Revision: server.Revision, Digest: server.Digest},
		Connection:               connection,
		SnapshotRevision:         1,
		Conformance:              "mcp/official-go-sdk-v1",
		RequestedExpiresUnixNano: testkit.FixedTime.Add(time.Minute).UnixNano(),
	}
}

func TestOfficialSDKDiscoveryV1PaginatesSortsAndSeals(t *testing.T) {
	session := completeFakeOfficialSDKSessionV1()
	discovery, err := newOfficialSDKDiscoveryV1(session, func() time.Time { return testkit.FixedTime }, DefaultOfficialSDKDiscoveryLimitsV1())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := discovery.DiscoverV1(context.Background(), officialSDKDiscoveryRequestV1())
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshot.ValidateCurrent(testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Tools) != 2 || snapshot.Tools[0].Name != "alpha" || snapshot.Tools[1].Name != "zeta" || len(snapshot.Resources) != 2 || snapshot.Resources[0].URI != "file:///alpha" || len(snapshot.Prompts) != 2 || snapshot.Prompts[0].Name != "alpha" {
		t.Fatalf("official discovery was not normalized: %+v", snapshot)
	}
	if session.toolCalls != 2 || session.resourceCalls != 1 || session.promptCalls != 1 {
		t.Fatalf("unexpected official list calls: tools=%d resources=%d prompts=%d", session.toolCalls, session.resourceCalls, session.promptCalls)
	}
}

func TestOfficialSDKDiscoveryV1SourceDigestIgnoresProviderOrder(t *testing.T) {
	first := completeFakeOfficialSDKSessionV1()
	second := completeFakeOfficialSDKSessionV1()
	second.tools = map[string]*officialmcp.ListToolsResult{"": {Tools: []*officialmcp.Tool{{Name: "alpha", Description: "alpha", InputSchema: map[string]any{"type": "object"}}, {Name: "zeta", Description: "zeta", InputSchema: map[string]any{"type": "object"}}}}}
	for _, session := range []*fakeOfficialSDKSessionV1{first, second} {
		session.initialize.Capabilities.Resources = nil
		session.initialize.Capabilities.Prompts = nil
	}
	d1, _ := newOfficialSDKDiscoveryV1(first, func() time.Time { return testkit.FixedTime }, DefaultOfficialSDKDiscoveryLimitsV1())
	d2, _ := newOfficialSDKDiscoveryV1(second, func() time.Time { return testkit.FixedTime }, DefaultOfficialSDKDiscoveryLimitsV1())
	s1, err := d1.DiscoverV1(context.Background(), officialSDKDiscoveryRequestV1())
	if err != nil {
		t.Fatal(err)
	}
	s2, err := d2.DiscoverV1(context.Background(), officialSDKDiscoveryRequestV1())
	if err != nil {
		t.Fatal(err)
	}
	if s1.SourceDigest != s2.SourceDigest || s1.Digest != s2.Digest {
		t.Fatalf("provider order changed canonical snapshot: %s/%s %s/%s", s1.SourceDigest, s1.Digest, s2.SourceDigest, s2.Digest)
	}
}

func TestOfficialSDKDiscoveryV1FailClosedBoundaries(t *testing.T) {
	var typedNil *fakeOfficialSDKSessionV1
	if _, err := newOfficialSDKDiscoveryV1(typedNil, func() time.Time { return testkit.FixedTime }, DefaultOfficialSDKDiscoveryLimitsV1()); err == nil {
		t.Fatal("typed-nil official session was accepted")
	}

	tests := []struct {
		name   string
		mutate func(*fakeOfficialSDKSessionV1, *OfficialSDKDiscoveryRequestV1)
		clock  func() time.Time
	}{
		{name: "protocol drift", mutate: func(s *fakeOfficialSDKSessionV1, _ *OfficialSDKDiscoveryRequestV1) {
			s.initialize.ProtocolVersion = "2025-06-18"
		}},
		{name: "session drift", mutate: func(s *fakeOfficialSDKSessionV1, _ *OfficialSDKDiscoveryRequestV1) { s.sessionID = "another-session" }},
		{name: "duplicate tool", mutate: func(s *fakeOfficialSDKSessionV1, _ *OfficialSDKDiscoveryRequestV1) {
			s.initialize.Capabilities.Resources, s.initialize.Capabilities.Prompts = nil, nil
			s.tools = map[string]*officialmcp.ListToolsResult{"": {Tools: []*officialmcp.Tool{{Name: "same", InputSchema: map[string]any{"type": "object"}}, {Name: "same", InputSchema: map[string]any{"type": "object"}}}}}
		}},
		{name: "cursor cycle", mutate: func(s *fakeOfficialSDKSessionV1, _ *OfficialSDKDiscoveryRequestV1) {
			s.initialize.Capabilities.Resources, s.initialize.Capabilities.Prompts = nil, nil
			s.tools = map[string]*officialmcp.ListToolsResult{"": {NextCursor: "again"}, "again": {NextCursor: "again"}}
		}},
		{name: "expired request", mutate: func(_ *fakeOfficialSDKSessionV1, r *OfficialSDKDiscoveryRequestV1) {
			r.RequestedExpiresUnixNano = testkit.FixedTime.UnixNano()
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			session, request := completeFakeOfficialSDKSessionV1(), officialSDKDiscoveryRequestV1()
			tc.mutate(session, &request)
			clock := tc.clock
			if clock == nil {
				clock = func() time.Time { return testkit.FixedTime }
			}
			discovery, err := newOfficialSDKDiscoveryV1(session, clock, DefaultOfficialSDKDiscoveryLimitsV1())
			if err != nil {
				t.Fatal(err)
			}
			if _, err = discovery.DiscoverV1(context.Background(), request); err == nil {
				t.Fatal("fail-closed discovery boundary was accepted")
			}
		})
	}

	discovery, _ := newOfficialSDKDiscoveryV1(completeFakeOfficialSDKSessionV1(), func() time.Time { return testkit.FixedTime }, DefaultOfficialSDKDiscoveryLimitsV1())
	if _, err := discovery.DiscoverV1(nil, officialSDKDiscoveryRequestV1()); err == nil {
		t.Fatal("nil context was accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := discovery.DiscoverV1(ctx, officialSDKDiscoveryRequestV1()); err != context.Canceled {
		t.Fatalf("canceled context was not preserved: %v", err)
	}
}

func TestOfficialSDKDiscoveryV1RejectsTTLAndClockCrossing(t *testing.T) {
	tests := []struct {
		name  string
		times []time.Time
	}{
		{name: "ttl crossing", times: []time.Time{testkit.FixedTime, testkit.FixedTime.Add(time.Minute)}},
		{name: "clock rollback", times: []time.Time{testkit.FixedTime, testkit.FixedTime.Add(-time.Nanosecond)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			session := completeFakeOfficialSDKSessionV1()
			session.initialize.Capabilities.Resources, session.initialize.Capabilities.Prompts = nil, nil
			var mu sync.Mutex
			index := 0
			clock := func() time.Time {
				mu.Lock()
				defer mu.Unlock()
				if index >= len(tc.times) {
					return tc.times[len(tc.times)-1]
				}
				value := tc.times[index]
				index++
				return value
			}
			discovery, _ := newOfficialSDKDiscoveryV1(session, clock, DefaultOfficialSDKDiscoveryLimitsV1())
			if _, err := discovery.DiscoverV1(context.Background(), officialSDKDiscoveryRequestV1()); err == nil {
				t.Fatal("discovery crossed current window")
			}
		})
	}
}

func TestOfficialSDKDiscoveryV1RejectsPageAndObjectLimitOverflow(t *testing.T) {
	for _, limits := range []OfficialSDKDiscoveryLimitsV1{
		{MaxPages: 1, MaxTools: 2, MaxResources: 1, MaxPrompts: 1},
		{MaxPages: 2, MaxTools: 1, MaxResources: 1, MaxPrompts: 1},
	} {
		session := completeFakeOfficialSDKSessionV1()
		session.initialize.Capabilities.Resources, session.initialize.Capabilities.Prompts = nil, nil
		discovery, err := newOfficialSDKDiscoveryV1(session, func() time.Time { return testkit.FixedTime }, limits)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := discovery.DiscoverV1(context.Background(), officialSDKDiscoveryRequestV1()); err == nil {
			t.Fatalf("discovery accepted bounded overflow with limits %+v", limits)
		}
	}
}

func TestOfficialSDKDiscoveryV1ConcurrentIndependentSnapshots(t *testing.T) {
	discovery, err := newOfficialSDKDiscoveryV1(completeFakeOfficialSDKSessionV1(), func() time.Time { return testkit.FixedTime }, DefaultOfficialSDKDiscoveryLimitsV1())
	if err != nil {
		t.Fatal(err)
	}
	const workers = 64
	var wg sync.WaitGroup
	results := make(chan toolcontract.MCPCapabilitySnapshotV2, workers)
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := discovery.DiscoverV1(context.Background(), officialSDKDiscoveryRequestV1())
			results <- result
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var digest string
	for result := range results {
		if digest == "" {
			digest = string(result.Digest)
		} else if digest != string(result.Digest) {
			t.Fatal("concurrent discovery produced non-deterministic digests")
		}
	}
}

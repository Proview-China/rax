package applicationadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	application "github.com/Proview-China/rax/ExecutionRuntime/application"
	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/refreshstore"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type ownerReaderStubV1 struct {
	owner applicationcontract.ContextOwnerKindV1
	body  []byte
	now   time.Time
	drift bool
}

type ownerReaderFaultV1 struct {
	*ownerReaderStubV1
	readErr error
	reads   int
}

func (r *ownerReaderFaultV1) ReadContextOwnerContentExactV1(ctx context.Context, request applicationcontract.ContextOwnerContentRequestV1) (applicationcontract.ContextOwnerContentObservationV1, []byte, error) {
	r.reads++
	if r.readErr != nil {
		return applicationcontract.ContextOwnerContentObservationV1{}, nil, r.readErr
	}
	return r.ownerReaderStubV1.ReadContextOwnerContentExactV1(ctx, request)
}

func (s *ownerReaderStubV1) InspectContextOwnerSourceCurrentV1(_ context.Context, request applicationcontract.ContextOwnerSourceRequestV1) (applicationcontract.ContextOwnerSourceEnvelopeV1, error) {
	item, err := applicationcontract.SealContextOwnerSourceItemV1(applicationcontract.ContextOwnerSourceItemV1{
		Rank: 0, ItemDigest: core.DigestBytes([]byte(string(s.owner) + "-item")), RecordRef: appRef(string(s.owner)+"/record", string(s.owner)+"-record", string(s.owner)+"-record"),
		StableOwnerChain: []applicationcontract.ContextRefreshExactRefV1{appRef(string(s.owner)+"/source", string(s.owner)+"-source", string(s.owner)+"-source")},
		ContentRef:       applicationcontract.ContextOwnerContentRefV1{ID: string(s.owner) + "-content", Digest: core.DigestBytes(s.body), Length: int64(len(s.body)), MediaType: "text/plain"},
		TokenEstimate:    3, Sensitivity: "internal", CitationDigest: core.DigestBytes([]byte(string(s.owner) + "-citation")), License: licenseForOwner(s.owner), ExpiresUnixNano: s.now.Add(8 * time.Second).UnixNano(),
	})
	if err != nil {
		return applicationcontract.ContextOwnerSourceEnvelopeV1{}, err
	}
	closure := core.DigestBytes([]byte(string(s.owner) + "-closure"))
	if s.drift && request.Phase == applicationcontract.ContextSourceCheckS2V1 {
		closure = core.DigestBytes([]byte("drift"))
	}
	envelope, err := applicationcontract.SealContextOwnerSourceEnvelopeV1(applicationcontract.ContextOwnerSourceEnvelopeV1{
		ID: string(s.owner) + "-envelope-" + string(request.Phase), Owner: s.owner,
		SourceSession: request.SourceSession, SessionApplicability: request.SessionApplicability, SourceTurn: request.SourceTurn, TurnApplicability: request.TurnApplicability,
		AttemptInspectionRef: appRef(string(s.owner)+"/inspection", string(s.owner)+"-inspection-"+string(request.Phase), string(s.owner)+"-inspection-"+string(request.Phase)),
		CurrentProjectionRef: appRef(string(s.owner)+"/projection", string(s.owner)+"-projection-"+string(request.Phase), string(s.owner)+"-projection-"+string(request.Phase)),
		StableClosureDigest:  closure, Items: []applicationcontract.ContextOwnerSourceItemV1{item}, Phase: request.Phase,
		CheckedUnixNano: s.now.UnixNano(), ExpiresUnixNano: s.now.Add(8 * time.Second).UnixNano(),
	})
	if err != nil {
		return applicationcontract.ContextOwnerSourceEnvelopeV1{}, err
	}
	if request.Phase == applicationcontract.ContextSourceCheckS2V1 && envelope.StableAssociationDigest != request.ExpectedStableDigest {
		return applicationcontract.ContextOwnerSourceEnvelopeV1{}, errors.New("stable association drift")
	}
	return envelope, nil
}

func (s *ownerReaderStubV1) ReadContextOwnerContentExactV1(_ context.Context, request applicationcontract.ContextOwnerContentRequestV1) (applicationcontract.ContextOwnerContentObservationV1, []byte, error) {
	if request.ValidateCurrent(s.now) != nil || request.Envelope.Owner != s.owner {
		return applicationcontract.ContextOwnerContentObservationV1{}, nil, contract.ErrInvalid
	}
	item := request.Envelope.Items[request.Rank]
	observation, err := applicationcontract.SealContextOwnerContentObservationV1(applicationcontract.ContextOwnerContentObservationV1{
		ID: string(s.owner) + "-content-observation-" + string(request.Envelope.Phase), Owner: s.owner,
		EnvelopeRef: appRef(string(s.owner)+"/context-envelope", request.Envelope.ID, string(request.Envelope.Digest)), ProjectionItemDigest: item.ItemDigest,
		ContentRef: item.ContentRef, ObservedLength: int64(len(s.body)), ObservedDigest: core.DigestBytes(s.body), CheckedUnixNano: s.now.UnixNano(), ExpiresUnixNano: s.now.Add(8 * time.Second).UnixNano(),
	})
	if err != nil {
		return applicationcontract.ContextOwnerContentObservationV1{}, nil, err
	}
	return observation, bytes.Clone(s.body), nil
}

func appRef(kind, id, seed string) applicationcontract.ContextRefreshExactRefV1 {
	return applicationcontract.ContextRefreshExactRefV1{Kind: runtimeports.NamespacedNameV2(kind), ID: id, Revision: 1, Digest: core.DigestBytes([]byte(seed))}
}

func licenseForOwner(owner applicationcontract.ContextOwnerKindV1) string {
	if owner == applicationcontract.ContextOwnerKnowledgeV1 {
		return "Apache-2.0"
	}
	return ""
}

type lostReplyContextPortV1 struct {
	*ContextTurnRefreshAdapterV1
	once sync.Once
}

type tamperApplyBindingStoreV1 struct {
	*refreshstore.Memory
	mode string
}

func (s *tamperApplyBindingStoreV1) ApplyContextTurnRefreshCurrentCASV1(ctx context.Context, commit contract.ContextTurnRefreshCommitV1) (contract.ContextTurnRefreshResultV1, error) {
	switch s.mode {
	case "settlement-s2":
		commit.Settlement.S2AssociationSetDigest = contract.Digest(core.DigestBytes([]byte("wrong-settlement-s2")))
	case "proof-ref":
		wrong := contract.FactRef{ID: "another-transition-proof", Revision: 1, Digest: contract.Digest(core.DigestBytes([]byte("another-transition-proof")))}
		commit.Apply.TransitionProofRef = &wrong
		commit.Settlement.TransitionProofRef = &wrong
		var err error
		commit.Apply, err = contract.SealApplyContextTurnRefreshRequestV1(commit.Apply, commit.AppliedUnixNano)
		if err != nil {
			return contract.ContextTurnRefreshResultV1{}, err
		}
	}
	return s.Memory.ApplyContextTurnRefreshCurrentCASV1(ctx, commit)
}

func (p *lostReplyContextPortV1) ApplyContextTurnRefreshV1(ctx context.Context, request applicationcontract.ContextTurnRefreshApplyRequestV1) (applicationcontract.ContextTurnRefreshResultV1, error) {
	result, err := p.ContextTurnRefreshAdapterV1.ApplyContextTurnRefreshV1(ctx, request)
	if err != nil {
		return result, err
	}
	lost := false
	p.once.Do(func() { lost = true })
	if lost {
		return applicationcontract.ContextTurnRefreshResultV1{}, errors.New("lost reply")
	}
	return result, nil
}

func TestContextRefreshCrossOwnerExactFrameAndLostReply(t *testing.T) {
	fixture, err := testfixture.NewRefreshFixtureWithOwnerSourcesV1()
	if err != nil {
		t.Fatal(err)
	}
	memory := &ownerReaderStubV1{owner: applicationcontract.ContextOwnerMemoryV1, body: []byte("memory exact body"), now: fixture.Now}
	knowledge := &ownerReaderStubV1{owner: applicationcontract.ContextOwnerKnowledgeV1, body: []byte("knowledge exact body"), now: fixture.Now}
	adapter, err := NewContextTurnRefreshAdapterV1(fixture.Service, fixture.Store, fixture.Store, fixture.Parent.Content, memory, knowledge, fixture.Clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := application.NewContextTurnRefreshCoordinatorV1(application.ContextTurnRefreshCoordinatorConfigV1{Context: &lostReplyContextPortV1{ContextTurnRefreshAdapterV1: adapter}, Memory: memory, Knowledge: knowledge, Clock: fixture.Clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	request := coordinationRequestV1(t, fixture, memory, knowledge)
	result, err := coordinator.CoordinateContextTurnRefreshV1(context.Background(), request)
	if err != nil || result.State != applicationcontract.ContextTurnRefreshAppliedStateV1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	frameRef := contract.FactRef{ID: result.FrameRef.ID, Revision: uint64(result.FrameRef.Revision), Digest: contract.Digest(result.FrameRef.Digest)}
	frame, err := fixture.Store.FrameByExactRef(context.Background(), frameRef, fixture.Request.ExpectedCurrent.ExecutionScopeDigest)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := fixture.Store.ManifestByExactRef(context.Background(), frame.ManifestRef, frame.Execution.ScopeDigest)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[contract.FragmentKind]bool{}
	for _, fragment := range manifest.Fragments {
		kinds[fragment.Kind] = true
	}
	if !kinds[contract.FragmentMemoryRecall] || !kinds[contract.FragmentKnowledgeReference] || !kinds[contract.FragmentToolResult] {
		t.Fatalf("missing exact source fragments: %#v", kinds)
	}

	inspected, err := adapter.InspectContextTurnRefreshV1(context.Background(), mustInspectRequestV1(t, result.AttemptRef))
	if err != nil || inspected.Digest != result.Digest {
		t.Fatalf("inspect=%+v err=%v", inspected, err)
	}

	results := make(chan error, 64)
	for range 64 {
		go func() {
			again, callErr := coordinator.CoordinateContextTurnRefreshV1(context.Background(), request)
			if callErr == nil && (again.State != applicationcontract.ContextTurnRefreshAppliedStateV1 || again.FrameRef != result.FrameRef) {
				callErr = errors.New("concurrent inspect returned another applied Frame")
			}
			results <- callErr
		}()
	}
	for range 64 {
		if callErr := <-results; callErr != nil {
			t.Fatal(callErr)
		}
	}
}

func TestContextRefreshS2StableDriftPublishesNothing(t *testing.T) {
	fixture, err := testfixture.NewRefreshFixtureWithOwnerSourcesV1()
	if err != nil {
		t.Fatal(err)
	}
	memory := &ownerReaderStubV1{owner: applicationcontract.ContextOwnerMemoryV1, body: []byte("memory exact body"), now: fixture.Now, drift: true}
	adapter, err := NewContextTurnRefreshAdapterV1(fixture.Service, fixture.Store, fixture.Store, fixture.Parent.Content, memory, nil, fixture.Clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := application.NewContextTurnRefreshCoordinatorV1(application.ContextTurnRefreshCoordinatorConfigV1{Context: adapter, Memory: memory, Clock: fixture.Clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	request := coordinationRequestV1(t, fixture, memory, nil)
	if _, err := coordinator.CoordinateContextTurnRefreshV1(context.Background(), request); err == nil {
		t.Fatal("stable S2 drift was accepted")
	}
	current, err := fixture.Store.InspectCurrentGenerationPointer(context.Background(), contract.ContextGenerationCurrentPointerRequestV1{ExecutionScopeDigest: fixture.Request.ExpectedCurrent.ExecutionScopeDigest, RunID: fixture.Request.ExpectedCurrent.RunID, SessionRef: fixture.Request.ExpectedCurrent.SessionRef, Turn: fixture.Request.ExpectedCurrent.Turn})
	if err != nil || current != fixture.Request.ExpectedCurrent {
		t.Fatalf("S2 failure changed current: %+v %v", current, err)
	}
}

func TestContextRefreshAtomicApplyRejectsProofAndSettlementAssociationDrift(t *testing.T) {
	for _, mode := range []string{"settlement-s2", "proof-ref"} {
		t.Run(mode, func(t *testing.T) {
			fixture, err := testfixture.NewRefreshFixtureWithOwnerSourcesV1()
			if err != nil {
				t.Fatal(err)
			}
			owner := &tamperApplyBindingStoreV1{Memory: fixture.Store, mode: mode}
			service, err := kernel.NewContextTurnRefreshServiceV1(owner, fixture.ToolReader, fixture.Parent.Content, fixture.Clock.Now, 30*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			memory := &ownerReaderStubV1{owner: applicationcontract.ContextOwnerMemoryV1, body: []byte("memory exact body"), now: fixture.Now}
			adapter, err := NewContextTurnRefreshAdapterV1(service, owner, owner, fixture.Parent.Content, memory, nil, fixture.Clock.Now)
			if err != nil {
				t.Fatal(err)
			}
			coordinator, err := application.NewContextTurnRefreshCoordinatorV1(application.ContextTurnRefreshCoordinatorConfigV1{Context: adapter, Memory: memory, Clock: fixture.Clock.Now})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = coordinator.CoordinateContextTurnRefreshV1(context.Background(), coordinationRequestV1(t, fixture, memory, nil)); err == nil {
				t.Fatal("tampered proof/association reached Context current")
			}
			current, inspectErr := fixture.Store.InspectCurrentGenerationPointer(context.Background(), contract.ContextGenerationCurrentPointerRequestV1{ExecutionScopeDigest: fixture.Request.ExpectedCurrent.ExecutionScopeDigest, RunID: fixture.Request.ExpectedCurrent.RunID, SessionRef: fixture.Request.ExpectedCurrent.SessionRef, Turn: fixture.Request.ExpectedCurrent.Turn})
			if inspectErr != nil || current != fixture.Request.ExpectedCurrent {
				t.Fatalf("failed atomic apply changed current: %+v %v", current, inspectErr)
			}
		})
	}
}

func TestContextRefreshOwnerReadErrorsStayIndeterminateAndPublishNothing(t *testing.T) {
	for name, injected := range map[string]error{
		"unknown":     contract.ErrUnknown,
		"unavailable": contract.ErrUnavailable,
	} {
		t.Run(name, func(t *testing.T) {
			fixture, err := testfixture.NewRefreshFixtureWithOwnerSourcesV1()
			if err != nil {
				t.Fatal(err)
			}
			reader := &ownerReaderFaultV1{ownerReaderStubV1: &ownerReaderStubV1{owner: applicationcontract.ContextOwnerMemoryV1, body: []byte("memory exact body"), now: fixture.Now}, readErr: injected}
			adapter, err := NewContextTurnRefreshAdapterV1(fixture.Service, fixture.Store, fixture.Store, fixture.Parent.Content, reader, nil, fixture.Clock.Now)
			if err != nil {
				t.Fatal(err)
			}
			coordinator, err := application.NewContextTurnRefreshCoordinatorV1(application.ContextTurnRefreshCoordinatorConfigV1{Context: adapter, Memory: reader, Clock: fixture.Clock.Now})
			if err != nil {
				t.Fatal(err)
			}
			result, err := coordinator.CoordinateContextTurnRefreshV1(context.Background(), coordinationRequestV1(t, fixture, reader.ownerReaderStubV1, nil))
			if !errors.Is(err, injected) || !reflect.DeepEqual(result, applicationcontract.ContextTurnRefreshResultV1{}) {
				t.Fatalf("read error was reclassified or produced result: %+v %v", result, err)
			}
			assertContextRefreshCurrentUnchangedV1(t, fixture)
		})
	}
}

func TestContextRefreshCanceledAndOversizedOwnerContentPublishNothing(t *testing.T) {
	fixture, err := testfixture.NewRefreshFixtureWithOwnerSourcesV1()
	if err != nil {
		t.Fatal(err)
	}
	reader := &ownerReaderFaultV1{ownerReaderStubV1: &ownerReaderStubV1{owner: applicationcontract.ContextOwnerMemoryV1, body: []byte("memory exact body"), now: fixture.Now}}
	adapter, err := NewContextTurnRefreshAdapterV1(fixture.Service, fixture.Store, fixture.Store, fixture.Parent.Content, reader, nil, fixture.Clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := application.NewContextTurnRefreshCoordinatorV1(application.ContextTurnRefreshCoordinatorConfigV1{Context: adapter, Memory: reader, Clock: fixture.Clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result, err := coordinator.CoordinateContextTurnRefreshV1(ctx, coordinationRequestV1(t, fixture, reader.ownerReaderStubV1, nil)); !errors.Is(err, context.Canceled) || !reflect.DeepEqual(result, applicationcontract.ContextTurnRefreshResultV1{}) {
		t.Fatalf("canceled refresh produced result: %+v %v", result, err)
	}
	assertContextRefreshCurrentUnchangedV1(t, fixture)

	oversizedFixture, err := testfixture.NewRefreshFixtureWithOwnerSourcesV1()
	if err != nil {
		t.Fatal(err)
	}
	oversized := &ownerReaderFaultV1{ownerReaderStubV1: &ownerReaderStubV1{
		owner: applicationcontract.ContextOwnerMemoryV1,
		body:  make([]byte, contract.MaxContextTurnRefreshSourceBytesV1+1),
		now:   oversizedFixture.Now,
	}}
	oversizedAdapter, err := NewContextTurnRefreshAdapterV1(oversizedFixture.Service, oversizedFixture.Store, oversizedFixture.Store, oversizedFixture.Parent.Content, oversized, nil, oversizedFixture.Clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	oversizedCoordinator, err := application.NewContextTurnRefreshCoordinatorV1(application.ContextTurnRefreshCoordinatorConfigV1{Context: oversizedAdapter, Memory: oversized, Clock: oversizedFixture.Clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	result, err := oversizedCoordinator.CoordinateContextTurnRefreshV1(context.Background(), coordinationRequestV1(t, oversizedFixture, oversized.ownerReaderStubV1, nil))
	if !errors.Is(err, contract.ErrLimitExceeded) || !reflect.DeepEqual(result, applicationcontract.ContextTurnRefreshResultV1{}) || oversized.reads != 0 {
		t.Fatalf("oversized content materialized: result=%+v err=%v reads=%d", result, err, oversized.reads)
	}
	assertContextRefreshCurrentUnchangedV1(t, oversizedFixture)
}

func assertContextRefreshCurrentUnchangedV1(t *testing.T, fixture *testfixture.RefreshFixtureV1) {
	t.Helper()
	current, err := fixture.Store.InspectCurrentGenerationPointer(context.Background(), contract.ContextGenerationCurrentPointerRequestV1{
		ExecutionScopeDigest: fixture.Request.ExpectedCurrent.ExecutionScopeDigest,
		RunID:                fixture.Request.ExpectedCurrent.RunID, SessionRef: fixture.Request.ExpectedCurrent.SessionRef,
		Turn: fixture.Request.ExpectedCurrent.Turn,
	})
	if err != nil || current != fixture.Request.ExpectedCurrent {
		t.Fatalf("failed refresh changed current: %+v %v", current, err)
	}
}

func coordinationRequestV1(t *testing.T, fixture *testfixture.RefreshFixtureV1, memory, knowledge *ownerReaderStubV1) application.ContextTurnRefreshCoordinationRequestV1 {
	t.Helper()
	sessionDigest := core.Digest(fixture.Request.ExpectedCurrent.SessionRef.Digest)
	session := applicationcontract.SingleCallSessionCoordinateV1{ID: fixture.Request.ExpectedCurrent.SessionRef.ID, Revision: core.Revision(fixture.Request.ExpectedCurrent.SessionRef.Revision), Digest: sessionDigest, Phase: applicationcontract.SingleCallSessionWaitingActionV1, CheckedUnixNano: fixture.Now.UnixNano(), ExpiresUnixNano: fixture.Now.Add(10 * time.Second).UnixNano()}
	sessionApplicability := applicationcontract.SingleCallSessionApplicabilitySourceCoordinateV1{Kind: applicationcontract.SingleCallSessionSourceKindV1, ID: "session:" + string(sessionDigest), Revision: session.Revision, Digest: sessionDigest}
	turnDigest := core.DigestBytes([]byte("turn-current"))
	turn := applicationcontract.SingleCallTurnCoordinateV1{ID: "turn:" + string(turnDigest), Ordinal: fixture.Request.ExpectedCurrent.Turn, Revision: 1, Digest: turnDigest}
	turnApplicability := applicationcontract.SingleCallTurnApplicabilitySourceCoordinateV1{Kind: applicationcontract.SingleCallTurnSourceKindV1, ID: turn.ID, Revision: 1, Digest: turnDigest}
	payload, err := json.Marshal(fixture.Request)
	if err != nil {
		t.Fatal(err)
	}
	result := application.ContextTurnRefreshCoordinationRequestV1{ID: "application-context-refresh-1", ExecutionScopeDigest: core.Digest(fixture.Request.ExpectedCurrent.ExecutionScopeDigest), RunID: core.AgentRunID(fixture.Request.ExpectedCurrent.RunID), SourceSession: session, SessionApplicability: sessionApplicability, SourceTurn: turn, TurnApplicability: turnApplicability, OpaqueContextRequest: payload, RequestedNotAfterNano: fixture.Request.NotAfterUnixNano}
	if memory != nil {
		r := sourceRequestV1(t, applicationcontract.ContextOwnerMemoryV1, session, sessionApplicability, turn, turnApplicability, fixture.Now)
		result.Memory = &r
	}
	if knowledge != nil {
		r := sourceRequestV1(t, applicationcontract.ContextOwnerKnowledgeV1, session, sessionApplicability, turn, turnApplicability, fixture.Now)
		result.Knowledge = &r
	}
	return result
}

func sourceRequestV1(t *testing.T, owner applicationcontract.ContextOwnerKindV1, session applicationcontract.SingleCallSessionCoordinateV1, sessionApplicability applicationcontract.SingleCallSessionApplicabilitySourceCoordinateV1, turn applicationcontract.SingleCallTurnCoordinateV1, turnApplicability applicationcontract.SingleCallTurnApplicabilitySourceCoordinateV1, now time.Time) applicationcontract.ContextOwnerSourceRequestV1 {
	t.Helper()
	request, err := applicationcontract.SealContextOwnerSourceRequestV1(applicationcontract.ContextOwnerSourceRequestV1{Owner: owner, SourceSession: session, SessionApplicability: sessionApplicability, SourceTurn: turn, TurnApplicability: turnApplicability, OwnerRequest: []byte(`{"fixture":true}`), Phase: applicationcontract.ContextSourceCheckS1V1, RequestedNotAfterNano: now.Add(10 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func mustInspectRequestV1(t *testing.T, attempt applicationcontract.ContextRefreshExactRefV1) applicationcontract.ContextTurnRefreshInspectRequestV1 {
	t.Helper()
	request, err := applicationcontract.SealContextTurnRefreshInspectRequestV1(applicationcontract.ContextTurnRefreshInspectRequestV1{AttemptRef: attempt})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

package integration_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application"
	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	contextadapter "github.com/Proview-China/rax/ExecutionRuntime/context-engine/applicationadapter"
	contextcontract "github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	harnessadapter "github.com/Proview-China/rax/ExecutionRuntime/harness/applicationadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type lostApplyContextPortV1 struct {
	applicationports.ContextTurnRefreshPortV1
	lost atomic.Bool
}

func (p *lostApplyContextPortV1) ApplyContextTurnRefreshV1(ctx context.Context, request applicationcontract.ContextTurnRefreshApplyRequestV1) (applicationcontract.ContextTurnRefreshResultV1, error) {
	result, err := p.ContextTurnRefreshPortV1.ApplyContextTurnRefreshV1(ctx, request)
	if err != nil {
		return result, err
	}
	if p.lost.CompareAndSwap(false, true) {
		return applicationcontract.ContextTurnRefreshResultV1{}, errors.New("injected Context Apply lost reply")
	}
	return result, nil
}

type rejectingContextCoordinatorV1 struct{ calls atomic.Int64 }

func (c *rejectingContextCoordinatorV1) CoordinateContextTurnRefreshV1(context.Context, application.ContextTurnRefreshCoordinationRequestV1) (applicationcontract.ContextTurnRefreshResultV1, error) {
	c.calls.Add(1)
	return applicationcontract.ContextTurnRefreshResultV1{}, errors.New("Context must not be called after final current recovery")
}

func TestTurnContinuationCoordinatorRealContextSQLiteLostRepliesAndRestartV1(t *testing.T) {
	fixture, err := testfixture.NewRefreshFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	request := liveTurnContinuationRequestV1(t, fixture)
	contextPort, err := contextadapter.NewContextTurnRefreshAdapterV1(fixture.Service, fixture.Store, fixture.Store, fixture.Parent.Content, nil, nil, fixture.Clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	contextCoordinator, err := application.NewContextTurnRefreshCoordinatorV1(application.ContextTurnRefreshCoordinatorConfigV1{Context: &lostApplyContextPortV1{ContextTurnRefreshPortV1: contextPort}, Clock: fixture.Clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "turn-continuation.db")
	store, err := harnessadapter.OpenSQLiteTurnContinuationStoreV1(context.Background(), harnessadapter.SQLiteTurnContinuationStoreConfigV1{Path: path, Clock: fixture.Clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	store.LoseNextBeginReplyForTestingV1()
	store.LoseNextCommitReplyForTestingV1()
	coordinator, err := application.NewTurnContinuationCoordinatorV1(application.TurnContinuationCoordinatorConfigV1{Continuation: store, Context: contextCoordinator, Clock: fixture.Clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	current, err := coordinator.CoordinateTurnContinuationV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := current.ModelTurnAllowedV1(fixture.Clock.Now()); err != nil {
		t.Fatal(err)
	}
	if current.AppliedContextRefresh == nil || current.AppliedContextRefresh.AttemptRef != request.Start.ExpectedContextRefreshAttempt {
		t.Fatalf("real Context result was not committed exactly: %+v", current)
	}
	if err := store.IntegrityCheckV1(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := harnessadapter.OpenSQLiteTurnContinuationStoreV1(context.Background(), harnessadapter.SQLiteTurnContinuationStoreConfigV1{Path: path, Clock: fixture.Clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	rejecting := &rejectingContextCoordinatorV1{}
	recovery, err := application.NewTurnContinuationCoordinatorV1(application.TurnContinuationCoordinatorConfigV1{Continuation: restarted, Context: rejecting, Clock: fixture.Clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := recovery.CoordinateTurnContinuationV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Digest != current.Digest || rejecting.calls.Load() != 0 {
		t.Fatalf("restart did not recover final current before Context: recovered=%s want=%s context_calls=%d", recovered.Digest, current.Digest, rejecting.calls.Load())
	}
	if err := recovered.ModelTurnAllowedV1(fixture.Clock.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestTurnContinuationCoordinatorMultiCoordinatorMultiSQLiteHandle64V1(t *testing.T) {
	fixture, err := testfixture.NewRefreshFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	request := liveTurnContinuationRequestV1(t, fixture)
	path := filepath.Join(t.TempDir(), "turn-continuation-concurrent.db")
	const handles = 4
	stores := make([]*harnessadapter.SQLiteTurnContinuationStoreV1, 0, handles)
	coordinators := make([]*application.TurnContinuationCoordinatorV1, 0, handles)
	contextPort, err := contextadapter.NewContextTurnRefreshAdapterV1(fixture.Service, fixture.Store, fixture.Store, fixture.Parent.Content, nil, nil, fixture.Clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	contextCoordinator, err := application.NewContextTurnRefreshCoordinatorV1(application.ContextTurnRefreshCoordinatorConfigV1{Context: contextPort, Clock: fixture.Clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	for range handles {
		store, openErr := harnessadapter.OpenSQLiteTurnContinuationStoreV1(context.Background(), harnessadapter.SQLiteTurnContinuationStoreConfigV1{Path: path, Clock: fixture.Clock.Now})
		if openErr != nil {
			t.Fatal(openErr)
		}
		stores = append(stores, store)
		coordinator, coordinatorErr := application.NewTurnContinuationCoordinatorV1(application.TurnContinuationCoordinatorConfigV1{Continuation: store, Context: contextCoordinator, Clock: fixture.Clock.Now})
		if coordinatorErr != nil {
			t.Fatal(coordinatorErr)
		}
		coordinators = append(coordinators, coordinator)
	}
	defer func() {
		for _, store := range stores {
			_ = store.Close()
		}
	}()

	const workers = 64
	values := make(chan applicationcontract.TurnContinuationCurrentV1, workers)
	errs := make(chan error, workers)
	var wait sync.WaitGroup
	for index := range workers {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			value, callErr := coordinators[index%len(coordinators)].CoordinateTurnContinuationV1(context.Background(), request)
			if callErr != nil {
				errs <- callErr
				return
			}
			values <- value
		}(index)
	}
	wait.Wait()
	close(values)
	close(errs)
	for callErr := range errs {
		t.Fatal(callErr)
	}
	var exact applicationcontract.TurnContinuationCurrentV1
	for value := range values {
		if exact.Digest == "" {
			exact = value
		}
		if value.Digest != exact.Digest || value.ActiveContext != exact.ActiveContext || value.AppliedContextRefresh == nil || exact.AppliedContextRefresh == nil || value.AppliedContextRefresh.FrameRef != exact.AppliedContextRefresh.FrameRef {
			t.Fatal("multi-coordinator execution produced another successor or ContextFrame")
		}
	}
	if err := exact.ModelTurnAllowedV1(fixture.Clock.Now()); err != nil {
		t.Fatal(err)
	}
	for _, store := range stores {
		if err := store.IntegrityCheckV1(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTurnContinuationCoordinatorSameAttemptDifferentPayloadAcrossSQLiteHandlesV1(t *testing.T) {
	fixture, err := testfixture.NewRefreshFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	request := liveTurnContinuationRequestV1(t, fixture)
	path := filepath.Join(t.TempDir(), "turn-continuation-splice.db")
	firstStore, err := harnessadapter.OpenSQLiteTurnContinuationStoreV1(context.Background(), harnessadapter.SQLiteTurnContinuationStoreConfigV1{Path: path, Clock: fixture.Clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	defer firstStore.Close()
	secondStore, err := harnessadapter.OpenSQLiteTurnContinuationStoreV1(context.Background(), harnessadapter.SQLiteTurnContinuationStoreConfigV1{Path: path, Clock: fixture.Clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	defer secondStore.Close()

	firstContext := &rejectingContextCoordinatorV1{}
	first, err := application.NewTurnContinuationCoordinatorV1(application.TurnContinuationCoordinatorConfigV1{Continuation: firstStore, Context: firstContext, Clock: fixture.Clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = first.CoordinateTurnContinuationV1(context.Background(), request); err == nil {
		t.Fatal("setup Context rejection was accepted")
	}

	spliced := request
	spliced.ContextRefresh.OpaqueContextRequest = []byte(`{"tool_result":"different"}`)
	secondContext := &rejectingContextCoordinatorV1{}
	second, err := application.NewTurnContinuationCoordinatorV1(application.TurnContinuationCoordinatorConfigV1{Continuation: secondStore, Context: secondContext, Clock: fixture.Clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = second.CoordinateTurnContinuationV1(context.Background(), spliced); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("same Attempt with another payload crossed the second handle: %v", err)
	}
	if secondContext.calls.Load() != 0 {
		t.Fatalf("payload splice reached Context through another coordinator: calls=%d", secondContext.calls.Load())
	}
	inspect, err := applicationcontract.SealTurnContinuationInspectRequestV1(applicationcontract.TurnContinuationInspectRequestV1{AttemptRef: request.Start.AttemptRefV1()})
	if err != nil {
		t.Fatal(err)
	}
	current, err := secondStore.InspectTurnContinuationV1(context.Background(), inspect)
	if err != nil {
		t.Fatal(err)
	}
	if current.State != applicationcontract.TurnContinuationPendingV1 || current.ModelTurnAllowedV1(fixture.Clock.Now()) == nil {
		t.Fatalf("payload splice changed pending state or opened Model gate: %+v", current)
	}
}

func liveTurnContinuationRequestV1(t *testing.T, fixture *testfixture.RefreshFixtureV1) application.TurnContinuationCoordinationRequestV1 {
	t.Helper()
	contextRequest := coordinationRequest(t, fixture)
	contextRequest.SourceSession.ExpiresUnixNano = fixture.Now.Add(20 * time.Second).UnixNano()
	source, err := applicationcontract.SealContextTurnSourceCurrentV1(applicationcontract.ContextTurnSourceCurrentV1{
		ExecutionScopeDigest: contextRequest.ExecutionScopeDigest, RunID: contextRequest.RunID,
		Session: contextRequest.SourceSession, SessionApplicability: contextRequest.SessionApplicability,
		Turn: contextRequest.SourceTurn, TurnApplicability: contextRequest.TurnApplicability,
		CheckedUnixNano: contextRequest.SourceSession.CheckedUnixNano, ExpiresUnixNano: contextRequest.SourceSession.ExpiresUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	manifestRef := contextApplicationRefV1("context/manifest", fixture.Parent.Frame.ManifestRef)
	frameDigest, err := fixture.Parent.Frame.DigestValue()
	if err != nil {
		t.Fatal(err)
	}
	frameRef := contextApplicationRefV1("context/frame", contextcontract.FactRef{ID: fixture.Parent.Frame.ID, Revision: fixture.Parent.Frame.Revision, Digest: frameDigest})
	generationDigest, err := fixture.Parent.Generation.DigestValue()
	if err != nil {
		t.Fatal(err)
	}
	generationRef := contextApplicationRefV1("context/generation", contextcontract.FactRef{ID: fixture.Parent.Generation.ID, Revision: fixture.Parent.Generation.Revision, Digest: generationDigest})
	pointerRef := contextApplicationRefV1("context/generation-current", contextcontract.FactRef{ID: fixture.Parent.Pointer.ID, Revision: fixture.Parent.Pointer.Revision, Digest: fixture.Parent.Pointer.Digest})
	active, err := applicationcontract.SealHarnessActiveContextRefV1(applicationcontract.HarnessActiveContextRefV1{
		Revision: 1, ExecutionScopeDigest: contextRequest.ExecutionScopeDigest, RunID: contextRequest.RunID, SessionID: source.Session.ID, TurnOrdinal: source.Turn.Ordinal,
		ManifestRef: manifestRef, FrameRef: frameRef, GenerationRef: generationRef, ContextCurrentPointerRef: pointerRef, UpdatedUnixNano: fixture.Now.Add(-time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	tool := fixture.ToolProjection.Request
	startInput := applicationcontract.TurnContinuationStartRequestV1{
		ExecutionScopeDigest: contextRequest.ExecutionScopeDigest, RunID: contextRequest.RunID, Source: source,
		SettledToolResult: applicationcontract.SingleCallToolActionResultRefV2{
			ID: "live-tool-result-coordinate", Revision: 1, Digest: core.Digest(fixture.ToolProjection.Digest),
			RequestID: tool.AttemptID, RequestRevision: 1, RequestDigest: core.Digest(tool.ApplySettlementRef.Digest),
			ActionCoordinateDigest: core.Digest(tool.AssociationRef.Digest), ToolResultID: tool.ToolResultRef.ID, ToolResultRevision: core.Revision(tool.ToolResultRef.Revision), ToolResultDigest: core.Digest(tool.ToolResultRef.Digest),
		},
		ExpectedActiveContext: active, TargetTurn: source.Turn.Ordinal + 1, RequestedNotAfterUnixNano: contextRequest.RequestedNotAfterNano,
	}
	attemptID, err := applicationcontract.DeriveTurnContinuationAttemptIDV1(startInput)
	if err != nil {
		t.Fatal(err)
	}
	contextRequest.ID = attemptID
	prepare, err := applicationcontract.SealContextTurnRefreshPrepareRequestV1(applicationcontract.ContextTurnRefreshPrepareRequestV1{
		ID: contextRequest.ID, ExecutionScopeDigest: contextRequest.ExecutionScopeDigest, RunID: contextRequest.RunID,
		SourceSession: contextRequest.SourceSession, SessionApplicability: contextRequest.SessionApplicability,
		SourceTurn: contextRequest.SourceTurn, TurnApplicability: contextRequest.TurnApplicability, ExpectedTargetTurn: contextRequest.SourceTurn.Ordinal + 1,
		OpaqueContextRequest: contextRequest.OpaqueContextRequest, RequestedNotAfterNano: contextRequest.RequestedNotAfterNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	startInput.ExpectedContextRefreshAttempt, err = prepare.AttemptRefV1()
	if err != nil {
		t.Fatal(err)
	}
	start, err := applicationcontract.SealTurnContinuationStartRequestV1(startInput)
	if err != nil {
		t.Fatal(err)
	}
	return application.TurnContinuationCoordinationRequestV1{Start: start, ContextRefresh: contextRequest}
}

func contextApplicationRefV1(kind string, ref contextcontract.FactRef) applicationcontract.ContextRefreshExactRefV1 {
	return applicationcontract.ContextRefreshExactRefV1{Kind: runtimeports.NamespacedNameV2(kind), ID: ref.ID, Revision: core.Revision(ref.Revision), Digest: core.Digest(ref.Digest)}
}

var (
	_ application.ContextTurnRefreshCoordinatorPortV1 = (*application.ContextTurnRefreshCoordinatorV1)(nil)
	_ applicationports.ContextTurnRefreshPortV1       = (*lostApplyContextPortV1)(nil)
)

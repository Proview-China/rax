package applicationadapter_test

import (
	"context"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/action"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/applicationadapter"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type modelReader struct {
	projection modelinvoker.ToolCallCandidateObservationProjectionV1
	err        error
	calls      atomic.Int32
}

func (r *modelReader) InspectExactProjectionV1(_ context.Context, ref modelinvoker.ToolCallCandidateObservationRefV1) (modelinvoker.ToolCallCandidateObservationProjectionV1, error) {
	r.calls.Add(1)
	if r.err != nil {
		return modelinvoker.ToolCallCandidateObservationProjectionV1{}, r.err
	}
	if ref != r.projection.Ref {
		return modelinvoker.ToolCallCandidateObservationProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Model ref drift")
	}
	return r.projection.Clone(), nil
}

type bindingReader struct {
	value applicationadapter.SingleCallToolExactBindingsV1
	calls atomic.Int32
}

type dynamicBindingReader struct {
	provider runtimeports.ProviderBindingRefV2
}

func (r dynamicBindingReader) InspectSingleCallToolExactBindingsV1(_ context.Context, request applicationcontract.SingleCallToolActionRequestV1) (applicationadapter.SingleCallToolExactBindingsV1, error) {
	return bindingsFor(request, r.provider), nil
}

func (r *bindingReader) InspectSingleCallToolExactBindingsV1(context.Context, applicationcontract.SingleCallToolActionRequestV1) (applicationadapter.SingleCallToolExactBindingsV1, error) {
	r.calls.Add(1)
	return r.value, nil
}

type enforcementReader struct {
	value runtimeports.OperationDispatchEnforcementPhaseRefV4
}

func (r enforcementReader) InspectCurrentOperationProviderExecuteEnforcementV1(context.Context, runtimeports.OperationSubjectV3, runtimeports.OperationDispatchEnforcementPhaseRefV4) (runtimeports.OperationDispatchEnforcementPhaseRefV4, error) {
	return r.value, nil
}

type handoffReader struct {
	value runtimeports.OperationScopeEvidenceProviderHandoffFactV3
}

func (r handoffReader) InspectCurrentOperationProviderEvidenceHandoffV1(context.Context, runtimeports.OperationScopeEvidenceProviderHandoffRefV3) (runtimeports.OperationScopeEvidenceProviderHandoffFactV3, error) {
	return r.value, nil
}

type settlementReader struct {
	inspection       runtimeports.OperationInspectionSettlementRefV4
	association      runtimeports.OperationSettlementEvidenceAssociationV4
	currentCalls     atomic.Int32
	associationCalls atomic.Int32
}

func (r *settlementReader) InspectCurrentOperationSettlementV4(_ context.Context, request runtimeports.InspectCurrentOperationSettlementRequestV4) (runtimeports.OperationInspectionSettlementRefV4, error) {
	r.currentCalls.Add(1)
	if request.EffectID != r.inspection.Settlement.EffectID || !runtimeports.SameOperationSubjectV3(request.Operation, r.inspection.DomainResult.Operation) {
		return runtimeports.OperationInspectionSettlementRefV4{}, core.NewError(core.ErrorConflict, core.ReasonEffectSettlementMissing, "settlement Inspect drift")
	}
	return r.inspection, nil
}
func (r *settlementReader) InspectOperationSettlementEvidenceAssociationV4(_ context.Context, operation runtimeports.OperationSubjectV3, ref runtimeports.OperationSettlementEvidenceAssociationRefV4) (runtimeports.OperationSettlementEvidenceAssociationV4, error) {
	r.associationCalls.Add(1)
	if !runtimeports.SameOperationSubjectV3(operation, r.inspection.DomainResult.Operation) || !runtimeports.SameOperationSettlementEvidenceAssociationRefV4(ref, r.association.RefV4()) {
		return runtimeports.OperationSettlementEvidenceAssociationV4{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Association Inspect drift")
	}
	return r.association, nil
}

// stageStubFlow isolates Application adapter gates and TTL behavior. The real
// Tool Owner closure is exercised by owner_flow_v1_test.go.
type stageStubFlow struct {
	coordination  *action.CoordinationStoreV1
	fixture       testkit.ApplicationG6AFixtureV1
	boundary      testkit.BoundaryFixtureV1
	calls         atomic.Int32
	providerCalls atomic.Int32
}

func (f *stageStubFlow) StartOrInspectToolOwnerSingleCallV1(ctx context.Context, input applicationadapter.ToolOwnerSingleCallExecutionV1) (toolcontract.ToolResultV2, error) {
	f.calls.Add(1)
	now := testkit.FixedTime
	w, err := f.coordination.BindCandidateV1(input.Watermark, f.fixture.ToolResult.Action, now)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	w, err = f.coordination.BindReservationV1(source(w), f.fixture.ToolResult.Reservation, now)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	w, err = f.coordination.BindRuntimeAttemptV1(source(w), f.boundary.Operation, f.boundary.Attempt, now)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	boundary, err := f.coordination.CrossProviderBoundaryV1(ctx, source(w), f.boundary.Enforcement, f.boundary.Handoff.RefV3(), now)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	f.providerCalls.Add(1)
	w, err = f.coordination.RecordProviderObservationV1(boundary, testkit.ProviderObservation(now), now)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	w, err = f.coordination.BindDomainResultV1(source(w), f.fixture.ToolResult.DomainResult, now)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	w, err = f.coordination.BindApplyV1(source(w), f.fixture.ToolResult.Apply, now)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	_, err = f.coordination.BindResultV1(source(w), contractRef(f.fixture.ToolResult.ID, f.fixture.ToolResult.Revision, f.fixture.ToolResult.Digest), now)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	return f.fixture.ToolResult, nil
}

type fixture struct {
	adapter *applicationadapter.SingleCallToolActionAdapterV1
	request applicationcontract.SingleCallToolActionRequestV1
	model   *modelReader
	flow    *stageStubFlow
	results applicationadapter.ApplicationResultStoreV1
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	now := testkit.FixedTime
	facts := testkit.ApplicationG6AFixture(now)
	model := &modelReader{projection: facts.Projection}
	boundary := testkit.BoundaryFixture(now)
	coordination := action.NewCoordinationStoreV1(model, enforcementReader{boundary.Enforcement}, handoffReader{boundary.Handoff}, testkit.SettlementOwner())
	bindings := dynamicBindingReader{provider: facts.Provider}
	flow := &stageStubFlow{coordination: coordination, fixture: facts, boundary: boundary}
	settlement := &settlementReader{inspection: facts.Inspection, association: facts.Association}
	results := applicationadapter.NewInMemoryApplicationResultStoreV1()
	adapter := applicationadapter.NewSingleCallToolActionAdapterV1(model, bindings, coordination, flow, settlement, results, fixedClock{now})
	return fixture{adapter, facts.Request, model, flow, results}
}

func TestApplicationAdapterKeyedGateSameKeyInvokesInjectedFlowOnce(t *testing.T) {
	f := newFixture(t)
	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	digests := make(chan core.Digest, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := f.adapter.ExecuteSingleCallToolActionV1(context.Background(), f.request)
			if err != nil {
				errs <- err
				return
			}
			digests <- result.Digest
		}()
	}
	wg.Wait()
	close(errs)
	close(digests)
	for err := range errs {
		t.Fatal(err)
	}
	var first core.Digest
	count := 0
	for digest := range digests {
		if first == "" {
			first = digest
		}
		if digest != first {
			t.Fatal("concurrent callers observed different Results")
		}
		count++
	}
	if count != workers || f.flow.calls.Load() != 1 || f.flow.providerCalls.Load() != 1 {
		t.Fatalf("results=%d flow=%d provider=%d", count, f.flow.calls.Load(), f.flow.providerCalls.Load())
	}
}

type parallelFlow struct {
	result  toolcontract.ToolResultV2
	entered chan struct{}
	release chan struct{}
	active  atomic.Int32
	maximum atomic.Int32
}

func (f *parallelFlow) StartOrInspectToolOwnerSingleCallV1(context.Context, applicationadapter.ToolOwnerSingleCallExecutionV1) (toolcontract.ToolResultV2, error) {
	active := f.active.Add(1)
	for current := f.maximum.Load(); active > current && !f.maximum.CompareAndSwap(current, active); current = f.maximum.Load() {
	}
	f.entered <- struct{}{}
	<-f.release
	f.active.Add(-1)
	return f.result, nil
}

func TestApplicationAdapterDifferentCanonicalKeysRunInParallel(t *testing.T) {
	now := testkit.FixedTime
	facts := testkit.ApplicationG6AFixture(now)
	second := facts.Request
	second.Turn.ID = "turn-app-second"
	second.Turn.Digest = testkit.Digest("turn-app-second")
	var err error
	second, err = applicationcontract.SealSingleCallToolActionRequestV1(second)
	if err != nil {
		t.Fatal(err)
	}
	model := &modelReader{projection: facts.Projection}
	boundary := testkit.BoundaryFixture(now)
	coordination := action.NewCoordinationStoreV1(model, enforcementReader{boundary.Enforcement}, handoffReader{boundary.Handoff}, testkit.SettlementOwner())
	bindings := dynamicBindingReader{provider: facts.Provider}
	flow := &parallelFlow{result: facts.ToolResult, entered: make(chan struct{}, 2), release: make(chan struct{}, 2)}
	settlement := &settlementReader{inspection: facts.Inspection, association: facts.Association}
	adapter := applicationadapter.NewSingleCallToolActionAdapterV1(model, bindings, coordination, flow, settlement, applicationadapter.NewInMemoryApplicationResultStoreV1(), fixedClock{now})
	errs := make(chan error, 2)
	for _, request := range []applicationcontract.SingleCallToolActionRequestV1{facts.Request, second} {
		request := request
		go func() { _, err := adapter.ExecuteSingleCallToolActionV1(context.Background(), request); errs <- err }()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-flow.entered:
		case <-time.After(time.Second):
			t.Fatal("different canonical keys were globally serialized")
		}
	}
	flow.release <- struct{}{}
	flow.release <- struct{}{}
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if flow.maximum.Load() != 2 {
		t.Fatalf("maximum concurrent flows=%d, want 2", flow.maximum.Load())
	}
}

func TestApplicationAdapterModelFailureIsZeroWatermarkAndInspectIsReadOnly(t *testing.T) {
	f := newFixture(t)
	f.model.err = core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "Model reader unavailable")
	if _, err := f.adapter.ExecuteSingleCallToolActionV1(context.Background(), f.request); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("Model failure=%v", err)
	}
	if f.flow.calls.Load() != 0 {
		t.Fatal("Model failure reached Tool flow")
	}
	f.model.err = nil
	result, err := f.adapter.ExecuteSingleCallToolActionV1(context.Background(), f.request)
	if err != nil {
		t.Fatal(err)
	}
	modelCalls := f.model.calls.Load()
	flowCalls := f.flow.calls.Load()
	inspected, err := f.adapter.InspectSingleCallToolActionV1(context.Background(), applicationports.InspectSingleCallToolActionRequestV1{RequestID: f.request.ID, RequestDigest: f.request.Digest, ScopeDigest: f.request.ExecutionScopeDigest})
	if err != nil || inspected.Digest != result.Digest {
		t.Fatalf("Inspect=%#v err=%v", inspected, err)
	}
	if f.model.calls.Load() != modelCalls || f.flow.calls.Load() != flowCalls {
		t.Fatal("Inspect invoked Model or Tool flow")
	}
}

type losingResultStore struct {
	inner *applicationadapter.InMemoryApplicationResultStoreV1
	lose  atomic.Bool
}

func (s *losingResultStore) CreateSingleCallApplicationResultV1(ctx context.Context, request applicationcontract.SingleCallToolActionRequestV1, result applicationcontract.SingleCallToolActionResultV1) (applicationcontract.SingleCallToolActionResultV1, error) {
	stored, err := s.inner.CreateSingleCallApplicationResultV1(ctx, request, result)
	if err == nil && s.lose.CompareAndSwap(true, false) {
		return applicationcontract.SingleCallToolActionResultV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "lost create reply")
	}
	return stored, err
}
func (s *losingResultStore) InspectSingleCallApplicationResultV1(ctx context.Context, key applicationports.InspectSingleCallToolActionRequestV1) (applicationcontract.SingleCallToolActionResultV1, error) {
	return s.inner.InspectSingleCallApplicationResultV1(ctx, key)
}
func TestApplicationAdapterLostResultReplyRecoversByExactInspect(t *testing.T) {
	f := newFixture(t)
	losing := &losingResultStore{inner: applicationadapter.NewInMemoryApplicationResultStoreV1()}
	losing.lose.Store(true)
	f.adapter = applicationadapter.NewSingleCallToolActionAdapterV1(f.model, &bindingReader{value: bindingsFor(f.request, testkit.ApplicationG6AFixture(testkit.FixedTime).Provider)}, f.flow.coordination, f.flow, &settlementReader{inspection: testkit.ApplicationG6AFixture(testkit.FixedTime).Inspection, association: testkit.ApplicationG6AFixture(testkit.FixedTime).Association}, losing, fixedClock{testkit.FixedTime})
	if _, err := f.adapter.ExecuteSingleCallToolActionV1(context.Background(), f.request); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("lost reply=%v", err)
	}
	result, err := f.adapter.ExecuteSingleCallToolActionV1(context.Background(), f.request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Digest == "" || f.flow.calls.Load() != 1 || f.flow.providerCalls.Load() != 1 {
		t.Fatalf("recovery result=%s flow=%d provider=%d", result.Digest, f.flow.calls.Load(), f.flow.providerCalls.Load())
	}
}

type scriptedClock struct {
	mu     sync.Mutex
	values []time.Time
	index  int
}

func (c *scriptedClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.values) == 0 {
		return time.Time{}
	}
	if c.index >= len(c.values) {
		return c.values[len(c.values)-1]
	}
	value := c.values[c.index]
	c.index++
	return value
}

func TestApplicationAdapterFreshClockTTLAndRollbackGates(t *testing.T) {
	t.Run("rollback before flow", func(t *testing.T) {
		f := newFixture(t)
		f.adapter = applicationadapter.NewSingleCallToolActionAdapterV1(f.model, &bindingReader{value: bindingsFor(f.request, testkit.ApplicationG6AFixture(testkit.FixedTime).Provider)}, f.flow.coordination, f.flow, &settlementReader{inspection: testkit.ApplicationG6AFixture(testkit.FixedTime).Inspection, association: testkit.ApplicationG6AFixture(testkit.FixedTime).Association}, applicationadapter.NewInMemoryApplicationResultStoreV1(), &scriptedClock{values: []time.Time{testkit.FixedTime, testkit.FixedTime.Add(-time.Nanosecond)}})
		if _, err := f.adapter.ExecuteSingleCallToolActionV1(context.Background(), f.request); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("rollback error=%v", err)
		}
		if f.flow.calls.Load() != 0 {
			t.Fatal("clock rollback reached Tool flow")
		}
	})
	t.Run("request expires after flow before settlement", func(t *testing.T) {
		f := newFixture(t)
		clock := &scriptedClock{values: []time.Time{testkit.FixedTime, testkit.FixedTime, testkit.FixedTime, testkit.FixedTime.Add(8 * time.Second)}}
		settlement := &settlementReader{inspection: testkit.ApplicationG6AFixture(testkit.FixedTime).Inspection, association: testkit.ApplicationG6AFixture(testkit.FixedTime).Association}
		f.adapter = applicationadapter.NewSingleCallToolActionAdapterV1(f.model, &bindingReader{value: bindingsFor(f.request, testkit.ApplicationG6AFixture(testkit.FixedTime).Provider)}, f.flow.coordination, f.flow, settlement, applicationadapter.NewInMemoryApplicationResultStoreV1(), clock)
		if _, err := f.adapter.ExecuteSingleCallToolActionV1(context.Background(), f.request); err == nil {
			t.Fatal("TTL crossing produced Result")
		}
		if f.flow.calls.Load() != 1 || settlement.currentCalls.Load() != 0 {
			t.Fatalf("flow=%d settlement=%d", f.flow.calls.Load(), settlement.currentCalls.Load())
		}
	})
	t.Run("request expires after association", func(t *testing.T) {
		f := newFixture(t)
		clock := &scriptedClock{values: []time.Time{testkit.FixedTime, testkit.FixedTime, testkit.FixedTime, testkit.FixedTime, testkit.FixedTime, testkit.FixedTime.Add(8 * time.Second)}}
		settlement := &settlementReader{inspection: testkit.ApplicationG6AFixture(testkit.FixedTime).Inspection, association: testkit.ApplicationG6AFixture(testkit.FixedTime).Association}
		f.adapter = applicationadapter.NewSingleCallToolActionAdapterV1(f.model, &bindingReader{value: bindingsFor(f.request, testkit.ApplicationG6AFixture(testkit.FixedTime).Provider)}, f.flow.coordination, f.flow, settlement, applicationadapter.NewInMemoryApplicationResultStoreV1(), clock)
		if _, err := f.adapter.ExecuteSingleCallToolActionV1(context.Background(), f.request); err == nil {
			t.Fatal("post-Association TTL crossing produced Result")
		}
		if settlement.currentCalls.Load() != 1 || settlement.associationCalls.Load() != 1 {
			t.Fatalf("settlement=%d association=%d", settlement.currentCalls.Load(), settlement.associationCalls.Load())
		}
	})
}

func TestApplicationAdapterImportBoundary(t *testing.T) {
	files, err := parser.ParseDir(token.NewFileSet(), ".", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range files {
		for name, file := range pkg.Files {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			for _, spec := range file.Imports {
				path, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(path, "/application/") && path != "github.com/Proview-China/rax/ExecutionRuntime/application/contract" && path != "github.com/Proview-China/rax/ExecutionRuntime/application/ports" {
					t.Fatalf("forbidden Application implementation import %s", path)
				}
				if strings.Contains(path, "/runtime/") && path != "github.com/Proview-China/rax/ExecutionRuntime/runtime/core" && path != "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports" {
					t.Fatalf("forbidden Runtime implementation import %s", path)
				}
				if strings.Contains(path, "/model-invoker/") {
					t.Fatalf("forbidden Model implementation import %s", path)
				}
			}
		}
	}
}

func source(w toolcontract.SingleCallToolActionCoordinationWatermarkV1) toolcontract.ToolProviderBoundarySourceRefV1 {
	return toolcontract.ToolProviderBoundarySourceRefV1{WatermarkID: w.ID, WatermarkRevision: w.Revision, WatermarkDigest: w.Digest}
}
func contractRef(id string, revision core.Revision, digest core.Digest) toolcontract.ObjectRef {
	return toolcontract.ObjectRef{ID: id, Revision: revision, Digest: digest}
}
func bindingsFor(request applicationcontract.SingleCallToolActionRequestV1, provider runtimeports.ProviderBindingRefV2) applicationadapter.SingleCallToolExactBindingsV1 {
	bindings := applicationadapter.SingleCallToolExactBindingsV1{ActionID: "action-app", PendingAction: toolcontract.PendingActionExactRefV2{ID: request.PendingAction.ActionRef, Revision: 1, RequestDigest: request.PendingAction.RequestDigest}, Capability: toolcontract.ObjectRef{ID: string(request.PendingAction.Capability), Revision: 1, Digest: testkit.Digest("capability-app")}, Tool: toolcontract.ObjectRef{ID: "tool-app", Revision: 1, Digest: testkit.Digest("tool-app")}, SourceCandidate: toolcontract.ObjectRef{ID: request.PendingAction.SourceCandidateID, Revision: request.PendingAction.SourceCandidateRevision, Digest: request.PendingAction.SourceCandidateDigest}, Provider: provider}
	bindings.Candidate = candidateForBindings(request, bindings)
	return bindings
}

func candidateForBindings(request applicationcontract.SingleCallToolActionRequestV1, bindings applicationadapter.SingleCallToolExactBindingsV1) toolcontract.ActionCandidateV2 {
	now := testkit.FixedTime
	owner := testkit.SettlementOwner()
	current := func(kind runtimeports.NamespacedNameV2, ref toolcontract.ObjectRef) toolcontract.OwnerCurrentRefV1 {
		return testkit.CurrentRef(kind, ref, owner, now)
	}
	payload := runtimeports.OpaquePayloadV2{Schema: request.PendingAction.PayloadSchema, ContentDigest: request.PendingAction.PayloadDigest, Length: 1, Ref: "test://single-call/arguments", LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "tool/standard", Digest: testkit.Digest("limit-bindings")}}
	pending := bindings.PendingAction
	schemaRef := toolcontract.ObjectRef{ID: "schema-bindings", Revision: 1, Digest: request.PendingAction.PayloadSchema.ContentDigest}
	surface := toolcontract.ObjectRef{ID: "surface-bindings", Revision: 1, Digest: testkit.Digest("surface-bindings")}
	candidate, err := toolcontract.SealActionCandidateV2(toolcontract.ActionCandidateV2{ID: bindings.ActionID, TenantID: request.ExecutionScope.Identity.TenantID, RunID: string(request.Run.RunID), SessionID: request.Session.ID, TurnID: request.Turn.ID, PendingAction: pending, SourceCandidate: bindings.SourceCandidate, Capability: bindings.Capability, Tool: bindings.Tool, InputSchema: request.PendingAction.PayloadSchema, Payload: payload, PayloadRevision: 1, OperationScopeDigest: request.ExecutionScopeDigest, EffectKind: "praxis.tool/execute", ExpectedOwner: owner, ConflictDomain: "tenant/tenant-v2/tool/bindings", IdempotencyKey: request.ID, CreatedUnixNano: now.Add(-time.Second).UnixNano(), RequestedExpiresUnixNano: now.Add(6 * time.Second).UnixNano(), PendingActionCurrent: current("praxis.harness/pending-action", toolcontract.ObjectRef{ID: pending.ID, Revision: pending.Revision, Digest: pending.RequestDigest}), SurfaceCurrent: current("praxis.tool/surface", surface), CapabilityCurrent: current("praxis.tool/capability", bindings.Capability), ToolCurrent: current("praxis.tool/descriptor", bindings.Tool), InputSchemaCurrent: current("praxis.tool/input-schema", schemaRef), SourceCandidateCurrent: current("praxis.model/source-candidate", bindings.SourceCandidate)})
	if err != nil {
		panic(err)
	}
	return candidate
}

var _ applicationports.SingleCallToolActionPortV1 = (*applicationadapter.SingleCallToolActionAdapterV1)(nil)
var _ applicationadapter.ApplicationResultStoreV1 = (*losingResultStore)(nil)

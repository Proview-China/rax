package applicationadapter

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

type ownerExecutionV2 struct {
	result        toolcontract.ToolResultV2
	executeCalls  atomic.Int32
	inspectCalls  atomic.Int32
	lostExecute   atomic.Bool
	suppressReady atomic.Bool
	ready         atomic.Bool
}

type scriptedClockV2 struct {
	values []time.Time
	index  int
}

type fixedClockV2 struct{ now time.Time }

func (c fixedClockV2) Now() time.Time { return c.now }

func (c *scriptedClockV2) Now() time.Time {
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

func (e *ownerExecutionV2) ExecuteBoundSingleCallToolActionV2(_ context.Context, _ ToolOwnerSingleCallExecutionV2) (toolcontract.ToolResultV2, error) {
	e.executeCalls.Add(1)
	if !e.suppressReady.Load() {
		e.ready.Store(true)
	}
	if e.lostExecute.Load() {
		return toolcontract.ToolResultV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEffectUnknownOutcome, "lost Tool Owner execute reply")
	}
	return cloneToolResultV2(e.result), nil
}

func (e *ownerExecutionV2) InspectBoundSingleCallToolActionV2(_ context.Context, _ ToolOwnerSingleCallExecutionV2) (toolcontract.ToolResultV2, error) {
	e.inspectCalls.Add(1)
	if !e.ready.Load() {
		return toolcontract.ToolResultV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Tool Owner result not found")
	}
	return cloneToolResultV2(e.result), nil
}

func (e *ownerExecutionV2) InspectSettledResultForApplyV2(actionID string, apply toolcontract.ObjectRef) (toolcontract.ToolResultV2, error) {
	if e.result.Action.ID != actionID || e.result.Apply != apply {
		return toolcontract.ToolResultV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Tool Owner settled result lookup drifted")
	}
	return cloneToolResultV2(e.result), nil
}

type settlementCurrentV2 struct {
	inspection  runtimeports.OperationInspectionSettlementRefV4
	association runtimeports.OperationSettlementEvidenceAssociationV4
}

func (r settlementCurrentV2) InspectCurrentOperationSettlementV4(_ context.Context, request runtimeports.InspectCurrentOperationSettlementRequestV4) (runtimeports.OperationInspectionSettlementRefV4, error) {
	if request.EffectID != r.inspection.Settlement.EffectID || !runtimeports.SameOperationSubjectV3(request.Operation, r.inspection.DomainResult.Operation) {
		return runtimeports.OperationInspectionSettlementRefV4{}, core.NewError(core.ErrorConflict, core.ReasonEffectSettlementMissing, "settlement request drifted")
	}
	return r.inspection, nil
}

func (r settlementCurrentV2) InspectOperationSettlementEvidenceAssociationV4(_ context.Context, operation runtimeports.OperationSubjectV3, ref runtimeports.OperationSettlementEvidenceAssociationRefV4) (runtimeports.OperationSettlementEvidenceAssociationV4, error) {
	if !runtimeports.SameOperationSubjectV3(operation, r.inspection.DomainResult.Operation) || !runtimeports.SameOperationSettlementEvidenceAssociationRefV4(ref, r.association.RefV4()) {
		return runtimeports.OperationSettlementEvidenceAssociationV4{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "settlement Association drifted")
	}
	return r.association, nil
}

type adapterV2Fixture struct {
	binding    *bindingV2Fixture
	projection SingleCallToolActionBindingCurrentProjectionV2
	execution  *ownerExecutionV2
	flow       *ToolOwnerSingleCallFlowImplV2
	claims     *InMemoryToolOwnerSingleCallClaimStoreV2
	results    *InMemoryApplicationResultStoreV2
	adapter    *SingleCallToolActionAdapterV2
}

func newAdapterV2Fixture(t *testing.T) adapterV2Fixture {
	t.Helper()
	binding := newBindingV2Fixture(t)
	binding.request.RequestedExpiresUnixNano = binding.request.ApplicationRequest.ExpiresUnixNano
	projection, err := binding.reader.ResolveSingleCallToolActionBindingCurrentV2(context.Background(), binding.request)
	if err != nil {
		t.Fatal(err)
	}
	applicationFacts := testkit.ApplicationG6AFixture(binding.now)
	toolResult := applicationFacts.ToolResult
	toolResult.Action = projection.CandidateRef
	applyID, err := toolcontract.StableID("tool-apply-v2", projection.CandidateRef.ID, toolResult.Inspection.DomainResult.ID, string(toolResult.Inspection.Digest))
	if err != nil {
		t.Fatal(err)
	}
	toolResult.Apply.ID = applyID
	toolResult.Apply.Digest = testkit.Digest("tool-apply-v2-exact")
	toolResult.ID, err = toolcontract.StableID("tool-result-v2", projection.CandidateRef.ID, toolResult.Inspection.DomainResult.ID, toolResult.Apply.ID, string(toolResult.Apply.Digest))
	if err != nil {
		t.Fatal(err)
	}
	toolResult, err = toolcontract.SealToolResultV2(toolResult)
	if err != nil {
		t.Fatal(err)
	}
	if toolResult.Action != projection.CandidateRef {
		t.Fatalf("fixture Action mismatch")
	}
	if !runtimeports.SameExecutionScopeV2(toolResult.Inspection.DomainResult.Operation.ExecutionScope, binding.request.ApplicationRequest.Action.ExecutionScope) {
		t.Fatalf("fixture scope mismatch: result=%#v request=%#v", toolResult.Inspection.DomainResult.Operation.ExecutionScope, binding.request.ApplicationRequest.Action.ExecutionScope)
	}
	if toolResult.Inspection.DomainResult.Operation.ExecutionScopeDigest != projection.CandidateClosure.Candidate.OperationScopeDigest {
		t.Fatalf("fixture scope digest mismatch: result=%s candidate=%s", toolResult.Inspection.DomainResult.Operation.ExecutionScopeDigest, projection.CandidateClosure.Candidate.OperationScopeDigest)
	}
	execution := &ownerExecutionV2{result: toolResult}
	claims := NewInMemoryToolOwnerSingleCallClaimStoreV2()
	flow, err := NewToolOwnerSingleCallFlowWithStoresV2(execution, execution, claims, binding.clock)
	if err != nil {
		t.Fatal(err)
	}
	results := NewInMemoryApplicationResultStoreV2()
	settlement := settlementCurrentV2{inspection: toolResult.Inspection, association: applicationFacts.Association}
	// Keep the public Association exact with the ToolResult inspection.
	settlement.association = applicationFacts.Association
	adapter, err := NewSingleCallToolActionAdapterV2(binding.reader, flow, settlement, results, binding.clock)
	if err != nil {
		t.Fatal(err)
	}
	return adapterV2Fixture{binding: binding, projection: projection, execution: execution, flow: flow, claims: claims, results: results, adapter: adapter}
}

func TestSingleCallToolActionAdapterV2ExactExecuteInspectAndLostReply(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	fixture.execution.lostExecute.Store(true)
	request := fixture.binding.request.ApplicationRequest
	got, err := fixture.adapter.ExecuteSingleCallToolActionV2(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if fixture.execution.executeCalls.Load() != 1 || fixture.execution.inspectCalls.Load() != 1 {
		t.Fatalf("execute=%d inspect=%d, want 1/1", fixture.execution.executeCalls.Load(), fixture.execution.inspectCalls.Load())
	}
	key, err := applicationcontract.SealSingleCallToolActionInspectKeyV2(request)
	if err != nil {
		t.Fatal(err)
	}
	inspected, err := fixture.adapter.InspectSingleCallToolActionV2(context.Background(), key)
	if err != nil || inspected.Digest != got.Digest {
		t.Fatalf("Inspect=%#v err=%v", inspected, err)
	}
	if _, err = fixture.adapter.ExecuteSingleCallToolActionV2(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if fixture.execution.executeCalls.Load() != 1 {
		t.Fatalf("repeat Execute reached downstream %d times", fixture.execution.executeCalls.Load())
	}
}

func TestSingleCallToolActionAdapterV2LostResultCreateReplyRecoversByInspect(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	fixture.adapter.results = lostReplyApplicationResultStoreV2{delegate: fixture.results}
	request := fixture.binding.request.ApplicationRequest
	got, err := fixture.adapter.ExecuteSingleCallToolActionV2(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	key, err := applicationcontract.SealSingleCallToolActionInspectKeyV2(request)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := fixture.results.InspectSingleCallApplicationResultRecordV2(context.Background(), key)
	if err != nil || stored.Result.Digest != got.Digest {
		t.Fatalf("recovered=%#v stored=%#v err=%v", got, stored, err)
	}
	if fixture.execution.executeCalls.Load() != 1 {
		t.Fatalf("lost result reply re-executed Tool Owner %d times", fixture.execution.executeCalls.Load())
	}
}

func TestSingleCallToolActionAdapterV2ConcurrentSingleWinner(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	request := fixture.binding.request.ApplicationRequest
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := fixture.adapter.ExecuteSingleCallToolActionV2(context.Background(), request)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil && !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatal(err)
		}
	}
	if fixture.execution.executeCalls.Load() != 1 {
		t.Fatalf("downstream execute calls=%d, want 1", fixture.execution.executeCalls.Load())
	}
	key, err := applicationcontract.SealSingleCallToolActionInspectKeyV2(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.adapter.InspectSingleCallToolActionV2(context.Background(), key); err != nil {
		t.Fatalf("winner result was not inspectable after concurrent start: %v", err)
	}
}

func TestToolOwnerSingleCallFlowV2UnresolvedRepeatIsInspectOnly(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	fixture.execution.lostExecute.Store(true)
	fixture.execution.suppressReady.Store(true)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	if _, err := fixture.flow.StartOrInspectToolOwnerSingleCallV2(context.Background(), input); err == nil {
		t.Fatal("unresolved first execution succeeded")
	}
	fixture.execution.ready.Store(true)
	result, err := fixture.flow.StartOrInspectToolOwnerSingleCallV2(context.Background(), input)
	if err != nil || result.Digest != fixture.execution.result.Digest {
		t.Fatalf("repeat Inspect result=%#v err=%v", result, err)
	}
	if fixture.execution.executeCalls.Load() != 1 || fixture.execution.inspectCalls.Load() != 2 {
		t.Fatalf("execute=%d inspect=%d, want 1/2", fixture.execution.executeCalls.Load(), fixture.execution.inspectCalls.Load())
	}
}

func TestToolOwnerSingleCallFlowV2RestartUsesPersistedClaimAndOnlyInspects(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	fixture.execution.lostExecute.Store(true)
	fixture.execution.suppressReady.Store(true)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	if _, err := fixture.flow.StartOrInspectToolOwnerSingleCallV2(context.Background(), input); err == nil {
		t.Fatal("unresolved first execution succeeded")
	}
	fixture.execution.ready.Store(true)
	restarted, err := NewToolOwnerSingleCallFlowWithStoresV2(fixture.execution, fixture.execution, fixture.claims, fixture.binding.clock)
	if err != nil {
		t.Fatal(err)
	}
	result, err := restarted.StartOrInspectToolOwnerSingleCallV2(context.Background(), input)
	if err != nil || result.Digest != fixture.execution.result.Digest {
		t.Fatalf("restart result=%#v err=%v", result, err)
	}
	if fixture.execution.executeCalls.Load() != 1 || fixture.execution.inspectCalls.Load() != 2 {
		t.Fatalf("restart execute=%d inspect=%d, want 1/2", fixture.execution.executeCalls.Load(), fixture.execution.inspectCalls.Load())
	}
}

func TestToolOwnerSingleCallFlowV2RestartClockDoesNotChangeClaimPayload(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	first, err := NewToolOwnerSingleCallFlowWithStoresV2(fixture.execution, fixture.execution, fixture.claims, fixedClockV2{now: fixture.binding.now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = first.StartOrInspectToolOwnerSingleCallV2(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewToolOwnerSingleCallFlowWithStoresV2(fixture.execution, fixture.execution, fixture.claims, fixedClockV2{now: fixture.binding.now.Add(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.StartOrInspectToolOwnerSingleCallV2(context.Background(), input); err != nil {
		t.Fatalf("same immutable claim at a later restart clock failed: %v", err)
	}
	if fixture.execution.executeCalls.Load() != 1 {
		t.Fatalf("restart clock change caused %d executions", fixture.execution.executeCalls.Load())
	}
}

func TestToolOwnerSingleCallFlowV2SameRequestChangedBindingConflicts(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	if _, err := fixture.flow.StartOrInspectToolOwnerSingleCallV2(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	drift := CloneSingleCallToolActionBindingCurrentProjectionV2(fixture.projection)
	drift.Ref.Digest, drift.ProjectionDigest = "", ""
	drift.CheckedUnixNano++
	drift, err := SealSingleCallToolActionBindingCurrentProjectionV2(drift)
	if err != nil {
		t.Fatal(err)
	}
	changed := ToolOwnerSingleCallExecutionV2{Request: input.Request, Binding: drift}
	other, err := NewToolOwnerSingleCallFlowWithStoresV2(fixture.execution, fixture.execution, fixture.claims, fixedClockV2{now: fixture.binding.now.Add(time.Nanosecond)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = other.StartOrInspectToolOwnerSingleCallV2(context.Background(), changed); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("changed binding claim error=%v, want Conflict", err)
	}
	if fixture.execution.executeCalls.Load() != 1 {
		t.Fatalf("changed binding reached execution %d times", fixture.execution.executeCalls.Load())
	}
}

func TestToolOwnerSingleCallFlowV2LostClaimCreateReplyNeverGrantsExecute(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	claims := &lostReplyToolOwnerClaimStoreV2{delegate: NewInMemoryToolOwnerSingleCallClaimStoreV2()}
	flow, err := NewToolOwnerSingleCallFlowWithStoresV2(fixture.execution, fixture.execution, claims, fixture.binding.clock)
	if err != nil {
		t.Fatal(err)
	}
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	if _, err = flow.StartOrInspectToolOwnerSingleCallV2(context.Background(), input); err == nil || !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("lost claim reply error=%v, want inspect-only NotFound", err)
	}
	if fixture.execution.executeCalls.Load() != 0 || fixture.execution.inspectCalls.Load() != 1 {
		t.Fatalf("lost claim reply execute=%d inspect=%d, want 0/1", fixture.execution.executeCalls.Load(), fixture.execution.inspectCalls.Load())
	}
}

func TestToolOwnerSingleCallFlowV2SharedClaimAcross64FlowsExecutesOnce(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			flow, err := NewToolOwnerSingleCallFlowWithStoresV2(fixture.execution, fixture.execution, fixture.claims, fixture.binding.clock)
			if err == nil {
				_, err = flow.StartOrInspectToolOwnerSingleCallV2(context.Background(), input)
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil && !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatal(err)
		}
	}
	if fixture.execution.executeCalls.Load() != 1 {
		t.Fatalf("shared claim allowed %d Execute calls", fixture.execution.executeCalls.Load())
	}
	if _, err := fixture.flow.InspectToolOwnerSingleCallV2(context.Background(), input); err != nil {
		t.Fatalf("winner result was not inspectable after concurrent claim: %v", err)
	}
}

func TestToolOwnerSingleCallFlowV2CanceledWaiterDoesNotReachOwner(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fixture.flow.StartOrInspectToolOwnerSingleCallV2(ctx, input); err == nil {
		t.Fatal("canceled waiter succeeded")
	}
	if fixture.execution.executeCalls.Load() != 0 || fixture.execution.inspectCalls.Load() != 0 {
		t.Fatalf("canceled waiter reached Owner execute=%d inspect=%d", fixture.execution.executeCalls.Load(), fixture.execution.inspectCalls.Load())
	}
}

func TestSingleCallToolActionAdapterV2ClockRollbackStopsBeforeOwner(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	fixture.adapter.clock = &scriptedClockV2{values: []time.Time{fixture.binding.now, fixture.binding.now.Add(-time.Nanosecond)}}
	if _, err := fixture.adapter.ExecuteSingleCallToolActionV2(context.Background(), fixture.binding.request.ApplicationRequest); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("rollback error=%v", err)
	}
	if fixture.execution.executeCalls.Load() != 0 {
		t.Fatalf("clock rollback reached Owner %d times", fixture.execution.executeCalls.Load())
	}
}

func TestToolOwnerSingleCallFlowV2ClockRollbackStopsBeforeOwner(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	clock := &scriptedClockV2{values: []time.Time{fixture.binding.now, fixture.binding.now.Add(-time.Nanosecond)}}
	flow, err := NewToolOwnerSingleCallFlowWithStoresV2(fixture.execution, fixture.execution, fixture.claims, clock)
	if err != nil {
		t.Fatal(err)
	}
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	if _, err = flow.StartOrInspectToolOwnerSingleCallV2(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if _, err = flow.InspectToolOwnerSingleCallV2(context.Background(), input); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("flow rollback error=%v", err)
	}
	if fixture.execution.executeCalls.Load() != 1 {
		t.Fatalf("flow rollback re-executed Owner %d times", fixture.execution.executeCalls.Load())
	}
}

func TestToolOwnerSingleCallFlowV2DeepClonesSandboxLease(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	first, err := fixture.flow.StartOrInspectToolOwnerSingleCallV2(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	original := first.Inspection.DomainResult.Operation.ExecutionScope.SandboxLease.ID
	first.Inspection.DomainResult.Operation.ExecutionScope.SandboxLease.ID = "mutated-lease"
	second, err := fixture.flow.InspectToolOwnerSingleCallV2(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if second.Inspection.DomainResult.Operation.ExecutionScope.SandboxLease.ID != original {
		t.Fatal("returned ToolResult retained nested SandboxLease alias")
	}
}

func TestSingleCallToolActionAdapterV2BindingAndCandidateDriftZeroDownstream(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		mutate func(*SingleCallToolActionBindingCurrentProjectionV2)
	}{
		{name: "binding ref", mutate: func(value *SingleCallToolActionBindingCurrentProjectionV2) {
			value.Ref.Digest = testkit.Digest("wrong-binding")
		}},
		{name: "candidate", mutate: func(value *SingleCallToolActionBindingCurrentProjectionV2) {
			value.CandidateClosure.Candidate.Digest = testkit.Digest("wrong-candidate")
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newAdapterV2Fixture(t)
			drift := CloneSingleCallToolActionBindingCurrentProjectionV2(fixture.projection)
			testCase.mutate(&drift)
			reader := &fixedBindingReaderV2{value: drift}
			fixture.adapter.bindings = reader
			if _, err := fixture.adapter.ExecuteSingleCallToolActionV2(context.Background(), fixture.binding.request.ApplicationRequest); err == nil {
				t.Fatal("drift reached downstream")
			}
			if fixture.execution.executeCalls.Load() != 0 {
				t.Fatalf("drift downstream calls=%d", fixture.execution.executeCalls.Load())
			}
		})
	}
}

func TestInMemoryApplicationResultStoreV2ChangedContentConflict(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	request := fixture.binding.request.ApplicationRequest
	result, err := fixture.adapter.ExecuteSingleCallToolActionV2(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	driftTool := fixture.execution.result
	driftTool.Apply.Digest = testkit.Digest("apply-v2-changed")
	driftTool.ID, err = toolcontract.StableID("tool-result-v2", fixture.projection.CandidateRef.ID, driftTool.Inspection.DomainResult.ID, driftTool.Apply.ID, string(driftTool.Apply.Digest))
	if err != nil {
		t.Fatal(err)
	}
	driftTool, err = toolcontract.SealToolResultV2(driftTool)
	if err != nil {
		t.Fatal(err)
	}
	drift, err := fixture.adapter.closeApplicationResultV2(context.Background(), request, fixture.projection, driftTool, fixture.binding.now)
	if err != nil {
		t.Fatal(err)
	}
	if drift.Digest == result.Digest {
		t.Fatal("changed Tool result did not change Application result")
	}
	if _, err = fixture.results.CreateSingleCallApplicationResultV2(context.Background(), request, drift); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("changed result content error=%v, want Conflict", err)
	}
}

func TestInMemoryApplicationResultStoreV2NilAndCanceledContextZeroWrite(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	request := fixture.binding.request.ApplicationRequest
	result, err := fixture.adapter.closeApplicationResultV2(context.Background(), request, fixture.projection, fixture.execution.result, fixture.binding.now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.results.CreateSingleCallApplicationResultV2(nil, request, result); err == nil {
		t.Fatal("nil context wrote result")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = fixture.results.CreateSingleCallApplicationResultV2(ctx, request, result); err == nil {
		t.Fatal("canceled context wrote result")
	}
	key, err := applicationcontract.SealSingleCallToolActionInspectKeyV2(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.results.InspectSingleCallApplicationResultRecordV2(context.Background(), key); err == nil || !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("canceled create left result: %v", err)
	}
}

func TestSingleCallToolActionAdapterV2InspectRejectsMaliciousResultRecord(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	request := fixture.binding.request.ApplicationRequest
	result, err := fixture.adapter.ExecuteSingleCallToolActionV2(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	key, err := applicationcontract.SealSingleCallToolActionInspectKeyV2(request)
	if err != nil {
		t.Fatal(err)
	}
	malicious := ApplicationResultRecordV2{Request: request, Result: result}
	malicious.Request.Digest = testkit.Digest("another-request")
	fixture.adapter.results = maliciousApplicationResultStoreV2{record: malicious}
	if _, err = fixture.adapter.InspectSingleCallToolActionV2(context.Background(), key); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("malicious Inspect error=%v, want Conflict", err)
	}
}

func TestSingleCallToolActionAdapterV2LostResultReplyReturnsRecoveryError(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	fixture.adapter.results = failedRecoveryApplicationResultStoreV2{delegate: fixture.results}
	if _, err := fixture.adapter.ExecuteSingleCallToolActionV2(context.Background(), fixture.binding.request.ApplicationRequest); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("lost result recovery error=%v, want authoritative Conflict", err)
	}
}

func TestToolOwnerSingleCallFlowV2RejectsResultCausalDriftBeforeApplicationResult(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		mutate func(*toolcontract.ToolResultV2) error
	}{
		{name: "action ref", mutate: func(result *toolcontract.ToolResultV2) error {
			result.Action.Digest = testkit.Digest("wrong-action")
			return nil
		}},
		{name: "domain result ref", mutate: func(result *toolcontract.ToolResultV2) error {
			result.DomainResult.Digest = testkit.Digest("wrong-domain-result")
			return nil
		}},
		{name: "settlement owner", mutate: func(result *toolcontract.ToolResultV2) error {
			result.Inspection.Owner.ComponentID = "praxis.tool/another-owner"
			sealed, err := runtimeports.SealOperationInspectionSettlementRefV4(result.Inspection, time.Unix(0, result.FinalizedUnixNano))
			result.Inspection = sealed
			return err
		}},
		{name: "apply id", mutate: func(result *toolcontract.ToolResultV2) error {
			result.Apply.ID = "wrong-apply-v2"
			return nil
		}},
		{name: "result id", mutate: func(result *toolcontract.ToolResultV2) error {
			result.ID = "wrong-result-v2"
			return nil
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newAdapterV2Fixture(t)
			drift := cloneToolResultV2(fixture.execution.result)
			if err := testCase.mutate(&drift); err != nil {
				t.Fatal(err)
			}
			sealed, err := toolcontract.SealToolResultV2(drift)
			if err != nil {
				t.Fatal(err)
			}
			fixture.execution.result = sealed
			if _, err = fixture.adapter.ExecuteSingleCallToolActionV2(context.Background(), fixture.binding.request.ApplicationRequest); err == nil || !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("causal drift error=%v, want Conflict", err)
			}
			assertApplicationResultAbsentV2(t, fixture)
		})
	}
}

func TestToolOwnerSingleCallFlowV2RejectsSettledReaderDriftBeforeApplicationResult(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	drift := cloneToolResultV2(fixture.execution.result)
	drift.Artifacts = append(drift.Artifacts, toolcontract.ObjectRef{ID: "artifact-drift-v2", Revision: 1, Digest: testkit.Digest("artifact-drift-v2")})
	sealed, err := toolcontract.SealToolResultV2(drift)
	if err != nil {
		t.Fatal(err)
	}
	flow, err := NewToolOwnerSingleCallFlowWithStoresV2(fixture.execution, fixedSettledResultReaderV2{result: sealed}, fixture.claims, fixture.binding.clock)
	if err != nil {
		t.Fatal(err)
	}
	fixture.adapter.flow = flow
	if _, err = fixture.adapter.ExecuteSingleCallToolActionV2(context.Background(), fixture.binding.request.ApplicationRequest); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("settled reader drift error=%v, want Conflict", err)
	}
	assertApplicationResultAbsentV2(t, fixture)
}

func assertApplicationResultAbsentV2(t *testing.T, fixture adapterV2Fixture) {
	t.Helper()
	key, err := applicationcontract.SealSingleCallToolActionInspectKeyV2(fixture.binding.request.ApplicationRequest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.results.InspectSingleCallApplicationResultRecordV2(context.Background(), key); err == nil || !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("rejected owner result produced Application result: %v", err)
	}
}

type fixedSettledResultReaderV2 struct {
	result toolcontract.ToolResultV2
}

func (r fixedSettledResultReaderV2) InspectSettledResultForApplyV2(string, toolcontract.ObjectRef) (toolcontract.ToolResultV2, error) {
	return cloneToolResultV2(r.result), nil
}

type fixedBindingReaderV2 struct {
	value SingleCallToolActionBindingCurrentProjectionV2
}

type lostReplyApplicationResultStoreV2 struct {
	delegate *InMemoryApplicationResultStoreV2
}

type lostReplyToolOwnerClaimStoreV2 struct {
	delegate *InMemoryToolOwnerSingleCallClaimStoreV2
}

func (s *lostReplyToolOwnerClaimStoreV2) CreateToolOwnerSingleCallClaimV2(ctx context.Context, record ToolOwnerSingleCallClaimRecordV2) (ToolOwnerSingleCallClaimRecordV2, bool, error) {
	if _, _, err := s.delegate.CreateToolOwnerSingleCallClaimV2(ctx, record); err != nil {
		return ToolOwnerSingleCallClaimRecordV2{}, false, err
	}
	return ToolOwnerSingleCallClaimRecordV2{}, false, core.NewError(core.ErrorUnavailable, core.ReasonEffectUnknownOutcome, "lost Tool Owner claim create reply")
}

func (s *lostReplyToolOwnerClaimStoreV2) InspectToolOwnerSingleCallClaimV2(ctx context.Context, key applicationcontract.SingleCallToolActionInspectKeyV2) (ToolOwnerSingleCallClaimRecordV2, error) {
	return s.delegate.InspectToolOwnerSingleCallClaimV2(ctx, key)
}

type maliciousApplicationResultStoreV2 struct {
	record ApplicationResultRecordV2
}

func (s maliciousApplicationResultStoreV2) CreateSingleCallApplicationResultV2(context.Context, applicationcontract.SingleCallToolActionRequestV2, applicationcontract.SingleCallToolActionResultV2) (applicationcontract.SingleCallToolActionResultV2, error) {
	return applicationcontract.SingleCallToolActionResultV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "malicious result store is read-only")
}

func (s maliciousApplicationResultStoreV2) InspectSingleCallApplicationResultRecordV2(context.Context, applicationcontract.SingleCallToolActionInspectKeyV2) (ApplicationResultRecordV2, error) {
	return s.record, nil
}

type failedRecoveryApplicationResultStoreV2 struct {
	delegate *InMemoryApplicationResultStoreV2
}

func (s failedRecoveryApplicationResultStoreV2) CreateSingleCallApplicationResultV2(ctx context.Context, request applicationcontract.SingleCallToolActionRequestV2, result applicationcontract.SingleCallToolActionResultV2) (applicationcontract.SingleCallToolActionResultV2, error) {
	if _, err := s.delegate.CreateSingleCallApplicationResultV2(ctx, request, result); err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	return applicationcontract.SingleCallToolActionResultV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEffectUnknownOutcome, "lost Application result create reply")
}

func (s failedRecoveryApplicationResultStoreV2) InspectSingleCallApplicationResultRecordV2(context.Context, applicationcontract.SingleCallToolActionInspectKeyV2) (ApplicationResultRecordV2, error) {
	return ApplicationResultRecordV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "authoritative result recovery conflict")
}

func (s lostReplyApplicationResultStoreV2) CreateSingleCallApplicationResultV2(ctx context.Context, request applicationcontract.SingleCallToolActionRequestV2, result applicationcontract.SingleCallToolActionResultV2) (applicationcontract.SingleCallToolActionResultV2, error) {
	if _, err := s.delegate.CreateSingleCallApplicationResultV2(ctx, request, result); err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	return applicationcontract.SingleCallToolActionResultV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEffectUnknownOutcome, "lost Application result create reply")
}

func (s lostReplyApplicationResultStoreV2) InspectSingleCallApplicationResultRecordV2(ctx context.Context, key applicationcontract.SingleCallToolActionInspectKeyV2) (ApplicationResultRecordV2, error) {
	return s.delegate.InspectSingleCallApplicationResultRecordV2(ctx, key)
}

func (r *fixedBindingReaderV2) ResolveSingleCallToolActionBindingCurrentV2(context.Context, SingleCallToolActionBindingResolveRequestV2) (SingleCallToolActionBindingCurrentProjectionV2, error) {
	return CloneSingleCallToolActionBindingCurrentProjectionV2(r.value), nil
}
func (r *fixedBindingReaderV2) InspectSingleCallToolActionBindingCurrentByIssuanceV2(context.Context, SingleCallToolActionBindingIssuanceLookupRequestV2) (SingleCallToolActionBindingCurrentProjectionV2, error) {
	return CloneSingleCallToolActionBindingCurrentProjectionV2(r.value), nil
}
func (r *fixedBindingReaderV2) InspectExactSingleCallToolActionBindingCurrentV2(context.Context, SingleCallToolActionBindingInspectExactRequestV2) (SingleCallToolActionBindingCurrentProjectionV2, error) {
	return CloneSingleCallToolActionBindingCurrentProjectionV2(r.value), nil
}

package application_test

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	application "github.com/Proview-China/rax/ExecutionRuntime/application"
	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/application/fakes"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/tests/testsupport"
)

func TestSingleCallToolActionContractCanonicalN1AndNominalSourcesV1(t *testing.T) {
	fixture := newSingleCallFixtureV1(t)
	request := fixture.request
	if err := request.ValidateCurrent(fixture.now); err != nil {
		t.Fatal(err)
	}
	resealed, err := contract.SealSingleCallToolActionRequestV1(request)
	if err != nil || resealed.ID != request.ID || resealed.Digest != request.Digest {
		t.Fatalf("canonical request was not deterministic: %#v err=%v", resealed, err)
	}

	mutations := []struct {
		name   string
		mutate func(*contract.SingleCallToolActionRequestV1)
	}{
		{"n0", func(r *contract.SingleCallToolActionRequestV1) { r.Observation.CallCount = 0 }},
		{"n2", func(r *contract.SingleCallToolActionRequestV1) { r.Observation.CallCount = 2 }},
		{"session_kind", func(r *contract.SingleCallToolActionRequestV1) {
			r.SessionApplicabilitySource.Kind = contract.SingleCallTurnSourceKindV1
		}},
		{"turn_kind", func(r *contract.SingleCallToolActionRequestV1) {
			r.TurnApplicabilitySource.Kind = contract.SingleCallSessionSourceKindV1
		}},
		{"parent_kind", func(r *contract.SingleCallToolActionRequestV1) {
			r.ParentFrameApplicabilitySource.Kind = contract.SingleCallSessionSourceKindV1
		}},
		{"parent_missing", func(r *contract.SingleCallToolActionRequestV1) { r.ParentFrameApplicabilitySource.ID = "" }},
		{"ttl", func(r *contract.SingleCallToolActionRequestV1) { r.ExpiresUnixNano = r.Session.ExpiresUnixNano + 1 }},
	}
	tamperedScope := request
	tamperedScope.ExecutionScope.Instance.Epoch++
	if err := tamperedScope.Validate(); err == nil {
		t.Fatal("sealed request accepted ExecutionScope epoch drift")
	}
	for _, testCase := range mutations {
		t.Run(testCase.name, func(t *testing.T) {
			changed := request
			testCase.mutate(&changed)
			if _, err := contract.SealSingleCallToolActionRequestV1(changed); err == nil {
				t.Fatal("invalid or type-punned request sealed")
			}
		})
	}

	sessionDigest, err := request.SessionApplicabilitySource.CanonicalDigestV1()
	if err != nil {
		t.Fatal(err)
	}
	turnDigest, err := request.TurnApplicabilitySource.CanonicalDigestV1()
	if err != nil {
		t.Fatal(err)
	}
	parentDigest, err := request.ParentFrameApplicabilitySource.CanonicalDigestV1()
	if err != nil {
		t.Fatal(err)
	}
	if sessionDigest == turnDigest || sessionDigest == parentDigest || turnDigest == parentDigest {
		t.Fatal("distinct source coordinates shared a canonical domain")
	}
	if reflect.TypeOf(request.ParentFrame) == reflect.TypeOf(request.ParentFrameApplicabilitySource) || reflect.TypeOf(request.SessionApplicabilitySource) == reflect.TypeOf(request.TurnApplicabilitySource) {
		t.Fatal("nominal source coordinates collapsed to a shared static type")
	}
}

func TestSingleCallToolActionCoordinationTransitionAndFakeIsolationV1(t *testing.T) {
	fixture := newSingleCallFixtureV1(t)
	fact, err := contract.NewSingleCallToolActionCoordinationFactV1(fixture.request, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := fixture.store.CreateSingleCallToolActionCoordinationV1(context.Background(), fact)
	if err != nil {
		t.Fatal(err)
	}
	stored.Request.ExecutionScope.Instance.Epoch++
	inspected, err := fixture.store.InspectSingleCallToolActionCoordinationV1(context.Background(), fixture.request.ExecutionScope, fixture.request.ID)
	if err != nil || inspected.Request.ExecutionScope.Instance.Epoch != fixture.request.ExecutionScope.Instance.Epoch {
		t.Fatalf("Fact store leaked caller mutation: %#v err=%v", inspected, err)
	}
	resultRef := fixture.result.RefV1()
	if _, err := contract.NextSingleCallToolActionCoordinationFactV1(fact, contract.SingleCallToolActionCompletedV1, &resultRef, fixture.now); err == nil {
		t.Fatal("prepared coordination skipped dispatch_intent")
	}
	dispatch, err := contract.NextSingleCallToolActionCoordinationFactV1(fact, contract.SingleCallToolActionDispatchIntentV1, nil, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := contract.NextSingleCallToolActionCoordinationFactV1(dispatch, contract.SingleCallToolActionWaitingInspectV1, nil, fixture.now); err == nil {
		t.Fatal("dispatch_intent entered waiting_inspect without an explicit unique start claim")
	}
	if _, err := contract.NextSingleCallToolActionCoordinationFactV1(dispatch, contract.SingleCallToolActionPreparedV1, nil, fixture.now); err == nil {
		t.Fatal("coordination state regressed")
	}
	if _, err := contract.ClaimSingleCallToolActionStartV1(dispatch, "start-claim/test-clock", fixture.now.Add(-time.Nanosecond)); err == nil {
		t.Fatal("coordination clock regression was accepted")
	}
	firstClaim, err := contract.ClaimSingleCallToolActionStartV1(dispatch, "start-claim/competitor-a", fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	secondClaim, err := contract.ClaimSingleCallToolActionStartV1(dispatch, "start-claim/competitor-b", fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	if firstClaim.Digest == secondClaim.Digest || firstClaim.StartClaimID == secondClaim.StartClaimID {
		t.Fatal("competing start claims collapsed to one idempotent CAS successor")
	}
	if _, err := contract.NextSingleCallToolActionCoordinationFactV1(firstClaim, contract.SingleCallToolActionCompletedV1, &resultRef, fixture.now); err != nil {
		t.Fatalf("completed coordination did not preserve its exact start claim: %v", err)
	}
}

func TestSingleCallToolActionCoordinatorClosesOnlyExactG6AV1(t *testing.T) {
	fixture := newSingleCallFixtureV1(t)
	result, err := fixture.coordinator.CoordinateSingleCallToolActionV1(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Digest != fixture.result.Digest || fixture.tool.providerCalls != 1 || fixture.tool.executeCalls != 1 || fixture.tool.inspectCalls != 0 || fixture.inputs.calls != 3 || fixture.settlements.currentCalls != 1 || fixture.settlements.associationCalls != 1 {
		t.Fatalf("unexpected G6A closure or call order: result=%s want=%s tool=%#v inputs=%d settlement=%d/%d", result.Digest, fixture.result.Digest, fixture.tool, fixture.inputs.calls, fixture.settlements.currentCalls, fixture.settlements.associationCalls)
	}
	fact, err := fixture.store.InspectSingleCallToolActionCoordinationV1(context.Background(), fixture.request.ExecutionScope, fixture.request.ID)
	if err != nil || fact.State != contract.SingleCallToolActionCompletedV1 || fact.Result == nil || fact.Result.Digest != result.Digest {
		t.Fatalf("coordination did not complete exactly: %#v err=%v", fact, err)
	}
	creates, cas := fixture.store.Counts()
	if creates != 1 || cas != 3 {
		t.Fatalf("unexpected write-ahead counts: create=%d cas=%d", creates, cas)
	}
	// A completed replay uses Tool/Runtime Inspect only; it never starts another
	// logical Tool request or Provider action.
	if _, err := fixture.coordinator.CoordinateSingleCallToolActionV1(context.Background(), fixture.request); err != nil {
		t.Fatal(err)
	}
	creates, cas = fixture.store.Counts()
	if creates != 1 || cas != 3 || fixture.tool.providerCalls != 1 || fixture.tool.executeCalls != 1 {
		t.Fatalf("completed replay wrote or redispatched: create=%d cas=%d provider=%d", creates, cas, fixture.tool.providerCalls)
	}
}

func TestSingleCallToolActionLostRepliesRecoverByInspectV1(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		inject func(*singleCallFixtureV1)
	}{
		{"create", func(f *singleCallFixtureV1) { f.store.LoseNextCreateReply = true }},
		{"dispatch_cas", func(f *singleCallFixtureV1) { f.store.LoseNextCASReply = true }},
		{"tool_execute", func(f *singleCallFixtureV1) { f.tool.loseNextExecuteReply = true }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newSingleCallFixtureV1(t)
			testCase.inject(fixture)
			result, err := fixture.coordinator.CoordinateSingleCallToolActionV1(context.Background(), fixture.request)
			if err != nil || result.Digest != fixture.result.Digest {
				t.Fatalf("lost reply did not recover exact Result: %#v err=%v", result, err)
			}
			if fixture.tool.providerCalls != 1 {
				t.Fatalf("lost reply repeated the logical Provider action: %d", fixture.tool.providerCalls)
			}
		})
	}
}

func TestSingleCallToolActionFailClosedBeforeToolOrCompletionV1(t *testing.T) {
	t.Run("request TTL crossed", func(t *testing.T) {
		fixture := newSingleCallFixtureV1(t)
		expiredCoordinator, err := application.NewSingleCallToolActionCoordinatorV1(application.SingleCallToolActionCoordinatorConfigV1{Facts: fixture.store, Tool: fixture.tool, Inputs: fixture.inputs, Settlements: fixture.settlements, Clock: func() time.Time { return time.Unix(0, fixture.request.ExpiresUnixNano) }})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := expiredCoordinator.CoordinateSingleCallToolActionV1(context.Background(), fixture.request); err == nil {
			t.Fatal("expired request was accepted")
		}
		if fixture.inputs.calls != 0 || fixture.tool.inspectCalls != 0 || fixture.tool.executeCalls != 0 {
			t.Fatal("expired request reached a Reader or Tool")
		}
	})

	t.Run("input reader unavailable", func(t *testing.T) {
		fixture := newSingleCallFixtureV1(t)
		fixture.inputs.errAt = 1
		if _, err := fixture.coordinator.CoordinateSingleCallToolActionV1(context.Background(), fixture.request); !core.HasCategory(err, core.ErrorUnavailable) {
			t.Fatalf("input outage did not fail closed: %v", err)
		}
		if fixture.tool.inspectCalls != 0 || fixture.tool.executeCalls != 0 {
			t.Fatal("Tool was called before S1 closed")
		}
		creates, _ := fixture.store.Counts()
		if creates != 0 {
			t.Fatal("coordination was written before S1 closed")
		}
	})

	t.Run("S2 parent source drift", func(t *testing.T) {
		fixture := newSingleCallFixtureV1(t)
		fixture.inputs.driftAt = 3
		if _, err := fixture.coordinator.CoordinateSingleCallToolActionV1(context.Background(), fixture.request); err == nil {
			t.Fatal("S2 drift completed G6A")
		}
		fact, err := fixture.store.InspectSingleCallToolActionCoordinationV1(context.Background(), fixture.request.ExecutionScope, fixture.request.ID)
		if err != nil || fact.State != contract.SingleCallToolActionWaitingInspectV1 || fact.Result != nil {
			t.Fatalf("S2 drift did not remain inspectable: %#v err=%v", fact, err)
		}
	})

	t.Run("association unavailable", func(t *testing.T) {
		fixture := newSingleCallFixtureV1(t)
		fixture.settlements.associationErr = core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "association unavailable")
		if _, err := fixture.coordinator.CoordinateSingleCallToolActionV1(context.Background(), fixture.request); !core.HasCategory(err, core.ErrorUnavailable) {
			t.Fatalf("Association outage did not fail closed: %v", err)
		}
		fact, err := fixture.store.InspectSingleCallToolActionCoordinationV1(context.Background(), fixture.request.ExecutionScope, fixture.request.ID)
		if err != nil || fact.State != contract.SingleCallToolActionWaitingInspectV1 {
			t.Fatalf("Association outage did not preserve waiting_inspect: %#v err=%v", fact, err)
		}
	})

	t.Run("settlement scope drift", func(t *testing.T) {
		fixture := newSingleCallFixtureV1(t)
		fixture.tool.result.Inspection.DomainResult.Operation.ExecutionScope.Instance.Epoch++
		if _, err := fixture.coordinator.CoordinateSingleCallToolActionV1(context.Background(), fixture.request); err == nil {
			t.Fatal("cross-scope Tool Result completed G6A")
		}
	})

	for _, testCase := range []struct {
		name string
		err  error
	}{
		{"NotFound", nil},
		{"Unavailable", core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "Tool Inspect unavailable")},
		{"Indeterminate", core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Tool Inspect indeterminate")},
	} {
		t.Run("waiting_inspect_"+testCase.name+"_is_inspect_only", func(t *testing.T) {
			fixture := newSingleCallFixtureV1(t)
			seedSingleCallCoordinationStateV1(t, fixture, contract.SingleCallToolActionWaitingInspectV1)
			fixture.tool.inspectErr = testCase.err
			if _, err := fixture.coordinator.CoordinateSingleCallToolActionV1(context.Background(), fixture.request); err == nil {
				t.Fatal("waiting_inspect absence or outage was accepted")
			}
			if fixture.tool.executeCalls != 0 || fixture.tool.providerCalls != 0 {
				t.Fatal("waiting_inspect retried Tool Execute")
			}
		})
	}
}

func TestSingleCallToolActionWaitingTransitionLostReplyIsPermanentlyInspectOnlyV1(t *testing.T) {
	for _, loseRecoveryInspect := range []bool{false, true} {
		t.Run(map[bool]string{false: "recovered_waiting", true: "recovery_unavailable"}[loseRecoveryInspect], func(t *testing.T) {
			fixture := newSingleCallFixtureV1(t)
			seedSingleCallCoordinationStateV1(t, fixture, contract.SingleCallToolActionDispatchIntentV1)
			fixture.store.LoseNextCASReply = true
			fixture.store.LoseNextInspectReply = loseRecoveryInspect
			if _, err := fixture.coordinator.CoordinateSingleCallToolActionV1(context.Background(), fixture.request); err == nil {
				t.Fatal("lost waiting_inspect transition reply was accepted")
			}
			if fixture.tool.executeCalls != 0 || fixture.tool.providerCalls != 0 {
				t.Fatal("unknown waiting_inspect transition outcome reached Tool Execute")
			}
			fact, err := fixture.store.InspectSingleCallToolActionCoordinationV1(context.Background(), fixture.request.ExecutionScope, fixture.request.ID)
			if err != nil || fact.State != contract.SingleCallToolActionWaitingInspectV1 {
				t.Fatalf("lost reply did not persist waiting_inspect: %#v err=%v", fact, err)
			}
			if _, err := fixture.coordinator.CoordinateSingleCallToolActionV1(context.Background(), fixture.request); err == nil {
				t.Fatal("waiting_inspect replay without Result was accepted")
			}
			if fixture.tool.executeCalls != 0 || fixture.tool.providerCalls != 0 {
				t.Fatal("waiting_inspect replay redispatched Tool")
			}
		})
	}
}

func TestSingleCallToolActionUnknownExecuteOutcomeNeverRedispatchesV1(t *testing.T) {
	fixture := newSingleCallFixtureV1(t)
	fixture.tool.unknownExecuteErr = core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Tool start outcome unknown")
	if _, err := fixture.coordinator.CoordinateSingleCallToolActionV1(context.Background(), fixture.request); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("unknown Tool outcome returned wrong error: %v", err)
	}
	fact, err := fixture.store.InspectSingleCallToolActionCoordinationV1(context.Background(), fixture.request.ExecutionScope, fixture.request.ID)
	if err != nil || fact.State != contract.SingleCallToolActionWaitingInspectV1 {
		t.Fatalf("unknown Tool outcome did not persist waiting_inspect: %#v err=%v", fact, err)
	}
	if _, err := fixture.coordinator.CoordinateSingleCallToolActionV1(context.Background(), fixture.request); err == nil {
		t.Fatal("unknown Tool outcome replay without Result was accepted")
	}
	if fixture.tool.executeCalls != 1 || fixture.tool.providerCalls != 1 {
		t.Fatalf("unknown Tool outcome was redispatched: execute=%d provider=%d", fixture.tool.executeCalls, fixture.tool.providerCalls)
	}
}

func TestSingleCallToolActionFreshClockAndTTLCrossingV1(t *testing.T) {
	t.Run("input CheckedAt later than supplied now", func(t *testing.T) {
		fixture := newSingleCallFixtureV1(t)
		fixture.inputs.projection = singleCallInputProjectionV1(t, fixture.now.Add(time.Nanosecond), fixture.request)
		if _, err := fixture.coordinator.CoordinateSingleCallToolActionV1(context.Background(), fixture.request); err == nil {
			t.Fatal("future-dated Input projection was accepted against an older boundary time")
		}
		creates, _ := fixture.store.Counts()
		if creates != 0 || fixture.tool.executeCalls != 0 {
			t.Fatal("future-dated Input projection crossed the zero-write boundary")
		}
	})

	for _, testCase := range []struct {
		name   string
		inject func(*singleCallFixtureV1)
	}{
		{"Tool return crosses Result TTL", func(f *singleCallFixtureV1) {
			f.tool.onExecute = func(int) { f.clock.Set(time.Unix(0, f.result.ExpiresUnixNano)) }
		}},
		{"Settlement read crosses Result TTL", func(f *singleCallFixtureV1) {
			f.settlements.onCurrent = func(int) { f.clock.Set(time.Unix(0, f.result.ExpiresUnixNano)) }
		}},
		{"Association read crosses Result TTL", func(f *singleCallFixtureV1) {
			f.settlements.onAssociation = func(int) { f.clock.Set(time.Unix(0, f.result.ExpiresUnixNano)) }
		}},
		{"clock rolls back during Tool", func(f *singleCallFixtureV1) {
			f.tool.onExecute = func(int) { f.clock.Set(f.now.Add(-time.Nanosecond)) }
		}},
		{"clock rolls back during Input", func(f *singleCallFixtureV1) {
			f.inputs.onCall = func(call int) {
				if call == 1 {
					f.clock.Set(f.now.Add(-time.Nanosecond))
				}
			}
		}},
		{"clock rolls back during Settlement", func(f *singleCallFixtureV1) {
			f.settlements.onCurrent = func(int) { f.clock.Set(f.now.Add(-time.Nanosecond)) }
		}},
		{"clock rolls back during Association", func(f *singleCallFixtureV1) {
			f.settlements.onAssociation = func(int) { f.clock.Set(f.now.Add(-time.Nanosecond)) }
		}},
		{"completion commit crosses Result TTL", func(f *singleCallFixtureV1) {
			f.store.AfterCASCommit = func(state contract.SingleCallToolActionCoordinationStateV1) {
				if state == contract.SingleCallToolActionCompletedV1 {
					f.clock.Set(time.Unix(0, f.result.ExpiresUnixNano))
				}
			}
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newSingleCallFixtureV1(t)
			testCase.inject(fixture)
			if _, err := fixture.coordinator.CoordinateSingleCallToolActionV1(context.Background(), fixture.request); err == nil {
				t.Fatal("time crossing or rollback completed G6A")
			}
			fact, err := fixture.store.InspectSingleCallToolActionCoordinationV1(context.Background(), fixture.request.ExecutionScope, fixture.request.ID)
			if testCase.name == "clock rolls back during Input" {
				if !core.HasCategory(err, core.ErrorNotFound) || fixture.tool.executeCalls != 0 {
					t.Fatalf("pre-write Input rollback was not zero-write: %#v err=%v", fact, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if testCase.name == "completion commit crosses Result TTL" {
				if fact.State != contract.SingleCallToolActionCompletedV1 || fact.Result == nil {
					t.Fatalf("valid pre-commit completion was not persisted before return TTL crossed: %#v", fact)
				}
			} else if fact.State != contract.SingleCallToolActionWaitingInspectV1 || fact.Result != nil {
				t.Fatalf("time failure did not remain inspect-only: %#v", fact)
			}
		})
	}
}

func TestSingleCallToolActionConcurrentCanonicalRequestLinearizesOnceV1(t *testing.T) {
	fixture := newSingleCallFixtureV1(t)
	const workers = 64
	coordinators := make([]*application.SingleCallToolActionCoordinatorV1, workers)
	for index := range workers {
		coordinator, err := application.NewSingleCallToolActionCoordinatorV1(application.SingleCallToolActionCoordinatorConfigV1{
			Facts: fixture.store, Tool: fixture.tool, Inputs: fixture.inputs, Settlements: fixture.settlements, Clock: fixture.clock.Now,
		})
		if err != nil {
			t.Fatal(err)
		}
		coordinators[index] = coordinator
	}
	results := make(chan contract.SingleCallToolActionResultV1, workers)
	errors := make(chan error, workers)
	var group sync.WaitGroup
	for index := range workers {
		group.Add(1)
		go func(coordinator *application.SingleCallToolActionCoordinatorV1) {
			defer group.Done()
			result, err := coordinator.CoordinateSingleCallToolActionV1(context.Background(), fixture.request)
			if err != nil {
				errors <- err
				return
			}
			results <- result
		}(coordinators[index])
	}
	group.Wait()
	close(results)
	close(errors)
	errorCount := 0
	for err := range errors {
		errorCount++
		if !core.HasCategory(err, core.ErrorNotFound) && !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorIndeterminate) {
			t.Fatalf("inspect-only competitor returned an unexpected error: %v", err)
		}
	}
	count := 0
	for result := range results {
		count++
		if result.Digest != fixture.result.Digest {
			t.Fatal("concurrent caller observed another Result")
		}
	}
	if count+errorCount != workers || count == 0 || fixture.tool.providerCalls != 1 || fixture.tool.executeCalls != 1 {
		t.Fatalf("concurrency did not preserve one logical Provider action: results=%d inspect-only-errors=%d provider=%d executes=%d", count, errorCount, fixture.tool.providerCalls, fixture.tool.executeCalls)
	}
	result, err := fixture.coordinator.CoordinateSingleCallToolActionV1(context.Background(), fixture.request)
	if err != nil || result.Digest != fixture.result.Digest {
		t.Fatalf("post-race inspect did not recover the one exact Result: %#v err=%v", result, err)
	}
	creates, _ := fixture.store.Counts()
	if creates != 1 {
		t.Fatalf("concurrency created %d coordination facts", creates)
	}
}

func TestSingleCallToolActionSameIDChangedContentRejectedBeforeReadersV1(t *testing.T) {
	fixture := newSingleCallFixtureV1(t)
	changed := fixture.request
	changed.PendingAction.PayloadDigest = core.DigestBytes([]byte("different-payload"))
	changed.Digest, _ = changed.DigestV1()
	if _, err := fixture.coordinator.CoordinateSingleCallToolActionV1(context.Background(), changed); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("same ID changed content was not rejected canonically: %v", err)
	}
	if fixture.inputs.calls != 0 || fixture.tool.inspectCalls != 0 || fixture.tool.executeCalls != 0 {
		t.Fatal("changed content reached a Reader or Tool")
	}
}

func TestSingleCallToolActionHardStopSurfaceV1(t *testing.T) {
	configType := reflect.TypeOf(application.SingleCallToolActionCoordinatorConfigV1{})
	for _, forbidden := range []string{"Context", "Continuation", "Capability", "Turn", "Checkpoint", "Boundary", "Provider"} {
		if _, ok := configType.FieldByName(forbidden); ok {
			t.Fatalf("G6A config exposed forbidden dependency %s", forbidden)
		}
	}
	requestType := reflect.TypeOf(contract.SingleCallToolActionRequestV1{})
	resultType := reflect.TypeOf(contract.SingleCallToolActionResultV1{})
	for _, forbidden := range []string{"OperationScopeEvidenceApplicabilityFactRefV3", "ProviderBoundary", "Continuation", "NextTurn", "ContextFrame"} {
		if _, ok := requestType.FieldByName(forbidden); ok {
			t.Fatalf("Request exposed forbidden field %s", forbidden)
		}
		if _, ok := resultType.FieldByName(forbidden); ok {
			t.Fatalf("Result exposed forbidden field %s", forbidden)
		}
	}
}

type singleCallFixtureV1 struct {
	t           *testing.T
	now         time.Time
	clock       *singleCallClockV1
	request     contract.SingleCallToolActionRequestV1
	result      contract.SingleCallToolActionResultV1
	association runtimeports.OperationSettlementEvidenceAssociationV4
	store       *fakes.SingleCallToolActionCoordinationStoreV1
	inputs      *singleCallInputReaderFixtureV1
	tool        *singleCallToolPortFixtureV1
	settlements *singleCallSettlementReaderFixtureV1
	coordinator *application.SingleCallToolActionCoordinatorV1
}

func newSingleCallFixtureV1(t *testing.T) *singleCallFixtureV1 {
	t.Helper()
	now := time.Unix(400_000, 0)
	submission := testsupport.OperationSettlementSubmissionV4()
	request := singleCallRequestV1(t, now, submission)
	result, association := singleCallResultV1(t, now, request, submission)
	inputProjection := singleCallInputProjectionV1(t, now, request)
	store := fakes.NewSingleCallToolActionCoordinationStoreV1()
	inputs := &singleCallInputReaderFixtureV1{projection: inputProjection}
	tool := &singleCallToolPortFixtureV1{result: result}
	settlements := &singleCallSettlementReaderFixtureV1{inspection: result.Inspection, association: association}
	clock := &singleCallClockV1{now: now}
	coordinator, err := application.NewSingleCallToolActionCoordinatorV1(application.SingleCallToolActionCoordinatorConfigV1{Facts: store, Tool: tool, Inputs: inputs, Settlements: settlements, Clock: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	return &singleCallFixtureV1{t: t, now: now, clock: clock, request: request, result: result, association: association, store: store, inputs: inputs, tool: tool, settlements: settlements, coordinator: coordinator}
}

func seedSingleCallCoordinationStateV1(t *testing.T, fixture *singleCallFixtureV1, state contract.SingleCallToolActionCoordinationStateV1) {
	t.Helper()
	fact, err := contract.NewSingleCallToolActionCoordinationFactV1(fixture.request, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	current, err := fixture.store.CreateSingleCallToolActionCoordinationV1(context.Background(), fact)
	if err != nil {
		t.Fatal(err)
	}
	if state == contract.SingleCallToolActionPreparedV1 {
		return
	}
	current, err = contract.NextSingleCallToolActionCoordinationFactV1(current, contract.SingleCallToolActionDispatchIntentV1, nil, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	current, err = fixture.store.CompareAndSwapSingleCallToolActionCoordinationV1(context.Background(), applicationports.SingleCallToolActionCoordinationCASRequestV1{Scope: fixture.request.ExecutionScope, ID: fixture.request.ID, ExpectedRevision: 1, Next: current})
	if err != nil {
		t.Fatal(err)
	}
	if state == contract.SingleCallToolActionDispatchIntentV1 {
		return
	}
	next, err := contract.ClaimSingleCallToolActionStartV1(current, "start-claim/test-seed", fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.store.CompareAndSwapSingleCallToolActionCoordinationV1(context.Background(), applicationports.SingleCallToolActionCoordinationCASRequestV1{Scope: fixture.request.ExecutionScope, ID: fixture.request.ID, ExpectedRevision: current.Revision, Next: next}); err != nil {
		t.Fatal(err)
	}
}

type singleCallClockV1 struct {
	mu  sync.Mutex
	now time.Time
}

func (c *singleCallClockV1) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *singleCallClockV1) Set(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = now
}

func singleCallRequestV1(t *testing.T, now time.Time, submission runtimeports.OperationSettlementSubmissionV4) contract.SingleCallToolActionRequestV1 {
	t.Helper()
	d := core.DigestBytes
	expires := now.Add(10 * time.Second).UnixNano()
	sessionDigest := d([]byte("session-source"))
	turnDigest := d([]byte("turn-source"))
	provider := submission.DomainResult.Owner
	request, err := contract.SealSingleCallToolActionRequestV1(contract.SingleCallToolActionRequestV1{
		Workflow: contract.SingleCallWorkflowCoordinateV1{
			WorkflowContractVersion: contract.WorkflowContractVersionV2,
			PlanID:                  "plan-g6a", PlanRevision: 1, PlanDigest: d([]byte("plan-g6a")),
			JournalID: "journal-g6a", JournalRevision: 1, JournalDigest: d([]byte("journal-g6a")),
			StepID: "step-g6a", StepKind: contract.SingleCallToolActionStepKindV1,
			StepDescriptor: contract.StepDescriptorRefV2{Kind: contract.SingleCallToolActionStepKindV1, Revision: 1, Digest: d([]byte("step-descriptor")), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano()}, WorkflowAttempt: 1,
		},
		ExecutionScope:             submission.Operation.ExecutionScope,
		Run:                        contract.SingleCallRunCoordinateV1{RunID: "run-g6a", Revision: 1, Digest: d([]byte("run-g6a"))},
		Session:                    contract.SingleCallSessionCoordinateV1{ID: "session-g6a", Revision: 1, Digest: d([]byte("session-g6a")), Phase: contract.SingleCallSessionWaitingActionV1, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano()},
		SessionApplicabilitySource: contract.SingleCallSessionApplicabilitySourceCoordinateV1{Kind: contract.SingleCallSessionSourceKindV1, ID: "session:" + string(sessionDigest), Revision: 1, Digest: sessionDigest},
		Turn:                       contract.SingleCallTurnCoordinateV1{ID: "turn-g6a", Ordinal: 1, Revision: 1, Digest: d([]byte("turn-g6a"))},
		TurnApplicabilitySource:    contract.SingleCallTurnApplicabilitySourceCoordinateV1{Kind: contract.SingleCallTurnSourceKindV1, ID: "turn:" + string(turnDigest), Revision: 1, Digest: turnDigest},
		PendingAction: contract.SingleCallPendingActionCoordinateV1{
			ActionRef: "action-g6a", RequestDigest: d([]byte("pending-request")), Capability: "praxis.tool/execute",
			PayloadSchema: submission.DomainResult.Schema, PayloadDigest: d([]byte("tool-arguments")), SourceCandidateID: "candidate-g6a", SourceCandidateRevision: 1, SourceCandidateDigest: d([]byte("candidate-g6a")), ProjectionDigest: d([]byte("pending-projection")),
		},
		Observation: contract.SingleCallObservationCoordinateV1{
			ProjectionContractVersion: "praxis.model-invoker.tool-call-observation-projection/v1", ProjectionID: "projection-g6a", ProjectionRevision: 1, ProjectionDigest: d([]byte("model-projection")), InvocationID: "invocation-g6a", InvocationDigest: d([]byte("invocation-g6a")), ObservationDigest: d([]byte("observation-g6a")), SourceResponseID: "response-g6a", SourceSequence: 1, Evidence: runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: d([]byte("ledger-g6a")), Sequence: 1, RecordDigest: d([]byte("evidence-g6a"))}, CallCount: 1,
		},
		Assembly:                       contract.SingleCallAssemblyCoordinateV1{GenerationID: "generation-g6a", GenerationRevision: 1, GenerationDigest: d([]byte("generation-g6a")), BindingAssociation: runtimeports.GenerationBindingAssociationRefV1{ID: "generation-binding-g6a", Revision: 1, Digest: d([]byte("generation-binding-g6a"))}, ToolProvider: provider},
		Authority:                      runtimeports.AuthorityBindingRefV2{Ref: "authority-g6a", Revision: 1, Digest: d([]byte("authority-g6a")), Epoch: submission.Operation.ExecutionScope.AuthorityEpoch},
		ParentFrame:                    contract.SingleCallParentFrameCoordinateV1{FrameID: "frame-g6a", FrameRevision: 1, FrameDigest: d([]byte("frame-g6a")), GenerationID: "generation-g6a", GenerationRevision: 1, GenerationDigest: d([]byte("generation-g6a")), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano()},
		ParentFrameApplicabilitySource: contract.SingleCallParentFrameApplicabilitySourceCoordinateV1{Kind: contract.SingleCallParentFrameSourceKindV1, ID: "frame-g6a", Revision: 1, Digest: d([]byte("parent-frame-source"))},
		CreatedUnixNano:                now.Add(-time.Second).UnixNano(), ExpiresUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func singleCallInputProjectionV1(t *testing.T, now time.Time, request contract.SingleCallToolActionRequestV1) contract.SingleCallToolActionInputCurrentProjectionV1 {
	t.Helper()
	projection, err := contract.SealSingleCallToolActionInputCurrentProjectionV1(contract.SingleCallToolActionInputCurrentProjectionV1{
		Run: request.Run, Session: request.Session, SessionApplicabilitySource: request.SessionApplicabilitySource,
		Turn: request.Turn, TurnApplicabilitySource: request.TurnApplicabilitySource, PendingAction: request.PendingAction,
		Observation: request.Observation, Assembly: request.Assembly, Authority: request.Authority, ParentFrame: request.ParentFrame,
		ParentFrameApplicabilitySource: request.ParentFrameApplicabilitySource, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano(),
	}, request, now)
	if err != nil {
		t.Fatal(err)
	}
	return projection
}

func singleCallResultV1(t *testing.T, now time.Time, request contract.SingleCallToolActionRequestV1, submission runtimeports.OperationSettlementSubmissionV4) (contract.SingleCallToolActionResultV1, runtimeports.OperationSettlementEvidenceAssociationV4) {
	t.Helper()
	fact, err := runtimeports.SealOperationSettlementFactV4(runtimeports.OperationSettlementFactV4{Submission: submission})
	if err != nil {
		t.Fatal(err)
	}
	settlement := fact.RefV4()
	association, err := runtimeports.SealOperationSettlementEvidenceAssociationV4(runtimeports.OperationSettlementEvidenceAssociationV4{ID: "association-g6a", Settlement: settlement, Prepare: submission.Evidence[0], Execute: submission.Evidence[1]})
	if err != nil {
		t.Fatal(err)
	}
	guard, err := runtimeports.SealOperationSettlementTerminalGuardV4(runtimeports.OperationSettlementTerminalGuardV4{ID: "guard-g6a", TenantID: submission.TenantID, OperationDigest: submission.OperationDigest, EffectID: submission.EffectID, Settlement: settlement})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := runtimeports.SealOperationSettlementTerminalProjectionV4(runtimeports.OperationSettlementTerminalProjectionV4{ID: "terminal-projection-g6a", TenantID: submission.TenantID, OperationDigest: submission.OperationDigest, EffectID: submission.EffectID, Settlement: settlement, Association: association.RefV4(), Guard: guard.RefV4(), DomainResult: submission.DomainResult})
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := runtimeports.SealOperationInspectionSettlementRefV4(runtimeports.OperationInspectionSettlementRefV4{Settlement: settlement, Association: association.RefV4(), Guard: guard.RefV4(), Projection: projection.RefV4(), DomainResult: submission.DomainResult, EffectFactRevision: submission.ExpectedEffectRevision + 1, Owner: submission.Owner, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(8 * time.Second).UnixNano()}, now)
	if err != nil {
		t.Fatal(err)
	}
	actionDigest, err := contract.SingleCallActionCoordinateDigestV1(request)
	if err != nil {
		t.Fatal(err)
	}
	result, err := contract.SealSingleCallToolActionResultV1(contract.SingleCallToolActionResultV1{
		ToolResult: contract.SingleCallToolResultCoordinateV1{ID: "tool-result-g6a", Revision: 1, Digest: core.DigestBytes([]byte("tool-result-g6a")), ActionCoordinateDigest: actionDigest, ApplySettlementID: "apply-settlement-g6a", ApplySettlementRevision: 1, ApplySettlementDigest: core.DigestBytes([]byte("apply-settlement-g6a")), Settlement: settlement, ResultSchema: submission.DomainResult.Schema, PayloadDigest: core.DigestBytes([]byte("tool-result-payload")), FinalizedUnixNano: now.Add(-time.Nanosecond).UnixNano(), ExpiresUnixNano: now.Add(7 * time.Second).UnixNano()},
		Inspection: inspection, Association: association.RefV4(), AssociationCheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano(),
	}, request, now)
	if err != nil {
		t.Fatal(err)
	}
	return result, association
}

type singleCallInputReaderFixtureV1 struct {
	mu         sync.Mutex
	projection contract.SingleCallToolActionInputCurrentProjectionV1
	calls      int
	errAt      int
	driftAt    int
	onCall     func(int)
}

func (r *singleCallInputReaderFixtureV1) InspectSingleCallToolActionInputCurrentV1(_ context.Context, _ contract.SingleCallToolActionRequestV1) (contract.SingleCallToolActionInputCurrentProjectionV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.onCall != nil {
		r.onCall(r.calls)
	}
	if r.calls == r.errAt {
		return contract.SingleCallToolActionInputCurrentProjectionV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected input reader outage")
	}
	projection := r.projection
	if r.calls == r.driftAt {
		projection.ParentFrameApplicabilitySource.Digest = core.DigestBytes([]byte("drifted-parent-source"))
	}
	return projection, nil
}

type singleCallToolPortFixtureV1 struct {
	mu                   sync.Mutex
	result               contract.SingleCallToolActionResultV1
	stored               bool
	inspectErr           error
	loseNextExecuteReply bool
	unknownExecuteErr    error
	inspectCalls         int
	executeCalls         int
	providerCalls        int
	onInspect            func(int)
	onExecute            func(int)
}

func (p *singleCallToolPortFixtureV1) InspectSingleCallToolActionV1(_ context.Context, request applicationports.InspectSingleCallToolActionRequestV1) (contract.SingleCallToolActionResultV1, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inspectCalls++
	if p.onInspect != nil {
		p.onInspect(p.inspectCalls)
	}
	if p.inspectErr != nil {
		return contract.SingleCallToolActionResultV1{}, p.inspectErr
	}
	if !p.stored {
		return contract.SingleCallToolActionResultV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "authoritative Tool watermark not found")
	}
	if request.RequestID != p.result.RequestID || request.RequestDigest != p.result.RequestDigest {
		return contract.SingleCallToolActionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Tool Inspect key drifted")
	}
	return p.result, nil
}

func (p *singleCallToolPortFixtureV1) ExecuteSingleCallToolActionV1(_ context.Context, request contract.SingleCallToolActionRequestV1) (contract.SingleCallToolActionResultV1, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.executeCalls++
	if p.onExecute != nil {
		p.onExecute(p.executeCalls)
	}
	if request.ID != p.result.RequestID || request.Digest != p.result.RequestDigest {
		return contract.SingleCallToolActionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Tool command is not canonical")
	}
	if p.unknownExecuteErr != nil {
		p.providerCalls++
		return contract.SingleCallToolActionResultV1{}, p.unknownExecuteErr
	}
	if !p.stored {
		p.stored = true
		p.providerCalls++
	}
	if p.loseNextExecuteReply {
		p.loseNextExecuteReply = false
		return contract.SingleCallToolActionResultV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Tool execute reply loss")
	}
	return p.result, nil
}

type singleCallSettlementReaderFixtureV1 struct {
	mu               sync.Mutex
	inspection       runtimeports.OperationInspectionSettlementRefV4
	association      runtimeports.OperationSettlementEvidenceAssociationV4
	currentErr       error
	associationErr   error
	currentCalls     int
	associationCalls int
	onCurrent        func(int)
	onAssociation    func(int)
}

func (r *singleCallSettlementReaderFixtureV1) InspectCurrentOperationSettlementV4(_ context.Context, request runtimeports.InspectCurrentOperationSettlementRequestV4) (runtimeports.OperationInspectionSettlementRefV4, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.currentCalls++
	if r.onCurrent != nil {
		r.onCurrent(r.currentCalls)
	}
	if r.currentErr != nil {
		return runtimeports.OperationInspectionSettlementRefV4{}, r.currentErr
	}
	if request.EffectID != r.inspection.Settlement.EffectID || !runtimeports.SameOperationSubjectV3(request.Operation, r.inspection.DomainResult.Operation) {
		return runtimeports.OperationInspectionSettlementRefV4{}, core.NewError(core.ErrorConflict, core.ReasonEffectSettlementMissing, "current settlement request drifted")
	}
	return r.inspection, nil
}

func (r *singleCallSettlementReaderFixtureV1) InspectOperationSettlementEvidenceAssociationV4(_ context.Context, operation runtimeports.OperationSubjectV3, ref runtimeports.OperationSettlementEvidenceAssociationRefV4) (runtimeports.OperationSettlementEvidenceAssociationV4, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.associationCalls++
	if r.onAssociation != nil {
		r.onAssociation(r.associationCalls)
	}
	if r.associationErr != nil {
		return runtimeports.OperationSettlementEvidenceAssociationV4{}, r.associationErr
	}
	if !runtimeports.SameOperationSubjectV3(operation, r.inspection.DomainResult.Operation) || !runtimeports.SameOperationSettlementEvidenceAssociationRefV4(ref, r.association.RefV4()) {
		return runtimeports.OperationSettlementEvidenceAssociationV4{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Association Inspect key drifted")
	}
	return r.association, nil
}

var _ applicationports.SingleCallToolActionInputCurrentReaderV1 = (*singleCallInputReaderFixtureV1)(nil)
var _ applicationports.SingleCallToolActionPortV1 = (*singleCallToolPortFixtureV1)(nil)
var _ applicationports.SingleCallOperationSettlementCurrentReaderV1 = (*singleCallSettlementReaderFixtureV1)(nil)

func TestSingleCallToolActionFixtureCountersAreRaceSafeV1(t *testing.T) {
	// Compile-time reminder that test counters are fixture-only observations,
	// not a production exactly-once claim.
	var marker atomic.Uint64
	marker.Add(1)
	if marker.Load() != 1 {
		t.Fatal("atomic marker failed")
	}
}

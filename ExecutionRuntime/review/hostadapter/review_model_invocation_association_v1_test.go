package hostadapter_test

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	hostcontract "github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	hostjournal "github.com/Proview-China/rax/ExecutionRuntime/agent-host/journal"
	hostports "github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
	"github.com/Proview-China/rax/ExecutionRuntime/review/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/hostadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type attemptReaderV1 struct {
	mu                       sync.RWMutex
	historical, current      contract.AutoReviewerAttemptV1
	failExactOnce            bool
	exactCalls, currentCalls atomic.Int64
}

func (r *attemptReaderV1) InspectAutoReviewerAttemptExactV1(ctx context.Context, tenant core.TenantID, ref contract.ExactResourceRefV1) (contract.AutoReviewerAttemptV1, error) {
	r.exactCalls.Add(1)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failExactOnce {
		r.failExactOnce = false
		return contract.AutoReviewerAttemptV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "lost exact read reply")
	}
	if ctx == nil || ctx.Err() != nil {
		return contract.AutoReviewerAttemptV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "read context ended")
	}
	value := r.historical
	if value.ID == "" {
		value = r.current
	}
	if value.TenantID != tenant || value.ExactRef() != ref {
		return contract.AutoReviewerAttemptV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Attempt exact missing")
	}
	return value, nil
}
func (r *attemptReaderV1) InspectAutoReviewerAttemptCurrentV1(ctx context.Context, tenant core.TenantID, id string) (contract.AutoReviewerAttemptV1, error) {
	r.currentCalls.Add(1)
	r.mu.RLock()
	defer r.mu.RUnlock()
	if ctx == nil || ctx.Err() != nil {
		return contract.AutoReviewerAttemptV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "read context ended")
	}
	if r.current.TenantID != tenant || r.current.ID != id {
		return contract.AutoReviewerAttemptV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Attempt current missing")
	}
	return r.current, nil
}

type faultHostPortV1 struct {
	base                                    hostports.ReviewModelInvocationAssociationPortV1
	cancel                                  context.CancelFunc
	loseCreate, loseResolve, loseInspect    atomic.Bool
	blockHistorical                         atomic.Bool
	notFoundInspect                         atomic.Bool
	createCalls, resolveCalls, inspectCalls atomic.Int64
}

func (f *faultHostPortV1) CreateReviewModelInvocationAssociationV1(ctx context.Context, v hostcontract.ReviewModelInvocationAssociationFactV1) (hostports.ReviewModelInvocationAssociationCreateReceiptV1, error) {
	f.createCalls.Add(1)
	receipt, err := f.base.CreateReviewModelInvocationAssociationV1(ctx, v)
	if err == nil && f.loseCreate.CompareAndSwap(true, false) {
		if f.cancel != nil {
			f.cancel()
		}
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, hostcontract.NewError(hostcontract.ErrorUnknownOutcome, "reply_lost", "lost association create reply")
	}
	return receipt, err
}
func (f *faultHostPortV1) ResolveCurrentReviewModelInvocationAssociationV1(ctx context.Context, s hostcontract.ReviewModelInvocationAssociationSubjectV1) (hostcontract.ReviewModelInvocationAssociationRefV1, error) {
	f.resolveCalls.Add(1)
	if f.loseResolve.CompareAndSwap(true, false) {
		return hostcontract.ReviewModelInvocationAssociationRefV1{}, hostcontract.NewError(hostcontract.ErrorUnavailable, "read_lost", "lost Resolve reply")
	}
	return f.base.ResolveCurrentReviewModelInvocationAssociationV1(ctx, s)
}
func (f *faultHostPortV1) InspectCurrentReviewModelInvocationAssociationV1(ctx context.Context, s hostcontract.ReviewModelInvocationAssociationSubjectV1, r hostcontract.ReviewModelInvocationAssociationRefV1) (hostcontract.ReviewModelInvocationAssociationFactV1, error) {
	f.inspectCalls.Add(1)
	if f.notFoundInspect.Load() {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, hostcontract.NewError(hostcontract.ErrorNotFound, "authoritative_not_found", "association is absent")
	}
	if f.loseInspect.CompareAndSwap(true, false) {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, hostcontract.NewError(hostcontract.ErrorUnavailable, "read_lost", "lost Inspect reply")
	}
	return f.base.InspectCurrentReviewModelInvocationAssociationV1(ctx, s, r)
}
func (f *faultHostPortV1) InspectHistoricalReviewModelInvocationAssociationV1(ctx context.Context, r hostcontract.ReviewModelInvocationAssociationRefV1) (hostcontract.ReviewModelInvocationAssociationFactV1, error) {
	if f.blockHistorical.Load() {
		<-ctx.Done()
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, hostcontract.NewError(hostcontract.ErrorUnavailable, "blocked_recovery", "blocked historical recovery")
	}
	return f.base.InspectHistoricalReviewModelInvocationAssociationV1(ctx, r)
}
func (f *faultHostPortV1) CompareAndSwapReviewModelInvocationAssociationV1(ctx context.Context, r hostports.ReviewModelInvocationAssociationCASRequestV1) (hostports.ReviewModelInvocationAssociationCASReceiptV1, error) {
	return f.base.CompareAndSwapReviewModelInvocationAssociationV1(ctx, r)
}

func TestReviewModelAssociationAdapterConformanceV1(t *testing.T) {
	now := time.Unix(1_900_500_000, 0)
	request := associationRequestV1(t, now)
	reader := &attemptReaderV1{historical: request.Attempt, current: request.Attempt}
	store := hostjournal.NewMemoryReviewModelInvocationAssociationStoreV1(func() time.Time { return now })
	adapter, err := hostadapter.NewReviewModelAssociationAdapterV1(reader, store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	report, err := conformance.RunReviewModelAssociationAdapterV1(context.Background(), adapter, request)
	if err != nil || !report.ExactMapping || !report.S1S2Current || !report.IdempotentReplay || report.ProductionEligible {
		t.Fatalf("report=%+v err=%v", report, err)
	}
}

func TestReviewModelAssociationAdapterLostRepliesAreInspectOnlyV1(t *testing.T) {
	now := time.Unix(1_900_500_000, 0)
	request := associationRequestV1(t, now)
	reader := &attemptReaderV1{historical: request.Attempt, current: request.Attempt, failExactOnce: true}
	base := hostjournal.NewMemoryReviewModelInvocationAssociationStoreV1(func() time.Time { return now })
	ctx, cancel := context.WithCancel(context.Background())
	fault := &faultHostPortV1{base: base, cancel: cancel}
	fault.loseCreate.Store(true)
	fault.loseResolve.Store(true)
	fault.loseInspect.Store(true)
	adapter, err := hostadapter.NewReviewModelAssociationAdapterV1(reader, fault, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	got, err := adapter.StartOrInspectAssociationV1(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if fault.createCalls.Load() != 1 || fault.resolveCalls.Load() != 2 || fault.inspectCalls.Load() < 2 || reader.exactCalls.Load() < 3 {
		t.Fatalf("mutation/read recovery calls create=%d resolve=%d inspect=%d exact=%d", fault.createCalls.Load(), fault.resolveCalls.Load(), fault.inspectCalls.Load(), reader.exactCalls.Load())
	}
	historical, err := base.InspectHistoricalReviewModelInvocationAssociationV1(context.Background(), got.RefV1())
	if err != nil || !reflect.DeepEqual(historical, got) {
		t.Fatalf("exact recovery=%+v %v", historical, err)
	}
}

func TestReviewModelAssociationAdapterBlockedCreateRecoveryIsTTLBoundedAndPreservesUnknownV1(t *testing.T) {
	now := time.Unix(1_900_500_000, 0)
	request := associationRequestV1(t, now)
	request.Attempt.ExpiresUnixNano = now.Add(20 * time.Millisecond).UnixNano()
	request.Attempt.Digest = ""
	var err error
	request.Attempt, err = contract.SealAutoReviewerAttemptV1(request.Attempt)
	if err != nil {
		t.Fatal(err)
	}
	reader := &attemptReaderV1{historical: request.Attempt, current: request.Attempt}
	base := hostjournal.NewMemoryReviewModelInvocationAssociationStoreV1(func() time.Time { return now })
	fault := &faultHostPortV1{base: base}
	fault.loseCreate.Store(true)
	fault.blockHistorical.Store(true)
	adapter, err := hostadapter.NewReviewModelAssociationAdapterV1(reader, fault, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	_, err = adapter.StartOrInspectAssociationV1(context.Background(), request)
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("blocked historical recovery exceeded Attempt TTL: %s", elapsed)
	}
	if !core.HasCategory(err, core.ErrorIndeterminate) || !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("blocked historical recovery overwrote original Unknown: %v", err)
	}
	if fault.createCalls.Load() != 1 {
		t.Fatalf("lost create reply caused mutation replay: creates=%d", fault.createCalls.Load())
	}
}

func TestReviewModelAssociationAdapterAuthoritativeNotFoundIsNotRetriedV1(t *testing.T) {
	now := time.Unix(1_900_500_000, 0)
	request := associationRequestV1(t, now)
	reader := &attemptReaderV1{historical: request.Attempt, current: request.Attempt}
	base := hostjournal.NewMemoryReviewModelInvocationAssociationStoreV1(func() time.Time { return now })
	adapter, err := hostadapter.NewReviewModelAssociationAdapterV1(reader, base, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	created, err := adapter.StartOrInspectAssociationV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	fault := &faultHostPortV1{base: base}
	fault.notFoundInspect.Store(true)
	inspectAdapter, _ := hostadapter.NewReviewModelAssociationAdapterV1(reader, fault, func() time.Time { return now })
	if _, err = inspectAdapter.InspectCurrentAssociationV1(context.Background(), request.Attempt, created.RefV1()); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("authoritative NotFound was not preserved: %v", err)
	}
	if fault.inspectCalls.Load() != 1 {
		t.Fatalf("authoritative NotFound triggered retry: calls=%d", fault.inspectCalls.Load())
	}
}

func TestReviewModelAssociationAdapterMinTTLDeepCloneAndDriftV1(t *testing.T) {
	now := time.Unix(1_900_500_000, 0)
	request := associationRequestV1(t, now)
	request.Attempt.ExpiresUnixNano = now.Add(90 * time.Second).UnixNano()
	request.Attempt.Digest = ""
	var err error
	request.Attempt, err = contract.SealAutoReviewerAttemptV1(request.Attempt)
	if err != nil {
		t.Fatal(err)
	}
	reader := &attemptReaderV1{historical: request.Attempt, current: request.Attempt}
	store := hostjournal.NewMemoryReviewModelInvocationAssociationStoreV1(func() time.Time { return now })
	adapter, _ := hostadapter.NewReviewModelAssociationAdapterV1(reader, store, func() time.Time { return now })
	got, err := adapter.StartOrInspectAssociationV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if got.ExpiresUnixNano != request.Attempt.ExpiresUnixNano {
		t.Fatalf("min TTL=%d want=%d", got.ExpiresUnixNano, request.Attempt.ExpiresUnixNano)
	}
	got.Command.Call.Request.Input[0].Type = "mutated"
	historical, err := store.InspectHistoricalReviewModelInvocationAssociationV1(context.Background(), got.RefV1())
	if err != nil || historical.Command.Call.Request.Input[0].Type == "mutated" {
		t.Fatalf("deep clone failed: %+v %v", historical, err)
	}
	reader.mu.Lock()
	drift := request.Attempt
	drift.Revision = 2
	origin := request.Attempt.ExactRef()
	drift.InvocationAttempt = &origin
	drift.State = contract.AutoReviewerAttemptWaitingInspectV1
	drift.UpdatedUnixNano++
	drift.Digest = ""
	drift, err = contract.SealAutoReviewerAttemptV1(drift)
	reader.current = drift
	reader.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = adapter.InspectCurrentAssociationV1(context.Background(), request.Attempt, historical.RefV1()); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("Attempt drift=%v", err)
	}
}

func TestReviewModelAssociationAdapterRejectsTypedNilAndChangedCanonicalV1(t *testing.T) {
	base := time.Unix(1_900_500_000, 0)
	request := associationRequestV1(t, base)
	store := hostjournal.NewMemoryReviewModelInvocationAssociationStoreV1(func() time.Time { return base })
	var nilReader *attemptReaderV1
	if _, err := hostadapter.NewReviewModelAssociationAdapterV1(nilReader, store, func() time.Time { return base }); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed-nil reader=%v", err)
	}
	reader := &attemptReaderV1{historical: request.Attempt, current: request.Attempt}
	adapter, err := hostadapter.NewReviewModelAssociationAdapterV1(reader, store, func() time.Time { return base })
	if err != nil {
		t.Fatal(err)
	}
	first, err := adapter.StartOrInspectAssociationV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	changed := request
	changed.Command.Call.Request.Model = "gpt-5.5-review-drift"
	if _, err = adapter.StartOrInspectAssociationV1(context.Background(), changed); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("changed canonical command=%v", err)
	}
	historical, err := store.InspectHistoricalReviewModelInvocationAssociationV1(context.Background(), first.RefV1())
	if err != nil || !reflect.DeepEqual(first, historical) {
		t.Fatalf("changed replay altered history: %+v %v", historical, err)
	}
}

func TestReviewModelAssociationAdapterActualPointTTLCrossingCreatesNothingV1(t *testing.T) {
	base := time.Unix(1_900_500_000, 0)
	request := associationRequestV1(t, base)
	reader := &attemptReaderV1{historical: request.Attempt, current: request.Attempt}
	store := hostjournal.NewMemoryReviewModelInvocationAssociationStoreV1(func() time.Time { return base })
	fault := &faultHostPortV1{base: store}
	adapter, err := hostadapter.NewReviewModelAssociationAdapterV1(reader, fault, sequenceClockV1(base, time.Unix(0, request.Attempt.ExpiresUnixNano)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = adapter.StartOrInspectAssociationV1(context.Background(), request); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("TTL crossing=%v", err)
	}
	if fault.createCalls.Load() != 0 {
		t.Fatalf("TTL crossing reached Host mutation: creates=%d", fault.createCalls.Load())
	}
}

func TestReviewModelAssociationAdapterClockRollbackAndConcurrent64V1(t *testing.T) {
	base := time.Unix(1_900_500_000, 0)
	request := associationRequestV1(t, base)
	t.Run("rollback", func(t *testing.T) {
		reader := &attemptReaderV1{historical: request.Attempt, current: request.Attempt}
		store := hostjournal.NewMemoryReviewModelInvocationAssociationStoreV1(func() time.Time { return base })
		clock := sequenceClockV1(base, base.Add(time.Second), base.Add(2*time.Second), base.Add(time.Second))
		adapter, _ := hostadapter.NewReviewModelAssociationAdapterV1(reader, store, clock)
		if _, err := adapter.StartOrInspectAssociationV1(context.Background(), request); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("rollback=%v", err)
		}
	})
	t.Run("concurrent64", func(t *testing.T) {
		reader := &attemptReaderV1{historical: request.Attempt, current: request.Attempt}
		store := hostjournal.NewMemoryReviewModelInvocationAssociationStoreV1(func() time.Time { return base })
		adapter, _ := hostadapter.NewReviewModelAssociationAdapterV1(reader, store, func() time.Time { return base })
		const n = 64
		var wg sync.WaitGroup
		var failures atomic.Int64
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := adapter.StartOrInspectAssociationV1(context.Background(), request); err != nil {
					failures.Add(1)
				}
			}()
		}
		wg.Wait()
		if failures.Load() != 0 {
			t.Fatalf("failures=%d", failures.Load())
		}
	})
}

func associationRequestV1(t *testing.T, now time.Time) hostadapter.ReviewModelAssociationRequestV1 {
	t.Helper()
	digest := testkit.Digest
	scope := testkit.Scope()
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	operation := runtimeports.OperationSubjectV3{Kind: runtimeports.OperationScopeRunV3, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, RunID: "run-a", SubjectRevision: 1, CurrentProjectionRef: "review-operation-current", CurrentProjectionDigest: digest("operation-current"), CurrentProjectionRevision: 1}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := contract.SealAutoReviewerAttemptV1(contract.AutoReviewerAttemptV1{FactIdentityV1: contract.FactIdentityV1{TenantID: "tenant-a", ID: "auto-attempt-association", Revision: 1, CreatedUnixNano: now.Add(-time.Minute).UnixNano(), UpdatedUnixNano: now.Add(-time.Minute).UnixNano()}, IdempotencyKey: "auto-association-idem", Case: exactRef("case", digest("case")), Round: exactRef("round", digest("round")), Assignment: exactRef("assignment", digest("assignment")), Target: exactRef("target", digest("target")), Rubric: exactRef("rubric", digest("rubric")), ContextFrameDigest: digest("context"), ReviewerID: "auto-reviewer", ReviewerAuthority: testkit.Authority("reviewer-authority"), ReviewerBinding: testkit.ReviewerBinding(), RouteID: "praxis.model/review", Operation: operation, OperationDigest: operationDigest, InvocationEffect: runtimeports.ReviewInvocationEffectRefV2{EffectID: "review-model-effect", EffectRevision: 1, EffectKind: "praxis.review/auto-reviewer-invoke", PayloadDigest: digest("payload"), Provider: runtimeports.ProviderBindingRefV2{BindingSetID: "model-binding", BindingSetRevision: 1, ComponentID: "praxis.model/reviewer", ManifestDigest: digest("manifest"), ArtifactDigest: digest("artifact"), Capability: "praxis.model/review"}}, ResultSchema: testkit.Schema("auto-reviewer-result"), RoundOrdinal: 1, MaxCostMicros: 1000, State: contract.AutoReviewerAttemptPreparedV1, ExpiresUnixNano: now.Add(5 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	requestDigest := digest("model-request")
	prepared, err := modelinvoker.SealPreparedModelInvocationFactV1(modelinvoker.PreparedModelInvocationFactV1{InvocationID: "review-model-invocation", InvocationDigest: requestDigest, UnifiedRequestDigest: requestDigest, RequestToolsDigest: digest("tools"), PreparedPlanDigest: digest("plan"), RouteDigest: digest("route"), ProfileDigest: digest("profile"), ActualToolSurfaceDigest: digest("surface"), ActualProviderInjectionDigest: digest("injection"), CapabilitySnapshotRef: modelinvoker.PreparedModelInvocationCapabilitySnapshotRefV1{ContractVersion: "1.0.0", ID: "capability", Revision: 1, Digest: digest("capability")}, RegistrySnapshotRef: runtimeports.RegistrySnapshotRefV1{Owner: core.OwnerRef{Domain: "registry", ID: "owner"}, ContractVersion: "1.0.0", ID: "registry", Revision: 1, Digest: digest("registry")}, CreatedUnixNano: now.Add(-time.Minute).UnixNano(), NotAfterUnixNano: now.Add(10 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	current, err := modelinvoker.SealPreparedModelInvocationCurrentV1(modelinvoker.PreparedModelInvocationCurrentProjectionV1{Prepared: prepared.Ref(), CapabilitySnapshotRef: prepared.CapabilitySnapshotRef, RegistrySnapshotRef: prepared.RegistrySnapshotRef, ActualToolSurfaceDigest: prepared.ActualToolSurfaceDigest, ActualProviderInjectionDigest: prepared.ActualProviderInjectionDigest, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(8 * time.Minute).UnixNano(), NotAfterUnixNano: prepared.NotAfterUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	strict := true
	call := modelinvoker.RouteCall{RouteID: "openai.direct.payg.responses", Invocation: upstream.InvocationContext{Explicit: true, Usage: upstream.InvocationGeneralAPI, Subject: upstream.SubjectService, Tenancy: upstream.TenancyMulti, Execution: upstream.ExecutionForeground}, Request: modelinvoker.Request{Model: "gpt-5.5", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "review")}, ToolChoice: modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceNone}, Output: modelinvoker.OutputConstraint{Type: modelinvoker.OutputJSONSchema, Name: "review", Schema: []byte(`{"type":"object"}`), Strict: &strict}}}
	return hostadapter.ReviewModelAssociationRequestV1{Attempt: attempt, Command: modelinvoker.GovernedModelInvocationCommandV1{PreparedRef: prepared.Ref(), CurrentRef: current.Ref(), AttemptRequestDigest: requestDigest, DispatchSequence: 1, ProviderAttemptOrdinal: 1, Call: call}}
}
func exactRef(id string, digest core.Digest) contract.ExactResourceRefV1 {
	return contract.ExactResourceRefV1{ID: id, Revision: 1, Digest: digest}
}
func sequenceClockV1(values ...time.Time) func() time.Time {
	var mu sync.Mutex
	index := 0
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		if index >= len(values) {
			return values[len(values)-1]
		}
		value := values[index]
		index++
		return value
	}
}

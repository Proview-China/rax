package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/storetestkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/service"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type replyLostStoreV1 struct {
	*memory.Store
	createCalls     int
	invalidateCalls int
	hiddenTraceID   string
	driftTraceID    string
	blockTraceID    string
	traceInspects   int
	cancel          context.CancelFunc
	clock           *testkit.ManualClock
	recoveryTTL     time.Duration
}

func (s *replyLostStoreV1) CreateTargetCaseV1(ctx context.Context, m reviewport.CreateTargetCaseMutationV1) (contract.ReviewCaseV1, error) {
	s.createCalls++
	value, err := s.Store.CreateTargetCaseV1(ctx, m)
	if err != nil {
		return value, err
	}
	return contract.ReviewCaseV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected create reply loss")
}
func (s *replyLostStoreV1) InvalidateV1(ctx context.Context, m reviewport.InvalidateMutationV1) (contract.ReviewCaseV1, *contract.VerdictV1, error) {
	s.invalidateCalls++
	value, verdict, err := s.Store.InvalidateV1(ctx, m)
	if err != nil {
		return value, verdict, err
	}
	if s.clock != nil && s.recoveryTTL > 0 {
		s.clock.Set(time.Unix(0, value.ExpiresUnixNano).Add(-s.recoveryTTL))
	}
	if s.cancel != nil {
		s.cancel()
	}
	return contract.ReviewCaseV1{}, nil, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected invalidation reply loss")
}

func (s *replyLostStoreV1) InspectTraceExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (contract.TraceFactV1, error) {
	s.traceInspects++
	if ref.ID == s.blockTraceID {
		<-ctx.Done()
		return contract.TraceFactV1{}, ctx.Err()
	}
	if ref.ID == s.hiddenTraceID {
		return contract.TraceFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "injected missing Trace during recovery")
	}
	value, err := s.Store.InspectTraceExactV1(ctx, tenant, ref)
	if err == nil && ref.ID == s.driftTraceID {
		value.SourceSequence++
	}
	return value, err
}

func TestServiceLostReplyInspectsOriginalMutationOnlyV1(t *testing.T) {
	now := time.Unix(1_900_900_000, 0)
	clock := testkit.NewClock(now)
	store := &replyLostStoreV1{Store: storetestkit.NewMemoryStoreV1(clock.Now)}
	testkit.PublishRubric(context.Background(), store, now, "tenant-a")
	owner, err := service.New(store, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	target := testkit.Target(now)
	request := testkit.Request(now, target, "case-lost-reply")
	trace := testkit.TraceForTarget(now, request.CaseID, target, contract.TraceRequestedV1, 1, request.ID)
	view, err := owner.SubmitV1(context.Background(), service.SubmitCommandV1{Request: request, Target: target, Trace: trace})
	if err != nil || view.Case.ID != request.CaseID || store.createCalls != 1 {
		t.Fatalf("create recovery failed calls=%d err=%v", store.createCalls, err)
	}
	clock.Advance(time.Second)
	cancelTrace := testkit.Trace(clock.Now(), view.Case, contract.TraceCancelledV1, 2, view.Case.ID)
	cancelled, err := owner.CancelV1(context.Background(), service.CancelCommandV1{TenantID: view.Case.TenantID, CaseID: view.Case.ID, Expected: reviewport.ExpectedV1(view.Case.Revision, view.Case.Digest), Reason: core.ReasonInvalidState, Trace: cancelTrace})
	if err != nil || cancelled.State != contract.CaseCancelledV1 || store.invalidateCalls != 1 {
		t.Fatalf("cancel recovery failed calls=%d err=%v", store.invalidateCalls, err)
	}
}

func TestServiceInvalidateLostReplyRequiresExactTraceV1(t *testing.T) {
	now := time.Unix(1_900_910_000, 0)
	clock := testkit.NewClock(now)
	store := &replyLostStoreV1{Store: storetestkit.NewMemoryStoreV1(clock.Now)}
	testkit.PublishRubric(context.Background(), store, now, "tenant-a")
	owner, err := service.New(store, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	target := testkit.Target(now)
	request := testkit.Request(now, target, "case-lost-trace-recovery")
	view, err := owner.SubmitV1(context.Background(), service.SubmitCommandV1{Request: request, Target: target, Trace: testkit.TraceForTarget(now, request.CaseID, target, contract.TraceRequestedV1, 1, request.ID)})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	cancelTrace := testkit.Trace(clock.Now(), view.Case, contract.TraceCancelledV1, 2, view.Case.ID)
	store.hiddenTraceID = cancelTrace.ID
	_, err = owner.CancelV1(context.Background(), service.CancelCommandV1{TenantID: view.Case.TenantID, CaseID: view.Case.ID, Expected: reviewport.ExpectedV1(view.Case.Revision, view.Case.Digest), Reason: core.ReasonInvalidState, Trace: cancelTrace})
	if !core.HasCategory(err, core.ErrorIndeterminate) || !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("missing exact Trace did not preserve original unknown result: %v", err)
	}
	if store.invalidateCalls != 1 {
		t.Fatalf("lost reply retried Invalidate: calls=%d", store.invalidateCalls)
	}
}

func TestServiceRejectsTypedNilStoreV1(t *testing.T) {
	var store *memory.Store
	if _, err := service.New(store, time.Now); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("typed nil store accepted: %v", err)
	}
}

func TestServiceInvalidateLostReplyDetachesCallerCancellationV1(t *testing.T) {
	now := time.Unix(1_900_920_000, 0)
	clock := testkit.NewClock(now)
	store := &replyLostStoreV1{Store: storetestkit.NewMemoryStoreV1(clock.Now)}
	testkit.PublishRubric(context.Background(), store, now, "tenant-a")
	owner, err := service.New(store, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	target := testkit.Target(now)
	request := testkit.Request(now, target, "case-cancelled-caller-recovery")
	view, err := owner.SubmitV1(context.Background(), service.SubmitCommandV1{Request: request, Target: target, Trace: testkit.TraceForTarget(now, request.CaseID, target, contract.TraceRequestedV1, 1, request.ID)})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	trace := testkit.Trace(clock.Now(), view.Case, contract.TraceCancelledV1, 2, view.Case.ID)
	ctx, cancel := context.WithCancel(context.Background())
	store.cancel = cancel
	got, err := owner.CancelV1(ctx, service.CancelCommandV1{TenantID: view.Case.TenantID, CaseID: view.Case.ID, Expected: reviewport.ExpectedV1(view.Case.Revision, view.Case.Digest), Reason: core.ReasonInvalidState, Trace: trace})
	if err != nil || got.State != contract.CaseCancelledV1 {
		t.Fatalf("detached exact recovery failed: state=%s err=%v", got.State, err)
	}
	if store.invalidateCalls != 1 {
		t.Fatalf("mutation was replayed: calls=%d", store.invalidateCalls)
	}
}

func TestServiceInvalidateLostReplyBlockingInspectStopsAtTTLAndPreservesUnknownV1(t *testing.T) {
	now := time.Unix(1_900_930_000, 0)
	clock := testkit.NewClock(now)
	store := &replyLostStoreV1{Store: storetestkit.NewMemoryStoreV1(clock.Now), clock: clock, recoveryTTL: 25 * time.Millisecond}
	testkit.PublishRubric(context.Background(), store, now, "tenant-a")
	owner, err := service.New(store, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	target := testkit.Target(now)
	request := testkit.Request(now, target, "case-blocking-recovery")
	view, err := owner.SubmitV1(context.Background(), service.SubmitCommandV1{Request: request, Target: target, Trace: testkit.TraceForTarget(now, request.CaseID, target, contract.TraceRequestedV1, 1, request.ID)})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	trace := testkit.Trace(clock.Now(), view.Case, contract.TraceCancelledV1, 2, view.Case.ID)
	store.blockTraceID = trace.ID
	started := time.Now()
	_, err = owner.CancelV1(context.Background(), service.CancelCommandV1{TenantID: view.Case.TenantID, CaseID: view.Case.ID, Expected: reviewport.ExpectedV1(view.Case.Revision, view.Case.Digest), Reason: core.ReasonInvalidState, Trace: trace})
	if !core.HasCategory(err, core.ErrorIndeterminate) || !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("blocking recovery replaced the original Unknown: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("blocking recovery exceeded the subject TTL: %v", elapsed)
	}
	if store.invalidateCalls != 1 {
		t.Fatalf("mutation was replayed: calls=%d", store.invalidateCalls)
	}
}

func TestServiceInvalidateLostReplyRejectsExactDriftV1(t *testing.T) {
	now := time.Unix(1_900_940_000, 0)
	clock := testkit.NewClock(now)
	store := &replyLostStoreV1{Store: storetestkit.NewMemoryStoreV1(clock.Now)}
	testkit.PublishRubric(context.Background(), store, now, "tenant-a")
	owner, err := service.New(store, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	target := testkit.Target(now)
	request := testkit.Request(now, target, "case-drift-recovery")
	view, err := owner.SubmitV1(context.Background(), service.SubmitCommandV1{Request: request, Target: target, Trace: testkit.TraceForTarget(now, request.CaseID, target, contract.TraceRequestedV1, 1, request.ID)})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	trace := testkit.Trace(clock.Now(), view.Case, contract.TraceCancelledV1, 2, view.Case.ID)
	store.driftTraceID = trace.ID
	_, err = owner.CancelV1(context.Background(), service.CancelCommandV1{TenantID: view.Case.TenantID, CaseID: view.Case.ID, Expected: reviewport.ExpectedV1(view.Case.Revision, view.Case.Digest), Reason: core.ReasonInvalidState, Trace: trace})
	if !core.HasCategory(err, core.ErrorIndeterminate) || !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("exact drift replaced the original Unknown: %v", err)
	}
	if store.invalidateCalls != 1 {
		t.Fatalf("mutation was replayed: calls=%d", store.invalidateCalls)
	}
}

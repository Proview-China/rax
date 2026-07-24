package service_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/storetestkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/service"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestServiceSubmitInspectListAndCancelV1(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	clock := testkit.NewClock(now)
	store := storetestkit.NewMemoryStoreV1(clock.Now)
	testkit.PublishRubric(context.Background(), store, now, "tenant-a")
	owner, err := service.New(store, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	target := testkit.Target(now)
	request := testkit.Request(now, target, "case-service")
	trace := testkit.TraceForTarget(now, request.CaseID, target, contract.TraceRequestedV1, 1, request.ID)
	view, err := owner.SubmitV1(context.Background(), service.SubmitCommandV1{Request: request, Target: target, Trace: trace})
	if err != nil {
		t.Fatal(err)
	}
	if view.Case.State != contract.CaseRequestedV1 || view.Target.Digest != target.Digest || view.Verdict != nil {
		t.Fatalf("unexpected submit view: %+v", view)
	}
	replayed, err := owner.SubmitV1(context.Background(), service.SubmitCommandV1{Request: request, Target: target, Trace: trace})
	if err != nil || replayed.Case.Digest != view.Case.Digest {
		t.Fatalf("canonical submit replay failed: %v", err)
	}
	page, err := owner.ListV1(context.Background(), reviewport.ListCasesRequestV1{TenantID: target.TenantID, Limit: 1})
	if err != nil || len(page.Cases) != 1 || page.Cases[0].ID != request.CaseID {
		t.Fatalf("list failed: %+v %v", page, err)
	}
	inspected, err := owner.InspectV1(context.Background(), target.TenantID, request.CaseID)
	if err != nil || inspected.Case.Digest != view.Case.Digest {
		t.Fatalf("inspect failed: %v", err)
	}
	clock.Advance(time.Second)
	cancelTrace := testkit.Trace(clock.Now(), view.Case, contract.TraceCancelledV1, 2, view.Case.ID)
	command := service.CancelCommandV1{TenantID: target.TenantID, CaseID: view.Case.ID, Expected: reviewport.ExpectedV1(view.Case.Revision, view.Case.Digest), Reason: core.ReasonInvalidState, Trace: cancelTrace}
	cancelled, err := owner.CancelV1(context.Background(), command)
	if err != nil || cancelled.State != contract.CaseCancelledV1 {
		t.Fatalf("cancel failed: %+v %v", cancelled, err)
	}
	replay, err := owner.CancelV1(context.Background(), command)
	if err != nil || replay.Digest != cancelled.Digest {
		t.Fatalf("cancel replay failed: %v", err)
	}
}

func TestServiceSubmitSameTargetPayloadConflictV1(t *testing.T) {
	now := time.Unix(1_900_000_100, 0)
	clock := testkit.NewClock(now)
	store := storetestkit.NewMemoryStoreV1(clock.Now)
	testkit.PublishRubric(context.Background(), store, now, "tenant-a")
	owner, _ := service.New(store, clock.Now)
	target := testkit.Target(now)
	request := testkit.Request(now, target, "case-conflict")
	trace := testkit.TraceForTarget(now, request.CaseID, target, contract.TraceRequestedV1, 1, request.ID)
	if _, err := owner.SubmitV1(context.Background(), service.SubmitCommandV1{Request: request, Target: target, Trace: trace}); err != nil {
		t.Fatal(err)
	}
	drifted := target
	drifted.PayloadDigest = testkit.Digest("drift")
	drifted.Digest = ""
	drifted, _ = contract.SealTargetSnapshotV1(drifted)
	driftedRequest := testkit.Request(now, drifted, request.CaseID)
	driftedTrace := testkit.TraceForTarget(now, request.CaseID, drifted, contract.TraceRequestedV1, 1, driftedRequest.ID)
	if _, err := owner.SubmitV1(context.Background(), service.SubmitCommandV1{Request: driftedRequest, Target: drifted, Trace: driftedTrace}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same Target revision drift must conflict, got %v", err)
	}
	historical, err := owner.InspectV1(context.Background(), target.TenantID, request.CaseID)
	if err != nil || historical.Target.Digest != target.Digest {
		t.Fatalf("conflict leaked a write: %v", err)
	}
}

func TestServiceSubmitPersistsExactResultBundleAtomicallyV1(t *testing.T) {
	now := time.Unix(1_900_000_150, 0)
	clock := testkit.NewClock(now)
	store := storetestkit.NewMemoryStoreV1(clock.Now)
	testkit.PublishRubric(context.Background(), store, now, "tenant-a")
	owner, _ := service.New(store, clock.Now)
	target := testkit.Target(now)
	bundle := testkit.ResultBundle(now, target.TenantID, "bundle-service")
	request := testkit.Request(now, target, "case-bundle")
	request.ResultBundle = &contract.ExactResourceRefV1{ID: bundle.ID, Revision: bundle.Revision, Digest: bundle.Digest}
	request.Digest = ""
	request, _ = contract.SealReviewRequestV1(request)
	trace := testkit.TraceForTarget(now, request.CaseID, target, contract.TraceRequestedV1, 1, request.ID)
	view, err := owner.SubmitV1(context.Background(), service.SubmitCommandV1{Request: request, ResultBundle: &bundle, Target: target, Trace: trace})
	if err != nil || view.ResultBundle == nil || view.ResultBundle.Digest != bundle.Digest {
		t.Fatalf("result bundle admission failed: %+v %v", view, err)
	}
	inspected, err := owner.InspectV1(context.Background(), target.TenantID, request.CaseID)
	if err != nil || inspected.ResultBundle == nil || inspected.ResultBundle.Digest != bundle.Digest {
		t.Fatalf("exact result bundle inspect failed: %+v %v", inspected, err)
	}
}

func TestServiceResultBundleFailureLeavesNoAdmissionV1(t *testing.T) {
	now := time.Unix(1_900_000_175, 0)
	clock := testkit.NewClock(now)
	store := storetestkit.NewMemoryStoreV1(clock.Now)
	testkit.PublishRubric(context.Background(), store, now, "tenant-a")
	owner, _ := service.New(store, clock.Now)
	target := testkit.Target(now)
	bundle := testkit.ResultBundle(now, target.TenantID, "bundle-missing")
	request := testkit.Request(now, target, "case-bundle-missing")
	request.ResultBundle = &contract.ExactResourceRefV1{ID: bundle.ID, Revision: bundle.Revision, Digest: bundle.Digest}
	request.Digest = ""
	request, _ = contract.SealReviewRequestV1(request)
	trace := testkit.TraceForTarget(now, request.CaseID, target, contract.TraceRequestedV1, 1, request.ID)
	if _, err := owner.SubmitV1(context.Background(), service.SubmitCommandV1{Request: request, Target: target, Trace: trace}); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("missing exact Result Bundle was admitted: %v", err)
	}
	if _, err := store.InspectCaseV1(context.Background(), target.TenantID, request.CaseID); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("failed bundle admission leaked Case: %v", err)
	}
	if _, err := store.InspectResultBundleExactV1(context.Background(), target.TenantID, reviewport.ExactV1(bundle.ID, bundle.Revision, bundle.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("failed bundle admission leaked Result Bundle: %v", err)
	}
}

func TestServicePublicShapeHasNoVerdictMutationV1(t *testing.T) {
	typeOf := reflect.TypeOf((*service.Service)(nil))
	for index := 0; index < typeOf.NumMethod(); index++ {
		name := typeOf.Method(index).Name
		if name == "DecideV1" || name == "CreateVerdictV1" || name == "DispatchV1" || name == "CommitV1" {
			t.Fatalf("service exposes forbidden mutation %s", name)
		}
	}
}

func TestServiceIdempotencyCannotMoveAcrossTargetOrCaseV1(t *testing.T) {
	now := time.Unix(1_900_000_200, 0)
	clock := testkit.NewClock(now)
	store := storetestkit.NewMemoryStoreV1(clock.Now)
	testkit.PublishRubric(context.Background(), store, now, "tenant-a")
	owner, _ := service.New(store, clock.Now)
	first := testkit.Target(now)
	request := testkit.Request(now, first, "case-idem-a")
	trace := testkit.TraceForTarget(now, request.CaseID, first, contract.TraceRequestedV1, 1, request.ID)
	if _, err := owner.SubmitV1(context.Background(), service.SubmitCommandV1{Request: request, Target: first, Trace: trace}); err != nil {
		t.Fatal(err)
	}
	second := first
	second.ID = "target-idem-b"
	second.Digest = ""
	second, _ = contract.SealTargetSnapshotV1(second)
	moved := testkit.Request(now, second, "case-idem-b")
	moved.IdempotencyKey = request.IdempotencyKey
	moved.Digest = ""
	moved, _ = contract.SealReviewRequestV1(moved)
	movedTrace := testkit.TraceForTarget(now, moved.CaseID, second, contract.TraceRequestedV1, 1, moved.ID)
	if _, err := owner.SubmitV1(context.Background(), service.SubmitCommandV1{Request: moved, Target: second, Trace: movedTrace}); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("idempotency key moved to another target/case: %v", err)
	}
}

func TestServiceCancelReplayRequiresOriginalExactTraceV1(t *testing.T) {
	now := time.Unix(1_900_000_250, 0)
	clock := testkit.NewClock(now)
	store := storetestkit.NewMemoryStoreV1(clock.Now)
	testkit.PublishRubric(context.Background(), store, now, "tenant-a")
	owner, _ := service.New(store, clock.Now)
	target := testkit.Target(now)
	request := testkit.Request(now, target, "case-cancel-replay")
	trace := testkit.TraceForTarget(now, request.CaseID, target, contract.TraceRequestedV1, 1, request.ID)
	view, err := owner.SubmitV1(context.Background(), service.SubmitCommandV1{Request: request, Target: target, Trace: trace})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	cancelTrace := testkit.Trace(clock.Now(), view.Case, contract.TraceCancelledV1, 2, view.Case.ID)
	command := service.CancelCommandV1{TenantID: target.TenantID, CaseID: view.Case.ID, Expected: reviewport.ExpectedV1(view.Case.Revision, view.Case.Digest), Reason: core.ReasonInvalidState, Trace: cancelTrace}
	if _, err := owner.CancelV1(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	drift := cancelTrace
	drift.ID = "trace-cancel-drift"
	drift.Digest = ""
	drift, _ = contract.SealTraceFactV1(drift)
	command.Trace = drift
	if _, err := owner.CancelV1(context.Background(), command); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("cancel replay accepted another Trace: %v", err)
	}
}

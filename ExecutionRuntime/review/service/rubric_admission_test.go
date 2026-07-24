package service_test

import (
	"context"
	"sync"
	"sync/atomic"
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

func submitWithRubricV1(now time.Time, target contract.TargetSnapshotV1, caseID string) service.SubmitCommandV1 {
	request := testkit.Request(now, target, caseID)
	trace := testkit.TraceForTarget(now, caseID, target, contract.TraceRequestedV1, 1, request.ID)
	return service.SubmitCommandV1{Request: request, Target: target, Trace: trace}
}

func TestServiceAdmissionRequiresReviewOwnedCurrentRubricV1(t *testing.T) {
	now := time.Unix(1_901_200_000, 0)
	store := memory.NewStore()
	owner, err := service.New(store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	target := testkit.Target(now)
	command := submitWithRubricV1(now, target, "case-no-rubric")
	if _, err := owner.SubmitV1(context.Background(), command); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("caller exact ref was trusted without Review-owned Rubric current: %v", err)
	}
	if _, err := store.InspectCaseV1(context.Background(), target.TenantID, command.Request.CaseID); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("missing Rubric admission leaked a Case: %v", err)
	}
}

type driftingRubricStoreV1 struct {
	*memory.Store
	next  contract.RubricDefinitionV1
	calls atomic.Int32
}

func (s *driftingRubricStoreV1) InspectRubricCurrentV1(ctx context.Context, tenant core.TenantID, expected contract.ExactResourceRefV1, now time.Time) (contract.RubricDefinitionV1, error) {
	if s.calls.Add(1) == 2 {
		previous := expected
		if _, err := s.Store.PublishRubricV1(ctx, reviewport.PublishRubricMutationV1{Expected: &previous, Next: s.next}); err != nil {
			return contract.RubricDefinitionV1{}, err
		}
	}
	return s.Store.InspectRubricCurrentV1(ctx, tenant, expected, now)
}

func TestServiceAdmissionRubricS1S2DriftLeavesZeroFactsV1(t *testing.T) {
	now := time.Unix(1_901_200_100, 0)
	baseStore := memory.NewStore()
	first := testkit.PublishRubric(context.Background(), baseStore, now, "tenant-a")
	next := first
	next.Revision++
	next.Name = "drifted between S1 and S2"
	next.UpdatedUnixNano = now.Add(time.Second).UnixNano()
	next.Digest = ""
	next, _ = contract.SealRubricDefinitionV1(next)
	store := &driftingRubricStoreV1{Store: baseStore, next: next}
	owner, _ := service.New(store, func() time.Time { return now.Add(2 * time.Second) })
	target := testkit.Target(now)
	command := submitWithRubricV1(now, target, "case-rubric-drift")
	if _, err := owner.SubmitV1(context.Background(), command); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("Rubric drift between S1/S2 was admitted: %v", err)
	}
	if _, err := store.InspectCaseV1(context.Background(), target.TenantID, command.Request.CaseID); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("Rubric S1/S2 drift leaked a Case: %v", err)
	}
}

type sequenceClockV1 struct {
	mu     sync.Mutex
	values []time.Time
	index  int
}

func (c *sequenceClockV1) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.index >= len(c.values) {
		return c.values[len(c.values)-1]
	}
	value := c.values[c.index]
	c.index++
	return value
}

func TestServiceAdmissionRubricClockRollbackLeavesZeroFactsV1(t *testing.T) {
	now := time.Unix(1_901_200_200, 0)
	store := memory.NewStore()
	testkit.PublishRubric(context.Background(), store, now, "tenant-a")
	clock := &sequenceClockV1{values: []time.Time{now.Add(2 * time.Second), now.Add(time.Second)}}
	owner, _ := service.New(store, clock.Now)
	target := testkit.Target(now)
	command := submitWithRubricV1(now, target, "case-rubric-clock")
	if _, err := owner.SubmitV1(context.Background(), command); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("Rubric admission clock rollback was accepted: %v", err)
	}
	if _, err := store.InspectCaseV1(context.Background(), target.TenantID, command.Request.CaseID); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("clock rollback leaked a Case: %v", err)
	}
}

func TestServiceAdmissionUsesRubricAsTTLInputV1(t *testing.T) {
	now := time.Unix(1_901_200_300, 0)
	store := memory.NewStore()
	rubric := testkit.Rubric(now, "tenant-a")
	rubric.ExpiresUnixNano = now.Add(10 * time.Minute).UnixNano()
	rubric.Digest = ""
	rubric, _ = contract.SealRubricDefinitionV1(rubric)
	if _, err := store.PublishRubricV1(context.Background(), reviewport.PublishRubricMutationV1{Next: rubric}); err != nil {
		t.Fatal(err)
	}
	owner, _ := service.New(store, func() time.Time { return now })
	target := testkit.Target(now)
	command := submitWithRubricV1(now, target, "case-rubric-ttl")
	command.Request.Rubric = rubric.ExactRef()
	command.Request.Digest = ""
	command.Request, _ = contract.SealReviewRequestV1(command.Request)
	if _, err := owner.SubmitV1(context.Background(), command); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("Request outlived its exact Rubric: %v", err)
	}
	if _, err := store.InspectCaseV1(context.Background(), target.TenantID, command.Request.CaseID); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("Rubric TTL failure leaked a Case: %v", err)
	}
}

func TestServiceAdmissionRubricStoreActualPointFailsClosedV1(t *testing.T) {
	base := time.Unix(1_901_200_400, 0)
	for _, test := range []struct {
		name        string
		actualPoint time.Time
		wantReason  core.ReasonCode
	}{
		{name: "ttl_crossing", actualPoint: base.Add(6 * time.Minute), wantReason: core.ReasonReviewVerdictStale},
		{name: "clock_rollback", actualPoint: base, wantReason: core.ReasonClockRegression},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			storeClock := testkit.NewClock(base.Add(time.Second))
			store := storetestkit.NewMemoryStoreV1(storeClock.Now)
			rubric := testkit.Rubric(base, "tenant-a")
			rubric.ExpiresUnixNano = base.Add(5 * time.Minute).UnixNano()
			rubric.Digest = ""
			rubric, _ = contract.SealRubricDefinitionV1(rubric)
			if _, err := store.PublishRubricV1(ctx, reviewport.PublishRubricMutationV1{Next: rubric}); err != nil {
				t.Fatal(err)
			}
			target := testkit.Target(base)
			command := submitWithRubricV1(base, target, "case-actual-point-"+test.name)
			command.Request.Rubric = rubric.ExactRef()
			command.Request.ExpiresUnixNano = base.Add(4 * time.Minute).UnixNano()
			command.Request.Digest = ""
			command.Request, _ = contract.SealReviewRequestV1(command.Request)
			command.Trace = testkit.TraceForTarget(base, command.Request.CaseID, target, contract.TraceRequestedV1, 1, command.Request.ID)
			owner, _ := service.New(store, func() time.Time { return base.Add(2 * time.Second) })
			storeClock.Set(test.actualPoint)
			if _, err := owner.SubmitV1(ctx, command); !core.HasReason(err, test.wantReason) {
				t.Fatalf("Store actual-point %s was admitted: %v", test.name, err)
			}
			if _, err := store.InspectCaseV1(ctx, target.TenantID, command.Request.CaseID); !core.HasCategory(err, core.ErrorNotFound) {
				t.Fatalf("Store actual-point failure leaked Case: %v", err)
			}
			if _, err := store.InspectRequestExactV1(ctx, target.TenantID, reviewport.ExactV1(command.Request.ID, command.Request.Revision, command.Request.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
				t.Fatalf("Store actual-point failure leaked Request: %v", err)
			}
		})
	}
}

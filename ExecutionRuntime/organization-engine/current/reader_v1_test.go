package current_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/organization-engine/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/organization-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/organization-engine/current"
	"github.com/Proview-China/rax/ExecutionRuntime/organization-engine/memory"
	"github.com/Proview-China/rax/ExecutionRuntime/organization-engine/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestDelegationAndSelfReviewFailClosedV1(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1800000000, 0)
	store := memory.NewStore()
	source := conformance.SeedDirectV1(t, ctx, store, now)
	closure, err := store.ReadReviewEligibilityClosureV1(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	delegator := sealIdentity(t, "lead", now)
	if err = store.PublishIdentityV1(ctx, nil, delegator); err != nil {
		t.Fatal(err)
	}
	delegation, err := contract.SealDelegationV1(contract.DelegationFactV1{FactMetaV1: meta(now, now.Add(20*time.Minute)), Delegator: delegator.ExactRef(), Delegate: closure.Identity.ExactRef(), DelegatorSubjectID: "lead", DelegateSubjectID: "reviewer", Role: "security", ScopeDigest: source.ScopeDigest})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.PublishDelegationV1(ctx, nil, delegation); err != nil {
		t.Fatal(err)
	}
	delegated := source.Clone()
	delegated.RequireDelegation = true
	delegated.DelegatorSubjectID = "lead"
	delegated.DelegatedRole = "security"
	reader, _ := current.NewReaderV1(store, incrementing(now))
	p, err := reader.ResolveCurrentReviewEligibilityV1(ctx, delegated)
	if err != nil {
		t.Fatalf("delegation rejected: %v", err)
	}
	if p.ExpiresUnixNano != now.Add(20*time.Minute).UnixNano() {
		t.Fatalf("delegation min TTL=%d", p.ExpiresUnixNano)
	}
	resp := closure.Responsibility
	resp.Revision++
	resp.UpdatedUnixNano = now.Add(time.Second).UnixNano()
	resp.Identity = closure.Identity.ExactRef()
	resp, err = contract.SealResponsibilityV1(resp)
	if err != nil {
		t.Fatal(err)
	}
	old := closure.Responsibility.ExactRef()
	if err = store.PublishResponsibilityV1(ctx, &old, resp); err != nil {
		t.Fatal(err)
	}
	reader, _ = current.NewReaderV1(store, incrementing(now.Add(2*time.Second)))
	if _, err = reader.ResolveCurrentReviewEligibilityV1(ctx, source); !core.HasCategory(err, core.ErrorForbidden) {
		t.Fatalf("self review category=%v", err)
	}
}

func TestClockRollbackAndTTLCrossingZeroProjectionV1(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1800000000, 0)
	store := memory.NewStore()
	source := conformance.SeedDirectV1(t, ctx, store, now)
	times := []time.Time{now.Add(10 * time.Second), now.Add(9 * time.Second)}
	var i atomic.Int64
	reader, _ := current.NewReaderV1(store, func() time.Time { return times[int(i.Add(1)-1)] })
	if _, err := reader.ResolveCurrentReviewEligibilityV1(ctx, source); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("rollback=%v", err)
	}
	reader, _ = current.NewReaderV1(store, func() time.Time { return now.Add(2 * time.Hour) })
	if _, err := reader.ResolveCurrentReviewEligibilityV1(ctx, source); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("TTL crossing=%v", err)
	}
}

type inspectUnknownStore struct {
	ports.StoreV1
	once atomic.Bool
}

type driftStore struct {
	ports.StoreV1
	count atomic.Int64
	role  contract.RoleGrantFactV1
	now   time.Time
}

func (s *driftStore) ReadReviewEligibilityClosureV1(ctx context.Context, source contract.ReviewEligibilitySourceV1) (ports.ReviewEligibilityClosureV1, error) {
	if s.count.Add(1) == 2 {
		next := s.role
		next.Revision++
		next.UpdatedUnixNano = s.now.Add(time.Second).UnixNano()
		next.ExpiresUnixNano = s.now.Add(30 * time.Minute).UnixNano()
		next, err := contract.SealRoleGrantV1(next)
		if err != nil {
			return ports.ReviewEligibilityClosureV1{}, err
		}
		old := s.role.ExactRef()
		if err = s.StoreV1.PublishRoleGrantV1(ctx, &old, next); err != nil {
			return ports.ReviewEligibilityClosureV1{}, err
		}
	}
	return s.StoreV1.ReadReviewEligibilityClosureV1(ctx, source)
}

func TestResolveRejectsS1S2CurrentDriftV1(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1800000000, 0)
	base := memory.NewStore()
	source := conformance.SeedDirectV1(t, ctx, base, now)
	closure, err := base.ReadReviewEligibilityClosureV1(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	wrapped := &driftStore{StoreV1: base, role: closure.Roles[0], now: now}
	reader, _ := current.NewReaderV1(wrapped, incrementing(now.Add(2*time.Second)))
	if _, err = reader.ResolveCurrentReviewEligibilityV1(ctx, source); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("drift=%v", err)
	}
}

func (s *inspectUnknownStore) InspectReviewEligibilityProjectionV1(ctx context.Context, r contract.ReviewEligibilityProjectionRefV1) (contract.ReviewEligibilityCurrentProjectionV1, error) {
	if !s.once.Swap(true) {
		return contract.ReviewEligibilityCurrentProjectionV1{}, ports.IndeterminateV1("lost reply")
	}
	return s.StoreV1.InspectReviewEligibilityProjectionV1(ctx, r)
}

func TestInspectLostReplyDetachesRemainingExactReadV1(t *testing.T) {
	now := time.Unix(1800000000, 0)
	base := memory.NewStore()
	source := conformance.SeedDirectV1(t, context.Background(), base, now)
	reader, _ := current.NewReaderV1(base, incrementing(now))
	p, err := reader.ResolveCurrentReviewEligibilityV1(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	wrapped := &inspectUnknownStore{StoreV1: base}
	reader, _ = current.NewReaderV1(wrapped, incrementing(now.Add(time.Minute)))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := reader.InspectCurrentReviewEligibilityV1(ctx, p.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProjectionDigest != p.ProjectionDigest {
		t.Fatal("lost reply recovered different projection")
	}
}

func TestConcurrentFullRefCASOnlyOneNextRevisionV1(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1800000000, 0)
	store := memory.NewStore()
	source := conformance.SeedDirectV1(t, ctx, store, now)
	closure, err := store.ReadReviewEligibilityClosureV1(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	expected := closure.Identity.ExactRef()
	var success atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v := closure.Identity
			v.Revision++
			v.UpdatedUnixNano = now.Add(time.Duration(i+1) * time.Nanosecond).UnixNano()
			v.DisplayHandle = string(rune('A'+i%26)) + time.Duration(i).String()
			v, err := contract.SealIdentityV1(v)
			if err != nil {
				t.Error(err)
				return
			}
			if err = store.PublishIdentityV1(ctx, &expected, v); err == nil {
				success.Add(1)
			} else if !core.HasCategory(err, core.ErrorConflict) {
				t.Errorf("unexpected category: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if success.Load() != 1 {
		t.Fatalf("success=%d", success.Load())
	}
	if _, err = store.InspectIdentityV1(ctx, expected); err != nil {
		t.Fatal(err)
	}
}

func TestConstructorRejectsTypedNilV1(t *testing.T) {
	var store *memory.Store
	if _, err := current.NewReaderV1(store, time.Now); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("typed nil=%v", err)
	}
}

func incrementing(now time.Time) func() time.Time {
	var n atomic.Int64
	return func() time.Time { return now.Add(time.Duration(n.Add(1)) * time.Nanosecond) }
}
func meta(now, expires time.Time) contract.FactMetaV1 {
	return contract.FactMetaV1{TenantID: "tenant-a", Revision: 1, CreatedUnixNano: now.Add(-time.Minute).UnixNano(), UpdatedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: expires.UnixNano(), State: contract.FactActiveV1}
}
func sealIdentity(t *testing.T, id string, now time.Time) contract.IdentityFactV1 {
	t.Helper()
	v, err := contract.SealIdentityV1(contract.IdentityFactV1{FactMetaV1: meta(now, now.Add(time.Hour)), SubjectKind: contract.SubjectHumanV1, SubjectID: id, DisplayHandle: id})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

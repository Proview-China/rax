package conformance

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/organization-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/organization-engine/current"
	"github.com/Proview-China/rax/ExecutionRuntime/organization-engine/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type StoreFactoryV1 func(*testing.T) (ports.StoreV1, func())

func RunStoreAndReaderV1(t *testing.T, factory StoreFactoryV1) {
	t.Helper()
	store, closeFn := factory(t)
	defer closeFn()
	ctx := context.Background()
	now := time.Unix(1800000000, 0)
	source := SeedDirectV1(t, ctx, store, now)
	var tick atomic.Int64
	clock := func() time.Time { return now.Add(time.Duration(tick.Add(1)) * time.Nanosecond) }
	reader, err := current.NewReaderV1(store, clock)
	if err != nil {
		t.Fatal(err)
	}
	p1, err := reader.ResolveCurrentReviewEligibilityV1(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	if p1.ExpiresUnixNano != now.Add(40*time.Minute).UnixNano() {
		t.Fatalf("min TTL=%d", p1.ExpiresUnixNano)
	}
	if len(p1.Roles) != 2 || p1.Roles[0].Role != "security" || p1.Roles[1].Role != "technical" {
		t.Fatalf("role closure not keyed by role: %+v", p1.Roles)
	}
	p2, err := reader.ResolveCurrentReviewEligibilityV1(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	if p1.CheckedUnixNano != p2.CheckedUnixNano || p1.ProjectionDigest != p2.ProjectionDigest {
		t.Fatal("same closure was re-sealed")
	}
	p3, err := reader.InspectCurrentReviewEligibilityV1(ctx, p1.Ref)
	if err != nil {
		t.Fatal(err)
	}
	p3.Source.RequiredRoles[0] = "mutated"
	p4, err := reader.InspectCurrentReviewEligibilityV1(ctx, p1.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if p4.Source.RequiredRoles[0] != "security" {
		t.Fatal("projection alias escaped")
	}
	old := p1.Identity
	next := old
	next.Revision++
	next.UpdatedUnixNano = now.Add(time.Minute).UnixNano()
	next.DisplayHandle = "reviewer-v2"
	next, err = contract.SealIdentityV1(next)
	if err != nil {
		t.Fatal(err)
	}
	expected := old.ExactRef()
	if err = store.PublishIdentityV1(ctx, &expected, next); err != nil {
		t.Fatal(err)
	}
	if _, err = store.InspectIdentityV1(ctx, old.ExactRef()); err != nil {
		t.Fatalf("historical exact lost: %v", err)
	}
	if _, err = reader.InspectCurrentReviewEligibilityV1(ctx, p1.Ref); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("drift category=%v", err)
	}
	bad := next
	bad.Revision++
	bad.UpdatedUnixNano = now.Add(2 * time.Minute).UnixNano()
	bad.DisplayHandle = "bad"
	bad, err = contract.SealIdentityV1(bad)
	if err != nil {
		t.Fatal(err)
	}
	wrong := old.ExactRef()
	if err = store.PublishIdentityV1(ctx, &wrong, bad); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("stale CAS=%v", err)
	}
	if _, err = store.InspectIdentityV1(ctx, next.ExactRef()); err != nil {
		t.Fatal(err)
	}
}

func SeedDirectV1(t *testing.T, ctx context.Context, store ports.StoreV1, now time.Time) contract.ReviewEligibilitySourceV1 {
	t.Helper()
	scope := core.DigestBytes([]byte("scope"))
	reviewer := identity(t, "reviewer", now, now.Add(time.Hour))
	author := identity(t, "author", now, now.Add(50*time.Minute))
	if err := store.PublishIdentityV1(ctx, nil, reviewer); err != nil {
		t.Fatal(err)
	}
	if err := store.PublishIdentityV1(ctx, nil, author); err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"technical", "security"} {
		v := contract.RoleGrantFactV1{FactMetaV1: meta(now, now.Add(40*time.Minute)), Identity: reviewer.ExactRef(), Role: role, ScopeDigest: scope, CanVeto: role == "security"}
		v, err := contract.SealRoleGrantV1(v)
		if err != nil {
			t.Fatal(err)
		}
		if err = store.PublishRoleGrantV1(ctx, nil, v); err != nil {
			t.Fatal(err)
		}
	}
	subject := core.DigestBytes([]byte("target"))
	resp := contract.ResponsibilityFactV1{FactMetaV1: meta(now, now.Add(45*time.Minute)), SubjectKind: "review-target", SubjectID: "target-a", SubjectDigest: subject, Identity: author.ExactRef()}
	resp, err := contract.SealResponsibilityV1(resp)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.PublishResponsibilityV1(ctx, nil, resp); err != nil {
		t.Fatal(err)
	}
	return contract.ReviewEligibilitySourceV1{TenantID: "tenant-a", ReviewerSubjectID: "reviewer", RequiredRoles: []string{"security", "technical"}, ScopeDigest: scope, ResponsibilitySubjectKind: "review-target", ResponsibilitySubjectID: "target-a", ResponsibilitySubjectDigest: subject, Production: true}
}

func identity(t *testing.T, id string, now, expires time.Time) contract.IdentityFactV1 {
	t.Helper()
	v, err := contract.SealIdentityV1(contract.IdentityFactV1{FactMetaV1: meta(now, expires), SubjectKind: contract.SubjectHumanV1, SubjectID: id, DisplayHandle: id})
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func meta(now, expires time.Time) contract.FactMetaV1 {
	return contract.FactMetaV1{TenantID: "tenant-a", Revision: 1, CreatedUnixNano: now.Add(-time.Minute).UnixNano(), UpdatedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: expires.UnixNano(), State: contract.FactActiveV1}
}

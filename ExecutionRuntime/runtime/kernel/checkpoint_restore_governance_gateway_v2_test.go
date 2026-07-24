package kernel_test

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestRestoreGovernanceV2HappyLostReplyAndCurrent(t *testing.T) {
	fixture := newRestoreGovernanceFixtureV2(t, "happy")
	fixture.store.LoseNextRestoreReplyV2()
	attempt, err := fixture.gateway.CreateRestoreAttemptV2(context.Background(), fixture.create)
	if err != nil || attempt.State != ports.RestoreAttemptReservedV2 || attempt.OperationScope.Identity.TargetInstance.ID == attempt.OperationScope.Identity.SourceInstance.ID {
		t.Fatalf("create Restore Attempt: %+v err=%v", attempt, err)
	}
	fixture.store.LoseNextRestoreReplyV2()
	bundle, err := fixture.gateway.IssueRestoreEligibilityV2(context.Background(), ports.IssueRestoreEligibilityRequestV2{EligibilityID: "restore-eligibility-happy", Attempt: attempt.Ref, RequestedTTL: time.Minute})
	if err != nil || bundle.Attempt.State != ports.RestoreAttemptEligibilityBoundV2 || bundle.Eligibility.State != ports.RestoreEligibilityActiveV2 {
		t.Fatalf("bind Restore Eligibility: %+v err=%v", bundle, err)
	}
	current, err := fixture.gateway.InspectCurrentRestoreEligibilityV2(context.Background(), ports.InspectRestoreEligibilityCurrentRequestV2{Attempt: bundle.Attempt.Ref, ExpectedEligibility: bundle.Eligibility.Ref})
	if err != nil || current.Ref != bundle.Eligibility.Ref {
		t.Fatalf("inspect current Eligibility: %+v err=%v", current, err)
	}
	fixture.inputs.now = fixture.now.Add(time.Nanosecond)
	if _, err := fixture.gateway.InspectCurrentRestoreEligibilityV2(context.Background(), ports.InspectRestoreEligibilityCurrentRequestV2{Attempt: bundle.Attempt.Ref, ExpectedEligibility: bundle.Eligibility.Ref}); err == nil {
		t.Fatal("drifted prerequisite projection left Restore Eligibility current")
	}
	historical, err := fixture.gateway.InspectRestoreAttemptHistoricalV2(context.Background(), attempt.Ref)
	if err != nil || historical.Ref != attempt.Ref {
		t.Fatalf("initial Attempt history lost: %+v err=%v", historical, err)
	}
}

func TestRestoreGovernanceV2S1S2DriftWritesNothing(t *testing.T) {
	fixture := newRestoreGovernanceFixtureV2(t, "plan-drift")
	fixture.plans.driftOnSecond.Store(true)
	if _, err := fixture.gateway.CreateRestoreAttemptV2(context.Background(), fixture.create); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("Plan S1/S2 drift was accepted: %v", err)
	}
	if _, err := fixture.store.InspectRestoreAttemptCurrentV2(context.Background(), ports.InspectRestoreAttemptRequestV2{TenantID: core.TenantID(fixture.create.RestorePlan.TenantID), AttemptID: fixture.create.AttemptID}); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("Plan drift wrote Attempt: %v", err)
	}

	fixture = newRestoreGovernanceFixtureV2(t, "inputs-drift")
	attempt, err := fixture.gateway.CreateRestoreAttemptV2(context.Background(), fixture.create)
	if err != nil {
		t.Fatal(err)
	}
	fixture.inputs.driftOnSecond.Store(true)
	if _, err := fixture.gateway.IssueRestoreEligibilityV2(context.Background(), ports.IssueRestoreEligibilityRequestV2{EligibilityID: "restore-eligibility-input-drift", Attempt: attempt.Ref, RequestedTTL: time.Minute}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("Eligibility S1/S2 drift was accepted: %v", err)
	}
	current, err := fixture.store.InspectRestoreAttemptCurrentV2(context.Background(), ports.InspectRestoreAttemptRequestV2{TenantID: attempt.Ref.TenantID, AttemptID: attempt.Ref.ID})
	if err != nil || current.Ref != attempt.Ref || current.Eligibility != nil {
		t.Fatalf("input drift changed Attempt: %+v err=%v", current, err)
	}
}

func TestRestoreGovernanceV2DifferentContentAndReservationConflict(t *testing.T) {
	fixture := newRestoreGovernanceFixtureV2(t, "conflict")
	first, err := fixture.gateway.CreateRestoreAttemptV2(context.Background(), fixture.create)
	if err != nil {
		t.Fatal(err)
	}
	drift := fixture.create
	drift.IdempotencyKey = "different-idempotency-conflict"
	if _, err := fixture.gateway.CreateRestoreAttemptV2(context.Background(), drift); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same Attempt ID different content was accepted: %v", err)
	}
	other := fixture.create
	other.AttemptID = "restore-attempt-conflict-other"
	other.IdempotencyKey = "restore-idempotency-conflict-other"
	if _, err := fixture.gateway.CreateRestoreAttemptV2(context.Background(), other); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same target Instance/Lease was reserved twice: %v", err)
	}
	current, _ := fixture.gateway.InspectRestoreAttemptV2(context.Background(), ports.InspectRestoreAttemptRequestV2{TenantID: first.Ref.TenantID, AttemptID: first.Ref.ID})
	if current.Ref != first.Ref {
		t.Fatal("conflict changed original Attempt")
	}
}

func TestRestoreGovernanceV2ConcurrentCreateAndBindSingleWinner(t *testing.T) {
	fixture := newRestoreGovernanceFixtureV2(t, "concurrent")
	var attempt ports.RestoreAttemptFactV2
	var firstErr error
	var mu sync.Mutex
	var wg sync.WaitGroup
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			created, err := fixture.gateway.CreateRestoreAttemptV2(context.Background(), fixture.create)
			mu.Lock()
			defer mu.Unlock()
			if err != nil && firstErr == nil {
				firstErr = err
			}
			if err == nil {
				if attempt.Ref.ID != "" && attempt.Ref != created.Ref {
					firstErr = core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "concurrent create refs drifted")
				}
				attempt = created
			}
		}()
	}
	wg.Wait()
	if firstErr != nil || attempt.Ref.ID == "" {
		t.Fatalf("concurrent create: %+v err=%v", attempt, firstErr)
	}

	results := make(chan ports.RestoreEligibilityBindBundleV2, 64)
	errors := make(chan error, 64)
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bundle, err := fixture.gateway.IssueRestoreEligibilityV2(context.Background(), ports.IssueRestoreEligibilityRequestV2{EligibilityID: "restore-eligibility-concurrent", Attempt: attempt.Ref, RequestedTTL: time.Minute})
			if err != nil {
				errors <- err
				return
			}
			results <- bundle
		}()
	}
	wg.Wait()
	close(results)
	close(errors)
	var winner ports.RestoreEligibilityBindBundleV2
	for result := range results {
		if winner.Eligibility.Ref.ID == "" {
			winner = result
		} else if winner.Eligibility.Ref != result.Eligibility.Ref {
			t.Fatal("concurrent bind produced multiple Eligibility facts")
		}
	}
	if winner.Eligibility.Ref.ID == "" {
		t.Fatalf("no bind winner; errors=%d", len(errors))
	}
	for err := range errors {
		if !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("unexpected concurrent bind error: %v", err)
		}
	}
}

func TestRestoreGovernanceV2EligibilityCASNoABA(t *testing.T) {
	fixture := newRestoreGovernanceFixtureV2(t, "cas")
	attempt, err := fixture.gateway.CreateRestoreAttemptV2(context.Background(), fixture.create)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := fixture.gateway.IssueRestoreEligibilityV2(context.Background(), ports.IssueRestoreEligibilityRequestV2{EligibilityID: "restore-eligibility-cas", Attempt: attempt.Ref, RequestedTTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	next := bundle.Eligibility.Clone()
	next.Ref.Revision++
	next.State = ports.RestoreEligibilityRevokedV2
	next.InvalidationReason = core.ReasonBindingDrift
	next.UpdatedUnixNano = fixture.now.Add(time.Second).UnixNano()
	next, err = ports.SealRestoreEligibilityFactV2(next)
	if err != nil {
		t.Fatal(err)
	}
	fixture.store.LoseNextRestoreReplyV2()
	updated, err := fixture.gateway.CompareAndSwapRestoreEligibilityV2(context.Background(), ports.RestoreEligibilityCASRequestV2{Expected: bundle.Eligibility.Ref, Next: next})
	if err != nil || updated.Ref != next.Ref {
		t.Fatalf("lost CAS reply recovery: %+v err=%v", updated, err)
	}
	if _, err := fixture.gateway.CompareAndSwapRestoreEligibilityV2(context.Background(), ports.RestoreEligibilityCASRequestV2{Expected: bundle.Eligibility.Ref, Next: next}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("stale CAS/ABA was accepted: %v", err)
	}
	if _, err := fixture.gateway.InspectCurrentRestoreEligibilityV2(context.Background(), ports.InspectRestoreEligibilityCurrentRequestV2{Attempt: bundle.Attempt.Ref, ExpectedEligibility: bundle.Eligibility.Ref}); err == nil {
		t.Fatal("revoked Eligibility remained current")
	}
}

func TestRestoreGovernanceV2CrossTenantAndTypedNil(t *testing.T) {
	first := newRestoreGovernanceFixtureV2(t, "tenant-a")
	second := newRestoreGovernanceFixtureV2(t, "tenant-b")
	secondPlan := second.plans.projection
	secondPlan.RestorePlan.TenantID = "tenant-restore-fixture-b"
	secondPlan.CheckpointConsistency.Ref.Attempt.TenantID = "tenant-restore-fixture-b"
	// A partially spliced Plan cannot be resealed into a valid current projection.
	if _, err := ports.SealRestorePlanCurrentProjectionV2(secondPlan, second.now); err == nil {
		t.Fatal("cross-tenant Plan splice was accepted")
	}
	var store *fakes.RestoreGovernanceStoreV2
	first.gateway.Facts = store
	if _, err := first.gateway.CreateRestoreAttemptV2(context.Background(), first.create); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("typed-nil Fact Owner was accepted: %v", err)
	}
}

func TestRestoreGovernanceV2NoExecutionSurface(t *testing.T) {
	typeOf := reflect.TypeOf((*ports.RestoreGovernancePortV2)(nil)).Elem()
	for _, forbidden := range []string{"Stage", "Activate", "Provider", "Permit", "Authorization", "Admission", "Execute", "Rollback"} {
		for index := 0; index < typeOf.NumMethod(); index++ {
			if containsFoldRestoreV2(typeOf.Method(index).Name, forbidden) {
				t.Fatalf("Restore governance public Port exposes forbidden capability %q", typeOf.Method(index).Name)
			}
		}
	}
	eligibility := reflect.TypeOf(ports.RestoreEligibilityFactV2{})
	for _, forbidden := range []string{"Verdict", "Authorization", "Permit"} {
		if _, ok := eligibility.FieldByName(forbidden); ok {
			t.Fatalf("Eligibility embeds accepted %s", forbidden)
		}
	}
}

type restoreGovernanceFixtureV2 struct {
	now     time.Time
	store   *fakes.RestoreGovernanceStoreV2
	plans   *restorePlanReaderV2
	inputs  *restoreInputsReaderV2
	gateway kernel.RestoreGovernanceGatewayV2
	create  ports.CreateRestoreAttemptRequestV2
}

func newRestoreGovernanceFixtureV2(t *testing.T, suffix string) restoreGovernanceFixtureV2 {
	t.Helper()
	now := time.Unix(1_780_000_000, 0).UTC()
	plan, err := fakes.BuildRestorePlanCurrentFixtureV2(suffix, now)
	if err != nil {
		t.Fatal(err)
	}
	plans := &restorePlanReaderV2{projection: plan, now: now}
	inputs := &restoreInputsReaderV2{now: now, tenant: core.TenantID(plan.RestorePlan.TenantID), scopeDigest: plan.SourceScopeDigest}
	store := fakes.NewRestoreGovernanceStoreV2()
	gateway := kernel.RestoreGovernanceGatewayV2{Facts: store, Plans: plans, Inputs: inputs, Clock: func() time.Time { return now }}
	create := ports.CreateRestoreAttemptRequestV2{AttemptID: "restore-attempt-" + suffix, IdempotencyKey: "restore-idempotency-" + suffix, RestorePlan: plan.RestorePlan, RequestedNotAfter: now.Add(5 * time.Minute).UnixNano()}
	return restoreGovernanceFixtureV2{now: now, store: store, plans: plans, inputs: inputs, gateway: gateway, create: create}
}

type restorePlanReaderV2 struct {
	projection    ports.RestorePlanCurrentProjectionV2
	now           time.Time
	calls         atomic.Int32
	driftOnSecond atomic.Bool
}

func (r *restorePlanReaderV2) InspectRestorePlanCurrentV2(_ context.Context, expected ports.CheckpointExternalExactFactRefV2) (ports.RestorePlanCurrentProjectionV2, error) {
	if expected != r.projection.RestorePlan {
		return ports.RestorePlanCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore Plan exact ref drift")
	}
	projection := r.projection
	if r.driftOnSecond.Load() && r.calls.Add(1) == 2 {
		projection.ExpiresUnixNano--
		projection.ProjectionDigest = ""
		return ports.SealRestorePlanCurrentProjectionV2(projection, r.now)
	}
	if !r.driftOnSecond.Load() {
		r.calls.Add(1)
	}
	return projection, nil
}

type restoreInputsReaderV2 struct {
	now           time.Time
	tenant        core.TenantID
	scopeDigest   core.Digest
	calls         atomic.Int32
	driftOnSecond atomic.Bool
}

func (r *restoreInputsReaderV2) InspectRestoreEligibilityInputsCurrentV2(_ context.Context, attempt ports.RestoreAttemptFactV2) (ports.RestoreEligibilityInputsCurrentProjectionV2, error) {
	values := func(kind string) []ports.CheckpointExternalExactFactRefV2 {
		return []ports.CheckpointExternalExactFactRefV2{restoreExternalRefV2(r.tenant, r.scopeDigest, kind+"-fact", kind)}
	}
	projection := ports.RestoreEligibilityInputsCurrentProjectionV2{
		Attempt: attempt.Ref, OperationScopeDigest: attempt.OperationScope.Digest, SourceScopeDigest: r.scopeDigest,
		ReviewTarget:          ports.OperationReviewTargetRefV4{Ref: "restore-review-target", Revision: 1, Digest: restoreDigestV2("restore-review-target")},
		ReviewRequirementRefs: values("review-requirement"), PolicyBasisRefs: values("policy-basis"), AuthorityRequirementRefs: values("authority"),
		ScopeRequirementRefs: values("scope"), BudgetRequirementRefs: values("budget"), BindingRequirementRefs: values("binding"), ContextRequirementRefs: values("context"),
		CheckedUnixNano: r.now.UnixNano(), ExpiresUnixNano: r.now.Add(3 * time.Minute).UnixNano(),
	}
	if r.driftOnSecond.Load() && r.calls.Add(1) == 2 {
		projection.ExpiresUnixNano--
	} else if !r.driftOnSecond.Load() {
		r.calls.Add(1)
	}
	return ports.SealRestoreEligibilityInputsCurrentProjectionV2(projection, r.now)
}

func restoreExternalRefV2(tenant core.TenantID, scope core.Digest, id, kind string) ports.CheckpointExternalExactFactRefV2 {
	digest := restoreDigestV2(id + kind)
	return ports.CheckpointExternalExactFactRefV2{
		ContractVersion: "praxis.test/" + kind + "/v1", SchemaRef: "praxis.test/" + kind + "-fact/v1",
		Owner:    ports.CheckpointManifestSealOwnerBindingV2{BindingSetID: "binding-set-" + kind, BindingRevision: 1, ComponentID: "praxis/" + kind, ManifestDigest: string(restoreDigestV2("manifest-" + kind)), ArtifactDigest: string(restoreDigestV2("artifact-" + kind)), Capability: kind + "-current", FactKind: kind + "-fact"},
		TenantID: string(tenant), ID: id, Revision: 1, Digest: string(digest), ScopeDigest: string(scope),
	}
}

func restoreDigestV2(value string) core.Digest { return core.DigestBytes([]byte(value)) }

func containsFoldRestoreV2(value, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		match := true
		for offset := range fragment {
			a, b := value[index+offset], fragment[offset]
			if a >= 'a' && a <= 'z' {
				a -= 'a' - 'A'
			}
			if b >= 'a' && b <= 'z' {
				b -= 'a' - 'A'
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

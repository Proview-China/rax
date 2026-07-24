package review_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	reviewsqlite "github.com/Proview-China/rax/ExecutionRuntime/review/storage/sqlite"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type bypassTestStore interface {
	reviewport.StoreV1
	reviewport.BypassStoreV1
}

type lostReplyBypassStore struct {
	bypassTestStore
	createCalls int
	casCalls    int
}

func (s *lostReplyBypassStore) CreateBypassDecisionV1(ctx context.Context, m reviewport.CreateBypassDecisionMutationV1) (contract.BypassDecisionV1, error) {
	s.createCalls++
	if _, err := s.bypassTestStore.CreateBypassDecisionV1(ctx, m); err != nil {
		return contract.BypassDecisionV1{}, err
	}
	return contract.BypassDecisionV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected lost create reply")
}

func (s *lostReplyBypassStore) CompareAndSwapBypassDecisionV1(ctx context.Context, m reviewport.BypassDecisionCASMutationV1) (contract.BypassDecisionV1, error) {
	s.casCalls++
	if _, err := s.bypassTestStore.CompareAndSwapBypassDecisionV1(ctx, m); err != nil {
		return contract.BypassDecisionV1{}, err
	}
	return contract.BypassDecisionV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected lost CAS reply")
}

func bypassDigest(s string) core.Digest { return core.DigestBytes([]byte("bypass-store-" + s)) }

func bypassDecisionFixture(t *testing.T, tenant core.TenantID, id string, now time.Time) contract.BypassDecisionV1 {
	t.Helper()
	scope := core.ExecutionScope{
		Identity: core.AgentIdentityRef{TenantID: tenant, ID: "agent-bypass", Epoch: 1},
		Lineage:  core.LineageRef{ID: "lineage-bypass", PlanDigest: bypassDigest("plan")},
		Instance: core.InstanceRef{ID: "instance-bypass", Epoch: 3}, AuthorityEpoch: 2,
	}
	policy := runtimeports.ReviewPolicyBindingRefV2{Ref: "policy-bypass", Revision: 4, Digest: bypassDigest("policy")}
	policyCurrent := runtimeports.ReviewDecisionPolicyCurrentProjectionRefV1{ID: "policy-current-1", Revision: 5, Digest: bypassDigest("policy-current")}
	expires := now.Add(time.Hour).UnixNano()
	proof, err := contract.SealBypassExternalCurrentProofV1(contract.BypassExternalCurrentProofV1{Policy: policyCurrent, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires})
	if err != nil {
		t.Fatal(err)
	}
	value, err := contract.SealBypassDecisionV1(contract.BypassDecisionV1{
		FactIdentityV1: contract.FactIdentityV1{TenantID: tenant, ID: id, Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()},
		Target:         contract.BypassTargetExactRefV1{TenantID: tenant, ID: "target-" + id, Revision: 2, Digest: bypassDigest("target-" + id)},
		Case:           contract.BypassCaseExactRefV1{TenantID: tenant, ID: "case-" + id, Revision: 3, Digest: bypassDigest("case-" + id)},
		IntentID:       core.EffectIntentID("intent-" + id), IntentRevision: 2, SubjectDigest: bypassDigest("subject-" + id), PayloadRevision: 7, PayloadDigest: bypassDigest("payload-" + id),
		Scope: scope, RunID: core.AgentRunID("run-" + id), ActionScopeDigest: bypassDigest("action-scope-" + id), Policy: policy, PolicyCurrentProjection: policyCurrent,
		PolicyDecisionRef:       "policy-decision-" + id,
		ActorAuthority:          runtimeports.AuthorityBindingRefV2{Ref: "authority-" + id, Revision: 8, Digest: bypassDigest("authority-" + id), Epoch: 2},
		CurrentScope:            runtimeports.ExecutionScopeBindingRefV2{Ref: "scope-current-" + id, Revision: 9, Digest: bypassDigest("scope-" + id)},
		TargetEvidenceSetDigest: bypassDigest("evidence-" + id), Profile: contract.ProfileYOLOV1, Risk: contract.RiskLowV1, EffectClass: contract.EffectObserveOnlyV1, Environment: contract.EnvironmentProductionV1,
		RouteDecisionDigest: bypassDigest("route-" + id), ExternalProof: proof, State: contract.BypassDecisionActiveV1, ExpiresUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func nextBypassDecision(t *testing.T, current contract.BypassDecisionV1, state contract.BypassDecisionStateV1, at time.Time) contract.BypassDecisionV1 {
	t.Helper()
	next := current
	next.Revision++
	next.UpdatedUnixNano = at.UnixNano()
	next.State = state
	next.InvalidationReason = core.ReasonReviewVerdictStale
	next.Digest = ""
	sealed, err := contract.SealBypassDecisionV1(next)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func bypassTrace(t *testing.T, d contract.BypassDecisionV1, id string, sequence uint64, event contract.TraceEventV1) contract.TraceFactV1 {
	t.Helper()
	trace, err := contract.SealTraceFactV1(contract.TraceFactV1{
		FactIdentityV1: contract.FactIdentityV1{TenantID: d.TenantID, ID: id, Revision: 1, CreatedUnixNano: d.UpdatedUnixNano, UpdatedUnixNano: d.UpdatedUnixNano},
		CaseID:         d.Case.ID, CaseRevision: d.Case.Revision, TargetID: d.Target.ID, TargetRevision: d.Target.Revision, TargetDigest: d.Target.Digest,
		Event: event, SourceID: "bypass-owner", SourceEpoch: 1, SourceSequence: sequence, CausationID: d.ID, CorrelationID: d.Case.ID, FactRefs: []string{d.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	return trace
}

func bypassMutations(t *testing.T, tenant core.TenantID, id string) (reviewport.CreateBypassDecisionMutationV1, reviewport.BypassDecisionCASMutationV1) {
	t.Helper()
	now := time.Unix(1_950_000_000, 0)
	first := bypassDecisionFixture(t, tenant, id, now)
	next := nextBypassDecision(t, first, contract.BypassDecisionRevokedV1, now.Add(time.Second))
	return reviewport.CreateBypassDecisionMutationV1{Decision: first, Trace: bypassTrace(t, first, "trace-create-"+id, 1, contract.TraceRoutedV1)}, reviewport.BypassDecisionCASMutationV1{Expected: first.ExactRef(), Next: next, Trace: bypassTrace(t, next, "trace-revoke-"+id, 2, contract.TraceRevokedV1)}
}

func runBypassConformance(t *testing.T, store bypassTestStore) {
	t.Helper()
	create, next := bypassMutations(t, "tenant-bypass-store", "decision-1")
	if err := conformance.CheckBypassStoreV1(context.Background(), store, conformance.BypassStoreFixtureV1{Create: create, Next: next}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InspectTraceExactV1(context.Background(), create.Trace.TenantID, reviewport.ExactV1(create.Trace.ID, create.Trace.Revision, create.Trace.Digest)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InspectTraceExactV1(context.Background(), next.Trace.TenantID, reviewport.ExactV1(next.Trace.ID, next.Trace.Revision, next.Trace.Digest)); err != nil {
		t.Fatal(err)
	}
}

func TestBypassStoreV1MemoryConformance(t *testing.T) { runBypassConformance(t, memory.NewStore()) }

func TestBypassStoreV1SQLiteRestartConformance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review-bypass.db")
	store, err := reviewsqlite.Open(context.Background(), reviewsqlite.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	runBypassConformance(t, store)
	create, next := bypassMutations(t, "tenant-bypass-sqlite", "decision-sqlite")
	if _, err = store.CreateBypassDecisionV1(context.Background(), create); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CompareAndSwapBypassDecisionV1(context.Background(), next); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = reviewsqlite.Open(context.Background(), reviewsqlite.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err = store.InspectBypassDecisionExactV1(context.Background(), create.Decision.ExactRef()); err != nil {
		t.Fatal(err)
	}
	current, err := store.InspectCurrentBypassDecisionByCaseV1(context.Background(), create.Decision.Case)
	if err != nil || current.Digest != next.Next.Digest {
		t.Fatalf("restart current drift: %v", err)
	}
	if err = store.IntegrityCheckV1(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestBypassStoreV1ConcurrentCanonicalCASAndTenantIsolation(t *testing.T) {
	store := memory.NewStore()
	create, next := bypassMutations(t, "tenant-bypass-a", "decision-race")
	if _, err := store.CreateBypassDecisionV1(context.Background(), create); err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 64)
	var wg sync.WaitGroup
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := store.CompareAndSwapBypassDecisionV1(context.Background(), next)
			if err == nil && got.Digest != next.Next.Digest {
				err = core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "concurrent replay returned wrong Decision")
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	other, _ := bypassMutations(t, "tenant-bypass-b", "decision-race")
	if _, err := store.CreateBypassDecisionV1(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InspectCurrentBypassDecisionByCaseV1(context.Background(), other.Decision.Case); err != nil {
		t.Fatal(err)
	}
}

func TestBypassStoreV1StagedTraceConflictLeaksNoDecision(t *testing.T) {
	store := memory.NewStore()
	create, _ := bypassMutations(t, "tenant-bypass-stage", "decision-stage")
	conflict := create.Trace
	conflict.ID = "trace-existing"
	conflict.FactRefs = []string{"different"}
	conflict.Digest = ""
	var err error
	conflict, err = contract.SealTraceFactV1(conflict)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.InjectTraceForTestV1(context.Background(), conflict); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateBypassDecisionV1(context.Background(), create); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("want staged trace conflict, got %v", err)
	}
	if _, err = store.InspectBypassDecisionExactV1(context.Background(), create.Decision.ExactRef()); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("failed create leaked Decision: %v", err)
	}
	if _, err = store.InspectCurrentBypassDecisionByCaseV1(context.Background(), create.Decision.Case); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("failed create leaked current index: %v", err)
	}
}

func TestBypassStoreV1ChangedReplayAndStaleCASConflict(t *testing.T) {
	store := memory.NewStore()
	create, next := bypassMutations(t, "tenant-bypass-conflict", "decision-conflict")
	if _, err := store.CreateBypassDecisionV1(context.Background(), create); err != nil {
		t.Fatal(err)
	}
	changed := create
	changed.Decision.RouteDecisionDigest = bypassDigest("changed")
	changed.Decision.Digest = ""
	changed.Decision, _ = contract.SealBypassDecisionV1(changed.Decision)
	if _, err := store.CreateBypassDecisionV1(context.Background(), changed); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("changed create replay accepted: %v", err)
	}
	withoutTrace := create
	withoutTrace.Trace = contract.TraceFactV1{}
	if _, err := store.CreateBypassDecisionV1(context.Background(), withoutTrace); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("create replay changed optional Trace presence: %v", err)
	}
	if _, err := store.CompareAndSwapBypassDecisionV1(context.Background(), next); err != nil {
		t.Fatal(err)
	}
	wrongExpectedReplay := next
	wrongExpectedReplay.Expected.Digest = bypassDigest("wrong-expected")
	if _, err := store.CompareAndSwapBypassDecisionV1(context.Background(), wrongExpectedReplay); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("CAS replay changed exact expected ref: %v", err)
	}
	third := nextBypassDecision(t, next.Next, contract.BypassDecisionSupersededV1, time.Unix(1_950_000_002, 0))
	stale := reviewport.BypassDecisionCASMutationV1{Expected: create.Decision.ExactRef(), Next: third}
	if _, err := store.CompareAndSwapBypassDecisionV1(context.Background(), stale); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("stale CAS accepted: %v", err)
	}
	if _, err := store.InspectBypassDecisionExactV1(context.Background(), create.Decision.ExactRef()); err != nil {
		t.Fatalf("historical exact read depends on current: %v", err)
	}
}

func TestBypassStoreV1TerminalCannotTransitionAgainOrReturnActive(t *testing.T) {
	store := memory.NewStore()
	create, revoke := bypassMutations(t, "tenant-bypass-terminal", "decision-terminal")
	if _, err := store.CreateBypassDecisionV1(context.Background(), create); err != nil {
		t.Fatal(err)
	}

	activeAgain := revoke
	activeAgain.Next = create.Decision
	activeAgain.Next.Revision++
	activeAgain.Next.UpdatedUnixNano = time.Unix(1_950_000_001, 0).UnixNano()
	activeAgain.Next.Digest = ""
	activeAgain.Next, _ = contract.SealBypassDecisionV1(activeAgain.Next)
	if _, err := store.CompareAndSwapBypassDecisionV1(context.Background(), activeAgain); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("active-to-active CAS accepted: %v", err)
	}

	if _, err := store.CompareAndSwapBypassDecisionV1(context.Background(), revoke); err != nil {
		t.Fatal(err)
	}
	terminalAgain := reviewport.BypassDecisionCASMutationV1{
		Expected: revoke.Next.ExactRef(),
		Next:     nextBypassDecision(t, revoke.Next, contract.BypassDecisionSupersededV1, time.Unix(1_950_000_002, 0)),
	}
	if _, err := store.CompareAndSwapBypassDecisionV1(context.Background(), terminalAgain); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("terminal-to-terminal CAS accepted: %v", err)
	}
	if _, err := store.InspectBypassDecisionExactV1(context.Background(), terminalAgain.Next.ExactRef()); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("rejected terminal CAS leaked history: %v", err)
	}
	current, err := store.InspectCurrentBypassDecisionByCaseV1(context.Background(), create.Decision.Case)
	if err != nil || current.Digest != revoke.Next.Digest {
		t.Fatalf("rejected terminal CAS moved current: %v", err)
	}
}

func TestBypassStoreV1CASStagedTraceConflictLeaksNoRevision(t *testing.T) {
	store := memory.NewStore()
	create, next := bypassMutations(t, "tenant-bypass-cas-stage", "decision-cas-stage")
	if _, err := store.CreateBypassDecisionV1(context.Background(), create); err != nil {
		t.Fatal(err)
	}
	conflict := next.Trace
	conflict.ID = "trace-existing-cas"
	conflict.FactRefs = []string{"different"}
	conflict.Digest = ""
	var err error
	conflict, err = contract.SealTraceFactV1(conflict)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.InjectTraceForTestV1(context.Background(), conflict); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CompareAndSwapBypassDecisionV1(context.Background(), next); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("want staged CAS trace conflict, got %v", err)
	}
	if _, err = store.InspectBypassDecisionExactV1(context.Background(), next.Next.ExactRef()); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("failed CAS leaked next history: %v", err)
	}
	current, err := store.InspectCurrentBypassDecisionByCaseV1(context.Background(), create.Decision.Case)
	if err != nil || current.Digest != create.Decision.Digest {
		t.Fatalf("failed CAS moved current index: %v", err)
	}
}

func TestBypassStoreV1LostReplyRecoversOnlyByExactInspect(t *testing.T) {
	base := memory.NewStore()
	store := &lostReplyBypassStore{bypassTestStore: base}
	create, next := bypassMutations(t, "tenant-bypass-lost", "decision-lost")
	ctx, cancel := context.WithCancel(context.Background())
	if _, err := store.CreateBypassDecisionV1(ctx, create); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("want injected create unknown outcome, got %v", err)
	}
	cancel()
	detached := context.WithoutCancel(ctx)
	if _, err := base.InspectBypassDecisionExactV1(detached, create.Decision.ExactRef()); err != nil {
		t.Fatalf("create lost reply exact recovery failed: %v", err)
	}
	if _, err := base.InspectTraceExactV1(detached, create.Trace.TenantID, reviewport.ExactV1(create.Trace.ID, create.Trace.Revision, create.Trace.Digest)); err != nil {
		t.Fatalf("create lost reply Trace recovery failed: %v", err)
	}
	if _, err := store.CompareAndSwapBypassDecisionV1(context.Background(), next); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("want injected CAS unknown outcome, got %v", err)
	}
	if _, err := base.InspectBypassDecisionExactV1(context.Background(), next.Next.ExactRef()); err != nil {
		t.Fatalf("CAS lost reply exact recovery failed: %v", err)
	}
	if store.createCalls != 1 || store.casCalls != 1 {
		t.Fatalf("lost reply recovery replayed a mutation: create=%d cas=%d", store.createCalls, store.casCalls)
	}
}

func TestBypassSnapshotV1LegacyNilExtensionRoundTrip(t *testing.T) {
	store := memory.NewStore()
	snapshot, err := store.ExportSnapshotV1("tenant-bypass-legacy")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Bypass != nil {
		t.Fatal("empty legacy-compatible snapshot unexpectedly materialized bypass extension")
	}
	if _, err = memory.NewStoreFromSnapshotV1(snapshot); err != nil {
		t.Fatal(err)
	}
}

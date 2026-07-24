package sqlite

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestSQLiteOperationReviewAuthorizationV5RestartHistoryDeepCloneAndABA(t *testing.T) {
	now := time.Unix(2_410_000_000, 0)
	path := testDBPath(t)
	store := openTestStore(t, path, func() time.Time { return now })
	fact := sqliteReviewAuthorizationFactV5(t, now, "tenant-a", "operation-a", "effect-a", "authorization-a")
	created, err := store.CreateOperationReviewAuthorizationV5(context.Background(), fact)
	if err != nil || created.Digest != fact.Digest {
		t.Fatalf("create V5 failed: %+v %v", created, err)
	}
	aliased, err := store.InspectOperationReviewAuthorizationExactV5(context.Background(), fact.RefV5())
	if err != nil {
		t.Fatal(err)
	}
	aliased.Review.Quorum.DecisionEvidence[0].RecordDigest = core.DigestBytes([]byte("mutated-alias"))
	clean, err := store.InspectOperationReviewAuthorizationExactV5(context.Background(), fact.RefV5())
	if err != nil || !reflect.DeepEqual(clean, fact) {
		t.Fatalf("exact history leaked alias: %+v %v", clean, err)
	}
	next := sqliteTerminalReviewAuthorizationFactV5(t, fact, now, ports.OperationReviewAuthorizationRevokedV5)
	if _, err := store.CompareAndSwapOperationReviewAuthorizationV5(context.Background(), ports.OperationReviewAuthorizationCASRequestV5{ExpectedRevision: fact.Revision, Next: next}); err != nil {
		t.Fatal(err)
	}
	if historical, err := store.InspectOperationReviewAuthorizationExactV5(context.Background(), fact.RefV5()); err != nil || historical.Digest != fact.Digest {
		t.Fatalf("terminal CAS overwrote history: %+v %v", historical, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTestStore(t, path, func() time.Time { return now })
	if current, err := reopened.InspectOperationReviewAuthorizationV5(context.Background(), fact.ID); err != nil || current.Digest != next.Digest {
		t.Fatalf("restart lost current terminal: %+v %v", current, err)
	}
	if old, err := reopened.InspectOperationReviewAuthorizationExactV5(context.Background(), fact.RefV5()); err != nil || old.Digest != fact.Digest {
		t.Fatalf("restart lost exact history: %+v %v", old, err)
	}
	if _, err := reopened.db.Exec(`UPDATE runtime_operation_review_authorization_current SET highest_revision=? WHERE contract_version=? AND authorization_id=?`, next.Revision+1, operationReviewAuthorizationVersionV5, fact.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.InspectOperationReviewAuthorizationV5(context.Background(), fact.ID); !core.HasReason(err, core.ReasonRevisionConflict) {
		t.Fatalf("highest-revision ABA corruption was accepted: %v", err)
	}
	if old, err := reopened.InspectOperationReviewAuthorizationExactV5(context.Background(), fact.RefV5()); err != nil || old.Digest != fact.Digest {
		t.Fatalf("bad current index contaminated exact history: %+v %v", old, err)
	}
	if _, err := reopened.db.Exec(`UPDATE runtime_operation_review_authorization_current SET revision=?,fact_digest=?,highest_revision=? WHERE contract_version=? AND authorization_id=?`, fact.Revision, string(fact.Digest), fact.Revision, operationReviewAuthorizationVersionV5, fact.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.InspectOperationReviewAuthorizationV5(context.Background(), fact.ID); !core.HasReason(err, core.ReasonRevisionConflict) {
		t.Fatalf("full current/highest ABA rollback was accepted despite newer history: %v", err)
	}
}

func TestSQLiteOperationReviewAuthorizationV5CreateOnceAndInitialSeal(t *testing.T) {
	now := time.Unix(2_415_000_000, 0)
	store := openTestStore(t, testDBPath(t), func() time.Time { return now })
	fact := sqliteReviewAuthorizationFactV5(t, now, "tenant-create", "operation-create", "effect-create", "authorization-create")
	if _, err := store.CreateOperationReviewAuthorizationV5(context.Background(), fact); err != nil {
		t.Fatal(err)
	}
	if replay, err := store.CreateOperationReviewAuthorizationV5(context.Background(), fact); err != nil || replay.Digest != fact.Digest {
		t.Fatalf("same canonical Create was not idempotent: %+v %v", replay, err)
	}
	changed := sqliteReviewAuthorizationFactV5(t, now, "tenant-create", "operation-drift", "effect-drift", fact.ID)
	if _, err := store.CreateOperationReviewAuthorizationV5(context.Background(), changed); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("same ID changed content was accepted: %v", err)
	}
	assertSQLiteReviewAuthorizationRows(t, store, 1, 1, 1)

	badInitial := sqliteReviewAuthorizationFactV5(t, now, "tenant-create", "operation-bad-initial", "effect-bad-initial", "authorization-bad-initial")
	badInitial.UpdatedUnixNano++
	badInitial, err := ports.SealOperationReviewAuthorizationFactV5(badInitial)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateOperationReviewAuthorizationV5(context.Background(), badInitial); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("revision-one fact with Updated!=Created was accepted: %v", err)
	}
	assertSQLiteReviewAuthorizationRows(t, store, 1, 1, 1)
}

func TestSQLiteOperationReviewAuthorizationV5StagedFailureAndLostReplyAreAtomic(t *testing.T) {
	now := time.Unix(2_420_000_000, 0)
	store := openTestStore(t, testDBPath(t), func() time.Time { return now })
	staged := sqliteReviewAuthorizationFactV5(t, now, "tenant-stage", "operation-stage", "effect-stage", "authorization-stage")
	store.failNextStageForTest()
	if _, err := store.CreateOperationReviewAuthorizationV5(context.Background(), staged); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("staged Create did not fail unavailable: %v", err)
	}
	assertSQLiteReviewAuthorizationRows(t, store, 0, 0, 0)
	if _, err := store.InspectOperationReviewAuthorizationExactV5(context.Background(), staged.RefV5()); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("staged Create leaked exact history: %v", err)
	}

	lost := sqliteReviewAuthorizationFactV5(t, now, "tenant-lost", "operation-lost", "effect-lost", "authorization-lost")
	store.loseNextReplyForTest()
	if _, err := store.CreateOperationReviewAuthorizationV5(context.Background(), lost); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost Create reply was not indeterminate: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if recovered, err := store.InspectOperationReviewAuthorizationExactV5(context.WithoutCancel(ctx), lost.RefV5()); err != nil || recovered.Digest != lost.Digest {
		t.Fatalf("lost Create did not recover by detached exact Inspect: %+v %v", recovered, err)
	}

	stageNext := sqliteTerminalReviewAuthorizationFactV5(t, lost, now, ports.OperationReviewAuthorizationRevokedV5)
	store.failNextStageForTest()
	if _, err := store.CompareAndSwapOperationReviewAuthorizationV5(context.Background(), ports.OperationReviewAuthorizationCASRequestV5{ExpectedRevision: lost.Revision, Next: stageNext}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("staged CAS did not fail unavailable: %v", err)
	}
	if current, err := store.InspectOperationReviewAuthorizationV5(context.Background(), lost.ID); err != nil || current.Digest != lost.Digest {
		t.Fatalf("staged CAS changed current: %+v %v", current, err)
	}
	if _, err := store.InspectOperationReviewAuthorizationExactV5(context.Background(), stageNext.RefV5()); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("staged CAS leaked next history: %v", err)
	}
	assertSQLiteReviewAuthorizationRows(t, store, 1, 1, 1)

	store.loseNextReplyForTest()
	if _, err := store.CompareAndSwapOperationReviewAuthorizationV5(context.Background(), ports.OperationReviewAuthorizationCASRequestV5{ExpectedRevision: lost.Revision, Next: stageNext}); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost CAS reply was not indeterminate: %v", err)
	}
	if recovered, err := store.InspectOperationReviewAuthorizationExactV5(context.WithoutCancel(ctx), stageNext.RefV5()); err != nil || recovered.Digest != stageNext.Digest {
		t.Fatalf("lost CAS did not recover exact terminal: %+v %v", recovered, err)
	}
	assertSQLiteReviewAuthorizationRows(t, store, 2, 1, 0)
}

func TestSQLiteOperationReviewAuthorizationV45SharedGuardConcurrent(t *testing.T) {
	now := time.Unix(2_430_000_000, 0)
	store := openTestStore(t, testDBPath(t), func() time.Time { return now })
	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	success := make(chan string, workers)
	v4Facts := make([]ports.OperationReviewAuthorizationFactV4, workers/2)
	v5Facts := make([]ports.OperationReviewAuthorizationFactV5, workers/2)
	for i := 0; i < workers/2; i++ {
		v4Facts[i] = sqliteReviewAuthorizationFactV4(t, now, "tenant-shared", "operation-shared", "effect-shared", fmt.Sprintf("authorization-v4-%d", i*2))
		v5Facts[i] = sqliteReviewAuthorizationFactV5(t, now, "tenant-shared", "operation-shared", "effect-shared", fmt.Sprintf("authorization-v5-%d", i*2+1))
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				fact := v4Facts[i/2]
				_, err := store.CreateOperationReviewAuthorizationV4(context.Background(), fact)
				if err == nil {
					success <- "v4"
				} else {
					errs <- err
				}
				return
			}
			fact := v5Facts[i/2]
			_, err := store.CreateOperationReviewAuthorizationV5(context.Background(), fact)
			if err == nil {
				success <- "v5"
			} else {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(success)
	close(errs)
	if len(success) != 1 {
		t.Fatalf("V4/V5 shared guard produced %d current facts", len(success))
	}
	for err := range errs {
		if !core.HasReason(err, core.ReasonEffectConflictDomainOccupied) {
			t.Fatalf("unexpected shared guard loser: %v", err)
		}
	}
	assertSQLiteReviewAuthorizationRows(t, store, 1, 1, 1)
}

func TestSQLiteOperationReviewAuthorizationV45SharedGuardReleasedOnlyByTerminalCAS(t *testing.T) {
	now := time.Unix(2_435_000_000, 0)
	store := openTestStore(t, testDBPath(t), func() time.Time { return now })
	v4 := sqliteReviewAuthorizationFactV4(t, now, "tenant-release", "operation-release", "effect-release", "authorization-release-v4")
	if _, err := store.CreateOperationReviewAuthorizationV4(context.Background(), v4); err != nil {
		t.Fatal(err)
	}
	v5 := sqliteReviewAuthorizationFactV5(t, now, "tenant-release", "operation-release", "effect-release", "authorization-release-v5")
	if _, err := store.CreateOperationReviewAuthorizationV5(context.Background(), v5); !core.HasReason(err, core.ReasonEffectConflictDomainOccupied) {
		t.Fatalf("V5 entered while V4 was active: %v", err)
	}
	v4Terminal := sqliteTerminalReviewAuthorizationFactV4(t, v4, now, ports.OperationReviewAuthorizationRevokedV4)
	if _, err := store.CompareAndSwapOperationReviewAuthorizationV4(context.Background(), ports.OperationReviewAuthorizationCASRequestV4{ExpectedRevision: v4.Revision, Next: v4Terminal}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateOperationReviewAuthorizationV5(context.Background(), v5); err != nil {
		t.Fatalf("V4 terminal CAS did not release shared guard: %v", err)
	}
	v5Terminal := sqliteTerminalReviewAuthorizationFactV5(t, v5, now, ports.OperationReviewAuthorizationSupersededV5)
	if _, err := store.CompareAndSwapOperationReviewAuthorizationV5(context.Background(), ports.OperationReviewAuthorizationCASRequestV5{ExpectedRevision: v5.Revision, Next: v5Terminal}); err != nil {
		t.Fatal(err)
	}
	v4Again := sqliteReviewAuthorizationFactV4(t, now, "tenant-release", "operation-release", "effect-release", "authorization-release-v4-again")
	if _, err := store.CreateOperationReviewAuthorizationV4(context.Background(), v4Again); err != nil {
		t.Fatalf("V5 terminal CAS did not release shared guard: %v", err)
	}
	assertSQLiteReviewAuthorizationRows(t, store, 5, 3, 1)
}

func TestSQLiteOperationReviewAuthorizationV5TenantAndOperationIsolation(t *testing.T) {
	now := time.Unix(2_440_000_000, 0)
	store := openTestStore(t, testDBPath(t), func() time.Time { return now })
	facts := []ports.OperationReviewAuthorizationFactV5{
		sqliteReviewAuthorizationFactV5(t, now, "tenant-one", "operation-one", "same-effect", "authorization-one"),
		sqliteReviewAuthorizationFactV5(t, now, "tenant-two", "operation-one", "same-effect", "authorization-two"),
		sqliteReviewAuthorizationFactV5(t, now, "tenant-one", "operation-two", "same-effect", "authorization-three"),
	}
	for _, fact := range facts {
		if _, err := store.CreateOperationReviewAuthorizationV5(context.Background(), fact); err != nil {
			t.Fatalf("isolated Operation Effect conflicted: %v", err)
		}
	}
	assertSQLiteReviewAuthorizationRows(t, store, 3, 3, 3)
}

func TestSQLiteOperationReviewAuthorizationV5SchemaMigration(t *testing.T) {
	store := openTestStore(t, testDBPath(t), func() time.Time { return time.Unix(2_450_000_000, 0) })
	var digest string
	if err := store.db.QueryRow(`SELECT digest FROM runtime_binding_schema WHERE version=?`, schemaVersionV3).Scan(&digest); err != nil || digest != string(core.DigestBytes([]byte(schemaV3))) {
		t.Fatalf("schema V3 was not recorded exactly: %q %v", digest, err)
	}
	for _, table := range []string{"runtime_operation_review_authorization_history", "runtime_operation_review_authorization_current", "runtime_operation_review_authorization_active_guard"} {
		var got string
		if err := store.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&got); err != nil || got != table {
			t.Fatalf("schema V3 table %s missing: %q %v", table, got, err)
		}
	}
}

func assertSQLiteReviewAuthorizationRows(t *testing.T, store *Store, history, current, guard int) {
	t.Helper()
	for table, expected := range map[string]int{"runtime_operation_review_authorization_history": history, "runtime_operation_review_authorization_current": current, "runtime_operation_review_authorization_active_guard": guard} {
		var got int
		if err := store.db.QueryRow(`SELECT COUNT(1) FROM ` + table).Scan(&got); err != nil || got != expected {
			t.Fatalf("%s rows=%d expected=%d err=%v", table, got, expected, err)
		}
	}
}

func sqliteReviewAuthorizationBaseV5(t *testing.T, now time.Time, tenant core.TenantID, operationToken string) (ports.OperationSubjectV3, ports.OperationReviewIntentBindingV4, ports.OperationReviewGovernanceBindingV4, core.Digest, int64) {
	t.Helper()
	expires := now.Add(20 * time.Second).UnixNano()
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: tenant, ID: core.AgentIdentityID("identity-" + operationToken), Epoch: 1}, Lineage: core.LineageRef{ID: core.InstanceLineageID("lineage-" + operationToken), PlanDigest: core.DigestBytes([]byte("lineage-" + operationToken))}, Instance: core.InstanceRef{ID: core.AgentInstanceID("instance-" + operationToken), Epoch: 1}, AuthorityEpoch: 1}
	scopeDigest, err := ports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	operation := ports.OperationSubjectV3{Kind: ports.OperationScopeActivationV3, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, ActivationAttemptID: "attempt-" + operationToken, SubjectRevision: 1, CurrentProjectionRef: "scope-" + operationToken, CurrentProjectionRevision: 1, CurrentProjectionDigest: core.DigestBytes([]byte("scope-" + operationToken))}
	provider := ports.ProviderBindingRefV2{BindingSetID: "binding-" + operationToken, BindingSetRevision: 1, ComponentID: "custom/provider", ManifestDigest: core.DigestBytes([]byte("manifest")), ArtifactDigest: core.DigestBytes([]byte("artifact")), Capability: "custom/execute"}
	policyDigest := core.DigestBytes([]byte("review-policy-" + operationToken))
	intentDigest := core.DigestBytes([]byte("intent-" + operationToken))
	intent := ports.OperationReviewIntentBindingV4{Operation: operation, IntentID: "effect-placeholder", IntentRevision: 1, IntentDigest: intentDigest, EffectFactRevision: 2, Target: "target-" + operationToken, PayloadSchema: ports.SchemaRefV2{Namespace: "custom", Name: "payload", Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("schema"))}, PayloadDigest: core.DigestBytes([]byte("payload-" + operationToken)), PayloadRevision: 1, Provider: provider, Authority: ports.AuthorityBindingRefV2{Ref: "authority-" + operationToken, Digest: core.DigestBytes([]byte("authority-" + operationToken)), Revision: 1, Epoch: 1}, ReviewBinding: ports.OperationReviewBindingRefV3{CaseRef: "case-" + operationToken, CandidateDigest: core.DigestBytes([]byte("target-digest-" + operationToken)), CandidateRevision: 1, PolicyDigest: policyDigest}, DispatchPolicy: ports.OperationPolicyBindingRefV3{Ref: "dispatch-policy-" + operationToken, Digest: core.DigestBytes([]byte("dispatch-policy-" + operationToken)), Revision: 1, SubjectDigest: core.DigestBytes([]byte("subject-" + operationToken))}, IntentExpires: expires}
	ref := func(id string, digest core.Digest) ports.OperationGovernanceFactRefV3 {
		return ports.OperationGovernanceFactRefV3{Ref: id, Revision: 1, Digest: digest, ExpiresUnixNano: expires}
	}
	governance := ports.OperationReviewGovernanceBindingV4{SnapshotDigest: core.DigestBytes([]byte("snapshot-" + operationToken)), ProjectionWatermark: 1, Identity: ref("identity-fact-"+operationToken, core.DigestBytes([]byte("identity-fact-"+operationToken))), Binding: ref(provider.BindingSetID, core.DigestBytes([]byte("binding-fact-"+operationToken))), CurrentScope: ref(operation.CurrentProjectionRef, operation.CurrentProjectionDigest), Authority: ref(intent.Authority.Ref, intent.Authority.Digest), Policy: ref(intent.DispatchPolicy.Ref, intent.DispatchPolicy.Digest), Budget: ref("budget-"+operationToken, core.DigestBytes([]byte("budget-"+operationToken))), CapabilityGrantDigest: core.DigestBytes([]byte("capability-" + operationToken)), CredentialGrantDigest: core.DigestBytes([]byte("credential-" + operationToken)), ExpiresUnixNano: expires}
	return operation, intent, governance, policyDigest, expires
}

func sqliteReviewAuthorizationFactV5(t *testing.T, now time.Time, tenant core.TenantID, operationToken string, effectID core.EffectIntentID, id string) ports.OperationReviewAuthorizationFactV5 {
	t.Helper()
	operation, intent, governance, policyDigest, expires := sqliteReviewAuthorizationBaseV5(t, now, tenant, operationToken)
	intent.IntentID = effectID
	ref := func(id string, digest core.Digest) ports.OperationGovernanceFactRefV3 {
		return ports.OperationGovernanceFactRefV3{Ref: id, Revision: 1, Digest: digest, ExpiresUnixNano: expires}
	}
	evidence := []ports.EvidenceRecordRefV2{{LedgerScopeDigest: core.DigestBytes([]byte("ledger-" + operationToken)), Sequence: 1, RecordDigest: core.DigestBytes([]byte("evidence-" + operationToken))}}
	q, err := ports.SealOperationReviewQuorumCurrentProjectionV5(ports.OperationReviewQuorumCurrentProjectionV5{Operation: operation, IntentID: effectID, IntentRevision: intent.IntentRevision, IntentDigest: intent.IntentDigest, PayloadSchema: intent.PayloadSchema, PayloadDigest: intent.PayloadDigest, PayloadRevision: intent.PayloadRevision, Target: ports.OperationReviewTargetRefV4{Ref: intent.Target, Revision: intent.ReviewBinding.CandidateRevision, Digest: intent.ReviewBinding.CandidateDigest}, Case: ports.OperationReviewCaseRefV5{TenantID: tenant, ID: intent.ReviewBinding.CaseRef, Revision: 1, Digest: core.DigestBytes([]byte("case-" + operationToken)), ExpiresUnixNano: expires}, Panel: ports.OperationReviewPanelRefV5{TenantID: tenant, ID: "panel-" + operationToken, Revision: 1, Digest: core.DigestBytes([]byte("panel-" + operationToken)), ExpiresUnixNano: expires}, QuorumDecision: ports.OperationReviewQuorumDecisionRefV5{TenantID: tenant, ID: "quorum-" + operationToken, Revision: 1, Digest: core.DigestBytes([]byte("quorum-" + operationToken)), ExpiresUnixNano: expires}, Verdict: ports.OperationReviewVerdictRefV5{TenantID: tenant, ID: "verdict-" + operationToken, Revision: 1, Digest: core.DigestBytes([]byte("verdict-" + operationToken)), ExpiresUnixNano: expires}, QuorumPolicy: ref("quorum-policy-"+operationToken, policyDigest), ReviewerSetDigest: core.DigestBytes([]byte("reviewers-" + operationToken)), AcceptCount: 2, Threshold: 2, SatisfiedRoleCounts: []ports.OperationReviewRoleCountV5{{Role: "security", Count: 1, Required: 1}}, ReviewerAuthorityRefs: []ports.OperationGovernanceFactRefV3{ref("reviewer-a-"+operationToken, core.DigestBytes([]byte("reviewer-a-"+operationToken))), ref("reviewer-b-"+operationToken, core.DigestBytes([]byte("reviewer-b-"+operationToken)))}, BindingRefs: []ports.OperationGovernanceFactRefV3{ref("review-binding-"+operationToken, core.DigestBytes([]byte("review-binding-"+operationToken)))}, ScopeRef: governance.CurrentScope, DecisionEvidence: evidence, Basis: ports.OperationReviewBasisAcceptedQuorumV5, Current: true, CurrentnessDigest: core.DigestBytes([]byte("current-" + operationToken)), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires}, now)
	if err != nil {
		t.Fatal(err)
	}
	review, err := ports.SealOperationReviewCurrentProjectionV5(ports.OperationReviewCurrentProjectionV5{Basis: ports.OperationReviewBasisAcceptedQuorumV5, Quorum: &q}, now)
	if err != nil {
		t.Fatal(err)
	}
	fence := core.ExecutionFence{BoundaryScope: core.FenceBoundaryActivation, Scope: operation.ExecutionScope, CapabilityGrantDigest: governance.CapabilityGrantDigest, EffectIntentID: effectID, EffectIntentRevision: intent.IntentRevision, CanonicalPayloadDigest: intent.PayloadDigest, ExpiresAt: time.Unix(0, expires)}
	fenceDigest, err := ports.DigestOperationExecutionFenceV3(fence, operation)
	if err != nil {
		t.Fatal(err)
	}
	fact, err := ports.SealOperationReviewAuthorizationFactV5(ports.OperationReviewAuthorizationFactV5{ID: id, Revision: 1, State: ports.OperationReviewAuthorizationActiveV5, Intent: intent, Review: review, Governance: governance, Fence: fence, FenceDigest: fenceDigest, RequestedTTLUnixNano: (20 * time.Second).Nanoseconds(), CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires})
	if err != nil {
		t.Fatal(err)
	}
	return fact
}

func sqliteReviewAuthorizationFactV4(t *testing.T, now time.Time, tenant core.TenantID, operationToken string, effectID core.EffectIntentID, id string) ports.OperationReviewAuthorizationFactV4 {
	t.Helper()
	operation, intent, governance, policyDigest, expires := sqliteReviewAuthorizationBaseV5(t, now, tenant, operationToken)
	intent.IntentID = effectID
	ref := func(id string, digest core.Digest) ports.OperationGovernanceFactRefV3 {
		return ports.OperationGovernanceFactRefV3{Ref: id, Revision: 1, Digest: digest, ExpiresUnixNano: expires}
	}
	review, err := ports.SealOperationReviewCurrentProjectionV4(ports.OperationReviewCurrentProjectionV4{Operation: operation, IntentID: effectID, IntentRevision: intent.IntentRevision, IntentDigest: intent.IntentDigest, PayloadSchema: intent.PayloadSchema, PayloadDigest: intent.PayloadDigest, PayloadRevision: intent.PayloadRevision, Target: ports.OperationReviewTargetRefV4{Ref: intent.Target, Revision: intent.ReviewBinding.CandidateRevision, Digest: intent.ReviewBinding.CandidateDigest}, Case: ref(intent.ReviewBinding.CaseRef, core.DigestBytes([]byte("case-"+operationToken))), Verdict: ref("verdict-v4-"+operationToken, core.DigestBytes([]byte("verdict-v4-"+operationToken))), Basis: ports.OperationReviewBasisAcceptedV4, Policy: ref("review-policy-v4-"+operationToken, policyDigest), ReviewerAuthority: ref("reviewer-authority-v4-"+operationToken, core.DigestBytes([]byte("reviewer-authority-v4-"+operationToken))), Scope: governance.CurrentScope, Binding: governance.Binding, DecisionEvidence: []ports.EvidenceRecordRefV2{{LedgerScopeDigest: core.DigestBytes([]byte("ledger-v4-" + operationToken)), Sequence: 1, RecordDigest: core.DigestBytes([]byte("evidence-v4-" + operationToken))}}, Current: true, CurrentnessDigest: core.DigestBytes([]byte("current-v4-" + operationToken)), ExpiresUnixNano: expires}, now)
	if err != nil {
		t.Fatal(err)
	}
	fence := core.ExecutionFence{BoundaryScope: core.FenceBoundaryActivation, Scope: operation.ExecutionScope, CapabilityGrantDigest: governance.CapabilityGrantDigest, EffectIntentID: effectID, EffectIntentRevision: intent.IntentRevision, CanonicalPayloadDigest: intent.PayloadDigest, ExpiresAt: time.Unix(0, expires)}
	fenceDigest, err := ports.DigestOperationExecutionFenceV3(fence, operation)
	if err != nil {
		t.Fatal(err)
	}
	fact, err := ports.SealOperationReviewAuthorizationFactV4(ports.OperationReviewAuthorizationFactV4{ID: id, Revision: 1, State: ports.OperationReviewAuthorizationActiveV4, Intent: intent, Review: review, Governance: governance, Fence: fence, FenceDigest: fenceDigest, RequestedTTLUnixNano: (20 * time.Second).Nanoseconds(), CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires})
	if err != nil {
		t.Fatal(err)
	}
	return fact
}

func sqliteTerminalReviewAuthorizationFactV5(t *testing.T, current ports.OperationReviewAuthorizationFactV5, now time.Time, state ports.OperationReviewAuthorizationStateV5) ports.OperationReviewAuthorizationFactV5 {
	t.Helper()
	next := current
	next.Revision++
	next.State = state
	next.InvalidationReason = core.ReasonReviewVerdictStale
	next.UpdatedUnixNano = now.UnixNano()
	sealed, err := ports.SealOperationReviewAuthorizationFactV5(next)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func sqliteTerminalReviewAuthorizationFactV4(t *testing.T, current ports.OperationReviewAuthorizationFactV4, now time.Time, state ports.OperationReviewAuthorizationStateV4) ports.OperationReviewAuthorizationFactV4 {
	t.Helper()
	next := current
	next.Revision++
	next.State = state
	next.InvalidationReason = core.ReasonReviewVerdictStale
	next.UpdatedUnixNano = now.UnixNano()
	sealed, err := ports.SealOperationReviewAuthorizationFactV4(next)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}
